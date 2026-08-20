# PUBLISH_GUIDE.md — Operator Runbook

**Purpose**: publish Qwen3.8-27B GGUF quants to `reh3376/mdemg-llm-v2:*` on Ollama Library so `mdemg model pull --name mdemg-llm-v2` works for beta testers and downstream operators.

**Prerequisites**:
- Ollama account with push access to the `reh3376/` namespace (same account used for `mdemg-llm-v1`).
- Local Ollama installed + logged in: `ollama --version` succeeds; `ollama whoami` shows `reh3376`.
- `llama.cpp` build with `llama-quantize` binary available (same tool used in MODEL-DIST-001's pipeline).
- Source model: Qwen3.8-27B base checkpoint (either MLX safetensors OR HF-safetensors). Winner of task #91 bake-off (0.9105 @ 180s on the 16-task UBENCH augmented eval; baseline v1 = 0.8047, +0.11 lift).
- Disk headroom: ~80 GB for intermediate f16 GGUF + all 3 quant outputs.
- Reliable upload bandwidth: 3 quants totalling ~63 GB will push to Ollama's CDN.

---

## Step 0 — Prepare the source

If starting from **MLX safetensors** (typical if the base was fetched via `mlx_lm` for the bake-off):

```bash
cd /Users/reh3376/mdemg/.local-models/qwen3.8-27b   # or wherever your MLX checkpoint lives
# Dequant to bf16 HF safetensors (mirror mdemg-llm-v1's pipeline)
mlx_lm.fuse --dequantize \
  --model . \
  --save-path /tmp/qwen3.8-27b-bf16
```

If starting from **HF safetensors** (e.g. downloaded via `huggingface-cli`), skip this step.

Then convert to f16 GGUF (baseline for quantization):

```bash
python3 /path/to/llama.cpp/convert_hf_to_gguf.py \
  --outtype f16 \
  --outfile /tmp/qwen3.8-27b-f16.gguf \
  /tmp/qwen3.8-27b-bf16
# → ~55 GB f16 GGUF
```

## Step 1 — Quantize to 3 tiers

Reuses MODEL-DIST-001's shipped pipeline; run all 3 quantizations from the same f16 source:

```bash
LLAMA_QUANTIZE=/path/to/llama.cpp/build/bin/llama-quantize
SRC=/tmp/qwen3.8-27b-f16.gguf
OUT=/tmp/qwen3.8-27b-quants

mkdir -p $OUT

$LLAMA_QUANTIZE $SRC $OUT/mdemg-llm-v2.Q4_K_M.gguf Q4_K_M
$LLAMA_QUANTIZE $SRC $OUT/mdemg-llm-v2.Q5_K_M.gguf Q5_K_M
$LLAMA_QUANTIZE $SRC $OUT/mdemg-llm-v2.Q8_0.gguf   Q8_0

# Verify sizes (rough estimates — real values captured in Step 4)
ls -lh $OUT
# Expected order of magnitude:
#   Q4_K_M: ~16 GB
#   Q5_K_M: ~19 GB
#   Q8_0:   ~28 GB
```

## Step 2 — Capture per-quant SHA256 (needed for Phase C manifest)

```bash
cd $OUT
for q in Q4_K_M Q5_K_M Q8_0; do
  echo "$q: $(shasum -a 256 mdemg-llm-v2.$q.gguf | awk '{print $1}')  size=$(stat -f%z mdemg-llm-v2.$q.gguf)"
done | tee /tmp/mdemg-llm-v2-shas.txt
```

Save `/tmp/mdemg-llm-v2-shas.txt` — Phase C sprint reads these to populate `quant_manifest_v2.json`.

## Step 3 — Author the Modelfiles

One Modelfile per quant. Template (adapt if the Qwen3.8 chat template differs from Qwen3-14B's — check the source model's `tokenizer_config.json` for the canonical Jinja template):

```dockerfile
# Modelfile.Q5_K_M
FROM ./mdemg-llm-v2.Q5_K_M.gguf

# Same template as mdemg-llm-v1 (Qwen3 chat template) — verify against the
# source model's tokenizer_config.json chat_template field. If Qwen3.8 uses
# a different template shape, replace this block.
TEMPLATE """{{ if .System }}<|im_start|>system
{{ .System }}<|im_end|>
{{ end }}{{ if .Prompt }}<|im_start|>user
{{ .Prompt }}<|im_end|>
{{ end }}<|im_start|>assistant
{{ .Response }}<|im_end|>
"""

PARAMETER stop "<|im_start|>"
PARAMETER stop "<|im_end|>"
PARAMETER num_ctx 32768

# Optional metadata:
LICENSE "Qwen — check upstream for exact terms"
```

Repeat for `Modelfile.Q4_K_M` (change FROM line to `./mdemg-llm-v2.Q4_K_M.gguf`) and `Modelfile.Q8_0`.

## Step 4 — Create + push each quant to Ollama Library

```bash
cd $OUT

for q in Q4_K_M Q5_K_M Q8_0; do
  # Create the local Ollama model
  ollama create reh3376/mdemg-llm-v2:$q -f Modelfile.$q

  # Push to Ollama Library (public)
  ollama push reh3376/mdemg-llm-v2:$q
  # → captures the Ollama manifest digest in the push output; save it
done
```

**During push, capture the Ollama manifest digest** — the CLI prints something like:

```
writing manifest
success
pushed reh3376/mdemg-llm-v2:Q5_K_M
digest sha256:ae6e54fe1ee0b487ae41260687ed14c46c30d1ffb0fece936282418b5bcb78e1
```

Record the digest per quant — Phase C's manifest needs both the GGUF SHA (from Step 2) and the Ollama manifest digest (from here).

## Step 5 — Verify published

```bash
# Public URL should return 200
curl -s -o /dev/null -w "%{http_code}\n" https://ollama.com/reh3376/mdemg-llm-v2
# → 200

# Per-quant URLs
for q in Q4_K_M Q5_K_M Q8_0; do
  curl -s -o /dev/null -w "$q: %{http_code}\n" "https://ollama.com/reh3376/mdemg-llm-v2:$q"
done
# → all 200

# End-to-end pull test on a scratch host (or ~/.mdemg-scratch/)
MDEMG_MODEL_NAME=mdemg-llm-v2 mdemg model pull --quant Q5_K_M
# → today, this will 200 on the Ollama fetch, but SHA verify prints
#   "skipped — loaded manifest is for mdemg-llm-v1, request is for mdemg-llm-v2"
#   (this is the Phase B safety fix — Phase C ships the real v2 manifest)
```

## Step 6 — Hand off to Phase C sprint

Send the following to the Phase C sprint (or attach to task #134):

1. `/tmp/mdemg-llm-v2-shas.txt` — 3 SHA256 values + file sizes
2. 3 Ollama manifest digests from Step 4 push outputs
3. Confirmed public URLs return 200 (Step 5)
4. Any deviations from this guide (e.g. chat template shape, quant tool version) so Phase C can document

Phase C sprint (`HOMEBREW-INSTALLER-QWEN-UPDATE-002`) creates `internal/cli/quant_manifest_v2.json` with these values, extends `LoadQuantManifest` to pick manifest by `cfg.ModelName`, updates the feature doc, and runs full end-to-end live smoke.

## Common pitfalls

- **Ollama push size limits**: Ollama Library has soft per-blob size limits. If Q8_0 (~28 GB) rejects, consider dropping Q8_0 from v2 (Q5_K_M is production canonical anyway). Update Phase C's `quant_manifest_v2.json` to reflect only the published quants.
- **Chat template drift**: If Qwen3.8 uses a different Jinja template than Qwen3.6/Qwen3-14B, the Modelfile's TEMPLATE block MUST reflect it. Symptom: `llama-server` responses are garbled or the model outputs raw template tokens. Test with a `curl POST /v1/chat/completions` immediately after Step 4 to catch this.
- **Ollama account push permissions**: if `ollama push` fails with 401/403, run `ollama whoami` to confirm auth. May need `ollama signin` to refresh credentials.
- **Local storage**: quant outputs live in `$OUT` (e.g. `/tmp/qwen3.8-27b-quants`) — total ~63 GB. Move to permanent storage if needed post-push, or delete after Phase C confirms the Ollama copies are reachable.
- **f16 intermediate**: `/tmp/qwen3.8-27b-f16.gguf` (~55 GB) can be deleted after all quantizations complete. Keep it if you plan to re-quantize (e.g. add Q6_K).
- **Task #91 verdict**: if you decide differently than the bake-off recommendation (Qwen3.8-27B), edit this guide's Step 0 source path + all downstream file/tag names to reflect Qwen3.6-27B (score 0.9010 vs 3.8's 0.9105).

## Estimated wall-clock

- Step 0: 15-30 min (depending on source form)
- Step 1: 30-60 min (3 llama-quantize runs, CPU-bound)
- Step 2: 5 min (SHA capture)
- Step 3: 10 min (Modelfile authoring; template verify)
- Step 4: 30-120 min (upload bandwidth-dependent)
- Step 5: 5 min (verify)
- Step 6: 5 min (hand-off)
- **Total: 2-4 hours** wall-clock; mostly `llama-quantize` + `ollama push` (both mostly-unattended after kickoff)
