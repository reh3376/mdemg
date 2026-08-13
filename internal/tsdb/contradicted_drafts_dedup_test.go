package tsdb

import (
	"os"
	"strings"
	"testing"
)

func readSourceFile(name string) (string, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// TestFetchPendingBySpace_SQLIncludesGradeDedup pins REVIEW-CANDIDATES-DEDUP-409-001
// (2026-08-13): the FetchPendingBySpace query MUST LEFT JOIN review_grades
// on (dataset_id='contradicted_drafts', item_id=d.id, rubric_version, NOT
// reversed) and filter r.item_id IS NULL. Pre-fix, drafts reviewed by an
// auto-grader whose sink refused (non-reinforceable verdict per
// HITL-CURATION-002 invariant) kept re-surfacing in the candidate queue
// forever because draft.status stayed 'pending'. Regression here → the
// contradicted_drafts queue re-serves the same items on every fetch and
// /v1/review/grade 409s on the idempotency contract.
//
// Query is inspected by string-search rather than executed against a real
// pool because the LEFT JOIN correctness requires Postgres SQL semantics
// that the fake pool doesn't emulate. The source-string pin is sufficient
// to catch the "somebody removed the JOIN" regression class.
func TestFetchPendingBySpace_SQLIncludesGradeDedup(t *testing.T) {
	// Read the writer source via the compiled query — build a minimal
	// probe by scanning the method body for the required predicates.
	src, err := readSourceFile("contradicted_drafts_writer.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	// Locate the FetchPendingBySpace function body.
	start := strings.Index(src, "func (w *ContradictedDraftsWriter) FetchPendingBySpace(")
	if start < 0 {
		t.Fatal("FetchPendingBySpace not found in contradicted_drafts_writer.go")
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not find end of FetchPendingBySpace")
	}
	body := src[start : start+end]

	required := []string{
		"LEFT JOIN review_grades",
		"dataset_id = 'contradicted_drafts'",
		"r.reversed = FALSE",
		"r.item_id IS NULL",
	}
	for _, req := range required {
		if !strings.Contains(body, req) {
			t.Errorf("FetchPendingBySpace SQL missing required predicate %q — REVIEW-CANDIDATES-DEDUP-409-001 regression", req)
		}
	}
}
