package tsdb

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DatasetProvider is the interface consumed by RSIC for trend analysis.
// Allows mocking in tests without a database.
type DatasetProvider interface {
	LLMPerformance(ctx context.Context, spaceID string, window time.Duration) ([]LLMPerformanceSummary, error)
	RetrievalQuality(ctx context.Context, spaceID string, window time.Duration) (*RetrievalQualitySummary, error)
	EmbeddingCoverage(ctx context.Context, spaceID string, window time.Duration) (*EmbeddingCoverageSummary, error)
	MetricTrend(ctx context.Context, spaceID, metricName string, window time.Duration) (*MetricTrend, error)
	TrainingDataReadiness(ctx context.Context) (*TrainingDataReadiness, error)
}

// LLMPerformanceSummary holds per-task LLM performance metrics.
type LLMPerformanceSummary struct {
	TaskName        string        `json:"task_name"`
	TotalCalls      int           `json:"total_calls"`
	ErrorRate       float64       `json:"error_rate"`
	LatencyP50      float64       `json:"latency_p50"`
	LatencyP95      float64       `json:"latency_p95"`
	AvgTokensIn     float64       `json:"avg_tokens_in"`
	AvgTokensOut    float64       `json:"avg_tokens_out"`
	DistinctModels  int           `json:"distinct_models"`
	HasSystemPrompt bool          `json:"has_system_prompt"`
	HasRAFTContext  bool          `json:"has_raft_context"`
	LastErrorAt     time.Time     `json:"last_error_at,omitzero"` // SUPERVISOR-002: most recent errored call in window (zero = none)
	Window          time.Duration `json:"window"`
}

// RetrievalQualitySummary holds retrieval pipeline quality metrics.
type RetrievalQualitySummary struct {
	TotalQueries           int           `json:"total_queries"`
	RecallRate             float64       `json:"recall_rate"`
	BM25Rate               float64       `json:"bm25_rate"`
	RerankRate             float64       `json:"rerank_rate"`
	GuidanceCorrelation    float64       `json:"guidance_correlation"`
	AvgRecallK             float64       `json:"avg_recall_k"`
	AvgResultCount         float64       `json:"avg_result_count"`
	LatencyP50Total        float64       `json:"latency_p50_total"`
	LatencyP95Total        float64       `json:"latency_p95_total"`
	LatencyP50Rerank       float64       `json:"latency_p50_rerank"`
	HardNegativeCandidates int           `json:"hard_negative_candidates"`
	Window                 time.Duration `json:"window"`
}

// EmbeddingCoverageSummary holds embedding pipeline coverage metrics.
type EmbeddingCoverageSummary struct {
	TotalEvents    int            `json:"total_events"`
	CacheHitRate   float64        `json:"cache_hit_rate"`
	CallSites      map[string]int `json:"call_sites"`
	EventTypes     map[string]int `json:"event_types"`
	EmptyCallSites int            `json:"empty_call_sites"`
	UniqueTexts    int            `json:"unique_texts"`
	Window         time.Duration  `json:"window"`
}

// MetricTrend holds trend analysis for a single metric.
type MetricTrend struct {
	MetricName string        `json:"metric_name"`
	Points     []MetricPoint `json:"points"`
	Slope      float64       `json:"slope"`
	AvgValue   float64       `json:"avg_value"`
	MinValue   float64       `json:"min_value"`
	MaxValue   float64       `json:"max_value"`
	Volatility float64       `json:"volatility"`
	Window     time.Duration `json:"window"`
}

// TaskReadiness holds training data readiness for a single task.
type TaskReadiness struct {
	TaskName             string    `json:"task_name"`
	TotalRows            int       `json:"total_rows"`
	HasSystemPrompt      int       `json:"has_system_prompt"`
	HasResponse          int       `json:"has_response"`
	HasQuality           int       `json:"has_quality"`
	ErrorCount           int       `json:"error_count"`
	DistinctPromptHashes int       `json:"distinct_prompt_hashes"`
	OldestRecord         time.Time `json:"oldest_record"`
	NewestRecord         time.Time `json:"newest_record"`
	Ready                bool      `json:"ready"`
	AvgDailyRate         float64   `json:"avg_daily_rate"`
	ProjectedDaysToReady int       `json:"projected_days_to_ready"`
}

// TrainingDataReadiness holds readiness assessments across all tasks.
type TrainingDataReadiness struct {
	Tasks []TaskReadiness `json:"tasks"`
}

// DefaultReadinessThreshold is the minimum rows per task to consider ready.
const DefaultReadinessThreshold = 500

// DatasetBuilder provides curated, structured queries against TSDB tables.
// Used by RSIC for trend analysis and by FT pipeline for training data curation.
type DatasetBuilder struct {
	pool               *pgxpool.Pool
	readinessThreshold int
}

// NewDatasetBuilder creates a DatasetBuilder from a connection pool.
func NewDatasetBuilder(pool *pgxpool.Pool) *DatasetBuilder {
	return &DatasetBuilder{
		pool:               pool,
		readinessThreshold: DefaultReadinessThreshold,
	}
}

// LLMPerformance returns per-task LLM performance summaries for the given window.
func (b *DatasetBuilder) LLMPerformance(ctx context.Context, spaceID string, window time.Duration) ([]LLMPerformanceSummary, error) {
	cutoff := time.Now().Add(-window)

	query := `
		SELECT
			task_name,
			COUNT(*)::int AS total_calls,
			COUNT(*) FILTER (WHERE error IS NOT NULL AND error != '')::int AS error_count,
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms IS NOT NULL), 0) AS latency_p50,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE latency_ms IS NOT NULL), 0) AS latency_p95,
			COALESCE(AVG(tokens_in)::float8, 0) AS avg_tokens_in,
			COALESCE(AVG(tokens_out)::float8, 0) AS avg_tokens_out,
			COUNT(DISTINCT model_name)::int AS distinct_models,
			COUNT(*) FILTER (WHERE system_prompt IS NOT NULL AND system_prompt != '')::int AS has_sys_prompt_count,
			COUNT(*) FILTER (WHERE retrieval_node_ids IS NOT NULL AND array_length(retrieval_node_ids, 1) > 0)::int AS has_raft_count,
			MAX(time) FILTER (WHERE error IS NOT NULL AND error != '') AS last_error_at
		FROM llm_interactions
		WHERE space_id = $1 AND time >= $2
		GROUP BY task_name
		ORDER BY total_calls DESC`

	rows, err := b.pool.Query(ctx, query, spaceID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("dataset_builder: llm_performance: %w", err)
	}
	defer rows.Close()

	var results []LLMPerformanceSummary
	for rows.Next() {
		var s LLMPerformanceSummary
		var errorCount, hasSysCount, hasRaftCount int
		var lastErrorAt *time.Time
		if err := rows.Scan(
			&s.TaskName, &s.TotalCalls, &errorCount,
			&s.LatencyP50, &s.LatencyP95,
			&s.AvgTokensIn, &s.AvgTokensOut,
			&s.DistinctModels, &hasSysCount, &hasRaftCount,
			&lastErrorAt,
		); err != nil {
			return nil, fmt.Errorf("dataset_builder: llm_performance scan: %w", err)
		}
		if lastErrorAt != nil {
			s.LastErrorAt = *lastErrorAt
		}
		if s.TotalCalls > 0 {
			s.ErrorRate = float64(errorCount) / float64(s.TotalCalls)
		}
		s.HasSystemPrompt = hasSysCount == s.TotalCalls
		s.HasRAFTContext = hasRaftCount > 0
		s.Window = window
		results = append(results, s)
	}
	return results, rows.Err()
}

// RetrievalQuality returns retrieval pipeline quality summary for the given window.
func (b *DatasetBuilder) RetrievalQuality(ctx context.Context, spaceID string, window time.Duration) (*RetrievalQualitySummary, error) {
	cutoff := time.Now().Add(-window)

	query := `
		SELECT
			COUNT(*)::int AS total_queries,
			COUNT(*) FILTER (WHERE recall_node_ids IS NOT NULL AND array_length(recall_node_ids, 1) > 0)::int AS has_recall,
			COUNT(*) FILTER (WHERE bm25_node_ids IS NOT NULL AND array_length(bm25_node_ids, 1) > 0)::int AS has_bm25,
			COUNT(*) FILTER (WHERE rerank_node_ids IS NOT NULL AND array_length(rerank_node_ids, 1) > 0)::int AS has_rerank,
			COUNT(*) FILTER (WHERE guidance_id IS NOT NULL AND guidance_id != '')::int AS has_guidance,
			COALESCE(AVG(recall_k)::float8, 0) AS avg_recall_k,
			COALESCE(AVG(result_count)::float8, 0) AS avg_result_count,
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY total_latency_ms) FILTER (WHERE total_latency_ms IS NOT NULL), 0) AS latency_p50_total,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY total_latency_ms) FILTER (WHERE total_latency_ms IS NOT NULL), 0) AS latency_p95_total,
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY rerank_latency_ms) FILTER (WHERE rerank_latency_ms IS NOT NULL), 0) AS latency_p50_rerank,
			COUNT(*) FILTER (WHERE recall_scores IS NOT NULL AND array_length(recall_scores, 1) > 0
			                   AND rerank_scores IS NOT NULL AND array_length(rerank_scores, 1) > 0)::int AS hard_negative_candidates
		FROM retrieval_events
		WHERE space_id = $1 AND time >= $2`

	var s RetrievalQualitySummary
	var hasRecall, hasBM25, hasRerank, hasGuidance int
	err := b.pool.QueryRow(ctx, query, spaceID, cutoff).Scan(
		&s.TotalQueries, &hasRecall, &hasBM25, &hasRerank, &hasGuidance,
		&s.AvgRecallK, &s.AvgResultCount,
		&s.LatencyP50Total, &s.LatencyP95Total, &s.LatencyP50Rerank,
		&s.HardNegativeCandidates,
	)
	if err != nil {
		return nil, fmt.Errorf("dataset_builder: retrieval_quality: %w", err)
	}

	if s.TotalQueries > 0 {
		s.RecallRate = float64(hasRecall) / float64(s.TotalQueries)
		s.BM25Rate = float64(hasBM25) / float64(s.TotalQueries)
		s.RerankRate = float64(hasRerank) / float64(s.TotalQueries)
		s.GuidanceCorrelation = float64(hasGuidance) / float64(s.TotalQueries)
	}
	s.Window = window
	return &s, nil
}

// EmbeddingCoverage returns embedding pipeline coverage summary for the given window.
func (b *DatasetBuilder) EmbeddingCoverage(ctx context.Context, spaceID string, window time.Duration) (*EmbeddingCoverageSummary, error) {
	cutoff := time.Now().Add(-window)

	// Main aggregation
	mainQuery := `
		SELECT
			COUNT(*)::int AS total_events,
			COUNT(*) FILTER (WHERE cached = true)::int AS cache_hits,
			COUNT(*) FILTER (WHERE call_site IS NULL OR call_site = '')::int AS empty_call_sites,
			COUNT(DISTINCT text_hash)::int AS unique_texts
		FROM embedding_events
		WHERE space_id = $1 AND time >= $2`

	var s EmbeddingCoverageSummary
	var cacheHits int
	err := b.pool.QueryRow(ctx, mainQuery, spaceID, cutoff).Scan(
		&s.TotalEvents, &cacheHits, &s.EmptyCallSites, &s.UniqueTexts,
	)
	if err != nil {
		return nil, fmt.Errorf("dataset_builder: embedding_coverage: %w", err)
	}

	if s.TotalEvents > 0 {
		s.CacheHitRate = float64(cacheHits) / float64(s.TotalEvents)
	}

	// Call site distribution
	s.CallSites = make(map[string]int)
	csRows, err := b.pool.Query(ctx,
		`SELECT COALESCE(NULLIF(call_site, ''), '<empty>'), COUNT(*)::int
		 FROM embedding_events WHERE space_id = $1 AND time >= $2
		 GROUP BY call_site ORDER BY count DESC`, spaceID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("dataset_builder: embedding call_sites: %w", err)
	}
	defer csRows.Close()
	for csRows.Next() {
		var site string
		var count int
		if err := csRows.Scan(&site, &count); err != nil {
			return nil, fmt.Errorf("dataset_builder: embedding call_sites scan: %w", err)
		}
		s.CallSites[site] = count
	}
	if err := csRows.Err(); err != nil {
		return nil, err
	}

	// Event type distribution
	s.EventTypes = make(map[string]int)
	etRows, err := b.pool.Query(ctx,
		`SELECT event_type, COUNT(*)::int
		 FROM embedding_events WHERE space_id = $1 AND time >= $2
		 GROUP BY event_type ORDER BY count DESC`, spaceID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("dataset_builder: embedding event_types: %w", err)
	}
	defer etRows.Close()
	for etRows.Next() {
		var etype string
		var count int
		if err := etRows.Scan(&etype, &count); err != nil {
			return nil, fmt.Errorf("dataset_builder: embedding event_types scan: %w", err)
		}
		s.EventTypes[etype] = count
	}
	if err := etRows.Err(); err != nil {
		return nil, err
	}

	s.Window = window
	return &s, nil
}

// MetricTrend returns trend analysis for a metric over a time window.
// Auto-selects granularity: raw (<24h), hourly (1-7d), daily (>7d).
// Computes linear regression slope and volatility (coefficient of variation).
func (b *DatasetBuilder) MetricTrend(ctx context.Context, spaceID, metricName string, window time.Duration) (*MetricTrend, error) {
	now := time.Now()
	from := now.Add(-window)

	// Auto-select granularity
	var query string
	switch {
	case window <= 24*time.Hour:
		query = `SELECT time, value, 0::float8 AS min_value, 0::float8 AS max_value, 1::bigint AS count, quality_tag
			FROM metric_samples
			WHERE metric_name = $1 AND space_id = $2 AND time >= $3 AND time <= $4
			ORDER BY time`
	case window <= 7*24*time.Hour:
		query = `SELECT bucket AS time, avg_value AS value, min_value, max_value, sample_count AS count, '' AS quality_tag
			FROM metrics_hourly
			WHERE metric_name = $1 AND space_id = $2 AND bucket >= $3 AND bucket <= $4
			ORDER BY bucket`
	default:
		query = `SELECT bucket AS time, avg_value AS value, min_value, max_value, sample_count AS count, '' AS quality_tag
			FROM metrics_daily
			WHERE metric_name = $1 AND space_id = $2 AND bucket >= $3 AND bucket <= $4
			ORDER BY bucket`
	}

	rows, err := b.pool.Query(ctx, query, metricName, spaceID, from, now)
	if err != nil {
		return nil, fmt.Errorf("dataset_builder: metric_trend query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.Time, &p.Value, &p.MinValue, &p.MaxValue, &p.Count, &p.QualityTag); err != nil {
			return nil, fmt.Errorf("dataset_builder: metric_trend scan: %w", err)
		}
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	trend := &MetricTrend{
		MetricName: metricName,
		Points:     points,
		Window:     window,
	}

	if len(points) > 0 {
		trend.Slope, trend.AvgValue, trend.MinValue, trend.MaxValue, trend.Volatility = computeTrendStats(points)
	}

	return trend, nil
}

// computeTrendStats computes slope (linear regression), average, min, max, and volatility.
func computeTrendStats(points []MetricPoint) (slope, avg, min, max, volatility float64) {
	n := float64(len(points))
	if n == 0 {
		return 0, 0, 0, 0, 0
	}

	min = points[0].Value
	max = points[0].Value
	var sumVal float64

	for _, p := range points {
		sumVal += p.Value
		if p.Value < min {
			min = p.Value
		}
		if p.Value > max {
			max = p.Value
		}
	}
	avg = sumVal / n

	// Standard deviation for volatility (coefficient of variation = stddev/mean)
	var sumSqDiff float64
	for _, p := range points {
		diff := p.Value - avg
		sumSqDiff += diff * diff
	}
	stddev := math.Sqrt(sumSqDiff / n)
	if avg != 0 {
		volatility = stddev / math.Abs(avg)
	}

	// Linear regression: slope = (n*sum(x*y) - sum(x)*sum(y)) / (n*sum(x^2) - (sum(x))^2)
	// Use index as x (0, 1, 2, ...) for simplicity
	if n < 2 {
		return slope, avg, min, max, volatility
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i, p := range points {
		x := float64(i)
		sumX += x
		sumY += p.Value
		sumXY += x * p.Value
		sumX2 += x * x
	}
	denom := n*sumX2 - sumX*sumX
	if denom != 0 {
		slope = (n*sumXY - sumX*sumY) / denom
	}

	return slope, avg, min, max, volatility
}

// TrainingDataReadiness returns readiness assessments for all tasks in llm_interactions.
func (b *DatasetBuilder) TrainingDataReadiness(ctx context.Context) (*TrainingDataReadiness, error) {
	query := `
		SELECT
			task_name,
			COUNT(*)::int AS total_rows,
			COUNT(*) FILTER (WHERE system_prompt IS NOT NULL AND system_prompt != '')::int AS has_system_prompt,
			COUNT(*) FILTER (WHERE response IS NOT NULL AND response != '')::int AS has_response,
			COUNT(*) FILTER (WHERE quality IS NOT NULL)::int AS has_quality,
			COUNT(*) FILTER (WHERE error IS NOT NULL AND error != '')::int AS error_count,
			COUNT(DISTINCT system_prompt_hash)::int AS distinct_prompt_hashes,
			MIN(time) AS oldest_record,
			MAX(time) AS newest_record
		FROM llm_interactions
		GROUP BY task_name
		ORDER BY total_rows DESC`

	rows, err := b.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("dataset_builder: training_readiness: %w", err)
	}
	defer rows.Close()

	var tasks []TaskReadiness
	for rows.Next() {
		var t TaskReadiness
		if err := rows.Scan(
			&t.TaskName, &t.TotalRows,
			&t.HasSystemPrompt, &t.HasResponse, &t.HasQuality,
			&t.ErrorCount, &t.DistinctPromptHashes,
			&t.OldestRecord, &t.NewestRecord,
		); err != nil {
			return nil, fmt.Errorf("dataset_builder: training_readiness scan: %w", err)
		}
		// Ready if: sufficient rows, low error rate, all have system prompt
		errorRate := 0.0
		if t.TotalRows > 0 {
			errorRate = float64(t.ErrorCount) / float64(t.TotalRows)
		}
		t.Ready = t.TotalRows >= b.readinessThreshold &&
			errorRate < 0.05 &&
			t.HasSystemPrompt == t.TotalRows

		// Compute accumulation rate: rows per day over the data span
		if !t.OldestRecord.IsZero() && !t.NewestRecord.IsZero() && t.TotalRows > 0 {
			daySpan := t.NewestRecord.Sub(t.OldestRecord).Hours() / 24.0
			if daySpan > 0 {
				t.AvgDailyRate = float64(t.TotalRows) / daySpan
				if t.TotalRows < b.readinessThreshold && t.AvgDailyRate > 0 {
					remaining := float64(b.readinessThreshold - t.TotalRows)
					t.ProjectedDaysToReady = int(remaining/t.AvgDailyRate) + 1
				}
			}
		}

		tasks = append(tasks, t)
	}

	return &TrainingDataReadiness{Tasks: tasks}, rows.Err()
}
