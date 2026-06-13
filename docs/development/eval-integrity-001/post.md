# Sprint Post — EVAL-INTEGRITY-001

2026-06-13 · `reh3376_dev01` · lead sprint of the training-integrity
remediation. Plan + recon + the parent audit in this dir / sibling.

## Shipped (harness correctness — committed)
- **Epic 1 dynamic_prompt** (`run_benchmark.Spec` + matcher + build_clean_eval
  + ape_reflect spec): enum-templated prompts match by task_name. Recovered
  ape.reflect's 71,033 rows; coverage 10→12 tasks. 90 neural tests pass.
- **Gate-set**: re-pointed benchmark_phase10.yaml + UBENCH spec from the
  99%-leaked valid_golden to the leak-free valid_clean (240/12); config +
  holdout SHAs re-pinned; ubench lint 100%.
- **Epic 3 zero-call hard-fail** (run_benchmark): degenerate runs →
  status=error + nonzero exit. Pin tests.
- **Capture-gap fix** (ingest.go): wired the LLM recorder in the ingest
  process so summarize.generate is captured (was silently dropped — the
  recorder-gap class via a process boundary).
- **Epic 2 leak-audit gate** (`make test-eval-leak`): re-verifies eval↔train
  disjointness; valid_clean CLEAN 0/240.

## Capture investigation result (operator-approved parallel lane)
5 zero-row tasks: 4 correctly GATED-OFF (consulting.synthesis,
guardrail.evaluate, metalearn.generalize, retrieval.rerank_nli — flags
default-false; expected zeros), 1 real recorder-gap (summarize.generate —
fixed above).

## Deferred to the reward sprint (closing steps, by design)
- **GGUF serving** in regression.py (off MLX side-server) — only exercised
  during a retrain.
- **Baseline recompute** — an honest baseline needs the FIXED reward; doing
  it now (current reward) would bake in the bug. It's the reward sprint's
  closing step.

## Next in the remediation
Reward-correctness sprint (per-task inclusion thresholds + correctness
terms so terse-correct answers aren't dropped) → then recompute the honest
baseline through this fixed harness in GGUF form → then the prune phase
(operator directive: purge non-conforming data once "conforming" is defined).

## Verification
clean eval 12-task leak-free (0/240); dynamic_prompt recovers ape.reflect
(1000 rows); zero-call guard pinned; recorder wired in ingest (build+lint
green); ubench lint 100%; config scanner green.
