// Phase 13.5 — LLM endpoint health events writer.
//
// Persists watchdog state-transition events + fast-fail bursts to the
// `llm_endpoint_health_events` hypertable (V0018). Low write volume by design
// — the watchdog only fires OnTransition on actual state changes, and
// fast-fail bursts are aggregated into a single row per rate-limit window.
// The historical record drives the Grafana "LLM Endpoint Health" panels on
// the Fine-Tuning Pipeline dashboard, and survives mdemg restarts (unlike
// process-local Prometheus counters).
package tsdb

import (
	"context"
	"log/slog"
	"mdemg/internal/sanitize"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nrednav/cuid2"
)

// LLMEndpointHealthEvent represents a single watchdog event for TSDB storage.
// Events are sparse: one row per state transition, plus rate-limited rows
// during fast-fail bursts. Steady state is silent.
type LLMEndpointHealthEvent struct {
	Time                time.Time
	EventID             string // CUIDv2 — auto-filled in Record() if empty
	EndpointURL         string // e.g. "http://127.0.0.1:8102/v1"
	EventKind           string // "state_transition" | "fast_fail_burst" | "probe_recovery"
	FromState           string // "up" | "degraded" | "down" | "unknown" (transitions only)
	ToState             string // "up" | "degraded" | "down" (transitions only; same as FromState for bursts)
	ErrorMessage        string // last probe error if any (truncated to 500 chars)
	ConsecutiveFailures int    // probe failure counter at event time
	BurstShortCircuits  int    // for fast_fail_burst: short-circuits aggregated into this row
	StateNumeric        int    // 0=up, 1=degraded, 2=down — explicit for fast Grafana queries
}

// LLMEndpointHealthWriter buffers endpoint-health events and flushes them to
// the llm_endpoint_health_events hypertable. Mirrors ConstraintOutcomesWriter
// (V0011/V0014) pattern.
type LLMEndpointHealthWriter struct {
	pool         poolIface
	buffer       []LLMEndpointHealthEvent
	mu           sync.Mutex
	flushTick    *time.Ticker
	done         chan struct{}
	flushSuccess atomic.Int64
	flushFailure atomic.Int64
	flushRows    atomic.Int64
}

// NewLLMEndpointHealthWriter creates a writer that auto-flushes at the given
// interval. Default flush interval is 30s — events are sparse so even an
// indefinitely-buffered writer would never accumulate much, but predictable
// flush cadence keeps Grafana panels close to live.
func NewLLMEndpointHealthWriter(pool poolIface, flushInterval time.Duration) *LLMEndpointHealthWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &LLMEndpointHealthWriter{
		pool:   pool,
		buffer: make([]LLMEndpointHealthEvent, 0, 8),
		done:   make(chan struct{}),
	}
	registerWriterStats("llm_endpoint_health_events", w.Stats)
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *LLMEndpointHealthWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("llm_endpoint_health: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers a health event. The Time + EventID fields are auto-filled
// when zero/empty so callers can construct minimal rows.
func (w *LLMEndpointHealthWriter) Record(ev LLMEndpointHealthEvent) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	if ev.EventID == "" {
		ev.EventID = cuid2.Generate()
	}
	ev.ErrorMessage = sanitize.CutRuneSafe(ev.ErrorMessage, 500)
	w.mu.Lock()
	w.buffer = append(w.buffer, ev)
	w.mu.Unlock()
}

// Flush writes all buffered events to TimescaleDB and clears the buffer.
func (w *LLMEndpointHealthWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]LLMEndpointHealthEvent, 0, 8)
	w.mu.Unlock()

	rows := make([][]any, len(batch))
	for i, ev := range batch {
		var errMsg, fromState, toState any
		if ev.ErrorMessage != "" {
			errMsg = ev.ErrorMessage
		}
		if ev.FromState != "" {
			fromState = ev.FromState
		}
		if ev.ToState != "" {
			toState = ev.ToState
		}
		var burstSC any
		if ev.BurstShortCircuits > 0 {
			burstSC = ev.BurstShortCircuits
		}
		rows[i] = []any{
			ev.EventID,
			ev.Time,
			ev.EndpointURL,
			ev.EventKind,
			fromState,
			toState,
			errMsg,
			ev.ConsecutiveFailures,
			burstSC,
			ev.StateNumeric,
		}
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"llm_endpoint_health_events"},
		[]string{
			"event_id", "recorded_at", "endpoint_url", "event_kind",
			"from_state", "to_state", "error_message",
			"consecutive_failures", "burst_short_circuits", "state_numeric",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("llm_endpoint_health: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
	} else {
		slog.Debug("llm_endpoint_health: flushed", "count", len(batch))
		w.flushSuccess.Add(1)
		w.flushRows.Add(int64(len(batch)))
	}
	return err
}

// Stats returns flush operation counters for monitoring.
func (w *LLMEndpointHealthWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
	}
}

// Close stops the auto-flush goroutine and flushes remaining records.
func (w *LLMEndpointHealthWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("llm_endpoint_health: final flush failed", "error", err)
	}
}
