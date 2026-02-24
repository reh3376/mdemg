package consulting

import (
	"context"
	"strings"
	"testing"

	"mdemg/internal/models"
)

// =============================================================================
// buildSynthesisPrompt tests
// =============================================================================

func TestBuildSynthesisPrompt_IncludesQuestion(t *testing.T) {
	req := SynthesisRequest{
		Question: "How does authentication work?",
		Results: []models.RetrieveResult{
			{NodeID: "n1", Name: "auth.go", Score: 0.9, Layer: 0},
		},
	}

	prompt := buildSynthesisPrompt(req)

	if !strings.Contains(prompt, "How does authentication work?") {
		t.Error("prompt should contain the question")
	}
	if !strings.Contains(prompt, "## Question") {
		t.Error("prompt should have Question section header")
	}
}

func TestBuildSynthesisPrompt_IncludesNodeIDs(t *testing.T) {
	req := SynthesisRequest{
		Question: "test?",
		Results: []models.RetrieveResult{
			{NodeID: "node-abc-123", Name: "file.go", Score: 0.8, Layer: 0, Summary: "A file"},
			{NodeID: "node-def-456", Name: "other.go", Score: 0.7, Layer: 1, Summary: "Another"},
		},
	}

	prompt := buildSynthesisPrompt(req)

	if !strings.Contains(prompt, "node-abc-123") {
		t.Error("prompt should contain first node ID")
	}
	if !strings.Contains(prompt, "node-def-456") {
		t.Error("prompt should contain second node ID")
	}
}

func TestBuildSynthesisPrompt_TruncatesLongContext(t *testing.T) {
	longContext := strings.Repeat("x", 5000)
	req := SynthesisRequest{
		Question: "test?",
		Context:  longContext,
		Results: []models.RetrieveResult{
			{NodeID: "n1", Name: "file.go", Score: 0.8, Layer: 0},
		},
	}

	prompt := buildSynthesisPrompt(req)

	// Context should be truncated at 3000 + "..."
	if strings.Contains(prompt, strings.Repeat("x", 3500)) {
		t.Error("prompt should truncate context longer than 3000 chars")
	}
	if !strings.Contains(prompt, "...") {
		t.Error("prompt should include ellipsis for truncated context")
	}
}

func TestBuildSynthesisPrompt_CapsResultsAt15(t *testing.T) {
	results := make([]models.RetrieveResult, 20)
	for i := range results {
		results[i] = models.RetrieveResult{
			NodeID:  "node-" + strings.Repeat("a", 1) + string(rune('A'+i)),
			Name:    "file.go",
			Score:   0.8,
			Layer:   0,
			Summary: "summary",
		}
	}

	req := SynthesisRequest{
		Question: "test?",
		Results:  results,
	}

	prompt := buildSynthesisPrompt(req)

	// Should include [15] but not [16]
	if !strings.Contains(prompt, "[15]") {
		t.Error("prompt should include up to 15 results")
	}
	if strings.Contains(prompt, "[16]") {
		t.Error("prompt should NOT include more than 15 results")
	}
}

func TestBuildSynthesisPrompt_IncludesConcepts(t *testing.T) {
	req := SynthesisRequest{
		Question: "test?",
		Results: []models.RetrieveResult{
			{NodeID: "n1", Name: "file.go", Score: 0.8, Layer: 0},
		},
		Concepts: []models.RelatedConcept{
			{NodeID: "concept-1", Name: "Auth Pattern", Layer: 3, Relevance: 0.9},
		},
	}

	prompt := buildSynthesisPrompt(req)

	if !strings.Contains(prompt, "Auth Pattern") {
		t.Error("prompt should include concept names")
	}
	if !strings.Contains(prompt, "concept-1") {
		t.Error("prompt should include concept node IDs")
	}
	if !strings.Contains(prompt, "Organizational Concepts") {
		t.Error("prompt should have Organizational Concepts section")
	}
}

func TestBuildSynthesisPrompt_HighlightsRisks(t *testing.T) {
	req := SynthesisRequest{
		Question: "test?",
		Results: []models.RetrieveResult{
			{NodeID: "n1", Name: "file.go", Score: 0.8, Layer: 0},
		},
		Suggestions: []models.Suggestion{
			{Type: models.SuggestionRisk, Content: "Deprecated API", Confidence: 0.7},
			{Type: models.SuggestionRisk, Content: "Known bug", Confidence: 0.6},
			{Type: models.SuggestionContext, Content: "Some context", Confidence: 0.5},
		},
	}

	prompt := buildSynthesisPrompt(req)

	if !strings.Contains(prompt, "WARNING: 2 risk signal(s) detected") {
		t.Error("prompt should highlight risk count with WARNING prefix")
	}
	if !strings.Contains(prompt, "Pre-Classified Signals") {
		t.Error("prompt should have Pre-Classified Signals section")
	}
}

// =============================================================================
// LLMSynthesizer.Synthesize behavior tests
// =============================================================================

func TestLLMSynthesizer_Synthesize_Disabled(t *testing.T) {
	s := NewLLMSynthesizer(SynthesisConfig{Enabled: false}, nil)

	_, err := s.Synthesize(context.Background(), SynthesisRequest{
		Question: "test?",
		Results: []models.RetrieveResult{
			{NodeID: "n1", Name: "file.go", Score: 0.8, Layer: 0},
		},
	})

	if err == nil {
		t.Error("expected error when synthesis is disabled")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error should mention disabled, got: %v", err)
	}
}

func TestLLMSynthesizer_Synthesize_EmptyResults(t *testing.T) {
	s := NewLLMSynthesizer(SynthesisConfig{Enabled: true}, nil)

	_, err := s.Synthesize(context.Background(), SynthesisRequest{
		Question: "test?",
		Results:  []models.RetrieveResult{},
	})

	if err == nil {
		t.Error("expected error when results are empty")
	}
	if !strings.Contains(err.Error(), "no results") {
		t.Errorf("error should mention no results, got: %v", err)
	}
}
