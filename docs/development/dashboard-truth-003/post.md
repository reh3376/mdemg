# DASHBOARD-TRUTH-003 — Sprint Post

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Parent trigger:** Operator dashboard-metrics flag → 7-agent deep-dive workflow
(`wf_30b994e6-1cb`) → synthesizer scoping. Verdict: 5 MEASUREMENT_ARTIFACT +
2 BOTH + 0 REAL_LOW — same class distribution as DASHBOARD-TRUTH-001/-002.

## Verdict

**Shipped.** 4 mdemg-jiminy.json panels honest, 1 hardcoded RSIC pair extracted
to config, staged embed synced, all tests green, lint clean.

## What shipped

### Dashboard honesty (mdemg-jiminy.json)

- **Guidance Health** panel — added description referencing DASHBOARD-TRUTH-003
  + recalibrated thresholds red<0.20 / yellow≥0.20 / green≥0.28 (was
  red<0.4 / green≥0.7 — chronically red on the honest ~0.14 steady state).
- **Follow Rate** panel — description + thresholds red null / yellow≥0.05 /
  green≥0.15 aligned with the honest steady-state band.
- **Actionable Compliance Rate** — renamed to "Actionable Compliance Rate
  (expected 10-13% by design)" with prominent ab_verdict.md reference in
  description; 4-band recalibration (red<0.05 / yellow 0.05-0.10 / light-green
  0.10-0.25 / green≥0.25). ⚠️ The 10-13% is the shipped steady state per
  JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 recalibration — this panel MUST
  NOT read green≥0.90 or it inverts the sprint's calibration.
- **Effectiveness Trends** — deleted refB (`mdemg_jiminy_constraint_effectiveness`
  gauge that contradicted the honest windowed refD series; DASHBOARD-TRUTH-001
  had already labeled it dishonest, but the trend panel still plotted it).
- **Auto-compact AVG** — copy-fixed the SQL from the j17 twin panel: distinct
  compact events, last 10 lifetime, no window filter (rare-event stream pattern);
  calc `mean` → `lastNotNull`.

### RSIC floor extraction (self_reflect.go)

- Pattern 9 `low_guidance_follow_rate` and pattern 15 `guidance_confidence_drift`
  no longer carry hardcoded 0.5/0.7 floors. Extracted to:
  - `RSIC_GUIDANCE_HEALTH_FOLLOW_FLOOR` — default **0.20** (recalibrated from
    legacy 0.5 that fired chronically on the honest ~0.14 steady state)
  - `RSIC_GUIDANCE_HEALTH_DRIFT_FLOOR` — default **0.25** (recalibrated from
    legacy 0.7)
- Floor=0 disables the pattern entirely (opt-out).
- Insight description now includes the actual floor value (was: "below 50%"
  regardless of what the operator had configured).

### Config wiring

- `internal/config/config.go`: two new float64 fields on `Config` struct with
  comment linking back to DASHBOARD-TRUTH-003 rationale.
- `FromEnv` block reads via `atof` with the new defaults.
- Struct-literal assembly threads them through.

### Test coverage (self_reflect_test.go)

- Existing `TestReflect_GuidanceConfidenceDrift` updated to explicitly set
  `RSICGuidanceHealthDriftFloor=0.7` (legacy floor); new floor=0 opt-out case.
- New `TestReflect_LowGuidanceFollowRate_FloorConfigurable` with 3 cases:
  fires below default 0.20, quiet at 0.25, legacy 0.5 semantics still work
  when explicitly configured.
- Existing `TestReflect_LowGuidanceFollowRate` updated to set both floors
  to legacy 0.5/0.7 (previously implicit via zero-value; new gate `floor > 0`
  means zero-value = disabled).

### Staged mirror

- `internal/cli/grafana_templates/staged/dashboards/{mdemg-jiminy,mdemg-j17}.json`
  synced via `make sync-grafana-embed`. `make verify-grafana-embed` clean.

## Not shipped (intentional)

- **J17 Min/Max Trust panels** — already correctly render "N/A" via
  DASHBOARD-TRUTH-002 E6 gating (N<2 → NULL). Descriptions comprehensive.
  Adding a "Solo Session Trust" companion panel was scoped but rejected —
  adds grid-shift fragility for a rare N=1 edge case; the N/A rendering
  is by-design correct behavior, and the description explains it.
- **`mdemg_jiminy_constraint_effectiveness` gauge retirement in Go code**
  (internal/jiminy/stats.go:113, internal/ape/live_constants.go:298,
  self_assess.go:846) — operator disposition was "Split to follow-up." The
  gauge is now unread by any panel but still emitted. Future sprint can
  retire the emit sites; not load-bearing for this sprint's shipped state.

## Live Tier-3 (mdemg-dev — pending operator run)

- Restart mdemg native binary
- Reload Grafana (Ctrl-R on dashboard)
- Verify: 5 panel changes render correctly; env override
  `RSIC_GUIDANCE_HEALTH_FOLLOW_FLOOR=0` disables pattern 9 (check RSIC
  self-reflection insight stream for absence).

## Rules pinned (CLAUDE.md update)

⚠️ **RSIC pattern thresholds MUST be config-extracted, not hardcoded, when
they gate on a metric that has a known-honest steady state**. The 0.5 and
0.7 GuidanceHealth floors predated JIMINY-CORPUS-001's recalibration and
fired chronically on healthy substrate for months. General rule: whenever
a sprint recalibrates a metric's honest steady state, audit `self_reflect.go`
for hardcoded floors gating on that metric.

⚠️ **Panel titles carrying the design-intent number ("expected 10-13% by
design") beat prose descriptions alone** — operators triage by scanning
panel titles; embedding the expected range in the title makes the honest
state legible at a glance. This is the concrete follow-on to DASHBOARD-TRUTH-002
E1/E2's description-tightening lesson.

## Follow-ups disclosed

1. **Retire `mdemg_jiminy_constraint_effectiveness` gauge emit sites** —
   dashboard no longer reads it (this sprint deleted the last consumer);
   3 Go emit sites can be removed. Not urgent (gauge is cheap, no
   incorrect data flowing anywhere it's read).
2. **J17 Solo Session Trust companion panel** — considered + rejected here
   but could ship if the N=1 case becomes chronic (mdemg-dev has been
   N=1 for weeks; a "here's the one session's trust score with no
   distributional stats" panel would be honest UI, just extra JSON to
   maintain).

## Documents Accessed

- Workflow output at `.../subagents/workflows/wf_30b994e6-1cb/`
- CLAUDE.md DASHBOARD-TRUTH-001/-002/JIMINY-CORPUS-001/JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 sections
- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` (5 panel edits)
- `deploy/docker/grafana/dashboards/mdemg-j17.json` (no-op — Min/Max Trust intentionally untouched)
- `internal/ape/self_reflect.go` lines 163-187 (patterns 9 + 15)
- `internal/ape/self_reflect_test.go` (3 tests updated/added)
- `internal/config/config.go` (RSIC block + FromEnv + struct literal)
- `internal/cli/grafana_templates/staged/dashboards/` (synced via Makefile)
