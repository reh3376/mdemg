-- Migration 037: verifiability_class column on guidance_training_rows
-- (Sprint JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001, Q5 §3 #2).
--
-- Siblings of migration 035 (which added the column to constraint_outcomes).
-- guidance_training_rows is the second write-site for JIMINY-METRIC-PARTITION-001
-- and, per this sprint, becomes the pending-queue for HITL human-class grading.
--
-- Semantics — extended:
--   - 'classifier' (default): pre-#158 rows + rows for classifier-verifiable rules
--   - 'process':   rows sourced from process-observer-graded outcomes (rare here;
--                  the write-sink is process_outcomes, not this table)
--   - 'hybrid':    rows for hybrid-verifiable rules
--   - 'human':     rows for human-verifiable rules — pending operator grade
--                  (outcome_type='pending_human_review') or graded
--                  (outcome_type='graded_human_review'; the graded outcome
--                  itself lands in constraint_outcomes with class='human')
--
-- New outcome_type values used by this sprint (informational; enum is variable):
--   - 'pending_human_review'  — emit when RecordOutcome sees a human-class item
--   - 'graded_human_review'   — writer marks the pending row after operator grades
--
-- HITL flow:
--   1. RecordOutcome emits pending_human_review row (E2)
--   2. HITL dataset 'human_class_queue' surfaces pending rows (E3)
--   3. Operator grades → sink writes constraint_outcomes with class='human'
--      + operator source, and updates this row to graded_human_review
--   4. `mdemg_jiminy_follow_rate_human` gauge moves off 0 via existing
--      GuidanceEffectivenessByClass UNION path
--
-- Additive ALTER; forward-only. Default 'classifier' preserves backward-
-- compat: existing rows + rows written by pre-#158 code all read as classifier.
--
-- Rollback (manual):
--   ALTER TABLE guidance_training_rows DROP COLUMN IF EXISTS verifiability_class;
--   DROP INDEX IF EXISTS idx_guidance_training_rows_class_time;
--   UPDATE tsdb_schema_meta SET value = '36' WHERE key = 'schema_version';

ALTER TABLE guidance_training_rows
    ADD COLUMN IF NOT EXISTS verifiability_class TEXT NOT NULL DEFAULT 'classifier';

-- Index the (class, time DESC) shape the HITL queue query uses.
CREATE INDEX IF NOT EXISTS idx_guidance_training_rows_class_time
    ON guidance_training_rows (space_id, verifiability_class, time DESC);

UPDATE tsdb_schema_meta SET value = '37' WHERE key = 'schema_version';
