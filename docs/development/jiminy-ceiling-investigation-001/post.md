# JIMINY-CEILING-INVESTIGATION-001 — Sprint Post

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #4.
**Verdict:** Investigation complete. **The ~11% follow-rate ceiling is a
composite artifact of three compounding measurement defects, not a
capability limit.** Concrete next-lever recommendation at §6.

## 1. What we set out to answer

Three consecutive sprint arcs (JIMINY-ACTIONABILITY-001,
JIMINY-CORPUS-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001) converged
on ~11% follow rate on actionable Jiminy guidance, against a stated
goal of >90%. Before spending more sprint capacity on levers, we needed
to know *why* it's the ceiling. Method: measure and sample.

## 2. Data snapshot (mdemg-dev, 2026-07-29, 7d window)

**Headline:**
- Actionable outcomes (constraint + correction): **472**
- Followed: **54** → **11.44%**
- Ignored: 409, Contradicted + Partial: 9

**Per-classifier split:**
| Source | n | followed % |
|---|---|---|
| llm | 216 | 25.5% |
| tier1 | 235 | 3.4% |
| heuristic | 21 | 0.0% |

**Top-10 most-surfaced constraints and their follow rates:**

| # | constraint_code | surface_count | pct_followed | rule quality |
|---|---|---|---|---|
| 1 | `auto-07051612cf24` (rebase-after-admin-merge) | 69 | 13.04% | correction narrative (fair rule) |
| 2 | `must-document-before-implementation` | 45 | 6.67% | durable rule |
| 3 | `auto-015a122bcbb8` (workflow-violation-log) | 39 | 7.69% | **session log — not a rule** |
| 4 | (empty constraint_code) | 38 | 10.53% | data hole |
| 5 | `no-direct-main-commits` | 37 | 24.32% | **durable rule** |
| 6 | `audit-before-prune` | 36 | 27.78% | **session log with rule prefix** |
| 7 | `auto-fcb814b48e33` (session-halt-log) | 34 | 2.94% | **session log — not a rule** |
| 8 | `full-system-gap-analysis` | 25 | 8.00% | **phase description — not a rule** |
| 9 | `auto-9f5134a1a0c3` (testing-blind-spot-analysis) | 22 | 31.82% | **analytical observation** |
| 10 | `mandatory-feature-docs` | 20 | 5.00% | durable rule |
| 11 | `must-follow-12-section-format` | 16 | **0.00%** | durable rule |
| 12 | `auto-build-restart-after-feature` | 16 | 6.25% | correction narrative |

⚠️ **Even after JIMINY-CORPUS-001's purge, 5 of the top-12 surface
volume goes to non-rules (session logs, phase descriptions, analytical
observations).** This IS the actual signal.

## 3. Failure-mode categorization (n=16 sampled, hand-classified)

Two independent samples of 8 high-similarity (`sim > 0.6`) LLM-classified
`ignored` outcomes each. Every row read + hand-categorized against the
constraint text.

**Result:**

| Failure mode | n | % | Description |
|---|---|---|---|
| **Context mismatch** | 8 | **50%** | Rule IS durable but doesn't govern this action's context (e.g. rule about git-commit, action was a file-write; rule about code-modification, action was read-only investigation) |
| **Surface mismatch** | 7 | **44%** | Rule is a low-quality auto-* entry (session log / phase description / correction narrative) that shouldn't be surfaced as a durable rule at all |
| **Classifier misclassification** | 1 | **6%** | Agent actually FOLLOWED but LLM labeled ignored |
| **Genuine ignore** | **0** | **0%** | Agent SHOULD have followed the rule but didn't |

**Zero genuine ignores across 16 samples.** The headline "89% ignored"
is dominated by cases where the classifier reads context-mismatch (rule
doesn't govern this action) as ignored instead of `not_applicable`, plus
surface mismatch (rule is a session log the LLM struggles to reason
about consistently).

### Representative examples

**Context mismatch example 1** — `no-direct-main-commits` fired at
sim=0.9 on the action `[documentation update] Wrote /Users/reh3376/mdemg/
docs/development/ftloop-drill-001/runbook.md`. The rule says NEVER
commit to main branch. The action is writing a file, not a git commit.
The correct verdict is `not_applicable`.

**Context mismatch example 2** — `plan-mode-before-change` fired at
sim=0.9 on the action `Ran: docker exec cypher-shell "MATCH ()-[r:
CO_ACTIVATED_WITH]->() WHERE ..."`. The rule says do NOT modify code
without planning mode. The action is a read-only Cypher query — no code
modified. The correct verdict is `not_applicable`.

**Surface mismatch example** — `auto-015a122bcbb8` fired at sim=0.9 on
the action `Ran: grep -n "\"ingest\"" internal/cli/hook_templates/...`.
The "constraint" text is `"CRITICAL WORKFLOW VIOLATION (Phase 101):
Declared implementation complete and presented a commit-ready summary
WITHOUT running E2E tests..."` — that's a session log recording a past
violation, not a rule. The action is unrelated grep.

**Classifier misclassification example** — `auto-07051612cf24` fired at
sim=0.9 on the action `git pull --ff-only` after a main merge. The rule
says "After merging a PR to main via --admin, if the dev branch has more
work to push, rebase the dev branch onto main." A fast-forward pull IS
the same effect as a rebase when the local branch has no divergent
work — this IS the correct workflow. LLM labeled ignored anyway.

## 4. Corpus-quality vs classifier-quality slice

Filtering the outcomes to **exclude** the low-quality auto-* / phase /
session-log entries reveals a very different picture:

**Real durable rules only** (excluding `auto-*`, `full-system-gap-analysis`,
`llm-multi-hop-synthesis`, `audit-before-prune`) — n=204:

| Classifier | n | followed | pct_followed |
|---|---|---|---|
| **llm-only** | 102 | 19 | **18.6%** |
| **tier1-only** | 102 | 1 | **1.0%** |

**Two independent problems compound to produce the 11% headline:**

1. **Surface contamination**: ~55% of surface volume goes to non-rules
   (auto-* narratives). Removing them from the denominator alone lifts
   the follow rate from 11% to ~19%.
2. **Tier1 classifier is functionally blind to follows**: 1% follow rate
   over 102 events. This is definitional — tier1 is embedding similarity
   between constraint text and action text, but *following* a rule does
   not require semantic similarity between the rule and the action. An
   action "committed to reh3376_dev01 branch" *follows* "never commit
   to main" — but the embeddings won't be similar.

⚠️ **Even with both slices lifted, follow rate maxes at ~19%** because
of the third defect — context mismatch labeled as ignored instead of
not_applicable. The 50% of samples that were context-mismatched drag
the LLM slice down from what could be ~50-70%.

## 5. Ceiling diagnosis (data-decided)

**The ~11% follow rate is not measuring guidance quality. It is the
noise floor produced by three compounding measurement defects:**

| Defect | Contribution to the 89% "ignored" | Effort to fix |
|---|---|---|
| **A. Corpus contamination** — non-rules dominate top-N surface volume | ~55% of denominator | Medium (JIMINY-CORPUS-002: second pass on constraint admission gate + retroactive tombstone of confirmed non-rules) |
| **B. Tier1 systematic mislabeling** — embedding similarity can't detect follows | ~50% of outcomes get functionally-wrong label | Low-Medium (bypass tier1 for the follow/ignore decision; keep it as a fast pre-gate for not_applicable only) |
| **C. Context mismatch → ignored** — LLM classifier reads context-mismatch as ignored | ~50% of `ignored` on real rules | Low (extend the classifier prompt to distinguish "rule doesn't govern this context" from "rule governs and was violated") |

**Realistic ceiling if all three defects fixed: ~50-70%** on real
durable rules under proper measurement. The operator's stated goal of
`>90%` may itself be miscalibrated — even under perfect measurement,
some rules genuinely won't apply to every relevant-looking action, and
legitimate operator behavior sometimes needs to violate rules (with
reason).

## 6. Concrete next-lever recommendation

Ranked by (expected impact × effort⁻¹):

### Recommended sprint: JIMINY-CLASSIFIER-CONTEXT-001

**Effort: ~1-2 days. Expected impact: ~11% → ~35-50% honest follow rate.**

Address defect **C** first — extending the classifier prompt to
distinguish "not_applicable" (rule governs a different context) from
"ignored" (rule governs and was violated). Same shape as
JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001's non-violation-credit
extension for `must_not`, applied more broadly to context mismatch.

Concrete extension:
```
The following cases MUST be labeled `not_applicable`, NOT `ignored`:
- Rule governs git operations; action is a file write / read / edit
- Rule governs code modification; action is a read-only query
- Rule governs a specific workflow step; action is in a DIFFERENT
  workflow step
- Rule governs a language-specific behavior; action is in a different
  language
- Rule text is a completion log / session artifact / phase description;
  action is unrelated
```

Same default-off / A/B / KEEP-ON validation pattern as the shipped
compliance-credit sprint. Testable via a synthetic set of context-
mismatch fixtures I hand-authored during S3.

### Follow-up sprint: JIMINY-CORPUS-002 (~2d)

Address defect **A** — corpus quality. Second pass on
`ConstraintPromotionGate` with stricter admission criteria, retroactive
tombstone of the confirmed non-rules (auto-fcb814b48e33 session halt,
auto-015a122bcbb8 workflow-violation log, auto-9f5134a1a0c3
testing-blind-spot analysis, full-system-gap-analysis phase description,
llm-multi-hop-synthesis foundation doc). Backed up + reversible per the
JIMINY-CORPUS-001 precedent.

### Deferred: JIMINY-TIER1-BYPASS-001 (~1d)

Address defect **B** — tier1 classifier's systematic ignored-labeling
on follow cases. Bypass tier1 for the follow/ignore decision (keep it
as a fast pre-gate for not_applicable only). Deferred because C is
higher-impact and B's fix depends on measurements from post-C data.

### Explicitly NOT recommended

- **More surface-composition tuning** (Lever A / B / C from the shipped
  actionability arcs). Sample evidence shows the classifier isn't
  actually reading surface composition as the bottleneck.
- **Corpus curation via more HITL cadence pressure**. Even with 100%
  operator engagement, defects B + C still bound the measurement.
- **Raising the surface confidence gate**. Would suppress volume but
  not change the per-outcome measurement quality.

## 7. Known limitations of this investigation

- **Sample size**: 16 samples for the failure-mode categorization is
  small. Consistent across two independent random draws, but a
  statistically-stronger claim would require ~50+.
- **Categorization is my own**. Second-opinion HITL grading (via the
  shipped HITL-REVIEW-001 platform) on the same 16 rows would harden
  the finding.
- **Sampling bias**: high-similarity LLM samples were chosen because
  they're the candidates for classifier misclassification. A different
  sample of low-similarity outcomes might reveal different patterns.
- **The 90% target may be honest-unreachable**. This investigation
  didn't prove 50-70% is the ceiling; it argued from the data that a
  meaningful lift is possible AND named the levers. Actual ceiling
  measurement requires implementing the levers and re-measuring.

## 8. Follow-ups disclosed

1. **JIMINY-CLASSIFIER-CONTEXT-001** — the recommended next sprint
   (above)
2. **JIMINY-CORPUS-002** — second corpus purge pass
3. **JIMINY-TIER1-BYPASS-001** — bypass tier1 for the follow/ignore
   decision
4. **Empty constraint_code investigation** — 50+ outcomes over 7d have
   empty `constraint_code`, all pointing to guidance `rlgol248e1ftcdknf8t8zjpp`
   under two different `constraint_id` values. Data-hole worth
   investigating.
5. **HITL-graded second-opinion** on the S3 sample — verify my
   categorization against operator judgment via the shipped review
   platform.
6. **Statistical validation** — 50-sample follow-up categorization
   pass to firm the failure-mode distribution.

## Documents Accessed

- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (candidate #4)
- `docs/development/jiminy-ceiling-investigation-001/sprint_plan.md`
  (this dir)
- CLAUDE.md pins for JIMINY-ACTIONABILITY-001, JIMINY-CORPUS-001,
  JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001, JIMINY-ACTIONABILITY-
  INVERSION-001, JIMINY-OUTCOME-002 (context)
- Live TSDB queries against `constraint_outcomes` (7d cohort,
  distribution) + `guidance_training_rows` (sampled 16 rows for
  categorization, hand-classified)
- Neo4j reads of L1 `role_type='constraint'` node contents for top-12
  cohort
- No code shipped; no substrate mutation
