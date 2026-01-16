package embeddings

import (
	"sync"
	"testing"
)

// TestNewEmbeddingCache tests the cache constructor
func TestNewEmbeddingCache(t *testing.T) {
	tests := []struct {
		name             string
		capacity         int
		expectedCapacity int
	}{
		{"valid capacity", 100, 100},
		{"zero capacity uses default", 0, 1000},
		{"negative capacity uses default", -10, 1000},
		{"capacity of 1", 1, 1},
		{"large capacity", 10000, 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewEmbeddingCache(tt.capacity)
			if cache == nil {
				t.Fatal("NewEmbeddingCache returned nil")
			}
			if cache.capacity != tt.expectedCapacity {
				t.Errorf("capacity = %d, expected %d", cache.capacity, tt.expectedCapacity)
			}
			if cache.items == nil {
				t.Error("items map not initialized")
			}
			if cache.lruList == nil {
				t.Error("lruList not initialized")
			}
			if cache.Len() != 0 {
				t.Errorf("new cache length = %d, expected 0", cache.Len())
			}
		})
	}
}

// TestEmbeddingCacheGetMiss tests cache misses
func TestEmbeddingCacheGetMiss(t *testing.T) {
	cache := NewEmbeddingCache(10)

	val, found := cache.Get("nonexistent")
	if found {
		t.Error("expected cache miss, got hit")
	}
	if val != nil {
		t.Errorf("expected nil value on miss, got %v", val)
	}
}

// TestEmbeddingCachePutAndGet tests basic put and get operations
func TestEmbeddingCachePutAndGet(t *testing.T) {
	cache := NewEmbeddingCache(10)

	// Put a value
	key := "test_key"
	value := []float32{0.1, 0.2, 0.3}
	cache.Put(key, value)

	// Verify cache size
	if cache.Len() != 1 {
		t.Errorf("cache length = %d, expected 1", cache.Len())
	}

	// Get the value
	retrieved, found := cache.Get(key)
	if !found {
		t.Fatal("expected cache hit, got miss")
	}

	// Verify value matches
	if len(retrieved) != len(value) {
		t.Fatalf("retrieved length = %d, expected %d", len(retrieved), len(value))
	}
	for i := range value {
		if retrieved[i] != value[i] {
			t.Errorf("retrieved[%d] = %f, expected %f", i, retrieved[i], value[i])
		}
	}
}

// TestEmbeddingCachePutUpdate tests updating an existing key
func TestEmbeddingCachePutUpdate(t *testing.T) {
	cache := NewEmbeddingCache(10)

	key := "update_key"
	value1 := []float32{0.1, 0.2}
	value2 := []float32{0.3, 0.4, 0.5}

	// Put initial value
	cache.Put(key, value1)
	if cache.Len() != 1 {
		t.Errorf("cache length after first put = %d, expected 1", cache.Len())
	}

	// Update with new value
	cache.Put(key, value2)
	if cache.Len() != 1 {
		t.Errorf("cache length after update = %d, expected 1 (not 2)", cache.Len())
	}

	// Verify updated value
	retrieved, found := cache.Get(key)
	if !found {
		t.Fatal("expected cache hit after update")
	}
	if len(retrieved) != len(value2) {
		t.Fatalf("retrieved length = %d, expected %d", len(retrieved), len(value2))
	}
	for i := range value2 {
		if retrieved[i] != value2[i] {
			t.Errorf("retrieved[%d] = %f, expected %f", i, retrieved[i], value2[i])
		}
	}
}

// TestEmbeddingCacheLRUEviction tests that least recently used items are evicted
func TestEmbeddingCacheLRUEviction(t *testing.T) {
	capacity := 3
	cache := NewEmbeddingCache(capacity)

	// Fill cache to capacity
	cache.Put("key1", []float32{0.1})
	cache.Put("key2", []float32{0.2})
	cache.Put("key3", []float32{0.3})

	if cache.Len() != 3 {
		t.Fatalf("cache length = %d, expected 3", cache.Len())
	}

	// Add one more item, should evict key1 (oldest)
	cache.Put("key4", []float32{0.4})

	if cache.Len() != 3 {
		t.Errorf("cache length after eviction = %d, expected 3", cache.Len())
	}

	// key1 should be evicted
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should have been evicted")
	}

	// Other keys should still exist
	for _, key := range []string{"key2", "key3", "key4"} {
		if _, found := cache.Get(key); !found {
			t.Errorf("key %s should still be in cache", key)
		}
	}
}

// TestEmbeddingCacheLRUOrderWithGet tests that Get updates LRU order
func TestEmbeddingCacheLRUOrderWithGet(t *testing.T) {
	cache := NewEmbeddingCache(3)

	// Fill cache
	cache.Put("key1", []float32{0.1})
	cache.Put("key2", []float32{0.2})
	cache.Put("key3", []float32{0.3})

	// Access key1 to make it most recently used
	_, _ = cache.Get("key1")

	// Add new item, should evict key2 (now oldest)
	cache.Put("key4", []float32{0.4})

	// key2 should be evicted
	_, found := cache.Get("key2")
	if found {
		t.Error("key2 should have been evicted")
	}

	// key1 should still exist (was accessed)
	_, found = cache.Get("key1")
	if !found {
		t.Error("key1 should still be in cache after being accessed")
	}
}

// TestEmbeddingCacheLRUOrderWithPutUpdate tests that updating a key moves it to front
func TestEmbeddingCacheLRUOrderWithPutUpdate(t *testing.T) {
	cache := NewEmbeddingCache(3)

	// Fill cache
	cache.Put("key1", []float32{0.1})
	cache.Put("key2", []float32{0.2})
	cache.Put("key3", []float32{0.3})

	// Update key1 to make it most recently used
	cache.Put("key1", []float32{0.1, 0.1})

	// Add new item, should evict key2 (now oldest)
	cache.Put("key4", []float32{0.4})

	// key2 should be evicted
	_, found := cache.Get("key2")
	if found {
		t.Error("key2 should have been evicted")
	}

	// key1 should still exist (was updated)
	val, found := cache.Get("key1")
	if !found {
		t.Error("key1 should still be in cache after being updated")
	}
	if len(val) != 2 {
		t.Errorf("key1 value length = %d, expected 2 (updated value)", len(val))
	}
}

// TestEmbeddingCacheClear tests clearing the cache
func TestEmbeddingCacheClear(t *testing.T) {
	cache := NewEmbeddingCache(10)

	// Add some items
	cache.Put("key1", []float32{0.1})
	cache.Put("key2", []float32{0.2})
	cache.Put("key3", []float32{0.3})

	if cache.Len() != 3 {
		t.Fatalf("cache length before clear = %d, expected 3", cache.Len())
	}

	// Clear cache
	cache.Clear()

	// Verify cache is empty
	if cache.Len() != 0 {
		t.Errorf("cache length after clear = %d, expected 0", cache.Len())
	}

	// Verify items are gone
	for _, key := range []string{"key1", "key2", "key3"} {
		if _, found := cache.Get(key); found {
			t.Errorf("key %s should not exist after clear", key)
		}
	}

	// Verify cache still works after clear
	cache.Put("new_key", []float32{0.9})
	if cache.Len() != 1 {
		t.Errorf("cache length after adding to cleared cache = %d, expected 1", cache.Len())
	}
}

// TestEmbeddingCacheValueIsolation tests that returned values are copies
func TestEmbeddingCacheValueIsolation(t *testing.T) {
	cache := NewEmbeddingCache(10)

	key := "isolation_test"
	original := []float32{0.1, 0.2, 0.3}
	cache.Put(key, original)

	// Get value and modify it
	retrieved1, _ := cache.Get(key)
	retrieved1[0] = 0.999

	// Get value again, should be unchanged
	retrieved2, found := cache.Get(key)
	if !found {
		t.Fatal("expected cache hit")
	}
	if retrieved2[0] != 0.1 {
		t.Errorf("value was mutated: retrieved2[0] = %f, expected 0.1", retrieved2[0])
	}

	// Modify original, cache should be unchanged
	original[1] = 0.888
	retrieved3, _ := cache.Get(key)
	if retrieved3[1] != 0.2 {
		t.Errorf("cache was affected by original mutation: retrieved3[1] = %f, expected 0.2", retrieved3[1])
	}
}

// TestEmbeddingCacheEmptyValue tests putting and getting empty slices
func TestEmbeddingCacheEmptyValue(t *testing.T) {
	cache := NewEmbeddingCache(10)

	key := "empty_key"
	emptyValue := []float32{}

	cache.Put(key, emptyValue)
	if cache.Len() != 1 {
		t.Errorf("cache length = %d, expected 1", cache.Len())
	}

	retrieved, found := cache.Get(key)
	if !found {
		t.Error("expected cache hit for empty value")
	}
	if len(retrieved) != 0 {
		t.Errorf("retrieved length = %d, expected 0", len(retrieved))
	}
}

// TestEmbeddingCacheNilValue tests putting nil values
func TestEmbeddingCacheNilValue(t *testing.T) {
	cache := NewEmbeddingCache(10)

	key := "nil_key"
	cache.Put(key, nil)

	if cache.Len() != 1 {
		t.Errorf("cache length = %d, expected 1", cache.Len())
	}

	retrieved, found := cache.Get(key)
	if !found {
		t.Error("expected cache hit for nil value")
	}
	if len(retrieved) != 0 {
		t.Errorf("retrieved length = %d, expected 0", len(retrieved))
	}
}

// TestEmbeddingCacheCapacityOne tests edge case of capacity=1
func TestEmbeddingCacheCapacityOne(t *testing.T) {
	cache := NewEmbeddingCache(1)

	cache.Put("key1", []float32{0.1})
	if cache.Len() != 1 {
		t.Errorf("cache length = %d, expected 1", cache.Len())
	}

	// Add second item, should evict first
	cache.Put("key2", []float32{0.2})
	if cache.Len() != 1 {
		t.Errorf("cache length = %d, expected 1", cache.Len())
	}

	// key1 should be gone
	_, found := cache.Get("key1")
	if found {
		t.Error("key1 should have been evicted")
	}

	// key2 should exist
	_, found = cache.Get("key2")
	if !found {
		t.Error("key2 should be in cache")
	}
}

// TestEmbeddingCacheLargeValues tests caching large embedding vectors
func TestEmbeddingCacheLargeValues(t *testing.T) {
	cache := NewEmbeddingCache(10)

	// Simulate a 1536-dimensional embedding (OpenAI ada-002 size)
	largeValue := make([]float32, 1536)
	for i := range largeValue {
		largeValue[i] = float32(i) * 0.001
	}

	key := "large_embedding"
	cache.Put(key, largeValue)

	retrieved, found := cache.Get(key)
	if !found {
		t.Fatal("expected cache hit for large value")
	}

	if len(retrieved) != len(largeValue) {
		t.Fatalf("retrieved length = %d, expected %d", len(retrieved), len(largeValue))
	}

	// Verify a few values
	if retrieved[0] != largeValue[0] {
		t.Errorf("retrieved[0] = %f, expected %f", retrieved[0], largeValue[0])
	}
	if retrieved[1535] != largeValue[1535] {
		t.Errorf("retrieved[1535] = %f, expected %f", retrieved[1535], largeValue[1535])
	}
}

// TestEmbeddingCacheConcurrentAccess tests thread safety with concurrent operations
func TestEmbeddingCacheConcurrentAccess(t *testing.T) {
	cache := NewEmbeddingCache(100)
	numGoroutines := 10
	operationsPerGoroutine := 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // readers, writers, and updaters

	// Concurrent writers
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < operationsPerGoroutine; i++ {
				key := "key_" + string(rune(id*operationsPerGoroutine+i))
				value := []float32{float32(id), float32(i)}
				cache.Put(key, value)
			}
		}(g)
	}

	// Concurrent readers
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < operationsPerGoroutine; i++ {
				key := "key_" + string(rune(id*operationsPerGoroutine+i))
				cache.Get(key)
			}
		}(g)
	}

	// Concurrent updaters
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < operationsPerGoroutine; i++ {
				key := "key_" + string(rune(id*operationsPerGoroutine+i))
				value := []float32{float32(id + 1), float32(i + 1)}
				cache.Put(key, value)
			}
		}(g)
	}

	wg.Wait()

	// Verify cache is still functional
	cache.Put("test", []float32{1.0})
	val, found := cache.Get("test")
	if !found {
		t.Error("cache not functional after concurrent access")
	}
	if val[0] != 1.0 {
		t.Errorf("cache value incorrect after concurrent access: got %f, expected 1.0", val[0])
	}
}

// TestEmbeddingCacheConcurrentClear tests thread safety of Clear with concurrent operations
func TestEmbeddingCacheConcurrentClear(t *testing.T) {
	cache := NewEmbeddingCache(50)

	var wg sync.WaitGroup
	wg.Add(3)

	// Writer goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cache.Put("key", []float32{float32(i)})
		}
	}()

	// Reader goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			cache.Get("key")
		}
	}()

	// Clear goroutine
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			cache.Clear()
		}
	}()

	wg.Wait()

	// Verify cache is still functional
	cache.Put("final", []float32{9.9})
	if cache.Len() < 1 {
		t.Error("cache not functional after concurrent clear operations")
	}
}

// TestEmbeddingCacheMultipleEvictions tests multiple evictions in sequence
func TestEmbeddingCacheMultipleEvictions(t *testing.T) {
	capacity := 5
	cache := NewEmbeddingCache(capacity)

	// Fill cache
	for i := 0; i < capacity; i++ {
		cache.Put("key"+string(rune(i)), []float32{float32(i)})
	}

	// Add 10 more items, causing 10 evictions
	for i := capacity; i < capacity+10; i++ {
		cache.Put("key"+string(rune(i)), []float32{float32(i)})
	}

	// Cache should still be at capacity
	if cache.Len() != capacity {
		t.Errorf("cache length = %d, expected %d", cache.Len(), capacity)
	}

	// First 10 keys should be gone
	for i := 0; i < 10; i++ {
		if _, found := cache.Get("key" + string(rune(i))); found {
			t.Errorf("key%d should have been evicted", i)
		}
	}

	// Last 5 keys should exist
	for i := 10; i < 15; i++ {
		if _, found := cache.Get("key" + string(rune(i))); !found {
			t.Errorf("key%d should be in cache", i)
		}
	}
}

// TestEmbeddingCacheLen tests the Len method
func TestEmbeddingCacheLen(t *testing.T) {
	cache := NewEmbeddingCache(10)

	if cache.Len() != 0 {
		t.Errorf("new cache length = %d, expected 0", cache.Len())
	}

	cache.Put("key1", []float32{0.1})
	if cache.Len() != 1 {
		t.Errorf("cache length after 1 put = %d, expected 1", cache.Len())
	}

	cache.Put("key2", []float32{0.2})
	cache.Put("key3", []float32{0.3})
	if cache.Len() != 3 {
		t.Errorf("cache length after 3 puts = %d, expected 3", cache.Len())
	}

	// Update existing key shouldn't change length
	cache.Put("key1", []float32{0.1, 0.1})
	if cache.Len() != 3 {
		t.Errorf("cache length after update = %d, expected 3", cache.Len())
	}

	cache.Clear()
	if cache.Len() != 0 {
		t.Errorf("cache length after clear = %d, expected 0", cache.Len())
	}
}
