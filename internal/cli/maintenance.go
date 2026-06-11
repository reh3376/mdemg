package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"

	"mdemg/internal/config"
)

func newMaintenanceCmd() *cobra.Command {
	var spaceID string
	var dryRun bool
	var excludeRoleTypes []string

	cmd := &cobra.Command{
		Use:   "maintenance",
		Short: "Run decay + prune maintenance cycle",
		Long: `Runs a full maintenance cycle: edge decay followed by orphan pruning.
Equivalent to running 'mdemg decay' then 'mdemg prune' with sensible defaults.
Suitable for scheduling via launchd, systemd, or cron.`,
		RunE: func(cmd *cobra.Command, args []string) (retErr error) {
			if spaceID == "" {
				spaceID = resolveSpaceID(cmd)
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}

			// NOSILENT-001: record this run's outcome + alert on failure so a
			// failing scheduled maintenance cycle is never silent.
			// MAINT-LIVE-001: record dry_run so the maintenance_no_live_run
			// rule can detect a schedule that only ever previews.
			startedAt := time.Now()
			defer func() {
				reportScheduledJobMeta("maintenance", spaceID, startedAt, retErr,
					map[string]any{"dry_run": dryRun})
			}()

			ctx := context.Background()

			// Load config (YAML → .env → env vars)
			if cfgPath := config.FindConfigFile(); cfgPath != "" {
				_ = config.LoadYAMLConfig(cfgPath)
			}
			_ = godotenv.Load()

			// Load Neo4j credentials once
			decayCfg := decayConfig{
				DecayRate:       0.02,
				PruneThreshold:  0.01,
				MinEvidence:     3,
				OlderThanDays:   7,
				MaxDecayPercent: 50.0,
				DryRun:          dryRun,
				SpaceID:         spaceID,
				BatchSize:       1000,
			}
			if err := parseNeo4jEnv(&decayCfg); err != nil {
				return fmt.Errorf("neo4j env: %w", err)
			}

			// Step 1: Decay
			fmt.Println("=== Maintenance: Step 1 — Edge Decay ===")
			decayDriver, err := newDecayDriver(decayCfg)
			if err != nil {
				return fmt.Errorf("decay driver: %w", err)
			}
			if err := decayDriver.VerifyConnectivity(ctx); err != nil {
				_ = decayDriver.Close(ctx)
				return fmt.Errorf("neo4j connectivity: %w", err)
			}
			if err := runDecayJob(ctx, decayDriver, decayCfg); err != nil {
				_ = decayDriver.Close(ctx)
				return fmt.Errorf("decay: %w", err)
			}
			_ = decayDriver.Close(ctx)

			// Step 2: Prune
			fmt.Println("\n=== Maintenance: Step 2 — Orphan Pruning ===")
			pruneCfg := pruneConfig{
				Neo4jURI:         decayCfg.Neo4jURI,
				Neo4jUser:        decayCfg.Neo4jUser,
				Neo4jPass:        decayCfg.Neo4jPass,
				WeightThreshold:  0.01,
				MinEvidence:      3,
				OlderThanDays:    30,
				RetentionDays:    90,
				MaxDegree:        1,
				IncludeLabels:    []string{"MemoryNode", "SymbolNode", "Observation"},
				ExcludeRoleTypes: excludeRoleTypes,
				DryRun:           dryRun,
				SpaceID:          spaceID,
				BatchSize:        1000,
			}
			pruneDriver, err := newPruneDriver(pruneCfg)
			if err != nil {
				return fmt.Errorf("prune driver: %w", err)
			}
			if err := runPruneJob(ctx, pruneDriver, pruneCfg); err != nil {
				_ = pruneDriver.Close(ctx)
				return fmt.Errorf("prune: %w", err)
			}
			_ = pruneDriver.Close(ctx)

			fmt.Println("\n=== Maintenance Complete ===")
			return nil
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID to maintain (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview mode (default: true)")
	cmd.Flags().StringSliceVar(&excludeRoleTypes, "exclude-role-types",
		envCSV("PRUNE_EXCLUDE_ROLE_TYPES"),
		"role_type values never tombstoned (env PRUNE_EXCLUDE_ROLE_TYPES)")

	return cmd
}
