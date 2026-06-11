package jiminy

import (
	"context"
	"testing"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// mockSignalLearner implements SignalLearnerProvider for testing.
type mockSignalLearner struct {
	emissions []string
	responses []string
}

func (m *mockSignalLearner) RecordEmission(code string) {
	m.emissions = append(m.emissions, code)
}

func (m *mockSignalLearner) RecordResponse(code string) {
	m.responses = append(m.responses, code)
}

func TestGuide_SignalLearnerEmission(t *testing.T) {
	sl := &mockSignalLearner{}
	consultant := &mockConsultant{
		resp: models.SuggestResponse{
			Constraints: []models.Constraint{
				{
					ConstraintType: "must",
					Description:    "always use transactions",
					Confidence:     0.9,
					SourceNodes:    []string{"node-1"},
				},
			},
		},
	}

	cfg := config.Config{
		JiminyMaxItems:      10,
		JiminyMinConfidence: 0.1,
		JiminyTimeoutMs:     5000,
	}
	svc := NewService(cfg, nil, consultant, nil)
	svc.SetSignalLearner(sl)

	_, err := svc.Guide(context.Background(), GuidanceRequest{
		SpaceID: "test",
		Context: "writing database code",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have recorded emissions for each surfaced item
	if len(sl.emissions) == 0 {
		t.Error("expected at least one signal emission")
	}

	// First emission should be "guidance:constraint" (no constraint_code set)
	found := false
	for _, code := range sl.emissions {
		if code == "guidance:constraint" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected emission 'guidance:constraint', got %v", sl.emissions)
	}
}

func TestRecordOutcome_SignalLearnerResponse(t *testing.T) {
	sl := &mockSignalLearner{}
	cfg := config.Config{
		JiminyEffectivenessTTLSec: 300,
	}
	svc := NewService(cfg, nil, nil, nil)
	svc.SetSignalLearner(sl)

	// Pre-track items so RecordOutcome can find them
	items := []GuidanceItem{
		{
			Type:           GuidanceConstraint,
			Content:        "use transactions",
			Confidence:     0.9,
			ConstraintCode: "use-txn",
			SourceNodes:    []string{"node-1"},
		},
	}
	svc.tracker.Track("test-guid-1", items)

	// Record a "followed" outcome — should trigger signal response
	_, err := svc.RecordOutcome(context.Background(), GuidanceFeedbackRequest{
		GuidanceID:    "test-guid-1",
		ActionSummary: "I used transactions everywhere as instructed",
		SpaceID:       "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have recorded a response
	if len(sl.responses) == 0 {
		t.Error("expected at least one signal response for followed outcome")
	}
	found := false
	for _, code := range sl.responses {
		if code == "guidance:use-txn" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected response 'guidance:use-txn', got %v", sl.responses)
	}
}

func TestGuidanceSignalCode(t *testing.T) {
	tests := []struct {
		name     string
		item     GuidanceItem
		expected string
	}{
		{
			name:     "with constraint code",
			item:     GuidanceItem{ConstraintCode: "use-txn", Type: GuidanceConstraint},
			expected: "guidance:use-txn",
		},
		{
			name:     "without constraint code, with type",
			item:     GuidanceItem{Type: GuidanceCorrection},
			expected: "guidance:correction",
		},
		{
			name:     "empty item",
			item:     GuidanceItem{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := guidanceSignalCode(tt.item)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
