// Package retrieval — intent_translator.go implements the Intent Translation Engine (Phase 102).
// This is the cognitive core of Gap 2: it rewrites conversational questions into keyword-dense
// search strings that maximize vector similarity against declarative knowledge graph text.
package retrieval

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
)

// IntentTranslator rewrites a conversational query into keyword-dense text
// optimized for vector similarity search against knowledge graph nodes.
type IntentTranslator interface {
	Translate(ctx context.Context, query string) (string, error)
}

// IntentConfig holds configuration for the LLM intent translator.
type IntentConfig struct {
	Enabled   bool
	Provider  string // "openai" or "ollama"
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// LLMIntentTranslator implements IntentTranslator using OpenAI or Ollama.
// Includes an LRU cache since temperature=0.0 produces deterministic results.
type LLMIntentTranslator struct {
	cfg        IntentConfig
	cbRegistry *circuitbreaker.Registry
	llm        *llmclient.Client

	// LRU cache for translated queries (deterministic at temp=0.0)
	cacheMu   sync.Mutex
	cacheMap  map[string]*list.Element
	cacheList *list.List
	cacheCap  int
}

type intentCacheEntry struct {
	query      string
	translated string
}

// NewLLMIntentTranslator creates a new LLM-based intent translator.
func NewLLMIntentTranslator(cfg IntentConfig, cbRegistry *circuitbreaker.Registry) *LLMIntentTranslator {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &LLMIntentTranslator{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("retrieval.intent_translate", ""),
		cacheMap:  make(map[string]*list.Element, 256),
		cacheList: list.New(),
		cacheCap:  256,
	}
}

// intentSystemPrompt is the system prompt that constrains the LLM to produce
// keyword-dense search strings. Temperature 0.0 ensures deterministic rewrites.
const intentSystemPrompt = `You are a query rewriter for a knowledge graph search engine.

The knowledge graph contains:
- Architecture decisions and rationale
- Code patterns and conventions
- File paths and function names
- Constraints and rules (must/must_not)
- Historical decisions and their contexts
- Configuration patterns and deployment workflows

Your task: Rewrite the user's conversational question into a dense, keyword-rich search string that will maximize vector similarity against the declarative text in the knowledge graph.

Rules:
- Output ONLY the rewritten query — no explanation, no preamble, no quotes
- Include domain-specific terms, file names, function names, and technical jargon
- Expand abbreviations and add synonyms
- Remove conversational filler (why, how, what, please, etc.)
- Keep the output under 100 words
- If the query is already keyword-dense, return it unchanged`

// Translate rewrites a conversational query into keyword-dense search text.
// Fail-open: on any error, returns the original query unchanged.
func (t *LLMIntentTranslator) Translate(ctx context.Context, query string) (string, error) {
	if !t.cfg.Enabled {
		return query, nil
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return query, nil
	}

	// Check LRU cache (deterministic at temperature=0.0)
	t.cacheMu.Lock()
	if elem, ok := t.cacheMap[trimmed]; ok {
		t.cacheList.MoveToFront(elem)
		cached := elem.Value.(*intentCacheEntry).translated
		t.cacheMu.Unlock()
		return cached, nil
	}
	t.cacheMu.Unlock()

	timeoutMs := t.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	msgs := []llmclient.Message{
		{Role: "system", Content: intentSystemPrompt},
		{Role: "user", Content: query},
	}

	maxTokens := t.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 150
	}
	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}

	cbName := "openai-intent"
	if t.cfg.Provider == "ollama" {
		cbName = "ollama-intent"
	}

	var translated string
	var err error

	if t.cbRegistry != nil {
		cb := t.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			translated, innerErr = t.llm.Complete(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return query, fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		translated, err = t.llm.Complete(timeoutCtx, msgs, opts)
	}

	if err != nil {
		// Fail-open: return original query on error
		return query, err
	}

	translated = strings.TrimSpace(translated)
	if translated == "" {
		return query, nil
	}

	// Store in LRU cache
	t.cacheMu.Lock()
	if _, ok := t.cacheMap[trimmed]; !ok {
		// Evict LRU if at capacity
		for len(t.cacheMap) >= t.cacheCap {
			back := t.cacheList.Back()
			if back == nil {
				break
			}
			evicted := t.cacheList.Remove(back).(*intentCacheEntry)
			delete(t.cacheMap, evicted.query)
		}
		elem := t.cacheList.PushFront(&intentCacheEntry{query: trimmed, translated: translated})
		t.cacheMap[trimmed] = elem
	}
	t.cacheMu.Unlock()

	return translated, nil
}

