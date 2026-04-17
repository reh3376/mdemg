package tsdb

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanConfig controls the TSDB error record cleanup operation.
type CleanConfig struct {
	SpaceID        string // required — scopes cleanup to a single space
	DryRun         bool   // if true, count and preview only — no deletes
	Limit          int    // max rows to delete per category (0 = unlimited)
	MinResponseLen int    // responses shorter than this are "silent failures" (default: 10)
}

// CleanPreview represents a single record that would be deleted.
type CleanPreview struct {
	Time         time.Time `json:"time"`
	TaskName     string    `json:"task_name"`
	ModelName    string    `json:"model_name"`
	ResponseLen  int       `json:"response_len"`
	Category     string    `json:"category"` // "error" or "silent_fail"
	ErrorSnippet string    `json:"error_snippet,omitempty"`
}

// CleanResult holds the outcome of a cleanup operation.
type CleanResult struct {
	DryRun             bool             `json:"dry_run"`
	TotalBefore        int64            `json:"total_before"`
	ErrorsFound        int64            `json:"errors_found"`
	SilentFailsFound   int64            `json:"silent_fails_found"`
	ErrorsDeleted      int64            `json:"errors_deleted"`
	SilentFailsDeleted int64            `json:"silent_fails_deleted"`
	TotalAfter         int64            `json:"total_after"`
	PerTaskCounts      map[string]int64 `json:"per_task_counts"`
	Preview            []CleanPreview   `json:"preview,omitempty"`
}

// execer is the interface shared by pgxpool.Pool and pgx.Tx for executing SQL.
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// RunClean removes error records and silent failures from llm_interactions.
// In dry-run mode it counts and previews without deleting.
// Matching criteria align with neural/training/quality_filter.py gates:
//   - Gate 2: error IS NOT NULL AND error != '' (error_present)
//   - Gate 1: response IS NULL OR length(response) < MinResponseLen (empty_response)
func RunClean(ctx context.Context, pool *pgxpool.Pool, cfg CleanConfig) (*CleanResult, error) {
	if cfg.SpaceID == "" {
		return nil, fmt.Errorf("cleaner: space_id is required")
	}
	if cfg.MinResponseLen <= 0 {
		cfg.MinResponseLen = 10
	}

	result := &CleanResult{
		DryRun:        cfg.DryRun,
		PerTaskCounts: make(map[string]int64),
	}

	// Count totals before cleanup.
	err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE error IS NOT NULL AND error != ''),
			COUNT(*) FILTER (WHERE (error IS NULL OR error = '') AND (response IS NULL OR length(response) < $2))
		FROM llm_interactions
		WHERE space_id = $1`,
		cfg.SpaceID, cfg.MinResponseLen,
	).Scan(&result.TotalBefore, &result.ErrorsFound, &result.SilentFailsFound)
	if err != nil {
		return nil, fmt.Errorf("cleaner: count before: %w", err)
	}

	// Preview: show up to 5 records from each category.
	result.Preview, err = previewRecords(ctx, pool, cfg)
	if err != nil {
		return nil, fmt.Errorf("cleaner: preview: %w", err)
	}

	if cfg.DryRun {
		result.TotalAfter = result.TotalBefore
		if err := fillPerTaskCounts(ctx, pool, cfg.SpaceID, cfg.MinResponseLen, result); err != nil {
			return nil, err
		}
		return result, nil
	}

	// Live mode: transactional delete.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("cleaner: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	result.ErrorsDeleted, err = deleteCategory(ctx, tx, cfg.SpaceID, cfg.Limit,
		`error IS NOT NULL AND error != ''`)
	if err != nil {
		return nil, fmt.Errorf("cleaner: delete errors: %w", err)
	}

	result.SilentFailsDeleted, err = deleteCategory(ctx, tx, cfg.SpaceID, cfg.Limit,
		fmt.Sprintf(`(error IS NULL OR error = '') AND (response IS NULL OR length(response) < %d)`, cfg.MinResponseLen))
	if err != nil {
		return nil, fmt.Errorf("cleaner: delete silent fails: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("cleaner: commit: %w", err)
	}

	// Count after cleanup.
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM llm_interactions WHERE space_id = $1`, cfg.SpaceID).Scan(&result.TotalAfter)
	if err != nil {
		return nil, fmt.Errorf("cleaner: count after: %w", err)
	}

	if err := fillPerTaskCounts(ctx, pool, cfg.SpaceID, cfg.MinResponseLen, result); err != nil {
		return nil, err
	}

	return result, nil
}

func previewRecords(ctx context.Context, pool *pgxpool.Pool, cfg CleanConfig) ([]CleanPreview, error) {
	var previews []CleanPreview

	// Error records.
	rows, err := pool.Query(ctx, `
		SELECT time, task_name, COALESCE(model_name, ''), length(COALESCE(response, '')), LEFT(error, 80)
		FROM llm_interactions
		WHERE space_id = $1 AND error IS NOT NULL AND error != ''
		ORDER BY time DESC LIMIT 5`,
		cfg.SpaceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p CleanPreview
		if err := rows.Scan(&p.Time, &p.TaskName, &p.ModelName, &p.ResponseLen, &p.ErrorSnippet); err != nil {
			return nil, err
		}
		p.Category = "error"
		previews = append(previews, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Silent failures.
	rows2, err := pool.Query(ctx, `
		SELECT time, task_name, COALESCE(model_name, ''), length(COALESCE(response, ''))
		FROM llm_interactions
		WHERE space_id = $1 AND (error IS NULL OR error = '') AND (response IS NULL OR length(response) < $2)
		ORDER BY time DESC LIMIT 5`,
		cfg.SpaceID, cfg.MinResponseLen,
	)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var p CleanPreview
		if err := rows2.Scan(&p.Time, &p.TaskName, &p.ModelName, &p.ResponseLen); err != nil {
			return nil, err
		}
		p.Category = "silent_fail"
		previews = append(previews, p)
	}
	return previews, rows2.Err()
}

// deleteCategory deletes rows matching condition within a transaction.
// Uses trace_id subquery for LIMIT support (ctid not supported on compressed TimescaleDB chunks).
func deleteCategory(ctx context.Context, ex execer, spaceID string, limit int, condition string) (int64, error) {
	if limit > 0 {
		sql := fmt.Sprintf(`
			DELETE FROM llm_interactions
			WHERE trace_id = ANY(
				ARRAY(SELECT trace_id FROM llm_interactions
				      WHERE space_id = $1 AND %s
				      ORDER BY time DESC LIMIT $2)
			)`, condition)
		tag, err := ex.Exec(ctx, sql, spaceID, limit)
		if err != nil {
			return 0, err
		}
		return tag.RowsAffected(), nil
	}

	sql := fmt.Sprintf(`DELETE FROM llm_interactions WHERE space_id = $1 AND %s`, condition)
	tag, err := ex.Exec(ctx, sql, spaceID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func fillPerTaskCounts(ctx context.Context, pool *pgxpool.Pool, spaceID string, minResponseLen int, result *CleanResult) error {
	rows, err := pool.Query(ctx, `
		SELECT task_name, COUNT(*)
		FROM llm_interactions
		WHERE space_id = $1 AND (error IS NULL OR error = '') AND length(COALESCE(response, '')) >= $2
		GROUP BY task_name ORDER BY COUNT(*) DESC`,
		spaceID, minResponseLen,
	)
	if err != nil {
		return fmt.Errorf("cleaner: per-task counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return fmt.Errorf("cleaner: per-task scan: %w", err)
		}
		result.PerTaskCounts[name] = count
	}
	return rows.Err()
}
