# Phase 11.5c — Clean Eval Post-Doc

**Sprint:** FT-LORA-PHASE11.5c
**Date:** 2026-04-28
**Branch:** `reh3376_dev01`
**Status:** EXECUTED — eval shipped, baselines run, strategic pivot recommended

---

## Executive Summary

Built `valid_clean.jsonl` (180 rows, 9 of 17 tasks) from production TSDB with **0% prompt leakage** against the 9 Phase 5 SFT train/valid sources. Verified existing `valid_golden.jsonl` had **94 of 95 prompts (99%) leaking** with training data — the Phase 10 baseline was largely measuring memorization.

Re-baselined Phase 5 dense and gpt-5.4-mini against the clean eval. Phase 5 dropped 2.86pp (0.8338 → 0.8052), gpt-mini dropped 2.07pp (0.8769 → 0.8562). The +5pp gap between models held essentially unchanged.

**Critical strategic finding:** the per-task picture **invalidates the premise of Phase 11 GRPO and Branch B distillation**. The "regressor" tasks chosen for those sprints (`retrieval.query_classify`) are not actually weak on production data — they only looked weak because the leaked golden carve picked hard edge cases. The **real** weak tasks (`consulting.classify`, `retrieval.rerank_cross`) were hidden by leak inflation and never targeted.

**Recommended next sprint:** **Branch B revised** — distillation on `consulting.classify` (primary) + `retrieval.rerank_cross` (secondary), targeting the realistic +5pp aggregate ceiling that gpt-mini exhibits on clean. Run 5/Run 7 should be re-baselined on clean first to confirm whether Phase 11 RL gains were real or memorization-preservation.

---

## What Was Built

| Artifact | Path | Description |
|---|---|---|
| Clean eval JSONL | `training_data/eval/valid_clean.jsonl` | 180 rows, 9 tasks, leak-free |
| Eval manifest | `training_data/eval/valid_clean_manifest.json` | Per-task TSDB→leak-filter→dedup→final counts, source SHA256s |
| Extractor | `scripts/build_clean_eval.py` | Reusable for future clean evals |
| Leakage auditor | `scripts/audit_eval_leakage.py` | Generic eval-vs-source overlap tool, exit-code gated |
| valid_clean audit | `training_data/eval/valid_clean_leakage_audit.json` | 0 / 180 overlap |
| valid_golden audit (historical) | `training_data/eval/valid_golden_leakage_audit.json` | 94 / 95 overlap (99% leakage) |
| Spec patches | 5 ULTS spec files (`*.json.bak` backups in same dir) | Aligned spec system_prompt_hash with current production code |
| Spec audit | `training_data/eval/spec_production_audit.json` | Cross-reference of spec/prod/TSDB hashes |
| Spec patch plan | `training_data/eval/spec_patch_plan.json` | Verified production hashes, applied patches |
| Phase 5 clean baseline | `training_data/eval/baseline_phase5_clean.json` | aggregate 0.8052 |
| gpt-mini clean baseline | `training_data/eval/baseline_gpt54mini_clean.json` | aggregate 0.8562 |
| Delta report | `training_data/eval/clean_vs_golden_delta.md` | Per-task golden-vs-clean comparison |

## Spec Patches Applied (Path B)

5 ULTS specs had stale `system_prompt_hash` not matching current production code:

| Task | Old | New | Reason |
|---|---|---|---|
| `ape.reflect` | `64d528cc23df…` | `ef068ab10984…` | Spec hash was over the unrendered template (with `%s` placeholder); patched to rendered hash with `AllowedLLMActions` enum substituted (matches what production sends and TSDB has 44,635 rows of) |
| `hidden.summarize` | `5110d51bc460…` | `ddeac4afb634…` | Spec was pointed at wrong file (`internal/summarize/service.go` instead of `internal/hidden/cluster_summarizer.go`); patched to actual production prompt |
| `hidden.reclassify` | `"dynamic"` | `["2fafc511…", "dynamic"]` | Templated prompt; added the dominant rendered hash (357 TSDB rows) while keeping `"dynamic"` flag |
| `jiminy.evaluate_llm` | `caf70a3d…` (same as `jiminy.evaluate`) | `["d8f4e237…", "1f02ee46…"]` | Spec hash was a copy-paste of the wrong task; patched to list both production variants (full + compact prompt selected by config) |
| `retrieval.rerank_cross` | `8a7750d2…` (full only) | `["8a7750d2…", "1f0f374d…"]` | Production runs in `RerankCompress=true` mode using the compact prompt; spec only listed the full prompt; extended to list both |

After patches: 11 of 17 tasks match TSDB (vs 6 of 17 before). Total matched rows: 54,216 (from 56,394 in TSDB).

`.bak` files preserved for each patched spec to enable rollback.

## Aggregate Comparison

| Model | valid_golden | valid_clean | Δ |
|---|---|---|---|
| Phase 5 dense | 0.8338 | 0.8052 | -2.86pp |
| gpt-5.4-mini | 0.8769 | 0.8562 | -2.07pp |
| Gap (gpt-mini − Phase 5) | +4.31pp | +5.10pp | Gap held |

Memorization was real but moderate — not the catastrophic 5pp+ collapse a first-pass measurement (with 60s MLX timeout truncation) suggested. The retry with 300s timeout produced the authoritative 0.8052.

## Per-Task Findings — The Strategic Pivot

| Task | Group | P5 golden | P5 clean | Δ | Interpretation |
|---|---|---|---|---|---|
| `consulting.classify` | C | 0.767 | **0.490** | **-27.7pp** | **Real regressor uncovered by clean eval. Distillation primary target.** |
| `retrieval.rerank_cross` | J | 0.900 | **0.720** | **-18.0pp** | Hidden weakness; secondary distillation target. |
| `jiminy.synthesize` | T | 0.755 | 0.668 | -8.7pp | Mild regression on production data. |
| `jiminy.codegen` | C | 1.000 | 0.970 | -3.0pp | Minor. |
| `ape.reflect` | T | 0.867 | 0.887 | +2.0pp | Stable. |
| `hidden.name_emergence` | J | 0.950 | 0.950 | 0.0pp | Stable. |
| `retrieval.intent_translate` | C | 1.000 | 1.000 | 0.0pp | Stable, ceiling-saturated. |
| `retrieval.query_classify` | C | 0.700 | **0.900** | **+20.0pp** | **Was NEVER a real regressor.** Golden carve picked hard cases; production data is easier. The original Phase 11 GRPO + Branch B target was a phantom. |
| `hidden.reclassify` | C | 0.500 | **0.800** | **+30.0pp** | Phase 5 is BETTER on production than on golden. Golden carve was harder than reality. |

**Three claims this invalidates:**

1. **"Phase 5 scores 0.60 on retrieval.query_classify and needs RL/distill help"** — false. Phase 5 scores 0.90 on real production data. The Phase 11 GRPO sprint and the original Branch B distillation set were chasing a leaked-golden artifact.

2. **"Run 5 +1.76pp aggregate gain over Phase 5"** — measured against leaked golden. We don't know what Run 5/Run 7 score on `valid_clean`. Re-baselining is the immediate next step.

3. **"Phase 5 has memorization-driven inflation in its aggregate score"** — true, but the magnitude is moderate (+2.86pp from leakage), not the larger drop a first-pass measurement showed. The bigger story is per-task variance: some tasks score *higher* on clean (hidden.reclassify +30pp, retrieval.query_classify +20pp), some much lower (consulting.classify -28pp, retrieval.rerank_cross -18pp).

## What Was Hidden by Leakage

`consulting.classify` is the most consequential discovery. It was scored at 0.767 on golden (matched gpt-mini's golden 0.767). On clean:
- Phase 5: **0.490**
- gpt-mini: **0.883**
- Gap: **+39.3pp** (gpt-mini ahead)

This is the highest-leverage distillation target in the model. Lifting Phase 5 here from 0.49 → 0.88 = +39pp on a C-group task with 1/5 of the C-group weight (0.35 / 5 = 0.07) → **+2.7pp aggregate** from a single task.

`retrieval.rerank_cross`: Phase 5 0.72 vs gpt-mini 0.90 on clean. +18pp gap on J-group (0.15 weight / 4 = 0.0375 per task) → **+0.7pp aggregate**.

Combined ceiling from distilling these two tasks: **~+3.4pp aggregate** if we hit gpt-mini parity. With realistic SFT yielding 60-80% of the gap, expect **+2.0-2.7pp aggregate** in practice. That's the realistic Branch B target.

## Data-Starved Tasks (8 of 17)

No production TSDB rows after spec/hash matching:
- `consulting.synthesis` — synthesis is dynamic; no rows logged with current prompt structure
- `guardrail.evaluate` — 3 TSDB rows with mismatching hashes (1 unique prompt usable)
- `hidden.summarize` — 1 TSDB row (untrainable)
- `jiminy.evaluate` — TSDB rows mislabeled (contain outcome_classifier content, wrong task)
- `jiminy.evaluate_llm` — TSDB rows tagged with `jiminy.evaluate` task_name; prompt content matches outcome_classifier but task_name routing is broken
- `metalearn.generalize` — no production traffic
- `retrieval.rerank_nli` — no production traffic
- `summarize.generate` — no production traffic

These need synthetic prompt augmentation OR mislabeled-task remediation as a follow-up sprint (Phase 11.5d). For now they are flagged in `valid_clean_manifest.json` and contribute nothing to the aggregate.

## Recommended Next Sprint

**Phase 11.5d — Branch B Revised (Distillation, leak-aware, retargeted)**

Replace the original Branch B distill set with the empirically-correct targets:

- **Primary:** `consulting.classify` (-28pp Phase 5 deficit, gpt-mini at 0.88 → +28pp ceiling, +2.7pp aggregate)
- **Secondary:** `retrieval.rerank_cross` (-18pp deficit, gpt-mini at 0.90 → +0.7pp aggregate)
- **Drop:** `retrieval.query_classify`, `ape.reflect`, `jiminy.synthesize`, `guardrail.evaluate` (Phase 5 either matches/beats gpt-mini OR data-starved)

Capture gpt-mini responses on production-style prompts that are NOT in any train/valid file. Use `scripts/build_clean_eval.py` infrastructure to source prompts; sample 50-100 per task at temp=0.7 (C-group) or temp=0.0 (J-group); filter reward ≥0.8.

Stage-1 SFT via `mlx_lm.lora` on `~150-200 pairs` (similar to original Branch B Epic 2 plan). Eval against `valid_clean.jsonl`. Target aggregate **0.83+** (from 0.8052 baseline → +2.5pp).

**Pre-sprint task:** Re-baseline Run 5 + Run 7 adapters on `valid_clean.jsonl`. ~30 min compute, no OpenAI cost. Confirms whether Phase 11 RL gains were real generalization or memorization preservation. If they hold (i.e., Run 5 ≥ Phase 5 on clean), use Run 7 as the distillation starting point. If they don't, restart from Phase 5 base.

## Costs Incurred

- OpenAI gpt-5.4-mini benchmark on valid_clean: **~$0.07** (well under $1)
- TSDB queries: free
- Phase 5 local MLX inference: free (local compute, ~30 min wall-clock)
- Total: **~$0.07**

## Documents Accessed

- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5c.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/branch_b_implementation_plan.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_5_sft_post.md`
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_phase_11_5.md`
- `/Users/reh3376/mdemg/docs/tests/ults/specs/*.ults.json` (17 specs)
- `/Users/reh3376/mdemg/internal/{ape,consulting,guardrail,hidden,jiminy,metalearn,retrieval,summarize}/*.go` (production prompt source code)
- `/Users/reh3376/mdemg/training_data/sft/{tier1,family_*}/{train,valid}.jsonl`
- `/Users/reh3376/mdemg/training_data/eval/valid_golden.jsonl`
- TSDB `llm_interactions` table (56,394 rows, queried per-task)
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`

## Approvals

- User selected Path B+C (spec audit + per-row TSDB extraction) at the strategic fork.
- User approved continued execution after preflight surfaced 11/17 task hash mismatch.

---

## Open Questions for Next Sprint

1. **Should Run 5 / Run 7 be re-baselined first?** Strongly recommended — they're the latest training artifacts and we don't know their honest scores.
2. **Should `valid_clean` augment the 8 data-starved tasks via synthetic prompt generation?** Defer to follow-up sprint; not blocking.
3. **Should Phase 5 SFT be retrained with task-balanced sampling to fix `consulting.classify`?** Possibly — 28pp drop suggests this task got insufficient training weight. Worth checking class balance in `tier1/train.jsonl`. If the task has enough training rows but still failed, it's a model-capacity or hyperparam issue, not data. If it had few training rows, retrain may help.
4. **Should the spec patches be committed?** Yes — they correct genuine drift between specs and production code. Backups preserved.
