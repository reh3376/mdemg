// Package metalearn implements cross-space concept promotion (Phase 105).
// High-value L4/L5 concepts are generalized via LLM and promoted to mdemg-global.
package metalearn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
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
}

// NewGeneralizer creates a new Generalizer with the given config and optional circuit breaker registry.
func NewGeneralizer(cfg GeneralizerConfig, cbRegistry *circuitbreaker.Registry) *Generalizer {
	return &Generalizer{cfg: cfg, cbRegistry: cbRegistry}
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

	var raw string
	var err error

	switch g.cfg.Provider {
	case "ollama":
		raw, err = g.generalizeWithOllama(timeoutCtx, userPrompt)
	default: // "openai" or unset
		raw, err = g.generalizeWithOpenAI(timeoutCtx, userPrompt)
	}
	if err != nil {
		return nil, err
	}

	raw = stripCodeFence(raw)
	raw = strings.TrimSpace(raw)

	var result GeneralizeResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM: %w (raw: %s)", err, truncate(raw, 200))
	}
	if strings.TrimSpace(result.Name) == "" {
		return nil, fmt.Errorf("LLM returned empty name")
	}

	return &result, nil
}

// --- OpenAI integration ---

type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_completion_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (g *Generalizer) generalizeWithOpenAI(ctx context.Context, userPrompt string) (string, error) {
	if g.cbRegistry != nil {
		cb := g.cbRegistry.Get("openai-metalearn")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = g.doGeneralizeWithOpenAI(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai metalearn circuit breaker open")
		}
		return result, err
	}
	return g.doGeneralizeWithOpenAI(ctx, userPrompt)
}

func (g *Generalizer) doGeneralizeWithOpenAI(ctx context.Context, userPrompt string) (string, error) {
	maxTokens := g.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 500
	}

	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	reqBody := openAIChatRequest{
		Model: g.cfg.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: generalizerSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := g.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// --- Ollama integration ---

type ollamaGenerateRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Stream  bool            `json:"stream"`
	Format  json.RawMessage `json:"format,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

var ollamaGeneralizerSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name": {"type": "string"},
		"description": {"type": "string"}
	},
	"required": ["name", "description"]
}`)

func (g *Generalizer) generalizeWithOllama(ctx context.Context, userPrompt string) (string, error) {
	if g.cbRegistry != nil {
		cb := g.cbRegistry.Get("ollama-metalearn")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = g.doGeneralizeWithOllama(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama metalearn circuit breaker open")
		}
		return result, err
	}
	return g.doGeneralizeWithOllama(ctx, userPrompt)
}

func (g *Generalizer) doGeneralizeWithOllama(ctx context.Context, userPrompt string) (string, error) {
	prompt := generalizerSystemPrompt + "\n\n" + userPrompt

	reqBody := ollamaGenerateRequest{
		Model:   g.cfg.Model,
		Prompt:  prompt,
		Stream:  false,
		Format:  ollamaGeneralizerSchema,
		Options: map[string]any{"temperature": 0.3},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := g.cfg.OllamaURL + "/api/generate"
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

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}

// --- Helpers ---

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSuffix(after, "```")
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSuffix(after, "```")
	}
	return strings.TrimSpace(s)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
