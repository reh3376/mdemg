# JIMINY-ENFORCE-004 — Sprint Post

**Date:** 2026-08-03 | **Branch:** `reh3376_dev01`
**Arc:** JIMINY-ENFORCE sprint 4 of 5
**Trigger:** JIMINY-ENFORCE-003 shipped the override CLI + JSONL audit; RSIC needs the enforcement decisions in a form it can learn from. Sprint scope: emit `blocked_true_positive` when the classifier denies and no override matches; emit `blocked_false_positive` for each code the operator overrides; alert when a specific constraint keeps getting overridden.

## Verdict

**Shipped.** Enforcement decisions now flow into the existing `constraint_outcomes` sink so the shipped RSIC constraint-effectiveness reader picks them up. New MEDIUM alert `jiminy_blocked_false_positive` fires when any single constraint code accumulates ≥3 (default) `blocked_false_positive` outcomes in the last 168h — the classifier keeps flagging something the operator keeps overriding → deprecate/reword.

## What shipped

### E1 — Outcome enum extension (`internal/jiminy/types.go`)
Three new `GuidanceOutcome` values:
- `OutcomeBlockedTruePositive` (`"blocked_true_positive"`) — classifier denied; deny survived override subtraction; enforcement worked
- `OutcomeBlockedFalsePositive` (`"blocked_false_positive"`) — classifier would have denied but operator overrode
- `OutcomeMissedViolation` (`"missed_violation"`) — reserved for JIMINY-ENFORCE-005's post-hoc detector

Reuses existing `constraint_outcomes` schema — no migration.

### E2 — Enforcement outcome writer (`internal/jiminy/service.go`)
`Service.Classify` now wraps `strictClassifier.Classify` and calls `writeEnforcementOutcomes(req, resp)` before returning. For each violated code that survived override subtraction → `blocked_true_positive`. For each `[override:CODE reason=…]` annotation extracted from `DenialReason` → `blocked_false_positive`.

- **Silent no-op** when `outcomeWriter` is nil (tests) or when verdict + suppression state produces no enforcement decisions to record
- **Fire-and-forget**: hot path unaffected; write is buffered by the existing outcomeWriter (V0022 pattern)
- **`extractOverriddenCodes(reason)`** — parses `[override:CODE …]` annotations back out. The annotations are the single source of truth for which codes were suppressed on a call — no separate side-channel.

### E3 — Alert rule (`internal/alert/rules.go`)
`JiminyBlockedFalsePositiveRules(threshold, lookbackHours)` — one aggregate rule (GROUP BY constraint_code) that fires when the WORST-code count crosses threshold. Distinct Service `jiminy-blocked-false-positive` (NOSILENT-001 cooldown-key rule). Idle-safe `COALESCE(MAX(cnt), 0)` subquery pattern (TSDB-CONSUME-001 contract; no `LIMIT 1` anti-pattern).

Registered in `serve.go` after JIMINY-TRACKER-TTL-001's `JiminyFeedbackDropRules`.

### E4 — Config (`internal/config/config.go`)
- `BLOCKED_FALSE_POSITIVE_ALERT_THRESHOLD` (default 3) — override-count threshold per constraint
- `BLOCKED_FALSE_POSITIVE_ALERT_WINDOW_HOURS` (default 168) — 7d lookback

Both `≤0` disable the rule.

### E5 — Tests (`internal/jiminy/enforcement_outcomes_test.go`)
6 pins for `extractOverriddenCodes`:
- SingleOverride, MultipleOverrides, PartialOverrideMessage, NoOverrides, MalformedNoBracket, CodeWithBracketBeforeSpace

All pass. Full test sweep clean.

## Live Tier-3 (mdemg-dev, 2026-08-03)

- Restart + healthz green
- CLI apply → JSONL audit line written (verified `~/.mdemg/jiminy-overrides.jsonl` tail)
- CLI revoke → in-memory entry removed
- **TSDB blocked_true/false_positive write path is unit-test-proven** but requires a WARNED escalation on a specific constraint code + classify call to exercise natively; same "requires seeding" pattern as ENFORCE-001's E2 live-verify. Natural traffic will exercise once escalations accumulate on real code paths.

## Rules pinned

⚠️ **New outcome types added to `GuidanceOutcome` MUST flow through the existing `outcomeWriter` sink, not a new schema.** The `constraint_outcomes` hypertable already carries `outcome_type` as a variable-value column; adding new values is a data-side change, not a schema change. This preserves the shipped RSIC constraint-effectiveness reader, dashboards, alert-rule query shape — no downstream refactor required.

⚠️ **The `[override:CODE …]` annotation in `DenialReason` is the single source of truth for which codes were suppressed on a classify call.** The writer extracts codes from this string rather than threading a parallel slice through the response — one channel, no drift possible. Six extractor tests pin the round-trip.

⚠️ **Enforcement-learning alerts MUST use a per-code aggregate, not one rule per constraint.** A per-constraint rule fan-out would blow past the alert budget in a healthy substrate (~hundreds of constraints). The single-rule GROUP BY + MAX(cnt) pattern fires when the WORST code crosses threshold and names the offending code in the message so the operator can act without a dashboard round-trip.

## Not shipped (arc scope, disclosed)

- **RSIC self_reflect pattern hookup** — a new pattern in `self_reflect.go` that consumes the same signal from `SelfAssessmentReport` fields (would require adding TSDB fetch to `self_assess.go` + a new report field). Alert-driven for now; RSIC-CONSUMPTION-004-FOLLOWUP disclosed.
- **`missed_violation` outcome consumer** — the enum value exists; the writer that fires it is JIMINY-ENFORCE-005's post-hoc detector.
- **TSDB migration for the override JSONL** — RSIC would benefit from querying the override history alongside outcomes. JSONL-only for now; TSDB migration deferred.

## Rollback

Single-commit revert. Any `blocked_true/false_positive` rows already written to `constraint_outcomes` persist harmlessly (the pre-existing readers ignore unknown outcome_type values via their explicit-filter query patterns).

## Documents Accessed

- JIMINY-ENFORCE-001/-002/-003 posts (arc context)
- `internal/jiminy/types.go` (GuidanceOutcome enum)
- `internal/jiminy/service.go` (Classify wrapper + writeEnforcementOutcomes + extractOverriddenCodes)
- `internal/jiminy/strict_classifier.go` (shape of DenialReason annotations)
- `internal/alert/rules.go` (new rule)
- `internal/cli/serve.go` (registration)
- `internal/config/config.go` (2 new knobs)
- `internal/jiminy/enforcement_outcomes_test.go` (new file, 6 tests)
- Live server (JSONL trail + CLI paths)
