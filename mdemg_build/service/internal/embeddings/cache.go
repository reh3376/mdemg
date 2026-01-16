package embeddings

import (
	"container/list"
	"sync"
)

// EmbeddingCache is a thread-safe LRU cache for embedding results.
// It uses a doubly-linked list for LRU tracking and a map for O(1) lookup.
type EmbeddingCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*list.Element
	lruList  *list.List
}

// cacheEntry holds a key-value pair in the LRU list.
type cacheEntry struct {
	key   string
	value []float32
}

// NewEmbeddingCache creates a new LRU cache with the specified capacity.
// Capacity must be greater than 0.
func NewEmbeddingCache(capacity int) *EmbeddingCache {
	if capacity <= 0 {
		capacity = 1000 // default capacity
	}
	return &EmbeddingCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

// Get retrieves a value from the cache.
// Returns (value, true) if found, (nil, false) if not found.
// On cache hit, the item is moved to the front (most recently used).
func (c *EmbeddingCache) Get(key string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[key]
	if !found {
		return nil, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)

	entry := elem.Value.(*cacheEntry)
	// Return a copy to prevent external mutations
	result := make([]float32, len(entry.value))
	copy(result, entry.value)
	return result, true
}

// Put adds or updates a value in the cache.
// If the key already exists, it updates the value and moves it to front.
// If adding a new key would exceed capacity, the least recently used item is evicted.
func (c *EmbeddingCache) Put(key string, value []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store a copy to prevent external mutations
	valueCopy := make([]float32, len(value))
	copy(valueCopy, value)

	// Check if key already exists
	if elem, found := c.items[key]; found {
		// Update existing entry and move to front
		c.lruList.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.value = valueCopy
		return
	}

	// Add new entry
	entry := &cacheEntry{
		key:   key,
		value: valueCopy,
	}
	elem := c.lruList.PushFront(entry)
	c.items[key] = elem

	// Evict least recently used if over capacity
	if c.lruList.Len() > c.capacity {
		c.evictOldest()
	}
}

// Clear removes all entries from the cache.
func (c *EmbeddingCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lruList.Init()
}

// Len returns the current number of items in the cache.
func (c *EmbeddingCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// evictOldest removes the least recently used item from the cache.
// Must be called with mutex locked.
func (c *EmbeddingCache) evictOldest() {
	elem := c.lruList.Back()
	if elem == nil {
		return
	}

	c.lruList.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
}
