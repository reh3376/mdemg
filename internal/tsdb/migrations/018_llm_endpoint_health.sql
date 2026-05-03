-- Migration 018: llm_endpoint_health_events (Phase 13.5 — MLX Server Stability cutover)
-- Purpose: One row per significant LLM-endpoint health signal — primarily
-- watchdog state transitions (up/degraded/down), plus fast-fail short-circuit
-- bursts when the gate engages. Low write volume by design (≤ 1 row per state
-- change, not per probe), so this table doubles as the historical record of
-- "how stable is the LLM endpoint" without inflating chunk size.
--
-- Why this exists:
--   Pre-Phase 13.5, mlx_lm.server crashed every ~14 min. Phase 13.5 migrated
--   to llama-server (GGUF Q5_K_M). Empirical bake-off showed 0 crashes in
--   160 min, but operators need a *historical* metric to confirm the
--   substrate stays stable in real production load over weeks/months. Prior
--   to this migration the only health signal was Prometheus gauges/counters
--   which are non-persistent. This table makes endpoint-health observable
--   in Grafana over arbitrary time ranges, surviving server restarts.
--
-- Population: written by `internal/mlxprobe.Prober.OnTransition` callback
-- (state changes) and `Prober.SetFastFailObserver` (rate-limited bursts) when
-- the watchdog is enabled (`MLX_WATCHDOG_ENABLED=true`, default). The writer
-- is wired in `internal/cli/serve.go` next to existing watchdog setup.
--
-- Sprint: POST-FT-LORA-PHASE13.5 (2026-05-03)
--
-- Rollback (manual):
--   DROP TABLE IF EXISTS llm_endpoint_health_events CASCADE;
--   UPDATE tsdb_schema_meta SET value = '17' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS llm_endpoint_health_events (
    event_id              TEXT             NOT NULL,        -- CUIDv2 (MEMORY)
    recorded_at           TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    -- The LLM endpoint URL the watchdog probes. Identifies the backend
    -- (e.g. http://127.0.0.1:8102/v1 = llama-server post-Phase-13.5;
    -- http://127.0.0.1:8101/v1 = legacy mlx_lm.server).
    endpoint_url          TEXT             NOT NULL,
    -- Event kind:
    --   'state_transition'         — watchdog state machine moved (up/degraded/down)
    --   'fast_fail_burst'          — fast-fail gate engaged (rate-limited; 1 row per
    --                                burst, not per short-circuit)
    --   'probe_recovery'           — explicit recovery marker (after Down → Up)
    event_kind            TEXT             NOT NULL,
    -- For state_transition: previous + new state (one of: up/degraded/down/unknown)
    -- For fast_fail_burst: from_state = 'down', to_state = 'down' (signals burst)
    from_state            TEXT,
    to_state              TEXT,
    -- Last error string from the failed probe (truncated to 500 chars). NULL
    -- on healthy transitions (down → up, degraded → up).
    error_message         TEXT,
    -- Probe consecutive-failure count at the moment of the event. 0 on
    -- healthy transitions; matches the hysteresis threshold (default 3) at
    -- up → down transitions.
    consecutive_failures  INTEGER,
    -- For fast_fail_burst: how many short-circuits were aggregated into this
    -- row (rate-limited window of 30s default). NULL for state transitions.
    burst_short_circuits  INTEGER,
    -- The Prometheus gauge value at event time (0=up, 1=degraded, 2=down).
    -- Stored explicitly for fast Grafana queries that don't need to derive
    -- it from to_state.
    state_numeric         INTEGER,

    PRIMARY KEY (recorded_at, event_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0016/V0017.
-- Low row volume means we'll see ~weeks per chunk in steady state, but the
-- consistent chunk policy simplifies retention scripts.
SELECT create_hypertable('llm_endpoint_health_events', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes for the most common query patterns:
--   1. "Recent events for this endpoint" (Grafana dashboards)
--   2. "All state transitions in window" (filter by event_kind)
--   3. "All transitions to down/degraded in window" (alerting + reports)
CREATE INDEX IF NOT EXISTS idx_llm_endpoint_health_url_time
    ON llm_endpoint_health_events (endpoint_url, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_endpoint_health_kind_time
    ON llm_endpoint_health_events (event_kind, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_endpoint_health_to_state
    ON llm_endpoint_health_events (to_state, recorded_at DESC)
    WHERE to_state IN ('degraded', 'down');

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing INTEGER;
BEGIN
    -- V0018 is additive-only. Re-apply must preserve any rows already
    -- written (e.g. from a previous partial apply during dev iteration).
    SELECT COUNT(*) INTO v_existing FROM llm_endpoint_health_events;
    IF v_existing > 0 THEN
        RAISE NOTICE 'V0018 pre-migration: found % existing llm_endpoint_health_events rows (preserved across re-apply)',
            v_existing;
    ELSE
        RAISE NOTICE 'V0018 pre-migration: empty table, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '18' WHERE key = 'schema_version';
