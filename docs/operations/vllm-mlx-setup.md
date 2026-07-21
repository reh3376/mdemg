# vllm-mlx Setup Guide

> ⚠️ **SUPERSEDED (2026-06-11, DOC-TRUTH-002).** This guide describes the
> abandoned MoE-era stack: vllm-mlx serving `Qwen3.6-35B-A3B`. The MoE
> target was permanently dropped on 2026-04-22 (Metal 499K MTLResource
> ceiling), and the production runtime since Phase 13.5 (2026-05-03) is
> **llama.cpp `llama-server` on :8102** serving
> `mdemg-llm-v1.Q5_K_M.gguf` (dense Qwen3-14B fine-tune). See the
> "Local LLM Runtime" section of `CLAUDE.md` and
> `docs/development/ft-lora/00_README_v2.md` for the current setup.
> Note also that the `LLM_BASE_URL` key shown below was never a real
> config key — the endpoint override key is `LLM_ENDPOINT` (current
> default `http://127.0.0.1:8102/v1`).
> Retained unchanged below as a historical record (R-LT-4 keep-history).

## Overview

vllm-mlx is the inference server for serving the LoRA fine-tuned model. It exposes an OpenAI-compatible API with prefix caching, continuous batching, and native Qwen3 reasoning parser. Since it's OpenAI-compatible, the existing MDEMG `llmclient` OpenAI provider works directly — no Go code changes needed.

## Installation

```bash
# Install vllm-mlx
uv tool install vllm-mlx

# Download the quantized base model (~17GB)
huggingface-cli download mlx-community/Qwen3.6-35B-A3B-4bit

# Start the server
vllm-mlx --model mlx-community/Qwen3.6-35B-A3B-4bit --port 8100
```

## MDEMG Configuration

Point MDEMG at the vllm-mlx server by setting these environment variables in `.env`:

```bash
# Use the OpenAI-compatible provider
LLM_PROVIDER=openai
LLM_BASE_URL=http://localhost:8100/v1
LLM_MODEL=mlx-community/Qwen3.6-35B-A3B-4bit

# Optional: dedicated provider for query classification
QUERY_CLASSIFY_PROVIDER=openai
QUERY_CLASSIFY_MODEL=mlx-community/Qwen3.6-35B-A3B-4bit
```

After updating `.env`, restart the MDEMG server:

```bash
docker compose restart mdemg
# Or for native dev:
./bin/mdemg start --auto-migrate
```

<!-- TODO (Sprint E): Update RAM estimate for asymmetric-quant footprint (shared+attention BF16, routed experts MXFP4_MOE per memo 07 v3.1 §3.8); current ~22 GB figure is a carry-over from Qwen3-30B-A3B Q4 symmetric quant. -->
## Memory Budget (M5 Max 128GB)

| Component | RAM Usage |
|-----------|----------|
| Qwen3.6-35B-A3B Q4 inference | ~22 GB |
| MDEMG server + Neo4j + TSDB | ~8 GB |
| Prefix cache (dynamic) | ~4 GB |
| **Inference total** | **~34 GB** |
| Available for training | ~94 GB |

Concurrent inference + training is feasible. vllm-mlx handles inference while `mlx-lm-lora` runs training in a separate process. MLX shares unified memory, so actual allocation is dynamic.

## Performance

- **Prefix caching**: 5.8x TTFT speedup for repeated system prompts (enabled by default)
- **Throughput**: ~80 tok/s decode on M5 Max for Q4 quantization
- **Continuous batching**: Multiple concurrent MDEMG tasks share the same model instance

## Verification

### Quick health check

```bash
curl -s http://localhost:8100/v1/models | python3 -m json.tool
```

### Smoke test all 16 tasks

```bash
python3 scripts/test_vllm_mlx.py --base-url http://localhost:8100/v1
```

This sends one test prompt per ULTS task and validates responses against each task's `output_schema`.

### Manual test

```bash
curl -s http://localhost:8100/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mlx-community/Qwen3.6-35B-A3B-4bit",
    "messages": [
      {"role": "system", "content": "You are a query classifier. Respond in JSON with a types array."},
      {"role": "user", "content": "How does the authentication middleware work?"}
    ],
    "max_tokens": 500
  }' | python3 -m json.tool
```

## Serving a Fine-Tuned Model

After training produces a LoRA adapter (Phase B.2), fuse and quantize it:

<!-- TODO (Sprint E): Rename output path to reflect asymmetric-quant strategy (mdemg-qwen3.6-35b-v1-asym/); path naming convention finalized by Sprint E's per-module quant selectors. -->
```bash
python -m training.quantize_deploy \
  --base-model Qwen/Qwen3.6-35B-A3B \
  --adapter-path adapters/v1/ \
  --output-path models/mdemg-qwen3-30b-v1-q4/ \
  --quantize 4bit
```

Then serve the fused model:

```bash
vllm-mlx --model models/mdemg-qwen3-30b-v1-q4/ --port 8100
```

Update `LLM_MODEL` in `.env` to point to the local path.

## Troubleshooting

| Issue | Fix |
|-------|-----|
| `ModuleNotFoundError: mlx` | Install on Apple Silicon only: `pip install mlx` |
| Out of memory | Reduce `--max-model-len` or use a smaller quantization |
| Slow first request | Expected — prefix cache warms up on first prompt per system message |
| JSON parsing errors | Add `"response_format": {"type": "json_object"}` to request |

## Documents Accessed

- `internal/llmclient/openai.go` — OpenAI provider (used for vllm-mlx compatibility)
- `docs/tests/ults/specs/*.ults.json` — 16 ULTS task specifications
- `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md` — Phase 3 (vllm-mlx)
