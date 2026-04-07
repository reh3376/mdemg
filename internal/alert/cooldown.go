package alert

import (
	"sync"
	"time"
)

type cooldownTracker struct {
	mu      sync.Mutex
	entries map[string]time.Time
	ttl     time.Duration
}

func newCooldownTracker(ttl time.Duration) *cooldownTracker {
	return &cooldownTracker{
		entries: make(map[string]time.Time),
		ttl:     ttl,
	}
}

func (c *cooldownTracker) key(service string, sev Severity) string {
	return service + ":" + string(sev)
}

// Allow returns true if the cooldown for this service+severity has expired or not been set.
func (c *cooldownTracker) Allow(service string, sev Severity) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	k := c.key(service, sev)
	last, ok := c.entries[k]
	if !ok {
		return true
	}
	return time.Since(last) > c.ttl
}

// Record marks the current time for this service+severity cooldown.
func (c *cooldownTracker) Record(service string, sev Severity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[c.key(service, sev)] = time.Now()
}
