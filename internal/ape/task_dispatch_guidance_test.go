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

// AdjustNodeConfidenceDirect records counter-free adjustments (RSIC-VALIDATE-001).
// Deltas map to the legacy outcome labels so existing assertions keep meaning:
// positive → "followed"-equivalent boost, negative → "ignored"-equivalent decay.
func (m *mockGuidanceCalibrator) AdjustNodeConfidenceDirect(_ context.Context, nodeID string, delta float64) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.updateCalls = append(m.updateCalls, nodeID)
	if delta >= 0 {
		m.updateOutcomes = append(m.updateOutcomes, "followed")
	} else {
		m.updateOutcomes = append(m.updateOutcomes, "ignored")
	}
	return nil
}

// ConfidenceCalibrationDeltas returns fixed magnitudes for tests.
func (m *mockGuidanceCalibrator) ConfidenceCalibrationDeltas() (float64, float64) { return 0.05, 0.03 }

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

// CONFIG-DEADFLAG-001: zero-valued threshold fields must resolve to the
// historical literals; SetGuidanceCalibrationThresholds overrides them.
func TestGuidanceCalibrationThresholds_DefaultsAndOverride(t *testing.T) {
	d := &Dispatcher{}
	if got := d.guidanceMinSurfaces(); got != 3 {
		t.Errorf("default min surfaces: expected 3, got %d", got)
	}
	if got := d.guidanceBoostThreshold(); got != 0.7 {
		t.Errorf("default boost threshold: expected 0.7, got %v", got)
	}
	if got := d.guidanceDecayThreshold(); got != 0.1 {
		t.Errorf("default decay threshold: expected 0.1, got %v", got)
	}
	if got := d.guidanceDecayMinSurfaces(); got != 5 {
		t.Errorf("default decay min surfaces: expected 5, got %d", got)
	}

	d.SetGuidanceCalibrationThresholds(7, 0.9, 0.2, 12)
	if got := d.guidanceMinSurfaces(); got != 7 {
		t.Errorf("overridden min surfaces: expected 7, got %d", got)
	}
	if got := d.guidanceBoostThreshold(); got != 0.9 {
		t.Errorf("overridden boost threshold: expected 0.9, got %v", got)
	}
	if got := d.guidanceDecayThreshold(); got != 0.2 {
		t.Errorf("overridden decay threshold: expected 0.2, got %v", got)
	}
	if got := d.guidanceDecayMinSurfaces(); got != 12 {
		t.Errorf("overridden decay min surfaces: expected 12, got %d", got)
	}

	// Non-positive values fall back to the historical defaults.
	d.SetGuidanceCalibrationThresholds(0, -1, 0, -2)
	if d.guidanceMinSurfaces() != 3 || d.guidanceBoostThreshold() != 0.7 ||
		d.guidanceDecayThreshold() != 0.1 || d.guidanceDecayMinSurfaces() != 5 {
		t.Error("non-positive thresholds should fall back to defaults")
	}
}

// CONFIG-DEADFLAG-001: configured thresholds must change executor behavior.
func TestExecuteAdjustGuidanceConfidence_ConfiguredThresholds(t *testing.T) {
	d := &Dispatcher{
		activeTasks: make(map[string]*activeTask),
		reports:     make(map[string][]RSICProgressReport),
	}
	mock := &mockGuidanceCalibrator{
		items: []GuidanceEffectivenessItem{
			{NodeID: "n-boost", TotalSurfaced: 10, EffectivenessRate: 0.85}, // boost only at threshold 0.8
			{NodeID: "n-decay", TotalSurfaced: 8, EffectivenessRate: 0.15},  // decay only at threshold 0.2/minSurfaces 8
			{NodeID: "n-skip", TotalSurfaced: 5, EffectivenessRate: 0.0},    // skipped at minSurfaces 6
		},
	}
	d.guidanceCalibrator = mock
	d.SetGuidanceCalibrationThresholds(6, 0.8, 0.2, 8)

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
	// With the historical defaults (3/0.7/0.1/5), n-skip would NOT be skipped
	// and n-decay (rate 0.15 >= 0.1) would not decay — verify the override took.
	if len(mock.updateCalls) != 2 {
		t.Fatalf("expected 2 update calls, got %d: %v", len(mock.updateCalls), mock.updateCalls)
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
