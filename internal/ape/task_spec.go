package ape

import (
	"fmt"
	"time"

	"mdemg/internal/config"
)

// BuildTaskSpec translates an ImprovementAction into a fully specified RSICTaskSpec.
// graphSize is the actual node count in the target space; blast radius is computed as a percentage of it.
func BuildTaskSpec(cfg config.Config, action ImprovementAction, cycleID string, baseline map[string]float64, graphSize int) *RSICTaskSpec {
	maxNodes := max(int(float64(graphSize)*cfg.RSICMaxNodePrunePct), 10)
	maxEdges := max(int(float64(graphSize)*cfg.RSICMaxEdgePrunePct), 10)
	spec := &RSICTaskSpec{
		TaskID:          fmt.Sprintf("%s-%s-%d", cycleID, action.ActionType, time.Now().UnixMilli()),
		CycleID:         cycleID,
		ActionType:      action.ActionType,
		Description:     descriptionForAction(action.ActionType),
		Rationale:       action.Rationale,
		TargetSpace:     action.TargetSpace,
		Priority:        action.Priority,
		BaselineMetrics: baseline,
		Safety: SafetyBounds{
			MaxNodesAffected: maxNodes,
			MaxEdgesAffected: maxEdges,
			ProtectedSpaces:  cfg.RSICProtectedSpaces,
			DryRun:           false,
		},
		ReportSchedule: ReportSchedule{
			IntervalType: "milestone",
			Values:       []string{"snapshot_taken", "dry_run_complete", "execution_complete", "validation_complete"},
		},
	}

	switch action.ActionType {
	case "prune_decayed_edges":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/learning/prune", Purpose: "prune decayed learning edges"},
			{Method: "GET", Path: "/v1/learning/stats", Purpose: "verify edge counts after prune"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Count of edges pruned", Format: "json", Required: true},
			{Name: "validation_check", Description: "Post-prune edge stats", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "edges_below_threshold_delta", Operator: "lt", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "prune_excess_edges":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/learning/prune", Purpose: "prune excess edges per node"},
			{Method: "GET", Path: "/v1/learning/stats", Purpose: "verify edge counts after prune"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Count of excess edges removed", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "total_edges_delta", Operator: "lt", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "trigger_consolidation":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/conversation/consolidate", Purpose: "trigger CMS consolidation"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Consolidation results (themes, concepts created)", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "consolidation_age_sec", Operator: "lt", Threshold: 3600},
		}
		spec.Timeout = 10 * time.Minute

	case "graduate_volatile":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/conversation/graduate", Purpose: "process volatile observation graduations"},
			{Method: "GET", Path: "/v1/conversation/volatile/stats", Purpose: "check volatile stats"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Graduation summary", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "volatile_count_delta", Operator: "lt", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "tombstone_stale":
		spec.AllowedEndpoints = []EndpointSpec{}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Nodes tombstoned", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "correction_rate_delta", Operator: "lte", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "refresh_stale_edges":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/memory/edges/stale/refresh", Purpose: "refresh stale edges"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Edges refreshed", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "avg_edge_weight_delta", Operator: "gt", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "archive_ineffective_constraints":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/v1/constraints/effectiveness", Purpose: "query constraint effectiveness"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Constraints archived due to low effectiveness", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "constraint_effectiveness_delta", Operator: "gt", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "review_guidance_effectiveness":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/v1/constraints/effectiveness", Purpose: "review guidance effectiveness"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Guidance effectiveness review", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "guidance_follow_rate_delta", Operator: "gt", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "adjust_guidance_confidence":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/v1/constraints/effectiveness", Purpose: "query constraint effectiveness"},
			{Method: "PATCH", Path: "/v1/memory/nodes/{id}/confidence", Purpose: "update node confidence"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "boosted", Description: "Count of items with confidence boosted", Format: "json", Required: true},
			{Name: "decayed", Description: "Count of items with confidence decayed", Format: "json", Required: true},
			{Name: "archived", Description: "Count of stale constraints archived", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "guidance_health_delta", Operator: "gte", Threshold: 0},
		}
		spec.Timeout = 5 * time.Minute

	case "codify_constraint":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/jiminy/protocol/codify", Purpose: "generate and freeze T1 code for constraint"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Constraint code generated and frozen", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "code_coverage_delta", Operator: "gt", Threshold: 0},
		}
		spec.Timeout = 2 * time.Minute

	case "codify_all_constraints":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/jiminy/protocol/codify", Purpose: "generate and freeze T1 code for each uncoded constraint"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Count of constraints codified during bootstrap", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "code_coverage_delta", Operator: "gt", Threshold: 0},
		}
		spec.Timeout = 10 * time.Minute

	case "retire_code":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/jiminy/protocol/retire", Purpose: "retire T1 code back to T2"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Code retired, constraint reverted to T2", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "avg_comprehension_delta", Operator: "gt", Threshold: 0},
		}
		spec.Timeout = 1 * time.Minute

	case "adjust_tier_threshold":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/jiminy/protocol/adjust-tiers", Purpose: "adjust tier selection parameters"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Tier thresholds adjusted", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "compression_ratio_delta", Operator: "gt", Threshold: 0},
		}
		spec.Timeout = 1 * time.Minute

	case "adjust_replay_buffer":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/jiminy/protocol/adjust-buffer", Purpose: "adjust replay buffer size"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Replay buffer adjusted", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "replay_frequency_delta", Operator: "lt", Threshold: 0},
		}
		spec.Timeout = 1 * time.Minute

	case "ingest_stale_spaces":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/v1/memory/freshness", Purpose: "query stale space list"},
			{Method: "POST", Path: "/v1/ingest", Purpose: "trigger incremental ingest for stale spaces"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Count of spaces re-ingested", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "stale_spaces_delta", Operator: "lte", Threshold: 0},
		}
		spec.Timeout = 10 * time.Minute

	case "alert_jiminy_critical":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/healthz", Purpose: "check server health"},
			{Method: "GET", Path: "/v1/jiminy/healthz", Purpose: "check jiminy subsystem health"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "alert_report", Description: "Jiminy critical alert published", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "alert_published", Operator: "eq", Threshold: 1},
		}
		spec.Timeout = 1 * time.Minute

	case "alert_memory_bloat":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/v1/synergy/status", Purpose: "check synergy/memory status"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "alert_report", Description: "Memory bloat assessment", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "alert_published", Operator: "eq", Threshold: 1},
		}
		spec.Timeout = 1 * time.Minute

	case "alert_synergy_overlap":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/v1/synergy/status", Purpose: "check synergy overlap status"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "alert_report", Description: "Synergy overlap assessment", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "alert_published", Operator: "eq", Threshold: 1},
		}
		spec.Timeout = 1 * time.Minute

	case "flush_recovery_buffer":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "POST", Path: "/v1/synergy/flush-buffer", Purpose: "flush recovery buffer entries"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "execution_report", Description: "Count of buffer entries flushed", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "entries_flushed", Operator: "gte", Threshold: 0},
		}
		spec.Timeout = 2 * time.Minute

	case "review_nli_calibration":
		spec.AllowedEndpoints = []EndpointSpec{
			{Method: "GET", Path: "/v1/jiminy/protocol/metrics", Purpose: "retrieve NLI calibration metrics"},
		}
		spec.Deliverables = []Deliverable{
			{Name: "calibration_report", Description: "NLI comprehension calibration report", Format: "json", Required: true},
		}
		spec.SuccessCriteria = []Criterion{
			{Metric: "calibration_published", Operator: "eq", Threshold: 1},
		}
		spec.Timeout = 2 * time.Minute

	default:
		spec.Timeout = 5 * time.Minute
	}

	return spec
}

func descriptionForAction(actionType string) string {
	switch actionType {
	case "prune_decayed_edges":
		return "Remove learning edges whose decayed weight has fallen below threshold"
	case "prune_excess_edges":
		return "Remove excess edges per node to prevent over-connectivity"
	case "trigger_consolidation":
		return "Run conversation consolidation to create themes and emergent concepts"
	case "graduate_volatile":
		return "Process volatile observations ready for graduation to permanent storage"
	case "tombstone_stale":
		return "Archive stale nodes that have been superseded by corrections"
	case "refresh_stale_edges":
		return "Refresh stale learning edges by re-computing their weights"
	case "archive_ineffective_constraints":
		return "Archive constraints with consistently low effectiveness rates"
	case "review_guidance_effectiveness":
		return "Review and optimize guidance effectiveness based on follow rates"
	case "adjust_guidance_confidence":
		return "Boost high-performing and decay low-performing guidance confidence based on effectiveness"
	case "codify_constraint":
		return "Generate T1 mnemonic code for constraint currently sent as T2"
	case "codify_all_constraints":
		return "Bootstrap codification: generate T1 codes for all uncoded constraints (cold start)"
	case "retire_code":
		return "Retire T1 code with low comprehension back to T2 telegraphic encoding"
	case "adjust_tier_threshold":
		return "Adjust tier selection parameters to improve compression ratio"
	case "adjust_replay_buffer":
		return "Adjust replay buffer size to reduce replay frequency"
	case "ingest_stale_spaces":
		return "Re-ingest stale spaces that have not been updated within the staleness threshold"
	case "alert_jiminy_critical":
		return "Publish alert for critical Jiminy guidance pipeline failure"
	case "alert_memory_bloat":
		return "Assess and alert on excessive memory/node growth"
	case "alert_synergy_overlap":
		return "Assess and alert on synergy layer overlap or redundancy"
	case "flush_recovery_buffer":
		return "Flush recovery buffer entries back into the memory pipeline"
	case "review_nli_calibration":
		return "Review NLI comprehension calibration metrics and publish report"
	default:
		return "Unknown action type: " + actionType
	}
}
