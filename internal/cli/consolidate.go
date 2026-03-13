package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/cli/neo4jutil"
	"mdemg/internal/config"
	"mdemg/internal/hidden"
)

// consolidateConfig holds CLI and environment configuration for the consolidation job
type consolidateConfig struct {
	// Neo4j connection
	neo4jURI  string
	neo4jUser string
	neo4jPass string

	// Legacy consolidation parameters (CO_ACTIVATED_WITH based)
	minClusterSize  int     // default: 3
	weightThreshold float64 // default: 0.5
	maxPromotions   int     // default: 50

	// Hidden layer parameters
	hiddenLayerEnabled    bool    // Run hidden layer operations
	multiLayer            bool    // Run full multi-layer consolidation (L0-L5)
	hiddenClusterEps      float64 // DBSCAN epsilon
	hiddenMinSamples      int     // DBSCAN min samples
	hiddenMaxNodes        int     // Max hidden nodes to create
	hiddenForwardAlpha    float64 // Forward pass alpha
	hiddenForwardBeta     float64 // Forward pass beta
	hiddenBackwardSelf    float64 // Backward pass self weight
	hiddenBackwardBase    float64 // Backward pass base weight
	hiddenBackwardConcept float64 // Backward pass concept weight

	// Operation modes
	legacyMode   bool // Run legacy CO_ACTIVATED_WITH consolidation
	forwardOnly  bool // Only run forward pass (hidden layer)
	backwardOnly bool // Only run backward pass (hidden layer)
	clusterOnly  bool // Only run clustering (hidden layer)

	// Processing options
	dryRun  bool
	spaceID string // REQUIRED
}

// newConsolidateCmd creates the consolidate command
func newConsolidateCmd() *cobra.Command {
	var cfg consolidateConfig

	cmd := &cobra.Command{
		Use:   "consolidate",
		Short: "Run graph consolidation (legacy CO_ACTIVATED_WITH or hidden layer)",
		Long: `Run consolidation operations on the MDEMG graph.

Supports two modes:
  1. Legacy consolidation: CO_ACTIVATED_WITH based cluster detection and abstraction
  2. Hidden layer: Multi-layer consolidation (L0-L5) with forward/backward passes

Examples:
  # Legacy consolidation (dry-run)
  mdemg consolidate --space-id myspace --legacy

  # Hidden layer consolidation (dry-run)
  mdemg consolidate --space-id myspace --hidden-layer

  # Full multi-layer consolidation (live)
  mdemg consolidate --space-id myspace --hidden-layer --multi-layer --dry-run=false

  # Forward pass only (live)
  mdemg consolidate --space-id myspace --hidden-layer --forward-only --dry-run=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Print banner
			fmt.Println("MDEMG Consolidation Job")
			fmt.Println("=======================")
			fmt.Println()

			// Resolve space ID from flag / global flag / env var
			if cfg.spaceID == "" {
				cfg.spaceID = resolveSpaceID(cmd)
			}

			// Load config (YAML → .env → env vars)
			if cfgPath := config.FindConfigFile(); cfgPath != "" {
				_ = config.LoadYAMLConfig(cfgPath)
			}
			_ = godotenv.Load()

			// Load Neo4j connection from environment
			if err := loadConsolidateNeo4jConfig(&cfg); err != nil {
				return err
			}

			// Validate configuration
			if err := validateConsolidateConfig(&cfg); err != nil {
				return err
			}

			ctx := context.Background()

			driver, err := newConsolidateDriver(cfg)
			if err != nil {
				return fmt.Errorf("failed to create neo4j driver: %w", err)
			}
			defer func() { _ = driver.Close(ctx) }()

			// Verify connectivity
			if err := driver.VerifyConnectivity(ctx); err != nil {
				return fmt.Errorf("failed to connect to neo4j: %w", err)
			}

			// Determine which operations to run
			if cfg.legacyMode {
				// Run legacy CO_ACTIVATED_WITH based consolidation
				if err := runLegacyConsolidationJob(ctx, driver, cfg); err != nil {
					return fmt.Errorf("legacy consolidation job failed: %w", err)
				}
			}

			if cfg.hiddenLayerEnabled {
				// Run hidden layer operations
				if err := runHiddenLayerJob(ctx, driver, cfg); err != nil {
					return fmt.Errorf("hidden layer job failed: %w", err)
				}
			}

			if !cfg.legacyMode && !cfg.hiddenLayerEnabled {
				return errors.New("no operations selected - use --hidden-layer and/or --legacy flags")
			}

			return nil
		},
	}

	// Legacy consolidation flags
	cmd.Flags().IntVar(&cfg.minClusterSize, "min-cluster-size", 3, "Minimum number of nodes to form a cluster (legacy)")
	cmd.Flags().Float64Var(&cfg.weightThreshold, "weight-threshold", 0.5, "Minimum CO_ACTIVATED_WITH weight (legacy)")
	cmd.Flags().IntVar(&cfg.maxPromotions, "max-promotions", 50, "Maximum abstraction nodes to create (legacy)")

	// Hidden layer flags
	cmd.Flags().BoolVar(&cfg.hiddenLayerEnabled, "hidden-layer", false, "Enable hidden layer operations")
	cmd.Flags().BoolVar(&cfg.multiLayer, "multi-layer", false, "Run full multi-layer consolidation (L0-L5)")
	cmd.Flags().Float64Var(&cfg.hiddenClusterEps, "hidden-eps", 0.3, "DBSCAN epsilon (max distance)")
	cmd.Flags().IntVar(&cfg.hiddenMinSamples, "hidden-min-samples", 3, "DBSCAN minimum samples per cluster")
	cmd.Flags().IntVar(&cfg.hiddenMaxNodes, "hidden-max-nodes", 500, "Maximum hidden nodes to create")
	cmd.Flags().Float64Var(&cfg.hiddenForwardAlpha, "hidden-fwd-alpha", 0.6, "Forward pass: weight of current embedding")
	cmd.Flags().Float64Var(&cfg.hiddenForwardBeta, "hidden-fwd-beta", 0.4, "Forward pass: weight of aggregated embedding")
	cmd.Flags().Float64Var(&cfg.hiddenBackwardSelf, "hidden-bwd-self", 0.2, "Backward pass: self weight")
	cmd.Flags().Float64Var(&cfg.hiddenBackwardBase, "hidden-bwd-base", 0.5, "Backward pass: base signal weight")
	cmd.Flags().Float64Var(&cfg.hiddenBackwardConcept, "hidden-bwd-concept", 0.3, "Backward pass: concept signal weight")

	// Operation mode flags
	cmd.Flags().BoolVar(&cfg.legacyMode, "legacy", false, "Run legacy CO_ACTIVATED_WITH consolidation")
	cmd.Flags().BoolVar(&cfg.forwardOnly, "forward-only", false, "Only run forward pass (hidden layer)")
	cmd.Flags().BoolVar(&cfg.backwardOnly, "backward-only", false, "Only run backward pass (hidden layer)")
	cmd.Flags().BoolVar(&cfg.clusterOnly, "cluster-only", false, "Only run clustering (hidden layer)")

	// Common flags
	cmd.Flags().BoolVar(&cfg.dryRun, "dry-run", true, "Preview mode - no modifications (default: true)")
	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID to process (or set MDEMG_SPACE_ID)")

	return cmd
}

// loadConsolidateNeo4jConfig loads Neo4j connection config from environment
func loadConsolidateNeo4jConfig(cfg *consolidateConfig) error {
	get := func(k, def string) string {
		v := strings.TrimSpace(os.Getenv(k))
		if v == "" {
			return def
		}
		return v
	}

	cfg.neo4jURI = get("NEO4J_URI", "")
	cfg.neo4jUser = get("NEO4J_USER", "")
	cfg.neo4jPass = get("NEO4J_PASS", "")

	if cfg.neo4jURI == "" || cfg.neo4jUser == "" || cfg.neo4jPass == "" {
		return errors.New("NEO4J_URI/NEO4J_USER/NEO4J_PASS environment variables are required")
	}

	return nil
}

// validateConsolidateConfig validates the configuration
func validateConsolidateConfig(cfg *consolidateConfig) error {
	if cfg.spaceID == "" {
		return errors.New("--space-id is required")
	}

	// Validate legacy parameters
	if cfg.legacyMode {
		if cfg.minClusterSize < 2 {
			return errors.New("min-cluster-size must be at least 2")
		}
		if cfg.weightThreshold < 0 || cfg.weightThreshold > 1 {
			return errors.New("weight-threshold must be between 0 and 1")
		}
		if cfg.maxPromotions <= 0 {
			return errors.New("max-promotions must be positive")
		}
	}

	// Validate hidden layer parameters
	if cfg.hiddenLayerEnabled {
		if cfg.hiddenClusterEps <= 0 || cfg.hiddenClusterEps > 1 {
			return errors.New("hidden-eps must be in range (0, 1]")
		}
		if cfg.hiddenMinSamples < 2 {
			return errors.New("hidden-min-samples must be at least 2")
		}
		if cfg.hiddenMaxNodes < 1 {
			return errors.New("hidden-max-nodes must be positive")
		}
	}

	return nil
}

// newConsolidateDriver creates a new Neo4j driver with the given configuration
func newConsolidateDriver(cfg consolidateConfig) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(cfg.neo4jURI, neo4j.BasicAuth(cfg.neo4jUser, cfg.neo4jPass, ""))
	if err != nil {
		return nil, err
	}
	return driver, nil
}

// runHiddenLayerJob executes hidden layer operations using the hidden service
func runHiddenLayerJob(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) error {
	fmt.Println("\n========================================")
	fmt.Println("Hidden Layer Operations")
	fmt.Println("========================================")

	if cfg.dryRun {
		fmt.Println("Mode: DRY RUN (preview only)")
	} else {
		fmt.Println("Mode: LIVE (changes will be applied)")
	}
	fmt.Printf("Space: %s\n", cfg.spaceID)
	fmt.Printf("Cluster Eps: %.2f\n", cfg.hiddenClusterEps)
	fmt.Printf("Min Samples: %d\n", cfg.hiddenMinSamples)
	fmt.Printf("Max Hidden Nodes: %d (dynamic floor: 10%% of source nodes)\n", cfg.hiddenMaxNodes)

	// Build config for hidden service
	svcCfg := config.Config{
		Neo4jURI:                cfg.neo4jURI,
		Neo4jUser:               cfg.neo4jUser,
		Neo4jPass:               cfg.neo4jPass,
		HiddenLayerEnabled:        !cfg.dryRun, // Disable writes in dry-run
		HiddenLayerClusterEps:     cfg.hiddenClusterEps,
		HiddenLayerMinSamples:     cfg.hiddenMinSamples,
		HiddenLayerMaxHidden:      cfg.hiddenMaxNodes,
		HiddenLayerMaxClusterSize: 200, // Default max cluster size before path-based splitting
		HiddenLayerPathGroupDepth: 2,   // Default path group depth for naming
		HiddenLayerForwardAlpha:   cfg.hiddenForwardAlpha,
		HiddenLayerForwardBeta:    cfg.hiddenForwardBeta,
		HiddenLayerBackwardSelf:   cfg.hiddenBackwardSelf,
		HiddenLayerBackwardBase:   cfg.hiddenBackwardBase,
		HiddenLayerBackwardConc:   cfg.hiddenBackwardConcept,
	}

	svc := hidden.NewService(svcCfg, driver, nil)

	// Determine operations
	runClustering := !cfg.forwardOnly && !cfg.backwardOnly
	runForward := !cfg.clusterOnly && !cfg.backwardOnly
	runBackward := !cfg.clusterOnly && !cfg.forwardOnly

	if cfg.dryRun {
		// Dry run: show what would happen
		fmt.Println("\nDry run - showing potential operations:")
		if runClustering {
			count, err := countClusterableBaseNodes(ctx, driver, cfg.spaceID)
			if err != nil {
				return fmt.Errorf("count clusterable nodes: %w", err)
			}
			fmt.Printf("  - L0 base nodes for re-clustering: %d\n", count)
			if count >= cfg.hiddenMinSamples {
				maxHidden := (count + 9) / 10 // ceil(count * 0.1)
				fmt.Printf("  - Will create %d hidden nodes (10%% of source)\n", maxHidden)
			}
		}
		if runForward {
			hiddenCount, conceptCount, err := countLayerNodes(ctx, driver, cfg.spaceID)
			if err != nil {
				return fmt.Errorf("count layer nodes: %w", err)
			}
			fmt.Printf("  - Hidden nodes to update (forward pass): %d\n", hiddenCount)
			fmt.Printf("  - Concept nodes to update (forward pass): %d\n", conceptCount)
		}
		if runBackward {
			hiddenCount, _, err := countLayerNodes(ctx, driver, cfg.spaceID)
			if err != nil {
				return fmt.Errorf("count layer nodes: %w", err)
			}
			fmt.Printf("  - Hidden nodes to update (backward pass): %d\n", hiddenCount)
		}
		fmt.Println("\nRun with --dry-run=false to apply changes.")
		return nil
	}

	// Live run
	fmt.Println("\nExecuting operations...")

	if runClustering {
		if cfg.multiLayer {
			fmt.Println("\nStep 1: Running full multi-layer consolidation (L0-L5)...")
			result, err := svc.RunConsolidation(ctx, cfg.spaceID)
			if err != nil {
				return fmt.Errorf("run multi-layer consolidation: %w", err)
			}
			fmt.Printf("  Hidden nodes created: %d\n", result.HiddenNodesCreated)
			for layer, count := range result.ConceptNodesCreated {
				fmt.Printf("  Layer %d concept nodes created: %d\n", layer, count)
			}
			fmt.Printf("  Total duration: %v\n", result.TotalDuration)
		} else {
			fmt.Println("\nStep 1: Creating hidden nodes from orphan base data (L0-L1)...")
			created, err := svc.CreateHiddenNodes(ctx, cfg.spaceID)
			if err != nil {
				return fmt.Errorf("create hidden nodes: %w", err)
			}
			fmt.Printf("  Hidden nodes created: %d\n", created)
		}
	}

	if runForward && !cfg.multiLayer {
		fmt.Println("\nStep 2: Running forward pass...")
		result, err := svc.ForwardPass(ctx, cfg.spaceID)
		if err != nil {
			return fmt.Errorf("forward pass: %w", err)
		}
		fmt.Printf("  Hidden nodes updated: %d\n", result.HiddenNodesUpdated)
		fmt.Printf("  Concept nodes updated: %d\n", result.ConceptNodesUpdated)
		fmt.Printf("  Duration: %v\n", result.Duration)
	}

	if runBackward && !cfg.multiLayer {
		fmt.Println("\nStep 3: Running backward pass...")
		result, err := svc.BackwardPass(ctx, cfg.spaceID)
		if err != nil {
			return fmt.Errorf("backward pass: %w", err)
		}
		fmt.Printf("  Hidden nodes updated: %d\n", result.HiddenNodesUpdated)
		fmt.Printf("  Duration: %v\n", result.Duration)
	}

	fmt.Println("\nHidden layer operations completed successfully.")
	return nil
}

// countClusterableBaseNodes counts all L0 base nodes eligible for clustering.
// This counts ALL L0 nodes (not just orphans) since re-clustering detaches old edges.
func countClusterableBaseNodes(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = sess.Close(ctx) }()

	cypher := `
MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})
WHERE b.embedding IS NOT NULL
  AND (b.role_type IS NULL OR b.role_type <> 'conversation_observation')
RETURN count(b) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			cnt, _ := res.Record().Get("cnt")
			return neo4jutil.AsInt(cnt), res.Err()
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	return result.(int), nil
}

// countLayerNodes counts hidden (layer 1) and concept (layer >= 2) nodes
func countLayerNodes(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (int, int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = sess.Close(ctx) }()

	cypher := `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.layer >= 1
RETURN n.layer AS layer, count(n) AS cnt`

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		hiddenCount := 0
		conceptCount := 0
		for res.Next(ctx) {
			rec := res.Record()
			layer, _ := rec.Get("layer")
			cnt, _ := rec.Get("cnt")
			l := neo4jutil.AsInt(layer)
			c := neo4jutil.AsInt(cnt)
			if l == 1 {
				hiddenCount = c
			} else if l >= 2 {
				conceptCount += c
			}
		}
		return []int{hiddenCount, conceptCount}, res.Err()
	})
	if err != nil {
		return 0, 0, err
	}
	counts := result.([]int)
	return counts[0], counts[1], nil
}

// ============================================================================
// Legacy consolidation code (CO_ACTIVATED_WITH based)
// ============================================================================

// consolidateStats tracks statistics for the consolidation job
type consolidateStats struct {
	clustersFound   int
	nodesPromoted   int
	edgesCreated    int
	skippedNoEmbed  int
	skippedTooSmall int
	samples         []clusterSample // first few clusters for sample output
}

// clusterSample holds information about a processed cluster for sample output
type clusterSample struct {
	clusterNum    int
	memberCount   int
	sourceLayer   int
	targetLayer   int
	memberIDs     []string
	abstractionID string // empty in dry-run
}

// clusterCandidate represents a node with its high-weight neighbors at the same layer
type clusterCandidate struct {
	nodeID      string
	layer       int
	embedding   []float64
	neighborIDs []string
}

// clusterMember represents a single node within a cluster
type clusterMember struct {
	nodeID    string
	embedding []float64
}

// cluster represents a group of co-activated nodes at the same layer
type cluster struct {
	members []clusterMember
	layer   int
}

// runLegacyConsolidationJob executes the cluster detection and abstraction promotion
func runLegacyConsolidationJob(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) error {
	fmt.Println("\n========================================")
	fmt.Println("Legacy Consolidation (CO_ACTIVATED_WITH)")
	fmt.Println("========================================")

	// Print header
	printConsolidateHeader(cfg)

	fmt.Println("\nProcessing...")

	stats := consolidateStats{
		samples: make([]clusterSample, 0, 5),
	}

	// Step 1: Query cluster candidates from Neo4j
	fmt.Println("\nStep 1: Detecting cluster candidates...")
	candidates, err := queryClusterCandidates(ctx, driver, cfg)
	if err != nil {
		return fmt.Errorf("query cluster candidates: %w", err)
	}
	fmt.Printf("Found %d nodes with sufficient high-weight neighbors\n", len(candidates))

	if len(candidates) == 0 {
		fmt.Println("\nNo cluster candidates found. Nothing to promote.")
		printConsolidateStats(stats, cfg.dryRun)
		return nil
	}

	// Step 2: Build clusters from candidates using greedy first-come assignment
	fmt.Println("\nStep 2: Building clusters...")
	clusters := buildClusters(candidates, cfg.minClusterSize)
	stats.clustersFound = len(clusters)
	fmt.Printf("Formed %d clusters (min size: %d)\n", len(clusters), cfg.minClusterSize)

	if len(clusters) == 0 {
		fmt.Println("\nNo clusters met the minimum size requirement. Nothing to promote.")
		printConsolidateStats(stats, cfg.dryRun)
		return nil
	}

	// Step 3: Process clusters and create abstractions
	fmt.Println("\nStep 3: Processing clusters for abstraction promotion...")
	promotionCount := 0

	for i, c := range clusters {
		// Respect max promotions cap
		if promotionCount >= cfg.maxPromotions {
			fmt.Printf("\nReached max promotions cap (%d). Stopping.\n", cfg.maxPromotions)
			break
		}

		// Collect embeddings from cluster members
		var embeddings [][]float64
		for _, member := range c.members {
			if len(member.embedding) > 0 {
				embeddings = append(embeddings, member.embedding)
			} else {
				stats.skippedNoEmbed++
			}
		}

		// Calculate averaged embedding for the abstraction node
		avgEmbedding := averageEmbeddings(embeddings)
		if avgEmbedding == nil {
			fmt.Printf("  Cluster %d: Skipped (no valid embeddings)\n", i+1)
			stats.skippedTooSmall++
			continue
		}

		// Report cluster info
		memberIDs := make([]string, 0, len(c.members))
		for _, m := range c.members {
			memberIDs = append(memberIDs, m.nodeID)
		}
		fmt.Printf("  Cluster %d: %d members at layer %d -> layer %d\n",
			i+1, len(c.members), c.layer, c.layer+1)

		if cfg.dryRun {
			// Dry-run mode: just count what would happen
			stats.nodesPromoted++
			stats.edgesCreated += len(c.members)

			// Collect sample cluster data (first 5)
			if len(stats.samples) < 5 {
				stats.samples = append(stats.samples, clusterSample{
					clusterNum:    i + 1,
					memberCount:   len(c.members),
					sourceLayer:   c.layer,
					targetLayer:   c.layer + 1,
					memberIDs:     memberIDs,
					abstractionID: "", // empty in dry-run
				})
			}
		} else {
			// Live mode: create abstraction node and edges
			result, err := createAbstraction(ctx, driver, cfg, c, avgEmbedding)
			if err != nil {
				return fmt.Errorf("create abstraction for cluster %d: %w", i+1, err)
			}
			fmt.Printf("    Created abstraction node: %s (%d edges)\n", result.nodeID, result.memberCount)
			stats.nodesPromoted++
			stats.edgesCreated += result.memberCount

			// Collect sample cluster data (first 5)
			if len(stats.samples) < 5 {
				stats.samples = append(stats.samples, clusterSample{
					clusterNum:    i + 1,
					memberCount:   len(c.members),
					sourceLayer:   c.layer,
					targetLayer:   c.layer + 1,
					memberIDs:     memberIDs,
					abstractionID: result.nodeID,
				})
			}
		}

		promotionCount++
	}

	// Print statistics
	printConsolidateStats(stats, cfg.dryRun)

	return nil
}

// printConsolidateHeader outputs the job configuration header
func printConsolidateHeader(cfg consolidateConfig) {
	if cfg.dryRun {
		fmt.Println("Mode: DRY RUN (no changes will be made)")
	} else {
		fmt.Println("Mode: LIVE (changes will be applied)")
	}

	fmt.Printf("Space: %s\n", cfg.spaceID)
	fmt.Printf("Min cluster size: %d\n", cfg.minClusterSize)
	fmt.Printf("Weight threshold: %g\n", cfg.weightThreshold)
	fmt.Printf("Max promotions: %d\n", cfg.maxPromotions)
}

// printConsolidateStats outputs the job statistics
func printConsolidateStats(stats consolidateStats, dryRun bool) {
	fmt.Println("\n----------------------------------------")
	fmt.Println("Legacy Consolidation Statistics")
	fmt.Println("----------------------------------------")

	// Main counts
	fmt.Printf("Clusters found:          %d\n", stats.clustersFound)
	if dryRun {
		fmt.Printf("Nodes to promote:        %d\n", stats.nodesPromoted)
		fmt.Printf("Edges to create:         %d\n", stats.edgesCreated)
	} else {
		fmt.Printf("Nodes promoted:          %d\n", stats.nodesPromoted)
		fmt.Printf("Edges created:           %d\n", stats.edgesCreated)
	}

	// Skip reasons
	fmt.Println("\nSkipped clusters:")
	fmt.Printf("- No valid embeddings:   %d\n", stats.skippedNoEmbed)
	fmt.Printf("- Too small:             %d\n", stats.skippedTooSmall)

	// Print sample clusters (up to 5)
	if len(stats.samples) > 0 {
		fmt.Println("\nSample clusters:")
		for _, s := range stats.samples {
			// Truncate member IDs display if too many
			displayIDs := s.memberIDs
			suffix := ""
			if len(displayIDs) > 4 {
				displayIDs = displayIDs[:4]
				suffix = fmt.Sprintf("... +%d more", len(s.memberIDs)-4)
			}

			memberList := strings.Join(displayIDs, ", ")
			if suffix != "" {
				memberList += " " + suffix
			}

			if s.abstractionID != "" {
				// Live run - show created abstraction ID
				fmt.Printf("  Cluster %d: %d members (layer %d->%d) -> %s\n",
					s.clusterNum, s.memberCount, s.sourceLayer, s.targetLayer, truncateID(s.abstractionID))
			} else {
				// Dry run - show member IDs
				fmt.Printf("  Cluster %d: %d members (layer %d->%d) [%s]\n",
					s.clusterNum, s.memberCount, s.sourceLayer, s.targetLayer, memberList)
			}
		}
	}

	fmt.Println()
	if dryRun {
		fmt.Println("Run with --dry-run=false to apply changes.")
	} else {
		fmt.Println("Changes applied successfully.")
	}
}

// truncateID shortens a UUID for display (first 8 chars + "...")
func truncateID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "..."
}

// queryClusterCandidates fetches nodes with sufficient high-weight neighbors at the same layer.
func queryClusterCandidates(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig) ([]clusterCandidate, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = sess.Close(ctx) }()

	cypher := `
MATCH (a:MemoryNode)-[r:CO_ACTIVATED_WITH]-(b:MemoryNode)
WHERE a.space_id = $spaceId
  AND r.weight >= $threshold
  AND a.layer = b.layer
WITH a, collect(DISTINCT b) AS neighbors
WHERE size(neighbors) >= $minNeighbors
RETURN a.node_id AS nodeId,
       a.layer AS layer,
       a.embedding AS embedding,
       [n IN neighbors | n.node_id] AS neighborIds
ORDER BY size(neighbors) DESC`

	params := map[string]any{
		"spaceId":      cfg.spaceID,
		"threshold":    cfg.weightThreshold,
		"minNeighbors": cfg.minClusterSize - 1,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		candidates := make([]clusterCandidate, 0)
		for res.Next(ctx) {
			rec := res.Record()

			nodeID, _ := rec.Get("nodeId")
			layer, _ := rec.Get("layer")
			embedding, _ := rec.Get("embedding")
			neighborIDs, _ := rec.Get("neighborIds")

			candidate := clusterCandidate{
				nodeID:      neo4jutil.AsString(nodeID),
				layer:       neo4jutil.AsInt(layer),
				embedding:   neo4jutil.AsFloat64Slice(embedding),
				neighborIDs: neo4jutil.AsStringSlice(neighborIDs),
			}
			candidates = append(candidates, candidate)
		}

		if err := res.Err(); err != nil {
			return nil, err
		}
		return candidates, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]clusterCandidate), nil
}

// buildClusters groups cluster candidates into non-overlapping clusters
func buildClusters(candidates []clusterCandidate, minSize int) []cluster {
	candidateMap := make(map[string]clusterCandidate)
	for _, c := range candidates {
		candidateMap[c.nodeID] = c
	}

	assigned := make(map[string]bool)
	var clusters []cluster

	for _, candidate := range candidates {
		if assigned[candidate.nodeID] {
			continue
		}

		var members []clusterMember
		members = append(members, clusterMember{
			nodeID:    candidate.nodeID,
			embedding: candidate.embedding,
		})

		for _, neighborID := range candidate.neighborIDs {
			if assigned[neighborID] {
				continue
			}

			var neighborEmbedding []float64
			if neighborCandidate, exists := candidateMap[neighborID]; exists {
				neighborEmbedding = neighborCandidate.embedding
			}

			members = append(members, clusterMember{
				nodeID:    neighborID,
				embedding: neighborEmbedding,
			})
		}

		if len(members) < minSize {
			continue
		}

		for _, member := range members {
			assigned[member.nodeID] = true
		}

		clusters = append(clusters, cluster{
			members: members,
			layer:   candidate.layer,
		})
	}

	return clusters
}

// averageEmbeddings computes the centroid of multiple embedding vectors
func averageEmbeddings(embeddings [][]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}

	var dim int
	for _, emb := range embeddings {
		if len(emb) > 0 {
			dim = len(emb)
			break
		}
	}
	if dim == 0 {
		return nil
	}

	result := make([]float64, dim)
	validCount := 0

	for _, emb := range embeddings {
		if len(emb) != dim {
			continue
		}
		for i, v := range emb {
			result[i] += v
		}
		validCount++
	}

	if validCount == 0 {
		return nil
	}

	count := float64(validCount)
	for i := range result {
		result[i] /= count
	}

	return result
}

// abstractionResult holds the result of creating an abstraction node
type abstractionResult struct {
	nodeID      string
	memberCount int
}

// createAbstraction creates a new MemoryNode at layer+1 and ABSTRACTS_TO edges
func createAbstraction(ctx context.Context, driver neo4j.DriverWithContext, cfg consolidateConfig, c cluster, embedding []float64) (*abstractionResult, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = sess.Close(ctx) }()

	name := generateAbstractionName(c.members)
	summary := fmt.Sprintf("Cluster abstraction of %d nodes at layer %d", len(c.members), c.layer)
	newLayer := c.layer + 1

	memberIDs := make([]string, 0, len(c.members))
	for _, m := range c.members {
		memberIDs = append(memberIDs, m.nodeID)
	}

	params := map[string]any{
		"spaceId":   cfg.spaceID,
		"name":      name,
		"summary":   summary,
		"layer":     newLayer,
		"embedding": embedding,
		"memberIds": memberIDs,
	}

	result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
CREATE (abs:MemoryNode {
  space_id: $spaceId,
  node_id: randomUUID(),
  name: $name,
  summary: $summary,
  layer: $layer,
  embedding: $embedding,
  created_at: datetime(),
  updated_at: datetime(),
  role_type: 'abstraction',
  version: 1
})
WITH abs
UNWIND $memberIds AS memberId
MATCH (m:MemoryNode {space_id: $spaceId, node_id: memberId})
CREATE (m)-[:ABSTRACTS_TO {
  space_id: $spaceId,
  edge_id: randomUUID(),
  created_at: datetime(),
  updated_at: datetime()
}]->(abs)
RETURN abs.node_id AS absNodeId, count(m) AS memberCount`

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("absNodeId")
			memberCount, _ := rec.Get("memberCount")
			return &abstractionResult{
				nodeID:      neo4jutil.AsString(nodeID),
				memberCount: neo4jutil.AsInt(memberCount),
			}, nil
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		return nil, errors.New("no result returned from abstraction creation query")
	})

	if err != nil {
		return nil, err
	}
	return result.(*abstractionResult), nil
}

// generateAbstractionName creates a descriptive name for the abstraction node
func generateAbstractionName(members []clusterMember) string {
	if len(members) == 0 {
		return "Abstraction: (empty)"
	}

	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.nodeID)
	}

	joined := strings.Join(ids, ", ")

	const maxLen = 60
	if len(joined) > maxLen {
		joined = joined[:maxLen-3] + "..."
	}

	return fmt.Sprintf("Abstraction: [%s]", joined)
}
