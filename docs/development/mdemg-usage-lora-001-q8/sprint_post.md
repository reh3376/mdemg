# MDEMG-USAGE-LORA-001-Q8 — Sprint Post

**Task**: #151
**Completed**: 2026-09-02 (~2.5h wall-clock)
**Verdict**: **❌ CONFIRM-NO-PROMOTE — Q8_0 aggregate 0.8417 vs baseline 0.9188 = −0.077.** Q8_0 gives essentially the same result as Q5_K_M (−0.0009 delta = pure noise). Quantization sensitivity is BROADER than Q5_K_M-specific. **MDEMG-USAGE-LORA-001 arc DEFINITIVELY CLOSED.**

Full 4-way comparison + follow-ups at `verdict.md`.

## What shipped

| Artifact | Path | Notes |
|---|---|---|
| Sprint plan | `sprint_plan.md` | 12-section per `must-follow-12-section-format` |
| Verdict | `verdict.md` | 4-way comparison table |
| Sprint post | `sprint_post.md` | This file |
| SHA log | `sha256.txt` | Q8_0 GGUF SHA |
| **Q8_0 GGUF** | `.local-models/qwen3-14b-mdemg-usage-fused.Q8_0.gguf` (15 GB, SHA `1df8f48d5ad3…`) | Local-only; retained for MDEMG-USAGE-LORA-001-ADAPTER-GGUF probe if pursued |
| Benchmark output | `training_data/eval/mdemg_usage_lora_001_q8_prod_runtime.json` | Local-only (gitignored) |
| CHANGELOG entry | `CHANGELOG.md` Unreleased | |

## Verification

| Check | Result |
|---|---|
| Epic 1 Q8_0 quantize — 15 GB (8.50 BPW) | ✅ 14s wall-clock |
| Epic 2 sanity chat "ok" | ✅ coherent |
| Epic 3 benchmark — 1450 completions | ✅ 148 min wall-clock |
| Epic 4 4-way verdict computed | ✅ Q8_0 = 0.8417, Δ vs Q5_K_M = −0.0009 |
| Production 8102 UNTOUCHED pre + during + post | ✅ verified 3× |
| No orphan bench-serve on 8103 | ✅ port free |

## Key finding

**Q8_0 does NOT help.**

| Runtime/Quant | Aggregate | Δ vs shipped baseline (0.9188) |
|---|---:|---:|
| v1 fused GGUF Q5_K_M (shipped baseline) | 0.9188 | 0 |
| mine GGUF Q5_K_M | 0.8426 | −0.0762 |
| mine GGUF Q8_0 | 0.8417 | −0.0771 |
| **Q8_0 vs Q5_K_M delta** | **−0.0009** | **PURE NOISE** |

**The quantization ceiling for my adapter on llama.cpp is ~0.842 regardless of whether we quantize to Q5_K_M or Q8_0.** The loss happens UPSTREAM of the quant-tier choice, likely at the `mlx_lm.fuse --dequantize` step where the LoRA-modified 4-bit MLX base is dequantized to bf16 HF for the HF-to-GGUF converter.

## Interpretation

**v1's shipping-path advantage**: Phase 5 SFT baked signal DIRECTLY into the weights before any GGUF conversion. Its bf16 HF safetensors are dense-SFT natural weights that survive the HF→GGUF pipeline cleanly (v1 picks up +0.074 aggregate through Q5_K_M).

**My adapter's shipping-path disadvantage**: rank-32 LoRA on 7 modules is APPLIED via matrix-multiply over already-quantized 4-bit MLX weights. `mlx_lm.fuse --dequantize` produces bf16 HF that includes both the base's dequantization artifacts AND the LoRA's low-rank additive perturbations. Neither the fuse step nor either GGUF quant tier can recover the compounded loss.

**No further quantization tier will help** (Q4_K_M would be worse; Q8_0 is already 8.5 BPW). The rescue paths (research-scoped, not shipping-scoped) are:
- Native adapter-GGUF via llama.cpp `--lora` flag (skips fuse+re-quantize)
- Retrain from fp16 base (skips initial 4-bit quantization step)

## MDEMG-USAGE-LORA-001 arc CLOSED

| Sprint | Task | Verdict | Aggregate |
|---|---|---|---:|
| MDEMG-USAGE-LORA-001 (initial) | #145 | ❌ FAIL (invalid cross-runtime) | 0.8388 vs 0.9188 |
| MDEMG-USAGE-LORA-001 (revised) | #145 | ⚠️ PARITY (same-runtime, incomplete) | tied at ~0.844 |
| MDEMG-USAGE-LORA-001-GGUF | #150 | ❌ NO-PROMOTE (Q5_K_M) | 0.8426 vs 0.9188 |
| **MDEMG-USAGE-LORA-001-Q8** | **#151** | **❌ CONFIRM (Q8_0 = Q5_K_M within noise)** | **0.8417 vs 0.9188** |

**Adapter capability parity confirmed 3×** (mlx, GGUF Q5, GGUF Q8 all ± 0.006 from v1 mlx same-runtime).
**Adapter shippability confirmed NOT MET 2×** (Q5_K_M and Q8_0 both < 0.90 aggregate on shipped-runtime).

v1 (`mdemg-llm-v1`) stays production. Substrate-side path (MDEMG-DOCS-INGEST-001 + RETRIEVAL-META-DOC-SUPPRESSION-001) remains recommended for MDEMG-usage capability.

## Two arch rules pinned (proposed for CLAUDE.md next PR)

1. **Adapter quantization sensitivity is BROADER than Q5_K_M-specific for LoRA-fused adapters.** Q8_0 (8.50 BPW, 1.5× more precision) yields Δ = −0.0009 vs Q5_K_M on this adapter — pure noise. The loss happens UPSTREAM of Q5_K_M/Q8_0 choice. Don't reach for Q8_0 as a rescue for a Q5_K_M failure on a LoRA-fused adapter. If Q5_K_M loses quality, Q8_0 will too. Rescue paths (if pursued): adapter-only GGUF via llama.cpp `--lora` (bypasses fuse+re-quantize), or retrain from fp16 base (skips the initial 4-bit stage).

2. **Adapter QUALITY parity is DIFFERENT from adapter SHIPPABILITY.** This arc confirmed capability parity 3× yet the adapter is not shippable at any tested quant tier. Capability parity in benchmark ≠ production shippability when the shipping path involves lossy transformations that don't affect the reference model. Future LoRA arcs should measure the ENTIRE shipping-path pipeline (fuse → convert → quantize → serve on production runtime) BEFORE declaring capability parity as promote-ready.

## Follow-ups filed

- **MDEMG-USAGE-LORA-001-ADAPTER-GGUF** (deferred): try llama.cpp `--lora` serving with adapter-GGUF format (skips fuse+re-quantize). Would need MODEL-DIST-002 shape for this adapter + llama.cpp `--lora` compat verification. Deferred unless operator wants one more probe.
- **MDEMG-USAGE-LORA-001-FULL-PRECISION-RETRAIN** (theoretical, deferred): retrain from fp16 base to skip initial 4-bit quantization. ~200h+ wall-clock; not recommended.

## Documents Accessed

- `docs/development/mdemg-usage-lora-001-q8/{sprint_plan,verdict}.md` (this sprint)
- `docs/development/mdemg-usage-lora-001-gguf/{sprint_plan,verdict,sprint_post,sha256}.md/.txt` (#150 sibling)
- `docs/development/mdemg-usage-lora-001/{sprint_plan,verdict,sprint_post}.md` (#145 grandparent)
- `configs/benchmark_phase10.yaml`
- `training_data/eval/valid_clean.jsonl`
- `training_data/eval/{mdemg_usage_lora_001_iter7200_bench.json, v1_fused_mlxserver_baseline.json, mdemg_usage_lora_001_gguf_prod_runtime.json}` (predecessor benchmarks)
- `training_data/eval/mdemg_usage_lora_001_q8_prod_runtime.json` (NEW — Q8_0 benchmark)
- `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` (Q8_0 source, from #150)
- `.local-models/qwen3-14b-mdemg-usage-fused.Q8_0.gguf` (Q8_0 output — this sprint)
- `.local-models/serving/current.gguf` (production symlink, verified UNCHANGED)
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (production, verified UNCHANGED)
- `/opt/homebrew/bin/{llama-quantize,llama-server}`
- `neural/.venv/bin/python -m neural.benchmarks.run_benchmark`
- CLAUDE.md pins:
  - Phase 13.5 cutover
  - HOMEBREW-INSTALLER-QWEN-UPDATE-001 (fuse pipeline reference)
  - #145 PHASE-E3 arch rule 5 (same-runtime comparison)
  - #150 arch rules (runtime-bonus-is-model-specific + LoRA-may-quantize-poorly)
  - `iterate-break-fix-verify`, `data-decides-not-operator`, `live-testing-tier-required`
- Operator directive 2026-09-01 ("proceed with #151")
