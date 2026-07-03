package hidden

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	cuid2 "github.com/nrednav/cuid2"
	"mdemg/internal/circuitbreaker"
	"mdemg/internal/config"
	"mdemg/internal/metrics"
)

// EdgePruner is an optional dependency that prunes excess CO_ACTIVATED_WITH edges per node.
// It is wired in from the learning service when LearningAutoPruneExcessEnabled is true.
type EdgePruner interface {
	PruneExcessEdgesPerNode(ctx context.Context, spaceID string) (int64, error)
}

// Service handles hidden layer operations including clustering and message passing
type Service struct {
	cfg        config.Config
	driver     neo4j.DriverWithContext
	pipeline   *Pipeline
	cbRegistry *circuitbreaker.Registry
	edgePruner EdgePruner // optional; nil if auto-prune is disabled

	// JIMINY-CORPUS-001: gates ConvObs→role_type='constraint' promotion in
	// CreateConstraintNodes. nil (e.g. hand-built Service in tests) = pass.
	constraintGate *ConstraintPromotionGate

	// Per-space consolidation lock: prevents concurrent consolidation on the same space
	consolidateLocks sync.Map // map[string]*sync.Mutex
}

// NewService creates a new hidden layer service.
// cbRegistry may be nil (e.g. CLI consolidation); LLM steps that need it will skip gracefully.
func NewService(cfg config.Config, driver neo4j.DriverWithContext, cbRegistry *circuitbreaker.Registry) *Service {
	s := &Service{cfg: cfg, driver: driver, cbRegistry: cbRegistry}
	s.constraintGate = NewConstraintPromotionGate(cfg)
	s.pipeline = s.buildPipeline()
	return s
}

// SetCircuitBreakerRegistry sets the circuit breaker registry after construction.
// This is needed when the registry is initialized after the hidden service.
func (s *Service) SetCircuitBreakerRegistry(reg *circuitbreaker.Registry) {
	s.cbRegistry = reg
}

// SetEdgePruner wires in an EdgePruner to be called at the end of RunConsolidation
// when LearningAutoPruneExcessEnabled is true.
func (s *Service) SetEdgePruner(p EdgePruner) {
	s.edgePruner = p
}

// newEmergenceNamer constructs an EmergenceNamer from the service config.
// Returns nil when emergence is disabled or cbRegistry is nil — callers handle nil gracefully.
func (s *Service) newEmergenceNamer() *EmergenceNamer {
	if !s.cfg.EmergenceEnabled {
		return nil
	}
	return NewEmergenceNamer(EmergenceNamerConfig{
		Enabled:   s.cfg.EmergenceEnabled,
		Provider:  s.cfg.EmergenceProvider,
		Model:     s.cfg.EmergenceModel,
		MaxTokens: s.cfg.EmergenceMaxTokens,
		TimeoutMs: s.cfg.EmergenceTimeoutMs,
		OpenAIKey: s.cfg.OpenAIAPIKey,
		OpenAIURL: s.cfg.EffectiveLLMEndpoint(),
		OllamaURL: s.cfg.OllamaEndpoint,
	}, s.cbRegistry)
}

// newReclassifier constructs a Reclassifier from the service config.
// Returns nil when reclassification is disabled — callers handle nil gracefully.
func (s *Service) newReclassifier() *Reclassifier {
	if !s.cfg.ReclassEnabled {
		return nil
	}
	return NewReclassifier(ReclassifierConfig{
		Enabled:       s.cfg.ReclassEnabled,
		Threshold:     s.cfg.ReclassThreshold,
		MaxSampleSize: s.cfg.ReclassMaxSampleSize,
		MaxCategories: s.cfg.ReclassMaxCategories,
		MaxIterations: s.cfg.ReclassMaxIterations,
		MaxDepth:      s.cfg.ReclassMaxDepth,
		Provider:      s.cfg.ReclassProvider,
		Model:         s.cfg.ReclassModel,
		MaxTokens:     s.cfg.ReclassMaxTokens,
		TimeoutMs:     s.cfg.ReclassTimeoutMs,
		OpenAIKey:     s.cfg.OpenAIAPIKey,
		OpenAIURL:     s.cfg.EffectiveLLMEndpoint(),
		OllamaURL:     s.cfg.OllamaEndpoint,
	}, s.cbRegistry)
}

// newClusterSummarizer constructs a ClusterSummarizer from the service config.
// Returns nil when cluster summarization is disabled — callers handle nil gracefully.
func (s *Service) newClusterSummarizer() *ClusterSummarizer {
	if !s.cfg.ClusterSummaryEnabled {
		return nil
	}
	return NewClusterSummarizer(ClusterSummarizerConfig{
		Enabled:   s.cfg.ClusterSummaryEnabled,
		Provider:  s.cfg.ClusterSummaryProvider,
		Model:     s.cfg.ClusterSummaryModel,
		MaxTokens: s.cfg.ClusterSummaryMaxTokens,
		TimeoutMs: s.cfg.ClusterSummaryTimeoutMs,
		OpenAIKey: s.cfg.OpenAIAPIKey,
		OpenAIURL: s.cfg.EffectiveLLMEndpoint(),
		OllamaURL: s.cfg.OllamaEndpoint,
	}, s.cbRegistry)
}

// baseNodesToClusterSummaries converts BaseNode slice to ClusterNodeSummary for LLM naming.
// BaseNode.Path holds the node name (per fetchOrphanLayerNodesWithName).
func baseNodesToClusterSummaries(members []BaseNode) []ClusterNodeSummary {
	summaries := make([]ClusterNodeSummary, len(members))
	for i, m := range members {
		summaries[i] = ClusterNodeSummary{
			NodeID: m.NodeID,
			Name:   m.Path, // Path field stores name for layer 1+ nodes
			Path:   m.Path,
		}
	}
	return summaries
}

// emergentConceptNodesToClusterSummaries converts EmergentConceptNode slice to ClusterNodeSummary.
func emergentConceptNodesToClusterSummaries(members []EmergentConceptNode) []ClusterNodeSummary {
	summaries := make([]ClusterNodeSummary, len(members))
	for i, m := range members {
		summaries[i] = ClusterNodeSummary{
			NodeID:  m.NodeID,
			Name:    m.Name,
			Summary: m.Summary,
			Layer:   m.Layer,
		}
	}
	return summaries
}

// mechanicalSummaryPrefixes identifies summaries that were generated mechanically
// and would benefit from LLM enhancement.
var mechanicalSummaryPrefixes = []string{
	"Pattern of ",
	"Concept-L",
	"Hidden concept: ",
	"Concern: ",
	"Cluster of ",
}

// isMechanicalSummary checks if a summary was generated mechanically.
func isMechanicalSummary(summary string) bool {
	for _, prefix := range mechanicalSummaryPrefixes {
		if strings.HasPrefix(summary, prefix) {
			return true
		}
	}
	return false
}

// EnhanceSummariesWithLLM replaces mechanical summaries on L1-L4 nodes with
// LLM-generated semantic summaries. Skips L5 nodes (already have EmergenceNamer summaries).
// Returns the number of summaries enhanced.
func (s *Service) EnhanceSummariesWithLLM(ctx context.Context, spaceID string) (int, error) {
	summarizer := s.newClusterSummarizer()
	if summarizer == nil {
		return 0, nil // Disabled or no circuit breaker
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Query L1-L4 nodes with mechanical summaries
	batchSize := s.cfg.ClusterSummaryBatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 1 AND n.layer <= 4
  AND n.summary IS NOT NULL
  AND NOT coalesce(n.is_archived, false)
RETURN n.node_id AS node_id, n.name AS name, n.summary AS summary, n.layer AS layer
LIMIT $limit`

	result, err := sess.Run(ctx, cypher, map[string]any{
		"spaceId": spaceID,
		"limit":   batchSize * 2, // Fetch more since we'll filter
	})
	if err != nil {
		return 0, fmt.Errorf("query cluster nodes: %w", err)
	}

	type clusterNode struct {
		NodeID  string
		Name    string
		Summary string
		Layer   int
	}

	var candidates []clusterNode
	for result.Next(ctx) {
		rec := result.Record()
		nid, _ := rec.Get("node_id")
		name, _ := rec.Get("name")
		summary, _ := rec.Get("summary")
		layer, _ := rec.Get("layer")

		sumStr := fmt.Sprint(summary)
		if !isMechanicalSummary(sumStr) {
			continue
		}

		candidates = append(candidates, clusterNode{
			NodeID:  fmt.Sprint(nid),
			Name:    fmt.Sprint(name),
			Summary: sumStr,
			Layer:   int(layer.(int64)),
		})

		if len(candidates) >= batchSize {
			break
		}
	}

	if err := result.Err(); err != nil {
		return 0, fmt.Errorf("iterate cluster nodes: %w", err)
	}

	if len(candidates) == 0 {
		return 0, nil
	}

	// For each candidate, fetch member names/summaries and generate LLM summary
	enhanced := 0
	for _, c := range candidates {
		// Fetch members of this cluster node
		memberCypher := `
MATCH (n:MemoryNode {node_id: $nodeId, space_id: $spaceId})<-[:GENERALIZES|ABSTRACTS_TO]-(m:MemoryNode)
RETURN m.name AS name, coalesce(m.summary, '') AS summary
LIMIT 20`

		memberResult, mErr := sess.Run(ctx, memberCypher, map[string]any{
			"nodeId":  c.NodeID,
			"spaceId": spaceID,
		})
		if mErr != nil {
			continue
		}

		var memberNames, memberSummaries []string
		for memberResult.Next(ctx) {
			rec := memberResult.Record()
			mn, _ := rec.Get("name")
			ms, _ := rec.Get("summary")
			memberNames = append(memberNames, fmt.Sprint(mn))
			memberSummaries = append(memberSummaries, fmt.Sprint(ms))
		}

		if len(memberNames) == 0 {
			// Fallback: use the node's own name
			memberNames = []string{c.Name}
			memberSummaries = []string{c.Summary}
		}

		// Call LLM for summary
		newSummary, sErr := summarizer.Summarize(ctx, memberNames, memberSummaries, c.Layer)
		if sErr != nil {
			slog.Warn("cluster-summary: LLM failed", "node_id", c.NodeID, "error", sErr)
			continue // Fail-open: keep mechanical summary
		}

		// Update in Neo4j
		updateCypher := `
MATCH (n:MemoryNode {node_id: $nodeId, space_id: $spaceId})
SET n.summary = $summary, n.updated_at = datetime()`

		_, uErr := sess.Run(ctx, updateCypher, map[string]any{
			"nodeId":  c.NodeID,
			"spaceId": spaceID,
			"summary": newSummary,
		})
		if uErr != nil {
			continue
		}

		enhanced++
	}

	return enhanced, nil
}

// buildPipeline registers all node-creation steps in phase order.
func (s *Service) buildPipeline() *Pipeline {
	p := NewPipeline()
	p.Register(&hiddenStep{svc: s})           // phase 10 — required
	p.Register(&concernStep{svc: s})          // phase 20
	p.Register(&configStep{svc: s})           // phase 20
	p.Register(&comparisonStep{svc: s})       // phase 20
	p.Register(&temporalStep{svc: s})         // phase 20
	p.Register(&uiStep{svc: s})               // phase 20
	p.Register(&constraintStep{svc: s})       // phase 20
	p.Register(&dynamicEmergenceStep{svc: s}) // phase 22 — LLM-driven concept naming (Phase 103)
	p.Register(&dynamicEdgesStep{svc: s})     // phase 25 — dynamic edges (after clustering)
	p.Register(&clusterSummaryStep{svc: s})   // phase 28 — LLM summary enhancement (L1-L4)
	p.Register(&emergentL5Step{svc: s})       // phase 30 — post-processing
	return p
}

// RunNodeCreationPipeline runs the node-creation steps of consolidation (phase 10-22).
// When enableEmergence is false, the dynamic_emergence step (phase 22) is skipped.
func (s *Service) RunNodeCreationPipeline(ctx context.Context, spaceID string, enableEmergence bool) (*PipelineResult, error) {
	skip := map[string]bool{}
	if !enableEmergence {
		skip["dynamic_emergence"] = true
	}
	return s.pipeline.RunPhaseRange(ctx, spaceID, skip, 10, 22)
}

// RunPostClusteringPipeline runs the post-clustering steps (phase 25-30: dynamic edges, L5).
// Call this AFTER forward/backward pass and concept clustering have completed.
func (s *Service) RunPostClusteringPipeline(ctx context.Context, spaceID string) (*PipelineResult, error) {
	return s.pipeline.RunPhaseRange(ctx, spaceID, nil, 25, 30)
}

// CreateHiddenNodes performs hybrid DBSCAN clustering on orphan base nodes
// Strategy (hybrid - allows emergent patterns):
//  1. Run DBSCAN on ALL nodes first to find natural embedding-based clusters
//  2. Split oversized clusters using path-based grouping (secondary organization)
//  3. Name clusters based on most common path patterns (descriptive, not prescriptive)
//  4. Small clusters stay intact - these are emergent cross-cutting patterns
func (s *Service) CreateHiddenNodes(ctx context.Context, spaceID string) (int, error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, nil
	}

	// Step 1: Fetch ALL base nodes (not just orphans — we re-cluster the full set)
	baseNodes, err := s.fetchAllBaseNodes(ctx, spaceID)
	if err != nil {
		return 0, fmt.Errorf("fetch base nodes: %w", err)
	}

	if len(baseNodes) < s.cfg.HiddenLayerMinSamples {
		return 0, nil // Not enough data to cluster
	}

	slog.Info("CreateHiddenNodes: L0 base nodes fetched for re-clustering", "count", len(baseNodes))

	// Step 1b: HIDDEN-CHURN-002 — the global detach-everything-then-recreate is
	// GONE. List existing L1 patterns so each cluster can match an existing node
	// by centroid and update it IN PLACE (stable node_id); only patterns matched
	// by no cluster this run are deleted at the end.
	existingPatterns, err := s.listHiddenPatternRefs(ctx, spaceID)
	if err != nil {
		return 0, fmt.Errorf("list existing hidden patterns: %w", err)
	}
	patternMemberIndex := buildMemberIndex(existingPatterns)
	jaccardMin := s.hiddenPatternMemberJaccardThreshold()
	cosineMin := s.hiddenPatternIdentityThreshold()
	claimedPatterns := make(map[string]bool, len(existingPatterns))

	// Step 2: Filter to nodes with valid embeddings
	validNodes := make([]BaseNode, 0, len(baseNodes))
	for _, node := range baseNodes {
		if len(node.Embedding) > 0 {
			validNodes = append(validNodes, node)
		}
	}

	if len(validNodes) < s.cfg.HiddenLayerMinSamples {
		return 0, nil
	}

	// Step 3: Classification-then-clustering.
	// First classify nodes by file extension into semantic categories,
	// then KMeans within each category. This produces coherent clusters because
	// Go code clusters with Go code, docs with docs, config with config —
	// regardless of embedding density distribution.
	classes := ClassifyByExtension(validNodes)
	slog.Info("CreateHiddenNodes: nodes classified", "valid_nodes", len(validNodes), "categories", len(classes))
	for cat, nodes := range classes {
		slog.Info("CreateHiddenNodes: category", "category", cat, "nodes", len(nodes))
	}

	// Dynamic reclassification of oversized categories (iterates until convergence)
	var categoryDescriptions map[string]string
	if s.cfg.ReclassEnabled {
		if rc := s.newReclassifier(); rc != nil {
			classes = rc.ReclassifyOversizedCategories(ctx, classes, len(validNodes))
			categoryDescriptions = rc.CategoryDescriptions
		}
	}

	// Step 4: Get existing hidden node count for unique naming
	existingCount, err := s.countHiddenNodes(ctx, spaceID)
	if err != nil {
		return 0, fmt.Errorf("count existing hidden nodes: %w", err)
	}

	// Step 5: KMeans within each classification category
	created := 0
	updated := 0
	clusterID := 0

	for category, classNodes := range classes {
		if len(classNodes) < s.cfg.HiddenLayerMinSamples {
			slog.Info("CreateHiddenNodes: skipping category below min_samples", "category", category, "nodes", len(classNodes), "min_samples", s.cfg.HiddenLayerMinSamples)
			continue
		}

		// maxHidden per class = ceil(classSize * 0.1)
		classK := (len(classNodes) + 9) / 10
		if classK < 1 {
			classK = 1
		}

		embeddings := extractEmbeddings(classNodes)
		labels := KMeansCluster(embeddings, classK, 50)
		clusters, _ := GroupByCluster(classNodes, labels)
		slog.Info("CreateHiddenNodes: KMeans result", "category", category, "k", classK, "clusters", len(clusters), "nodes", len(classNodes))

		for _, members := range clusters {
			if len(members) < s.cfg.HiddenLayerMinSamples {
				continue
			}

			// For oversized clusters, chunk them
			subClusters := SplitLargeCluster(members, s.cfg.HiddenLayerMaxClusterSize)

			for _, subMembers := range subClusters {
				if len(subMembers) < s.cfg.HiddenLayerMinSamples {
					continue
				}

				centroid := ComputeCentroid(extractEmbeddings(subMembers))
				if centroid == nil {
					continue
				}

				uniqueID := existingCount + clusterID
				clusterName := inferClusterName(subMembers, s.cfg.HiddenLayerPathGroupDepth)
				name := fmt.Sprintf("Hidden-%s-%s-%d", category, clusterName, uniqueID)
				catDesc := categoryDescriptions[category] // may be "" for non-reclassified categories

				// HIDDEN-CHURN-002: match-or-create. A cluster that overlaps an
				// existing pattern's L0 members (Jaccard) — or, failing that, is
				// centroid-close — UPDATES that pattern in place, so node_id and
				// inbound references survive the cycle instead of being destroyed
				// and recreated. Member overlap is the primary signal because it
				// is stable under KMeans repartition jitter (centroid-only left
				// ~28% churn/cycle in live Tier-3).
				clusterMemberIDs := make([]string, len(subMembers))
				for mi, m := range subMembers {
					clusterMemberIDs[mi] = m.NodeID
				}
				if matched := matchHiddenPattern(clusterMemberIDs, centroid, existingPatterns, patternMemberIndex, claimedPatterns, jaccardMin, cosineMin); matched != "" {
					claimedPatterns[matched] = true
					if _, err := s.updateHiddenNodeWithEdges(ctx, spaceID, matched, name, centroid, subMembers, category, catDesc); err != nil {
						return created, fmt.Errorf("update hidden node %s: %w", matched, err)
					}
					updated++
					clusterID++
					continue
				}

				// Unmatched cluster → new pattern with a CUIDv2 id (Cypher
				// cannot mint CUIDv2; HIDDEN-CHURN-002 replaces randomUUID()).
				newID := cuid2.Generate()
				if err := s.createHiddenNodeWithEdges(ctx, spaceID, newID, name, centroid, subMembers, category, catDesc); err != nil {
					return created, fmt.Errorf("create hidden node %s: %w", name, err)
				}
				created++
				clusterID++

				if created%50 == 0 {
					slog.Info("hidden node progress", "created", created)
				}
			}
		}
	}

	// HIDDEN-CHURN-002: only patterns matched by NO cluster this run die —
	// the scoped replacement for the old wipe-all orphan sweep.
	if removed, err := s.deleteUnmatchedHiddenPatterns(ctx, spaceID, claimedPatterns, existingPatterns); err != nil {
		slog.Warn("CreateHiddenNodes: stale-pattern cleanup failed", "error", err)
	} else if removed > 0 {
		slog.Info("CreateHiddenNodes: removed stale hidden patterns", "count", removed)
	}

	slog.Info("hidden node creation complete", "created", created, "updated", updated)
	return created, nil
}

// IncrementalHiddenNodes is HIDDEN-CHURN-003's default consolidation hidden step:
// it assigns only ORPHAN L0 nodes (no GENERALIZES edge) to their nearest existing
// pattern, and clusters the unassigned remainder into NEW patterns. Existing
// patterns are NEVER destroyed or re-clustered, so their node_ids (and every
// reinforcement/abstraction edge referencing them) survive every cycle (~0%
// churn), and only the small orphan set is clustered (no full-52k-node re-cluster
// → much lower CPU). Returns (newPatternsCreated, orphansAssigned).
func (s *Service) IncrementalHiddenNodes(ctx context.Context, spaceID string) (int, int, error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, 0, nil
	}

	orphans, err := s.fetchOrphanBaseNodes(ctx, spaceID)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch orphan base nodes: %w", err)
	}
	valid := make([]BaseNode, 0, len(orphans))
	for _, n := range orphans {
		if len(n.Embedding) > 0 {
			valid = append(valid, n)
		}
	}
	if len(valid) == 0 {
		return 0, 0, nil // nothing new to assign — existing patterns untouched
	}
	slog.Info("IncrementalHiddenNodes: orphan L0 nodes", "count", len(valid))

	refs, err := s.listHiddenPatternRefs(ctx, spaceID)
	if err != nil {
		return 0, 0, fmt.Errorf("list hidden patterns: %w", err)
	}

	assigned, unassigned, err := s.assignOrphansToPatterns(ctx, spaceID, valid, refs, s.hiddenIncrementalAssignThreshold())
	if err != nil {
		return 0, 0, err
	}

	existingCount, err := s.countHiddenNodes(ctx, spaceID)
	if err != nil {
		return 0, assigned, fmt.Errorf("count existing hidden nodes: %w", err)
	}
	created, err := s.clusterNewBaseNodes(ctx, spaceID, unassigned, existingCount)
	if err != nil {
		return created, assigned, err
	}

	slog.Info("IncrementalHiddenNodes complete", "assigned", assigned, "new_patterns", created, "unassigned_clustered", len(unassigned))
	return created, assigned, nil
}

// clusterNewBaseNodes clusters a set of base nodes into NEW hidden patterns
// (classify → KMeans → create with a CUIDv2 id). Create-only: no matching, no
// delete — used for the orphan remainder that cleared no existing pattern.
// Reclassification (the non-deterministic LLM step) is intentionally skipped:
// the remainder is small and reclassification is a churn source.
func (s *Service) clusterNewBaseNodes(ctx context.Context, spaceID string, nodes []BaseNode, existingCount int) (int, error) {
	if len(nodes) < s.cfg.HiddenLayerMinSamples {
		return 0, nil
	}
	classes := ClassifyByExtension(nodes)
	created := 0
	clusterID := 0
	for category, classNodes := range classes {
		if len(classNodes) < s.cfg.HiddenLayerMinSamples {
			continue
		}
		classK := (len(classNodes) + 9) / 10
		if classK < 1 {
			classK = 1
		}
		labels := KMeansCluster(extractEmbeddings(classNodes), classK, 50)
		clusters, _ := GroupByCluster(classNodes, labels)
		for _, members := range clusters {
			if len(members) < s.cfg.HiddenLayerMinSamples {
				continue
			}
			for _, subMembers := range SplitLargeCluster(members, s.cfg.HiddenLayerMaxClusterSize) {
				if len(subMembers) < s.cfg.HiddenLayerMinSamples {
					continue
				}
				centroid := ComputeCentroid(extractEmbeddings(subMembers))
				if centroid == nil {
					continue
				}
				name := fmt.Sprintf("Hidden-%s-%s-%d", category, inferClusterName(subMembers, s.cfg.HiddenLayerPathGroupDepth), existingCount+clusterID)
				if err := s.createHiddenNodeWithEdges(ctx, spaceID, cuid2.Generate(), name, centroid, subMembers, category, ""); err != nil {
					return created, fmt.Errorf("create hidden node %s: %w", name, err)
				}
				created++
				clusterID++
			}
		}
	}
	return created, nil
}

// extractEmbeddings extracts embeddings from a slice of nodes
func extractEmbeddings(nodes []BaseNode) [][]float64 {
	embeddings := make([][]float64, len(nodes))
	for i, n := range nodes {
		embeddings[i] = n.Embedding
	}
	return embeddings
}

// inferClusterName determines a name for a cluster based on most common path patterns
func inferClusterName(members []BaseNode, depth int) string {
	if len(members) == 0 {
		return "misc"
	}

	// Count path prefixes
	prefixCounts := make(map[string]int)
	for _, m := range members {
		prefix := extractPathPrefix(m.Path, depth)
		prefixCounts[prefix]++
	}

	// Find most common prefix
	maxCount := 0
	mostCommon := "misc"
	for prefix, count := range prefixCounts {
		if count > maxCount {
			maxCount = count
			mostCommon = prefix
		}
	}

	// If dominant prefix covers >50% of members, use it; otherwise mark as "mixed"
	if float64(maxCount)/float64(len(members)) > 0.5 {
		return sanitizePathPrefix(mostCommon)
	}
	return "mixed"
}

// sanitizePathPrefix cleans a path prefix for use in node names
func sanitizePathPrefix(prefix string) string {
	if prefix == "" || prefix == "_unknown_" || prefix == "_root_" {
		return "misc"
	}
	// Replace slashes with dashes and limit length
	result := ""
	for _, ch := range prefix {
		if ch == '/' {
			result += "-"
		} else if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			result += string(ch)
		}
	}
	if len(result) > 30 {
		result = result[:30]
	}
	if result == "" {
		return "misc"
	}
	return result
}

// countHiddenNodes returns the current count of hidden nodes for unique naming
func (s *Service) countHiddenNodes(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1, role_type: 'hidden'})
RETURN count(h) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("cnt")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// fetchAllBaseNodes retrieves ALL L0 base nodes with embeddings for full
// re-clustering. Does not filter by existing GENERALIZES edges.
func (s *Service) fetchAllBaseNodes(ctx context.Context, spaceID string) ([]BaseNode, error) {
	return s.fetchBaseNodes(ctx, spaceID, false)
}

// fetchOrphanBaseNodes retrieves only L0 base nodes NOT yet assigned to any L1
// hidden pattern (no GENERALIZES edge) — the incremental input for
// HIDDEN-CHURN-003. Clustering only these (a few hundred per cycle) instead of
// the full ~52k set is what eliminates the per-cycle membership reshuffle that
// left ~25% identity churn (and drove the full-recluster CPU cost).
func (s *Service) fetchOrphanBaseNodes(ctx context.Context, spaceID string) ([]BaseNode, error) {
	return s.fetchBaseNodes(ctx, spaceID, true)
}

func (s *Service) fetchBaseNodes(ctx context.Context, spaceID string, orphanOnly bool) ([]BaseNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// HIDDEN-CHURN-002: ORDER BY node_id makes the L0 fetch order DETERMINISTIC.
	// KMeans init here is order-dependent (first centroid = point 0, then
	// farthest-first), so a stable input order yields reproducible partitions
	// across cycles — which is what lets member-overlap / centroid matching find
	// the same patterns next cycle. Without it, Neo4j's arbitrary scan order
	// reshuffled members every run and defeated identity matching (~28% churn).
	orphanFilter := ""
	if orphanOnly {
		orphanFilter = "  AND NOT (b)-[:GENERALIZES]->(:HiddenPattern {space_id: $spaceId, layer: 1})\n"
	}
	cypher := `
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})
WHERE b.embedding IS NOT NULL
  AND (b.role_type IS NULL OR b.role_type <> 'conversation_observation')
` + orphanFilter + `RETURN b.node_id AS nodeId, b.path AS path, b.embedding AS embedding,
       coalesce(b.summary, '') AS summary
ORDER BY b.node_id`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var nodes []BaseNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			path, _ := rec.Get("path")
			embedding, _ := rec.Get("embedding")
			summary, _ := rec.Get("summary")

			nodes = append(nodes, BaseNode{
				NodeID:    asString(nodeID),
				SpaceID:   spaceID,
				Path:      asString(path),
				Summary:   asString(summary),
				Embedding: asFloat64Slice(embedding),
			})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]BaseNode), nil
}

// createHiddenNodeWithEdges creates a hidden node and GENERALIZES edges from members
func (s *Service) createHiddenNodeWithEdges(ctx context.Context, spaceID, nodeID, name string, centroid []float64, members []BaseNode, category, categorySummary string) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
CREATE (h:MemoryNode:HiddenPattern {
  space_id: $spaceId,
  node_id: $nodeId,
  name: $name,
  layer: 1,
  role_type: 'hidden',
  embedding: $centroid,
  message_pass_embedding: $centroid,
  aggregation_count: $memberCount,
  stability_score: 1.0,
  last_forward_pass: datetime(),
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH h
UNWIND $memberEdges AS me
MATCH (b:MemoryNode {space_id: $spaceId, node_id: me.memberId})
CREATE (b)-[:GENERALIZES {
  space_id: $spaceId,
  edge_id: me.edgeId,
  // HIDDEN-WEIGHT-001: point.distance() is a spatial-Point function — on
  // embedding lists it returns NULL, so this weight was never set (100% of
  // GENERALIZES edges were weightless). vector.similarity.cosine returns
  // [0,1] directly. Null-guard added (this site had none).
  weight: CASE WHEN b.embedding IS NOT NULL AND h.embedding IS NOT NULL
          THEN vector.similarity.cosine(b.embedding, h.embedding)
          ELSE 0.5
          END,
  category: $category,
  category_summary: $categorySummary,
  created_at: datetime(),
  updated_at: datetime()
}]->(h)
RETURN h.node_id AS hiddenId, count(b) AS edgeCount`

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":         spaceID,
			"nodeId":          nodeID,
			"name":            name,
			"centroid":        toFloat32Slice(centroid),
			"memberCount":     len(members),
			"memberEdges":     memberEdgePairs(memberIDs),
			"category":        category,
			"categorySummary": categorySummary,
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})

	return err
}

// GenerateSummaries creates summaries for hidden and concept nodes based on their members
func (s *Service) GenerateSummaries(ctx context.Context, spaceID string) (int, error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Update hidden nodes (layer 1) with summaries from base node paths
	// Extract parent directories from paths and create a summary
	cypherHidden := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WHERE h.summary IS NULL OR h.summary = ''
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[:GENERALIZES]->(h)
WITH h, count(b) AS memberCount,
     collect(DISTINCT CASE
       WHEN b.path IS NOT NULL AND size(split(b.path, '/')) > 2
       THEN split(b.path, '/')[-2]
       ELSE null
     END) AS dirs
WITH h, memberCount, [d IN dirs WHERE d IS NOT NULL][0..5] AS topDirs
SET h.summary = 'Pattern of ' + toString(memberCount) + ' code elements' +
    CASE WHEN size(topDirs) > 0
      THEN ' in: ' + reduce(s = '', d IN topDirs | s + CASE WHEN s = '' THEN '' ELSE ', ' END + d)
      ELSE ''
    END,
    h.updated_at = datetime()
RETURN count(h) AS updated`

	hiddenUpdated, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypherHidden, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("updated")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("update hidden summaries: %w", err)
	}

	// Update concept nodes (layer 2+) with summaries from hidden node names
	cypherConcept := `
MATCH (c:MemoryNode {space_id: $spaceId})
WHERE c.layer >= 2 AND (c.summary IS NULL OR c.summary = '')
MATCH (h:MemoryNode)-[:ABSTRACTS_TO]->(c)
WITH c, collect(DISTINCT coalesce(h.name, h.node_id)) AS hiddenNames,
     collect(DISTINCT h.summary) AS hiddenSummaries
WITH c,
     size(hiddenNames) AS hiddenCount,
     [s IN hiddenSummaries WHERE s IS NOT NULL AND s <> ''][0..3] AS topSummaries
SET c.summary = 'Meta-concept over ' + toString(hiddenCount) + ' clusters' +
    CASE WHEN size(topSummaries) > 0
      THEN '. Sub-concepts: ' + reduce(s = '', t IN topSummaries | s + CASE WHEN s = '' THEN '' ELSE '; ' END + t)
      ELSE ''
    END,
    c.updated_at = datetime()
RETURN count(c) AS updated`

	conceptUpdated, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypherConcept, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("updated")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return hiddenUpdated.(int), fmt.Errorf("update concept summaries: %w", err)
	}

	return hiddenUpdated.(int) + conceptUpdated.(int), nil
}

// CreateConceptNodes performs hybrid DBSCAN clustering on source layer nodes
// Strategy (hybrid - allows emergent patterns):
//  1. Run DBSCAN on ALL source nodes first (embedding-based clustering)
//  2. Split oversized clusters using name-prefix grouping (secondary organization)
//  3. Name clusters based on dominant patterns (descriptive, not prescriptive)
//  4. Small clusters stay intact - emergent cross-cutting patterns
//
// ADAPTIVE CONSTRAINTS (loosen as layers increase):
//   - Epsilon increases with layer (allows more distant concepts to cluster)
//   - MinSamples decreases with layer (smaller emergent groups allowed)
//   - MaxClusterSize stays generous (concepts can be broad)
func (s *Service) CreateConceptNodes(ctx context.Context, spaceID string, targetLayer int) (created int, merged int, err error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, 0, nil
	}

	sourceLayer := targetLayer - 1
	if sourceLayer < 1 {
		return 0, 0, fmt.Errorf("target layer must be >= 2 (source layer 1 = hidden)")
	}

	// Step 1: Fetch source layer nodes (includes name for secondary grouping)
	sourceNodes, err := s.fetchOrphanLayerNodesWithName(ctx, spaceID, sourceLayer, targetLayer)
	if err != nil {
		return 0, 0, fmt.Errorf("fetch orphan layer %d nodes: %w", sourceLayer, err)
	}

	// Calculate ADAPTIVE parameters - constraints loosen as we go up
	// Higher layers represent more abstract concepts that should cluster more freely
	layerFactor := float64(targetLayer - 1) // 1 for L2, 2 for L3, etc.

	// MinSamples shrinks with layer: base - layer (min 2) → allows smaller emergent clusters at top
	adaptiveMinSamples := s.cfg.HiddenLayerMinSamples - int(layerFactor)
	if adaptiveMinSamples < 2 {
		adaptiveMinSamples = 2 // Minimum 2 nodes to form a cluster
	}

	if len(sourceNodes) < adaptiveMinSamples {
		return 0, 0, nil // Not enough data to cluster at this layer
	}

	// Step 2: Filter to nodes with embeddings, use message_pass_embedding if available
	validNodes := make([]BaseNode, 0, len(sourceNodes))
	for _, node := range sourceNodes {
		emb := node.Embedding
		if len(node.MessagePassEmbedding) > 0 {
			emb = node.MessagePassEmbedding
		}
		if len(emb) > 0 {
			node.Embedding = emb // Store effective embedding
			validNodes = append(validNodes, node)
		}
	}

	if len(validNodes) < adaptiveMinSamples {
		return 0, 0, nil
	}

	// maxHidden = ceil(sourceNodes * 0.1) — this is the equation, NOT a configurable cap.
	maxHidden := (len(validNodes) + 9) / 10 // ceil(len/10)
	if maxHidden < 1 {
		maxHidden = 1
	}

	// Step 3: KMeans clustering to partition into exactly maxHidden groups.
	embeddings := extractEmbeddings(validNodes)
	labels := KMeansCluster(embeddings, maxHidden, 50)
	clusters, _ := GroupByCluster(validNodes, labels)
	slog.Info("concept KMeans result", "layer", targetLayer, "k", maxHidden, "clusters", len(clusters), "nodes", len(validNodes))

	// Step 4: Get existing concept node count for unique naming
	existingCount, err := s.countLayerNodes(ctx, spaceID, targetLayer)
	if err != nil {
		return 0, 0, fmt.Errorf("count existing layer %d nodes: %w", targetLayer, err)
	}

	// Max cluster size stays generous - don't artificially limit concept breadth
	// Higher layers can have broader concepts (gradual reduction only)
	maxConceptSize := s.cfg.HiddenLayerMaxClusterSize
	if targetLayer >= 4 {
		// Only slight reduction at very high layers to prevent mega-clusters
		maxConceptSize = maxConceptSize * 3 / 4
	}

	// Step 5: Process each natural cluster
	created = 0
	merged = 0
	clusterID := 0

	for _, members := range clusters {
		if created >= maxHidden {
			break
		}

		// For clusters within size limit, keep them intact (emergent patterns)
		if len(members) <= maxConceptSize {
			if len(members) < adaptiveMinSamples {
				continue // Use adaptive threshold, not fixed
			}

			centroid := ComputeCentroid(extractEmbeddings(members))
			if centroid == nil {
				continue
			}

			// Check for existing similar concept to merge into
			if s.cfg.ConceptMergeEnabled {
				existingID, similarity, mergeErr := s.findSimilarConcept(ctx, spaceID, targetLayer, centroid, s.cfg.ConceptMergeThreshold)
				if mergeErr == nil && existingID != "" && similarity > s.cfg.ConceptMergeThreshold {
					// Merge into existing concept instead of creating new
					mergeErr = s.mergeIntoExistingConcept(ctx, spaceID, existingID, members, centroid)
					if mergeErr == nil {
						merged++
						continue // Skip creating new node
					}
					// If merge failed, fall through to create new node
				}
			}

			// Name based on most common name prefix pattern (mechanical fallback)
			uniqueID := existingCount + clusterID
			inferredName := inferConceptName(members)
			name := fmt.Sprintf("Concept-L%d-%s-%d", targetLayer, inferredName, uniqueID)
			if targetLayer == 5 {
				if namer := s.newEmergenceNamer(); namer != nil {
					clusterNodes := baseNodesToClusterSummaries(members)
					if result, llmErr := namer.NameL5Concept(ctx, clusterNodes); llmErr != nil {
						fmt.Printf("L5 concept LLM naming failed (using mechanical): %v\n", llmErr)
					} else {
						name = result.Name
					}
				}
			}
			err := s.createConceptNodeWithEdges(ctx, spaceID, name, centroid, members, targetLayer)
			if err != nil {
				return created, merged, fmt.Errorf("create concept node %s: %w", name, err)
			}
			created++
			clusterID++
			continue
		}

		// For oversized clusters, use name-prefix splitting as secondary organization
		nameGroups := groupByNamePrefix(members)

		for namePrefix, groupMembers := range nameGroups {
			if created >= maxHidden {
				break
			}

			// Split groups that are still too large
			subClusters := SplitLargeCluster(groupMembers, maxConceptSize)

			for _, subMembers := range subClusters {
				if created >= maxHidden {
					break
				}

				if len(subMembers) < s.cfg.HiddenLayerMinSamples {
					continue
				}

				centroid := ComputeCentroid(extractEmbeddings(subMembers))
				if centroid == nil {
					continue
				}

				// Check for existing similar concept to merge into
				if s.cfg.ConceptMergeEnabled {
					existingID, similarity, mergeErr := s.findSimilarConcept(ctx, spaceID, targetLayer, centroid, s.cfg.ConceptMergeThreshold)
					if mergeErr == nil && existingID != "" && similarity > s.cfg.ConceptMergeThreshold {
						// Merge into existing concept instead of creating new
						mergeErr = s.mergeIntoExistingConcept(ctx, spaceID, existingID, subMembers, centroid)
						if mergeErr == nil {
							merged++
							continue // Skip creating new node
						}
						// If merge failed, fall through to create new node
					}
				}

				uniqueID := existingCount + clusterID
				name := fmt.Sprintf("Concept-L%d-%s-%d", targetLayer, sanitizePathPrefix(namePrefix), uniqueID)
				if targetLayer == 5 {
					if namer := s.newEmergenceNamer(); namer != nil {
						clusterNodes := baseNodesToClusterSummaries(subMembers)
						if result, llmErr := namer.NameL5Concept(ctx, clusterNodes); llmErr != nil {
							fmt.Printf("L5 concept LLM naming failed (using mechanical): %v\n", llmErr)
						} else {
							name = result.Name
						}
					}
				}
				err := s.createConceptNodeWithEdges(ctx, spaceID, name, centroid, subMembers, targetLayer)
				if err != nil {
					return created, merged, fmt.Errorf("create concept node %s: %w", name, err)
				}
				created++
				clusterID++
			}
		}
	}

	return created, merged, nil
}

// groupByNamePrefix groups nodes by extracting the prefix from their Path field (which contains the name for layer 1+ nodes)
func groupByNamePrefix(nodes []BaseNode) map[string][]BaseNode {
	groups := make(map[string][]BaseNode)

	for _, node := range nodes {
		prefix := extractNamePrefix(node.Path)
		groups[prefix] = append(groups[prefix], node)
	}

	return groups
}

// extractNamePrefix extracts the prefix from a node name by removing the trailing number
// e.g., "Hidden-apps-whk-wms-39" -> "Hidden-apps-whk-wms"
func extractNamePrefix(name string) string {
	if name == "" {
		return "_unknown_"
	}

	// Find the last dash followed by only digits
	lastDash := -1
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' {
			suffix := name[i+1:]
			allDigits := len(suffix) > 0
			for _, ch := range suffix {
				if ch < '0' || ch > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				lastDash = i
				break
			}
		}
	}

	if lastDash > 0 {
		return name[:lastDash]
	}
	return name
}

// inferConceptName determines a name for a concept cluster based on member name patterns
func inferConceptName(members []BaseNode) string {
	if len(members) == 0 {
		return "misc"
	}

	// Count name prefixes
	prefixCounts := make(map[string]int)
	for _, m := range members {
		prefix := extractNamePrefix(m.Path)
		// Further simplify: extract the key part after "Hidden-" or "Concept-Ln-"
		simplified := simplifyPrefix(prefix)
		prefixCounts[simplified]++
	}

	// Find most common
	maxCount := 0
	mostCommon := "mixed"
	for prefix, count := range prefixCounts {
		if count > maxCount {
			maxCount = count
			mostCommon = prefix
		}
	}

	// If dominant prefix covers >50%, use it; otherwise "mixed"
	if float64(maxCount)/float64(len(members)) > 0.5 {
		return sanitizePathPrefix(mostCommon)
	}
	return "mixed"
}

// simplifyPrefix extracts the meaningful part from a hidden/concept node name prefix
// e.g., "Hidden-apps-whk-wms" -> "apps-whk-wms"
// e.g., "Concept-L2-frontend" -> "frontend"
func simplifyPrefix(prefix string) string {
	if len(prefix) > 7 && prefix[:7] == "Hidden-" {
		return prefix[7:]
	}
	// Handle "Concept-Ln-" pattern
	if len(prefix) > 10 && prefix[:8] == "Concept-" {
		// Find the second dash after "Concept-Ln"
		for i := 9; i < len(prefix); i++ {
			if prefix[i] == '-' {
				return prefix[i+1:]
			}
		}
	}
	return prefix
}

// fetchOrphanLayerNodesWithName retrieves nodes from sourceLayer with name for grouping
func (s *Service) fetchOrphanLayerNodesWithName(ctx context.Context, spaceID string, sourceLayer, targetLayer int) ([]BaseNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: $sourceLayer})
WHERE NOT (n)-[:ABSTRACTS_TO]->(:MemoryNode {layer: $targetLayer})
  AND (n.embedding IS NOT NULL OR n.message_pass_embedding IS NOT NULL)
RETURN n.node_id AS nodeId, n.name AS name, n.embedding AS embedding, n.message_pass_embedding AS messagePassEmbedding`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"sourceLayer": sourceLayer,
			"targetLayer": targetLayer,
		})
		if err != nil {
			return nil, err
		}

		var nodes []BaseNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			embedding, _ := rec.Get("embedding")
			msgPassEmb, _ := rec.Get("messagePassEmbedding")

			nodes = append(nodes, BaseNode{
				NodeID:               asString(nodeID),
				SpaceID:              spaceID,
				Path:                 asString(name), // Store name in Path for grouping
				Embedding:            asFloat64Slice(embedding),
				MessagePassEmbedding: asFloat64Slice(msgPassEmb),
			})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]BaseNode), nil
}

// countLayerNodes returns the count of nodes at a specific layer
func (s *Service) countLayerNodes(ctx context.Context, spaceID string, layer int) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: $layer})
RETURN count(n) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID, "layer": layer})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("cnt")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// createConceptNodeWithEdges creates a concept node and ABSTRACTS_TO edges from members
func (s *Service) createConceptNodeWithEdges(ctx context.Context, spaceID, name string, centroid []float64, members []BaseNode, layer int) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
CREATE (c:MemoryNode:Concept {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $name,
  layer: $layer,
  role_type: 'concept',
  embedding: $centroid,
  message_pass_embedding: $centroid,
  aggregation_count: $memberCount,
  stability_score: 1.0,
  last_forward_pass: datetime(),
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c
UNWIND $memberIds AS memberId
MATCH (m:MemoryNode {space_id: $spaceId, node_id: memberId})
CREATE (m)-[:ABSTRACTS_TO {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS conceptId, count(m) AS edgeCount`

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"name":        name,
			"layer":       layer,
			"centroid":    toFloat32Slice(centroid),
			"memberCount": len(members),
			"memberIds":   memberIDs,
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})

	return err
}

// findSimilarConcept searches for an existing concept at the same layer with similar embedding.
// Returns the node ID and similarity score if found, empty string otherwise.
func (s *Service) findSimilarConcept(ctx context.Context, spaceID string, layer int, centroid []float64, threshold float64) (string, float64, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Query existing concepts at same layer and compute cosine similarity
	cypher := `
	MATCH (c:MemoryNode {space_id: $spaceId, layer: $layer, role_type: 'concept'})
	WHERE c.embedding IS NOT NULL
	WITH c, vector.similarity.cosine(c.embedding, $centroid) AS similarity
	WHERE similarity > $threshold
	RETURN c.node_id AS nodeId, similarity
	ORDER BY similarity DESC
	LIMIT 1`

	var bestNodeID string
	var bestSimilarity float64

	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"layer":     layer,
			"centroid":  toFloat32Slice(centroid),
			"threshold": threshold,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			record := res.Record()
			nodeID, _ := record.Get("nodeId")
			similarity, _ := record.Get("similarity")
			bestNodeID = nodeID.(string)
			bestSimilarity = similarity.(float64)
		}
		return nil, res.Err()
	})

	return bestNodeID, bestSimilarity, err
}

// mergeIntoExistingConcept merges new cluster members into an existing concept.
// Creates ABSTRACTS_TO edges and updates the concept's embedding via exponential moving average.
func (s *Service) mergeIntoExistingConcept(ctx context.Context, spaceID, existingNodeID string, newMembers []BaseNode, newCentroid []float64) error {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(newMembers))
	for i, m := range newMembers {
		memberIDs[i] = m.NodeID
	}

	// EMA alpha for updating embedding (0.3 weights new centroid, 0.7 keeps existing)
	const emaAlpha = 0.3

	// Two-phase update:
	// 1. Create new ABSTRACTS_TO edges from new members to existing concept
	// 2. Update concept's embedding using EMA: new = (1-α)*old + α*newCentroid
	cypher := `
	MATCH (c:MemoryNode {space_id: $spaceId, node_id: $conceptId})
	WITH c
	UNWIND $memberIds AS memberId
	MATCH (m:MemoryNode {space_id: $spaceId, node_id: memberId})
	WHERE NOT EXISTS((m)-[:ABSTRACTS_TO]->(c))
	CREATE (m)-[:ABSTRACTS_TO {
	  space_id: $spaceId,
	  edge_id: randomUUID(),
	  weight: 1.0,
	  created_at: datetime(),
	  updated_at: datetime()
	}]->(c)
	WITH c, count(m) AS newEdges
	SET c.aggregation_count = c.aggregation_count + newEdges,
	    c.embedding = [i IN range(0, size(c.embedding)-1) |
	        (1.0 - $emaAlpha) * c.embedding[i] + $emaAlpha * $newCentroid[i]],
	    c.message_pass_embedding = [i IN range(0, size(c.message_pass_embedding)-1) |
	        (1.0 - $emaAlpha) * c.message_pass_embedding[i] + $emaAlpha * $newCentroid[i]],
	    c.updated_at = datetime()
	RETURN c.node_id AS conceptId, newEdges`

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"conceptId":   existingNodeID,
			"memberIds":   memberIDs,
			"newCentroid": toFloat32Slice(newCentroid),
			"emaAlpha":    emaAlpha,
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})

	return err
}

// ForwardPass updates hidden and concept layer embeddings by aggregating from lower layers
func (s *Service) ForwardPass(ctx context.Context, spaceID string) (*ForwardPassResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ForwardPassResult{}, nil
	}

	start := time.Now()
	result := &ForwardPassResult{}

	// Phase 1: Update hidden layer from base data
	hiddenUpdated, err := s.forwardPassHiddenLayer(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass hidden layer: %w", err)
	}
	result.HiddenNodesUpdated = hiddenUpdated

	// Phase 2: Update concept layer from hidden (using ABSTRACTS_TO)
	conceptUpdated, err := s.forwardPassConceptLayer(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass concept layer: %w", err)
	}
	result.ConceptNodesUpdated = conceptUpdated

	result.Duration = time.Since(start)
	return result, nil
}

// forwardPassHiddenLayer aggregates base node embeddings into hidden nodes.
// Batched to prevent OOM on large graphs — each batch processes a subset of
// hidden nodes in its own transaction.
func (s *Service) forwardPassHiddenLayer(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// GraphSAGE-style weighted mean aggregation with embedding combination.
	// Uses SKIP/LIMIT to process hidden nodes in batches.
	cypher := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WITH h ORDER BY h.node_id SKIP $skip LIMIT $limit
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[r:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, collect({emb: b.embedding, weight: coalesce(r.weight, 1.0)}) AS neighbors
WHERE size(neighbors) > 0
WITH h, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH h, neighbors, totalWeight,
     [i IN range(0, size(h.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $alpha * coalesce(h.embedding[i], 0) + $beta * aggregated[i]
    ],
    h.last_forward_pass = datetime(),
    h.aggregation_count = size(neighbors)
RETURN count(h) AS updated`

	const forwardBatchSize = 50
	totalUpdated := 0
	for skip := 0; ; skip += forwardBatchSize {
		result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, cypher, map[string]any{
				"spaceId": spaceID,
				"alpha":   s.cfg.HiddenLayerForwardAlpha,
				"beta":    s.cfg.HiddenLayerForwardBeta,
				"skip":    skip,
				"limit":   forwardBatchSize,
			})
			if err != nil {
				return 0, err
			}
			if res.Next(ctx) {
				rec := res.Record()
				updated, _ := rec.Get("updated")
				return asInt(updated), res.Err()
			}
			return 0, res.Err()
		})

		if err != nil {
			return totalUpdated, fmt.Errorf("forward pass hidden batch at skip=%d: %w", skip, err)
		}
		batch := result.(int)
		totalUpdated += batch
		if batch == 0 {
			break
		}
	}
	return totalUpdated, nil
}

// forwardPassConceptLayer aggregates hidden node embeddings into concept nodes
func (s *Service) forwardPassConceptLayer(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Update concept nodes (layer >= 2) from hidden layer via ABSTRACTS_TO.
	// Batched to prevent OOM on large graphs.
	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId})
WHERE c.layer >= 2
WITH c ORDER BY c.node_id SKIP $skip LIMIT $limit
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})-[r:ABSTRACTS_TO]->(c)
WHERE h.message_pass_embedding IS NOT NULL OR h.embedding IS NOT NULL
WITH c, collect({
  emb: coalesce(h.message_pass_embedding, h.embedding),
  weight: coalesce(r.weight, 1.0)
}) AS neighbors
WHERE size(neighbors) > 0
WITH c, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH c, neighbors, totalWeight,
     [i IN range(0, size(c.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET c.message_pass_embedding = [i IN range(0, size(c.embedding)-1) |
      $alpha * coalesce(c.embedding[i], 0) + $beta * aggregated[i]
    ],
    c.last_forward_pass = datetime(),
    c.aggregation_count = size(neighbors)
RETURN count(c) AS updated`

	const conceptBatchSize = 50
	totalUpdated := 0
	for skip := 0; ; skip += conceptBatchSize {
		result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, cypher, map[string]any{
				"spaceId": spaceID,
				"alpha":   s.cfg.HiddenLayerForwardAlpha,
				"beta":    s.cfg.HiddenLayerForwardBeta,
				"skip":    skip,
				"limit":   conceptBatchSize,
			})
			if err != nil {
				return 0, err
			}
			if res.Next(ctx) {
				rec := res.Record()
				updated, _ := rec.Get("updated")
				return asInt(updated), res.Err()
			}
			return 0, res.Err()
		})

		if err != nil {
			return totalUpdated, fmt.Errorf("forward pass concept batch at skip=%d: %w", skip, err)
		}
		batch := result.(int)
		totalUpdated += batch
		if batch == 0 {
			break
		}
	}
	return totalUpdated, nil
}

// BackwardPass propagates feedback from concepts to hidden layers
func (s *Service) BackwardPass(ctx context.Context, spaceID string) (*BackwardPassResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &BackwardPassResult{}, nil
	}

	start := time.Now()
	result := &BackwardPassResult{}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Update hidden nodes with signals from both concepts (above) and base data (below).
	// Batched to prevent OOM on large graphs.
	cypher := `
MATCH (h:MemoryNode {space_id: $spaceId, layer: 1})
WITH h ORDER BY h.node_id SKIP $skip LIMIT $limit
OPTIONAL MATCH (h)-[rUp:ABSTRACTS_TO]->(c:MemoryNode)
WHERE c.layer >= 2 AND (c.message_pass_embedding IS NOT NULL OR c.embedding IS NOT NULL)
WITH h, collect(coalesce(c.message_pass_embedding, c.embedding)) AS conceptEmbs
OPTIONAL MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})-[rDown:GENERALIZES]->(h)
WHERE b.embedding IS NOT NULL
WITH h, conceptEmbs, collect(b.embedding) AS baseEmbs
WHERE size(conceptEmbs) > 0 OR size(baseEmbs) > 0
WITH h, conceptEmbs, baseEmbs,
     CASE WHEN size(conceptEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN conceptEmbs | sum + e[i]) / size(conceptEmbs)
       ]
     ELSE null END AS conceptSignal,
     CASE WHEN size(baseEmbs) > 0 THEN
       [i IN range(0, size(h.embedding)-1) |
         reduce(sum = 0.0, e IN baseEmbs | sum + e[i]) / size(baseEmbs)
       ]
     ELSE null END AS baseSignal
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $selfW * coalesce(h.embedding[i], 0) +
      $baseW * coalesce(baseSignal[i], h.embedding[i]) +
      $concW * coalesce(conceptSignal[i], h.embedding[i])
    ],
    h.last_backward_pass = datetime()
RETURN count(h) AS updated`

	const backwardBatchSize = 50
	totalUpdated := 0
	for skip := 0; ; skip += backwardBatchSize {
		updated, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, cypher, map[string]any{
				"spaceId": spaceID,
				"selfW":   s.cfg.HiddenLayerBackwardSelf,
				"baseW":   s.cfg.HiddenLayerBackwardBase,
				"concW":   s.cfg.HiddenLayerBackwardConc,
				"skip":    skip,
				"limit":   backwardBatchSize,
			})
			if err != nil {
				return 0, err
			}
			if res.Next(ctx) {
				rec := res.Record()
				u, _ := rec.Get("updated")
				return asInt(u), res.Err()
			}
			return 0, res.Err()
		})

		if err != nil {
			return nil, fmt.Errorf("backward pass batch at skip=%d: %w", skip, err)
		}
		batch := updated.(int)
		totalUpdated += batch
		if batch == 0 {
			break
		}
	}

	result.HiddenNodesUpdated = totalUpdated
	result.Duration = time.Since(start)
	return result, nil
}

// RunConsolidation performs a full consolidation: clustering + forward + backward pass
// Multi-layer hierarchy: base (L0) → hidden (L1) → concepts (L2, L3, ...)
func (s *Service) RunConsolidation(ctx context.Context, spaceID string) (*ConsolidationResult, error) {
	// Per-space lock: prevent concurrent consolidation from creating duplicate concepts
	muI, _ := s.consolidateLocks.LoadOrStore(spaceID, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	if !mu.TryLock() {
		slog.Warn("consolidation: skipping — already running for space", "space_id", spaceID)
		return &ConsolidationResult{Skipped: true}, nil
	}
	defer mu.Unlock()

	start := time.Now()
	result := &ConsolidationResult{
		ConceptNodesCreated: make(map[int]int),
	}

	// Step 1: Run node-creation steps via pipeline (phase 10-20: hidden, concern, config, comparison, temporal, ui, constraint)
	// HIDDEN-CHURN-001: delegate to the single range source. The hardcoded
	// (10, 20) here silently skipped phase 22 (dynamic_emergence) — with
	// EMERGENCE_ENABLED=true the AUTOMATED consolidation path never ran LLM
	// concept emergence while the manual path did. RunNodeCreationPipeline
	// owns the range (10–22) and the emergence gate.
	phaseStart := time.Now()
	pipelineResult, err := s.RunNodeCreationPipeline(ctx, spaceID, s.cfg.EmergenceEnabled)
	if err != nil {
		return nil, fmt.Errorf("node creation pipeline: %w", err)
	}
	metrics.RecordConsolidationPhase(spaceID, "node_creation", phaseStart)
	result.PipelineSteps = pipelineResult.Steps

	// Map pipeline results back to ConsolidationResult for backward compatibility
	if sr, ok := pipelineResult.Steps["hidden"]; ok {
		result.HiddenNodesCreated = sr.NodesCreated
	}
	if sr, ok := pipelineResult.Steps["constraint"]; ok {
		result.ConstraintNodesResult = &ConstraintNodeResult{
			Created: sr.NodesCreated,
			Updated: sr.NodesUpdated,
			Linked:  sr.EdgesCreated,
		}
		if sr.NodesCreated > 0 || sr.NodesUpdated > 0 {
			fmt.Printf("constraint nodes: %d created, %d updated, %d linked\n",
				sr.NodesCreated, sr.NodesUpdated, sr.EdgesCreated)
		}
	}

	// Log non-fatal pipeline step errors
	for _, stepErr := range pipelineResult.Errors {
		fmt.Printf("warning: failed to run %s step: %s\n", stepErr.Step, stepErr.Message)
	}

	// Step 2: Forward pass (update embeddings up the hierarchy)
	phaseStart = time.Now()
	fwdResult, err := s.ForwardPass(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("forward pass: %w", err)
	}
	metrics.RecordConsolidationPhase(spaceID, "forward_initial", phaseStart)
	result.ForwardPass = fwdResult

	// Guard: skip concept clustering and backward pass if graph is empty
	if fwdResult.HiddenNodesUpdated == 0 && result.HiddenNodesCreated == 0 {
		slog.Warn("consolidation: empty graph, skipping concept clustering", "space_id", spaceID)
		result.EmptyGraph = true
		result.TotalDuration = time.Since(start)
		metrics.Metrics().EmergenceCycleDuration(spaceID, "consolidation").Set(result.TotalDuration.Seconds())
		return result, nil
	}

	// Step 3: Multi-layer concept clustering (L1 → L2, L2 → L3, etc.)
	// Try ALL layers - don't break early. Upper layers have looser constraints
	// and may form clusters even if intermediate layers don't.
	phaseStart = time.Now()
	var repeatedForwardDur time.Duration // the per-layer ForwardPass re-runs (Sprint-B signal)
	maxLayers := 5
	for targetLayer := 2; targetLayer <= maxLayers; targetLayer++ {
		conceptCreated, conceptMerged, err := s.CreateConceptNodes(ctx, spaceID, targetLayer)
		if err != nil {
			return nil, fmt.Errorf("create concept nodes layer %d: %w", targetLayer, err)
		}
		result.ConceptNodesMerged += conceptMerged
		if conceptCreated > 0 {
			result.ConceptNodesCreated[targetLayer] = conceptCreated

			// Run forward pass to update new concept embeddings
			fwdStart := time.Now()
			_, err = s.ForwardPass(ctx, spaceID)
			if err != nil {
				return nil, fmt.Errorf("forward pass after layer %d: %w", targetLayer, err)
			}
			repeatedForwardDur += time.Since(fwdStart)
		}
	}
	metrics.RecordConsolidationPhase(spaceID, "concept_clustering", phaseStart)
	// Sub-signal: how much of the cycle is the per-layer ForwardPass re-runs
	// (the "run ForwardPass once, not 5×" Sprint-B opportunity).
	metrics.Metrics().ConsolidationPhaseDuration(spaceID, "concept_forward_repeats").Set(repeatedForwardDur.Seconds())

	// Step 4: Backward pass (propagate signals down)
	phaseStart = time.Now()
	bwdResult, err := s.BackwardPass(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("backward pass: %w", err)
	}
	metrics.RecordConsolidationPhase(spaceID, "backward", phaseStart)
	result.BackwardPass = bwdResult

	// Step 5: Post-clustering pipeline (phase 25-30: dynamic edges + L5 emergent nodes)
	phaseStart = time.Now()
	postResult, err := s.pipeline.RunPhaseRange(ctx, spaceID, nil, 25, 30)
	if err != nil {
		fmt.Printf("warning: post-clustering pipeline failed: %v\n", err)
	} else {
		// Merge post-clustering steps into pipeline results
		for name, sr := range postResult.Steps {
			result.PipelineSteps[name] = sr
		}
		for _, stepErr := range postResult.Errors {
			fmt.Printf("warning: post-clustering step %s failed: %s\n", stepErr.Step, stepErr.Message)
		}
		if sr, ok := postResult.Steps["dynamic_edges"]; ok {
			result.DynamicEdgesCreated = sr.EdgesCreated
		}
		if sr, ok := postResult.Steps["emergent_l5"]; ok {
			result.L5NodesCreated = sr.NodesCreated
			result.L5GroundingEdgesCreated = sr.EdgesCreated
		}
	}
	metrics.RecordConsolidationPhase(spaceID, "post_clustering", phaseStart)

	// Step 6: Auto-prune excess edges per node (F18: GAP-18)
	// Only runs when both the config flag is set and an EdgePruner has been wired in.
	if s.cfg.LearningAutoPruneExcessEnabled && s.edgePruner != nil {
		phaseStart = time.Now()
		pruned, err := s.edgePruner.PruneExcessEdgesPerNode(ctx, spaceID)
		if err != nil {
			fmt.Printf("warning: auto-prune excess edges failed: %v\n", err)
		} else if pruned > 0 {
			slog.Info("RunConsolidation: auto-pruned excess edges", "count", pruned, "space_id", spaceID)
		}
		metrics.RecordConsolidationPhase(spaceID, "auto_prune", phaseStart)
	}

	result.TotalDuration = time.Since(start)
	metrics.Metrics().EmergenceCycleDuration(spaceID, "consolidation").Set(result.TotalDuration.Seconds())
	return result, nil
}

// CreateConcernNodes creates dedicated nodes for cross-cutting concerns
// based on "concern:*" tags in the base data layer.
// This addresses P1 in the development roadmap - improving retrieval for
// ACL, RBAC, authentication, error-handling, and other cross-cutting patterns.
func (s *Service) CreateConcernNodes(ctx context.Context, spaceID string) (*ConcernNodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ConcernNodeResult{}, nil
	}

	result := &ConcernNodeResult{
		Concerns: make([]string, 0),
	}

	// Step 1: Find all unique concerns from tags
	concerns, err := s.fetchUniqueConcerns(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("fetch unique concerns: %w", err)
	}

	if len(concerns) == 0 {
		return result, nil
	}

	result.Concerns = concerns

	// Step 2: For each concern, create a ConcernNode and edges
	for _, concern := range concerns {
		created, edges, err := s.createConcernNodeWithEdges(ctx, spaceID, concern)
		if err != nil {
			return result, fmt.Errorf("create concern node %s: %w", concern, err)
		}
		if created {
			result.ConcernNodesCreated++
			result.EdgesCreated += edges
		}
	}

	return result, nil
}

// fetchUniqueConcerns finds all unique "concern:*" tags in the space
func (s *Service) fetchUniqueConcerns(ctx context.Context, spaceID string) ([]string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE n.tags IS NOT NULL
UNWIND n.tags AS tag
WITH tag WHERE tag STARTS WITH 'concern:'
RETURN DISTINCT tag AS concern
ORDER BY concern`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var concerns []string
		for res.Next(ctx) {
			rec := res.Record()
			concern, _ := rec.Get("concern")
			if concern != nil {
				concerns = append(concerns, asString(concern))
			}
		}
		return concerns, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// createConcernNodeWithEdges creates a ConcernNode and IMPLEMENTS_CONCERN edges
// Returns (created, edgeCount, error)
func (s *Service) createConcernNodeWithEdges(ctx context.Context, spaceID, concern string) (bool, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Extract concern type from "concern:authentication" -> "authentication"
	concernType := concern
	if len(concern) > 8 && concern[:8] == "concern:" {
		concernType = concern[8:]
	}

	// Check if concern node already exists
	checkCypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'concern', name: $concernName})
RETURN c.node_id AS nodeId`

	existing, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, checkCypher, map[string]any{
			"spaceId":     spaceID,
			"concernName": concern,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			return true, nil
		}
		return false, res.Err()
	})
	if err != nil {
		return false, 0, err
	}
	if existing.(bool) {
		// Already exists, skip
		return false, 0, nil
	}

	// Create the concern node and edges in a single transaction
	// Works with or without embeddings - computes centroid only if embeddings exist
	createCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE $concern IN n.tags
WITH collect(n) AS members,
     [m IN collect(n) WHERE m.embedding IS NOT NULL | m.embedding] AS embeddings
WHERE size(members) > 0
WITH members, embeddings,
     CASE WHEN size(embeddings) > 0 THEN
       [i IN range(0, size(embeddings[0])-1) |
         reduce(sum = 0.0, emb IN embeddings | sum + emb[i]) / size(embeddings)
       ]
     ELSE null END AS centroid
CREATE (c:MemoryNode:Concern {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $concernName,
  concern_type: $concernType,
  layer: 1,
  role_type: 'concern',
  embedding: centroid,
  message_pass_embedding: centroid,
  aggregation_count: size(members),
  stability_score: 1.0,
  summary: 'Cross-cutting concern: ' + $concernType + ' (' + toString(size(members)) + ' implementations)',
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c, members
UNWIND members AS m
CREATE (m)-[:IMPLEMENTS_CONCERN {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  concern_type: $concernType,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS nodeId, size(members) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId":     spaceID,
			"concern":     concern,
			"concernName": concern,
			"concernType": concernType,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, err
	}
	return true, result.(int), nil
}

// CreateConfigNodes creates a dedicated summary node for configuration files
// based on "config" tag in the base data layer.
// This addresses P2 Track 4.3 - improving retrieval for configuration-related queries.
func (s *Service) CreateConfigNodes(ctx context.Context, spaceID string) (*ConfigNodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ConfigNodeResult{}, nil
	}

	result := &ConfigNodeResult{}

	// Check if config node already exists
	exists, err := s.configNodeExists(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("check config node exists: %w", err)
	}
	if exists {
		return result, nil // Already exists
	}

	// Create the config summary node and edges
	created, edges, err := s.createConfigNodeWithEdges(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("create config node: %w", err)
	}

	if created {
		result.ConfigNodeCreated = true
		result.EdgesCreated = edges
	}

	return result, nil
}

// configNodeExists checks if a config summary node already exists
func (s *Service) configNodeExists(ctx context.Context, spaceID string) (bool, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'config'})
RETURN count(c) > 0 AS exists`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			exists, _ := rec.Get("exists")
			return exists.(bool), nil
		}
		return false, res.Err()
	})

	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// createConfigNodeWithEdges creates a ConfigNode and IMPLEMENTS_CONFIG edges
func (s *Service) createConfigNodeWithEdges(ctx context.Context, spaceID string) (bool, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Create the config node and edges in a single transaction
	// Also extracts config file categories for the summary
	createCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE 'config' IN n.tags
WITH collect(n) AS members,
     [m IN collect(n) WHERE m.embedding IS NOT NULL | m.embedding] AS embeddings,
     collect(DISTINCT
       CASE
         WHEN n.path CONTAINS 'docker' THEN 'docker'
         WHEN n.path CONTAINS '.env' THEN 'environment'
         WHEN n.path CONTAINS 'package.json' OR n.path CONTAINS 'tsconfig' THEN 'package'
         WHEN n.path CONTAINS 'config' THEN 'app-config'
         ELSE 'other'
       END
     ) AS categories
WHERE size(members) > 0
WITH members, embeddings, categories,
     CASE WHEN size(embeddings) > 0 THEN
       [i IN range(0, size(embeddings[0])-1) |
         reduce(sum = 0.0, emb IN embeddings | sum + emb[i]) / size(embeddings)
       ]
     ELSE null END AS centroid
CREATE (c:MemoryNode:ConfigPattern {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: 'configuration',
  layer: 1,
  role_type: 'config',
  embedding: centroid,
  message_pass_embedding: centroid,
  aggregation_count: size(members),
  stability_score: 1.0,
  summary: 'Configuration summary: ' + toString(size(members)) + ' config files. Categories: ' +
           reduce(s = '', cat IN categories | s + CASE WHEN s = '' THEN '' ELSE ', ' END + cat),
  tags: ['config', 'configuration-summary'],
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c, members
UNWIND members AS m
CREATE (m)-[:IMPLEMENTS_CONFIG {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS nodeId, size(members) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId": spaceID,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, err
	}
	if result.(int) == 0 {
		return false, 0, nil // No config files found
	}
	return true, result.(int), nil
}

// ConfigNodeResult tracks config node creation
type ConfigNodeResult struct {
	ConfigNodeCreated bool
	EdgesCreated      int
}

// ComparisonNodeResult tracks comparison node creation for similar modules
type ComparisonNodeResult struct {
	ComparisonNodesCreated int
	EdgesCreated           int
	ModulesCompared        int
}

// CreateComparisonNodes creates comparison nodes for similar modules (P2 Track 3)
// This helps answer questions like "What is the purpose of having both X and Y?"
func (s *Service) CreateComparisonNodes(ctx context.Context, spaceID string) (*ComparisonNodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ComparisonNodeResult{}, nil
	}

	result := &ComparisonNodeResult{}

	// Step 1: Find module-like base nodes (files with Module, Service, Controller patterns)
	modules, err := s.fetchModuleNodes(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("fetch module nodes: %w", err)
	}

	if len(modules) < 2 {
		return result, nil // Need at least 2 modules to compare
	}

	// Step 2: Group modules by similarity (name pattern and embedding)
	groups := s.groupSimilarModules(modules)

	// Step 3: Create comparison nodes for each group
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		created, edges, err := s.createComparisonNodeWithEdges(ctx, spaceID, group)
		if err != nil {
			return result, fmt.Errorf("create comparison node: %w", err)
		}
		if created {
			result.ComparisonNodesCreated++
			result.EdgesCreated += edges
			result.ModulesCompared += len(group)
		}
	}

	return result, nil
}

// ModuleNode represents a module-like base node for comparison detection
type ModuleNode struct {
	NodeID    string
	Name      string
	Path      string
	Embedding []float64
}

// fetchModuleNodes retrieves base layer nodes that look like modules/services
func (s *Service) fetchModuleNodes(ctx context.Context, spaceID string) ([]ModuleNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Find nodes whose names suggest they are modules, services, or controllers
	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE n.name IS NOT NULL
  AND (n.name CONTAINS 'Module' OR n.name CONTAINS 'Service' OR n.name CONTAINS 'Controller'
       OR n.name CONTAINS 'Provider' OR n.name CONTAINS 'Handler' OR n.name CONTAINS 'Manager')
  AND n.embedding IS NOT NULL
RETURN n.node_id AS nodeId, n.name AS name, n.path AS path, n.embedding AS embedding
ORDER BY n.name`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var modules []ModuleNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			path, _ := rec.Get("path")
			embedding, _ := rec.Get("embedding")

			modules = append(modules, ModuleNode{
				NodeID:    asString(nodeID),
				Name:      asString(name),
				Path:      asString(path),
				Embedding: asFloat64Slice(embedding),
			})
		}
		return modules, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]ModuleNode), nil
}

// groupSimilarModules groups modules by naming patterns
// Looks for pairs/groups of modules with names differing by a prefix (e.g., SyncModule vs DeltaSyncModule)
func (s *Service) groupSimilarModules(modules []ModuleNode) [][]ModuleNode {
	// Build a map of base names -> modules
	baseToModules := make(map[string][]ModuleNode)
	for _, m := range modules {
		base := extractModuleBaseName(m.Name)
		if len(base) >= 4 {
			baseToModules[base] = append(baseToModules[base], m)
		}
	}

	// Find modules where one name is a suffix of another (e.g., "sync" contained in "deltasync")
	groups := make(map[string][]ModuleNode)
	baseNames := make([]string, 0, len(baseToModules))
	for base := range baseToModules {
		baseNames = append(baseNames, base)
	}

	for i, base1 := range baseNames {
		for j := i + 1; j < len(baseNames); j++ {
			base2 := baseNames[j]
			// Check if one is a suffix of the other (with at least 4 char overlap)
			if len(base1) >= 4 && len(base2) >= 4 {
				if strings.HasSuffix(base2, base1) || strings.HasSuffix(base1, base2) {
					// Create a group key from the shorter base
					key := base1
					if len(base2) < len(base1) {
						key = base2
					}
					// Add modules from both bases
					for _, m := range baseToModules[base1] {
						found := false
						for _, existing := range groups[key] {
							if existing.NodeID == m.NodeID {
								found = true
								break
							}
						}
						if !found {
							groups[key] = append(groups[key], m)
						}
					}
					for _, m := range baseToModules[base2] {
						found := false
						for _, existing := range groups[key] {
							if existing.NodeID == m.NodeID {
								found = true
								break
							}
						}
						if !found {
							groups[key] = append(groups[key], m)
						}
					}
				}
			}
		}
	}

	// Convert to slice and filter to groups with 2-6 members (focused comparisons)
	result := make([][]ModuleNode, 0, len(groups))
	for _, group := range groups {
		if len(group) >= 2 && len(group) <= 6 {
			result = append(result, group)
		}
	}

	return result
}

// extractModuleBaseName extracts the base name from a module name
// e.g., "DeltaSyncModule" -> "sync", "UserService" -> "user"
func extractModuleBaseName(name string) string {
	// Remove common suffixes
	suffixes := []string{"Module", "Service", "Controller", "Provider", "Handler", "Manager"}
	base := name
	for _, suffix := range suffixes {
		if len(base) > len(suffix) && base[len(base)-len(suffix):] == suffix {
			base = base[:len(base)-len(suffix)]
			break
		}
	}
	// Convert to lowercase for comparison
	return strings.ToLower(base)
}

// comparisonNodeExists checks if a comparison node already exists for a group
func (s *Service) comparisonNodeExists(ctx context.Context, spaceID string, moduleIDs []string) (bool, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Check if any comparison node links to ALL modules in this group
	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'comparison'})
WHERE ALL(moduleId IN $moduleIds WHERE
      (c)<-[:COMPARED_IN]-(:MemoryNode {node_id: moduleId}))
RETURN count(c) > 0 AS exists`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":   spaceID,
			"moduleIds": moduleIDs,
		})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			exists, _ := rec.Get("exists")
			return exists.(bool), nil
		}
		return false, res.Err()
	})

	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// createComparisonNodeWithEdges creates a comparison node and COMPARED_IN edges
func (s *Service) createComparisonNodeWithEdges(ctx context.Context, spaceID string, modules []ModuleNode) (bool, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Extract module IDs and names for the query
	moduleIDs := make([]string, len(modules))
	moduleNames := make([]string, len(modules))
	for i, m := range modules {
		moduleIDs[i] = m.NodeID
		moduleNames[i] = m.Name
	}

	// Check if comparison node already exists
	exists, err := s.comparisonNodeExists(ctx, spaceID, moduleIDs)
	if err != nil {
		return false, 0, err
	}
	if exists {
		return false, 0, nil
	}

	// Compute centroid embedding for the comparison node
	var centroid []float64
	count := 0
	for _, m := range modules {
		if len(m.Embedding) > 0 {
			if centroid == nil {
				centroid = make([]float64, len(m.Embedding))
			}
			for i, v := range m.Embedding {
				centroid[i] += v
			}
			count++
		}
	}
	if count > 0 {
		for i := range centroid {
			centroid[i] /= float64(count)
		}
	}

	// Generate comparison name from module names
	compName := "comparison:" + strings.Join(moduleNames[:min(len(moduleNames), 3)], "-vs-")
	if len(moduleNames) > 3 {
		compName += fmt.Sprintf("-and-%d-more", len(moduleNames)-3)
	}

	// Generate rich summary using pattern analysis (Track 3.2)
	summary := generateComparisonSummary(modules)

	// Create comparison node and COMPARED_IN edges
	createCypher := `
CREATE (c:MemoryNode:Comparison {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $compName,
  layer: 1,
  role_type: 'comparison',
  embedding: $centroid,
  message_pass_embedding: $centroid,
  aggregation_count: $moduleCount,
  stability_score: 1.0,
  summary: $summary,
  tags: ['comparison', 'architecture'],
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c
UNWIND $moduleIds AS moduleId
MATCH (m:MemoryNode {space_id: $spaceId, node_id: moduleId})
CREATE (m)-[:COMPARED_IN {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS nodeId, count(m) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId":     spaceID,
			"compName":    compName,
			"centroid":    toFloat32Slice(centroid),
			"moduleCount": len(modules),
			"moduleIds":   moduleIDs,
			"summary":     summary,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, err
	}
	if result.(int) == 0 {
		return false, 0, nil
	}
	return true, result.(int), nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateComparisonSummary creates a rich, pattern-based summary for comparison nodes
// Analyzes module names to identify architectural patterns and relationships
func generateComparisonSummary(modules []ModuleNode) string {
	if len(modules) == 0 {
		return "Empty comparison group"
	}

	// Extract names for analysis
	names := make([]string, len(modules))
	for i, m := range modules {
		names[i] = m.Name
	}

	// Find the common base pattern
	baseName := findCommonBase(names)

	// Identify variant prefixes/suffixes
	variants := identifyVariants(names, baseName)

	// Find common path (domain/subsystem)
	domain := extractDomain(modules)

	// Build the summary
	var summary strings.Builder

	// Lead with the relationship type
	summary.WriteString(fmt.Sprintf("Architectural variants of %s", baseName))

	// Add domain context if found
	if domain != "" {
		summary.WriteString(fmt.Sprintf(" in %s", domain))
	}

	summary.WriteString(". ")

	// Describe the variants
	if len(variants) > 0 {
		summary.WriteString("Includes ")
		for i, v := range variants {
			if i > 0 {
				if i == len(variants)-1 {
					summary.WriteString(" and ")
				} else {
					summary.WriteString(", ")
				}
			}
			summary.WriteString(describeVariant(v))
		}
		summary.WriteString(" implementations.")
	}

	// Add pattern insight
	pattern := detectArchitecturalPattern(variants)
	if pattern != "" {
		summary.WriteString(" ")
		summary.WriteString(pattern)
	}

	return summary.String()
}

// findCommonBase extracts the common base name from a list of module names
func findCommonBase(names []string) string {
	if len(names) == 0 {
		return "module"
	}

	// Find the shortest name as likely base
	shortest := names[0]
	for _, n := range names[1:] {
		clean := cleanModuleName(n)
		if len(clean) < len(cleanModuleName(shortest)) {
			shortest = n
		}
	}

	// Clean and use as base
	base := cleanModuleName(shortest)
	if base == "" {
		return "module"
	}
	return base
}

// cleanModuleName removes common suffixes like Module, Service, Controller
func cleanModuleName(name string) string {
	suffixes := []string{"Module", "Service", "Controller", "Provider", "Handler", "Manager"}
	result := name
	for _, suffix := range suffixes {
		if strings.HasSuffix(result, suffix) {
			result = result[:len(result)-len(suffix)]
			break
		}
	}
	return result
}

// identifyVariants extracts the variant prefixes from module names
func identifyVariants(names []string, baseName string) []string {
	variants := make([]string, 0, len(names))
	baseLower := strings.ToLower(baseName)

	for _, name := range names {
		clean := cleanModuleName(name)
		cleanLower := strings.ToLower(clean)

		// Extract the prefix/variant part
		if cleanLower == baseLower {
			variants = append(variants, "base")
		} else if strings.HasSuffix(cleanLower, baseLower) {
			prefix := clean[:len(clean)-len(baseName)]
			if prefix != "" {
				variants = append(variants, prefix)
			}
		} else {
			// Use the whole name as variant
			variants = append(variants, clean)
		}
	}
	return variants
}

// extractDomain extracts the common domain/subsystem from module paths
func extractDomain(modules []ModuleNode) string {
	if len(modules) == 0 {
		return ""
	}

	// Extract path components
	pathCounts := make(map[string]int)
	for _, m := range modules {
		if m.Path == "" {
			continue
		}
		parts := strings.Split(m.Path, "/")
		for _, part := range parts {
			if part != "" && !strings.HasPrefix(part, ".") {
				pathCounts[part]++
			}
		}
	}

	// Find most common meaningful path component
	var domain string
	maxCount := 0
	skipWords := map[string]bool{
		"src": true, "lib": true, "pkg": true, "internal": true,
		"modules": true, "services": true, "components": true,
	}

	for part, count := range pathCounts {
		if count > maxCount && !skipWords[part] && len(part) > 2 {
			domain = part
			maxCount = count
		}
	}
	return domain
}

// describeVariant returns a human-readable description of a variant type
func describeVariant(variant string) string {
	lowerVariant := strings.ToLower(variant)

	descriptions := map[string]string{
		"base":     "base/standard",
		"delta":    "incremental/delta",
		"full":     "full/complete",
		"partial":  "partial/subset",
		"async":    "asynchronous",
		"sync":     "synchronous",
		"batch":    "batch processing",
		"stream":   "streaming",
		"mock":     "mock/test",
		"stub":     "stub/placeholder",
		"default":  "default",
		"custom":   "custom/specialized",
		"legacy":   "legacy/deprecated",
		"v2":       "version 2",
		"new":      "new/updated",
		"extended": "extended",
		"simple":   "simplified",
		"advanced": "advanced",
		"core":     "core/essential",
		"enhanced": "enhanced",
	}

	if desc, ok := descriptions[lowerVariant]; ok {
		return desc
	}
	return variant
}

// detectArchitecturalPattern identifies common architectural patterns from variants
func detectArchitecturalPattern(variants []string) string {
	variantSet := make(map[string]bool)
	for _, v := range variants {
		variantSet[strings.ToLower(v)] = true
	}

	// Check for specific patterns
	if variantSet["delta"] && (variantSet["full"] || variantSet["base"]) {
		return "Pattern: Delta synchronization with full/incremental modes for efficient data transfer."
	}
	if variantSet["async"] && variantSet["sync"] {
		return "Pattern: Dual sync/async implementations for flexibility in blocking/non-blocking contexts."
	}
	if variantSet["batch"] && (variantSet["stream"] || variantSet["single"]) {
		return "Pattern: Batch and individual processing modes for throughput optimization."
	}
	if variantSet["mock"] || variantSet["stub"] {
		return "Pattern: Includes test doubles for unit testing support."
	}
	if variantSet["legacy"] && (variantSet["base"] || variantSet["new"]) {
		return "Pattern: Migration path with legacy support alongside current implementation."
	}
	if variantSet["simple"] && variantSet["advanced"] {
		return "Pattern: Tiered complexity with simple and advanced implementations."
	}

	return ""
}

// Helper functions for type conversion

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func asInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

func asFloat64Slice(v any) []float64 {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		result := make([]float64, 0, len(arr))
		for _, item := range arr {
			switch n := item.(type) {
			case float64:
				result = append(result, n)
			case int64:
				result = append(result, float64(n))
			}
		}
		return result
	}
	if arr, ok := v.([]float64); ok {
		return arr
	}
	return nil
}

// toFloat32Slice converts []float64 to []float32 for Neo4j vector index compatibility.
// The vector index expects float32 embeddings; hidden layer centroids are computed in
// float64 for arithmetic precision but must be stored as float32.
func toFloat32Slice(f64 []float64) []float32 {
	if f64 == nil {
		return nil
	}
	f32 := make([]float32, len(f64))
	for i, v := range f64 {
		f32[i] = float32(v)
	}
	return f32
}

// =============================================================================
// DYNAMIC EDGE AND NODE TYPE INFERENCE FOR UPPER LAYERS (L4-H4-L5)
// =============================================================================

// InferEdgeType determines the dynamic edge type between two upper-layer nodes
// based on embedding geometry, structural position, and co-activation patterns
func (s *Service) InferEdgeType(source, target UpperLayerNode, coActivation float64) *EdgeInference {
	thresholds := DefaultInferenceThresholds()

	// Calculate metrics
	metrics := EdgeMetrics{
		CosineSimilarity: cosineSimilarity(source.Embedding, target.Embedding),
		CoActivation:     coActivation,
		LayerDistance:    abs(source.Layer - target.Layer),
	}

	// Infer type based on metrics
	var inferredType DynamicEdgeType
	var confidence float64
	var evidence string

	switch {
	// High similarity + same layer = ANALOGOUS_TO
	case metrics.CosineSimilarity >= thresholds.AnalogousMinSim && metrics.LayerDistance == 0:
		inferredType = EdgeAnalogous
		confidence = metrics.CosineSimilarity
		evidence = fmt.Sprintf("High embedding similarity (%.2f) at same layer suggests analogous concepts", metrics.CosineSimilarity)

	// Low similarity + high co-activation = CONTRASTS_WITH (often accessed together but different)
	case metrics.CosineSimilarity <= thresholds.ContrastsMaxSim && metrics.CoActivation >= thresholds.ComposesMinCoact:
		inferredType = EdgeContrasts
		confidence = metrics.CoActivation * (1 - metrics.CosineSimilarity)
		evidence = fmt.Sprintf("Low similarity (%.2f) but high co-activation (%.2f) suggests contrasting approaches", metrics.CosineSimilarity, metrics.CoActivation)

	// High co-activation + moderate similarity = COMPOSES_WITH
	case metrics.CoActivation >= thresholds.ComposesMinCoact && metrics.CosineSimilarity >= 0.4 && metrics.CosineSimilarity < thresholds.AnalogousMinSim:
		inferredType = EdgeComposes
		confidence = metrics.CoActivation
		evidence = fmt.Sprintf("High co-activation (%.2f) with moderate similarity suggests composition", metrics.CoActivation)

	// Cross-layer with moderate similarity = BRIDGES
	case metrics.LayerDistance > 0 && metrics.CosineSimilarity >= 0.4 && metrics.CosineSimilarity < thresholds.AnalogousMinSim:
		inferredType = EdgeBridges
		confidence = metrics.CosineSimilarity * (1.0 + 0.1*float64(metrics.LayerDistance))
		if confidence > 1.0 {
			confidence = 1.0
		}
		evidence = fmt.Sprintf("Cross-layer bridge (layer distance %d, sim %.2f) connects disparate domains", metrics.LayerDistance, metrics.CosineSimilarity)

	// Layer difference with high similarity = SPECIALIZES or GENERALIZES_TO
	case metrics.LayerDistance > 0 && metrics.CosineSimilarity >= 0.7:
		if source.Layer > target.Layer {
			inferredType = EdgeGeneralizes
			evidence = fmt.Sprintf("Higher layer node generalizes lower layer concept (layer %d → %d)", source.Layer, target.Layer)
		} else {
			inferredType = EdgeSpecializes
			evidence = fmt.Sprintf("Lower layer node specializes higher layer concept (layer %d → %d)", source.Layer, target.Layer)
		}
		confidence = metrics.CosineSimilarity

	// Moderate similarity = INFLUENCES (default soft relationship)
	default:
		inferredType = EdgeInfluences
		confidence = 0.5
		evidence = "Default relationship - moderate structural connection"
	}

	return &EdgeInference{
		SourceID:     source.NodeID,
		TargetID:     target.NodeID,
		InferredType: inferredType,
		Confidence:   confidence,
		Evidence:     evidence,
		Metrics:      metrics,
	}
}

// InferNodeType determines the dynamic node type for an upper-layer concept
// based on structural position, connectivity, and embedding stability
func (s *Service) InferNodeType(ctx context.Context, spaceID, nodeID string, layer int) (*NodeInference, error) {
	thresholds := DefaultInferenceThresholds()

	// Fetch node metrics from graph
	metrics, err := s.fetchNodeMetrics(ctx, spaceID, nodeID)
	if err != nil {
		return nil, fmt.Errorf("fetch node metrics: %w", err)
	}

	// Infer type based on metrics
	var inferredType DynamicNodeType
	var confidence float64
	var evidence string

	totalDegree := metrics.InDegree + metrics.OutDegree

	switch {
	// High cross-domain links = BRIDGE
	case metrics.CrossDomainLinks >= thresholds.BridgeMinDomains:
		inferredType = NodeBridge
		confidence = float64(metrics.CrossDomainLinks) / float64(maxInt(totalDegree, 1))
		evidence = fmt.Sprintf("Connects %d distinct domains - acts as a bridge concept", metrics.CrossDomainLinks)

	// High degree = HUB
	case totalDegree >= thresholds.HubMinDegree:
		inferredType = NodeHub
		confidence = minFloat(1.0, float64(totalDegree)/float64(thresholds.HubMinDegree*2))
		evidence = fmt.Sprintf("High connectivity (degree %d) - central hub concept", totalDegree)

	// High stability + high aggregation = PRINCIPLE
	case metrics.StabilityScore >= thresholds.EstablishedMinStab && metrics.AggregationDepth >= 2:
		inferredType = NodePrinciple
		confidence = metrics.StabilityScore
		evidence = fmt.Sprintf("Stable (%.2f) with deep aggregation (%d layers) - guiding principle", metrics.StabilityScore, metrics.AggregationDepth)

	// High stability = ESTABLISHED
	case metrics.StabilityScore >= thresholds.EstablishedMinStab:
		inferredType = NodeEstablished
		confidence = metrics.StabilityScore
		evidence = fmt.Sprintf("High embedding stability (%.2f) - established concept", metrics.StabilityScore)

	// Multiple children with diversity = PATTERN
	case metrics.InDegree >= 3 && metrics.ChildDiversity >= 0.5:
		inferredType = NodePattern
		confidence = metrics.ChildDiversity
		evidence = fmt.Sprintf("Diverse children (%.2f diversity) suggest recurring pattern", metrics.ChildDiversity)

	// Low stability = EMERGENT
	case metrics.StabilityScore < 0.5:
		inferredType = NodeEmergent
		confidence = 1.0 - metrics.StabilityScore
		evidence = fmt.Sprintf("Low stability (%.2f) - newly emergent concept", metrics.StabilityScore)

	// Default = PATTERN
	default:
		inferredType = NodePattern
		confidence = 0.5
		evidence = "Default classification as architectural pattern"
	}

	return &NodeInference{
		NodeID:       nodeID,
		InferredType: inferredType,
		Confidence:   confidence,
		Evidence:     evidence,
		Metrics:      *metrics,
	}, nil
}

// fetchNodeMetrics retrieves structural metrics for a node from the graph
func (s *Service) fetchNodeMetrics(ctx context.Context, spaceID, nodeID string) (*NodeMetrics, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
OPTIONAL MATCH (n)<-[inEdge]-()
OPTIONAL MATCH (n)-[outEdge]->()
OPTIONAL MATCH (n)<-[:ABSTRACTS_TO*1..3]-(child:MemoryNode)
WITH n,
     count(DISTINCT inEdge) AS inDegree,
     count(DISTINCT outEdge) AS outDegree,
     count(DISTINCT child) AS childCount,
     collect(DISTINCT split(child.path, '/')[0]) AS childDomains
RETURN inDegree, outDegree, childCount,
       size(childDomains) AS crossDomainLinks,
       COALESCE(n.stability_score, 0.5) AS stabilityScore,
       n.layer AS layer`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			inDegree, _ := rec.Get("inDegree")
			outDegree, _ := rec.Get("outDegree")
			childCount, _ := rec.Get("childCount")
			crossDomainLinks, _ := rec.Get("crossDomainLinks")
			stabilityScore, _ := rec.Get("stabilityScore")
			layer, _ := rec.Get("layer")

			// Calculate child diversity (unique types / total children)
			childDiversity := 0.0
			if asInt(childCount) > 0 {
				childDiversity = float64(asInt(crossDomainLinks)) / float64(asInt(childCount))
				if childDiversity > 1.0 {
					childDiversity = 1.0
				}
			}

			return &NodeMetrics{
				InDegree:         asInt(inDegree),
				OutDegree:        asInt(outDegree),
				CrossDomainLinks: asInt(crossDomainLinks),
				StabilityScore:   asFloat64(stabilityScore),
				AggregationDepth: asInt(layer),
				ChildDiversity:   childDiversity,
			}, nil
		}
		return nil, res.Err()
	})

	if err != nil {
		return nil, err
	}
	if result == nil {
		return &NodeMetrics{StabilityScore: 0.5}, nil
	}
	return result.(*NodeMetrics), nil
}

// ClassifyUpperLayerNodes classifies all nodes at L4+ with dynamic types
func (s *Service) ClassifyUpperLayerNodes(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Fetch all L4+ nodes without classification
	fetchCypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 4 AND (n.node_type IS NULL OR n.node_type = '')
RETURN n.node_id AS nodeId, n.layer AS layer
LIMIT 100`

	nodes, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, fetchCypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var nodes []struct {
			NodeID string
			Layer  int
		}
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			nodes = append(nodes, struct {
				NodeID string
				Layer  int
			}{asString(nodeID), asInt(layer)})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return 0, err
	}

	nodeList := nodes.([]struct {
		NodeID string
		Layer  int
	})

	classified := 0
	for _, node := range nodeList {
		inference, err := s.InferNodeType(ctx, spaceID, node.NodeID, node.Layer)
		if err != nil {
			continue // Skip on error, don't fail entire operation
		}

		// Update node with inferred type
		updateCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
SET n.node_type = $nodeType,
    n.type_confidence = $confidence,
    n.type_evidence = $evidence,
    n.type_inferred_at = datetime()
RETURN n.node_id`

		_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, updateCypher, map[string]any{
				"spaceId":    spaceID,
				"nodeId":     node.NodeID,
				"nodeType":   string(inference.InferredType),
				"confidence": inference.Confidence,
				"evidence":   inference.Evidence,
			})
			return nil, err
		})

		if err == nil {
			classified++
		}
	}

	return classified, nil
}

// CreateDynamicEdges creates edges with inferred types for upper layer relationships
func (s *Service) CreateDynamicEdges(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// RETRIEVAL-TYPED-EDGES-002: connect semantically-similar concept nodes.
	// MinLayer default 1 (not the old L5SourceMinLayer=3) — a fresh corpus has
	// almost no L3+ concepts, so semantic edges must reach the abundant L1/L2
	// layers for the population to grow.
	minLayer := s.cfg.DynamicEdgeMinLayer
	if minLayer < 1 {
		minLayer = 1
	}
	topK := s.cfg.DynamicEdgeTopK
	if topK <= 0 {
		topK = 10
	}
	oversample := s.cfg.DynamicEdgeOversample
	if oversample <= 0 {
		oversample = 8
	}
	simThreshold := s.cfg.DynamicEdgeSimThreshold
	if simThreshold <= 0 {
		simThreshold = 0.30
	}

	// RETRIEVAL-TYPED-EDGES-002: the O(n²) Cartesian cross-join (which the
	// CONSOLIDATE-PERF-001 circuit-breaker had to skip on large graphs) is
	// replaced by a per-node top-K query via the memNodeEmbedding vector index
	// (O(n·logn)). For each L≥minLayer node, fetch its top (K×oversample) nearest
	// neighbours from the index, filter to same-space ∧ L≥minLayer ∧ not-already-
	// connected ∧ under the degree cap ∧ sim ≥ threshold, keep the best K. The
	// oversample absorbs L0 crowding the global top-K. No circuit-breaker needed
	// (the fan-out is per-node bounded).
	// The degree cap counts only the DYNAMIC semantic-edge types (not the
	// structural GENERALIZES/ABSTRACTS_TO membership edges) — else L1/L2 concept
	// nodes, which have many member edges, would always exceed the cap and be
	// excluded, defeating the whole point of connecting the concept layers.
	findPairsCypher := `
MATCH (a:MemoryNode {space_id: $spaceId})
WHERE a.layer >= $minLayer AND a.embedding IS NOT NULL
  AND COUNT { (a)-[:ANALOGOUS_TO|CONTRASTS_WITH|COMPOSES_WITH|INFLUENCES|SPECIALIZES|GENERALIZES_TO|BRIDGES]-() } < $degreeCap
CALL db.index.vector.queryNodes('memNodeEmbedding', $fetchK, a.embedding) YIELD node AS b, score
WITH a, b, score
WHERE b.space_id = $spaceId AND b.layer >= $minLayer
  AND a.node_id < b.node_id
  AND b.embedding IS NOT NULL
  AND COUNT { (b)-[:ANALOGOUS_TO|CONTRASTS_WITH|COMPOSES_WITH|INFLUENCES|SPECIALIZES|GENERALIZES_TO|BRIDGES]-() } < $degreeCap
  AND NOT (a)-[:ANALOGOUS_TO|CONTRASTS_WITH|COMPOSES_WITH|INFLUENCES|SPECIALIZES|GENERALIZES_TO|BRIDGES]-(b)
  AND score >= $simThreshold
WITH a, b, score
ORDER BY score DESC
WITH a, collect({b: b, sim: score})[0..$topK] AS tops
UNWIND tops AS t
WITH a, t.b AS b, t.sim AS sim
RETURN a.node_id AS sourceId, b.node_id AS targetId,
       a.embedding AS sourceEmb, b.embedding AS targetEmb,
       a.layer AS sourceLayer, b.layer AS targetLayer,
       a.name AS sourceName, b.name AS targetName,
       sim
LIMIT $maxEdges`

	pairs, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, findPairsCypher, map[string]any{
			"spaceId":      spaceID,
			"degreeCap":    s.cfg.DynamicEdgeDegreeCap,
			"minLayer":     minLayer,
			"fetchK":       topK * oversample,
			"topK":         topK,
			"simThreshold": simThreshold,
			"maxEdges":     500000, // global safety cap; the per-node topK is the real bound
		})
		if err != nil {
			return nil, err
		}
		var pairs []struct {
			Source UpperLayerNode
			Target UpperLayerNode
			Sim    float64
		}
		for res.Next(ctx) {
			rec := res.Record()
			sourceId, _ := rec.Get("sourceId")
			targetId, _ := rec.Get("targetId")
			sourceEmb, _ := rec.Get("sourceEmb")
			targetEmb, _ := rec.Get("targetEmb")
			sourceLayer, _ := rec.Get("sourceLayer")
			targetLayer, _ := rec.Get("targetLayer")
			sourceName, _ := rec.Get("sourceName")
			targetName, _ := rec.Get("targetName")
			sim, _ := rec.Get("sim")

			pairs = append(pairs, struct {
				Source UpperLayerNode
				Target UpperLayerNode
				Sim    float64
			}{
				Source: UpperLayerNode{
					NodeID:    asString(sourceId),
					Layer:     asInt(sourceLayer),
					Name:      asString(sourceName),
					Embedding: asFloat64Slice(sourceEmb),
				},
				Target: UpperLayerNode{
					NodeID:    asString(targetId),
					Layer:     asInt(targetLayer),
					Name:      asString(targetName),
					Embedding: asFloat64Slice(targetEmb),
				},
				Sim: asFloat64(sim),
			})
		}
		return pairs, res.Err()
	})

	if err != nil {
		return 0, err
	}

	pairList := pairs.([]struct {
		Source UpperLayerNode
		Target UpperLayerNode
		Sim    float64
	})

	created := 0
	// Group inferences by type for batched edge creation
	grouped := make(map[string][]map[string]any)
	for _, pair := range pairList {
		inference := s.InferEdgeType(pair.Source, pair.Target, 0.0) // Pass 0 — no real Hebbian co-activation data available

		// Skip low-confidence inferences
		if inference.Confidence < s.cfg.DynamicEdgeMinConfidence {
			continue
		}

		edgeType := string(inference.InferredType)
		grouped[edgeType] = append(grouped[edgeType], map[string]any{
			"sourceId":   inference.SourceID,
			"targetId":   inference.TargetID,
			"confidence": inference.Confidence,
			"evidence":   inference.Evidence,
			"spaceId":    spaceID,
			// CUIDv2 minted in Go — Cypher cannot mint CUIDv2 (the project
			// identifier standard; randomUUID() was a CUIDv2-rule violation).
			"edgeId": cuid2.Generate(),
		})
	}

	// Create edges per type using proper Neo4j relationship types (no APOC)
	for edgeType, records := range grouped {
		cypher := fmt.Sprintf(`
UNWIND $rels AS rel
MATCH (a:MemoryNode {space_id: rel.spaceId, node_id: rel.sourceId})
MATCH (b:MemoryNode {space_id: rel.spaceId, node_id: rel.targetId})
MERGE (a)-[r:%s {space_id: rel.spaceId}]->(b)
ON CREATE SET
    r.edge_id = rel.edgeId,
    r.weight = rel.confidence,
    r.confidence = rel.confidence,
    r.evidence = rel.evidence,
    r.created_at = datetime(),
    r.updated_at = datetime(),
    r.inferred_at = datetime(),
    r.evidence_count = 1,
    r.version = 1,
    r.status = 'active'
ON MATCH SET
    r.updated_at = datetime(),
    r.evidence_count = r.evidence_count + 1,
    r.confidence = CASE WHEN rel.confidence > r.confidence THEN rel.confidence ELSE r.confidence END
`, edgeType)

		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, cypher, map[string]any{"rels": records})
			if err != nil {
				return nil, err
			}
			summary, _ := res.Consume(ctx)
			if summary != nil {
				created += summary.Counters().RelationshipsCreated()
			}
			return nil, nil
		})
		if err != nil {
			fmt.Printf("warning: failed to create %s edges: %v\n", edgeType, err)
		}
	}

	return created, nil
}

// CreateL5EmergentNodes creates L5 emergent concepts from L4 nodes
// connected by high-evidence ANALOGOUS_TO or BRIDGES edges.
// L5 represents meta-patterns spanning multiple L4 domains.
// Returns (nodesCreated, groundingEdgesCreated, error).
func (s *Service) CreateL5EmergentNodes(ctx context.Context, spaceID string, namer *EmergenceNamer) (int, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	minEvidence := s.cfg.L5BridgeEvidenceMin
	if minEvidence < 1 {
		minEvidence = 1
	}
	minSourceLayer := s.cfg.L5SourceMinLayer
	if minSourceLayer < 1 {
		minSourceLayer = 3
	}

	// Find clusters of L3+ nodes connected by high-evidence qualifying edges
	clusterCypher := `
MATCH (a:MemoryNode {space_id: $spaceId})-[r:ANALOGOUS_TO|BRIDGES|COMPOSES_WITH]->(b:MemoryNode {space_id: $spaceId})
WHERE a.layer >= $minSourceLayer AND b.layer >= $minSourceLayer
  AND r.evidence_count >= $minEvidence
  AND NOT EXISTS {
    MATCH (a)-[:ABSTRACTS_TO]->(l5:MemoryNode {space_id: $spaceId, layer: 5})
    WHERE (b)-[:ABSTRACTS_TO]->(l5)
  }
RETURN a.node_id AS sourceId, b.node_id AS targetId,
       a.name AS sourceName, b.name AS targetName,
       a.embedding AS sourceEmb, b.embedding AS targetEmb,
       r.evidence_count AS evidence, type(r) AS relType
ORDER BY evidence DESC
LIMIT 20`

	type l5Pair struct {
		SourceID, TargetID     string
		SourceName, TargetName string
		SourceEmb, TargetEmb   []float64
		Evidence               int
		RelType                string
	}

	pairs, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, clusterCypher, map[string]any{
			"spaceId":        spaceID,
			"minEvidence":    minEvidence,
			"minSourceLayer": minSourceLayer,
		})
		if err != nil {
			return nil, err
		}
		var pairs []l5Pair
		for res.Next(ctx) {
			rec := res.Record()
			p := l5Pair{
				SourceID:   asString(rec.Values[0]),
				TargetID:   asString(rec.Values[1]),
				SourceName: asString(rec.Values[2]),
				TargetName: asString(rec.Values[3]),
				Evidence:   asInt(rec.Values[6]),
				RelType:    asString(rec.Values[7]),
			}
			if emb := rec.Values[4]; emb != nil {
				p.SourceEmb = asFloat64Slice(emb)
			}
			if emb := rec.Values[5]; emb != nil {
				p.TargetEmb = asFloat64Slice(emb)
			}
			pairs = append(pairs, p)
		}
		return pairs, res.Err()
	})
	if err != nil {
		return 0, 0, fmt.Errorf("L5 cluster query: %w", err)
	}

	pairList := pairs.([]l5Pair)
	if len(pairList) == 0 {
		return 0, 0, nil
	}

	// Group connected components (simple union-find)
	parent := make(map[string]string)
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" || parent[x] == x {
			parent[x] = x
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	union := func(a, b string) {
		fa, fb := find(a), find(b)
		if fa != fb {
			parent[fa] = fb
		}
	}

	nameMap := make(map[string]string)
	for _, p := range pairList {
		union(p.SourceID, p.TargetID)
		nameMap[p.SourceID] = p.SourceName
		nameMap[p.TargetID] = p.TargetName
	}

	// Build clusters
	clusters := make(map[string][]string)
	for id := range nameMap {
		root := find(id)
		clusters[root] = append(clusters[root], id)
	}

	created := 0
	totalGroundingEdges := 0
	for _, members := range clusters {
		if len(members) < 2 {
			continue
		}

		// Collect member names and build ClusterNodeSummary for LLM naming
		names := make([]string, 0, len(members))
		clusterNodes := make([]ClusterNodeSummary, 0, len(members))
		for _, m := range members {
			if nm, ok := nameMap[m]; ok && nm != "" {
				names = append(names, nm)
				clusterNodes = append(clusterNodes, ClusterNodeSummary{
					NodeID: m,
					Name:   nm,
					Layer:  5, // These are L3+ nodes being unified into L5
				})
			}
		}

		// Try LLM naming first; fall back to mechanical name
		var l5Name, l5Summary, l5Label string
		if namer != nil {
			result, err := namer.NameL5Concept(ctx, clusterNodes)
			if err != nil {
				fmt.Printf("L5 LLM naming failed (falling back to mechanical): %v\n", err)
			} else {
				l5Name = result.Name
				l5Summary = result.Description
				l5Label = result.ProposedLabel
			}
		}

		// Fallback: mechanical intersection name
		if l5Name == "" {
			l5Name = "Emergent: " + strings.Join(names, " ∩ ")
			if len(l5Name) > 200 {
				l5Name = l5Name[:200]
			}
		}

		// Create L5 node + ABSTRACTS_TO edges from members
		createCypher := `
WITH $members AS memberIds, $name AS l5Name, $spaceId AS sid,
     $summary AS l5Summary, $label AS l5Label
CREATE (l5:MemoryNode:EmergentConcept {
    node_id: randomUUID(),
    space_id: sid,
    name: l5Name,
    summary: CASE WHEN l5Summary <> '' THEN l5Summary ELSE null END,
    proposed_label: CASE WHEN l5Label <> '' THEN l5Label ELSE null END,
    layer: 5,
    role_type: 'emergent_concept',
    created_at: datetime(),
    updated_at: datetime(),
    version: 1,
    status: 'active'
})
WITH l5, memberIds
UNWIND memberIds AS mid
MATCH (m:MemoryNode {space_id: l5.space_id, node_id: mid})
CREATE (m)-[:ABSTRACTS_TO {
    space_id: l5.space_id,
    edge_id: randomUUID(),
    created_at: datetime(),
    updated_at: datetime(),
    weight: 1.0,
    evidence_count: 1,
    version: 1,
    status: 'active'
}]->(l5)
RETURN l5.node_id AS l5Id`

		l5NodeID, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, createCypher, map[string]any{
				"members": members,
				"name":    l5Name,
				"summary": l5Summary,
				"label":   l5Label,
				"spaceId": spaceID,
			})
			if err != nil {
				return "", err
			}
			// The query returns one row per ABSTRACTS_TO edge; grab l5Id from the first
			if res.Next(ctx) {
				id, _ := res.Record().Get("l5Id")
				if s, ok := id.(string); ok {
					return s, nil
				}
			}
			return "", res.Err()
		})
		if err != nil {
			fmt.Printf("warning: failed to create L5 node: %v\n", err)
			continue
		}
		created++

		// ANN Optimization Phase C: Create GROUNDED_BY edges from L5 to most representative L0 observations
		if l5ID, ok := l5NodeID.(string); ok && l5ID != "" {
			groundingCount, groundingErr := s.createGroundingEdges(ctx, spaceID, l5ID, members)
			if groundingErr != nil {
				fmt.Printf("warning: failed to create grounding edges for L5 %s: %v\n", l5ID, groundingErr)
			} else {
				totalGroundingEdges += groundingCount
			}
		}
	}

	return created, totalGroundingEdges, nil
}

// createGroundingEdges creates GROUNDED_BY edges from an L5 node to the most
// representative L0 observations within its cluster's descendant tree.
// This implements L0 Skip Connections (ANN Optimization Phase C), allowing
// high-level concepts to maintain direct grounding to source observations.
func (s *Service) createGroundingEdges(ctx context.Context, spaceID string, l5NodeID string, memberNodeIDs []string) (int, error) {
	if s.cfg.L5GroundingMaxEdges <= 0 {
		return 0, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Find L0 descendants of the L5 node's cluster members via ABSTRACTS_TO chain,
	// then pick the ones with embeddings most similar to the L5 node's embedding.
	// Scoped to cluster descendants to avoid full-space scan.
	cypher := `
MATCH (l5:MemoryNode {node_id: $l5NodeId, space_id: $spaceId})
WITH l5
UNWIND $memberIds AS memberId
MATCH (member:MemoryNode {node_id: memberId, space_id: $spaceId})
// Traverse ABSTRACTS_TO chain down to L0
MATCH (member)-[:ABSTRACTS_TO*0..5]->(l0:MemoryNode {space_id: $spaceId})
WHERE l0.layer = 0 AND l0.embedding IS NOT NULL AND l5.embedding IS NOT NULL
WITH DISTINCT l5, l0,
     vector.similarity.cosine(l5.embedding, l0.embedding) AS sim
WHERE sim >= $minSim
ORDER BY sim DESC
LIMIT $maxEdges
MERGE (l5)-[r:GROUNDED_BY {space_id: $spaceId}]->(l0)
ON CREATE SET r.weight = $initialWeight, r.similarity = sim,
              r.evidence_count = 1, r.created_at = datetime()
ON MATCH SET r.evidence_count = r.evidence_count + 1,
             r.similarity = sim, r.updated_at = datetime()
RETURN count(*) AS created`

	params := map[string]any{
		"spaceId":       spaceID,
		"l5NodeId":      l5NodeID,
		"memberIds":     memberNodeIDs,
		"minSim":        s.cfg.L5GroundingMinSim,
		"maxEdges":      s.cfg.L5GroundingMaxEdges,
		"initialWeight": s.cfg.L5GroundingInitialWeight,
	}

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			count, _ := res.Record().Get("created")
			if c, ok := count.(int64); ok {
				return int(c), nil
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// CreateDynamicEmergentNodes discovers dense CO_ACTIVATED_WITH clusters that don't match
// any hardcoded pattern and names them via LLM. This is the core of Phase 103: Dynamic Emergence.
func (s *Service) CreateDynamicEmergentNodes(ctx context.Context, spaceID string, namer *EmergenceNamer) (int, error) {
	if namer == nil {
		return 0, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	minWeight := s.cfg.EmergenceMinWeight
	if minWeight <= 0 {
		minWeight = 0.3
	}

	// Step 1: Query CO_ACTIVATED_WITH edges between L0-L4 nodes that:
	// - Have weight >= minWeight
	// - Don't already have a role_type (i.e., not classified by hardcoded patterns)
	// - Don't already share a dynamic_emergent parent (idempotency)
	clusterCypher := `
MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH]->(b:MemoryNode {space_id: $spaceId})
WHERE a.layer >= 0 AND a.layer <= 4
  AND b.layer >= 0 AND b.layer <= 4
  AND r.weight >= $minWeight
  AND NOT a.role_type IN ['concern','config','comparison','temporal','ui','constraint','dynamic_emergent']
  AND NOT b.role_type IN ['concern','config','comparison','temporal','ui','constraint','dynamic_emergent']
  AND NOT EXISTS {
    MATCH (a)-[:ABSTRACTS_TO]->(ec:MemoryNode {space_id: $spaceId, role_type: 'dynamic_emergent'})
    WHERE (b)-[:ABSTRACTS_TO]->(ec)
  }
RETURN a.node_id AS sourceId, b.node_id AS targetId,
       a.name AS sourceName, b.name AS targetName,
       a.summary AS sourceSummary, b.summary AS targetSummary,
       a.path AS sourcePath, b.path AS targetPath,
       a.layer AS sourceLayer, b.layer AS targetLayer,
       r.weight AS weight
ORDER BY weight DESC
LIMIT 200`

	type edgePair struct {
		SourceID, TargetID           string
		SourceName, TargetName       string
		SourceSummary, TargetSummary string
		SourcePath, TargetPath       string
		SourceLayer, TargetLayer     int
		Weight                       float64
	}

	pairs, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, clusterCypher, map[string]any{
			"spaceId":   spaceID,
			"minWeight": minWeight,
		})
		if err != nil {
			return nil, err
		}
		var pairs []edgePair
		for res.Next(ctx) {
			rec := res.Record()
			p := edgePair{
				SourceID:      asString(rec.Values[0]),
				TargetID:      asString(rec.Values[1]),
				SourceName:    asString(rec.Values[2]),
				TargetName:    asString(rec.Values[3]),
				SourceSummary: asString(rec.Values[4]),
				TargetSummary: asString(rec.Values[5]),
				SourcePath:    asString(rec.Values[6]),
				TargetPath:    asString(rec.Values[7]),
				SourceLayer:   asInt(rec.Values[8]),
				TargetLayer:   asInt(rec.Values[9]),
				Weight:        asFloat64(rec.Values[10]),
			}
			pairs = append(pairs, p)
		}
		return pairs, res.Err()
	})
	if err != nil {
		return 0, fmt.Errorf("dynamic emergence cluster query: %w", err)
	}

	pairList := pairs.([]edgePair)
	if len(pairList) == 0 {
		return 0, nil
	}

	// Step 2: Union-find clustering (same pattern as CreateL5EmergentNodes)
	parent := make(map[string]string)
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" || parent[x] == x {
			parent[x] = x
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	union := func(a, b string) {
		fa, fb := find(a), find(b)
		if fa != fb {
			parent[fa] = fb
		}
	}

	// Build node info map and cluster weights
	type nodeInfo struct {
		Name, Summary, Path string
		Layer               int
	}
	nodeMap := make(map[string]nodeInfo)
	clusterWeight := make(map[string]float64) // root -> total weight

	for _, p := range pairList {
		union(p.SourceID, p.TargetID)
		nodeMap[p.SourceID] = nodeInfo{p.SourceName, p.SourceSummary, p.SourcePath, p.SourceLayer}
		nodeMap[p.TargetID] = nodeInfo{p.TargetName, p.TargetSummary, p.TargetPath, p.TargetLayer}
	}

	// Accumulate weights per cluster
	for _, p := range pairList {
		root := find(p.SourceID)
		clusterWeight[root] += p.Weight
	}

	// Build clusters
	clusters := make(map[string][]string)
	for id := range nodeMap {
		root := find(id)
		clusters[root] = append(clusters[root], id)
	}

	// Step 3: Filter by min size and sort by total weight (densest first)
	minClusterSize := s.cfg.EmergenceMinClusterSize
	if minClusterSize < 2 {
		minClusterSize = 3
	}
	maxClusters := s.cfg.EmergenceMaxClusters
	if maxClusters < 1 {
		maxClusters = 10
	}

	type rankedCluster struct {
		root    string
		members []string
		weight  float64
	}
	var ranked []rankedCluster
	for root, members := range clusters {
		if len(members) < minClusterSize {
			continue
		}
		ranked = append(ranked, rankedCluster{root: root, members: members, weight: clusterWeight[root]})
	}

	// Sort by weight descending
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].weight > ranked[i].weight {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}

	// Cap at maxClusters
	if len(ranked) > maxClusters {
		ranked = ranked[:maxClusters]
	}

	// Step 4: For each cluster, build node summaries and call LLM
	created := 0
	for _, rc := range ranked {
		nodes := make([]ClusterNodeSummary, 0, len(rc.members))
		maxLayer := 0
		for _, id := range rc.members {
			info := nodeMap[id]
			nodes = append(nodes, ClusterNodeSummary{
				NodeID:  id,
				Name:    info.Name,
				Summary: info.Summary,
				Path:    info.Path,
				Layer:   info.Layer,
			})
			if info.Layer > maxLayer {
				maxLayer = info.Layer
			}
		}

		// Create EmergentConcept node at layer = max(member.layer) + 1
		targetLayer := maxLayer + 1

		// Call LLM for naming — use L5-specific meta-concept prompt for top-layer nodes
		var result *EmergenceNamingResult
		if targetLayer >= 5 {
			result, err = namer.NameL5Concept(ctx, nodes)
		} else {
			result, err = namer.Name(ctx, nodes)
		}
		if err != nil {
			// Fail-open: skip this cluster, continue with next
			fmt.Printf("warning: emergence namer failed for cluster (root=%s, %d members): %v\n",
				rc.root, len(rc.members), err)
			continue
		}
		memberIDs := rc.members

		createCypher := `
WITH $members AS memberIds, $name AS nodeName, $description AS nodeDesc,
     $label AS nodeLabel, $spaceId AS sid, $layer AS targetLayer
CREATE (ec:MemoryNode:EmergentConcept {
    node_id: randomUUID(),
    space_id: sid,
    name: nodeName,
    summary: nodeDesc,
    layer: targetLayer,
    role_type: 'dynamic_emergent',
    proposed_label: nodeLabel,
    created_at: datetime(),
    updated_at: datetime(),
    version: 1,
    status: 'active'
})
WITH ec, memberIds
UNWIND memberIds AS mid
MATCH (m:MemoryNode {space_id: ec.space_id, node_id: mid})
CREATE (m)-[:ABSTRACTS_TO {
    space_id: ec.space_id,
    edge_id: randomUUID(),
    created_at: datetime(),
    updated_at: datetime(),
    weight: 1.0,
    evidence_count: 1,
    version: 1,
    status: 'active'
}]->(ec)
RETURN ec.node_id AS ecId`

		_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, createCypher, map[string]any{
				"members":     memberIDs,
				"name":        result.Name,
				"description": result.Description,
				"label":       result.ProposedLabel,
				"spaceId":     spaceID,
				"layer":       targetLayer,
			})
			return nil, err
		})
		if err != nil {
			fmt.Printf("warning: failed to create dynamic emergent node: %v\n", err)
			continue
		}
		created++
	}

	return created, nil
}

// Helper functions for inference

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func asFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

// =============================================================================
// TRACK 5: TEMPORAL PATTERN DETECTION
// =============================================================================

// CreateTemporalNodes creates a dedicated summary node for temporal patterns
// based on "concern:temporal" tag in the base data layer.
// This addresses P3 Track 5 - improving retrieval for temporal modeling patterns
// like validFrom/validTo, soft deletes, date ranges, and versioning.
func (s *Service) CreateTemporalNodes(ctx context.Context, spaceID string) (*TemporalNodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &TemporalNodeResult{}, nil
	}

	result := &TemporalNodeResult{}

	// Check if temporal node already exists
	exists, err := s.temporalNodeExists(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("check temporal node exists: %w", err)
	}
	if exists {
		return result, nil // Already exists
	}

	// Detect temporal patterns and create the node
	created, edges, patterns, err := s.createTemporalNodeWithEdges(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("create temporal node: %w", err)
	}

	if created {
		result.TemporalNodeCreated = true
		result.EdgesCreated = edges
		result.PatternsDetected = patterns
	}

	return result, nil
}

// temporalNodeExists checks if a temporal summary node already exists
func (s *Service) temporalNodeExists(ctx context.Context, spaceID string) (bool, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (t:MemoryNode {space_id: $spaceId, role_type: 'temporal'})
RETURN count(t) > 0 AS exists`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			exists, _ := rec.Get("exists")
			return exists.(bool), nil
		}
		return false, res.Err()
	})

	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// createTemporalNodeWithEdges creates a temporal pattern node and SHARES_TEMPORAL_PATTERN edges
func (s *Service) createTemporalNodeWithEdges(ctx context.Context, spaceID string) (bool, int, []string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// First, analyze what temporal patterns are present
	patterns, err := s.detectTemporalPatterns(ctx, spaceID)
	if err != nil {
		return false, 0, nil, fmt.Errorf("detect temporal patterns: %w", err)
	}

	if len(patterns) == 0 {
		return false, 0, nil, nil // No temporal patterns found
	}

	// Create the temporal node and edges
	// Works with nodes tagged with "concern:temporal"
	createCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE 'concern:temporal' IN n.tags
WITH collect(n) AS members,
     [m IN collect(n) WHERE m.embedding IS NOT NULL | m.embedding] AS embeddings,
     collect(DISTINCT
       CASE
         WHEN toLower(n.name) CONTAINS 'valid' THEN 'validity-period'
         WHEN toLower(n.name) CONTAINS 'effective' OR toLower(n.name) CONTAINS 'expir' THEN 'effective-dates'
         WHEN toLower(n.name) CONTAINS 'delete' OR toLower(n.name) CONTAINS 'soft' THEN 'soft-delete'
         WHEN toLower(n.name) CONTAINS 'version' OR toLower(n.name) CONTAINS 'snapshot' THEN 'versioning'
         WHEN toLower(n.name) CONTAINS 'history' OR toLower(n.name) CONTAINS 'audit' THEN 'audit-history'
         WHEN toLower(n.name) CONTAINS 'range' OR toLower(n.name) CONTAINS 'period' THEN 'date-ranges'
         ELSE 'temporal-general'
       END
     ) AS categories
WHERE size(members) > 0
WITH members, embeddings, categories,
     CASE WHEN size(embeddings) > 0 THEN
       [i IN range(0, size(embeddings[0])-1) |
         reduce(sum = 0.0, emb IN embeddings | sum + emb[i]) / size(embeddings)
       ]
     ELSE null END AS centroid
CREATE (t:MemoryNode:TemporalPattern {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: 'temporal-patterns',
  layer: 1,
  role_type: 'temporal',
  embedding: centroid,
  message_pass_embedding: centroid,
  aggregation_count: size(members),
  stability_score: 1.0,
  summary: 'Temporal modeling patterns: ' + toString(size(members)) + ' entities. Patterns: ' +
           reduce(s = '', cat IN categories | s + CASE WHEN s = '' THEN '' ELSE ', ' END + cat) +
           '. Common patterns include validFrom/validTo date ranges, soft deletes with deletedAt, and versioned/audited entities.',
  tags: ['temporal', 'temporal-patterns', 'cross-cutting'] + $patterns,
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH t, members
UNWIND members AS m
CREATE (m)-[:SHARES_TEMPORAL_PATTERN {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(t)
RETURN t.node_id AS nodeId, size(members) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId":  spaceID,
			"patterns": patterns,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, nil, err
	}
	if result.(int) == 0 {
		return false, 0, nil, nil // No temporal entities found
	}
	return true, result.(int), patterns, nil
}

// detectTemporalPatterns analyzes the codebase for specific temporal patterns
func (s *Service) detectTemporalPatterns(ctx context.Context, spaceID string) ([]string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Look for nodes with temporal concern tag and analyze their names/content
	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE 'concern:temporal' IN n.tags
WITH collect(DISTINCT
  CASE
    WHEN toLower(n.name) CONTAINS 'validfrom' OR toLower(n.name) CONTAINS 'validto' OR
         toLower(n.name) CONTAINS 'valid_from' OR toLower(n.name) CONTAINS 'valid_to' THEN 'validity-period'
    WHEN toLower(n.name) CONTAINS 'effective' OR toLower(n.name) CONTAINS 'expir' THEN 'effective-dates'
    WHEN toLower(n.name) CONTAINS 'deleteat' OR toLower(n.name) CONTAINS 'deleted_at' OR
         toLower(n.name) CONTAINS 'softdelete' THEN 'soft-delete'
    WHEN toLower(n.name) CONTAINS 'version' OR toLower(n.name) CONTAINS 'snapshot' THEN 'versioning'
    WHEN toLower(n.name) CONTAINS 'history' OR toLower(n.name) CONTAINS 'audit' THEN 'audit-history'
    WHEN toLower(n.name) CONTAINS 'createdat' OR toLower(n.name) CONTAINS 'updatedat' THEN 'timestamps'
    WHEN toLower(n.name) CONTAINS 'range' OR toLower(n.name) CONTAINS 'period' THEN 'date-ranges'
    ELSE null
  END
) AS patterns
RETURN [p IN patterns WHERE p IS NOT NULL] AS detectedPatterns`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			patternsRaw, _ := rec.Get("detectedPatterns")
			if patternsRaw == nil {
				return []string{}, nil
			}
			// Convert to string slice
			patternsSlice, ok := patternsRaw.([]any)
			if !ok {
				return []string{}, nil
			}
			patterns := make([]string, 0, len(patternsSlice))
			for _, p := range patternsSlice {
				if ps, ok := p.(string); ok {
					patterns = append(patterns, ps)
				}
			}
			return patterns, nil
		}
		return []string{}, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// =============================================================================
// TRACK 6: UI/UX PATTERN NODES
// =============================================================================
// Creates dedicated summary nodes for UI patterns (React/Next.js):
// - store: Zustand/Redux state management
// - component: React component patterns
// - routing: Next.js/React Router navigation
// - data-fetching: React Query/SWR data management
// - ui-library: Component libraries (dnd-kit, framer-motion, radix-ui)
// - form: Form handling (react-hook-form, formik)
// - context: React Context patterns

// CreateUINodes creates dedicated summary nodes for UI patterns
// based on "ui:*" tags in the base data layer.
// This addresses P4 in the development roadmap - improving retrieval for
// React/Next.js UI components, state management, and data fetching patterns.
func (s *Service) CreateUINodes(ctx context.Context, spaceID string) (*UINodeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &UINodeResult{}, nil
	}

	result := &UINodeResult{}

	// Get list of UI patterns present in the codebase
	patterns, err := s.detectUIPatternTypes(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("detect UI pattern types: %w", err)
	}

	if len(patterns) == 0 {
		return result, nil // No UI patterns found
	}

	result.PatternsDetected = patterns

	// Create a node for each distinct UI pattern type
	for _, pattern := range patterns {
		exists, err := s.uiNodeExists(ctx, spaceID, pattern)
		if err != nil {
			return nil, fmt.Errorf("check UI node exists (%s): %w", pattern, err)
		}
		if exists {
			continue // Already exists
		}

		created, edges, err := s.createUINodeWithEdges(ctx, spaceID, pattern)
		if err != nil {
			return nil, fmt.Errorf("create UI node (%s): %w", pattern, err)
		}

		if created {
			result.UINodesCreated++
			result.EdgesCreated += edges
		}
	}

	return result, nil
}

// uiNodeExists checks if a UI pattern summary node already exists for the given pattern
func (s *Service) uiNodeExists(ctx context.Context, spaceID, pattern string) (bool, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (u:MemoryNode {space_id: $spaceId, role_type: $roleType})
RETURN count(u) > 0 AS exists`

	roleType := "ui-" + pattern // e.g., "ui-store", "ui-component"

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":  spaceID,
			"roleType": roleType,
		})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			exists, _ := rec.Get("exists")
			return exists.(bool), nil
		}
		return false, res.Err()
	})

	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

// createUINodeWithEdges creates a UI pattern node and SHARES_UI_PATTERN edges
func (s *Service) createUINodeWithEdges(ctx context.Context, spaceID, pattern string) (bool, int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	tagToMatch := "ui:" + pattern // e.g., "ui:store", "ui:component"
	roleType := "ui-" + pattern   // e.g., "ui-store", "ui-component"

	// Create the UI pattern node and edges
	createCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE $tag IN n.tags
WITH collect(n) AS members,
     [m IN collect(n) WHERE m.embedding IS NOT NULL | m.embedding] AS embeddings
WHERE size(members) > 0
WITH members, embeddings,
     CASE WHEN size(embeddings) > 0 THEN
       [i IN range(0, size(embeddings[0])-1) |
         reduce(sum = 0.0, emb IN embeddings | sum + emb[i]) / size(embeddings)
       ]
     ELSE null END AS centroid
CREATE (u:MemoryNode:UIPattern {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $patternName,
  layer: 1,
  role_type: $roleType,
  embedding: centroid,
  message_pass_embedding: centroid,
  aggregation_count: size(members),
  stability_score: 1.0,
  summary: $summary,
  tags: ['ui', $tag, 'ui-pattern', 'cross-cutting'],
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH u, members
UNWIND members AS m
CREATE (m)-[:SHARES_UI_PATTERN {
  space_id: $spaceId,
  edge_id: randomUUID(),
  weight: 1.0,
  created_at: datetime(),
  updated_at: datetime()
}]->(u)
RETURN u.node_id AS nodeId, size(members) AS edgeCount`

	patternName := "ui-" + pattern + "-patterns"
	summary := s.getUIPatternSummary(pattern)

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, createCypher, map[string]any{
			"spaceId":     spaceID,
			"tag":         tagToMatch,
			"roleType":    roleType,
			"patternName": patternName,
			"summary":     summary,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), nil
		}
		return 0, res.Err()
	})

	if err != nil {
		return false, 0, err
	}
	if result.(int) == 0 {
		return false, 0, nil // No UI entities found
	}
	return true, result.(int), nil
}

// detectUIPatternTypes analyzes the codebase for specific UI pattern types
func (s *Service) detectUIPatternTypes(ctx context.Context, spaceID string) ([]string, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Find all distinct UI pattern tags
	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId, layer: 0})
WHERE any(tag IN n.tags WHERE tag STARTS WITH 'ui:')
WITH n
UNWIND n.tags AS tag
WITH tag WHERE tag STARTS WITH 'ui:'
WITH DISTINCT substring(tag, 3) AS pattern
RETURN collect(pattern) AS patterns`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			patternsRaw, _ := rec.Get("patterns")
			if patternsRaw == nil {
				return []string{}, nil
			}
			patternsSlice, ok := patternsRaw.([]any)
			if !ok {
				return []string{}, nil
			}
			patterns := make([]string, 0, len(patternsSlice))
			for _, p := range patternsSlice {
				if ps, ok := p.(string); ok {
					patterns = append(patterns, ps)
				}
			}
			return patterns, nil
		}
		return []string{}, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// getUIPatternSummary returns a human-readable summary for each UI pattern type
func (s *Service) getUIPatternSummary(pattern string) string {
	summaries := map[string]string{
		"store": "State management patterns using Zustand, Redux, or Recoil. " +
			"Includes store definitions, selectors, actions, and state slices.",
		"component": "React component patterns including hooks (useState, useEffect, useMemo, useCallback), " +
			"refs, memoization, and functional/class component structures.",
		"routing": "Navigation and routing patterns using Next.js App Router, Pages Router, or React Router. " +
			"Includes useRouter, usePathname, useSearchParams, and route definitions.",
		"data-fetching": "Data fetching patterns using React Query (TanStack Query), SWR, or similar libraries. " +
			"Includes useQuery, useMutation, query clients, and caching strategies.",
		"ui-library": "UI component library patterns including drag-and-drop (dnd-kit), " +
			"animations (framer-motion), headless UI (Radix UI), and styling utilities (Tailwind, clsx).",
		"form": "Form handling patterns using react-hook-form, Formik, or similar. " +
			"Includes useForm, useController, validation, and field arrays.",
		"context": "React Context patterns for dependency injection and shared state. " +
			"Includes createContext, useContext, and Provider components.",
	}

	if summary, ok := summaries[pattern]; ok {
		return summary
	}
	return "UI pattern: " + pattern
}

// =============================================================================
// PHASE 3: CONVERSATION HIDDEN LAYER CLUSTERING
// =============================================================================
// Clusters conversation_observation nodes into conversation_theme nodes (Layer 1).
// This is kept separate from code file clustering to maintain clear boundaries
// between code knowledge and conversation knowledge.

// ClusterConversations performs DBSCAN clustering on conversation_observation nodes
// and creates conversation_theme nodes from the resulting clusters.
// This is the conversation equivalent of CreateHiddenNodes.
func (s *Service) ClusterConversations(ctx context.Context, spaceID string) (*ConversationThemeResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &ConversationThemeResult{}, nil
	}

	result := &ConversationThemeResult{}

	// Step 1: Fetch ALL clusterable observations (noise filtered at query level)
	observations, err := s.fetchClusterableConversationObservations(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("fetch clusterable conversation observations: %w", err)
	}

	if len(observations) < s.cfg.HiddenLayerMinSamples {
		return result, nil // Not enough data to cluster
	}

	result.ObservationsUsed = len(observations)
	slog.Info("ClusterConversations: clusterable observations", "count", len(observations))

	// HIDDEN-CHURN-001: the global detach-everything step is GONE. Themes are
	// matched to existing nodes by centroid similarity and updated in place
	// (theme-scoped rewires); only themes matched by no cluster are deleted.
	existingThemes, err := s.listConversationThemes(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("list existing themes: %w", err)
	}
	claimedThemes := make(map[string]bool, len(existingThemes))

	// Step 3: Filter to observations with embeddings
	validObs := make([]ConversationObservation, 0, len(observations))
	for _, obs := range observations {
		if len(obs.Embedding) > 0 {
			validObs = append(validObs, obs)
		}
	}

	if len(validObs) < s.cfg.HiddenLayerMinSamples {
		return result, nil
	}

	// Step 4: Run KMeans clustering on observation embeddings
	embeddings := make([][]float64, len(validObs))
	for i, obs := range validObs {
		embeddings[i] = obs.Embedding
	}

	// HIDDEN-CHURN-001 PR-B: themes-per-observation ratio is config-driven
	// (HIDDEN_THEME_TARGET_RATIO, default 0.1 — preserves ceil(n/10)).
	ratio := s.cfg.HiddenThemeTargetRatio
	if ratio <= 0 {
		ratio = 0.1
	}
	maxThemes := int(float64(len(validObs))*ratio + 0.999999)
	if maxThemes < 1 {
		maxThemes = 1
	}

	// KMeans clustering — partition observations into exactly maxThemes groups
	labels := KMeansCluster(embeddings, maxThemes, 50)
	clusters, noise := groupObservationsByCluster(validObs, labels)
	result.NoiseObservations = len(noise)

	// Step 5: Get existing theme count for unique naming
	existingCount, err := s.countConversationThemes(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("count existing conversation themes: %w", err)
	}

	// Step 6: Create theme nodes for each cluster
	themeID := 0
	// PR-B: every dropped cluster's members join the noise pool for density
	// assignment below — KMeans never emits label -1, so before this the
	// noise list was structurally empty and the min-samples/max-themes
	// drops silently excluded observations from the hierarchy forever.
	for _, members := range clusters {
		if result.ThemesCreated >= maxThemes {
			noise = append(noise, members...)
			continue
		}

		if len(members) < s.cfg.HiddenLayerMinSamples {
			noise = append(noise, members...)
			continue
		}

		// Compute centroid
		clusterEmbeddings := make([][]float64, len(members))
		for i, m := range members {
			clusterEmbeddings[i] = m.Embedding
		}
		centroid := ComputeCentroid(clusterEmbeddings)
		if centroid == nil {
			noise = append(noise, members...)
			continue
		}

		// Generate theme summary from observation content
		summary := generateConversationThemeSummary(members)

		// Find dominant observation type and average surprise score
		dominantType, avgSurprise := analyzeClusterMetadata(members)

		// HIDDEN-CHURN-001: match-or-create. A cluster whose centroid is
		// within the identity threshold of an existing theme UPDATES that
		// theme in place — node_id, inbound references, and evidence chains
		// survive the cycle instead of being destroyed every ~5 minutes.
		if matched := matchTheme(centroid, existingThemes, claimedThemes, s.themeIdentityThreshold()); matched != "" {
			claimedThemes[matched] = true
			edgesCreated, err := s.updateConversationThemeWithEdges(ctx, spaceID, matched, summary, centroid, members, dominantType, avgSurprise)
			if err != nil {
				return result, fmt.Errorf("update conversation theme %s: %w", matched, err)
			}
			result.ThemesUpdated++
			result.EdgesCreated += edgesCreated
			result.ThemeSummaries = append(result.ThemeSummaries, summary)
			continue
		}

		// Create unique name
		uniqueID := existingCount + themeID
		name := fmt.Sprintf("ConvTheme-%s-%d", sanitizeThemeName(summary), uniqueID)

		// Create the theme node and GENERALIZES edges
		edgesCreated, err := s.createConversationThemeWithEdges(ctx, spaceID, name, summary, centroid, members, dominantType, avgSurprise)
		if err != nil {
			return result, fmt.Errorf("create conversation theme %s: %w", name, err)
		}

		result.ThemesCreated++
		result.EdgesCreated += edgesCreated
		result.ThemeSummaries = append(result.ThemeSummaries, summary)
		themeID++
	}

	// HIDDEN-CHURN-001 PR-B: density assignment — NOISE observations attach
	// to their nearest theme when within the assignment threshold (edges
	// only, no new themes). Closes part of the 94% coverage gap: previously
	// every sub-threshold observation was silently dropped from the
	// hierarchy forever.
	if s.cfg.HiddenThemeAssignSimThreshold > 0 && len(noise) > 0 {
		assigned, aErr := s.assignNoiseToThemes(ctx, spaceID, noise, s.cfg.HiddenThemeAssignSimThreshold)
		if aErr != nil {
			slog.Warn("ClusterConversations: noise assignment failed", "error", aErr)
		} else if assigned > 0 {
			result.NoiseAssigned = assigned
			slog.Info("ClusterConversations: density-assigned noise observations", "count", assigned)
		}
	}

	// HIDDEN-CHURN-001: only themes matched by NO cluster this run die.
	if removed, err := s.deleteUnmatchedThemes(ctx, spaceID, claimedThemes, existingThemes); err != nil {
		slog.Warn("ClusterConversations: stale-theme cleanup failed", "error", err)
	} else if removed > 0 {
		slog.Info("ClusterConversations: removed stale themes", "count", removed)
	}

	return result, nil
}

// themeIdentityThreshold resolves the centroid-similarity floor for theme
// identity matching (HIDDEN_THEME_IDENTITY_SIM_THRESHOLD, default 0.90).
func (s *Service) themeIdentityThreshold() float64 {
	if s.cfg.HiddenThemeIdentitySimThreshold > 0 {
		return s.cfg.HiddenThemeIdentitySimThreshold
	}
	return 0.90
}

// fetchClusterableConversationObservations retrieves all conversation observations eligible for clustering.
// Filters out mechanical noise (build telemetry, session machinery) that adds no cognitive value.
// Returns ALL eligible observations regardless of existing theme edges — the caller handles
// detaching old edges before re-clustering.
func (s *Service) fetchClusterableConversationObservations(ctx context.Context, spaceID string) ([]ConversationObservation, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Fetch ALL clusterable observations — noise patterns excluded at query level.
	// No orphan filtering: caller detaches old GENERALIZES edges before re-clustering.
	noiseFilters := `
  AND NOT (o.content STARTS WITH 'Build/test succeeded')
  AND NOT (o.content STARTS WITH 'Git push detected')
  AND NOT (o.content STARTS WITH 'Ingest job')
  AND NOT (o.content STARTS WITH 'Session resumed')
  AND NOT (o.content STARTS WITH 'Co-activation round')
  AND NOT (o.content STARTS WITH 'Co-activation')
  AND NOT (o.content STARTS WITH 'Session reinforcement')
  AND NOT (o.content STARTS WITH '[doctor-probe]')`

	var cypher string
	if s.cfg.HiddenLayerBatchSize > 0 {
		cypher = fmt.Sprintf(`
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation', layer: 0})
WHERE o.embedding IS NOT NULL AND NOT coalesce(o.is_archived, false)%s
RETURN o.node_id AS nodeId, o.obs_type AS obsType, o.content AS content,
       o.summary AS summary, o.embedding AS embedding, o.surprise_score AS surpriseScore,
       o.session_id AS sessionId, o.tags AS tags
LIMIT %d`, noiseFilters, s.cfg.HiddenLayerBatchSize)
	} else {
		cypher = fmt.Sprintf(`
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation', layer: 0})
WHERE o.embedding IS NOT NULL AND NOT coalesce(o.is_archived, false)%s
RETURN o.node_id AS nodeId, o.obs_type AS obsType, o.content AS content,
       o.summary AS summary, o.embedding AS embedding, o.surprise_score AS surpriseScore,
       o.session_id AS sessionId, o.tags AS tags`, noiseFilters)
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var observations []ConversationObservation
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			obsType, _ := rec.Get("obsType")
			content, _ := rec.Get("content")
			summary, _ := rec.Get("summary")
			embedding, _ := rec.Get("embedding")
			surpriseScore, _ := rec.Get("surpriseScore")
			sessionID, _ := rec.Get("sessionId")
			tags, _ := rec.Get("tags")

			observations = append(observations, ConversationObservation{
				NodeID:        asString(nodeID),
				SpaceID:       spaceID,
				ObsType:       asString(obsType),
				Content:       asString(content),
				Summary:       asString(summary),
				Embedding:     asFloat64Slice(embedding),
				SurpriseScore: asFloat64(surpriseScore),
				SessionID:     asString(sessionID),
				Tags:          asStringSlice(tags),
			})
		}
		return observations, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]ConversationObservation), nil
}

// countConversationThemes returns the current count of conversation theme nodes
func (s *Service) countConversationThemes(ctx context.Context, spaceID string) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (t:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme'})
RETURN count(t) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("cnt")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// createConversationThemeWithEdges creates a conversation_theme node and GENERALIZES edges
func (s *Service) createConversationThemeWithEdges(ctx context.Context, spaceID, name, summary string, centroid []float64, members []ConversationObservation, dominantType string, avgSurprise float64) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
CREATE (t:MemoryNode:ConversationTheme {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $name,
  layer: 1,
  role_type: 'conversation_theme',
  summary: $summary,
  embedding: $centroid,
  message_pass_embedding: $centroid,
  aggregation_count: $memberCount,
  dominant_obs_type: $dominantType,
  avg_surprise_score: $avgSurprise,
  stability_score: 1.0,
  last_forward_pass: datetime(),
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH t
UNWIND $memberEdges AS me
MATCH (o:MemoryNode {space_id: $spaceId, node_id: me.memberId})
WITH t, o, me,
     // HIDDEN-WEIGHT-001: point.distance() returns NULL on embedding lists
     // (the CASE guard passed, then the THEN expr evaluated NULL — so edges
     // with GOOD embeddings got no weight while embedding-less ones got 0.5).
     CASE WHEN t.embedding IS NOT NULL AND o.embedding IS NOT NULL
          THEN vector.similarity.cosine(o.embedding, t.embedding)
          ELSE 0.5
     END AS similarity
CREATE (o)-[:GENERALIZES {
  space_id: $spaceId,
  edge_id: me.edgeId,
  weight: similarity,
  similarity_score: similarity,
  created_at: datetime(),
  updated_at: datetime()
}]->(t)
RETURN t.node_id AS themeId, count(o) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":      spaceID,
			"name":         name,
			"summary":      summary,
			"centroid":     toFloat32Slice(centroid),
			"memberCount":  len(members),
			"memberEdges":  memberEdgePairs(memberIDs),
			"dominantType": dominantType,
			"avgSurprise":  avgSurprise,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// groupObservationsByCluster groups conversation observations by their DBSCAN cluster labels
func groupObservationsByCluster(observations []ConversationObservation, labels []int) (map[int][]ConversationObservation, []ConversationObservation) {
	clusters := make(map[int][]ConversationObservation)
	var noise []ConversationObservation

	for i, label := range labels {
		if label == -1 {
			noise = append(noise, observations[i])
		} else {
			clusters[label] = append(clusters[label], observations[i])
		}
	}

	return clusters, noise
}

// generateConversationThemeSummary creates a summary for a conversation theme
// Uses keyword extraction and observation type analysis (no LLM calls)
func generateConversationThemeSummary(members []ConversationObservation) string {
	if len(members) == 0 {
		return "Empty conversation theme"
	}

	// Extract keywords from all member content
	keywords := extractKeywordsFromObservations(members)

	// Get dominant observation type
	dominantType, _ := analyzeClusterMetadata(members)

	// Build summary based on type and keywords
	var summary strings.Builder

	// Start with type-based prefix
	switch dominantType {
	case "decision":
		summary.WriteString("Decisions about ")
	case "correction":
		summary.WriteString("Corrections regarding ")
	case "learning":
		summary.WriteString("Learning about ")
	case "preference":
		summary.WriteString("Preferences for ")
	case "error":
		summary.WriteString("Error patterns in ")
	case "task":
		summary.WriteString("Tasks related to ")
	default:
		summary.WriteString("Observations about ")
	}

	// Add top keywords
	if len(keywords) > 0 {
		topKeywords := keywords
		if len(topKeywords) > 5 {
			topKeywords = topKeywords[:5]
		}
		summary.WriteString(strings.Join(topKeywords, ", "))
	} else {
		summary.WriteString("various topics")
	}

	// Add member count
	summary.WriteString(fmt.Sprintf(" (%d observations)", len(members)))

	return summary.String()
}

// extractKeywordsFromObservations extracts important keywords from observation content
func extractKeywordsFromObservations(observations []ConversationObservation) []string {
	// Build word frequency map
	wordFreq := make(map[string]int)

	// Common stop words to filter out
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
		"being": true, "have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true, "can": true,
		"to": true, "of": true, "in": true, "for": true, "on": true, "with": true,
		"at": true, "by": true, "from": true, "as": true, "into": true, "through": true,
		"that": true, "which": true, "who": true, "whom": true, "this": true, "these": true,
		"those": true, "it": true, "its": true, "they": true, "them": true, "their": true,
		"we": true, "us": true, "our": true, "you": true, "your": true, "he": true,
		"she": true, "him": true, "her": true, "his": true, "hers": true, "i": true,
		"me": true, "my": true, "not": true, "no": true, "yes": true, "if": true,
		"then": true, "else": true, "when": true, "where": true, "why": true, "how": true,
		"all": true, "each": true, "every": true, "both": true, "few": true, "more": true,
		"most": true, "other": true, "some": true, "such": true, "only": true, "own": true,
		"same": true, "so": true, "than": true, "too": true, "very": true, "just": true,
		"also": true, "now": true, "use": true, "used": true, "using": true,
	}

	for _, obs := range observations {
		// Tokenize content
		content := strings.ToLower(obs.Content + " " + obs.Summary)
		words := strings.FieldsFunc(content, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
		})

		for _, word := range words {
			// Filter short words and stop words
			if len(word) >= 3 && !stopWords[word] {
				wordFreq[word]++
			}
		}
	}

	// Sort by frequency and return top keywords
	type wordCount struct {
		word  string
		count int
	}
	counts := make([]wordCount, 0, len(wordFreq))
	for word, count := range wordFreq {
		counts = append(counts, wordCount{word, count})
	}

	// Sort by count descending
	for i := 0; i < len(counts)-1; i++ {
		for j := i + 1; j < len(counts); j++ {
			if counts[j].count > counts[i].count {
				counts[i], counts[j] = counts[j], counts[i]
			}
		}
	}

	// Extract top words
	result := make([]string, 0, min(10, len(counts)))
	for i := 0; i < len(counts) && i < 10; i++ {
		if counts[i].count >= 2 { // Only include words that appear at least twice
			result = append(result, counts[i].word)
		}
	}

	return result
}

// analyzeClusterMetadata determines dominant observation type and average surprise score
func analyzeClusterMetadata(members []ConversationObservation) (dominantType string, avgSurprise float64) {
	if len(members) == 0 {
		return "unknown", 0.0
	}

	// Count observation types
	typeCounts := make(map[string]int)
	totalSurprise := 0.0

	for _, m := range members {
		typeCounts[m.ObsType]++
		totalSurprise += m.SurpriseScore
	}

	// Find dominant type
	maxCount := 0
	for obsType, count := range typeCounts {
		if count > maxCount {
			maxCount = count
			dominantType = obsType
		}
	}

	// Calculate average surprise
	avgSurprise = totalSurprise / float64(len(members))

	return dominantType, avgSurprise
}

// sanitizeThemeName creates a safe name fragment from the summary
func sanitizeThemeName(summary string) string {
	// Extract first meaningful words from summary
	words := strings.Fields(summary)
	if len(words) == 0 {
		return "misc"
	}

	// Skip type prefixes like "Decisions about"
	startIdx := 0
	skipWords := map[string]bool{
		"decisions": true, "corrections": true, "learning": true,
		"preferences": true, "error": true, "errors": true, "tasks": true,
		"observations": true, "about": true, "regarding": true, "for": true,
		"related": true, "to": true, "patterns": true, "in": true,
	}

	for i, w := range words {
		if !skipWords[strings.ToLower(w)] {
			startIdx = i
			break
		}
	}

	// Take up to 3 meaningful words
	endIdx := startIdx + 3
	if endIdx > len(words) {
		endIdx = len(words)
	}

	result := strings.Join(words[startIdx:endIdx], "-")

	// Clean up the result
	cleaned := ""
	for _, ch := range result {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' {
			cleaned += string(ch)
		}
	}

	if len(cleaned) > 30 {
		cleaned = cleaned[:30]
	}
	if cleaned == "" {
		return "misc"
	}

	return strings.ToLower(cleaned)
}

// asStringSlice converts an interface to a string slice
func asStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	if arr, ok := v.([]string); ok {
		return arr
	}
	return nil
}

// RunConversationConsolidation performs consolidation specifically for conversation data
// This can be called independently or as part of the main consolidation
func (s *Service) RunConversationConsolidation(ctx context.Context, spaceID string) (*ConversationConsolidationResult, error) {
	start := time.Now()
	result := &ConversationConsolidationResult{}

	// Step 1: Cluster conversation observations into themes
	themeResult, err := s.ClusterConversations(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("cluster conversations: %w", err)
	}
	result.ThemeResult = themeResult

	// Step 2: Run forward pass to update theme embeddings if any were created
	if themeResult.ThemesCreated > 0 {
		fwdResult, err := s.forwardPassConversationThemes(ctx, spaceID)
		if err != nil {
			return nil, fmt.Errorf("forward pass conversation themes: %w", err)
		}
		result.ForwardPass = fwdResult
	}

	result.TotalDuration = time.Since(start)
	metrics.Metrics().EmergenceCycleDuration(spaceID, "conversation").Set(result.TotalDuration.Seconds())
	return result, nil
}

// forwardPassConversationThemes updates conversation theme embeddings by aggregating from observations
func (s *Service) forwardPassConversationThemes(ctx context.Context, spaceID string) (*ForwardPassResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	start := time.Now()

	cypher := `
MATCH (t:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme', layer: 1})
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})-[r:GENERALIZES]->(t)
WHERE o.embedding IS NOT NULL
WITH t, collect({emb: o.embedding, weight: coalesce(r.weight, 1.0)}) AS neighbors
WHERE size(neighbors) > 0
WITH t, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH t, neighbors, totalWeight,
     [i IN range(0, size(t.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET t.message_pass_embedding = [i IN range(0, size(t.embedding)-1) |
      $alpha * coalesce(t.embedding[i], 0) + $beta * aggregated[i]
    ],
    t.last_forward_pass = datetime(),
    t.aggregation_count = size(neighbors)
RETURN count(t) AS updated`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"alpha":   s.cfg.HiddenLayerForwardAlpha,
			"beta":    s.cfg.HiddenLayerForwardBeta,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			updated, _ := rec.Get("updated")
			return asInt(updated), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return nil, err
	}

	return &ForwardPassResult{
		HiddenNodesUpdated: result.(int),
		Duration:           time.Since(start),
	}, nil
}

// GenerateConversationSummaries creates summaries for conversation theme nodes
func (s *Service) GenerateConversationSummaries(ctx context.Context, spaceID string) (int, error) {
	if !s.cfg.HiddenLayerEnabled {
		return 0, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Update conversation themes without summaries
	cypher := `
MATCH (t:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme', layer: 1})
WHERE t.summary IS NULL OR t.summary = ''
MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})-[:GENERALIZES]->(t)
WITH t, count(o) AS memberCount,
     collect(DISTINCT o.obs_type) AS obsTypes,
     collect(DISTINCT CASE WHEN o.summary IS NOT NULL AND o.summary <> '' THEN o.summary ELSE null END)[0..5] AS sampleSummaries
SET t.summary = 'Conversation theme of ' + toString(memberCount) + ' observations. ' +
    'Types: ' + reduce(s = '', tp IN obsTypes | s + CASE WHEN s = '' THEN '' ELSE ', ' END + tp) +
    CASE WHEN size(sampleSummaries) > 0
      THEN '. Examples: ' + reduce(s = '', sm IN [x IN sampleSummaries WHERE x IS NOT NULL] | s + CASE WHEN s = '' THEN '' ELSE '; ' END + sm)
      ELSE ''
    END,
    t.updated_at = datetime()
RETURN count(t) AS updated`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			updated, _ := rec.Get("updated")
			return asInt(updated), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// =============================================================================
// PHASE 4: EMERGENT CONCEPT FORMATION
// =============================================================================
// Emergent concepts form from clustering conversation_theme nodes (L1) into
// higher-level emergent_concept nodes (L2+). This creates a hierarchy:
// L0: conversation_observation (raw learnings)
// L1: conversation_theme (clustered observations)
// L2+: emergent_concept (higher-level abstractions)

// ClusterEmergentConcepts clusters conversation themes (L1) into emergent concepts (L2+).
// This method supports multi-layer clustering:
//   - L1 themes -> L2 emergent concepts
//   - L2 concepts -> L3 emergent concepts
//   - etc.
//
// Cross-session learning: Themes from different sessions can cluster together,
// forming concepts that represent understanding spanning multiple conversations.
func (s *Service) ClusterEmergentConcepts(ctx context.Context, spaceID string, targetLayer int) (*EmergentConceptResult, error) {
	if !s.cfg.HiddenLayerEnabled {
		return &EmergentConceptResult{ConceptsCreated: make(map[int]int)}, nil
	}

	if targetLayer < 2 {
		return nil, fmt.Errorf("target layer must be >= 2 for emergent concepts")
	}

	result := &EmergentConceptResult{
		ConceptsCreated: make(map[int]int),
	}

	sourceLayer := targetLayer - 1

	// Fetch source nodes based on layer
	var sourceNodes []EmergentConceptNode
	var err error

	if sourceLayer == 1 {
		// Clustering themes (L1) into emergent concepts (L2)
		themes, err := s.fetchOrphanConversationThemes(ctx, spaceID, targetLayer)
		if err != nil {
			return nil, fmt.Errorf("fetch orphan conversation themes: %w", err)
		}
		// Convert themes to EmergentConceptNode for unified handling
		sourceNodes = convertThemesToConceptNodes(themes)
		result.ThemesUsed = len(sourceNodes)
	} else {
		// Clustering emergent concepts (L2+) into higher layer concepts
		sourceNodes, err = s.fetchOrphanEmergentConcepts(ctx, spaceID, sourceLayer, targetLayer)
		if err != nil {
			return nil, fmt.Errorf("fetch orphan emergent concepts layer %d: %w", sourceLayer, err)
		}
	}

	// Calculate ADAPTIVE parameters - constraints loosen as layers increase
	// This mirrors the behavior in CreateConceptNodes for code clustering
	layerFactor := float64(targetLayer - 1) // 1 for L2, 2 for L3, etc.

	// Epsilon grows with layer: base * (1 + 0.4*layer)
	// Higher layers should cluster more freely as concepts become more abstract
	adaptiveEps := s.cfg.HiddenLayerClusterEps * (1.0 + 0.4*layerFactor)
	if adaptiveEps > 0.6 {
		adaptiveEps = 0.6 // Cap to maintain semantic coherence
	}

	// MinSamples shrinks with layer: base - layer (min 2)
	// Emergent concepts need at least 3 themes/concepts to form (configurable)
	minSamplesForConcepts := 3 // Default: require 3 themes to form an emergent concept
	adaptiveMinSamples := minSamplesForConcepts - int(layerFactor-1)
	if adaptiveMinSamples < 2 {
		adaptiveMinSamples = 2 // Minimum 2 nodes to form a cluster
	}

	if len(sourceNodes) < adaptiveMinSamples {
		return result, nil // Not enough data to cluster
	}

	// Filter to nodes with embeddings, prefer message_pass_embedding
	validNodes := make([]EmergentConceptNode, 0, len(sourceNodes))
	for _, node := range sourceNodes {
		emb := node.Embedding
		if len(node.MessagePassEmbedding) > 0 {
			emb = node.MessagePassEmbedding
		}
		if len(emb) > 0 {
			node.Embedding = emb // Store effective embedding
			validNodes = append(validNodes, node)
		}
	}

	if len(validNodes) < adaptiveMinSamples {
		return result, nil
	}

	// Run DBSCAN clustering
	embeddings := make([][]float64, len(validNodes))
	for i, n := range validNodes {
		embeddings[i] = n.Embedding
	}

	labels := DBSCAN(embeddings, adaptiveEps, adaptiveMinSamples)
	clusters, noise := groupEmergentConceptsByCluster(validNodes, labels)
	result.NoiseThemes = len(noise)

	// Get existing emergent concept count for unique naming
	existingCount, err := s.countEmergentConcepts(ctx, spaceID, targetLayer)
	if err != nil {
		return nil, fmt.Errorf("count existing emergent concepts layer %d: %w", targetLayer, err)
	}

	// Create emergent concept nodes for each cluster
	conceptID := 0
	for _, members := range clusters {
		if len(members) < adaptiveMinSamples {
			continue
		}

		// Compute centroid
		clusterEmbeddings := make([][]float64, len(members))
		for i, m := range members {
			clusterEmbeddings[i] = m.Embedding
		}
		centroid := ComputeCentroid(clusterEmbeddings)
		if centroid == nil {
			continue
		}

		// Generate elevated summary from member summaries
		summary := generateEmergentConceptSummary(members, targetLayer)

		// Aggregate keywords from members
		keywords := aggregateKeywordsFromConcepts(members)

		// Calculate aggregate metadata
		avgSurprise, sessionCount := calculateEmergentConceptMetadata(members)

		// Create unique name (mechanical fallback)
		uniqueID := existingCount + conceptID
		name := fmt.Sprintf("EmergentConcept-L%d-%s-%d", targetLayer, sanitizeConceptName(summary), uniqueID)
		if targetLayer == 5 {
			if namer := s.newEmergenceNamer(); namer != nil {
				clusterNodes := emergentConceptNodesToClusterSummaries(members)
				if llmResult, llmErr := namer.NameL5Concept(ctx, clusterNodes); llmErr != nil {
					fmt.Printf("L5 emergent concept LLM naming failed (using mechanical): %v\n", llmErr)
				} else {
					name = llmResult.Name
				}
			}
		}

		// Create the emergent concept node and ABSTRACTS_TO edges
		edgesCreated, err := s.createEmergentConceptWithEdges(ctx, spaceID, name, summary, centroid, members, keywords, avgSurprise, sessionCount, targetLayer)
		if err != nil {
			return result, fmt.Errorf("create emergent concept %s: %w", name, err)
		}

		result.ConceptsCreated[targetLayer]++
		result.EdgesCreated += edgesCreated
		result.ConceptSummaries = append(result.ConceptSummaries, summary)
		if targetLayer > result.MaxLayerReached {
			result.MaxLayerReached = targetLayer
		}
		conceptID++
	}

	return result, nil
}

// convertThemesToConceptNodes converts ConversationThemeForClustering to EmergentConceptNode
func convertThemesToConceptNodes(themes []ConversationThemeForClustering) []EmergentConceptNode {
	nodes := make([]EmergentConceptNode, len(themes))
	for i, t := range themes {
		nodes[i] = EmergentConceptNode{
			NodeID:               t.NodeID,
			SpaceID:              t.SpaceID,
			Layer:                1,
			Name:                 t.Name,
			Summary:              t.Summary,
			Embedding:            t.Embedding,
			MessagePassEmbedding: t.MessagePassEmbedding,
			Keywords:             t.Keywords,
			SessionCount:         len(t.SessionIDs),
		}
	}
	return nodes
}

// fetchOrphanConversationThemes retrieves conversation themes without ABSTRACTS_TO edge to target layer
func (s *Service) fetchOrphanConversationThemes(ctx context.Context, spaceID string, targetLayer int) ([]ConversationThemeForClustering, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (t:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme', layer: 1})
WHERE NOT (t)-[:ABSTRACTS_TO]->(:MemoryNode {layer: $targetLayer, role_type: 'emergent_concept'})
  AND (t.embedding IS NOT NULL OR t.message_pass_embedding IS NOT NULL)
OPTIONAL MATCH (o:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})-[:GENERALIZES]->(t)
WITH t, collect(DISTINCT o.session_id) AS sessionIds
RETURN t.node_id AS nodeId, t.name AS name, t.summary AS summary,
       t.embedding AS embedding, t.message_pass_embedding AS messagePassEmbedding,
       t.aggregation_count AS memberCount, t.avg_surprise_score AS avgSurprise,
       sessionIds`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"targetLayer": targetLayer,
		})
		if err != nil {
			return nil, err
		}

		var themes []ConversationThemeForClustering
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			embedding, _ := rec.Get("embedding")
			msgPassEmb, _ := rec.Get("messagePassEmbedding")
			memberCount, _ := rec.Get("memberCount")
			avgSurprise, _ := rec.Get("avgSurprise")
			sessionIDs, _ := rec.Get("sessionIds")

			themes = append(themes, ConversationThemeForClustering{
				NodeID:               asString(nodeID),
				SpaceID:              spaceID,
				Name:                 asString(name),
				Summary:              asString(summary),
				Embedding:            asFloat64Slice(embedding),
				MessagePassEmbedding: asFloat64Slice(msgPassEmb),
				MemberCount:          asInt(memberCount),
				AvgSurpriseScore:     asFloat64(avgSurprise),
				SessionIDs:           asStringSlice(sessionIDs),
			})
		}
		return themes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]ConversationThemeForClustering), nil
}

// fetchOrphanEmergentConcepts retrieves emergent concepts at sourceLayer without ABSTRACTS_TO to targetLayer
func (s *Service) fetchOrphanEmergentConcepts(ctx context.Context, spaceID string, sourceLayer, targetLayer int) ([]EmergentConceptNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept', layer: $sourceLayer})
WHERE NOT (c)-[:ABSTRACTS_TO]->(:MemoryNode {layer: $targetLayer, role_type: 'emergent_concept'})
  AND (c.embedding IS NOT NULL OR c.message_pass_embedding IS NOT NULL)
RETURN c.node_id AS nodeId, c.name AS name, c.summary AS summary,
       c.embedding AS embedding, c.message_pass_embedding AS messagePassEmbedding,
       c.keywords AS keywords, c.session_count AS sessionCount`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":     spaceID,
			"sourceLayer": sourceLayer,
			"targetLayer": targetLayer,
		})
		if err != nil {
			return nil, err
		}

		var concepts []EmergentConceptNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			embedding, _ := rec.Get("embedding")
			msgPassEmb, _ := rec.Get("messagePassEmbedding")
			keywords, _ := rec.Get("keywords")
			sessionCount, _ := rec.Get("sessionCount")

			concepts = append(concepts, EmergentConceptNode{
				NodeID:               asString(nodeID),
				SpaceID:              spaceID,
				Layer:                sourceLayer,
				Name:                 asString(name),
				Summary:              asString(summary),
				Embedding:            asFloat64Slice(embedding),
				MessagePassEmbedding: asFloat64Slice(msgPassEmb),
				Keywords:             asStringSlice(keywords),
				SessionCount:         asInt(sessionCount),
			})
		}
		return concepts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]EmergentConceptNode), nil
}

// countEmergentConcepts returns the count of emergent concept nodes at a specific layer
func (s *Service) countEmergentConcepts(ctx context.Context, spaceID string, layer int) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept', layer: $layer})
RETURN count(c) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"layer":   layer,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			cnt, _ := rec.Get("cnt")
			return asInt(cnt), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// groupEmergentConceptsByCluster groups nodes by their DBSCAN cluster labels
func groupEmergentConceptsByCluster(nodes []EmergentConceptNode, labels []int) (map[int][]EmergentConceptNode, []EmergentConceptNode) {
	clusters := make(map[int][]EmergentConceptNode)
	var noise []EmergentConceptNode

	for i, label := range labels {
		if label == -1 {
			noise = append(noise, nodes[i])
		} else {
			clusters[label] = append(clusters[label], nodes[i])
		}
	}

	return clusters, noise
}

// generateEmergentConceptSummary creates an elevated summary from member summaries
// The summary represents a higher-level abstraction of the underlying themes
func generateEmergentConceptSummary(members []EmergentConceptNode, layer int) string {
	if len(members) == 0 {
		return "Empty emergent concept"
	}

	// Extract keywords from all member summaries
	keywords := aggregateKeywordsFromConcepts(members)

	// Build summary with elevation language based on layer
	var summary strings.Builder

	// Layer-appropriate prefix
	switch {
	case layer == 2:
		summary.WriteString("Emerging pattern: ")
	case layer == 3:
		summary.WriteString("Core understanding: ")
	case layer >= 4:
		summary.WriteString("Foundational principle: ")
	}

	// Add top keywords
	if len(keywords) > 0 {
		topKeywords := keywords
		if len(topKeywords) > 4 {
			topKeywords = topKeywords[:4]
		}
		summary.WriteString(strings.Join(topKeywords, ", "))
	} else {
		// Fall back to extracting concepts from member names
		concepts := extractConceptsFromNames(members)
		if len(concepts) > 0 {
			summary.WriteString(strings.Join(concepts, ", "))
		} else {
			summary.WriteString("diverse observations")
		}
	}

	// Add member count
	summary.WriteString(fmt.Sprintf(" (%d themes, L%d)", len(members), layer))

	return summary.String()
}

// extractConceptsFromNames extracts meaningful concept words from member node names
func extractConceptsFromNames(members []EmergentConceptNode) []string {
	wordFreq := make(map[string]int)

	for _, m := range members {
		// Extract words from name and summary
		words := strings.FieldsFunc(m.Name+" "+m.Summary, func(r rune) bool {
			return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
		})

		skipWords := map[string]bool{
			"convtheme": true, "emergentconcept": true, "hidden": true, "concept": true,
			"about": true, "the": true, "and": true, "for": true, "with": true,
			"observations": true, "themes": true, "pattern": true, "emerging": true,
		}

		for _, word := range words {
			w := strings.ToLower(word)
			if len(w) >= 3 && !skipWords[w] {
				wordFreq[w]++
			}
		}
	}

	// Sort by frequency
	type wordCount struct {
		word  string
		count int
	}
	counts := make([]wordCount, 0, len(wordFreq))
	for word, count := range wordFreq {
		counts = append(counts, wordCount{word, count})
	}

	// Sort descending by count
	for i := 0; i < len(counts)-1; i++ {
		for j := i + 1; j < len(counts); j++ {
			if counts[j].count > counts[i].count {
				counts[i], counts[j] = counts[j], counts[i]
			}
		}
	}

	// Return top words
	result := make([]string, 0, 3)
	for i := 0; i < len(counts) && i < 3; i++ {
		if counts[i].count >= 2 {
			result = append(result, counts[i].word)
		}
	}

	return result
}

// aggregateKeywordsFromConcepts combines keywords from all members and returns top ones
func aggregateKeywordsFromConcepts(members []EmergentConceptNode) []string {
	wordFreq := make(map[string]int)

	for _, m := range members {
		// Add member's keywords
		for _, kw := range m.Keywords {
			wordFreq[strings.ToLower(kw)]++
		}

		// Also extract keywords from summary if no keywords stored
		if len(m.Keywords) == 0 && m.Summary != "" {
			words := strings.FieldsFunc(strings.ToLower(m.Summary), func(r rune) bool {
				return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
			})

			stopWords := map[string]bool{
				"the": true, "and": true, "for": true, "about": true, "with": true,
				"observations": true, "theme": true, "themes": true, "pattern": true,
				"learning": true, "decisions": true, "preferences": true, "emerging": true,
			}

			for _, word := range words {
				if len(word) >= 3 && !stopWords[word] {
					wordFreq[word]++
				}
			}
		}
	}

	// Sort and return top keywords
	type wordCount struct {
		word  string
		count int
	}
	counts := make([]wordCount, 0, len(wordFreq))
	for word, count := range wordFreq {
		counts = append(counts, wordCount{word, count})
	}

	// Sort descending
	for i := 0; i < len(counts)-1; i++ {
		for j := i + 1; j < len(counts); j++ {
			if counts[j].count > counts[i].count {
				counts[i], counts[j] = counts[j], counts[i]
			}
		}
	}

	// Return top keywords
	result := make([]string, 0, 10)
	for i := 0; i < len(counts) && i < 10; i++ {
		result = append(result, counts[i].word)
	}

	return result
}

// calculateEmergentConceptMetadata calculates aggregate metadata for an emergent concept
func calculateEmergentConceptMetadata(members []EmergentConceptNode) (avgSurprise float64, sessionCount int) {
	if len(members) == 0 {
		return 0.0, 0
	}

	// Track unique sessions (approximate by summing session counts)
	totalSessions := 0
	for _, m := range members {
		totalSessions += m.SessionCount
	}

	// Session count is an approximation (could have overlap)
	sessionCount = totalSessions
	if sessionCount > len(members)*2 {
		// Cap at reasonable estimate to avoid over-counting
		sessionCount = len(members) * 2
	}

	// avgSurprise would need to be fetched from DB for themes
	// For now, return 0 as it's recalculated during creation
	return 0.0, sessionCount
}

// sanitizeConceptName creates a safe name fragment from the summary
func sanitizeConceptName(summary string) string {
	words := strings.Fields(summary)
	if len(words) == 0 {
		return "misc"
	}

	// Skip prefix words (check without punctuation)
	skipWords := map[string]bool{
		"emerging": true, "pattern": true, "core": true, "understanding": true,
		"foundational": true, "principle": true, "about": true, "the": true,
	}

	// Helper to strip punctuation from a word for comparison
	stripPunctuation := func(w string) string {
		result := ""
		for _, ch := range w {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				result += string(ch)
			}
		}
		return result
	}

	startIdx := len(words) // Default to end if all are skip words
	for i, w := range words {
		cleanWord := stripPunctuation(strings.ToLower(w))
		if cleanWord != "" && !skipWords[cleanWord] {
			startIdx = i
			break
		}
	}

	// If all words were skip words, return misc
	if startIdx >= len(words) {
		return "misc"
	}

	// Take up to 2 meaningful words
	endIdx := startIdx + 2
	if endIdx > len(words) {
		endIdx = len(words)
	}

	result := strings.Join(words[startIdx:endIdx], "-")

	// Clean up - remove non-alphanumeric except dash
	cleaned := ""
	for _, ch := range result {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '-' {
			cleaned += string(ch)
		}
	}

	if len(cleaned) > 25 {
		cleaned = cleaned[:25]
	}
	if cleaned == "" {
		return "misc"
	}

	return strings.ToLower(cleaned)
}

// createEmergentConceptWithEdges creates an emergent_concept node and ABSTRACTS_TO edges
func (s *Service) createEmergentConceptWithEdges(ctx context.Context, spaceID, name, summary string, centroid []float64, members []EmergentConceptNode, keywords []string, avgSurprise float64, sessionCount, layer int) (int, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	memberIDs := make([]string, len(members))
	for i, m := range members {
		memberIDs[i] = m.NodeID
	}

	cypher := `
CREATE (c:MemoryNode:EmergentConcept {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $name,
  layer: $layer,
  role_type: 'emergent_concept',
  summary: $summary,
  embedding: $centroid,
  message_pass_embedding: $centroid,
  keywords: $keywords,
  aggregation_count: $memberCount,
  avg_surprise_score: $avgSurprise,
  session_count: $sessionCount,
  stability_score: 1.0,
  last_forward_pass: datetime(),
  created_at: datetime(),
  updated_at: datetime(),
  version: 1
})
WITH c
UNWIND $memberEdges AS me
MATCH (m:MemoryNode {space_id: $spaceId, node_id: me.memberId})
WITH c, m, me,
     // HIDDEN-WEIGHT-001: point.distance() returns NULL on embedding lists —
     // 95% of ABSTRACTS_TO edges were weightless. cosine returns [0,1].
     CASE WHEN c.embedding IS NOT NULL AND m.embedding IS NOT NULL
          THEN vector.similarity.cosine(m.embedding, c.embedding)
          ELSE 0.5
     END AS similarity
CREATE (m)-[:ABSTRACTS_TO {
  space_id: $spaceId,
  edge_id: me.edgeId,
  weight: similarity,
  similarity_score: similarity,
  created_at: datetime(),
  updated_at: datetime()
}]->(c)
RETURN c.node_id AS conceptId, count(m) AS edgeCount`

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":      spaceID,
			"name":         name,
			"layer":        layer,
			"summary":      summary,
			"centroid":     toFloat32Slice(centroid),
			"keywords":     keywords,
			"memberCount":  len(members),
			"memberEdges":  memberEdgePairs(memberIDs),
			"avgSurprise":  avgSurprise,
			"sessionCount": sessionCount,
		})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			edgeCount, _ := rec.Get("edgeCount")
			return asInt(edgeCount), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// RunFullConversationConsolidation performs complete conversation consolidation including emergent concepts
// This extends RunConversationConsolidation to include:
// 1. L0 observations -> L1 themes (ClusterConversations)
// 2. L1 themes -> L2 emergent concepts (ClusterEmergentConcepts)
// 3. L2 concepts -> L3 concepts, etc. (iterative ClusterEmergentConcepts)
func (s *Service) RunFullConversationConsolidation(ctx context.Context, spaceID string) (*ConversationConsolidationResult, error) {
	start := time.Now()
	result := &ConversationConsolidationResult{
		ConceptResult: &EmergentConceptResult{
			ConceptsCreated: make(map[int]int),
		},
	}

	// Step 1: Cluster conversation observations into themes (L0 -> L1)
	themeResult, err := s.ClusterConversations(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("cluster conversations: %w", err)
	}
	result.ThemeResult = themeResult

	// Step 2: Run forward pass to update theme embeddings
	if themeResult.ThemesCreated > 0 {
		fwdResult, err := s.forwardPassConversationThemes(ctx, spaceID)
		if err != nil {
			return nil, fmt.Errorf("forward pass conversation themes: %w", err)
		}
		result.ForwardPass = fwdResult
	}

	// Step 3: Multi-layer emergent concept clustering (L1 -> L2, L2 -> L3, etc.)
	maxLayers := 5 // Reasonable depth limit for conversation concepts
	for targetLayer := 2; targetLayer <= maxLayers; targetLayer++ {
		conceptResult, err := s.ClusterEmergentConcepts(ctx, spaceID, targetLayer)
		if err != nil {
			return nil, fmt.Errorf("cluster emergent concepts layer %d: %w", targetLayer, err)
		}

		// Merge results
		for layer, count := range conceptResult.ConceptsCreated {
			result.ConceptResult.ConceptsCreated[layer] += count
		}
		result.ConceptResult.EdgesCreated += conceptResult.EdgesCreated
		result.ConceptResult.ConceptSummaries = append(result.ConceptResult.ConceptSummaries, conceptResult.ConceptSummaries...)
		if conceptResult.MaxLayerReached > result.ConceptResult.MaxLayerReached {
			result.ConceptResult.MaxLayerReached = conceptResult.MaxLayerReached
		}

		// Run forward pass if concepts were created
		if conceptResult.ConceptsCreated[targetLayer] > 0 {
			_, err = s.forwardPassEmergentConcepts(ctx, spaceID, targetLayer)
			if err != nil {
				return nil, fmt.Errorf("forward pass emergent concepts layer %d: %w", targetLayer, err)
			}
		}
	}

	result.TotalDuration = time.Since(start)
	metrics.Metrics().EmergenceCycleDuration(spaceID, "conversation_full").Set(result.TotalDuration.Seconds())
	return result, nil
}

// forwardPassEmergentConcepts updates emergent concept embeddings by aggregating from children
func (s *Service) forwardPassEmergentConcepts(ctx context.Context, spaceID string, layer int) (*ForwardPassResult, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	start := time.Now()

	// The source for L2 is conversation_theme, for L3+ it's emergent_concept
	var cypher string
	if layer == 2 {
		cypher = `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept', layer: 2})
MATCH (t:MemoryNode {space_id: $spaceId, role_type: 'conversation_theme'})-[r:ABSTRACTS_TO]->(c)
WHERE t.embedding IS NOT NULL OR t.message_pass_embedding IS NOT NULL
WITH c, collect({
  emb: coalesce(t.message_pass_embedding, t.embedding),
  weight: coalesce(r.weight, 1.0)
}) AS neighbors
WHERE size(neighbors) > 0
WITH c, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH c, neighbors, totalWeight,
     [i IN range(0, size(c.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET c.message_pass_embedding = [i IN range(0, size(c.embedding)-1) |
      $alpha * coalesce(c.embedding[i], 0) + $beta * aggregated[i]
    ],
    c.last_forward_pass = datetime(),
    c.aggregation_count = size(neighbors)
RETURN count(c) AS updated`
	} else {
		// For L3+, aggregate from lower-layer emergent concepts
		cypher = `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept', layer: $layer})
MATCH (m:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept', layer: $sourceLayer})-[r:ABSTRACTS_TO]->(c)
WHERE m.embedding IS NOT NULL OR m.message_pass_embedding IS NOT NULL
WITH c, collect({
  emb: coalesce(m.message_pass_embedding, m.embedding),
  weight: coalesce(r.weight, 1.0)
}) AS neighbors
WHERE size(neighbors) > 0
WITH c, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH c, neighbors, totalWeight,
     [i IN range(0, size(c.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET c.message_pass_embedding = [i IN range(0, size(c.embedding)-1) |
      $alpha * coalesce(c.embedding[i], 0) + $beta * aggregated[i]
    ],
    c.last_forward_pass = datetime(),
    c.aggregation_count = size(neighbors)
RETURN count(c) AS updated`
	}

	params := map[string]any{
		"spaceId": spaceID,
		"alpha":   s.cfg.HiddenLayerForwardAlpha,
		"beta":    s.cfg.HiddenLayerForwardBeta,
	}
	if layer > 2 {
		params["layer"] = layer
		params["sourceLayer"] = layer - 1
	}

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			updated, _ := rec.Get("updated")
			return asInt(updated), res.Err()
		}
		return 0, res.Err()
	})

	if err != nil {
		return nil, err
	}

	return &ForwardPassResult{
		ConceptNodesUpdated: result.(int),
		Duration:            time.Since(start),
	}, nil
}

// =============================================================================
// RETRIEVAL INTEGRATION HOOKS (Phase 5 Preparation)
// =============================================================================
// These functions prepare for Phase 5: Context-Aware Retrieval

// FindEmergentConceptsForSpace retrieves all emergent concepts in a space
func (s *Service) FindEmergentConceptsForSpace(ctx context.Context, spaceID string) ([]EmergentConcept, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	cypher := `
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept'})
RETURN c.node_id AS nodeId, c.layer AS layer, c.name AS name, c.summary AS summary,
       c.embedding AS embedding, c.aggregation_count AS memberCount,
       c.keywords AS keywords, c.avg_surprise_score AS avgSurprise, c.session_count AS sessionCount
ORDER BY c.layer DESC, c.aggregation_count DESC`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}

		var concepts []EmergentConcept
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			embedding, _ := rec.Get("embedding")
			memberCount, _ := rec.Get("memberCount")
			keywords, _ := rec.Get("keywords")
			avgSurprise, _ := rec.Get("avgSurprise")
			sessionCount, _ := rec.Get("sessionCount")

			concepts = append(concepts, EmergentConcept{
				NodeID:           asString(nodeID),
				SpaceID:          spaceID,
				Layer:            asInt(layer),
				Name:             asString(name),
				Summary:          asString(summary),
				Embedding:        asFloat64Slice(embedding),
				MemberCount:      asInt(memberCount),
				Keywords:         asStringSlice(keywords),
				AvgSurpriseScore: asFloat64(avgSurprise),
				SessionCount:     asInt(sessionCount),
			})
		}
		return concepts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]EmergentConcept), nil
}

// TraverseConceptToObservations traverses from an emergent concept down to its source observations
// This enables spreading activation through the concept hierarchy
func (s *Service) TraverseConceptToObservations(ctx context.Context, spaceID, conceptNodeID string, maxDepth int) ([]ConceptHierarchyNode, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	if maxDepth <= 0 {
		maxDepth = 5 // Default max depth
	}

	// Use variable-length path to traverse down through ABSTRACTS_TO and GENERALIZES edges
	cypher := `
MATCH path = (c:MemoryNode {space_id: $spaceId, node_id: $nodeId})<-[:ABSTRACTS_TO|GENERALIZES*1..` + fmt.Sprintf("%d", maxDepth) + `]-(n:MemoryNode)
WITH DISTINCT n
RETURN n.node_id AS nodeId, n.layer AS layer, n.role_type AS roleType,
       n.name AS name, n.summary AS summary, n.embedding AS embedding
ORDER BY n.layer DESC`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeId":  conceptNodeID,
		})
		if err != nil {
			return nil, err
		}

		var nodes []ConceptHierarchyNode
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			roleType, _ := rec.Get("roleType")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			embedding, _ := rec.Get("embedding")

			nodes = append(nodes, ConceptHierarchyNode{
				NodeID:    asString(nodeID),
				SpaceID:   spaceID,
				Layer:     asInt(layer),
				RoleType:  asString(roleType),
				Name:      asString(name),
				Summary:   asString(summary),
				Embedding: asFloat64Slice(embedding),
			})
		}
		return nodes, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]ConceptHierarchyNode), nil
}

// FindRelatedConcepts finds emergent concepts similar to a query embedding
// This supports spreading activation for retrieval
func (s *Service) FindRelatedConcepts(ctx context.Context, spaceID string, queryEmbedding []float64, minLayer int, topK int) ([]EmergentConcept, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	if topK <= 0 {
		topK = 10
	}
	if minLayer < 2 {
		minLayer = 2
	}

	// Use cosine similarity to find related concepts
	cypher := fmt.Sprintf(`
MATCH (c:MemoryNode {space_id: $spaceId, role_type: 'emergent_concept'})
WHERE c.layer >= $minLayer AND c.embedding IS NOT NULL
WITH c,
     reduce(dot = 0.0, i IN range(0, size(c.embedding)-1) |
       dot + c.embedding[i] * $queryEmb[i]) /
     (sqrt(reduce(sq = 0.0, i IN range(0, size(c.embedding)-1) | sq + c.embedding[i] * c.embedding[i])) *
      sqrt(reduce(sq = 0.0, i IN range(0, size($queryEmb)-1) | sq + $queryEmb[i] * $queryEmb[i]))) AS similarity
WHERE similarity > 0.5
RETURN c.node_id AS nodeId, c.layer AS layer, c.name AS name, c.summary AS summary,
       c.embedding AS embedding, c.aggregation_count AS memberCount,
       c.keywords AS keywords, c.avg_surprise_score AS avgSurprise, c.session_count AS sessionCount,
       similarity
ORDER BY similarity DESC
LIMIT %d`, topK)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId":  spaceID,
			"queryEmb": queryEmbedding,
			"minLayer": minLayer,
		})
		if err != nil {
			return nil, err
		}

		var concepts []EmergentConcept
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			name, _ := rec.Get("name")
			summary, _ := rec.Get("summary")
			embedding, _ := rec.Get("embedding")
			memberCount, _ := rec.Get("memberCount")
			keywords, _ := rec.Get("keywords")
			avgSurprise, _ := rec.Get("avgSurprise")
			sessionCount, _ := rec.Get("sessionCount")

			concepts = append(concepts, EmergentConcept{
				NodeID:           asString(nodeID),
				SpaceID:          spaceID,
				Layer:            asInt(layer),
				Name:             asString(name),
				Summary:          asString(summary),
				Embedding:        asFloat64Slice(embedding),
				MemberCount:      asInt(memberCount),
				Keywords:         asStringSlice(keywords),
				AvgSurpriseScore: asFloat64(avgSurprise),
				SessionCount:     asInt(sessionCount),
			})
		}
		return concepts, res.Err()
	})

	if err != nil {
		return nil, err
	}
	return result.([]EmergentConcept), nil
}
