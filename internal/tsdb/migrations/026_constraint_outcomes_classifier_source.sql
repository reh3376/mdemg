-- V0026: classifier_source on constraint_outcomes (JIMINY-OUTCOME-002).
--
-- Records WHICH mechanism produced each outcome verdict:
--   'tier1'     — embedding-similarity band (high → followed, low → not_applicable)
--   'llm'       — Tier-2 LLM classifier
--   'heuristic' — fallback when the LLM is unavailable/failed (the class that
--                 half-credited everything as partial_compliance while 94.9% of
--                 Tier-2 calls were context-cancelled, pre-SUPERVISOR-002)
--   'explicit'  — caller-provided outcome
--   ''          — historical rows (pre-V0026; source only inferable from the
--                 confidence value's shape)
--
-- Forward-only: no backfill. Effectiveness analysis should treat history
-- before 2026-06-11T19:00Z as heuristic-dominated (not comparable to LLM-
-- classified data).
--
-- Rollback (manual, standard convention):
--   ALTER TABLE constraint_outcomes DROP COLUMN IF EXISTS classifier_source;
--   UPDATE tsdb_schema_meta SET value = '25' WHERE key = 'schema_version';

ALTER TABLE constraint_outcomes
    ADD COLUMN IF NOT EXISTS classifier_source TEXT NOT NULL DEFAULT '';

UPDATE tsdb_schema_meta SET value = '26' WHERE key = 'schema_version';
