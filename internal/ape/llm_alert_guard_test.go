package ape

import "testing"

// RSIC-LLM-ALERT-GUARD-001: the LLM reflector must never introduce a
// deterministic threshold-gated alert the rule-based path didn't raise — that
// was the source of the false `alert_jiminy_critical` "Jiminy Service
// Unavailable" CRITICALs while jiminy_healthy=true.

func TestAllowedLLMActions_ExcludesDeterministicAlerts(t *testing.T) {
	for _, banned := range []string{"alert_jiminy_critical", "alert_memory_bloat", "alert_synergy_overlap"} {
		if validActions[banned] {
			t.Errorf("%q must NOT be in the LLM whitelist — the rule-based reflector owns it (ungrounded LLM copy hallucinates CRITICALs)", banned)
		}
	}
}

func TestDeduplicateInsights_DropsLLMDeterministicAlert(t *testing.T) {
	// Reproduces the bug: rule-based path raised NO alert_jiminy_critical
	// (Jiminy healthy), but the LLM hallucinated one. It must be dropped.
	base := []ReflectionInsight{
		{PatternID: "rule:prune", RecommendedAction: "prune_decayed_edges", Severity: SeverityMedium},
	}
	llm := []ReflectionInsight{
		{PatternID: "llm:halluc", RecommendedAction: "alert_jiminy_critical", Severity: SeverityCritical},
		{PatternID: "llm:novel", RecommendedAction: "trigger_consolidation", Severity: SeverityMedium},
	}
	out := deduplicateInsights(base, llm)
	for _, i := range out {
		if i.RecommendedAction == "alert_jiminy_critical" {
			t.Fatalf("LLM-recommended alert_jiminy_critical must be dropped (ungrounded hallucination)")
		}
	}
	// A genuinely novel (non-deterministic-alert) LLM action still merges.
	found := false
	for _, i := range out {
		if i.RecommendedAction == "trigger_consolidation" {
			found = true
		}
	}
	if !found {
		t.Errorf("novel LLM action (trigger_consolidation) should still merge — only deterministic alerts are dropped")
	}
}

func TestDeduplicateInsights_PreservesRuleBasedDeterministicAlert(t *testing.T) {
	// When the rule-based path legitimately raises a deterministic alert (real
	// condition true), it must survive — only LLM-originated copies are dropped.
	base := []ReflectionInsight{
		{PatternID: "rule:jiminy", RecommendedAction: "alert_jiminy_critical", Severity: SeverityCritical},
	}
	out := deduplicateInsights(base, nil)
	found := false
	for _, i := range out {
		if i.RecommendedAction == "alert_jiminy_critical" {
			found = true
		}
	}
	if !found {
		t.Errorf("rule-based alert_jiminy_critical must be preserved (only LLM copies are guarded)")
	}
}
