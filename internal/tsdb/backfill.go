package tsdb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BackfillInstanceID sets instance_id on all records that have an empty value.
// This is a one-time operation for pre-migration-008 data. It runs in a single
// transaction and is idempotent — subsequent calls are no-ops when no empty
// instance_id rows remain.
func BackfillInstanceID(ctx context.Context, pool *pgxpool.Pool, instanceID string) error {
	if instanceID == "" {
		return fmt.Errorf("backfill: instance_id is empty, skipping")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("backfill: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tables := []string{"llm_interactions", "embedding_events", "retrieval_events"}
	totalAffected := int64(0)

	for _, table := range tables {
		tag, err := tx.Exec(ctx,
			fmt.Sprintf("UPDATE %s SET instance_id = $1 WHERE instance_id = ''", table),
			instanceID,
		)
		if err != nil {
			return fmt.Errorf("backfill %s: %w", table, err)
		}
		if tag.RowsAffected() > 0 {
			slog.Info("tsdb: instance_id backfill",
				"table", table,
				"rows_updated", tag.RowsAffected(),
				"instance_id", instanceID,
			)
			totalAffected += tag.RowsAffected()
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("backfill: commit: %w", err)
	}

	if totalAffected > 0 {
		slog.Info("tsdb: instance_id backfill complete", "total_rows", totalAffected)
	}

	return nil
}
