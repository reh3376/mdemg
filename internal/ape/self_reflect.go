package ape

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

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
	datasetProvider  tsdb.DatasetProvider  // RSIC-DATA: curated datasets for trend-based reflection
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

// SetDatasetProvider attaches a TSDB curated dataset provider for trend-based reflection (RSIC-DATA).
func (r *Reflector) SetDatasetProvider(p tsdb.DatasetProvider) {
	r.datasetProvider = p
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
		// 17. Jiminy down but synergy migration occurred — catastrophic forgetting risk.
		// JIMINY-SIGNAL-001: guard on SynergyAssessed — JiminyHealthy is only
		// meaningful when the synergy block actually ran and set it. Without this
		// guard the Go zero-value false (synergy reader unwired/skipped) fired this
		// CRITICAL ~8×/day on a healthy, actively-delivering Jiminy.
		if report.SynergyAssessed && !report.JiminyHealthy && (report.SynergyLinesClaude+report.SynergyLinesMemory) > 0 {
			slog.Warn("rsic: synergy_jiminy_unhealthy CRITICAL firing",
				"synergy_assessed", report.SynergyAssessed,
				"jiminy_healthy", report.JiminyHealthy,
				"synergy_lines", report.SynergyLinesClaude+report.SynergyLinesMemory)
			insights = append(insights, ReflectionInsight{
				PatternID:         "synergy_jiminy_unhealthy",
				Severity:          SeverityCritical,
				Description:       "Jiminy service unavailable (disabled or not initialized) while .md files are pruned — catastrophic forgetting risk. Restore Jiminy immediately.",
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

	// 21. Sidecar down while Jiminy enabled — J17 protocol degraded
	if report.JiminyHealthy && !report.SidecarHealthy {
		severity := SeverityHigh
		desc := "Neural sidecar is down — J17 protocol running in fallback mode (100% T3, no ML tier prediction, no NLI scoring)."
		if report.ProtocolHealth > 0 {
			severity = SeverityCritical
			desc = "Neural sidecar is down with active J17 traffic — all events using T3 fallback. Restart sidecar immediately."
		}
		insights = append(insights, ReflectionInsight{
			PatternID:         "sidecar_unhealthy",
			Severity:          severity,
			Description:       desc,
			RecommendedAction: "alert_sidecar_down",
			Metric:            "sidecar_healthy",
			Value:             0,
			Threshold:         1,
		})
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

	// ─── RSIC-DATA: TSDB-aware reflection patterns ───

	// 25. LLM latency regression: p95 > 2× historical average (7d trend)
	for _, perf := range report.LLMPerformance {
		if perf.LatencyP95 > 0 && r.datasetProvider != nil {
			trend, tErr := r.datasetProvider.MetricTrend(ctx, report.SpaceID,
				fmt.Sprintf("mdemg_llm_%s_latency_p95", perf.TaskName), 7*24*time.Hour)
			if tErr == nil && trend != nil && trend.Slope > 0 && trend.AvgValue > 0 && perf.LatencyP95 > trend.AvgValue*2 {
				insights = append(insights, ReflectionInsight{
					PatternID:         "llm_latency_regression",
					Severity:          SeverityMedium,
					Description:       fmt.Sprintf("Task %s p95 latency %.0fms exceeds 2× average %.0fms (slope: +%.1f)", perf.TaskName, perf.LatencyP95, trend.AvgValue, trend.Slope),
					RecommendedAction: "review_llm_provider",
					Metric:            "llm_latency_p95",
					Value:             perf.LatencyP95,
					Threshold:         trend.AvgValue * 2,
				})
			}
		}
	}

	// 26. LLM error rate spike: >5% error rate.
	// SUPERVISOR-002 recency gate: the error-rate window is 24h, so a
	// transient burst kept this insight (and its HIGH alert) firing for up
	// to a day after the incident self-resolved. Require the most recent
	// error to be fresh (RSIC_LLM_ERROR_RECENCY_MIN, default 60; 0 = legacy
	// behavior, no gate).
	recency := time.Duration(r.cfg.RSICLLMErrorRecencyMin) * time.Minute
	for _, perf := range report.LLMPerformance {
		if perf.ErrorRate > 0.05 && perf.TotalCalls > 10 {
			// ALERT-TRUTH-001: absolute error-count floor. The rate-only gate
			// re-fired HIGH 23× on just 2 "context canceled" errors (5.7% of 35
			// low-volume calls). Require a meaningful absolute count so a couple
			// of transient errors at low volume can't pin a HIGH alert.
			errorCount := int(perf.ErrorRate*float64(perf.TotalCalls) + 0.5)
			if r.cfg.RSICLLMErrorMinCount > 0 && errorCount < r.cfg.RSICLLMErrorMinCount {
				slog.Debug("RSIC reflect: llm_error_rate_spike suppressed (below count floor)",
					"task", perf.TaskName, "error_count", errorCount, "min_count", r.cfg.RSICLLMErrorMinCount)
				continue
			}
			if recency > 0 && !perf.LastErrorAt.IsZero() && time.Since(perf.LastErrorAt) > recency {
				slog.Debug("RSIC reflect: llm_error_rate_spike suppressed (stale)",
					"task", perf.TaskName, "last_error_at", perf.LastErrorAt, "recency_gate", recency)
				continue
			}
			insights = append(insights, ReflectionInsight{
				PatternID:         "llm_error_rate_spike",
				Severity:          SeverityHigh,
				Description:       fmt.Sprintf("Task %s error rate %.1f%% exceeds 5%% (%d calls)", perf.TaskName, perf.ErrorRate*100, perf.TotalCalls),
				RecommendedAction: "alert_llm_health",
				Metric:            "llm_error_rate",
				Value:             perf.ErrorRate,
				Threshold:         0.05,
			})
		}
	}

	// 27. Retrieval quality degradation: rerank < 90% or guidance correlation < 80%
	if report.RetrievalDataset != nil && report.RetrievalDataset.TotalQueries > 10 {
		if report.RetrievalDataset.RerankRate < 0.9 {
			insights = append(insights, ReflectionInsight{
				PatternID:         "retrieval_quality_degradation",
				Severity:          SeverityHigh,
				Description:       fmt.Sprintf("Rerank rate %.1f%% below 90%% (%d queries)", report.RetrievalDataset.RerankRate*100, report.RetrievalDataset.TotalQueries),
				RecommendedAction: "review_guidance_effectiveness",
				Metric:            "retrieval_rerank_rate",
				Value:             report.RetrievalDataset.RerankRate,
				Threshold:         0.9,
			})
		}
		if report.RetrievalDataset.GuidanceCorrelation < 0.8 {
			insights = append(insights, ReflectionInsight{
				PatternID:         "retrieval_quality_degradation",
				Severity:          SeverityMedium,
				Description:       fmt.Sprintf("Guidance correlation %.1f%% below 80%% — feedback loop may be broken", report.RetrievalDataset.GuidanceCorrelation*100),
				RecommendedAction: "review_guidance_effectiveness",
				Metric:            "retrieval_guidance_correlation",
				Value:             report.RetrievalDataset.GuidanceCorrelation,
				Threshold:         0.8,
			})
		}
	}

	// 28. Embedding pipeline regression: empty call_sites reappearing (Sprint 1 regression check)
	if report.EmbeddingDataset != nil && report.EmbeddingDataset.EmptyCallSites > 0 {
		insights = append(insights, ReflectionInsight{
			PatternID:         "embedding_pipeline_regression",
			Severity:          SeverityCritical,
			Description:       fmt.Sprintf("Embedding pipeline regression: %d events with empty call_site", report.EmbeddingDataset.EmptyCallSites),
			RecommendedAction: "alert_embedding_regression",
			Metric:            "embedding_empty_call_sites",
			Value:             float64(report.EmbeddingDataset.EmptyCallSites),
			Threshold:         0,
		})
	}

	// 29. Training data readiness: any task has sufficient data for SFT
	if report.TrainingReadiness != nil {
		readyCount := 0
		for _, task := range report.TrainingReadiness.Tasks {
			if task.Ready {
				readyCount++
			}
		}
		if readyCount > 0 {
			insights = append(insights, ReflectionInsight{
				PatternID:         "training_data_ready",
				Severity:          SeverityLow,
				Description:       fmt.Sprintf("%d/%d tasks have sufficient training data for SFT", readyCount, len(report.TrainingReadiness.Tasks)),
				RecommendedAction: "trigger_training_pipeline",
				Metric:            "training_ready_tasks",
				Value:             float64(readyCount),
				Threshold:         1,
			})
		}
	}

	// 30. Trust trajectory decline: 24h slope < -0.01
	if r.datasetProvider != nil {
		trustTrend, tErr := r.datasetProvider.MetricTrend(ctx, report.SpaceID, "mdemg_j17_avg_trust_score", 24*time.Hour)
		if tErr == nil && trustTrend != nil && trustTrend.Slope < -0.01 && len(trustTrend.Points) > 2 {
			lastVal := trustTrend.Points[len(trustTrend.Points)-1].Value
			insights = append(insights, ReflectionInsight{
				PatternID:         "trust_trajectory_decline",
				Severity:          SeverityMedium,
				Description:       fmt.Sprintf("Trust score declining (slope: %.4f/interval, current: %.3f)", trustTrend.Slope, lastVal),
				RecommendedAction: "review_guidance_effectiveness",
				Metric:            "trust_trajectory_slope",
				Value:             trustTrend.Slope,
				Threshold:         -0.01,
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
//
// deterministicAlertActions are threshold-gated alerts produced ONLY by the
// rule-based reflector from real metrics. The LLM reflector must never introduce
// them (RSIC-LLM-ALERT-GUARD-001): when an LLM insight recommends one and the
// rule-based path did NOT (because the real condition is false), the merge would
// otherwise admit a hallucinated, ungrounded alert — e.g. a false
// `alert_jiminy_critical` "Jiminy Service Unavailable" while jiminy_healthy=true.
// This is a structural guard independent of the AllowedLLMActions whitelist, so
// it holds even if those actions are re-added there.
var deterministicAlertActions = map[string]bool{
	"alert_jiminy_critical": true,
	"alert_memory_bloat":    true,
	"alert_synergy_overlap": true,
}

func deduplicateInsights(base, llmInsights []ReflectionInsight) []ReflectionInsight {
	seen := make(map[string]bool, len(base))
	for _, i := range base {
		seen[i.RecommendedAction] = true
	}
	for _, li := range llmInsights {
		if deterministicAlertActions[li.RecommendedAction] {
			// LLM may not introduce a deterministic alert the rule-based path
			// didn't already raise — prevents ungrounded hallucinated CRITICALs.
			slog.Warn("rsic: dropping LLM-recommended deterministic alert (rule-based path owns it)",
				"action", li.RecommendedAction, "pattern", li.PatternID)
			continue
		}
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
