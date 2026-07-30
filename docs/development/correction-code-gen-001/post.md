# CORRECTION-CODE-GEN-001 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** JIMINY-CODE-BACKFILL-001 disclosed follow-up.

## Verdict

**Shipped.** `role_type='correction'` nodes now carry `constraint_code`
via a new `BootstrapCorrectionCodes` startup job mirroring the shipped
`BootstrapCodes` for constraints. **Live verified on mdemg-dev: 35/35
correction nodes now have codes** (0/35 pre-sprint). Two downstream
lookups widened to include correction (`matchConstraintCodeByEmbedding`,
`FindConstraintCodeForNode`). Taxonomy pin test updated:
`GuidanceCorrection` moved from "without codes" to "with codes" side.

## What shipped

- **`internal/jiminy/service.go::BootstrapCorrectionCodes`** — new
  function mirroring `BootstrapCodes`. Queries `role_type='correction'`
  nodes lacking codes, generates via the shipped
  `ConstraintCodeGenerator` LLM path (with fallback to
  `auto-<hash>`), writes back with
  `constraint_code_assigned_by="mdemg-bootstrap-correction"`.
- **`internal/api/server.go`** — wired into the B3 startup goroutine
  alongside `BootstrapCodes`. Each phase now gets its OWN 120s
  context (widened from shared 60s — live-caught: sharing starved the
  second phase mid-run when the first consumed most of the budget;
  11/35 codified on the first attempt, then 24 more on the widened-
  budget retry).
- **`internal/jiminy/service.go::matchConstraintCodeByEmbedding`** —
  widened `c.role_type = 'constraint'` filter to
  `c.role_type IN ['constraint', 'correction']`. Correction-typed
  guidance items can now match their own correction node's code
  (near-1.0 sim).
- **`internal/jiminy/persistence.go::FindConstraintCodeForNode`** —
  the JIMINY-CODE-BACKFILL-001 defensive backfill helper widened to
  include correction (was constraint-only).
- **`internal/jiminy/service.go::RecordOutcome`** — the defensive-
  backfill gate widened: `item.Type == GuidanceConstraint` → 
  `item.Type == GuidanceConstraint || item.Type == GuidanceCorrection`
  so the safety net covers both codifiable types.
- **`internal/jiminy/empty_code_taxonomy_test.go`** — MOVED
  `GuidanceCorrection` from `guidanceTypesWithoutCodes` to
  `guidanceTypesWithCodes` with justification comment referencing
  this sprint. Existing "disjoint partition" + "every type
  classified" invariants still hold.

## Live Tier-3 verification on mdemg-dev

**Pre-sprint state:**
```
MATCH (n:MemoryNode {space_id:'mdemg-dev', role_type:'correction'})
WHERE NOT coalesce(n.is_archived, false)
RETURN count(*) AS total, count(CASE WHEN n.constraint_code<>'' THEN 1 END) AS with_code;
=> total=35, with_code=0
```

**Post-sprint state (after 2 bootstrap runs — first at 60s ran out
of budget, second at 120s completed):**
```
=> total=35, with_code=35, bootstrapped=35
```

**Sample generated codes:**
- `always-commit-before-goreleaser` (mnemonic, from LLM)
- `always-follow-12-section-format` (mnemonic)
- `always-use-cuidv2` (mnemonic)
- `auto-29156377a1de` (fallback hash — LLM either collided or
  returned unusable content; both are acceptable per the
  `fallbackCode` design)

**Startup log:**
```
level=INFO msg="jiminy: bootstrap correction codification complete" codified=11  (first restart, ran out of budget)
level=INFO msg="jiminy: bootstrap correction codification complete" codified=24  (second restart, widened budget)
```

## Rules pinned

⚠️ **When the startup B3 goroutine bootstraps codes across two
categories, each phase gets its OWN context.** Sharing a single
timeout drains the budget of the second phase when the first
consumes most of it (live-caught: 11/35 corrections codified on the
first attempt after the constraint phase ran ~50s of the 60s shared
budget). Widen to 120s per phase minimum and always give each phase
its own `context.WithTimeout`.

## Follow-ups

- **In-line code-gen at correction promotion time** (deferred). The
  correction producer at `internal/hidden/correction_nodes.go`
  runs inside a Neo4j write transaction with no service-scope access
  to the `ConstraintCodeGenerator`. Adding it inline would require an
  interface injection + the LLM call inside a transaction (risky).
  Startup-time bootstrap covers 100% of cases idempotently and runs
  fast on typical corpus sizes. If corpus grows to >200 corrections
  and startup-time codegen becomes slow, revisit.
- **Retroactive backfill of the 21 historical Class C empty-code
  `constraint_outcomes` rows** — deferred. Forward-facing writes now
  carry codes correctly; retroactive TSDB backfill would require a
  migration and doesn't unblock any consumer.

## Documents Accessed

- `docs/development/jiminy-code-backfill-001/post.md` (parent —
  disclosed follow-up)
- `docs/development/correction-code-gen-001/sprint_plan.md` (this dir)
- `internal/jiminy/codegen.go` (`ConstraintCodeGenerator` reference)
- `internal/jiminy/service.go::BootstrapCodes` (mirror template)
- `internal/hidden/correction_nodes.go` (correction producer — kept
  as-is; codegen deferred to startup bootstrap)
- `internal/api/server.go` (startup wire site — the shared-timeout
  bug lived here)
- `internal/jiminy/empty_code_taxonomy_test.go` (pin update)
- Live Neo4j queries + server log for verification
