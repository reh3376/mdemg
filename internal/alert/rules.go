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

// DefaultRules returns the server-native alert rules migrated from Grafana.
// SQL queries use raw TimescaleDB queries against the metric_samples table.
//
// The two orphan rules (high_orphan_count / high_orphan_ratio) were extracted
// to OrphanRules() by ORPHAN-ALERT-001 (they needed a config-driven min-node
// significance floor + deterministic idle-safe aggregation).
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
		// low_graph_health extracted to OrphanRules() (ORPHAN-ALERT-001 follow-up)
		// — it had the same `ORDER BY time DESC LIMIT 1` + no-floor defect: the
		// degenerate `global` test space (health 0.0) tripped it while mdemg-dev
		// was healthy (0.995).
		// high_orphan_count + high_orphan_ratio extracted to OrphanRules()
		// (ORPHAN-ALERT-001) — they needed a minimum-node significance floor
		// (1-node test spaces were tripping ratio=1.0) and deterministic
		// idle-safe aggregation, so they are config-parameterized + appended
		// in serve.go like the other parameterized rule groups.
		{
			// ALERT-TRUTH-001: LIMIT 1 → windowed AVG + COALESCE (TSDB-CONSUME-001
			// idle-safe contract). mem_percent is 0–100 normalized, so 80% is a
			// real fixed threshold; AVG over the window removes single-sample flap.
			ID:          "neo4j_high_memory",
			Title:       "MDEMG Neo4j High Memory Usage",
			Service:     "neo4j",
			Severity:    SeverityMedium,
			Interval:    30 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: `SELECT COALESCE(AVG(value), 0) AS mem_pct FROM metric_samples
				WHERE metric_name = 'mdemg_neo4j_container_mem_percent'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '5 minutes'`,
			Threshold: 80,
			Operator:  "gt",
			Enabled:   true,
		},
		// neo4j_high_cpu extracted to Neo4jCPURule() (ALERT-TRUTH-001): the
		// threshold needs to be config-driven + host-relative. The old fixed 80
		// was "% of ONE core" read via ORDER BY time DESC LIMIT 1 — on multi-core
		// hardware normal consolidation runs 200–837%, so it tripped on any real
		// graph work, and the single-sample read flapped on the 0/burst probe
		// pattern (7d p50=1, p95=255). Appended in serve.go like the other
		// config-parameterized rule groups.
		// graph_node_drop extracted to GraphNodeDropRule() by NODE-DROP-CALIBRATION-001
		// (mirror of ORPHAN-ALERT-001): the fixed 100-node threshold was 0.12% of a
		// mature 84k substrate and 20% of a 500-node scratch space, so every
		// operator-authorized recluster (routine 5–10% L1 tightening) tripped it as
		// CRITICAL, while a degenerate scratch space losing 3 nodes tripped it too.
		// Now split into ratio + absolute rules with a min-node significance floor,
		// SeverityHigh (was CRITICAL — reserved for data-loss emergencies), and
		// distinct Services (NOSILENT-001 cooldown-key contract). Appended in
		// serve.go like the other parameterized rule groups.
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
			// ALERT-TRUTH-001: LIMIT 1 → windowed AVG; COALESCE to 1.0 (healthy)
			// on an idle window so absence never fires this "lt" rule.
			QuerySQL: `SELECT COALESCE(AVG(value), 1.0) AS hit_ratio FROM metric_samples
				WHERE metric_name = 'mdemg_cache_hit_ratio'
				  AND metric_type = 'gauge'
				  AND labels->>'cache' = 'query'
				  AND time > now() - interval '10 minutes'`,
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
			// ALERT-TRUTH-001: LIMIT 1 → windowed AVG; COALESCE to 1.0 (healthy)
			// on an idle window so absence never fires this "lt" rule.
			QuerySQL: `SELECT COALESCE(AVG(value), 1.0) AS follow_rate FROM metric_samples
				WHERE metric_name = 'mdemg_jiminy_follow_rate'
				  AND metric_type = 'gauge'
				  AND time > now() - interval '30 minutes'`,
			Threshold: 0.3,
			Operator:  "lt",
			Enabled:   true,
		},
	}
}

// OrphanRules returns the graph-health orphan alerts (ORPHAN-ALERT-001).
//
// Replaces the two hardcoded DefaultRules entries (high_orphan_count /
// high_orphan_ratio) that produced chronic false positives:
//   - The ratio rule used `ORDER BY ratio DESC LIMIT 1` with NO node floor, so
//     a 1-node UATS/test space (1 orphan ⇒ ratio 1.0) tripped the 0.10
//     threshold while the real substrate was healthy (mdemg-dev 693/83034 =
//     0.8%). The count rule used `ORDER BY time DESC LIMIT 1` — whichever
//     space's gauge was written last, non-deterministic across spaces.
//
// Both rules now join the per-space orphans + nodes gauges, EXCLUDE spaces
// below `minNodes` (significance floor — tiny scratch/test spaces cannot fire
// a graph-health alert), and aggregate with COALESCE(MAX(...),0) so they
// ALWAYS return one non-NULL row (idle-safe; no `ORDER BY … LIMIT 1`, per the
// TSDB-CONSUME-001 alert-SQL contract). The gauges already exclude archived
// nodes, so the ratio is live-orphans / live-nodes.
//
// minNodes ≤ 0 → 50; ratioThreshold ≤ 0 → 0.10; countThreshold ≤ 0 → 1000
// (above mdemg-dev's accepted historical-orphan baseline — the ratio rule is
// the scale-aware primary signal; the count rule catches an absolute spike);
// healthFloor ≤ 0 → 0.5. The low_graph_health rule (ORPHAN-ALERT-001 follow-up)
// shares the same min-node floor + deterministic aggregation — it had the same
// `ORDER BY time DESC LIMIT 1`/no-floor defect (the degenerate `global` space's
// 0.0 health tripped it).
func OrphanRules(minNodes int, ratioThreshold float64, countThreshold int, healthFloor float64) []AlertRule {
	if minNodes <= 0 {
		minNodes = 50
	}
	if ratioThreshold <= 0 {
		ratioThreshold = 0.10
	}
	if countThreshold <= 0 {
		countThreshold = 1000
	}
	if healthFloor <= 0 {
		healthFloor = 0.5
	}
	// Shared per-space CTEs: latest orphan + node gauge per space, joined and
	// floor-gated. MAX picks the worst SIGNIFICANT space; COALESCE makes an
	// empty/idle window return 0 (a non-NULL row) instead of "no rows".
	cte := `WITH latest AS (
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
		)`
	return []AlertRule{
		{
			ID:    "high_orphan_count",
			Title: "MDEMG High Orphan Count",
			// Distinct Service per rule (NOSILENT-001 cooldown-key contract):
			// low_graph_health already owns "graph-health"; sharing it would
			// make the two alarms suppress each other.
			Service:     "graph-health-count",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 15 * time.Minute,
			QuerySQL: cte + fmt.Sprintf(`
				SELECT COALESCE(MAX(
				  CASE WHEN n.total_nodes >= %d THEN l.orphans ELSE 0 END
				), 0) AS max_orphans
				FROM latest l JOIN nodes n ON l.space_id = n.space_id`, minNodes),
			Threshold: float64(countThreshold),
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "high_orphan_ratio",
			Title:       "MDEMG High Orphan Ratio",
			Service:     "graph-health-ratio",
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 15 * time.Minute,
			QuerySQL: cte + fmt.Sprintf(`
				SELECT COALESCE(MAX(
				  CASE WHEN n.total_nodes >= %d AND n.total_nodes > 0
				    THEN l.orphans / n.total_nodes ELSE 0 END
				), 0) AS ratio
				FROM latest l JOIN nodes n ON l.space_id = n.space_id`, minNodes),
			Threshold: ratioThreshold,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "low_graph_health",
			Title:       "MDEMG Low Graph Health Score",
			Service:     "graph-health", // distinct from -count / -ratio
			Severity:    SeverityMedium,
			Interval:    60 * time.Second,
			ForDuration: 10 * time.Minute,
			// MIN health among SIGNIFICANT spaces (total_nodes >= minNodes);
			// COALESCE to 1.0 (healthy) when none — idle-safe, deterministic, no
			// `ORDER BY … LIMIT 1` (TSDB-CONSUME-001 contract). Degenerate 1-2
			// node test spaces (e.g. `global` at health 0.0) are excluded.
			QuerySQL: `WITH health AS (
				  SELECT DISTINCT ON (labels->>'space_id')
				    labels->>'space_id' AS space_id, value AS score
				  FROM metric_samples
				  WHERE metric_name = 'mdemg_neo4j_graph_health_score'
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
				SELECT COALESCE(MIN(
				  CASE WHEN n.total_nodes >= ` + fmt.Sprintf("%d", minNodes) + ` THEN h.score ELSE NULL END
				), 1.0) AS min_health
				FROM health h JOIN nodes n ON h.space_id = n.space_id`,
			Threshold: healthFloor,
			Operator:  "lt",
			Enabled:   true,
		},
	}
}

// GraphNodeDropRule returns the split graph_node_drop alerts (NODE-DROP-CALIBRATION-001).
//
// Replaces the hardcoded DefaultRules entry that produced chronic false
// positives:
//   - Fixed absolute threshold `> 100 nodes` was 0.12% of a mature 84k
//     substrate and 20% of a 500-node scratch space, so every
//     operator-authorized recluster tripped it as CRITICAL, and every
//     tombstone burst on a tiny space tripped it too.
//   - NO min-node significance floor — degenerate scratch spaces dominated.
//   - CRITICAL severity was overweight for identity-preserving pattern
//     cleanup; reserved for actual data-loss emergencies.
//
// Both rules join the per-space old vs current node-count gauges, EXCLUDE
// spaces below `minNodes` (significance floor), and aggregate with
// COALESCE(MAX(...),0) so they ALWAYS return one non-NULL row (idle-safe,
// no `ORDER BY … LIMIT 1`, per the TSDB-CONSUME-001 alert-SQL contract).
// Distinct Services per rule per the NOSILENT-001 cooldown-key contract.
//
// The comparison window is 60 min ago (55–65 min band) vs now (last 5 min),
// preserving the shipped rule's window.
//
// minNodes ≤ 0 → 50; ratioThreshold ≤ 0 → 0.10 (10%); absoluteThreshold ≤ 0
// → 10000 (~10× the largest operator-authorized recluster delta observed
// on mdemg-dev — catches mass loss on huge substrates where 10% would
// still be too large in absolute terms).
func GraphNodeDropRule(minNodes int, ratioThreshold float64, absoluteThreshold int) []AlertRule {
	if minNodes <= 0 {
		minNodes = 50
	}
	if ratioThreshold <= 0 {
		ratioThreshold = 0.10
	}
	if absoluteThreshold <= 0 {
		absoluteThreshold = 10000
	}
	// Shared CTEs: current + old snapshots by space, joined with the current
	// node-count gauge to gate on the min-node floor.
	cte := `WITH current_val AS (
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
		)`
	return []AlertRule{
		{
			ID:          "graph_node_drop_ratio",
			Title:       "MDEMG Significant Node Count Drop (Ratio)",
			Service:     "graph-node-drop-ratio",
			Severity:    SeverityHigh,
			Interval:    60 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: cte + fmt.Sprintf(`
				SELECT COALESCE(MAX(
				  CASE WHEN c.value >= %d AND o.value > 0 AND o.value > c.value
				    THEN (o.value - c.value) / o.value ELSE 0 END
				), 0) AS drop_ratio
				FROM old_val o JOIN current_val c ON o.space_id = c.space_id`, minNodes),
			Threshold: ratioThreshold,
			Operator:  "gt",
			Enabled:   true,
		},
		{
			ID:          "graph_node_drop_count",
			Title:       "MDEMG Significant Node Count Drop (Absolute)",
			Service:     "graph-node-drop-count",
			Severity:    SeverityHigh,
			Interval:    60 * time.Second,
			ForDuration: 5 * time.Minute,
			QuerySQL: cte + fmt.Sprintf(`
				SELECT COALESCE(MAX(
				  CASE WHEN c.value >= %d AND o.value > c.value
				    THEN (o.value - c.value) ELSE 0 END
				), 0) AS drop_count
				FROM old_val o JOIN current_val c ON o.space_id = c.space_id`, minNodes),
			Threshold: float64(absoluteThreshold),
			Operator:  "gt",
			Enabled:   true,
		},
	}
}

// ReadinessStalenessRule returns the FT-RECURSIVE-001 SF-1 rule: the RSIC
// training-readiness assessment emits a heartbeat gauge
// (mdemg_rsic_readiness_assessed) on every SUCCESSFUL run. If a silent query
// failure stops it, the loop goes dormant with no other signal — this rule
// fires when the most recent heartbeat is older than stalenessMin minutes.
//
// Idle-safe per the TSDB-CONSUME-001 alert-SQL contract: a wide window finds
// the last sample's real age; COALESCE returns a large staleness when the
// heartbeat is absent entirely (truly dormant), so the query always returns
// one non-NULL row. stalenessMin ≤ 0 falls back to 30.
func ReadinessStalenessRule(stalenessMin int) AlertRule {
	if stalenessMin <= 0 {
		stalenessMin = 30
	}
	return AlertRule{
		ID:          "training_readiness_stale",
		Title:       "MDEMG Training Readiness Assessment Stale",
		Service:     "ft-readiness",
		Severity:    SeverityMedium,
		Interval:    60 * time.Second,
		ForDuration: 5 * time.Minute,
		// FT-RECURSIVE-003 E1 (FTLOOP-DRILL-001 finding): while the recursive-
		// retrain compute lease is held, RSIC is deliberately quiesced and the
		// readiness heartbeat SHOULD pause — a held lease within the last 5
		// minutes suppresses the rule (staleness reads 0). MAX over the window
		// (idle-safe COALESCE, no ORDER BY…LIMIT 1 — the TSDB-CONSUME-001
		// contract).
		QuerySQL: `SELECT CASE WHEN COALESCE((
			    SELECT MAX(value) FROM metric_samples
			    WHERE metric_name = 'mdemg_ftloop_lease_held'
			      AND time > now() - interval '5 minutes'), 0) >= 1
			THEN 0
			ELSE COALESCE(
			    EXTRACT(EPOCH FROM (now() - MAX(time))) / 60.0, 1000000)
			END AS stale_minutes
			FROM metric_samples
			WHERE metric_name = 'mdemg_rsic_readiness_assessed'
			  AND metric_type = 'gauge'
			  AND time > now() - interval '24 hours'`,
		Threshold: float64(stalenessMin),
		Operator:  "gt",
		Enabled:   true,
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
//   - DASHBOARD-TRUTH-001: rows whose outcome_type is 'not_applicable' (or
//     blank/unknown) are excluded from BOTH numerator and denominator via the
//     outcome_type IN (...) allowlist — n/a rows carry no follow/ignore verdict
//     (~35% of actionable rows live), and counting them as 0.0 understated the
//     rate (0.094 vs 0.145 excl n/a) and could false-fire this rule. Keeps the
//     rule in agreement with the dashboard's Should-Follow Follow Rate panel.
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
			Title:       "Actionable Compliance Rate Low",
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
				  AND guidance_type IN ('constraint', 'correction')
				  AND outcome_type IN ('followed', 'partial_compliance', 'ignored', 'contradicted')`, lookbackHours),
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

// FTBenchmarkStalenessRule alerts when the FT `benchmark_runs` table has no
// completed run in the window (FT-BENCH-REFRESH-001). Reads `benchmark_runs`
// directly (not via scheduled_job_events) because the benchmark writer is a
// standalone Python CLI (`neural.benchmarks.run_benchmark --persist-tsdb`)
// not a Go scheduled job. Nothing schedules the benchmark automatically today
// (FT-RECURSIVE-002 default-off); this rule ensures operator awareness when
// the dashboard's "Latest Run" panel starts reading a stale row.
//
// Idle-safe SQL: aggregate + COALESCE, no ORDER BY … LIMIT 1
// (TSDB-CONSUME-001 contract).
// Distinct Service label (NOSILENT-001 cooldown-key contract).
func FTBenchmarkStalenessRule(stalenessDays int) AlertRule {
	if stalenessDays <= 0 {
		stalenessDays = 7
	}
	return AlertRule{
		ID:          "ft_benchmark_stale",
		Title:       "FT Benchmark Not Refreshed",
		Service:     "ft-benchmark",
		Severity:    SeverityHigh,
		Interval:    1 * time.Hour,
		ForDuration: 0,
		QuerySQL: `SELECT COALESCE(EXTRACT(EPOCH FROM (now() - MAX(completed_at))) / 86400.0, 999.0) AS age_days
			FROM benchmark_runs
			WHERE completed_at IS NOT NULL`,
		Threshold: float64(stalenessDays),
		Operator:  "gt",
		Enabled:   true,
	}
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

// Neo4jCPURule returns the Neo4j container CPU alert (ALERT-TRUTH-001).
//
// docker-stats CPU% is reported relative to a SINGLE core, so on multi-core
// hardware healthy parallel graph work (consolidation, Hebbian writes) routinely
// runs several hundred percent — the old fixed `80` tripped on any real activity.
// The probe also alternates near-zero/burst samples (live 7d p50=1, p95=255), so
// a single `ORDER BY time DESC LIMIT 1` read flapped. This rule instead evaluates
// the AVG over the 5-minute window (idle-safe via COALESCE) against a host-relative
// threshold. Calibrated default 500: the worst NORMAL-operation 5-min windowed AVG
// observed in 24h was 471 (heavy consolidation); 0 windows exceeded 500 — so it
// fires only on load worse than any normal consolidation. Operators on smaller
// machines lower NEO4J_CPU_ALERT_THRESHOLD_PERCENT. Reducing the consolidation
// cost itself is a separate concern (it is expected to be CPU-heavy).
func Neo4jCPURule(thresholdPercent float64) AlertRule {
	if thresholdPercent <= 0 {
		thresholdPercent = 500
	}
	return AlertRule{
		ID:          "neo4j_high_cpu",
		Title:       "MDEMG Neo4j High CPU Usage",
		Service:     "neo4j-cpu", // distinct Service (NOSILENT-001 cooldown-key rule)
		Severity:    SeverityMedium,
		Interval:    30 * time.Second,
		ForDuration: 5 * time.Minute,
		QuerySQL: `SELECT COALESCE(AVG(value), 0) AS cpu_pct FROM metric_samples
			WHERE metric_name = 'mdemg_neo4j_container_cpu_percent'
			  AND metric_type = 'gauge'
			  AND time > now() - interval '5 minutes'`,
		Threshold: thresholdPercent,
		Operator:  "gt",
		Enabled:   true,
	}
}

// FtLoopNeverRanRule fires when the recursive-retrain actuator is enabled
// but the cycle ledger has seen NO events within the staleness window — the
// "loop silently dormant" guarantee (FT-RECURSIVE-004 E1; SPEC §3 Monitor).
// Purpose-specific rule over ft_training_cycles (the FT-BENCH-REFRESH-001
// lesson: tables not written by Go scheduled jobs get their own rule, never
// forced through scheduled_job_events). Idle-safe: aggregate + COALESCE
// always returns one row; 999 when the ledger is empty. The caller wires it
// ONLY when FT_LOOP_ENABLED — a disabled actuator must not nag.
func FtLoopNeverRanRule(stalenessDays int) AlertRule {
	if stalenessDays <= 0 {
		stalenessDays = 14
	}
	return AlertRule{
		ID:          "ft_loop_never_ran",
		Title:       "MDEMG FT Loop Has Not Run",
		Service:     "ft-loop-staleness",
		Severity:    SeverityMedium,
		Interval:    time.Hour,
		ForDuration: 5 * time.Minute,
		QuerySQL: `SELECT COALESCE(
			    EXTRACT(EPOCH FROM (now() - MAX(time))) / 86400.0, 999.0) AS stale_days
			FROM ft_training_cycles`,
		Threshold: float64(stalenessDays),
		Operator:  "gt",
		Enabled:   true,
	}
}

// FtProductionDriftRule fires when the latest benchmark aggregate has
// fallen more than margin below the ACTIVE model version's recorded score
// (FT-RECURSIVE-004 E2; SPEC §3 Monitor). DH-004 no-data gates: an active
// score <= 0 or an empty benchmark_runs yields 0 (no drift) — never a
// false fire from missing data. The scalar reads avoid the ORDER BY…LIMIT 1
// literal via MAX(completed_at) correlation (idle-safe under COALESCE).
func FtProductionDriftRule(margin float64) AlertRule {
	if margin <= 0 {
		margin = 0.05
	}
	return AlertRule{
		ID:          "ft_production_drift",
		Title:       "MDEMG FT Production Drift",
		Service:     "ft-production-drift",
		Severity:    SeverityHigh,
		Interval:    time.Hour,
		ForDuration: 5 * time.Minute,
		QuerySQL: `SELECT CASE
			WHEN COALESCE((SELECT MAX(overall_score) FROM ft_model_versions WHERE status = 'active'), 0) <= 0 THEN 0
			WHEN (SELECT MAX(completed_at) FROM benchmark_runs) IS NULL THEN 0
			ELSE GREATEST(0,
			    COALESCE((SELECT MAX(overall_score) FROM ft_model_versions WHERE status = 'active'), 0)
			  - COALESCE((SELECT MAX(aggregate_weighted_score) FROM benchmark_runs
			              WHERE completed_at = (SELECT MAX(completed_at) FROM benchmark_runs)), 0))
			END AS drift`,
		Threshold: margin,
		Operator:  "gt",
		Enabled:   true,
	}
}
