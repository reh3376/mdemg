# CONTEXT-LIVE-001 Recon Findings (2026-06-13, HEAD post-#455)

Two read-only lanes + orchestrator live queries. All roadmap claims
CONFIRMED; sharper specifics below.

## Lane 1 — Phase-B refresh / version skew
- `RefineWithCoactivations` (internal/conversation/fingerprint.go:146):
  ZERO non-test callers. Stage-6 hook (internal/ape/cycle.go:370→412)
  only rebuilds the catalog (`BuildForSpace`) — never re-fingerprints
  nodes. CycleOrchestrator lacks a neo4j driver (the missing dep).
- Live skew (orchestrator-verified): mdemg-dev v0=753, v1=76,906,
  v2=22, v3=425; active catalog v3 (2026-06-08). whk-wms fully v3.
  Skew is structural: every weekly catalog bump widens it.
- ContextColumn (column_context.go:44-75): NO version guard — fetches
  `context_fingerprint_version` (service.go:1161) but nothing consumes
  it; cross-version Jaccard compared silently (bit positions reallocate
  per build → noise).
- Backfill CLI heals skew (migrate_context_fingerprint.go:181,264) but
  is observe-time-style only and operator-run — skew re-accumulates.
- Server-side query derivation gated on `?context=` ∈ {auto,true,1}
  (handlers.go:496-503); zero callers pass it → the 5th column is
  dormant for ALL live traffic (empty query fp → success-empty).

## Lane 2 — Category dispatch / consensus
- `req.Category` sources: body field or `?category=` only
  (handlers.go:472-478) → sparse gate (service.go:768→gate.go:51,140)
  and RRF context-weight override (service.go:718→scoring_rrf.go:89-93).
  QueryClassifier output is consumed ONLY by
  ComputeRetrievalHintsWithLLM (scoring.go:454-471) → hints; never
  feeds Category. Both per-category protections dead on live traffic.
- Vocabulary mismatch: classifier emits {code, architecture,
  relationship, data_flow, symbol_lookup, generic} (multi-label,
  '+'-joined); override keys are UVTS names (data_flow_integration,
  architecture_structure, service_relationships,
  business_logic_constraints, relationship). Only `relationship`
  overlaps; service_relationships + business_logic_constraints have NO
  classifier equivalent.
- Consensus (consensus.go:204-240): coverage = nCols_with_node /
  colsAttempted. Counting errored columns is DOCUMENTED intent
  (consensus.go:84-85). Real defects: (i) live ContextColumn always
  empty yet counted → every live query's consensus hard-capped at 0.8;
  (ii) inconsistent: disabled structural column is omitted from the
  slice (scoring_rrf.go:57-61) while other disabled columns count;
  (iii) column_context.go:8-9 comment claims exclusion that doesn't
  happen.
- Cache: Category already in CacheKey (cache.go:91,116); classifier
  runs (service.go:416) before CacheKey (service.go:444) → pre-CacheKey
  assignment is cache-safe. Gap: scorerVersion() omits the per-category
  weight map + sparse override map.
