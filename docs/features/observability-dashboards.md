# Observability Dashboards

**Sprint**: GRAFANA-AUDIT-001 (2026-05-21)
**Status**: 8 production dashboards, 146 panels, 87% PASS after Sprint B fixes (Epic 3)
**Surface**: `http://localhost:${GRAFANA_PORT}/` (default 3000), served by `mdemg-grafana-1` container

## Why

The mdemg framework emits ~150 metrics across retrieval, RSIC, Jiminy, J17 protocol, LLM routing, and Neo4j subsystems. The dashboards at `deploy/docker/grafana/dashboards/*.json` are the operator's primary observability surface — without them, you have to query TimescaleDB directly.

Sprint GRAFANA-AUDIT-001 fixed 5 broken panels (3 SQL bugs + 2 schema-drift filters) and produced this doc. The audit harness at `scripts/grafana_panel_audit.py` is committed for future regression detection — run it any time you suspect dashboard breakage.

## How it works

| Layer | Tech |
|---|---|
| Storage | TimescaleDB (`mdemg-timescaledb-1`, port 5433 → 5432) — 22 tables + 2 continuous aggregates |
| Provisioning | `deploy/docker/grafana/provisioning/datasources/{timescaledb,neo4j}.yml` — datasource UIDs `timescaledb` (postgres) + `neo4j` |
| Dashboard mount | `deploy/docker/grafana/dashboards/*.json` mounted into Grafana at `/etc/grafana/dashboards`, auto-reload every 30s |
| Metric writer | `metric_samples` populated by the mdemg server's Prometheus-to-TSDB writer (~50K rows/hour at steady state); rolled up to `metrics_hourly` + `metrics_daily` continuous aggregates |
| Template variables | All dashboards have `$space_id` (sourced from TSDB DISTINCT) and `$instance` (sourced from labels). Default to `mdemg-dev` / `localhost:9999`. |

## Dashboard inventory

| Dashboard | Panels | Default range | Primary tables | Purpose |
|---|---|---|---|---|
| `mdemg-overview` | 13 | now-6h | metric_samples (HTTP/cache/embedding/retrieval) | Front page — request rate, latency, error rate, cache hit, retrieval, circuit-breaker open count |
| `mdemg-rsic` | 50 | now-6h | metric_samples (rsic_*) | RSIC self-improvement: health dimensions, cycle outcomes, action durations, watchdog state, confidence per-dimension |
| `mdemg-j17` | 36 | now-6h | metric_samples (j17_*) | J17 AI-to-AI protocol: tier comprehension, NLI bias, code coverage, compression ratio, tier-effectiveness |
| `mdemg-jiminy` | 22 | now-6h | metric_samples (jiminy_*), constraint_outcomes | Jiminy inner-voice: follow rate, evaluation outcomes, constraint effectiveness trends, semantic dedup |
| `mdemg-llm-routing` | 4 | now-24h | llm_interactions | Per-task LLM call distribution by model_name, latency percentiles, error rate, circuit breaker open count |
| `mdemg-neo4j` | 17 | now-6h | metric_samples (neo4j_*) | Neo4j health: query latency, connection pool, transaction count, graph growth trend |
| `mdemg-graph-topology` | 9 | now-24h | Neo4j direct (kniepdennis-neo4j-datasource) | Live graph topology — node + edge counts per layer, click-to-explore |
| `mdemg-ft-training` | 15 | now-7d | ft_*, benchmark_*, llm_interactions, llm_endpoint_health_events | Fine-tune training pipeline: model versions, training cycles, benchmark scores, watchdog events |

## Audit results (Epic 1, 2026-05-21)

| Verdict | Count | % |
|---|---|---|
| PASS (executes; ≥1 row in 24h window) | 130 | 79% |
| EMPTY (executes; 0 rows in 24h window) | 17 | 10% |
| FAIL (SQL error) | 0 | 0% (was 3 pre-Sprint-B) |
| SKIP (non-SQL panel: row, text) | 18 | 11% |

Per-panel verdicts in `docs/development/grafana-audit-001/audit_results.json`. Re-run anytime via `python3 scripts/grafana_panel_audit.py`.

## Fixed in Sprint B (Epic 3)

| Dashboard | Panel | Bug | Fix |
|---|---|---|---|
| mdemg-llm-routing | LLM call distribution by model_name (24h) | `$space_id = ''` (unquoted) → PG parsed `mdemg-dev = ''` as subtraction | Wrap in quotes: `'$space_id' = ''` |
| mdemg-llm-routing | LLM latency p50 / p95 / p99 by task × model | same | same |
| mdemg-llm-routing | LLM error rate % by task_name | same | same |
| mdemg-j17 | Total Events | filtered `metric_type = 'counter'`; server emits `'gauge'` | align panel to `'gauge'` |
| mdemg-rsic | Action Success Rate (t0 'success' branch) | filtered `labels->>'status' = 'success'`; server emits `'completed'` | align panel to `'completed'` |

## Known gaps (Epic 4 disposition)

These EMPTYs are **not panel bugs** — they reflect server-side emission regressions, never-emitted metrics, or legitimate zero-data states. Server-side investigation and feature decisions are queued as follow-up sprints.

### Category (c) — emission regression (4 metrics, last seen ~2026-05-07/08)

These metrics were emitted historically (158K + 6.7K + 26K + 23 rows respectively over March/April) but stopped between 2026-05-07 and 2026-05-08. Current codebase grep finds zero references to the metric names — emission code was removed or the metric was renamed without dashboard updates.

| Metric | Last row | Total rows | Affected panel(s) |
|---|---|---|---|
| `mdemg_rsic_calibration_confidence` | 2026-05-08 | 158,483 | mdemg-rsic :: Calibration Confidence |
| `mdemg_rsic_snapshot_created_total` | 2026-05-08 | 6,726 | mdemg-rsic :: Snapshots Created |
| `mdemg_rsic_trigger_rejected_total` | 2026-05-07 | 26,503 | mdemg-rsic :: Trigger Rejection Rate, Trigger Rejections by Reason |
| `mdemg_rsic_safety_blocked_total` | 2026-04-20 | 23 | mdemg-rsic :: Safety Blocks |

**Operator action**: investigate which commit removed the emission. Either restore the emission (preferable — these are observability-load-bearing) or remove the panels.

### Category (c) — never-emitted metrics (2 metrics)

These metric names are in panel SQL but the server has never emitted them.

| Metric | Affected panel | Likely intent |
|---|---|---|
| `mdemg_http_request_duration_seconds_p50` | mdemg-overview :: Request Latency Distribution (t0) | p50 latency — server emits p95 + p99, p50 never wired |
| `mdemg_rate_limit_rejected_total` | mdemg-overview :: Rate Limit Rejections | Per-route rate limit counter |

**Operator action**: trivial server-side additions (one Prometheus counter line each); queued as follow-up.

### Category (d) — sparse / accurate-zero (8+ panels)

Panels are correct; data legitimately doesn't exist in the audit window.

- `mdemg-ft-training` (8 panels): `ft_model_versions`, `ft_benchmarks`, `ft_training_cycles`, `benchmark_runs`, `llm_endpoint_health_events` — operator runs training elsewhere; these tables are zero on this dev TSDB. Panels are correct and will populate when training data flows here.
- `mdemg-rsic :: Action Success Rate t1` ('failed' branch) — server emits no `'failed'` status. Zero is the accurate observation.
- `mdemg-jiminy :: Effectiveness Trends` (CTE target) — uses `HAVING COUNT(*) >= 2` on `constraint_outcomes`; legitimate sparse-data.

**No action**: these panels work when their data flows; document only.

## Refresh expectations

- **metric_samples**: live (server writes every 60s on Prometheus scrape; ~3,000 rows/minute at steady state, ~50K rows/hour)
- **metrics_hourly / metrics_daily** (continuous aggregates): hourly / daily refresh per TimescaleDB CAGG policy
- **llm_interactions**: every LLM API call (synchronous insert)
- **constraint_outcomes**: every Jiminy guidance outcome (synchronous insert)
- **model_install_events** (V0021): every `mdemg model pull|verify|remove` invocation (per Sprint MODEL-DIST-001)
- **sparse_gate_metrics** (V0019): every retrieve call when sparse gate fires (buffered + 30s flush)
- **uvts_runs / uvts_results** (V0016): every `make test-uvts-{quick,full}` invocation
- **ft_*** tables: every training run / benchmark / HITL decision

## Operator playbook: detecting + fixing dashboard regressions

```bash
# Run the audit harness against all dashboards
python3 scripts/grafana_panel_audit.py

# Audit one dashboard
python3 scripts/grafana_panel_audit.py --dashboard mdemg-rsic.json

# Audit against a different space
python3 scripts/grafana_panel_audit.py --space-id myspace

# Per-target verdicts in audit_results.json:
jq '.results[] | select(.verdict=="FAIL")' docs/development/grafana-audit-001/audit_results.json

# Per-dashboard summary
jq '.audit_meta.by_dashboard' docs/development/grafana-audit-001/audit_results.json
```

For each FAIL, the `error` field captures the exact PostgreSQL error message. For each EMPTY, drill into `tables_referenced` + run the SQL with a 7-day window to differentiate sparse-data from real bugs.

## Forward-looking

- **Sprint follow-up** (queued): server-side investigation of the 4 May-7-8 emission regressions. Restore emission OR remove dashboard panels.
- **Sprint follow-up** (queued): emit `mdemg_rate_limit_rejected_total` and `mdemg_http_request_duration_seconds_p50`.
- **Coverage expansion**: 11 TSDB tables currently have ZERO dashboard panels — `sparse_gate_metrics` (V0019), `context_catalog_versions` (V0020), `model_install_events` (V0021), `retrieval_audit` (V0017), `embedding_events`, `guidance_conflicts` (V0015), `rl_training_runs/_steps` (V0013), `uvts_results/_runs` (V0016), `ft_hitl_decisions`. Many are currently sparse / never-populated on the dev TSDB; expansion gated on data accumulation OR per-table operator priority.
- **CI integration**: run `grafana_panel_audit.py` in CI nightly against a snapshot TSDB; alert on net-new FAILs.

## Metric honesty — DASHBOARD-TRUTH-001 (2026-07-03)

A follow-up to ALERT-TRUTH-001, fixing 6 measurement artifacts that made healthy subsystems read as failing. Corrected metric semantics operators should know:

- **RSIC "Cycle Success Rate"** is a *single windowed aggregate* over `mdemg_rsic_cycle_total` (completed+dry_run ÷ all terminal), NOT a per-`time_bucket` ratio. A per-bucket ratio reduced by `lastNotNull` latches 0 on a started-only bucket — the defect that showed 0% while RSIC was at 100%. **Any stat panel showing a rate must aggregate over the whole window, never bucket-then-`lastNotNull`.** A genuinely empty window shows `N/A`.
- **J17 "NLI Mean Bias" / Bias Alert** compares NLI *comprehension* vs a *compliance* heuristic — these diverge by design on `ignored` outcomes, so `ignored` samples are excluded and the alert only fires above a min-sample floor (`J17_NLI_CALIBRATION_MIN_SAMPLES`, default 50). Below the floor the gauge reads 0 / no-alert (insufficient data), gated at the source (`nli_calibration.Report()`) so every consumer agrees. **"Sidecar Requests" counts the tier-prediction shadow client only** — real NLI-call volume/latency is `mdemg_j17_nli_requests_total` / `mdemg_j17_nli_latency_ms`.
- **J17 "Protocol" health** anchors its compression sub-score to `J17_COMPRESSION_TARGET_RATIO` (default 3.0 = "excellent"), calibrated above the 30d p95 (~2.0). A freshly-restarted server reads low Protocol until its in-memory J17 window warms (cold-start transient; DH-005 confidence down-weights it).
- **J17 "Min/Avg/Max/Count Trust"** count only *significant live* sessions — within the TTL of last update AND ≥ `J17_TRUST_MIN_FEEDBACK_COUNT` (default 5) feedback events. Stale test sessions expire (TTL cleanup now actually runs; hydration preserves `last_feed_at` provenance).
- **Jiminy "Outcome Distribution" / "Outcome Trends"** read *windowed* `constraint_outcomes` counts, NOT the lifetime-cumulative multi-credit gauges (which credit a guidance_id as followed if ANY edge followed → inflated ~3× vs the honest follow rate). **"Should-Follow Follow Rate"** excludes `not_applicable` from the denominator. **"Guidance Items With Recorded Outcomes"** (formerly "Total Guidance Issued") counts distinct guidance_ids with ≥1 outcome edge, all-time — not issuance volume.

New config knobs: `J17_NLI_CALIBRATION_MIN_SAMPLES` (50), `J17_COMPRESSION_TARGET_RATIO` (3.0, >1.0 — recalibrated to 2.0 by DASHBOARD-TRUTH-002), `J17_TRUST_MIN_FEEDBACK_COUNT` (5). Sprint: `docs/development/dashboard-truth-001/`.

## DASHBOARD-TRUTH-002 (2026-07-20) — second wave

9 additional artifacts closed across the same 5 dashboards. Same forensic family: wrong-anchor thresholds, missing no-data gates, hardcoded values inappropriate for mature substrates, gauge-vs-transition-log mixups.

**RSIC scoring formula fixes** (`internal/ape/self_assess.go`):
- `scoreRetrieval`: `saturated=0.7` → `saturated=0.9` (was penalising graph maturity relative to `warm=0.9`).
- `scoreEdge`: hardcoded 0.5 entropy floor → new `RSIC_EDGE_ENTROPY_FLOOR` config (default 0.2, live-calibrated below observed healthy value 0.27; set to 0 to disable). A mature Hebbian substrate has long-tail evidence-count distribution → entropy is naturally low, was permanently -0.2'd.
- `scoreProtocol`: applies the DH-004 "no data = neutral" gate to `restoreScore` when `TicketRestoreTotal=0`. `J17_COMPRESSION_TARGET_RATIO` default recalibrated 3.0 → 2.0 (30d live p95).

**Grafana panel-shape fixes**:
- Retrieval Pipeline Health (`mdemg-rsic.json`): `gauge`/`lastNotNull` on a 3-row UNION was silently showing only the last row (rerank); converted to `bargauge` horizontal.
- Cycle Admission Ratio companion (`mdemg-rsic.json`): new stat panel `cycles/(cycles+rejections)` — normalises the raw Trigger Rejection Rate for operators (RSIC-STORM-001 designs aggressive rejection).
- Min/Max Trust Score (`mdemg-j17.json`): SQL now gates on `trust_session_count>=2` via CTE → renders "N/A" when N=1 (a distributional stat over a set of one is degenerate).
- Constraint Effectiveness (`mdemg-jiminy.json`): All-Time and Selected Range unified to `constraint_outcomes` with `classifier_source IN ('llm','tier1','explicit')` filter — was reading Neo4j cumulative (pre-fix half-credit inflated) vs TSDB windowed with different thresholds.
- LLM endpoint state (`mdemg-ft-training.json`): read `mdemg_mlx_health_state` continuous gauge from `metric_samples` (was reading the transition-log table which is empty during steady-state UP).
- Recent watchdog events (`mdemg-ft-training.json`): drop `$__timeFilter` (rare-event stream — most recent event may be >30d ago).

**Regression pin (`neural/training/tests/test_reward_functions.py::test_compute_reward_output_keys_are_exactly_spec`)** — asserts the benchmark writer never persists reward-vector keys outside the spec's declared set. Catches the class of drift that led to the historical `hidden.reclassify=0.500` display artifact.

**Architectural rules pinned**:
1. RSIC dimension scoring functions must read real signals, NOT enum-lookups of maturity/phase.
2. Shannon entropy over edge weights needs a config-tunable floor (mature graphs are low-p by design).
3. Any success-rate metric with a "no data" case must gate to neutral 1.0 when the denominator is 0 — the DH-004 pattern generalises.
4. Multi-row gauge panels must NOT use `lastNotNull` — use bargauge or split into stat panels.
5. Aggregate stats (min/max) computed over a live set must gate visibility on a set-size floor.
6. Writers must NOT persist reward-vector keys the spec doesn't declare.
7. Panels reading "state" must use the continuous gauge, NOT the transition-log.

New config knob: `RSIC_EDGE_ENTROPY_FLOOR` (default 0.2). Recalibrated: `J17_COMPRESSION_TARGET_RATIO` default 3.0 → 2.0. Sprint: `docs/development/dashboard-truth-002/`.

## Filter contract: LLM error-rate panels (GRAFANA-PANEL-FILTER-001, 2026-07-20)

Any Grafana panel that reads `llm_interactions.error` in an aggregate (`SUM/CASE/WHERE` on the `error` column, not display-only `SELECT error FROM …`) MUST filter caller-cancellation using the same clause the alert rule uses:

```sql
WHEN error IS NOT NULL AND error != '' AND error NOT LIKE 'caller_canceled:%'
```

**Why.** LLM-HEALTH-INVESTIGATION-001 E1 (`internal/llmclient/client.go`) tags `context.Canceled` / `context.DeadlineExceeded` cases with the `caller_canceled:` prefix. These represent the caller giving up (typically `retrieval.rerank_cross` when the retrieve deadline expires); the LLM was healthy, rerank fails-open, user impact is zero. Counting these as errors misclassifies fails-open recovery as failure. The filter source-of-truth is `internal/tsdb/dataset_builder.go::LLMPerformance` (which feeds the RSIC `llm_error_rate_spike` rule).

**Whitelist.** A panel may opt out with an inline SQL comment: `-- GRAFANA-PANEL-FILTER-001: intentionally unfiltered` — for forensic/debug panels that want to see everything.

**Enforcement.** `internal/grafanapin/dashboards_test.go` (`TestGrafanaPanel_LLMInteractionsErrorFilter`) walks every checked-in dashboard JSON, matches every panel target reading `llm_interactions` AND doing `error` aggregate math, and fails the build if any lacks the required filter (unless whitelisted). Runs under `go test ./...`. A contract-liveness assertion fails the test if ZERO panels match the pattern — prevents the test from becoming a silent no-op.

**Extending.** When the recorder in `internal/llmclient/client.go` starts emitting a new noise-class prefix (e.g. `sidecar_unhealthy:`), extend both:
1. `internal/grafanapin/dashboards_test.go::requiredExclusions[]` (the enforcement)
2. `internal/tsdb/dataset_builder.go::LLMPerformance` (the alert-rule filter — source of truth)

in the same PR — the two are the enforcement pair.

## References

- Sprint plan: `docs/development/grafana-audit-001/sprint_plan_grafana_audit_001.md`
- Audit findings: `docs/development/grafana-audit-001/findings.md`
- Audit results (machine-readable): `docs/development/grafana-audit-001/audit_results.json`
- Audit summary: `docs/development/grafana-audit-001/audit_summary.md`
- Sprint close: `docs/development/grafana-audit-001/post.md`
- Audit harness: `scripts/grafana_panel_audit.py`
