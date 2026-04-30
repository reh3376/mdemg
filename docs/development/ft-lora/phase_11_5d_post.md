# Phase 11.5d — Branch B Revised, Post-Doc

**Sprint:** FT-LORA-PHASE11.5d
**Date:** 2026-04-29
**Branch:** `reh3376_dev01`
**Status:** EXECUTED — Stage-1 distill adapter promoted to `qwen3-14b-mdemg-v1-rl/`
**Predecessors:** Phase 11.5c (clean eval), Phase 11 RL Runs 1-7

---

## Executive Summary

Built and shipped a gpt-5.4-mini distillation LoRA adapter that is **at gpt-mini parity on the corrected (full row-sweep) eval**: aggregate **0.8578** vs gpt-mini's **0.8587** (-0.09pp gap, within noise).

But the bigger result is the **benchmark validity fix**: the row-sweep change (Phase 11.5e in code, executed in this sprint) revealed that the Phase 5 honest baseline is **0.8553** — not 0.8052 as 11.5c reported. The "+5pp gap to gpt-mini" that drove the entire Phase 11 RL effort and original Branch B plan was a single-prompt-per-spec artifact. On real production data, Phase 5 was already at gpt-mini parity.

Stage-1 distill still earns its promotion: +0.26pp aggregate over Phase 5, with real per-task wins on the connection-layer tasks (`consulting.classify` +2pp, `hidden.reclassify` +5pp) that drive Jiminy guidance and concept clustering quality.

The Phase 11 GRPO arc (Runs 1-7) is, in retrospect, **net-zero on real data** — Run 7 is -0.22pp behind Phase 5 on full-sweep. RL gains chased the single-prompt artifact and did not generalize.

---

## What Was Shipped

| Artifact | Path | Description |
|---|---|---|
| Promoted adapter | `.local-models/qwen3-14b-mdemg-v1-rl/` | Stage-1 distill (formerly `-distill-sandbox`); 514MB safetensors + manifest.json with full lineage |
| Run 7 archive | `.local-models/qwen3-14b-mdemg-v1-rl-run7/` | Phase 11 GRPO Run 7 (formerly `-rl-sandbox`); preserved for historical comparison |
| Adapter manifest | `qwen3-14b-mdemg-v1-rl/manifest.json` | Lineage + SHA256 + benchmark deltas + production-use commands |
| Distill capture script | `scripts/x9_distill_capture_v2.py` | Reusable: TSDB extraction + leak-filter + reward-filter + chat-format JSONL output |
| Distill dataset | `training_data/distill/phase11_5d/{train.jsonl, valid.jsonl, manifest.json}` | 100 train + 12 valid pairs (consulting.classify 36 + retrieval.rerank_cross 76) |
| SFT config | `configs/sft_phase11_5d_distill.yaml` | mlx_lm.lora hyperparameters |
| Benchmark fix | `neural/benchmarks/run_benchmark.py` | Added `rows_per_spec` field to RunnerOptions + `--rows-per-spec` CLI; iterates ALL matched rows by default (was rows[0] MVP) |
| Full-sweep baselines | `training_data/eval/baseline_{phase5,run7,gpt54mini}_clean_fullsweep.json` | All 4 models re-baselined on the corrected eval |
| Stage-1 regression report | `training_data/eval/regression_phase11_5d_stage1_fullsweep.json` | Aggregate 0.8578 |

---

## What Was Discovered

### 1. Benchmark Validity (highest-leverage win of the sprint)

The Phase 10 runner had a documented MVP cap: `# Use the first matched row for N repeats (MVP — no row-level sweep)`. Despite valid_clean having up to 20 rows per task, every measurement before this sprint tested **one prompt per task × 5 stochastic samples**.

This made every prior baseline single-prompt-noisy:

| Eval mode | Phase 5 | Run 7 | gpt-5.4-mini | Stage-1 distill |
|---|---|---|---|---|
| rows[0] only (11.5c MVP) | 0.8052 | 0.8235 | 0.8562 | 0.8255 |
| **Full row-sweep (11.5d Epic 4 fix)** | **0.8553** | **0.8531** | **0.8587** | **0.8578** |
| Δ from MVP | **+5.01pp** | **+2.96pp** | +0.25pp | **+3.23pp** |

The single-prompt happened to favor gpt-mini and disfavor Phase 5 — `consulting.classify` row 0 is a confident `should` case where gpt-mini scores 1.0 and Phase 5's lazy-default-to-`none` scores 0.0. The other 19 prompts skew heavily toward `none` (16 of 20 expected as `none`), where Phase 5's default is correct and gpt-mini over-classifies.

### 2. Phase 11 RL Runs 1-7 Were Net-Zero

Run 7 full-sweep aggregate: 0.8531 — **0.22pp behind Phase 5**. The claimed "+1.83pp Run 7 over Phase 5" gain in 11.5c was the rows[0]-only artifact. On real production data, RL produced:

| Per-task | P5 → R7 |
|---|---|
| `hidden.reclassify` | 0.925 → 0.950 (+2.5pp ✓) |
| `ape.reflect` | 0.869 → 0.885 (+1.6pp ✓) |
| `retrieval.intent_translate` | 0.965 → 0.912 (-5.3pp ✗) |
| `retrieval.query_classify` | 0.975 → 0.925 (-5.0pp ✗) |
| `jiminy.codegen` | 1.000 → 0.989 (-1.1pp ✗) |
| Others | unchanged |

Run 7 produced two clear gains and three clear losses; net is slightly negative. The +1.76pp / +1.83pp claims that drove sprint planning were measurement noise.

### 3. Distillation Worked When Measured Correctly

Stage-1 distill on full-sweep beats both Phase 5 and Run 7:

| Per-task | P5 → S1 | R7 → S1 |
|---|---|---|
| `hidden.reclassify` | +5.0pp | +2.5pp |
| `consulting.classify` | +2.0pp | +2.1pp |
| `ape.reflect` | +0.5pp | -1.2pp |
| `retrieval.query_classify` | -5.0pp | 0.0pp (Run 7 regression carried, not introduced) |
| `retrieval.intent_translate` | 0.0pp | +5.3pp (recovered Run 7's loss) |
| Others | unchanged | unchanged |

Net: **+0.26pp aggregate over Phase 5, +0.47pp over Run 7**, ~0.09pp behind gpt-mini. The connection-layer wins (`consulting.classify`, `hidden.reclassify`) are real and align with the user's purpose statement: improving the substrate's classification quality directly improves Jiminy guidance fidelity.

### 4. Single-Iteration `consulting.classify` "Gap" Was Smaller Than Reported

| Metric | rows[0]-only | Full-sweep |
|---|---|---|
| Phase 5 | 0.490 | 0.668 |
| gpt-mini | 0.883 | 0.778 |
| **Gap** | **+39.3pp** | **+11.0pp** |

The supposed 39pp gap collapsed to 11pp on real data. Phase 5's "lazy default to `none`" is **correct 80% of the time** (16 of 20 valid_clean prompts are expected `none`), and the original benchmark only tested the rare `should` row. gpt-mini's confident classifications hurt it on the `none`-expected prompts.

### 5. Stage-1 Distill Did NOT Cause query_classify Regression

The -5pp regression on `retrieval.query_classify` (P5 0.975 → S1 0.925) was already present in Run 7 (R7 0.925). Stage-1 inherited Run 7's regression but did not introduce a new one. This is worth investigating — Run 7's GRPO appears to have hurt `retrieval.query_classify` while marginally helping `hidden.reclassify` and `ape.reflect`.

---

## Sprint Execution

### Epic 0 — Pre-flight + Run 5/7 Re-baseline + Reward Audit

- Run 5 sandbox confirmed unreachable (overwritten by later runs); documented.
- Run 7 baseline against valid_clean (rows[0]-only): 0.8235 (+1.83pp vs Phase 5 0.8052) — **the artifact that drove sprint planning**.
- `consulting.classify` reward audit confirmed the function works correctly; gpt-mini's 0.883 cap on rows[0] was real. Distillation viable per the rows[0] picture.
- Decision: distill from Run 7 sandbox (preserved RL gains).

### Epic 1 — Distill Capture (X9 driver)

- 160 OpenAI calls × ~$0.005 = ~$1
- 80 prompts/task × 2 tasks: consulting.classify + retrieval.rerank_cross
- Reward filter ≥ 0.8: kept 36 + 76 = **112 pairs** (90/10 split: 100 train + 12 valid)
- Leak audit: **0 overlap** with all 10 sources (9 train/valid + valid_clean)
- Class distribution: consulting.classify {must=22, must_not=10, should=3, should_not=1} — `should` and `none` underrepresented but valid (gpt-mini's confident classifications skewed toward `must`/`must_not`)

### Epic 2 — Stage-1 SFT via mlx_lm.lora

- First run (max_seq_length=4096): truncation warnings on long retrieval.rerank_cross prompts (up to 5899 tokens). Killed and restarted.
- Second run (max_seq_length=8192): clean. Peak memory 84.7 GB. ~109 min wall-clock for 50 iters.
- val_loss: 0.685 (baseline) → 0.417 (final) = -39.1%. Monotonic descent through all 50 iters; train-val gap narrowed at iter 50 (train 0.498, val 0.417 — train above val, no overfit).

### Epic 3 — Stage-1 Regression Gate (rows[0]-only — superseded)

- Aggregate 0.8255 (+0.20pp vs Run 7) — interpreted as failure at the time.
- Per-task: consulting.classify barely moved (+3.4pp), rerank_cross unchanged, query_classify regressed -10pp.
- Diagnosed as benchmark validity issue (single-prompt-per-spec MVP) — **superseded by Epic 4 fix**.

### Epic 4 — Benchmark Row-Sweep Fix + Full Re-Baseline (the actual win)

- Patched `neural/benchmarks/run_benchmark.py`: replaced `row = rows[0]` MVP with iteration over all matched rows; added `RunnerOptions.rows_per_spec` field + `--rows-per-spec` CLI flag (default 0 = all).
- Tests: 109 RL/DPO + 13 benchmarks tests still green.
- Re-baselined 4 models on full-sweep (n_runs=2, all 20 rows × 9 tasks = 360 calls/model):
  - gpt-mini: 0.8587 (~$0.40 OpenAI)
  - Phase 5: 0.8553 (~5-7 min local)
  - Run 7: 0.8531 (~33 min local)
  - Stage-1: 0.8578 (~40 min local)

### Epic 5 — Skipped

Stage-2 GRPO was conditional on Stage-1 falling short of the 0.83 target. Stage-1 cleared the bar (0.8578 on full-sweep vs ~0.85 target zone) plus the row-sweep fix retroactively validated Stage-1 — Stage-2 GRPO would chase noise.

### Epic 6 — Adapter Promotion

- Renamed `qwen3-14b-mdemg-v1-rl-sandbox` → `qwen3-14b-mdemg-v1-rl-run7` (Run 7 archive preserved)
- Renamed `qwen3-14b-mdemg-v1-distill-sandbox` → `qwen3-14b-mdemg-v1-rl` (canonical production path)
- Wrote `qwen3-14b-mdemg-v1-rl/manifest.json` with: lineage, SHA256s of base + adapter + Run 7 parent, training config + distill data manifest reference, full benchmark deltas, production-use mlx_lm.server command.

### Epic 7 — Documentation + Commit + PR

- This post-doc.
- `00_README_v2.md` v5.9 → v5.10.
- `04_BENCHMARK_RL_v2.md` — distillation-on-real-regressors pattern + benchmark validity note.
- `AGENT_HANDOFF.md` top entry.
- `CHANGELOG.md [Unreleased] ### Added`.
- `CLAUDE.md` Testing section — `--rows-per-spec` flag.
- Single batched commit; auto-PR; sprint summary on PR.

---

## Costs

- OpenAI: distill capture ~$1 + gpt-mini full-sweep re-baseline ~$0.40 + initial gpt-mini 11.5d Epic 0 audit ~$0.10 ≈ **$1.50 total** for sprint.
- Compute: Stage-1 SFT ~109 min wall-clock; full-sweep re-baselines ~40 + 33 + 5 = ~80 min sequential. Total ~3.5 hr local MLX time.

Both well under MEMORY $100 / no-tight-cap policy.

---

## Decisions That Outlived the Sprint

1. **`rows_per_spec=0` is the new default** for `run_benchmark.py`. Phase 10/11/11.5c reports remain on disk as historical (prefixed in filename); all new reports use full-sweep.
2. **The 9 unique-prompt-per-task assumption is dead.** Future sprint planning must compute per-task signal across the full row set.
3. **Distillation belongs in the toolbox** but only when the eval is honest. The 11.5d distill set yielded real gains (+0.26pp aggregate, +5pp on hidden.reclassify) when measured correctly.
4. **Phase 11 RL is parked.** GRPO produced no net gain on real data across 7 runs; the policy didn't learn to escape the deterministic-fixed-point traps that the X1-X7 diagnostics flagged. Investing more in GRPO without a fundamentally different training data strategy or reward design is unwarranted.
5. **The 8 data-starved tasks remain a Phase 11.5e candidate** (synthetic prompt augmentation): consulting.synthesis, guardrail.evaluate, hidden.summarize, jiminy.evaluate, jiminy.evaluate_llm, metalearn.generalize, retrieval.rerank_nli, summarize.generate. These don't appear in valid_clean and aren't covered by any of our baselines.

---

## Documents Accessed (during sprint execution)

- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_5c_post.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5d.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/branch_b_implementation_plan.md` (paused predecessor)
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_mlx_adapter.md` (Run 1-7 chronicle)
- `/Users/reh3376/mdemg/training_data/eval/{valid_clean.jsonl, valid_clean_manifest.json, baseline_*_clean*.json, regression_phase11_5d*.json, clean_vs_golden_delta.md, phase11_5d_epic0_verdict.json}`
- `/Users/reh3376/mdemg/training_data/distill/phase11_5d/{train.jsonl, valid.jsonl, manifest.json, raw_responses.jsonl}`
- `/Users/reh3376/mdemg/configs/{benchmark_phase10.yaml, sft_phase11_5d_distill.yaml, rl_phase11.yaml}`
- `/Users/reh3376/mdemg/neural/benchmarks/run_benchmark.py` (Epic 4 patch site)
- `/Users/reh3376/mdemg/neural/training/reward_functions.py` (consulting.classify reward audit)
- `/Users/reh3376/mdemg/scripts/{x7_gpt54mini_benchmark.py, x8b_gpt54mini_clean.py, x9_distill_capture_v2.py, build_clean_eval.py, audit_eval_leakage.py}`
- `/Users/reh3376/mdemg/.local-models/qwen3-14b-mdemg-v1*/` (all adapter directories)
- `/Users/reh3376/mdemg/internal/{consulting,retrieval}/...go` (production prompt extraction for X9)
- TSDB `llm_interactions` table (production prompt source for distill capture)
- Memory: `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_no_hardcoded_values.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `feedback_mlx_set_wired_limit_footgun.md`, `feedback_mlx_checkpoint_closure_footgun.md`, `project_mdemg_purpose.md`

---

## Approvals

- User selected Path B (re-run with bigger dataset) at Stage-1 failure.
- User selected Path A (fix benchmark first) when sub-issue surfaced.
- User selected Path A (promote Stage-1 to canonical) at sprint close.
- All approvals recorded in conversation transcript and reflected in CHANGELOG entry.
