package ape

import (
	"testing"
)

func TestLLMReflector_ParseResponse_Valid(t *testing.T) {
	lr := &LLMReflector{}

	raw := `[
		{"pattern_id": "recurring_prune_failure", "severity": "high", "description": "Edge pruning has failed in 3 consecutive cycles", "recommended_action": "prune_excess_edges", "reasoning": "Pattern detected"},
		{"pattern_id": "orphan_stagnation", "severity": "medium", "description": "Orphan ratio unchanged after consolidation", "recommended_action": "trigger_consolidation", "reasoning": "Consolidation ineffective"}
	]`

	insights, err := lr.parseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(insights) != 2 {
		t.Fatalf("expected 2 insights, got %d", len(insights))
	}
	if insights[0].PatternID != "llm:recurring_prune_failure" {
		t.Errorf("expected patternID 'llm:recurring_prune_failure', got %q", insights[0].PatternID)
	}
	if insights[0].Severity != SeverityHigh {
		t.Errorf("expected severity High, got %s", insights[0].Severity)
	}
	if insights[1].RecommendedAction != "trigger_consolidation" {
		t.Errorf("expected action 'trigger_consolidation', got %q", insights[1].RecommendedAction)
	}
}

func TestLLMReflector_ParseResponse_Empty(t *testing.T) {
	lr := &LLMReflector{}

	insights, err := lr.parseResponse("[]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(insights) != 0 {
		t.Errorf("expected 0 insights, got %d", len(insights))
	}
}

func TestLLMReflector_ParseResponse_InvalidSeverity(t *testing.T) {
	lr := &LLMReflector{}

	raw := `[{"pattern_id": "test", "severity": "unknown", "description": "test", "recommended_action": "prune_decayed_edges", "reasoning": "test"}]`

	insights, err := lr.parseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Invalid severity should be skipped
	if len(insights) != 0 {
		t.Errorf("expected 0 insights (invalid severity skipped), got %d", len(insights))
	}
}

func TestLLMReflector_ParseResponse_InvalidAction(t *testing.T) {
	lr := &LLMReflector{}

	raw := `[{"pattern_id": "test", "severity": "high", "description": "test", "recommended_action": "delete_everything", "reasoning": "test"}]`

	insights, err := lr.parseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Invalid action should be skipped
	if len(insights) != 0 {
		t.Errorf("expected 0 insights (invalid action skipped), got %d", len(insights))
	}
}

func TestLLMReflector_ParseResponse_CodeFence(t *testing.T) {
	lr := &LLMReflector{}

	raw := "```json\n[{\"pattern_id\": \"test\", \"severity\": \"low\", \"description\": \"test\", \"recommended_action\": \"prune_decayed_edges\", \"reasoning\": \"test\"}]\n```"

	insights, err := lr.parseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(insights) != 1 {
		t.Errorf("expected 1 insight, got %d", len(insights))
	}
}

func TestLLMReflector_ParseResponse_InvalidJSON(t *testing.T) {
	lr := &LLMReflector{}

	_, err := lr.parseResponse("not json at all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLLMReflector_Disabled(t *testing.T) {
	lr := NewLLMReflector(LLMReflectorConfig{Enabled: false}, nil, nil)

	insights, err := lr.Reflect(nil, &SelfAssessmentReport{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if insights != nil {
		t.Errorf("expected nil insights when disabled, got %v", insights)
	}
}

func TestDeduplicateInsights(t *testing.T) {
	base := []ReflectionInsight{
		{PatternID: "a", RecommendedAction: "prune_decayed_edges"},
		{PatternID: "b", RecommendedAction: "tombstone_stale"},
	}
	llm := []ReflectionInsight{
		{PatternID: "llm:x", RecommendedAction: "prune_decayed_edges"}, // duplicate action
		{PatternID: "llm:y", RecommendedAction: "trigger_consolidation"}, // new action
	}

	merged := deduplicateInsights(base, llm)
	if len(merged) != 3 {
		t.Fatalf("expected 3 insights after merge, got %d", len(merged))
	}

	// Check that the duplicate was skipped and the new one added
	actions := make(map[string]bool)
	for _, i := range merged {
		actions[i.RecommendedAction] = true
	}
	if !actions["trigger_consolidation"] {
		t.Error("expected 'trigger_consolidation' in merged results")
	}
}
