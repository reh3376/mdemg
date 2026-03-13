// Package retrieval — intent_translator.go implements the Intent Translation Engine (Phase 102).
// This is the cognitive core of Gap 2: it rewrites conversational questions into keyword-dense
// search strings that maximize vector similarity against declarative knowledge graph text.
package retrieval

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mdemg/internal/circuitbreaker"
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
	return &LLMIntentTranslator{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		cacheMap:   make(map[string]*list.Element, 256),
		cacheList:  list.New(),
		cacheCap:   256,
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

	var translated string
	var err error

	switch t.cfg.Provider {
	case "ollama":
		translated, err = t.translateWithOllama(timeoutCtx, query)
	default: // "openai" or unset
		translated, err = t.translateWithOpenAI(timeoutCtx, query)
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

// --- OpenAI integration ---

// Local copies of request/response types (follows established duplication pattern in rerank.go, synthesis.go)
type intentOpenAIChatRequest struct {
	Model     string                `json:"model"`
	Messages  []intentOpenAIMessage `json:"messages"`
	MaxTokens int                   `json:"max_completion_tokens"`
}

type intentOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type intentOpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (t *LLMIntentTranslator) translateWithOpenAI(ctx context.Context, query string) (string, error) {
	if t.cbRegistry != nil {
		cb := t.cbRegistry.Get("openai-intent")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = t.doTranslateWithOpenAI(ctx, query)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai intent circuit breaker open")
		}
		return result, err
	}
	return t.doTranslateWithOpenAI(ctx, query)
}

func (t *LLMIntentTranslator) doTranslateWithOpenAI(ctx context.Context, query string) (string, error) {
	maxTokens := t.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 150
	}

	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	reqBody := intentOpenAIChatRequest{
		Model: t.cfg.Model,
		Messages: []intentOpenAIMessage{
			{Role: "system", Content: intentSystemPrompt},
			{Role: "user", Content: query},
		},
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := t.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp intentOpenAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// --- Ollama integration ---

type intentOllamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type intentOllamaGenerateResponse struct {
	Response string `json:"response"`
}

func (t *LLMIntentTranslator) translateWithOllama(ctx context.Context, query string) (string, error) {
	if t.cbRegistry != nil {
		cb := t.cbRegistry.Get("ollama-intent")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = t.doTranslateWithOllama(ctx, query)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama intent circuit breaker open")
		}
		return result, err
	}
	return t.doTranslateWithOllama(ctx, query)
}

func (t *LLMIntentTranslator) doTranslateWithOllama(ctx context.Context, query string) (string, error) {
	// Build prompt with system instructions inlined (Ollama generate API uses single prompt)
	prompt := intentSystemPrompt + "\n\nUser query:\n" + query

	reqBody := intentOllamaGenerateRequest{
		Model:  t.cfg.Model,
		Prompt: prompt,
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := t.cfg.OllamaURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp intentOllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}
