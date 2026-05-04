# Phase 13.5 MLX Server Stability — Crash Forensics Report

**Stream 1 (Internal Evidence): Crash Symbolication + TSDB/Log Correlation**
**Date:** 2026-05-02
**Analyst:** Research Stream 1 (automated)
**Sources:** 7× `.ips` crash reports, `~/.mdemg/logs/mlx-server.err.log`, `llm_interactions` TSDB table

---

## Crash Inventory

Note: the sprint plan describes 6 crashes; the diagnostic report directory contains 7 files (including `Python-2026-05-02-105021.ips`, the initial crash from the first server instance). The mlx-server.err.log records 8 distinct `kIOGPU` error lines (crashes 6 and 7 fired within the same millisecond on the same log line range). The table below covers all 7 `.ips` files.

| # | Crash Timestamp (ET) | Fault Offset in libmlx.dylib | IOAccelerator (Metal GPU) in vmSummary | Actual Launch→Crash (s) | KV Cache at Crash (seqs / GB) | Preceding LLM Task (TSDB) |
|---|----------------------|------------------------------|----------------------------------------|-------------------------|-------------------------------|---------------------------|
| 1 | 2026-05-02 10:50:20 | `0xe4c534` (+244 in `check_error`) | 50.0 GB | 1421 s | 49 seqs / **56.29 GB** | `ape.reflect` (tokens_in=6092, prompt 5702 tokens in progress) |
| 2 | 2026-05-02 11:41:10 | `0xe4c534` (+244 in `check_error`) | 11.6 GB | 3050 s | 86 seqs / **52.47 GB** | `ape.reflect` (tokens_in=5812; fired immediately after POST /chat/completions 200) |
| 3 | 2026-05-02 12:08:22 | `0xe4c534` (+244 in `check_error`) | 6.0 GB | 998 s | None logged (BrokenPipe cascade) / est ~40 GB | `ape.reflect` + `retrieval.query_classify` (prompt 5457 tokens, processing progress 5456/5457) |
| 4 | 2026-05-02 12:19:20 | `0xe4c534` (+244 in `check_error`) | 5.4 GB | 658 s | 63 seqs / **31.77 GB** | `ape.reflect` (tokens_in=5826, 81 786 ms), `retrieval.rerank_cross` (tokens_in=1639) concurrent |
| 5 | 2026-05-02 12:30:03 | `0xe4c534` (+244 in `check_error`) | N/A (not in vmSummary) | 643 s | 64 seqs / **33.88 GB** | `ape.reflect` (tokens_in=5799) + `retrieval.query_classify` + `retrieval.intent_translate` concurrent |
| 6 | 2026-05-02 12:47:18 | `0xe4c534` (+244 in `check_error`) | 4.0 GB | 1035 s | 61 seqs / **51.48 GB** | `ape.reflect` (tokens_in=5847, 56 111 ms — long decode) |
| 7 | 2026-05-02 13:07:42 | `0xe4c534` (+244 in `check_error`) | 68.3 MB | 1224 s | 58 seqs / **60.43 GB** | `ape.reflect` (prompt 5463 tokens processing: 5463/5463 just completed) |

**Symbolicated fault address (crash #2 example):**
```
libmlx.dylib base: 0x10b5a7000  (4 483 465 216)
imageOffset:       0x00e4c534   (14 992 692)
fault PC:          0x10c3f3534
atos result: mlx::core::gpu::check_error(MTL::CommandBuffer*) (in libmlx.dylib) + 244
```

**Full crashed-thread backtrace (representative — identical across all 7):**
```
[0]  __pthread_kill +8
[1]  pthread_kill +296
[2]  abort +124
[3]  __abort_message +132
[4]  demangling_terminate_handler() +280
[5]  _objc_terminate() +172
[6]  std::__terminate(void (*)()) +16
[7]  __cxxabiv1::failed_throw(__cxxabiv1::__cxa_exception*) +88
[8]  __cxa_throw +92
[9]  mlx::core::gpu::check_error(MTL::CommandBuffer*) +244   ← FAULT
[10] invocation function for block in MTL::CommandBuffer::addCompletedHandler(...) +48
[11] MTLDispatchListApply +52
[12] -[_MTLCommandBuffer didCompleteWithStartTime:endTime:error:] +612
[13] -[IOGPUMetalCommandBuffer didCompleteWithStartTime:endTime:error:] +220
[14] -[_MTLCommandQueue commandBufferDidComplete:...] +108
[15] __62-[IOGPUMetalCommandBuffer fillCommandBufferArgs:]_block_invoke.52 +172
[16] IOGPUNotificationQueueDispatchAvailableCompletionNotifications +136
[17] __IOGPUNotificationQueueSetDispatchQueue_block_invoke +64
[18-29] libdispatch serial queue drain / workloop
```
Thread queue: `com.Metal.CompletionQueueDispatch` (all 7 crashes).

**libmlx.dylib identity (all 7 identical):**
- UUID: `3df26163-ed40-3022-ae47-ee5632fb18df`
- Size: 17 137 664 bytes
- arch: `arm64` (not arm64e — note: Apple's Metal/AGX frameworks load arm64e)

**`uptime` field note:** Every crash report has `"uptime": 600000` (exactly 600 000 ms). The actual launch→capture durations measured from `procLaunch` and `captureTime` fields range from 642 s to 3050 s. The 600 000 value is a sentinel/cap in the crash reporter, not a real uptime. Do not use this field to infer process age.

---

## Crash Fingerprint Verdict

**VERDICT: Deterministic crash at a single instruction, driven by cumulative KV-cache memory pressure.**

Evidence:
1. **Identical fault offset `0xe4c534` across all 7 crashes.** The `imageOffset` is relative to the library load base (which varies per ASLR load), so this represents the same instruction byte in `mlx::core::gpu::check_error`. The libmlx.dylib UUID is also identical, confirming it is the same binary in every run.

2. **The crash is an explicit C++ exception throw, not a null deref or corruption.** Frame [8] is `__cxa_throw`, frame [9] is `check_error`. mlx explicitly calls `throw std::runtime_error(...)` when the Metal command buffer completion callback returns an error code. The SIGABRT arises because the exception propagates into a `std::terminate` handler on the GCD dispatch thread (a context where exception propagation is undefined).

3. **The mlx-server.err.log confirms the Metal error code:** `[METAL] Command buffer execution failed: Insufficient Memory (00000008:kIOGPUCommandBufferCallbackErrorOutOfMemory)`. Error code `0x00000008` = `kIOGPUCommandBufferCallbackErrorOutOfMemory`. This is the GPU's out-of-memory signal, returned asynchronously when the command buffer is retired.

4. **KV-cache growth is unbounded and monotonic within each server lifetime.** The cache grew from 0 GB to 56.29 GB over 1421 s before crash 1, accumulating ~1.15 GB per `ape.reflect` call (each `assistant` sequence adds ~1.13 GB). There is no eviction observed at any point in the log.

5. **KV cache sizes at crash vary (31.77–60.43 GB)**, not a single threshold. This refutes a fixed watermark and instead suggests the crash fires when the *next* Metal allocation attempt — the NEW prompt being processed at crash time — cannot be satisfied given the already-allocated KV cache. The crash fires during `Prompt processing progress` (i.e., GPU compute for a new request, not during a cache read). The total Metal footprint at crash = KV cache + model weights (~8.4 GB) + active compute buffers for the new ~5400-token prompt.

6. **IOAccelerator (Metal GPU) virtual memory in crash vmSummary varies from 68 MB to 50 GB**, directly reflecting how much KV cache had accumulated. The 50 GB crash (crash 1) is the most instrumented; the 68 MB crash (crash 7) reflects the vmSummary being captured after Metal had already begun releasing allocations in response to the abort signal.

---

## Pre-crash Activity Patterns

### Task type
**Every crash was immediately preceded by `ape.reflect` as the triggering task.** This is the RSIC self-improvement reflection task. All 16 mdemg LLM call sites are active during typical operation, but `ape.reflect` was the in-flight request when the Metal OOM fired in every case.

`ape.reflect` consistently sends ~5800–6100 `tokens_in` (large prompts). Cross-referencing with the log, these correspond to 5400–5700 token prompts in the `Prompt processing progress` lines, which aligns: each `ape.reflect` call processes the full RSIC context window.

### Concurrent calls at crash time
Crashes 3, 4, 5 show concurrent `retrieval.query_classify`, `retrieval.intent_translate`, and `retrieval.rerank_cross` calls active within the same second as the `ape.reflect` OOM crash. These are the mdemg retrieval pipeline tasks, which run in parallel when user queries arrive. The concurrency does not appear to be the trigger (since crashes 1, 2, 6, 7 show `ape.reflect` alone), but concurrent calls increase peak Metal working set.

### Prompt token sizes
`ape.reflect` token sizes are consistently in the 5800–6100 range (from TSDB). Each new prompt processing cycle needs to allocate KV tensors for ~5500 tokens on top of the existing KV cache. At ~1 GB per 1000 tokens (observed ~1.13 GB per assistant sequence), a 5500-token prompt requires ~6+ GB of *additional* Metal allocation during prefill, on top of a cache already holding 30–60 GB.

### Post-crash retry storm pattern
After each crash, the TSDB shows 100–107 `ape.reflect` 404 errors per minute for 20–50 minutes. These are the RSIC semaphore-gated retry loop retrying against the crashed server before the watchdog restores it. This is the expected behavior from the `ErrMLXDown` / `MLX_FAIL_FAST_ENABLED` circuit breaker, though the 100+/min rate suggests the circuit-breaker trip latency is not immediate after the first 404 burst.

### KV cache accumulation rate
- Crash 1 (first process, started fresh 10:26:39): 1421 s to grow 0→56 GB at ~0.04 GB/s
- Crash 2 (second process, started 10:50:23): 3050 s, 0→52 GB — slower because the first ~28 min were the RSIC retry storm (404 errors, no successful inference, no cache growth). Once actual `ape.reflect` calls succeeded (~11:18), growth resumed at similar ~1.13 GB/call rate.
- Crashes 3–7: 643–1224 s per lifetime, 0→31–60 GB. Shorter lifetimes because mdemg queued more work (multiple task types running, not just ape.reflect).

---

## Mac System Context

### System log (Metal/GPU events)
`log show --predicate 'composedMessage CONTAINS "kIOGPU" OR composedMessage CONTAINS "InsufficientMemory"'` returned no results for the crash window. This is expected: the kIOGPU error is delivered to the process via the IOKit user client completion callback, not broadcast to the system log.

### Current vm_stat
As of report time:
- Pages wired: 6 772 688 × 16 384 bytes = **110.9 GB wired**
- Pages in compressor: 3 427 698 (this is swap-compressed pages)
- Swapouts: 48 667 940 — significant swap activity over system lifetime

The 110 GB wired figure is consistent with a 128 GB system running Docker (Neo4j + TimescaleDB + other containers), mlx_lm.server (model loaded into Metal VRAM), and macOS system processes. On M-series Macs, the GPU and CPU share the same unified memory pool; Metal GPU allocations are wired and count toward the same physical RAM as CPU wired pages. When the KV cache grows to 50+ GB wired Metal + 8.4 GB model weights + ~2–4 GB model compute buffers, the total Metal footprint approaches or exceeds the available non-wired pool.

### AGX Metal driver version
From the crash binary images: `AGXMetalG17X` version `345.20.4`, CFBundleVersion `345.20.4`. This is the Apple GPU (AGX) Metal driver for the M5 Max (GPU family G17X). Metal framework version `371.5`. These are current versions for macOS 26.3.2 (25D2150); no anomaly detected in versioning.

---

## Confidence Assessment

| Finding | Confidence | Basis |
|---------|-----------|-------|
| All 7 crashes share fault at `libmlx.dylib:0xe4c534` (`check_error` +244) | **Certain** | Direct IPS file extraction; imageOffset identical across all 7; atos confirms symbol |
| Crash cause is `kIOGPUCommandBufferCallbackErrorOutOfMemory` | **Certain** | mlx-server.err.log contains exact error string immediately before each crash |
| KV cache grows unboundedly with no eviction | **Certain** | Log shows monotonic sequence count growth from 0 to 49–86 sequences; cache resets to 0 only on process restart |
| Each `ape.reflect` call adds ~1.13 GB to the Metal KV cache | **High** | Observed in crash-1 log: consistent ~1.13 GB per assistant sequence added |
| `ape.reflect` was the in-flight task when the Metal OOM fired in all 7 crashes | **High** | Log + TSDB: every crash happens during `Prompt processing progress` for a ~5400–5700 token prompt, TSDB confirms task=`ape.reflect` |
| KV cache size at crash varies (31–60 GB) — no single watermark threshold | **High** | Measured from 7 data points; variation explained by concurrent allocations during prefill |
| Concurrent retrieval tasks may accelerate peak Metal pressure | **Medium** | Observed in 3 of 7 crashes; other 4 show `ape.reflect` alone — not the primary trigger |
| mlx-lm prompt cache has no upper bound configured | **High** | `--prompt-cache-size 256` in plist — this flag controls cache entry COUNT (sequences), not size in GB. With 86 sequences × ~0.61 GB each = 52 GB. No GB-based eviction |
| IOAccelerator vmSummary variation (68 MB–50 GB) reflects Metal allocation state at snapshot time | **Medium** | Snapshot timing varies; Metal may have started releasing pages before vmSummary was captured |
| System Metal driver bug or macOS 26.3.2 regression as causal factor | **Low/Speculative** | No system log events, no driver anomalies found. Crash is entirely explicable as Metal OOM from unbounded KV cache growth |

**What is NOT confirmed by this stream:**
- Whether mlx-lm 0.31.3 (current version, +1 patch) contains a KV cache eviction fix. This requires Stream 2 (web research).
- Whether `--prompt-cache-size 256` is supposed to enforce a byte/GB cap or a sequence count cap. From observed behavior, it appears to be sequence count only.
- Whether the model weights (8.4 GB) are re-loaded on each restart or remain wired between restarts. The 3-second restart time (crash→server ready) suggests weights stay in Metal VRAM.

---

## Documents Accessed

- `~/Library/Logs/DiagnosticReports/Python-2026-05-02-{105021,114111,120823,121921,123004,124719,130743}.ips` — all 7 crash reports
- `~/.mdemg/logs/mlx-server.err.log` — 7061 lines, full session
- `~/.mdemg/logs/mlx-server.out.log` — empty (0 bytes)
- TSDB `llm_interactions` table — queried across all 7 crash windows
- `/Users/reh3376/mdemg/docs/development/post-ft-lora/sprint_plan_phase_13_5_mlx_stability.md` — §2 Problem Statement
- `atos` symbolication against `/Users/reh3376/.venv/mdemg-ft-lora/lib/python3.12/site-packages/mlx/lib/libmlx.dylib`
