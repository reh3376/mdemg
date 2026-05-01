package retrieval

import (
	"testing"
	"time"

	"mdemg/internal/models"
)

// BenchmarkQueryCacheGet benchmarks cache lookup performance
func BenchmarkQueryCacheGet(b *testing.B) {
	cache := NewQueryCache(1000, 300*time.Second)

	// Pre-populate cache with sample entries
	for i := 0; i < 500; i++ {
		req := models.RetrieveRequest{
			SpaceID:   "test-space",
			QueryText: "test query " + string(rune('A'+i%26)),
			TopK:      10,
		}
		resp := models.RetrieveResponse{
			SpaceID: "test-space",
			Results: []models.RetrieveResult{
				{NodeID: "node-1", Score: 0.9},
			},
		}
		cache.Put(req, "", resp)
	}

	searchReq := models.RetrieveRequest{
		SpaceID:   "test-space",
		QueryText: "test query K",
		TopK:      10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(searchReq, "")
	}
}

// BenchmarkQueryCachePut benchmarks cache insertion performance
func BenchmarkQueryCachePut(b *testing.B) {
	cache := NewQueryCache(1000, 300*time.Second)

	req := models.RetrieveRequest{
		SpaceID:   "test-space",
		QueryText: "test query",
		TopK:      10,
	}
	resp := models.RetrieveResponse{
		SpaceID: "test-space",
		Results: []models.RetrieveResult{
			{NodeID: "node-1", Score: 0.9},
			{NodeID: "node-2", Score: 0.8},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(req, "", resp)
	}
}

// BenchmarkCacheKeyGeneration benchmarks cache key generation
func BenchmarkCacheKeyGeneration(b *testing.B) {
	req := models.RetrieveRequest{
		SpaceID:         "test-space",
		QueryText:       "what is the default timeout for HTTP connections in the codebase",
		CandidateK:      100,
		TopK:            10,
		HopDepth:        2,
		IncludeEvidence: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CacheKey(req, "")
	}
}

// BenchmarkRetrieveResultAllocation benchmarks result slice allocation
func BenchmarkRetrieveResultAllocation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := make([]models.RetrieveResult, 0, 100)
		for j := 0; j < 100; j++ {
			results = append(results, models.RetrieveResult{
				NodeID:     "node-" + string(rune(j)),
				Score:      0.9 - float64(j)*0.01,
				VectorSim:  0.85,
				Activation: 0.6,
			})
		}
		_ = results
	}
}

// BenchmarkCandidateAllocation benchmarks candidate slice allocation
func BenchmarkCandidateAllocation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		candidates := make([]Candidate, 0, 100)
		for j := 0; j < 100; j++ {
			candidates = append(candidates, Candidate{
				NodeID:    "node-" + string(rune(j)),
				VectorSim: 0.7 + float64(j%30)*0.01,
			})
		}
		_ = candidates
	}
}

// BenchmarkRRFScoreCalculation benchmarks RRF fusion score calculation
func BenchmarkRRFScoreCalculation(b *testing.B) {
	// Pre-populate rank maps
	vectorRanks := make(map[string]int)
	bm25Ranks := make(map[string]int)

	for i := 0; i < 100; i++ {
		nodeID := "node-" + string(rune(i))
		vectorRanks[nodeID] = i + 1
		bm25Ranks[nodeID] = 100 - i
	}

	k := float64(60) // RRF constant

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fusedScores := make(map[string]float64)
		for nodeID, vRank := range vectorRanks {
			bRank := bm25Ranks[nodeID]
			// RRF formula: sum(1/(k+rank))
			fusedScores[nodeID] = 1.0/(k+float64(vRank)) + 1.0/(k+float64(bRank))
		}
		_ = fusedScores
	}
}

// BenchmarkQueryCacheStats benchmarks stats collection
func BenchmarkQueryCacheStats(b *testing.B) {
	cache := NewQueryCache(1000, 300*time.Second)

	// Pre-populate cache
	for i := 0; i < 100; i++ {
		req := models.RetrieveRequest{
			SpaceID:   "test-space",
			QueryText: "query " + string(rune(i)),
		}
		cache.Put(req, "", models.RetrieveResponse{})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.Stats()
	}
}

// BenchmarkCacheInvalidateSpace benchmarks space invalidation
func BenchmarkCacheInvalidateSpace(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		cache := NewQueryCache(1000, 300*time.Second)
		// Pre-populate cache with mixed spaces
		for j := 0; j < 500; j++ {
			space := "space-A"
			if j%2 == 0 {
				space = "space-B"
			}
			req := models.RetrieveRequest{
				SpaceID:   space,
				QueryText: "query " + string(rune(j)),
			}
			cache.Put(req, "", models.RetrieveResponse{})
		}
		b.StartTimer()

		cache.InvalidateSpace("space-A")
	}
}
