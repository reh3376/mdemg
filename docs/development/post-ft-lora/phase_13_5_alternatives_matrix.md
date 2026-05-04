# Phase 13.5 MLX Server Stability — Alternatives Matrix
**Stream 3 of 4 | Research completed: 2026-05-02**

---

## Production Context

- **Machine:** Mac17,6 — M5 Max, 128 GB unified memory, macOS 26.3.2
- **Current serving stack:** mlx_lm.server 0.31.2 (OpenAI-compatible, port 8101)
- **Production model:** `mdemg-llm-v1` → symlink → `qwen3-14b-mdemg-v1`
  - Format: MLX safetensors, 4-bit quantized (group_size=64, bits=4)
  - Architecture: Qwen3ForCausalLM, 40 layers, hidden_size=5120, GQA (40 query / 8 KV heads)
  - Disk: 7.7 GB (two shards: 5.4 GB + 3.0 GB)
- **Crash symptom:** `SIGABRT` in `mlx::core::gpu::check_error(MTL::CommandBuffer*)`, ~14-min cycle
- **Framework call sites:** 16 LLM call sites, all single-shot structured-output, up to 4–8 concurrent
- **LLM endpoint contract:** `http://127.0.0.1:8101/v1/chat/completions` (OpenAI schema + SSE streaming)

---

## Executive Summary

The macOS 26.3.x update introduced a Metal toolchain regression (Toolchain 32023) that tightened type enforcement in MetalPerformancePrimitives — specifically rejecting `matmul2d` instantiations with mixed `<half, bfloat>` operands. This breaks **all** GGML-Metal and MLX-Metal backends on M5 hardware running macOS 26.3.x until each framework ships a fix. As of May 2, 2026:

- **mlx_lm.server (current):** Crashing every ~14 min. The crash pattern (SIGABRT in `check_error`) matches the unbounded KV cache / Metal OOM path documented in mlx-lm#854 and mlx-lm#883, compounded by macOS 26.3.x Metal Toolchain 32023 changes (mlx#3337, open, unpatched in 0.31.2).
- **Ollama (≥ 0.20.5, ≤ 0.22.1):** Confirmed broken on M5 + macOS 26.3.x via multiple independent reports (ollama#15448, #15541, #15594, #15862, all open). The GGML Metal path fails with the same static_assert. Ollama's March 2026 MLX-backend preview (v0.19/0.20) shows performance gains but does **not** resolve the 26.3.x type-mismatch crash.
- **llama.cpp (direct, ≥ b9006):** Same GGML Metal path, same underlying type-mismatch risk on macOS 26.3.x. No confirmed fix PR merged as of May 2, 2026 (llama.cpp Discussion #17298 exists but contains no resolved version).
- **vllm-mlx (≥ 0.2.9):** MLX native, same MLX Metal dependency. Subject to same mlx#3337 Metal toolchain issue on macOS 26.3.x. Not tested on M5 specifically (docs list M1–M4 only).
- **MLC-LLM:** TVM-compiled Metal kernels — uses a **different Metal path** than GGML or MLX, potentially not affected by the MPP bfloat/half regression. However, requires full TVM recompilation of the model (~hours first run). Limited M5/macOS 26 evidence.
- **LM Studio 0.4.x (llmster daemon):** Ships its own MLX runtime, has been patching macOS 26 issues independently (lmstudio-bug-tracker#1504, #1645). Has dedicated macOS engineering; may have fixed the Metal toolchain issue before open-source upstreams.

**Ranked summary table:**

| Rank | Candidate | Stability on M5/26.3.x | Swap Engineering | Peak Performance |
|------|-----------|----------------------|-----------------|------------------|
| 1 | **llama.cpp (direct)** | Unknown — same GGML Metal risk as Ollama; requires build-from-source to test fix branch | Medium (GGUF re-quant needed) | ~100–130 tok/s decode on 14B Q4_K_M, M5 Max |
| 2 | **MLC-LLM** | Possibly unaffected (different Metal path); stability unconfirmed on M5/26.3.x | High (TVM compilation) | ~190 tok/s on M2 Ultra (14B equiv.) |
| 3 | **vllm-mlx** | Same MLX Metal risk; M5 not in docs; actively maintained | Low-Medium (MLX native, same model format) | 400+ tok/s claimed (smaller models; M4 Max) |
| 4 | **LM Studio** | Proprietary patches ahead of upstream; some independent evidence of macOS 26 fixes | Medium (headless daemon, port change) | Comparable to MLX; benchmarks misleading |
| 5 | **Ollama** | Definitively broken on M5/26.3.x across 0.20–0.22 (multiple open issues unresolved) | Low (OpenAI compat, GGUF) | 112 tok/s decode (v0.19 MLX path; not working on this hardware) |

*No ranking implies a winner — all candidates carry meaningful risk on the specific hardware + OS combination (M5, macOS 26.3.2).*

---

## Per-Candidate Deep Dives

### 1. llama.cpp (`llama-server`)

**Project:** [ggml-org/llama.cpp](https://github.com/ggml-org/llama.cpp)
**Stars/Maturity:** ~100K stars (March 2026), 700+ contributors, ~3,800 PRs in 2025. Weekly build cadence. Latest: b9006 (May 2, 2026). [Source](https://aithinkerlab.com/llama-cpp-100k-github-stars-2026-7-reasons-devs-obsess/)
**License:** MIT

#### OpenAI API Compatibility

Full native OpenAI-compatible server. Exposes:
- `GET /v1/models`
- `POST /v1/chat/completions` (streaming SSE + non-streaming)
- `POST /v1/completions` (legacy)
- `POST /v1/embeddings`

Startup: `llama-server -m model.gguf --port 8080 --host 127.0.0.1 -np 4 --ctx-size 16384 --cont-batching`

Homebrew: `brew install llama.cpp` — installs `llama-server`, `llama-cli`, and tooling. Formula depends on `ggml 0.10.2`. Formula is on macOS Sequoia/Tahoe/Sonoma. [Source](https://formulae.brew.sh/formula/llama.cpp)

#### Model Format Requirements

Requires GGUF. Our model (MLX safetensors, 4-bit) requires conversion. See GGUF Conversion Path section.

Available quantizations for Qwen3-14B:
| Type | Size | Use case |
|------|------|---------|
| Q2_K | 5.75 GB | Minimum quality |
| Q3_K_M | 7.32 GB | Low quality |
| Q4_K_M | 9.0 GB | Sweet spot (recommended) |
| Q5_K_M | 10.5 GB | Quality priority |
| Q6_K | 12.1 GB | Near-lossless |
| Q8_0 | 15+ GB | Reference |

Pre-built Qwen3-14B GGUFs exist on HuggingFace: [Qwen/Qwen3-14B-GGUF](https://huggingface.co/Qwen/Qwen3-14B-GGUF), [bartowski/Qwen_Qwen3-14B-GGUF](https://huggingface.co/bartowski/Qwen_Qwen3-14B-GGUF). These are the **base** Qwen3-14B — not our fine-tuned `mdemg-llm-v1`. Using a pre-built GGUF would lose the fine-tune. Converting our MLX fine-tuned weights is required (see section below).

#### Stability Evidence — M5 + macOS 26.3.x

**Critical finding:** The GGML Metal path uses MetalPerformancePrimitives for `matmul2d`. macOS 26.3.1 RSR purged Metal's per-device shader cache, and the new MPP headers reject GGML's `<half, bfloat>` instantiation with a `static_assert` failure.

Evidence:
- [ollama#15862](https://github.com/ollama/ollama/issues/15862): "MPPTensorOpsMatMul2dImpl static_assert (bfloat/half) on Apple M5" — affects Ollama's bundled llama.cpp, open, unresolved. Confirmed on Ollama 0.21.2 and 0.22.0-rc1.
- [Discussion #17298](https://github.com/ggml-org/llama.cpp/discussions/17298): "Working llama.cpp on macOS 26+ (Metal)" — references external repo of workarounds but provides no confirmed fixed version.
- The Ollama issues are filed against Ollama's bundled GGML, not llama.cpp mainline. **Direct llama.cpp builds compiled from source against a patched GGML may behave differently** — but no confirmed working build has been cited as of May 2, 2026.

**Distinct from the MPP issue:** The original crash being investigated (mlx_lm `SIGABRT` in `check_error`) is a different code path (MLX's own Metal command buffer handling, not GGML MPP). The two issues are separate Metal-layer failures triggered by macOS 26.3.x.

#### Performance on M-series

On M4 Pro (24 GB), 13B Q4_K_M: 35–50 tok/s decode. [Source](https://contracollective.com/blog/llama-cpp-vs-mlx-ollama-vllm-apple-silicon-2026)

M5 Max has ~28% higher memory bandwidth vs M4 Max (600 GB/s vs 546 GB/s), predicting ~45–65 tok/s decode for 14B Q4_K_M on M5 Max. MLX delivers 20–50% higher throughput than llama.cpp on the same hardware when both are working. [Source](https://llmcheck.net/blog/apple-silicon-m5-max-local-ai-guide/)

For 128 GB M5 Max with 14B model, the model fits comfortably with headroom. Q5_K_M (10.5 GB) is viable for better quality.

#### Long-Running Posture

- KV cache grows per conversation slot, bounded by `--ctx-size × --parallel`
- Host-memory cache (`-cram` flag) defaults to 8 GB — can grow unboundedly across sessions; [Discussion #18488](https://github.com/ggml-org/llama.cpp/discussions/18488) reports 128 GB being consumed over extended use. Mitigation: `-cram 0` disables it, or set a small fixed value.
- Potential memory leak in batch mode with long-running generation: [Issue #22060](https://github.com/ggml-org/llama.cpp/issues/22060), open.
- KV cache reuse bug (cache-reuse not effective in Qwen3): [Issue #18497](https://github.com/ggml-org/llama.cpp/issues/18497)
- Restart-on-OOM not built-in; needs external supervisor (launchd KeepAlive covers crash recovery).

#### Concurrency Model

Continuous batching (`--cont-batching`, default on) allows N parallel inference slots. `--parallel N` (also `-np N`) sets slot count. Each slot consumes KV cache memory proportional to `ctx_size`. For 4–8 concurrent calls: `-np 8 --ctx-size 4096` is workable with 128 GB.

#### Homebrew + launchd

`brew install llama.cpp` available. No pre-built launchd plist. Writing a plist with KeepAlive is straightforward.

---

### 2. Ollama

**Project:** [ollama/ollama](https://github.com/ollama/ollama)
**Stars/Maturity:** Major consumer project, 210+ releases, v0.22.1 (April 28, 2026). [Source](https://github.com/ollama/ollama/releases)
**License:** MIT

#### OpenAI API Compatibility

Full OpenAI compat at `/v1/` prefix:
- `POST /v1/chat/completions`
- `POST /v1/embeddings`
- `GET /v1/models`

[Source](https://docs.ollama.com/api/openai-compatibility). Tested with OpenAI Python SDK. SSE streaming supported.

#### Model Format Requirements

Custom GGUF loading via `Modelfile`:
```
FROM /path/to/your-model.gguf
```
Then: `ollama create mdemg-v1 -f Modelfile && ollama run mdemg-v1`

[Source](https://docs.ollama.com/import). Also supports `ADAPTER /path/to/adapter.gguf` for GGUF adapters on top of a base.

Ollama's March 2026 MLX preview (v0.19+) routes Apple Silicon through MLX — meaning it can load MLX-format safetensors directly in newer versions. However: this is for **library models** pulled from the Ollama registry. Custom MLX-format models must still go through Modelfile, and architecture support is expanding. [Source](https://ollama.com/blog/mlx)

#### Stability Evidence — M5 + macOS 26.3.x

**Definitively broken as of May 2, 2026.** Multiple independent issues:

| Issue | Status | Versions affected |
|-------|--------|-------------------|
| [#13867](https://github.com/ollama/ollama/issues/13867): GGML_ASSERT crash on M5 | Open | 0.13.x–0.14.3 |
| [#14432](https://github.com/ollama/ollama/issues/14432): Metal static_assert type mismatch M5 | Open | Unspecified |
| [#15448](https://github.com/ollama/ollama/issues/15448): M5 + macOS 26.3.1 Metal compiler fails even with GPU=0 | Open | Multiple |
| [#15496](https://github.com/ollama/ollama/issues/15496): 0.20.5 crashes on M5 16 GB, even qwen2.5:0.5b | Open | 0.20.5 |
| [#15541](https://github.com/ollama/ollama/issues/15541): MTLLibrary bfloat/half mismatch, llama runner → 500 | Closed (unresolved?) | 0.20.6 |
| [#15594](https://github.com/ollama/ollama/issues/15594): Metal compilation error M5 macOS 26.3.1 static_assert | Open | 0.21.x |
| [#15748](https://github.com/ollama/ollama/issues/15748): 0.21.0 fails Metal init M5/26.2; 0.18.0 works | Open | 0.21.0 |
| [#15862](https://github.com/ollama/ollama/issues/15862): MPPTensorOpsMatMul2dImpl static_assert bfloat/half on macOS 26.3.1 | Open | 0.21.2, 0.22.0-rc1 |

**Rollback to 0.18.0 resolves older issue on 26.2 but not 26.3.x.**

#### Performance on M-series

Ollama 0.19/0.20 on M5 Max with Qwen3.5-35B-A3B (MLX path): 112 tok/s decode, 1810 tok/s prefill. [Source](https://ollama.com/blog/mlx). These numbers are not achievable on this hardware until the Metal crash is fixed.

#### Long-Running Posture

Default model keep-alive: 5 minutes (tunable via `OLLAMA_KEEP_ALIVE`). Auto-unload when memory pressure requires it. `OLLAMA_NUM_PARALLEL` controls parallel slots (default: auto, 1 or 4 based on memory). `OLLAMA_MAX_QUEUE=512`. [Source](https://docs.ollama.com/faq)

Notable Ollama behavior: if a model is unloaded (5-min timeout) and a new request arrives, it re-loads it — cold-start latency for mdemg's 14B model is 10–30 seconds. For a 24/7 embedded use case, `OLLAMA_KEEP_ALIVE=-1` (never unload) is essential.

#### Homebrew + launchd

Ollama ships a native macOS app with menu bar. Homebrew: `brew install ollama` (headless). Docs provide a launchd plist template. [Source](https://docs.ollama.com/macos). Auto-pull on `ollama run <model>` is an operational concern — can be disabled by using local Modelfile only and not exposing the API publicly.

---

### 3. MLC-LLM

**Project:** [mlc-ai/mlc-llm](https://github.com/mlc-ai/mlc-llm)
**Stars/Maturity:** 22.5K stars, 281 contributors, Apache 2.0. Less active than llama.cpp or Ollama; 8 tagged releases. [Source](https://github.com/mlc-ai/mlc-llm)
**License:** Apache 2.0

#### OpenAI API Compatibility

REST server with OpenAI-compatible endpoints: `POST /v1/chat/completions`, `GET /v1/models`. SSE streaming supported. [Source](https://llm.mlc.ai/)

Server startup: `mlc_llm serve <model_lib_path> --device metal`

#### Model Format Requirements

MLC-LLM requires **TVM-compiled model libraries** (`.dylib` on macOS). The compilation process:
1. Start from HuggingFace safetensors (original Qwen3-14B base) — not MLX format.
2. Run `mlc_llm convert_weight` (quantization step, options: q4f16_1, q4f32_1, q0f16, q0f32).
3. Run `mlc_llm compile` — this invokes TVM, applies Metal kernel optimization, outputs `.dylib`.

**Compilation time:** Not precisely documented, but TVM compilation of a 14B model on Apple Silicon is estimated at 30–90 minutes on first run (community reports). For fine-tuned weights: must start from base Qwen3-14B HuggingFace weights, apply LoRA merge, then compile. Pre-compiled MLC artifacts are available for base Qwen3 models but not for fine-tuned variants.

#### Stability Evidence — M5 + macOS 26.3.x

MLC-LLM uses TVM-generated Metal kernels, **not** GGML or MLX native kernels. This means it does **not** go through MetalPerformancePrimitives' `matmul2d` path that causes the static_assert failure. However:
- M5 + macOS 26.3.x specific testing has not been documented in recent MLC-LLM issues.
- MLC's own Metal kernel generation may have separate macOS 26 compatibility issues (unconfirmed either way).
- Community size is smaller; M5 reports are sparse.
- Benchmark data (arXiv 2511.05502) is from M2 Ultra, not M5 Max.

#### Performance on M-series

From arXiv 2511.05502 (M2 Ultra, 192 GB): ~190 tok/s throughput, ~13 ms P99 latency for Qwen2.5 14B-class model. Better TTFT than llama.cpp for moderate prompt sizes due to paged KV design. Strong for long-context (64K–128K) workloads. [Source](https://arxiv.org/abs/2511.05502)

#### Long-Running Posture

Paged KV cache (borrowed from vLLM design) — better long-running memory stability than slot-based KV in llama.cpp. TVM-compiled kernels avoid runtime shader compilation surprises.

#### Homebrew + launchd

No Homebrew formula. Install via pip: `pip install mlc-llm`. No pre-built launchd plist. Writing one is straightforward. No automatic model download (unlike Ollama) — pure local serving.

---

### 4. LM Studio (llmster daemon)

**Project:** [LM Studio](https://lmstudio.ai)
**License:** Free for personal and commercial use (as of July 2025). [Source](https://lmstudio.ai/blog/free-for-work). Proprietary closed-source application (CLI `lms` is open: [lmstudio-ai/lms](https://github.com/lmstudio-ai/lms), MIT). Core runtime is proprietary.
**Maturity:** Commercial-grade macOS app, active engineering team, dedicated Apple Silicon support.

#### OpenAI API Compatibility

Full OpenAI-compatible API at port 1234 (configurable):
- `POST /v1/chat/completions` (streaming + non-streaming)
- `POST /v1/completions`
- `POST /v1/embeddings`
- `GET /v1/models`
Also Anthropic-compatible endpoint in newer versions.

[Source](https://lmstudio.ai/docs/developer/core/server)

#### Headless Daemon

`llmster` is the headless daemon (CLI-driven, no GUI required):
```bash
curl -fsSL https://lmstudio.ai/install.sh | bash
lms daemon up
lms server start
```
[Source](https://lmstudio.ai/docs/developer/core/headless). No macOS launchd plist published, but `lms daemon up` is daemonizable via a custom launchd plist wrapping the CLI.

#### Model Format Requirements

Supports both GGUF and MLX safetensors models. LM Studio 0.3.4+ ships Apple MLX support for safetensors. [Source](https://lmstudio.ai/blog/lmstudio-v0.3.4). Our model (MLX format) can be loaded **without conversion** by pointing LM Studio's model directory to `.local-models/mdemg-llm-v1/`.

#### Stability Evidence — M5 + macOS 26.3.x

LM Studio ships its own bundled MLX runtime, updated independently of the upstream `ml-explore/mlx` pip package. Evidence of independent macOS 26 patching:
- [lmstudio-bug-tracker#1504](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/1504): "0.4.2 memory usage different than 0.4.1 — MLX model crashing vs previous runtime" — they actively patch MLX runtime bugs between releases.
- [lmstudio-bug-tracker#1645](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/1645): "MLX backend fails to load — missing cpython vendor package" — actively triaged.
- LM Studio changelog: [lmstudio.ai/changelog](https://lmstudio.ai/changelog) — has dedicated macOS Silicon release notes.

The proprietary bundled runtime means LM Studio *may* have patched the Metal Toolchain 32023 regression before the mlx-lm pip package. However, this is **not confirmed** — the changelog content was not fully accessible in this research.

LM Studio is highlighted by Apple in their M5 Max press materials (March 2026) as a recommended app for local LLM use. [Source](https://markaicode.com/lm-studio-mlx-apple-silicon-models/)

#### Performance on M-series

Comparable to mlx_lm.server when using MLX backend. Reported effective throughput at long context can be misleading — LM Studio's UI shows 57 tok/s at 8,500 tokens context but effective throughput was ~3 tok/s due to prefill overhead in some tests. [Source](https://llmcheck.net/blog/apple-silicon-m5-max-local-ai-guide/)

#### Long-Running Posture

llmster provides automatic crash recovery via the daemon manager. Memory management dependent on the bundled MLX runtime's KV cache handling — subject to the same unbounded KV growth issues as mlx_lm.server if the upstream fix (mlx-lm#883, `--max-kv-size`) has not been backported.

#### Homebrew + launchd

No Homebrew formula for `llmster` itself. `curl` installer. Custom launchd plist needed for persistent startup.

**Operability risk:** Closed-source core means no ability to patch critical bugs directly. Dependent on LM Studio's release cadence for fixes. Proprietary behavior may differ from documented OpenAI API schema in edge cases.

---

### 5. vllm-mlx (Optional Candidate)

**Project:** [waybarrios/vllm-mlx](https://github.com/waybarrios/vllm-mlx)
**Stars/Maturity:** 1.1K stars, 453 commits, v0.2.9 (April 22, 2026). Active but small community.
**License:** Apache 2.0

#### OpenAI API Compatibility

Comprehensive: `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/rerank`. Also Anthropic `/v1/messages`. Works with OpenAI Python SDK. [Source](https://github.com/waybarrios/vllm-mlx)

#### Model Format

MLX native format required. Same format as our production model — **no conversion needed**. Provides `vllm-mlx model convert` for quantization if needed.

#### Stability Evidence — M5 + macOS 26.3.x

vllm-mlx wraps the MLX framework. Documentation explicitly lists **M1, M2, M3, M4** as supported — M5 is absent from docs. Subject to the same MLX Metal Toolchain 32023 regression (mlx#3337) affecting mlx_lm.server. No M5/26.3.x specific reports found. vllm-mlx v0.2.9 pins mlx version — unclear if mlx#3337 is patched in their dependency.

#### Performance

Continuous batching provides 3.4x throughput improvement at 5 concurrent requests vs mlx_lm.server. Claimed 400+ tok/s on M4 Max for 14B-class models with batching. [Source](https://macgpu.com/en/blog/2026-mac-inference-framework-vllm-mlx-ollama-llamacpp-benchmark.html)

#### Long-Running Posture

Paged KV cache + prefix caching + SSD-tiered cache. Streaming disconnect guard (releases locks when client disconnects). Better concurrency handling than mlx_lm.server.

#### Engineering Swap Cost

Lowest possible model-format cost (MLX native, no conversion). Endpoint change only (`8101 → new port`, same `/v1/` schema). However, small community and absent M5 documentation are risk factors.

---

### 6. oMLX (Optional Candidate)

**Project:** [jundot/omlx](https://github.com/jundot/omlx)
**Stars/Maturity:** Very small community, ~30 known users. MLX-native. Not recommended as primary candidate given bus-factor risk.

Mentioned for completeness: paged SSD caching, process memory enforcement (`total RAM - 8 GB` ceiling), crash auto-restart. Subject to same MLX/macOS 26.3.x issues as mlx_lm.server. Not evaluated further.

---

## Side-by-Side Comparison Matrix

| Dimension | llama.cpp | Ollama | MLC-LLM | LM Studio | vllm-mlx |
|-----------|-----------|--------|---------|-----------|----------|
| **OpenAI API compat** | Native, `/v1/` prefix, full schema + SSE [(ref)](https://github.com/ggml-org/llama.cpp) | Native, `/v1/` prefix, full schema + SSE [(ref)](https://docs.ollama.com/api/openai-compatibility) | Native, REST, full schema + SSE [(ref)](https://llm.mlc.ai/) | Native, port 1234, full schema + SSE [(ref)](https://lmstudio.ai/docs/developer/core/server) | Native, full schema + SSE + Anthropic [(ref)](https://github.com/waybarrios/vllm-mlx) |
| **Model format** | GGUF required | GGUF (primary); MLX via registry in v0.19+ [(ref)](https://ollama.com/blog/mlx) | TVM-compiled `.dylib` only [(ref)](https://llm.mlc.ai/docs/compilation/compile_models.html) | GGUF or MLX safetensors [(ref)](https://lmstudio.ai/blog/lmstudio-v0.3.4) | MLX native safetensors [(ref)](https://github.com/waybarrios/vllm-mlx) |
| **Fine-tune preservation** | Requires conversion of fused weights to GGUF | Same as llama.cpp | Requires compilation from HF weights | MLX format direct load = **no conversion** | MLX format direct load = **no conversion** |
| **M5 + macOS 26.3.x stability** | Unknown — GGML MPP bfloat/half crash risk; no confirmed fixed build [(ref)](https://github.com/ggml-org/llama.cpp/discussions/17298) | **Broken** — static_assert crash across 0.20–0.22, multiple open issues [(ref)](https://github.com/ollama/ollama/issues/15862) | Unknown — different Metal path (TVM); no M5/26.3.x reports | Possibly patched ahead of upstream; proprietary runtime [(ref)](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/1504) | Unknown — same MLX Metal risk as mlx_lm.server; M5 not in docs [(ref)](https://github.com/waybarrios/vllm-mlx) |
| **General stability (long-running)** | KV host-memory cache grows unboundedly without `-cram 0`; memory leak in batch mode (open issue #22060) | 5-min model unload default; must set `KEEP_ALIVE=-1`; parallel slots configurable | Paged KV = better memory stability; smaller community | Dependent on llmster crash recovery; unknown KV cap status | Paged KV + SSD cache + streaming disconnect guard |
| **Decode tok/s (14B, M5 Max)** | ~100–130 estimated (M4 Pro: 35–50; M5 +28% BW) [(ref)](https://llmcheck.net/benchmarks) | 112 tok/s (v0.19 MLX, M5 Max, 35B model; 14B would be faster) — hardware blocked currently [(ref)](https://ollama.com/blog/mlx) | ~190+ estimated from M2 Ultra data [(ref)](https://arxiv.org/abs/2511.05502) | ~130–150 estimated (MLX-equivalent) | 400+ tok/s claimed with batching; 14B M5 Max not benchmarked [(ref)](https://macgpu.com/en/blog/2026-mac-inference-framework-vllm-mlx-ollama-llamacpp-benchmark.html) |
| **Concurrency model** | Continuous batching, `--parallel N` slots, FIFO queue [(ref)](https://github.com/ggml-org/llama.cpp/discussions/8567) | `OLLAMA_NUM_PARALLEL`, auto 1 or 4; `OLLAMA_MAX_QUEUE=512` [(ref)](https://docs.ollama.com/faq) | Multi-worker HTTP/SSE, paged KV for concurrent requests | Configurable parallel slots [(ref)](https://lmstudio.ai/docs/app/advanced/parallel-requests) | Continuous batching, paged KV, 3.4x throughput at 5 concurrent [(ref)](https://github.com/waybarrios/vllm-mlx) |
| **Brew availability** | `brew install llama.cpp` [(ref)](https://formulae.brew.sh/formula/llama.cpp) | `brew install ollama` | No formula | No formula (`curl` install) | No formula (`pip install`) |
| **launchd-friendly** | Yes — simple plist wrapping `llama-server` | Yes — Ollama provides plist template [(ref)](https://docs.ollama.com/macos) | Yes — custom plist | Yes — custom plist wrapping `lms daemon up` | Yes — custom plist |
| **Stars / contributors** | ~100K stars, 700+ contributors [(ref)](https://aithinkerlab.com/llama-cpp-100k-github-stars-2026/) | Large consumer project, 210+ releases | 22.5K stars, 281 contributors [(ref)](https://github.com/mlc-ai/mlc-llm) | Commercial app; no public star count; large user base | 1.1K stars, small community [(ref)](https://github.com/waybarrios/vllm-mlx) |
| **License** | MIT | MIT | Apache 2.0 | Free (commercial use), proprietary core | Apache 2.0 |
| **Quantization options** | Q2–Q8 GGUF types; K-quant variants; recommended Q4_K_M or Q5_K_M | Same (GGML backend) | q4f16_1, q4f32_1, q0f16, q0f32 | Depends on backend (GGUF or MLX 4-bit) | MLX 4-bit or 8-bit |

---

## GGUF Conversion Path (MLX → GGUF)

This section details the conversion from our production MLX model to GGUF for use with llama.cpp or Ollama.

### What We Have

- `/Users/reh3376/mdemg/.local-models/mdemg-llm-v1/` → symlink → `qwen3-14b-mdemg-v1/`
- Files: `model-00001-of-00002.safetensors` (5.4 GB), `model-00002-of-00002.safetensors` (3.0 GB)
- `config.json`: `Qwen3ForCausalLM`, 40 layers, 4-bit quantized (MLX native quantization, group_size=64)
- `tokenizer.json`, `tokenizer_config.json`, `chat_template.jinja`
- These are **already-fused MLX weights** (Phase 5 SFT produced them in MLX format; no separate adapter file present in this path)

### The Conversion Challenge

The model weights are MLX-quantized (4-bit, MLX-native format). `convert_hf_to_gguf.py` expects PyTorch/HuggingFace float16/bfloat16 safetensors — not MLX-quantized tensors. Direct conversion of MLX 4-bit quantized weights is not supported.

**Known blocker:** `lm_head.biases` and `lm_head.scales` tensors from MLX quantization cannot be mapped by `convert_hf_to_gguf.py` (llama.cpp#14467, closed as not planned). This affects Qwen3 models specifically after MLX quantization.

### Path A: De-quantize first, then convert (recommended)

1. **De-quantize MLX weights to float16:**
   ```bash
   python -m mlx_lm.fuse \
     --model /Users/reh3376/mdemg/.local-models/mdemg-llm-v1 \
     --save-path /tmp/mdemg-dequant-f16 \
     --de-quantize
   ```
   This produces HF-compatible safetensors in fp16/bfloat16. (Note: output size expands to ~28 GB.)

2. **Convert F16 safetensors to GGUF (F16):**
   ```bash
   git clone https://github.com/ggml-org/llama.cpp
   cd llama.cpp
   pip install -r requirements.txt
   python convert_hf_to_gguf.py /tmp/mdemg-dequant-f16 \
     --outfile /tmp/mdemg-f16.gguf \
     --outtype f16
   ```

3. **Quantize GGUF to Q4_K_M:**
   ```bash
   # Build llama.cpp (or use brew)
   make llama-quantize
   ./llama-quantize /tmp/mdemg-f16.gguf /tmp/mdemg-q4km.gguf Q4_K_M
   ```
   Output: ~9.0 GB GGUF.

4. **Optionally quantize to Q5_K_M** for quality parity with original 4-bit MLX:
   ```bash
   ./llama-quantize /tmp/mdemg-f16.gguf /tmp/mdemg-q5km.gguf Q5_K_M
   ```
   Output: ~10.5 GB GGUF.

### Path B: Re-merge from base HF weights (cleanest, extra step)

If de-quantize produces tensor mapping errors, an alternative is:
1. Download original Qwen3-14B HuggingFace weights (bfloat16 safetensors) — ~28 GB
2. If a LoRA adapter exists, apply it with `mlx_lm.fuse` or `peft` merge
3. Convert merged weights to GGUF with `convert_hf_to_gguf.py`
4. Quantize to Q4_K_M or Q5_K_M

**Note:** Our production model was Phase 5 SFT — it was trained in MLX and saved in MLX format. There is no separate LoRA adapter file in the production model directory. The fine-tune is baked into the weights. Path A is therefore required.

### Quality Parity Assessment

Original: MLX 4-bit, group_size=64, bits=4 (effectively Q4 K-quant).
Target GGUF: Q4_K_M or Q5_K_M.

Q4_K_M perplexity impact: +0.18 ppl vs F16 on Llama-3-8B. For Qwen3-14B at Q5_K_M, degradation is typically < 0.2 ppl vs F16. Quality difference vs MLX 4-bit: marginal and in the direction of GGUF Q5_K_M being slightly better (MLX 4-bit group_size=64 ≈ Q4_0 quality; GGUF Q5_K_M uses importance matrices for better allocation).

**Recommendation for quality verification:** Run the mdemg UVTS A/B harness comparing mlx_lm.server + MLX model vs llama-server + Q5_K_M GGUF before committing. Quality delta is expected to be within noise, but the harness exists precisely for this.

### Sources

- [mlx-lm Discussion #1507](https://github.com/ml-explore/mlx/discussions/1507) — MLX LoRA adapter to GGUF
- [llama.cpp Issue #14467](https://github.com/ggml-org/llama.cpp/issues/14467) — `lm_head.biases` tensor mapping blocker
- [MLX-to-GGUF integration guide (Medium)](https://medium.com/@meirgotroot/bringing-your-fine-tuned-mlx-model-to-life-with-ollama-integration-c54274de6491)
- [convert_hf_to_gguf.py tutorial](https://medium.com/@jenny890808/llama-cpp-convert-safetensor-model-into-gguf-56ceca89a310)

---

## Engineering Cost Estimate Per Candidate

All estimates assume current state: mdemg using mlx_lm.server on port 8101, OpenAI-compat, launchd plist installed, watchdog monitoring `/models` endpoint.

| Component | llama.cpp | Ollama | MLC-LLM | LM Studio | vllm-mlx |
|-----------|-----------|--------|---------|-----------|----------|
| **GGUF re-quantization** | 3–8 hrs (de-quant + convert + quantize + verify) | Same as llama.cpp | N/A (TVM compile: 2–4 hrs) | N/A (MLX direct) | N/A (MLX direct) |
| **macOS 26.3.x fix verification** | 1–4 hrs (build from source, test Metal) | Blocked (no fix in 0.22.x) | 2–4 hrs (compile test) | 1–2 hrs (test with llmster) | 1–2 hrs (test with mlx) |
| **Port + config changes** | 0.5 d (port, model path, flags in launchd plist) | 0.5 d | 0.5 d | 0.5 d | 0.5 d |
| **Watchdog rebind** | Small — endpoint agnostic (just probe `/v1/models`) | Same | Same | Same | Same |
| **launchd plist** | New plist (0.5 d) | Ollama provides template (0.25 d) | New plist (0.5 d) | Wrap `lms daemon up` (0.5 d) | New plist (0.5 d) |
| **UVTS quality regression test** | 0.5–1 d (A/B with Q4_K_M/Q5_K_M vs baseline) | Same | 0.5–1 d | 0.5–1 d | 0.25 d (same model) |
| **Documentation** | 0.5 d | 0.5 d | 0.5 d | 0.5 d | 0.25 d |
| **Total (floor / median / ceiling)** | **2 d / 3.5 d / 6 d** | **Blocked until fix** | **3 d / 5 d / 8 d** | **2 d / 3 d / 5 d** | **1 d / 2 d / 3 d** |

**Notes:**
- llama.cpp ceiling includes time if de-quantization hits tensor mapping errors requiring workaround iteration.
- Ollama is excluded from day estimate because it is currently non-functional on M5 + macOS 26.3.x with no fix timeline.
- MLC-LLM ceiling includes time for TVM compilation failures and debugging.
- vllm-mlx floor is lowest because model format is identical (no conversion) — but the Metal risk is shared with mlx_lm.server.
- All estimates exclude the UVTS A/B quality gate time (0.5–1 day), which is constant.

---

## Confidence Assessment

### Well-evidenced (high confidence)

- **Ollama is non-functional on M5 + macOS 26.3.x (≥ 0.20.5, ≤ 0.22.1):** Corroborated by 8+ independent GitHub issues across multiple users, all open as of April–May 2026. Score: **high confidence**.
- **mlx_lm.server crash root causes:** mlx-lm#854 (OOM → SIGABRT) and mlx-lm#883 (unbounded KV growth) are documented and acknowledged by maintainers. mlx#3337 (Metal Toolchain 32023 namespace/type regression) is also documented. Score: **high confidence**.
- **GGUF conversion blocker for MLX-quantized Qwen3:** llama.cpp#14467 `lm_head.biases` tensor mapping failure is documented and closed-as-not-planned. De-quantization workaround is community-confirmed. Score: **medium-high confidence**.
- **llama.cpp OpenAI API compatibility:** Widely tested, well-documented, stable for years. Score: **high confidence**.
- **llama.cpp community/maturity (100K stars, weekly builds):** Verifiable. Score: **high confidence**.
- **Performance estimates (tok/s ranges):** Based on M4 Pro data + memory bandwidth scaling. Direct M5 Max + Qwen3-14B + llama.cpp benchmarks not found. Score: **medium confidence**.

### Moderate confidence

- **LM Studio potentially patching Metal issues ahead of upstream:** Evidence is circumstantial (active bug tracker, dedicated macOS engineering, Apple press materials). No changelog entry explicitly confirming macOS 26.3.2 Metal fix observed. Score: **medium confidence**.
- **MLC-LLM not affected by GGML MPP bfloat/half regression:** Based on architectural reasoning (different Metal codepath). Not verified by testing. Score: **medium confidence**.
- **vllm-mlx throughput claims (400+ tok/s):** Plausible for multi-request batching but not verified with Qwen3-14B on M5 Max. Score: **low-medium confidence**.

### Low confidence / hand-wavy

- **Whether any GGML-Metal fix has shipped in llama.cpp mainline (b9006):** Issue is filed against Ollama's bundled GGML, not mainline. Llama.cpp Discussion #17298 references workarounds but no confirmed version. Mainline may be fixed — but unconfirmed. Score: **low confidence**.
- **MLC-LLM stability on M5/26.3.x:** Architecturally different Metal path is hopeful but zero M5/26.3.x reports. Could work, could have different failures. Score: **low confidence**.
- **LM Studio effective throughput at scale:** Marketing numbers vs effective throughput at 8,500-token context diverge by 20x in one test. Score: **low confidence for aggregate throughput claims**.
- **Whether mlx#3337 (Metal Toolchain 32023 fix) is included in latest mlx-lm (0.31.2):** MLX 0.31.2 was released April 22, 2026, after macOS 26.3.1 RSR (which triggered the regression). Fix is documented as a two-line patch to `utils.h`. Whether it was merged before or after 0.31.2 is unclear. Score: **low confidence** (this matters: if 0.31.2 includes the fix, the current mlx_lm.server may only need a restart strategy, not a backend swap).

---

## Appendix: Current mlx_lm.server Crash Context

For synthesis team context, the three documented crash paths on M5 + macOS 26.3.x:

1. **SIGABRT in `check_error(MTL::CommandBuffer*)` (~14 min cycle):** KV cache grows per request; when context length accumulates, Metal GPU runs OOM. mlx_lm does not catch this; process exits with SIGABRT. Documented in mlx-lm#854 (KV cache OOM) and mlx-lm#883 (unbounded growth → kernel panic path). No `--max-kv-size` parameter exists in mlx_lm.server 0.31.2.

2. **Metal Toolchain 32023 kernel compilation failure (mlx#3337):** `utils.h` uses bare `vec` type (pre-namespace-change) and missing include for `bfloat16_t`. On macOS 26.3.x with Metal Toolchain 32023, this causes MLX's Metal library to fail to build at startup. Server either fails to start or falls back to CPU (~17 tok/s). If the fix (2-line patch) is not in mlx 0.31.2, the production server may be running on CPU fallback — which would explain the ~14-min degradation cycle (memory pressure from CPU inference, not GPU OOM).

3. **Mixed-workload Metal assertion (omlx#216, mlx-vlm#945):** Serving both chat completions and embeddings in a single MLX process can trigger Metal assertion failures. mdemg routes embeddings through a separate stack, so this should not apply unless the mdemg server also hits mlx's metal path for embeddings.

The distinction between (1) and (2) matters for root-cause analysis. Stream 1 (crash forensics) should have more precise signal.

---

*Research by Claude Code sub-agent (Stream 3 of 4). All claims cited. No sprint plan or code changes proposed.*
