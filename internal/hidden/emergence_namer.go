// Package hidden — emergence_namer.go implements LLM-driven concept naming (Phase 103).
// When dense clusters of CO_ACTIVATED_WITH edges don't match any hardcoded pattern,
// this namer sends cluster members to an LLM for automatic concept discovery.
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
}

// NewEmergenceNamer creates a new namer with the given config and optional circuit breaker registry.
func NewEmergenceNamer(cfg EmergenceNamerConfig, cbRegistry *circuitbreaker.Registry) *EmergenceNamer {
	return &EmergenceNamer{cfg: cfg, cbRegistry: cbRegistry}
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

// Name sends cluster members to the LLM and returns a naming result.
func (n *EmergenceNamer) Name(ctx context.Context, nodes []ClusterNodeSummary) (*EmergenceNamingResult, error) {
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

	var raw string
	var err error

	switch n.cfg.Provider {
	case "ollama":
		raw, err = n.nameWithOllama(timeoutCtx, userPrompt)
	default: // "openai" or unset
		raw, err = n.nameWithOpenAI(timeoutCtx, userPrompt)
	}

	if err != nil {
		return nil, err
	}

	// Strip markdown code fences if present (LLMs sometimes wrap JSON)
	raw = stripCodeFence(raw)
	raw = strings.TrimSpace(raw)

	var result EmergenceNamingResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM: %w (raw: %s)", err, truncateForLog(raw, 200))
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

// --- OpenAI integration ---

type emergenceOpenAIChatRequest struct {
	Model       string                    `json:"model"`
	Messages    []emergenceOpenAIMessage  `json:"messages"`
	Temperature float64                   `json:"temperature"`
	MaxTokens   int                       `json:"max_tokens"`
}

type emergenceOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type emergenceOpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (n *EmergenceNamer) nameWithOpenAI(ctx context.Context, userPrompt string) (string, error) {
	if n.cbRegistry != nil {
		cb := n.cbRegistry.Get("openai-emergence")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = n.doNameWithOpenAI(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai emergence circuit breaker open")
		}
		return result, err
	}
	return n.doNameWithOpenAI(ctx, userPrompt)
}

func (n *EmergenceNamer) doNameWithOpenAI(ctx context.Context, userPrompt string) (string, error) {
	maxTokens := n.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 500
	}

	reqBody := emergenceOpenAIChatRequest{
		Model: n.cfg.Model,
		Messages: []emergenceOpenAIMessage{
			{Role: "system", Content: emergenceSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
		MaxTokens:   maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := n.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+n.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp emergenceOpenAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// --- Ollama integration ---

type emergenceOllamaGenerateRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Stream  bool            `json:"stream"`
	Format  json.RawMessage `json:"format,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

type emergenceOllamaGenerateResponse struct {
	Response string `json:"response"`
}

func (n *EmergenceNamer) nameWithOllama(ctx context.Context, userPrompt string) (string, error) {
	if n.cbRegistry != nil {
		cb := n.cbRegistry.Get("ollama-emergence")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = n.doNameWithOllama(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama emergence circuit breaker open")
		}
		return result, err
	}
	return n.doNameWithOllama(ctx, userPrompt)
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

func (n *EmergenceNamer) doNameWithOllama(ctx context.Context, userPrompt string) (string, error) {
	prompt := emergenceSystemPrompt + "\n\n" + userPrompt

	reqBody := emergenceOllamaGenerateRequest{
		Model:   n.cfg.Model,
		Prompt:  prompt,
		Stream:  false,
		Format:  ollamaEmergenceSchema,
		Options: map[string]any{"temperature": 0.3},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := n.cfg.OllamaURL + "/api/generate"
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

	var ollamaResp emergenceOllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Response), nil
}

// --- Helpers ---

// stripCodeFence removes markdown code fences that LLMs sometimes wrap around JSON.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimSuffix(s, "```")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

// truncateForLog truncates a string for error log messages.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
