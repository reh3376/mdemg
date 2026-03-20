// V0021: Constraint Conflicts (FSD-2026-001 Phase F4)
// Adds CONFLICTS_WITH edge type between constraint nodes and decay properties.

// 1. Index for CONFLICTS_WITH edge lookup
CREATE INDEX idx_conflicts_with_resolution IF NOT EXISTS
FOR ()-[r:CONFLICTS_WITH]-()
ON (r.resolution_status);

// 2. Backfill expires_at and decay_rate on existing constraint nodes
MATCH (n:MemoryNode)
WHERE n.constraint_type IS NOT NULL
  AND n.expires_at IS NULL
SET n.expires_at = null,
    n.decay_rate = 0.0;

// 3. Record migration version
MERGE (m:SchemaMigration {version: 21})
SET m.applied_at = datetime(),
    m.description = 'Constraint conflicts: CONFLICTS_WITH edge, expires_at, decay_rate';
