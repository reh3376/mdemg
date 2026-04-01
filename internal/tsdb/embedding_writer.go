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

// EmbeddingEventRow is the TSDB-side representation of an embedding event.
// This mirrors embeddings.EmbeddingEvent without importing the embeddings
// package (which would create an import cycle via metrics → tsdb).
type EmbeddingEventRow struct {
	Time        time.Time
	EventID     string
	EventType   string
	SpaceID     string
	TextContent string
	TextHash    string
	TextLength  int
	ElementKind string
	Language    string
	FilePath    string
	ChunkStart  int
	ChunkEnd    int
	PackageName string
	Signature   string
	Tags        []string
	CallSite    string
	QueryText   string
	ModelName   string
	Provider    string
	Dimensions  int
	LatencyMs   int
	Cached      bool
	NodeID     string
	InstanceID string
}

// EmbeddingEventWriter buffers embedding events and flushes them to the
// embedding_events hypertable. Same pattern as LLMInteractionWriter.
type EmbeddingEventWriter struct {
	pool         poolIface
	buffer       []EmbeddingEventRow
	mu           sync.Mutex
	flushTick    *time.Ticker
	done         chan struct{}
	flushSuccess atomic.Int64
	flushFailure atomic.Int64
	flushRows    atomic.Int64
}

// NewEmbeddingEventWriter creates a writer that auto-flushes at the given interval.
func NewEmbeddingEventWriter(pool poolIface, flushInterval time.Duration) *EmbeddingEventWriter {
	if flushInterval <= 0 {
		flushInterval = 30 * time.Second
	}
	w := &EmbeddingEventWriter{
		pool:   pool,
		buffer: make([]EmbeddingEventRow, 0, 32),
		done:   make(chan struct{}),
	}
	w.flushTick = time.NewTicker(flushInterval)
	go w.flushLoop()
	return w
}

func (w *EmbeddingEventWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("embedding_events: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers an embedding event row. Privacy scrubbing is applied to TextContent.
func (w *EmbeddingEventWriter) Record(event EmbeddingEventRow) {
	if event.EventID == "" {
		event.EventID = cuid2.Generate()
	}
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	event.TextContent = llmclient.ScrubString(event.TextContent)
	w.mu.Lock()
	w.buffer = append(w.buffer, event)
	w.mu.Unlock()
}

// Flush writes all buffered events to TimescaleDB and clears the buffer.
func (w *EmbeddingEventWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]EmbeddingEventRow, 0, 32)
	w.mu.Unlock()

	rows := make([][]any, len(batch))
	for i, e := range batch {
		rows[i] = []any{
			e.Time, e.EventID, e.EventType, e.SpaceID,
			e.TextContent, e.TextHash, e.TextLength,
			e.ElementKind, e.Language, e.FilePath,
			e.ChunkStart, e.ChunkEnd,
			e.PackageName, e.Signature, e.Tags,
			e.CallSite, e.QueryText,
			e.ModelName, e.Provider, e.Dimensions,
			e.LatencyMs, e.Cached, e.NodeID,
			e.InstanceID,
		}
	}

	_, err := w.pool.CopyFrom(ctx,
		pgx.Identifier{"embedding_events"},
		[]string{
			"time", "event_id", "event_type", "space_id",
			"text_content", "text_hash", "text_length",
			"element_kind", "language", "file_path",
			"chunk_start", "chunk_end",
			"package_name", "signature", "tags",
			"call_site", "query_text",
			"model_name", "provider", "dimensions",
			"latency_ms", "cached", "node_id",
			"instance_id",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		slog.Error("embedding_events: flush failed", "count", len(batch), "error", err)
		w.flushFailure.Add(1)
	} else {
		slog.Debug("embedding_events: flushed", "count", len(batch))
		w.flushSuccess.Add(1)
		w.flushRows.Add(int64(len(batch)))
	}
	return err
}

// Stats returns flush operation counters for monitoring.
func (w *EmbeddingEventWriter) Stats() FlushStats {
	return FlushStats{
		SuccessCount: w.flushSuccess.Load(),
		FailureCount: w.flushFailure.Load(),
		TotalRows:    w.flushRows.Load(),
	}
}

// Close stops the auto-flush goroutine and flushes remaining records.
func (w *EmbeddingEventWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("embedding_events: final flush failed", "error", err)
	}
}
