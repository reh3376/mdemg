# DORMANT-CENSUS-002 — Sprint Post

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #8.
**Parent sprint:** DORMANT-CENSUS-001 (`verify_route_consumers.py`
+ route inventory).

## Verdict

**Shipped.** New forcing function `verify_tsdb_consumers.py` +
adjudicated inventory of all 26 TSDB tables, wired into CI. Two
dormant-to-remove tables identified. Two column-level dormant
surfaces + one guidance data-hole disclosed as follow-ups.

## What we set out to answer

The Q4 deep-dive named three specific defects and framed them as an
instance of a repeatable class: "the deep-dive named X as unemitted /
Y as nonexistent / Z as unwatched — the class recurs." DORMANT-CENSUS-001
had shipped this pattern for API routes; this sprint extends it to
TSDB tables.

## S1 — Verify 3 named deep-dive defects

| # | Named defect | Verdict | Detail |
|---|---|---|---|
| 1 | `mdemg_ft_production_drift` gauge as unemitted | **STILL TRUE** | The `FtProductionDriftRule` alert queries `ft_model_versions` + `benchmark_runs` directly. No gauge is emitted, so no historical trace exists outside alert-fire events. Not urgent — the alert works — but there's no dashboard-visible drift trend. |
| 2 | `suggested_guidance` table as nonexistent | **PARTIALLY RESOLVED** | No standalone table (correct: doesn't exist), but a COLUMN with this name exists on `review_grades` (added by HITL-REVIEW-001 migration 029). Populated in 2/18 rows. Zero code readers. Column-level dormant surface. |
| 3 | heuristic-classifier-share as unwatched | **RESOLVED** | CLASSIFIER-CONSISTENCY-001 shipped `HeuristicShareRule` (`heuristic_share_high` alert, `HEURISTIC_SHARE_THRESHOLD`/`HEURISTIC_SHARE_LOOKBACK_HOURS` config). Verified wired in `serve.go` + `alert/rules.go`. |

## S2 — 2 session-surfaced signals

**Empty `constraint_code` guidance** (from JIMINY-CEILING-INVESTIGATION-001):
Guidance `rlgol248e1ftcdknf8t8zjpp` had 50+ empty-code outcomes in the
deep-dive's 7d window. Re-query today shows 99 empty-code outcomes over
the last 48h from various guidance_ids — **the specific ID rolled out
but the CLASS persists.** Root cause: guidance items synthesized without
knowing their source constraint's code emit `item.ConstraintCode = ""`
which flows to `constraint_outcomes` via `s.outcomeWriter.RecordOutcome`.

Not a broken pipeline (all rows have `constraint_id` and other fields);
it's a lookup-key-missing data hole. Follow-up: named as
`JIMINY-CODE-BACKFILL-001` in the disclosed section below.

**Retrieval reverse-lookup gap** (from RETRIEVAL-QUALITY-AUDIT-001):
No existing reference-index / grep-column / symbol-references machinery
in `internal/retrieval/`. Confirmed. Follow-up spec is
`RETRIEVAL-REVERSE-LOOKUP-001` (already disclosed by RQA-001).

## S3 — Broader inventory sweep

**26 TSDB tables** declared in migrations (excluding
`tsdb_schema_meta` infra table).

**Row counts on mdemg-dev** (live):

| Category | Count | Examples |
|---|---|---|
| High-volume load-bearing (>10k rows) | 8 | `metric_samples` (80M), `embedding_events` (1.6M), `reinforcement_events` (556k), `llm_interactions` (107k), `constraint_outcomes` (8.9k), `guidance_conflicts` (8.8k), `retrieval_events` (8.3k), `retrieval_audit` (7.5k) |
| Medium-volume active (100-10k rows) | 5 | `guidance_training_rows`, `sparse_gate_metrics`, `scheduled_job_events`, `uvts_results`, `benchmark_results` |
| Low-volume active (1-100 rows) | 11 | `context_catalog_versions`, `contradicted_correction_drafts`, `ft_model_versions`, `ft_training_cycles`, `llm_endpoint_health_events`, `model_install_events`, `benchmark_runs`, `review_grades`, `rl_training_runs`, `rl_training_steps`, `uvts_runs` |
| **Zero rows** | **2** | **`ft_benchmarks`, `ft_hitl_decisions`** |

**Zero-row tables — genuine dormant surfaces:**

1. **`ft_benchmarks`** — defined in `002_ft_schema.sql` (early FT
   schema). NO writers. Only reference is an informational listing in
   `internal/cli/tsdb.go:252`. Superseded by `benchmark_runs` +
   `benchmark_results`. **Disposition: DORMANT_TO_REMOVE**.
2. **`ft_hitl_decisions`** — same migration, same pattern. NO writers.
   Referenced in `internal/cli/tsdb.go:253,275` for informational
   listing + space-scoped teardown. Superseded by HITL-REVIEW-001's
   `review_grades`. **Disposition: DORMANT_TO_REMOVE**.

**Metrics inventory** (146 emitted, 57 dashboards+alerts references):
raw grep-diff showed 98 candidate-dormant-writers + 9
candidate-empty-readers, but the diff has high false-positive rate
(histogram base names + `_p95` percentile derivatives) — a
gauge-level forcing function is NOT shipped this sprint; deferred to
a follow-up because the false-positive triage would substantially
enlarge scope.

## S4 — Forcing function shipped

**`scripts/verify_tsdb_consumers.py`** (mirrors DORMANT-CENSUS-001's
`verify_route_consumers.py` pattern):

- Greps `internal/tsdb/migrations/*.sql` for `CREATE TABLE` declarations
- Diffs against `docs/api/tsdb_consumer_inventory.json` (writers +
  readers + disposition per table)
- Fails on bidirectional drift + UNREVIEWED disposition
- `--generate` mode rebuilds the inventory skeleton (preserving
  existing adjudications)
- Wired into `.github/workflows/ci.yml` as a merge-blocking step
  alongside the shipped verifiers

**Initial adjudicated inventory**: 26 tables, 24 IN_USE, 2
DORMANT_TO_REMOVE (`ft_benchmarks`, `ft_hitl_decisions`).

**Verification**: `python3 scripts/verify_tsdb_consumers.py` returns
"tables: 26 declared, 26 inventoried; OK — no drift".

## Follow-ups disclosed

Ranked by (expected impact × effort⁻¹):

1. **FT-DORMANT-CLEANUP-001** (~1d) — migration to DROP `ft_benchmarks`
   + `ft_hitl_decisions`; update `internal/cli/tsdb.go` listings.
   Non-destructive (tables have 0 rows); operator-authorization for
   the schema-drop migration.
2. **REVIEW-SUGGESTED-GUIDANCE-CONSUME-001** (~2d) — wire a consumer
   for `review_grades.suggested_guidance` (SME corrective examples).
   Options: (a) include in the retrain corpus assembly, (b) surface
   in a HITL analytics panel, (c) formally deprecate the column if
   the intent has shifted.
3. **JIMINY-CODE-BACKFILL-001** (~1d) — investigate the empty-
   `constraint_code` class in `constraint_outcomes` (99 rows/48h);
   either backfill from `constraint_id → constraint_code` join at
   write-time or document the class as expected for a subset of
   guidance items.
4. **DORMANT-CENSUS-003** (metrics gauges, ~3d) — build the
   gauge-level verifier with false-positive triage (histogram base
   ↔ percentile derivative mapping, snapshot-reader recognition).
5. **RETRIEVAL-REVERSE-LOOKUP-001** — already disclosed by
   RETRIEVAL-QUALITY-AUDIT-001; not a DORMANT-CENSUS-002 discovery.

## Rules pinned

1. **Every new TSDB table declared in `internal/tsdb/migrations/*.sql`
   MUST be adjudicated in `docs/api/tsdb_consumer_inventory.json`** in
   the same PR. CI fails the merge otherwise. Same pattern DOC-CURRENCY-002
   and DORMANT-CENSUS-001 established for other surfaces.
2. **A `DORMANT_TO_REMOVE` disposition names the table as safe-to-drop
   in a future cleanup sprint** but does NOT trigger removal — cleanup
   is per-defect follow-up work with operator authorization + a
   migration.
3. **When a column-level dormant surface is found** (like
   `suggested_guidance` — column populated but no reader), record it
   in the parent table's `notes` field; if unresolved after 30d,
   escalate to its own cleanup sprint.

## Documents Accessed

- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (candidate #8)
- `docs/development/dormant-census-002/sprint_plan.md` (this dir)
- `docs/development/jiminy-ceiling-investigation-001/post.md` +
  `docs/development/retrieval-quality-audit-001/post.md` (this
  session's investigation findings that surfaced signals #4 + #5)
- `docs/api/route_consumer_inventory.json` (DORMANT-CENSUS-001 pattern)
- `scripts/verify_route_consumers.py` (mirrored shape)
- `internal/tsdb/migrations/*.sql` (26 CREATE TABLE statements)
- `internal/tsdb/*_writer.go` files (writer greps)
- `internal/alert/rules.go` + `dashboards/*.json` (reader greps)
- Live TSDB `information_schema.tables` + per-table `count(*)` queries
- Verifier local run + inventory generation
