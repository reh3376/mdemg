package jiminy

import (
	"testing"
	"time"
)

func TestBuildDataset_Grading(t *testing.T) {
	now := time.Now()
	snapshot := &ProtocolMetrics{
		TierComprehension: [3]float64{0.95, 0.75, 0.45},
		TierOutcomeCount:  [3]int64{20, 15, 10},
		WindowStart:       now.Add(-time.Hour),
		WindowEnd:         now,
	}

	ds := BuildTierEffectivenessDataset(snapshot, 5, 0.6)
	if ds == nil {
		t.Fatal("expected non-nil dataset")
	}

	// Tier 1 (0.95) → A
	if ds.TierGrades[0].Grade != "A" {
		t.Errorf("T1 grade = %s, want A", ds.TierGrades[0].Grade)
	}
	// Tier 2 (0.75) → C
	if ds.TierGrades[1].Grade != "C" {
		t.Errorf("T2 grade = %s, want C", ds.TierGrades[1].Grade)
	}
	// Tier 3 (0.45) → F
	if ds.TierGrades[2].Grade != "F" {
		t.Errorf("T3 grade = %s, want F", ds.TierGrades[2].Grade)
	}

	if ds.TotalOutcomes != 45 {
		t.Errorf("total outcomes = %d, want 45", ds.TotalOutcomes)
	}
}

func TestBuildDataset_DriftDetection(t *testing.T) {
	snapshot := &ProtocolMetrics{
		TierComprehension: [3]float64{0.4, 0.9, 0},
		TierOutcomeCount:  [3]int64{10, 10, 0},
		TierCodeComprehension: map[int]map[string]float64{
			1: {"code-a": 0.4},
			2: {"code-a": 0.9},
		},
	}

	ds := BuildTierEffectivenessDataset(snapshot, 5, 0.6)
	if ds == nil {
		t.Fatal("expected non-nil dataset")
	}

	if len(ds.CodeDrift) == 0 {
		t.Fatal("expected code drift entries")
	}

	foundDrift := false
	for _, entry := range ds.CodeDrift {
		if entry.Code == "code-a" && entry.DriftDetected {
			foundDrift = true
			if entry.RecommendedTier != 2 {
				t.Errorf("recommended tier = %d, want 2", entry.RecommendedTier)
			}
		}
	}
	if !foundDrift {
		t.Error("expected drift detected for code-a")
	}
}

func TestBuildDataset_Recommendations(t *testing.T) {
	snapshot := &ProtocolMetrics{
		TierComprehension: [3]float64{0.4, 0.9, 0},
		TierOutcomeCount:  [3]int64{10, 10, 0},
		TierCodeComprehension: map[int]map[string]float64{
			1: {"code-a": 0.4},
			2: {"code-a": 0.9},
		},
	}

	ds := BuildTierEffectivenessDataset(snapshot, 5, 0.6)
	if ds == nil {
		t.Fatal("expected non-nil dataset")
	}

	if len(ds.Recommendations) == 0 {
		t.Fatal("expected recommendations")
	}

	rec := ds.Recommendations[0]
	if rec.Code != "code-a" {
		t.Errorf("rec code = %s, want code-a", rec.Code)
	}
	if rec.Action != "demote_to_t2" {
		t.Errorf("rec action = %s, want demote_to_t2", rec.Action)
	}
	if rec.Delta < 0.49 || rec.Delta > 0.51 {
		t.Errorf("rec delta = %f, want ~0.5", rec.Delta)
	}
}

func TestBuildDataset_EmptyMetrics(t *testing.T) {
	ds := BuildTierEffectivenessDataset(nil, 5, 0.6)
	if ds == nil {
		t.Fatal("nil snapshot should return empty dataset, not nil")
	}
	if ds.TotalOutcomes != 0 {
		t.Errorf("total outcomes = %d, want 0", ds.TotalOutcomes)
	}

	empty := &ProtocolMetrics{}
	ds = BuildTierEffectivenessDataset(empty, 5, 0.6)
	if ds == nil {
		t.Fatal("empty snapshot should return dataset, not nil")
	}
}

func TestLetterGrade(t *testing.T) {
	tests := []struct {
		comp  float64
		count int64
		want  string
	}{
		{0.95, 10, "A"},
		{0.90, 10, "A"},
		{0.85, 10, "B"},
		{0.80, 10, "B"},
		{0.75, 10, "C"},
		{0.55, 10, "D"},
		{0.40, 10, "F"},
		{0.0, 0, "N/A"},
	}
	for _, tt := range tests {
		got := letterGrade(tt.comp, tt.count)
		if got != tt.want {
			t.Errorf("letterGrade(%.2f, %d) = %s, want %s", tt.comp, tt.count, got, tt.want)
		}
	}
}
