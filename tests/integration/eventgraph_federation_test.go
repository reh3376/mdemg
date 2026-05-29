//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/eventgraph"
	"mdemg/internal/tsdb"
)

// TestEventGraph_Federation_RoundTrip is the full Pattern Y1 happy-path:
// build a small graph in Neo4j + populate reinforcement_events in TSDB,
// then call the federation Service and assert events appear with
// SrcInNeighborhood/DstInNeighborhood flags correctly stamped.
func TestEventGraph_Federation_RoundTrip(t *testing.T) {
	driver := SetupTestNeo4j(t)
	client := newTestClient(t)
	pool := client.Pool()
	ctx := context.Background()

	spaceID := "test-eventgraph-federation"
	if err := CleanupSpace(ctx, driver, spaceID); err != nil {
		t.Fatalf("cleanup neo4j: %v", err)
	}
	cleanupReinforcementEvents(t, client, spaceID)

	// Build a tiny graph: seed -- mid -- leaf, plus an off-neighborhood node.
	if err := seedNeighborhoodGraph(ctx, driver, spaceID); err != nil {
		t.Fatalf("seed graph: %v", err)
	}
	t.Cleanup(func() { _ = CleanupSpace(context.Background(), driver, spaceID) })

	// Emit 3 reinforcement events directly via the writer:
	//   1) seed ↔ mid       (both in neighborhood at hops=1)
	//   2) mid ↔ leaf       (both in neighborhood at hops=2; mid only at hops=1)
	//   3) seed ↔ off-node  (seed in, off-node out)
	w := tsdb.NewReinforcementEventsWriter(pool, 200*time.Millisecond, 100)
	t.Cleanup(w.Close)

	pairs := []struct{ src, dst string }{
		{"seed", "mid"},
		{"mid", "leaf"},
		{"seed", "off-node"},
	}
	for _, p := range pairs {
		w.Record(tsdb.ReinforcementEventRow{
			SpaceID:            spaceID,
			SrcNodeID:          p.src,
			DstNodeID:          p.dst,
			PrevWeight:         0.1,
			NewWeight:          0.2,
			DeltaWeight:        0.1,
			EvidenceCountAfter: 1,
			Direction:          "bidirectional",
			CreatedNewEdge:     true,
			TriggerPath:        "apply_coactivation",
		})
	}
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	svc := eventgraph.NewService(driver, pool)

	// Hops=1 from seed: neighborhood = {seed, mid}. Events 1 and 3 touch the
	// neighborhood; event 2 (mid-leaf) does too (mid is in the neighborhood).
	res1, err := svc.EventsInGraphNeighborhood(ctx, eventgraph.FederationRequest{
		SpaceID:    spaceID,
		SeedNodeID: "seed",
		Hops:       1,
		Since:      time.Hour,
		Limit:      100,
	})
	if err != nil {
		t.Fatalf("federation hops=1: %v", err)
	}
	if len(res1.NeighborNodeIDs) < 2 {
		t.Errorf("hops=1 neighborhood should include seed + mid, got %v", res1.NeighborNodeIDs)
	}
	if len(res1.Events) < 3 {
		t.Errorf("expected ≥ 3 events touching neighborhood, got %d", len(res1.Events))
	}

	// Assert in-neighborhood flags for the seed↔mid event.
	for _, ev := range res1.Events {
		if (ev.SrcNodeID == "seed" && ev.DstNodeID == "mid") || (ev.SrcNodeID == "mid" && ev.DstNodeID == "seed") {
			if !ev.SrcInNeighborhood || !ev.DstInNeighborhood {
				t.Errorf("seed↔mid event should have both endpoints in neighborhood, got src=%v dst=%v",
					ev.SrcInNeighborhood, ev.DstInNeighborhood)
			}
		}
		if (ev.SrcNodeID == "seed" && ev.DstNodeID == "off-node") || (ev.SrcNodeID == "off-node" && ev.DstNodeID == "seed") {
			// One side in, one side out at hops=1.
			if ev.SrcInNeighborhood == ev.DstInNeighborhood {
				t.Errorf("seed↔off-node event should have exactly one endpoint in neighborhood at hops=1, got src=%v dst=%v",
					ev.SrcInNeighborhood, ev.DstInNeighborhood)
			}
		}
	}

	// Hops=0: neighborhood = {seed} alone. Only events touching seed return.
	res0, err := svc.EventsInGraphNeighborhood(ctx, eventgraph.FederationRequest{
		SpaceID:    spaceID,
		SeedNodeID: "seed",
		Hops:       0,
		Since:      time.Hour,
		Limit:      100,
	})
	if err != nil {
		t.Fatalf("federation hops=0: %v", err)
	}
	if len(res0.NeighborNodeIDs) != 1 || res0.NeighborNodeIDs[0] != "seed" {
		t.Errorf("hops=0 neighborhood = %v, want [seed]", res0.NeighborNodeIDs)
	}
	// Events: seed↔mid + seed↔off-node touch seed; mid↔leaf does not.
	if len(res0.Events) < 2 {
		t.Errorf("hops=0 should match ≥ 2 events touching seed, got %d", len(res0.Events))
	}
	for _, ev := range res0.Events {
		if ev.SrcNodeID == "mid" && ev.DstNodeID == "leaf" {
			t.Errorf("hops=0 should not return mid↔leaf event (neither endpoint is the seed)")
		}
	}
}

// seedNeighborhoodGraph creates: seed --[CO_ACTIVATED_WITH]-- mid --[CO_ACTIVATED_WITH]-- leaf
// plus a disconnected "off-node" MemoryNode (no edge to seed/mid/leaf so it
// only enters via direct event src/dst, not via graph walk).
func seedNeighborhoodGraph(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MERGE (s:MemoryNode {space_id: $sid, node_id: 'seed'}) ON CREATE SET s.path = '/seed.go', s.layer = 0
MERGE (m:MemoryNode {space_id: $sid, node_id: 'mid'})  ON CREATE SET m.path = '/mid.go',  m.layer = 0
MERGE (l:MemoryNode {space_id: $sid, node_id: 'leaf'}) ON CREATE SET l.path = '/leaf.go', l.layer = 0
MERGE (o:MemoryNode {space_id: $sid, node_id: 'off-node'}) ON CREATE SET o.path = '/off.go', o.layer = 0
MERGE (s)-[:CO_ACTIVATED_WITH {space_id: $sid, weight: 0.5}]->(m)
MERGE (m)-[:CO_ACTIVATED_WITH {space_id: $sid, weight: 0.5}]->(l)
`, map[string]any{"sid": spaceID})
		return nil, err
	})
	return err
}
