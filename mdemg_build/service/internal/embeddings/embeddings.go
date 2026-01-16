// Package embeddings provides text embedding generation for MDEMG.
// Supports OpenAI and Ollama backends.
package embeddings

import (
	"context"
	"errors"
)

// ErrNoProvider is returned when no embedding provider is configured.
var ErrNoProvider = errors.New("no embedding provider configured")

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates an embedding for the given text.
	// Returns a slice of float32 values (typically 1536 dimensions for OpenAI ada-002).
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
	OpenAIModel    string // default: text-embedding-ada-002
	OpenAIEndpoint string // default: https://api.openai.com/v1

	// Ollama settings
	OllamaEndpoint string // default: http://localhost:11434
	OllamaModel    string // default: nomic-embed-text
}

// CachedEmbedder wraps an Embedder with an LRU cache.
// It implements the Embedder interface and transparently caches results.
type CachedEmbedder struct {
	embedder Embedder
	cache    *EmbeddingCache
}

// NewCachedEmbedder creates a new CachedEmbedder wrapping the given embedder.
// cacheSize determines the maximum number of cached embeddings.
func NewCachedEmbedder(embedder Embedder, cacheSize int) *CachedEmbedder {
	return &CachedEmbedder{
		embedder: embedder,
		cache:    NewEmbeddingCache(cacheSize),
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
		return cached, nil
	}

	// Cache miss - call underlying embedder
	embedding, err := c.embedder.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.Put(cacheKey, embedding)

	return embedding, nil
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
		return results, nil
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
func New(cfg Config) (Embedder, error) {
	switch cfg.Provider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, errors.New("OPENAI_API_KEY is required for openai provider")
		}
		return NewOpenAI(cfg)
	case "ollama":
		return NewOllama(cfg)
	case "", "none":
		return nil, ErrNoProvider
	default:
		return nil, errors.New("unknown embedding provider: " + cfg.Provider)
	}
}
