-- Migration 029: suggested_guidance on review_grades (Sprint HITL-REVIEW-001).
--
-- SME corrective capture: when a human grades a guidance item as low-quality,
-- the review form lets them author "what would have been better guidance" — a
-- task-specific, actionable directive. That free-text correction is the
-- highest-value training signal for the eventual guidance-synthesis retrain
-- (gold examples of GOOD guidance authored by an SME), so it gets its own
-- column the curation pipeline can read.
--
-- Additive ALTER (028 was already applied live, so the column lands here, not
-- in 028). Forward-only.
--
-- Rollback (manual):
--   ALTER TABLE review_grades DROP COLUMN IF EXISTS suggested_guidance;
--   UPDATE tsdb_schema_meta SET value = '28' WHERE key = 'schema_version';

ALTER TABLE review_grades
    ADD COLUMN IF NOT EXISTS suggested_guidance TEXT NOT NULL DEFAULT '';

UPDATE tsdb_schema_meta SET value = '29' WHERE key = 'schema_version';
