package ape

import (
	"context"
	"math"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/config"
	"mdemg/internal/retrieval"
)

// Assessor gathers health metrics from subsystems to produce a SelfAssessmentReport.
type Assessor struct {
	cfg             config.Config
	driver          neo4j.DriverWithContext
	learner         LearningStatsProvider
	convSvc         ConversationStatsProvider
	jiminyProvider  JiminyStatsProvider // J10: guidance stats provider
}

// NewAssessor creates an Assessor wired to the given subsystem providers.
func NewAssessor(cfg config.Config, driver neo4j.DriverWithContext, learner LearningStatsProvider, convSvc ConversationStatsProvider) *Assessor {
	return &Assessor{cfg: cfg, driver: driver, learner: learner, convSvc: convSvc}
}

// SetJiminyProvider attaches a Jiminy stats provider for guidance health assessment (J10).
func (a *Assessor) SetJiminyProvider(p JiminyStatsProvider) {
	a.jiminyProvider = p
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
		jiminyStats, jErr := a.jiminyProvider.GetGuidanceStats(ctx, spaceID)
		if jErr == nil && jiminyStats.TotalGuidanceIssued > 0 {
			report.GuidanceHealth = a.scoreGuidance(jiminyStats)
		}
	}

	// 6. Weighted overall (adjusted weights for 5 dimensions)
	if report.GuidanceHealth > 0 {
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

	return report, nil
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
	return clamp(float64(dataPoints)/4.0, 0.1, 1.0)
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
