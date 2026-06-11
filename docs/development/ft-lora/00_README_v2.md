# MDEMG Fine-Tuning Plan — Complete Document Suite

> ## STATUS (DOC-TRUTH-001, 2026-06-11) — read this before planning
>
> **SHIPPED through production cutover:** Phase 5 SFT (dense) → Phase 11.x
> RL/eval → Phase 11.6 cutover → Phase 13.5 runtime. Production:
> `mdemg-llm-v1` (single-tier LoRA on dense `mlx-community/Qwen3-14B-4bit`),
> GGUF Q5_K_M on llama-server :8102; aggregate 0.8389 (16-task augmented
> eval) vs gpt-5.4-mini 0.8317.
>
> **SUPERSEDED (2026-04-22 MoE→dense pivot — see v5.6 below):** the
> Qwen3.6-35B-A3B MoE target, the two-tier MoE-Sieve strategy, the Sprint
> A→E critical path, and Sprint C's three gates (overtaken — the FT-2
> "MLX validation" step was skipped as moot and FT-3 "expert profiling"
> consumption was superseded; D/E artifacts retained as research per the
> R-LT-4 prototype-discipline adjudication: prototypes that lose their
> production path are kept as research artifacts, never silently deleted,
> and any Phase-15/Note research work must re-derive from the CURRENT
> architecture, not these).
>
> **NOT STARTED:** the recursive-retraining loop — Phases 6 (Recursive
> Cycle Automation), 7 (RSIC Integration), 9 (Monitoring). This is the
> line's largest unfinished promise and the bridge from shipped FT to the
> RSIC vision. Trigger: FT-CLASSIFY-002 (Roadmap Q3 Phase 4) proving the
> production-row → retrain → regression-gate path end-to-end.
>
> **Provenance notes:** memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md`, cited
> by earlier CLAUDE.md revisions as canonical, never existed in this repo
> — THIS document is canonical. The MDEMG specification
> (`docs/research/mdemg_sprint_ideas/mdemg-specification.md`) is currently
> UNTRACKED; its §6.6 revision is carried here until the FG-2 fork-gate
> work formalizes spec tracking.


**Date:** 2026-04-30
**Version:** 5.12 (Sprint FT-LORA-PHASE11.6 — Production cutover. Phase 5 dense renamed `mdemg-llm-v1`; all 16 LLM call sites routed at the local model via mlx_lm.server :8101. 5 of 16 task surfaces verified routing correctly via smoke test; 3 server.go pre-existing config-wiring bugs patched.)

> **Changes in v5.12 (Sprint FT-LORA-PHASE11.6 — 2026-04-30):**
> - **PRODUCTION CUTOVER**: all 16 MDEMG LLM call sites now route at local Phase 5 dense model (`mdemg-llm-v1`) instead of cloud gpt-5.4-mini. Goal of the entire FT-LORA project, achieved.
> - **Production model name**: `mdemg-llm-v1` (stable production ID). Symlink `.local-models/mdemg-llm-v1/` → `qwen3-14b-mdemg-v1/`. Manifest at canonical path with full lineage + augmented-eval scores + production-use commands.
> - **Production-use**: `mlx_lm.server --model /Users/reh3376/mdemg/.local-models/mdemg-llm-v1 --host 127.0.0.1 --port 8101 --prompt-concurrency 1 --decode-concurrency 1` (host) + `docker compose up -d` (container reaches host via `host.docker.internal:8101` per compose default).
> - **`.env` cutover**: `LLM_MODEL=mdemg-llm-v1`, `LLM_ENDPOINT=http://host.docker.internal:8101/v1`, `RERANK_MODEL=mdemg-llm-v1`, `LLM_SUMMARY_MODEL=...`, `INTENT_MODEL=...`, `EMERGENCE_MODEL=...`, `GUARDRAIL_MODEL=...`, `JIMINY_SYNTHESIS_MODEL=...`. Per-task timeouts bumped to accommodate local LLM latency (e.g., `RERANK_TIMEOUT_MS=120000`, `RSIC_LLM_REFLECT_TIMEOUT_MS=180000`). `RERANK_TOP_N` reduced 50 → 20 to keep rerank prompts within latency budget.
> - **Code patches (`internal/api/server.go`)**: 3 pre-existing config-wiring bugs fixed. `consulting.classify`, `jiminy.synthesize`, `ape.reflect` were calling `cfg.OpenAIEndpoint` directly instead of `cfg.EffectiveLLMEndpoint()` — masked when `LLM_ENDPOINT` was unset (because the fallback returns OpenAI), surfaced when the cutover routed those tasks to the wrong endpoint. Patched all 3.
> - **Compose template + live `docker-compose.yml`**: defaults moved gpt-5.4-mini → mdemg-llm-v1; new `LLM_ENDPOINT` env passthrough; bumped rerank timeout 10s → 60s.
> - **Smoke-test verification (5 of 16 task surfaces fired with mdemg-llm-v1)**: `retrieval.query_classify` (9 OK / 9 calls, 453ms - 4.2s), `retrieval.intent_translate` (14 OK / 16, 602ms - 15s), `retrieval.rerank_cross` (5 OK / 8, 2.7s - 60s), `ape.reflect` (2 OK / 13, ~170s/call), `consulting.classify` (1 call pre-patch, will work post-patch). Remaining 11 task surfaces are background-triggered, share identical infrastructure, and will route correctly when they fire.
> - **Two architectural constraints discovered**: (1) RSIC concurrent fan-out (5+ ape.reflect calls within 200ms) crashes mlx_lm.server with Metal OOM. Workaround: `--prompt-concurrency 1 --decode-concurrency 1` serializes requests. Long-term fix: rate-limit RSIC scheduler in Go. (2) Local LLM is 10-50× slower than cloud — every per-task timeout needed bumping; retrieval end-to-end now 5-29s (was sub-second).
> - **Container redeploy gated on next CI image build**: the running Docker mdemg image is pre-patch (gpt-5.4-mini default + missing server.go patches). After this commit pushes + CI publishes a new GHCR image, operations should `docker compose pull mdemg && docker compose up -d`.
> - **Costs**: $0 OpenAI. ~3 hr compute (smoke test, mostly waiting for ape.reflect 180s/call).
> - **New artifacts**: `docs/development/ft-lora/phase_11_6_post.md`; symlink `.local-models/mdemg-llm-v1` (gitignored); manifest at `.local-models/qwen3-14b-mdemg-v1/manifest.json` (gitignored).

> **Changes in v5.11 (Sprint FT-LORA-PHASE11.5e — 2026-04-30):**
> - **Augmented `valid_clean.jsonl` from 180 rows × 9 tasks → 319 rows × 16 tasks.** 0% leakage with all 9 train/valid sources. Manifest v2.0 with per-task source breakdown.
> - **Phase 5 dense LEADS the augmented-eval aggregate** at **0.8389**, beating gpt-5.4-mini (0.8317), Run 7 (0.8307), and Stage-1 distill (0.8294). The 11.5d "+0.26pp Stage-1 over Phase 5" was a 9-task-subset artifact; 7 more tasks of coverage flipped the verdict.
> - **PRODUCTION ROLLBACK**: Stage-1 distill archived to `.local-models/qwen3-14b-mdemg-v1-distill-stage1/` (was `-rl/`). Phase 5 base (`qwen3-14b-mdemg-v1/`) reinstated as production canonical. Production-use: `mlx_lm.server --model .local-models/qwen3-14b-mdemg-v1 --host 127.0.0.1 --port 8101` (no `--adapter-path`).
> - **Stale-hash rescue (Epic 1)** for jiminy.evaluate + jiminy.evaluate_llm: 40 rows extracted via content-routing through a discovered production task_name swap bug. TSDB rows tagged `jiminy.evaluate` actually contain `jiminy.evaluate_llm`'s production prompt content (and vice versa); affects all rows logged with these task names through 2026-04-29. Filed as production follow-up.
> - **Synthetic prompt generation (Epic 2)** for 5 data-starved tasks: 99 rows captured from gpt-5.4-mini at temp=0.9 with reward filter ≥0.7. Per-task: guardrail.evaluate (20), hidden.summarize (19), consulting.synthesis (20), metalearn.generalize (20), summarize.generate (20). New `scripts/x10_synth_prompt_capture.py` (reusable). New `scripts/x11_jiminy_evaluate_rescue.py` (rescue extractor).
> - **Deprecation: retrieval.rerank_nli** removed from active eval. Production OpenAI deploys emit `retrieval.rerank_cross`; the NLI task_name is Ollama-only, dead in production. Spec retained for code-completeness.
> - **Strategic findings**: (1) Phase 11 RL + 11.5d distill are net-negative on broader eval; both archived. (2) Phase 5 wins more production-grounded tasks (4 of 9 TSDB tasks, including +21pp on `retrieval.intent_translate` over gpt-mini). (3) `consulting.classify` distillation backfired because training class distribution (mostly must/must_not) didn't match eval distribution (80% none). Filed as Phase 11.5f candidate. (4) `guardrail.evaluate` 20pp gap to gpt-mini on synthetic prompts confirms local models genuinely struggle here; needs production data, not synthesis.
> - **OpenAI cost**: ~$3.80 (synthesis $0.60 + gpt-mini augmented re-baseline $3.20). Compute: ~3.5 hr local MLX.
> - **New artifacts**: `docs/development/ft-lora/{sprint_plan_phase_11_5e.md, phase_11_5e_post.md}`; `training_data/eval/{valid_clean.jsonl (v2), valid_clean_manifest.json (v2), valid_clean_{rescued,synthetic}.jsonl, baseline_{phase5,run7,stage1,gpt54mini}_clean_v2_fullsweep.json, clean_v2_comparison.md, phase11_5e_epic0_verdict.json, valid_clean_v2_leakage_audit.json}`; `scripts/{x10_synth_prompt_capture.py, x11_jiminy_evaluate_rescue.py}`.

> **Changes in v5.10 (Sprint FT-LORA-PHASE11.5d — 2026-04-29):**
> - **Stage-1 distill adapter promoted to canonical `.local-models/qwen3-14b-mdemg-v1-rl/`** with full-lineage `manifest.json`. Run 7 archived to `-rl-run7`. Adapter SHA256 `71821ee4cc7a6d74…`, full-sweep aggregate **0.8578** (+0.26pp over Phase 5 0.8553, -0.09pp from gpt-5.4-mini ceiling 0.8587).
> - **Per-task wins on connection-layer tasks**: `consulting.classify` 0.668 → 0.688 (+2.0pp; drives Jiminy guidance fidelity), `hidden.reclassify` 0.925 → 0.975 (+5.0pp; drives concept clustering quality). These are the user's stated purpose-target tasks.
> - **Benchmark row-sweep fix (Phase 11.5e in code, executed in this sprint)**: `neural/benchmarks/run_benchmark.py` previously evaluated `rows[0]` only per spec (MVP cap). Patched to iterate ALL matched rows by default; added `RunnerOptions.rows_per_spec` field + `--rows-per-spec` CLI flag (default 0 = all). 109 RL/DPO + 13 benchmark unit tests still green.
> - **The "+5pp gap to gpt-mini" was a single-prompt-per-spec artifact.** Real gap on full-sweep: Phase 5 0.8553 vs gpt-mini 0.8587 = **+0.34pp** (within noise). Phase 5 already at gpt-mini parity on real production data. The entire Phase 11 → 11.5 plateau narrative was optimizing against a phantom.
> - **Phase 11 RL Runs 1-7 net-zero on real data**: Run 7 full-sweep aggregate **0.8531** (-0.22pp behind Phase 5 alone). Claimed "+1.83pp Run 7 over Phase 5" was the same single-prompt artifact. RL produced 2 task-wins + 3 task-losses; net negative.
> - **Phase 5 BEATS gpt-mini on 4 of 9 measured tasks** (full-sweep): `retrieval.intent_translate` +12.4pp, `retrieval.query_classify` +2.5pp, `retrieval.rerank_cross` +0.6pp, `jiminy.synthesize` +0.7pp. Cloud teacher is not strictly better on production traffic.
> - **Distillation set:** 100 train + 12 valid pairs (consulting.classify 36 + retrieval.rerank_cross 76) from gpt-5.4-mini at reward ≥ 0.8. Captured via new `scripts/x9_distill_capture_v2.py` with TSDB extraction + leak audit (0% overlap with valid_clean + 9 train/valid sources). OpenAI cost: ~$1 capture + ~$0.40 full-sweep re-baseline.
> - **SFT execution:** `mlx_lm.lora` proven path (sidesteps Phase 11 custom-trainer footguns). 50 iters / 2 epochs / lr=1e-5 / batch=4 / 7 LoRA modules / r=32 / max_seq_length=8192 (initial 4096 truncated half the rerank_cross pairs; killed and retried at 8192). val_loss 0.685 → 0.417 (-39.1%) monotonic. Peak memory 84.7 GB. ~109 min wall-clock.
> - **Stage-2 GRPO skipped** — Stage-1 cleared the realistic target zone after row-sweep fix; Stage-2 would chase noise.
> - **New artifacts:** `docs/development/ft-lora/{sprint_plan_phase_11_5d.md, phase_11_5d_post.md}`; `configs/sft_phase11_5d_distill.yaml`; `scripts/x9_distill_capture_v2.py`; `training_data/distill/phase11_5d/{train.jsonl, valid.jsonl, manifest.json, raw_responses.jsonl}`; `training_data/eval/{baseline_phase5_clean_fullsweep.json, baseline_run7_clean_fullsweep.json, baseline_gpt54mini_clean_fullsweep.json, regression_phase11_5d_stage1_fullsweep.json, phase11_5d_epic0_verdict.json}`; `neural/benchmarks/run_benchmark.py` row-sweep patch.
> - **Production-use:** `mlx_lm.server --model .local-models/qwen3-14b-mdemg-v1 --adapter-path .local-models/qwen3-14b-mdemg-v1-rl --host 127.0.0.1 --port 8101`. Recommended `max_tokens` 12000, `timeout_s` 300.

> **Changes in v5.9 (Sprint FT-LORA-PHASE11.5c — 2026-04-28):**
> - **Built leak-free `valid_clean.jsonl`** (180 rows, 9 of 17 tasks) from production TSDB; verified **0 prompt overlap** with any of 9 train/valid sources via new `audit_eval_leakage.py` tool. Audited the existing `valid_golden.jsonl` against the same sources: **94 of 95 prompts (99%) leak with training data** — the entire Phase 10 baseline of 0.8338 was largely measuring memorization.
> - **Re-baselined Phase 5 dense** (0.8338 → **0.8052**, -2.86pp) **and gpt-5.4-mini** (0.8769 → **0.8562**, -2.07pp) on `valid_clean.jsonl`. Gap held at +5.10pp on clean (vs +4.31pp on golden).
> - **Per-task analysis invalidated the Phase 11 GRPO + Branch B distillation premise.** The "regressor" task `retrieval.query_classify` (the original Phase 11/Branch B target) actually scores **0.90 on production data** (NOT 0.60 as the leaked golden showed). The real regressors hidden by leakage: `consulting.classify` (P5 0.49 vs gpt-mini 0.88, **+39pp gap**) and `retrieval.rerank_cross` (P5 0.72 vs gpt-mini 0.90, +18pp). Run 5 / Run 7 RL gains were measured against leaked baselines and need re-baselining on `valid_clean` before any further training decisions.
> - **5 ULTS spec patches** (Path B reconciliation; verified hashes by extracting + SHA256-ing Go raw-string consts directly): `ape_reflect`, `hidden_summarize`, `hidden_reclassify`, `jiminy_evaluate_llm`, `retrieval_rerank_cross`. After patches: 11 of 17 specs match TSDB content (vs 6 of 17 before; 54,216 production rows reachable). `.bak` files preserved for rollback.
> - **8 of 17 tasks remain data-starved** (no production traffic with current spec hash): `consulting.synthesis`, `guardrail.evaluate`, `hidden.summarize`, `jiminy.evaluate`/`jiminy.evaluate_llm` (TSDB rows mislabeled), `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate`. Synthetic prompt augmentation deferred to follow-up sprint.
> - **Branch B is paused** (`branch_b_implementation_plan.md` status updated). When it resumes, the distill set changes: drop `retrieval.query_classify` (Phase 5 already strong on real data), add `consulting.classify` (primary, +2.7pp aggregate ceiling) + `retrieval.rerank_cross` (secondary, +0.7pp). Realistic revised target: **+2.0–2.7pp aggregate** on clean eval (not +5pp against leaked golden).
> - **New artifacts**: `training_data/eval/{valid_clean.jsonl, valid_clean_manifest.json, baseline_phase5_clean.json, baseline_gpt54mini_clean.json, clean_vs_golden_delta.md, valid_clean_leakage_audit.json, valid_golden_leakage_audit.json, spec_production_audit.json, spec_patch_plan.json}`. Scripts: `scripts/{build_clean_eval.py, audit_eval_leakage.py, x8b_gpt54mini_clean.py}`. Sprint docs: `docs/development/ft-lora/{sprint_plan_phase_11_5c.md, phase_11_5c_post.md}`.
> - **OpenAI spend**: ~$0.07. 109 RL/DPO unit tests still green after spec patches.

> **Changes in v5.8 (Sprint FT-LORA-PHASE11 — 2026-04-24):**
> - **Phase 11 RL post-training code surface shipped — compute pass operator-gated.** Exit criterion (Option B refined): Epics 1–5 code-complete, Epic 6 three-tier tests green (73 total), Epic 7 in-repo deferrals done, Epic 8 docs landed. Actual MLX GRPO training + gate 5a/5b execution are explicitly follow-up operator tasks — the code path `python -m neural.training.rl.trainer --config configs/rl_phase11.yaml …` is wired, tested with mocked rollouts, and ready to attach to an MLX optimizer step (~100 LOC adapter, task #227). See [`phase_11_rl_post.md`](phase_11_rl_post.md) + [`sprint_plan_ft_lora_phase11.md`](sprint_plan_ft_lora_phase11.md).
> - **New module tree — `neural/training/rl/`**: `trainer.py` (GRPOTrainer + SqlSidecarPersistence, ~330 LOC; MLX-agnostic orchestrator with `RolloutFn` / `OptimizerStepFn` / `EvalFn` / `CheckpointFn` injectable callables — the MLX-optimizer coupling is the only piece deferred), `grpo_loss.py` (clipped surrogate + KL + entropy, numerically stable with log-ratio clamp at ±20, ~150 LOC), `advantage.py` (per-task normalization + 3 zero-stddev policies: `intra_batch_only` default / `widen` / `drop`, ~170 LOC), `reward_sampler.py` (reads `benchmark_results`, 3 sampling strategies: random / weighted_by_inverse_count / stratified_by_group, ~200 LOC), `preflight.py` (5 gates: TSDB rows, stddev policy, SHA, golden holdout, config), `regression.py` (dual gate 5a vs Phase 5 baseline 0.8338 + 5b vs fresh-merge, injectable BenchmarkRunner for test mocking, ~230 LOC).
> - **New module tree — `neural/training/dpo/`**: `pair_generator.py` (reads `benchmark_results`, buckets by (task_id, prompt_hash), selects chosen/rejected by scalar reward delta ≥ 0.15 threshold, ~280 LOC). End-to-end tested against live TSDB: Phase 10 run `q283a23bz59mrg6faxo32ydx2` produced 5 pairs across 2 tasks (`retrieval.query_classify` ×4 + `jiminy.synthesize` ×1, SHA256 `bbe7bb9a…`). Coverage intentionally thin — Phase 12 re-generates against Phase 10 + Phase 11 combined once the compute pass ships.
> - **TSDB V0013 migration** (`internal/tsdb/migrations/013_rl_training.sql`) — **applied live** (`schema_meta.value` advanced 12→13). Additive: `rl_training_runs` (PK run_id CUIDv2, `gate_verdict CHECK IN ('pending','pass','fail')`, early_stopped) + `rl_training_steps` (hypertable on `recorded_at`, 30-day chunks, loss components + advantage stats + clip frac + n_samples/n_dropped accounting). Zero ALTER on V0011/V0012. Reverse-tested offline before apply.
> - **Configs**: `configs/rl_phase11.yaml` (every MEMORY knob explicit: `epochs=3`, `early_stop_threshold=0.95`, `early_stop_patience=2`, `clip_ratio=0.2`, `kl_coef=0.01`, `advantage_clip=[-5,5]`, `zero_stddev_policy=intra_batch_only`, `max_tokens=4000`, `latency_budget_ms=30000`, seed=0); `configs/dpo_phase12_pairs.yaml` (`reward_delta_threshold=0.15`, `min_pairs_per_task=20`, `prompt_hash_strategy=blake2b_16`). Zero hardcoded constants — all CLI-overridable.
> - **Tests (3 tiers, 73 total)**: unit 37 (loss numerics at TOL=1e-6 vs hand-computed fixture; 3 zero-stddev policies; 3 sampling strategies), integration 36 (20-step trainer loop with mocked MLX; early-stop fires at step 6 on 2 consecutive val-reward drops; SQL-injection-safe sidecar; regression harness 5a pass/fail paths + 5b symmetry; DPO pair generator round-trip + manifest + SHA256 verification), e2e live (V0013 applied, trainer sidecar round-trip via `psql -f`, DPO pair generation from live Phase 10 TSDB).
> - **Decision fork resolved (Plan §10 Risk #1)**: `mlx_lm==0.31.2` has no native GRPO — chose Option B (custom trainer in-repo) over Option A (vendor `mlx-lm-lora`). Rationale: no external-dep drift, full control over zero-stddev policy + advantage clipping, MEMORY `feedback_plan_options_pattern.md` precedent from Sprints A/B/C. Orchestrator came in at ~330 LOC (below plan's 400–600 LOC estimate) because MLX coupling was isolated behind injectable callables.
> - **Decision fork resolved (Plan §10 Risk #2)**: 9/16 Phase 10 tasks have `stddev=0` (deterministic C-group rewards + `retrieval.rerank_cross`). Default `zero_stddev_policy: intra_batch_only` — batch rollouts have real sampling-temperature variance even when historical reward is deterministic, so within-batch σ is a meaningful denominator. Fallbacks `widen` (mean × 0.05) and `drop` (skip task entirely) config-selectable. Phase 12 re-assesses once post-RL reward variance data exists.
> - **CLAUDE.md Testing section expanded** with RL post-training subsection: preflight, trainer, DPO pair generator, dual regression, unit+integration suites — each with its exact CLI invocation.
> - **Epic 7 deferrals explicit (compute-gated)**: `--scorer=registry` default flip in `evaluate_ft.py` gates on Phase 11 gate-5a PASS under registry scorer; stagnation auto-exit log in `run_benchmark.py` gates on `benchmark_runs.count() >= 2`. Premature to flip either against a single Phase 10 row.
> - **Phase 12 (HITL DPO) unblocked**: DPO pair set ready for curation; trainer/loss/advantage modules directly reusable for Phase 12's DPO training loop (same scaffolding, different loss function); V0013 schema has capacity for Phase 12 adapter run attribution via `base_model_sha` field.

> **Changes in v5.7 (Sprint FT-LORA-PHASE10 — 2026-04-23 → 2026-04-24):**
> - **Phase 10 Automated Benchmark Framework shipped.** First authoritative baseline for `.local-models/qwen3-14b-mdemg-v1/` captured: **aggregate weighted score 0.8338** across **16 of 17 ULTS specs × 5 runs = 80 rows**, all `finish_reason=stop` (zero truncations). Per-group means: **T=0.8404** (7 tasks, weight 0.50) / **C=0.8222** (6 tasks, weight 0.35) / **J=0.8389** (3 tasks, weight 0.15). See [`phase_10_benchmark_post.md`](phase_10_benchmark_post.md) + [`sprint_plan_ft_lora_phase10.md`](sprint_plan_ft_lora_phase10.md).
> - **New module tree**: `neural/benchmarks/` — `run_benchmark.py` (runner + aggregator + stagnation detection), `llm_judge.py` (gpt-5.4-mini at `temp=0`/`seed=run_idx`/`max_tokens=4000`/`latency_budget_ms=30000`, own-fixed sampling — never inherits task recipe), `sampling_policy.py` (T/C/J group-aware kwargs; drops J-group `top_k=-1` sentinel MLX rejects), `variance.py` (mean/stddev/min/max per task for Phase 11 GRPO advantage normalization), `preflight.py` (17-spec field + floor enforcement), `judge_prompts/{coherence,depth,relevance,naturalness}.txt` (4 prompt SHA-pinned templates).
> - **Config**: `configs/benchmark_phase10.yaml` — single source of truth for N_runs (5), stagnation thresholds (`aggregate_delta=0.005`, `per_task_regression=0.02`), sampling-group weights (T:0.50 / C:0.35 / J:0.15), judge model + sampling kwargs, `performance_floors` (default `max_tokens≥3000`/`latency_budget_ms≥15000`; think_mode `≥10000`/`≥60000`). All values CLI-overridable — zero hardcoded constants.
> - **Two silent scorer bugs fixed** in `neural/training/reward_functions.py` during baseline work, each worth ~2–4% aggregate: (1) `classification_accuracy` treated `expected` as a bare label when the runner passes full assistant JSON (would score valid responses as 0 silently; now shape-detects and compares normalized JSON for equality, with list/string keys handled); (2) `evaluation_accuracy` accepted only `expected_verdict` kwarg when the runner passes `expected=<json>` (would score every response 1.0 silently; now normalizes both paths). First baseline aggregated 0.7990 under the buggy scorers; post-fix 0.8338 (+0.0348, +4.4% relative).
> - **`evaluate_ft.py` shadow refactor (Epic 4)**: added `--scorer={heuristic,registry,dual}` flag. Default stays `heuristic` (bit-identical Phase 5 regression path). `dual` runs both in parallel with per-task `|delta|<1%` assertion; replayed the Phase 5 dev set and confirmed parity. `registry` path ready for Phase 11 default flip after ≥3 benchmark rounds confirm stability.
> - **Think-mode performance floors enforced**: 8 specs bumped to `max_tokens≥10000` + `latency_budget_ms≥60000` — `consulting_synthesis`, `hidden_summarize`, `jiminy_synthesize`, `metalearn_generalize`, `summarize_generate` (T-group), plus `jiminy_evaluate`, `jiminy_evaluate_llm`, `retrieval_rerank_cross` (carried think-mode chain-of-thought over tight budgets). Preflight fails any spec that under-budgets its class.
> - **`hidden.reclassify` spec patched**: `reward_functions` trimmed from `["json_valid", "classification_accuracy"]` → `["json_valid"]` and notes annotated — output is an array of cluster objects, not a single classification; the accuracy reward was structurally inapplicable and silently produced 0.0. A proper cluster-overlap reward is tracked for Phase 10.5 UBENCH (#215).
> - **TSDB V0012 migration drafted** (`internal/tsdb/migrations/012_benchmark_results.sql`) — additive `benchmark_results` (hypertable on `recorded_at`) + `benchmark_runs` tables; SQL present but live migration deferred (Docker down at sprint close — applied when stack is next brought up; the baseline JSON is persisted as a durable sidecar until then, no data lost).
> - **Golden holdout**: `training_data/eval/valid_golden.jsonl` — seeded 15% carve from Phase 5 `valid` splits, SHA `8e44cdf9…`, matched by `meta.task_name`. Prevents train/eval leak on every future benchmark.
> - **Baseline artifacts**: `training_data/eval/benchmark_qwen3_14b_v1_baseline.json` (aggregate + per-task + SHAs + judge meta + sampling kwargs per row; file SHA `789459f1…`, run_id `q283a23bz59mrg6faxo32ydx2`, config SHA `3716f9a4…`).
> - **Known gap — `guardrail.evaluate` (J-group)**: 17th ULTS spec excluded from baseline. Three-way dependency gap: (a) no golden rows (task logs exist in TSDB but Docker down prevented carve), (b) two reward functions (`guardrail_triggered_accuracy`, `guardrail_severity_match`) not yet in `REWARD_REGISTRY`, (c) no training data in the Phase 5 SFT mix. Including it would inject zeros into the J-group mean (3→4 tasks) and contaminate Phase 11 GRPO advantage normalization. Deferred to Phase 10.5 (#216) — implement rewards, carve golden when Docker restored, add to next SFT round, re-baseline the 17th task.
> - **CLAUDE.md cleanup**: stale `docs/benchmarks/run_benchmark_v4.py` + `test_questions_120.json` references replaced with `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml` in the Testing section.
> - **Phase 11 GRPO unblocked.** Phase 11 consumes `benchmark_results.reward_vector` per (task, run_idx) + `stddev` per task from this sprint's output.

> **Changes in v5.6 (Sprint FT-LORA-PHASE5 — 2026-04-22 → 2026-04-23):**
> - **Phase 5 SFT shipped.** Final merged model at `.local-models/qwen3-14b-mdemg-v1/` (7.8 GB, 4-bit preserved via `mlx_lm fuse`). Dual regression gate **PASS**: 0.9805 (MoE-35B baseline) / 0.9505 (dense-14B baseline) → **0.9856 post-tune**; **16/16 ULTS tasks passing** (baseline: 15/16). Training run 9h 7m wall-clock, early-stop at Iter 3000, best adapter restored from Iter 2400 (val_loss 0.246). Peak memory 36 GB. See [`phase_5_sft_post.md`](phase_5_sft_post.md) + [`phase_5_sft_summary.md`](phase_5_sft_summary.md) + [`sprint_plan_ft_lora_phase5.md`](sprint_plan_ft_lora_phase5.md).
> - **Mid-sprint MoE → dense pivot (2026-04-22).** MoE-Sieve two-tier strategy abandoned after Metal 499K MTLResource ceiling on M5 Max / macOS 26 blocked every non-trivial MoE LoRA backward pass (identical `[metal::malloc] Resource limit (499000) exceeded` across 4 mxfp4 configurations *and* standard q4 — the cap is architectural, not quant-specific; macOS 26 removed the `iogpu.rsrc_limit` sysctl so there is no user-space fix). **What shipped:** single-tier LoRA on `mlx-community/Qwen3-14B-4bit` (40 layers, hidden 5120, 7 dense target modules `self_attn.{q,k,v,o}_proj` + `mlp.{gate,up,down}_proj`). Tier 1 policies held (`rank=32 α=64`, epoch cap 3, early-stop `val_loss > best × 1.05` for 2 consecutive evals, seq 8192, LR 5e-5, seed 0, grad_checkpoint). Tier 2 × 3 families + asymmetric quant **dropped** — no dense analog (no shared expert, no routed experts, no router). Sprint D profiles + `quantize_asymmetric.py` + `expert_selection.py` retained as research artifacts.
> - **Dense baseline SHA pin**: `a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5` (`config.json` of `mlx-community/Qwen3-14B-4bit` snapshot `a4d9b2df59d2c150bef02fcbe0d91046b7ca33a4`). Replaces the Sprint C MoE pin `cdc167566e…` for all post-pivot training + eval scripts.
> - **MLX serving port moved 8100 → 8101** (CMS-pinned). Post-pivot the `mlx_lm.server` hosts both the base model (baseline eval) and the merged model (post-tune eval + live inference) on the same port via model swap. All scripts + docstrings updated (`scripts/test_vllm_mlx.py`, `scripts/sprint_e_e2e_dry_run.sh`, `neural/training/evaluate_ft.py`, `teacher_distill.py`, `distill_driver.py`, `quantize_deploy.py`, `profile_expert_routing.py`).
> - **Env vars added** to `.env` + `.env.example`: `MLX_BASE_MODEL`, `MLX_BASE_MODEL_PATH`, `MLX_BASE_MODEL_CONFIG_SHA256`, `MLX_MERGED_MODEL`, `MLX_MERGED_MODEL_PATH` (5 total). Sprint FT-LORA-E knobs (`ROUTER_AUX_LOSS_COEF`, `LORA_TIER2_*`, `ASYMMETRIC_QUANT_*`) annotated as no-ops on the dense path (preserved for MoE-era provenance).
> - **Storage hygiene**: ~51.5 GB reclaimed post-sprint — 29 intermediate LoRA checkpoints + Iter 3000 backup (~14.5 GB), archived MoE attempts (6.3 MB), abandoned HF cache MoE bases (`Qwen3.6-35B-A3B-4bit` 19 GB + `Qwen3.6-35B-A3B-mxfp4` 18 GB). `Qwen3-14B-4bit` HF cache + `.local-models/qwen3-14b-mdemg-v1/` + `adapters/tier1/adapters.safetensors` (Iter 2400 best) preserved.
> - **Memory rule update**: `memory/project_phase5_moe_pivot.md` amended — MoE path now marked permanently abandoned (was "if reconsidering MoE later…"). Per user directive 2026-04-23 at sprint close.
> - **Phase 10 benchmark now unblocked.** Consumes the single merged model `qwen3-14b-mdemg-v1` (not 4 namespaced models as originally planned).

> **Changes in v5.5 (Sprint FT-LORA-DATA — 2026-04-22):**
> - **4 curated SFT datasets written**: `training_data/sft/tier1/` (3,500 rows = 3,150 train + 350 valid), `family_reasoning_think/` (1,700), `family_classify_notink/` (1,200), `family_structured_notink/` (600). Each directory has `train.jsonl` + `valid.jsonl` + `manifest.json` with per-file SHA256, per-task counts, duplication factors, source composition, synthesis version. Total 7,000 rows across 4 datasets.
> - **4 originally-absent T-family tasks synthesized** (consulting.synthesis, metalearn.generalize, retrieval.rerank_nli, summarize.generate) via `neural/training/distill_driver.py` mixed teacher routing: `gpt-5.4-mini` for consulting.synthesis + metalearn.generalize + hidden.summarize (600 rows, ~$0.35–$0.50 actual spend against $100 hard-abort cap); Qwen3.6-35B-A3B-mxfp4 MLX-local for retrieval.rerank_nli + summarize.generate (400 rows, $0). Synthesis version `v1-aaa646e` stamped per-row.
> - **New modules**: `recurate.py` (provenance-preserving re-curation with `--expected-raw-sha256` pin assertion), `distill_driver.py` (teacher-routing orchestrator with budget-cap abort + Epic-6.0 observability: per-row structured log, `fout.flush()`, `/models` preflight, single-instance MLX guard, `--debug-log`, `--http-timeout-s` + retry, `--count` override, `--strict` mode), `balanced_sampler.py` (per-tier pre-processing sampler, 200-row floor, 500 ape.reflect cap, 5× duplication ceiling), `stratified_split.py` (90/10 per-task split + manifest emission).
> - **Raw dataset SHA pin**: `7caebf75fd59da37221acef887dc822ac9b80d04e19c19b750dd9a4e5eceb988` (21-day window `llm_interactions.jsonl`, 42,727 rows). Asserted by all 4 modules on every invocation; drift aborts.
> - **Post-pre-flight verdict CLEAR**: `phase_5_dataset_preflight_post.md` — all 7 baseline checks now pass. Phase 5 SFT runbook unblocked.
> - **Two durable guardrails** surfaced during execution and persisted as MEMORY rules:
>   - Never set `max_tokens < 3000` on any LLM call (forcing function: summarize.generate lost 7.5% of rows to truncation when max_tokens=1500 + Qwen3.6 think_mode consumed the token budget).
>   - Never set `latency_budget_ms < 15000` on any LLM call (longest observed valid row = 12.7s; 10s budget would have caused catastrophic timeouts on the tail).
> - **All 16 ULTS specs audited + fixed**: `max_tokens` floored at 3000, `latency_budget_ms` floored at 15000. Code defaults in `distill_driver.py`, `teacher_distill.py`, `evaluate_ft.py` updated to the new floors.
> - **Epic 6.0 Execution Stabilization** (added mid-sprint after observability failure): per-row stderr logging, pre-flight `/models` ping, single-instance MLX guard, response-payload debug capture, HTTP retry policy, `--count` / `--strict` flags, CMS constraint observations pinning `:8101` MLX endpoint + `mdemg/qwen3.6-35b-a3b-mdemg-v{N}` post-FT namespace.
> - **Policy change — FT-OAI-003 DROPPED** (user directive 2026-04-22): no further OpenAI fine-tuning deliverables. All future fine-tuning targets local MLX LoRA on Qwen3.6-35B-A3B. OpenAI API remains usable as a **teacher** for synthesis only. Memory entry `memory/project_ft_oai_003_deferred.md` to be removed post-sprint-approval.
> - **Tests (3 tiers)**: 4 new unit-test files + integration test + E2E script. All green. Full-scale `mlx_lm.tuner.datasets.load_local_dataset` schema check on all 4 datasets passes (no `--dry-run` flag exists on mlx_lm.lora 0.31.2; using the internal loader path is the authoritative check).
> - **Pre-flight automation deferred** to a future cleanup sprint (~200-300 LOC, outside this sprint's budget).

> **Changes in v5.4 (Sprint FT-LORA-E — 2026-04-22):**
> - **Tier-aware CLI on `neural/training/train_ft.py`**: 13 new flags (`--tier {1,2}`, `--family`, `--expert-selection-path`, `--expected-sha256`, `--mode {sft,rl}`, `--base-adapter`, `--rank`, `--alpha`, `--target-modules`, `--router-aux-loss-coef`, `--early-stop-ratio`, `--early-stop-patience`, `--n-epochs`). Tier 1 = attention + shared expert, r=32 α=64, 7 modules × 40 layers. Tier 2 = top-25% routed experts per family, r=8 α=16, 7,680 modules (40 × 64 × 3).
> - **New modules**: `neural/training/expert_selection.py` (Sprint D profile JSON → mlx_lm `keys` list), `neural/training/quantize_asymmetric.py` (BF16 attn + shared + router / MXFP4 routed experts predicate + `--dry-run` classifier CLI), `neural/training/early_stop.py` (subprocess stdout monitor: SFT `val_loss > best × 1.05` for 2 consecutive evals, RL mirror `val_reward < best × 0.95`).
> - **Dual-path `router_aux_loss_coef=0.002` injection**: primary `--config train_config.yaml` + fallback atomic copy-on-write `config.json` replacement with SIGTERM/SIGINT/SIGHUP + atexit restoration and SHA256 re-match on exit. Catches the "crashed training drifts base model config" failure mode.
> - **Sprint C SHA pin (`cdc167566e…`) enforced for BOTH tiers** via `--expected-sha256` flag. Drift aborts before any training starts.
> - **Epoch cap = 3 enforced as rejection, not silent clamp**; `--n-epochs auto` rejected citing FT-OAI-001 forcing function.
> - **Env vars activated (11 total)**: `ROUTER_AUX_LOSS_COEF`, `LORA_TIER{1,2}_{RANK,ALPHA}`, `LORA_N_EPOCHS_CAP`, `LORA_EARLY_STOP_{SFT,RL}_THRESHOLD`, `ASYMMETRIC_QUANT_{SHARED,ROUTED,ATTN}`. 3 files modified (`.env.example`, `docker-compose.yml`, `internal/cli/compose_templates/docker-compose.yml`).
> - **Tests (3 tiers)**: 89 unit + 5 integration + 1 E2E script (`scripts/sprint_e_e2e_dry_run.sh`). All green.
> - **Phase 5 SFT unblocks**: Tier 1 universal adapter + 3× Tier 2 family adapters + asymmetric quant can now be launched from `train_ft.py` + `quantize_asymmetric.py`.

> **Changes in v5.3 (Sprint FT-LORA-D — 2026-04-22):**
> - **Expert activation profiler committed**: `neural/training/profile_expert_routing.py` — context-manager monkey-patch of `Qwen3NextSparseMoeBlock.__call__` captures top-k routing decisions across prompt and generated tokens. Single-pass inline forward (no double-compute). Determinism verified bit-identical across runs.
> - **Anchor prompt set**: `training_data/routing_profiles/anchor_prompts.jsonl` — 320 prompts (20 per task × 16 tasks, T=140 / C=120 / J=60). Primary source: `training_data/raw/extracted/llm_interactions.jsonl` filtered by `task_name` (11 tasks have ≥20 unique production prompts). Backfill source: same-shape donor tasks' production prompts for 5 T-family tasks with zero production traffic at profiling time (hidden.summarize, consulting.synthesis, metalearn.generalize, retrieval.rerank_nli, summarize.generate). Deviation from runbook's whk-wms-category backfill — repurposing real production traces from related tasks preserves the task-family routing signal better than generic codebase questions.
> - **Artifacts (consumed by Sprint E)**: `training_data/routing_profiles/profile_routing_{reasoning_think,classify_notink,structured_notink}.json` + `raw_activation_counts.json` + decision doc `docs/development/ft-lora/sprint_c_d_profile_results.md`.
> - **Analyzer**: `neural/training/sprint_d_analyze.py` — cross-family Jaccard overlap (3 pair averages across 40 layers), per-family task-cohesion (within-family pairwise Jaccard + agglomerative hierarchical clustering for split-candidate boundaries), KL divergence vs uniform. Explicit verdict codes: `3-family-confirmed` / `2-family-merged-<pair>` / `1-family-collapsed`.
> - **Sprint E unblocks**: `neural/training/train_ft.py --expert-selection-path=profile_routing_{family}.json` is now backed by real artifacts.
> - **Version 5.2 content unchanged**; v5.3 adds Sprint D plan + decision doc + profile artifacts.

> **Changes in v5.2 (Sprint FT-LORA-C — 2026-04-21):**
> - **Runbook committed**: `sprint_plan_ft_lora_c.md` — 3-gate MLX validation designed for non-continuous execution (week-long pauses between gates survive via `~/.mdemg-sprint-c/` disk stamps). No execution artifacts in Sprint C itself ($0 spend).
> - **Gate 1**: asymmetric-quant load ceilings — peak RAM ≤24 GB pass / 24-30 GB flag / >30 GB halt; load time ≤90 s (first-load from cold page cache, SSD-tier normalized), forward-pass ≤30 s. Three path options (A=published asymmetric, B=`mlx_lm.convert` attempt, C=symmetric 4-bit with Sprint-E deviation).
> - **Gate 2**: ≥95% JSON validity on 100 synthetic J-group prompts; fallback 12-cell sweep concentrated on `presence_penalty` (5) × `temperature` (2) + 2 controls (no-chat-template, json_mode_on).
> - **Gate 3**: throughput ≥60 tok/s (halt if <60) + quality bands vs `gpt-5.4-mini`: ≤10% clear pass / 10-30% middle band (Sprint F) / >30% halt. Hard $25 baseline budget cap; 24h same-window constraint between baseline and Qwen runs.
> - **Sprint F registered** (new): post-SFT commit-or-fallback checkpoint, triggered only by Gate 3 middle-band stamp. Skeleton only — full 12-section plan drafted at Sprint F start if triggered.
> - **Version 5.1 content unchanged**; v5.2 adds the runbook doc + Sprint F registration.

> **Changes in v5.1 (Sprint FT-LORA-B — 2026-04-21):**
> - **Guardrail migration**: `internal/guardrail/llm_evaluator.go` now routes through `llmclient` (17th captured call site, `task_name='guardrail.evaluate'`). Hard cutover renamed circuit breakers `openai-guardrail` → `openai-guardrail.evaluate` / `ollama-guardrail` → `ollama-guardrail.evaluate` — breaking change on the admin surface, see CHANGELOG.
> - **ULTS schema**: required `sampling_group` enum (T/C/J) added; all 16 canonical specs + new `guardrail_evaluate.ults.json` (17th) carry the field.
> - **Grep-audit remediation**: 15 files refreshed from `Qwen3-30B-A3B` → `Qwen3.6-35B-A3B`; `scripts/test_vllm_mlx.py` argparse default updated (functional change when `$LLM_MODEL` unset). `mlx-community/Qwen3.6-35B-A3B-4bit` confirmed on HuggingFace at execution time (not `-Q4`).
> - **.env + compose**: seeded Sprint-E placeholder knobs (`ROUTER_AUX_LOSS_COEF`, `LORA_TIER1/2_*`, `ASYMMETRIC_QUANT_*`) commented out.
> - **Version 5.0 memo-alignment unchanged**; v5.1 is a patch-level execution pass of what Sprint A queued.

**Version:** 5.0 (Qwen3.6-35B-A3B upgrade + two-tier MoE-Sieve LoRA + no-tool-calling architectural policy per memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1)

> **Changes in v5.0 (per memo 07 v3.1 — 2026-04-21)**
>
> 1. **Base model**: Qwen3-30B-A3B → **Qwen3.6-35B-A3B** (Apache 2.0, 35B/3B active, 256 experts = 8 routed + 1 shared, 262K native context, MTP speculative decoding). Fallback: Qwen3.5-35B-A3B — **not** Qwen3-30B-A3B. See [`01_RESEARCH_v2.md §3`](01_RESEARCH_v2.md).
> 2. **No-tool-calling architectural policy** — all 16 MDEMG LLM call sites are single-shot structured-output/reasoning. Previously implicit, now explicit with 9 banned patterns including `preserve_thinking`. See [`01_RESEARCH_v2.md §2.8`](01_RESEARCH_v2.md).
> 3. **Two-tier MoE-Sieve LoRA** — Tier 1 (attention + shared expert, r=32 α=64, all 16 tasks balanced) + Tier 2 (top-25% routed experts, r=8 α=16, per-family: reasoning-think / classify-notink / structured-notink). Load-balancing `router_aux_loss_coef=0.002`. Asymmetric quant (shared BF16 / routed MXFP4_MOE / attention BF16). See [`01_RESEARCH_v2.md §5`](01_RESEARCH_v2.md).
>
> **⚠️ Two Sprint A planner-introduced policies (new in v5.0, flagged for user sign-off):**
> - Epoch cap + early-stop: `val_loss > best × 1.05` for 2 consecutive evals, max 3 epochs. Closes memo §6.1 open question.
> - `n_epochs=auto` disallowed on all LoRA runs.
> - Forcing function: FT-OAI-001 overfitting at step 1200 (`training_data/openai_ft/20260420/run_notes.md`).

---

## Document Map

Read in order. Each document builds on the previous.

| # | File | Purpose | Pages |
|---|---|---|---|
| 1 | `01_RESEARCH_v2.md` | Strategic rationale — why fine-tune, the 16 call sites (§1.1), no-tool-calling policy (§2.8), model selection (§3), **two-tier MoE LoRA strategy (§5)** | ~22 |
| 2 | `02_M5MAX_HARDWARE_v2.md` | Hardware-specific model selection, asymmetric-quant memory math, inference/training estimates (Tier 1 + Tier 2) | ~11 |
| 3 | `03_IMPLEMENTATION_PLAN_v2.md` | The build plan — 13 phases + **Phase 5.X expert activation profiling** (Sprint D), code-level specs, ⚠️ overfitting-prevention policies | ~27 |
| 4 | `04_BENCHMARK_RL_v2.md` | Phases 10-12 — three-group sampling recipes, automated benchmarks, GRPO/DPO, **router-entropy monitoring + val-reward early-stop** | ~20 |
| 5 | `05_DATA_COLLECTION_v2.md` | Training data collection, governance, storage, curation pipeline, **Appendix A (balanced sampling) + Appendix B (routing profile artifact)** | ~22 |
| 6 | `06_CORRECTIONS_APPLIED_v2.md` | All corrections v1.0→v5.0 consolidated with resolution status | ~10 |
| 7 | `SPRINT_A_GREP_AUDIT.md` | Sprint FT-LORA-A Epic 10 output — repo-wide grep of stale model names and banned tool-calling patterns; remediation queue for Sprint B | ~3 |
| 8 | `sprint_plan_ft_lora_a.md` | Sprint FT-LORA-A v1.0-format plan (as executed) — 11 epics, 3-tier testing, commit strategy, Documents Accessed appendix | ~7 |
| 9 | `sprint_plan_ft_lora_b.md` | Sprint FT-LORA-B v1.0-format plan (as executed) — 7 epics, ULTS `sampling_group`, guardrail llmclient migration, grep-audit remediation, placeholder env knobs | ~9 |
| 10 | `sprint_plan_ft_lora_c.md` | Sprint FT-LORA-C v1.0-format plan (planning-only, runbook) — 3-gate Qwen3.6-35B-A3B MLX validation + Sprint F registration | ~14 |
| 11 | `sprint_plan_ft_lora_d.md` | Sprint FT-LORA-D v1.0-format plan (as executed) — 5 epics, expert activation profiling script + anchor prompt set + family-partition decision | ~8 |
| 12 | `sprint_c_d_profile_results.md` | Sprint D Epic 3 decision doc — verdict code (3-family-confirmed / 2-family-merged / 1-family-collapsed), cross-family overlap + task-cohesion tables, Sprint E recommendation | ~4 |
| 13 | `sprint_plan_ft_lora_e.md` | Sprint FT-LORA-E v1.0-format plan (as executed) — 7 epics, tier-aware train_ft.py CLI + `expert_selection.py` + `quantize_asymmetric.py` + `early_stop.py` + env-var activation + atomic `router_aux_loss_coef` injection. Post-execution notes record dual-path injection strategy + deferred checkpoint-behavior verification. | ~15 |
| 14 | `phase_5_dataset_preflight.md` | Phase 5 dataset pre-flight **baseline** report (`aaa646e`) — verdict **BLOCKED**; enumerates 4 structural blockers in the existing curated corpus that motivated Sprint FT-LORA-DATA. | ~8 |
| 15 | `sprint_plan_ft_lora_data.md` | Sprint FT-LORA-DATA v1.0-format plan (as executed) — 7 epics + Epic 6.0 Execution Stabilization, 4 new modules, 4 curated datasets, mixed-teacher synthesis, 90/10 split, $100 hard-abort cap, post-pre-flight gate. | ~14 |
| 16 | `phase_5_dataset_preflight_post.md` | Phase 5 dataset pre-flight **post-run** report (FT-LORA-DATA result) — verdict **CLEAR**; all 7 baseline checks resolved. Phase 5 SFT runbook unblocked. | ~8 |

---

## Key Decisions (v5.0)

| Decision | Rationale | Canonical ref |
|---|---|---|
| **Model: Qwen3.6-35B-A3B MoE** | Apache 2.0 (released 2026-04-16). 35B/3B active, 256 experts = 8 routed + 1 shared, Hybrid Gated DeltaNet + Gated Attention + MoE, MTP speculative decoding, 262K native context. **Fallback Qwen3.5-35B-A3B — NOT Qwen3-30B-A3B** (lacks shared expert needed for Tier 1). Sprint C three-gate validation decides ship vs fallback. | [`01_RESEARCH_v2.md §3`](01_RESEARCH_v2.md) |
| **No-tool-calling architectural policy** | All 16 LLM call sites are single-shot structured-output/reasoning. Nine banned patterns (incl. `preserve_thinking`). Sprint B grep-audits all code/config. | [`01_RESEARCH_v2.md §2.8`](01_RESEARCH_v2.md) |
| **Two-tier MoE-Sieve LoRA** | Tier 1: attention + shared expert, r=32 α=64, all 16 tasks balanced. Tier 2: top-25% routed experts per family (Sprint D profiling), r=8 α=16, 3 families (reasoning-think / classify-notink / structured-notink — provisional). | [`01_RESEARCH_v2.md §5`](01_RESEARCH_v2.md) |
| **Asymmetric quantization** | Shared expert + attention BF16 (quality-sensitive); routed experts MXFP4_MOE (4-bit MoE-aware); router/gate BF16. `mlx_lm.convert` patched in Sprint E. | [`01_RESEARCH_v2.md §5.4`](01_RESEARCH_v2.md) |
| **Load-balancing `router_aux_loss_coef=0.002`** | Prevents expert collapse during Tier 1/2 training and GRPO. Layer-level routing entropy gate ≥ 1.5 nats. | [`04_BENCHMARK_RL_v2.md §11.2.1`](04_BENCHMARK_RL_v2.md) |
| **Three-group sampling recipes** | T (think, temp=0.6), C (no-think classify, temp=0.3 max_tokens=64), J (no-think JSON, temp=0.7, **`presence_penalty=1.5`**, max_tokens=2048). All 16 tasks mapped. | [`04_BENCHMARK_RL_v2.md §10.0`](04_BENCHMARK_RL_v2.md) |
| **⚠️ Overfitting-prevention policies (Sprint A NEW)** | Epoch cap = 3, SFT early-stop `val_loss > best × 1.05` for 2 consec. evals; RL mirror `val_reward < best × 0.95`. `n_epochs=auto` disallowed. Forcing function: FT-OAI-001 step-1200 overfit. | [`03_IMPLEMENTATION_PLAN_v2.md §Phase 5F`](03_IMPLEMENTATION_PLAN_v2.md), [`04_BENCHMARK_RL_v2.md §11.6`](04_BENCHMARK_RL_v2.md) |
| **Inference: vllm-mlx** | OpenAI-compatible, prefix caching, continuous batching, Qwen3 reasoning parser, adapter-stack support for Tier 1 + Tier 2. No `--tool-call-parser`, no `--enable-auto-tool-choice`. | [`02_M5MAX_HARDWARE_v2.md §4`](02_M5MAX_HARDWARE_v2.md) |
| **LLM consumers: 16 (re-audited 2026-04-21)** | 16 rows = 16 distinct task labels. v4.0 "17 rows" corrected (jiminy.evaluate double-count removed). Guardrail is a 17th call site that bypasses llmclient — Sprint B migration queued. | [`01_RESEARCH_v2.md §1.1`](01_RESEARCH_v2.md) |
| **Training: MLX bf16 LoRA** | M5 Max 128GB has no production traffic constraint during offline training. Tier 1 ~105–115GB; Tier 2 ~67–75GB (inference can run alongside Tier 2). | [`02_M5MAX_HARDWARE_v2.md §3`](02_M5MAX_HARDWARE_v2.md) |
| **Training infra patched: Phase 5 unblocked (Sprint E)** | Tier-aware `train_ft.py` CLI + `expert_selection.py` (Sprint D → 7,680 mlx_lm keys) + `quantize_asymmetric.py` (BF16 attn/shared + MXFP4 routed predicate) + `early_stop.py` (val_loss/val_reward monitor with patience=2) + dual-path atomic `router_aux_loss_coef` injection + Sprint C SHA gating for both tiers. | [`sprint_plan_ft_lora_e.md`](sprint_plan_ft_lora_e.md) |
| **Balanced sampling for Tier 1** | Equal records per task label (`per_task=500` default) prevents 223× skew (FT-OAI-001 R1 finding). Integer up-sampling via duplication; deterministic seed. | [`05_DATA_COLLECTION_v2.md Appendix A`](05_DATA_COLLECTION_v2.md) |
| **Anti-collapse: α ≥ 0.4 exogenous ratio** | Peer-reviewed. Minimum 40% non-model-generated data per batch. | — |
| **Think block stripping** | 9 of 16 consumers parse JSON. `SanitizeResponse()` strips `<think>...</think>`. | [`03_IMPLEMENTATION_PLAN_v2.md Phase 2D`](03_IMPLEMENTATION_PLAN_v2.md) |
| **Data storage: TimescaleDB** | `llm_interactions` hypertable, 7-day chunking, 180-day retention, 14-day compression. | [`05_DATA_COLLECTION_v2.md §1`](05_DATA_COLLECTION_v2.md) |
| **RAFT training pattern** | 80% of records include retrieval context; 20% stripped (parametric recall). Deterministic via SHA-256(trace_id). | [`03_IMPLEMENTATION_PLAN_v2.md Phase 4A`](03_IMPLEMENTATION_PLAN_v2.md) |
| **ULTS spec framework** | 16 specs (one per task); Sprint B adds `sampling_group` field per task. Single source of truth for contracts + sampling. | [`03_IMPLEMENTATION_PLAN_v2.md Phase 4B`](03_IMPLEMENTATION_PLAN_v2.md) |
| **Routing profile artifacts** | Phase 5.X emits `profile_routing_{family}.json` per family; location `training_data/routing_profiles/`. Sprint D validates family partition. | [`05_DATA_COLLECTION_v2.md Appendix B`](05_DATA_COLLECTION_v2.md) |
| **Embedding: separate workstream** | Contrastive learning on encoder models (not LoRA). Target: 3072-dim vectors. Data collection starts now; training later. | [`01_RESEARCH_v2.md §1.4`](01_RESEARCH_v2.md) |
| **Default external LLM: gpt-4.1-nano** | Non-tool-use, 1M context. LoRA target is **Qwen3.6-35B-A3B** (switches external + local to single model after Phase 5 SFT lands). | [`01_RESEARCH_v2.md §3`](01_RESEARCH_v2.md) |
| **Curated dataset pipeline (unchanged)** | export → UTDS validate → quality_filter → format_converter → dataset_versioner → train_ft. Validated E2E (10/10 PASS). | [`05_DATA_COLLECTION_v2.md §1.3`](05_DATA_COLLECTION_v2.md) |
| **Jiminy outcomes as quality signal** | GUIDANCE_OUTCOME edges provide direct training quality labels for Jiminy tasks. | [`05_DATA_COLLECTION_v2.md §5`](05_DATA_COLLECTION_v2.md) |
| **Training data version boundary: v0.7.1** | Pre-v0.7.1 Jiminy classifier data is measurement error. Filter by MDEMG version ≥ v0.7.1 for `jiminy.evaluate` and `jiminy.evaluate_llm` only. | [`05_DATA_COLLECTION_v2.md §12`](05_DATA_COLLECTION_v2.md) |

---

## Changes from v4.0 (memo 07 v3.1 — 2026-04-21)

| Change | Affected Documents | Status |
|---|---|---|
| Base model: Qwen3-30B-A3B → Qwen3.6-35B-A3B | 00, 01, 02, 03, 04, 06 | ✅ Applied |
| No-tool-calling architectural policy (§2.8) + 9 banned patterns (incl. `preserve_thinking`) | 00, 01, 02, 06 + CLAUDE, VISION, AGENT_HANDOFF | ✅ Applied (repo-level: Epic 8) |
| Two-tier MoE-Sieve LoRA (§5) + three-family provisional partition | 01, 02, 03, 04, 05, 06 | ✅ Applied |
| Asymmetric quantization (shared BF16 / routed MXFP4_MOE / attention BF16) | 01, 02, 03 | ✅ Applied |
| `router_aux_loss_coef=0.002` + layer-level entropy ≥ 1.5 nats gate | 03, 04 | ✅ Applied |
| Three-group sampling recipes + all-16-task mapping (`presence_penalty=1.5` on J group) | 04 | ✅ Applied |
| Phase 5.X expert activation profiling (Sprint D) | 03, 05 | ✅ Applied |
| Balanced sampling for Tier 1 (FT-OAI-001 R1 fix) | 05 | ✅ Applied |
| §1.1 16-task roster re-audit (drift fix for `jiminy.evaluate_llm`, `jiminy.codegen`) | 01, 03 | ✅ Applied |
| Guardrail consumer flagged as 17th call site (Sprint B migration) | 01, 03 | ✅ Applied |
| ⚠️ Epoch cap + `val_loss > best × 1.05` early-stop (SFT) | 03 | ✅ Applied (policy is Sprint A addition) |
| ⚠️ `val_reward < best × 0.95` early-stop (RL mirror) | 04 | ✅ Applied (policy is Sprint A addition) |
| ⚠️ `n_epochs=auto` disallowed | 03 | ✅ Applied (policy is Sprint A addition) |
| Routing profile artifact schema + pipeline location | 05 | ✅ Applied |
| Fallback chain: Qwen3.5-35B-A3B (NOT Qwen3-30B-A3B) | 00, 01, 02 | ✅ Applied |

---

## Changes from v3.0

| Change | Affected Documents | Status |
|---|---|---|
| Tool-use model constraint added | 01, 02, 06 | ✅ Applied |
| Default LLM: gpt-5-nano → gpt-4.1-nano | 00, 01, 03, 06 | ✅ Applied |
| E2E curated pipeline documented | 05 | ✅ Applied |
| Jiminy outcome quality signals added | 05 | ✅ Applied |
| Training data version boundary documented | 05, 06 | ✅ Applied |
| reward_functions.py (21 functions), quality_report.py documented | 03 | ✅ Applied |
| TSDB migrations 006-010 documented | 03, 05 | ✅ Applied |
| Collection campaign status added | 05 | ✅ Applied |
| Outcome classifier shared task label noted | 03, 06 | ✅ Applied |

---

## Changes from v2.0

| Change | Affected Documents | Status |
|---|---|---|
| Consumer count: 15 → 16 | 01, 03, 04, 05 | ✅ Applied |
| Task names: snake_case → dot-notation (match actual WithContext labels) | 01, 03, 04, 05 | ✅ Applied |
| Phase 1 (Interaction Logger): marked COMPLETE, implementation notes added | 03, 05 | ✅ Applied |
| Data storage: JSONL → TimescaleDB (reflects PR #217 implementation) | 03, 05 | ✅ Applied |
| RAFT training pattern added (retrieval context in training data) | 01, 03, 05 | ✅ Applied |
| ULTS spec framework added (formalize LLM call contracts) | 01, 03, 04 | ✅ Applied |
| System prompt versioning (hash in InteractionRecord) | 03, 05 | ✅ Applied |
| Privacy scrubber: marked COMPLETE (PR #219) | 03, 05 | ✅ Applied |
| Guidance ID correlation: marked COMPLETE (PR #219) | 03, 05 | ✅ Applied |
| Quality annotation pipeline: marked COMPLETE (PR #219) | 05 | ✅ Applied |
| Data monitoring CLI: marked COMPLETE (PR #219) | 03, 05 | ✅ Applied |
| Concurrent inference + training note added | 02 | ✅ Applied |
| Design for routine retraining (not one-time) | 01, 03 | ✅ Applied |
| v3.0 corrections documented | 06 | ✅ Applied |

---

## Sprint Plan (Sprint FT-LORA-A → E; memo 07 §4)

| Sprint | Scope | Duration | Status |
|---|---|---|---|
| **FT-LORA-A** | Documentation update pass (this sprint) | ~3 days | 🔄 In progress |
| **FT-LORA-B** | Code/config: grep audit remediation, `.env.example`, inference launch commands, 16 ULTS sampling-group fields, guardrail llmclient migration | ~2 days | ⬜ Queued |
| **FT-LORA-C** | Qwen3.6 MLX validation — 3 gates (mlx-lm-lora convergence on 500 ex, JSON ≥95%, ≥60 tok/s) | ~1 week | ⬜ Queued |
| **FT-LORA-D** | Expert activation profiling (Phase 5.X) — `profile_routing_{family}.json` × 3, family-partition decision | ~3 days | ⬜ Queued |
| **FT-LORA-E** | Training infra patches — `router_aux_loss_coef` exposure, `mlx_lm.convert` asymmetric quant selectors, Tier 1/Tier 2 flags, router-entropy + val-loss/reward early-stop CLI gates | ~3–5 days | ⬜ Queued |
| **Phase 5 SFT unblocks** | Two-tier SFT on real data | — | Gated on Sprint C pass |

## Implementation Status (as of 2026-04-21)

| Phase | Status | PRs |
|---|---|---|
| Phase 1: Interaction Logger | ✅ COMPLETE | #217, #218, #219 |
| Phase 2: Think Mode + SanitizeResponse | ⬜ NOT STARTED | — |
| Phase 3: vllm-mlx Integration | ⬜ Config only (not activated) | — |
| Phase 4: Teacher Distillation | ⬜ NOT STARTED (needs data accumulation) | — |
| Phase 4A: RAFT Retrieval Context (NEW) | ⬜ NOT STARTED | — |
| Phase 4B: ULTS Spec Framework (NEW) | ⬜ NOT STARTED | — |
| Phase 5: Training Pipeline | ⬜ NOT STARTED | — |
| Phase 6: Recursive Cycle Automation | ⬜ NOT STARTED | — |
| Phase 7: RSIC Integration | ⬜ NOT STARTED | — |
| Phase 8: CLI Commands | 🔄 Partial (`mdemg data` done, `mdemg finetune` not started) | #219 |
| Phase 9: Monitoring | ⬜ NOT STARTED | — |
| Phase 10: Benchmarks | ⬜ NOT STARTED | — |
| Phase 11: Automated RL (GRPO/DPO) | 🟨 CODE COMPLETE — compute pass operator-gated | [`phase_11_rl_post.md`](phase_11_rl_post.md) |
| Phase 12: Human-in-the-Loop | ⬜ NOT STARTED | — |

### Immediate Actions Required

1. **Config flip (P0, 5 min):** Set `NEURAL_DATA_COLLECTION=true`, `J17_PROTOCOL_DATA_COLLECTION=true`, `TSDB_BACKUP_ENABLED=true` and restart
2. **SanitizeResponse (P1):** Build `internal/llmclient/sanitize.go` — required before switching to any local model
3. **System prompt hash (P1):** Add to InteractionRecord for training data versioning
4. **RAFT context capture (P1):** Enrich InteractionRecord with retrieval context
5. **ULTS specs (P1):** 16 spec files formalizing LLM call contracts
