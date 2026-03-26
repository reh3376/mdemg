package jiminy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCalibrationFlowsToRSIC verifies the NLI calibration report data
// reaches the ProtocolMetrics snapshot and can be consumed by the RSIC adapter.
func TestCalibrationFlowsToRSIC(t *testing.T) {
	// Mock NLI sidecar returning entailment with 0.85 score
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

	// Create calibration tracker with tight bias threshold for testing
	tracker := NewNLICalibrationTracker(100, 0.10)

	// Create NLI scorer pointing at mock sidecar
	nliScorer := NewNLIComprehensionScorer(sidecar.URL, 5000, true)

	// Simulate 20 feedback rounds: NLI score ~0.85, heuristic = 1.0 (OutcomeFollowed)
	// This creates a systematic negative bias (NLI < heuristic by ~0.15)
	for range 20 {
		nliScore, _ := nliScorer.ScoreComprehension(context.Background(),
			"Never force push to main", "I followed the constraint carefully", true)
		heuristicScore := 1.0 // OutcomeFollowed always maps to 1.0
		tracker.Track(nliScore, heuristicScore)
	}

	// Get calibration report
	report := tracker.Report()

	// Verify basic report fields
	if report.WindowSize != 20 {
		t.Errorf("window size = %d, want 20", report.WindowSize)
	}
	if report.MeanNLI <= 0 {
		t.Errorf("mean NLI = %f, want > 0", report.MeanNLI)
	}
	if report.MeanHeuristic != 1.0 {
		t.Errorf("mean heuristic = %f, want 1.0", report.MeanHeuristic)
	}

	// NLI scores 0.85, heuristic 1.0 → bias = 0.85 - 1.0 = -0.15
	// With threshold 0.10, this should trigger a bias alert
	if report.MeanBias >= 0 {
		t.Errorf("mean bias = %f, expected negative (NLI < heuristic)", report.MeanBias)
	}
	if !report.BiasAlert {
		t.Error("expected bias alert to be true (|bias| > 0.10 threshold)")
	}
}

// TestCalibrationNoBiasWhenAligned verifies no alert when NLI and heuristic agree.
func TestCalibrationNoBiasWhenAligned(t *testing.T) {
	tracker := NewNLICalibrationTracker(100, 0.15)

	// Track aligned scores: NLI ≈ heuristic
	for range 30 {
		tracker.Track(1.0, 1.0) // Both agree on full comprehension
	}
	for range 20 {
		tracker.Track(0.5, 0.5) // Both agree on partial comprehension
	}

	report := tracker.Report()

	if report.WindowSize != 50 {
		t.Errorf("window size = %d, want 50", report.WindowSize)
	}
	if report.BiasAlert {
		t.Errorf("expected no bias alert, but got bias = %f", report.MeanBias)
	}
	// Mean bias should be ~0
	if report.MeanBias > 0.01 || report.MeanBias < -0.01 {
		t.Errorf("mean bias = %f, expected ~0 when aligned", report.MeanBias)
	}
}

// TestCalibrationReportWithMetricsSnapshot verifies calibration data
// can coexist with protocol metrics in a single snapshot pipeline.
func TestCalibrationReportWithMetricsSnapshot(t *testing.T) {
	metrics := NewProtocolMetricsCollector()
	tracker := NewNLICalibrationTracker(100, 0.15)

	// Simulate mixed feedback:
	// T1 outcomes with NLI scoring
	metrics.RecordOutcomeWithTier("no-force-push", 1, 0.85)
	tracker.Track(0.85, 1.0)

	metrics.RecordOutcomeWithTier("no-force-push", 1, 0.90)
	tracker.Track(0.90, 1.0)

	// T2 outcomes
	metrics.RecordOutcomeWithTier("lint-before-commit", 2, 0.70)
	tracker.Track(0.70, 1.0)

	// Verify metrics snapshot has tier data
	snapshot := metrics.Snapshot()
	if snapshot.TierOutcomeCount[0] != 2 {
		t.Errorf("T1 outcome count = %d, want 2", snapshot.TierOutcomeCount[0])
	}
	if snapshot.TierOutcomeCount[1] != 1 {
		t.Errorf("T2 outcome count = %d, want 1", snapshot.TierOutcomeCount[1])
	}

	// Verify calibration report independently
	report := tracker.Report()
	if report.WindowSize != 3 {
		t.Errorf("calibration window = %d, want 3", report.WindowSize)
	}
	// All NLI < heuristic → negative bias
	if report.MeanBias >= 0 {
		t.Errorf("expected negative bias, got %f", report.MeanBias)
	}

	// Both systems produce valid output simultaneously
	if snapshot.TierComprehension[0] <= 0 {
		t.Error("T1 comprehension should be > 0")
	}
	if report.MeanNLI <= 0 {
		t.Error("calibration mean NLI should be > 0")
	}
}
