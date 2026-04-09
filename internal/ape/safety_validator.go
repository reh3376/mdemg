package ape

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/metrics"
)

// SafetyDecision is the result of evaluating safety bounds for an action.
type SafetyDecision struct {
	Allowed           bool   `json:"allowed"`
	Reason            string `json:"reason,omitempty"`
	EstimatedAffected int    `json:"estimated_affected"`
	Limit             int    `json:"limit"`
}

// SafetyValidator checks safety bounds before RSIC mutation executors run.
type SafetyValidator struct {
	driver neo4j.DriverWithContext
}

// NewSafetyValidator creates a safety validator wired to the Neo4j driver.
func NewSafetyValidator(driver neo4j.DriverWithContext) *SafetyValidator {
	return &SafetyValidator{driver: driver}
}

// ValidateAction checks whether the given action is safe to execute.
// It enforces protected-space policy and blast-radius limits.
func (sv *SafetyValidator) ValidateAction(ctx context.Context, spec *RSICTaskSpec, actionType string) SafetyDecision {
	// Non-destructive actions are always allowed
	if !IsDestructiveAction(actionType) {
		return SafetyDecision{Allowed: true, Limit: -1}
	}

	// Check protected-space policy for destructive actions
	for _, ps := range spec.Safety.ProtectedSpaces {
		if ps == spec.TargetSpace {
			metrics.Metrics().RSICSafetyBlocked(actionType, "protected_space").Inc()
			return SafetyDecision{
				Allowed: false,
				Reason:  fmt.Sprintf("space %s is protected from destructive RSIC actions", spec.TargetSpace),
			}
		}
	}

	// Estimate blast radius
	estimated, err := sv.EstimateBlastRadius(ctx, actionType, spec.TargetSpace)
	if err != nil {
		slog.Warn("RSIC safety: blast radius estimation failed, allowing with caution", "action", actionType, "error", err)
		// Allow on estimation failure — don't block cycles due to count query errors
		return SafetyDecision{Allowed: true, Reason: "estimation_error", EstimatedAffected: -1}
	}

	// Determine applicable limit
	limit := sv.limitForAction(actionType, spec)
	if limit > 0 && estimated > limit {
		metrics.Metrics().RSICSafetyBlocked(actionType, "blast_radius").Inc()
		return SafetyDecision{
			Allowed:           false,
			Reason:            fmt.Sprintf("blast radius %d exceeds limit %d for %s", estimated, limit, actionType),
			EstimatedAffected: estimated,
			Limit:             limit,
		}
	}

	return SafetyDecision{
		Allowed:           true,
		EstimatedAffected: estimated,
		Limit:             limit,
	}
}

// BuildDelta creates an ActionDelta for dry-run mode without executing.
func (sv *SafetyValidator) BuildDelta(ctx context.Context, spec *RSICTaskSpec, actionType string) ActionDelta {
	delta := ActionDelta{
		Action: actionType,
	}

	// Check protected-space
	if IsDestructiveAction(actionType) {
		for _, ps := range spec.Safety.ProtectedSpaces {
			if ps == spec.TargetSpace {
				delta.WouldExecute = false
				delta.ProtectedSpaceBlocked = true
				delta.RejectionReason = fmt.Sprintf("space %s is protected from destructive RSIC actions", spec.TargetSpace)
				return delta
			}
		}
	}

	// Estimate blast radius
	estimated, err := sv.EstimateBlastRadius(ctx, actionType, spec.TargetSpace)
	if err != nil {
		delta.EstimatedAffected = -1
		delta.WouldExecute = true
		delta.WithinBounds = true
		delta.Note = "estimation error: " + err.Error()
		return delta
	}

	delta.EstimatedAffected = estimated

	if !IsDestructiveAction(actionType) {
		delta.SafetyLimit = -1
		delta.WouldExecute = true
		delta.WithinBounds = true
		delta.Note = "constructive action, no blast-radius limit"
		return delta
	}

	limit := sv.limitForAction(actionType, spec)
	delta.SafetyLimit = limit
	delta.WithinBounds = limit <= 0 || estimated <= limit
	delta.WouldExecute = delta.WithinBounds
	if !delta.WithinBounds {
		delta.RejectionReason = fmt.Sprintf("blast radius %d exceeds limit %d", estimated, limit)
	}
	return delta
}

// EstimateBlastRadius runs a lightweight COUNT query to estimate how many
// nodes or edges would be affected by the given action.
func (sv *SafetyValidator) EstimateBlastRadius(ctx context.Context, actionType string, spaceID string) (int, error) {
	cypher, params := sv.countQueryForAction(actionType, spaceID)
	if cypher == "" {
		return 0, nil // no estimation query for this action
	}

	if sv.driver == nil {
		return -1, fmt.Errorf("no driver available for blast radius estimation")
	}

	sess := sv.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("affected"); ok {
				switch n := v.(type) {
				case int64:
					return int(n), nil
				case float64:
					return int(n), nil
				}
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

func (sv *SafetyValidator) limitForAction(actionType string, spec *RSICTaskSpec) int {
	switch actionType {
	case "prune_decayed_edges", "prune_excess_edges":
		return spec.Safety.MaxEdgesAffected
	case "tombstone_stale":
		return spec.Safety.MaxNodesAffected
	default:
		return -1 // no limit for non-destructive
	}
}

func (sv *SafetyValidator) countQueryForAction(actionType string, spaceID string) (string, map[string]any) {
	params := map[string]any{"spaceId": spaceID}

	switch actionType {
	case "prune_decayed_edges":
		return `MATCH ()-[e:CO_ACTIVATED_WITH {space_id: $spaceId}]-()
			WHERE e.weight < 0.1
			RETURN count(e) AS affected`, params

	case "prune_excess_edges":
		// Count edges that would be pruned due to excess per node
		return `MATCH (n:MemoryNode {space_id: $spaceId})-[e:CO_ACTIVATED_WITH]-()
			WITH n, count(e) AS edgeCount
			WHERE edgeCount > 50
			RETURN sum(edgeCount - 50) AS affected`, params

	case "tombstone_stale":
		return `MATCH (correction:MemoryNode {space_id: $spaceId, obs_type: 'correction'})
			WHERE correction.created_at > datetime() - duration('P7D')
			WITH correction
			MATCH (stale:MemoryNode {space_id: $spaceId})
			WHERE stale.role_type = 'conversation_observation'
			  AND stale.obs_type <> 'correction'
			  AND stale.created_at < correction.created_at
			  AND NOT coalesce(stale.is_archived, false)
			RETURN count(DISTINCT stale) AS affected`, params

	default:
		return "", nil
	}
}
