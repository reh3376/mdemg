package ape

import (
	"fmt"
	"time"

	"mdemg/internal/config"
)

// BuildTaskSpec translates an ImprovementAction into a fully specified RSICTaskSpec.
func BuildTaskSpec(cfg config.Config, action ImprovementAction, cycleID string, baseline map[string]float64) *RSICTaskSpec {
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
			MaxNodesAffected: int(float64(1000) * cfg.RSICMaxNodePrunePct),
			MaxEdgesAffected: int(float64(1000) * cfg.RSICMaxEdgePrunePct),
			ProtectedSpaces:  []string{"mdemg-dev"},
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
	default:
		return "Unknown action type: " + actionType
	}
}
