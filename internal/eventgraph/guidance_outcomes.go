// Sprint EVENTGRAPH-002 — Pattern Y1 federation, second event class.
//
// GuidanceOutcomesInNeighborhood orchestrates the same two-step federation as
// EventsInGraphNeighborhood, but for guidance-outcome events:
//   1. Cypher graph traversal from a seed node, capturing the N-hop
//      neighborhood AND each neighbor's constraint_code (the join key).
//   2. TSDB query against constraint_outcomes for outcomes whose
//      constraint_code is present in the neighborhood, within a lookback window.
//   3. Go-side join: annotate each outcome with the Neo4j constraint node its
//      code resolves to.
//
// Why constraint_code (not node_id): TSDB constraint_outcomes.constraint_id is a
// UUID that does NOT match the Neo4j node_id (CUID). The only reliable key
// linking the graph constraint nodes to the outcome rows is constraint_code,
// which both carry (Neo4j as a node property, TSDB as a column). The V0023
// index (idx_constraint_outcomes_code) backs the join.
//
// The outcome stream already lives in constraint_outcomes (written by the Jiminy
// /v1/jiminy/feedback path, RRF-SCALE-001 + JIMINY-OUTCOME-001) — EVENTGRAPH-002
// federates it, it does NOT create a new sink.
package eventgraph

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// GuidanceOutcomeRequest is the validated input to GuidanceOutcomesInNeighborhood.
type GuidanceOutcomeRequest struct {
	SpaceID    string        // required
	SeedNodeID string        // required
	Hops       int           // graph traversal depth; 0 = seed-only
	Since      time.Duration // lookback window; clamped to ≥ 1s by the helper
	Limit      int           // max rows returned; 0 → server default
}

// GuidanceOutcomeWithContext is one constraint_outcomes row enriched with
// graph-walk context (which constraint node in the neighborhood its
// constraint_code resolves to).
type GuidanceOutcomeWithContext struct {
	Time             time.Time `json:"time"`
	ConstraintID     string    `json:"constraint_id"`      // TSDB source node UUID (NOT a Neo4j node_id)
	ConstraintCode   string    `json:"constraint_code"`    // the join key
	GuidanceID       string    `json:"guidance_id"`
	SessionID        string    `json:"session_id,omitempty"`
	OutcomeType      string    `json:"outcome_type"`       // followed | ignored | contradicted | partial_compliance | not_applicable
	Similarity       float64   `json:"similarity"`
	GuidanceType     string    `json:"guidance_type,omitempty"`
	ConstraintNodeID string    `json:"constraint_node_id"` // Neo4j node_id whose constraint_code matched (in-neighborhood)
	InNeighborhood   bool      `json:"in_neighborhood"`    // always true (matched a neighborhood code); kept for parity + future widening
}

// GuidanceOutcomeResult is the federation API response payload.
type GuidanceOutcomeResult struct {
	Outcomes                []GuidanceOutcomeWithContext `json:"outcomes"`
	NeighborNodeIDs         []string                     `json:"neighbor_node_ids"`
	NeighborConstraintCodes []string                     `json:"neighbor_constraint_codes"`
	GraphHops               int                          `json:"graph_hops"`
	TSDBRowsScanned         int                          `json:"tsdb_rows_scanned"`
	Truncated               bool                         `json:"truncated"`
}

// GuidanceOutcomesInNeighborhood runs the federated query. Returns an empty
// result (not an error) when the seed has no neighborhood, the neighborhood
// carries no constraint codes, or no outcomes match within the window.
func (s *Service) GuidanceOutcomesInNeighborhood(ctx context.Context, req GuidanceOutcomeRequest) (*GuidanceOutcomeResult, error) {
	if req.SpaceID == "" {
		return nil, fmt.Errorf("space_id is required")
	}
	if req.SeedNodeID == "" {
		return nil, fmt.Errorf("seed_node_id is required")
	}
	if req.Hops < 0 {
		return nil, fmt.Errorf("hops must be ≥ 0, got %d", req.Hops)
	}
	if req.Since < time.Second {
		req.Since = time.Second
	}
	if req.Limit <= 0 {
		req.Limit = 500
	}

	// Step 1 — graph traversal: neighborhood node IDs + code→node map.
	neighbors, codeToNode, err := s.walkNeighborhoodWithCodes(ctx, FederationRequest{
		SpaceID: req.SpaceID, SeedNodeID: req.SeedNodeID, Hops: req.Hops,
	})
	if err != nil {
		return nil, fmt.Errorf("graph walk failed: %w", err)
	}
	if neighbors == nil {
		neighbors = []string{} // EVENTGRAPH-CLI-001 lesson: never serialize nil as null
	}
	codes := sortedKeys(codeToNode)

	result := &GuidanceOutcomeResult{
		Outcomes:                []GuidanceOutcomeWithContext{},
		NeighborNodeIDs:         neighbors,
		NeighborConstraintCodes: codes,
		GraphHops:               req.Hops,
	}

	// No constraint codes in the neighborhood → no outcomes to join.
	if len(codes) == 0 {
		return result, nil
	}

	// Step 2 — TSDB query for outcomes whose code is in the neighborhood.
	outcomes, err := s.queryGuidanceOutcomes(ctx, req, codes)
	if err != nil {
		return nil, fmt.Errorf("tsdb query failed: %w", err)
	}
	result.TSDBRowsScanned = len(outcomes)

	// Step 3 — Go-side join: resolve each outcome's code → neighborhood node.
	for i := range outcomes {
		outcomes[i].ConstraintNodeID = codeToNode[outcomes[i].ConstraintCode]
		outcomes[i].InNeighborhood = true
	}

	if req.Limit > 0 && len(outcomes) >= req.Limit {
		result.Truncated = true
	}
	result.Outcomes = outcomes
	return result, nil
}

// walkNeighborhoodWithCodes returns the seed plus all nodes reachable within
// req.Hops via CO_ACTIVATED_WITH or GENERALIZES, AND a map from each
// constraint node's constraint_code → a representative node_id in the
// neighborhood. Hops=0 returns just the seed (if it exists).
//
// Same parameterized-variable-length-path approach as walkNeighborhood (the
// hop count is a validated non-negative integer interpolated into the literal
// hop range Neo4j requires).
func (s *Service) walkNeighborhoodWithCodes(ctx context.Context, req FederationRequest) ([]string, map[string]string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := fmt.Sprintf(`
MATCH (seed:MemoryNode {space_id: $spaceId, node_id: $seed})
OPTIONAL MATCH (seed)-[:CO_ACTIVATED_WITH|GENERALIZES*0..%d]-(m:MemoryNode {space_id: $spaceId})
WITH collect(DISTINCT coalesce(m, seed)) AS nodes
RETURN [n IN nodes | n.node_id] AS ids,
       [n IN nodes WHERE n.constraint_code IS NOT NULL AND n.constraint_code <> '' | [n.constraint_code, n.node_id]] AS codePairs
`, req.Hops)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": req.SpaceID,
			"seed":    req.SeedNodeID,
		})
		if err != nil {
			return nil, err
		}
		ids := []string{}
		codeToNode := map[string]string{}
		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("ids"); ok && v != nil {
				if arr, ok := v.([]any); ok {
					for _, x := range arr {
						if s, ok := x.(string); ok && s != "" {
							ids = append(ids, s)
						}
					}
				}
			}
			if v, ok := rec.Get("codePairs"); ok && v != nil {
				if arr, ok := v.([]any); ok {
					for _, p := range arr {
						pair, ok := p.([]any)
						if !ok || len(pair) != 2 {
							continue
						}
						code, _ := pair[0].(string)
						node, _ := pair[1].(string)
						// First node wins per code (codes can repeat across
						// duplicate constraint nodes — semantics: "outcomes for
						// codes present in the neighborhood").
						if code != "" {
							if _, seen := codeToNode[code]; !seen {
								codeToNode[code] = node
							}
						}
					}
				}
			}
		}
		return []any{ids, codeToNode}, res.Err()
	})
	if err != nil {
		return nil, nil, err
	}
	tuple, _ := result.([]any)
	ids, _ := tuple[0].([]string)
	codeToNode, _ := tuple[1].(map[string]string)
	return ids, codeToNode, nil
}

// queryGuidanceOutcomes pulls constraint_outcomes whose constraint_code is in
// the neighborhood code set, within the lookback window, newest-first. Backed
// by the V0023 idx_constraint_outcomes_code index.
func (s *Service) queryGuidanceOutcomes(ctx context.Context, req GuidanceOutcomeRequest, codes []string) ([]GuidanceOutcomeWithContext, error) {
	const sql = `
SELECT time, constraint_id, coalesce(constraint_code, '') AS constraint_code,
       guidance_id, coalesce(session_id, '') AS session_id,
       outcome_type, coalesce(similarity, 0.0) AS similarity,
       coalesce(guidance_type, '') AS guidance_type
FROM constraint_outcomes
WHERE space_id = $1
  AND constraint_code = ANY($2)
  AND time > NOW() - $3::interval
ORDER BY time DESC
LIMIT $4
`
	rows, err := s.pool.Query(ctx, sql,
		req.SpaceID, codes, intervalString(req.Since), req.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var outcomes []GuidanceOutcomeWithContext
	for rows.Next() {
		var o GuidanceOutcomeWithContext
		if err := rows.Scan(
			&o.Time, &o.ConstraintID, &o.ConstraintCode,
			&o.GuidanceID, &o.SessionID,
			&o.OutcomeType, &o.Similarity, &o.GuidanceType,
		); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, o)
	}
	return outcomes, rows.Err()
}

// sortedKeys returns the map keys in deterministic (sorted) order — so the
// neighbor_constraint_codes output and the ANY($2) query parameter are stable.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
