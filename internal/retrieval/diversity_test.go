package retrieval

import (
	"testing"

	"mdemg/internal/models"
)

// RETRIEVAL-DIVERSITY-001 unit tests.

func mkResults(names ...string) []models.RetrieveResult {
	out := make([]models.RetrieveResult, len(names))
	for i, n := range names {
		out[i] = models.RetrieveResult{NodeID: n + "-id-" + divItoa(i), Name: n, Score: 1.0 - float64(i)*0.1}
	}
	return out
}

func divItoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

func names(rs []models.RetrieveResult) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}

// TestApplyDiversityFilter_ExactNameDedup — primary case from RQA-001:
// input has a duplicate name in the top-K; strict dedup MaxPerName=1
// removes the second occurrence.
func TestApplyDiversityFilter_ExactNameDedup(t *testing.T) {
	in := mkResults("A", "B", "B", "C", "D")
	got := ApplyDiversityFilter(in, 5, DiversityCfg{Enabled: true, MaxPerName: 1})
	if len(got) != 4 {
		t.Fatalf("expected 4 results (one B dropped), got %d", len(got))
	}
	want := []string{"A", "B", "C", "D"}
	for i, n := range want {
		if got[i].Name != n {
			t.Errorf("[%d] name = %q, want %q; full: %v", i, got[i].Name, n, names(got))
		}
	}
}

// TestApplyDiversityFilter_SafetyNet_MinOutput — pathological all-same-name.
// Default MinOutput=1: dedup collapses to 1 (correct — that's the only
// diverse result). Operators who prefer completeness can raise MinOutput.
func TestApplyDiversityFilter_SafetyNet_MinOutput(t *testing.T) {
	in := mkResults("A", "A", "A", "A", "A")
	// Default (MinOutput=1): correct diverse output = 1.
	got := ApplyDiversityFilter(in, 3, DiversityCfg{Enabled: true, MaxPerName: 1})
	if len(got) != 1 {
		t.Fatalf("default MinOutput=1 should keep only 1 diverse result; got %d names=%v", len(got), names(got))
	}
	// With MinOutput=3, back-fill kicks in to reach 3 (accepts duplicates).
	got2 := ApplyDiversityFilter(in, 3, DiversityCfg{Enabled: true, MaxPerName: 1, MinOutput: 3})
	if len(got2) != 3 {
		t.Fatalf("MinOutput=3 should back-fill to 3; got %d", len(got2))
	}
}

// TestApplyDiversityFilter_DisabledPassthrough — Enabled=false is a no-op
// even under conditions that would trigger dedup.
func TestApplyDiversityFilter_DisabledPassthrough(t *testing.T) {
	in := mkResults("A", "A", "B")
	got := ApplyDiversityFilter(in, 3, DiversityCfg{Enabled: false, MaxPerName: 1})
	if len(got) != 3 {
		t.Fatalf("disabled must be no-op; got %d results", len(got))
	}
	if got[1].Name != "A" {
		t.Errorf("disabled must not reorder or drop; got %v", names(got))
	}
}

// TestApplyDiversityFilter_EmptyName — results with empty Name bypass
// dedup entirely (treated as always-diverse). Under default (MinOutput=1),
// the second A dedup-drops → output = 3 (2 empty + 1 A).
func TestApplyDiversityFilter_EmptyName(t *testing.T) {
	in := mkResults("", "", "A", "A")
	got := ApplyDiversityFilter(in, 4, DiversityCfg{Enabled: true, MaxPerName: 1})
	if len(got) != 3 {
		t.Fatalf("expected 3 (2 empty-name kept + 1 A kept, second A dedup-dropped); got %d names=%v", len(got), names(got))
	}
	// Verify empty-name results are kept — dedup does NOT apply to them.
	emptyCount := 0
	for _, r := range got {
		if r.Name == "" {
			emptyCount++
		}
	}
	if emptyCount != 2 {
		t.Errorf("both empty-name results should be kept; got %d empty in %v", emptyCount, names(got))
	}
}

// TestApplyDiversityFilter_MaxPerName_2 — MaxPerName=2 keeps 2 of the same
// name before dropping. Config-tunable strength.
func TestApplyDiversityFilter_MaxPerName_2(t *testing.T) {
	in := mkResults("A", "A", "A", "B", "C")
	got := ApplyDiversityFilter(in, 5, DiversityCfg{Enabled: true, MaxPerName: 2})
	if len(got) != 4 {
		t.Fatalf("expected 4 (2 A kept + B + C; third A dropped), got %d names=%v", len(got), names(got))
	}
	want := []string{"A", "A", "B", "C"}
	for i, n := range want {
		if got[i].Name != n {
			t.Errorf("[%d] want %q got %q; full: %v", i, n, got[i].Name, names(got))
		}
	}
}

// TestApplyDiversityFilter_InputShorterThanTopK — dedup STILL runs when
// input is already smaller than topK. The design intent is "prefer diverse
// coverage over completeness" — 1 diverse result is better than 3 duplicates,
// even if the caller asked for topK=5.
func TestApplyDiversityFilter_InputShorterThanTopK(t *testing.T) {
	in := mkResults("A", "A", "A")
	got := ApplyDiversityFilter(in, 5, DiversityCfg{Enabled: true, MaxPerName: 1})
	if len(got) != 1 {
		t.Fatalf("dedup should collapse all-A input to 1; got %d names=%v", len(got), names(got))
	}
}

// TestApplyDiversityFilter_ZeroMaxDefaults — MaxPerName <= 0 defaults to 1
// (strict dedup) for safety.
func TestApplyDiversityFilter_ZeroMaxDefaults(t *testing.T) {
	in := mkResults("A", "A", "B", "C", "D")
	got := ApplyDiversityFilter(in, 5, DiversityCfg{Enabled: true, MaxPerName: 0})
	if len(got) != 4 {
		t.Fatalf("MaxPerName=0 should default to 1 (strict); expected 4 got %d", len(got))
	}
}
