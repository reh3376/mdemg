package jiminy

import (
	"math"
	"testing"
	"time"
)

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestTrustScorer_InitialScore(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{Initial: 0.5})

	score := ts.GetScore("new-session")
	if score != 0.5 {
		t.Errorf("initial score = %f, want 0.5", score)
	}
}

func TestTrustScorer_FollowedBoosts(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:        0.5,
		BoostPerFollow: 0.1,
	})

	ts.RecordOutcome("sess-1", OutcomeFollowed)
	score := ts.GetScore("sess-1")
	if !approxEqual(score, 0.6) {
		t.Errorf("score after follow = %f, want 0.6", score)
	}

	// Multiple follows
	ts.RecordOutcome("sess-1", OutcomeFollowed)
	ts.RecordOutcome("sess-1", OutcomeFollowed)
	score = ts.GetScore("sess-1")
	if !approxEqual(score, 0.8) {
		t.Errorf("score after 3 follows = %f, want 0.8", score)
	}
}

func TestTrustScorer_IgnoreDecays(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:      0.5,
		DecayPerIgnore: 0.1,
	})

	ts.RecordOutcome("sess-1", OutcomeIgnored)
	score := ts.GetScore("sess-1")
	if !approxEqual(score, 0.4) {
		t.Errorf("score after ignore = %f, want 0.4", score)
	}
}

func TestTrustScorer_ContradictDecaysMore(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:            0.5,
		DecayPerIgnore:     0.03,
		DecayPerContradict: 0.05,
	})

	ts.RecordOutcome("sess-1", OutcomeContradicted)
	score := ts.GetScore("sess-1")
	if !approxEqual(score, 0.45) {
		t.Errorf("score after contradict = %f, want 0.45", score)
	}
}

func TestTrustScorer_ClampBounds(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:        0.9,
		BoostPerFollow: 0.2,
	})

	// Should clamp to 1.0
	ts.RecordOutcome("sess-1", OutcomeFollowed)
	score := ts.GetScore("sess-1")
	if score != 1.0 {
		t.Errorf("score should clamp to 1.0, got %f", score)
	}

	// Should clamp to 0.0
	ts2 := NewTrustScorer(TrustConfig{
		Initial:      0.05,
		DecayPerIgnore: 0.1,
	})
	ts2.RecordOutcome("sess-2", OutcomeIgnored)
	score = ts2.GetScore("sess-2")
	if score != 0.0 {
		t.Errorf("score should clamp to 0.0, got %f", score)
	}
}

func TestTrustScorer_SetScore(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{Initial: 0.5})

	ts.SetScore("restored-session", 0.75)
	score := ts.GetScore("restored-session")
	if score != 0.75 {
		t.Errorf("SetScore(0.75) → GetScore = %f, want 0.75", score)
	}
}

func TestTrustScorer_SessionIsolation(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:        0.5,
		BoostPerFollow: 0.1,
	})

	ts.RecordOutcome("sess-1", OutcomeFollowed)
	ts.RecordOutcome("sess-1", OutcomeFollowed)

	score1 := ts.GetScore("sess-1")
	score2 := ts.GetScore("sess-2")

	if !approxEqual(score1, 0.7) {
		t.Errorf("sess-1 score = %f, want 0.7", score1)
	}
	if !approxEqual(score2, 0.5) {
		t.Errorf("sess-2 score = %f, want 0.5 (initial)", score2)
	}
}

func TestTrustScorer_PerSessionIndependence(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:            0.5,
		BoostPerFollow:     0.1,
		DecayPerIgnore:     0.1,
		DecayPerContradict: 0.15,
	})

	// Boost session-A 3 times
	ts.RecordOutcome("session-A", OutcomeFollowed)
	ts.RecordOutcome("session-A", OutcomeFollowed)
	ts.RecordOutcome("session-A", OutcomeFollowed)

	// Decay session-B once via contradict
	ts.RecordOutcome("session-B", OutcomeContradicted)

	scoreA := ts.GetScore("session-A")
	scoreB := ts.GetScore("session-B")

	if scoreA <= 0.5 {
		t.Errorf("session-A score = %f, want > 0.5 after 3 follows", scoreA)
	}
	if scoreB >= 0.5 {
		t.Errorf("session-B score = %f, want < 0.5 after contradict", scoreB)
	}
}

func TestTrustScorer_ConfigurableTTL(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:        0.5,
		BoostPerFollow: 0.1,
		TTL:            50 * time.Millisecond,
	})

	ts.RecordOutcome("sess-ttl", OutcomeFollowed)
	score := ts.GetScore("sess-ttl")
	if !approxEqual(score, 0.6) {
		t.Errorf("score before TTL = %f, want 0.6", score)
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	score = ts.GetScore("sess-ttl")
	if !approxEqual(score, 0.5) {
		t.Errorf("score after TTL = %f, want 0.5 (initial)", score)
	}
}

func TestTrustScorer_SixFollowsReachHighThreshold(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:        0.5,
		BoostPerFollow: 0.05,
		HighThreshold:  0.8,
	})

	for i := 0; i < 6; i++ {
		ts.RecordOutcome("sess-6", OutcomeFollowed)
	}
	score := ts.GetScore("sess-6")
	// 0.5 + 6*0.05 = 0.80
	if !approxEqual(score, 0.80) {
		t.Errorf("score after 6 follows = %f, want 0.80", score)
	}
	if score < ts.HighThreshold() {
		t.Errorf("score %f should be >= high threshold %f", score, ts.HighThreshold())
	}
}

func TestTrustScorer_SetThresholds(t *testing.T) {
	ts := NewTrustScorer(TrustConfig{
		Initial:       0.5,
		HighThreshold: 0.8,
		LowThreshold:  0.4,
	})

	// Valid thresholds
	ts.SetThresholds(0.7, 0.3)
	if ts.HighThreshold() != 0.7 {
		t.Errorf("high = %f, want 0.7", ts.HighThreshold())
	}
	if ts.LowThreshold() != 0.3 {
		t.Errorf("low = %f, want 0.3", ts.LowThreshold())
	}

	// Invalid: low >= high — should not update
	ts.SetThresholds(0.5, 0.5)
	if ts.HighThreshold() != 0.7 {
		t.Errorf("high should remain 0.7 after invalid set, got %f", ts.HighThreshold())
	}

	// Invalid: high > 1.0
	ts.SetThresholds(1.1, 0.2)
	if ts.HighThreshold() != 0.7 {
		t.Errorf("high should remain 0.7 after out-of-range set, got %f", ts.HighThreshold())
	}
}
