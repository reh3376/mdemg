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
ignored, partially complied with, contradicted a piece of guidance — or whether the guidance
simply did not apply to the action taken.

Classification rules:
- "followed": The agent's action clearly aligns with the guidance intent.
- "partial_compliance": The agent addressed some aspects but missed others, or followed
  the spirit but not the letter of the guidance.
- "ignored": The guidance was relevant to this action — the agent could and should have
  applied it — but the action shows no evidence of considering it.
- "not_applicable": The guidance topic is unrelated to this specific action. The agent is
  delivered many guidance items per action; most cannot apply to any single action. If the
  action neither could have followed nor violated the guidance, it is "not_applicable",
  NOT "ignored". Use "ignored" only when applying the guidance was genuinely possible here.
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
const classifySystemPromptCompact = `Guidance outcome classifier. Classify: followed/partial_compliance/ignored/not_applicable/contradicted.
not_applicable = guidance topic unrelated to this action (use it, not "ignored", when the action could neither follow nor violate the guidance).
must/must_not=strict, should/should_not=flexible. Consider intent not literal text.
Action format: "replaced 'OLD' with 'NEW'" = OLD was REMOVED, NEW was ADDED. Focus on NEW text and overall effect.
Negation words in quoted code are NOT contradiction indicators.
JSON only: {"outcome": "...", "confidence": 0.0-1.0, "reasoning": "..."}`

// nonViolationCreditClause is appended to the classifier system prompt when
// OutcomeClassifierConfig.NonViolationCredit is true (JIMINY-ACTIONABILITY-
// COMPLIANCE-CREDIT-001). Routes must_not-type "the action didn't touch the
// mechanism" verdicts to `not_applicable` (already filtered from
// constraint_outcomes by the shipped service.go:1730,1762 writer gate) rather
// than `ignored` (which inflates the actionable denominator).
//
// Default OFF; operator flips JIMINY_NONVIOLATION_CREDIT_ENABLED=true in .env
// after running the 3-day A/B recipe (ab_recipe.md).
const nonViolationCreditClause = `

NON-VIOLATION CREDIT for must_not-type constraints:
- If a must_not-type constraint (e.g. "NEVER commit directly to main", "must_not use raw SQL", "never call X directly") applies to a mechanism the action DID NOT touch, classify the action as "not_applicable", NOT "ignored".
- Only use "ignored" when the action clearly touched the constraint's mechanism AND the agent had genuine opportunity to apply the constraint but didn't.
- Example: constraint "never commit to main" + action "read a config file" = not_applicable (the action didn't touch git-commit mechanism), not ignored.
- Example: constraint "never commit to main" + action "committed on dev branch and pushed" = followed (respected the constraint by using dev).
- Example: constraint "never commit to main" + action "committed directly to main" = contradicted.
This rule reduces false-ignored labels on constraints that don't apply to the specific action being taken.`

// resolveClassifySystemPrompt returns the effective system prompt for tier-2
// classification. When nonViolationCredit is true, appends the extension
// clause. Default-off path returns the historical byte-identical prompt.
func (oc *OutcomeClassifier) resolveClassifySystemPrompt() string {
	base := classifySystemPrompt
	if oc.compressPrompts {
		base = classifySystemPromptCompact
	}
	if oc.nonViolationCredit {
		return base + nonViolationCreditClause
	}
	return base
}

// ollamaClassifySchema is the JSON schema for Ollama grammar-constrained classification.
var ollamaClassifySchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"outcome": {"type": "string", "enum": ["followed", "partial_compliance", "ignored", "not_applicable", "contradicted"]},
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
	compressPrompts       bool    // J17-PC: compress classification prompts
	highThreshold         float64 // above this similarity = followed (default: 0.7)
	lowThreshold          float64 // below this similarity = sub-LOW band (ignored / not_applicable, see naThreshold) (default: 0.2)
	naThreshold           float64 // JIMINY-CORPUS-001 Epic 4 relevance gate: below this = not_applicable; [this, low) = ignored. ≤0 disables (whole sub-LOW tail is not_applicable)
	nonViolationCredit    bool    // JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001: when true, LLM classifier prompt gets an extended clause telling the LLM to classify must_not-type constraints as not_applicable (not ignored) when the action didn't touch the constraint's mechanism. Routes ~50% of current unrelated-context ignored verdicts to not_applicable, which the shipped writer gate (service.go:1730,1762) filters from constraint_outcomes. Predicted to lift constraint follow rate 10%→~20%. Default false; operator runs 3-day A/B before flipping.
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
	LLMEnabled             bool
	LLMProvider            string
	LLMModel               string
	LLMAPIKey              string
	LLMBaseURL             string
	HighThreshold          float64
	LowThreshold           float64
	NotApplicableThreshold float64 // relevance gate; ≤0 disables (sub-LOW tail = not_applicable)
	MaxTokens              int     // J14: max tokens (default: 100)
	CacheSize              int     // J14: LRU cache capacity (default: 256)
	CompressPrompts        bool    // J17-PC: compress classification prompts to reduce tokens
	NonViolationCredit     bool    // JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001: extend classifier prompt with must_not non-violation credit clause. Default false.
}

// classifyCacheEntry holds a cached classification result.
type classifyCacheEntry struct {
	key    string
	result ClassificationResult
}

// NewOutcomeClassifier creates a new semantic outcome classifier.
func NewOutcomeClassifier(embedder embeddings.Embedder, cfg OutcomeClassifierConfig) *OutcomeClassifier {
	oc := &OutcomeClassifier{
		embedder:           embedder,
		llmEnabled:         cfg.LLMEnabled,
		compressPrompts:    cfg.CompressPrompts,
		highThreshold:      cfg.HighThreshold,
		lowThreshold:       cfg.LowThreshold,
		maxTokens:          cfg.MaxTokens,
		nonViolationCredit: cfg.NonViolationCredit,
		cacheMap:           make(map[string]*list.Element),
		cacheList:          list.New(),
	}

	if oc.highThreshold <= 0 {
		oc.highThreshold = 0.55
	}
	if oc.lowThreshold <= 0 {
		oc.lowThreshold = 0.20
	}
	// JIMINY-CORPUS-001 Epic 4: relevance gate. Unlike high/low, ≤0 means
	// DISABLED, not "use default" — the production default flows in from
	// config (JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY, 0.10), and an
	// explicit 0 must keep the pre-gate behavior byte-identical.
	oc.naThreshold = cfg.NotApplicableThreshold
	if oc.naThreshold < 0 {
		oc.naThreshold = 0
	}
	if oc.naThreshold > oc.lowThreshold {
		slog.Warn("jiminy classifier: not_applicable threshold above low threshold — clamping to low",
			"not_applicable_threshold", cfg.NotApplicableThreshold, "low_threshold", oc.lowThreshold)
		oc.naThreshold = oc.lowThreshold
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
		}).WithContext("jiminy.evaluate_llm", "")
		// Phase 11.6.x — task_name now matches the prompt content. The OutcomeClassifier
		// emits classifySystemPrompt (hashes 1f02ee46... and historical f897ae32...),
		// which the ULTS spec assigns to jiminy.evaluate_llm. Pre-fix this site was
		// tagging rows as jiminy.evaluate; V0014 backfills the ~338 historical rows.
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
		return ClassificationResult{Outcome: outcome, Confidence: sim, Source: "heuristic"}
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
		return ClassificationResult{Outcome: outcome, Confidence: sim, Source: "heuristic"}
	}

	actionEmbed, err := oc.embedder.Embed(ctx, actionSummary)
	if err != nil {
		slog.Error("jiminy classifier: action embedding failed", "error", err)
		outcome, sim := classifyOutcome(item, strings.ToLower(actionSummary))
		return ClassificationResult{Outcome: outcome, Confidence: sim, Source: "heuristic"}
	}

	similarity := cosineSimilarity(guidanceEmbed, actionEmbed)

	// Detect negation indicators in action summary (saved for LLM context, not short-circuited)
	actionLower := strings.ToLower(actionSummary)
	hasNegation, matchedPattern := detectNegation(actionLower)

	// Sub-LOW band. JIMINY-CORPUS-001 Epic 4 relevance gate: only the clearly-
	// unrelated tail (< naThreshold) is not_applicable — the guidance did not
	// apply to this action. The [naThreshold, lowThreshold) band shares enough
	// topical signal that the guidance plausibly applied and was not followed —
	// a real ignore, not irrelevance. Gate disabled (naThreshold ≤ 0): the whole
	// sub-LOW tail is not_applicable (the JIMINY-OUTCOME-002 behavior).
	//
	// This is a tier-1 verdict only: the tier-2 LLM never runs below LOW, so
	// the gate can never override an LLM relevance verdict — LLM verdicts
	// (Source "llm", which may themselves say ignored/not_applicable) are
	// always returned as-is further down.
	if similarity < oc.lowThreshold {
		if oc.naThreshold > 0 && similarity >= oc.naThreshold {
			return ClassificationResult{Outcome: OutcomeIgnored, Confidence: similarity, Source: "tier1"}
		}
		return ClassificationResult{Outcome: OutcomeNotApplicable, Confidence: similarity, Source: "tier1"}
	}

	// High similarity + no negation = followed
	if similarity >= oc.highThreshold && !hasNegation {
		return ClassificationResult{Outcome: OutcomeFollowed, Confidence: similarity, Source: "tier1"}
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
		return ClassificationResult{Outcome: OutcomeContradicted, Confidence: similarity, Source: "heuristic"}
	}
	if similarity >= oc.highThreshold {
		return ClassificationResult{Outcome: OutcomeFollowed, Confidence: similarity, Source: "heuristic"}
	}
	return ClassificationResult{Outcome: OutcomePartialCompliance, Confidence: similarity, Source: "heuristic"}
}

// llmClassify uses an LLM to determine the outcome for uncertain cases (J14 upgraded).
func (oc *OutcomeClassifier) llmClassify(ctx context.Context, item GuidanceItem, actionSummary string, baseSimilarity float64, hasNegation bool, matchedPattern string) ClassificationResult {
	// Check cache first
	cacheKey := classifyCacheKey(item.Content, actionSummary)
	if cached := oc.classifyCacheGet(cacheKey); cached != nil {
		return *cached
	}

	prompt := buildClassifyPrompt(item, actionSummary, baseSimilarity, oc.compressPrompts, hasNegation, matchedPattern)

	sysPrompt := oc.resolveClassifySystemPrompt()

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
			response, _, _, innerErr = oc.llm.CompleteWithUsage(cbCtx, msgs, opts)
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
		response, _, _, err = oc.llm.CompleteWithUsage(ctx, msgs, opts)
		if err != nil {
			slog.Error("jiminy classifier: LLM classification failed", "error", err)
			return ClassificationResult{Outcome: OutcomeUnknown, Confidence: baseSimilarity}
		}
	}

	slog.Debug("jiminy classifier: raw LLM response", "response_len", len(response), "response_preview", encoding.TruncateAtWord(response, 200))

	// Parse structured response
	cr := parseClassifyResponse(response, baseSimilarity)
	if cr.Outcome != OutcomeUnknown {
		cr.Source = "llm"
	}

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
