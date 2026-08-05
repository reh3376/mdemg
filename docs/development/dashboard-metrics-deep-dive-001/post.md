# DASHBOARD-METRICS-DEEP-DIVE-001 — Sprint Post

**Date:** 2026-08-05 | **Branch:** `reh3376_dev01`
**Trigger:** Operator-flagged 2026-08-01 (Q4-disclosed follow-up). Two clusters of 7 total metrics "reading much lower than effective" across `mdemg-jiminy` + `mdemg-j17` dashboards. Same triage pattern as DASHBOARD-TRUTH-001 (6/8 flagged were artifacts) + DASHBOARD-TRUTH-002 (9/14 were artifacts).

## Verdict

Shipped. All 7 metrics triaged. **6 of 7 are MEASUREMENT_ARTIFACT (perception mismatches — panel shows correct value; operator eye reads it as low because the design-intent isn't visible without hovering description)**. 1 is "BOTH" (measurement correct AND on low edge of honest range — LEVER-C-TIGHTEN-001 is the active lift attempt). 0 REAL_LOW. Matches the DASHBOARD-TRUTH-001/002 pattern (~75-90% of operator-flagged low metrics are artifacts).

Fix: mirrors DASHBOARD-TRUTH-002's title-embedded-design-intent pattern. Embed the design-intent range directly in the panel title so the reader doesn't need to hover to know whether 0.14 is a healthy 14% (which it is) or a broken 14% (which it isn't).

## Triage table

| Metric | Live 24h | Design | Class | Fix |
|---|---|---|---|---|
| **j17: Min Trust** | N/A | needs ≥2 sessions to be non-degenerate | ARTIFACT | Title + noValue explain the gate |
| **j17: Max Trust** | N/A | same | ARTIFACT | same |
| **jiminy: Guidance Health** | 0.3186 | green ≥0.28 target | ARTIFACT (in green zone) | Title embeds target |
| **jiminy: Follow Rate** | 0.1398 | honest 15-25% | **BOTH** (low edge of honest range) | Title embeds honest ceiling |
| **jiminy: Constraint Eff. All-Time** | 0.1452 | matches raw follow ~14% | ARTIFACT | Title embeds honest ~15% |
| **jiminy: Constraint Eff. Selected Range** | 0.1452 | same | ARTIFACT | same |
| **jiminy: Actionable Compliance** | 0.1460 | 10-13% by design | ARTIFACT (at TOP of range) | Already fixed by DASHBOARD-TRUTH-003 |
| **jiminy: Auto-compact AVG** | 5459m (~91h) | LONGER=BETTER | ARTIFACT (91h is healthy) | Title embeds LONGER=BETTER |

## What shipped

### `deploy/docker/grafana/dashboards/mdemg-jiminy.json` — 5 panel-title edits
- `Guidance Health` → `Guidance Health (green ≥0.28 target)`
- `Follow Rate` → `Follow Rate (honest 15-25% by design)`
- `Constraint Eff. (All-Time)` → `Constraint Eff. All-Time (honest ~15% by design)`
- `Constraint Eff. (Selected Range)` → `Constraint Eff. Selected Range (honest ~15% by design)`
- `Auto-compact AVG` → `Auto-compact AVG interval (LONGER=BETTER)`

### `deploy/docker/grafana/dashboards/mdemg-j17.json` — 2 panel-title + 2 noValue edits
- `Min Trust Score` → `Min Trust Score (needs ≥2 sessions)`
- `Max Trust Score` → `Max Trust Score (needs ≥2 sessions)`
- `noValue: "N/A"` → `noValue: "N/A (n<2)"` on BOTH (via `replace_all`, hit both panels)

### Grafana embed sync + pin test
`make sync-grafana-embed` (16 files); `make verify-grafana-embed` clean; `internal/grafanapin` test green.

## Live Tier-3 (mdemg-dev)

Provisioning reload triggered:
```
curl -X POST /api/admin/provisioning/dashboards/reload
→ {"message":"Dashboards config reloaded"}
```

Post-reload panel titles verified via `/api/dashboards/uid/<id>`:
```
=== mdemg-jiminy ===
  ✓ Guidance Health (green ≥0.28 target)
  ✓ Follow Rate (honest 15-25% by design)
  ✓ Constraint Eff. All-Time (honest ~15% by design)
  ✓ Constraint Eff. Selected Range (honest ~15% by design)
  ✓ Auto-compact AVG interval (LONGER=BETTER)
=== mdemg-j17 ===
  ✓ Min Trust Score (needs ≥2 sessions)
  ✓ Max Trust Score (needs ≥2 sessions)
```

All 7 panels retitled + rendered. Existing SQL, thresholds, and gates unchanged — only display strings.

## Rules pinned

⚠️ **When an operator flags a metric as "reading much lower than effective," verify the value against the panel's DESIGN-INTENT threshold before assuming REAL_LOW** — the DASHBOARD-TRUTH sprint family has now shown 3 times (001: 6/8, 002: 9/14, 003 continued the pattern, 001-this-sprint: 6/7) that most operator-flagged low metrics are artifacts. The value is correct; the design-intent isn't visible. Triage this class BEFORE spawning a substrate-quality investigation.

⚠️ **Panel titles are the operator's fastest visibility surface — hover-only descriptions are second-class** — a title that reads `Follow Rate` alongside "0.14" reads as broken; a title that reads `Follow Rate (honest 15-25% by design)` alongside "0.14" reads as low-end-of-target. Zero infrastructure cost, high operator-trust payoff. When shipping a metric with a non-obvious design range, embed the range in the title in the SAME commit that ships the metric — not as a follow-up sprint. Follow-up sprints exist because we didn't do this originally.

⚠️ **`noValue` on a gated stat panel MUST name the gate**, not just render as blank/N-A — "N/A" reads as broken; "N/A (n<2)" reads as gated-by-design. Same principle as the title fix at the empty-value surface.

## Not shipped (intentional)

- **Follow Rate lift** — the ONE metric where the honest steady state IS at the low edge of the design range. That's LEVER-C-TIGHTEN-001's job (in-flight, T+4d looking favorable). This sprint is NOT a substrate-quality intervention; it's a display-clarity sprint.
- **Constraint Eff. gauge deprecation** — DASHBOARD-TRUTH-003 E4 already deleted the dashboard READER of `mdemg_jiminy_constraint_effectiveness` (the dishonest lifetime-cumulative gauge). The WRITER still emits it 1/min (1439 samples/24h) with zero readers. Retiring the gauge emit sites is a small separate sprint (`METRICS-DEPRECATE-JIMINY-CONSTRAINT-EFF-001`).
- **Cluster 1 fix beyond title/noValue** — could replace the panel with a stack-view of "Active Trust Sessions" that renders per-session trust values directly (no min/max at all). Deferred — the current N/A + title/noValue is honest and cheap.

## Follow-ups disclosed

- **METRICS-DEPRECATE-JIMINY-CONSTRAINT-EFF-001** — retire `mdemg_jiminy_constraint_effectiveness` writer sites (dashboard reader was deleted DASHBOARD-TRUTH-003 E4; per DORMANT-METRICS-CLEANUP-001 discipline, no dashboards means no writer). ~1h.
- **Design-intent-in-title lint** — a small `grafanapin` test that warns when a stat panel's title doesn't reference either an alert threshold or a design-intent range. Would prevent regression of THIS sprint's fix.

## Rollback

Single-commit revert. Zero-risk change (only display strings; no SQL, no thresholds, no data).

## Documents Accessed

- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` (5 panel titles edited)
- `deploy/docker/grafana/dashboards/mdemg-j17.json` (2 panel titles + 2 noValue values edited)
- Live TSDB metric snapshots on mdemg-dev (7 metric triage cross-checks)
- `docs/development/roadmap/ROADMAP_2026Q4.md` §5 (the operator's original flag)
- DASHBOARD-TRUTH-001 + DASHBOARD-TRUTH-002 + DASHBOARD-TRUTH-003 posts (the shipped triage-pattern sibling arc)
- `Makefile:520` (sync-grafana-embed, verify-grafana-embed)
- `internal/grafanapin/` (pin test — green post-edit)
