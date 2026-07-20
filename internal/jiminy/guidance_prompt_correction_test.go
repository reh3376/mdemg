package jiminy

import (
	"strings"
	"testing"
)

// JIMINY-STRUCTURED-CORRECTION-001 Epic 4 — prompt rendering pins.

func TestBuildGuidancePrompt_CorrectionRendersStructuredWhenPresent(t *testing.T) {
	items := []GuidanceItem{
		{
			Type:                GuidanceCorrection,
			Priority:            "high",
			Content:             "CORRECTION: Incorrect: X | Correct: Y | Context: Z",
			Confidence:          0.7,
			SourceNodes:         []string{"node-1"},
			CorrectionIncorrect: "X",
			CorrectionCorrect:   "Y",
			CorrectionContext:   "Z",
		},
	}
	prompt := buildGuidancePrompt(items, "some task", "", 1000, 1000)
	// Should render "Do Y — not X" AND cite the node.
	if !strings.Contains(prompt, "Do Y — not X") {
		t.Errorf("rendered prompt missing structured 'Do Y — not X' form:\n%s", prompt)
	}
	if !strings.Contains(prompt, "(Context: Z)") {
		t.Errorf("rendered prompt missing context clause:\n%s", prompt)
	}
	if !strings.Contains(prompt, "(Node: node-1)") {
		t.Errorf("rendered prompt missing node citation:\n%s", prompt)
	}
	// Should NOT contain the raw joined content anymore (structured replaces it).
	if strings.Contains(prompt, "CORRECTION: Incorrect: X") {
		t.Errorf("structured form present AND raw content — should be one or the other:\n%s", prompt)
	}
}

func TestBuildGuidancePrompt_CorrectionFallsBackToContentWhenNoStructured(t *testing.T) {
	items := []GuidanceItem{
		{
			Type:        GuidanceCorrection,
			Content:     "CORRECTION: Incorrect: OLD | Correct: NEW",
			Confidence:  0.5,
			SourceNodes: []string{"legacy-1"},
			// No CorrectionIncorrect/Correct set — old-format L1 or non-Correct-source.
		},
	}
	prompt := buildGuidancePrompt(items, "task", "", 1000, 1000)
	if !strings.Contains(prompt, "CORRECTION: Incorrect: OLD | Correct: NEW") {
		t.Errorf("fallback to Content missing:\n%s", prompt)
	}
}

func TestBuildGuidancePrompt_CorrectionOnlyCorrectFieldSet(t *testing.T) {
	// If only CorrectionCorrect is set (partial structured), render just "Do Y"
	// without the "— not X" tail; still cleaner than the raw content.
	items := []GuidanceItem{
		{
			Type:              GuidanceCorrection,
			Content:           "raw",
			Confidence:        0.6,
			SourceNodes:       []string{"n"},
			CorrectionCorrect: "always use a dev branch",
		},
	}
	prompt := buildGuidancePrompt(items, "", "", 1000, 1000)
	if !strings.Contains(prompt, "Do always use a dev branch") {
		t.Errorf("missing 'Do' rendering:\n%s", prompt)
	}
	if strings.Contains(prompt, "— not ") {
		t.Errorf("'— not' rendered with no Incorrect field:\n%s", prompt)
	}
}
