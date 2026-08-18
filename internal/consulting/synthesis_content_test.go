// Sprint INGEST-TOPOLOGY-REPAIR-001 Epic 5 — unit test for the content-render
// branch in buildSynthesisPrompt. When RetrieveResult.Content is present, the
// synthesis prompt MUST include a fenced content block after the Summary line
// so the synthesis LLM can quote verbatim ground truth (EffortLevel values,
// type definitions, exact enum members, etc.).
package consulting

import (
	"strings"
	"testing"

	"mdemg/internal/models"
)

func TestBuildSynthesisPrompt_RendersContentWhenPresent(t *testing.T) {
	req := SynthesisRequest{
		SpaceID:  "test-space",
		Question: "What are the exact EffortLevel values?",
		Context:  "python agent",
		Results: []models.RetrieveResult{
			{
				NodeID:  "n_effort",
				Path:    "claude-docs/agent-sdk--python/023__effortlevel",
				Name:    "`EffortLevel`",
				Summary: "Agent SDK reference - Python — `EffortLevel`",
				Layer:   0,
				Score:   0.8,
				Content: `EffortLevel = Literal["low","medium","high","xhigh","max"]`,
			},
		},
	}
	prompt := buildSynthesisPrompt(req, false)
	if !strings.Contains(prompt, "**Content**:") {
		t.Errorf("prompt missing '**Content**:' section when Content is present:\n%s", prompt)
	}
	if !strings.Contains(prompt, `EffortLevel = Literal["low","medium","high","xhigh","max"]`) {
		t.Errorf("prompt missing verbatim content:\n%s", prompt)
	}
	if !strings.Contains(prompt, "```\n") {
		t.Errorf("prompt missing fenced code block for content:\n%s", prompt)
	}
	// Summary should still be rendered too (both, not either/or)
	if !strings.Contains(prompt, "**Summary**:") {
		t.Errorf("prompt should still render Summary when Content also present:\n%s", prompt)
	}
}

func TestBuildSynthesisPrompt_NoContentSectionWhenContentEmpty(t *testing.T) {
	req := SynthesisRequest{
		SpaceID:  "test-space",
		Question: "q",
		Context:  "c",
		Results: []models.RetrieveResult{
			{
				NodeID:  "n_1",
				Name:    "n",
				Summary: "s",
				Layer:   0,
				Score:   0.5,
				// Content: intentionally empty
			},
		},
	}
	prompt := buildSynthesisPrompt(req, false)
	if strings.Contains(prompt, "**Content**:") {
		t.Errorf("prompt should NOT include Content section when Content is empty:\n%s", prompt)
	}
}

func TestBuildSynthesisPrompt_ContentAndSummaryCoexist(t *testing.T) {
	// Regression pin: both Summary and Content should be present in prompt
	// when Content is set. Ensures the E6 addition didn't accidentally
	// remove Summary rendering.
	req := SynthesisRequest{
		SpaceID:  "test-space",
		Question: "q",
		Results: []models.RetrieveResult{
			{NodeID: "n_a", Summary: "summary_a", Content: "content_a", Layer: 0, Score: 0.5},
		},
	}
	prompt := buildSynthesisPrompt(req, false)
	if !strings.Contains(prompt, "summary_a") {
		t.Errorf("Summary missing from prompt: %s", prompt)
	}
	if !strings.Contains(prompt, "content_a") {
		t.Errorf("Content missing from prompt: %s", prompt)
	}
}
