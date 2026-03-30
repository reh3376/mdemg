// Package consulting — llm_classifier.go implements LLM-powered constraint
// classification (Phase AR-3). When enabled, it replaces the keyword-based
// constraint detection in findApplicableConstraints with LLM classification.
package consulting

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
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
	llm        *llmclient.Client

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
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &ConstraintClassifier{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("consulting.classify", ""),
		cacheMap:  make(map[string]*list.Element, 512),
		cacheList: list.New(),
		cacheCap:  512,
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

	msgs := []llmclient.Message{
		{Role: "system", Content: constraintClassifySystemPrompt},
		{Role: "user", Content: text},
	}

	maxTokens := cc.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 500
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}
	if cc.cfg.Provider == "ollama" {
		opts.Format = ollamaConstraintSchema
		opts.Options = map[string]any{"temperature": 0.1}
	}

	cbName := "openai-constraint-classify"
	if cc.cfg.Provider == "ollama" {
		cbName = "ollama-constraint-classify"
	}

	var raw string
	var err error

	if cc.cbRegistry != nil {
		cb := cc.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			raw, innerErr = cc.llm.Complete(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		raw, err = cc.llm.Complete(timeoutCtx, msgs, opts)
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

// ollamaConstraintSchema is the JSON schema for grammar-constrained output (Ollama v0.5+).
var ollamaConstraintSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"type": {"type": "string", "enum": ["must","must_not","should","should_not","none"]},
		"summary": {"type": "string"}
	},
	"required": ["type", "summary"]
}`)

// --- Response parsing ---

var validConstraintTypes = map[string]bool{
	"must": true, "must_not": true, "should": true, "should_not": true, "none": true,
}

func (cc *ConstraintClassifier) parseResponse(raw string) (*ConstraintClassification, error) {
	raw = llmclient.StripCodeFence(raw)
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

