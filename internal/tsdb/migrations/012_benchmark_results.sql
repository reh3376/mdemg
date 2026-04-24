-- Migration 012: Phase 10 Automated Benchmark Results
-- Purpose: Persist per-(task, run) deterministic rewards + LLM-judge scores
-- for every Phase 10 benchmark run, plus a one-row-per-benchmark aggregate.
-- Downstream consumers:
--   * Phase 11 GRPO reads benchmark_results.reward_vector + stddev as
--     advantage-normalization denominator.
--   * Grafana (`mdemg-ft-training`) reads benchmark_runs for per-task
--     pass-rate + stagnation flag.
-- Sprint: FT-LORA-PHASE10 (2026-04-23)
--
-- Rollback (manual — no down-migration framework in this codebase):
--   DROP TABLE IF EXISTS benchmark_results CASCADE;
--   DROP TABLE IF EXISTS benchmark_runs CASCADE;
--   UPDATE tsdb_schema_meta SET value = '11' WHERE key = 'schema_version';

-- ── benchmark_runs: one row per benchmark invocation ────────────────────────
CREATE TABLE IF NOT EXISTS benchmark_runs (
    run_id                   TEXT             PRIMARY KEY,  -- CUIDv2 (MEMORY)
    started_at               TIMESTAMPTZ      NOT NULL,
    completed_at             TIMESTAMPTZ,
    model_path               TEXT             NOT NULL,
    base_sha                 TEXT,
    adapter_manifest_sha     TEXT,
    dataset_sha              TEXT,
    config_sha               TEXT             NOT NULL,
    n_runs_per_task          INTEGER          NOT NULL,
    specs_total              INTEGER          NOT NULL,
    specs_with_matched_rows  INTEGER          NOT NULL,
    aggregate_weighted_score DOUBLE PRECISION,
    stagnation_flag          BOOLEAN          NOT NULL DEFAULT FALSE,
    stagnation_reason        TEXT,
    report_path              TEXT
);

CREATE INDEX IF NOT EXISTS idx_benchmark_runs_started
    ON benchmark_runs (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_benchmark_runs_model
    ON benchmark_runs (model_path, started_at DESC);

-- ── benchmark_results: one row per (run, task, run_idx) ─────────────────────
CREATE TABLE IF NOT EXISTS benchmark_results (
    recorded_at       TIMESTAMPTZ  NOT NULL,
    run_id            TEXT         NOT NULL,        -- FK → benchmark_runs.run_id
    task_id           TEXT         NOT NULL,
    run_idx           INTEGER      NOT NULL,
    sampling_group    TEXT         NOT NULL,        -- T | C | J
    response_text     TEXT,
    reward_vector     JSONB        NOT NULL,        -- deterministic rewards
    judge_scores      JSONB,                        -- {metric: score 0-1}
    judge_meta        JSONB,                        -- {metric: {model, seed, prompt_template_sha, latency_ms}}
    latency_ms        INTEGER,
    adapter_sha       TEXT,
    base_sha          TEXT,
    dataset_sha       TEXT
);

-- Hypertable partitioned on recorded_at per codebase convention (see 011).
SELECT create_hypertable('benchmark_results', 'recorded_at',
    chunk_time_interval => INTERVAL '30 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_benchmark_results_run
    ON benchmark_results (run_id, task_id, run_idx);
CREATE INDEX IF NOT EXISTS idx_benchmark_results_task
    ON benchmark_results (task_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_benchmark_results_group
    ON benchmark_results (sampling_group, recorded_at DESC);

UPDATE tsdb_schema_meta SET value = '12' WHERE key = 'schema_version';
