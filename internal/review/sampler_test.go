package review

import "testing"

func TestSampler_PrefersUncertainAndDisagreeing(t *testing.T) {
	items := []ReviewItem{
		{ItemID: "confident", AutoScore: 0.95, Stratum: "s"},                                  // low info
		{ItemID: "uncertain", AutoScore: 0.5, Stratum: "s"},                                   // high info (band bonus)
		{ItemID: "disagree", AutoScore: 0.1, Stratum: "s", Signals: map[string]float64{"sim": 0.9}}, // high info (disagreement)
	}
	s := Sampler{SampleSize: 2, UncertaintyBand: 0.4, Seed: 1}
	got := s.Select(items)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	// The confident item must not be among the top 2.
	for _, it := range got {
		if it.ItemID == "confident" {
			t.Errorf("confident (low-info) item should not be selected over uncertain/disagree")
		}
	}
}

func TestSampler_StratumNotStarved(t *testing.T) {
	// Two strata; cap 2 → round-robin must take one from each, not both from one.
	items := []ReviewItem{
		{ItemID: "a1", AutoScore: 0.5, Stratum: "A"},
		{ItemID: "a2", AutoScore: 0.5, Stratum: "A"},
		{ItemID: "b1", AutoScore: 0.5, Stratum: "B"},
	}
	s := Sampler{SampleSize: 2, UncertaintyBand: 0.4, Seed: 7}
	got := s.Select(items)
	strata := map[string]bool{}
	for _, it := range got {
		strata[it.Stratum] = true
	}
	if !strata["A"] || !strata["B"] {
		t.Errorf("both strata should be represented when cap allows, got %+v", got)
	}
}

func TestSampler_DeterministicForFixedSeed(t *testing.T) {
	items := []ReviewItem{
		{ItemID: "a1", AutoScore: 0.5, Stratum: "A"},
		{ItemID: "b1", AutoScore: 0.5, Stratum: "B"},
		{ItemID: "c1", AutoScore: 0.5, Stratum: "C"},
	}
	s := Sampler{SampleSize: 2, UncertaintyBand: 0.4, Seed: 42}
	first := s.Select(items)
	for i := 0; i < 5; i++ {
		again := s.Select(items)
		if len(again) != len(first) {
			t.Fatalf("length varied: %d vs %d", len(again), len(first))
		}
		for j := range first {
			if again[j].ItemID != first[j].ItemID {
				t.Fatalf("non-deterministic for fixed seed: %s vs %s at %d", again[j].ItemID, first[j].ItemID, j)
			}
		}
	}
}

func TestSampler_CapHonoredAndEmpty(t *testing.T) {
	if got := (Sampler{}).Select(nil); got != nil {
		t.Errorf("empty input should return nil")
	}
	items := make([]ReviewItem, 10)
	for i := range items {
		items[i] = ReviewItem{ItemID: string(rune('a' + i)), AutoScore: 0.5, Stratum: "s"}
	}
	got := Sampler{SampleSize: 3, Seed: 1}.Select(items)
	if len(got) != 3 {
		t.Errorf("cap 3 not honored: got %d", len(got))
	}
}
