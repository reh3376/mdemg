package review

import (
	"context"
	"testing"
)

func TestRegistry_RegisterGetList(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(StubDataset{}); err != nil {
		t.Fatalf("register stub: %v", err)
	}
	// Duplicate id rejected.
	if err := r.Register(StubDataset{}); err == nil {
		t.Error("expected duplicate registration to error")
	}
	// nil rejected.
	if err := r.Register(nil); err == nil {
		t.Error("expected nil registration to error")
	}
	d, ok := r.Get("stub")
	if !ok {
		t.Fatal("stub not found after register")
	}
	if d.ID() != "stub" || d.DisplayName() == "" {
		t.Errorf("unexpected dataset identity: %q / %q", d.ID(), d.DisplayName())
	}
	if got := r.List(); len(got) != 1 || got[0].ID() != "stub" {
		t.Errorf("List() = %d datasets, want [stub]", len(got))
	}
	if _, ok := r.Get("nope"); ok {
		t.Error("Get(nope) should be false")
	}
}

func TestStubDataset_RoundTrips(t *testing.T) {
	s := StubDataset{}
	rb := s.Rubric()
	if rb.Kind != RubricRated || len(rb.Dimensions) == 0 {
		t.Fatalf("stub rubric should be rated with dimensions, got kind=%v dims=%d", rb.Kind, len(rb.Dimensions))
	}
	for _, dim := range rb.Dimensions {
		for lvl, a := range dim.Anchors {
			if a == "" {
				t.Errorf("stub dimension %q has an empty anchor at level %d", dim.Key, lvl)
			}
		}
	}
	if _, ok := s.Sink().(NoopSink); !ok {
		t.Errorf("stub sink should be NoopSink, got %T", s.Sink())
	}
	items, err := s.FetchCandidates(context.Background(), CandidateQuery{Limit: 3})
	if err != nil || len(items) != 3 {
		t.Fatalf("FetchCandidates(limit 3) = %d items, err=%v", len(items), err)
	}
	if items[0].ItemID == "" || items[0].Content == "" {
		t.Error("stub item missing id/content")
	}
}

func TestNoopSink_NoMutation(t *testing.T) {
	s := NoopSink{}
	g := Grade{GradeID: "g1"}
	pv, err := s.Preview(context.Background(), g)
	if err != nil || pv.Summary == "" {
		t.Fatalf("noop preview: %v / %q", err, pv.Summary)
	}
	d, err := s.Apply(context.Background(), g)
	if err != nil || d.GradeID != "g1" || d.SinkID != "noop" {
		t.Fatalf("noop apply: %v / %+v", err, d)
	}
	if err := s.Reverse(context.Background(), d); err != nil {
		t.Fatalf("noop reverse: %v", err)
	}
}
