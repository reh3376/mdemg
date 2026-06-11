package ape

import (
	"context"
	"strings"
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

	insights, err := lr.Reflect(context.TODO(), &SelfAssessmentReport{})
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
		{PatternID: "llm:x", RecommendedAction: "prune_decayed_edges"},   // duplicate action
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

func TestBuildUserPrompt_Compressed(t *testing.T) {
	lr := &LLMReflector{
		cfg: LLMReflectorConfig{CompressPrompts: true},
	}

	report := &SelfAssessmentReport{
		SpaceID:          "test-space",
		RetrievalQuality: 0.8,
		MemoryHealth:     0.9,
	}

	prompt := lr.buildUserPrompt(report)

	// Compressed mode uses CompactJSON (single-line), so no indented JSON
	if strings.Contains(prompt, "  \"") {
		t.Error("compressed prompt should NOT contain indented JSON (double-space + quote)")
	}
	if !strings.Contains(prompt, "## Current Assessment") {
		t.Error("prompt should still contain the Current Assessment header")
	}
}

func TestBuildUserPrompt_BackwardCompat(t *testing.T) {
	lr := &LLMReflector{
		cfg: LLMReflectorConfig{CompressPrompts: false},
	}

	report := &SelfAssessmentReport{
		SpaceID:          "test-space",
		RetrievalQuality: 0.8,
		MemoryHealth:     0.9,
	}

	prompt := lr.buildUserPrompt(report)

	// Uncompressed mode uses MarshalIndent, so should contain indented JSON
	if !strings.Contains(prompt, "  \"") {
		t.Error("uncompressed prompt should contain indented JSON (double-space + quote)")
	}
}

func TestLLMReflector_ValidActions_Has20Entries(t *testing.T) {
	if len(validActions) != 20 {
		t.Errorf("expected 20 valid actions, got %d", len(validActions))
	}

	expected := []string{
		"prune_decayed_edges", "prune_excess_edges", "tombstone_stale",
		"graduate_volatile", "trigger_consolidation", "refresh_stale_edges",
		"codify_constraint", "codify_all_constraints", "retire_code",
		"adjust_tier_threshold", "adjust_replay_buffer",
		"review_guidance_effectiveness", "adjust_guidance_confidence",
		"archive_ineffective_constraints", "flush_recovery_buffer",
		"review_nli_calibration",
		// Diagnostic actions (expanded whitelist)
		"ingest_stale_spaces", "alert_jiminy_critical",
		"alert_memory_bloat", "alert_synergy_overlap",
	}
	for _, a := range expected {
		if !validActions[a] {
			t.Errorf("expected %s in validActions", a)
		}
	}
}

func TestAllowedLLMActions_MatchesValidActionsMap(t *testing.T) {
	// Verify AllowedLLMActions and validActions are in sync
	if len(AllowedLLMActions) != len(validActions) {
		t.Errorf("AllowedLLMActions (%d) != validActions (%d)", len(AllowedLLMActions), len(validActions))
	}
	for _, a := range AllowedLLMActions {
		if !validActions[a] {
			t.Errorf("AllowedLLMActions entry %q not in validActions map", a)
		}
	}
}

func TestSanitizeLLMInput_StripInjection(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"normal text", "normal text"},
		{"text <|system|> injected", "text  injected"},
		{"text [INST] injected [/INST]", "text  injected "},
		{"<<SYS>> override <</SYS>>", " override "},
		{"<|im_start|> bypass <|im_end|>", " bypass "},
	}
	for _, tc := range cases {
		got := sanitizeLLMInput(tc.input, 0)
		if got != tc.want {
			t.Errorf("sanitizeLLMInput(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSanitizeLLMInput_TruncateLong(t *testing.T) {
	long := strings.Repeat("a", 600)
	got := sanitizeLLMInput(long, 500)
	if len(got) > 504 { // 500 + "…" (3 bytes UTF-8)
		t.Errorf("expected truncated length <= 504, got %d", len(got))
	}
}

func TestSanitizeLLMInput_StripControlChars(t *testing.T) {
	input := "hello\x00world\x01test\nnewline\ttab"
	got := sanitizeLLMInput(input, 0)
	if strings.ContainsAny(got, "\x00\x01") {
		t.Error("expected control characters to be stripped")
	}
	if !strings.Contains(got, "\n") || !strings.Contains(got, "\t") {
		t.Error("expected newlines and tabs to be preserved")
	}
}

func TestLLMReflector_ParseResponse_ExpandedActions(t *testing.T) {
	lr := &LLMReflector{}

	// Test that the newly added actions are accepted
	raw := `[{"pattern_id": "test1", "severity": "medium", "description": "constraints need codification", "recommended_action": "codify_constraint", "reasoning": "test"},
	{"pattern_id": "test2", "severity": "low", "description": "guidance needs review", "recommended_action": "review_guidance_effectiveness", "reasoning": "test"}]`

	insights, err := lr.parseResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(insights) != 2 {
		t.Errorf("expected 2 insights for expanded actions, got %d", len(insights))
	}
}
