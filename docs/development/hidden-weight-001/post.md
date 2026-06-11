# HIDDEN-WEIGHT-001 — Sprint Close

**Date:** 2026-06-11 · **Branch:** `reh3376_dev01` · **Roadmap:** Q3 Phase 1, rank #3

## What shipped

The abstraction hierarchy has real weights for the first time in the
project's history. `point.distance()` (spatial-Point function) silently
returned NULL on embedding lists at all three creation sites — 100% of
GENERALIZES and 95% of ABSTRACTS_TO edges were weightless, and everything
that consumes them (decay, prune, backward pass, RRF graph column) computed
over missing data.

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Plan + live proof (NULL vs 0.627 on the same pair; [0,1] cosine semantics) | `9627404` |
| 1 | 3 sites → `vector.similarity.cosine` + CUIDv2 edge ids; EXPLAIN-validated | `2921719` |
| 2 | `mdemg graph backfill-weights`: ~56k edges healed → **0 NULL / 57,395 globally** | `d39bed6` |
| 3 | Gauge + `null_weight_abstraction_edges` rule (evaluator 16→17) | `24f66f3` |
| 4–5 | Tier 3 + docs | (this) |

## Live highlights

- **The bug class self-corrected mid-sprint in the most literal way:** the
  backfill count GREW during the run (36,416 → 41,623) because the running
  server still predated the Epic-1 fix and kept minting NULL edges —
  caught live, server restarted on the fixed binary, stragglers swept.
- **End-to-end through the real pipeline:** a real consolidation (22
  themes) minted edges with varied real cosine weights (0.83–0.94) and
  CUIDv2 ids.
- **LIMIT-5 discipline paid off:** the trial surfaced the ≈1.0 weight mass
  before the full run; investigation attributed it to single-member-cluster
  degeneracy (centroid ≡ member embedding) — real data, faithfully
  encoded, and a quantified head start for HIDDEN-CHURN-001.
- **UVTS harness discovery + restoration (operator-directed):** the quick
  profile's corpus space `lnl-demo-whk` had been deleted with zero trace —
  no UVTS run since 2026-05-04 measured anything real. Restored by full
  reingest of `/Users/reh3376/whk-wms` @ `c1e4263e` (8,974 files, 0 errors,
  35,146 symbols, 28m) — and the post-ingest consolidation became the fixed
  creation sites' first at-scale run: **9,500 abstraction edges, 0 NULL,
  weights min 0.742 / mean 0.923 / max 1.0.** A fresh baseline NUMBER
  remains blocked by further harness rot found live (grader/persist
  breakage, expected-path format drift, vector post-filter dilution
  amplified by the duplicate `whk-wms` space) — full defect inventory in
  `verification.md`, handed to **UXTS-CI-001** as its starting audit.
  Retrieval ranking on the restored corpus verified correct (expected
  files at ranks 1–4).
- **Second live-smoke fix (own commit):** the ingest CLI's consolidation
  call shared the 300s batch timeout — client gave up while the server
  finished (GUIDANCE-SYNTH-001 bug class). New `--consolidate-timeout` /
  `INGEST_CONSOLIDATE_TIMEOUT_SEC` (default 1800s), live-verified.

## Decisions (disclosed)

- **Standalone `backfill-weights` subcommand, not a `graph repair` step:**
  repair's orphan sweep would delete the pre-fix orphan observations the
  operator chose to keep (2026-06-10). Avoids a destructive side effect
  hiding inside a healing command.
- **Backfilled all spaces** (mdemg-dev + whk-wms 8,755 + linear 199) — the
  bug was global, the heal is global.
- **Historical UUID edge_ids left in place** (alert-ID precedent: opaque
  strings, both formats valid).

## Follow-ups

- **HIDDEN-CHURN-001** (Phase 2) — now with quantified degeneracy evidence:
  ~50% of abstraction edges sit at ≈1.0 because their parents are
  single-member clusters.
- Decay/prune still don't run on schedule — **MAINT-LIVE-001** is next
  (Phase 1 #4); the weights they need are now real.

## Documents Accessed

- `internal/hidden/service.go` (3 sites) + `member_edges.go` (new)
- `internal/cli/graph_repair.go` (step pattern; deliberately not extended),
  `graph_weight_backfill.go` (new), `graph.go`
- `internal/api/server.go::collectNeo4jGraphData` (Query 4),
  `internal/metrics/collectors.go`, `internal/alert/rules.go`,
  `internal/config/config.go`, `internal/cli/serve.go`
- Live Neo4j (proofs, counts, distributions); `metric_samples` (gauge rows)
- `docs/development/roadmap/ROADMAP_2026Q3.md`
