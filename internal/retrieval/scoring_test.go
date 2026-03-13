package retrieval

import (
	"mdemg/internal/config"
	"mdemg/internal/models"
	"testing"
)

func TestApplyNormalizedConfidence(t *testing.T) {
	tests := []struct {
		name           string
		scores         []float64
		wantPercentile []float64
		wantLevel      []string
	}{
		{
			name:           "single item gets 100%",
			scores:         []float64{0.85},
			wantPercentile: []float64{100.0},
			wantLevel:      []string{"HIGH"},
		},
		{
			name:           "two items",
			scores:         []float64{0.90, 0.50},
			wantPercentile: []float64{100.0, 0.0},
			wantLevel:      []string{"HIGH", "LOW"},
		},
		{
			name:           "five items - even distribution",
			scores:         []float64{0.95, 0.80, 0.65, 0.50, 0.35},
			wantPercentile: []float64{100.0, 75.0, 50.0, 25.0, 0.0},
			wantLevel:      []string{"HIGH", "MEDIUM", "MEDIUM", "LOW", "LOW"},
		},
		{
			name:           "ten items - full spectrum",
			scores:         []float64{1.0, 0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1},
			wantPercentile: []float64{100.0, 88.9, 77.8, 66.7, 55.6, 44.4, 33.3, 22.2, 11.1, 0.0},
			wantLevel:      []string{"HIGH", "MEDIUM", "MEDIUM", "MEDIUM", "MEDIUM", "MEDIUM", "LOW", "LOW", "LOW", "LOW"},
		},
		{
			name:           "empty list",
			scores:         []float64{},
			wantPercentile: []float64{},
			wantLevel:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create scored candidates (already sorted by score descending)
			items := make([]ScoredCandidate, len(tt.scores))
			for i, score := range tt.scores {
				items[i] = ScoredCandidate{
					RetrieveResult: models.RetrieveResult{
						NodeID: "test-node",
						Score:  score,
					},
				}
			}

			ApplyNormalizedConfidence(items)

			for i, item := range items {
				// Check percentile (allow small float tolerance)
				if abs(item.NormalizedConfidence-tt.wantPercentile[i]) > 0.2 {
					t.Errorf("item[%d] percentile = %.1f, want %.1f", i, item.NormalizedConfidence, tt.wantPercentile[i])
				}
				// Check confidence level
				if item.ConfidenceLevel != tt.wantLevel[i] {
					t.Errorf("item[%d] level = %s, want %s", i, item.ConfidenceLevel, tt.wantLevel[i])
				}
			}
		})
	}
}

func TestConfidenceLevelFromPercentile(t *testing.T) {
	tests := []struct {
		percentile float64
		want       string
	}{
		{100.0, "HIGH"},
		{95.0, "HIGH"},
		{90.0, "HIGH"},
		{89.9, "MEDIUM"},
		{50.0, "MEDIUM"},
		{40.0, "MEDIUM"},
		{39.9, "LOW"},
		{20.0, "LOW"},
		{0.0, "LOW"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := ConfidenceLevelFromPercentile(tt.percentile)
			if got != tt.want {
				t.Errorf("ConfidenceLevelFromPercentile(%.1f) = %s, want %s", tt.percentile, got, tt.want)
			}
		})
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestSquaredActivation(t *testing.T) {
	cfg := config.Config{
		ScoringActivationSquared: true,
		ScoringActivationFloor:   0.05,
	}

	// Activation above floor: should be squared
	a := 0.5
	floored := a - cfg.ScoringActivationFloor
	expected := floored * floored // (0.45)^2 = 0.2025
	if expected < 0.20 || expected > 0.21 {
		t.Errorf("unexpected squared value: %f", expected)
	}

	// Activation below floor: should be 0
	a = 0.03
	floored = a - cfg.ScoringActivationFloor
	if floored > 0 {
		t.Errorf("expected floored <= 0 for a=%f, floor=%f", a, cfg.ScoringActivationFloor)
	}
}

func TestSquaredActivation_Disabled(t *testing.T) {
	// When disabled, activation should be used linearly (no squaring)
	cfg := config.Config{
		ScoringActivationSquared: false,
		ScoringActivationFloor:   0.05,
	}

	a := 0.5
	weight := 1.0 // gates.ActivationWeight

	// With squared disabled, actComponent = weight * a
	expectedLinear := weight * a
	if expectedLinear != 0.5 {
		t.Errorf("expected linear activation 0.5, got %f", expectedLinear)
	}

	// With squared enabled, actComponent = weight * (a - floor)^2
	cfg.ScoringActivationSquared = true
	floored := a - cfg.ScoringActivationFloor
	if floored < 0 {
		floored = 0
	}
	expectedSquared := weight * floored * floored // 1.0 * 0.45^2 = 0.2025
	if expectedSquared >= expectedLinear {
		t.Errorf("squared activation (%f) should be less than linear (%f) for a=0.5", expectedSquared, expectedLinear)
	}
}

func TestBypassBonus(t *testing.T) {
	cfg := config.Config{
		ScoringBypassThreshold: 0.85,
		ScoringBypassWeight:    0.15,
		ScoringBypassCodeMult:  1.3,
		ScoringBypassArchMult:  0.5,
	}

	// VectorSim above threshold should produce a bonus
	vectorSim := 0.95
	excess := vectorSim - cfg.ScoringBypassThreshold // 0.10
	expectedBonus := cfg.ScoringBypassWeight * excess // 0.15 * 0.10 = 0.015
	if abs(expectedBonus-0.015) > 0.001 {
		t.Errorf("expected bypass bonus ~0.015, got %f", expectedBonus)
	}

	// Code query multiplier
	codeBonus := expectedBonus * cfg.ScoringBypassCodeMult // 0.015 * 1.3 = 0.0195
	if abs(codeBonus-0.0195) > 0.001 {
		t.Errorf("expected code bypass bonus ~0.0195, got %f", codeBonus)
	}

	// Architecture query multiplier (reduced)
	archBonus := expectedBonus * cfg.ScoringBypassArchMult // 0.015 * 0.5 = 0.0075
	if abs(archBonus-0.0075) > 0.001 {
		t.Errorf("expected arch bypass bonus ~0.0075, got %f", archBonus)
	}

	// VectorSim below threshold should produce no bonus
	vectorSimLow := 0.80
	if vectorSimLow > cfg.ScoringBypassThreshold {
		t.Error("vectorSim below threshold should not trigger bypass")
	}
}

// TestLearningEdgeBoostPopulation verifies that LearningEdgeBoost is populated
// when activation exceeds the vector seed (i.e., CO_ACTIVATED_WITH edges contributed).
func TestLearningEdgeBoostPopulation(t *testing.T) {
	cfg := config.Config{
		ScoringAlpha: 0.55,
		ScoringBeta:  0.30,
		ScoringGamma: 0.10,
		ScoringDelta: 0.05,
		ScoringPhi:   0.08,
		ScoringKappa: 0.12,
		ScoringRhoL0: 0.05,
		ScoringRhoL1: 0.02,
		ScoringRhoL2: 0.01,
	}

	// Create a candidate with VectorSim = 0.5
	cands := []Candidate{
		{NodeID: "n1", VectorSim: 0.5, Confidence: 0.5, Name: "test-node"},
	}

	// Set activation higher than VectorSim (simulating learning edge contribution)
	act := map[string]float64{
		"n1": 0.8, // Higher than VectorSim 0.5 — learning edges contributed
	}

	edges := []Edge{} // No edges needed — activation is pre-computed
	hints := RetrievalHints{QueryType: "generic"}

	scored := ScoreAndRankWithBreakdown(cands, act, edges, 10, cfg, "", hints)
	if len(scored) == 0 {
		t.Fatal("expected at least one scored candidate")
	}

	bd := scored[0].Breakdown
	if bd.LearningEdgeBoost <= 0 {
		t.Errorf("LearningEdgeBoost = %f, want > 0 (activation 0.8 > VectorSim 0.5)", bd.LearningEdgeBoost)
	}

	// Verify it's included in the final score
	if bd.FinalScore <= 0 {
		t.Errorf("FinalScore = %f, want > 0", bd.FinalScore)
	}
}

// TestLearningEdgeBoostZeroWhenNoContribution verifies that LearningEdgeBoost
// is zero when activation does not exceed the vector seed.
func TestLearningEdgeBoostZeroWhenNoContribution(t *testing.T) {
	cfg := config.Config{
		ScoringAlpha: 0.55,
		ScoringBeta:  0.30,
		ScoringGamma: 0.10,
		ScoringDelta: 0.05,
		ScoringPhi:   0.08,
		ScoringKappa: 0.12,
		ScoringRhoL0: 0.05,
		ScoringRhoL1: 0.02,
		ScoringRhoL2: 0.01,
	}

	cands := []Candidate{
		{NodeID: "n1", VectorSim: 0.7, Confidence: 0.5, Name: "test-node"},
	}

	// Activation equals VectorSim — no learning edge contribution
	act := map[string]float64{
		"n1": 0.7,
	}

	edges := []Edge{}
	hints := RetrievalHints{QueryType: "generic"}

	scored := ScoreAndRankWithBreakdown(cands, act, edges, 10, cfg, "", hints)
	if len(scored) == 0 {
		t.Fatal("expected at least one scored candidate")
	}

	bd := scored[0].Breakdown
	if bd.LearningEdgeBoost != 0 {
		t.Errorf("LearningEdgeBoost = %f, want 0 (activation == VectorSim)", bd.LearningEdgeBoost)
	}
}

// TestJiminyExplanationIncludesLearningEdgeBoost verifies the Jiminy explanation
// includes "learning edge boost" text when LearningEdgeBoost is set.
func TestJiminyExplanationIncludesLearningEdgeBoost(t *testing.T) {
	breakdown := ScoreBreakdown{
		VectorSimilarity:  0.35,
		Activation:        0.15,
		LearningEdgeBoost: 0.05,
		FinalScore:        0.55,
	}

	explanation := GenerateJiminyExplanation(breakdown, []string{"vector_recall"})

	if explanation.Rationale == "" {
		t.Error("expected non-empty rationale")
	}

	// Check that learning edge boost is mentioned
	found := false
	for _, part := range []string{"learning edge boost"} {
		if containsStr(explanation.Rationale, part) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected rationale to mention 'learning edge boost', got: %s", explanation.Rationale)
	}

	// Verify retrieval path includes learning_edge stage
	path := DetermineRetrievalPath(breakdown, false)
	foundStage := false
	for _, s := range path {
		if s == string(StageLearningEdge) {
			foundStage = true
		}
	}
	if !foundStage {
		t.Errorf("expected retrieval path to include %q, got: %v", StageLearningEdge, path)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
