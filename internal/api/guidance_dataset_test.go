package api

import (
	"strings"
	"testing"

	"mdemg/internal/review"
)

// HITL-CURATION-003 E2 pin: guidanceDataset implements AutogradePromptHinter
// so the autograder can splice the per-dataset typology hint into its system
// prompt. Without this, the generic LLM prompt gives noisy grades because the
// rubric axis `outcome_label_correctness` is about the CLASSIFIER being right,
// not the guidance itself.
func TestGuidanceDataset_AutogradePromptHint_NonemptyAndCoversTypology(t *testing.T) {
	var _ review.AutogradePromptHinter = (*guidanceDataset)(nil) // interface assertion

	d := &guidanceDataset{}
	hint := d.AutogradePromptHint()
	if len(hint) < 200 {
		t.Fatalf("hint should be substantive (>=200 chars), got %d chars", len(hint))
	}

	// Must name the rubric axis being scored — otherwise the autograder
	// doesn't know what to grade.
	if !strings.Contains(hint, "outcome_label_correctness") {
		t.Error("hint must name the outcome_label_correctness rubric dim")
	}

	// Must teach the outcome_type enum values so the model knows what
	// AutoLabel means.
	for _, label := range []string{"followed", "ignored", "contradicted", "partial_compliance"} {
		if !strings.Contains(hint, label) {
			t.Errorf("hint must name outcome_type enum value %q so autograder can reason about it", label)
		}
	}

	// Must clarify the not_applicable filter behavior (rows never appear in
	// the dataset) so the model doesn't score against a missing category.
	if !strings.Contains(hint, "not_applicable") {
		t.Error("hint should mention not_applicable filter behavior")
	}

	// Must distinguish actionable vs abstract guidance_type strata so the
	// model knows when abstract guidance can be legitimately "not applied."
	for _, stratum := range []string{"constraint", "correction", "pattern", "learning"} {
		if !strings.Contains(hint, stratum) {
			t.Errorf("hint must name guidance_type stratum %q", stratum)
		}
	}
}
