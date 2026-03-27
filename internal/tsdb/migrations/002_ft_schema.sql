-- Fine-Tuning Data Tables
-- Created now for schema completeness, populated by FT pipeline later.

-- LLM interaction records: every generative LLM call
CREATE TABLE IF NOT EXISTS llm_interactions (
    time          TIMESTAMPTZ      NOT NULL,
    trace_id      TEXT             NOT NULL,
    task_name     TEXT             NOT NULL,
    space_id      TEXT             NOT NULL,
    session_id    TEXT,
    system_prompt TEXT,
    user_prompt   TEXT             NOT NULL,
    response      TEXT,
    think_content TEXT,
    think_mode    BOOLEAN          NOT NULL DEFAULT FALSE,
    latency_ms    INTEGER,
    tokens_in     INTEGER,
    tokens_out    INTEGER,
    model_name    TEXT,
    provider      TEXT,
    error         TEXT,
    quality       DOUBLE PRECISION,
    quality_source TEXT,
    used_for_train BOOLEAN         DEFAULT FALSE,
    dataset_ver   TEXT
);

SELECT create_hypertable('llm_interactions', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_llm_interactions_task_time
    ON llm_interactions (task_name, time DESC);

SELECT add_retention_policy('llm_interactions', INTERVAL '180 days', if_not_exists => true);

ALTER TABLE llm_interactions SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'task_name, space_id',
    timescaledb.compress_orderby = 'time DESC'
);
SELECT add_compression_policy('llm_interactions', INTERVAL '14 days', if_not_exists => true);


-- Benchmark results per model evaluation run
CREATE TABLE IF NOT EXISTS ft_benchmarks (
    time           TIMESTAMPTZ      NOT NULL,
    model_version  TEXT             NOT NULL,
    task_name      TEXT             NOT NULL,
    weighted_score DOUBLE PRECISION NOT NULL,
    metrics        JSONB            NOT NULL,
    passed         BOOLEAN          NOT NULL,
    vs_previous    DOUBLE PRECISION,
    vs_baseline    DOUBLE PRECISION,
    latency_p50_ms DOUBLE PRECISION,
    latency_p95_ms DOUBLE PRECISION,
    sample_outputs JSONB
);

SELECT create_hypertable('ft_benchmarks', 'time',
    chunk_time_interval => INTERVAL '30 days',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_ft_benchmarks_version_time
    ON ft_benchmarks (model_version, time DESC);


-- Training cycle events
CREATE TABLE IF NOT EXISTS ft_training_cycles (
    time            TIMESTAMPTZ      NOT NULL,
    cycle_id        TEXT             NOT NULL,
    model_version   TEXT             NOT NULL,
    status          TEXT             NOT NULL,
    stage           TEXT,
    dataset_version TEXT,
    exogenous_ratio DOUBLE PRECISION,
    training_config JSONB,
    eval_results    JSONB,
    duration_secs   DOUBLE PRECISION,
    error           TEXT
);

SELECT create_hypertable('ft_training_cycles', 'time',
    chunk_time_interval => INTERVAL '90 days',
    if_not_exists => TRUE
);


-- Model version registry (regular table — low volume, needs UNIQUE)
CREATE TABLE IF NOT EXISTS ft_model_versions (
    deployed_at    TIMESTAMPTZ      NOT NULL,
    version        TEXT             NOT NULL UNIQUE,
    model_path     TEXT             NOT NULL,
    adapter_path   TEXT,
    base_model     TEXT             NOT NULL,
    training_cycle TEXT,
    overall_score  DOUBLE PRECISION,
    status         TEXT             NOT NULL DEFAULT 'active',
    notes          TEXT
);


-- HITL review decisions
CREATE TABLE IF NOT EXISTS ft_hitl_decisions (
    time        TIMESTAMPTZ NOT NULL,
    item_id     TEXT        NOT NULL,
    task_name   TEXT        NOT NULL,
    prompt      TEXT        NOT NULL,
    output_a    TEXT        NOT NULL,
    output_b    TEXT        NOT NULL,
    preference  TEXT        NOT NULL,
    quality_a   INTEGER,
    quality_b   INTEGER,
    reasoning   TEXT,
    reviewer    TEXT        NOT NULL
);

SELECT create_hypertable('ft_hitl_decisions', 'time',
    chunk_time_interval => INTERVAL '90 days',
    if_not_exists => TRUE
);

-- Bump schema version to 2
UPDATE tsdb_schema_meta SET value = '2' WHERE key = 'schema_version';
