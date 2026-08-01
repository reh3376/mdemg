# DASHBOARD-TRUTH-003 — Sprint Plan

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Parent trigger:** Operator flag on 2 J17 metrics reading "N/A" + 5 Jiminy
metrics reading "much lower than they should be." Multi-agent deep-dive
(7 investigators + synthesizer, workflow `wf_30b994e6`) classified 7 metrics:
5 pure MEASUREMENT_ARTIFACT, 2 BOTH (artifact + design), 0 REAL_LOW.

Same shape as DASHBOARD-TRUTH-001/-002: most alarming-looking numbers on
Jiminy/J17 panels are calibration/wiring artifacts, not substrate failures.

## Section 1 — Header & Metadata

- Line: DASHBOARD-TRUTH
- Sprint: 003
- Sequence: post-001 (2026-07-03) → post-002 (2026-07-20)
- Related: JIMINY-CORPUS-001 (rate steady state), JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 (rate math)

## Section 2 — Problem Statement

7 dashboard metrics look broken; deep-dive proved 5 are measurement artifacts
+ 2 are correct-by-design but under-described. Same class as prior sprints in
the DASHBOARD-TRUTH line — wrong thresholds, wrong SQL shape, missing panel
description, hardcoded RSIC floors that predate the calibrated steady state.

## Section 3 — Scope & Constraints

**In scope:** dashboard threshold recalibration, description prose, one panel
SQL copy-fix (auto-compact) from a shipped twin, extraction of 2 hardcoded
RSIC floors to config.

**Out of scope:** substrate work (guidance/J17 corpora are at their honest
steady state; a "raise the rate" push is HITL-arc territory, not this sprint).
J17 Min/Max Trust panels already correctly render N/A via DASHBOARD-TRUTH-002
E6 gating; only description tightening if needed.

**Constraint:** ULTS `system_prompt_hash` pins untouched (this sprint edits
zero LLM prompts). CLAUDE.md steady-state guarantees (Jiminy follow ~14%
honest by design) MUST be preserved in threshold choice — don't recalibrate
back into always-red.

## Section 4 — Dependencies

- Deep-dive workflow output (7-agent triage + synthesizer recommendations)
- CLAUDE.md pinned steady-state numbers (JIMINY-CORPUS-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001)
- Grafana embed sync target (`make sync-grafana-embed` + `verify-grafana-embed`)

## Section 5 — Implementation Plan

- **E1** Guidance Health panel: add description referencing DASHBOARD-TRUTH-003 + recalibrate thresholds red<0.20 / yellow≥0.20 / green≥0.28.
- **E2** Follow Rate panel: add description + recalibrate red null / yellow≥0.05 / green≥0.15.
- **E3** Actionable Compliance Rate: rename to "Actionable Compliance Rate (expected 10-13% by design)"; add ab_verdict.md reference in description; 4-band recalibration (red<0.05 / yellow 0.05-0.10 / light-green 0.10-0.25 / green≥0.25).
- **E4** Effectiveness Trends panel: DELETE refB (`mdemg_jiminy_constraint_effectiveness` — the honest series is refD).
- **E5** Auto-compact AVG: copy-fix SQL from j17 twin (lifetime-last-10 rare-event pattern; `lastNotNull` calc).
- **E6** Extract self_reflect.go hardcoded 0.5/0.7 GuidanceHealth floors → `RSIC_GUIDANCE_HEALTH_FOLLOW_FLOOR` (default 0.20) + `RSIC_GUIDANCE_HEALTH_DRIFT_FLOOR` (default 0.25). Pattern 9 + 15 both gated on config; floor=0 disables.
- **E7** Config knobs added + FromEnv wired + struct comment.
- **E8** Test coverage: existing pattern 15 test updated to pass explicit floor; new pattern 9 test with 4 cases (fires below, quiet at floor, legacy 0.5 works, floor=0 disables).
- **E9** Staged mirror sync via `make sync-grafana-embed`; verify no drift.
- **E10** Feature doc + CLAUDE.md pin.

Note: J17 Min/Max Trust already correctly renders "N/A" when trust_session_count < 2
(DASHBOARD-TRUTH-002 E6, shipped 2026-07-20). Descriptions already comprehensive.
No changes to those panels — this is the "correct-by-design" branch.

## Section 6 — Testing Plan

- **Tier 1 unit:** `go test ./internal/ape/ -run 'GuidanceConfidenceDrift|LowGuidanceFollowRate' -count=1` — 4 cases (existing + 3 new) validating floor-driven gating.
- **Tier 2 integration:** `go test ./internal/config/ ./internal/grafanapin/ -count=1` — config parse + dashboard pin.
- **Tier 3 live e2e:** restart mdemg native binary → reload Grafana → verify: (a) Guidance Health panel color matches new bands, (b) Actionable Compliance panel shows new title + no-Q hostile red band, (c) Auto-compact panel renders numeric (was "No data"), (d) Effectiveness Trends shows only 2 series (refA + refD, no gauge series), (e) launchctl setenv `RSIC_GUIDANCE_HEALTH_FOLLOW_FLOOR=0` → server startup log shows the value.

## Section 7 — Commit Strategy

Single commit. All edits are cohesive dashboard-honesty work; no cross-cutting refactor.

## Section 8 — Verification Checklist

- [ ] `go build ./...` clean
- [ ] `go test ./...` all green
- [ ] `golangci-lint run ./internal/ape/ ./internal/config/ ./internal/grafanapin/` clean
- [ ] `make verify-grafana-embed` no drift
- [ ] Live smoke: 5 dashboard panels render new state on mdemg-dev

## Section 9 — Documentation Update

- `docs/features/rsic-guidance-health-floors.md` (new — DASHBOARD-TRUTH-003 §Why/Choices/HowItWorks/HowToUse)
- CLAUDE.md: pin the sprint under the DASHBOARD-TRUTH line
- CHANGELOG: v0.11.x entry

## Section 10 — Risks & Mitigations

- **Risk:** RSIC pattern 9 was firing on healthy substrate ~always at floor 0.5; extracting to 0.20 may leave a genuine emergency undetected. **Mitigation:** floor is env-configurable; operator can restore 0.5 semantics without a rebuild. Legacy semantics regression-tested.
- **Risk:** Grafana embed sync forgotten. **Mitigation:** `make verify-grafana-embed` in CI is merge-blocking.

## Section 11 — Documents Accessed

- Deep-dive workflow output (7-agent + synthesizer, `wf_30b994e6-1cb`)
- CLAUDE.md (DASHBOARD-TRUTH-001, DASHBOARD-TRUTH-002, JIMINY-CORPUS-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001)
- `deploy/docker/grafana/dashboards/mdemg-jiminy.json`
- `deploy/docker/grafana/dashboards/mdemg-j17.json`
- `internal/ape/self_reflect.go` (patterns 9, 15)
- `internal/config/config.go` (RSIC knob group)
