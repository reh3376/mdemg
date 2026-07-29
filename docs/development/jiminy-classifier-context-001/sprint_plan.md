# JIMINY-CLASSIFIER-CONTEXT-001 — Sprint Plan

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent:** JIMINY-CEILING-INVESTIGATION-001's recommended #1 lever.
**Sibling pattern:** JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001
(same shape: prompt extension via resolver method, default-off flag,
A/B validation).

## 1. Header & Metadata

Extend the jiminy classifier prompt with a **context-mismatch credit
clause** — routes context-mismatch verdicts to `not_applicable`
(filtered out by the shipped writer gate at `service.go:1730,1762`)
instead of `ignored` (inflates the actionable denominator). Effort
~1-2 days. Risk low: same architectural pattern as the shipped
compliance-credit extension, default-off in code, A/B-gated flip.

## 2. Problem Statement

JIMINY-CEILING-INVESTIGATION-001's 16-sample hand-categorization found
**8/16 (50%) of `ignored` verdicts are context mismatch** — the rule
is durable but doesn't govern the action's context, and the classifier
labels "ignored" instead of "not_applicable".

Representative examples (from that investigation):
- Rule `no-direct-main-commits` (about git-commit-to-main), action
  `Wrote runbook.md` — Rule governs git; action was file write. Should
  be `not_applicable`; classifier said `ignored`.
- Rule `plan-mode-before-change` (about code modification), action
  `docker exec cypher-shell` (read-only query). Rule governs code
  modification; action is a query. Should be `not_applicable`.
- Rule `mandatory-e2e-docs-before-commit`, action `sed; grep`
  (investigation). Rule governs commit-time behavior; action is
  investigation. Should be `not_applicable`.

The shipped JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 sprint
addressed this class for `must_not`-type constraints ("the action
didn't touch the mechanism"). This sprint **generalizes it** to
non-`must_not` constraints that also suffer from context mismatch.

**Expected impact per JIMINY-CEILING-INVESTIGATION-001 §6:** 11%
follow rate → **~35-50%** honest follow rate (routing ~50% of current
`ignored` verdicts to `not_applicable` which gets filtered out of the
actionable denominator).

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- Add `contextMismatchCreditClause` constant to
  `internal/jiminy/outcome_classifier.go` (mirrors the shipped
  `nonViolationCreditClause` shape verbatim)
- Add `contextMismatchCredit bool` field to `OutcomeClassifier` +
  `ContextMismatchCredit bool` to `OutcomeClassifierConfig`
- Extend `resolveClassifySystemPrompt()` to splice the new clause
  when `contextMismatchCredit && !nonViolationCredit` (avoid
  double-clause when both flags on)
- Config knob `JIMINY_CONTEXT_MISMATCH_CREDIT_ENABLED` (code default
  `false`; operator flips in `.env` after A/B smoke)
- Wire through `config.go` → `NewService`/`NewOutcomeClassifier` chain
- Pin tests: byte-identical default-off (ULTS-CI-001 hash pin
  preservation) + splice-when-on
- Live smoke on `mdemg-dev`: sample 2-3 context-mismatch prompts
  flag-off vs flag-on; verify verdict flips from `ignored` →
  `not_applicable`
- Docs + CLAUDE.md pin + CHANGELOG entry

**Out of scope:**

- Multi-day A/B measurement window (deferred — same as the sibling
  compliance-credit sprint's plan for a passive re-read after 3-7d)
- Bulk relabel of historic `constraint_outcomes` rows (forward-only
  fix, per the shipped pattern)
- Any code changes to the writer gate (already correctly filters
  `not_applicable`)

## 4. Dependencies

- **JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001** (shipped): pattern
  being mirrored — same resolver method, same default-off gate, same
  ULTS pin preservation trick
- **JIMINY-OUTCOME-002** (shipped): the 4-band relevance gate that
  already routes sub-0.20 similarity to `not_applicable` (this sprint
  extends the "not_applicable" verdict range to CONTEXT-mismatched
  ABOVE-0.20 similarity as well)
- **Writer gate** (`service.go:1730,1762`, shipped): filters
  `not_applicable` out of both `constraint_outcomes` and
  `guidance_training_rows` — this sprint relies on the gate being
  correct
- **ULTS-CI-001** hash pin: `system_prompt_hash` for
  `jiminy.evaluate_llm` (default-off render must stay byte-identical
  to the historical const to avoid ULTS hash bump)

## 5. Implementation Plan (single sprint, 4 phases)

**Phase 1 — Add the clause + config**
- Constant `contextMismatchCreditClause` in
  `internal/jiminy/outcome_classifier.go` (after
  `nonViolationCreditClause`)
- New field on `OutcomeClassifier` struct + `OutcomeClassifierConfig`
- Extend `resolveClassifySystemPrompt()` to splice
- Env var `JIMINY_CONTEXT_MISMATCH_CREDIT_ENABLED` read in
  `internal/config/config.go`; wire through
- **Gate**: `go build ./...` clean

**Phase 2 — Pin tests**
- `TestResolveClassifySystemPrompt_ContextMismatch_DefaultOff_ByteIdentical`
  — flag off, prompt matches the historical const (ULTS hash
  preservation)
- `TestResolveClassifySystemPrompt_ContextMismatch_On_Extended` —
  flag on, clause appended
- `TestResolveClassifySystemPrompt_BothClauses` — both flags on,
  BOTH clauses appended (or documented alternate behavior)
- **Gate**: `go test ./internal/jiminy/... -count=1` green

**Phase 3 — Live Tier-3 smoke**
- Craft 2-3 context-mismatch fixtures (constraint + action pair)
- Direct-LLM call flag-off (via env override) → verify verdict
- Flag-on → verify verdict flips from `ignored` to `not_applicable`
- Verify service.go writer gate still filters `not_applicable` rows
  from `constraint_outcomes` (invariant)
- **Gate**: at least 1 clear flip on a real example

**Phase 4 — Docs + flag flip in .env + commit**
- CHANGELOG entry under `### Added`
- CLAUDE.md architectural pin (pattern generalization)
- Sprint post.md with data
- Flip `JIMINY_CONTEXT_MISMATCH_CREDIT_ENABLED=true` in `.env`
  (mirrors the sibling sprint's post-smoke flag-flip pattern)
- Single commit

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit)**: 3 pin tests (default-off byte-identical,
  flag-on splice, both-flags interaction)
- **Tier 2 (contract)**: `go build ./...`, `golangci-lint 0 issues`,
  `go test ./... -count=1` full green. ULTS `system_prompt_hash`
  pin preserved (verify by running `ults_runner --verify-hashes` if
  present).
- **Tier 3 (live)**: Phase 3 smoke on mdemg-dev; verify a
  context-mismatch fixture flips verdict `ignored` → `not_applicable`.

## 7. Commit Strategy

Single commit under `JIMINY-CLASSIFIER-CONTEXT-001`. Small sprint,
tight coupling between phases.

## 8. Verification Checklist

- [ ] `contextMismatchCreditClause` const added
- [ ] `contextMismatchCredit` field on struct + config
- [ ] `resolveClassifySystemPrompt` extended (default-off preserves
      byte-identical prompt)
- [ ] Env var wired; default false in code
- [ ] 3 unit tests green
- [ ] `go build`, `golangci-lint`, `go test ./...` full green
- [ ] Live smoke: 1+ context-mismatch prompt flips verdict `ignored`
      → `not_applicable`
- [ ] Writer-gate invariant verified (not_applicable stays filtered)
- [ ] CHANGELOG entry
- [ ] CLAUDE.md architectural note
- [ ] Flag flipped in `.env` after smoke

## 9. Rollback Procedures

- Revert commit → code default returns to flag-off
- `.env` flip is reversible via a single line change
- No schema change, no substrate mutation, no historic-row relabel

## 10. Risks & Mitigations

- **Risk**: over-routing to `not_applicable` — genuinely-ignored
  actions incorrectly excluded from the actionable denominator,
  making follow rate look artificially high.
  - **Mitigation**: same as the sibling sprint — the LLM prompt gives
    explicit examples of `followed` / `ignored` / `contradicted` /
    `not_applicable` so the model uses the whole range, not just
    `not_applicable`. Post-flip A/B measurement window (passive
    re-read at 3-7d) catches over-routing if follow rate rises past
    the predicted 35-50% band.
- **Risk**: interaction between context-mismatch + non-violation
  credits (both flags on) — clauses may contradict each other.
  - **Mitigation**: pin test `TestResolveClassifySystemPrompt_BothClauses`
    documents the intended behavior; the two clauses are compatible
    (non-violation credit is a SUBSET of context mismatch —
    must_not-specific — so combining them is additive with
    non-violation acting as a specific example).
- **Risk**: ULTS `system_prompt_hash` pin breaks if default-off
  isn't byte-identical.
  - **Mitigation**: pin test explicitly compares against the historical
    prompt const; the resolver method DOES NOT mutate the base const.

## 11. Documents Accessed

Filled in `post.md`.

## 12. Documentation Update

Final phase — never cut. Covered by Phase 4.
