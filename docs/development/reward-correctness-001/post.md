# REWARD-CORRECTNESS-001 — Sprint Post

**Date:** 2026-06-13 · branch `reh3376_dev01` · training-integrity remediation
(operator-sequenced after EVAL-INTEGRITY-001).

## What shipped
- **Epic 0** — sprint plan (`sprint_plan_reward_correctness_001.md`).
- **Epic 1** — four length/count-biased reward functions made length-neutral
  (`coverage_score`, `explanation_quality`, `coherence_score`, `insight_count`)
  in `neural/training/reward_functions.py`. A substantive, valid response now
  floors high regardless of length; verbosity/bullet-count is never rewarded
  upward; empty/pure-repetition stays low. Tests rewritten (78 pass).
- **Epic 2** — `--reward-threshold-map` per-task inclusion gate in
  `scripts/x9_distill_capture_v2.py`; recorded per-row + in the manifest.
- **Epic 3 (live Tier 3)** — `live_findings.md`. Validated Epic 1 on the real
  corpus and surfaced the larger correctness issues (see below).

## Live verification (required — real stack)
Scored REAL production `llm_interactions` at the 0.8 distill gate, old vs new
rewards, per ULTS reward array:
- **hidden.summarize: 69/72 real concise summaries recovered** — the old
  length ladder dropped 96% of valid summaries; now kept. The length-bias fix,
  proven on the wire.
- **Epic 2 flag**: live x9 run (real OpenAI + TSDB) with
  `{"consulting.classify": 0.6}` applied the override end-to-end (3/3 captured;
  manifest `per_task.reward_threshold = 0.6`).

## What the live run reframed (the important outcome)
The length bias was real but **not** the dominant suppressor for the biggest
tasks. Per-function means on real rows isolated three larger issues:
1. **ape.reflect (54k, largest target): ~87% truncated** → `json_valid` 0.133.
   Production serving defect (KV-slot overflow), not a reward bug. **Operator
   chose this as the next sprint.**
2. **jiminy.evaluate: `explanation_quality` = 0.0** on correct responses —
   reward/schema mismatch (no top-level explanation key). Scoped follow-up.
3. **jiminy.synthesize: keyword-bag just below gate** — the deferred Epic 1
   continuation (`specificity`/`actionability`/`follow_rate`).

## Deferred / carried forward
- **Epic 4 (GGUF serving in `regression.py` + honest baseline recompute)** —
  held behind the ape.reflect truncation fix. Recomputing over a known-truncated
  corpus would bake in the corruption (same anti-pattern as the EVAL-INTEGRITY
  provisional-baseline deferral). Re-run as the closing step once the corpus is
  sound.
- The reward/schema mismatch (jiminy.evaluate) + keyword-bag continuation
  (jiminy.synthesize) — scoped follow-ups, documented in `live_findings.md`.

## Next
ape.reflect truncation fix (own sprint) — raise the effective output budget so
prompt + reflection output fits the per-slot KV bound, re-capture, verify
`json_valid` recovers on the largest training corpus.

## Documents Accessed
`sprint_plan_reward_correctness_001.md`; `live_findings.md`;
`neural/training/reward_functions.py` + `tests/test_reward_functions.py`;
`scripts/x9_distill_capture_v2.py`; ULTS specs (ape_reflect, jiminy_evaluate,
jiminy_synthesize, hidden_summarize, hidden_name_emergence); the audit doc
(`docs/development/sidecar-loop-001/training_pipeline_correctness_audit.md`);
EVAL-INTEGRITY-001 sprint plan + feature doc.
