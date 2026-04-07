# M5 Max Hardware-Specific Configuration

**Date:** 2026-04-07 (v4.0)
**Hardware:** Apple M5 Max — 18-core CPU (6 super + 12 performance), 40-core GPU with Neural Accelerators, 128GB unified memory, 614 GB/s bandwidth, Thunderbolt 5, 2TB internal SSD + unlimited external SSD
**Model:** Qwen3-30B-A3B MoE (Apache 2.0, 30B total / 3B active per token)
**Constraint:** Single model for all 16 MDEMG generative LLM tasks

> **Tool-use exclusion:** Qwen3-30B-A3B is a base MoE model without tool-use training. This is required — tool-use variants produce function-call JSON structures that break MDEMG's JSON-parsing consumers. If Qwen releases a "-Tool" variant, do not use it.

---

## 1. Why MoE over Dense

Real-world benchmarks contradicted the v1.0 speed estimates for Qwen3-32B dense:

| Source | Model | Hardware | Speed |
|---|---|---|---|
| HuggingFace (@wolfram) | Qwen3-32B dense | M4 Max Q4 | <10 tok/s |
| InsiderLLM | Qwen3-32B dense | 48GB Mac Q4 | 15-22 tok/s |
| HuggingFace (@wolfram) | Qwen3-30B-A3B MoE | Apple MLX Q4 | ~64 tok/s |
| DeepNewz benchmark | Qwen3-30B-A3B MoE | M4 Max MLX Q4 8K ctx | 87.58 tok/s |

Quality is identical: both score 82.20% on MMLU-Pro (Computer Science). The MoE model is 4-5x faster because only 3B of its 30B parameters activate per token.

---

## 2. Inference Performance on M5 Max

The M5 Max's 614 GB/s bandwidth (vs M4 Max ~546 GB/s) plus Neural Accelerators yield ~20-30% improvement over M4 Max benchmarks.

**Estimated Qwen3-30B-A3B on M5 Max:**

| Quantization | Model Size | Generation Speed | TTFT (2K prompt) | Meets All Tasks? |
|---|---|---|---|---|
| Q4_K_M | ~17GB | 80-100 tok/s | <2s | ✅ |
| Q6_K | ~22GB | 60-80 tok/s | <3s | ✅ |
| Q8_0 | ~30GB | 40-55 tok/s | <4s | ✅ |

All quantization levels meet every MDEMG latency requirement with massive headroom.

**vllm-mlx prefix caching bonus:** MDEMG's 16 tasks share system prompts per task type. With prefix caching, repeated task calls skip the system prompt prefill entirely — 5.8x TTFT improvement on cached prefixes.

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
| vllm-mlx + Qwen3-30B-A3B Q4 | ~22GB |
| **Total used** | **~42-43GB** |
| **Available for KV cache + context** | **~85-86GB** |

85-86GB of KV cache supports context windows well beyond 128K tokens for a 3B-active MoE.

### Training Mode (Offline — All Services Stopped)

| Component | Memory |
|---|---|
| macOS baseline | ~4GB |
| Qwen3-30B-A3B bf16 LoRA (r=32) | ~74GB |
| Training batch + gradients (batch=4, seq=8192) | ~30-40GB |
| **Total** | **~108-118GB** |
| **Headroom** | **~10-20GB** |

No production traffic means training can use the full 128GB. bf16 LoRA is the default quality path — no need for QLoRA compromises.

**Training time estimate (9,400 examples, 3 epochs, batch=4):** ~6-10 hours. MoE training is faster than dense because only 3B active parameters compute gradients per token. `mlx-lm-lora` has a 12x MoE training speed optimization with 35% less memory than standard implementations.

### Concurrent Inference + QLoRA Training (v3.0 Addition)

For routine retraining without stopping inference:

| Component | Memory |
|---|---|
| macOS + services (Neo4j, TSDB, MDEMG, sidecar) | ~20GB |
| vllm-mlx + Q4 inference | ~22GB |
| QLoRA training (4-bit base + LoRA adapters) | ~40-50GB |
| **Total** | **~82-92GB** |
| **Headroom** | **~36-46GB** |

This fits in 128GB. Speed is reduced (~60% of standalone training speed) but allows retraining during low-usage periods without service interruption. Use bf16 LoRA (the default plan) for maximum quality; use QLoRA only when concurrent operation is needed.

---

## 4. Inference Server: vllm-mlx

### Why vllm-mlx

| Feature | Custom generator.py (v1.0) | vllm-mlx (v3.0) |
|---|---|---|
| API compatibility | Custom format | OpenAI + Anthropic compatible |
| Prefix caching | Not implemented | 5.8x TTFT speedup |
| Continuous batching | Not implemented | 3.7x throughput at 16 concurrent |
| Paged KV cache | Not implemented | SSD-tiered cold cache |
| Think mode parser | Manual regex | Native Qwen3 reasoning parser |
| LoRA adapter swap | Custom reload logic | Built-in |
| Production maturity | New code, untested | EuroMLSys '26 paper, used with Claude Code |

### Setup

```bash
# Install
uv tool install git+https://github.com/waybarrios/vllm-mlx.git

# Serve Qwen3-30B-A3B
vllm-mlx serve mlx-community/Qwen3-30B-A3B-4bit \
  --port 8100 \
  --continuous-batching \
  --use-paged-cache \
  --reasoning-parser qwen3

# MDEMG config — use existing OpenAI provider pointed at vllm-mlx
LLM_PROVIDER=openai
LLM_BASE_URL=http://localhost:8100/v1
LLM_MODEL=default
```

No new Go provider code needed. `llmclient`'s existing OpenAI path works directly.

---

## 5. Storage Strategy

| Item | Location | Size |
|---|---|---|
| Base model weights (bf16) | Internal SSD | ~60GB (downloaded once) |
| Quantized inference model (Q4) | Internal SSD | ~17GB |
| LoRA adapters (per version) | Internal SSD | ~200-400MB each |
| Training data (TimescaleDB) | Internal SSD (Docker volume) | ~2-3GB (6 months) |
| Training data (JSONL) | Internal SSD | ~500MB-1GB (6 months) |
| Curated datasets (per version) | Internal SSD | ~20-50MB each |
| Model archive (old versions) | External SSD (TB5) | ~17GB per version |
| Training checkpoints | External SSD | ~2-4GB per checkpoint |

Internal SSD budget: ~95GB. Well within 2TB.

---

## 6. Think Mode as Task Router

Qwen3's built-in `/think` and `/no_think` modes replace the two-model strategy:

| Mode | Tasks | Speed | Use Case |
|---|---|---|---|
| `/no_think` | consulting.classify, hidden.name_emergence, hidden.reclassify, jiminy.codegen, retrieval.intent_translate, retrieval.query_classify, retrieval.rerank_nli | Full speed (~80 tok/s) | Classification, codegen, query rewriting |
| `/think` | ape.reflect, consulting.synthesis, hidden.summarize, jiminy.evaluate, jiminy.evaluate_llm, jiminy.synthesize, metalearn.generalize, retrieval.rerank_cross, summarize.generate | ~60 tok/s (reasoning overhead) | RSIC reflection, synthesis, evaluation |

Implementation: `CompleteOpts.Think` boolean in each call site. vllm-mlx's `--reasoning-parser qwen3` handles the `<think>` block extraction automatically on the server side. The Go consumers receive clean responses after the `SanitizeResponse()` function strips any residual think blocks.
