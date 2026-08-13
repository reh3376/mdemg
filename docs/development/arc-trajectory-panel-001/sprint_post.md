# ARC-TRAJECTORY-PANEL-001 — Sprint Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Deferred from:** JIMINY-CEILING-BREAK-2 arc — visualization gap

## Problem

The JIMINY-CEILING-BREAK-2 arc shipped its Phase 1–3 substrate changes on 2026-08-12 with a baseline snapshot at 13.25% actionable follow rate + a target of ≥80%. The T+20h / T+72h / T+168h passive re-check windows fire on 2026-08-13, 2026-08-15, and 2026-08-19 respectively. Between shipping and the first re-check, operators inspecting Grafana see the shipped stat panels titled `... (arc target ≥80%; see JIMINY-CEILING-BREAK-2)` but have no live-updating trajectory visualization — no way to spot mid-arc regressions or watch the lift live. The existing "Follow Rate" stat panel shows a single point-in-time number, not the trajectory shape.

## Arc-safety

**100% safe for the JIMINY-CEILING-BREAK-2 measurement window** (through 2026-08-19):
- Pure Grafana dashboard JSON change; zero Go code touched
- Zero substrate impact — panel READS from `constraint_outcomes`, never writes
- Zero LLM cost — SQL-only, runs on the shipped TimescaleDB datasource
- Panel is additive (appended at y=51 below existing layout); does not disturb any existing panel

## Shipped

**Dashboard** (`deploy/docker/grafana/dashboards/mdemg-jiminy.json`):
- New row `JIMINY-CEILING-BREAK-2 Arc Trajectory` at y=51
- New timeseries panel `Actionable Follow Rate — Arc Trajectory (baseline 13.25% → target ≥80%)` at y=52, h=10, w=24
- Two data series:
  - `Actionable Follow Rate (%)` (blue solid) — `time_bucket('1 hour', time)` over `constraint_outcomes`, filtered to `guidance_type IN ('constraint','correction')` and `outcome_type IN ('followed','partial_compliance','ignored','contradicted')`, computed as `SUM(followed=1.0 + partial=0.5) / COUNT(*) × 100`
  - `Baseline (2026-08-12 18:17 UTC)` (orange dashed) — constant 13.25 across the range for visual anchor
- Threshold band via `fieldConfig.thresholds` (line mode): red <20 → orange <40 → yellow <80 → green ≥80. The green band starts at the arc target so an operator glancing at the panel immediately sees "how far to green"
- Y-axis clamped 0–100 (percent unit) so a lift from 13 → 45 doesn't visually vanish under auto-scaling
- Legend calcs: `lastNotNull, min, max, mean` — the mean row is the summary number an operator wants at T+72h
- Description embeds the shipping baseline (13.25% at 2026-08-12 18:17 UTC), the commit ref, the re-check windows, and an explicit "do NOT normalize any current-value framing as 'by design'" line — the arc's framing hygiene rule pinned into the panel itself so a future operator reading it inherits the constraint

**Staged embed sync:**
- `make sync-grafana-embed` → 16 files, no diff on siblings
- `make verify-grafana-embed` → OK, no drift
- `go test ./internal/grafanapin/...` → pass

## Live Tier-3 (mdemg-dev, 2026-08-12)

- `curl POST /api/admin/provisioning/dashboards/reload` → `{"message":"Dashboards config reloaded"}`
- `curl GET /api/dashboards/uid/mdemg-jiminy` → 22 panels (was 20), 6 arc-related (4 pre-existing reframed stats + new row + new timeseries)
- SQL query verified live against `mdemg-dev` — returns real hour-bucket data:
  - 2026-08-13 01:00 UTC → 0.00% (3 rows)
  - 2026-08-13 00:00 UTC → 9.09% (11 rows)
  - 2026-08-12 19:00 UTC → 12.70% (63 rows)
  - 2026-08-12 18:00 UTC → 11.76% (17 rows) ← baseline moment
  - 2026-08-12 16:00 UTC → 5.88% (34 rows)
- Datasource UID matches sibling panels (`timescaledb` — the shipped provisioning UID, NOT `${DS_TIMESCALEDB}` env-var form). Fixed via `fix_uid.py` after initial write

## Two arch rules pinned (CLAUDE.md)

1. **When shipping a new Grafana panel, ALWAYS verify datasource UID against a sibling panel in the same dashboard file** — the `${DS_ENVVAR}` form only works in dashboards imported from Grafana.com; provisioned dashboards use the literal UID from `deploy/docker/grafana/datasources/*.yml` (in this case `timescaledb`). A wrong UID silently renders "Data source not found" — the panel appears in the dashboard but shows no data, which is the second-worst failure mode (worst is falsely showing 0%).

2. **Trajectory panels for named arcs MUST embed baseline + target + re-check windows in the panel description** — an operator opening Grafana six months from now won't remember the arc's shipping moment. The description is the panel's persistent context; use it. Also embed the anti-normalization rule directly into the description text — the shipping sprint's framing hygiene must survive the arc's completion. Same pattern shape as DASHBOARD-TRUTH-002 E1/E2 (title carries design-intent number) but extended to panel description for arc-context.

## Follow-ups disclosed

- **Second-derivative panel** — if the T+72h re-check shows a lift trajectory, add a second panel plotting the 24h-window mean-of-mean to smooth out the hour-bucket noise. Ship only if operator wants trend clarity beyond the raw hourly signal.
- **Retire the orange baseline series post-arc** — once the arc closes (target reached, or explicitly declared complete), the baseline reference becomes historical noise. Convert to an annotation instead so it collapses out of the legend. Do NOT auto-remove; the arc's post-mortem needs the visual reference.

## Documents Accessed

- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` (structure inspection + panel append)
- `docs/development/jiminy-ceiling-break-2/README.md` (baseline snapshot + re-check window context)
- `docs/development/framing-hygiene-sweep-001/sprint_post.md` (the "arc target ≥80%" title reframe pattern I mirrored in the trajectory panel's own title + description)
- `internal/grafanapin/dashboards_test.go` (pin test contract I had to preserve)
- Live: TimescaleDB SQL query verification, Grafana provisioning reload endpoint, `/api/dashboards/uid/mdemg-jiminy`
- CLAUDE.md pins: DASHBOARD-TRUTH-002 (title-embedded design intent), FRAMING-HYGIENE-SWEEP-001 (arc-target framing), JIMINY-CEILING-BREAK-2 (baseline + re-check windows)
