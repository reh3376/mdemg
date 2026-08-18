package retrieval

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"encoding/binary"
	"math"
	"mdemg/internal/models"
)

// QueryCache is a thread-safe TTL-LRU cache for retrieval results.
// It caches query results to avoid redundant Neo4j queries and embeddings.
type QueryCache struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	lruList  *list.List

	// Metrics
	hits   atomic.Int64
	misses atomic.Int64
}

// queryCacheEntry holds a cached query result with expiration.
type queryCacheEntry struct {
	key       string
	spaceID   string
	value     models.RetrieveResponse
	expiresAt time.Time
}

// NewQueryCache creates a new TTL-LRU cache for query results.
// capacity: max number of cached queries (default: 500)
// ttl: time-to-live for cache entries (default: 5 minutes)
func NewQueryCache(capacity int, ttl time.Duration) *QueryCache {
	if capacity <= 0 {
		capacity = 500
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &QueryCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		lruList:  list.New(),
	}
}

// CacheKey generates a cache key from query parameters + an opaque
// scorerVersion namespace. Phase 13 (Note 04 Column-Voting Retrieval) added
// the scorerVersion parameter so a config flag flip (legacy linear scorer →
// RRF column voting, or per-column weight changes) bumps to a new cache
// namespace and stale entries are bypassed automatically. Pass an empty
// scorerVersion to preserve the pre-Phase-13 namespace ("v0-pre-versioned").
func CacheKey(req models.RetrieveRequest, scorerVersion string) string {
	// Create a deterministic key from the relevant query parameters
	// CACHE-KEY-002: every result-affecting RetrieveRequest field MUST be in
	// this key (the v0.7.0 cache-key class — second occurrence fixed here).
	// A reflection test (cache_key_coverage_test.go) forces new fields to be
	// classified: either added here or explicitly allowlisted as
	// result-neutral.
	keyData := struct {
		SpaceID            string         `json:"s"`
		QueryText          string         `json:"q"`
		QueryEmbeddingHash string         `json:"qe,omitempty"`
		CandidateK         int            `json:"ck"`
		TopK               int            `json:"tk"`
		HopDepth           int            `json:"hd"`
		IncludeEvidence    bool           `json:"ie"`
		IncludeGlobalSpace bool           `json:"ig"`
		CodeOnly           bool           `json:"co"`
		TranslateIntent    bool           `json:"ti"`
		IncludeExtensions  []string       `json:"inx,omitempty"`
		ExcludeExtensions  []string       `json:"exx,omitempty"`
		TemporalAfter      string         `json:"ta,omitempty"`
		TemporalBefore     string         `json:"tb,omitempty"`
		PolicyContext      map[string]any `json:"pc,omitempty"`
		Cursor             string         `json:"cu,omitempty"`
		Limit              int            `json:"li,omitempty"`
		SparseOverride     bool           `json:"so,omitempty"`
		SparseEnabled      bool           `json:"se,omitempty"`
		SparsePercentile   float64        `json:"sp,omitempty"`
		Category           string         `json:"ca,omitempty"`
		ContextFingerprint []uint16       `json:"cf,omitempty"`
		ContextFPVersion   int            `json:"cv,omitempty"`
		StrictContextMode  bool           `json:"sc,omitempty"`
		ConcreteOverride   bool           `json:"cro,omitempty"` // RETRIEVAL-LAYER-BALANCE-001
		ConcreteEnabled    bool           `json:"cre,omitempty"` // RETRIEVAL-LAYER-BALANCE-001
		ReverseRefOverride bool           `json:"rro,omitempty"` // RETRIEVAL-REVERSE-LOOKUP-001
		ReverseRefEnabled  bool           `json:"rre,omitempty"` // RETRIEVAL-REVERSE-LOOKUP-001
		IncludeContent     bool           `json:"ic,omitempty"`  // INGEST-TOPOLOGY-REPAIR-001
		ScorerVersion      string         `json:"sv"`
	}{
		SpaceID:            req.SpaceID,
		QueryText:          req.QueryText,
		QueryEmbeddingHash: hashEmbedding(req.QueryEmbedding),
		CandidateK:         req.CandidateK,
		TopK:               req.TopK,
		HopDepth:           req.HopDepth,
		IncludeEvidence:    req.IncludeEvidence,
		IncludeGlobalSpace: req.IncludeGlobalSpace,
		CodeOnly:           req.CodeOnly,
		TranslateIntent:    req.TranslateIntent,
		IncludeExtensions:  req.IncludeExtensions,
		ExcludeExtensions:  req.ExcludeExtensions,
		TemporalAfter:      req.TemporalAfter,
		TemporalBefore:     req.TemporalBefore,
		PolicyContext:      req.PolicyContext,
		Cursor:             req.Cursor,
		Limit:              req.Limit,
		SparseOverride:     req.SparseOverridePresent,
		SparseEnabled:      req.SparseEnabled,
		SparsePercentile:   req.SparsePercentile,
		Category:           req.Category,
		ContextFingerprint: req.QueryContextFingerprint,
		ContextFPVersion:   req.QueryContextFingerprintVersion,
		StrictContextMode:  req.StrictContextMode,
		ConcreteOverride:   req.ConcreteRecallOverridePresent,
		ConcreteEnabled:    req.ConcreteRecallEnabled,
		ReverseRefOverride: req.ReverseRefOverridePresent,
		ReverseRefEnabled:  req.ReverseRefEnabled,
		IncludeContent:     req.IncludeContent, // INGEST-TOPOLOGY-REPAIR-001
		ScorerVersion:      scorerVersion,
	}

	data, _ := json.Marshal(keyData)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:16]) // First 16 bytes = 32 hex chars
}

// Get retrieves a cached response for the given request + scorer version
// namespace. Returns (response, true) if found and not expired,
// (zero, false) otherwise. scorerVersion segments the cache: a flag flip
// in the orchestrator (legacy linear → RRF column voting) maps to a fresh
// namespace, ensuring stale entries are not served against the new scorer.
func (c *QueryCache) Get(req models.RetrieveRequest, scorerVersion string) (models.RetrieveResponse, bool) {
	key := CacheKey(req, scorerVersion)

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[key]
	if !found {
		c.misses.Add(1)
		return models.RetrieveResponse{}, false
	}

	entry := elem.Value.(*queryCacheEntry)

	// Check TTL expiration
	if time.Now().After(entry.expiresAt) {
		// Remove expired entry
		c.lruList.Remove(elem)
		delete(c.items, key)
		c.misses.Add(1)
		return models.RetrieveResponse{}, false
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(elem)
	c.hits.Add(1)
	return entry.value, true
}

// Put caches a response for the given request + scorer version namespace.
// See [QueryCache.Get] for namespace semantics.
func (c *QueryCache) Put(req models.RetrieveRequest, scorerVersion string, resp models.RetrieveResponse) {
	key := CacheKey(req, scorerVersion)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if elem, found := c.items[key]; found {
		// Update existing entry and move to front
		c.lruList.MoveToFront(elem)
		entry := elem.Value.(*queryCacheEntry)
		entry.value = resp
		entry.expiresAt = time.Now().Add(c.ttl)
		return
	}

	// Add new entry
	entry := &queryCacheEntry{
		key:       key,
		spaceID:   req.SpaceID,
		value:     resp,
		expiresAt: time.Now().Add(c.ttl),
	}
	elem := c.lruList.PushFront(entry)
	c.items[key] = elem

	// Evict least recently used if over capacity
	if c.lruList.Len() > c.capacity {
		c.evictOldest()
	}
}

// InvalidateSpace removes all cached entries for a specific space.
// Call this after ingest, consolidate, or other mutations.
func (c *QueryCache) InvalidateSpace(spaceID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	elem := c.lruList.Front()
	for elem != nil {
		next := elem.Next()
		entry := elem.Value.(*queryCacheEntry)
		if entry.spaceID == spaceID {
			c.lruList.Remove(elem)
			delete(c.items, entry.key)
			removed++
		}
		elem = next
	}
	return removed
}

// Clear removes all entries from the cache.
func (c *QueryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lruList.Init()
}

// Len returns the current number of items in the cache.
func (c *QueryCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// Stats returns cache statistics.
func (c *QueryCache) Stats() map[string]any {
	c.mu.RLock()
	size := c.lruList.Len()
	c.mu.RUnlock()

	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return map[string]any{
		"size":     size,
		"capacity": c.capacity,
		"ttl_sec":  int(c.ttl.Seconds()),
		"hits":     hits,
		"misses":   misses,
		"hit_rate": hitRate,
	}
}

// evictOldest removes the least recently used item from the cache.
// Must be called with mutex locked.
func (c *QueryCache) evictOldest() {
	elem := c.lruList.Back()
	if elem == nil {
		return
	}

	c.lruList.Remove(elem)
	entry := elem.Value.(*queryCacheEntry)
	delete(c.items, entry.key)
}

// hashEmbedding fingerprints a caller-supplied query embedding for the cache
// key ("" when absent). Without this, two different embedding-only queries
// collided on one cache entry (CACHE-KEY-002).
func hashEmbedding(emb []float32) string {
	if len(emb) == 0 {
		return ""
	}
	h := sha256.New()
	buf := make([]byte, 4)
	for _, f := range emb {
		binary.LittleEndian.PutUint32(buf, math.Float32bits(f))
		h.Write(buf)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}
