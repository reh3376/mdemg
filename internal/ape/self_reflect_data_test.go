package ape

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/tsdb"
)

// mockDatasetProvider implements tsdb.DatasetProvider for tests.
type mockDatasetProvider struct {
	metricTrends map[string]*tsdb.MetricTrend
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

func (m *mockDatasetProvider) GuidanceEffectiveness(_ context.Context, _ string, _ time.Duration) (float64, int, error) {
	return 0, 0, nil // JIMINY-SIGNAL-001: mock returns no data → caller uses Neo4j fallback
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
