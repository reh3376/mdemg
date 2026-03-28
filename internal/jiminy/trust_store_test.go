package jiminy

import (
	"sort"
	"sync"
	"testing"
	"time"
)

func TestTrustStore_MarkDirtyAndDrain(t *testing.T) {
	store := NewTrustStore(nil)

	store.MarkDirty("sess-1")
	store.MarkDirty("sess-2")
	store.MarkDirty("sess-3")

	keys := store.DrainDirty()
	if len(keys) != 3 {
		t.Fatalf("DrainDirty() returned %d keys, want 3", len(keys))
	}

	sort.Strings(keys)
	want := []string{"sess-1", "sess-2", "sess-3"}
	for i, k := range keys {
		if k != want[i] {
			t.Errorf("keys[%d] = %q, want %q", i, k, want[i])
		}
	}

	// Second drain should return nil (empty).
	keys2 := store.DrainDirty()
	if keys2 != nil {
		t.Errorf("second DrainDirty() = %v, want nil", keys2)
	}
}

func TestTrustStore_RemarkDirty(t *testing.T) {
	store := NewTrustStore(nil)

	store.MarkDirty("sess-A")
	store.MarkDirty("sess-B")

	// Drain clears the dirty set.
	_ = store.DrainDirty()

	// Remark one session as dirty (simulates flush failure retry).
	store.RemarkDirty("sess-A")

	keys := store.DrainDirty()
	if len(keys) != 1 {
		t.Fatalf("DrainDirty() after RemarkDirty returned %d keys, want 1", len(keys))
	}
	if keys[0] != "sess-A" {
		t.Errorf("remarked key = %q, want %q", keys[0], "sess-A")
	}
}

func TestTrustStore_GetStatus(t *testing.T) {
	store := NewTrustStore(nil)

	store.MarkDirty("sess-1")
	store.MarkDirty("sess-2")

	status := store.GetStatus()

	enabled, ok := status["enabled"].(bool)
	if !ok || !enabled {
		t.Errorf("status[enabled] = %v, want true", status["enabled"])
	}

	dirtyKeys, ok := status["dirty_keys"].(int)
	if !ok || dirtyKeys != 2 {
		t.Errorf("status[dirty_keys] = %v, want 2", status["dirty_keys"])
	}

	flushErrors, ok := status["flush_errors"].(int64)
	if !ok || flushErrors != 0 {
		t.Errorf("status[flush_errors] = %v, want 0", status["flush_errors"])
	}

	// last_flush should be absent when no flush has occurred.
	if _, exists := status["last_flush"]; exists {
		t.Errorf("status[last_flush] should not be present before any flush")
	}
}

func TestTrustScorer_OnDirtyCallback(t *testing.T) {
	var mu sync.Mutex
	var dirtied []string

	scorer := NewTrustScorer(TrustConfig{
		Initial:        0.5,
		BoostPerFollow: 0.1,
	})
	scorer.SetOnDirty(func(sessionID string) {
		mu.Lock()
		dirtied = append(dirtied, sessionID)
		mu.Unlock()
	})

	// RecordOutcome should fire callback.
	scorer.RecordOutcome("sess-cb-1", OutcomeFollowed)

	// SetScore should also fire callback.
	scorer.SetScore("sess-cb-2", 0.9)

	mu.Lock()
	defer mu.Unlock()

	if len(dirtied) != 2 {
		t.Fatalf("onDirty called %d times, want 2", len(dirtied))
	}
	if dirtied[0] != "sess-cb-1" {
		t.Errorf("dirtied[0] = %q, want %q", dirtied[0], "sess-cb-1")
	}
	if dirtied[1] != "sess-cb-2" {
		t.Errorf("dirtied[1] = %q, want %q", dirtied[1], "sess-cb-2")
	}
}

func TestTrustScorer_GetAllEntries(t *testing.T) {
	scorer := NewTrustScorer(TrustConfig{
		Initial:        0.5,
		BoostPerFollow: 0.1,
	})

	scorer.SetScore("sess-A", 0.7)
	scorer.SetScore("sess-B", 0.3)
	scorer.SetScore("sess-C", 0.9)

	entries := scorer.GetAllEntries()
	if len(entries) != 3 {
		t.Fatalf("GetAllEntries() returned %d entries, want 3", len(entries))
	}

	checks := map[string]float64{
		"sess-A": 0.7,
		"sess-B": 0.3,
		"sess-C": 0.9,
	}
	for id, wantScore := range checks {
		entry, ok := entries[id]
		if !ok {
			t.Errorf("missing entry for %q", id)
			continue
		}
		if !approxEqual(entry.Score, wantScore) {
			t.Errorf("entries[%q].Score = %f, want %f", id, entry.Score, wantScore)
		}
	}
}

func TestTrustScorer_ExtendedDefaultTTL(t *testing.T) {
	// Zero-value TTL config should produce the 168h (7-day) default.
	scorer := NewTrustScorer(TrustConfig{
		Initial: 0.5,
		// TTL intentionally omitted (zero value).
	})

	want := 168 * time.Hour
	if scorer.ttl != want {
		t.Errorf("default ttl = %v, want %v", scorer.ttl, want)
	}
}
