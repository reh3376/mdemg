# Adapter Promotion Gate — Trustworthy Eval

**Sprint**: EVAL-INTEGRITY-001 (2026-06-13) · lead sprint of the
training-integrity remediation · **Status**: harness-correctness fixes
shipped; GGUF-serving + baseline-recompute sequenced with the reward sprint.

## Why
A data-correctness audit (triggered by 3 LLM-adapter retrains discarded as
"worse than baseline") found the promotion gate could not tell a good
adapter from a bad one — so those verdicts were uninterpretable. Five
defects in the measurement harness, all file-verified
(`docs/development/sidecar-loop-001/training_pipeline_correctness_audit.md`,
`docs/development/eval-integrity-001/`).

## What shipped (harness correctness)

1. **Leak-free eval set.** The gate evaled on `valid_golden.jsonl` — 99%
   leaked with training data, measuring memorization. Re-pointed to a
   rebuilt **leak-free `valid_clean`** (240 rows / 12 tasks), config +
   holdout SHAs re-pinned. `make test-eval-leak` re-verifies disjointness
   against the SFT sources every run (the script existed with zero callers);
   verified CLEAN, 0/240 overlap.

2. **Dynamic-prompt matching.** The eval builder + benchmark matcher
   strict-hash-matched any spec with a pinned `system_prompt_hash`,
   ignoring `dynamic_prompt`. `ape.reflect`'s prompt is built by
   interpolating the `AllowedLLMActions` enum, so its hash drifts on every
   action change — its **71,033 production rows were invisible** to a filter
   pinned to a stale hash (the single largest training target's entire
   corpus, excluded from eval AND curation). `Spec` now carries
   `dynamic_prompt`; dynamic/enum-templated prompts match by `task_name`.
   Coverage 10 → 12 tasks.

3. **Zero-call hard-fail.** A run with zero successful calls (or no task
   contributing a score) reported aggregate **0.0 — indistinguishable from
   a genuine low score** (the false-0.0 class behind FT-CLASSIFY-002's four
   silent failures). The runner now flags `status=error` + exits nonzero so
   the gate/operator rejects a broken run instead of mistaking it for a
   model judgment.

4. **Capture-gap fix.** `summarize.generate` (default-on) runs in the
   `mdemg ingest` subprocess, where the LLM recorder was never wired (only
   `mdemg serve` wired it) — every summary call was made and silently
   dropped. The recorder is now wired in the ingest process (flush-on-exit),
   so its production corpus accumulates (a future 13th coverable task). The
   other 4 zero-row tasks are correctly gated-off (expected zeros).

## Sequenced with the reward sprint (closing steps)
Two fixes are deliberately deferred to land WITH the reward-correctness
sprint, because they only matter when there's a candidate to judge and a
fixed reward to judge it under:

- **GGUF serving** — `regression.py` spawns the MLX side-server (which
  OOM-crashed mid-sweep); the promotion form is GGUF via llama-server :8102.
  The change is only exercised during a retrain.
- **Baseline recompute** — the baseline is a frozen constant (`0.8338`)
  computed under the OLD reward + MLX form. It must be recomputed through
  the fixed harness — but an *honest* baseline requires the *fixed reward*,
  which is the next sprint. So the baseline recompute is that sprint's
  closing step (a `provisional` baseline before then would just bake in the
  current reward bug).

## How to use
- Before any promotion/benchmark: `make test-eval-leak` (gate aborts on leak).
- Rebuild the clean eval as the corpus grows: `python scripts/build_clean_eval.py`
  then re-pin the UBENCH SHAs (the runner asserts them).
- A degenerate benchmark run now exits nonzero — treat `status=error` as
  "not a valid judgment," never as a low score.
