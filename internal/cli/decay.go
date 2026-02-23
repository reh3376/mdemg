package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/cli/neo4jutil"
	"mdemg/internal/config"
)

// edge represents a relationship in the graph for decay processing
type edge struct {
	ID            int64
	RelType       string
	SourceID      string
	TargetID      string
	Weight        float64
	EvidenceCount int
	Pinned        bool
	LastActivated time.Time
}

// decayResult holds the result of decay calculation for a single edge
type decayResult struct {
	Edge          edge
	OldWeight     float64
	NewWeight     float64
	DecayPercent  float64
	ShouldPrune   bool
	Protected     bool
	ProtectReason string // "high_evidence", "pinned", or ""
}

// decayConfig holds CLI and environment configuration for the decay job
type decayConfig struct {
	// Neo4j connection
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string

	// Decay parameters
	DecayRate      float64
	PruneThreshold float64
	MinEvidence    int
	OlderThanDays  int

	// Processing options
	DryRun    bool
	SpaceID   string
	BatchSize int
}

// decayStats tracks statistics for the decay job
type decayStats struct {
	scanned           int
	decayed           int
	pruned            int
	protectedEvidence int
	protectedPinned   int
	samples           []decayResult // first few decay results for sample output
}

// newDecayCmd creates the decay subcommand for the unified CLI
func newDecayCmd() *cobra.Command {
	var cfg decayConfig

	cmd := &cobra.Command{
		Use:   "decay",
		Short: "Apply temporal decay to learning edges",
		Long: `Apply temporal decay to learning edges based on time since last activation.

Exponential decay formula: w_new = w_old * exp(-decay_rate * days_since_activation)

Edges are pruned if ALL of the following are true:
  - weight < prune_threshold
  - evidence_count < min_evidence
  - edge not pinned

Protected edges (high evidence count or pinned) are never pruned.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load config (YAML → .env → env vars)
			if cfgPath := config.FindConfigFile(); cfgPath != "" {
				_ = config.LoadYAMLConfig(cfgPath)
			}
			_ = godotenv.Load()

			// Parse environment variables for Neo4j connection
			if err := parseNeo4jEnv(&cfg); err != nil {
				return err
			}

			// Validate flag values
			if err := validateDecayConfig(&cfg); err != nil {
				return err
			}

			// Resolve space ID from global flag / env var if not set locally
			if cfg.SpaceID == "" {
				cfg.SpaceID = resolveSpaceID(cmd)
			}

			// Print banner
			fmt.Println("MDEMG Decay Job")
			fmt.Println("===============")
			fmt.Println()

			// Create Neo4j driver
			ctx := context.Background()
			driver, err := newDecayDriver(cfg)
			if err != nil {
				return fmt.Errorf("failed to create neo4j driver: %w", err)
			}
			defer func() { _ = driver.Close(ctx) }()

			// Verify connectivity
			if err := driver.VerifyConnectivity(ctx); err != nil {
				return fmt.Errorf("failed to connect to neo4j: %w", err)
			}

			// Run the decay job
			return runDecayJob(ctx, driver, cfg)
		},
	}

	// Register flags
	cmd.Flags().Float64Var(&cfg.DecayRate, "decay-rate", 0.1, "Exponential decay rate constant")
	cmd.Flags().Float64Var(&cfg.PruneThreshold, "prune-threshold", 0.01, "Minimum weight to keep (below = prune candidate)")
	cmd.Flags().IntVar(&cfg.MinEvidence, "min-evidence", 3, "Minimum evidence_count to protect from pruning")
	cmd.Flags().IntVar(&cfg.OlderThanDays, "older-than", 7, "Only process edges older than N days")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", true, "Preview mode - no modifications (default: true)")
	cmd.Flags().StringVar(&cfg.SpaceID, "space-id", "", "Limit to specific space (empty = all)")
	cmd.Flags().IntVar(&cfg.BatchSize, "batch-size", 1000, "Process edges in batches of this size")

	return cmd
}

// parseNeo4jEnv reads Neo4j connection details from environment variables
func parseNeo4jEnv(cfg *decayConfig) error {
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

	return nil
}

// validateDecayConfig validates decay configuration values
func validateDecayConfig(cfg *decayConfig) error {
	if cfg.DecayRate < 0 {
		return errors.New("decay-rate must be non-negative")
	}
	if cfg.PruneThreshold < 0 {
		return errors.New("prune-threshold must be non-negative")
	}
	if cfg.MinEvidence < 0 {
		return errors.New("min-evidence must be non-negative")
	}
	if cfg.OlderThanDays < 0 {
		return errors.New("older-than must be non-negative")
	}
	if cfg.BatchSize <= 0 {
		return errors.New("batch-size must be positive")
	}
	return nil
}

// newDecayDriver creates a new Neo4j driver with the given configuration
func newDecayDriver(cfg decayConfig) (neo4j.DriverWithContext, error) {
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		return nil, err
	}
	return driver, nil
}

// runDecayJob executes the decay and pruning operations
func runDecayJob(ctx context.Context, driver neo4j.DriverWithContext, cfg decayConfig) error {
	// Print header
	printDecayHeader(cfg)

	fmt.Println("\nProcessing...")

	now := time.Now()
	stats := decayStats{
		samples: make([]decayResult, 0, 5),
	}
	offset := 0
	batchNum := 0

	for {
		// Query batch of edges
		edges, err := queryEdgeBatch(ctx, driver, cfg, offset)
		if err != nil {
			return fmt.Errorf("query batch %d: %w", batchNum+1, err)
		}

		if len(edges) == 0 {
			break
		}

		batchNum++
		fmt.Printf("Batch %d: %d edges\n", batchNum, len(edges))

		// Collect results to update/delete
		var toUpdate []decayResult
		var toDelete []decayResult

		for _, e := range edges {
			stats.scanned++

			result := processEdge(e, cfg, now)

			// Track protection stats
			if result.Protected {
				if result.ProtectReason == "pinned" {
					stats.protectedPinned++
				} else if result.ProtectReason == "high_evidence" {
					stats.protectedEvidence++
				}
			}

			// Collect sample results for output (first 5)
			if len(stats.samples) < 5 && result.DecayPercent > 0.1 {
				stats.samples = append(stats.samples, result)
			}

			// Decide action
			if result.ShouldPrune {
				toDelete = append(toDelete, result)
			} else if result.OldWeight != result.NewWeight {
				toUpdate = append(toUpdate, result)
			}
		}

		// Apply changes if not dry-run
		if !cfg.DryRun {
			// Update weights
			for _, r := range toUpdate {
				if err := updateEdgeWeight(ctx, driver, r.Edge.ID, r.NewWeight); err != nil {
					return fmt.Errorf("update edge %d: %w", r.Edge.ID, err)
				}
			}

			// Delete pruned edges
			for _, r := range toDelete {
				if err := deleteEdge(ctx, driver, r.Edge.ID); err != nil {
					return fmt.Errorf("delete edge %d: %w", r.Edge.ID, err)
				}
			}
		}

		stats.decayed += len(toUpdate)
		stats.pruned += len(toDelete)

		offset += len(edges)

		// If we got fewer than batch size, we're done
		if len(edges) < cfg.BatchSize {
			break
		}
	}

	// Print statistics
	printDecayStats(stats, cfg.DryRun)

	return nil
}

// processEdge calculates decay and determines pruning status for a single edge
func processEdge(e edge, cfg decayConfig, now time.Time) decayResult {
	// Calculate decayed weight
	newWeight, decayPercent := calculateDecay(e, cfg.DecayRate, now)

	// Determine if edge should be pruned
	prune, protected, reason := shouldPrune(newWeight, e.EvidenceCount, e.Pinned, cfg.PruneThreshold, cfg.MinEvidence)

	return decayResult{
		Edge:          e,
		OldWeight:     e.Weight,
		NewWeight:     newWeight,
		DecayPercent:  decayPercent,
		ShouldPrune:   prune,
		Protected:     protected,
		ProtectReason: reason,
	}
}

// calculateDecay applies exponential decay formula to an edge weight.
// Formula: w_new = w_old * exp(-decay_rate * days_since_activation)
// From docs/04_Activation_and_Learning.md Section 5
func calculateDecay(e edge, decayRate float64, now time.Time) (newWeight float64, decayPercent float64) {
	// Calculate days since last activation
	daysSince := daysSinceActivation(e.LastActivated, now)

	// Apply exponential decay: w_new = w_old * exp(-decay_rate * days)
	decayFactor := math.Exp(-decayRate * daysSince)
	newWeight = e.Weight * decayFactor

	// Calculate percentage decay for reporting
	if e.Weight > 0 {
		decayPercent = (1 - newWeight/e.Weight) * 100
	} else {
		decayPercent = 0
	}

	return newWeight, decayPercent
}

// daysSinceActivation calculates the number of days between lastActivated and now.
// Returns 0 if lastActivated is zero or in the future.
func daysSinceActivation(lastActivated time.Time, now time.Time) float64 {
	if lastActivated.IsZero() {
		return 0
	}
	duration := now.Sub(lastActivated)
	if duration < 0 {
		return 0
	}
	return duration.Hours() / 24.0
}

// shouldPrune determines if an edge should be pruned based on protection rules.
// From docs/07_Consolidation_and_Pruning.md Section 5.1:
// Prune edges if ALL are true:
// - weight < prune_threshold
// - evidence_count < min_evidence
// - edge not pinned
// Special case: weight <= 0 always marks for pruning (regardless of other factors)
func shouldPrune(weight float64, evidenceCount int, pinned bool, pruneThreshold float64, minEvidence int) (prune bool, protected bool, reason string) {
	// Special case: zero or negative weight always pruned
	if weight <= 0 {
		return true, false, ""
	}

	// Check if weight is below threshold
	if weight >= pruneThreshold {
		// Weight is high enough, no need to prune
		return false, false, ""
	}

	// Weight is below threshold - check protection rules

	// Protection: pinned edges are never pruned
	if pinned {
		return false, true, "pinned"
	}

	// Protection: high evidence count protects the edge
	if evidenceCount >= minEvidence {
		return false, true, "high_evidence"
	}

	// All prune conditions met: weight < threshold AND evidence < min AND !pinned
	return true, false, ""
}

// queryEdgeBatch fetches a batch of edges from Neo4j for decay processing.
// It filters by space_id (optional) and age threshold (older than N days).
func queryEdgeBatch(ctx context.Context, driver neo4j.DriverWithContext, cfg decayConfig, offset int) ([]edge, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = sess.Close(ctx) }()

	// Build the Cypher query with optional space filtering
	// Note: We use updated_at as fallback if last_activated_at is not present
	cypher := `
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE (r.last_activated_at IS NOT NULL AND r.last_activated_at < datetime() - duration({days: $olderThan}))
   OR (r.last_activated_at IS NULL AND r.updated_at IS NOT NULL AND r.updated_at < datetime() - duration({days: $olderThan}))
WITH a, b, r
WHERE $spaceId = '' OR r.space_id = $spaceId
RETURN id(r) AS edgeId,
       type(r) AS relType,
       a.node_id AS sourceId,
       b.node_id AS targetId,
       coalesce(r.weight, 0.0) AS weight,
       coalesce(r.evidence_count, 0) AS evidence,
       coalesce(r.pinned, false) AS pinned,
       coalesce(r.last_activated_at, r.updated_at, r.created_at) AS lastActivated
ORDER BY lastActivated ASC
SKIP $offset
LIMIT $batchSize`

	params := map[string]any{
		"olderThan": cfg.OlderThanDays,
		"spaceId":   cfg.SpaceID,
		"offset":    offset,
		"batchSize": cfg.BatchSize,
	}

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		edges := make([]edge, 0)
		for res.Next(ctx) {
			rec := res.Record()

			edgeID, _ := rec.Get("edgeId")
			relType, _ := rec.Get("relType")
			sourceID, _ := rec.Get("sourceId")
			targetID, _ := rec.Get("targetId")
			weight, _ := rec.Get("weight")
			evidence, _ := rec.Get("evidence")
			pinned, _ := rec.Get("pinned")
			lastActivated, _ := rec.Get("lastActivated")

			e := edge{
				ID:            edgeID.(int64),
				RelType:       relType.(string),
				SourceID:      neo4jutil.AsString(sourceID),
				TargetID:      neo4jutil.AsString(targetID),
				Weight:        neo4jutil.AsFloat64(weight),
				EvidenceCount: neo4jutil.AsInt(evidence),
				Pinned:        neo4jutil.AsBool(pinned),
				LastActivated: neo4jutil.AsTime(lastActivated),
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
	return result.([]edge), nil
}

// updateEdgeWeight updates the weight of an edge in the database
func updateEdgeWeight(ctx context.Context, driver neo4j.DriverWithContext, edgeID int64, newWeight float64) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = sess.Close(ctx) }()

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `
MATCH ()-[r]->()
WHERE id(r) = $edgeId
SET r.weight = $newWeight,
    r.updated_at = datetime()
RETURN count(*) AS updated`

		params := map[string]any{
			"edgeId":    edgeID,
			"newWeight": newWeight,
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

// deleteEdge removes an edge from the database
func deleteEdge(ctx context.Context, driver neo4j.DriverWithContext, edgeID int64) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() { _ = sess.Close(ctx) }()

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

// printDecayHeader outputs the job configuration header
func printDecayHeader(cfg decayConfig) {
	if cfg.DryRun {
		fmt.Println("Mode: DRY RUN (no changes will be made)")
	} else {
		fmt.Println("Mode: LIVE (changes will be applied)")
	}

	if cfg.SpaceID == "" {
		fmt.Println("Space: all")
	} else {
		fmt.Printf("Space: %s\n", cfg.SpaceID)
	}

	fmt.Printf("Edges older than: %d days\n", cfg.OlderThanDays)
	fmt.Printf("Decay rate: %g\n", cfg.DecayRate)
	fmt.Printf("Prune threshold: %g\n", cfg.PruneThreshold)
	fmt.Printf("Min evidence to keep: %d\n", cfg.MinEvidence)
}

// printDecayStats outputs the job statistics
func printDecayStats(stats decayStats, dryRun bool) {
	fmt.Println("\nStatistics:")
	fmt.Printf("- Edges scanned: %d\n", stats.scanned)
	fmt.Printf("- Edges decayed: %d\n", stats.decayed)
	if dryRun {
		fmt.Printf("- Edges to prune: %d\n", stats.pruned)
	} else {
		fmt.Printf("- Edges pruned: %d\n", stats.pruned)
	}
	fmt.Printf("- Edges protected (high evidence): %d\n", stats.protectedEvidence)
	fmt.Printf("- Edges protected (pinned): %d\n", stats.protectedPinned)

	// Print sample decays
	if len(stats.samples) > 0 {
		fmt.Println("\nSample decays:")
		for _, r := range stats.samples {
			fmt.Printf("  [%s]-%s->[%s]: %.3f -> %.3f (%.1f%% decay)\n",
				r.Edge.SourceID, r.Edge.RelType, r.Edge.TargetID,
				r.OldWeight, r.NewWeight, r.DecayPercent)
		}
	}

	if dryRun {
		fmt.Println("\nRun with --dry-run=false to apply changes.")
	} else {
		fmt.Println("\nChanges applied successfully.")
	}
}
