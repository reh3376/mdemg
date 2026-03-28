package jiminy

import (
	"container/list"
	"sync"
	"time"
)

// trackedGuidance holds the items returned for a single Guide() call.
type trackedGuidance struct {
	items     []GuidanceItem
	createdAt time.Time
}

// EffectivenessTracker tracks guidance items by guidance_id for later feedback correlation.
// Items expire after a configurable TTL.
type EffectivenessTracker struct {
	mu       sync.Mutex
	cache    map[string]*list.Element
	order    *list.List
	capacity int
	ttl      time.Duration
}

type effectivenessEntry struct {
	guidanceID string
	guidance   trackedGuidance
}

// NewEffectivenessTracker creates a tracker with the given capacity and TTL.
func NewEffectivenessTracker(capacity int, ttlSeconds int) *EffectivenessTracker {
	if capacity <= 0 {
		capacity = 1000
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 7200 // 2 hours
	}
	return &EffectivenessTracker{
		cache:    make(map[string]*list.Element, capacity),
		order:    list.New(),
		capacity: capacity,
		ttl:      time.Duration(ttlSeconds) * time.Second,
	}
}

// Track stores guidance items associated with a guidance_id.
func (et *EffectivenessTracker) Track(guidanceID string, items []GuidanceItem) {
	et.mu.Lock()
	defer et.mu.Unlock()

	// Evict if at capacity
	for et.order.Len() >= et.capacity {
		oldest := et.order.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*effectivenessEntry)
		delete(et.cache, entry.guidanceID)
		et.order.Remove(oldest)
	}

	// Remove existing entry if updating
	if elem, ok := et.cache[guidanceID]; ok {
		et.order.Remove(elem)
		delete(et.cache, guidanceID)
	}

	entry := &effectivenessEntry{
		guidanceID: guidanceID,
		guidance: trackedGuidance{
			items:     items,
			createdAt: time.Now(),
		},
	}
	elem := et.order.PushFront(entry)
	et.cache[guidanceID] = elem
}

// Lookup retrieves tracked guidance items by guidance_id.
// Returns nil if not found or expired.
func (et *EffectivenessTracker) Lookup(guidanceID string) []GuidanceItem {
	et.mu.Lock()
	defer et.mu.Unlock()

	elem, ok := et.cache[guidanceID]
	if !ok {
		return nil
	}

	entry := elem.Value.(*effectivenessEntry)
	if time.Since(entry.guidance.createdAt) > et.ttl {
		// Expired — remove and return nil
		delete(et.cache, guidanceID)
		et.order.Remove(elem)
		return nil
	}

	// Move to front (LRU)
	et.order.MoveToFront(elem)
	return entry.guidance.items
}

// Size returns the number of tracked guidance entries.
func (et *EffectivenessTracker) Size() int {
	et.mu.Lock()
	defer et.mu.Unlock()
	return len(et.cache)
}

// CleanupExpired removes all expired entries.
func (et *EffectivenessTracker) CleanupExpired() int {
	et.mu.Lock()
	defer et.mu.Unlock()

	removed := 0
	for e := et.order.Back(); e != nil; {
		prev := e.Prev()
		entry := e.Value.(*effectivenessEntry)
		if time.Since(entry.guidance.createdAt) > et.ttl {
			delete(et.cache, entry.guidanceID)
			et.order.Remove(e)
			removed++
		}
		e = prev
	}
	return removed
}
