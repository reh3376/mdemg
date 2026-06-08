-- Migration 023: constraint_outcomes constraint_code index (Sprint EVENTGRAPH-002)
-- Purpose: support the guidance-outcome federation join. EVENTGRAPH-002 walks a
-- constraint's Neo4j neighborhood, collects each neighbor's constraint_code, and
-- queries constraint_outcomes WHERE constraint_code = ANY(codes). The existing
-- indexes (011_constraint_outcomes.sql) cover (space_id, time), (constraint_id,
-- time), and (outcome_type, time) — none cover constraint_code, so the federation
-- join would seq-scan as the table grows. This adds the missing index.
--
-- Why constraint_code (not constraint_id): the graph↔event join key is
-- constraint_code. Neo4j role_type='constraint' nodes carry node_id (CUID) +
-- a constraint_code property; constraint_outcomes carries constraint_id (a UUID
-- that does NOT match the Neo4j node_id) + constraint_code. constraint_code is
-- the only reliable key linking the two.
--
-- Additive + idempotent — no data change, no table rewrite. The index is built
-- CONCURRENTLY-safe via IF NOT EXISTS (plain CREATE INDEX; the table is small
-- and this runs inside the migration transaction like V0011's own indexes).
--
-- Sprint: EVENTGRAPH-002 (2026-06-08)
--
-- Rollback (manual — matches V0014..V0022 convention):
--   DROP INDEX IF EXISTS idx_constraint_outcomes_code;
--   UPDATE tsdb_schema_meta SET value = '22' WHERE key = 'schema_version';

CREATE INDEX IF NOT EXISTS idx_constraint_outcomes_code
    ON constraint_outcomes (space_id, constraint_code, time DESC)
    WHERE constraint_code IS NOT NULL AND constraint_code <> '';

UPDATE tsdb_schema_meta SET value = '23' WHERE key = 'schema_version';
