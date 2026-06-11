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

// DefaultRules returns the 13 server-native alert rules migrated from Grafana.
// SQL queries use raw TimescaleDB queries against the metric_samples table.
func DefaultRules() []AlertRule {
	return []AlertRule{
		{
			ID:          "high_p95_latency",
			Title:       "MDEMG P95 Latency Exceeds SLO",
			Service:     "latency-slo",
			Severity:    SeverityMedium,
			Interval:    30 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_http_request_duration_seconds_p95'
				  AND metric_type = 'gauge'
				  AND labels->>'path' = '/v1/memory/retrieve'
				  AND time > now() - interval '5 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 0.250,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "critical_p99_latency",
			Title:       "MDEMG P99 Latency Critical",
			Service:     "latency-slo",
			Severity:    SeverityCritical,
			Interval:    30 * time.Second,
			ForDuration: 2 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_http_request_duration_seconds_p99'
				  AND metric_type = 'gauge'
				  AND labels->>'path' = '/v1/memory/retrieve'
				  AND time > now() - interval '5 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 0.500,
			Operator:  "gt",
			Enabled:   true,
		},
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
			ID:          "neo4j_pool_exhausted",
			Title:       "MDEMG Neo4j Connection Pool Exhausted",
			Service:     "neo4j",
			Severity:    SeverityCritical,
			Interval:    30 * time.Second,
			ForDuration: 2 * time.Minute,
			QuerySQL: `SELECT value FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_pool_waiting_requests'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '2 minutes'
				ORDER BY time DESC LIMIT 1`,
			Threshold: 5,
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
		rules = append(rules, AlertRule{
			ID:          "backup_no_recent_success",
			Title:       "No Successful TSDB Backup In Window",
			Service:     "scheduled-job-staleness",
			Severity:    SeverityHigh,
			Interval:    5 * time.Minute,
			ForDuration: 0,
			QuerySQL: fmt.Sprintf(`SELECT count(*) FROM scheduled_job_events
				WHERE job_name = 'tsdb-backup' AND success = true
				  AND recorded_at > now() - interval '%d hours'`, stalenessHours),
			Threshold: 0.5, // < 0.5 ⇒ zero successes ⇒ stale
			Operator:  "lt",
			Enabled:   true,
		})
	}

	return rules
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
