package metrics

import (
	"log/slog"
	"mdemg/internal/sanitize"
	"sync/atomic"
	"time"

	"mdemg/internal/circuitbreaker"
	"mdemg/internal/ratelimit"
	"mdemg/internal/tsdb"
)

// RecordConsolidationPhase emits the per-phase duration gauge + a structured log
// (CONSOLIDATE-PERF-001 Sprint A, Epic 1). Shared by both consolidation paths —
// the service-level RunConsolidation (the RSIC-watchdog driver) and the
// handleConsolidate HTTP handler (manual triggers) — so the per-phase breakdown
// that targets the Sprint-B optimization is captured regardless of trigger.
func RecordConsolidationPhase(spaceID, phase string, start time.Time) {
	dur := time.Since(start)
	Metrics().ConsolidationPhaseDuration(spaceID, phase).Set(dur.Seconds())
	slog.Info("consolidation phase complete", "space_id", spaceID, "phase", phase, "dur_ms", dur.Milliseconds())
}

// StandardMetrics holds pre-registered standard metrics.
type StandardMetrics struct {
	registry *Registry

	// HTTP metrics
	HTTPRequestsTotal   func(method, path, status string) *Counter
	HTTPRequestDuration func(method, path string) *Histogram

	// Retrieval metrics
	RetrievalLatency *Histogram
	// DORMANT-METRICS-CLEANUP-001: retrieval_cache_hits_total and
	// retrieval_cache_misses_total had zero writer sites + zero samples/7d.
	// Retrieval cache accounting is done via a separate path
	// (retrieval column cache in service.go); these fields were dead.

	// Rate limiting metrics
	RateLimitRejected *Counter
	// lastRateLimitRejected tracks the last-collected cumulative rejection
	// total so CollectRateLimitMetrics adds deltas, not running totals.
	// Atomic: collection can run from the flush loop and request handlers.
	lastRateLimitRejected atomic.Int64

	// Circuit breaker metrics (by service)
	CircuitBreakerState func(service string) *Gauge

	// Cache metrics
	CacheHitRatio func(cache string) *Gauge

	// Embedding metrics
	EmbeddingLatency *Histogram
	EmbeddingBatches *Counter

	// TSDB (pgx) connection pool — the only pool with a real stats API
	// (TSDB-CONSUME-001). The former mdemg_neo4j_pool_* gauges were fake:
	// the neo4j Go driver exposes no pool stats, so a probe loop counted
	// VerifyConnectivity successes as "acquisitions" and Active/Idle/Waiting
	// were perpetual zeros (which made the neo4j_pool_exhausted rule
	// unfireable). Deleted rather than kept lying.
	TSDBPoolTotal        *Gauge // Total connections in the pgx pool
	TSDBPoolIdle         *Gauge // Idle connections
	TSDBPoolAcquired     *Gauge // Connections currently checked out
	TSDBPoolMax          *Gauge // Pool capacity (MaxConns)
	TSDBPoolEmptyAcquire *Gauge // Cumulative acquires that waited on an empty pool

	// Buffered TSDB writer flush stats, labeled by the hypertable the writer
	// feeds (TSDB-CONSUME-001). Cumulative process-lifetime values exposed as
	// gauges; the tsdb_writer_flush_failures rule alerts on in-window growth.
	TSDBWriterFlushSuccess  func(writer string) *Gauge
	TSDBWriterFlushFailures func(writer string) *Gauge
	TSDBWriterRowsFlushed   func(writer string) *Gauge
	TSDBWriterRowsDropped   func(writer string) *Gauge

	// Guidance conflict counter (TSDB-CONSUME-001) — makes idea 09's
	// go/no-go criterion measurable from metric_samples.
	GuidanceConflicts func(spaceID string) *Counter

	// Surface-vs-outcome split (JIMINY-BUDGET-001): surfaced is the honest
	// denominator; dropped feedback makes the silent gap observable.
	JiminyGuidanceSurfaced func(spaceID string) *Counter
	JiminyFeedbackDropped  func(spaceID string) *Counter

	// Emergence/consolidation cycle wall time (TSDB-CONSUME-001) — the
	// observable the DBSCAN O(n²) deferral is conditioned on (>60s revisit
	// threshold). Gauge of the LAST completed cycle, not a histogram: the
	// registry's fixed ≤10s latency buckets would clamp multi-minute cycles
	// exactly like the HTTP percentiles this sprint un-broke, and at ~1-6
	// cycles per flush window a percentile is noise anyway.
	EmergenceCycleDuration func(spaceID, cycle string) *Gauge

	// ConsolidationPhaseDuration is the wall time of the last run of each
	// consolidation phase (CONSOLIDATE-PERF-001 Sprint A) — the per-phase
	// breakdown that targets the Sprint-B algorithmic optimization.
	ConsolidationPhaseDuration func(spaceID, phase string) *Gauge

	// Memory pressure metrics (Phase 48.4.4)
	MemoryPressureRejected *Gauge // Requests rejected due to memory pressure (cumulative)
	MemoryHeapBytes        *Gauge // Current heap allocation in bytes

	// CMS observation lifecycle metrics (CMS Hardening)
	CMSObserveTotal      func(outcome string) *Counter // "success", "degraded", "deduplicated"
	CMSEmbeddingFailures *Counter
	CMSDedupMergeFails   *Counter
	// DORMANT-METRICS-CLEANUP-001: CMSDedupSkips, CMSRecallTotal, CMSResumeTotal,
	// CMSLearningEdgeFails removed — zero writer sites + zero samples/7d.
	// CMS observability is currently covered by hook-observation flow (which
	// writes directly to Neo4j observations), not through these counters.

	// CMS error metrics
	CMSWriteJSONFails       *Counter
	CMSStabilityUpdateFails *Counter

	// Neo4j graph per-space metrics (Grafana Neo4j Dashboard)
	Neo4jGraphNodes           func(spaceID string) *Gauge
	Neo4jGraphEdges           func(spaceID string) *Gauge
	Neo4jGraphObservations    func(spaceID string) *Gauge
	Neo4jGraphOrphans         func(spaceID string) *Gauge
	Neo4jGraphNullWeightEdges func(spaceID string) *Gauge
	Neo4jConversationCoverage func(spaceID string) *Gauge
	Neo4jGraphHealthScore     func(spaceID string) *Gauge
	Neo4jGraphLearningEdges   func(spaceID string) *Gauge

	// Neo4j graph totals
	Neo4jGraphTotalNodes  *Gauge
	Neo4jGraphTotalEdges  *Gauge
	Neo4jGraphTotalSpaces *Gauge

	// Neo4j container resource metrics (via docker stats)
	Neo4jContainerCPUPercent *Gauge
	Neo4jContainerMemUsed    *Gauge
	Neo4jContainerMemLimit   *Gauge
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

	// Phase 11.6.x: RSIC LLM-stage concurrency-limit instrumentation. Increments
	// each time CycleOrchestrator must wait for an in-flight slot to free up
	// before reflector.Reflect can run. Sustained non-zero values indicate the
	// configured RSIC_LLM_CONCURRENCY_LIMIT is the rate-determining factor.
	RSICLLMSemaphoreBlocked *Counter

	// RSIC watchdog metrics
	RSICWatchdogDecay      func(spaceID string) *Gauge
	RSICWatchdogEscalation func(spaceID string) *Gauge
	RSICWatchdogForce      *Counter

	// RSIC calibration
	RSICCalibrationConfidence func(action string) *Gauge

	// RSIC snapshot
	RSICSnapshotCreated func(action string) *Counter

	// RSIC health sub-scores (published after each assessment)
	RSICHealthOverall func(spaceID string) *Gauge
	// RSICReadinessAssessed is a heartbeat set to 1 each time the RSIC
	// training-readiness assessment query SUCCEEDS (SF-1, FT-RECURSIVE-001).
	// Its sample freshness backs the training_readiness_stale alert rule: a
	// silent readiness-query failure stops the heartbeat and the loop goes
	// dormant — the rule catches that absence.
	RSICReadinessAssessed func(spaceID string) *Gauge
	RSICHealthRetrieval   func(spaceID string) *Gauge
	RSICHealthMemory      func(spaceID string) *Gauge
	RSICHealthEdge        func(spaceID string) *Gauge
	RSICHealthTask        func(spaceID string) *Gauge
	RSICHealthGuidance    func(spaceID string) *Gauge
	RSICHealthProtocol    func(spaceID string) *Gauge
	RSICHealthSynergy     func(spaceID string) *Gauge
	RSICHealthConfidence  func(spaceID string) *Gauge

	// RSIC per-dimension data-sufficiency confidence (DH-005). Published
	// alongside the score gauges above so operators can distinguish "this
	// dimension scored 0.0 because the system is broken" from "this dimension
	// scored 0.0 because we have no data." Feeds the dimension-confidence row
	// on the mdemg-rsic Grafana dashboard.
	RSICHealthRetrievalConfidence func(spaceID string) *Gauge
	RSICHealthMemoryConfidence    func(spaceID string) *Gauge
	RSICHealthEdgeConfidence      func(spaceID string) *Gauge
	RSICHealthTaskConfidence      func(spaceID string) *Gauge
	RSICHealthGuidanceConfidence  func(spaceID string) *Gauge
	RSICHealthProtocolConfidence  func(spaceID string) *Gauge
	RSICHealthSynergyConfidence   func(spaceID string) *Gauge

	// RSIC synergy metrics (published after each assessment)
	RSICSynergyClaudeLines   func(spaceID string) *Gauge
	RSICSynergyMemoryLines   func(spaceID string) *Gauge
	RSICSynergyOverflowRate  func(spaceID string) *Gauge
	RSICSynergyBufferEntries func(spaceID string) *Gauge

	// J17 Protocol metrics (published after each assessment)
	J17TierT1Fraction       func(spaceID string) *Gauge
	J17TierT2Fraction       func(spaceID string) *Gauge
	J17TierT3Fraction       func(spaceID string) *Gauge
	J17CompressionRatio     func(spaceID string) *Gauge
	J17AvgComprehension     func(spaceID string) *Gauge
	J17AvgTokensPerGuidance func(spaceID string) *Gauge
	J17ReplayFrequency      func(spaceID string) *Gauge
	J17TicketRestoreRate    func(spaceID string) *Gauge
	J17CodeCoverage         func(spaceID string) *Gauge
	J17EventsTotal          func(spaceID string) *Gauge
	J17TierT1Comprehension  func(spaceID string) *Gauge
	J17TierT2Comprehension  func(spaceID string) *Gauge
	J17TierT3Comprehension  func(spaceID string) *Gauge

	// J17 NLI calibration metrics
	J17NLIMeanBias      func(spaceID string) *Gauge
	J17NLIBiasAlert     func(spaceID string) *Gauge
	J17NLIFallbackTotal func(spaceID string) *Gauge

	// J17 tier outcome counts (sample size context)
	J17TierT1OutcomeCount func(spaceID string) *Gauge
	J17TierT2OutcomeCount func(spaceID string) *Gauge
	J17TierT3OutcomeCount func(spaceID string) *Gauge

	// J17 trust score aggregates
	J17AvgTrustScore     func(spaceID string) *Gauge
	J17MinTrustScore     func(spaceID string) *Gauge
	J17MaxTrustScore     func(spaceID string) *Gauge
	J17TrustSessionCount func(spaceID string) *Gauge

	// J17 sidecar metrics — these count ONLY the tier-prediction shadow client
	// (RecordSidecarCall), which is gated off by default. They do NOT count NLI
	// comprehension calls.
	J17SidecarRequests      func(spaceID string) *Gauge
	J17SidecarErrors        func(spaceID string) *Gauge
	J17SidecarTimeouts      func(spaceID string) *Gauge
	J17SidecarAgreementRate func(spaceID string) *Gauge
	J17SidecarOverrideRate  func(spaceID string) *Gauge
	J17SidecarLatency       func(spaceID string) *Gauge

	// DASHBOARD-TRUTH-001 — real NLI comprehension call observability. Before
	// this, actual NLI sidecar calls (the nli_comprehension scoring path in
	// jiminy feedback) were counted nowhere.
	J17NLIRequests  func(spaceID, result string) *Counter // mdemg_j17_nli_requests_total{space_id,result} — result: ok|fallback
	J17NLILatencyMs func(spaceID string) *Histogram       // mdemg_j17_nli_latency_ms{space_id} — wall time of each NLI comprehension call attempt

	// Jiminy guidance metrics (published after each assessment)
	JiminyFollowRate              func(spaceID string) *Gauge
	JiminyConstraintEffectiveness func(spaceID string) *Gauge
	// JIMINY-ACTIONABILITY-001 — surfaced-composition observability (the fraction
	// of the surfaced guidance set that is the actionable vs abstraction class —
	// what Lever A directly moves).
	JiminySurfacedActionableFraction  func(spaceID string) *Gauge
	JiminySurfacedAbstractionFraction func(spaceID string) *Gauge
	JiminySourceDiversity             func(spaceID string) *Gauge
	JiminyTotalIssued                 func(spaceID string) *Gauge
	JiminyTotalFollowed               func(spaceID string) *Gauge
	JiminyTotalPartialCompliance      func(spaceID string) *Gauge
	JiminyTotalIgnored                func(spaceID string) *Gauge
	JiminyTotalContradicted           func(spaceID string) *Gauge
	CompactEventTimestamp             func(spaceID string) *Gauge

	// Jiminy Guide + Warm metrics (event-driven pre-computation)
	// DORMANT-METRICS-CLEANUP-001: JiminyGuideTimeout removed — zero writer
	// sites + zero samples/7d. Timeouts on Guide() are covered by the
	// jiminy_warm_errors_total path (which IS wired).
	JiminyGuideCalls    func(spaceID string) *Counter
	JiminyGuideEmpty    func(spaceID string) *Counter
	JiminyWarmCompleted func(spaceID string) *Counter
	JiminyWarmErrors    func(spaceID string) *Counter
	JiminyWarmDebounced func(spaceID string) *Counter
	// GUARDRAIL-PRODUCER-001: async producer outcomes (queued/dropped/disabled).
	GuardrailProducer func(status string) *Counter
	// FT-RECURSIVE-003 E1: lease-held gauge (republished each controller poll).
	FtLoopLeaseHeld    func() *Gauge
	JiminyLatestAge    func(spaceID string) *Gauge
	JiminyLatestServed func(spaceID string) *Counter

	// Phase 11.6.3 — MLX Watchdog. State + fast-fail + transition metrics for
	// the goroutine in internal/mlxprobe and the gate in internal/llmclient.
	MLXHealthState      func(endpoint string) *Gauge     // 0=up, 1=degraded, 2=down
	MLXFastFailTotal    func(callerTask string) *Counter // increment when llmclient short-circuits a call
	MLXStateTransitions func(from, to string) *Counter   // increment on each up/degraded/down transition

	// Phase 13 — Note 04 Column-Voting Retrieval. consensus_strength
	// distribution + per-column wall-clock + per-column failure counter.
	// Populated only when cfg.RetrievalColumnVotingEnabled is true.
	RetrievalConsensusStrength *Histogram                           // mdemg_retrieval_consensus_strength — aggregate consensus per retrieve call
	RetrievalColumnLatency     func(column string) *Histogram       // mdemg_retrieval_column_latency_seconds{column}
	RetrievalColumnFailedTotal func(column, reason string) *Counter // mdemg_retrieval_column_failed_total{column,reason}

	// Phase 14 Epic 1 — Note 06 sparse activation gate. Populated only when
	// cfg.SparseRetrievalEnabled is true (or per-request override sets it).
	// Histograms over per-call gate behavior — V0020 sparse_gate_metrics
	// captures the same data with longer retention than Prometheus reset
	// cycles allow.
	SparseGateActiveCount     *Histogram // mdemg_sparse_gate_active_count — active set size after clamps
	SparseGateDroppedFraction *Histogram // mdemg_sparse_gate_dropped_fraction — fraction dropped (0..1)
	SparseGateThreshold       *Histogram // mdemg_sparse_gate_threshold — score-value at percentile cutoff

	// EVENTGRAPH-001 — reinforcement_events writer counters. Surfaced from
	// the writer's Stats() into Prometheus by the api server's metrics
	// snapshot path. Counters monotonically increase from process start.
	EventgraphRowsEnqueued *Counter // mdemg_eventgraph_writer_rows_enqueued_total — successful CopyFrom rows
	EventgraphRowsDropped  *Counter // mdemg_eventgraph_writer_rows_dropped_total — FIFO-evicted rows (buffer-full)
	EventgraphFlushFailure *Counter // mdemg_eventgraph_writer_flush_failure_total — CopyFrom errors

	// JIMINY-RELEVANCE-001 — guidance_training_rows writer counters.
	GuidanceCorpusRowsEnqueued *Counter // mdemg_guidance_corpus_rows_enqueued_total — training-evidence rows written to TSDB
	GuidanceCorpusRowsDropped  *Counter // mdemg_guidance_corpus_rows_dropped_total — rows FIFO-evicted (buffer-full)
	GuidanceCorpusFlushFailure *Counter // mdemg_guidance_corpus_flush_failure_total — writer CopyFrom failures
	// JIMINY-RELEVANCE-001 Epic 2 — label-quality observability.
	GuidanceCorpusHeuristicFraction *Gauge // mdemg_guidance_corpus_heuristic_label_fraction — share of recent rows still on a heuristic/blank label (target: low; the auto-relabel job drives this down)
}

// Registry returns the underlying metric registry.
func (m *StandardMetrics) Registry() *Registry {
	return m.registry
}

// nliLatencyMsBuckets are millisecond-scale buckets for mdemg_j17_nli_latency_ms.
// The shared registry buckets are seconds-scale (5ms…120s) — reused for a
// millisecond-valued metric they would top out at 120ms and clamp every
// percentile (the ALERT-TRUTH-001 bucket-coverage class). This set covers the
// J17_SIDECAR_TIMEOUT_MS default (1000ms, 100ms floor) with headroom for
// operator overrides.
var nliLatencyMsBuckets = []float64{1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

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
	m.TSDBPoolTotal = r.NewGauge("tsdb_pool_total_conns", "Total connections in the TSDB pgx pool", nil)
	m.TSDBPoolIdle = r.NewGauge("tsdb_pool_idle_conns", "Idle connections in the TSDB pgx pool", nil)
	m.TSDBPoolAcquired = r.NewGauge("tsdb_pool_acquired_conns", "TSDB pgx pool connections currently checked out", nil)
	m.TSDBPoolMax = r.NewGauge("tsdb_pool_max_conns", "TSDB pgx pool capacity (MaxConns)", nil)
	m.TSDBPoolEmptyAcquire = r.NewGauge("tsdb_pool_empty_acquire_total", "Cumulative TSDB pool acquires that waited on an empty pool", nil)

	m.TSDBWriterFlushSuccess = func(writer string) *Gauge {
		return r.NewGauge("tsdb_writer_flush_success_total", "Cumulative successful flushes per buffered TSDB writer", map[string]string{"writer": writer})
	}
	m.TSDBWriterFlushFailures = func(writer string) *Gauge {
		return r.NewGauge("tsdb_writer_flush_failures_total", "Cumulative failed flushes per buffered TSDB writer", map[string]string{"writer": writer})
	}
	m.TSDBWriterRowsFlushed = func(writer string) *Gauge {
		return r.NewGauge("tsdb_writer_rows_flushed_total", "Cumulative rows flushed per buffered TSDB writer", map[string]string{"writer": writer})
	}
	m.TSDBWriterRowsDropped = func(writer string) *Gauge {
		return r.NewGauge("tsdb_writer_rows_dropped_total", "Cumulative rows dropped (buffer overflow) per buffered TSDB writer", map[string]string{"writer": writer})
	}

	m.JiminyGuidanceSurfaced = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_guidance_surfaced_total", "Guidance items returned by Guide() — the denominator the outcome counts were missing (JIMINY-BUDGET-001)", map[string]string{"space_id": spaceID})
	}
	m.JiminyFeedbackDropped = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_feedback_dropped_total", "Feedback arriving after the guidance_id expired from the tracker", map[string]string{"space_id": spaceID})
	}
	m.GuidanceConflicts = func(spaceID string) *Counter {
		return r.NewCounter("guidance_conflicts_total", "Conflicts detected by consulting.Suggest (idea 09 go/no-go signal)", map[string]string{"space_id": spaceID})
	}
	m.EmergenceCycleDuration = func(spaceID, cycle string) *Gauge {
		return r.NewGauge("emergence_cycle_duration_seconds", "Wall time of the last completed emergence/consolidation cycle (DBSCAN O(n²) deferral tripwire)", map[string]string{"space_id": spaceID, "cycle": cycle})
	}
	m.ConsolidationPhaseDuration = func(spaceID, phase string) *Gauge {
		return r.NewGauge("consolidation_phase_duration_seconds", "Wall time of the last run of each consolidation phase (CONSOLIDATE-PERF-001 per-phase breakdown)", map[string]string{"space_id": spaceID, "phase": phase})
	}

	// Memory pressure metrics (Phase 48.4.4)
	m.MemoryPressureRejected = r.NewGauge("memory_pressure_rejected_total", "Requests rejected due to memory pressure", nil)
	m.MemoryHeapBytes = r.NewGauge("memory_heap_bytes", "Current heap allocation in bytes", nil)

	// CMS observation lifecycle metrics (CMS Hardening)
	m.CMSObserveTotal = func(outcome string) *Counter {
		labels := map[string]string{"outcome": outcome}
		return r.NewCounter("cms_observe_total", "Total CMS observe operations", labels)
	}
	m.CMSEmbeddingFailures = r.NewCounter("cms_embedding_failures_total", "CMS observations created with failed embeddings", nil)
	m.CMSDedupMergeFails = r.NewCounter("cms_dedup_merge_failures_total", "CMS dedup merge operation failures", nil)

	// CMS error metrics
	m.CMSWriteJSONFails = r.NewCounter("cms_writejson_failures_total", "JSON encoding failures in writeJSON", nil)
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
	m.Neo4jGraphNullWeightEdges = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_graph_null_weight_edges", "NULL-weight abstraction edges per space (HIDDEN-WEIGHT-001; steady state 0)", map[string]string{"space_id": spaceID})
	}
	m.Neo4jConversationCoverage = func(spaceID string) *Gauge {
		return r.NewGauge("neo4j_conversation_coverage_ratio", "Themed/total conversation observations per space (HIDDEN-CHURN-001)", map[string]string{"space_id": spaceID})
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

	// Phase 11.6.x — RSIC LLM-stage concurrency throttle counter
	// Hotfix 11.6.3.1 followups — name registered without `mdemg_` prefix
	// (registry adds it automatically; previous registration produced
	// double-prefixed `mdemg_mdemg_rsic_llm_semaphore_blocked_total`).
	m.RSICLLMSemaphoreBlocked = r.NewCounter("rsic_llm_semaphore_blocked_total",
		"RSIC cycles that waited for an in-flight LLM-stage slot before reflector.Reflect", nil)

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
	m.RSICReadinessAssessed = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_readiness_assessed",
			"Heartbeat=1 on each successful RSIC training-readiness assessment (SF-1)",
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

	// RSIC per-dimension data-sufficiency confidence (DH-005)
	m.RSICHealthRetrievalConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_retrieval_confidence", "Retrieval score data-sufficiency confidence (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthMemoryConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_memory_confidence", "Memory score data-sufficiency confidence (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthEdgeConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_edge_confidence", "Edge score data-sufficiency confidence (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthTaskConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_task_confidence", "Task score data-sufficiency confidence (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthGuidanceConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_guidance_confidence", "Guidance score data-sufficiency confidence (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthProtocolConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_protocol_confidence", "Protocol score data-sufficiency confidence (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.RSICHealthSynergyConfidence = func(spaceID string) *Gauge {
		return r.NewGauge("rsic_health_synergy_confidence", "Synergy score data-sufficiency confidence (0-1)",
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

	// J17 Protocol metrics
	m.J17TierT1Fraction = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t1_fraction", "Fraction of guidance at T1 (coded)",
			map[string]string{"space_id": spaceID})
	}
	m.J17TierT2Fraction = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t2_fraction", "Fraction of guidance at T2 (telegraphic)",
			map[string]string{"space_id": spaceID})
	}
	m.J17TierT3Fraction = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t3_fraction", "Fraction of guidance at T3 (full NL)",
			map[string]string{"space_id": spaceID})
	}
	m.J17CompressionRatio = func(spaceID string) *Gauge {
		return r.NewGauge("j17_compression_ratio", "Token compression ratio",
			map[string]string{"space_id": spaceID})
	}
	m.J17AvgComprehension = func(spaceID string) *Gauge {
		return r.NewGauge("j17_avg_comprehension", "Average comprehension score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.J17AvgTokensPerGuidance = func(spaceID string) *Gauge {
		return r.NewGauge("j17_avg_tokens_per_guidance", "Average tokens per guidance item",
			map[string]string{"space_id": spaceID})
	}
	m.J17ReplayFrequency = func(spaceID string) *Gauge {
		return r.NewGauge("j17_replay_frequency_per_hour", "Replay events per hour",
			map[string]string{"space_id": spaceID})
	}
	m.J17TicketRestoreRate = func(spaceID string) *Gauge {
		return r.NewGauge("j17_ticket_restore_success_rate", "Ticket restore success rate (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.J17CodeCoverage = func(spaceID string) *Gauge {
		return r.NewGauge("j17_code_coverage", "Fraction of constraints with T1 codes (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.J17EventsTotal = func(spaceID string) *Gauge {
		return r.NewGauge("j17_events_total", "Total protocol events in window",
			map[string]string{"space_id": spaceID})
	}
	m.J17TierT1Comprehension = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t1_comprehension", "T1 tier comprehension score",
			map[string]string{"space_id": spaceID})
	}
	m.J17TierT2Comprehension = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t2_comprehension", "T2 tier comprehension score",
			map[string]string{"space_id": spaceID})
	}
	m.J17TierT3Comprehension = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t3_comprehension", "T3 tier comprehension score",
			map[string]string{"space_id": spaceID})
	}

	// J17 NLI calibration
	m.J17NLIMeanBias = func(spaceID string) *Gauge {
		return r.NewGauge("j17_nli_mean_bias", "NLI calibration mean bias",
			map[string]string{"space_id": spaceID})
	}
	m.J17NLIBiasAlert = func(spaceID string) *Gauge {
		return r.NewGauge("j17_nli_bias_alert", "NLI bias alert state (0=normal, 1=alert)",
			map[string]string{"space_id": spaceID})
	}
	m.J17NLIFallbackTotal = func(spaceID string) *Gauge {
		return r.NewGauge("j17_nli_fallback_total", "NLI sidecar fallback events (degraded state)",
			map[string]string{"space_id": spaceID})
	}

	// J17 tier outcome counts
	m.J17TierT1OutcomeCount = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t1_outcome_count", "T1 comprehension event count",
			map[string]string{"space_id": spaceID})
	}
	m.J17TierT2OutcomeCount = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t2_outcome_count", "T2 comprehension event count",
			map[string]string{"space_id": spaceID})
	}
	m.J17TierT3OutcomeCount = func(spaceID string) *Gauge {
		return r.NewGauge("j17_tier_t3_outcome_count", "T3 comprehension event count",
			map[string]string{"space_id": spaceID})
	}

	// J17 trust score aggregates
	m.J17AvgTrustScore = func(spaceID string) *Gauge {
		return r.NewGauge("j17_avg_trust_score", "Average trust score across active sessions (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.J17MinTrustScore = func(spaceID string) *Gauge {
		return r.NewGauge("j17_min_trust_score", "Minimum trust score across active sessions (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.J17MaxTrustScore = func(spaceID string) *Gauge {
		return r.NewGauge("j17_max_trust_score", "Maximum trust score across active sessions (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.J17TrustSessionCount = func(spaceID string) *Gauge {
		return r.NewGauge("j17_trust_session_count", "Number of active trust-scored sessions",
			map[string]string{"space_id": spaceID})
	}

	// J17 sidecar metrics
	m.J17SidecarRequests = func(spaceID string) *Gauge {
		return r.NewGauge("j17_sidecar_requests", "Sidecar request count",
			map[string]string{"space_id": spaceID})
	}
	m.J17SidecarErrors = func(spaceID string) *Gauge {
		return r.NewGauge("j17_sidecar_errors", "Sidecar error count",
			map[string]string{"space_id": spaceID})
	}
	m.J17SidecarTimeouts = func(spaceID string) *Gauge {
		return r.NewGauge("j17_sidecar_timeouts", "Sidecar timeout count",
			map[string]string{"space_id": spaceID})
	}
	m.J17SidecarAgreementRate = func(spaceID string) *Gauge {
		return r.NewGauge("j17_sidecar_agreement_rate", "Sidecar agreement rate",
			map[string]string{"space_id": spaceID})
	}
	m.J17SidecarOverrideRate = func(spaceID string) *Gauge {
		return r.NewGauge("j17_sidecar_override_rate", "Sidecar override rate",
			map[string]string{"space_id": spaceID})
	}
	m.J17SidecarLatency = func(spaceID string) *Gauge {
		return r.NewGauge("j17_sidecar_avg_latency_ms", "Sidecar average latency (ms)",
			map[string]string{"space_id": spaceID})
	}

	// DASHBOARD-TRUTH-001 — NLI comprehension call observability
	m.J17NLIRequests = func(spaceID, result string) *Counter {
		return r.NewCounter("j17_nli_requests_total",
			"NLI comprehension call attempts against the J17 sidecar (result: ok|fallback). Distinct from j17_sidecar_requests, which counts only the tier-prediction shadow client.",
			map[string]string{"space_id": spaceID, "result": result})
	}
	m.J17NLILatencyMs = func(spaceID string) *Histogram {
		return r.NewHistogramWithBuckets("j17_nli_latency_ms",
			"Wall time (ms) of each NLI comprehension call attempt",
			map[string]string{"space_id": spaceID}, nliLatencyMsBuckets)
	}

	// Jiminy guidance metrics
	m.JiminyFollowRate = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_follow_rate", "Guidance follow rate (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.JiminySurfacedActionableFraction = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_surfaced_actionable_fraction", "Fraction of the surfaced guidance set that is the actionable class (constraint/correction) — JIMINY-ACTIONABILITY-001",
			map[string]string{"space_id": spaceID})
	}
	m.JiminySurfacedAbstractionFraction = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_surfaced_abstraction_fraction", "Fraction of the surfaced guidance set that is the abstraction class (pattern/learning/concept/…) — JIMINY-ACTIONABILITY-001",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyConstraintEffectiveness = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_constraint_effectiveness", "Constraint effectiveness rate (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.JiminySourceDiversity = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_source_diversity", "Source diversity score (0-1)",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyTotalIssued = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_guidance_total", "Total guidance items issued",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyTotalFollowed = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_followed_total", "Guidance items followed",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyTotalPartialCompliance = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_partial_compliance_total", "Guidance items partially complied with",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyTotalIgnored = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_ignored_total", "Guidance items ignored",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyTotalContradicted = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_contradicted_total", "Guidance items contradicted",
			map[string]string{"space_id": spaceID})
	}
	m.CompactEventTimestamp = func(spaceID string) *Gauge {
		return r.NewGauge("compact_event_ts", "Timestamp of last compact event (unix seconds)",
			map[string]string{"space_id": spaceID})
	}

	// ─── Jiminy Guide + Warm metrics ───
	m.JiminyGuideCalls = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_guide_calls_total", "Total Guide() invocations",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyGuideEmpty = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_guide_empty_total", "Guide() calls returning zero items",
			map[string]string{"space_id": spaceID})
	}
	// DORMANT-METRICS-CLEANUP-001: JiminyGuideTimeout factory removed.
	m.JiminyWarmCompleted = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_warm_completed_total", "Warm pre-computations completed",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyWarmErrors = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_warm_errors_total", "Warm pre-computations that errored",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyWarmDebounced = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_warm_debounced_total", "Warm requests skipped by debounce",
			map[string]string{"space_id": spaceID})
	}
	m.GuardrailProducer = func(status string) *Counter {
		return r.NewCounter("guardrail_producer_total", "Guardrail async producer requests by outcome",
			map[string]string{"status": status})
	}
	m.FtLoopLeaseHeld = func() *Gauge {
		return r.NewGauge("ftloop_lease_held", "1 while the recursive-retrain compute lease is held (suppresses training_readiness_stale)", nil)
	}
	m.JiminyLatestAge = func(spaceID string) *Gauge {
		return r.NewGauge("jiminy_latest_age_ms", "Age of latest pre-computed guidance in ms",
			map[string]string{"space_id": spaceID})
	}
	m.JiminyLatestServed = func(spaceID string) *Counter {
		return r.NewCounter("jiminy_latest_served_total", "GET /latest requests served",
			map[string]string{"space_id": spaceID})
	}

	// Phase 11.6.3 — MLX Watchdog metrics. Note: registry prepends `mdemg_`
	// to all names automatically, so we register the unqualified suffix and
	// the exposed metric name is `mdemg_mlx_health_state` etc. (Hotfix
	// 11.6.3.1 fixed a double-prefix bug — names were registered as
	// `mdemg_mlx_*` and exposed as `mdemg_mdemg_mlx_*`.)
	m.MLXHealthState = func(endpoint string) *Gauge {
		return r.NewGauge("mlx_health_state",
			"mlx_lm.server health state per endpoint (0=up, 1=degraded, 2=down)",
			map[string]string{"endpoint": endpoint})
	}
	m.MLXFastFailTotal = func(callerTask string) *Counter {
		return r.NewCounter("mlx_fast_fail_total",
			"Total LLM calls short-circuited by the watchdog fast-fail gate",
			map[string]string{"caller_task": callerTask})
	}
	m.MLXStateTransitions = func(from, to string) *Counter {
		return r.NewCounter("mlx_state_transitions_total",
			"Total mlx watchdog state transitions",
			map[string]string{"from": from, "to": to})
	}

	// Phase 13 — Column-Voting Retrieval metrics (unqualified names; registry
	// adds the `mdemg_` prefix automatically).
	m.RetrievalConsensusStrength = r.NewHistogram(
		"retrieval_consensus_strength",
		"Aggregate consensus_strength per retrieve call (0.0-1.0; higher = more column agreement)",
		nil)
	m.RetrievalColumnLatency = func(column string) *Histogram {
		return r.NewHistogram("retrieval_column_latency_seconds",
			"Per-column retrieval wall-clock in seconds",
			map[string]string{"column": column})
	}
	m.RetrievalColumnFailedTotal = func(column, reason string) *Counter {
		return r.NewCounter("retrieval_column_failed_total",
			"Total per-column retrieval failures (column timed out, errored, or returned empty)",
			map[string]string{"column": column, "reason": reason})
	}

	// Phase 14 Epic 1 — Note 06 sparse activation gate metrics.
	m.SparseGateActiveCount = r.NewHistogram(
		"sparse_gate_active_count",
		"Active set size after Note 06 percentile gate + clamps",
		nil)
	m.SparseGateDroppedFraction = r.NewHistogram(
		"sparse_gate_dropped_fraction",
		"Fraction of input candidates dropped by Note 06 gate (0..1)",
		nil)
	m.SparseGateThreshold = r.NewHistogram(
		"sparse_gate_threshold",
		"Score value at the per-call activation percentile cutoff",
		nil)

	// EVENTGRAPH-001 — reinforcement_events writer counters.
	m.EventgraphRowsEnqueued = r.NewCounter(
		"eventgraph_writer_rows_enqueued_total",
		"Reinforcement-event rows successfully written to TSDB", nil)
	m.EventgraphRowsDropped = r.NewCounter(
		"eventgraph_writer_rows_dropped_total",
		"Reinforcement-event rows FIFO-evicted (buffer-full)", nil)
	m.EventgraphFlushFailure = r.NewCounter(
		"eventgraph_writer_flush_failure_total",
		"Reinforcement-event writer CopyFrom failures", nil)

	// JIMINY-RELEVANCE-001 — guidance_training_rows writer counters.
	m.GuidanceCorpusRowsEnqueued = r.NewCounter(
		"guidance_corpus_rows_enqueued_total",
		"Guidance training-evidence rows successfully written to TSDB", nil)
	m.GuidanceCorpusRowsDropped = r.NewCounter(
		"guidance_corpus_rows_dropped_total",
		"Guidance training-evidence rows FIFO-evicted (buffer-full)", nil)
	m.GuidanceCorpusFlushFailure = r.NewCounter(
		"guidance_corpus_flush_failure_total",
		"Guidance training-evidence writer CopyFrom failures", nil)
	m.GuidanceCorpusHeuristicFraction = r.NewGauge(
		"guidance_corpus_heuristic_label_fraction",
		"Share of recent guidance_training_rows still on a heuristic/blank label (auto-relabel job drives this down)", nil)

	return m
}

// normalizePath normalizes an HTTP path for metric labels.
// Replaces dynamic path segments (UUIDs, IDs) with placeholders.
func normalizePath(path string) string {
	// Common patterns to normalize
	// /v1/memory/nodes/{uuid} -> /v1/memory/nodes/:id
	// /v1/plugins/{name} -> /v1/plugins/:name

	// Simple normalization: truncate at known dynamic segments
	return sanitize.CutRuneSafeSuffix(path, 50, "...")
}

// CollectRateLimitMetrics updates rate limit metrics from the ratelimit package.
// RejectedTotal is cumulative, so only the delta since the last collection is
// added — the previous implementation added the running total every flush
// cycle, inflating the counter quadratically (TSDB-CONSUME-001).
func (m *StandardMetrics) CollectRateLimitMetrics() {
	total := ratelimit.RejectedTotal()
	last := m.lastRateLimitRejected.Swap(total)
	if delta := total - last; delta > 0 {
		m.RateLimitRejected.Add(delta)
	}
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

// CollectTSDBPoolMetrics updates TSDB pgx connection pool gauges from a
// pgxpool.Stat() snapshot (passed as plain ints to keep this package free of
// a pgx dependency). Replaces the fake Neo4j pool collector (TSDB-CONSUME-001).
func (m *StandardMetrics) CollectTSDBPoolMetrics(total, idle, acquired, maxConns, emptyAcquire int64) {
	m.TSDBPoolTotal.Set(float64(total))
	m.TSDBPoolIdle.Set(float64(idle))
	m.TSDBPoolAcquired.Set(float64(acquired))
	m.TSDBPoolMax.Set(float64(maxConns))
	m.TSDBPoolEmptyAcquire.Set(float64(emptyAcquire))
}

// CollectTSDBWriterStats updates the per-writer flush gauges from
// tsdb.AllWriterStats() (TSDB-CONSUME-001). Stats are cumulative
// process-lifetime values; the tsdb_writer_flush_failures evaluator rule
// alerts on in-window growth. (metrics→tsdb is the established import
// direction — recorder.go already depends on tsdb; tsdb never imports
// metrics, by design — see reinforcement_writer.go.)
func (m *StandardMetrics) CollectTSDBWriterStats(stats map[string]tsdb.FlushStats) {
	for writer, st := range stats {
		m.TSDBWriterFlushSuccess(writer).Set(float64(st.SuccessCount))
		m.TSDBWriterFlushFailures(writer).Set(float64(st.FailureCount))
		m.TSDBWriterRowsFlushed(writer).Set(float64(st.TotalRows))
		m.TSDBWriterRowsDropped(writer).Set(float64(st.OverflowCount))
	}
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
	// NullWeightEdges counts GENERALIZES/ABSTRACTS_TO edges with NULL weight
	// (HIDDEN-WEIGHT-001) — steady state 0; >0 = the bug class regressed.
	NullWeightEdges int
	// ConversationCoverage = themed/total conversation observations
	// (HIDDEN-CHURN-001 PR-B — the 94%-uncovered gap's gauge).
	ConversationCoverage float64
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
		m.registry.RemoveGaugesByPrefix("neo4j_graph_null_weight_edges|")
		m.registry.RemoveGaugesByPrefix("neo4j_conversation_coverage_ratio|")
		m.registry.RemoveGaugesByPrefix("neo4j_graph_health_score|")
		m.registry.RemoveGaugesByPrefix("neo4j_graph_learning_edges|")
	}

	totalNodes, totalEdges := 0, 0
	for _, s := range spaces {
		m.Neo4jGraphNodes(s.SpaceID).Set(float64(s.Nodes))
		m.Neo4jGraphEdges(s.SpaceID).Set(float64(s.Edges))
		m.Neo4jGraphObservations(s.SpaceID).Set(float64(s.Observations))
		m.Neo4jGraphOrphans(s.SpaceID).Set(float64(s.Orphans))
		m.Neo4jGraphNullWeightEdges(s.SpaceID).Set(float64(s.NullWeightEdges))
		if s.ConversationCoverage >= 0 { // -1 sentinel: below CONVERSATION_COVERAGE_MIN_OBS, no gauge
			m.Neo4jConversationCoverage(s.SpaceID).Set(s.ConversationCoverage)
		}
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
