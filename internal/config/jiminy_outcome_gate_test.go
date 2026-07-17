package config

import (
	"strings"
	"testing"
)

// JIMINY-CORPUS-001 Epic 4: the outcome-classifier relevance gate defaults
// sanely (0.10 — half the LOW relevance floor, carving only the clearly-
// unrelated tail), is env-overridable, and Validate warns when it is
// misordered above LOW.
func TestJiminyOutcomeNotApplicableSimilarity_Default(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.JiminyOutcomeNotApplicableSim != 0.10 {
		t.Errorf("JiminyOutcomeNotApplicableSim = %v, want 0.10", cfg.JiminyOutcomeNotApplicableSim)
	}
	// Ordering invariant with defaults: gate ≤ LOW ≤ HIGH
	if cfg.JiminyOutcomeNotApplicableSim > cfg.JiminyOutcomeSimilarityLow {
		t.Errorf("default gate %v > default LOW %v — must be ≤",
			cfg.JiminyOutcomeNotApplicableSim, cfg.JiminyOutcomeSimilarityLow)
	}
}

func TestJiminyOutcomeNotApplicableSimilarity_Override(t *testing.T) {
	setMinimalEnv(t)

	t.Setenv("JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY", "0.05")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.JiminyOutcomeNotApplicableSim != 0.05 {
		t.Errorf("JiminyOutcomeNotApplicableSim = %v, want 0.05", cfg.JiminyOutcomeNotApplicableSim)
	}

	// Explicit 0 disables the gate (pre-gate behavior) — it must NOT be re-defaulted.
	t.Setenv("JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY", "0")
	cfg, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.JiminyOutcomeNotApplicableSim != 0 {
		t.Errorf("JiminyOutcomeNotApplicableSim = %v, want 0 (explicit disable)", cfg.JiminyOutcomeNotApplicableSim)
	}
}

func TestJiminyOutcomeNotApplicableSimilarity_ValidateOrdering(t *testing.T) {
	setMinimalEnv(t)
	t.Setenv("JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY", "0.30")
	t.Setenv("JIMINY_OUTCOME_SIMILARITY_LOW", "0.20")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	warnings, err := cfg.Validate()
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ordering warning for gate > LOW, got warnings: %v", warnings)
	}

	// Well-ordered config produces no gate warning.
	t.Setenv("JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY", "0.10")
	cfg, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	warnings, err = cfg.Validate()
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "JIMINY_OUTCOME_NOT_APPLICABLE_SIMILARITY") {
			t.Errorf("unexpected gate warning for well-ordered config: %s", w)
		}
	}
}
