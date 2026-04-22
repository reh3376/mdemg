package guardrail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
)

// llmEvalResult holds the parsed LLM evaluation output.
type llmEvalResult struct {
	Violations []llmViolation `json:"violations"`
	Warnings   []llmViolation `json:"warnings"`
}

// llmViolation represents a single violation/warning from the LLM.
type llmViolation struct {
	ConstraintNodeID string `json:"constraint_node_id"`
	Description      string `json:"description"`
	Rationale        string `json:"rationale"`
}

// Per-task circuit breaker names — Sprint FT-LORA-B hard cutover.
// Previous names ("openai-guardrail", "ollama-guardrail") are REMOVED.
// POST /v1/admin/breakers/reset with the old names returns 404; operators
// must discover current names via GET /v1/admin/breakers. See CHANGELOG.
const (
	breakerNameOpenAI = "openai-guardrail.evaluate"
	breakerNameOllama = "ollama-guardrail.evaluate"
)

// evaluateWithLLM dispatches to llmclient for constraint evaluation.
// Both OpenAI and Ollama paths now go through the same llmclient.Completer;
// provider selection happened at GuardrailService construction time (in server.go).
// Circuit breaker name is per-task (guardrail.evaluate) and per-provider.
func (g *GuardrailService) evaluateWithLLM(ctx context.Context, diffCtx DiffContext, constraints []constraintMatch) (*llmEvalResult, error) {
	if g.llm == nil {
		return nil, fmt.Errorf("guardrail: llm client not configured")
	}

	prompt := buildEvalPrompt(diffCtx, constraints, g.cfg.CompressPrompts)

	sysPrompt := guardrailSystemPrompt
	if g.cfg.CompressPrompts {
		sysPrompt = guardrailSystemPromptCompact
	}

	maxTokens := g.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1000
	}
	if maxTokens < 2000 {
		maxTokens = 2000 // reasoning models consume tokens for internal thought
	}

	messages := []llmclient.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: prompt},
	}
	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}

	// Select the per-task circuit breaker name by provider. When no registry
	// is configured, bypass the breaker entirely (matches the pattern in
	// consulting/llm_classifier.go:138-150).
	breakerName := breakerNameOpenAI
	if g.cfg.Provider == "ollama" {
		breakerName = breakerNameOllama
	}

	var rawJSON string
	var err error
	if g.cbRegistry != nil {
		cb := g.cbRegistry.Get(breakerName)
		err = cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			rawJSON, innerErr = g.llm.Complete(ctx, messages, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, fmt.Errorf("%s circuit breaker open", breakerName)
		}
	} else {
		rawJSON, err = g.llm.Complete(ctx, messages, opts)
	}

	if err != nil {
		return nil, err
	}

	// Parse JSON response
	cleaned := cleanJSONResponse(rawJSON)

	var result llmEvalResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parse LLM response: %w (raw: %s)", err, truncateString(cleaned, 200))
	}

	// Ensure non-nil slices
	if result.Violations == nil {
		result.Violations = []llmViolation{}
	}
	if result.Warnings == nil {
		result.Warnings = []llmViolation{}
	}

	return &result, nil
}

// cleanJSONResponse strips markdown code fences and extracts the first valid
// JSON object from LLM responses. Handles trailing braces, commentary after
// the JSON, and text before the opening brace that some models emit.
func cleanJSONResponse(raw string) string {
	s := strings.TrimSpace(raw)

	// Strip ```json ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		// Remove opening fence
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			s = s[idx+1:]
		}
		// Remove closing fence
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}

	// Extract the first valid JSON object. LLMs sometimes emit trailing
	// braces, duplicate objects, or commentary after the JSON.
	// json.Decoder reads exactly one complete value and stops.
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}

	dec := json.NewDecoder(strings.NewReader(s[start:]))
	var obj json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return s // Fall back to fence-stripped text
	}
	return string(obj)
}
