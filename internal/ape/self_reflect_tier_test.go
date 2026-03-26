package ape

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

func TestReflect_J17TierIneffective_Detected(t *testing.T) {
	cfg := config.Config{
		J17TierDriftDetectionEnabled:   true,
		J17TierEffectivenessMinSamples: 5,
		J17TierIneffectiveThreshold:    0.6,
	}
	r := NewReflector(cfg, nil)

	// Mock protocol provider that returns tier data with T1 < T2 by >15%
	r.SetProtocolProvider(&mockTierProtocolProvider{
		stats: ProtocolStatsResult{
			TierComprehension: [3]float64{0.4, 0.85, 0},
			TierOutcomeCount:  [3]int64{10, 10, 0},
			TierCodeComprehension: map[int]map[string]float64{
				1: {"code-a": 0.4},
				2: {"code-a": 0.85},
			},
		},
	})

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		OverallHealth:  0.7,
		ProtocolHealth: 0.5,
		Confidence:     1.0,
	}

	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	found := false
	for _, ins := range insights {
		if ins.PatternID == "j17_tier_ineffective" {
			found = true
			if ins.Severity != SeverityHigh {
				t.Errorf("severity = %s, want high", ins.Severity)
			}
			break
		}
	}
	if !found {
		t.Error("expected j17_tier_ineffective insight, got none")
	}
}

func TestReflect_J17TierIneffective_InsufficientSamples(t *testing.T) {
	cfg := config.Config{
		J17TierDriftDetectionEnabled:   true,
		J17TierEffectivenessMinSamples: 20, // high threshold
		J17TierIneffectiveThreshold:    0.6,
	}
	r := NewReflector(cfg, nil)
	r.SetProtocolProvider(&mockTierProtocolProvider{
		stats: ProtocolStatsResult{
			TierComprehension: [3]float64{0.4, 0.85, 0},
			TierOutcomeCount:  [3]int64{5, 5, 0}, // below min samples
			TierCodeComprehension: map[int]map[string]float64{
				1: {"code-a": 0.4},
				2: {"code-a": 0.85},
			},
		},
	})

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		OverallHealth:  0.7,
		ProtocolHealth: 0.5,
		Confidence:     1.0,
	}

	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	for _, ins := range insights {
		if ins.PatternID == "j17_tier_ineffective" {
			t.Error("should not fire with insufficient samples")
		}
	}
}

func TestReflect_J17TierIneffective_NoDrift(t *testing.T) {
	cfg := config.Config{
		J17TierDriftDetectionEnabled:   true,
		J17TierEffectivenessMinSamples: 5,
		J17TierIneffectiveThreshold:    0.6,
	}
	r := NewReflector(cfg, nil)
	r.SetProtocolProvider(&mockTierProtocolProvider{
		stats: ProtocolStatsResult{
			TierComprehension: [3]float64{0.85, 0.85, 0},
			TierOutcomeCount:  [3]int64{10, 10, 0},
			TierCodeComprehension: map[int]map[string]float64{
				1: {"code-a": 0.85},
				2: {"code-a": 0.85},
			},
		},
	})

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		OverallHealth:  0.7,
		ProtocolHealth: 0.5,
		Confidence:     1.0,
	}

	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	for _, ins := range insights {
		if ins.PatternID == "j17_tier_ineffective" {
			t.Error("should not fire when tiers are equal")
		}
	}
}

func TestReflect_J17TierIneffective_Disabled(t *testing.T) {
	cfg := config.Config{
		J17TierDriftDetectionEnabled: false, // disabled
	}
	r := NewReflector(cfg, nil)
	r.SetProtocolProvider(&mockTierProtocolProvider{
		stats: ProtocolStatsResult{
			TierComprehension: [3]float64{0.3, 0.9, 0},
			TierOutcomeCount:  [3]int64{10, 10, 0},
			TierCodeComprehension: map[int]map[string]float64{
				1: {"code-a": 0.3},
				2: {"code-a": 0.9},
			},
		},
	})

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		OverallHealth:  0.7,
		ProtocolHealth: 0.5,
		Confidence:     1.0,
	}

	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	for _, ins := range insights {
		if ins.PatternID == "j17_tier_ineffective" {
			t.Error("should not fire when disabled")
		}
	}
}

func TestReflect_J17NLICalibrationDrift(t *testing.T) {
	cfg := config.Config{
		J17NLICalibrationBiasThreshold: 0.15,
	}
	r := NewReflector(cfg, nil)
	r.SetProtocolProvider(&mockTierProtocolProvider{
		stats: ProtocolStatsResult{
			NLIMeanBias:  0.25,
			NLIBiasAlert: true,
		},
	})

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		OverallHealth:  0.7,
		ProtocolHealth: 0.5,
		Confidence:     1.0,
	}

	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("Reflect failed: %v", err)
	}

	found := false
	for _, ins := range insights {
		if ins.PatternID == "j17_nli_calibration_drift" {
			found = true
			if ins.Severity != SeverityMedium {
				t.Errorf("severity = %s, want medium", ins.Severity)
			}
			break
		}
	}
	if !found {
		t.Error("expected j17_nli_calibration_drift insight")
	}
}

// mockTierProtocolProvider implements ProtocolStatsProvider for testing.
type mockTierProtocolProvider struct {
	stats ProtocolStatsResult
}

func (m *mockTierProtocolProvider) GetProtocolStats(_ context.Context, _ string) (ProtocolStatsResult, error) {
	return m.stats, nil
}

func TestReflect_J17LowComprehension_InsufficientSamples(t *testing.T) {
	cfg := config.Config{
		J17TierEffectivenessMinSamples: 5,
		J17ComprehensionMinThreshold:   0.7,
	}
	r := NewReflector(cfg, nil)
	r.SetProtocolProvider(&mockTierProtocolProvider{
		stats: ProtocolStatsResult{
			CodeComprehension: map[string]float64{"weak-code": 0.2},
			CodeOutcomeCount:  map[string]int64{"weak-code": 3}, // < 5 minimum
		},
	})

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		ProtocolHealth: 0.5,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, insight := range insights {
		if insight.PatternID == "j17_low_comprehension" {
			t.Error("Pattern 11 should NOT fire with insufficient samples (3 < 5)")
		}
	}
}

func TestReflect_J17LowComprehension_SufficientSamples(t *testing.T) {
	cfg := config.Config{
		J17TierEffectivenessMinSamples: 5,
		J17ComprehensionMinThreshold:   0.7,
	}
	r := NewReflector(cfg, nil)
	r.SetProtocolProvider(&mockTierProtocolProvider{
		stats: ProtocolStatsResult{
			CodeComprehension: map[string]float64{"weak-code": 0.2},
			CodeOutcomeCount:  map[string]int64{"weak-code": 10}, // >= 5 minimum
		},
	})

	report := &SelfAssessmentReport{
		SpaceID:        "test",
		ProtocolHealth: 0.5,
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, insight := range insights {
		if insight.PatternID == "j17_low_comprehension" {
			found = true
			if insight.RecommendedAction != "retire_code" {
				t.Errorf("expected action retire_code, got %s", insight.RecommendedAction)
			}
		}
	}
	if !found {
		t.Error("Pattern 11 should fire with sufficient samples (10 >= 5) and low comprehension (0.2 < 0.7)")
	}
}
