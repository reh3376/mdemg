# GRAFANA-AUDIT-002 — Sprint Post

## Summary

Re-audited the 8 Grafana dashboards (~5 weeks + ~10 metric-changing sprints after GRAFANA-AUDIT-001) and improved them. **Phase 1:** dashboards report correctly — 139 PASS / 7 EMPTY / **0 FAIL** / 18 SKIP (no broken queries; EMPTY down from audit-001's 17 as data accrued + writerless `ft_*` panels were removed by TSDB-CONSUME-001). **Phase 2:** fixed the one dead panel target + added panels for new operator-valuable gauges → **143 PASS / 6 EMPTY / 0 FAIL**.

## Changes

- **Fixed:** `overview/Request Latency Distribution` removed its dead `_p50` target (only `_p95`/`_p99` are emitted; `_p50` never written → permanently-blank series). Panel renders p95/p99.
- **Added** (all PASS, data confirmed via re-audit):
  - `mdemg-jiminy` → **Surfaced Actionable Fraction** (`mdemg_jiminy_surfaced_actionable_fraction` + `_abstraction_fraction`).
  - `mdemg-neo4j` → **Null-Weight Abstraction Edges** (`mdemg_neo4j_graph_null_weight_edges`).
  - `mdemg-neo4j` → **Conversation Coverage Ratio** (`mdemg_neo4j_conversation_coverage_ratio`).
- **Documented** the 6 remaining EMPTY as legit-empty (no events in-window: benchmark >24h, ft watchdog, 0 rate-limit rejections, 0 force-triggers) except one no-producer gap: `ft-training/Entropy Health` reads `llm_interactions.quality`, which has no writer (metric-instrumentation follow-up, not a dashboard fix).

## Testing

- **Phase 1 + post re-audit:** `scripts/grafana_panel_audit.py` against live TSDB — `audit_results.json` (pre) + `audit_results_post.json` (post). 0 FAIL both runs; the 3 new panels confirmed PASS by name.
- **JSON validity:** all edited dashboards re-parsed.
- No UOTS/UOBS panel-count spec → no CI drift from additions.

## Findings carried forward

- **`llm_interactions.quality` has no writer** — the Entropy Health panel can't populate until something computes/writes per-interaction quality. Producer task, not dashboard.
- The standing **Neo4j High CPU / LLM error-rate** alerts are real ongoing conditions (slowed test runs this session) — pre-existing, separate from this sprint.

## Documents Accessed
- `scripts/grafana_panel_audit.py`, `docs/development/grafana-audit-002/{audit_results,audit_results_post,audit_summary}`
- `deploy/docker/grafana/dashboards/{mdemg-overview,mdemg-jiminy,mdemg-neo4j}.json`
- Live `mdemg-timescaledb-1` `metric_samples` + the empty-panel source tables
