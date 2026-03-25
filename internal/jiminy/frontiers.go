package jiminy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// findRelevantFrontiers performs vector search for frontier nodes
// (is_frontier=true) that are semantically similar to the given context.
func (s *Service) findRelevantFrontiers(ctx context.Context, spaceID string, embedding []float32, minSim float64, limit int) ([]frontierMatch, error) {
	if !s.cfg.JiminyIncludeFrontiers {
		return nil, nil
	}

	if limit <= 0 {
		limit = 3
	}
	if minSim <= 0 {
		minSim = s.cfg.JiminyFrontierMinSim
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use HNSW vector index for O(log N) recall, then post-filter by is_frontier
	indexName := s.cfg.VectorIndexName
	if indexName == "" {
		indexName = "memNodeEmbedding"
	}
	candidateK := limit * 20
	if candidateK < 50 {
		candidateK = 50
	}

	cypher := `
	CALL db.index.vector.queryNodes($indexName, $candidateK, $embedding)
	YIELD node, score
	WHERE node.space_id = $spaceId
	  AND node.is_frontier = true
	  AND NOT coalesce(node.is_archived, false)
	  AND score > $minSim
	RETURN node.node_id AS node_id, node.name AS name, node.summary AS summary, score AS sim
	ORDER BY sim DESC LIMIT $limit`

	var matches []frontierMatch

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":    spaceID,
			"embedding":  embedding,
			"minSim":     minSim,
			"limit":      int64(limit),
			"indexName":  indexName,
			"candidateK": int64(candidateK),
		})
		if err != nil {
			return nil, err
		}

		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			sim, _ := rec.Get("sim")

			matches = append(matches, frontierMatch{
				NodeID:     asString(nodeID),
				Name:       asString(name),
				Summary:    asString(summary),
				Similarity: asFloat64(sim),
			})
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("frontier vector search: %w", err)
	}

	slog.Info("jiminy: found relevant frontiers", "count", len(matches), "space_id", spaceID)
	return matches, nil
}
