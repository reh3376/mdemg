package hidden

import "testing"

func TestMatchTheme(t *testing.T) {
	existing := []themeRef{
		{NodeID: "t1", Centroid: []float64{1, 0, 0}},
		{NodeID: "t2", Centroid: []float64{0, 1, 0}},
	}
	claimed := map[string]bool{}

	// Exact match clears the threshold.
	if got := matchTheme([]float64{1, 0, 0}, existing, claimed, 0.90); got != "t1" {
		t.Fatalf("exact match: got %q want t1", got)
	}
	// Below-threshold similarity matches nothing.
	if got := matchTheme([]float64{0.7, 0.7, 0.14}, existing, claimed, 0.95); got != "" {
		t.Fatalf("sub-threshold must not match, got %q", got)
	}
	// Claimed themes are skipped (one theme per cluster per run).
	claimed["t1"] = true
	if got := matchTheme([]float64{1, 0, 0}, existing, claimed, 0.90); got != "" {
		t.Fatalf("claimed theme must not rematch, got %q", got)
	}
	// Best of multiple candidates wins.
	existing = append(existing, themeRef{NodeID: "t3", Centroid: []float64{0.99, 0.1, 0}})
	claimed = map[string]bool{}
	if got := matchTheme([]float64{0.995, 0.05, 0}, existing, claimed, 0.90); got != "t3" && got != "t1" {
		t.Fatalf("best candidate expected, got %q", got)
	}
}
