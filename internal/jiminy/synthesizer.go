package jiminy

import (
	"context"
	"fmt"
	"log"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
)

// SynthesisConfig holds configuration for the Jiminy LLM synthesizer.
type SynthesisConfig struct {
	Enabled     bool
	Provider    string
	Model       string
	MaxTokens   int
	TimeoutMs   int
	OpenAIKey   string
	OpenAIURL   string
	OllamaURL   string
	Temperature    *float64 // J15: optional temperature (nil = API default)
	ContextMaxChars int     // J16: max chars for agent context (default: 200000, 0 = unlimited)
	OutputMaxChars  int     // J16: max chars for agent output (default: 200000, 0 = unlimited)
}

// GuidanceSynthesizer synthesizes guidance items into coherent LLM-generated narratives.
type GuidanceSynthesizer struct {
	cfg        SynthesisConfig
	cbRegistry *circuitbreaker.Registry
	llm        *llmclient.Client
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
		}),
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
		{Role: "system", Content: guidanceSystemPrompt},
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
			narrative, _, innerErr = gs.llm.CompleteWithUsage(ctx, msgs, opts)
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
		narrative, _, err = gs.llm.CompleteWithUsage(timeoutCtx, msgs, opts)
		if err != nil {
			return "", fmt.Errorf("synthesis failed: %w", err)
		}
	}

	log.Printf("jiminy: synthesized guidance narrative (%d chars)", len(narrative))
	return narrative, nil
}
