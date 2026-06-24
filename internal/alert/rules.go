package alert

import (
	"fmt"
	"time"
)

// AlertRule defines a server-native alert rule evaluated against TSDB.
type AlertRule struct {
	ID          string        // e.g. "high_p95_latency"
	Title       string        // human-readable title
	Service     string        // alert service label
	Severity    Severity      // alert severity
	Interval    time.Duration // how often to evaluate
	ForDuration time.Duration // condition must be true for this long before firing
	QuerySQL    string        // SQL returning a single numeric value
	Threshold   float64       // comparison value
	Operator    string        // "gt", "lt", "gte", "lte"
	Enabled     bool
}

// DefaultRules returns the 10 server-native alert rules migrated from Grafana.
// SQL queries use raw TimescaleDB queries against the metric_samples table.
//
// Removed by TSDB-CONSUME-001:
//   - high_p95_latency / critical_p99_latency — they read the synthetic
//     mdemg_http_request_duration_seconds_p95/_p99 gauges, which were
//     lifetime-cumulative (one slow call ever → permanently pegged; live
//     value was the 9.95 top-bucket clamp, a perpetual false CRITICAL) and
//     LIMIT 1 over an idle window returned zero rows (the recurring
//     rule-health-*_latency noise). Replaced by RetrieveLatencyRules below,
//     which computes real windowed percentiles over retrieval_audit.
//   - neo4j_pool_exhausted — it read mdemg_neo4j_pool_waiting_requests,
//     which was a perpetual zero (the neo4j driver has no pool-stats API;
//     the "pool" gauges were a VerifyConnectivity probe in disguise). The
//     rule could never fire. Neo4j liveness is the health prober's job.
func DefaultRules() []AlertRule {
	return []AlertRule{
		{
			ID:          "high_error_rate",
			Title:       "MDEMG Error Rate Exceeds SLO",
			Service:     "error-rate",
			Severity:    SeverityMedium,
			Interval:    30 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: `SELECT CASE WHEN SUM(value) > 0 THEN
				    SUM(CASE WHEN labels->>'status' ~ '^5' THEN value ELSE 0 END)
				    / SUM(value) * 100
				  ELSE 0 END AS error_pct
				FROM metric_samples
				WHERE metric_name = 'mdemg_http_requests_total'
				  AND metric_type = 'counter'
				  AND time > now() - interval '5 minutes'`,
			Threshold: 0.1,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "low_graph_health",
			Title:       "MDEMG Low Graph Health Score",
			Service:     "graph-health",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 10 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_graph_health_score'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '10 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 0.5,
			Operator:  "lt",
			Enabled:   true,
		},
		{
			ID:          "high_orphan_count",
			Title:       "MDEMG High Orphan Count",
			Service:     "graph-health",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 15 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_graph_orphans'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '15 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 50,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "high_orphan_ratio",
			Title:       "MDEMG High Orphan Ratio",
			Service:     "graph-health",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 15 * time.Minute,
			QuerySQL: `WITH latest AS (
				  SELECT DISTINCT ON (labels->>'space_id')
				    labels->>'space_id' AS space_id, value AS orphans
				  FROM metric_samples
				  WHERE metric_name = 'mdemg_neo4j_graph_orphans'
				    AND metric_type = 'gauge'
				    AND time > now() - interval '15 minutes'
				  ORDER BY labels->>'space_id', time DESC
				),
				nodes AS (
				  SELECT DISTINCT ON (labels->>'space_id')
				    labels->>'space_id' AS space_id, value AS total_nodes
				  FROM metric_samples
				  WHERE metric_name = 'mdemg_neo4j_graph_nodes'
				    AND metric_type = 'gauge'
				    AND time > now() - interval '15 minutes'
				  ORDER BY labels->>'space_id', time DESC
				)
				SELECT CASE WHEN n.total_nodes > 0 THEN l.orphans / n.total_nodes ELSE 0 END AS ratio
				FROM latest l JOIN nodes n ON l.space_id = n.space_id
				ORDER BY ratio DESC LIMIT 1`,
			Threshold: 0.10,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "neo4j_high_memory",
			Title:       "MDEMG Neo4j High Memory Usage",
			Service:     "neo4j",
			Severity:    SeverityMedium,
			Interval:    30 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_container_mem_percent'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '5 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 80,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "neo4j_high_cpu",
			Title:       "MDEMG Neo4j High CPU Usage",
			Service:     "neo4j",
			Severity:    SeverityMedium,
			Interval:    30 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_container_cpu_percent'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '5 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 80,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "graph_node_drop",
			Title:       "MDEMG Significant Node Count Drop",
			Service:     "graph-health",
			Severity:    SeverityCritical,
			Interval:    60 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: `WITH current_val AS (
				  SELECT DISTINCT ON (labels->>'space_id')
				    labels->>'space_id' AS space_id, value
				  FROM metric_samples
				  WHERE metric_name = 'mdemg_neo4j_graph_nodes'
				    AND metric_type = 'gauge'
				    AND time > now() - interval '5 minutes'
				  ORDER BY labels->>'space_id', time DESC
				),
				old_val AS (
				  SELECT DISTINCT ON (labels->>'space_id')
				    labels->>'space_id' AS space_id, value
				  FROM metric_samples
				  WHERE metric_name = 'mdemg_neo4j_graph_nodes'
				    AND metric_type = 'gauge'
				    AND time BETWEEN now() - interval '65 minutes' AND now() - interval '55 minutes'
				  ORDER BY labels->>'space_id', time DESC
				)
				SELECT COALESCE(MAX(o.value - c.value), 0) AS drop_count
				FROM old_val o JOIN current_val c ON o.space_id = c.space_id
				WHERE o.value - c.value > 0`,
			Threshold: 100,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "rate_limiting_active",
			Title:       "MDEMG High Rate Limit Rejections",
			Service:     "rate-limiter",
			Severity:    SeverityLow,
			Interval:    30 * time.Second,
			ForDuration: 2 * time.Minute,
			QuerySQL: `SELECT COALESCE(SUM(value) / 60.0, 0) AS rate_per_sec
				FROM metric_samples
				WHERE metric_name = 'mdemg_rate_limit_rejected_total'
				  AND metric_type = 'counter'
				  AND time > now() - interval '1 minute'`,
			Threshold: 10,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "low_cache_hit_ratio",
			Title:       "MDEMG Low Query Cache Hit Ratio",
			Service:     "cache",
			Severity:    SeverityLow,
			Interval:    60 * time.Second,
			ForDuration: 10 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_cache_hit_ratio'
				  AND metric_type = 'gauge'
				  AND labels->>'cache' = 'query'
				  AND time > now() - interval '10 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 0.5,
			Operator:  "lt",
			Enabled:   true,
		},
		{
			ID:          "jiminy_follow_rate_drop",
			Title:       "Jiminy Follow Rate Drop",
			Service:     "jiminy",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 30 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_jiminy_follow_rate'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '30 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 0.3,
			Operator:  "lt",
			Enabled:   true,
		},
	}
}

// GuidanceShouldFollowRules returns the should-follow follow-rate rule
// (JIMINY-RELEVANCE-001 Epic 4).
//
// "Follow rate on guidance that SHOULD have been followed" — the metric the
// >90% operator goal actually means. The Step-1 diagnostic (TL;DR #4) showed
// the naive follow rate is partly the wrong target because much guidance is
// CORRECTLY ignored (advisory abstractions). This rule restricts the
// denominator to the ACTIONABLE guidance types (constraint/correction) — the
// items the diagnostic found are followed ~2× better and are the ones that
// should be followed — and excludes the pattern/learning/concept advisory class.
//
// SQL contract (TSDB-CONSUME-001, pin-tested):
//   - guidance_training_rows' time column is `time` (NOT recorded_at).
//   - Aggregate (AVG) + COALESCE so the query ALWAYS returns one non-NULL row:
//     an idle/fresh-corpus window yields 1.0 (above the floor → no false fire),
//     never the "no rows in result set" failure that fires rule-health noise.
//   - Unique Service label "guidance-should-follow" (dispatcher cooldown key).
//
// rateFloor ≤ 0 disables the rule (returns empty). Certified-gold grounding via
// HITL-REVIEW-001's review_grades (preferring human verdicts over auto-labels)
// is a documented follow-up — added when that table exists; this rule ships on
// the auto-labels alone so it is not blocked on HITL-REVIEW-001.
func GuidanceShouldFollowRules(rateFloor float64, lookbackHours int) []AlertRule {
	if rateFloor <= 0 {
		return nil // disabled
	}
	if rateFloor > 1 {
		rateFloor = 0.5
	}
	if lookbackHours <= 0 {
		lookbackHours = 168
	}
	return []AlertRule{
		{
			ID:          "guidance_should_follow_rate_low",
			Title:       "Guidance Should-Follow Follow Rate Low",
			Service:     "guidance-should-follow",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 0,
			QuerySQL: fmt.Sprintf(`SELECT COALESCE(AVG(
					CASE outcome_type
						WHEN 'followed' THEN 1.0
						WHEN 'partial_compliance' THEN 0.5
						ELSE 0.0
					END), 1.0) AS should_follow_rate
				FROM guidance_training_rows
				WHERE time > now() - interval '%d hours'
				  AND guidance_type IN ('constraint', 'correction')`, lookbackHours),
			Threshold: rateFloor,
			Operator:  "lt",
			Enabled:   true,
		},
	}
}

// RetrieveLatencyRules returns the retrieve-latency SLO rules
// (TSDB-CONSUME-001), computed over retrieval_audit.total_latency_ms — the
// real per-call wall time — instead of the lifetime-cumulative synthetic
// HTTP percentile gauges the old rules read.
//
// SQL shape requirements (pinned by tests):
//   - retrieval_audit's time column is recorded_at (NOT time — that is
//     metric_samples; the inverse of the HIDDEN-CHURN-001 bug class).
//   - Aggregate + COALESCE so the query ALWAYS returns one non-NULL row:
//     an idle window yields 0, not the "no rows in result set" failures
//     that fired rule-health-*_latency every quiet period.
//
// Thresholds are in milliseconds and config-driven
// (ALERT_RETRIEVE_P95_MS / ALERT_RETRIEVE_P99_MS /
// ALERT_RETRIEVE_LATENCY_LOOKBACK_MIN). Defaults are calibrated against the
// live distribution (7d: p50 20.4s, p95 61.6s, p99 90.0s — local-LLM rerank
// dominates), so they catch regressions, not steady state. Non-positive
// inputs fall back to defaults.
func RetrieveLatencyRules(p95ThreshMs, p99ThreshMs float64, lookbackMin int) []AlertRule {
	if p95ThreshMs <= 0 {
		p95ThreshMs = 120000
	}
	if p99ThreshMs <= 0 {
		p99ThreshMs = 300000
	}
	if lookbackMin <= 0 {
		lookbackMin = 30
	}
	latencySQL := func(q float64) string {
		return fmt.Sprintf(`SELECT COALESCE(
				percentile_cont(%g) WITHIN GROUP (ORDER BY total_latency_ms), 0)
			FROM retrieval_audit
			WHERE recorded_at > now() - interval '%d minutes'`, q, lookbackMin)
	}
	return []AlertRule{
		{
			ID:          "retrieve_p95_latency",
			Title:       "MDEMG Retrieve P95 Latency Exceeds SLO",
			Service:     "latency-slo",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL:    latencySQL(0.95),
			Threshold:   p95ThreshMs,
			Operator:    "gt",
			Enabled:     true,
		},
		{
			ID:          "retrieve_p99_latency",
			Title:       "MDEMG Retrieve P99 Latency Critical",
			Service:     "latency-slo",
			Severity:    SeverityCritical,
			Interval:    60 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL:    latencySQL(0.99),
			Threshold:   p99ThreshMs,
			Operator:    "gt",
			Enabled:     true,
		},
	}
}

// ScorerDriftRules returns the retrieval_audit tripwire rules
// (TSDB-CONSUME-001). The RRF-SCALE-001 class — a scorer change silently
// breaking every downstream Score consumer (24-day Hebbian no-op, 9-week
// guidance dormancy) — is detectable from data retrieval_audit already
// records, but the table had no readers.
//
//   - scorer_version_change: >1 distinct scorer_version inside the lookback.
//     Fires (medium) while old and new versions coexist in the window —
//     deliberately announces every scorer change for ~lookback hours, which
//     is the prompt to re-audit RetrieveResult.Score consumers per the
//     score-scale contract (CLAUDE.md RRF-SCALE-001).
//   - consensus_shift: |mean(consensus_strength) recent − baseline| above
//     threshold, sample-count gated on both windows (live calibration:
//     mean 0.31, σ 0.097 — default 0.10 ≈ 1σ).
//
// Both query retrieval_audit (time column recorded_at) and always return a
// row.
func ScorerDriftRules(changeLookbackHours int, shiftThreshold float64, shiftRecentHours, shiftBaselineDays, shiftMinSamples int) []AlertRule {
	if changeLookbackHours <= 0 {
		changeLookbackHours = 24
	}
	if shiftThreshold <= 0 {
		shiftThreshold = 0.10
	}
	if shiftRecentHours <= 0 {
		shiftRecentHours = 6
	}
	if shiftBaselineDays <= 0 {
		shiftBaselineDays = 7
	}
	if shiftMinSamples <= 0 {
		shiftMinSamples = 20
	}
	return []AlertRule{
		{
			ID:          "scorer_version_change",
			Title:       "MDEMG retrieval scorer version changed",
			Service:     "scorer-drift",
			Severity:    SeverityMedium,
			Interval:    5 * time.Minute,
			ForDuration: 0,
			QuerySQL: fmt.Sprintf(`SELECT COALESCE(COUNT(DISTINCT scorer_version), 0)
				FROM retrieval_audit
				WHERE recorded_at > now() - interval '%d hours'`, changeLookbackHours),
			Threshold: 1,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "consensus_shift",
			Title:       "MDEMG retrieval consensus distribution shifted",
			Service:     "consensus-shift",
			Severity:    SeverityMedium,
			Interval:    5 * time.Minute,
			ForDuration: 15 * time.Minute,
			QuerySQL: fmt.Sprintf(`SELECT CASE
				WHEN recent.n >= %d AND base.n >= %d
				THEN ABS(recent.avg_c - base.avg_c)
				ELSE 0 END
			FROM
				(SELECT COALESCE(AVG(consensus_strength), 0) AS avg_c,
				        COUNT(consensus_strength) AS n
				 FROM retrieval_audit
				 WHERE recorded_at > now() - interval '%d hours') recent,
				(SELECT COALESCE(AVG(consensus_strength), 0) AS avg_c,
				        COUNT(consensus_strength) AS n
				 FROM retrieval_audit
				 WHERE recorded_at BETWEEN now() - interval '%d days'
				       AND now() - interval '%d hours') base`,
				shiftMinSamples, shiftMinSamples,
				shiftRecentHours, shiftBaselineDays, shiftRecentHours),
			Threshold: shiftThreshold,
			Operator:  "gt",
			Enabled:   true,
		},
	}
}

// EmergenceCycleRules returns the slow-emergence-cycle rule
// (TSDB-CONSUME-001). The DBSCAN O(n²) ceiling deferral (roadmap §4) is
// conditioned on cycles exceeding ~60s — previously unobservable because
// ConsolidationResult.TotalDuration was computed and discarded. The gauge
// mdemg_emergence_cycle_duration_seconds{space_id,cycle} records each
// completed cycle; this rule takes the window MAX so any slow cycle inside
// the lookback fires (COALESCE: idle window → 0 → quiet).
func EmergenceCycleRules(thresholdSec float64, lookbackMin int) []AlertRule {
	if thresholdSec <= 0 {
		thresholdSec = 60
	}
	if lookbackMin <= 0 {
		lookbackMin = 120
	}
	return []AlertRule{
		{
			ID:          "emergence_cycle_slow",
			Title:       "MDEMG emergence cycle exceeded duration threshold",
			Service:     "emergence-cycle",
			Severity:    SeverityMedium,
			Interval:    5 * time.Minute,
			ForDuration: 0,
			QuerySQL: fmt.Sprintf(`SELECT COALESCE(MAX(value), 0) FROM metric_samples
				WHERE metric_name = 'mdemg_emergence_cycle_duration_seconds'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '%d minutes'`, lookbackMin),
			Threshold: thresholdSec,
			Operator:  "gt",
			Enabled:   true,
		},
	}
}

// TSDBWriterRules returns the buffered-writer flush-failure rule
// (TSDB-CONSUME-001). Every buffered TSDB writer reports cumulative flush
// stats into the mdemg_tsdb_writer_* gauge family; this rule fires when any
// writer's failure count grew inside the lookback window — a wedged writer
// previously dropped rows in silence (only the reinforcement writer had
// metrics visibility). MAX-MIN per writer label, summed: restart-safe
// (counts reset to 0, delta stays ≥ 0) and COALESCE'd so an empty window
// returns a row.
func TSDBWriterRules(lookbackMin int) []AlertRule {
	if lookbackMin <= 0 {
		lookbackMin = 60
	}
	return []AlertRule{
		{
			ID:          "tsdb_writer_flush_failures",
			Title:       "MDEMG TSDB writer flush failures",
			Service:     "tsdb-writer",
			Severity:    SeverityHigh,
			Interval:    60 * time.Second,
			ForDuration: 2 * time.Minute,
			QuerySQL: fmt.Sprintf(`SELECT COALESCE(SUM(delta), 0) FROM (
				SELECT labels->>'writer' AS writer, MAX(value) - MIN(value) AS delta
				FROM metric_samples
				WHERE metric_name = 'mdemg_tsdb_writer_flush_failures_total'
				  AND time > now() - interval '%d minutes'
				GROUP BY labels->>'writer'
			) per_writer`, lookbackMin),
			Threshold: 0,
			Operator:  "gt",
			Enabled:   true,
		},
	}
}

// JobHealthRules returns the NOSILENT-001 scheduled-job alert rules, evaluated
// against the V0024 scheduled_job_events hypertable. These make a failed — or
// never-run — scheduled job LOUD instead of a silent log line.
//
//   - stalenessHours: alert if no successful tsdb-backup landed within this
//     window. This is the key "job never ran" guarantee — it fires from the
//     server's observation of *absent* success, so it catches a job that
//     silently died or never started, not just one that ran and errored.
//   - failureLookbackMin: alert if ANY scheduled job recorded a failure within
//     this lookback.
//   - includeBackupStaleness: only emit the staleness rule when backups are
//     actually enabled (otherwise "0 successes" is expected, not an incident).
//
// Both thresholds are config-driven (no hardcoded literals).
func JobHealthRules(stalenessHours, failureLookbackMin int, includeBackupStaleness bool) []AlertRule {
	if stalenessHours <= 0 {
		stalenessHours = 48 // safety fallback; caller normally derives interval×2
	}
	if failureLookbackMin <= 0 {
		failureLookbackMin = 60
	}

	rules := []AlertRule{
		{
			ID:    "scheduled_job_recent_failure",
			Title: "Scheduled Job Recently Failed",
			// Distinct Service per rule: the dispatcher cooldown key is
			// (Service, Severity), so two scheduled-job rules sharing one
			// service would suppress each other (caught in NOSILENT-001 live
			// testing — the staleness alert was masked by the failure alert).
			Service:     "scheduled-job-failure",
			Severity:    SeverityHigh,
			Interval:    60 * time.Second,
			ForDuration: 0, // fire promptly — a failure is already a discrete event
			QuerySQL: fmt.Sprintf(`SELECT count(*) FROM scheduled_job_events
				WHERE success = false
				  AND recorded_at > now() - interval '%d minutes'`, failureLookbackMin),
			Threshold: 0,
			Operator:  "gt",
			Enabled:   true,
		},
	}

	if includeBackupStaleness {
		rules = append(rules, jobStalenessRule(
			"backup_no_recent_success", "No Successful TSDB Backup In Window",
			"scheduled-job-staleness", "tsdb-backup", stalenessHours))
	}

	return rules
}

// jobStalenessRule builds a per-job "no recent success" rule over the V0024
// scheduled_job_events hypertable (BACKUP-RESTORE-VERIFY-001 generalization
// of the NOSILENT-001 tsdb-backup rule). This is the "job never ran"
// guarantee: it fires from the server observing ABSENT success, catching a
// job that silently died or never started. jobName must be an internal
// constant (it is interpolated into SQL). Each rule gets its own Service —
// the dispatcher cooldown key is (Service, Severity).
func jobStalenessRule(id, title, service, jobName string, stalenessHours int) AlertRule {
	if stalenessHours <= 0 {
		stalenessHours = 48
	}
	return AlertRule{
		ID:          id,
		Title:       title,
		Service:     service,
		Severity:    SeverityHigh,
		Interval:    5 * time.Minute,
		ForDuration: 0,
		QuerySQL: fmt.Sprintf(`SELECT count(*) FROM scheduled_job_events
			WHERE job_name = '%s' AND success = true
			  AND recorded_at > now() - interval '%d hours'`, jobName, stalenessHours),
		Threshold: 0.5, // < 0.5 ⇒ zero successes ⇒ stale
		Operator:  "lt",
		Enabled:   true,
	}
}

// Neo4jBackupStalenessRule alerts when the default-ON Neo4j backup scheduler
// has recorded no successful run in the window (BACKUP-RESTORE-VERIFY-001 —
// it previously had zero jobhealth coverage, the inverse of NOSILENT-001).
func Neo4jBackupStalenessRule(stalenessHours int) AlertRule {
	return jobStalenessRule(
		"neo4j_backup_no_recent_success", "No Successful Neo4j Backup In Window",
		"scheduled-job-staleness-neo4j", "neo4j-backup", stalenessHours)
}

// HookHealthRules returns the HOOKSYNC-001 hook-channel absence rule. The
// per-prompt delivery channel (prompt-context) had a months-long silent
// outage caught only by manual audit; this is the "job never ran" guarantee
// applied to the channel. Two independent heartbeats land in
// scheduled_job_events via POST /v1/hooks/event: hook:post-tool-observe
// (throttled; proves sessions are ACTIVE) and hook:prompt-context (per
// prompt; the monitored channel). Sessions demonstrably active + zero
// prompt-context fires = the channel is silently dead.
//
//   - lookbackHours: observation window (HOOK_SILENT_LOOKBACK_HOURS).
//   - minActivityEvents: post-tool-observe rows required before the rule is
//     eligible (HOOK_ACTIVITY_MIN_EVENTS) — prevents false fires on idle days.
func HookHealthRules(lookbackHours, minActivityEvents int) []AlertRule {
	if lookbackHours <= 0 {
		lookbackHours = 24
	}
	if minActivityEvents <= 0 {
		minActivityEvents = 5
	}
	return []AlertRule{
		{
			ID:    "hook_channel_silent",
			Title: "Per-Prompt Hook Channel Silent While Sessions Active",
			// Distinct Service per the NOSILENT-001 cooldown-collision rule.
			Service:     "hook-channel-silent",
			Severity:    SeverityHigh,
			Interval:    300 * time.Second,
			ForDuration: 0,
			QuerySQL: fmt.Sprintf(`SELECT CASE WHEN
				(SELECT count(*) FROM scheduled_job_events
				   WHERE job_name = 'hook:post-tool-observe'
				     AND recorded_at > now() - interval '%d hours') >= %d
				AND
				(SELECT count(*) FROM scheduled_job_events
				   WHERE job_name = 'hook:prompt-context'
				     AND recorded_at > now() - interval '%d hours') = 0
				THEN 1 ELSE 0 END`, lookbackHours, minActivityEvents, lookbackHours),
			Threshold: 0,
			Operator:  "gt",
			Enabled:   true,
		},
	}
}

// WeightIntegrityRules returns the HIDDEN-WEIGHT-001 graph-weight rule:
// NULL-weight abstraction edges (GENERALIZES/ABSTRACTS_TO) above threshold.
// Steady state post-backfill is 0; sustained reappearance means the
// point.distance bug class regressed at a creation site. threshold ≤ 0 →
// default 100 (tolerates in-flight creation bursts between collector ticks).
func WeightIntegrityRules(threshold int) []AlertRule {
	if threshold <= 0 {
		threshold = 100
	}
	return []AlertRule{
		{
			ID:    "null_weight_abstraction_edges",
			Title: "NULL-Weight Abstraction Edges Reappearing",
			// Distinct Service per the NOSILENT-001 cooldown-collision rule.
			Service:     "graph-weight-integrity",
			Severity:    SeverityHigh,
			Interval:    300 * time.Second,
			ForDuration: 10 * time.Minute,
			QuerySQL: `SELECT coalesce(sum(value), 0) FROM (
				SELECT DISTINCT ON (labels->>'space_id') value
				FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_graph_null_weight_edges'
				  AND time > now() - interval '10 minutes'
				ORDER BY labels->>'space_id', time DESC) latest`,
			Threshold: float64(threshold),
			Operator:  "gt",
			Enabled:   true,
		},
	}
}

// MaintenanceLivenessRules returns the MAINT-LIVE-001 rule: maintenance runs
// are being recorded but NONE executed live (metadata dry_run=false) within
// the lookback window — the only-ever-dry-runs pattern that let the weekly
// decay+prune cycle silently no-op for the project's entire history while
// reporting success. lookbackDays ≤ 0 → default 8 (weekly cadence + buffer).
func MaintenanceLivenessRules(lookbackDays int) []AlertRule {
	if lookbackDays <= 0 {
		lookbackDays = 8
	}
	return []AlertRule{
		{
			ID:    "maintenance_no_live_run",
			Title: "Maintenance Only Dry-Running — Decay/Prune Not Executing",
			// Distinct Service per the NOSILENT-001 cooldown-collision rule.
			Service:     "maintenance-liveness",
			Severity:    SeverityHigh,
			Interval:    3600 * time.Second,
			ForDuration: 0,
			QuerySQL: fmt.Sprintf(`SELECT CASE WHEN
				(SELECT count(*) FROM scheduled_job_events
				   WHERE job_name = 'maintenance'
				     AND recorded_at > now() - interval '%d days') > 0
				AND
				(SELECT count(*) FROM scheduled_job_events
				   WHERE job_name = 'maintenance'
				     AND success = true
				     AND metadata->>'dry_run' = 'false'
				     AND recorded_at > now() - interval '%d days') = 0
				THEN 1 ELSE 0 END`, lookbackDays, lookbackDays),
			Threshold: 0,
			Operator:  "gt",
			Enabled:   true,
		},
	}
}

// CoverageRules returns the HIDDEN-CHURN-001 conversation-coverage rule:
// the primary space's themed/total observation ratio staying below the
// floor. The audited state was ~6% coverage (94% of observations never
// entered the hierarchy); the density-assignment retune should lift it —
// this rule keeps the gap visible instead of silent. floor ≤ 0 → 0.2.
func CoverageRules(floor float64) []AlertRule {
	if floor <= 0 {
		floor = 0.2
	}
	return []AlertRule{
		{
			ID:    "low_conversation_coverage",
			Title: "Conversation Coverage Below Floor — Hierarchy Missing Most Observations",
			// Distinct Service per the NOSILENT-001 cooldown-collision rule.
			Service:     "conversation-coverage",
			Severity:    SeverityMedium,
			Interval:    1800 * time.Second,
			ForDuration: 6 * time.Hour, // long: coverage converges over consolidation cycles
			QuerySQL: fmt.Sprintf(`SELECT coalesce(min(value), 1) FROM (
				SELECT DISTINCT ON (labels->>'space_id') value
				FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_conversation_coverage_ratio'
				  AND time > now() - interval '1 hour'
				ORDER BY labels->>'space_id', time DESC) latest
				WHERE value < %f`, floor),
			Threshold: floor,
			Operator:  "lt",
			Enabled:   true,
		},
	}
}
