package api

import (
	"os"
	"strings"
	"testing"
)

// TestLLMDataset_FetchCandidates_ExcludesErrorAndEmptyRows pins
// REVIEW-CANDIDATES-EXCLUDE-ERROR-ROWS-001 (2026-08-13): the llm:*
// dataset FetchCandidates query MUST filter out (a) rows tagged with the
// 'caller_canceled:' error prefix from LLM-HEALTH-INVESTIGATION-001 (the
// recorder's convention for HTTP-client-cancellation, which the
// alert-rule ALREADY filters out of the error-rate signal), and (b) rows
// with empty or NULL response text (nothing to grade). Pre-fix, both
// classes surfaced in the candidate queue and wasted per-item grader
// reasoning cost. Regression here → the queue re-includes ungradeable
// artifacts + grading agents burn tokens on non-content.
func TestLLMDataset_FetchCandidates_ExcludesErrorAndEmptyRows(t *testing.T) {
	b, err := os.ReadFile("llm_dataset.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	src := string(b)
	start := strings.Index(src, "func (d llmCallSiteDataset) FetchCandidates(")
	if start < 0 {
		t.Fatal("FetchCandidates not found in llm_dataset.go")
	}
	end := strings.Index(src[start:], "\n}\n")
	if end < 0 {
		t.Fatal("could not find end of FetchCandidates")
	}
	body := src[start : start+end]

	required := []string{
		"caller_canceled:",   // exclusion pattern for LLM cancellations
		"NOT LIKE",           // the exclusion form
		"i.response IS NOT NULL",
		"LENGTH(i.response) > 0",
	}
	for _, req := range required {
		if !strings.Contains(body, req) {
			t.Errorf("FetchCandidates SQL missing required predicate %q — REVIEW-CANDIDATES-EXCLUDE-ERROR-ROWS-001 regression", req)
		}
	}
}
