-- Migration 008: Add instance_id to training data tables
-- Enables multi-instance data isolation when sharing TSDB infrastructure.
-- Existing data gets empty string (backward compatible).

ALTER TABLE llm_interactions
    ADD COLUMN IF NOT EXISTS instance_id TEXT NOT NULL DEFAULT '';

ALTER TABLE embedding_events
    ADD COLUMN IF NOT EXISTS instance_id TEXT NOT NULL DEFAULT '';

ALTER TABLE retrieval_events
    ADD COLUMN IF NOT EXISTS instance_id TEXT NOT NULL DEFAULT '';

-- Composite indexes for instance-filtered queries
CREATE INDEX IF NOT EXISTS idx_llm_interactions_instance_task_time
    ON llm_interactions (instance_id, task_name, time DESC);

CREATE INDEX IF NOT EXISTS idx_embedding_events_instance_time
    ON embedding_events (instance_id, time DESC);

CREATE INDEX IF NOT EXISTS idx_retrieval_events_instance_time
    ON retrieval_events (instance_id, time DESC);
