# Phase 11 Automated RL Post-Training — Post-Run Report

**Sprint:** FT-LORA-PHASE11
**Date executed:** 2026-04-24
**Branch:** `reh3376_dev01`
**Final verdict:** ✅ **CODE COMPLETE — GRPO trainer + DPO pair generator + dual regression harness shipped, 73 tests green. Compute run (real MLX GRPO + regression gates 5a/5b) deferred to operator execution.**

---

## Executive Summary

Phase 11 delivered the full RL post-training code surface that Phase 10's reward-signal factory was built to feed: a framework-light GRPO trainer, a configurable advantage estimator (with three zero-stddev policies), a reward sampler that reads `benchmark_results`, a DPO pair generator that turns the same table into preference pairs, and a dual regression gate harness (5a vs Phase 5 SFT baseline 0.8338 + 5b vs fresh-merge). V0013 TSDB migration (`rl_training_runs` + `rl_training_steps` hypertable) is applied live.

**Exit criterion met** (Option B refined, confirmed mid-sprint): Epics 1–5 code-complete and unit-test-green; Epic 6 three-tier harness green; Epic 7 in-repo deferrals that don't require compute are done; Epic 8 docs landed. **Phase 11 compute** — actual MLX GRPO training run + dual regression gate execution — is explicitly operator-gated and not part of this sprint's commit. The code path `python -m neural.training.rl.trainer --config configs/rl_phase11.yaml …` is wired, tested with mocked rollouts, and ready to attach to an MLX optimizer step (follow-up task `#227`, see §7).

**Why this scope:** the plan's Risk #1 (no native GRPO in `mlx_lm==0.31.2`) and Option B (custom trainer in-repo) both held. Option B's ~400–600 LOC budget was met at ~330 LOC for the orchestrator plus ~150 LOC for loss + ~170 LOC for advantage + ~200 LOC for reward sampler — tight because the MLX-optimizer coupling was isolated behind an injectable `OptimizerStepFn`. The unit suite hand-computes the expected loss to 1e-6 tolerance against a known fixture, so when the MLX wiring lands the numerics layer is already proven.

**DPO pair generation validated end-to-end against live TSDB** — Phase 10's `q283a23bz59mrg6faxo32ydx2` baseline yielded 5 pairs across 2 tasks (`retrieval.query_classify` ×4 + `jiminy.synthesize` ×1). Coverage is intentionally thin for Phase 11 (there's only one source benchmark run); Phase 12 will re-generate against Phase 10 + Phase 11 combined once the compute pass ships.

---

## 1. Scope Delivered (MVP per sprint plan §3)

| # | Deliverable | Path | Status |
|---|---|---|---|
| 1 | GRPO loss (clipped surrogate + KL + entropy) | `neural/training/rl/grpo_loss.py` | ✅ |
| 2 | Advantage estimator (3 zero-stddev policies) | `neural/training/rl/advantage.py` | ✅ |
| 3 | Reward sampler (reads `benchmark_results`) | `neural/training/rl/reward_sampler.py` | ✅ |
| 4 | GRPO trainer (orchestrator, MLX-agnostic) | `neural/training/rl/trainer.py` | ✅ |
| 5 | Preflight (5 gates: TSDB rows, stddev, SHA, golden, config) | `neural/training/rl/preflight.py` | ✅ |
| 6 | Dual regression harness (5a + 5b) | `neural/training/rl/regression.py` | ✅ |
| 7 | DPO pair generator | `neural/training/dpo/pair_generator.py` | ✅ |
| 8 | RL config | `configs/rl_phase11.yaml` | ✅ |
| 9 | DPO config | `configs/dpo_phase12_pairs.yaml` | ✅ |
| 10 | TSDB V0013 migration | `internal/tsdb/migrations/013_rl_training.sql` | ✅ (applied live, schema_meta=13) |
| 11 | Tests (3 tiers, 73 total) | `neural/training/{rl,dpo}/tests/` | ✅ |
| 12 | DPO pair set + manifest (end-to-end from Phase 10 run) | `training_data/dpo/phase11/{pairs.jsonl,manifest.json}` | ✅ |
| 13 | Sprint docs | This file + `sprint_plan_ft_lora_phase11.md` | ✅ |

**Deferred (explicit, operator-gated):**

- **MLX optimizer wiring** — the trainer's `OptimizerStepFn` / `RolloutFn` / `EvalFn` / `CheckpointFn` callables are Protocol-typed; the adapter that binds them to `mlx.optimizers.AdamW` + LoRA state lives in the follow-up task (#227). Reason: Phase 5's MLX training code ran against `mlx_lm.lora` CLI, not a custom in-process trainer; the adapter is ~100 LOC but needs a live-MLX smoke before it's useful, and a live MLX run is outside the sprint's code-complete exit criterion.
- **Real GRPO training run** — operator task. Runbook: §7 below.
- **Dual regression gate execution (5a + 5b)** — operator task, gated on the training run above.
- **Adapter promotion** `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/ → .local-models/qwen3-14b-mdemg-v1-rl/` — gated on gate-5a PASS (aggregate ≥ 0.8505 AND no per-task > 2pp regression) AND gate-5b PASS (|5a − 5b| ≤ 0.5pp).
- **`--scorer=registry` default flip** in `evaluate_ft.py` (Epic 7) — gated on Phase 11 gate-5a passing under the registry scorer; premature otherwise.
- **Stagnation auto-exit log** in `run_benchmark.py` (Epic 7) — gated on Phase 11 producing `benchmark_runs.count() >= 2`; the single Phase 10 row isn't enough to trip the detector.

---

## 2. Code Architecture

### 2.1 Module tree

```
neural/training/
├── rl/
│   ├── __init__.py
│   ├── grpo_loss.py            # compute_grpo_loss(logprobs_new, logprobs_old, advantages, ref_logprobs, …)
│   ├── advantage.py            # estimate_advantage(rewards, task_ids, stddev_cache, mean_cache, policy, …)
│   ├── reward_sampler.py       # read_benchmark_rows / build_stddev_cache / sample_batch
│   ├── trainer.py              # GRPOTrainer + GRPOTrainerConfig + SqlSidecarPersistence
│   ├── preflight.py            # 5 gates before training starts
│   ├── regression.py           # evaluate_gate_5a / evaluate_gate_5b / run_dual_regression
│   └── tests/
│       ├── test_grpo_loss.py           # 8 tests
│       ├── test_advantage.py           # 13 tests
│       ├── test_reward_sampler.py      # 16 tests
│       ├── test_trainer_integration.py # 8 tests (Tier 2, mocked MLX)
│       └── test_regression_harness.py  # 12 tests (Tier 2, mocked Phase 10 runner)
└── dpo/
    ├── __init__.py
    ├── pair_generator.py       # scalarize + bucket + filter + manifest
    └── tests/
        └── test_pair_generator.py      # 16 tests
```

### 2.2 Injectable-callable pattern (why this is the shape)

The trainer does **not** depend on MLX directly. It takes four `typing.Protocol`-typed callables:

- `RolloutFn(prompts, responses, sampling_group) → list[RolloutResult]` — produces `(logprob_new, logprob_old, logprob_ref)` triples.
- `OptimizerStepFn(loss, step_idx) → None` — backprops and updates LoRA params.
- `EvalFn(step_idx) → float` — runs `valid_golden.jsonl` subset and returns `val_reward`.
- `CheckpointFn(step_idx, path) → None` — persists adapter weights.

The test suite passes pure-Python mocks for all four, so every invariant (step loop, early-stop, DB persistence, drop-policy handling) is covered in <500 ms per test with no MLX import. When the MLX adapter lands, the three callables are swapped for MLX-bound implementations — no core-trainer change required. This is the same architectural pattern Phase 10's `run_benchmark.py` used to decouple the runner from `llm_judge.py` (injected as `judge_fn`).

### 2.3 Persistence (SQL sidecar, Phase 10 precedent)

`SqlSidecarPersistence` writes `INSERT` / `UPDATE` statements to a `.sql` file as the trainer runs. `rl_training_runs` gets one row per training invocation; `rl_training_steps` gets one row per optimizer step (or one per "skipped" step when `zero_stddev_policy=drop` empties the batch). The sidecar file is applied by the operator via `psql -f`; a live psycopg writer is follow-up work, intentionally not built this sprint because Phase 10 shipped with the same SQL-sidecar pattern and there's no regression risk in deferring.

SQL literal quoting is hand-rolled via `SqlSidecarPersistence._q()` (doubles single quotes). The injection-safety test `test_persistence_sidecar_sql_injection_safe` passes an adversarial string containing `'; DROP TABLE rl_training_runs; --` and verifies the emitted SQL escapes it correctly.

---

## 3. Test Suite

### 3.1 Tier 1 — Unit (37 tests)

| File | Tests | What it covers |
|---|---:|---|
| `test_grpo_loss.py` | 8 | Hand-computed fixture (TOL=1e-6) on policy/KL/entropy/total/mean_ratio/frac_clipped; zero-loss for identical policies; log-ratio clamp protects from `exp()` overflow at ±20; invalid shapes raise. |
| `test_advantage.py` | 13 | Per-task normalization by historical σ; three zero-σ policies (`intra_batch_only`, `widen`, `drop`); missing-mean fallback; `min_stddev` floor prevents divide-by-zero; advantage clip bounds enforced; diagnostics populated. |
| `test_reward_sampler.py` | 16 | Scalarization (mean, skips bools/strings/None, empty dict); mean/stddev cache matches `statistics.pstdev`; zero-stddev task listing; all three sampling strategies (random, weighted_by_inverse_count, stratified_by_group); stratified covers all groups when `n ≥ n_groups`; reproducibility under fixed rng_seed. |

### 3.2 Tier 2 — Integration (36 tests)

| File | Tests | What it covers |
|---|---:|---|
| `test_trainer_integration.py` | 8 | Full 20-step loop with 3-task × 2-row fixture and mocked rollout; SQL sidecar has correct INSERT/UPDATE counts; early-stop fires at step 6 with 2 consecutive val-reward drops (best=1.0 → 0.9); `drop` policy handles zero-batch gracefully (0 optimizer calls, n_samples=0 rows); rollout length mismatch raises; step diagnostics populated; external-source `run_id` respected; SQL-injection-safe. |
| `test_regression_harness.py` | 12 | Gate 5a pass + fail (aggregate below target) + fail (per-task regression) + missing-task flag + `per_task_regressions` dict; Gate 5b pass + fail (delta exceeds cap) + symmetry under arg-order swap; `run_dual_regression` passes both / fails 5a only / fails 5b only / writes full report file with all expected keys. |
| `test_pair_generator.py` | 16 | Scalarize mean + weighted + fallback; prompt-hash strategies (default groups by task, `task_first_tokens` distinguishes by response); delta filter; max-per-bucket cap; zero-delta excluded; multi-task independence; chosen > rejected sort invariant; single-row buckets skipped; `pair_id` stable across calls; manifest `under_floor` + histogram + empty-pairs; round-trip `write_outputs` writes valid JSONL + SHA256. |

### 3.3 Tier 3 — E2E (live-system validation, this sprint)

- **V0013 migration applied live** against the dev docker-compose TSDB. `mdemg tsdb status` returns `schema_meta.value = 13`; `\dt rl_training_*` shows both new tables; `rl_training_steps` is a hypertable with 30-day chunks (confirmed via `SELECT * FROM _timescaledb_catalog.hypertable`).
- **Trainer sidecar round-trip** — ran the trainer with the 20-step mocked fixture, emitted a `.sql` sidecar, applied it via `psql -f`: 1 `INSERT INTO rl_training_runs`, 20 `INSERT INTO rl_training_steps`, 1 `UPDATE rl_training_runs SET completed_at=…`. No errors; constraint `gate_verdict IN ('pending','pass','fail')` accepted the default `pending`.
- **DPO pair generation end-to-end** — `TSDB_PORT=5433 python -m neural.training.dpo.pair_generator --config configs/dpo_phase12_pairs.yaml` read the 80 rows from Phase 10 run `q283a23bz59mrg6faxo32ydx2` and produced `training_data/dpo/phase11/pairs.jsonl` (5 pairs) + `manifest.json` (SHA256 `bbe7bb9a…`, `tasks_under_floor` reflecting the thin Phase 11 source set).
- **Full MLX GRPO smoke + regression gate** — **not run this sprint** (operator task; see §7).

---

## 4. Configuration

### 4.1 `configs/rl_phase11.yaml`

All MEMORY-mandated knobs explicit:

- `training.epochs: 3` (MEMORY: epoch cap = 3; `n_epochs=auto` disallowed).
- `training.early_stop_threshold: 0.95`, `early_stop_patience: 2` (MEMORY: `val_reward < best × 0.95` for 2 consecutive evals).
- `training.seed: 0`.
- `grpo.clip_ratio: 0.2`, `kl_coef: 0.01`, `entropy_coef: 0.0`, `advantage_clip: [-5.0, 5.0]`.
- `advantage.zero_stddev_policy: intra_batch_only` (default — see §5), `widen_factor: 0.05`, `min_stddev: 1.0e-4`.
- `reward_sampler.strategy: stratified_by_group`, `samples_per_task_per_step: 8`.
- `rollout.max_tokens: 4000` (MEMORY: ≥ 3000), `latency_budget_ms: 30000` (MEMORY: ≥ 15000).
- `regression.phase5_baseline_aggregate: 0.8338`, `aggregate_target_multiplier: 1.02`, `per_task_max_regression: 0.02`, `fresh_merge_max_delta: 0.005`.
- `regression.judge_model: gpt-5.4-mini`, `judge_max_tokens: 4000`, `judge_latency_budget_ms: 30000`.

### 4.2 `configs/dpo_phase12_pairs.yaml`

- `reward_delta_threshold: 0.15`
- `min_pairs_per_task: 20`
- `prompt_hash_strategy: blake2b_16`
- `max_pairs_per_bucket: 4`
- `reward_aggregation: mean`

---

## 5. Zero-Stddev Policy Decision (Plan §10 Risk #2)

Per the Phase 10 baseline, 9 of 16 tasks have `stddev=0` in `benchmark_results` — every C-group task (deterministic JSON-valid + classification-accuracy rewards) and the J-group's `retrieval.rerank_cross`. For these tasks the historical σ can't be used as an advantage-normalization denominator.

**Chosen default: `intra_batch_only`.** Rationale:

1. Even when the historical reward is deterministic, the RL batch's rollouts have real variance from sampling temperature. Computing σ from the current batch gives a meaningful denominator.
2. The alternative `widen` policy (mean × 0.05) is a floor, not a signal — advantages all compress to one scale regardless of which rollout was better. Useful as a fallback, not a default.
3. `drop` is a safety valve — it lets the operator carve out a task entirely if its rollouts are pathological, without editing training data.

The config exposes all three (`advantage.zero_stddev_policy`), and the trainer handles the `drop` case specially: if a batch ends up with zero samples because every task in it was dropped, the optimizer step is skipped (not a NaN loss) and a diagnostic row is still written with `n_samples=0` and `n_dropped=<batch_size>`. Covered by `test_drop_policy_handles_zero_batch`.

**Phase 12 feedback loop:** once Phase 11's compute pass runs and we have post-training reward variance data, re-assess whether `intra_batch_only` held. If the C-group tasks overfit (zero advantage signal → no learning), switch to `drop` for those tasks and backfill Phase 12 DPO training on them instead.

---

## 6. TSDB V0013 Migration

**Authored:** `internal/tsdb/migrations/013_rl_training.sql`
**Applied live:** 2026-04-24 via `./bin/mdemg tsdb migrate` on the dev docker-compose TSDB instance.
**Schema version:** `tsdb_schema_meta.value` advanced `12 → 13`.

### 6.1 New tables (additive — zero ALTER on existing tables)

**`rl_training_runs`**

| Column | Type | Constraint |
|---|---|---|
| `run_id` | TEXT | PK (CUIDv2) |
| `base_model_path` | TEXT | NOT NULL |
| `base_model_sha` | TEXT | NOT NULL |
| `adapter_manifest_sha` | TEXT | nullable (set at completion) |
| `output_path` | TEXT | sandbox path until gate PASS |
| `config_sha` | TEXT | SHA of `configs/rl_phase11.yaml` used |
| `phase5_baseline_run_id` | TEXT | FK to `benchmark_runs.run_id` |
| `started_at` | TIMESTAMPTZ | NOT NULL |
| `completed_at` | TIMESTAMPTZ | nullable |
| `final_aggregate_reward` | NUMERIC | nullable |
| `gate_verdict` | TEXT | `CHECK IN ('pending','pass','fail')` default `'pending'` |
| `early_stopped` | BOOLEAN | default `false` |
| `early_stop_reason` | TEXT | nullable |
| `notes` | TEXT | nullable |

Indices: `(started_at DESC)`, `(gate_verdict, started_at DESC)`.

**`rl_training_steps`** (hypertable on `recorded_at`, 30-day chunks)

| Column | Type | Notes |
|---|---|---|
| `step_id` | TEXT | PK (CUIDv2) |
| `run_id` | TEXT | FK to `rl_training_runs.run_id` |
| `step_idx` | INTEGER | monotonic per run |
| `recorded_at` | TIMESTAMPTZ | hypertable dimension |
| `policy_loss` / `kl_loss` / `total_loss` | NUMERIC | |
| `advantage_mean` / `advantage_std` | NUMERIC | after clip |
| `reward_batch_mean` | NUMERIC | scalar reward over batch |
| `mean_ratio` | NUMERIC | mean π_new/π_old (pre-clip) |
| `frac_clipped` | NUMERIC | fraction of ratios outside clip range |
| `n_samples` / `n_dropped` | INTEGER | accounting for zero-stddev drops |

Indices: `(run_id, step_idx)`, `(recorded_at DESC)`.

### 6.2 Reverse migration

`mdemg tsdb migrate --target V0012` drops both tables cleanly; `benchmark_runs` + `benchmark_results` untouched. Reverse-tested offline before apply.

---

## 7. Operator Runbook — Compute Pass (follow-up task)

The code path is wired; these are the sequenced steps an operator runs to produce the Phase 11 blessed adapter. Not part of this commit.

```bash
# Step 1 — Preflight (5 gates: TSDB rows, stddev policy, SHA, golden holdout, config).
TSDB_PORT=5433 python -m neural.training.rl.preflight --config configs/rl_phase11.yaml

# Step 2 — MLX single-instance check + venv activate.
source /Users/reh3376/mdemg-ft-lora/bin/activate
ps aux | grep -c "[m]lx_lm.server.*8101" # must equal 1

# Step 3 — Wire MLX adapter (one-time, ~100 LOC bind of trainer callables to
# mlx.optimizers.AdamW + LoRA param gather). Follow-up task #227.

# Step 4 — Full GRPO run.
python -m neural.training.rl.trainer \
  --config configs/rl_phase11.yaml \
  --base .local-models/qwen3-14b-mdemg-v1 \
  --out .local-models/qwen3-14b-mdemg-v1-rl-sandbox \
  --out-sidecar training_data/eval/rl_phase11_run.sql

# Step 5 — Apply sidecar to TSDB.
docker exec mdemg-tsdb-1 psql -U mdemg -d mdemg_metrics \
  -f /path/to/rl_phase11_run.sql

# Step 6 — Dual regression (5a vs Phase 5 baseline + 5b vs fresh-merge).
python -m neural.training.rl.regression \
  --config configs/rl_phase11.yaml \
  --sandbox-adapter .local-models/qwen3-14b-mdemg-v1-rl-sandbox \
  --fresh-adapter  .local-models/qwen3-14b-mdemg-v1-rl-fresh  # operator re-merge

# Step 7 — On PASS (both gates): promote sandbox → blessed.
mv .local-models/qwen3-14b-mdemg-v1-rl-sandbox \
   .local-models/qwen3-14b-mdemg-v1-rl

# Step 8 — Flip registry scorer default (Epic 7 deferral).
sed -i '' 's/default="heuristic"/default="registry"/' neural/training/evaluate_ft.py
# re-run Phase 5 regression replay to confirm bit-identical delta < 1%.

# Step 9 — Enable stagnation log (Epic 7 deferral).
# run_benchmark.py already reads benchmark_runs.count(); no code change needed.
```

Budget: ~4–8 hrs MLX wall-clock for Step 4; $10–20 OpenAI judge spend for Step 6. Plan §2 / §10 Risk #5/#6.

---

## 8. Artifacts Shipped

| Artifact | Path | Sha / Size |
|---|---|---|
| GRPO trainer | `neural/training/rl/trainer.py` | ~330 LOC |
| GRPO loss | `neural/training/rl/grpo_loss.py` | ~150 LOC |
| Advantage estimator | `neural/training/rl/advantage.py` | ~170 LOC |
| Reward sampler | `neural/training/rl/reward_sampler.py` | ~200 LOC |
| Preflight | `neural/training/rl/preflight.py` | ~130 LOC |
| Regression harness | `neural/training/rl/regression.py` | ~230 LOC |
| DPO pair generator | `neural/training/dpo/pair_generator.py` | ~280 LOC |
| RL config | `configs/rl_phase11.yaml` | 157 lines |
| DPO config | `configs/dpo_phase12_pairs.yaml` | ~30 lines |
| V0013 migration | `internal/tsdb/migrations/013_rl_training.sql` | applied live, schema_meta=13 |
| Test suite | `neural/training/{rl,dpo}/tests/` | 73 tests, all green |
| DPO pair set | `training_data/dpo/phase11/pairs.jsonl` | 5 pairs, 2 tasks |
| DPO manifest | `training_data/dpo/phase11/manifest.json` | SHA256 `bbe7bb9a…` |
| Sprint plan | `docs/development/ft-lora/sprint_plan_ft_lora_phase11.md` | 12-section v1.0 format |
| Sprint post | `docs/development/ft-lora/phase_11_rl_post.md` | this file |

---

## 9. Verification Checklist (Sprint plan §8)

- [x] Epic 0: V0012 apply + V0013 schema_meta=13; configs load + schema-validate; preflight 5-gate module in place.
- [x] Epic 1: GRPO loss + advantage + reward sampler unit tests green; numerics match hand-computed fixture at TOL=1e-6.
- [x] Epic 2: trainer integration green on 20-step mocked run; sidecar writes; early-stop wired; drop-policy zero-batch handled.
- [x] Epic 3: V0013 up + down clean; hypertable on `rl_training_steps.recorded_at`; applied live.
- [x] Epic 4: DPO pair generator runs end-to-end against Phase 10 TSDB; manifest + SHA logged; under-coverage documented (§1).
- [x] Epic 5: regression harness 12 tests green (code-level gate logic proven; real gate execution deferred to operator).
- [x] Epic 6: all 3 test tiers green (73 tests).
- [x] Epic 7: CLAUDE.md Testing section updated with Phase 11 RL commands; `--scorer=registry` flip + stagnation auto-exit deferred with explicit gate (§1).
- [x] Epic 8: sprint plan + this post doc + 00_README_v2 v5.7→v5.8 + §Phase 11 EXECUTED in 03_IMPL + 04_BENCH + AGENT_HANDOFF + CHANGELOG.
- [ ] **Operator:** MLX adapter wiring + compute pass + regression gate execution + blessing (§7).

---

## 10. Policy Compliance Checklist

| Policy | Evidence |
|---|---|
| Epoch cap = 3 | `configs/rl_phase11.yaml` `training.epochs: 3` explicit |
| `n_epochs=auto` disallowed | trainer reads integer from config; no `auto` branch exists |
| Early-stop `val_reward < best × 0.95` × 2 | `GRPOTrainer._eval_and_check_early_stop()` in `trainer.py` |
| CUIDv2 identifiers | `neural/benchmarks/_ids.new_run_id()` reused for `run_id` + `step_id` |
| No hardcoded values | every knob in `rl_phase11.yaml` / `dpo_phase12_pairs.yaml` with CLI override |
| Sequential epics | one epic at a time, docs before implementation within each |
| 3-tier testing | unit (37) + integration (36) + e2e (live V0013 apply + DPO from live TSDB) |
| Single batched commit at sprint close | one commit on `reh3376_dev01` |
| Sprint summary on PR | posted immediately after push per MEMORY `feedback_sprint_summary_on_pr` |
| `max_tokens ≥ 3000` | rollout `4000`, judge `4000` |
| `latency_budget_ms ≥ 15000` | rollout `30000`, judge `30000` |
| No tight LLM budget caps | est. $10–20 regression spend, no cap below $100 |
| Plan-options pattern | Risk #1 (GRPO custom vs vendor) + zero-stddev policy disclosed at PR |

---

## 11. Deferrals + Phase 12 Unblocks

**Deferred (with explicit gates):**
- MLX adapter wiring (follow-up task #227) — gates on operator compute availability.
- Real GRPO run + regression gates 5a/5b — gates on #227.
- Adapter blessing + `--scorer=registry` flip + stagnation log — gate on gate-5a PASS.

**Phase 12 (HITL DPO) unblocks:** `training_data/dpo/phase11/pairs.jsonl` is the curation input; once Phase 11's compute run lands and we re-generate against Phase 10 + Phase 11 combined, Phase 12 starts with a full preference-pair set. The trainer + loss + advantage modules are also directly reusable for Phase 12's DPO training loop — the scaffolding is the same, just a different loss function.

---

## 12. Documents Accessed (during execution)

- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_ft_lora_phase11.md` — this sprint's plan
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_10_benchmark_post.md` — Phase 10 baseline numbers + run_id
- `/Users/reh3376/mdemg/docs/development/ft-lora/04_BENCHMARK_RL_v2.md §Phase 11` — GRPO spec + reward-source contract
- `/Users/reh3376/mdemg/docs/development/ft-lora/00_README_v2.md` v5.7 — phase roadmap
- `/Users/reh3376/mdemg/neural/benchmarks/run_benchmark.py` — Phase 10 runner shape (inherited by regression harness)
- `/Users/reh3376/mdemg/neural/benchmarks/variance.py` — stddev utility (reused by advantage estimator)
- `/Users/reh3376/mdemg/neural/benchmarks/_ids.py` — CUIDv2 generator (reused for run_id + step_id)
- `/Users/reh3376/mdemg/internal/tsdb/migrations/012_benchmark_results.sql` — V0013 stacks on top
- `/Users/reh3376/mdemg/configs/benchmark_phase10.yaml` — sampling recipe inheritance
- `/Users/reh3376/mdemg/CLAUDE.md` — MEMORY rules + Testing section
- Memory: `feedback_sprint_plan_format.md`, `feedback_sprint_summary_on_pr.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`
