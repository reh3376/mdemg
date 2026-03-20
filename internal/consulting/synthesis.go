// Package consulting — synthesis.go implements the SME Synthesis Engine (Phase 101).
// This is the cognitive core: it takes retrieved graph evidence + the user's question
// and produces a coherent organizational SME narrative via LLM, grounded exclusively
// in graph evidence (not general LLM knowledge).
package consulting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mdemg/internal/sanitize"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/models"
)

// Synthesizer defines the interface for LLM-based synthesis of graph evidence.
type Synthesizer interface {
	Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error)
}

// SynthesisRequest contains all the evidence needed for synthesis.
type SynthesisRequest struct {
	SpaceID     string
	Question    string
	Context     string
	Results     []models.RetrieveResult
	Concepts    []models.RelatedConcept
	Suggestions []models.Suggestion
}

// SynthesisResult contains the synthesized narrative and metadata.
type SynthesisResult struct {
	Narrative  string
	TokensUsed int
	LatencyMs  float64
	Provider   string
	Model      string
}

// SynthesisConfig holds configuration for the LLM synthesizer.
type SynthesisConfig struct {
	Enabled   bool
	Provider  string
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// LLMSynthesizer implements Synthesizer using OpenAI or Ollama.
type LLMSynthesizer struct {
	cfg        SynthesisConfig
	cbRegistry *circuitbreaker.Registry
}

// NewLLMSynthesizer creates a new LLM-based synthesizer.
func NewLLMSynthesizer(cfg SynthesisConfig, cbRegistry *circuitbreaker.Registry) *LLMSynthesizer {
	return &LLMSynthesizer{
		cfg:        cfg,
		cbRegistry: cbRegistry,
	}
}

// Synthesize produces a coherent SME narrative from graph evidence.
func (s *LLMSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	if !s.cfg.Enabled {
		return SynthesisResult{}, fmt.Errorf("synthesis is disabled")
	}

	if len(req.Results) == 0 {
		return SynthesisResult{}, fmt.Errorf("no results to synthesize")
	}

	prompt := buildSynthesisPrompt(req)

	timeoutMs := s.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	start := time.Now()

	var narrative string
	var tokensUsed int
	var err error

	switch s.cfg.Provider {
	case "ollama":
		narrative, tokensUsed, err = s.synthesizeWithOllama(timeoutCtx, prompt)
	default: // "openai" or unset
		narrative, tokensUsed, err = s.synthesizeWithOpenAI(timeoutCtx, prompt)
	}

	if err != nil {
		return SynthesisResult{}, fmt.Errorf("synthesis failed: %w", err)
	}

	return SynthesisResult{
		Narrative:  narrative,
		TokensUsed: tokensUsed,
		LatencyMs:  float64(time.Since(start).Milliseconds()),
		Provider:   s.cfg.Provider,
		Model:      s.cfg.Model,
	}, nil
}

// --- OpenAI integration ---

// Local copies of request/response types (follows established duplication pattern in rerank.go)
type synthOpenAIChatRequest struct {
	Model     string               `json:"model"`
	Messages  []synthOpenAIMessage `json:"messages"`
	MaxTokens int                  `json:"max_completion_tokens"`
}

type synthOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type synthOpenAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *LLMSynthesizer) synthesizeWithOpenAI(ctx context.Context, prompt string) (string, int, error) {
	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get("openai-synthesis")
		var narrative string
		var tokens int
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			narrative, tokens, innerErr = s.doSynthesizeWithOpenAI(ctx, prompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", 0, fmt.Errorf("openai synthesis circuit breaker open")
		}
		return narrative, tokens, err
	}
	return s.doSynthesizeWithOpenAI(ctx, prompt)
}

func (s *LLMSynthesizer) doSynthesizeWithOpenAI(ctx context.Context, prompt string) (string, int, error) {
	maxTokens := s.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}

	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	reqBody := synthOpenAIChatRequest{
		Model: s.cfg.Model,
		Messages: []synthOpenAIMessage{
			{Role: "user", Content: prompt},
		},
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := s.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp synthOpenAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", 0, fmt.Errorf("no choices in response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), chatResp.Usage.TotalTokens, nil
}

// --- Ollama integration ---

type synthOllamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type synthOllamaGenerateResponse struct {
	Response string `json:"response"`
}

func (s *LLMSynthesizer) synthesizeWithOllama(ctx context.Context, prompt string) (string, int, error) {
	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get("ollama-synthesis")
		var narrative string
		var tokens int
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			narrative, tokens, innerErr = s.doSynthesizeWithOllama(ctx, prompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", 0, fmt.Errorf("ollama synthesis circuit breaker open")
		}
		return narrative, tokens, err
	}
	return s.doSynthesizeWithOllama(ctx, prompt)
}

func (s *LLMSynthesizer) doSynthesizeWithOllama(ctx context.Context, prompt string) (string, int, error) {
	reqBody := synthOllamaGenerateRequest{
		Model:  s.cfg.Model,
		Prompt: prompt,
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := s.cfg.OllamaURL + "/api/generate"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp synthOllamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Response), 0, nil // Ollama doesn't report token usage
}

// --- Prompt construction (THE COGNITIVE CORE) ---

// buildSynthesisPrompt constructs the system prompt that constrains the LLM
// to synthesize ONLY from graph evidence, not general knowledge.
func buildSynthesisPrompt(req SynthesisRequest) string {
	var sb strings.Builder

	// Section 1: System framing — constrain LLM to graph evidence only
	sb.WriteString("You are a Subject Matter Expert for a specific software organization. ")
	sb.WriteString("You have deep expertise ONLY in the patterns, decisions, constraints, and history ")
	sb.WriteString("stored in this organization's knowledge graph. ")
	sb.WriteString("You do NOT have general programming knowledge beyond what is in the graph.\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Cite every claim as (Node: <node_id>)\n")
	sb.WriteString("- Do NOT invent information not present in the evidence below\n")
	sb.WriteString("- Surface risks prominently with WARNING prefix\n")
	sb.WriteString("- Use markdown formatting\n\n")

	// Section 2: Developer context + question (sanitized to prevent prompt injection)
	sb.WriteString("## Developer Context\n\n")
	ctx := sanitize.SanitizeUserContext(req.Context, 3000)
	if ctx != "" {
		sb.WriteString(ctx)
		sb.WriteString("\n\n")
	}
	sb.WriteString("## Question\n\n")
	sb.WriteString(req.Question)
	sb.WriteString("\n\n")

	// Section 3: Retrieved evidence (up to 15 nodes)
	sb.WriteString("## Retrieved Evidence\n\n")
	sb.WriteString("These are memory nodes from the organization's knowledge graph:\n\n")
	maxResults := 15
	if len(req.Results) < maxResults {
		maxResults = len(req.Results)
	}
	for i := 0; i < maxResults; i++ {
		r := req.Results[i]
		sb.WriteString(fmt.Sprintf("### [%d] Node: %s\n", i+1, r.NodeID))
		sb.WriteString(fmt.Sprintf("- **Score**: %.3f | **Layer**: %d\n", r.Score, r.Layer))
		if r.Name != "" {
			sb.WriteString(fmt.Sprintf("- **Name**: %s\n", r.Name))
		}
		if r.Path != "" {
			sb.WriteString(fmt.Sprintf("- **Path**: %s\n", r.Path))
		}
		summary := r.Summary
		if len(summary) > 800 {
			summary = summary[:800] + "..."
		}
		if summary != "" {
			sb.WriteString(fmt.Sprintf("- **Summary**: %s\n", summary))
		}
		sb.WriteString("\n")
	}

	// Section 4: Organizational concepts (hidden layer L2+)
	if len(req.Concepts) > 0 {
		sb.WriteString("## Organizational Concepts\n\n")
		sb.WriteString("These are emergent concepts identified by analyzing patterns across many source nodes:\n\n")
		for _, c := range req.Concepts {
			sb.WriteString(fmt.Sprintf("- **%s** (Node: %s, Layer: %d, Relevance: %.2f)\n", c.Name, c.NodeID, c.Layer, c.Relevance))
		}
		sb.WriteString("\n")
	}

	// Section 5: Pre-classified signals (keyword suggestions as hints)
	if len(req.Suggestions) > 0 {
		sb.WriteString("## Pre-Classified Signals\n\n")
		riskCount := 0
		for _, s := range req.Suggestions {
			if s.Type == models.SuggestionRisk {
				riskCount++
			}
		}
		if riskCount > 0 {
			sb.WriteString(fmt.Sprintf("**WARNING: %d risk signal(s) detected.**\n\n", riskCount))
		}
		for _, s := range req.Suggestions {
			sb.WriteString(fmt.Sprintf("- [%s] %s (confidence: %.2f)\n", s.Type, s.Content, s.Confidence))
		}
		sb.WriteString("\n")
	}

	// Section 6: Output format
	sb.WriteString("## Required Output Format\n\n")
	sb.WriteString("Respond with these sections:\n\n")
	sb.WriteString("### Direct Answer\n")
	sb.WriteString("Answer the developer's question using ONLY the evidence above. Cite nodes.\n\n")
	sb.WriteString("### Key Patterns & Constraints\n")
	sb.WriteString("Relevant patterns, conventions, and constraints from the graph. Cite nodes.\n\n")
	sb.WriteString("### Risks & Warnings\n")
	sb.WriteString("Any risks, deprecated patterns, or known issues. Cite nodes. If none, say \"No risks identified.\"\n\n")
	sb.WriteString("### Recommended Approach\n")
	sb.WriteString("Based on the evidence, what approach should the developer take? Cite nodes.\n")

	return sb.String()
}
