package retrieval

import (
	"os"
	"strings"
	"testing"

	"mdemg/internal/models"
)

// TestBuildRerankPrompt_NamesExpectedScoreCount pins RERANK-LENGTH-STRICT-001
// (2026-08-13): the user prompt MUST end with an explicit count-of-scores
// contract naming N=<candidate count>. Fable HITL pass 2 observed the LLM
// returning 15-cand queries as 16-17-score arrays; naming N in the
// immediate context of the candidates list (bottom of prompt) shifts LLM
// attention to the count constraint at generation time. Regression here →
// the prompt reverts to leaving N implicit, and length-mismatch rates
// climb again.
func TestBuildRerankPrompt_NamesExpectedScoreCount(t *testing.T) {
	cands := []models.RetrieveResult{
		{Name: "a", Path: "/a"},
		{Name: "b", Path: "/b"},
		{Name: "c", Path: "/c"},
	}
	prompt := buildRerankPrompt("q", cands, false)
	// The contract line must name the exact count.
	if !strings.Contains(prompt, "Return exactly 3 scores") {
		t.Errorf("prompt missing count-of-scores contract for len=3; got tail:\n%s", tailLines(prompt, 3))
	}
	// Also try compressed form + a different count.
	prompt2 := buildRerankPrompt("q", cands[:2], true)
	if !strings.Contains(prompt2, "Return exactly 2 scores") {
		t.Errorf("prompt (compress) missing count contract for len=2; got tail:\n%s", tailLines(prompt2, 3))
	}
}

// TestBuildRerankRetryPrompt_NamesBothCounts pins the corrective retry
// contract: the retry prompt MUST name both the previous incorrect count
// and the expected count verbatim.
func TestBuildRerankRetryPrompt_NamesBothCounts(t *testing.T) {
	orig := "Query: foo\n\nCandidates:\n[0] a\n[1] b\n"
	retry := buildRerankRetryPrompt(orig, 2, 5)
	if !strings.Contains(retry, "5 scores but there are 2 candidates") {
		t.Errorf("retry prompt missing verbatim count comparison; got: %s", retry[:min(len(retry), 200)])
	}
	if !strings.Contains(retry, "Return exactly 2 scores") {
		t.Errorf("retry prompt missing corrected-count contract; got: %s", retry[:min(len(retry), 200)])
	}
	if !strings.Contains(retry, orig) {
		t.Error("retry prompt should include the original candidates section verbatim")
	}
}

// TestRerankTemperature_IsZero pins RERANK-LENGTH-STRICT-001's determinism
// contract: reranking is a relevance-scoring task; temperature=0 is
// required for reproducible score arrays across identical inputs. Fable
// HITL pass 2 caught the pre-fix nondeterminism ("same query + candidates
// gives different score arrays across rows"). Regression here → the
// audit-corpus becomes unusable for pattern analysis because identical
// inputs produce different labels.
func TestRerankTemperature_IsZero(t *testing.T) {
	if rerankTemperature != 0.0 {
		t.Errorf("rerankTemperature = %v, want 0.0 (RERANK-LENGTH-STRICT-001 determinism contract)", rerankTemperature)
	}
	// Also assert the completion options in the two LLM-provider paths
	// pass the pointer through. Source-string pin — LLM calls happen
	// under context (ctx-timeout, circuit breakers) and can't be
	// unit-exercised without heavy fake infra.
	b, err := os.ReadFile("rerank.go")
	if err != nil {
		t.Fatalf("read rerank.go: %v", err)
	}
	src := string(b)
	// Both provider paths should thread Temperature=&rerankTemperature.
	tempRefs := strings.Count(src, "Temperature: &rerankTemperature")
	if tempRefs < 2 {
		t.Errorf("expected Temperature: &rerankTemperature in ≥2 provider paths (openai + ollama); found %d", tempRefs)
	}
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
