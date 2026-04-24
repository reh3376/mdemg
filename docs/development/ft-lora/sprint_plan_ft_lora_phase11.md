# Sprint FT-LORA-PHASE11 — Automated RL Post-Training (GRPO Primary, DPO Secondary)

## Context

Phase 10 (PR #348, merged 2026-04-24) shipped the automated benchmark framework and established the first authoritative Phase 10 baseline for `.local-models/qwen3-14b-mdemg-v1/`: **aggregate weighted score 0.8338**, 16 of 17 ULTS tasks exercised (run_id `q283a23bz59mrg6faxo32ydx2`). Per-task reward-variance stddev is persisted to `benchmark_results` (TSDB V0012 migration authored; live-apply deferred to Phase 11 ops). Per `04_BENCHMARK_RL_v2.md §Phase 11` and `03_IMPLEMENTATION_PLAN_v2.md §5F/Phase 11`, the next sprint is **automated RL post-training** — reading Phase 10's reward signal factory and producing an RL-refined adapter that:

1. Consumes `benchmark_results.reward_vector` per (task, run_idx) as the GRPO reward source and `benchmark_results.stddev` as the advantage-normalization denominator.
2. Trains a GRPO adapter on top of Phase 5 dense Qwen3-14B SFT (`.local-models/qwen3-14b-mdemg-v1/`, base SHA `a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5`).
3. Ships a DPO pair generator consuming `benchmark_results` rows (win/loss selection by reward delta) — training deferred to Phase 12 HITL per `00_README_v2.md §Phase 12`.
4. Unblocks Phase 12 (HITL DPO) by producing a reproducible RL pipeline + dual regression gate (vs Phase 5 SFT baseline 0.8338 + vs fresh dense-14B).

**Why this sprint now:** Phase 10 closed with the `guardrail.evaluate` 3-way gap unaddressed and the `hidden.reclassify` patch still shadow-mode only — both flagged as Phase 10.5 tasks (#216/#215) but not gating Phase 11. The Phase 10 baseline 0.8338 is non-regression-tested (single benchmark, no historical baselines for stagnation detection). Phase 11 GRPO cannot skip the baseline; the first Phase 11 artifact must be the reward-delta against 0.8338.

**Why MVP scope (Option C — GRPO primary, DPO secondary):** Option A (GRPO only) is too narrow — DPO pair generation is the forcing function for Phase 12 HITL, and delaying it pushes Phase 12 back by a full sprint. Option B (GRPO + DPO training) is too wide — DPO training without HITL pair curation is unvalidated and risks regression on structured-notink tasks. Option C ships GRPO training + DPO pair generator (training deferred), which is the smallest increment that unblocks Phase 12 while preserving Phase 11's forcing function (producing a reward-refined adapter).

**Phase dependency chain:** Phase 5 SFT (done) → Phase 10 benchmark (done, PR #348) → **Phase 11 (this) — GRPO training + DPO pair generator** → Phase 12 HITL DPO → Phase 13 distillation (out of scope, `00_README_v2.md §Phase 13`).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE11 |
| Title | Automated RL Post-Training — GRPO trainer + DPO pair generator + dual regression gate |
| Date | 2026-04-24 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-PHASE10 (PR #348, merged 2026-04-24); FT-LORA-PHASE5 (PR #347, merged 2026-04-23) |
| Successors | FT-LORA-PHASE12 (HITL DPO) |
| Type | Code-heavy (~1200-1500 LOC: GRPO trainer + DPO pair generator + reward sampler + regression harness); infra-light (V0013 TSDB migration); compute-heavy (~4-8 hrs MLX training on Apple Silicon) |
| Risk | HIGH (Risk #1 below — no native GRPO in `mlx_lm==0.31.2`) |
| Budget | OpenAI judge replay via Phase 10 harness during regression: ~$10-20; well under $100 cap |
| Model under test | Base: `.local-models/qwen3-14b-mdemg-v1/` dense SHA pin `a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5`; Output: `.local-models/qwen3-14b-mdemg-v1-rl/` (Phase 11 adapter merged onto Phase 5 dense) |
| MLX port | `127.0.0.1:8101` (single-instance constraint preserved) |
| New TSDB migration | **013_rl_training.sql** (head is `012_benchmark_results.sql` authored in Phase 10 but not live-applied; 013 depends on 012 — apply 012 as part of Epic 0 preflight). Path: `internal/tsdb/migrations/` (NOT `internal/storage/tsdb/migrations/`); naming convention `NNN_name.sql`. Creates `rl_training_runs` + `rl_training_steps` tables |
| Post-sprint artifacts | `neural/training/rl/{trainer.py,grpo_loss.py,reward_sampler.py,advantage.py}`; `neural/training/dpo/{pair_generator.py}`; `neural/training/rl/tests/test_*.py`; `configs/rl_phase11.yaml`; migration V0013; Phase 11 regression report; RL-refined adapter `.local-models/qwen3-14b-mdemg-v1-rl/`; sprint docs |

## 2. Problem Statement

Build a reproducible, CI-compatible RL post-training pipeline that, given the Phase 5 dense SFT adapter + Phase 10 benchmark history, produces:

1. **GRPO-refined LoRA adapter** (r=32, α=64, `router_aux_loss_coef` N/A for dense) trained on top of `.local-models/qwen3-14b-mdemg-v1/`, with explicit `epochs=3` cap (MEMORY: `n_epochs=auto` disallowed), early-stop on `val_reward < best × 0.95` for 2 consecutive evals.
2. **Per-task advantage normalization** using `benchmark_results.stddev` per (task, sampling_group). Zero-stddev policy (9 of 16 tasks currently show stddev=0 in Phase 10 baseline — investigated below) configurable: default `intra_batch_only` (compute advantage denominator from within-batch variance when historical stddev is 0), fallback `widen` (scale widening factor × mean reward), escape hatch `drop` (skip task in RL loop).
3. **DPO pair generator** — reads `benchmark_results` rows, buckets by (task_id, prompt_hash), selects win/loss pairs by `reward_vector` delta ≥ configurable threshold (default 0.15), writes JSONL pair files under `training_data/dpo/phase11/`. Pair count, reward delta distribution, task coverage logged. **Training deferred to Phase 12**.
4. **Dual regression gate** — (5a) vs Phase 5 SFT baseline 0.8338 via Phase 10 harness replay (no regression > 2% per task, aggregate ≥ 0.8338 × 1.02 = 0.8505 target); (5b) vs fresh dense-14B re-mergerd (defensive — catches adapter-merge corruption). Both gates must pass before adapter merge is blessed.
5. **Persistence** — `rl_training_runs` (run_id CUIDv2, adapter paths, config SHA, started/completed_at, final_aggregate_reward, gate_verdict) + `rl_training_steps` (step_idx, loss components, advantage stats, reward samples) rows per training run. V0013 migration additive.
6. **Stagnation interop** — Phase 11 adapter's benchmark_run becomes benchmark #2; Phase 10's stagnation detector is informational with count=2, blocks nothing this sprint.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | GRPO trainer | `neural/training/rl/trainer.py` |
| 2 | GRPO loss (clipped surrogate + KL penalty) | `neural/training/rl/grpo_loss.py` |
| 3 | Reward sampler (reads `benchmark_results` + streams via Phase 10 harness) | `neural/training/rl/reward_sampler.py` |
| 4 | Advantage estimator (zero-stddev policies) | `neural/training/rl/advantage.py` |
| 5 | DPO pair generator | `neural/training/dpo/pair_generator.py` |
| 6 | RL config | `configs/rl_phase11.yaml` (epochs=3, clip_ratio=0.2, kl_coef=0.01, lr=1e-5, batch_size, zero_stddev_policy, advantage_clip, early_stop, seed) |
| 7 | DPO config | `configs/dpo_phase12_pairs.yaml` (reward_delta_threshold, per-task min pairs, prompt-hash strategy) |
| 8 | TSDB migration 013 | `internal/tsdb/migrations/013_rl_training.sql` |
| 9 | Phase 11 regression harness | `neural/training/rl/regression.py` — invokes Phase 10 runner twice, compares against 0.8338 + fresh dense-14B |
| 10 | Phase 11 regression report | `training_data/eval/phase11_regression_report.json` |
| 11 | DPO pair set | `training_data/dpo/phase11/pairs.jsonl` + manifest with SHAs |
| 12 | Unit + integration + e2e tests | `neural/training/rl/tests/test_*.py`, `neural/training/dpo/tests/test_*.py` |
| 13 | Sprint docs | `docs/development/ft-lora/sprint_plan_ft_lora_phase11.md`, `phase_11_rl_post.md` |
| 14 | Doc updates | `00_README_v2.md` v5.7 → v5.8; `03_IMPLEMENTATION_PLAN_v2.md §Phase 11` EXECUTED with SHA stamps; `04_BENCHMARK_RL_v2.md §Phase 11` EXECUTED; `AGENT_HANDOFF.md` top entry; `CHANGELOG.md` `[Unreleased] ### Added`; `CLAUDE.md` — add RL training invocation under Testing |

**Out of scope (deferred to Phase 12 or later):**
- DPO training loop (pair generator ships; training defers to Phase 12 HITL)
- Nightly RL scheduler / launchd plist
- `mdemg finetune rl` CLI wiring (manual `python -m neural.training.rl.trainer` sufficient this sprint)
- Distillation (Phase 13)
- Multi-adapter ensembling
- Reward-model training (using deterministic reward registry + LLM judge only, per Phase 10)

**Constraints (hard):**
- **MEMORY: epoch cap = 3** on all LoRA runs; `n_epochs=auto` disallowed. Explicit `epochs: 3` in `configs/rl_phase11.yaml`.
- **MEMORY: early-stop on `val_reward < best × 0.95` for 2 consecutive evals** (RL path per `CLAUDE.md §Overfitting-prevention policies`).
- **MEMORY: no hardcoded values** — clip_ratio, kl_coef, lr, batch_size, zero_stddev_policy, advantage_clip_range all in `configs/rl_phase11.yaml` with CLI overrides.
- **MEMORY: CUIDv2 for run_id** — `cuid2` Python package (Phase 10 precedent, already vendored via `neural/benchmarks/`).
- **MEMORY: sequential epics** — no parallel epic execution; docs before implementation within each epic.
- **MEMORY: 3-tier testing** — unit / integration (mocked reward stream + mocked MLX) / e2e (real Phase 5 adapter + real Phase 10 reward stream, 3-task × N=2 smoke run before full).
- **MEMORY: min max_tokens ≥ 3000, min latency_budget_ms ≥ 15000** — applies to any LLM judge calls in regression phase (inherited from Phase 10 `llm_judge.py`).
- **MEMORY: no tight budget caps** — target $10-20 OpenAI spend; flag only if >$100.
- **MEMORY: plan-options pattern** — Risk #1 (native GRPO vs custom) + zero-stddev policy + DPO scope are the three decision forks; recommendations + rationale documented, user picks at execution, PR discloses.
- **MEMORY: single batched commit at sprint close**.
- **MEMORY: sprint summary posted to PR comments immediately after push** (not gated on CI).
- **Base model read-only** — never overwrite `.local-models/qwen3-14b-mdemg-v1/`; RL output goes to `.local-models/qwen3-14b-mdemg-v1-rl/` (new directory).
- **SHA pins asserted** — base model SHA, Phase 5 adapter manifest SHAs, Phase 10 baseline run_id stamped in every `rl_training_runs` row, Phase 11 regression report, and sprint post doc.
- **MLX single-instance** — preflight `ps` for exactly one `mlx_lm.server` on `127.0.0.1:8101` before starting.
- **TSDB additive** — V0013 creates new tables only; zero ALTER on V0011/V0012 tables.
- **V0012 apply gate** — V0012 was authored in Phase 10 but deferred on live apply; Epic 0 preflight applies V0012 to dev DB (reverse-tested), then V0013 stacks on top.
- **Dual regression gate blocking** — Phase 11 adapter cannot be merged to `.local-models/` until both 5a + 5b pass. Abort + rollback if either fails.

## 4. Dependencies

**Consumed (code, pre-existing):**
- `neural/training/reward_functions.py` — `REWARD_REGISTRY` + `compute_reward()` (Phase 10 consumer, unchanged here).
- `neural/benchmarks/run_benchmark.py` — Phase 10 runner; invoked by Phase 11 regression harness as black box.
- `neural/benchmarks/sampling_policy.py` — T/C/J group recipes (inherited by GRPO rollout sampler).
- `neural/benchmarks/variance.py` — `RunAggregator`; reused by advantage estimator for intra-batch variance when historical stddev=0.
- `neural/benchmarks/llm_judge.py` — judge pipeline; only called during regression 5a (never during RL rollouts — too expensive per step).
- `neural/training/evaluate_ft.py` — Phase 5 evaluator; read-only access to `regression_gate.py` verdict logic.
- `.local-models/qwen3-14b-mdemg-v1/` — Phase 5 dense adapter + base merged; RL trains on top.
- `configs/benchmark_phase10.yaml` — reference for sampling recipes, group weights (0.50/0.35/0.15); Phase 11 config inherits these.
- `internal/tsdb/migrations/012_benchmark_results.sql` — must apply before 013.

**Consumed (data):**
- `training_data/eval/valid_golden.jsonl` — Phase 10 golden holdout; reused for RL eval passes + regression.
- `training_data/eval/benchmark_qwen3_14b_v1_baseline.json` — Phase 10 baseline (aggregate 0.8338); gate 5a target.
- `docs/tests/ults/specs/*.ults.json` — 17 ULTS specs; read during GRPO rollout construction.
- `benchmark_results` TSDB rows (16 specs × 5 runs = 80 rows from Phase 10 run_id `q283a23bz59mrg6faxo32ydx2`) — primary reward source.

**Consumed (compute):**
- Apple Silicon (M-series, ≥48GB unified memory) — MLX dense-14B training with LoRA r=32 fits within Metal 499K MTLResource ceiling (validated in Phase 5).
- OpenAI API — `gpt-5.4-mini` judge replay during regression 5a (≤$20 spend).

**External services:**
- Local MLX server on `127.0.0.1:8101` (rollout inference during GRPO).
- OpenAI API (regression only; never during RL step loop).
- TSDB — reads V0012 rows; writes V0013 new tables only.

No Neo4j writes. No network writes beyond OpenAI + localhost.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` clean; venv `mdemg-ft-lora` active; Phase 5 model present + SHA matches pin; Phase 10 benchmark row present in TSDB; OpenAI key in `.env`; MLX `:8101` single-instance.

### Epic 0 — Preflight + TSDB 012 Apply + Config Scaffolding

1. Apply 012 to dev DB: `mdemg tsdb migrate --dry-run` → `mdemg tsdb migrate`. Reverse-test (roll back 012 → re-apply) to validate rollback path. Phase 10 authored `internal/tsdb/migrations/012_benchmark_results.sql` but deferred live-apply; Phase 11 cannot ship without 012 rows.
2. Preflight script `python -m neural.training.rl.preflight`:
   - Assert `benchmark_results` contains ≥1 run_id (Phase 10 baseline).
   - Assert per-task row count ≥ N (default 5, config-driven). Abort if any task < N with per-task diff.
   - Assert stddev is computed per task (NULL check). Flag zero-stddev tasks for policy decision.
   - Assert Phase 5 adapter manifest SHAs match `.local-models/qwen3-14b-mdemg-v1/` on disk.
3. `configs/rl_phase11.yaml` — all MEMORY-mandated knobs: `epochs: 3`, `early_stop_patience: 2`, `early_stop_threshold: 0.95`, `clip_ratio: 0.2`, `kl_coef: 0.01`, `lr: 1e-5`, `batch_size: 8`, `zero_stddev_policy: intra_batch_only`, `advantage_clip: [-5.0, 5.0]`, `seed: 0`, `judge_model: gpt-5.4-mini` (regression only).
4. `configs/dpo_phase12_pairs.yaml` — `reward_delta_threshold: 0.15`, `min_pairs_per_task: 20`, `prompt_hash_strategy: blake2b_16`.

**Gate:** 012 apply+reverse green; preflight report attached; zero-stddev tasks enumerated with policy proposal; configs load + schema-validate.

### Epic 1 — GRPO Loss + Advantage Estimator (supporting modules)

**Decision fork (Risk #1):** `mlx_lm==0.31.2` has no native GRPO. Two options:

- **Option A:** Vendor `mlx-lm-lora` (PyPI `mlx-lm-lora==0.x`). Risk: version pin drift, transitive deps, unknown maintenance. ~100-200 LOC glue.
- **Option B:** Write custom GRPO in `neural/training/rl/trainer.py` — clipped surrogate with ratio `exp(logπ_new - logπ_old)`, KL penalty against Phase 5 reference, advantage normalization by `benchmark_results.stddev`. ~400-600 LOC.

**Recommendation: Option B** (custom). Reasons: (1) no external dep drift, (2) full control over zero-stddev policy + advantage clipping, (3) MEMORY `feedback_plan_options_pattern.md` precedent — Sprints A/B/C all chose in-repo over vendor. Disclose at PR.

1. `grpo_loss.py` — `compute_grpo_loss(logprobs_new, logprobs_old, advantages, ref_logprobs, clip_ratio, kl_coef)` → scalar loss + component dict (policy, kl, entropy). Numerically stable (clamp ratio, log-space KL).
2. `advantage.py` — `estimate_advantage(rewards, task_ids, stddev_cache, policy)` → per-sample advantage. Zero-stddev policies: `intra_batch_only` (use within-batch stddev when historical=0), `widen` (scale by mean × constant), `drop` (skip task in loss). Advantage clipping to config range.
3. `reward_sampler.py` — reads `benchmark_results` for a (task, prompt_hash) window; samples N rewards; returns `(prompt, response, reward_vector, stddev, sampling_group)` tuples for batch construction.
4. Unit tests (fixtures only, no MLX):
   - Loss numerics (known hand-computed scalars).
   - Zero-stddev handling (all 3 policies).
   - Reward sampler DB access (mocked TSDB).

**Gate:** unit suite green; loss regression against hand-computed fixture ≤ 1e-6 delta.

### Epic 2 — GRPO Trainer (integration)

1. `trainer.py` — `GRPOTrainer` class:
   - Loads Phase 5 dense adapter as initial policy; loads fresh copy as reference policy (frozen).
   - Per step: sample batch from `reward_sampler`, rollout via MLX `:8101` (honoring sampling group), compute logprobs under current + reference, call `advantage.estimate_advantage()`, call `grpo_loss.compute_grpo_loss()`, backprop with MLX optim.
   - Per N steps (config): eval on `valid_golden.jsonl` subset, compute val_reward, apply early-stop gate.
   - Emits per-step row to `rl_training_steps` (step_idx, loss components, advantage mean/std, reward batch mean).
   - On completion: write `rl_training_runs` row (CUIDv2 run_id, paths, gate_verdict initially NULL).
2. CLI: `python -m neural.training.rl.trainer --config configs/rl_phase11.yaml --base .local-models/qwen3-14b-mdemg-v1 --out .local-models/qwen3-14b-mdemg-v1-rl --run-id <cuidv2>`.
3. Integration test (3-task × N=2 × 20 steps, mocked MLX rollouts): verifies step loop, DB persistence, early-stop plumbing, checkpoint write.

**Gate:** integration green; `rl_training_steps` has 20 rows; checkpoint written to `.local-models/test-rl/`; MLX single-instance assertion fires on duplicate.

### Epic 3 — TSDB 013 Migration

1. `internal/tsdb/migrations/013_rl_training.sql` — `rl_training_runs` (run_id CUIDv2 PK, base_model_path, base_model_sha, adapter_manifest_sha, output_path, config_sha, started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ, final_aggregate_reward NUMERIC, gate_verdict TEXT CHECK IN ('pending','pass','fail'), phase10_baseline_run_id REF); `rl_training_steps` (step_id CUIDv2, run_id FK, step_idx, policy_loss, kl_loss, total_loss, advantage_mean, advantage_std, reward_batch_mean, recorded_at TIMESTAMPTZ).
2. Hypertable on `rl_training_steps.recorded_at`.
3. Up + down migrations; `mdemg tsdb migrate --dry-run` green; no ALTER on 011/012.

**Gate:** 012 → 013 forward + 013 → 012 reverse green on test DB; `benchmark_results` + `benchmark_runs` row counts unchanged.

### Epic 4 — DPO Pair Generator

1. `pair_generator.py` — reads `benchmark_results`; for each (task_id, prompt_hash) bucket with ≥2 rows, emits pairs where `|reward_a - reward_b| ≥ threshold`; writes JSONL `{prompt, chosen, rejected, chosen_reward, rejected_reward, task_id, sampling_group, pair_id}`.
2. CLI: `python -m neural.training.dpo.pair_generator --config configs/dpo_phase12_pairs.yaml --source <phase10_run_id>,<phase11_run_id> --out training_data/dpo/phase11/pairs.jsonl`.
3. Manifest: total pairs, per-task coverage, reward delta histogram, SHAs. Saved alongside `pairs.jsonl`.
4. Unit test: fixture with 3 tasks × 5 rows → expected pair count + delta math.

**Gate:** pairs.jsonl written; per-task coverage ≥ `min_pairs_per_task` for ≥ N-1 of N active tasks (documented exceptions); manifest SHA logged.

### Epic 5 — Dual Regression Gate (5a + 5b)

1. `regression.py` — orchestrator:
   - **5a (vs Phase 5 SFT baseline 0.8338):** merge Phase 11 adapter onto fresh Phase 5 dense base, invoke Phase 10 `run_benchmark.py` with `configs/benchmark_phase10.yaml`, compare aggregate + per-task against 0.8338 / per-task Phase 10 baseline. Pass: aggregate ≥ 0.8338 × 1.02 (0.8505) AND no per-task regression > 2%.
   - **5b (vs fresh dense-14B re-merge):** re-merge Phase 11 adapter onto a freshly-downloaded base; run Phase 10 benchmark; assert delta vs 5a ≤ 0.5% (catches adapter-merge corruption).
2. Gate verdict written to `rl_training_runs.gate_verdict` ('pass' | 'fail'); on fail, adapter stays in sandbox path `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/` and sprint rollback applies.
3. `phase11_regression_report.json` — full per-task delta table, aggregate, judge spend, SHAs.

**Gate:** both 5a + 5b pass OR sprint aborts with full report + rollback plan invoked. On pass: adapter blessed to `.local-models/qwen3-14b-mdemg-v1-rl/` (rename from sandbox).

### Epic 6 — Testing (3 Tiers)

**Tier 1 (Unit):** `pytest -xvs neural/training/rl/tests/ neural/training/dpo/tests/` — loss numerics, advantage policies, pair generator math, reward sampler mock, preflight checks. ≥90% coverage of new code.

**Tier 2 (Integration):**
- `test_trainer_integration.py` — 3-task × N=2 × 20 steps with mocked MLX rollouts + mocked TSDB; assert DB rows, early-stop, checkpoint.
- `test_pair_generator_integration.py` — real TSDB with seeded fixture; assert JSONL correctness + manifest.
- `test_regression_harness_mocked.py` — Phase 10 runner mocked to return canned aggregates; assert pass/fail routing.

**Tier 3 (E2E):**
- T-subset smoke (3 ULTS tasks × 2 runs × 50 steps) against real MLX + real Phase 5 adapter. Budget-conscious; validates end-to-end before full RL run.
- Full RL run (16 tasks, N_runs per config, full epoch cap 3) → regression 5a + 5b.
- TSDB spot-check: `bash scripts/tsdb_spot_check.sh` confirms `rl_training_runs` + `rl_training_steps` row counts.

**Gate:** all 3 tiers green; T-subset smoke passes before full run launched.

### Epic 7 — Phase 10 Deferrals Pulled Forward (opportunistic)

Low-cost items that Phase 10 deferred, resolved alongside Phase 11 so Phase 12 inherits a cleaner base:

1. **`--scorer=registry` default flip** (Phase 10 Epic 4 deferred) — after Phase 11 regression 5a passes with registry scorer, flip default in `evaluate_ft.py`. Reduces dual-path maintenance debt.
2. **Stagnation auto-exit** (Phase 10 deferred) — with Phase 11 producing benchmark #2, enable informational stagnation log in `run_benchmark.py` (still non-blocking; Phase 12 decides if it hard-gates).
3. **CLAUDE.md RL testing section** — add `python -m neural.training.rl.trainer --config configs/rl_phase11.yaml` invocation under Testing.

**Gate:** each deferral's test re-runs green; no existing behavior changes.

### Epic 8 — Documentation (Final Epic — Never Cut)

1. `docs/development/ft-lora/sprint_plan_ft_lora_phase11.md` — this plan verbatim.
2. `docs/development/ft-lora/phase_11_rl_post.md` — executed-truth doc: final aggregate reward, regression table (5a + 5b), DPO pair stats, zero-stddev policy outcome, training wall-clock, judge spend actual, any policy choices made at execution.
3. `00_README_v2.md` v5.7 → v5.8: Phase 11 EXECUTED; link trainer + post report.
4. `03_IMPLEMENTATION_PLAN_v2.md §Phase 11` — mark EXECUTED with SHA stamps; note Phase 12 unblocked.
5. `04_BENCHMARK_RL_v2.md §Phase 11` — mark EXECUTED; reference regression report.
6. `AGENT_HANDOFF.md` top entry: Phase 11 complete; Phase 12 HITL DPO unblocked; DPO pair set ready.
7. `CHANGELOG.md [Unreleased] ### Added`: RL trainer, GRPO loss, advantage estimator, DPO pair generator, V0013 migration, Phase 11 regression report, adapter `qwen3-14b-mdemg-v1-rl`.
8. `CLAUDE.md` — add RL training command under Testing section (Epic 7.3).

**Gate:** all docs committed; cross-refs valid; `grep -r "Phase 11.*pending\|Phase 11.*planned" docs/development/ft-lora/` returns zero hits.

## 6. Testing Plan (Three Tiers)

Covered in Epic 6. **State restoration (MEMORY):** all changes additive. Rollback = revert commit; `rm -rf neural/training/rl/ neural/training/dpo/ .local-models/qwen3-14b-mdemg-v1-rl* training_data/dpo/phase11/ training_data/eval/phase11_regression_report.json configs/rl_phase11.yaml configs/dpo_phase12_pairs.yaml`; `mdemg tsdb migrate --target V0012` (reverse V0013).

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ft-lora): Sprint FT-LORA-PHASE11 — automated RL post-training (GRPO + DPO pair gen + dual regression)`
- Body: scope summary, GRPO-vs-vendor decision + rationale, zero-stddev policy chosen at execution, new module tree, V0013 migration note, regression 5a + 5b verdicts, DPO pair count + coverage, policy compliance checklist (CUIDv2, no hardcoded values, sequential epics, 3-tier testing, epoch cap 3, early-stop wired).
- Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`.

Push to `reh3376_dev01` → auto-PR opens → **sprint summary comment posted to PR per MEMORY rule (not gated on CI)**.

## 8. Verification Checklist

- [ ] Epic 0: V0012 apply+reverse green; preflight report attached; configs load
- [ ] Epic 1: loss + advantage + reward sampler unit tests green; numerics ≤1e-6 delta
- [ ] Epic 2: GRPO trainer integration green (20-step mocked); MLX single-instance asserted
- [ ] Epic 3: V0013 up+down clean; hypertable on `rl_training_steps.recorded_at`
- [ ] Epic 4: DPO pair generator writes ≥ min_pairs_per_task for ≥15/16 tasks; manifest SHA logged
- [ ] Epic 5: regression 5a aggregate ≥ 0.8505 AND no per-task > 2% drop; 5b delta ≤ 0.5%
- [ ] Epic 6: all 3 test tiers green; T-subset smoke before full run
- [ ] Epic 7: `--scorer=registry` default flip test green; stagnation log enabled; CLAUDE.md updated
- [ ] Epic 8: sprint plan + post report + v5.8 banners + §Phase 11 EXECUTED × 2 docs + AGENT_HANDOFF + CHANGELOG
- [ ] Commit pushed; auto-PR opened; **sprint summary posted to PR immediately**
- [ ] OpenAI spend logged, under $100 cap

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 8: `sprint_plan_ft_lora_phase11.md`, `phase_11_rl_post.md`, `00_README_v2.md` v5.7→v5.8, `03_IMPLEMENTATION_PLAN_v2.md §Phase 11` EXECUTED, `04_BENCHMARK_RL_v2.md §Phase 11` EXECUTED, `AGENT_HANDOFF.md` prepended, `CHANGELOG.md [Unreleased] ### Added`, `CLAUDE.md` Testing section.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | **`mlx_lm==0.31.2` has NO native GRPO** | Certain | Option B recommended: custom trainer in `neural/training/rl/trainer.py` (~400-600 LOC); unit-tested loss numerics against hand-computed fixtures | Option A: vendor `mlx-lm-lora`; pin version; document in CLAUDE.md |
| 2 | **9 of 16 Phase 10 tasks show stddev=0** (deterministic rewards — C/J groups) | High (known) | Default `zero_stddev_policy: intra_batch_only` — compute within-batch stddev when historical=0; disclose at PR; Phase 12 decides long-term policy | Fallback policies `widen` / `drop` configurable per run |
| 3 | **Dual regression 5a fails (Phase 5 baseline regression)** | Medium | Early-stop on `val_reward < best × 0.95`; config-driven `kl_coef` bump; sandbox checkpoint path so base Phase 5 remains clean | Revert to Phase 5 adapter; file Phase 11.5 remediation; keep DPO pairs (Phase 10 data, still valid) |
| 4 | **Adapter merge corruption (5b fails, 5a passes)** | Low | Re-merge on fresh dense base; assert delta ≤ 0.5% vs 5a; MLX version + tooling pinned | Redo merge with bf16-explicit; document in post |
| 5 | **Compute budget exhausted (>8 hrs wall-clock)** | Medium | T-subset smoke gates full run; `max_steps` config cap; `early_stop_patience: 2` kills non-productive runs | Resume from checkpoint (`--resume-from <run_id>`); split across sessions |
| 6 | **OpenAI budget exceeded ($100 cap)** | Low | Judge only called during 5a regression (one-shot), never per RL step; est. $10-20 | Disable `--enable-judge` for regression; use registry-only verdict |
| 7 | **V0013 migration breaks V0012 rows** | Low | Additive only (new tables); reverse test gates forward | `mdemg tsdb migrate --target V0012` cleanly rolls back |
| 8 | **DPO pair generator yields <min_pairs for majority of tasks** | Medium | Lower `reward_delta_threshold` adaptively; document exceptions; Phase 12 can re-gen with Phase 11 + Phase 10 combined | Ship with coverage diff; Phase 12 plans supplemental generation |
| 9 | **MLX single-instance violation (two servers on :8101)** | Low | Preflight `ps` check; abort before training | Kill stray; restart; document in post |
| 10 | **Phase 10 baseline (0.8338) is non-reproducible** | Low | Re-run Phase 10 benchmark on fresh Phase 5 adapter at Epic 0; confirm aggregate within 1% | Re-baseline Phase 10 first; shift Phase 11 by half-day |
| 11 | **`cuid2` package behavior drift** | Low | Vendored in Phase 10; use same pin | Fallback `time.time_ns()` + blake2b per Phase 10 |
| 12 | **Zero-stddev policy choice regresses T-group tasks** | Medium | Default `intra_batch_only` tested on T-subset before full run; config-driven switch at any point | `widen` fallback; document in post |

## 11. Documents Accessed (during planning)

**Read during planning:**
- `/Users/reh3376/mdemg/docs/development/ft-lora/04_BENCHMARK_RL_v2.md` §Phase 11 (GRPO spec, reward sources, advantage normalization)
- `/Users/reh3376/mdemg/docs/development/ft-lora/00_README_v2.md` v5.7 Phase 10 block + Phase 11 roadmap
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_10_benchmark_post.md` — baseline 0.8338, run_id, zero-stddev task count
- `/Users/reh3376/mdemg/AGENT_HANDOFF.md` top entry — Phase 10 unblock
- `/Users/reh3376/mdemg/CHANGELOG.md [Unreleased]` — Phase 10 entries
- `/Users/reh3376/mdemg/CLAUDE.md` — overfitting policies, testing section, MEMORY references
- `/Users/reh3376/mdemg/neural/benchmarks/run_benchmark.py` — Phase 10 runner interface
- `/Users/reh3376/mdemg/neural/benchmarks/variance.py` — intra-batch variance utility
- `/Users/reh3376/mdemg/neural/benchmarks/sampling_policy.py` — T/C/J recipes
- `/Users/reh3376/mdemg/neural/training/reward_functions.py` — registry
- `/Users/reh3376/mdemg/internal/storage/tsdb/migrations/V0012__benchmark_results.sql` — schema for V0013 to extend
- `/Users/reh3376/mdemg/configs/benchmark_phase10.yaml` — weights/sampling inheritance
- `/Users/reh3376/mdemg/training_data/eval/benchmark_qwen3_14b_v1_baseline.json` — regression 5a target
- Memory: `feedback_sprint_plan_format.md`, `feedback_sprint_summary_on_pr.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `project_phase5_moe_pivot.md`

## 12. Rollback

All changes additive.

1. `git revert <final commit SHA>`.
2. `rm -rf neural/training/rl/ neural/training/dpo/ .local-models/qwen3-14b-mdemg-v1-rl* training_data/dpo/phase11/ training_data/eval/phase11_regression_report.json configs/rl_phase11.yaml configs/dpo_phase12_pairs.yaml`.
3. `mdemg tsdb migrate --target V0012` (reverse V0013; drops `rl_training_runs` + `rl_training_steps` only).
4. Revert Epic 7 defaults (scorer flip, stagnation log) — bit-identical Phase 10 paths.
5. Revert Grafana JSON to Phase 10 version (if any panels added).

Phase 5 + Phase 10 artifacts untouched throughout. No Neo4j writes. V0013 rows dropped by reverse migration (auditable beforehand).

---

## Post-Sprint — Phase 12 (HITL DPO) Unblocks

On merge, Phase 12 planning begins. Phase 12 consumes:
- `training_data/dpo/phase11/pairs.jsonl` + manifest → HITL curation input
- `.local-models/qwen3-14b-mdemg-v1-rl/` → Phase 12 DPO base
- `rl_training_runs.final_aggregate_reward` → Phase 12 success benchmark floor
- Zero-stddev policy outcome → informs Phase 12 reward strategy

Phase 11 is intentionally MVP: DPO training, nightly RL scheduler, CLI wiring, distillation all deferred. The deferred pieces are Phase 12/13 scaffolding, not Phase 11 blockers.
