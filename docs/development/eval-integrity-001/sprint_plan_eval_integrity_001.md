# Sprint Plan — EVAL-INTEGRITY-001: Make the Adapter Promotion Gate Trustworthy

## 1. Header & Metadata
2026-06-13 · branch `reh3376_dev01` · training-integrity remediation
(lead sprint; operator-sequenced after the data-correctness audit) ·
effort ~3.5d · risk medium (no model trained; changes the measurement
harness that gates every future retrain — getting it WRONG silently
mis-judges adapters, which is the very failure being fixed).

## 2. Problem Statement
The audit (`docs/development/sidecar-loop-001/training_pipeline_correctness_audit.md`)
found the promotion gate cannot tell a good adapter from a bad one — the
mechanism behind 3 retrains discarded as "worse than baseline" that were
likely mis-judged. Verified at HEAD: the gate evals on the **99%-leaked
`valid_golden.jsonl`** (measures memorization); compares against a
**frozen constant baseline 0.8338** computed under the OLD reward + MLX
serving form; serves candidates via the **MLX side-server** (which
OOM-crashed mid-sweep), not the GGUF promotion form; **never runs the
leak audit**; and reports **aggregate 0.0 (not an error)** when a run
makes zero successful calls (4 false-0.0s in one sprint). This sprint
fixes the MEASUREMENT HARNESS so a promote/reject verdict is
interpretable. (The reward-corpus skew is the NEXT sprint; the honest
baseline NUMBER is re-established at the close of that one — see §3.)

## 3. Scope & Constraints
**In**: (1) **Clean eval set** — rebuild a leak-free holdout via
`scripts/build_clean_eval.py` covering the gate's 17-task contract
(valid_clean is stale 30-Apr / 319-row / 16-task — wrong shape);
re-point `benchmark_phase10.yaml golden_holdout.out_path` and the ubench
spec at it; **re-pin `mdemg.ubench.json` sha256 + expected_rows +
expected_tasks in the same PR** (the runner asserts them). (2)
**Leak-audit preflight** — `audit_eval_leakage.py` runs IN the gate
(against the SFT train/valid splits) and aborts before any model call on
nonzero exit; add a `make test-eval-leak` target. (3) **GGUF serving** —
`regression.py` serves candidates via llama-server :8102 GGUF (the
promotion form), not `mlx_lm server`; fix `rl_phase11.yaml mlx_port
8101→8102`; reuse the dense→GGUF / adapter→GGUF fuse helpers
(`quantize_deploy.py`, MODEL-DIST-002 scripts). (4) **Zero-call
hard-fail** — `run_benchmark.py`/`variance.py` ERROR (not 0.0) when any
task has zero successful calls or `weight_used==0`. (5) **Baseline as
recomputed report, not frozen constant** — add a `recompute-baseline`
path that runs the baseline model (`.local-models/qwen3-14b-mdemg-v1`,
Phase 5 dense) through the SAME clean-eval + GGUF harness and writes a
fresh `benchmark_qwen3_14b_v1_baseline.json`; `regression.py` derives the
target from the report (assert constant ≈ report agg, or drop the
constant). **Out**: the reward-function correctness fix (next sprint);
TRAINING any adapter; the DPO pairing fix (deferred). **Sequencing
constraint**: the recomputed baseline uses the CURRENT (still-buggy)
reward — so this sprint stamps the harness + a *provisional* baseline and
the reward sprint re-runs the baseline as its closing step. Documented
loudly so no one treats the provisional number as final.

## 4. Dependencies
Recon (this dir); audit doc; `configs/{benchmark_phase10,rl_phase11}.yaml`;
`docs/tests/ubench/specs/mdemg.ubench.json` + `ubench_runner.py`;
`neural/benchmarks/run_benchmark.py` + `variance.py`;
`neural/training/rl/regression.py`; `scripts/{build_clean_eval,
audit_eval_leakage}.py`; `neural/training/quantize_deploy.py`; llama-server
:8102.

## 5. Implementation Plan
Epic 0 plan+recon · **Epic 1** clean eval rebuild (17-task, leak-checked
at build) + re-point configs + ubench SHA re-pin + lint · **Epic 2**
leak-audit preflight (gate abort + make target + CI lint-safe wiring) ·
**Epic 3** zero-call hard-fail (run_benchmark + variance; pin tests) ·
**Epic 4** GGUF serving in regression.py + port fix + endpoint knobs ·
**Epic 5** baseline-recompute path + derive target from report · **Epic
6** live Tier 3 (recompute baseline through the full fixed harness;
confirm leak-audit aborts on a seeded leak; confirm a zero-call run now
ERRORs; confirm GGUF serving) · **Epic 7** docs (feature doc, CHANGELOG,
post), push.

## 6. Testing Plan
T1: zero-call guard unit (task w/ 0 successful calls → error not 0.0);
leak-audit detects a seeded exact-match leak (exit 1); build_clean_eval
produces the 17-task contract shape; ubench SHA/rows/tasks match the new
file. T2: `make test-ubench-lint` green with re-pinned SHA; `pytest
neural/` benchmark + regression suites; config scanner; go build (Go
side untouched but verify). T3 (live): run the baseline model through the
new clean-eval + GGUF harness → fresh baseline report written + agg
recorded; seed a known leak into the eval → preflight aborts; force a
zero-call condition (bad port) → run ERRORs (not 0.0); candidate served
via llama-server :8102 (not MLX) — confirm in process list / logs.

## 7. Commit Strategy
Per-epic · lint (Go) + ruff (Python) each · push once · summary · CI watch.

## 8. Verification Checklist
- [ ] Gate evals on a rebuilt leak-free 17-task holdout; ubench SHA re-pinned
- [ ] Leak audit runs in-gate and aborts before model calls on leak
- [ ] Candidates + baseline served via GGUF llama-server :8102 (not MLX)
- [ ] Zero successful calls → ERROR, never aggregate 0.0
- [ ] Baseline recomputed from a fresh report (no frozen 0.8338 constant)
- [ ] Provisional-baseline caveat documented (re-run after reward fix)
- [ ] Feature doc + CHANGELOG + post

## 9. Documentation Update — Epic 7 (never cut).

## 10. Risks & Mitigations
Rebuilt eval still leaks vs the (changing) corpus → leak-audit runs every
gate, not once; eval rebuilt again if the corpus shifts. Wrong task set
in the clean eval → mirror the gate's 17-task contract exactly; assert
counts. GGUF serving harness diverges from production → reuse the
documented Phase-13.5 llama-server invocation + MODEL-DIST fuse scripts.
Provisional baseline mistaken for final → loud doc + a `provisional:true`
field in the baseline report until the reward sprint re-runs it. Changing
the gate could mask a real regression → every change makes the gate
STRICTER (leak-free, real form, hard-fail), never looser.

## 11. Documents Accessed
audit doc + eval-integrity recon (this dir + sibling); the configs/specs/
scripts in §4; FT-CLASSIFY-002 run_record (GGUF-form + zero-call lessons);
Phase 13.5 cutover (llama-server invocation).

## 12. Rollback Procedures
All config/code reverts; the rebuilt eval + baseline report are new files
(old ones retained). No model trained, nothing promoted. Frozen-constant
baseline can be restored by revert if the recompute path regresses.
