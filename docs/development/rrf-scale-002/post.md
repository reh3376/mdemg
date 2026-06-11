# RRF-SCALE-002 + CACHE-KEY-002 — Sprint Close

**Date:** 2026-06-11 · **Roadmap:** Q3 Phase 2, #7

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Plan (5 sites read at line level) | `c76d587` |
| 1 | Persistent rerank clients — failure alerting re-armed on the hottest LLM path | `4f...` (Epic 1 commit) |
| 2 | Config-driven thresholds: Suggest floor, MCP reflect tiers, guardrail sim floor | `15b170e` |
| 3–4 | CacheKey covers ALL result-affecting fields + two forcing functions | `b8180eb` |
| 5–6 | Live recalibration + close | (this) |

## Live verification highlights
- **Suggest revived with live-data calibration:** the audit's 0.45 default
  STILL over-filtered (real query distribution: good hits 0.416/0.214/0.205,
  noise tail 0.004 — the 0.49–0.58 "strong band" is for exceptional matches;
  Suggest is a recall surface). Default set to **0.2 from observed data**:
  2/12 results pass (top: the AlertRule struct — on-point), noise rejected.
- **The reflection forcing-function caught 8 MORE missing cache-key fields
  on its first run** beyond the audit's 5: sparse-gate overrides
  (`?sparse=` params), pagination, context-fingerprint params. All keyed.
- **The score-literal scan triaged 3 scale-local sites** on its first run
  (allowlisted with justifications; jiminy/corrections.go noted as a
  config candidate mirroring the guardrail floor).
- Rerank counter persistence: structural (shared *atomic via WithContext);
  Tier 1 pins the once-per-provider construction.

## Follow-ups
- MCP reflect tiers verified by code path (config plumbed); live MCP
  session check rides with MCP-REVIVE-001 (Phase 3).
- jiminy/corrections.go cosine floor → config (small, with MCP-REVIVE or
  the hygiene batch).

**Documents Accessed:** consulting/service.go, cli/mcp.go,
retrieval/{rerank,cache,service}.go, guardrail/{guardrail,
constraint_retrieval}.go, api/server.go, models.RetrieveRequest, live
score distributions (mdemg-dev), roadmap.
