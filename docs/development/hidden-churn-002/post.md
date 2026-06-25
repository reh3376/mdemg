# HIDDEN-CHURN-002 — Sprint Post

## Summary

Fixed the hidden-pattern (L1) destroy-and-recreate churn that fired the CRITICAL `graph_node_drop` alert, and — surfaced during live Tier-3 — the deeper latent defect that the consolidation pipeline **never completed** on production-scale spaces. Net result: consolidation completes for the first time, hidden-pattern identity churn dropped **100% → ~25%/cycle**, node ids are CUIDv2, and the total-node gauge oscillation shrank from ±2,676 to ±780.

## What shipped

- **Match-in-place, not wipe-and-recreate** (`CreateHiddenNodes`): clusters match existing patterns and update them in place (node_id survives); only patterns matched by no current cluster are deleted; new patterns get a **CUIDv2** id (was `randomUUID()`). Mirrors HIDDEN-CHURN-001's theme fix. `detachBaseNodeHiddenEdges` (the wipe-all sweep) removed.
- **Completion fix (live-smoke finding):** `handleConsolidate` runs the whole pipeline under `context.WithoutCancel(r.Context())` + a server-side deadline (`CONSOLIDATE_TIMEOUT_MS`, default 30 min), so a client timeout can no longer abort the cycle mid-write. This is what lets the cycle reach `deleteUnmatched`.
- **Member-overlap (Jaccard) matching, centroid-cosine fallback** (`hidden_identity.go`): the primary identity signal is the cluster's L0 member set, which is more stable under KMeans jitter than the centroid. `HIDDEN_PATTERN_MEMBER_JACCARD_THRESHOLD` (0.5) + `HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD` (0.90).
- **Deterministic L0 ordering:** `fetchAllBaseNodes` now `ORDER BY b.node_id`, making KMeans (order-dependent init) reproducible across cycles.

## Live Tier-3 evidence (mdemg-dev)

| | Before | After |
|---|---|---|
| Consolidate completes | never (236× 500 / `context canceled`) | always |
| `deleteUnmatched` runs | never | yes (removed 702 → 1,090 → 1,160 stale across cycles) |
| Id churn / cycle | 100% | ~25% (survival 3093/4144 = 74.6%) |
| New ids | `randomUUID()` | CUIDv2 |
| Total-node gauge swing | ±2,676 | ±780 |

The first-ever completing cycle logged `created=1614 updated=2747` + `removed stale hidden patterns count=702`.

## Investigation arc (why the scope grew)

1. Diagnosed the CRITICAL alert as a true-positive destroy-recreate churn (hidden L1 layer, the bug class HIDDEN-CHURN-001 fixed for themes but never for `hidden`).
2. Built match-in-place + CUIDv2 (Epics 1–3). Live Tier-3 then showed the count **growing** (1,240→3,449), not stabilizing.
3. Traced it to `context canceled` on every consolidate (236× 500): the hidden step over 52k L0 nodes takes >5 min but ran under the caller's cancellable context → aborted before `deleteUnmatched`. Operator approved expanding scope to add the completion fix.
4. With completion fixed, churn dropped to ~28% (centroid-only). Operator approved adding member-overlap matching.
5. Member-overlap + determinism only reached ~75% survival — the residual is structural.

## Residual — UNRESOLVED DEFECT, must be fully remedied (HIDDEN-CHURN-003)

⚠️ **HIDDEN-CHURN-002 is a PARTIAL fix, not a closed defect.** It reduced hidden-pattern identity churn from 100% → ~25% per cycle, but **~25% residual churn remains and is an open defect that MUST be fully remedied — it is not an optional enhancement and not indefinitely deferrable.** Every cycle it still orphans ~25% of reinforcement / abstraction edges (the `CO_ACTIVATED_WITH` / `reinforcement_events` / HIDDEN-WEIGHT-001 weight references that pointed at the churned patterns) and re-runs the backward-pass weight computation from scratch. The cognitive substrate is not stable until this is closed.

The residual is **structural**: (a) non-deterministic **LLM category reclassification** (`RECLASS_ENABLED`) re-splits oversized categories differently each cycle, and (b) full re-clustering of a growing L0 set reshuffles membership — no centroid/Jaccard match threshold can recover it. **The remedy (HIDDEN-CHURN-003, committed) is incremental clustering** — assign only new/changed L0 nodes to existing patterns, leave stable patterns untouched — and/or deterministic/cached reclassification, targeting ~95%+ identity stability.

**This is logged so the sprint line is NOT mistaken for "done." Operator decided (2026-06-24) to ship the partial fix now and remedy the residual in HIDDEN-CHURN-003; that decision does not downgrade the residual from must-fix to nice-to-have.**

## Testing

- **Tier 1 unit** (`internal/hidden/hidden_identity_test.go`): member-overlap primary, centroid fallback, no-match/claimed, Jaccard + identity thresholds, CUIDv2-shape guard, matchTheme reuse. (10 tests)
- **Tier 2 integration** (`tests/integration/hidden_test.go::TestHiddenPatternIdentityStable`): double-run id survival + CUIDv2 shape, skip-safe.
- **Tier 3 live** (this doc): real binary against mdemg-dev, two consecutive completing cycles, survival measured from Neo4j.

## Documents Accessed

- `internal/hidden/service.go`, `internal/hidden/hidden_identity.go`, `internal/hidden/theme_identity.go`, `internal/hidden/member_edges.go`, `internal/hidden/clustering.go`, `internal/hidden/reclassifier.go`
- `internal/api/handlers.go` (`handleConsolidate`), `internal/config/config.go`
- `internal/alert/rules.go` (`graph_node_drop`)
- Live TSDB `metric_samples`, Neo4j (`HiddenPattern` counts/ids), `~/.mdemg/logs/server.log`
