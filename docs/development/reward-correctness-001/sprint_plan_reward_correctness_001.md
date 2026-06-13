# Sprint Plan — REWARD-CORRECTNESS-001: Stop Dropping Terse-Correct Training Data

## 1. Header & Metadata
2026-06-13 · branch `reh3376_dev01` · training-integrity remediation
(2nd sprint; eval-integrity shipped #460) · effort ~4d · risk
medium-high (changes the reward definitions that gate BOTH training-data
inclusion AND the benchmark eval metric; closes with the honest baseline
recompute). No adapter trained.

## 2. Problem Statement
The distill inclusion gate is `kept = mean(reward_vector) >= 0.8`
(global, `scripts/x9_distill_capture_v2.py:360-365`); each task's
`reward_vector` comes from its ULTS `reward_functions`. Many reward
functions are length/keyword-biased and under-credit a SPEC-CORRECT but
terse answer: `coverage_score` caps <20 words at 0.4 / <50 at 0.7
(reward_functions.py:271); `explanation_quality` caps <20 words at 0.6
(:310); `coherence_score` needs ≥2 sentences (:246); `insight_count`
rewards bullet count; `jiminy.synthesize`/`consulting.synthesis` have NO
correctness term. So `summarize.generate`, and `ape.reflect` (the
LARGEST target, 500 rows), drop their terse-correct majority below 0.8 —
and `balanced_sampler` upsamples the SURVIVORS, amplifying the
verbose-skew it can't undo. The FT-CLASSIFY-002 `summary_quality` fix
special-cased ONE pattern (`{type:none}`→1.0); the class is systemic and
unfixed. This is the corpus-skew root cause behind the 3 discarded
retrains (audit: `docs/development/sidecar-loop-001/training_pipeline_correctness_audit.md`).

## 3. Scope & Constraints
**Principle**: inclusion must select for CORRECTNESS, not length — a
spec-correct response clears the gate regardless of verbosity, and
verbosity is not rewarded (fixes BOTH the terse-drop and the
verbose-skew).
**In**: (1) **Length-neutral correctness rewards** — `coverage_score`,
`explanation_quality`, `coherence_score` give a valid non-empty response
a high floor (≥ the inclusion bar) with length only a small BOUNDED
adjustment, and remove the upward verbosity reward (no score gain past a
reasonable length); generalize `summary_quality`'s "spec-correct → high"
beyond the single none-case where a structured-correctness signal exists
(json_valid + schema_match). Functions stay pure + unit-tested. (2)
**Per-task inclusion thresholds** — `x9` gains `--reward-threshold-map`
(JSON, per-task; falls back to the global default) so a task whose
correct answers legitimately score ~0.7 on imperfect heuristics isn't
gutted; thresholds calibrated by the forcing function below. (3)
**Forcing function** (the test the bug would have failed): for each of
the 12 production-covered tasks, a KNOWN-CORRECT golden row (from the
leak-free `valid_clean`) scores ≥ its inclusion threshold — pins
"spec-correct is always included." (4) **Distribution check** — after
the fix, re-run x9 (dry, no OpenAI) over production rows and confirm the
kept-set class distribution matches production (the FT-CLASSIFY-002
goal), not the verbose subset. (5) **Closing step (eval-integrity
deferred)**: GGUF serving in `regression.py` + recompute the honest
baseline through the now-fixed reward + clean eval + GGUF form; replace
the frozen 0.8338 constant with the recomputed report.
**Out**: training an adapter; the DPO pairing fix (separate, when DPO is
next used); the prune phase (operator's separate directive, after this).

## 4. Dependencies
audit + reward-audit findings; `neural/training/reward_functions.py`
(coverage:271/coherence:246/explanation:310/summary_quality:289/
specificity:335/insight_count/actionability/naming/recall/ndcg);
`scripts/x9_distill_capture_v2.py:247,360-365`;
`neural/training/balanced_sampler.py`; ULTS specs' `reward_functions`;
`neural/training/rl/regression.py` (GGUF serving + baseline);
`training_data/eval/valid_clean.jsonl` (forcing-function golden rows).

## 5. Implementation Plan
Epic 0 plan · **Epic 1** length-neutral correctness rewards + unit tests
(per-function: spec-correct terse answer scores high; verbosity capped) ·
**Epic 2** per-task inclusion thresholds in x9 (config map + default) ·
**Epic 3** forcing-function test (each of 12 tasks: a valid_clean golden
row clears its threshold) + distribution check (kept-set matches
production) · **Epic 4** GGUF serving in regression.py + recompute the
honest baseline (live; replaces 0.8338) · **Epic 5** live Tier 3 (run x9
dry over production → terse-correct rows now kept, distribution matches;
baseline report regenerated in GGUF form) · **Epic 6** docs (feature
doc, CHANGELOG, post), push.

## 6. Testing Plan
T1: per-function — a spec-correct terse golden scores ≥ threshold; a
verbose-but-empty/garbage response does NOT score higher than a correct
concise one (verbosity not rewarded); summary_quality generalization. T2:
`pytest neural/` (reward + benchmark + regression suites); the
forcing-function test over all 12 tasks; ruff. T3 (live): x9 dry-run over
production (no OpenAI calls needed for the reward recompute on stored
responses) → kept-set per-task distribution ≈ production (not
verbose-skewed); baseline recomputed via GGUF llama-server :8102 →
fresh `benchmark_qwen3_14b_v1_baseline.json`, frozen constant retired.

## 7. Commit Strategy
Per-epic · ruff each · push once · summary · CI watch.

## 8. Verification Checklist
- [ ] Length-neutral rewards: spec-correct terse answer ≥ threshold (per-fn tests)
- [ ] Verbosity no longer rewarded upward
- [ ] Per-task inclusion thresholds (no single global 0.8)
- [ ] Forcing-function test: 12/12 tasks' golden rows clear their gate
- [ ] Kept-set distribution matches production (no verbose-skew)
- [ ] GGUF serving in regression.py
- [ ] Honest baseline recomputed (frozen 0.8338 retired)
- [ ] Feature doc + CHANGELOG + post

## 9. Documentation Update — Epic 6 (never cut).

## 10. Risks & Mitigations
Changing rewards shifts the eval metric too → that's correct (eval +
inclusion must agree on "correct"); the baseline is recomputed under the
new reward in the SAME sprint so the gate stays apples-to-apples. Over-
crediting garbage when raising floors → floors require VALID structure
(json_valid/schema_match/non-empty), not mere non-emptiness; unit tests
pin that garbage/empty stays low. Per-task thresholds become a new
hardcoding vector → calibrated by the forcing function (golden rows), not
guessed; documented. Baseline recompute is live/heavy → it's the closing
step; provisional until run, then committed as the report.

## 11. Documents Accessed
ROADMAP:64 (sidecar-loop parent); the training-pipeline audit +
reward-audit; eval-integrity post (deferred GGUF+baseline); reward_functions.py
+ x9 gate (read); valid_clean (forcing-function source).

## 12. Rollback Procedures
Reward-function + x9 changes revert via git; per-task threshold map
defaults to the prior global 0.8 when unset. Baseline: the recomputed
report is a new file; the frozen constant can be restored by revert.
No adapter trained.
