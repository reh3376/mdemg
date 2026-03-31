package retrieval

import (
	"strings"
	"testing"

	"mdemg/internal/models"
)

func TestBuildRerankPrompt_Compressed(t *testing.T) {
	candidates := []models.RetrieveResult{
		{NodeID: "n1", Name: "auth.go", Path: "internal/auth/auth.go", Summary: "Authentication handler", Score: 0.9},
		{NodeID: "n2", Name: "config.go", Path: "internal/config/config.go", Summary: "Configuration loader", Score: 0.8},
	}

	prompt := buildRerankPrompt("How does auth work?", candidates, true)

	// Compressed mode uses pipe-separated format
	if !strings.Contains(prompt, "|") {
		t.Error("compressed prompt should use pipe-separated format")
	}
	// System framing moved to system role — should NOT appear in user prompt
	if strings.Contains(prompt, "Relevance judge") {
		t.Error("compressed prompt should NOT contain system framing (moved to system role)")
	}
	// Summary should be truncated to 300 chars (these are short so just check presence)
	if !strings.Contains(prompt, "Authentication handler") {
		t.Error("prompt should contain candidate summary")
	}
}

func TestBuildRerankPrompt_SummaryTruncation(t *testing.T) {
	longSummary := strings.Repeat("word ", 100) // 500 chars
	candidates := []models.RetrieveResult{
		{NodeID: "n1", Name: "big.go", Path: "pkg/big.go", Summary: longSummary, Score: 0.9},
	}

	prompt := buildRerankPrompt("test query", candidates, true)

	// The full 500-char summary should NOT appear in the output
	if strings.Contains(prompt, longSummary) {
		t.Error("compressed prompt should truncate summaries longer than 300 chars")
	}
	// The prompt should contain a truncated version (with "..." appended by TruncateAtWord)
	if !strings.Contains(prompt, "...") {
		t.Error("truncated summary should end with '...'")
	}
}

func TestBuildRerankPrompt_BackwardCompat(t *testing.T) {
	candidates := []models.RetrieveResult{
		{NodeID: "n1", Name: "auth.go", Path: "internal/auth/auth.go", Summary: "Auth handler", Score: 0.9},
	}

	prompt := buildRerankPrompt("How does auth work?", candidates, false)

	// System framing is now in rerankCrossSystemPrompt const, not in user prompt
	if strings.Contains(prompt, "Rate how relevant") {
		t.Error("user prompt should NOT contain system framing (moved to system role)")
	}
	// Uncompressed should use multi-line format with indented fields
	if !strings.Contains(prompt, "    Path:") {
		t.Error("uncompressed prompt should contain multi-line format with indented 'Path:'")
	}
	// Should contain the query
	if !strings.Contains(prompt, "Query: How does auth work?") {
		t.Error("prompt should contain the query")
	}
}

func TestRerankSystemPrompts(t *testing.T) {
	// Verify system prompt consts exist and contain expected content
	if !strings.Contains(rerankCrossSystemPrompt, "relevance judge") {
		t.Error("cross system prompt should contain 'relevance judge'")
	}
	if !strings.Contains(rerankNLISystemPrompt, "Relevance judge") {
		t.Error("NLI system prompt should contain 'Relevance judge'")
	}
	if !strings.Contains(rerankCrossSystemPrompt, "JSON array") {
		t.Error("cross system prompt should mention JSON array output format")
	}
}
