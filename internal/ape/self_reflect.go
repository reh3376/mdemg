package ape

import (
	"context"
	"log"
	"sort"

	"mdemg/internal/config"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Reflector analyses a SelfAssessmentReport and produces actionable insights.
type Reflector struct {
	cfg          config.Config
	driver       neo4j.DriverWithContext
	llmReflector *LLMReflector
}

// NewReflector creates a Reflector.
func NewReflector(cfg config.Config, driver neo4j.DriverWithContext) *Reflector {
	return &Reflector{cfg: cfg, driver: driver}
}

// SetLLMReflector attaches an optional LLM-powered reflector for deeper analysis.
func (r *Reflector) SetLLMReflector(lr *LLMReflector) {
	r.llmReflector = lr
}

// Reflect examines the assessment report and returns ordered insights.
func (r *Reflector) Reflect(ctx context.Context, report *SelfAssessmentReport) ([]ReflectionInsight, error) {
	var insights []ReflectionInsight

	// 1. Saturation check
	if report.LearningPhase == "saturated" {
		insights = append(insights, ReflectionInsight{
			PatternID:         "saturated_edges",
			Severity:          SeverityHigh,
			Description:       "Learning edge count has reached saturation — edge weights may lose discriminative power",
			RecommendedAction: "prune_decayed_edges",
			Metric:            "learning_phase",
			Value:             1, // indicator
			Threshold:         0, // any saturation triggers
		})
	}

	// 2. Orphan ratio
	if report.OrphanRatio > 0.2 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "high_orphan_ratio",
			Severity:          SeverityMedium,
			Description:       "More than 20% of nodes are orphaned (no edges) — indicates poor connectivity",
			RecommendedAction: "trigger_consolidation",
			Metric:            "orphan_ratio",
			Value:             report.OrphanRatio,
			Threshold:         0.2,
		})
	}

	// 3. Correction rate
	if report.CorrectionRate > 0.15 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "high_correction_rate",
			Severity:          SeverityMedium,
			Description:       "More than 15% of recent observations are corrections — knowledge quality issue",
			RecommendedAction: "tombstone_stale",
			Metric:            "correction_rate",
			Value:             report.CorrectionRate,
			Threshold:         0.15,
		})
	}

	// 4. Consolidation freshness
	if report.ConsolidationAgeSec > 86400 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "stale_consolidation",
			Severity:          SeverityLow,
			Description:       "Last consolidation was more than 24 hours ago",
			RecommendedAction: "trigger_consolidation",
			Metric:            "consolidation_age_sec",
			Value:             float64(report.ConsolidationAgeSec),
			Threshold:         86400,
		})
	}

	// 5. Edge weight entropy
	if report.EdgeWeightEntropy < 0.5 && report.EdgeCount > 0 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "low_edge_entropy",
			Severity:          SeverityMedium,
			Description:       "Edge weight entropy is low — weights are clustered at extremes, losing discriminative value",
			RecommendedAction: "prune_excess_edges",
			Metric:            "edge_weight_entropy",
			Value:             report.EdgeWeightEntropy,
			Threshold:         0.5,
		})
	}

	// 6. Volatile backlog
	if report.VolatileCount > 100 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "volatile_backlog",
			Severity:          SeverityMedium,
			Description:       "More than 100 volatile observations pending graduation",
			RecommendedAction: "graduate_volatile",
			Metric:            "volatile_count",
			Value:             float64(report.VolatileCount),
			Threshold:         100,
		})
	}

	// 7. Excess edges below threshold
	if report.EdgeCount > 0 {
		belowRatio := float64(report.EdgesBelowThreshold) / float64(report.EdgeCount)
		if belowRatio > 0.3 {
			insights = append(insights, ReflectionInsight{
				PatternID:         "many_weak_edges",
				Severity:          SeverityMedium,
				Description:       "More than 30% of edges are below the decay threshold — dead weight",
				RecommendedAction: "prune_decayed_edges",
				Metric:            "edges_below_ratio",
				Value:             belowRatio,
				Threshold:         0.3,
			})
		}
	}

	// 8. Edge staleness — check via refresh
	if report.EdgeCount > 100 && report.AvgEdgeWeight < 0.2 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "stale_edges",
			Severity:          SeverityLow,
			Description:       "Average edge weight is very low — edges may need refreshing",
			RecommendedAction: "refresh_stale_edges",
			Metric:            "avg_edge_weight",
			Value:             report.AvgEdgeWeight,
			Threshold:         0.2,
		})
	}

	// 9. J10: Low guidance follow rate
	if report.GuidanceHealth > 0 && report.GuidanceHealth < 0.5 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "low_guidance_follow_rate",
			Severity:          SeverityMedium,
			Description:       "Less than 50% of high-priority guidance is being followed — agent may be ignoring constraints",
			RecommendedAction: "review_guidance_effectiveness",
			Metric:            "guidance_health",
			Value:             report.GuidanceHealth,
			Threshold:         0.5,
		})
	}

	// Phase AR-3: Merge LLM reflector insights (fail-open — rule-based results used alone on error)
	if r.llmReflector != nil {
		llmInsights, err := r.llmReflector.Reflect(ctx, report)
		if err != nil {
			log.Printf("RSIC LLM reflector failed (using rule-based only): %v", err)
		} else if len(llmInsights) > 0 {
			insights = deduplicateInsights(insights, llmInsights)
		}
	}

	// Sort by severity DESC
	sort.Slice(insights, func(i, j int) bool {
		return severityRank(insights[i].Severity) > severityRank(insights[j].Severity)
	})

	return insights, nil
}

// deduplicateInsights merges llmInsights into base, skipping any with a
// recommended_action already present in base.
func deduplicateInsights(base, llmInsights []ReflectionInsight) []ReflectionInsight {
	seen := make(map[string]bool, len(base))
	for _, i := range base {
		seen[i.RecommendedAction] = true
	}
	for _, li := range llmInsights {
		if !seen[li.RecommendedAction] {
			seen[li.RecommendedAction] = true
			base = append(base, li)
		}
	}
	return base
}

func severityRank(s InsightSeverity) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}
