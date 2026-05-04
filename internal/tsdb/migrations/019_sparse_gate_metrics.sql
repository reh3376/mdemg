-- Migration 019: sparse_gate_metrics (Phase 14 Epic 1 — Note 06 sparse activation gate)
-- Purpose: One row per retrieve call when the Note 06 percentile gate fires
-- (SPARSE_RETRIEVAL_ENABLED=true OR per-request override). Captures the gate's
-- actual behavior (active set size, dropped count, threshold, percentile
-- applied) with longer retention than Prometheus's reset cycle allows. Phase
-- 14.1 reads this to retune SPARSE_ACTIVATION_PERCENTILE / MIN_ACTIVE /
-- MAX_ACTIVE from production traffic instead of synthetic distributions.
--
-- Population: written by `internal/retrieval/service.Retrieve` via the same
-- TSDB pool that V0017 retrieval_audit uses. Same buffered-flush pattern
-- (CopyFrom every TSDB_FLUSH_INTERVAL_SEC seconds, default 30).
--
-- Companion to V0017 retrieval_audit. V0017 captures pre-gate scorer output
-- (top_k_node_ids, consensus_strength). V0019 captures the gate's effect on
-- that output. Together they answer "what fraction of the candidate set
-- crossed the bar at percentile p, and what threshold did p resolve to?"
--
-- Sprint: POST-FT-LORA-PHASE14 (2026-05-04)
--
-- Rollback (manual — matches V0014/V0015/V0016/V0017/V0018 convention):
--   DROP TABLE IF EXISTS sparse_gate_metrics CASCADE;
--   UPDATE tsdb_schema_meta SET value = '18' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS sparse_gate_metrics (
    metric_id           TEXT             NOT NULL,        -- CUIDv2 (MEMORY)
    recorded_at         TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    space_id            TEXT             NOT NULL,
    -- The percentile actually applied for this call (after any per-request
    -- override). Allows distinguishing config-default vs override calls in
    -- Phase 14.1 retune analysis.
    percentile_applied  NUMERIC          NOT NULL,
    -- Score value at the percentile cutoff. The gate admits candidates with
    -- score >= threshold, then applies MIN/MAX clamps.
    threshold_score     NUMERIC,
    -- Input candidate count (size of the list the gate received).
    input_count         INTEGER          NOT NULL,
    -- Active set size after clamps (what the gate returned to rerank).
    active_count        INTEGER          NOT NULL,
    -- Dropped count = input_count - active_count.
    dropped_count       INTEGER          NOT NULL,
    -- True when the MIN_ACTIVE floor grew the active set above what the
    -- percentile alone would have admitted. Indicates the gate was over-
    -- selective for this call's distribution.
    floor_applied       BOOLEAN          NOT NULL DEFAULT FALSE,
    -- True when the MAX_ACTIVE ceiling shrunk the active set below the
    -- percentile's admit count. Indicates a homogeneous distribution where
    -- many candidates tied at the high tail.
    ceiling_applied     BOOLEAN          NOT NULL DEFAULT FALSE,
    -- The scorer that produced the input list (denormalized from V0017 for
    -- standalone analysis without joining). Same vocabulary: 'v0-linear' /
    -- 'v1-rrf4|...'.
    scorer_version      TEXT,

    PRIMARY KEY (recorded_at, metric_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0017.
SELECT create_hypertable('sparse_gate_metrics', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes for the most common Phase 14.1 retune queries:
--   1. "Recent gate firings on this space" (Grafana panel)
--   2. "Distribution of active_count per percentile" (retune)
--   3. "Floor/ceiling firing rate" (gate is too selective / too lax)
CREATE INDEX IF NOT EXISTS idx_sparse_gate_space_time
    ON sparse_gate_metrics (space_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_sparse_gate_percentile
    ON sparse_gate_metrics (percentile_applied, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_sparse_gate_clamps
    ON sparse_gate_metrics (recorded_at DESC)
    WHERE floor_applied OR ceiling_applied;

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_existing FROM sparse_gate_metrics;
    IF v_existing > 0 THEN
        RAISE NOTICE 'V0019 pre-migration: found % existing sparse_gate_metrics rows (preserved across re-apply)',
            v_existing;
    ELSE
        RAISE NOTICE 'V0019 pre-migration: empty table, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '19' WHERE key = 'schema_version';
