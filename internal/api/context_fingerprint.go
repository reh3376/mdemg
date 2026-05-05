// Phase 14.2.1 — Vector-based query fingerprint derivation.
//
// Replaces the original Phase 14.2 tokenize-and-match `?context=auto` helper
// (handlers.go::tokenizeForFingerprint) which matched query tokens directly
// against catalog tag/path bit ref strings. That approach failed in Phase
// 14.2 Epic 6 A/B because catalog tags are LLM-summary buckets (`api`,
// `architecture`, `caching`...) while UVTS queries use domain vocab
// (`barrel ownership`, `validation error`...). Empty fingerprint →
// ContextColumn voted nothing → B ≈ A.
//
// This file embeds the query and the catalog refs in the same vector space
// (text-embedding-3-large) and selects the top-K closest refs by cosine
// similarity. The resulting fingerprint reflects semantic affinity rather
// than literal token overlap.
//
// Caching:
//   - Catalog ref embeddings are computed once per (space_id, catalog_version)
//     and stored in-process. Refresh on catalog version bump is automatic
//     (cache key includes the version).
//   - Query embeddings are NOT cached here (the retrieve path embeds the
//     query for vector recall anyway; we re-use that downstream).
//
// Performance:
//   - First `?context=auto` call per (space, version): one embed-batch call
//     for 256 catalog refs (~$0.0001 at openai rates).
//   - Subsequent calls: one extra embed call for the query (negligible
//     since vector recall would do this anyway) + 256 dot products (~0.5ms).
package api

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"

	"mdemg/internal/embeddings"
	"mdemg/internal/hidden"
)

// catalogVecCache holds one (space_id, version) snapshot of catalog ref
// embeddings. Built once per snapshot, reused for all subsequent
// `?context=auto` calls until a new catalog version supersedes it.
type catalogVecCache struct {
	spaceID string
	version int
	totalBits uint16
	refs    []vecCacheEntry
}

type vecCacheEntry struct {
	position uint16
	kind     hidden.BitKind
	ref      string // canonical key (path / tag / "rt|layer" / symbol id)
	vec      []float32
}

// contextFingerprintCache is the per-Server in-process cache. Indexed by
// space_id; supersedes the entry on catalog version bump.
type contextFingerprintCache struct {
	mu       sync.RWMutex
	bySpace  map[string]*catalogVecCache
	embedder embeddings.Embedder
}

func newContextFingerprintCache(embedder embeddings.Embedder) *contextFingerprintCache {
	return &contextFingerprintCache{
		bySpace:  make(map[string]*catalogVecCache),
		embedder: embedder,
	}
}

// derive returns the top-K catalog bit positions whose ref embeddings are
// closest to queryVec by cosine similarity. The cache for (spaceID, cat.Version)
// is built lazily on first call. Returns nil on any error or when topK is
// non-positive.
func (c *contextFingerprintCache) derive(ctx context.Context, cat *hidden.Catalog, queryText string, topK int) ([]uint16, error) {
	if c == nil || c.embedder == nil || cat == nil || queryText == "" || topK <= 0 {
		return nil, nil
	}

	snap, err := c.getOrBuild(ctx, cat)
	if err != nil || snap == nil {
		return nil, err
	}
	if len(snap.refs) == 0 {
		return nil, nil
	}

	qVec, err := c.embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Score every ref by cosine sim; keep top-K.
	type scored struct {
		pos   uint16
		score float64
	}
	all := make([]scored, 0, len(snap.refs))
	for i := range snap.refs {
		s := cosineSim(qVec, snap.refs[i].vec)
		// Keep only positively correlated bits (a negative cosine means the
		// ref is *less* relevant than a random vector — bringing it in
		// would actively hurt the column).
		if s <= 0 {
			continue
		}
		all = append(all, scored{pos: snap.refs[i].position, score: s})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })

	if topK > len(all) {
		topK = len(all)
	}
	out := make([]uint16, topK)
	for i := 0; i < topK; i++ {
		out[i] = all[i].pos
	}
	// Fingerprints must be sorted ascending for Jaccard-friendly storage.
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// getOrBuild returns the cached snapshot for (spaceID, cat.Version), building
// it if missing or if the cached version is stale.
func (c *contextFingerprintCache) getOrBuild(ctx context.Context, cat *hidden.Catalog) (*catalogVecCache, error) {
	c.mu.RLock()
	snap := c.bySpace[cat.SpaceID]
	c.mu.RUnlock()
	if snap != nil && snap.version == cat.Version {
		return snap, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Re-check under the write lock (another goroutine may have built it).
	if cur, ok := c.bySpace[cat.SpaceID]; ok && cur.version == cat.Version {
		return cur, nil
	}

	// Embed every non-empty catalog ref. The order matches cat.Bits so
	// position lookup is direct (snap.refs[i].position == cat.Bits[i].Position).
	texts := make([]string, 0, len(cat.Bits))
	indices := make([]int, 0, len(cat.Bits))
	for i, b := range cat.Bits {
		text := refEmbedText(b)
		if text == "" {
			continue
		}
		texts = append(texts, text)
		indices = append(indices, i)
	}
	if len(texts) == 0 {
		return nil, nil
	}

	vecs, err := c.embedder.EmbedBatch(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed batch (%d catalog refs): %w", len(texts), err)
	}
	if len(vecs) != len(texts) {
		return nil, fmt.Errorf("embed batch returned %d vectors for %d texts", len(vecs), len(texts))
	}

	refs := make([]vecCacheEntry, len(texts))
	for i, ix := range indices {
		b := cat.Bits[ix]
		refs[i] = vecCacheEntry{
			position: b.Position,
			kind:     b.Kind,
			ref:      b.Ref,
			vec:      vecs[i],
		}
	}
	snap = &catalogVecCache{
		spaceID:   cat.SpaceID,
		version:   cat.Version,
		totalBits: cat.TotalBits,
		refs:      refs,
	}
	c.bySpace[cat.SpaceID] = snap
	slog.Info("context fingerprint vec cache built",
		"space_id", cat.SpaceID,
		"version", cat.Version,
		"refs_embedded", len(refs))
	return snap, nil
}

// refEmbedText converts a catalog bit's ref/token into the text to embed.
// For tags and paths the canonical ref is the most informative; for
// role_type×layer tuples the human-readable token is preferred (the raw
// "leaf|0" key embeds poorly).
func refEmbedText(b hidden.BitEntry) string {
	switch b.Kind {
	case hidden.BitKindTag, hidden.BitKindPath, hidden.BitKindSymbol:
		return b.Ref
	case hidden.BitKindRoleTypeLayer:
		if b.Token != "" {
			return b.Token
		}
		return b.Ref
	}
	return b.Ref
}

// cosineSim computes cosine similarity between two embedding vectors.
// Mirrors retrieval.cosineSimilarity but lives here to keep the api package
// independent of internal/retrieval (avoid import cycles).
func cosineSim(a, b []float32) float64 {
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
