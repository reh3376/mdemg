//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"mdemg/internal/tsdb"
)

// TestEventGraph_Writer_RoundTrip exercises the V0022 writer end-to-end against
// a real TSDB instance: record three rows, sleep one flush window, observe the
// rows in the reinforcement_events hypertable, then clean up.
func TestEventGraph_Writer_RoundTrip(t *testing.T) {
	client := newTestClient(t)
	pool := client.Pool()

	spaceID := "test-eventgraph-roundtrip"
	cleanupReinforcementEvents(t, client, spaceID)

	// flushInterval kept short so the test is fast — production default is 30s.
	w := tsdb.NewReinforcementEventsWriter(pool, 200*time.Millisecond, 100)
	t.Cleanup(w.Close)

	for i := range 3 {
		w.Record(tsdb.ReinforcementEventRow{
			SpaceID:            spaceID,
			SrcNodeID:          "node-a-" + string(rune('0'+i)),
			DstNodeID:          "node-b-" + string(rune('0'+i)),
			PrevWeight:         0.1,
			NewWeight:          0.2,
			DeltaWeight:        0.1,
			EvidenceCountAfter: 1,
			Direction:          "bidirectional",
			CreatedNewEdge:     true,
			TriggerPath:        "apply_coactivation",
		})
	}

	// Wait for at least one flush window.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if countReinforcementRows(t, client, spaceID) >= 3 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	got := countReinforcementRows(t, client, spaceID)
	if got != 3 {
		t.Fatalf("expected 3 rows in reinforcement_events for %s, got %d", spaceID, got)
	}

	stats := w.Stats()
	if stats.SuccessCount < 1 || stats.TotalRows < 3 {
		t.Errorf("writer stats unexpected: %+v", stats)
	}
}

// TestEventGraph_Writer_DrainOnClose verifies Close() flushes the buffer
// before returning — Epic 4 server shutdown depends on this.
func TestEventGraph_Writer_DrainOnClose(t *testing.T) {
	client := newTestClient(t)
	pool := client.Pool()
	spaceID := "test-eventgraph-drain"
	cleanupReinforcementEvents(t, client, spaceID)

	// Long flush interval so the ticker won't fire during the test —
	// only Close() can drain.
	w := tsdb.NewReinforcementEventsWriter(pool, 1*time.Hour, 100)
	for range 5 {
		w.Record(tsdb.ReinforcementEventRow{
			SpaceID:            spaceID,
			SrcNodeID:          "src",
			DstNodeID:          "dst",
			PrevWeight:         0.1,
			NewWeight:          0.2,
			DeltaWeight:        0.1,
			EvidenceCountAfter: 1,
			Direction:          "bidirectional",
			CreatedNewEdge:     true,
			TriggerPath:        "apply_coactivation",
		})
	}
	w.Close()

	got := countReinforcementRows(t, client, spaceID)
	if got != 5 {
		t.Errorf("expected 5 rows after Close-drain, got %d", got)
	}
}

func cleanupReinforcementEvents(t *testing.T, client *tsdb.Client, spaceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Pool().Exec(ctx, `DELETE FROM reinforcement_events WHERE space_id = $1`, spaceID)
	if err != nil {
		t.Fatalf("cleanup reinforcement_events failed: %v", err)
	}
}

func countReinforcementRows(t *testing.T, client *tsdb.Client, spaceID string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row := client.Pool().QueryRow(ctx,
		`SELECT count(*) FROM reinforcement_events WHERE space_id = $1`, spaceID)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	return n
}
