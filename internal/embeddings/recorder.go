package embeddings

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"
)

// EmbeddingEvent captures a single Embed/EmbedBatch call for contrastive training data.
type EmbeddingEvent struct {
	Time        time.Time
	EventID     string
	EventType   string // "ingest", "query", "internal"
	SpaceID     string
	TextContent string
	TextHash    string
	TextLength  int
	ElementKind string // "function", "class", "struct", "file", etc.
	Language    string // "go", "python", "typescript", etc.
	FilePath    string
	ChunkStart  int
	ChunkEnd    int
	PackageName string
	Signature   string
	Tags        []string
	CallSite    string // "ingest", "retrieve", "consult", "jiminy.evaluate", etc.
	QueryText   string // for query events: the search query
	ModelName   string
	Provider    string
	Dimensions  int
	LatencyMs   int
	Cached      bool
	NodeID      string
}

// EmbeddingEventRecorder records embedding events for contrastive training data collection.
type EmbeddingEventRecorder interface {
	RecordEmbed(ctx context.Context, event EmbeddingEvent)
}

// embeddingMetaKey is the context key for embedding metadata.
type embeddingMetaKey struct{}

// EmbeddingMeta carries parser metadata through context to the embedder.
type EmbeddingMeta struct {
	ElementKind string
	Language    string
	FilePath    string
	CallSite    string
	PackageName string
	Signature   string
	ChunkStart  int
	ChunkEnd    int
	Tags        []string
	SpaceID     string
	QueryText   string
	NodeID      string
}

// WithEmbeddingMeta attaches embedding metadata to a context.
func WithEmbeddingMeta(ctx context.Context, meta EmbeddingMeta) context.Context {
	return context.WithValue(ctx, embeddingMetaKey{}, &meta)
}

// GetEmbeddingMeta retrieves embedding metadata from a context, or nil if not set.
func GetEmbeddingMeta(ctx context.Context) *EmbeddingMeta {
	m, _ := ctx.Value(embeddingMetaKey{}).(*EmbeddingMeta)
	return m
}

// TextHash computes SHA-256 of text content for deduplication.
func TextHash(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}
