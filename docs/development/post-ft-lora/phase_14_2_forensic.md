---
created: 2026-05-05
updated: 2026-05-05
status: phase 14.2 epic 0 output
phase: POST-FT-LORA-PHASE14.2 Epic 0
predecessor: phase 14.1.1 (commit 028bfdc)
---

# Phase 14.2 Epic 0 — Multi-Space Density Audit

> Cross-space Cypher density audit (whk-wms, mdemg-dev, linear, ubts-load) confirms Phase 14 Epic 0's single-space finding generalizes: **no space has populated `symbol` or `role` properties**. The Note 05 spec's static 64/64/64/64 split wastes 128 bits on every production space surveyed. The adaptive Catalog Builder design from Phase 14 Epic 0 (paths-dominant + tags + role_type×layer) is grounded in 4-space data; ready for Epic 1 schema implementation.

## TL;DR

| Space | Nodes | Symbol distinct | Path distinct | Role distinct | role_type | layer | Tagged |
|---|---|---|---|---|---|---|---|
| whk-wms | 9,121 | **0** | 8,360 | **0** | 5 | 5 | 8,389 |
| mdemg-dev | 80,203 | **0** | 52,116 | **0** | 12 | 6 | 57,693 |
| linear | 4,933 | **0** | 4,910 | **0** | 3 | 3 | 1,900 |
| ubts-load | 3,009 | **0** | 3,009 | **0** | 1 | 1 | **0** |

**Recommended adaptive Builder allocation** (256 bits per space):

```
Reserved: 32 bits → top-32 (role_type, layer) tuples
Reserved: 32 bits → top-32 tags (or 0 if space has no tags; bits redistributed to paths)
Discretionary: 192 bits → distributed across (paths, symbols) by log-density:
  When symbols=0: 192 → paths (current state for all 4 surveyed spaces)
  When symbols>0: split with floor 16 bits each
```

**No property conflicts**: 0 nodes have `context_fingerprint_*` properties; 0 `ContextCatalog` nodes exist. V0028 + V0029 ready to ship.

## Section 1 — Per-space density tables

### Top-10 tags per space

| whk-wms | mdemg-dev | linear |
|---|---|---|
| typescript (7353) | error-handling (46382) | linear (1900) |
| error-handling (5876) | class (33083) | issue (1751) |
| validation (5351) | logging (32232) | TEC (1733) |
| logging (3914) | python (30264) | completed (1211) |
| authentication (3176) | validation (28223) | whk-wms (709) |
| module (3164) | authorization (26294) | Feature (389) |
| temporal (2874) | authentication (24767) | unstarted (340) |
| authorization (2416) | caching (19717) | Bug (305) |
| class (2209) | temporal (9038) | it (196) |
| interface (2080) | typescript (8776) | Chore (144) |

`ubts-load`: no tags — represents the "minimal-feature space" edge case the Builder must handle.

### `role_type` distributions

- whk-wms: `leaf(8360), hidden(682), concept(50), comparison(28), config(1)` — 5 distinct
- **mdemg-dev**: `leaf(52116), emergent_concept(18305), conversation_observation(5340), hidden(3395), concept(788), comparison(123), constraint(107), conversation_theme(21), workflow(3), reference(2)` — **12 distinct** (richest space)
- linear: `leaf(4910), hidden(22), concept(1)` — 3 distinct
- ubts-load: `leaf(3009)` — 1 distinct (minimal)

### `layer` distributions

- whk-wms: L0–L4 (5 layers)
- mdemg-dev: L0–L5 (6 layers — has the L5 emergent layer)
- linear: L0–L2 (3 layers)
- ubts-load: L0 only (1 layer)

## Section 2 — Adaptive Builder algorithm (refined from Phase 14 Epic 0)

```
Inputs:
  spaceID, totalBits=256, floorBitsPerKind=16
  density per kind: paths, symbols, roles, role_types, layers, tags

Step 1: Allocate 32 bits to top-32 (role_type × layer) tuples
  (always informative; works on every space with >=1 role_type + >=1 layer)
  Edge case: ubts-load has 1×1 = 1 tuple; allocate 1 bit, save 31 for redistribution

Step 2: Allocate up to 32 bits to top-32 tags
  When tag count == 0: allocate 0 bits, save 32 for redistribution
  When tag count < 32: allocate min(distinct_count, 32) bits

Step 3: Distribute remaining bits across (paths, symbols) by log-density:
  When symbols=0: all remaining → top-N paths
  When symbols>0: split by log(1+distinct_count) ratio with floor 16 each

Step 4: Persist allocation in ContextCatalog.bits[]:
  bits = [
    {position: 0, kind: 'role_type_layer', ref: '(leaf,L0)', token: '...'},
    {position: 1, kind: 'role_type_layer', ref: '(hidden,L0)', token: '...'},
    ...
    {position: 32, kind: 'tag', ref: 'typescript', token: '...'},
    ...
    {position: 64, kind: 'path', ref: '/Users/reh3376/whk-wms/.../inventory.processor.ts', token: '...'},
    ...
  ]
```

### Per-space allocations under this algorithm

| Space | role_type×layer | tags | paths | symbols | total |
|---|---|---|---|---|---|
| whk-wms | 32 (full top-32 of 25 tuples; pad if needed) | 32 | 192 | 0 | 256 |
| mdemg-dev | 32 (top-32 of 72 tuples) | 32 | 192 | 0 | 256 |
| linear | 9 (top-9 of 9 tuples; 23 bits redistributed → tags+paths) | 32 | 215 | 0 | 256 |
| ubts-load | 1 (only L0×leaf tuple; 31 bits redistributed) | 0 (no tags; 32 bits redistributed) | 255 | 0 | 256 |

## Section 3 — Property-conflict audit

V0028 + V0029 can ship without breaking existing constraints:

| Check | Result |
|---|---|
| `context_fingerprint_active` already populated on any MemoryNode? | **0 nodes** across all 4 spaces — no conflict |
| `context_fingerprint_version` already populated? | **0 nodes** — no conflict |
| `ContextCatalog` nodes already in graph? | **0 nodes** — fresh namespace |
| Existing index named `context_fingerprint_idx`? | (verify in Epic 1 before CREATE INDEX; use `IF NOT EXISTS` regardless) |

## Section 4 — Implications for Phase 14.2 Epics

**Epic 1 (schema):**
- `MemoryNode.context_fingerprint_active []uint16` + `_version int` — no constraints needed (no existing data)
- `ContextCatalog` node label can use `is_active=true|false` for active-version selection (no migration of existing data)
- `bits[]` array of `{position, kind, ref, token}` matches Note 05 spec; `kind` enum: `role_type_layer | tag | path | symbol`

**Epic 2 (Builder):**
- Builder's `BuildForSpace(ctx, spaceID)` returns a 256-element `bits[]` array
- Cypher queries for density measurement (per kind): see Section 1 query shapes
- Determinism: `ORDER BY count DESC, name ASC` for tie-break
- Idempotent: re-running on same input produces identical bit assignments

**Epic 3 (observe-time fingerprint):**
- Path bit: lookup `obs.Metadata["file_path"]` in `bits[]` where `kind='path'`; set bit if found
- Tag bits: for each tag in `obs.Tags`, lookup in `bits[]` where `kind='tag'`; set bit if found
- role_type×layer bit: lookup `(obs.RoleType, obs.Layer)` tuple; set bit if found
- Symbol bits: deferred to post-hoc refresh (no `symbol` data exists yet anyway)

**Epic 6 (A/B):**
- The combined gate+fingerprint A/B against Phase 14.1.1 baseline is the dominant signal
- Live polysemy demo on `mdemg-dev` (richest space) is the qualitative validation

## Section 5 — Refinements to the sprint plan

The sprint plan §13 fork #2 (bit assignment policy) recommended adaptive — confirmed by data. No change needed.

**One refinement to consider**: the sprint plan's 12 config knobs include `CONTEXT_CATALOG_TOP_N_PATHS=192` and `CONTEXT_CATALOG_TOP_N_TAGS=32` as defaults. Per Section 2 table, these are the right defaults for code-rich and convo-rich spaces. The Builder should use them as caps, not floors — small spaces (linear, ubts-load) get smaller allocations by data, not by config.

## Documents Accessed

- Live Cypher queries against Neo4j on `whk-wms`, `mdemg-dev`, `linear`, `ubts-load` spaces
- `docs/development/post-ft-lora/sprint_plan_phase_14_2_note_05_sparse_fingerprints.md` (frozen plan)
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` §4 (single-space prior)
- `docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md`
