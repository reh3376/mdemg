package jiminy

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/embeddings"
	"mdemg/internal/encoding"
	"mdemg/internal/llmclient"
)

const classifySystemPrompt = `You are a guidance outcome classifier. You determine whether an AI coding agent followed,
ignored, partially complied with, or contradicted a piece of guidance.

Classification rules:
- "followed": The agent's action clearly aligns with the guidance intent.
- "partial_compliance": The agent addressed some aspects but missed others, or followed
  the spirit but not the letter of the guidance.
- "ignored": The agent's action shows no evidence of considering the guidance.
- "contradicted": The agent's action directly opposes the guidance intent.

Consider:
- Constraint type (must/must_not = strict compliance expected; should/should_not = flexible)
- Similarity between guidance and action (provided as context)
- Whether the action achieves the intent of the guidance, not just literal text matching

Action summary format:
- "Edited FILE: replaced 'OLD' with 'NEW'" means the agent REMOVED the OLD text and ADDED the NEW text.
  The OLD text is what was deleted — do not treat its content as the agent's intent.
  Focus on the NEW text and the overall effect of the change.
- Negation words appearing inside quoted code strings (variable names, log messages, assertions)
  are NOT indicators of contradiction. Only treat negation as contradiction when the agent's
  overall action semantically opposes the guidance.

Respond with ONLY valid JSON: {"outcome": "...", "confidence": 0.0-1.0, "reasoning": "..."}`

// classifySystemPromptCompact is a condensed system prompt for classification
// used when CompressPrompts is enabled.
const classifySystemPromptCompact = `Guidance outcome classifier. Classify: followed/partial_compliance/ignored/contradicted.
must/must_not=strict, should/should_not=flexible. Consider intent not literal text.
Action format: "replaced 'OLD' with 'NEW'" = OLD was REMOVED, NEW was ADDED. Focus on NEW text and overall effect.
Negation words in quoted code are NOT contradiction indicators.
JSON only: {"outcome": "...", "confidence": 0.0-1.0, "reasoning": "..."}`

// ollamaClassifySchema is the JSON schema for Ollama grammar-constrained classification.
var ollamaClassifySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"outcome": {"type": "string", "enum": ["followed", "partial_compliance", "ignored", "contradicted"]},
		"confidence": {"type": "number"},
		"reasoning": {"type": "string"}
	},
	"required": ["outcome", "confidence", "reasoning"]
}`)

// OutcomeClassifier performs semantic outcome classification using embeddings
// and optional LLM-based judgment for uncertain cases.
type OutcomeClassifier struct {
	embedder        embeddings.Embedder
	llm             *llmclient.Client // optional, for Tier 2 classification
	llmEnabled      bool
	llmProvider     string  // provider name for conditional behavior (e.g., "ollama" vs "openai")
	compressPrompts bool    // J17-PC: compress classification prompts
	highThreshold   float64 // above this similarity = followed (default: 0.7)
	lowThreshold    float64 // below this similarity = ignored (default: 0.3)
	maxTokens       int     // J14: max tokens for LLM classification

	// G8: circuit breaker for LLM calls
	cbRegistry *circuitbreaker.Registry

	// J14: LRU cache for classification results
	cacheMu   sync.Mutex
	cacheMap  map[string]*list.Element
	cacheList *list.List
	cacheCap  int
}

// OutcomeClassifierConfig configures the semantic outcome classifier.
type OutcomeClassifierConfig struct {
	LLMEnabled      bool
	LLMProvider     string
	LLMModel        string
	LLMAPIKey       string
	LLMBaseURL      string
	HighThreshold   float64
	LowThreshold    float64
	MaxTokens       int  // J14: max tokens (default: 100)
	CacheSize       int  // J14: LRU cache capacity (default: 256)
	CompressPrompts bool // J17-PC: compress classification prompts to reduce tokens
}

// classifyCacheEntry holds a cached classification result.
type classifyCacheEntry struct {
	key    string
	result ClassificationResult
}

// NewOutcomeClassifier creates a new semantic outcome classifier.
func NewOutcomeClassifier(embedder embeddings.Embedder, cfg OutcomeClassifierConfig) *OutcomeClassifier {
	oc := &OutcomeClassifier{
		embedder:        embedder,
		llmEnabled:      cfg.LLMEnabled,
		compressPrompts: cfg.CompressPrompts,
		highThreshold:   cfg.HighThreshold,
		lowThreshold:    cfg.LowThreshold,
		maxTokens:       cfg.MaxTokens,
		cacheMap:        make(map[string]*list.Element),
		cacheList:       list.New(),
	}

	if oc.highThreshold <= 0 {
		oc.highThreshold = 0.55
	}
	if oc.lowThreshold <= 0 {
		oc.lowThreshold = 0.20
	}
	if oc.maxTokens <= 0 {
		oc.maxTokens = 500
	}

	oc.cacheCap = cfg.CacheSize
	if oc.cacheCap <= 0 {
		oc.cacheCap = 256
	}

	if cfg.LLMEnabled && cfg.LLMProvider != "" {
		oc.llmProvider = cfg.LLMProvider
		oc.llm = llmclient.New(llmclient.Config{
			Provider:  cfg.LLMProvider,
			Model:     cfg.LLMModel,
			APIKey:    cfg.LLMAPIKey,
			BaseURL:   cfg.LLMBaseURL,
			TimeoutMs: 15000,
		}).WithContext("jiminy.evaluate", "")
	}

	return oc
}

// SetCircuitBreakerRegistry sets the circuit breaker registry for LLM calls.
func (oc *OutcomeClassifier) SetCircuitBreakerRegistry(reg *circuitbreaker.Registry) {
	oc.cbRegistry = reg
}

// Classify determines the outcome of a guidance item given an action summary.
// Tier 1: embedding cosine similarity. Tier 2: optional LLM classification for uncertain cases.
// Returns ClassificationResult with outcome, confidence, and optional reasoning (J14).
func (oc *OutcomeClassifier) Classify(ctx context.Context, item GuidanceItem, actionSummary string) ClassificationResult {
	if oc.embedder == nil {
		// Fall back to text overlap
		outcome, sim := classifyOutcome(item, strings.ToLower(actionSummary))
		return ClassificationResult{Outcome: outcome, Confidence: sim}
	}

	// Tier 1: Embedding-based comparison
	ctx = embeddings.WithEmbeddingMeta(ctx, embeddings.EmbeddingMeta{
		CallSite:    "jiminy.outcome",
		ElementKind: string(item.Type),
		QueryText:   actionSummary,
	})
	guidanceEmbed, err := oc.embedder.Embed(ctx, item.Content)
	if err != nil {
		slog.Error("jiminy classifier: guidance embedding failed", "error", err)
		outcome, sim := classifyOutcome(item, strings.ToLower(actionSummary))
		return ClassificationResult{Outcome: outcome, Confidence: sim}
	}

	actionEmbed, err := oc.embedder.Embed(ctx, actionSummary)
	if err != nil {
		slog.Error("jiminy classifier: action embedding failed", "error", err)
		outcome, sim := classifyOutcome(item, strings.ToLower(actionSummary))
		return ClassificationResult{Outcome: outcome, Confidence: sim}
	}

	similarity := cosineSimilarity(guidanceEmbed, actionEmbed)

	// Detect negation indicators in action summary (saved for LLM context, not short-circuited)
	actionLower := strings.ToLower(actionSummary)
	hasNegation, matchedPattern := detectNegation(actionLower)

	// Low similarity = not applicable (topics don't overlap — guidance wasn't relevant to this action)
	if similarity < oc.lowThreshold {
		return ClassificationResult{Outcome: OutcomeNotApplicable, Confidence: similarity}
	}

	// High similarity + no negation = followed
	if similarity >= oc.highThreshold && !hasNegation {
		return ClassificationResult{Outcome: OutcomeFollowed, Confidence: similarity}
	}

	// Uncertain range OR high-similarity-with-negation: try LLM Tier 2 if available.
	// Negation is passed as context so the LLM can judge whether it's a real contradiction
	// or a false positive (e.g., negation words appearing in quoted code content).
	if oc.llm != nil && oc.llmEnabled {
		cr := oc.llmClassify(ctx, item, actionSummary, similarity, hasNegation, matchedPattern)
		if cr.Outcome != OutcomeUnknown {
			return cr
		}
	}

	// Heuristic fallback: no LLM available or LLM returned unknown.
	if hasNegation {
		return ClassificationResult{Outcome: OutcomeContradicted, Confidence: similarity}
	}
	if similarity >= oc.highThreshold {
		return ClassificationResult{Outcome: OutcomeFollowed, Confidence: similarity}
	}
	return ClassificationResult{Outcome: OutcomePartialCompliance, Confidence: similarity}
}

// llmClassify uses an LLM to determine the outcome for uncertain cases (J14 upgraded).
func (oc *OutcomeClassifier) llmClassify(ctx context.Context, item GuidanceItem, actionSummary string, baseSimilarity float64, hasNegation bool, matchedPattern string) ClassificationResult {
	// Check cache first
	cacheKey := classifyCacheKey(item.Content, actionSummary)
	if cached := oc.classifyCacheGet(cacheKey); cached != nil {
		return *cached
	}

	prompt := buildClassifyPrompt(item, actionSummary, baseSimilarity, oc.compressPrompts, hasNegation, matchedPattern)

	sysPrompt := classifySystemPrompt
	if oc.compressPrompts {
		sysPrompt = classifySystemPromptCompact
	}

	msgs := []llmclient.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: prompt},
	}

	opts := llmclient.CompleteOpts{
		MaxTokens: oc.maxTokens,
	}

	// Ollama: set JSON schema for grammar-constrained output
	if oc.llmProvider == "ollama" {
		opts.Format = ollamaClassifySchema
	}

	var response string
	if oc.cbRegistry != nil {
		cbName := "jiminy-outcome-classifier"
		cb := oc.cbRegistry.Get(cbName)
		err := cb.Execute(ctx, func(cbCtx context.Context) error {
			var innerErr error
			response, _, innerErr = oc.llm.CompleteWithUsage(cbCtx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			slog.Warn("jiminy classifier: circuit breaker open, using heuristic fallback")
			return ClassificationResult{Outcome: OutcomeUnknown, Confidence: baseSimilarity}
		}
		if err != nil {
			slog.Error("jiminy classifier: LLM classification failed", "error", err)
			return ClassificationResult{Outcome: OutcomeUnknown, Confidence: baseSimilarity}
		}
	} else {
		var err error
		response, _, err = oc.llm.CompleteWithUsage(ctx, msgs, opts)
		if err != nil {
			slog.Error("jiminy classifier: LLM classification failed", "error", err)
			return ClassificationResult{Outcome: OutcomeUnknown, Confidence: baseSimilarity}
		}
	}

	slog.Debug("jiminy classifier: raw LLM response", "response_len", len(response), "response_preview", encoding.TruncateAtWord(response, 200))

	// Parse structured response
	cr := parseClassifyResponse(response, baseSimilarity)

	// Cache the result
	oc.classifyCachePut(cacheKey, cr)

	return cr
}

// buildClassifyPrompt constructs the enriched Tier 2 classification prompt (J14).
// When compress is true, removes redundant Task section and truncates content.
// hasNegation/matchedPattern provide negation detection context for the LLM.
func buildClassifyPrompt(item GuidanceItem, actionSummary string, similarity float64, compress bool, hasNegation bool, matchedPattern string) string {
	var sb strings.Builder

	content := item.Content
	action := actionSummary
	if compress {
		content = encoding.TruncateAtWord(content, 300)
		action = encoding.TruncateAtWord(action, 400)
	}

	sb.WriteString("## Guidance Item\n")
	fmt.Fprintf(&sb, "- Type: %s\n", item.Type)
	fmt.Fprintf(&sb, "- Priority: %s\n", item.Priority)
	fmt.Fprintf(&sb, "- Content: %s\n", content)
	if len(item.SourceNodes) > 0 {
		fmt.Fprintf(&sb, "- Source Node: %s\n", item.SourceNodes[0])
	}
	fmt.Fprintf(&sb, "- Vector Similarity: %.3f\n", similarity)
	if hasNegation {
		fmt.Fprintf(&sb, "- Negation Detected: true (matched: %q)\n", matchedPattern)
		sb.WriteString("- IMPORTANT: If the action uses \"replaced 'OLD' with 'NEW'\" format, the OLD text ")
		sb.WriteString("was REMOVED by the agent. Negation words in the OLD text are from deleted code ")
		sb.WriteString("and do NOT represent the agent's intent. Focus on the NEW text and the overall ")
		sb.WriteString("effect of the change to determine the outcome.\n")
	}
	sb.WriteString("\n")

	sb.WriteString("## Agent Action\n")
	sb.WriteString(action)
	sb.WriteString("\n\n")

	// The Task section duplicates the system prompt instructions — omit when compressing
	if !compress {
		sb.WriteString("## Task\n")
		sb.WriteString("Classify this outcome. Respond with JSON: {\"outcome\": \"...\", \"confidence\": 0.0-1.0, \"reasoning\": \"...\"}")
	}

	return sb.String()
}

// parseClassifyResponse parses the LLM classification JSON response (J14).
func parseClassifyResponse(raw string, fallbackConfidence float64) ClassificationResult {
	cleaned := llmclient.SanitizeResponse(raw)

	var result struct {
		Outcome    string  `json:"outcome"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		slog.Warn("jiminy classifier: failed to parse LLM response", "error", err)
		return ClassificationResult{Outcome: OutcomeUnknown, Confidence: fallbackConfidence}
	}

	// Validate outcome enum
	outcome := mapOutcomeString(result.Outcome)
	confidence := result.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = fallbackConfidence
	}

	return ClassificationResult{
		Outcome:    outcome,
		Confidence: confidence,
		Reasoning:  result.Reasoning,
	}
}

// mapOutcomeString maps a string to a valid GuidanceOutcome.
func mapOutcomeString(s string) GuidanceOutcome {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "followed":
		return OutcomeFollowed
	case "partial_compliance":
		return OutcomePartialCompliance
	case "ignored":
		return OutcomeIgnored
	case "contradicted":
		return OutcomeContradicted
	case "not_applicable":
		return OutcomeNotApplicable
	default:
		return OutcomeUnknown
	}
}

// --- LRU cache methods (J14) ---

func classifyCacheKey(guidanceContent, actionSummary string) string {
	h := sha256.Sum256([]byte(guidanceContent + ":" + actionSummary))
	return fmt.Sprintf("%x", h[:16])
}

func (oc *OutcomeClassifier) classifyCacheGet(key string) *ClassificationResult {
	oc.cacheMu.Lock()
	defer oc.cacheMu.Unlock()

	if elem, ok := oc.cacheMap[key]; ok {
		oc.cacheList.MoveToFront(elem)
		result := elem.Value.(*classifyCacheEntry).result
		return &result
	}
	return nil
}

func (oc *OutcomeClassifier) classifyCachePut(key string, result ClassificationResult) {
	oc.cacheMu.Lock()
	defer oc.cacheMu.Unlock()

	if elem, ok := oc.cacheMap[key]; ok {
		oc.cacheList.MoveToFront(elem)
		elem.Value.(*classifyCacheEntry).result = result
		return
	}

	entry := &classifyCacheEntry{key: key, result: result}
	elem := oc.cacheList.PushFront(entry)
	oc.cacheMap[key] = elem

	for oc.cacheList.Len() > oc.cacheCap {
		back := oc.cacheList.Back()
		if back != nil {
			oc.cacheList.Remove(back)
			delete(oc.cacheMap, back.Value.(*classifyCacheEntry).key)
		}
	}
}

// negationPatterns are substring indicators of potential contradiction.
var negationPatterns = []string{"instead of", "did not", "didn't", "ignored", "skipped", "contrary to"}

// detectNegation checks if any negation patterns appear in the lowercased action text.
// Returns true and the matched pattern, or false and empty string.
func detectNegation(actionLower string) (bool, string) {
	for _, neg := range negationPatterns {
		if strings.Contains(actionLower, neg) {
			return true, neg
		}
	}
	return false, ""
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0.0
	}
	return dot / denom
}
