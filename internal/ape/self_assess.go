package ape

import (
	"bufio"
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
	cfg               config.Config
	driver            neo4j.DriverWithContext
	learner           LearningStatsProvider
	convSvc           ConversationStatsProvider
	jiminyProvider    JiminyStatsProvider         // J10: guidance stats provider
	protocolProvider  ProtocolStatsProvider       // J17: protocol metrics provider
	synergyReader     SynergyFileReader           // Synergy: file metrics provider
	freshnessProvider FreshnessProvider           // Phase 47.2: ingest staleness provider
	sidecarChecker    func(context.Context) bool  // Sidecar health checker (nil = not configured)
	reportCallback    func(*SelfAssessmentReport) // TSDB Sprint: called after Assess with the report
	datasetProvider   tsdb.DatasetProvider        // RSIC-DATA: TSDB curated datasets for trend analysis
}

// NewAssessor creates an Assessor wired to the given subsystem providers.
func NewAssessor(cfg config.Config, driver neo4j.DriverWithContext, learner LearningStatsProvider, convSvc ConversationStatsProvider) *Assessor {
	return &Assessor{cfg: cfg, driver: driver, learner: learner, convSvc: convSvc}
}

// healthWeights builds a HealthWeights struct from the Assessor's config.
// When all weights are zero (operator error or unset in a test Config),
// DefaultHealthWeights is returned so overall_health stays meaningful.
// DH-005.
func (a *Assessor) healthWeights() HealthWeights {
	w := HealthWeights{
		Retrieval: a.cfg.RSICHealthWeightRetrieval,
		Memory:    a.cfg.RSICHealthWeightMemory,
		Edge:      a.cfg.RSICHealthWeightEdge,
		Task:      a.cfg.RSICHealthWeightTask,
		Guidance:  a.cfg.RSICHealthWeightGuidance,
		Protocol:  a.cfg.RSICHealthWeightProtocol,
		Synergy:   a.cfg.RSICHealthWeightSynergy,
	}
	sum := w.Retrieval + w.Memory + w.Edge + w.Task + w.Guidance + w.Protocol + w.Synergy
	if sum <= 0 {
		return DefaultHealthWeights()
	}
	return w
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

	// 5. Compute sub-scores + confidences (DH-005)
	report.RetrievalQuality, report.RetrievalConfidence = a.scoreRetrieval(report)
	report.MemoryHealth, report.MemoryConfidence = a.scoreMemory(report)
	report.EdgeHealth, report.EdgeConfidence = a.scoreEdge(report)
	report.TaskPerformance, report.TaskConfidence = a.scoreTask(report)

	// 5b. J10: Compute guidance health if Jiminy stats available
	if a.jiminyProvider != nil {
		js, jErr := a.jiminyProvider.GetGuidanceStats(ctx, spaceID)
		if jErr == nil && js.TotalGuidanceIssued > 0 {
			a.applyHonestFollowRate(ctx, spaceID, &js)
			report.GuidanceHealth, report.GuidanceConfidence = a.scoreGuidance(js)
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
			report.ProtocolHealth, report.ProtocolConfidence = a.scoreProtocol(protoStats)
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
		report.SynergyAssessed = true // JIMINY-SIGNAL-001: JiminyHealthy is now real, not the zero-value default
		report.SynergyHealth, report.SynergyConfidence = a.scoreSynergy(report)

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
			// SCORE-RETRIEVAL-REAL-SIGNALS-001: section 5 scored the dimension
			// BEFORE this fetch (live-smoke-caught ordering bug) — recompute now
			// that the real signal is present so the primary path actually fires.
			report.RetrievalQuality, report.RetrievalConfidence = a.scoreRetrieval(report)
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
			// SF-1 heartbeat: only on SUCCESS. A silent query failure stops
			// this signal and the training_readiness_stale rule fires.
			if m := metrics.Metrics(); m != nil {
				m.RSICReadinessAssessed(spaceID).Set(1)
			}
		} else {
			slog.Warn("RSIC assess: training readiness query failed", "error", dErr)
		}
		// DRIFT-TRIGGER-001: production drift signal for reflect pattern 31.
		if drift, dErr := a.datasetProvider.ProductionDrift(ctx); dErr == nil {
			report.ProductionDrift = drift
		} else {
			slog.Warn("RSIC assess: production drift query failed", "error", dErr)
		}
	}

	// 6. Weighted overall (single source: ComputeOverallHealthWith + cfg weights)
	report.OverallHealth = ComputeOverallHealthWith(report, a.healthWeights())

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
	// DH-005: per-dimension data-sufficiency confidence gauges
	m.RSICHealthRetrievalConfidence(sid).Set(r.RetrievalConfidence)
	m.RSICHealthMemoryConfidence(sid).Set(r.MemoryConfidence)
	m.RSICHealthEdgeConfidence(sid).Set(r.EdgeConfidence)
	m.RSICHealthTaskConfidence(sid).Set(r.TaskConfidence)
	m.RSICHealthGuidanceConfidence(sid).Set(r.GuidanceConfidence)
	m.RSICHealthProtocolConfidence(sid).Set(r.ProtocolConfidence)
	m.RSICHealthSynergyConfidence(sid).Set(r.SynergyConfidence)
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
		// Orphan count + total nodes.
		// ORPHAN-ALERT-001: EXCLUDE archived (tombstoned) nodes from BOTH the
		// total and the orphan count. Tombstones have their edges removed so
		// they are always zero-degree; counting them (4,457 in mdemg-dev)
		// inflated OrphanRatio to 6.2% vs the true live 1.0% and polluted the
		// RSIC health computation. The mdemg_neo4j_graph_orphans gauge already
		// excludes archived — this aligns RSIC with it. (HIDDEN-CHURN-001 class.)
		cypher := `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE NOT coalesce(n.is_archived, false)
			WITH count(n) AS total
			OPTIONAL MATCH (orphan:MemoryNode {space_id: $spaceId})
			WHERE NOT (orphan)--() AND NOT coalesce(orphan.is_archived, false)
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

// Data-sufficiency thresholds for per-dimension confidence (DH-005).
// At or above these counts, the dimension is treated as fully confident (1.0).
// Below, confidence scales linearly. Rationale in sprint DH-005 plan.
const (
	confidenceThresholdMemoryNodes    = 100 // TotalNodes
	confidenceThresholdEdges          = 50  // EdgeCount
	confidenceThresholdTaskObs        = 50  // VolatileCount+PermanentCount
	confidenceThresholdGuidanceEvents = 30  // TotalGuidanceIssued
	confidenceThresholdProtocolEvents = 30  // TotalEvents
	// SCORE-RETRIEVAL-REAL-SIGNALS-001: retrieval_events rows in the window
	// for full-confidence RetrievalQuality.
	confidenceThresholdRetrievalEvents = 50
)

// defaultCompressionTargetRatio mirrors the J17_COMPRESSION_TARGET_RATIO
// config default (DASHBOARD-TRUTH-001 Epic 3, recalibrated 3.0→2.0 by
// DASHBOARD-TRUTH-002 E3) — used as a defensive fallback when the Assessor
// is built with a Config literal (target unset / <= 1.0). Keep in sync
// with FromEnv's default in internal/config/config.go.
const defaultCompressionTargetRatio = 2.0

// scoreRetrieval returns (score, confidence). Primary path: mean of the
// per-stage fill rates from retrieval_events (real quality signal;
// SCORE-RETRIEVAL-REAL-SIGNALS-001). Fallback: the LearningPhase maturity
// enum (DASHBOARD-TRUTH-002 E1 fixed its saturated=0.7 wrong-anchor to 0.9)
// for spaces with zero retrieval traffic in the assessment window.
func (a *Assessor) scoreRetrieval(r *SelfAssessmentReport) (float64, float64) {
	// SCORE-RETRIEVAL-REAL-SIGNALS-001 (DASHBOARD-TRUTH-002 E1 full fix):
	// score from the REAL pipeline signal the assessment already collects —
	// per-stage fill rates over the window from retrieval_events
	// (report.RetrievalDataset, populated before scoring). Confidence is
	// proportional to sample size (DH-005: low-N windows are naturally
	// down-weighted; zero-N excluded), so no extra config knob is needed.
	// The enum below remains the FALLBACK for spaces with no retrieval
	// traffic in the window (maturity prior, weak confidence).
	if d := r.RetrievalDataset; d != nil && d.TotalQueries > 0 {
		score := (d.RecallRate + d.BM25Rate + d.RerankRate) / 3.0
		conf := math.Min(1.0, float64(d.TotalQueries)/float64(confidenceThresholdRetrievalEvents))
		return clamp(score, 0, 1), conf
	}
	switch r.LearningPhase {
	case "cold":
		return 0.3, 0.4
	case "learning":
		return 0.6, 0.7
	case "warm":
		return 0.9, 1.0
	case "saturated":
		return 0.9, 1.0
	default:
		return 0.5, 0.1
	}
}

func (a *Assessor) scoreMemory(r *SelfAssessmentReport) (float64, float64) {
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
	conf := math.Min(1.0, float64(r.TotalNodes)/float64(confidenceThresholdMemoryNodes))
	return score, conf
}

func (a *Assessor) scoreEdge(r *SelfAssessmentReport) (float64, float64) {
	score := 1.0
	if r.EdgeCount > 0 {
		belowRatio := float64(r.EdgesBelowThreshold) / float64(r.EdgeCount)
		if belowRatio > 0.3 {
			score -= 0.3
		}
	}
	// DASHBOARD-TRUTH-002 E2 (2026-07-20): the old hardcoded 0.5 floor
	// permanently fired on any mature Hebbian graph. computeEdgeWeightEntropy
	// is binary entropy over `strong_edges / total` where strong = evidence_count>=5.
	// A mature substrate accumulates many single-touch co-activations that
	// never re-trigger (long-tail); live mdemg-dev shows p≈0.047 → entropy≈0.27,
	// well below the old 0.5 floor. Config-tunable now (RSIC_EDGE_ENTROPY_FLOOR
	// default 0.2, calibrated below the observed healthy value). Set to 0 to
	// disable the penalty entirely.
	floor := a.cfg.RSICEdgeEntropyFloor
	if floor > 0 && r.EdgeWeightEntropy < floor {
		score -= 0.2
	}
	if score < 0 {
		score = 0
	}
	// No edges → no signal; exclude from formula via conf=0.
	if r.EdgeCount == 0 {
		return score, 0
	}
	conf := math.Min(1.0, float64(r.EdgeCount)/float64(confidenceThresholdEdges))
	return score, conf
}

func (a *Assessor) scoreTask(r *SelfAssessmentReport) (float64, float64) {
	// Without external task success tracking, use volatile backlog as proxy
	total := r.VolatileCount + r.PermanentCount
	if total == 0 {
		return 0.5, 0
	}
	permanentRatio := float64(r.PermanentCount) / float64(total)
	conf := math.Min(1.0, float64(total)/float64(confidenceThresholdTaskObs))
	return clamp(permanentRatio, 0, 1), conf
}

// scoreGuidance computes guidance health from Jiminy stats (J10).
func (a *Assessor) scoreGuidance(stats JiminyStatsResult) (float64, float64) {
	// Weighted combination: follow rate (50%), effectiveness (30%), diversity (20%)
	followScore := clamp(stats.FollowRate, 0, 1)
	effScore := clamp(stats.ConstraintEffRate, 0, 1)
	diversityScore := clamp(stats.SourceDiversity, 0, 1)
	score := 0.5*followScore + 0.3*effScore + 0.2*diversityScore
	if stats.TotalGuidanceIssued <= 0 {
		return score, 0
	}
	conf := math.Min(1.0, float64(stats.TotalGuidanceIssued)/float64(confidenceThresholdGuidanceEvents))
	return score, conf
}

// scoreSynergy computes Claude Code ↔ MDEMG synergy health.
// Jiminy must be healthy — without it, synergy pruning is dangerous.
// Returns confidence 0 (excluded from OverallHealth formula) when both
// files are missing or Jiminy is unhealthy.
func (a *Assessor) scoreSynergy(r *SelfAssessmentReport) (float64, float64) {
	// G3: Files not found → cannot assess, exclude from formula
	if r.SynergyLinesClaude == 0 && r.SynergyLinesMemory == 0 {
		return 0.0, 0
	}
	if !r.JiminyHealthy {
		return 0.0, 0
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
	return clamp(score, 0, 1), 1.0
}

// scoreProtocol computes protocol health from J17 metrics.
func (a *Assessor) scoreProtocol(stats ProtocolStatsResult) (float64, float64) {
	// 35% comprehension (are codes being understood?)
	comprehensionScore := clamp(stats.AvgComprehension, 0, 1)

	// 5% NLI calibration (is the NLI scorer aligned with heuristic?)
	calibrationScore := 1.0
	if stats.NLIBiasAlert {
		calibrationScore = 0.3
	}

	// 25% compression (ratio of target = excellent, 1.0 = no compression).
	// Calibration anchor (DH-005 class): J17_COMPRESSION_TARGET_RATIO sets
	// the ratio scored as 1.0 — raising the target LOWERS the Protocol score,
	// lowering it RAISES it. The old hardcoded 5.0 anchor was unreachable
	// (real J17 compression: ~1.8 typical, ~3 achievable; 30d live p50 1.56,
	// p95 2.0) and permanently dragged the dimension by ~0.20
	// (DASHBOARD-TRUTH-001 Epic 3). Default 3.0 = defensibly-excellent.
	target := a.cfg.J17CompressionTargetRatio
	if target <= 1.0 {
		// Defensive fallback for Config literals that never went through
		// FromEnv (which warns + falls back on misconfiguration).
		target = defaultCompressionTargetRatio
	}
	compressionScore := clamp((stats.CompressionRatio-1.0)/(target-1.0), 0, 1)

	// 20% coverage (do all constraints have codes?)
	coverageScore := clamp(stats.CodeCoverage, 0, 1)

	// 15% stability (ticket restores + low replay frequency).
	// DASHBOARD-TRUTH-002 E3 (2026-07-20): apply the DH-004 "no data =
	// neutral" gate to restoreScore. When TicketRestoreTotal==0 (no
	// restore attempts happened in the window), the rate field is 0.0 by
	// default — but 0.0 means "no data", NOT "0% success rate". Same
	// pattern DH-004 shipped for J17 Protocol Health's own view of this
	// field. Without the gate, an idle-ish substrate had its Protocol
	// dimension permanently dragged by 0.075 (0.15 stability × 0.5
	// restoreScore weight × 1.0 mis-scored delta).
	replayPenalty := clamp(stats.ReplayFrequencyPerHour/10.0, 0, 1)
	restoreScore := 1.0
	if stats.TicketRestoreTotal > 0 {
		restoreScore = clamp(stats.TicketRestoreSuccessRate, 0, 1)
	}
	stabilityScore := 0.5*restoreScore + 0.5*(1.0-replayPenalty)

	score := 0.35*comprehensionScore + 0.05*calibrationScore + 0.25*compressionScore +
		0.20*coverageScore + 0.15*stabilityScore
	if stats.TotalEvents <= 0 {
		return score, 0
	}
	conf := math.Min(1.0, float64(stats.TotalEvents)/float64(confidenceThresholdProtocolEvents))
	return score, conf
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
// Uses streaming scanner to avoid loading the entire file into memory.
func countLocalBufferEntries(path string) int {
	if path == "" {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
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

	// Event-derived gauges: only meaningful when events exist this process
	// (shared gate with live_collectors — see j17EventGaugesMeaningful).
	if j17EventGaugesMeaningful(stats) {
		// Tier distribution
		m.J17TierT1Fraction(spaceID).Set(stats.TierDistribution[0])
		m.J17TierT2Fraction(spaceID).Set(stats.TierDistribution[1])
		m.J17TierT3Fraction(spaceID).Set(stats.TierDistribution[2])

		// Core event-derived metrics
		m.J17CompressionRatio(spaceID).Set(stats.CompressionRatio)
		m.J17AvgComprehension(spaceID).Set(stats.AvgComprehension)
		m.J17AvgTokensPerGuidance(spaceID).Set(stats.AvgTokensPerGuidance)

		// Per-tier comprehension
		m.J17TierT1Comprehension(spaceID).Set(stats.TierComprehension[0])
		m.J17TierT2Comprehension(spaceID).Set(stats.TierComprehension[1])
		m.J17TierT3Comprehension(spaceID).Set(stats.TierComprehension[2])
	}

	// Honest-at-zero / own-no-data-semantics gauges: always publish.
	m.J17ReplayFrequency(spaceID).Set(stats.ReplayFrequencyPerHour)
	m.J17TicketRestoreRate(spaceID).Set(stats.TicketRestoreSuccessRate)
	m.J17CodeCoverage(spaceID).Set(stats.CodeCoverage)
	m.J17EventsTotal(spaceID).Set(float64(stats.TotalEvents))

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

// applyHonestFollowRate overrides stats.FollowRate with the windowed
// constraint_outcomes TSDB rate (the dashboard panels' source + math) when
// available. JIMINY-SIGNAL-001: the Neo4j dedup-by-guidance_id rate is inflated
// ~4× (double-credits multi-outcome guidance_ids: 0.73 vs the panels' ~0.27);
// this makes the gauge, RSIC GuidanceHealth, and the panels agree. No-op when
// the dataset provider is unavailable or the window has no data (Neo4j fallback).
// Called by BOTH the assessment path AND the live Prometheus collector
// (live_collectors.go) — the gauge has two publishers, so both must honest-ize.
func (a *Assessor) applyHonestFollowRate(ctx context.Context, spaceID string, stats *JiminyStatsResult) {
	if a == nil || a.datasetProvider == nil || stats == nil {
		return
	}
	window := time.Duration(a.cfg.RSICGuidanceEffectivenessWindowHours) * time.Hour
	if rate, n, err := a.datasetProvider.GuidanceEffectiveness(ctx, spaceID, window); err == nil && n > 0 {
		stats.FollowRate = rate
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
