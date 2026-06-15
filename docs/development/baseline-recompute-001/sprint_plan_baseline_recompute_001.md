# Sprint Plan — BASELINE-RECOMPUTE-001: Honest Baseline Through the Fixed Harness

## 1. Header & Metadata
2026-06-15 · branch `reh3376_dev01` · training-integrity remediation (the
capstone — deferred from EVAL-INTEGRITY-001 + REWARD-CORRECTNESS-001/002 until
the corpora + rewards were sound) · effort ~0.5d (mostly a benchmark run) ·
risk **low** (re-measures the baseline; no model trained, no production change).

## 2. Problem Statement
The adapter-promotion gate compares candidates against a **frozen constant
baseline 0.8338** (`regression.py::phase5_baseline_aggregate`,
`rl_phase11.yaml`) computed under the OLD reward functions + MLX serving + the
99%-leaked `valid_golden` eval. Every input to that number has since been
fixed: leak-free `valid_clean` (EVAL-INTEGRITY-001), length-neutral +
schema-correct rewards (REWARD-CORRECTNESS-001/002), GGUF serving on
llama-server :8102 (Phase 13.5), and the corpus pruned of corrupt rows
(DATAPRUNE). So 0.8338 is stale and uninterpretable as a gate target. Recompute
it honestly through the fixed harness so future retrains are judged against a
real number.

## 3. Scope & Constraints
**In:** (1) fix `rl_phase11.yaml mlx_port 8101→8102` (stale GGUF-serving wiring
flagged by EVAL-INTEGRITY-001). (2) Run `run_benchmark.py` against the live
GGUF :8102 on `valid_clean` (12 tasks) with the fixed deterministic rewards
(judge OFF) → fresh baseline report. (3) Promote it to
`benchmark_qwen3_14b_v1_baseline.json` (back up the old). (4) Update
`regression.py` + `rl_phase11.yaml` `phase5_baseline_aggregate` to the
recomputed value, marked recomputed (and prefer deriving from the report). **Out:**
training/promoting any adapter; changing reward functions or the eval set;
the regression-harness server-spawn refactor (regression.py only *compares*
reports — serving is run_benchmark's job, already on :8102). **Constraint:** the
benchmark is the live system (real GGUF :8102, real eval, real I/O observed in
the report) — this run IS the Tier 3 evidence.

## 4. Dependencies
`configs/{benchmark_phase10,rl_phase11}.yaml`; `neural/benchmarks/run_benchmark.py`;
`neural/training/rl/regression.py`; `training_data/eval/valid_clean.jsonl` +
`benchmark_qwen3_14b_v1_baseline.json`; live llama-server :8102 serving
`mdemg-llm-v1.Q5_K_M.gguf`; the fixed `reward_functions.py`.

## 5. Implementation Plan
- **Epic 0** — this plan (committed).
- **Epic 1** — fix `rl_phase11.yaml mlx_port 8101→8102`.
- **Epic 2 (recompute = live Tier 3)** — back up the old baseline report; run
  `run_benchmark.py --config benchmark_phase10.yaml --out <fresh>` against :8102
  (all rows, judge off); confirm served via :8102 (report `mlx_base_url`),
  status not error, no zero-call tasks. Record the fresh aggregate + per-task.
- **Epic 3** — promote the fresh report to
  `benchmark_qwen3_14b_v1_baseline.json`; set `phase5_baseline_aggregate` (both
  configs) to the recomputed value; add a comment noting it's the
  fixed-harness recompute (date, eval, serving). Re-pin the UBENCH baseline SHA
  if the runner asserts it.
- **Epic 4** — docs (feature-doc note on the promotion gate, CHANGELOG, post),
  push → PR → CI.

## 6. Testing Plan (3 tiers)
T1: regression.py still loads + gate math uses the new aggregate (unit/quick
import check). T2: `pytest neural/training/rl/tests/` (regression harness),
`pytest neural/benchmarks/tests/`, UBENCH lint (SHA), config scanner. T3
(LIVE): the benchmark run itself — fresh report written from the real GGUF
:8102, aggregate recorded, zero-call hard-fail not triggered, served-via-:8102
confirmed in the report; spot-check per-task scores are sane (no all-zeros, no
absurd inflation from the reward fixes).

## 7. Commit Strategy
Per-epic; push once at sprint end; PR summary; CI watch. The fresh baseline
report + the old backup are data files (gitignored training_data/eval — the
aggregate value lands in the committed configs + post).

## 8. Verification Checklist
- [ ] rl_phase11.yaml mlx_port = 8102
- [ ] Fresh baseline report written from GGUF :8102 on valid_clean, judge off
- [ ] No zero-call tasks / status != error; served-via-:8102 confirmed
- [ ] phase5_baseline_aggregate updated in regression.py + rl_phase11.yaml,
      marked recomputed; old value + report backed up
- [ ] Regression + benchmark pytest green; UBENCH lint green
- [ ] CHANGELOG + post with the old→new aggregate + per-task delta

## 9. Documentation Update — Epic 4 (never cut).

## 10. Risks & Mitigations
The recompute diverges sharply from 0.8338 (different eval + rewards) → that is
EXPECTED and the point; document old→new with the reasons. A reward fix
over-inflates a task → the per-task table is inspected (REWARD-CORRECTNESS-002
already live-checked no over-inflation). llama-server slow on long prompts →
`--mlx-timeout-s 300`. The number must not be treated as comparable to the old
0.8338 (apples to oranges) → label it clearly as the new fixed-harness baseline;
future retrains compare against IT, not the historical constant.

## 11. Documents Accessed
The two configs; run_benchmark.py + regression.py; EVAL-INTEGRITY-001 +
REWARD-CORRECTNESS-001/002 sprints; the current baseline report; live :8102.

## 12. Rollback Procedures
Restore the backed-up `benchmark_qwen3_14b_v1_baseline.json` + revert the
config aggregate values. No model/schema/production change.
