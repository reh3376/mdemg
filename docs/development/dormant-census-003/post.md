# DORMANT-CENSUS-003 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 disclosed follow-up #4.

## Verdict

**Shipped.** Metrics-registry gauges, counters, and histograms now
have a build-time adjudication registry mirroring DORMANT-CENSUS-001
(routes) and DORMANT-CENSUS-002 (TSDB tables). 152 declared metrics
enumerated + inventoried + adjudicated on first-pass; merge-blocking
CI check wired. Both false-positive classes DORMANT-CENSUS-002 named
as deferral blockers are handled: histogram-derivative expansion (base
→ `_p95`/`_p99`/`_bucket`/`_sum`/`_count`) and snapshot-reader
recognition.

## What shipped

- **`scripts/verify_metrics_consumers.py`** — enumerates declared
  metrics via regex over `internal/**/*.go` (excluding `_test.go`)
  matching `\w+\.New(Counter|Gauge|Histogram)("<name>"…)`. Consumer
  walker scans:
  - `deploy/docker/grafana/**/*.json` (dashboard panels)
  - `internal/cli/grafana_templates/staged/**/*.json` (embedded mirror)
  - `internal/alert/*.go` (evaluator rules)
  - `internal/ape/`, `internal/api/`, `internal/tsdb/`,
    `internal/consulting/` (in-process readers)
  - Extensions searched: `.go`, `.json`, `.yaml`, `.yml`, `.sql`
  - For histograms, ALL five derivative names are searched with and
    without the `mdemg_` namespace prefix — a consumer that only
    references `mdemg_retrieval_latency_seconds_p95` is correctly
    counted for the base `retrieval_latency_seconds` declaration.
- **`docs/api/metrics_consumer_inventory.json`** — 152 entries. Each
  carries `type`, `declaration` (file:line), `consumers` list,
  `disposition`, `notes`. Same shape as
  `tsdb_consumer_inventory.json`.
- **Disposition vocabulary** extended with two new values documented
  in the verifier docstring:
  - `IN_USE_TSDB_ONLY` — no direct dashboard/alert/reader hit but the
    metric is written to `metric_samples` by the recorder every
    flush. Reflects the shipped architecture (`internal/metrics/
    recorder.go`); a follow-up review can re-adjudicate specific
    entries to `DORMANT_TO_REMOVE` if downstream value is confirmed
    absent.
  - `IN_USE_SNAPSHOT_ONLY` — consumed only via
    `/v1/metrics/snapshot` (soft-signal path; the walker records
    snapshot-consumer file count for operator adjudication).
- **`.github/workflows/ci.yml`** — new "Metrics consumer guard
  (merge-blocking)" step alongside the shipped route + TSDB guards.
- **`docs/development/UXTS_FRAMEWORK_MATRIX.md`** — new §5a
  "Companion Adjudication Registries" documenting the three-strong
  DORMANT-CENSUS family (routes, TSDB tables, metrics) as sibling
  build-time drift checks that complement UOBS (runtime metric-
  presence contracts) and UOTS (artifact-level observability), rather
  than overlapping with them.

## First-pass adjudication census

Regeneration output:
```
generated 152 new entries + refreshed 0 existing (total 152, live metrics 152)
metrics: 152 declared, 152 inventoried; OK — no drift
```

Distribution:

| Disposition | Count |
|---|---|
| IN_USE (direct dashboard/alert/reader hit) | 92 |
| IN_USE_TSDB_ONLY (auto-adjudicated: recorder writes to metric_samples) | 60 |
| **TOTAL** | **152** |

By type: 100 gauges, 42 counters, 10 histograms.

**Zero UNREVIEWED** — the auto-disposition path adjudicates every
metric to either IN_USE (grep-hit found) or IN_USE_TSDB_ONLY
(recorder is the load-bearing consumer). Operators can re-adjudicate
IN_USE_TSDB_ONLY entries in a future cleanup sprint if a downstream
review confirms specific metrics have no consumer value.

## Rules pinned

⚠️ **When adding a new metric via `r.NewCounter/NewGauge/NewHistogram`
in `internal/**/*.go`, the CI drift check auto-generates an
inventory entry on next `--generate` run.** Run `python3
scripts/verify_metrics_consumers.py --generate` locally + commit the
inventory update in the same PR. The CI check fails on missing
entries.

⚠️ **When adding a metric consumer** (Grafana panel, alert rule, Go
reader), no manual inventory update is needed — the next
`--generate` run picks up the new consumer path automatically and
promotes the metric's disposition from IN_USE_TSDB_ONLY to IN_USE.
But for consistency, refresh the inventory in the same PR so the
consumers list stays accurate.

⚠️ **Histograms produce FIVE derivative names in TSDB** (base +
`_p95`, `_p99`, `_bucket`, `_sum`, `_count`). Grep-based consumer
detection MUST expand each histogram declaration to all five variants
— this is the deferral-blocker class DORMANT-CENSUS-002 named
explicitly. The verifier handles it in `_variant_names()`.

⚠️ **The `metric_samples` recorder is a load-bearing consumer of
every declared metric.** Auto-disposition for metrics without direct
grep-hits is IN_USE_TSDB_ONLY, not DORMANT_TO_REMOVE, because the
declaration IS observed by the recorder-to-TSDB path. Re-adjudicate
to DORMANT_TO_REMOVE only after a downstream review confirms no
consumer value.

## Follow-ups disclosed

1. **DORMANT-METRICS-CLEANUP-001** (future, ~1-2d) — go through the
   60 IN_USE_TSDB_ONLY entries and re-adjudicate each. Some are
   likely genuinely dormant (never had a downstream consumer even in
   design) and can be re-marked DORMANT_TO_REMOVE, gathered into a
   removal PR. Others reflect intentional "collect now, dashboard
   later" telemetry and stay as IN_USE_TSDB_ONLY.
2. **Snapshot API consumer inventory** — the `/v1/metrics/snapshot`
   endpoint currently has consumers in tests + docs, not live
   dashboards. If a future operator tool starts consuming it, the
   metrics accessible via snapshot become an inventory of its own.
   Not urgent.
3. **UxTS relationship documented** — DORMANT-CENSUS-003 is NOT a
   UxTS spec (mirrors DORMANT-CENSUS-001/-002 as a build-time script
   + JSON audit). UOBS is the closest UxTS peer: it declares
   RUNTIME metric-presence contracts against
   `/v1/metrics/snapshot`. The two are COMPLEMENTARY — UOBS asserts
   a spec-required metric IS live in the snapshot; DORMANT-CENSUS-003
   asserts EVERY declared metric has an adjudicated consumer at
   build time. No overlap; no duplication.

## Documents Accessed

- `docs/development/dormant-census-002/post.md` (parent — follow-up
  #4)
- `docs/development/dormant-census-003/sprint_plan.md` (this dir)
- `scripts/verify_route_consumers.py` (DORMANT-CENSUS-001 shape
  reference)
- `scripts/verify_tsdb_consumers.py` (DORMANT-CENSUS-002 shape
  reference — direct template)
- `internal/metrics/registry.go` (declaration API)
- `internal/metrics/collectors.go` (bulk of declarations)
- `internal/metrics/recorder.go` (histogram → 5-derivative expansion
  logic that motivated the false-positive triage)
- `internal/metrics/snapshot.go` (`/v1/metrics/snapshot` endpoint)
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` (updated §5a companion
  registries)
- `.github/workflows/ci.yml` (CI hook placement)
- `docs/tests/uobs/specs/` (UOBS peer reference —
  spec-required-metrics contract)
