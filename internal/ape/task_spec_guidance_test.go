package ape

import (
	"testing"

	"mdemg/internal/config"
)

func TestBuildTaskSpec_AdjustGuidanceConfidence(t *testing.T) {
	cfg := config.Config{
		RSICMaxNodePrunePct: 0.05,
		RSICMaxEdgePrunePct: 0.10,
	}

	action := ImprovementAction{
		ActionType:  "adjust_guidance_confidence",
		TargetSpace: "test-space",
		Rationale:   "guidance health drifting",
		Priority:    1,
	}

	spec := BuildTaskSpec(cfg, action, "cycle-123", nil, 1000)

	if spec.ActionType != "adjust_guidance_confidence" {
		t.Errorf("expected action type adjust_guidance_confidence, got %s", spec.ActionType)
	}

	if len(spec.AllowedEndpoints) != 2 {
		t.Errorf("expected 2 allowed endpoints, got %d", len(spec.AllowedEndpoints))
	}

	if len(spec.Deliverables) != 3 {
		t.Errorf("expected 3 deliverables (boosted, decayed, archived), got %d", len(spec.Deliverables))
	}

	expectedDeliverables := map[string]bool{"boosted": false, "decayed": false, "archived": false}
	for _, d := range spec.Deliverables {
		if _, ok := expectedDeliverables[d.Name]; ok {
			expectedDeliverables[d.Name] = true
		}
	}
	for name, found := range expectedDeliverables {
		if !found {
			t.Errorf("missing deliverable: %s", name)
		}
	}

	if len(spec.SuccessCriteria) != 1 || spec.SuccessCriteria[0].Metric != "guidance_health_delta" {
		t.Errorf("expected success criterion guidance_health_delta, got %+v", spec.SuccessCriteria)
	}

	// Test description
	desc := descriptionForAction("adjust_guidance_confidence")
	if desc == "" || desc == "Unknown action type: adjust_guidance_confidence" {
		t.Error("missing description for adjust_guidance_confidence")
	}
}
