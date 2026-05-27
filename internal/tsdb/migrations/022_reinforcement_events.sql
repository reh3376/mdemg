-- Migration 022: reinforcement_events (Sprint EVENTGRAPH-001 — Pattern Y1)
-- Purpose: one row per Hebbian co-activation pair update written via
-- internal/learning/service.go::ApplyCoactivation. Buffered + flushed via
-- CopyFrom on the standard TSDB_FLUSH_INTERVAL_SEC cadence (default 30s).
-- Pattern: V0019 (sparse_gate_metrics) buffered writer, NOT V0021
-- (model_install_events) synchronous writer — Hebbian writes are per-retrieve,
-- far higher volume than CLI-driven model install events.
--
-- Federation API: POST /v1/eventgraph/reinforcement-neighborhood orchestrates
-- (Neo4j graph traversal) + (TSDB events touching the neighborhood) for the
-- "graph-walk + event context" query class.
--
-- Sprint: EVENTGRAPH-001 (2026-05-27)
--
-- Rollback (manual — matches V0014..V0021 convention):
--   DROP TABLE IF EXISTS reinforcement_events CASCADE;
--   UPDATE tsdb_schema_meta SET value = '21' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS reinforcement_events (
    event_id              TEXT             NOT NULL,        -- CUIDv2
    recorded_at           TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    space_id              TEXT             NOT NULL,
    src_node_id           TEXT             NOT NULL,
    dst_node_id           TEXT             NOT NULL,
    -- Edge weight before this update (coalesce(r.weight, initialWeight) at the
    -- WITH clause that captures w in the Hebbian Cypher).
    prev_weight           DOUBLE PRECISION NOT NULL,
    -- Edge weight after the tanh-clamped Hebbian SET. r.weight in the Cypher.
    new_weight            DOUBLE PRECISION NOT NULL,
    -- Signed delta (new_weight - prev_weight). Positive for strengthening,
    -- negative for weakening (when this writer is later extended to
    -- ApplyNegativeFeedback in EVENTGRAPH-003).
    delta_weight          DOUBLE PRECISION NOT NULL,
    -- Evidence count AFTER the ON MATCH increment (or 1 on ON CREATE).
    evidence_count_after  INTEGER          NOT NULL,
    -- Effective eta for this pair: cfg.LearningEta * etaMult (the conv/config/
    -- same-dir multipliers). Useful for diagnosing why some pairs strengthened
    -- more than others under the same effectiveEta.
    eta_effective         DOUBLE PRECISION,
    -- The 1.0/1.5/2.0 surprise factor (HIGH/MEDIUM/NORMAL) for conversation
    -- nodes; 1.0 for code-only pairs.
    surprise_factor       DOUBLE PRECISION,
    -- ai * aj (the activation product driving Hebbian Δw).
    activation_product    DOUBLE PRECISION,
    -- pathPrefixSimilarity(a.path, b.path) from the Go pair builder.
    path_sim              DOUBLE PRECISION,
    -- Role types of the two endpoints at write time. Useful for filtering
    -- "show me reinforcement events between code and conversation nodes."
    role_a                TEXT,
    role_b                TEXT,
    obs_type_a            TEXT,
    obs_type_b            TEXT,
    -- Session driving the activation (coalesce of either endpoint's session_id).
    -- Empty string when neither endpoint has a session_id.
    session_id            TEXT,
    -- 'forward' | 'reverse' | 'bidirectional'. In symmetric mode (default,
    -- LearningAsymmetricEnabled=false), only one row per pair is emitted with
    -- direction='bidirectional'. In asymmetric mode, two rows per pair (one
    -- 'forward', one 'reverse').
    direction             TEXT,
    -- True when the ON CREATE branch fired (new edge). False when ON MATCH
    -- fired (reinforcement of an existing edge). Distinguishes "new connection
    -- formed" from "existing connection strengthened" at analysis time.
    created_new_edge      BOOLEAN          NOT NULL,
    -- Which Hebbian entry point produced this row. v1 always
    -- 'apply_coactivation'. EVENTGRAPH-003 will extend with
    -- 'apply_symbol_coactivation', 'coactivate_session',
    -- 'apply_negative_feedback'.
    trigger_path          TEXT             NOT NULL,

    PRIMARY KEY (recorded_at, event_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0017–V0021.
SELECT create_hypertable('reinforcement_events', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes for the federation query (src/dst lookups in N-hop neighborhoods)
-- plus the Grafana panel (per-space time series + session drilldown).
CREATE INDEX IF NOT EXISTS idx_reinforcement_space_time
    ON reinforcement_events (space_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_reinforcement_src
    ON reinforcement_events (space_id, src_node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_reinforcement_dst
    ON reinforcement_events (space_id, dst_node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_reinforcement_session
    ON reinforcement_events (space_id, session_id, recorded_at DESC)
    WHERE session_id IS NOT NULL AND session_id <> '';

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_existing FROM reinforcement_events;
    IF v_existing > 0 THEN
        RAISE NOTICE 'V0022 pre-migration: found % existing reinforcement_events rows (preserved across re-apply)',
            v_existing;
    ELSE
        RAISE NOTICE 'V0022 pre-migration: empty table, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '22' WHERE key = 'schema_version';
