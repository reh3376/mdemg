// V0019 — Performance indexes for frequently filtered properties
// Supports: Jiminy frontier search, correction vector search, edge staleness queries

// Frontier nodes: used by jiminy/frontiers.go for is_frontier=true filtering
CREATE INDEX memorynode_frontier_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.is_frontier);

// Observation type: used by jiminy/corrections.go for obs_type='correction' filtering
CREATE INDEX memorynode_obs_type_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.obs_type);

// Role type: used by guardrail/constraint_retrieval.go for role_type='constraint' filtering
// (extends existing layer_role_idx which is on space_id+layer+role_type)
CREATE INDEX memorynode_role_type_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.role_type);

// CO_ACTIVATED_WITH edge updated_at: used by decay/prune for staleness checks
CREATE INDEX coactivated_updated_idx IF NOT EXISTS
FOR ()-[r:CO_ACTIVATED_WITH]-() ON (r.updated_at);

// Record migration
MERGE (m:Migration {version: 19})
ON CREATE SET m.name='V0019__performance_indexes',
              m.applied_at=datetime(),
              m.checksum=null;

MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 19
SET s.current_version = 19,
    s.updated_at = datetime();
