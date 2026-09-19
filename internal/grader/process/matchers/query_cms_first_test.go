// JIMINY-PROCESS-OBSERVER-03 (task #162) — query-cms-first matcher tests.
package matchers

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/grader/process"
)

// The fakePool + fakeRows helpers from sequential_epics_test.go are reused
// (same package). See that file for their contract.

func TestQueryCmsFirst_PriorRetrieval_Followed(t *testing.T) {
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"r_prior"}}}, // mostRecentRetrieval → one row
	}}
	m := NewQueryCmsFirst(300)
	v, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "s1", SpaceID: "mdemg-dev", SessionID: "sess1", Time: time.Now(),
	})
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed, got %q", v.OutcomeType)
	}
	if v.EvidenceEventID != "r_prior" {
		t.Fatalf("want evidence=r_prior, got %q", v.EvidenceEventID)
	}
	if v.ConstraintCode != "query-mdemg-cms-file-paths" {
		t.Fatalf("want constraint code query-mdemg-cms-file-paths, got %q", v.ConstraintCode)
	}
}

func TestQueryCmsFirst_NoPriorRetrieval_Missed(t *testing.T) {
	pool := &fakePool{responses: []fakeRows{
		{rows: nil}, // no prior retrieval on session in window
	}}
	m := NewQueryCmsFirst(300)
	v, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "s1", SpaceID: "mdemg-dev", SessionID: "sess1", Time: time.Now(),
	})
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_missed" {
		t.Fatalf("want process_missed, got %q reason=%q", v.OutcomeType, v.Reason)
	}
	if v.EvidenceEventID != "" {
		t.Fatalf("process_missed should have empty EvidenceEventID; got %q", v.EvidenceEventID)
	}
}

func TestQueryCmsFirst_EmptySessionID_Skip(t *testing.T) {
	pool := &fakePool{responses: []fakeRows{}}
	m := NewQueryCmsFirst(300)
	_, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "s1", SpaceID: "mdemg-dev", SessionID: "", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true when session_id empty")
	}
	// Ensure we never queried the pool
	if pool.calls != 0 {
		t.Fatalf("expected 0 pool calls for empty session_id, got %d", pool.calls)
	}
}

func TestQueryCmsFirst_DefaultWindowFallback(t *testing.T) {
	// Constructor with 0 → falls back to 300s
	m := NewQueryCmsFirst(0)
	if m.WindowSeconds != 300 {
		t.Fatalf("expected WindowSeconds=300 when constructed with 0, got %d", m.WindowSeconds)
	}
	// Negative → also 300
	m2 := NewQueryCmsFirst(-1)
	if m2.WindowSeconds != 300 {
		t.Fatalf("expected WindowSeconds=300 when constructed with -1, got %d", m2.WindowSeconds)
	}
}

func TestQueryCmsFirst_ReasonNamesWindowSec(t *testing.T) {
	// Followed reason must include the window seconds so operators can
	// see the applied threshold in a process_outcomes row.
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"r_id"}}},
	}}
	m := NewQueryCmsFirst(150)
	v, _, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "s1", SpaceID: "mdemg-dev", SessionID: "sess1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !contains(v.Reason, "150s") {
		t.Fatalf("expected reason to name window 150s, got %q", v.Reason)
	}

	// missed reason also names the window
	pool2 := &fakePool{responses: []fakeRows{{rows: nil}}}
	v2, _, err := m.Grade(context.Background(), pool2, process.TerminalEvent{
		EventID: "s1", SpaceID: "mdemg-dev", SessionID: "sess1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !contains(v2.Reason, "150s") {
		t.Fatalf("expected missed reason to name window 150s, got %q", v2.Reason)
	}
}

func TestQueryCmsFirst_Metadata(t *testing.T) {
	m := NewQueryCmsFirst(300)
	if m.Name() != "query_cms_first" {
		t.Fatalf("Name mismatch: %q", m.Name())
	}
	if m.TerminalEventType() != "filesystem_search" {
		t.Fatalf("TerminalEventType mismatch: %q", m.TerminalEventType())
	}
	if m.ConstraintCode() != "query-mdemg-cms-file-paths" {
		t.Fatalf("ConstraintCode mismatch: %q", m.ConstraintCode())
	}
}

// contains is a tiny strings.Contains-alike to avoid an extra import.
func contains(haystack, needle string) bool {
	return len(needle) == 0 ||
		(len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
