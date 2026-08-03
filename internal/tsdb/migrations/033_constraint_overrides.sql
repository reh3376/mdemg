-- Migration 033: constraint_overrides (Sprint ENFORCE-OVERRIDES-TSDB, 2026-08-03)
--
-- Persists operator-installed constraint overrides to TSDB so RSIC + UI can
-- query the history alongside outcomes. JSONL audit at
-- ~/.mdemg/jiminy-overrides.jsonl is retained for forensic/portability, but
-- the TSDB copy is the queryable source-of-truth.
--
-- Op enum:
--   apply   → operator installed an override via CLI / UI / POST /v1/jiminy/override
--   revoke  → operator revoked before scheduled expiry via CLI / UI / DELETE
--   expire  → time-based auto-expiry (lazy-purged on next Get()/List())
--
-- Writer: internal/jiminy/override.go — writes synchronously on each
-- apply/revoke/expire (low volume; typical: <10/day per session). If TSDB
-- write fails, WARN log + JSONL still succeeds — audit trail is best-effort.
--
-- Reader: internal/tsdb/dataset_builder.go::OverrideHistory — queried by
-- RSIC (action-execution consumes to gate deprecate/reword decisions) + UI
-- (Jiminy tab timeline).

CREATE TABLE IF NOT EXISTS constraint_overrides (
    time             TIMESTAMPTZ NOT NULL,       -- op timestamp (audit key)
    space_id         TEXT NOT NULL,
    session_id       TEXT NOT NULL,
    constraint_code  TEXT NOT NULL,
    reason           TEXT NOT NULL,              -- required at apply; propagated to revoke/expire rows
    op               TEXT NOT NULL,              -- apply | revoke | expire
    applied_at       TIMESTAMPTZ NOT NULL,       -- original apply time (survives across op rows)
    expires_at       TIMESTAMPTZ NOT NULL,       -- scheduled expiry
    CONSTRAINT constraint_overrides_op_check CHECK (op IN ('apply', 'revoke', 'expire'))
);

-- Hypertable partitioned on time (7-day chunks — matches constraint_outcomes cadence).
SELECT create_hypertable('constraint_overrides', 'time',
    chunk_time_interval => interval '7 days',
    if_not_exists => TRUE);

-- Query-shape indices: RSIC and UI both filter on (space_id, constraint_code)
-- over a time window. Index avoids full-scan when the substrate accumulates
-- volume.
CREATE INDEX IF NOT EXISTS idx_constraint_overrides_space_code
    ON constraint_overrides (space_id, constraint_code, time DESC);

-- Session-scoped queries (e.g. "show me all overrides for the current session"
-- in the UI Jiminy tab).
CREATE INDEX IF NOT EXISTS idx_constraint_overrides_session
    ON constraint_overrides (session_id, time DESC);

-- Retention: 180d (parity with 90d telemetry / 365d idea-09 mid-band; overrides
-- are ~5-30 events per operator-session, so a decade-long retention is safe
-- and useful for retrospective analysis of enforcement calibration).
SELECT add_retention_policy('constraint_overrides', interval '180 days',
    if_not_exists => TRUE);

-- Bump schema version marker so RSIC's schema_drift_detected pattern stays
-- silent (see internal/ape/self_reflect.go — tsdbVer < TSDBRequiredSchemaVersion
-- fires alert_schema_drift). Every migration bumps by 1; TSDB_REQUIRED_SCHEMA_
-- VERSION default in internal/config/config.go must land at the same value.
UPDATE tsdb_schema_meta SET value = '33' WHERE key = 'schema_version';
