package retrieval

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/config"
	"mdemg/internal/models"
)

// LLM-HEALTH-INVESTIGATION-001 E2 test — rerank pre-check skips when the
// caller's ctx deadline is closer than RerankMinBudgetMs. Verifies the
// fail-open shape (pre-rerank candidates returned unchanged; no error).

func newRerankTestService(minBudgetMs int) *Service {
	// Just enough config to reach the pre-check. The dispatch wouldn't run
	// anyway (no llama-server in the test), but RerankEnabled needs to be
	// true for the outer Rerank() method to proceed to the pre-check.
	return &Service{cfg: config.Config{
		RerankEnabled:     true,
		RerankProvider:    "openai",
		RerankTopN:        5,
		RerankTimeoutMs:   30000,
		RerankMinBudgetMs: minBudgetMs,
	}}
}

func TestRerank_PreCheckSkipsWhenInsufficientBudget(t *testing.T) {
	s := newRerankTestService(12000)
	// Caller ctx has 200ms remaining — well under the 12s min budget.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	cands := []models.RetrieveResult{
		{NodeID: "a", Score: 0.9},
		{NodeID: "b", Score: 0.7},
		{NodeID: "c", Score: 0.5},
	}
	res, err := s.Rerank(ctx, RerankRequest{
		SpaceID:    "s",
		Query:      "test",
		Candidates: cands,
		TopN:       3,
	})
	if err != nil {
		t.Fatalf("pre-check skip must be fail-open, got err: %v", err)
	}
	if res == nil {
		t.Fatal("expected fail-open RerankResult, got nil")
	}
	if len(res.Results) != len(cands) {
		t.Errorf("skip result Results len = %d, want %d (unchanged)", len(res.Results), len(cands))
	}
	// Ordering preserved.
	for i, c := range res.Results {
		if c.NodeID != cands[i].NodeID {
			t.Errorf("skip result [%d] node_id = %q, want %q", i, c.NodeID, cands[i].NodeID)
		}
	}
	if res.LatencyMs != 0 {
		t.Errorf("skip result LatencyMs = %v, want 0", res.LatencyMs)
	}
}

func TestRerank_PreCheckAllowsWhenSufficientBudget(t *testing.T) {
	// Very small min budget; caller ctx has plenty of room. Pre-check must
	// PROCEED past the skip guard (it will then fail on the llama-server
	// dispatch, but that's outside the pre-check contract). We only assert
	// that the returned error is NOT the fail-open shape.
	s := newRerankTestService(50)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cands := []models.RetrieveResult{{NodeID: "a", Score: 0.9}}
	res, err := s.Rerank(ctx, RerankRequest{
		SpaceID:    "s",
		Query:      "test",
		Candidates: cands,
		TopN:       1,
	})
	// Either err != nil (dispatch failed as expected — no llama-server here)
	// OR res.LatencyMs > 0 (dispatch completed but this test env can't reach
	// that path). Both are valid; what MUST be false is "skip fired" — i.e.,
	// the result must NOT be the immediate fail-open zero-latency shape with
	// a nil error.
	if err == nil && res != nil && res.LatencyMs == 0 && len(res.Results) == len(cands) {
		t.Error("pre-check skipped when caller had sufficient budget — false positive")
	}
}

func TestRerank_PreCheckBypassedWhenNoDeadline(t *testing.T) {
	// context.Background() has no deadline — the pre-check must bypass
	// (nothing to guard against; a CLI direct call or a background job has
	// no caller cancellation risk).
	s := newRerankTestService(12000)
	cands := []models.RetrieveResult{{NodeID: "a", Score: 0.9}}
	res, err := s.Rerank(context.Background(), RerankRequest{
		SpaceID:    "s",
		Query:      "test",
		Candidates: cands,
		TopN:       1,
	})
	// Same as sufficient-budget: pre-check must NOT fire; dispatch will
	// then fail (no llama-server). The MUST-NOT is "skip shape returned".
	if err == nil && res != nil && res.LatencyMs == 0 && len(res.Results) == len(cands) {
		t.Error("pre-check skipped when deadline was absent — bypass broken")
	}
}

func TestRerank_PreCheckDisabledByZeroBudget(t *testing.T) {
	// RerankMinBudgetMs=0 disables the pre-check entirely. Even a nearly-
	// expired ctx must NOT trigger the skip.
	s := newRerankTestService(0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cands := []models.RetrieveResult{{NodeID: "a", Score: 0.9}}
	res, err := s.Rerank(ctx, RerankRequest{
		SpaceID: "s", Query: "test", Candidates: cands, TopN: 1,
	})
	if err == nil && res != nil && res.LatencyMs == 0 && len(res.Results) == len(cands) {
		t.Error("pre-check skipped when RerankMinBudgetMs=0 — disable-guard broken")
	}
}
