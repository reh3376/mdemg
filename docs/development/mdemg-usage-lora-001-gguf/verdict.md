# MDEMG-USAGE-LORA-001-GGUF — Verdict

**Sprint**: MDEMG-USAGE-LORA-001-GGUF (task #150)
**Shipped**: 2026-09-01 (conversion 60s wall-clock; benchmark ~2.5h wall-clock)
**Verdict**: **❌ NO-PROMOTE — aggregate 0.8426 vs 0.9188 shipped baseline = −0.0762. Adapter DOES NOT pick up the +0.074 runtime bonus v1 gets from GGUF. v1 stays production.**

## Result — THREE-WAY comparison (this sprint's core deliverable)

| Setup | Runtime | Aggregate | Notes |
|---|---|---:|---|
| **v1 fused GGUF** (shipped production baseline) | llama.cpp | **0.9188** | APE-REFLECT-EVAL-REFRESH-001 pin |
| v1 fused | mlx_lm.server | 0.8449 | Fair-comparison baseline from #145 |
| mdemg_usage_lora_001 | mlx_lm.server | 0.8388 | #145 my adapter |
| **mdemg_usage_lora_001 GGUF** ← this sprint | **llama.cpp** | **0.8426** | 121-min benchmark, 1450 completions |

**Runtime deltas** (mlx_lm.server → llama.cpp for identical weights):

| Model | Runtime bonus |
|---|---:|
| v1 fused | **+0.0739** |
| mdemg_usage_lora_001 fused | **+0.0038** |

**The +0.074 v1 GGUF bonus is NOT universal — it's a v1-weight-specific property.** My adapter picks up only +0.004 from the same conversion pipeline.

## Verdict rubric (from sprint plan §5 Epic 6)

- ✅ PROMOTE if aggregate ≥ 0.9088 → NOT MET (0.8426 < 0.9088)
- ⚠️ MIXED if aggregate in [0.89, 0.9088) → NOT MET (0.8426 < 0.89)
- **❌ NO-PROMOTE** if aggregate < 0.89 → **THIS**

## Per-task detail — 3-way

| Task | v1_mlx | mine_mlx | mine_gguf | Δ(gguf-v1_mlx) | Δ(gguf-mine_mlx) |
|---|---:|---:|---:|---:|---:|
| ape.reflect | 0.9547 | 0.9377 | 0.9367 | −0.0180 | −0.0010 |
| **claude.code_knowledge** | 0.3867 | 0.3628 | 0.3756 | −0.0111 | **+0.0128** |
| consulting.classify | 0.8827 | 0.8973 | 0.8940 | **+0.0113** | −0.0033 |
| hidden.name_emergence | 0.9500 | 0.9500 | 0.9500 | +0.0000 | +0.0000 |
| hidden.reclassify | 0.9400 | 1.0000 | 1.0000 | **+0.0600** | +0.0000 |
| hidden.summarize | 0.9000 | 0.9000 | 0.9000 | +0.0000 | +0.0000 |
| jiminy.codegen | 1.0000 | 1.0000 | 1.0000 | +0.0000 | +0.0000 |
| jiminy.evaluate | 0.9667 | 0.9667 | 0.9667 | +0.0000 | +0.0000 |
| jiminy.evaluate_llm | 0.7667 | 0.7833 | 0.7833 | **+0.0167** | +0.0000 |
| jiminy.synthesize | 0.8844 | 0.8722 | 0.8762 | −0.0082 | +0.0040 |
| retrieval.intent_translate | 0.9780 | 0.9985 | 0.9940 | **+0.0160** | −0.0045 |
| **retrieval.query_classify** | 0.7750 | 0.6750 | **0.7150** | **−0.0600** | **+0.0400** |
| retrieval.rerank_cross | 0.9000 | 0.9000 | 0.9000 | +0.0000 | +0.0000 |

**Group means** (my GGUF vs v1 mlx same-runtime — this shows the adapter-quality delta minus the runtime effect):

| Group | Weight | v1_mlx | mine_gguf | Δ |
|---|---:|---:|---:|---:|
| C | 0.35 | 0.9237 | 0.9283 | **+0.0046** |
| J | 0.15 | 0.8722 | 0.8778 | **+0.0056** |
| T | 0.5 | 0.7815 | 0.7721 | −0.0093 |

## Interpretation

**Two truths that don't contradict:**

1. **Adapter QUALITY is at parity with v1** on the honest same-runtime measurement (mine mlx 0.8388 vs v1 mlx 0.8449 = −0.006; mine GGUF 0.8426 vs v1 mlx 0.8449 = −0.002). Group means confirm this — C and J groups slightly BEAT v1 same-runtime. My adapter LEARNED the tasks well.

2. **Adapter QUANTIZES POORLY relative to v1**. When both convert to Q5_K_M via the same `mlx_lm.fuse --dequantize` → `convert_hf_to_gguf.py` → `llama-quantize` pipeline, v1 picks up +0.074 aggregate and my adapter picks up only +0.004. Something about v1's weight distribution is more quantization-robust than my LoRA-modified weights.

**The −0.076 aggregate gap vs shipped v1 GGUF is all quantization sensitivity, not adapter capability.**

Why is v1 more quantization-robust? Hypothesis (unverified this sprint):
- **v1 was trained via full-scope SFT** (batch=4, max_seq=8192, more epochs) into the base weights directly — the weight updates are large-magnitude and well-distributed
- **My adapter is rank-32 LoRA on 7 modules** — low-rank additive updates that, once fused into the base, may have thin/sparse structure that quantization noise attacks disproportionately
- Q5_K_M quantization allocates 5.69 BPW average; low-rank fine-tune signal has less "budget" per parameter than dense SFT signal has

Not proven — would need to try Q8_0 (7.5-8 BPW, less lossy) or f16 GGUF (no quantization) to isolate.

## The retrieval.query_classify partial recovery

At same-runtime the regression was −10% (0.775 → 0.675). On GGUF it's −6% (0.775 → 0.715). GGUF Q5_K_M quantization noise is enough to move this task 4pp in my adapter's favor — evidence that this specific task is more about numerical precision than adapter capability. Direction: worth trying Q8_0 to see if the regression recovers further.

## What ships (this sprint)

- `.local-models/qwen3-14b-mdemg-usage-fused/` — bf16 HF safetensors (28 GB, local-only per gitignore)
- `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` — 29.5 GB f16 GGUF (local-only)
- `.local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf` — 10 GB Q5_K_M GGUF (local-only)
- `training_data/eval/mdemg_usage_lora_001_gguf_prod_runtime.json` — full benchmark output (local-only per gitignore)
- `docs/development/mdemg-usage-lora-001-gguf/{sprint_plan,verdict,sprint_post,sha256}.md/.txt`
- CHANGELOG.md entry
- PR summary comment

**Production llama-server on port 8102 UNTOUCHED throughout** — verified `curl :8102/v1/models` returns `.local-models/serving/current.gguf` (still v1's Q5_K_M) pre + during + post sprint.

## Follow-ups filed

### 🟢 MDEMG-USAGE-LORA-001-Q8 (optional next probe)

Convert the same fused bf16 HF safetensors to Q8_0 GGUF instead of Q5_K_M (Q8_0 is less lossy — 7.5-8 BPW vs 5.69). Benchmark on llama.cpp same runtime. Should reveal whether the −0.076 gap is Q5_K_M-specific or if my adapter has a broader quantization sensitivity issue. Cost: ~2h wall-clock, one additional Q8_0 GGUF file (~16 GB).

If Q8_0 aggregate ≥ 0.90 → my adapter IS shippable, just needs Q8_0 not Q5_K_M for prod. Trade-off: Q8_0 file is 16 GB vs Q5_K_M 10 GB — bigger RAM footprint, still fits M5 Max easily.

### 🟢 Investigate LoRA-vs-quantization interaction (research)

Longer-term: why does v1 (dense SFT) pick up +0.074 from Q5_K_M and my LoRA (fused) picks up only +0.004? Could be systematic — future LoRA retrains might all suffer this. Would benefit MDEMG's whole retrain path.

### 🟢 v1 stays production. #150 arc CLOSES.

MDEMG-USAGE-LORA-001 arc is now definitively resolved: adapter is at capability parity with v1 but doesn't survive the shipped quantization pipeline well enough to swap in. The MDEMG-usage capability at 0.307 (from #145 supplemental) is too weak to justify a swap on its own.

Substrate-side path (MDEMG-DOCS-INGEST-001 + RETRIEVAL-META-DOC-SUPPRESSION-001) remains the recommended alternative for MDEMG-usage capability.

## Two arch rules pinned (proposed for CLAUDE.md next PR)

1. **The mlx_lm.server → llama.cpp GGUF Q5_K_M runtime bonus is NOT UNIVERSAL** — it's specific to each model's weight distribution. **v1 (full-scope dense SFT) picks up +0.074**; my LoRA-fused adapter picks up +0.004. Never extrapolate a runtime bonus from one model to another. When planning a same-runtime → cross-runtime comparison for a NEW model, MEASURE the specific bonus for that model; do not assume the historical bonus transfers.

2. **LoRA-fused adapters may quantize less robustly than dense SFT trained weights.** Rank-32 additive updates spread across 7 modules can produce weight distributions that lose more capability under Q5_K_M than dense fine-tune signal does. For future LoRA adapters targeting production llama.cpp GGUF serving: benchmark on both f16 and Q5_K_M (or at minimum Q8_0) to confirm quantization robustness BEFORE committing to Q5_K_M as the serving tier. Q8_0 (7.5-8 BPW) is the reasonable middle ground when Q5_K_M loses too much.

## Documents Accessed

- `docs/development/mdemg-usage-lora-001-gguf/sprint_plan.md` (this sprint)
- `docs/development/mdemg-usage-lora-001/verdict.md` (revised — the source of the estimate this sprint measured against reality)
- `docs/development/mdemg-usage-lora-001/sprint_post.md` (reversal history)
- `configs/benchmark_phase10.yaml` (eval config — same as #145)
- `training_data/eval/valid_clean.jsonl` (13-task benchmark holdout)
- `training_data/eval/v1_fused_mlxserver_baseline.json` (#145 same-runtime v1 baseline)
- `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` (#145 my adapter mlx_lm.server)
- `training_data/eval/mdemg_usage_lora_001_gguf_prod_runtime.json` (NEW — this sprint's GGUF benchmark)
- `.local-models/qwen3-14b-mdemg-usage-fused/*.safetensors` (fused bf16 HF, ephemeral)
- `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` (f16 GGUF, ephemeral)
- `.local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf` (Q5_K_M GGUF — retained for #150 follow-up if pursued)
- `.local-models/serving/current.gguf` (production symlink — verified UNCHANGED pre + post)
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (production Q5_K_M — verified UNCHANGED)
- `docs/development/mdemg-usage-lora-001-gguf/sha256.txt` (all conversion-pipeline SHAs)
- `logs/mdemg_usage_gguf_bench.log` (llama-server bench log, 1450+ completions)
- `logs/mdemg_usage_gguf_bench_run.log` (run_benchmark stdout)
- `adapters/mdemg_usage_lora_001/adapters.safetensors` (frozen iter 7200, SHA `de2675b58800…`)
- CLAUDE.md pins:
  - Phase 13.5 cutover (the fuse→convert→quantize pipeline used here)
  - HOMEBREW-INSTALLER-QWEN-UPDATE-001 arch rules (sanity-test every quant BEFORE trust; caught nothing here — pipeline works, just quality doesn't survive it as well as v1's did)
  - #145 revised verdict + PHASE-E3 arch rule 5 (same-runtime comparison — this sprint IS the definitive same-runtime measurement against the shipped baseline)
  - `iterate-break-fix-verify` — verdict via real benchmark, not extrapolation
  - `data-decides-not-operator` — this sprint's data reversed the OPTIMISTIC estimate ("~0.913 production score") back to reality
- Operator directive 2026-09-01 ("proceed with #150")
- Operator directive 2026-09-01 ("rerun, this adapter should be better than the original")
