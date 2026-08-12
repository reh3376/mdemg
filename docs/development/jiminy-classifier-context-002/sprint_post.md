# JIMINY-CLASSIFIER-CONTEXT-002 — Sprint Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Phase 3 of:** `docs/development/jiminy-ceiling-break-2/README.md`
**Prior phases:** JIMINY-CORPUS-003 (Phase 1) + LEVER-C-TIGHTEN-002 (Phase 2)

## Problem

CONTEXT-001 (2026-07-21) predicted +18-25pp actionable follow-rate lift; delivered **+3pp**. Root cause per that sprint's `ab_verdict.md`: 88% of the extra `not_applicable` routing landed on ADVISORY guidance (outside the actionable denominator), so the actionable rate barely moved. The residual class the arc needs to attack: imperative constraints (`must`/`must_not`/`should`) where the mechanism-verb didn't appear in the action, but the LLM still labeled `ignored` on topic-similarity grounds.

Ceiling data (7d ending 2026-08-12):
- `never-direct-main-commits` correction: 75 events / 62 ignored (14.7% follow)
- `never-direct-main-commits` constraint: 57 events / 48 ignored (15.8%)
- Similarity for followed p50=0.900; for ignored p50=0.800 — the LLM is NOT being tricked by embedding similarity but by TOPIC similarity ("this action mentions commits, therefore it's about commits, therefore this commit-rule applies").

## Shipped

**New classifier prompt clause: `mechanismScopeCreditClause`**

Hard-precedence gate. When enabled, the classifier is instructed to:

1. Identify the constraint's mechanism-verb(s) — the specific action-verbs indicating what the rule regulates (commit/push/merge for git rules, generate-id for identifier rules, ALTER TABLE for schema rules, etc.)
2. Check whether the agent's action-text contains any mechanism-verb OR describes performing that mechanism.
3. If NO match → `outcome=not_applicable` **UNCONDITIONALLY**. No surface similarity, keyword overlap, or topic relatedness can override.
4. Only proceed to followed/ignored/partial/contradicted determination AFTER passing the mechanism-scope gate.

This mirrors LEVER-C-TIGHTEN-002's scope-gate on the SURFACING side. After Phase 2 suppresses most out-of-scope items at surfacing, the residual misclassifications on items that DID reach the classifier are exactly what this clause catches.

**Ordering:** clause is appended LAST in `resolveClassifySystemPrompt` (after `nonViolationCreditClause` + `contextMismatchCreditClause`). LLM prompt recency effect gives the STRONGEST gate the last word. Ordering pin test in place.

**Config knob:** `JIMINY_MECHANISM_SCOPE_CREDIT_ENABLED`. Default `false` in code (ULTS `system_prompt_hash` pin preserved — default render byte-identical to base). Set `true` in `.env` after live smoke.

**Files:**
- `internal/jiminy/outcome_classifier.go` — new clause const + field on struct + resolveClassifySystemPrompt extension + NewOutcomeClassifier wire
- `internal/config/config.go` — struct field + env parse + literal wire
- `internal/jiminy/service.go` — OutcomeClassifierConfig population site + boot log includes `mechanism_scope_credit`
- `internal/jiminy/outcome_classifier_test.go` — 4 new pin tests: default-off byte-identical (ULTS pin), on renders extended, propagate through NewOutcomeClassifier, all-three-credits ordering
- `.env` — `JIMINY_MECHANISM_SCOPE_CREDIT_ENABLED=true` (operator opt-in; new operators get code default false)

## Live Tier-3 (mdemg-dev, 2026-08-12 09:43 UTC post-restart)

**Boot log:** `jiminy: semantic outcome classifier enabled ... nonviolation_credit=true context_mismatch_credit=true mechanism_scope_credit=true tier1_bypass=true` — all 4 credit flags loaded from `.env`.

**Classify endpoint smoke:** `/v1/jiminy/classify` on a doc-editing action returns `verdict:pass, confidence:0.9` — endpoint operational.

**Deep behavioral confirmation** requires natural traffic against the new prompt (i.e. real feedback events on real guidance IDs). Arrives via passive re-check: expected to see `constraint_outcomes.outcome_type='not_applicable'` share INCREASE on imperative constraints, and correspondingly the shipped writer gate (`service.go:1730,1762`) filter those from the actionable denominator → follow rate LIFTS.

## Expected delta

Per the JIMINY-CEILING-BREAK-2 arc plan: **+5-10pp actionable follow rate** over 7d.

Composite Phase 1+2+3 expected: **12% → 22-35%** by **2026-08-19** (Phase 2's passive-re-check date).

Individual attribution (Phase 3's own contribution) can be teased apart from Phase 2 by comparing pre- and post-2026-08-12-09:43 UTC constraint_outcomes classifier_source='llm' rows — the mechanism-scope gate fires only in the LLM path.

## Two arch rules pinned (CLAUDE.md)

1. **Classifier prompt-extension flags MUST default off in code so ULTS `system_prompt_hash` pin is preserved.** Behavior change ships in `.env` after live smoke. Default-off render must be BYTE-IDENTICAL to the base `classifySystemPrompt` const — verified by `TestResolveClassifySystemPrompt_MechanismScope_DefaultOff_ByteIdentical`. Any future prompt-extension clause MUST follow this shape (CONTEXT-001 established, CONTEXT-002 extends).

2. **Prompt clause ordering is narrower→broader, strongest-gate-last.** Recency-weighted LLM attention gives the LAST clause the most influence on the final verdict. When the newest clause is a HARD-PRECEDENCE gate (as `mechanismScopeCreditClause` is), it MUST be appended AFTER weaker clauses so the LLM applies it last. Pin-tested via `TestResolveClassifySystemPrompt_AllThreeCredits_Ordering`. Any future prompt clause insertion MUST justify its ordering position.

## Follow-ups disclosed

- **Per-constraint-type prompt variants** — CONTEXT-001's disclosed idea; still deferred. The current `mechanismScopeCreditClause` handles all types uniformly. If the passive re-check shows `must` vs `must_not` vs `should` classifications behave differently under the new gate, split into per-type clauses.
- **Aggregate telemetry on classifier_source='llm' verdicts** — capture pre/post `not_applicable` share to attribute Phase 3's contribution independently of Phase 2. Simple ad-hoc SQL for now; graph panel if the arc runs multiple more phases.
- **`applies_to` structured metadata on constraints** — LEVER-C-TIGHTEN-002 disclosed this as a deferred follow-up. If the mechanism-scope heuristic (both surface-side + classifier-side) proves brittle across families, structured metadata replaces both.

## Documents Accessed

- `docs/development/jiminy-ceiling-break-2/README.md` — Phase 3 spec
- `docs/development/lever-c-tighten-002/sprint_post.md` — Phase 2 precedent + mirror-mechanism reference
- `docs/development/jiminy-corpus-003/post.md` — Phase 1 precedent + arc position
- `docs/development/jiminy-classifier-context-001/ab_verdict.md` — the "+3pp of predicted +18-25pp" evidence base
- `docs/development/jiminy-actionability-compliance-credit-001/` — CONTEXT-001 shape precedent
- `internal/jiminy/outcome_classifier.go` (nonViolationCreditClause + contextMismatchCreditClause + resolveClassifySystemPrompt shape)
- `internal/config/config.go`, `internal/jiminy/service.go` (wiring)
- Live SQL: 7d constraint_outcomes similarity-distribution + top-25 follow-rate query
- Live: `/v1/jiminy/classify` smoke + boot log grep post-restart
- CLAUDE.md pins: JIMINY-CLASSIFIER-CONTEXT-001, JIMINY-CORPUS-003, LEVER-C-TIGHTEN-002, JIMINY-CEILING-BREAK-2
