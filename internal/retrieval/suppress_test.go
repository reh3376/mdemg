package retrieval

import (
	"reflect"
	"testing"

	"mdemg/internal/models"
)

// Sprint RETRIEVAL-META-DOC-SUPPRESSION-001 (task #143) — Tier 1 pin tests.

func TestSuppressCandidatesByPath_EmptyList(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", Path: "/CHANGELOG.md", RRFScore: 0.9},
		{NodeID: "b", Path: "/other.md", RRFScore: 0.5},
	}
	original := make([]Candidate, len(cands))
	copy(original, cands)

	out := SuppressCandidatesByPath(cands, nil, 0.3)
	if !reflect.DeepEqual(out, original) {
		t.Errorf("empty suppressPaths must be a no-op, got %+v", out)
	}
}

func TestSuppressCandidatesByPath_MatchingPathsMultiplied(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", Path: "/CHANGELOG.md", RRFScore: 0.9},
		{NodeID: "b", Path: "/feature/doc.md", RRFScore: 0.3},
		{NodeID: "c", Path: "/CLAUDE.md", RRFScore: 0.8},
		{NodeID: "d", Path: "/user/cli.md", RRFScore: 0.2},
	}
	out := SuppressCandidatesByPath(cands, []string{"/CHANGELOG.md", "/CLAUDE.md"}, 0.3)

	// Locate by NodeID (order may have changed due to re-sort)
	byID := map[string]Candidate{}
	for _, c := range out {
		byID[c.NodeID] = c
	}

	// Suppressed: a (0.9 * 0.3 = 0.27), c (0.8 * 0.3 = 0.24)
	if got := byID["a"].RRFScore; got != 0.27 {
		t.Errorf("a.RRFScore = %v, want 0.27", got)
	}
	if got := byID["c"].RRFScore; got != 0.24 {
		t.Errorf("c.RRFScore = %v, want 0.24", got)
	}
	// Untouched: b (0.3), d (0.2)
	if got := byID["b"].RRFScore; got != 0.3 {
		t.Errorf("b.RRFScore = %v, want 0.3 (untouched)", got)
	}
	if got := byID["d"].RRFScore; got != 0.2 {
		t.Errorf("d.RRFScore = %v, want 0.2 (untouched)", got)
	}
}

func TestSuppressCandidatesByPath_ResortsCorrectly(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", Path: "/CHANGELOG.md", RRFScore: 0.9},
		{NodeID: "b", Path: "/feature/doc.md", RRFScore: 0.4},
		{NodeID: "c", Path: "/CLAUDE.md", RRFScore: 0.7},
		{NodeID: "d", Path: "/user/cli.md", RRFScore: 0.35},
	}
	// After suppression with factor 0.3:
	//   a: 0.9 * 0.3 = 0.27
	//   b: 0.4 (untouched)
	//   c: 0.7 * 0.3 = 0.21
	//   d: 0.35 (untouched)
	// Sort desc: b(0.4), d(0.35), a(0.27), c(0.21)
	out := SuppressCandidatesByPath(cands, []string{"/CHANGELOG.md", "/CLAUDE.md"}, 0.3)

	wantOrder := []string{"b", "d", "a", "c"}
	gotOrder := make([]string, len(out))
	for i, c := range out {
		gotOrder[i] = c.NodeID
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Errorf("post-suppress order = %v, want %v", gotOrder, wantOrder)
	}
}

func TestSuppressCandidatesByPath_ZeroFactor(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", Path: "/CHANGELOG.md", RRFScore: 0.9},
		{NodeID: "b", Path: "/feature/doc.md", RRFScore: 0.5},
	}
	out := SuppressCandidatesByPath(cands, []string{"/CHANGELOG.md"}, 0.0)

	byID := map[string]Candidate{}
	for _, c := range out {
		byID[c.NodeID] = c
	}
	if got := byID["a"].RRFScore; got != 0.0 {
		t.Errorf("factor=0 on matched path: a.RRFScore = %v, want 0.0", got)
	}
	// But still in the pool
	if _, ok := byID["a"]; !ok {
		t.Error("factor=0 must keep node in pool for downstream traversal, but node a is missing")
	}
	// Untouched cand unchanged
	if got := byID["b"].RRFScore; got != 0.5 {
		t.Errorf("b.RRFScore = %v, want 0.5 (untouched by factor=0 on other path)", got)
	}
}

func TestSuppressCandidatesByPath_IdempotentReRun(t *testing.T) {
	cands := []Candidate{
		{NodeID: "a", Path: "/CHANGELOG.md", RRFScore: 0.9},
		{NodeID: "b", Path: "/feature/doc.md", RRFScore: 0.3},
	}
	// Note: idempotency here means "same input → same output". Re-running
	// the function ON ITS OWN OUTPUT will multiply AGAIN — that's expected
	// arithmetic behavior. The intended usage is once per retrieval call.
	// So we test: two independent calls on separate fresh input slices
	// produce identical results.
	input1 := []Candidate{
		{NodeID: "a", Path: "/CHANGELOG.md", RRFScore: 0.9},
		{NodeID: "b", Path: "/feature/doc.md", RRFScore: 0.3},
	}
	input2 := make([]Candidate, len(input1))
	copy(input2, input1)

	out1 := SuppressCandidatesByPath(input1, []string{"/CHANGELOG.md"}, 0.3)
	out2 := SuppressCandidatesByPath(input2, []string{"/CHANGELOG.md"}, 0.3)

	if !reflect.DeepEqual(out1, out2) {
		t.Errorf("independent runs with same inputs must produce identical outputs\nout1=%+v\nout2=%+v", out1, out2)
	}
	_ = cands
}

// TestSuppressResultsByPath_MatchAndResort — post-scoring twin of the
// pre-fusion candidate suppression. Required because column-voting
// (ScoreAndRankRRF) overwrites cand.RRFScore; only post-scoring
// suppression on []models.RetrieveResult actually reorders the RRF path.
func TestSuppressResultsByPath_MatchAndResort(t *testing.T) {
	results := []models.RetrieveResult{
		{NodeID: "a", Path: "/CHANGELOG.md", Score: 0.8},
		{NodeID: "b", Path: "/feature/doc.md", Score: 0.3},
		{NodeID: "c", Path: "/CLAUDE.md", Score: 0.7},
		{NodeID: "d", Path: "/user/cli.md", Score: 0.35},
	}
	out := SuppressResultsByPath(results, []string{"/CHANGELOG.md", "/CLAUDE.md"}, 0.3)
	// Suppressed: a (0.8 * 0.3 = 0.24), c (0.7 * 0.3 = 0.21)
	// Untouched: b (0.3), d (0.35)
	// Desc order: b(0.3), d(0.35) → wait: d(0.35) > b(0.3), so d first
	// Then: d(0.35), b(0.3), a(0.24), c(0.21)
	wantOrder := []string{"d", "b", "a", "c"}
	gotOrder := make([]string, len(out))
	for i, r := range out {
		gotOrder[i] = r.NodeID
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Errorf("post-suppress result order = %v, want %v", gotOrder, wantOrder)
	}

	byID := map[string]models.RetrieveResult{}
	for _, r := range out {
		byID[r.NodeID] = r
	}
	// Verify actual score values
	if got := byID["a"].Score; got != 0.24 {
		t.Errorf("a.Score = %v, want 0.24 (0.8 * 0.3)", got)
	}
	if got := byID["c"].Score; got != 0.21 {
		t.Errorf("c.Score = %v, want 0.21 (0.7 * 0.3)", got)
	}
	if got := byID["b"].Score; got != 0.3 {
		t.Errorf("b.Score = %v, want 0.3 (untouched)", got)
	}
}

func TestSuppressResultsByPath_EmptyList(t *testing.T) {
	results := []models.RetrieveResult{
		{NodeID: "a", Path: "/CHANGELOG.md", Score: 0.9},
	}
	out := SuppressResultsByPath(results, nil, 0.3)
	if len(out) != 1 || out[0].Score != 0.9 {
		t.Errorf("empty suppress list must be no-op, got %+v", out)
	}
}

func TestSuppressCandidatesByPath_UnitFactor(t *testing.T) {
	// factor=1.0 should still re-sort (in case caller passed unsorted input)
	// but scores unchanged.
	cands := []Candidate{
		{NodeID: "a", Path: "/CHANGELOG.md", RRFScore: 0.9},
		{NodeID: "b", Path: "/feature/doc.md", RRFScore: 0.3},
	}
	out := SuppressCandidatesByPath(cands, []string{"/CHANGELOG.md"}, 1.0)

	byID := map[string]Candidate{}
	for _, c := range out {
		byID[c.NodeID] = c
	}
	if got := byID["a"].RRFScore; got != 0.9 {
		t.Errorf("factor=1.0: a.RRFScore = %v, want 0.9 (unchanged)", got)
	}
	if got := byID["b"].RRFScore; got != 0.3 {
		t.Errorf("factor=1.0: b.RRFScore = %v, want 0.3 (unchanged)", got)
	}
	// Order preserved (highest first)
	if out[0].NodeID != "a" {
		t.Errorf("factor=1.0 post-sort: [0] = %v, want a", out[0].NodeID)
	}
}
