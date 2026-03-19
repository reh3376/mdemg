// Package retrieval — query_classifier.go implements LLM-powered query type
// classification (Phase AR-3). When enabled, it replaces regex-based query type
// detection in ComputeRetrievalHints with few-shot LLM classification.
package retrieval

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"mdemg/internal/circuitbreaker"
)

// QueryClassifierConfig holds configuration for the LLM query classifier.
type QueryClassifierConfig struct {
	Enabled   bool
	Provider  string // "openai" or "ollama"
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// QueryClassification is the LLM's assessment of a query's type(s).
type QueryClassification struct {
	Types    []string `json:"types"`    // e.g., ["code", "architecture"]
	Temporal string   `json:"temporal"` // "recent", "historical", "none"
}

// QueryClassifier uses an LLM to classify queries into retrieval types.
// Follows the EmergenceNamer pattern (OpenAI/Ollama, circuit breaker, JSON output).
// Includes an LRU cache keyed by query text hash.
type QueryClassifier struct {
	cfg        QueryClassifierConfig
	cbRegistry *circuitbreaker.Registry

	cacheMu   sync.Mutex
	cacheMap  map[string]*list.Element
	cacheList *list.List
	cacheCap  int
}

type queryClassifyCacheEntry struct {
	hash   string
	result QueryClassification
}

// NewQueryClassifier creates a new LLM-powered query classifier.
func NewQueryClassifier(cfg QueryClassifierConfig, cbRegistry *circuitbreaker.Registry) *QueryClassifier {
	return &QueryClassifier{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		cacheMap:   make(map[string]*list.Element, 256),
		cacheList:  list.New(),
		cacheCap:   256,
	}
}

const queryClassifySystemPrompt = `You are a query classifier for a knowledge graph search engine.

Classify the user's query into one or more types:
- "code" — looking for specific code, functions, implementations
- "architecture" — asking about system design, patterns, structure
- "relationship" — asking how things connect, depend on, or relate to each other
- "data_flow" — tracing data through the system, pipelines, transformations
- "symbol_lookup" — looking up a specific named entity (function, class, variable)
- "generic" — general question that doesn't fit other categories

Also classify the temporal intent:
- "recent" — asking about recent changes or current state
- "historical" — asking about past decisions or evolution
- "none" — no temporal dimension

A query CAN have multiple types (e.g., "how does the auth middleware handle JWT validation?" is both "code" and "architecture").

Examples:
- "Where is getUserById defined?" → {"types": ["symbol_lookup"], "temporal": "none"}
- "How does the API handle authentication?" → {"types": ["architecture", "code"], "temporal": "none"}
- "What changed in the config module recently?" → {"types": ["code"], "temporal": "recent"}
- "How does data flow from ingestion to the hidden layer?" → {"types": ["data_flow", "architecture"], "temporal": "none"}
- "What's the relationship between retrieval and learning?" → {"types": ["relationship"], "temporal": "none"}

Output ONLY valid JSON: {"types": [...], "temporal": "..."}`

// Classify determines the query type(s) using LLM few-shot classification.
// Results are cached by query text hash.
func (qc *QueryClassifier) Classify(ctx context.Context, query string) (*QueryClassification, error) {
	if !qc.cfg.Enabled {
		return nil, nil
	}

	queryHash := hashQuery(query)

	// Check cache
	if cached := qc.cacheGet(queryHash); cached != nil {
		return cached, nil
	}

	timeoutMs := qc.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var raw string
	var err error

	switch qc.cfg.Provider {
	case "ollama":
		raw, err = qc.callOllama(timeoutCtx, query)
	default:
		raw, err = qc.callOpenAI(timeoutCtx, query)
	}

	if err != nil {
		return nil, fmt.Errorf("query classifier: %w", err)
	}

	result, err := qc.parseResponse(raw)
	if err != nil {
		return nil, err
	}

	qc.cachePut(queryHash, *result)
	return result, nil
}

func hashQuery(query string) string {
	h := sha256.Sum256([]byte(query))
	return hex.EncodeToString(h[:16])
}

// --- Cache ---

func (qc *QueryClassifier) cacheGet(hash string) *QueryClassification {
	qc.cacheMu.Lock()
	defer qc.cacheMu.Unlock()

	if elem, ok := qc.cacheMap[hash]; ok {
		qc.cacheList.MoveToFront(elem)
		entry := elem.Value.(*queryClassifyCacheEntry)
		return &entry.result
	}
	return nil
}

func (qc *QueryClassifier) cachePut(hash string, result QueryClassification) {
	qc.cacheMu.Lock()
	defer qc.cacheMu.Unlock()

	if elem, ok := qc.cacheMap[hash]; ok {
		qc.cacheList.MoveToFront(elem)
		elem.Value.(*queryClassifyCacheEntry).result = result
		return
	}

	for qc.cacheList.Len() >= qc.cacheCap {
		oldest := qc.cacheList.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*queryClassifyCacheEntry)
		delete(qc.cacheMap, entry.hash)
		qc.cacheList.Remove(oldest)
	}

	entry := &queryClassifyCacheEntry{hash: hash, result: result}
	elem := qc.cacheList.PushFront(entry)
	qc.cacheMap[hash] = elem
}

// --- OpenAI ---

func (qc *QueryClassifier) callOpenAI(ctx context.Context, query string) (string, error) {
	if qc.cbRegistry != nil {
		cb := qc.cbRegistry.Get("openai-query-classify")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = qc.doCallOpenAI(ctx, query)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai query-classify circuit breaker open")
		}
		return result, err
	}
	return qc.doCallOpenAI(ctx, query)
}

type queryOpenAIRequest struct {
	Model     string                 `json:"model"`
	Messages  []queryOpenAIMessage   `json:"messages"`
	MaxTokens int                    `json:"max_completion_tokens"`
}

type queryOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type queryOpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (qc *QueryClassifier) doCallOpenAI(ctx context.Context, query string) (string, error) {
	maxTokens := qc.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 500
	}

	reqBody := queryOpenAIRequest{
		Model: qc.cfg.Model,
		Messages: []queryOpenAIMessage{
			{Role: "system", Content: queryClassifySystemPrompt},
			{Role: "user", Content: query},
		},
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := qc.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+qc.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp queryOpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// --- Ollama ---

type queryOllamaRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Stream  bool            `json:"stream"`
	Format  json.RawMessage `json:"format,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

type queryOllamaResponse struct {
	Response string `json:"response"`
}

var ollamaQuerySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"types": {"type": "array", "items": {"type": "string", "enum": ["code","architecture","relationship","data_flow","symbol_lookup","generic"]}},
		"temporal": {"type": "string", "enum": ["recent","historical","none"]}
	},
	"required": ["types", "temporal"]
}`)

func (qc *QueryClassifier) callOllama(ctx context.Context, query string) (string, error) {
	if qc.cbRegistry != nil {
		cb := qc.cbRegistry.Get("ollama-query-classify")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = qc.doCallOllama(ctx, query)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama query-classify circuit breaker open")
		}
		return result, err
	}
	return qc.doCallOllama(ctx, query)
}

func (qc *QueryClassifier) doCallOllama(ctx context.Context, query string) (string, error) {
	prompt := queryClassifySystemPrompt + "\n\n" + query

	reqBody := queryOllamaRequest{
		Model:   qc.cfg.Model,
		Prompt:  prompt,
		Stream:  false,
		Format:  ollamaQuerySchema,
		Options: map[string]any{"temperature": 0.1},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := qc.cfg.OllamaURL + "/api/generate"
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

	var ollamaResp queryOllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return strings.TrimSpace(ollamaResp.Response), nil
}

// --- Response parsing ---

var validQueryTypes = map[string]bool{
	"code": true, "architecture": true, "relationship": true,
	"data_flow": true, "symbol_lookup": true, "generic": true,
}

var validTemporalIntents = map[string]bool{
	"recent": true, "historical": true, "none": true,
}

func (qc *QueryClassifier) parseResponse(raw string) (*QueryClassification, error) {
	raw = stripQueryCodeFence(raw)
	raw = strings.TrimSpace(raw)

	var result QueryClassification
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from query classifier: %w (raw: %.200s)", err, raw)
	}

	// Validate and filter types
	var validTypes []string
	for _, t := range result.Types {
		if validQueryTypes[t] {
			validTypes = append(validTypes, t)
		}
	}
	if len(validTypes) == 0 {
		validTypes = []string{"generic"}
	}
	result.Types = validTypes

	if !validTemporalIntents[result.Temporal] {
		result.Temporal = "none"
	}

	return &result, nil
}

func stripQueryCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSuffix(after, "```")
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSuffix(after, "```")
	}
	return strings.TrimSpace(s)
}
