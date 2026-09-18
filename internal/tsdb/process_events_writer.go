// Sprint JIMINY-PROCESS-OBSERVER-01 (task #160) — process_events writer (V0036).
//
// One row per client-side process-observation event. Written by the
// .claude/hooks/post-tool-observe.py hook via POST /v1/process/event →
// this buffered writer. Pattern mirrors ReinforcementEventsWriter (V0022) —
// buffered + CopyFrom, FIFO eviction on overflow, drop counter surfaced via
// registerWriterStats for the tsdb_writer_flush_failures alert family.
package tsdb

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nrednav/cuid2"
)

// ProcessEventRow mirrors the V0036 process_events columns. Text fields default
// to "" (writer serializes as-is); Metadata may be nil (serialized as {}).
// ExitCode is a pointer to distinguish "0" (real exit code) from "unset".
type ProcessEventRow struct {
	SpaceID      string
	SessionID    string
	EventType    string            // lint_run | git_commit | file_write | model_call | mcp_call | retrieval_call | bash_command | git_push | test_run | plan_mode_entry
	EventSubtype string            // e.g. "golangci-lint", "ruff", "git commit"
	Outcome      string            // success | failure | unknown
	ExitCode     *int              // optional
	Metadata     map[string]any    // free-form JSON per event type; nil → "{}"
	SourceHook   string            // emitter id
	Time         time.Time         // event time (client-supplied; server clamps to NOW() if zero)
}

// ProcessEventsWriter buffers V0036 rows and flushes via CopyFrom. Lifecycle
// owned by internal/cli/serve.go (constructed at boot, Close() on shutdown).
type ProcessEventsWriter struct {
	pool          poolIface
	buffer        []ProcessEventRow
	maxBufferSize int
	mu            sync.Mutex
	flushTick     *time.Ticker
	done          chan struct{}
	flushSuccess  atomic.Int64
	flushFailure  atomic.Int64
	flushRows     atomic.Int64
	droppedRows   atomic.Int64
}

// NewProcessEventsWriter constructs a buffered writer. flushInterval ≤ 0 falls
// back to 30s. maxBufferSize ≤ 0 means unlimited.
func NewProcessEventsWriter(pool poolIface, flushInterval time.Duration, maxBufferSize int) *ProcessEventsWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &ProcessEventsWriter{
		pool:          pool,
		buffer:        make([]ProcessEventRow, 0, 128),
		maxBufferSize: maxBufferSize,
		done:          make(chan struct{}),
	}
	registerWriterStats("process_events", func() FlushStats {
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

func (w *ProcessEventsWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("process_events: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers one row. FIFO-evicts on overflow. Non-blocking — HTTP hot
// path must not stall on the writer.
func (w *ProcessEventsWriter) Record(row ProcessEventRow) {
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

// Flush writes buffered rows via CopyFrom.
func (w *ProcessEventsWriter) Flush(ctx context.Context) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]ProcessEventRow, 0, 128)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	nowFallback := time.Now()
	for _, rec := range batch {
		ts := rec.Time
		if ts.IsZero() {
			ts = nowFallback
		}
		outcome := rec.Outcome
		if outcome == "" {
			outcome = "unknown"
		}
		var metaJSON []byte
		if rec.Metadata != nil {
			b, err := json.Marshal(rec.Metadata)
			if err == nil {
				metaJSON = b
			}
		}
		if metaJSON == nil {
			metaJSON = []byte(`{}`)
		}
		var exitCode any
		if rec.ExitCode != nil {
			exitCode = *rec.ExitCode
		}
		rows = append(rows, []any{
			cuid2.Generate(),
			ts,
			rec.SpaceID,
			rec.SessionID,
			rec.EventType,
			rec.EventSubtype,
			outcome,
			exitCode,
			metaJSON,
			rec.SourceHook,
		})
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"process_events"},
		[]string{
			"event_id", "time", "space_id", "session_id",
			"event_type", "event_subtype", "outcome",
			"exit_code", "metadata", "source_hook",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("process_events: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
		return err
	}
	slog.Debug("process_events: flushed", "count", len(batch))
	w.flushSuccess.Add(1)
	w.flushRows.Add(int64(len(batch)))
	return nil
}

// ProcessEventsStats snapshot of the writer's counters.
type ProcessEventsStats struct {
	SuccessCount int64
	FailureCount int64
	TotalRows    int64
	DroppedRows  int64
}

func (w *ProcessEventsWriter) Stats() ProcessEventsStats {
	if w == nil {
		return ProcessEventsStats{}
	}
	return ProcessEventsStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
		DroppedRows:  w.droppedRows.Load(),
	}
}

// Close stops the ticker and drains the buffer with a final flush.
func (w *ProcessEventsWriter) Close() {
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
		slog.Warn("process_events: final flush failed", "error", err)
	}
}
