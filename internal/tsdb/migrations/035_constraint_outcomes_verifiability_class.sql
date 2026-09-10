-- Migration 035: verifiability_class column on constraint_outcomes (Sprint JIMINY-METRIC-PARTITION-001, task #158).
--
-- Path 3 of JIMINY-METRIC-DENOMINATOR-DESIGN-001: per-class metric partition.
-- The follow-rate ceiling gap surfaced by JIMINY-CEILING-INVESTIGATION-002 is
-- rooted in mixing rules of different verifiability classes into a single
-- follow-rate gauge. This column lets the aggregator split outcomes by class
-- so the honest classifier follow-rate is queryable + gauge-emittable.
--
-- Class values: 'classifier' (default), 'process', 'hybrid', 'human'.
-- See internal/jiminy/types.go::VerifiabilityClass for enum semantics.
--
-- Additive ALTER (mirror of 034's notes addition on review_grades). Forward-only.
-- Default 'classifier' preserves backward-compat: existing rows + rows written
-- by pre-#158 code + rows written for un-classed rules all read as classifier.
--
-- Rollback (manual):
--   ALTER TABLE constraint_outcomes DROP COLUMN IF EXISTS verifiability_class;
--   DROP INDEX IF EXISTS idx_constraint_outcomes_class_time;
--   UPDATE tsdb_schema_meta SET value = '34' WHERE key = 'schema_version';

ALTER TABLE constraint_outcomes
    ADD COLUMN IF NOT EXISTS verifiability_class TEXT NOT NULL DEFAULT 'classifier';

-- Index the (class, time DESC) shape the per-class gauge queries use.
CREATE INDEX IF NOT EXISTS idx_constraint_outcomes_class_time
    ON constraint_outcomes (verifiability_class, time DESC);

UPDATE tsdb_schema_meta SET value = '35' WHERE key = 'schema_version';
