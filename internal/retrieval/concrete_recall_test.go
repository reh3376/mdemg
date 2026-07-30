package retrieval

import (
	"testing"
)

func TestParseConcreteRoleTypes_Default(t *testing.T) {
	got := parseConcreteRoleTypes("leaf,constraint,correction,conversation_observation")
	want := []string{"leaf", "constraint", "correction", "conversation_observation"}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d]=%q want %q", i, got[i], v)
		}
	}
}

func TestParseConcreteRoleTypes_EmptyReturnsNil(t *testing.T) {
	if got := parseConcreteRoleTypes(""); got != nil {
		t.Errorf("empty string should return nil, got %v", got)
	}
	if got := parseConcreteRoleTypes("   "); got != nil {
		t.Errorf("whitespace-only should return nil, got %v", got)
	}
	if got := parseConcreteRoleTypes(","); got != nil {
		t.Errorf("just-commas should return nil, got %v", got)
	}
}

func TestParseConcreteRoleTypes_DedupAndTrim(t *testing.T) {
	got := parseConcreteRoleTypes(" leaf , constraint,  leaf , correction ")
	want := []string{"leaf", "constraint", "correction"}
	if len(got) != len(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d]=%q want %q", i, got[i], v)
		}
	}
}

func TestMergeConcreteCandidates_EmptyConcrete(t *testing.T) {
	primary := []Candidate{{NodeID: "a"}, {NodeID: "b"}}
	got, added := mergeConcreteCandidates(primary, nil)
	if added != 0 {
		t.Errorf("added=%d want 0", added)
	}
	if len(got) != 2 {
		t.Errorf("len=%d want 2", len(got))
	}
}

func TestMergeConcreteCandidates_AllNewAppended(t *testing.T) {
	primary := []Candidate{{NodeID: "a"}, {NodeID: "b"}}
	concrete := []Candidate{{NodeID: "c"}, {NodeID: "d"}}
	got, added := mergeConcreteCandidates(primary, concrete)
	if added != 2 {
		t.Errorf("added=%d want 2", added)
	}
	if len(got) != 4 {
		t.Fatalf("len=%d want 4", len(got))
	}
	if got[2].NodeID != "c" || got[3].NodeID != "d" {
		t.Errorf("append order wrong: %v", got)
	}
}

func TestMergeConcreteCandidates_DedupPrimaryWins(t *testing.T) {
	// The primary pool's Candidate for "b" carries a real score signal
	// (BM25 + vector). The concrete-recall Candidate for "b" only has
	// VectorSim. Dedup must KEEP the primary version.
	primary := []Candidate{
		{NodeID: "a", VectorSim: 0.5, BM25Score: 2.1},
		{NodeID: "b", VectorSim: 0.6, BM25Score: 3.4},
	}
	concrete := []Candidate{
		{NodeID: "b", VectorSim: 0.99}, // higher sim, no BM25
		{NodeID: "c", VectorSim: 0.4},
	}
	got, added := mergeConcreteCandidates(primary, concrete)
	if added != 1 {
		t.Errorf("added=%d want 1 (only 'c' is new)", added)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	// Slot 1 must still be the primary "b" (BM25Score=3.4 preserved)
	if got[1].NodeID != "b" || got[1].BM25Score != 3.4 {
		t.Errorf("primary 'b' should win on dedup; got %+v", got[1])
	}
}

func TestMergeConcreteCandidates_ConcreteDupWithinItselfHandled(t *testing.T) {
	primary := []Candidate{{NodeID: "a"}}
	concrete := []Candidate{{NodeID: "b"}, {NodeID: "b"}, {NodeID: "c"}}
	got, added := mergeConcreteCandidates(primary, concrete)
	if added != 2 {
		t.Errorf("added=%d want 2 (b,c — second b dropped)", added)
	}
	if len(got) != 3 {
		t.Errorf("len=%d want 3", len(got))
	}
}
