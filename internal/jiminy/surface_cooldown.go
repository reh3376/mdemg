package jiminy

import (
	"container/list"
	"sync"
	"time"
)

// JIMINY-CORPUS-001 Epic 3, Lever A — per-session surfacing cooldown.
//
// The Epic-3 diagnostic showed a handful of guidance nodes dominating the
// outcome stream (~49× per node per week) irrespective of task relevance, so
// correctly-ignored repetition flooded the corpus. The cooldown tracker counts
// CONSECUTIVE ignored outcomes per (session, node); once the count reaches the
// configured threshold the node is suppressed from surfacing in that session
// until the counter is released.
//
// Release semantics:
//   - followed / partial_compliance → the entry is deleted (counter reset to 0)
//   - ignored → counter increments
//   - contradicted → NO increment: an actively-violated constraint is highly
//     relevant and must keep surfacing (escalation owns that path)
//   - not_applicable / unknown → no change (off-topic verdicts carry no
//     repetition signal)
//
// Memory bound: a strict LRU over (session, node) entries, capacity from
// JIMINY_SURFACE_COOLDOWN_CAPACITY (default 5000). Each entry is ~100 bytes,
// so the default bound is ~500 KB worst case. Least-recently-updated entries
// evict first; an evicted entry simply restarts its count at 0, which fails
// open (surfaces again) — never fails closed.
type SurfaceCooldownTracker struct {
	mu       sync.Mutex
	cache    map[cooldownKey]*list.Element
	order    *list.List // front = most recently updated (LRU)
	capacity int
}

type cooldownKey struct {
	SessionID string
	NodeID    string
}

type cooldownEntry struct {
	key                cooldownKey
	consecutiveIgnored int
	lastIgnoredAt      time.Time
}

// defaultSurfaceCooldownCapacity is a constructor safety floor only — the
// operator-facing default lives in config (JIMINY_SURFACE_COOLDOWN_CAPACITY).
const defaultSurfaceCooldownCapacity = 5000

// NewSurfaceCooldownTracker creates a bounded per-session cooldown tracker.
func NewSurfaceCooldownTracker(capacity int) *SurfaceCooldownTracker {
	if capacity <= 0 {
		capacity = defaultSurfaceCooldownCapacity
	}
	return &SurfaceCooldownTracker{
		cache:    make(map[cooldownKey]*list.Element, capacity),
		order:    list.New(),
		capacity: capacity,
	}
}

// RecordOutcome updates the consecutive-ignored counter for a node in a session.
func (ct *SurfaceCooldownTracker) RecordOutcome(sessionID, nodeID string, outcome GuidanceOutcome) {
	if ct == nil || sessionID == "" || nodeID == "" {
		return
	}
	ct.mu.Lock()
	defer ct.mu.Unlock()

	key := cooldownKey{SessionID: sessionID, NodeID: nodeID}

	switch outcome {
	case OutcomeFollowed, OutcomePartialCompliance:
		// Release: reset by deleting the entry (frees the LRU slot too).
		if elem, ok := ct.cache[key]; ok {
			ct.order.Remove(elem)
			delete(ct.cache, key)
		}
	case OutcomeIgnored:
		elem, ok := ct.cache[key]
		if !ok {
			// Evict least-recently-updated beyond capacity.
			for ct.order.Len() >= ct.capacity {
				oldest := ct.order.Back()
				if oldest == nil {
					break
				}
				old := oldest.Value.(*cooldownEntry)
				delete(ct.cache, old.key)
				ct.order.Remove(oldest)
			}
			entry := &cooldownEntry{key: key, consecutiveIgnored: 1, lastIgnoredAt: time.Now()}
			ct.cache[key] = ct.order.PushFront(entry)
			return
		}
		entry := elem.Value.(*cooldownEntry)
		entry.consecutiveIgnored++
		entry.lastIgnoredAt = time.Now()
		ct.order.MoveToFront(elem)
	default:
		// contradicted / not_applicable / unknown — no repetition signal.
	}
}

// State returns the consecutive-ignored count and the last-ignored timestamp
// for a node in a session. A missing entry returns (0, zero time).
func (ct *SurfaceCooldownTracker) State(sessionID, nodeID string) (int, time.Time) {
	if ct == nil {
		return 0, time.Time{}
	}
	ct.mu.Lock()
	defer ct.mu.Unlock()

	elem, ok := ct.cache[cooldownKey{SessionID: sessionID, NodeID: nodeID}]
	if !ok {
		return 0, time.Time{}
	}
	entry := elem.Value.(*cooldownEntry)
	return entry.consecutiveIgnored, entry.lastIgnoredAt
}

// Size returns the number of tracked (session, node) entries.
func (ct *SurfaceCooldownTracker) Size() int {
	if ct == nil {
		return 0
	}
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return len(ct.cache)
}
