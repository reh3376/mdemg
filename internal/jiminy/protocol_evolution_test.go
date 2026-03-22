package jiminy

import (
	"context"
	"testing"
)

func TestProtocolEvolver_CodifyConstraint(t *testing.T) {
	codeGen := NewConstraintCodeGenerator(nil) // no LLM → fallback codes
	metrics := NewProtocolMetricsCollector()
	evolver := NewProtocolEvolver(metrics, codeGen, nil, nil)

	result, err := evolver.CodifyConstraint(context.Background(), "test-space", "constraint-123")
	if err != nil {
		t.Fatalf("CodifyConstraint failed: %v", err)
	}

	if result["constraint_id"] != "constraint-123" {
		t.Errorf("constraint_id = %v, want constraint-123", result["constraint_id"])
	}
	if result["new_code"] == nil || result["new_code"] == "" {
		t.Error("new_code should not be empty")
	}
	if result["action"] != "codify_constraint" {
		t.Errorf("action = %v, want codify_constraint", result["action"])
	}
}

func TestProtocolEvolver_RetireCode(t *testing.T) {
	evolver := NewProtocolEvolver(nil, nil, nil, nil)

	result, err := evolver.RetireCode(context.Background(), "test-space", "no-force-push")
	if err != nil {
		t.Fatalf("RetireCode failed: %v", err)
	}

	if result["retired_code"] != "no-force-push" {
		t.Errorf("retired_code = %v, want no-force-push", result["retired_code"])
	}
	if result["action"] != "retire_code" {
		t.Errorf("action = %v, want retire_code", result["action"])
	}
}

func TestProtocolEvolver_AdjustTierThresholds(t *testing.T) {
	metrics := NewProtocolMetricsCollector()
	// Simulate low T1 distribution
	for range 3 {
		metrics.RecordGuidance(2, 50, nil)
	}
	metrics.RecordGuidance(1, 15, nil)

	evolver := NewProtocolEvolver(metrics, nil, nil, nil)

	result, err := evolver.AdjustTierThresholds(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("AdjustTierThresholds failed: %v", err)
	}

	if result["action"] != "adjust_tier_threshold" {
		t.Errorf("action = %v, want adjust_tier_threshold", result["action"])
	}
}

func TestProtocolEvolver_AdjustReplayBuffer(t *testing.T) {
	metrics := NewProtocolMetricsCollector()
	evolver := NewProtocolEvolver(metrics, nil, nil, nil)

	result, err := evolver.AdjustReplayBuffer(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("AdjustReplayBuffer failed: %v", err)
	}

	if result["action"] != "adjust_replay_buffer" {
		t.Errorf("action = %v, want adjust_replay_buffer", result["action"])
	}
}

func TestProtocolEvolver_NeedsDeepAnalysis(t *testing.T) {
	metrics := NewProtocolMetricsCollector()
	evolver := NewProtocolEvolver(metrics, nil, nil, nil)

	// Empty metrics → no deep analysis needed
	if evolver.NeedsDeepAnalysis() {
		t.Error("empty metrics should not need deep analysis")
	}
}

func TestProtocolEvolver_NilCodeGen(t *testing.T) {
	evolver := NewProtocolEvolver(nil, nil, nil, nil)

	_, err := evolver.CodifyConstraint(context.Background(), "test-space", "node-1")
	if err == nil {
		t.Error("expected error for nil code generator")
	}
}
