# Sprint GRAFANA-AUDIT-001 — Epic 1 Audit Summary

**Audit run**: 2026-05-21, against live `mdemg-timescaledb-1` (TSDB schema V0021, `space_id=mdemg-dev`, `instance=localhost:9999`, default time-window `now() - interval '24 hours'`).

**Harness**: `scripts/grafana_panel_audit.py` (17 Tier 1 unit tests green). Per-panel rawSql is extracted from each dashboard JSON, Grafana macros + template variables substituted, executed via `docker exec psql`, classified PASS / EMPTY / FAIL / SKIP. Full per-panel results in `audit_results.json`.

## Headline numbers

| Verdict | Count | % of total |
|---|---|---|
| **PASS** (executes; ≥1 row in 24h window) | 125 | 76% |
| **EMPTY** (executes; 0 rows in window) | 19 | 12% |
| **FAIL** (SQL error) | 3 | 1.8% |
| **SKIP** (text/row panel — no SQL) | 18 | 11% |
| **Total target executions** | 165 | — |

165 target executions across 146 panels (some panels have multiple SQL targets — e.g. `Request Latency Distribution` has 3 percentile targets).

## Per-dashboard breakdown

| Dashboard | PASS | EMPTY | FAIL | SKIP | Health |
|---|---|---|---|---|---|
| `mdemg-rsic` (largest, 50 targets) | 43 | 7 | 0 | 0 | 86% PASS |
| `mdemg-j17` | 34 | 1 | 0 | 1 | 97% PASS |
| `mdemg-jiminy` | 19 | 1 | 0 | 2 | 95% PASS |
| `mdemg-neo4j` | 11 | 0 | 0 | 6 | 100% PASS |
| `mdemg-overview` | 11 | 2 | 0 | 0 | 85% PASS |
| `mdemg-ft-training` | 6 | 8 | 0 | 1 | 43% PASS (largest EMPTY share) |
| `mdemg-llm-routing` | 1 | 0 | 3 | 0 | 25% PASS (only failing dashboard) |
| `mdemg-graph-topology` | 0 | 0 | 0 | 8 | n/a (all panels are non-SQL types) |

## Verdict by root cause

### 3 FAILs — all on `mdemg-llm-routing`, single root cause (category-a SQL bug)

All three panels share the same authoring error: hardcoded `mdemg-dev` (without quotes) in the SQL `WHERE` clause instead of using the template variable `'$space_id'`. PostgreSQL parses `mdemg-dev` as `mdemg minus dev` → "column 'mdemg' does not exist" error.

This violates the framework's no-hardcoding rule (memory: `feedback_no_hardcoded_values.md`) AND is a real SQL bug.

**Affected panels (`mdemg-llm-routing.json`):**
1. `LLM call distribution by model_name (24h)` — t0
2. `LLM latency p50 / p95 / p99 by task × model` — t0
3. `LLM error rate % by task_name (selected range)` — t0

Fix: replace the literal `mdemg-dev` with `'$space_id'` in each panel's `WHERE` clause. Minimum-change JSON edit, ~3 line changes total.

### 19 EMPTYs — three sub-categories

#### (b) Schema drift — label/type mismatch (potential fix targets, ~5 panels)

The metric is emitted to `metric_samples` but with `metric_type` or `labels` shape that doesn't match what the panel's SQL filters on. Real authoring drift; fixable by aligning panel filters with the actual emitted data.

- **`mdemg-j17 :: Total Events`** — panel filters `metric_type = 'counter'` (Prometheus naming convention because metric is `mdemg_j17_events_total`), but server emits `metric_type = 'gauge'`. Mismatch: panel-vs-server. Fix candidate: align panel filter, OR re-emit as counter on server side.
- **`mdemg-rsic :: Action Success Rate`** (2 targets) — panel filters `labels->>'status' = 'success'` (assumed; truncated SQL); server emits `labels->>'status' = 'completed'`. Fix candidate: align panel filter.
- **`mdemg-rsic :: Calibration Confidence`** — metric exists; panel may have stale labels filter. Needs full SQL inspection.
- **`mdemg-rsic :: Trigger Rejection Rate`** — same metric exists (`mdemg_rsic_trigger_rejected_total`, counter). EMPTY suggests no rejections in the last 24h (legitimate zero) OR label filter mismatch.

#### (c) Missing server-side metric emission (2 panels — server-side work)

Panel SQL references a metric_name that doesn't exist in `metric_samples` at all. Server-side emission never wired; documenting the gap is in-scope, fixing the emission is a follow-up sprint.

- **`mdemg-overview :: Request Latency Distribution`** (t0) — references `mdemg_http_request_duration_seconds_p50`. Only `_p95` and `_p99` are emitted. Server emits no p50.
- **`mdemg-overview :: Rate Limit Rejections`** — references `mdemg_rate_limit_rejected_total`. Not emitted anywhere.

#### (d) Sparse-data EMPTYs (legitimate; documentation-only)

Panel SQL is correct; the underlying tables just don't have rows in the audit's 24-hour window. Not breakage. Fixable by widening default time-range in panels for rare events.

- **`mdemg-ft-training`** (8 EMPTYs): `ft_model_versions`, `ft_benchmarks`, `ft_training_cycles`, `benchmark_runs`, `llm_interactions` (Entropy Health quality-filtered), `llm_endpoint_health_events` (2 rows total) — training pipeline tables on this dev instance haven't been touched in 24h.
- **`mdemg-jiminy :: Effectiveness Trends`** — CTE-based query; `constraint_outcomes` has 1.1K rows but the panel's filter or grouping returns 0.
- **`mdemg-rsic`** (3 sparse EMPTYs): `Trigger Rejections by Reason`, `Safety Blocks`, `Snapshots Created` — counters that may legitimately be 0 across 24h on a quiet dev box.

## Triage for Epic 2 → Epic 3/4

| Category | Count | Sprint plan epic |
|---|---|---|
| (a) Real SQL bug (hardcoded `mdemg-dev`) | 3 | Epic 3 (Tier 1 fix) |
| (b) Schema drift — label/type filter mismatch | ~5 | Epic 3 (Tier 1 fix) |
| (c) Missing server-side metric | 2 | Epic 4: doc in "Known gaps"; server emission is follow-up sprint |
| (d) Sparse-data EMPTYs (legitimate) | ~11 | Epic 4: widen default time-range to 7d for rare events |
| Subtotal needing Epic 3/4 action | 21 of 165 (13%) | |
| Healthy panels | 125 PASS + 18 SKIP-by-design | 87% |

## Notes on harness reliability

First-pass audit (before harness fix) reported 20 FAILs. Investigation showed 18 of those were **false positives** from a quoting bug in the `$__interval` substitution — the harness wrapped the substituted value in quotes, but Grafana convention is for panel SQL to provide the quotes itself, producing `''1 minute''` (doubled). Fixed in commit `7679fec` (Epic 0 harness) follow-up — `$__interval` now substitutes BARE `1 minute` so panel-provided quotes work correctly. Verified by re-running: 20→3 FAILs, confirms only mdemg-llm-routing has real authoring bugs.

Lesson: trust rigorous audit-with-fix-loop over single-pass samples. The first 11-panel manual sample in Phase 1 happened to land on PASS panels and missed all 3 real FAILs; the harness caught all 3 on first sweep.

## Next steps

- **Epic 2 (findings.md)** — group failures by root cause; surface to operator
- **Epic 3** — fix 3 category-(a) panels in `mdemg-llm-routing.json` + 5 category-(b) panels; re-run harness; expect ≤16 EMPTY post-fix (only sparse-data legitimate ones)
- **Epic 4** — widen time-ranges for sparse-data panels (category d); document category-(c) gaps in feature doc
- **Epic 5** — add coverage panels for the 11 unused TSDB tables
