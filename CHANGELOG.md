# Changelog

All notable changes to MDEMG will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **FT-LORA-PHASE11: Automated RL Post-Training — CODE EXECUTED, GRPO trainer + DPO pair generator + dual regression harness shipped, compute pass operator-gated** (2026-04-24) — Phase 11 RL code surface delivered. Exit criterion (Option B refined): Epics 1–5 code-complete, Epic 6 three-tier tests green (73 total), Epic 7 in-repo deferrals done, Epic 8 docs landed. **Phase 12 HITL DPO unblocked.**
  - **New module tree — `neural/training/rl/`**: `trainer.py` (~330 LOC; MLX-agnostic `GRPOTrainer` with injectable `RolloutFn` / `OptimizerStepFn` / `EvalFn` / `CheckpointFn` Protocol callables + `SqlSidecarPersistence` matching Phase 10's `persist.py` pattern — MLX optimizer wiring is isolated behind the callables so unit tests use pure-Python mocks), `grpo_loss.py` (~150 LOC; clipped surrogate + KL + entropy; log-ratio clamp at ±20 prevents `exp()` overflow; hand-computed fixture asserted at TOL=1e-6), `advantage.py` (~170 LOC; per-task normalization by historical σ + 3 zero-stddev policies: `intra_batch_only` default / `widen` / `drop`), `reward_sampler.py` (~200 LOC; reads `benchmark_results`, 3 sampling strategies `random` / `weighted_by_inverse_count` / `stratified_by_group`), `preflight.py` (~130 LOC; 5 gates), `regression.py` (~230 LOC; dual gate 5a vs Phase 5 SFT baseline 0.8338 × 1.02 = 0.8505 target + 5b vs fresh-merge ≤ 0.5pp delta, injectable `BenchmarkRunner` for test mocking).
  - **New module tree — `neural/training/dpo/`**: `pair_generator.py` (~280 LOC; reads `benchmark_results`, buckets by `(task_id, prompt_hash)`, sorts descending by scalar reward, selects chosen/rejected pairs where `|Δ reward| ≥ 0.15`, caps `max_pairs_per_bucket=4`). End-to-end tested against live Phase 10 TSDB (`q283a23bz59mrg6faxo32ydx2`): 5 pairs across 2 tasks (`retrieval.query_classify` ×4 + `jiminy.synthesize` ×1), manifest SHA256 `bbe7bb9a…`. Artifact: `training_data/dpo/phase11/pairs.jsonl` + `manifest.json`.
  - **TSDB V0013 migration applied live** (`internal/tsdb/migrations/013_rl_training.sql`) — schema_meta 12 → 13. Additive only. New: `rl_training_runs` (PK run_id CUIDv2, FK `phase5_baseline_run_id` → `benchmark_runs.run_id`, `gate_verdict CHECK IN ('pending','pass','fail')`) + `rl_training_steps` (hypertable on `recorded_at`, 30-day chunks, per-step loss components + advantage stats + clip frac + n_samples/n_dropped accounting). Reverse-tested offline before apply.
  - **Configs — zero hardcoded values**: `configs/rl_phase11.yaml` (every MEMORY knob explicit: `training.epochs: 3`, `early_stop_threshold: 0.95`, `early_stop_patience: 2`, `clip_ratio: 0.2`, `kl_coef: 0.01`, `advantage_clip: [-5, 5]`, `zero_stddev_policy: intra_batch_only`, `rollout.max_tokens: 4000` ≥ 3000 MEMORY floor, `rollout.latency_budget_ms: 30000` ≥ 15000 MEMORY floor, `seed: 0`, `regression.phase5_baseline_aggregate: 0.8338`, `aggregate_target_multiplier: 1.02`, `per_task_max_regression: 0.02`, `fresh_merge_max_delta: 0.005`); `configs/dpo_phase12_pairs.yaml` (`reward_delta_threshold: 0.15`, `min_pairs_per_task: 20`, `prompt_hash_strategy: blake2b_16`).
  - **Tests — 3 tiers, 73 total, all green**: Tier 1 unit 37 (`test_grpo_loss.py` 8 + `test_advantage.py` 13 + `test_reward_sampler.py` 16). Tier 2 integration 36 (`test_trainer_integration.py` 8 — 20-step loop with mocked MLX, early-stop fires at step 6 on 2 consecutive val_reward drops (best=1.0 → 0.9), SQL-injection-safe sidecar; `test_regression_harness.py` 12 — gate 5a/5b pass/fail/symmetry + full orchestrator; `test_pair_generator.py` 16 — scalarize + prompt_hash + delta filter + max_per_bucket + pair_id stability + manifest round-trip). Tier 3 e2e live (V0013 apply + trainer sidecar round-trip + DPO from live TSDB).
  - **Plan fork resolutions (§10 Risks #1, #2)**: Risk #1 — chose Option B (custom in-repo trainer) over Option A (vendor `mlx-lm-lora`). `mlx_lm==0.31.2` has no native GRPO. Orchestrator came in at ~330 LOC, below plan's 400–600 LOC estimate. Risk #2 — default `zero_stddev_policy: intra_batch_only` for the 9/16 Phase 10 tasks with σ=0; `widen`/`drop` fallbacks config-selectable.
  - **CLAUDE.md Testing section expanded** with RL post-training subsection: preflight, trainer, DPO pair generator, dual regression, unit+integration suites.
  - **Deferred, operator-gated**: (1) MLX adapter wiring for `OptimizerStepFn`+`RolloutFn` (~100 LOC, task #227); (2) real GRPO run; (3) gate 5a/5b execution; (4) adapter blessing `sandbox → .local-models/qwen3-14b-mdemg-v1-rl/`; (5) `--scorer=registry` default flip (gates on 5a PASS); (6) stagnation auto-exit log (gates on `benchmark_runs.count() ≥ 2`).
  - **Docs**: `docs/development/ft-lora/sprint_plan_ft_lora_phase11.md` (12-section v1.0 format), `phase_11_rl_post.md` (12-section post-run report), `00_README_v2.md` v5.7 → v5.8, `03_IMPLEMENTATION_PLAN_v2.md §Phase 5.11` EXECUTED banner, `04_BENCHMARK_RL_v2.md §Phase 11` EXECUTED banner with code-complete/compute-deferred delineation, `AGENT_HANDOFF.md` top entry, this CHANGELOG entry, CLAUDE.md Testing section.
  - **Sprint chain**: A (#335) → B (#336) → C (#338/#339/#340) → D (#343) → E (`14cd2b3`) → DATA (#346 `234baec`) → PHASE5 (#347 `c0be250`) → PHASE10 (#348 `b81c5fb`) → **PHASE11 (this commit)** → operator compute pass → Phase 12 HITL DPO.

- **FT-LORA-PHASE10: Automated Benchmark Framework — EXECUTED, first authoritative baseline captured, Phase 11 GRPO unblocked** (2026-04-23 → 2026-04-24) — Supersedes the ad-hoc `docs/benchmarks/run_benchmark_v4.py` + `test_questions_120.json` pair. `.local-models/qwen3-14b-mdemg-v1/` scored **aggregate weighted 0.8338** across **16 of 17 ULTS specs × 5 runs = 80 rows**, all `finish_reason=stop` (zero truncations). Per-group means: **T=0.8404** (7 tasks, weight 0.50) / **C=0.8222** (6 tasks, weight 0.35) / **J=0.8389** (3 tasks, weight 0.15). Baseline artifact: `training_data/eval/benchmark_qwen3_14b_v1_baseline.json` (run_id `q283a23bz59mrg6faxo32ydx2`, config SHA `3716f9a4…`, golden SHA `8e44cdf9…`, file SHA `789459f1…`). **Phase 11 GRPO now unblocked** — consumes `benchmark_results.reward_vector` + `stddev` per task as advantage-normalization denominator.
  - **New module tree — `neural/benchmarks/`**: `run_benchmark.py` (runner + group-weighted aggregator + 3-row stagnation detection), `llm_judge.py` (`gpt-5.4-mini` at `temp=0`/`seed=run_idx`/`max_tokens=4000`/`latency_budget_ms=30000`, own-fixed sampling — never inherits task recipe so J-group `presence_penalty=1.5` can't leak into judge reasoning), `sampling_policy.py` (T/C/J group-aware kwargs; drops J-group `top_k=-1` sentinel that MLX rejects), `variance.py` (mean/stddev/min/max per task for GRPO), `preflight.py` (17-spec field + think-mode-aware floor enforcement), `judge_prompts/{coherence,depth,relevance,naturalness}.txt` (4 SHA-pinned templates).
  - **Config — `configs/benchmark_phase10.yaml`**: single source of truth for N_runs (5), stagnation thresholds (`aggregate_delta=0.005`, `per_task_regression=0.02`), sampling-group weights (T:0.50 / C:0.35 / J:0.15), judge kwargs, `performance_floors` (default `max_tokens≥3000`/`latency_budget_ms≥15000`; think_mode `≥10000`/`≥60000`). All CLI-overridable — zero hardcoded constants.
  - **Two silent scorer bugs fixed** in `neural/training/reward_functions.py`, each worth ~2-4% aggregate: (1) `classification_accuracy` treated `expected` as a bare label when the runner passes full assistant JSON — would score valid responses as 0 silently; now shape-detects and compares normalized JSON for equality, list/string keys handled (e.g. `retrieval.query_classify`'s `{"types":["symbol_lookup"],...}`); (2) `evaluation_accuracy` accepted only `expected_verdict` when the runner passes `expected=<json>` — would score every response 1.0 silently; now normalizes both paths. First baseline aggregated 0.7990 under the buggy scorers; post-fix 0.8338 (+0.0348, +4.4% relative). Regression tests added.
  - **`evaluate_ft.py` Epic-4 shadow refactor**: added `--scorer={heuristic,registry,dual}` flag. Default stays `heuristic` (bit-identical Phase 5 regression path). `dual` runs both in parallel with per-task `|delta|<1%` assertion; replayed Phase 5 dev set and confirmed parity. Flip to `registry` default deferred to Phase 11 after ≥3 benchmark rounds confirm stability.
  - **Think-mode performance floors enforced — 8 specs bumped** to `max_tokens≥10000` + `latency_budget_ms≥60000`: `consulting_synthesis`, `hidden_summarize`, `jiminy_synthesize`, `metalearn_generalize`, `summarize_generate` (T-group), plus `jiminy_evaluate`, `jiminy_evaluate_llm`, `retrieval_rerank_cross` (think-mode chain-of-thought). Preflight fails any spec that under-budgets its class (silent truncation → silent zero-credit was the failure mode).
  - **`hidden.reclassify` spec patched**: `reward_functions` trimmed from `["json_valid", "classification_accuracy"]` → `["json_valid"]` and notes annotated — output is an array of cluster objects, not a single classification; the accuracy reward was structurally inapplicable and silently produced 0.0. Cluster-overlap reward tracked for Phase 10.5 UBENCH (#215).
  - **TSDB V0012 migration drafted** (`internal/tsdb/migrations/012_benchmark_results.sql`) — additive `benchmark_results` (hypertable on `recorded_at`; run_id CUIDv2, task_id, run_idx, sampling_group, response_text, reward_vector JSONB, judge_scores JSONB, adapter_sha, base_sha, dataset_sha) + `benchmark_runs` (run_id PK, model_path, aggregate_weighted_score, stagnation_flag, config_sha). SQL present; live migration deferred — Docker down at sprint close. Baseline JSON persisted as durable sidecar (forward-only migrate, no data lost).
  - **Golden holdout** (`training_data/eval/valid_golden.jsonl`): seeded 15% carve from Phase 5 `valid` splits, SHA `8e44cdf9…`, matched via `meta.task_name`.
  - **Known gap — `guardrail.evaluate` (17th spec)**: excluded from baseline for a three-way dependency gap — no golden rows (TSDB has task logs but Docker down), 2 reward functions not yet in `REWARD_REGISTRY` (`guardrail_triggered_accuracy`, `guardrail_severity_match`), no Phase 5 SFT training data. Including it would contaminate J-group mean + Phase 11 GRPO advantage normalization. Deferred to Phase 10.5 (#216).
  - **CLAUDE.md cleanup**: stale `docs/benchmarks/run_benchmark_v4.py` + `test_questions_120.json` references replaced with `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml` in the Testing section.
  - **Deferred** (Phase 10.5 / Phase 11 operational scaffolding): live V0012 TSDB migration; Grafana panels; `benchmark_scheduler.py` + launchd plist; `mdemg finetune benchmark` CLI wiring; `--scorer=registry` default flip; UBENCH promotion to formal UxTS framework (#215).
  - **Docs:** `docs/development/ft-lora/sprint_plan_ft_lora_phase10.md`, `phase_10_benchmark_post.md` (10-section post-run report), `00_README_v2.md` v5.6 → v5.7, `04_BENCHMARK_RL_v2.md §Phase 10` EXECUTED banner with SHA stamps, `AGENT_HANDOFF.md` top entry, this CHANGELOG entry, CLAUDE.md Testing refresh.
  - **Sprint chain:** A (#335) → B (#336) → C (#338/#339/#340) → D (#343) → E (`14cd2b3`) → DATA (#346 `234baec`) → PHASE5 (#347 `c0be250`) → **PHASE10 (this commit)** → Phase 11 GRPO (unblocked).

- **FT-LORA-PHASE5: Phase 5 SFT — EXECUTED, mid-sprint MoE → dense pivot, dual regression gate PASS** (2026-04-22 → 2026-04-23) — First real MDEMG fine-tuning shipped. Merged model at `.local-models/qwen3-14b-mdemg-v1/` (7.8 GB, 4-bit preserved via `mlx_lm fuse`). **Dual regression gate PASS**: 0.9805 (pre-pivot MoE-35B baseline) / 0.9505 (fresh dense-14B baseline) → **0.9856 post-tune**; **16/16 ULTS tasks passing** (baseline: 15/16). Training wall-clock 9h 7m on M5 Max, early-stop fired at Iter 3000, best adapter restored from Iter 2400 (val_loss 0.246). Peak memory 36 GB. **Phase 10 benchmark now unblocked.**
  - **Mid-sprint MoE → dense pivot (2026-04-22).** MoE-Sieve two-tier strategy abandoned after the Metal 499K MTLResource cap on M5 Max / macOS 26 blocked every non-trivial MoE LoRA backward pass (identical `[metal::malloc] Resource limit (499000) exceeded` across 4 mxfp4 configs *and* standard q4 — the cap is architectural, not quant-specific; macOS 26 removed the `iogpu.rsrc_limit` sysctl so no user-space fix exists).
  - **What shipped:** single-tier LoRA on `mlx-community/Qwen3-14B-4bit` (40 layers, hidden 5120, 7 dense target modules `self_attn.{q,k,v,o}_proj` + `mlp.{gate,up,down}_proj`). Tier 1 policies held (`rank=32 α=64`, epoch cap 3, early-stop `val_loss > best × 1.05` patience=2, seq 8192, LR 5e-5, seed 0, grad_checkpoint=true).
  - **What dropped:** Tier 2 × 3 family adapters (`reasoning_think`, `classify_notink`, `structured_notink`) + asymmetric quant — no dense analog (no shared expert, no routed experts, no router). Sprint D routing profiles + `quantize_asymmetric.py` + `expert_selection.py` retained in-repo as research artifacts.
  - **Dense baseline SHA pin**: `a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5` (`config.json` of `mlx-community/Qwen3-14B-4bit` snapshot `a4d9b2df59d2c150bef02fcbe0d91046b7ca33a4`). Replaces the Sprint C MoE pin `cdc167566e…` for all post-pivot training + eval scripts.
  - **MLX serving port moved 8100 → 8101** (CMS-pinned). Same port hosts the base model (baseline eval) + merged model (post-tune eval + live inference) via model swap.
  - **Env vars added** to `.env` + `.env.example`: `MLX_BASE_MODEL`, `MLX_BASE_MODEL_PATH`, `MLX_BASE_MODEL_CONFIG_SHA256`, `MLX_MERGED_MODEL`, `MLX_MERGED_MODEL_PATH`. Sprint FT-LORA-E knobs (`ROUTER_AUX_LOSS_COEF`, `LORA_TIER2_*`, `ASYMMETRIC_QUANT_*`) annotated as no-ops on the dense path (preserved for MoE-era provenance).
  - **Post-pivot sweep (9 files, no behavior change):** `neural/training/{train_ft.py, evaluate_ft.py, teacher_distill.py, distill_driver.py, quantize_deploy.py, profile_expert_routing.py}` + `scripts/{sprint_e_e2e_dry_run.sh, test_vllm_mlx.py}` + `.env.example`. Dense paths, port 8101, and MoE-abandoned banners added throughout.
  - **Storage hygiene**: ~51.5 GB reclaimed at sprint close — 29 intermediate LoRA checkpoints + Iter 3000 backup (~14.5 GB), archived MoE attempts (6.3 MB), abandoned HF cache MoE bases (`Qwen3.6-35B-A3B-4bit` 19 GB + `Qwen3.6-35B-A3B-mxfp4` 18 GB). Preserved: `Qwen3-14B-4bit` HF cache, `.local-models/qwen3-14b-mdemg-v1/`, `adapters/tier1/adapters.safetensors` (Iter 2400 best).
  - **Docs:** `docs/development/ft-lora/sprint_plan_ft_lora_phase5.md` (planning plan + pivot amendment header), `phase_5_sft_post.md` (10-section post-run report), `phase_5_sft_summary.md` (process / findings / current state / testing & benchmarking / risks & opportunities), `00_README_v2.md` v5.5 → v5.6, `03_IMPLEMENTATION_PLAN_v2.md §Phase 5` EXECUTED marker with pivot note, `AGENT_HANDOFF.md` top entry.
  - **Memory rule update**: `memory/project_phase5_moe_pivot.md` amended — MoE path now marked permanently abandoned per user directive 2026-04-23.
  - **Sprint chain:** A (#335) → B (#336) → C (#338/#339/#340) → D (#343) → E (`14cd2b3`) → DATA (#346 `234baec`) → **PHASE5 (this commit)** → Phase 10 benchmark (unblocked).

- **FT-LORA-DATA: Phase 5 dataset curation — re-curated + synthesized + balanced + split + pinned** (2026-04-22) — Last pre-Phase-5-SFT sprint. Produces 4 training-ready datasets Phase 5 SFT consumes directly: `training_data/sft/{tier1, family_reasoning_think, family_classify_notink, family_structured_notink}/{train,valid}.jsonl + manifest.json`. Row totals: **tier1 3,500** (3,150 train + 350 valid; all 16 tasks balanced) + **T-family 1,700** (1,530 + 170; 6 non-ape × 200 + ape.reflect × 500 per D-X5 diversity audit) + **C-family 1,200** (1,080 + 120) + **J-family 600** (540 + 60). Post-run pre-flight verdict **CLEAR** vs baseline BLOCKED (`aaa646e`). Aggregate OpenAI teacher spend ~$0.35–0.50 (3 × 200 rows × `gpt-5.4-mini`), well under $100 hard abort cap. **Phase 5 SFT now unblocked.**
  - **`neural/training/recurate.py`** — provenance-preserving re-curation with raw-SHA pin assertion (`7caebf75fd59da37221acef887dc822ac9b80d04e19c19b750dd9a4e5eceb988`). Preserves `task_name`, `trace_id`, `instance_id`, `space_id`, `dataset_ver`, `quality`, `quality_source`, `source_path` on every row. Quality filter default ≥ 0.6. SHA drift aborts with field-level diff — output `training_data/curated/sft_interactions_v2/recurated.jsonl`.
  - **`neural/training/distill_driver.py`** — mixed-teacher orchestrator thin over `teacher_distill.py`. Routes 3 OpenAI-teacher tasks (`consulting.synthesis`, `metalearn.generalize`, `hidden.summarize`) through `gpt-5.4-mini` and 2 MLX-teacher tasks (`retrieval.rerank_nli`, `summarize.generate`) through Qwen3.6 MLX local (pinned to `http://127.0.0.1:8101/v1`, single-instance enforced via `ps -Ao pid,command` pre-flight). `--budget-cap-usd 100.00` hard abort (per user 2026-04-22: "If cost will exceed $100 I want to know, otherwise dont worry about the llm call budget"). **Epic 6.0 observability hardening**: per-row structured log with `fout.flush()` after each successful write, endpoint pre-flight `GET /models` health check, response-payload capture on failure to `_debug.jsonl`, HTTP retry policy with exponential backoff (1s/2s/4s × 3 attempts on timeout/5xx/429), `--count N` override for smoke tests, `--strict` abort-on-first-error mode, `synthesis_version` stamped per-row as `v1-{commit_sha_short}` with no retroactive relabeling. `metalearn.generalize` rows carry `weak_signal: True`.
  - **`neural/training/balanced_sampler.py`** — per-tier pre-processing sampler (Appendix A Path A; NOT a mlx_lm patch). Upsample via integer duplication / downsample via seeded `random.sample` (seed=42) / passthrough. Duplication ceiling 5× (highest actual: `jiminy.evaluate_llm` at 4.348× — 45 rows → 200 floor, inside ceiling). `TIER_TASKS` mapping embedded as single source of truth (tier1=16 tasks; T=7 including ape.reflect; C=6; J=3). Determinism gate: same seed + same inputs → identical output SHA256 from balanced_sampler onward (Epic 2 synthesis non-deterministic at teacher temp=0.7).
  - **`neural/training/stratified_split.py`** — 90/10 stratified splitter per task (min 2 valid rows when count ≥ 20, else count-wise) + SHA256-stamped `manifest.json` writer pinning `generator_sha`, `trained_against_model_sha` (Sprint C `cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734`), `raw_dataset_sha_pin`, per-task train/valid counts, `source_composition` (2,501 real + 599 synth-gpt-5.4-mini + 400 synth-qwen3.6-local for tier1), `duplication_factors`, seed, `file_sha256`, `ults_spec_versions`, `synthesis_version` (`v1-aaa646e`), and a non-determinism note on the teacher-temp pathway.
  - **Tests (3-tier):** `test_recurate.py`, `test_distill_driver.py`, `test_balanced_sampler.py`, `test_stratified_split.py` (unit); `test_data_pipeline_integration.py` (integration against synthetic 16-task corpus, ~500 rows); `scripts/sprint_ft_lora_data_e2e.sh` (E2E, includes early mlx_lm fixture dry-run); `test_distill_driver_live.py` (live MLX integration, gated on `MDEMG_LIVE_MLX=1`).
  - **mlx_lm loader validation:** `mlx_lm.lora` has NO `--dry-run` flag in 0.31.2 (plan called for one); instead used `mlx_lm.tuner.datasets.load_local_dataset` (the authoritative loader path `mlx_lm.lora` uses internally) with a FakeTokenizer. All 4 datasets load cleanly. `meta` field is embedded in row JSON — mlx_lm accepts extra top-level keys; sidecar fallback unnecessary.
  - **Durable MEMORY guardrails added** (both forcing functions from Epic 6 incidents this sprint):
    - `feedback_min_max_tokens_3000.md` — never set `max_tokens < 3000` on any LLM call. Truncation is catastrophic: the partial response is discarded after the cost is already spent. Initial `summarize.generate` run truncated at `max_tokens=1500` (think_mode=True + long JSON schemas compete for token budget); bumped to 3000 and re-ran 200/200 clean (100% valid-rate).
    - `feedback_min_latency_budget_15000.md` — never set `latency_budget_ms < 15000` on any LLM call. On 2026-04-22 observed Qwen3.6 think_mode elapsed_ms up to 11,506ms on valid responses; a 10000ms budget would have dropped them as spurious timeouts. OpenAI gpt-5.4-mini responses sit at ~1-2s so 15s is a wide margin for both pathways. For think-mode with complex JSON schemas, 20000-30000ms is prudent.
    - 16 ULTS specs at `docs/tests/ults/specs/*.ults.json` and code-fallback defaults in `neural/training/*.py` audited + bumped to the new floors.
  - **Docs:** `docs/development/ft-lora/sprint_plan_ft_lora_data.md` (copied from planning-phase plan with raw SHA pin populated at Epic 1 Step 0); `docs/development/ft-lora/phase_5_dataset_preflight_post.md` (hand-written, mirrors baseline structure, verdict CLEAR); `00_README_v2.md` v5.4 → v5.5 (new changelog block + Document Map rows 14/15/16); `03_IMPLEMENTATION_PLAN_v2.md §Phase 5` (Sprint E's file-based `--dataset` cheat-sheet superseded by authoritative directory-based `--data` cheat-sheet; new §5.X-Data section; baseline+post pre-flight cross-links); this CHANGELOG entry + `AGENT_HANDOFF.md` entry.
  - **Sprint chain:** A (#335) → B (#336) → C (#338/#339/#340) → D (#343) → E (`14cd2b3`) → **FT-LORA-DATA (this commit)** → Phase 5 SFT (unblocked) → F (Gate 3 middle band not triggered, F inactive).

- **FT-LORA-E: Training infrastructure patches — tier-aware CLI, asymmetric quant, early-stop** (2026-04-22) — Last pre-Phase-5 sprint. Makes `neural/training/train_ft.py` speak the MoE-Sieve two-tier dialect; Phase 5 SFT unblocked on merge. **No training launches** — gate is dry-run of Phase 5's full invocation matrix (1 Tier 1 + 3 Tier 2 families + 1 asymmetric-quant classification). 7 sequential epics; 94 tests passing + 1 skipped; E2E dry-run script exits 0.
  - **`neural/training/train_ft.py` — tier-aware CLI** (13 new flags): `--tier {1,2}`, `--family {reasoning-think,classify-notink,structured-notink}`, `--expert-selection-path`, `--rank`/`--alpha` (tier-defaulted 32/64 T1, 8/16 T2), `--target-modules` (CSV override), `--router-aux-loss-coef` (default 0.002), `--n-epochs` (rejects `auto`; enforces `LORA_N_EPOCHS_CAP` — does NOT silently clamp), `--early-stop-ratio`/`--early-stop-patience`, `--mode {sft,rl}`, `--base-adapter`, `--expected-sha256` (required for BOTH tiers — deliberate symmetry to catch stale config.json drift), `--dry-run`. Existing `validate_manifest` + `load_manifest` behavior preserved for the legacy symmetric-quant path.
  - **`router_aux_loss_coef` injection — dual-path with signal-safe fallback**: primary YAML via `--config` attempted + grep-verified; fallback atomic copy-on-write `config.json` replacement with SIGTERM/SIGINT/SIGHUP handlers + `atexit`, SHA re-match on every exit path, `.pre-train-backup` cleanup only after verified restore. Fallback is ALWAYS installed regardless of primary-path verification — training crashes cannot leave the base model drifted.
  - **`neural/training/expert_selection.py`** — Sprint D profile loader. `ExpertSelection.load(path, expected_family)` validates schema + family + Sprint C config-SHA pin; `mlx_lm_keys()` emits exactly **40 layers × 64 experts × 3 projs = 7,680** module-path strings for mlx_lm's `linear_to_lora_layers` `keys` filter. Format: `language_model.model.layers.{L}.mlp.experts.{E}.{down|up|gate}_proj`. Normalizes hyphen ↔ underscore family names.
  - **`neural/training/quantize_asymmetric.py`** — `build_asymmetric_predicate()` returns an mlx_lm-compatible `quant_predicate` callable. Routed experts → `{"group_size": 32, "bits": 4, "mode": "mxfp4"}`; attention + shared-expert + router → `False` (keep BF16). `experts.N.gate_proj` correctly classified ROUTED (not confused with router `mlp.gate`). CLI wrapper with `--dry-run` summary mode for classification inspection; full conversion deferred to Phase 5.
  - **`neural/training/early_stop.py`** — Subprocess stdout monitor wrapping `mlx_lm.lora`. SFT regex `r'Val loss (\d+\.\d+)'`; fires on `val_loss > best × 1.05` for 2 consecutive evals (patience=2). RL regex `r'Val reward (\d+\.\d+)'` wired but unvalidated (Sprint E scope = SFT only). Orphan-checkpoint handling: `.partial` staging path + `.earlystop.json` sidecar written on fire (records trigger_line, val_loss_history, stopped_at_iter). Checkpoint-behavior empirical verification (best-vs-current adapter) deferred to Phase 5 runbook — Sprint E gate is dry-run only, no training launched.
  - **11 env vars activated** (uncommented) in `.env.example:415-429`, `docker-compose.yml:109-120`, `internal/cli/compose_templates/docker-compose.yml:109-120`: `ROUTER_AUX_LOSS_COEF=0.002`, `LORA_TIER1_RANK=32`, `LORA_TIER1_ALPHA=64`, `LORA_TIER2_RANK=8`, `LORA_TIER2_ALPHA=16`, `LORA_N_EPOCHS_CAP=3`, `LORA_EARLY_STOP_SFT_THRESHOLD=1.05`, `LORA_EARLY_STOP_RL_THRESHOLD=0.95`, `ASYMMETRIC_QUANT_SHARED=bf16`, `ASYMMETRIC_QUANT_ROUTED=mxfp4_moe`, `ASYMMETRIC_QUANT_ATTN=bf16`. (Plan said "10 vars × 4 files"; reality 11 vars × 3 files — `docker-compose.dev.yml` is an overlay-only sidecar file with no server `environment:` section. Discrepancy documented in Sprint E Post-Execution Notes.)
  - **Bug fix in `build_mlx_lm_command`**: removed non-existent `--lora-layers` flag (mlx_lm 0.31.2 exposes only `--num-layers`). Caught during Tier 1 dry-run verification.
  - **Tests**: 3 new Tier-1 unit test files (`test_expert_selection.py` 18 tests, `test_quantize_asymmetric.py` 15 tests, `test_early_stop.py` 18 tests) + `test_train_ft.py` rewritten to 38 tests preserving legacy manifest coverage and adding tier-aware CLI surface coverage (epoch-cap rejection not clamp, SHA drift abort on BOTH tiers, `--target-modules` CSV override, tier-2 7,680-key emission, `--base-adapter` argv threading). 1 new Tier-2 integration test file (`test_train_ft_integration.py`, 6 tests, gated on Sprint C model availability + `SPRINT_E_HEAVY_INTEGRATION=1` for full `named_parameters()` structural match). 1 new Tier-3 E2E shell script (`scripts/sprint_e_e2e_dry_run.sh`) exercising 5-step invocation matrix. Total: **94 passed + 1 skipped**.
  - **Sprint chain**: A (#335) → B (#336) → C (#338/#339/#340) → D (#343) → **E (this commit)** → Phase 5 SFT (unblocked).

### Removed

- **FT-OAI-003 calibration run scope dropped** (2026-04-22) — Per user direction, no OpenAI fine-tuning deliverables going forward. All fine-tuning work now targets local MLX LoRA on Qwen3.6-35B-A3B (Sprint C validated, all 3 gates green). OpenAI API remains usable as a *teacher* for synthesis only (see FT-LORA-DATA: 3 × 200 rows × `gpt-5.4-mini`, ~$0.35–0.50), never as a fine-tuned-model deliverable. `memory/project_ft_oai_003_deferred.md` flagged for removal post-sprint-merge.

### Changed

- **FT-LORA-C Gate 3 execution result — PASS (both sub-gates)** (2026-04-22) — Throughput + benchmark parity vs hosted `gpt-5.4-mini` validated on M5 Max. Sprint C now fully green across all three gates; FT-LORA line unblocked through Sprint D; Phase 5 SFT pre-Gate-3 blocker cleared. **Throughput sub-gate:** median **126.65 tok/s** over 5×500-token measurements at canonical T recipe (`temp=0.6, top_p=0.95, top_k=20`) after a discarded 200-token warm-up — 2.1× the ≥60 tok/s floor; per-run spread 122.75 / 126.65 / 128.91 / 124.38 / 126.74 (mean 125.85; all `finish_reason=length` at exactly 500 tokens). **Quality sub-gate: `clear_pass` band — gap = −13.83%** (Qwen3.6 **ahead** of gpt-5.4-mini on the 120-question whk-wms benchmark). Qwen mean score **0.0672** (std 0.0279) vs gpt-5.4-mini mean **0.0591** (std 0.0247). Per-question: **Qwen 78 wins / gpt 42 wins / 0 ties**. Per-category (all 8 Qwen-ahead): architecture_structure −13.05%, business_logic_constraints −13.01%, computed_value −17.59%, cross_cutting_concerns −23.76%, data_flow_integration −15.67%, disambiguation −1.31%, relationship −4.34%, service_relationships −13.50%. **Path A deviation — runbook-sanctioned drift-catch.** Pre-flight `--help` on `run_benchmark_v4.py` surfaced the architectural mismatch the runbook anticipated: `--model` accepts only `sonnet|opus|haiku` and synthesis is via `subprocess.run(["claude", "--model", ..., "--print", ...])` at `answer_generator.py:311` — no `LLM_BASE_URL` / `LLM_MODEL` env hook. Path A bypasses `run_benchmark_v4.py` entirely via a ~220-line direct driver that POSTs to mlx_lm.server :8200 (Qwen) and `api.openai.com/v1/chat/completions` (gpt-5.4-mini), emitting `answers_*.jsonl` in the shape `grader_v4.py` expects. Scope excludes MDEMG retrieval + Claude-agent synthesis + file citations (absolute scores ~0.06 on both sides are an expected artifact of the symmetric handicap; the gap between backends remains meaningful). **Sprint F (post-SFT MDEMG-wrapped eval) is the complementary test** measuring end-user-observable quality of the full pipeline. Deviation recorded at `~/.mdemg-sprint-c/gate3/deviation.md` with explicit Reason / Scope / Relationship-to-Sprint-F / Budget-impact sections. **OpenAI spend: $0.021** against the $25 cap (10× below the $0.22 tiktoken pre-estimate; 11,211 prompt + 14,689 completion tokens). Mid-execution fix: initial run hit 120/120 HTTP 400 `"Unsupported parameter: 'max_tokens'"` — gpt-5.x Chat Completions requires `max_completion_tokens`; `build_body()` now dispatches by backend (qwen uses `max_tokens` + `temp`/`top_p`; openai uses `max_completion_tokens` and omits sampling params per reasoning-series contract). **Same-wall-clock-window rule** satisfied: Qwen 2026-04-22T07:36:19Z, gpt-5.4-mini 2026-04-22T07:40:57Z → 4.6 min apart (well inside 24h). Cross-gate model SHA consistency: all three gates ran against `cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734`. Stamp: `~/.mdemg-sprint-c/gate3/passed_20260422T074500Z.json` (execution-time artifact). **Gate 3 middle band NOT triggered** → Sprint F remains a registered-but-inactive safety checkpoint; no `middle_band_*.json` stamp. Sprint chain: A (#335) → B (#336) → **C (all 3 gates pass, #338 / #339 / #340 + this commit)** → D (expert activation profiling) → E (training infra patches) → Phase 5 SFT.

- **FT-LORA-C Gate 2 execution result — PASS** (2026-04-22) — J-group structured-output correctness validated on 100 synthetic prompts across all 4 J-group tasks (`hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`, `guardrail.evaluate`, 25 each) at the canonical J recipe `temp=0.7, top_p=0.95, top_k=20, presence_penalty=1.5, max_tokens=4096`. **Tier-1 JSON-parse rate: 100.0% (100/100)**, Tier-2 shape-match rate 100.0% (100/100), vs ≥95% acceptance criterion. Zero errors across a 14.0 min wall-clock run. Per-task latency means (sec): hidden.name_emergence 12.68, jiminy.evaluate_llm 5.65, retrieval.rerank_cross 8.18, guardrail.evaluate 7.09 — all comfortably within the latency budgets declared by the ULTS specs. Fallback 12-cell pp×temp sweep not triggered. Supplementary 20-prompt prod-config test (`max_tokens=2048`): tier-1 95% (19/20), with a 5.0% `production_config_cutoff_rate` concentrated on `hidden.name_emergence` (1/5 at seed 3). Exactly at the 5% flag threshold — Sprint-E signal: either raise J-group `max_tokens` to 4096 OR tighten `<think>` discipline during SFT. Gate-2 deviations recorded in the stamp: (a) `mlx_lm.server` used vs runbook-prescribed `vllm-mlx` (identical OpenAI `/v1/chat/completions` shape; Docker shadowed 8100/8101 so server bound to :8200 — no impact); (b) **ULTS spec drift finding** — `docs/tests/ults/specs/jiminy_evaluate_llm.ults.json` `output_schema` requires `{evaluations:[...]}` but the actual `evalSystemPrompt` (`internal/jiminy/eval_prompt.go:31`) asks the model for `{violations, warnings}`. Grader validated against the system-prompt shape (production truth); the ULTS spec is drifted and needs a doc-only cleanup in a follow-up. Stamp: `~/.mdemg-sprint-c/gate2/passed_20260422T041105Z.json` (execution-time artifact). Prompt set SHA `b4dba3c6…`; default-recipe responses SHA `322726bb…`; prod-2048 responses SHA `682ba6b0…`. Gate 3 now unblocked (contiguous ~half-day session required per `project_sprint_c_gate3_contiguous.md`).

- **FT-LORA-C Gate 1 execution result — PASS** (2026-04-22) — Qwen3.6-35B-A3B MLX asymmetric-quant load validated on M5 Max. Model: `mlx-community/Qwen3.6-35B-A3B-mxfp4` (Path A with deviation — attention quantized MXFP4 rather than BF16 per memo §3.8; flagged to Sprint E `mlx_lm.convert` per-module patch). All 4 numeric acceptance criteria met: peak RAM **19.03 GB** (ceiling 24 GB, band=normal), cold-load wall time **3.34 s** (ceiling 90 s — 27× headroom), forward-pass **0.84 s** (ceiling 30 s), `LOADED_OK` + `FORWARD_OK` both present (exit 0). Stamp: `~/.mdemg-sprint-c/gate1/passed_20260422T034121Z.json` (execution-time artifact). Model config SHA: `cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734`. Venv `~/.venv/mdemg-ft-lora` (Python 3.12.13, mlx_lm 0.31.2, hf_hub 1.11.0). Hardware: M5 Max, 128 GB unified, macOS 26.3.2, internal Apple Fabric NVMe. Calibration note: 3.34 s cold-load is ~15× below the ~52 s estimate — recorded as telemetry for Gates 2/3 budget sizing. Gate 1 → Gate 2 unblocked.

- **FT-LORA-C: Qwen3.6-35B-A3B MLX Validation Runbook (planning-only)** (2026-04-21) — Planning-only sprint. Deliverable is a single runbook doc (`docs/development/ft-lora/sprint_plan_ft_lora_c.md`, ~14 pages) designed to be executed interactively by a future Claude Code session; survives arbitrary pauses (including week-long inter-gate pauses) via disk-stamp resume protocol under `~/.mdemg-sprint-c/gateN/`. Zero execution artifacts this sprint ($0 spend).
  - **Gate 1 (MLX asymmetric-quant load)** — 4 numeric acceptance criteria: no-OOM under `mlx_lm.load`, peak RAM ≤24 GB (24-30 GB = flag with Sprint-E-recalibration note, >30 GB = halt), first-load time ≤90 s from **cold page cache** (SSD-tier-normalized: M5 Max internal NVMe baseline; USB 3.2 ≤180 s / halt 360 s; TB3/4 ≤135 s / halt 270 s; warm-load reload ~10-20 s is **not** gate-qualifying), forward-pass ≤30 s on a 512-token probe. Path options A=published asymmetric quant if available, B=`mlx_lm.convert` per-module selector attempt, C=symmetric 4-bit fallback with Sprint-E deviation recorded. Fail → halt FT-LORA line; replan Sprint C' with Qwen3.5-35B-A3B base.
  - **Gate 2 (J-group structured-output correctness)** — ≥95% JSON validity on 100 synthetic prompts (25 per J-group task: `hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`, `guardrail.evaluate`) at the canonical J recipe `temp=0.7, top_p=0.95, top_k=20, presence_penalty=1.5, **max_tokens=4096**` — **deliberate deviation** from memo §3.3's production 2048 ceiling to isolate sampling-param effects from truncation effects during the sweep. Supplementary post-sweep 20-prompt re-test at `max_tokens=2048` records non-blocking `production_config_truncation_rate` — flagged to Sprint E for memo §3.3 revision review if >5%. Fallback sweep matrix concentrates on the two most relevant levers: `presence_penalty` (5 levels: 1.0, 1.25, 1.5, 1.75, 2.0) × `temperature` (2 levels: 0.5, 0.7) + 2 control cells (`no_chat_template`, `json_mode_on`); 12 cells total, ~7.5 h single-session budget. Best cell ≥95% → pass with recipe-update flag to Sprint E; 90-94.9% or <90% → halt.
  - **Gate 3 (throughput + quality parity vs `gpt-5.4-mini`)** — throughput sub-gate: ≥60 tok/s median on M5 Max over 5×500-token runs at canonical T recipe (halt if <60). Quality sub-gate: 120-question benchmark (`test_questions_120.json`) with explicit disposition bands — ≤10% gap from baseline = clear pass, 10-30% gap = **middle band** (stamp `middle_band_*.json`, proceed to Sprints D/E, defer commit-or-fallback decision to new **Sprint F** post-SFT), >30% gap = catastrophic halt. **Hard $25 budget cap** for baseline capture: pre-run `tiktoken` estimation pauses if estimate >$25; cumulative spend tracked in `~/.mdemg-sprint-c/gate3/openai_cost.json` and aborts mid-run on cap breach. **24h same-window constraint**: baseline capture + Qwen3.6 runs must pair within a rolling 24h window (OpenAI silent model-revision drift mitigation); if the pair exceeds 24h, baseline re-capture required. Inter-gate pauses of arbitrary length remain supported — the 24h window applies only to the two halves of Gate 3 itself.
  - **Sprint FT-LORA-F registered** (new, skeleton only) — post-SFT commit-or-fallback checkpoint. Triggered only if Gate 3 leaves a `middle_band_*.json` stamp. Decision rules: ≤10% gap post-SFT → commit Qwen3.6 for MDEMG production; 10-30% → Qwen3.5 fallback; >30% → halt the FT-LORA line and revisit strategy. Full 12-section plan drafted at Sprint F start (after Phase 5 SFT completes).
  - **Mid-sprint user checkpoint** passed after plan draft review; 4 edits applied (sweep matrix reshaped from pp×top_p to pp×temp, $20→$25 cap, 24h same-window constraint added, load-time SSD-tier + first-load/warm-load clarification) + max_tokens=4096 Gate 2 deviation.
  - **Sprint chain**: A (#335, done) → B (#336, done) → **C (this plan, to commit)** → D (expert activation profiling) → E (training infra patches) → Phase 5 SFT → F (commit-or-fallback — only if Gate 3 middle band).

- **FT-LORA-B: Code/config alignment + guardrail llmclient migration + ULTS `sampling_group`** (2026-04-21) — Code-side execution of what Sprint A queued. 7 sequential epics; mid-sprint user checkpoint after Epic 3 passed.
  - **Guardrail consumer migration**: `internal/guardrail/llm_evaluator.go` now routes through `llmclient.Completer` (both OpenAI + Ollama direct-HTTP branches removed, ~180 lines). Interactions are captured as `task_name='guardrail.evaluate'` in the `llm_interactions` TSDB hypertable when `GUARDRAIL_ENABLED=true`. New exported `llmclient.Completer` interface + `llmclient.TestClient{CompleteFn}` stub (`internal/llmclient/testclient.go`) for unit-testability. `NewGuardrailService` signature gains an `llm llmclient.Completer` parameter; `internal/api/server.go` instantiation builds the llmclient from `GuardrailConfig` and calls `.WithContext(guardrail.TaskName, "")`. New constant `guardrail.TaskName = "guardrail.evaluate"` links Go code to the ULTS spec.
  - **ULTS schema adds required `sampling_group` enum (T/C/J)** per memo 07 v3.1 §3.3. All 16 existing canonical specs + new `guardrail_evaluate.ults.json` (17th) carry the field. Runner (`docs/tests/ults/runners/ults_runner.py`) gained `parity_sampling_group` check with actionable errors on missing/invalid values. T=7 / C=6 / J=3 counts verified. Future ULTS specs without `sampling_group` will fail runner validation.
  - **New ULTS spec (17th)**: `docs/tests/ults/specs/guardrail_evaluate.ults.json` — sampling_group=J (structured JSON output with `presence_penalty=1.5`), think_mode=false, system_prompt_hash captured as `[full, compact]` array, quality_metrics = json_validity / violation_recall / false_positive_rate. **Excluded from Phase 5 SFT training target** (remains 16 tasks × 500–1000 anchor examples); guardrail is opt-in / disabled by default. See footnote at `docs/development/ft-lora/01_RESEARCH_v2.md §1.1`.
  - **15 files refreshed** from `Qwen3-30B-A3B` → `Qwen3.6-35B-A3B` per memo 07 v3.1 (8 docs + 7 code/config). HF sanity check at execution time confirmed `mlx-community/Qwen3.6-35B-A3B-4bit` is published (2026-04-16, 34.8k downloads) — used `-4bit` suffix, not the preliminary `-Q4` guess. **Functional change**: `scripts/test_vllm_mlx.py:236` argparse default updated (affects runtime when `$LLM_MODEL` unset). Output-path examples (`mdemg-qwen3-30b-v1-q4/`) preserved with Sprint-E TODO comments; `docs/operations/vllm-mlx-setup.md` memory table carries Sprint-E TODO for asymmetric-quant recalibration.
  - **.env.example + both compose files in parity** gained `GUARDRAIL_TASK_NAME=guardrail.evaluate` (uncommented) + 10 commented Sprint-E placeholder knobs (`ROUTER_AUX_LOSS_COEF`, `LORA_TIER1/2_{RANK,ALPHA}`, `LORA_N_EPOCHS_CAP`, `LORA_EARLY_STOP_{SFT,RL}_THRESHOLD`, `ASYMMETRIC_QUANT_{SHARED,ROUTED,ATTN}`). Seeds naming convention for Sprint E; current code ignores them.
  - **Tests**: 5 new unit tests in `internal/guardrail/llm_evaluator_test.go` (messages/opts wiring, compact prompt toggle, nil-client error, error propagation, TaskName constant). New build-tagged integration test `tests/integration/guardrail_tsdb_capture_test.go` (Tier 3 E2E) gated on `TEST_GUARDRAIL_LIVE=1` — verifies `POST /v1/memory/guardrail/validate` lands a row in `llm_interactions` with `task_name='guardrail.evaluate'`.
  - Tracked as tasks #165–#172.

### Breaking changes (admin surface)

- **Circuit breaker rename (guardrail) — hard cutover.** Old names `openai-guardrail` / `ollama-guardrail` are **removed**; new names `openai-guardrail.evaluate` / `ollama-guardrail.evaluate` registered in their place (per-task naming convention matching the ULTS task). `POST /v1/admin/breakers/reset` with `{"name":"openai-guardrail"}` now returns 404 `"unknown breaker"`. **Operators**: call `GET /v1/admin/breakers` first to discover current names, then reset by the enumerated name. Grafana dashboards and alerting rules referring to the old names must be updated. Blast radius is limited: admin endpoint is gated by `AUTH_API_KEYS`; `GUARDRAIL_ENABLED=false` by default, so most deployments are unaffected.

- **FT-LORA-A: ft-lora plan suite aligned to memo 07 v3.1** (2026-04-21) — docs-only sprint (zero code / zero behavior change) propagating three locked-in decisions across 14 files: base model Qwen3-30B-A3B → **Qwen3.6-35B-A3B** (fallback Qwen3.5-35B-A3B; Apache 2.0 2026-04-16; 35B/3B-active MoE, 256 experts = 8 routed + 1 shared, 262K native context, MTP speculative decoding); **no-tool-calling architectural policy** (all 16 MDEMG LLM call sites are single-shot; 9 banned patterns including `preserve_thinking`); **two-tier MoE-Sieve LoRA** (Tier 1 attention + shared expert r=32 α=64 all tasks balanced; Tier 2 top-25% routed experts r=8 α=16 per-family reasoning-think/classify-notink/structured-notink; `router_aux_loss_coef=0.002`; asymmetric quant shared BF16 / routed MXFP4_MOE / attention BF16). Files updated: `docs/development/ft-lora/{00_README,01_RESEARCH,02_M5MAX_HARDWARE,03_IMPLEMENTATION_PLAN,04_BENCHMARK_RL,05_DATA_COLLECTION,06_CORRECTIONS_APPLIED}_v2.md`, `docs/development/ft-lora/ft-lora-dev/{MDEMG_FT_PLAN_DEEP_DIVE_ANALYSIS,SPRINT_EMBEDDING_DATA_COLLECTION}_v2.md`, `docs/development/UXTS_FRAMEWORK_MATRIX.md`, plus new `docs/development/ft-lora/{SPRINT_A_GREP_AUDIT,sprint_plan_ft_lora_a}.md`; repo-level `VISION.md` / `CLAUDE.md` / `AGENT_HANDOFF.md`. **Two planner-introduced engineering policies** (flagged for audit trail, not in memo): (1) epoch cap 3 per tier with early-stop on `val_loss > best × 1.05` for 2 consecutive evals (SFT) / `val_reward < best × 0.95` (RL); (2) `n_epochs=auto` disallowed on all LoRA runs. Forcing function: FT-OAI-001 overfit at step 1200. Sprint sequence: **A (docs, done) → B (code/config) → C (Qwen3.6 MLX validation) → D (expert profiling) → E (training infra patches)** — Phase 5 SFT unblocks only after Sprint C passes.

### Added

- **FT-LORA-D: Expert activation profiling + family partition decision** (2026-04-22) — Read-only sprint that produces the inputs Sprint E Tier 2 LoRA needs.
  - **`neural/training/profile_expert_routing.py`** — MoE router capture via context-manager class-level monkey-patch of `Qwen3NextSparseMoeBlock.__call__`. Single-pass inline forward (no double-compute; monkey-patch restored in `finally`). Captures `inds` (top-k expert indices) + `scores` (routing weights) per token per layer. Full-generation-mode profiling using `mlx_lm.stream_generate` — captures both prompt-side and generated-token routing. SHA256 guard on `config.json` against Sprint C pin (`cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734`); aborts on mismatch. Deterministic tie-break (sort by `(-count, expert_idx)`). CLI flags: `--model-path --expected-sha256 --anchor-prompts --output-dir --top-pct --max-prompt-tokens --max-gen-tokens --limit --task --force --verbose`.
  - **`neural/training/sprint_d_analyze.py`** — analyzer that consumes the 3 family artifacts + raw counts, applies the 0/1/≥2 threshold rule at 0.80 Jaccard, computes per-family within-family task-cohesion Jaccard, runs hierarchical single-linkage agglomerative clustering on split-candidate families, and emits the decision doc.
  - **Artifacts** under `training_data/routing_profiles/`:
    - `profile_routing_reasoning_think.json` / `profile_routing_classify_notink.json` / `profile_routing_structured_notink.json` — per-family top-64-of-256 expert mask per layer (40 layers), consumed by Sprint E Tier 2 training via `--expert-selection-path`.
    - `raw_activation_counts.json` — per-(task, layer, expert) counts for post-hoc analysis.
    - `anchor_prompts.jsonl` (320 records, 20/task × 16 tasks; T=140 / C=120 / J=60) + `anchor_prompts.sha256` (`7eddeccafa9ffcdd4098a2f762afce6522f65b28ab96c87d5faeb96b0acd7087`). Primary source: production LLM interaction traces (221/320); backfill from same-shape donor tasks for 5 tasks with 0-1 production records (99/320) — documented deviation from the plan's whk-wms backfill, chosen to preserve task-family routing signal over category diversity.
  - **`docs/development/ft-lora/sprint_c_d_profile_results.md`** — decision doc with verdict code (3-family-confirmed / 2-family-merged-<pair> / 1-family-collapsed), cross-family Jaccard overlap table (3 pair averages + per-layer breakdown), per-family task-cohesion table (within-family pairwise Jaccard; ≥0.70 cohesive / 0.40-0.70 ambiguous / <0.40 split-candidate), KL divergence vs uniform summary, Sprint E recommendation.
  - **`docs/development/ft-lora/sprint_plan_ft_lora_d.md`** — canonical sprint plan with Documents Accessed appendix.
  - **Tests**: 22 unit tests in `neural/training/tests/test_profile_expert_routing.py` (anchor-prompts loader, top-k aggregation, top-pct mask + tie-break + KL=0 for uniform, Jaccard overlap, task-cohesion verdicts, hierarchical clustering boundary, SHA256 guard). Tier 2 integration: `--limit 10` on real model validates artifact schema + monkey-patch cleanup. Tier 3 E2E: full 320-prompt run; determinism confirmed **bit-identical** for MLX + mxfp4 (A/B-run SHA256 match, 1.0 Jaccard across all 40 layers — strict determinism-check branch).
  - **Bimodality coefficient rejected** as methodologically unsound for 256-bin discrete distributions with n=60-140; replaced with direct task-cohesion analysis on raw per-(task, layer, expert) counts. Rationale recorded in `03_IMPLEMENTATION_PLAN_v2.md §5B`.
  - **Sprint E unblocked**; family partition verdict recorded in commit body + PR summary.

- **FT-OAI-001: OpenAI Fine-Tuning Pipeline** (2026-04-21) — First in-house fine-tune closes the write→train→eval→promote loop for hosted models:
  - **New post-processor**: `neural/training/openai_ft_adapter.py` — converts MLX chat JSONL (from `mdemg data curate --paradigm sft`) into OpenAI-shaped `combined_{train,val}.jsonl` plus `manifest.json` + `rejection_log.jsonl`. Strips `<think>` blocks, validates message schema, counts tokens via `tiktoken.encoding_for_model()` (resolves to `o200k_base` for gpt-4o/4.1 families — **not** hard-coded `cl100k_base`), gates per-record context at 65,536 tokens, emits optional per-task specialists under `by_task/`. Does not duplicate the temporal split — remains a pure post-processor on `dataset_versioner.py` output.
  - **New tooling**: `scripts/openai_ft_upload_and_launch.py` (cost-gated via `--max-cost-usd` before any network call), `scripts/openai_ft_check.py` (job poller with `--watch`), `scripts/openai_ft_baseline_eval.py` (seeded deterministic eval — `--seed` + `--sample-size` produce identical records across base and FT runs), `scripts/openai_ft_compare.py` (side-by-side comparator → `eval_comparison.md` with per-task W/L/T, 5 worst regressions, 5 best gains).
  - **First FT delivered**: `ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai001:DX9KJuuq`. Training: 31M tokens, 3 epochs (auto), final train loss 0.623, best val loss 0.684 at step 1200/1500.
  - **Held-out eval vs training-base `gpt-4.1-mini`** (300 records, seed=42, 10 tasks): mean cosine +0.032 (0.832 → 0.864), parse-pass rate unchanged at 0.973, W/L/T = 133/17/150 (7.8:1 win ratio). Verdict (in-frame): **MARGINAL**. Per-task gains: `hidden.reclassify` +0.087, `retrieval.rerank_cross` +0.075, `consulting.classify` +0.057. Flagged regression: `retrieval.intent_translate` −0.079 (n=4).
  - **Cross-base bench vs production base `gpt-5.4-mini`** (2026-04-21, same 300-record seeded sample, same 4096 cap): stock `gpt-5.4-mini` scored mean cosine **0.898** vs FT's 0.864 — quality-only Δ = **−0.034**, W/L/T = **14/61/225**. **Strategic framing (north star):** `gpt-4.1-mini` is materially cheaper per token than `gpt-5.4-mini`; the real goal is `FT(gpt-4.1-mini) ≈ prod(gpt-5.4-mini)` quality at the cheaper base's inference cost. Gap-closing progress: stock-4.1-mini 0.832 → FT 0.864 → stock-5.4-mini 0.898. **FT-OAI-001 closed ~48% of the stock-4.1-mini → stock-5.4-mini gap** (0.0319 / 0.0658). Deploy decision is deferred pending quantification of the actual `gpt-4.1-mini` vs `gpt-5.4-mini` per-token cost ratio on recent OpenAI billing — if the cost saving justifies the remaining quality gap at scale (especially for volume tasks like `ape.reflect`, 72% of traffic), the FT model deploys for those tasks while prod stays on `gpt-5.4-mini` elsewhere. For now `.env` / `.env.example` remain on `gpt-5.4-mini`. See `training_data/openai_ft/20260420/eval_comparison_vs_gpt54mini.md`. FT-OAI-003 target: close the remaining ~52% of the gap (fix `__unattributed__` training-data noise −0.113, tune `n_epochs=2` per the step-1200 best-val signal, upweight regressed tasks `retrieval.intent_translate` −0.149 / `hidden.name_emergence` −0.114, consider `gpt-4o-mini` as an even-cheaper training base).
  - **Full feature doc**: `docs/features/fine-tuning-pipeline.md` (architecture, workflow, config, known limitations, FT-OAI-002 follow-ups).
  - **Artifacts**: `training_data/openai_ft/20260420/` — `run_notes.md` (run log across 3 job attempts: quota-exceeded, queue-stuck-cancelled, succeeded), `eval_comparison.md`, `ft_training_metrics.csv`, `manifest.json`, `eval/{baseline,ft}/{results.jsonl,summary.json}`.
  - **Known gaps** (carried into FT-OAI-002): per-record `parse_ok` field always `False` in `results.jsonl` (aggregate in `summary.json` is correct), `finish_reason` always null, token counts not persisted per record, baseline/FT `--max-output-tokens` asymmetric (1024 vs 4096) due to FT response verbosity.

### Added

- **FT-OAI-002: Fine-Tuning v2 Data Capture & Harness Hardening** (2026-04-21) — executed at `docs/development/ft-oai/sprint_plan_ft_oai_002.md`. Harness + tooling + telemetry sprint — no second FT launch (deferred to FT-OAI-003 with explicit user authorisation). Addresses G1/G2/G3/R1/O1–O4 limitations carried from FT-OAI-001:
  - **Epic 1 (G1a/b/c)** — eval harness bug fixes. `scripts/openai_ft_baseline_eval.py` now writes canonical `parse_ok` alongside legacy `parses_as_json`, captures `finish_reason` from `resp.choices[0].finish_reason`, and records `prompt_tokens`/`completion_tokens` from `resp.usage`. `scripts/openai_ft_results_backfill.py` retroactively populates archived `results.jsonl` (confirmed: FT-OAI-001 baseline + FT both backfill to 0.9733 parse rate, matching `summary.json`).
  - **Epic 2 (A1–A7)** — per-record metric fields: `latency_ms` (via `time.perf_counter()`), `retry_count`, `truncation_flag` (derived from `finish_reason=="length"`), `embedding_model_version`, `request_id`, `hallucination_indicator` (fires when GT `{"type":"none","summary":""}` but response is non-none), `input_chars`, `output_chars`. `summary.json` gained `latency_stats` (mean/p50/p95/p99), `truncation_rate`, `mean_retries`, `hallucination_rate_on_none_gt`. New `call_with_retry()` wrapper makes completion-call retries explicit and countable. 21 unit tests covering the new fields, retry counter, percentile aggregates, and backfill helper.
  - **Epic 3 (G2)** — cap-symmetric baseline re-eval at 4096 tokens. Harness + command staged in `run_notes.md`; **actual run deferred** pending user auth (~$4.05 live OpenAI cost).
  - **Epic 4 (G3)** — `__unattributed__` attribution investigation at `training_data/openai_ft/20260420/unattributed_investigation.md`. Finding: the bucket is 100% recoverable by a sys-prompt-only fallback (14 unique system_prompts in `filtered.jsonl`, zero cross-task collisions, 605/605 test records resolved). Also surfaces a `consulting.classify` "should"-bias hallucination pattern (5 worst regressions all hallucinated alerts against none-typed GT). Fix deferred to FT-OAI-003 per Epic 4 gate (no mid-sprint heuristic changes).
  - **Epic 5 (R1)** — `retrieval.intent_translate` regression deep-dive at `training_data/openai_ft/20260420/intent_translate_investigation.md`. Traces Δ=−0.079 (0W/3L/1T over n=4) to under-representation (127 train records vs 28,324 for `ape.reflect`, 223× less) plus one confabulation tail (record i=252 invents "subpackage184", "Godoc style edit container signature update"). Mitigation = 8× upsample via `--task-weights`, lands in FT-OAI-003.
  - **Epic 6 (T1–T4)** — training-signal persistence + per-task weights. `scripts/openai_ft_check.py --on-complete --job-id <id>` parses OpenAI's result_file CSV into `training_metrics.json` with `best_val_loss_step`, `best_val_loss`, full per-step `train_loss_series` / `valid_loss_series`. `openai_ft_upload_and_launch.py --n-epochs <int|auto> --n-epochs-rationale "<reason>"` passes `hyperparameters={"n_epochs": int}` to the job and records the rationale in `run_notes.md`. `neural/training/openai_ft_adapter.py --task-weights '{"task.name": N}' --sys-prompt-map <path>` deterministically duplicates matching records N× via sha256(system_prompt) attribution (records with unresolved tasks get weight 1; fractional weights rejected for determinism).
  - **Epic 7 (O1–O4)** — operational telemetry. `--on-complete` writes `job_lifecycle.json` (`created_at`, `running_at` from events stream, `finished_at`, `queue_seconds`, `train_seconds`, `n_epochs_actual`). `--queue-timeout-minutes N` emits an entry to `alert.log` if queued > N min with no `running` transition — **by explicit design does not auto-cancel**, human decides. `openai_ft_upload_and_launch.py` now probes OpenAI billing API pre-upload with graceful fallback on non-admin key tiers (`--skip-quota-check` for known-unavailable tiers; `--quota-buffer 1.66` defaults to the observed FT-OAI-001 auto-epoch overshoot). Manifest `totals` now surfaces `cost_estimate_low_usd` (1 epoch) / `cost_estimate_usd` (midpoint, cap-gated) / `cost_estimate_high_usd` (3 epochs — observed auto-epoch ceiling).
  - **Epic 8** — 10-record integration smoke, staged but **deferred** pending user auth (~$0.15 live OpenAI cost).
  - **Epic 9** — documentation: `docs/features/fine-tuning-pipeline.md` gained "Resolved in FT-OAI-002" section; `neural/training/README.md` documents `--task-weights` + cost envelope; this entry; FT-OAI-002 row added to `AGENT_HANDOFF.md`.
  - **Quality headline unchanged** — no re-train: FT-OAI-001's 0.8641 mean cosine still stands. FT-OAI-002 is pure tooling + telemetry + investigation, producing inputs for FT-OAI-003. Tracked as task #142.

### Planned
- **FT-OAI-003: Close the Gap to Production Base** (north-star sprint) — plan at `docs/development/ft-oai/sprint_plan_ft_oai_003.md` (v1.0 12-section format, 10 epics + rollback). Goal: close the remaining ~52% of the stock-4.1-mini → stock-5.4-mini quality gap so `FT(cheap-base) ≈ prod(gpt-5.4-mini)` at the cheaper base's inference cost. **Prerequisites**: FT-OAI-002 landed (task #142), plus an economic analysis quantifying the actual OpenAI-bill per-token cost ratio. **Levers**: L1 — `__unattributed__` training-data cleanup (applies FT-OAI-002 E4 findings, targets the −0.113 delta that dominates the gap); L2 — force `n_epochs=2` per the step-1200/1500 best-val signal (mild overfit past that); L3 — upweight regressed tasks via `--task-weights` (`retrieval.intent_translate` −0.149, `hidden.name_emergence` −0.114); L4 — optionally switch training base to `gpt-4o-mini` if economic + pre-training bench both favour it. **Deploy criterion** (produced in Epic 1): cosine-Δ-per-dollar-saved threshold derived from actual OpenAI billing. **Quality floor**: FT-OAI-003 cross-base mean cosine ≥ 0.8322 (stock `gpt-4.1-mini`) — a regression below the training base invalidates the sprint. **Cost cap**: ≤ $250. Tracked as task #143.

## [0.8.5] - 2026-04-20

### Added

- **DH-005: Health Formula Reweighting & Confidence-Adaptive Scoring** (2026-04-17) — Follow-up to DH-004 (previously carved out as an exception). Restructures `ComputeOverallHealth` from a near-uniform weight table (0.12–0.18, inversely correlated with dimension reliability) to a principled hybrid reliability × user-impact prior, with automatic exclusion of dimensions that lack data:
  - **New formula**: `overall = Σ(w_i · c_i · s_i) / Σ(w_i · c_i)` — normalised weighted-confidence average over 7 sub-dimensions. Dimensions with `c=0` contribute nothing to either numerator or denominator. Replaces the 4/5/6/7-dimension branch table.
  - **New hybrid priors** (`DefaultHealthWeights`): Retrieval 0.08, Memory 0.15, Edge 0.15, Task 0.20, Guidance 0.17, Protocol 0.20, Synergy 0.05. Sum = 1.00. See `docs/features/rsic-feedback-loop.md` for derivation.
  - **Per-dimension data-sufficiency confidence**: each `score*` function now returns `(score, confidence float64)`. Confidence ramps linearly to 1.0 at 100 nodes (Memory), 50 edges (Edge), 50 observations (Task), 30 events (Guidance, Protocol); LearningPhase lookup (Retrieval); binary (Synergy). `SelfAssessmentReport` has 7 new `<Dim>Confidence` fields.
  - **7 new Prometheus gauges**: `mdemg_rsic_health_<dim>_confidence` where `<dim>` is `retrieval|memory|edge|task|guidance|protocol|synergy`. Exposed via `/metrics` and persisted by TSDB writeback.
  - **Operator weight knobs**: `RSIC_HEALTH_WEIGHT_<DIM>` (default values above). Negative values fall back to default with a warning log; zero is honoured as "disable this dimension"; all-zero triggers a `Validate()` warning. Exposed in all 3 compose templates.
  - **Grafana dashboard**: `Overall Health` stat panel gains a description citing the new formula and its threshold-preserving property. New "Dimension Confidence (DH-005)" row with 7 stat panels so operators can distinguish "scored 0 because broken" from "scored 0 because no data."
  - **Tests added**: `internal/ape/health_formula_test.go` (9 cases: all-dim, zero-confidence exclusion, all-zero, fully-healthy under skewed weights, preserves thresholds, hybrid priors, partial instrumentation, disabled dimension, partial confidence). `internal/config/config_health_weights_test.go` (5 cases: defaults, env override, negative ignored, zero allowed, all-zero warns). `TestLiveCollectors_LastGaugeValues_PerDimensionConfidence` asserts TSDB writeback.
- **DH-004: J17 Protocol & Jiminy Dashboard Remediation** (2026-04-17) — Grafana dashboard hygiene + metric integrity fixes:
  - New admin endpoints: `GET /v1/admin/breakers` (list state of all circuit breakers) and `POST /v1/admin/breakers/reset` (force a named breaker to `StateClosed`). Gated by `AUTH_API_KEYS`. Operator escape hatch when a breaker trips on a transient incident but hasn't auto-recovered.
  - New env var `LLM_RETRY_DEADLINE_ENABLED` (default: `true`) — retry once on `context.DeadlineExceeded` from OpenAI iff remaining context budget > 2× base delay. Prevents a single slow upstream response from tripping `openai-constraint-classify` / `jiminy-synthesis` breakers.
  - New field `TicketRestoreTotal` on `ProtocolStats` — distinguishes "no data" (total=0) from "true 100%" (total>0 && ok==total) for downstream dashboard/alert consumers.
  - Compose templates (`internal/cli/compose_templates/docker-compose.yml`, root `docker-compose.yml`, `deploy/docker/docker-compose.prod.yml`) now expose 7 J17 sidecar env vars: `J17_SIDECAR_URL`, `J17_SIDECAR_TIMEOUT_MS`, `J17_SIDECAR_MODE`, `J17_SIDECAR_CONFIDENCE_FLOOR`, `J17_SIDECAR_CB_FAILURE_THRESHOLD`, `J17_SIDECAR_CB_TIMEOUT_SEC`, `J17_NLI_COMPREHENSION_ENABLED`, `J17_NLI_CALIBRATION_BIAS_THRESHOLD`.
- **/strict Mode Foundation** (STRICT-P0P1-2026-04-11) — Deterministic agent governance via escalation persistence and /strict mode enforcement:
  - **T1/T2 comprehension fix (P0)**: Bootstrap header + decoding instruction injected with T1/T2 guidance; comprehension gate auto-downgrades T1 to T2 when follow rate < 50% (`J17_T1_COMPREHENSION_GATE`)
  - **Escalation persistence**: Write-behind `EscalationStore` persists J12 state to Neo4j (`J12EscalationState` label), survives server restarts, piggybacked on trust flush ticker
  - **Strict mode toggle**: `POST /v1/jiminy/strict` enables per-session strict mode; state file at `~/.mdemg/.jiminy-strict-mode` readable by hooks without HTTP
  - **Prompt reformulation**: `POST /v1/jiminy/reformulate` replaces multi-section advisory guidance (~430 tokens) with imperative directives (~200-350 tokens); BLOCKED items emit "STOP." prefix
  - **Response classification**: `POST /v1/jiminy/classify` evaluates agent output against constraints; graduated enforcement (SURFACED=pass, WARNED+=deny); 5s timeout budget
  - **PreToolUse enforcement**: `pre-write-check.py` hook blocks Write/Edit when strict-mode active + escalated constraint violated; fail-open when server unreachable
  - New config: `JIMINY_ESCALATION_PERSIST_ENABLED`, `JIMINY_STRICT_STATE_PATH`, `J17_T1_COMPREHENSION_GATE`
  - New endpoints: `/v1/jiminy/strict`, `/v1/jiminy/reformulate`, `/v1/jiminy/classify`
- **UAITS Framework** (UAITS-2026-04-10) — Universal AI Training Specification, the 10th UxTS framework. Spec-driven training data curation with 4 paradigms (SFT, DPO, RAFT, curriculum):
  - UAITS JSON Schema (`docs/tests/uaits/schema/uaits.schema.json`) and MDEMG spec with 4 datasets
  - UAITS runner with 41 schema + data compliance checks
  - DPO pair builder: constructs preference pairs from `constraint_outcomes` + `llm_interactions` on `guidance_id`
  - Paradigm router: spec-driven pipeline dispatch across all 4 paradigms
  - DPO format support in `format_converter.py` (`convert_dpo_record`, `run_dpo_converter`)
  - Spec-driven gate overrides in `quality_filter.py` (`uaits_spec_path` parameter)
  - Paradigm metadata in `dataset_versioner.py` manifest
  - CLI: `mdemg data curate` and `mdemg data validate` commands
- **RSIC overhaul** (RSIC-OVH-2026-04-09) — transforms RSIC from zero-value to high-value recursive self-improvement:
  - `RSIC_PROTECTED_SPACES` env var replaces hardcoded `mdemg-dev` protection (default: empty = no spaces blocked)
  - Graph-relative blast radius computed from actual node count (was hardcoded 1000)
  - `RSIC_MACRO_CRON_SPACE` configures macro cron target space
  - Calibration-aware planner suppresses low-confidence actions (`RSIC_MIN_ACTION_CONFIDENCE`, default: 0.2)
  - LLM reflector whitelist expanded from 6 to 16 action types
  - Diagnostic action classification — alert-only actions excluded from calibration tracking
  - Default changes: micro cycles enabled, macro daily (was weekly), LLM reflection enabled
- **Real RSIC executors** — `flush_recovery_buffer` graduates stable volatile nodes, `refresh_stale_edges` recomputes weights via co-activation decay, `review_nli_calibration` queries actual constraint metrics
- **Browser UI audit tests** (UI-AUDIT-2026-04-09) — 76 new Playwright tests: 30 screenshot/JS-error/API-5xx baselines (10 tabs), 10 Training Data tab read-only tests, 3 Training Data API tests, 25 interactive/functional tests across all 10 tabs, 8 new test classes. Total suite: 309 tests (306 pass, 3 skip).
- **UI gap analysis** (`docs/features/ui-gap-analysis.md`) — documents 48/125 API endpoints with UI coverage (38%), identifies 77 uncovered routes across Jiminy, conversation, constraints, ingestion, metrics, and infrastructure.

### Changed

- **DH-005 `ComputeOverallHealth` rewrite** (2026-04-17) — replaces the 4/5/6/7-dimension branch table with a single normalised weighted-confidence sum. Weights were previously inversely correlated with dimension reliability (`RetrievalQuality`, a static `LearningPhase` lookup, held the highest weight at 0.18; `EdgeHealth` and `ProtocolHealth`, the two highest-fidelity dimensions, only 0.13). Under the new priors, Protocol and Task (both high-reliability, high-impact) lead at 0.20; Retrieval drops to 0.08; Synergy (file-size proxy) drops to 0.05. Dimensions without data are excluded automatically by the confidence multiplier — no more "5-dim" / "6-dim" / "7-dim" fallback branches. Dashboard thresholds (red <0.4, green ≥0.7) are preserved by construction because the result remains a weighted average in [0,1].
- **DH-004 config defaults** (2026-04-17):
  - `CONSULTING_CLASSIFY_TIMEOUT_MS` default bumped 15000 → 30000 (matches `JIMINY_SYNTHESIS_TIMEOUT_MS`; survives typical `gpt-5.4-mini` latency without tripping the circuit breaker on one slow call).
  - `J17_SIDECAR_TIMEOUT_MS` default bumped 200 → 1000, with a 100ms floor enforced in `FromEnv()`. NLI primary-path calls were timing out at 200ms ~56% of the time, inflating `j17_nli_mean_bias`.
- **LLM Model Config** (TRAIN-DQ-2026-04-10) — standardized all LLM tasks to gpt-5.4 (from mixed gpt-4.1/gpt-4o-mini) for training data quality during distillation campaign
- **Token Counting** — fixed `tokens_in` always recording 0 in TSDB; now properly captures `prompt_tokens` and `completion_tokens` from OpenAI API response as separate fields
- **RAFT Context** — wired `retrieval_node_ids` for `consulting.synthesis` and `retrieval.rerank_cross` (enables retrieval-augmented fine-tuning)
- **Task Activation** — enabled `CONSULTING_LLM_CONSTRAINTS_ENABLED` and `JIMINY_EVALUATE_LLM_ENABLED` in .env, .env.example, and compose template for training data collection

### Fixed

- **DOC-UPDATE-01 docs staleness remediation** (2026-04-20) — aligned `docs/user/cli-reference.md`, `docs/architecture/06_Retrieval_API_and_Scoring.md`, `docs/development/ft-lora/{05_DATA_COLLECTION,06_CORRECTIONS_APPLIED}_v2.md`, and `.env.example` with post-DH-004/DH-005 config defaults. Added docs for `RSIC_HEALTH_WEIGHT_*` × 7, `LLM_RETRY_DEADLINE_ENABLED`, updated `CONSULTING_CLASSIFY_TIMEOUT_MS` (15000→30000) and `J17_SIDECAR_TIMEOUT_MS` (200→1000). Corrected retrieval scoring hyperparameter table (α 0.55→0.60, β 0.30→0.20, γ 0.10→0.15, single-row ρ split into layered `ρ_L0`/`ρ_L1`/`ρ_L2`). Fixed stale `LLM_MODEL` default (`gpt-4.1-nano`→`gpt-5.4-mini`) in cli-reference and ft-lora Issue 21. Bumped batch ingest limit mention from 100 to 500. No code changes.
- **DH-004 dashboard & metric fixes** (2026-04-17):
  - **J17 Protocol Health null-tolerance**: `TicketRestoreSuccessRate` now defaults to `1.0` when `ticketRestoreTotal == 0` (matches the existing `codeCoverage` null-tolerance pattern at `protocol_metrics.go:307`). A healthy system with no restore events no longer drags the 15% stability weight to zero. Lifts `rsic_health_protocol` from 0.432 toward ≥ 0.7.
  - **NLI fallback counting (gate-aware)**: `RecordNLIFallback` now only fires when `nliScorer.IsOperational()` (enabled AND sidecar URL set). A gated-off scorer no longer inflates `j17_nli_mean_bias` when `J17_NLI_COMPREHENSION_ENABLED=false`.
  - **Alert cooldown TOCTOU race**: `cooldown.Allow` + `cooldown.Record` were separate lock acquisitions; concurrent `Dispatcher.Send()` calls could both pass the gate. New atomic `TryRecord()` closes the race — at most one caller wins per (service, severity) cooldown window. Fixes repeating "Jiminy Pipeline Critical" alerts.
  - **Context Cooler graduation**: `CoactivateSession` now reinforces `stability_score` for every session observation via `reinforceSessionObservations`. Previously only created `CO_ACTIVATED_WITH` edges, never raising stability — so 99.7% of conversation observations stayed volatile forever (`rsic_health_task` = 0.019). Forward-only fix; existing volatile data self-heals via ongoing session activity.
  - **LLM retry budget-aware DeadlineExceeded**: `shouldRetry()` gains retry-on-deadline path gated by remaining context budget (`time.Until(deadline) > 2*baseDelay`). Avoids doubling OpenAI spend under sustained slowness while recovering from transient timeouts.
  - **Dashboard panel overlap** (`deploy/docker/grafana/dashboards/mdemg-j17.json`): "Ticket Restore Rate" and "Total Events" were both rendered at `{x:6, y:24, w:6, h:4}`. Relocated Total Events to full-width summary at `{x:0, y:28, w:24, h:4}`; bumped subsequent panels down. Added panel description to `jiminy_latest_age_ms` in `mdemg-jiminy.json` documenting expected /strict-mode staleness.
- **RSIC hardening** (RSIC-HDN-2026-04-09) — 32 findings from deep dive remediated across 6 epics:
  - P0: Nil postReport no longer silently inflates calibration — `CriteriaMet=false` when post-assessment fails
  - P0: Nil driver guards added to 7 executor methods (prevents nil-pointer panics)
  - P1: `dryRun` data race eliminated — passed as `Dispatch()` parameter, not mutable shared field
  - P1: `executeFlushRecoveryBuffer` rewritten to target recovery buffer nodes (was duplicating volatile graduation)
  - P1: `executeCodifyConstraint`/`executeRetireCode` receive proper node ID/code parameters
  - P1: Per-task `CriteriaMet` evaluation replaces cycle-level shared flag
  - P2: Watchdog releases lock before I/O, resets decay counter only on trigger success
  - P2: Safety validator fails closed on estimation error, blast radius LIMIT aligned with executor batch size
  - P2: Orchestration status snapshot is atomic (single lock scope)
  - P2: `ComputeOverallHealth` extracted as single source of truth (was duplicated in 2 files)
  - P2: Synergy reader caches file reads with 60s TTL
  - P2: Config cross-field validation warns on protected space conflicts
  - P2: LLM reflector single action source, expanded whitelist (16→20), prompt injection sanitization
  - P2: SSE job stream race fix (`Job.Snapshot()` for thread-safe reads)
  - P2: `CleanupExpired()` wired to macro cron ticker, `CompleteCycle` skipped on timeout
  - P3: Dead code cleanup (unreachable branch, cron parser documented)
  - CI: `-race` flag added to test pipeline
- **Adversarial Codebase Analysis Bug Fix Campaign** (ACA-BFC-2026-04-10) — 14 bugs remediated from adversarial analysis with systematic refutation:
  - C1: Docker healthcheck hardcoded to `:9999` (was using `${MDEMG_PORT}` inside container)
  - C2: CI coverage generation wired (`-coverprofile=coverage.out` added to `go test`)
  - C3: `train_ft.py` passes `resolved_rank` to `--lora-rank` (was `--num-layers`)
  - H1: Config struct comments aligned with `FromEnv()` defaults (`gpt-5.4`, cascade)
  - H2: Compose `TSDB_DBNAME` → `TSDB_DATABASE` (matches Go `FromEnv()`)
  - H4: Real evaluation metrics replace `check_non_empty()` stubs (coherence, coverage, specificity, follow_rate)
  - M1: Circuit breaker trip guard — `atomic.Bool` + `CompareAndSwap` fires alert once per trip, resets on recovery
  - M2: 502 (Bad Gateway) added to LLM client retry set
  - M3: Jiminy semantic dedup via embedding cosine similarity (fallback to exact-match)
  - M4: Temporal decay on correction retrieval (`JIMINY_CORRECTION_DECAY_RATE`, default 0.01)
  - M5: Dead `ScoringRho` config field removed (suffixed variants unaffected)
  - M6: Bounded ticket map with LRU eviction (`J17_TICKET_CACHE_SIZE`, default 1000)
  - L1: Eval cache wired into `llmEvaluate()` (was defined but never called)
  - L2: Dead trust store goroutine removed (flush managed by `Service.StartTrustPersistence`)
- **Training Data tab renders empty** — `helpSection()` in `training_data.js` passed string array instead of `{term, description}` objects to `helpPanel()`, causing `entries.map()` to throw silently.
- **Form inputs show `[object HTMLInputElement]`** — `infoRow()` in `dom.js` used `String(value)` which stringified DOM Node elements. Added `instanceof Node` check to pass elements through directly.

## [0.7.4] - 2026-04-08

### Added

- **Code comprehension feedback loop** (P1-15) — `CodeComprehensionTracker` monitors per-constraint-code comprehension scores in a sliding window. When average drops below threshold, triggers code regeneration via `ConstraintCodeGenerator`. Feature-gated off by default (`JIMINY_CODE_REGEN_ENABLED=false`). Config: `JIMINY_CODE_REGEN_THRESHOLD` (0.3), `JIMINY_CODE_REGEN_MIN_SAMPLES` (10). Includes cooldown timer + max-regen-per-hour cap.
- **NLI bias alert consumer** (P2-15) — `RecordOutcome` now checks NLI calibration report after tracking and logs a warning when systematic NLI-vs-heuristic bias is detected.
- **Embedding cache TTL** (P2-21) — `NodeEmbeddingCache` now supports TTL-based eviction. Config: `NODE_EMBEDDING_CACHE_TTL_SEC` (default: 3600, 0 = no TTL). Stale entries evicted on access.
- **EdgeTypeStrategy validation** (P2-2) — `Config.Validate()` now rejects invalid `EDGE_TYPE_STRATEGY` values.
- **TSDB schema version CI check** (P2-12) — CI now validates `TSDB_REQUIRED_SCHEMA_VERSION` matches migration file count.
- **Goroutine semaphore** (P2-16) — RSIC task `Dispatcher` now bounds concurrent goroutines to 50 via channel semaphore.
- **Synergy file reader** (`internal/ape/synergy_reader.go`) — implements the `SynergyFileReader` interface for RSIC health assessment. Reads CLAUDE.md and MEMORY.md line counts from disk with auto-detection of file paths. Wired in `server.go` when `SYNERGY_ASSESSMENT_ENABLED=true` (default). Fixes 4 dashboard panels (Synergy gauge, CLAUDE.md Lines, MEMORY.md Lines, Synergy Overflow & Buffer) that previously showed 0.
- **Assessment confidence debug logging** — `computeConfidence()` now logs data point values when confidence drops below 0.3 threshold for faster diagnosis of low-confidence cycle bailouts.

### Fixed

- **Sequence counter not restored on resume** (P1-16) — `ResumeProtocol()` now calls `SetCounter(req.LastSeq)` after event replay, ensuring monotonic sequence continuity.
- **Tier predictor timeout conflation** (P1-17) — `TierPredictResult` now carries `TimedOut` bool. Timeouts logged at `slog.Warn` with latency for distinct metrics recording.
- **Training script TOCTOU** (P1-23) — `build_train_config()` now accepts manifest row count directly instead of re-counting file lines after manifest validation.
- **Watchdog context race** (P1-13) — `Restart()` now holds `mu.Lock` when reassigning `ctx`/`cancel`, preventing concurrent `check()` from reading stale context.
- **postReport lock upgrade race** (P1-12) — Replaced `RLock→RUnlock→Lock` gap with single write `Lock` for atomic read-then-write.
- **Task cycle detection stale reads** (P1-10) — Added `stateVersion` counter to `Dispatcher`, incremented on every task state change. `WaitForCycle` detects stale reads by comparing versions between polls.
- **TryLock skip not reported** (P1-22) — Consolidation handler now checks `result.Skipped` and returns a warning instead of counting skipped consolidation as success.
- **Consolidation cascade on empty graph** (P1-9) — `RunConsolidation` now guards against empty graphs (zero hidden nodes created + zero forward pass updates) and returns early with `EmptyGraph: true`.
- **Healthcheck port hardcoded** (P1-19) — Both `docker-compose.yml` files now use `CMD-SHELL` with `${MDEMG_PORT:-9999}` variable interpolation.
- **Effectiveness TTL too short** (P2-1) — Default `JIMINY_EFFECTIVENESS_TTL_SEC` raised from 7200 to 86400 (24 hours).
- **LISTEN_PORT dead code** (P2-3) — Removed unused `LISTEN_PORT` env var from both compose files.
- **Missing stop_grace_period** (P2-5) — Added `stop_grace_period: 35s` to mdemg service (5s > graceful shutdown timeout).
- **AUTH_API_KEYS naming** (P2-6) — Compose files now accept both `AUTH_API_KEYS` and `MDEMG_API_KEYS` via fallback interpolation.
- **Decay formula NaN** (P2-7) — All 4 Cypher decay formulas now clamp the base to 0.01 when `(1 - decay/sqrt(evidence*surprise))` goes non-positive.
- **CONFLICTS_WITH not idempotent** (P2-11) — Changed `CREATE` to `MERGE` with `ON CREATE SET`/`ON MATCH SET` for safe re-runs.
- **LLM handler timeouts** (P2-10) — `handleJiminyGuide` and `handleRecall` now use `context.WithTimeout(30s)`.
- **TSDB schema version stale** (P2-12) — Default `TSDB_REQUIRED_SCHEMA_VERSION` updated from 8 to 10.
- **Trust store eventual consistency** (P1-10) — Documented that `feedbackCounts` may lag by up to 30s after crash recovery; impact is cosmetic (protocolStatus display only).
- **Dashboard sparse-event panels** — Action Success Rate, Safety Blocks, Snapshots Created, and Trigger Rejections panels now display "None" instead of confusing "No data" when no events exist in the current time window.

> **Validated 2026-04-08**: All 30+ DD-P1P2 fixes live-validated — 14 static checks, 8 unit/race suites, 15 live_validation.py tests, 379 UATS contract tests, 7 fix-specific API tests, 139 integration tests. Zero failures, zero regressions. See [`docs/testing/dd-p1p2-validation-report.md`](docs/testing/dd-p1p2-validation-report.md).

## [0.7.3] - 2026-04-07

### Added

- **Server-native alert evaluator** (`internal/alert/evaluator.go`) — 13 TSDB-query alert rules migrated from Grafana to run natively on the server. Periodic evaluation with configurable interval (`ALERT_EVALUATOR_INTERVAL_SEC`, default: 30s), ForDuration state tracking to prevent flapping, and graceful degradation when TSDB is unavailable. Grafana is no longer required for alert evaluation.
- **Goroutine supervisor** (`internal/supervisor/`) — monitors background goroutines (health prober, alert evaluator) with panic recovery, automatic restart with exponential backoff (5s base, max 3 retries), and alerts on restart (warning) and permanent failure (critical).
- **LLM consecutive failure alert** — tracks consecutive LLM call failures per-client via shared atomic counter; fires high-severity alert through dispatcher when threshold reached (default: 3, configurable via `LLM_CONSECUTIVE_FAILURE_THRESHOLD`). Counter resets on success. Late-binding callback avoids init ordering issues.
- **Alert dispatcher** (`internal/alert/`) — new package with file backend (atomic JSON writes, FIFO eviction at configurable cap), macOS notification backend (opt-in, `//go:build darwin`), per-(service,severity) cooldown dedup, and fire-and-forget dispatch to multiple backends
- **Hook alert delivery** — `prompt-context.sh` shows all pending alerts per prompt; `session-start.sh` shows critical/high alerts at session start; both read from `~/.mdemg/alerts/current.json`
- **Alert wiring** — RSIC alert actions (5 handlers), circuit breaker state change callbacks, and health prober transitions now dispatch through the alert system for real-time user notification
- **LLM retry with exponential backoff** — `llmclient.Client` retries on 429 (with `Retry-After` header support) and 503; configurable max attempts, base delay, jitter; never retries 4xx client errors
- **Enhanced `/healthz`** — now returns lightweight subsystem checks (Neo4j driver, circuit breaker open count, TSDB client, Jiminy) with `status: "degraded"` when subsystems are unhealthy; `session-start.sh` parses and reports degraded status
- **TSDB buffer overflow detection** — `LLMInteractionWriter` enforces configurable max buffer size with FIFO eviction, overflow counter, and alert callback on first overflow
- **Health prober instantiation** — orphaned `internal/healthprobe/` package now wired into production startup with configurable interval and alert callbacks on healthy↔unhealthy transitions
- **Grafana contact point** — `provisioning/notifiers/` with webhook contact point routing alerts to `POST /v1/alerts/grafana`; notification policy with 30s group wait
- **7 new Grafana alert rules** — TSDB writer overflow, LLM retry exhausted, probe down (API, Neo4j, TSDB, sidecar), Jiminy follow rate drop (total: 28 rules across 4 groups)

### Fixed

- **Trust persistence goroutine leak** — `StartTrustPersistence` now wraps context with `WithCancel`; new `StopTrustPersistence()` called during `Shutdown()` ensures final flush and clean goroutine exit.
- **Dead startup code wired** — `StartContextCoolerProcessing` and `StartWeeklyGapInterviews` were fully implemented but never called; now wired behind opt-in config gates (`CONTEXT_COOLER_ENABLED`, `WEEKLY_GAP_INTERVIEWS_ENABLED`, both default: false).
- **Grafana alert rules demoted to supplementary** — contact point and notification policy disabled; alert rules kept in `alerts.yml` with header comment for users who want redundant Grafana alerting. `/v1/alerts/grafana` endpoint preserved for backward compatibility.
- **Hook alert banners broken on macOS** — `timeout` command (GNU coreutils) is not available on macOS. Both `session-start.sh` and `prompt-context.sh` used `timeout 2 jq ...` to parse the alert file, which failed silently and suppressed all alert banners. Removed the `timeout` wrapper — jq reads a small local JSON file and completes instantly.

### Changed

- **`ALERT_COOLDOWN_SEC=0` means no cooldown** — previously defaulted to 300s. Zero is useful for testing; negative values still fall back to 300s default.
- **`/readyz` check #5 (conversation)** — upgraded from nil guard to live `Ping()` query (`RETURN 1`), detecting CMS degradation when Neo4j is under stress
- **Circuit breaker expansion** — Jiminy outcome classifier and constraint code generator now wrapped with circuit breakers. On breaker open, classifier falls back to heuristic and codegen falls back to deterministic hash.
- **Health prober alert callback wired** — prober healthy↔unhealthy transitions now dispatch through the alert system
- **TSDB writer alert callback wired** — buffer overflow events now fire medium-severity alerts through the dispatcher
- **Default LLM model: gpt-5-nano → gpt-4.1-nano** — all 16 classification/evaluation tasks use prompt-engineered JSON (`json_object` mode), not tool-call schemas. gpt-4.1-nano is non-tool-use, 2x cheaper output tokens ($0.20/M vs $0.40/M), and has a 1M context window. Affects `LLM_MODEL`, `RECLASS_MODEL`, `RERANK_MODEL` defaults in config, compose templates, and CLI init prompts.
- Fine-tuning plan documents updated from v3.0 to v4.0 — adds tool-use architectural constraint, curated dataset pipeline, Jiminy quality signals, and v0.7.1 classifier overhaul as a critical training data versioning boundary (Issues 20-28 in corrections log)

### Added (UATS)

- **`grafana_alert_webhook.uats.json`** — 3 variants: base firing alert (200), empty alerts (200), resolved alert (200)
- **`healthz_enhanced.uats.json`** — 8 assertions validating enhanced `/healthz` response: `status` one_of `[ok, degraded]`, `checks` map exists with `neo4j`, `circuit_breakers`, `tsdb` keys
- **`health.uats.json`** updated — added `$.checks` exists assertion for enhanced `/healthz` compatibility

## [0.7.2] - 2026-04-06

### Fixed

- **Trust accrual: partial_compliance excluded from trust scoring** — `trustRelevanceThreshold` lowered from 0.5 to 0.20, aligning with the classifier's `not_applicable` cutoff. 38% of outcomes (partial_compliance) were filtered before reaching the trust scorer, halving effective trust growth rate.
- **Trust accrual: OutcomePartialCompliance missing from aggregate switch** — partial compliance outcomes were silently dropped in the trust aggregate logic, never reaching `TrustScorer.RecordOutcome()`. Added `partialCount` to the aggregate with conservative priority (boosted only when no ignores present).
- **WarmStore upward-crossing invalidation** — cache invalidation only fired on downward tier crossings (T1→T2, T2→T3). Added upward crossing checks so T3→T2 and T2→T1 promotions immediately invalidate stale lower-tier guidance.
- **J8 synthesis overrides T1 compact encoding** — synthesis unconditionally replaced tier-encoded augmentation with a ~2000-token LLM narrative, nullifying T1's 5.2x compression. Synthesis is now skipped at T1 trust (> 0.75); compact coded format is delivered directly. Added `EncodedAugmentation` response field to preserve J17-encoded form when synthesis runs at T2/T3.
- **Partial compliance in metrics pipeline** — added `partial_compliance` outcome to all Jiminy Guidance Dashboard panels (pie chart, trends, per-constraint table), follow rate formula (`(followed + 0.5*partial) / total`), and constraint effectiveness (volume-weighted with >= 5 surfaced minimum).

### Investigation

- **J17 tier promotion analysis** — tested full T3→T2→T1 chain; trust accrued 0.22→0.76 in 15 cycles. Found J8 synthesis overrides T1 compact encoding (P1). See `docs/development/J17_TIER_PROMOTION_ANALYSIS.md`.

## [0.7.1] - 2026-04-06

### Fixed

- **Negation detection false positives** — negation patterns ("instead of", "did not", "skipped", etc.) in action summaries no longer short-circuit to `contradicted` before LLM Tier 2. The `Classify()` flow now defers negation to the LLM with full context (matched pattern, action format guidance). Heuristic fallback only applies when LLM is unavailable. Eliminates constant ~4.5% contradicted rate caused by negation words in quoted code within `replaced 'OLD' with 'NEW'` action summaries.
- **LLM system prompt: action summary format** — classification prompt now explains that `"replaced 'OLD' with 'NEW'"` means OLD was removed and NEW was added. Negation words in OLD text (deleted code) are not indicators of contradiction. Reduces LLM misclassification of edit actions.
- **Source Diversity metric query** — `computeSourceDiversity()` was grouping by `n.obs_type` which is null on constraint nodes (they use `role_type`). Changed to `COALESCE(r.guidance_type, n.obs_type)` which uses the `guidance_type` property already stored on every GUIDANCE_OUTCOME edge. Restores diversity metric from 0% to ~68%.
- **Outcome classifier: `not_applicable` for topically unrelated guidance** — items with cosine similarity below the low threshold (0.20) are now classified as `not_applicable` instead of `ignored`. This prevents false confidence decay (-0.03/item), false escalation advancement, and polluted GUIDANCE_OUTCOME edges for guidance that was topically unrelated to the action taken. `OutcomeIgnored` is now only reachable via LLM Tier 2 semantic judgment.
- **Guidance content normalization** — structured metadata from ingestion (`"Module: X. Related to: a, b. Key functions: f"`) is now normalized to natural language before embedding comparison. Items with LLM-generated `SEMANTIC:` blocks use the natural language portion directly. This raises the cosine similarity ceiling from ~0.33 to ~0.59 for matching topics, making the "followed" threshold (0.55) reachable.
- Jiminy outcome classifier LLM tier enabled by default (`JIMINY_OUTCOME_LLM_ENABLED=true`) — was disabled, causing 35% of items to hit a binary heuristic fallback
- Heuristic fallback now produces `partial_compliance` for the uncertain similarity range (0.20-0.55) instead of a binary followed/ignored split at 0.5
- Outcome similarity thresholds adjusted (high: 0.7→0.55, low: 0.3→0.20) to match action summary embedding characteristics
- Action summaries enriched with file content snippets (Write: first 5 lines up to 300 chars, Edit: 200 chars) and intent annotations for improved cosine similarity
- GUIDANCE_OUTCOME edges filtered to typed nodes only (constraint, correction, pattern, learning) — eliminates 92% of edges on untyped code description nodes
- Feedback cooldown reduced from 30s to 10s for higher coverage (~48% suppression → ~15%)

### Added

- `guidance_type` property on GUIDANCE_OUTCOME edges for downstream analysis
- `JIMINY_OUTCOME_LLM_ENABLED`, `JIMINY_OUTCOME_SIMILARITY_HIGH`, `JIMINY_OUTCOME_SIMILARITY_LOW` env vars in Docker Compose templates and `mdemg init`
- `JIMINY_OUTCOME_CLASSIFIER_ENABLED` env var in Docker Compose templates — was missing, causing inconsistency between Docker and native mode
- DocComment enrichment in `generateSummary()` — appends exported symbol doc comments (up to 400 chars) to structural summaries for improved embedding similarity
- `scripts/cleanup_foreign_symbols.sh` — batch cleanup script for foreign SymbolNodes ingested into wrong space
- `space_id` filter on `GetSymbolsForMemoryNode()` SymbolNode matches — prevents cross-space symbol contamination in retrieval

### Investigation

- Jiminy guidance effectiveness analysis: diagnostic script + findings report
- `scripts/jiminy_effectiveness_report.py` — reusable diagnostic for ongoing monitoring

## [0.7.0] - 2026-04-05

### Added

- V0024 migration — `SignalState` node type for signal learner persistence
- `com.mdemg.maintenance` LaunchAgent — weekly scheduled maintenance (decay + prune) via `mdemg service install`
- `Config.Validate()` — cross-field constraint checking for weight sums, bound relationships
- Pool metrics collector — periodic Neo4j connectivity probe (10s interval)
- `NilSafe` embedder wrapper �� returns `ErrNoEmbedder` instead of panic when no provider configured

### Fixed

- **P0: RRF activation bias** — activation seeding now uses RRF score (authoritative fused ranking signal) instead of `max(VectorSim, BM25Score)` which compared unequal scales, systematically suppressing BM25-only candidates
- **P0: Pre-bash guard fail-open** — hook now decodes/compiles patterns individually with fail-closed design; blocks ALL commands when zero patterns load; returns deny on broken stdin
- **P0: Schema version drift** — deploy configs (Docker, K8s, Helm, .env.example) updated from stale 15/19 to 23; CI validation step checks all configs match migration count
- **P1: Signal learner ephemeral** — persists Hebbian signal intelligence to Neo4j via `HydrateSignals`/`FlushSignals` with 30s periodic flush and graceful shutdown flush
- **P1: Goroutine lifecycle** — all background goroutines tracked with `sync.WaitGroup`; `Shutdown()` waits for completion before closing writers
- **P1: Consolidation race** — per-space `TryLock` on `RunConsolidation` prevents concurrent runs from creating duplicate L2+ concepts
- **P1: Cache key gap** — `IncludeGlobalSpace`, `CodeOnly`, `TranslateIntent` now included in cache key to prevent cross-flag result contamination
- **P2: Learning writeback timeout** — async Hebbian writeback goroutine now has 10s context timeout
- **P2: Sidecar confidence floor** — NLI comprehension scorer applies `J17SidecarConfidenceFloor` uniformly with tier predictor

## [0.6.1] - 2026-04-05

### Fixed

- `mdemg init` now propagates Jiminy config (`JIMINY_ENABLED`, synthesis model/provider, evaluate model/provider) to `.env` for Docker Compose (#265)
- Hook templates use runtime port discovery instead of hardcoded URL (#267) — reads `.mdemg.port` → `.env` MDEMG_PORT → fallback 9999
- Claude hook templates include `# MDEMG` marker for lifecycle management (install/uninstall/re-install)
- Hook template ingest error logging upstreamed from installed hooks (session-start staleness check, pre-compact error capture, post-tool ingest failure logging)

### Changed

- `mdemg init` force-updates hooks to latest templates on re-run (ensures port discovery and markers are deployed)

## [0.6.0] - 2026-04-05

### Added

- `mdemg graph repair` command — weight-preserving SymbolNode dedup with CO_ACTIVATED_WITH edge aggregation, vendor cleanup, orphan sweep, embedding backfill, and V0023 readiness check
- `mdemg maintenance` command — combined decay + prune cycle suitable for scheduling via launchd/cron
- `mdemg embeddings backfill` command — find and fill missing embeddings on MemoryNodes
- `mdemg prune --match-ignore` flag — delete nodes matching `.mdemgignore` patterns
- `mdemg prune --include-labels` flag — control which labels are scanned for orphans (default: MemoryNode)
- V0023 self-healing migration — batched dedup of duplicate SymbolNodes before uniqueness constraint, safe on any user graph
- Evidence-weighted decay formula — `rate/sqrt(evidence_count)` protects well-evidenced edges, with `--max-decay-percent` safety cap
- Upgrade guide (`docs/user/upgrade-guide.md`) for v0.5.x → v0.6.0

### Changed

- Decay rate default `0.1` → `0.02` (less aggressive, evidence-weighted formula compensates)
- SymbolNode MERGE key now uses `(space_id, name, file_path, symbol_type)` natural key
- `QUERY_CLASSIFY_ENABLED` compose default `false` → `true` (users with explicit `.env` setting are unaffected)

### Fixed

- Training data export produces invalid archive when `MDEMG_INSTANCE_ID` not set — `instance_id` auto-generated as `{hostname}-{space_id}`
- `mdemg init` now writes `MDEMG_INSTANCE_ID` to `.env` for server/CLI consistency (both native and Docker modes)
- Export filename and `export_id` no longer contain double-dash when instance ID was previously empty
- SymbolNode duplication (BUG-1) — MERGE uses natural key; V0023 constraint prevents recurrence
- Decay formula (BUG-5) — unified to single evidence-weighted system with safety cap
- Prune label scope (BUG-6) — `--include-labels` flag allows scanning SymbolNode, Observation, etc.
- Hidden layer OOM during L2-L5 consolidation — batched orphan HiddenPattern deletion (500 per transaction)
- `data check --pre-campaign` TSDB Writable check uses correct `metric_samples` column names
- `data check --pre-campaign` Instance ID check returns WARN (not FAIL) when `MDEMG_INSTANCE_ID` is empty
- `live_validation.py` python3 resolution — uses `shutil.which("python3")` instead of bare `python`
- `live_validation.py` curation pipeline — correct argument names for quality_filter, format_converter, dataset_versioner
- `space.go` `reEmbedNodes` type scope panic — `nodeContent` struct moved before closure

## [0.5.4] - 2026-04-03

### Added

- Multi-instance deployment guide (`docs/user/multi-instance.md`) with resource measurements (PR #256)
- Multi-instance testing results (`docs/operations/multi-instance-testing-results.md`) — 4 simultaneous instances, port allocation, data isolation verified (PR #256)
- TSDB backup before teardown via `--export` flag — pg_dump runs before `docker compose down -v` destroys volumes (PR #258)
- Upgrade automation: `mdemg upgrade` and `brew upgrade mdemg` now auto-update running Docker instances (PR #260)
- New upgrade flags: `--no-docker` (skip Docker), `--docker-only` (Docker only) (PR #260)
- GoReleaser `post_install` hook for Homebrew — `brew upgrade` triggers Docker instance updates (PR #260)

### Fixed

- `mdemg teardown` does not stop Docker Compose services — uses legacy single-container naming (PR #257)
- Teardown silently destroys TSDB training data when removing Docker volumes (PR #258)

## [0.5.3] - 2026-04-03

### Added

- `WithSpaceID` context helper — query_classify and intent_translate TSDB records now get correct request space_id instead of defaultSpaceID (PR #254)
- Campaign env vars forwarded in compose template: `QUERY_CLASSIFY_ENABLED`, `INTENT_ENABLED`, `JIMINY_ENABLED`, `EMERGENCE_ENABLED`, `LLM_INTERACTION_LOGGING` (PR #254)
- Campaign task activation prompt during interactive `mdemg init` — writes campaign env vars to `.env` (PR #254)
- `scripts/live_validation.py` — 19 automated end-to-end tests covering API, TSDB recording, export pipeline, regression gate, and error handling (PR #254)
- Weekly cron safety net for `docker-publish.yml` (Monday 6am UTC) — catches missed `workflow_run` triggers (PR #254)
- Post-fix re-validation results doc (`docs/operations/post-fix-revalidation-results.md`) (PR #254)

### Fixed

- TSDB schema version stuck at 7 despite 10 migrations applied — migration 010 corrects to 10 (PR #254)
- `data check --pre-campaign` threshold updated from >= 9 to >= 10 to match migration 010 (PR #254)

## [0.5.2] - 2026-04-03

### Added

- `AUTO_MIGRATE` env var for unified Neo4j + TSDB schema migration in Docker deployments (PR #252)
- Neural-sidecar Docker image published to GHCR — `ghcr.io/reh3376/mdemg-neural-sidecar` replaces `build: ./neural` (PR #252)
- `docker-publish.yml` triggers via `workflow_run` on Release completion — GHCR images update on every tagged release (PR #252)
- LaunchAgent templates embedded in binary via `embed.FS` — `mdemg service install` works without repo checkout (PR #252)
- `PersistentPreRun` on `data` command loads `.env` file for all subcommands including `export`, `check` (PR #252)
- `session_id` field on `/v1/memory/retrieve` and `/v1/memory/consult` endpoints — propagated to TSDB for session-level training data analysis (PR #253)
- Live validation findings documentation in `docs/operations/` (PR #252, #253)

### Fixed

- neural-sidecar `build: ./neural` in compose breaks all non-repo installs (PR #252)
- Fresh Neo4j crash-loops with "SchemaMeta missing" — no AUTO_MIGRATE for first-start schema creation (PR #252)
- Docker Publish CI never triggers on release — GHCR images stuck at v0.3.4 since original publish (PR #252)
- `mdemg data` commands don't load `.env` — TSDB connection refused on Docker installs where creds are in `.env` (PR #252)
- `mdemg service install` fails for Homebrew users — LaunchAgent templates not in release tarball (PR #252)
- `session_id` in TSDB contains instance_id instead of request-provided session_id (PR #253)
- `space_id` in TSDB reranking records shows defaultSpaceID instead of request space_id (PR #253)
- `query_classify` and `intent_translate` produce zero TSDB records — recorder nil at construction due to init ordering (PR #253)
- `mdemg data export` silently returns 0 rows — instance_id auto-detection generates wrong ID for Docker containers (PR #253)

## [0.5.1] - 2026-04-02

### Added

- Embedded `docker-compose.yml` in binary for Homebrew/edge installs — `mdemg init` works without repo checkout (PR #244)
- `mdemg data export-auto` command with retention management (`--keep N`) and `latest.tar.gz` symlink (PR #245)
- Training-export LaunchAgent for automated 24h export cycle via `mdemg service install` (PR #245)
- vllm-mlx setup guide (`docs/operations/vllm-mlx-setup.md`) with M5 Max memory budget and prefix caching docs (PR #246)
- 16-task smoke test for vllm-mlx (`scripts/test_vllm_mlx.py`) validating all ULTS tasks through OpenAI-compatible API (PR #246)
- `train_ft.py` — LoRA fine-tuning script with manifest validation, anti-collapse gate (exogenous ratio >= 0.4), and ULTS-aware LoRA rank resolution (PR #246)
- `evaluate_ft.py` — per-task evaluation against held-out test set using ULTS quality_metrics contract, supports stored responses and live inference (PR #247)
- `regression_gate.py` — deployment gate comparing candidate vs baseline eval reports: no task regresses >5%, >=2 tasks improve >=2%, JSON validity >=95%, overall score >= baseline. Exits PASS/FAIL/WARN (PR #248)
- `teacher_distill.py` — synthetic training data generation for under-represented tasks using teacher LLM with exact MDEMG system prompts, Go source prompt extraction, input templates for all 16 tasks (PR #249)
- `reward_functions.py` — 21 GRPO reward functions covering all 18 ULTS reward_function names. Registry with `get_reward_function()` and `compute_reward()` API (PR #249)
- `quantize_deploy.py` — fuse LoRA adapter into base model via `mlx_lm.fuse`, quantize to 4-bit/8-bit, optional verification inference (PR #250)
- `mlx-lm` optional dependency in `pyproject.toml` `[lora]` extras group (PR #246)

### Fixed

- `mdemg init` fails for Homebrew/edge binary users — compose file not in release tarball (PR #244)
- `mdemg tsdb start/stop` hardcoded to repo-relative `deploy/docker/docker-compose.observability.yml` path (PR #244)
- `docker-compose.yml` candidate order in `internal/tsdb/backup.go` — cwd checked first (PR #244)

## [0.5.0] - 2026-04-02

### Added

- `mdemg data check --pre-campaign` with 8 automated validation checks for collection campaign readiness (PR #243)
- QueryClassifier wired into retrieval service with `QUERY_CLASSIFY_ENABLED` env var (PR #243)
- `session_id` propagation from API handlers through to LLM interaction TSDB records (PR #243)
- Campaign task activation guide (`docs/operations/campaign-task-activation.md`) (PR #243)
- FT Implementation Plan reconciliation — Phases 1-12 current with `03_IMPLEMENTATION_PLAN_v2.md` (PR #243)

## [0.4.2] - 2026-04-01

### Added

- Instance ID backfill on server startup — all training tables get `instance_id` (PR #242)
- `defaultSpaceID` fallback for all 16 LLM consumers (PR #242)
- Neo4j memory tiering based on system RAM detection during `mdemg init` (PR #242)
- Migration 009: `space_id` backfill for existing TSDB records (PR #242)

### Fixed

- All 16 LLM consumers wrote empty `space_id` to TSDB records (PR #242)
- Neo4j defaults too aggressive for 32GB machines — tiered config based on RAM (PR #242)

## [0.4.1] - 2026-03-31

### Added

- FT-HARDENING: per-field privacy skip patterns for multi-table export (PR #241)
- Instance ID column on training tables — migration 008 (PR #241)
- Schema version bumped to 8 (PR #241)
- CI: edge publish artifact download fix (PR #241)

## [0.4.0] - 2026-03-30

### Added (FT-DATA Training Data Export + Curation Pipeline)

- **UTDS Spec Framework** (`docs/tests/utds/`): 14th UxTS framework — JSON Schema validating export `manifest.json` files. Schema enforces `privacy_scrub_violations == 0` (hard gate), `schema_version >= 7`, `export_id` pattern `^exp-`. 3 fixture specs (standard/llm-only/minimal), validation runner, 23 unit tests.
- **`mdemg data export` CLI** (`internal/cli/data_export.go`): Export TSDB training data as `.tar.gz` archives containing JSONL + manifest. Flags: `--tables`, `--since`, `--until`, `--exclude-embedding`, `--dry-run`, `--no-validate`. Streams rows via pgx with O(1) memory. Privacy scans ALL 10 text fields across 3 tables (llm_interactions, retrieval_events, embedding_events) — export BLOCKED if any violations detected.
- **Training Data Export API** (`internal/api/handlers_training_data.go`): `POST /v1/training-data/export` (async via jobs queue), `GET /v1/training-data/status/{id}`, `GET /v1/training-data/download/{id}`. Auth: `ScopeAdminSpaces`.
- **Training Data Browser Tab** (`internal/api/ui/tabs/training_data.js`): 10th browser UI tab — export form with table selection, date range, status polling at 5s, download on completion.
- **`quality_filter.py`** (`neural/training/quality_filter.py`): 8 quality gates (privacy hard-reject, empty response, error present, duplicate prompt, latency exceeded, unknown model, stale prompt hash, ULTS output invalid). Privacy patterns mirror Go scrubber exactly. 25 unit tests.
- **`format_converter.py`** (`neural/training/format_converter.py`): Converts filtered JSONL to HuggingFace MLX chat format. RAFT 80/20 context handling (deterministic via SHA-256 trace_id seed). Think-mode wrapping. MLX format validation. 21 unit tests.
- **`dataset_versioner.py`** (`neural/training/dataset_versioner.py`): Temporal train/test/val splits (NEVER random), cross-source deduplication, SHA-256 per split, task balance warnings, exogenous ratio checks, dataset manifest generation. 20 unit tests.
- **Round-trip verified**: TSDB → export (449 rows) → UTDS validate (26/26) → quality filter (449→287) → format convert (287 MLX) → dataset version (229 train / 28 test / 30 val) — all quality gates passed.

### Added (Distribution Automation)

- **Edge binary CI** (`.github/workflows/cli-publish.yml`): Platform-specific CLI binaries built on every push to main. Published as rolling `edge` GitHub Release with SHA-256 checksums. Supports darwin/arm64, darwin/amd64, linux/amd64, linux/arm64 via CGO + zig cross-compilation.
- **`mdemg upgrade --edge`**: Self-update to latest edge build (bare binary from GitHub). Compares commit hashes to skip if already current. Also copies updated binary to `./bin/mdemg` if the directory exists.
- **`mdemg update` alias**: Alias for `mdemg upgrade`.
- **`mdemg init` binary copy**: Copies the running mdemg binary to `./bin/mdemg` during init, ensuring hooks have a local binary available.
- **Healthz commit field**: `/healthz` response now includes `"commit"` field for version mismatch detection between CLI binary and running server.
- **Session-start version check**: Hook detects CLI/server commit mismatch and warns with upgrade instructions.
- **Install script edge channel**: `CHANNEL=edge bash install.sh` downloads the latest edge binary (bare format, no tar.gz extraction).

### Added (RSIC-DATA Sprint)

- **RSIC-DATA: TSDBDatasetBuilder** (`internal/tsdb/dataset_builder.go`): Curated data access layer providing 5 structured datasets — LLMPerformance, RetrievalQuality, EmbeddingCoverage, MetricTrend (linear regression slope + volatility), TrainingDataReadiness. DatasetProvider interface allows mocking without a database. Consolidates and supersedes dead-code `trend_analyzer.go`.
- **RSIC-DATA: 6 TSDB-aware reflection patterns** (patterns 25-30): `llm_latency_regression`, `llm_error_rate_spike`, `retrieval_quality_degradation`, `embedding_pipeline_regression` (CRITICAL), `training_data_ready`, `trust_trajectory_decline`. RSIC now analyzes trends over time, not just point-in-time snapshots.
- **RSIC-DATA: Assessor + Reflector wiring**: DatasetProvider wired via CycleOrchestrator → Assessor (report population) + Reflector (trend queries). SelfAssessmentReport extended with LLMPerformance, RetrievalDataset, EmbeddingDataset, TrainingReadiness fields.
- **RSIC-DATA: 6 dispatch handlers**: `review_llm_provider`, `alert_llm_health`, `alert_embedding_regression`, `trigger_training_pipeline` (new), `alert_tsdb_health`, `alert_schema_drift` (gap fixes for existing patterns).
- **RSIC-DATA: Grafana Data-Driven Insights row**: 4 new panels on mdemg-rsic dashboard — LLM Performance by Task, Retrieval Pipeline Health, Training Data Volume, Trust Trajectory.

### Added (DATA-GOV Data Governance)

- **DATA-GOV Sprint 2: TSDB Data Quality Diagnostic Script** (`scripts/tsdb_data_review.py`): Comprehensive Python diagnostic that connects to TimescaleDB and produces a data quality report across all 8 TSDB tables. 7 diagnostic sections: schema health, metric_samples catalog, llm_interactions analysis (task coverage, error rate, latency percentiles, privacy scrub verification), embedding_events (call_site regression check, scrub asymmetry), retrieval_events (pipeline stage completeness, hard-negative mining viability), ft_* tables, cross-cutting analysis (growth rates, flush gap detection, config flag check). Output formats: text (ANSI color), JSON, or both. Privacy patterns mirror `internal/llmclient/scrubber.go` exactly. 17 unit tests. UV project setup (`scripts/pyproject.toml`).
- **DATA-GOV Sprint 1: Pipeline Gap Fixes**: System prompt coverage for all LLM callers, call_site propagation fixes, PROMPT-COV (system prompt hash + coverage analysis).
- **DATA-GOV Sprint 0: TSDB_ENABLED Fix**: `TSDB_ENABLED=true` in Docker compose environment, `TSDB_AUTO_MIGRATE` env var support in `serve.go`. Feature doc: `docs/features/tsdb-data-governance.md`.

### Added (J17 Comprehension Pipeline)

- **J17 Comprehension Pipeline 5-Break Fix**: Sigmoid normalization midpoint 2.0→1.5 (retrieval_source.go + consulting/service.go), NLI guard broadened for non-constraint items, constraint_code propagation in hidden layer, trust relevance filter (trustRelevanceThreshold=0.5).
- **J17 Trust Parameter Rebalance**: Initial trust 0.5→0.65, high threshold 0.8→0.75, boost +0.05/follow, decay -0.02/ignore, -0.04/contradict. T1 compression achievable in 3 follows instead of 7+.
- **TSDB Trust Score Historization**: 4 new Prometheus gauges (`j17_avg_trust_score`, `j17_min_trust_score`, `j17_max_trust_score`, `j17_trust_session_count`) wired through TrustScorer.Aggregates() → ProtocolMetrics → adapter → live_collectors → TSDB flush. J17 Grafana dashboard: Trust Score row with 4 stat panels + trend timeseries.
- **Multi-Instance UI Dropdown**: Instance selector in browser dashboard header for switching between MDEMG instances.

### Fixed

- **TSDB_ENABLED not set in Docker flow**: `docker-compose.yml` now sets `TSDB_ENABLED: "true"`, `TSDB_AUTO_MIGRATE: "true"`, and `TSDB_OPTIONAL: "true"` in the mdemg service environment. `mdemg init` writes `TSDB_ENABLED=true` to `.env`. Without this, TimescaleDB metrics collection was silently disabled in Docker deployments.

### Added

- **Docker Deployment — Phase 3: Backup UI + Distribution + Cleanup (DOCKER-P3)**:
  - **Backup Tab**: 9th browser tab wrapping all 7 backup REST endpoints — trigger backup (space + type selector), backup history table with type filter and delete, restore from completed backups with confirmation, active operation status polling at 5s intervals.
  - **Credential prompts in `mdemg init`**: Interactive mode now prompts for Neo4j, Grafana, and TimescaleDB passwords. `--defaults`/`--quick` uses sensible defaults. Credentials written to Docker `.env` as `GRAFANA_PASSWORD` and `TSDB_PASSWORD`.
  - **Enhanced post-install summary**: Shows Dashboard URL, Grafana/Neo4j credentials, and common Docker Compose commands.
  - **Distribution cleanup**: Removed Windows native build job, Scoop manifest step, and .deb packaging (nfpms) from release pipeline. Removed systemd unit files from archives. Linux/Windows now use WSL2 + `scripts/install.sh`.
  - **Submodule cleanup**: Archived 5 repos (mdemg-windows, mdemg-menubar, mdemg_linux, mdemg-linux-sidebar, apt-mdemg). Removed corresponding submodules and `apt-publish.yml` workflow.
  - **`db start`/`db stop` deprecated**: Commands still work but print deprecation warning directing users to `docker compose`.
  - **221 Playwright e2e tests**: 21 new backup tab + API tests added to existing suite.

- **Docker Deployment — Phase 2: Browser Dashboard (DOCKER-P2)**:
  - **Browser UI at `/ui/`**: Lightweight dashboard served via Go `embed.FS` from the MDEMG server. Vanilla HTML + JS + CSS (Catppuccin Mocha theme), no build step. 6 tabs: Status (health badges + Grafana links), Memory (layer breakdown + export/import), Learning (Hebbian stats + freeze/unfreeze/prune), Config (effective config table), Logs (searchable, color-coded viewer), RSIC (trigger cycle + Grafana link).
  - **`GET /v1/admin/config`**: Returns effective configuration with source attribution (env/yaml/default) and masking for sensitive values.
  - **`GET /v1/admin/logs`**: Returns recent log entries from in-process ring buffer. Client-side filtering by level and text search.
  - **LogRingBuffer**: Thread-safe ring buffer implementing `io.Writer`, wired via `io.MultiWriter(os.Stderr, ringBuf)` into slog initialization. Captures structured log output for the browser log viewer.
  - **Grafana deduplication**: Browser UI links to 7 existing Grafana dashboards for time-series metrics rather than duplicating them. UI focuses on unique data (memory stats, config, logs) and actions (freeze, prune, export, RSIC trigger).
  - **Documentation**: `docs/features/browser-ui.md` (architecture, tabs, API), quickstart-docker.md updated, api-reference.md updated with new endpoints.

- **Docker Deployment — Phase 2b: Browser UI/UX Overhaul (DOCKER-P2b)**:
  - **Bug Fixes**: Fixed field mapping mismatches in Memory tab (7 fields), Learning tab (10 fields), and Status tab (duplicate status row, orphaned Embeddings section). Root cause: JS used assumed API field names that differed from actual Go JSON responses.
  - **RSIC Service Controls**: RSIC tab now shows service status/state badges and Start/Stop/Restart buttons. Watchdog gained `Restart()` and `Running()` methods.
  - **Server Restart**: Status tab has Restart Server button (`btn-danger` with confirm dialog). Platform-specific re-exec via `syscall.Exec` (Unix) / `exec.Command` (Windows). JS polls `/healthz` after restart.
  - **`PATCH /v1/admin/config`**: Editable config tab — text inputs, dropdowns (known enums), checkboxes (booleans), dirty-field tracking with Save All. Rejects env-sourced, sensitive, and unknown keys. Writes to YAML config file with validation.
  - **`POST /v1/admin/rsic/start|stop|restart`**: RSIC watchdog lifecycle endpoints.
  - **`POST /v1/admin/restart`**: Graceful server restart via re-exec.
  - **Plugins Tab**: New tab showing installed plugins as cards with type/state badges and Start/Stop/Restart/Validate/Details controls. Plugin lifecycle via `POST /v1/plugins/{id}/start|stop|restart`.
  - **Features Tab**: New tab listing all services in two groups (Controllable with lifecycle buttons, Config-Only with status display). `GET /v1/admin/features` and `POST /v1/admin/features/start|stop|restart`.
  - **Dashboard now 8 tabs**: Status, Memory, Learning, Config, Logs, RSIC, Plugins, Features.
  - **193 Playwright e2e tests**: Comprehensive browser testing covering all 8 tabs, API contracts, dirty-field tracking, save bar visibility, theme verification, and polling behavior.

- **Docker Deployment — Phase 1 (DOCKER-P1)**:
  - **Docker Compose consolidation**: Root `docker-compose.yml` rewritten with all 5 services (mdemg, neo4j, timescaledb, neural-sidecar, grafana). All ports parameterized via `.env`. Neo4j community edition (`neo4j:5`). No `container_name` directives for multi-instance isolation via `COMPOSE_PROJECT_NAME`. `neo4j-monitor` moved to `docker-compose.dev.yml`.
  - **Docker image CI**: `.github/workflows/docker-publish.yml` — multi-arch (amd64/arm64) image build pushed to GHCR on release tags and main branch pushes. Uses GitHub Actions cache for layer reuse.
  - **`mdemg init` is now Docker-first**: Default init flow checks Docker availability, scans 6 free host ports, generates `.env` with port assignments and credentials, creates `config.yaml` with Docker defaults, runs `docker compose up -d`, and waits for health check. Native init available via `--native` flag (dev-only).
  - **Dockerfile.prod fix**: Healthcheck now uses `LISTEN_PORT` env var for portability across compose configurations.
  - **`.env.example` updated**: Docker Deployment section added at top with `COMPOSE_PROJECT_NAME`, port variables.
  - **Documentation**: `docs/user/quickstart-docker.md` (Docker deployment guide), `docs/user/mdemg_beta_testing.md` (unified cross-platform beta testing guide), `docs/quickstart.md` updated with Docker-first section. Dynamic port notes added to api-reference, cli-reference, ingestion-guide, cms-rsic-guide. `README.md` rewritten with Docker-first Quick Start.

- **Service Resilience (SVC-RES Sprint)**:
  - **Hook Auto-Recovery**: session-start.sh auto-starts MDEMG server if down (10s polling cap within 15s hook timeout). prompt-context.sh shows visible "CMS unavailable" warning. All ingest operations log to `~/.mdemg/logs/ingest-claude-md.log`. TimescaleDB health check at session start.
  - **Ingest JSONL Buffer**: `mdemg ingest-claude-md` buffers locally to `.mdemg/ingest-buffer.jsonl` when server is unreachable. FIFO eviction at 100 entries (configurable via `INGEST_BUFFER_MAX_ENTRIES`). Automatic flush-on-reconnect.
  - **Prune-Guard Detection**: `post-tool-observe.py` checks CMS metadata before ingesting; if file shrank >10 lines, records protective `[prune-guard]` observation.
  - **Protected Overflow**: MEMORY.md overflow now uses `POST /v1/memory/ingest` (stable leaf node) instead of `POST /v1/conversation/observe` (volatile with decay).
  - **macOS Process Supervision**: 3 LaunchAgent plists for server, neural sidecar, and ingest timer. KeepAlive with 30s throttle, timer-based ingest every 30 min.
  - **`mdemg service` CLI**: 5 subcommands (install, uninstall, status, restart, logs) with platform support for macOS (launchd), Linux (systemd), and stub for unsupported platforms.
  - **Hook Registration Fix**: `claudeHookFiles()` expanded from 2 to 5 hooks. Matcher support added at group level in settings.local.json. Templates synced from active hooks.
  - **`mdemg data audit`**: Compare disk state vs CMS state for tracked claude-md files. Reports current/stale/shrank/deleted/not-ingested status plus service health and buffer state.

- **Training Data Capture Verification (TD-VERIFY)**: 17 verification tests across all 3 TSDB writers confirming column-position correctness, privacy scrubbing, response sanitization, and metadata completeness. Upgraded shared `mockPool` to capture `CopyFrom` row values. Documents scrubbing boundary (client vs writer), scrub asymmetry (embedding TextContent only), and empty TaskName regression guard. Feature doc: `docs/features/training-data-capture-verification.md`.

- **SanitizeResponse (Phase A)**: Unified LLM response cleaning pipeline (`StripThinkBlock` + `StripCodeFence` + `TrimSpace`) in `internal/llmclient/sanitize.go`. Wired into all 11 JSON-parsing call sites across 10 files. Enables local model deployment with think mode (Qwen3 `<think>...</think>` blocks).
- **System Prompt Hash**: SHA-256 hash of system prompt added to `InteractionRecord`, enabling training data curation by prompt version and stale data filtering.
- **RAFT Context Enrichment (Phase B)**: `RetrievalContext` struct captures which nodes were retrieved and their relevance scores alongside every LLM interaction. Wired into consulting and jiminy services. TSDB `llm_interactions` expanded from 22 to 26 columns. Migration 007.
- **ULTS Spec Framework (Phase C)**: Universal LLM Task Specification — machine-readable JSON contracts for all 16 LLM tasks. Defines system prompt hashes, output schemas, quality metrics, reward functions, and training config. Schema: `docs/tests/ults/schema/ults.schema.json`. Runner: `ults_runner.py` (16/16 specs pass).
- **Embedding Event Collection (Phase D)**: New `embedding_events` TimescaleDB hypertable captures every `Embed()`/`EmbedBatch()` call with parser metadata (element_kind, language, chunk boundaries, signature), provenance (call_site), and model info. `WithEmbeddingMeta` wired at 9 call sites. Privacy scrubbing applied to text content.
- **Retrieval Event Collection (Phase D)**: New `retrieval_events` TimescaleDB hypertable captures full retrieval pipeline execution (vector recall → BM25 → rerank → final results) with pre/post rerank scores for hard-negative mining. Migration 006.

### Fixed

- **TimescaleDB Compose Image Mismatch**: Observability compose used `timescale/timescaledb-ha:pg16` with mount path `/home/postgres/pgdata`, diverging from prod which uses `timescale/timescaledb:2.25.1-pg16` at `/var/lib/postgresql/data`. Standardized to match prod. Note: developers with existing `-ha` volumes should run `docker compose down -v` and recreate the TSDB container (data will be re-populated by the metrics recorder).
- **Config Comment Stale**: `TSDBRequiredSchemaVersion` comment said `(default: 3)` but actual default was `4`. Fixed.
- **Phase 47.2 Status Inconsistency**: AGENT_HANDOFF.md phase table showed 47.2 as `🔄 (APE INGEST pending)` but implementation was complete (`executeIngestStaleSpaces()` wired via RSIC dispatcher). Reconciled to `✅`.

- **Grafana Dashboard Remediation (7 dashboards, 2-day sprint)**:
  - **TSDB Volume Name Mismatch (CRITICAL)**: `docker-compose.prod.yml` used `tsdb-data` (hyphenated) creating Docker volume `docker_tsdb-data`, while real data lived in `docker_tsdb_data` (underscored, from observability compose). Container mounted an empty volume. Fixed by standardizing to `tsdb_data`. Pinned TimescaleDB image to `timescale/timescaledb:2.25.1-pg16` to match data format.
  - **MetricsRecorder Never Wired to TSDB Writer (CRITICAL)**: `NewMetricsRecorder()` was called with `nil` writer in `server.go`. `SetTSDBClient()` created a `tsdbWriter` but never called `SetWriter()` or `Start()` on the recorder. `FlushToTSDB()` returned immediately on nil writer check. Fixed by adding `SetWriter()` + `Start()` in `SetTSDBClient()` and `Stop()` in `Shutdown()`. First flush: 99 metric samples.
  - **J17/Jiminy Empty space_id Default**: Both dashboards had no `current` block on the `space_id` template variable, causing queries to return nothing. Added `"current": {"text": "mdemg-dev", "value": "mdemg-dev"}`.
  - **Neo4j "Heap Memory" Panel Showed Go Process Heap**: The Neo4j dashboard "Heap Memory" stat panel queried `mdemg_memory_heap_bytes` (MDEMG Go runtime), not Neo4j memory. Renamed to "Neo4j Memory" and changed metric to `mdemg_neo4j_container_mem_used_bytes`.
  - **RSIC Watchdog Escalation Green with No Data**: Watchdog Escalation Level stat panel showed bright green "No data" (threshold color implied healthy). Added `null+nan` special value mapping with `"Awaiting Data"` text and neutral color.
  - **FT Training Dashboard All TODO Queries**: All 6 panel queries were commented-out TODOs. Replaced with real SQL against `llm_interactions`, `ft_benchmarks`, `ft_training_cycles`, and `ft_model_versions` tables. Removed `coming-soon` tag.
  - **Overview/Neo4j Stat Panels Ungraceful Empty State**: Added `"noValue": "N/A"` to all stat panels across Overview (4 panels) and Neo4j (6 panels) dashboards.
  - **E2E Test Coverage**: Expanded Playwright suite from 21 to 70 tests. New test classes: `TestSpaceIdDefaultValue` (7 dashboards), `TestNoDataGraceful` (6 dashboards with nonexistent space), `TestFTTrainingStatusNotice`, `TestStatPanelNoValue`. Expanded `TestNoPanelErrors` to include ft-training, `TestDashboardNavigation` to all 6 dashboards.
  - **Dashboard Labels All "value" (6 dashboards)**: Every panel across Overview, Neo4j, Jiminy, J17, RSIC, and FT Training showed "value" as legend names and gauge labels because SQL queries return a column literally named `value` with no `displayName` configured. Fixed by: (1) adding `fieldConfig.defaults.displayName` on all single-series stat/timeseries/gauge panels (~40 panels), (2) adding `fieldConfig.overrides` with `byFrameRefID` matcher for all multi-series panels (gauges, piecharts, multi-query timeseries), (3) fixing broken overrides in Jiminy (Outcome Distribution/Trends) and J17 (Tokens & Replay) that matched non-existent series names, (4) adding metric-name overrides for historical trend panels that show raw `metric_name` strings. Graph Topology dashboard unaffected (no metric_samples queries).

- **J17 Protocol Pipeline: 7 Cascading Breaks Fixed**: The entire J17 tier selection, feedback loop, and trust progression pipeline was non-functional due to 7 interconnected breaks forming a cascading failure chain. ALL fixed simultaneously:
  1. **Code lookup queried wrong node population entirely** — `lookupConstraintCodes` queried by node ID, but guidance source nodes come from vector search (file-ingested `n_*` nodes) while constraint codes live on `Constraint` nodes (promoted from `ConversationObs` with UUID IDs). These are completely different populations with no shared IDs or edges. Replaced node-ID lookup with content-similarity matching: loads all space constraint codes, matches each guidance item by significant-word overlap (>=3 words). Result: 90% code coverage on first call.
  2. **Trust scores lost on server restart** — purely in-memory `map[string]*trustEntry`. Added `TrustStore` with write-behind Neo4j persistence (dirty-mark + 30s flush + hydrate-on-startup), following the RSICStore pattern.
  3. **Trust TTL too short (4h → 168h)** — idle sessions lost earned trust. Extended to 7 days since trust is now persisted to Neo4j.
  4. **Feedback state file gated on WARM=true** — `prompt-context.sh` only wrote guidance_id when warm store returned warm=true, but fresh guidance also needs feedback. Removed the WARM gate; added cleanup of stale state files.
  5. **Comprehension recording gated on missing codes** — consequence of break #1; avg_comprehension = 0%. Transitively fixed by the Cypher fix.
  6. **EffectivenessTracker TTL too short (30min → 2h)** — warm-cached guidance_ids expired before feedback arrived. Extended default TTL. Added re-registration on cache hits and `/v1/jiminy/latest` reads. Added warning log when feedback is dropped due to expired guidance_id.
  7. **feedbackCounts lost on restart** — in-memory map. Now included in TrustSnapshot persistence and hydrated on startup.

  With all fixes: codes resolve via edge traversal → tier selection produces T2 at trust 0.5 → feedback closes the loop → trust persists and survives restarts → after 6 follows, trust reaches 0.8 → T1 coded encoding activates.

  Additional breaks found and fixed via deep pipeline trace:
  8. **Shell hooks never entered J17 code paths** — `session-start.sh` and `pre-compact.sh` gated J17 logic on `${J17_ENABLED:-false}` but never sourced `.env`. Fixed by querying `/v1/jiminy/ready` for `features.j17` at runtime.
  9. **`JiminyCacheJ17Bypass` defined but never implemented** — config field existed (default: true) but `service.go` never referenced it. Cached guidance responses served stale tier assignments after trust evolved. Wired cache bypass on both `Get` and `Put` paths when J17 bypass is active.
  10. **Encoder tier thresholds not synced with config** — `ProtocolEncoder` hardcoded 0.8/0.4 thresholds, ignoring `J17_TRUST_HIGH_THRESHOLD` / `J17_TRUST_LOW_THRESHOLD` env vars. Added `SetTierThresholds()` call at encoder construction.
  11. **Protocol/sidecar metrics never flushed to TSDB** — `CollectProtocolMetrics()` only called from deprecated `/v1/prometheus` handler (returns 410). `MetricsRecorder.FlushToTSDB()` wrote gauge values that were never set, producing flat zeros in Grafana. Fixed by adding `SetPreFlushHook` on `MetricsRecorder` — server wires it to call `CollectProtocolMetrics()`, `CollectGuidanceMetrics()`, and `CollectHealthMetrics()` before each TSDB flush cycle.
  12. **Code matching applied only to `constraint` type items** — guidance items are classified as `correction`, `pattern`, `constraint`, etc. but any type can correspond to a codified constraint. Broadened code matching to ALL guidance types.

- **J17 Feedback Loop Broken — Tier Graduation Never Occurred**: The entire J17 tier graduation pipeline was fully implemented server-side but never activated because the hooks pipeline never delivered feedback. `prompt-context.sh` discarded `guidance_id`, `post-tool-observe.py` didn't correlate actions with guidance, and trust remained stuck at 0.5 (T1 requires 0.8). Fixed by: (1) capturing `guidance_id` from `GET /v1/jiminy/latest` and writing it to `~/.mdemg/.jiminy-guidance-state`, (2) reading the state file in `post-tool-observe.py` and firing `POST /v1/jiminy/feedback` after Write/Edit/Bash tool actions, (3) bootstrap codification trigger in `session-start.sh` when code_coverage=0. New constants: `FEEDBACK_COOLDOWN_SEC=30`, `FEEDBACK_STATE_MAX_AGE=1800`. 4 new functions in post-tool-observe.py, 3 integration tests.
- **Guidance Response Control Character Parsing Failure**: `prompt-context.sh` stored guidance responses in shell variables and piped through `echo | jq`, which failed because LLM-generated guidance text contains JSON control characters (U+0000–U+001F) that break both `jq` and shell variable expansion. Fixed by writing curl output to a temp file with inline `perl -pe 's/[\x00-\x08\x0b\x0c\x0e-\x1f]//g'` sanitization, then parsing with `jq` directly from the file.
- **Cache Hit Metrics Bypass**: `service.go` returned cached guidance without recording J17 protocol metrics, keeping `TotalEvents` at 0 despite active usage (~90% cache hit rate). Added `recordCacheHitMetrics()` method that replicates the recording logic from the cache-miss path. 10 unit tests.
- **RSIC Health Gauges Zero at Startup**: No RSIC assessment ran until the first periodic cycle (minutes after startup), leaving all `mdemg_rsic_health_*` Prometheus gauges at 0. Added bootstrap assessment goroutine that runs `Assess()` 10 seconds after startup to populate the cached report.
- **Integration Test Metric Name Mismatch**: `j17_metrics_test.go` and `j17_feedback_loop_test.go` searched for `j17_total_events` but the actual Prometheus metric is `mdemg_j17_events_total`. Fixed assertions.
- **Daemon .env Loading Order**: `mdemg start` now loads `.env` before YAML config, matching the documented config priority chain (`defaults → yaml → keychain → .env → env vars → flags`). Previously, `LoadYAMLConfig()` ran first and set Go zero-value `JIMINY_ENABLED=false` before `.env` could override it, causing Jiminy to be silently disabled in daemon mode despite `.env` having `JIMINY_ENABLED=true`.
- **CMS Control Character Sanitization**: `asString()` and `asStringSlice()` Neo4j record helpers now strip JSON-invalid control characters (U+0000–U+001F, except tab/newline/CR) before returning values. Prevents `json.Marshal` failures when Neo4j stores data containing embedded control chars from external ingestion.
- **GitHub Actions Node.js 20 Deprecation**: Pinned `aquasecurity/trivy-action@master` to `@v0.35.0` (eliminates supply chain risk from floating `@master`). Updated `gitleaks/gitleaks-action@v2` to `@v2.3.9` (latest v2 with Node.js 24 support). Deadline for Node.js 20 EOL: June 2, 2026.
- **Linux Systemd Bugs (6 fixes)**: (1) Split goreleaser archives by platform — systemd files no longer ship in macOS tarballs. (2) `install.sh` now persists systemd units to `/usr/local/share/mdemg/systemd/` alongside `/etc/systemd/system/`. (3) `install.sh` warns when systemd files are missing from archive instead of silently skipping. (4) `mdemg upgrade` now updates systemd unit files on Linux when units are present in the archive and already installed. (5) `mdemg teardown --full` now checks both `/etc/systemd/system/` and `/usr/lib/systemd/system/` for unit cleanup (.deb installs use the latter), plus cleans `/usr/local/share/mdemg/systemd/`. (6) `mdemg.service` ExecStartPre now tries `docker start mdemg-neo4j-dev` before falling back to `mdemg db start`, and uses `Wants=docker.service` instead of `Requires=` (fail-open).
- **Hook Tracking Inconsistency**: Added `.gitignore` negation patterns for 5 active hooks so they're properly tracked. Deleted orphan `pre-tool-enforce.py` (unregistered in settings, never fired). Extended `embed.go` to include `*.py` templates. Created 3 new canonical hook templates (`pre-bash-check.py`, `pre-compact.sh`, `post-tool-observe.py`). Synced `session-start.sh` template with live hook's degraded-health investigation checklist.

- **Neural Sidecar Health Check Broken (1,560 failures)**: Docker health check used `curl` in `python:3.12-slim` (no curl installed). Switched to Python `urllib.request` health check in both `Dockerfile` and `docker-compose.yml`. Removed `profiles: - neural` so sidecar always starts with Jiminy.
- **Sidecar Not Monitored by MDEMG**: Health prober, RSIC reflection, readyz endpoint, and watchdog were all unaware of sidecar status. Added: `probeSidecar()` to health prober, `sidecar_unhealthy` reflection pattern (#21) with `alert_sidecar_down` action, `neural_sidecar` readyz check (degraded, not blocking), watchdog `sidecar-unhealthy` anomaly detection, `IsSidecarHealthy()` on WatchdogSignalProvider interface.
- **Sidecar Missing from Production Compose**: `docker-compose.prod.yml` had no neural-sidecar service. Added service definition with health check, `J17_SIDECAR_URL` env var, dependency wiring, and resource limits.

- **Deep-Dive Remediation Sprint — Phase 1 (Error Sanitization)**: Control character sanitization for CMS Neo4j record helpers (`asString()`, `asStringSlice()`), guidance response shell handling (`prompt-context.sh`), and new `internal/sanitize/controlchars.go` package with unit tests.
- **Deep-Dive Remediation Sprint — Phase 2 (Documentation Remediation)**: Updated 11 documentation files referencing the deleted `/v1/prometheus` endpoint (returns 410 Gone since PR #213). Replaced with `/v1/metrics/snapshot` (JSON format). Added TimescaleDB as metrics backend in component tables, updated curl examples, added caveat notes to historical docs, documented `GET /v1/jiminy/protocol/status` endpoint.

### Changed

- **Gauge Dirty Flag — TSDB Zero-Noise Reduction**: Gauges now track a `dirty` flag (atomic int32). Only gauges mutated since the last flush cycle are written to TimescaleDB — clean gauges are skipped entirely. Reduces per-flush writes from 73 (all registered gauges) to only those actively being Set/Inc/Dec'd. Debug logging reports `flushed` vs `skipped_clean` counts. 3 new tests.

- **CUIDv2 for Guidance & Evaluation IDs**: `guidance_id` (from `Guide()`) and `eval_id` (from `Evaluate()`) now use CUIDv2 identifiers (`github.com/nrednav/cuid2 v1.1.0`) instead of UUID v4. CUIDv2 is collision-resistant, k-sortable, and shorter. Consumers treating these as opaque strings are unaffected. New dependency: `github.com/nrednav/cuid2`.
- **J17 Control-Loop Optimization (7 Gaps)**: Stress-test-driven fixes to J17 protocol internals — RSIC health formula correction (normalized to 0-1 range), MetricsCollector moved to struct field, `protocolDataCollector` nil guard, sidecar tier predictor idle-session guard, sidecar health retry loop, session-start.sh checkpoint error handling, guardrail prompt escaping. 3 new config vars: `JIMINY_CACHE_J17_BYPASS`, `J17_SIDECAR_URL`, `J17_SIDECAR_TIMEOUT_MS`. 11 new tests.

### Added

- **TimescaleDB Backup & Restore Service**: New `internal/tsdb/backup.go` provides pg_dump-based backup/restore via `docker compose exec -T timescaledb` (uses compose service name, not container name, avoiding `COMPOSE_PROJECT_NAME` fragility). Features: manifest sidecar files (SHA256 checksum, format version, metadata), count-based + age-based retention (mirrors `internal/backup/retention.go` semantics with cross-reference comment), ticker-based scheduler with configurable interval, bounded growth. CLI commands: `mdemg tsdb backup trigger`, `mdemg tsdb backup list [--limit N]`, `mdemg tsdb backup config`, `mdemg tsdb backup restore <file>`. 7 config env vars (`TSDB_BACKUP_ENABLED`, `TSDB_BACKUP_STORAGE_DIR`, `TSDB_BACKUP_COMPOSE_FILE`, `TSDB_BACKUP_SERVICE`, `TSDB_BACKUP_INTERVAL_HOURS`, `TSDB_BACKUP_RETENTION_COUNT`, `TSDB_BACKUP_RETENTION_MAX_AGE_DAYS`). Server wiring with clean shutdown. Default: disabled. 11 unit tests. 1 UOBS spec (`tsdb_backup_health`), 1 UOTS spec (`tsdb_backup_manifest`).
- **Grafana Alert Rule Validation**: All 21 TSDB-backed alert rules verified provisioned and functional. 4 critical alert SQL queries (P99 latency, Neo4j pool exhaustion, node count drop, RSIC watchdog force triggers) validated against live TimescaleDB — all parse correctly and handle empty result sets gracefully.

- **J17 Feedback Loop Closure**: Automated feedback delivery from Claude Code hooks to MDEMG server. `prompt-context.sh` captures `guidance_id` from warm guidance and writes state file. `post-tool-observe.py` reads state file after tool execution and fires `POST /v1/jiminy/feedback` with action summary (fire-and-forget, 30s cooldown). Bootstrap codification trigger in `session-start.sh` when code_coverage=0. Enables tier graduation: 6 consecutive positive feedbacks raise trust from 0.5 to 0.8 (T1 threshold). Feature doc: `docs/features/j17-feedback-loop-closure.md`.
- **Prometheus Observability Monitoring**: Self-monitoring blackbox probe (`http://localhost:9090/-/healthy`) added to `prometheus.yml`. 4 new alert rules in `deploy/docker/prometheus/alerts/observability.yaml`: `MDEMGScrapeTargetDown` (critical, 2m), `MDEMGPrometheusUnhealthy` (critical, 1m), `MDEMGPrometheusScrapeSlowdown` (warning, 5m), `MDEMGPrometheusStorageHigh` (warning, 10m). Feature doc: `docs/features/prometheus-observability-monitoring.md`.
- **J17 Feedback Loop Integration Tests**: 3 new tests in `tests/integration/j17_feedback_loop_test.go` — `TestJ17_FeedbackUpdatesMetrics` (full guide→feedback→metrics loop), `TestJ17_FeedbackEndpointReturnsOK`, `TestJ17_PrometheusHasJ17Metrics`. 2 new tests in `tests/integration/rsic_health_test.go` — `TestRSIC_HealthGaugesExist`, `TestRSIC_ReadyzContainsRSIC`.
- **Claude .md File Ingestion with Content-Hash Change Detection**: New `mdemg ingest-claude-md` CLI command discovers and ingests all Claude Code .md files (CLAUDE.md, MEMORY.md, AGENT_HANDOFF.md, VISION.md, plans, rules, auto-memory) with SHA256 content-hash deduplication. New `GET /v1/memory/node/meta` endpoint exposes per-node content hash, file size, line count, and ingestion timestamp for fast change detection. `IngestObservation` Cypher extended with `content_hash`, `file_size`, `line_count` properties and skip-unchanged logic. Tombstoning now sets `pruned_at` alongside `tombstoned_at`. Hook integration: `session-start.sh` runs background ingest on startup, `pre-compact.sh` runs forced ingest before compaction, `post-tool-observe.py` triggers targeted ingest on claude .md file writes. Files: `internal/cli/ingest_claude_md.go` (new), `internal/api/handlers_node_meta.go` (new), `internal/models/models.go`, `internal/retrieval/service.go`, `internal/cli/prune.go`, 3 hook files, UATS spec `node_meta.uats.json`.
- **UITS Framework (UxTS #12)**: Universal Iterative-Improvement Test Specification — formalizes T1-encoded content comprehension validation as declarative specs + reusable runner. Schema (`uits.schema.json`), 11 specs (10 architecture maps + 1 J17 constraint), Python runner with validate/validate-all/optimize/add-hashes/verify-hashes/profiles subcommands, versioned scoring profiles (comprehension 0.40, compaction 0.25, token_efficiency 0.20, fidelity 0.15), convergence detection (3 consecutive runs ≥9.0 mean, 0 WEAK questions). Follows UETS runner pattern with shared `uxts_runner_core.py` and `uxts_report.py` (Section 8A canonical report format). Soft-fail CI gate. All 11 specs have SHA256 integrity hashes.
- **T1 Architecture Map Optimization**: 9 iterative optimization rounds on 10 compact T1-encoded architecture maps for Jiminy context injection. Key techniques: structural grouping (COMPANION-APPS subsection), WHY-ANNOTATIONS footer, STATUS-EXCEPTIONS footer, column headers, list-final positioning for exceptions, parenthetical→key:value conversion. Final suite: 9.2/10 mean, 0 WEAK questions, 8 maps converged (≥9.0) + 2 accepted stable plateaus (8.6-8.8).
- **Synergy Optimization (Claude Code ↔ MDEMG)**: Reduces token overhead by ~60% by trimming CLAUDE.md (348→124 lines), MEMORY.md (220→40 lines), and auto-memory files (14→3). Displaced knowledge migrated to CMS via `scripts/synergy-migrate.sh` (Jiminy health gate, persistent flag, dev safety net). New `GET /v1/synergy/status` endpoint returns health metrics. New `mdemg synergy {status,migrate,check}` CLI commands. SynergyHealth added as 7th RSIC dimension (10% weight) with `scoreSynergy()`. Three new reflection patterns (#17 `synergy_jiminy_unhealthy` critical, #18 `memory_file_bloat`, #19 `synergy_overlap_drift`). Memory overflow interceptor in `post-tool-observe.py` auto-ingests MEMORY.md overflow to CMS. Synergy fingerprint in `session-start.sh`. 13 `SYNERGY_*` config vars. 17 unit tests, 1 UATS spec (9 assertions). ~100K tokens saved over a 50-turn session.
- **NLI Feedback Loop: Tier Effectiveness + Calibration** — Closes the feedback loop from NLI comprehension scoring to protocol tier adjustment via RSIC. Per-tier comprehension tracking (`RecordOutcomeWithTier`), tier effectiveness grading (`GradeTierEffectiveness`), curated RSIC datasets (`BuildTierEffectivenessDataset`), NLI calibration tracker (ring-buffer bias detection), double-counting bug fix in NLI→metrics gate, comprehension-aware `AdjustTierThresholds`, 2 new RSIC reflection patterns (#15 `j17_tier_ineffective`, #16 `j17_nli_calibration_drift`), NLI calibration weight in `scoreProtocol`, new `GET /v1/jiminy/protocol/tier-effectiveness` endpoint, tier effectiveness dataset generation at meso/macro RSIC cycle boundaries. 6 new config vars, 9 new Go files, 33+ new tests, 1 UATS spec.
- **J17 Neural Sidecar Promotion (NS-01 through NS-15)** — Promotes the neural sidecar (TierPredictor + NLIComprehensionScorer) from shadow-only observation to causal tier selection via a four-stage rollout (shadow → compare → canary → active). New `SidecarArbitrator` arbitration layer with per-mode logic, precedent-protected constraint codes (NS-10), multi-dimensional feedback (adherence + comprehension + applicability), NLI score-of-record option (NS-02), circuit breaker protection for sidecar HTTP calls (NS-06), structured sidecar telemetry (NS-07), sidecar health in ready/status endpoints (NS-08), expanded protocol training data records (NS-14), and comprehensive test matrix (20 arbitrator tests, 12+ tier predictor tests, 8+ NLI tests). 9 new config vars (`J17_SIDECAR_MODE`, `J17_SIDECAR_CANARY_PERCENTAGE`, `J17_SIDECAR_CONFIDENCE_FLOOR`, `J17_NLI_SCORE_OF_RECORD`, `J17_PRECEDENT_PROTECTED_CODES`, `J17_PRECEDENT_LOG_ENABLED`, `J17_SIDECAR_CB_ENABLED`, `J17_SIDECAR_CB_FAILURE_THRESHOLD`, `J17_SIDECAR_CB_TIMEOUT_SEC`). 4 new spec docs (contract, ML objectives, benchmark protocol, rollout plan). Key files: `sidecar_arbitrator.go` (new), `protocol_metrics.go`, `tier_predictor.go`, `nli_comprehension.go`, `service.go`, `config.go`.
- **Phase J17: AI-to-AI Communication Protocol** — Complete agent-to-agent communication protocol for the Jiminy inner-voice guidance service. 5 sub-phases (J17-1 through J17-5):
  - **J17-1: Three-Tier Encoding**: Compact protocol encoding with three tiers — T1 coded (~15 tokens, 80% of traffic), T2 telegraphic (~50-100 tokens, 15%), T3 full NL (~200+ tokens, 5%). Constraint codes are LLM-generated mnemonic kebab-case identifiers (e.g., `no-force-push-main`). New `internal/jiminy/encoder.go`, `sequence.go`, `protocol.go`.
  - **J17-2: Constraint Code Generation**: LLM-powered code generator (`internal/jiminy/codegen.go`) wired into constraint detection flow. Codes auto-generated when constraints are detected and stored as `constraint_code` property on Neo4j MemoryNode. Collision avoidance via startup population from existing codes. `ConstraintCodeGen` interface in conversation service for dependency injection. Constraint codes populated on GuidanceItems via batch Neo4j lookup in `Guide()`.
  - **J17-3: Trust & Session Tickets**: Per-session trust scoring (`internal/jiminy/trust.go`), signed session tickets for state persistence across context resets (`internal/jiminy/ticket.go`), escalation tracking (`internal/jiminy/escalation.go`). `ProtocolInfo.TrustScore` now populated in `Guide()` response.
  - **J17-4: Protocol Evolution**: RSIC-driven protocol metrics collection (`internal/jiminy/protocol_metrics.go`, `protocol_data_collector.go`), protocol evolution engine (`protocol_evolution.go`), NLI comprehension testing (`nli_comprehension.go`), extension negotiation (`extensions.go`). Protocol feedback and learn endpoints (`internal/api/handlers_j17.go`).
  - **J17-5: ML Tier Prediction**: Neural sidecar `TierModel` class (`neural/neural_sidecar/tier_model.py`) using CrossEncoder for tier prediction with `/protocol/predict-tier` endpoint. Training pipeline (`neural/neural_sidecar/train_protocol.py`) fine-tunes models from J17 protocol JSONL data collected during live usage. Go-side tier predictor (`internal/jiminy/tier_predictor.go`) calls sidecar with fallback to rule-based selection.
  - **4 new UATS specs**: `j17_bootstrap`, `j17_trust_modulation`, `j17_protocol_feedback`, `j17_protocol_learn` (10 total J17 UATS specs).
  - **Migration V0022**: `constraint_code` index on MemoryNode.
  - Config: `J17_ENABLED`, `J17_CODEGEN_ENABLED`, `J17_CODEGEN_PROVIDER`, `J17_CODEGEN_MODEL`, `JIMINY_TRUST_*`, `JIMINY_ESCALATION_*`, `JIMINY_PROTOCOL_*`, `NEURAL_TIER_MODEL`.
- **Jiminy J16: Full-Context Input** — Removed aggressive input truncation that limited Jiminy to ~10% of agent context. Guidance and evaluation prompts now receive full context (default 200K chars, configurable via 4 new `JIMINY_*` env vars). Fixed cache key collision bug where inputs sharing a 200-char prefix produced identical hashes. Increased all Jiminy-related timeouts to 30s to accommodate larger payloads. Increased guidance system prompt word limit from 500 to 1500 words.
- **Jiminy Init Wizard Integration**: `mdemg init` wizard now prompts for Jiminy inner-voice configuration (enabled by default). Users select a Jiminy-specific LLM model — defaults to `gpt-5.4-nano` (OpenAI) or `qwen3:8b` (Ollama) for cheap/fast JSON classification tasks. `--defaults`/`--quick` modes auto-configure with recommended settings. All 3 platform installers (macOS Homebrew caveats, Windows post-install, Linux post-install) updated to mention Jiminy. J13-J15 config vars added to jiminy-inner-voice.md. Fixed stale J15 defaults in documentation.
- **Debian Native Packaging (.deb + APT Repository)**: Native `.deb` package generation via GoReleaser nfpms plugin — no external `fpm` dependency needed. Packages include CLI binary, man pages, systemd template units (`mdemg@.service`, `mdemg-rsic@.service`, `mdemg-rsic@.timer`), and UxTS plugin manifest. Docker listed as `recommends` (not hard dependency) so the package installs cleanly on systems where Docker isn't yet configured. APT repository hosted on GitHub Pages (`apt-mdemg` repo) with GPG-signed Release files, automated by `apt-publish.yml` workflow triggered after each release. Users can install via `sudo apt install mdemg` after adding the repository. Sidebar `.deb` (built by Tauri) also included in the same APT repo. AUR PKGBUILD template provided for Arch Linux users (`packaging/aur/PKGBUILD`). Package scripts handle systemd daemon-reload on install, service stop on remove, and `/usr/share/mdemg` cleanup on purge. Flatpak was evaluated and rejected — MDEMG requires Docker for Neo4j, which conflicts with Flatpak's sandbox model.
- **Linux Distribution — Binary Builds + Sidebar Application**: Full Linux platform support with binary builds and desktop companion app. **Phase 1 (Binary Builds):** 4 goreleaser Linux build entries (mdemg + uxts-module, amd64 + arm64) using zig cross-compilation for CGO. Fixed `install.sh` systemd bug (units now bundled in release tarball for curl-pipe installs). Updated beta docs to reflect actual available install methods. **Phase 2 (Sidebar App):** Full Tauri 1.x implementation ported from macOS menubar — Rust backend (7 modules: types, api_client, cli_executor, server_discovery, instance_store, instance_scanner, 30+ commands) + vanilla JS frontend (pub/sub state, 7 tab renderers for Status/Memory/Learning/Neo4j/Config/Logs/RSIC, Catppuccin Mocha UI, polling manager, multi-instance support with auto-discovery). `cargo check` passes cleanly. Submodules: `packaging/mdemg_linux` (installer, systemd, docs), `packaging/mdemg-linux-sidebar` (Tauri app). Supports Ubuntu 20.04+, Debian 11+, Fedora 36+, RHEL 8+, Arch Linux.
- **AutoResearch Integration — Phase AR-1: RSIC Feedback Loop**: Post-cycle re-assessment populates `metrics_after` in `CycleOutcome` by running `Assessor.Assess()` after task execution, enabling before/after metric comparison. Success criteria evaluation checks `RSICTaskSpec.SuccessCriteria` against actual metric deltas — `CriteriaMet` and `CriteriaDetail` fields added to `CycleOutcome`. Auto-rollback for reversible actions (`tombstone_stale`, `graduate_volatile`) that didn't improve metrics via `SnapshotStore.Rollback()`. Prometheus counter `mdemg_rsic_rollbacks_total`. `UpdateCalibration` now only counts tasks as "success" if criteria were met. 8 new unit tests in `calibration_test.go`. Files: `internal/ape/calibration.go`, `internal/ape/cycle.go`, `internal/ape/types_rsic.go`.
- **AutoResearch Integration — Phase AR-2: Jiminy Guidance Effectiveness Tracking**: `POST /v1/jiminy/feedback` endpoint for correlating agent actions with Jiminy guidance. `GuidanceEffectivenessTracker` with LRU cache (TTL-based expiry, configurable via `JIMINY_EFFECTIVENESS_TTL_SEC`). `Guide()` now returns `guidance_id` (CUID2 unique identifier) in response for tracking. Outcome classification via text overlap scoring with negation detection: `followed`, `ignored`, `contradicted`, `unknown`. Config: `JIMINY_EFFECTIVENESS_ENABLED` (default: true), `JIMINY_EFFECTIVENESS_TTL_SEC` (default: 1800). 9 new unit tests. Files: `internal/jiminy/effectiveness.go` (new), `internal/jiminy/service.go`, `internal/jiminy/types.go`, `internal/api/handlers_jiminy.go`.
- **AutoResearch Integration — Phase AR-3: LLM-Powered Intelligence**: Three LLM classifiers following the EmergenceNamer pattern (OpenAI/Ollama dual provider, circuit breaker, JSON grammar-constrained output, fail-open). (R3) LLM Reflector for RSIC — analyzes `SelfAssessmentReport` + last 5 cycle outcomes + calibration confidence to produce pattern insights, merged with rule-based results via `deduplicateInsights()`. (J3) LLM Constraint Classifier — replaces keyword-based constraint detection with LLM classification (`must`/`must_not`/`should`/`should_not`/`none`), LRU cache (512 entries), falls back to improved keyword matching that correctly prioritizes "must not" over "must". (C1) LLM Query Classifier — replaces regex-based query type detection with LLM few-shot classification into `code`/`architecture`/`relationship`/`data_flow`/`symbol_lookup`/`generic` with temporal intent, LRU cache (256 entries, SHA256 keyed), multi-label support with most-permissive hint selection. All opt-in via config (`RSIC_LLM_REFLECT_ENABLED`, `CONSULTING_LLM_CONSTRAINTS_ENABLED`, `RETRIEVAL_LLM_CLASSIFY_ENABLED`, all default: false). 12 new config vars. 27 new unit tests across 3 test files. Files: `internal/ape/llm_reflector.go` (new), `internal/ape/self_reflect.go`, `internal/consulting/llm_classifier.go` (new), `internal/consulting/service.go`, `internal/retrieval/query_classifier.go` (new), `internal/retrieval/scoring.go`.
- **AutoResearch Integration Tests**: 8 integration tests in `tests/integration/autoresearch_test.go` covering AR-1 metrics_after/criteria fields, AR-2 guidance_id/feedback roundtrip/validation, AR-3 LLM reflector fail-open behavior.
- **AutoResearch Feature Documentation**: 3 new feature docs — `docs/features/rsic-feedback-loop.md` (AR-1), `docs/features/jiminy-effectiveness-tracking.md` (AR-2), `docs/features/llm-powered-intelligence.md` (AR-3). Updated `docs/features/jiminy-inner-voice.md` with feedback endpoint and effectiveness tracking references.

- **Transfer HTTP API Endpoints (S15 Extension)**: 3 new HTTP endpoints for space export/import via API (previously CLI-only). `POST /v1/admin/spaces/export` — profile-based export with all filter overrides (obs_types, tags, exclude_volatile, only_pinned, min/max_layer, no_observations, no_symbols). `POST /v1/admin/spaces/import` — chunked import with conflict modes (skip, overwrite, error), optional space_id remapping. `GET /v1/admin/spaces/export/preview` — lightweight entity count estimation without data transfer. 3 UATS contract specs (9 variants). 20-step shell acceptance test (`scripts/transfer-acceptance.sh`). 8 new Go integration tests (filter coverage + conflict modes + chunk size control). Makefile: `test-transfer`, `test-transfer-unit`, `test-transfer-integration`, `test-transfer-acceptance` targets.
- **Shareable Knowledge Export/Import (Phase S15)**: Export organization-level CMS knowledge for sharing between MDEMG instances. New `--profile shareable` export profile filters to domain knowledge only (learning, decision, correction, technical_note, insight, preference), excluding volatile/session-specific data. Composable filters: `--obs-types`, `--tags`, `--exclude-volatile`, `--only-pinned`. Import enhancements: `--target-space` remaps space_id, `--consolidate` runs hidden layer pipeline, `--re-embed` regenerates embeddings. Menubar: Knowledge Sharing UI section in Memory tab with export/import buttons, profile picker, and post-import options.
- **Sidecar Quickstart & Hook Enhancements (PR #127 Gap Closure)**:
  - **`mdemg sidecar quickstart`**: One-command onboarding — runs `init → install → up → attach-agent → generate-hooks` sequentially with state-aware skipping and failure reporting. Flags: `--profile`, `--agents`, `--endpoint`, `--dry-run`, `--format json`. New file: `internal/cli/sidecar_quickstart.go`.
  - **`generate-hooks` now produces `prompt-context.sh`**: Previously only generated `session-start.sh`. Now generates both hooks with parameterized endpoint/space_id/session_id from sidecar config. Registers both in `.claude/settings.local.json` via `mergeClaudeSettings()`. The generated `prompt-context.sh` performs CMS recall, Jiminy guidance, and background spreading activation per prompt.
  - **`attach-agent` enables `enableAllProjectMcpServers`**: After writing `.claude/mcp.json`, the claude-code adapter now also sets `enableAllProjectMcpServers: true` in `.claude/settings.local.json` to prevent MCP from being silently disabled. New `--no-settings` flag to skip. New function: `ensureProjectMcpEnabled()`.
  - PR #127 (`feat/claude-code-plugin`) closed — its gaps addressed in the sidecar system instead of a standalone plugin package.
- **Phase Jiminy: Jiminy Inner Voice Guidance**: `POST /v1/jiminy/guide` proactive guidance endpoint for coding agents. Orchestrates 4 knowledge sources in parallel (constraints via `consulting.Suggest()`, correction vector search, contradiction edge queries, frontier node detection) with 6s timeout. Returns structured `GuidanceItem` array plus pre-formatted `═══ JIMINY GUIDANCE ═══` prompt augmentation block for hook injection. MCP `jiminy_guide` tool for IDE integration. Hook integration in `.claude/hooks/prompt-context.sh` (guarded by `JIMINY_ENABLED`). Fixed `LearningEdgeBoost` dead code in scoring pipeline — now computed as `(activation - vectorSim) * beta` when CO_ACTIVATED_WITH edges contribute. New package: `internal/jiminy/` (7 files). Config: 6 `JIMINY_*` env vars (default: `JIMINY_ENABLED=false`). UATS: 2 specs (8 variants, 100% passing).
- **Jiminy J6b-J6e: Hook Distribution & Cross-Platform Support**:
  - **J6b**: Embedded hook templates in binary via `//go:embed`. New package `internal/cli/hook_templates/` (embed.go, prompt-context.sh, session-start.sh). `mdemg hooks install --type claude` installs parameterized hook scripts with `{{SPACE_ID}}`/`{{MDEMG_URL}}` placeholder substitution and registers them in `.claude/settings.local.json`. `mdemg hooks uninstall --type claude` removes them.
  - **J6c**: `mdemg init` wizard auto-installs Claude Code hooks when `.claude/` directory is detected. Auto-installs in `--defaults`/`--quick` mode.
  - **J6d**: Windows PowerShell hook equivalents (`prompt-context.ps1`, `session-start.ps1`) using native `Invoke-RestMethod`/`ConvertFrom-Json`. Platform detection selects `.ps1` on Windows, `.sh` on Unix. PowerShell scripts invoked via `powershell.exe -ExecutionPolicy Bypass` in settings.
  - **J6e**: Settings merge (`mergeClaudeSettings()`) preserves existing user settings when registering hooks. Detects existing MDEMG hooks by command path, updates in-place.
- **ANN Optimization Suite (10 optimizations)**: Comprehensive neural learning improvements across learning, retrieval, consolidation, and API subsystems. 28 new config parameters. Inspired by techniques from autonomous research (Muon optimizer, ResFormer, Gemma soft-capping, sliding window attention).
  - **Tanh Soft-Capping**: Smooth saturation replaces hard weight clamp at `wmax`. Prevents edge weight plateaus at 1.0, allowing continued learning. Formula: `wmax * tanh(w / wmax)`. Applied in both Go helper and Cypher (using Neo4j native `exp()`).
  - **Cautious Decay**: Skip decay for edges reinforced within a configurable window (`LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS`, default 24h). Uses existing `last_activated_at` property. Avoids wasteful decay→re-strengthen cycles.
  - **Multi-Rate Learning**: Context-specific eta multipliers computed in Cypher. Conversation observations learn 2x faster, config↔code edges 1.5x, same-directory nodes 1.2x. Multipliers stack (max ~3.6x), bounded by tanh cap.
  - **Time-Based LR Schedule**: Maturity-aware learning rate scaling. Cold spaces (0 edges) learn at 2x, learning spaces (1-10k) at 1x, warm (10k-50k) at 0.5x, saturated (50k+) at 0.25x. Edge count cached with 5-min TTL.
  - **Squared Activation**: Sharper, sparser activation signals via `β * max(0, activation - floor)²`. Eliminates low-activation noise (floor=0.05) while preserving strong signals.
  - **Local-First Activation Spreading**: Per-hop minimum weight thresholds. Hop 0 requires strong edges (≥0.5), hop 1 moderate (≥0.2), hop 2+ any (≥0.05). Degree normalization uses filtered edge count.
  - **Value Residual Bypass**: Additive bypass bonus for high-confidence vector matches (VectorSim > 0.85). Query-type gated: code queries 1.3x, architecture 0.5x. Max bonus ~0.03 — gentle nudge, not dramatic reranking.
  - **L0 Skip Connections (GROUNDED_BY)**: L5 emergent concepts get direct edges to most representative L0 observations. Prevents grounding loss when intermediate layers merge/prune. New `GROUNDED_BY` edge type with attention weight support.
  - **Negative Result Tracking**: `POST /v1/learning/negative-feedback` endpoint. Weakens CO_ACTIVATED_WITH edges or creates CONTRADICTS edges for rejected results. Caps at 20 nodes per request. Closes the negative-feedback loop in learning.
  - **Frontier Detection**: `GET /v1/memory/frontiers` endpoint. Identifies L3+ nodes with low outgoing degree, sufficient evidence, and no L5 parent — candidates for concept expansion. Read-only, LIMIT-bounded.
- **RSIC Orchestration Reset**: `POST /v1/self-improve/orchestration/reset` endpoint for clearing active cycles, cooldown, and dedupe state. Used for test isolation between UATS runs. Added to Makefile `test-api` target.
- **Phase 104: Active MCP Guardrails**: `POST /v1/memory/guardrail/validate` endpoint for proactive constraint enforcement. 4-step pipeline: diff parsing (regex symbol extraction for Go/Python/JS) → constraint retrieval (vector similarity + keyword match against `role_type: 'constraint'` nodes) → LLM evaluation (OpenAI/Ollama dual provider, Temperature 0.0, circuit breaker protection) → response building (re-validates LLM output against actual constraint types, maps `must`/`must_not` to Block, `should`/`should_not` to Warning). Fail-open on any pipeline error (returns Pass with warning). MCP `validate_changes` tool for IDE integration. New package: `internal/guardrail/` (6 files). Config: 6 `GUARDRAIL_*` env vars (default: `GUARDRAIL_ENABLED=false`). Closes Gap 4 from Cognitive Intelligence Gap Analysis.
- **Phase 103b: Emergence Model Evaluation & MLX Server Integration**: `LLM_ENDPOINT` env var decouples LLM text-generation endpoints from embedding endpoints (`EffectiveLLMEndpoint()` falls back to `OPENAI_ENDPOINT` if unset). Ollama `format` JSON schema parameter for grammar-constrained output — eliminates invalid JSON regardless of model quality. UETS (Universal Emergence Test Specification) framework: schema, 8 model specs (qwen2.5-72b-mlx, qwen2.5-14b-ollama, qwen3-8b-ollama, llama3.2-3b-ollama, llama3.2-3b-macstudio, llama3.2-3b-fp16-macstudio, llama3.3-70b-ollama, llama3.3-70b-macstudio), Python runner with validate/validate-all/add-hashes/verify-hashes/extract-clusters commands plus `--endpoint` override for remote execution, `num_ctx` config support. 7 cluster fixtures from Neo4j. Baseline: 7/7 passing — `llama3.2:3b` Q4_K_M recommended as default emergence model (fastest latency, top name quality). Updated FRAMEWORK_GOVERNANCE.md and UXTS_FRAMEWORK_MATRIX.md with UETS.
- **Phase 103: Dynamic Emergence**: LLM-driven concept naming for unclassified clusters during consolidation via `enable_dynamic_emergence: true` request flag. Dense `CO_ACTIVATED_WITH` clusters that don't match any hardcoded pattern (concern, config, temporal, UI, comparison, constraint) are sent to an LLM for automatic naming and classification. Creates `:MemoryNode:EmergentConcept` nodes with `role_type: 'dynamic_emergent'` and `proposed_label` from constrained set (pattern, principle, bridge, concern, workflow). Pipeline step at phase 22 (after hardcoded patterns, before dynamic edges). Union-find clustering on behavioral edges with idempotency via `NOT EXISTS` subquery. Fail-open per cluster (LLM errors skip individual clusters, don't abort run). Circuit breaker protection via `openai-emergence`/`ollama-emergence` breakers. 8 config vars (`EMERGENCE_*`), 11 unit tests. Fully backward compatible — existing consolidation unchanged unless emergence explicitly requested. Closes Gap 3 from Cognitive Intelligence Gap Analysis.
- **Phase 101: SME Synthesis Engine**: Optional LLM synthesis for `/v1/memory/consult` via `llm_synthesis: true` request flag. Retrieved graph nodes + user question sent to LLM (OpenAI/Ollama) with a prompt that constrains the model to synthesize ONLY from graph evidence — producing coherent organizational SME narrative with mandatory `(Node: <node_id>)` citations. Three graceful fallback paths: `llm_synthesis=false` (skipped), `SYNTHESIS_ENABLED=false` (nil synthesizer), LLM error (debug populated, response intact). Circuit breaker protection via `openai-synthesis`/`ollama-synthesis` breakers. New `Synthesizer` interface in consulting package. 5 config vars (`SYNTHESIS_*`), 13 new tests (5 service integration + 8 unit). Fully backward compatible — existing responses unchanged unless synthesis explicitly requested.
- **Phase 102: Intent Translation**: LLM-driven query rewriting before vector embedding for `/v1/memory/retrieve`, `/v1/memory/consult`, and `/v1/memory/suggest` via `translate_intent: true` request flag. Conversational queries ("Why do we use Redis?") rewritten into keyword-dense search strings ("Redis session state architecture decision caching") optimized for vector similarity against declarative knowledge graph text. Three graceful fallback paths: `translate_intent=false` (skipped), `INTENT_ENABLED=false` (nil translator), LLM error (fail-open, original query used). Circuit breaker protection via `openai-intent`/`ollama-intent` breakers. Temperature 0.0 for deterministic rewrites. Strict 2s timeout (NFR-1). Original question preserved for Phase 101 synthesis — only embedding input is translated. New `IntentTranslator` interface in retrieval package. 5 config vars (`INTENT_*`), 11 new tests (7 unit + 4 consulting integration). `translated_intent` string exposed in API response for transparency. Fully backward compatible.
- **Phase 97: Process Lifecycle + Secret Management**: `mdemg start/stop/restart/status` for background daemon mode with PID file management (`.mdemg/mdemg.pid`), log file (`.mdemg/logs/mdemg.log`), and auto-start of Neo4j container. `mdemg config set-secret/get-secret/list-secrets` for system keychain integration via `go-keyring` (macOS Keychain, Linux secret-tool, Windows Credential Manager). Config priority updated: defaults → yaml → keychain → .env → env vars → flags. Default `.mdemgignore` now includes `.env` and `.env.*` patterns. New dependency: `github.com/zalando/go-keyring`.
- **Phase 96: IDE + Repo Integration**: `mdemg hooks install/uninstall/list` for standalone git hook management (install with `--force`/`--space-id`, uninstall only MDEMG-managed hooks, list shows hook status). Claude Code MCP config generation (`.claude/mcp.json`) in `mdemg init` when `.claude/` directory is detected. `mdemg serve --mcp` launches MCP server as a subprocess alongside the HTTP server with automatic `MDEMG_ENDPOINT` propagation and graceful co-shutdown. Shared `InstallGitHook()`/`UninstallGitHook()` functions extracted from init.go for reuse.
- **Phase 95: Database + Embedding + Migrations**: Go-native migration runner with `//go:embed` embedded Cypher files — `mdemg db migrate` with `--status`, `--dry-run`, `--migrations-dir` flags. Statement splitter handles `CALL {} IN TRANSACTIONS` blocks and `//` comments. `mdemg db start/stop/status/shell` Docker container management with lightweight dev profile (1GB heap, 512MB page cache). `mdemg embeddings check` performs actual test embedding (reports dimensions and provider status). `mdemg serve --auto-migrate` applies pending migrations on startup. `REQUIRED_SCHEMA_VERSION` auto-detects from embedded migrations if unset. CI simplified: replaced cypher-shell download + shell loop with `./bin/mdemg db migrate`. 10 unit tests for migration runner. Removed dead code (`countMigrations`, `portFromString` from init.go).
- **Phase 94: Config Simplification + Project Init**: `mdemg init` interactive wizard for project scaffolding (generates `.mdemg/config.yaml`, `.mdemgignore`, git hooks, IDE configs). `mdemg config show` displays effective configuration with source annotations (yaml/env/default). `mdemg config validate` checks YAML syntax and probes Neo4j/embedding reachability. YAML-to-env-var bridge: `.mdemg/config.yaml` exposes ~20 curated settings, converted to env vars before `FromEnv()` — zero changes to existing config parsing. Layered priority: defaults → yaml → .env → env vars → flags. `.mdemgignore` gitignore-style patterns applied during `mdemg ingest` file walk. Shared `loadConfig()` helper wired into all CLI commands. Schema version fixed in `.env.example` (4→17). Git hook updated to prefer `mdemg ingest` over legacy `ingest-codebase`.
- **Phase 93: Unified CLI Foundation**: Merged 12 separate Go binaries into single `mdemg` binary using Cobra CLI framework. Command tree: `serve`, `mcp`, `ingest`, `consolidate`, `decay`, `prune`, `extract-symbols`, `watch`, `db reset`, `space <sub>`, `plugin <sub>`, `version`. Shared Neo4j type conversion utilities in `internal/cli/neo4jutil/`. Languages package moved to `internal/languages/`. Old binaries converted to deprecation shims. Build-time version injection via ldflags. CI updated to build and test with unified binary. Makefile: `build-cli` target.
- **Phase 92: Gap Analysis — Deployable MDEMG Package**: Comprehensive gap analysis (`docs/specs/phase92-gap-analysis.md`) identifying 15 gap categories between current state and Phase 100 (deployable `mdemg` package for developers). Phase dependency graph mapping Phases 93-100: Unified CLI (93), Config + Init (94), Database + Embedding (95), IDE + Repo Integration (96), Process Lifecycle + Security (97), Build + Release (98), Onboarding (99), Deployable Package (100). AGENT_HANDOFF.md updated with full Phase 92-100 roadmap.
- **Phase 38: UNTS Hash Verification REST API**: 8 REST endpoints under `/v1/hash-verification/` exposing the UNTS hash verification registry via HTTP. Handlers call Registry/Scanner directly (not through gRPC). Endpoints: register, get, list, verify, verify-all, update, revert, scan. 8 UATS specs with 19 variants (100% pass rate). Config: `UNTS_ENABLED` (default: false), `UNTS_BASE_PATH` (default: "."). Makefile targets: `test-unts`, `test-unts-uats`.
- **Phase 91: RSIC Observability & Operations**: 12 Prometheus metrics (`mdemg_rsic_*`) across cycle, trigger, action, safety, watchdog, and calibration domains. Grafana dashboard with 16 panels across 4 rows (Overview, Cycles, Actions, Watchdog). 8 Prometheus alert rules (cycle failure, force triggers, rejection rate, action failures, safety blocks, low confidence, high decay, duration spikes). Operations Runbook §11 with failure mode playbooks, safe mode instructions, and SLO targets. UATS spec for Prometheus RSIC metric validation.
- **Phase 90: RSIC Conformance & CI Gating**: 6 Go integration tests (`tests/integration/rsic_test.go`), CI UATS pipeline split (core merge-gating vs embedding best-effort), UATS tag filtering (`--include-tag`/`--exclude-tag`), sequential mode for idempotency testing, Make targets (`test-rsic`, `test-rsic-unit`, `test-rsic-integration`, `test-rsic-uats`). Idempotency spec promoted from drafts, 7 draft stubs cleaned up. 109 specs, 180 variants, 100% passing.
- **Phase 89: RSIC Persistence & Multi-Space Correctness**: Write-behind persistence via Neo4j `RSICState` nodes (30s flush goroutine, dirty key tracking). Multi-space compliance (`RSICWatchdogSpaceID` config). DateTime coercion for Neo4j. Health endpoint persistence block. Session identity aggregation via `SessionTracker.GetAllStates()`.
- **Phase 88: RSIC Safety & Policy Enforcement**: Safety validator with blast-radius estimation and protected-space blocking. Dry-run mode with mutation deltas. Rollback support (tombstone/graduate reversible). `SafetyVersion = "phase88-v1"` stamped on all outcomes. 3 UATS specs.
- **Phase 87: RSIC Orchestration Activation**: Trigger source tracking (`manual_api`, `micro_auto`, `session_periodic`, `macro_cron`, `watchdog_force`). Cooldown/dedupe/overlap policy with configurable bounds (`RSIC_TRIGGER_COOLDOWN_SEC`, `RSIC_TRIGGER_DEDUPE_SEC`). Trigger metadata in cycle outcomes, health, and history responses. Macro cron scheduler, session-periodic meso on resume. 3 UATS promoted from drafts.
- **Cross-Space Graph Orphan Cleanup**: `POST /v1/memory/cleanup/graph-orphans` — scans all or specified spaces for zero-edge nodes with scan/consolidate/archive/delete fix actions. Protected space enforcement (mdemg-dev skipped for destructive actions). UATS spec with 6 variants.
- **Phase 49 Complete (LLM Plugin SDK)**: All deliverables verified — plugin scaffolding (`cmd/plugin-scaffold/`), validation framework (`cmd/plugin-validate/`, `internal/plugins/validator.go`), creation API (`POST /v1/plugins/create`, `GET /v1/plugins/{id}`, `POST /v1/plugins/{id}/validate`), capability gap detection (`internal/gaps/`). UATS specs: `plugin_create.uats.json` (6 variants), `capability_gaps.uats.json`, `capability_gaps_full.uats.json` (4 variants), `gap_interviews.uats.json`.
- **Phase 9.4: Plugin-Specific Triggers**: File watcher REST API (start/status/stop), event-driven module updates with `EventDispatcher`, wildcard subscription support. 3 UATS specs, 7 variants.
- **Phase 80: CMS ANN Meta-Cognition**: Server-side anomaly detection on resume/recall, HTTP headers (`X-MDEMG-Memory-State`, `X-MDEMG-Anomaly`), session anomalies endpoint, signal effectiveness endpoint. WatchdogSignalProvider for multi-dimensional monitoring. Hebbian SignalLearner for adaptive enforcement. 3 UATS specs. Config: 4 `METACOG_*` env vars.
- **Phase 76: Neo4j State Monitor**: `GET /v1/neo4j/overview` — consolidated database health, per-space statistics (nodes, edges, layers, health score, staleness, orphans, learning edges), and backup overview. 6 batched Cypher queries. 1 UATS spec with 7 body assertions.
- **Phase 75C: L5 Emergent Layer**: BRIDGES edge type, evidence threshold 3→1, L5 edges with COMPOSES_WITH, L3+ source layer for emergence, co-activation fix, dynamic edges via pipeline. Split pipeline execution (`RunPhaseRange`). New config: `L5SourceMinLayer`.
- **Phase 70: Neo4j Backup & Restore**: Full database dump via `docker exec neo4j-admin` and partial space-level export via `.mdemg` format. Ticker-based scheduler (full weekly, partial daily), retention engine (count/age/storage-based cleanup), restore from full dump. 7 API endpoints under `/v1/backup/`, 7 UATS specs, migration V0013. Config: 11 `BACKUP_*` env vars (default: `BACKUP_ENABLED=false`). E2E verified against live mdemg-dev space (21,033 nodes, 232,434 edges, 101MB backup).
- **Phase 51: Web Scraper Ingestion Module**: Plugin-based web scraping with section chunking, quality scoring, dedup, and user review workflow. 6 API endpoints under `/v1/scraper/`, 6 UATS specs, UPTS-validated MarkdownParser. Config: 8 `SCRAPER_*` env vars (default: `SCRAPER_ENABLED=false`).
- **Diagnostics Framework**: Structured `Diagnostic` struct with severity, code, message, parser, and context fields; `DiagnosticSummary` for aggregate reporting; `TruncateContentWithInfo()` and `NewDiagnostic()` helpers; wired into `walkCodebase` with summary logging
- **9 New Language Parsers**: C# (.cs), Kotlin (.kt, .kts), Terraform/HCL (.tf, .tfvars), Makefile (.mk, Makefile), Protocol Buffers (.proto), GraphQL (.graphql, .gql), OpenAPI (via content detection), Markdown (.md), XML (.xml, .csproj) — all with UPTS specs, test fixtures, and diagnostics support
- **UPTS Evidence Validation**: Structural consistency checks in the Go-native test harness — validates LineEnd consistency, CodeElement ranges, symbol containment, and LineEnd matching against specs; enabled for Go and Rust parsers
- **27 UPTS-Validated Parsers**: All 27 language parsers pass CI validation (100% pass rate) — Go, Python, TypeScript, Rust, Java, C, C++, CUDA, SQL, Cypher, YAML, TOML, JSON, INI, Dockerfile, Shell, C#, Kotlin, Terraform, Makefile, Protocol Buffers, GraphQL, OpenAPI, Markdown, XML, Lua, Scraper Markdown
- **UPTS Summary Document**: `docs/lang-parser/lang-parse-spec/upts/UPTS_SUMMARY.md` — comprehensive parser table with parent-child relationships, pattern coverage, and validation commands

### Fixed

- **`space copy` infinite loop**: Cypher-based deduplication was unreliable for copy operations (creates new nodes, no natural termination). Replaced with two-phase approach: collect all source node IDs upfront, then batch by explicit ID list (`WHERE src.node_id IN $ids`). Added `:MemoryNode` label to all MATCH clauses for consistency with `delete` and `rename`. Previously caused 14,239 orphaned nodes from a 10-node source.
- **Full backup "database in use" failure**: `runFullBackup()` attempted `neo4j-admin database dump` which requires exclusive database access (incompatible with running MDEMG server). Replaced with logical export by delegating to `runPartialBackup()` with all spaces. Both `full` and `partial_space` backups now produce portable `.mdemg` files that work with a live database. Restore auto-detects format (`.mdemg` logical import vs legacy `.dump` physical restore).
- **Snapshot API plain text error responses**: All 7 snapshot handlers (`handleSnapshots`, `handleSnapshotByID`, `handleListSnapshots`, `handleCreateSnapshot`, `handleGetSnapshot`, `handleDeleteSnapshot`, `handleLatestSnapshot`, `handleCleanupSnapshots`) used `http.Error()` which returns plain text. Replaced 20+ calls with `writeJSON()` for consistent `{"error": "..."}` JSON responses with proper `Content-Type: application/json` header.
- **Linear module 503 error handling**: Unconfigured or unavailable Linear module now returns 503 (Service Unavailable) instead of 500/400. Handler detects gRPC `Unimplemented`/`unknown service` errors and `not configured`/`api_key` error strings. All 8 Linear UATS variants passing.
- **RSIC orchestration state leaking across test runs**: `OrchestrationPolicy.Hydrate()` restores cooldown records from Neo4j on startup. Previous UATS runs left 300s cooldown state, causing 409 on subsequent runs. Fixed by adding `ResetState()` method and calling it in Makefile before UATS runs.
- **UATS deep merge override for missing_space_id variants**: Empty objects `{}` still deep-merge with base request body/query. Fixed `frontier_detection` and `learning_negative_feedback` specs by explicitly setting `"space_id": ""` in variant body/query to override base values.
- **Phase 90: CycleOutcome missing `idempotency_key`**: Added field to struct and all 4 return paths in `RunCycle`. Dedup fast-path response now includes `trigger_source` and `idempotency_key`. `Hydrate()` filters expired trigger records to prevent stale cooldown on server restart.
- **Ingestion whitelist**: `getEnabledLanguages()` now includes all 27 registered parsers (was missing yaml, toml, ini, dockerfile, shell, cuda, cypher + new parsers)
- **OpenAPI parser routing**: YAML parser now skips files containing `openapi:` or `swagger:` markers to ensure OpenAPI parser handles them (Go map iteration order is non-deterministic)
- **Makefile parser `:=` assignment**: Fixed disambiguation logic that incorrectly rejected `:=` variable assignments as target definitions

### Previously Added

- **UPTS Go-Native Test Harness**: `upts_test.go` and `upts_types.go` — validates all language parsers directly via `go test` without external dependencies
- **Phase 9.5: Conflict Resolution & Consistency**: Data integrity during concurrent updates, orphan detection, and edge consistency
  - Version tracking: `version` counter incremented on every MERGE update, archive, and unarchive operation
  - `last_ingested_at` timestamp on every ingest update, distinct from `updated_at`
  - Conflict logging: DEBUG log when a node is updated (update_count > 1) with version and update_count
  - `POST /v1/memory/cleanup/orphans` — Orphan detection endpoint with `list`, `archive`, and `delete` actions; supports `dry_run` mode and `limit` parameter
  - Protected space enforcement: `delete` action blocked on protected spaces (e.g., `mdemg-dev`)
  - `edges_stale` flag: set on nodes when embedding changes during re-ingest
  - `RefreshStaleEdges()` method: refreshes ASSOCIATED_WITH edge weights for stale nodes, propagates staleness to parent hidden nodes
  - Edge refresh wired into consolidation pipeline as Step 6
- **Phase 9.4: Plugin-Specific Triggers**: Event-driven integration layer for external event sources
  - `TriggerEventWithContext()` on APE scheduler — passes `space_id`, `ingest_type`, and other context to APE modules
  - `POST /v1/webhooks/linear` — Linear webhook endpoint with HMAC-SHA256 signature verification, 10s debouncing, and automatic observation ingestion via plugin Parse
  - `cmd/watch` — Standalone file watcher binary using fsnotify; monitors directories for changes and triggers file ingestion via API
  - APE event wiring: `source_changed` and `ingest_complete` events fired after all ingest completion paths (batch, file, codebase)
  - Config: `LINEAR_WEBHOOK_SECRET`, `LINEAR_WEBHOOK_SPACE_ID` environment variables
- **Phase 9.1: Git Commit Hooks**: `--quiet` and `--log-file` CLI flags for `ingest-codebase`; git hook passes `--quiet` by default
- **Phase 9.2: Time-Based Scheduled Sync**: TapRoot freshness tracking (`last_ingest_at`, `last_ingest_type`, `ingest_count`), `GET /v1/memory/spaces/{space_id}/freshness` endpoint, periodic scheduled sync via `SYNC_INTERVAL_MINUTES`, stale space detection, MCP `memory_space_freshness` tool
- **Phase 9.3: User-Triggered Re-Ingestion**: Wired `runIngestJob()` to CLI binary with streaming progress via `--progress-json`
- **File-level re-ingest endpoint**: `POST /v1/memory/ingest/files` for targeted file re-ingestion (sync ≤50 files, background >50)
- **MCP tool `memory_ingest_files`**: Re-ingest specific files from IDE
- **CLI `--progress-json` flag**: Structured JSON progress events on stdout for `ingest-codebase`

### Fixed (Additional)

- **MCP `memory_ingest_trigger` field mismatch**: `source_path` → `path`, `mode` → `incremental`, `exclude_pattern` → `exclude_dirs`

### Deprecated

- **`/v1/memory/ingest-codebase` endpoint**: Superseded by `/v1/memory/ingest/trigger` with superior job tracking; responses include `Deprecation` header
- **Linear CRUD Operations**: Full Create/Read/Update/Delete for issues, projects, and comments via Linear GraphQL API
- **CRUDModule protobuf service**: Generic gRPC service with entity_type dispatch and map fields, reusable by future plugins
- **Linear REST API endpoints**: `/v1/linear/issues`, `/v1/linear/projects`, `/v1/linear/comments` with full HTTP method dispatch
- **Linear MCP tools**: 6 tools for IDE integration — `linear_create_issue`, `linear_list_issues`, `linear_read_issue`, `linear_update_issue`, `linear_add_comment`, `linear_search`
- **Workflow engine**: Config-driven YAML automation with triggers (on-create/update/delete), conditions (eq/neq/contains/changed_to/exists), and actions (add-comment, auto-assign, auto-label, auto-transition, set-field)
- **Plugin additional_services**: Backward-compatible mechanism for modules to declare extra capabilities (e.g., INGESTION + CRUD)
- Edge-Type Attention for query-aware activation spreading
- Query-type detection (symbol_lookup, data_flow, architecture, generic)
- RetrievalHints for fine-grained retrieval control
- Layer-specific temporal decay (L0: 0.05/day, L1: 0.02/day, L2: 0.01/day)
- Hybrid edge strategy with query-aware graph expansion
- Universal Parser Test Schema (UPTS) v1.1 with 16 language parsers passing
- Universal API Test Schema (UATS) v1.0.1 with 41 endpoint specs
- Conversation Memory System (CMS) with hooks and protocols
- MCP server for IDE integration
- Codebase ingestion CLI and API endpoint (`/v1/memory/ingest-codebase`)
- Hidden layer concept abstraction and consolidation
- Hebbian learning loop with co-activation edge creation
- Edge weight decay and pruning CLI commands
- Plugin system with scaffold and validation tools
- CI pipeline with build, test, lint, and Trivy security scanning
- SECURITY.md with vulnerability reporting policy
- CONTRIBUTING.md with development guidelines

### Fixed (Parser and Spec Quality)

- **Parser symbol extraction**: Fixed C, C++, CUDA, SQL, Cypher parsers for correct function name extraction (was extracting parameter names)
- **CUDA multi-line kernel signatures**: Kernel pattern now handles `__global__` functions with parameters spanning multiple lines
- **SQL DEFAULT value parsing**: Parenthesis balancing prevents truncation of function calls like `gen_random_uuid()`
- **Cypher symbol types**: Labels, relationships, constraints, and indexes now emit correct UPTS types
- **C++ `static const` extraction**: Parser now recognizes `static const` and `static constexpr` constants
- **UPTS spec corrections**: Fixed 45 spec authoring errors across C (16), C++ (21), and CUDA (16) specs where auto-generated entries had parameter names instead of function names
- VectorSim floor to prevent spurious learning edges
- Migration files excluded from learning edge creation
- L0-only learning scope to reduce noise
- File extension filter handling for `#symbol` suffix queries
- Duplicate node prevention via idempotent ingestion

### Changed

- Standardized symbol field names to UPTS across codebase
- Reorganized documentation structure

## [FSD-2026-001] — 2026-03-19

### Added

- **Constraint enforcement hook (GAP-01)**: PreToolUse hook blocks/warns on Write/Edit based on guardrail validation
- **Contradiction detection (GAP-02)**: Embedding similarity + negation heuristics, with optional NLI sidecar enhancement
- **Effectiveness feedback persistence (GAP-03/08)**: GUIDANCE_OUTCOME edges, Bayesian confidence evolution for constraints
- **Cross-constraint conflict detection (GAP-04)**: Pairwise conflict scan via embedding similarity + type opposition
- **Dynamic confidence inheritance (GAP-05)**: Constraints inherit detection confidence instead of hardcoded 0.8
- **LLM constraint classification gate (GAP-06)**: NLI sidecar confirms/rejects regex detections
- **Constraint scope filtering (GAP-07)**: File path glob matching limits constraint applicability
- **Determinism score metric (GAP-09/19)**: D = (informed/total) * compliance * coverage
- **Jiminy guidance cache (GAP-10)**: LRU + TTL cache for sub-second repeat queries
- **Configurable dimension weights (GAP-11)**: Semantic/temporal/coactivation weights via config
- **Prompt injection sanitization (GAP-14)**: Strips injection phrases, role lines, code fences, excessive repetition
- **Authority level filtering (GAP-20)**: org_policy > team_standard > preference hierarchy
- **Neural re-ranker Python sidecar (NR-2)**: FastAPI with cross-encoder re-ranking + NLI classification
- **Go neural integration (NR-3)**: HTTP client with circuit breaker for sidecar /rerank endpoint
- **Training data collection (NR-1)**: Async JSONL logging of (query, candidate, score) tuples
- **Python neural training pipeline (NR-4)**: `train.py` (fine-tune cross-encoder from collected JSONL data with configurable epochs, batch size, validation split, checkpoint resume), `evaluate.py` (offline evaluation comparing neural vs LLM re-rank scores, top-k reporting), model versioning with timestamped checkpoint directories, CLI entrypoints `mdemg-neural-train` and `mdemg-neural-evaluate`
- **LLM client deduplication (F21)**: Extracted unified `internal/llmclient/` package (725 lines) consolidating duplicate OpenAI/Ollama HTTP client code spread across 5 packages (`summarize`, `consulting`, `retrieval`, `hidden`, `guardrail`). Single `Client` type with `Complete()` and `CompleteWithUsage()` methods. Ollama returns `tokens=0` (no usage reporting). 16 unit tests in `internal/llmclient/client_test.go`.
- **NR-5 + FSD-Final**: E2E acceptance script (`scripts/fsd-acceptance.sh`, 23 test steps), Docker Compose neural sidecar service (`neural/Dockerfile`, profile `neural`), 6 new Makefile targets (`test-fsd`, `test-fsd-unit`, `test-fsd-integration`, `test-fsd-acceptance`, `build-sidecar`, `test-sidecar-python`), phase spec document (`docs/specs/phase-fsd-constraint-lifecycle.md`, 520 lines)
- 8 new API endpoints, 38 new config parameters (all default disabled), 12 new UATS specs
- 2 Neo4j migrations: V0020 (constraint lifecycle), V0021 (constraint conflicts)

### Changed

- `SanitizeUserContext` now appends "..." when truncating
- Constraint retrieval accepts `trustLevel` parameter for authority-based filtering
- Activation spreading uses configurable dimension weights instead of hardcoded values

### Technical

- 70+ files changed across FSD-2026-001 (33 new + 21 modified in core, plus acceptance, Dockerfile, Makefile, spec doc)
- 171 total UATS specs (up from 159)
- All Go tests passing, 0 lint issues

## [0.1.0] - 2026-01-15

### Added (0.1.0)

- Initial project scaffolding
- Neo4j graph database integration with vector indexes
- Semantic retrieval with embedding-based search (OpenAI, Ollama)
- Graph-based knowledge representation with memory nodes
- Core API server with health, ingest, retrieve, and consolidate endpoints
- Database migration framework (10 idempotent Cypher migrations)
- Docker Compose configuration for Neo4j
- Environment configuration via `.env` with example template

[Unreleased]: https://github.com/reh3376/mdemg/compare/v0.7.4...HEAD
[0.7.4]: https://github.com/reh3376/mdemg/compare/v0.7.3...v0.7.4
[0.7.3]: https://github.com/reh3376/mdemg/compare/v0.7.2...v0.7.3
[0.7.2]: https://github.com/reh3376/mdemg/compare/v0.7.1...v0.7.2
[0.7.1]: https://github.com/reh3376/mdemg/compare/v0.7.0...v0.7.1
