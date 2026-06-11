package ape

import (
	"context"
	"fmt"
	"testing"
)

// mockGuidanceCalibrator implements GuidanceCalibrationProvider for testing.
type mockGuidanceCalibrator struct {
	items          []GuidanceEffectivenessItem
	getErr         error
	updateErr      error
	archiveCount   int
	archiveErr     error
	updateCalls    []string // node IDs updated
	updateOutcomes []string // outcomes applied
}

func (m *mockGuidanceCalibrator) GetConstraintEffectiveness(_ context.Context, _ string) ([]GuidanceEffectivenessItem, error) {
	return m.items, m.getErr
}

func (m *mockGuidanceCalibrator) UpdateNodeConfidence(_ context.Context, nodeID string, outcome string) error {
	m.updateCalls = append(m.updateCalls, nodeID)
	m.updateOutcomes = append(m.updateOutcomes, outcome)
	return m.updateErr
}

func (m *mockGuidanceCalibrator) ArchiveStaleConstraints(_ context.Context, _ string) (int, error) {
	return m.archiveCount, m.archiveErr
}

func TestExecuteReviewGuidanceEffectiveness(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	// No calibrator — should fail
	_, err := d.executeReviewGuidanceEffectiveness(context.Background(), "test-space")
	if err == nil {
		t.Fatal("expected error when calibrator is nil")
	}

	// With calibrator
	mock := &mockGuidanceCalibrator{
		items: []GuidanceEffectivenessItem{
			{NodeID: "n1", TotalSurfaced: 10, EffectivenessRate: 0.9}, // high
			{NodeID: "n2", TotalSurfaced: 10, EffectivenessRate: 0.3}, // low
			{NodeID: "n3", TotalSurfaced: 2, EffectivenessRate: 0.0},  // insufficient
			{NodeID: "n4", TotalSurfaced: 8, EffectivenessRate: 0.05}, // low
		},
	}
	d.guidanceCalibrator = mock

	deliverables, err := d.executeReviewGuidanceEffectiveness(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deliverables["total"] != 4 {
		t.Errorf("expected total=4, got %v", deliverables["total"])
	}
	if deliverables["high"] != 1 {
		t.Errorf("expected high=1, got %v", deliverables["high"])
	}
	if deliverables["low"] != 2 {
		t.Errorf("expected low=2, got %v", deliverables["low"])
	}
	if deliverables["insufficient"] != 1 {
		t.Errorf("expected insufficient=1, got %v", deliverables["insufficient"])
	}
}

func TestExecuteAdjustGuidanceConfidence(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	mock := &mockGuidanceCalibrator{
		items: []GuidanceEffectivenessItem{
			{NodeID: "boost-me", TotalSurfaced: 10, EffectivenessRate: 0.8},  // boost (>= 0.7)
			{NodeID: "decay-me", TotalSurfaced: 5, EffectivenessRate: 0.05},  // decay (< 0.1 && >= 5 surfaces)
			{NodeID: "skip-low", TotalSurfaced: 2, EffectivenessRate: 0.0},   // skip (< 3 surfaces)
			{NodeID: "mid-range", TotalSurfaced: 10, EffectivenessRate: 0.4}, // neither boost nor decay
		},
		archiveCount: 1,
	}
	d.guidanceCalibrator = mock

	deliverables, err := d.executeAdjustGuidanceConfidence(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deliverables["boosted"] != 1 {
		t.Errorf("expected boosted=1, got %v", deliverables["boosted"])
	}
	if deliverables["decayed"] != 1 {
		t.Errorf("expected decayed=1, got %v", deliverables["decayed"])
	}
	if deliverables["archived"] != 1 {
		t.Errorf("expected archived=1, got %v", deliverables["archived"])
	}

	// Verify correct node IDs were updated
	if len(mock.updateCalls) != 2 {
		t.Fatalf("expected 2 update calls, got %d", len(mock.updateCalls))
	}
	if mock.updateCalls[0] != "boost-me" || mock.updateOutcomes[0] != "followed" {
		t.Errorf("first update should be boost-me/followed, got %s/%s", mock.updateCalls[0], mock.updateOutcomes[0])
	}
	if mock.updateCalls[1] != "decay-me" || mock.updateOutcomes[1] != "ignored" {
		t.Errorf("second update should be decay-me/ignored, got %s/%s", mock.updateCalls[1], mock.updateOutcomes[1])
	}
}

func TestExecuteArchiveIneffectiveConstraints(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	// No calibrator — should fail
	_, err := d.executeArchiveIneffectiveConstraints(context.Background(), "test-space")
	if err == nil {
		t.Fatal("expected error when calibrator is nil")
	}

	// With calibrator
	mock := &mockGuidanceCalibrator{archiveCount: 3}
	d.guidanceCalibrator = mock

	deliverables, err := d.executeArchiveIneffectiveConstraints(context.Background(), "test-space")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deliverables["archived"] != 3 {
		t.Errorf("expected archived=3, got %v", deliverables["archived"])
	}
}

func TestExecuteAdjustGuidanceConfidence_GetError(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}

	mock := &mockGuidanceCalibrator{
		getErr: fmt.Errorf("db error"),
	}
	d.guidanceCalibrator = mock

	_, err := d.executeAdjustGuidanceConfidence(context.Background(), "test-space")
	if err == nil || err.Error() != "db error" {
		t.Errorf("expected db error, got: %v", err)
	}
}
