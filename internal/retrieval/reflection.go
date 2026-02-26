package retrieval

import (
	"context"
	"errors"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/models"
)

// Default values for reflection parameters
const (
	DefaultReflectMaxDepth = 3
	DefaultReflectMaxNodes = 50
	DefaultReflectSeedK    = 20 // number of vector search results for core memories
)

// Reflect performs deep context exploration on a topic:
// 1) SEED: vector recall to find core memories matching the topic
// 2) EXPAND: lateral traversal via CO_ACTIVATED_WITH and ASSOCIATED_WITH edges
// 3) ABSTRACT: upward traversal via ABSTRACTS_TO edges to find abstractions
// 4) INSIGHTS: detect patterns in the traversed subgraph
func (s *Service) Reflect(ctx context.Context, req models.ReflectRequest) (models.ReflectResponse, error) {
	// Validate required fields
	if req.SpaceID == "" {
		return models.ReflectResponse{}, errors.New("space_id is required")
	}
	if req.Topic == "" {
		return models.ReflectResponse{}, errors.New("topic is required")
	}

	// Apply defaults for optional parameters
	maxDepth := req.MaxDepth
	if maxDepth <= 0 {
		maxDepth = DefaultReflectMaxDepth
	}
	if maxDepth > 5 {
		maxDepth = 5 // cap to prevent runaway traversal
	}

	maxNodes := req.MaxNodes
	if maxNodes <= 0 {
		maxNodes = DefaultReflectMaxNodes
	}
	if maxNodes > 200 {
		maxNodes = 200 // cap to prevent memory issues
	}

	// Validate embedding is provided (embeddings are generated upstream, per project design)
	if len(req.TopicEmbedding) == 0 {
		return models.ReflectResponse{}, errors.New("topic_embedding is required (wire your embedder upstream)")
	}

	// Initialize response with empty slices (not nil)
	resp := models.ReflectResponse{
		Topic:           req.Topic,
		CoreMemories:    []models.ScoredNode{},
		RelatedConcepts: []models.ScoredNode{},
		Abstractions:    []models.ScoredNode{},
		Insights:        []models.Insight{},
		GraphContext: &models.GraphContext{
			NodesExplored:   0,
			EdgesTraversed:  0,
			MaxLayerReached: 0,
		},
	}

	// Stage 1: SEED - Vector search for topic using embedding
	// Use vectorRecall to find core memories matching the topic
	// Note: Reflect doesn't support file filtering - it explores all nodes
	cands, err := s.vectorRecall(ctx, []string{req.SpaceID}, req.TopicEmbedding, DefaultReflectSeedK, FileFilter{})
	if err != nil {
		return models.ReflectResponse{}, err
	}

	// Initialize visited set with core memory node IDs
	visited := make(map[string]struct{}, len(cands))

	// Convert Candidate to ScoredNode with Distance=0 for core memories
	for _, c := range cands {
		visited[c.NodeID] = struct{}{}
		resp.CoreMemories = append(resp.CoreMemories, models.ScoredNode{
			NodeID:   c.NodeID,
			Name:     c.Name,
			Path:     c.Path,
			Summary:  c.Summary,
			Layer:    0, // Will be updated when we fetch node layer info
			Score:    c.VectorSim,
			Distance: 0, // Core memories are at distance 0
		})
	}

	// Update graph context with nodes explored
	resp.GraphContext.NodesExplored = len(cands)

	// If no core memories found, return early with empty response
	if len(cands) == 0 {
		return resp, nil
	}

	// Stage 2: EXPAND - Lateral traversal via CO_ACTIVATED_WITH and ASSOCIATED_WITH
	// BFS traversal from core memories to find related concepts
	frontier := make([]string, 0, len(cands))
	for _, c := range cands {
		frontier = append(frontier, c.NodeID)
	}

	// Track distance from seed for each node
	nodeDistance := make(map[string]int, len(visited))
	for id := range visited {
		nodeDistance[id] = 0 // core memories are at distance 0
	}

	// Track total edges traversed
	totalEdgesTraversed := 0

	// BFS expansion up to maxDepth
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		// Check if we've reached maxNodes limit
		if len(visited) >= maxNodes {
			break
		}

		// Fetch lateral edges for frontier nodes
		lateralEdges, err := s.fetchLateralEdges(ctx, req.SpaceID, frontier)
		if err != nil {
			return models.ReflectResponse{}, err
		}
		totalEdgesTraversed += len(lateralEdges)

		// Collect new nodes discovered at this depth
		nextFrontier := make([]string, 0)
		for _, e := range lateralEdges {
			// Check both directions - the edge might point to or from a frontier node
			targetID := ""
			if _, ok := visited[e.Src]; ok {
				targetID = e.Dst
			} else if _, ok := visited[e.Dst]; ok {
				targetID = e.Src
			}

			if targetID == "" {
				continue // neither end is in visited set
			}

			// Skip if already visited
			if _, ok := visited[targetID]; ok {
				continue
			}

			// Check maxNodes limit
			if len(visited) >= maxNodes {
				break
			}

			// Mark as visited and record distance
			visited[targetID] = struct{}{}
			nodeDistance[targetID] = depth
			nextFrontier = append(nextFrontier, targetID)
		}

		frontier = nextFrontier
	}

	// Fetch node metadata for all related concepts (nodes found during EXPAND, excluding core memories)
	relatedIDs := make([]string, 0, len(visited)-len(cands))
	for id := range visited {
		isCoreMemory := false
		for _, c := range cands {
			if c.NodeID == id {
				isCoreMemory = true
				break
			}
		}
		if !isCoreMemory {
			relatedIDs = append(relatedIDs, id)
		}
	}

	if len(relatedIDs) > 0 {
		nodeMetadata, err := s.fetchNodeMetadata(ctx, req.SpaceID, relatedIDs)
		if err != nil {
			return models.ReflectResponse{}, err
		}

		// Convert to ScoredNode and add to RelatedConcepts
		for _, meta := range nodeMetadata {
			dist := nodeDistance[meta.NodeID]
			// Score decays with distance (closer = higher score)
			score := 1.0 / float64(1+dist)
			resp.RelatedConcepts = append(resp.RelatedConcepts, models.ScoredNode{
				NodeID:   meta.NodeID,
				Name:     meta.Name,
				Path:     meta.Path,
				Summary:  meta.Summary,
				Layer:    meta.Layer,
				Score:    score,
				Distance: dist,
			})
		}
	}

	// Update graph context
	resp.GraphContext.NodesExplored = len(visited)
	resp.GraphContext.EdgesTraversed = totalEdgesTraversed

	// Stage 3: ABSTRACT - Upward traversal via ABSTRACTS_TO edges
	// Collect all explored node IDs for abstraction traversal
	exploredNodeIDs := make([]string, 0, len(visited))
	for id := range visited {
		exploredNodeIDs = append(exploredNodeIDs, id)
	}

	// Track abstractions separately from visited (abstractions can be re-discovered via different paths)
	abstractionIDs := make(map[string]struct{})
	abstractionDistance := make(map[string]int)
	maxLayerReached := 0

	// BFS upward traversal via ABSTRACTS_TO edges
	abstractionFrontier := exploredNodeIDs
	for depth := 1; depth <= maxDepth && len(abstractionFrontier) > 0; depth++ {
		// Check if we've hit maxNodes limit (counting visited + abstractions)
		if len(visited)+len(abstractionIDs) >= maxNodes {
			break
		}

		// Fetch upward ABSTRACTS_TO edges for frontier nodes
		abstractionEdges, err := s.fetchAbstractionEdges(ctx, req.SpaceID, abstractionFrontier)
		if err != nil {
			return models.ReflectResponse{}, err
		}
		totalEdgesTraversed += len(abstractionEdges)

		// Collect new abstraction nodes discovered at this depth
		nextAbstractionFrontier := make([]string, 0)
		for _, e := range abstractionEdges {
			// ABSTRACTS_TO edges point from concrete (src) to abstract (dst)
			// We're traversing upward, so we want the destination
			targetID := e.Dst

			// Skip if already found as an abstraction
			if _, ok := abstractionIDs[targetID]; ok {
				continue
			}

			// Skip if already in visited set (core memories or related concepts)
			if _, ok := visited[targetID]; ok {
				continue
			}

			// Check maxNodes limit
			if len(visited)+len(abstractionIDs) >= maxNodes {
				break
			}

			// Mark as discovered abstraction and record distance
			abstractionIDs[targetID] = struct{}{}
			abstractionDistance[targetID] = depth
			nextAbstractionFrontier = append(nextAbstractionFrontier, targetID)
		}

		abstractionFrontier = nextAbstractionFrontier
	}

	// Fetch metadata for all abstraction nodes
	if len(abstractionIDs) > 0 {
		absIDs := make([]string, 0, len(abstractionIDs))
		for id := range abstractionIDs {
			absIDs = append(absIDs, id)
		}

		absMetadata, err := s.fetchNodeMetadata(ctx, req.SpaceID, absIDs)
		if err != nil {
			return models.ReflectResponse{}, err
		}

		// Convert to ScoredNode and add to Abstractions
		for _, meta := range absMetadata {
			dist := abstractionDistance[meta.NodeID]
			// Score decays with distance, but abstractions get a boost based on layer
			// Higher layer = more abstract = potentially more valuable for context
			layerBoost := 1.0 + float64(meta.Layer)*0.1
			score := layerBoost / float64(1+dist)
			resp.Abstractions = append(resp.Abstractions, models.ScoredNode{
				NodeID:   meta.NodeID,
				Name:     meta.Name,
				Path:     meta.Path,
				Summary:  meta.Summary,
				Layer:    meta.Layer,
				Score:    score,
				Distance: dist,
			})

			// Track max layer reached
			if meta.Layer > maxLayerReached {
				maxLayerReached = meta.Layer
			}
		}
	}

	// Also check layer of related concepts for max layer
	for _, rc := range resp.RelatedConcepts {
		if rc.Layer > maxLayerReached {
			maxLayerReached = rc.Layer
		}
	}

	// Update final graph context
	resp.GraphContext.NodesExplored = len(visited) + len(abstractionIDs)
	resp.GraphContext.EdgesTraversed = totalEdgesTraversed
	resp.GraphContext.MaxLayerReached = maxLayerReached

	// Stage 4: INSIGHT GENERATION
	// Collect all explored node IDs for insight detection
	allExploredIDs := make([]string, 0, len(visited)+len(abstractionIDs))
	for id := range visited {
		allExploredIDs = append(allExploredIDs, id)
	}
	for id := range abstractionIDs {
		allExploredIDs = append(allExploredIDs, id)
	}

	// Generate insights from the explored subgraph
	insights, err := s.generateInsights(ctx, req.SpaceID, allExploredIDs)
	if err != nil {
		// Log error but don't fail the entire request - insights are optional
		// Just return empty insights
		resp.Insights = []models.Insight{}
	} else {
		resp.Insights = insights
	}

	return resp, nil
}

// LateralEdge represents an edge used for lateral traversal in reflection
type LateralEdge struct {
	Src     string
	Dst     string
	RelType string
	Weight  float64
}

// NodeMetadata holds basic metadata for a memory node
type NodeMetadata struct {
	NodeID  string
	Name    string
	Path    string
	Summary string
	Layer   int
}

// fetchLateralEdges fetches CO_ACTIVATED_WITH and ASSOCIATED_WITH edges for the given node IDs
func (s *Service) fetchLateralEdges(ctx context.Context, spaceID string, nodeIDs []string) ([]LateralEdge, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
		"maxNbr":  s.cfg.MaxNeighborsPerNode,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Query for lateral edges (CO_ACTIVATED_WITH and ASSOCIATED_WITH)
		// We query both outgoing and incoming edges to ensure bidirectional traversal
		cypher := `UNWIND $nodeIds AS sid
MATCH (src:MemoryNode {space_id:$spaceId, node_id:sid})
CALL {
  WITH src
  MATCH (src)-[r:CO_ACTIVATED_WITH|ASSOCIATED_WITH]-(dst:MemoryNode {space_id:$spaceId})
  WHERE coalesce(r.status,'active')='active'
  RETURN src.node_id AS s, dst.node_id AS d, type(r) AS t,
         coalesce(r.weight,0.0) AS w
  ORDER BY w DESC
  LIMIT $maxNbr
}
RETURN DISTINCT s, d, t, w`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]LateralEdge, 0, 256)
		for res.Next(ctx) {
			rec := res.Record()
			s, _ := rec.Get("s")
			d, _ := rec.Get("d")
			t, _ := rec.Get("t")
			w, _ := rec.Get("w")

			e := LateralEdge{
				Src:     fmt.Sprint(s),
				Dst:     fmt.Sprint(d),
				RelType: fmt.Sprint(t),
				Weight:  toFloat64(w, 0),
			}
			edges = append(edges, e)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return edges, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]LateralEdge), nil
}

// fetchNodeMetadata fetches basic metadata for a list of node IDs
func (s *Service) fetchNodeMetadata(ctx context.Context, spaceID string, nodeIDs []string) ([]NodeMetadata, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `UNWIND $nodeIds AS nid
MATCH (n:MemoryNode {space_id:$spaceId, node_id:nid})
RETURN n.node_id AS node_id,
       coalesce(n.name,'') AS name,
       coalesce(n.path,'') AS path,
       coalesce(n.summary,'') AS summary,
       coalesce(n.layer,0) AS layer`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		nodes := make([]NodeMetadata, 0, len(nodeIDs))
		for res.Next(ctx) {
			rec := res.Record()
			nid, _ := rec.Get("node_id")
			name, _ := rec.Get("name")
			path, _ := rec.Get("path")
			summary, _ := rec.Get("summary")
			layer, _ := rec.Get("layer")

			meta := NodeMetadata{
				NodeID:  fmt.Sprint(nid),
				Name:    fmt.Sprint(name),
				Path:    fmt.Sprint(path),
				Summary: fmt.Sprint(summary),
				Layer:   int(toFloat64(layer, 0)),
			}
			nodes = append(nodes, meta)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return nodes, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]NodeMetadata), nil
}

// AbstractionEdge represents an ABSTRACTS_TO edge for upward traversal
type AbstractionEdge struct {
	Src     string
	Dst     string
	Weight  float64
}

// fetchAbstractionEdges fetches ABSTRACTS_TO edges for upward traversal from the given node IDs
// ABSTRACTS_TO edges point from concrete nodes (src) to abstract nodes (dst)
func (s *Service) fetchAbstractionEdges(ctx context.Context, spaceID string, nodeIDs []string) ([]AbstractionEdge, error) {
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
		"maxNbr":  s.cfg.MaxNeighborsPerNode,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Query for ABSTRACTS_TO edges (upward traversal to higher layers)
		// We follow outgoing ABSTRACTS_TO edges from concrete to abstract
		cypher := `UNWIND $nodeIds AS sid
MATCH (src:MemoryNode {space_id:$spaceId, node_id:sid})
CALL {
  WITH src
  MATCH (src)-[r:ABSTRACTS_TO]->(dst:MemoryNode {space_id:$spaceId})
  WHERE coalesce(r.status,'active')='active'
  RETURN src.node_id AS s, dst.node_id AS d,
         coalesce(r.weight,0.0) AS w
  ORDER BY w DESC
  LIMIT $maxNbr
}
RETURN DISTINCT s, d, w`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]AbstractionEdge, 0, 128)
		for res.Next(ctx) {
			rec := res.Record()
			s, _ := rec.Get("s")
			d, _ := rec.Get("d")
			w, _ := rec.Get("w")

			e := AbstractionEdge{
				Src:    fmt.Sprint(s),
				Dst:    fmt.Sprint(d),
				Weight: toFloat64(w, 0),
			}
			edges = append(edges, e)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return edges, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]AbstractionEdge), nil
}

// generateInsights detects patterns in the explored subgraph
// Returns a list of insights including clusters, patterns, and gaps
func (s *Service) generateInsights(ctx context.Context, spaceID string, nodeIDs []string) ([]models.Insight, error) {
	if len(nodeIDs) < 3 {
		// Need at least 3 nodes for meaningful insight detection
		return []models.Insight{}, nil
	}

	// Fetch all edges within the explored node set for analysis
	edges, err := s.fetchEdgesAmongNodes(ctx, spaceID, nodeIDs)
	if err != nil {
		return nil, err
	}

	insights := make([]models.Insight, 0)

	// Cluster detection: find groups of 3+ nodes with mutual edges
	clusters := detectClusters(nodeIDs, edges)
	for _, cluster := range clusters {
		insights = append(insights, models.Insight{
			Type:        "cluster",
			Description: fmt.Sprintf("Found cluster of %d densely connected nodes", len(cluster)),
			NodeIDs:     cluster,
		})
	}

	// Pattern detection: identify dominant edge types
	patterns := detectPatterns(edges)
	insights = append(insights, patterns...)

	// Gap detection: find core memories that aren't connected
	gaps := detectGaps(nodeIDs, edges)
	insights = append(insights, gaps...)

	return insights, nil
}

// InsightEdge represents an edge for insight analysis
type InsightEdge struct {
	Src     string
	Dst     string
	RelType string
	Weight  float64
}

// fetchEdgesAmongNodes fetches all edges between the given node IDs
// Used for insight generation to analyze the subgraph structure
func (s *Service) fetchEdgesAmongNodes(ctx context.Context, spaceID string, nodeIDs []string) ([]InsightEdge, error) {
	if len(nodeIDs) == 0 {
		return []InsightEdge{}, nil
	}

	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	params := map[string]any{
		"spaceId": spaceID,
		"nodeIds": nodeIDs,
	}

	outAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Query for all edges between the specified nodes
		// This includes CO_ACTIVATED_WITH, ASSOCIATED_WITH, ABSTRACTS_TO, etc.
		cypher := `MATCH (src:MemoryNode {space_id:$spaceId})-[r]-(dst:MemoryNode {space_id:$spaceId})
WHERE src.node_id IN $nodeIds AND dst.node_id IN $nodeIds
  AND src.node_id < dst.node_id
  AND coalesce(r.status,'active')='active'
RETURN DISTINCT src.node_id AS s, dst.node_id AS d, type(r) AS t,
       coalesce(r.weight,0.0) AS w`
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		edges := make([]InsightEdge, 0, 256)
		for res.Next(ctx) {
			rec := res.Record()
			srcVal, _ := rec.Get("s")
			dstVal, _ := rec.Get("d")
			typeVal, _ := rec.Get("t")
			weightVal, _ := rec.Get("w")

			e := InsightEdge{
				Src:     fmt.Sprint(srcVal),
				Dst:     fmt.Sprint(dstVal),
				RelType: fmt.Sprint(typeVal),
				Weight:  toFloat64(weightVal, 0),
			}
			edges = append(edges, e)
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return edges, nil
	})
	if err != nil {
		return nil, err
	}
	return outAny.([]InsightEdge), nil
}

// detectClusters finds groups of 3+ nodes that form densely connected subgraphs
// Uses triangle detection as the basis for identifying clusters
func detectClusters(nodeIDs []string, edges []InsightEdge) [][]string {
	if len(nodeIDs) < 3 || len(edges) < 3 {
		return nil
	}

	// Build adjacency map for fast lookups
	nodeSet := make(map[string]struct{}, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = struct{}{}
	}

	// Adjacency list: node -> set of neighbors
	adj := make(map[string]map[string]struct{})
	for _, id := range nodeIDs {
		adj[id] = make(map[string]struct{})
	}

	// Populate adjacency list from edges
	for _, e := range edges {
		if _, ok := nodeSet[e.Src]; !ok {
			continue
		}
		if _, ok := nodeSet[e.Dst]; !ok {
			continue
		}
		adj[e.Src][e.Dst] = struct{}{}
		adj[e.Dst][e.Src] = struct{}{}
	}

	// Find all triangles (3-cliques)
	triangles := make([][]string, 0)
	processed := make(map[string]struct{})

	for _, a := range nodeIDs {
		for b := range adj[a] {
			if _, done := processed[b]; done {
				continue
			}
			for c := range adj[a] {
				if c == b {
					continue
				}
				if _, done := processed[c]; done {
					continue
				}
				// Check if b and c are connected
				if _, connected := adj[b][c]; connected {
					// Found a triangle: a, b, c
					tri := []string{a, b, c}
					// Sort for consistent ordering
					sortStrings(tri)
					triangles = append(triangles, tri)
				}
			}
		}
		processed[a] = struct{}{}
	}

	// Deduplicate triangles
	triangles = deduplicateTriangles(triangles)

	// Merge overlapping triangles into larger clusters
	clusters := mergeOverlappingClusters(triangles)

	// Filter to return only clusters with 3+ nodes
	result := make([][]string, 0)
	for _, cluster := range clusters {
		if len(cluster) >= 3 {
			result = append(result, cluster)
		}
	}

	return result
}

// sortStrings sorts a slice of strings in place
func sortStrings(s []string) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// deduplicateTriangles removes duplicate triangles
func deduplicateTriangles(triangles [][]string) [][]string {
	seen := make(map[string]struct{})
	result := make([][]string, 0, len(triangles))

	for _, tri := range triangles {
		key := tri[0] + "|" + tri[1] + "|" + tri[2]
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, tri)
		}
	}

	return result
}

// mergeOverlappingClusters merges triangles that share 2+ nodes into larger clusters
func mergeOverlappingClusters(triangles [][]string) [][]string {
	if len(triangles) == 0 {
		return nil
	}

	// Use union-find to merge overlapping triangles
	// Start with each triangle as its own cluster
	clusters := make([]map[string]struct{}, len(triangles))
	for i, tri := range triangles {
		clusters[i] = make(map[string]struct{})
		for _, id := range tri {
			clusters[i][id] = struct{}{}
		}
	}

	// Merge clusters that share 2+ nodes
	merged := true
	for merged {
		merged = false
		for i := 0; i < len(clusters); i++ {
			for j := i + 1; j < len(clusters); j++ {
				// Count shared nodes
				sharedCount := 0
				for id := range clusters[i] {
					if _, exists := clusters[j][id]; exists {
						sharedCount++
					}
				}

				// Merge if 2+ shared nodes
				if sharedCount >= 2 {
					// Merge j into i
					for id := range clusters[j] {
						clusters[i][id] = struct{}{}
					}
					// Remove j
					clusters = append(clusters[:j], clusters[j+1:]...)
					merged = true
					break
				}
			}
			if merged {
				break
			}
		}
	}

	// Convert back to slices
	result := make([][]string, 0, len(clusters))
	for _, cluster := range clusters {
		ids := make([]string, 0, len(cluster))
		for id := range cluster {
			ids = append(ids, id)
		}
		sortStrings(ids)
		result = append(result, ids)
	}

	return result
}

// detectPatterns analyzes edge type distribution to identify dominant patterns
func detectPatterns(edges []InsightEdge) []models.Insight {
	if len(edges) == 0 {
		return nil
	}

	// Count edge types
	typeCounts := make(map[string]int)
	for _, e := range edges {
		typeCounts[e.RelType]++
	}

	insights := make([]models.Insight, 0)
	totalEdges := len(edges)

	// Check for dominant edge type (> 50%)
	for relType, count := range typeCounts {
		percentage := float64(count) / float64(totalEdges) * 100
		if percentage > 50 {
			insights = append(insights, models.Insight{
				Type:        "pattern",
				Description: fmt.Sprintf("Dominant relationship: %s (%.0f%% of edges)", relType, percentage),
				NodeIDs:     []string{}, // Pattern applies to the whole subgraph
			})
		}
	}

	return insights
}

// detectGaps identifies potential missing connections between core memories
// If we have nodes that should be connected but aren't, flag as a gap
func detectGaps(nodeIDs []string, edges []InsightEdge) []models.Insight {
	if len(nodeIDs) < 2 {
		return nil
	}

	// Build adjacency set for fast lookups
	connected := make(map[string]struct{})
	for _, e := range edges {
		// Create bidirectional key
		key1 := e.Src + "|" + e.Dst
		key2 := e.Dst + "|" + e.Src
		connected[key1] = struct{}{}
		connected[key2] = struct{}{}
	}

	// Find isolated nodes (nodes with no edges to other explored nodes)
	isolatedNodes := make([]string, 0)
	nodeEdgeCount := make(map[string]int)

	for _, e := range edges {
		nodeEdgeCount[e.Src]++
		nodeEdgeCount[e.Dst]++
	}

	for _, id := range nodeIDs {
		if nodeEdgeCount[id] == 0 {
			isolatedNodes = append(isolatedNodes, id)
		}
	}

	insights := make([]models.Insight, 0)

	// If we have isolated nodes, flag as a gap
	if len(isolatedNodes) > 0 && len(isolatedNodes) < len(nodeIDs) {
		insights = append(insights, models.Insight{
			Type:        "gap",
			Description: fmt.Sprintf("%d node(s) have no connections to other explored concepts", len(isolatedNodes)),
			NodeIDs:     isolatedNodes,
		})
	}

	return insights
}
