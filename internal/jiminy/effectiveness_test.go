package jiminy

import (
	"testing"
	"time"
)

func TestEffectivenessTracker_TrackAndLookup(t *testing.T) {
	tracker := NewEffectivenessTracker(100, 1800)

	items := []GuidanceItem{
		{Type: GuidanceConstraint, Content: "never hardcode values", Confidence: 0.8},
		{Type: GuidanceCorrection, Content: "use config files", Confidence: 0.7},
	}

	tracker.Track("guid-1", items)

	got := tracker.Lookup("guid-1")
	if got == nil {
		t.Fatal("expected items, got nil")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].Content != "never hardcode values" {
		t.Errorf("unexpected content: %s", got[0].Content)
	}
}

func TestEffectivenessTracker_LookupNotFound(t *testing.T) {
	tracker := NewEffectivenessTracker(100, 1800)

	got := tracker.Lookup("nonexistent")
	if got != nil {
		t.Errorf("expected nil for missing guidance_id, got %v", got)
	}
}

func TestEffectivenessTracker_Expiry(t *testing.T) {
	// 1 second TTL for testing
	tracker := NewEffectivenessTracker(100, 1)

	items := []GuidanceItem{{Content: "test", Confidence: 0.5}}
	tracker.Track("guid-exp", items)

	// Should be found immediately
	if tracker.Lookup("guid-exp") == nil {
		t.Fatal("expected items immediately after track")
	}

	// Wait for expiry
	time.Sleep(1100 * time.Millisecond)

	if tracker.Lookup("guid-exp") != nil {
		t.Error("expected nil after TTL expiry")
	}
}

func TestEffectivenessTracker_CapacityEviction(t *testing.T) {
	tracker := NewEffectivenessTracker(3, 1800)

	tracker.Track("a", []GuidanceItem{{Content: "a"}})
	tracker.Track("b", []GuidanceItem{{Content: "b"}})
	tracker.Track("c", []GuidanceItem{{Content: "c"}})

	if tracker.Size() != 3 {
		t.Fatalf("expected size=3, got %d", tracker.Size())
	}

	// Adding a 4th should evict the oldest
	tracker.Track("d", []GuidanceItem{{Content: "d"}})

	if tracker.Size() != 3 {
		t.Fatalf("expected size=3 after eviction, got %d", tracker.Size())
	}

	// "a" should be evicted (oldest)
	if tracker.Lookup("a") != nil {
		t.Error("expected 'a' to be evicted")
	}
	if tracker.Lookup("d") == nil {
		t.Error("expected 'd' to be present")
	}
}

func TestEffectivenessTracker_CleanupExpired(t *testing.T) {
	tracker := NewEffectivenessTracker(100, 1)

	tracker.Track("x", []GuidanceItem{{Content: "x"}})
	tracker.Track("y", []GuidanceItem{{Content: "y"}})

	time.Sleep(1100 * time.Millisecond)

	removed := tracker.CleanupExpired()
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
	if tracker.Size() != 0 {
		t.Errorf("expected size=0 after cleanup, got %d", tracker.Size())
	}
}

func TestClassifyOutcome_Followed(t *testing.T) {
	item := GuidanceItem{
		Type:    GuidanceConstraint,
		Content: "Always use configurable variables for endpoints and paths",
	}

	outcome, sim := classifyOutcome(item, "used configurable variables for the endpoint path configuration")

	if outcome != OutcomeFollowed {
		t.Errorf("expected OutcomeFollowed, got %s", outcome)
	}
	if sim < 0.3 {
		t.Errorf("expected similarity >= 0.3, got %f", sim)
	}
}

func TestClassifyOutcome_Contradicted(t *testing.T) {
	item := GuidanceItem{
		Type:    GuidanceConstraint,
		Content: "Always use configurable variables for endpoints",
	}

	outcome, _ := classifyOutcome(item, "did not use configurable variables, instead of following that, hardcoded endpoints")

	if outcome != OutcomeContradicted {
		t.Errorf("expected OutcomeContradicted, got %s", outcome)
	}
}

func TestClassifyOutcome_Unknown(t *testing.T) {
	item := GuidanceItem{
		Type:    GuidanceConstraint,
		Content: "Always use configurable variables",
	}

	outcome, sim := classifyOutcome(item, "refactored the database schema to add new tables")

	if outcome != OutcomeUnknown {
		t.Errorf("expected OutcomeUnknown, got %s", outcome)
	}
	if sim != 0.0 {
		t.Errorf("expected similarity=0.0 for unrelated action, got %f", sim)
	}
}

func TestSignificantWords(t *testing.T) {
	words := significantWords("use config files for all paths (required)")
	// "use" (3) and "for" (3) and "all" (3) should be excluded (< 4 chars)
	for _, w := range words {
		if len(w) < 4 {
			t.Errorf("significantWords included short word: %q", w)
		}
	}
	// "config", "files", "paths", "required" should be included
	found := map[string]bool{}
	for _, w := range words {
		found[w] = true
	}
	for _, expected := range []string{"config", "files", "paths", "required"} {
		if !found[expected] {
			t.Errorf("expected %q in significantWords, got %v", expected, words)
		}
	}
}
