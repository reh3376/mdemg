-- Migration 015: Conflicting-guidance tracker (Phase 11.6.x — Action 1)
-- Purpose: Persist one row per detected divergence between Jiminy / RSIC /
-- Consulting recommendations on the same context. The 3-month observation
-- window across this table is what makes Note 09 (FEP capstone) empirically
-- justifiable — without longitudinal divergence data, the FEP unification
-- argument has no forcing function.
--
-- Population: written by `internal/conversation/conflict_tracker.go` via the
-- pgxpool. Rate-limited at 1 row per (space_id, minute) on the writer side so
-- a busy session cannot balloon the table. context_hash + divergence_kind let
-- analytic queries collapse repeated divergences for the same situation.
--
-- Sprint: FT-LORA-PHASE11.6.x (2026-05-01)
--
-- Rollback (manual — matches 012/013/014 convention):
--   DROP TABLE IF EXISTS guidance_conflicts CASCADE;
--   UPDATE tsdb_schema_meta SET value = '14' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS guidance_conflicts (
    -- TSDB hypertable partition key
    time                       TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    -- One row uniquely identified by its CUIDv2 (MEMORY)
    conflict_id                TEXT             NOT NULL,
    space_id                   TEXT             NOT NULL,
    -- Hash of the inputs the three subsystems were reasoning about, so
    -- repeated divergences on the same situation can be deduplicated
    -- post-hoc without a heavy join. Computed by the writer.
    context_hash               TEXT             NOT NULL,
    -- Per-subsystem recommendation captured as a short label (textual,
    -- numeric, ordinal — the divergence_kind column tells the reader how
    -- to compare them). NULL means that subsystem didn't produce a
    -- recommendation in this round (which is itself informative).
    jiminy_recommendation      TEXT,
    rsic_recommendation        TEXT,
    consulting_recommendation  TEXT,
    -- One of: 'textual' (string mismatch), 'numeric' (threshold cross),
    -- 'ordinal' (rank disagreement), 'binary' (yes/no flip), 'absent'
    -- (one or more subsystems stayed silent). Free text so future kinds
    -- can be added without a schema change.
    divergence_kind            TEXT             NOT NULL,
    -- Optional human-readable rationale captured by the tracker — what
    -- specifically diverged (e.g. "consulting=block, jiminy=warn,
    -- rsic=allow"). Bounded to 1024 chars by the writer.
    rationale                  TEXT,
    -- Provenance: which decision callback site fired this record.
    -- e.g. "jiminy.guide", "rsic.cycle", "consulting.classify"
    source                     TEXT             NOT NULL,
    instance_id                TEXT             NOT NULL DEFAULT '',
    PRIMARY KEY (time, conflict_id)
);

-- TimescaleDB hypertable on time. 7-day chunk fits a 3-month window in
-- ~13 chunks while keeping insert overhead low.
SELECT create_hypertable(
    'guidance_conflicts',
    'time',
    if_not_exists => TRUE,
    chunk_time_interval => INTERVAL '7 days'
);

CREATE INDEX IF NOT EXISTS idx_guidance_conflicts_space_time
    ON guidance_conflicts (space_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_guidance_conflicts_context
    ON guidance_conflicts (context_hash);
CREATE INDEX IF NOT EXISTS idx_guidance_conflicts_kind
    ON guidance_conflicts (divergence_kind);

UPDATE tsdb_schema_meta SET value = '15' WHERE key = 'schema_version';
