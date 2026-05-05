// V0026: ContextCatalog node label (Phase 14.2 — Note 05 sparse fingerprints).
//
// Each ContextCatalog node represents one (space_id, version) snapshot of the
// adaptive bit allocation. The Builder produces a new version on each weekly
// CycleOrchestrator refresh tick:
//   1. Mark previous active version: SET prev.is_active = false
//   2. Create new ContextCatalog node with is_active = true
//   3. Persist `bits_json` property: a JSON-encoded string holding the
//      256-element [{position, kind, ref, token}] array. Stored as a
//      string because Neo4j disallows arrays of map literals as property
//      values. The Loader unmarshals back into []BitEntry on read.
//
// `kind` enum: 'role_type_layer' | 'tag' | 'path' | 'symbol'
//
// Idempotent: indexes use IF NOT EXISTS. New ContextCatalog nodes are created
// per refresh; old versions retained for cross-version fingerprint queries
// (Note 05 §4.3 fallback).
//
// Phase 14.2 forensic confirmed: 0 ContextCatalog nodes exist pre-V0026; no
// conflict with existing labels.

// Index on (space_id, is_active) supports the fast-path "load active catalog
// for this space" lookup used by Service.Observe at observe-time fingerprint
// computation. Hot path; read-heavy.
CREATE INDEX context_catalog_active_idx IF NOT EXISTS
FOR (c:ContextCatalog) ON (c.space_id, c.is_active);

// Index on (space_id, version) supports cross-version queries for the version-
// mismatch fallback path (Note 05 §4.3) and for forensic audits.
CREATE INDEX context_catalog_version_idx IF NOT EXISTS
FOR (c:ContextCatalog) ON (c.space_id, c.version);

// Record migration as applied.
MERGE (mig:Migration {version: 26})
ON CREATE SET mig.name = 'V0026__context_catalog',
              mig.applied_at = datetime();
