# JIMINY-CORRECTION-PRODUCER-001 — Sprint Post (2026-07-20)

## Summary
Mints the missing L1 `role_type='correction'` layer that JIMINY-ROLETYPE-ADAPTER-001
had left unblocked. 32 L0 correction observations in `mdemg-dev` sat unpromoted
because `CreateConstraintNodes` had a producer since inception and the correction
side had none. This sprint adds the sibling producer, gate, and pipeline step.

## What shipped
- **E0** — `sprint_plan_jiminy_correction_producer_001.md`.
- **E1+E2** — `internal/hidden/correction_nodes.go::CreateCorrectionNodes`
  (mirrors `CreateConstraintNodes`, 1:1 obs→node, idempotent via
  `IMPLEMENTS_CORRECTION` guard + name-identity reinforce path) +
  `internal/hidden/correction_gate.go::CorrectionPromotionGate` (min content
  length + config-driven regex deny-set). Config: `CORRECTION_PROMOTION_ENABLED`
  (default true), `_MIN_CONTENT_LEN` (default 20), `_REJECT_PATTERNS` (JSON
  regex array; defaults reuse the constraint gate's junk-class set).
- **E3** — `correctionStep` registered at consolidation phase 20 alongside
  `constraintStep`; 24 gate subtests green (junk shapes + too-short + genuine
  live corrections + edge cases); full `go test ./...` green;
  `golangci-lint run` clean.
- **E4** — live Tier-3 evidence in `live_verification.md`.
- **E5** — canonical docs updated.

## Commits (on `reh3376_dev01`)
1. `docs(jiminy-correction-producer-001): E0 — sprint plan` — `6747875`
2. `feat(jiminy-correction-producer-001): E1+E2 — CreateCorrectionNodes + gate` — `206c33b`
3. `feat(jiminy-correction-producer-001): E3 — pipeline wiring + gate tests` — `a1b98a5`
4. `docs(jiminy-correction-producer-001): E4 — live Tier-3 verification` — `4d7078c`
5. `docs(jiminy-correction-producer-001): E5 — CLAUDE.md/CHANGELOG/feature/post`

## Live evidence highlights
| Signal | Pre-sprint | Post-sprint |
|---|---|---|
| L1 `role_type='correction'` nodes in `mdemg-dev` | 0 | 32 |
| `IMPLEMENTS_CORRECTION` edges | 0 | 32 (1:1) |
| Gate rejections during first live run | — | 0 (all 32 accepted) |
| Avg L1 correction confidence | — | 0.679 |
| `/v1/memory/retrieve` result carrying `role_type='correction'` | never possible | ✅ (result [0] on "max_completion_tokens gpt-5") |
| Jiminy `/latest` `type='correction'` items | 0 | 1/8 in the smoke |
| `constraint_outcomes.guidance_type='correction'` rows | 0 (ever) | ≥1 (`followed`, tier1) |

## Lessons captured
1. **New role_type producers must mirror the shipped producer's structure.**
   Predicate + idempotency guard + gate + pipeline step + tests. Constraint
   was the template; adding a new role (e.g. `warning`, `preference`) should
   follow this exact shape and NOT invent a new promotion path.
2. **Corrections are 1:1 with their L0 obs — no type-grouping.** Constraints
   have `constraint_type` (must / must_not / …), so one obs can mint many
   constraint nodes. Corrections don't; the L1 identity is content-derived.
   This is a semantic asymmetry, not a code shortcut.
3. **Dual-promotion is intentional.** An L0 obs that carries BOTH
   `obs_type='correction'` AND `constraint:*` tags will get separate L1
   constraint AND correction nodes — the two surface differently (constraint
   before-the-fact prevention, correction after-the-fact teaching), so both
   are useful. Live-verified: no observed noise from this.
4. **Predicate-based promotion doesn't need an obs_type deny-set.** The
   constraint gate needs `deny_obs_types` because tag-based promotion can
   pick up transient obs (progress / error / task) that happen to contain
   keyword matches. The correction predicate is `obs_type='correction'`
   itself — a stronger signal — so the gate only needs content-shape
   defense (junk-content regex + min length).
5. **Dev-loop launchd/port artifact:** the fresh binary landed on `:10000`
   because a stale direct-run from a prior sprint's E4 still held `:9999`.
   Not a code issue; documented in `live_verification.md`. A clean host
   restart via full pid cleanup lands on `:9999` as normal.

## Non-goals (respected)
- Did NOT build a Jiminy contradicted-outcome → correction bridge (separate
  sprint; requires new signal semantics + operator-review gate).
- Did NOT add an operator-authored correction CLI (the existing
  `POST /v1/conversation/correct` endpoint already creates L0 obs; this
  sprint's producer picks them up).
- No schema / migration / compose changes; `IMPLEMENTS_CORRECTION` is a
  fresh Neo4j edge type (schemaless per relationship type — no migration).

## Follow-ups
- **Jiminy contradicted-outcome → correction bridge**: when Jiminy classifies
  an outcome as `contradicted`, mint a fresh L0 correction obs capturing
  "action X contradicted guidance Y". Highest-value producer for autonomous
  correction generation. Its own sprint.
- **Structured correction propagation**: `POST /v1/conversation/correct`
  captures `Incorrect` + `Correct` + `Context` as structured fields, but
  the current L1 correction content is the rendered "CORRECTION: Incorrect:
  X | Correct: Y | Context: Z" string. Propagating the structured metadata
  to L1 `structured_data` would enable first-class parsing for synthesis
  (e.g., Lever B directive phrasing could render "Do Y, not X" imperative).
- **Effectiveness measurement**: as correction outcomes accumulate, the
  JIMINY-CORPUS-001 Lever B effectiveness prior will start applying to
  correction rankings — track whether corrections get followed more or
  less than constraints of similar surface priority (informs future
  producer / surfacing tuning).

## Acceptance criteria — all met
- [x] L1 `role_type='correction'` MemoryNodes exist in `mdemg-dev` (count > 0).
- [x] `POST /v1/memory/retrieve` returns non-empty `role_type='correction'`
      on a correction-relevant query.
- [x] `/v1/jiminy/latest` surfaces `type='correction'` items.
- [x] `constraint_outcomes` gains ≥1 `guidance_type='correction'` row.
- [x] Full test suite green; lint clean.
- [x] Canonical docs updated.
