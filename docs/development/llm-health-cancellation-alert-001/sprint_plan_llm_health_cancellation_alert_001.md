# Sprint LLM-HEALTH-CANCELLATION-ALERT-001 — consecutive-failure alerter honors the caller-cancellation contract

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | LLM-HEALTH-CANCELLATION-ALERT-001 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~0.5 dev-day |
| Parent | LLM-HEALTH-INVESTIGATION-001 (the contract: "caller-cancellation is NOT an LLM health event") — this closes the last consumer that ignores it |

## 2. Problem Statement

LLM-HEALTH-INVESTIGATION-001 E1 tags caller cancellations (`context.Canceled`
/ `context.DeadlineExceeded`) in the recorder, and E3 filters them from the
RSIC `llm_error_rate_spike` rule. But `llmclient.trackResult` — the
per-task consecutive-failure counter behind the `LLM_CONSECUTIVE_FAILURE_THRESHOLD`
HIGH alert — still counts them. Evidence: 2026-07-21 20:34–20:49Z, the
doc-audit agent fleet's hook traffic saturated llama-server (831
`jiminy.evaluate_llm` + 293 `rerank_cross` at 34s avg in 90 min) → **8 HIGH
alerts across 5 tasks, all "context deadline exceeded", while filtered real
errors in the same window = 0**. Pure noise by the shipped contract; it also
buries real alerts (dispatcher cooldown consumed by noise).

## 3. Scope & Constraints

**In scope:** extract the E1 predicate into `isCallerCancellation(err)`;
use it in `recordInteraction` (no behavior change) and in `trackResult`,
where cancellations become **neutral** — do not increment (not an LLM health
event) and do not reset (no evidence the endpoint is healthy either). Unit
tests. Live Tier-3 with forced cancellations. Docs (CHANGELOG + CLAUDE.md
note amendment).
**Out of scope:** the watchdog (probe-based server-down detection stays the
authority for "endpoint down"); alert evaluator rules (already filtered);
retry logic (`shouldRetry` already handles ctx errors separately).
**Constraints:** real HTTP 5xx / parse / provider-timeout errors must still
trip at threshold exactly as before (pin-tested).

## 4. Dependencies

✅ E1 predicate inline at `client.go:360`; ✅ existing trackResult tests (if
any) to extend; ✅ live stack for Tier-3.

## 5. Implementation Plan (sequential)

- **E0** this plan.
- **E1** helper + trackResult neutral-skip + unit tests
  (cancellation ×N → no trip, counter unchanged; real ×threshold → trip;
  success → reset; interleaved cancellation does not break a real streak).
- **E2** live Tier-3: build + restart; force caller-cancellations
  (retrieve with a tiny budget so rerank is canceled ≥3×); assert **no new**
  `llm-retrieval.rerank_cross` alert lands in `~/.mdemg/alerts/current.json`
  / TSDB; then confirm a real-failure path still alerts (scratch client
  against an unreachable port, threshold 3).
- **E3** docs: CHANGELOG; CLAUDE.md LLM-HEALTH note amendment; sprint post.

## 6. Testing Plan

Tier 1: new unit tests in `internal/llmclient/`. Tier 2: `go test ./...`.
Tier 3: E2 live forced-cancellation + real-failure alert checks.

## 7. Commit Strategy

`docs(E0)` → `fix(E1)` → `docs(E2 evidence + E3)`.

## 8. Verification Checklist

unit green · `go test ./...` green · lint clean · live: ≥3 forced
cancellations → 0 new alerts · live: real failures still alert · docs ·
pushed.

## 9. Documentation Update

CHANGELOG Fixed; CLAUDE.md LLM-HEALTH-INVESTIGATION-001 note gains the
"consecutive-failure alerter also skips cancellations" clause; post.md.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Masking a real outage that only manifests as timeouts | Low | The watchdog probes `/v1/models` independently (state machine up/degraded/down + fast-fail); server-down was never this alert's job |
| Neutral (no-reset) starves the reset path under all-cancellation traffic | Low | Intended: a later real success still resets; a later real failure resumes the streak accurately |

## 11. Rollback

Revert the fix commit; behavior returns to counting cancellations.

## 12. Documents Accessed

`internal/llmclient/client.go` (trackResult, recordInteraction, shouldRetry);
LLM-HEALTH-INVESTIGATION-001 sprint docs; live alert evidence
2026-07-21 20:34–20:49Z; `llm_interactions` window query (0 real errors).
