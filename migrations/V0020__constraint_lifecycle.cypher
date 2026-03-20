// V0020 — Constraint Lifecycle
// Adds GUIDANCE_OUTCOME relationship type, enriches ConstraintNode properties,
// adds direction property to CO_ACTIVATED_WITH edges, and new indexes.

// =============================================================================
// 1. GUIDANCE_OUTCOME RELATIONSHIP TYPE DOCUMENTATION
// =============================================================================
//
// GUIDANCE_OUTCOME: (source:MemoryNode)-[:GUIDANCE_OUTCOME]->(target:MemoryNode)
//   Tracks the outcome of guidance (constraint/correction) being surfaced.
//   Properties:
//     guidance_id    : string  — ID of the guidance observation that was surfaced
//     outcome_type   : string  — one of: "followed", "ignored", "contradicted"
//     similarity     : float64 — semantic similarity between guidance and action
//     session_id     : string  — session in which the outcome occurred
//     created_at     : datetime
//
// Neo4j creates relationship types implicitly on first use.
// Index for space_id lookups on GUIDANCE_OUTCOME edges:

CREATE INDEX guidance_outcome_space_idx IF NOT EXISTS
FOR ()-[r:GUIDANCE_OUTCOME]-() ON (r.space_id);

CREATE INDEX guidance_outcome_type_idx IF NOT EXISTS
FOR ()-[r:GUIDANCE_OUTCOME]-() ON (r.outcome_type);

CREATE INDEX guidance_outcome_guidance_idx IF NOT EXISTS
FOR ()-[r:GUIDANCE_OUTCOME]-() ON (r.guidance_id);

// =============================================================================
// 2. ENRICH CONSTRAINT NODES WITH LIFECYCLE PROPERTIES
// =============================================================================
//
// New properties on MemoryNode (role_type='constraint'):
//   detection_confidence : float64 (0-1) — confidence of constraint detection
//   scope               : string         — constraint scope (e.g., "space", "global", "session")
//   authority_level      : string         — authority source (e.g., "user", "system", "inferred")
//   last_surfaced_at     : datetime       — when constraint was last shown to the agent
//   effectiveness_score  : float64 (0-1)  — ratio of followed / total surfaced
//   total_surfaced       : int            — total times constraint was surfaced
//   total_followed       : int            — times the guidance was followed
//   total_ignored        : int            — times the guidance was ignored
//   total_contradicted   : int            — times the guidance was contradicted

// Backfill defaults on existing constraint nodes
CALL {
  MATCH (n:MemoryNode)
  WHERE n.role_type = 'constraint'
    AND n.detection_confidence IS NULL
  SET n.detection_confidence = 1.0,
      n.scope = 'space',
      n.authority_level = 'inferred',
      n.last_surfaced_at = n.created_at,
      n.effectiveness_score = 0.0,
      n.total_surfaced = 0,
      n.total_followed = 0,
      n.total_ignored = 0,
      n.total_contradicted = 0
} IN TRANSACTIONS OF 500 ROWS;

// =============================================================================
// 3. ADD DIRECTION PROPERTY TO CO_ACTIVATED_WITH EDGES
// =============================================================================
//
// direction: "bidirectional" (default), "forward", or "backward"
// Enables asymmetric Hebbian learning in future phases.

CALL {
  MATCH ()-[r:CO_ACTIVATED_WITH]-()
  WHERE r.direction IS NULL
  SET r.direction = 'bidirectional'
} IN TRANSACTIONS OF 500 ROWS;

// =============================================================================
// 4. NEW INDEXES
// =============================================================================

// Index on scope for constraint filtering
CREATE INDEX memorynode_scope_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.scope);

// Index on authority_level for constraint priority sorting
CREATE INDEX memorynode_authority_level_idx IF NOT EXISTS
FOR (n:MemoryNode) ON (n.space_id, n.authority_level);

// =============================================================================
// RECORD MIGRATION
// =============================================================================

MERGE (m:Migration {version: 20})
ON CREATE SET m.name='V0020__constraint_lifecycle',
              m.applied_at=datetime(),
              m.checksum=null;

MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 20
SET s.current_version = 20,
    s.updated_at = datetime();
