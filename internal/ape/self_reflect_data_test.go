package ape

import (
	"context"
	"strings"
	"testing"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/tsdb"
)

// mockDatasetProvider implements tsdb.DatasetProvider for tests.
type mockDatasetProvider struct {
	metricTrends        map[string]*tsdb.MetricTrend
	enforcementOutcomes map[string]tsdb.EnforcementOutcomeCounts // ENFORCE-004-FOLLOWUP
	overrideHistory     []tsdb.OverrideEvent                     // ENFORCE-OVERRIDES-TSDB
}

func (m *mockDatasetProvider) LLMPerformance(_ context.Context, _ string, _ time.Duration) ([]tsdb.LLMPerformanceSummary, error) {
	return nil, nil
}

func (m *mockDatasetProvider) RetrievalQuality(_ context.Context, _ string, _ time.Duration) (*tsdb.RetrievalQualitySummary, error) {
	return nil, nil
}

func (m *mockDatasetProvider) EmbeddingCoverage(_ context.Context, _ string, _ time.Duration) (*tsdb.EmbeddingCoverageSummary, error) {
	return nil, nil
}

func (m *mockDatasetProvider) MetricTrend(_ context.Context, _ string, metricName string, _ time.Duration) (*tsdb.MetricTrend, error) {
	if t, ok := m.metricTrends[metricName]; ok {
		return t, nil
	}
	return &tsdb.MetricTrend{MetricName: metricName}, nil
}

func (m *mockDatasetProvider) TrainingDataReadiness(_ context.Context) (*tsdb.TrainingDataReadiness, error) {
	return nil, nil
}

func (m *mockDatasetProvider) ProductionDrift(_ context.Context) (*tsdb.ProductionDriftSummary, error) {
	return nil, nil
}

func (m *mockDatasetProvider) GuidanceEffectiveness(_ context.Context, _ string, _ time.Duration) (float64, int, error) {
	return 0, 0, nil // JIMINY-SIGNAL-001: mock returns no data → caller uses Neo4j fallback
}

// ENFORCE-004-FOLLOWUP: mock returns empty map by default; per-test EnforcementOutcomes
// override lives in the test that consumes the reflect pattern.
func (m *mockDatasetProvider) EnforcementOutcomes(_ context.Context, _ string, _ time.Duration) (map[string]tsdb.EnforcementOutcomeCounts, error) {
	return m.enforcementOutcomes, nil
}

// ENFORCE-OVERRIDES-TSDB: mock returns nil slice by default.
func (m *mockDatasetProvider) OverrideHistory(_ context.Context, _ string, _ time.Duration) ([]tsdb.OverrideEvent, error) {
	return m.overrideHistory, nil
}

// ─── Pattern 25: LLM Latency Regression ───

func TestReflect_LLMLatencyRegression_Detected(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)
	r.SetDatasetProvider(&mockDatasetProvider{
		metricTrends: map[string]*tsdb.MetricTrend{
			"mdemg_llm_consult_latency_p95": {
				MetricName: "mdemg_llm_consult_latency_p95",
				Slope:      5.0,
				AvgValue:   200.0,
				Points:     []tsdb.MetricPoint{{Value: 200}},
			},
		},
	})

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			{TaskName: "consult", TotalCalls: 50, LatencyP95: 500}, // 500 > 2×200=400
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, i := range insights {
		if i.PatternID == "llm_latency_regression" {
			found = true
			if i.RecommendedAction != "review_llm_provider" {
				t.Errorf("expected action review_llm_provider, got %s", i.RecommendedAction)
			}
		}
	}
	if !found {
		t.Error("llm_latency_regression pattern not triggered with p95=500, avg=200")
	}
}

func TestReflect_LLMLatencyRegression_Normal(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)
	r.SetDatasetProvider(&mockDatasetProvider{
		metricTrends: map[string]*tsdb.MetricTrend{
			"mdemg_llm_consult_latency_p95": {
				MetricName: "mdemg_llm_consult_latency_p95",
				Slope:      1.0,
				AvgValue:   200.0,
				Points:     []tsdb.MetricPoint{{Value: 200}},
			},
		},
	})

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			{TaskName: "consult", TotalCalls: 50, LatencyP95: 300}, // 300 < 2×200=400
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, i := range insights {
		if i.PatternID == "llm_latency_regression" {
			t.Error("llm_latency_regression should not trigger with p95=300, avg=200")
		}
	}
}

// ─── Pattern 26: LLM Error Rate Spike ───

func TestReflect_LLMErrorRateSpike(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			{TaskName: "consult", TotalCalls: 100, ErrorRate: 0.08}, // 8% > 5%
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, i := range insights {
		if i.PatternID == "llm_error_rate_spike" {
			found = true
			if i.Severity != SeverityHigh {
				t.Errorf("expected severity high, got %s", i.Severity)
			}
			if i.RecommendedAction != "alert_llm_health" {
				t.Errorf("expected action alert_llm_health, got %s", i.RecommendedAction)
			}
		}
	}
	if !found {
		t.Error("llm_error_rate_spike not triggered with error rate 8%")
	}
}

func TestReflect_LLMErrorRateSpike_Normal(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			{TaskName: "consult", TotalCalls: 100, ErrorRate: 0.03}, // 3% < 5%
		},
	}
	insights, _ := r.Reflect(context.Background(), report)

	for _, i := range insights {
		if i.PatternID == "llm_error_rate_spike" {
			t.Error("llm_error_rate_spike should not trigger with error rate 3%")
		}
	}
}

// SUPERVISOR-002: recency gate — a high error rate computed over the 24h
// window must not alarm when the most recent error is stale.
func TestReflect_LLMErrorRateSpike_StaleSuppressed(t *testing.T) {
	cfg := config.Config{RSICLLMErrorRecencyMin: 60}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			// 33% errors, but the last one was 12h ago (the 2026-06-11
			// jiminy.synthesize false-critical scenario)
			{TaskName: "jiminy.synthesize", TotalCalls: 36, ErrorRate: 0.33,
				LastErrorAt: time.Now().Add(-12 * time.Hour)},
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, i := range insights {
		if i.PatternID == "llm_error_rate_spike" {
			t.Error("stale error burst should be suppressed by the recency gate")
		}
	}
}

func TestReflect_LLMErrorRateSpike_FreshFires(t *testing.T) {
	cfg := config.Config{RSICLLMErrorRecencyMin: 60}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			{TaskName: "consult", TotalCalls: 100, ErrorRate: 0.08,
				LastErrorAt: time.Now().Add(-5 * time.Minute)},
		},
	}
	insights, _ := r.Reflect(context.Background(), report)
	found := false
	for _, i := range insights {
		if i.PatternID == "llm_error_rate_spike" {
			found = true
		}
	}
	if !found {
		t.Error("fresh error spike must fire through the recency gate")
	}
}

func TestReflect_LLMErrorRateSpike_ZeroLastErrorFiresLegacy(t *testing.T) {
	// A summary without LastErrorAt (older data source) must keep legacy
	// behavior even with the gate enabled — never silently widen the gate.
	cfg := config.Config{RSICLLMErrorRecencyMin: 60}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			{TaskName: "consult", TotalCalls: 100, ErrorRate: 0.08},
		},
	}
	insights, _ := r.Reflect(context.Background(), report)
	found := false
	for _, i := range insights {
		if i.PatternID == "llm_error_rate_spike" {
			found = true
		}
	}
	if !found {
		t.Error("zero LastErrorAt must not be treated as stale")
	}
}

func TestReflect_LLMErrorRateSpike_GateDisabled(t *testing.T) {
	// RSIC_LLM_ERROR_RECENCY_MIN=0 disables the gate (legacy behavior).
	cfg := config.Config{RSICLLMErrorRecencyMin: 0}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		LLMPerformance: []tsdb.LLMPerformanceSummary{
			{TaskName: "consult", TotalCalls: 100, ErrorRate: 0.08,
				LastErrorAt: time.Now().Add(-48 * time.Hour)},
		},
	}
	insights, _ := r.Reflect(context.Background(), report)
	found := false
	for _, i := range insights {
		if i.PatternID == "llm_error_rate_spike" {
			found = true
		}
	}
	if !found {
		t.Error("disabled gate must preserve legacy firing")
	}
}

// ─── Pattern 27: Retrieval Quality Degradation ───

func TestReflect_RetrievalQualityDegradation(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		RetrievalDataset: &tsdb.RetrievalQualitySummary{
			TotalQueries:        50,
			RerankRate:          0.85, // below 90%
			GuidanceCorrelation: 0.75, // below 80%
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rerankFound := false
	guidanceFound := false
	for _, i := range insights {
		if i.PatternID == "retrieval_quality_degradation" {
			if i.Metric == "retrieval_rerank_rate" {
				rerankFound = true
			}
			if i.Metric == "retrieval_guidance_correlation" {
				guidanceFound = true
			}
		}
	}
	if !rerankFound {
		t.Error("retrieval_quality_degradation (rerank) not triggered with rate 85%")
	}
	if !guidanceFound {
		t.Error("retrieval_quality_degradation (guidance) not triggered with correlation 75%")
	}
}

// ─── Pattern 28: Embedding Pipeline Regression ───

func TestReflect_EmbeddingRegression_EmptyCallSites(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		EmbeddingDataset: &tsdb.EmbeddingCoverageSummary{
			TotalEvents:    1000,
			EmptyCallSites: 5,
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, i := range insights {
		if i.PatternID == "embedding_pipeline_regression" {
			found = true
			if i.Severity != SeverityCritical {
				t.Errorf("expected severity critical, got %s", i.Severity)
			}
		}
	}
	if !found {
		t.Error("embedding_pipeline_regression not triggered with 5 empty call_sites")
	}
}

func TestReflect_EmbeddingRegression_NoEmpty(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		EmbeddingDataset: &tsdb.EmbeddingCoverageSummary{
			TotalEvents:    1000,
			EmptyCallSites: 0,
		},
	}
	insights, _ := r.Reflect(context.Background(), report)

	for _, i := range insights {
		if i.PatternID == "embedding_pipeline_regression" {
			t.Error("embedding_pipeline_regression should not trigger with 0 empty call_sites")
		}
	}
}

// ─── Pattern 29: Training Data Ready ───

func TestReflect_TrainingDataReady(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		TrainingReadiness: &tsdb.TrainingDataReadiness{
			Tasks: []tsdb.TaskReadiness{
				{TaskName: "consult", TotalRows: 600, Ready: true},
				{TaskName: "recall", TotalRows: 100, Ready: false},
			},
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, i := range insights {
		if i.PatternID == "training_data_ready" {
			found = true
			if i.RecommendedAction != "trigger_training_pipeline" {
				t.Errorf("expected action trigger_training_pipeline, got %s", i.RecommendedAction)
			}
			if i.Value != 1 {
				t.Errorf("expected 1 ready task, got %.0f", i.Value)
			}
		}
	}
	if !found {
		t.Error("training_data_ready not triggered with 1 ready task")
	}
}

func TestReflect_TrainingDataReady_NoneReady(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		TrainingReadiness: &tsdb.TrainingDataReadiness{
			Tasks: []tsdb.TaskReadiness{
				{TaskName: "consult", TotalRows: 100, Ready: false},
			},
		},
	}
	insights, _ := r.Reflect(context.Background(), report)

	for _, i := range insights {
		if i.PatternID == "training_data_ready" {
			t.Error("training_data_ready should not trigger when no tasks are ready")
		}
	}
}

// ─── Pattern 30: Trust Trajectory Decline ───

func TestReflect_TrustTrajectoryDecline(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)
	r.SetDatasetProvider(&mockDatasetProvider{
		metricTrends: map[string]*tsdb.MetricTrend{
			"mdemg_j17_avg_trust_score": {
				MetricName: "mdemg_j17_avg_trust_score",
				Slope:      -0.05, // declining
				Points: []tsdb.MetricPoint{
					{Value: 0.8}, {Value: 0.7}, {Value: 0.6},
				},
			},
		},
	})

	report := &SelfAssessmentReport{SpaceID: "test"}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, i := range insights {
		if i.PatternID == "trust_trajectory_decline" {
			found = true
			if i.Severity != SeverityMedium {
				t.Errorf("expected severity medium, got %s", i.Severity)
			}
		}
	}
	if !found {
		t.Error("trust_trajectory_decline not triggered with slope -0.05")
	}
}

func TestReflect_TrustTrajectoryDecline_Stable(t *testing.T) {
	cfg := config.Config{}
	r := NewReflector(cfg, nil)
	r.SetDatasetProvider(&mockDatasetProvider{
		metricTrends: map[string]*tsdb.MetricTrend{
			"mdemg_j17_avg_trust_score": {
				MetricName: "mdemg_j17_avg_trust_score",
				Slope:      0.001, // stable/improving
				Points: []tsdb.MetricPoint{
					{Value: 0.8}, {Value: 0.8}, {Value: 0.81},
				},
			},
		},
	})

	report := &SelfAssessmentReport{SpaceID: "test"}
	insights, _ := r.Reflect(context.Background(), report)

	for _, i := range insights {
		if i.PatternID == "trust_trajectory_decline" {
			t.Error("trust_trajectory_decline should not trigger with stable trust")
		}
	}
}

// ─── Pattern 31: Production Drift Detected (DRIFT-TRIGGER-001) ───

// findInsight returns the first insight with the given pattern id (or nil).
func findInsight(insights []ReflectionInsight, patternID string) *ReflectionInsight {
	for i := range insights {
		if insights[i].PatternID == patternID {
			return &insights[i]
		}
	}
	return nil
}

func TestReflect_ProductionDriftDetected_Fires(t *testing.T) {
	cfg := config.Config{FtDriftMargin: 0.05}
	r := NewReflector(cfg, nil)
	r.SetDatasetProvider(&mockDatasetProvider{})
	report := &SelfAssessmentReport{
		SpaceID: "test",
		ProductionDrift: &tsdb.ProductionDriftSummary{
			HasActive: true, HasBench: true,
			ActiveScore: 0.90, LatestBenchScore: 0.80, Delta: 0.10,
		},
	}
	insights, _ := r.Reflect(context.Background(), report)
	got := findInsight(insights, "production_drift_detected")
	if got == nil {
		t.Fatalf("expected production_drift_detected insight; got: %v", insights)
	}
	if got.RecommendedAction != "trigger_training_pipeline" {
		t.Errorf("recommended action must be trigger_training_pipeline (wires to actuator gate); got %q", got.RecommendedAction)
	}
	if got.Value != 0.10 {
		t.Errorf("value=%v want 0.10", got.Value)
	}
	if got.Threshold != 0.05 {
		t.Errorf("threshold must be FtDriftMargin (single source of truth with alert rule); got %v", got.Threshold)
	}
	// Description carries both scores + margin — self-explanatory for operator.
	if !strings.Contains(got.Description, "0.9000") || !strings.Contains(got.Description, "0.8000") {
		t.Errorf("description must include active + bench scores; got: %s", got.Description)
	}
}

// The three suppression conditions: no-active, no-bench, delta below margin.
// Each MUST prevent the insight from firing — spurious fires would open
// unnecessary retrain cycles once the actuator is enabled.
func TestReflect_ProductionDriftDetected_DoesNotFire_NoData(t *testing.T) {
	cfg := config.Config{FtDriftMargin: 0.05}
	r := NewReflector(cfg, nil)
	r.SetDatasetProvider(&mockDatasetProvider{})

	// (a) drift struct absent entirely (dataset provider returned nil)
	report := &SelfAssessmentReport{SpaceID: "test", ProductionDrift: nil}
	insights, _ := r.Reflect(context.Background(), report)
	if findInsight(insights, "production_drift_detected") != nil {
		t.Error("nil ProductionDrift must not fire the pattern")
	}

	// (b) has_active=false — no active model yet (fresh install)
	report = &SelfAssessmentReport{SpaceID: "test", ProductionDrift: &tsdb.ProductionDriftSummary{
		HasActive: false, HasBench: true, LatestBenchScore: 0.80, Delta: 0.90,
	}}
	insights, _ = r.Reflect(context.Background(), report)
	if findInsight(insights, "production_drift_detected") != nil {
		t.Error("has_active=false must not fire (no active model to be drifted against)")
	}

	// (c) has_bench=false — no benchmarks yet
	report = &SelfAssessmentReport{SpaceID: "test", ProductionDrift: &tsdb.ProductionDriftSummary{
		HasActive: true, HasBench: false, ActiveScore: 0.90, Delta: 0.90,
	}}
	insights, _ = r.Reflect(context.Background(), report)
	if findInsight(insights, "production_drift_detected") != nil {
		t.Error("has_bench=false must not fire (no benchmark to measure drift against)")
	}
}

func TestReflect_ProductionDriftDetected_DoesNotFire_BelowMargin(t *testing.T) {
	cfg := config.Config{FtDriftMargin: 0.05}
	r := NewReflector(cfg, nil)
	r.SetDatasetProvider(&mockDatasetProvider{})
	// Delta exactly AT margin → should NOT fire (strict >).
	for _, delta := range []float64{0.0, 0.03, 0.05} {
		report := &SelfAssessmentReport{SpaceID: "test", ProductionDrift: &tsdb.ProductionDriftSummary{
			HasActive: true, HasBench: true,
			ActiveScore: 0.85, LatestBenchScore: 0.85 - delta, Delta: delta,
		}}
		insights, _ := r.Reflect(context.Background(), report)
		if got := findInsight(insights, "production_drift_detected"); got != nil {
			t.Errorf("delta=%v (<=margin) must not fire; got: %+v", delta, got)
		}
	}
}

// TestReflect_ProductionDriftDetected_UsesConfigMargin: pin that the pattern
// reads r.cfg.FtDriftMargin (not a hardcoded literal) — else the pattern and
// the alert rule can drift apart.
func TestReflect_ProductionDriftDetected_UsesConfigMargin(t *testing.T) {
	// Custom stricter margin — same delta must fire under 0.05 but NOT under 0.20.
	tests := []struct {
		margin  float64
		delta   float64
		wantFire bool
	}{
		{0.05, 0.10, true},
		{0.20, 0.10, false},
		{0.20, 0.25, true},
	}
	for _, tc := range tests {
		cfg := config.Config{FtDriftMargin: tc.margin}
		r := NewReflector(cfg, nil)
		r.SetDatasetProvider(&mockDatasetProvider{})
		report := &SelfAssessmentReport{SpaceID: "test", ProductionDrift: &tsdb.ProductionDriftSummary{
			HasActive: true, HasBench: true, ActiveScore: 0.9, LatestBenchScore: 0.9 - tc.delta, Delta: tc.delta,
		}}
		insights, _ := r.Reflect(context.Background(), report)
		got := findInsight(insights, "production_drift_detected") != nil
		if got != tc.wantFire {
			t.Errorf("margin=%v delta=%v: got fire=%v want %v", tc.margin, tc.delta, got, tc.wantFire)
		}
	}
}

// ─── ENFORCE-004-FOLLOWUP: enforcement outcome patterns ───

func TestReflect_EnforcementFalsePositiveHigh_Fires(t *testing.T) {
	// Fires when a constraint has ≥ threshold blocked_false_positive outcomes.
	cfg := config.Config{BlockedFalsePositiveAlertThreshold: 3}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		EnforcementOutcomes: map[string]tsdb.EnforcementOutcomeCounts{
			"OVERRIDDEN-RULE": {BlockedFalsePositive: 5},
			"OK-RULE":         {BlockedFalsePositive: 2}, // below threshold
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	var got *ReflectionInsight
	for i := range insights {
		if insights[i].PatternID == "enforcement_false_positive_high" {
			got = &insights[i]
			break
		}
	}
	if got == nil {
		t.Fatal("enforcement_false_positive_high did not fire for 5 blocked_false_positive")
	}
	if got.RecommendedAction != "archive_ineffective_constraints" {
		t.Errorf("action = %q, want archive_ineffective_constraints", got.RecommendedAction)
	}
	if got.Value != 5 {
		t.Errorf("value = %v, want 5", got.Value)
	}
	// The OK-RULE (count 2) must not fire.
	for _, i := range insights {
		if i.PatternID == "enforcement_false_positive_high" && i.Value < 3 {
			t.Errorf("below-threshold fire on count %v", i.Value)
		}
	}
}

func TestReflect_EnforcementMissedViolationHigh_Fires(t *testing.T) {
	cfg := config.Config{MissedViolationAlertThreshold: 3}
	r := NewReflector(cfg, nil)

	report := &SelfAssessmentReport{
		SpaceID: "test",
		EnforcementOutcomes: map[string]tsdb.EnforcementOutcomeCounts{
			"MISSED-RULE": {MissedViolation: 4},
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	var got *ReflectionInsight
	for i := range insights {
		if insights[i].PatternID == "enforcement_missed_violation_high" {
			got = &insights[i]
			break
		}
	}
	if got == nil {
		t.Fatal("enforcement_missed_violation_high did not fire for 4 missed_violation")
	}
	if got.RecommendedAction != "adjust_guidance_confidence" {
		t.Errorf("action = %q, want adjust_guidance_confidence", got.RecommendedAction)
	}
}

func TestReflect_EnforcementOutcomes_NoDataNoFire(t *testing.T) {
	cfg := config.Config{BlockedFalsePositiveAlertThreshold: 3, MissedViolationAlertThreshold: 3}
	r := NewReflector(cfg, nil)
	report := &SelfAssessmentReport{SpaceID: "test"} // EnforcementOutcomes nil
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range insights {
		if i.PatternID == "enforcement_false_positive_high" || i.PatternID == "enforcement_missed_violation_high" {
			t.Errorf("enforcement pattern fired with no data: %+v", i)
		}
	}
}

func TestReflect_EnforcementOutcomes_ZeroThresholdDisables(t *testing.T) {
	// Config threshold ≤0 → pattern uses safe default 3, not disabled.
	// (Alert rule respects ≤0 = disabled; reflector uses defaults for its
	// per-cycle guard so an unconfigured operator still gets the signal.)
	cfg := config.Config{BlockedFalsePositiveAlertThreshold: 0}
	r := NewReflector(cfg, nil)
	report := &SelfAssessmentReport{
		SpaceID: "test",
		EnforcementOutcomes: map[string]tsdb.EnforcementOutcomeCounts{
			"HOT": {BlockedFalsePositive: 5},
		},
	}
	insights, err := r.Reflect(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range insights {
		if i.PatternID == "enforcement_false_positive_high" {
			found = true
			if i.Threshold != 3 {
				t.Errorf("default threshold should be 3, got %v", i.Threshold)
			}
		}
	}
	if !found {
		t.Error("threshold=0 should fall back to safe default 3, not disable the pattern")
	}
}
