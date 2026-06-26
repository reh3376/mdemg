# INTENT-DISABLE-001 — Post

## Decision
**Disabled intent translation.** Data-decided from a UVTS A/B, not preference.

## The evidence

| Profile | Intent OFF | Intent ON | Δ | Verdict |
|---|---|---|---|---|
| Quick 16q | 0.3900 | 0.3960 | +0.006 | pass (within noise) |
| **Full 120q** | **0.4170** | **0.4070** | **−0.010** | **FAIL** |

The 16q quick suggested a tiny benefit; the robust 120q corpus flipped it — intent translation is **net-negative** on retrieval quality (4 per-question improvements but a negative mean; 0 regressions > 0.1). Lesson (re-affirmed): never make a costly architectural call on the 16q sample.

## Why disable, not tune
Net-negative quality **and** large cost:
- ~15% chronic timeout rate; **70% of all LLM errors** (123/7d) — the driver of the recurring `alert_llm_health` spike + HIGH consecutive-failure alerts.
- Synchronous on the retrieval hot path: avg 3.8s, up to 15s added latency before the fail-open fallback.

There is nothing to tune toward — a slower, slightly-worse retrieval is strictly dominated by intent-off.

## Changes
- `internal/api/handlers.go` — added `?intent=true|false` URL-param override (mirrors `?sparse=`/`?strict_context=`); enables the A/B and future re-verification without restarts.
- `internal/config/config.go` — `INTENT_TIMEOUT_MS` default `2000 → 15000` (2000ms < avg latency 4400ms guaranteed timeouts; honors the ≥15000ms rule). Floor stays 200 (intent is fail-open; operators MAY fast-fail).
- `.env` (local, gitignored) — `INTENT_ENABLED=false`.
- `docs/api/api-spec/uats/specs/memory_retrieve_sparse_context.uats.json` — `?intent=` true/false contract variants; hash re-pinned.

## Testing
- **Tier 1:** `go test ./internal/config/ ./internal/api/` green; `verify_config_consumers.py` 726/726; lint 0.
- **Tier 2:** UATS `validate` — 7/7 pass incl. both new intent variants; hashes verified.
- **Tier 3 (live):** the 120q A/B itself; after restart with intent off, **0 new `retrieval.intent_translate` interactions** (newest call 04:33:16 precedes the 04:33:52 server start). `?intent=true` against the disabled server correctly performs no translation (translator not built) yet still returns 200 — the override is a no-op when the feature is off, as designed.

## Notes / follow-ups
- The intent translator code is **retained** — re-enableable via `INTENT_ENABLED=true`. Before re-enabling after any model or substrate change, re-run the `?intent=true` UVTS A/B; only re-enable if it shows a real, cost-justifying lift.
- The shipped default (`INTENT_ENABLED=false`) is now **evidence-backed**, where before it was a guess.

## Documents Accessed
- `internal/api/handlers.go`, `internal/config/config.go`, `internal/retrieval/intent_translator.go`
- `docs/tests/uvts/runners/uvts_runner.py`, `uvts_ab_compare.py`, `lnl_demo_validation.uvts.json`
- `docs/api/api-spec/uats/specs/memory_retrieve_sparse_context.uats.json`
- Live `llm_interactions` TSDB; `ab_verdict_{quick_16q,full_120q}.json` (archived here)
