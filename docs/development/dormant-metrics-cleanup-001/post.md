# DORMANT-METRICS-CLEANUP-001 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-003 disclosed follow-up.

## Verdict

**Conservative first-pass cleanup shipped.** Reviewed all 60
IN_USE_TSDB_ONLY entries from DORMANT-CENSUS-003. Two-gate proof of
death applied: (a) `.Field`/`.Method()` grep across `internal/`
returns ZERO writer sites, AND (b) `metric_samples` shows ZERO samples
over 7 days. Seven metrics passed BOTH gates and are removed from
`internal/metrics/collectors.go`. Inventory updated + verifier passes
clean (152→145 declared, 152→145 inventoried, zero drift).

## Adjudication method

DORMANT-CENSUS-003 auto-adjudicated the 60 UNREVIEWED entries to
`IN_USE_TSDB_ONLY` — the recorder flushes every declared metric to
`metric_samples` regardless of whether anything downstream reads it.
This sprint runs a stricter test: is the metric ACTUALLY WRITTEN by
any code path?

**Two-gate rule for a HARD-DEAD verdict:**
1. `.Field`/`.Method()` grep across `internal/**/*.go` (excluding
   test files + the declaration file itself) returns ZERO writer
   sites, AND
2. `metric_samples` shows ZERO samples over the 7-day window on
   mdemg-dev

Gate 2 alone isn't enough — a `_failures_total` counter can be wired
correctly but sit at zero simply because no failure occurred in 7d.
Gate 1 confirms nothing even TRIES to write the value.

## Live diagnosis distribution

Applied to the 60 IN_USE_TSDB_ONLY entries:

| Bucket | Count | Action |
|---|---|---|
| Zero writer sites + zero 7d samples (HARD-DEAD) | 7 | REMOVE |
| Has ≥1 writer + zero 7d samples (wired, quiet) | 11 | KEEP (real telemetry, no trigger in 7d) |
| Has ≥1 writer + non-zero 7d samples (active) | 42 | KEEP |

## Seven metrics removed

| Metric | Field/Method | Reason |
|---|---|---|
| `cms_dedup_skips_total` | `CMSDedupSkips` | 0 writers + 0 samples/7d |
| `cms_learning_edge_failures_total` | `CMSLearningEdgeFails` | 0 writers + 0 samples/7d |
| `cms_recall_total` | `CMSRecallTotal` | 0 writers + 0 samples/7d |
| `cms_resume_total` | `CMSResumeTotal` | 0 writers + 0 samples/7d |
| `jiminy_guide_timeout_total` | `JiminyGuideTimeout` | 0 writers + 0 samples/7d (timeouts covered by `jiminy_warm_errors_total`) |
| `retrieval_cache_hits_total` | `RetrievalCacheHits` | 0 writers + 0 samples/7d (retrieval column cache accounted separately) |
| `retrieval_cache_misses_total` | `RetrievalCacheMiss` | 0 writers + 0 samples/7d (same) |

Total field/method declarations removed:
- **7 struct fields** in `StandardMetrics`
- **1 factory-function assignment** (`JiminyGuideTimeout`)
- **6 `r.New…` initializer statements**
- **1 test-only usage** (`m.RetrievalCacheHits.Inc()` in
  `metrics_test.go` — the metric was never used outside this one
  test)

Each removal-site block carries a `DORMANT-METRICS-CLEANUP-001`
comment explaining WHY the metric was dropped + pointing to the
alternate path (where one exists).

## Eleven metrics kept as IN_USE_TSDB_ONLY despite zero 7d samples

These have real writer code but the trigger just hasn't fired in 7d:

- `cms_dedup_merge_failures_total`, `cms_embedding_failures_total`,
  `cms_observe_total`, `cms_stability_update_failures_total`,
  `cms_writejson_failures_total` — error/rare-path CMS counters
- `eventgraph_writer_flush_failure_total`, `guidance_corpus_flush_failure_total`,
  `guidance_corpus_rows_dropped_total` — buffered-writer error paths
- `jiminy_warm_errors_total` — warm-compute error path
- `retrieval_column_failed_total` — retrieval column error path
- `rsic_llm_semaphore_blocked_total` — semaphore-block signal

**KEEPING these is deliberate**: they're real error/failure signals
that we WANT to see if they ever fire, and they cost effectively
nothing to keep declared (the recorder flushes zero values with
minimal storage overhead). Removing them would silently lose the
signal if the underlying condition ever recurs.

## Rules pinned

⚠️ **Two-gate rule for HARD-DEAD metric verdict**: BOTH zero
`.Field`/`.Method()` writer sites in the source tree AND zero samples
over a 7d live window. Either alone is insufficient — the sample gate
misses wired-but-quiet error counters (real signals kept as
IN_USE_TSDB_ONLY); the writer gate misses dead factories that a
`_test.go` file happens to keep referencing.

⚠️ **When removing a metric declaration, its inventory entry must be
DELETED (not marked REMOVED)** — metrics differ from TSDB tables here:
tables have historical CREATE-TABLE statements that persist in their
original migration files, so DORMANT-CENSUS-002's `REMOVED`
disposition (entry stays, disposition flips) is coherent. Metric
declarations vanish entirely from the code, so the inventory entry
must vanish too. The verifier's "removed inventory" check enforces
this cleanly.

⚠️ **When removing a metric, scan the test files for stale
references** — `metrics_test.go` had a single `m.RetrievalCacheHits.
Inc()` line that would have broken the build. The grep should extend
to `_test.go` when doing the removal (even though `_test.go` is
excluded from the writer-site count that establishes deadness — the
distinction is: tests can reference a dead metric harmlessly IF the
metric still exists, but a test reference doesn't ARGUE for keeping
the metric alive).

## Follow-ups disclosed

None. The other 53 metrics (42 active + 11 wired-but-quiet) are
all healthy and don't need further review. If a future cleanup sprint
wants to be more aggressive it could revisit the 11 wired-but-quiet
counters after 30+ days of continued zero-emission, but the
information cost of that removal is real (loss of an error signal)
and shouldn't be paid unless the storage cost becomes material.

## Documents Accessed

- `docs/development/dormant-census-003/post.md` (parent — disclosed
  follow-up)
- `docs/api/metrics_consumer_inventory.json` (the 60 IN_USE_TSDB_ONLY
  entries under review)
- `internal/metrics/collectors.go` (declarations)
- `internal/metrics/metrics_test.go` (test stale-reference fix)
- Live SQL against `metric_samples` on mdemg-dev (7d sample counts)
- Grep across `internal/**/*.go` for `.<Field>` / `.<Method>(`
  writer-site counts
