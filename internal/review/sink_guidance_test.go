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

// HITL-CURATION-003: GuidanceSink also satisfies NonReinforcingApplier.
var _ NonReinforcingApplier = GuidanceSink{}

// TestGuidanceSink_ApplyNonReinforcing_OnlyUnclearIsHandled pins the load-
// bearing gating from HITL-CURATION-003: dims where Apply would mutate
// substrate (<=1 invert, >=3 affirm) MUST return handled=false so the row
// stays operator-actionable via FetchCandidates. Only dim==2 (unclear, where
// Apply itself returns "noop:unclear") is drainable.
func TestGuidanceSink_ApplyNonReinforcing_OnlyUnclearIsHandled(t *testing.T) {
	sink := GuidanceSink{R: newFakeReinforcer(), ConfidenceNudge: 0.05}
	for _, dim := range []int{0, 1, 3, 4} {
		d, ok, err := sink.ApplyNonReinforcing(context.Background(), guidanceGrade(dim, "followed"))
		if err != nil {
			t.Errorf("dim=%d: unexpected err %v", dim, err)
		}
		if ok {
			t.Errorf("dim=%d: mutation-worthy verdict must return ok=false, got ok=true detail=%+v", dim, d)
		}
	}
	d, ok, err := sink.ApplyNonReinforcing(context.Background(), guidanceGrade(2, "followed"))
	if err != nil {
		t.Fatalf("dim=2 unexpected err %v", err)
	}
	if !ok {
		t.Fatal("dim=2 (unclear) should return handled=true so operator queue drains")
	}
	if d.Verb != "guidance:autograde:noop:unclear" {
		t.Errorf("dim=2 verb should be 'guidance:autograde:noop:unclear', got %q", d.Verb)
	}
	if d.SinkID != "guidance" {
		t.Errorf("dim=2 SinkID should be 'guidance', got %q", d.SinkID)
	}
	if d.GradeID != "g1" {
		t.Errorf("dim=2 GradeID should be 'g1' from fixture, got %q", d.GradeID)
	}
}

// TestGuidanceSink_ApplyNonReinforcing_ZeroSubstrateMutation confirms the
// unclear-verdict handled path touches neither trust EMA nor confidence.
// This is the HITL-CURATION-002 invariant applied to guidance: auto-grades
// never mutate the running substrate.
func TestGuidanceSink_ApplyNonReinforcing_ZeroSubstrateMutation(t *testing.T) {
	f := newFakeReinforcer()
	f.trust["sess-1"] = 0.42
	f.conf["node-a"] = 0.55
	sink := GuidanceSink{R: f, ConfidenceNudge: 0.05}
	_, ok, err := sink.ApplyNonReinforcing(context.Background(), guidanceGrade(2, "followed"))
	if err != nil || !ok {
		t.Fatalf("dim=2 should be handled: ok=%v err=%v", ok, err)
	}
	if f.trust["sess-1"] != 0.42 {
		t.Errorf("trust must NOT move on non-reinforcing apply, got %v (was 0.42)", f.trust["sess-1"])
	}
	if f.conf["node-a"] != 0.55 {
		t.Errorf("confidence must NOT move on non-reinforcing apply, got %v (was 0.55)", f.conf["node-a"])
	}
}

// TestGuidanceSink_Apply_UnchangedByHITLCuration003 is a regression pin —
// the reinforce=true path must be byte-identical to pre-HITL-CURATION-003
// behavior. Extending ApplyNonReinforcing must not change Apply's outputs.
func TestGuidanceSink_Apply_UnchangedByHITLCuration003(t *testing.T) {
	f := newFakeReinforcer()
	f.trust["sess-1"] = 0.30
	f.conf["node-a"] = 0.60
	sink := GuidanceSink{R: f, ConfidenceNudge: 0.05}
	d, err := sink.Apply(context.Background(), guidanceGrade(4, "followed"))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if d.Verb != "guidance_outcome:followed" {
		t.Errorf("verb regression: got %q", d.Verb)
	}
	if f.trust["sess-1"] != 0.40 {
		t.Errorf("trust EMA move regression: got %v (expected +0.10)", f.trust["sess-1"])
	}
	if f.conf["node-a"] != 0.65 {
		t.Errorf("confidence nudge regression: got %v (expected +0.05)", f.conf["node-a"])
	}
	if d.PriorState["trust"] != 0.30 {
		t.Errorf("PriorState.trust regression: got %v", d.PriorState["trust"])
	}
}

// TestGuidanceSink_Reverse_AutogradeNoop_IsNoop pins Reverse safety for
// ApplyNonReinforcing detail: with empty PriorState (no trust key, no
// confidence_node key), Reverse must return nil without any adapter call.
// This ensures a future operator-triggered reversal of an auto-graded
// (never-reinforced) grade is safe.
func TestGuidanceSink_Reverse_AutogradeNoop_IsNoop(t *testing.T) {
	f := newFakeReinforcer()
	f.trust["sess-1"] = 0.20
	f.conf["node-a"] = 0.70
	sink := GuidanceSink{R: f, ConfidenceNudge: 0.05}
	detail := ReinforcementDetail{
		SinkID:     "guidance",
		GradeID:    "g1",
		Verb:       "guidance:autograde:noop:unclear",
		PriorState: map[string]any{},
		Applied:    map[string]any{},
	}
	if err := sink.Reverse(context.Background(), detail); err != nil {
		t.Fatalf("reverse of noop detail should be nil-err, got %v", err)
	}
	if f.trust["sess-1"] != 0.20 {
		t.Errorf("reverse of noop detail must not touch trust, got %v", f.trust["sess-1"])
	}
	if f.conf["node-a"] != 0.70 {
		t.Errorf("reverse of noop detail must not touch confidence, got %v", f.conf["node-a"])
	}
}
