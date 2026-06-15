# BASELINE-RECOMPUTE-001 — Sprint Post

**Date:** 2026-06-15 · branch `reh3376_dev01` · the capstone of the
training-integrity remediation (deferred from EVAL-INTEGRITY-001 +
REWARD-CORRECTNESS-001/002 until the corpora + rewards were sound).

## Outcome
The adapter-promotion gate's baseline is no longer a stale frozen constant.
Recomputed through the **fixed harness** (leak-free `valid_clean` eval +
RC-001/002 corrected rewards + GGUF llama-server :8102), the honest baseline is
**0.8655**, replacing the frozen **0.8338**.

**The two numbers are NOT comparable** (apples-to-oranges): 0.8338 was computed
on the 99%-leaked `valid_golden` eval, under the old length-biased/mismatched
rewards, served via decommissioned `mlx_lm.server`. 0.8655 is the model's true
quality under honest grading. The +0.03 is *not* "the model improved" — it's
"honest grading scores it 0.8655." Future retrains compare against **0.8655**,
not the historical 0.8338.

## What shipped
- **Epic 1** — `rl_phase11.yaml mlx_port 8101→8102` (stale GGUF-serving wiring,
  flagged by EVAL-INTEGRITY-001).
- **Epic 2 (recompute = live Tier 3)** — `run_benchmark.py --config
  benchmark_phase10.yaml --rows-per-spec 10` against the live GGUF :8102, judge
  off (deterministic rewards). `status: ok`, `mlx_base_url:
  http://127.0.0.1:8102/v1`, 12 tasks, **50 samples/task**, 0 zero-call tasks.
- **Epic 3** — `evaluate_gate_5a` now **derives the baseline aggregate from the
  loaded baseline REPORT** (single source of truth), not the frozen constant;
  the `phase5_baseline_aggregate` constant (0.8338 → **0.8655**) is retained
  only as a >5pp drift tripwire (warns). `regression.py` docstring +
  `rl_phase11.yaml` aggregate/target updated. Old baseline report backed up to
  `.mdemg-backup-20260613_195431/benchmark_qwen3_14b_v1_baseline.PRE-RECOMPUTE.json`;
  the recomputed report promoted to `benchmark_qwen3_14b_v1_baseline.json`.

## Recomputed per-task (fixed harness, GGUF :8102, 50 samples/task)
| task | mean | | task | mean |
|------|-----:|-|------|-----:|
| ape.reflect | 0.696 | | retrieval.rerank_cross | 0.900 |
| jiminy.evaluate_llm | 0.800 | | consulting.classify | 0.910 |
| retrieval.query_classify | 0.820 | | hidden.reclassify | 0.920 |
| retrieval.intent_translate | 0.874 | | hidden.name_emergence | 0.950 |
| jiminy.synthesize | 0.880 | | jiminy.evaluate | 0.967 |
| hidden.summarize | 0.900 | | jiminy.codegen | 1.000 |
| | | | **aggregate** | **0.8655** |

**Note — ape.reflect (0.696) is dragged by `json_valid=0.24`, an eval-harness
artifact, not a model regression:** the benchmark sends the *stored* historical
ape.reflect prompts (~7.5k tokens) straight to :8102, bypassing the runtime
prompt-budget enforcement (APE-PROMPT-BUDGET-001 bounds the prompt in the live
server's `buildUserPrompt`, not in the benchmark's direct call), so they still
truncate at the 8192-token KV slot. The fix REWARD-CORRECTNESS-002/APE-PROMPT-
BUDGET-001 verified live (fresh ape.reflect 100% valid); the eval just replays
pre-fix-length prompts. A follow-up could bound the eval's ape.reflect prompts.

## Notes / carried forward
- The recompute used `--rows-per-spec 10` (50 samples/task) after the full
  240-row × n_runs-5 run proved impractically slow under single-threaded
  client + long-output generations (killed at ~1h11m). 50 samples/task is a
  statistically solid per-task mean for the gate target; a full-corpus recompute
  can refine it off-peak.
- **Disclosed follow-up:** `run_benchmark.py` is single-threaded against a
  4-slot llama-server (3 slots idle). Client-side concurrency would cut wall-time
  ~4× — a worthwhile small follow-up sprint.

## Documents Accessed
`configs/{benchmark_phase10,rl_phase11}.yaml`; `neural/benchmarks/run_benchmark.py`;
`neural/training/rl/regression.py` + tests; the recomputed report + the backed-up
frozen baseline; EVAL-INTEGRITY-001 + REWARD-CORRECTNESS-001/002 sprints; live
llama-server :8102.
