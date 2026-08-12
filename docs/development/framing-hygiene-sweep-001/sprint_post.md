# FRAMING-HYGIENE-SWEEP-001 — Sprint Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Deferred from:** JIMINY-CEILING-BREAK-2 §"Doc + framing hygiene cleanup"

## Problem

The JIMINY-CEILING-BREAK-2 master arc doc explicitly rejects the "~12% honest by design" framing and pins as a cross-cutting arc rule that follow-rate framing MUST be trajectory language ("current N%, target M%, owning arc X"), NEVER "by design" language. But shipped artifacts from before the arc still used the legacy framing:

- 4 Grafana panels on `mdemg-jiminy.json` with titles like `"Follow Rate (honest 15-25% by design)"` and `"Actionable Compliance Rate (expected 10-13% by design)"`
- Panel descriptions with "Post-fix honest steady state is ~11-12%" and "green>=0.10 = normal operating range"
- JIMINY-HEURISTIC-DEFAULT-001 + JIMINY-FOLLOW-RATE-REMEASURE-001 sprint posts (5 files) with "not urgent", "honest steady state" framing

Operators reading these fresh in 2026-08 would infer that the ~12% follow rate is the target, not the arc's STARTING point. This sprint closes that gap.

## Shipped

**Grafana panel titles reframed** (4 titles on `mdemg-jiminy.json`):

| Before | After |
|---|---|
| `Follow Rate (honest ~12% post-heuristic-fix)` | `Follow Rate (arc target ≥80%; see JIMINY-CEILING-BREAK-2)` |
| `Constraint Eff. All-Time (honest ~15% by design)` | `Constraint Eff. All-Time (arc target ≥80%; see JIMINY-CEILING-BREAK-2)` |
| `Constraint Eff. Selected Range (honest ~15% by design)` | `Constraint Eff. Selected Range (arc target ≥80%; see JIMINY-CEILING-BREAK-2)` |
| `Actionable Compliance Rate (expected 10-13% by design)` | `Actionable Compliance Rate (arc target ≥80%; see JIMINY-CEILING-BREAK-2)` |

**Panel descriptions** on those 4 panels: prepended a TRAJECTORY banner explaining the current value is arc-in-progress with target ≥80%, listing Phases 1-4a + arc-adjacent purges shipped 2026-08-11/12, referencing the 2026-08-19 passive re-check and the Phase 4b + 5 remaining levers, and warning "do NOT normalize the current value as 'by design'." Historical prose preserved below the banner as context. Also replaced 2 specific substrings ("post-fix honest steady state is ~11-12%" → "post-fix honest baseline is ~11-12% (this was the arc's STARTING point, not its target)"; "green>=0.10 = normal operating range" → "green>=0.10 = pre-arc-shipping normal; target ≥0.80 per JIMINY-CEILING-BREAK-2").

**Staged embed synced** via `make sync-grafana-embed`; `make verify-grafana-embed` clean (no drift).

**5 pre-arc sprint docs annotated** with a trajectory banner at the top pointing to `docs/development/jiminy-ceiling-break-2/README.md` as the successor arc and warning "do NOT act on the 'not urgent' / 'by design' conclusions in the body":
- `docs/development/jiminy-follow-rate-remeasure-001/{sprint_plan,post,verdict}.md`
- `docs/development/jiminy-heuristic-default-001/{sprint_plan,post}.md`

Historical prose preserved below the banner. No content-truth mutation — those sprint records are correct as historical artifacts; the annotation just flags that their VERDICTS are superseded.

## What was NOT changed (deliberate)

- Older sprint posts that REFERENCE the legacy framing as historical evidence (JIMINY-CORPUS-003 post + sprint_plan, JIMINY-TRACKER-TTL-001 post, DASHBOARD-METRICS-DEEP-DIVE-001 post, DASHBOARD-TRUTH-003 post) — these are historical records + do not present verdicts. Annotating them would be noise.
- Feature docs with legitimate uses of "by design" for actual design descriptions (e.g. `jiminy-governance.md` "Write/Edit gate is fail-open by design"; `hitl-review.md` "reversible by design") — not follow-rate framing.
- `rsic.json`'s "5-10/min rejections by design" description — describes RSIC-STORM-001's admission-reservation behavior, not follow rate.
- CLAUDE.md pins for JIMINY-HEURISTIC-DEFAULT-001 + JIMINY-FOLLOW-RATE-REMEASURE-001 — the master pin for JIMINY-CEILING-BREAK-2 (added in JIMINY-CORPUS-003's commit) already tags those as "using the now-rejected framing" and this sprint doesn't re-annotate them (the arc pin is the load-bearing reference).

## One arch rule pinned (CLAUDE.md)

**Trajectory-annotation banners on superseded sprint docs preserve history AND close the framing gap.** When an arc supersedes a prior sprint's framing conclusion, the correct fix is (a) add a bold banner at the TOP of the superseded doc pointing to the successor arc + naming the specific framing being rejected + preserving historical prose below, (b) NEVER rewrite the historical body (loses institutional memory of how the framing evolved), (c) sync any operator-facing artifacts (panels, alert-comments, CLI help) to the new framing so a fresh operator sees the current truth without needing to trace history. Only the OPERATOR-FACING surface gets destructively updated; the historical record gets non-destructive annotation.

## Live Tier-3

- `make sync-grafana-embed` clean (16 files); `make verify-grafana-embed` clean
- 4 panels reframed; descriptions banner-prepended
- 5 sprint docs annotated with trajectory banners
- No code changes — pure docs + JSON

## Follow-ups disclosed

- **Panel bands recalibration** — the panel thresholds/bands themselves (color-coding red/yellow/green) still map to the ~12% steady state. When arc reaches ~30% (post-2026-08-19 re-check), recalibrate bands so red < 20%, yellow 20-50%, green ≥ 50%. When arc reaches ~50%, recalibrate again. Not urgent until the underlying value moves.
- **Alert-floor recalibration** — `JIMINY_FOLLOW_RATE_ALERT_FLOOR` default 0.05 is still appropriate ("catches genuine collapse" is orthogonal to "target ≥80%"); revisit if the floor starts flapping.

## Documents Accessed

- `docs/development/jiminy-ceiling-break-2/README.md` — the cross-cutting §"Doc + framing hygiene cleanup" that named this deferred sweep
- `deploy/docker/grafana/dashboards/mdemg-jiminy.json` — 4 panels reframed
- `internal/cli/grafana_templates/staged/dashboards/mdemg-jiminy.json` — synced
- 5 pre-arc sprint docs annotated
- CLAUDE.md pins: JIMINY-CEILING-BREAK-2 (source-of-truth for the "trajectory language" rule)
