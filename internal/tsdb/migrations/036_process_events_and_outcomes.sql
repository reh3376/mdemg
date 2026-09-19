-- Migration 036: process_events + process_outcomes (Sprint JIMINY-PROCESS-OBSERVER-01, task #160).
--
-- Path 2 of JIMINY-METRIC-DENOMINATOR-DESIGN-001: process-observation platform.
-- Ships the first observer (lint-before-commit); subsequent observers land in
-- sibling sprints (JIMINY-PROCESS-OBSERVER-02..-06) without further schema changes.
--
-- Purpose:
--   process_events   — one row per observed process event (lint runs, model calls,
--                      mcp calls, retrieval calls, bash commands, git commits, plan
--                      mode entries). Written by the client-side hook (post-tool
--                      -observe.py) via POST /v1/process/event → buffered writer.
--   process_outcomes — one row per process-graded outcome. Parallel to
--                      constraint_outcomes shape. Written by the internal grader
--                      loop (internal/grader/process/loop.go) that reads events
--                      and runs registered matchers (starting with lint_before_commit).
--
-- Retention: 90 days on both (matches TSDB-CONSUME-001 telemetry family default).
-- Compression: 7 days on both (matches telemetry family).
-- Chunk interval: 7 days (matches V0022 shape).
--
-- Rollback (manual — matches V0022..V0035 convention):
--   DROP TABLE IF EXISTS process_events CASCADE;
--   DROP TABLE IF EXISTS process_outcomes CASCADE;
--   UPDATE tsdb_schema_meta SET value = '35' WHERE key = 'schema_version';

-- ── process_events ─────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS process_events (
    event_id        TEXT             NOT NULL,        -- CUIDv2
    time            TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    space_id        TEXT             NOT NULL,
    session_id      TEXT             NOT NULL,
    -- Enum: lint_run | model_call | mcp_call | retrieval_call | bash_command |
    --       git_commit | git_push | test_run | plan_mode_entry | file_write
    event_type      TEXT             NOT NULL,
    -- Free-form subtype (e.g. 'golangci-lint', 'ruff', 'git commit', 'haiku').
    event_subtype   TEXT             NOT NULL DEFAULT '',
    -- Enum: success | failure | unknown
    outcome         TEXT             NOT NULL DEFAULT 'unknown',
    -- Exit code for shell events (0 for success), NULL otherwise.
    exit_code       INTEGER,
    -- Free-form per-event details (file_path for file_write, tool_name, etc.).
    metadata        JSONB            NOT NULL DEFAULT '{}'::jsonb,
    -- Emitter identifier ('post-tool-observe.py', 'llmclient.recorder', etc.).
    source_hook     TEXT             NOT NULL DEFAULT '',

    PRIMARY KEY (time, event_id)
);

SELECT create_hypertable('process_events', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Session drilldown (used by matchers)
CREATE INDEX IF NOT EXISTS idx_process_events_session_time
    ON process_events (space_id, session_id, time DESC);

-- Event-type filter (used by matchers when scanning fresh events by type)
CREATE INDEX IF NOT EXISTS idx_process_events_type_time
    ON process_events (space_id, event_type, time DESC);

-- ── process_outcomes ───────────────────────────────────────────────────────
-- Parallel to constraint_outcomes shape so DatasetProvider aggregation can
-- treat the two tables uniformly via UNION.
CREATE TABLE IF NOT EXISTS process_outcomes (
    outcome_id           TEXT             NOT NULL,   -- CUIDv2
    time                 TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    space_id             TEXT             NOT NULL,
    session_id           TEXT             NOT NULL,
    -- The Neo4j constraint node the process graded (matcher-populated).
    constraint_node_id   TEXT             NOT NULL DEFAULT '',
    constraint_code      TEXT             NOT NULL DEFAULT '',
    -- Enum: process_followed | process_missed | process_incomplete
    outcome_type         TEXT             NOT NULL,
    -- The event_id from process_events that satisfied the grade (empty for missed).
    evidence_event_id    TEXT             NOT NULL DEFAULT '',
    -- Human-readable why (matcher-generated).
    reason               TEXT             NOT NULL DEFAULT '',
    -- Which grader matcher emitted this row (e.g. 'lint_before_commit').
    matcher_name         TEXT             NOT NULL,
    -- Class label (always 'process' for rows in this table — kept explicit for
    -- symmetry with constraint_outcomes.verifiability_class and for future
    -- UNION-friendly aggregation).
    verifiability_class  TEXT             NOT NULL DEFAULT 'process',

    PRIMARY KEY (time, outcome_id)
);

SELECT create_hypertable('process_outcomes', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_process_outcomes_code_time
    ON process_outcomes (space_id, constraint_code, time DESC);

CREATE INDEX IF NOT EXISTS idx_process_outcomes_session_time
    ON process_outcomes (space_id, session_id, time DESC);

CREATE INDEX IF NOT EXISTS idx_process_outcomes_matcher_time
    ON process_outcomes (space_id, matcher_name, time DESC);

-- ── retention + compression (TSDB-CONSUME-001 telemetry family default) ──
-- Enable columnstore (guarded, idempotent) before add_compression_policy —
-- mirrors V0025's pattern. add_compression_policy errors with "columnstore
-- not enabled on hypertable" when the underlying ALTER hasn't fired.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'process_events' AND compression_enabled
    ) THEN
        ALTER TABLE process_events SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id, event_type',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'process_outcomes' AND compression_enabled
    ) THEN
        ALTER TABLE process_outcomes SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id, matcher_name',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;
END $$;

SELECT add_retention_policy('process_events',   INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('process_outcomes', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_compression_policy('process_events',   INTERVAL '7 days', if_not_exists => TRUE);
SELECT add_compression_policy('process_outcomes', INTERVAL '7 days', if_not_exists => TRUE);

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_events   INTEGER;
    v_outcomes INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_events   FROM process_events;
    SELECT COUNT(*) INTO v_outcomes FROM process_outcomes;
    IF v_events > 0 OR v_outcomes > 0 THEN
        RAISE NOTICE 'V0036 pre-migration: found % process_events rows and % process_outcomes rows (preserved across re-apply)',
            v_events, v_outcomes;
    ELSE
        RAISE NOTICE 'V0036 pre-migration: empty tables, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '36' WHERE key = 'schema_version';
