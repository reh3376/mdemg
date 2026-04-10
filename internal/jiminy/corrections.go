package jiminy

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// findRelevantCorrections performs vector search for obs_type='correction' nodes
// that are semantically similar to the given context.
func (s *Service) findRelevantCorrections(ctx context.Context, spaceID string, embedding []float32, limit int) ([]correctionMatch, error) {
	if limit <= 0 {
		limit = 5
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use HNSW vector index for O(log N) recall, then post-filter by obs_type
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
	  AND node.obs_type = 'correction'
	  AND NOT coalesce(node.is_archived, false)
	  AND score > 0.4
	RETURN node.node_id AS node_id, node.content AS content, node.summary AS summary,
	       score AS sim, node.created_at AS created_at
	ORDER BY sim DESC LIMIT $limit`

	var matches []correctionMatch

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":    spaceID,
			"embedding":  embedding,
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
			content, _ := rec.Get("content")
			summary, _ := rec.Get("summary")
			sim, _ := rec.Get("sim")
			createdAtRaw, _ := rec.Get("created_at")

			createdAt := asTime(createdAtRaw)
			similarity := asFloat64(sim)

			// Apply temporal decay: recent corrections rank higher
			lambda := s.cfg.JiminyCorrectionDecayRate
			if lambda <= 0 {
				lambda = 0.01
			}
			if !createdAt.IsZero() {
				daysSince := time.Since(createdAt).Hours() / 24.0
				similarity *= math.Exp(-lambda * daysSince)
			}

			matches = append(matches, correctionMatch{
				NodeID:     asString(nodeID),
				Content:    asString(content),
				Summary:    asString(summary),
				Similarity: similarity,
				CreatedAt:  createdAt,
			})
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("correction vector search: %w", err)
	}

	slog.Info("jiminy: found relevant corrections", "count", len(matches), "space_id", spaceID)
	return matches, nil
}

// asTime safely converts an interface{} to time.Time.
// Handles Neo4j time types and RFC3339 strings.
func asTime(v any) time.Time {
	if v == nil {
		return time.Time{}
	}
	switch t := v.(type) {
	case time.Time:
		return t
	case string:
		if parsed, err := time.Parse(time.RFC3339, t); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// asString safely converts an interface{} to string.
func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// asFloat64 safely converts an interface{} to float64.
func asFloat64(v any) float64 {
	if v == nil {
		return 0.0
	}
	switch f := v.(type) {
	case float64:
		return f
	case float32:
		return float64(f)
	case int64:
		return float64(f)
	case int:
		return float64(f)
	default:
		return 0.0
	}
}
