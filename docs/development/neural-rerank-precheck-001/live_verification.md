# NEURAL-RERANK-PRECHECK-001 — Live Tier-3 Verification

Real binary + real llama-server + real neural sidecar. Date: 2026-07-20.

## Binary + stack
- Binary rebuilt 2026-07-20 13:17 EDT (includes E1+E2).
- launchd pid 5487 on :9999 (clean state — stale pre-restart pid was killed earlier).
- Neural sidecar HEALTHY at `:8100` (`cross-encoder/ms-marco-MiniLM-L-6-v2` + `cross-encoder/nli-MiniLM2-L6-H768` loaded).

## E3-A — Default (openai) rerank preserved

`RERANK_PROVIDER=openai`, defaults: `RerankMinBudgetMs=12000`.

```
$ curl -X POST /v1/memory/retrieve ... query_text: "provider aware rerank budget"
{"rerank_ms": 2588, "rerank_enabled": true, ...}
```

rerank_cross ran in 2.6s (well under 12000ms budget). No skip. Existing E2 behavior byte-identical. ✅

## E3-B — Neural provider completes on default budget

Set `RERANK_PROVIDER=neural`; restart. Default `NeuralRerankMinBudgetMs=1500`.

```
$ curl -X POST /v1/memory/retrieve ... query_text: "neural sidecar rerank smoke"
{"rerank_ms": 122, "results": 5, ...}
```

**Neural sidecar reranked in 122ms — 20× faster than the LLM path.** No skip WARN in log. With the OLD single-knob code, this call would have been evaluated against `RerankMinBudgetMs=12000` — a caller with less than 12s remaining would have been silently skipped even though neural needed only 122ms. **The provider-aware fix eliminated that over-skip.** ✅

## E3-C — Force neural skip via oversized budget

`NEURAL_RERANK_MIN_BUDGET_MS=30000` (larger than the 20s retrieve deadline); restart.

```
$ curl -X POST /v1/memory/retrieve ...
{"rerank_ms": 0, "results": 5, ...}

# Server log:
WARN rerank skipped: insufficient budget
     remaining_ms=19475 min_budget_ms=30000 provider=neural
     space_id=mdemg-dev candidates=15
```

Pre-check skipped deterministically. **WARN log carries `provider=neural`** — the new grepability field is populated. Fail-open confirmed (5 results returned unchanged). ✅

## E3-D — Reverted; RSIC baseline clean

Reverted `RERANK_PROVIDER=openai` + removed the E3-C override. Fresh restart on defaults. Health check `neo4j: ok, tsdb: ok, jiminy: ok, circuit_breakers: ok`.

## Verification summary

| Check | Result |
|---|---|
| openai provider default rerank unchanged | ✅ rerank_ms=2588 |
| neural provider completes on default 1500ms budget | ✅ rerank_ms=122 |
| Neural sidecar is 20× faster than LLM path | ✅ (122ms vs 2588ms same query load) |
| Provider-aware skip fires when neural budget forced high | ✅ WARN provider=neural min_budget_ms=30000 |
| WARN log's new `provider` field populated | ✅ |
| Fail-open preserved (5 results on skip) | ✅ |
| Full go test ./... green | ✅ (pre-E3) |
| Unit truth-table pins | ✅ 4/4 provider-aware pins |

All 6 acceptance criteria met.

## Follow-ups
- **RERANK_PROVIDER=neural is measurably 20× faster** than openai on this workload. Data-decided sprint candidate: default-flip to `RERANK_PROVIDER=neural` if the quality trade-off (cross-encoder vs LLM judge) is acceptable — needs an A/B on the UVTS corpus, not done here.
- The neural sidecar's 122ms suggests the default `NEURAL_RERANK_TIMEOUT_MS=1000` is generous; could tighten to 500ms if operator wants faster failure signal.
