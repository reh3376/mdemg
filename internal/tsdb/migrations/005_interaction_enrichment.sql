-- Migration 005: Add correlation and metadata columns to llm_interactions
-- Enables guidance_id correlation for GRPO/DPO training and source document linkage.

ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS guidance_id TEXT;
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS source_path TEXT;

-- Index for guidance_id correlation joins (conditional to avoid bloat on NULL rows)
CREATE INDEX IF NOT EXISTS idx_llm_interactions_guidance_id
    ON llm_interactions (guidance_id, time DESC)
    WHERE guidance_id IS NOT NULL;

-- Index for source_path filtering
CREATE INDEX IF NOT EXISTS idx_llm_interactions_source_path
    ON llm_interactions (source_path, time DESC)
    WHERE source_path IS NOT NULL;

UPDATE tsdb_schema_meta SET value = '5' WHERE key = 'schema_version';
