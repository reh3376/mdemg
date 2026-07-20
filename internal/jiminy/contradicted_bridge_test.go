package jiminy

import (
	"strings"
	"testing"
)

// JIMINY-CONTRADICTED-BRIDGE-001 Epic 4 — bridge helper Tier-1 pins.

func TestHashAction_WhitespaceTolerant(t *testing.T) {
	a := hashAction("gid1", "the  agent  called  a  destructive  op")
	b := hashAction("gid1", "the agent called a destructive op")
	c := hashAction("gid1", "The Agent Called a Destructive Op")
	if a != b {
		t.Errorf("whitespace-only jitter changed hash: %s vs %s", a, b)
	}
	if a != c {
		t.Errorf("case jitter changed hash: %s vs %s", a, c)
	}
	if len(a) != 16 {
		t.Errorf("hash length = %d, want 16 hex chars", len(a))
	}
}

func TestHashAction_DistinctByGuidance(t *testing.T) {
	a := hashAction("gid1", "same action")
	b := hashAction("gid2", "same action")
	if a == b {
		t.Errorf("distinct guidance_id must produce distinct hashes; both = %s", a)
	}
}

func TestClipContent_TruncatesAndTrims(t *testing.T) {
	s := "   " + strings.Repeat("x", 500) + "   "
	got := clipContent(s, 100)
	if len(got) != 100 {
		t.Errorf("clipContent len = %d, want 100", len(got))
	}
	if got != strings.Repeat("x", 100) {
		t.Errorf("clipContent didn't trim leading whitespace before cap")
	}
	if got := clipContent("short", 100); got != "short" {
		t.Errorf("clipContent shortened a within-cap string: %q", got)
	}
	if got := clipContent("anything", 0); got != "anything" {
		t.Errorf("clipContent maxLen=0 must be no-op: %q", got)
	}
}

func TestBuildContradictedDraft_ExpectedShape(t *testing.T) {
	incorrect, correct := buildContradictedDraft("skipped e2e testing before declaring complete", "always run e2e before declaring complete", 500)
	if incorrect == "" || correct == "" {
		t.Errorf("draft fields must be non-empty; got %q / %q", incorrect, correct)
	}
	if incorrect == correct {
		t.Errorf("draft Incorrect and Correct should not be identical")
	}
}

func TestContradictedBridgeCache_LRUDedup(t *testing.T) {
	c := newContradictedBridgeCache(3)
	if c.TrySeen("g1", "h1") {
		t.Error("first insertion returned seen=true")
	}
	if !c.TrySeen("g1", "h1") {
		t.Error("second insertion returned seen=false — dedup broke")
	}
	// Fill to cap; then adding a fourth unique key MUST evict the LRU.
	c.TrySeen("g2", "h2") // [g2, g1]
	c.TrySeen("g3", "h3") // [g3, g2, g1]
	c.TrySeen("g4", "h4") // [g4, g3, g2, g1] → evict back (g1) since cap=3.
	// (g1,h1) was pushed to front on the first insertion and MoveToFront'd on
	// the repeat hit; but g2/g3/g4 have all been pushed to front since, so g1
	// is now the LRU. Verify it's forgotten.
	if c.TrySeen("g1", "h1") {
		t.Error("expected (g1,h1) to have been LRU-evicted after 3 newer pushes; dedup cache didn't age it out")
	}
}

func TestContradictedBridgeCache_EmptyHashIsNotSeen(t *testing.T) {
	c := newContradictedBridgeCache(10)
	if c.TrySeen("g1", "") {
		t.Error("empty action_hash must not be treated as seen (bridge would silently skip legitimate drafts)")
	}
}
