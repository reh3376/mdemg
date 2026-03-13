package guardrail

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// retrieveConstraints performs two-phase constraint retrieval:
// Phase A: Semantic (vector similarity) search against constraint embeddings
// Phase B: Keyword matching against constraint content
// Results are deduplicated and capped at MaxConstraints.
func (g *GuardrailService) retrieveConstraints(ctx context.Context, spaceID string, diffCtx DiffContext) ([]constraintMatch, error) {
	seen := make(map[string]bool)
	var results []constraintMatch

	maxConstraints := g.cfg.MaxConstraints
	if maxConstraints <= 0 {
		maxConstraints = 10
	}

	// Phase A: Semantic search (requires embedder and non-empty summary)
	if g.embedder != nil && diffCtx.Summary != "" {
		semantic, err := g.semanticSearch(ctx, spaceID, diffCtx.Summary)
		if err != nil {
			log.Printf("guardrail: semantic search failed (continuing with keyword): %v", err)
		} else {
			for _, c := range semantic {
				if !seen[c.NodeID] {
					seen[c.NodeID] = true
					results = append(results, c)
				}
			}
		}
	}

	// Phase B: Keyword matching
	keywords := collectKeywords(diffCtx)
	if len(keywords) > 0 {
		kwResults, err := g.keywordSearch(ctx, spaceID, keywords)
		if err != nil {
			log.Printf("guardrail: keyword search failed: %v", err)
		} else {
			for _, c := range kwResults {
				if !seen[c.NodeID] {
					seen[c.NodeID] = true
					results = append(results, c)
				}
			}
		}
	}

	// Cap results
	if len(results) > maxConstraints {
		results = results[:maxConstraints]
	}

	return results, nil
}

// semanticSearch embeds the diff summary and finds constraints by vector similarity.
func (g *GuardrailService) semanticSearch(ctx context.Context, spaceID, summary string) ([]constraintMatch, error) {
	embedding, err := g.embedder.Embed(ctx, summary)
	if err != nil {
		return nil, fmt.Errorf("embed diff summary: %w", err)
	}

	sess := g.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use HNSW vector index for O(log N) recall, then post-filter by role_type
	indexName := g.cfg.VectorIndexName
	if indexName == "" {
		indexName = "memNodeEmbedding"
	}

	cypher := `
	CALL db.index.vector.queryNodes($indexName, 200, $embedding)
	YIELD node AS c, score AS sim
	WHERE c.space_id = $spaceId
	  AND c.role_type = 'constraint'
	  AND NOT coalesce(c.is_archived, false)
	  AND sim > 0.3
	RETURN c.node_id AS node_id, c.name AS name, c.constraint_type AS constraint_type,
	       c.content AS content, c.confidence AS confidence, sim
	ORDER BY sim DESC LIMIT 10`

	var matches []constraintMatch

	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"embedding": embedding,
			"indexName": indexName,
		})
		if err != nil {
			return nil, err
		}

		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			cType, _ := rec.Get("constraint_type")
			content, _ := rec.Get("content")
			confidence, _ := rec.Get("confidence")
			sim, _ := rec.Get("sim")

			matches = append(matches, constraintMatch{
				NodeID:         asString(nodeID),
				Name:           asString(name),
				ConstraintType: asString(cType),
				Content:        asString(content),
				Confidence:     asFloat64(confidence),
				Similarity:     asFloat64(sim),
			})
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("semantic constraint search: %w", err)
	}

	return matches, nil
}

// keywordSearch finds constraints whose content contains any of the given keywords.
func (g *GuardrailService) keywordSearch(ctx context.Context, spaceID string, keywords []string) ([]constraintMatch, error) {
	if len(keywords) == 0 {
		return nil, nil
	}

	sess := g.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build keyword match conditions
	// Each keyword is checked via toLower(c.content) CONTAINS kw
	cypher := `
	MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'constraint'})
	WHERE NOT coalesce(c.is_archived, false)
	  AND ANY(kw IN $keywords WHERE toLower(c.content) CONTAINS kw)
	RETURN c.node_id AS node_id, c.name AS name, c.constraint_type AS constraint_type,
	       c.content AS content, c.confidence AS confidence
	ORDER BY c.confidence DESC LIMIT 10`

	var matches []constraintMatch

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":  spaceID,
			"keywords": keywords,
		})
		if err != nil {
			return nil, err
		}

		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			cType, _ := rec.Get("constraint_type")
			content, _ := rec.Get("content")
			confidence, _ := rec.Get("confidence")

			matches = append(matches, constraintMatch{
				NodeID:         asString(nodeID),
				Name:           asString(name),
				ConstraintType: asString(cType),
				Content:        asString(content),
				Confidence:     asFloat64(confidence),
			})
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("keyword constraint search: %w", err)
	}

	return matches, nil
}

// collectKeywords builds a deduplicated, lowercased keyword list from the diff context.
func collectKeywords(ctx DiffContext) []string {
	seen := make(map[string]bool)
	var keywords []string

	addKW := func(s string) {
		lower := strings.ToLower(strings.TrimSpace(s))
		if lower != "" && len(lower) >= 3 && !seen[lower] {
			seen[lower] = true
			keywords = append(keywords, lower)
		}
	}

	for _, f := range ctx.FunctionNames {
		addKW(f)
	}
	for _, p := range ctx.ImportPaths {
		addKW(p)
	}
	for _, k := range ctx.Keywords {
		addKW(k)
	}
	// Include file basenames
	for _, fp := range ctx.FilePaths {
		parts := strings.Split(fp, "/")
		if len(parts) > 0 {
			addKW(parts[len(parts)-1])
		}
	}

	return keywords
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
