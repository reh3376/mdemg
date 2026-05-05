// V0025: MemoryNode context fingerprint properties (Phase 14.2 — Note 05 sparse fingerprints).
//
// Adds `context_fingerprint_active []uint16` + `context_fingerprint_version int`
// properties on MemoryNode for Phase 14.2's two-phase fingerprint design:
//   - observe-time fingerprint uses observation-local features (path, role_type,
//     layer, top-N tags) computed from the catalog produced by V0026
//   - post-hoc refresh in CycleOrchestrator macro-cycle adds symbol bits from
//     CO_ACTIVATED_WITH edges
//
// Idempotent: property addition only (no constraint, no data migration).
//             Index uses IF NOT EXISTS.
//
// Schema: properties default NULL on existing nodes; new observations from
// `Service.Observe` set them at MERGE time once V0025 + V0026 are applied AND
// `cfg.ContextFingerprintEnabled=true`.
//
// Phase 14.2 forensic confirmed (across whk-wms, mdemg-dev, linear, ubts-load):
//   - 0 nodes have context_fingerprint_active or _version pre-existing
//   - No naming conflicts with existing properties
// See docs/development/post-ft-lora/phase_14_2_forensic.md for the audit.

// Index on _version supports version-mismatch fallback queries (cross-version
// fingerprints fall back to no-context match per Note 05 §4.3) AND the
// post-hoc refresh batch query "WHERE m.context_fingerprint_version < $latest".
CREATE INDEX context_fingerprint_idx IF NOT EXISTS
FOR (m:MemoryNode) ON (m.context_fingerprint_version);

// Record migration as applied.
MERGE (mig:Migration {version: 25})
ON CREATE SET mig.name = 'V0025__context_fingerprints',
              mig.applied_at = datetime();
