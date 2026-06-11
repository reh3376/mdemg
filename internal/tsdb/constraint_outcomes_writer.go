package tsdb

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// ConstraintOutcomeRow represents a single guidance outcome event for TSDB storage.
type ConstraintOutcomeRow struct {
	Time           time.Time
	SpaceID        string
	ConstraintID   string
	ConstraintCode string
	GuidanceID     string
	SessionID      string
	OutcomeType    string
	Similarity     float64
	GuidanceType   string
	InstanceID     string
}

// ConstraintOutcomesWriter buffers constraint outcome events and flushes them
// to the constraint_outcomes hypertable. Grafana queries this table directly
// to compute Constraint Effectiveness over any user-selected time range.
type ConstraintOutcomesWriter struct {
	pool         poolIface
	buffer       []ConstraintOutcomeRow
	mu           sync.Mutex
	flushTick    *time.Ticker
	done         chan struct{}
	flushSuccess atomic.Int64
	flushFailure atomic.Int64
	flushRows    atomic.Int64
}

// NewConstraintOutcomesWriter creates a writer that auto-flushes at the given interval.
func NewConstraintOutcomesWriter(pool poolIface, flushInterval time.Duration) *ConstraintOutcomesWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &ConstraintOutcomesWriter{
		pool:   pool,
		buffer: make([]ConstraintOutcomeRow, 0, 32),
		done:   make(chan struct{}),
	}
	registerWriterStats("constraint_outcomes", w.Stats)
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *ConstraintOutcomesWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("constraint_outcomes: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// RecordOutcome implements jiminy.OutcomeWriter by converting and buffering the record.
// This avoids an import cycle between jiminy and tsdb packages.
func (w *ConstraintOutcomesWriter) RecordOutcome(spaceID, constraintID, constraintCode, guidanceID, sessionID, outcomeType, guidanceType, instanceID string, similarity float64) {
	w.Record(ConstraintOutcomeRow{
		SpaceID:        spaceID,
		ConstraintID:   constraintID,
		ConstraintCode: constraintCode,
		GuidanceID:     guidanceID,
		SessionID:      sessionID,
		OutcomeType:    outcomeType,
		Similarity:     similarity,
		GuidanceType:   guidanceType,
		InstanceID:     instanceID,
	})
}

// Record buffers a constraint outcome event.
func (w *ConstraintOutcomesWriter) Record(row ConstraintOutcomeRow) {
	if row.Time.IsZero() {
		row.Time = time.Now()
	}
	w.mu.Lock()
	w.buffer = append(w.buffer, row)
	w.mu.Unlock()
}

// Flush writes all buffered events to TimescaleDB and clears the buffer.
func (w *ConstraintOutcomesWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]ConstraintOutcomeRow, 0, 32)
	w.mu.Unlock()

	rows := make([][]any, len(batch))
	for i, r := range batch {
		rows[i] = []any{
			r.Time, r.SpaceID, r.ConstraintID, r.ConstraintCode,
			r.GuidanceID, r.SessionID, r.OutcomeType,
			r.Similarity, r.GuidanceType, r.InstanceID,
		}
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"constraint_outcomes"},
		[]string{
			"time", "space_id", "constraint_id", "constraint_code",
			"guidance_id", "session_id", "outcome_type",
			"similarity", "guidance_type", "instance_id",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("constraint_outcomes: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
	} else {
		slog.Debug("constraint_outcomes: flushed", "count", len(batch))
		w.flushSuccess.Add(1)
		w.flushRows.Add(int64(len(batch)))
	}
	return err
}

// Stats returns flush operation counters for monitoring.
func (w *ConstraintOutcomesWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
	}
}

// Close stops the auto-flush goroutine and flushes remaining records.
func (w *ConstraintOutcomesWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("constraint_outcomes: final flush failed", "error", err)
	}
}
