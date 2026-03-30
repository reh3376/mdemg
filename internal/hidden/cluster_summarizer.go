// Package hidden — cluster_summarizer.go implements LLM-driven summarization for L1-L4 nodes.
// Replaces mechanical summaries (e.g., "Pattern of X, Y, Z") with semantic descriptions
// that capture the domain-specific meaning of clustered concepts.
package hidden

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
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
	llm        *llmclient.Client
}

// NewClusterSummarizer creates a new summarizer with the given config and optional circuit breaker registry.
func NewClusterSummarizer(cfg ClusterSummarizerConfig, cbRegistry *circuitbreaker.Registry) *ClusterSummarizer {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &ClusterSummarizer{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("hidden.summarize", ""),
	}
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

	msgs := []llmclient.Message{
		{Role: "system", Content: clusterSummarySystemPrompt},
		{Role: "user", Content: userPrompt},
	}

	maxTokens := cs.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 100
	}
	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}
	if cs.cfg.Provider == "ollama" {
		opts.Options = map[string]any{"temperature": 0.3}
	}

	cbName := "openai-cluster-summary"
	if cs.cfg.Provider == "ollama" {
		cbName = "ollama-cluster-summary"
	}

	var raw string
	var err error

	if cs.cbRegistry != nil {
		cb := cs.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			raw, innerErr = cs.llm.Complete(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		raw, err = cs.llm.Complete(timeoutCtx, msgs, opts)
	}

	if err != nil {
		return "", err
	}

	// Clean up response
	raw = llmclient.StripCodeFence(raw)
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
