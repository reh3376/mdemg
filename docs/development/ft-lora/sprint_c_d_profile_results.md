# Sprint FT-LORA-D — Expert Activation Profile Results

**Generated at execution time: 2026-04-22T09:11:09Z**

**Runbook ref:** `docs/development/ft-lora/sprint_plan_ft_lora_d.md` §5 Epic 3

---

## Executive Summary

**Verdict: `3-family-confirmed`**

No family-pair exceeds the 0.80 Jaccard overlap ceiling. The 3-family partition (reasoning-think / classify-notink / structured-notink) is empirically validated. Sprint E proceeds with three Tier 2 LoRA adapters as planned in memo 07 v3.1 §5.

**Per-family task-cohesion:**

- `reasoning_think`: 7 tasks, mean within-family Jaccard = 0.4202 → **ambiguous**
- `classify_notink`: 6 tasks, mean within-family Jaccard = 0.4886 → **ambiguous**
- `structured_notink`: 3 tasks, mean within-family Jaccard = 0.4880 → **ambiguous**

**Determinism sanity check:** bit-identical (strict)

---

## Cross-family Jaccard Overlap

Top-25% routed-expert sets (64 of 256 per layer) compared pairwise. Mean Jaccard averaged across 40 layers. Threshold for partition merge: > 0.8.

| Family pair | Mean Jaccard | Exceeds 0.80? |
|---|---|---|
| `classify_notink__vs__reasoning_think` | 0.4952 | no |
| `classify_notink__vs__structured_notink` | 0.6863 | no |
| `reasoning_think__vs__structured_notink` | 0.5371 | no |

## Per-family Task-Cohesion

Within-family pairwise Jaccard of per-task top-25% expert sets, averaged across 40 layers. Verdict rule: ≥0.70 cohesive / 0.40-0.70 ambiguous / <0.40 split-candidate.

| Family | n_tasks | Mean within-family Jaccard | Verdict |
|---|---|---|---|
| `reasoning_think` | 7 | 0.4202 | ambiguous |
| `classify_notink` | 6 | 0.4886 | ambiguous |
| `structured_notink` | 3 | 0.4880 | ambiguous |

### Detailed pairwise task-cohesion

**reasoning_think** — 7 tasks

| Task pair | Mean Jaccard |
|---|---|
| `ape.reflect__vs__consulting.synthesis` | 0.1681 |
| `ape.reflect__vs__hidden.summarize` | 0.2266 |
| `ape.reflect__vs__jiminy.synthesize` | 0.2960 |
| `ape.reflect__vs__metalearn.generalize` | 0.2301 |
| `ape.reflect__vs__retrieval.rerank_nli` | 0.3091 |
| `ape.reflect__vs__summarize.generate` | 0.9696 |
| `consulting.synthesis__vs__hidden.summarize` | 0.5705 |
| `consulting.synthesis__vs__jiminy.synthesize` | 0.5063 |
| `consulting.synthesis__vs__metalearn.generalize` | 0.5502 |
| `consulting.synthesis__vs__retrieval.rerank_nli` | 0.4587 |
| `consulting.synthesis__vs__summarize.generate` | 0.1715 |
| `hidden.summarize__vs__jiminy.synthesize` | 0.4788 |
| `hidden.summarize__vs__metalearn.generalize` | 0.9196 |
| `hidden.summarize__vs__retrieval.rerank_nli` | 0.4225 |
| `hidden.summarize__vs__summarize.generate` | 0.2292 |
| `jiminy.synthesize__vs__metalearn.generalize` | 0.4805 |
| `jiminy.synthesize__vs__retrieval.rerank_nli` | 0.5676 |
| `jiminy.synthesize__vs__summarize.generate` | 0.3001 |
| `metalearn.generalize__vs__retrieval.rerank_nli` | 0.4253 |
| `metalearn.generalize__vs__summarize.generate` | 0.2327 |
| `retrieval.rerank_nli__vs__summarize.generate` | 0.3117 |

**classify_notink** — 6 tasks

| Task pair | Mean Jaccard |
|---|---|
| `consulting.classify__vs__hidden.reclassify` | 0.3869 |
| `consulting.classify__vs__jiminy.codegen` | 0.3318 |
| `consulting.classify__vs__jiminy.evaluate` | 0.6733 |
| `consulting.classify__vs__retrieval.intent_translate` | 0.7164 |
| `consulting.classify__vs__retrieval.query_classify` | 0.7227 |
| `hidden.reclassify__vs__jiminy.codegen` | 0.3597 |
| `hidden.reclassify__vs__jiminy.evaluate` | 0.3805 |
| `hidden.reclassify__vs__retrieval.intent_translate` | 0.2989 |
| `hidden.reclassify__vs__retrieval.query_classify` | 0.3032 |
| `jiminy.codegen__vs__jiminy.evaluate` | 0.3959 |
| `jiminy.codegen__vs__retrieval.intent_translate` | 0.3111 |
| `jiminy.codegen__vs__retrieval.query_classify` | 0.3088 |
| `jiminy.evaluate__vs__retrieval.intent_translate` | 0.6179 |
| `jiminy.evaluate__vs__retrieval.query_classify` | 0.6105 |
| `retrieval.intent_translate__vs__retrieval.query_classify` | 0.9112 |

**structured_notink** — 3 tasks

| Task pair | Mean Jaccard |
|---|---|
| `hidden.name_emergence__vs__jiminy.evaluate_llm` | 0.5125 |
| `hidden.name_emergence__vs__retrieval.rerank_cross` | 0.4200 |
| `jiminy.evaluate_llm__vs__retrieval.rerank_cross` | 0.5316 |

## KL Divergence vs Uniform (per family)

High KL = concentrated routing (Sieve thesis supported). Low KL = diffuse routing (flag for Sprint E commit-or-fallback).

| Family | KL(min) | KL(mean) | KL(max) |
|---|---|---|---|
| `reasoning_think` | 0.163 | 0.593 | 0.918 |
| `classify_notink` | 0.199 | 0.654 | 0.913 |
| `structured_notink` | 0.216 | 0.687 | 0.925 |

## Coverage

**Anchor prompts:** 320 total (20/task × 16 tasks). SHA256: `7eddeccafa9ffcdd…`

Per-family:
- `reasoning_think`: 140 prompts, 178,891 tokens processed
- `classify_notink`: 120 prompts, 107,724 tokens processed
- `structured_notink`: 60 prompts, 77,312 tokens processed

## Recommendation to Sprint E

Proceed as planned: train three Tier 2 LoRA adapters (r=8 α=16), one per family. Load `profile_routing_{family}.json` via `--expert-selection-path` in `neural/training/train_ft.py`.

**Secondary signals for Sprint E planner:**

1. **Highest cross-family overlap is `classify_notink ↔ structured_notink` at 0.6863** — below the 0.80 merge threshold but materially higher than the other two pairs (both ≈0.50). Structured-output tasks (J) lean on classifier-like routing; if Tier 2 training of these two families shows correlated loss curves or eval interference, Sprint E may revisit with a merged `classify-or-structured` adapter + dedicated `reasoning-think` adapter (a 2-family partition).

2. **All three families scored `ambiguous` on within-family task-cohesion (0.42 / 0.49 / 0.49)** — no family is `cohesive` (≥0.70), none is `split-candidate` (<0.40). The 3-family grouping is defensible but not tight; Tier 2 adapters should train with `router_aux_loss_coef=0.002` as planned and eval per-task (not just per-family) to catch within-family regressions.

3. **Backfill artifact warning.** Three task pairs show extreme within-family Jaccard (`ape.reflect ↔ summarize.generate` = 0.9696; `hidden.summarize ↔ metalearn.generalize` = 0.9196; `retrieval.intent_translate ↔ retrieval.query_classify` = 0.9112). The first two are direct artifacts of the Epic 1 donor-backfill (those 5 T-family tasks with 0-1 production records were backfilled from same-shape donor tasks — see `docs/development/ft-lora/00_README_v2.md` v5.3 changelog). The third (0.9112) appears to be real production signal since both tasks are retrieval-sourced. **Net effect:** the `reasoning_think` within-family mean of 0.4202 is **inflated** by the donor pairs; true cohesion is likely lower. This reinforces the "ambiguous" verdict and the per-task eval recommendation above.

4. **KL concentration is healthy** across all three families (mean 0.59-0.69 nats vs uniform_256 max ln(256) ≈ 5.55). Routing is concentrated enough that the Sieve thesis — top-25% selection captures the signal — holds empirically. No commit-or-fallback escalation needed from this signal alone.


## Artifact SHAs

- `profile_routing_reasoning_think.json`: `f29e38141373d8ab1951b17fb2a4a93b34d2f167e39ac343e869e04e57417271`
- `profile_routing_classify_notink.json`: `12acdf803f8c4f43255e0ca10406ad86e3428c89836abee807e64cbe34c240c5`
- `profile_routing_structured_notink.json`: `602fa02d69a8cc91f56137726c9ed1d63ff3c7ba8dd68655f62e3f1186b30d4d`
- `raw_activation_counts.json`: `ed5e210bf2e7b6531eca1600bdf7a98332dd839cde5976c5e027ef02098d5e33`

## Documents Accessed

- `training_data/routing_profiles/profile_routing_reasoning_think.json`
- `training_data/routing_profiles/profile_routing_classify_notink.json`
- `training_data/routing_profiles/profile_routing_structured_notink.json`
- `training_data/routing_profiles/raw_activation_counts.json`
- `training_data/routing_profiles/anchor_prompts.jsonl`
- `docs/development/ft-lora/sprint_plan_ft_lora_d.md`
- `docs/development/ft-lora/01_RESEARCH_v2.md §5`
- `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X`

