package cli

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/cli/neo4jutil"
	"mdemg/internal/config"
)

// newPruneCmd creates the prune command
func newPruneCmd() *cobra.Command {
	cfg := pruneConfig{}

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune weak edges, tombstone orphan nodes, and merge redundant nodes",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config (YAML → .env → env vars)
			if cfgPath := config.FindConfigFile(); cfgPath != "" {
				_ = config.LoadYAMLConfig(cfgPath)
			}
			_ = godotenv.Load()

			// Environment variables for Neo4j connection
			get := func(k, def string) string {
				v := strings.TrimSpace(os.Getenv(k))
				if v == "" {
					return def
				}
				return v
			}

			cfg.Neo4jURI = get("NEO4J_URI", "")
			cfg.Neo4jUser = get("NEO4J_USER", "")
			cfg.Neo4jPass = get("NEO4J_PASS", "")

			if cfg.Neo4jURI == "" || cfg.Neo4jUser == "" || cfg.Neo4jPass == "" {
				return errors.New("NEO4J_URI/NEO4J_USER/NEO4J_PASS environment variables are required")
			}

			// Resolve space ID from flag / global flag / env var
			if cfg.SpaceID == "" {
				cfg.SpaceID = resolveSpaceID(cmd)
			}
			if cfg.SpaceID == "" {
				return errors.New("--space-id is required (or set MDEMG_SPACE_ID)")
			}

			// Validate flag values
			if cfg.WeightThreshold < 0 || cfg.WeightThreshold > 1 {
				return errors.New("weight-threshold must be between 0 and 1")
			}
			if cfg.MinEvidence < 0 {
				return errors.New("min-evidence must be non-negative")
			}
			if cfg.OlderThanDays < 0 {
				return errors.New("older-than-days must be non-negative")
			}
			if cfg.RetentionDays < 0 {
				return errors.New("retention-days must be non-negative")
			}
			if cfg.MaxDegree < 0 {
				return errors.New("max-degree must be non-negative")
			}
			if cfg.SimilarityThreshold < 0 || cfg.SimilarityThreshold > 1 {
				return errors.New("similarity-threshold must be between 0 and 1")
			}
			if cfg.BatchSize <= 0 {
				return errors.New("batch-size must be positive")
			}

			ctx := context.Background()

			driver, err := newPruneDriver(cfg)
			if err != nil {
				return fmt.Errorf("failed to create neo4j driver: %w", err)
			}
			defer driver.Close(ctx)

			// Verify connectivity
			if err := driver.VerifyConnectivity(ctx); err != nil {
				return fmt.Errorf("failed to connect to neo4j: %w", err)
			}

			// Run the prune job
			return runPruneJob(ctx, driver, cfg)
		},
	}

	// Edge pruning parameters
	cmd.Flags().Float64Var(&cfg.WeightThreshold, "weight-threshold", 0.01, "Minimum weight to keep (below = prune candidate)")
	cmd.Flags().IntVar(&cfg.MinEvidence, "min-evidence", 3, "Minimum evidence_count to protect from pruning")
	cmd.Flags().IntVar(&cfg.OlderThanDays, "older-than-days", 30, "Only prune edges older than N days")

	// Node tombstoning parameters
	cmd.Flags().IntVar(&cfg.RetentionDays, "retention-days", 90, "Days without observation to tombstone")
	cmd.Flags().IntVar(&cfg.MaxDegree, "max-degree", 1, "Max edges for orphan detection")

	// Node merging parameters
	cmd.Flags().Float64Var(&cfg.SimilarityThreshold, "similarity-threshold", 0.98, "Vector similarity threshold for merge")
	cmd.Flags().BoolVar(&cfg.MergeEnabled, "merge-enabled", false, "Enable node merging (more destructive)")
	cmd.Flags().StringVar(&cfg.VectorIndexName, "vector-index", "memNodeEmbedding", "Vector index name for similarity search")

	// Processing options
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", true, "Preview mode - no modifications (default: true)")
	cmd.Flags().StringVar(&cfg.SpaceID, "space-id", "", "Space ID to process (or set MDEMG_SPACE_ID)")
	cmd.Flags().IntVar(&cfg.BatchSize, "batch-size", 1000, "Process items in batches of this size")

	return cmd
}

// pruneEdge represents a relationship in the graph for prune processing
type pruneEdge struct {
	ID            int64
	RelType       string
	SourceID      string
	TargetID      string
	Weight        float64
	EvidenceCount int
	Pinned        bool
	UpdatedAt     time.Time
}

// edgePruneResult holds the result of pruning evaluation for a single edge
type edgePruneResult struct {
	Edge          pruneEdge
	ShouldPrune   bool
	Protected     bool
	ProtectReason string // "pinned", "high_evidence", or ""
}

// node represents a MemoryNode in the graph for prune processing
type node struct {
	NodeID              string
	Degree              int       // Total number of edges (in + out)
	LastObservationTime time.Time // Most recent observation timestamp
	InAbstractionChain  bool      // Has ABSTRACTS_TO or INSTANTIATES relationships
	Status              string    // Current status (active, tombstoned, etc.)
}

// nodeTombstoneResult holds the result of tombstoning evaluation for a single node
type nodeTombstoneResult struct {
	Node            node
	ShouldTombstone bool
	Protected       bool
	ProtectReason   string // "high_degree", "recent_observation", "abstraction_chain", or ""
}

// pruneConfig holds CLI and environment configuration for the pruning job
type pruneConfig struct {
	// Neo4j connection
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string

	// Edge pruning parameters
	WeightThreshold float64 // default: 0.01
	MinEvidence     int     // default: 3
	OlderThanDays   int     // default: 30

	// Node tombstoning parameters
	RetentionDays int // default: 90
	MaxDegree     int // default: 1

	// Node merging parameters
	SimilarityThreshold float64 // default: 0.98
	MergeEnabled        bool    // default: false
	VectorIndexName     string  // default: memNodeEmbedding

	// Processing options
	DryRun    bool
	SpaceID   string // REQUIRED
	BatchSize int    // default: 1000
}

// pruneStats tracks statistics for the prune job
type pruneStats struct {
	// Edge pruning stats
	edgesScanned   int
	edgesPruned    int
	edgesProtected int

	// Node tombstoning stats
	nodesScanned    int
	nodesTombstoned int
	nodesProtected  int

	// Node merging stats
	mergesPerformed int
	nodesMerged     int
}

// edgePruneStats holds edge pruning statistics
type edgePruneStats struct {
	scanned   int
	pruned    int
	protected int
}

// nodeTombstoneStats holds node tombstoning statistics
type nodeTombstoneStats struct {
	scanned    int
	tombstoned int
	protected  int
}

// nodeMergeStats holds node merging statistics
type nodeMergeStats struct {
	merges int
	merged int
}

// mergePair represents a pair of nodes that should be merged.
// The survivor is the older node (by created_at) which will remain,
// and the merged node will be tombstoned after transferring its edges/observations.
type mergePair struct {
	SurvivorID        string
	MergedID          string
	Similarity        float64
	SurvivorCreatedAt time.Time
	MergedCreatedAt   time.Time
}

// mergeCandidate represents a node with its embedding for merge candidate search
type mergeCandidate struct {
	NodeID    string
	Layer     int
	Embedding []float32
	CreatedAt time.Time
}

// similarNode represents a node similar to a query node
type similarNode struct {
	NodeID     string
	Layer      int
	CreatedAt  time.Time
	Similarity float64
}

// unionFind implements a union-find (disjoint-set) data structure
// for resolving transitive merge chains.
// The key is a node ID, parent maps each node to its parent,
// and nodeInfo stores the creation time for each node to determine the oldest.
type unionFind struct {
	parent   map[string]string
	rank     map[string]int
	nodeInfo map[string]time.Time // node_id -> created_at
}

// newPruneDriver creates a new Neo4j driver with the given configuration
func newPruneDriver(cfg pruneConfig) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		return nil, err
	}
	return driver, nil
}

// runPruneJob executes the pruning operations
func runPruneJob(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig) error {
	// Print header
	printHeader(cfg)

	fmt.Println("\nProcessing...")

	stats := pruneStats{}

	// Step 1: Edge pruning
	fmt.Println("\nStep 1: Edge pruning...")
	edgeStats, err := pruneEdges(ctx, driver, cfg)
	if err != nil {
		return fmt.Errorf("edge pruning: %w", err)
	}
	stats.edgesScanned = edgeStats.scanned
	stats.edgesPruned = edgeStats.pruned
	stats.edgesProtected = edgeStats.protected

	// Step 2: Orphan node tombstoning
	fmt.Println("\nStep 2: Orphan node tombstoning...")
	nodeStats, err := tombstoneOrphans(ctx, driver, cfg)
	if err != nil {
		return fmt.Errorf("orphan tombstoning: %w", err)
	}
	stats.nodesScanned = nodeStats.scanned
	stats.nodesTombstoned = nodeStats.tombstoned
	stats.nodesProtected = nodeStats.protected

	// Step 3: Node merging (if enabled)
	if cfg.MergeEnabled {
		fmt.Println("\nStep 3: Redundant node merging...")
		mergeStats, err := mergeRedundantNodes(ctx, driver, cfg)
		if err != nil {
			return fmt.Errorf("node merging: %w", err)
		}
		stats.mergesPerformed = mergeStats.merges
		stats.nodesMerged = mergeStats.merged
	} else {
		fmt.Println("\nStep 3: Node merging... (skipped, --merge-enabled=false)")
	}

	// Print statistics
	printStats(stats, cfg)

	return nil
}

// shouldPruneEdge determines if an edge should be pruned based on pruning rules.
// From docs/07_Consolidation_and_Pruning.md Section 5.1:
// Prune edges if ALL are true:
// - weight < weight_threshold
// - evidence_count < min_evidence
// - updated_at older than olderThanDays
// - edge not pinned
// Special case: weight <= 0 always marks for pruning (regardless of other factors)
func shouldPruneEdge(e pruneEdge, weightThreshold float64, minEvidence int, olderThanDays int, now time.Time) (prune bool, protected bool, reason string) {
	// Special case: zero or negative weight always pruned
	if e.Weight <= 0 {
		return true, false, ""
	}

	// Check if weight is below threshold
	if e.Weight >= weightThreshold {
		// Weight is high enough, no need to prune
		return false, false, ""
	}

	// Weight is below threshold - check protection rules

	// Protection: pinned edges are never pruned
	if e.Pinned {
		return false, true, "pinned"
	}

	// Protection: high evidence count protects the edge
	if e.EvidenceCount >= minEvidence {
		return false, true, "high_evidence"
	}

	// Check age criterion: edge must be old enough to prune
	if !isOlderThan(e.UpdatedAt, olderThanDays, now) {
		// Edge is too recent, don't prune
		return false, false, ""
	}

	// All prune conditions met: weight < threshold AND evidence < min AND age > days AND !pinned
	return true, false, ""
}

// isOlderThan checks if the given timestamp is older than the specified number of days.
// Returns true if updatedAt is older than olderThanDays, false otherwise.
// If updatedAt is zero (unset), treats as very old (should be pruned).
func isOlderThan(updatedAt time.Time, olderThanDays int, now time.Time) bool {
	if updatedAt.IsZero() {
		// No timestamp means very old, consider it old enough
		return true
	}
	cutoff := now.AddDate(0, 0, -olderThanDays)
	return updatedAt.Before(cutoff)
}

// processEdgeForPruning evaluates a single edge for pruning
func processEdgeForPruning(e pruneEdge, cfg pruneConfig, now time.Time) edgePruneResult {
	prune, protected, reason := shouldPruneEdge(e, cfg.WeightThreshold, cfg.MinEvidence, cfg.OlderThanDays, now)

	return edgePruneResult{
		Edge:          e,
		ShouldPrune:   prune,
		Protected:     protected,
		ProtectReason: reason,
	}
}

// shouldTombstoneNode determines if a node should be tombstoned based on pruning rules.
// From docs/07_Consolidation_and_Pruning.md Section 5.2:
// Tombstone nodes if ALL are true:
// - degree <= maxDegree (low connectivity, orphan-like)
// - no observations within retentionDays (no recent activity)
// - not part of any abstraction chain (no ABSTRACTS_TO/INSTANTIATES relationships)
//
// Protection rules (node will NOT be tombstoned):
// - If degree > maxDegree (well-connected node)
// - If has observation within retention window (recently active)
// - If part of abstraction chain (structural importance)
// - If already tombstoned (no need to re-tombstone)
func shouldTombstoneNode(n node, maxDegree int, retentionDays int, now time.Time) (tombstone bool, protected bool, reason string) {
	// Skip already tombstoned nodes
	if n.Status == "tombstoned" {
		return false, false, ""
	}

	// Protection: nodes in abstraction chains are never tombstoned
	// These are structurally important for the concept hierarchy
	if n.InAbstractionChain {
		return false, true, "abstraction_chain"
	}

	// Protection: nodes with high degree are well-connected and valuable
	if n.Degree > maxDegree {
		return false, true, "high_degree"
	}

	// Protection: nodes with recent observations show active use
	if hasRecentObservation(n.LastObservationTime, retentionDays, now) {
		return false, true, "recent_observation"
	}

	// All tombstone conditions met:
	// - degree <= maxDegree (orphan-like)
	// - no recent observations
	// - not in abstraction chain
	return true, false, ""
}

// hasRecentObservation checks if the node has an observation within the retention window.
// Returns true if the lastObservationTime is within retentionDays of now.
// If lastObservationTime is zero (no observations), returns false.
func hasRecentObservation(lastObservationTime time.Time, retentionDays int, now time.Time) bool {
	if lastObservationTime.IsZero() {
		// No observations at all means no recent observation
		return false
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	return lastObservationTime.After(cutoff) || lastObservationTime.Equal(cutoff)
}

// processNodeForTombstoning evaluates a single node for tombstoning
func processNodeForTombstoning(n node, cfg pruneConfig, now time.Time) nodeTombstoneResult {
	tombstone, protected, reason := shouldTombstoneNode(n, cfg.MaxDegree, cfg.RetentionDays, now)

	return nodeTombstoneResult{
		Node:            n,
		ShouldTombstone: tombstone,
		Protected:       protected,
		ProtectReason:   reason,
	}
}

// pruneEdges removes weak/deprecated edges
func pruneEdges(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig) (edgePruneStats, error) {
	now := time.Now()
	stats := edgePruneStats{}
	offset := 0
	batchNum := 0

	for {
		// Query batch of edges
		edges, err := queryPruneEdgeBatch(ctx, driver, cfg, offset)
		if err != nil {
			return stats, fmt.Errorf("query batch %d: %w", batchNum+1, err)
		}

		if len(edges) == 0 {
			break
		}

		batchNum++
		fmt.Printf("  Batch %d: %d edges\n", batchNum, len(edges))

		// Collect edges to delete
		var toDelete []edgePruneResult

		for _, e := range edges {
			stats.scanned++

			result := processEdgeForPruning(e, cfg, now)

			// Track protection stats
			if result.Protected {
				stats.protected++
			}

			// Collect edges to prune
			if result.ShouldPrune {
				toDelete = append(toDelete, result)
			}
		}

		// Apply changes if not dry-run
		if !cfg.DryRun {
			for _, r := range toDelete {
				if err := deletePruneEdge(ctx, driver, r.Edge.ID); err != nil {
					return stats, fmt.Errorf("delete edge %d: %w", r.Edge.ID, err)
				}
			}
		}

		stats.pruned += len(toDelete)

		offset += len(edges)

		// If we got fewer than batch size, we're done
		if len(edges) < cfg.BatchSize {
			break
		}
	}

	return stats, nil
}

// deletePruneEdge removes an edge from the database
func deletePruneEdge(ctx context.Context, driver neo4j.DriverWithContext, edgeID int64) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH ()-[r]->()
WHERE id(r) = $edgeId
DELETE r`

		params := map[string]any{
			"edgeId": edgeID,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		// Consume result
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})

	return err
}

// queryEdgeBatch fetches a batch of edges from Neo4j for prune processing.
// It filters by space_id (required) and age threshold (older than N days).
func queryPruneEdgeBatch(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig, offset int) ([]pruneEdge, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Build the Cypher query with space filtering
	// For pruning, we use updated_at to determine edge age
	// Note: space_id is REQUIRED for prune operations
	cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE r.space_id = $spaceId
  AND (
    (r.updated_at IS NOT NULL AND r.updated_at < datetime() - duration({days: $olderThanDays}))
    OR (r.updated_at IS NULL AND r.created_at IS NOT NULL AND r.created_at < datetime() - duration({days: $olderThanDays}))
    OR (r.updated_at IS NULL AND r.created_at IS NULL)
  )
RETURN id(r) AS edgeId,
       type(r) AS relType,
       a.node_id AS sourceId,
       b.node_id AS targetId,
       coalesce(r.weight, 0.0) AS weight,
       coalesce(r.evidence_count, 0) AS evidence,
       coalesce(r.pinned, false) AS pinned,
       coalesce(r.updated_at, r.created_at) AS updatedAt
ORDER BY updatedAt ASC
SKIP $offset
LIMIT $batchSize`

	params := map[string]any{
		"spaceId":       cfg.SpaceID,
		"olderThanDays": cfg.OlderThanDays,
		"offset":        offset,
		"batchSize":     cfg.BatchSize,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		edges := make([]pruneEdge, 0)
		for res.Next(ctx) {
			rec := res.Record()

			edgeID, _ := rec.Get("edgeId")
			relType, _ := rec.Get("relType")
			sourceID, _ := rec.Get("sourceId")
			targetID, _ := rec.Get("targetId")
			weight, _ := rec.Get("weight")
			evidence, _ := rec.Get("evidence")
			pinned, _ := rec.Get("pinned")
			updatedAt, _ := rec.Get("updatedAt")

			e := pruneEdge{
				ID:            edgeID.(int64),
				RelType:       relType.(string),
				SourceID:      neo4jutil.AsString(sourceID),
				TargetID:      neo4jutil.AsString(targetID),
				Weight:        neo4jutil.AsFloat64(weight),
				EvidenceCount: neo4jutil.AsInt(evidence),
				Pinned:        neo4jutil.AsBool(pinned),
				UpdatedAt:     neo4jutil.AsTime(updatedAt),
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
	return result.([]pruneEdge), nil
}

// tombstoneOrphans marks orphan nodes as tombstoned
func tombstoneOrphans(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig) (nodeTombstoneStats, error) {
	now := time.Now()
	stats := nodeTombstoneStats{}

	// Query all orphan candidates (nodes with low degree)
	candidates, err := queryOrphanCandidates(ctx, driver, cfg)
	if err != nil {
		return stats, fmt.Errorf("query orphan candidates: %w", err)
	}

	if len(candidates) == 0 {
		fmt.Println("  No orphan candidates found")
		return stats, nil
	}

	fmt.Printf("  Found %d orphan candidates\n", len(candidates))

	// Collect nodes to tombstone
	var toTombstone []nodeTombstoneResult

	for _, n := range candidates {
		stats.scanned++

		result := processNodeForTombstoning(n, cfg, now)

		// Track protection stats
		if result.Protected {
			stats.protected++
		}

		// Collect nodes to tombstone
		if result.ShouldTombstone {
			toTombstone = append(toTombstone, result)
		}
	}

	// Apply changes if not dry-run
	if !cfg.DryRun {
		for _, r := range toTombstone {
			if err := tombstoneNode(ctx, driver, cfg.SpaceID, r.Node.NodeID); err != nil {
				return stats, fmt.Errorf("tombstone node %s: %w", r.Node.NodeID, err)
			}
		}
	}

	stats.tombstoned = len(toTombstone)

	return stats, nil
}

// tombstoneNode marks a node as tombstoned in the database by setting status='tombstoned'
func tombstoneNode(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, nodeID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND n.node_id = $nodeId
SET n.status = 'tombstoned',
    n.tombstoned_at = datetime()
RETURN count(*) AS updated`

		params := map[string]any{
			"spaceId": spaceID,
			"nodeId":  nodeID,
		}

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		// Consume result
		for res.Next(ctx) {
			// consume
		}
		return nil, res.Err()
	})

	return err
}

// queryOrphanCandidates fetches nodes that are candidates for tombstoning.
// A node is an orphan candidate if:
// - It belongs to the specified space_id
// - It has low degree (total edges <= maxDegree)
// - It is not already tombstoned
// Returns nodes with their degree, last observation time, and abstraction chain status.
// The actual tombstoning decision is made by shouldTombstoneNode() after evaluating
// observation recency and abstraction chain membership.
func queryOrphanCandidates(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig) ([]node, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Query for nodes with low degree that are potential orphan candidates.
	// We fetch:
	// - Node basic info (node_id, status)
	// - Degree (count of all edges, both directions)
	// - Last observation time (max timestamp from HAS_OBSERVATION relationships)
	// - Whether the node is part of an abstraction chain (has ABSTRACTS_TO or INSTANTIATES)
	//
	// Note: We filter by degree in the query for efficiency, but the final
	// tombstoning decision is made in Go code by shouldTombstoneNode().
	cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND coalesce(n.status, 'active') <> 'tombstoned'
WITH n
// Count all edges (both directions) for degree calculation
OPTIONAL MATCH (n)-[e]-()
WITH n, count(DISTINCT e) AS degree
WHERE degree <= $maxDegree
// Get the most recent observation timestamp
OPTIONAL MATCH (n)-[:HAS_OBSERVATION]->(obs:Observation)
WITH n, degree, max(obs.observed_at) AS lastObsTime
// Check if node is part of abstraction chain (ABSTRACTS_TO or INSTANTIATES)
OPTIONAL MATCH (n)-[:ABSTRACTS_TO|INSTANTIATES]-()
WITH n, degree, lastObsTime, count(*) > 0 AS inAbstractionChain
RETURN n.node_id AS nodeId,
       degree,
       lastObsTime,
       inAbstractionChain,
       coalesce(n.status, 'active') AS status
ORDER BY degree ASC, lastObsTime ASC`

	params := map[string]any{
		"spaceId":   cfg.SpaceID,
		"maxDegree": cfg.MaxDegree,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		nodes := make([]node, 0)
		for res.Next(ctx) {
			rec := res.Record()

			nodeID, _ := rec.Get("nodeId")
			degree, _ := rec.Get("degree")
			lastObsTime, _ := rec.Get("lastObsTime")
			inAbstractionChain, _ := rec.Get("inAbstractionChain")
			status, _ := rec.Get("status")

			n := node{
				NodeID:              neo4jutil.AsString(nodeID),
				Degree:              neo4jutil.AsInt(degree),
				LastObservationTime: neo4jutil.AsTime(lastObsTime),
				InAbstractionChain:  neo4jutil.AsBool(inAbstractionChain),
				Status:              neo4jutil.AsString(status),
			}
			nodes = append(nodes, n)
		}

		if err := res.Err(); err != nil {
			return nil, err
		}
		return nodes, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]node), nil
}

// mergeRedundantNodes merges highly similar nodes
func mergeRedundantNodes(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig) (nodeMergeStats, error) {
	stats := nodeMergeStats{}

	// Query all merge candidates
	pairs, err := queryMergeCandidates(ctx, driver, cfg)
	if err != nil {
		return stats, fmt.Errorf("query merge candidates: %w", err)
	}

	if len(pairs) == 0 {
		fmt.Println("  No merge candidates found")
		return stats, nil
	}

	fmt.Printf("  Found %d initial merge pairs\n", len(pairs))

	// Resolve transitive chains (A->B, B->C all merge to oldest)
	resolvedPairs := resolveTransitiveMerges(pairs)
	if len(resolvedPairs) != len(pairs) {
		fmt.Printf("  Resolved to %d merge pairs after transitive chain resolution\n", len(resolvedPairs))
	}

	// Apply merges if not dry-run
	if !cfg.DryRun {
		for _, pair := range resolvedPairs {
			if err := mergeNodes(ctx, driver, cfg.SpaceID, pair.SurvivorID, pair.MergedID); err != nil {
				return stats, fmt.Errorf("merge node %s into %s: %w", pair.MergedID, pair.SurvivorID, err)
			}
		}
	}

	stats.merges = len(resolvedPairs)
	stats.merged = len(resolvedPairs) // Each pair merges one node

	return stats, nil
}

// mergeNodes transfers all edges and observations from the merged node to the survivor,
// then tombstones the merged node. This performs a complete merge operation:
// 1. Transfer outgoing edges from merged -> target to survivor -> target
// 2. Transfer incoming edges from source -> merged to source -> survivor
// 3. Transfer HAS_OBSERVATION relationships from merged to survivor
// 4. Tombstone the merged node with merged_into pointing to survivor
func mergeNodes(ctx context.Context, driver neo4j.DriverWithContext, spaceID, survivorID, mergedID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"spaceId":    spaceID,
			"survivorId": survivorID,
			"mergedId":   mergedID,
		}

		// Step 1: Transfer outgoing edges from merged node to survivor
		// Neo4j requires relationship types to be literals in CREATE, so we handle
		// each relationship type separately to preserve edge types and properties
		if err := transferEdgesManual(ctx, tx, spaceID, survivorID, mergedID, true); err != nil {
			return nil, fmt.Errorf("transfer outgoing edges: %w", err)
		}

		// Step 2: Transfer incoming edges from sources to survivor
		if err := transferEdgesManual(ctx, tx, spaceID, survivorID, mergedID, false); err != nil {
			return nil, fmt.Errorf("transfer incoming edges: %w", err)
		}

		// Step 3: Transfer HAS_OBSERVATION relationships
		obsCypher := `
MATCH (merged:MemoryNode {space_id: $spaceId, node_id: $mergedId})-[r:HAS_OBSERVATION]->(obs)
MATCH (survivor:MemoryNode {space_id: $spaceId, node_id: $survivorId})
// Check if survivor already has this observation to avoid duplicates
WHERE NOT exists((survivor)-[:HAS_OBSERVATION]->(obs))
CREATE (survivor)-[:HAS_OBSERVATION]->(obs)
DELETE r
RETURN count(*) AS transferred`

		res, err := tx.Run(ctx, obsCypher, params)
		if err != nil {
			return nil, fmt.Errorf("transfer observations: %w", err)
		}
		// Consume result
		for res.Next(ctx) {
		}
		if err := res.Err(); err != nil {
			return nil, err
		}

		// Delete any remaining HAS_OBSERVATION from merged (duplicates that weren't transferred)
		cleanupObsCypher := `
MATCH (merged:MemoryNode {space_id: $spaceId, node_id: $mergedId})-[r:HAS_OBSERVATION]->()
DELETE r
RETURN count(*) AS deleted`

		res, err = tx.Run(ctx, cleanupObsCypher, params)
		if err != nil {
			return nil, fmt.Errorf("cleanup duplicate observations: %w", err)
		}
		for res.Next(ctx) {
		}
		if err := res.Err(); err != nil {
			return nil, err
		}

		// Step 4: Tombstone the merged node
		tombstoneCypher := `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $mergedId})
SET n.status = 'tombstoned',
    n.merged_into = $survivorId,
    n.tombstoned_at = datetime()
RETURN count(*) AS tombstoned`

		res, err = tx.Run(ctx, tombstoneCypher, params)
		if err != nil {
			return nil, fmt.Errorf("tombstone merged node: %w", err)
		}
		for res.Next(ctx) {
		}
		return nil, res.Err()
	})

	return err
}

// transferEdgesManual transfers edges without APOC by handling each relationship type individually.
// If outgoing is true, transfers merged->target edges to survivor->target.
// If outgoing is false, transfers source->merged edges to source->survivor.
func transferEdgesManual(ctx context.Context, tx neo4j.ManagedTransaction, spaceID, survivorID, mergedID string, outgoing bool) error {
	// Get all relationship types that need to be transferred
	var findRelTypesCypher string
	if outgoing {
		findRelTypesCypher = `
MATCH (merged:MemoryNode {space_id: $spaceId, node_id: $mergedId})-[r]->(target)
WHERE NOT target.node_id = $survivorId
RETURN DISTINCT type(r) AS relType`
	} else {
		findRelTypesCypher = `
MATCH (source)-[r]->(merged:MemoryNode {space_id: $spaceId, node_id: $mergedId})
WHERE NOT source.node_id = $survivorId
RETURN DISTINCT type(r) AS relType`
	}

	params := map[string]any{
		"spaceId":    spaceID,
		"survivorId": survivorID,
		"mergedId":   mergedID,
	}

	res, err := tx.Run(ctx, findRelTypesCypher, params)
	if err != nil {
		return err
	}

	var relTypes []string
	for res.Next(ctx) {
		rec := res.Record()
		relType, _ := rec.Get("relType")
		if rt, ok := relType.(string); ok {
			relTypes = append(relTypes, rt)
		}
	}
	if err := res.Err(); err != nil {
		return err
	}

	// Transfer each relationship type
	for _, relType := range relTypes {
		if err := transferEdgesByType(ctx, tx, spaceID, survivorID, mergedID, relType, outgoing); err != nil {
			return fmt.Errorf("transfer %s edges: %w", relType, err)
		}
	}

	return nil
}

// transferEdgesByType transfers edges of a specific relationship type.
// This uses dynamic Cypher generation since Neo4j requires relationship types to be literals in CREATE.
func transferEdgesByType(ctx context.Context, tx neo4j.ManagedTransaction, spaceID, survivorID, mergedID, relType string, outgoing bool) error {
	// Build Cypher based on relationship type and direction
	// Note: We use string formatting for relType since Neo4j doesn't support parameterized relationship types
	var transferCypher string

	if outgoing {
		transferCypher = fmt.Sprintf(`
MATCH (merged:MemoryNode {space_id: $spaceId, node_id: $mergedId})-[r:%s]->(target)
WHERE NOT target.node_id = $survivorId
WITH merged, r, target, properties(r) AS props
MATCH (survivor:MemoryNode {space_id: $spaceId, node_id: $survivorId})
CREATE (survivor)-[nr:%s]->(target)
SET nr = props
DELETE r
RETURN count(*) AS transferred`, relType, relType)
	} else {
		transferCypher = fmt.Sprintf(`
MATCH (source)-[r:%s]->(merged:MemoryNode {space_id: $spaceId, node_id: $mergedId})
WHERE NOT source.node_id = $survivorId
WITH source, r, merged, properties(r) AS props
MATCH (survivor:MemoryNode {space_id: $spaceId, node_id: $survivorId})
CREATE (source)-[nr:%s]->(survivor)
SET nr = props
DELETE r
RETURN count(*) AS transferred`, relType, relType)
	}

	params := map[string]any{
		"spaceId":    spaceID,
		"survivorId": survivorID,
		"mergedId":   mergedID,
	}

	res, err := tx.Run(ctx, transferCypher, params)
	if err != nil {
		return err
	}

	// Consume result
	for res.Next(ctx) {
	}
	return res.Err()
}

// queryMergeCandidates finds pairs of highly similar nodes at the same layer
// that can be merged. Uses vector similarity search to find candidates.
// From spec: Nodes with vector similarity > threshold, same space_id, same layer,
// not abstraction nodes, are merged.
func queryMergeCandidates(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig) ([]mergePair, error) {
	// Step 1: Get all nodes with embeddings in this space (not tombstoned, not abstraction nodes)
	candidates, err := queryNodesWithEmbeddings(ctx, driver, cfg)
	if err != nil {
		return nil, fmt.Errorf("query nodes with embeddings: %w", err)
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Step 2: For each node, find similar nodes at the same layer
	// Track which nodes we've already paired to avoid duplicates
	paired := make(map[string]bool)
	var pairs []mergePair

	for _, candidate := range candidates {
		// Skip if this node has already been paired
		if paired[candidate.NodeID] {
			continue
		}

		// Find similar nodes for this candidate
		similarNodes, err := findSimilarNodes(ctx, driver, cfg, candidate)
		if err != nil {
			// Log error but continue with other candidates
			log.Printf("prune: error finding similar nodes for %s: %v", candidate.NodeID, err)
			continue
		}

		// Create merge pairs from similar nodes
		for _, similar := range similarNodes {
			// Skip if already paired
			if paired[similar.NodeID] {
				continue
			}

			// Determine survivor (older node) vs merged (newer node)
			var pair mergePair
			if candidate.CreatedAt.Before(similar.CreatedAt) || candidate.CreatedAt.Equal(similar.CreatedAt) {
				pair = mergePair{
					SurvivorID:        candidate.NodeID,
					MergedID:          similar.NodeID,
					Similarity:        similar.Similarity,
					SurvivorCreatedAt: candidate.CreatedAt,
					MergedCreatedAt:   similar.CreatedAt,
				}
			} else {
				pair = mergePair{
					SurvivorID:        similar.NodeID,
					MergedID:          candidate.NodeID,
					Similarity:        similar.Similarity,
					SurvivorCreatedAt: similar.CreatedAt,
					MergedCreatedAt:   candidate.CreatedAt,
				}
			}

			pairs = append(pairs, pair)

			// Mark both nodes as paired
			paired[candidate.NodeID] = true
			paired[similar.NodeID] = true

			// Once this candidate is paired, move to next candidate
			break
		}
	}

	return pairs, nil
}

// queryNodesWithEmbeddings returns all non-tombstoned nodes with embeddings
// that are not part of abstraction chains (i.e., don't have ABSTRACTS_TO relationships)
func queryNodesWithEmbeddings(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig) ([]mergeCandidate, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Query for nodes with embeddings that are not tombstoned and not abstraction nodes
	cypher := `
MATCH (n:MemoryNode)
WHERE n.space_id = $spaceId
  AND coalesce(n.status, 'active') <> 'tombstoned'
  AND n.embedding IS NOT NULL
  AND size(n.embedding) > 0
// Exclude nodes that are part of abstraction chains
WITH n
WHERE NOT exists((n)-[:ABSTRACTS_TO]->())
  AND NOT exists((n)<-[:ABSTRACTS_TO]-())
RETURN n.node_id AS nodeId,
       coalesce(n.layer, 0) AS layer,
       n.embedding AS embedding,
       n.created_at AS createdAt
ORDER BY n.created_at ASC`

	params := map[string]any{
		"spaceId": cfg.SpaceID,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		candidates := make([]mergeCandidate, 0)
		for res.Next(ctx) {
			rec := res.Record()

			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			embedding, _ := rec.Get("embedding")
			createdAt, _ := rec.Get("createdAt")

			// Convert embedding to []float32
			emb := neo4jutil.AsFloat32Slice(embedding)
			if len(emb) == 0 {
				continue
			}

			candidates = append(candidates, mergeCandidate{
				NodeID:    neo4jutil.AsString(nodeID),
				Layer:     neo4jutil.AsInt(layer),
				Embedding: emb,
				CreatedAt: neo4jutil.AsTime(createdAt),
			})
		}

		if err := res.Err(); err != nil {
			return nil, err
		}
		return candidates, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]mergeCandidate), nil
}

// findSimilarNodes uses vector similarity search to find nodes similar to the given candidate.
// Returns nodes at the same layer with similarity >= threshold, excluding abstraction nodes.
func findSimilarNodes(ctx context.Context, driver neo4j.DriverWithContext, cfg pruneConfig, candidate mergeCandidate) ([]similarNode, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Vector similarity search for similar nodes at the same layer
	// Following pattern from internal/anomaly/service.go
	cypher := `
CALL db.index.vector.queryNodes($indexName, 10, $embedding)
YIELD node, score
WHERE node.space_id = $spaceId
  AND node.node_id <> $nodeId
  AND coalesce(node.layer, 0) = $layer
  AND score >= $threshold
  AND coalesce(node.status, 'active') <> 'tombstoned'
  AND NOT exists((node)-[:ABSTRACTS_TO]->())
  AND NOT exists((node)<-[:ABSTRACTS_TO]-())
RETURN node.node_id AS nodeId,
       coalesce(node.layer, 0) AS layer,
       node.created_at AS createdAt,
       score
ORDER BY score DESC`

	params := map[string]any{
		"indexName": cfg.VectorIndexName,
		"embedding": candidate.Embedding,
		"spaceId":   cfg.SpaceID,
		"nodeId":    candidate.NodeID,
		"layer":     candidate.Layer,
		"threshold": cfg.SimilarityThreshold,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		nodes := make([]similarNode, 0)
		for res.Next(ctx) {
			rec := res.Record()

			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			createdAt, _ := rec.Get("createdAt")
			score, _ := rec.Get("score")

			nodes = append(nodes, similarNode{
				NodeID:     neo4jutil.AsString(nodeID),
				Layer:      neo4jutil.AsInt(layer),
				CreatedAt:  neo4jutil.AsTime(createdAt),
				Similarity: neo4jutil.AsFloat64(score),
			})
		}

		if err := res.Err(); err != nil {
			return nil, err
		}
		return nodes, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]similarNode), nil
}

// newUnionFind creates a new union-find data structure
func newUnionFind() *unionFind {
	return &unionFind{
		parent:   make(map[string]string),
		rank:     make(map[string]int),
		nodeInfo: make(map[string]time.Time),
	}
}

// addNode adds a node to the union-find structure if not already present.
// Each node starts as its own parent (self-loop).
func (uf *unionFind) addNode(nodeID string, createdAt time.Time) {
	if _, exists := uf.parent[nodeID]; !exists {
		uf.parent[nodeID] = nodeID
		uf.rank[nodeID] = 0
		uf.nodeInfo[nodeID] = createdAt
	}
}

// find returns the root of the set containing nodeID, with path compression.
func (uf *unionFind) find(nodeID string) string {
	if uf.parent[nodeID] != nodeID {
		// Path compression: make all nodes on the path point directly to root
		uf.parent[nodeID] = uf.find(uf.parent[nodeID])
	}
	return uf.parent[nodeID]
}

// union merges the sets containing nodeA and nodeB.
// The older node (by created_at) becomes the root to ensure the survivor
// is always the oldest node in the merged set.
func (uf *unionFind) union(nodeA, nodeB string) {
	rootA := uf.find(nodeA)
	rootB := uf.find(nodeB)

	if rootA == rootB {
		return // Already in same set
	}

	// Determine which root is older
	timeA := uf.nodeInfo[rootA]
	timeB := uf.nodeInfo[rootB]

	// The older node becomes the root (survivor)
	// If times are equal, use lexicographic order for determinism
	var olderRoot, newerRoot string
	if timeA.Before(timeB) {
		olderRoot = rootA
		newerRoot = rootB
	} else if timeB.Before(timeA) {
		olderRoot = rootB
		newerRoot = rootA
	} else {
		// Equal times: use lexicographic order for determinism
		if rootA < rootB {
			olderRoot = rootA
			newerRoot = rootB
		} else {
			olderRoot = rootB
			newerRoot = rootA
		}
	}

	// Always make the older node the parent (survivor)
	uf.parent[newerRoot] = olderRoot
	// Update rank if needed
	if uf.rank[olderRoot] == uf.rank[newerRoot] {
		uf.rank[olderRoot]++
	}
}

// resolveTransitiveMerges takes a slice of merge pairs and resolves transitive chains.
// For example, if pairs contain (A->B) and (B->C), all three nodes should merge
// to the oldest one. This function uses union-find to identify connected components
// and ensures the oldest node (by created_at) in each component becomes the survivor.
//
// Returns a new slice of merge pairs where each non-survivor node in a component
// is paired with the component's survivor (oldest node).
func resolveTransitiveMerges(pairs []mergePair) []mergePair {
	if len(pairs) == 0 {
		return pairs
	}

	// Build union-find structure from all pairs
	uf := newUnionFind()

	// First pass: add all nodes from pairs
	for _, p := range pairs {
		uf.addNode(p.SurvivorID, p.SurvivorCreatedAt)
		uf.addNode(p.MergedID, p.MergedCreatedAt)
	}

	// Second pass: union all pairs
	for _, p := range pairs {
		uf.union(p.SurvivorID, p.MergedID)
	}

	// Third pass: group nodes by their root (representative)
	// The root of each set is the oldest node in that set
	components := make(map[string][]string) // root -> list of non-root nodes
	for nodeID := range uf.parent {
		root := uf.find(nodeID)
		if nodeID != root {
			components[root] = append(components[root], nodeID)
		}
	}

	// Build the resolved merge pairs
	// Each non-root node in a component merges to the root (oldest node)
	var resolved []mergePair
	for survivorID, mergedNodes := range components {
		survivorCreatedAt := uf.nodeInfo[survivorID]

		for _, mergedID := range mergedNodes {
			mergedCreatedAt := uf.nodeInfo[mergedID]

			// Find the original similarity score if available
			// Use the maximum similarity among related pairs
			var similarity float64
			for _, p := range pairs {
				if (p.SurvivorID == survivorID && p.MergedID == mergedID) ||
					(p.SurvivorID == mergedID && p.MergedID == survivorID) ||
					(p.MergedID == mergedID) ||
					(p.SurvivorID == mergedID) {
					if p.Similarity > similarity {
						similarity = p.Similarity
					}
				}
			}

			resolved = append(resolved, mergePair{
				SurvivorID:        survivorID,
				MergedID:          mergedID,
				Similarity:        similarity,
				SurvivorCreatedAt: survivorCreatedAt,
				MergedCreatedAt:   mergedCreatedAt,
			})
		}
	}

	return resolved
}

// printHeader outputs the job configuration header
func printHeader(cfg pruneConfig) {
	if cfg.DryRun {
		fmt.Println("Mode: DRY RUN (no changes will be made)")
	} else {
		fmt.Println("Mode: LIVE (changes will be applied)")
	}

	fmt.Printf("Space: %s\n", cfg.SpaceID)
	fmt.Printf("Batch size: %d\n", cfg.BatchSize)

	fmt.Println("\nEdge pruning settings:")
	fmt.Printf("  Weight threshold: %g\n", cfg.WeightThreshold)
	fmt.Printf("  Min evidence to protect: %d\n", cfg.MinEvidence)
	fmt.Printf("  Older than: %d days\n", cfg.OlderThanDays)

	fmt.Println("\nNode tombstoning settings:")
	fmt.Printf("  Retention window: %d days\n", cfg.RetentionDays)
	fmt.Printf("  Max degree for orphan: %d\n", cfg.MaxDegree)

	fmt.Println("\nNode merging settings:")
	fmt.Printf("  Enabled: %v\n", cfg.MergeEnabled)
	if cfg.MergeEnabled {
		fmt.Printf("  Similarity threshold: %g\n", cfg.SimilarityThreshold)
	}
}

// printStats outputs the job statistics
func printStats(stats pruneStats, cfg pruneConfig) {
	fmt.Println("\n========================================")
	fmt.Println("Statistics")
	fmt.Println("========================================")

	// Edge pruning stats
	fmt.Println("\nEdge Pruning:")
	fmt.Printf("  Edges scanned:   %d\n", stats.edgesScanned)
	if cfg.DryRun {
		fmt.Printf("  Edges to prune:  %d\n", stats.edgesPruned)
	} else {
		fmt.Printf("  Edges pruned:    %d\n", stats.edgesPruned)
	}
	fmt.Printf("  Edges protected: %d\n", stats.edgesProtected)

	// Node tombstoning stats
	fmt.Println("\nNode Tombstoning:")
	fmt.Printf("  Nodes scanned:     %d\n", stats.nodesScanned)
	if cfg.DryRun {
		fmt.Printf("  Nodes to tombstone: %d\n", stats.nodesTombstoned)
	} else {
		fmt.Printf("  Nodes tombstoned:   %d\n", stats.nodesTombstoned)
	}
	fmt.Printf("  Nodes protected:    %d\n", stats.nodesProtected)

	// Node merging stats (if enabled)
	if cfg.MergeEnabled {
		fmt.Println("\nNode Merging:")
		if cfg.DryRun {
			fmt.Printf("  Merges to perform: %d\n", stats.mergesPerformed)
			fmt.Printf("  Nodes to merge:    %d\n", stats.nodesMerged)
		} else {
			fmt.Printf("  Merges performed:  %d\n", stats.mergesPerformed)
			fmt.Printf("  Nodes merged:      %d\n", stats.nodesMerged)
		}
	}

	fmt.Println()
	if cfg.DryRun {
		fmt.Println("Run with --dry-run=false to apply changes.")
	} else {
		fmt.Println("Changes applied successfully.")
	}
}
