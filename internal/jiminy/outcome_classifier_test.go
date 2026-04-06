package jiminy

import (
	"context"
	"math"
	"testing"
)

// sequenceEmbedder returns different embeddings on successive calls to control cosine similarity.
// First call returns [1, 0, 0], second call returns [targetSim, sqrt(1-targetSim^2), 0].
// The cosine similarity between these two vectors equals targetSim.
type sequenceEmbedder struct {
	targetSim float64
	callCount int
}

func (s *sequenceEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	s.callCount++
	if s.callCount%2 == 1 {
		// Guidance embedding (first call)
		return []float32{1, 0, 0}, nil
	}
	// Action embedding (second call) — cosine similarity with [1,0,0] = targetSim
	return []float32{float32(s.targetSim), float32(math.Sqrt(1 - s.targetSim*s.targetSim)), 0}, nil
}

func (s *sequenceEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

func (s *sequenceEmbedder) Name() string       { return "test-sequence" }
func (s *sequenceEmbedder) Dimensions() int     { return 3 }

func newTestClassifier(embedder *sequenceEmbedder, llmEnabled bool) *OutcomeClassifier {
	return NewOutcomeClassifier(embedder, OutcomeClassifierConfig{
		LLMEnabled:    llmEnabled,
		HighThreshold: 0.55,
		LowThreshold:  0.20,
	})
}

func TestHeuristicFallback_PartialCompliance(t *testing.T) {
	// Similarity 0.4 is in the uncertain range (0.20-0.55) — should be partial_compliance
	emb := &sequenceEmbedder{targetSim: 0.4}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "ensure error handling uses structured logging", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "added error handling to handler.go")

	if cr.Outcome != OutcomePartialCompliance {
		t.Errorf("expected partial_compliance for similarity=0.4, got %s", cr.Outcome)
	}
	if math.Abs(cr.Confidence-0.4) > 0.05 {
		t.Errorf("expected confidence ~0.4, got %f", cr.Confidence)
	}
}

func TestHeuristicFallback_NotApplicableBelowLow(t *testing.T) {
	// Similarity 0.15 is below low threshold (0.20) — should be not_applicable (topics unrelated)
	emb := &sequenceEmbedder{targetSim: 0.15}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "use CUIDv2 for all identifiers", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "updated README documentation")

	if cr.Outcome != OutcomeNotApplicable {
		t.Errorf("expected not_applicable for similarity=0.15, got %s", cr.Outcome)
	}
	if math.Abs(cr.Confidence-0.15) > 0.05 {
		t.Errorf("expected confidence ~0.15, got %f", cr.Confidence)
	}
}

func TestHeuristicFallback_FollowedAboveHigh(t *testing.T) {
	// Similarity 0.6 is above high threshold (0.55) — should be followed
	emb := &sequenceEmbedder{targetSim: 0.6}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "add structured logging to handlers", Type: GuidancePattern}

	cr := oc.Classify(context.Background(), item, "added structured slog.Error logging to all handler return paths")

	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed for similarity=0.6, got %s", cr.Outcome)
	}
}

func TestNewThresholdDefaults(t *testing.T) {
	// Zero-value config should use the new defaults (0.55/0.20)
	oc := NewOutcomeClassifier(nil, OutcomeClassifierConfig{})

	if oc.highThreshold != 0.55 {
		t.Errorf("expected default highThreshold=0.55, got %f", oc.highThreshold)
	}
	if oc.lowThreshold != 0.20 {
		t.Errorf("expected default lowThreshold=0.20, got %f", oc.lowThreshold)
	}
}

func TestHeuristicFallback_LLMDisabled(t *testing.T) {
	// With LLM disabled, similarity 0.35 should hit heuristic fallback → partial_compliance
	emb := &sequenceEmbedder{targetSim: 0.35}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "never use UUID v4, always use CUIDv2", Type: GuidanceCorrection}

	cr := oc.Classify(context.Background(), item, "implemented identifier generation using cuid2 package")

	if cr.Outcome != OutcomePartialCompliance {
		t.Errorf("expected partial_compliance for similarity=0.35 with LLM disabled, got %s", cr.Outcome)
	}
}

func TestMapOutcomeString_NotApplicable(t *testing.T) {
	got := mapOutcomeString("not_applicable")
	if got != OutcomeNotApplicable {
		t.Errorf("expected OutcomeNotApplicable, got %s", got)
	}
	// Also test with whitespace
	got = mapOutcomeString("  Not_Applicable  ")
	if got != OutcomeNotApplicable {
		t.Errorf("expected OutcomeNotApplicable for padded input, got %s", got)
	}
}

func TestNotApplicable_VerySimilarIsStillFollowed(t *testing.T) {
	// High similarity should still be followed even with the not_applicable change
	emb := &sequenceEmbedder{targetSim: 0.7}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "always validate config weights sum to 1.0", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "added weight validation to config.go Validate()")

	if cr.Outcome != OutcomeFollowed {
		t.Errorf("expected followed for similarity=0.7, got %s", cr.Outcome)
	}
}

func TestNotApplicable_BoundaryAtLowThreshold(t *testing.T) {
	// Exactly at lowThreshold (0.20) should be in the uncertain range, NOT not_applicable
	emb := &sequenceEmbedder{targetSim: 0.20}
	oc := newTestClassifier(emb, false)
	item := GuidanceItem{Content: "use error wrapping with %w", Type: GuidanceConstraint}

	cr := oc.Classify(context.Background(), item, "wrote error handling code")

	// 0.20 is >= lowThreshold, so it enters the uncertain range → partial_compliance (LLM disabled)
	if cr.Outcome != OutcomePartialCompliance {
		t.Errorf("expected partial_compliance at boundary sim=0.20, got %s", cr.Outcome)
	}
}
