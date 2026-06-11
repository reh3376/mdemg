package alert

import (
	"strings"
	"testing"
)

func TestHookHealthRules_Defaults(t *testing.T) {
	rules := HookHealthRules(0, 0) // zero → defaults 24h / 5 events
	if len(rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rules))
	}
	r := rules[0]
	if r.ID != "hook_channel_silent" {
		t.Errorf("ID = %q", r.ID)
	}
	// Distinct Service per the NOSILENT-001 cooldown-collision rule.
	if r.Service != "hook-channel-silent" {
		t.Errorf("Service = %q — must be unique per rule", r.Service)
	}
	if r.Severity != SeverityHigh || !r.Enabled {
		t.Errorf("severity/enabled = %v/%v", r.Severity, r.Enabled)
	}
	for _, want := range []string{"hook:post-tool-observe", "hook:prompt-context", "'24 hours'", ">= 5"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
	if r.Threshold != 0 || r.Operator != "gt" {
		t.Errorf("threshold/op = %v/%v", r.Threshold, r.Operator)
	}
}

func TestHookHealthRules_CustomParams(t *testing.T) {
	r := HookHealthRules(6, 10)[0]
	for _, want := range []string{"'6 hours'", ">= 10"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
}

func TestWeightIntegrityRules(t *testing.T) {
	r := WeightIntegrityRules(0)[0] // 0 → default 100
	if r.ID != "null_weight_abstraction_edges" || r.Service != "graph-weight-integrity" {
		t.Errorf("id/service = %q/%q", r.ID, r.Service)
	}
	if r.Threshold != 100 {
		t.Errorf("default threshold = %v, want 100", r.Threshold)
	}
	if !strings.Contains(r.QuerySQL, "mdemg_neo4j_graph_null_weight_edges") {
		t.Errorf("QuerySQL missing the gauge name")
	}
	if got := WeightIntegrityRules(250)[0].Threshold; got != 250 {
		t.Errorf("custom threshold = %v, want 250", got)
	}
}

func TestMaintenanceLivenessRules(t *testing.T) {
	r := MaintenanceLivenessRules(0)[0] // 0 → default 8 days
	if r.ID != "maintenance_no_live_run" || r.Service != "maintenance-liveness" {
		t.Errorf("id/service = %q/%q", r.ID, r.Service)
	}
	for _, want := range []string{"'8 days'", "dry_run", "job_name = 'maintenance'", "success = true"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
	if got := MaintenanceLivenessRules(14)[0]; !strings.Contains(got.QuerySQL, "'14 days'") {
		t.Errorf("custom lookback not applied")
	}
}

func TestCoverageRules(t *testing.T) {
	r := CoverageRules(0)[0]
	if r.ID != "low_conversation_coverage" || r.Service != "conversation-coverage" {
		t.Errorf("id/service = %q/%q", r.ID, r.Service)
	}
	for _, want := range []string{"mdemg_neo4j_conversation_coverage_ratio", "time >"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
}

// RSIC-VALIDATE/HIDDEN-CHURN regression pin: metric_samples rules must use
// the table's actual time column — `recorded_at` silently errored every
// evaluation (Debug-only logging) for the null-weight rule's first hours.
func TestMetricSamplesRules_UseTimeColumn(t *testing.T) {
	for _, r := range append(WeightIntegrityRules(0), CoverageRules(0)...) {
		if strings.Contains(r.QuerySQL, "recorded_at") {
			t.Errorf("rule %s queries metric_samples with recorded_at (column is `time`)", r.ID)
		}
	}
}

// TSDB-CONSUME-001: the retrieve-latency rules replaced the broken
// lifetime-cumulative HTTP synthetic rules. Pins:
//   - query retrieval_audit (real per-call wall time), whose time column is
//     recorded_at — the INVERSE of the metric_samples pin above
//   - aggregate + COALESCE so an idle window returns 0, not "no rows in
//     result set" (the recurring rule-health-*_latency failure mode)
//   - never LIMIT 1 (latest-sample semantics caused both failure modes)
func TestRetrieveLatencyRules_Defaults(t *testing.T) {
	rules := RetrieveLatencyRules(0, 0, 0)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	p95, p99 := rules[0], rules[1]
	if p95.ID != "retrieve_p95_latency" || p99.ID != "retrieve_p99_latency" {
		t.Errorf("ids = %q/%q", p95.ID, p99.ID)
	}
	if p95.Threshold != 120000 || p99.Threshold != 300000 {
		t.Errorf("default thresholds = %v/%v, want 120000/300000", p95.Threshold, p99.Threshold)
	}
	if p95.Severity != SeverityMedium || p99.Severity != SeverityCritical {
		t.Errorf("severities = %v/%v", p95.Severity, p99.Severity)
	}
	for _, r := range rules {
		for _, want := range []string{"retrieval_audit", "recorded_at", "COALESCE", "percentile_cont", "total_latency_ms", "'30 minutes'"} {
			if !strings.Contains(r.QuerySQL, want) {
				t.Errorf("rule %s QuerySQL missing %q", r.ID, want)
			}
		}
		if strings.Contains(r.QuerySQL, "LIMIT 1") {
			t.Errorf("rule %s uses LIMIT 1 (idle window → no rows → rule-health noise)", r.ID)
		}
		if strings.Contains(r.QuerySQL, "metric_samples") {
			t.Errorf("rule %s queries metric_samples (must read retrieval_audit real wall time)", r.ID)
		}
		if !r.Enabled {
			t.Errorf("rule %s should be enabled", r.ID)
		}
	}
}

func TestRetrieveLatencyRules_CustomParams(t *testing.T) {
	rules := RetrieveLatencyRules(45000, 90000, 15)
	if rules[0].Threshold != 45000 || rules[1].Threshold != 90000 {
		t.Errorf("custom thresholds not applied: %v/%v", rules[0].Threshold, rules[1].Threshold)
	}
	if !strings.Contains(rules[0].QuerySQL, "'15 minutes'") {
		t.Errorf("custom lookback not applied")
	}
}

// Pin: the dead rules stay dead. high_p95_latency/critical_p99_latency read
// lifetime-cumulative synthetics (perpetually pegged at the 9.95 bucket
// clamp); neo4j_pool_exhausted read a perpetual-zero fake gauge.
func TestDefaultRules_RemovedRulesStayRemoved(t *testing.T) {
	for _, r := range DefaultRules() {
		switch r.ID {
		case "high_p95_latency", "critical_p99_latency", "neo4j_pool_exhausted":
			t.Errorf("rule %s was removed by TSDB-CONSUME-001 and must not return", r.ID)
		}
	}
}

// TSDB-CONSUME-001: writer flush-failure rule pins — per-writer MAX-MIN
// delta (restart-safe) over metric_samples (time column), COALESCE'd.
func TestTSDBWriterRules(t *testing.T) {
	r := TSDBWriterRules(0)[0]
	if r.ID != "tsdb_writer_flush_failures" || r.Service != "tsdb-writer" {
		t.Errorf("id/service = %q/%q", r.ID, r.Service)
	}
	if r.Severity != SeverityHigh || r.Threshold != 0 || r.Operator != "gt" {
		t.Errorf("severity/threshold/op = %v/%v/%q", r.Severity, r.Threshold, r.Operator)
	}
	for _, want := range []string{"mdemg_tsdb_writer_flush_failures_total", "labels->>'writer'", "MAX(value) - MIN(value)", "COALESCE", "'60 minutes'", "time >"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
	if strings.Contains(r.QuerySQL, "recorded_at") {
		t.Errorf("metric_samples rule uses recorded_at (column is `time`)")
	}
	if got := TSDBWriterRules(15)[0]; !strings.Contains(got.QuerySQL, "'15 minutes'") {
		t.Errorf("custom lookback not applied")
	}
}
