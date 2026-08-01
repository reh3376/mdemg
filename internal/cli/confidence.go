// Package cli provides Cobra commands for the unified MDEMG CLI.
package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/hidden"
)

// newConfidenceCmd creates the `mdemg confidence` group — HEBB-ETA-001.
func newConfidenceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "confidence",
		Short: "Per-node activation confidence (HEBB-ETA-001)",
		Long: `Per-node activation confidence commands for the precision-weighted
Hebbian η feature (HEBB-ETA-001). Ships with PRECISION_WEIGHTED_ETA_ENABLED
default false — backfill nodes with confidence values BEFORE enabling the
flag, else the Cypher fallback of 0.5 will multiplicatively shrink η by
4× for every un-backfilled pair.`,
	}
	cmd.AddCommand(newConfidenceBackfillCmd())
	return cmd
}

// newConfidenceBackfillCmd — walks nodes in a space and writes ActivationConfidence
// computed from ComputeActivationConfidence over (reinforce_count, last_activated_at,
// surprise_history). Idempotent: safe to re-run on schedule.
func newConfidenceBackfillCmd() *cobra.Command {
	var (
		spaceID   string
		neo4jURI  string
		neo4jUser string
		neo4jPass string
		batchSize int
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Backfill MemoryNode.activation_confidence for a space",
		Long: `Compute + store per-node ActivationConfidence for every non-archived
MemoryNode in the given space. Idempotent; safe to re-run.

The confidence formula (see internal/hidden/confidence.go):
  c = sigmoid( alpha*log(1+n_reinforce) + beta*recency_decay(halflife) - gamma*surprise_variance )
  clamped to [0.05, 1.0]

Reinforcement count comes from Neo4j MATCH ()-[r:CO_ACTIVATED_WITH]->(n) — the
sum of evidence_count on incoming edges. Last activation is the max last_activated_at
on those edges (or created_at if no edges). Surprise history is currently the node's
own surprise_score (single value → no variance penalty). A richer history would come
from the reinforcement_events TSDB table but is deferred to a follow-up.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("--space-id required")
			}
			if neo4jURI == "" {
				neo4jURI = os.Getenv("NEO4J_URI")
			}
			if neo4jUser == "" {
				neo4jUser = os.Getenv("NEO4J_USER")
			}
			if neo4jPass == "" {
				neo4jPass = os.Getenv("NEO4J_PASS")
			}
			if neo4jURI == "" || neo4jUser == "" || neo4jPass == "" {
				return fmt.Errorf("NEO4J_URI/NEO4J_USER/NEO4J_PASS are required (via flags or env)")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()

			driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPass, ""))
			if err != nil {
				return fmt.Errorf("neo4j: %w", err)
			}
			defer driver.Close(ctx)

			cfg := hidden.DefaultConfidenceConfig()
			slog.Info("confidence backfill start",
				"space_id", spaceID, "batch_size", batchSize, "dry_run", dryRun,
				"alpha", cfg.Alpha, "beta", cfg.Beta, "gamma", cfg.Gamma, "half_life_sec", cfg.HalfLifeSec)

			n, err := hidden.BackfillActivationConfidence(ctx, driver, spaceID, cfg, batchSize, dryRun)
			if err != nil {
				return fmt.Errorf("backfill: %w", err)
			}
			mode := "wrote"
			if dryRun {
				mode = "would-write"
			}
			fmt.Printf("HEBB-ETA-001: %s activation_confidence on %d nodes (space=%s)\n", mode, n, spaceID)
			return nil
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space to backfill (required)")
	cmd.Flags().StringVar(&neo4jURI, "neo4j-uri", "", "Neo4j URI (default: $NEO4J_URI)")
	cmd.Flags().StringVar(&neo4jUser, "neo4j-user", "", "Neo4j user (default: $NEO4J_USER)")
	cmd.Flags().StringVar(&neo4jPass, "neo4j-pass", "", "Neo4j password (default: $NEO4J_PASS)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 500, "Nodes per batch")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Compute + report only; do not write")

	return cmd
}
