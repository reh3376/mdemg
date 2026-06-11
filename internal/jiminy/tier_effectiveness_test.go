package jiminy

import "testing"

func TestGradeTierEffectiveness_SingleTier(t *testing.T) {
	snapshot := &ProtocolMetrics{
		TierComprehension: [3]float64{0.9, 0, 0},
		TierOutcomeCount:  [3]int64{10, 0, 0},
		TierCodeComprehension: map[int]map[string]float64{
			1: {"no-force-push": 0.9},
		},
	}
	report := GradeTierEffectiveness(snapshot, 5, 0.6)
	if report != nil {
		t.Error("expected nil report when only one tier has data")
	}
}

func TestGradeTierEffectiveness_CrossTierDelta(t *testing.T) {
	snapshot := &ProtocolMetrics{
		TierComprehension: [3]float64{0.5, 0.9, 0},
		TierOutcomeCount:  [3]int64{10, 10, 0},
		TierCodeComprehension: map[int]map[string]float64{
			1: {"no-force-push": 0.5},
			2: {"no-force-push": 0.9},
		},
	}
	report := GradeTierEffectiveness(snapshot, 5, 0.6)
	if report == nil {
		t.Fatal("expected non-nil report with cross-tier data")
	}
	if len(report.CodeTierDelta) == 0 {
		t.Fatal("expected at least one code tier delta entry")
	}
	delta := report.CodeTierDelta[0]
	if delta.BestTier != 2 {
		t.Errorf("best tier = %d, want 2", delta.BestTier)
	}
	if delta.WorstTier != 1 {
		t.Errorf("worst tier = %d, want 1", delta.WorstTier)
	}
	if delta.DeltaPercent < 39 || delta.DeltaPercent > 41 {
		t.Errorf("delta percent = %.1f, want ~40", delta.DeltaPercent)
	}
}

func TestGradeTierEffectiveness_MinSamples(t *testing.T) {
	snapshot := &ProtocolMetrics{
		TierComprehension: [3]float64{0.5, 0.9, 0},
		TierOutcomeCount:  [3]int64{2, 2, 0}, // below min samples
		TierCodeComprehension: map[int]map[string]float64{
			1: {"code-a": 0.5},
			2: {"code-a": 0.9},
		},
	}
	report := GradeTierEffectiveness(snapshot, 5, 0.6)
	if report != nil {
		t.Error("expected nil report when samples below minimum")
	}
}

func TestGradeTierEffectiveness_IneffectiveTiers(t *testing.T) {
	snapshot := &ProtocolMetrics{
		TierComprehension: [3]float64{0.4, 0.85, 0},
		TierOutcomeCount:  [3]int64{10, 10, 0},
		TierCodeComprehension: map[int]map[string]float64{
			1: {"code-a": 0.4},
			2: {"code-a": 0.85},
		},
	}
	report := GradeTierEffectiveness(snapshot, 5, 0.6)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.IneffectiveTiers) == 0 {
		t.Fatal("expected at least one ineffective tier entry")
	}
	entry := report.IneffectiveTiers[0]
	if entry.Tier != 1 {
		t.Errorf("ineffective tier = %d, want 1", entry.Tier)
	}
	if entry.BetterTier != 2 {
		t.Errorf("better tier = %d, want 2", entry.BetterTier)
	}
}

func TestGradeTierEffectiveness_NilOnInsufficientData(t *testing.T) {
	report := GradeTierEffectiveness(nil, 5, 0.6)
	if report != nil {
		t.Error("expected nil for nil snapshot")
	}

	empty := &ProtocolMetrics{}
	report = GradeTierEffectiveness(empty, 5, 0.6)
	if report != nil {
		t.Error("expected nil for empty snapshot")
	}
}
