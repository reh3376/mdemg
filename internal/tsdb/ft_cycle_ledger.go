// Sprint FT-RECURSIVE-002 (Phase 6b) — ft_training_cycles ledger (V0002 table,
// previously zero writers; this is its first writer).
//
// The recursive-retrain loop's single source of truth for "is a cycle running"
// and "when did we last train". Modeled EVENT-SOURCED: one appended row per
// state transition (the natural shape for a TimescaleDB hypertable + a forensic
// trail). The current state of a cycle is the latest row for its cycle_id; an
// "open" cycle is one whose latest status is non-terminal. Single-flight and the
// retrain-interval gate are both DB queries over this table, so they survive a
// restart (the RSIC 300 s in-memory cooldown is the wrong tool at the ~9 h
// retrain scale — SPEC §3).
//
// Synchronous single-row INSERT (mirrors RecordJobEvent / the V0021 model
// writer): cycle transitions are rare (a handful per retrain, ~weekly), so no
// buffering.
package tsdb

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// FtCycleStatus is the recursive-retrain cycle state-machine (SPEC §3).
type FtCycleStatus string

const (
	FtCycleTriggered      FtCycleStatus = "triggered"
	FtCycleCurating       FtCycleStatus = "curating"
	FtCycleTraining       FtCycleStatus = "training"
	FtCycleGating         FtCycleStatus = "gating"
	FtCyclePromotePending FtCycleStatus = "promote_pending"
	FtCyclePromoted       FtCycleStatus = "promoted"
	FtCycleFailed         FtCycleStatus = "failed"
	FtCycleRolledBack     FtCycleStatus = "rolled_back"
)

// IsTerminal reports whether a cycle in this status is finished — a terminal
// cycle no longer bars a new trigger.
func (s FtCycleStatus) IsTerminal() bool {
	switch s {
	case FtCyclePromoted, FtCycleFailed, FtCycleRolledBack:
		return true
	default:
		return false
	}
}

// IsValidCycleStatus guards against typo'd transitions reaching the ledger.
func IsValidCycleStatus(s FtCycleStatus) bool {
	switch s {
	case FtCycleTriggered, FtCycleCurating, FtCycleTraining, FtCycleGating,
		FtCyclePromotePending, FtCyclePromoted, FtCycleFailed, FtCycleRolledBack:
		return true
	default:
		return false
	}
}

// FtCycleEvent maps the ft_training_cycles columns for one state transition.
type FtCycleEvent struct {
	CycleID        string
	ModelVersion   string // the model being produced/replaced (NOT NULL column)
	Status         FtCycleStatus
	Stage          string         // free-form stage label (curate/train/benchmark/gate/promote)
	DatasetVersion string         // curated dataset version (set from CURATING on)
	ExogenousRatio float64        // dataset exogenous-ratio (optional)
	TrainingConfig map[string]any // epoch/rank/etc. → JSONB
	EvalResults    map[string]any // gate scores → JSONB
	DurationSecs   float64        // stage/cycle wall-clock (optional)
	Error          string         // cause when Status=failed
}

const ftCycleErrMaxLen = 2048

// RecordCycleEvent appends one state-transition row synchronously. nil pool →
// no-op (TSDB disabled). Rejects an invalid status (a typo must not silently
// poison the single-flight predicate). model_version is required by the schema;
// it defaults to "pending" if unset so an early TRIGGERED row is always writable.
func RecordCycleEvent(ctx context.Context, pool jobEventPool, ev FtCycleEvent) error {
	if pool == nil {
		return nil
	}
	if !IsValidCycleStatus(ev.Status) {
		return errors.New("ft_training_cycles: invalid status " + string(ev.Status))
	}
	if ev.CycleID == "" {
		return errors.New("ft_training_cycles: cycle_id required")
	}
	modelVersion := ev.ModelVersion
	if modelVersion == "" {
		modelVersion = "pending"
	}
	if len(ev.Error) > ftCycleErrMaxLen {
		ev.Error = ev.Error[:ftCycleErrMaxLen]
	}

	opt := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	var exo any
	if ev.ExogenousRatio > 0 {
		exo = ev.ExogenousRatio
	}
	var dur any
	if ev.DurationSecs > 0 {
		dur = ev.DurationSecs
	}
	jsonOrNil := func(m map[string]any) any {
		if len(m) == 0 {
			return nil
		}
		if b, err := json.Marshal(m); err == nil {
			return string(b)
		}
		return nil
	}

	const insertSQL = `
		INSERT INTO ft_training_cycles (
			time, cycle_id, model_version, status, stage, dataset_version,
			exogenous_ratio, training_config, eval_results, duration_secs, error
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := pool.Exec(ctx, insertSQL,
		time.Now(),
		ev.CycleID,
		modelVersion,
		string(ev.Status),
		opt(ev.Stage),
		opt(ev.DatasetVersion),
		exo,
		jsonOrNil(ev.TrainingConfig),
		jsonOrNil(ev.EvalResults),
		dur,
		opt(ev.Error),
	)
	if err != nil {
		slog.Warn("ft_training_cycles: insert failed", "error", err, "cycle_id", ev.CycleID, "status", ev.Status)
		return err
	}
	return nil
}

// FtCycleState is the current (latest) state of a cycle.
type FtCycleState struct {
	CycleID      string
	ModelVersion string
	Status       FtCycleStatus
	Stage        string
	UpdatedAt    time.Time
}

// ftCycleQuerier is the read contract (a *pgxpool.Pool satisfies it).
type ftCycleQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// OpenCycle returns the single currently-open (non-terminal latest-status)
// cycle, or nil if none — the DB-backed single-flight check. A well-behaved
// loop has at most one open cycle; if more exist (shouldn't), the most recently
// updated is returned (and a new trigger is still barred).
func OpenCycle(ctx context.Context, pool ftCycleQuerier) (*FtCycleState, error) {
	if pool == nil {
		return nil, nil
	}
	// latest row per cycle_id, keep only non-terminal, newest first.
	const q = `
		WITH latest AS (
			SELECT DISTINCT ON (cycle_id)
				cycle_id, model_version, status, stage, time
			FROM ft_training_cycles
			ORDER BY cycle_id, time DESC
		)
		SELECT cycle_id, model_version, status, COALESCE(stage,''), time
		FROM latest
		WHERE status NOT IN ('promoted','failed','rolled_back')
		ORDER BY time DESC
		LIMIT 1`
	var st FtCycleState
	var status string
	err := pool.QueryRow(ctx, q).Scan(&st.CycleID, &st.ModelVersion, &status, &st.Stage, &st.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	st.Status = FtCycleStatus(status)
	return &st, nil
}

// CycleStatus returns the current (latest) status of a specific cycle, or
// ("", false) if the cycle id is unknown.
func CycleStatus(ctx context.Context, pool ftCycleQuerier, cycleID string) (FtCycleStatus, bool, error) {
	if pool == nil {
		return "", false, nil
	}
	const q = `SELECT status FROM ft_training_cycles WHERE cycle_id = $1 ORDER BY time DESC LIMIT 1`
	var status string
	if err := pool.QueryRow(ctx, q, cycleID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return FtCycleStatus(status), true, nil
}

// FreshInteractionFraction returns the fraction of llm_interactions newer than
// `since` over the total — the "is there new signal since we last trained"
// trigger gate (SPEC §3). Returns 1.0 when there are no interactions (a fresh
// corpus is trivially "all new"). The fraction is a coarse all-task proxy; the
// controller refines per-task at curation time.
func FreshInteractionFraction(ctx context.Context, pool ftCycleQuerier, since time.Time) (float64, error) {
	if pool == nil {
		return 1.0, nil
	}
	const q = `
		SELECT
			COUNT(*) FILTER (WHERE time > $1)::float8 AS fresh,
			COUNT(*)::float8 AS total
		FROM llm_interactions`
	var fresh, total float64
	if err := pool.QueryRow(ctx, q, since).Scan(&fresh, &total); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 1.0, nil
		}
		return 0, err
	}
	if total <= 0 {
		return 1.0, nil
	}
	return fresh / total, nil
}

// LastCycleStart returns the most recent cycle's first (triggered) timestamp,
// for the retrain-interval gate. ok=false when no cycle has ever run.
func LastCycleStart(ctx context.Context, pool ftCycleQuerier) (time.Time, bool, error) {
	if pool == nil {
		return time.Time{}, false, nil
	}
	const q = `SELECT MAX(time) FROM ft_training_cycles WHERE status = 'triggered'`
	var t *time.Time
	if err := pool.QueryRow(ctx, q).Scan(&t); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	if t == nil {
		return time.Time{}, false, nil
	}
	return *t, true, nil
}

// CycleCandidatePath returns the candidate GGUF path recorded on the cycle's
// promote_pending event (FT-RECURSIVE-003 E3 — the promotion executor's
// swap target), plus the gate score when present.
func CycleCandidatePath(ctx context.Context, pool ftCycleQuerier, cycleID string) (path string, gateScore float64, err error) {
	if pool == nil {
		return "", 0, errors.New("ft_training_cycles: no pool")
	}
	row := pool.QueryRow(ctx, `
		SELECT COALESCE(eval_results->>'candidate_gguf', ''),
		       COALESCE((eval_results->>'gate_score')::float8, 0)
		FROM ft_training_cycles
		WHERE cycle_id = $1 AND status = 'promote_pending'
		ORDER BY time DESC LIMIT 1`, cycleID)
	if scanErr := row.Scan(&path, &gateScore); scanErr != nil {
		if scanErr.Error() == "no rows in result set" {
			return "", 0, nil
		}
		return "", 0, scanErr
	}
	return path, gateScore, nil
}
