package guardrail

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// scopeClause returns a Cypher fragment that filters constraint nodes by scope.
// When ConstraintScopeFilteringEnabled is false, the empty string is returned
// (no filtering applied).  When enabled, it injects an AND condition that
// allows a constraint to pass if it has no scope (applies everywhere) OR its
// scope regex matches the supplied file path.
//
// The caller is responsible for supplying a non-empty $filePath parameter when
// the returned string is non-empty.
func (g *GuardrailService) scopeClause() string {
	if !g.cfg.ConstraintScopeFilteringEnabled {
		return ""
	}
	return "\n  AND (c.scope IS NULL OR c.scope = '' OR $filePath =~ c.scope)"
}

// authorityClause returns a Cypher fragment that filters constraint nodes by authority level.
// When ConstraintAuthorityEnabled is false, or trustLevel is empty/unrecognised, returns "".
//
// Authority model (F20):
//   - "restricted" → sees ALL constraints (no additional filter)
//   - "standard"   → sees org_policy + team_standard only
//   - "elevated"   → sees org_policy only
//
// The caller is responsible for the $trustLevel parameter being available in params,
// but since the filter is inlined as literal Cypher (not a parameter) no extra param is needed.
func (g *GuardrailService) authorityClause(trustLevel string) string {
	if !g.cfg.ConstraintAuthorityEnabled {
		return ""
	}
	switch trustLevel {
	case "standard":
		return "\n  AND coalesce(c.authority_level, 'team_standard') IN ['org_policy', 'team_standard']"
	case "elevated":
		return "\n  AND coalesce(c.authority_level, 'team_standard') = 'org_policy'"
	default:
		// "restricted" or unrecognised → no additional filter
		return ""
	}
}

// primaryFilePath returns the first file path from the diff context, or an
// empty string if none are present.  Used as the $filePath parameter for
// scope-based WHERE clauses.
func primaryFilePath(diffCtx DiffContext) string {
	if len(diffCtx.FilePaths) > 0 {
		return diffCtx.FilePaths[0]
	}
	return ""
}

// retrieveConstraints performs two-phase constraint retrieval:
// Phase A: Semantic (vector similarity) search against constraint embeddings
// Phase B: Keyword matching against constraint content
// Results are deduplicated and capped at MaxConstraints.
// trustLevel is used for F20 authority-level filtering ("restricted"/"standard"/"elevated").
func (g *GuardrailService) retrieveConstraints(ctx context.Context, spaceID string, diffCtx DiffContext, trustLevel string) ([]constraintMatch, error) {
	seen := make(map[string]bool)
	var results []constraintMatch

	maxConstraints := g.cfg.MaxConstraints
	if maxConstraints <= 0 {
		maxConstraints = 10
	}

	// Phase A: Semantic search (requires embedder and non-empty summary)
	if g.embedder != nil && diffCtx.Summary != "" {
		semantic, err := g.semanticSearch(ctx, spaceID, diffCtx.Summary, diffCtx, trustLevel)
		if err != nil {
			slog.Warn("guardrail: semantic search failed, continuing with keyword", "error", err)
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
		kwResults, err := g.keywordSearch(ctx, spaceID, keywords, diffCtx, trustLevel)
		if err != nil {
			slog.Warn("guardrail: keyword search failed", "error", err)
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
// diffCtx is used for optional scope filtering (F7).
// trustLevel is used for optional authority-level filtering (F20).
func (g *GuardrailService) semanticSearch(ctx context.Context, spaceID, summary string, diffCtx DiffContext, trustLevel string) ([]constraintMatch, error) {
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
	  AND sim > 0.3` + g.scopeClause() + g.authorityClause(trustLevel) + `
	RETURN c.node_id AS node_id, c.name AS name, c.constraint_type AS constraint_type,
	       c.content AS content, c.confidence AS confidence, sim
	ORDER BY sim DESC LIMIT 10`

	params := map[string]any{
		"spaceId":   spaceID,
		"embedding": embedding,
		"indexName": indexName,
	}
	if g.cfg.ConstraintScopeFilteringEnabled {
		params["filePath"] = primaryFilePath(diffCtx)
	}

	var matches []constraintMatch

	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
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
// diffCtx is used for optional scope filtering (F7).
// trustLevel is used for optional authority-level filtering (F20).
func (g *GuardrailService) keywordSearch(ctx context.Context, spaceID string, keywords []string, diffCtx DiffContext, trustLevel string) ([]constraintMatch, error) {
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
	  AND ANY(kw IN $keywords WHERE toLower(c.content) CONTAINS kw)` + g.scopeClause() + g.authorityClause(trustLevel) + `
	RETURN c.node_id AS node_id, c.name AS name, c.constraint_type AS constraint_type,
	       c.content AS content, c.confidence AS confidence
	ORDER BY c.confidence DESC LIMIT 10`

	params := map[string]any{
		"spaceId":  spaceID,
		"keywords": keywords,
	}
	if g.cfg.ConstraintScopeFilteringEnabled {
		params["filePath"] = primaryFilePath(diffCtx)
	}

	var matches []constraintMatch

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
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
