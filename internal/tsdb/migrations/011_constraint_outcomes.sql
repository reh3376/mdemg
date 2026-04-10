-- Migration 011: Constraint Outcomes Event Table
-- Purpose: Records individual Jiminy constraint guidance outcomes to TSDB so
-- Grafana can compute Constraint Effectiveness dynamically over user-selected
-- time ranges (instead of relying on a pre-computed all-time or fixed-window gauge).

CREATE TABLE IF NOT EXISTS constraint_outcomes (
    time            TIMESTAMPTZ      NOT NULL,
    space_id        TEXT             NOT NULL,
    constraint_id   TEXT             NOT NULL,
    constraint_code TEXT,
    guidance_id     TEXT             NOT NULL,
    session_id      TEXT,
    outcome_type    TEXT             NOT NULL,
    similarity      DOUBLE PRECISION DEFAULT 0.0,
    guidance_type   TEXT,
    instance_id     TEXT
);

SELECT create_hypertable('constraint_outcomes', 'time',
    chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_constraint_outcomes_space
    ON constraint_outcomes (space_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_constraint_outcomes_constraint
    ON constraint_outcomes (constraint_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_constraint_outcomes_outcome
    ON constraint_outcomes (outcome_type, time DESC);

UPDATE tsdb_schema_meta SET value = '11' WHERE key = 'schema_version';
