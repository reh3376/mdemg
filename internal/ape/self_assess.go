package ape

import (
	"context"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/config"
	"mdemg/internal/metrics"
	"mdemg/internal/retrieval"
	"mdemg/internal/tsdb"
)

// SynergyFileReader provides synergy file metrics for RSIC assessment.
type SynergyFileReader interface {
	ReadSynergyMetrics() SynergyMetrics
}

// SynergyMetrics holds line counts for Claude Code integration files.
type SynergyMetrics struct {
	ClaudeMDLines   int
	MemoryMDLines   int
	AutoMemoryFiles int
	AutoMemoryLines int
	JiminyHealthy   bool
	OverflowRate    float64
	OverlapScore    float64
}

// Assessor gathers health metrics from subsystems to produce a SelfAssessmentReport.
type Assessor struct {
	cfg              config.Config
	driver           neo4j.DriverWithContext
	learner          LearningStatsProvider
	convSvc          ConversationStatsProvider
	jiminyProvider   JiminyStatsProvider   // J10: guidance stats provider
	protocolProvider ProtocolStatsProvider // J17: protocol metrics provider
	synergyReader     SynergyFileReader     // Synergy: file metrics provider
	freshnessProvider FreshnessProvider     // Phase 47.2: ingest staleness provider
	sidecarChecker    func(context.Context) bool  // Sidecar health checker (nil = not configured)
	reportCallback    func(*SelfAssessmentReport) // TSDB Sprint: called after Assess with the report
	datasetProvider   tsdb.DatasetProvider         // RSIC-DATA: TSDB curated datasets for trend analysis
}

// NewAssessor creates an Assessor wired to the given subsystem providers.
func NewAssessor(cfg config.Config, driver neo4j.DriverWithContext, learner LearningStatsProvider, convSvc ConversationStatsProvider) *Assessor {
	return &Assessor{cfg: cfg, driver: driver, learner: learner, convSvc: convSvc}
}

// SetJiminyProvider attaches a Jiminy stats provider for guidance health assessment (J10).
func (a *Assessor) SetJiminyProvider(p JiminyStatsProvider) {
	a.jiminyProvider = p
}

// SetProtocolProvider attaches a J17 protocol stats provider for protocol health assessment.
func (a *Assessor) SetProtocolProvider(p ProtocolStatsProvider) {
	a.protocolProvider = p
}

// SetSynergyReader attaches a synergy file metrics provider for synergy health assessment.
func (a *Assessor) SetSynergyReader(r SynergyFileReader) {
	a.synergyReader = r
}

// SetFreshnessProvider attaches an ingest freshness provider for staleness detection (Phase 47.2).
func (a *Assessor) SetFreshnessProvider(p FreshnessProvider) {
	a.freshnessProvider = p
}

// SetSidecarChecker attaches a sidecar health checker for assessment.
func (a *Assessor) SetSidecarChecker(fn func(context.Context) bool) {
	a.sidecarChecker = fn
}

// SetReportCallback sets a callback invoked after each Assess() with the report.
// Used by LiveCollectors to cache the latest report for inter-cycle health publishing.
func (a *Assessor) SetReportCallback(cb func(*SelfAssessmentReport)) {
	a.reportCallback = cb
}

// SetDatasetProvider attaches a TSDB curated dataset provider for trend analysis (RSIC-DATA).
func (a *Assessor) SetDatasetProvider(p tsdb.DatasetProvider) {
	a.datasetProvider = p
}

// Assess runs the assessment stage and returns a SelfAssessmentReport.
func (a *Assessor) Assess(ctx context.Context, spaceID string, tier CycleTier) (*SelfAssessmentReport, error) {
	report := &SelfAssessmentReport{
		SpaceID:   spaceID,
		Tier:      tier,
		Timestamp: time.Now(),
	}

	// 1. Distribution / learning phase
	dm := retrieval.GetDistributionMonitor()
	if dm != nil {
		stats := dm.GetStats(spaceID)
		report.LearningPhase = string(stats.Phase)
		report.EdgeCount = stats.EdgeCount
	}

	// 2. Learning edge stats
	if a.learner != nil {
		edgeStats, err := a.learner.GetLearningEdgeStats(ctx, spaceID)
		if err == nil {
			report.AvgEdgeWeight = toFloat64(edgeStats["avg_decayed_weight"])
			report.EdgesBelowThreshold = toInt64(edgeStats["edges_below_threshold"])

			// Edge weight entropy (normalised Shannon entropy over weight buckets)
			report.EdgeWeightEntropy = computeEdgeWeightEntropy(edgeStats)
		}
	}

	// 3. Volatile stats
	if a.convSvc != nil {
		vs, err := a.convSvc.GetVolatileStats(ctx, spaceID)
		if err == nil {
			report.VolatileCount = vs.VolatileCount
			report.PermanentCount = vs.PermanentCount
		}
	}

	// 4. Graph-level metrics via Neo4j
	if err := a.queryGraphMetrics(ctx, spaceID, report); err != nil {
		return report, err // return partial report + error
	}

	// 5. Compute sub-scores
	report.RetrievalQuality = a.scoreRetrieval(report)
	report.MemoryHealth = a.scoreMemory(report)
	report.EdgeHealth = a.scoreEdge(report)
	report.TaskPerformance = a.scoreTask(report)

	// 5b. J10: Compute guidance health if Jiminy stats available
	if a.jiminyProvider != nil {
		js, jErr := a.jiminyProvider.GetGuidanceStats(ctx, spaceID)
		if jErr == nil && js.TotalGuidanceIssued > 0 {
			report.GuidanceHealth = a.scoreGuidance(js)
			a.publishGuidanceMetrics(spaceID, js)
		} else if jErr != nil {
			slog.Warn("rsic: guidance stats unavailable, retaining previous metrics", "error", jErr)
		}
	}

	// 5c. J17: Compute protocol health if protocol stats available
	var protoStats ProtocolStatsResult
	if a.protocolProvider != nil {
		ps, pErr := a.protocolProvider.GetProtocolStats(ctx, spaceID)
		if pErr == nil && ps.TotalEvents > 0 {
			protoStats = ps
			report.ProtocolHealth = a.scoreProtocol(protoStats)
			a.publishProtocolMetrics(spaceID, protoStats)
		} else if pErr != nil {
			slog.Warn("rsic: protocol stats unavailable, retaining previous metrics", "error", pErr)
		}
	}

	// 5d. Synergy: Compute Claude Code ↔ MDEMG synergy health
	if a.synergyReader != nil && a.cfg.SynergyAssessmentEnabled {
		sm := a.synergyReader.ReadSynergyMetrics()
		report.SynergyLinesClaude = sm.ClaudeMDLines
		report.SynergyLinesMemory = sm.MemoryMDLines
		report.SynergyOverflowRate = sm.OverflowRate
		report.SynergyOverlapScore = sm.OverlapScore
		report.JiminyHealthy = sm.JiminyHealthy
		report.SynergyHealth = a.scoreSynergy(report)

		// Recovery buffer: count pending entries (CMS space + local JSONL)
		report.SynergyRecoveryBufferEntries = countBufferSpaceEntries(ctx, a.driver, a.cfg.SynergyRecoveryBufferSpace) +
			countLocalBufferEntries(a.cfg.SynergyRecoveryBufferPath)
	}

	// 5d-sidecar: Check neural sidecar health
	if a.sidecarChecker != nil {
		report.SidecarHealthy = a.sidecarChecker(ctx)
	} else {
		report.SidecarHealthy = true // not configured = not required
	}

	// 5e. Freshness: Count stale spaces for RSIC ingest awareness (Phase 47.2)
	if a.freshnessProvider != nil && a.cfg.APEIngestSyncEnabled {
		staleCount, fErr := a.freshnessProvider.GetStaleSpaceCount(ctx, a.cfg.SyncStaleThresholdHours)
		if fErr != nil {
			slog.Warn("RSIC assess: freshness query failed", "error", fErr)
		} else {
			report.StaleIngestSpaces = staleCount
		}
	}

	// 5f. TSDB datasets (if available) — populate LLM, retrieval, embedding, training readiness
	if a.datasetProvider != nil {
		window := 24 * time.Hour
		if llmPerf, dErr := a.datasetProvider.LLMPerformance(ctx, spaceID, window); dErr == nil {
			report.LLMPerformance = llmPerf
		} else {
			slog.Warn("RSIC assess: LLM performance query failed", "error", dErr)
		}
		if retQual, dErr := a.datasetProvider.RetrievalQuality(ctx, spaceID, window); dErr == nil {
			report.RetrievalDataset = retQual
		} else {
			slog.Warn("RSIC assess: retrieval quality query failed", "error", dErr)
		}
		if embCov, dErr := a.datasetProvider.EmbeddingCoverage(ctx, spaceID, window); dErr == nil {
			report.EmbeddingDataset = embCov
		} else {
			slog.Warn("RSIC assess: embedding coverage query failed", "error", dErr)
		}
		if readiness, dErr := a.datasetProvider.TrainingDataReadiness(ctx); dErr == nil {
			report.TrainingReadiness = readiness
		} else {
			slog.Warn("RSIC assess: training readiness query failed", "error", dErr)
		}
	}

	// 6. Weighted overall (adjusted weights for dimensions present)
	if report.SynergyHealth > 0 && report.ProtocolHealth > 0 && report.GuidanceHealth > 0 {
		// All 7 dimensions
		report.OverallHealth = 0.18*report.RetrievalQuality +
			0.18*report.MemoryHealth +
			0.13*report.EdgeHealth +
			0.13*report.TaskPerformance +
			0.13*report.GuidanceHealth +
			0.13*report.ProtocolHealth +
			0.12*report.SynergyHealth
	} else if report.ProtocolHealth > 0 && report.GuidanceHealth > 0 {
		// All 6 dimensions (no synergy)
		report.OverallHealth = 0.20*report.RetrievalQuality +
			0.20*report.MemoryHealth +
			0.15*report.EdgeHealth +
			0.15*report.TaskPerformance +
			0.15*report.GuidanceHealth +
			0.15*report.ProtocolHealth
	} else if report.GuidanceHealth > 0 {
		report.OverallHealth = 0.25*report.RetrievalQuality +
			0.25*report.MemoryHealth +
			0.20*report.EdgeHealth +
			0.15*report.TaskPerformance +
			0.15*report.GuidanceHealth
	} else {
		report.OverallHealth = 0.30*report.RetrievalQuality +
			0.25*report.MemoryHealth +
			0.25*report.EdgeHealth +
			0.20*report.TaskPerformance
	}

	report.Confidence = a.computeConfidence(report)

	// Publish health sub-scores as Prometheus gauges for dashboard trending
	a.publishHealthMetrics(report)

	// TSDB Sprint: Cache report for LiveCollectors
	if a.reportCallback != nil {
		a.reportCallback(report)
	}

	return report, nil
}

// publishHealthMetrics sets Prometheus gauge values from an assessment report.
func (a *Assessor) publishHealthMetrics(r *SelfAssessmentReport) {
	m := metrics.Metrics()
	sid := r.SpaceID
	m.RSICHealthOverall(sid).Set(r.OverallHealth)
	m.RSICHealthRetrieval(sid).Set(r.RetrievalQuality)
	m.RSICHealthMemory(sid).Set(r.MemoryHealth)
	m.RSICHealthEdge(sid).Set(r.EdgeHealth)
	m.RSICHealthTask(sid).Set(r.TaskPerformance)
	m.RSICHealthGuidance(sid).Set(r.GuidanceHealth)
	m.RSICHealthProtocol(sid).Set(r.ProtocolHealth)
	m.RSICHealthSynergy(sid).Set(r.SynergyHealth)
	m.RSICHealthConfidence(sid).Set(r.Confidence)
	m.RSICSynergyClaudeLines(sid).Set(float64(r.SynergyLinesClaude))
	m.RSICSynergyMemoryLines(sid).Set(float64(r.SynergyLinesMemory))
	m.RSICSynergyOverflowRate(sid).Set(r.SynergyOverflowRate)
	m.RSICSynergyBufferEntries(sid).Set(float64(r.SynergyRecoveryBufferEntries))
}

// queryGraphMetrics runs Neo4j queries for orphan count, correction rate, consolidation freshness.
func (a *Assessor) queryGraphMetrics(ctx context.Context, spaceID string, r *SelfAssessmentReport) error {
	sess := a.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Orphan count + total nodes
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WITH count(n) AS total
			OPTIONAL MATCH (orphan:MemoryNode {space_id: $spaceId})
			WHERE NOT (orphan)--()
			RETURN total, count(orphan) AS orphans
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			r.TotalNodes = toInt64FromRecord(rec, "total")
			r.OrphanCount = toInt64FromRecord(rec, "orphans")
			if r.TotalNodes > 0 {
				r.OrphanRatio = float64(r.OrphanCount) / float64(r.TotalNodes)
			}
		}

		// Correction rate (corrections / total observations in last 24h)
		cypher2 := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND n.created_at > datetime() - duration('PT24H')
			WITH count(n) AS total,
			     count(CASE WHEN n.obs_type = 'correction' THEN 1 END) AS corrections
			RETURN total, corrections
		`
		res2, err := tx.Run(ctx, cypher2, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res2.Next(ctx) {
			rec := res2.Record()
			total := toInt64FromRecord(rec, "total")
			corrections := toInt64FromRecord(rec, "corrections")
			if total > 0 {
				r.CorrectionRate = float64(corrections) / float64(total)
			}
		}

		// Consolidation freshness
		cypher3 := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type IN ['conversation_theme', 'hidden']
			RETURN max(n.created_at) AS lastConsolidation
		`
		res3, err := tx.Run(ctx, cypher3, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res3.Next(ctx) {
			rec := res3.Record()
			if v, ok := rec.Get("lastConsolidation"); ok && v != nil {
				if t, ok := v.(time.Time); ok {
					r.ConsolidationAgeSec = int64(time.Since(t).Seconds())
				}
			}
		}

		return nil, res3.Err()
	})

	return err
}

// ─── Scoring helpers ───

func (a *Assessor) scoreRetrieval(r *SelfAssessmentReport) float64 {
	// Based on learning phase: cold=0.3, learning=0.6, warm=0.9, saturated=0.7
	switch r.LearningPhase {
	case "cold":
		return 0.3
	case "learning":
		return 0.6
	case "warm":
		return 0.9
	case "saturated":
		return 0.7
	default:
		return 0.5
	}
}

func (a *Assessor) scoreMemory(r *SelfAssessmentReport) float64 {
	score := 1.0
	// Penalise high orphan ratio
	if r.OrphanRatio > 0.2 {
		score -= 0.3
	} else if r.OrphanRatio > 0.1 {
		score -= 0.1
	}
	// Penalise high correction rate
	if r.CorrectionRate > 0.15 {
		score -= 0.2
	}
	// Penalise stale consolidation (>24h)
	if r.ConsolidationAgeSec > 86400 {
		score -= 0.2
	}
	if score < 0 {
		score = 0
	}
	return score
}

func (a *Assessor) scoreEdge(r *SelfAssessmentReport) float64 {
	score := 1.0
	if r.EdgeCount > 0 {
		belowRatio := float64(r.EdgesBelowThreshold) / float64(r.EdgeCount)
		if belowRatio > 0.3 {
			score -= 0.3
		}
	}
	if r.EdgeWeightEntropy < 0.5 {
		score -= 0.2
	}
	if score < 0 {
		score = 0
	}
	return score
}

func (a *Assessor) scoreTask(r *SelfAssessmentReport) float64 {
	// Without external task success tracking, use volatile backlog as proxy
	total := r.VolatileCount + r.PermanentCount
	if total == 0 {
		return 0.5
	}
	permanentRatio := float64(r.PermanentCount) / float64(total)
	return clamp(permanentRatio, 0, 1)
}

// scoreGuidance computes guidance health from Jiminy stats (J10).
func (a *Assessor) scoreGuidance(stats JiminyStatsResult) float64 {
	// Weighted combination: follow rate (50%), effectiveness (30%), diversity (20%)
	followScore := clamp(stats.FollowRate, 0, 1)
	effScore := clamp(stats.ConstraintEffRate, 0, 1)
	diversityScore := clamp(stats.SourceDiversity, 0, 1)
	return 0.5*followScore + 0.3*effScore + 0.2*diversityScore
}

// scoreSynergy computes Claude Code ↔ MDEMG synergy health.
// Jiminy must be healthy — without it, synergy pruning is dangerous.
// Returns 0.0 (excluded from OverallHealth formula) when both files are missing.
func (a *Assessor) scoreSynergy(r *SelfAssessmentReport) float64 {
	// G3: Files not found → cannot assess, exclude from formula
	if r.SynergyLinesClaude == 0 && r.SynergyLinesMemory == 0 {
		return 0.0
	}
	if !r.JiminyHealthy {
		return 0.0
	}
	score := 1.0
	// Penalise bloated CLAUDE.md
	if r.SynergyLinesClaude > a.cfg.SynergyTargetClaudeLines+50 {
		score -= 0.3
	} else if r.SynergyLinesClaude > a.cfg.SynergyTargetClaudeLines {
		score -= 0.1
	}
	// Penalise bloated MEMORY.md
	if r.SynergyLinesMemory > a.cfg.SynergyTargetMemoryLines+60 {
		score -= 0.3
	} else if r.SynergyLinesMemory > a.cfg.SynergyTargetMemoryLines {
		score -= 0.1
	}
	// Penalise high overflow rate
	if r.SynergyOverflowRate > 10 {
		score -= 0.3
	} else if r.SynergyOverflowRate > 5 {
		score -= 0.1
	}
	// Penalise high overlap
	if r.SynergyOverlapScore > 0.5 {
		score -= 0.2
	}
	return clamp(score, 0, 1)
}

// scoreProtocol computes protocol health from J17 metrics.
func (a *Assessor) scoreProtocol(stats ProtocolStatsResult) float64 {
	// 35% comprehension (are codes being understood?)
	comprehensionScore := clamp(stats.AvgComprehension, 0, 1)

	// 5% NLI calibration (is the NLI scorer aligned with heuristic?)
	calibrationScore := 1.0
	if stats.NLIBiasAlert {
		calibrationScore = 0.3
	}

	// 25% compression (ratio of 5.0 = perfect, 1.0 = no compression)
	compressionScore := clamp((stats.CompressionRatio-1.0)/4.0, 0, 1)

	// 20% coverage (do all constraints have codes?)
	coverageScore := clamp(stats.CodeCoverage, 0, 1)

	// 15% stability (ticket restores + low replay frequency)
	replayPenalty := clamp(stats.ReplayFrequencyPerHour/10.0, 0, 1)
	restoreScore := clamp(stats.TicketRestoreSuccessRate, 0, 1)
	stabilityScore := 0.5*restoreScore + 0.5*(1.0-replayPenalty)

	return 0.35*comprehensionScore + 0.05*calibrationScore + 0.25*compressionScore +
		0.20*coverageScore + 0.15*stabilityScore
}

func (a *Assessor) computeConfidence(r *SelfAssessmentReport) float64 {
	// Confidence is higher when we have more data points
	dataPoints := 0
	if r.EdgeCount > 0 {
		dataPoints++
	}
	if r.TotalNodes > 0 {
		dataPoints++
	}
	if r.VolatileCount+r.PermanentCount > 0 {
		dataPoints++
	}
	if r.ConsolidationAgeSec > 0 {
		dataPoints++
	}
	confidence := clamp(float64(dataPoints)/4.0, 0.1, 1.0)
	if confidence < 0.3 {
		slog.Warn("rsic: low assessment confidence",
			"space_id", r.SpaceID,
			"confidence", confidence,
			"data_points", dataPoints,
			"edge_count", r.EdgeCount,
			"total_nodes", r.TotalNodes,
			"volatile_count", r.VolatileCount,
			"permanent_count", r.PermanentCount,
			"consolidation_age_sec", r.ConsolidationAgeSec,
		)
	}
	return confidence
}

// ─── Utility ───

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case int:
		return float64(val)
	default:
		return 0
	}
}

func toInt64(v any) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case float64:
		return int64(val)
	case int:
		return int64(val)
	default:
		return 0
	}
}

func toInt64FromRecord(rec *neo4j.Record, key string) int64 {
	if v, ok := rec.Get(key); ok && v != nil {
		return toInt64(v)
	}
	return 0
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// countBufferSpaceEntries queries Neo4j for observations in the recovery buffer space.
func countBufferSpaceEntries(ctx context.Context, driver neo4j.DriverWithContext, bufferSpace string) int {
	if driver == nil || bufferSpace == "" {
		return 0
	}
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.role_type = 'conversation_observation'
			  AND ANY(t IN n.tags WHERE t = 'recovery-buffer')
			RETURN count(n) AS cnt
		`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": bufferSpace})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("cnt"); ok && v != nil {
				if count, ok := v.(int64); ok {
					return int(count), nil
				}
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0
	}
	if count, ok := result.(int); ok {
		return count
	}
	return 0
}

// countLocalBufferEntries counts lines in the local JSONL recovery buffer file.
func countLocalBufferEntries(path string) int {
	if path == "" {
		return 0
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func computeEdgeWeightEntropy(stats map[string]any) float64 {
	// Simple proxy: ratio of strong_edges to total gives a measure
	total := toFloat64(stats["total_edges"])
	strong := toFloat64(stats["strong_edges"])
	if total == 0 {
		return 1.0 // no edges = no issue
	}
	p := strong / total
	if p <= 0 || p >= 1 {
		return 0 // all edges the same weight = low entropy
	}
	// Binary entropy: -p*log2(p) - (1-p)*log2(1-p)
	return -p*math.Log2(p) - (1-p)*math.Log2(1-p)
}

// publishProtocolMetrics publishes J17 protocol telemetry to Prometheus gauges.
func (a *Assessor) publishProtocolMetrics(spaceID string, stats ProtocolStatsResult) {
	m := metrics.Metrics()

	// Tier distribution
	m.J17TierT1Fraction(spaceID).Set(stats.TierDistribution[0])
	m.J17TierT2Fraction(spaceID).Set(stats.TierDistribution[1])
	m.J17TierT3Fraction(spaceID).Set(stats.TierDistribution[2])

	// Core metrics
	m.J17CompressionRatio(spaceID).Set(stats.CompressionRatio)
	m.J17AvgComprehension(spaceID).Set(stats.AvgComprehension)
	m.J17AvgTokensPerGuidance(spaceID).Set(stats.AvgTokensPerGuidance)
	m.J17ReplayFrequency(spaceID).Set(stats.ReplayFrequencyPerHour)
	m.J17TicketRestoreRate(spaceID).Set(stats.TicketRestoreSuccessRate)
	m.J17CodeCoverage(spaceID).Set(stats.CodeCoverage)
	m.J17EventsTotal(spaceID).Set(float64(stats.TotalEvents))

	// Per-tier comprehension
	m.J17TierT1Comprehension(spaceID).Set(stats.TierComprehension[0])
	m.J17TierT2Comprehension(spaceID).Set(stats.TierComprehension[1])
	m.J17TierT3Comprehension(spaceID).Set(stats.TierComprehension[2])

	// Per-tier outcome counts (sample size context)
	m.J17TierT1OutcomeCount(spaceID).Set(float64(stats.TierOutcomeCount[0]))
	m.J17TierT2OutcomeCount(spaceID).Set(float64(stats.TierOutcomeCount[1]))
	m.J17TierT3OutcomeCount(spaceID).Set(float64(stats.TierOutcomeCount[2]))

	// NLI calibration
	m.J17NLIMeanBias(spaceID).Set(stats.NLIMeanBias)
	biasAlertVal := 0.0
	if stats.NLIBiasAlert {
		biasAlertVal = 1.0
	}
	m.J17NLIBiasAlert(spaceID).Set(biasAlertVal)

	// NLI fallback tracking (degraded-state awareness) — always set to avoid stale non-zero values
	m.J17NLIFallbackTotal(spaceID).Set(float64(stats.NLIFallbackCount))

	// Sidecar metrics — always set to clear stale values when sidecar is inactive
	if stats.Sidecar != nil {
		m.J17SidecarRequests(spaceID).Set(float64(stats.Sidecar.Requests))
		m.J17SidecarErrors(spaceID).Set(float64(stats.Sidecar.Errors))
		m.J17SidecarTimeouts(spaceID).Set(float64(stats.Sidecar.Timeouts))
		m.J17SidecarAgreementRate(spaceID).Set(stats.Sidecar.AgreementRate)
		m.J17SidecarOverrideRate(spaceID).Set(stats.Sidecar.OverrideRate)
		m.J17SidecarLatency(spaceID).Set(stats.Sidecar.AvgLatencyMs)
	} else {
		m.J17SidecarRequests(spaceID).Set(0)
		m.J17SidecarErrors(spaceID).Set(0)
		m.J17SidecarTimeouts(spaceID).Set(0)
		m.J17SidecarAgreementRate(spaceID).Set(0)
		m.J17SidecarOverrideRate(spaceID).Set(0)
		m.J17SidecarLatency(spaceID).Set(0)
	}
}

// publishGuidanceMetrics publishes Jiminy guidance telemetry to Prometheus gauges.
func (a *Assessor) publishGuidanceMetrics(spaceID string, stats JiminyStatsResult) {
	m := metrics.Metrics()

	m.JiminyFollowRate(spaceID).Set(stats.FollowRate)
	m.JiminyConstraintEffectiveness(spaceID).Set(stats.ConstraintEffRate)
	m.JiminySourceDiversity(spaceID).Set(stats.SourceDiversity)
	m.JiminyTotalIssued(spaceID).Set(float64(stats.TotalGuidanceIssued))
	m.JiminyTotalFollowed(spaceID).Set(float64(stats.TotalFollowed))
	m.JiminyTotalPartialCompliance(spaceID).Set(float64(stats.TotalPartialCompliance))
	m.JiminyTotalIgnored(spaceID).Set(float64(stats.TotalIgnored))
	m.JiminyTotalContradicted(spaceID).Set(float64(stats.TotalContradicted))
}
