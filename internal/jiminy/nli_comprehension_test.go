package jiminy

import (
	"context"
	"testing"
)

func TestNLIComprehensionScorer_DisabledFallback(t *testing.T) {
	scorer := NewNLIComprehensionScorer("", 100, false)

	// Followed → 1.0
	score := scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", true)
	if score != 1.0 {
		t.Errorf("disabled scorer followed = %f, want 1.0", score)
	}

	// Not followed → 0.0
	score = scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", false)
	if score != 0.0 {
		t.Errorf("disabled scorer not followed = %f, want 0.0", score)
	}
}

func TestNLIComprehensionScorer_NoSidecarFallback(t *testing.T) {
	scorer := NewNLIComprehensionScorer("http://localhost:99999", 100, true)

	// Sidecar unreachable → falls back to heuristic
	score := scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", true)
	if score != 1.0 {
		t.Errorf("unreachable sidecar followed = %f, want 1.0", score)
	}
}

func TestNLIComprehensionScorer_EmptyURL(t *testing.T) {
	scorer := NewNLIComprehensionScorer("", 100, true)

	// Empty URL → fallback
	score := scorer.ScoreComprehension(context.Background(), "constraint text", "agent action", false)
	if score != 0.0 {
		t.Errorf("empty URL not followed = %f, want 0.0", score)
	}
}
