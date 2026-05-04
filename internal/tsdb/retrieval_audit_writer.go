// Phase 14 Epic 0 follow-up — wire the V0017 retrieval_audit writer.
//
// Phase 13 Epic 6 shipped the V0017 schema, the RetrievalAuditWriter interface,
// and the service-side write site (internal/retrieval/service.go:847) but never
// wired a concrete writer at server startup. As a result V0017 has been empty
// since Phase 13 merged. Phase 14 Epic 0 needs that audit data to inform Note
// 06 percentile defaults; this writer closes that gap.
//
// Mirrors the LLMEndpointHealthWriter (V0018) pattern: buffer + flush via
// CopyFrom at a configurable interval. Audit row volume is one-per-retrieve —
// modest steady-state but bursty under UVTS A/B; batching keeps DB load
// predictable.
//
// Note: this writer does NOT import internal/retrieval (which would create an
// import cycle since retrieval imports tsdb). The adapter that translates
// retrieval.RetrievalAuditRecord → tsdb.RetrievalAuditRow lives in
// internal/api/server.go alongside retrievalRecorderAdapter.
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

// RetrievalAuditRow is the tsdb-package representation of one retrieval_audit
// row. Mirrors retrieval.RetrievalAuditRecord but without importing it (cycle
// avoidance). The api adapter translates between the two.
type RetrievalAuditRow struct {
	SpaceID            string
	QueryTextHash      string
	ScorerVersion      string
	ConsensusStrength  float64
	PerColumnLatencyMs map[string]int64 // already converted from time.Duration
	ColumnsQueried     int
	ColumnsReturned    int
	TopKNodeIDs        []string
	TotalLatencyMs     int64
}

// RetrievalAuditWriter buffers retrieval_audit rows and flushes them via
// CopyFrom.
type RetrievalAuditWriter struct {
	pool         poolIface
	buffer       []RetrievalAuditRow
	mu           sync.Mutex
	flushTick    *time.Ticker
	done         chan struct{}
	flushSuccess atomic.Int64
	flushFailure atomic.Int64
	flushRows    atomic.Int64
}

// NewRetrievalAuditWriter constructs a buffered writer auto-flushing at the
// given interval. flushInterval ≤ 0 falls back to 30s.
func NewRetrievalAuditWriter(pool poolIface, flushInterval time.Duration) *RetrievalAuditWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &RetrievalAuditWriter{
		pool:   pool,
		buffer: make([]RetrievalAuditRow, 0, 64),
		done:   make(chan struct{}),
	}
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *RetrievalAuditWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("retrieval_audit: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers one row. The api adapter calls this after translating
// retrieval.RetrievalAuditRecord → RetrievalAuditRow.
func (w *RetrievalAuditWriter) Record(row RetrievalAuditRow) {
	w.mu.Lock()
	w.buffer = append(w.buffer, row)
	w.mu.Unlock()
}

// Flush writes all buffered rows via CopyFrom and clears the buffer.
func (w *RetrievalAuditWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]RetrievalAuditRow, 0, 64)
	w.mu.Unlock()

	rows := make([][]any, 0, len(batch))
	now := time.Now()
	for _, rec := range batch {
		var perColJSON any
		if len(rec.PerColumnLatencyMs) > 0 {
			b, err := json.Marshal(rec.PerColumnLatencyMs)
			if err == nil {
				perColJSON = string(b)
			}
		}
		var topK any
		if len(rec.TopKNodeIDs) > 0 {
			topK = rec.TopKNodeIDs
		}
		var queryHash any
		if rec.QueryTextHash != "" {
			queryHash = rec.QueryTextHash
		}
		rows = append(rows, []any{
			cuid2.Generate(),
			now,
			rec.SpaceID,
			queryHash,
			rec.ScorerVersion,
			rec.ConsensusStrength,
			perColJSON,
			rec.ColumnsQueried,
			rec.ColumnsReturned,
			topK,
			rec.TotalLatencyMs,
		})
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"retrieval_audit"},
		[]string{
			"audit_id", "recorded_at", "space_id", "query_text_hash",
			"scorer_version", "consensus_strength", "per_column_latency",
			"columns_queried", "columns_returned", "top_k_node_ids",
			"total_latency_ms",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("retrieval_audit: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
	} else {
		slog.Debug("retrieval_audit: flushed", "count", len(batch))
		w.flushSuccess.Add(1)
		w.flushRows.Add(int64(len(batch)))
	}
	return err
}

// Stats returns flush counters for monitoring.
func (w *RetrievalAuditWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
	}
}

// Close stops the auto-flush goroutine and flushes remaining rows.
func (w *RetrievalAuditWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("retrieval_audit: final flush failed", "error", err)
	}
}
