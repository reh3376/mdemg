-- Migration 009: Backfill empty space_id on pre-migration records.
-- All existing records with empty space_id belong to 'mdemg-dev'.
-- 18 embedding_events with space_id='synergy-buffer' are untouched
-- (WHERE space_id = '' excludes them).

UPDATE llm_interactions SET space_id = 'mdemg-dev' WHERE space_id = '';
UPDATE embedding_events SET space_id = 'mdemg-dev' WHERE space_id = '';
UPDATE retrieval_events SET space_id = 'mdemg-dev' WHERE space_id = '';
