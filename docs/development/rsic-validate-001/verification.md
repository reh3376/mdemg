# RSIC-VALIDATE-001 — Verification (Tiers 1–3)

**Date:** 2026-06-11 · **Stack:** native serve (restarted on the fixed
binary) + Docker Neo4j/TSDB. Space `mdemg-dev`.

## Tier 1
- `reportMetricsMap` covers every report-resolvable criteria key (test pins
  the list incl. the `total_edges` alias); mutating-action registry tests.
- The legacy test asserting the vacuous pass (`CriteriaMet=true` on missing
  data for a mutating action) was the bug pinned as the contract — rewritten
  to the fail-closed contract + an advisory companion for observational
  actions. ape/jiminy/api suites green; lint 0.

## Tier 2
Both rewritten executor statements EXPLAIN-validated live. Honest scoping
note: 0 corrections exist in the current 7-day window, so old and new
tombstone scopes are both 0 RIGHT NOW — the hazard was conditional (any
future correction re-armed the old query against the full pool of thousands
of unrelated observations; the new query bounds it to correction-linked
nodes: same session OR 1-hop CO_ACTIVATED_WITH).

## Tier 3 — a real RSIC cycle through the fixed pipeline
Cycle `rsic-micro-01c5ae30` (7 actions executed):
- `metrics_before`: **19 keys** / `metrics_after`: **19 keys** (previously
  10 vs 7, with only 2 intersecting the criteria vocabulary).
- `criteria_detail` — REAL evaluations, zero vacuous skips:
  `avg_edge_weight_delta → not_met (delta=0.0000, op=gt)`,
  `edges_below_threshold_delta → not_met`, `total_edges_delta → not_met`,
  `volatile_count_delta → met`.
- **`criteria_met: false`** — the honest verdict. Before this sprint the
  same cycle recorded success with zero evidence; the criteria-driven
  rollback path is now reachable.
- Counter-free calibration: RSIC confidence adjustments no longer write
  `total_surfaced/followed/ignored` (code path verified; the counters move
  only on real guidance feedback).

## Notes
- The cycle HTTP call exceeds the client window (~6 min server-side) —
  outcomes read from `/v1/self-improve/history`; same long-request class
  as consolidate (config-driven budgets), noted for the hygiene backlog.
- Expect `criteria_met=false` to be COMMON now: that is the honest steady
  state until actions actually move their metrics; calibration scores will
  recalibrate accordingly.
