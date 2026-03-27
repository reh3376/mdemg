-- MDEMG Metrics TimescaleDB Schema

-- Schema version tracking (mirrors Neo4j's schema_meta pattern)
CREATE TABLE IF NOT EXISTS tsdb_schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO tsdb_schema_meta (key, value) VALUES ('schema_version', '0')
ON CONFLICT (key) DO NOTHING;

-- Operational metrics: one row per metric per timestamp
CREATE TABLE IF NOT EXISTS metric_samples (
    time        TIMESTAMPTZ      NOT NULL,
    space_id    TEXT             NOT NULL,
    metric_name TEXT             NOT NULL,
    value       DOUBLE PRECISION NOT NULL,
    source      TEXT             NOT NULL DEFAULT 'live',
    quality_tag TEXT             NOT NULL DEFAULT 'nominal',
    labels      JSONB
);

SELECT create_hypertable('metric_samples', 'time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

-- Indexes for Grafana time-range + metric queries
CREATE INDEX IF NOT EXISTS idx_metric_samples_name_time
    ON metric_samples (metric_name, time DESC);
CREATE INDEX IF NOT EXISTS idx_metric_samples_space_time
    ON metric_samples (space_id, time DESC);

-- Continuous aggregate: hourly rollups
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_hourly
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    space_id,
    metric_name,
    avg(value)   AS avg_value,
    min(value)   AS min_value,
    max(value)   AS max_value,
    count(*)     AS sample_count
FROM metric_samples
GROUP BY bucket, space_id, metric_name
WITH NO DATA;

-- Continuous aggregate: daily rollups
CREATE MATERIALIZED VIEW IF NOT EXISTS metrics_daily
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', time) AS bucket,
    space_id,
    metric_name,
    avg(value)   AS avg_value,
    min(value)   AS min_value,
    max(value)   AS max_value,
    count(*)     AS sample_count
FROM metric_samples
GROUP BY bucket, space_id, metric_name
WITH NO DATA;

-- Retention policies
SELECT add_retention_policy('metric_samples', INTERVAL '90 days', if_not_exists => true);
SELECT add_retention_policy('metrics_hourly', INTERVAL '365 days', if_not_exists => true);

-- Compression: compress chunks older than 7 days
ALTER TABLE metric_samples SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'space_id, metric_name',
    timescaledb.compress_orderby = 'time DESC'
);
SELECT add_compression_policy('metric_samples', INTERVAL '7 days', if_not_exists => true);

-- Bump schema version to 1
UPDATE tsdb_schema_meta SET value = '1' WHERE key = 'schema_version';
