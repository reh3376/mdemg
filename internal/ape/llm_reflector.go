// Package ape — llm_reflector.go implements LLM-powered RSIC reflection (Phase AR-3).
// When enabled, an LLM analyses the current assessment report and recent cycle
// history to produce insights that complement the rule-based reflector.
package ape

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/encoding"
	"mdemg/internal/llmclient"
	"mdemg/internal/metrics"
)

// LLMReflectorConfig holds configuration for the LLM reflector.
type LLMReflectorConfig struct {
	Enabled        bool
	Provider       string // "openai" or "ollama"
	Model          string
	MaxTokens      int
	TimeoutMs      int
	OpenAIKey      string
	OpenAIURL      string
	OllamaURL      string
	CompressPrompts bool // J17-PC: compress reflection prompts to reduce tokens
}

// LLMReflector sends assessment data and cycle history to an LLM for pattern detection.
// Follows the EmergenceNamer pattern (OpenAI/Ollama, circuit breaker, JSON-constrained output).
type LLMReflector struct {
	cfg        LLMReflectorConfig
	cbRegistry *circuitbreaker.Registry
	calibrator *Calibrator
	llm        *llmclient.Client
}

// NewLLMReflector creates a new LLM-powered reflector.
func NewLLMReflector(cfg LLMReflectorConfig, cbRegistry *circuitbreaker.Registry, calibrator *Calibrator) *LLMReflector {
	baseURL := cfg.OpenAIURL
	apiKey := cfg.OpenAIKey
	if cfg.Provider == "ollama" {
		baseURL = cfg.OllamaURL
	}

	return &LLMReflector{
		cfg:        cfg,
		cbRegistry: cbRegistry,
		calibrator: calibrator,
		llm: llmclient.New(llmclient.Config{
			Provider:  cfg.Provider,
			Model:     cfg.Model,
			APIKey:    apiKey,
			BaseURL:   baseURL,
			TimeoutMs: cfg.TimeoutMs,
		}).WithContext("ape.reflect", ""),
	}
}

// llmReflectSystemPromptTemplate is formatted at init with the action list.
var llmReflectSystemPrompt string

func init() {
	quoted := make([]string, len(AllowedLLMActions))
	for i, a := range AllowedLLMActions {
		quoted[i] = `"` + a + `"`
	}
	actionEnum := strings.Join(quoted, ", ")

	llmReflectSystemPrompt = `You are an RSIC (Recursive Self-Improvement Cycle) reflection engine for a knowledge graph memory system.

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
- recommended_action MUST be one of: ` + actionEnum + `
- Output ONLY valid JSON — no markdown, no preamble

[{"pattern_id": "...", "severity": "...", "description": "...", "recommended_action": "...", "reasoning": "..."}]`
}

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

	userPrompt := lr.buildUserPrompt(report)

	msgs := []llmclient.Message{
		{Role: "system", Content: llmReflectSystemPrompt},
		{Role: "user", Content: userPrompt},
	}

	maxTokens := lr.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2000
	}
	if maxTokens < 2000 {
		maxTokens = 2000
	}

	opts := llmclient.CompleteOpts{MaxTokens: maxTokens}
	if lr.cfg.Provider == "ollama" {
		opts.Format = ollamaReflectSchema
		opts.Options = map[string]any{"temperature": 0.3}
	}

	cbName := "openai-rsic-reflect"
	if lr.cfg.Provider == "ollama" {
		cbName = "ollama-rsic-reflect"
	}

	var raw string
	var err error

	if lr.cbRegistry != nil {
		cb := lr.cbRegistry.Get(cbName)
		err = cb.Execute(timeoutCtx, func(ctx context.Context) error {
			var innerErr error
			raw, innerErr = lr.llm.Complete(ctx, msgs, opts)
			return innerErr
		})
		if err == circuitbreaker.ErrCircuitOpen {
			metrics.Metrics().RSICActionTotal("llm_reflect", "circuit_open").Inc()
			return nil, fmt.Errorf("%s circuit breaker open", cbName)
		}
	} else {
		raw, err = lr.llm.Complete(timeoutCtx, msgs, opts)
	}

	if err != nil {
		return nil, fmt.Errorf("llm reflector: %w", err)
	}

	return lr.parseResponse(raw)
}

// sanitizeLLMInput strips prompt injection markers, control characters, and
// truncates overly long strings before they are interpolated into the LLM prompt.
func sanitizeLLMInput(s string, maxLen int) string {
	// Strip known prompt injection markers
	injectionPatterns := []string{
		"<|system|>", "<|user|>", "<|assistant|>", "<|im_start|>", "<|im_end|>",
		"### Instructions", "### System", "[INST]", "[/INST]",
		"<s>", "</s>", "<<SYS>>", "<</SYS>>",
	}
	for _, p := range injectionPatterns {
		s = strings.ReplaceAll(s, p, "")
	}

	// Strip control characters (keep newlines, tabs, and printable)
	var clean strings.Builder
	clean.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\t' || (unicode.IsPrint(r) && !unicode.IsControl(r)) {
			clean.WriteRune(r)
		}
	}
	s = clean.String()

	// Truncate
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}

func (lr *LLMReflector) buildUserPrompt(report *SelfAssessmentReport) string {
	var sb strings.Builder
	compress := lr.cfg.CompressPrompts

	// Sanitize report string fields before serialization
	sanitized := *report
	sanitized.SpaceID = sanitizeLLMInput(sanitized.SpaceID, 100)

	// Current assessment — compact JSON saves ~40% tokens vs indented
	sb.WriteString("## Current Assessment\n")
	if compress {
		sb.WriteString(encoding.CompactJSON(&sanitized))
	} else {
		reportJSON, _ := json.MarshalIndent(&sanitized, "", "  ")
		sb.Write(reportJSON)
	}
	sb.WriteString("\n\n")

	// Recent cycle history (last 5)
	if lr.calibrator != nil {
		history := lr.calibrator.GetHistory(5)
		if len(history) > 0 {
			sb.WriteString("## Recent Cycle History\n")
			for i, h := range history {
				cycleID := sanitizeLLMInput(h.CycleID, 200)
				fmt.Fprintf(&sb, "### Cycle %d: %s (tier=%s)\n", i+1, cycleID, h.Tier)
				fmt.Fprintf(&sb, "- Actions: %d executed, %d success, %d failed\n", h.ActionsExecuted, h.SuccessCount, h.FailedCount)
				fmt.Fprintf(&sb, "- Criteria met: %v\n", h.CriteriaMet)
				if len(h.MetricsBefore) > 0 {
					fmt.Fprintf(&sb, "- Metrics before: %v\n", h.MetricsBefore)
				}
				if len(h.MetricsAfter) > 0 {
					fmt.Fprintf(&sb, "- Metrics after: %v\n", h.MetricsAfter)
				}
				if len(h.CriteriaDetail) > 0 {
					detail := sanitizeLLMInput(fmt.Sprintf("%v", h.CriteriaDetail), 500)
					if compress {
						detail = encoding.TruncateAtWord(detail, 200)
					}
					fmt.Fprintf(&sb, "- Criteria detail: %s\n", detail)
				}
				sb.WriteString("\n")
			}
		}

		// Calibration confidence per action type
		calibration := lr.calibrator.GetCalibration()
		if len(calibration) > 0 {
			sb.WriteString("## Calibration Confidence (action → success rate)\n")
			for action, conf := range calibration {
				action = sanitizeLLMInput(action, 100)
				fmt.Fprintf(&sb, "- %s: %.2f\n", action, conf)
			}
		}
	}

	return sb.String()
}

// ollamaReflectSchema is the JSON schema for grammar-constrained output (Ollama v0.5+).
// Generated from AllowedLLMActions at init time.
var ollamaReflectSchema json.RawMessage

func init() {
	quoted := make([]string, len(AllowedLLMActions))
	for i, a := range AllowedLLMActions {
		quoted[i] = `"` + a + `"`
	}
	actionEnum := strings.Join(quoted, ",")

	ollamaReflectSchema = json.RawMessage(`{
	"type": "array",
	"items": {
		"type": "object",
		"properties": {
			"pattern_id": {"type": "string"},
			"severity": {"type": "string", "enum": ["low","medium","high","critical"]},
			"description": {"type": "string"},
			"recommended_action": {"type": "string", "enum": [` + actionEnum + `]},
			"reasoning": {"type": "string"}
		},
		"required": ["pattern_id", "severity", "description", "recommended_action", "reasoning"]
	}
}`)
}

// --- Response parsing ---

// validSeverities constrains LLM output to known severity levels.
var validSeverities = map[string]InsightSeverity{
	"low":      SeverityLow,
	"medium":   SeverityMedium,
	"high":     SeverityHigh,
	"critical": SeverityCritical,
}

// AllowedLLMActions is the single source of truth for actions the LLM reflector may
// recommend. The system prompt enum, Ollama JSON schema enum, and validation map
// are all derived from this slice. Diagnostic-only alert actions are included so the
// LLM can recommend them; they are excluded from calibration by design.
var AllowedLLMActions = []string{
	// Graph mutation actions
	"prune_decayed_edges",
	"prune_excess_edges",
	"tombstone_stale",
	"graduate_volatile",
	"trigger_consolidation",
	"refresh_stale_edges",
	// Constraint/code management
	"codify_constraint",
	"codify_all_constraints",
	"retire_code",
	// Tuning actions
	"adjust_tier_threshold",
	"adjust_replay_buffer",
	"review_guidance_effectiveness",
	"adjust_guidance_confidence",
	"archive_ineffective_constraints",
	// Recovery/review
	"flush_recovery_buffer",
	"review_nli_calibration",
	// Diagnostic actions (alert-only, not calibrated)
	"ingest_stale_spaces",
	"alert_jiminy_critical",
	"alert_memory_bloat",
	"alert_synergy_overlap",
}

// validActions is derived from AllowedLLMActions at init time.
var validActions map[string]bool

func init() {
	validActions = make(map[string]bool, len(AllowedLLMActions))
	for _, a := range AllowedLLMActions {
		validActions[a] = true
	}
}

func (lr *LLMReflector) parseResponse(raw string) ([]ReflectionInsight, error) {
	raw = llmclient.SanitizeResponse(raw)

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

