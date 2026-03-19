package ape

import (
	"context"
	"testing"
)

func TestValidateWithPostReport(t *testing.T) {
	cal := NewCalibrator(nil, 100)

	tasks := []RSICTaskSpec{
		{TaskID: "t1", ActionType: "prune_decayed_edges"},
	}
	reports := []RSICProgressReport{
		{TaskID: "t1", Status: "completed"},
	}
	metricsBefore := map[string]float64{
		"overall_health": 0.6,
		"edge_health":    0.5,
	}

	postReport := &SelfAssessmentReport{
		OverallHealth:    0.75,
		RetrievalQuality: 0.8,
		MemoryHealth:     0.9,
		EdgeHealth:       0.7,
		OrphanRatio:      0.1,
		CorrectionRate:   0.05,
		EdgeWeightEntropy: 0.6,
	}

	outcome := cal.Validate(context.Background(), "cycle-1", TierMicro, "test-space", tasks, reports, metricsBefore, postReport)

	if outcome.MetricsAfter["overall_health"] != 0.75 {
		t.Errorf("expected overall_health=0.75, got %f", outcome.MetricsAfter["overall_health"])
	}
	if outcome.MetricsAfter["retrieval_quality"] != 0.8 {
		t.Errorf("expected retrieval_quality=0.8, got %f", outcome.MetricsAfter["retrieval_quality"])
	}
	if outcome.MetricsAfter["edge_weight_entropy"] != 0.6 {
		t.Errorf("expected edge_weight_entropy=0.6, got %f", outcome.MetricsAfter["edge_weight_entropy"])
	}
}

func TestValidateWithNilPostReport(t *testing.T) {
	cal := NewCalibrator(nil, 100)

	tasks := []RSICTaskSpec{
		{TaskID: "t1", ActionType: "prune_decayed_edges"},
	}
	reports := []RSICProgressReport{
		{TaskID: "t1", Status: "completed"},
	}

	outcome := cal.Validate(context.Background(), "cycle-2", TierMicro, "test-space", tasks, reports, nil, nil)

	// MetricsAfter should be empty map (initialized in Validate)
	if len(outcome.MetricsAfter) != 0 {
		t.Errorf("expected empty MetricsAfter with nil postReport, got %v", outcome.MetricsAfter)
	}
	// With no criteria to evaluate, CriteriaMet should be true
	if !outcome.CriteriaMet {
		t.Error("expected CriteriaMet=true when no success criteria defined")
	}
}

func TestCriteriaEvaluation_Met(t *testing.T) {
	cal := NewCalibrator(nil, 100)

	tasks := []RSICTaskSpec{
		{
			TaskID:     "t1",
			ActionType: "prune_decayed_edges",
			SuccessCriteria: []Criterion{
				{Metric: "overall_health", Operator: "gte", Threshold: 0.05},
			},
		},
	}
	reports := []RSICProgressReport{
		{TaskID: "t1", Status: "completed"},
	}
	metricsBefore := map[string]float64{"overall_health": 0.6}
	postReport := &SelfAssessmentReport{OverallHealth: 0.7} // delta = +0.1 >= 0.05

	outcome := cal.Validate(context.Background(), "cycle-3", TierMicro, "test-space", tasks, reports, metricsBefore, postReport)

	if !outcome.CriteriaMet {
		t.Error("expected CriteriaMet=true, got false")
	}
	if outcome.CriteriaDetail["overall_health"] != "met" {
		t.Errorf("expected criteria detail 'met', got %q", outcome.CriteriaDetail["overall_health"])
	}
}

func TestCriteriaEvaluation_NotMet(t *testing.T) {
	cal := NewCalibrator(nil, 100)

	tasks := []RSICTaskSpec{
		{
			TaskID:     "t1",
			ActionType: "tombstone_stale",
			SuccessCriteria: []Criterion{
				{Metric: "overall_health", Operator: "gte", Threshold: 0.2},
			},
		},
	}
	reports := []RSICProgressReport{
		{TaskID: "t1", Status: "completed"},
	}
	metricsBefore := map[string]float64{"overall_health": 0.6}
	postReport := &SelfAssessmentReport{OverallHealth: 0.65} // delta = +0.05 < 0.2

	outcome := cal.Validate(context.Background(), "cycle-4", TierMicro, "test-space", tasks, reports, metricsBefore, postReport)

	if outcome.CriteriaMet {
		t.Error("expected CriteriaMet=false, got true")
	}
}

func TestCriteriaEvaluation_MissingData(t *testing.T) {
	cal := NewCalibrator(nil, 100)

	tasks := []RSICTaskSpec{
		{
			TaskID:     "t1",
			ActionType: "prune_decayed_edges",
			SuccessCriteria: []Criterion{
				{Metric: "nonexistent_metric", Operator: "gte", Threshold: 0.1},
			},
		},
	}
	reports := []RSICProgressReport{
		{TaskID: "t1", Status: "completed"},
	}

	outcome := cal.Validate(context.Background(), "cycle-5", TierMicro, "test-space", tasks, reports, map[string]float64{}, nil)

	// Missing data should not fail criteria (CriteriaMet stays true)
	if !outcome.CriteriaMet {
		t.Error("expected CriteriaMet=true for missing data (no delta to evaluate)")
	}
	if outcome.CriteriaDetail["nonexistent_metric"] != "missing_data" {
		t.Errorf("expected 'missing_data', got %q", outcome.CriteriaDetail["nonexistent_metric"])
	}
}

func TestEvaluateCriterion_AllOperators(t *testing.T) {
	tests := []struct {
		name     string
		c        Criterion
		delta    float64
		afterVal float64
		want     bool
	}{
		{"gte_met", Criterion{Operator: "gte", Threshold: 0.1}, 0.15, 0, true},
		{"gte_not", Criterion{Operator: "gte", Threshold: 0.2}, 0.1, 0, false},
		{"gte_exact", Criterion{Operator: "gte", Threshold: 0.1}, 0.1, 0, true},
		{"gt_met", Criterion{Operator: "gt", Threshold: 0.1}, 0.15, 0, true},
		{"gt_exact_fail", Criterion{Operator: "gt", Threshold: 0.1}, 0.1, 0, false},
		{"lte_met", Criterion{Operator: "lte", Threshold: 0.1}, 0.05, 0, true},
		{"lte_not", Criterion{Operator: "lte", Threshold: 0.1}, 0.2, 0, false},
		{"lt_met", Criterion{Operator: "lt", Threshold: 0.1}, 0.05, 0, true},
		{"lt_exact_fail", Criterion{Operator: "lt", Threshold: 0.1}, 0.1, 0, false},
		{"eq_met", Criterion{Operator: "eq", Threshold: 0.5}, 0, 0.5, true},
		{"eq_not", Criterion{Operator: "eq", Threshold: 0.5}, 0, 0.6, false},
		{"unknown_passes", Criterion{Operator: "???"}, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateCriterion(tt.c, tt.delta, tt.afterVal)
			if got != tt.want {
				t.Errorf("evaluateCriterion(%v, %f, %f) = %v, want %v", tt.c, tt.delta, tt.afterVal, got, tt.want)
			}
		})
	}
}

func TestUpdateCalibration_CriteriaAffectsSuccess(t *testing.T) {
	cal := NewCalibrator(nil, 100)

	// Cycle where task completed but criteria NOT met
	outcome := &CycleOutcome{
		CycleID:     "cycle-crit",
		CriteriaMet: false,
	}
	tasks := []RSICTaskSpec{
		{TaskID: "t1", ActionType: "tombstone_stale"},
	}
	reports := []RSICProgressReport{
		{TaskID: "t1", Status: "completed"},
	}

	cal.UpdateCalibration(outcome, tasks, reports)

	confidence := cal.GetCalibration()
	// Task completed but criteria not met → should be recorded as failure
	if confidence["tombstone_stale"] != 0.0 {
		t.Errorf("expected confidence=0.0 (completed but criteria not met), got %f", confidence["tombstone_stale"])
	}

	// Now add a cycle where criteria ARE met
	outcome2 := &CycleOutcome{
		CycleID:     "cycle-crit2",
		CriteriaMet: true,
	}
	cal.UpdateCalibration(outcome2, tasks, reports)

	confidence = cal.GetCalibration()
	if confidence["tombstone_stale"] != 0.5 {
		t.Errorf("expected confidence=0.5 (1 success out of 2), got %f", confidence["tombstone_stale"])
	}
}

func TestIsReversibleAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"tombstone_stale", true},
		{"graduate_volatile", true},
		{"prune_decayed_edges", false},
		{"prune_excess_edges", false},
		{"trigger_consolidation", false},
		{"refresh_stale_edges", false},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			if got := isReversibleAction(tt.action); got != tt.want {
				t.Errorf("isReversibleAction(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}
