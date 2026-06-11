// HIDDEN-WEIGHT-001 — backfill NULL weights on abstraction edges.
//
// point.distance() (a spatial-Point function) silently returned NULL on
// embedding lists at all three abstraction-edge creation sites, so 100% of
// GENERALIZES and 95% of ABSTRACTS_TO edges carried no weight. The creation
// sites are fixed (vector.similarity.cosine); this command heals the
// existing graph. Weights are a pure function of endpoint embeddings, so
// the backfill is idempotent and re-runnable.
//
// Deliberately a STANDALONE subcommand (not folded into `graph repair`):
// repair's orphan sweep would also delete pre-fix orphan observations that
// the operator chose to keep as historical record (2026-06-10 decision).
package cli

import (
	"context"
	"fmt"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
)

func newGraphBackfillWeightsCmd() *cobra.Command {
	var (
		spaceID   string
		dryRun    bool
		batchSize int
		limit     int
	)

	cmd := &cobra.Command{
		Use:   "backfill-weights",
		Short: "Backfill NULL weights on GENERALIZES/ABSTRACTS_TO edges",
		Long: `Backfills NULL weights on abstraction edges (HIDDEN-WEIGHT-001).

Weight = vector.similarity.cosine(endpoint embeddings) when both exist,
else 0.5 (the creation sites' own fallback). similarity_score is set
alongside. Idempotent — weights are a pure function of embeddings.

  mdemg graph backfill-weights --space-id myspace              # Preview
  mdemg graph backfill-weights --space-id myspace --limit 5 --dry-run=false   # Small-batch trial
  mdemg graph backfill-weights --space-id myspace --dry-run=false             # Full run`,
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

			return runWeightBackfill(ctx, driver, spaceID, dryRun, batchSize, limit)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space to backfill (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Preview mode (default: true)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Edges updated per transaction")
	cmd.Flags().IntVar(&limit, "limit", 0, "Stop after N edges (0 = all; use 5 for a verified trial first)")

	return cmd
}

func runWeightBackfill(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, dryRun bool, batchSize, limit int) error {
	fmt.Println("MDEMG Abstraction-Weight Backfill (HIDDEN-WEIGHT-001)")
	fmt.Println("=====================================================")
	fmt.Printf("Space: %s\n", spaceID)
	if dryRun {
		fmt.Println("Mode:  DRY RUN (no changes)")
	} else if limit > 0 {
		fmt.Printf("Mode:  LIVE — capped at %d edges\n", limit)
	} else {
		fmt.Println("Mode:  LIVE (full backfill)")
	}

	found, err := countNullWeightEdges(ctx, driver, spaceID)
	if err != nil {
		return fmt.Errorf("count: %w", err)
	}
	fmt.Printf("NULL-weight abstraction edges: %d\n", found)
	if dryRun || found == 0 {
		return nil
	}

	updated := 0
	for {
		batch := batchSize
		if limit > 0 && updated+batch > limit {
			batch = limit - updated
		}
		if batch <= 0 {
			break
		}
		n, err := backfillNullWeightBatch(ctx, driver, spaceID, batch)
		if err != nil {
			return fmt.Errorf("backfill batch (after %d updated): %w", updated, err)
		}
		if n == 0 {
			break
		}
		updated += n
		fmt.Printf("  updated %d / %d\n", updated, found)
	}

	remaining, err := countNullWeightEdges(ctx, driver, spaceID)
	if err != nil {
		return fmt.Errorf("post-count: %w", err)
	}
	fmt.Printf("Updated: %d | Remaining NULL: %d\n", updated, remaining)
	return nil
}

func countNullWeightEdges(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	res, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		r, err := tx.Run(ctx, `
MATCH ()-[r:GENERALIZES|ABSTRACTS_TO]->()
WHERE r.space_id = $spaceId AND r.weight IS NULL
RETURN count(r) AS c`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if r.Next(ctx) {
			c, _ := r.Record().Get("c")
			return int(c.(int64)), r.Err()
		}
		return 0, r.Err()
	})
	if err != nil {
		return 0, err
	}
	return res.(int), nil
}

// backfillNullWeightBatch fills up to `batch` NULL weights in one write
// transaction. Cosine when both endpoint embeddings exist; 0.5 fallback
// (matching the creation sites). Sets similarity_score alongside.
func backfillNullWeightBatch(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, batch int) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	res, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		r, err := tx.Run(ctx, `
MATCH (a)-[r:GENERALIZES|ABSTRACTS_TO]->(b)
WHERE r.space_id = $spaceId AND r.weight IS NULL
WITH a, r, b LIMIT $batch
WITH a, r, b,
     CASE WHEN a.embedding IS NOT NULL AND b.embedding IS NOT NULL
          THEN vector.similarity.cosine(a.embedding, b.embedding)
          ELSE 0.5
     END AS sim
SET r.weight = sim,
    r.similarity_score = sim,
    r.updated_at = datetime()
RETURN count(r) AS updated`, map[string]any{"spaceId": spaceID, "batch": batch})
		if err != nil {
			return 0, err
		}
		if r.Next(ctx) {
			u, _ := r.Record().Get("updated")
			return int(u.(int64)), r.Err()
		}
		return 0, r.Err()
	})
	if err != nil {
		return 0, err
	}
	return res.(int), nil
}
