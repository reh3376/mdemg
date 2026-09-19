// Sprint JIMINY-PROCESS-OBSERVER-01 (task #160) — process_outcomes writer (V0036).
//
// Parallel to ProcessEventsWriter but written from the grader loop side
// (internal/grader/process/loop.go), not from HTTP ingress. Same buffered
// CopyFrom pattern as constraint_outcomes_writer / reinforcement_writer.
// Grader emits one row per (matcher_name, session_id, constraint_code)
// verdict; DatasetProvider.GuidanceEffectivenessByClass UNIONs these into
// the process class aggregate that feeds the follow_rate_process_verifiable
// gauge shipped in #158.
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

// ProcessOutcomeRow mirrors the V0036 process_outcomes columns.
type ProcessOutcomeRow struct {
	SpaceID           string
	SessionID         string
	ConstraintNodeID  string
	ConstraintCode    string
	OutcomeType       string // process_followed | process_missed | process_incomplete
	EvidenceEventID   string
	Reason            string
	MatcherName       string
	Time              time.Time // optional; zero → NOW()
}

// ProcessOutcomesWriter buffers V0036 process_outcomes rows.
type ProcessOutcomesWriter struct {
	pool          poolIface
	buffer        []ProcessOutcomeRow
	maxBufferSize int
	mu            sync.Mutex
	flushTick     *time.Ticker
	done          chan struct{}
	flushSuccess  atomic.Int64
	flushFailure  atomic.Int64
	flushRows     atomic.Int64
	droppedRows   atomic.Int64
}

func NewProcessOutcomesWriter(pool poolIface, flushInterval time.Duration, maxBufferSize int) *ProcessOutcomesWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &ProcessOutcomesWriter{
		pool:          pool,
		buffer:        make([]ProcessOutcomeRow, 0, 64),
		maxBufferSize: maxBufferSize,
		done:          make(chan struct{}),
	}
	registerWriterStats("process_outcomes", func() FlushStats {
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

func (w *ProcessOutcomesWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("process_outcomes: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

func (w *ProcessOutcomesWriter) Record(row ProcessOutcomeRow) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.maxBufferSize > 0 && len(w.buffer) >= w.maxBufferSize {
		evict := len(w.buffer) - w.maxBufferSize + 1
		w.buffer = w.buffer[evict:]
		w.droppedRows.Add(int64(evict))
	}
	w.buffer = append(w.buffer, row)
	w.mu.Unlock()
}

func (w *ProcessOutcomesWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]ProcessOutcomeRow, 0, 64)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	nowFallback := time.Now()
	for _, rec := range batch {
		ts := rec.Time
		if ts.IsZero() {
			ts = nowFallback
		}
		rows = append(rows, []any{
			cuid2.Generate(),
			ts,
			rec.SpaceID,
			rec.SessionID,
			rec.ConstraintNodeID,
			rec.ConstraintCode,
			rec.OutcomeType,
			rec.EvidenceEventID,
			rec.Reason,
			rec.MatcherName,
			"process", // verifiability_class always "process" in this table
		})
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"process_outcomes"},
		[]string{
			"outcome_id", "time", "space_id", "session_id",
			"constraint_node_id", "constraint_code",
			"outcome_type", "evidence_event_id", "reason",
			"matcher_name", "verifiability_class",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("process_outcomes: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
		return err
	}
	slog.Debug("process_outcomes: flushed", "count", len(batch))
	w.flushSuccess.Add(1)
	w.flushRows.Add(int64(len(batch)))
	return nil
}

type ProcessOutcomesStats struct {
	SuccessCount int64
	FailureCount int64
	TotalRows    int64
	DroppedRows  int64
}

func (w *ProcessOutcomesWriter) Stats() ProcessOutcomesStats {
	if w == nil {
		return ProcessOutcomesStats{}
	}
	return ProcessOutcomesStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
		DroppedRows:  w.droppedRows.Load(),
	}
}

func (w *ProcessOutcomesWriter) Close() {
	if w == nil {
		return
	}
	w.flushTick.Stop()
	select {
	case <-w.done:
	default:
		close(w.done)
	}
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("process_outcomes: final flush failed", "error", err)
	}
}
