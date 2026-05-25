# Sprint MODEL-DIST-002 — Epic 3 Live Verification

**Date**: 2026-05-25
**Result**: PASS — (base + adapter) produces semantically-aligned outputs vs (fused merged GGUF).

## Setup

Two llama-server instances running side-by-side:

| Endpoint | Model | Adapter | Source |
|---|---|---|---|
| `127.0.0.1:8102` (production) | `mdemg-llm-v1.Q5_K_M.gguf` | — | Phase 13.5 production canonical (fused merged) |
| `127.0.0.1:18103` (test) | `mdemg-llm-v1.f16.gguf` | `mdemg-llm-v1-adapter.gguf` (257 MB, SHA `0cfaf4bae3215a4aea664a8d28ae9a41d73ee740cbcce5c2eef950232cfe1de5`) | Sprint MODEL-DIST-002 Epic 2 output via MLX → PEFT → GGUF LoRA pipeline |

Both serve `mdemg-llm-v1`. 18103 uses `--ctx-size 2048 --no-webui --jinja`.

## Pipeline summary

```
adapters/tier1/adapters.safetensors                       (514 MB MLX, Phase 5 SFT Iter 2400)
         │
         │ scripts/mlx_adapter_to_peft.py (Epic 1, 14 unit tests)
         │ – Key rename: model.layers.<N>.<module>.lora_a → base_model.model.model.layers.<N>.<module>.lora_A.weight
         │ – Tensor transpose: lora_a (in,rank) → (rank,in); lora_b (rank,out) → (out,rank)
         │ – adapter_config.json schema translation (MLX → PEFT)
         ▼
.local-models/mdemg-llm-v1-adapter-peft/                  (514 MB PEFT-format dir)
  ├── adapter_config.json                                  (PEFT schema, r=32, alpha=64, 7 target_modules)
  └── adapter_model.safetensors                            (560 tensors, transposed shapes, renamed keys)
         │
         │ scripts/vendor/llama_cpp/convert_lora_to_gguf.py (llama.cpp b9000)
         │ – Reads PEFT config + safetensors
         │ – Base model config from .local-models/qwen3-14b-mdemg-v1-bf16/config.json
         │ – Emits Little-Endian GGUF with 560 LoRA tensors
         ▼
.local-models/mdemg-llm-v1-adapter.gguf                   (257 MB f16 GGUF LoRA)
         │
         │ llama-server --model <f16 base> --lora <adapter.gguf>
         ▼
127.0.0.1:18103 — base + adapter inference
```

## Sanity inference comparison

**Prompt**: `In one sentence: what is MDEMG?`
**Temperature**: 0.0
**Max tokens**: 100

| Endpoint | Response |
|---|---|
| 18103 (base + adapter) | "MDEMG is a knowledge graph memory system that captures domain-specific insights, constraints, and patterns from training data to support reasoning, validation, and context-aware responses." |
| 8102 (fused production) | "MDEMG is a knowledge graph that captures the collective understanding of a memory system, representing concepts, relationships, and insights derived from interactions and learning processes." |

Both responses are:
- Coherent, complete sentences (no garbage tokens)
- Semantically aligned (both describe MDEMG as a knowledge-graph memory system)
- Topically appropriate (mention domain-specific learning + insights)

This confirms:
1. The MLX → PEFT tensor transpose is correct (otherwise outputs would be garbage).
2. The PEFT key naming maps cleanly to the GGUF LoRA layout llama.cpp expects.
3. The Phase 5 LoRA fine-tune is meaningfully applied via the `--lora` runtime path.

Token-level identity isn't expected (Q5_K_M's quantization vs f16+adapter at full precision), but **semantic agreement is the bar**, and it holds.

## Counts

| Metric | Value |
|---|---|
| MLX input tensors | 560 |
| PEFT output tensors | 560 |
| GGUF LoRA output tensors | 560 |
| GGUF file size | 257 MB (~35× smaller than fused Q5_K_M's ~9 GB) |
| Pipeline wall-clock (Epic 1 + 2 combined) | <30 sec on M5 Max |

## Outstanding for Epic 4

- Modelfile.adapter authoring + `ollama create reh3376/mdemg-llm-v1-adapter:latest` (local only).
- **Operator-gated push** per MODEL-DIST-001 pattern (one-way action).
- Capture published Ollama manifest digest into `quant_manifest.json`.
