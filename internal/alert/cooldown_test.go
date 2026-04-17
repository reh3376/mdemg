package alert

import (
	"sync"
	"testing"
	"time"
)

func TestCooldownTracker_AllowAndRecord(t *testing.T) {
	ct := newCooldownTracker(100 * time.Millisecond)

	// First call should be allowed.
	if !ct.Allow("neo4j", SeverityHigh) {
		t.Fatal("first call should be allowed")
	}

	ct.Record("neo4j", SeverityHigh)

	// Second call within cooldown should be blocked.
	if ct.Allow("neo4j", SeverityHigh) {
		t.Fatal("second call within cooldown should be blocked")
	}

	// Different service should be allowed.
	if !ct.Allow("openai", SeverityHigh) {
		t.Fatal("different service should be allowed")
	}

	// Different severity same service should be allowed.
	if !ct.Allow("neo4j", SeverityCritical) {
		t.Fatal("different severity should be allowed")
	}

	// After cooldown expires, should be allowed again.
	time.Sleep(120 * time.Millisecond)
	if !ct.Allow("neo4j", SeverityHigh) {
		t.Fatal("should be allowed after cooldown expires")
	}
}

func TestCooldownTracker_ConcurrentAccess(t *testing.T) {
	ct := newCooldownTracker(50 * time.Millisecond)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			ct.Allow("svc", SeverityHigh)
			ct.Record("svc", SeverityHigh)
		})
	}
	wg.Wait()
}

// DH-004 E4.4: TryRecord must return true exactly once per cooldown window,
// even under concurrent access. Allow+Record as separate calls had a TOCTOU
// race where multiple goroutines could all pass Allow() before any recorded.
func TestCooldownTracker_TryRecord_AtomicUnderRace(t *testing.T) {
	ct := newCooldownTracker(500 * time.Millisecond)

	const goroutines = 100
	var wins int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})

	for range goroutines {
		wg.Go(func() {
			<-start
			if ct.TryRecord("jiminy", SeverityCritical) {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		})
	}
	close(start) // release all goroutines at once
	wg.Wait()

	if wins != 1 {
		t.Errorf("TryRecord allowed %d goroutines past cooldown, want exactly 1", wins)
	}
}

func TestCooldownTracker_TryRecord_ReAllowsAfterTTL(t *testing.T) {
	ct := newCooldownTracker(50 * time.Millisecond)

	if !ct.TryRecord("jiminy", SeverityCritical) {
		t.Fatal("first TryRecord should succeed")
	}
	if ct.TryRecord("jiminy", SeverityCritical) {
		t.Fatal("second TryRecord within cooldown should be suppressed")
	}
	time.Sleep(60 * time.Millisecond)
	if !ct.TryRecord("jiminy", SeverityCritical) {
		t.Fatal("TryRecord after cooldown expires should succeed")
	}
}

func TestCooldownTracker_TryRecord_PerServiceSeverity(t *testing.T) {
	ct := newCooldownTracker(500 * time.Millisecond)

	if !ct.TryRecord("jiminy", SeverityCritical) {
		t.Fatal("jiminy critical should succeed")
	}
	if !ct.TryRecord("jiminy", SeverityHigh) {
		t.Fatal("jiminy high is a separate cooldown key, should succeed")
	}
	if !ct.TryRecord("openai", SeverityCritical) {
		t.Fatal("openai critical is a separate cooldown key, should succeed")
	}
	if ct.TryRecord("jiminy", SeverityCritical) {
		t.Fatal("jiminy critical repeat should be suppressed")
	}
}
