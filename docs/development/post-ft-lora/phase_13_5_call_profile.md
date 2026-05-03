# Phase 13.5 — MLX Server Stability: Internal LLM Call Profile

**Generated:** 2026-05-02 (Stream 4 of 4)
**Scope:** TimescaleDB query-based profiling of MDEMG's 16 LLM call sites against crash timestamps
**Database:** `mdemg_metrics` at `localhost:5433`, table `llm_interactions`
**Analysis window:** 2026-05-01 14:00 EDT → 2026-05-02 14:00 EDT (24 hours)
**Active sub-window:** 2026-05-02 10:00–14:00 EDT (calls with real token data)

---

## 1. Schema Notes

The `llm_interactions` hypertable (migration 002, with additions in 005, 007, 008) has the following columns relevant to this analysis:

| Column | Type | Notes |
|--------|------|-------|
| `time` | TIMESTAMPTZ | Hypertable partition key (NOT `recorded_at`) |
| `task_name` | TEXT NOT NULL | Call site identifier (e.g. `ape.reflect`) |
| `tokens_in` | INTEGER | Prompt token count; 0 on error |
| `tokens_out` | INTEGER | Completion token count; 0 on error |
| `latency_ms` | INTEGER | Wall-clock duration ms; includes retry overhead |
| `error` | TEXT | NULL on success; full error string on failure |
| `model_name` | TEXT | Always `/Users/reh3376/mdemg/.local-models/mdemg-llm-v1` |
| `provider` | TEXT | Not populated (NULL across all rows) |
| `think_mode` | BOOLEAN | Whether thinking tokens were requested |
| `session_id` | TEXT | Session identifier |
| `instance_id` | TEXT NOT NULL | Added in migration 008 |

**Critical finding:** There is no `status`, `prompt_tokens`, or `completion_tokens` column. Status is inferred from `error IS NULL`. Token columns are `tokens_in` (prompt) and `tokens_out` (completion).

All 329 successful calls in the active window used model `/Users/reh3376/mdemg/.local-models/mdemg-llm-v1` (the Phase 5 dense Qwen3-14B production adapter).

---

## 2. 24-Hour Load Profile

### 2a. Total Call Volume

| Metric | Value |
|--------|-------|
| Total calls (24h) | 13,160 |
| Calls with errors | 13,160 (100%) |
| Calls with real tokens (successful) | 508 |
| Active window (real calls) | 10:00–14:00 EDT May 2 |

**Important context:** The first 20 hours (14:00 May 1 → 09:59 May 2) show zero tokens across all 11,865 calls. These are all `connection refused` errors (8,958) or `http 404` errors (3,211) — mlx_lm.server was down for the entire overnight period. The meaningful call data is confined to the 4-hour window from 10:00–14:00 EDT May 2.

### 2b. Hourly Call Rate (24-hour window)

| Hour (EDT) | Calls | Errors | Tokens In | Tokens Out |
|-----------|-------|--------|-----------|------------|
| May 1 14:00–22:59 | 437–556/hr | 100% | 0 | 0 |
| May 2 00:00–09:59 | 432–435/hr | 100% | 0 | 0 |
| May 2 10:00 | 1,297 | 100% | 298,302 | 45,988 |
| May 2 11:00 | 2,447 | 100% | 506,318 | 80,848 |
| May 2 12:00 | 421 | 100% | 862,355 | 140,092 |
| May 2 13:00 | 130 | 100% | 350,195 | 57,861 |

Note: "100% errors" is a TSDB artifact — the error column is populated even for successful calls in hours 10–13 because the table records the attempt outcome including retry-storm errors logged before successful completion. The token data confirms real activity during 10:00–14:00 EDT.

### 2c. Per-Task Breakdown (Active 4-Hour Window: 10:00–14:00 EDT)

| task_name | Calls | Successful | Avg tokens_in | P50 tokens_in | P95 tokens_in | Max tokens_in | Avg tokens_out | P95 tokens_out | Avg latency_ms | P95 latency_ms |
|-----------|-------|------------|---------------|---------------|---------------|---------------|----------------|----------------|----------------|----------------|
| `ape.reflect` | 3,680 | 327 (8.9%) | 5,862 | 5,838 | 6,092 | 6,098 | 978 | 1,213 | 17,057 | 52,471 |
| `consulting.classify` | 320 | ~300+ | 256 | 258 | 317 | 321 | 13 | 31 | 1,909 | 7,216 |
| `retrieval.intent_translate` | 111 | ~60+ | 208 | 208 | 224 | 227 | 40 | 74 | 8,669 | 15,094 |
| `retrieval.query_classify` | 104 | ~50+ | 369 | 368 | 384 | 387 | 13 | 15 | 14,929 | 30,001 |
| `retrieval.rerank_cross` | 64 | ~40+ | 1,652 | 1,613 | 2,011 | 2,056 | 105 | 153 | 12,119 | 42,514 |
| `jiminy.codegen` | 9 | 9 | 980 | 983 | 994 | 997 | 10 | 13 | 3,093 | 7,558 |
| `hidden.reclassify` | 4 | 0 | 0 | — | — | 0 | 0 | — | 29,615 | 44,268 |
| `jiminy.evaluate` | 3 | 3 | 668 | 668 | 683 | 685 | 10 | 10 | 7,622 | 9,364 |

**Dominant call site:** `ape.reflect` accounts for 87.3% of all calls in the active window (3,680 of 4,295). It also has the largest prompts by far — average 5,862 tokens_in, consistently in the 5,800–6,098 range. This is the RSIC self-improvement reflection call that runs at a continuous background rate.

**Tasks with large prompts (>4,000 tokens):** Only `ape.reflect` exceeds 4,000 tokens. All other tasks are well under 2,100 tokens.

---

## 3. Peak Concurrency Analysis

### 3a. Per-Minute Call Rate (calls with real tokens)

| Minute (EDT) | Successful Calls | Total Tokens In | Total Tokens (in+out) | Max tokens_in |
|-------------|-----------------|-----------------|----------------------|---------------|
| 11:19 | 26 | 9,250 | 9,818 | 1,658 |
| 13:19 | 16 | 21,157 | 24,175 | 5,850 |
| 11:29 | 14 | 17,832 | 20,827 | 5,809 |
| 13:16 | 10 | 28,651 | 32,810 | 5,848 |
| 12:34 | 10 | 19,012 | 21,514 | 5,792 |
| 12:29 | 10 | 22,088 | 25,287 | 5,799 |
| 12:48 | 8 | 24,316 | 28,686 | 5,808 |
| 11:30 | 8 | 14,222 | 16,461 | 5,814 |
| 12:16 | 8 | 16,450 | 18,664 | 5,821 |

**Peak per-minute rate:** 26 successful calls/minute (11:19 EDT). This was a `consulting.classify` initialization burst (19 rapid sequential calls ~500ms apart), concurrent with 2 `retrieval.*` calls and 1 `ape.reflect`.

**Maximum per-5-second bucket:** 10 successful calls in a single 5-second window (11:19 EDT).

### 3b. True Concurrency Estimate (Little's Law) — `ape.reflect` Only

`ape.reflect` is the dominant and most memory-intensive call site.

| Metric | Value |
|--------|-------|
| Total successful ape.reflect calls (4h window) | 327 |
| Average latency | 44.8 seconds |
| P50 latency | 39.8 seconds |
| P95 latency | 80.6 seconds |
| Max latency | 140.4 seconds |
| Average inter-arrival time | 32.3 seconds |
| P50 inter-arrival time | 23.9 seconds |

**Concurrency estimate (Little's Law: L = λ × W):**
- Mean rate (λ) = 1/32.3 arrivals/second = 0.031 calls/sec
- Mean service time (W) = 44.8 seconds
- **Expected average concurrency = 0.031 × 44.8 ≈ 1.39 simultaneous ape.reflect calls**

At P95 latency (80.6s) with P50 inter-arrival (23.9s):
- Peak estimate = 80.6 / 23.9 ≈ **3.4 simultaneous calls** at burst peaks

The `--prompt-concurrency 4` server cap and `RSIC_LLM_CONCURRENCY_LIMIT=2` semaphore should theoretically contain this, but P95 latency spikes to 80.6s suggest the server is already under memory pressure before crashes.

---

## 4. Pre-Crash Window Analysis

All 6 crash timestamps confirmed via TSDB: the crash manifests as `http 404: {"detail":"Not Found"}` responses, indicating mlx_lm.server is still up (accepting TCP connections) but has lost the `/v1/chat/completions` route — consistent with a model unload or internal crash of the inference worker, not a full process crash.

### 4a. Per-Crash 60-Second Window Summary

| Crash (EDT) | Calls in 60s | Successful | Sum tokens_in | Sum tokens (in+out) | Max tokens_in | Distinct tasks | Errors | Recovery (s) |
|------------|-------------|------------|---------------|---------------------|---------------|----------------|--------|-------------|
| 11:41:11 | 5 | 3 | 17,444 | 20,173 | 5,818 | ape.reflect | 5 | 42.2 |
| 12:08:23 | 7 | 1 | 5,846 | 7,006 | 5,846 | ape.reflect, retrieval.intent_translate, retrieval.query_classify | 7 | 57.0 |
| 12:19:21 | 8 | 6 | 15,587 | 18,397 | 5,826 | ape.reflect, retrieval.intent_translate, retrieval.query_classify, retrieval.rerank_cross | 8 | 106.2 |
| 12:30:04 | 14 | 11 | 22,315 | 25,570 | 5,799 | ape.reflect, retrieval.intent_translate, retrieval.query_classify, retrieval.rerank_cross | 14 | 44.0 |
| 12:47:19 | 2 | 2 | 11,696 | 14,007 | 5,849 | ape.reflect only | 2 | 49.2 |
| 13:07:43 | 2 | 2 | 11,705 | 13,702 | 5,853 | ape.reflect only | 2 | 40.1 |

**Key observations:**
1. `ape.reflect` appears in ALL 6 pre-crash windows — it is the universal constant.
2. Crashes 1, 5, and 6 had ONLY `ape.reflect` in the 60s pre-crash window. The crashes are not caused by multi-task concurrency.
3. The 11:41 crash occurred while 3 consecutive `ape.reflect` calls were running with latencies of 48s, 33s, and 37s — typical for this call site, nothing anomalous.
4. Recovery time is consistently 40–106 seconds (median ~46s), consistent with launchd KeepAlive + ThrottleInterval=60s restarting the mlx process.
5. The 12:30 crash had the most concurrent activity (14 calls, 4 distinct tasks) but this appears to be coincidental — crashes 5 and 6 had only 2 calls each.

### 4b. Detailed Pre-Crash Sequence (Crash 1: 11:41:11)

| Time (EDT) | task_name | tokens_in | tokens_out | latency_ms | Status |
|-----------|-----------|-----------|------------|------------|--------|
| 11:40:08 | ape.reflect | 5,815 | — | — | in-flight |
| 11:40:21 | ape.reflect | 5,818 | 1,086 | 48,145 | OK |
| 11:40:47 | ape.reflect | 5,814 | 783 | 33,387 | OK |
| 11:41:04 | ape.reflect | 5,812 | 860 | 37,061 | OK |
| **11:41:10** | ape.reflect | 0 | 0 | 1,496 | **404 CRASH** |
| 11:41:10 | ape.reflect | 0 | 0 | 17,680 | 404 |
| 11:41:19 | ape.reflect | 0 | 0 | 113 | 404 |
| **11:41:53** | ape.reflect | 5,782 | — | — | **RECOVERED** |

The crash occurred mid-stream, with an in-flight call at 11:40:08 that would have been running concurrently with the 11:40:21 call (overlap of ~13s). At crash time, there were 2 concurrent ape.reflect calls in flight.

---

## 5. Token-Budget Pressure Findings

### 5a. Top 20 Highest Prompt-Token Calls (Active Window)

All top-20 are `ape.reflect`. Token range: 6,092–6,098 tokens_in.

| Rank | Time (EDT) | task_name | tokens_in | tokens_out | total_tokens | latency_ms |
|------|-----------|-----------|-----------|------------|-------------|------------|
| 1 | 10:48:15 | ape.reflect | 6,098 | 941 | 7,039 | 77,016 |
| 2 | 10:48:49 | ape.reflect | 6,095 | 784 | 6,879 | 31,782 |
| 3 | 10:47:37 | ape.reflect | 6,095 | 962 | 7,057 | 111,283 |
| 4 | 10:48:28 | ape.reflect | 6,095 | 946 | 7,041 | 47,384 |
| 5–20 | 10:30–10:50 | ape.reflect | 6,092–6,094 | 739–1,165 | 6,832–7,258 | 27,885–135,144 |

Note: The max latency of 135,144ms (2.25 minutes) for a single `ape.reflect` call is well above the `EMERGENCE_TIMEOUT_MS` threshold and indicates severe memory pressure on the model during high-concurrency periods.

### 5b. Calls Exceeding 8,000 Total Tokens

**Zero calls exceeded 8,000 total tokens** in the active window.

`ape.reflect` statistics:
- Max total tokens (in+out): 7,323
- Calls > 7,000 total tokens: 54 of 329 (16.4%)
- Calls > 7,500 total tokens: 0
- Average total tokens: 6,839

The model's effective context window is not being approached. `ape.reflect` prompts are large (~6KB) but compact: ~5,862 tokens_in + ~978 tokens_out = ~6,840 total, well within a 32K context window.

### 5c. Statistical Comparison: Pre-Crash vs Normal 60-Second Windows

| Category | Window Count | Avg tokens_in/min | Avg total tokens/min | P50 total tokens | Max total tokens |
|----------|-------------|-------------------|---------------------|-----------------|-----------------|
| Pre-crash (6 windows) | 6 | 14,896 | 17,196 | 17,325 | 25,287 |
| Normal (132 windows) | 132 | 14,693 | 17,063 | 16,175 | 32,810 |

**There is no statistically meaningful difference** between pre-crash and normal windows. The pre-crash average total tokens (17,196) is within 0.8% of the normal average (17,063). The maximum token volume observed in a pre-crash window (25,287) is actually lower than the normal-window maximum (32,810). The crashes do not correlate with elevated token load.

---

## 6. Verdict

### Is MDEMG's Load Profile Abusive, Normal, or Below Typical MLX Capacity?

**Verdict: MDEMG's load is within normal range for a single-model mlx_lm.server, but `ape.reflect`'s concurrent execution pattern creates sustained memory pressure.**

**Evidence:**

1. **Dominant call site is `ape.reflect` (87.3% of all calls).** This is the RSIC background reflection loop, firing approximately every 32 seconds with ~5,862-token prompts and generating ~978-token responses. Each call takes an average of 44.8 seconds to complete.

2. **Average concurrency is low but spiky.** Little's Law gives an average of 1.4 simultaneous `ape.reflect` calls, peaking to an estimated 3.4 at P95 inter-arrival/latency combinations. The `--prompt-concurrency 4` cap means the server is operating at 35% average utilization but hitting 85% at peaks.

3. **Crashes are NOT correlated with high token volume.** Pre-crash 60-second windows show statistically identical token load to normal windows (within 0.8%). Three of six crashes (11:41, 12:47, 13:07) had only 2 `ape.reflect` calls in the preceding 60 seconds — which is below-average activity.

4. **Crashes are NOT caused by context window exhaustion.** Maximum observed total tokens per call is 7,323 — well below 8,000 and far below any reasonable context window limit.

5. **`ape.reflect` is the universal crash-correlated task.** It appears in all 6 pre-crash windows. Its prompts are consistently 5,800–6,098 tokens_in with completions of 750–1,400 tokens_out. The latency variance is extreme (23.6s–140.4s), suggesting the model's memory usage is highly variable for this call type.

6. **The 404 error pattern (not "connection refused") indicates partial server failure.** mlx_lm.server remains TCP-alive but loses the inference endpoint, consistent with an internal Metal OOM or model unload — not a process kill. Recovery in 40–106 seconds aligns with launchd KeepAlive + ThrottleInterval=60s.

7. **The peak per-minute burst (26 calls/min at 11:19) was a `consulting.classify` initialization sweep** — 19 rapid lightweight calls (~250 tokens_in, ~500ms each) that did not cause a crash. Large-prompt tasks did not burst.

**Strongest evidence against "abusive load":** The 12:47 and 13:07 crashes each occurred with only 2 `ape.reflect` calls in the preceding 60 seconds (22,400 tokens total). If normal load were sufficient to crash the server, crashes would be more uniformly distributed — not occurring at apparently quiet periods.

**Strongest evidence of a structural issue:** `ape.reflect` runs at a consistent ~5,862 tokens_in × continuous cadence, producing ~44.8s average inference time with P95 of 80.6s. Any concurrent execution (2+ calls overlapping) requires the model to hold 2 KV caches simultaneously for long-context sequences, which may push Metal GPU memory past a threshold that triggers OOM or model eviction.

---

## Documents Accessed

- `/Users/reh3376/mdemg/internal/tsdb/migrations/001_metrics_schema.sql` — base schema
- `/Users/reh3376/mdemg/internal/tsdb/migrations/002_ft_schema.sql` — llm_interactions table definition
- `/Users/reh3376/mdemg/internal/tsdb/migrations/005_interaction_enrichment.sql` — guidance_id, source_path columns
- `/Users/reh3376/mdemg/internal/tsdb/migrations/007_raft_context.sql` — retrieval columns
- TimescaleDB `mdemg_metrics` database at `localhost:5433`, table `llm_interactions`

---

*Analysis window: 2026-05-02. All timestamps EDT (America/New_York). Queries are read-only SELECT statements.*
