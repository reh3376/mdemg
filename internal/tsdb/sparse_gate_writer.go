// Phase 14 Epic 1 — Note 06 sparse_gate_metrics writer (V0019).
//
// One row per retrieve call where the Note 06 percentile gate fired (either
// via SPARSE_RETRIEVAL_ENABLED config default or per-request override).
// Phase 14.1 reads this hypertable to retune the percentile / clamps from
// production traffic instead of synthetic distributions. Buffered + flushed
// via CopyFrom on the standard TSDB_FLUSH_INTERVAL_SEC cadence (default 30s)
// — mirrors the LLMEndpointHealthWriter (V0018) and RetrievalAuditWriter
// (V0017) patterns.
//
// Like the audit writer, this lives in tsdb (not retrieval) to avoid the
// import cycle that would block compile (retrieval already imports tsdb for
// other recorders). The api adapter at internal/api/server.go translates the
// retrieval-side type into SparseGateMetricRow before forwarding.
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

// SparseGateMetricRow mirrors the V0019 sparse_gate_metrics columns.
type SparseGateMetricRow struct {
	SpaceID            string
	PercentileApplied  float64
	ThresholdScore     float64
	InputCount         int
	ActiveCount        int
	DroppedCount       int
	FloorApplied       bool
	CeilingApplied     bool
	ScorerVersion      string
}

// SparseGateMetricsWriter buffers V0019 rows and flushes them via CopyFrom.
type SparseGateMetricsWriter struct {
	pool         poolIface
	buffer       []SparseGateMetricRow
	mu           sync.Mutex
	flushTick    *time.Ticker
	done         chan struct{}
	flushSuccess atomic.Int64
	flushFailure atomic.Int64
	flushRows    atomic.Int64
}

// NewSparseGateMetricsWriter constructs a buffered writer auto-flushing at
// the given interval. flushInterval ≤ 0 falls back to 30s.
func NewSparseGateMetricsWriter(pool poolIface, flushInterval time.Duration) *SparseGateMetricsWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &SparseGateMetricsWriter{
		pool:   pool,
		buffer: make([]SparseGateMetricRow, 0, 64),
		done:   make(chan struct{}),
	}
	registerWriterStats("sparse_gate_metrics", w.Stats)
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *SparseGateMetricsWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("sparse_gate_metrics: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers one row.
func (w *SparseGateMetricsWriter) Record(row SparseGateMetricRow) {
	w.mu.Lock()
	w.buffer = append(w.buffer, row)
	w.mu.Unlock()
}

// Flush writes all buffered rows via CopyFrom and clears the buffer.
func (w *SparseGateMetricsWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]SparseGateMetricRow, 0, 64)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	now := time.Now()
	for _, rec := range batch {
		var threshold any
		if rec.ThresholdScore != 0 {
			threshold = rec.ThresholdScore
		}
		var scorer any
		if rec.ScorerVersion != "" {
			scorer = rec.ScorerVersion
		}
		rows = append(rows, []any{
			cuid2.Generate(),
			now,
			rec.SpaceID,
			rec.PercentileApplied,
			threshold,
			rec.InputCount,
			rec.ActiveCount,
			rec.DroppedCount,
			rec.FloorApplied,
			rec.CeilingApplied,
			scorer,
		})
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"sparse_gate_metrics"},
		[]string{
			"metric_id", "recorded_at", "space_id",
			"percentile_applied", "threshold_score",
			"input_count", "active_count", "dropped_count",
			"floor_applied", "ceiling_applied",
			"scorer_version",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("sparse_gate_metrics: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
	} else {
		slog.Debug("sparse_gate_metrics: flushed", "count", len(batch))
		w.flushSuccess.Add(1)
		w.flushRows.Add(int64(len(batch)))
	}
	return err
}

// Stats returns flush counters for monitoring.
func (w *SparseGateMetricsWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
	}
}

// Close stops the auto-flush goroutine and flushes remaining rows.
func (w *SparseGateMetricsWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("sparse_gate_metrics: final flush failed", "error", err)
	}
}
