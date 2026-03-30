// Package embeddings provides text embedding generation for MDEMG.
// Supports OpenAI and Ollama backends.
package embeddings

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"
)

// ErrNoProvider is returned when no embedding provider is configured.
var ErrNoProvider = errors.New("no embedding provider configured")

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates an embedding for the given text.
	// Returns a slice of float32 values (typically 1536 dimensions for OpenAI/qwen3-embedding).
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embeddings for multiple texts.
	// More efficient than calling Embed multiple times.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of embeddings produced.
	Dimensions() int

	// Name returns the provider name for logging.
	Name() string
}

// Config holds embedding provider configuration.
type Config struct {
	// Provider: "openai" or "ollama"
	Provider string

	// OpenAI settings
	OpenAIAPIKey   string
	OpenAIModel    string // default: text-embedding-3-large
	OpenAIEndpoint string // default: https://api.openai.com/v1

	// Ollama settings
	OllamaEndpoint string // default: http://localhost:11434
	OllamaModel    string // default: qwen3-embedding:8b

	// Dimension control
	TargetDimensions int // if >0, request MRL truncation to this size (e.g., 3072 for qwen3-embedding:8b)

	// Cache settings
	CacheEnabled bool // Enable LRU caching of embeddings
	CacheSize    int  // Maximum number of cached embeddings (ignored if CacheEnabled=false)
}

// CachedEmbedder wraps an Embedder with an LRU cache.
// It implements the Embedder interface and transparently caches results.
type CachedEmbedder struct {
	embedder Embedder
	cache    *EmbeddingCache
	recorder EmbeddingEventRecorder
	debug    bool // Enable debug logging for cache hits/misses
}

// SetRecorder attaches an embedding event recorder for training data collection.
func (c *CachedEmbedder) SetRecorder(r EmbeddingEventRecorder) {
	c.recorder = r
}

// NewCachedEmbedder creates a new CachedEmbedder wrapping the given embedder.
// cacheSize determines the maximum number of cached embeddings.
// Debug logging is enabled if EMBEDDING_CACHE_DEBUG=true environment variable is set.
func NewCachedEmbedder(embedder Embedder, cacheSize int) *CachedEmbedder {
	debug := strings.ToLower(os.Getenv("EMBEDDING_CACHE_DEBUG")) == "true"
	return &CachedEmbedder{
		embedder: embedder,
		cache:    NewEmbeddingCache(cacheSize),
		debug:    debug,
	}
}

// Name returns the underlying provider name with cache indicator.
func (c *CachedEmbedder) Name() string {
	return c.embedder.Name() + "+cache"
}

// Dimensions returns the dimensionality of embeddings produced.
func (c *CachedEmbedder) Dimensions() int {
	return c.embedder.Dimensions()
}

// Embed generates an embedding for the given text, using cache when possible.
// Cache key format: providerName + ':' + text
func (c *CachedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Build cache key
	cacheKey := c.embedder.Name() + ":" + text

	// Check cache first
	if cached, found := c.cache.Get(cacheKey); found {
		if c.debug {
			truncated := text
			if len(text) > 50 {
				truncated = text[:50] + "..."
			}
			slog.Debug("embedding cache hit", "text", truncated, "cache_size", c.cache.Len())
		}
		c.recordEvent(ctx, text, true, 0)
		return cached, nil
	}

	// Cache miss - call underlying embedder
	if c.debug {
		truncated := text
		if len(text) > 50 {
			truncated = text[:50] + "..."
		}
		slog.Debug("embedding cache miss", "text", truncated, "cache_size", c.cache.Len())
	}

	start := time.Now()
	embedding, err := c.embedder.Embed(ctx, text)
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.Put(cacheKey, embedding)
	c.recordEvent(ctx, text, false, latencyMs)

	return embedding, nil
}

// recordEvent sends an embedding event to the recorder if one is set.
func (c *CachedEmbedder) recordEvent(ctx context.Context, text string, cached bool, latencyMs int) {
	if c.recorder == nil {
		return
	}
	meta := GetEmbeddingMeta(ctx)
	event := EmbeddingEvent{
		Time:        time.Now(),
		TextContent: text,
		TextHash:    TextHash(text),
		TextLength:  len(text),
		ModelName:   c.embedder.Name(),
		Dimensions:  c.embedder.Dimensions(),
		LatencyMs:   latencyMs,
		Cached:      cached,
	}
	if meta != nil {
		event.EventType = classifyEventType(meta.CallSite)
		event.SpaceID = meta.SpaceID
		event.ElementKind = meta.ElementKind
		event.Language = meta.Language
		event.FilePath = meta.FilePath
		event.ChunkStart = meta.ChunkStart
		event.ChunkEnd = meta.ChunkEnd
		event.PackageName = meta.PackageName
		event.Signature = meta.Signature
		event.Tags = meta.Tags
		event.CallSite = meta.CallSite
		event.QueryText = meta.QueryText
		event.NodeID = meta.NodeID
	}
	c.recorder.RecordEmbed(ctx, event)
}

func classifyEventType(callSite string) string {
	switch callSite {
	case "ingest":
		return "ingest"
	case "retrieve", "consult":
		return "query"
	default:
		return "internal"
	}
}

// EmbedBatch generates embeddings for multiple texts, using cache when possible.
// Each text is processed through the cache individually.
func (c *CachedEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	results := make([][]float32, len(texts))
	misses := make(map[int]string) // index -> text for cache misses

	// Check cache for each text
	for i, text := range texts {
		cacheKey := c.embedder.Name() + ":" + text
		if cached, found := c.cache.Get(cacheKey); found {
			results[i] = cached
		} else {
			misses[i] = text
		}
	}

	// If all were cached, return immediately
	if len(misses) == 0 {
		if c.debug {
			slog.Debug("embedding cache batch: all cached", "count", len(texts), "cache_size", c.cache.Len())
		}
		return results, nil
	}

	if c.debug {
		slog.Debug("embedding cache batch", "hits", len(texts)-len(misses), "misses", len(misses), "total", len(texts), "cache_size", c.cache.Len())
	}

	// Build list of texts to embed
	missTexts := make([]string, 0, len(misses))
	missIndices := make([]int, 0, len(misses))
	for idx, text := range misses {
		missTexts = append(missTexts, text)
		missIndices = append(missIndices, idx)
	}

	// Get embeddings for cache misses
	missEmbeddings, err := c.embedder.EmbedBatch(ctx, missTexts)
	if err != nil {
		return nil, err
	}

	// Store results and cache them
	for i, idx := range missIndices {
		if i < len(missEmbeddings) {
			embedding := missEmbeddings[i]
			results[idx] = embedding
			cacheKey := c.embedder.Name() + ":" + missTexts[i]
			c.cache.Put(cacheKey, embedding)
		}
	}

	return results, nil
}

// New creates an Embedder based on the configuration.
// Returns ErrNoProvider if no valid provider is configured.
// If CacheEnabled=true, wraps the embedder with LRU caching.
func New(cfg Config) (Embedder, error) {
	var embedder Embedder
	var err error

	switch cfg.Provider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, errors.New("OPENAI_API_KEY is required for openai provider")
		}
		embedder, err = NewOpenAI(cfg)
	case "ollama":
		embedder, err = NewOllama(cfg)
	case "stub":
		embedder = NewStub()
	case "", "none":
		return nil, ErrNoProvider
	default:
		return nil, errors.New("unknown embedding provider: " + cfg.Provider)
	}

	if err != nil {
		return nil, err
	}

	// Wrap with cache if enabled
	if cfg.CacheEnabled && cfg.CacheSize > 0 {
		return NewCachedEmbedder(embedder, cfg.CacheSize), nil
	}

	return embedder, nil
}
