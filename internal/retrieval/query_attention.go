package retrieval

import (
	"container/list"
	"context"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
)

// NodeEmbeddingCache provides an LRU cache for node embeddings to avoid repeated DB queries.
// Uses container/list for O(1) access, insertion, and eviction (no linear scans).
type NodeEmbeddingCache struct {
	mu       sync.Mutex
	items    map[string]*list.Element // nodeID -> list element
	lruList  *list.List              // front = most recent, back = least recent
	capacity int
	hits     int64
	misses   int64
}

type embeddingEntry struct {
	key        string
	embedding  []float32
	accessedAt time.Time
}

// NewNodeEmbeddingCache creates a new embedding cache with the specified capacity.
// Minimum capacity is 1 for production use (config enforces minimum of 100 at env level).
func NewNodeEmbeddingCache(capacity int) *NodeEmbeddingCache {
	if capacity < 1 {
		capacity = 1
	}
	return &NodeEmbeddingCache{
		items:    make(map[string]*list.Element, capacity),
		lruList:  list.New(),
		capacity: capacity,
	}
}

// Get retrieves an embedding from cache. Returns nil if not found. O(1).
func (c *NodeEmbeddingCache) Get(nodeID string) []float32 {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[nodeID]
	if !ok {
		c.misses++
		return nil
	}

	c.hits++
	entry := elem.Value.(*embeddingEntry)
	entry.accessedAt = time.Now()
	c.lruList.MoveToFront(elem)

	return entry.embedding
}

// Put stores an embedding in cache, evicting LRU if at capacity. O(1).
func (c *NodeEmbeddingCache) Put(nodeID string, embedding []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Already in cache - update
	if elem, ok := c.items[nodeID]; ok {
		entry := elem.Value.(*embeddingEntry)
		entry.embedding = embedding
		entry.accessedAt = time.Now()
		c.lruList.MoveToFront(elem)
		return
	}

	// Evict LRU if at capacity
	for len(c.items) >= c.capacity {
		back := c.lruList.Back()
		if back == nil {
			break
		}
		evicted := c.lruList.Remove(back).(*embeddingEntry)
		delete(c.items, evicted.key)
	}

	// Add new entry
	entry := &embeddingEntry{
		key:        nodeID,
		embedding:  embedding,
		accessedAt: time.Now(),
	}
	elem := c.lruList.PushFront(entry)
	c.items[nodeID] = elem
}

// Stats returns cache statistics.
func (c *NodeEmbeddingCache) Stats() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()

	hitRate := float64(0)
	total := c.hits + c.misses
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return map[string]any{
		"size":     len(c.items),
		"capacity": c.capacity,
		"hits":     c.hits,
		"misses":   c.misses,
		"hit_rate": hitRate,
	}
}

// EdgeWithAttention extends Edge with an attention score for query-aware ranking.
type EdgeWithAttention struct {
	Edge
	AttentionScore float64
}

// ComputeQueryAwareAttention calculates attention score for a neighbor.
// This blends query-destination similarity with edge weight and dimensions.
//
// Formula: attention = α * cosSim(query, dst) + (1-α) * edgeSignal
// where edgeSignal = 0.4*weight + 0.3*dimSemantic + 0.2*dimCoactivation + 0.1*dimTemporal
//
// The attention weight α is configurable via QUERY_AWARE_ATTENTION_WEIGHT (default: 0.5).
func ComputeQueryAwareAttention(queryEmb, dstEmb []float32, edge Edge, attentionWeight float64) float64 {
	// Query-destination cosine similarity
	queryDstSim := cosineSimilarity(queryEmb, dstEmb)

	// Edge signal from existing features
	edgeSignal := 0.4*edge.Weight + 0.3*edge.DimSemantic + 0.2*edge.DimCoactivation + 0.1*edge.DimTemporal

	// Blend query similarity with edge signal
	attention := attentionWeight*queryDstSim + (1-attentionWeight)*edgeSignal

	// Clamp to [0, 1]
	if attention < 0 {
		attention = 0
	}
	if attention > 1 {
		attention = 1
	}

	return attention
}

// cosineSimilarity computes cosine similarity between two embedding vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// fetchNodeEmbeddings fetches embeddings for a batch of node IDs from Neo4j.
// Returns a map of nodeID -> embedding. Nodes without embeddings are omitted.
func fetchNodeEmbeddings(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, nodeIDs []string) (map[string][]float32, error) {
	if len(nodeIDs) == 0 {
		return map[string][]float32{}, nil
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
			UNWIND $nodeIds AS nid
			MATCH (n:MemoryNode {space_id: $spaceId, node_id: nid})
			WHERE n.embedding IS NOT NULL
			RETURN n.node_id AS id, n.embedding AS emb
		`
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeIds": nodeIDs,
		})
		if err != nil {
			return nil, err
		}

		embeddings := make(map[string][]float32)
		for res.Next(ctx) {
			rec := res.Record()
			id, _ := rec.Get("id")
			embAny, _ := rec.Get("emb")

			nodeID := id.(string)
			if embSlice, ok := embAny.([]any); ok {
				emb := make([]float32, len(embSlice))
				for i, v := range embSlice {
					if f, ok := v.(float64); ok {
						emb[i] = float32(f)
					}
				}
				embeddings[nodeID] = emb
			}
		}

		return embeddings, res.Err()
	})

	if err != nil {
		return nil, err
	}

	return result.(map[string][]float32), nil
}

// ReRankEdgesByAttention re-ranks edges using query-aware attention scores.
// It fetches destination node embeddings (using cache when available) and
// computes attention for each edge, then returns edges sorted by attention score descending.
//
// The maxPerNode parameter limits how many edges to return per source node.
func ReRankEdgesByAttention(
	ctx context.Context,
	driver neo4j.DriverWithContext,
	cache *NodeEmbeddingCache,
	spaceID string,
	queryEmb []float32,
	edges []Edge,
	maxPerNode int,
	cfg config.Config,
) ([]Edge, error) {
	if len(edges) == 0 || len(queryEmb) == 0 {
		return edges, nil
	}

	// Collect unique destination node IDs that need embeddings
	needFetch := make([]string, 0, len(edges))
	dstSet := make(map[string]struct{})

	for _, e := range edges {
		if _, ok := dstSet[e.Dst]; ok {
			continue
		}
		dstSet[e.Dst] = struct{}{}

		// Check cache first
		if cache != nil && cache.Get(e.Dst) != nil {
			continue
		}
		needFetch = append(needFetch, e.Dst)
	}

	// Fetch missing embeddings from DB
	if len(needFetch) > 0 {
		fetched, err := fetchNodeEmbeddings(ctx, driver, spaceID, needFetch)
		if err != nil {
			slog.Warn("failed to fetch node embeddings for attention", "error", err)
			// Fall back to original edges without attention re-ranking
			return edges, nil
		}

		// Populate cache
		if cache != nil {
			for nodeID, emb := range fetched {
				cache.Put(nodeID, emb)
			}
		}
	}

	// Compute attention for each edge
	edgesWithAttn := make([]EdgeWithAttention, 0, len(edges))
	for _, e := range edges {
		var dstEmb []float32
		if cache != nil {
			dstEmb = cache.Get(e.Dst)
		}

		var attn float64
		if dstEmb != nil {
			attn = ComputeQueryAwareAttention(queryEmb, dstEmb, e, cfg.QueryAwareAttentionWeight)
		} else {
			// No embedding available - use edge weight as fallback
			attn = e.Weight
		}

		edgesWithAttn = append(edgesWithAttn, EdgeWithAttention{
			Edge:           e,
			AttentionScore: attn,
		})
	}

	// Sort by attention score descending
	sort.Slice(edgesWithAttn, func(i, j int) bool {
		return edgesWithAttn[i].AttentionScore > edgesWithAttn[j].AttentionScore
	})

	// Apply per-node limit if specified
	if maxPerNode > 0 {
		srcCount := make(map[string]int)
		filtered := make([]EdgeWithAttention, 0, len(edgesWithAttn))

		for _, e := range edgesWithAttn {
			if srcCount[e.Src] < maxPerNode {
				filtered = append(filtered, e)
				srcCount[e.Src]++
			}
		}
		edgesWithAttn = filtered
	}

	// Convert back to Edge slice
	result := make([]Edge, len(edgesWithAttn))
	for i, e := range edgesWithAttn {
		result[i] = e.Edge
	}

	return result, nil
}
