// Package metalearn implements cross-space concept promotion (Phase 105).
// High-value L4/L5 concepts are generalized via LLM and promoted to mdemg-global.
package metalearn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
)

// GeneralizerConfig holds LLM configuration for concept generalization.
type GeneralizerConfig struct {
	Enabled   bool
	Provider  string // "openai" or "ollama"
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// GeneralizeResult is the JSON response expected from the LLM.
type GeneralizeResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Generalizer calls an LLM to strip local specifics from a concept.
type Generalizer struct {
	cfg        GeneralizerConfig
	cbRegistry *circuitbreaker.Registry
	llm        *llmclient.Client
}

// NewGeneralizer creates a new Generalizer with the given config and optional circuit breaker registry.
func NewGeneralizer(cfg GeneralizerConfig, cbRegistry *circuitbreaker.Registry) *Generalizer {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &Generalizer{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("metalearn.generalize", ""),
	}
}

const generalizerSystemPrompt = `You are a knowledge abstraction engine. Given a concept discovered in a specific codebase,
generalize it into a space-agnostic principle that would be useful across any project.

## Rules
- STRIP all repo-specific names (file paths, variable names, package names)
- STRIP any credentials, secrets, or internal URLs
- PRESERVE the core architectural insight or pattern
- Output ONLY valid JSON — no markdown, no preamble

{"name": "<generalized name>", "description": "<generalized principle>"}`

// Generalize strips local specifics from a concept via LLM.
func (g *Generalizer) Generalize(ctx context.Context, name, summary, description string) (*GeneralizeResult, error) {
	if !g.cfg.Enabled {
		return nil, fmt.Errorf("generalizer is disabled")
	}

	timeoutMs := g.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	var sb strings.Builder
	sb.WriteString("## Concept to Generalize\n")
	fmt.Fprintf(&sb, "Name: %s\n", name)
	if summary != "" {
		fmt.Fprintf(&sb, "Summary: %s\n", summary)
	}
	if description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", description)
	}
	userPrompt := sb.String()

	msgs := []llmclient.Message{
		{Role: "system", Content: generalizerSystemPrompt},
		{Role: "user", Content: userPrompt},
	}

	maxTokens := g.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 500
	}
	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}
	if g.cfg.Provider == "ollama" {
		opts.Format = ollamaGeneralizerSchema
		opts.Options = map[string]any{"temperature": 0.3}
	}

	cbName := "openai-metalearn"
	if g.cfg.Provider == "ollama" {
		cbName = "ollama-metalearn"
	}

	var raw string
	var err error

	if g.cbRegistry != nil {
		cb := g.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			raw, innerErr = g.llm.Complete(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		raw, err = g.llm.Complete(timeoutCtx, msgs, opts)
	}
	if err != nil {
		return nil, err
	}

	raw = llmclient.SanitizeResponse(raw)

	var result GeneralizeResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM: %w (raw: %s)", err, llmclient.TruncateForLog(raw, 200))
	}
	if strings.TrimSpace(result.Name) == "" {
		return nil, fmt.Errorf("LLM returned empty name")
	}

	return &result, nil
}

// ollamaGeneralizerSchema is the JSON schema for grammar-constrained output (Ollama v0.5+).
var ollamaGeneralizerSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name": {"type": "string"},
		"description": {"type": "string"}
	},
	"required": ["name", "description"]
}`)
