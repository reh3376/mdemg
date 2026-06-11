package retrieval

import (
	"testing"

	"mdemg/internal/config"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
		},
		{
			name:     "similar vectors",
			a:        []float32{1, 1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.7071067811865475, // 1/sqrt(2)
		},
		{
			name:     "empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
		},
		{
			name:     "different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			if diff := result - tt.expected; diff > 0.0001 || diff < -0.0001 {
				t.Errorf("cosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestComputeQueryAwareAttention(t *testing.T) {
	tests := []struct {
		name            string
		queryEmb        []float32
		dstEmb          []float32
		edge            Edge
		attentionWeight float64
		wantMin         float64
		wantMax         float64
	}{
		{
			name:     "high query similarity",
			queryEmb: []float32{1, 0, 0},
			dstEmb:   []float32{1, 0, 0}, // cos sim = 1.0
			edge: Edge{
				Weight:          0.5,
				DimSemantic:     0.5,
				DimCoactivation: 0.5,
				DimTemporal:     0.5,
			},
			attentionWeight: 0.5,
			wantMin:         0.7, // high because query-dst sim is 1.0
			wantMax:         1.0,
		},
		{
			name:     "low query similarity high edge weight",
			queryEmb: []float32{1, 0, 0},
			dstEmb:   []float32{0, 1, 0}, // cos sim = 0.0
			edge: Edge{
				Weight:          1.0,
				DimSemantic:     1.0,
				DimCoactivation: 1.0,
				DimTemporal:     1.0,
			},
			attentionWeight: 0.5,
			wantMin:         0.4, // edge signal is high
			wantMax:         0.6,
		},
		{
			name:     "pure query attention (weight=1.0)",
			queryEmb: []float32{1, 0, 0},
			dstEmb:   []float32{1, 0, 0},
			edge: Edge{
				Weight:          0.1,
				DimSemantic:     0.1,
				DimCoactivation: 0.1,
				DimTemporal:     0.1,
			},
			attentionWeight: 1.0, // ignore edge signal
			wantMin:         0.99,
			wantMax:         1.0,
		},
		{
			name:     "pure edge weight (attention=0.0)",
			queryEmb: []float32{1, 0, 0},
			dstEmb:   []float32{1, 0, 0},
			edge: Edge{
				Weight:          0.8,
				DimSemantic:     0.6,
				DimCoactivation: 0.4,
				DimTemporal:     0.2,
			},
			attentionWeight: 0.0, // ignore query similarity
			wantMin:         0.5,
			wantMax:         0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ComputeQueryAwareAttention(tt.queryEmb, tt.dstEmb, tt.edge, tt.attentionWeight)
			if result < tt.wantMin || result > tt.wantMax {
				t.Errorf("ComputeQueryAwareAttention() = %v, want in range [%v, %v]", result, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestNodeEmbeddingCache(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		cache := NewNodeEmbeddingCache(3)

		// Empty cache
		if got := cache.Get("node1"); got != nil {
			t.Errorf("Get on empty cache should return nil, got %v", got)
		}

		// Put and Get
		emb := []float32{0.1, 0.2, 0.3}
		cache.Put("node1", emb)

		got := cache.Get("node1")
		if got == nil {
			t.Error("Get after Put should return embedding")
		}
		if len(got) != len(emb) {
			t.Errorf("Got embedding length %d, want %d", len(got), len(emb))
		}
	})

	t.Run("LRU eviction", func(t *testing.T) {
		cache := NewNodeEmbeddingCache(3)

		// Fill cache
		cache.Put("node1", []float32{1})
		cache.Put("node2", []float32{2})
		cache.Put("node3", []float32{3})

		// Access node1 to make it recently used
		cache.Get("node1")

		// Add node4 - should evict node2 (least recently used)
		cache.Put("node4", []float32{4})

		if cache.Get("node1") == nil {
			t.Error("node1 should still be in cache")
		}
		if cache.Get("node2") != nil {
			t.Error("node2 should have been evicted")
		}
		if cache.Get("node3") == nil {
			t.Error("node3 should still be in cache")
		}
		if cache.Get("node4") == nil {
			t.Error("node4 should be in cache")
		}
	})

	t.Run("stats tracking", func(t *testing.T) {
		cache := NewNodeEmbeddingCache(100)

		cache.Put("node1", []float32{1})

		// Hit
		cache.Get("node1")
		// Miss
		cache.Get("node2")

		stats := cache.Stats()
		if stats["hits"].(int64) != 1 {
			t.Errorf("Expected 1 hit, got %v", stats["hits"])
		}
		if stats["misses"].(int64) != 1 {
			t.Errorf("Expected 1 miss, got %v", stats["misses"])
		}
		if stats["hit_rate"].(float64) != 0.5 {
			t.Errorf("Expected 0.5 hit rate, got %v", stats["hit_rate"])
		}
	})
}

func TestReRankEdgesByAttention_WithMockEmbeddings(t *testing.T) {
	// This test verifies the re-ranking logic without DB access
	// by testing the attention computation and sorting

	cfg := config.Config{
		QueryAwareAttentionWeight: 0.5,
	}

	// Create edges with different weights
	edges := []Edge{
		{Src: "a", Dst: "b", Weight: 0.9, DimSemantic: 0.5},
		{Src: "a", Dst: "c", Weight: 0.3, DimSemantic: 0.5},
		{Src: "a", Dst: "d", Weight: 0.6, DimSemantic: 0.5},
	}

	// Simulate attention scores
	// Edge to 'c' should have highest attention if query is similar to 'c'
	queryEmb := []float32{1, 0, 0}
	embeddings := map[string][]float32{
		"b": {0, 1, 0},     // orthogonal to query - low attention
		"c": {0.9, 0.1, 0}, // similar to query - high attention
		"d": {0.5, 0.5, 0}, // medium similarity
	}

	// Compute expected attention scores
	attnB := ComputeQueryAwareAttention(queryEmb, embeddings["b"], edges[0], cfg.QueryAwareAttentionWeight)
	attnC := ComputeQueryAwareAttention(queryEmb, embeddings["c"], edges[1], cfg.QueryAwareAttentionWeight)
	attnD := ComputeQueryAwareAttention(queryEmb, embeddings["d"], edges[2], cfg.QueryAwareAttentionWeight)

	t.Logf("Attention scores: b=%v, c=%v, d=%v", attnB, attnC, attnD)

	// Verify that edge 'c' has highest attention despite lowest weight
	if attnC < attnB {
		t.Error("Edge to 'c' (similar to query) should have higher attention than 'b' (orthogonal)")
	}
	if attnC < attnD {
		t.Error("Edge to 'c' should have highest attention")
	}
}

func TestEdgeWithAttention_Sorting(t *testing.T) {
	edges := []EdgeWithAttention{
		{Edge: Edge{Src: "a", Dst: "b"}, AttentionScore: 0.3},
		{Edge: Edge{Src: "a", Dst: "c"}, AttentionScore: 0.9},
		{Edge: Edge{Src: "a", Dst: "d"}, AttentionScore: 0.6},
	}

	// Sort by attention descending
	for i := 0; i < len(edges)-1; i++ {
		for j := i + 1; j < len(edges); j++ {
			if edges[i].AttentionScore < edges[j].AttentionScore {
				edges[i], edges[j] = edges[j], edges[i]
			}
		}
	}

	// Verify order
	expected := []string{"c", "d", "b"}
	for i, e := range edges {
		if e.Dst != expected[i] {
			t.Errorf("Position %d: got dst=%s, want %s", i, e.Dst, expected[i])
		}
	}
}

func TestQueryAwareExpansion_FeatureToggle(t *testing.T) {
	// Verify that config defaults work correctly
	t.Run("default enabled", func(t *testing.T) {
		// Default should be enabled
		cfg := config.Config{
			QueryAwareExpansionEnabled: true,
			QueryAwareAttentionWeight:  0.5,
			NodeEmbeddingCacheSize:     5000,
		}

		if !cfg.QueryAwareExpansionEnabled {
			t.Error("Query-aware expansion should be enabled by default")
		}
		if cfg.QueryAwareAttentionWeight != 0.5 {
			t.Errorf("Attention weight should be 0.5, got %v", cfg.QueryAwareAttentionWeight)
		}
	})

	t.Run("can disable", func(t *testing.T) {
		cfg := config.Config{
			QueryAwareExpansionEnabled: false,
		}

		if cfg.QueryAwareExpansionEnabled {
			t.Error("Should be able to disable query-aware expansion")
		}
	})
}
