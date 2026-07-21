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

// ─── DASHBOARD-TRUTH-001 Epic 4: hydration provenance + gauge significance ───

// (a) Hydration must preserve a persisted old LastUpdate (not stamp boot-time),
// so a session last-fed >TTL ago is eligible for TTL cleanup after load.
func TestHydrateTrust_PreservesProvenance_StaleSessionExpires(t *testing.T) {
	scorer := NewTrustScorer(TrustConfig{Initial: 0.65, TTL: 168 * time.Hour})
	s := &Service{
		trustScorer:    scorer,
		feedbackCounts: make(map[string]*sessionFeedback),
	}

	staleFeed := time.Now().Add(-200 * time.Hour) // > 168h TTL
	freshFeed := time.Now().Add(-1 * time.Hour)
	snapshots := map[string]*TrustSnapshot{
		"stale-march-test": {
			SessionID: "stale-march-test", Score: 0.3,
			LastUpdate: time.Now(), // the rotted boot-stamp seen live — must NOT win
			LastFeedAt: staleFeed, FeedbackCount: 1,
		},
		// (b) a freshly-persisted session survives.
		"fresh-live": {
			SessionID: "fresh-live", Score: 0.8,
			LastUpdate: freshFeed, LastFeedAt: freshFeed, FeedbackCount: 12,
		},
	}

	hydrated, expired := s.hydrateTrustSnapshots(snapshots)
	if hydrated != 2 {
		t.Fatalf("hydrated = %d, want 2", hydrated)
	}
	if expired != 1 {
		t.Fatalf("expired = %d, want 1 (the stale March session)", expired)
	}

	// Stale session was dropped by the TTL sweep at hydration time.
	if _, _, ok := scorer.GetEntry("stale-march-test"); ok {
		t.Errorf("stale session survived hydration; want TTL-expired (provenance was not preserved)")
	}
	// Fresh session survives with its provenance intact.
	score, lastUpdate, ok := scorer.GetEntry("fresh-live")
	if !ok {
		t.Fatalf("fresh session missing after hydration")
	}
	if !approxEqual(score, 0.8) {
		t.Errorf("fresh session score = %f, want 0.8", score)
	}
	if !lastUpdate.Equal(freshFeed) {
		t.Errorf("fresh session LastUpdate = %v, want preserved %v (not boot-time)", lastUpdate, freshFeed)
	}
	// Feedback counts restored only for survivors.
	s.feedbackMu.RLock()
	defer s.feedbackMu.RUnlock()
	if _, ok := s.feedbackCounts["stale-march-test"]; ok {
		t.Errorf("stale session feedback count restored; want dropped with the entry")
	}
	if sf := s.feedbackCounts["fresh-live"]; sf == nil || sf.Count != 12 {
		t.Errorf("fresh session feedback count = %v, want 12", sf)
	}
}

func TestEffectiveHydrationTimestamp_FallbackChain(t *testing.T) {
	feed := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	upd := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	node := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		snap TrustSnapshot
		want time.Time
	}{
		// last_feed_at is the "has this session gone silent" signal and wins
		// even over a newer last_update (last_update was boot-rotted live).
		{"feed_preferred", TrustSnapshot{LastFeedAt: feed, LastUpdate: upd, NodeUpdatedAt: node}, feed},
		{"last_update_fallback", TrustSnapshot{LastUpdate: upd, NodeUpdatedAt: node}, upd},
		{"node_updated_fallback", TrustSnapshot{NodeUpdatedAt: node}, node},
		// No provenance at all → zero (immediately expired), NEVER now().
		{"zero_never_now", TrustSnapshot{}, time.Time{}},
	}
	for _, tc := range cases {
		if got := effectiveHydrationTimestamp(&tc.snap); !got.Equal(tc.want) {
			t.Errorf("%s: effectiveHydrationTimestamp = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Hydration is a read-back, not a change: RestoreEntry must not fire onDirty,
// or the next flush rewrites the persisted last_update (the live rot: all 116
// Neo4j rows carried the same boot instant).
func TestRestoreEntry_DoesNotMarkDirtyOrStampNow(t *testing.T) {
	scorer := NewTrustScorer(TrustConfig{Initial: 0.65})
	dirtied := 0
	scorer.SetOnDirty(func(string) { dirtied++ })

	old := time.Now().Add(-500 * time.Hour)
	scorer.RestoreEntry("sess-old", 0.42, old)

	if dirtied != 0 {
		t.Errorf("RestoreEntry fired onDirty %d times, want 0", dirtied)
	}
	score, lastUpdate, ok := scorer.GetEntry("sess-old")
	if !ok {
		t.Fatalf("entry missing after RestoreEntry")
	}
	if !approxEqual(score, 0.42) {
		t.Errorf("score = %f, want 0.42 (trust value must be untouched)", score)
	}
	if !lastUpdate.Equal(old) {
		t.Errorf("LastUpdate = %v, want preserved %v", lastUpdate, old)
	}
	// And it is TTL-eligible: CleanupExpired removes it.
	if removed := scorer.CleanupExpired(); removed != 1 {
		t.Errorf("CleanupExpired removed %d, want 1", removed)
	}
}

// FlushTrust persists the entry's real LastUpdate, not flush time.
func TestFlushSnapshotProvenance_GetEntryReturnsRealTimestamp(t *testing.T) {
	scorer := NewTrustScorer(TrustConfig{Initial: 0.65})
	old := time.Now().Add(-2 * time.Hour)
	scorer.RestoreEntry("sess-flush", 0.7, old)

	_, lastUpdate, ok := scorer.GetEntry("sess-flush")
	if !ok {
		t.Fatalf("entry missing")
	}
	if !lastUpdate.Equal(old) {
		t.Errorf("GetEntry LastUpdate = %v, want %v (flush must persist real provenance)", lastUpdate, old)
	}
}

// (c) The min/avg/max/count gauges exclude sub-threshold and stale sessions
// and include a significant live one.
func TestAggregatesFiltered_SignificanceFloor(t *testing.T) {
	scorer := NewTrustScorer(TrustConfig{Initial: 0.65, TTL: 168 * time.Hour})

	now := time.Now()
	scorer.RestoreEntry("stale-test", 0.05, now.Add(-2000*time.Hour)) // stale → excluded by TTL
	scorer.RestoreEntry("one-shot", 0.10, now.Add(-1*time.Hour))      // fresh, insignificant → excluded by floor
	scorer.RestoreEntry("significant", 0.80, now.Add(-1*time.Hour))   // fresh + significant → included

	feedback := map[string]int{"stale-test": 20, "one-shot": 1, "significant": 12}
	minFeed := 5
	significant := func(sid string) bool { return feedback[sid] >= minFeed }

	avg, min, max, count := scorer.AggregatesFiltered(significant)
	if count != 1 {
		t.Fatalf("count = %d, want 1 (only the significant live session)", count)
	}
	if !approxEqual(min, 0.80) || !approxEqual(max, 0.80) || !approxEqual(avg, 0.80) {
		t.Errorf("avg/min/max = %f/%f/%f, want 0.80 each", avg, min, max)
	}

	// Nil filter: recency-only (floor disabled) — the one-shot re-enters.
	_, min2, _, count2 := scorer.AggregatesFiltered(nil)
	if count2 != 2 {
		t.Errorf("nil-filter count = %d, want 2 (TTL still excludes the stale session)", count2)
	}
	if !approxEqual(min2, 0.10) {
		t.Errorf("nil-filter min = %f, want 0.10", min2)
	}
}
