package tsdb

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nrednav/cuid2"
	"mdemg/internal/llmclient"
)

// RetrievalEventRow is the TSDB-side representation of a retrieval pipeline event.
type RetrievalEventRow struct {
	Time              time.Time
	EventID           string
	SpaceID           string
	CallSite          string
	QueryText         string
	QueryHash         string
	RecallNodeIDs     []string
	RecallScores      []float64
	RecallK           int
	BM25NodeIDs       []string
	BM25Scores        []float64
	RerankNodeIDs     []string
	RerankScores      []float64
	RerankModel       string
	ResultNodeIDs     []string
	ResultScores      []float64
	ResultCount       int
	GuidanceID        string
	DownstreamQuality float64
	RecallLatencyMs   int
	RerankLatencyMs   int
	TotalLatencyMs int
	InstanceID     string
}

// RetrievalEventWriter buffers retrieval events and flushes them to the
// retrieval_events hypertable. Same pattern as LLMInteractionWriter.
type RetrievalEventWriter struct {
	pool         poolIface
	buffer       []RetrievalEventRow
	mu           sync.Mutex
	flushTick    *time.Ticker
	done         chan struct{}
	flushSuccess atomic.Int64
	flushFailure atomic.Int64
	flushRows    atomic.Int64
}

// NewRetrievalEventWriter creates a writer that auto-flushes at the given interval.
func NewRetrievalEventWriter(pool poolIface, flushInterval time.Duration) *RetrievalEventWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &RetrievalEventWriter{
		pool:   pool,
		buffer: make([]RetrievalEventRow, 0, 32),
		done:   make(chan struct{}),
	}
	registerWriterStats("retrieval_events", w.Stats)
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *RetrievalEventWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("retrieval_events: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers a retrieval event row.
//
// EXPORT-SCRUB-INTAKE-001 (2026-08-11): scrubs QueryText at INTAKE time so
// the row is clean-by-construction and the export scan-gate never blocks
// on a workspace-path false-positive. Skip list matches
// `retrievalEventsSpec.textFields[4] = {"abs_path"}` in exporter.go
// EXACTLY (must stay in sync): queries reference file paths BY DESIGN
// (retrieval by file-path is a legitimate query shape), so abs_path
// stays raw; api_key / env_secret / email / neo4j_cred still scrub if
// they somehow appear in a query.
func (w *RetrievalEventWriter) Record(event RetrievalEventRow) {
	if event.EventID == "" {
		event.EventID = cuid2.Generate()
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	// Intake scrub — abs_path skipped (matches export spec).
	event.QueryText = llmclient.ScrubStringExcluding(event.QueryText, []string{"abs_path"})
	w.mu.Lock()
	w.buffer = append(w.buffer, event)
	w.mu.Unlock()
}

// Flush writes all buffered events to TimescaleDB and clears the buffer.
func (w *RetrievalEventWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]RetrievalEventRow, 0, 32)
	w.mu.Unlock()

	rows := make([][]any, len(batch))
	for i, e := range batch {
		rows[i] = []any{
			e.Time, e.EventID, e.SpaceID, e.CallSite,
			e.QueryText, e.QueryHash,
			e.RecallNodeIDs, e.RecallScores, e.RecallK,
			e.BM25NodeIDs, e.BM25Scores,
			e.RerankNodeIDs, e.RerankScores, e.RerankModel,
			e.ResultNodeIDs, e.ResultScores, e.ResultCount,
			e.GuidanceID, e.DownstreamQuality,
			e.RecallLatencyMs, e.RerankLatencyMs, e.TotalLatencyMs,
			e.InstanceID,
		}
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"retrieval_events"},
		[]string{
			"time", "event_id", "space_id", "call_site",
			"query_text", "query_hash",
			"recall_node_ids", "recall_scores", "recall_k",
			"bm25_node_ids", "bm25_scores",
			"rerank_node_ids", "rerank_scores", "rerank_model",
			"result_node_ids", "result_scores", "result_count",
			"guidance_id", "downstream_quality",
			"recall_latency_ms", "rerank_latency_ms", "total_latency_ms",
			"instance_id",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("retrieval_events: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
	} else {
		slog.Debug("retrieval_events: flushed", "count", len(batch))
		w.flushSuccess.Add(1)
		w.flushRows.Add(int64(len(batch)))
	}
	return err
}

// Stats returns flush operation counters for monitoring.
func (w *RetrievalEventWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
	}
}

// Close stops the auto-flush goroutine and flushes remaining records.
func (w *RetrievalEventWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("retrieval_events: final flush failed", "error", err)
	}
}
