package retrieval

import (
	"math"
	"testing"

	"mdemg/internal/models"
)

// makeCandidates builds a desc-sorted candidate list with the given scores.
// Uses NodeID = "n0", "n1", ... so test assertions can match by ID.
func makeCandidates(scores ...float64) []models.RetrieveResult {
	out := make([]models.RetrieveResult, len(scores))
	for i, s := range scores {
		out[i] = models.RetrieveResult{
			NodeID: idForIndex(i),
			Score:  s,
		}
	}
	return out
}

func idForIndex(i int) string {
	return "n" + string(rune('0'+i))
}

func TestApplySparseGate_Disabled_PassThrough(t *testing.T) {
	in := makeCandidates(10, 9, 8, 7, 6, 5, 4, 3, 2, 1)
	active, dropped, meta := ApplySparseGate(in, SparseGateOpts{
		Enabled:    false,
		Percentile: 0.95,
		MinActive:  3,
		MaxActive:  20,
	})
	if len(active) != 10 {
		t.Fatalf("disabled gate must pass through all candidates; got %d", len(active))
	}
	if len(dropped) != 0 {
		t.Fatalf("disabled gate must drop nothing; got %d", len(dropped))
	}
	if meta.ActiveCount != 10 || meta.DroppedCount != 0 {
		t.Fatalf("metadata mismatch: %+v", meta)
	}
}

func TestApplySparseGate_Empty(t *testing.T) {
	active, dropped, meta := ApplySparseGate(nil, SparseGateOpts{
		Enabled:    true,
		Percentile: 0.95,
		MinActive:  3,
		MaxActive:  20,
	})
	if len(active) != 0 || len(dropped) != 0 {
		t.Fatalf("empty input must return empty active/dropped; got %d/%d", len(active), len(dropped))
	}
	if meta.InputCount != 0 || meta.ActiveCount != 0 {
		t.Fatalf("empty metadata mismatch: %+v", meta)
	}
}

func TestApplySparseGate_PercentileCutoff(t *testing.T) {
	// Scores 1..10. p95 → threshold ≈ 9.55 → only score 10 admits → MIN_ACTIVE=3
	// floor pulls in scores 9 + 8.
	in := makeCandidates(10, 9, 8, 7, 6, 5, 4, 3, 2, 1)
	active, dropped, meta := ApplySparseGate(in, SparseGateOpts{
		Enabled:    true,
		Percentile: 0.95,
		MinActive:  3,
		MaxActive:  20,
	})
	if len(active) != 3 {
		t.Fatalf("p95 + MIN_ACTIVE=3 must keep top 3; got %d", len(active))
	}
	if len(dropped) != 7 {
		t.Fatalf("expected 7 dropped; got %d", len(dropped))
	}
	if !meta.FloorApplied {
		t.Fatalf("MIN_ACTIVE floor should have been applied (p95 alone admits only 1)")
	}
	// Active set should contain the top 3 scores (10, 9, 8). Order may shift
	// during floor backfill but membership is what matters.
	idsActive := make(map[string]bool)
	for _, a := range active {
		idsActive[a.NodeID] = true
	}
	for _, want := range []string{"n0", "n1", "n2"} {
		if !idsActive[want] {
			t.Fatalf("expected NodeID %s in active set; got %v", want, idsActive)
		}
	}
}

func TestApplySparseGate_MaxActiveCeiling(t *testing.T) {
	// Tight homogeneous distribution: all 30 candidates score the same. p95
	// admits all of them, but MAX_ACTIVE=10 caps it.
	scores := make([]float64, 30)
	for i := range scores {
		scores[i] = 5.0
	}
	in := makeCandidates(scores...)
	active, dropped, meta := ApplySparseGate(in, SparseGateOpts{
		Enabled:    true,
		Percentile: 0.95,
		MinActive:  3,
		MaxActive:  10,
	})
	if len(active) != 10 {
		t.Fatalf("MAX_ACTIVE=10 must cap; got %d", len(active))
	}
	if len(dropped) != 20 {
		t.Fatalf("expected 20 dropped; got %d", len(dropped))
	}
	if !meta.CeilingApplied {
		t.Fatalf("MAX_ACTIVE ceiling should have been applied")
	}
}

func TestApplySparseGate_NoFloorWhenPercentileAdmitsEnough(t *testing.T) {
	// p50 on 1..10 → threshold ≈ 5.5 → admits scores 6, 7, 8, 9, 10 = 5
	// items, well above MIN_ACTIVE=3. Floor must not fire.
	in := makeCandidates(10, 9, 8, 7, 6, 5, 4, 3, 2, 1)
	active, _, meta := ApplySparseGate(in, SparseGateOpts{
		Enabled:    true,
		Percentile: 0.50,
		MinActive:  3,
		MaxActive:  20,
	})
	if len(active) != 5 {
		t.Fatalf("p50 must admit top 5; got %d", len(active))
	}
	if meta.FloorApplied {
		t.Fatalf("MIN_ACTIVE floor should NOT have fired (p50 admits 5)")
	}
}

func TestApplySparseGate_Monotonicity(t *testing.T) {
	// Higher percentile must admit ≤ same count for the same input. This is
	// the "more selective gate keeps fewer items" invariant.
	in := makeCandidates(100, 90, 80, 70, 60, 50, 40, 30, 20, 10)
	prevCount := 11 // larger than any possible output to start
	for _, p := range []float64{0.5, 0.7, 0.8, 0.9, 0.95, 0.99} {
		active, _, _ := ApplySparseGate(in, SparseGateOpts{
			Enabled:    true,
			Percentile: p,
			MinActive:  0, // disable floor to test pure percentile monotonicity
			MaxActive:  0, // disable ceiling
		})
		if len(active) > prevCount {
			t.Fatalf("monotonicity violated at p=%.2f: %d > %d", p, len(active), prevCount)
		}
		prevCount = len(active)
	}
}

func TestApplySparseGate_FloorNeverExceedsInput(t *testing.T) {
	// Edge case: MIN_ACTIVE=10 but input has only 3 items. Floor should clamp
	// to len(input), not panic or exceed.
	in := makeCandidates(5, 3, 1)
	active, _, _ := ApplySparseGate(in, SparseGateOpts{
		Enabled:    true,
		Percentile: 0.99,
		MinActive:  10,
		MaxActive:  20,
	})
	if len(active) != 3 {
		t.Fatalf("floor must clamp to input size; got %d", len(active))
	}
}

func TestApplySparseGate_HeavyTailRealistic(t *testing.T) {
	// Realistic per-call distribution from Epic 0 (retrieval.rerank_cross
	// shape, K=42): p50≈2.1, p95≈6.4, p98≈9.3. Synthesize a distribution
	// matching that shape. p98 should give a tiny set (clamp dominates);
	// p95 should give ~2 raw → MIN_ACTIVE=3 floor → 3.
	scores := []float64{
		// Top tail (10 items, scores 5-12)
		12, 10, 9, 8, 7.5, 7, 6.5, 6, 5.5, 5,
		// Bulk (32 items, scores 0.5-4)
		4, 3.8, 3.5, 3.2, 3, 2.8, 2.5, 2.3, 2.1, 2.0,
		1.8, 1.7, 1.5, 1.4, 1.3, 1.2, 1.1, 1.0, 0.95, 0.9,
		0.85, 0.8, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5, 0.45, 0.4,
		0.35, 0.3,
	}
	in := makeCandidates(scores...)

	// p98 with MIN_ACTIVE=3 → 3 items
	active98, _, _ := ApplySparseGate(in, SparseGateOpts{
		Enabled: true, Percentile: 0.98, MinActive: 3, MaxActive: 20,
	})
	if len(active98) != 3 {
		t.Fatalf("realistic p98 + MIN=3 expected 3 items; got %d", len(active98))
	}

	// p95 with MIN_ACTIVE=3 → 3 items (same outcome — clamp dominates)
	active95, _, _ := ApplySparseGate(in, SparseGateOpts{
		Enabled: true, Percentile: 0.95, MinActive: 3, MaxActive: 20,
	})
	if len(active95) != 3 {
		t.Fatalf("realistic p95 + MIN=3 expected 3 items; got %d", len(active95))
	}

	// p70 with MIN_ACTIVE=3 → ~13 items (no floor; pure percentile)
	active70, _, meta70 := ApplySparseGate(in, SparseGateOpts{
		Enabled: true, Percentile: 0.70, MinActive: 3, MaxActive: 20,
	})
	if len(active70) < 10 || len(active70) > 16 {
		t.Fatalf("realistic p70 should give ~13 items; got %d (threshold=%.3f)", len(active70), meta70.Threshold)
	}
}

func TestPercentileLinear_KnownValues(t *testing.T) {
	// R-7 / Excel-default linear interpolation: q*(n-1) index.
	// scores [1, 2, 3, 4, 5] (n=5):
	//   p50  → idx 0.5*4 = 2 → exact element [2] = 3
	//   p25  → idx 0.25*4 = 1 → exact element [1] = 2
	//   p75  → idx 0.75*4 = 3 → exact element [3] = 4
	//   p100 → element [4] = 5
	cases := []struct {
		q    float64
		want float64
	}{
		{0.0, 1},
		{0.25, 2},
		{0.5, 3},
		{0.75, 4},
		{1.0, 5},
	}
	scores := []float64{1, 2, 3, 4, 5}
	for _, c := range cases {
		got := percentileLinear(scores, c.q)
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("p%.2f: want %v got %v", c.q, c.want, got)
		}
	}
}
