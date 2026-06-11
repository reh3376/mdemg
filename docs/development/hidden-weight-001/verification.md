# HIDDEN-WEIGHT-001 — Verification (Tiers 1–3)

**Date:** 2026-06-11 · **Stack:** native `mdemg serve` (restarted on the
fixed binary mid-sprint — see the live catch below) + Docker Neo4j/TSDB.

## The bug, proven live before fixing

- `point.distance(emb_a, emb_b)` → **NULL** on a real embedding pair where
  `vector.similarity.cosine(emb_a, emb_b)` → 0.627.
- Cosine semantics verified: identical → 1.0, orthogonal → 0.5,
  opposite → 0.0 (Neo4j returns [0,1] — drop-in for the weight range).
- Scale at sprint open: GENERALIZES 28,332/28,332 NULL (100%);
  ABSTRACTS_TO 36,110/37,996 (95%) — and growing live via consolidation.

## Epic 1 — creation sites (Tier 1 + 2)

All 3 edited statements EXPLAIN-validated against live Neo4j. Pair-builder
unit tests (CUIDv2 uniqueness/format, empty input). hidden suite green.

## Epic 2 — backfill (Tier 3, small-batch-first)

1. Dry-run count: 36,416 (mdemg-dev).
2. **LIMIT-5 live trial** → hand-verified: stored ≡ independently
   recomputed cosine to 6 decimal places.
3. Distribution preview over 2,000 (read-only): min 0.704, p25 0.923,
   mean 0.96; ~50% near-1.0 = single-member-cluster degeneracy (centroid ≡
   member embedding) — faithfully encoded; cluster identity is
   HIDDEN-CHURN-001 (Phase 2) scope.
4. Full run — **live catch:** the NULL count GREW mid-run
   (36,416 → 41,623): the running server predated Epic 1 and kept minting
   NULL edges. Restarted on the fixed binary, swept 5,233 stragglers, then
   whk-wms (8,755) + linear (199).
5. **Final: 0 NULL / 57,395 abstraction edges, all spaces.**

## Epic 3 — observability (Tier 3)

- Evaluator `rules=16 → 17` across the restart (`null_weight_abstraction_edges` loaded).
- Gauge `mdemg_neo4j_graph_null_weight_edges` persisting to
  `metric_samples` at **value 0 per space**.

## Epic 4 — end-to-end through the real pipeline (Tier 3)

`POST /v1/conversation/consolidate` (real run, 22 themes created) → newly
minted GENERALIZES edges carry **varied real cosine weights**
(0.83–0.94 — member-to-centroid similarity, not NULL, not uniform) and
**CUIDv2 edge ids** (e.g. `mxr6m2mw2x3dya1kujh8n49w` — no UUID hyphens).
Gauge re-checked post-consolidation: still 0.

## UVTS regression guard — and a harness discovery

The quick profile failed with mean **0.002** — investigated, NOT a
regression: the spec's corpus space **`lnl-demo-whk` no longer exists in
the graph** (zero nodes; zero trace in 2.5 months of `metric_samples`
history). The last persisted UVTS runs are 2026-05-03/04 (means
0.39–0.41) — **no UVTS run since then can have measured anything real.**
Control probes: retrieval against `mdemg-dev` healthy (top 0.456, normal
RRF band); the surviving `whk-wms` space (a different/partial ingest of
the same codebase) grades 0 on all 16 questions. The May baseline SHA
(`508ff5e`) no longer exists in the whk-wms repo history.

**Restoration (operator-directed):** full reingest of
`/Users/reh3376/whk-wms` @ current HEAD (`c1e4263e`) into `lnl-demo-whk`
(8,974 files, 0 errors, 35,146 symbols, 28m). The consolidation pass was
the fixed creation sites' first at-scale run: **9,500 abstraction edges,
0 NULL, weights min 0.742 / mean 0.923 / max 1.0.**

**Baseline attempt — blocked by additional harness rot (full audit →
UXTS-CI-001).** Retrieval itself is mechanically healthy on the restored
corpus: for the probe question, the exact expected files rank 1–4.
But a fresh baseline could not be honestly recorded; layered findings:
1. **Grading/persist breakage:** the fresh run retrieved 5/5 results for
   all 16 questions, then rendered its report from a stale grades file;
   `--persist-tsdb` reported "unavailable"; no new grades were written.
2. **Expected-path format drift:** the spec's `expected_files` are
   absolute (`/Users/reh3376/whk-wms/…`); today's ingest stores
   root-relative paths (`/apps/…`) — the May corpus evidently stored
   absolute. Evidence matching is sensitive to this.
3. **Vector post-filter dilution:** the embedding column queries the
   global vector index then space-filters
   (`internal/retrieval/service.go:1137-1139`); with 95k indexed nodes —
   including a duplicate copy of this codebase in the `whk-wms` space —
   in-space candidates rank low globally → RRF scores ~0.006 for
   correctly-ranked files (healthy band 0.45–0.58). Real multi-space
   retrieval issue (candidate: VECTOR-PREFILTER); the duplicate `whk-wms`
   space should also be retired or the spec repointed.
4. (Resolved tonight) corpus deletion + the ingest consolidation timeout.

Conclusion: **no UVTS baseline number is claimable until UXTS-CI-001
repairs the harness** — and that sprint now starts with a complete,
live-verified defect inventory instead of a blank page. Sprint-scope
verification (weights) is unaffected: retrieval ranking on the restored
corpus is correct, and the weight deliverables are verified independently
above.

## Acceptance criteria — met

1. ✅ 3 sites on `vector.similarity.cosine`, null-guards intact (site 1
   gained one), EXPLAIN-validated.
2. ✅ CUIDv2 edge ids on new edges (verified through a real consolidation).
3. ✅ LIMIT-5 verified before full backfill; 0 NULL globally after.
4. ✅ Weight distribution sane and explained (degeneracy mass documented
   for HIDDEN-CHURN-001).
5. ✅ Gauge + rule live; steady state 0; regression self-reports.
