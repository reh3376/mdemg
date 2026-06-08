// Sprint EVENTGRAPH-001 Epic 5 — Pattern Y1 federation helper.
//
// EventsInGraphNeighborhood orchestrates a two-step query:
//   1. Cypher graph traversal from a seed node, capturing the N-hop
//      neighborhood (node IDs + layer).
//   2. TSDB query against reinforcement_events for events touching ANY
//      node in the neighborhood, within a lookback window.
//   3. Go-side join: annotate events with InNeighborhood and prune any
//      events that don't share at least one endpoint with the neighborhood
//      (defensive — should not happen given the WHERE clause).
//
// This is the cheapest path to "graph-walk + event context" without
// reifying events as graph nodes (Pattern Y2, deferred). When operators
// hit query shapes that require single-pass Cypher across events, that's
// the signal to escalate.
package eventgraph

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// FederationRequest is the validated input to EventsInGraphNeighborhood.
type FederationRequest struct {
	SpaceID    string        // required
	SeedNodeID string        // required
	Hops       int           // graph traversal depth; 0 = seed-only
	Since      time.Duration // lookback window; clamped to ≥ 1s by the helper
	Limit      int           // max rows returned; 0 → server default
}

// EventWithContext is one reinforcement_events row enriched with graph-walk
// context (which endpoints fell inside the requested N-hop neighborhood).
type EventWithContext struct {
	EventID            string    `json:"event_id"`
	RecordedAt         time.Time `json:"recorded_at"`
	SrcNodeID          string    `json:"src_node_id"`
	DstNodeID          string    `json:"dst_node_id"`
	PrevWeight         float64   `json:"prev_weight"`
	NewWeight          float64   `json:"new_weight"`
	DeltaWeight        float64   `json:"delta_weight"`
	EvidenceCountAfter int       `json:"evidence_count_after"`
	Direction          string    `json:"direction,omitempty"`
	SessionID          string    `json:"session_id,omitempty"`
	CreatedNewEdge     bool      `json:"created_new_edge"`
	TriggerPath        string    `json:"trigger_path"`
	SrcInNeighborhood  bool      `json:"src_in_neighborhood"`
	DstInNeighborhood  bool      `json:"dst_in_neighborhood"`
}

// FederationResult is the federation API response payload.
type FederationResult struct {
	Events          []EventWithContext `json:"events"`
	NeighborNodeIDs []string           `json:"neighbor_node_ids"`
	GraphHops       int                `json:"graph_hops"`
	TSDBRowsScanned int                `json:"tsdb_rows_scanned"`
	Truncated       bool               `json:"truncated"`
}

// Service holds the two clients the federation needs. Owned by api/server.go;
// constructed once at boot, passed into the handler.
type Service struct {
	driver neo4j.DriverWithContext
	pool   *pgxpool.Pool
}

// NewService constructs the federation Service.
func NewService(driver neo4j.DriverWithContext, pool *pgxpool.Pool) *Service {
	return &Service{driver: driver, pool: pool}
}

// EventsInGraphNeighborhood runs the federated query. Returns an empty result
// (not an error) when the seed node has no neighborhood OR when no events
// touch the neighborhood within the lookback window.
func (s *Service) EventsInGraphNeighborhood(ctx context.Context, req FederationRequest) (*FederationResult, error) {
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

	// Step 1 — graph traversal.
	neighbors, err := s.walkNeighborhood(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("graph walk failed: %w", err)
	}
	// Guarantee a non-nil slice so the JSON contract is consistent with
	// Events: both array fields always serialize as [] (never null) when
	// empty — e.g. an unknown seed has no neighborhood. Caught in EVENTGRAPH-
	// CLI-001 live contract testing (UATS happy-path asserts type_is array).
	if neighbors == nil {
		neighbors = []string{}
	}

	result := &FederationResult{
		Events:          []EventWithContext{},
		NeighborNodeIDs: neighbors,
		GraphHops:       req.Hops,
	}

	// Empty neighborhood → no TSDB call needed.
	if len(neighbors) == 0 {
		return result, nil
	}

	// Step 2 — TSDB query for events touching the neighborhood.
	events, err := s.queryEvents(ctx, req, neighbors)
	if err != nil {
		return nil, fmt.Errorf("tsdb query failed: %w", err)
	}
	result.TSDBRowsScanned = len(events)

	// Step 3 — Go-side join: stamp InNeighborhood flags. Both endpoints can
	// be in the neighborhood (true,true) or just one (the other endpoint is
	// outside the seed's N-hop reach but the event still touches our subgraph).
	neighSet := make(map[string]struct{}, len(neighbors))
	for _, n := range neighbors {
		neighSet[n] = struct{}{}
	}
	for i := range events {
		_, srcOK := neighSet[events[i].SrcNodeID]
		_, dstOK := neighSet[events[i].DstNodeID]
		events[i].SrcInNeighborhood = srcOK
		events[i].DstInNeighborhood = dstOK
	}

	if req.Limit > 0 && len(events) >= req.Limit {
		result.Truncated = true
	}
	result.Events = events
	return result, nil
}

// walkNeighborhood returns the seed plus all nodes reachable within
// req.Hops via CO_ACTIVATED_WITH or GENERALIZES edges. Hops=0 returns
// just the seed (if it exists).
//
// Uses a parameterized variable-length path. Neo4j requires the hop
// range to be literal in Cypher, so we use string interpolation on a
// sanitized integer — defensible since req.Hops is already validated to
// be ≥ 0 and ≤ a server-side ceiling enforced by the handler.
func (s *Service) walkNeighborhood(ctx context.Context, req FederationRequest) ([]string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Hop range: 0..N inclusive. The seed itself is hop 0.
	cypher := fmt.Sprintf(`
MATCH (seed:MemoryNode {space_id: $spaceId, node_id: $seed})
OPTIONAL MATCH (seed)-[:CO_ACTIVATED_WITH|GENERALIZES*0..%d]-(n:MemoryNode {space_id: $spaceId})
WITH collect(DISTINCT coalesce(n.node_id, seed.node_id)) AS ids
RETURN ids
`, req.Hops)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": req.SpaceID,
			"seed":    req.SeedNodeID,
		})
		if err != nil {
			return nil, err
		}
		var out []string
		if res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("ids"); ok && v != nil {
				if arr, ok := v.([]any); ok {
					for _, x := range arr {
						if s, ok := x.(string); ok && s != "" {
							out = append(out, s)
						}
					}
				}
			}
		}
		return out, res.Err()
	})
	if err != nil {
		return nil, err
	}
	ids, _ := result.([]string)
	return ids, nil
}

// queryEvents pulls reinforcement_events where src OR dst is in the
// neighborhood, within the lookback window, ordered newest-first.
func (s *Service) queryEvents(ctx context.Context, req FederationRequest, neighbors []string) ([]EventWithContext, error) {
	const sql = `
SELECT event_id, recorded_at, src_node_id, dst_node_id,
       prev_weight, new_weight, delta_weight, evidence_count_after,
       coalesce(direction, '') AS direction,
       coalesce(session_id, '') AS session_id,
       created_new_edge, trigger_path
FROM reinforcement_events
WHERE space_id = $1
  AND (src_node_id = ANY($2) OR dst_node_id = ANY($2))
  AND recorded_at > NOW() - $3::interval
ORDER BY recorded_at DESC
LIMIT $4
`
	rows, err := s.pool.Query(ctx, sql,
		req.SpaceID, neighbors, intervalString(req.Since), req.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []EventWithContext
	for rows.Next() {
		var ev EventWithContext
		if err := rows.Scan(
			&ev.EventID, &ev.RecordedAt, &ev.SrcNodeID, &ev.DstNodeID,
			&ev.PrevWeight, &ev.NewWeight, &ev.DeltaWeight, &ev.EvidenceCountAfter,
			&ev.Direction, &ev.SessionID,
			&ev.CreatedNewEdge, &ev.TriggerPath,
		); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// intervalString formats a time.Duration as a PostgreSQL interval literal.
// PostgreSQL accepts "N seconds" / "N hours" — we use seconds for precision.
func intervalString(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}
