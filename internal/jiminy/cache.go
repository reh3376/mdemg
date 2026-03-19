package jiminy

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// GuidanceCache caches guidance responses keyed by a normalized context hash.
// Uses LRU eviction and TTL-based expiry.
type GuidanceCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element
	order    *list.List
	capacity int
	ttl      time.Duration
}

type cacheEntry struct {
	key       string
	response  GuidanceResponse
	createdAt time.Time
}

// NewGuidanceCache creates a new guidance cache.
func NewGuidanceCache(capacity int, ttlSeconds int) *GuidanceCache {
	if capacity <= 0 {
		capacity = 200
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 300
	}
	return &GuidanceCache{
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
		capacity: capacity,
		ttl:      time.Duration(ttlSeconds) * time.Second,
	}
}

// cacheKey generates a normalized key from space_id + context.
// Uses first 200 chars of context + SHA256 for stable hashing.
func cacheKey(spaceID, context string) string {
	// Normalize: lowercase, trim whitespace, take first 200 chars
	normalized := strings.ToLower(strings.TrimSpace(context))
	if len(normalized) > 200 {
		normalized = normalized[:200]
	}
	h := sha256.Sum256([]byte(spaceID + ":" + normalized))
	return hex.EncodeToString(h[:16]) // 128-bit key
}

// Get retrieves a cached guidance response if available and not expired.
func (gc *GuidanceCache) Get(spaceID, context string) (GuidanceResponse, bool) {
	key := cacheKey(spaceID, context)

	gc.mu.Lock()
	defer gc.mu.Unlock()

	elem, ok := gc.items[key]
	if !ok {
		return GuidanceResponse{}, false
	}

	entry := elem.Value.(*cacheEntry)
	if time.Since(entry.createdAt) > gc.ttl {
		// Expired
		delete(gc.items, key)
		gc.order.Remove(elem)
		return GuidanceResponse{}, false
	}

	// Move to front (LRU)
	gc.order.MoveToFront(elem)
	return entry.response, true
}

// Put stores a guidance response in the cache.
func (gc *GuidanceCache) Put(spaceID, context string, resp GuidanceResponse) {
	key := cacheKey(spaceID, context)

	gc.mu.Lock()
	defer gc.mu.Unlock()

	// Update existing
	if elem, ok := gc.items[key]; ok {
		gc.order.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.response = resp
		entry.createdAt = time.Now()
		return
	}

	// Evict if at capacity
	for gc.order.Len() >= gc.capacity {
		oldest := gc.order.Back()
		if oldest == nil {
			break
		}
		oldEntry := oldest.Value.(*cacheEntry)
		delete(gc.items, oldEntry.key)
		gc.order.Remove(oldest)
	}

	entry := &cacheEntry{
		key:       key,
		response:  resp,
		createdAt: time.Now(),
	}
	elem := gc.order.PushFront(entry)
	gc.items[key] = elem
}

// Size returns the number of cached entries.
func (gc *GuidanceCache) Size() int {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	return len(gc.items)
}

// Invalidate removes a specific entry from the cache.
func (gc *GuidanceCache) Invalidate(spaceID, context string) {
	key := cacheKey(spaceID, context)
	gc.mu.Lock()
	defer gc.mu.Unlock()
	if elem, ok := gc.items[key]; ok {
		delete(gc.items, key)
		gc.order.Remove(elem)
	}
}

// Clear removes all entries from the cache.
func (gc *GuidanceCache) Clear() {
	gc.mu.Lock()
	defer gc.mu.Unlock()
	gc.items = make(map[string]*list.Element, gc.capacity)
	gc.order.Init()
}
