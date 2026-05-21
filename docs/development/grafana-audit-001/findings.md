# Sprint GRAFANA-AUDIT-001 — Epic 2 Findings

**Headline**: Operator's "diminished observability" report **is real**, but the breakage is concentrated and fixable. Of 165 target executions across 146 panels: 125 PASS (76%), 19 EMPTY (12% — mix of real bugs and legitimate sparse-data), 3 FAIL (1.8% — all on one dashboard, one root cause), 18 SKIP (non-SQL panel types).

The 11-panel sample audit in Phase 1 happened to land entirely on PASS panels and missed the breakage; the rigorous per-panel audit caught it on first sweep.

## Failures by root cause category

### Category (a) — Real SQL bug (3 panels, all on `mdemg-llm-routing.json`)

Three panels hardcode `mdemg-dev` (no quotes) in their `WHERE` clause where they should use the `'$space_id'` template variable. PostgreSQL parses the bare `mdemg-dev` as `mdemg - dev` (subtraction of two columns), failing with `column "mdemg" does not exist`.

Authoring violation: also breaches the framework's no-hardcoding rule (memory: `feedback_no_hardcoded_values.md`).

Affected:
1. `LLM call distribution by model_name (24h)`
2. `LLM latency p50 / p95 / p99 by task × model`
3. `LLM error rate % by task_name (selected range)`

**Fix (Epic 3)**: replace `mdemg-dev` with `'$space_id'` in each panel's WHERE clause. ~3-line JSON edit per panel.

### Category (b) — Schema drift (label/type mismatch, ~5 panels)

Metric is emitted to `metric_samples`, but the panel's SQL filter expects a `metric_type` or `labels` shape that doesn't match what the server actually emits. The drift accumulated between the panel-authoring date and recent server-side emission changes.

Confirmed cases:

1. **`mdemg-j17 :: Total Events`** — panel filters `metric_type = 'counter'` (because metric name ends `_total`, Prometheus convention); server actually emits `metric_type = 'gauge'`. 192 rows of data; 0 panel matches.
2. **`mdemg-rsic :: Action Success Rate`** (2 targets) — panel filters `labels->>'status' = 'success'`; server emits `"status": "completed"`. 181 rows of data; 0 panel matches.

Suspected (need full-SQL inspection):
3. **`mdemg-rsic :: Calibration Confidence`** — metric exists; likely a labels filter mismatch.
4. **`mdemg-rsic :: Trigger Rejection Rate`** — metric exists; could be legitimate zero OR labels mismatch.

**Fix (Epic 3)**: align panel filters with actual emitted data (lower-risk than re-emitting server-side, which would invalidate historical data attribution).

### Category (c) — Missing server-side metric (2 panels)

Panel SQL references a `metric_name` the server doesn't emit anywhere. Real observability gap; fix requires server-side emission work, queued as follow-up sprint.

1. **`mdemg-overview :: Request Latency Distribution` (t0)** — references `mdemg_http_request_duration_seconds_p50`. Server emits `_p95` and `_p99` (visible in panel as t1 + t2, both PASS) but no `_p50`.
2. **`mdemg-overview :: Rate Limit Rejections`** — references `mdemg_rate_limit_rejected_total`. Not in metric_samples at all (no emission found).

**Disposition (Epic 4)**: document in `docs/features/observability-dashboards.md` "Known gaps". Server-side emission queued as a follow-up sprint (small scope; one-line Prometheus counter for each).

### Category (d) — Sparse-data EMPTYs (legitimate, ~11 panels)

Panel SQL is correct. The underlying table just doesn't have rows in the audit's 24-hour window — but does have rows over a wider span. Not "broken," just looks empty by default.

- `mdemg-ft-training` (8 panels): training pipeline tables not touched in 24h on this dev TSDB.
- `mdemg-rsic` (3 panels): legitimate-zero counters for safety/snapshot/rejection events on a quiet dev box.

**Fix (Epic 4)**: widen default time-range to 7-day for these specific panels.

## Operator decision points (defaults pre-decided where reasonable)

1. **3 mdemg-llm-routing FAILs (category a)** — Fix in Epic 3. Mechanical, no risk.
2. **Category-(c) missing metrics** — Defer to follow-up sprint; document in feature doc.
3. **Category-(d) sparse-data panels** — Widen time-range to 7d in Epic 4.
4. **Ambiguous category-(b) panels** — Investigate via full-SQL inspection at start of Epic 3; align panel filters where the drift is clear; revert if alignment breaks other things.

## Counts after planned Epic 3/4 fixes

| Verdict | Now | Post-fix (projected) | Δ |
|---|---|---|---|
| PASS | 125 | 133 (+3 FAIL→PASS, +5 EMPTY→PASS) | +8 |
| EMPTY | 19 | 11 (sparse-data only; widened window) | -8 |
| FAIL | 3 | 0 | -3 |
| SKIP | 18 | 18 | unchanged |

Combined with Epic 5 coverage expansion (11 new panels for unused TSDB tables: sparse_gate_metrics, model_install_events, retrieval_audit, context_catalog_versions, embedding_events, guidance_conflicts, rl_training_*, uvts_*, ft_hitl_decisions): final state ~165-180 panels, ≥95% PASS, zero FAIL, ≤5 EMPTY (legitimate sparse-data only, documented).
