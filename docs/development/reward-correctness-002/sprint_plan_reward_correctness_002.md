# Sprint Plan — REWARD-CORRECTNESS-002: Schema/Reward-Mismatch Fixes

## 1. Header & Metadata
2026-06-15 · branch `reh3376_dev01` · training-integrity remediation (the
REWARD-CORRECTNESS-001 follow-ups + the live-findings schema mismatches) ·
effort ~1d · risk **low-medium** (changes reward grading for 4 tasks; getting
it wrong mis-grades the benchmark/baseline — but every change makes grading
MORE correct, and is unit + live verified).

## 2. Problem Statement
REWARD-CORRECTNESS-001's live run found three reward/schema mismatches where a
reward scores a CORRECT response wrong (the same "reward not measuring
correctness" class as the length bias, different mechanism):
- **hidden.summarize**: ULTS schema declares `{object, required:["summary"]}`,
  but production (`cluster_summarizer.go::Summarize`) emits **bare prose**
  ("Output ONLY the summary text … under 50 words"). 72 valid summaries were
  mis-flagged invalid-JSON.
- **jiminy.evaluate + jiminy.evaluate_llm**: `explanation_quality` reads a
  TOP-LEVEL `explanation`/`reasoning` field, but the schema nests reasoning in
  `violations[].reasoning` → scores **0.0** on correct responses (mean dragged
  to 0.667).
- **jiminy.synthesize**: keyword-bag `specificity_score`/`follow_rate` sit just
  below the gate on valid-but-concise guidance that lacks the ~6 hard-coded
  magic words.

These mis-gradings corrupt the benchmark/baseline (a correct response graded
low), so they must be fixed before the honest baseline recompute.

## 3. Scope & Constraints
**In:** (1) hidden.summarize ULTS schema `object`→`string`. (2)
`explanation_quality` schema-aware: when no top-level explanation/reasoning,
credit nested `violations[]/warnings[]` reasoning (helps both jiminy.evaluate
tasks); fall back to current flat behavior. (3) keyword-bag
`specificity_score`/`actionability_score` made **substantive-floored** (valid
non-hedging response floors at a reasonable level; keyword presence is a
bounded BONUS, hedging/empty stays low) — `follow_rate` inherits it; document
jiminy.synthesize's per-task `--reward-threshold-map` use for capture. **Out:**
new reward functions; changing what the tasks emit (production unchanged);
distill TRAINING. **Decisions (proposed, picked at execution, memory
`feedback_plan_options_pattern`):** explanation_quality fix-in-place vs
drop-from-array → **fix in place** (both evaluate tasks have nested reasoning,
the function is the wrong one not the array); keyword-bag floor level →
calibrate so a substantive valid guidance lands ~0.7 (recovers false-drops)
while specific/actionable approaches 1.0 and hedging stays < 0.5.

**Constraints:** no-hardcoding; every reward change unit-tested for
(a) correct-terse scores high, (b) garbage/hedging stays low, (c) no over-
inflation of the other consumers (ape.reflect/consulting.synthesis for
actionability). Tier 3 live required.

## 4. Dependencies
`docs/tests/ults/specs/{hidden_summarize,jiminy_evaluate,jiminy_evaluate_llm,
jiminy_synthesize}.ults.json`; `neural/training/reward_functions.py`
(`explanation_quality`, `specificity_score`, `actionability_score`,
`follow_rate`) + tests; `internal/hidden/cluster_summarizer.go` (confirms
prose); live TSDB rows for the 4 tasks; the REWARD-CORRECTNESS-001 per-task
threshold map (Epic 2) for jiminy.synthesize capture.

## 5. Implementation Plan
- **Epic 0** — this plan (committed).
- **Epic 1** — hidden.summarize schema `object`→`string` + a one-line note;
  verify benchmark matching still resolves the task.
- **Epic 2** — `explanation_quality` schema-aware (nested reasoning) + unit
  tests (jiminy.evaluate `{violations:[{reasoning}],warnings:[]}` scores high;
  empty `{violations:[],warnings:[]}` still scores reasonably as a valid
  no-violation verdict; flat explanation still works; missing reasoning low).
- **Epic 3** — `specificity_score`/`actionability_score` substantive-floored +
  tests (concise valid guidance ≥ floor; specific/actionable → ~1.0; hedging/
  empty low; `follow_rate` inherits). Re-run the existing reward tests + the
  ape.reflect/consulting.synthesis sanity (no regression to absurd highs).
- **Epic 4 (Tier 3 LIVE)** — re-score REAL production rows for the 4 tasks
  against old vs new grading; confirm correct responses now score correctly
  (hidden.summarize prose no longer invalid-JSON; jiminy.evaluate correct
  verdicts clear; jiminy.synthesize valid guidance recovers).
- **Epic 5** — docs (feature-doc note / CHANGELOG / post), push → PR → CI.

## 6. Testing Plan (3 tiers)
T1: per-function unit tests above (pin new semantics; the existing 78+ reward
tests stay green). T2: `pytest neural/training/tests/`, ULTS lint, UBENCH lint
(schema change), config scanner, go build (Go untouched but verify). T3 (LIVE):
real TSDB rows for the 4 tasks scored through the new functions — correct
responses score correctly; spot-check 5 per task.

## 7. Commit Strategy
Per-epic; ruff each; push once at sprint end; PR summary; CI watch. Live-surprise
fixes get their own commit.

## 8. Verification Checklist
- [ ] hidden.summarize schema is `string`; benchmark matching resolves it
- [ ] explanation_quality credits nested violations[].reasoning; flat still works
- [ ] specificity/actionability: concise valid ≥ floor, hedging/empty low,
      no over-inflation for ape.reflect/consulting.synthesis
- [ ] follow_rate inherits the fix
- [ ] All reward unit tests green; ULTS + UBENCH lint green
- [ ] LIVE: real rows for the 4 tasks score correctly old→new
- [ ] CHANGELOG + post

## 9. Documentation Update — Epic 5 (never cut).

## 10. Risks & Mitigations
Over-flooring keyword-bag erases the quality signal → floor recovers false-drops
only (~0.7), keeps specific>generic ordering + hedging penalty; pair with the
per-task threshold for capture rather than flooring to 0.8. Schema-aware
explanation_quality breaks the flat path → fall back to flat when no nested
structure; unit-test both. hidden.summarize schema change breaks eval matching →
UBENCH lint + a benchmark-match check. Changing shared functions affects other
consumers → explicit no-regression tests for ape.reflect/consulting.synthesis.

## 11. Documents Accessed
The 4 ULTS specs; `reward_functions.py`; `cluster_summarizer.go`; REWARD-
CORRECTNESS-001 `live_findings.md`; live TSDB rows.

## 12. Rollback Procedures
All changes are reward-function code + ULTS spec edits (revertable); no schema
migration, no production behavior change, nothing trained.
