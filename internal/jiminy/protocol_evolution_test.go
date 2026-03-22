package jiminy

import (
	"context"
	"testing"
)

func TestProtocolEvolver_CodifyConstraint(t *testing.T) {
	codeGen := NewConstraintCodeGenerator(nil) // no LLM → fallback codes
	metrics := NewProtocolMetricsCollector()
	evolver := NewProtocolEvolver(metrics, codeGen, nil, nil, nil, nil)

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
	evolver := NewProtocolEvolver(nil, nil, nil, nil, nil, nil)

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

	evolver := NewProtocolEvolver(metrics, nil, nil, nil, nil, nil)

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
	evolver := NewProtocolEvolver(metrics, nil, nil, nil, nil, nil)

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
	evolver := NewProtocolEvolver(metrics, nil, nil, nil, nil, nil)

	// Empty metrics → no deep analysis needed
	if evolver.NeedsDeepAnalysis() {
		t.Error("empty metrics should not need deep analysis")
	}
}

func TestProtocolEvolver_NilCodeGen(t *testing.T) {
	evolver := NewProtocolEvolver(nil, nil, nil, nil, nil, nil)

	_, err := evolver.CodifyConstraint(context.Background(), "test-space", "node-1")
	if err == nil {
		t.Error("expected error for nil code generator")
	}
}

func TestProtocolEvolver_RetireCode_ClearsCollisionSet(t *testing.T) {
	codeGen := NewConstraintCodeGenerator(nil)
	codeGen.RegisterExistingCode("test-retire-code")

	evolver := NewProtocolEvolver(nil, codeGen, nil, nil, nil, nil)
	result, err := evolver.RetireCode(context.Background(), "test-space", "test-retire-code")
	if err != nil {
		t.Fatalf("RetireCode failed: %v", err)
	}

	if result["retired_code"] != "test-retire-code" {
		t.Errorf("retired_code = %v, want test-retire-code", result["retired_code"])
	}

	// The code should be unregistered — generating a new code with the same description
	// should not collide (this tests that UnregisterCode was called)
	codeGen.mu.Lock()
	exists := codeGen.existing["test-retire-code"]
	codeGen.mu.Unlock()
	if exists {
		t.Error("retired code should be removed from collision set")
	}
}

func TestProtocolEvolver_AdjustTierThresholds_WriteBack(t *testing.T) {
	metrics := NewProtocolMetricsCollector()
	// Simulate very low T1 usage to trigger threshold decrease
	for range 10 {
		metrics.RecordGuidance(3, 80, nil) // all T3
	}

	trustScorer := NewTrustScorer(TrustConfig{
		Initial:       0.5,
		HighThreshold: 0.8,
		LowThreshold:  0.4,
	})
	encoder := NewProtocolEncoder(TierCoded)

	evolver := NewProtocolEvolver(metrics, nil, encoder, nil, nil, trustScorer)
	result, err := evolver.AdjustTierThresholds(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("AdjustTierThresholds failed: %v", err)
	}

	// Should have adjusted thresholds downward (T1 distribution is 0%)
	newHigh, ok := result["new_high_threshold"].(float64)
	if !ok {
		t.Fatal("new_high_threshold not in result")
	}
	if newHigh >= 0.8 {
		t.Errorf("new_high_threshold = %.2f, expected < 0.8 (should decrease)", newHigh)
	}

	// Verify write-back to trust scorer
	if trustScorer.HighThreshold() == 0.8 {
		t.Error("trust scorer high threshold should have been updated")
	}

	// Verify write-back to encoder
	encHigh, encLow := encoder.GetTierThresholds()
	if encHigh == 0.8 || encLow == 0.4 {
		t.Errorf("encoder thresholds should have been updated, got high=%.2f low=%.2f", encHigh, encLow)
	}
}

func TestCodeGenerator_UnregisterCode(t *testing.T) {
	codeGen := NewConstraintCodeGenerator(nil)

	codeGen.RegisterExistingCode("foo-bar")
	codeGen.RegisterExistingCode("baz-qux")

	// Unregister one
	codeGen.UnregisterCode("foo-bar")

	codeGen.mu.Lock()
	hasFoo := codeGen.existing["foo-bar"]
	hasBaz := codeGen.existing["baz-qux"]
	codeGen.mu.Unlock()

	if hasFoo {
		t.Error("foo-bar should be unregistered")
	}
	if !hasBaz {
		t.Error("baz-qux should still be registered")
	}
}
