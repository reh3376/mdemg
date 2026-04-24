-- Migration 013: Phase 11 RL (GRPO) Training Runs + Steps
-- Purpose: Persist one row per GRPO training run + one row per optimization
-- step. Consumed by:
--   * neural/training/rl/trainer.py (SqlSidecarPersistence writes the INSERTs,
--     operator applies via psql; live psycopg writer is a follow-up swap).
--   * Phase 12 HITL DPO — reads rl_training_runs.final_aggregate_reward as
--     the benchmark floor for any post-GRPO refinement.
--   * Grafana `mdemg-ft-training` — step-loss trajectory + advantage
--     distribution over time.
-- Sprint: FT-LORA-PHASE11 (2026-04-24)
--
-- Rollback (manual — matches 012 convention):
--   DROP TABLE IF EXISTS rl_training_steps CASCADE;
--   DROP TABLE IF EXISTS rl_training_runs CASCADE;
--   UPDATE tsdb_schema_meta SET value = '12' WHERE key = 'schema_version';

-- ── rl_training_runs: one row per GRPO training run ─────────────────────────
CREATE TABLE IF NOT EXISTS rl_training_runs (
    run_id                   TEXT             PRIMARY KEY,  -- CUIDv2 (MEMORY)
    started_at               TIMESTAMPTZ      NOT NULL,
    completed_at             TIMESTAMPTZ,
    base_model_path          TEXT             NOT NULL,
    base_model_sha           TEXT             NOT NULL,
    adapter_manifest_sha     TEXT,
    output_path              TEXT             NOT NULL,
    config_sha               TEXT,
    -- Reward after the final eval pass (Epic 5 regression replaces this with
    -- the Phase 10 harness's aggregate when it runs; during training loop it
    -- holds the last in-loop val_reward).
    final_aggregate_reward   DOUBLE PRECISION,
    gate_verdict             TEXT             NOT NULL DEFAULT 'pending'
        CHECK (gate_verdict IN ('pending', 'pass', 'fail')),
    phase10_baseline_run_id  TEXT,                          -- advisory FK
    early_stopped            BOOLEAN          NOT NULL DEFAULT FALSE,
    early_stop_reason        TEXT
);

CREATE INDEX IF NOT EXISTS idx_rl_training_runs_started
    ON rl_training_runs (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_rl_training_runs_verdict
    ON rl_training_runs (gate_verdict, started_at DESC);

-- ── rl_training_steps: one row per optimizer step ───────────────────────────
CREATE TABLE IF NOT EXISTS rl_training_steps (
    recorded_at        TIMESTAMPTZ       NOT NULL,
    step_id            TEXT              NOT NULL,          -- CUIDv2 per step
    run_id             TEXT              NOT NULL,          -- FK → rl_training_runs
    step_idx           INTEGER           NOT NULL,
    -- Loss components from compute_grpo_loss.
    policy_loss        DOUBLE PRECISION  NOT NULL,
    kl_loss            DOUBLE PRECISION  NOT NULL,
    total_loss         DOUBLE PRECISION  NOT NULL,
    -- Advantage + reward diagnostics for post-hoc analysis.
    advantage_mean     DOUBLE PRECISION  NOT NULL,
    advantage_std      DOUBLE PRECISION  NOT NULL,
    reward_batch_mean  DOUBLE PRECISION  NOT NULL,
    mean_ratio         DOUBLE PRECISION  NOT NULL,
    frac_clipped       DOUBLE PRECISION  NOT NULL,
    n_samples          INTEGER           NOT NULL,
    n_dropped          INTEGER           NOT NULL DEFAULT 0
);

-- Hypertable per 011/012 convention; chunk size matches benchmark_results
-- (30 days) for consistency in Grafana range queries.
SELECT create_hypertable('rl_training_steps', 'recorded_at',
    chunk_time_interval => INTERVAL '30 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_rl_training_steps_run
    ON rl_training_steps (run_id, step_idx);
CREATE INDEX IF NOT EXISTS idx_rl_training_steps_recorded
    ON rl_training_steps (recorded_at DESC);

UPDATE tsdb_schema_meta SET value = '13' WHERE key = 'schema_version';
