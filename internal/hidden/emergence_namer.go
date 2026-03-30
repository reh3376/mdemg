// Package hidden — emergence_namer.go implements LLM-driven concept naming (Phase 103).
// When dense clusters of CO_ACTIVATED_WITH edges don't match any hardcoded pattern,
// this namer sends cluster members to an LLM for automatic concept discovery.
package hidden

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/llmclient"
)

// EmergenceNamerConfig holds LLM configuration for the emergence namer.
type EmergenceNamerConfig struct {
	Enabled   bool
	Provider  string // "openai" or "ollama"
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// ClusterNodeSummary describes a single node within a cluster for the LLM prompt.
type ClusterNodeSummary struct {
	NodeID  string
	Name    string
	Summary string
	Path    string
	Layer   int
}

// EmergenceNamingResult is the JSON response expected from the LLM.
type EmergenceNamingResult struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	ProposedLabel string `json:"proposed_label"`
}

// validLabels defines the constrained set of proposed labels the LLM may choose.
var validLabels = map[string]bool{
	"pattern":   true,
	"principle": true,
	"bridge":    true,
	"concern":   true,
	"workflow":  true,
}

// EmergenceNamer calls an LLM to name emergent clusters.
type EmergenceNamer struct {
	cfg        EmergenceNamerConfig
	cbRegistry *circuitbreaker.Registry
	llm        *llmclient.Client
}

// NewEmergenceNamer creates a new namer with the given config and optional circuit breaker registry.
func NewEmergenceNamer(cfg EmergenceNamerConfig, cbRegistry *circuitbreaker.Registry) *EmergenceNamer {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &EmergenceNamer{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("hidden.name_emergence", ""),
	}
}

const emergenceSystemPrompt = `You are an AI concept discovery engine analyzing a knowledge graph.

You have found a cluster of nodes frequently co-activated together but not
matching any known pattern category (not auth, config, temporal, UI, or comparisons).

Your task: Name the emergent concept they collectively represent.

## Rules
- Name MUST be concise (3-6 words), descriptive, domain-specific
- Description MUST explain WHY these nodes cluster (1-2 sentences)
- proposed_label MUST be one of: "pattern", "principle", "bridge", "concern", "workflow"
- Output ONLY valid JSON — no markdown, no preamble

{"name": "<concept>", "description": "<why>", "proposed_label": "<label>"}`

const l5EmergenceSystemPrompt = `You are an AI concept discovery engine analyzing the highest layer of a knowledge graph.

You have found a cluster of ABSTRACT CONCEPTS (not raw code nodes) that are
structurally connected via analogous, bridging, or compositional relationships.
These are already higher-level abstractions — your job is to name the META-CONCEPT
that unifies them into a single coherent idea.

Think like a human synthesizing understanding: not "memory A intersected with memory B"
but "the underlying principle that connects them."

## Rules
- Name MUST be concise (3-6 words), abstract, and meaningful
- Name should capture the ESSENCE, not list the components
- Description MUST explain the unifying principle (1-2 sentences)
- proposed_label MUST be one of: "pattern", "principle", "bridge", "concern", "workflow"
- Output ONLY valid JSON — no markdown, no preamble

{"name": "<concept>", "description": "<why>", "proposed_label": "<label>"}`

// Name sends cluster members to the LLM and returns a naming result.
func (n *EmergenceNamer) Name(ctx context.Context, nodes []ClusterNodeSummary) (*EmergenceNamingResult, error) {
	return n.nameWithPrompt(ctx, emergenceSystemPrompt, nodes)
}

// NameL5Concept names a meta-concept from higher-level abstract nodes (L3+).
// Uses a system prompt tailored for synthesizing meaning from already-abstract concepts.
func (n *EmergenceNamer) NameL5Concept(ctx context.Context, nodes []ClusterNodeSummary) (*EmergenceNamingResult, error) {
	return n.nameWithPrompt(ctx, l5EmergenceSystemPrompt, nodes)
}

// nameWithPrompt is the shared implementation for Name and NameL5Concept.
func (n *EmergenceNamer) nameWithPrompt(ctx context.Context, sysPrompt string, nodes []ClusterNodeSummary) (*EmergenceNamingResult, error) {
	if !n.cfg.Enabled {
		return nil, fmt.Errorf("emergence namer is disabled")
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("empty cluster")
	}

	timeoutMs := n.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 10000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Build user prompt with cluster members
	var sb strings.Builder
	sb.WriteString("## Cluster Members\n")
	for _, node := range nodes {
		fmt.Fprintf(&sb, "[Node: name=%q, path=%q, summary=%q]\n", node.Name, node.Path, node.Summary)
	}

	userPrompt := sb.String()

	msgs := []llmclient.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt},
	}

	maxTokens := n.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 500
	}
	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}
	if n.cfg.Provider == "ollama" {
		opts.Format = ollamaEmergenceSchema
		opts.Options = map[string]any{"temperature": 0.3}
	}

	cbName := "openai-emergence"
	if n.cfg.Provider == "ollama" {
		cbName = "ollama-emergence"
	}

	var raw string
	var err error

	if n.cbRegistry != nil {
		cb := n.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			raw, innerErr = n.llm.Complete(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return nil, fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		raw, err = n.llm.Complete(timeoutCtx, msgs, opts)
	}

	if err != nil {
		return nil, err
	}

	raw = llmclient.SanitizeResponse(raw)

	var result EmergenceNamingResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM: %w (raw: %s)", err, llmclient.TruncateForLog(raw, 200))
	}

	// Validate proposed_label
	if !validLabels[result.ProposedLabel] {
		return nil, fmt.Errorf("invalid proposed_label %q (must be one of: pattern, principle, bridge, concern, workflow)", result.ProposedLabel)
	}

	if strings.TrimSpace(result.Name) == "" {
		return nil, fmt.Errorf("LLM returned empty name")
	}

	return &result, nil
}

// ollamaEmergenceSchema is the JSON schema for grammar-constrained output (Ollama v0.5+).
var ollamaEmergenceSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name": {"type": "string"},
		"description": {"type": "string"},
		"proposed_label": {"type": "string", "enum": ["pattern","principle","bridge","concern","workflow"]}
	},
	"required": ["name", "description", "proposed_label"]
}`)
