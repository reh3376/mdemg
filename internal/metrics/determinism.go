package metrics

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// DeterminismScore represents the behavioral determinism metrics for a space.
type DeterminismScore struct {
	SpaceID         string  `json:"space_id"`
	Score           float64 `json:"score"`           // D-score: (informed/total) * compliance * coverage
	InformedActions int     `json:"informed_actions"` // actions with guidance available
	TotalActions    int     `json:"total_actions"`    // total observed actions
	ComplianceRate  float64 `json:"compliance_rate"`  // followed / (followed + ignored + contradicted)
	CoverageRatio   float64 `json:"coverage_ratio"`   // constraints with recent surfacing / total constraints
}

// ComputeDeterminismScore calculates the D-score for a space:
//
//	D = (informed_actions / total_actions) * compliance_rate * coverage_ratio
func ComputeDeterminismScore(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (*DeterminismScore, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx) //nolint:errcheck

	// Query: Get total observations (proxy for total actions) and guided ones
	cypher := `
MATCH (n:MemoryNode {space_id: $spaceID, role_type: 'conversation_observation'})
WITH count(n) AS total_actions
OPTIONAL MATCH (g:MemoryNode {space_id: $spaceID})
WHERE g.constraint_type IS NOT NULL
WITH total_actions, count(g) AS total_constraints
OPTIONAL MATCH (g2:MemoryNode {space_id: $spaceID})
WHERE g2.constraint_type IS NOT NULL
  AND g2.last_surfaced_at IS NOT NULL
WITH total_actions, total_constraints, count(g2) AS surfaced_constraints
OPTIONAL MATCH (g3:MemoryNode {space_id: $spaceID})-[r:GUIDANCE_OUTCOME]-()
WITH total_actions, total_constraints, surfaced_constraints,
     count(CASE WHEN r.outcome_type = 'followed' THEN 1 END) AS followed,
     count(CASE WHEN r.outcome_type = 'ignored' THEN 1 END) AS ignored,
     count(CASE WHEN r.outcome_type = 'contradicted' THEN 1 END) AS contradicted,
     count(r) AS total_outcomes
RETURN total_actions, total_constraints, surfaced_constraints,
       followed, ignored, contradicted, total_outcomes`

	result, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceID": spaceID,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return &DeterminismScore{SpaceID: spaceID}, res.Err()
		}
		rec := res.Record()

		totalActions := toInt(rec, "total_actions")
		totalConstraints := toInt(rec, "total_constraints")
		surfacedConstraints := toInt(rec, "surfaced_constraints")
		followed := toInt(rec, "followed")
		ignored := toInt(rec, "ignored")
		contradicted := toInt(rec, "contradicted")
		totalOutcomes := toInt(rec, "total_outcomes")

		// informed_actions = total outcomes (each outcome represents an informed action)
		informedActions := totalOutcomes

		// compliance_rate = followed / (followed + ignored + contradicted)
		var complianceRate float64
		denominator := followed + ignored + contradicted
		if denominator > 0 {
			complianceRate = float64(followed) / float64(denominator)
		}

		// coverage_ratio = surfaced_constraints / total_constraints
		var coverageRatio float64
		if totalConstraints > 0 {
			coverageRatio = float64(surfacedConstraints) / float64(totalConstraints)
		}

		// D-score = (informed/total) * compliance * coverage
		var dScore float64
		if totalActions > 0 {
			dScore = (float64(informedActions) / float64(totalActions)) * complianceRate * coverageRatio
		}

		return &DeterminismScore{
			SpaceID:         spaceID,
			Score:           dScore,
			InformedActions: informedActions,
			TotalActions:    totalActions,
			ComplianceRate:  complianceRate,
			CoverageRatio:   coverageRatio,
		}, res.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("compute determinism score: %w", err)
	}
	return result.(*DeterminismScore), nil
}

func toInt(rec *neo4j.Record, key string) int {
	raw, _ := rec.Get(key)
	if raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case int64:
		return int(v)
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}
