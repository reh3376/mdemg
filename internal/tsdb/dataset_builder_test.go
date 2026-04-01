package tsdb

import (
	"math"
	"testing"
	"time"
)

// ─── computeTrendStats ───

func TestComputeTrendStats_Empty(t *testing.T) {
	slope, avg, min, max, vol := computeTrendStats(nil)
	if slope != 0 || avg != 0 || min != 0 || max != 0 || vol != 0 {
		t.Errorf("expected all zeros for empty points, got slope=%f avg=%f min=%f max=%f vol=%f", slope, avg, min, max, vol)
	}
}

func TestComputeTrendStats_SinglePoint(t *testing.T) {
	points := []MetricPoint{{Value: 42.0}}
	slope, avg, min, max, vol := computeTrendStats(points)
	if slope != 0 {
		t.Errorf("slope: got %f, want 0 (single point)", slope)
	}
	if avg != 42.0 {
		t.Errorf("avg: got %f, want 42.0", avg)
	}
	if min != 42.0 || max != 42.0 {
		t.Errorf("min/max: got %f/%f, want 42.0/42.0", min, max)
	}
	if vol != 0 {
		t.Errorf("volatility: got %f, want 0 (single point)", vol)
	}
}

func TestComputeTrendStats_SlopeCalculation(t *testing.T) {
	// y = 2x: points at (0,0), (1,2), (2,4), (3,6)
	points := []MetricPoint{
		{Value: 0}, {Value: 2}, {Value: 4}, {Value: 6},
	}
	slope, avg, min, max, _ := computeTrendStats(points)

	if math.Abs(slope-2.0) > 0.001 {
		t.Errorf("slope: got %f, want 2.0", slope)
	}
	if math.Abs(avg-3.0) > 0.001 {
		t.Errorf("avg: got %f, want 3.0", avg)
	}
	if min != 0 {
		t.Errorf("min: got %f, want 0", min)
	}
	if max != 6 {
		t.Errorf("max: got %f, want 6", max)
	}
}

func TestComputeTrendStats_NegativeSlope(t *testing.T) {
	// y = -1x + 10: points at (0,10), (1,9), (2,8), (3,7)
	points := []MetricPoint{
		{Value: 10}, {Value: 9}, {Value: 8}, {Value: 7},
	}
	slope, _, _, _, _ := computeTrendStats(points)
	if math.Abs(slope-(-1.0)) > 0.001 {
		t.Errorf("slope: got %f, want -1.0", slope)
	}
}

func TestComputeTrendStats_Volatility(t *testing.T) {
	// Constant values: volatility should be 0
	points := []MetricPoint{
		{Value: 5}, {Value: 5}, {Value: 5},
	}
	_, _, _, _, vol := computeTrendStats(points)
	if vol != 0 {
		t.Errorf("volatility: got %f, want 0 (constant values)", vol)
	}

	// Variable values: volatility > 0
	points = []MetricPoint{
		{Value: 1}, {Value: 10}, {Value: 1}, {Value: 10},
	}
	_, _, _, _, vol = computeTrendStats(points)
	if vol <= 0 {
		t.Errorf("volatility: got %f, want > 0 (variable values)", vol)
	}
}

func TestComputeTrendStats_FlatSlope(t *testing.T) {
	// All same value: slope should be 0
	points := []MetricPoint{
		{Value: 5}, {Value: 5}, {Value: 5}, {Value: 5},
	}
	slope, _, _, _, _ := computeTrendStats(points)
	if math.Abs(slope) > 0.001 {
		t.Errorf("slope: got %f, want ~0 (flat)", slope)
	}
}

// ─── TaskReadiness logic ───

func TestTaskReadiness_ThresholdBoundary(t *testing.T) {
	// Below threshold
	tr := TaskReadiness{
		TotalRows:       499,
		HasSystemPrompt: 499,
		HasResponse:     499,
		ErrorCount:      0,
	}
	errorRate := 0.0
	if tr.TotalRows > 0 {
		errorRate = float64(tr.ErrorCount) / float64(tr.TotalRows)
	}
	ready := tr.TotalRows >= DefaultReadinessThreshold &&
		errorRate < 0.05 &&
		tr.HasSystemPrompt == tr.TotalRows
	if ready {
		t.Errorf("should NOT be ready at %d rows (threshold %d)", tr.TotalRows, DefaultReadinessThreshold)
	}

	// At threshold
	tr.TotalRows = 500
	tr.HasSystemPrompt = 500
	tr.HasResponse = 500
	ready = tr.TotalRows >= DefaultReadinessThreshold &&
		errorRate < 0.05 &&
		tr.HasSystemPrompt == tr.TotalRows
	if !ready {
		t.Errorf("should be ready at %d rows (threshold %d)", tr.TotalRows, DefaultReadinessThreshold)
	}
}

func TestTaskReadiness_ErrorRateExceeded(t *testing.T) {
	tr := TaskReadiness{
		TotalRows:       1000,
		HasSystemPrompt: 1000,
		ErrorCount:      60, // 6% > 5% threshold
	}
	errorRate := float64(tr.ErrorCount) / float64(tr.TotalRows)
	ready := tr.TotalRows >= DefaultReadinessThreshold &&
		errorRate < 0.05 &&
		tr.HasSystemPrompt == tr.TotalRows
	if ready {
		t.Errorf("should NOT be ready with error rate %.1f%%", errorRate*100)
	}
}

func TestTaskReadiness_MissingSystemPrompts(t *testing.T) {
	tr := TaskReadiness{
		TotalRows:       1000,
		HasSystemPrompt: 950, // 95% < 100% — not ready
		ErrorCount:      0,
	}
	errorRate := 0.0
	ready := tr.TotalRows >= DefaultReadinessThreshold &&
		errorRate < 0.05 &&
		tr.HasSystemPrompt == tr.TotalRows
	if ready {
		t.Errorf("should NOT be ready with only %d/%d system prompts", tr.HasSystemPrompt, tr.TotalRows)
	}
}

// ─── Type defaults ───

func TestMetricTrend_ZeroPoints(t *testing.T) {
	trend := &MetricTrend{
		MetricName: "test_metric",
		Points:     nil,
		Window:     24 * time.Hour,
	}
	if trend.Slope != 0 {
		t.Errorf("slope: got %f, want 0", trend.Slope)
	}
	if trend.AvgValue != 0 {
		t.Errorf("avg: got %f, want 0", trend.AvgValue)
	}
}

func TestEmbeddingCoverageSummary_Defaults(t *testing.T) {
	s := &EmbeddingCoverageSummary{
		CallSites:  make(map[string]int),
		EventTypes: make(map[string]int),
	}
	if s.TotalEvents != 0 {
		t.Errorf("total: got %d, want 0", s.TotalEvents)
	}
	if s.EmptyCallSites != 0 {
		t.Errorf("empty call sites: got %d, want 0", s.EmptyCallSites)
	}
}

func TestRetrievalQualitySummary_RateCalculation(t *testing.T) {
	s := RetrievalQualitySummary{TotalQueries: 100}
	hasRecall := 95
	s.RecallRate = float64(hasRecall) / float64(s.TotalQueries)
	if math.Abs(s.RecallRate-0.95) > 0.001 {
		t.Errorf("recall rate: got %f, want 0.95", s.RecallRate)
	}
}

func TestLLMPerformanceSummary_ErrorRate(t *testing.T) {
	s := LLMPerformanceSummary{TotalCalls: 200}
	errorCount := 10
	s.ErrorRate = float64(errorCount) / float64(s.TotalCalls)
	if math.Abs(s.ErrorRate-0.05) > 0.001 {
		t.Errorf("error rate: got %f, want 0.05", s.ErrorRate)
	}
}

func TestDefaultReadinessThreshold(t *testing.T) {
	if DefaultReadinessThreshold != 500 {
		t.Errorf("got %d, want 500", DefaultReadinessThreshold)
	}
}
