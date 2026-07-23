// FT-RECURSIVE-003 E2: first writer for the ft_model_versions table (V0002
// DDL, zero-writer since inception). One row per serving swap — the
// promotion audit trail and the rollback target source.
package tsdb

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// ModelVersionRow mirrors ft_model_versions (002_ft_schema.sql).
type ModelVersionRow struct {
	DeployedAt    time.Time
	Version       string // unique — e.g. "mdemg-llm-v1" or "mdemg-llm-v1-c<cycle>"
	ModelPath     string // the GGUF the serving symlink points at
	AdapterPath   string
	BaseModel     string
	TrainingCycle string
	OverallScore  float64
	Status        string // active | superseded | rolled_back
	Notes         string
}

// Version status values.
const (
	ModelVersionActive     = "active"
	ModelVersionSuperseded = "superseded"
	ModelVersionRolledBack = "rolled_back"
)

// RecordModelVersion inserts one row synchronously (promotions are rare —
// the V0021 one-shot pattern, not a buffered writer). Nil-safe on pool.
func RecordModelVersion(ctx context.Context, pool jobEventPool, row ModelVersionRow) error {
	if pool == nil {
		slog.Debug("ft_model_versions: no pool, skipping record")
		return nil
	}
	if row.Version == "" || row.ModelPath == "" || row.BaseModel == "" {
		return errors.New("ft_model_versions: version, model_path and base_model are required")
	}
	if row.Status == "" {
		row.Status = ModelVersionActive
	}
	if row.DeployedAt.IsZero() {
		row.DeployedAt = time.Now().UTC()
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO ft_model_versions
			(deployed_at, version, model_path, adapter_path, base_model,
			 training_cycle, overall_score, status, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (version) DO UPDATE SET
			deployed_at = EXCLUDED.deployed_at,
			model_path  = EXCLUDED.model_path,
			status      = EXCLUDED.status,
			notes       = EXCLUDED.notes`,
		row.DeployedAt, row.Version, row.ModelPath, nullIfEmpty(row.AdapterPath),
		row.BaseModel, nullIfEmpty(row.TrainingCycle), row.OverallScore,
		row.Status, nullIfEmpty(row.Notes))
	if err != nil {
		slog.Warn("ft_model_versions: insert failed", "error", err, "version", row.Version)
	}
	return err
}

// MarkModelVersionStatus transitions a version's status (active→superseded on
// promote; active→rolled_back on rollback).
func MarkModelVersionStatus(ctx context.Context, pool jobEventPool, version, status string) error {
	if pool == nil {
		return nil
	}
	_, err := pool.Exec(ctx,
		`UPDATE ft_model_versions SET status = $2 WHERE version = $1`, version, status)
	return err
}

// ActiveModelVersion returns the current 'active' row (latest deployed_at),
// or nil when none exists.
func ActiveModelVersion(ctx context.Context, pool ftCycleQuerier) (*ModelVersionRow, error) {
	return latestVersionByStatus(ctx, pool, ModelVersionActive)
}

// PreviousModelVersion returns the most recently superseded row — the
// rollback target.
func PreviousModelVersion(ctx context.Context, pool ftCycleQuerier) (*ModelVersionRow, error) {
	return latestVersionByStatus(ctx, pool, ModelVersionSuperseded)
}

func latestVersionByStatus(ctx context.Context, pool ftCycleQuerier, status string) (*ModelVersionRow, error) {
	row := pool.QueryRow(ctx, `
		SELECT deployed_at, version, model_path, COALESCE(adapter_path,''),
		       base_model, COALESCE(training_cycle,''), COALESCE(overall_score,0),
		       status, COALESCE(notes,'')
		FROM ft_model_versions WHERE status = $1
		ORDER BY deployed_at DESC LIMIT 1`, status)
	var v ModelVersionRow
	if err := row.Scan(&v.DeployedAt, &v.Version, &v.ModelPath, &v.AdapterPath,
		&v.BaseModel, &v.TrainingCycle, &v.OverallScore, &v.Status, &v.Notes); err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// LLMCallStatsSince returns total LLM calls and REAL errors (caller-
// cancellation-filtered per the LLM-HEALTH-INVESTIGATION-001 contract)
// recorded since t — the post-swap tripwire's signal (FT-RECURSIVE-003 E5).
func LLMCallStatsSince(ctx context.Context, pool ftCycleQuerier, since time.Time) (total, realErrors int64, err error) {
	row := pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE error <> '' AND error NOT LIKE 'caller_canceled:%')
		FROM llm_interactions WHERE time >= $1`, since)
	if err := row.Scan(&total, &realErrors); err != nil {
		return 0, 0, err
	}
	return total, realErrors, nil
}

// OperatorConfirmedPromotions counts DISTINCT cycles promoted by an explicit
// operator decision — the FT_LOOP_AUTO_PROMOTE_AFTER policy input
// (FT-RECURSIVE-003 E6; spec §5 fork 3: auto only after N human confirms).
func OperatorConfirmedPromotions(ctx context.Context, pool ftCycleQuerier) (int, error) {
	row := pool.QueryRow(ctx, `
		SELECT count(DISTINCT cycle_id) FROM ft_training_cycles
		WHERE status = 'promoted'
		  AND eval_results->>'operator_decision' IN ('operator', 'confirm')`) // 'confirm' = the pre-E6 CLI event form
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
