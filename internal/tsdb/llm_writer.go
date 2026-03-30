package tsdb

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"mdemg/internal/llmclient"

	"github.com/nrednav/cuid2"
)

// LLMInteractionWriter buffers LLM interaction records and flushes them to
// the llm_interactions hypertable. It mirrors the MetricWriter pattern.
type LLMInteractionWriter struct {
	pool      poolIface
	buffer    []llmclient.InteractionRecord
	mu        sync.Mutex
	flushTick *time.Ticker
	done      chan struct{}
}

// poolIface allows testing without a real pgxpool.Pool.
type poolIface interface {
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string,
		rowSrc pgx.CopyFromSource) (int64, error)
}

// NewLLMInteractionWriter creates a writer that auto-flushes at the given interval.
func NewLLMInteractionWriter(pool poolIface, flushInterval time.Duration) *LLMInteractionWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &LLMInteractionWriter{
		pool:   pool,
		buffer: make([]llmclient.InteractionRecord, 0, 32),
		done:   make(chan struct{}),
	}
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *LLMInteractionWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("llm_interactions: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record implements llmclient.InteractionRecorder.
func (w *LLMInteractionWriter) Record(_ context.Context, rec llmclient.InteractionRecord) {
	if rec.TraceID == "" {
		rec.TraceID = cuid2.Generate()
	}
	llmclient.Scrub(&rec)
	w.mu.Lock()
	w.buffer = append(w.buffer, rec)
	w.mu.Unlock()
}

// Flush writes all buffered interactions to TimescaleDB and clears the buffer.
func (w *LLMInteractionWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]llmclient.InteractionRecord, 0, 32)
	w.mu.Unlock()

	rows := make([][]any, len(batch))
	for i, r := range batch {
		// Extract RAFT context fields
		var retrievalNodeIDs []string
		var retrievalScores []float64
		var oracleNodeID string
		if r.RetrievalCtx != nil {
			retrievalNodeIDs = r.RetrievalCtx.NodeIDs
			retrievalScores = r.RetrievalCtx.Scores
			oracleNodeID = r.RetrievalCtx.OracleID
		}
		rows[i] = []any{
			r.Time, r.TraceID, r.TaskName, r.SpaceID, r.SessionID,
			r.SystemPrompt, r.UserPrompt, r.Response,
			r.ThinkContent, r.ThinkMode,
			r.LatencyMs, r.TokensIn, r.TokensOut,
			r.ModelName, r.Provider, r.Error,
			r.Quality, r.QualitySource,
			false, "", // used_for_train, dataset_ver
			r.GuidanceID, r.SourcePath,
			retrievalNodeIDs, retrievalScores, oracleNodeID,
			r.SystemPromptHash,
		}
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"llm_interactions"},
		[]string{
			"time", "trace_id", "task_name", "space_id", "session_id",
			"system_prompt", "user_prompt", "response",
			"think_content", "think_mode",
			"latency_ms", "tokens_in", "tokens_out",
			"model_name", "provider", "error",
			"quality", "quality_source",
			"used_for_train", "dataset_ver",
			"guidance_id", "source_path",
			"retrieval_node_ids", "retrieval_scores", "oracle_node_id",
			"system_prompt_hash",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("llm_interactions: flush failed", "count", len(batch), "error", err)
	} else {
		slog.Debug("llm_interactions: flushed", "count", len(batch))
	}
	return err
}

// Close stops the auto-flush goroutine and flushes remaining records.
func (w *LLMInteractionWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("llm_interactions: final flush failed", "error", err)
	}
}
