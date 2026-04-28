# Sprint FT-LORA-PHASE11.5 — Research-Driven Breakthrough Past the +1.76pp Ceiling

## Context

Phase 11 GRPO LoRA training (Run 5: kl=0.05 / lr=2e-6 / 7 LoRA modules / stratified_by_task / 500 steps) produced the first real adapter that moved task scores: aggregate **0.8514** vs Phase 5 baseline **0.8338** = **+1.76pp**. The 5a aggregate target (+2pp = 0.8505) was met by 0.09pp; the per-task cap (-2pp) failed on three persistent C-group regressors (`consulting.classify` -4.33pp, `consulting.synthesis` -3.04pp, `retrieval.query_classify` -10.00pp). Run 6 (kl=0.10) regressed below baseline. Run 7 (warmup-cosine LR schedule) shows no dramatic improvement at step 400. **Hyperparameter tuning has plateaued.** This sprint replaces ~60h of reactive tuning with a hypothesis-driven research effort to clear a +5pp aggregate gain while eliminating per-task regressions >2pp, plus a documented gpt-5.4-mini comparison on the same benchmark.

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE11.5 |
| Title | Research-driven breakthrough — past the +1.76pp GRPO ceiling |
| Date | 2026-04-28 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-PHASE11 (Runs 1-7), Phase 11 MLX adapter chronicle |
| Successors | FT-LORA-PHASE12 (HITL DPO) — may absorb residual regressors |
| Type | Research-heavy: 7 diagnostic experiments, 1-3 strategy branches, training run reproduction |
| Risk | MEDIUM-HIGH: target requires real algorithmic insight, not hyperparameter sweep |
| Budget | OpenAI spend ≤ $80: gpt-5.4-mini benchmark ($10-30), optional judge-reward experiments ($20-40), seed-variance reruns ($5-15) |
| Targets | Aggregate ≥ Phase-5 + 5pp = **0.8838**; ∀task: Δ ≥ -2pp; documented gpt-5.4-mini number |
| Wall-clock estimate | 5-9 days (1-2d diagnostics, 2-5d branch execution, 1-2d gating + docs) |
| Base model | `.local-models/qwen3-14b-mdemg-v1` (locked, SHA `a54ec18f…`) |
| MLX server | `127.0.0.1:8101` (single instance) |

## 2. Problem Statement

After fixing the silent gradient-checkpointing bug (Run 5, PR #356 + 5504b36), the Phase 11 GRPO pipeline produces working training but the resulting adapter saturates at **+1.76pp aggregate**. Three structural pathologies are observed empirically:

1. **Hyperparameter span is exhausted.** kl ∈ {0.01, 0.05, 0.10}, lr ∈ {1e-5 (diverged), 2e-6}, sampler ∈ {by_group, by_task}, LoRA targets ∈ {attn-only, attn+mlp}, lr-schedule ∈ {flat, warmup+cosine} have all been swept. None move the ceiling materially. The "kl=0.10 regressed below baseline AND made `retrieval.query_classify` worse" finding is the dispositive evidence that the regressor mode is not a kl problem.
2. **Three persistent C-group regressors.** `consulting.classify`, `consulting.synthesis`, `retrieval.query_classify` regress on every meaningful run. They share: deterministic temp=0 evaluation, 5-7 reward-source rows, single-token-flip score sensitivity (one wrong label = 10-20pp aggregate move).
3. **Run 5's "+1.76pp" is fragile.** Removing the `hidden.reclassify` 0.50→1.00 flip (likely a baseline-side scoring artifact, identical across all 4 prior no-op runs) collapses the net per-task ledger to approximately neutral. Gate 5b's 0.0131 byte-identical-adapter delta defines a noise floor that swamps the apparent gain.

This sprint must produce a fundamentally different result — at least +5pp aggregate over Phase 5, no per-task regressions >2pp, and a documented gpt-5.4-mini benchmark number — by isolating which of seven concrete hypotheses (H1-H7) is the binding constraint and then committing to a strategy branch driven by that data.

## 3. Scope & Constraints

**In scope:** Diagnostic experiments X1-X7 (one per hypothesis), each cheap and time-boxed with explicit pass/fail. A decision gate that selects one of three strategy branches (Sampling-Fix / Reward-Regeneration / Multi-Stage SFT→DPO→GRPO) from diagnostic data. Execution of selected branch including any training infra changes, training run, dual-gate regression (5a + 5b), and post-mortem. Mandatory gpt-5.4-mini benchmark on the same Phase 10 16-task suite. Reduced-noise re-evaluation methodology (more runs, multi-seed) as a precondition for trusting any +5pp claim. 3-tier tests for any code change. Documentation updates.

**Out of scope:** Hyperparameter tuning (already exhausted). Base-model changes (locked per Sprint A). Tool-use additions (architectural ban). Phase 12 HITL DPO loop (separate sprint). Distillation / multi-adapter ensembling. Live psycopg writer for `rl_training_*` tables.

**Constraints (hard):** MEMORY policies (epochs ≤ 3, n_epochs=auto disallowed, max_tokens ≥ 3000, latency_budget_ms ≥ 15000, CUIDv2 run_ids, no hardcoded values). Single MLX server `127.0.0.1:8101`. No-tool-calling 9-pattern grep audit on every commit. Single batched commit at sprint close. Auto-PR workflow on push (never manual). Sprint summary posted to PR comments immediately after push. Dual regression gate (5a + 5b) blocking before any adapter is promoted to `…-rl/`. The 5b threshold may be widened to `0.020` based on the empirically measured byte-identical noise floor — *only* if X6 confirms it, with the change documented in `configs/rl_phase11.yaml`.

## 4. Dependencies

**Consumed code/data:** `neural/training/rl/{trainer.py, mlx_adapter.py, grpo_loss.py, reward_sampler.py, advantage.py, live_wiring.py, regression.py}`, `neural/training/dpo/pair_generator.py`, `neural/benchmarks/{run_benchmark.py, sampling_policy.py, llm_judge.py, variance.py}`, `configs/{rl_phase11.yaml, benchmark_phase10.yaml, dpo_phase12_pairs.yaml}`, `docs/tests/ults/specs/*.ults.json`, `training_data/eval/{benchmark_qwen3_14b_v1_baseline.json, phase11_regression_report_real.json, valid_golden.jsonl}`, `benchmark_results` TSDB rows (Phase 10 run_id `q283a23bz59mrg6faxo32ydx2`), Run 5 sandbox adapter at `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/`.

**External services:** OpenAI API (gpt-5.4-mini), local MLX server `:8101`, TSDB (additive). No Neo4j writes.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate (Epic 0):** branch clean, venv active, `:8101` single-instance, Phase 5 base SHA pin asserted, OpenAI key present, Run 5 sandbox adapter present, Run 7 status read.

### Epic 1 — Diagnostic Phase: Experiments X1-X7

#### X1 — Training/eval sampling distribution (H1)
Hypothesis: training rollouts use mlx_lm.generate defaults (temp ≈ 1.0); C-group eval uses temp=0. Mismatch is dominant cause of C-group regressions. Probe: instrument `MLXGRPOAdapter.rollout_fn` to log sampling kwargs + per-token entropy on 32-rollout sample for each of 3 regressors at default vs C-group recipe vs J-group recipe. Compute argmax-flip rate. Decision: **confirms** at flip rate ≥30%, **refutes** at <5%. Cost: $0, ~30 min. Requires Metal (run after Run 7 completes).

#### X2 — Reward-signal coverage (H2)
Hypothesis: Phase 10 base-rollout responses bound GRPO's exploration. Probe: from `benchmark_results` rows, compute per-task row count, reward mean/stddev, distinct response cluster count. Persist to `x2_reward_coverage.json`. Decision: **confirms** if regressors have ≤2 clusters or stddev=0; **refutes** at ≥3 clusters with non-trivial stddev. Cost: $0, ~20 min.

#### X3 — Dataset scale (H3)
Hypothesis: 5-7 rows is below GRPO floor for tail tasks. Probe: count rows by sampling_group × task × outcome. Persist to `x3_dataset_counts.json`. Decision: **confirms** if regressors ≤7 rows AND ≤2 max-reward; **refutes** at ≥10 rows with diversity. Cost: $0, ~10 min.

#### X4 — Reward-function alignment (H4)
Hypothesis: registry rewards don't correlate with benchmark `overall_mean`. Probe: per-row Spearman correlation between scalarized registry reward and benchmark per-task overall_mean. Decision: **confirms** if regressor correlations <0.5 AND non-regressor controls >0.7; **refutes** at correlations ≥0.7. Cost: $0, ~30 min (optional $20-30 judge-reward POC).

#### X5 — DPO pair viability (H5)
Hypothesis: preference learning suits small-row deterministic-eval tasks better than GRPO. Probe: run DPO pair generator against existing `benchmark_results`. Report per-task pair count, reward-delta distribution. Decision: **confirms viability** at ≥8 high-quality pairs/regressor (delta ≥0.30); **refutes** at <4 pairs/regressor. Cost: $0, ~15 min.

#### X6 — Evaluation noise floor (H6)
Hypothesis: +1.76pp is comparable to seed-and-sampling noise. Probe: run Phase 10 benchmark 3x against base at different RNG seeds + once with n_runs=20. Report cross-seed mean ± std. Decision: **confirms** if cross-seed agg std ≥1.0pp OR regressor std ≥5pp (eval methodology must tighten before any branch); **refutes** at agg std ≤0.3pp / per-task ≤2pp. Cost: $5-15 OpenAI, ~3-4h. Requires Metal (run after Run 7 completes).

#### X7 — gpt-5.4-mini comparison (H7) [ALWAYS]
One-shot Phase 10 benchmark with `model_under_test` pointed at gpt-5.4-mini. Persist to `gpt54mini_benchmark.json`. Mandatory data-collection regardless of outcome. Cost: $10-30, ~30-45 min. No Metal required.

#### Diagnostic Decision Matrix (Epic 1 exit gate)

Branch selection rule:
- If H1 confirmed AND H6 refuted → **Branch A** (Sampling-Fix).
- Else if H2/H3/H4 collectively confirmed → **Branch B** (Reward-Regeneration).
- Else if H5 confirmed → **Branch C** (Multi-Stage SFT→DPO→GRPO).
- If H6 confirmed, *prepend* methodology-tightening sub-epic to selected branch.

### Epic 2 — Strategy Branch Execution

Exactly one branch executes; unselected branches' design notes retained in post-mortem.

**Branch A — Sampling-Fix:** Load `rollout.sampling_policy_config` YAML; propagate per-sample-group sampling kwargs (temp, top_p, top_k, presence_penalty) into `generation_kwargs` per sample. RewardSampler already surfaces sampling_group; rollout function selects right recipe per sample. Validation: unit test asserts mock generate called with right kwargs per [T,C,J] sample. Training run: Run 5 config + sampling fix, 500 steps, ~6-8h. Re-eval: dual gate at (possibly tightened per X6) thresholds.

**Branch B — Reward-Regeneration:** B1 (mandatory if H2): regenerate `benchmark_results` rows using Run 5 adapter as rollout source. Train Phase 11.5 GRPO from richer reward distribution. B2 (mandatory if H3): generate 30-50 additional rollouts/regressor at varying temps (0.0, 0.4, 0.7, 1.0); score via gpt-5.4-mini judge or seed DPO pair generation. B3 (mandatory if H4): introduce `judge_reward` reward function — gpt-5.4-mini single-call score; **only during reward-source-regeneration, never per RL step** (enforced by call-site assertion). Code: `reward_sampler.py` multi-source loader, `reward_functions.py` judge_reward, new YAML key `reward_sampler.synthetic_sources`.

**Branch C — Multi-Stage SFT→DPO→GRPO:** Stage 1 (SFT): small SFT on high-reward responses for 3 regressors only, ~50-100 examples, 1-2 epochs, low LR → `…-rl-sandbox-sft/`. Stage 2 (DPO): train DPO on Phase 11 pair generator output against SFT checkpoint, existing config extended with per-task min-pair counts → `…-rl-sandbox-dpo/`. Stage 3 (GRPO): standard Phase 11 GRPO from DPO checkpoint, Run 5 hyperparameters. Each stage has intermediate dual-gate 5a check; end-to-end gated on final 5a/5b. Code: new `neural/training/dpo/trainer.py` (was deferred from Phase 11). Wall-clock: ~12-18h cumulative.

### Epic 3 — Methodology Tightening (conditional on H6)

Modify `configs/benchmark_phase10.yaml`: add `n_runs_for_gating: 20`, `gating_seed_count: 3`. Update `regression.py`: gate 5a uses cross-seed mean; gate 5b threshold raised to ≤ measured noise floor + 1 std (estimate ~0.015-0.020). Threshold changes documented in YAML comments with X6 result.

### Epic 4 — gpt-5.4-mini Comparison Documentation [ALWAYS]

Persist X7 result. Build 3-column comparison table in `phase_11_mlx_adapter.md` (Phase 11.5 section): base / Phase 11.5 adapter / gpt-5.4-mini, per-task and aggregate. Cost-replacement decision matrix.

### Epic 5 — Final Dual-Gate + Adapter Promotion

Run regression harness at (possibly tightened) thresholds. If 5a + 5b PASS → promote to `.local-models/qwen3-14b-mdemg-v1-rl/`. If FAIL → adapter stays in sandbox; honest post-mortem.

### Epic 6 — Documentation (Final Epic — Never Cut)

Append "Phase 11.5 — Research-driven breakthrough" section to `phase_11_mlx_adapter.md` covering: per-experiment results (X1-X7), branch-selection rationale, training-run characterization, gate verdict, gpt-5.4-mini comparison, "what this rules in / rules out" lessons section. Update `00_README_v2.md`, `04_BENCHMARK_RL_v2.md`, `AGENT_HANDOFF.md`, `CHANGELOG.md`, `CLAUDE.md`.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit, ≤2s):** Branch A: per-sample generation_kwargs propagation; sampling-policy YAML loader. Branch B: synthetic-source loader; judge_reward (mocked OpenAI); reward-sampler multi-source merge. Branch C: DPO trainer step (mocked MLX); SFT-DPO-GRPO orchestration. Always: any new YAML knob has parser test.

**Tier 2 (Integration, ≤30s):** 3-task × 5-step training smoke under selected branch with mocked MLX adapter, asserting kwargs flow end-to-end. X1-X7 each have 1-task fixture-driven smoke. Regression harness end-to-end with stub benchmark runner.

**Tier 3 (E2E, operator-gated):** Real MLX 25-step smoke. Final 500-step training. Final dual-gate Phase 10 benchmark.

Quality bar: all 109 prior tests stay green; no test-count regressions.

## 7. Commit Strategy

Single batched commit at sprint close: configs/, neural/training/rl/ + dpo/, tests, training_data/eval/phase11_5_diagnostics/, sprint plan + chronicle, top-level docs. Auto-PR fires on push; sprint summary posted to PR comments immediately.

## 8. Verification Checklist

- [ ] All seven diagnostic experiments executed; results in `training_data/eval/phase11_5_diagnostics/`
- [ ] Decision-matrix outcome documented with branch-selection rationale
- [ ] Selected branch's code changes have unit + integration tests; all tests green (109+ + new)
- [ ] H6 noise-floor measurement done; 5b threshold either kept at 0.005 (refuted) or widened with explicit value + rationale (confirmed)
- [ ] gpt-5.4-mini benchmark run completed; per-task table in docs
- [ ] Final training run completed; adapter SHA recorded
- [ ] Dual-gate 5a + 5b verdict recorded (PASS → promoted; FAIL → post-mortem)
- [ ] Aggregate ≥ 0.8838 (Phase-5 + 5pp): met / not-met (documented either way)
- [ ] No per-task regression > 2pp
- [ ] No-tool-calling grep audit clean
- [ ] All MEMORY policies honored
- [ ] Branch `reh3376_dev01`, single batched commit, auto-PR fired, sprint summary in PR

## 9. Documentation Update (Final Epic — Never Cut)

Per `phase_11_mlx_adapter.md` precedent. Per-experiment results (X1-X7) with metrics + decision threshold + verdict. Branch-selection rationale. Training-run characterization. Gate verdict and per-task delta table. gpt-5.4-mini comparison and cost-replacement matrix. "What this rules in / rules out" load-bearing for Phase 11.6 / 12 decisions.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| 1 | All seven hypotheses weak; no clear binding constraint | Medium | High | Diagnostic phase is itself the deliverable; document "no clear answer" honestly; recommend architectural escalation (different base / LoRA rank ≥64 / reward-model training) |
| 2 | gpt-5.4-mini benchmark spend overruns | Low | Low | Hard-cap per-task budget at $5; abort if exceeded |
| 3 | Branch C training (~12-18h) hits Mac memory wall | Medium | Medium | Re-use Phase 11 Tier 3 gradient-checkpointing path; operator-gated full run; preconditions (fresh reboot, no GPU consumers, batch_size=1 first) |
| 4 | Methodology tightening makes prior runs incomparable | Medium | Low | Re-run Phase 5 baseline at tightened settings before declaring any gate verdict |
| 5 | Branch B's judge-reward becomes per-step hot path by accident | Low | High | Hard rule: judge_reward only during reward-source-regeneration, never inside `MLXGRPOAdapter.rollout_fn`. Enforced by call-site assertion |
| 6 | Run 7 finishes during sprint and changes baseline | Medium | Low | Capture Run 7's adapter and aggregate at sprint kickoff; treat as new ceiling reference if non-trivially better; +5pp target stays anchored to Phase 5 (0.8838) |
| 7 | "+1.76pp" was largely noise (`hidden.reclassify` artifact + 5b 0.0131 floor) | High | High | X6 measures this directly; if confirmed, prior runs' apparent gains de-rated; new run must clear noise floor with margin; +5pp target unaffected |

## 11. Documents Accessed (during planning)

- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_mlx_adapter.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_ft_lora_phase11.md` (format reference)
- `/Users/reh3376/mdemg/configs/rl_phase11.yaml`
- `/Users/reh3376/mdemg/configs/benchmark_phase10.yaml`
- `/Users/reh3376/mdemg/neural/training/rl/trainer.py`
- `/Users/reh3376/mdemg/neural/training/rl/reward_sampler.py`
- `/Users/reh3376/mdemg/neural/training/rl/regression.py`
- `/Users/reh3376/mdemg/neural/training/rl/live_wiring.py`
- `/Users/reh3376/mdemg/neural/training/rl/mlx_adapter.py`
- `/Users/reh3376/mdemg/neural/training/dpo/pair_generator.py`
- `/Users/reh3376/mdemg/training_data/eval/phase11_regression_report_real.json`

## 12. Rollback Procedures

**Diagnostic phase:** all X1-X7 outputs are read-only data files in `training_data/eval/phase11_5_diagnostics/`. Removable individually if results need to be regenerated.
**Branch A code:** revert sampling-policy propagation patch; only behavior change is `mlx_adapter.py` rollout call site and `trainer.main()` YAML loading.
**Branch B code:** synthetic-source loader and judge_reward additive. Setting `reward_sampler.synthetic_sources: []` and disabling judge_reward in YAML restores Phase 11 behavior bit-identically. New TSDB rows additive (no schema changes).
**Branch C code:** new `dpo/trainer.py` additive. SFT and DPO checkpoints in own sandbox dirs.
**Adapter promotion:** if 5a/5b fail, new adapter stays in sandbox; production `…-rl/` (if from Phase 11) untouched. If 5a/5b pass and promoted adapter later misbehaves, restore previous sandbox SHA from `phase_11_mlx_adapter.md`.
**Methodology change:** YAML threshold revert is one-line; only persistent effect is documented values, versioned in git.
