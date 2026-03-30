-- Migration 007: Add RAFT context columns to llm_interactions
-- Stores retrieval context (node IDs, scores, oracle node) alongside each
-- LLM interaction for RAFT-style training data enrichment.
-- Also adds system_prompt_hash for training data curation by prompt version.

ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS retrieval_node_ids TEXT[];
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS retrieval_scores DOUBLE PRECISION[];
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS oracle_node_id TEXT;
ALTER TABLE llm_interactions ADD COLUMN IF NOT EXISTS system_prompt_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_llm_interactions_prompt_hash
    ON llm_interactions (system_prompt_hash, time DESC)
    WHERE system_prompt_hash IS NOT NULL;

UPDATE tsdb_schema_meta SET value = '7' WHERE key = 'schema_version';
