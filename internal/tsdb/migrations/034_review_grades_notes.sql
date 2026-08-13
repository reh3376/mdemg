-- Migration 034: notes on review_grades (Sprint REVIEW-GRADE-NOTES-FIELD-001).
--
-- Grader reasoning capture: distinct from the shipped suggested_guidance
-- field (which is an SME-authored corrective example — "what would have
-- been better guidance") and from the rubric dimension scores. `notes` is
-- a free-text short reasoning per grade — an explicit "why this score"
-- the grader (human or auto:*) can attach so downstream corpus reviewers
-- and Phase 4b retrain analysts see the WHY alongside the WHAT.
--
-- Additive ALTER (mirror of 029's suggested_guidance addition). Forward-only.
--
-- Rollback (manual):
--   ALTER TABLE review_grades DROP COLUMN IF EXISTS notes;
--   UPDATE tsdb_schema_meta SET value = '33' WHERE key = 'schema_version';

ALTER TABLE review_grades
    ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';

UPDATE tsdb_schema_meta SET value = '34' WHERE key = 'schema_version';
