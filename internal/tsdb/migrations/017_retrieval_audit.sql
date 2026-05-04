-- Migration 017: retrieval_audit (Phase 13 — Note 04 Column-Voting Retrieval)
-- Purpose: One row per retrieve API call (when RETRIEVAL_AUDIT_ENABLED=true)
-- recording the active scorer version, the column-voting consensus_strength,
-- and per-column latency + presence breakdown. Two consumers:
--
--   1. UVTS A/B harness — Phase 13 Epic 7 reads aggregate consensus_strength
--      across the candidate run and compares to baseline.
--   2. Phase 14 (Notes 05+06) — sparse fingerprints + percentile activation
--      gate consume mean consensus_strength as an input signal. Having the
--      historical baseline available from day 1 of Phase 13's rollout makes
--      Phase 14's A/B substantially cleaner.
--
-- Population: written by `internal/retrieval/service.Retrieve` via the
-- existing TSDB pool when `cfg.RetrievalAuditEnabled=true`. Default off to
-- avoid unbounded TSDB growth — operators opt in for observation windows.
--
-- Sprint: POST-FT-LORA-PHASE13 (2026-05-01)
--
-- Rollback (manual — matches V0014/V0015/V0016 convention):
--   DROP TABLE IF EXISTS retrieval_audit CASCADE;
--   UPDATE tsdb_schema_meta SET value = '16' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS retrieval_audit (
    audit_id            TEXT             NOT NULL,        -- CUIDv2 (MEMORY)
    recorded_at         TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    -- Identifies the retrieval call. Hashing query_text avoids storing PII
    -- if queries contain user-typed content; the hash + space_id is enough
    -- for Phase 14 to locate "all retrievals for this query template" if
    -- needed.
    space_id            TEXT             NOT NULL,
    query_text_hash     TEXT,                              -- SHA256(query_text)[:16]
    -- The active scorer ('v0-linear' or 'v1-rrf4'). Bumps when column set
    -- or weight defaults change so historical rows stay segmented.
    scorer_version      TEXT             NOT NULL,
    -- Phase 13 RRF aggregate consensus across the returned top-N (mean).
    -- NULL for v0-linear rows since the legacy scorer doesn't compute it.
    consensus_strength  NUMERIC,
    -- Per-column wall-clock in milliseconds. Allows latency dashboarding +
    -- spotting a slow column without per-call spans.
    -- Shape: {"embedding": 12, "bm25": 8, "graph": 45, "structural": 67}
    per_column_latency  JSONB,
    -- How many columns the aggregator attempted (denominator for the
    -- consensus computation) and how many returned ≥1 candidate. A drop in
    -- columns_returned (e.g. graph backend down) shows up as lower
    -- consensus_strength + a Grafana panel can watch the ratio.
    columns_queried     INTEGER,
    columns_returned    INTEGER,
    -- The top-K node IDs of the final ranked output. Useful for forensic
    -- "did Phase 13 return the same nodes as legacy on this query?" queries.
    top_k_node_ids      TEXT[],
    -- Total wall-clock for the retrieve call (ms). For p50/p95 dashboards.
    total_latency_ms    NUMERIC,

    PRIMARY KEY (recorded_at, audit_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0016
-- uvts_results, V0015 guidance_conflicts, and V0013 rl_training_steps.
SELECT create_hypertable('retrieval_audit', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes for the most common query patterns:
--   1. "Recent retrievals for this space" (Grafana dashboards)
--   2. "All retrievals under this scorer_version" (Phase 13 A/B forensics)
--   3. "Find rows where the query hash matches" (Phase 14 dedup)
CREATE INDEX IF NOT EXISTS idx_retrieval_audit_space_time
    ON retrieval_audit (space_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_retrieval_audit_scorer
    ON retrieval_audit (scorer_version, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_retrieval_audit_query_hash
    ON retrieval_audit (query_text_hash);

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing INTEGER;
BEGIN
    -- V0017 is additive-only. Re-apply must preserve any rows already
    -- written (e.g. from a previous partial apply during dev iteration).
    SELECT COUNT(*) INTO v_existing FROM retrieval_audit;
    IF v_existing > 0 THEN
        RAISE NOTICE 'V0017 pre-migration: found % existing retrieval_audit rows (preserved across re-apply)',
            v_existing;
    ELSE
        RAISE NOTICE 'V0017 pre-migration: empty table, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '17' WHERE key = 'schema_version';
