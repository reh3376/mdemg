# LLM-HEALTH-INVESTIGATION-001 — Follow-up Closure (data-decided, 2026-07-21)

The sprint disclosed two conditional follow-ups. Both are closed as NO-OP by live data:

## 1. `RETRIEVE_TIMEOUT_MS` 20→30s — NO (condition not met)
Decision rule was "only if skip-rate becomes non-negligible." Measured: the pre-check
has produced **zero organic skips** since shipping — the only 2 WARN lines in the
server log are the NEURAL-RERANK-PRECHECK-001 forced-override smoke tests
(`min_budget_ms=30000`). The default 12000ms floor has never naturally triggered.

## 2. Skip-rate gauge — NO (would be a flatline-zero panel)
With zero organic skips, a dedicated gauge is the writerless/flatline panel class
DASHBOARD-TRUTH-002 removed elsewhere. Observability already exists:
- the skip WARN log line (with `provider` field, grep-able)
- rerank fill rate on the RSIC Pipeline Health bargauge (live 87.8% last 24h,
  UP from 76.8% the prior 24h)
- `caller_canceled` counts in `llm_interactions` (filtered from alerts + panels)

## Residual observation
34/279 (12.2%) rerank calls in the last 24h were caller-cancelled — concentrated in
today's three benchmark runs that saturated llama-server (rerank latency pushed past
p99 while budgets were legal at dispatch). Known fails-open transient; retrieval
returns pre-rerank RRF results; no alert noise (the E1/E3 tagging + filters hold).
Revisit only if cancellations persist at >10% during NON-benchmark windows.
