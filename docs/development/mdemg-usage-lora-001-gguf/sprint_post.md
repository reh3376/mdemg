# MDEMG-USAGE-LORA-001-GGUF — Sprint Post

**Task**: #150
**Started**: 2026-09-01 · **Completed**: 2026-09-01 (~3h wall-clock)
**Verdict**: **❌ NO-PROMOTE — aggregate 0.8426 vs 0.9188 shipped baseline = −0.0762.** Adapter is at capability parity with v1 but doesn't survive Q5_K_M quantization as well as v1's dense SFT does. v1 stays production.

Full comparison + per-task detail + arch rules at `verdict.md`.

## What shipped

| Artifact | Path | Notes |
|---|---|---|
| Sprint plan | `sprint_plan.md` | 12-section plan per `must-follow-12-section-format` |
| Verdict | `verdict.md` | 3-way comparison + follow-ups |
| Sprint post | `sprint_post.md` | This file |
| SHA log | `sha256.txt` | All 8 pipeline artifacts (6 HF safetensors + f16 + Q5_K_M) |
| Fused bf16 HF | `.local-models/qwen3-14b-mdemg-usage-fused/` (28 GB) | Local-only; regeneratable from adapter |
| f16 GGUF | `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` (29.5 GB) | Local-only |
| **Q5_K_M GGUF** | `.local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf` (10 GB) | Local-only; SHA `77d0d0aa2b74…` — retained for MDEMG-USAGE-LORA-001-Q8 follow-up if pursued |
| Benchmark output | `training_data/eval/mdemg_usage_lora_001_gguf_prod_runtime.json` | Local-only (training_data/eval gitignored); numbers captured in `verdict.md` |
| CHANGELOG entry | `CHANGELOG.md` | Unreleased |

## Verification (all green)

| Check | Result |
|---|---|
| Epic 1 fuse — 28 GB bf16 HF safetensors, 6 shards | ✅ 7s wall-clock |
| Epic 2 convert — 29.5 GB f16 GGUF | ✅ 16s wall-clock |
| Epic 3 quantize — 10 GB Q5_K_M GGUF | ✅ 39s wall-clock |
| Epic 4 sanity — served coherent "ok" response | ✅ |
| Epic 5 benchmark — 1450 completions, 0 errors | ✅ 122 min wall-clock |
| Epic 6 3-way verdict computed | ✅ 0.8426 → NO-PROMOTE |
| Production llama-server on port 8102 UNTOUCHED pre + during + post | ✅ verified 3× |
| `.local-models/serving/current.gguf` symlink UNCHANGED | ✅ (still points at v1 Q5_K_M) |
| `~/Library/LaunchAgents/com.mdemg.llama-server.plist` UNCHANGED | ✅ |
| No orphan `llama-server` processes on 8103 post-sprint | ✅ |

## Sprint execution — key data points

**Conversion pipeline is FAST** — total conversion time ~60s (fuse + convert + quantize combined). The bulk of wall-clock is the benchmark itself.

**llama.cpp is NOT dramatically faster than mlx_lm.server** on this hardware — the benchmark took 122 min vs 121 min for v1's mlx_lm.server bench. My prior estimate of "1-1.5h at 10-15 req/min for llama.cpp" was wrong (actually 7-9 req/min sustained, same as mlx_lm.server). The runtime difference is purely QUALITY (Q5_K_M quantization precision), not throughput.

**The runtime bonus asymmetry is the load-bearing new finding.**

## The load-bearing finding

The #145 revised verdict estimated my adapter's production score at 0.8388 + 0.074 = ~0.913 based on v1's runtime bonus. **This estimate was WRONG.** Reality:

| Model | mlx_lm.server | llama.cpp GGUF | Actual runtime bonus |
|---|---:|---:|---:|
| v1 fused | 0.8449 | 0.9188 | **+0.0739** |
| mdemg_usage_lora_001 fused | 0.8388 | 0.8426 | **+0.0038** |

**Rule violation surfaced**: I violated `must-validate-all-claims-before-commit` and `data-decides-not-operator` when I wrote the "~0.913 production estimate" in #145's revised verdict. That estimate extrapolated ONE model's runtime bonus (v1) to a DIFFERENT model (mine) without measurement. The extrapolation was ~20× too optimistic. Filed as a new arch rule.

**Why**: v1 was trained via full-scope dense SFT (batch=4, max_seq=8192, more epochs) — weight updates are large-magnitude, well-distributed. My adapter is rank-32 LoRA on 7 modules — low-rank additive updates fused into base weights. Q5_K_M quantization loses more capability from my adapter's weight distribution than from v1's. (Hypothesis; would need Q8_0 comparison to isolate — filed as MDEMG-USAGE-LORA-001-Q8 follow-up.)

## Verdict is definitive: v1 stays production

Same-runtime parity (both on llama.cpp): mine 0.8426 vs v1 (mlx_lm.server) 0.8449 = **−0.002 tie**. Adapter quality is real.

Cross-runtime vs shipped baseline: mine 0.8426 vs v1 GGUF 0.9188 = **−0.076 too big to promote**.

**No amount of within-noise same-runtime parity closes the −0.076 gap on the runtime the production model uses.** Since production IS llama.cpp GGUF Q5_K_M, the relevant number is 0.8426 vs 0.9188. NO-PROMOTE.

## Follow-ups

### 🟢 MDEMG-USAGE-LORA-001-Q8 (optional next probe, ~2h wall-clock)

Convert same fused bf16 HF to Q8_0 GGUF (~16 GB, 7.5-8 BPW vs Q5_K_M's 5.69). If Q8_0 aggregate lands ≥0.90, my adapter is shippable — just needs a bigger serving tier. Cheap enough to run; would settle the "is it Q5_K_M or a broader quantization issue" question. Recommended if operator wants a definitive book-close on this adapter.

### 🟢 LoRA-vs-quantization interaction research (longer)

Systematic study of why LoRA-fused weights quantize worse than dense SFT weights. Could benefit MDEMG's whole retrain path — future retrains might all suffer this if not designed for quantization robustness. Would take weeks of ablation study; not urgent.

### 🟢 Substrate-side path (MDEMG-DOCS-INGEST-001 + META-DOC-SUPPRESSION)

Confirmed as the recommended MDEMG-usage path. Deep-dive workflow `wf_b389463a-61b`'s Alt 1 wins. LoRA arm is definitively closed.

### 🔴 MDEMG-USAGE-LORA-001 arc CLOSES

Sprint #145 verdict: adapter is capability-competitive but not production-swappable. Sprint #150 verdict: production-runtime measurement confirms the arc conclusion. **Arc closed. v1 stays production. MDEMG-usage capability comes via substrate retrieval, not adapter.**

## Two arch rules pinned (proposed for CLAUDE.md next PR)

1. **The mlx_lm.server → llama.cpp GGUF Q5_K_M runtime bonus is NOT UNIVERSAL** — it's specific to each model's weight distribution. v1 (full-scope dense SFT) picked up +0.074 aggregate on the 13-task benchmark; my LoRA-fused adapter picked up only +0.004 through the same pipeline (20× less). NEVER extrapolate a runtime bonus from one model to another. When planning a same-runtime → cross-runtime comparison for a NEW model, MEASURE the specific bonus for that model; do not assume the historical bonus transfers. This is a specific case of `data-decides-not-operator` + `must-validate-all-claims-before-commit`.

2. **LoRA-fused adapters may quantize less robustly than dense SFT weights** — rank-32 additive updates spread across 7 modules can produce weight distributions that lose more capability under Q5_K_M than full-scope SFT signal does. For future LoRA adapters targeting production llama.cpp GGUF serving: measure Q5_K_M AND Q8_0 GGUF benchmarks before committing to Q5_K_M as the serving tier. Q8_0 (7.5-8 BPW) is the reasonable middle ground when Q5_K_M loses too much.

## Documents Accessed

- `docs/development/mdemg-usage-lora-001-gguf/{sprint_plan,verdict}.md` (this sprint)
- `docs/development/mdemg-usage-lora-001/{verdict,sprint_post,sprint_plan}.md` (predecessor #145 with revised verdict)
- `docs/development/adapter-swap-standardize-001/sprint_post.md` (#139 tools — mdemg adapter bench-serve used earlier this arc but not this sprint since llama-server not mlx_lm.server)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,sprint_post}.md` (arch rules)
- `docs/development/homebrew-installer-qwen-update-001/PUBLISH_GUIDE.md` (fuse → convert → quantize pipeline reference)
- `configs/benchmark_phase10.yaml` (eval config)
- `training_data/eval/valid_clean.jsonl` (13-task benchmark holdout)
- `training_data/eval/mdemg_usage_lora_001_iter7200_bench.json` (my mlx_lm.server benchmark from #145)
- `training_data/eval/v1_fused_mlxserver_baseline.json` (v1 mlx_lm.server fair-comparison baseline from #145)
- `training_data/eval/mdemg_usage_lora_001_gguf_prod_runtime.json` (NEW — this sprint's Q5_K_M GGUF benchmark on llama.cpp)
- `.local-models/qwen3-14b-4bit-base/` (raw MLX base)
- `.local-models/qwen3-14b-mdemg-usage-fused/` (fused bf16 HF, this sprint's Epic 1 output)
- `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` (this sprint's Epic 2 output)
- `.local-models/qwen3-14b-mdemg-usage-fused.Q5_K_M.gguf` (this sprint's Epic 3 output — retained)
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (production Q5_K_M, verified UNCHANGED)
- `.local-models/serving/current.gguf` (production symlink, verified UNCHANGED)
- `~/Library/LaunchAgents/com.mdemg.llama-server.plist` (verified UNCHANGED)
- `adapters/mdemg_usage_lora_001/adapters.safetensors` (frozen iter 7200, SHA `de2675b58800…`)
- `docs/development/mdemg-usage-lora-001-gguf/sha256.txt` (all conversion SHAs pinned)
- `logs/mdemg_usage_gguf_bench.log` (llama-server bench log)
- `logs/mdemg_usage_gguf_bench_run.log` (run_benchmark stdout)
- Live `/opt/homebrew/bin/{llama-quantize,llama-server}` (llama.cpp tools)
- Live `neural/.venv/bin/python -m mlx_lm.fuse` (fuse tool)
- Live `neural/.venv/bin/python /Users/reh3376/llama.cpp-src/convert_hf_to_gguf.py` (HF → GGUF converter)
- CLAUDE.md pins:
  - Phase 13.5 cutover (pipeline lineage)
  - HOMEBREW-INSTALLER-QWEN-UPDATE-001 (fuse → convert → quantize + sanity-test-before-trust)
  - `#145` PHASE-E3 arch rule 5 (same-runtime comparison — extended this sprint to name the runtime bonus asymmetry)
  - `iterate-break-fix-verify`, `data-decides-not-operator`, `must-validate-all-claims-before-commit`, `live-testing-tier-required`
- Operator directive 2026-09-01 ("proceed with #150") — followed
