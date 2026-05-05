// Phase 14.2 Epic 5 — `mdemg migrate context-fingerprint` backfill CLI.
//
// Walks every observation MemoryNode in a space whose
// context_fingerprint_version is older than the active catalog and
// recomputes its observe-time-style fingerprint (path + role_type×layer +
// tags). Idempotent: skips observations already at the active version.
// Streaming Neo4j cursor pattern from graph_repair.go.
//
// New observations get fingerprints automatically (Epic 3 wiring on
// Service.Observe). This CLI is for historical observations on legacy
// spaces. Refresh cadence is weekly via the CycleOrchestrator stage 6
// hook (Epic 2); this CLI is the one-shot operator-run alternative.
package cli

import (
	"context"
	"fmt"
	"time"

	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/config"
	"mdemg/internal/conversation"
	"mdemg/internal/hidden"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Schema + data migrations",
		Long: `Schema + data migrations across the MDEMG storage tier.

Subcommands:
  context-fingerprint   Backfill Phase 14.2 sparse fingerprints on existing
                        observations (Note 05).
`,
	}
	cmd.AddCommand(newMigrateContextFingerprintCmd())
	return cmd
}

func newMigrateContextFingerprintCmd() *cobra.Command {
	var spaceID string
	var dryRun bool
	var batchSize int
	var build bool

	cmd := &cobra.Command{
		Use:   "context-fingerprint",
		Short: "Backfill Phase 14.2 sparse context fingerprints on existing observations",
		Long: `Backfills Phase 14.2 (Note 05) sparse context fingerprints on
historical MemoryNodes that pre-date the feature.

Algorithm:
  1. Look up the active ContextCatalog for --space-id. If none exists and
     --build is set, run the adaptive Builder against this space first.
  2. Stream every MemoryNode in the space whose
     context_fingerprint_version < active.version (idempotent skip on equal).
  3. For each node, compute the observe-time-style fingerprint from
     path + role_type + layer + tags (matches the live observe path).
  4. Write context_fingerprint_active + _version in batches.

  mdemg migrate context-fingerprint --space-id mdemg-dev --dry-run         # preview
  mdemg migrate context-fingerprint --space-id mdemg-dev --dry-run=false   # execute
  mdemg migrate context-fingerprint --space-id newspace  --build           # cold-start: build catalog first
`,
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

			cfg, cerr := config.FromEnv()
			if cerr != nil {
				return fmt.Errorf("config: %w", cerr)
			}
			return runMigrateContextFingerprint(ctx, driver, cfg, spaceID, dryRun, batchSize, build)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space to backfill (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview mode (default: true)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 500, "Process observations in batches of this size")
	cmd.Flags().BoolVar(&build, "build", false, "Build a new catalog if no active version exists (cold start)")

	return cmd
}

func runMigrateContextFingerprint(
	ctx context.Context,
	driver neo4j.DriverWithContext,
	cfg config.Config,
	spaceID string,
	dryRun bool,
	batchSize int,
	build bool,
) error {
	fmt.Println("MDEMG Context Fingerprint Backfill (Phase 14.2)")
	fmt.Println("================================================")
	fmt.Printf("Space:      %s\n", spaceID)
	fmt.Printf("Batch:      %d\n", batchSize)
	if dryRun {
		fmt.Println("Mode:       DRY RUN (no changes)")
	} else {
		fmt.Println("Mode:       LIVE (changes will be applied)")
	}
	fmt.Println()

	// Step 1: load (or build) active catalog.
	loader := hidden.NewNeo4jLoader(driver)
	cat, err := loader.LoadActive(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("load active catalog: %w", err)
	}
	if cat == nil {
		if !build {
			return fmt.Errorf("no active ContextCatalog for space=%q; pass --build to bootstrap one (or run a CycleOrchestrator macro tick)", spaceID)
		}
		if dryRun {
			return fmt.Errorf("no active catalog and --dry-run is set; cannot build catalog in dry-run mode (re-run with --dry-run=false --build)")
		}
		fmt.Println("Step 0: No active catalog; building via adaptive Builder...")
		builder := hidden.NewNeo4jBuilder(driver, hidden.BuilderOptsFromConfig(cfg))
		cat, err = builder.BuildForSpace(ctx, spaceID)
		if err != nil {
			return fmt.Errorf("build catalog: %w", err)
		}
		fmt.Printf("  Built version=%d total_bits=%d\n", cat.Version, cat.TotalBits)
	} else {
		fmt.Printf("Active catalog: version=%d total_bits=%d\n", cat.Version, cat.TotalBits)
	}
	fmt.Println()

	// Step 2: stream nodes needing refresh.
	type obsRow struct {
		nodeID   string
		roleType string
		layer    int
		tags     []string
		filePath string
		curVer   int
	}

	startedAt := time.Now()
	var (
		scanned int
		updated int
		skipped int
	)

	// Single Cypher walk that returns every MemoryNode whose fingerprint
	// version is older than the active catalog. Keyed on node_id (not
	// obs_id) so code-derived MemoryNodes (which have no obs_id) are also
	// backfilled — fingerprint is a property of the node itself, not just
	// conversation observations. Filter at query-time so we don't ship
	// every node over the wire.
	queryQ := `
		MATCH (m:MemoryNode {space_id: $space_id})
		WHERE coalesce(m.context_fingerprint_version, 0) < $target_version
		  AND m.role_type IS NOT NULL
		RETURN m.node_id AS node_id,
		       coalesce(m.role_type, 'conversation_observation') AS role_type,
		       coalesce(m.layer, 0) AS layer,
		       coalesce(m.tags, []) AS tags,
		       coalesce(m.path, '') AS file_path,
		       coalesce(m.context_fingerprint_version, 0) AS cur_ver
	`
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	rowsRaw, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, queryQ, map[string]any{
			"space_id":       spaceID,
			"target_version": int64(cat.Version),
		})
		if err != nil {
			return nil, err
		}
		var rows []obsRow
		for res.Next(ctx) {
			rec := res.Record()
			nodeID, _ := rec.Get("node_id")
			roleType, _ := rec.Get("role_type")
			layerRaw, _ := rec.Get("layer")
			tagsRaw, _ := rec.Get("tags")
			pathRaw, _ := rec.Get("file_path")
			verRaw, _ := rec.Get("cur_ver")
			if nodeID == nil {
				continue
			}
			row := obsRow{
				nodeID:   fmt.Sprint(nodeID),
				roleType: fmt.Sprint(roleType),
				filePath: fmt.Sprint(pathRaw),
			}
			if li, ok := layerRaw.(int64); ok {
				row.layer = int(li)
			}
			if tarr, ok := tagsRaw.([]any); ok {
				for _, t := range tarr {
					if s, ok := t.(string); ok {
						row.tags = append(row.tags, s)
					}
				}
			}
			if vi, ok := verRaw.(int64); ok {
				row.curVer = int(vi)
			}
			rows = append(rows, row)
		}
		return rows, res.Err()
	})
	_ = sess.Close(ctx)
	if err != nil {
		return fmt.Errorf("scan observations: %w", err)
	}
	rows, _ := rowsRaw.([]obsRow)
	scanned = len(rows)
	fmt.Printf("Step 1: Found %d observations < version %d\n", scanned, cat.Version)
	if scanned == 0 {
		fmt.Println("Nothing to do — all observations are up to date.")
		return nil
	}
	fmt.Println()

	// Step 3: compute fingerprint per row + batch-write.
	type writeBatch struct {
		nodeIDs []string
		bits    [][]int64
	}
	var batch writeBatch
	flush := func() error {
		if len(batch.nodeIDs) == 0 {
			return nil
		}
		if dryRun {
			batch = writeBatch{}
			return nil
		}
		writeQ := `
			UNWIND $rows AS row
			MATCH (m:MemoryNode {node_id: row.node_id})
			SET m.context_fingerprint_active = row.bits,
			    m.context_fingerprint_version = $version
		`
		writeRows := make([]map[string]any, len(batch.nodeIDs))
		for i, id := range batch.nodeIDs {
			writeRows[i] = map[string]any{
				"node_id": id,
				"bits":    batch.bits[i],
			}
		}
		ws := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		_, werr := ws.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, writeQ, map[string]any{
				"rows":    writeRows,
				"version": int64(cat.Version),
			})
			return nil, err
		})
		_ = ws.Close(ctx)
		if werr != nil {
			return werr
		}
		batch = writeBatch{}
		return nil
	}

	for i, row := range rows {
		obs := &conversation.Observation{
			Tags: row.tags,
			Metadata: map[string]any{
				"role_type": row.roleType,
				"layer":     row.layer,
			},
		}
		if row.filePath != "" {
			obs.Metadata["file_path"] = row.filePath
		}
		fp := conversation.ComputeContextFingerprintLocal(obs, cat)
		if fp == nil {
			skipped++
			continue
		}
		batch.nodeIDs = append(batch.nodeIDs, row.nodeID)
		batch.bits = append(batch.bits, uint16ToInt64(fp))
		if len(batch.nodeIDs) >= batchSize {
			if err := flush(); err != nil {
				return fmt.Errorf("flush batch: %w", err)
			}
		}
		if (i+1)%(batchSize) == 0 || i == len(rows)-1 {
			fmt.Printf("  progress: %d/%d processed\n", i+1, len(rows))
		}
	}
	if err := flush(); err != nil {
		return fmt.Errorf("final flush: %w", err)
	}
	updated = scanned - skipped

	fmt.Println()
	fmt.Printf("Done. scanned=%d updated=%d no-bits-skipped=%d duration=%s\n",
		scanned, updated, skipped, time.Since(startedAt).Truncate(time.Millisecond))
	if dryRun {
		fmt.Println("(no writes performed; rerun with --dry-run=false to apply)")
	}
	return nil
}

func uint16ToInt64(in []uint16) []int64 {
	out := make([]int64, len(in))
	for i, v := range in {
		out[i] = int64(v)
	}
	return out
}
