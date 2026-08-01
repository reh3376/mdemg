# METRICS-VERIFIER-UOBS-UOTS-001 — Sprint Post

**Date:** 2026-07-31 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-METRICS-CLEANUP-001 CI cycle disclosed
follow-up.

## Verdict

**Shipped.** `scripts/verify_metrics_consumers.py` now scans UOBS
+ UOTS spec paths as consumer roots. Four IN_USE_TSDB_ONLY entries
that were coupled to observability contracts got correctly
auto-promoted to IN_USE. The class of failure that CI-caught during
DORMANT-METRICS-CLEANUP-001 (removing `retrieval_cache_hits_total`
broke three specs that still asserted its presence) is now
surface-visible at inventory-review time BEFORE removal.

## What shipped

- **`scripts/verify_metrics_consumers.py`** —
  - `CONSUMER_ROOTS` extended with:
    - `docs/tests/uobs/specs` (UOBS runtime metric-presence contracts)
    - `docs/api/api-spec/uots/specs` (UOTS artifact-observability contracts)
  - Docstring's false-positive triage section updated to document
    this coverage
  - `generate()` — new auto-promotion step: an existing
    IN_USE_TSDB_ONLY entry is promoted to IN_USE the moment a
    UOBS/UOTS spec references it. Does NOT auto-demote (operator
    marking DORMANT_* explicitly still wins). Idempotent.
- **`docs/api/metrics_consumer_inventory.json`** — 4 entries promoted
  IN_USE_TSDB_ONLY → IN_USE:
  - `neo4j_graph_total_spaces` (referenced by
    `prometheus_neo4j_graph.uots.json`)
  - `probe_latency_ms` (referenced by `health_probes.uobs.json`)
  - `tsdb_pool_empty_acquire_total` (referenced by
    `prometheus_neo4j_pool.uots.json`)
  - `tsdb_pool_max_conns` (referenced by
    `prometheus_neo4j_pool.uots.json`)

## Inventory distribution

| Disposition | Before | After |
|---|---|---|
| IN_USE | 92 | **96** |
| IN_USE_TSDB_ONLY | 53 | **49** |
| **TOTAL** | 145 | 145 |

## Rules pinned

⚠️ **When adding a UOBS/UOTS spec assertion for a metric, the metric-
consumer inventory now auto-promotes** — no manual `--generate` +
adjudication needed. But when REMOVING a metric, the operator MUST
still scan the UOBS/UOTS specs and remove the orphaned assertions in
the same PR (the promotion is a signal FOR keeping the metric alive,
not a fix for the removal path).

⚠️ **Auto-promotion is one-directional** — an IN_USE_TSDB_ONLY entry
that gains a spec assertion becomes IN_USE, but a DORMANT_* or
IN_USE_SNAPSHOT_ONLY entry keeps its operator-set disposition. The
verifier respects explicit adjudication over passive discovery.

## Documents Accessed

- `docs/development/dormant-metrics-cleanup-001/post.md` (parent —
  the CI cycle that disclosed this gap)
- `scripts/verify_metrics_consumers.py` (target)
- `docs/tests/uobs/specs/` + `docs/api/api-spec/uots/specs/` (new
  consumer roots)
- `docs/api/metrics_consumer_inventory.json` (updated)
