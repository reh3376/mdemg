# CORRECTION-CODE-GEN-001 — Sprint Plan

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** JIMINY-CODE-BACKFILL-001 disclosed follow-up.

## 1. Header & Metadata

Mint `constraint_code` for `role_type='correction'` nodes at
promotion time via the shipped `ConstraintCodeGenerator` LLM path,
backfill the 35 existing correction nodes on mdemg-dev (mirroring the
shipped `BootstrapCodes` for constraints), and widen
`matchConstraintCodeByEmbedding` so correction-typed guidance items
can match their own correction node's code (near-1.0 sim). Move
`GuidanceCorrection` from the "without codes" taxonomy to the "with
codes" side + update the CLAUDE.md pin. ~1-1.5h effort.

## 2. Problem Statement

JIMINY-CODE-BACKFILL-001 live diagnosis showed **0/35 correction nodes
on mdemg-dev carry `constraint_code`** — the correction producer
(`internal/hidden/correction_nodes.go`, JIMINY-CORRECTION-PRODUCER-001)
mints role_type='correction' L1 nodes but never populates a code.
Result: correction-typed guidance items always land in
`constraint_outcomes` with empty `constraint_code` (Class C, 21
rows/7d), and Neo4j `GUIDANCE_OUTCOME` edges can't be attached to a
correction-node-scoped effectiveness measurement.

The taxonomy pin test in JIMINY-CODE-BACKFILL-001 marks
`GuidanceCorrection` as "without codes" — but that's tempora  ry, with
an explicit note: *"When corrections DO get codes, remove
GuidanceCorrection from this list and update the CLAUDE.md pin in
the same PR."* This sprint does that.

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- **`internal/jiminy/service.go::BootstrapCorrectionCodes`** — new
  function mirroring `BootstrapCodes` but for `role_type='correction'`.
  Sets `constraint_code`, `constraint_code_assigned_at`,
  `constraint_code_assigned_by="mdemg-bootstrap-correction"`.
- **`internal/api/server.go`** — wire `BootstrapCorrectionCodes` into
  the same startup goroutine that calls `BootstrapCodes` when
  `J17BootstrapCodification` is enabled.
- **`internal/jiminy/service.go::matchConstraintCodeByEmbedding`** —
  widen the `c.role_type = 'constraint'` filter to include correction:
  `c.role_type IN ['constraint', 'correction']`. `loadSpaceConstraintCodes`
  already returns any node with a non-empty code — no change needed there.
- **`internal/jiminy/persistence.go::FindConstraintCodeForNode`** —
  the defensive backfill from JIMINY-CODE-BACKFILL-001 is currently
  scoped to `role_type='constraint'` only. Widen to include correction.
- **`internal/jiminy/empty_code_taxonomy_test.go`** — MOVE
  `GuidanceCorrection` from `guidanceTypesWithoutCodes` to
  `guidanceTypesWithCodes`. Justification comment updated.
- **CLAUDE.md pin update** — reflect that `correction` nodes now carry
  codes; the empty-code expected list is 7 types, not 8.
- **Live smoke on mdemg-dev**: restart server → BootstrapCorrectionCodes
  runs → verify all 35 correction nodes have codes → run a jiminy
  feedback cycle that surfaces a correction → verify the resulting
  `constraint_outcomes` row has a non-empty code.

**Out of scope:**

- Retroactive backfill of the 21 historical Class C empty-code
  `constraint_outcomes` rows — these are historical rows and
  correcting them requires a separate migration. Not needed; the
  forward-facing writes are what matters.
- A CLI `mdemg corrections codegen-backfill` — the startup bootstrap
  handles it. Add a CLI only if the operator wants on-demand
  invocation later.

## 4. Method

1. Add `BootstrapCorrectionCodes` in `service.go`
2. Wire into startup at `api/server.go`
3. Widen `matchConstraintCodeByEmbedding` role filter
4. Widen `FindConstraintCodeForNode` role filter
5. Update taxonomy pin test
6. Build, lint, test
7. Live smoke: restart + verify 35 codes assigned + smoke a feedback cycle
8. Docs (post, CHANGELOG, CLAUDE.md pin update)
9. Commit

## 5. Testing Plan

- **Tier 1 (unit)**: taxonomy pin test asserts `GuidanceCorrection` is
  now in `guidanceTypesWithCodes`; existing constraint pin still holds
- **Tier 2 (integration)**: existing tests (`go test ./internal/jiminy/`)
  green
- **Tier 3 (live)**:
  - Pre-restart: `MATCH (c:MemoryNode {space_id:'mdemg-dev', role_type:'correction'}) WHERE c.constraint_code IS NOT NULL RETURN count(*)` = 0
  - Restart server with `J17BootstrapCodification=true`
  - Post-restart poll: all 35 correction nodes carry codes
    (`constraint_code_assigned_by='mdemg-bootstrap-correction'`)
  - Trigger a jiminy feedback cycle that surfaces a correction; verify
    the resulting `constraint_outcomes` row has non-empty code

## 6. Commit Strategy

Single commit under `CORRECTION-CODE-GEN-001`.

## 7. Verification Checklist

- [ ] `BootstrapCorrectionCodes` implemented (mirror of `BootstrapCodes`)
- [ ] Wired into startup goroutine
- [ ] `matchConstraintCodeByEmbedding` widened to include correction
- [ ] `FindConstraintCodeForNode` widened to include correction
- [ ] Taxonomy pin test updated (moved GuidanceCorrection to "with codes")
- [ ] Existing tests green + lint clean
- [ ] Live smoke: 35 codes assigned; feedback cycle produces non-empty
      code in `constraint_outcomes`
- [ ] CHANGELOG + CLAUDE.md pin update + post

## 8. Rollback

Revert commit. No substrate schema change; the added
`constraint_code` values on correction nodes stay (harmless — they
just aren't consumed post-revert). To fully clean state, `MATCH …
WHERE role_type='correction' AND constraint_code_assigned_by=
'mdemg-bootstrap-correction' SET constraint_code=null`.

## 9. Risks

- **Risk**: LLM code-gen for 35 corrections at startup adds ~35× LLM
  latency (~1-2 min startup delay). Mitigation: `BootstrapCodes` for
  constraints already handles the same class at startup — runs in a
  background goroutine that doesn't block server-ready. Same shape
  applies here.
- **Risk**: code collision between a constraint and a correction with
  the same theme. Mitigation: `ConstraintCodeGenerator` maintains a
  global existing-codes set for collision avoidance; corrections and
  constraints share the codebook naturally (they're both in
  `loadSpaceConstraintCodes`).
- **Risk**: correction codes leaking into places that specifically
  need constraint-only codes. Live audit: only `loadSpaceConstraintCodes`
  and `matchConstraintCodeByEmbedding` and `FindConstraintCodeForNode`
  read this data — all three are updated in this sprint to correctly
  handle the widened set.

## 10. Documents Accessed

Filled in `post.md`.
