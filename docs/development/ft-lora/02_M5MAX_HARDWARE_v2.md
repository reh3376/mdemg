# M5 Max Hardware-Specific Configuration

**Date:** 2026-04-21 (v5.0 — Qwen3.6-35B-A3B upgrade + asymmetric quant per memo 07 v3.1)
**Hardware:** Apple M5 Max — 18-core CPU (6 super + 12 performance), 40-core GPU with Neural Accelerators, 128GB unified memory, 614 GB/s bandwidth, Thunderbolt 5, 2TB internal SSD + unlimited external SSD
**Model:** Qwen3.6-35B-A3B MoE (Apache 2.0, released 2026-04-16, 35B total / 3B active per token, 256 experts = 8 routed + 1 shared)
**Fallback:** Qwen3.5-35B-A3B (Apache 2.0, mature) — NOT Qwen3-30B-A3B (superseded)
**Constraint:** Single model for all 16 MDEMG generative LLM tasks

> **Tool-use exclusion:** MDEMG bans tool-calling across the entire stack. See [`01_RESEARCH_v2.md §2.8`](01_RESEARCH_v2.md) — No-Tool-Calling Architectural Policy. The target must be a base/instruct variant without tool-use training. Sprint A Epic 10 grep-audits nine banned patterns including `preserve_thinking` (multi-turn agent hook, default must not be changed).

---

## Changes in v5.0 (per memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1 — 2026-04-21)

1. **Base model**: Qwen3-30B-A3B → **Qwen3.6-35B-A3B** (see [`01_RESEARCH_v2.md §3`](01_RESEARCH_v2.md)).
2. **Throughput target** tightened from speculative "~80 tok/s" to **≥60 tok/s Sprint C gate 3** (measured on vllm-mlx + MXFP4_MOE routed experts, see §2).
3. **Asymmetric quantization** (see [`01_RESEARCH_v2.md §5.4`](01_RESEARCH_v2.md)) replaces uniform Q4_K_M:
   - Attention + shared-expert MLP: **BF16**
   - Routed-expert MLPs (all 255 routed per layer, with or without Tier 2 LoRA): **MXFP4_MOE**
   - Router/gate weights: **BF16**
4. **Two-tier LoRA training memory** profiled separately from uniform LoRA (§3 below). Tier 1 uses the full balanced 16-task mix; Tier 2 is per-family.

---

## 1. Why MoE over Dense (unchanged rationale)

Real-world benchmarks contradicted the v1.0 speed estimates for dense 30B+ models:

| Source | Model | Hardware | Speed |
|---|---|---|---|
| HuggingFace (@wolfram) | Qwen3-32B dense | M4 Max Q4 | <10 tok/s |
| InsiderLLM | Qwen3-32B dense | 48GB Mac Q4 | 15-22 tok/s |
| HuggingFace (@wolfram) | Qwen3-30B-A3B MoE | Apple MLX Q4 | ~64 tok/s |
| DeepNewz benchmark | Qwen3-30B-A3B MoE | M4 Max MLX Q4 8K ctx | 87.58 tok/s |

Qwen3.6-35B-A3B preserves the 3B-active-per-token economy (same as Qwen3-30B-A3B) despite 35B total parameters, because the routed-expert pool expanded from 128 to 255 while top-k-per-token stayed at 8. Throughput on M5 Max is expected comparable to or better than the Qwen3-30B-A3B benchmark figures above, validated in Sprint C.

---

## 2. Inference Performance on M5 Max

The M5 Max's 614 GB/s bandwidth (vs M4 Max ~546 GB/s) plus Neural Accelerators yield ~20-30% improvement over M4 Max benchmarks on the same MoE topology.

**Estimated Qwen3.6-35B-A3B on M5 Max (asymmetric quant per [`01_RESEARCH_v2.md §5.4`](01_RESEARCH_v2.md)):**

| Component | Precision | Size |
|---|---|---|
| Shared expert MLP (1 per layer × all layers) | BF16 | ~4.5GB |
| Attention (q/k/v/o_proj × all layers) | BF16 | ~2.5GB |
| Routed expert MLPs (255 per layer × all layers) | MXFP4_MOE | ~13.5GB |
| Router/gate weights | BF16 | ~0.4GB |
| **Total on-disk quantized** | — | **~20.9GB** |

| Config | Generation speed | TTFT (2K prompt) | Meets all tasks? |
|---|---|---|---|
| Asymmetric (BF16 shared + MXFP4_MOE routed) | **≥60 tok/s** (Sprint C gate 3) | <3s | ✅ |
| Uniform Q6_K fallback (Qwen3.5 path) | 55-75 tok/s | <3s | ✅ |

The ≥60 tok/s figure is a **gate**, not a measured value — Sprint C's three validation gates (mlx-lm-lora convergence on 500 examples, JSON ≥95% on 9 structured tasks, ≥60 tok/s) determine whether Qwen3.6 ships or we fall back to Qwen3.5-35B-A3B.

**vllm-mlx prefix caching bonus:** MDEMG's 16 tasks share system prompts per task type (one per task, 14 unique system prompts per FT-OAI-002 Epic 4). With prefix caching, repeated task calls skip the system prompt prefill entirely — 5.8x TTFT improvement on cached prefixes.

**MTP speculative decoding (Qwen3.6-only bonus):** Qwen3.6 supports Multi-Token Prediction. On long think-mode generations (T-group in §1.1), MTP can add 15-30% throughput with no quality loss. vllm-mlx MTP support lands in the version pinned by Sprint E — if unavailable at Sprint C, the ≥60 tok/s gate is measured without MTP.

---

## 3. Memory Budget

### Inference Mode

| Service | Memory |
|---|---|
| macOS + desktop | ~8GB |
| Neo4j Docker (34K+ nodes) | ~6GB |
| TimescaleDB Docker | ~1-2GB |
| MDEMG Go server | ~1GB |
| Neural sidecar (3 cross-encoders) | ~2GB |
| Claude Code | ~2GB |
| vllm-mlx + Qwen3.6-35B-A3B asymmetric quant | **~21GB** |
| **Total used** | **~41-42GB** |
| **Available for KV cache + context** | **~86-87GB** |

86-87GB of KV cache supports context windows well beyond 128K tokens for a 3B-active MoE. Qwen3.6's 262K native context fits comfortably; YaRN extension to 1M is viable but not a Sprint A commitment.

### Training Mode — Tier 1 (Offline — All Services Stopped)

Tier 1 = attention + shared expert, r=32, all 16 tasks balanced. See [`01_RESEARCH_v2.md §5.1`](01_RESEARCH_v2.md).

| Component | Memory |
|---|---|
| macOS baseline | ~4GB |
| Qwen3.6-35B-A3B bf16 base (attention + shared expert unfrozen) | ~62GB |
| Tier 1 LoRA adapters (r=32, attention + shared expert) | ~0.6GB |
| Routed experts frozen (MXFP4_MOE, held for routing) | ~13.5GB |
| Training batch + gradients (batch=4, seq=8192) | ~25-35GB |
| **Total** | **~105-115GB** |
| **Headroom** | **~13-23GB** |

### Training Mode — Tier 2 (Per-Family, Sequential)

Tier 2 = top-25% routed experts per family, r=8. Runs **after** Tier 1 merges. Attention + shared expert are frozen during Tier 2.

| Component | Memory |
|---|---|
| macOS baseline | ~4GB |
| Qwen3.6-35B-A3B with Tier 1 merged, frozen | ~20.9GB |
| Top-25% routed experts unfrozen (BF16 during training) | ~22GB |
| Tier 2 LoRA adapters (r=8, top-25% experts of one family) | ~0.15GB |
| Training batch + gradients (batch=4, seq=8192) | ~20-28GB |
| **Total** | **~67-75GB** |
| **Headroom** | **~53-61GB** |

Tier 2 is substantially lighter than Tier 1 because most of the model stays quantized and frozen. One Tier 2 run per family (3 families → 3 sequential runs).

### Concurrent Inference + Tier 2 Training (supported)

Tier 2's ~67-75GB footprint leaves room for vllm-mlx inference (~21GB) alongside it:

| Component | Memory |
|---|---|
| macOS + services (Neo4j, TSDB, MDEMG, sidecar) | ~20GB |
| vllm-mlx + Qwen3.6 asymmetric inference | ~21GB |
| Tier 2 training (one family at a time) | ~55GB |
| **Total** | **~96GB** |
| **Headroom** | **~32GB** |

Tier 1 training cannot run concurrently with inference — it touches the shared expert that inference also needs. Tier 2 can, because different experts are trained than the ones inference typically hits (top-25% per family partition).

**Training time estimates (9,400 examples, 3 epochs, batch=4):**
- Tier 1 (attention + shared expert, all 16 tasks): ~5-8h
- Tier 2 (per family, top-25% experts): ~2-4h × 3 families = ~6-12h
- Total Tier 1 + all 3 Tier 2: ~11-20h

`mlx-lm-lora` has a 12x MoE training speed optimization with 35% less memory than standard implementations. Sprint E adds `router_aux_loss_coef` exposure (memo §3.5, set to 0.002) and per-module-class quantization selectors for `mlx_lm.convert` (memo §3.8).

---

## 4. Inference Server: vllm-mlx

### Why vllm-mlx

| Feature | Custom generator.py (v1.0) | vllm-mlx (v5.0) |
|---|---|---|
| API compatibility | Custom format | OpenAI + Anthropic compatible |
| Prefix caching | Not implemented | 5.8x TTFT speedup |
| Continuous batching | Not implemented | 3.7x throughput at 16 concurrent |
| Paged KV cache | Not implemented | SSD-tiered cold cache |
| Think mode parser | Manual regex | Native Qwen3 reasoning parser |
| LoRA adapter swap | Custom reload logic | Built-in; Tier 1 + Tier 2 adapters stackable |
| Asymmetric quant (BF16 + MXFP4_MOE) | N/A | Per memo §3.8, Sprint E patch |
| MTP speculative decoding | N/A | Qwen3.6 MTP head support |
| Production maturity | New code, untested | EuroMLSys '26 paper, used with Claude Code |

### Setup (post-Sprint E; target config)

```bash
# Install
uv tool install git+https://github.com/waybarrios/vllm-mlx.git

# Serve Qwen3.6-35B-A3B with asymmetric quant and Tier 1 + Tier 2 adapters merged
vllm-mlx serve mlx-community/Qwen3.6-35B-A3B-mxfp4-moe \
  --port 8100 \
  --continuous-batching \
  --use-paged-cache \
  --reasoning-parser qwen3 \
  --adapter-stack tier1_universal.safetensors,tier2_reasoning_think.safetensors \
  # NO --tool-call-parser, NO --enable-auto-tool-choice (banned per 01 §2.8)

# MDEMG config — existing OpenAI provider pointed at vllm-mlx
LLM_PROVIDER=openai
LLM_BASE_URL=http://localhost:8100/v1
LLM_MODEL=default
# NOTE: preserve_thinking is NOT set — stays at default per 01 §2.8 rule 5
```

No new Go provider code needed. `llmclient`'s existing OpenAI path works directly. Sprint FT-LORA-B audits all launch commands and adapter configs against the nine banned tool-calling patterns.

---

## 5. Storage Strategy

| Item | Location | Size |
|---|---|---|
| Base model weights (bf16) | Internal SSD | ~70GB (Qwen3.6-35B-A3B; downloaded once) |
| Quantized inference model (asymmetric) | Internal SSD | ~21GB |
| Tier 1 LoRA adapter (per version) | Internal SSD | ~600MB each |
| Tier 2 LoRA adapters (per family × version) | Internal SSD | ~150MB × 3 families = ~450MB per version |
| Training data (TimescaleDB) | Internal SSD (Docker volume) | ~2-3GB (6 months) |
| Training data (JSONL) | Internal SSD | ~500MB-1GB (6 months) |
| Curated datasets (per version) | Internal SSD | ~20-50MB each |
| Routing profile artifacts (Sprint D) | Internal SSD | ~5MB per profile × 3 families |
| Model archive (old versions) | External SSD (TB5) | ~21GB per version |
| Training checkpoints | External SSD | ~2-4GB per checkpoint |

Internal SSD budget: ~105GB. Well within 2TB.

---

## 6. Think Mode and Task Groups

Qwen3.6's `/think` and `/no_think` modes align to the three sampling groups (memo §3.3; canonical definitions in [`01_RESEARCH_v2.md §1.1`](01_RESEARCH_v2.md) and the full sampling recipe table lives in `04_BENCHMARK_RL_v2.md`):

| Mode | Group | Tasks (from §1.1 group column) | Target speed |
|---|---|---|---|
| `/think` | **T** (7 tasks) | ape.reflect, consulting.synthesis, hidden.summarize, jiminy.synthesize, metalearn.generalize, retrieval.rerank_nli, summarize.generate | ~60 tok/s (reasoning overhead; MTP may lift this) |
| `/no_think` | **C** (6 tasks) | consulting.classify, hidden.reclassify, jiminy.evaluate, jiminy.codegen, retrieval.intent_translate, retrieval.query_classify | Full speed (target ≥60 tok/s; short output so wall-clock is dominated by TTFT) |
| `/no_think` | **J** (3 tasks) | hidden.name_emergence, jiminy.evaluate_llm, retrieval.rerank_cross | Full speed (≥60 tok/s; presence_penalty=1.5 avoids JSON repetition collapse) |

Implementation: `CompleteOpts.Think` boolean in each call site. vllm-mlx's `--reasoning-parser qwen3` handles the `<think>` block extraction automatically on the server side. The Go consumers receive clean responses after the `SanitizeResponse()` function strips any residual think blocks.

**`preserve_thinking` is NOT set on any call site** — default value only, per [`01_RESEARCH_v2.md §2.8`](01_RESEARCH_v2.md) rule 5. This parameter is a Qwen3.6 multi-turn agent hook; MDEMG's single-shot pattern must not enable it.
