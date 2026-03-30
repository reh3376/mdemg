// Package summarize provides LLM-based semantic summary generation for code elements.
// Summaries describe WHAT code does (purpose, behavior) rather than what it contains structurally.
package summarize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"mdemg/internal/llmclient"
)

// Config holds configuration for the summarize service.
type Config struct {
	Enabled      bool   // Feature toggle (default: false)
	Provider     string // "openai" or "ollama" (default: openai)
	Model        string // Model to use (default: gpt-4o-mini)
	MaxTokens    int    // Max tokens in response (default: 150)
	BatchSize    int    // Files per API call (default: 10)
	TimeoutMs    int    // Request timeout in ms (default: 30000)
	CacheEnabled bool   // Cache summaries to avoid regenerating (default: true)
	CacheSize    int    // Max cached summaries (default: 5000)
	Debug        bool   // Enable debug logging

	// OpenAI settings
	OpenAIAPIKey   string
	OpenAIEndpoint string // default: https://api.openai.com/v1

	// Ollama settings (for local LLM)
	OllamaEndpoint string // default: http://localhost:11434
}

// CodeElement represents a code element to summarize.
// This mirrors the structure used in ingest-codebase.
type CodeElement struct {
	Name     string   // Element name
	Kind     string   // package, function, struct, module, etc.
	Path     string   // Full path including anchors
	Content  string   // Full content of the element
	Package  string   // Package/module name
	FilePath string   // File path
	Tags     []string // Associated tags
	Concerns []string // Cross-cutting concerns
}

// Service provides LLM-based semantic summary generation.
type Service struct {
	config     Config
	llm        *llmclient.Client
	cache      *summaryCache
	structFn   func(CodeElement) string // Fallback structural summary function
	mu         sync.Mutex
	totalCalls int64
	totalHits  int64
}

// summaryCache provides thread-safe LRU caching of summaries.
type summaryCache struct {
	mu       sync.RWMutex
	items    map[string]cacheEntry
	order    []string // LRU order
	capacity int
}

type cacheEntry struct {
	summary   string
	timestamp time.Time
}

func newSummaryCache(capacity int) *summaryCache {
	return &summaryCache{
		items:    make(map[string]cacheEntry),
		order:    make([]string, 0, capacity),
		capacity: capacity,
	}
}

func (c *summaryCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if entry, ok := c.items[key]; ok {
		return entry.summary, true
	}
	return "", false
}

func (c *summaryCache) Put(key, summary string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if _, exists := c.items[key]; exists {
		c.items[key] = cacheEntry{summary: summary, timestamp: time.Now()}
		return
	}

	// Evict oldest if at capacity
	if len(c.order) >= c.capacity {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}

	c.items[key] = cacheEntry{summary: summary, timestamp: time.Now()}
	c.order = append(c.order, key)
}

func (c *summaryCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// New creates a new summarize service.
// If structuralFallback is provided, it will be used when LLM fails.
func New(cfg Config, structuralFallback func(CodeElement) string) (*Service, error) {
	if !cfg.Enabled {
		return nil, errors.New("summarize service is disabled")
	}

	// Set defaults
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 150
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.TimeoutMs <= 0 {
		cfg.TimeoutMs = 30000
	}
	if cfg.CacheSize <= 0 {
		cfg.CacheSize = 5000
	}
	if cfg.OpenAIEndpoint == "" {
		cfg.OpenAIEndpoint = "https://api.openai.com/v1"
	}
	if cfg.OllamaEndpoint == "" {
		cfg.OllamaEndpoint = "http://localhost:11434"
	}

	// Validate provider config
	switch cfg.Provider {
	case "openai":
		if cfg.OpenAIAPIKey == "" {
			return nil, errors.New("OPENAI_API_KEY is required for openai provider")
		}
	case "ollama":
		// No API key needed for local Ollama
	default:
		return nil, fmt.Errorf("unknown summarize provider: %s", cfg.Provider)
	}

	var cache *summaryCache
	if cfg.CacheEnabled {
		cache = newSummaryCache(cfg.CacheSize)
	}

	baseURL := cfg.OpenAIEndpoint
	apiKey := cfg.OpenAIAPIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaEndpoint
	}

	return &Service{
		config: cfg,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("summarize.generate", ""),
		cache:    cache,
		structFn: structuralFallback,
	}, nil
}

// cacheKey generates a unique cache key from element content.
// Uses SHA256 hash of content to handle large code blocks efficiently.
func (s *Service) cacheKey(elem CodeElement) string {
	// Create a deterministic key from element properties
	data := fmt.Sprintf("%s:%s:%s:%s", elem.Kind, elem.Name, elem.Path, elem.Content)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes (32 hex chars)
}

// Summarize generates a semantic summary for a single code element.
// Returns structural fallback if LLM fails.
func (s *Service) Summarize(ctx context.Context, elem CodeElement) string {
	summaries := s.SummarizeBatch(ctx, []CodeElement{elem})
	if len(summaries) > 0 {
		return summaries[0]
	}
	return s.fallback(elem)
}

// SummarizeBatch generates semantic summaries for multiple code elements.
// Uses batching to minimize API calls. Returns structural fallback for failures.
func (s *Service) SummarizeBatch(ctx context.Context, elements []CodeElement) []string {
	if len(elements) == 0 {
		return nil
	}

	s.mu.Lock()
	s.totalCalls++
	s.mu.Unlock()

	results := make([]string, len(elements))
	uncached := make([]int, 0) // Indices of elements needing LLM call

	// Check cache first
	if s.cache != nil {
		for i, elem := range elements {
			key := s.cacheKey(elem)
			if summary, found := s.cache.Get(key); found {
				results[i] = summary
				s.mu.Lock()
				s.totalHits++
				s.mu.Unlock()
				if s.config.Debug {
					slog.Debug("summarize: cache hit", "name", elem.Name, "kind", elem.Kind)
				}
			} else {
				uncached = append(uncached, i)
			}
		}

		if len(uncached) == 0 {
			return results // All cached
		}
	} else {
		// No cache - all need LLM
		for i := range elements {
			uncached = append(uncached, i)
		}
	}

	if s.config.Debug {
		slog.Debug("summarize: processing uncached elements", "uncached", len(uncached), "total", len(elements))
	}

	// Process uncached elements in batches
	for i := 0; i < len(uncached); i += s.config.BatchSize {
		end := i + s.config.BatchSize
		if end > len(uncached) {
			end = len(uncached)
		}

		batchIndices := uncached[i:end]
		batchElements := make([]CodeElement, len(batchIndices))
		for j, idx := range batchIndices {
			batchElements[j] = elements[idx]
		}

		// Call LLM for this batch
		summaries, err := s.callLLM(ctx, batchElements)
		if err != nil {
			if s.config.Debug {
				slog.Warn("summarize: LLM call failed, using fallback", "error", err)
			}
			// Use fallback for failed batch
			for _, idx := range batchIndices {
				results[idx] = s.fallback(elements[idx])
			}
			continue
		}

		// Store results and cache them
		for j, idx := range batchIndices {
			if j < len(summaries) && summaries[j] != "" {
				results[idx] = summaries[j]
				if s.cache != nil {
					s.cache.Put(s.cacheKey(elements[idx]), summaries[j])
				}
			} else {
				results[idx] = s.fallback(elements[idx])
			}
		}
	}

	return results
}

// callLLM makes the actual LLM API call for a batch of elements.
func (s *Service) callLLM(ctx context.Context, elements []CodeElement) ([]string, error) {
	switch s.config.Provider {
	case "openai":
		return s.callOpenAI(ctx, elements)
	case "ollama":
		return s.callOllama(ctx, elements)
	default:
		return nil, fmt.Errorf("unknown provider: %s", s.config.Provider)
	}
}

// buildPrompt creates the prompt for summarizing code elements.
func (s *Service) buildPrompt(elements []CodeElement) string {
	var sb strings.Builder

	sb.WriteString(`You are a code retrieval expert. Generate semantic summaries optimized for code search and retrieval.

For each code element, write a summary (2-3 sentences, max 300 characters) that includes:
1. PRIMARY PURPOSE: What specific problem or functionality does this implement?
2. KEY OPERATIONS: Name the 2-3 most important methods/functions and what they do
3. DOMAIN TERMS: Include specific technical terms that someone might search for (e.g., "circuit breaker", "retry logic", "batch processing", "validation", "rate limiting")

CRITICAL: Include specific method names and domain vocabulary that would help find this code via search.
DO NOT use generic terms like "handles data" or "manages state". Be specific about WHAT data and HOW.

Example good summary: "Implements circuit breaker pattern for inventory processing. Key methods: resetCircuitBreaker (resets after success), checkThreshold (validates batch limits). Handles retry logic for failed uploads."

Example bad summary: "Module for processing data. Contains methods for handling operations."

Respond with a JSON array of summaries in the same order as the inputs. Each summary should be a string.

Code elements to summarize:
`)

	for i, elem := range elements {
		sb.WriteString(fmt.Sprintf("\n--- Element %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("Type: %s\n", elem.Kind))
		sb.WriteString(fmt.Sprintf("Name: %s\n", elem.Name))
		if elem.Package != "" {
			sb.WriteString(fmt.Sprintf("Package: %s\n", elem.Package))
		}
		sb.WriteString(fmt.Sprintf("Path: %s\n", elem.Path))
		if len(elem.Concerns) > 0 {
			sb.WriteString(fmt.Sprintf("Concerns: %s\n", strings.Join(elem.Concerns, ", ")))
		}

		// Include truncated content (first 1500 chars to stay within token limits)
		content := elem.Content
		if len(content) > 1500 {
			content = content[:1500] + "\n... [truncated]"
		}
		sb.WriteString(fmt.Sprintf("Content:\n%s\n", content))
	}

	sb.WriteString("\nRespond ONLY with a JSON array of summary strings:")

	return sb.String()
}

func (s *Service) callOpenAI(ctx context.Context, elements []CodeElement) ([]string, error) {
	prompt := s.buildPrompt(elements)

	maxTokens := s.config.MaxTokens * len(elements) // Scale with batch size
	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	msgs := []llmclient.Message{
		{Role: "system", Content: "You are a helpful code analysis assistant. Respond only with valid JSON."},
		{Role: "user", Content: prompt},
	}

	content, err := s.llm.Complete(ctx, msgs, llmclient.CompleteOpts{MaxTokens: maxTokens})
	if err != nil {
		return nil, err
	}

	// Strip markdown code blocks
	content = llmclient.StripCodeFence(content)

	var summaries []string
	if err := json.Unmarshal([]byte(content), &summaries); err != nil {
		if s.config.Debug {
			slog.Warn("summarize: failed to parse JSON response", "content", content)
		}
		return nil, fmt.Errorf("parse summaries: %w", err)
	}

	return summaries, nil
}

func (s *Service) callOllama(ctx context.Context, elements []CodeElement) ([]string, error) {
	prompt := s.buildPrompt(elements)

	numPredict := s.config.MaxTokens * len(elements)

	msgs := []llmclient.Message{
		{Role: "user", Content: prompt},
	}

	content, err := s.llm.Complete(ctx, msgs, llmclient.CompleteOpts{
		Options: map[string]any{"num_predict": numPredict},
	})
	if err != nil {
		return nil, err
	}

	// Strip markdown code blocks
	content = llmclient.StripCodeFence(content)

	var summaries []string
	if err := json.Unmarshal([]byte(content), &summaries); err != nil {
		return nil, fmt.Errorf("parse summaries: %w", err)
	}

	return summaries, nil
}

// openAIChatResponse is kept for backward compatibility with tests
// that construct mock responses using this type.
type openAIChatResponse = llmclient.OpenAIChatResponse

// fallback returns a structural summary when LLM fails.
func (s *Service) fallback(elem CodeElement) string {
	if s.structFn != nil {
		return s.structFn(elem)
	}
	// Default minimal fallback
	return fmt.Sprintf("%s: %s", elem.Kind, elem.Name)
}

// Stats returns cache statistics.
func (s *Service) Stats() (totalCalls, cacheHits, cacheSize int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	size := int64(0)
	if s.cache != nil {
		size = int64(s.cache.Len())
	}
	return s.totalCalls, s.totalHits, size
}

// CombineSummary combines a structural summary with a semantic description.
// This creates the enriched summary format for storage.
func CombineSummary(structural, semantic string) string {
	if semantic == "" {
		return structural
	}
	if structural == "" {
		return semantic
	}

	// Check if structural already contains semantic info (avoid duplication)
	if strings.Contains(structural, semantic) {
		return structural
	}

	// Format: "Structural info. SEMANTIC: Description"
	// This allows easy parsing if needed later
	return structural + " SEMANTIC: " + semantic
}
