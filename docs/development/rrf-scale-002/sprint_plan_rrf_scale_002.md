# Sprint Plan RRF-SCALE-002 + CACHE-KEY-002 — Finish the Score-Scale Contract

## 1. Header & Metadata
- **Sprint ID:** RRF-SCALE-002 (+CACHE-KEY-002) — Q3 Phase 2, ranked #7
- **Line:** `docs/development/rrf-scale-002/` · **Date:** 2026-06-11 · **Branch:** `reh3376_dev01`
- **Target:** v0.10.x · **Effort:** ~4.5d budgeted · **Spend:** $0 · **Risk:** Low-Medium

## 2. Problem Statement
RRF-SCALE-001 fixed three consumers hardcoding thresholds against the
retrieval score scale and wrote the standing instruction: *audit every
score comparison; gate via config with RRF-calibrated defaults.* Four
verified leftovers, plus the cache-key class recurring:
1. **`consulting.Suggest` floor** (`service.go:628`): `minConfidence`
   default 0.5 against a scale where strong RRF matches top out 0.49–0.58
   — `/v1/memory/suggest` filters nearly everything (the audit's
   "Suggest revival" item).
2. **MCP reflect tiers** (`cli/mcp.go:426`): `score >= 0.7` high / `>= 0.4`
   medium against raw RRF — the high tier is unreachable.
3. **Per-call `llmclient.New` in rerank** (`rerank.go` doRerankWithOpenAI/
   doRerankWithOllama): a fresh client per call resets the
   consecutive-failure counter, so `LLM_CONSECUTIVE_FAILURE_THRESHOLD` can
   NEVER fire for `retrieval.rerank_cross`/`rerank_nli` — failure alerting
   disarmed on the hottest LLM path (also: no HTTP connection reuse).
4. **Guardrail `sim > 0.3`** hardcoded in Cypher
   (`constraint_retrieval.go:150`) — cosine-stable today but inside the
   class; config-drive it.
5. **CACHE-KEY-002:** `CacheKey` omits result-affecting RetrieveRequest
   fields: `include_extensions`, `exclude_extensions`, `temporal_after`,
   `temporal_before`, `policy_context` (+ caller-supplied
   `query_embedding`, hashed) — two requests differing only in these
   collide on one cache entry (the v0.7.0 P1 cache-key class, second
   occurrence).

## 3. Scope & Constraints
**In:** config-driven thresholds with RRF-calibrated defaults
(`CONSULTING_SUGGEST_MIN_CONFIDENCE` 0.45-band; `MCP_REFLECT_SCORE_HIGH`/
`_MEDIUM` mapped to the RRF bands); persistent rerank clients (lazy,
per-provider, on Service); `GUARDRAIL_CONSTRAINT_SIM_FLOOR` (default 0.3);
CacheKey extension; **forcing functions:** (a) reflection test — every
`RetrieveRequest` field must be in CacheKey OR an explicit result-neutral
allowlist (new fields fail until classified); (b) score-literal scan test
— flags `.Score <op> 0.x` comparisons outside an allowlist of
config-driven sites.
**Out:** NormalizedConfidence redesign (positional-percentile caveat
stands); retrieval scorer changes; JIMINY display fixes (MCP-REVIVE).

## 4. Dependencies
RRF-SCALE-001 conventions (`CONSULTING_*_SCORE_FLOOR` pattern, sigmoid
config); llmclient breaker/counter machinery; query cache namespace
versioning.

## 5. Implementation Plan
Epic 0 plan (done — all five sites read). Epic 1 rerank clients. Epic 2
thresholds (suggest/MCP/guardrail). Epic 3 CacheKey + scorerVersion
review. Epic 4 forcing-function tests. Epic 5 Tier 3 live (suggest
returns results on a real query; MCP tiers populated; rerank breaker
counters persist across calls — verify via /v1/admin/breakers or
counters; cache distinguishes extension-filtered requests). Epic 6 docs.

## 6. Testing Plan
Tier 1: threshold defaults + env overrides; cache-key field tests; the two
forcing-function tests themselves. Tier 2: suites + lint + UATS untouched.
Tier 3: live smoke per Epic 5. UVTS: not required (no scorer change) —
cache-key change only SPLITS entries (no wrong-hit risk introduced).

## 7. Commit Strategy
One commit per epic; surprises standalone; push → auto-PR → summary.

## 8. Verification Checklist
- [ ] /v1/memory/suggest returns suggestions on a real query (was ~empty)
- [ ] MCP reflect populates high/medium tiers on real scores
- [ ] Rerank failure counters persist across calls (threshold reachable)
- [ ] Guardrail sim floor config-driven
- [ ] CacheKey distinguishes the 5(+1) added fields; reflection test green
- [ ] Score-literal scan green with allowlist; suites + lint clean

## 9. Documentation Update — final epic.

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Lower suggest floor admits noise | M | L | RRF-calibrated default per SCALE-001 precedent (0.45); env-tunable |
| Cache-key change cold-starts the cache | High | Low | One-time; namespace versioning already handles scorer flips |
| Score-literal scan false positives | M | L | Explicit allowlist with comments; test failure message names the file:line |

## 11. Documents Accessed
internal/consulting/service.go (Suggest floor + SCALE-001 sigmoid block);
internal/cli/mcp.go:426; internal/retrieval/{rerank.go,cache.go};
internal/guardrail/constraint_retrieval.go:150; models.RetrieveRequest;
roadmap; CLAUDE.md score-scale contract note.

## 12. Rollback
Revert commits; cache namespace self-heals; thresholds return to the
broken hardcodes (prior state).
