package alert

import "time"

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
