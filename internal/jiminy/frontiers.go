package jiminy

import (
	"context"
	"fmt"
	"log"

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

	cypher := `
	MATCH (n:MemoryNode {space_id: $spaceId})
	WHERE n.is_frontier = true AND n.embedding IS NOT NULL
	  AND NOT coalesce(n.is_archived, false)
	WITH n, vector.similarity.cosine(n.embedding, $embedding) AS sim
	WHERE sim > $minSim
	RETURN n.node_id AS node_id, n.name AS name, n.summary AS summary, sim
	ORDER BY sim DESC LIMIT $limit`

	var matches []frontierMatch

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"embedding": embedding,
			"minSim":    minSim,
			"limit":     int64(limit),
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

	log.Printf("jiminy: found %d relevant frontiers (space=%s)", len(matches), spaceID)
	return matches, nil
}
