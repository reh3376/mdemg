package retrieval

import (
	"context"
	"testing"
)

func TestJaccardFingerprint(t *testing.T) {
	cases := []struct {
		name string
		a, b []uint16
		want float64
	}{
		{"identical small set", []uint16{1, 5, 10}, []uint16{1, 5, 10}, 1.0},
		{"disjoint", []uint16{1, 2, 3}, []uint16{4, 5, 6}, 0.0},
		{"partial overlap", []uint16{1, 2, 3}, []uint16{2, 3, 4}, 2.0 / 4.0},
		{"single overlap", []uint16{0, 32, 64}, []uint16{32, 100, 200}, 1.0 / 5.0},
		{"empty a", []uint16{}, []uint16{1, 2}, 0.0},
		{"empty b", []uint16{1, 2}, []uint16{}, 0.0},
		{"both empty", []uint16{}, []uint16{}, 0.0},
		{"a subset of b", []uint16{1, 2}, []uint16{1, 2, 3, 4}, 2.0 / 4.0},
		{"large equal sets", []uint16{0, 1, 2, 3, 4, 5}, []uint16{0, 1, 2, 3, 4, 5}, 1.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := JaccardFingerprint(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("JaccardFingerprint(%v, %v) = %f; want %f", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestContextColumn_Disabled(t *testing.T) {
	col := NewContextColumn(false)
	q := ColumnQuery{
		QueryContextFingerprint: []uint16{1, 2, 3},
		Candidates:              []Candidate{{NodeID: "a", ContextFingerprintActive: []uint16{1, 2, 3}}},
	}
	res := col.Run(context.Background(), q)
	if res.Column != "context" {
		t.Errorf("Column = %q; want context", res.Column)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("disabled column should return empty candidates; got %d", len(res.Candidates))
	}
}

func TestContextColumn_EmptyQueryFingerprint(t *testing.T) {
	col := NewContextColumn(true)
	q := ColumnQuery{
		QueryContextFingerprint: nil,
		Candidates:              []Candidate{{NodeID: "a", ContextFingerprintActive: []uint16{1}}},
	}
	res := col.Run(context.Background(), q)
	if len(res.Candidates) != 0 {
		t.Errorf("empty query fingerprint should return empty; got %d", len(res.Candidates))
	}
}

func TestContextColumn_RankByJaccard(t *testing.T) {
	col := NewContextColumn(true)
	cands := []Candidate{
		{NodeID: "no_overlap", ContextFingerprintActive: []uint16{100, 200}},
		{NodeID: "perfect_match", ContextFingerprintActive: []uint16{1, 2, 3}},
		{NodeID: "partial", ContextFingerprintActive: []uint16{1, 2, 999}},
	}
	q := ColumnQuery{
		QueryContextFingerprint: []uint16{1, 2, 3},
		Candidates:              cands,
		TopN:                    3,
	}
	res := col.Run(context.Background(), q)
	if len(res.Candidates) != 3 {
		t.Fatalf("got %d candidates; want 3", len(res.Candidates))
	}
	want := []string{"perfect_match", "partial", "no_overlap"}
	for i, c := range res.Candidates {
		if c.NodeID != want[i] {
			t.Errorf("candidates[%d].NodeID = %q; want %q (got %v)", i, c.NodeID, want[i], res.Candidates)
		}
	}
}

func TestContextColumn_TopNTruncation(t *testing.T) {
	col := NewContextColumn(true)
	cands := []Candidate{
		{NodeID: "match_a", ContextFingerprintActive: []uint16{1, 2, 3}},
		{NodeID: "match_b", ContextFingerprintActive: []uint16{1, 2}},
		{NodeID: "no_match", ContextFingerprintActive: []uint16{99}},
	}
	q := ColumnQuery{
		QueryContextFingerprint: []uint16{1, 2, 3},
		Candidates:              cands,
		TopN:                    2,
	}
	res := col.Run(context.Background(), q)
	if len(res.Candidates) != 2 {
		t.Errorf("TopN=2 → got %d candidates", len(res.Candidates))
	}
}

func TestContextColumn_EmptyCandidates(t *testing.T) {
	col := NewContextColumn(true)
	q := ColumnQuery{
		QueryContextFingerprint: []uint16{1, 2, 3},
		Candidates:              nil,
	}
	res := col.Run(context.Background(), q)
	if len(res.Candidates) != 0 {
		t.Errorf("empty cands → expected empty result; got %d", len(res.Candidates))
	}
}

func TestContextColumn_StableTieBreak(t *testing.T) {
	col := NewContextColumn(true)
	// Two candidates with identical Jaccard scores — should preserve input order.
	cands := []Candidate{
		{NodeID: "first", ContextFingerprintActive: []uint16{1, 2}},
		{NodeID: "second", ContextFingerprintActive: []uint16{1, 2}},
	}
	q := ColumnQuery{
		QueryContextFingerprint: []uint16{1, 2},
		Candidates:              cands,
		TopN:                    2,
	}
	res := col.Run(context.Background(), q)
	if len(res.Candidates) != 2 || res.Candidates[0].NodeID != "first" || res.Candidates[1].NodeID != "second" {
		t.Errorf("stable tie-break failed; got %+v", res.Candidates)
	}
}

func TestContextColumn_Name(t *testing.T) {
	col := NewContextColumn(true)
	if col.Name() != "context" {
		t.Errorf("Name = %q; want context", col.Name())
	}
}

// CONTEXT-LIVE-001: cross-version fingerprints are incomparable — the guard
// must zero them rather than compute Jaccard noise. mdemg-dev had 76,906
// v1-fingerprinted nodes silently compared against v3-derived query bits.
func TestContextColumn_VersionGuard(t *testing.T) {
	col := NewContextColumn(true)
	fp := []uint16{1, 2, 3}
	cands := []Candidate{
		{NodeID: "match-v3", ContextFingerprintActive: []uint16{1, 2, 3}, ContextFingerprintVersion: 3},
		{NodeID: "stale-v1", ContextFingerprintActive: []uint16{1, 2, 3}, ContextFingerprintVersion: 1},
	}

	// Guard on (query version 3): identical bits at v1 must NOT outrank
	// anything — stale candidate scores 0, current candidate scores 1.
	res := col.Run(context.Background(), ColumnQuery{
		QueryContextFingerprint:        fp,
		QueryContextFingerprintVersion: 3,
		Candidates:                     cands,
		TopN:                           2,
	})
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if len(res.Candidates) != 2 || res.Candidates[0].NodeID != "match-v3" {
		t.Fatalf("version guard failed: got order %v", res.Candidates)
	}

	// Guard off (version 0 = unknown, explicit-fingerprint callers):
	// legacy behavior — both score 1.0, upstream order preserved.
	res = col.Run(context.Background(), ColumnQuery{
		QueryContextFingerprint: fp,
		Candidates:              cands,
		TopN:                    2,
	})
	if res.Candidates[0].NodeID != "match-v3" {
		t.Fatalf("version-0 fallback changed ordering: got %v", res.Candidates[0].NodeID)
	}
}
