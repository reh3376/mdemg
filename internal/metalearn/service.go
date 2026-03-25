package metalearn

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/embeddings"
	"mdemg/internal/models"
)

// Service implements cross-space concept promotion (Phase 105).
type Service struct {
	driver        neo4j.DriverWithContext
	embedder      embeddings.Embedder
	generalizer   *Generalizer
	cbRegistry    *circuitbreaker.Registry
	globalSpaceID string
}

// NewService creates a new MetaLearn service.
func NewService(
	driver neo4j.DriverWithContext,
	emb embeddings.Embedder,
	genCfg GeneralizerConfig,
	cbRegistry *circuitbreaker.Registry,
	globalSpaceID string,
) *Service {
	return &Service{
		driver:        driver,
		embedder:      emb,
		generalizer:   NewGeneralizer(genCfg, cbRegistry),
		cbRegistry:    cbRegistry,
		globalSpaceID: globalSpaceID,
	}
}

// candidateNode represents a high-value node eligible for promotion.
type candidateNode struct {
	NodeID      string
	Name        string
	Summary     string
	Description string
	Layer       int
	UpdateCount int
}

// Promote finds high-value L4/L5 concepts in sourceSpaceID, generalizes them via LLM,
// and writes them to the global space with an ORIGINATED_FROM edge back to the original.
func (s *Service) Promote(ctx context.Context, req models.MetaLearnRequest) (models.MetaLearnResponse, error) {
	minLayer := req.MinLayer
	if minLayer <= 0 {
		minLayer = 4
	}
	minUpdateCount := req.MinUpdateCount
	if minUpdateCount <= 0 {
		minUpdateCount = 5
	}

	// 1. Find candidates
	candidates, err := s.findCandidates(ctx, req.SourceSpaceID, minLayer, minUpdateCount)
	if err != nil {
		return models.MetaLearnResponse{}, fmt.Errorf("find candidates: %w", err)
	}

	if len(candidates) == 0 {
		return models.MetaLearnResponse{
			Status:            "ok",
			ConceptsEvaluated: 0,
			ConceptsPromoted:  0,
			PromotedNodes:     []models.PromotedConcept{},
		}, nil
	}

	// 2. Generalize and promote each candidate
	promoted := make([]models.PromotedConcept, 0, len(candidates))
	for _, cand := range candidates {
		result, err := s.generalizer.Generalize(ctx, cand.Name, cand.Summary, cand.Description)
		if err != nil {
			slog.Warn("metalearn: generalization failed", "node_id", cand.NodeID, "error", err)
			continue
		}

		// 3. Embed the generalized text
		embText := result.Name + ": " + result.Description
		embedding, err := s.embedder.Embed(ctx, embText)
		if err != nil {
			slog.Warn("metalearn: embedding failed", "node_id", cand.NodeID, "error", err)
			continue
		}

		// 4. Write to global space + create ORIGINATED_FROM edge
		globalNodeID, err := s.writePromotedNode(ctx, cand, result, embedding)
		if err != nil {
			slog.Error("metalearn: write failed", "node_id", cand.NodeID, "error", err)
			continue
		}

		promoted = append(promoted, models.PromotedConcept{
			ID:            globalNodeID,
			OriginalID:    cand.NodeID,
			Name:          result.Name,
			GlobalSpaceID: s.globalSpaceID,
		})
	}

	return models.MetaLearnResponse{
		Status:            "ok",
		ConceptsEvaluated: len(candidates),
		ConceptsPromoted:  len(promoted),
		PromotedNodes:     promoted,
	}, nil
}

// findCandidates queries Neo4j for high-value L4/L5 nodes not yet promoted.
func (s *Service) findCandidates(ctx context.Context, sourceSpaceID string, minLayer, minUpdateCount int) ([]candidateNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `MATCH (n:MemoryNode {space_id: $sourceSpaceId})
WHERE n.layer >= $minLayer
  AND coalesce(n.update_count, 0) >= $minUpdateCount
  AND NOT coalesce(n.is_archived, false)
  AND NOT EXISTS {
    MATCH (g:MemoryNode {space_id: $globalSpaceId})-[:ORIGINATED_FROM]->(n)
  }
RETURN n.node_id AS node_id, n.name AS name, coalesce(n.summary, '') AS summary,
       n.layer AS layer, coalesce(n.update_count, 0) AS update_count,
       coalesce(n.description, '') AS description
ORDER BY n.layer DESC, update_count DESC
LIMIT 20`

	params := map[string]any{
		"sourceSpaceId":  sourceSpaceID,
		"globalSpaceId":  s.globalSpaceID,
		"minLayer":       minLayer,
		"minUpdateCount": minUpdateCount,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		var cands []candidateNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			layer, _ := rec.Get("layer")
			updateCount, _ := rec.Get("update_count")
			desc, _ := rec.Get("description")

			cands = append(cands, candidateNode{
				NodeID:      fmt.Sprint(nodeID),
				Name:        fmt.Sprint(name),
				Summary:     fmt.Sprint(summary),
				Description: fmt.Sprint(desc),
				Layer:       int(layer.(int64)),
				UpdateCount: int(updateCount.(int64)),
			})
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return cands, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]candidateNode), nil
}

// writePromotedNode creates a new MemoryNode in the global space and an ORIGINATED_FROM edge.
func (s *Service) writePromotedNode(ctx context.Context, cand candidateNode, result *GeneralizeResult, embedding []float32) (string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	globalNodeID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	cypher := `
CREATE (g:MemoryNode {
  node_id: $globalNodeId,
  space_id: $globalSpaceId,
  name: $name,
  summary: $description,
  description: $description,
  layer: $layer,
  role_type: $roleType,
  source: $source,
  embedding: $embedding,
  created_at: datetime($now),
  updated_at: datetime($now)
})
WITH g
MATCH (src:MemoryNode {node_id: $sourceNodeId})
WHERE src.space_id <> $globalSpaceId
CREATE (g)-[:ORIGINATED_FROM {
  created_at: datetime($now),
  weight: 1.0,
  evidence_count: 1
}]->(src)
RETURN g.node_id AS global_node_id`

	params := map[string]any{
		"globalNodeId":  globalNodeID,
		"globalSpaceId": s.globalSpaceID,
		"name":          result.Name,
		"description":   result.Description,
		"layer":         cand.Layer,
		"roleType":      "promoted_concept",
		"source":        "meta-learn",
		"embedding":     embedding,
		"now":           now,
		"sourceNodeId":  cand.NodeID,
	}

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("no result returned from promote write")
		}
		return nil, res.Err()
	})
	if err != nil {
		return "", err
	}
	return globalNodeID, nil
}
