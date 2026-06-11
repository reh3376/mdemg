package ape

import (
	"context"
	"testing"

	"mdemg/internal/config"
)

func TestPlan_SuppressesLowConfidenceActions(t *testing.T) {
	cfg := config.Config{
		RSICMaxNodePrunePct:     0.05,
		RSICMaxEdgePrunePct:     0.10,
		RSICMinActionConfidence: 0.2,
	}

	// Set up calibrator with known low-confidence action
	cal := NewCalibrator(nil, 100)
	// Record 10 failures for prune_decayed_edges (0% success)
	for range 10 {
		cal.UpdateCalibration(&CycleOutcome{CriteriaMet: false}, []RSICTaskSpec{
			{TaskID: "t1", ActionType: "prune_decayed_edges"},
		}, []RSICProgressReport{
			{TaskID: "t1", Status: "completed"},
		})
	}

	planner := NewPlanner(cfg)
	planner.SetCalibrator(cal)

	insights := []ReflectionInsight{
		{RecommendedAction: "prune_decayed_edges", Severity: SeverityMedium, Description: "edges decayed"},
		{RecommendedAction: "trigger_consolidation", Severity: SeverityMedium, Description: "needs consolidation"},
	}

	specs, err := planner.Plan(context.Background(), insights, "test-space", map[string]float64{"total_nodes": 1000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// prune_decayed_edges should be suppressed (0% < 20% threshold)
	for _, s := range specs {
		if s.ActionType == "prune_decayed_edges" {
			t.Error("expected prune_decayed_edges to be suppressed due to low confidence")
		}
	}
	// trigger_consolidation should remain (no calibration data = not suppressed)
	found := false
	for _, s := range specs {
		if s.ActionType == "trigger_consolidation" {
			found = true
		}
	}
	if !found {
		t.Error("expected trigger_consolidation to be present")
	}
}

func TestPlan_AllowsCriticalEvenLowConfidence(t *testing.T) {
	cfg := config.Config{
		RSICMaxNodePrunePct:     0.05,
		RSICMaxEdgePrunePct:     0.10,
		RSICMinActionConfidence: 0.2,
	}

	cal := NewCalibrator(nil, 100)
	// Record 10 failures for tombstone_stale
	for range 10 {
		cal.UpdateCalibration(&CycleOutcome{CriteriaMet: false}, []RSICTaskSpec{
			{TaskID: "t1", ActionType: "tombstone_stale"},
		}, []RSICProgressReport{
			{TaskID: "t1", Status: "completed"},
		})
	}

	planner := NewPlanner(cfg)
	planner.SetCalibrator(cal)

	insights := []ReflectionInsight{
		{RecommendedAction: "tombstone_stale", Severity: SeverityCritical, Description: "critical stale data"},
	}

	specs, err := planner.Plan(context.Background(), insights, "test-space", map[string]float64{"total_nodes": 1000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Critical severity should bypass confidence suppression
	found := false
	for _, s := range specs {
		if s.ActionType == "tombstone_stale" {
			found = true
		}
	}
	if !found {
		t.Error("expected tombstone_stale to be present (critical severity bypasses low confidence)")
	}
}

func TestPlan_NoCalibrator(t *testing.T) {
	cfg := config.Config{
		RSICMaxNodePrunePct:     0.05,
		RSICMaxEdgePrunePct:     0.10,
		RSICMinActionConfidence: 0.2,
	}
	planner := NewPlanner(cfg)
	// No calibrator set — should plan without filtering

	insights := []ReflectionInsight{
		{RecommendedAction: "prune_decayed_edges", Severity: SeverityMedium, Description: "test"},
	}

	specs, err := planner.Plan(context.Background(), insights, "test-space", map[string]float64{"total_nodes": 1000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Errorf("expected 1 spec without calibrator, got %d", len(specs))
	}
}
