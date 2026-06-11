// RSIC-VALIDATE-001 — honest success-criteria evaluation.
//
// Before this sprint the cycle baseline populated 10 metric keys while task
// criteria referenced ~15 others — only volatile_count and correction_rate
// intersected. Every other criterion resolved to missing_data, which the
// evaluator skipped, so ~16/17 actions validated vacuously and the
// criteria-driven rollback path was unreachable.
package ape

// reportMetricsMap extracts every criterion-resolvable metric from a
// self-assessment report. Single source for MetricsBefore AND MetricsAfter —
// the key mismatch that caused vacuous validation cannot recur as long as
// both sides come from here.
func reportMetricsMap(report *SelfAssessmentReport) map[string]float64 {
	if report == nil {
		return nil
	}
	return map[string]float64{
		"overall_health":        report.OverallHealth,
		"retrieval_quality":     report.RetrievalQuality,
		"memory_health":         report.MemoryHealth,
		"edge_health":           report.EdgeHealth,
		"guidance_health":       report.GuidanceHealth,
		"protocol_health":       report.ProtocolHealth,
		"total_nodes":           float64(report.TotalNodes),
		"edge_count":            float64(report.EdgeCount),
		"total_edges":           float64(report.EdgeCount), // criteria alias
		"orphan_ratio":          report.OrphanRatio,
		"orphan_count":          float64(report.OrphanCount),
		"volatile_count":        float64(report.VolatileCount),
		"permanent_count":       float64(report.PermanentCount),
		"correction_rate":       report.CorrectionRate,
		"edge_weight_entropy":   report.EdgeWeightEntropy,
		"avg_edge_weight":       report.AvgEdgeWeight,
		"edges_below_threshold": float64(report.EdgesBelowThreshold),
		"consolidation_age_sec": float64(report.ConsolidationAgeSec),
		"synergy_health":        report.SynergyHealth,
	}
}

// mutatingActions are RSIC actions that change graph or guidance state.
// For these, fail-closed validation applies: a criterion whose evidence is
// missing counts as NOT met (an unverifiable mutation must not be recorded
// as a success). Observational / alerting actions keep advisory semantics.
var mutatingActions = map[string]bool{
	"prune_decayed_edges":             true,
	"prune_excess_edges":              true,
	"trigger_consolidation":           true,
	"graduate_volatile":               true,
	"tombstone_stale":                 true,
	"refresh_stale_edges":             true,
	"codify_constraint":               true,
	"codify_all_constraints":          true,
	"retire_code":                     true,
	"adjust_tier_threshold":           true,
	"adjust_replay_buffer":            true,
	"adjust_guidance_confidence":      true,
	"archive_ineffective_constraints": true,
	"ingest_stale_spaces":             true,
	"flush_recovery_buffer":           true,
}

// isMutatingAction reports whether the named action mutates state.
func isMutatingAction(action string) bool { return mutatingActions[action] }
