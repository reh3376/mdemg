-- Migration 030: contradicted_correction_drafts (Sprint JIMINY-CONTRADICTED-BRIDGE-001)
--
-- Bridges Jiminy's contradicted-outcome verdict into a reviewable draft that
-- the operator can approve via HITL-REVIEW-001. On approve, the draft's
-- Incorrect/Correct fields are handed to conversation.Service.Correct which
-- mints an L0 obs_type='correction' observation — JIMINY-CORRECTION-PRODUCER-001
-- (V0.11.2) then promotes that L0 obs to an L1 role_type='correction' node on
-- the next consolidation cycle.
--
-- Status transitions:
--   pending  -> approved   (Sink.Apply, verdict=approved; applied_at + applied_obs_id set)
--   pending  -> dismissed  (Sink.Apply, verdict=dismissed)
--   {approved,dismissed} -> pending  (Sink.Reverse — re-review invitation)
--
-- Reversal note: reversing an approve does NOT undo the L0 obs it created —
-- that must be tombstoned via mdemg concepts tombstone. Documented in
-- docs/features/hitl-review.md.
--
-- Writer: internal/tsdb/contradicted_drafts_writer.go — buffered + CopyFrom
-- (low volume ~3/mo baseline).
--
-- Time column is `time` (matches sibling tables + TSDB-CONSUME-001 SQL contract).
--
-- Rollback: see docs/development/jiminy-contradicted-bridge-001/sprint_plan_*.md
-- (§12 Rollback Procedures) — additive; DB-side reversal is manual by design.

CREATE TABLE IF NOT EXISTS contradicted_correction_drafts (
    time              TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    id                TEXT             NOT NULL,
    space_id          TEXT             NOT NULL,
    guidance_id       TEXT             NOT NULL,
    guidance_type     TEXT             NOT NULL DEFAULT '',
    source_node_id    TEXT             NOT NULL DEFAULT '',
    guidance_content  TEXT             NOT NULL DEFAULT '',
    action_summary    TEXT             NOT NULL DEFAULT '',
    similarity        DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    action_hash       TEXT             NOT NULL DEFAULT '',
    draft_incorrect   TEXT             NOT NULL DEFAULT '',
    draft_correct     TEXT             NOT NULL DEFAULT '',
    status            TEXT             NOT NULL DEFAULT 'pending',
    session_id        TEXT             NOT NULL DEFAULT '',
    applied_at        TIMESTAMPTZ,
    applied_obs_id    TEXT,
    instance_id       TEXT,

    PRIMARY KEY (time, id),
    CONSTRAINT contradicted_drafts_status_check CHECK (
        status IN ('pending', 'approved', 'dismissed')
    )
);

SELECT create_hypertable('contradicted_correction_drafts', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_contradicted_drafts_pending
    ON contradicted_correction_drafts (space_id, status, time DESC);
CREATE INDEX IF NOT EXISTS idx_contradicted_drafts_dedup
    ON contradicted_correction_drafts (guidance_id, action_hash, time DESC);
CREATE INDEX IF NOT EXISTS idx_contradicted_drafts_id
    ON contradicted_correction_drafts (id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'contradicted_correction_drafts' AND compression_enabled
    ) THEN
        ALTER TABLE contradicted_correction_drafts SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;
END $$;

SELECT add_compression_policy('contradicted_correction_drafts', INTERVAL '30 days', if_not_exists => true);
SELECT add_retention_policy('contradicted_correction_drafts',   INTERVAL '365 days', if_not_exists => true);

UPDATE tsdb_schema_meta SET value = '30' WHERE key = 'schema_version';
