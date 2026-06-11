//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/ape"
)

// RSIC-STORM-001 Tier 3 drill: tombstone_stale's rollback snapshot must
// capture the SAME node set the executor archives. Pre-fix the snapshot
// used a drifted (unlinked) predicate and rollback restored nothing
// (restored_count=0 live, 2026-06-11).
func TestTombstoneStale_RollbackRestoresArchivedSet(t *testing.T) {
	driver := SetupTestNeo4j(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	space := "rsic-rollback-drill"
	if err := CleanupSpace(ctx, driver, space); err != nil {
		t.Fatalf("pre-clean: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ccancel()
		_ = CleanupSpace(cctx, driver, space)
	})

	// Seed: one recent correction + 3 older same-session observations
	// (linked per the RSIC-VALIDATE-001 predicate) + 1 unlinked observation
	// (different session, no co-activation) that must NOT be touched.
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
			CREATE (c:MemoryNode {space_id: $space, node_id: 'drill-correction',
				role_type: 'conversation_observation', obs_type: 'correction',
				session_id: 'drill-session', created_at: datetime()})
			CREATE (:MemoryNode {space_id: $space, node_id: 'drill-obs-1',
				role_type: 'conversation_observation', obs_type: 'note',
				session_id: 'drill-session', created_at: datetime() - duration('PT2H')})
			CREATE (:MemoryNode {space_id: $space, node_id: 'drill-obs-2',
				role_type: 'conversation_observation', obs_type: 'note',
				session_id: 'drill-session', created_at: datetime() - duration('PT3H')})
			CREATE (:MemoryNode {space_id: $space, node_id: 'drill-obs-3',
				role_type: 'conversation_observation', obs_type: 'progress',
				session_id: 'drill-session', created_at: datetime() - duration('PT4H')})
			CREATE (:MemoryNode {space_id: $space, node_id: 'drill-unlinked',
				role_type: 'conversation_observation', obs_type: 'note',
				session_id: 'other-session', created_at: datetime() - duration('PT5H')})
		`, map[string]any{"space": space})
		return nil, err
	})
	sess.Close(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	ss := ape.NewSnapshotStore(driver, 3600)
	d := ape.NewDispatcher(driver, nil, nil, nil)
	d.SetSnapshotStore(ss)

	// 1. Snapshot BEFORE the action (as the cycle does).
	snap, err := ss.CaptureSnapshot(ctx, "drill-cycle", "tombstone_stale", space)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if !snap.Reversible || snap.AffectedCount != 3 {
		t.Fatalf("snapshot should capture exactly the 3 linked candidates, got affected=%d reversible=%v",
			snap.AffectedCount, snap.Reversible)
	}

	// 2. Execute the archival.
	out, err := d.ExecuteTombstoneStaleForTest(ctx, space, "drill-cycle")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out["tombstoned"].(int64) != 3 {
		t.Fatalf("expected 3 tombstoned, got %v", out["tombstoned"])
	}

	// Attribution metadata present; unlinked node untouched.
	assertCount(t, ctx, driver, space,
		`MATCH (n:MemoryNode {space_id: $space})
		 WHERE n.is_archived = true AND n.archive_reason = 'rsic_tombstone_stale'
		   AND n.archived_cycle_id = 'drill-cycle' AND n.archived_at IS NOT NULL
		 RETURN count(n) AS c`, 3)
	assertCount(t, ctx, driver, space,
		`MATCH (n:MemoryNode {space_id: $space, node_id: 'drill-unlinked'})
		 WHERE NOT coalesce(n.is_archived, false) RETURN count(n) AS c`, 1)

	// 3. Roll back — must restore the SAME 3 nodes.
	rb, err := ss.Rollback(ctx, snap.SnapshotID)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rb.RestoredCount != 3 {
		t.Fatalf("expected RestoredCount=3 (pre-fix this was 0), got %d", rb.RestoredCount)
	}
	assertCount(t, ctx, driver, space,
		`MATCH (n:MemoryNode {space_id: $space})
		 WHERE coalesce(n.is_archived, false) RETURN count(n) AS c`, 0)
	assertCount(t, ctx, driver, space,
		`MATCH (n:MemoryNode {space_id: $space})
		 WHERE n.archive_reason IS NOT NULL OR n.archived_cycle_id IS NOT NULL
		 RETURN count(n) AS c`, 0)
}

func assertCount(t *testing.T, ctx context.Context, driver neo4j.DriverWithContext, space, cypher string, want int64) {
	t.Helper()
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)
	res, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		r, err := tx.Run(ctx, cypher, map[string]any{"space": space})
		if err != nil {
			return nil, err
		}
		if r.Next(ctx) {
			v, _ := r.Record().Get("c")
			return v, nil
		}
		return int64(-1), r.Err()
	})
	if err != nil {
		t.Fatalf("assertCount: %v", err)
	}
	if res.(int64) != want {
		t.Fatalf("count = %d, want %d (query: %s)", res.(int64), want, cypher)
	}
}
