-- Migration 031: applied_node_id on contradicted_correction_drafts
-- (Sprint CONTRADICTED-BRIDGE-APPLIED-NODE-ID-001)
--
-- Additive column: when the sink approves a draft it calls
-- conversation.Service.Correct which returns *ObserveResponse carrying BOTH
-- ObsID and NodeID (they can differ). PR #508's V0030 persisted only
-- applied_obs_id; downstream forensic joins against the Neo4j graph want
-- the node_id directly. This column carries the Neo4j MemoryNode.node_id
-- of the L0 correction obs the sink minted, so operators can graph-join
-- without a secondary lookup that could stale-out over consolidation.
--
-- Historical rows (pre-V0031) keep applied_node_id NULL — no backfill.
-- Deferrable follow-up CLI can populate it via
-- (obs:MemoryNode {obs_id: draft.applied_obs_id}) → obs.node_id.
--
-- Rollback (manual): the column stays (schemaless-friendly). Reverting
-- the writer code + UPDATE tsdb_schema_meta SET value='30' restores the
-- pre-sprint contract.

ALTER TABLE contradicted_correction_drafts
    ADD COLUMN IF NOT EXISTS applied_node_id TEXT;

UPDATE tsdb_schema_meta SET value = '31' WHERE key = 'schema_version';
