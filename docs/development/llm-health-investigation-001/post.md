# LLM-HEALTH-INVESTIGATION-001 — Sprint Post (2026-07-20)

## Summary
Silenced the recurring `rsic-alert_llm_health` spike (6-12 fires/hr) by making the LLM error-rate signal honest. Investigation showed 100% of `retrieval.rerank_cross` errors were `context canceled` on the llama-server POST — a caller-cancellation pattern, not an LLM health event. Rerank fails-open (caller returns pre-rerank RRF results), so user impact was zero; the alert channel was signaling wrong. Sprint adds recorder-side classification, a pre-check that prevents wasted LLM slots, and an alert-rule filter — the noisy signal goes quiet without hiding real LLM health issues.

## What shipped
- **E0** — sprint plan (v1.0 12-section format).
- **E1** — `llmclient/client.go` on `callErr != nil`: `errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)` → prefix `caller_canceled: <raw>` in the recorded Error string. Real HTTP-500s / provider timeouts / parse errors are neither Canceled nor DeadlineExceeded so land unchanged. Raw text preserved after the prefix for forensic use.
- **E2** — `retrieval/rerank.go::Rerank`: read the CALLER's `ctx.Deadline()` before dispatching; if remaining < `RerankMinBudgetMs` (default 12000, floor 3000, 0=disabled), return the pre-rerank candidates unchanged + `slog.Warn "rerank skipped: insufficient budget"`. Deadline-absent callers (CLI, tests) bypass the check. Prevents both the wasted llama-server slot AND the misleading `caller_canceled` row.
- **E3** — `tsdb/dataset_builder.go::LLMPerformance` + `exporter.go::computeLLMQuality`: error_count FILTER now excludes `error LIKE 'caller_canceled:%'`. Same filter on `MAX(time) FILTER (...)` for the SUPERVISOR-002 recency gate. Feeds the RSIC `llm_error_rate_spike` rule (`self_reflect.go`) via `report.LLMPerformance.ErrorRate` — no changes needed there since it consumes the filtered value.
- **E4** — 3 recorder classify pins (caller-cancel tagged, real 500-error NOT tagged, success empty-error) + 4 rerank pre-check pins (skip on insufficient budget, proceed on sufficient, bypass on no-deadline, disable on 0-budget). Full go test ./... green; golangci-lint clean.
- **E5** — live Tier-3 on `mdemg-dev`: fresh binary's first observed rerank cancellation (14:11:59) landed with `caller_canceled:` prefix; alert-rule SQL returned `real_errors=0` (1 all_errors correctly filtered); pre-check skip fired deterministically under forced override (RerankMinBudgetMs=60000); 30-min window on default budget → 0 new `alert_llm_health` fires.
- **E6** — canonical docs: CLAUDE.md architecture note (with two pins: "caller-cancellation is NOT an LLM health event" + "rerank-shaped LLM calls need a budget pre-check"); CHANGELOG `[Unreleased] > Fixed`; this post. README.md refreshed with a 2026 sprint-line "Recently Completed" section covering the guidance-quality arc + substrate-honesty sprints (per operator request).

## Commits (on `reh3376_dev01`)
1. `docs(llm-health-investigation-001): E0 — sprint plan` — `85c4fdb`
2. `feat(llm-health-investigation-001): E1 — recorder tags caller-cancellation` — `c26b33d`
3. `feat(llm-health-investigation-001): E2 — rerank budget pre-check` — `8161a96`
4. `feat(llm-health-investigation-001): E3 — RSIC alert rule excludes caller-canceled` — `6241e50`
5. `test(llm-health-investigation-001): E4 — unit + integration tests` — `4da55c4`
6. `docs(llm-health-investigation-001): E5 — live Tier-3 verification` — `ce6641c`
7. `docs(llm-health-investigation-001): E6 — CLAUDE.md/CHANGELOG/README/post`

## Live evidence highlights
| Signal | Pre-sprint | Post-sprint |
|---|---|---|
| `rsic-alert_llm_health` fire rate | 6-12/hr | 0 in 30-min post-restart window |
| retrieval.rerank_cross error rows | untagged; all counted as errors | tagged `caller_canceled:` when caller cancels; real errors unchanged |
| LLM error rate as seen by RSIC | inflated by cancellations | honest — only real errors count |
| Pre-check skip behavior | none — rerank started even when budget was already exhausted | skips + WARNs + fail-opens when remaining ctx < min budget |
| User-visible retrieval quality | rerank fails-open, RRF results served | unchanged (same fail-open, just earlier) |

## Lessons captured
1. **Caller-cancellation is NOT an LLM health event.** A dead upstream deadline is a *client*-side condition; the LLM was healthy and about to serve. Recording the cancellation as a generic error and letting alerts fire on it created weeks of noise. The fix is at the recorder: tag it distinctly at write time so every downstream consumer (alert rule, dashboard, export manifest) can filter correctly.
2. **Rerank-shaped LLM calls (long p95, upstream deadline) need a budget pre-check.** If the call's own p99 is close to the caller's remaining budget, cancellation is deterministic on the slow half. A pre-check that fails-open with WARN is cheaper than a wasted LLM slot and a misleading error row. This pattern generalizes to ANY LLM call whose p99 approaches the caller's timeout.
3. **When adding an LLM call site whose error signal feeds alerting, filter `NOT LIKE 'caller_canceled:%'`** — OR read via `LLMPerformance.ErrorRate` which does the filtering for you.
4. **Investigate before opening a sprint.** ~10 min of bash-only investigation (TSDB queries + latency percentiles + grep for call site) surfaced the exact defect before writing a single line of code. The pre-sprint findings framed E1-E3 sharply.
5. **Live-testing default flips requires unmasking the override.** Verified in E5-C: temporarily setting RerankMinBudgetMs=60000 forced deterministic pre-check skips; reverted post-verification. Same pattern as the JIMINY-CONTRADICTED-BRIDGE-001 E6 flag-flip validation (`.env` line removed, restart, check log).

## Non-goals (respected)
- Raising `RETRIEVE_TIMEOUT_MS` 20 → 30s (data-decided follow-up; only if skip-rate becomes non-negligible in production).
- Grafana panel filter (dashboard panels not modified this sprint; a follow-up can apply the same filter — the data-side truth is honest via the export/RSIC path).
- Neural sidecar rerank budget preflight (same pattern applies; deferred to a follow-up if the primary path proves insufficient).

## Follow-ups
- **Skip-rate gauge**: `mdemg_rerank_skip_count{reason='insufficient_budget'}` Prometheus counter. Would give an early-warning signal for budget pressure without waiting for a downstream regression.
- **Grafana panel filter refresh**: any panel reading `llm_interactions.error` rate directly (not via `LLMPerformance`) should apply the same `NOT LIKE 'caller_canceled:%'` filter.
- **Neural sidecar rerank pre-check**: mirror the pattern in `rerankWithNeural` for parity.
- **`RETRIEVE_TIMEOUT_MS` calibration**: watch skip-rate over the next week; if >5% of retrieves skip rerank, raising RETRIEVE_TIMEOUT_MS is the honest fix (buys back real quality that the pre-check gives up).

## Acceptance criteria — all met
- [x] Recorder tags context.Canceled / DeadlineExceeded distinctly.
- [x] Rerank pre-check skips + WARN-logs when remaining ctx deadline < min budget.
- [x] RSIC alert rule (via `LLMPerformance` + `computeLLMQuality`) excludes caller_canceled from the error count.
- [x] Fresh boot → 30 min elapsed with normal traffic → 0 new `alert_llm_health` fires.
- [x] Full test suite green; lint clean.
- [x] Canonical docs updated (CLAUDE.md, CHANGELOG, README, post).
