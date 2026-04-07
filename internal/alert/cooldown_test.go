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
