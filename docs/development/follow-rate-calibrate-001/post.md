# FOLLOW-RATE-CALIBRATE-001 — Sprint Post

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Trigger:** HEBB-ETA-001 live-smoke surfaced a `Jiminy Follow Rate Drop` (MEDIUM) alert
against a substrate whose LEVER-C-TIGHTEN-001 revert tripwires were all CLEAR (actionable-
compliance 19-29%, well above 0.10 floor). Investigation showed the alert rule was hardcoded
at 0.30 threshold — exactly the honest post-JIMINY-CORPUS-001 raw follow-rate steady state
— so it flapped chronically on a healthy substrate. Same DASHBOARD-TRUTH class drift the
sprint arc has been fixing.

## Verdict

**Shipped.** Rule extracted from `DefaultRules()` to config-driven `JiminyFollowRateRules(floor)`;
new `JIMINY_FOLLOW_RATE_ALERT_FLOOR` config knob (default **0.15**, was hardcoded 0.30).
Floor=0 disables the rule (opt-out). Live-verified: prior fired alert cleared post-restart;
no new fires in a 90s observation window.

## What shipped

- `internal/alert/rules.go`: new `JiminyFollowRateRules(floor float64) []AlertRule` mirrors the extraction pattern used by `OrphanRules`, `Neo4jCPURule`, `GraphNodeDropRule`, `GuidanceShouldFollowRules`. Old inline rule removed from `DefaultRules()` (default rule count 5→4).
- `internal/cli/serve.go`: registered via `alert.JiminyFollowRateRules(cfg.JiminyFollowRateAlertFloor)`.
- `internal/config/config.go`: `JiminyFollowRateAlertFloor` field + `JIMINY_FOLLOW_RATE_ALERT_FLOOR` env (default 0.15, floor=0 disables).
- `internal/alert/rules_test.go`: new `TestJiminyFollowRateRules` (4 assertions: shape, idle-safe SQL, no LIMIT 1 anti-pattern, floor≤0 disables).
- `internal/alert/evaluator_test.go`: `TestDefaultRules_Count` bumped 5→4 with comment referencing this sprint.

## Live Tier-3

- Rebuild + `launchctl kickstart` → `/healthz` green.
- `~/.mdemg/alerts/current.json`: prior `Jiminy Follow Rate Drop` alert (id `gd0fr722umgiy86u999d94cl`) now shows `cleared: true`.
- 90s post-restart observation window: no new `jiminy_follow_rate_drop` fires.

## Rules pinned

⚠️ **Alert thresholds against a metric with a known-honest steady state MUST sit BELOW that state, and MUST be config-driven.** The 0.30 threshold predated JIMINY-CORPUS-001 by weeks — the corpus purge shifted the raw follow-rate steady state right onto the threshold. Extracting to config + defaulting to 0.15 (below the ~0.30 steady state) means the rule fires only on genuine collapse, not on healthy state. Same class as DASHBOARD-TRUTH-003's RSIC floor extraction; same class as the JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR` recalibration.

⚠️ **When a sprint changes a substrate steady state (via corpus purge, tightening, or reweighting), audit `internal/alert/*.go` for hardcoded thresholds that gate on the affected metric — the alert rules are as prone to steady-state-drift as the RSIC self-reflection patterns and Grafana panels.**

## Rollback

Single-commit revert restores the hardcoded 0.30. The `JiminyFollowRateAlertFloor` field stays in the struct (harmless — nothing else reads it).

## Documents Accessed

- `internal/alert/rules.go` (extraction site)
- `internal/alert/rules_test.go` (test)
- `internal/alert/evaluator_test.go` (count pin)
- `internal/cli/serve.go` (rule registration)
- `internal/config/config.go` (new knob)
- `~/.mdemg/alerts/current.json` (live verification)
- 4h TSDB query on `constraint_outcomes` (mdemg-dev; confirmed honest ~0.30 raw + ~0.20 actionable steady state)
- CLAUDE.md: DASHBOARD-TRUTH-003, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001, LEVER-C-TIGHTEN-001, JIMINY-CORPUS-001
