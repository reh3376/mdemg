# Branch B Implementation Plan — Knowledge Distillation from gpt-5.4-mini

**Sprint:** FT-LORA-PHASE11.5 (Branch B)
**Date:** 2026-04-28
**Branch:** `reh3376_dev01`
**Status:** PAUSED 2026-04-28 — superseded by Phase 11.5c findings. See `phase_11_5c_post.md`.
**Predecessor:** sprint_plan_phase_11_5.md (Phase 11.5 diagnostic phase, X1–X5+X7 complete)

> **PAUSED:** Phase 11.5c clean re-baseline (`valid_clean.jsonl`) revealed that the
> "regressor" tasks chosen for this Branch B distillation set were artifacts of a
> 99% leaked golden eval. The original primary target `retrieval.query_classify`
> scores 0.90 on real production data (NOT 0.60 as the leaked golden showed).
> The real regressors are `consulting.classify` (P5 0.49 vs gpt-mini 0.88, +39pp gap)
> and `retrieval.rerank_cross` (P5 0.72 vs gpt-mini 0.90). When this plan resumes,
> the distill target set changes accordingly. Realistic ceiling: +2.0–2.7pp aggregate
> on `valid_clean` (not the previously claimed +5pp on leaked golden).

---

## 1. Problem Statement

Six successive GRPO runs (Runs 1–7) on Qwen3-14B-4bit + Phase 5 SFT plateau at **+1.76pp aggregate over Phase 5 baseline** (0.8338 → 0.8514). Three C-group classifier tasks regress persistently:

| Task | Phase 5 | Run 5 (best RL) | gpt-5.4-mini | Gap to gpt-mini |
|---|---|---|---|---|
| `retrieval.query_classify` | 0.90 | 0.60 | 0.90 | +30pp |
| `consulting.classify` | (~baseline) | regressed | (high) | meaningful |
| `guardrail.evaluate` | (~baseline) | regressed | (high) | meaningful |

Diagnostics (X1–X5, X7) **ruled out**:
- **H1 sampling mismatch** — REFUTED. X1 argmax-flip rate 0.0% across `default_greedy`, `c_recipe`, `training_typical`. Training is already greedy at temp=0.
- **H2 reward coverage gap** — PARTIAL (2/3 regressors confirmed; not the binding constraint)
- **H3 dataset starvation** — REFUTED. X3 confirms all 16 tasks have ≥5 golden rows.
- **H4 reward/eval misalignment** — REFUTED. ρ=1.0 between training reward and eval signal.
- **H5 DPO viability** — UNVIABLE. X5 found only 5 total reward-delta pairs across 16 tasks.

**Binding constraint identified (H6):** the base Qwen3-14B-4bit model produces **deterministically-wrong responses** on the 3 regressor tasks. `distinct_first_token=1` across all temperatures. Reward-driven RL cannot escape a deterministic fixed point — the gradient signal is zero when the policy collapses to one wrong output.

**gpt-5.4-mini benchmark (X7):** aggregate **0.8769** (`training_data/eval/phase11_5_diagnostics/x7_gpt54mini_benchmark.json`). gpt-mini outperforms Run 5 on the 3 regressors but **Run 5 already beats gpt-mini** on `ape.reflect` (+5pp) and `jiminy.synthesize` (+5.5pp). Distillation must preserve those wins.

**Goal:** aggregate **≥ 0.8838** (+5pp over Phase 5), zero per-task regressions > 2pp, retained Run 5 wins on `ape.reflect` and `jiminy.synthesize`.

---

## 2. Strategy — Two-Stage Distillation

```
Phase 5 SFT (0.8338)
       │
       ▼
Stage 1: SFT distillation from gpt-5.4-mini on 5 distillation tasks
         (the 3 regressors + 2 wins-to-preserve as anchor)
         → train via mlx_lm.lora (proven SFT path, not custom GRPO)
         → expected: regressors recover; aggregate ≈ 0.86–0.88
       │
       ▼
Stage-1 regression gate (5a + 5b)
       │
       ├─ PASS @ ≥0.8838 → promote → Epic 6
       │
       └─ PASS but < 0.8838 → Stage 2 (GRPO from distilled adapter)
              │
              ▼
       Stage 2: GRPO continues from Stage-1 adapter, ratchets remaining tasks
              │
              ▼
              Final regression gate → promote
```

**Why distillation works where GRPO failed:** SFT puts a *correct* output in the loss directly. The gradient is nonzero by construction. Once the policy can produce the correct first token, downstream RL has signal to refine.

**Why mlx_lm.lora (not custom):** the Phase 5 SFT pipeline is the proven one. Reusing it sidesteps the custom-trainer footguns (gradient checkpointing closure-capture, etc.) we hit in the GRPO path.

---

## 3. Scope & Constraints

**In scope:**
- New gpt-5.4-mini response-capture driver (`scripts/x8_distill_dataset_capture.py`, extends X7 transport)
- Distillation JSONL dataset on 5 tasks (3 regressors + 2 anchor wins)
- New SFT config `configs/sft_phase11_5_distill.yaml` (lr=1e-5, 2 epochs, batch=4, same 7 LoRA modules as Run 7)
- Stage-1 regression via existing `neural/training/rl/regression.py`
- Conditional Stage-2 GRPO (only if Stage-1 doesn't hit target)
- Documentation update

**Out of scope:**
- Re-pivoting to vendor `mlx-lm-lora` for GRPO
- Multi-teacher distillation (Claude + gpt-mini)
- Reward model training
- Phase 12 HITL DPO (separate sprint)
- New TSDB migrations (V0013 already covers RL training; SFT distill reuses Phase 5 paths)

**Hard constraints (MEMORY):**
- **Epoch cap = 3** on all LoRA runs; explicit integer epochs (`n_epochs=auto` disallowed). Stage-1 SFT uses `epochs: 2` to leave headroom for early-stop.
- **Early-stop:** SFT path → `val_loss > best × 1.05` for 2 consecutive evals.
- **No hardcoded values** — lr, batch, epochs, LoRA modules all in `configs/sft_phase11_5_distill.yaml`.
- **CUIDv2** for any new run_ids (reuse `cuid2` Python package).
- **Sequential epics** — Epic N+1 starts only after Epic N gate passes.
- **3-tier testing** — unit (dataset construction), integration (mlx_lm.lora invocation, mocked OpenAI), e2e (real Phase 5 base + real OpenAI capture, regression gate).
- **min `max_tokens` ≥ 3000, min `latency_budget_ms` ≥ 15000** for any LLM judge in regression.
- **Plan-options pattern** — distillation-task-set, GRPO start point both disclosed at PR.
- **Single batched commit** at sprint close.
- **Sprint summary on PR comments** immediately after push.
- **Base model read-only** — `.local-models/qwen3-14b-mdemg-v1/` never overwritten.
- **MLX single-instance** preflight on `127.0.0.1:8101` before training and regression.

---

## 4. Dependencies

**Consumed (code, pre-existing):**
- `neural/benchmarks/run_benchmark.py` — Phase 10 runner; the X7 driver wires it to OpenAI.
- `neural/training/rl/regression.py` — Phase 11 dual regression gate; reused for Stage-1 verdict.
- `mlx_lm.lora` CLI — proven SFT trainer used in Phase 5.
- `.local-models/qwen3-14b-mdemg-v1/` — Phase 5 dense base + adapter, distillation starting point.
- `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/` — Run 7 adapter (514MB, 7 LoRA modules) — Stage-2 GRPO start point.
- `configs/benchmark_phase10.yaml` — sampling recipes, group weights.
- `scripts/x7_gpt54mini_benchmark.py` — OpenAI HTTP transport (port for Epic 1 capture driver).
- `training_data/eval/valid_golden.jsonl` — golden holdout, source of distillation prompts.

**Consumed (data):**
- `docs/tests/ults/specs/*.ults.json` — 17 ULTS specs (16 active).
- `training_data/eval/benchmark_qwen3_14b_v1_baseline.json` — Phase 5 baseline 0.8338.
- `training_data/eval/phase11_5_diagnostics/x7_gpt54mini_benchmark.json` — gpt-mini benchmark 0.8769.

**Consumed (compute):**
- Apple Silicon ≥48GB unified memory (Phase 5/Run 7 fit).
- OpenAI API — `gpt-5.4-mini` capture (Epic 1) + judge replay during regression (Epic 3). Estimate $10–20.

---

## 5. Implementation Plan (Sequential Epics + Gates)

### Epic 0 — Pre-flight (≈1 hr)

1. Verify Run 7 sandbox adapter intact: `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/adapters.safetensors` (514MB, 7 modules per `adapter_config.json`). ✅ verified
2. Confirm `training_data/eval/phase11_5_diagnostics/x7_gpt54mini_benchmark.json` aggregate=0.8769. ✅ verified — but file does NOT contain per-task `response_text` (only summary stats). **Epic 1 must rerun gpt-mini with response capture.**
3. Smoke-test SFT pipeline: `python -m mlx_lm lora --help` returns 0; venv has `mlx_lm` deps.
4. `pytest neural/training/rl/tests/ neural/training/dpo/tests/` — confirm test suite still green.
5. Confirm MLX single-instance: only one `mlx_lm.server :8101` process if running.
6. **Decision recorded:** Stage-2 GRPO start point = Run 7 sandbox adapter (saves ≈13hr re-training; ≤0.3pp aggregate sacrifice that Stage-1 SFT recovers).

**Gate:** all 5 checks green; Epic 1 unblocked.

### Epic 1 — Capture gpt-5.4-mini Distillation Dataset (≈1.5 hr, $5–10 OpenAI)

1. New driver `scripts/x8_distill_dataset_capture.py` — extends X7 transport to log `{prompt, response_text, task_id, golden_row_idx, sampling_group}` per call to `training_data/distill/phase11_5/raw_responses.jsonl`.
2. Capture targets — **5 distillation tasks** (default):
   - 3 regressors: `retrieval.query_classify`, `consulting.classify`, `guardrail.evaluate`
   - 2 anchor wins (preserve): `ape.reflect`, `jiminy.synthesize`
3. Per-task capture: 5 golden prompts × 10 samples (max_completion_tokens=3000, temp=0.0) = 50 responses/task = 250 OpenAI calls.
4. Filter: keep only responses where `compute_reward(response)` ≥ 0.8 (high-quality teacher signal). Document drop rate.
5. Convert to mlx_lm.lora chat-format JSONL: `{"messages":[{"role":"user","content":<prompt>},{"role":"assistant","content":<response>}]}`.
6. Output: `training_data/distill/phase11_5/train.jsonl` + `valid.jsonl` (90/10 split, stratified by task).
7. Manifest: per-task pair count, reward distribution, total tokens, OpenAI spend, SHAs.

**Gate:** ≥40 high-quality pairs/task (≥80% retention); manifest written; spend logged.

### Epic 2 — Stage-1 SFT Training via mlx_lm.lora (≈3–4 hr)

1. New config `configs/sft_phase11_5_distill.yaml`:
   ```yaml
   model: ".local-models/qwen3-14b-mdemg-v1"  # Phase 5 base
   train: true
   data: "training_data/distill/phase11_5/"
   lora_layers: 40                            # match Phase 5
   lora_parameters:
     rank: 32
     scale: 2.0
     dropout: 0.05
     keys:                                    # 7 modules — match Run 7
       - "self_attn.q_proj"
       - "self_attn.k_proj"
       - "self_attn.v_proj"
       - "self_attn.o_proj"
       - "mlp.gate_proj"
       - "mlp.up_proj"
       - "mlp.down_proj"
   batch_size: 4
   iters: <derived from epochs=2>
   learning_rate: 1.0e-5
   max_seq_length: 4096
   adapter_path: ".local-models/qwen3-14b-mdemg-v1-distill-sandbox"
   save_every: 100
   steps_per_eval: 50
   ```
2. Invoke: `python -m mlx_lm lora --config configs/sft_phase11_5_distill.yaml --train`.
3. Monitor: train_loss + val_loss every 50 steps; early-stop if `val_loss > best × 1.05` two consecutive evals.
4. Output: `.local-models/qwen3-14b-mdemg-v1-distill-sandbox/adapters.safetensors`.

**Gate:** training completes within 2 epochs OR early-stops; final val_loss < initial × 0.5 (sanity: actually learned).

### Epic 3 — Stage-1 Regression Gate (≈1 hr, $5 OpenAI judge)

1. Merge Stage-1 adapter onto fresh Phase 5 base → `.local-models/qwen3-14b-mdemg-v1-distill-fresh-merge/`.
2. `python -m neural.training.rl.regression --config configs/rl_phase11.yaml --sandbox-adapter .local-models/qwen3-14b-mdemg-v1-distill-sandbox --fresh-adapter .local-models/qwen3-14b-mdemg-v1-distill-fresh-merge`.
3. **5a target:** aggregate ≥ 0.8838 (+5pp), no per-task regression > 2pp, retained `ape.reflect` and `jiminy.synthesize` Run 5 deltas.
4. **5b target:** sandbox vs fresh-merge delta ≤ 0.5pp.
5. Verdict: pass → Epic 6 (skip Stage 2); fail-but-improving → Epic 4 (Stage 2 GRPO).

**Gate:** verdict recorded with full per-task table.

### Epic 4 — Stage-2 GRPO (CONDITIONAL, ≈4–6 hr) — only if Epic 3 passes 5b but misses 5a aggregate

1. Reuse `configs/rl_phase11.yaml` with starting adapter = Stage-1 Distill (NOT Run 7).
2. Restrict GRPO updates to the 3 regressor tasks if Stage-1 already saturated others (filter via `--task-filter`).
3. lr=2e-6, kl_coef=0.05 (Run 5 hyperparams), max_steps=300.
4. Same memory hygiene: `mx.clear_cache()` per step, combined `mx.eval` barrier, no `set_wired_limit`.

**Gate:** Stage-2 completes; regression repeated.

### Epic 5 — Final Regression (CONDITIONAL, follows Epic 4)

Same harness as Epic 3, against Stage-2 sandbox + fresh-merge.

**Gate:** ≥ 0.8838 aggregate, no per-task > 2pp regression, anchor wins retained.

### Epic 6 — Adapter Promotion

On final pass: rename winning sandbox → `.local-models/qwen3-14b-mdemg-v1-rl/`. Update `manifest.json` with config SHA, parent (Phase 5 base SHA), distillation manifest SHA, regression report SHA.

### Epic 7 — Documentation (Final Epic — Never Cut)

1. `docs/development/ft-lora/phase_11_5_distillation_post.md` — executed-truth: per-task delta table, OpenAI spend, distillation manifest stats, Stage-1/2 verdicts, plan-options decisions disclosed.
2. `00_README_v2.md` v5.8 → v5.9: Phase 11.5 EXECUTED.
3. `03_IMPLEMENTATION_PLAN_v2.md §Phase 11.5` — EXECUTED with SHA stamps.
4. `04_BENCHMARK_RL_v2.md` — distillation as fallback to GRPO documented.
5. `AGENT_HANDOFF.md` top entry; `CHANGELOG.md [Unreleased] ### Added`.
6. `CLAUDE.md` Testing section — add SFT-distill invocation.

**Gate:** all docs committed; cross-refs valid.

---

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit):**
- `tests/training/test_distill_dataset.py` — JSONL conversion correctness, reward filter, train/valid split stratification.
- `tests/training/test_x8_capture_driver.py` — mocked OpenAI; verify response_text persistence; reward function compatibility.

**Tier 2 (Integration):**
- mlx_lm.lora invocation against tiny stub dataset (10 pairs); verify adapter file written, val_loss recorded.
- Regression harness against canned Stage-1 adapter (mocked benchmark scores) — verifies pass/fail routing.

**Tier 3 (E2E):**
- Smoke (3 distillation tasks × 5 prompts × 5 samples = 75 OpenAI calls, $1–2): validate full pipeline.
- Full Epic 1 capture, Epic 2 SFT, Epic 3 regression — end-to-end on real Phase 5 base.

---

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ft-lora): Phase 11.5 Branch B — gpt-5.4-mini knowledge distillation (+Xpp aggregate)`
- Body: motivation (GRPO plateau diagnosis), distillation task set + rationale, Stage-1 verdict, Stage-2 verdict (if run), per-task delta table, OpenAI spend, plan-options disclosed, policy-compliance checklist.
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push → auto-PR → sprint summary posted to PR comments immediately.

---

## 8. Verification Checklist

- [ ] Epic 0: sandbox intact; X7 verified; mlx_lm.lora smoke green; existing test suite green; MLX single-instance
- [ ] Epic 1: ≥40 pairs/task × 5 tasks; manifest with SHAs; OpenAI spend ≤ $10
- [ ] Epic 2: SFT completes within 2 epochs; val_loss converges; sandbox adapter written
- [ ] Epic 3: regression aggregate ≥ 0.8838 OR fail-improving (Stage 2)
- [ ] Epic 4 (cond): Stage-2 GRPO completes
- [ ] Epic 5 (cond): final regression passes
- [ ] Epic 6: adapter promoted to canonical path
- [ ] Epic 7: docs committed
- [ ] Single commit pushed; auto-PR opened; sprint summary on PR
- [ ] OpenAI spend logged, under $100 cap

---

## 9. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | Distillation regresses Run 5 wins on `ape.reflect` / `jiminy.synthesize` | Medium | Include both tasks in distillation set as anchors; gate on per-task delta ≤ 2pp | Drop those tasks from distill set; rely on Stage-2 GRPO to recover |
| 2 | gpt-5.4-mini responses fail reward filter (filter rate > 50%) | Low–Med | Filter threshold configurable; X7 already showed 0.8769 aggregate so most score well | Lower filter to 0.7; document compromise |
| 3 | mlx_lm.lora overfits with 250 distill pairs | Medium | epochs=2 cap, early-stop on val_loss, batch=4 (regularizes) | Reduce to 1 epoch; or augment with Phase 5 SFT tail |
| 4 | Stage-1 alone falls < 0.8838 (Stage-2 needed) | Medium | Plan budgets for Stage-2 explicitly; conditional epics 4-5 | Accept Stage-1 verdict + document gap; Phase 12 picks up |
| 5 | OpenAI spend > $20 (Epic 1 retries / capture overruns) | Low | max_completion_tokens=3000 cap; deterministic temp=0; one retry per call | Halt Epic 1 at $15 mark, ship with reduced distill set |
| 6 | Adapter merge corruption (5b fails) | Low | Same dual-merge pattern from Phase 11 regression; pin mlx version | Re-merge with bf16-explicit; document |

---

## 10. Documents Accessed (during planning)

- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_mlx_adapter.md`
- `/Users/reh3376/mdemg/training_data/eval/phase11_5_diagnostics/x{1,2,3,4,5,7}*.json`
- `/Users/reh3376/mdemg/scripts/x7_gpt54mini_benchmark.py`
- `/Users/reh3376/mdemg/configs/rl_phase11.yaml`
- `/Users/reh3376/mdemg/neural/training/rl/{trainer,mlx_adapter,regression,reward_sampler}.py`
- `/Users/reh3376/mdemg/.local-models/qwen3-14b-mdemg-v1-rl-sandbox/adapter_config.json`
- `/Users/reh3376/mdemg/CLAUDE.md`
- Memory: `feedback_mlx_set_wired_limit_footgun.md`, `feedback_mlx_checkpoint_closure_footgun.md`, `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `project_phase5_moe_pivot.md`

---

## 11. Rollback

Distillation is additive.

1. `git revert <final commit SHA>`.
2. `rm -rf .local-models/qwen3-14b-mdemg-v1-distill-sandbox/ .local-models/qwen3-14b-mdemg-v1-distill-fresh-merge/ training_data/distill/phase11_5/ configs/sft_phase11_5_distill.yaml scripts/x8_distill_dataset_capture.py`.
3. If Epic 6 ran: restore previous `.local-models/qwen3-14b-mdemg-v1-rl/` from sandbox backup.
4. Phase 5 + Phase 10 + Phase 11 (Run 7) artifacts untouched throughout.

No TSDB writes (Epic 1 captures to JSONL only). No Neo4j writes.

---

## 12. Time + Budget Projection

| Path | Wall-clock | OpenAI $ |
|---|---|---|
| Best case (Stage-1 hits 0.8838) | 7–9 hr | ~$10 |
| Full path (Stage-1 + Stage-2) | 14–16 hr | ~$15 |
| Worst case (Stage-1 + Stage-2 + remediation) | 22–24 hr | ~$20 |

Within MEMORY $100 cap at all paths.

---

## Approval

User approved 2026-04-28 with directive: "save plan and proceed".
Defaults accepted: 5 distillation tasks (3 regressors + 2 anchor wins); Run 7 sandbox as Stage-2 starting point.
