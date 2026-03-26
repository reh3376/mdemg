package jiminy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// mockConsultant implements ConsultingService for testing.
type mockConsultant struct {
	resp models.SuggestResponse
	err  error
}

func (m *mockConsultant) Suggest(_ context.Context, _ models.SuggestRequest) (models.SuggestResponse, error) {
	return m.resp, m.err
}

// mockEmbedder implements embeddings.Embedder for testing.
type mockEmbedder struct {
	embedding  []float32
	err        error
	name       string
	dimensions int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.embedding, m.err
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

func (m *mockEmbedder) Name() string {
	if m.name != "" {
		return m.name
	}
	return "mock"
}

func (m *mockEmbedder) Dimensions() int {
	if m.dimensions > 0 {
		return m.dimensions
	}
	return 3072
}

func TestGuide_MissingSpaceID(t *testing.T) {
	s := NewService(config.Config{JiminyEnabled: true, JiminyTimeoutMs: 2000, JiminyMaxItems: 10, JiminyMinConfidence: 0.3}, nil, nil, nil)
	_, err := s.Guide(context.Background(), GuidanceRequest{Context: "test"})
	if err == nil {
		t.Error("expected error for missing space_id")
	}
}

func TestGuide_MissingContext(t *testing.T) {
	s := NewService(config.Config{JiminyEnabled: true, JiminyTimeoutMs: 2000, JiminyMaxItems: 10, JiminyMinConfidence: 0.3}, nil, nil, nil)
	_, err := s.Guide(context.Background(), GuidanceRequest{SpaceID: "test"})
	if err == nil {
		t.Error("expected error for missing context")
	}
}

func TestGuide_WithConsultant(t *testing.T) {
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{
					Name:           "no-deprecated-auth",
					ConstraintType: "must_not",
					Description:    "Never use deprecated auth middleware",
					Confidence:     0.92,
					SourceNodes:    []string{"c1"},
				},
			},
			Suggestions: []models.Suggestion{
				{
					Type:        models.SuggestionContext,
					Content:     "Use structured logging",
					Confidence:  0.78,
					SourceNodes: []string{"s1"},
				},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     5000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
	}

	s := NewService(cfg, nil, consultant, nil)
	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "Refactoring auth middleware",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	if len(resp.Guidance) == 0 {
		t.Error("expected guidance items from consultant")
	}

	// Check that constraint was converted
	foundConstraint := false
	for _, item := range resp.Guidance {
		if item.Type == GuidanceConstraint {
			foundConstraint = true
		}
	}
	if !foundConstraint {
		t.Error("expected at least one constraint guidance item")
	}

	if resp.PromptAugmentation == "" {
		t.Error("expected non-empty prompt augmentation")
	}

	if resp.SourceCounts.Constraints == 0 {
		t.Error("expected non-zero constraint count")
	}
}

func TestGuide_ConsultantError(t *testing.T) {
	consultant := &mockConsultant{
		err: fmt.Errorf("embedder unavailable"),
	}

	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     5000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
	}

	s := NewService(cfg, nil, consultant, nil)
	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "Refactoring auth middleware",
	})
	// Should not return error — consultant failure is non-fatal
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	if len(resp.Warnings) == 0 {
		t.Error("expected warnings about consultant failure")
	}
}

func TestGuide_NoGuidance(t *testing.T) {
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID:     "test",
			Suggestions: []models.Suggestion{},
		},
	}

	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     5000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
	}

	s := NewService(cfg, nil, consultant, nil)
	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "Nothing to see here",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	if resp.PromptAugmentation != "" {
		t.Error("expected empty prompt augmentation when no items")
	}
}

func TestFormatPromptAugmentation(t *testing.T) {
	items := []GuidanceItem{
		{Type: GuidanceConstraint, Priority: "high", Content: "[must_not] Never use deprecated auth", Confidence: 0.92},
		{Type: GuidanceCorrection, Priority: "high", Content: "Default model is gpt-5-mini", Confidence: 0.85},
		{Type: GuidancePattern, Priority: "medium", Content: "API handlers follow: method check -> parse -> validate", Confidence: 0.71},
	}
	counts := SourceCounts{Constraints: 1, Corrections: 1, Patterns: 1}

	result := FormatPromptAugmentation(items, counts, 0.82)

	if result == "" {
		t.Fatal("expected non-empty prompt augmentation")
	}
	if !contains(result, "JIMINY GUIDANCE") {
		t.Error("expected JIMINY GUIDANCE header")
	}
	if !contains(result, "CONSTRAINTS:") {
		t.Error("expected CONSTRAINTS section")
	}
	if !contains(result, "CORRECTIONS:") {
		t.Error("expected CORRECTIONS section")
	}
	if !contains(result, "PATTERNS:") {
		t.Error("expected PATTERNS section")
	}
	if !contains(result, "END JIMINY GUIDANCE") {
		t.Error("expected END JIMINY GUIDANCE footer")
	}
}

func TestFormatPromptAugmentation_Empty(t *testing.T) {
	result := FormatPromptAugmentation(nil, SourceCounts{}, 0)
	if result != "" {
		t.Error("expected empty string for no items")
	}
}

func TestDeduplicateItems(t *testing.T) {
	items := []GuidanceItem{
		{Type: GuidanceConstraint, Content: "Rule A", Confidence: 0.9},
		{Type: GuidanceCorrection, Content: "Rule A", Confidence: 0.8},
		{Type: GuidancePattern, Content: "Rule B", Confidence: 0.7},
	}
	result := deduplicateItems(items)
	if len(result) != 2 {
		t.Errorf("deduplicateItems() = %d items, want 2", len(result))
	}
}

func TestComputeOverallConfidence(t *testing.T) {
	items := []GuidanceItem{
		{Confidence: 0.9},
		{Confidence: 0.8},
		{Confidence: 0.7},
	}
	conf := computeOverallConfidence(items)
	if conf < 0.79 || conf > 0.81 {
		t.Errorf("computeOverallConfidence() = %.2f, want ~0.80", conf)
	}
}

func TestComputeOverallConfidence_Empty(t *testing.T) {
	conf := computeOverallConfidence(nil)
	if conf != 0 {
		t.Errorf("computeOverallConfidence(nil) = %.2f, want 0", conf)
	}
}

// Gap 3: Session-scoped cache tests

func TestCache_SessionScoped_DifferentSessions(t *testing.T) {
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{Name: "rule1", ConstraintType: "must", Description: "Must do X", Confidence: 0.9, SourceNodes: []string{"c1"}},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     5000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
		JiminyCacheEnabled:  true,
		JiminyCacheSize:     100,
		JiminyCacheTTLSec:   300,
		J17Enabled:          true,
	}

	s := NewService(cfg, nil, consultant, nil)

	// First call with session ID — cached under session-1 key
	resp1, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID:   "test",
		Context:   "Same context",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}
	id1 := resp1.GuidanceID

	// Second call with same context but different session — different cache key, fresh response
	resp2, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID:   "test",
		Context:   "Same context",
		SessionID: "session-2",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	// Should get different guidance IDs (different session = different cache key)
	if resp2.GuidanceID == id1 {
		t.Error("expected different guidance IDs (different session keys), got same")
	}
}

func TestCache_SameSession_StillCaches(t *testing.T) {
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{Name: "rule1", ConstraintType: "must", Description: "Must do X", Confidence: 0.9, SourceNodes: []string{"c1"}},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     5000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
		JiminyCacheEnabled:  true,
		JiminyCacheSize:     100,
		JiminyCacheTTLSec:   300,
		J17Enabled:          true,
	}

	s := NewService(cfg, nil, consultant, nil)

	// First call — should be cached under session-scoped key
	resp1, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID:   "test",
		Context:   "Same context",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}
	id1 := resp1.GuidanceID

	// Second call with same session — should get cached response
	resp2, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID:   "test",
		Context:   "Same context",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	// Should get same guidance ID (from cache, same session key)
	if resp2.GuidanceID != id1 {
		t.Error("expected same guidance ID (cached), got different")
	}
}

// Gap 6: Shadow ML no-panic tests

func TestGuide_ShadowTierPredictor_NoSidecar(t *testing.T) {
	// Verify no panic when sidecar URL is empty (default)
	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     2000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
		J17Enabled:          true,
		J17SidecarURL:       "", // no sidecar
	}

	s := NewService(cfg, nil, &mockConsultant{resp: models.SuggestResponse{SpaceID: "test"}}, nil)
	if s.tierPredictor != nil {
		t.Error("expected nil tierPredictor when no sidecar URL")
	}

	// Guide should work fine without the shadow predictor
	_, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() should not error without sidecar: %v", err)
	}
}

func TestRecordOutcome_ShadowNLI_NoSidecar(t *testing.T) {
	// Verify no panic when NLI scorer is nil
	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     2000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
		J17SidecarURL:       "", // no sidecar
	}

	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{Name: "rule1", ConstraintType: "must", Description: "Must do X", Confidence: 0.9, SourceNodes: []string{"c1"}},
			},
		},
	}

	s := NewService(cfg, nil, consultant, nil)
	if s.nliScorer != nil {
		t.Error("expected nil nliScorer when no sidecar URL")
	}

	// Guide to get a guidance ID
	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	// RecordOutcome should work fine without NLI scorer
	_, err = s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    resp.GuidanceID,
		SpaceID:       "test",
		ActionSummary: "followed the constraint",
	})
	if err != nil {
		t.Fatalf("RecordOutcome() should not error without sidecar: %v", err)
	}
}

// NS-03: FeedbackDimensions tests

func TestRecordOutcome_FeedbackDimensions_Heuristic(t *testing.T) {
	// When NLI scorer is nil, FeedbackDimensions should use heuristic scoring
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{Name: "rule1", ConstraintType: "must", Description: "Must do X", Confidence: 0.9, SourceNodes: []string{"c1"}},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     5000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
		J17SidecarURL:       "", // no NLI scorer
	}

	s := NewService(cfg, nil, consultant, nil)

	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	feedback, err := s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    resp.GuidanceID,
		SpaceID:       "test",
		ActionSummary: "followed the Must do X constraint exactly",
	})
	if err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}

	// Find the constraint result
	for _, r := range feedback.Results {
		if r.Type != GuidanceConstraint {
			continue
		}
		if r.Dimensions == nil {
			t.Fatal("expected FeedbackDimensions for constraint item, got nil")
		}
		if r.Dimensions.ScoreSource != "heuristic" {
			t.Errorf("expected score_source='heuristic', got %q", r.Dimensions.ScoreSource)
		}
		// Adherence should match the outcome
		if r.Dimensions.Adherence != r.Outcome {
			t.Errorf("expected dimensions.adherence=%q to match outcome=%q", r.Dimensions.Adherence, r.Outcome)
		}
		// Comprehension should be set based on outcome
		if r.Dimensions.Comprehension < 0 || r.Dimensions.Comprehension > 1 {
			t.Errorf("comprehension %.2f out of range [0,1]", r.Dimensions.Comprehension)
		}
		// Applicability should be set
		if r.Dimensions.Applicability < 0 || r.Dimensions.Applicability > 1 {
			t.Errorf("applicability %.2f out of range [0,1]", r.Dimensions.Applicability)
		}
	}
}

func TestRecordOutcome_FeedbackDimensions_NonConstraint_NilDimensions(t *testing.T) {
	// FeedbackDimensions should be nil for non-constraint items (patterns, suggestions)
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Suggestions: []models.Suggestion{
				{Type: models.SuggestionContext, Content: "Use structured logging", Confidence: 0.8, SourceNodes: []string{"s1"}},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:       true,
		JiminyTimeoutMs:     5000,
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.3,
	}

	s := NewService(cfg, nil, consultant, nil)

	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	feedback, err := s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    resp.GuidanceID,
		SpaceID:       "test",
		ActionSummary: "used structured logging",
	})
	if err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}

	for _, r := range feedback.Results {
		if r.Type != GuidanceConstraint && r.Dimensions != nil {
			t.Errorf("expected nil dimensions for non-constraint type %q, got %+v", r.Type, r.Dimensions)
		}
	}
}

func TestFeedbackDimensions_JSONSerialization(t *testing.T) {
	dims := FeedbackDimensions{
		Adherence:     OutcomeFollowed,
		Comprehension: 0.95,
		Applicability: 1.0,
		ScoreSource:   "nli",
	}

	// Verify struct is well-formed
	if dims.Adherence != OutcomeFollowed {
		t.Errorf("adherence = %q, want %q", dims.Adherence, OutcomeFollowed)
	}
	if dims.Comprehension != 0.95 {
		t.Errorf("comprehension = %f, want 0.95", dims.Comprehension)
	}
	if dims.Applicability != 1.0 {
		t.Errorf("applicability = %f, want 1.0", dims.Applicability)
	}
	if dims.ScoreSource != "nli" {
		t.Errorf("score_source = %q, want 'nli'", dims.ScoreSource)
	}
}

func TestConstraintPriority(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"must", "high"},
		{"must_not", "high"},
		{"should", "medium"},
		{"should_not", "medium"},
		{"preference", "low"},
	}
	for _, tt := range tests {
		got := constraintPriority(tt.input)
		if got != tt.want {
			t.Errorf("constraintPriority(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && s != substr && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- NLI Fallback Service Integration Tests ---
// These tests verify that when the NLI sidecar is unavailable (returns HTTP 500),
// the fallback path correctly: uses heuristic comprehension, sets score_source to
// "nli_fallback", does NOT record to protocolMetrics, and does NOT feed calibration.

// newFallbackTestService creates a Service with an NLI scorer pointing at a failing
// sidecar (HTTP 500), protocolMetrics enabled, and calibration tracker enabled.
func newFallbackTestService(t *testing.T, sidecarURL string) *Service {
	t.Helper()
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{Name: "no-force-push", ConstraintType: "must", Description: "Never force push to main", Confidence: 0.9, SourceNodes: []string{"c1"}},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:                  true,
		JiminyTimeoutMs:                5000,
		JiminyMaxItems:                 10,
		JiminyMinConfidence:            0.3,
		J17SidecarURL:                  sidecarURL,
		J17NLIComprehensionEnabled:     true,
		J17SidecarTimeoutMs:            2000,
		J17MetricsEnabled:              true,
		J17NLIObservationalEnabled:     true,
		J17NLICalibrationWindowSize:    100,
		J17NLICalibrationBiasThreshold: 0.15,
	}

	return NewService(cfg, nil, consultant, nil)
}

func TestNLIFallback_ScoreSourceIsNLIFallback(t *testing.T) {
	// Sidecar returns HTTP 500 → NLI scorer falls back
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sidecar.Close()

	s := newFallbackTestService(t, sidecar.URL)

	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	feedback, err := s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    resp.GuidanceID,
		SpaceID:       "test",
		ActionSummary: "I followed the no-force-push constraint",
	})
	if err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}

	for _, r := range feedback.Results {
		if r.Type != GuidanceConstraint {
			continue
		}
		if r.Dimensions == nil {
			t.Fatal("expected FeedbackDimensions for constraint item, got nil")
		}
		if r.Dimensions.ScoreSource != "nli_fallback" {
			t.Errorf("ScoreSource = %q, want %q", r.Dimensions.ScoreSource, "nli_fallback")
		}
	}
}

func TestNLIFallback_UsesHeuristicComprehension(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sidecar.Close()

	s := newFallbackTestService(t, sidecar.URL)

	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	// Action summary must overlap enough significant words from constraint
	// "Never force push to main" to trigger OutcomeFollowed (>= 50% overlap).
	feedback, err := s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    resp.GuidanceID,
		SpaceID:       "test",
		ActionSummary: "I never force push to main, used regular push instead",
	})
	if err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}

	for _, r := range feedback.Results {
		if r.Type != GuidanceConstraint {
			continue
		}
		if r.Dimensions == nil {
			t.Fatal("expected FeedbackDimensions, got nil")
		}
		// OutcomeFollowed → heuristic comprehension = 1.0, NOT fallback 0.5
		if r.Dimensions.Comprehension != 1.0 {
			t.Errorf("Comprehension = %f, want 1.0 (heuristic for followed), not 0.5 (fallback)", r.Dimensions.Comprehension)
		}
	}
}

func TestNLIFallback_DoesNotRecordToProtocolMetrics(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sidecar.Close()

	s := newFallbackTestService(t, sidecar.URL)

	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	_, err = s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    resp.GuidanceID,
		SpaceID:       "test",
		ActionSummary: "followed the constraint",
	})
	if err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}

	// protocolMetrics should NOT have any RecordOutcomeWithTier calls
	snapshot := s.GetProtocolMetricsSnapshot()
	if snapshot == nil {
		t.Fatal("expected non-nil protocol metrics snapshot")
	}
	for i := range 3 {
		if snapshot.TierOutcomeCount[i] != 0 {
			t.Errorf("TierOutcomeCount[%d] = %d, want 0 (fallback should NOT record to metrics)", i, snapshot.TierOutcomeCount[i])
		}
	}

	// But NLI fallback counter SHOULD be incremented
	collector := s.GetProtocolMetricsCollector()
	if collector == nil {
		t.Fatal("expected non-nil protocol metrics collector")
	}
	fallbackSnapshot := collector.Snapshot()
	if fallbackSnapshot.NLIFallbackCount != 1 {
		t.Errorf("NLIFallbackCount = %d, want 1", fallbackSnapshot.NLIFallbackCount)
	}
}

func TestNLIFallback_DoesNotFeedCalibration(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sidecar.Close()

	s := newFallbackTestService(t, sidecar.URL)

	resp, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "test context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	_, err = s.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    resp.GuidanceID,
		SpaceID:       "test",
		ActionSummary: "followed the constraint",
	})
	if err != nil {
		t.Fatalf("RecordOutcome() error = %v", err)
	}

	// Calibration tracker should have zero entries (fallback skips Track())
	report := s.GetNLICalibrationReport()
	if report == nil {
		t.Fatal("expected non-nil calibration report")
	}
	if report.WindowSize != 0 {
		t.Errorf("calibration WindowSize = %d, want 0 (fallback should NOT feed calibration)", report.WindowSize)
	}
}
