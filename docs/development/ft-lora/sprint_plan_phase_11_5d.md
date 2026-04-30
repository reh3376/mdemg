# Sprint Plan — Phase 11.5d: Branch B Revised — Distillation on Real Regressors

**Sprint:** FT-LORA-PHASE11.5d (Branch B Resumed, Retargeted)
**Date:** 2026-04-28
**Branch:** `reh3376_dev01`
**Status:** PLANNED — awaiting approval
**Predecessors:** Phase 11.5c (clean eval + honest re-baseline), Phase 11 RL runs 1-7 (plateaued against leaked golden), original Branch B (paused)
**Successor:** Phase 12 HITL DPO (consumes the distilled adapter as base) OR Phase 11.5e (synthetic prompt augmentation for the 8 data-starved tasks)

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE11.5d |
| Title | Branch B Revised — knowledge distillation on the real regressors (consulting.classify + retrieval.rerank_cross), measured against valid_clean |
| Type | Code-medium (~400-600 LOC: capture driver tweak + SFT config + distill JSONL builder + regression harness adaptation), data-medium (~200 distill pairs), compute-medium (~3-4 hr SFT + 30 min regression) |
| Risk | MEDIUM-HIGH (Risk #1 — consulting.classify reward function may be the limiter, not the model. If gpt-mini also can't beat 0.88 with reward=1.0 on perfect output, the reward function caps every learner.) |
| Budget | OpenAI gpt-5.4-mini distill capture + judge replay: ~$1-3; well under $100 cap |
| Output adapter | `.local-models/qwen3-14b-mdemg-v1-rl/` (sandbox first → promoted on regression PASS); Run 7 retained as `qwen3-14b-mdemg-v1-rl-run7` |
| New TSDB migration | None (V0013 already covers any RL training metadata; SFT distill reuses Phase 5 paths) |
| Post-sprint artifacts | `training_data/distill/phase11_5d/{train.jsonl, valid.jsonl, manifest.json}`; `configs/sft_phase11_5d_distill.yaml`; `training_data/eval/{baseline_run5_clean.json, baseline_run7_clean.json, regression_phase11_5d.json}`; sprint post doc; revised adapter on canonical path |

---

## 2. Problem Statement

Phase 11.5c established that the Phase 5 honest baseline against held-out production data is **0.8052** on `valid_clean.jsonl` (180 rows, 9 of 17 tasks, 0% leakage with training data). gpt-5.4-mini scores **0.8562** on the same eval — **+5.10pp gap**.

Per-task analysis identified two real regressors that were hidden by `valid_golden`'s 99% leakage:

| Task | Group | Phase 5 (clean) | gpt-mini (clean) | Gap | Aggregate ceiling |
|---|---|---|---|---|---|
| `consulting.classify` | C | **0.490** | 0.883 | **+39.3pp** | C weight 0.35 / 5 = +0.07 per pp → **+2.75pp** if closed fully |
| `retrieval.rerank_cross` | J | **0.720** | 0.900 | **+18.0pp** | J weight 0.15 / 4 = +0.0375 per pp → **+0.68pp** if closed fully |

**Combined realistic ceiling: ~+2.0-2.7pp aggregate** if distillation closes 60-80% of the per-task gaps. Target: **valid_clean aggregate ≥ 0.83** (from 0.8052 baseline → +2.5pp).

**Why distillation works here where GRPO failed in Phase 11:**

1. The Phase 11 RL plateau diagnosis identified `retrieval.query_classify` as a regressor — that was a phantom of leaked-golden carving. Real-data score is 0.90. **Phase 11 was retraining the wrong tasks.**
2. SFT distillation puts a *correct* output in the loss directly. For `consulting.classify` (Phase 5: 0.49), the policy is producing wrong outputs — the gradient signal in GRPO collapsed. With SFT teacher (gpt-mini at 0.88), the loss has structure that bootstraps the student.
3. `mlx_lm.lora` is the proven SFT trainer used in Phase 5; reusing it sidesteps the Phase 11 custom-trainer footguns (gradient checkpointing closure-capture, etc.).

**Pre-sprint mandate:** before any training, **re-baseline Run 5 + Run 7 against `valid_clean.jsonl`**. If their golden gains generalize (Run 7 ≥ Phase 5 on clean), Run 7 becomes the distill starting point — saves the +1.76pp golden lift if it's real. If they don't (Run 5/Run 7 ≤ Phase 5 on clean), restart from Phase 5 base, treat the prior runs as memorization-preservation, not generalization gain.

---

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Run 5 + Run 7 clean re-baselines | `training_data/eval/baseline_run5_clean.json`, `baseline_run7_clean.json` |
| 2 | `consulting.classify` reward-function audit | `training_data/eval/consulting_classify_reward_audit.json` (Epic 0 verdict: distillation viable Y/N) |
| 3 | Distill dataset capture driver | `scripts/x9_distill_capture_v2.py` (extends X8/X8b transport with the 2 retargeted tasks) |
| 4 | Distill JSONL train+valid + manifest | `training_data/distill/phase11_5d/{train.jsonl, valid.jsonl, manifest.json}` |
| 5 | Stage-1 SFT config | `configs/sft_phase11_5d_distill.yaml` (lr=1e-5, epochs=2, batch=4, 7 LoRA modules — same as Run 7) |
| 6 | Stage-1 SFT adapter | `.local-models/qwen3-14b-mdemg-v1-distill-sandbox/adapters.safetensors` |
| 7 | Stage-1 regression report | `training_data/eval/regression_phase11_5d_stage1.json` |
| 8 | Conditional Stage-2 GRPO + final regression | (only if Stage-1 misses target) |
| 9 | Promoted adapter | `.local-models/qwen3-14b-mdemg-v1-rl/` (renamed from sandbox on PASS); Run 7 archived to `-rl-run7` |
| 10 | Sprint post-doc | `docs/development/ft-lora/phase_11_5d_post.md` |
| 11 | Doc cascade | `00_README_v2.md` v5.9 → v5.10; CHANGELOG; AGENT_HANDOFF top entry; CLAUDE.md Testing section |

**Out of scope:**
- Synthetic prompt augmentation for the 8 data-starved tasks (defer to Phase 11.5e)
- Fixing `jiminy.evaluate` / `jiminy.evaluate_llm` task-name mislabeling in production logging (separate Go-code change)
- Phase 12 HITL DPO (consumes this sprint's adapter as base)
- Re-running Phase 5 SFT with task-balanced sampling (`Path B` from prior discussion — deferred unless Stage-1 fails dramatically)

**Hard constraints (MEMORY):**
- **Epoch cap = 3** on all LoRA runs (Stage-1 SFT uses `epochs: 2` to leave headroom for early-stop)
- **Early-stop:** SFT path → `val_loss > best × 1.05` for 2 consecutive evals
- **No hardcoded values** — lr, batch, epochs, LoRA modules all in `configs/sft_phase11_5d_distill.yaml` with CLI overrides
- **CUIDv2** for run_ids
- **Sequential epics** — no parallel epic execution; docs before implementation within each epic
- **3-tier testing** — unit (distill-dataset construction), integration (mocked OpenAI capture + mocked mlx_lm.lora invocation), e2e (real Phase 5 base + real OpenAI + real regression on valid_clean)
- **min `max_tokens` ≥ 3000, min `latency_budget_ms` ≥ 15000** (inherited from Phase 10 `llm_judge.py`)
- **Plan-options pattern** — Stage-2 GRPO start point + reward-function decision both disclosed at PR
- **Single batched commit** at sprint close
- **Sprint summary on PR comments** immediately after push
- **Base model + Phase 5 adapter read-only** — `.local-models/qwen3-14b-mdemg-v1/` never overwritten
- **Run 7 sandbox preserved** — archived to `-rl-run7` before any new sandbox is created
- **MLX single-instance** preflight on `127.0.0.1:8101`
- **No `mx.set_wired_limit`** call (memory: kernel-panic footgun documented)
- **gradient checkpointing**: pass `model.trainable_parameters()` as explicit input, not closure-captured (memory: silent zero-gradient footgun)

---

## 4. Dependencies

**Consumed (code, pre-existing):**
- `mlx_lm.lora` CLI (proven Phase 5 SFT trainer) — reused as-is
- `neural/benchmarks/run_benchmark.py` (Phase 10 runner) — reused unchanged for all baselines + regressions
- `neural/training/rl/regression.py` (Phase 11 dual gate) — reused with `--golden valid_clean.jsonl` override
- `scripts/x7_gpt54mini_benchmark.py` (OpenAI HTTP transport) — port for Epic 1 capture
- `scripts/build_clean_eval.py` + `audit_eval_leakage.py` (from 11.5c) — sanity check distill dataset doesn't leak into valid_clean
- `.local-models/qwen3-14b-mdemg-v1/` (Phase 5 dense base) — distillation starting point (or Run 7 if pre-flight finds Run 7 generalizes)
- `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/` (Run 7, 514MB, 7 LoRA modules) — candidate alternative starting point
- `training_data/eval/valid_clean.jsonl` (180 rows, 9 tasks, leak-free) — primary eval
- `training_data/eval/baseline_phase5_clean.json` (0.8052) — regression target

**Consumed (data, leakage check sources — must NOT overlap with distill train set):**
- All 9 sources in `LEAKAGE_SOURCES` from `scripts/build_clean_eval.py` (tier1 + family train/valid + valid_golden) **PLUS** `valid_clean.jsonl` itself

**Consumed (compute):**
- Apple Silicon ≥48GB unified memory (Phase 5/Run 7 dense fits)
- OpenAI API — gpt-5.4-mini distill capture: ~200 calls × $0.005 = ~$1
- Local MLX inference for Run 5/Run 7 baselines + Stage-1/Stage-2 regression: ~1.5 hr wall-clock total

**External services:**
- Local MLX server `:8101` (single-instance)
- OpenAI API (capture + regression judge replay; never during RL step loop if Stage-2 runs)
- TSDB read-only (no schema changes)

---

## 5. Implementation Plan (Sequential Epics + Gates)

### Epic 0 — Pre-flight + Run 5/7 Re-baseline + Reward-Function Audit (≈2 hr)

1. **Verify environment**: 109 RL/DPO unit tests still green; `valid_clean.jsonl` SHA matches manifest; MLX single-instance check; venv active (`mdemg-ft-lora`).
2. **Re-baseline Run 5 against `valid_clean.jsonl`**:
   - Locate Run 5 sandbox adapter (per Phase 11 chronicle in `phase_11_mlx_adapter.md`). If overwritten, document and skip — treat Run 5 as unreachable.
   - Merge onto fresh Phase 5 dense → temporary path; benchmark via `run_benchmark.py --golden valid_clean.jsonl --mlx-timeout-s 300`.
   - Save `baseline_run5_clean.json`.
3. **Re-baseline Run 7 against `valid_clean.jsonl`**:
   - `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/` is intact (Phase 11.5c Epic 0 verified, 514MB, 7 modules).
   - Merge onto fresh Phase 5 dense; benchmark on valid_clean; save `baseline_run7_clean.json`.
4. **Compare**: build mini-table comparing Phase 5 / Run 5 / Run 7 / gpt-mini on valid_clean. Decide Stage-1 distillation starting point:
   - If `Run 7 clean ≥ Phase 5 clean` AND no per-task regression > 2pp: **start from Run 7** (preserves +Xpp lift)
   - Else: **start from Phase 5 base**, treat Phase 11 RL gains as memorization
5. **Audit `consulting.classify` reward function**: investigate why gpt-mini caps at 0.88 (not 1.0). Read the reward function in `neural/training/reward_functions.py` for this task. Verify the reward function correctly scores gpt-mini's outputs on a sample of 5-10 production rows. Possible outcomes:
   - **Reward function correct** (gpt-mini's 0.88 reflects honest task difficulty / noise in labels): proceed with distillation; expect ~0.80-0.85 ceiling for Phase 5 after distill (reaching gpt-mini parity)
   - **Reward function buggy** (some valid outputs scored as 0): fix the reward function FIRST; re-run Phase 5 baseline; possibly the gap closes without distillation
   - **Reward function too strict** (gpt-mini hits an artificial ceiling): document; distillation still useful but ceiling is at gpt-mini's 0.88, not 1.0
   Save audit verdict in `training_data/eval/consulting_classify_reward_audit.json`.
6. **Plan-options decision points** (recommendations + rationale; final pick at execution):
   - **Distillation starting point** (Phase 5 base vs Run 7 sandbox) — driven by step 4 outcome
   - **Distillation task set** — recommend `consulting.classify` (primary) + `retrieval.rerank_cross` (secondary). Optional include `jiminy.synthesize` (-8.7pp on clean) as third — but Run 5 was ahead of gpt-mini on this task per X7 data, so distillation might regress it. Default: SKIP it.
   - **Reward-function-first vs distillation-first** — driven by step 5 outcome

**Gate:** Run 5/Run 7 baselines complete OR documented unreachable; reward audit verdict written; distillation starting point + task set decided.

### Epic 1 — Capture Distillation Dataset (≈1.5 hr, $1-3 OpenAI)

1. **New driver `scripts/x9_distill_capture_v2.py`** — extends X8b's OpenAI HTTP transport with:
   - Per-task production-row sourcing (uses `build_clean_eval.py`'s leak-filter logic)
   - Captures `{prompt, response_text, task_id, golden_row_idx, sampling_group, reward_score}` per call
   - Streams to `training_data/distill/phase11_5d/raw_responses.jsonl`
2. **Capture targets** (per Epic 0 decision):
   - `consulting.classify`: pull 50 user_prompts from production TSDB that are NOT in any train/valid OR valid_clean source. Sample gpt-mini at temp=0.7 (C-group). 50 calls.
   - `retrieval.rerank_cross`: pull 50 user_prompts similarly. Sample gpt-mini at temp=0.0 (J-group, deterministic). 50 calls.
   - Total: **100 calls × ~$0.01 = ~$1.00**.
3. **Filter**: keep only responses where `compute_reward(response)` ≥ 0.8 (high-quality teacher signal). Document drop rate in manifest. Adaptive lower threshold (0.7) if drop rate > 50%.
4. **Convert to mlx_lm.lora chat-format JSONL**: `{"messages":[{"role":"system","content":<system_prompt>},{"role":"user","content":<user>},{"role":"assistant","content":<gpt-mini response>}]}`. Use the **current production system prompt** (per Phase 11.5c spec patches) for each task.
5. **Train/valid split**: 90/10, stratified by task. Output: `training_data/distill/phase11_5d/{train.jsonl, valid.jsonl}`.
6. **Manifest**: per-task pair count, reward distribution, total tokens, OpenAI spend, source SHA256s, leak-audit verification (using `audit_eval_leakage.py` against `valid_clean.jsonl` + 9 train/valid sources). Saved as `manifest.json`.
7. **Leak audit gate**: `python scripts/audit_eval_leakage.py --eval training_data/distill/phase11_5d/train.jsonl --against <10 sources including valid_clean.jsonl>` MUST exit 0 (zero overlap).

**Gate:** ≥40 pairs/task × 2 tasks = ≥80 pairs; leak audit zero overlap; manifest written; spend logged.

### Epic 2 — Stage-1 SFT Training via mlx_lm.lora (≈3-4 hr)

1. **New config `configs/sft_phase11_5d_distill.yaml`**:
   ```yaml
   model: ".local-models/qwen3-14b-mdemg-v1"  # OR Run 7 sandbox per Epic 0 decision
   train: true
   data: "training_data/distill/phase11_5d/"
   lora_layers: 40
   lora_parameters:
     rank: 32
     scale: 2.0
     dropout: 0.05
     keys:  # 7 modules — match Run 7
       - "self_attn.q_proj"
       - "self_attn.k_proj"
       - "self_attn.v_proj"
       - "self_attn.o_proj"
       - "mlp.gate_proj"
       - "mlp.up_proj"
       - "mlp.down_proj"
   batch_size: 4
   iters: <derived: ceil(90 train pairs / batch=4 × epochs=2) ≈ 45>
   learning_rate: 1.0e-5
   max_seq_length: 4096
   adapter_path: ".local-models/qwen3-14b-mdemg-v1-distill-sandbox"
   save_every: 25
   steps_per_eval: 10
   ```
2. **Invoke**: `python -m mlx_lm lora --config configs/sft_phase11_5d_distill.yaml --train`.
3. **Monitor**: train_loss + val_loss every 10 steps; early-stop manual kill if `val_loss > best × 1.05` two consecutive evals.
4. **Output**: `.local-models/qwen3-14b-mdemg-v1-distill-sandbox/adapters.safetensors`.

**Gate:** training completes within 2 epochs (≤45 iters); final val_loss < initial × 0.5 (sanity: actually learned); adapter file present, ~514MB.

### Epic 3 — Stage-1 Regression Gate against `valid_clean` (≈45 min, ~$0.10 OpenAI)

1. Merge Stage-1 adapter onto fresh Phase 5 base → `.local-models/qwen3-14b-mdemg-v1-distill-fresh-merge/`.
2. Run Phase 10 benchmark on Stage-1 (sandbox path, NOT canonical) against `valid_clean.jsonl`:
   ```bash
   python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml \
     --golden training_data/eval/valid_clean.jsonl \
     --mlx-base-url http://127.0.0.1:8101/v1 \
     --mlx-model-name <abs path to merged Stage-1> \
     --mlx-timeout-s 300 \
     --out training_data/eval/regression_phase11_5d_stage1_5a.json
   ```
3. Run on fresh-merge path against `valid_clean.jsonl`:
   ```bash
   --out training_data/eval/regression_phase11_5d_stage1_5b.json
   ```
4. **Gate 5a (vs valid_clean baseline 0.8052)**:
   - PASS condition: aggregate ≥ 0.83 (= 0.8052 + 2.5pp) AND no per-task regression > 2pp
   - Per-task target: `consulting.classify` lifted from 0.49 toward gpt-mini's 0.88 (target ≥ 0.70 is meaningful improvement); `retrieval.rerank_cross` lifted from 0.72 toward 0.90 (target ≥ 0.80)
5. **Gate 5b (vs fresh-merge — sandbox vs fresh-merge delta ≤ 0.5pp)**: catches adapter-merge corruption.
6. Save dual report `training_data/eval/regression_phase11_5d_stage1.json`.

**Gate verdict:**
- **PASS at 0.83+ aggregate** → skip Stage 2, proceed to Epic 6 (promotion)
- **PASS-but-improving (e.g., 0.81-0.82)** → proceed to Epic 4 (Stage 2 GRPO)
- **FAIL (≤ 0.8052 OR per-task regression > 2pp)** → abort, document; Phase 11.5e investigates reward function or alternative approach

### Epic 4 — Stage-2 GRPO (CONDITIONAL, ≈4-6 hr) — only if Epic 3 misses 0.83 target

1. Reuse `configs/rl_phase11.yaml` with `starting_adapter` = Stage-1 sandbox (NOT Run 7).
2. Restrict GRPO updates to the 2 distill tasks if Stage-1 already saturated others. CLI `--task-filter consulting.classify,retrieval.rerank_cross`.
3. lr=2e-6, kl_coef=0.05 (Run 5 hyperparams that didn't exhibit instability), max_steps=300.
4. Same memory hygiene: `mx.clear_cache()` per step, combined `mx.eval` barrier, no `set_wired_limit`.
5. Output: `.local-models/qwen3-14b-mdemg-v1-distill-stage2-sandbox/`.

**Gate:** Stage-2 completes; checkpoint written.

### Epic 5 — Final Regression (CONDITIONAL — follows Epic 4)

Same harness as Epic 3 against Stage-2 sandbox + fresh-merge. Same pass criteria.

### Epic 6 — Adapter Promotion

On PASS (Stage-1 OR Stage-2):
1. Archive Run 7: `mv .local-models/qwen3-14b-mdemg-v1-rl-sandbox/ .local-models/qwen3-14b-mdemg-v1-rl-run7/`
2. Promote winning sandbox: `mv .local-models/qwen3-14b-mdemg-v1-distill-sandbox/ .local-models/qwen3-14b-mdemg-v1-rl/` (canonical path)
3. Update `manifest.json` with config SHA, parent (Phase 5 base SHA), distill manifest SHA, regression report SHA, both Run 7 archive + Stage-1/Stage-2 lineage.
4. **CMS-pinned port unchanged**: `:8101`. Operator restarts mlx_lm.server with new adapter when ready.

### Epic 7 — Documentation (Final Epic — Never Cut)

1. `docs/development/ft-lora/phase_11_5d_post.md` — executed-truth: Run 5/7 clean baselines, reward audit verdict, distill stats, Stage-1 verdict, Stage-2 verdict (if run), per-task delta table on valid_clean, OpenAI spend, plan-options decisions.
2. `00_README_v2.md` v5.9 → v5.10: Phase 11.5d EXECUTED.
3. `04_BENCHMARK_RL_v2.md` — distillation-on-real-regressors path documented as the canonical post-Phase-5 sprint pattern.
4. `AGENT_HANDOFF.md` top entry; `CHANGELOG.md [Unreleased] ### Added`.
5. `CLAUDE.md` Testing section — add SFT-distill-on-clean-eval invocation if not already present.

**Gate:** all docs committed; cross-refs valid; `grep -r "Phase 11.5d.*pending\|Phase 11.5d.*planned" docs/development/ft-lora/` returns zero hits.

---

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit):**
- `tests/scripts/test_x9_distill_capture.py` — mocked OpenAI; verify response_text persistence, reward filter, JSONL chat-format conversion, train/valid split stratification.
- Reuse `tests/scripts/test_audit_eval_leakage.py` from 11.5c.

**Tier 2 (Integration):**
- `mlx_lm.lora` invocation against tiny stub dataset (10 pairs); verify adapter file written, val_loss recorded.
- Regression harness against canned Stage-1 adapter (mocked benchmark scores) — verifies pass/fail routing on `valid_clean`.

**Tier 3 (E2E):**
- Smoke (3 distillation tasks × 3 prompts × 3 samples = 27 OpenAI calls, ~$0.30): validate full pipeline.
- Full Epic 1 capture, Epic 2 SFT, Epic 3 regression — end-to-end on real Phase 5/Run 7 base.

---

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ft-lora): Phase 11.5d — Branch B revised — distillation on real regressors (consulting.classify + retrieval.rerank_cross)`
- Body: motivation (11.5c findings → retargeted distill set), Run 5/7 clean baseline summary, Epic 0 reward-audit verdict, distillation pair counts, Stage-1 + Stage-2 (if run) verdicts, per-task delta on valid_clean, OpenAI spend, plan-options disclosed (start point, distill task set, reward-function path), policy-compliance checklist (epoch cap, early-stop, no hardcoded values, CUIDv2, sequential epics, 3-tier testing, gradient-checkpoint pattern correct, no set_wired_limit).
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push → auto-PR → sprint summary on PR comments immediately.

---

## 8. Verification Checklist

- [ ] Epic 0: Run 5 + Run 7 baselines complete (or documented unreachable); reward audit verdict written; distillation start point + task set + reward path decided
- [ ] Epic 1: ≥40 pairs/task × 2 tasks; leak audit zero overlap; manifest with SHAs; OpenAI spend ≤ $3
- [ ] Epic 2: SFT completes within 2 epochs; val_loss converges; sandbox adapter written
- [ ] Epic 3: regression aggregate ≥ 0.83 OR fail-improving (Stage 2 path) OR clean fail (abort)
- [ ] Epic 4 (cond): Stage-2 GRPO completes
- [ ] Epic 5 (cond): final regression passes
- [ ] Epic 6: Run 7 archived; winning sandbox promoted to canonical path; manifest updated
- [ ] Epic 7: all docs committed
- [ ] Single commit pushed; auto-PR opened; sprint summary on PR
- [ ] OpenAI spend logged, under $100 cap

---

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 7: `phase_11_5d_post.md`, `00_README v5.10`, `04_BENCHMARK_RL_v2.md` distillation pattern, AGENT_HANDOFF top entry, CHANGELOG, CLAUDE.md Testing section.

---

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | **`consulting.classify` reward function caps gpt-mini at 0.88** — may be a measurement bug rather than honest difficulty | Medium-High | Epic 0 reward audit verifies gpt-mini outputs are correct; if reward function is buggy, FIX it first → re-baseline → re-evaluate distill ROI | Skip distillation; ship reward-function fix as Phase 11.5d output instead |
| 2 | Run 7 clean baseline ≤ Phase 5 clean (Phase 11 RL gains were memorization-only) | Medium | Pre-flight detects this in Epic 0; pivot start point to Phase 5 base; document Phase 11 RL as null-result | Continue with Phase 5 base; preserve Run 7 for archive |
| 3 | Distillation regresses anchor tasks (e.g., `ape.reflect`, where Phase 5 ≈ gpt-mini) | Medium | Per-task gate at ≤ 2pp regression; manual review of training samples; eval at smaller batch sizes during SFT to spot regression early | Reduce SFT epochs; or include anchor tasks as eval-only in Stage-1 |
| 4 | gpt-mini responses fail reward filter (drop rate > 50%) | Low–Med | Filter threshold configurable (0.8 → 0.7 fallback); X7 already showed 0.88 mean for consulting.classify so most score well | Lower filter to 0.7; document compromise |
| 5 | mlx_lm.lora overfits with 100-200 distill pairs | Medium | epochs=2 cap, early-stop on val_loss, batch=4 (regularizes), 10/90 valid split | Reduce to 1 epoch; or augment with Phase 5 SFT tail |
| 6 | OpenAI spend > $5 (Epic 1 retries) | Low | max_completion_tokens=3000 cap; deterministic temp=0 for J-group; one retry per call | Halt at $4 mark, ship with reduced distill set |
| 7 | Adapter merge corruption (5b fails) | Low | Same dual-merge pattern from Phase 11 regression; pin mlx version | Re-merge with bf16-explicit; document |
| 8 | Stage-1 falls below 0.83 target → Stage-2 GRPO ≤ 4-6 hr → over budget | Medium | Plan-options at PR — accept Stage-1 result if ≥ 0.81 (still +1pp aggregate) | Document gap; Phase 11.5e investigates remaining tasks |
| 9 | Distill prompts overlap with valid_clean.jsonl (re-introduces leakage) | Low | Epic 1 leak-audit gate against `valid_clean.jsonl` + 9 train/valid sources; abort if overlap | Re-extract distill prompts excluding valid_clean's user_prompts |
| 10 | Run 5 sandbox unreachable (overwritten by later runs) | High (likely) | Document as unreachable in Epic 0; baseline Run 7 alone | Run 7 represents the latest RL attempt; sufficient signal |
| 11 | gradient checkpointing closure-capture footgun reintroduced | Low (memory documented) | Use `mlx_lm.lora` CLI directly (proven path); avoid custom trainer entirely for SFT | If `mlx_lm.lora` fails, fall back to mlx_lm.lora's library API and follow the documented pattern |
| 12 | MLX timeout on long production prompts (Phase 11.5c hit this with 60s default) | Low | Use `--mlx-timeout-s 300` for all valid_clean evals (precedent set in 11.5c) | Bump to 600s if specific prompts still time out |

---

## 11. Documents Accessed (during planning)

- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_5c_post.md` (immediate predecessor)
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5c.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/branch_b_implementation_plan.md` (paused, replaced by this plan)
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5.md` (X1-X7 diagnostics)
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_mlx_adapter.md` (Run 1-7 chronicle)
- `/Users/reh3376/mdemg/training_data/eval/clean_vs_golden_delta.md` (per-task analysis)
- `/Users/reh3376/mdemg/training_data/eval/baseline_phase5_clean.json` (target floor 0.8052)
- `/Users/reh3376/mdemg/training_data/eval/baseline_gpt54mini_clean.json` (target ceiling 0.8562)
- `/Users/reh3376/mdemg/training_data/eval/valid_clean.jsonl` + manifest (eval set)
- `/Users/reh3376/mdemg/scripts/build_clean_eval.py` + `audit_eval_leakage.py` + `x7_gpt54mini_benchmark.py` + `x8b_gpt54mini_clean.py`
- `/Users/reh3376/mdemg/configs/rl_phase11.yaml` (template for Stage-2)
- `/Users/reh3376/mdemg/neural/training/reward_functions.py` (consulting.classify scoring — Epic 0 audit target)
- `/Users/reh3376/mdemg/.local-models/qwen3-14b-mdemg-v1-rl-sandbox/` (Run 7 candidate start point)
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `feedback_mlx_set_wired_limit_footgun.md`, `feedback_mlx_checkpoint_closure_footgun.md`, `project_phase5_moe_pivot.md`

---

## 12. Rollback

All changes additive.

1. `git revert <final commit SHA>`
2. `rm -rf .local-models/qwen3-14b-mdemg-v1-distill-sandbox/ .local-models/qwen3-14b-mdemg-v1-distill-stage2-sandbox/ .local-models/qwen3-14b-mdemg-v1-distill-fresh-merge/ training_data/distill/phase11_5d/ configs/sft_phase11_5d_distill.yaml scripts/x9_distill_capture_v2.py training_data/eval/baseline_run5_clean.json training_data/eval/baseline_run7_clean.json training_data/eval/regression_phase11_5d*.json training_data/eval/consulting_classify_reward_audit.json`
3. If Epic 6 ran: restore Run 7 from archive (`mv .local-models/qwen3-14b-mdemg-v1-rl-run7 .local-models/qwen3-14b-mdemg-v1-rl-sandbox`); delete promoted canonical path
4. Phase 5 + Phase 10 + Phase 11 (Run 7) artifacts untouched throughout; valid_clean.jsonl, valid_golden.jsonl, all spec hashes untouched

No TSDB writes (Epic 1 captures to JSONL only). No Neo4j writes.

---

## Time + Budget Projection

| Path | Wall-clock | OpenAI $ |
|---|---|---|
| Best case (Stage-1 hits 0.83+ on first try) | ~6-8 hr | ~$2 |
| Typical (Stage-1 at 0.81-0.82, Stage-2 closes gap) | ~12-14 hr | ~$4 |
| Reward-audit reveals broken reward function (skip distill) | ~3 hr | ~$0.50 |
| Worst case (Stage-1 + Stage-2 + remediation epic) | ~22-24 hr | ~$6 |

All paths well within MEMORY $100 cap.

---

## Open Decision Points (Plan-Options at Execution)

1. **Distillation starting point**: Phase 5 base vs Run 7 sandbox — Epic 0 results decide
2. **Distillation task set**: 2 tasks (default) vs 3 (add `jiminy.synthesize`) vs 1 (consulting.classify only if Run 7 already strong on rerank_cross)
3. **Reward-function-first vs distillation-first**: Epic 0 audit decides
4. **Stage-2 GRPO yes/no**: Epic 3 verdict decides (PASS=skip, fail-improving=run)
5. **Final commit prompt**: PR auto-opens; sprint summary comment template ready
