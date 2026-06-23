# JIMINY-SIGNAL-001 — Sprint Post

**Date:** 2026-06-23 · branch `reh3376_dev01` · P0 (operator-bumped) · first of
two sprints from the guidance-not-reaching-agent investigation.

## Outcome
The two lying Jiminy health signals are now honest:
1. The false-positive "guidance not reaching agent" CRITICAL no longer fires on
   a healthy Jiminy.
2. The `mdemg_jiminy_follow_rate` gauge now reads the same `constraint_outcomes`
   TSDB source as the dashboard panels, so the gauge, RSIC's `GuidanceHealth`,
   and the panels all agree — replacing the ~4×-inflated 0.725.

This sprint changes only the SIGNALS. The real ~18–27% effectiveness it now
honestly reports is the actual guidance-quality problem — fixed in the follow-on
JIMINY-EFFECTIVENESS-001.

## What shipped
- **Fix 1 — false CRITICAL** (`self_reflect.go`, `self_assess.go`,
  `types_rsic.go`, `task_dispatch.go`). The `synergy_jiminy_unhealthy` insight
  was gated on `!report.JiminyHealthy`, but `JiminyHealthy` is only set inside a
  synergy block (`self_assess.go`) that is conditionally skipped — when skipped,
  the Go zero-value `false` fired the CRITICAL ~8×/day on a delivering Jiminy.
  Added `report.SynergyAssessed` (true only when the block runs), guarded the
  insight on it, and renamed the alert off the misleading "guidance not reaching
  agent" → "Jiminy Service Unavailable — catastrophic-forgetting risk" (its real
  purpose is a Jiminy-presence guard, not a delivery-quality check). 3 unit tests
  pin the guard (unassessed → no fire; assessed+down → fires; healthy → no fire).
- **Fix 2 — de-inflate the gauge** (`tsdb/dataset_builder.go`, `self_assess.go`,
  `jiminy/stats.go`, `config.go`). The Neo4j Cypher
  `count(DISTINCT CASE WHEN followed THEN guidance_id) / count(DISTINCT guidance_id)`
  double-credits multi-outcome guidance_ids (59/217 carry multiple outcomes), so
  a guidance_id lands in the "followed" numerator if ANY edge is followed →
  0.725. New `DatasetProvider.GuidanceEffectiveness` reads `constraint_outcomes`
  with the panels' exact math (followed=1.0, partial=0.5) over a config window
  (`RSIC_GUIDANCE_EFFECTIVENESS_WINDOW_HOURS`, default 168=7d). The assessor
  overrides `js.FollowRate` with it before scoring + publishing, so the gauge AND
  RSIC `GuidanceHealth` AND the panels agree. The Neo4j path is retained as a
  documented fallback.

## Live Tier 3 — caught two bugs the unit tests couldn't (its whole point)
- **Fix 1 alert:** after restart, **0 `jiminy CRITICAL` fires** across multiple
  RSIC cycles (the false alarm previously fired ~8×/day). But the FIRST restart
  still fired it — surfacing **Bug A**: `ReadSynergyMetrics` set `JiminyHealthy`
  only `if jiminyCheck != nil`, so a nil check left the zero-value `false`
  (= "down") and the guarded insight still fired. Fixed by defaulting
  `JiminyHealthy=true` (real outages covered by `/healthz`+watchdog).
- **Fix 2 gauge:** the gauge stayed inflated at **0.732** even though the
  assessor override ran correctly (diagnostic: `tsdb_rate=0.09, n=22, err=nil`)
  — surfacing **Bug B**: the gauge has **two publishers**
  (`self_assess.go` + `live_collectors.go`'s 15s Prometheus collector); the live
  collector republished the un-overridden Neo4j rate, overwriting it. Fixed by
  extracting `Assessor.applyHonestFollowRate` and calling it from both paths.
  **Verified live: gauge now 0.235, matching the TSDB 7d panel value (0.2353).**

## Disclosure
Making the gauge honest **lowers** RSIC's `GuidanceHealth` dimension (0.725 →
the true ~0.1–0.27) — that is correct: the inflated gauge was masking the real
guidance-effectiveness problem in the very health score meant to catch it.

## Carried forward
- **JIMINY-EFFECTIVENESS-001** (the real fix): stop surfacing low-confidence/
  empty/irrelevant guidance + don't decay trust on `Ignored`-of-empty guidance —
  the J17 T1-promotion unblocker, now validated against these honest signals.
- Follow-up: de-inflate or retire the Neo4j `stats.go` follow-rate fallback once
  the TSDB path is proven.

## Documents Accessed
The two subagent root-cause reports; `internal/ape/{self_reflect,self_assess,
task_dispatch,types_rsic}.go`; `internal/jiminy/stats.go`;
`internal/tsdb/dataset_builder.go`; `internal/config/config.go`; live TSDB
`constraint_outcomes`; the mdemg-jiminy Grafana dashboard; `/healthz` +
`~/.mdemg/alerts/current.json`.
