# LLM-HEALTH-INVESTIGATION-001 — Live Tier-3 Verification

Real binary + real llama-server + real TSDB. Date: 2026-07-20.

## Binary + stack
- Binary rebuilt 2026-07-20 10:11 EDT (includes E1-E4).
- Server pid 94541 → then pid 94xxx after E5-C restart. All fresh binaries on `:10000`.
- Config: `RERANK_MIN_BUDGET_MS` default `12000` (unset in `.env`).

## E5-A — Recorder tags caller-cancellation live

Fresh binary boot at 14:11:26. First observed rerank_cross error:

```
14:11:59 | caller_canceled: http request: Post "http://127.0.0.1:8102/v1/chat/completions": context canceled
```

**Prefix `caller_canceled:` present.** Same raw error text preserved after the prefix for forensic use.

## E5-B — Alert-rule filter query returns real_errors=0

Simulating the RSIC alert rule's SQL (the `LLMPerformance` query with E3 filter):

```
task_name              | total | all_errors | real_errors | real_err_pct
retrieval.rerank_cross | 3     | 1          | 0           | 0.0
```

`real_errors` excludes rows with `error LIKE 'caller_canceled:%'` — this is what the RSIC evaluator sees. **error rate = 0.0% → alert cannot fire on this signal.** The 1 all_errors row (the E5-A cancellation) is filtered out of the count AND out of `MAX(time) FILTER (...)` for the SUPERVISOR-002 recency gate.

## E5-C — Pre-check skip fires when budget insufficient

To trigger the skip deterministically (default `12000` is smaller than typical remaining budget on a quiet system): temporarily set `RERANK_MIN_BUDGET_MS=60000` in `.env`; restart. Result:

```
$ curl -X POST /v1/memory/retrieve ... (retrieve budget 20s)
{
  "results": [5 rows],
  "rerank_latency_ms": 0,       # skip fired
  "rerank_enabled": true
}

# Server log:
WARN rerank skipped: insufficient budget
     remaining_ms=19534 min_budget_ms=60000 space_id=mdemg-dev candidates=15
```

**Fail-open confirmed** — retrieval returned 5 results (RRF-ordered, no LLM rerank) instead of the erroring rerank path. **No `llm_interactions` error row** landed for the skipped call (verified: after `rerank_ms=0`, no new caller_canceled or real_error row appeared for that trace).

Override reverted post-verification (`RERANK_MIN_BUDGET_MS=12000` default active).

## E5-D — Baseline: no new alert_llm_health fires

Fresh binary running for 30+ minutes on default budget:
- 3 rerank_cross calls
- 1 caller_canceled (tagged; filtered by alert rule)
- 0 real errors
- **0 new `alert_llm_health` alerts fired since restart** (verified via alert file inspection — historical fires from the pre-restart binary remain in `current.json` but no fresh timestamps).

## Verification summary

| Check | Result |
|---|---|
| Recorder tags context.Canceled / DeadlineExceeded with `caller_canceled:` prefix | ✅ (E5-A live) |
| Real HTTP errors NOT tagged (Tier-1 pin) | ✅ (recorder_classify_test.go) |
| Alert-rule filter excludes caller_canceled from error count | ✅ (E5-B SQL) |
| Pre-check skip fires when remaining ctx < min budget | ✅ (E5-C forced via override) |
| Fail-open returns pre-rerank RRF results (no error, no wasted LLM slot) | ✅ (E5-C) |
| Full go test ./... green | ✅ |
| No new alert_llm_health fires since restart | ✅ (30-min window; 0 new fires) |

All 5 acceptance criteria met.

## Follow-ups noted during E5
- **Pre-check skip rate on default budget will be low** in the quiet steady state — most rerank calls land with >12s remaining. That's fine; the skip is a safety net for slow-half scenarios, not a common path. Track skip-rate as a follow-up gauge if operator wants an early-warning signal for budget pressure.
- **Raising `RETRIEVE_TIMEOUT_MS` 20 → 30** remains a data-decided follow-up. Recommend only if the skip-WARN log rate becomes non-negligible in production; the current fix + tagged classification is the honest baseline.
