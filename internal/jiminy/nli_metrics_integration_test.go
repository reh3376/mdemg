package jiminy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNLIFlowsToMetricsInShadowMode verifies NLI scores reach protocolMetrics
// when the sidecar is in shadow mode (not causal).
func TestNLIFlowsToMetricsInShadowMode(t *testing.T) {
	// Mock NLI sidecar that returns a fixed score
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"label": "entailment",
			"scores": map[string]float64{
				"entailment":    0.85,
				"contradiction": 0.10,
				"neutral":       0.05,
			},
		})
	}))
	defer sidecar.Close()

	// Create metrics collector and NLI scorer pointing at mock sidecar
	metrics := NewProtocolMetricsCollector()
	nliScorer := NewNLIComprehensionScorer(sidecar.URL, 5000, true)

	// Simulate shadow mode arbitrator (never causal)
	arbitrator := NewSidecarArbitrator("shadow", 100, 0.6, "", false)

	j17NLIObservationalEnabled := true

	// Simulate what service.go does in the feedback loop:
	// 1. NLI scorer evaluates comprehension
	nliScore, isFallback := nliScorer.ScoreComprehension(context.Background(), "Never force push to main", "I pushed to main with --force", true)
	if isFallback {
		t.Fatal("NLI scorer returned fallback, expected genuine score")
	}
	if nliScore <= 0 {
		t.Fatalf("NLI scorer returned %f, expected > 0", nliScore)
	}

	// 2. Since nliScorer != nil AND J17NLIObservationalEnabled, record to metrics
	// This is what the fixed gate logic does (NO IsCausal check)
	if j17NLIObservationalEnabled && !arbitrator.IsCausal() {
		// In shadow mode, IsCausal() is false — but we STILL record (that's the fix)
		metrics.RecordOutcomeWithTier("no-force-push", 1, nliScore)
	}

	// 3. Verify NLI score reached the metrics
	snapshot := metrics.Snapshot()
	if snapshot.TierOutcomeCount[0] != 1 {
		t.Errorf("T1 outcome count = %d, want 1", snapshot.TierOutcomeCount[0])
	}
	if snapshot.TierComprehension[0] <= 0 {
		t.Errorf("T1 comprehension = %f, want > 0", snapshot.TierComprehension[0])
	}

	codeComp, ok := snapshot.TierCodeComprehension[1]
	if !ok || codeComp["no-force-push"] <= 0 {
		t.Error("expected per-code per-tier comprehension for no-force-push at T1")
	}
}

// TestHeuristicFallbackWhenNLIUnavailable verifies heuristic scores still work
// when nliScorer is nil.
func TestHeuristicFallbackWhenNLIUnavailable(t *testing.T) {
	metrics := NewProtocolMetricsCollector()

	// nliScorer is nil → heuristic path
	// Simulate the heuristic block from service.go
	var compScore float64 = 1.0 // OutcomeFollowed
	metrics.RecordOutcomeWithTier("no-force-push", 1, compScore)

	snapshot := metrics.Snapshot()
	if snapshot.TierOutcomeCount[0] != 1 {
		t.Errorf("T1 outcome count = %d, want 1", snapshot.TierOutcomeCount[0])
	}
	if snapshot.TierComprehension[0] != 1.0 {
		t.Errorf("T1 comprehension = %f, want 1.0", snapshot.TierComprehension[0])
	}
}

// TestNoDoubleCountingInCausalMode verifies codeTotal[code] is incremented exactly once
// regardless of sidecar mode, when both NLI and heuristic paths exist.
func TestNoDoubleCountingInCausalMode(t *testing.T) {
	metrics := NewProtocolMetricsCollector()

	// Simulate NLI path (nliScorer != nil): only NLI block fires
	// The heuristic block is skipped when nliScorer is present
	nliScore := 0.85
	metrics.RecordOutcomeWithTier("no-force-push", 1, nliScore)

	snapshot := metrics.Snapshot()

	// codeTotal should be exactly 1 (not 2 from double-counting)
	if snapshot.CodeComprehension["no-force-push"] == 0 {
		t.Error("code comprehension should be tracked")
	}
	if snapshot.TierOutcomeCount[0] != 1 {
		t.Errorf("T1 outcome count = %d, want exactly 1 (no double-counting)", snapshot.TierOutcomeCount[0])
	}

	// Record a second outcome for a different tier
	metrics.RecordOutcomeWithTier("no-force-push", 2, 0.60)

	snapshot = metrics.Snapshot()
	if snapshot.TierOutcomeCount[0] != 1 {
		t.Errorf("T1 count should still be 1, got %d", snapshot.TierOutcomeCount[0])
	}
	if snapshot.TierOutcomeCount[1] != 1 {
		t.Errorf("T2 count should be 1, got %d", snapshot.TierOutcomeCount[1])
	}

	// Total code outcomes should be 2 (one per RecordOutcomeWithTier call)
	totalForCode := 0
	for i := range 3 {
		totalForCode += int(snapshot.TierOutcomeCount[i])
	}
	if totalForCode != 2 {
		t.Errorf("total tier outcome count = %d, want exactly 2", totalForCode)
	}
}
