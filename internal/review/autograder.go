package review

// HITL-CURATION-002 E1 — Autograder is the LLM-graded confidence-marker for
// datasets managed by HITL-REVIEW-001. Its output flows into the same
// `review_grades` table as operator grades, distinguished by grader_id prefix
// ("auto:<model>@<binary>"), and — this is the load-bearing invariant —
// NEVER triggers the reinforcement side-effect on the substrate. Auto-grade
// is a confidence-marking pass over the pending queue: high-confidence
// verdicts get recorded so the human is freed to spend attention on the
// borderline minority; the reinforcement effect (mint an L0 obs, adjust a
// trust score, etc.) stays gated on operator confirmation.
//
// Contract:
//   - AutoGraderPrefix identifies auto-grade rows for both grep-testable
//     invariant checks and dashboard splits.
//   - Autograder does NOT construct the http.Request that lands the grade;
//     it produces the GradeSubmission + a confidence float, and the caller
//     is expected to POST /v1/review/grade with `reinforce: false`.
//     This keeps the invariant enforceable at ONE layer (the endpoint) rather
//     than at every consumer.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AutoGraderPrefix — a review_grades.grader_id starting with this string
// identifies an auto-grade row. Never change this prefix — the invariant
// tests and dashboards read it.
const AutoGraderPrefix = "auto:"

// LLMGrader is what an Autograder uses to talk to a model. Kept minimal so
// the caller can wire llmclient.Client, a stub, or a teacher-model client
// without pulling llmclient into internal/review.
type LLMGrader interface {
	CompleteJSON(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error)
}

// Autograder produces confidence-gated GradeSubmissions from ReviewItems by
// prompting an LLM against the dataset's rubric. The Autograder itself is
// stateless; per-call state (item, rubric) is captured in Grade().
type Autograder struct {
	llm            LLMGrader
	modelID        string
	binarySHA      string
	maxTokens      int
	minConfidence  float64
}

// AutograderConfig configures a new Autograder.
type AutograderConfig struct {
	LLM           LLMGrader // required
	ModelID       string    // e.g. "mdemg-llm-v1"; embedded in grader_id
	BinarySHA     string    // short commit sha of the mdemg binary; embedded in grader_id
	MaxTokens     int       // upper bound on the grader's JSON output; 0 → 1000
	MinConfidence float64   // grade is returned only when >= this; 0 → 0.80
}

// NewAutograder constructs an Autograder. Defaults are applied for any zero-value
// numeric field. LLM is required; nil returns nil (caller-friendly).
func NewAutograder(cfg AutograderConfig) *Autograder {
	if cfg.LLM == nil {
		return nil
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 1000
	}
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.80
	}
	if cfg.ModelID == "" {
		cfg.ModelID = "unknown"
	}
	if cfg.BinarySHA == "" {
		cfg.BinarySHA = "dev"
	}
	return &Autograder{
		llm:           cfg.LLM,
		modelID:       cfg.ModelID,
		binarySHA:     cfg.BinarySHA,
		maxTokens:     cfg.MaxTokens,
		minConfidence: cfg.MinConfidence,
	}
}

// GraderID returns the review_grades.grader_id this Autograder writes.
// Grep-testable: begins with AutoGraderPrefix.
func (a *Autograder) GraderID() string {
	return AutoGraderPrefix + a.modelID + "@" + a.binarySHA
}

// GradeResult carries the model's verdict + confidence + rationale. The caller
// inspects Confidence >= MinConfidence to decide whether to persist as a grade.
type GradeResult struct {
	Submission  GradeSubmission
	Confidence  float64
	Rationale   string
}

// AutogradePromptHinter is optionally implemented by datasets to supply
// dataset-specific anti-pattern language spliced into the autograder's system
// prompt. Rubric anchors describe the SCORING scale; the hint describes what
// the item SHOULD look like (durable rule vs completed-action log vs
// session-noise). The initial dry-run on contradicted_drafts on 2026-07-27
// showed the anchors alone weren't enough: the model labeled a
// "DATAPRUNE EXECUTED: deleted 1898 rows" log line as durable_rule=4/conf=0.90
// — high-confidence + structurally-wrong. Adding this hint fixes that class.
//
// A dataset that does not implement AutogradePromptHinter gets the generic
// autograde prompt (rubric anchors only) — no behavior change.
type AutogradePromptHinter interface {
	AutogradePromptHint() string
}

// Grade produces an auto-grade for one item against a dataset's rubric.
// Returns (result, true) when the model returned a well-formed verdict at or
// above MinConfidence; (result, false) when it returned a well-formed verdict
// BELOW MinConfidence (the caller should leave the item pending); error when
// the model output was unparseable or the LLM call failed.
//
// datasetID identifies the source dataset for the GradeSubmission; ReviewItem
// itself is dataset-agnostic in the platform, so the caller passes it in.
// datasetHint (optional) is a dataset-specific anti-pattern hint spliced into
// the system prompt; caller passes "" to skip.
//
// The Autograder DOES NOT call the /v1/review/grade endpoint. It produces the
// GradeSubmission; the caller decides whether to POST it (and with what
// reinforce flag — the invariant is that all Autograder-produced POSTs use
// reinforce:false).
func (a *Autograder) Grade(ctx context.Context, datasetID string, item ReviewItem, rubric Rubric) (GradeResult, bool, error) {
	return a.GradeWithHint(ctx, datasetID, item, rubric, "")
}

// GradeWithHint is Grade with a dataset-specific anti-pattern hint.
func (a *Autograder) GradeWithHint(ctx context.Context, datasetID string, item ReviewItem, rubric Rubric, datasetHint string) (GradeResult, bool, error) {
	if rubric.Kind != RubricRated {
		// Ranked (DPO) autograde is a HITL-REVIEW-002 concern — different prompt
		// shape (chosen/rejected/confidence). Reject explicitly.
		return GradeResult{}, false, fmt.Errorf("autograde: only rated rubrics supported, got %s", rubric.Kind)
	}
	if len(rubric.Dimensions) == 0 {
		return GradeResult{}, false, fmt.Errorf("autograde: rubric %q has no dimensions", rubric.Version)
	}

	sys := buildAutogradeSystemPrompt(rubric, datasetHint)
	usr := buildAutogradeUserPrompt(item)

	raw, err := a.llm.CompleteJSON(ctx, sys, usr, a.maxTokens)
	if err != nil {
		return GradeResult{}, false, fmt.Errorf("autograde: llm call: %w", err)
	}

	parsed, err := parseAutogradeResponse(raw, rubric)
	if err != nil {
		return GradeResult{}, false, fmt.Errorf("autograde: parse response: %w (raw=%q)", err, truncForErr(raw, 200))
	}

	res := GradeResult{
		Submission: GradeSubmission{
			DatasetID:  datasetID,
			ItemID:     item.ItemID,
			Dimensions: parsed.Dimensions,
		},
		Confidence: parsed.Confidence,
		Rationale:  parsed.Rationale,
	}
	return res, res.Confidence >= a.minConfidence, nil
}

// buildAutogradeSystemPrompt renders the rubric into an LLM-legible grading
// instruction. Dimensions carry their WRITTEN anchors — the model grades
// against the same anchored standard the human would. datasetHint (optional)
// is dataset-specific anti-pattern language spliced BEFORE the rubric — it
// teaches the model what an item OF THE RIGHT SHAPE looks like, which the
// rubric anchors alone can't (they describe scoring, not typology).
func buildAutogradeSystemPrompt(rubric Rubric, datasetHint string) string {
	var b strings.Builder
	b.WriteString("You are an automated grader for a human-in-the-loop review pipeline. " +
		"Your job is to assign each rubric dimension a 0-4 score and report your CONFIDENCE " +
		"in that verdict (0.0-1.0). If you are not confident, say so — the operator will " +
		"grade low-confidence items themselves.\n\n")
	if datasetHint != "" {
		b.WriteString("Dataset-specific guidance:\n")
		b.WriteString(datasetHint)
		if !strings.HasSuffix(datasetHint, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Rubric: " + rubric.Version + "\n\n")
	for _, dim := range rubric.Dimensions {
		b.WriteString("Dimension \"" + dim.Key + "\":\n")
		for level, anchor := range dim.Anchors {
			fmt.Fprintf(&b, "  %d = %s\n", level, anchor)
		}
		b.WriteString("\n")
	}
	b.WriteString("Return STRICTLY valid JSON with this shape (no prose, no markdown):\n")
	b.WriteString("{\n")
	b.WriteString("  \"dimensions\": { <key>: <int 0..4>, ... },  // all rubric dimensions required\n")
	b.WriteString("  \"confidence\": <float 0.0..1.0>,             // your certainty in the verdict\n")
	b.WriteString("  \"rationale\": \"<one sentence>\"              // why you scored this way\n")
	b.WriteString("}\n")
	return b.String()
}

// buildAutogradeUserPrompt renders the item's content + context for grading.
// The rubric's anchors already tell the model what the dimensions mean; the
// user prompt is just the material to grade.
func buildAutogradeUserPrompt(item ReviewItem) string {
	var b strings.Builder
	b.WriteString("Item to grade:\n\n")
	b.WriteString("CONTENT:\n" + item.Content + "\n\n")
	if item.Context != "" {
		b.WriteString("CONTEXT:\n" + item.Context + "\n\n")
	}
	if len(item.Meta) > 0 {
		if metaJSON, err := json.Marshal(item.Meta); err == nil {
			b.WriteString("META: " + string(metaJSON) + "\n")
		}
	}
	return b.String()
}

// autogradeResponse is the strict JSON contract we ask the model to return.
type autogradeResponse struct {
	Dimensions map[string]int `json:"dimensions"`
	Confidence float64        `json:"confidence"`
	Rationale  string         `json:"rationale"`
}

// parseAutogradeResponse enforces the JSON contract + rubric-shape validity.
// The rubric's Score() will re-validate at grade-write time, but we reject
// here so the caller gets a clear error at the grader boundary.
func parseAutogradeResponse(raw string, rubric Rubric) (autogradeResponse, error) {
	// Some models wrap JSON in markdown fences. Strip a leading/trailing fence.
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var r autogradeResponse
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return r, fmt.Errorf("invalid json: %w", err)
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return r, fmt.Errorf("confidence %v out of range [0,1]", r.Confidence)
	}
	if r.Dimensions == nil {
		return r, fmt.Errorf("missing dimensions map")
	}
	// Every rubric dimension must be present and in [0,4].
	for _, dim := range rubric.Dimensions {
		v, ok := r.Dimensions[dim.Key]
		if !ok {
			return r, fmt.Errorf("missing dimension %q", dim.Key)
		}
		if v < 0 || v > 4 {
			return r, fmt.Errorf("dimension %q score %d out of range [0,4]", dim.Key, v)
		}
	}
	return r, nil
}

// truncForErr keeps an error's raw-payload context readable without dumping
// megabytes into a log. Byte-safe (never splits a rune).
func truncForErr(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	// Back off to a rune boundary — matches sanitize.CutRuneSafe semantics
	// without pulling in a cross-package dep for one helper.
	cut := maxBytes
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut] + "..."
}
