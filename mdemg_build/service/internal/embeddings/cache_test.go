package embeddings

import (
	"context"
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

// mockEmbedder is a test embedder that tracks call counts and returns predictable embeddings.
type mockEmbedder struct {
	mu              sync.Mutex
	callCount       int
	batchCallCount  int
	dimensions      int
	name            string
	embedFunc       func(text string) ([]float32, error)
	embedBatchFunc  func(texts []string) ([][]float32, error)
}

func newMockEmbedder(name string, dims int) *mockEmbedder {
	return &mockEmbedder{
		dimensions: dims,
		name:       name,
	}
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	if m.embedFunc != nil {
		return m.embedFunc(text)
	}

	// Default: return predictable embedding based on text length
	result := make([]float32, m.dimensions)
	for i := range result {
		result[i] = float32(len(text)) * 0.01
	}
	return result, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.batchCallCount++
	m.mu.Unlock()

	if m.embedBatchFunc != nil {
		return m.embedBatchFunc(texts)
	}

	// Default: embed each text individually
	results := make([][]float32, len(texts))
	for i, text := range texts {
		result := make([]float32, m.dimensions)
		for j := range result {
			result[j] = float32(len(text)) * 0.01
		}
		results[i] = result
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int {
	return m.dimensions
}

func (m *mockEmbedder) Name() string {
	return m.name
}

func (m *mockEmbedder) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockEmbedder) getBatchCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.batchCallCount
}

// TestCachedEmbedder tests the CachedEmbedder integration with cache hits
func TestCachedEmbedder(t *testing.T) {
	t.Run("cache hit avoids underlying embedder call", func(t *testing.T) {
		mock := newMockEmbedder("test-provider", 128)
		cached := NewCachedEmbedder(mock, 10)

		ctx := context.Background()
		text := "test input text"

		// First call - should hit the mock embedder
		emb1, err := cached.Embed(ctx, text)
		if err != nil {
			t.Fatalf("first embed failed: %v", err)
		}
		if len(emb1) != 128 {
			t.Errorf("embedding dimensions = %d, expected 128", len(emb1))
		}
		if mock.getCallCount() != 1 {
			t.Errorf("mock call count after first call = %d, expected 1", mock.getCallCount())
		}

		// Second call with same text - should hit cache, not call mock
		emb2, err := cached.Embed(ctx, text)
		if err != nil {
			t.Fatalf("second embed failed: %v", err)
		}
		if len(emb2) != 128 {
			t.Errorf("embedding dimensions = %d, expected 128", len(emb2))
		}
		if mock.getCallCount() != 1 {
			t.Errorf("mock call count after cache hit = %d, expected 1 (not 2)", mock.getCallCount())
		}

		// Verify embeddings are identical
		for i := range emb1 {
			if emb1[i] != emb2[i] {
				t.Errorf("embedding mismatch at index %d: first=%f, second=%f", i, emb1[i], emb2[i])
			}
		}

		// Third call with different text - should call mock again
		emb3, err := cached.Embed(ctx, "different text")
		if err != nil {
			t.Fatalf("third embed failed: %v", err)
		}
		if len(emb3) != 128 {
			t.Errorf("embedding dimensions = %d, expected 128", len(emb3))
		}
		if mock.getCallCount() != 2 {
			t.Errorf("mock call count after different text = %d, expected 2", mock.getCallCount())
		}
	})

	t.Run("cache respects provider name in key", func(t *testing.T) {
		mock1 := newMockEmbedder("provider-A", 64)
		mock2 := newMockEmbedder("provider-B", 64)

		cached1 := NewCachedEmbedder(mock1, 10)
		cached2 := NewCachedEmbedder(mock2, 10)

		ctx := context.Background()
		text := "same text"

		// Call both embedders with same text
		_, err := cached1.Embed(ctx, text)
		if err != nil {
			t.Fatalf("cached1 embed failed: %v", err)
		}
		_, err = cached2.Embed(ctx, text)
		if err != nil {
			t.Fatalf("cached2 embed failed: %v", err)
		}

		// Both should have called their underlying embedders (different cache keys)
		if mock1.getCallCount() != 1 {
			t.Errorf("mock1 call count = %d, expected 1", mock1.getCallCount())
		}
		if mock2.getCallCount() != 1 {
			t.Errorf("mock2 call count = %d, expected 1", mock2.getCallCount())
		}
	})

	t.Run("Name method includes cache indicator", func(t *testing.T) {
		mock := newMockEmbedder("my-provider", 128)
		cached := NewCachedEmbedder(mock, 10)

		name := cached.Name()
		expected := "my-provider+cache"
		if name != expected {
			t.Errorf("cached name = %q, expected %q", name, expected)
		}
	})

	t.Run("Dimensions method returns underlying dimensions", func(t *testing.T) {
		mock := newMockEmbedder("test-provider", 256)
		cached := NewCachedEmbedder(mock, 10)

		dims := cached.Dimensions()
		if dims != 256 {
			t.Errorf("dimensions = %d, expected 256", dims)
		}
	})

	t.Run("batch operations use cache", func(t *testing.T) {
		mock := newMockEmbedder("batch-provider", 128)
		cached := NewCachedEmbedder(mock, 10)

		ctx := context.Background()
		texts := []string{"text1", "text2", "text3"}

		// First batch call - all misses
		embs1, err := cached.EmbedBatch(ctx, texts)
		if err != nil {
			t.Fatalf("first batch embed failed: %v", err)
		}
		if len(embs1) != 3 {
			t.Fatalf("batch result length = %d, expected 3", len(embs1))
		}
		if mock.getBatchCallCount() != 1 {
			t.Errorf("mock batch call count = %d, expected 1", mock.getBatchCallCount())
		}

		// Second batch call with same texts - all hits, no mock call
		embs2, err := cached.EmbedBatch(ctx, texts)
		if err != nil {
			t.Fatalf("second batch embed failed: %v", err)
		}
		if len(embs2) != 3 {
			t.Fatalf("batch result length = %d, expected 3", len(embs2))
		}
		if mock.getBatchCallCount() != 1 {
			t.Errorf("mock batch call count after cache hits = %d, expected 1 (not 2)", mock.getBatchCallCount())
		}

		// Verify embeddings match
		for i := range embs1 {
			for j := range embs1[i] {
				if embs1[i][j] != embs2[i][j] {
					t.Errorf("embedding mismatch at [%d][%d]: first=%f, second=%f", i, j, embs1[i][j], embs2[i][j])
				}
			}
		}
	})

	t.Run("batch with mixed hits and misses", func(t *testing.T) {
		mock := newMockEmbedder("mixed-provider", 128)
		cached := NewCachedEmbedder(mock, 10)

		ctx := context.Background()

		// Pre-populate cache with some texts
		_, err := cached.Embed(ctx, "cached1")
		if err != nil {
			t.Fatalf("pre-populate failed: %v", err)
		}
		_, err = cached.Embed(ctx, "cached2")
		if err != nil {
			t.Fatalf("pre-populate failed: %v", err)
		}

		// Reset counters
		mock.mu.Lock()
		mock.callCount = 0
		mock.batchCallCount = 0
		mock.mu.Unlock()

		// Batch with mix of cached and new texts
		texts := []string{"cached1", "new1", "cached2", "new2"}
		embs, err := cached.EmbedBatch(ctx, texts)
		if err != nil {
			t.Fatalf("mixed batch embed failed: %v", err)
		}
		if len(embs) != 4 {
			t.Fatalf("batch result length = %d, expected 4", len(embs))
		}

		// Should only call batch embedder for the 2 new texts
		if mock.getBatchCallCount() != 1 {
			t.Errorf("mock batch call count = %d, expected 1 (for 2 misses)", mock.getBatchCallCount())
		}

		// All results should be valid
		for i, emb := range embs {
			if len(emb) != 128 {
				t.Errorf("embedding[%d] dimensions = %d, expected 128", i, len(emb))
			}
		}
	})

	t.Run("cache eviction behavior with CachedEmbedder", func(t *testing.T) {
		mock := newMockEmbedder("eviction-provider", 64)
		cached := NewCachedEmbedder(mock, 2) // Small cache

		ctx := context.Background()

		// Fill cache
		_, err := cached.Embed(ctx, "text1")
		if err != nil {
			t.Fatalf("embed text1 failed: %v", err)
		}
		_, err = cached.Embed(ctx, "text2")
		if err != nil {
			t.Fatalf("embed text2 failed: %v", err)
		}

		// At this point, callCount should be 2
		if mock.getCallCount() != 2 {
			t.Fatalf("call count after filling cache = %d, expected 2", mock.getCallCount())
		}

		// Add third item, should evict text1
		_, err = cached.Embed(ctx, "text3")
		if err != nil {
			t.Fatalf("embed text3 failed: %v", err)
		}

		// callCount should be 3
		if mock.getCallCount() != 3 {
			t.Fatalf("call count after eviction = %d, expected 3", mock.getCallCount())
		}

		// Re-embed text1 - should be cache miss (was evicted)
		_, err = cached.Embed(ctx, "text1")
		if err != nil {
			t.Fatalf("re-embed text1 failed: %v", err)
		}
		if mock.getCallCount() != 4 {
			t.Errorf("call count after re-embedding evicted text = %d, expected 4", mock.getCallCount())
		}

		// Re-embed text2 - should be cache hit (still in cache)
		_, err = cached.Embed(ctx, "text2")
		if err != nil {
			t.Fatalf("re-embed text2 failed: %v", err)
		}
		if mock.getCallCount() != 4 {
			t.Errorf("call count after cache hit = %d, expected 4 (not 5)", mock.getCallCount())
		}
	})
}
