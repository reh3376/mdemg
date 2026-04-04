package cli

import (
	"context"
	"fmt"

	neo4j "github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
)

func newEmbeddingsBackfillCmd() *cobra.Command {
	var spaceID string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Fill missing embeddings for MemoryNodes",
		Long: `Finds MemoryNodes that have content but missing embeddings and generates
embeddings for them using the configured embedding provider.`,
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

			if dryRun {
				return backfillDryRun(ctx, driver, spaceID)
			}
			return reEmbedNodes(ctx, driver, spaceID)
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Space ID to backfill (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Only count nodes needing embeddings")

	return cmd
}

func backfillDryRun(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.content IS NOT NULL AND n.content <> '' AND n.embedding IS NULL
RETURN count(n) AS cnt`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			cnt, _ := res.Record().Get("cnt")
			return cnt, nil
		}
		return int64(0), res.Err()
	})
	if err != nil {
		return err
	}
	fmt.Printf("[dry-run] %v MemoryNodes need embedding backfill in space %s\n", result, spaceID)
	return nil
}
