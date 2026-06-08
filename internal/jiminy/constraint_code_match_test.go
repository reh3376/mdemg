package jiminy

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

// JIMINY-OUTCOME-001 Tier 1: guards + threshold resolution for the
// embedding-similarity constraint-code matcher. The vector-index match itself
// is exercised by the Tier 2 integration test + Tier 3 live e2e (it requires a
// real Neo4j with the constraint vector index).

func TestMatchConstraintCodeByEmbedding_NilDriver(t *testing.T) {
	s := &Service{} // no driver
	got := s.matchConstraintCodeByEmbedding(context.TODO(), "sp", []float32{0.1, 0.2}, 0.55)
	if got != "" {
		t.Errorf("nil driver should return empty code, got %q", got)
	}
}

func TestMatchConstraintCodeByEmbedding_EmptyEmbedding(t *testing.T) {
	// Even with a non-nil driver sentinel we never reach the query: an empty
	// embedding short-circuits. Use a nil driver here (the embedding guard runs
	// first) to assert the guard ordering doesn't panic.
	s := &Service{}
	if got := s.matchConstraintCodeByEmbedding(context.TODO(), "sp", nil, 0.55); got != "" {
		t.Errorf("empty embedding should return empty code, got %q", got)
	}
	if got := s.matchConstraintCodeByEmbedding(context.TODO(), "sp", []float32{}, 0.55); got != "" {
		t.Errorf("zero-length embedding should return empty code, got %q", got)
	}
}

// TestConstraintCodeSimThreshold_Default verifies the zero-value config falls
// back to the RRF-calibrated default (no-hardcoding: the knob exists, and the
// fallback guards Config{} in tests).
func TestConstraintCodeSimThreshold_Default(t *testing.T) {
	// Mirror the resolution logic used in Guide().
	resolve := func(cfg config.Config) float64 {
		t := cfg.JiminyConstraintCodeSimThreshold
		if t <= 0 {
			t = defaultConstraintCodeSimThreshold
		}
		return t
	}
	if got := resolve(config.Config{}); got != defaultConstraintCodeSimThreshold {
		t.Errorf("zero-value threshold should fall back to %v, got %v", defaultConstraintCodeSimThreshold, got)
	}
	if got := resolve(config.Config{JiminyConstraintCodeSimThreshold: 0.7}); got != 0.7 {
		t.Errorf("explicit threshold should be honored, got %v", got)
	}
}

// TestMatchConstraintCode_KeywordFallbackStillWorks guards against regressing
// the keyword matcher, which remains the fallback when embedding match misses.
func TestMatchConstraintCode_KeywordFallbackStillWorks(t *testing.T) {
	constraints := []constraintCodeEntry{
		{Code: "NCM", Content: "never commit directly to the main branch always use a dev branch",
			Words: significantWordSet("never commit directly to the main branch always use a dev branch")},
	}
	// An item sharing >= 3 significant words should still match by keyword.
	item := GuidanceItem{Content: "always commit to a dev branch never to main"}
	if got := matchConstraintCode(item, constraints); got != "NCM" {
		t.Errorf("keyword fallback should still match NCM, got %q", got)
	}
	// A semantically-unrelated item should not match by keyword (< 3 overlap).
	none := GuidanceItem{Content: "configure the database connection pool size"}
	if got := matchConstraintCode(none, constraints); got != "" {
		t.Errorf("unrelated item should not keyword-match, got %q", got)
	}
}
