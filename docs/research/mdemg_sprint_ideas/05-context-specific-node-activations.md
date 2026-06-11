# 05 — Context-Specific Node Activations

**Sprint ID**: HTM-CONTEXT-ACTIVATION
**Date**: 2026-04-21 (plan authored)
**Branch**: TBD
**Scope**: Shift from "one embedding per node" to "one embedding per **(node, context)** pair," where context is a lightweight co-activation fingerprint. This resolves the polysemy problem — `ErrorHandler` in `auth.go` activated during authentication review is a materially different thing from `ErrorHandler` in `payments.go` activated during incident triage — without cluttering the node graph with duplicate symbols.

**Upstream**: [White Paper Review](mdemg-white-paper-review.md) Paper 2 (Hawkins & Ahmad 2016 — HTM; why neurons have 1000s of synapses; context-specific distal activation).

---

## Sprint Framing

The HTM thesis is that a single neuron represents "A in the context of ABC" differently from "A in the context of XYZ" via which distal dendrite fires — not by being a different neuron. MDEMG's current architecture collapses both into a single `MemoryNode` with a single embedding. This loses information that humans (and HTM neurons) preserve.

The practical symptom today: when retrieval asks about `ErrorHandler`, it returns all observations involving any `ErrorHandler` regardless of which module or task context. The one-node-one-embedding model can't discriminate. Workarounds (space scoping, role filtering) help but don't address the underlying flattening.

Proposed architecture: keep **one MemoryNode per symbol** (the graph stays stable), but each **observation** carries a lightweight `context_fingerprint` — a sparse representation of what else was active when this observation was captured. Retrieval then asks for "observations of node X whose fingerprint matches my current query context." This is HTM's distal-dendrite story, mapped onto MDEMG's observation model.

Risk is moderate: we're adding a field and a retrieval-time filter, not restructuring the graph. But the field must be cheap to compute and query at scale.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Context Fingerprint Schema | 1 | 2 | 0 | 0 | **3** |
| Fingerprint Computation | 0 | 3 | 1 | 0 | **4** |
| Context-Aware Retrieval | 0 | 3 | 1 | 0 | **4** |
| Migration for Historical Observations | 0 | 2 | 1 | 0 | **3** |
| Observability | 0 | 1 | 2 | 0 | **3** |
| Testing & Verification | 0 | 3 | 1 | 0 | **4** |
| Mandatory Documentation Phase | 0 | 5 | 2 | 0 | **7** |
| **Total** | **1** | **19** | **8** | **0** | **28** |

---

## Phase 1: Context Fingerprint Schema

**Goal**: Define what a fingerprint is, where it lives, how it's queried.

### 1.1 Fingerprint definition (CRITICAL)

**Decision** — a fingerprint is a **sparse vector** of size F (default 256) where each position corresponds to a "context bit" (a commonly-activated cluster). Only ~2% of positions are active per fingerprint. Representation: a Go `[]uint16` of active indices (sparse), or a bitset for query-time ops.

Stored on the `MemoryNode` observation records, not on the semantic symbol nodes:

```cypher
(obs:MemoryNode {role_type: 'observation', ...})
SET obs.context_fingerprint_active = [12, 47, 101, 189, 203] // sparse indices
SET obs.context_fingerprint_version = 1
```

Why sparse bitset over dense embedding:
- Much cheaper to store (few tens of bytes vs kilobytes)
- Overlap = set intersection, computable in Cypher without vector ops
- Aligns with HTM's actual SDR representation (≈2% active)

**Files**: `internal/migrations/V0028_context_fingerprint.cypher` (new), `internal/hidden/types.go` (extend observation struct)

---

### 1.2 Context bit catalog (HIGH)

**Gap**: Need to decide what each of the 256 positions *means* — the "feature dictionary."

**Fix** — Bit positions are assigned dynamically at ingestion time:

- First 64 bits: top-64 most-frequent symbol co-activations in the space (updated weekly)
- Next 64 bits: top-64 most-frequent file/package paths
- Next 64 bits: top-64 most-frequent agent roles / task types
- Last 64 bits: reserved for future use

This is per-space; each space maintains its own catalog. Catalog versioning:

```cypher
(catalog:ContextCatalog {
  space_id: $sid,
  version: 1,
  created_at: datetime(),
  bits: [ { position: 0, kind: 'symbol', ref: 'node-id-x', token: '...' }, ... ]
})
```

**Files**: `internal/hidden/context_catalog.go` (new), `internal/migrations/V0029_context_catalog.cypher`

---

### 1.3 Catalog refresh job (HIGH)

**Fix** — Weekly background job in `internal/ape/cycle.go` (macro cycle) recomputes top-N frequent features per space, issues a new catalog version. Old versions retained for historical observation lookups.

**Files**: `internal/ape/cycle.go`

---

## Phase 2: Fingerprint Computation

### 2.1 Compute fingerprint at observation time (HIGH)

**Fix** — In `internal/conversation/service.go`, after an observation is captured and the session's co-active nodes are known:

```go
func (s *Service) computeContextFingerprint(
    ctx context.Context, obs *Observation, coActiveNodes []string,
) ([]uint16, error) {
    catalog, err := s.getActiveCatalog(ctx, obs.SpaceID)
    if err != nil {
        return nil, err
    }
    activeBits := []uint16{}

    // Symbol bits: for each co-active node in catalog, set its bit
    for _, nodeID := range coActiveNodes {
        if bit, ok := catalog.SymbolBit(nodeID); ok {
            activeBits = append(activeBits, bit)
        }
    }
    // File/package bits: from obs.Metadata["file_path"]
    if filePath, ok := obs.Metadata["file_path"].(string); ok {
        if bit, ok := catalog.PathBit(filePath); ok {
            activeBits = append(activeBits, bit)
        }
    }
    // Role bits: from obs.Source
    if bit, ok := catalog.RoleBit(obs.Source); ok {
        activeBits = append(activeBits, bit)
    }
    sort.Slice(activeBits, func(i, j int) bool { return activeBits[i] < activeBits[j] })
    return activeBits, nil
}
```

**Files**: `internal/conversation/service.go`, `internal/conversation/service_test.go`

---

### 2.2 Persist on observation (HIGH)

**Fix** — Set `context_fingerprint_active` and `context_fingerprint_version` when the observation is MERGEd into Neo4j. Already wired via 1.1 schema.

**Files**: `internal/conversation/service.go`

---

### 2.3 Cheap fingerprint similarity (HIGH)

**Fix** — Define similarity as Jaccard on the sparse sets, computed in Cypher or Go:

```cypher
// Cypher: size of intersection / size of union
WITH obs, $queryFingerprint AS qfp
WITH obs, [x IN obs.context_fingerprint_active WHERE x IN qfp] AS intersect,
     obs.context_fingerprint_active + [x IN qfp WHERE NOT x IN obs.context_fingerprint_active] AS unionFp
WITH obs, toFloat(size(intersect)) / toFloat(size(unionFp)) AS ctxSim
```

For large-scale queries, offload to a bitset operation in Go.

**Files**: `internal/retrieval/context_similarity.go` (new)

---

### 2.4 Default fallback when catalog is cold (MEDIUM)

**Gap**: In a new space with no catalog yet, observations cannot get fingerprints.

**Fix** — Fallback to an empty fingerprint. Context-aware retrieval degrades gracefully (matches everything equally). After the first catalog refresh, new observations get real fingerprints.

**Files**: `internal/hidden/context_catalog.go`

---

## Phase 3: Context-Aware Retrieval

### 3.1 Query context fingerprint (HIGH)

**Fix** — A retrieval query carries its own fingerprint:

```go
type Query struct {
    // ...existing fields
    ContextFingerprint []uint16 // optional; if provided, retrieval weights
                                 // candidates by fingerprint similarity
}
```

When the caller provides context (e.g., "I'm currently working in auth.go"), the API computes the query fingerprint from the current session's recent co-activations. When no context is provided, no filtering.

**Files**: `internal/retrieval/types.go`, `internal/api/handlers_retrieve.go`

---

### 3.2 Context-similarity as a retrieval signal (HIGH)

**Fix** — Add `ctxSim` as a scoring term. In linear-scoring mode, it's another factor with weight `RETRIEVAL_CTX_SIMILARITY_WEIGHT` (default 0.15). In column-voting mode (Sprint 04), it becomes its own column `ContextColumn`.

**Files**: `internal/retrieval/column_context.go` (new, if Sprint 04 has landed), `internal/api/handlers_retrieve.go`

---

### 3.3 Context-exclusive filtering mode (HIGH)

**Fix** — Option `strict_context=true` on retrieval API: returns only observations whose fingerprint overlaps the query fingerprint by ≥ threshold (default 0.25 Jaccard). Useful when the caller wants "what did we see about X in this exact context?"

**Files**: `internal/api/handlers_retrieve.go`

---

### 3.4 Explanation surface (MEDIUM)

**Fix** — When context-aware retrieval is used, include the matching bits in the per-candidate explanation payload. Aids debugging.

**Files**: `internal/retrieval/explanation.go` (or wherever explain lives)

---

## Phase 4: Migration for Historical Observations

### 4.1 Historical backfill job (HIGH)

**Gap**: Existing observations have no fingerprint. Retrieval over legacy data degrades until they're filled.

**Fix** — One-shot backfill CLI: `mdemg migrate context-fingerprint --space <id>`. For each observation, look at its `session_id`'s co-active nodes at the time, compute fingerprint against the current catalog, persist.

Runs in batches of 1000 observations. Idempotent (skips observations with existing fingerprints of the same catalog version).

**Files**: `internal/cli/migrate.go`, `internal/hidden/context_catalog.go`

---

### 4.2 Opt-in, gradual rollout (HIGH)

**Gap**: Forced global backfill on launch is risky.

**Fix** — Default behavior: new observations get fingerprints; historical ones don't until manually triggered. Old observations without fingerprints match the "no context" fallback in retrieval.

**Files**: documentation only

---

### 4.3 Catalog migration handling (MEDIUM)

**Gap**: When the catalog refreshes (weekly), old fingerprints reference old bit positions.

**Fix** — Store `context_fingerprint_version` on each observation. Context similarity is computed only when versions match. Cross-version comparison falls back to no-context. Eventually, a batch job can re-fingerprint observations under the new catalog.

**Files**: `internal/retrieval/context_similarity.go`

---

## Phase 5: Observability

### 5.1 Prometheus metrics (HIGH)

```
mdemg_context_fingerprint_size{space_id} - histogram (number of active bits per observation)
mdemg_context_catalog_version{space_id} - gauge
mdemg_context_similarity_score{space_id} - histogram
mdemg_observations_without_fingerprint{space_id} - gauge
```

**Files**: `internal/metrics/registry.go`

### 5.2 CLI introspection (MEDIUM)

`mdemg context catalog show --space <id>` — prints current catalog.
`mdemg context fingerprint <obs_id>` — decodes fingerprint back to named features.

**Files**: `internal/cli/context.go` (new)

### 5.3 Grafana panel (MEDIUM)

New row on RSIC dashboard: "Context Fingerprinting". Coverage %, mean fingerprint size, catalog version.

**Files**: `deploy/grafana/dashboards/mdemg-rsic.json`

---

## Phase 6: Testing & Verification

### 6.1 Unit tests (HIGH)
- Catalog assignment (symbol → bit mapping is stable within a version)
- Fingerprint computation (deterministic given inputs)
- Jaccard similarity (symmetric, bounded [0,1])

### 6.2 Integration test (HIGH)
Seed 3 clusters of observations with distinct contexts (auth / payments / health-check), query from "auth" context, verify auth-cluster observations rank higher than the other two even when embedding similarity is equal.

### 6.3 A/B benchmark (HIGH)
Run whk-wms with `RETRIEVAL_CTX_SIMILARITY_WEIGHT=0` (off) vs 0.15 (default). Expected: small improvement in precision, no recall regression.

### 6.4 Catalog refresh idempotency (MEDIUM)
Run refresh twice consecutively, assert no churn in bit assignments.

---

## Phase 7: Mandatory Documentation Phase

### 7.1 CHANGELOG.md (HIGH)
### 7.2 AGENT_HANDOFF.md (HIGH)
### 7.3 VISION.md — add HTM-context section (HIGH)
### 7.4 CLAUDE.md — add context-fingerprint vocabulary (HIGH)
### 7.5 `docs/features/context-fingerprinting.md` (new feature doc) (HIGH)
### 7.6 `docs/user/cli-reference.md` — new env vars, new CLI commands (MEDIUM)
### 7.7 Homebrew beta testing guide + submodule bump (MEDIUM)

---

## Risk Analysis & Rollback

### R1: Fingerprint storage inflates observation size

**Likelihood**: Low. ~5 active bits × 2 bytes = 10 bytes per observation. Negligible.

**Mitigation**: Metrics on mean fingerprint size to catch bloat.

**Rollback**: Drop the property.

### R2: Context-aware retrieval rejects relevant observations

**Likelihood**: Medium. If the catalog is noisy or the query fingerprint is off, strict-mode retrieval may miss relevant hits.

**Mitigation**:
- Strict mode is opt-in (default is soft context weighting).
- Default Jaccard threshold 0.25 is permissive.
- Fallback on empty fingerprint (no filtering).

**Rollback**: Set `RETRIEVAL_CTX_SIMILARITY_WEIGHT=0` — fingerprints are stored but not used.

### R3: Catalog refresh breaks live queries

**Likelihood**: Low. Catalog versioning (4.3) handles version mismatch gracefully.

**Mitigation**: Versioned fingerprints; cross-version queries fall back to no-context.

**Rollback**: Revert to previous catalog version (kept for N weeks).

### R4: Migration churn on large existing graphs

**Likelihood**: Medium for spaces with >1M observations.

**Mitigation**: Backfill is opt-in, batched, resumable.

**Rollback**: Skip backfill; new observations get fingerprints, old ones don't.

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. Schema | 1.5 days |
| 2. Fingerprint Computation | 2 days |
| 3. Context-Aware Retrieval | 2 days |
| 4. Migration & Backfill | 1.5 days |
| 5. Observability | 1 day |
| 6. Testing & Verification | 2 days |
| 7. Mandatory Documentation | 0.5 day |
| **Total** | **~10.5 days** |

---

## Dependencies

**Blocks**: 06-sparse-retrieval-activation (sparse fingerprints make sparse retrieval cheaper); 08-htm-sequence-memory (HTM sequence memory benefits from context-specific activations).

**Blocked by**: None directly, but **strongly benefits from Sprint 04 landing first** — column-voting gives a natural place for `ContextColumn` to live.

**Touches but does not block**: 03 (top-down predictions could incorporate context — future extension).

---

## Documents Accessed

- `internal/conversation/service.go`
- `internal/hidden/types.go`
- White paper review Paper 2
