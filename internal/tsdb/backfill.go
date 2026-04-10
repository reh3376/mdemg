package tsdb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
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

// BackfillConstraintOutcomes reads GUIDANCE_OUTCOME edges from Neo4j and inserts
// any that don't yet exist in the constraint_outcomes TSDB table. This is a
// one-time catchup for data created before the TSDB writer was wired up.
// Idempotent: skips rows whose (constraint_id, guidance_id, time) already exist.
func BackfillConstraintOutcomes(ctx context.Context, pool *pgxpool.Pool, driver neo4j.DriverWithContext, spaceID string) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	cypher := `
	MATCH (c:MemoryNode {space_id: $spaceId})-[r:GUIDANCE_OUTCOME]-()
	WHERE r.outcome_type IS NOT NULL AND r.created_at IS NOT NULL
	RETURN c.node_id      AS constraint_id,
	       c.constraint_code AS constraint_code,
	       r.guidance_id  AS guidance_id,
	       r.session_id   AS session_id,
	       r.outcome_type AS outcome_type,
	       r.similarity   AS similarity,
	       r.guidance_type AS guidance_type,
	       r.created_at   AS created_at`

	result, err := sess.Run(ctx, cypher, map[string]any{"spaceId": spaceID})
	if err != nil {
		return fmt.Errorf("backfill constraint_outcomes: neo4j query: %w", err)
	}

	var rows [][]any
	for result.Next(ctx) {
		rec := result.Record()
		constraintID, _ := rec.Get("constraint_id")
		constraintCode, _ := rec.Get("constraint_code")
		guidanceID, _ := rec.Get("guidance_id")
		sessionID, _ := rec.Get("session_id")
		outcomeType, _ := rec.Get("outcome_type")
		similarity, _ := rec.Get("similarity")
		guidanceType, _ := rec.Get("guidance_type")
		createdAt, _ := rec.Get("created_at")

		var t time.Time
		switch v := createdAt.(type) {
		case string:
			parsed, parseErr := time.Parse(time.RFC3339, v)
			if parseErr != nil {
				continue
			}
			t = parsed
		case time.Time:
			t = v
		default:
			continue
		}

		var sim float64
		switch v := similarity.(type) {
		case float64:
			sim = v
		case int64:
			sim = float64(v)
		}

		rows = append(rows, []any{
			t,
			spaceID,
			str(constraintID),
			str(constraintCode),
			str(guidanceID),
			str(sessionID),
			str(outcomeType),
			sim,
			str(guidanceType),
			"", // instance_id not tracked in Neo4j edges
		})
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("backfill constraint_outcomes: neo4j iterate: %w", err)
	}

	if len(rows) == 0 {
		slog.Debug("backfill constraint_outcomes: no edges to backfill")
		return nil
	}

	// Check how many rows already exist to determine if backfill is needed.
	var existing int64
	err = pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM constraint_outcomes WHERE space_id = $1", spaceID,
	).Scan(&existing)
	if err != nil {
		return fmt.Errorf("backfill constraint_outcomes: count existing: %w", err)
	}
	if existing >= int64(len(rows)) {
		slog.Debug("backfill constraint_outcomes: already populated",
			"existing", existing, "neo4j_edges", len(rows))
		return nil
	}

	// Truncate and re-insert to avoid partial state from prior interrupted runs.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("backfill constraint_outcomes: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, "DELETE FROM constraint_outcomes WHERE space_id = $1", spaceID)
	if err != nil {
		return fmt.Errorf("backfill constraint_outcomes: delete existing: %w", err)
	}

	_, err = tx.CopyFrom(ctx,
		pgx.Identifier{"constraint_outcomes"},
		[]string{
			"time", "space_id", "constraint_id", "constraint_code",
			"guidance_id", "session_id", "outcome_type",
			"similarity", "guidance_type", "instance_id",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("backfill constraint_outcomes: copy: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("backfill constraint_outcomes: commit: %w", err)
	}

	slog.Info("tsdb: constraint_outcomes backfill complete",
		"space_id", spaceID, "rows_inserted", len(rows))
	return nil
}

// str safely converts an interface{} to string.
func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
