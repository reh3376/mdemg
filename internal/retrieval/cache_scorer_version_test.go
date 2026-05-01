package retrieval

import (
	"testing"
	"time"

	"mdemg/internal/models"
)

// TestCacheKey_ScorerVersionAffectsKey verifies the Phase 13 cache-key
// extension: identical requests under different scorer-version namespaces
// produce different keys, isolating legacy from RRF entries.
func TestCacheKey_ScorerVersionAffectsKey(t *testing.T) {
	req := models.RetrieveRequest{SpaceID: "mdemg-dev", QueryText: "test"}
	keyLegacy := CacheKey(req, "v0-linear")
	keyRRF := CacheKey(req, "v1-rrf4")
	if keyLegacy == keyRRF {
		t.Errorf("scorer_version=v0-linear and v1-rrf4 produced same key %q — Phase 13 cache isolation broken", keyLegacy)
	}
}

// TestCacheKey_EmptyVersionPreservesLegacyNamespace documents the contract
// that scorerVersion="" reserves a stable namespace for callers that don't
// care (e.g., tests, pre-Phase-13 callers).
func TestCacheKey_EmptyVersionPreservesLegacyNamespace(t *testing.T) {
	req := models.RetrieveRequest{SpaceID: "x", QueryText: "y"}
	k1 := CacheKey(req, "")
	k2 := CacheKey(req, "")
	if k1 != k2 {
		t.Errorf("CacheKey with same args + empty version not deterministic: %q vs %q", k1, k2)
	}
}

// TestQueryCache_ScorerVersionIsolation verifies that a Put under one
// scorer_version is not retrievable under another — the core cache-pollution
// guard for the Phase 13 A/B harness.
func TestQueryCache_ScorerVersionIsolation(t *testing.T) {
	cache := NewQueryCache(10, 5*time.Minute)
	req := models.RetrieveRequest{SpaceID: "x", QueryText: "y"}
	respLegacy := models.RetrieveResponse{
		Results: []models.RetrieveResult{{NodeID: "legacy-1", Score: 0.5}},
	}
	respRRF := models.RetrieveResponse{
		Results: []models.RetrieveResult{{NodeID: "rrf-1", Score: 0.9}},
	}

	cache.Put(req, "v0-linear", respLegacy)
	cache.Put(req, "v1-rrf4", respRRF)

	got, ok := cache.Get(req, "v0-linear")
	if !ok || len(got.Results) != 1 || got.Results[0].NodeID != "legacy-1" {
		t.Errorf("v0-linear lookup returned wrong entry: ok=%v results=%+v", ok, got.Results)
	}
	got, ok = cache.Get(req, "v1-rrf4")
	if !ok || len(got.Results) != 1 || got.Results[0].NodeID != "rrf-1" {
		t.Errorf("v1-rrf4 lookup returned wrong entry: ok=%v results=%+v", ok, got.Results)
	}

	// A different version should miss entirely.
	_, ok = cache.Get(req, "v99-future")
	if ok {
		t.Error("v99-future lookup should miss; got hit")
	}
}
