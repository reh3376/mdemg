// Sprint INGEST-TOPOLOGY-REPAIR-001 Epic 3 — backfill MemoryNode.content from
// the latest linked HAS_OBSERVATION.content for legacy-ingest nodes never
// re-ingested after the E1 ingest fix landed. Idempotent + additive-only
// (never overwrites a non-null n.content). Safe to run repeatedly.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

type backfillNodeContentConfig struct {
	spaceID   string
	dryRun    bool
	batchSize int
	limit     int
	verbose   bool
}

func newBackfillNodeContentCmd() *cobra.Command {
	opts := &backfillNodeContentConfig{}
	cmd := &cobra.Command{
		Use:   "backfill-node-content",
		Short: "Lift latest HAS_OBSERVATION.content to MemoryNode.content for legacy-ingest nodes",
		Long: `Backfills n.content from the latest Observation.content for nodes that
have HAS_OBSERVATION edges but no direct n.content property. This is the
one-time repair for nodes ingested before INGEST-TOPOLOGY-REPAIR-001 shipped
the ingest fix that writes n.content directly on the MemoryNode.

Idempotent: never overwrites a non-null n.content. Additive-only: safe to
run repeatedly. Runs against the space specified by --space-id or from
MDEMG_SPACE_ID env; defaults to mdemg-dev.

Batches via LIMIT to avoid stressing Neo4j on very large substrates.
Deterministic Observation selection via ORDER BY o.created_at DESC LIMIT 1
(matches the read-path fetchNodeContents helper).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBackfillNodeContent(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.spaceID, "space-id", "",
		"Target MDEMG space (default: MDEMG_SPACE_ID env, else mdemg-dev)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false,
		"Report count of candidate nodes without writing")
	cmd.Flags().IntVar(&opts.batchSize, "batch-size", 500,
		"Max nodes to backfill per Cypher call (default 500)")
	cmd.Flags().IntVar(&opts.limit, "limit", 0,
		"Cap total nodes to backfill across all batches (0 = unlimited)")
	cmd.Flags().BoolVar(&opts.verbose, "verbose", false, "Verbose per-batch logging")
	return cmd
}

func runBackfillNodeContent(ctx context.Context, opts *backfillNodeContentConfig) error {
	if opts.spaceID == "" {
		if v := os.Getenv("MDEMG_SPACE_ID"); v != "" {
			opts.spaceID = v
		} else {
			opts.spaceID = "mdemg-dev"
		}
	}
	if opts.batchSize <= 0 {
		opts.batchSize = 500
	}

	cfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	driver, err := neo4jDriverFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}
	defer driver.Close(ctx)

	// Count candidates first — informational + gate for --dry-run.
	candidateCount, err := backfillCountCandidates(ctx, driver, opts.spaceID)
	if err != nil {
		return fmt.Errorf("count candidates: %w", err)
	}
	fmt.Printf("backfill-node-content: %d candidate nodes in space=%s (nodes with HAS_OBSERVATION but no n.content)\n",
		candidateCount, opts.spaceID)

	if opts.dryRun {
		fmt.Println("[dry-run] no writes performed")
		return nil
	}
	if candidateCount == 0 {
		fmt.Println("nothing to backfill")
		return nil
	}

	var totalRepaired int
	for {
		batch := opts.batchSize
		if opts.limit > 0 && totalRepaired+batch > opts.limit {
			batch = opts.limit - totalRepaired
			if batch <= 0 {
				break
			}
		}
		repaired, err := backfillBatch(ctx, driver, opts.spaceID, batch)
		if err != nil {
			return fmt.Errorf("backfill batch: %w", err)
		}
		totalRepaired += repaired
		if opts.verbose || totalRepaired%2000 == 0 {
			fmt.Printf("  progress: %d/%d repaired\n", totalRepaired, candidateCount)
		}
		if repaired == 0 || (opts.limit > 0 && totalRepaired >= opts.limit) {
			break
		}
	}
	fmt.Printf("\nbackfill DONE: %d nodes repaired in space=%s\n", totalRepaired, opts.spaceID)
	fmt.Fprintf(os.Stderr, "[%s] backfill-node-content: %d repaired (space=%s)\n",
		time.Now().UTC().Format(time.RFC3339), totalRepaired, opts.spaceID)
	return nil
}

func backfillCountCandidates(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)
	out, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id:$spaceId})
WHERE (n.content IS NULL OR n.content = '')
  AND exists { MATCH (n)-[:HAS_OBSERVATION]->() }
RETURN count(n) AS c`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			c, _ := res.Record().Get("c")
			if cInt, ok := c.(int64); ok {
				return int(cInt), nil
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	c, _ := out.(int)
	return c, nil
}

func backfillBatch(ctx context.Context, driver neo4j.DriverWithContext, spaceID string, batchSize int) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// Deterministic Observation selection matches read-path fetchNodeContents.
	// Explicit CALL subquery pattern (not list comprehension) so ORDER BY works.
	cypher := `
MATCH (n:MemoryNode {space_id:$spaceId})
WHERE (n.content IS NULL OR n.content = '')
  AND exists { MATCH (n)-[:HAS_OBSERVATION]->() }
WITH n
LIMIT $batch
CALL {
  WITH n
  MATCH (n)-[:HAS_OBSERVATION]->(o)
  WHERE o.content IS NOT NULL AND o.content <> ''
  RETURN o.content AS obs_content
  ORDER BY o.created_at DESC
  LIMIT 1
}
SET n.content = obs_content
RETURN count(n) AS repaired`

	out, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, map[string]any{"spaceId": spaceID, "batch": batchSize})
		if err != nil {
			return 0, err
		}
		if res.Next(ctx) {
			r, _ := res.Record().Get("repaired")
			if rInt, ok := r.(int64); ok {
				return int(rInt), nil
			}
		}
		return 0, res.Err()
	})
	if err != nil {
		return 0, err
	}
	r, _ := out.(int)
	return r, nil
}

// neo4jDriverFromConfig — small helper mirroring the shape used across other
// CLI commands. Returns a connected driver or an error.
func neo4jDriverFromConfig(cfg config.Config) (neo4j.DriverWithContext, error) {
	if cfg.Neo4jURI == "" {
		return nil, errors.New("NEO4J_URI not configured")
	}
	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""),
	)
	if err != nil {
		return nil, err
	}
	if err := driver.VerifyConnectivity(context.Background()); err != nil {
		slog.Warn("backfill: neo4j connectivity check failed", "error", err)
		return nil, err
	}
	return driver, nil
}
