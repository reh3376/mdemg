# PUBLISH_GUIDE.md — Operator Runbook

**Purpose**: publish Qwen3.8-27B GGUF quants to `reh3376/mdemg-llm-v2:*` on Ollama Library so `mdemg model pull --name mdemg-llm-v2` works for beta testers and downstream operators.

**Prerequisites**:
- **Ollama ≥ 0.32.14** — earlier versions 412 on `ollama pull qwen3.8:27b-*` (registry requires newer schema for qwen3.8). Check with `ollama --version`. If skew shows (client 0.32.14 / server 0.32.4), the DAEMON needs restart:
  - `.app`-installed (typical macOS): quit via menu bar → relaunch `/Applications/Ollama.app` (or `pkill -x ollama && open /Applications/Ollama.app`); verify `ollama --version` reports 0.32.14 for BOTH client and server.
  - `install.sh`-installed: `curl -fsSL https://ollama.com/install.sh | sh` to update binary + restart daemon.
  - `brew install --cask ollama-app --force` upgrades in-place and adopts under brew management (future: `brew upgrade --cask ollama-app`).
- Ollama account with push access to the `reh3376/` namespace (same account used for `mdemg-llm-v1`); `ollama whoami` shows `reh3376`.
- **`llama-quantize` binary** — installed via `brew install llama.cpp` → `/opt/homebrew/bin/llama-quantize`. Compiled binaries only; the Python `convert_hf_to_gguf.py` is NOT included (see Path 3b/3c).
- Source model: Qwen3.8-27B base checkpoint — the winner of task #91 bake-off (0.9105 @ 180s on the 16-task UBENCH augmented eval; baseline v1 = 0.8047, +0.11 lift). Sourcing options are enumerated in Step 0.
- Disk headroom: depends on Step 0 path (see per-path estimates).
- Reliable upload bandwidth: 3 quants totalling ~63 GB will push to Ollama's CDN.

---

## Step 0 — Prepare the source

**Three sourcing options — pick one based on what you have + your bandwidth budget.**

⚠️ **Verified availability (2026-08-19)**: Ollama Library carries `qwen3.8:27b` (200), `qwen3.8:27b-q8_0` (200), `qwen3.8:27b-q4_K_M` (200). **No fp16/f16/instruct variants exist** (all 404). Q8_0 is the highest precision available on Ollama for qwen3.8:27b.

### Path 3a — Ollama-source (Q8_0 ceiling; RECOMMENDED for Ollama-native workflow)

Use Ollama Library's `qwen3.8:27b-q8_0` as the highest-precision source available on that channel. Q8→Q5 and Q8→Q4 are quant-of-quant BUT with negligible practical loss (Q8 has enough dynamic range that lower-tier requantization stays close to native fidelity — much cleaner than the Q5→Q4 case with your existing local Q5_K_M).

```bash
# Pull the highest-precision Ollama-hosted variant
ollama pull qwen3.8:27b-q8_0
# → ~28 GB download from Ollama's CDN

# Locate the model blob on disk. Ollama stores every tag as a manifest
# under manifests/ pointing at a content-addressed GGUF blob in blobs/.
OLLAMA_MODELS="${OLLAMA_MODELS:-$HOME/.ollama/models}"
MANIFEST="$OLLAMA_MODELS/manifests/registry.ollama.ai/library/qwen3.8/27b-q8_0"
ls -la "$MANIFEST"   # sanity: file exists

# The layer with mediaType "application/vnd.ollama.image.model" is the GGUF
DIGEST=$(cat "$MANIFEST" | jq -r '.layers[] | select(.mediaType == "application/vnd.ollama.image.model") | .digest' | sed 's/^sha256://')
SRC="$OLLAMA_MODELS/blobs/sha256-$DIGEST"

# Confirm it's a GGUF (magic bytes at offset 0)
head -c 4 "$SRC"; echo   # → "GGUF"
ls -lh "$SRC"            # → ~28 GB
```

Disk headroom for Path 3a: ~28 GB (source blob) + ~63 GB (3 output quants) = **~91 GB**. Skip Path 3a's "convert to f16" step — the pulled Ollama blob IS already GGUF; use it directly as `$SRC` for Step 1.

### Path 3b — MLX source (dequantize from your local `.local-models/qwen3.8-27b-mlx-4bit/`)

⚠️ **BROKEN ON QWEN3.8-27B AS OF 2026-08-20 (arch `Qwen3_5ForConditionalGeneration` / `qwen35`)**: `llama.cpp`'s `convert_hf_to_gguf.py` (b6600-era shipped via brew Sep 2025) produces a GGUF file **missing critical tensors** — specifically `blk.64.attn_norm.weight` for the last transformer block. Any downstream `llama-quantize` output loads but errors on inference with `tensor '<name>' not found`. Root cause: the `qwen35` arch definition in `convert_hf_to_gguf.py` doesn't yet emit all required tensors for the multi-modal variant. **This blocks Paths 3b and 3c entirely** until llama.cpp lands an updated Qwen3.5 arch handler. **Use Path 3a instead** — Ollama Library's own Q8_0 blob works correctly (their converter is ahead).

⚠️ Your local MLX copy is **already 4-bit** (`config.json` shows `"quantization": {"bits": 4}`). Dequantizing to bf16 then requantizing to Q5/Q8 gives quant-of-quant — the Q8_0 output would be a high-bpw encoding of already-lossy 4-bit data, not true Q8 fidelity. Strictly worse than Path 3a. **Use only if Path 3a is unavailable AND llama.cpp's Qwen3.5 converter is fixed.**

Prerequisite: **`convert_hf_to_gguf.py`** — NOT shipped by brew's `llama.cpp` (compiled binaries only). Shallow-clone the source repo once:

```bash
# One-time setup (~500 MB)
cd /Users/reh3376   # or any persistent dir
git clone --depth 1 https://github.com/ggml-org/llama.cpp.git llama.cpp-src

# Install convert deps into your existing neural venv (mlx_lm already there)
# The venv uses uv; use `uv pip` from the neural dir. If pip isn't available:
cd /Users/reh3376/mdemg/neural
uv pip install 'protobuf>=4.21.0,<5.0.0'
# The other deps (numpy, sentencepiece, transformers, gguf, torch) are already
# installed by mlx_lm. Only protobuf is typically missing.
```

Then dequant + convert:

```bash
cd /Users/reh3376/mdemg/.local-models/qwen3.8-27b-mlx-4bit

# NOTE: use mlx_lm.convert (pure dequant tool), NOT mlx_lm.fuse (LoRA merger).
# mlx_lm.fuse defaults to searching for ./adapters/ and errors "adapter path
# does not exist" — that tool is for merging LoRA adapters, not pure dequant.
/Users/reh3376/mdemg/neural/.venv/bin/mlx_lm.convert \
  -d \
  --hf-path . \
  --mlx-path /tmp/qwen3.8-27b-bf16 \
  --dtype bfloat16
# → ~55 GB bf16 safetensors (11 shards)

/Users/reh3376/mdemg/neural/.venv/bin/python \
  /Users/reh3376/llama.cpp-src/convert_hf_to_gguf.py \
  --outtype f16 \
  --outfile /tmp/qwen3.8-27b-f16.gguf \
  /tmp/qwen3.8-27b-bf16
# → ~54 GB f16 GGUF (~1 min wall-clock on M5)

SRC=/tmp/qwen3.8-27b-f16.gguf
```

Disk headroom for Path 3b: ~55 GB (bf16 intermediate) + ~54 GB (f16 GGUF) + ~63 GB (3 quants) = **~172 GB**.

### Path 3c — HF-safetensors source (native bf16 from Qwen's release — best absolute quality)

⚠️ **BROKEN by the same `convert_hf_to_gguf.py` Qwen3.5 arch bug documented in Path 3b.** Even a pristine bf16 safetensors input produces a tensor-missing GGUF. Use Path 3a until llama.cpp fixes the qwen35 arch handler.

If you have access to Qwen3.8-27B's native bf16/fp16 safetensors (from Qwen's official release or a compatible mirror — not necessarily HuggingFace, e.g. Modelscope or Qwen's own storage), this gives true full-fidelity Q4/Q5/Q8 **once the converter is fixed**. Use the same `convert_hf_to_gguf.py --outtype f16` command as Path 3b's second block (including the one-time clone+deps setup), pointing at the safetensors directory. Skip the `mlx_lm.convert` dequant step (safetensors already at full precision).

Disk headroom: ~55 GB (safetensors) + ~54 GB (f16 GGUF) + ~63 GB (3 quants) = **~172 GB**.

### Path 3d — Recovery from broken convert pipeline (2026-08-20)

If you attempted Path 3b or 3c and got a broken GGUF (symptom: `llama-server` boots successfully but every inference returns `tensor '<name>' not found`), the recovery is to fall through to Path 3a: pull `qwen3.8:27b-q8_0` from Ollama Library and use it as the source for Q4/Q5 requantization + `ollama cp` for Q8_0. This is what shipped for the 2026-08-20 v2 publish. See `publish_manifest_v2.json` for the SHAs captured at that publish.

**Not-recommended paths** (documented for completeness):
- **Path 1** — publish your existing `.local-models/qwen3.8-27b-gguf/Qwen3.8-27B-Q5_K_M.gguf` as `reh3376/mdemg-llm-v2:Q5_K_M` only (skip Q4 + Q8). Q5_K_M is production canonical per shipped docs; single-tier v2 unblocks E4 promote. But: no Q4 tier for RAM-constrained operators; no Q8 tier for high-fidelity operators.
- **Path 2** — dequantize your existing Q5_K_M GGUF then requantize. **Strictly dominated by Path 3a** (Q8→Q4/Q5 has less quality loss than Q5→Q4).

## Step 1 — Quantize to 3 tiers

`$SRC` is set by your Step 0 path choice:
- Path 3a: `$SRC` is the Ollama Q8_0 blob (~28 GB, already GGUF)
- Path 3b/3c: `$SRC` is the f16 GGUF you produced (~54 GB)

Brew's `llama.cpp` installs `llama-quantize` to `/opt/homebrew/bin/llama-quantize` — no `$LLAMA_QUANTIZE=` env var needed; `llama-quantize` resolves via PATH. Run all 3 quantizations from the same `$SRC`:

```bash
OUT=/tmp/qwen3.8-27b-quants
mkdir -p $OUT

llama-quantize $SRC $OUT/mdemg-llm-v2.Q4_K_M.gguf Q4_K_M   # ~10-15 min on M5, ~17 GB output
llama-quantize $SRC $OUT/mdemg-llm-v2.Q5_K_M.gguf Q5_K_M   # ~10-15 min on M5, ~20 GB output

# ⚠ Path 3a only: Q8_0 output is a copy of the source Q8_0 blob (no requantization needed).
#   For Paths 3b/3c the Q8_0 output is a real quantization from f16.
if [ -n "$DIGEST" ] && [ "$SRC" = "$OLLAMA_MODELS/blobs/sha256-$DIGEST" ]; then
  # Path 3a — the source IS Q8_0; copy rather than requantize
  cp $SRC $OUT/mdemg-llm-v2.Q8_0.gguf
else
  # Paths 3b/3c — quantize f16 → Q8_0 (~10-15 min on M5, ~28 GB output)
  llama-quantize $SRC $OUT/mdemg-llm-v2.Q8_0.gguf Q8_0
fi

# Verify sizes (rough estimates — real values captured in Step 2)
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

**Path 3a (recommended) shortcut**: extract the reference Modelfile from the upstream Q8 tag and rewrite `FROM` per quant — this preserves Ollama's built-in Qwen3.8 renderer + Qwen3.5 parser + the CLIP vision projector (multi-modal). Do NOT hand-author a `<|im_start|>...` TEMPLATE block for qwen35 — Ollama's `RENDERER qwen3.8` handles the Jinja chat template natively and is required for correct tool-use + thinking-block handling.

```bash
# Grab the working reference (must have already run `ollama pull qwen3.8:27b-q8_0` in Step 0)
ollama show --modelfile qwen3.8:27b-q8_0 > /tmp/qwen3.8-ref.modelfile

# Extract projector blob path (mediaType application/vnd.ollama.image.projector)
# ollama show prints two FROM lines: the LLM blob + the projector blob (~931 MB CLIP)
PROJECTOR=$(grep '^FROM ' /tmp/qwen3.8-ref.modelfile | sed -n '2p' | sed 's/^FROM //')
echo "projector: $PROJECTOR"   # → /Users/<you>/.ollama/models/blobs/sha256-ac3714bfdd...
```

Then create per-quant Modelfiles that reuse the shared projector blob (Q4/Q5 keep vision capability):

```bash
OUT=/tmp/qwen3.8-27b-quants   # from Step 1
for q in Q4_K_M Q5_K_M; do
  cat > $OUT/Modelfile.$q <<MODEOF
FROM ./mdemg-llm-v2.$q.gguf
FROM $PROJECTOR
TEMPLATE {{ .Prompt }}
RENDERER qwen3.8
PARSER qwen3.5
PARAMETER top_k 20
PARAMETER top_p 0.95
PARAMETER min_p 0
PARAMETER presence_penalty 0
PARAMETER repeat_penalty 1
PARAMETER temperature 1
MODEOF
done
```

**Q8_0 shortcut**: if source blob IS Ollama's Q8_0 (Path 3a), skip `ollama create -f Modelfile.Q8_0` entirely and use `ollama cp` — content-addressed dedup makes this instant and byte-preserving:

```bash
ollama cp qwen3.8:27b-q8_0 reh3376/mdemg-llm-v2:Q8_0
```

For Paths 3b/3c (when the converter is fixed), also write `Modelfile.Q8_0` with the same shape, `FROM ./mdemg-llm-v2.Q8_0.gguf`.

⚠️ **`RENDERER qwen3.8` + `PARSER qwen3.5` require Ollama ≥ 0.32.14** (registry+client contract for qwen3.8 arch). Older versions will fall back to a stub renderer and emit raw template tokens — sanity-check with `ollama run reh3376/mdemg-llm-v2:<Q> "reply with only ok"` before pushing (see Step 4a).

## Step 4 — Create + push each quant to Ollama Library

```bash
cd $OUT

# Path 3a: create Q4/Q5 from local Modelfiles; Q8_0 via ollama cp (see Step 3)
for q in Q4_K_M Q5_K_M; do
  ollama create reh3376/mdemg-llm-v2:$q -f Modelfile.$q
done
# ollama cp qwen3.8:27b-q8_0 reh3376/mdemg-llm-v2:Q8_0    # if not already done in Step 3
```

### Step 4a — MANDATORY local sanity check BEFORE push

Every quant MUST pass a real inference before push — a tensor-missing GGUF (Path 3b/3c broken-convert class) will `ollama create` cleanly but crash on first token. **Push a broken tag and every consumer breaks + the tag has to be republished.** Fail-fast locally:

```bash
for q in Q4_K_M Q5_K_M Q8_0; do
  echo "=== sanity $q ==="
  ollama run reh3376/mdemg-llm-v2:$q "reply with only the word ok, nothing else"
  # Expected: "ok" (possibly preceded by a <think>...</think> block — that's normal)
  # FAIL SIGNATURES: "tensor '...' not found", model process exits, garbled template tokens
done
```

If ANY quant fails, do NOT push it. Debug: check `ollama show <tag>` for FROM lines pointing at the right blob; regenerate the Modelfile per Step 3; if the underlying GGUF is broken (Path 3b/3c class), fall through to Path 3d recovery.

### Step 4b — Push each quant

```bash
for q in Q4_K_M Q5_K_M Q8_0; do
  ollama push reh3376/mdemg-llm-v2:$q
  # → captures the Ollama manifest digest in the push output; save it
done
```

Push size × 3 tiers = ~63 GB total upload. Parallel pushes are supported but throttle by the daemon; sequential is usually simpler.

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

## Common pitfalls (all paths)

- **Ollama push size limits**: Ollama Library has soft per-blob size limits. If Q8_0 (~28 GB) rejects, consider dropping Q8_0 from v2 (Q5_K_M is production canonical anyway). Update Phase C's `quant_manifest_v2.json` to reflect only the published quants.
- **Chat template drift**: If Qwen3.8 uses a different Jinja template than Qwen3.6/Qwen3-14B, the Modelfile's TEMPLATE block MUST reflect it. Symptom: `llama-server` responses are garbled or the model outputs raw template tokens. Test with a `curl POST /v1/chat/completions` immediately after Step 4 to catch this.
- **Ollama account push permissions**: if `ollama push` fails with 401/403, run `ollama whoami` to confirm auth. May need `ollama signin` to refresh credentials.
- **Local storage**: quant outputs live in `$OUT` (e.g. `/tmp/qwen3.8-27b-quants`) — total ~63 GB. Move to permanent storage if needed post-push, or delete after Phase C confirms the Ollama copies are reachable.
- **f16 intermediate (Paths 3b/3c only)**: `/tmp/qwen3.8-27b-f16.gguf` (~55 GB) can be deleted after all quantizations complete. Keep it if you plan to re-quantize (e.g. add Q6_K).
- **Ollama blob source (Path 3a) MUST NOT be deleted while quantize runs**: `$SRC` points at Ollama's content-addressed blob (shared store). `rm` on it while `llama-quantize` reads it will corrupt the run + break other Ollama tags that share the blob.
- **Task #91 verdict**: if you decide differently than the bake-off recommendation (Qwen3.8-27B), edit this guide's Step 0 source path + all downstream file/tag names to reflect Qwen3.6-27B (score 0.9010 vs 3.8's 0.9105).

## Common pitfalls (Path 3a — Ollama source)

- **`ollama pull` progress bar stalls / drops**: rerun `ollama pull qwen3.8:27b-q8_0` — Ollama resumes from the last successfully downloaded chunk.
- **`jq` not installed for the DIGEST extraction**: install via `brew install jq` OR read the manifest JSON manually and grep for the model layer's `digest`.
- **Manifest path drift**: if `$OLLAMA_MODELS/manifests/registry.ollama.ai/library/qwen3.8/27b-q8_0` doesn't exist after pull, check `ls $OLLAMA_MODELS/manifests/` — some ollama versions omit the `registry.ollama.ai` prefix; the manifest may live at `.../library/qwen3.8/27b-q8_0` directly.
- **Blob is symlinked, not copied**: the `SRC` variable points at the shared blob store. `llama-quantize` will READ it (fine); don't `rm` it while quantize is running.
- **Q8_0 published as a source-blob copy (Path 3a)**: this is a byte-for-byte copy of Ollama's `qwen3.8:27b-q8_0` blob under our namespace tag. Ollama's dedupe MAY notice this at push time and short-circuit — that's OK; the tag will still resolve for consumers.

## Estimated wall-clock

**Path 3a (Ollama source, recommended)**:
- Step 0: 15-60 min (ollama pull, ~28 GB, bandwidth-dependent)
- Step 1: 20-40 min (2 `llama-quantize` runs — Q4_K_M + Q5_K_M; Q8_0 is a source-blob copy so ~free)
- Step 2: 5 min (SHA capture)
- Step 3: 10 min (Modelfile authoring; template verify)
- Step 4: 30-120 min (upload ~63 GB total to Ollama's CDN)
- Step 5: 5 min (verify)
- Step 6: 5 min (hand-off)
- **Total: 1.5-4 hours** wall-clock; mostly download + upload (both unattended); `llama-quantize` runs a Q8→lower requantize which is faster than a full f16→quant.

**Path 3b (MLX-4bit dequant, degraded quality)**:
- Step 0: 45-90 min (mlx_lm.fuse dequant + convert_hf_to_gguf, disk-bound)
- Step 1: 30-60 min (3 full `llama-quantize` runs from f16)
- Steps 2-6: same as 3a
- **Total: 2.5-5 hours** wall-clock. Q8_0 output is degraded (was 4-bit source).

**Path 3c (HF-safetensors native bf16, best quality)**:
- Step 0: 30-120 min (download ~55 GB safetensors + convert_hf_to_gguf)
- Step 1: 30-60 min (3 full `llama-quantize` runs from f16)
- Steps 2-6: same as 3a
- **Total: 2-5 hours** wall-clock; only path where all 3 tiers are native full-fidelity quantizations.
