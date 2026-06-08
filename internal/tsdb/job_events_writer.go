// Sprint NOSILENT-001 — scheduled_job_events writer (V0024).
//
// One row per run of a scheduled/background core job (tsdb-backup, maintenance,
// export-auto). Synchronous single-row INSERT (mirrors V0021 model_install
// writer): jobs are infrequent, so no buffering. Fire-and-forget semantics —
// a write failure logs a warning and returns the error; it must never block or
// fail the job itself.
package tsdb

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nrednav/cuid2"
)

// jobEventPool is the minimal single-row INSERT contract (a *pgxpool.Pool
// satisfies it). Matches the modelInstallPool pattern.
type jobEventPool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// JobEventRow mirrors the V0024 scheduled_job_events columns.
type JobEventRow struct {
	JobName      string         // "tsdb-backup" | "maintenance" | "export-auto"
	SpaceID      string         // may be empty for space-agnostic jobs
	InstanceID   string         // hostname-derived instance id
	Success      bool           // outcome
	LatencyMS    int64          // wall-clock latency
	ErrorMessage string         // truncated to jobErrMessageMaxLen when success=false
	Metadata     map[string]any // job-specific detail → JSONB
}

// jobErrMessageMaxLen bounds error_message at write time (matches V0021).
const jobErrMessageMaxLen = 1024

// RecordJobEvent inserts one scheduled_job_events row synchronously. Pass a nil
// pool to no-op (TSDB disabled). Mirrors RecordModelInstallEvent.
func RecordJobEvent(ctx context.Context, pool jobEventPool, row JobEventRow) error {
	if pool == nil {
		return nil
	}
	if len(row.ErrorMessage) > jobErrMessageMaxLen {
		row.ErrorMessage = row.ErrorMessage[:jobErrMessageMaxLen]
	}

	// Optional fields → NULL when unset.
	var spaceID any
	if row.SpaceID != "" {
		spaceID = row.SpaceID
	}
	var instanceID any
	if row.InstanceID != "" {
		instanceID = row.InstanceID
	}
	var latency any
	if row.LatencyMS > 0 {
		latency = row.LatencyMS
	}
	var errMsg any
	if row.ErrorMessage != "" {
		errMsg = row.ErrorMessage
	}
	var metadata any
	if len(row.Metadata) > 0 {
		if b, err := json.Marshal(row.Metadata); err == nil {
			metadata = string(b)
		}
	}

	const insertSQL = `
		INSERT INTO scheduled_job_events (
			event_id, recorded_at, job_name, space_id, instance_id,
			success, latency_ms, error_message, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := pool.Exec(ctx, insertSQL,
		cuid2.Generate(),
		time.Now(),
		row.JobName,
		spaceID,
		instanceID,
		row.Success,
		latency,
		errMsg,
		metadata,
	)
	if err != nil {
		slog.Warn("scheduled_job_events: insert failed", "error", err, "job_name", row.JobName)
		return err
	}
	return nil
}
