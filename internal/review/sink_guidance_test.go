package review

import (
	"context"
	"testing"
)

type fakeReinforcer struct {
	trust map[string]float64
	conf  map[string]float64
}

func newFakeReinforcer() *fakeReinforcer {
	return &fakeReinforcer{trust: map[string]float64{}, conf: map[string]float64{}}
}
func (f *fakeReinforcer) GetTrust(s string) float64 { return f.trust[s] }
func (f *fakeReinforcer) RecordTrust(s, _ string) float64 {
	f.trust[s] += 0.1 // simulate an EMA move
	return f.trust[s]
}
func (f *fakeReinforcer) SetTrust(s string, v float64) { f.trust[s] = v }
func (f *fakeReinforcer) AdjustConfidence(_ context.Context, node string, d float64) error {
	f.conf[node] += d
	return nil
}

func guidanceGrade(correctness int, autoLabel string) Grade {
	return Grade{
		GradeID:        "g1",
		DatasetID:      "guidance",
		ItemID:         "row-1",
		SpaceID:        "mdemg-dev",
		GoldDimensions: map[string]any{"outcome_label_correctness": correctness},
		Item: ReviewItem{
			AutoLabel: autoLabel,
			Meta:      map[string]string{"session_id": "sess-1", "source_node_id": "node-a"},
		},
	}
}

func TestGuidanceSink_ApplyThenReverse_RestoresTrustAndConfidence(t *testing.T) {
	f := newFakeReinforcer()
	f.trust["sess-1"] = 0.40
	f.conf["node-a"] = 0.50
	sink := GuidanceSink{R: f, ConfidenceNudge: 0.05}

	// correctness=4 (auto verdict right), auto=followed → affirm followed.
	detail, err := sink.Apply(context.Background(), guidanceGrade(4, "followed"))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if f.trust["sess-1"] == 0.40 {
		t.Error("trust should have moved on apply")
	}
	if f.conf["node-a"] != 0.55 {
		t.Errorf("confidence should be 0.55 after +0.05 nudge, got %v", f.conf["node-a"])
	}
	if detail.PriorState["trust"] != 0.40 {
		t.Errorf("detail must capture prior trust 0.40, got %v", detail.PriorState["trust"])
	}

	// Reverse restores prior trust + inverse confidence.
	if err := sink.Reverse(context.Background(), detail); err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if f.trust["sess-1"] != 0.40 {
		t.Errorf("reverse should restore trust to 0.40, got %v", f.trust["sess-1"])
	}
	if f.conf["node-a"] != 0.50 {
		t.Errorf("reverse should restore confidence to 0.50, got %v", f.conf["node-a"])
	}
}

func TestGuidanceSink_CorrectedOutcome(t *testing.T) {
	f := newFakeReinforcer()
	sink := GuidanceSink{R: f, ConfidenceNudge: 0.05}
	// correctness<=1 (auto wrong) on auto=ignored → invert to followed.
	d, _ := sink.Apply(context.Background(), guidanceGrade(0, "ignored"))
	if d.Verb != "guidance_outcome:followed" {
		t.Errorf("wrong+ignored should invert to followed, got verb %q", d.Verb)
	}
	// correctness==2 (unclear) → no reinforcement.
	d2, _ := sink.Apply(context.Background(), guidanceGrade(2, "ignored"))
	if d2.Verb != "noop:unclear" {
		t.Errorf("unclear should be a no-op, got verb %q", d2.Verb)
	}
}

func TestGuidanceSink_PreviewMutatesNothing(t *testing.T) {
	f := newFakeReinforcer()
	f.trust["sess-1"] = 0.40
	f.conf["node-a"] = 0.50
	sink := GuidanceSink{R: f, ConfidenceNudge: 0.05}
	pv, err := sink.Preview(context.Background(), guidanceGrade(4, "followed"))
	if err != nil || pv.Summary == "" {
		t.Fatalf("preview: %v / %q", err, pv.Summary)
	}
	if f.trust["sess-1"] != 0.40 || f.conf["node-a"] != 0.50 {
		t.Errorf("preview must not mutate: trust=%v conf=%v", f.trust["sess-1"], f.conf["node-a"])
	}
}

// GuidanceSink satisfies ReinforcementSink.
var _ ReinforcementSink = GuidanceSink{}
