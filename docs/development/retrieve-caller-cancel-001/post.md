# RETRIEVE-CALLER-CANCEL-001 — Sprint Post

**Date:** 2026-08-04 | **Branch:** `reh3376_dev01`
**Trigger:** SLO alert `high_error_rate` (MEDIUM) fired 2026-08-03T15:26:32Z at 1.50% error rate. Triage found the 5xx driver was `/v1/memory/retrieve` returning HTTP 500 on `context.Canceled` — client-cancellations misclassified as server errors. 24h pattern: 20 of 289 windows above the 0.1% threshold, all driven by curl walking away from a 6-8s retrieve. Same class as LLM-HEALTH-INVESTIGATION-001 (LLM-recorder `caller_canceled:` tag), applied at the HTTP handler layer.

## Verdict

Shipped. Every handler routing through `writeInternalError` now distinguishes caller-cancellation (HTTP 499, INFO log, "request cancelled") from real server error (HTTP 500, ERROR log, "internal error"). Since the SLO alert's `^5` regex excludes 499, caller-cancels no longer inflate `mdemg_http_requests_total` 5xx counts. The chronic false-positive channel is closed.

## What shipped

`internal/api/server.go`:
- New helper `isCallerCancelled(err) bool` — true iff `errors.Is(err, context.Canceled)` AND NOT `errors.Is(err, context.DeadlineExceeded)`. Wraps the ergonomic distinction between "client walked away" and "our timeout expired."
- New const `httpStatusClientClosedRequest = 499` (nginx "Client Closed Request").
- `sanitizeError` — logs INFO (not ERROR) on caller-cancel; returns "request cancelled during X" instead of "internal error during X."
- `writeInternalError` — returns HTTP 499 on caller-cancel, HTTP 500 otherwise.

All 28 sites that call `writeInternalError` (`retrieve`, `consult`, `suggest`, `ingest observation`, `backward pass`, `archive node`, etc.) inherit the fix — no per-site edits.

## Tests

`internal/api/caller_cancel_test.go` — 8 pin tests, all pass:
- `TestIsCallerCancelled_ContextCanceled`
- `TestIsCallerCancelled_DeadlineExceededIsNot` (server-side timeout — MUST be treated as real error)
- `TestIsCallerCancelled_WrappedCanceled` (unwraps `fmt.Errorf("...: %w", context.Canceled)`)
- `TestIsCallerCancelled_RealErrorIsNot`
- `TestIsCallerCancelled_NilIsNot`
- `TestWriteInternalError_CallerCancelReturns499` (also asserts 499 is outside 5xx range — the SLO-alert regex contract)
- `TestWriteInternalError_DeadlineExceededReturns500` (server-side deadline SHOULD alert)
- `TestWriteInternalError_RealErrorReturns500`

Package `go test ./internal/api/...` clean; lint 0 issues.

## Live Tier-3 (mdemg-dev)

Reproduced the failure class: `curl --max-time 0.1 -X POST http://localhost:9999/v1/memory/retrieve ...` × 4 → 4 log lines with `status=499 duration_ms≈100 level=WARN` (was `status=500 duration_ms=6-8s level=ERROR msg="operation failed"` pre-fix, per baseline lines in `/Users/reh3376/.mdemg/logs/server.log` from 2026-08-03). One successful `status=200` call confirmed the happy path unchanged.

Baseline vs post-fix log-line comparison:
```
# PRE-FIX (2026-08-03)
level=ERROR msg="http request" method=POST path=/v1/memory/retrieve status=500 duration_ms=6523 ...
level=ERROR msg="operation failed" operation=retrieve error="context canceled"

# POST-FIX (2026-08-04)
level=WARN  msg="http request" method=POST path=/v1/memory/retrieve status=499 duration_ms=101 ...
level=INFO  msg="operation cancelled by caller" operation=retrieve error="context canceled"
```

Alert-rule SQL over the current 5-min window: `err_5xx=0, err_pct=0.0000` (well below 0.1% threshold). No fire.

## Rules pinned

⚠️ **HTTP handlers MUST distinguish client-cancellation from server error** — same principle as the llmclient `caller_canceled:` recorder tag (LLM-HEALTH-INVESTIGATION-001), applied at the HTTP layer. Any handler that returns 500 on `context.Canceled` from the request context will fire the SLO alert chronically on any impatient client (curl `--max-time`, Ctrl-C, browser tab close, upstream proxy timeout). The distinguishing signal is `errors.Is(err, context.Canceled)` AND NOT `errors.Is(err, context.DeadlineExceeded)` — the second condition preserves alerting on the server's OWN timeouts.

⚠️ **`mdemg_http_requests_total`'s `status` label is the SLO-alert regex contract** — `^5` means 4xx/499 are the "server did its job, client didn't want the answer" classes. Any new HTTP status class that represents a real server error MUST land in the 5xx range; anything representing caller-side behavior MUST land elsewhere (4xx, 499). Adding a new status through `writeJSON` requires this discrimination check.

⚠️ **Centralize the classification in the helper, not per-handler** — `writeInternalError` is called 28 times across `handlers.go`; a per-site edit would be a maintenance trap (a new handler added tomorrow would drop back to 500-on-cancel). Fixing the ONE helper covers every current + future site.

## Not shipped (intentional)

- **Metric-label enrichment** (an `error_class` label on `mdemg_http_requests_total`): unnecessary — the status code IS the classification, and downstream consumers already read it. Adding a label expands cardinality without new signal.
- **Extension to non-`writeInternalError` sites**: a few handlers write raw `http.StatusInternalServerError` (grep found ~4 in `handlers.go`). Reviewed each; they're either LLM-path (already handled by llmclient's `caller_canceled:` tag) or explicit fault paths (auth, config validation). Not caller-cancel-sensitive.

## Follow-ups disclosed

- Passive 24h re-check of `high_error_rate` fire count. Pre-fix: 20 windows/day. Post-fix expectation: near-zero (real 5xx from other paths remain). Verify T+1d.
- Consider whether `mdemg_http_requests_total` should gain a per-status histogram of duration to distinguish `duration<1s cancel` (browser tab close) from `duration≈timeout cancel` (real client hang) — measurement only, no code change today.

## Rollback

Single-commit revert. Pre-sprint behavior (500-on-cancel) is byte-identical restoration.

## Documents Accessed

- `internal/api/handlers.go:436` (handleRetrieve error path)
- `internal/api/server.go:3115` (writeJSON) + `:3125` (sanitizeError + writeInternalError, edited)
- `internal/alert/rules.go:186` (high_error_rate rule; SLO alert contract)
- `~/.mdemg/logs/server.log` (baseline error-line evidence)
- `docs/development/llm-health-investigation-001/` (pattern parent)
