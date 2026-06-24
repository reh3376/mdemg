-- Migration 028: review_grades (Sprint HITL-REVIEW-001 — Epic 1)
-- Purpose: the gold-grade + reversal-audit record for the Human-in-the-Loop
-- review platform. One row per certified human grade of a reviewable dataset
-- item (guidance, sft, dpo, …), keyed conceptually by (dataset_id, item_id).
-- This is the human-certification overlay JIMINY-RELEVANCE-001's
-- guidance_training_rows points to via item_id == row_id.
--
-- The headline capability is that a grade can REINFORCE the live cognitive
-- substrate (trust + GUIDANCE_OUTCOME edge + node confidence). When it does,
-- reinforcement_detail captures EXACTLY what was applied (prior trust/confidence,
-- created edge id, sink verb) — the reversal payload — so the apply is fully
-- undoable via POST /v1/review/reverse (which writes a NEW row referencing the
-- original and sets reversed=true on it).
--
-- Writer: internal/tsdb/review_grades_writer.go — buffered + CopyFrom (human-
-- paced volume) + a synchronous LatestGradeForItem read for idempotency/reversal.
--
-- Time column is `time` (NOT recorded_at) — matches the sibling tables + the
-- TSDB-CONSUME-001 alert-rule SQL contract.
--
-- Sprint: HITL-REVIEW-001 (2026-06-24). Migration 028 (027 was taken by
-- JIMINY-RELEVANCE-001's guidance_training_rows — the coordinated-pair rebase).
--
-- Rollback (manual — matches V0014..V0027 convention):
--   DROP TABLE IF EXISTS review_grades CASCADE;
--   UPDATE tsdb_schema_meta SET value = '27' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS review_grades (
    time                  TIMESTAMPTZ      NOT NULL DEFAULT NOW(),  -- hypertable dim (= graded_at)
    grade_id              TEXT             NOT NULL,                -- CUIDv2
    dataset_id            TEXT             NOT NULL,                -- e.g. 'guidance'
    item_id               TEXT             NOT NULL,                -- stable item key; joins guidance_training_rows.row_id
    space_id              TEXT             NOT NULL,
    gold_score            DOUBLE PRECISION NOT NULL,                -- normalized 0–1
    -- Per-dimension 0–4 scores + anchors hit (Rated); for Ranked (DPO) the
    -- {chosen, rejected, confidence} object.
    gold_dimensions       JSONB            NOT NULL,
    grader_id             TEXT             NOT NULL,                -- human handle or 'auto:<model>'
    rubric_version        TEXT             NOT NULL,                -- pinned per grade (e.g. 'gr-v1')
    graded_at             TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    -- Did this grade write to the live system?
    reinforcement_applied BOOLEAN          NOT NULL DEFAULT FALSE,
    -- EXACTLY what was applied — the reversal payload (prior trust, prior
    -- confidence, created edge id, sink id, applied verb). NULL when gold-only.
    reinforcement_detail  JSONB,
    -- A reversal writes a NEW row (reverses_grade_id set) AND sets reversed=true
    -- on the original.
    reversed              BOOLEAN          NOT NULL DEFAULT FALSE,
    reverses_grade_id     TEXT,                                     -- non-null on a reversal row
    instance_id           TEXT,

    PRIMARY KEY (time, grade_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0017–V0027.
SELECT create_hypertable('review_grades', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes: idempotency/reversal lookup by (dataset_id,item_id), grade_id lookup,
-- and reversal-row lookup.
CREATE INDEX IF NOT EXISTS idx_review_grades_item
    ON review_grades (dataset_id, item_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_review_grades_grade_id
    ON review_grades (grade_id);
CREATE INDEX IF NOT EXISTS idx_review_grades_reverses
    ON review_grades (reverses_grade_id)
    WHERE reverses_grade_id IS NOT NULL;

-- Data-management policies (TSDB-CONSUME-001 / V0025 — no unbounded hypertable).
-- Audit/forensic class: 180-day retention, compress after 14 days.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'review_grades' AND compression_enabled
    ) THEN
        ALTER TABLE review_grades SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'dataset_id',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;
END $$;

SELECT add_compression_policy('review_grades', INTERVAL '14 days', if_not_exists => true);
SELECT add_retention_policy('review_grades',   INTERVAL '180 days', if_not_exists => true);

UPDATE tsdb_schema_meta SET value = '28' WHERE key = 'schema_version';
