# JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 — Sprint Post

**Shipped:** 2026-07-21 | **Branch:** `reh3376_dev01` | **PR:** (pending push)

## What shipped

Implements the JIMINY-ACTIONABILITY-INVERSION-001 fix_spec.md's **Fix 2 (highest leverage / lowest risk)** — a "non-violation credit for must_not" clause in the tier-2 LLM classifier prompt that routes unrelated-context `ignored` verdicts to `not_applicable` (which the shipped writer gate already filters from `constraint_outcomes`). Also bundles **Fix 1** (Actionable Compliance Rate panel wording + alert threshold recal).

Ships behind default-off config `JIMINY_NONVIOLATION_CREDIT_ENABLED`. Operator runs the 3-day A/B recipe before flipping.

## Epics (all committed)

- **E0** — Sprint plan (`66f1853`)
- **E1** — Baseline stats (`30fefe7`): actionable follow rate 10.20% (46/451), 83 not_applicable / 503 ignored / 88 followed 7d LLM classifications
- **E2+E3+E4** — Prompt gate + tier-1 tests + ULTS pin verified (`57adfe7`): OutcomeClassifier gains `nonViolationCredit` field + `resolveClassifySystemPrompt()` method; 3 unit tests pin behavior; ULTS `--verify-hashes` PASSES (default-off = HEAD byte-identical)
- **E5** — Live Tier-3 A/B (`5ac7e0b`): direct-LLM against llama-server temperature 0.0; clear-cut cases both prompts agree; discriminative borderline case correctly shifts from `ignored` → `not_applicable` under the new prompt
- **E6** — Panel wording + alert recal (`af6997a`): "Should-Follow Follow Rate" → "Actionable Compliance Rate"; alert threshold 0.5 → 0.15
- **E7** — Operator A/B recipe (`41e8656`): pre/post SQL, flip options, tripwires, rollback
- **E8** — Canonical docs (this commit)

## Key evidence

**Baseline (7d, mdemg-dev, before ship):**
- Actionable follow rate: 10.20% (46/451)
- LLM classifier: 88 followed / 503 ignored / **83 not_applicable** / 8 contradicted / 701 total

**Discriminative A/B (E5):**

| Case | OLD prompt | NEW prompt |
|---|---|---|
| Unrelated action + must_not constraint | `ignored` | **`not_applicable`** |

Same underlying reasoning ("didn't touch the mechanism"), different classification. The shipped writer gate filters `not_applicable` from `constraint_outcomes`, so the routed row correctly stops inflating the actionable denominator.

**Predicted post-flip lift**: constraint follow rate 10% → ~20% (if ~50% of `ignored` shifts). Requires operator's 3-day A/B to pin the real magnitude.

## Two design choices worth pinning

1. **Default-off render is byte-identical to the historical const.** This preserves the ULTS-CI-001 `system_prompt_hash` pin without a hash bump. The flag flips runtime-rendered prompt, not the const declaration. Precedent for any future config-gated prompt extension.
2. **Alert threshold recalibrated to match honest baseline.** The 0.5 default was inherited from a pre-INVERSION-001 understanding of the metric; it caused chronic alerting on by-design steady-state. New 0.15 default still catches a genuine collapse (baseline 0.10 → alert fires if it drops below 0.15 for the eval window).

## What operators still need to do

1. Read `docs/development/jiminy-actionability-compliance-credit-001/ab_recipe.md`.
2. Capture pre-flip baseline SQL (Step 1).
3. Flip `JIMINY_NONVIOLATION_CREDIT_ENABLED=true` in `.env` + restart (Step 2).
4. Wait ≥3 days, use MDEMG normally (Step 3).
5. Run post-flip SQL (Step 4).
6. Interpret against the tripwires (Step 5). Keep or revert.

## What I did NOT do

- No default-flip. That's operator-time per the "flag flipped only after live smoke" contract.
- No changes to `constraint_outcomes` schema or the `not_applicable` gate at `service.go:1730,1762`. Those stay correct.
- No changes to Lever C top-K. Per fix_spec.md, Fix 2 achieves the same denominator reduction without cutting coverage.
- No JIMINY-CORPUS-002 (the deeper corpus-quality follow-up remains a separate arc).

## Rollback

- **Data**: N/A. Flag doesn't backfill. Rows classified under flag-on stay classified as they were.
- **Code**: revert per-commit; the flag itself can be disabled via `.env` without any code change.
- **Config**: `JIMINY_NONVIOLATION_CREDIT_ENABLED=false` (default) is the rollback state.
- **Panel wording**: revert the mdemg-jiminy.json + rules.go + config.go E6 edits (single-panel + single-title + single-default rename).

## Deviations from plan

- E2/E3/E4 combined into one commit (same file cluster, tightly coupled — same pattern used in DASHBOARD-TRUTH-002).
- E5's warm-then-fetch produced 0 results on the freshly-restarted server; pivoted to direct-LLM A/B via `/v1/chat/completions` (equivalent proof at the classifier layer without depending on retrieval warm-up). Documented in live_verification.md.

## Sweep + follow-up sprints — status

The DASHBOARD-TRUTH-002 sweep is complete (5 sprints shipped + this one as the follow-up spec'd during sprint 3). All 14 operator-flagged concerns resolved OR spec'd-and-shipped. Remaining forward-looking work:

- **Deeper corpus-quality lift** (JIMINY-RELEVANCE-001 arc / HITL curation) — the operator's next optional decision point.
- **Doc-hygiene stale references** in `ui-gap-analysis.md` + `j17-feedback-loop-closure.md` (DOC-CURRENCY-001 line).
- **`--persist-tsdb` auto-apply for the benchmark runner** (FT-RECURSIVE-002 follow-up disclosed by FT-BENCH-REFRESH-001).
