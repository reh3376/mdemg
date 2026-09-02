# MDEMG-USAGE-LORA-001-Q8 — Verdict

**Sprint**: MDEMG-USAGE-LORA-001-Q8 (task #151)
**Shipped**: 2026-09-02 (Q8_0 quantize 14s + benchmark ~2.6h wall-clock)
**Verdict**: **❌ CONFIRM-NO-PROMOTE. Q8_0 aggregate 0.8417 vs Q5_K_M 0.8426 = −0.0009 (pure noise). Quantization ceiling on llama.cpp is ~0.842 regardless of tier. MDEMG-USAGE-LORA-001 arc DEFINITIVELY CLOSED. v1 stays production.**

## Result — FOUR-WAY comparison (this sprint's core deliverable)

| Setup | Runtime | Aggregate | Δ vs v1 mlx | Δ vs baseline (0.9188) |
|---|---|---:|---:|---:|
| v1 fused GGUF Q5_K_M (shipped) | llama.cpp | **0.9188** | +0.0739 | 0 |
| v1 fused | mlx_lm.server | 0.8449 | 0 | −0.0739 |
| mdemg_usage_lora_001 | mlx_lm.server | 0.8388 | −0.0061 | −0.0800 |
| mdemg_usage_lora_001 GGUF Q5_K_M | llama.cpp | 0.8426 | −0.0023 | −0.0762 |
| **mdemg_usage_lora_001 GGUF Q8_0 ← this sprint** | **llama.cpp** | **0.8417** | **−0.0032** | **−0.0771** |

**Load-bearing finding: Q8_0 delta vs Q5_K_M is −0.0009 — pure noise.** More precise quantization (8.50 BPW vs 5.69 BPW) does NOT recover the runtime bonus v1 gets. This confirms Sprint #150's second hypothesis: the quantization sensitivity is BROADER than Q5_K_M-specific.

## Per-task detail — 4-way (sorted by q8-vs-v1_mlx ascending)

| Task | v1_mlx | me_mlx | me_q5 | me_q8 | q8-v1_mlx |
|---|---:|---:|---:|---:|---:|
| **retrieval.query_classify** | 0.7750 | 0.6750 | 0.7150 | 0.7000 | **−0.0750** |
| claude.code_knowledge | 0.3867 | 0.3628 | 0.3756 | 0.3698 | −0.0170 |
| ape.reflect | 0.9547 | 0.9377 | 0.9367 | 0.9407 | −0.0140 |
| jiminy.synthesize | 0.8844 | 0.8722 | 0.8762 | 0.8732 | −0.0112 |
| hidden.name_emergence | 0.9500 | 0.9500 | 0.9500 | 0.9500 | +0.0000 |
| hidden.summarize | 0.9000 | 0.9000 | 0.9000 | 0.9000 | +0.0000 |
| jiminy.codegen | 1.0000 | 1.0000 | 1.0000 | 1.0000 | +0.0000 |
| jiminy.evaluate | 0.9667 | 0.9667 | 0.9667 | 0.9667 | +0.0000 |
| retrieval.rerank_cross | 0.9000 | 0.9000 | 0.9000 | 0.9000 | +0.0000 |
| consulting.classify | 0.8827 | 0.8973 | 0.8940 | 0.8973 | **+0.0147** |
| jiminy.evaluate_llm | 0.7667 | 0.7833 | 0.7833 | 0.7833 | **+0.0167** |
| retrieval.intent_translate | 0.9780 | 0.9985 | 0.9940 | 1.0000 | **+0.0220** |
| hidden.reclassify | 0.9400 | 1.0000 | 1.0000 | 1.0000 | **+0.0600** |

**Group means**: C **+0.0036**, J **+0.0056**, T −0.0106 (dragged by claude.code_knowledge + retrieval.query_classify).

Non-monotonicity on retrieval.query_classify: mlx 0.6750 → Q5 0.7150 → Q8 0.7000. Q5's quantization noise happened to move this task 4pp in my adapter's favor; Q8's more-precise quantization partially reverses. Noise-level, not signal.

## Interpretation — where the −0.077 comes from

The **fuse step** (`mlx_lm.fuse --dequantize` on the 4-bit MLX base + LoRA adapter → bf16 HF safetensors) is the load-bearing lossy step. v1's Phase 5 SFT baked signal DIRECTLY into the (dequantized then requantized) weights; my adapter's rank-32 LoRA signal was ADDED via matrix multiplication over already-quantized 4-bit weights, then dequantized to bf16, then re-quantized to GGUF. Each conversion introduces error, and my LoRA signal (low-rank additive perturbation) is more fragile under compounded quantization noise than v1's full-magnitude SFT signal.

Q5_K_M (5.69 BPW) vs Q8_0 (8.50 BPW) is only the FINAL step of the pipeline. If the loss happens at fuse-time (upstream of both quant tiers), no downstream quantization choice will recover it.

**No further quantization tier will help.** Options that MIGHT: (a) native adapter-GGUF serving via llama.cpp `--lora` flag (skips the fuse+re-quantize cycle entirely, keeps LoRA logic in llama.cpp), (b) retrain with `--fp16` full-precision base + fuse (skip the initial 4-bit stage), (c) accept the ~0.076 gap. Options (a) and (b) are their own multi-day sprints.

## Verdict rubric (from sprint plan §5 Epic 4)

- ✅ PROMOTE-VIA-Q8 if aggregate ≥ 0.9088 → **NOT MET** (0.8417 < 0.9088)
- ⚠️ MIXED if in [0.89, 0.9088) → **NOT MET**
- **❌ CONFIRM-NO-PROMOTE** if < 0.89 → **THIS**. Broader quantization sensitivity confirmed. Arc closed.

## MDEMG-USAGE-LORA-001 arc — DEFINITIVELY CLOSED

Three sprints, three verdicts:

| Sprint | Task | Verdict | Aggregate |
|---|---|---|---:|
| MDEMG-USAGE-LORA-001 (initial) | #145 | ❌ FAIL (invalid cross-runtime) | 0.8388 vs 0.9188 |
| MDEMG-USAGE-LORA-001 (revised) | #145 | ⚠️ PARITY (same-runtime, incomplete) | 0.8388 vs 0.8449 = tied |
| MDEMG-USAGE-LORA-001-GGUF | #150 | ❌ NO-PROMOTE (Q5_K_M) | 0.8426 vs 0.9188 |
| **MDEMG-USAGE-LORA-001-Q8** | **#151** | **❌ CONFIRM-NO-PROMOTE (Q8_0)** | **0.8417 vs 0.9188** |

**Adapter capability IS at parity with v1** on same-runtime (confirmed 3× — mlx, GGUF Q5, GGUF Q8 all sit at v1 mlx same-runtime ± 0.006). **Adapter does NOT survive the shipped fuse+quantize pipeline** at any tier tested. v1 (`mdemg-llm-v1`) stays production. Substrate-side path (MDEMG-DOCS-INGEST-001 + RETRIEVAL-META-DOC-SUPPRESSION-001) remains the recommended MDEMG-usage alternative.

## What ships

- `.local-models/qwen3-14b-mdemg-usage-fused.Q8_0.gguf` — 15 GB Q8_0 GGUF (SHA `1df8f48d5ad3…`, local-only per gitignore)
- `training_data/eval/mdemg_usage_lora_001_q8_prod_runtime.json` — full benchmark output
- `docs/development/mdemg-usage-lora-001-q8/{sprint_plan,verdict,sprint_post,sha256}.md/.txt`
- CHANGELOG.md Unreleased entry
- PR summary comment

**Production llama-server on port 8102 UNTOUCHED throughout** — verified pre + during + post.

## Follow-ups filed

### 🟢 MDEMG-USAGE-LORA-001-ADAPTER-GGUF (research, optional, low-priority)

Try serving via llama.cpp's `--lora` flag with an adapter-GGUF (skips fuse+re-quantize, keeps LoRA logic in llama.cpp runtime). If this closes the gap, MDEMG-USAGE-LORA-001 adapter becomes shippable via a different serving path. Would need to test the shipped MODEL-DIST-002 adapter-GGUF format for this adapter + verify llama.cpp `--lora` compat with Qwen3-14B-4bit base. Deferred; only pursue if operator wants one more probe.

### 🟢 MDEMG-USAGE-LORA-001-FULL-PRECISION-RETRAIN (theoretical, deferred)

Retrain from full-precision (fp16) base instead of 4-bit MLX base. Would skip the initial quantization step, potentially yielding a fuse+quantize pipeline that preserves capability better. Wall-clock: ~200h+ retrain + ~2h eval. Not recommended unless operator has strong architectural reason to pursue.

### ✅ **ARC CLOSED**: no promote; substrate-side path recommended.

## Two arch rules pinned (proposed for CLAUDE.md next PR)

1. **Adapter quantization sensitivity is BROADER than Q5_K_M-specific for LoRA-fused adapters.** Q8_0 (8.50 BPW, 1.5× more precision) yields Δ = −0.0009 vs Q5_K_M on this adapter — pure noise. The ~0.076 quality loss vs v1's shipped runtime happens UPSTREAM of the Q5_K_M/Q8_0 choice, likely at the `mlx_lm.fuse --dequantize` step where the LoRA-modified 4-bit MLX base is dequantized to bf16 HF for the HF-to-GGUF converter. Don't reach for Q8_0 as a rescue for a Q5_K_M failure; if Q5_K_M loses quality on a LoRA-fused adapter, Q8_0 will too. The rescue path (if one exists) is upstream: adapter-only GGUF via llama.cpp `--lora` (bypasses fuse+re-quantize), or retrain from fp16 base (skips the initial 4-bit quantization).

2. **Adapter QUALITY parity is DIFFERENT from adapter SHIPPABILITY.** MDEMG-USAGE-LORA-001 confirmed capability parity 3× (mlx, GGUF Q5, GGUF Q8 all at v1 mlx same-runtime ± 0.006). The adapter LEARNED the tasks. But NONE of those runtime configurations produce a model that ships at ≥ v1's shipped-baseline aggregate (0.9188). Capability parity in bench ≠ production shippability when the shipping path involves lossy transformations that don't affect the reference model. Future LoRA arcs should measure the ENTIRE shipping-path pipeline (fuse → convert → quantize → serve on production runtime) BEFORE declaring capability parity as a promote-ready outcome.

## Documents Accessed

- `docs/development/mdemg-usage-lora-001-q8/sprint_plan.md` (this sprint)
- `docs/development/mdemg-usage-lora-001-gguf/{sprint_plan,verdict,sprint_post}.md` (#150 predecessor — the Q5_K_M half of this quant-tier study)
- `docs/development/mdemg-usage-lora-001/{sprint_plan,verdict,sprint_post}.md` (#145 grandparent — adapter capability establishment)
- `configs/benchmark_phase10.yaml` (unchanged eval config)
- `training_data/eval/valid_clean.jsonl` (unchanged benchmark)
- `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` (mlx baseline for my adapter, from #145)
- `training_data/eval/v1_fused_mlxserver_baseline.json` (v1 mlx same-runtime, from #145)
- `training_data/eval/mdemg_usage_lora_001_gguf_prod_runtime.json` (Q5_K_M GGUF, from #150)
- `training_data/eval/mdemg_usage_lora_001_q8_prod_runtime.json` (NEW — this sprint's Q8_0 GGUF)
- `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` (from #150, used as Q8_0 source)
- `.local-models/qwen3-14b-mdemg-usage-fused.Q8_0.gguf` (NEW — this sprint's Q8_0 GGUF)
- `.local-models/serving/current.gguf` (production symlink, verified UNCHANGED)
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (production Q5_K_M, verified UNCHANGED)
- `docs/development/mdemg-usage-lora-001-q8/sha256.txt` (Q8_0 SHA)
- `logs/mdemg_usage_q8_bench.log` (llama-server bench log, 1450 completions)
- `logs/mdemg_usage_q8_bench_run.log` (run_benchmark stdout)
- `/opt/homebrew/bin/llama-quantize` + `/opt/homebrew/bin/llama-server`
- `neural/.venv/bin/python -m neural.benchmarks.run_benchmark`
- CLAUDE.md pins:
  - Phase 13.5 cutover (production runtime lineage)
  - HOMEBREW-INSTALLER-QWEN-UPDATE-001 (fuse → convert → quantize pipeline reference)
  - #145 revised verdict + PHASE-E3 arch rule 5 (same-runtime comparison requirement)
  - #150 arch rules (runtime-bonus-is-model-specific + LoRA-may-quantize-poorly)
  - `iterate-break-fix-verify`, `data-decides-not-operator`, `live-testing-tier-required`
- Operator directive 2026-09-01 ("proceed with #151")
- Operator directive 2026-09-01 ("rerun, this adapter should be better than the original. Review the process to ensure you are acting on accurate and reliable data.") — VERIFIED CORRECT on same-runtime capability; production-runtime shipping does not follow from capability parity for this LoRA-fused adapter
