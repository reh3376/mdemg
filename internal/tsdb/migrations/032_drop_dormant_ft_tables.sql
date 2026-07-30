-- Migration 032: DROP the two dormant FT tables (FT-DORMANT-CLEANUP-001)
--
-- DORMANT-CENSUS-002 disclosed these as DORMANT_TO_REMOVE — early FT-schema
-- stubs (introduced in migration 002) that never wired to writers and were
-- superseded by:
--   - ft_benchmarks     → benchmark_runs + benchmark_results (V0012)
--   - ft_hitl_decisions → review_grades (V0028)
--
-- Row counts verified 0 live before this migration lands. DROP is safe
-- (no data loss). If future FT work needs the old shape, restore from
-- 002_ft_schema.sql — but the current shipped path uses the V0012/V0028
-- replacements.
--
-- Rollback: recreate the tables from 002_ft_schema.sql; UPDATE
-- tsdb_schema_meta SET value='31' WHERE key='schema_version'. No writer
-- ever produced rows for these, so there is no data-restore concern.

DROP TABLE IF EXISTS ft_benchmarks CASCADE;
DROP TABLE IF EXISTS ft_hitl_decisions CASCADE;

UPDATE tsdb_schema_meta SET value = '32' WHERE key = 'schema_version';
