package jiminy

import (
	"math"
	"sort"
	"testing"

	"mdemg/internal/config"
)

// DORMANT-CENSUS-001: Tier 1 tests for the signal-strength sort blend.

func sortKeyService(weight float64, sl SignalLearnerProvider) *Service {
	svc := NewService(config.Config{JiminySignalStrengthWeight: weight}, nil, nil, nil)
	if sl != nil {
		svc.SetSignalLearner(sl)
	}
	return svc
}

func TestGuidanceSortKey_WeightZeroIsPureConfidence(t *testing.T) {
	sl := &mockSignalLearner{strengths: map[string]float64{"guidance:CODE-A": 1.0}}
	svc := sortKeyService(0, sl)
	item := GuidanceItem{Confidence: 0.42, ConstraintCode: "CODE-A"}
	if got := svc.guidanceSortKey(item); got != 0.42 {
		t.Errorf("weight 0 must return pure confidence, got %v", got)
	}
}

func TestGuidanceSortKey_NilLearnerIsPureConfidence(t *testing.T) {
	svc := sortKeyService(0.5, nil)
	item := GuidanceItem{Confidence: 0.42, ConstraintCode: "CODE-A"}
	if got := svc.guidanceSortKey(item); got != 0.42 {
		t.Errorf("nil learner must return pure confidence, got %v", got)
	}
}

func TestGuidanceSortKey_BlendFormula(t *testing.T) {
	sl := &mockSignalLearner{strengths: map[string]float64{"guidance:CODE-A": 0.9}}
	svc := sortKeyService(0.2, sl)
	item := GuidanceItem{Confidence: 0.5, ConstraintCode: "CODE-A"}
	want := 0.8*0.5 + 0.2*0.9
	if got := svc.guidanceSortKey(item); math.Abs(got-want) > 1e-9 {
		t.Errorf("blend = %v, want %v", got, want)
	}
}

func TestGuidanceSortKey_UnknownCodeUsesNeutralDefault(t *testing.T) {
	sl := &mockSignalLearner{}
	svc := sortKeyService(0.2, sl)
	item := GuidanceItem{Confidence: 0.5, ConstraintCode: "NEVER-SEEN"}
	want := 0.8*0.5 + 0.2*0.5
	if got := svc.guidanceSortKey(item); math.Abs(got-want) > 1e-9 {
		t.Errorf("unknown code blend = %v, want %v (0.5 neutral)", got, want)
	}
}

func TestGuidanceSortKey_WeightClampedToOne(t *testing.T) {
	sl := &mockSignalLearner{strengths: map[string]float64{"guidance:CODE-A": 0.9}}
	svc := sortKeyService(5.0, sl)
	item := GuidanceItem{Confidence: 0.1, ConstraintCode: "CODE-A"}
	if got := svc.guidanceSortKey(item); math.Abs(got-0.9) > 1e-9 {
		t.Errorf("weight >1 must clamp to pure strength, got %v", got)
	}
}

// TestGuidanceSort_StrengthTiebreaksWithinPriority verifies the full sort
// contract: priority dominates unconditionally; within equal priority a
// strong learned signal can overtake a slightly-higher-confidence item.
func TestGuidanceSort_StrengthTiebreaksWithinPriority(t *testing.T) {
	sl := &mockSignalLearner{strengths: map[string]float64{
		"guidance:STRONG": 0.95,
		"guidance:WEAK":   0.05,
	}}
	svc := sortKeyService(0.3, sl)

	items := []GuidanceItem{
		{Priority: "medium", Confidence: 0.60, ConstraintCode: "WEAK"},   // 0.7·0.60+0.3·0.05 = 0.435
		{Priority: "medium", Confidence: 0.55, ConstraintCode: "STRONG"}, // 0.7·0.55+0.3·0.95 = 0.670
		{Priority: "high", Confidence: 0.10, ConstraintCode: "WEAK"},     // high priority wins regardless
	}
	sort.Slice(items, func(i, j int) bool {
		pi, pj := priorityRank(items[i].Priority), priorityRank(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return svc.guidanceSortKey(items[i]) > svc.guidanceSortKey(items[j])
	})

	if items[0].Priority != "high" {
		t.Fatalf("priority must dominate: got %q first", items[0].Priority)
	}
	if items[1].ConstraintCode != "STRONG" {
		t.Errorf("learned strength must overtake within priority: got %q before %q",
			items[1].ConstraintCode, items[2].ConstraintCode)
	}

	// And with weight 0 the same set falls back to pure confidence order.
	svc0 := sortKeyService(0, sl)
	sort.Slice(items, func(i, j int) bool {
		pi, pj := priorityRank(items[i].Priority), priorityRank(items[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return svc0.guidanceSortKey(items[i]) > svc0.guidanceSortKey(items[j])
	})
	if items[1].ConstraintCode != "WEAK" {
		t.Errorf("weight 0 must restore confidence order: got %q second", items[1].ConstraintCode)
	}
}
