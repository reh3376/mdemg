// Package consulting — llm_classifier.go implements LLM-powered constraint
// classification (Phase AR-3). When enabled, it replaces the keyword-based
// constraint detection in findApplicableConstraints with LLM classification.
package consulting

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

// ConstraintClassifierConfig holds configuration for the LLM constraint classifier.
type ConstraintClassifierConfig struct {
	Enabled   bool
	Provider  string // "openai" or "ollama"
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// ConstraintClassification is the LLM's assessment of a text's constraint type.
type ConstraintClassification struct {
	Type    string `json:"type"`    // "must", "must_not", "should", "should_not", "none"
	Summary string `json:"summary"` // brief summary of the constraint
}

// ConstraintClassifier uses an LLM to classify whether text expresses a constraint.
// Follows the EmergenceNamer pattern (OpenAI/Ollama, circuit breaker, JSON output).
// Includes an LRU cache keyed by node_id since constraints don't change frequently.
type ConstraintClassifier struct {
	cfg        ConstraintClassifierConfig
	cbRegistry *circuitbreaker.Registry

	// LRU cache
	cacheMu   sync.Mutex
	cacheMap  map[string]*list.Element
	cacheList *list.List
	cacheCap  int
}

type constraintCacheEntry struct {
	nodeID string
	result ConstraintClassification
}

// NewConstraintClassifier creates a new LLM-powered constraint classifier.
func NewConstraintClassifier(cfg ConstraintClassifierConfig, cbRegistry *circuitbreaker.Registry) *ConstraintClassifier {
	return &ConstraintClassifier{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		cacheMap:   make(map[string]*list.Element, 512),
		cacheList:  list.New(),
		cacheCap:   512,
	}
}

const constraintClassifySystemPrompt = `You are a constraint classifier for a knowledge graph.

Given a text snippet from a knowledge node, determine if it expresses a requirement, prohibition, or recommendation.

Classify as one of:
- "must" — a hard requirement (e.g., "must use", "required", "always")
- "must_not" — a hard prohibition (e.g., "must not", "never", "forbidden")
- "should" — a soft recommendation (e.g., "should", "recommended", "prefer")
- "should_not" — a soft discouragement (e.g., "should not", "avoid", "discouraged")
- "none" — no constraint expressed

Provide a brief summary of the constraint (or empty string if "none").

Output ONLY valid JSON: {"type": "...", "summary": "..."}`

// Classify determines the constraint type of a text snippet.
// Results are cached by nodeID.
func (cc *ConstraintClassifier) Classify(ctx context.Context, nodeID, text string) (*ConstraintClassification, error) {
	if !cc.cfg.Enabled {
		return nil, nil
	}

	// Check cache
	if cached := cc.cacheGet(nodeID); cached != nil {
		return cached, nil
	}

	timeoutMs := cc.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var raw string
	var err error

	switch cc.cfg.Provider {
	case "ollama":
		raw, err = cc.callOllama(timeoutCtx, text)
	default:
		raw, err = cc.callOpenAI(timeoutCtx, text)
	}

	if err != nil {
		return nil, fmt.Errorf("constraint classifier: %w", err)
	}

	result, err := cc.parseResponse(raw)
	if err != nil {
		return nil, err
	}

	cc.cachePut(nodeID, *result)
	return result, nil
}

// --- Cache ---

func (cc *ConstraintClassifier) cacheGet(nodeID string) *ConstraintClassification {
	cc.cacheMu.Lock()
	defer cc.cacheMu.Unlock()

	if elem, ok := cc.cacheMap[nodeID]; ok {
		cc.cacheList.MoveToFront(elem)
		entry := elem.Value.(*constraintCacheEntry)
		return &entry.result
	}
	return nil
}

func (cc *ConstraintClassifier) cachePut(nodeID string, result ConstraintClassification) {
	cc.cacheMu.Lock()
	defer cc.cacheMu.Unlock()

	if elem, ok := cc.cacheMap[nodeID]; ok {
		cc.cacheList.MoveToFront(elem)
		elem.Value.(*constraintCacheEntry).result = result
		return
	}

	// Evict if at capacity
	for cc.cacheList.Len() >= cc.cacheCap {
		oldest := cc.cacheList.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*constraintCacheEntry)
		delete(cc.cacheMap, entry.nodeID)
		cc.cacheList.Remove(oldest)
	}

	entry := &constraintCacheEntry{nodeID: nodeID, result: result}
	elem := cc.cacheList.PushFront(entry)
	cc.cacheMap[nodeID] = elem
}

// --- OpenAI ---

func (cc *ConstraintClassifier) callOpenAI(ctx context.Context, text string) (string, error) {
	if cc.cbRegistry != nil {
		cb := cc.cbRegistry.Get("openai-constraint-classify")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = cc.doCallOpenAI(ctx, text)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai constraint-classify circuit breaker open")
		}
		return result, err
	}
	return cc.doCallOpenAI(ctx, text)
}

type constraintOpenAIRequest struct {
	Model     string                    `json:"model"`
	Messages  []constraintOpenAIMessage `json:"messages"`
	MaxTokens int                       `json:"max_completion_tokens"`
}

type constraintOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type constraintOpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (cc *ConstraintClassifier) doCallOpenAI(ctx context.Context, text string) (string, error) {
	maxTokens := cc.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 500
	}

	reqBody := constraintOpenAIRequest{
		Model: cc.cfg.Model,
		Messages: []constraintOpenAIMessage{
			{Role: "system", Content: constraintClassifySystemPrompt},
			{Role: "user", Content: text},
		},
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := cc.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cc.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp constraintOpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// --- Ollama ---

type constraintOllamaRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Stream  bool            `json:"stream"`
	Format  json.RawMessage `json:"format,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

type constraintOllamaResponse struct {
	Response string `json:"response"`
}

var ollamaConstraintSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"type": {"type": "string", "enum": ["must","must_not","should","should_not","none"]},
		"summary": {"type": "string"}
	},
	"required": ["type", "summary"]
}`)

func (cc *ConstraintClassifier) callOllama(ctx context.Context, text string) (string, error) {
	if cc.cbRegistry != nil {
		cb := cc.cbRegistry.Get("ollama-constraint-classify")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = cc.doCallOllama(ctx, text)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama constraint-classify circuit breaker open")
		}
		return result, err
	}
	return cc.doCallOllama(ctx, text)
}

func (cc *ConstraintClassifier) doCallOllama(ctx context.Context, text string) (string, error) {
	prompt := constraintClassifySystemPrompt + "\n\n" + text

	reqBody := constraintOllamaRequest{
		Model:   cc.cfg.Model,
		Prompt:  prompt,
		Stream:  false,
		Format:  ollamaConstraintSchema,
		Options: map[string]any{"temperature": 0.1},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := cc.cfg.OllamaURL + "/api/generate"
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

	var ollamaResp constraintOllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return strings.TrimSpace(ollamaResp.Response), nil
}

// --- Response parsing ---

var validConstraintTypes = map[string]bool{
	"must": true, "must_not": true, "should": true, "should_not": true, "none": true,
}

func (cc *ConstraintClassifier) parseResponse(raw string) (*ConstraintClassification, error) {
	raw = stripConstraintCodeFence(raw)
	raw = strings.TrimSpace(raw)

	var result ConstraintClassification
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from constraint classifier: %w (raw: %.200s)", err, raw)
	}
	if !validConstraintTypes[result.Type] {
		return nil, fmt.Errorf("invalid constraint type %q", result.Type)
	}
	return &result, nil
}

func stripConstraintCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSuffix(after, "```")
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSuffix(after, "```")
	}
	return strings.TrimSpace(s)
}
