# Sprint FT-LORA-PHASE5 — Phase 5 SFT Execution + Monitoring

> **AMENDMENT (2026-04-23) — Mid-sprint MoE → dense pivot.**
> This plan was authored 2026-04-22 against the **MoE-Sieve two-tier strategy** (Tier 1 universal attn+shared expert + 3× Tier 2 family adapters on Qwen3.6-35B-A3B-mxfp4). On 2026-04-22, mid-execution, the MoE path was **abandoned**: the Metal 499K MTLResource ceiling on M5 Max / macOS 26 blocked every non-trivial MoE LoRA backward pass (identical `[metal::malloc] Resource limit (499000) exceeded` across 4 mxfp4 configurations *and* standard q4 — the cap is architectural, not quant-specific; macOS 26 removed the `iogpu.rsrc_limit` sysctl so there is no user-space fix).
>
> **What actually shipped:** single-tier LoRA on **dense `mlx-community/Qwen3-14B-4bit`** (40 layers, hidden 5120, 7 target modules `self_attn.{q,k,v,o}_proj` + `mlp.{gate,up,down}_proj`). Tier 1 policies held (`rank=32 α=64`, epoch cap 3, early-stop `val_loss > best × 1.05` for 2 consecutive evals, seq 8192, LR 5e-5, seed 0, grad_checkpoint). Three Tier 2 family adapters + asymmetric quant were **dropped** for Phase 5 — no dense analog exists (no shared expert, no routed experts, no router). Sprint D routing profiles + `quantize_asymmetric.py` + `expert_selection.py` remain in-repo as research artifacts.
>
> **Final result:** single merged model `.local-models/qwen3-14b-mdemg-v1/` (7.8 GB, 4-bit preserved via `mlx_lm fuse`); dual regression gate **PASS** against both the pre-pivot MoE baseline (0.9805 → 0.9856) and a fresh dense baseline (0.9505 → 0.9856); 16/16 ULTS tasks passing post-tune vs 15/16 at baseline. See [`phase_5_sft_post.md`](phase_5_sft_post.md) + [`phase_5_sft_summary.md`](phase_5_sft_summary.md) for the executed pipeline's full details.
>
> **The body below is preserved unmodified as the planning-phase record** (MEMORY rule: sprint plans go in `docs/development/<sprint-line>/`, never `~/Downloads/`). It documents the *original intent* — not what shipped. Cross-reference `phase_5_sft_post.md` and `phase_5_sft_summary.md` for execution truth.

---

## Context

Sprint FT-LORA-DATA (PR #346) merged `234baec` on 2026-04-22, producing four curated, SHA256-pinned training datasets under `training_data/sft/{tier1, family_reasoning_think, family_classify_notink, family_structured_notink}/`. This unblocks **Phase 5 SFT**: the first real LoRA fine-tuning against Qwen3.6-35B-A3B-mxfp4 using the MoE-Sieve two-tier strategy locked in by memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1.

**Why now:**
- All preconditions are green: Sprint C (MLX validation, 3 gates), Sprint D (expert profiling, 3 families), Sprint E (training infra patches — router_aux_loss_coef, asymmetric quant, early-stop, regression gate), FT-LORA-DATA (dataset curation).
- The forcing function — FT-OAI-001 step-1200 overfit — is what drove the overfitting-prevention policies (epoch cap 3, explicit integer `n_epochs`, early-stop `val_loss > best × 1.05` for 2 evals). Phase 5 is the first run where those policies are exercised on Qwen3.6.
- Phase 5 SFT is **stage 1 of a 5-stage pipeline** per `04_BENCHMARK_RL_v2.md:19-35`: SFT → Phase 10 benchmarks → GRPO/DPO automated → HITL DPO → deploy. Other fine-tuning methods (GRPO, DPO) are deliberately deferred — they require SFT-produced policies as warm-start anchors, and GRPO needs RLVR-ready reward signal that Phase 10 benchmarks define. Running GRPO/DPO before SFT stabilizes the base-model behavior inverts the dependency graph.

**Sprint chain:** A(#335) → B(#336) → C(#338/#339/#340) → D(#343) → E(`14cd2b3`) → DATA(#346, `234baec`) → **PHASE5 (this sprint)** → Phase 10 benchmark → F (GRPO/DPO).

**User design decisions (captured 2026-04-22 via AskUserQuestion):**
1. **Execution cadence:** gated pause between Tier 1 and each Tier 2 run (Sprint C-style).
2. **Regression gate FAIL/WARN response:** halt + triage; no Tier 2 until resolved.
3. **Baseline source:** fresh ULTS 16-task capture on untuned Qwen3.6-mxfp4.

---

(The remainder of this file is the original 12-section sprint plan. See `~/.claude/plans/breezy-dancing-lerdorf.md` for the verbatim pre-pivot text. To avoid duplicating a superseded plan in-repo, the execution docs — `phase_5_sft_post.md`, `phase_5_sft_summary.md`, `adapters/tier1/manifest.json` — are the authoritative record of what shipped.)
