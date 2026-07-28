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
		case "graph_node_drop":
			t.Errorf("rule %s was extracted to GraphNodeDropRule() by NODE-DROP-CALIBRATION-001 and must not return in DefaultRules", r.ID)
		}
	}
}

// allRules gathers every rule the evaluator can run, mirroring the serve.go
// assembly, so contract pins cover the parameterized groups too.
func allRules() []AlertRule {
	rules := DefaultRules()
	rules = append(rules, WeightIntegrityRules(0)...)
	rules = append(rules, OrphanRules(0, 0, 0, 0)...)
	rules = append(rules, GraphNodeDropRule(0, 0, 0)...)
	rules = append(rules, HITLCurationStalledRule(0, 0))
	rules = append(rules, ReadinessStalenessRule(0))
	rules = append(rules, CoverageRules(0)...)
	rules = append(rules, MaintenanceLivenessRules(0)...)
	rules = append(rules, RetrieveLatencyRules(0, 0, 0)...)
	rules = append(rules, GuidanceShouldFollowRules(0.5, 0)...)
	rules = append(rules, TSDBWriterRules(0)...)
	rules = append(rules, ScorerDriftRules(0, 0, 0, 0, 0)...)
	rules = append(rules, EmergenceCycleRules(0, 0)...)
	rules = append(rules, Neo4jCPURule(0))
	rules = append(rules, FTBenchmarkStalenessRule(0))
	return rules
}

// TSDB-CONSUME-001 contract pin (ALERT-TRUTH-001 swept the last 4 offenders:
// neo4j_high_cpu/_memory, low_cache_hit_ratio, jiminy_follow_rate_drop): NO rule
// may use `ORDER BY ... LIMIT 1` — an idle window returns zero rows (rule-health
// noise) and a single latest sample flaps on bursty gauges. Rules must aggregate
// + COALESCE so they always return exactly one non-NULL row.
func TestAllRules_NoLimitOneAntiPattern(t *testing.T) {
	for _, r := range allRules() {
		if strings.Contains(r.QuerySQL, "LIMIT 1") {
			t.Errorf("rule %s uses LIMIT 1 — aggregate + COALESCE instead (TSDB-CONSUME-001)", r.ID)
		}
	}
}

// NOSILENT-001 cooldown-key contract: every rule needs a non-empty Service, and
// no two rules at the same Severity may share a Service (one would mask the other
// via the (Service,Severity) cooldown key).
func TestAllRules_DistinctServicePerSeverity(t *testing.T) {
	seen := map[string]string{} // service|severity -> ruleID
	for _, r := range allRules() {
		if r.Service == "" {
			t.Errorf("rule %s has empty Service", r.ID)
			continue
		}
		key := r.Service + "|" + string(r.Severity)
		if prior, ok := seen[key]; ok {
			t.Errorf("rules %s and %s share Service %q at the same Severity — cooldown collision", r.ID, prior, r.Service)
		}
		seen[key] = r.ID
	}
}

// ALERT-TRUTH-001: Neo4j CPU rule is config-driven + host-relative, windowed AVG.
func TestNeo4jCPURule(t *testing.T) {
	def := Neo4jCPURule(0)
	if def.Threshold != 500 {
		t.Errorf("zero/neg → default 500, got %v", def.Threshold)
	}
	if def.ID != "neo4j_high_cpu" {
		t.Errorf("ID = %q", def.ID)
	}
	// Distinct from the memory rule's "neo4j" Service (NOSILENT-001).
	if def.Service != "neo4j-cpu" {
		t.Errorf("Service = %q — must be distinct from neo4j_high_memory", def.Service)
	}
	if !strings.Contains(def.QuerySQL, "AVG(value)") || !strings.Contains(def.QuerySQL, "COALESCE") {
		t.Errorf("query must be a COALESCE'd windowed AVG, got: %s", def.QuerySQL)
	}
	if got := Neo4jCPURule(750); got.Threshold != 750 {
		t.Errorf("custom threshold not honored, got %v", got.Threshold)
	}
}

// FT-BENCH-REFRESH-001: staleness rule reads benchmark_runs directly,
// COALESCE'd (999 when zero rows), Service "ft-benchmark" (distinct
// per NOSILENT-001 cooldown-key contract). Default 7d fallback on ≤0.
func TestFTBenchmarkStalenessRule(t *testing.T) {
	def := FTBenchmarkStalenessRule(0)
	if def.Threshold != 7 {
		t.Errorf("zero/neg → default 7d, got %v", def.Threshold)
	}
	if def.ID != "ft_benchmark_stale" {
		t.Errorf("ID = %q, want ft_benchmark_stale", def.ID)
	}
	if def.Service != "ft-benchmark" {
		t.Errorf("Service = %q, want ft-benchmark (distinct cooldown key per NOSILENT-001)", def.Service)
	}
	if def.Severity != SeverityHigh {
		t.Errorf("Severity = %v, want HIGH", def.Severity)
	}
	if !strings.Contains(def.QuerySQL, "COALESCE") {
		t.Errorf("query must be COALESCE'd (idle-safe), got: %s", def.QuerySQL)
	}
	if !strings.Contains(def.QuerySQL, "MAX(completed_at)") {
		t.Errorf("query must aggregate MAX(completed_at), got: %s", def.QuerySQL)
	}
	if strings.Contains(def.QuerySQL, "LIMIT 1") {
		t.Errorf("query must NOT use LIMIT 1 (TSDB-CONSUME-001), got: %s", def.QuerySQL)
	}
	if got := FTBenchmarkStalenessRule(30); got.Threshold != 30 {
		t.Errorf("custom threshold not honored, got %v", got.Threshold)
	}
	if def.Operator != "gt" {
		t.Errorf("Operator = %q, want gt (fires when age exceeds threshold)", def.Operator)
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

// TSDB-CONSUME-001: scorer-drift tripwires — the RRF-SCALE-001 regression
// class (scorer changes silently breaking Score consumers) self-detects.
// Pins: retrieval_audit time column is recorded_at; always-return-a-row
// (COALESCE / CASE over aggregates); unique Service per rule (dispatcher
// cooldown key is (Service, Severity) — shared labels mask each other).
func TestScorerDriftRules(t *testing.T) {
	rules := ScorerDriftRules(0, 0, 0, 0, 0)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	change, shift := rules[0], rules[1]
	if change.ID != "scorer_version_change" || shift.ID != "consensus_shift" {
		t.Errorf("ids = %q/%q", change.ID, shift.ID)
	}
	if change.Service == shift.Service {
		t.Errorf("rules share Service %q — cooldown key collision masks alerts", change.Service)
	}
	for _, want := range []string{"COUNT(DISTINCT scorer_version)", "COALESCE", "retrieval_audit", "recorded_at", "'24 hours'"} {
		if !strings.Contains(change.QuerySQL, want) {
			t.Errorf("scorer_version_change QuerySQL missing %q", want)
		}
	}
	if change.Threshold != 1 || change.Operator != "gt" {
		t.Errorf("change threshold/op = %v/%q", change.Threshold, change.Operator)
	}
	for _, want := range []string{"consensus_strength", "ABS(", ">= 20", "'6 hours'", "'7 days'", "recorded_at"} {
		if !strings.Contains(shift.QuerySQL, want) {
			t.Errorf("consensus_shift QuerySQL missing %q", want)
		}
	}
	if shift.Threshold != 0.10 {
		t.Errorf("shift threshold = %v, want 0.10", shift.Threshold)
	}
	for _, r := range rules {
		if strings.Contains(r.QuerySQL, "LIMIT 1") {
			t.Errorf("rule %s uses LIMIT 1", r.ID)
		}
		if strings.Contains(r.QuerySQL, "metric_samples") {
			t.Errorf("rule %s must query retrieval_audit directly", r.ID)
		}
	}
}

func TestScorerDriftRules_CustomParams(t *testing.T) {
	rules := ScorerDriftRules(48, 0.25, 12, 14, 50)
	if !strings.Contains(rules[0].QuerySQL, "'48 hours'") {
		t.Errorf("custom change lookback not applied")
	}
	if rules[1].Threshold != 0.25 {
		t.Errorf("custom shift threshold not applied: %v", rules[1].Threshold)
	}
	for _, want := range []string{">= 50", "'12 hours'", "'14 days'"} {
		if !strings.Contains(rules[1].QuerySQL, want) {
			t.Errorf("consensus_shift custom QuerySQL missing %q", want)
		}
	}
}

// TSDB-CONSUME-001: the DBSCAN O(n²) deferral condition (>60s emergence
// cycles) is observable. Gauge MAX over window, metric_samples time column,
// COALESCE'd.
func TestEmergenceCycleRules(t *testing.T) {
	r := EmergenceCycleRules(0, 0)[0]
	if r.ID != "emergence_cycle_slow" || r.Service != "emergence-cycle" {
		t.Errorf("id/service = %q/%q", r.ID, r.Service)
	}
	if r.Threshold != 60 || r.Operator != "gt" {
		t.Errorf("threshold/op = %v/%q", r.Threshold, r.Operator)
	}
	for _, want := range []string{"mdemg_emergence_cycle_duration_seconds", "COALESCE(MAX(value), 0)", "'120 minutes'", "time >"} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("QuerySQL missing %q", want)
		}
	}
	if strings.Contains(r.QuerySQL, "recorded_at") {
		t.Errorf("metric_samples rule uses recorded_at (column is `time`)")
	}
	custom := EmergenceCycleRules(90, 30)[0]
	if custom.Threshold != 90 || !strings.Contains(custom.QuerySQL, "'30 minutes'") {
		t.Errorf("custom params not applied")
	}
}

// FT-RECURSIVE-003 E1: the readiness-staleness rule suppresses while the
// recursive-retrain compute lease is held (FTLOOP-DRILL-001 finding — the
// heartbeat SHOULD pause during a legitimate quiesce).
func TestReadinessStalenessRule_LeaseAware(t *testing.T) {
	r := ReadinessStalenessRule(30)
	for _, want := range []string{
		"mdemg_ftloop_lease_held",
		"interval '5 minutes'",
		"THEN 0",
		"mdemg_rsic_readiness_assessed",
	} {
		if !strings.Contains(r.QuerySQL, want) {
			t.Errorf("rule SQL missing %q", want)
		}
	}
}

// FT-RECURSIVE-004: rule-shape pins.
func TestFtLoopNeverRanRule_Shape(t *testing.T) {
	r := FtLoopNeverRanRule(14)
	for _, w := range []string{"ft_training_cycles", "COALESCE", "999.0"} {
		if !strings.Contains(r.QuerySQL, w) {
			t.Errorf("missing %q", w)
		}
	}
	if r.Service != "ft-loop-staleness" {
		t.Errorf("service: %s", r.Service)
	}
}

func TestFtProductionDriftRule_NoDataGates(t *testing.T) {
	r := FtProductionDriftRule(0.05)
	for _, w := range []string{"status = 'active'", "<= 0 THEN 0", "IS NULL THEN 0", "GREATEST(0"} {
		if !strings.Contains(r.QuerySQL, w) {
			t.Errorf("missing no-data gate fragment %q", w)
		}
	}
	if r.Threshold != 0.05 {
		t.Errorf("threshold: %f", r.Threshold)
	}
}

// NODE-DROP-CALIBRATION-001: split graph_node_drop with min-node floor +
// ratio + absolute, distinct Services, HIGH severity (was CRITICAL), config
// defaults matching ORPHAN-ALERT-001 semantics.
func TestGraphNodeDropRule_Defaults(t *testing.T) {
	rules := GraphNodeDropRule(0, 0, 0)
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules (ratio + absolute), got %d", len(rules))
	}
	byID := map[string]AlertRule{}
	for _, r := range rules {
		byID[r.ID] = r
	}
	ratio, okR := byID["graph_node_drop_ratio"]
	count, okC := byID["graph_node_drop_count"]
	if !okR || !okC {
		t.Fatalf("expected IDs graph_node_drop_ratio + graph_node_drop_count, got %v", byID)
	}
	// Defaults: min-node floor 50, ratio 0.10, absolute 10000.
	if ratio.Threshold != 0.10 {
		t.Errorf("ratio threshold default: %f (want 0.10)", ratio.Threshold)
	}
	if count.Threshold != 10000 {
		t.Errorf("count threshold default: %f (want 10000)", count.Threshold)
	}
	for _, r := range rules {
		if r.Severity != SeverityHigh {
			t.Errorf("rule %s severity: %s (want HIGH — CRITICAL is reserved for data-loss)", r.ID, r.Severity)
		}
		// Distinct services per NOSILENT-001 cooldown-key contract.
		if r.Service == "graph-health" {
			t.Errorf("rule %s must not reuse the old 'graph-health' Service — pick distinct labels", r.ID)
		}
		// Min-node significance floor visible in SQL.
		if !strings.Contains(r.QuerySQL, "c.value >= 50") {
			t.Errorf("rule %s missing min-node floor 'c.value >= 50': %s", r.ID, r.QuerySQL)
		}
		// TSDB-CONSUME-001 idle-safe contract: MAX + COALESCE, no LIMIT 1.
		if !strings.Contains(r.QuerySQL, "COALESCE(MAX(") {
			t.Errorf("rule %s must aggregate via COALESCE(MAX(...)) — idle-safe contract", r.ID)
		}
	}
}

// HITL-CURATION-002 E4: hitl_curation_stalled fires only when BOTH the pending
// queue is above minPending AND there are zero operator grades in the window.
// The autograde grader_id starts with "auto:" and must be EXCLUDED from the
// operator-count — else the sprint's own auto-clear would suppress the alert.
func TestHITLCurationStalledRule_Defaults(t *testing.T) {
	r := HITLCurationStalledRule(0, 0)
	if r.ID != "hitl_curation_stalled" {
		t.Errorf("id: %s", r.ID)
	}
	if r.Service != "hitl-curation" {
		t.Errorf("service: %s (NOSILENT-001 cooldown-key contract — must be distinct)", r.Service)
	}
	if r.Severity != SeverityMedium {
		t.Errorf("severity: %s (stall is not an emergency; MEDIUM)", r.Severity)
	}
	// Must exclude auto-grades — else the sprint's own auto-clear suppresses
	// the stall alert.
	if !strings.Contains(r.QuerySQL, "grader_id NOT LIKE 'auto:%'") {
		t.Errorf("query MUST exclude auto:* grader_id from the operator-graded count; got: %s", r.QuerySQL)
	}
	// Idle-safe contract (TSDB-CONSUME-001): COALESCE, no LIMIT 1.
	if !strings.Contains(r.QuerySQL, "COALESCE(") {
		t.Errorf("query must use COALESCE for idle-safe non-NULL row; got: %s", r.QuerySQL)
	}
	if strings.Contains(r.QuerySQL, "LIMIT 1") {
		t.Errorf("query must not use LIMIT 1 (TSDB-CONSUME-001 anti-pattern); got: %s", r.QuerySQL)
	}
	if r.Threshold != 5 {
		t.Errorf("default min-pending threshold: %f (want 5)", r.Threshold)
	}
	// ForDuration guards against a slow week — a single-day silence must
	// not fire.
	if r.ForDuration.Hours() < 12 {
		t.Errorf("ForDuration=%s too short — will flap on a slow curation week", r.ForDuration)
	}
}

func TestHITLCurationStalledRule_CustomParams(t *testing.T) {
	r := HITLCurationStalledRule(10, 336)
	if r.Threshold != 10 {
		t.Errorf("threshold: %f", r.Threshold)
	}
	if !strings.Contains(r.QuerySQL, "336 hours") {
		t.Errorf("lookback 336h not substituted; got: %s", r.QuerySQL)
	}
}

func TestGraphNodeDropRule_CustomParams(t *testing.T) {
	rules := GraphNodeDropRule(100, 0.20, 5000)
	for _, r := range rules {
		if !strings.Contains(r.QuerySQL, "c.value >= 100") {
			t.Errorf("rule %s custom min-nodes 100 not substituted: %s", r.ID, r.QuerySQL)
		}
	}
	byID := map[string]AlertRule{}
	for _, r := range rules {
		byID[r.ID] = r
	}
	if byID["graph_node_drop_ratio"].Threshold != 0.20 {
		t.Errorf("custom ratio not applied: %f", byID["graph_node_drop_ratio"].Threshold)
	}
	if byID["graph_node_drop_count"].Threshold != 5000 {
		t.Errorf("custom absolute not applied: %f", byID["graph_node_drop_count"].Threshold)
	}
}
