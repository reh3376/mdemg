package conversation

import (
	"context"
	"testing"
)

func TestContradictionChecker_NoEmbedding(t *testing.T) {
	// A checker with nil embedder/driver is fine for this test because
	// the method short-circuits before any I/O when embedding is empty.
	checker := NewContradictionChecker(nil, nil)
	cfg := DefaultContradictionConfig()

	obs := Observation{
		ObsID:   "obs-1",
		SpaceID: "test-space",
		Content: "some text without embedding",
		// Embedding intentionally left nil
	}

	score, err := checker.CheckContradictions(context.Background(), obs, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != 0.0 {
		t.Errorf("expected 0.0 for no-embedding observation, got %f", score)
	}
}

func TestNegationDetection_SimpleNegation(t *testing.T) {
	textA := "The system uses Redis for caching"
	textB := "The system does not use Redis for caching"

	if !detectNegation(textA, textB) {
		t.Error("expected negation to be detected between affirming and negating texts")
	}
	// Symmetric check.
	if !detectNegation(textB, textA) {
		t.Error("expected negation detection to be symmetric")
	}
}

func TestNegationDetection_NoNegation(t *testing.T) {
	textA := "The system uses Redis for caching"
	textB := "The system uses Memcached for caching"

	if detectNegation(textA, textB) {
		t.Error("expected no negation between two affirming texts with different subjects")
	}
}

func TestNegationDetection_MustNot(t *testing.T) {
	textA := "You must use environment variables for secrets"
	textB := "You must not use environment variables for secrets"

	if !detectNegation(textA, textB) {
		t.Error("expected negation detected for 'must not' marker")
	}
}

func TestNegationDetection_InsteadOf(t *testing.T) {
	textA := "Use PostgreSQL instead of MySQL for the database"
	textB := "Use MySQL for the database"

	if !detectNegation(textA, textB) {
		t.Error("expected negation detected for 'instead of' marker")
	}
}

func TestContradictionConfig_Defaults(t *testing.T) {
	cfg := DefaultContradictionConfig()

	if !cfg.Enabled {
		t.Error("expected default Enabled to be true")
	}
	if cfg.SimThreshold != 0.75 {
		t.Errorf("expected default SimThreshold 0.75, got %f", cfg.SimThreshold)
	}
	if cfg.MaxCandidates != 20 {
		t.Errorf("expected default MaxCandidates 20, got %d", cfg.MaxCandidates)
	}
}
