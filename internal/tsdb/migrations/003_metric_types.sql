-- Migration 003: Add metric_type column to metric_samples
-- Supports counter deltas, histogram buckets, and typed gauge storage
-- for the MetricsRecorder (Prometheus elimination track).

-- Add metric_type column with default 'gauge' (backward-compatible)
ALTER TABLE metric_samples
    ADD COLUMN IF NOT EXISTS metric_type TEXT NOT NULL DEFAULT 'gauge';

-- Composite index for typed metric queries
CREATE INDEX IF NOT EXISTS idx_metric_samples_type_name_time
    ON metric_samples (metric_type, metric_name, time DESC);

-- Update schema version
UPDATE tsdb_schema_meta SET value = '3' WHERE key = 'schema_version';
