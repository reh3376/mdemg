// Sprint JIMINY-RELEVANCE-001 Epic 1 — guidance_training_rows writer (V0027).
//
// One row per guidance-feedback item, persisting the TRAINING EVIDENCE that was
// previously discarded: the surfaced guidance text, its source-node
// role_type/layer, the agent-action text, and the audited verdict + classifier
// source. Buffered + flushed via CopyFrom on the standard flush cadence.
// Pattern mirrors V0022 ReinforcementEventsWriter (bounded FIFO buffer, drop
// counter, optional Prometheus counters) — this is per-prompt-feedback volume,
// so buffered, not a synchronous single-row INSERT.
//
// Buffer-full handling: FIFO eviction of oldest rows when the cap is hit; the
// feedback hot path must never block on the TSDB writer.
//
// ⚠️ This table carries NO gold-grading columns. Human/auto gold grades live in
// HITL-REVIEW-001's review_grades hypertable keyed by (dataset_id, item_id)
// where item_id == this row's row_id.
package tsdb

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nrednav/cuid2"
)

// GuidanceTrainingRow mirrors the V0027 guidance_training_rows columns. The
// writer treats optional string fields as "" → NULL via nullableString;
// SourceLayer is a *int so a valid layer 0 (raw L0 node) is distinguishable
// from "unresolved" (nil → NULL). RowID + Time are stamped at Record time if
// unset.
type GuidanceTrainingRow struct {
	RowID            string // CUIDv2; stamped at Record if empty
	Time             time.Time
	SpaceID          string
	SessionID        string
	InstanceID       string
	GuidanceID       string
	GuidanceType     string
	GuidanceContent  string // already truncated to MAX_CONTENT_BYTES at emit
	SourceNodeID     string
	SourceRoleType   string // "" when unresolved
	SourceLayer      *int   // nil when unresolved (layer 0 is valid)
	ActionSummary    string // already truncated to MAX_CONTENT_BYTES at emit
	OutcomeType      string // NOT NULL
	Similarity       float64
	ClassifierSource string // NOT NULL ("" allowed)
	ConstraintCode   string
}

// GuidanceTrainingRowsWriter buffers V0027 rows and flushes them via CopyFrom.
// Lifecycle owned by internal/api/server.go (constructed at boot, Close() called
// on graceful shutdown so the buffer drains before the process exits).
type GuidanceTrainingRowsWriter struct {
	pool          poolIface
	buffer        []GuidanceTrainingRow
	maxBufferSize int
	mu            sync.Mutex
	flushTick     *time.Ticker
	done          chan struct{}
	flushSuccess  atomic.Int64
	flushFailure  atomic.Int64
	flushRows     atomic.Int64
	droppedRows   atomic.Int64 // FIFO-evicted rows (buffer-full)

	// Optional Prometheus counters (mirror flushRows / droppedRows /
	// flushFailure). Wired by api/server.go after construction; nil = no-op.
	promRowsEnqueued PrometheusCounter
	promRowsDropped  PrometheusCounter
	promFlushFailure PrometheusCounter
}

// SetPrometheusCounters wires the three Prometheus counters that mirror the
// writer's internal atomic counters. Optional — nil counters are tolerated.
func (w *GuidanceTrainingRowsWriter) SetPrometheusCounters(rowsEnqueued, rowsDropped, flushFailure PrometheusCounter) {
	w.promRowsEnqueued = rowsEnqueued
	w.promRowsDropped = rowsDropped
	w.promFlushFailure = flushFailure
}

// NewGuidanceTrainingRowsWriter constructs a buffered writer that auto-flushes
// at the given interval. flushInterval ≤ 0 falls back to 30s. maxBufferSize ≤ 0
// means unlimited (the operator-disabled-cap escape hatch).
func NewGuidanceTrainingRowsWriter(pool poolIface, flushInterval time.Duration, maxBufferSize int) *GuidanceTrainingRowsWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &GuidanceTrainingRowsWriter{
		pool:          pool,
		buffer:        make([]GuidanceTrainingRow, 0, 128),
		maxBufferSize: maxBufferSize,
		done:          make(chan struct{}),
	}
	registerWriterStats("guidance_training_rows", func() FlushStats {
		st := w.Stats()
		return FlushStats{
			SuccessCount:  st.SuccessCount,
			FailureCount:  st.FailureCount,
			TotalRows:     st.TotalRows,
			OverflowCount: st.DroppedRows,
		}
	})
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *GuidanceTrainingRowsWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("guidance_training_rows: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers one row. Stamps RowID (CUIDv2) + Time if unset. Applies FIFO
// eviction when the buffer is at cap. Non-blocking; the feedback hot path must
// never wait on the TSDB writer.
func (w *GuidanceTrainingRowsWriter) Record(row GuidanceTrainingRow) {
	if row.RowID == "" {
		row.RowID = cuid2.Generate()
	}
	if row.Time.IsZero() {
		row.Time = time.Now()
	}
	w.mu.Lock()
	if w.maxBufferSize > 0 && len(w.buffer) >= w.maxBufferSize {
		evict := len(w.buffer) - w.maxBufferSize + 1
		w.buffer = w.buffer[evict:]
		w.droppedRows.Add(int64(evict))
		if w.promRowsDropped != nil {
			w.promRowsDropped.Add(int64(evict))
		}
	}
	w.buffer = append(w.buffer, row)
	w.mu.Unlock()
}

// Flush writes all buffered rows via CopyFrom and clears the buffer.
func (w *GuidanceTrainingRowsWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]GuidanceTrainingRow, 0, 128)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	for _, rec := range batch {
		var layer any
		if rec.SourceLayer != nil {
			layer = *rec.SourceLayer
		}
		rows = append(rows, []any{
			rec.RowID,
			rec.Time,
			rec.SpaceID,
			nullableString(rec.SessionID),
			nullableString(rec.InstanceID),
			rec.GuidanceID,
			nullableString(rec.GuidanceType),
			nullableString(rec.GuidanceContent),
			nullableString(rec.SourceNodeID),
			nullableString(rec.SourceRoleType),
			layer,
			nullableString(rec.ActionSummary),
			rec.OutcomeType,
			rec.Similarity,
			rec.ClassifierSource,
			nullableString(rec.ConstraintCode),
		})
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"guidance_training_rows"},
		[]string{
			"row_id", "time", "space_id", "session_id", "instance_id",
			"guidance_id", "guidance_type", "guidance_content",
			"source_node_id", "source_role_type", "source_layer",
			"action_summary", "outcome_type", "similarity",
			"classifier_source", "constraint_code",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("guidance_training_rows: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
		if w.promFlushFailure != nil {
			w.promFlushFailure.Add(1)
		}
		return err
	}
	slog.Debug("guidance_training_rows: flushed", "count", len(batch))
	w.flushSuccess.Add(1)
	w.flushRows.Add(int64(len(batch)))
	if w.promRowsEnqueued != nil {
		w.promRowsEnqueued.Add(int64(len(batch)))
	}
	return nil
}

// GuidanceTrainingRowsStats returns flush + drop counters for monitoring.
type GuidanceTrainingRowsStats struct {
	SuccessCount int64
	FailureCount int64
	TotalRows    int64
	DroppedRows  int64
}

func (w *GuidanceTrainingRowsWriter) Stats() GuidanceTrainingRowsStats {
	return GuidanceTrainingRowsStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
		DroppedRows:  w.droppedRows.Load(),
	}
}

// Close stops the auto-flush goroutine and drains the buffer with a final flush.
// Idempotent.
func (w *GuidanceTrainingRowsWriter) Close() {
	w.flushTick.Stop()
	select {
	case <-w.done:
		// already closed
	default:
		close(w.done)
	}
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("guidance_training_rows: final flush failed", "error", err)
	}
}
