package cli

import (
	"context"
	"fmt"

	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/cli/neo4jutil"
)

func newGraphRepairCmd() *cobra.Command {
	var spaceID string
	var dryRun bool
	var batchSize int

	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Repair graph health issues (run before upgrading)",
		Long: `Repairs common graph health issues:

  1. Removes nodes matching .mdemgignore patterns (vendor cleanup)
  2. Merges duplicate SymbolNodes (keeps oldest, aggregates edge weights)
  3. Sweeps orphaned nodes across all labels
  4. Backfills missing embeddings
  5. Validates readiness for V0023 uniqueness constraint

Run this command before upgrading to versions that include the
SymbolNode natural-key constraint (V0023). The repair is idempotent
and safe to run multiple times.

  mdemg graph repair --space-id myspace --dry-run   # Preview
  mdemg graph repair --space-id myspace --dry-run=false  # Execute`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}

			ctx := context.Background()
			driver, err := newDriver()
			if err != nil {
				return fmt.Errorf("neo4j driver: %w", err)
			}
			defer func() { _ = driver.Close(ctx) }()

			if err := driver.VerifyConnectivity(ctx); err != nil {
				return fmt.Errorf("neo4j connectivity: %w", err)
			}

			return runGraphRepair(ctx, driver, spaceID, dryRun, batchSize)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space to repair (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview mode (default: true)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 500, "Process items in batches of this size")

	return cmd
}

func runGraphRepair(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, dryRun bool, batchSize int) error {
	fmt.Println("MDEMG Graph Repair")
	fmt.Println("==================")
	fmt.Printf("Space: %s\n", spaceID)
	if dryRun {
		fmt.Println("Mode:  DRY RUN (no changes)")
	} else {
		fmt.Println("Mode:  LIVE (changes will be applied)")
	}
	fmt.Println()

	// Step 1: Vendor cleanup (match-ignore)
	fmt.Println("Step 1: Vendor / ignore-pattern cleanup")
	pCfg := pruneConfig{
		SpaceID:   spaceID,
		DryRun:    dryRun,
		BatchSize: batchSize,
	}
	ignoreStats, err := sweepIgnoreMatches(ctx, driver, pCfg)
	if err != nil {
		return fmt.Errorf("vendor cleanup: %w", err)
	}
	fmt.Printf("  Found: %d nodes matching .mdemgignore\n", ignoreStats.found)
	if !dryRun {
		fmt.Printf("  Deleted: %d\n", ignoreStats.deleted)
	}
	fmt.Println()

	// Step 2: SymbolNode dedup with weight aggregation
	fmt.Println("Step 2: SymbolNode deduplication")
	dupGroups, dupExcess, err := countSymbolDuplicates(ctx, driver, spaceID)
	if err != nil {
		return fmt.Errorf("count duplicates: %w", err)
	}
	fmt.Printf("  Duplicate groups: %d (%d excess nodes)\n", dupGroups, dupExcess)
	if dupGroups > 0 && !dryRun {
		merged, err := mergeSymbolDuplicates(ctx, driver, spaceID, batchSize)
		if err != nil {
			return fmt.Errorf("merge duplicates: %w", err)
		}
		fmt.Printf("  Merged: %d excess nodes removed\n", merged)
	}
	fmt.Println()

	// Step 3: Orphan sweep
	fmt.Println("Step 3: Orphan sweep")
	symStats, err := sweepOrphanSymbolNodes(ctx, driver, pCfg)
	if err != nil {
		return fmt.Errorf("sweep orphan symbols: %w", err)
	}
	fmt.Printf("  Orphan SymbolNodes: %d\n", symStats.found)

	obsStats, err := sweepOrphanObservations(ctx, driver, pCfg)
	if err != nil {
		return fmt.Errorf("sweep orphan observations: %w", err)
	}
	fmt.Printf("  Orphan Observations: %d\n", obsStats.found)
	fmt.Println()

	// Step 4: Embedding backfill
	fmt.Println("Step 4: Embedding backfill")
	if dryRun {
		err = backfillDryRun(ctx, driver, spaceID)
	} else {
		err = reEmbedNodes(ctx, driver, spaceID)
	}
	if err != nil {
		fmt.Printf("  Warning: embedding backfill failed: %v\n", err)
		// Non-fatal — continue to readiness check
	}
	fmt.Println()

	// Step 5: V0023 readiness check
	fmt.Println("Step 5: V0023 constraint readiness")
	remainingGroups, _, err := countSymbolDuplicates(ctx, driver, spaceID)
	if err != nil {
		return fmt.Errorf("readiness check: %w", err)
	}
	if remainingGroups == 0 {
		fmt.Println("  Status: READY — zero duplicate groups")
	} else {
		fmt.Printf("  Status: NOT READY — %d duplicate groups remain\n", remainingGroups)
		fmt.Println("  Run again with --dry-run=false to merge duplicates")
	}

	fmt.Println()
	fmt.Println("Graph repair complete.")
	return nil
}

// countSymbolDuplicates returns the number of duplicate groups and total excess nodes.
func countSymbolDuplicates(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (groups int, excess int, err error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $spaceId})
WITH s.name AS name, s.file_path AS fp, s.symbol_type AS st, count(s) AS cnt
WHERE cnt > 1
RETURN count(*) AS groups, sum(cnt - 1) AS excess`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			g, _ := rec.Get("groups")
			e, _ := rec.Get("excess")
			return []int{neo4jutil.AsInt(g), neo4jutil.AsInt(e)}, res.Err()
		}
		return []int{0, 0}, res.Err()
	})
	if err != nil {
		return 0, 0, err
	}
	counts := result.([]int)
	return counts[0], counts[1], nil
}

// mergeSymbolDuplicates merges duplicate SymbolNode groups with CO_ACTIVATED_WITH
// weight aggregation. Returns total number of excess nodes removed.
func mergeSymbolDuplicates(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, batchSize int) (int, error) {
	totalMerged := 0

	for {
		sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})

		// Find one batch of duplicate groups
		result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $spaceId})
WITH s.name AS name, s.file_path AS fp, s.symbol_type AS st,
     collect(s) AS dupes
WHERE size(dupes) > 1
RETURN dupes LIMIT $limit`, map[string]any{"spaceId": spaceID, "limit": batchSize})
			if err != nil {
				return nil, err
			}
			var groups [][]neo4j.Node
			for res.Next(ctx) {
				rec := res.Record()
				raw, _ := rec.Get("dupes")
				nodeList := raw.([]any)
				var nodes []neo4j.Node
				for _, n := range nodeList {
					nodes = append(nodes, n.(neo4j.Node))
				}
				groups = append(groups, nodes)
			}
			return groups, res.Err()
		})
		sess.Close(ctx)
		if err != nil {
			return totalMerged, fmt.Errorf("find duplicates: %w", err)
		}

		groups := result.([][]neo4j.Node)
		if len(groups) == 0 {
			break
		}

		// Process each group
		for _, group := range groups {
			keeper := group[0]
			victims := group[1:]
			keeperID := keeper.ElementId

			for _, victim := range victims {
				victimID := victim.ElementId
				if err := mergeOneSymbolNode(ctx, driver, spaceID, keeperID, victimID); err != nil {
					return totalMerged, fmt.Errorf("merge node %s: %w", victimID, err)
				}
				totalMerged++
			}
		}
		fmt.Printf("  Merged batch: %d total so far\n", totalMerged)
	}

	return totalMerged, nil
}

// mergeOneSymbolNode transfers edges from victim to keeper, aggregating
// CO_ACTIVATED_WITH weights, then deletes the victim.
func mergeOneSymbolNode(ctx context.Context, driver neo4j.DriverWithContext, spaceID, keeperID, victimID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// 1. Transfer DEFINES_SYMBOL edges (incoming)
		_, err := tx.Run(ctx, `
MATCH (m)-[r:DEFINES_SYMBOL]->(victim)
WHERE elementId(victim) = $victimId
AND NOT EXISTS { MATCH (m)-[:DEFINES_SYMBOL]->(keeper) WHERE elementId(keeper) = $keeperId }
WITH m, r, victim
MATCH (keeper) WHERE elementId(keeper) = $keeperId
CREATE (m)-[:DEFINES_SYMBOL {space_id: r.space_id}]->(keeper)
DELETE r`,
			map[string]any{"victimId": victimID, "keeperId": keeperID})
		if err != nil {
			return nil, fmt.Errorf("transfer DEFINES_SYMBOL: %w", err)
		}

		// 2. Transfer CALLS/IMPORTS/EXTENDS/IMPLEMENTS (outgoing from victim)
		for _, relType := range []string{"CALLS", "IMPORTS", "EXTENDS", "IMPLEMENTS"} {
			_, err = tx.Run(ctx, fmt.Sprintf(`
MATCH (victim)-[r:%s]->(target)
WHERE elementId(victim) = $victimId
AND NOT EXISTS { MATCH (keeper)-[:%s]->(target) WHERE elementId(keeper) = $keeperId }
WITH r, target
MATCH (keeper) WHERE elementId(keeper) = $keeperId
CREATE (keeper)-[nr:%s]->(target)
SET nr = properties(r)
DELETE r`, relType, relType, relType),
				map[string]any{"victimId": victimID, "keeperId": keeperID})
			if err != nil {
				return nil, fmt.Errorf("transfer %s: %w", relType, err)
			}
		}

		// 3. Aggregate CO_ACTIVATED_WITH edges (outgoing)
		_, err = tx.Run(ctx, `
MATCH (victim)-[vr:CO_ACTIVATED_WITH]->(target)
WHERE elementId(victim) = $victimId AND elementId(target) <> $keeperId
WITH vr, target
MATCH (keeper) WHERE elementId(keeper) = $keeperId
MERGE (keeper)-[kr:CO_ACTIVATED_WITH {space_id: $spaceId}]->(target)
ON CREATE SET kr.weight = vr.weight,
              kr.evidence_count = coalesce(vr.evidence_count, 1),
              kr.created_at = coalesce(vr.created_at, datetime()),
              kr.updated_at = datetime()
ON MATCH SET  kr.weight = kr.weight + vr.weight,
              kr.evidence_count = kr.evidence_count + coalesce(vr.evidence_count, 1),
              kr.updated_at = datetime()
DELETE vr`,
			map[string]any{"victimId": victimID, "keeperId": keeperID, "spaceId": spaceID})
		if err != nil {
			return nil, fmt.Errorf("aggregate CO_ACTIVATED_WITH outgoing: %w", err)
		}

		// 4. Aggregate CO_ACTIVATED_WITH edges (incoming)
		_, err = tx.Run(ctx, `
MATCH (source)-[vr:CO_ACTIVATED_WITH]->(victim)
WHERE elementId(victim) = $victimId AND elementId(source) <> $keeperId
WITH vr, source
MATCH (keeper) WHERE elementId(keeper) = $keeperId
MERGE (source)-[kr:CO_ACTIVATED_WITH {space_id: $spaceId}]->(keeper)
ON CREATE SET kr.weight = vr.weight,
              kr.evidence_count = coalesce(vr.evidence_count, 1),
              kr.created_at = coalesce(vr.created_at, datetime()),
              kr.updated_at = datetime()
ON MATCH SET  kr.weight = kr.weight + vr.weight,
              kr.evidence_count = kr.evidence_count + coalesce(vr.evidence_count, 1),
              kr.updated_at = datetime()
DELETE vr`,
			map[string]any{"victimId": victimID, "keeperId": keeperID, "spaceId": spaceID})
		if err != nil {
			return nil, fmt.Errorf("aggregate CO_ACTIVATED_WITH incoming: %w", err)
		}

		// 5. Delete remaining edges on victim and the victim node
		_, err = tx.Run(ctx, `
MATCH (victim) WHERE elementId(victim) = $victimId
DETACH DELETE victim`,
			map[string]any{"victimId": victimID})
		if err != nil {
			return nil, fmt.Errorf("delete victim: %w", err)
		}

		return nil, nil
	})

	return err
}
