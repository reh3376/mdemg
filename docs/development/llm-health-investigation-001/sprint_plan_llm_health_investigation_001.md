# Sprint LLM-HEALTH-INVESTIGATION-001 — retrieval.rerank_cross error-rate spike

## 1. Header & Metadata
- **Sprint ID:** LLM-HEALTH-INVESTIGATION-001
- **Sprint line:** `docs/development/llm-health-investigation-001/`
- **Date opened:** 2026-07-20
- **Target version:** v0.11.5 (patch — recorder classification + rerank budget preflight; no schema migration)
- **Estimated effort:** ~0.5 dev-day, 6 sequential epics
- **OpenAI spend:** $0
- **Risk level:** Low — additive tagging + defensive skip; no removal of existing error signal (real errors still count)

## 2. Problem Statement
`rsic-alert_llm_health` has fired 10+ times in the past hour on `mdemg-dev`. Investigation (bash-only, pre-sprint) shows:

- **22.5% error rate on `retrieval.rerank_cross`** in the last 4h (9 errors / 40 calls).
- **Every other task at 0% error** (jiminy.evaluate_llm, ape.reflect, hidden.name_emergence, retrieval.query_classify, consulting.classify, jiminy.synthesize, jiminy.codegen).
- **100% of the rerank_cross errors are identical**: `http request: Post "http://127.0.0.1:8102/v1/chat/completions": context canceled`.
- Successful `rerank_cross` latency distribution: p50=5.9s, p95=11.1s, p99=11.7s (24h window).
- Retrieve budget is 20s (`RETRIEVE_TIMEOUT_MS`, from RETRIEVAL-TYPED-EDGES-002 Epic 2). When earlier stages (vector recall + BM25 + graph column + structural walk + activation spread + cache lookup) consume 10-14s, rerank starts with 6-10s remaining — insufficient for the p95=11s call → deterministic cancellation on the slow half.
- User impact: **none**. Rerank already fails-open — caller `slog.Warn`s and returns pre-rerank RRF-ordered results (`internal/retrieval/service.go`).
- Quality impact: **modest**. ~22% of retrieves return non-LLM-reranked results (RRF-only).
- Alert channel impact: **significant**. Legitimate-error signal that isn't actually an LLM health event.

**Root cause diagnosis:** the `llm_interactions.error` column records the caller-cancelled call as a generic error, and the RSIC alert rule (`llm_error_rate_spike`) counts it as such. The LLM (llama-server) is healthy; only the *caller* aborted the request. The alert is technically correct that the call failed, but semantically misleading — it's not an LLM health event, it's an insufficient-budget event.

## 3. Scope & Constraints

### In scope
- **E1 — recorder classifies caller-cancellation distinctly.** In the `llm_interactions` writer path, when the recorded error is `context.Canceled` or `context.DeadlineExceeded` AND the LLM client did not initiate the cancellation itself (i.e., the deadline came from the caller's context, not the client's `RerankTimeoutMs`-derived timeout), tag the row as `caller_canceled` in a dedicated column (or repurpose an existing column). The alert consumer excludes caller_canceled from the error rate.
- **E2 — rerank pre-check budget.** In `internal/retrieval/rerank.go::Rerank`, before dispatching to the provider, check `deadline, ok := ctx.Deadline(); remaining := deadline.Sub(now)` — if `remaining < RerankMinBudgetMs` (config-driven, default 12000 = p99+margin), return the pre-rerank order with `slog.Warn "rerank skipped: insufficient budget"`. Prevents the wasted llama-server slot AND the misleading `caller_canceled` row.
- **E3 — RSIC alert rule filter.** The rule that calculates `error_rate` for a task must exclude `caller_canceled` rows (either via a column filter or by excluding the specific error prefix). Verified live: alert must not fire when a burst of caller_canceled rows lands. Any Grafana panel showing the same signal gets the same filter.
- **E4 — Tier-1 + Tier-2 tests.** Recorder classify truth table (context.Canceled → caller_canceled; other error → real error; nil err → success). Rerank pre-check truth table (deadline >= min → proceed; deadline < min → skip). Alert-rule SQL — a caller_canceled row does not bump the error count.
- **E5 — live Tier-3.** Trigger a retrieve with a synthetically-tight ctx (< 12s remaining when rerank starts) → verify pre-check skip fires + no llm_interactions error row. Trigger a normal retrieve → verify rerank runs to completion. Wait for the next RSIC evaluation cycle → verify no new `alert_llm_health` fires. Historical caller_canceled rows retroactively don't contribute to the rate.
- **E6 — canonical docs.**

### Out of scope
- Raising `RETRIEVE_TIMEOUT_MS` from 20s → 30s. Deferrable as a config-tuning follow-up if the pre-check skip rate is high in production (would indicate genuine budget crunch, not just occasional slow-half rerank). Sprint's fixes make the observability honest; retuning is a data-decided follow-up.
- Rewriting the rerank pipeline to be async / streaming. Out of scope.
- Removing existing error-row backfill (the 9 historical caller_canceled rows in TSDB) — leave in place; the alert-rule filter handles them going forward.
- Neural sidecar rerank (`internal/retrieval/rerank.go::rerankWithNeural`) — the same budget concern applies; deferrable if the primary path (`openai` via llama-server) is the observed hotspot.

### Constraints
- Sequential epics.
- **Live Tier-3 required for every modified surface.**
- No new LLM calls added.
- Additive recorder change — old `error` column stays; new tag is a separate discriminator (either a column or a well-defined error-string prefix).
- Fail-open behavior preserved: pre-check skip returns valid results (RRF order), just without LLM rerank.
- RRF-SCALE-001-safe: no changes to any score gate.

## 4. Dependencies
- **RETRIEVAL-TYPED-EDGES-002** (merged) — introduced `RETRIEVE_TIMEOUT_MS=20000` which combined with rerank p95=11s produces the budget-crunch pattern.
- No new env vars beyond `RERANK_MIN_BUDGET_MS`.

## 5. Implementation Plan

### Epic 0 — Sprint plan committed
This document.

### Epic 1 — Recorder classifies caller-cancellation
- `internal/llmclient/recorder.go` (or wherever the interaction row is written): inspect `err`. If `errors.Is(err, context.Canceled)` or `errors.Is(err, context.DeadlineExceeded)`, set a new dedicated column `caller_canceled=true` OR write to `error` with a distinct prefix `caller_canceled: <original>`.
- Chosen approach TBD in E1 — column is cleaner, prefix is migration-free. Investigation-note documents the trade.
- Preserve the raw error string in another column for forensic use (or in the same column with the prefix).

### Epic 2 — Rerank budget pre-check
- `internal/retrieval/rerank.go::Rerank`: after `timeoutCtx, cancel := context.WithTimeout(ctx, RerankTimeoutMs*ms)`, ALSO check the incoming `ctx.Deadline()` — if the CALLER's deadline is closer than `now + RerankMinBudgetMs`, skip. Return `RerankResult{Results: req.Candidates, LatencyMs: 0}` and log `slog.Warn "rerank skipped: insufficient budget" remaining_ms=<n>`.
- New config: `RerankMinBudgetMs int` (env `RERANK_MIN_BUDGET_MS`, default 12000, floor 3000 — calibrated to observed p99 = 11.7s plus 300ms safety margin).
- No metric emission on skip (an operator-visible WARN is enough at this scale; can add a Prometheus counter as a follow-up if needed).

### Epic 3 — RSIC alert rule filter
- Locate the SQL that computes `alert_llm_health` (search `internal/ape/` for the rule + `internal/alert/` for the rule generator).
- Filter: `WHERE error <> '' AND NOT (error LIKE 'caller_canceled:%')` OR add explicit `AND caller_canceled = FALSE` if column path chosen in E1.
- Same filter applied to any Grafana panel using the same signal.

### Epic 4 — Tests
- Tier 1: recorder classify truth table (nil / context.Canceled / context.DeadlineExceeded / real error).
- Tier 1: rerank pre-check truth table (deadline in ctx / deadline absent / deadline >= min / deadline < min).
- Tier 2: alert-rule SQL against a fixture — a caller_canceled row doesn't bump the count.

### Epic 5 — Live Tier-3 on `mdemg-dev`
1. Set `RERANK_MIN_BUDGET_MS=12000` in `.env` (if the default needs override).
2. Rebuild + restart.
3. Trigger a `POST /v1/memory/retrieve` normally → verify rerank runs (no skip WARN).
4. Trigger a retrieve with a client-side tight timeout (e.g., `curl -m 5`) → verify server logs "rerank skipped: insufficient budget" + no `llm_interactions` error row.
5. Check TSDB: since the fresh binary boot, no new `retrieval.rerank_cross` errors OR they're classified as `caller_canceled`.
6. Wait 5 min (one RSIC eval cycle) → verify no new `alert_llm_health` fires.
7. `live_verification.md`.

### Epic 6 — Docs
CLAUDE.md architecture note; CHANGELOG `[Unreleased] > Fixed`; `docs/features/rsic-feedback-loop.md` alert-rule refinement note; `post.md`.

## 6. Testing (3 tiers)
- **Tier 1** — 8 unit pins across recorder + rerank pre-check + alert-rule SQL.
- **Tier 2** — recorder round-trip through a fixture DB (or a code-level assertion on the writer's row-shape).
- **Tier 3** — the live sequence in §Epic 5.

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`:
1. `docs(llm-health-investigation-001): E0 — sprint plan`
2. `feat(llm-health-investigation-001): E1 — recorder classifies caller-cancellation`
3. `feat(llm-health-investigation-001): E2 — rerank budget pre-check`
4. `feat(llm-health-investigation-001): E3 — RSIC alert rule excludes caller-canceled`
5. `test(llm-health-investigation-001): E4 — unit + integration tests`
6. `docs(llm-health-investigation-001): E5 — live Tier-3 verification`
7. `docs(llm-health-investigation-001): E6 — CLAUDE.md/CHANGELOG/feature/post`

## 8. Verification Checklist
- [ ] E0 committed
- [ ] Recorder classifies context.Canceled / DeadlineExceeded distinctly
- [ ] Rerank pre-check skips when remaining budget < RerankMinBudgetMs
- [ ] RSIC alert rule excludes caller_canceled from the error count
- [ ] Grafana panel (if any) filters same
- [ ] `go build ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] `golangci-lint run` clean
- [ ] Live: normal retrieve → rerank completes; error row absent
- [ ] Live: tight-budget retrieve → rerank skip WARN; NO error row (or caller_canceled row)
- [ ] Live: no new alert_llm_health fires post-restart in a 30-min window
- [ ] CLAUDE.md note
- [ ] CHANGELOG entry
- [ ] Feature doc section
- [ ] post.md

## 9. Documentation Update — Epic 6.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Pre-check skip misfires when ctx has no deadline (e.g., CLI direct call) | Low | Low | `deadline, ok := ctx.Deadline(); if !ok { proceed }` — no deadline means no caller cancellation to guard against |
| Recorder misclassifies a legitimate server error as caller_canceled | Very Low | Medium | `errors.Is` is strict on the sentinel types — a real HTTP-500 or provider timeout is neither Canceled nor DeadlineExceeded |
| Alert-rule filter hides a REAL cancellation spike caused by an upstream infra bug (e.g., HTTP proxy sends abort) | Low | Medium | Log every caller_canceled at INFO with the task_name; a spike is still visible in logs. Also expose a per-task caller_canceled COUNT panel as a follow-up if operator wants an early warning. |
| RerankMinBudgetMs default too high → over-skips in a fast environment | Low | Low | Calibrated to observed p99 (11.7s) + 300ms; operator can lower via env; live smoke measures skip rate |
| Historical error rows aren't retroactively reclassified | Certain | Low | Deliberate — the rule-side filter handles them going forward; alerts will decay as the rolling window ages out |

## 11. Documents Accessed
- `internal/retrieval/rerank.go` (call site + timeout + fail-open handling)
- `internal/retrieval/service.go` (upstream caller handling of rerank error)
- `internal/config/config.go::RerankTimeoutMs` (existing timeout knob)
- `internal/llmclient/*.go` (recorder path — location of the `llm_interactions` write)
- `internal/ape/` (RSIC alert rule for `llm_error_rate_spike`)
- Live TSDB: `llm_interactions` recent errors filtered by task_name
- Live TSDB: latency distribution across p50/p95/p99 on rerank_cross

## 12. Rollback Procedures
- **E1 recorder tagging**: additive — the raw error string is preserved either in the same column with a prefix or in a companion column. Rolling back the code makes future rows revert to the old format; historical caller_canceled rows retain their tag.
- **E2 pre-check**: pure code change; revert removes the pre-check and the config knob. RerankTimeoutMs behavior unchanged (rerank runs, may cancel, fails-open).
- **E3 alert rule**: revert restores the un-filtered COUNT; alerts will fire again on caller_canceled rows.
- **Config**: `RERANK_MIN_BUDGET_MS` default in-code; env override optional.

## Acceptance Criteria
1. `llm_interactions` recorder writes a distinct classification for caller-cancelled calls.
2. Rerank pre-check skips + WARN-logs when remaining ctx deadline < min budget.
3. RSIC `alert_llm_health` rule excludes caller_canceled from the error count (verified via fixture SQL + live no-alert-fire).
4. Fresh boot → 30 min elapsed with normal traffic → 0 new `alert_llm_health` fires (evidence in `live_verification.md`).
5. Full test suite green; lint clean.
6. Canonical docs updated.
