package guardrail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"mdemg/internal/circuitbreaker"
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

// Local copies of request/response types (follows established duplication pattern in rerank.go/synthesis.go)
type guardrailOpenAIChatRequest struct {
	Model     string                   `json:"model"`
	Messages  []guardrailOpenAIMessage `json:"messages"`
	MaxTokens int                      `json:"max_completion_tokens"`
}

type guardrailOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type guardrailOpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type guardrailOllamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type guardrailOllamaGenerateResponse struct {
	Response string `json:"response"`
}

// evaluateWithLLM dispatches to OpenAI or Ollama for constraint evaluation.
func (g *GuardrailService) evaluateWithLLM(ctx context.Context, diffCtx DiffContext, constraints []constraintMatch) (*llmEvalResult, error) {
	prompt := buildEvalPrompt(diffCtx, constraints, g.cfg.CompressPrompts)

	var rawJSON string
	var err error

	switch g.cfg.Provider {
	case "ollama":
		rawJSON, err = g.evaluateWithOllama(ctx, prompt)
	default: // "openai" or unset
		rawJSON, err = g.evaluateWithOpenAI(ctx, prompt)
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

// evaluateWithOpenAI calls OpenAI with circuit breaker protection.
func (g *GuardrailService) evaluateWithOpenAI(ctx context.Context, prompt string) (string, error) {
	if g.cbRegistry != nil {
		cb := g.cbRegistry.Get("openai-guardrail")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = g.doEvaluateWithOpenAI(ctx, prompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai guardrail circuit breaker open")
		}
		return result, err
	}
	return g.doEvaluateWithOpenAI(ctx, prompt)
}

func (g *GuardrailService) doEvaluateWithOpenAI(ctx context.Context, prompt string) (string, error) {
	maxTokens := g.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1000
	}

	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	sysPrompt := guardrailSystemPrompt
	if g.cfg.CompressPrompts {
		sysPrompt = guardrailSystemPromptCompact
	}

	reqBody := guardrailOpenAIChatRequest{
		Model: g.cfg.Model,
		Messages: []guardrailOpenAIMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: prompt},
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

	var chatResp guardrailOpenAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// evaluateWithOllama calls Ollama with circuit breaker protection.
func (g *GuardrailService) evaluateWithOllama(ctx context.Context, prompt string) (string, error) {
	if g.cbRegistry != nil {
		cb := g.cbRegistry.Get("ollama-guardrail")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = g.doEvaluateWithOllama(ctx, prompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama guardrail circuit breaker open")
		}
		return result, err
	}
	return g.doEvaluateWithOllama(ctx, prompt)
}

func (g *GuardrailService) doEvaluateWithOllama(ctx context.Context, prompt string) (string, error) {
	// Combine system prompt + user prompt for Ollama (no chat endpoint)
	sysPrompt := guardrailSystemPrompt
	if g.cfg.CompressPrompts {
		sysPrompt = guardrailSystemPromptCompact
	}
	fullPrompt := sysPrompt + "\n\n" + prompt

	reqBody := guardrailOllamaGenerateRequest{
		Model:  g.cfg.Model,
		Prompt: fullPrompt,
		Stream: false,
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

	var ollamaResp guardrailOllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Response), nil
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
