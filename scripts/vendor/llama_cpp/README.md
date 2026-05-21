# Vendored from llama.cpp

This directory contains files copied verbatim from the [llama.cpp](https://github.com/ggml-org/llama.cpp) repository (MIT license). Files are fetched once and pinned for Sprint MODEL-DIST-002's adapter-conversion pipeline.

## Files

| File | Source | Date pinned | Purpose |
|---|---|---|---|
| `convert_lora_to_gguf.py` | https://github.com/ggml-org/llama.cpp/blob/master/convert_lora_to_gguf.py | 2026-05-21 | Convert a PEFT-format LoRA adapter directory to a `.gguf` adapter file consumable by `llama-server --lora`. |
| `LICENSE.llama_cpp` | https://github.com/ggml-org/llama.cpp/blob/master/LICENSE | 2026-05-21 | MIT license. Required attribution. |

## Why vendored

Per Sprint MODEL-DIST-001 epic_2_forensic.md: `brew install llama.cpp` ships `convert_hf_to_gguf.py` but NOT `convert_lora_to_gguf.py`. The latter is required to produce a standalone GGUF LoRA file (the `--lora` argument llama-server accepts). Rather than ask operators to clone llama.cpp source, we vendor the one script we need with explicit attribution.

## Refresh policy

Re-fetch on next sprint touching this code:

```bash
curl -fsSL https://raw.githubusercontent.com/ggml-org/llama.cpp/master/convert_lora_to_gguf.py \
  -o scripts/vendor/llama_cpp/convert_lora_to_gguf.py
curl -fsSL https://raw.githubusercontent.com/ggml-org/llama.cpp/master/LICENSE \
  -o scripts/vendor/llama_cpp/LICENSE.llama_cpp
```

Update the "Date pinned" column above. If the script's CLI surface changes, regenerate the verification step in MODEL-DIST-002 Epic 2 to confirm the new options still produce the expected GGUF.

## Upstream license

llama.cpp is MIT-licensed; see `LICENSE.llama_cpp`. Use is compatible with this project's license.
