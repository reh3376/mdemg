# MAINT-LIVE-001 — Sprint Close

**Date:** 2026-06-11 · **Branch:** `reh3376_dev01` · **Roadmap:** Q3 Phase 1, rank #4

## What shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Plan + safety verification (tombstone-not-delete, protections) | `1a95850` |
| 1 | Plist passes `--dry-run=false` (schedule live; CLI default stays safe) + `dry_run` job-event metadata | `d30c4b5` |
| 2 | `maintenance_no_live_run` evaluator rule (born firing — a true positive) | `b325c5e` |
| 3 | `mdemg upgrade` (darwin) refreshes installed LaunchAgents + hooks; substitutions single-sourced | `0775916` |
| 4a | `--exclude-role-types` / `PRUNE_EXCLUDE_ROLE_TYPES` — context-dependent orphan policy (operator-decided exclusions baked into the schedule) | `da485ad` |
| — | **Live-smoke fix (own commit):** orphan sweeps' batched deletes require implicit transactions | `d9d8c0e` |
| 4b–5 | First-ever live run + verification + docs | (this) |

## The headline

The first live execution in the maintenance path's history:
1. **Caught a bug no test could reach** — the batched delete statement only
   executes outside dry-run, and it was illegal Cypher-in-context
   (CALL-IN-TRANSACTIONS inside ExecuteWrite). Fixed, re-run, succeeded.
2. **Proved the NOSILENT chain under real failure** — dispatcher alert +
   evaluator rule + hook delivery all fired for the genuine failure.
3. **Deleted 20,236 orphan SymbolNodes** (the Memory-Bloat bulk;
   re-derivable code artifacts), with the operator's context-dependent
   exclusions protecting governance constraints + conversation history.
4. **Silenced its own liveness rule** — `maintenance_no_live_run` was born
   firing against real data (1 prior run ever, itself a failure) and went
   quiet on the first genuine execution.

## Operator decision honored

"Orphan disposition is context-dependent" — the uniform degree/age rule
now composes with `--exclude-role-types`; the shipped schedule excludes
`constraint,conversation_observation`. The census + rationale live in
`verification.md` and the plist comment.

## Follow-ups

- Session-aware prune policy (distinguish `claude-core` history from
  `uxts-module` junk) — candidate rider for HIDDEN-CHURN-001.
- Decay→prune steady-state: edge pruning bites only after decay erodes
  weights across weekly cycles — revisit counts after ~3 scheduled runs.
- The 2026-06-08 scheduled run's failure (`success=f`, pre-dockerbin era
  artifact) needed no action — superseded by this sprint's live success.

## Documents Accessed

- `internal/cli/maintenance.go`, `prune.go` (sweeps, tombstone rules),
  `job_report.go`, `service_darwin.go`, `upgrade.go`
- `packaging/launchd/com.mdemg.maintenance.plist` (+ embedded copy,
  installed copy)
- `internal/alert/rules.go`, `internal/config/config.go`,
  `internal/cli/serve.go`
- Live Neo4j (candidate census, before/after totals), V0024
  `scheduled_job_events` (3-row story), alert file
- `docs/development/roadmap/ROADMAP_2026Q3.md`
