// HIDDEN-CHURN-001 — stable conversation-theme identity.
//
// ClusterConversations previously detached EVERY observation→theme edge,
// deleted childless themes, and re-created all themes from scratch each
// cycle (~5 min cadence): new node_ids every run, evidence chains destroyed
// continuously, and recall flooded with stacks of near-identical concepts.
// Themes are now matched to existing nodes by centroid similarity and
// updated IN PLACE — node identity (and everything referencing it) survives
// across cycles; only themes matched by no current cluster are deleted.
package hidden

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// themeRef is an existing conversation theme available for identity matching.
type themeRef struct {
	NodeID   string
	Centroid []float64
}

// listConversationThemes returns the space's themes with their centroids.
func (s *Service) listConversationThemes(ctx context.Context, spaceID string) ([]themeRef, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (t:ConversationTheme {space_id: $spaceId})
WHERE t.embedding IS NOT NULL
RETURN t.node_id AS nodeId, t.embedding AS centroid`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var refs []themeRef
		for res.Next(ctx) {
			rec := res.Record()
			id, _ := rec.Get("nodeId")
			emb, _ := rec.Get("centroid")
			ref := themeRef{NodeID: id.(string)}
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

// matchTheme returns the best unclaimed existing theme whose centroid cosine
// similarity to the cluster centroid meets the threshold ("" when none).
func matchTheme(centroid []float64, existing []themeRef, claimed map[string]bool, threshold float64) string {
	best, bestSim := "", threshold
	for _, ref := range existing {
		if claimed[ref.NodeID] || len(ref.Centroid) == 0 {
			continue
		}
		if sim := cosineSimilarity(centroid, ref.Centroid); sim >= bestSim {
			best, bestSim = ref.NodeID, sim
		}
	}
	return best
}

// updateConversationThemeWithEdges refreshes a matched theme in place —
// properties + a THEME-SCOPED member-edge rewire (the global detach is
// gone). The node_id and all inbound references survive.
func (s *Service) updateConversationThemeWithEdges(ctx context.Context, spaceID, nodeID, summary string, centroid []float64, members []ConversationObservation, dominantType string, avgSurprise float64) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
MATCH (t:ConversationTheme {space_id: $spaceId, node_id: $nodeId})
SET t.summary = $summary,
    t.embedding = $centroid,
    t.message_pass_embedding = $centroid,
    t.aggregation_count = $memberCount,
    t.dominant_type = $dominantType,
    t.avg_surprise_score = $avgSurprise,
    t.updated_at = datetime()
WITH t
// Theme-scoped rewire: drop only THIS theme's member edges, then relink.
OPTIONAL MATCH (old)-[oldEdge:GENERALIZES]->(t)
DELETE oldEdge
WITH DISTINCT t
UNWIND $memberEdges AS me
MATCH (o:MemoryNode {space_id: $spaceId, node_id: me.memberId})
WITH t, o, me,
     CASE WHEN t.embedding IS NOT NULL AND o.embedding IS NOT NULL
          THEN vector.similarity.cosine(o.embedding, t.embedding)
          ELSE 0.5
     END AS similarity
CREATE (o)-[:GENERALIZES {
  space_id: $spaceId,
  edge_id: me.edgeId,
  weight: similarity,
  similarity_score: similarity,
  created_at: datetime(),
  updated_at: datetime()
}]->(t)
RETURN count(o) AS edgeCount`

	out, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":      spaceID,
			"nodeId":       nodeID,
			"summary":      summary,
			"centroid":     toFloat32Slice(centroid),
			"memberCount":  len(members),
			"memberEdges":  memberEdgePairs(memberIDs),
			"dominantType": dominantType,
			"avgSurprise":  avgSurprise,
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
		return 0, fmt.Errorf("update theme %s: %w", nodeID, err)
	}
	return out.(int), nil
}

// deleteUnmatchedThemes removes themes claimed by NO cluster this run —
// the replacement for the old delete-everything-then-recreate orphan sweep.
// Detach-delete here is the SAME operation the old sweep performed; the
// difference is scope (only genuinely stale themes die).
func (s *Service) deleteUnmatchedThemes(ctx context.Context, spaceID string, claimed map[string]bool, existing []themeRef) (int, error) {
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

	out, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
UNWIND $ids AS id
MATCH (t:ConversationTheme {space_id: $spaceId, node_id: id})
DETACH DELETE t
RETURN count(*) AS removed`, map[string]any{"spaceId": spaceID, "ids": stale})
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
		return 0, err
	}
	return out.(int), nil
}

// assignNoiseToThemes density-assigns sub-threshold (noise) observations to
// their nearest CURRENT theme when cosine ≥ threshold — edges only, no new
// themes (HIDDEN-CHURN-001 PR-B coverage retune). Observations below the
// floor remain unthemed, honestly.
func (s *Service) assignNoiseToThemes(ctx context.Context, spaceID string, noise []ConversationObservation, threshold float64) (int, error) {
	themes, err := s.listConversationThemes(ctx, spaceID)
	if err != nil {
		return 0, err
	}
	if len(themes) == 0 {
		return 0, nil
	}

	type pair struct{ obsID, themeID string }
	var pairs []pair
	for _, obs := range noise {
		if len(obs.Embedding) == 0 {
			continue
		}
		best, bestSim := "", threshold
		for _, t := range themes {
			if len(t.Centroid) == 0 {
				continue
			}
			if sim := cosineSimilarity(obs.Embedding, t.Centroid); sim >= bestSim {
				best, bestSim = t.NodeID, sim
			}
		}
		if best != "" {
			pairs = append(pairs, pair{obs.NodeID, best})
		}
	}
	if len(pairs) == 0 {
		return 0, nil
	}

	rows := make([]map[string]any, len(pairs))
	for i, p := range pairs {
		rows[i] = map[string]any{"obsId": p.obsID, "themeId": p.themeID, "edgeId": memberEdgePairs([]string{p.obsID})[0]["edgeId"]}
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	out, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
UNWIND $rows AS row
MATCH (o:MemoryNode {space_id: $spaceId, node_id: row.obsId})
MATCH (t:ConversationTheme {space_id: $spaceId, node_id: row.themeId})
WHERE NOT (o)-[:GENERALIZES]->(t)
WITH o, t, row,
     CASE WHEN o.embedding IS NOT NULL AND t.embedding IS NOT NULL
          THEN vector.similarity.cosine(o.embedding, t.embedding)
          ELSE 0.5 END AS similarity
CREATE (o)-[:GENERALIZES {
  space_id: $spaceId, edge_id: row.edgeId,
  weight: similarity, similarity_score: similarity,
  density_assigned: true,
  created_at: datetime(), updated_at: datetime()
}]->(t)
RETURN count(o) AS assigned`, map[string]any{"spaceId": spaceID, "rows": rows})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			n, _ := res.Record().Get("assigned")
			return asInt(n), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	return out.(int), nil
}
