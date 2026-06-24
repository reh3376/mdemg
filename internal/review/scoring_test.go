package review

import (
	"math"
	"testing"
)

func ratedRubric() Rubric {
	return Rubric{Version: "gr-v1", Kind: RubricRated, Dimensions: []RubricDimension{
		{Key: "a", Anchors: [5]string{"0", "1", "2", "3", "4"}},
		{Key: "b", Anchors: [5]string{"0", "1", "2", "3", "4"}},
	}}
}

func rankedRubric() Rubric {
	return Rubric{Version: "gr-v1", Kind: RubricRanked}
}

func TestScore_Rated_MeanOverFour(t *testing.T) {
	r := ratedRubric()
	gold, dims, err := r.Score(GradeSubmission{Dimensions: map[string]int{"a": 4, "b": 2}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// mean(4,2)=3 → 3/4 = 0.75
	if math.Abs(gold-0.75) > 1e-9 {
		t.Errorf("gold = %v, want 0.75", gold)
	}
	if dims["a"] != 4 || dims["b"] != 2 || dims["_kind"] != "rated" {
		t.Errorf("dims = %+v", dims)
	}
	// Bounds: all 4 → 1.0; all 0 → 0.0.
	g1, _, _ := r.Score(GradeSubmission{Dimensions: map[string]int{"a": 4, "b": 4}})
	g0, _, _ := r.Score(GradeSubmission{Dimensions: map[string]int{"a": 0, "b": 0}})
	if g1 != 1.0 || g0 != 0.0 {
		t.Errorf("bounds: all-4=%v all-0=%v", g1, g0)
	}
}

func TestScore_Rated_Errors(t *testing.T) {
	r := ratedRubric()
	if _, _, err := r.Score(GradeSubmission{Dimensions: map[string]int{"a": 4}}); err == nil {
		t.Error("missing dimension b should error")
	}
	if _, _, err := r.Score(GradeSubmission{Dimensions: map[string]int{"a": 5, "b": 2}}); err == nil {
		t.Error("out-of-range score should error")
	}
	// Ranked submission to a rated rubric → mismatch.
	if _, _, err := r.Score(GradeSubmission{ChosenAltID: "x", RejectedAltID: "y"}); err == nil {
		t.Error("ranked submission to rated rubric should error")
	}
}

func TestScore_Ranked(t *testing.T) {
	r := rankedRubric()
	// Clear pick (no confidence) → 1.0.
	gold, dims, err := r.Score(GradeSubmission{ChosenAltID: "x", RejectedAltID: "y"})
	if err != nil || gold != 1.0 {
		t.Fatalf("clear pick: gold=%v err=%v", gold, err)
	}
	if dims["chosen"] != "x" || dims["rejected"] != "y" || dims["_kind"] != "ranked" {
		t.Errorf("dims = %+v", dims)
	}
	// Confidence 2 → 0.5 + 0.5*2/4 = 0.75.
	g, _, _ := r.Score(GradeSubmission{ChosenAltID: "x", RejectedAltID: "y", Confidence: 2})
	if math.Abs(g-0.75) > 1e-9 {
		t.Errorf("confidence 2 → %v, want 0.75", g)
	}
	// Errors: same chosen/rejected; rated submission to ranked rubric.
	if _, _, err := r.Score(GradeSubmission{ChosenAltID: "x", RejectedAltID: "x"}); err == nil {
		t.Error("chosen==rejected should error")
	}
	if _, _, err := r.Score(GradeSubmission{Dimensions: map[string]int{"a": 1}}); err == nil {
		t.Error("rated submission to ranked rubric should error")
	}
}

func TestGuidanceRubric_AnchorsNonEmpty(t *testing.T) {
	r := GuidanceRubric("gr-v1")
	if r.Kind != RubricRated || len(r.Dimensions) != 3 {
		t.Fatalf("guidance rubric should be rated w/ 3 dims, got %v / %d", r.Kind, len(r.Dimensions))
	}
	wantDims := map[string]bool{"relevance": true, "actionability": true, "outcome_label_correctness": true}
	for _, d := range r.Dimensions {
		if !wantDims[d.Key] {
			t.Errorf("unexpected dimension %q", d.Key)
		}
		for lvl, a := range d.Anchors {
			if a == "" {
				t.Errorf("dimension %q level %d has an empty anchor", d.Key, lvl)
			}
		}
	}
}
