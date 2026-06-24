-- Migration 027: guidance_training_rows (Sprint JIMINY-RELEVANCE-001 — Epic 1)
-- Purpose: persist the TRAINING EVIDENCE for every guidance-feedback interaction —
-- the surfaced guidance text, its source-node role_type/layer, the agent-action
-- text, and the audited verdict + classifier source. The Step-1 diagnostic
-- (docs/development/jiminy-relevance-001/diagnostic_ignored_population.md, Finding 1)
-- established that MDEMG persists VERDICTS everywhere (constraint_outcomes,
-- Neo4j GUIDANCE_OUTCOME edges) and EVIDENCE nowhere: the action_summary POSTed
-- to /v1/jiminy/feedback was classified then discarded, so every
-- (context -> surfaced-guidance -> did-the-agent-follow) training triple was
-- unrecoverable. This table starts the collection clock — forward-only, no
-- backfill (the historical evidence does not exist to recover).
--
-- This is the EVIDENCE source; it carries NO gold-grading columns. Human/auto
-- GOLD grades live in HITL-REVIEW-001's `review_grades` hypertable keyed by
-- (dataset_id, item_id), where item_id joins back to this table's row_id. Keep
-- the two separate: this table is append-heavy production evidence; review_grades
-- is the human-certification overlay.
--
-- Writer: internal/tsdb/guidance_training_rows_writer.go — buffered + CopyFrom
-- (V0022 reinforcement_events pattern: bounded FIFO buffer, drop counter,
-- registerWriterStats). Per-prompt-feedback volume, so buffered not sync-INSERT.
--
-- Time column is `time` (NOT recorded_at) — matches the sibling
-- constraint_outcomes table and the TSDB-CONSUME-001 alert-rule SQL contract
-- (the should-follow rule in Epic 4 reads `time`).
--
-- Sprint: JIMINY-RELEVANCE-001 (2026-06-23)
--
-- ⚠️ Migration-number coordination with HITL-REVIEW-001: that sprint also
-- proposes `027` (027_review_grades.sql). Whichever merges first takes 027; the
-- second renumbers to 028 and bumps TSDBRequiredSchemaVersion to 28 in the same
-- PR (a mechanical rebase).
--
-- Rollback (manual — matches V0014..V0026 convention):
--   DROP TABLE IF EXISTS guidance_training_rows CASCADE;
--   UPDATE tsdb_schema_meta SET value = '26' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS guidance_training_rows (
    row_id            TEXT             NOT NULL,          -- CUIDv2; HITL review_grades.item_id joins here
    time              TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    space_id          TEXT             NOT NULL,
    session_id        TEXT,
    instance_id       TEXT,
    guidance_id       TEXT             NOT NULL,
    -- The actionability class (constraint|correction|pattern|learning|concept|…).
    -- Finding 2: pattern/learning/concept (abstractions) are ignored 53–65%;
    -- constraint/correction (actionable) ~2× better. This column is the primary
    -- actionability signal even before the source-node layer below.
    guidance_type     TEXT,
    -- Snapshot of the surfaced guidance item content (truncated to
    -- GUIDANCE_CORPUS_MAX_CONTENT_BYTES with a '…[truncated]' marker). The
    -- binding evidence that was previously discarded.
    guidance_content  TEXT,
    -- First source node id (item.SourceNodes[0]); lets source role/layer be
    -- re-resolved offline if the inline lookup below was disabled/missed.
    source_node_id    TEXT,
    -- Best-effort bounded Neo4j resolution of the source node's role_type and
    -- layer (Finding 5: 172:1 abstraction:constraint node ratio). "" / NULL when
    -- the lookup is disabled, times out, or misses — never blocks the hot path.
    source_role_type  TEXT,
    source_layer      INTEGER,
    -- The agent-action text (req.ActionSummary), truncated. The other half of
    -- the training triple — previously classified then thrown away.
    action_summary    TEXT,
    -- Audited verdict + how it was produced.
    outcome_type      TEXT             NOT NULL,
    similarity        DOUBLE PRECISION,
    -- tier1|llm|heuristic|explicit|'' — see V0026. Finding 4: ~51% of labels are
    -- heuristic/blank noise; Epic 2's auto-relabel upgrades these in place.
    classifier_source TEXT             NOT NULL DEFAULT '',
    constraint_code   TEXT,

    PRIMARY KEY (time, row_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0017–V0022.
SELECT create_hypertable('guidance_training_rows', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes: per-space time series (curation + the should-follow panel), the
-- FetchCandidates scan by classifier_source (auto-relabel + review sampling),
-- and guidance_id correlation.
CREATE INDEX IF NOT EXISTS idx_guidance_training_space_time
    ON guidance_training_rows (space_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_guidance_training_classifier
    ON guidance_training_rows (space_id, classifier_source, time DESC);
CREATE INDEX IF NOT EXISTS idx_guidance_training_guidance
    ON guidance_training_rows (space_id, guidance_id, time DESC);

-- Data-management policies (TSDB-CONSUME-001 / V0025 contract — no unbounded
-- hypertable). The corpus is the rolling source; durable training snapshots are
-- exported by `mdemg data curate` (Epic 5). 365-day retention covers the 3–6
-- month collection window with margin; compress the bulky text after 30 days.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'guidance_training_rows' AND compression_enabled
    ) THEN
        ALTER TABLE guidance_training_rows SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;
END $$;

SELECT add_compression_policy('guidance_training_rows', INTERVAL '30 days', if_not_exists => true);
SELECT add_retention_policy('guidance_training_rows',   INTERVAL '365 days', if_not_exists => true);

UPDATE tsdb_schema_meta SET value = '27' WHERE key = 'schema_version';
