# Phase 5 SFT — Executive Summary

**Sprint:** FT-LORA-PHASE5
**Period:** 2026-04-22 → 2026-04-23
**Branch:** `reh3376_dev01`
**Status:** ✅ Complete — both regression gates PASS
**Companion docs:**
[`phase_5_sft_post.md`](phase_5_sft_post.md) (full post-run report, 10 sections)
[`adapters/tier1/manifest.json`](../../../adapters/tier1/manifest.json) (SHA-pinned run metadata)

---

## 1. Process

**What we set out to do.** Fine-tune a local MDEMG universal LoRA adapter against an MoE base model (Qwen3.6-35B-A3B-mxfp4) using the two-tier MoE-Sieve strategy locked in memo 07 v3.1 — Tier 1 universal attention + shared expert, then 3× Tier 2 family adapters keyed on Sprint D expert routing profiles.

**What actually executed.** Mid-sprint pivot on 2026-04-22 to a **dense Qwen3-14B-4bit** single-tier path. Everything downstream of the base-model swap held: the same tier1 dataset (3,150 train + 350 valid, SHA-pinned), same `evaluate_ft.py` + `regression_gate.py`, same epoch cap + early-stop policy, same 16-task ULTS eval.

**The executed pipeline:**

1. **Pre-flight** — env verification, single-MLX-instance enforcement on `:8101`, base model SHA pin (`a54ec18f…`), dataset SHA pins (train `d739a69c…` / valid `031d1c08…` / raw `7caebf75…`).
2. **Baseline capture (MoE-35B)** — 0.9805 weighted, 15/16 tasks passing. Captured before pivot.
3. **Pivot decision** — MoE failed Metal 499K MTLResource ceiling on every non-trivial backward pass (§2 below). Dense Qwen3-14B-4bit selected as final base.
4. **Training run** — `neural/training/train_ft.py --tier 1 --mode sft` with `--target-modules` override to drop `shared_expert` (no MoE shared expert on dense). Ran 9h 7m → early-stop at Iter 3000 → active adapter restored to Iter 2400 best (val_loss 0.246).
5. **Baseline capture (dense-14B)** — 0.9505 weighted, 15/16 tasks. Fresh untuned control for clean training-gain measurement.
6. **Merge** — `mlx_lm fuse` folds LoRA Iter 2400 into base → `.local-models/qwen3-14b-mdemg-v1/` (7.8 GB, 4-bit preserved).
7. **Post-tune eval** — 0.9856 weighted, **16/16 tasks passing**.
8. **Dual regression gate** — both PASS, zero regressions.

**Why a dual gate.** The pivot introduced two confounded variables (architecture change + training gain). User decision (2026-04-23) was to gate against both the original MoE baseline *and* a fresh dense baseline — the first to prove the pivot preserved the plan's intent, the second to prove a clean training signal.

---

## 2. Findings

### Finding 1 — The Metal 499K ceiling is architectural, not quant-specific

| Config | mxfp4 | standard q4 |
|---|---|---|
| 40L × 7 keys × 8192 seq | ❌ | ❌ |
| 40L × 4 keys × 8192 seq | ❌ | — |
| 20L × 7 keys × 8192 seq | ❌ | — |
| 40L × 7 keys × 6144 seq | ❌ | — |
| 40L × 7 keys × 7168 seq | ❌ | — |
| 8L × 2 keys × 2048 seq (toy) | ✅ | — |

Requantization to standard 4-bit did not help. macOS 26 removed `iogpu.rsrc_limit` — no user-space tunable exists. **Root cause:** 256 experts × top-8 routing × 40 layers generates more Metal command-buffer objects than the 499K ceiling permits on the backward pass.

### Finding 2 — Dense 14B beats untuned MoE 35B after fine-tune

Fine-tuned dense 14B (**0.9856**) outperforms untuned MoE 35B (**0.9805**) at 40% the parameter count on the 350-record × 16-task eval. The training gain plus task-specific adaptation compensates entirely for the architectural downgrade on this workload.

### Finding 3 — Val-loss oscillation is endemic, not pathological

The run had three single-strike spikes (Iter 1800, 2200, 2800 entry) that self-recovered, then one clean two-strike divergence at Iter 2800→3000 that triggered early-stop exactly as policy designed. Without `patience=2` the run would have stopped 1200 iters earlier on a false signal and missed the Iter 2400 minimum.

### Finding 4 — Peak memory was 92 GB below budget

Dense 14B peaked at **36.03 GB** on the 128 GB M5 Max — vs 105–115 GB projected for MoE. Leaves headroom for concurrent tasks during training, or for pushing batch_size / seq_length higher on the next run.

### Finding 5 — Namespace collision with CLI binary

The original plan called for merged-model output at `mdemg/qwen3-14b-mdemg-v1/`, but `/Users/reh3376/mdemg/mdemg` is the 50 MB MDEMG CLI binary (same path). Output landed at `.local-models/qwen3-14b-mdemg-v1/` (gitignored; 7.8 GB exceeds GitHub's 100 MB/file limit anyway).

---

## 3. Current State

### Models on disk (as of 2026-04-23)

| Model | Path | Size | Purpose |
|---|---|---|---|
| **Baseline** (dense, untuned) | `~/hf_cache/hub/models--mlx-community--Qwen3-14B-4bit/…` | 7.8 GB | Trained-against base; reproducibility |
| **Active LoRA** (Iter 2400 best) | `adapters/tier1/adapters.safetensors` | 514 MB | Applied on top of base at inference time |
| **Merged** (LoRA folded in) | `.local-models/qwen3-14b-mdemg-v1/` | 7.8 GB | Standalone deliverable; Phase 10 consumer |

### Adapter artifacts tracked in repo

- `adapters/tier1/manifest.json` — full run metadata (SHAs, hyperparameters, val-loss history, eval + regression results)
- `adapters/tier1/{adapter_config,train_config,train_report,.earlystop}.{json,yaml}` — mlx_lm configs + orchestrator report + early-stop sidecar
- `training_data/eval/{baseline_qwen3.6_untuned,baseline_qwen3_14b_untuned,post_tier1}.json` — three eval reports
- `training_data/eval/regression_tier1_vs_{moe35b,dense14b}.json` — both gate verdicts

### Env & code reflecting the pivot

- `.env.example` + `.env` — `MLX_BASE_MODEL`, `MLX_BASE_MODEL_PATH`, `MLX_BASE_MODEL_CONFIG_SHA256`, `MLX_MERGED_MODEL`, `MLX_MERGED_MODEL_PATH` added. MoE-only knobs (`ASYMMETRIC_QUANT_*`, `ROUTER_AUX_LOSS_COEF`) annotated as no-ops on dense path.
- `neural/training/{train_ft,evaluate_ft,teacher_distill,distill_driver,quantize_deploy}.py` — docstring examples and CLI defaults refreshed to dense Qwen3-14B-4bit at port 8101.
- `neural/training/profile_expert_routing.py` — status banner marks MoE-Sieve path abandoned; Sprint D artifacts retained for archival only.
- `scripts/{sprint_e_e2e_dry_run.sh, test_vllm_mlx.py}` — defaults swapped to dense model + port 8101.

### Policies honored

- Epoch cap 3 (no `n_epochs=auto`)
- Early-stop `val_loss > best × 1.05` × 2 evals (fired exactly as designed)
- No hardcoded values (LR/batch/seed/eval-interval all CLI-driven)
- Single-instance MLX on `:8101`
- Base model read-only; merge wrote to a separate namespace
- SHA pins asserted pre-launch; sequential epics; single batched commit at sprint close

---

## 4. Testing & Benchmarking Results

### Evaluation (same set, 350 records × 16 tasks, same ULTS specs, same scoring rubric)

| Model | Weighted score | Tasks passing ≥80% | Notable per-task |
|---|---|---|---|
| Qwen3.6-35B-A3B-mxfp4 (MoE, untuned) | 0.9805 | 15/16 | `guardrail.evaluate` has no test data |
| Qwen3-14B-4bit (dense, untuned) | 0.9505 | 15/16 | weakest: `ape.reflect` 0.740 |
| **qwen3-14b-mdemg-v1 (dense, fine-tuned)** | **0.9856** | **16/16** | `ape.reflect` → 1.000; 4 tasks lifted ≥2% |

### Gate 4a — vs MoE-35B baseline

```
Verdict: PASS
Overall: 0.9856 vs 0.9805  (Δ +0.0051)
Regressions (≥5% drop): 0
Improvements (≥2% lift):
  metalearn.generalize       0.9500 → 1.0000  (+5.3%)
  retrieval.rerank_cross     0.8300 → 0.9000  (+8.4%)
```

Confounded (architecture change + training gain). PASS proves the pivot preserved the plan's intent.

### Gate 4b — vs Dense-14B baseline

```
Verdict: PASS
Overall: 0.9856 vs 0.9505  (Δ +0.0351)
Regressions (≥5% drop): 0
Improvements (≥2% lift):
  ape.reflect                0.7400 → 1.0000  (+35.1%)
  consulting.classify        0.9000 → 1.0000  (+11.1%)
  hidden.reclassify          0.8500 → 1.0000  (+17.6%)
  retrieval.rerank_cross     0.8500 → 0.9000  (+5.9%)
```

Clean (same architecture, training gain only). `ape.reflect` lift (+35 pts) is the standout — that was the weakest untuned task and moved to a perfect score.

### Pre-existing unit + integration tests (Sprint E)

- `test_train_ft.py` (38 tests) + `test_expert_selection.py` (18) + `test_quantize_asymmetric.py` (15) + `test_early_stop.py` (18) + `test_evaluate_ft.py` + `test_regression_gate.py`
- `test_train_ft_integration.py` (6 tests, gated on SPRINT_E_HEAVY_INTEGRATION=1)
- `scripts/sprint_e_e2e_dry_run.sh` (5-step invocation matrix, all exit 0)

Total Sprint-E suite: **94 passed + 1 skipped**. Phase 5 re-validated this suite before launch.

### Training run itself as E2E test (Tier 3)

The 9h 7m run with clean early-stop fire + dual regression PASS *is* the E2E test. Exit code −15 (SIGTERM from early-stop monitor); no exceptions, no NaN loss, no Metal OOM, no checkpoint corruption. Manifest SHAs reproduce deterministically from pinned inputs.

---

## 5. Risks & Opportunities

### Risks carried forward

| # | Risk | Likelihood | Severity | Mitigation |
|---|------|---|---|---|
| R1 | **Single-model dependency** — the fine-tuned MDEMG model is a single 7.8 GB artifact stored only in `.local-models/` (gitignored). Accidental deletion destroys 9h of training. | Low-Medium | High | Manifest SHAs are authoritative; retraining is deterministic from pinned dataset + base SHA. Consider backup to object storage (R4 below). |
| R2 | **Val-loss divergence after Iter 2600 hints at overfitting onset** — Iter 2800 + 3000 spikes weren't noise; they're the beginning of the overfit curve. If a future dataset expansion relaxes the early-stop policy, we'll miss the same signal. | Medium | Medium | Epoch cap 3 + early-stop policies are durable MEMORY rules. Future sprints must not loosen them without new forcing-function evidence. |
| R3 | **MoE path is dead for all practical purposes.** If Apple reinstates `iogpu.rsrc_limit` or MLX ships a reduced-object MoE backward kernel, reviving the path requires revalidating Sprints C/D/E against whatever Qwen3.6 revision is current then. Expect 2–4 weeks of rebuild work. | Low | Low | Document status clearly (done — `profile_expert_routing.py` banner). No further investment unless kernel upstream changes. |
| R4 | **No offsite backup of trained artifacts.** Merged model + adapter SHAs live on one M5 Max. Disk failure = full retrain. | Medium | Medium | Consider: (a) rsync to secondary volume, (b) snapshot to external NVMe after each sprint, (c) upload to private HF repo with access gate. **Not yet implemented.** |
| R5 | **The 14B dense model gives up ~20% nominal reasoning headroom vs 35B MoE.** On tasks the eval set doesn't stress (long-horizon multi-hop, open-ended synthesis), the production delta may be larger than the 0.51-pt test-set lift suggests. | Medium | Medium | Phase 10 benchmark will surface this on the 120-question set. Decision on whether to invest in a 32B-class dense alternative (e.g., Qwen2.5-32B-Instruct-4bit) deferred to Phase 10 findings. |
| R6 | **Metal peak was 36 GB vs 92 GB headroom** — we may be *under*-training. LoRA rank=32 at batch=1 / seq=8192 is conservative given the actual memory envelope. We left perf on the table. | Low | Low-Medium | Future runs: explore rank=64, batch=2, or larger seq → new val-loss curve. |
| R7 | **14.5 GB of intermediate checkpoints + ~20 GB of archived failed MoE attempts still on disk.** Not a correctness risk; storage hygiene risk. | Low | Low | Deletion plan proposed and pending user confirmation. |
| R8 | **Eval set size (350 records / ~20 per task) is small.** `guardrail.evaluate` has zero test data. Per-task scores carry high variance — a +5.9% lift on rerank_cross means +1–2 rows. | Medium | Low-Medium | Phase 10 benchmark expansion (120-question set) is the mitigation. Until then, read deltas with conservative confidence intervals. |

### Opportunities unblocked

| # | Opportunity | Action |
|---|------|-----|
| O1 | **Phase 10 benchmark is fully unblocked.** The fine-tuned model is the consumer Phase 10 needs. RLVR-ready reward signal from Phase 10 feeds Sprint F (GRPO/DPO). | Stand up Phase 10 authoring next. Existing 120-question set + `evaluate_ft.py` + regression_gate harness transfer cleanly. |
| O2 | **The dense pivot is a reusable template.** We now have a proven recipe for (dataset + ULTS + regression gate + early-stop) → fine-tuned small dense model. Future tasks that don't need MoE scale can follow this exact path in ~9h per run. | Document the template as a reusable sprint-plan skeleton if a second fine-tune comes up. |
| O3 | **Headroom for a second LoRA pass.** 92 GB free during training means we could train a second adapter (e.g., a RAG-focused specialist) on the same hardware simultaneously or in quick succession. Batch_size 2 or rank 64 runs are also within envelope. | Queue as a "hyperparameter exploration" follow-up once Phase 10 gives us a bigger eval signal. |
| O4 | **`ape.reflect` +35% lift suggests more room on self-reflective tasks.** The weakest untuned task moved to 1.000 with minimal volume (50 rows). Adding RL-style preference data on reflection quality could push synthesis tasks further. | Revisit after Phase 10. Candidate for GRPO/DPO's first signal. |
| O5 | **Standalone merged model simplifies deployment.** No adapter-path overhead at inference; `mlx_lm.server --model .local-models/qwen3-14b-mdemg-v1 --port 8101` is the one-line serve. Pairs cleanly with MDEMG's `LLM_ENDPOINT` env var. | Wire into deployment integration (Sprint F+) once Phase 10 validates production fit. |
| O6 | **Training metrics streaming to TSDB was not wired for this run.** Grafana dashboard JSON is in the plan but not authored. Next run is a chance to build it. | Low-priority follow-up. File-based logs in `adapters/tier1/train.log` are authoritative for now. |
| O7 | **Sprint D routing profiles can be repurposed as a research artifact.** Even though the MoE path is dead, the per-family expert activation maps are real data about how the base model organizes knowledge. Publishable as a small research note if we write it up. | Nice-to-have, not on the critical path. |

### Explicit non-goals

- **Reviving MoE Tier 2 family adapters** — abandoned, not deferred. Requires upstream Metal/MLX changes outside our control.
- **Running GRPO/DPO before Phase 10** — inverts the dependency graph. Phase 10 must produce the reward signal first.
- **OpenAI fine-tuning** — FT-OAI-003 dropped 2026-04-22. All fine-tuning targets local MLX going forward.

---

## 6. Tl;dr — For the PR / Handoff

- **Outcome:** 0.9856 weighted vs 0.9505 untuned dense baseline (+3.5 pts), vs 0.9805 untuned MoE 35B (+0.5 pts at 40% params). **16/16 tasks passing ≥80%.**
- **Artifact:** `.local-models/qwen3-14b-mdemg-v1/` (7.8 GB). Adapter at `adapters/tier1/adapters.safetensors` (Iter 2400 best). Manifest SHAs pinned.
- **Pivot:** MoE → dense mid-sprint. Metal 499K ceiling is architectural; not reversible from our side.
- **Policies:** Epoch cap 3, early-stop 1.05×2, no hardcoded values, single-instance MLX, SHA pins — all honored.
- **Unblocks:** Phase 10 benchmark authoring; Sprint F (GRPO/DPO) after Phase 10.
- **Biggest open risk:** no offsite backup of merged model (R4). Biggest open opportunity: Phase 10 (O1).

---

## Documents Accessed

- [`phase_5_sft_post.md`](phase_5_sft_post.md) — companion full post-run report
- [`adapters/tier1/manifest.json`](../../../adapters/tier1/manifest.json) — SHA-pinned run metadata
- [`adapters/tier1/train_report.json`](../../../adapters/tier1/train_report.json) — orchestrator report
- [`adapters/tier1/.earlystop.json`](../../../adapters/tier1/.earlystop.json) — early-stop sidecar
- [`training_data/eval/`](../../../training_data/eval/) — three eval reports + two regression verdicts
- `00_README_v2.md` (v5.5, pre-pivot plan context)
- `03_IMPLEMENTATION_PLAN_v2.md §5A-5D`
- `~/.claude/plans/breezy-dancing-lerdorf.md` (pre-pivot Phase 5 plan — superseded)
- `memory/project_phase5_moe_pivot.md`
