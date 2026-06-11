-- V0025: Retention + compression for the unbounded hypertables (TSDB-CONSUME-001).
--
-- Before this migration only V0001 (metric_samples 90d/7d) and V0002
-- (llm_interactions 180d/14d) had data-management policies; every hypertable
-- added since grew forever (embedding_events had reached 2.7 GB live).
--
-- Windows (rationale in docs/development/tsdb-consume-001/sprint_plan_tsdb_consume_001.md §5):
--   90 d  retention, 7 d compression  — embedding_events, retrieval_events (pure telemetry)
--   180 d retention, 14 d compression — reinforcement_events, retrieval_audit (forensics)
--   180 d retention, no compression   — sparse_gate_metrics, scheduled_job_events,
--                                       llm_endpoint_health_events (small)
--   365 d retention                   — constraint_outcomes, guidance_conflicts
--                                       (idea-09 3-month observation window with margin)
--   none                              — uvts_*, benchmark_*, rl_*, model_install_events,
--                                       ft_*, context_catalog_versions (tiny; audit/benchmark history)
--
-- Idempotency: migrations re-run on every auto-migrate startup (migrate.go runs
-- all files). Retention/compression policies use if_not_exists; the compression
-- ALTERs are guarded because re-applying compression settings to a hypertable
-- with compressed chunks raises an error.

-- ── Compression (guarded ALTER + policy) ────────────────────────────────────

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'embedding_events' AND compression_enabled
    ) THEN
        ALTER TABLE embedding_events SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id, event_type',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'retrieval_events' AND compression_enabled
    ) THEN
        ALTER TABLE retrieval_events SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id',
            timescaledb.compress_orderby = 'time DESC'
        );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'reinforcement_events' AND compression_enabled
    ) THEN
        ALTER TABLE reinforcement_events SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id, trigger_path',
            timescaledb.compress_orderby = 'recorded_at DESC'
        );
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_name = 'retrieval_audit' AND compression_enabled
    ) THEN
        ALTER TABLE retrieval_audit SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'space_id, scorer_version',
            timescaledb.compress_orderby = 'recorded_at DESC'
        );
    END IF;
END $$;

SELECT add_compression_policy('embedding_events',     INTERVAL '7 days',  if_not_exists => true);
SELECT add_compression_policy('retrieval_events',     INTERVAL '7 days',  if_not_exists => true);
SELECT add_compression_policy('reinforcement_events', INTERVAL '14 days', if_not_exists => true);
SELECT add_compression_policy('retrieval_audit',      INTERVAL '14 days', if_not_exists => true);

-- ── Retention ────────────────────────────────────────────────────────────────

SELECT add_retention_policy('embedding_events',           INTERVAL '90 days',  if_not_exists => true);
SELECT add_retention_policy('retrieval_events',           INTERVAL '90 days',  if_not_exists => true);
SELECT add_retention_policy('reinforcement_events',       INTERVAL '180 days', if_not_exists => true);
SELECT add_retention_policy('retrieval_audit',            INTERVAL '180 days', if_not_exists => true);
SELECT add_retention_policy('sparse_gate_metrics',        INTERVAL '180 days', if_not_exists => true);
SELECT add_retention_policy('scheduled_job_events',       INTERVAL '180 days', if_not_exists => true);
SELECT add_retention_policy('llm_endpoint_health_events', INTERVAL '180 days', if_not_exists => true);
SELECT add_retention_policy('constraint_outcomes',        INTERVAL '365 days', if_not_exists => true);
SELECT add_retention_policy('guidance_conflicts',         INTERVAL '365 days', if_not_exists => true);

UPDATE tsdb_schema_meta SET value = '25' WHERE key = 'schema_version';
