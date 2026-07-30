# JIMINY-CODE-BACKFILL-001 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 follow-up #3 / JIMINY-CEILING
follow-up #4. Q4 follow-up #5.

## Verdict

**Documentation + defensive-backfill + pin test shipped.** Live
investigation showed 79% of empty-code rows are legitimately empty
by design; the remaining 21% split into two classes where the
`constraint_id → constraint_code` join spec'd by DORMANT-CENSUS-002
wouldn't help (target nodes lack codes). The correct ship is the
DOCUMENTATION path plus a narrow defensive safety net for a
constraint-role edge case + a pin test that forces future
GuidanceType additions to explicitly classify their code semantics.

## Live diagnosis (mdemg-dev 7d)

**221 empty-code rows / 1402 total = 15.8%.**

| Class | Rows | % | Root cause | Fixable via join? |
|---|---|---|---|---|
| A | 175 | 79% | `guidance_type IN (pattern, concept, learning, decision)` — source nodes are not role_type='constraint' | ❌ No code exists to backfill |
| B | 26 | 12% | `guidance_type=constraint` but source is `emergent_concept` or `leaf` — mis-tagging by a code path bypassing `classifyRetrievalItem` | ❌ Target nodes don't have codes |
| C | 21 | 9% | `guidance_type=correction` against real correction node — but 0/35 correction nodes on mdemg-dev carry `constraint_code` (no producer exists) | ❌ Needs correction-code producer (follow-up sprint) |

**Zero rows** in the 47 non-Class-A "broken" cases would be helped by
the spec'd join. The ship is honest documentation + a narrow
defensive backfill for the edge case where a constraint-role source
DOES have a code but the guidance item lost it.

## What shipped

- **`internal/jiminy/persistence.go::FindConstraintCodeForNode`** —
  new narrow helper: given `spaceID + nodeID`, returns
  `constraint_code` iff the node is `role_type='constraint'` AND has
  a non-empty code. Returns empty for any other role or no code.
  Fail-open (empty on any error).
- **`internal/jiminy/service.go::RecordOutcome`** — defensive backfill
  gate:
  ```go
  if constraintCode == "" && item.Type == GuidanceConstraint &&
     constraintID != "" && s.persistence != nil {
      constraintCode = s.persistence.FindConstraintCodeForNode(ctx, req.SpaceID, constraintID)
  }
  ```
  Narrow: only fires for constraint-typed items with an empty code
  and a source node. Zero cost when unneeded.
- **`internal/jiminy/empty_code_taxonomy_test.go`** — pin test
  enforcing two invariants:
  1. `guidanceTypesWithCodes` and `guidanceTypesWithoutCodes` are
     disjoint (partition — no ambiguity)
  2. Every `GuidanceType` enum value is classified in exactly one map
     (new types force explicit decision, same forcing-function shape
     as CACHE-KEY-002)

Current taxonomy:
- **With codes**: `GuidanceConstraint` (role_type='constraint' L1
  nodes carry codes)
- **Without codes**: `GuidancePattern`, `GuidanceConcept`,
  `GuidanceLearning`, `GuidanceDecision`, `GuidanceCorrection`,
  `GuidancePreference`, `GuidanceRisk`, `GuidanceConflict`

**⚠️ `GuidanceCorrection` is in "without codes" temporarily** — once
CORRECTION-CODE-GEN-001 ships (extending `correction_nodes.go` to
mint codes at promotion time), move it to the "with codes" set + update
the CLAUDE.md pin in the same PR.

## Rules pinned

⚠️ **Empty `constraint_code` in `constraint_outcomes` is EXPECTED for
`guidance_type IN (pattern, concept, learning, decision, correction,
preference, risk, conflict)`** — these guidance types map to source
nodes that don't carry codes (only `role_type='constraint'` L1 nodes
do). Do NOT filter empty-code rows out of dashboards or alerts as if
they're a data hole; do NOT try to backfill them from Neo4j — the
target nodes have no code. Rows for `guidance_type=constraint` with
empty code ARE a mild data quality signal (either mis-tagging or
Guide()-time embedding-sim miss) and the defensive backfill in
`RecordOutcome` catches the edge case where the source node is a
real constraint but the item lost its code.

⚠️ **Adding a new GuidanceType enum value REQUIRES updating one of the
two taxonomy maps in `empty_code_taxonomy_test.go` in the same PR** —
the pin test fails otherwise. Same forcing-function shape as
CACHE-KEY-002 and DORMANT-CENSUS-002's TSDB-table inventory.

## Follow-up disclosed

**CORRECTION-CODE-GEN-001 (~1d)**: extend `internal/hidden/correction_nodes.go`
to mint a `constraint_code` at correction promotion time (mirroring
the pattern used by constraint nodes via `ConstraintCodeGenerator`).
Backfill the 35 existing correction nodes with generated codes via a
one-shot CLI (`mdemg corrections codegen-backfill` or similar). Then
extend `loadSpaceConstraintCodes` in service.go to include
`role_type='correction'` nodes so correction-typed guidance items can
match their own correction node's code (near-1.0 sim). Then move
`GuidanceCorrection` from the "without codes" taxonomy to the "with
codes" side + update this CLAUDE.md pin.

## Documents Accessed

- `docs/development/dormant-census-002/post.md` (parent — follow-up #3)
- `docs/development/jiminy-ceiling-investigation-001/post.md` (parent —
  follow-up #4, 38 empty-code rows in top-12 surface volume)
- `docs/development/jiminy-code-backfill-001/sprint_plan.md` (this dir)
- `internal/jiminy/persistence.go` (added `FindConstraintCodeForNode`)
- `internal/jiminy/service.go::RecordOutcome` (wire site)
- `internal/jiminy/types.go` (GuidanceType enum)
- `internal/hidden/correction_nodes.go` (correction producer — no
  code-gen today)
- Live queries against `constraint_outcomes` (class distribution) and
  Neo4j (target node role_type + code presence)
