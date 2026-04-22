# MDEMG Fine-Tuning Plan — Complete Document Suite

**Date:** 2026-04-22
**Version:** 5.4 (Sprint FT-LORA-E — Training infrastructure patched; Phase 5 SFT unblocked)

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
| Phase 11: Automated RL (GRPO/DPO) | ⬜ NOT STARTED | — |
| Phase 12: Human-in-the-Loop | ⬜ NOT STARTED | — |

### Immediate Actions Required

1. **Config flip (P0, 5 min):** Set `NEURAL_DATA_COLLECTION=true`, `J17_PROTOCOL_DATA_COLLECTION=true`, `TSDB_BACKUP_ENABLED=true` and restart
2. **SanitizeResponse (P1):** Build `internal/llmclient/sanitize.go` — required before switching to any local model
3. **System prompt hash (P1):** Add to InteractionRecord for training data versioning
4. **RAFT context capture (P1):** Enrich InteractionRecord with retrieval context
5. **ULTS specs (P1):** 16 spec files formalizing LLM call contracts
