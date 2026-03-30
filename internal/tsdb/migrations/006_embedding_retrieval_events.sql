-- Migration 006: Embedding and Retrieval Event Tables
-- Purpose: Contrastive training data collection for future embedding model fine-tuning.
-- embedding_events captures every Embed() call with parser metadata.
-- retrieval_events captures every Retrieve() pipeline with pre/post rerank scores.

CREATE TABLE IF NOT EXISTS embedding_events (
    time           TIMESTAMPTZ      NOT NULL,
    event_id       TEXT             NOT NULL,
    event_type     TEXT             NOT NULL,
    space_id       TEXT             NOT NULL,
    text_content   TEXT             NOT NULL,
    text_hash      TEXT             NOT NULL,
    text_length    INTEGER          NOT NULL,
    element_kind   TEXT,
    language       TEXT,
    file_path      TEXT,
    chunk_start    INTEGER,
    chunk_end      INTEGER,
    package_name   TEXT,
    signature      TEXT,
    tags           TEXT[],
    call_site      TEXT,
    query_text     TEXT,
    model_name     TEXT,
    provider       TEXT,
    dimensions     INTEGER,
    latency_ms     INTEGER,
    cached         BOOLEAN          DEFAULT FALSE,
    node_id        TEXT
);

SELECT create_hypertable('embedding_events', 'time',
    chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_embedding_events_type
    ON embedding_events (event_type, time DESC);
CREATE INDEX IF NOT EXISTS idx_embedding_events_element_kind
    ON embedding_events (element_kind, time DESC)
    WHERE element_kind IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_embedding_events_call_site
    ON embedding_events (call_site, time DESC)
    WHERE call_site IS NOT NULL;

CREATE TABLE IF NOT EXISTS retrieval_events (
    time               TIMESTAMPTZ      NOT NULL,
    event_id           TEXT             NOT NULL,
    space_id           TEXT             NOT NULL,
    call_site          TEXT             NOT NULL,
    query_text         TEXT             NOT NULL,
    query_hash         TEXT             NOT NULL,
    recall_node_ids    TEXT[]           NOT NULL,
    recall_scores      DOUBLE PRECISION[] NOT NULL,
    recall_k           INTEGER,
    bm25_node_ids      TEXT[],
    bm25_scores        DOUBLE PRECISION[],
    rerank_node_ids    TEXT[],
    rerank_scores      DOUBLE PRECISION[],
    rerank_model       TEXT,
    result_node_ids    TEXT[],
    result_scores      DOUBLE PRECISION[],
    result_count       INTEGER,
    guidance_id        TEXT,
    downstream_quality DOUBLE PRECISION,
    recall_latency_ms  INTEGER,
    rerank_latency_ms  INTEGER,
    total_latency_ms   INTEGER
);

SELECT create_hypertable('retrieval_events', 'time',
    chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_retrieval_events_call_site
    ON retrieval_events (call_site, time DESC);
CREATE INDEX IF NOT EXISTS idx_retrieval_events_guidance
    ON retrieval_events (guidance_id, time DESC)
    WHERE guidance_id IS NOT NULL;

UPDATE tsdb_schema_meta SET value = '6' WHERE key = 'schema_version';
