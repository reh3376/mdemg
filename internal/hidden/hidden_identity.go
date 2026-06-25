// HIDDEN-CHURN-002 — stable hidden-pattern (L1) identity.
//
// CreateHiddenNodes previously detached EVERY L0→L1 GENERALIZES edge, deleted
// all childless HiddenPattern nodes, and re-created every pattern from scratch
// each consolidation cycle (~5 min): new randomUUID() node_ids every run,
// ~2,636 nodes + ~31,106 edges destroyed and rebuilt continuously, firing the
// CRITICAL graph_node_drop alert and orphaning every reinforcement / abstraction
// edge that referenced a pattern node_id. This mirrors the HIDDEN-CHURN-001
// theme fix (theme_identity.go): patterns are matched to existing nodes by
// centroid similarity and updated IN PLACE — node identity (and everything
// referencing it) survives across cycles; only patterns matched by no current
// cluster are deleted, and new patterns get a CUIDv2 id.
package hidden

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// hiddenPatternIdentityThreshold resolves the centroid-similarity floor for
// hidden-pattern identity matching (HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD,
// default 0.90), parallel to themeIdentityThreshold.
func (s *Service) hiddenPatternIdentityThreshold() float64 {
	if s.cfg.HiddenPatternIdentitySimThreshold > 0 {
		return s.cfg.HiddenPatternIdentitySimThreshold
	}
	return 0.90
}

// listHiddenPatterns returns the space's L1 hidden patterns with their
// centroids, available for identity matching (reuses themeRef — node_id +
// centroid is the same shape, so matchTheme applies unchanged).
func (s *Service) listHiddenPatterns(ctx context.Context, spaceID string) ([]themeRef, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (h:HiddenPattern {space_id: $spaceId, layer: 1})
WHERE h.embedding IS NOT NULL
RETURN h.node_id AS nodeId, h.embedding AS centroid`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var refs []themeRef
		for res.Next(ctx) {
			rec := res.Record()
			id, _ := rec.Get("nodeId")
			emb, _ := rec.Get("centroid")
			idStr, ok := id.(string)
			if !ok || idStr == "" {
				continue
			}
			ref := themeRef{NodeID: idStr}
			if arr, ok := emb.([]any); ok {
				ref.Centroid = make([]float64, 0, len(arr))
				for _, v := range arr {
					switch f := v.(type) {
					case float64:
						ref.Centroid = append(ref.Centroid, f)
					case float32:
						ref.Centroid = append(ref.Centroid, float64(f))
					}
				}
			}
			refs = append(refs, ref)
		}
		return refs, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return out.([]themeRef), nil
}

// updateHiddenNodeWithEdges refreshes a matched hidden pattern in place —
// properties + a NODE-SCOPED member-edge rewire (the global detach is gone).
// The node_id and all inbound references survive. Edge weights are recomputed
// with vector.similarity.cosine (HIDDEN-WEIGHT-001), and category/
// category_summary are carried exactly as createHiddenNodeWithEdges sets them.
func (s *Service) updateHiddenNodeWithEdges(ctx context.Context, spaceID, nodeID, name string, centroid []float64, members []BaseNode, category, categorySummary string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
MATCH (h:HiddenPattern {space_id: $spaceId, node_id: $nodeId})
SET h.name = $name,
    h.embedding = $centroid,
    h.message_pass_embedding = $centroid,
    h.aggregation_count = $memberCount,
    h.last_forward_pass = datetime(),
    h.updated_at = datetime(),
    h.version = coalesce(h.version, 1) + 1
WITH h
// Node-scoped rewire: drop only THIS pattern's member edges, then relink.
OPTIONAL MATCH (old:MemoryNode {space_id: $spaceId, layer: 0})-[oldEdge:GENERALIZES]->(h)
DELETE oldEdge
WITH DISTINCT h
UNWIND $memberEdges AS me
MATCH (b:MemoryNode {space_id: $spaceId, node_id: me.memberId})
WITH h, b, me,
     CASE WHEN b.embedding IS NOT NULL AND h.embedding IS NOT NULL
          THEN vector.similarity.cosine(b.embedding, h.embedding)
          ELSE 0.5
     END AS similarity
CREATE (b)-[:GENERALIZES {
  space_id: $spaceId,
  edge_id: me.edgeId,
  weight: similarity,
  category: $category,
  category_summary: $categorySummary,
  created_at: datetime(),
  updated_at: datetime()
}]->(h)
RETURN count(b) AS edgeCount`

	out, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":         spaceID,
			"nodeId":          nodeID,
			"name":            name,
			"centroid":        toFloat32Slice(centroid),
			"memberCount":     len(members),
			"memberEdges":     memberEdgePairs(memberIDs),
			"category":        category,
			"categorySummary": categorySummary,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			ec, _ := res.Record().Get("edgeCount")
			return asInt(ec), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("update hidden pattern %s: %w", nodeID, err)
	}
	return out.(int), nil
}

// deleteUnmatchedHiddenPatterns removes hidden patterns claimed by NO cluster
// this run — the replacement for the old detach-everything-then-recreate orphan
// sweep. Batched (the unmatched set can be large on the first post-deploy run)
// and mdemg-dev-safe: only genuinely-stale patterns die, never a wholesale wipe.
func (s *Service) deleteUnmatchedHiddenPatterns(ctx context.Context, spaceID string, claimed map[string]bool, existing []themeRef) (int, error) {
	var stale []string
	for _, ref := range existing {
		if !claimed[ref.NodeID] {
			stale = append(stale, ref.NodeID)
		}
	}
	if len(stale) == 0 {
		return 0, nil
	}
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	const batchSize = 500
	total := 0
	for start := 0; start < len(stale); start += batchSize {
		end := start + batchSize
		if end > len(stale) {
			end = len(stale)
		}
		batch := stale[start:end]
		removed, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, `
UNWIND $ids AS id
MATCH (h:HiddenPattern {space_id: $spaceId, node_id: id})
DETACH DELETE h
RETURN count(*) AS removed`, map[string]any{"spaceId": spaceID, "ids": batch})
			if err != nil {
				return 0, err
			}
			if res.Next(ctx) {
				n, _ := res.Record().Get("removed")
				return asInt(n), res.Err()
			}
			return 0, res.Err()
		})
		if err != nil {
			return total, fmt.Errorf("delete unmatched hidden patterns: %w", err)
		}
		total += removed.(int)
	}
	return total, nil
}
