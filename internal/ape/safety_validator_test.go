package ape

import (
	"context"
	"testing"
)

func TestIsDestructiveAction_ClassifiesCorrectly(t *testing.T) {
	destructive := []string{"prune_decayed_edges", "prune_excess_edges", "tombstone_stale"}
	nonDestructive := []string{"trigger_consolidation", "graduate_volatile", "refresh_stale_edges"}

	for _, a := range destructive {
		if !IsDestructiveAction(a) {
			t.Errorf("expected %s to be destructive", a)
		}
	}
	for _, a := range nonDestructive {
		if IsDestructiveAction(a) {
			t.Errorf("expected %s to be non-destructive", a)
		}
	}
}

func TestValidateAction_AllowsNonDestructiveAlways(t *testing.T) {
	sv := &SafetyValidator{} // no driver needed for non-destructive
	spec := &RSICTaskSpec{
		ActionType:  "trigger_consolidation",
		TargetSpace: "mdemg-dev",
		Safety: SafetyBounds{
			MaxNodesAffected: 50,
			MaxEdgesAffected: 100,
			ProtectedSpaces:  []string{"mdemg-dev"},
		},
	}

	decision := sv.ValidateAction(context.TODO(), spec, "trigger_consolidation")
	if !decision.Allowed {
		t.Errorf("expected non-destructive action on protected space to be allowed, got rejected: %s", decision.Reason)
	}
}

func TestValidateAction_BlocksCustomProtectedSpaceDestructive(t *testing.T) {
	sv := &SafetyValidator{}
	spec := &RSICTaskSpec{
		ActionType:  "prune_decayed_edges",
		TargetSpace: "production-space",
		Safety: SafetyBounds{
			MaxEdgesAffected: 100,
			ProtectedSpaces:  []string{"production-space"},
		},
	}

	decision := sv.ValidateAction(context.TODO(), spec, "prune_decayed_edges")
	if decision.Allowed {
		t.Error("expected destructive action on custom protected space to be rejected")
	}
	if decision.Reason == "" {
		t.Error("expected rejection reason")
	}
}

func TestValidateAction_FailsClosed_OnEstimationError(t *testing.T) {
	// Without a driver, blast radius estimation fails — safety must fail closed
	sv := &SafetyValidator{}
	spec := &RSICTaskSpec{
		ActionType:  "prune_decayed_edges",
		TargetSpace: "mdemg-dev",
		Safety: SafetyBounds{
			MaxEdgesAffected: 100,
			ProtectedSpaces:  nil,
		},
	}

	decision := sv.ValidateAction(context.TODO(), spec, "prune_decayed_edges")
	if decision.Allowed {
		t.Error("expected destructive action to be blocked when estimation fails (fail-closed)")
	}
	if decision.EstimatedAffected != -1 {
		t.Errorf("expected estimated_affected=-1, got %d", decision.EstimatedAffected)
	}
}

func TestValidateAction_AllowsProtectedSpaceConstructive(t *testing.T) {
	sv := &SafetyValidator{}
	actions := []string{"trigger_consolidation", "graduate_volatile", "refresh_stale_edges"}

	for _, action := range actions {
		spec := &RSICTaskSpec{
			TargetSpace: "mdemg-dev",
			Safety: SafetyBounds{
				ProtectedSpaces: []string{"mdemg-dev"},
			},
		}
		decision := sv.ValidateAction(context.TODO(), spec, action)
		if !decision.Allowed {
			t.Errorf("expected constructive action %s on protected space to be allowed", action)
		}
	}
}

func TestBuildDelta_ProtectedSpaceDestructive(t *testing.T) {
	sv := &SafetyValidator{}
	spec := &RSICTaskSpec{
		TargetSpace: "protected-space",
		Safety: SafetyBounds{
			ProtectedSpaces: []string{"protected-space"},
		},
	}

	delta := sv.BuildDelta(context.TODO(), spec, "tombstone_stale")
	if delta.WouldExecute {
		t.Error("expected would_execute=false for destructive on protected space")
	}
	if !delta.ProtectedSpaceBlocked {
		t.Error("expected protected_space_blocked=true")
	}
}

func TestBuildDelta_EmptyProtectedSpacesAllows(t *testing.T) {
	sv := &SafetyValidator{}
	spec := &RSICTaskSpec{
		TargetSpace: "mdemg-dev",
		Safety: SafetyBounds{
			MaxNodesAffected: 50,
			ProtectedSpaces:  nil, // default: no protected spaces
		},
	}

	delta := sv.BuildDelta(context.TODO(), spec, "tombstone_stale")
	if delta.ProtectedSpaceBlocked {
		t.Error("expected protected_space_blocked=false with empty ProtectedSpaces")
	}
}

func TestBuildDelta_NonDestructiveAllowed(t *testing.T) {
	sv := &SafetyValidator{}
	spec := &RSICTaskSpec{
		TargetSpace: "test-space",
		Safety:      SafetyBounds{},
	}

	delta := sv.BuildDelta(context.TODO(), spec, "trigger_consolidation")
	if !delta.WouldExecute {
		t.Error("expected would_execute=true for non-destructive action")
	}
	if delta.SafetyLimit != -1 {
		t.Errorf("expected safety_limit=-1 for constructive, got %d", delta.SafetyLimit)
	}
}

func TestLimitForAction_CorrectLimits(t *testing.T) {
	sv := &SafetyValidator{}
	spec := &RSICTaskSpec{
		Safety: SafetyBounds{
			MaxNodesAffected: 50,
			MaxEdgesAffected: 100,
		},
	}

	cases := []struct {
		action string
		want   int
	}{
		{"prune_decayed_edges", 100},
		{"prune_excess_edges", 100},
		{"tombstone_stale", 50},
		{"trigger_consolidation", -1},
		{"graduate_volatile", -1},
		{"refresh_stale_edges", -1},
	}

	for _, tc := range cases {
		got := sv.limitForAction(tc.action, spec)
		if got != tc.want {
			t.Errorf("limitForAction(%s) = %d, want %d", tc.action, got, tc.want)
		}
	}
}

func TestCountQueryForAction_ReturnsQueriesForDestructive(t *testing.T) {
	sv := &SafetyValidator{}

	destructive := []string{"prune_decayed_edges", "prune_excess_edges", "tombstone_stale"}
	for _, a := range destructive {
		cypher, params := sv.countQueryForAction(a, "test-space")
		if cypher == "" {
			t.Errorf("expected count query for %s", a)
		}
		if params["spaceId"] != "test-space" {
			t.Errorf("expected spaceId param for %s", a)
		}
	}

	nonDestructive := []string{"trigger_consolidation", "graduate_volatile", "refresh_stale_edges"}
	for _, a := range nonDestructive {
		cypher, _ := sv.countQueryForAction(a, "test-space")
		if cypher != "" {
			t.Errorf("expected no count query for %s, got: %s", a, cypher)
		}
	}
}

func TestSafetySummary_AccumulatesCorrectly(t *testing.T) {
	summary := &SafetySummary{}

	// Simulate 3 checks: 2 allowed, 1 rejected
	summary.ActionsChecked++
	summary.ActionsAllowed++

	summary.ActionsChecked++
	summary.ActionsAllowed++
	summary.SnapshotsCreated++

	summary.ActionsChecked++
	summary.ActionsRejected++
	summary.Rejections = append(summary.Rejections, SafetyRejection{
		Action:            "prune_decayed_edges",
		Reason:            "blast_radius_exceeded",
		EstimatedAffected: 150,
		Limit:             100,
	})

	if summary.ActionsChecked != 3 {
		t.Errorf("expected 3 checked, got %d", summary.ActionsChecked)
	}
	if summary.ActionsAllowed != 2 {
		t.Errorf("expected 2 allowed, got %d", summary.ActionsAllowed)
	}
	if summary.ActionsRejected != 1 {
		t.Errorf("expected 1 rejected, got %d", summary.ActionsRejected)
	}
	if len(summary.Rejections) != 1 {
		t.Errorf("expected 1 rejection detail, got %d", len(summary.Rejections))
	}
	if summary.SnapshotsCreated != 1 {
		t.Errorf("expected 1 snapshot, got %d", summary.SnapshotsCreated)
	}
}

func TestSafetyVersion_IsSet(t *testing.T) {
	if SafetyVersion == "" {
		t.Error("SafetyVersion should not be empty")
	}
	if SafetyVersion != "phase88-v1" {
		t.Errorf("expected phase88-v1, got %s", SafetyVersion)
	}
}

func TestDestructiveActionsMap_HasCorrectEntries(t *testing.T) {
	expected := map[string]bool{
		"prune_decayed_edges": true,
		"prune_excess_edges":  true,
		"tombstone_stale":     true,
	}

	if len(DestructiveActions) != len(expected) {
		t.Errorf("expected %d destructive actions, got %d", len(expected), len(DestructiveActions))
	}

	for k, v := range expected {
		if DestructiveActions[k] != v {
			t.Errorf("expected DestructiveActions[%s] = %v", k, v)
		}
	}
}

func TestActionDelta_Shape(t *testing.T) {
	delta := ActionDelta{
		Action:                "prune_decayed_edges",
		WouldExecute:          true,
		EstimatedAffected:     47,
		SafetyLimit:           100,
		WithinBounds:          true,
		ProtectedSpaceBlocked: false,
	}

	if delta.Action != "prune_decayed_edges" {
		t.Errorf("unexpected action: %s", delta.Action)
	}
	if !delta.WouldExecute {
		t.Error("expected would_execute=true")
	}
	if !delta.WithinBounds {
		t.Error("expected within_bounds=true")
	}
}
