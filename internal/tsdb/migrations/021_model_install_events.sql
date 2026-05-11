-- Migration 021: model_install_events (Sprint MODEL-DIST-001 — `mdemg model` observability)
-- Purpose: One row per `mdemg model pull|verify|remove` invocation. Captures the
-- resolved backend / namespace / name / quant tuple, success state, latency, and
-- the computed SHA + declared size. Phase B of MODEL-DIST-001 wires Grafana
-- panels on top of this; for this sprint we only land the table + writer so
-- the data accumulates for later analysis.
--
-- Population: written synchronously after each CLI operation by
-- `internal/tsdb/model_install_writer.go::RecordModelInstallEvent`. Unlike the
-- buffered hypertables (V0017/V0018/V0019), this writer fires single-row
-- INSERTs because the CLI is one-shot and writes are infrequent
-- (one per `mdemg model pull` invocation, not per request).
--
-- Sprint: MODEL-DIST-001 (2026-05-11)
--
-- Rollback (manual — matches V0014..V0020 convention):
--   DROP TABLE IF EXISTS model_install_events CASCADE;
--   UPDATE tsdb_schema_meta SET value = '20' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS model_install_events (
    event_id            TEXT             NOT NULL,        -- CUIDv2 (MEMORY)
    recorded_at         TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    -- Action that produced this row. One of: 'pull', 'verify', 'remove'.
    event_type          TEXT             NOT NULL,
    -- Distribution backend that handled the request. v1: 'ollama'. Future:
    -- 'hf', 's3', 'github-release', 'file'. Pluggable per
    -- internal/cli/model_fetcher.go's NewFetcher dispatch.
    backend_name        TEXT             NOT NULL,
    -- Registry namespace (MDEMG_MODEL_NAMESPACE). Default 'reh3376'; forks
    -- override.
    namespace           TEXT             NOT NULL,
    -- Model name (MDEMG_MODEL_NAME). Default 'mdemg-llm-v1'; multi-model
    -- support without code change.
    model_name          TEXT             NOT NULL,
    -- Quant tag (e.g. 'Q5_K_M'). Empty when the event is adapter-only and
    -- adapter publication has landed (deferred to MODEL-DIST-002).
    quant               TEXT,
    -- True when --adapter was set. Sprint MODEL-DIST-001 always emits false
    -- for fused-only operations.
    adapter             BOOLEAN          NOT NULL DEFAULT FALSE,
    -- Outcome.
    success             BOOLEAN          NOT NULL,
    -- Wall-clock latency in milliseconds (Fetcher.Fetch returns this).
    latency_ms          INTEGER,
    -- Verified SHA256 of the pulled artifact (hex, lowercase). Empty on
    -- failed/verify-only/remove operations where no fresh hash was computed.
    sha256              TEXT,
    -- Declared size from the backend manifest (bytes). Useful for
    -- distinguishing aborted partial pulls from full ones in retrospect.
    size_bytes          BIGINT,
    -- Truncated error message when success=false. Backend-native error text;
    -- max 1 KB enforced by writer (PostgreSQL TEXT has no in-table limit).
    err_message         TEXT,

    PRIMARY KEY (recorded_at, event_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0017–V0020.
SELECT create_hypertable('model_install_events', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes for likely Grafana / operator queries (Phase B of MODEL-DIST-001
-- and Sprint B Grafana audit):
--   1. "Recent pulls" (most common operator query) — covered by PK.
--   2. "Pulls by quant" — RAM-tier dispatch analytics.
--   3. "Failed events" — alerting + investigation.
CREATE INDEX IF NOT EXISTS idx_model_install_quant_time
    ON model_install_events (quant, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_model_install_failed
    ON model_install_events (recorded_at DESC)
    WHERE success = FALSE;
CREATE INDEX IF NOT EXISTS idx_model_install_backend_event
    ON model_install_events (backend_name, event_type, recorded_at DESC);

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_existing FROM model_install_events;
    IF v_existing > 0 THEN
        RAISE NOTICE 'V0021 pre-migration: found % existing model_install_events rows (preserved across re-apply)',
            v_existing;
    ELSE
        RAISE NOTICE 'V0021 pre-migration: empty table, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '21' WHERE key = 'schema_version';
