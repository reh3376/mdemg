package jiminy

import (
	"context"
	"fmt"
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

// Gap 3: Cache J17 bypass tests

func TestCache_J17_Bypass(t *testing.T) {
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{Name: "rule1", ConstraintType: "must", Description: "Must do X", Confidence: 0.9, SourceNodes: []string{"c1"}},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:        true,
		JiminyTimeoutMs:      5000,
		JiminyMaxItems:       10,
		JiminyMinConfidence:  0.3,
		JiminyCacheEnabled:   true,
		JiminyCacheSize:      100,
		JiminyCacheTTLSec:    300,
		JiminyCacheJ17Bypass: true,
		J17Enabled:           true,
	}

	s := NewService(cfg, nil, consultant, nil)

	// First call with session ID — should not be cached due to J17 bypass
	resp1, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID:   "test",
		Context:   "Same context",
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}
	id1 := resp1.GuidanceID

	// Second call with same context but different session — should get fresh response
	resp2, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID:   "test",
		Context:   "Same context",
		SessionID: "session-2",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	// Should get different guidance IDs (not cached)
	if resp2.GuidanceID == id1 {
		t.Error("expected different guidance IDs (cache bypass), got same")
	}
}

func TestCache_NonJ17_StillCaches(t *testing.T) {
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			SpaceID: "test",
			Constraints: []models.Constraint{
				{Name: "rule1", ConstraintType: "must", Description: "Must do X", Confidence: 0.9, SourceNodes: []string{"c1"}},
			},
		},
	}

	cfg := config.Config{
		JiminyEnabled:        true,
		JiminyTimeoutMs:      5000,
		JiminyMaxItems:       10,
		JiminyMinConfidence:  0.3,
		JiminyCacheEnabled:   true,
		JiminyCacheSize:      100,
		JiminyCacheTTLSec:    300,
		JiminyCacheJ17Bypass: true,
		J17Enabled:           false, // J17 disabled
	}

	s := NewService(cfg, nil, consultant, nil)

	// First call without J17 — should be cached
	resp1, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "Same context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}
	id1 := resp1.GuidanceID

	// Second call — should get cached response
	resp2, err := s.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "Same context",
	})
	if err != nil {
		t.Fatalf("Guide() error = %v", err)
	}

	// Should get same guidance ID (from cache)
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
