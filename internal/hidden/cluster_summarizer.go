// Package hidden — cluster_summarizer.go implements LLM-driven summarization for L1-L4 nodes.
// Replaces mechanical summaries (e.g., "Pattern of X, Y, Z") with semantic descriptions
// that capture the domain-specific meaning of clustered concepts.
package hidden

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

// ClusterSummarizerConfig holds LLM configuration for the cluster summarizer.
type ClusterSummarizerConfig struct {
	Enabled   bool
	Provider  string // "openai" or "ollama"
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// ClusterSummarizer calls an LLM to generate semantic summaries for cluster nodes.
type ClusterSummarizer struct {
	cfg        ClusterSummarizerConfig
	cbRegistry *circuitbreaker.Registry
}

// NewClusterSummarizer creates a new summarizer with the given config and optional circuit breaker registry.
func NewClusterSummarizer(cfg ClusterSummarizerConfig, cbRegistry *circuitbreaker.Registry) *ClusterSummarizer {
	return &ClusterSummarizer{cfg: cfg, cbRegistry: cbRegistry}
}

const clusterSummarySystemPrompt = `You are a knowledge graph summarizer. Given a list of related code/concept nodes that form a cluster, write a concise semantic summary that captures what this cluster represents.

Rules:
- Output ONLY the summary text — no explanation, no preamble, no quotes
- Keep it under 50 words
- Be domain-specific and precise
- Focus on what unifies these nodes, not listing them
- If the nodes are code files, describe the shared concern or pattern`

// Summarize generates a semantic summary for a cluster of nodes.
func (cs *ClusterSummarizer) Summarize(ctx context.Context, memberNames, memberSummaries []string, layer int) (string, error) {
	if !cs.cfg.Enabled {
		return "", fmt.Errorf("cluster summarizer is disabled")
	}
	if len(memberNames) == 0 {
		return "", fmt.Errorf("empty member list")
	}

	timeoutMs := cs.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000 // Reasoning models need more time
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Build user prompt with cluster members
	var sb strings.Builder
	fmt.Fprintf(&sb, "Layer: %d\n\nCluster Members:\n", layer)
	for i, name := range memberNames {
		summary := ""
		if i < len(memberSummaries) {
			summary = memberSummaries[i]
		}
		if summary != "" {
			fmt.Fprintf(&sb, "- %s: %s\n", name, summary)
		} else {
			fmt.Fprintf(&sb, "- %s\n", name)
		}
	}

	userPrompt := sb.String()

	var raw string
	var err error

	switch cs.cfg.Provider {
	case "ollama":
		raw, err = cs.summarizeWithOllama(timeoutCtx, userPrompt)
	default: // "openai" or unset
		raw, err = cs.summarizeWithOpenAI(timeoutCtx, userPrompt)
	}

	if err != nil {
		return "", err
	}

	// Clean up response
	raw = stripCodeFence(raw)
	raw = strings.TrimSpace(raw)
	// Remove surrounding quotes if present
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}

	if raw == "" {
		return "", fmt.Errorf("LLM returned empty summary")
	}

	return raw, nil
}

// --- OpenAI integration ---

type csOpenAIChatRequest struct {
	Model     string            `json:"model"`
	Messages  []csOpenAIMessage `json:"messages"`
	MaxTokens int               `json:"max_completion_tokens"`
}

type csOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type csOpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (cs *ClusterSummarizer) summarizeWithOpenAI(ctx context.Context, userPrompt string) (string, error) {
	if cs.cbRegistry != nil {
		cb := cs.cbRegistry.Get("openai-cluster-summary")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = cs.doSummarizeWithOpenAI(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai cluster summary circuit breaker open")
		}
		return result, err
	}
	return cs.doSummarizeWithOpenAI(ctx, userPrompt)
}

func (cs *ClusterSummarizer) doSummarizeWithOpenAI(ctx context.Context, userPrompt string) (string, error) {
	maxTokens := cs.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 100
	}

	// Use at least 2000 tokens — reasoning models consume tokens for internal thought
	// before producing visible output
	if maxTokens < 2000 {
		maxTokens = 2000
	}

	reqBody := csOpenAIChatRequest{
		Model: cs.cfg.Model,
		Messages: []csOpenAIMessage{
			{Role: "system", Content: clusterSummarySystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := cs.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cs.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp csOpenAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("empty content (finish_reason=%s, model may need more tokens)", chatResp.Choices[0].FinishReason)
	}

	return content, nil
}

// --- Ollama integration ---

type csOllamaGenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

type csOllamaGenerateResponse struct {
	Response string `json:"response"`
}

func (cs *ClusterSummarizer) summarizeWithOllama(ctx context.Context, userPrompt string) (string, error) {
	if cs.cbRegistry != nil {
		cb := cs.cbRegistry.Get("ollama-cluster-summary")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = cs.doSummarizeWithOllama(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama cluster summary circuit breaker open")
		}
		return result, err
	}
	return cs.doSummarizeWithOllama(ctx, userPrompt)
}

func (cs *ClusterSummarizer) doSummarizeWithOllama(ctx context.Context, userPrompt string) (string, error) {
	prompt := clusterSummarySystemPrompt + "\n\n" + userPrompt

	reqBody := csOllamaGenerateRequest{
		Model:   cs.cfg.Model,
		Prompt:  prompt,
		Stream:  false,
		Options: map[string]any{"temperature": 0.3},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := cs.cfg.OllamaURL + "/api/generate"
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

	var ollamaResp csOllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}
