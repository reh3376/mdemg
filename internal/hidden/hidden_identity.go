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
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// hiddenPatternIdentityThreshold resolves the centroid-similarity floor for the
// FALLBACK match (HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD, default 0.90).
func (s *Service) hiddenPatternIdentityThreshold() float64 {
	if s.cfg.HiddenPatternIdentitySimThreshold > 0 {
		return s.cfg.HiddenPatternIdentitySimThreshold
	}
	return 0.90
}

// hiddenPatternMemberJaccardThreshold resolves the PRIMARY member-overlap floor
// (HIDDEN_PATTERN_MEMBER_JACCARD_THRESHOLD, default 0.5). ≤0 disables member
// matching (centroid-only fallback path).
func (s *Service) hiddenPatternMemberJaccardThreshold() float64 {
	if s.cfg.HiddenPatternMemberJaccardThreshold > 0 {
		return s.cfg.HiddenPatternMemberJaccardThreshold
	}
	return 0.5
}

// hiddenIncrementalAssignThreshold resolves the cosine floor for assigning an
// orphan L0 node to an existing pattern in the incremental path
// (HIDDEN_INCREMENTAL_ASSIGN_SIM_THRESHOLD, default 0.80). Below it, the orphan
// is left for the new-pattern clustering step rather than forced into a
// poorly-matching pattern.
func (s *Service) hiddenIncrementalAssignThreshold() float64 {
	if s.cfg.HiddenIncrementalAssignSimThreshold > 0 {
		return s.cfg.HiddenIncrementalAssignSimThreshold
	}
	return 0.80
}

// nearestPatternByCentroid returns the node_id of the pattern whose centroid is
// closest (cosine) to emb and clears threshold, or "" if none does.
func nearestPatternByCentroid(emb []float64, refs []hiddenPatternRef, threshold float64) string {
	best, bestSim := "", threshold
	for _, r := range refs {
		if len(r.Centroid) == 0 {
			continue
		}
		if sim := cosineSimilarity(emb, r.Centroid); sim >= bestSim {
			best, bestSim = r.NodeID, sim
		}
	}
	return best
}

// incrementalMean folds k new vectors (summed into sum) into an existing mean of
// n vectors: (old·n + sum) / (n + k). Returns old unchanged on a length/empty
// mismatch.
func incrementalMean(old, sum []float64, n, k int) []float64 {
	if len(old) != len(sum) || (n+k) == 0 {
		return old
	}
	out := make([]float64, len(old))
	denom := float64(n + k)
	for i := range old {
		out[i] = (old[i]*float64(n) + sum[i]) / denom
	}
	return out
}

// assignOrphansToPatterns is HIDDEN-CHURN-003's incremental core: each orphan L0
// node is attached to its nearest existing pattern (cosine ≥ threshold) via a
// GENERALIZES edge, and that pattern's centroid is updated by incremental mean.
// Existing patterns are NEVER destroyed or re-clustered — only extended — so
// their node_ids (and everything referencing them) survive the cycle. Orphans
// that clear no pattern are RETURNED for the caller to cluster into new
// patterns. Mirrors assignNoiseToThemes (HIDDEN-CHURN-001 PR-B) for the L1 layer.
func (s *Service) assignOrphansToPatterns(ctx context.Context, spaceID string, orphans []BaseNode, refs []hiddenPatternRef, threshold float64) (assigned int, unassigned []BaseNode, err error) {
	if len(refs) == 0 {
		return 0, orphans, nil // no patterns yet — everything clusters fresh
	}

	type assignment struct{ obsID, patternID string }
	var pairs []assignment
	// Per-pattern accumulation for the incremental-mean centroid update.
	sums := map[string][]float64{}
	counts := map[string]int{}
	refByID := make(map[string]*hiddenPatternRef, len(refs))
	for i := range refs {
		refByID[refs[i].NodeID] = &refs[i]
	}

	for _, o := range orphans {
		if len(o.Embedding) == 0 {
			continue
		}
		best := nearestPatternByCentroid(o.Embedding, refs, threshold)
		if best == "" {
			unassigned = append(unassigned, o)
			continue
		}
		pairs = append(pairs, assignment{o.NodeID, best})
		if sums[best] == nil {
			sums[best] = make([]float64, len(o.Embedding))
		}
		for i, v := range o.Embedding {
			sums[best][i] += v
		}
		counts[best]++
	}

	if len(pairs) == 0 {
		return 0, unassigned, nil
	}

	// 1) Batch-create the GENERALIZES edges (cosine weight, CUIDv2 edge id).
	rows := make([]map[string]any, len(pairs))
	for i, p := range pairs {
		rows[i] = map[string]any{"obsId": p.obsID, "patternId": p.patternID, "edgeId": memberEdgePairs([]string{p.obsID})[0]["edgeId"]}
	}
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	out, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
UNWIND $rows AS row
MATCH (b:MemoryNode {space_id: $spaceId, node_id: row.obsId})
MATCH (h:HiddenPattern {space_id: $spaceId, node_id: row.patternId})
WHERE NOT (b)-[:GENERALIZES]->(h)
WITH b, h, row,
     CASE WHEN b.embedding IS NOT NULL AND h.embedding IS NOT NULL
          THEN vector.similarity.cosine(b.embedding, h.embedding)
          ELSE 0.5 END AS similarity
CREATE (b)-[:GENERALIZES {
  space_id: $spaceId, edge_id: row.edgeId,
  weight: similarity, incremental_assigned: true,
  created_at: datetime(), updated_at: datetime()
}]->(h)
RETURN count(b) AS assigned`, map[string]any{"spaceId": spaceID, "rows": rows})
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
		return 0, orphans, fmt.Errorf("assign orphans to patterns: %w", err)
	}
	assigned = out.(int)

	// 2) Incremental-mean centroid update for the patterns that gained members:
	//    new = (old·n + Σnew) / (n + k). Keeps centroids honest without
	//    re-fetching the full member set.
	centroidRows := make([]map[string]any, 0, len(sums))
	for pid, sum := range sums {
		ref := refByID[pid]
		if ref == nil || len(ref.Centroid) != len(sum) {
			continue
		}
		n := len(ref.Members)
		k := counts[pid]
		newC := incrementalMean(ref.Centroid, sum, n, k)
		centroidRows = append(centroidRows, map[string]any{"patternId": pid, "centroid": toFloat32Slice(newC), "count": n + k})
	}
	if len(centroidRows) > 0 {
		_, cErr := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			return nil, func() error {
				_, e := tx.Run(ctx, `
UNWIND $rows AS row
MATCH (h:HiddenPattern {space_id: $spaceId, node_id: row.patternId})
SET h.embedding = row.centroid,
    h.message_pass_embedding = row.centroid,
    h.aggregation_count = row.count,
    h.updated_at = datetime()`, map[string]any{"spaceId": spaceID, "rows": centroidRows})
				return e
			}()
		})
		if cErr != nil {
			slog.Warn("assignOrphansToPatterns: centroid update failed", "error", cErr)
		}
	}

	return assigned, unassigned, nil
}

// hiddenPatternRef is an existing L1 pattern available for identity matching. It
// carries BOTH identity signals: the centroid (cosine fallback) and the L0
// member set (Jaccard — stable under KMeans repartition jitter, which centroid
// cosine alone is not: live Tier-3 left ~28% churn/cycle on centroid-only).
type hiddenPatternRef struct {
	NodeID   string
	Centroid []float64
	Members  map[string]bool
}

// listHiddenPatternRefs returns the space's L1 hidden patterns with their
// centroids AND their current L0 member sets.
func (s *Service) listHiddenPatternRefs(ctx context.Context, spaceID string) ([]hiddenPatternRef, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (h:HiddenPattern {space_id: $spaceId, layer: 1})
WHERE h.embedding IS NOT NULL
OPTIONAL MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[:GENERALIZES]->(h)
RETURN h.node_id AS nodeId, h.embedding AS centroid, collect(b.node_id) AS members`,
			map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var refs []hiddenPatternRef
		for res.Next(ctx) {
			rec := res.Record()
			id, _ := rec.Get("nodeId")
			emb, _ := rec.Get("centroid")
			mem, _ := rec.Get("members")
			idStr, ok := id.(string)
			if !ok || idStr == "" {
				continue
			}
			ref := hiddenPatternRef{NodeID: idStr, Members: map[string]bool{}}
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
			if arr, ok := mem.([]any); ok {
				for _, v := range arr {
					if mid, ok := v.(string); ok && mid != "" {
						ref.Members[mid] = true
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
	return out.([]hiddenPatternRef), nil
}

// buildMemberIndex maps each L0 member node_id → indices of refs containing it,
// so a new cluster's overlap with every candidate pattern is tallied in one
// pass over its members instead of O(clusters × patterns).
func buildMemberIndex(refs []hiddenPatternRef) map[string][]int {
	idx := make(map[string][]int)
	for i, r := range refs {
		for m := range r.Members {
			idx[m] = append(idx[m], i)
		}
	}
	return idx
}

// matchHiddenPattern returns the best unclaimed existing pattern for a new
// cluster: member-overlap (Jaccard) PRIMARY, centroid-cosine FALLBACK. Returns
// "" when neither clears its threshold (→ create a new pattern).
func matchHiddenPattern(clusterMembers []string, centroid []float64, refs []hiddenPatternRef, idx map[string][]int, claimed map[string]bool, jaccardMin, cosineMin float64) string {
	// --- Primary: Jaccard member overlap (stable under centroid drift). ---
	if jaccardMin > 0 && len(clusterMembers) > 0 {
		inter := make(map[int]int)
		for _, m := range clusterMembers {
			for _, ri := range idx[m] {
				inter[ri]++
			}
		}
		best, bestJac := "", jaccardMin
		for ri, in := range inter {
			r := refs[ri]
			if claimed[r.NodeID] {
				continue
			}
			union := len(clusterMembers) + len(r.Members) - in
			if union <= 0 {
				continue
			}
			if jac := float64(in) / float64(union); jac >= bestJac {
				best, bestJac = r.NodeID, jac
			}
		}
		if best != "" {
			return best
		}
	}

	// --- Fallback: centroid cosine (membership fully turned over but the
	// pattern is semantically the same). ---
	if cosineMin > 0 && len(centroid) > 0 {
		best, bestSim := "", cosineMin
		for _, r := range refs {
			if claimed[r.NodeID] || len(r.Centroid) == 0 {
				continue
			}
			if sim := cosineSimilarity(centroid, r.Centroid); sim >= bestSim {
				best, bestSim = r.NodeID, sim
			}
		}
		return best
	}
	return ""
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
func (s *Service) deleteUnmatchedHiddenPatterns(ctx context.Context, spaceID string, claimed map[string]bool, existing []hiddenPatternRef) (int, error) {
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
