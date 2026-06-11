// Sprint EVENTGRAPH-001 Epic 2 — reinforcement_events writer (V0022).
//
// One row per Hebbian co-activation pair update produced by
// internal/learning/service.go::ApplyCoactivation. Buffered + flushed via
// CopyFrom on the standard TSDB_FLUSH_INTERVAL_SEC cadence (default 30s).
// Pattern mirrors V0019 SparseGateMetricsWriter; explicitly NOT V0021
// model_install_events (single-row INSERT) because Hebbian writes are
// per-retrieve volume, far higher than CLI-driven events.
//
// Buffer-full handling: FIFO eviction of oldest rows when the cap is hit,
// matching the LLMInteractionWriter precedent. Eviction is counted in
// droppedRows for Prometheus surfacing (Epic 6 adds the gauge).
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

// ReinforcementEventRow mirrors the V0022 reinforcement_events columns. The
// writer treats string fields as "" → NULL and float64 fields as 0 → NULL
// (eta_effective, surprise_factor, activation_product, path_sim are optional).
// EvidenceCountAfter and the weight triplet are required.
type ReinforcementEventRow struct {
	SpaceID            string
	SrcNodeID          string
	DstNodeID          string
	PrevWeight         float64
	NewWeight          float64
	DeltaWeight        float64 // signed (NewWeight - PrevWeight); negative for weakening
	EvidenceCountAfter int
	EtaEffective       float64 // optional: cfg.LearningEta * etaMult
	SurpriseFactor     float64 // optional: 1.0 | 1.5 | 2.0
	ActivationProduct  float64 // optional: ai * aj
	PathSim            float64 // optional: pathPrefixSimilarity
	RoleA              string
	RoleB              string
	ObsTypeA           string
	ObsTypeB           string
	SessionID          string
	Direction          string // "forward" | "reverse" | "bidirectional"
	CreatedNewEdge     bool
	TriggerPath        string // v1: always "apply_coactivation"
}

// PrometheusCounter is the narrow interface NewReinforcementEventsWriter
// accepts for optional Prometheus-counter wiring. Matches the Add method on
// metrics.Counter without forcing the tsdb package to import internal/metrics
// (which would cycle).
type PrometheusCounter interface {
	Add(delta int64)
}

// ReinforcementEventsWriter buffers V0022 rows and flushes them via CopyFrom.
// Lifecycle owned by internal/api/server.go (constructed at boot, Stop()
// called on graceful shutdown so the buffer drains before the process exits).
type ReinforcementEventsWriter struct {
	pool          poolIface
	buffer        []ReinforcementEventRow
	maxBufferSize int
	mu            sync.Mutex
	flushTick     *time.Ticker
	done          chan struct{}
	flushSuccess  atomic.Int64
	flushFailure  atomic.Int64
	flushRows     atomic.Int64
	droppedRows   atomic.Int64 // FIFO-evicted rows (buffer-full)

	// Optional Prometheus counters (mirror flushRows / droppedRows /
	// flushFailure). Wired by api/server.go after construction via
	// SetPrometheusCounters; nil means "metrics disabled" — no-op.
	promRowsEnqueued PrometheusCounter
	promRowsDropped  PrometheusCounter
	promFlushFailure PrometheusCounter
}

// SetPrometheusCounters wires the three Prometheus counters that mirror the
// writer's internal atomic counters. Optional — nil counters are tolerated
// (the writer continues to track stats via Stats() either way).
func (w *ReinforcementEventsWriter) SetPrometheusCounters(rowsEnqueued, rowsDropped, flushFailure PrometheusCounter) {
	w.promRowsEnqueued = rowsEnqueued
	w.promRowsDropped = rowsDropped
	w.promFlushFailure = flushFailure
}

// NewReinforcementEventsWriter constructs a buffered writer that auto-flushes
// at the given interval. flushInterval ≤ 0 falls back to 30s. maxBufferSize
// ≤ 0 means unlimited (the operator-disabled-cap escape hatch).
func NewReinforcementEventsWriter(pool poolIface, flushInterval time.Duration, maxBufferSize int) *ReinforcementEventsWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &ReinforcementEventsWriter{
		pool:          pool,
		buffer:        make([]ReinforcementEventRow, 0, 128),
		maxBufferSize: maxBufferSize,
		done:          make(chan struct{}),
	}
	registerWriterStats("reinforcement_events", func() FlushStats {
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

func (w *ReinforcementEventsWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("reinforcement_events: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers one row. Applies FIFO eviction when the buffer is at cap.
// Non-blocking; the Hebbian write path must never wait on the TSDB writer.
func (w *ReinforcementEventsWriter) Record(row ReinforcementEventRow) {
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
func (w *ReinforcementEventsWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]ReinforcementEventRow, 0, 128)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	now := time.Now()
	for _, rec := range batch {
		rows = append(rows, []any{
			cuid2.Generate(),
			now,
			rec.SpaceID,
			rec.SrcNodeID,
			rec.DstNodeID,
			rec.PrevWeight,
			rec.NewWeight,
			rec.DeltaWeight,
			rec.EvidenceCountAfter,
			nullableFloat(rec.EtaEffective),
			nullableFloat(rec.SurpriseFactor),
			nullableFloat(rec.ActivationProduct),
			nullableFloat(rec.PathSim),
			nullableString(rec.RoleA),
			nullableString(rec.RoleB),
			nullableString(rec.ObsTypeA),
			nullableString(rec.ObsTypeB),
			nullableString(rec.SessionID),
			nullableString(rec.Direction),
			rec.CreatedNewEdge,
			rec.TriggerPath,
		})
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"reinforcement_events"},
		[]string{
			"event_id", "recorded_at", "space_id",
			"src_node_id", "dst_node_id",
			"prev_weight", "new_weight", "delta_weight",
			"evidence_count_after",
			"eta_effective", "surprise_factor", "activation_product", "path_sim",
			"role_a", "role_b", "obs_type_a", "obs_type_b",
			"session_id", "direction", "created_new_edge", "trigger_path",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("reinforcement_events: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
		if w.promFlushFailure != nil {
			w.promFlushFailure.Add(1)
		}
		return err
	}
	slog.Debug("reinforcement_events: flushed", "count", len(batch))
	w.flushSuccess.Add(1)
	w.flushRows.Add(int64(len(batch)))
	if w.promRowsEnqueued != nil {
		w.promRowsEnqueued.Add(int64(len(batch)))
	}
	return nil
}

// Stats returns flush + drop counters for monitoring. DroppedRows surfaces
// in Epic 6 as mdemg_eventgraph_writer_rows_dropped_total.
type ReinforcementEventsStats struct {
	SuccessCount int64
	FailureCount int64
	TotalRows    int64
	DroppedRows  int64
}

func (w *ReinforcementEventsWriter) Stats() ReinforcementEventsStats {
	return ReinforcementEventsStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
		DroppedRows:  w.droppedRows.Load(),
	}
}

// Close stops the auto-flush goroutine and drains the buffer with a final
// flush. Idempotent — safe to call multiple times because the done channel
// is closed on first call and Flush no-ops on an empty buffer.
func (w *ReinforcementEventsWriter) Close() {
	w.flushTick.Stop()
	select {
	case <-w.done:
		// already closed
	default:
		close(w.done)
	}
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("reinforcement_events: final flush failed", "error", err)
	}
}

// nullableFloat returns nil (→ DB NULL) for the zero value, else the value.
// The Hebbian path emits 0 when a field isn't applicable for a given pair;
// we'd rather see NULL in analytic queries than confuse 0 with "unset."
func nullableFloat(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
