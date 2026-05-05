-- Migration 020: context_catalog_versions (Phase 14.2 — Note 05 sparse fingerprints).
--
-- Historical snapshots of ContextCatalog versions per space. The Builder
-- writes one row per (space_id, version) refresh in
-- CycleOrchestrator.RunCycle stage 6. Phase 14.2.1 reads this hypertable to
-- diagnose catalog drift (e.g. "did the bit assignments change unexpectedly
-- between v3 and v4 on whk-wms?") and to support per-version retune.
--
-- Population: written by `internal/hidden/context_catalog.go::Builder.BuildForSpace`
-- via the same TSDB pool that V0017/V0019 use. Buffered + flushed via
-- CopyFrom on TSDB_FLUSH_INTERVAL_SEC (default 30s) — mirrors the V0017/V0019
-- patterns. Low write volume by design (one row per space per week).
--
-- Sprint: POST-FT-LORA-PHASE14.2 (2026-05-05)
--
-- Rollback (manual — matches V0017+V0019 convention):
--   DROP TABLE IF EXISTS context_catalog_versions CASCADE;
--   UPDATE tsdb_schema_meta SET value = '19' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS context_catalog_versions (
    version_id          TEXT             NOT NULL,        -- CUIDv2 (MEMORY)
    recorded_at         TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    space_id            TEXT             NOT NULL,
    -- Sequential version per space. Builder mints version=N+1 each refresh
    -- tick where N is the prior active version (0 if cold-start).
    version             INTEGER          NOT NULL,
    -- Total bit budget for this catalog (typically 256 per Note 05 spec, but
    -- kept here for forward compatibility if a future sprint changes it).
    total_bits          INTEGER          NOT NULL,
    -- Full bit allocation breakdown — JSON-serialized array of
    -- {position, kind, ref, token} objects. Mirrors the bits[] array on the
    -- Neo4j ContextCatalog node, but JSON here (TimescaleDB) for
    -- queryability + history retention beyond Neo4j's "active version" cap.
    allocation_json     JSONB,
    -- Convenience denormalized arrays for fast dashboarding queries
    -- (e.g. "show me how the top-32 paths shifted between v3 and v4").
    -- Only the position metadata; full token text lives in allocation_json.
    top_symbols         TEXT[],
    top_paths           TEXT[],
    top_tags            TEXT[],
    -- Per-kind bit counts so a Grafana panel can show drift over time
    -- without parsing allocation_json.
    bits_role_type_layer INTEGER,
    bits_tag             INTEGER,
    bits_path            INTEGER,
    bits_symbol          INTEGER,

    PRIMARY KEY (recorded_at, version_id)
);

-- Hypertable for time-series locality — same 7-day chunk size as V0017+V0019.
SELECT create_hypertable('context_catalog_versions', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

-- Indexes for the most common Phase 14.2.1 retune queries:
--   1. "Latest catalog version per space" (Builder.BuildForSpace lookup)
--   2. "All versions for a space ordered" (drift analysis)
CREATE INDEX IF NOT EXISTS idx_catalog_versions_space_time
    ON context_catalog_versions (space_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_catalog_versions_space_version
    ON context_catalog_versions (space_id, version DESC);

-- ── pre-migration audit ─────────────────────────────────────────────────────
DO $$
DECLARE
    v_existing INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_existing FROM context_catalog_versions;
    IF v_existing > 0 THEN
        RAISE NOTICE 'V0020 pre-migration: found % existing context_catalog_versions rows (preserved across re-apply)',
            v_existing;
    ELSE
        RAISE NOTICE 'V0020 pre-migration: empty table, fresh install';
    END IF;
END $$;

UPDATE tsdb_schema_meta SET value = '20' WHERE key = 'schema_version';
