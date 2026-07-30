# JIMINY-CODE-BACKFILL-001 — Sprint Plan

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 disclosed follow-up #3 /
JIMINY-CEILING-INVESTIGATION-001 disclosed follow-up #4. Q4
follow-up #5.

## 1. Header & Metadata

Investigate the empty-`constraint_code` class in `constraint_outcomes`
(99 rows/48h at census time). Per the DORMANT-CENSUS-002 spec: either
backfill from `constraint_id → constraint_code` join at write-time, OR
document the class as expected for a subset of guidance items. Live
investigation showed **the vast majority (79%) is expected-empty by
design**, so the deliverable is the DOCUMENTATION path + a defensive
edge-case safety net + a pin test enforcing the taxonomy. ~1h effort.

## 2. Problem Statement

Live diagnosis on mdemg-dev (7d window):

**221 empty-code rows / 1402 total = 15.8%** — three classes:

| Class | Rows | % | Root cause |
|---|---|---|---|
| **A — Expected empty** | 175 | 79% | `guidance_type IN (pattern, concept, learning, decision)` — source nodes are NOT role_type='constraint', so no code exists to backfill |
| **B — Constraint-typed against non-constraint node** | 26 | 12% | Some code path tagged the item `constraint` guidance_type but the source is `emergent_concept` or `leaf`. Live-verified: target Neo4j nodes DON'T have codes either — join wouldn't help. Likely pre JIMINY-ROLETYPE-ADAPTER-001 residue or a bypass code path |
| **C — Correction-typed** | 21 | 9% | `role_type='correction'` nodes exist and are correctly tagged BUT the correction producer doesn't populate `constraint_code` (0/35 corrections on mdemg-dev have codes) — real gap, needs a code producer for corrections |

**Zero rows** in the 47 non-Class-A "broken" cases would be helped by
a `constraint_id → constraint_code` join, because the target nodes
themselves don't have codes (Class B: not constraint role; Class C:
correction role has no code producer).

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- **`internal/jiminy/persistence.go::FindConstraintCodeForNode`** —
  new helper that resolves `constraint_code` from a Neo4j node by
  node_id, but only when the node IS `role_type='constraint'` with a
  non-empty code. Deliberately narrow — non-constraint role types are
  EXPECTED to be codeless.
- **`internal/jiminy/service.go::RecordOutcome`** — defensive backfill
  gate: when `item.ConstraintCode` is empty AND
  `item.Type == GuidanceConstraint` AND source node_id is present, try
  the lookup. Safety net for the edge case where a constraint-role
  source reached the outcome writer without a code populated (e.g.
  concurrent code assignment mid-request, embedder failure at
  Guide()-time). Zero-cost when no lookup is needed.
- **`internal/jiminy/empty_code_taxonomy_test.go`** — pin test
  enumerating GuidanceType values that ARE expected to have empty
  codes vs those that MUST carry codes. Two invariants:
  (a) the two taxonomies are disjoint (partition, no ambiguity),
  (b) every GuidanceType enum value is classified in one of them.
  Changing the taxonomy requires updating the doc pin in the same PR.
- **Docs**: post + CHANGELOG + CLAUDE.md pin documenting the three
  empty-code classes + when the defensive backfill fires.
- **Follow-up disclosed**: CORRECTION-CODE-GEN-001 — extend
  `correction_nodes.go` to mint a code at promotion time using the
  same generator as constraints. Backfill 35 existing correction
  nodes. Once shipped, remove `GuidanceCorrection` from the
  "without codes" taxonomy in the pin test.

**Out of scope:**

- Retroactive backfill of the 221 historical empty-code rows —
  Class A (79%) is correct as-is; Class B target nodes don't have
  codes to backfill from; Class C would need CORRECTION-CODE-GEN-001
  shipped first
- Correction-code generation — separate follow-up sprint
- Class B mis-tagging investigation — the code path that tags a
  non-constraint source as `guidance_type='constraint'` is a
  separate audit (likely feedback POSTs that explicitly set the
  type, bypassing `classifyRetrievalItem`)

## 4. Method

1. Add `FindConstraintCodeForNode` helper in `persistence.go`
2. Wire the defensive backfill in `service.go::RecordOutcome`
3. Add pin test `empty_code_taxonomy_test.go`
4. Run tests + lint
5. Docs (post, CHANGELOG, CLAUDE.md pin)
6. Commit

## 5. Testing Plan

- **Tier 1 (unit)**: pin test enforces the taxonomy is a partition +
  covers every GuidanceType enum value. Existing jiminy tests remain
  green.
- **Tier 2 (integration)**: none — the defensive backfill is a
  targeted lookup with a narrow gate; no new integration surface.
- **Tier 3 (live)**: passive observation over 24-48h. The defensive
  backfill fires only for a specific edge case; if it fires
  organically, that means the primary flow missed and this safety
  net caught it. Neutral either way.

## 6. Commit Strategy

Single commit under `JIMINY-CODE-BACKFILL-001`.

## 7. Verification Checklist

- [x] `FindConstraintCodeForNode` implemented (narrow, defensive)
- [x] Defensive backfill wired at `RecordOutcome`
- [x] Pin test asserts taxonomy partition + completeness
- [x] `go test ./...` green
- [x] `golangci-lint run ./...` clean
- [x] CHANGELOG + CLAUDE.md pin + post
- [x] CORRECTION-CODE-GEN-001 disclosed as follow-up

## 8. Rollback

Revert commit. No substrate mutation; no schema change; no config
default flipped.

## 9. Risks

- **Risk**: the defensive backfill masks a real bug in the primary
  flow (Guide()-time code assignment). If the backfill fires often
  organically, the primary flow needs investigation. Mitigation: the
  lookup is bounded to constraint-role sources with a non-empty
  code — it can only ever provide a CORRECT code, never fabricate.
  Firing frequency will be visible via `constraint_code` presence
  distribution over time.
- **Risk**: the pin test forbids new guidance types from silently
  landing without taxonomy classification. This is the INTENDED
  effect — new types force an explicit decision. Same shape as the
  CACHE-KEY-002 forcing function.

## 10. Documents Accessed

Filled in `post.md`.
