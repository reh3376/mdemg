// Package ape — llm_reflector.go implements LLM-powered RSIC reflection (Phase AR-3).
// When enabled, an LLM analyses the current assessment report and recent cycle
// history to produce insights that complement the rule-based reflector.
package ape

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mdemg/internal/sanitize"
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
	Enabled         bool
	Provider        string // "openai" or "ollama"
	Model           string
	MaxTokens       int
	TimeoutMs       int
	OpenAIKey       string
	OpenAIURL       string
	OllamaURL       string
	CompressPrompts bool // J17-PC: compress reflection prompts to reduce tokens
	// APE-PROMPT-BUDGET-001: bound the assembled user prompt so output never
	// starves the llama-server per-slot KV budget.
	PromptBudgetTokens int  // max assembled user-prompt tokens; 0 disables the guard
	HistoryCycles      int  // recent cycles included in the prompt (default applied if <=0)
	IncludeDatasets    bool // include verbose TSDB dataset fields in the prompt
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
	s = sanitize.CutRuneSafeSuffix(s, maxLen, "…")
	return s
}

// estimateTokens approximates the llama-server token count of s. Calibrated to
// the measured ape.reflect ratio (~2.3 chars/token for dense health-report JSON
// + markdown; live rows: 17456 chars → 7489 tokens). Slightly conservative so
// the budget guard stays under the per-slot KV ceiling rather than over it.
func estimateTokens(s string) int {
	return len(s) * 10 / 23
}

func (lr *LLMReflector) buildUserPrompt(report *SelfAssessmentReport) string {
	compress := lr.cfg.CompressPrompts

	// Sanitize report string fields before serialization
	sanitized := *report
	sanitized.SpaceID = sanitizeLLMInput(sanitized.SpaceID, 100)

	// APE-PROMPT-BUDGET-001: gate the verbose TSDB dataset fields. They dominate
	// the Current Assessment (~3895 of 7489 prompt tokens live) but are rarely
	// referenced by real pattern-detection; excluded by default, opt-in via
	// RSIC_LLM_REFLECT_INCLUDE_DATASETS. The scalar health/edge/orphan metrics
	// the detectors actually use are always kept.
	if !lr.cfg.IncludeDatasets {
		sanitized.LLMPerformance = nil
		sanitized.RetrievalDataset = nil
		sanitized.EmbeddingDataset = nil
		sanitized.TrainingReadiness = nil
		sanitized.ProductionDrift = nil // DRIFT-TRIGGER-001
	}

	// Current assessment — compact JSON saves ~40% tokens vs indented
	var ab strings.Builder
	ab.WriteString("## Current Assessment\n")
	if compress {
		ab.WriteString(encoding.CompactJSON(&sanitized))
	} else {
		reportJSON, _ := json.MarshalIndent(&sanitized, "", "  ")
		ab.Write(reportJSON)
	}
	assessmentSection := ab.String()

	// Recent cycle history (most-recent-first; count is config-bounded).
	historyCycles := lr.cfg.HistoryCycles
	if historyCycles <= 0 {
		historyCycles = 5 // back-compat default for direct constructors
	}
	var historyBlocks []string
	var calibrationSection string
	if lr.calibrator != nil {
		history := lr.calibrator.GetHistory(historyCycles)
		for i, h := range history {
			var hb strings.Builder
			cycleID := sanitizeLLMInput(h.CycleID, 200)
			fmt.Fprintf(&hb, "### Cycle %d: %s (tier=%s)\n", i+1, cycleID, h.Tier)
			fmt.Fprintf(&hb, "- Actions: %d executed, %d success, %d failed\n", h.ActionsExecuted, h.SuccessCount, h.FailedCount)
			fmt.Fprintf(&hb, "- Criteria met: %v\n", h.CriteriaMet)
			if len(h.MetricsBefore) > 0 {
				fmt.Fprintf(&hb, "- Metrics before: %v\n", h.MetricsBefore)
			}
			if len(h.MetricsAfter) > 0 {
				fmt.Fprintf(&hb, "- Metrics after: %v\n", h.MetricsAfter)
			}
			if len(h.CriteriaDetail) > 0 {
				detail := sanitizeLLMInput(fmt.Sprintf("%v", h.CriteriaDetail), 500)
				if compress {
					detail = encoding.TruncateAtWord(detail, 200)
				}
				fmt.Fprintf(&hb, "- Criteria detail: %s\n", detail)
			}
			hb.WriteString("\n")
			historyBlocks = append(historyBlocks, hb.String())
		}

		// Calibration confidence per action type
		calibration := lr.calibrator.GetCalibration()
		if len(calibration) > 0 {
			var cb strings.Builder
			cb.WriteString("## Calibration Confidence (action → success rate)\n")
			for action, conf := range calibration {
				action = sanitizeLLMInput(action, 100)
				fmt.Fprintf(&cb, "- %s: %.2f\n", action, conf)
			}
			calibrationSection = cb.String()
		}
	}

	assemble := func(hblocks []string) string {
		var sb strings.Builder
		sb.WriteString(assessmentSection)
		sb.WriteString("\n\n")
		if len(hblocks) > 0 {
			sb.WriteString("## Recent Cycle History\n")
			for _, b := range hblocks {
				sb.WriteString(b)
			}
		}
		sb.WriteString(calibrationSection)
		return sb.String()
	}

	out := assemble(historyBlocks)

	// APE-PROMPT-BUDGET-001 budget guard: if the assembled prompt would starve
	// output of the per-slot KV budget, drop history oldest-first, then truncate
	// the assessment tail as a last resort. Never silent — log what was dropped.
	budget := lr.cfg.PromptBudgetTokens
	if budget > 0 && estimateTokens(out) > budget {
		droppedCycles := 0
		for len(historyBlocks) > 0 && estimateTokens(out) > budget {
			historyBlocks = historyBlocks[:len(historyBlocks)-1] // drop oldest (tail)
			droppedCycles++
			out = assemble(historyBlocks)
		}
		truncatedAssessment := false
		if estimateTokens(out) > budget {
			const marker = "\n…[truncated to prompt budget]"
			maxChars := budget*23/10 - len(marker) // reserve room for the marker
			if maxChars > 0 && len(out) > maxChars {
				out = sanitize.CutRuneSafeSuffix(out, maxChars, marker)
				truncatedAssessment = true
			}
		}
		slog.Warn("ape.reflect prompt exceeded budget; trimmed to protect output headroom",
			"budget_tokens", budget,
			"dropped_history_cycles", droppedCycles,
			"truncated_assessment", truncatedAssessment,
			"final_est_tokens", estimateTokens(out))
	}

	return out
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
	// ENFORCE-AUTO-EXECUTE (2026-08-03): targeted per-code archive with
	// provenance stamp; dispatcher applies strict guards (rate limit,
	// per-code cooldown, protected-space allowlist, dry-run mode).
	"archive_constraint_by_code",
	// Recovery/review
	"flush_recovery_buffer",
	"review_nli_calibration",
	// Diagnostic actions (alert-only, not calibrated)
	"ingest_stale_spaces",
	// NOTE (RSIC-LLM-ALERT-GUARD-001): the deterministic threshold-gated alerts
	// `alert_jiminy_critical` / `alert_memory_bloat` / `alert_synergy_overlap`
	// were REMOVED from this whitelist. They are produced correctly by the
	// rule-based reflector (self_reflect.go) from real metrics; letting the LLM
	// also recommend them only let it HALLUCINATE an ungrounded CRITICAL (e.g. a
	// false "Jiminy Service Unavailable" while jiminy_healthy=true). The LLM
	// reflector's job is to surface NOVEL patterns, not duplicate the
	// deterministic alert set. deduplicateInsights enforces this as a structural
	// guard too (deterministicAlertActions) so re-adding them here stays safe.
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
