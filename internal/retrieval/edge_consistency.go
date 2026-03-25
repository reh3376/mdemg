package retrieval

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// EdgeStalenessConfig configures edge staleness cascade behavior.
type EdgeStalenessConfig struct {
	// Enabled controls whether edge staleness cascade is active.
	Enabled bool

	// RefreshBatchSize limits how many edges to refresh per call.
	RefreshBatchSize int

	// ReclusterThreshold is the centroid drift threshold for triggering re-clustering.
	ReclusterThreshold float64
}

// DefaultEdgeStalenessConfig returns sensible defaults for edge staleness.
func DefaultEdgeStalenessConfig() EdgeStalenessConfig {
	return EdgeStalenessConfig{
		Enabled:            true,
		RefreshBatchSize:   100,
		ReclusterThreshold: 0.3,
	}
}

// PropagateEdgeStalenessResult contains statistics from a staleness propagation.
type PropagateEdgeStalenessResult struct {
	// NodesMarked is the number of nodes marked with edges_stale=true.
	NodesMarked int

	// CoactivationEdgesMarked is the number of CO_ACTIVATED_WITH edges marked stale.
	CoactivationEdgesMarked int

	// AssociatedEdgesMarked is the number of ASSOCIATED_WITH edges marked stale.
	AssociatedEdgesMarked int

	// HiddenNodesAffected is the number of hidden (L1+) nodes affected by member changes.
	HiddenNodesAffected int
}

// PropagateEdgeStaleness marks connected edges as stale when a node's embedding changes.
// This extends the existing edges_stale=true flag on nodes to also mark CO_ACTIVATED_WITH
// and ASSOCIATED_WITH edges as stale.
//
// Cascade flow:
//  1. L0 node embedding changes → node.edges_stale=true (done by IngestObservation)
//  2. This function propagates: mark connected CO_ACTIVATED_WITH edges as stale
//  3. Mark connected ASSOCIATED_WITH edges as stale
//  4. Mark parent hidden nodes (L1+) for potential re-clustering
func (s *Service) PropagateEdgeStaleness(ctx context.Context, spaceID, nodeID string) (PropagateEdgeStalenessResult, error) {
	result := PropagateEdgeStalenessResult{}

	// Check if edge staleness cascade is enabled
	if !s.cfg.EdgeStalenessCascadeEnabled {
		return result, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Mark CO_ACTIVATED_WITH edges as stale
	coactRes, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
WHERE n.edges_stale = true
WITH n
OPTIONAL MATCH (n)-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]-()
WHERE r IS NOT NULL
SET r.stale = true, r.staleness_reason = 'content_changed', r.staleness_source = $nodeId
RETURN count(r) AS edges_marked`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("edges_marked")
			return toInt(count, 0), nil
		}
		return 0, res.Err()
	})
	if err != nil {
		return result, fmt.Errorf("mark CO_ACTIVATED_WITH edges stale: %w", err)
	}
	result.CoactivationEdgesMarked = coactRes.(int)

	// Mark ASSOCIATED_WITH edges as stale
	assocRes, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
WHERE n.edges_stale = true
WITH n
OPTIONAL MATCH (n)-[r:ASSOCIATED_WITH {space_id: $spaceId}]-()
WHERE r IS NOT NULL
SET r.stale = true, r.staleness_reason = 'content_changed', r.staleness_source = $nodeId
RETURN count(r) AS edges_marked`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("edges_marked")
			return toInt(count, 0), nil
		}
		return 0, res.Err()
	})
	if err != nil {
		return result, fmt.Errorf("mark ASSOCIATED_WITH edges stale: %w", err)
	}
	result.AssociatedEdgesMarked = assocRes.(int)

	// Mark parent hidden nodes (L1+) as needing potential re-clustering
	hiddenRes, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
WHERE n.edges_stale = true
WITH n
OPTIONAL MATCH (n)-[:ABSTRACTS_TO {space_id: $spaceId}]->(h:MemoryNode)
WHERE h.layer >= 1
SET h.member_changed = true, h.member_changed_at = datetime()
RETURN count(DISTINCT h) AS hidden_affected`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("hidden_affected")
			return toInt(count, 0), nil
		}
		return 0, res.Err()
	})
	if err != nil {
		return result, fmt.Errorf("mark hidden nodes affected: %w", err)
	}
	result.HiddenNodesAffected = hiddenRes.(int)

	// Log if significant propagation occurred
	if result.CoactivationEdgesMarked > 0 || result.AssociatedEdgesMarked > 0 || result.HiddenNodesAffected > 0 {
		slog.Info("edge_staleness: propagated", "node_id", nodeID, "coactivation_edges", result.CoactivationEdgesMarked, "associated_edges", result.AssociatedEdgesMarked, "hidden_nodes", result.HiddenNodesAffected)
	}

	return result, nil
}

// RefreshStaleCoactivationEdgesResult contains statistics from refreshing stale edges.
type RefreshStaleCoactivationEdgesResult struct {
	// EdgesRefreshed is the number of edges that were recalculated.
	EdgesRefreshed int

	// EdgesRemoved is the number of edges removed (e.g., similarity dropped below threshold).
	EdgesRemoved int

	// Errors is the number of edges that failed to refresh.
	Errors int
}

// RefreshStaleCoactivationEdges recalculates the dim_semantic property on stale
// CO_ACTIVATED_WITH edges based on current node embeddings.
// Processes up to cfg.EdgeStalenessRefreshBatchSize edges per call.
func (s *Service) RefreshStaleCoactivationEdges(ctx context.Context, spaceID string) (RefreshStaleCoactivationEdgesResult, error) {
	result := RefreshStaleCoactivationEdgesResult{}

	batchSize := s.cfg.EdgeStalenessRefreshBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Find and refresh stale CO_ACTIVATED_WITH edges
	// Uses vector distance to recalculate semantic similarity
	refreshRes, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH {stale: true}]->(b:MemoryNode {space_id: $spaceId})
WHERE a.embedding IS NOT NULL AND b.embedding IS NOT NULL
WITH a, b, r, 1.0 - (vector.similarity.cosine(a.embedding, b.embedding) / 2.0) AS newSemantic
SET r.dim_semantic = newSemantic,
    r.stale = false,
    r.refreshed_at = datetime(),
    r.staleness_reason = null,
    r.staleness_source = null
RETURN count(r) AS refreshed
LIMIT $batchSize`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"batchSize": batchSize,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("refreshed")
			return toInt(count, 0), nil
		}
		return 0, res.Err()
	})
	if err != nil {
		return result, fmt.Errorf("refresh stale CO_ACTIVATED_WITH edges: %w", err)
	}
	result.EdgesRefreshed = refreshRes.(int)

	// Clear stale flag on edges where embedding is missing (can't refresh)
	_, clearErr := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH {stale: true}]->(b:MemoryNode {space_id: $spaceId})
WHERE a.embedding IS NULL OR b.embedding IS NULL
SET r.stale = false, r.staleness_reason = 'no_embedding'
RETURN count(r) AS cleared
LIMIT $batchSize`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"batchSize": batchSize,
		})
		if err != nil {
			return 0, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})
	if clearErr != nil {
		slog.Warn("failed to clear stale flag on edges without embeddings", "error", clearErr)
	}

	if result.EdgesRefreshed > 0 {
		slog.Info("edge_consistency: refreshed stale CO_ACTIVATED_WITH edges", "edges_refreshed", result.EdgesRefreshed, "space_id", spaceID)
	}

	return result, nil
}

// RefreshStaleAssociatedWithEdges recalculates the similarity property on stale
// ASSOCIATED_WITH edges based on current node embeddings.
func (s *Service) RefreshStaleAssociatedWithEdges(ctx context.Context, spaceID string) (int, error) {
	batchSize := s.cfg.EdgeStalenessRefreshBatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	refreshRes, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (a:MemoryNode {space_id: $spaceId})-[r:ASSOCIATED_WITH {stale: true}]->(b:MemoryNode {space_id: $spaceId})
WHERE a.embedding IS NOT NULL AND b.embedding IS NOT NULL
WITH a, b, r, vector.similarity.cosine(a.embedding, b.embedding) AS newSimilarity
SET r.similarity = newSimilarity,
    r.weight = newSimilarity,
    r.stale = false,
    r.refreshed_at = datetime(),
    r.staleness_reason = null,
    r.staleness_source = null
RETURN count(r) AS refreshed
LIMIT $batchSize`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"batchSize": batchSize,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("refreshed")
			return toInt(count, 0), nil
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("refresh stale ASSOCIATED_WITH edges: %w", err)
	}

	refreshed := refreshRes.(int)
	if refreshed > 0 {
		slog.Info("edge_consistency: refreshed stale ASSOCIATED_WITH edges", "edges_refreshed", refreshed, "space_id", spaceID)
	}

	return refreshed, nil
}

// GetStaleEdgeStats returns counts of stale edges by type for monitoring.
type StaleEdgeStats struct {
	CoactivationStale int `json:"coactivation_stale"`
	AssociatedStale   int `json:"associated_stale"`
	NodesWithStale    int `json:"nodes_with_stale_edges"`
	HiddenWithChanges int `json:"hidden_with_member_changes"`
}

// GetStaleEdgeStats returns statistics about stale edges in a space.
func (s *Service) GetStaleEdgeStats(ctx context.Context, spaceID string) (StaleEdgeStats, error) {
	var stats StaleEdgeStats

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
OPTIONAL MATCH (n)-[coact:CO_ACTIVATED_WITH {stale: true}]-()
OPTIONAL MATCH (n)-[assoc:ASSOCIATED_WITH {stale: true}]-()
WITH
  count(DISTINCT CASE WHEN n.edges_stale = true THEN n END) AS nodesStale,
  count(DISTINCT CASE WHEN n.layer >= 1 AND n.member_changed = true THEN n END) AS hiddenChanged,
  count(DISTINCT coact) AS coactStale,
  count(DISTINCT assoc) AS assocStale
RETURN nodesStale, hiddenChanged, coactStale, assocStale`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return StaleEdgeStats{}, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			ns, _ := rec.Get("nodesStale")
			hc, _ := rec.Get("hiddenChanged")
			cs, _ := rec.Get("coactStale")
			as, _ := rec.Get("assocStale")
			return StaleEdgeStats{
				NodesWithStale:    toInt(ns, 0),
				HiddenWithChanges: toInt(hc, 0),
				CoactivationStale: toInt(cs, 0),
				AssociatedStale:   toInt(as, 0),
			}, nil
		}
		return StaleEdgeStats{}, res.Err()
	})
	if err != nil {
		return stats, err
	}

	return result.(StaleEdgeStats), nil
}

// RefreshAllStaleEdges is a convenience method that refreshes all types of stale edges.
// It calls the existing RefreshStaleEdges (for ASSOCIATED_WITH via embedding similarity)
// and RefreshStaleCoactivationEdges (for CO_ACTIVATED_WITH).
func (s *Service) RefreshAllStaleEdges(ctx context.Context, spaceID string) (int, error) {
	total := 0

	// Refresh ASSOCIATED_WITH edges (existing method)
	assocRefreshed, err := s.RefreshStaleEdges(ctx, spaceID)
	if err != nil {
		return total, fmt.Errorf("refresh ASSOCIATED_WITH: %w", err)
	}
	total += assocRefreshed

	// Refresh CO_ACTIVATED_WITH edges
	coactResult, err := s.RefreshStaleCoactivationEdges(ctx, spaceID)
	if err != nil {
		return total, fmt.Errorf("refresh CO_ACTIVATED_WITH: %w", err)
	}
	total += coactResult.EdgesRefreshed

	// Also refresh stale ASSOCIATED_WITH edges marked by propagation
	assocStaleRefreshed, err := s.RefreshStaleAssociatedWithEdges(ctx, spaceID)
	if err != nil {
		return total, fmt.Errorf("refresh stale ASSOCIATED_WITH: %w", err)
	}
	total += assocStaleRefreshed

	// Clear edges_stale flag on nodes after refresh
	if total > 0 {
		if err := s.clearEdgesStaleFlag(ctx, spaceID); err != nil {
			slog.Warn("failed to clear edges_stale flag", "error", err)
		}
		// Invalidate cache after edge refresh
		s.invalidateSpaceCacheForEdgeConsistency(spaceID)
	}

	return total, nil
}

// clearEdgesStaleFlag clears the edges_stale flag on nodes after edges have been refreshed.
func (s *Service) clearEdgesStaleFlag(ctx context.Context, spaceID string) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.edges_stale = true
// Only clear if no stale edges remain
AND NOT EXISTS {
  (n)-[r:CO_ACTIVATED_WITH {stale: true}]-()
}
AND NOT EXISTS {
  (n)-[r:ASSOCIATED_WITH {stale: true}]-()
}
REMOVE n.edges_stale
RETURN count(n) AS cleared`
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})
	return err
}

// invalidateSpaceCacheForEdgeConsistency invalidates the query cache after edge changes.
// Uses the existing InvalidateSpaceCache method from service.go.
func (s *Service) invalidateSpaceCacheForEdgeConsistency(spaceID string) {
	invalidated := s.InvalidateSpaceCache(spaceID)
	if invalidated > 0 {
		slog.Info("cache: invalidated entries after edge consistency update", "entries", invalidated, "space_id", spaceID)
	}
}
