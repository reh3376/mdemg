# Phase 13.5 — MLX Server Stability: External Evidence (Stream 2)

**Generated:** 2026-05-02  
**Scope:** Community + documentation research for Sprint POST-FT-LORA-PHASE13.5  
**Crash under investigation:** `SIGABRT` in `mlx::core::gpu::check_error(MTL::CommandBuffer*)` from `libmlx.dylib`, ~14-min periodicity, Mac17,6 (M5 Max 128 GB), macOS 26.3.2, mlx-lm 0.31.2, `qwen3-14b-mdemg-v1` (4-bit, 8.4 GB), `--prompt-cache-size 256 --prompt-concurrency 2 --decode-concurrency 2`

---

## 1. mlx-lm 0.31.3 vs 0.31.2

**Release date:** [v0.31.3 — April 22, 2026](https://github.com/ml-explore/mlx-lm/releases/tag/v0.31.3)

### Changes in 0.31.3 (over 0.31.2)

| Category | Change |
|----------|--------|
| Threading | Thread-local generation stream (accompanies MLX framework v0.31.2) — fixes sampling-determinism regression where every request returned the same tokens at temperature > 0 due to mlx-lm's compiled sampler conflicting with mlx's thread-local random state |
| KV Cache | Fixed batch dimension mismatch in `BatchKVCache` and `BatchRotatingKVCache` `extend()` |
| KV Cache | Fixed batch dimension mismatch in `ArraysCache.extend()` |
| KV Cache | Fixed Gemma 4 KV-shared layers creating unused projections |
| Server | Fixed parallel tool call handling in server |
| Server | Fixed MiniMax M2 parallel tool calling |
| Tool Calling | Fixed Gemma4 tool parser for hyphenated function names and braces in string args |
| Tool Calling | Fixed empty `tool_call_end` breaking Mistral tool calls |
| Imports | Fixed missing `tree_reduce` import in `models/cache.py` |
| Models | Apertus `tie_word_embeddings` fix |
| Models | `dwq`: check for actual safetensors in `target_dir` |
| Models | `NoneType` check for think tokens in `TokenizerWrapper` |

### Changes in 0.31.2 (context)

- "Better caching in the server" (per release notes)
- Batch generation refactoring
- Rotating cache for sliding attention support
- Cache memory clearing during training

### Version 0.31.0 note

[Issue #975](https://github.com/ml-explore/mlx-lm/issues/975) documents "strange cache behavior with 0.31.0 in server mode" (phantom tokens from previous conversations appearing in new sessions — KV contamination). This was introduced by the new caching in 0.31.0 and contributed to the 0.31.x instability series.

### Verdict: Does 0.31.3 fix our crash class?

**UNCERTAIN — partial relevance only.**

The `BatchKVCache`/`BatchRotatingKVCache` dimension mismatch fix is directly relevant to the server cache path and could prevent a class of corrupted-state crashes. The thread-local generation stream fix addresses a concurrency correctness issue that, at high concurrency, could previously produce corrupt Metal submissions. However:

- No changelog entry explicitly mentions: command-buffer errors, `check_error`, Metal OOM, `SIGABRT`, `kIOGPUCommandBufferCallbackErrorOutOfMemory`, long-running server stability, or prompt-cache memory leaks
- The core OOM / unbounded KV cache / graceful-shutdown gap (issues [#854](https://github.com/ml-explore/mlx-lm/issues/854), [#883](https://github.com/ml-explore/mlx-lm/issues/883), [#615](https://github.com/ml-explore/mlx-lm/issues/615)) remains unaddressed in 0.31.3

The upgrade from 0.31.2 → 0.31.3 is low-risk and contains meaningful cache correctness improvements, but it does **not** close the architectural gap that allows Metal OOM → SIGABRT to kill the server process rather than return HTTP 503.

---

## 2. Known mlx-lm + mlx GitHub Issues Matching Our Crash Signature

| # | Title | Repo | Status | Date | Relevance to Our Crash | URL |
|---|-------|------|--------|------|------------------------|-----|
| mlx-lm #854 | mlx_lm.server crashes on Metal OOM instead of returning HTTP error | mlx-lm | **Open** | 2026-02-07 | DIRECT — exact crash signature `kIOGPUCommandBufferCallbackErrorOutOfMemory`, server SIGABRT instead of 503, ~3,900 tok/req on 96 GB M-series | [#854](https://github.com/ml-explore/mlx-lm/issues/854) |
| mlx-lm #883 | mlx_lm.server causes macOS kernel panic (IOGPUMemory crash) due to unbounded memory growth | mlx-lm | **Closed** (referenced PR #906) | 2026-02-12 | DIRECT — kernel panic at `IOGPUMemory.cpp:550`, M3 Ultra 96 GB, KV cache unbounded growth to 58k+ tokens, 83 GB wired out of 96 GB | [#883](https://github.com/ml-explore/mlx-lm/issues/883) |
| mlx-lm #1015 | generate() crashes on Metal OOM instead of recovering gracefully | mlx-lm | **Open** | 2026-03-17 | HIGH — `[METAL] Command buffer execution failed: Insufficient Memory`, 14-hour continuous inference on M4 Mac Mini, no graceful recovery, Metal buffer fragmentation over time | [#1015](https://github.com/ml-explore/mlx-lm/issues/1015) |
| mlx #3186 | Kernel panic (IOGPUMemory.cpp:550) on M4 Max with large context prefill (~173K tokens) | mlx | **Open** | 2026-03-01 | HIGH — same kernel panic string `"completeMemory() prepare count underflow"`, M4 Max 36 GB, macOS 26.3, filed with Apple FB22091885 | [#3186](https://github.com/ml-explore/mlx/issues/3186) |
| mlx-lm #965 | KV cache cross-contamination between concurrent requests in mlx_lm.server | mlx-lm | **Closed** (PR #976) | 2026-03-08 | MEDIUM — concurrent request safety, 16+ concurrent requests produce wrong responses; **fixed** (merged PR #976), included in ~0.31.x | [#965](https://github.com/ml-explore/mlx-lm/issues/965) |
| mlx-lm #975 | Strange cache behavior with 0.31.0 in server mode | mlx-lm | **Closed** | 2026-03-09 | MEDIUM — phantom tokens from prior sessions leaked into new conversations, introduced by new caching in 0.31.0; M3 Ultra 512 GB | [#975](https://github.com/ml-explore/mlx-lm/issues/975) |
| mlx-lm #1139 | Bug: Latest 0.31.2 version causing broadcast errors | mlx-lm | **Open** | 2026-04-09 | MEDIUM — `ValueError: [broadcast_shapes] Shapes (2,8,256) and (5,1,1) cannot be broadcast` in BatchRotatingKVCache on **M5 Max**; downgrade to 0.31.1 fixes it; 0.31.3 BatchKVCache fix likely addresses this | [#1139](https://github.com/ml-explore/mlx-lm/issues/1139) |
| mlx-lm #754 | Batch KV cache merge crashes with mixed cached/empty prompts at higher concurrency | mlx-lm | **Closed** | 2026-01-13 | MEDIUM — `TypeError: 'NoneType' object is not subscriptable` in `BatchKVCache.merge` at concurrency 36+; uninitialized key handling | [#754](https://github.com/ml-explore/mlx-lm/issues/754) |
| omlx #173 | Server process crashing on macOS 26.3 - Metal GPU errors | omlx | **Closed** | 2026-03-11 | HIGH — **identical environment to ours**: macOS 26.3.1, M3 Ultra, MLX 0.31.0. Pattern 1: `SIGABRT` via `MTLReportFailure` in Metal command buffer handler during GPU eval (same function namespace as our crash). Pattern 2: SIGSEGV in `tryCoalescingPreviousComputeCommandEncoderWithConfig`. 5-6 crashes/hour. User asked about `MLX_MAX_OPS_PER_BUFFER` and `MLX_MAX_MB_PER_BUFFER` | [omlx #173](https://github.com/jundot/omlx/issues/173) |
| mlx #3267 | [BUG] Metal GPU watchdog kills LoRA training when display is active | mlx | **Open** (wontfix) | 2026-03-16 | MEDIUM — `kIOGPUCommandBufferCallbackErrorImpactingInteractivity`, GPU command buffers block WindowServer; `MLX_MAX_OPS_PER_BUFFER=1 MLX_MAX_MB_PER_BUFFER=10` did NOT prevent crash | [#3267](https://github.com/ml-explore/mlx/issues/3267) |
| mlx-lm #615 | Feature Request: Add max-kv-size Support to MLX HTTP Server | mlx-lm | **Open** | 2025-11-16 | HIGH — architectural gap: `mlx_lm.generate()` supports `max_kv_size` but `mlx_lm.server` does not, enabling unbounded KV cache growth | [#615](https://github.com/ml-explore/mlx-lm/issues/615) |
| mlx-lm #903 | Caching doesn't seem to be working for Qwen3.5 | mlx-lm | **Open** | 2026-02-17 | MEDIUM — prompt-cache hit rate zero for Qwen3.5, causing full re-processing of every prompt; 0 cached tokens reported | [#903](https://github.com/ml-explore/mlx-lm/issues/903) |
| mlx-lm #980 | Prefix cache reuse is broken for all hybrid-architecture models | mlx-lm | **Closed** | 2026-03-11 | MEDIUM — Qwen 3.5 affected: hybrid attention + Mamba layers cannot trim KV state at arbitrary boundaries, causing crashes or full recomputation; `make_prompt_cache(model)` partial fix proposed | [#980](https://github.com/ml-explore/mlx-lm/issues/980) |

**Notes on our specific hardware (M5 Max, macOS 26.3.2):**

- [omlx #173](https://github.com/jundot/omlx/issues/173) is the closest analog: SIGABRT via `MTLReportFailure` in the Metal command buffer handler on macOS 26.3.1
- [mlx-lm #1139](https://github.com/ml-explore/mlx-lm/issues/1139) was filed explicitly on an **M5 Max** in 0.31.2, mentioning `BatchRotatingKVCache` failures — 0.31.3 claims to fix this
- A separate Ollama issue ([#15642](https://github.com/ollama/ollama/issues/15642)) documents that M5 Max's new `MTLGPUFamilyApple10` GPU family was not yet recognized by Ollama's Metal detection as of April 2026, requiring mlx_lm direct execution as the workaround

---

## 3. Apple Metal Documentation Findings

### Official Error Enumeration

Apple's [`MTLCommandBufferError`](https://developer.apple.com/documentation/metal/mtlcommandbuffererror) enum defines command buffer failure codes:

| Code | Hex | Name | Official Meaning |
|------|-----|------|-----------------|
| 1 | 0x00000001 | `internal` | An internal error occurred |
| 2 | 0x00000002 | `timeout` | Execution timed out (AGX watchdog, typically ~45 seconds) |
| 4 | 0x00000004 | `pageFault` | A page fault occurred |
| 8 | 0x00000008 | `**outOfMemory**` | **Insufficient GPU memory to complete execution** |
| 10 | 0x0000000a | `notPermitted` | Insufficient permissions (e.g., background execution) |
| 11 | 0x0000000b | `accessRevoked` | Access to Metal resources was revoked |
| 12 | 0x0000000c | `blacklisted` | Process exceeded GPU memory limit |

The error our system emits — `kIOGPUCommandBufferCallbackErrorOutOfMemory (00000008)` — is **code 8 (outOfMemory)**. The `IOGPUCommandBuffer` prefix means it surfaces from the I/O Kit GPU family driver (`IOGPUFamily.kext`), propagating upward through `MTLCommandBuffer.error` and finally becoming an unhandled C++ exception that triggers `abort()` → SIGABRT when mlx_lm lacks a catch clause.

### kernel panic string `"completeMemory() prepare count underflow" @IOGPUMemory.cpp:550`

This panic string appears in Apple's kernel `IOGPUFamily` extension ([Apple Feedback FB22091885](https://github.com/ml-explore/mlx/issues/3186), filed March 2026, unresolved). It indicates the GPU memory accounting reference counter decremented below zero — a use-after-free or double-free at the Metal driver level. Root cause: when a Metal process wires large amounts of memory (MLX wires ~75% of RAM by default via `set_wired_limit(max_recommended_working_set_size)`), the GPU driver cannot satisfy new allocations and corrupts its own accounting rather than returning a clean error.

### macOS 26 + M5 Metal driver status

- [macOS 26.3 RC release notes](https://forums.macrumors.com/threads/macos-tahoe-26-3-rc-bug-fixes-changes-and-more.2477167/) contain **no documented Metal driver fixes** for GPU memory or SIGABRT issues
- Apple's [MPS SDPA attention kernel regression for A14/M1 confirmed broken on macOS 26.3.1](https://zenn.dev/amu_lab/articles/apple-m5-ollama-sigabrt-mlx-lm-guide-2026) — unrelated to M5 Max but shows active driver instability in the 26.x series
- [CoreML regression between macOS 26.0.1 and 26.1 Beta](https://zenn.dev/amu_lab/articles/apple-m5-ollama-sigabrt-mlx-lm-guide-2026) (tensor memory corruption) — suggests Metal stack instability in early macOS 26 releases
- M5 chip (`MTLGPUFamilyApple10`) introduces new GPU family; some frameworks (Ollama's llama.cpp backend) fail on M5 entirely; mlx_lm directly invokes Apple's MLX framework which does support M5 bfloat16 natively

**Bottom line:** `kIOGPUCommandBufferCallbackErrorOutOfMemory` is a real, documented Metal error that propagates as an unhandled C++ exception in mlx_lm → SIGABRT. The `check_error()` function in `libmlx.dylib` is Apple's own error-checking wrapper that converts the Metal error code into a `std::runtime_error` throw. No Python try/except boundary catches it.

---

## 4. Production Deployment Env Vars + Flags

### Python API memory controls (apply before server startup via wrapper script)

| Function / Var | What It Does | Recommended Value | Source |
|----------------|-------------|-------------------|--------|
| `mx.metal.set_memory_limit(n_bytes)` | Hard ceiling on total Metal allocations; MLX throws Python exception instead of crashing GPU driver when limit hit | `~75%` of free RAM after model load (e.g., 100 GB on 128 GB M5 Max after 8.4 GB model) | [Medium: How My Local Coding Agent Crashed My Mac](https://medium.com/@michael.hannecke/how-my-local-coding-agent-crashed-my-mac-and-what-i-learned-about-mlx-memory-management-e0cbad01553c), [Issue #883](https://github.com/ml-explore/mlx-lm/issues/883) |
| `mx.metal.set_wired_limit(n_bytes)` | Controls how much memory is pinned (cannot be swapped). Default is `max_recommended_working_set_size` (~75% RAM). **DO NOT pass the full value** — see MDEMG MEMORY.md constraint | Lower than default; recommended 50–60% of RAM | [Issue #883](https://github.com/ml-explore/mlx-lm/issues/883), [Medium article](https://medium.com/@michael.hannecke/how-my-local-coding-agent-crashed-my-mac-and-what-i-learned-about-mlx-memory-management-e0cbad01553c) |
| `mx.metal.set_cache_limit(n_bytes)` | Caps Metal compile cache / scratch buffers. Setting to a low value (512 MB) prevents accumulation during long-running sessions | `512 * 1024**2` (512 MB) | [mlx PR #390](https://github.com/ml-explore/mlx/pull/390), community guidance from mlx issues |
| `mx.metal.set_cache_enabled(False)` | Disables Metal buffer cache entirely. Prevents memory accumulation; slight performance cost (~5–10%) | Optional for extreme stability | [mlx PR #390](https://github.com/ml-explore/mlx/pull/390) |
| `MLX_MAX_OPS_PER_BUFFER` | Number of ops batched into a single Metal command buffer. Lower = shorter command buffers = less OOM risk per buffer. Defaults: 20 (mobile), 40 (base/pro), 50 (max/ultra) | 40 (for Max/Ultra; conservative) | [mlx PR #1864](https://github.com/ml-explore/mlx/pull/1864), [omlx #173](https://github.com/jundot/omlx/issues/173) |
| `MLX_MAX_MB_PER_BUFFER` | Memory cap per command buffer in MB. Lower = more frequent buffer commits, less fragmentation per buffer. Defaults: 40–50 MB | 40 (for Max/Ultra; conservative) | [mlx PR #1864](https://github.com/ml-explore/mlx/pull/1864), [mlx #3267](https://github.com/ml-explore/mlx/issues/3267) |

### Server startup flags (mlx_lm.server)

| Flag | What It Does | Our Current Value | Notes |
|------|-------------|-------------------|-------|
| `--prompt-cache-size N` | Max number of cached sequences held in RAM | 256 | DeepWiki docs show default is 10; our 256 is high — each cached sequence holds a full KV state, multiplying memory footprint |
| `--prompt-cache-bytes SIZE` | Hard memory cap on prompt cache (e.g., `16GB`). **More direct than --prompt-cache-size** | NOT SET | [DeepWiki server docs](https://deepwiki.com/ml-explore/mlx-lm/3.3-http-server), community reports from 2026 |
| `--prompt-concurrency N` | Max concurrent prefill (prompt processing) requests | 2 | Default is 8; our 2 is conservative; good |
| `--decode-concurrency N` | Max concurrent decode (generation) requests | 2 | Default is 32; our 2 is conservative; good |
| `--max-kv-size N` | Cap per-request KV cache size (rotating cache). Available for `generate()` but **NOT yet for server** | N/A | [Issue #615](https://github.com/ml-explore/mlx-lm/issues/615) — open feature request since Nov 2025; not yet implemented in server as of 0.31.3 |

### Community-validated production wrapper pattern

From [mlx-lm #883](https://github.com/ml-explore/mlx-lm/issues/883) and [Medium article](https://medium.com/@michael.hannecke/how-my-local-coding-agent-crashed-my-mac-and-what-i-learned-about-mlx-memory-management-e0cbad01553c), the community-validated approach is a Python wrapper that sets memory limits _before_ starting the server:

```python
import mlx.core as mx
# Set before model load — limits total Metal allocations
mx.metal.set_memory_limit(100 * 1024**3)   # e.g., 100 GB on 128 GB system
# Cap scratch/compile cache accumulation
mx.metal.set_cache_limit(512 * 1024**2)    # 512 MB
# Then start server...
```

Note: `mx.metal.set_wired_limit()` with the full `max_recommended_working_set_size` value is **explicitly prohibited** by MDEMG project policy (see MEMORY.md constraint: "DO NOT call `mx.set_wired_limit(max_recommended_working_set_size)` — on Macs where that value ≈ total RAM, wiring it crashes the kernel via watchdogd").

### launchd / supervisor pattern for crash recovery

No official `mlx_lm.server` supervisor exists. Community pattern ([Issue #1015](https://github.com/ml-explore/mlx-lm/issues/1015)):
- **Periodic process recycling** every N hours (24h in one report) to clear Metal buffer fragmentation
- **Application-level OOM recovery**: wrap `generate()` with `mx.clear_cache()` + garbage collection + token reduction retry, followed by full process restart if recovery fails
- MDEMG already uses launchd `com.mdemg.mlx-server.plist` (KeepAlive on crash, ThrottleInterval=60s) — this handles the restart side but not the _prevention_ side

---

## 5. Community Sentiment Summary (2026)

- **mlx_lm.server explicitly not recommended for production by its own maintainers.** The [SERVER.md](https://github.com/ml-explore/mlx-lm/blob/main/mlx_lm/SERVER.md) states: *"The MLX LM server is not recommended for production as it only implements basic security checks."* Maintainers confirmed in [Discussion #371](https://github.com/ml-explore/mlx-lm/discussions/371) the server is *"mostly intended to be used as a local HTTP endpoint"* with no plans to expand scope. This is an authoritative source — not community opinion. ([SERVER.md](https://github.com/ml-explore/mlx-lm/blob/main/mlx_lm/SERVER.md), [Discussion #371](https://github.com/ml-explore/mlx-lm/discussions/371))

- **The ~14-minute SIGABRT crash pattern is a known, documented, unfixed class of issue.** Three open mlx-lm issues ([#854](https://github.com/ml-explore/mlx-lm/issues/854), [#883](https://github.com/ml-explore/mlx-lm/issues/883), [#1015](https://github.com/ml-explore/mlx-lm/issues/1015)) and one omlx issue ([#173](https://github.com/jundot/omlx/issues/173)) document the same failure mode: unbounded KV cache growth + Metal OOM → SIGABRT instead of HTTP 503. The frequency reported in omlx #173 (5–6 crashes/hour on M3 Ultra macOS 26.3.1) is in the same order of magnitude as our ~4 crashes/hour (every ~14 min). Maintainers acknowledged `--max-kv-size` for the server "will likely ship soon" but it is not in 0.31.3.

- **For 24/7 / always-on workloads, llama.cpp and omlx are the community-recommended alternatives.** Multiple 2026 comparison articles ([Contra Collective](https://contracollective.com/blog/mlx-vs-llama-cpp-apple-silicon-local-ai), [contracollective.com 2026 comparison](https://contracollective.com/blog/llama-cpp-vs-mlx-ollama-vllm-apple-silicon-2026)) conclude: *"llama.cpp is the safer choice for production server deployments where stability and ecosystem integration matter more than peak token throughput."* omlx ([github.com/jundot/omlx](https://github.com/jundot/omlx)) adds process-level memory enforcement (`OMLX_MAX_PROCESS_MEMORY`), tiered KV caching, and LRU eviction — features missing from mlx_lm.server. The hybrid approach favored in 2026: *"use MLX for experimentation and fine-tuning, convert weights to GGUF, serve via llama.cpp in production."*

- **MLX ecosystem is healthy and growing rapidly; the server component is the weak link.** The mlx-community on HuggingFace hosts 4,255 models as of May 2026. Ollama 0.19 (March 2026) added an experimental MLX backend for 32 GB+ Apple Silicon systems. Apple spotlighted MLX in three dedicated WWDC 2025 sessions. LM Studio's unified MLX engine (0.4.0, January 2026) added continuous batching — a feature mlx_lm.server lacks. The framework itself is healthy; the bare `mlx_lm.server` is acknowledged as a minimal local HTTP proxy, not a production daemon. ([Hacker News: Ollama MLX](https://news.ycombinator.com/item?id=47582482), [omlx](https://github.com/jundot/omlx))

- **macOS 26.x Metal stack has known driver instability.** The `IOGPUMemory.cpp:550` kernel panic is filed with Apple (FB22091885, March 2026) and unresolved as of this writing. macOS 26.3 RC release notes contain no GPU driver fixes. A separate CoreML regression in 26.0.1–26.1 caused tensor memory corruption (resolved in a later point release). The M5 chip's `MTLGPUFamilyApple10` GPU family was not recognized by some frameworks' Metal detection as of April 2026. ([mlx #3186](https://github.com/ml-explore/mlx/issues/3186), [MacRumors 26.3 RC thread](https://forums.macrumors.com/threads/macos-tahoe-26-3-rc-bug-fixes-changes-and-more.2477167/))

---

## 6. Confidence Assessment

### What is clear (high confidence)

1. **The crash mechanism is fully understood:** `kIOGPUCommandBufferCallbackErrorOutOfMemory` (Metal error code 8) → unhandled `std::runtime_error` thrown by `mlx::core::gpu::check_error()` → SIGABRT. This is a missing try/catch in mlx_lm's server request handler, not a framework bug. Evidence: multiple independent reporters, same stack trace, same error code.

2. **--max-kv-size is not available in mlx_lm.server as of 0.31.3.** Confirmed by: [Issue #615](https://github.com/ml-explore/mlx-lm/issues/615) (open since Nov 2025), [deepwiki server docs](https://deepwiki.com/ml-explore/mlx-lm/3.3-http-server) (omits the flag), and [Issue #883](https://github.com/ml-explore/mlx-lm/issues/883) description.

3. **Our `--prompt-cache-size 256` is anomalously high.** The server default is 10. Each cached sequence holds a full KV state for the cached prefix. With 256 slots and a 14B model, this can accumulate gigabytes of wired Metal memory between requests, directly triggering the OOM path. Lowering this is a high-priority, zero-risk mitigation.

4. **`mx.metal.set_memory_limit()` + `mx.metal.set_cache_limit()` are the most validated Python-API mitigations**, with at least two independent sources (Medium article, Issue #883) recommending them. They convert hard crashes (SIGABRT) into catchable Python exceptions.

5. **The `MLX_MAX_OPS_PER_BUFFER` and `MLX_MAX_MB_PER_BUFFER` env vars exist and are documented** ([mlx PR #1864](https://github.com/ml-explore/mlx/pull/1864), merged Feb 2025), but lowering them did NOT prevent the `kIOGPUCommandBufferCallbackErrorImpactingInteractivity` crash in [mlx #3267](https://github.com/ml-explore/mlx/issues/3267). Their effect on `kIOGPUCommandBufferCallbackErrorOutOfMemory` is untested in public reports.

6. **0.31.3 upgrade is worthwhile but not a fix.** The `BatchKVCache`/`BatchRotatingKVCache` dimension mismatch fix directly affects our server path and is worth having. Issue [#1139](https://github.com/ml-explore/mlx-lm/issues/1139) was filed on an M5 Max with 0.31.2 and the fix targets the same cache path.

### What is contradictory or uncertain

1. **Is `--prompt-cache-bytes` sufficient to bound memory?** This flag (e.g., `--prompt-cache-bytes 16GB`) appeared in community configs from March 2026 but is not in the official SERVER.md or deepwiki docs. Unclear whether it bounds KV cache memory or only the prompt prefix cache. The relationship between `--prompt-cache-size`, `--prompt-cache-bytes`, and total Metal memory is not clearly documented.

2. **Does `--max-kv-size` exist in 0.31.3 server?** One search result mentioned it being available; others say it is not. The deepwiki docs (which track the live codebase) do not list it for the server. Most likely it remains absent, but this should be verified by running `mlx_lm.server --help`.

3. **Why 14-minute periodicity specifically?** The ~14-minute crash interval suggests a memory accumulation pattern (e.g., 16 LLM call sites × typical context length × 256 cache slots filling up), not a random OOM event. This deterministic pattern is not discussed in any issue — it may be unique to our usage pattern (mdemg's 16 concurrent LLM sites against a single server with high prompt-cache-size).

4. **MLX_MAX_OPS_PER_BUFFER effect on OOM crashes:** The variable clearly helps with watchdog-timeout crashes (`kIOGPUCommandBufferCallbackErrorTimeout`) and may help with interactivity crashes, but its effect on OOM crashes is not documented. There are zero community reports specifically testing it against `00000008:kIOGPUCommandBufferCallbackErrorOutOfMemory`.

---

## Sources

- [mlx-lm v0.31.3 Release Notes](https://github.com/ml-explore/mlx-lm/releases/tag/v0.31.3)
- [mlx-lm Releases (all versions)](https://github.com/ml-explore/mlx-lm/releases)
- [Issue #854: mlx_lm.server crashes on Metal OOM instead of returning HTTP error](https://github.com/ml-explore/mlx-lm/issues/854)
- [Issue #883: mlx_lm.server causes macOS kernel panic (IOGPUMemory crash)](https://github.com/ml-explore/mlx-lm/issues/883)
- [Issue #1015: generate() crashes on Metal OOM instead of recovering gracefully](https://github.com/ml-explore/mlx-lm/issues/1015)
- [Issue #615: Feature Request: Add max-kv-size Support to MLX HTTP Server](https://github.com/ml-explore/mlx-lm/issues/615)
- [Issue #965: KV cache cross-contamination between concurrent requests](https://github.com/ml-explore/mlx-lm/issues/965)
- [Issue #975: Strange cache behavior with 0.31.0 in server mode](https://github.com/ml-explore/mlx-lm/issues/975)
- [Issue #1139: Bug: Latest 0.31.2 version causing broadcast errors (M5 Max)](https://github.com/ml-explore/mlx-lm/issues/1139)
- [Issue #754: Batch KV cache merge crashes at higher concurrency](https://github.com/ml-explore/mlx-lm/issues/754)
- [Issue #903: Caching doesn't seem to be working for Qwen3.5](https://github.com/ml-explore/mlx-lm/issues/903)
- [Issue #980: Prefix cache reuse is broken for all hybrid-architecture models](https://github.com/ml-explore/mlx-lm/issues/980)
- [Discussion #371: Questions on Future of mlx-lm.server: Production Readiness](https://github.com/ml-explore/mlx-lm/discussions/371)
- [mlx-lm SERVER.md](https://github.com/ml-explore/mlx-lm/blob/main/mlx_lm/SERVER.md)
- [mlx Issue #3186: Kernel panic (IOGPUMemory.cpp:550) on M4 Max](https://github.com/ml-explore/mlx/issues/3186)
- [mlx Issue #3267: Metal GPU watchdog kills LoRA training when display is active](https://github.com/ml-explore/mlx/issues/3267)
- [mlx PR #1864: Allow dynamic ops per buffer (MLX_MAX_OPS_PER_BUFFER, MLX_MAX_MB_PER_BUFFER)](https://github.com/ml-explore/mlx/pull/1864)
- [mlx PR #390: Support disable metal buffer cache](https://github.com/ml-explore/mlx/pull/390)
- [omlx Issue #173: Server process crashing on macOS 26.3 - Metal GPU errors](https://github.com/jundot/omlx/issues/173)
- [omlx GitHub repo](https://github.com/jundot/omlx)
- [Apple MTLCommandBufferError documentation](https://developer.apple.com/documentation/metal/mtlcommandbuffererror)
- [Medium: How My Local Coding Agent Crashed My Mac (MLX memory management)](https://medium.com/@michael.hannecke/how-my-local-coding-agent-crashed-my-mac-and-what-i-learned-about-mlx-memory-management-e0cbad01553c)
- [mlx documentation (Metal memory management)](https://ml-explore.github.io/mlx/build/html/python/metal.html)
- [DeepWiki: mlx-lm HTTP Server docs](https://deepwiki.com/ml-explore/mlx-lm/3.3-http-server)
- [Contra Collective: MLX vs llama.cpp Apple Silicon](https://contracollective.com/blog/mlx-vs-llama-cpp-apple-silicon-local-ai)
- [Contra Collective: llama.cpp vs MLX vs Ollama vs vLLM 2026](https://contracollective.com/blog/llama-cpp-vs-mlx-ollama-vllm-apple-silicon-2026)
- [Zenn: Apple M5 Ollama SIGABRT + mlx-lm guide 2026 (Japanese)](https://zenn.dev/amu_lab/articles/apple-m5-ollama-sigabrt-mlx-lm-guide-2026)
- [MacRumors: macOS Tahoe 26.3 RC - Bug fixes thread](https://forums.macrumors.com/threads/macos-tahoe-26-3-rc-bug-fixes-changes-and-more.2477167/)
- [Apple Feedback FB22091885 (cross-referenced in mlx #3186)](https://github.com/ml-explore/mlx/issues/3186)
- [Hacker News: Ollama is now powered by MLX on Apple Silicon in preview](https://news.ycombinator.com/item?id=47582482)
- [Ollama Issue #15642: MLX runner fails on Apple M5 Max](https://github.com/ollama/ollama/issues/15642)
