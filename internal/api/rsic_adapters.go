package api

import (
	"context"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/ape"
	"mdemg/internal/consulting"
	"mdemg/internal/conversation"
	"mdemg/internal/hidden"
	"mdemg/internal/learning"
)

// constraintGateAdapter adapts consulting.ConstraintClassifier to conversation.ConstraintGateClassifier.
// This adapter bridges the consulting and conversation packages without creating a circular import.
type constraintGateAdapter struct {
	cc *consulting.ConstraintClassifier
}

func (a *constraintGateAdapter) Classify(ctx context.Context, nodeID, text string) (*conversation.ConstraintGateResult, error) {
	result, err := a.cc.Classify(ctx, nodeID, text)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &conversation.ConstraintGateResult{Type: result.Type}, nil
}

// rsicLearningAdapter adapts *learning.Service to ape.LearningStatsProvider.
type rsicLearningAdapter struct {
	svc *learning.Service
}

func (a *rsicLearningAdapter) GetLearningEdgeStats(ctx context.Context, spaceID string) (map[string]any, error) {
	return a.svc.GetLearningEdgeStats(ctx, spaceID)
}

func (a *rsicLearningAdapter) PruneDecayedEdges(ctx context.Context, spaceID string) (int64, error) {
	return a.svc.PruneDecayedEdges(ctx, spaceID)
}

func (a *rsicLearningAdapter) PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error) {
	return a.svc.PruneExcessEdgesPerNode(ctx, spaceID)
}

// rsicConvAdapter adapts *conversation.ContextCooler to ape.ConversationStatsProvider.
type rsicConvAdapter struct {
	cooler *conversation.ContextCooler
}

func (a *rsicConvAdapter) GetVolatileStats(ctx context.Context, spaceID string) (ape.VolatileStatsResult, error) {
	if a == nil || a.cooler == nil {
		return ape.VolatileStatsResult{}, nil
	}
	stats, err := a.cooler.GetVolatileStats(ctx, spaceID)
	if err != nil {
		return ape.VolatileStatsResult{}, err
	}
	return ape.VolatileStatsResult{
		VolatileCount:        stats.VolatileCount,
		PermanentCount:       stats.PermanentCount,
		AvgVolatileStability: stats.AvgVolatileStability,
	}, nil
}

// rsicHiddenAdapter adapts *hidden.Service to ape.HiddenLayerProvider.
type rsicHiddenAdapter struct {
	svc *hidden.Service
}

func (a *rsicHiddenAdapter) RunFullConversationConsolidation(ctx context.Context, spaceID string) (any, error) {
	return a.svc.RunFullConversationConsolidation(ctx, spaceID)
}

// rsicWatchdogSignalAdapter implements ape.WatchdogSignalProvider
// by wrapping SessionTracker and conversation.Service.
type rsicWatchdogSignalAdapter struct {
	sessionTracker *conversation.SessionTracker
	driver         neo4j.DriverWithContext
}

func (a *rsicWatchdogSignalAdapter) GetSessionHealthScore(sessionID string) float64 {
	if a.sessionTracker == nil {
		return 0
	}
	// Phase 89: Aggregate across all sessions when sessionID is empty
	if sessionID == "" {
		states := a.sessionTracker.GetAllStates()
		if len(states) == 0 {
			return 0
		}
		var total float64
		for _, s := range states {
			total += s.HealthScore()
		}
		return total / float64(len(states))
	}
	state := a.sessionTracker.GetState(sessionID)
	if state == nil {
		return 0
	}
	return state.HealthScore()
}

func (a *rsicWatchdogSignalAdapter) GetObservationRate(spaceID string) float64 {
	if a.sessionTracker == nil {
		return 0
	}
	// Phase 89: Aggregate across all active sessions instead of hard-coded "claude-core"
	sessions := a.sessionTracker.GetAllStates()
	var totalObs int
	var maxElapsed float64
	for _, state := range sessions {
		totalObs += state.ObservationsSinceResume
		elapsed := time.Since(state.CreatedAt).Hours()
		if elapsed > maxElapsed {
			maxElapsed = elapsed
		}
	}
	if maxElapsed < 0.01 {
		return 0
	}
	return float64(totalObs) / maxElapsed
}

func (a *rsicWatchdogSignalAdapter) GetConsolidationAgeSec(ctx context.Context, spaceID string) (int64, error) {
	sess := a.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
            MATCH (n:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme'})
            RETURN max(n.created_at) AS last_consolidation
        `, map[string]any{"spaceId": spaceID})
		if err != nil {
			return int64(0), err
		}
		if res.Next(ctx) {
			if v, ok := res.Record().Get("last_consolidation"); ok && v != nil {
				// Phase 89: Handle all Neo4j datetime types
				switch tv := v.(type) {
				case time.Time:
					return int64(time.Since(tv).Seconds()), nil
				case int64:
					return tv, nil
				case float64:
					return int64(tv), nil
				case string:
					if t, err := time.Parse(time.RFC3339, tv); err == nil {
						return int64(time.Since(t).Seconds()), nil
					}
				}
			}
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}
