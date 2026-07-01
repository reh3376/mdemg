package retrieval

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/config"
	"mdemg/internal/llmclient"
	"mdemg/internal/metrics"
	"mdemg/internal/models"
	"sync"
)

type Service struct {
	cfg    config.Config
	driver neo4j.DriverWithContext
	// RRF-SCALE-002: persistent rerank clients. A fresh llmclient.New per
	// call reset the consecutive-failure counter (the alert threshold could
	// NEVER fire for retrieval.rerank_cross/rerank_nli) and discarded the
	// HTTP transport on the hottest LLM path. WithContext() shallow-copies
	// and SHARES the *atomic counter + breaker, so deriving per-call
	// contexts from these bases preserves failure accounting.
	rerankOpenAIOnce     sync.Once
	rerankOpenAIClient   *llmclient.Client
	rerankOllamaOnce     sync.Once
	rerankOllamaClient   *llmclient.Client
	reasoningProvider    ReasoningProvider
	queryCache           *QueryCache
	embeddingCache       *NodeEmbeddingCache      // Cache for node embeddings (query-aware expansion)
	cbRegistry           *circuitbreaker.Registry // Circuit breaker registry for external API calls
	intentTranslator     IntentTranslator         // Optional BM25 query rewriter
	dataCollector        *DataCollector           // Neural re-ranker training data collector (NR-1)
	retrievalRecorder    RetrievalEventRecorder   // Optional retrieval event recorder for contrastive training data
	queryClassifier      *QueryClassifier         // Optional LLM query type classifier (PROD-READINESS)
	retrievalAuditWriter RetrievalAuditWriter     // Phase 13 — Optional retrieval_audit writer (V0017); nil-safe
	sparseGateRecorder   SparseGateRecorder       // Phase 14 — Optional V0019 sparse_gate_metrics writer; nil-safe
}

// SparseGateRecorder is the contract for persisting V0019 sparse_gate_metrics
// rows. Implementation lives in internal/tsdb (avoids the import cycle that
// blocks moving the type definition there). Wired by api.NewServer.
type SparseGateRecorder interface {
	RecordGate(spaceID string, meta SparseGateMetadata, scorerVersion string)
}

// SetSparseGateRecorder attaches a V0019 metrics recorder. Pass nil to disable.
// service.Retrieve calls Record only when the gate fires, so the recorder
// captures the same set of events as the in-process Prometheus histograms but
// with TSDB durability.
func (s *Service) SetSparseGateRecorder(r SparseGateRecorder) {
	s.sparseGateRecorder = r
}

// SetRetrievalRecorder attaches a retrieval event recorder for training data collection.
func (s *Service) SetRetrievalRecorder(r RetrievalEventRecorder) {
	s.retrievalRecorder = r
}

// scorerVersion returns an opaque short string identifying the active scoring
// pipeline. Used to namespace the query cache so a config flip between the
// legacy linear scorer and the Phase 13 (Note 04) RRF column-voting
// aggregator does not serve stale entries from one against requests
// expecting the other.
//
// Phase 13.1: extended to include the per-column weight + hop-depth +
// per-column-enable flags so that ablation sweeps automatically invalidate
// cache between presets without operator intervention. Two presets that
// produce different rankings will produce different scorer versions →
// different cache namespaces → no contamination.
//
// Phase 14.2 Epic 4: bumped to "v1-rrf5" + extended with context column
// flag/weight/strict-threshold so toggling the 5th column (ContextColumn)
// flips the cache namespace.
func (s *Service) scorerVersion() string {
	if !s.cfg.RetrievalColumnVotingEnabled {
		return "v0-linear"
	}
	// v2 (CONTEXT-LIVE-001): consensus denominator counts only columns able
	// to vote; cross-version fingerprint guard active. catmaps hashes the
	// per-category context weights + sparse overrides — operator edits to
	// either JSON map must flip the cache namespace (the v0.7.0 cache-key
	// class).
	// RETRIEVAL-TYPED-EDGES-001: the typed-edges flag + its 7 semantic weights
	// join the namespace so flipping the flag or tuning a weight invalidates the
	// cache (the v0.7.0 cache-key class). Only meaningful when the flag is on, but
	// always included for a stable, collision-free key.
	typedEdges := "off"
	if s.cfg.RetrievalGraphTypedEdgesEnabled {
		typedEdges = fmt.Sprintf("on|an=%.3f|br=%.3f|cw=%.3f|ct=%.3f|in=%.3f|ds=%.3f|th=%.3f",
			s.cfg.EdgeAttentionAnalogousTo, s.cfg.EdgeAttentionBridges,
			s.cfg.EdgeAttentionComposesWith, s.cfg.EdgeAttentionContrastsWith,
			s.cfg.EdgeAttentionInfluences, s.cfg.EdgeAttentionDefinesSymbol,
			s.cfg.EdgeAttentionThemeOf)
	}
	return fmt.Sprintf(
		"v2-rrf5|e=%.3f|b=%.3f|g=%.3f|s=%.3f|c=%.3f|hops=%d|emb=%t|bm=%t|gr=%t|st=%t|ctx=%t|strict=%.3f|catmaps=%s|tge=%s",
		s.cfg.RetrievalColumnWeightEmbedding,
		s.cfg.RetrievalColumnWeightBM25,
		s.cfg.RetrievalColumnWeightGraph,
		s.cfg.RetrievalColumnWeightStructural,
		s.cfg.RetrievalContextColumnWeight,
		s.cfg.RetrievalStructuralHops,
		s.cfg.RetrievalColumnEmbeddingEnabled,
		s.cfg.RetrievalColumnBM25Enabled,
		s.cfg.RetrievalColumnGraphEnabled,
		s.cfg.RetrievalColumnStructuralEnabled,
		s.cfg.RetrievalContextColumnEnabled,
		s.cfg.RetrievalContextStrictThreshold,
		s.categoryMapsHash(),
		typedEdges,
	)
}

// categoryMapsHash returns a short stable hash over the per-category
// context-column weight map and the sparse-gate category override map, in
// sorted-key order (map iteration is random; the hash must be stable).
func (s *Service) categoryMapsHash() string {
	h := sha256.New()
	keys := make([]string, 0, len(s.cfg.RetrievalContextColumnCategoryWeights))
	for k := range s.cfg.RetrievalContextColumnCategoryWeights {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "cw|%s=%.4f;", k, s.cfg.RetrievalContextColumnCategoryWeights[k])
	}
	keys = keys[:0]
	for k := range s.cfg.SparseGateCategoryOverrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		// Dereference pointer fields — %+v on pointers prints addresses,
		// which are not stable across processes.
		o := s.cfg.SparseGateCategoryOverrides[k]
		ma, mx, pc := -1, -1, -1.0
		if o.MinActive != nil {
			ma = *o.MinActive
		}
		if o.MaxActive != nil {
			mx = *o.MaxActive
		}
		if o.Percentile != nil {
			pc = *o.Percentile
		}
		fmt.Fprintf(h, "sg|%s=%d,%d,%.4f;", k, ma, mx, pc)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// deriveCategoryFromQueryType maps the classifier's '+'-joined query types
// onto the UVTS category vocabulary (CONTEXT-LIVE-001). Explicit category
// wins; first mapped type wins for multi-label; empty map disables.
func deriveCategoryFromQueryType(existing, queryType string, m map[string]string) string {
	if existing != "" || len(m) == 0 || queryType == "" {
		return existing
	}
	for _, qt := range strings.Split(queryType, "+") {
		if mapped, ok := m[qt]; ok && mapped != "" {
			return mapped
		}
	}
	return ""
}

// SetIntentTranslator sets the intent translator for BM25 query rewriting.
func (s *Service) SetIntentTranslator(t IntentTranslator) {
	s.intentTranslator = t
}

// SetQueryClassifier sets the LLM query type classifier.
func (s *Service) SetQueryClassifier(qc *QueryClassifier) {
	s.queryClassifier = qc
}

// FileFilter specifies file extension filtering for retrieval queries.
// This helps focus code-related queries on actual source code files.
type FileFilter struct {
	IncludeExtensions []string // Only include files with these extensions (e.g., ["java", "go"])
	ExcludeExtensions []string // Exclude files with these extensions (e.g., ["md", "txt"])
}

// CodeOnlyExclusions returns the standard exclusions for code-focused queries.
// These are common non-code file types that often pollute code search results.
var CodeOnlyExclusions = []string{"md", "txt", "json", "yaml", "yml", "toml", "xml", "csv", "lock", "sum"}

// NewFileFilterFromRequest creates a FileFilter from retrieval request parameters.
func NewFileFilterFromRequest(req models.RetrieveRequest) FileFilter {
	filter := FileFilter{
		IncludeExtensions: req.IncludeExtensions,
		ExcludeExtensions: req.ExcludeExtensions,
	}
	// CodeOnly is a convenience shorthand that adds common non-code exclusions
	if req.CodeOnly {
		filter.ExcludeExtensions = append(filter.ExcludeExtensions, CodeOnlyExclusions...)
	}
	return filter
}

// IsEmpty returns true if no filters are specified.
func (f FileFilter) IsEmpty() bool {
	return len(f.IncludeExtensions) == 0 && len(f.ExcludeExtensions) == 0
}

// BuildCypherFilter returns the Cypher WHERE clause fragment for file filtering.
// Returns empty string if no filters are specified.
//
// Note: MDEMG paths can have symbol suffixes like "/path/file.ts#ClassName".
// We use CONTAINS instead of ENDS WITH to handle both cases:
// - /path/file.ts (normal path)
// - /path/file.ts#Symbol (path with symbol suffix)
func (f FileFilter) BuildCypherFilter() string {
	if f.IsEmpty() {
		return ""
	}

	clauses := []string{}

	// Include filter: path must contain one of the specified extensions
	// Using CONTAINS '.ext' OR ENDS WITH '.ext' to handle #symbol suffix
	if len(f.IncludeExtensions) > 0 {
		clauses = append(clauses, `ANY(ext IN $includeExtensions WHERE node.path CONTAINS ('.' + ext + '#') OR node.path ENDS WITH ('.' + ext))`)
	}

	// Exclude filter: path must NOT contain any of the specified extensions
	// Using CONTAINS '.ext' OR ENDS WITH '.ext' to handle #symbol suffix
	if len(f.ExcludeExtensions) > 0 {
		clauses = append(clauses, `NOT ANY(ext IN $excludeExtensions WHERE node.path CONTAINS ('.' + ext + '#') OR node.path ENDS WITH ('.' + ext))`)
	}

	if len(clauses) == 1 {
		return " AND " + clauses[0]
	}
	return " AND (" + clauses[0] + " AND " + clauses[1] + ")"
}

func NewService(cfg config.Config, driver neo4j.DriverWithContext) *Service {
	// Initialize query cache with configurable TTL (default: 5 minutes, capacity: 500)
	cacheTTL := time.Duration(cfg.QueryCacheTTLSeconds) * time.Second
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Minute
	}
	cacheCapacity := cfg.QueryCacheCapacity
	if cacheCapacity <= 0 {
		cacheCapacity = 500
	}

	// Initialize node embedding cache for query-aware expansion
	embCacheSize := cfg.NodeEmbeddingCacheSize
	if embCacheSize <= 0 {
		embCacheSize = 5000
	}
	var embCacheTTL time.Duration
	if cfg.NodeEmbeddingCacheTTLSec > 0 {
		embCacheTTL = time.Duration(cfg.NodeEmbeddingCacheTTLSec) * time.Second
	}

	slog.Info("query cache initialized", "enabled", cfg.QueryCacheEnabled, "capacity", cacheCapacity, "ttl", cacheTTL)
	slog.Info("node embedding cache initialized", "enabled", cfg.QueryAwareExpansionEnabled, "capacity", embCacheSize, "ttl_sec", cfg.NodeEmbeddingCacheTTLSec)

	svc := &Service{
		cfg:               cfg,
		driver:            driver,
		reasoningProvider: &NoOpReasoningProvider{}, // Default: no reasoning modules
		queryCache:        NewQueryCache(cacheCapacity, cacheTTL),
		embeddingCache:    NewNodeEmbeddingCache(embCacheSize, embCacheTTL),
	}

	// Initialize neural re-ranker training data collector (NR-1)
	if cfg.NeuralDataCollection {
		svc.dataCollector = NewDataCollector(true, cfg.NeuralDataDir)
		slog.Info("neural data collector initialized", "dir", cfg.NeuralDataDir)
	}

	return svc
}

// SetReasoningProvider sets the reasoning provider for the service.
// This allows reasoning modules to be wired in after service creation.
func (s *Service) SetReasoningProvider(provider ReasoningProvider) {
	if provider != nil {
		s.reasoningProvider = provider
	}
}

// SetCircuitBreakerRegistry sets the circuit breaker registry for external API calls.
// This allows the service to use circuit breakers for rerank and other LLM calls.
func (s *Service) SetCircuitBreakerRegistry(registry *circuitbreaker.Registry) {
	s.cbRegistry = registry
}

// QueryCacheStats returns query cache statistics.
func (s *Service) QueryCacheStats() map[string]any {
	if s.queryCache == nil {
		return map[string]any{"enabled": false}
	}
	stats := s.queryCache.Stats()
	stats["enabled"] = s.cfg.QueryCacheEnabled
	return stats
}

// EmbeddingCacheStats returns node embedding cache statistics (for query-aware expansion).
func (s *Service) EmbeddingCacheStats() map[string]any {
	if s.embeddingCache == nil {
		return map[string]any{"enabled": false}
	}
	stats := s.embeddingCache.Stats()
	stats["enabled"] = s.cfg.QueryAwareExpansionEnabled
	return stats
}

// InvalidateSpaceCache invalidates all cached queries for a space.
// Call this after ingest, consolidate, or other mutations.
func (s *Service) InvalidateSpaceCache(spaceID string) int {
	if s.queryCache == nil {
		return 0
	}
	return s.queryCache.InvalidateSpace(spaceID)
}

// ClearQueryCache clears all entries from the query cache.
// Returns the number of entries that were cleared.
func (s *Service) ClearQueryCache() int {
	if s.queryCache == nil {
		return 0
	}
	count := s.queryCache.Len()
	s.queryCache.Clear()
	return count
}

// IsPrunableSpace returns true if a space_id has a test/temp prefix (test-, uats-).
func IsPrunableSpace(spaceID string) bool {
	return strings.HasPrefix(spaceID, "test-") || strings.HasPrefix(spaceID, "uats-")
}

// UpdateTapRootFreshness updates the TapRoot node for a space with the latest
// ingest timestamp and type. Creates the TapRoot if it doesn't exist.
// On creation, auto-detects prunable status from space_id prefix.
func (s *Service) UpdateTapRootFreshness(ctx context.Context, spaceID, ingestType string, prunable bool) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
			MERGE (t:TapRoot {space_id: $spaceId})
			ON CREATE SET t.name = 'tap_root', t.created_at = datetime(), t.prunable = $prunable
			SET t.last_ingest_at = datetime(),
			    t.last_ingest_type = $ingestType,
			    t.ingest_count = coalesce(t.ingest_count, 0) + 1
		`, map[string]any{
			"spaceId":    spaceID,
			"ingestType": ingestType,
			"prunable":   prunable,
		})
		return nil, err
	})
	if err != nil {
		return fmt.Errorf("update TapRoot freshness: %w", err)
	}
	return nil
}

// GetTapRootFreshness returns freshness properties for a space's TapRoot node.
// Returns nil map if the TapRoot doesn't exist.
func (s *Service) GetTapRootFreshness(ctx context.Context, spaceID string) (map[string]any, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	raw, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, `
			MATCH (t:TapRoot {space_id: $spaceId})
			RETURN t.last_ingest_at AS last_ingest_at,
			       t.last_ingest_type AS last_ingest_type,
			       t.ingest_count AS ingest_count,
			       t.created_at AS created_at
		`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("get TapRoot freshness: %w", err)
	}

	records := raw.([]*neo4j.Record)
	if len(records) > 0 {
		record := records[0]
		props := make(map[string]any)
		for _, key := range record.Keys {
			val, _ := record.Get(key)
			props[key] = val
		}
		return props, nil
	}
	return nil, nil // TapRoot doesn't exist
}

// GetAllTapRootFreshness returns freshness data for all TapRoot nodes.
func (s *Service) GetAllTapRootFreshness(ctx context.Context) ([]map[string]any, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	raw, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, `
			MATCH (t:TapRoot)
			RETURN t.space_id AS space_id,
			       t.last_ingest_at AS last_ingest_at,
			       t.last_ingest_type AS last_ingest_type,
			       t.ingest_count AS ingest_count
		`, nil)
		if err != nil {
			return nil, err
		}
		return result.Collect(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("get all TapRoot freshness: %w", err)
	}

	records := raw.([]*neo4j.Record)
	var results []map[string]any
	for _, record := range records {
		props := make(map[string]any)
		for _, key := range record.Keys {
			val, _ := record.Get(key)
			props[key] = val
		}
		results = append(results, props)
	}
	return results, nil
}

// Retrieve performs:
// 1) vector recall (top candidateK)
// 2) bounded neighborhood expansion (<= hopDepth, degree caps)
// 3) spreading activation in memory
// 4) scoring + topK selection
func (s *Service) Retrieve(ctx context.Context, req models.RetrieveRequest) (models.RetrieveResponse, error) {
	// Instrument retrieval latency for Prometheus metrics
	start := time.Now()
	defer func() {
		metrics.Metrics().RetrievalLatency.Observe(time.Since(start).Seconds())
	}()

	if req.SpaceID == "" {
		return models.RetrieveResponse{}, errors.New("space_id is required")
	}
	candK := req.CandidateK
	if candK <= 0 {
		candK = s.cfg.DefaultCandidateK
	}
	topK := req.TopK
	if topK <= 0 {
		topK = s.cfg.DefaultTopK
	}
	hopDepth := req.HopDepth
	if hopDepth <= 0 {
		hopDepth = s.cfg.DefaultHopDepth
	}
	// Bound hop depth by MaxHopDepth (configurable, default 3)
	maxHop := s.cfg.MaxHopDepth
	if maxHop <= 0 {
		maxHop = 3
	}
	if hopDepth > maxHop {
		hopDepth = maxHop
	}

	if len(req.QueryEmbedding) == 0 {
		// Intentionally not generating embeddings here; plug your embedder in upstream.
		return models.RetrieveResponse{}, errors.New("query_embedding is required (wire your embedder upstream)")
	}

	// Compute query-type aware retrieval hints (V0011 - Query-Aware Retrieval)
	// Uses LLM classifier when available, falls back to regex-based detection.
	hints := ComputeRetrievalHintsWithLLM(ctx, req.QueryText, s.cfg, s.queryClassifier)

	// Override temporal intent with explicit API fields if provided
	if req.TemporalAfter != "" || req.TemporalBefore != "" {
		hints.TemporalIntent = BuildExplicitTemporalIntent(req.TemporalAfter, req.TemporalBefore)
	}

	slog.Info("query type detected",
		"type", hints.QueryType, "seed_n", hints.SeedN, "hop_depth", hints.HopDepth,
		"vec_weight", hints.VectorWeight, "bm25_weight", hints.BM25Weight, "temporal", hints.TemporalIntent.Mode)

	// CONTEXT-LIVE-001: classifier→category dispatch. Live traffic never
	// passes ?category=, so the per-category protections (sparse-gate
	// overrides, context-column weights) only ever fired on benchmark
	// calls. Map the classifier's types onto the UVTS category vocabulary;
	// first mapped type wins (classifier emits types in order); explicit
	// request category always takes precedence. Runs BEFORE cacheReq/
	// CacheKey so the derived category participates in cache identity.
	if derived := deriveCategoryFromQueryType(req.Category, hints.QueryType, s.cfg.QueryClassifyCategoryMap); derived != req.Category {
		req.Category = derived
		slog.Debug("category derived from query classifier",
			"query_type", hints.QueryType, "category", derived)
	}

	// Override hopDepth with hints if request didn't specify
	if req.HopDepth <= 0 {
		hopDepth = hints.HopDepth
	}

	// Normalize request for cache key (fill in defaults)
	cacheReq := req
	cacheReq.CandidateK = candK
	cacheReq.TopK = topK
	cacheReq.HopDepth = hopDepth

	// Check query cache (skip for Jiminy-enabled requests and temporal queries).
	// scorerVersion segments the cache namespace so Phase 13 (Note 04) flag
	// flips between the legacy linear scorer and the RRF column-voting
	// aggregator do not serve stale entries from one scorer to a request
	// expecting the other.
	scorerVersion := s.scorerVersion()
	cacheKey := CacheKey(cacheReq, scorerVersion)
	slog.Info("query cache check", "enabled", s.cfg.QueryCacheEnabled, "jiminy", req.JiminyEnabled, "temporal", hints.TemporalIntent.Mode, "key", cacheKey[:16], "scorer_version", scorerVersion)
	if s.cfg.QueryCacheEnabled && !req.JiminyEnabled &&
		hints.TemporalIntent.Mode == TemporalModeNone && s.queryCache != nil {
		if cached, ok := s.queryCache.Get(cacheReq, scorerVersion); ok {
			// Ensure Debug map exists and set cache_hit flag
			if cached.Debug == nil {
				cached.Debug = make(map[string]any)
			}
			cached.Debug["cache_hit"] = true
			slog.Info("query cache HIT", "space", req.SpaceID, "query", req.QueryText[:min(50, len(req.QueryText))])
			return cached, nil
		}
		slog.Info("query cache MISS", "space", req.SpaceID)
	}

	// Build file filter from request
	filter := NewFileFilterFromRequest(req)

	// Compute space IDs for multi-space retrieval (Phase 105: Global Meta-Learning)
	spaceIDs := []string{req.SpaceID}
	if req.IncludeGlobalSpace {
		spaceIDs = append(spaceIDs, "mdemg-global")
	}

	// 1) Vector recall + BM25 in parallel (independent Neo4j queries)
	// For temporal queries, clean temporal keywords from BM25 query to avoid pollution
	bm25QueryText := req.QueryText
	if hints.TemporalIntent.Mode != TemporalModeNone {
		bm25QueryText = CleanTemporalKeywords(req.QueryText, hints.TemporalIntent.Keywords)
	}

	// Launch BM25 concurrently if hybrid retrieval is enabled
	type bm25Outcome struct {
		results []BM25Result
		err     error
	}
	bm25Ch := make(chan bm25Outcome, 1)
	if s.cfg.HybridRetrievalEnabled && req.QueryText != "" {
		// Translate BM25 query for better keyword matching (fail-open)
		if s.intentTranslator != nil {
			if translated, tErr := s.intentTranslator.Translate(ctx, bm25QueryText); tErr == nil && translated != "" {
				bm25QueryText = translated
			}
		}
		go func() {
			results, bm25Err := s.BM25Search(ctx, spaceIDs, bm25QueryText, s.cfg.BM25TopK, filter)
			bm25Ch <- bm25Outcome{results, bm25Err}
		}()
	} else {
		bm25Ch <- bm25Outcome{} // no BM25 needed
	}

	// Vector recall runs in main goroutine
	recallStart := time.Now()
	vectorCands, err := s.vectorRecall(ctx, spaceIDs, req.QueryEmbedding, candK, filter)
	if err != nil {
		<-bm25Ch // drain channel to avoid goroutine leak
		return models.RetrieveResponse{}, err
	}
	recallLatencyMs := time.Since(recallStart).Milliseconds()

	// Collect BM25 result and fuse
	var cands []Candidate
	bm25Count := 0
	bm25Out := <-bm25Ch
	if s.cfg.HybridRetrievalEnabled && req.QueryText != "" {
		if bm25Out.err != nil {
			slog.Warn("BM25 search failed, using vector-only", "error", bm25Out.err)
			cands = vectorCands
		} else {
			bm25Count = len(bm25Out.results)
			fused := ReciprocalRankFusion(vectorCands, bm25Out.results, hints.VectorWeight, hints.BM25Weight, s.cfg.RRFConstant)
			cands = ConvertFusedToCandidates(fused)
		}
	} else {
		cands = vectorCands
	}

	// Hard-mode temporal filter: keep only candidates within the time range
	if s.cfg.TemporalEnabled && s.cfg.TemporalHardFilterEnabled &&
		hints.TemporalIntent.Mode == TemporalModeHard {
		preFilterCount := len(cands)
		cands = FilterCandidatesByTime(cands, hints.TemporalIntent.Constraint)
		slog.Info("temporal hard filter applied",
			"before", preFilterCount, "after", len(cands), "constraint", hints.TemporalIntent.Constraint.Description)
	}

	if len(cands) == 0 {
		return models.RetrieveResponse{SpaceID: req.SpaceID, Results: []models.RetrieveResult{}}, nil
	}

	// Seeds: use query-type aware seed count (V0011)
	seedN := minInt(hints.SeedN, len(cands))
	seedIDs := make([]string, 0, seedN)
	for i := 0; i < seedN; i++ {
		seedIDs = append(seedIDs, cands[i].NodeID)
	}

	// 2) Expansion: iterative 1-hop fetch up to hopDepth
	// Skip expansion for symbol lookups (V0011 - Query-Aware Retrieval)
	edges := make([]Edge, 0, 1024)
	seenEdge := map[string]struct{}{}
	frontier := append([]string{}, seedIDs...)
	seenNode := map[string]struct{}{}
	for _, id := range frontier {
		seenNode[id] = struct{}{}
	}

	// Use query-type aware hop depth
	effectiveHopDepth := hopDepth
	if !hints.EnableExpansion {
		effectiveHopDepth = 0 // Skip expansion entirely for symbol lookups
		slog.Info("skipping graph expansion for query type", "type", hints.QueryType)
	}

	for d := 0; d < effectiveHopDepth; d++ {
		if len(frontier) == 0 {
			break
		}

		// Get edge types and attention flag for this hop depth (V0010 - Hybrid Edge Type Strategy)
		edgeTypes, applyAttention := s.getEdgeTypesForHop(d)

		// Fetch edges with the appropriate edge types for this hop
		var batchEdges []Edge
		var nextNodes []string
		var fetchErr error
		if s.cfg.EdgeTypeStrategy == "all" {
			// Use original function for "all" strategy (backward compatible)
			batchEdges, nextNodes, fetchErr = s.fetchOutgoingEdges(ctx, spaceIDs, frontier)
		} else {
			// Use type-filtered function for other strategies
			batchEdges, nextNodes, fetchErr = s.fetchOutgoingEdgesWithTypes(ctx, spaceIDs, frontier, edgeTypes)
		}
		if fetchErr != nil {
			return models.RetrieveResponse{}, fetchErr
		}

		// Apply query-aware attention re-ranking if enabled for this hop (V0009 + V0010)
		// This uses cosine similarity between query and destination nodes
		// to prioritize query-relevant neighbors over purely high-weight edges
		// Note: applyAttention is determined by the hybrid edge type strategy
		if applyAttention && len(req.QueryEmbedding) > 0 {
			batchEdges, err = ReRankEdgesByAttention(
				ctx,
				s.driver,
				s.embeddingCache,
				req.SpaceID,
				req.QueryEmbedding,
				batchEdges,
				s.cfg.MaxNeighborsPerNode,
				s.cfg,
			)
			if err != nil {
				// Log warning but continue with original edges
				slog.Warn("query-aware attention re-ranking failed", "error", err)
			}

			// Rebuild nextNodes from re-ranked edges
			nextNodes = nextNodes[:0]
			for _, e := range batchEdges {
				nextNodes = append(nextNodes, e.Dst)
			}
		}

		frontier = frontier[:0]
		for _, e := range batchEdges {
			key := e.Src + "|" + e.RelType + "|" + e.Dst
			if _, ok := seenEdge[key]; ok {
				continue
			}
			seenEdge[key] = struct{}{}
			edges = append(edges, e)
		}
		for _, nid := range nextNodes {
			if _, ok := seenNode[nid]; ok {
				continue
			}
			seenNode[nid] = struct{}{}
			frontier = append(frontier, nid)
		}
	}

	// 3) Activation physics with edge-type attention
	// Build query context for attention modulation
	queryCtx := QueryContext{
		QueryText:   req.QueryText,
		IsCodeQuery: isCodeQuery(req.QueryText),
		IsArchQuery: isArchitectureQuery(req.QueryText),
	}

	// Build hop min weights from config for local-first activation spreading
	hopMinWeights := []float64{
		s.cfg.ActivationHop0MinWeight,
		s.cfg.ActivationHop1MinWeight,
		s.cfg.ActivationHop2MinWeight,
	}

	// F9: Apply direction-aware weight scaling for asymmetric learning edges
	if s.cfg.LearningAsymmetricEnabled {
		edges = applyAsymmetricWeights(edges)
	}

	// Compute attention weights or use default (original behavior)
	var act map[string]float64
	if s.cfg.EdgeAttentionEnabled {
		attention := ComputeEdgeAttention(queryCtx, s.cfg)
		act = SpreadingActivationWithAttention(cands, edges, s.cfg.ActivationSteps, s.cfg.ActivationLambda, attention, hopMinWeights, DimWeightsFromConfig(s.cfg))
	} else {
		// Fallback to original behavior (CO_ACTIVATED_WITH only)
		act = SpreadingActivation(cands, edges, s.cfg.ActivationSteps, s.cfg.ActivationLambda, hopMinWeights, DimWeightsFromConfig(s.cfg))
	}

	// 4) Initial ranking (pass query text for path-based boosting)
	// Request more candidates for re-ranking if enabled
	initialTopK := topK
	if s.cfg.RerankEnabled && req.QueryText != "" {
		initialTopK = s.cfg.RerankTopN
	}

	// Phase 14.2 Epic 4: strict-context pre-aggregation filter. When
	// req.StrictContextMode is true AND the query carries a fingerprint,
	// drop candidates whose Jaccard similarity falls below the configured
	// threshold BEFORE scoring. Has no effect on non-RRF paths or when the
	// query fingerprint is empty (graceful degradation).
	if req.StrictContextMode && len(req.QueryContextFingerprint) > 0 && s.cfg.RetrievalContextStrictThreshold > 0 {
		threshold := s.cfg.RetrievalContextStrictThreshold
		filtered := make([]Candidate, 0, len(cands))
		for _, c := range cands {
			// CONTEXT-LIVE-001 version guard: a candidate fingerprinted
			// against a different catalog version carries incomparable
			// bits — treat as below-threshold rather than Jaccard noise.
			if req.QueryContextFingerprintVersion > 0 &&
				c.ContextFingerprintVersion != req.QueryContextFingerprintVersion {
				continue
			}
			if JaccardFingerprint(req.QueryContextFingerprint, c.ContextFingerprintActive) >= threshold {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) == 0 {
			// All candidates filtered out — fail open: keep the original set
			// rather than returning a hard empty (matches the Note 05 §3.2
			// "no-eligible-context" fallback). Operators get a log line they
			// can spot when tuning the threshold.
			slog.Warn("strict_context filter zeroed candidate pool; falling back to unfiltered set",
				"threshold", threshold, "before", len(cands))
		} else {
			slog.Debug("strict_context filter applied",
				"threshold", threshold, "before", len(cands), "after", len(filtered))
			cands = filtered
		}
	}

	// Phase 13 (Note 04 Column-Voting Retrieval) scorer fork. When the
	// flag is on, the RRF aggregator replaces the linear scorer. The
	// aggregator's `consensus_strength` signal is captured for downstream
	// consumers (rerank, DH-005) and the V0017 retrieval_audit row.
	//
	// Jiminy compatibility: when Jiminy is requested, we keep using the
	// linear scorer's breakdown-enabled path because Jiminy's explanation
	// surfaces the per-component breakdown that RRF doesn't produce.
	// Jiminy + RRF integration is deferred to a follow-up sprint.
	var scoredCands []ScoredCandidate
	var results []models.RetrieveResult
	var consensusResult ConsensusResult // populated only on RRF path
	useRRF := s.cfg.RetrievalColumnVotingEnabled && !req.JiminyEnabled
	switch {
	case req.JiminyEnabled:
		scoredCands = ScoreAndRankWithBreakdown(cands, act, edges, initialTopK, s.cfg, req.QueryText, hints)
		results = make([]models.RetrieveResult, len(scoredCands))
		for i, sc := range scoredCands {
			results[i] = sc.RetrieveResult
		}
	case useRRF:
		var rrfErr error
		results, consensusResult, rrfErr = s.ScoreAndRankRRF(
			ctx, cands, act, initialTopK,
			req.QueryEmbedding, req.QueryText, spaceIDs, filter,
			req.QueryContextFingerprint,
			req.QueryContextFingerprintVersion,
			req.Category,
		)
		if rrfErr != nil {
			// Fail open to legacy scorer rather than the user.
			slog.Warn("RRF scorer failed, falling back to linear", "error", rrfErr)
			results = ScoreAndRank(cands, act, edges, initialTopK, s.cfg, req.QueryText, hints)
		} else {
			// Emit Phase 13 metrics. consensusResult.PerColumnLatency is
			// populated for every attempted column (success or fail) so the
			// histogram + per-column failure counter stay aligned.
			stdMetrics := metrics.Metrics()
			stdMetrics.RetrievalConsensusStrength.Observe(consensusResult.AggregateConsensus)
			for col, lat := range consensusResult.PerColumnLatency {
				stdMetrics.RetrievalColumnLatency(col).Observe(lat.Seconds())
			}
			for col, err := range consensusResult.PerColumnError {
				if err != nil {
					reason := "error"
					if err.Error() != "" && len(err.Error()) > 0 {
						reason = "error"
					}
					stdMetrics.RetrievalColumnFailedTotal(col, reason).Inc()
				}
			}
		}
	default:
		results = ScoreAndRank(cands, act, edges, initialTopK, s.cfg, req.QueryText, hints)
	}
	_ = consensusResult // Audit row write happens at the end of Retrieve (Epic 6)

	// Phase 14 Epic 1 — Note 06 sparse activation gate. Cuts the candidate
	// list to those whose score crosses the per-call activation percentile.
	// Operates pre-rerank so the LLM-bound rerank prompt receives a 4-6×
	// smaller input (the dominant quantitative win — see
	// docs/development/post-ft-lora/phase_14_score_distribution_analysis.md).
	// Per-request override via SparseEnabled/SparsePercentile on the request.
	var (
		gateMeta    SparseGateMetadata
		gateDropped []models.RetrieveResult
		gateFired   bool
	)
	gateOpts := SparseGateOpts{
		Enabled:    s.cfg.SparseRetrievalEnabled,
		Percentile: s.cfg.SparseActivationPercentile,
		MinActive:  s.cfg.SparseMinActive,
		MaxActive:  s.cfg.SparseMaxActive,
		// Phase 14.1 Epic 2 — translate config map to gate-side type at the
		// call site (cycle-safe: gate.go doesn't import config). Empty map →
		// no overrides applied; same outcome as Phase 14.
		CategoryOverrides: translateCategoryOverrides(s.cfg.SparseGateCategoryOverrides),
		Category:          req.Category,
	}
	if req.SparseOverridePresent {
		gateOpts.Enabled = req.SparseEnabled
	}
	if req.SparsePercentile > 0 {
		gateOpts.Percentile = req.SparsePercentile
	}
	if gateOpts.Enabled && len(results) > 0 {
		results, gateDropped, gateMeta = ApplySparseGate(results, gateOpts)
		gateFired = true
		stdMetrics := metrics.Metrics()
		stdMetrics.SparseGateActiveCount.Observe(float64(gateMeta.ActiveCount))
		stdMetrics.SparseGateThreshold.Observe(gateMeta.Threshold)
		if gateMeta.InputCount > 0 {
			stdMetrics.SparseGateDroppedFraction.Observe(
				float64(gateMeta.DroppedCount) / float64(gateMeta.InputCount),
			)
		}
		// Persist to V0019 sparse_gate_metrics for durability beyond the
		// Prometheus reset cycle. Phase 14.1 retunes from this hypertable.
		if s.sparseGateRecorder != nil {
			s.sparseGateRecorder.RecordGate(req.SpaceID, gateMeta, scorerVersion)
		}
	}

	// 5) Reasoning Module Processing (if available and query text provided)
	var reasoningModuleID string
	var reasoningLatencyMs float64
	var reasoningTokens int
	if s.reasoningProvider != nil && s.reasoningProvider.Available() && req.QueryText != "" && len(results) > 0 {
		reasoningReq := ReasoningRequest{
			QueryText:  req.QueryText,
			Candidates: results,
			TopK:       initialTopK,
			Context: map[string]string{
				"space_id": req.SpaceID,
			},
		}

		reasoningResult, reasoningErr := s.reasoningProvider.Process(ctx, reasoningReq)
		if reasoningErr != nil {
			slog.Warn("reasoning module processing failed, using initial results", "error", reasoningErr)
		} else if len(reasoningResult.Results) > 0 {
			results = reasoningResult.Results
			reasoningModuleID = reasoningResult.ModuleID
			reasoningLatencyMs = reasoningResult.LatencyMs
			reasoningTokens = reasoningResult.TokensUsed
		}
	}

	// 6) LLM Re-ranking (if enabled and query text provided)
	// Snapshot pre-rerank results for contrastive training data
	preRerankResults := make([]models.RetrieveResult, len(results))
	copy(preRerankResults, results)
	var rerankLatencyMs float64
	var rerankTokens int
	wasReranked := false
	if s.cfg.RerankEnabled && req.QueryText != "" && len(results) > 0 {
		// Store pre-rerank scores for delta calculation
		preRerankScores := make(map[string]float64)
		for _, r := range results {
			preRerankScores[r.NodeID] = r.Score
		}

		rerankResult, rerankErr := s.Rerank(ctx, RerankRequest{
			SpaceID:    req.SpaceID,
			Query:      req.QueryText,
			Candidates: results,
			TopN:       s.cfg.RerankTopN,
			ReturnK:    topK,
		})
		if rerankErr != nil {
			// Log warning but continue with initial results
			slog.Warn("LLM rerank failed, using initial results", "error", rerankErr)
		} else {
			wasReranked = true
			results = rerankResult.Results
			rerankLatencyMs = rerankResult.LatencyMs
			rerankTokens = rerankResult.TokensUsed

			// Update breakdowns with rerank delta if Jiminy is enabled
			if req.JiminyEnabled && scoredCands != nil {
				// Create a map for quick lookup
				breakdownMap := make(map[string]*ScoreBreakdown)
				for i := range scoredCands {
					breakdownMap[scoredCands[i].NodeID] = &scoredCands[i].Breakdown
				}

				// Calculate rerank delta for each result
				for i := range results {
					nodeID := results[i].NodeID
					if bd, ok := breakdownMap[nodeID]; ok {
						preScore := preRerankScores[nodeID]
						bd.RerankDelta = results[i].Score - preScore
						bd.FinalScore = results[i].Score
					}
				}
			}
		}
	}

	// Truncate to topK if needed
	if len(results) > topK {
		results = results[:topK]
	}

	// Apply normalized confidence to final results
	// This must happen AFTER all post-processing (reasoning modules, reranking, truncation)
	// to ensure percentiles reflect the final ordering
	ApplyNormalizedConfidenceToResults(results)

	// Generate Jiminy explanations if enabled
	if req.JiminyEnabled && scoredCands != nil {
		// Create a map for quick lookup
		breakdownMap := make(map[string]ScoreBreakdown)
		for _, sc := range scoredCands {
			breakdownMap[sc.NodeID] = sc.Breakdown
		}

		for i := range results {
			nodeID := results[i].NodeID
			if bd, ok := breakdownMap[nodeID]; ok {
				path := DetermineRetrievalPath(bd, wasReranked)
				jiminy := GenerateJiminyExplanation(bd, path)
				results[i].Jiminy = &models.JiminyExplanation{
					Rationale:           jiminy.Rationale,
					Confidence:          jiminy.Confidence,
					RetrievalPath:       jiminy.RetrievalPath,
					ContributingModules: jiminy.ContributingModules,
					ScoreBreakdown:      jiminy.ScoreBreakdown,
				}
			}
		}
	}

	resp := models.RetrieveResponse{
		SpaceID: req.SpaceID,
		Results: results,
		Debug: map[string]any{
			"candidate_k":                candK,
			"seed_n":                     seedN,
			"edges_fetched":              len(edges),
			"hop_depth":                  effectiveHopDepth,
			"query_type":                 hints.QueryType,
			"vector_weight":              hints.VectorWeight,
			"bm25_weight":                hints.BM25Weight,
			"hybrid_enabled":             s.cfg.HybridRetrievalEnabled,
			"vector_count":               len(vectorCands),
			"bm25_count":                 bm25Count,
			"fused_count":                len(cands),
			"reasoning_module":           reasoningModuleID,
			"reasoning_latency_ms":       reasoningLatencyMs,
			"reasoning_tokens":           reasoningTokens,
			"rerank_enabled":             s.cfg.RerankEnabled,
			"rerank_latency_ms":          rerankLatencyMs,
			"rerank_tokens":              rerankTokens,
			"jiminy_enabled":             req.JiminyEnabled,
			"cache_hit":                  false,
			"edge_type_strategy":         s.cfg.EdgeTypeStrategy,
			"hybrid_switch_hop":          s.cfg.HybridSwitchHop,
			"temporal_mode":              string(hints.TemporalIntent.Mode),
			"temporal_keywords":          hints.TemporalIntent.Keywords,
			"temporal_confidence":        hints.TemporalIntent.Confidence,
			"temporal_source_type_decay": s.cfg.TemporalSourceTypeDecayEnabled,
			"temporal_stale_ref_days":    s.cfg.TemporalStaleRefDays,
		},
	}

	// Add temporal constraint description to debug if present
	if hints.TemporalIntent.Constraint != nil {
		resp.Debug["temporal_constraint"] = hints.TemporalIntent.Constraint.Description
	}

	// Phase 14 Epic 1 — surface sparse-gate state to debug. Always present
	// when the gate fired so operators can confirm it ran; below_threshold
	// candidates only attached when JiminyEnabled (preserves Phase 13
	// breakdown-debug pattern). The gate metadata is small and useful for
	// ad-hoc troubleshooting even outside Jiminy.
	if gateFired {
		resp.Debug["sparse_gate_active_count"] = gateMeta.ActiveCount
		resp.Debug["sparse_gate_dropped_count"] = gateMeta.DroppedCount
		resp.Debug["sparse_gate_threshold"] = gateMeta.Threshold
		resp.Debug["sparse_gate_percentile"] = gateMeta.PercentileApplied
		resp.Debug["sparse_gate_floor_applied"] = gateMeta.FloorApplied
		resp.Debug["sparse_gate_ceiling_applied"] = gateMeta.CeilingApplied
		if req.JiminyEnabled && len(gateDropped) > 0 {
			// Truncate node-id list to keep response size bounded — the
			// scores tell the operator the rest. 50 is well below MAX_ACTIVE.
			cap := len(gateDropped)
			if cap > 50 {
				cap = 50
			}
			belowIDs := make([]string, 0, cap)
			belowScores := make([]float64, 0, cap)
			for i := 0; i < cap; i++ {
				belowIDs = append(belowIDs, gateDropped[i].NodeID)
				belowScores = append(belowScores, gateDropped[i].Score)
			}
			resp.Debug["below_threshold_node_ids"] = belowIDs
			resp.Debug["below_threshold_scores"] = belowScores
		}
	}

	// Store in query cache (skip for Jiminy-enabled and temporal requests)
	if s.cfg.QueryCacheEnabled && !req.JiminyEnabled &&
		hints.TemporalIntent.Mode == TemporalModeNone && s.queryCache != nil {
		s.queryCache.Put(cacheReq, scorerVersion, resp)
		slog.Info("query cache PUT", "space", req.SpaceID, "query", req.QueryText[:min(50, len(req.QueryText))], "cache_size", s.queryCache.Len())
	}

	// Record retrieval event for contrastive training data
	s.recordRetrievalEvent(ctx, req, vectorCands, bm25Out.results, preRerankResults,
		results, wasReranked, rerankLatencyMs, recallLatencyMs, time.Since(start).Milliseconds())

	// Phase 13 — write retrieval_audit row when enabled. Non-fatal on
	// write error: the user-facing retrieve already succeeded and is
	// returning; an audit-write failure shouldn't disrupt it.
	if s.cfg.RetrievalAuditEnabled && s.retrievalAuditWriter != nil {
		topKIDs := make([]string, 0, len(results))
		for _, r := range results {
			topKIDs = append(topKIDs, r.NodeID)
		}
		auditRec := RetrievalAuditRecord{
			SpaceID:           req.SpaceID,
			QueryTextHash:     hashQueryText(req.QueryText),
			ScorerVersion:     scorerVersion,
			ConsensusStrength: consensusResult.AggregateConsensus,
			PerColumnLatency:  consensusResult.PerColumnLatency,
			ColumnsQueried:    consensusResult.ColumnsQueried,
			ColumnsReturned:   consensusResult.ColumnsReturned,
			TopKNodeIDs:       topKIDs,
			TotalLatencyMs:    time.Since(start).Milliseconds(),
		}
		// Fire-and-forget — the writer logs errors internally. Use a fresh
		// context.Background-derived context because the request-scoped ctx
		// may be cancelled by the time the response returns to the caller;
		// we want the audit write to complete regardless. Same pattern as
		// internal/conversation/conflict_tracker.go.
		go func() { //nolint:gosec // G118: audit row outlives the request-scoped ctx by design
			ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := s.retrievalAuditWriter.Write(ctx2, auditRec); err != nil {
				slog.Warn("retrieval_audit: write failed", "error", err)
			}
		}()
	}

	return resp, nil
}

// hashQueryText returns a stable 16-hex-char digest of the query text.
// Avoids storing PII in retrieval_audit while preserving the ability to
// bucket by query template.
func hashQueryText(s string) string {
	if s == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8]) // 8 bytes = 16 hex chars
}

// recordRetrievalEvent sends a retrieval pipeline event to the recorder if one is set.
func (s *Service) recordRetrievalEvent(ctx context.Context, req models.RetrieveRequest,
	vectorCands []Candidate, bm25Results []BM25Result, preRerankResults []models.RetrieveResult,
	results []models.RetrieveResult, wasReranked bool,
	rerankLatencyMs float64, recallLatencyMs int64, totalLatencyMs int64) {
	if s.retrievalRecorder == nil {
		return
	}
	// Build recall arrays (vector recall)
	recallIDs := make([]string, len(vectorCands))
	recallScores := make([]float64, len(vectorCands))
	for i, c := range vectorCands {
		recallIDs[i] = c.NodeID
		recallScores[i] = c.VectorSim
	}
	// Build BM25 arrays
	bm25IDs := make([]string, len(bm25Results))
	bm25Scores := make([]float64, len(bm25Results))
	for i, b := range bm25Results {
		bm25IDs[i] = b.NodeID
		bm25Scores[i] = b.Score
	}
	// Build result arrays (final post-rerank)
	resultIDs := make([]string, len(results))
	resultScores := make([]float64, len(results))
	for i, r := range results {
		resultIDs[i] = r.NodeID
		resultScores[i] = r.Score
	}

	event := RetrievalEvent{
		Time:            time.Now(),
		SpaceID:         req.SpaceID,
		CallSite:        "retrieve",
		QueryText:       req.QueryText,
		QueryHash:       QueryHash(req.QueryText),
		RecallNodeIDs:   recallIDs,
		RecallScores:    recallScores,
		RecallK:         len(vectorCands),
		BM25NodeIDs:     bm25IDs,
		BM25Scores:      bm25Scores,
		ResultNodeIDs:   resultIDs,
		ResultScores:    resultScores,
		ResultCount:     len(results),
		RecallLatencyMs: int(recallLatencyMs), //nolint:gosec // narrowing int64→int is fine for ms values
		TotalLatencyMs:  int(totalLatencyMs),  //nolint:gosec // narrowing int64→int is fine for ms values
		GuidanceID:      llmclient.GuidanceIDFromContext(ctx),
	}

	if wasReranked {
		event.RerankLatencyMs = int(rerankLatencyMs)
		event.RerankModel = s.cfg.RerankModel
		// Pre-rerank IDs/scores (input to reranker)
		preIDs := make([]string, len(preRerankResults))
		preScores := make([]float64, len(preRerankResults))
		for i, r := range preRerankResults {
			preIDs[i] = r.NodeID
			preScores[i] = r.Score
		}
		event.RerankNodeIDs = preIDs
		event.RerankScores = preScores
	}

	s.retrievalRecorder.RecordRetrieval(ctx, event)
}

type Candidate struct {
	NodeID        string
	Path          string
	Name          string
	Summary       string
	UpdatedAt     time.Time
	CanonicalTime time.Time // Phase 2: content-relevant time (zero = use UpdatedAt)
	Confidence    float64
	VectorSim     float64
	BM25Score     float64  // Actual BM25 score (0.0 for vector-only)
	RRFScore      float64  // Fused RRF score (authoritative ranking signal)
	Layer         int      // 0=base, 1=hidden/concern, 2+=concept
	Tags          []string // Tags for scoring boosts (e.g., "config")

	// Phase 14.2 Epic 4 — Sparse context fingerprint (Note 05). Populated
	// from MemoryNode.context_fingerprint_active when fetched; nil for
	// pre-Phase-14.2 nodes (cold-start fallback). Read by ContextColumn
	// for Jaccard scoring; ignored by other columns.
	ContextFingerprintActive  []uint16
	ContextFingerprintVersion int
}

// SimilarNode represents a node returned from vector similarity search
type SimilarNode struct {
	NodeID string
	Score  float64
}

func (s *Service) vectorRecall(ctx context.Context, spaceIDs []string, q []float32, k int, filter FileFilter) ([]Candidate, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceIds": spaceIDs,
		"k":        k,
		"q":        q,
		"index":    s.cfg.VectorIndexName,
	}

	// Add filter parameters if specified
	filterClause := ""
	if !filter.IsEmpty() {
		filterClause = filter.BuildCypherFilter()
		if len(filter.IncludeExtensions) > 0 {
			params["includeExtensions"] = filter.IncludeExtensions
		}
		if len(filter.ExcludeExtensions) > 0 {
			params["excludeExtensions"] = filter.ExcludeExtensions
		}
	}

	cypher := `WITH $q AS q
CALL db.index.vector.queryNodes($index, $k, q)
YIELD node, score
WHERE node.space_id IN $spaceIds AND NOT coalesce(node.is_archived, false)` + filterClause + `
RETURN node.node_id AS node_id,
       node.path AS path,
       node.name AS name,
       coalesce(node.summary,'') AS summary,
       coalesce(node.confidence,0.6) AS confidence,
       coalesce(node.updated_at, datetime()) AS updated_at,
       coalesce(node.canonical_time, node.updated_at, datetime()) AS canonical_time,
       coalesce(node.layer, 0) AS layer,
       coalesce(node.tags, []) AS tags,
       coalesce(node.context_fingerprint_active, []) AS context_fingerprint_active,
       coalesce(node.context_fingerprint_version, 0) AS context_fingerprint_version,
       score AS score
ORDER BY score DESC`

	timer := StartQueryTimer("vectorRecall", cypher, params)
	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		cands := make([]Candidate, 0, k)
		for res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			path, _ := rec.Get("path")
			name, _ := rec.Get("name")
			sum, _ := rec.Get("summary")
			conf, _ := rec.Get("confidence")
			upd, _ := rec.Get("updated_at")
			canonicalAny, _ := rec.Get("canonical_time")
			layer, _ := rec.Get("layer")
			tagsAny, _ := rec.Get("tags")
			fpAny, _ := rec.Get("context_fingerprint_active")
			fpVerAny, _ := rec.Get("context_fingerprint_version")
			sc, _ := rec.Get("score")

			ct := Candidate{
				NodeID:                    fmt.Sprint(nid),
				Path:                      fmt.Sprint(path),
				Name:                      fmt.Sprint(name),
				Summary:                   fmt.Sprint(sum),
				Confidence:                toFloat64(conf, 0.6),
				VectorSim:                 toFloat64(sc, 0),
				Layer:                     toInt(layer, 0),
				Tags:                      toStringSlice(tagsAny),
				ContextFingerprintActive:  toUint16Slice(fpAny),
				ContextFingerprintVersion: toInt(fpVerAny, 0),
			}
			// neo4j returns time as neo4j.LocalDateTime or time.Time depending on driver
			switch v := upd.(type) {
			case time.Time:
				ct.UpdatedAt = v
			default:
				ct.UpdatedAt = time.Now()
			}
			// Parse canonical_time (Phase 2 Temporal)
			switch v := canonicalAny.(type) {
			case time.Time:
				ct.CanonicalTime = v
			default:
				ct.CanonicalTime = ct.UpdatedAt // fallback
			}
			cands = append(cands, ct)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return cands, nil
	})
	timer.End()
	if err != nil {
		return nil, err
	}
	typed, ok := outAny.([]Candidate)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", outAny)
	}
	return typed, nil
}

// FindSimilarNodes queries the vector index for nodes similar to the provided embedding,
// excluding the specified node (self-match). Used for semantic edge creation on ingest.
func (s *Service) FindSimilarNodes(ctx context.Context, spaceID string, embedding []float32, excludeNodeID string, topN int) ([]SimilarNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Use a larger k value to account for cross-space filtering.
	// The vector index returns top-k across ALL spaces, then we filter by space_id.
	// Using DefaultCandidateK ensures we have enough candidates after filtering.
	queryK := s.cfg.DefaultCandidateK
	if queryK < topN+1 {
		queryK = topN + 1
	}

	params := map[string]any{
		"spaceId":       spaceID,
		"k":             queryK,
		"q":             embedding,
		"index":         s.cfg.VectorIndexName,
		"excludeNodeId": excludeNodeID,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `WITH $q AS q
CALL db.index.vector.queryNodes($index, $k, q)
YIELD node, score
WHERE node.space_id = $spaceId AND node.node_id <> $excludeNodeId AND NOT coalesce(node.is_archived, false)
RETURN node.node_id AS node_id, score
ORDER BY score DESC
LIMIT $k`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		results := make([]SimilarNode, 0, topN)
		for res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			sc, _ := rec.Get("score")

			sn := SimilarNode{
				NodeID: fmt.Sprint(nid),
				Score:  toFloat64(sc, 0),
			}
			results = append(results, sn)
			// Stop after topN results
			if len(results) >= topN {
				break
			}
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return results, nil
	})
	if err != nil {
		return nil, err
	}
	typed, ok := outAny.([]SimilarNode)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", outAny)
	}
	return typed, nil
}

// CreateAssociatedWithEdge creates or updates an ASSOCIATED_WITH edge between two nodes.
// Uses MERGE to avoid duplicates. On create, sets initial weight from config.
// On match, increments weight and evidence_count.
func (s *Service) CreateAssociatedWithEdge(ctx context.Context, spaceID, fromNodeID, toNodeID string, similarityScore float64) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId":         spaceID,
		"fromNodeId":      fromNodeID,
		"toNodeId":        toNodeID,
		"initialWeight":   s.cfg.SemanticEdgeInitialWeight,
		"similarityScore": similarityScore,
	}

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (a:MemoryNode {space_id:$spaceId, node_id:$fromNodeId})
MATCH (b:MemoryNode {space_id:$spaceId, node_id:$toNodeId})
MERGE (a)-[r:ASSOCIATED_WITH]->(b)
ON CREATE SET
    r.edge_id = randomUUID(),
    r.space_id = $spaceId,
    r.weight = $initialWeight,
    r.dim_semantic = $similarityScore,
    r.evidence_count = 1,
    r.status = 'active',
    r.created_at = datetime(),
    r.updated_at = datetime()
ON MATCH SET
    r.weight = CASE WHEN r.weight + ($similarityScore * 0.1) > 1.0 THEN 1.0 ELSE r.weight + ($similarityScore * 0.1) END,
    r.evidence_count = r.evidence_count + 1,
    r.updated_at = datetime()`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		// Consume the result
		_, err = res.Consume(ctx)
		return nil, err
	})
	return err
}

type Edge struct {
	Src             string
	Dst             string
	RelType         string
	Weight          float64
	DimSemantic     float64
	DimTemporal     float64
	DimCoactivation float64
	Direction       string // "forward", "backward", or "bidirectional" (F9: asymmetric learning)
	UpdatedAt       time.Time
}

// applyAsymmetricWeights scales edge weights based on direction for asymmetric Hebbian learning (F9).
// - "forward"       → full weight (1.0x)
// - "backward"      → half weight (0.5x) — reverse edges carry less signal
// - "bidirectional" → full weight (1.0x, preserves legacy behaviour)
func applyAsymmetricWeights(edges []Edge) []Edge {
	out := make([]Edge, len(edges))
	copy(out, edges)
	for i := range out {
		if out[i].Direction == "backward" {
			out[i].Weight *= 0.5
		}
	}
	return out
}

// edgeTraversalPack bundles edges and next-hop node IDs from graph traversal.
type edgeTraversalPack struct {
	Edges []Edge
	Next  []string
}

// fetchOutgoingEdgesWithTypes is a variant that uses specific edge types instead of AllowedRelationshipTypes.
// Used by the hybrid edge type strategy to fetch different edge types at different hop depths.
func (s *Service) fetchOutgoingEdgesWithTypes(ctx context.Context, spaceIDs []string, nodeIDs []string, edgeTypes []string) ([]Edge, []string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Get decay parameters with defaults
	decayPerDay := s.cfg.LearningDecayPerDay
	if decayPerDay <= 0 {
		decayPerDay = 0.05 // 5% decay per day
	}
	pruneThreshold := s.cfg.LearningPruneThreshold
	if pruneThreshold <= 0 {
		pruneThreshold = 0.05 // prune edges below 0.05 weight
	}

	params := map[string]any{
		"spaceIds":       spaceIDs,
		"nodeIds":        nodeIDs,
		"allowed":        edgeTypes, // Use provided edge types instead of AllowedRelationshipTypes
		"maxNbr":         s.cfg.MaxNeighborsPerNode,
		"maxTotal":       s.cfg.MaxTotalEdgesFetched,
		"decayPerDay":    decayPerDay,
		"pruneThreshold": pruneThreshold,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Same query as fetchOutgoingEdges, but uses provided edge types
		cypher := `UNWIND $nodeIds AS sid
MATCH (src:MemoryNode)
WHERE src.space_id IN $spaceIds AND src.node_id = sid
CALL {
  WITH src
  MATCH (src)-[r]->(dst:MemoryNode)
  WHERE dst.space_id IN $spaceIds
    AND type(r) IN $allowed AND coalesce(r.status,'active')='active'
  WITH src, r, dst, type(r) AS relType,
       CASE WHEN type(r) = 'CO_ACTIVATED_WITH' THEN
         duration.between(coalesce(r.last_activated_at, r.created_at, datetime()), datetime()).days
       ELSE 0 END AS daysSinceActive,
       coalesce(r.weight, 0.0) AS rawWeight,
       coalesce(r.evidence_count, 1) AS evidenceCount,
       coalesce(r.surprise_factor, 1.0) AS surpriseFactor
  WITH src, r, dst, relType, daysSinceActive, rawWeight, evidenceCount, surpriseFactor,
       CASE WHEN relType = 'CO_ACTIVATED_WITH' AND daysSinceActive > 0 THEN
         rawWeight * (CASE WHEN (1.0 - $decayPerDay / sqrt(toFloat(evidenceCount) * surpriseFactor)) <= 0 THEN 0.01 ELSE (1.0 - $decayPerDay / sqrt(toFloat(evidenceCount) * surpriseFactor)) END ^ daysSinceActive)
       ELSE rawWeight END AS decayedWeight
  WHERE NOT (relType = 'CO_ACTIVATED_WITH' AND decayedWeight < $pruneThreshold)
  RETURN src.node_id AS s, dst.node_id AS d, relType AS t,
         decayedWeight AS w,
         coalesce(r.dim_semantic,0.0) AS ds,
         coalesce(r.dim_temporal,0.0) AS dt,
         coalesce(r.dim_coactivation,0.0) AS dc,
         coalesce(r.direction, 'bidirectional') AS dir,
         coalesce(r.updated_at, datetime()) AS upd
  ORDER BY w DESC
  LIMIT $maxNbr
}
RETURN s, d, t, w, ds, dt, dc, dir, upd
LIMIT $maxTotal`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]Edge, 0, 1024)
		next := make([]string, 0, 1024)
		seenNext := map[string]struct{}{}
		for res.Next(ctx) {
			rec := res.Record()
			s, _ := rec.Get("s")
			d, _ := rec.Get("d")
			t, _ := rec.Get("t")
			w, _ := rec.Get("w")
			ds, _ := rec.Get("ds")
			dt, _ := rec.Get("dt")
			dc, _ := rec.Get("dc")
			dir, _ := rec.Get("dir")
			upd, _ := rec.Get("upd")

			e := Edge{
				Src:             fmt.Sprint(s),
				Dst:             fmt.Sprint(d),
				RelType:         fmt.Sprint(t),
				Weight:          toFloat64(w, 0),
				DimSemantic:     toFloat64(ds, 0),
				DimTemporal:     toFloat64(dt, 0),
				DimCoactivation: toFloat64(dc, 0),
				Direction:       fmt.Sprint(dir),
				UpdatedAt:       time.Now(),
			}
			edges = append(edges, e)
			if _, ok := seenNext[e.Dst]; !ok {
				seenNext[e.Dst] = struct{}{}
				next = append(next, e.Dst)
			}
			_ = upd
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return edgeTraversalPack{Edges: edges, Next: next}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	pack, ok := outAny.(edgeTraversalPack)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected result type: %T", outAny)
	}
	return pack.Edges, pack.Next, nil
}

// getEdgeTypesForHop returns the edge types to use based on hop depth and strategy (V0010).
// This implements the Hybrid Edge Type Strategy from GAT Phase 2.
func (s *Service) getEdgeTypesForHop(hopDepth int) (edgeTypes []string, applyAttention bool) {
	switch s.cfg.EdgeTypeStrategy {
	case "structural_first":
		// Structural edges for early hops, all edges for later hops
		if hopDepth < s.cfg.HybridSwitchHop {
			return s.cfg.StructuralEdgeTypes, false
		}
		return s.cfg.AllowedRelationshipTypes, s.cfg.QueryAwareExpansionEnabled

	case "learned_only":
		// Only learned edges (CO_ACTIVATED_WITH), always with attention
		return s.cfg.LearnedEdgeTypes, s.cfg.QueryAwareExpansionEnabled

	case "hybrid":
		// Structural edges for early hops, learned edges with attention for later hops
		if hopDepth < s.cfg.HybridSwitchHop {
			return s.cfg.StructuralEdgeTypes, false
		}
		return s.cfg.LearnedEdgeTypes, s.cfg.QueryAwareExpansionEnabled

	default: // "all"
		// All edge types with optional attention (original behavior)
		return s.cfg.AllowedRelationshipTypes, s.cfg.QueryAwareExpansionEnabled
	}
}

func (s *Service) fetchOutgoingEdges(ctx context.Context, spaceIDs []string, nodeIDs []string) ([]Edge, []string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Get decay parameters with defaults
	decayPerDay := s.cfg.LearningDecayPerDay
	if decayPerDay <= 0 {
		decayPerDay = 0.05 // 5% decay per day
	}
	pruneThreshold := s.cfg.LearningPruneThreshold
	if pruneThreshold <= 0 {
		pruneThreshold = 0.05 // prune edges below 0.05 weight
	}

	params := map[string]any{
		"spaceIds":       spaceIDs,
		"nodeIds":        nodeIDs,
		"allowed":        s.cfg.AllowedRelationshipTypes,
		"maxNbr":         s.cfg.MaxNeighborsPerNode,
		"maxTotal":       s.cfg.MaxTotalEdgesFetched,
		"decayPerDay":    decayPerDay,
		"pruneThreshold": pruneThreshold,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Query applies evidence-based decay to CO_ACTIVATED_WITH edges:
		// - Calculates days since last_activated_at
		// - Decay rate is reduced by sqrt(evidence_count) - frequently co-activated edges decay slower
		// - Formula: weight * (1 - decayPerDay/sqrt(evidence_count * surprise_factor))^days
		// - Filters out edges below pruneThreshold
		// This ensures edges that have been repeatedly strengthened persist while
		// spurious one-off connections decay quickly.
		cypher := `UNWIND $nodeIds AS sid
MATCH (src:MemoryNode)
WHERE src.space_id IN $spaceIds AND src.node_id = sid
CALL {
  WITH src
  MATCH (src)-[r]->(dst:MemoryNode)
  WHERE dst.space_id IN $spaceIds
    AND type(r) IN $allowed AND coalesce(r.status,'active')='active'
  WITH src, r, dst, type(r) AS relType,
       CASE WHEN type(r) = 'CO_ACTIVATED_WITH' THEN
         duration.between(coalesce(r.last_activated_at, r.created_at, datetime()), datetime()).days
       ELSE 0 END AS daysSinceActive,
       coalesce(r.weight, 0.0) AS rawWeight,
       coalesce(r.evidence_count, 1) AS evidenceCount,
       coalesce(r.surprise_factor, 1.0) AS surpriseFactor
  WITH src, r, dst, relType, daysSinceActive, rawWeight, evidenceCount, surpriseFactor,
       // Evidence-based decay: stronger edges (more evidence) decay slower
       // effectiveDecay = baseDecay / sqrt(evidenceCount * surpriseFactor)
       CASE WHEN relType = 'CO_ACTIVATED_WITH' AND daysSinceActive > 0 THEN
         rawWeight * (CASE WHEN (1.0 - $decayPerDay / sqrt(toFloat(evidenceCount) * surpriseFactor)) <= 0 THEN 0.01 ELSE (1.0 - $decayPerDay / sqrt(toFloat(evidenceCount) * surpriseFactor)) END ^ daysSinceActive)
       ELSE rawWeight END AS decayedWeight
  WHERE NOT (relType = 'CO_ACTIVATED_WITH' AND decayedWeight < $pruneThreshold)
  RETURN src.node_id AS s, dst.node_id AS d, relType AS t,
         decayedWeight AS w,
         coalesce(r.dim_semantic,0.0) AS ds,
         coalesce(r.dim_temporal,0.0) AS dt,
         coalesce(r.dim_coactivation,0.0) AS dc,
         coalesce(r.direction, 'bidirectional') AS dir,
         coalesce(r.updated_at, datetime()) AS upd
  ORDER BY w DESC
  LIMIT $maxNbr
}
RETURN s, d, t, w, ds, dt, dc, dir, upd
LIMIT $maxTotal`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]Edge, 0, 1024)
		next := make([]string, 0, 1024)
		seenNext := map[string]struct{}{}
		for res.Next(ctx) {
			rec := res.Record()
			s, _ := rec.Get("s")
			d, _ := rec.Get("d")
			t, _ := rec.Get("t")
			w, _ := rec.Get("w")
			ds, _ := rec.Get("ds")
			dt, _ := rec.Get("dt")
			dc, _ := rec.Get("dc")
			dir, _ := rec.Get("dir")
			upd, _ := rec.Get("upd")

			e := Edge{
				Src:             fmt.Sprint(s),
				Dst:             fmt.Sprint(d),
				RelType:         fmt.Sprint(t),
				Weight:          toFloat64(w, 0),
				DimSemantic:     toFloat64(ds, 0),
				DimTemporal:     toFloat64(dt, 0),
				DimCoactivation: toFloat64(dc, 0),
				Direction:       fmt.Sprint(dir),
				UpdatedAt:       time.Now(),
			}
			edges = append(edges, e)
			if _, ok := seenNext[e.Dst]; !ok {
				seenNext[e.Dst] = struct{}{}
				next = append(next, e.Dst)
			}
			_ = upd
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return edgeTraversalPack{Edges: edges, Next: next}, nil
	})
	if err != nil {
		return nil, nil, err
	}
	pack, ok := outAny.(edgeTraversalPack)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected result type: %T", outAny)
	}
	return pack.Edges, pack.Next, nil
}

// IngestObservation is intentionally minimal: append-only Observation + basic node upsert.
func (s *Service) IngestObservation(ctx context.Context, req models.IngestRequest) (models.IngestResponse, error) {
	if req.SpaceID == "" {
		return models.IngestResponse{}, errors.New("space_id is required")
	}
	if req.Source == "" {
		return models.IngestResponse{}, errors.New("source is required")
	}
	if req.Timestamp == "" {
		return models.IngestResponse{}, errors.New("timestamp is required")
	}

	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = newID("n")
	}
	obsID := newID("o")

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Auto-extract canonical_time if not explicitly provided (Phase 2 Temporal)
	canonicalTimeStr := req.CanonicalTime
	if canonicalTimeStr == "" {
		if contentStr, ok := req.Content.(string); ok && contentStr != "" {
			if extracted := ExtractCanonicalTime(contentStr, req.Tags); !extracted.IsZero() {
				canonicalTimeStr = extracted.Format(time.RFC3339)
			}
		}
	}

	params := map[string]any{
		"spaceId":       req.SpaceID,
		"nodeId":        nodeID,
		"path":          req.Path,
		"name":          req.Name,
		"summary":       req.Summary,
		"obsId":         obsID,
		"timestamp":     req.Timestamp,
		"source":        req.Source,
		"content":       req.Content,
		"tags":          req.Tags,
		"sensitivity":   req.Sensitivity,
		"confidence":    req.Confidence,
		"embedding":     req.Embedding, // May be nil/empty
		"canonicalTime": canonicalTimeStr,
		"prunable":      IsPrunableSpace(req.SpaceID),
		"contentHash":   req.ContentHash,
		"fileSize":      req.FileSize,
		"lineCount":     req.LineCount,
	}

	// Determine merge key: prefer path if provided, else use node_id
	mergeKey := "node_id"
	mergeValue := nodeID
	if req.Path != "" {
		mergeKey = "path"
		mergeValue = req.Path
	}
	params["mergeValue"] = mergeValue

	// Content-hash skip: if hash provided and matches existing node, skip observation creation
	// Only update last_ingested_at + metadata, avoiding observation bloat for unchanged files
	if req.ContentHash != "" && mergeKey == "path" {
		skipResult, skipErr := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			checkCypher := `
MATCH (n:MemoryNode {space_id:$spaceId, path:$mergeValue})
WHERE n.content_hash = $contentHash
SET n.last_ingested_at = datetime(),
    n.file_size = CASE WHEN $fileSize > 0 THEN $fileSize ELSE n.file_size END,
    n.line_count = CASE WHEN $lineCount > 0 THEN $lineCount ELSE n.line_count END
RETURN n.node_id AS node_id`
			res, err := tx.Run(ctx, checkCypher, params)
			if err != nil {
				return nil, err
			}
			if res.Next(ctx) {
				nid, _ := res.Record().Get("node_id")
				return nid, nil
			}
			return nil, res.Err()
		})
		if skipErr == nil && skipResult != nil {
			existingNodeID, _ := skipResult.(string)
			if existingNodeID != "" {
				slog.Debug("ingest: content unchanged, skipping observation", "path", req.Path, "content_hash", req.ContentHash)
				return models.IngestResponse{
					SpaceID: req.SpaceID,
					NodeID:  existingNodeID,
					Skipped: true,
				}, nil
			}
		}
		// If skip check failed or no match, fall through to full ingest
	}

	// Build embedding SET clause (only if embedding provided)
	// Also flag edges as stale when embedding changes (Phase 9.5.3)
	embeddingClause := ""
	if len(req.Embedding) > 0 {
		embeddingClause = ", n.embedding = $embedding, n.edges_stale = true"
	}

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Build cypher dynamically based on merge key
		var cypher string
		if mergeKey == "path" {
			cypher = `
MERGE (t:TapRoot {space_id:$spaceId})
ON CREATE SET t.name='tap_root', t.created_at=datetime(), t.prunable=$prunable
WITH t
MERGE (n:MemoryNode {space_id:$spaceId, path:$mergeValue})
ON CREATE SET n.node_id=$nodeId,
              n.name=coalesce($name, $mergeValue),
              n.layer=0,
              n.role_type='leaf',
              n.version=1,
              n.status='active',
              n.created_at=datetime(),
              n.updated_at=datetime(),
              n.canonical_time = CASE WHEN $canonicalTime <> '' THEN datetime($canonicalTime) ELSE datetime() END,
              n.update_count=0,
              n.summary=coalesce($summary,''),
              n.description='',
              n.confidence=coalesce($confidence, 0.6),
              n.sensitivity=coalesce($sensitivity,'internal'),
              n.tags=coalesce($tags,[])
WITH n
CREATE (o:Observation {
  space_id:$spaceId,
  obs_id:$obsId,
  timestamp: datetime($timestamp),
  source:$source,
  content:$content,
  created_at: datetime()
})
MERGE (n)-[:HAS_OBSERVATION {space_id:$spaceId, created_at:datetime()}]->(o)
SET n.updated_at=datetime(),
    n.canonical_time = CASE WHEN $canonicalTime <> '' THEN datetime($canonicalTime) ELSE n.canonical_time END,
    n.update_count = coalesce(n.update_count,0) + 1,
    n.version = coalesce(n.version, 0) + 1,
    n.last_ingested_at = datetime(),
    n.summary = CASE WHEN $summary IS NOT NULL AND $summary <> '' THEN $summary ELSE n.summary END,
    n.content_hash = CASE WHEN $contentHash IS NOT NULL AND $contentHash <> '' THEN $contentHash ELSE n.content_hash END,
    n.file_size = CASE WHEN $fileSize > 0 THEN $fileSize ELSE n.file_size END,
    n.line_count = CASE WHEN $lineCount > 0 THEN $lineCount ELSE n.line_count END` + embeddingClause + `
RETURN n.node_id AS node_id, n.version AS version, n.update_count AS update_count`
		} else {
			cypher = `
MERGE (t:TapRoot {space_id:$spaceId})
ON CREATE SET t.name='tap_root', t.created_at=datetime(), t.prunable=$prunable
WITH t
MERGE (n:MemoryNode {space_id:$spaceId, node_id:$mergeValue})
ON CREATE SET n.path=coalesce($path, $mergeValue),
              n.name=coalesce($name, $mergeValue),
              n.layer=0,
              n.role_type='leaf',
              n.version=1,
              n.status='active',
              n.created_at=datetime(),
              n.updated_at=datetime(),
              n.canonical_time = CASE WHEN $canonicalTime <> '' THEN datetime($canonicalTime) ELSE datetime() END,
              n.update_count=0,
              n.summary=coalesce($summary,''),
              n.description='',
              n.confidence=coalesce($confidence, 0.6),
              n.sensitivity=coalesce($sensitivity,'internal'),
              n.tags=coalesce($tags,[])
With n
CREATE (o:Observation {
  space_id:$spaceId,
  obs_id:$obsId,
  timestamp: datetime($timestamp),
  source:$source,
  content:$content,
  created_at: datetime()
})
MERGE (n)-[:HAS_OBSERVATION {space_id:$spaceId, created_at:datetime()}]->(o)
SET n.updated_at=datetime(),
    n.canonical_time = CASE WHEN $canonicalTime <> '' THEN datetime($canonicalTime) ELSE n.canonical_time END,
    n.update_count = coalesce(n.update_count,0) + 1,
    n.version = coalesce(n.version, 0) + 1,
    n.last_ingested_at = datetime(),
    n.summary = CASE WHEN $summary IS NOT NULL AND $summary <> '' THEN $summary ELSE n.summary END,
    n.content_hash = CASE WHEN $contentHash IS NOT NULL AND $contentHash <> '' THEN $contentHash ELSE n.content_hash END,
    n.file_size = CASE WHEN $fileSize > 0 THEN $fileSize ELSE n.file_size END,
    n.line_count = CASE WHEN $lineCount > 0 THEN $lineCount ELSE n.line_count END` + embeddingClause + `
RETURN n.node_id AS node_id, n.version AS version, n.update_count AS update_count`
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			version, _ := rec.Get("version")
			updateCount, _ := rec.Get("update_count")
			vc := toInt(version, 1)
			uc := toInt(updateCount, 0)
			if uc > 1 {
				slog.Info("ingest: node updated", "node_id", nodeID, "version", vc, "update_count", uc)
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		return models.IngestResponse{}, err
	}

	// Semantic edge creation: link to similar existing nodes
	if s.cfg.SemanticEdgeOnIngest && len(req.Embedding) > 0 {
		similarNodes, findErr := s.FindSimilarNodes(ctx, req.SpaceID, req.Embedding, nodeID, s.cfg.SemanticEdgeTopN)
		if findErr != nil {
			// Log warning but don't fail the ingest
			slog.Warn("FindSimilarNodes failed", "node_id", nodeID, "error", findErr)
		} else {
			for _, sn := range similarNodes {
				if sn.Score >= s.cfg.SemanticEdgeMinSimilarity {
					edgeErr := s.CreateAssociatedWithEdge(ctx, req.SpaceID, nodeID, sn.NodeID, sn.Score)
					if edgeErr != nil {
						// Log warning but don't fail the ingest
						slog.Warn("CreateAssociatedWithEdge failed", "from", nodeID, "to", sn.NodeID, "error", edgeErr)
					}
				}
			}
		}

		// Phase 47: Propagate edge staleness when embedding changes
		// This marks connected CO_ACTIVATED_WITH and ASSOCIATED_WITH edges as stale
		s.PropagateEdgeStalenessAfterIngest(ctx, req.SpaceID, nodeID)
	}

	return models.IngestResponse{SpaceID: req.SpaceID, NodeID: nodeID, ObsID: obsID}, nil
}

// BatchIngestObservations processes multiple observations in a single batch.
// Returns results for each item, supporting partial success (some items may fail while others succeed).
func (s *Service) BatchIngestObservations(ctx context.Context, req models.BatchIngestRequest) (models.BatchIngestResponse, error) {
	if req.SpaceID == "" {
		return models.BatchIngestResponse{}, errors.New("space_id is required")
	}
	if len(req.Observations) == 0 {
		return models.BatchIngestResponse{}, errors.New("observations array is required and must not be empty")
	}

	results := make([]models.BatchIngestResult, 0, len(req.Observations))
	successCount := 0
	errorCount := 0

	for i, obs := range req.Observations {
		// Convert BatchIngestItem to IngestRequest
		ingestReq := models.IngestRequest{
			SpaceID:       req.SpaceID,
			Timestamp:     obs.Timestamp,
			Source:        obs.Source,
			Content:       obs.Content,
			Tags:          obs.Tags,
			NodeID:        obs.NodeID,
			Path:          obs.Path,
			Name:          obs.Name,
			Summary:       obs.Summary,
			Sensitivity:   obs.Sensitivity,
			Confidence:    obs.Confidence,
			Embedding:     obs.Embedding,
			CanonicalTime: obs.CanonicalTime,
		}

		// Validate required fields
		if ingestReq.Source == "" {
			results = append(results, models.BatchIngestResult{
				Index:  i,
				Status: "error",
				Error:  "source is required",
			})
			errorCount++
			continue
		}
		if ingestReq.Timestamp == "" {
			results = append(results, models.BatchIngestResult{
				Index:  i,
				Status: "error",
				Error:  "timestamp is required",
			})
			errorCount++
			continue
		}

		// Use existing IngestObservation logic
		resp, err := s.IngestObservation(ctx, ingestReq)
		if err != nil {
			results = append(results, models.BatchIngestResult{
				Index:  i,
				Status: "error",
				Error:  err.Error(),
			})
			errorCount++
			continue
		}

		result := models.BatchIngestResult{
			Index:  i,
			Status: "success",
			NodeID: resp.NodeID,
			ObsID:  resp.ObsID,
		}
		if resp.EmbeddingDims > 0 {
			result.EmbeddingDims = resp.EmbeddingDims
		}
		results = append(results, result)
		successCount++
	}

	return models.BatchIngestResponse{
		SpaceID:      req.SpaceID,
		TotalItems:   len(req.Observations),
		SuccessCount: successCount,
		ErrorCount:   errorCount,
		Results:      results,
	}, nil
}

// RefreshStaleEdges finds nodes with edges_stale=true and refreshes their
// ASSOCIATED_WITH edges based on current embeddings. Processes up to 50
// nodes per call. Returns the number of nodes refreshed.
func (s *Service) RefreshStaleEdges(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Step 1: Find stale nodes (limit 50 per call)
	findCypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.edges_stale = true
  AND n.embedding IS NOT NULL
  AND NOT coalesce(n.is_archived, false)
RETURN n.node_id AS node_id
LIMIT 50`

	result, err := sess.Run(ctx, findCypher, map[string]any{"spaceId": spaceID})
	if err != nil {
		return 0, fmt.Errorf("find stale nodes: %w", err)
	}

	var staleNodeIDs []string
	for result.Next(ctx) {
		rec := result.Record()
		nid, _ := rec.Get("node_id")
		staleNodeIDs = append(staleNodeIDs, fmt.Sprint(nid))
	}
	if err := result.Err(); err != nil {
		return 0, fmt.Errorf("read stale nodes: %w", err)
	}
	sess.Close(ctx)

	if len(staleNodeIDs) == 0 {
		return 0, nil
	}

	// Step 2: For each stale node, get its embedding and refresh edges
	refreshed := 0
	for _, nodeID := range staleNodeIDs {
		// Get the node's current embedding
		embSess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		embResult, embErr := embSess.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
RETURN n.embedding AS embedding`, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if embErr != nil {
			embSess.Close(ctx)
			slog.Warn("failed to get embedding for stale node", "node_id", nodeID, "error", embErr)
			continue
		}

		var embedding []float32
		if embResult.Next(ctx) {
			rec := embResult.Record()
			embVal, _ := rec.Get("embedding")
			if embSlice, ok := embVal.([]any); ok {
				embedding = make([]float32, len(embSlice))
				for i, v := range embSlice {
					switch fv := v.(type) {
					case float64:
						embedding[i] = float32(fv)
					case float32:
						embedding[i] = fv
					}
				}
			}
		}
		embSess.Close(ctx)

		if len(embedding) == 0 {
			continue
		}

		// Find similar nodes and update edges
		similarNodes, findErr := s.FindSimilarNodes(ctx, spaceID, embedding, nodeID, s.cfg.SemanticEdgeTopN)
		if findErr != nil {
			slog.Warn("FindSimilarNodes failed for stale node", "node_id", nodeID, "error", findErr)
			continue
		}

		for _, sn := range similarNodes {
			if sn.Score >= s.cfg.SemanticEdgeMinSimilarity {
				edgeErr := s.CreateAssociatedWithEdge(ctx, spaceID, nodeID, sn.NodeID, sn.Score)
				if edgeErr != nil {
					slog.Warn("CreateAssociatedWithEdge failed", "from", nodeID, "to", sn.NodeID, "error", edgeErr)
				}
			}
		}

		// Clear the stale flag and propagate to parent hidden nodes
		clearSess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, clearErr := clearSess.Run(ctx, `
MATCH (n:MemoryNode {node_id: $nodeId, space_id: $spaceId})
REMOVE n.edges_stale`, map[string]any{
			"nodeId":  nodeID,
			"spaceId": spaceID,
		})
		if clearErr != nil {
			slog.Warn("failed to clear edges_stale for node", "node_id", nodeID, "error", clearErr)
		}

		// Propagate staleness to parent hidden nodes
		_, propErr := clearSess.Run(ctx, `
MATCH (h:MemoryNode)-[:GENERALIZES]->(n:MemoryNode {node_id: $nodeId, space_id: $spaceId})
WHERE h.layer > 0
SET h.edges_stale = true`, map[string]any{
			"nodeId":  nodeID,
			"spaceId": spaceID,
		})
		if propErr != nil {
			slog.Warn("failed to propagate edges_stale to parents", "node_id", nodeID, "error", propErr)
		}
		clearSess.Close(ctx)

		refreshed++
	}

	return refreshed, nil
}

func newID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func toFloat64(v any, def float64) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	default:
		return def
	}
}

func toInt(v any, def int) int {
	switch x := v.(type) {
	case int64:
		return int(x)
	case int:
		return x
	case float64:
		return int(x)
	default:
		return def
	}
}

func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		result := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// toUint16Slice converts a Neo4j integer-array property to []uint16. Used
// for context_fingerprint_active. Neo4j stores integers as int64; we
// safely narrow each element. Phase 14.2 Epic 4.
func toUint16Slice(v any) []uint16 {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []uint16:
		return x
	case []any:
		result := make([]uint16, 0, len(x))
		for _, item := range x {
			switch n := item.(type) {
			case int64:
				if n >= 0 && n <= 0xFFFF {
					result = append(result, uint16(n))
				}
			case int:
				if n >= 0 && n <= 0xFFFF {
					result = append(result, uint16(n))
				}
			}
		}
		if len(result) == 0 {
			return nil
		}
		return result
	default:
		return nil
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// =============================================================================
// PHASE 5: CONTEXT-AWARE RETRIEVAL
// =============================================================================
// Blends conversation knowledge into retrieval results by:
// 1. Finding relevant conversation observations/themes/concepts
// 2. Using spreading activation through the concept hierarchy
// 3. Boosting results that have supporting conversation knowledge

// ConversationContextResult represents conversation knowledge that supports retrieval
type ConversationContextResult struct {
	NodeID  string
	Type    string // conversation_observation, conversation_theme, emergent_concept
	Content string
	Score   float64
	Layer   int
}

// RetrieveWithConversationContext extends Retrieve with conversation knowledge blending.
// When enabled, it finds relevant conversation knowledge and boosts results that
// are semantically connected to prior learnings.
func (s *Service) RetrieveWithConversationContext(ctx context.Context, req models.RetrieveRequest, boostFactor float64) (models.RetrieveResponse, error) {
	// First, get standard retrieval results
	resp, err := s.Retrieve(ctx, req)
	if err != nil {
		return resp, err
	}

	if len(resp.Results) == 0 {
		return resp, nil
	}

	// Default boost factor
	if boostFactor <= 0 {
		boostFactor = 1.2
	}

	// Find relevant conversation knowledge
	conversationContext, err := s.findRelevantConversationContext(ctx, req.SpaceID, req.QueryEmbedding, 10)
	if err != nil {
		// Log warning but continue with unmodified results
		slog.Warn("failed to find conversation context, returning unmodified results", "error", err)
		return resp, nil
	}

	if len(conversationContext) == 0 {
		return resp, nil
	}

	// Apply conversation-based boosting via spreading activation
	resp = s.applyConversationBoost(ctx, req.SpaceID, resp, conversationContext, boostFactor)

	// Add debug info
	if resp.Debug == nil {
		resp.Debug = make(map[string]any)
	}
	resp.Debug["conversation_context_count"] = len(conversationContext)
	resp.Debug["conversation_boost_factor"] = boostFactor

	return resp, nil
}

// findRelevantConversationContext finds conversation knowledge relevant to the query
func (s *Service) findRelevantConversationContext(ctx context.Context, spaceID string, embedding []float32, topK int) ([]ConversationContextResult, error) {
	if len(embedding) == 0 {
		return nil, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Query conversation nodes (observations, themes, concepts) by vector similarity
	cypher := `
CALL db.index.vector.queryNodes($index, $k, $q)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.role_type IN ['conversation_observation', 'conversation_theme', 'emergent_concept']
  AND NOT coalesce(node.is_archived, false)
RETURN node.node_id AS nodeId, node.role_type AS roleType,
       coalesce(node.summary, node.content, node.name, '') AS content,
       node.layer AS layer, score
ORDER BY score DESC
LIMIT $k`

	params := map[string]any{
		"spaceId": spaceID,
		"k":       topK * 2, // Fetch more, filter to topK
		"q":       embedding,
		"index":   s.cfg.VectorIndexName,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var results []ConversationContextResult
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			roleType, _ := rec.Get("roleType")
			content, _ := rec.Get("content")
			layer, _ := rec.Get("layer")
			sc, _ := rec.Get("score")

			results = append(results, ConversationContextResult{
				NodeID:  fmt.Sprint(nodeID),
				Type:    fmt.Sprint(roleType),
				Content: fmt.Sprint(content),
				Score:   toFloat64(sc, 0),
				Layer:   toInt(layer, 0),
			})

			if len(results) >= topK {
				break
			}
		}
		return results, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.([]ConversationContextResult)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// applyConversationBoost boosts retrieval results that are connected to conversation knowledge
// Uses spreading activation through GENERALIZES and ABSTRACTS_TO edges
func (s *Service) applyConversationBoost(ctx context.Context, spaceID string, resp models.RetrieveResponse, conversationContext []ConversationContextResult, boostFactor float64) models.RetrieveResponse {
	if len(conversationContext) == 0 || len(resp.Results) == 0 {
		return resp
	}

	// Build a map of conversation context scores by node ID
	contextScores := make(map[string]float64)
	for _, cc := range conversationContext {
		contextScores[cc.NodeID] = cc.Score
	}

	// Find nodes that are connected to conversation context via edges
	connectedNodes, err := s.findConversationConnectedNodes(ctx, spaceID, conversationContext)
	if err != nil {
		slog.Warn("failed to find conversation-connected nodes", "error", err)
		return resp
	}

	// Apply boost to results that are connected to conversation context
	for i := range resp.Results {
		nodeID := resp.Results[i].NodeID

		// Check if directly in context
		if score, ok := contextScores[nodeID]; ok {
			// Boost based on conversation context score
			boost := 1.0 + (boostFactor-1.0)*score
			resp.Results[i].Score *= boost
			continue
		}

		// Check if connected via edges
		if connectionStrength, ok := connectedNodes[nodeID]; ok {
			// Boost based on connection strength (decayed by hops)
			boost := 1.0 + (boostFactor-1.0)*connectionStrength*0.5
			resp.Results[i].Score *= boost
		}
	}

	// Re-sort by score after boosting
	sortResultsByScore(resp.Results)

	return resp
}

// findConversationConnectedNodes finds code nodes connected to conversation context
// Returns a map of node_id -> connection_strength (0.0-1.0)
func (s *Service) findConversationConnectedNodes(ctx context.Context, spaceID string, conversationContext []ConversationContextResult) (map[string]float64, error) {
	if len(conversationContext) == 0 {
		return nil, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Extract conversation node IDs
	convNodeIDs := make([]string, len(conversationContext))
	convScores := make(map[string]float64)
	for i, cc := range conversationContext {
		convNodeIDs[i] = cc.NodeID
		convScores[cc.NodeID] = cc.Score
	}

	// Find code nodes connected to conversation nodes through CO_ACTIVATED_WITH edges
	// This leverages the Hebbian learning edges that connect observations to code
	cypher := `
UNWIND $convNodeIds AS convId
MATCH (conv:MemoryNode {space_id: $spaceId, node_id: convId})
      -[r:CO_ACTIVATED_WITH]-
      (code:MemoryNode {space_id: $spaceId})
WHERE code.role_type IS NULL OR code.role_type NOT IN ['conversation_observation', 'conversation_theme', 'emergent_concept']
WITH code.node_id AS codeNodeId, convId, r.weight AS weight
RETURN codeNodeId, convId, weight
ORDER BY weight DESC
LIMIT 100`

	params := map[string]any{
		"spaceId":     spaceID,
		"convNodeIds": convNodeIDs,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		connections := make(map[string]float64)
		for res.Next(ctx) {
			rec := res.Record()
			codeNodeID, _ := rec.Get("codeNodeId")
			convID, _ := rec.Get("convId")
			weight, _ := rec.Get("weight")

			codeID := fmt.Sprint(codeNodeID)
			convIDStr := fmt.Sprint(convID)
			w := toFloat64(weight, 0)

			// Combine edge weight with conversation context score
			convScore := convScores[convIDStr]
			connectionStrength := w * convScore

			// Keep the strongest connection
			if existing, ok := connections[codeID]; !ok || connectionStrength > existing {
				connections[codeID] = connectionStrength
			}
		}
		return connections, res.Err()
	})

	if err != nil {
		return nil, err
	}
	typed, ok := result.(map[string]float64)
	if !ok {
		return nil, fmt.Errorf("unexpected result type: %T", result)
	}
	return typed, nil
}

// sortResultsByScore sorts retrieval results by score in descending order
func sortResultsByScore(results []models.RetrieveResult) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
