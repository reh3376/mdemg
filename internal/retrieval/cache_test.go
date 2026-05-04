package retrieval

import (
	"testing"
	"time"

	"mdemg/internal/models"
)

func TestNewQueryCache(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cache := NewQueryCache(0, 0)
		if cache == nil {
			t.Fatal("expected non-nil cache")
		}
		if cache.capacity != 500 {
			t.Errorf("expected default capacity 500, got %d", cache.capacity)
		}
		if cache.ttl != 5*time.Minute {
			t.Errorf("expected default TTL 5m, got %v", cache.ttl)
		}
	})

	t.Run("custom values", func(t *testing.T) {
		cache := NewQueryCache(100, 1*time.Minute)
		if cache.capacity != 100 {
			t.Errorf("expected capacity 100, got %d", cache.capacity)
		}
		if cache.ttl != 1*time.Minute {
			t.Errorf("expected TTL 1m, got %v", cache.ttl)
		}
	})
}

func TestCacheKey(t *testing.T) {
	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "test"}
	req2 := models.RetrieveRequest{SpaceID: "space1", QueryText: "test"}
	req3 := models.RetrieveRequest{SpaceID: "space2", QueryText: "test"}

	key1 := CacheKey(req1, "")
	key2 := CacheKey(req2, "")
	key3 := CacheKey(req3, "")

	if key1 != key2 {
		t.Error("same request should produce same key")
	}
	if key1 == key3 {
		t.Error("different requests should produce different keys")
	}
	if len(key1) != 32 {
		t.Errorf("expected 32 char hex key, got %d", len(key1))
	}
}

func TestQueryCache_PutAndGet(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)

	req := models.RetrieveRequest{SpaceID: "space1", QueryText: "test query"}
	resp := models.RetrieveResponse{
		Results: []models.RetrieveResult{{NodeID: "node1", Score: 0.9}},
	}

	// Put a cache entry
	cache.Put(req, "", resp)

	// Get it back
	got, found := cache.Get(req, "")
	if !found {
		t.Fatal("expected to find cached entry")
	}
	if len(got.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(got.Results))
	}
	if got.Results[0].NodeID != "node1" {
		t.Errorf("expected node1, got %s", got.Results[0].NodeID)
	}
}

func TestQueryCache_Len(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)

	if cache.Len() != 0 {
		t.Errorf("expected empty cache, got %d", cache.Len())
	}

	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query1"}
	cache.Put(req1, "", models.RetrieveResponse{})
	if cache.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", cache.Len())
	}

	req2 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query2"}
	cache.Put(req2, "", models.RetrieveResponse{})
	if cache.Len() != 2 {
		t.Errorf("expected 2 entries, got %d", cache.Len())
	}
}

func TestQueryCache_Clear(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)

	// Add some entries
	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query1"}
	req2 := models.RetrieveRequest{SpaceID: "space2", QueryText: "query2"}
	req3 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query3"}
	cache.Put(req1, "", models.RetrieveResponse{})
	cache.Put(req2, "", models.RetrieveResponse{})
	cache.Put(req3, "", models.RetrieveResponse{})

	if cache.Len() != 3 {
		t.Errorf("expected 3 entries before clear, got %d", cache.Len())
	}

	// Clear
	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", cache.Len())
	}

	// Verify entries are gone
	_, found := cache.Get(req1, "")
	if found {
		t.Error("expected req1 to be cleared")
	}
}

func TestQueryCache_InvalidateSpace(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)

	// Add entries for different spaces
	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query1"}
	req2 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query2"}
	req3 := models.RetrieveRequest{SpaceID: "space2", QueryText: "query3"}
	cache.Put(req1, "", models.RetrieveResponse{})
	cache.Put(req2, "", models.RetrieveResponse{})
	cache.Put(req3, "", models.RetrieveResponse{})

	if cache.Len() != 3 {
		t.Errorf("expected 3 entries before invalidate, got %d", cache.Len())
	}

	// Invalidate space1
	removed := cache.InvalidateSpace("space1")
	if removed != 2 {
		t.Errorf("expected 2 entries removed, got %d", removed)
	}

	if cache.Len() != 1 {
		t.Errorf("expected 1 entry after invalidate, got %d", cache.Len())
	}

	// Verify space1 entries are gone
	_, found := cache.Get(req1, "")
	if found {
		t.Error("expected req1 to be invalidated")
	}

	// Verify space2 entry remains
	_, found = cache.Get(req3, "")
	if !found {
		t.Error("expected req3 to remain")
	}
}

func TestQueryCache_Stats(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)

	// Add entry and access it
	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query1"}
	req2 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query2"}
	cache.Put(req1, "", models.RetrieveResponse{})
	cache.Get(req1, "") // hit
	cache.Get(req2, "") // miss

	stats := cache.Stats()

	if stats["size"].(int) != 1 {
		t.Errorf("expected size 1, got %v", stats["size"])
	}
	if stats["capacity"].(int) != 10 {
		t.Errorf("expected capacity 10, got %v", stats["capacity"])
	}
	if stats["hits"].(int64) != 1 {
		t.Errorf("expected 1 hit, got %v", stats["hits"])
	}
	if stats["misses"].(int64) != 1 {
		t.Errorf("expected 1 miss, got %v", stats["misses"])
	}
}

func TestQueryCache_Expiration(t *testing.T) {
	// Create cache with 10ms TTL for fast testing
	cache := NewQueryCache(10, 10*time.Millisecond)

	req := models.RetrieveRequest{SpaceID: "space1", QueryText: "test"}
	cache.Put(req, "", models.RetrieveResponse{Results: []models.RetrieveResult{{NodeID: "node1"}}})

	// Should find it immediately
	got, found := cache.Get(req, "")
	if !found {
		t.Fatal("expected to find cached entry immediately")
	}
	if len(got.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(got.Results))
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should not find it after expiration
	_, found = cache.Get(req, "")
	if found {
		t.Error("expected entry to be expired")
	}
}

func TestQueryCache_LRUEviction(t *testing.T) {
	// Create cache with capacity 3
	cache := NewQueryCache(3, 5*time.Minute)

	// Fill the cache
	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query1"}
	req2 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query2"}
	req3 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query3"}
	cache.Put(req1, "", models.RetrieveResponse{})
	cache.Put(req2, "", models.RetrieveResponse{})
	cache.Put(req3, "", models.RetrieveResponse{})

	// Access req1 to make it recently used
	cache.Get(req1, "")

	// Add req4 - should evict req2 (least recently used)
	req4 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query4"}
	cache.Put(req4, "", models.RetrieveResponse{})

	if cache.Len() != 3 {
		t.Errorf("expected 3 entries after eviction, got %d", cache.Len())
	}

	// req2 should be evicted
	_, found := cache.Get(req2, "")
	if found {
		t.Error("expected req2 to be evicted")
	}

	// req1 should remain
	_, found = cache.Get(req1, "")
	if !found {
		t.Error("expected req1 to remain (was recently accessed)")
	}
}

func TestQueryCache_ConcurrentAccess(t *testing.T) {
	cache := NewQueryCache(100, 5*time.Minute)
	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				req := models.RetrieveRequest{
					SpaceID:   "space1",
					QueryText: string(rune('a'+id)) + string(rune('0'+j%10)),
				}
				cache.Put(req, "", models.RetrieveResponse{})
			}
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				req := models.RetrieveRequest{
					SpaceID:   "space1",
					QueryText: string(rune('a'+id)) + string(rune('0'+j%10)),
				}
				cache.Get(req, "")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	// Should not panic and should have some entries
	if cache.Len() == 0 {
		t.Error("expected some entries after concurrent access")
	}
}

func TestQueryCache_UpdateExistingEntry(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)

	req := models.RetrieveRequest{SpaceID: "space1", QueryText: "test"}
	resp1 := models.RetrieveResponse{Results: []models.RetrieveResult{{NodeID: "node1"}}}
	resp2 := models.RetrieveResponse{Results: []models.RetrieveResult{{NodeID: "node2"}}}

	cache.Put(req, "", resp1)
	if cache.Len() != 1 {
		t.Errorf("expected 1 entry, got %d", cache.Len())
	}

	// Update with new response
	cache.Put(req, "", resp2)
	if cache.Len() != 1 {
		t.Errorf("expected still 1 entry after update, got %d", cache.Len())
	}

	// Should get the updated response
	got, found := cache.Get(req, "")
	if !found {
		t.Fatal("expected to find cached entry")
	}
	if got.Results[0].NodeID != "node2" {
		t.Errorf("expected updated result 'node2', got '%s'", got.Results[0].NodeID)
	}
}

// Service-level tests for ClearQueryCache and InvalidateSpaceCache

func TestService_ClearQueryCache_NilCache(t *testing.T) {
	// Service without cache initialized
	svc := &Service{queryCache: nil}
	count := svc.ClearQueryCache()
	if count != 0 {
		t.Errorf("expected 0 for nil cache, got %d", count)
	}
}

func TestService_ClearQueryCache_WithEntries(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)
	svc := &Service{queryCache: cache}

	// Add some entries
	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query1"}
	req2 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query2"}
	cache.Put(req1, "", models.RetrieveResponse{})
	cache.Put(req2, "", models.RetrieveResponse{})

	count := svc.ClearQueryCache()
	if count != 2 {
		t.Errorf("expected 2 entries cleared, got %d", count)
	}

	if cache.Len() != 0 {
		t.Errorf("expected cache to be empty, got %d", cache.Len())
	}
}

func TestService_InvalidateSpaceCache_NilCache(t *testing.T) {
	// Service without cache initialized
	svc := &Service{queryCache: nil}
	count := svc.InvalidateSpaceCache("space1")
	if count != 0 {
		t.Errorf("expected 0 for nil cache, got %d", count)
	}
}

func TestService_InvalidateSpaceCache_WithEntries(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)
	svc := &Service{queryCache: cache}

	// Add entries for different spaces
	req1 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query1"}
	req2 := models.RetrieveRequest{SpaceID: "space1", QueryText: "query2"}
	req3 := models.RetrieveRequest{SpaceID: "space2", QueryText: "query3"}
	cache.Put(req1, "", models.RetrieveResponse{})
	cache.Put(req2, "", models.RetrieveResponse{})
	cache.Put(req3, "", models.RetrieveResponse{})

	count := svc.InvalidateSpaceCache("space1")
	if count != 2 {
		t.Errorf("expected 2 entries invalidated, got %d", count)
	}

	if cache.Len() != 1 {
		t.Errorf("expected 1 entry remaining, got %d", cache.Len())
	}
}
