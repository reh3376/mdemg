package retrieval

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

// QueryMetrics tracks query execution statistics.
type QueryMetrics struct {
	TotalQueries    atomic.Int64
	SlowQueries     atomic.Int64
	TotalDurationNs atomic.Int64
}

var globalMetrics = &QueryMetrics{}

// SlowQueryThreshold is the duration above which queries are logged as slow.
const SlowQueryThreshold = 100 * time.Millisecond

// QueryTimer helps track query execution time.
type QueryTimer struct {
	name      string
	cypher    string
	params    map[string]any
	startTime time.Time
}

// StartQueryTimer creates a new query timer.
func StartQueryTimer(name, cypher string, params map[string]any) *QueryTimer {
	return &QueryTimer{
		name:      name,
		cypher:    cypher,
		params:    params,
		startTime: time.Now(),
	}
}

// End stops the timer and logs if the query was slow.
// Returns the duration for use in debug output.
func (qt *QueryTimer) End() time.Duration {
	duration := time.Since(qt.startTime)
	globalMetrics.TotalQueries.Add(1)
	globalMetrics.TotalDurationNs.Add(duration.Nanoseconds())

	if duration >= SlowQueryThreshold {
		globalMetrics.SlowQueries.Add(1)
		// Log slow query with sanitized params (exclude large arrays like embeddings)
		sanitizedParams := sanitizeParams(qt.params)
		slog.Warn("slow query detected", "name", qt.name, "duration_ms", duration.Milliseconds(), "cypher", compactCypher(qt.cypher), "params", sanitizedParams)
	}

	return duration
}

// TimeQuery is a helper to time a function and log slow execution.
func TimeQuery(ctx context.Context, name string, cypher string, params map[string]any, fn func() error) (time.Duration, error) {
	timer := StartQueryTimer(name, cypher, params)
	err := fn()
	duration := timer.End()
	return duration, err
}

// GetQueryMetrics returns current query execution metrics.
func GetQueryMetrics() map[string]any {
	total := globalMetrics.TotalQueries.Load()
	slow := globalMetrics.SlowQueries.Load()
	totalDuration := globalMetrics.TotalDurationNs.Load()

	avgDuration := int64(0)
	if total > 0 {
		avgDuration = totalDuration / total / 1e6 // Convert to ms
	}

	return map[string]any{
		"total_queries":     total,
		"slow_queries":      slow,
		"slow_query_pct":    float64(slow) / float64(max(total, 1)) * 100,
		"avg_duration_ms":   avgDuration,
		"total_duration_ms": totalDuration / 1e6,
	}
}

// ResetQueryMetrics resets the query metrics counters.
func ResetQueryMetrics() {
	globalMetrics.TotalQueries.Store(0)
	globalMetrics.SlowQueries.Store(0)
	globalMetrics.TotalDurationNs.Store(0)
}

// sanitizeParams removes large values (like embeddings) from params for logging.
func sanitizeParams(params map[string]any) map[string]any {
	result := make(map[string]any, len(params))
	for k, v := range params {
		switch val := v.(type) {
		case []float32:
			result[k] = "[float32 array, len=" + string(rune('0'+len(val)/100)) + "...]"
		case []float64:
			result[k] = "[float64 array, len=" + string(rune('0'+len(val)/100)) + "...]"
		case []any:
			if len(val) > 10 {
				result[k] = "[array, len=" + string(rune('0'+len(val)/10)) + "0+...]"
			} else {
				result[k] = val
			}
		default:
			result[k] = v
		}
	}
	return result
}

// compactCypher removes extra whitespace from Cypher for logging.
func compactCypher(cypher string) string {
	// Replace multiple spaces/newlines with single space
	lines := strings.Split(cypher, "\n")
	var parts []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	result := strings.Join(parts, " ")
	// Truncate very long queries
	if len(result) > 200 {
		return result[:200] + "..."
	}
	return result
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
