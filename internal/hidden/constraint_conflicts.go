package hidden

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// ConstraintConflict represents a detected conflict between two constraint nodes.
type ConstraintConflict struct {
	ID               string  `json:"id"` // conflict edge ID (source_id + ":" + target_id)
	SourceNodeID     string  `json:"source_node_id"`
	SourceName       string  `json:"source_name"`
	TargetNodeID     string  `json:"target_node_id"`
	TargetName       string  `json:"target_name"`
	SimilarityScore  float64 `json:"similarity_score"`
	DetectionMethod  string  `json:"detection_method"`  // "embedding_similarity", "nli"
	ResolutionStatus string  `json:"resolution_status"` // "unresolved", "resolved"
	ResolutionText   string  `json:"resolution_text,omitempty"`
	DetectedAt       string  `json:"detected_at"`
}

// ConflictDetector detects pairwise conflicts between constraint nodes.
type ConflictDetector struct {
	driver       neo4j.DriverWithContext
	simThreshold float64
	maxPairs     int
}

// NewConflictDetector creates a new conflict detector.
func NewConflictDetector(driver neo4j.DriverWithContext, cfg config.Config) *ConflictDetector {
	simThreshold := cfg.ConstraintConflictSimThreshold
	if simThreshold <= 0 {
		simThreshold = 0.6
	}
	maxPairs := cfg.ConstraintConflictMaxPairs
	if maxPairs <= 0 {
		maxPairs = 500
	}
	return &ConflictDetector{
		driver:       driver,
		simThreshold: simThreshold,
		maxPairs:     maxPairs,
	}
}

// DetectConflicts finds pairs of constraint nodes in the given space whose embeddings
// are similar (above simThreshold) but have opposing constraint_type (e.g., must vs must_not).
// For each detected pair, a CONFLICTS_WITH edge is created if one doesn't already exist.
// Returns the number of new conflicts detected.
func (d *ConflictDetector) DetectConflicts(ctx context.Context, spaceID string) (int, error) {
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx) //nolint:errcheck

	// Find constraint pairs with high embedding similarity and opposing types
	cypher := `
MATCH (a:MemoryNode {space_id: $spaceID}), (b:MemoryNode {space_id: $spaceID})
WHERE a.constraint_type IS NOT NULL
  AND b.constraint_type IS NOT NULL
  AND a.node_id < b.node_id
  AND a.embedding IS NOT NULL
  AND b.embedding IS NOT NULL
  AND NOT EXISTS { MATCH (a)-[:CONFLICTS_WITH]-(b) }
WITH a, b,
     gds.similarity.cosine(a.embedding, b.embedding) AS sim
WHERE sim >= $simThreshold
  AND (
    (a.constraint_type IN ['must', 'should'] AND b.constraint_type IN ['must_not', 'should_not'])
    OR (a.constraint_type IN ['must_not', 'should_not'] AND b.constraint_type IN ['must', 'should'])
  )
WITH a, b, sim
LIMIT $maxPairs
CREATE (a)-[r:CONFLICTS_WITH {
  similarity_score: sim,
  detection_method: 'embedding_similarity',
  resolution_status: 'unresolved',
  resolution_text: '',
  detected_at: $detectedAt
}]->(b)
RETURN count(r) AS new_conflicts`

	now := time.Now().UTC().Format(time.RFC3339)

	result, err := neo4j.ExecuteWrite(ctx, sess, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceID":      spaceID,
			"simThreshold": d.simThreshold,
			"maxPairs":     d.maxPairs,
			"detectedAt":   now,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			raw, _ := rec.Get("new_conflicts")
			if n, ok := raw.(int64); ok {
				return int(n), res.Err()
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("detect conflicts: %w", err)
	}
	n, _ := result.(int)
	return n, nil
}

// ListConflicts returns unresolved conflicts for the given space.
func (d *ConflictDetector) ListConflicts(ctx context.Context, spaceID string, limit int) ([]ConstraintConflict, error) {
	if limit <= 0 {
		limit = 50
	}

	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
MATCH (a:MemoryNode {space_id: $spaceID})-[r:CONFLICTS_WITH]->(b:MemoryNode)
WHERE r.resolution_status = 'unresolved'
RETURN a.node_id AS source_id, a.name AS source_name,
       b.node_id AS target_id, b.name AS target_name,
       r.similarity_score AS similarity_score,
       r.detection_method AS detection_method,
       r.resolution_status AS resolution_status,
       r.resolution_text AS resolution_text,
       r.detected_at AS detected_at
ORDER BY r.similarity_score DESC
LIMIT $limit`

	result, err := neo4j.ExecuteRead(ctx, sess, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceID": spaceID,
			"limit":   limit,
		})
		if err != nil {
			return nil, err
		}
		var conflicts []ConstraintConflict
		for res.Next(ctx) {
			rec := res.Record()
			srcID := asStringVal(rec, "source_id")
			tgtID := asStringVal(rec, "target_id")
			conflicts = append(conflicts, ConstraintConflict{
				ID:               srcID + ":" + tgtID,
				SourceNodeID:     srcID,
				SourceName:       asStringVal(rec, "source_name"),
				TargetNodeID:     tgtID,
				TargetName:       asStringVal(rec, "target_name"),
				SimilarityScore:  asFloat64Val(rec, "similarity_score"),
				DetectionMethod:  asStringVal(rec, "detection_method"),
				ResolutionStatus: asStringVal(rec, "resolution_status"),
				ResolutionText:   asStringVal(rec, "resolution_text"),
				DetectedAt:       asStringVal(rec, "detected_at"),
			})
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return conflicts, nil
	})
	if err != nil {
		return nil, fmt.Errorf("list conflicts: %w", err)
	}
	if result == nil {
		return []ConstraintConflict{}, nil
	}
	return result.([]ConstraintConflict), nil
}

// ResolveConflict marks a conflict edge as resolved with the given resolution text.
func (d *ConflictDetector) ResolveConflict(ctx context.Context, sourceNodeID, targetNodeID, resolutionText string) error {
	sess := d.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: "neo4j"})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
MATCH (a:MemoryNode {node_id: $sourceID})-[r:CONFLICTS_WITH]-(b:MemoryNode {node_id: $targetID})
SET r.resolution_status = 'resolved',
    r.resolution_text = $resolutionText
RETURN r`

	_, err := neo4j.ExecuteWrite(ctx, sess, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"sourceID":       sourceNodeID,
			"targetID":       targetNodeID,
			"resolutionText": resolutionText,
		})
		if err != nil {
			return nil, err
		}
		_, err = res.Consume(ctx)
		return nil, err
	})
	if err != nil {
		return fmt.Errorf("resolve conflict: %w", err)
	}
	return nil
}

// asStringVal safely extracts a string from a Neo4j record by key.
func asStringVal(rec *neo4j.Record, key string) string {
	raw, _ := rec.Get(key)
	if raw == nil {
		return ""
	}
	s, _ := raw.(string)
	return s
}

// asFloat64Val safely extracts a float64 from a Neo4j record by key.
func asFloat64Val(rec *neo4j.Record, key string) float64 {
	raw, _ := rec.Get(key)
	if raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	default:
		return 0
	}
}
