# LLM-HEALTH-CANCELLATION-ALERT-001 — Sprint Post

**Date:** 2026-07-21 | **Branch:** `reh3376_dev01`
**Parent:** LLM-HEALTH-INVESTIGATION-001 — closes the last consumer ignoring
the "caller-cancellation is NOT an LLM health event" contract.

## Trigger (live evidence)

2026-07-21 20:34–20:49Z: the DOC-CURRENCY-002 agent fleet's hook traffic
saturated llama-server (90-min window: 831 `jiminy.evaluate_llm` calls,
293 `rerank_cross` at 34s avg). Result: **8 HIGH consecutive-failure alerts
across 5 tasks — every one "context deadline exceeded" — while the
caller-canceled-filtered real-error count in the same window was 0.** Pure
noise by the shipped contract, and it consumes the dispatcher cooldown that
real alerts need.

## Fix

`internal/llmclient/client.go`:

- Extracted `isCallerCancellation(err)` (errors.Is Canceled/DeadlineExceeded)
  — now shared by the E1 recorder tagging and the failure tracker.
- `trackResult`: cancellations are **NEUTRAL** — no increment (not a health
  event), no reset (no evidence of health either). Real errors trip exactly
  as before; the watchdog remains the server-down authority.

Tests: `TestConsecutiveFailure_CallerCancellationNeutral` (6 wrapped
cancellations → 0 alerts, counter 0) and
`TestConsecutiveFailure_CancellationPreservesRealStreak` (real, cancel,
real, cancel, real → fires exactly once at count 3). Existing threshold /
reset / trip-guard / retrip pins unchanged-green.

## Live Tier-3 (mdemg-dev)

1. Built + kickstarted the new binary under launchd.
2. Forced in-flight rerank cancellations: `launchctl setenv
   RETRIEVE_TIMEOUT_MS 4000` + `RERANK_MIN_BUDGET_MS 0` (0 = documented
   pre-check disable) + kickstart. (First attempt with 9000/3000 didn't
   cancel — the quiet system's rerank finishes in ~2.5s; the pre-check is
   also load-shedding correctly. Disabling the pre-check was required to
   force the in-flight path.)
3. **5 consecutive `LLM rerank failed … context deadline exceeded` WARNs →
   0 `alert: dispatching` lines, 0 new entries in
   `~/.mdemg/alerts/current.json`.** Pre-fix behavior fires on the 3rd.
4. Recorder unchanged: TSDB shows 6 `caller_canceled:` rows, 0 real errors
   for the window (E1 tagging intact through the shared predicate).
5. State restored: both env vars unset, kickstart, healthz ok, process env
   verified clean (defaults back: RETRIEVE_TIMEOUT_MS 20000, pre-check
   12000/1500 per provider).

## Verification checklist

- [x] Unit tests green (`go test ./internal/llmclient/...`)
- [x] `golangci-lint` 0 issues
- [x] Live: ≥3 consecutive forced cancellations → 0 alerts
- [x] Live: recorder rows still tagged `caller_canceled:`
- [x] State restored after destructive-ish env forcing
- [x] CHANGELOG + CLAUDE.md note amended

## Documents Accessed

`internal/llmclient/client.go` (trackResult/recordInteraction/shouldRetry),
`internal/llmclient/client_test.go` (existing consecutive-failure pins),
LLM-HEALTH-INVESTIGATION-001 sprint docs, live `llm_interactions` window
queries, `~/.mdemg/logs/server.log`, `~/.mdemg/alerts/current.json`.
