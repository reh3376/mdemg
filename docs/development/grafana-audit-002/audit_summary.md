# GRAFANA-AUDIT-002 — Phase 1 Correctness Audit (read-only)

Re-ran `scripts/grafana_panel_audit.py` (the GRAFANA-AUDIT-001 harness) against the **current** live TSDB (`mdemg-timescaledb-1`, space `mdemg-dev`), 2026-06-25 — ~5 weeks + ~10 metric-changing sprints after audit-001.

## Verdict: dashboards report correctly (0 broken queries)

| | audit-001 (2026-05-21) | audit-002 (2026-06-25) |
|---|---|---|
| PASS | 130 | **139** |
| EMPTY | 17 | **7** |
| FAIL | 0 | **0** |
| SKIP | 18 | 18 |
| Total | — | 164 panel-targets |

No panel query FAILs (no dead-column/renamed-metric breakage) — the per-sprint alert-rule SQL discipline (CLAUDE.md) held the dashboards too. EMPTY dropped 17→7 as the system accrued data + TSDB-CONSUME-001 removed the writerless `ft_*` panels.

## The 7 EMPTY panels (valid query, no rows in the audit window)

| Dashboard | Panel | Likely cause |
|---|---|---|
| ft-training | Phase 10 Benchmark — Aggregate Weighted Score (history) | legit-empty: no benchmark run persisted recently |
| ft-training | Entropy Health | legit-empty: no recent FT training |
| ft-training | LLM endpoint state (0=up,1=degraded,2=down) | check: should have watchdog data |
| ft-training | Recent watchdog events (last 30 in window) | check: watchdog event table |
| overview | Request Latency Distribution | **check: should have request data** |
| overview | Rate Limit Rejections | legit-empty: no rate-limit rejections (0 = healthy) |
| rsic | Watchdog Force Triggers (1h rate) | legit-empty: no force triggers in 1h |

Most are *legitimately empty* (no events of that type in-window). Two warrant a query check (Request Latency Distribution; the ft watchdog panels) — Epic 1 triages each as legit-empty (document) vs dead (fix/remove).

## The other half: metrics with NO panel

The audit only inspects *existing* panels — it can't see metrics that have no panel. A code-vs-dashboard inventory (`NewGauge/Counter/Histogram` names vs dashboard JSON) flags ~83 candidates, but that list is noisy (test-fixture metrics; `mdemg_`-prefix + Prometheus-datasource false positives). The **genuinely-new, operator-valuable gauges with no panel** (to be confirmed/deduped in Epic 1) include:

- `mdemg_jiminy_surfaced_actionable_fraction` / `_abstraction_fraction` (JIMINY-ACTIONABILITY-001 — added this session, no panel)
- `mdemg_neo4j_conversation_coverage_ratio` (HIDDEN-CHURN-001)
- `mdemg_neo4j_graph_null_weight_edges` (HIDDEN-WEIGHT-001)
- `mdemg_emergence_cycle_duration_seconds`, `mdemg_guidance_conflicts_total` (TSDB-CONSUME-001)
- jiminy operational counters (`jiminy_guide_*`, `jiminy_warm_*`, `jiminy_latest_*`)

(HIDDEN-CHURN-003's incremental-clustering signals are surfaced via existing gauges; no dedicated gauge added.)

## Phase 2 (improve) — see the sprint plan
Triage the 7 EMPTY (fix/remove/document) + add panels for the deduped new-metric set. UOBS/UOTS dashboard specs gate the result.
