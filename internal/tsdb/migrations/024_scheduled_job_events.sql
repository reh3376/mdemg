-- Migration 024: scheduled_job_events (Sprint NOSILENT-001 — fail-loud scheduled jobs)
-- Purpose: One row per run of a scheduled/background core job (tsdb-backup,
-- maintenance, export-auto). Records success/failure, latency, and a truncated
-- error so the server-native alert evaluator can detect BOTH "a job failed" and
-- "a job never ran" (staleness) — the gap that let the TSDB backup scheduler
-- fail every 24h run with only a buried slog.Warn.
--
-- Population: written synchronously after each job run via
-- internal/tsdb/job_events_writer.go::RecordJobEvent (single-row INSERT, mirrors
-- V0021 model_install_events — jobs are infrequent, not per-request). The server
-- backup scheduler writes via a result hook; the maintenance/export-auto CLI
-- commands write directly. internal/jobhealth.Report pairs the write with a
-- high-severity alert on failure.
--
-- Sprint: NOSILENT-001 (2026-06-08)
--
-- Rollback (manual — matches V0014..V0023 convention):
--   DROP TABLE IF EXISTS scheduled_job_events CASCADE;
--   UPDATE tsdb_schema_meta SET value = '23' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS scheduled_job_events (
    event_id        TEXT             NOT NULL,        -- CUIDv2 (MEMORY)
    recorded_at     TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    -- Job identity. One of: 'tsdb-backup', 'maintenance', 'export-auto'
    -- (extensible — new scheduled jobs reuse this table).
    job_name        TEXT             NOT NULL,
    -- Space the job ran for (may be empty for space-agnostic jobs like backup).
    space_id        TEXT,
    -- MDEMG instance identifier (hostname-derived) for multi-instance setups.
    instance_id     TEXT,
    -- Outcome.
    success         BOOLEAN          NOT NULL,
    -- Wall-clock latency in milliseconds.
    latency_ms      INTEGER,
    -- Truncated error message when success=false. Max 1 KB enforced by writer.
    error_message   TEXT,
    -- Job-specific structured detail (rows exported, orphans pruned, backup
    -- size, etc.). JSONB so per-job metadata needs no schema change.
    metadata        JSONB,

    PRIMARY KEY (recorded_at, event_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0017–V0023.
SELECT create_hypertable('scheduled_job_events', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes for the alert rules + operator investigation:
--   1. Per-job freshness ("latest success for job X") — staleness rule.
--   2. Recent failures across all jobs — failure rule + investigation.
--   3. Per-space drilldown.
CREATE INDEX IF NOT EXISTS idx_scheduled_job_name_time
    ON scheduled_job_events (job_name, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_scheduled_job_failed
    ON scheduled_job_events (recorded_at DESC)
    WHERE success = FALSE;
CREATE INDEX IF NOT EXISTS idx_scheduled_job_space_time
    ON scheduled_job_events (space_id, recorded_at DESC)
    WHERE space_id IS NOT NULL AND space_id <> '';

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_existing FROM scheduled_job_events;
    IF v_existing > 0 THEN
        RAISE NOTICE 'V0024 pre-migration: found % existing scheduled_job_events rows (preserved across re-apply)',
            v_existing;
    ELSE
        RAISE NOTICE 'V0024 pre-migration: empty table, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '24' WHERE key = 'schema_version';
