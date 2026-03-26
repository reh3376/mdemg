package metrics

import (
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/db"
	"mdemg/internal/ratelimit"
)

// StandardMetrics holds pre-registered standard metrics.
type StandardMetrics struct {
	registry *Registry

	// HTTP metrics
	HTTPRequestsTotal   func(method, path, status string) *Counter
	HTTPRequestDuration func(method, path string) *Histogram

	// Retrieval metrics
	RetrievalLatency    *Histogram
	RetrievalCacheHits  *Counter
	RetrievalCacheMiss  *Counter

	// Rate limiting metrics
	RateLimitRejected *Counter

	// Circuit breaker metrics (by service)
	CircuitBreakerState func(service string) *Gauge

	// Cache metrics
	CacheHitRatio func(cache string) *Gauge

	// Embedding metrics
	EmbeddingLatency *Histogram
	EmbeddingBatches *Counter

	// Neo4j pool metrics (Phase 48.4.1)
	// Note: Using gauges for all metrics since we read absolute values from driver
	Neo4jPoolActive   *Gauge // Current active connections
	Neo4jPoolIdle     *Gauge // Current idle connections
	Neo4jPoolWaiting  *Gauge // Waiting requests
	Neo4jPoolAcquired *Gauge // Total connections acquired (absolute value from driver)
	Neo4jPoolCreated  *Gauge // Total connections created (absolute value from driver)
	Neo4jPoolClosed   *Gauge // Total connections closed (absolute value from driver)
	Neo4jPoolFailed   *Gauge // Total failed acquire attempts (absolute value from driver)

	// Memory pressure metrics (Phase 48.4.4)
	MemoryPressureRejected *Gauge // Requests rejected due to memory pressure (cumulative)
	MemoryHeapBytes        *Gauge // Current heap allocation in bytes

	// CMS observation lifecycle metrics (CMS Hardening)
	CMSObserveTotal      func(outcome string) *Counter // "success", "degraded", "deduplicated"
	CMSEmbeddingFailures *Counter
	CMSDedupSkips        *Counter
	CMSDedupMergeFails   *Counter

	// CMS retrieval metrics
	CMSRecallTotal  *Counter
	CMSResumeTotal  *Counter

	// CMS error metrics
	CMSWriteJSONFails       *Counter
	CMSLearningEdgeFails    *Counter
	CMSStabilityUpdateFails *Counter

	// Neo4j graph per-space metrics (Grafana Neo4j Dashboard)
	Neo4jGraphNodes         func(spaceID string) *Gauge
	Neo4jGraphEdges         func(spaceID string) *Gauge
	Neo4jGraphObservations  func(spaceID string) *Gauge
	Neo4jGraphOrphans       func(spaceID string) *Gauge
	Neo4jGraphHealthScore   func(spaceID string) *Gauge
	Neo4jGraphLearningEdges func(spaceID string) *Gauge

	// Neo4j graph totals
	Neo4jGraphTotalNodes  *Gauge
	Neo4jGraphTotalEdges  *Gauge
	Neo4jGraphTotalSpaces *Gauge

	// Neo4j container resource metrics (via docker stats)
	Neo4jContainerCPUPercent *Gauge
	Neo4jContainerMemUsed   *Gauge
	Neo4jContainerMemLimit  *Gauge
	Neo4jContainerMemPercent *Gauge

	// RSIC cycle metrics (Phase 91)
	RSICCycleTotal    func(tier, source, outcome string) *Counter
	RSICCycleDuration func(tier string) *Histogram

	// RSIC trigger orchestration
	RSICTriggerRejected func(source, reason string) *Counter

	// RSIC action metrics
	RSICActionTotal    func(action, status string) *Counter
	RSICActionDuration func(action string) *Histogram

	// RSIC safety metrics
	RSICSafetyBlocked func(action, reason string) *Counter

	// RSIC watchdog metrics
	RSICWatchdogDecay      func(spaceID string) *Gauge
	RSICWatchdogEscalation func(spaceID string) *Gauge
	RSICWatchdogForce      *Counter

	// RSIC calibration
	RSICCalibrationConfidence func(action string) *Gauge

	// RSIC snapshot
	RSICSnapshotCreated func(action string) *Counter

	// RSIC health sub-scores (published after each assessment)
	RSICHealthOverall    func(spaceID string) *Gauge
	RSICHealthRetrieval  func(spaceID string) *Gauge
	RSICHealthMemory     func(spaceID string) *Gauge
	RSICHealthEdge       func(spaceID string) *Gauge
	RSICHealthTask       func(spaceID string) *Gauge
	RSICHealthGuidance   func(spaceID string) *Gauge
	RSICHealthProtocol   func(spaceID string) *Gauge
	RSICHealthSynergy    func(spaceID string) *Gauge
	RSICHealthConfidence func(spaceID string) *Gauge

	// RSIC synergy metrics (published after each assessment)
	RSICSynergyClaudeLines  func(spaceID string) *Gauge
	RSICSynergyMemoryLines  func(spaceID string) *Gauge
	RSICSynergyOverflowRate func(spaceID string) *Gauge
	RSICSynergyBufferEntries func(spaceID string) *Gauge
}

// NewStandardMetrics creates and registers all standard MDEMG metrics.
func NewStandardMetrics(r *Registry) *StandardMetrics {
	m := &StandardMetrics{registry: r}

	// HTTP metrics - use factory functions for labeled metrics
	httpReqCounters := make(map[string]*Counter)
	m.HTTPRequestsTotal = func(method, path, status string) *Counter {
		labels := map[string]string{"method": method, "path": normalizePath(path), "status": status}
		return r.NewCounter("http_requests_total", "Total HTTP requests", labels)
	}

	httpReqHistograms := make(map[string]*Histogram)
	_ = httpReqHistograms // silence unused warning
	m.HTTPRequestDuration = func(method, path string) *Histogram {
		labels := map[string]string{"method": method, "path": normalizePath(path)}
		return r.NewHistogram("http_request_duration_seconds", "HTTP request latency in seconds", labels)
	}
	_ = httpReqCounters // silence unused warning

	// Retrieval metrics
	m.RetrievalLatency = r.NewHistogram("retrieval_latency_seconds", "Retrieval operation latency", nil)
	m.RetrievalCacheHits = r.NewCounter("retrieval_cache_hits_total", "Retrieval cache hits", nil)
	m.RetrievalCacheMiss = r.NewCounter("retrieval_cache_misses_total", "Retrieval cache misses", nil)

	// Rate limiting
	m.RateLimitRejected = r.NewCounter("rate_limit_rejected_total", "Requests rejected by rate limiting", nil)

	// Circuit breaker
	m.CircuitBreakerState = func(service string) *Gauge {
		labels := map[string]string{"service": service}
		return r.NewGauge("circuit_breaker_state", "Circuit breaker state (0=closed, 1=open, 2=half-open)", labels)
	}

	// Cache hit ratio
	m.CacheHitRatio = func(cache string) *Gauge {
		labels := map[string]string{"cache": cache}
		return r.NewGauge("cache_hit_ratio", "Cache hit ratio (0-1)", labels)
	}

	// Embedding metrics
	m.EmbeddingLatency = r.NewHistogram("embedding_latency_seconds", "Embedding operation latency", nil)
	m.EmbeddingBatches = r.NewCounter("embedding_batches_total", "Total embedding batch operations", nil)

	// Neo4j pool metrics (Phase 48.4.1)
	m.Neo4jPoolActive = r.NewGauge("neo4j_pool_active_connections", "Current active Neo4j connections", nil)
	m.Neo4jPoolIdle = r.NewGauge("neo4j_pool_idle_connections", "Current idle Neo4j connections", nil)
	m.Neo4jPoolWaiting = r.NewGauge("neo4j_pool_waiting_requests", "Requests waiting for Neo4j connection", nil)
	m.Neo4jPoolAcquired = r.NewGauge("neo4j_pool_acquired_total", "Total Neo4j connections acquired", nil)
	m.Neo4jPoolCreated = r.NewGauge("neo4j_pool_created_total", "Total Neo4j connections created", nil)
	m.Neo4jPoolClosed = r.NewGauge("neo4j_pool_closed_total", "Total Neo4j connections closed", nil)
	m.Neo4jPoolFailed = r.NewGauge("neo4j_pool_failed_acquire_total", "Total failed Neo4j connection acquire attempts", nil)

	// Memory pressure metrics (Phase 48.4.4)
	m.MemoryPressureRejected = r.NewGauge("memory_pressure_rejected_total", "Requests rejected due to memory pressure", nil)
	m.MemoryHeapBytes = r.NewGauge("memory_heap_bytes", "Current heap allocation in bytes", nil)

	// CMS observation lifecycle metrics (CMS Hardening)
	m.CMSObserveTotal = func(outcome string) *Counter {
		labels := map[string]string{"outcome": outcome}
		return r.NewCounter("cms_observe_total", "Total CMS observe operations", labels)
	}
	m.CMSEmbeddingFailures = r.NewCounter("cms_embedding_failures_total", "CMS observations created with failed embeddings", nil)
	m.CMSDedupSkips = r.NewCounter("cms_dedup_skips_total", "CMS dedup checks skipped (no embedding)", nil)
	m.CMSDedupMergeFails = r.NewCounter("cms_dedup_merge_failures_total", "CMS dedup merge operation failures", nil)

	// CMS retrieval metrics
	m.CMSRecallTotal = r.NewCounter("cms_recall_total", "Total CMS recall operations", nil)
	m.CMSResumeTotal = r.NewCounter("cms_resume_total", "Total CMS resume operations", nil)

	// CMS error metrics
	m.CMSWriteJSONFails = r.NewCounter("cms_writejson_failures_total", "JSON encoding failures in writeJSON", nil)
	m.CMSLearningEdgeFails = r.NewCounter("cms_learning_edge_failures_total", "Learning edge creation failures", nil)
	m.CMSStabilityUpdateFails = r.NewCounter("cms_stability_update_failures_total", "Stability reinforcement update failures", nil)

	// Neo4j graph per-space metrics (Grafana Neo4j Dashboard)
	m.Neo4jGraphNodes = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_graph_nodes", "Total nodes per space", map[string]string{"space_id": spaceID})
	}
	m.Neo4jGraphEdges = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_graph_edges", "Total edges per space", map[string]string{"space_id": spaceID})
	}
	m.Neo4jGraphObservations = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_graph_observations", "Conversation observations per space", map[string]string{"space_id": spaceID})
	}
	m.Neo4jGraphOrphans = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_graph_orphans", "Zero-edge (orphan) nodes per space", map[string]string{"space_id": spaceID})
	}
	m.Neo4jGraphHealthScore = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_graph_health_score", "Graph health score per space (0-1)", map[string]string{"space_id": spaceID})
	}
	m.Neo4jGraphLearningEdges = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_graph_learning_edges", "Learning (Hebbian) edges per space", map[string]string{"space_id": spaceID})
	}

	// Neo4j graph totals
	m.Neo4jGraphTotalNodes = r.NewGauge("neo4j_graph_total_nodes", "Total nodes across all spaces", nil)
	m.Neo4jGraphTotalEdges = r.NewGauge("neo4j_graph_total_edges", "Total edges across all spaces", nil)
	m.Neo4jGraphTotalSpaces = r.NewGauge("neo4j_graph_total_spaces", "Total number of spaces", nil)

	// Neo4j container resource metrics (via docker stats)
	m.Neo4jContainerCPUPercent = r.NewGauge("neo4j_container_cpu_percent", "Neo4j container CPU usage percent", nil)
	m.Neo4jContainerMemUsed = r.NewGauge("neo4j_container_mem_used_bytes", "Neo4j container memory used in bytes", nil)
	m.Neo4jContainerMemLimit = r.NewGauge("neo4j_container_mem_limit_bytes", "Neo4j container memory limit in bytes", nil)
	m.Neo4jContainerMemPercent = r.NewGauge("neo4j_container_mem_percent", "Neo4j container memory usage percent", nil)

	// RSIC cycle metrics (Phase 91)
	m.RSICCycleTotal = func(tier, source, outcome string) *Counter {
		return r.NewCounter("rsic_cycle_total", "Total RSIC cycles",
			map[string]string{"tier": tier, "source": source, "outcome": outcome})
	}
	m.RSICCycleDuration = func(tier string) *Histogram {
		return r.NewHistogram("rsic_cycle_duration_seconds", "RSIC cycle duration",
			map[string]string{"tier": tier})
	}

	// RSIC trigger orchestration
	m.RSICTriggerRejected = func(source, reason string) *Counter {
		return r.NewCounter("rsic_trigger_rejected_total", "RSIC trigger rejections",
			map[string]string{"source": source, "reason": reason})
	}

	// RSIC action metrics
	m.RSICActionTotal = func(action, status string) *Counter {
		return r.NewCounter("rsic_action_total", "Total RSIC actions",
			map[string]string{"action": action, "status": status})
	}
	m.RSICActionDuration = func(action string) *Histogram {
		return r.NewHistogram("rsic_action_duration_seconds", "RSIC per-action duration",
			map[string]string{"action": action})
	}

	// RSIC safety metrics
	m.RSICSafetyBlocked = func(action, reason string) *Counter {
		return r.NewCounter("rsic_safety_blocked_total", "RSIC safety blocks",
			map[string]string{"action": action, "reason": reason})
	}

	// RSIC watchdog metrics
	m.RSICWatchdogDecay = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_watchdog_decay_score", "RSIC watchdog decay score",
			map[string]string{"space_id": spaceID})
	}
	m.RSICWatchdogEscalation = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_watchdog_escalation_level", "RSIC watchdog escalation level (0=nominal..3=force)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICWatchdogForce = r.NewCounter("rsic_watchdog_force_total", "RSIC watchdog force triggers", nil)

	// RSIC calibration
	m.RSICCalibrationConfidence = func(action string) *Gauge {
		return r.NewGauge("rsic_calibration_confidence", "RSIC per-action confidence (0-1)",
			map[string]string{"action": action})
	}

	// RSIC snapshot
	m.RSICSnapshotCreated = func(action string) *Counter {
		return r.NewCounter("rsic_snapshot_created_total", "RSIC snapshots captured",
			map[string]string{"action": action})
	}

	// RSIC health sub-scores
	m.RSICHealthOverall = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_overall", "Overall cognitive health score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthRetrieval = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_retrieval", "Retrieval quality score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthMemory = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_memory", "Memory health score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthEdge = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_edge", "Edge health score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthTask = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_task", "Task performance score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthGuidance = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_guidance", "Guidance (Jiminy) health score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthProtocol = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_protocol", "Protocol (J17) health score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthSynergy = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_synergy", "Synergy health score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_confidence", "Assessment confidence (0.1-1.0)",
			map[string]string{"space_id": spaceID})
	}

	// RSIC synergy metrics
	m.RSICSynergyClaudeLines = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_synergy_claude_md_lines", "CLAUDE.md line count",
			map[string]string{"space_id": spaceID})
	}
	m.RSICSynergyMemoryLines = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_synergy_memory_md_lines", "MEMORY.md line count",
			map[string]string{"space_id": spaceID})
	}
	m.RSICSynergyOverflowRate = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_synergy_overflow_rate", "CMS overflow events rate",
			map[string]string{"space_id": spaceID})
	}
	m.RSICSynergyBufferEntries = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_synergy_buffer_entries", "Recovery buffer pending entries",
			map[string]string{"space_id": spaceID})
	}

	return m
}

// normalizePath normalizes an HTTP path for metric labels.
// Replaces dynamic path segments (UUIDs, IDs) with placeholders.
func normalizePath(path string) string {
	// Common patterns to normalize
	// /v1/memory/nodes/{uuid} -> /v1/memory/nodes/:id
	// /v1/plugins/{name} -> /v1/plugins/:name

	// Simple normalization: truncate at known dynamic segments
	if len(path) > 50 {
		return path[:50] + "..."
	}
	return path
}

// CollectRateLimitMetrics updates rate limit metrics from the ratelimit package.
func (m *StandardMetrics) CollectRateLimitMetrics() {
	// Get current rejected count from ratelimit package
	m.RateLimitRejected.Add(ratelimit.RejectedTotal())
}

// CollectCircuitBreakerMetrics updates circuit breaker metrics from a registry.
func (m *StandardMetrics) CollectCircuitBreakerMetrics(cbRegistry *circuitbreaker.Registry) {
	if cbRegistry == nil {
		return
	}

	for name, state := range cbRegistry.States() {
		gauge := m.CircuitBreakerState(name)
		switch state {
		case circuitbreaker.StateClosed:
			gauge.Set(0)
		case circuitbreaker.StateOpen:
			gauge.Set(1)
		case circuitbreaker.StateHalfOpen:
			gauge.Set(2)
		}
	}
}

// CollectCacheMetrics updates cache hit ratio metrics from cache stats.
func (m *StandardMetrics) CollectCacheMetrics(cacheStats map[string]map[string]any) {
	for cacheName, stats := range cacheStats {
		if hitRate, ok := stats["hit_rate"].(float64); ok {
			gauge := m.CacheHitRatio(cacheName)
			gauge.Set(hitRate)
		}
	}
}

// CollectNeo4jPoolMetrics updates Neo4j connection pool metrics.
func (m *StandardMetrics) CollectNeo4jPoolMetrics() {
	poolMetrics := db.GetPoolMetrics()

	m.Neo4jPoolActive.Set(float64(poolMetrics.ActiveConnections))
	m.Neo4jPoolIdle.Set(float64(poolMetrics.IdleConnections))
	m.Neo4jPoolWaiting.Set(float64(poolMetrics.WaitingRequests))
	m.Neo4jPoolAcquired.Set(float64(poolMetrics.TotalAcquired))
	m.Neo4jPoolCreated.Set(float64(poolMetrics.TotalCreated))
	m.Neo4jPoolClosed.Set(float64(poolMetrics.TotalClosed))
	m.Neo4jPoolFailed.Set(float64(poolMetrics.TotalFailedAcquire))
}

// SpaceGraphData holds per-space graph stats for Prometheus collection.
type SpaceGraphData struct {
	SpaceID       string
	Nodes         int
	Edges         int
	Observations  int
	Orphans       int
	LearningEdges int
	HealthScore   float64
}

// CollectNeo4jGraphMetrics updates Neo4j graph per-space metrics.
// Purges stale space gauges before setting current values so deleted spaces
// don't persist in Prometheus/Grafana.
func (m *StandardMetrics) CollectNeo4jGraphMetrics(spaces []SpaceGraphData) {
	// Remove all per-space graph gauges — they'll be recreated for current spaces
	if m.registry != nil {
		m.registry.RemoveGaugesByPrefix("neo4j_graph_nodes|")
		m.registry.RemoveGaugesByPrefix("neo4j_graph_edges|")
		m.registry.RemoveGaugesByPrefix("neo4j_graph_observations|")
		m.registry.RemoveGaugesByPrefix("neo4j_graph_orphans|")
		m.registry.RemoveGaugesByPrefix("neo4j_graph_health_score|")
		m.registry.RemoveGaugesByPrefix("neo4j_graph_learning_edges|")
	}

	totalNodes, totalEdges := 0, 0
	for _, s := range spaces {
		m.Neo4jGraphNodes(s.SpaceID).Set(float64(s.Nodes))
		m.Neo4jGraphEdges(s.SpaceID).Set(float64(s.Edges))
		m.Neo4jGraphObservations(s.SpaceID).Set(float64(s.Observations))
		m.Neo4jGraphOrphans(s.SpaceID).Set(float64(s.Orphans))
		m.Neo4jGraphHealthScore(s.SpaceID).Set(s.HealthScore)
		m.Neo4jGraphLearningEdges(s.SpaceID).Set(float64(s.LearningEdges))
		totalNodes += s.Nodes
		totalEdges += s.Edges
	}
	m.Neo4jGraphTotalNodes.Set(float64(totalNodes))
	m.Neo4jGraphTotalEdges.Set(float64(totalEdges))
	m.Neo4jGraphTotalSpaces.Set(float64(len(spaces)))
}

// ContainerStats holds resource metrics from docker stats.
type ContainerStats struct {
	CPUPercent float64
	MemUsed    float64 // bytes
	MemLimit   float64 // bytes
	MemPercent float64
}

// CollectNeo4jContainerMetrics updates Neo4j container resource metrics.
func (m *StandardMetrics) CollectNeo4jContainerMetrics(stats *ContainerStats) {
	if stats == nil {
		return
	}
	m.Neo4jContainerCPUPercent.Set(stats.CPUPercent)
	m.Neo4jContainerMemUsed.Set(stats.MemUsed)
	m.Neo4jContainerMemLimit.Set(stats.MemLimit)
	m.Neo4jContainerMemPercent.Set(stats.MemPercent)
}

// CollectMemoryMetrics updates memory metrics from runtime stats.
func (m *StandardMetrics) CollectMemoryMetrics(heapBytes uint64, rejectedCount int64) {
	m.MemoryHeapBytes.Set(float64(heapBytes))
	m.MemoryPressureRejected.Set(float64(rejectedCount))
}

// global standard metrics instance
var globalMetrics *StandardMetrics

// InitStandardMetrics initializes the global standard metrics.
func InitStandardMetrics() *StandardMetrics {
	globalMetrics = NewStandardMetrics(globalRegistry)
	return globalMetrics
}

// Metrics returns the global standard metrics.
func Metrics() *StandardMetrics {
	if globalMetrics == nil {
		return InitStandardMetrics()
	}
	return globalMetrics
}
