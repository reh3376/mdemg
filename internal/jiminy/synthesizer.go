package jiminy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
	"mdemg/internal/sanitize"
)

// SynthesisConfig holds configuration for the Jiminy LLM synthesizer.
type SynthesisConfig struct {
	Enabled         bool
	Provider        string
	Model           string
	MaxTokens       int
	TimeoutMs       int
	OpenAIKey       string
	OpenAIURL       string
	OllamaURL       string
	Temperature     *float64 // J15: optional temperature (nil = API default)
	ContextMaxChars int      // J16: max chars for agent context (default: 200000, 0 = unlimited)
	OutputMaxChars  int      // J16: max chars for agent output (default: 200000, 0 = unlimited)
	// JIMINY-ACTIONABILITY-001 Lever B: render abstraction-type guidance as
	// imperative directives. Default-off; bounded prompt; reuses this same call.
	DirectiveMode            bool
	DirectiveMaxPromptTokens int
}

// GuidanceSynthesizer synthesizes guidance items into coherent LLM-generated narratives.
type GuidanceSynthesizer struct {
	cfg        SynthesisConfig
	cbRegistry *circuitbreaker.Registry
	llm        *llmclient.Client
}

// boundDirectivePrompt trims prompt to ~maxPromptTokens (×4 chars/token) in
// directive mode so the augmented system prompt + user prompt stay inside the
// llama-server KV slot. maxPromptTokens ≤ 0 = no bound.
func boundDirectivePrompt(prompt string, maxPromptTokens int) string {
	if maxPromptTokens <= 0 {
		return prompt
	}
	maxChars := maxPromptTokens * 4
	if len(prompt) > maxChars {
		return prompt[:maxChars] + "\n…[bounded]"
	}
	return prompt
}

// NewGuidanceSynthesizer creates a new GuidanceSynthesizer.
func NewGuidanceSynthesizer(cfg SynthesisConfig, cbRegistry *circuitbreaker.Registry) *GuidanceSynthesizer {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &GuidanceSynthesizer{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("jiminy.synthesize", ""),
	}
}

// Synthesize produces a coherent guidance narrative from items using LLM reasoning.
// Returns empty string and error on failure; caller should fall back to FormatPromptAugmentation.
func (gs *GuidanceSynthesizer) Synthesize(ctx context.Context, items []GuidanceItem, agentContext, agentOutput string) (string, error) {
	if !gs.cfg.Enabled {
		return "", fmt.Errorf("synthesis disabled")
	}
	if len(items) == 0 {
		return "", nil
	}

	prompt := buildGuidancePrompt(items, agentContext, agentOutput, gs.cfg.ContextMaxChars, gs.cfg.OutputMaxChars)

	// JIMINY-ACTIONABILITY-001 Lever B: in directive mode, augment the system
	// prompt to render abstraction principles as imperative directives, and bound
	// the user prompt to the token budget (fixed addition, no growth-with-state).
	systemPrompt := guidanceSystemPrompt
	if gs.cfg.DirectiveMode {
		systemPrompt = guidanceSystemPrompt + "\n\n" + directiveSynthesisInstruction
		prompt = boundDirectivePrompt(prompt, gs.cfg.DirectiveMaxPromptTokens)
	}

	timeoutMs := gs.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	maxTokens := gs.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	// Enforce minimum for reasoning models
	if maxTokens < 2000 {
		maxTokens = 2000
	}

	msgs := []llmclient.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: prompt},
	}
	opts := llmclient.CompleteOpts{
		MaxTokens:   maxTokens,
		Temperature: gs.cfg.Temperature,
	}
	// J15: Ollama structured output schema
	if gs.cfg.Provider == "ollama" {
		opts.Format = ollamaSynthesisSchema
	}

	cbName := "jiminy-synthesis"
	if gs.cfg.Provider == "ollama" {
		cbName = "jiminy-synthesis-ollama"
	}

	var narrative string

	if gs.cbRegistry != nil {
		cb := gs.cbRegistry.Get(cbName)
		err := cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			narrative, _, _, innerErr = gs.llm.CompleteWithUsage(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("%s circuit breaker open", cbName)
		}
		if err != nil {
			return "", fmt.Errorf("synthesis failed: %w", err)
		}
	} else {
		var err error
		narrative, _, _, err = gs.llm.CompleteWithUsage(timeoutCtx, msgs, opts)
		if err != nil {
			return "", fmt.Errorf("synthesis failed: %w", err)
		}
	}

	slog.Info("jiminy: synthesized guidance narrative", "chars", len(narrative))
	return sanitize.StripControlChars(narrative), nil
}
