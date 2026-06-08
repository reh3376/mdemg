//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/eventgraph"
)

// TestEventGraph_GuidanceOutcomes_RoundTrip is the EVENTGRAPH-002 Pattern Y1
// happy path: build a small constraint graph in Neo4j (nodes carry
// constraint_code) + populate constraint_outcomes in TSDB, then call
// GuidanceOutcomesInNeighborhood and assert outcomes join back on
// constraint_code with the correct neighborhood node resolved.
func TestEventGraph_GuidanceOutcomes_RoundTrip(t *testing.T) {
	driver := SetupTestNeo4j(t)
	client := newTestClient(t)
	pool := client.Pool()
	ctx := context.Background()

	spaceID := "test-eventgraph-guidance-outcomes"
	if err := CleanupSpace(ctx, driver, spaceID); err != nil {
		t.Fatalf("cleanup neo4j: %v", err)
	}
	cleanupConstraintOutcomes(t, pool, spaceID)

	// Graph: seedC --[CO_ACTIVATED_WITH]-- relatedC  (both role_type=constraint
	// with a constraint_code); offC is disconnected (its code must NOT appear
	// in the seed's neighborhood, so its outcomes must NOT be returned).
	if err := seedConstraintGraph(ctx, driver, spaceID); err != nil {
		t.Fatalf("seed graph: %v", err)
	}
	t.Cleanup(func() {
		_ = CleanupSpace(context.Background(), driver, spaceID)
		cleanupConstraintOutcomes(t, pool, spaceID)
	})

	// Insert constraint_outcomes for all three codes.
	insertConstraintOutcome(t, pool, spaceID, "code-seed", "followed")
	insertConstraintOutcome(t, pool, spaceID, "code-seed", "ignored")
	insertConstraintOutcome(t, pool, spaceID, "code-related", "followed")
	insertConstraintOutcome(t, pool, spaceID, "code-off", "followed") // off-neighborhood

	svc := eventgraph.NewService(driver, pool)

	// Hops=1 from seedC: neighborhood codes = {code-seed, code-related}.
	// Outcomes for those two codes return (3 rows); code-off must NOT.
	res, err := svc.GuidanceOutcomesInNeighborhood(ctx, eventgraph.GuidanceOutcomeRequest{
		SpaceID:    spaceID,
		SeedNodeID: "seedC",
		Hops:       1,
		Since:      time.Hour,
		Limit:      100,
	})
	if err != nil {
		t.Fatalf("federation hops=1: %v", err)
	}
	if len(res.NeighborConstraintCodes) != 2 {
		t.Errorf("expected 2 neighborhood codes {code-seed, code-related}, got %v", res.NeighborConstraintCodes)
	}
	if len(res.Outcomes) != 3 {
		t.Errorf("expected 3 outcomes (2 code-seed + 1 code-related), got %d", len(res.Outcomes))
	}
	for _, o := range res.Outcomes {
		if o.ConstraintCode == "code-off" {
			t.Errorf("code-off is outside the neighborhood and must not be returned")
		}
		if o.ConstraintNodeID == "" || !o.InNeighborhood {
			t.Errorf("outcome %q should resolve to a neighborhood node, got node=%q in=%v",
				o.ConstraintCode, o.ConstraintNodeID, o.InNeighborhood)
		}
		if o.ConstraintCode == "code-seed" && o.ConstraintNodeID != "seedC" {
			t.Errorf("code-seed should resolve to seedC, got %q", o.ConstraintNodeID)
		}
		if o.ConstraintCode == "code-related" && o.ConstraintNodeID != "relatedC" {
			t.Errorf("code-related should resolve to relatedC, got %q", o.ConstraintNodeID)
		}
	}

	// Hops=0: neighborhood = {seedC} → only code-seed → 2 outcomes.
	res0, err := svc.GuidanceOutcomesInNeighborhood(ctx, eventgraph.GuidanceOutcomeRequest{
		SpaceID:    spaceID,
		SeedNodeID: "seedC",
		Hops:       0,
		Since:      time.Hour,
		Limit:      100,
	})
	if err != nil {
		t.Fatalf("federation hops=0: %v", err)
	}
	if len(res0.NeighborConstraintCodes) != 1 || res0.NeighborConstraintCodes[0] != "code-seed" {
		t.Errorf("hops=0 codes = %v, want [code-seed]", res0.NeighborConstraintCodes)
	}
	if len(res0.Outcomes) != 2 {
		t.Errorf("hops=0 should return 2 code-seed outcomes, got %d", len(res0.Outcomes))
	}

	// Unknown seed → empty, non-nil slices.
	resEmpty, err := svc.GuidanceOutcomesInNeighborhood(ctx, eventgraph.GuidanceOutcomeRequest{
		SpaceID: spaceID, SeedNodeID: "does-not-exist", Hops: 2, Since: time.Hour, Limit: 100,
	})
	if err != nil {
		t.Fatalf("federation unknown seed: %v", err)
	}
	if resEmpty.Outcomes == nil || resEmpty.NeighborNodeIDs == nil || resEmpty.NeighborConstraintCodes == nil {
		t.Error("unknown seed must yield non-nil empty slices (JSON contract)")
	}
	if len(resEmpty.Outcomes) != 0 {
		t.Errorf("unknown seed should yield 0 outcomes, got %d", len(resEmpty.Outcomes))
	}
}

// seedConstraintGraph creates two connected constraint nodes (with
// constraint_code) + one disconnected constraint node.
func seedConstraintGraph(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MERGE (s:MemoryNode {space_id: $sid, node_id: 'seedC'})
  ON CREATE SET s.role_type = 'constraint', s.constraint_code = 'code-seed', s.layer = 0
MERGE (r:MemoryNode {space_id: $sid, node_id: 'relatedC'})
  ON CREATE SET r.role_type = 'constraint', r.constraint_code = 'code-related', r.layer = 0
MERGE (o:MemoryNode {space_id: $sid, node_id: 'offC'})
  ON CREATE SET o.role_type = 'constraint', o.constraint_code = 'code-off', o.layer = 0
MERGE (s)-[:CO_ACTIVATED_WITH {space_id: $sid, weight: 0.5}]->(r)
`, map[string]any{"sid": spaceID})
		return nil, err
	})
	return err
}

// insertConstraintOutcome writes one constraint_outcomes row directly (column
// order per migration 011). constraint_id is a synthetic UUID-shaped value to
// mirror production (where it does NOT match the Neo4j node_id).
func insertConstraintOutcome(t *testing.T, pool *pgxpool.Pool, spaceID, code, outcome string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
INSERT INTO constraint_outcomes
  (time, space_id, constraint_id, constraint_code, guidance_id, session_id, outcome_type, similarity, guidance_type, instance_id)
VALUES (NOW(), $1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		spaceID, "uuid-"+code, code, "guid-"+code+"-"+outcome, "sess-test",
		outcome, 0.6, "constraint", "test-instance")
	if err != nil {
		t.Fatalf("insert constraint_outcome (%s/%s): %v", code, outcome, err)
	}
}

func cleanupConstraintOutcomes(t *testing.T, pool *pgxpool.Pool, spaceID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`DELETE FROM constraint_outcomes WHERE space_id = $1`, spaceID); err != nil {
		t.Fatalf("cleanup constraint_outcomes: %v", err)
	}
}
