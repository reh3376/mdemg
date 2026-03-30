// Package consulting — synthesis.go implements the SME Synthesis Engine (Phase 101).
// This is the cognitive core: it takes retrieved graph evidence + the user's question
// and produces a coherent organizational SME narrative via LLM, grounded exclusively
// in graph evidence (not general LLM knowledge).
package consulting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/encoding"
	"mdemg/internal/llmclient"
	"mdemg/internal/models"
	"mdemg/internal/sanitize"
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
	Enabled         bool
	Provider        string
	Model           string
	MaxTokens       int
	TimeoutMs       int
	OpenAIKey       string
	OpenAIURL       string
	OllamaURL       string
	CompressPrompts bool // J17-PC: compress synthesis prompts to reduce tokens
}

// LLMSynthesizer implements Synthesizer using OpenAI or Ollama.
type LLMSynthesizer struct {
	cfg        SynthesisConfig
	cbRegistry *circuitbreaker.Registry
	llm        *llmclient.Client
}

// NewLLMSynthesizer creates a new LLM-based synthesizer.
func NewLLMSynthesizer(cfg SynthesisConfig, cbRegistry *circuitbreaker.Registry) *LLMSynthesizer {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &LLMSynthesizer{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("consulting.synthesis", ""),
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

	prompt := buildSynthesisPrompt(req, s.cfg.CompressPrompts)

	timeoutMs := s.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	start := time.Now()

	msgs := []llmclient.Message{
		{Role: "user", Content: prompt},
	}

	maxTokens := s.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	if maxTokens < 2000 {
		maxTokens = 2000 // Reasoning models consume tokens for internal thought
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}

	cbName := "openai-synthesis"
	if s.cfg.Provider == "ollama" {
		cbName = "ollama-synthesis"
	}

	var narrative string
	var tokensUsed int
	var err error

	if s.cbRegistry != nil {
		cb := s.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			narrative, tokensUsed, innerErr = s.llm.CompleteWithUsage(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return SynthesisResult{}, fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		narrative, tokensUsed, err = s.llm.CompleteWithUsage(timeoutCtx, msgs, opts)
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

// --- Prompt construction (THE COGNITIVE CORE) ---

// buildSynthesisPrompt constructs the system prompt that constrains the LLM
// to synthesize ONLY from graph evidence, not general knowledge.
// When compress is true, system framing and output format are condensed.
func buildSynthesisPrompt(req SynthesisRequest, compress bool) string {
	var sb strings.Builder

	// Section 1: System framing — constrain LLM to graph evidence only
	if compress {
		sb.WriteString("SME for software org. Expertise ONLY from knowledge graph below. ")
		sb.WriteString("Rules: cite as (Node: <id>), no invented info, WARNING prefix for risks, markdown.\n\n")
	} else {
		sb.WriteString("You are a Subject Matter Expert for a specific software organization. ")
		sb.WriteString("You have deep expertise ONLY in the patterns, decisions, constraints, and history ")
		sb.WriteString("stored in this organization's knowledge graph. ")
		sb.WriteString("You do NOT have general programming knowledge beyond what is in the graph.\n\n")
		sb.WriteString("Rules:\n")
		sb.WriteString("- Cite every claim as (Node: <node_id>)\n")
		sb.WriteString("- Do NOT invent information not present in the evidence below\n")
		sb.WriteString("- Surface risks prominently with WARNING prefix\n")
		sb.WriteString("- Use markdown formatting\n\n")
	}

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
	if !compress {
		sb.WriteString("These are memory nodes from the organization's knowledge graph:\n\n")
	}
	maxResults := 15
	if len(req.Results) < maxResults {
		maxResults = len(req.Results)
	}
	summaryMaxLen := 800
	if compress {
		summaryMaxLen = 400
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
		if compress && summary != "" {
			summary = encoding.CompressSection(summary, summaryMaxLen)
		} else if len(summary) > summaryMaxLen {
			summary = summary[:summaryMaxLen] + "..."
		}
		if summary != "" {
			sb.WriteString(fmt.Sprintf("- **Summary**: %s\n", summary))
		}
		sb.WriteString("\n")
	}

	// Section 4: Organizational concepts (hidden layer L2+)
	if len(req.Concepts) > 0 {
		sb.WriteString("## Organizational Concepts\n\n")
		if !compress {
			sb.WriteString("These are emergent concepts identified by analyzing patterns across many source nodes:\n\n")
		}
		// Cap concepts at 10 when compressing (currently uncapped)
		maxConcepts := len(req.Concepts)
		if compress && maxConcepts > 10 {
			maxConcepts = 10
		}
		for i := 0; i < maxConcepts; i++ {
			c := req.Concepts[i]
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
	if compress {
		sb.WriteString("## Output: Direct Answer (cite nodes) | Key Patterns | Risks & Warnings | Recommended Approach\n")
	} else {
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
	}

	return sb.String()
}
