package ape

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"

	"mdemg/internal/config"
	"mdemg/internal/tsdb"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Reflector analyses a SelfAssessmentReport and produces actionable insights.
type Reflector struct {
	cfg              config.Config
	driver           neo4j.DriverWithContext
	llmReflector     *LLMReflector
	protocolProvider ProtocolStatsProvider // J17: protocol metrics for reflection
	tsdbClient       *tsdb.Client          // optional: TimescaleDB client for schema drift detection
}

// NewReflector creates a Reflector.
func NewReflector(cfg config.Config, driver neo4j.DriverWithContext) *Reflector {
	return &Reflector{cfg: cfg, driver: driver}
}

// SetLLMReflector attaches an optional LLM-powered reflector for deeper analysis.
func (r *Reflector) SetLLMReflector(lr *LLMReflector) {
	r.llmReflector = lr
}

// SetProtocolProvider attaches a J17 protocol stats provider for protocol reflection.
func (r *Reflector) SetProtocolProvider(p ProtocolStatsProvider) {
	r.protocolProvider = p
}

// SetTSDBClient attaches an optional TimescaleDB client for schema drift detection.
func (r *Reflector) SetTSDBClient(c *tsdb.Client) {
	r.tsdbClient = c
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

	// 15. RSIC-SK1: Per-constraint confidence calibration
	if report.GuidanceHealth > 0 && report.GuidanceHealth < 0.7 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "guidance_confidence_drift",
			Severity:          SeverityMedium,
			Description:       "Guidance health below 70% — per-constraint confidence may need calibration",
			RecommendedAction: "adjust_guidance_confidence",
			Metric:            "guidance_health",
			Value:             report.GuidanceHealth,
			Threshold:         0.7,
		})
	}

	// 10-14. J17: Protocol reflection patterns
	if report.ProtocolHealth > 0 && r.protocolProvider != nil {
		protoStats, pErr := r.protocolProvider.GetProtocolStats(ctx, report.SpaceID)
		if pErr == nil {
			// 10. Codification opportunity: constraint sent as T2 too often
			for constraintID, count := range protoStats.T2FrequencyByConstraint {
				if count > r.cfg.J17CodificationThreshold {
					insights = append(insights, ReflectionInsight{
						PatternID:         "j17_codification_opportunity",
						Severity:          SeverityHigh,
						Description:       fmt.Sprintf("Constraint %s sent as T2 in %d sessions — candidate for T1 codification", constraintID, count),
						RecommendedAction: "codify_constraint",
						Metric:            "t2_frequency",
						Value:             float64(count),
						Threshold:         float64(r.cfg.J17CodificationThreshold),
					})
					break // one codification per cycle
				}
			}

			// 11. Low comprehension: code failing its purpose
			for code, rate := range protoStats.CodeComprehension {
				// Guard: don't retire based on insufficient or fallback-dominated data
				if count, ok := protoStats.CodeOutcomeCount[code]; ok && count < int64(r.cfg.J17TierEffectivenessMinSamples) {
					continue
				}
				if rate < r.cfg.J17ComprehensionMinThreshold {
					insights = append(insights, ReflectionInsight{
						PatternID:         "j17_low_comprehension",
						Severity:          SeverityHigh,
						Description:       fmt.Sprintf("Code '%s' comprehension %.0f%% — below %.0f%% threshold, retire to T2", code, rate*100, r.cfg.J17ComprehensionMinThreshold*100),
						RecommendedAction: "retire_code",
						Metric:            "code_comprehension",
						Value:             rate,
						Threshold:         r.cfg.J17ComprehensionMinThreshold,
					})
					break // one retirement per cycle
				}
			}

			// 12. High replay frequency: ticket renewal triggers may be failing
			if protoStats.ReplayFrequencyPerHour > r.cfg.J17ReplayFrequencyMax {
				insights = append(insights, ReflectionInsight{
					PatternID:         "j17_high_replay",
					Severity:          SeverityMedium,
					Description:       fmt.Sprintf("Event replay frequency %.1f/hr — ticket renewal triggers may be failing", protoStats.ReplayFrequencyPerHour),
					RecommendedAction: "adjust_replay_buffer",
					Metric:            "replay_frequency_per_hour",
					Value:             protoStats.ReplayFrequencyPerHour,
					Threshold:         r.cfg.J17ReplayFrequencyMax,
				})
			}

			// 13. Compression regression: encoding inefficiency
			if protoStats.CompressionRatio > 0 && protoStats.CompressionRatio < r.cfg.J17CompressionMinRatio {
				insights = append(insights, ReflectionInsight{
					PatternID:         "j17_compression_regression",
					Severity:          SeverityMedium,
					Description:       fmt.Sprintf("Compression ratio %.1fx — below %.1fx target, review tier selection parameters", protoStats.CompressionRatio, r.cfg.J17CompressionMinRatio),
					RecommendedAction: "adjust_tier_threshold",
					Metric:            "compression_ratio",
					Value:             protoStats.CompressionRatio,
					Threshold:         r.cfg.J17CompressionMinRatio,
				})
			}

			// 14. Low code coverage: codegen may be failing
			if protoStats.CodeCoverage < 0.8 {
				insights = append(insights, ReflectionInsight{
					PatternID:         "j17_low_code_coverage",
					Severity:          SeverityLow,
					Description:       fmt.Sprintf("Only %.0f%% of active constraints have T1 codes — codegen may need attention", protoStats.CodeCoverage*100),
					RecommendedAction: "codify_constraint",
					Metric:            "code_coverage",
					Value:             protoStats.CodeCoverage,
					Threshold:         0.8,
				})
			}

			// 14b. Cold start: 0% code coverage with active events — bootstrap all constraints
			if protoStats.CodeCoverage == 0 && protoStats.TotalEvents > 0 {
				insights = append(insights, ReflectionInsight{
					PatternID:         "j17_cold_start_codification",
					Severity:          SeverityHigh,
					Description:       "J17 cold start: 0% code coverage — bootstrap codification needed",
					RecommendedAction: "codify_all_constraints",
					Metric:            "code_coverage",
					Value:             0,
					Threshold:         0.1,
				})
			}

			// 15. Tier ineffectiveness: specific tier degrades comprehension for a code
			if r.cfg.J17TierDriftDetectionEnabled && len(protoStats.TierCodeComprehension) > 0 {
				for tier, codemap := range protoStats.TierCodeComprehension {
					if tier < 1 || tier > 3 {
						continue
					}
					for code, comp := range codemap {
						if protoStats.TierOutcomeCount[tier-1] < int64(r.cfg.J17TierEffectivenessMinSamples) {
							continue
						}
						if comp < r.cfg.J17TierIneffectiveThreshold {
							for otherTier, otherMap := range protoStats.TierCodeComprehension {
								if otherTier == tier {
									continue
								}
								if otherComp, ok := otherMap[code]; ok && otherComp > comp+0.15 {
									insights = append(insights, ReflectionInsight{
										PatternID:         "j17_tier_ineffective",
										Severity:          SeverityHigh,
										Description:       fmt.Sprintf("Code '%s' at T%d: %.0f%% comprehension vs %.0f%% at T%d", code, tier, comp*100, otherComp*100, otherTier),
										RecommendedAction: "adjust_tier_threshold",
										Metric:            "tier_comprehension_delta",
										Value:             comp,
										Threshold:         r.cfg.J17TierIneffectiveThreshold,
									})
									break
								}
							}
						}
					}
				}
			}

			// 16. NLI calibration drift
			if protoStats.NLIBiasAlert {
				insights = append(insights, ReflectionInsight{
					PatternID:         "j17_nli_calibration_drift",
					Severity:          SeverityMedium,
					Description:       fmt.Sprintf("NLI shows %.0f%% mean bias vs heuristic", protoStats.NLIMeanBias*100),
					RecommendedAction: "review_nli_calibration",
					Metric:            "nli_mean_bias",
					Value:             math.Abs(protoStats.NLIMeanBias),
					Threshold:         r.cfg.J17NLICalibrationBiasThreshold,
				})
			}
		}
	}

	// 17-20. Synergy monitoring patterns
	if r.cfg.SynergyAssessmentEnabled {
		// 17. Jiminy down but synergy migration occurred — catastrophic forgetting risk
		if !report.JiminyHealthy && (report.SynergyLinesClaude+report.SynergyLinesMemory) > 0 {
			insights = append(insights, ReflectionInsight{
				PatternID:         "synergy_jiminy_unhealthy",
				Severity:          SeverityCritical,
				Description:       "Jiminy is down but .md files are pruned — catastrophic forgetting risk. Restore Jiminy immediately.",
				RecommendedAction: "alert_jiminy_critical",
				Metric:            "jiminy_healthy",
				Value:             0,
				Threshold:         1,
			})
		}

		// 18. Memory file bloat — overflow rate exceeds alert threshold
		if report.SynergyOverflowRate > float64(r.cfg.SynergyOverflowAlertThreshold) {
			insights = append(insights, ReflectionInsight{
				PatternID:         "memory_file_bloat",
				Severity:          SeverityMedium,
				Description:       fmt.Sprintf("Synergy overflow rate %.1f exceeds threshold %d — memory files may be bloating", report.SynergyOverflowRate, r.cfg.SynergyOverflowAlertThreshold),
				RecommendedAction: "alert_memory_bloat",
				Metric:            "synergy_overflow_rate",
				Value:             report.SynergyOverflowRate,
				Threshold:         float64(r.cfg.SynergyOverflowAlertThreshold),
			})
		}

		// 19. Synergy overlap drift — overlap score exceeds 0.4 threshold
		if report.SynergyOverlapScore > 0.4 {
			insights = append(insights, ReflectionInsight{
				PatternID:         "synergy_overlap_drift",
				Severity:          SeverityMedium,
				Description:       fmt.Sprintf("Synergy overlap score %.2f exceeds 0.4 threshold — CMS and .md content are diverging", report.SynergyOverlapScore),
				RecommendedAction: "alert_synergy_overlap",
				Metric:            "synergy_overlap_score",
				Value:             report.SynergyOverlapScore,
				Threshold:         0.4,
			})
		}

		// 20. Recovery buffer has pending entries — Jiminy outage data needs flushing
		if report.SynergyRecoveryBufferEntries > 0 {
			severity := SeverityMedium
			if report.SynergyRecoveryBufferEntries > 20 {
				severity = SeverityHigh
			}
			insights = append(insights, ReflectionInsight{
				PatternID:         "synergy_recovery_buffer_pending",
				Severity:          severity,
				Description:       fmt.Sprintf("Recovery buffer has %d pending entries from Jiminy outage — flush when Jiminy recovers", report.SynergyRecoveryBufferEntries),
				RecommendedAction: "flush_recovery_buffer",
				Metric:            "synergy_recovery_buffer_entries",
				Value:             float64(report.SynergyRecoveryBufferEntries),
				Threshold:         0,
			})
		}
	}

	// Phase 47.2: Stale ingest detection
	if r.cfg.APEIngestSyncEnabled && report.StaleIngestSpaces > 0 {
		severity := SeverityLow
		if report.StaleIngestSpaces >= 3 {
			severity = SeverityMedium
		}
		insights = append(insights, ReflectionInsight{
			PatternID:         "stale_ingest",
			Severity:          severity,
			Description:       fmt.Sprintf("%d space(s) past ingest staleness threshold (%dh)", report.StaleIngestSpaces, r.cfg.SyncStaleThresholdHours),
			RecommendedAction: "ingest_stale_spaces",
			Metric:            "stale_ingest_spaces",
			Value:             float64(report.StaleIngestSpaces),
			Threshold:         0,
		})
	}

	// Schema version drift between databases
	if r.tsdbClient != nil {
		tsdbVer, err := r.tsdbClient.GetSchemaVersion(ctx)
		if err != nil {
			insights = append(insights, ReflectionInsight{
				PatternID:         "schema_drift_tsdb_unreachable",
				Severity:          SeverityMedium,
				Description:       fmt.Sprintf("TimescaleDB unreachable during schema check: %v", err),
				RecommendedAction: "alert_tsdb_health",
			})
		} else if tsdbVer < r.cfg.TSDBRequiredSchemaVersion {
			insights = append(insights, ReflectionInsight{
				PatternID:         "schema_drift_detected",
				Severity:          SeverityHigh,
				Description:       fmt.Sprintf("TimescaleDB schema v%d behind required v%d", tsdbVer, r.cfg.TSDBRequiredSchemaVersion),
				RecommendedAction: "alert_schema_drift",
			})
		}
	}

	// Phase AR-3: Merge LLM reflector insights (fail-open — rule-based results used alone on error)
	if r.llmReflector != nil {
		llmInsights, err := r.llmReflector.Reflect(ctx, report)
		if err != nil {
			slog.Warn("RSIC LLM reflector failed, using rule-based only", "error", err)
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
