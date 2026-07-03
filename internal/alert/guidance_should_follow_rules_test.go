package alert

import (
	"strings"
	"testing"
)

// JIMINY-RELEVANCE-001 Epic 4 — pin the should-follow rule's SQL contract
// (TSDB-CONSUME-001): correct table + time column, aggregate+COALESCE so an idle
// window returns one non-NULL row (never "no rows"), actionable-only denominator,
// and a unique Service label (the dispatcher cooldown key).
func TestGuidanceShouldFollowRules_SQLContract(t *testing.T) {
	rules := GuidanceShouldFollowRules(0.5, 168)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	r := rules[0]

	if r.Service != "guidance-should-follow" {
		t.Errorf("Service = %q, want unique guidance-should-follow", r.Service)
	}
	if !strings.Contains(r.QuerySQL, "guidance_training_rows") {
		t.Errorf("rule must query guidance_training_rows")
	}
	// guidance_training_rows' time column is `time`, NOT recorded_at — crossing
	// them silently errors every evaluation (the HIDDEN-CHURN-001 class).
	if strings.Contains(r.QuerySQL, "recorded_at") {
		t.Errorf("rule uses recorded_at; guidance_training_rows' time column is `time`")
	}
	if !strings.Contains(r.QuerySQL, "now() - interval") || !strings.Contains(r.QuerySQL, "time >") {
		t.Errorf("rule must window on the `time` column")
	}
	// Aggregate + COALESCE so an idle window returns one non-NULL row.
	if !strings.Contains(r.QuerySQL, "COALESCE") || !strings.Contains(r.QuerySQL, "AVG(") {
		t.Errorf("rule must aggregate + COALESCE (always one non-NULL row), got: %s", r.QuerySQL)
	}
	if strings.Contains(strings.ToUpper(r.QuerySQL), "LIMIT 1") {
		t.Errorf("rule must NOT use ORDER BY … LIMIT 1 (idle → no rows); use an aggregate")
	}
	// Denominator restricted to the actionable should-follow types.
	if !strings.Contains(r.QuerySQL, "'constraint'") || !strings.Contains(r.QuerySQL, "'correction'") {
		t.Errorf("should-follow denominator must restrict to constraint/correction types")
	}
	// DASHBOARD-TRUTH-001: not_applicable (and blank/unknown) rows carry no
	// follow/ignore verdict and must be excluded from BOTH numerator and
	// denominator — counting them as 0.0 understated the rate (0.094 vs 0.145
	// excl n/a live) and desynced the rule from the dashboard panel. Pinned as
	// an outcome_type allowlist (which excludes 'not_applicable' by omission).
	if !strings.Contains(r.QuerySQL, "outcome_type IN ('followed', 'partial_compliance', 'ignored', 'contradicted')") {
		t.Errorf("rule must allowlist verdict-bearing outcome_types (excluding not_applicable/blank/unknown), got: %s", r.QuerySQL)
	}
	if strings.Contains(r.QuerySQL, "'not_applicable'") {
		t.Errorf("not_applicable must not appear in the rule SQL (excluded by allowlist omission): %s", r.QuerySQL)
	}
	if r.Operator != "lt" {
		t.Errorf("Operator = %q, want lt (fires when rate drops below floor)", r.Operator)
	}
}

func TestGuidanceShouldFollowRules_Tunables(t *testing.T) {
	// Floor ≤ 0 disables the rule.
	if got := GuidanceShouldFollowRules(0, 168); got != nil {
		t.Errorf("rateFloor=0 should disable the rule, got %d rules", len(got))
	}
	// Lookback flows into the SQL.
	r := GuidanceShouldFollowRules(0.6, 72)[0]
	if !strings.Contains(r.QuerySQL, "'72 hours'") {
		t.Errorf("lookback 72h not reflected in SQL: %s", r.QuerySQL)
	}
	if r.Threshold != 0.6 {
		t.Errorf("Threshold = %v, want 0.6", r.Threshold)
	}
}
