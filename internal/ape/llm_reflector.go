// Package ape — llm_reflector.go implements LLM-powered RSIC reflection (Phase AR-3).
// When enabled, an LLM analyses the current assessment report and recent cycle
// history to produce insights that complement the rule-based reflector.
package ape

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

// LLMReflectorConfig holds configuration for the LLM reflector.
type LLMReflectorConfig struct {
	Enabled   bool
	Provider  string // "openai" or "ollama"
	Model     string
	MaxTokens int
	TimeoutMs int
	OpenAIKey string
	OpenAIURL string
	OllamaURL string
}

// LLMReflector sends assessment data and cycle history to an LLM for pattern detection.
// Follows the EmergenceNamer pattern (OpenAI/Ollama, circuit breaker, JSON-constrained output).
type LLMReflector struct {
	cfg        LLMReflectorConfig
	cbRegistry *circuitbreaker.Registry
	calibrator *Calibrator
}

// NewLLMReflector creates a new LLM-powered reflector.
func NewLLMReflector(cfg LLMReflectorConfig, cbRegistry *circuitbreaker.Registry, calibrator *Calibrator) *LLMReflector {
	return &LLMReflector{cfg: cfg, cbRegistry: cbRegistry, calibrator: calibrator}
}

const llmReflectSystemPrompt = `You are an RSIC (Recursive Self-Improvement Cycle) reflection engine for a knowledge graph memory system.

You receive:
1. A self-assessment report with health metrics
2. Recent cycle outcomes showing what actions were taken and whether they succeeded

Your task: Identify patterns the rule-based reflector might miss. Look for:
- Recurring failures in specific action types
- Metric trends across cycles (improving/degrading)
- Action types that consistently fail to meet criteria
- Correlations between metrics (e.g., high orphan ratio + low edge entropy)

## Rules
- Return a JSON array of insights (may be empty [])
- Each insight MUST have: pattern_id, severity, description, recommended_action, reasoning
- severity MUST be one of: "low", "medium", "high", "critical"
- recommended_action MUST be one of: "prune_decayed_edges", "prune_excess_edges", "tombstone_stale", "graduate_volatile", "trigger_consolidation", "refresh_stale_edges"
- Output ONLY valid JSON — no markdown, no preamble

[{"pattern_id": "...", "severity": "...", "description": "...", "recommended_action": "...", "reasoning": "..."}]`

// llmReflectInsight is the JSON structure returned by the LLM.
type llmReflectInsight struct {
	PatternID         string `json:"pattern_id"`
	Severity          string `json:"severity"`
	Description       string `json:"description"`
	RecommendedAction string `json:"recommended_action"`
	Reasoning         string `json:"reasoning"`
}

// Reflect sends assessment data and history to the LLM for analysis.
func (lr *LLMReflector) Reflect(ctx context.Context, report *SelfAssessmentReport) ([]ReflectionInsight, error) {
	if !lr.cfg.Enabled {
		return nil, nil
	}

	timeoutMs := lr.cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 15000
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Build user prompt with assessment + recent history
	userPrompt := lr.buildUserPrompt(report)

	var raw string
	var err error

	switch lr.cfg.Provider {
	case "ollama":
		raw, err = lr.callOllama(timeoutCtx, userPrompt)
	default: // "openai" or unset
		raw, err = lr.callOpenAI(timeoutCtx, userPrompt)
	}

	if err != nil {
		return nil, fmt.Errorf("llm reflector: %w", err)
	}

	return lr.parseResponse(raw)
}

func (lr *LLMReflector) buildUserPrompt(report *SelfAssessmentReport) string {
	var sb strings.Builder

	// Current assessment
	reportJSON, _ := json.MarshalIndent(report, "", "  ")
	sb.WriteString("## Current Assessment\n")
	sb.Write(reportJSON)
	sb.WriteString("\n\n")

	// Recent cycle history (last 5)
	if lr.calibrator != nil {
		history := lr.calibrator.GetHistory(5)
		if len(history) > 0 {
			sb.WriteString("## Recent Cycle History\n")
			for i, h := range history {
				fmt.Fprintf(&sb, "### Cycle %d: %s (tier=%s)\n", i+1, h.CycleID, h.Tier)
				fmt.Fprintf(&sb, "- Actions: %d executed, %d success, %d failed\n", h.ActionsExecuted, h.SuccessCount, h.FailedCount)
				fmt.Fprintf(&sb, "- Criteria met: %v\n", h.CriteriaMet)
				if len(h.MetricsBefore) > 0 {
					fmt.Fprintf(&sb, "- Metrics before: %v\n", h.MetricsBefore)
				}
				if len(h.MetricsAfter) > 0 {
					fmt.Fprintf(&sb, "- Metrics after: %v\n", h.MetricsAfter)
				}
				if len(h.CriteriaDetail) > 0 {
					fmt.Fprintf(&sb, "- Criteria detail: %v\n", h.CriteriaDetail)
				}
				sb.WriteString("\n")
			}
		}

		// Calibration confidence per action type
		calibration := lr.calibrator.GetCalibration()
		if len(calibration) > 0 {
			sb.WriteString("## Calibration Confidence (action → success rate)\n")
			for action, conf := range calibration {
				fmt.Fprintf(&sb, "- %s: %.2f\n", action, conf)
			}
		}
	}

	return sb.String()
}

// --- OpenAI integration ---

type llmReflectOpenAIRequest struct {
	Model     string                    `json:"model"`
	Messages  []llmReflectOpenAIMessage `json:"messages"`
	MaxTokens int                       `json:"max_completion_tokens"`
}

type llmReflectOpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type llmReflectOpenAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (lr *LLMReflector) callOpenAI(ctx context.Context, userPrompt string) (string, error) {
	if lr.cbRegistry != nil {
		cb := lr.cbRegistry.Get("openai-rsic-reflect")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = lr.doCallOpenAI(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("openai rsic-reflect circuit breaker open")
		}
		return result, err
	}
	return lr.doCallOpenAI(ctx, userPrompt)
}

func (lr *LLMReflector) doCallOpenAI(ctx context.Context, userPrompt string) (string, error) {
	maxTokens := lr.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	if maxTokens < 2000 {
		maxTokens = 2000
	}

	reqBody := llmReflectOpenAIRequest{
		Model: lr.cfg.Model,
		Messages: []llmReflectOpenAIMessage{
			{Role: "system", Content: llmReflectSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens: maxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := lr.cfg.OpenAIURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+lr.cfg.OpenAIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp llmReflectOpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

// --- Ollama integration ---

type llmReflectOllamaRequest struct {
	Model   string          `json:"model"`
	Prompt  string          `json:"prompt"`
	Stream  bool            `json:"stream"`
	Format  json.RawMessage `json:"format,omitempty"`
	Options map[string]any  `json:"options,omitempty"`
}

type llmReflectOllamaResponse struct {
	Response string `json:"response"`
}

var ollamaReflectSchema = json.RawMessage(`{
	"type": "array",
	"items": {
		"type": "object",
		"properties": {
			"pattern_id": {"type": "string"},
			"severity": {"type": "string", "enum": ["low","medium","high","critical"]},
			"description": {"type": "string"},
			"recommended_action": {"type": "string", "enum": ["prune_decayed_edges","prune_excess_edges","tombstone_stale","graduate_volatile","trigger_consolidation","refresh_stale_edges"]},
			"reasoning": {"type": "string"}
		},
		"required": ["pattern_id", "severity", "description", "recommended_action", "reasoning"]
	}
}`)

func (lr *LLMReflector) callOllama(ctx context.Context, userPrompt string) (string, error) {
	if lr.cbRegistry != nil {
		cb := lr.cbRegistry.Get("ollama-rsic-reflect")
		var result string
		err := cb.Execute(ctx, func(ctx context.Context) error {
			var innerErr error
			result, innerErr = lr.doCallOllama(ctx, userPrompt)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			return "", fmt.Errorf("ollama rsic-reflect circuit breaker open")
		}
		return result, err
	}
	return lr.doCallOllama(ctx, userPrompt)
}

func (lr *LLMReflector) doCallOllama(ctx context.Context, userPrompt string) (string, error) {
	prompt := llmReflectSystemPrompt + "\n\n" + userPrompt

	reqBody := llmReflectOllamaRequest{
		Model:   lr.cfg.Model,
		Prompt:  prompt,
		Stream:  false,
		Format:  ollamaReflectSchema,
		Options: map[string]any{"temperature": 0.3},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	endpoint := lr.cfg.OllamaURL + "/api/generate"
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

	var ollamaResp llmReflectOllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return strings.TrimSpace(ollamaResp.Response), nil
}

// --- Response parsing ---

// validSeverities constrains LLM output to known severity levels.
var validSeverities = map[string]InsightSeverity{
	"low":      SeverityLow,
	"medium":   SeverityMedium,
	"high":     SeverityHigh,
	"critical": SeverityCritical,
}

// validActions constrains LLM output to known action types.
var validActions = map[string]bool{
	"prune_decayed_edges":  true,
	"prune_excess_edges":   true,
	"tombstone_stale":      true,
	"graduate_volatile":    true,
	"trigger_consolidation": true,
	"refresh_stale_edges":  true,
}

func (lr *LLMReflector) parseResponse(raw string) ([]ReflectionInsight, error) {
	// Strip markdown code fences if present
	raw = stripLLMCodeFence(raw)
	raw = strings.TrimSpace(raw)

	var llmInsights []llmReflectInsight
	if err := json.Unmarshal([]byte(raw), &llmInsights); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM reflector: %w (raw: %.200s)", err, raw)
	}

	var insights []ReflectionInsight
	for _, li := range llmInsights {
		severity, ok := validSeverities[li.Severity]
		if !ok {
			continue // skip invalid severity
		}
		if !validActions[li.RecommendedAction] {
			continue // skip invalid action
		}
		if li.PatternID == "" || li.Description == "" {
			continue
		}
		insights = append(insights, ReflectionInsight{
			PatternID:         "llm:" + li.PatternID,
			Severity:          severity,
			Description:       li.Description,
			RecommendedAction: li.RecommendedAction,
		})
	}
	return insights, nil
}

// stripLLMCodeFence removes markdown code fences that LLMs sometimes wrap around JSON.
func stripLLMCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSuffix(after, "```")
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSuffix(after, "```")
	}
	return strings.TrimSpace(s)
}
