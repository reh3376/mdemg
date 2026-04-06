package jiminy

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// PersistenceStore provides write-through Neo4j persistence for guidance outcomes.
// It records GUIDANCE_OUTCOME edges from source nodes to lightweight outcome records,
// enabling constraint effectiveness analysis across sessions.
type PersistenceStore struct {
	driver neo4j.DriverWithContext
	cfg    config.Config
}

// NewPersistenceStore creates a new PersistenceStore.
func NewPersistenceStore(driver neo4j.DriverWithContext, cfg config.Config) *PersistenceStore {
	return &PersistenceStore{
		driver: driver,
		cfg:    cfg,
	}
}

// ConstraintEffectiveness holds per-constraint aggregated guidance outcome metrics.
type ConstraintEffectiveness struct {
	NodeID            string  `json:"node_id"`
	Name              string  `json:"name"`
	Confidence        float64 `json:"confidence"`
	TotalSurfaced     int     `json:"total_surfaced"`
	TotalFollowed     int     `json:"total_followed"`
	TotalIgnored      int     `json:"total_ignored"`
	TotalContradicted int     `json:"total_contradicted"`
	EffectivenessRate float64 `json:"effectiveness_rate"` // followed / surfaced
}

// PersistGuidanceOutcome creates a GUIDANCE_OUTCOME edge on the appropriate node.
// For constraint items with a constraint_code, it targets the matching constraint
// node (role_type='constraint') so effectiveness metrics are correctly attributed.
// For other item types, it targets the first source node. No-op if no target found.
func (ps *PersistenceStore) PersistGuidanceOutcome(
	ctx context.Context,
	spaceID, guidanceID, sessionID string,
	item GuidanceItem,
	outcome GuidanceOutcome,
	similarity float64,
) error {
	// Resolve target node: prefer constraint node for any item with a constraint_code.
	// Constraint codes are assigned to ALL guidance types (constraints, corrections,
	// patterns, learnings) via matchConstraintCode — not just type="constraint" items.
	targetNodeID := ""
	if item.ConstraintCode != "" {
		targetNodeID = ps.findConstraintNodeID(ctx, spaceID, item.ConstraintCode)
	}
	if targetNodeID == "" && len(item.SourceNodes) > 0 {
		targetNodeID = item.SourceNodes[0]
	}
	if targetNodeID == "" {
		return nil
	}

	sess := ps.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx) //nolint:errcheck

	// Only persist outcomes on typed guidance nodes (constraint, correction, pattern,
	// learning). Generic MemoryNodes (code descriptions, progress notes) produce
	// meaningless similarity comparisons and pollute outcome data.
	// NOTE: constraint nodes resolved by findConstraintNodeID use role_type='constraint'
	// (not obs_type), so we check both properties.
	cypher := `
		MATCH (src:MemoryNode {node_id: $targetNodeID, space_id: $spaceID})
		WHERE src.obs_type IN ['constraint', 'correction', 'pattern', 'learning']
		   OR src.role_type = 'constraint'
		CREATE (src)-[r:GUIDANCE_OUTCOME {
			guidance_id:   $guidanceID,
			outcome_type:  $outcomeType,
			guidance_type: $guidanceType,
			similarity:    $similarity,
			session_id:    $sessionID,
			created_at:    $createdAt
		}]->(src)
		RETURN r`

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, txErr := tx.Run(ctx, cypher, map[string]any{
			"targetNodeID": targetNodeID,
			"spaceID":      spaceID,
			"guidanceID":   guidanceID,
			"outcomeType":  string(outcome),
			"guidanceType": string(item.Type),
			"similarity":   similarity,
			"sessionID":    sessionID,
			"createdAt":    time.Now().UTC().Format(time.RFC3339),
		})
		return nil, txErr
	})
	if err != nil {
		return fmt.Errorf("persist guidance outcome: %w", err)
	}

	return nil
}

// findConstraintNodeID looks up a constraint node by its constraint_code property.
// Returns the node_id if found, empty string otherwise.
func (ps *PersistenceStore) findConstraintNodeID(ctx context.Context, spaceID, constraintCode string) string {
	sess := ps.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, `
			MATCH (c:MemoryNode {space_id: $spaceID, role_type: 'constraint', constraint_code: $code})
			RETURN c.node_id AS cid
			LIMIT 1`,
			map[string]any{"spaceID": spaceID, "code": constraintCode})
		if txErr != nil {
			return "", txErr
		}
		if !res.Next(ctx) {
			return "", nil
		}
		v, _ := res.Record().Get("cid")
		s, _ := v.(string)
		return s, nil
	})
	if err != nil || result == nil {
		return ""
	}
	return result.(string)
}

// GetConstraintEffectiveness queries Neo4j for all constraint nodes in the space
// and returns aggregated GUIDANCE_OUTCOME statistics per constraint.
func (ps *PersistenceStore) GetConstraintEffectiveness(ctx context.Context, spaceID string) ([]ConstraintEffectiveness, error) {
	sess := ps.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
		MATCH (c:MemoryNode {space_id: $spaceID, role_type: 'constraint'})
		OPTIONAL MATCH (c)-[r:GUIDANCE_OUTCOME]-()
		RETURN
			c.node_id    AS node_id,
			c.name       AS name,
			c.confidence AS confidence,
			count(r)                                                         AS total_surfaced,
			count(CASE WHEN r.outcome_type = 'followed'     THEN 1 END)     AS total_followed,
			count(CASE WHEN r.outcome_type = 'ignored'      THEN 1 END)     AS total_ignored,
			count(CASE WHEN r.outcome_type = 'contradicted' THEN 1 END)     AS total_contradicted`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, txErr := tx.Run(ctx, cypher, map[string]any{
			"spaceID": spaceID,
		})
		if txErr != nil {
			return nil, txErr
		}

		var rows []ConstraintEffectiveness
		for res.Next(ctx) {
			rec := res.Record()

			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			confidence, _ := rec.Get("confidence")
			totalSurfaced, _ := rec.Get("total_surfaced")
			totalFollowed, _ := rec.Get("total_followed")
			totalIgnored, _ := rec.Get("total_ignored")
			totalContradicted, _ := rec.Get("total_contradicted")

			surfaced := asInt(totalSurfaced)
			followed := asInt(totalFollowed)

			var rate float64
			if surfaced > 0 {
				rate = float64(followed) / float64(surfaced)
			}

			rows = append(rows, ConstraintEffectiveness{
				NodeID:            asString(nodeID),
				Name:              asString(name),
				Confidence:        asFloat64(confidence),
				TotalSurfaced:     surfaced,
				TotalFollowed:     followed,
				TotalIgnored:      asInt(totalIgnored),
				TotalContradicted: asInt(totalContradicted),
				EffectivenessRate: rate,
			})
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return rows, nil
	})
	if err != nil {
		return nil, fmt.Errorf("get constraint effectiveness: %w", err)
	}

	if result == nil {
		return []ConstraintEffectiveness{}, nil
	}
	return result.([]ConstraintEffectiveness), nil
}

// asInt safely converts an interface{} value returned by the Neo4j driver to int.
// Handles int64 (the Neo4j default integer type), int, and float64.
func asInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
