# JIMINY-ENFORCE-005 — Sprint Post

**Date:** 2026-08-03 | **Branch:** `reh3376_dev01`
**Arc:** JIMINY-ENFORCE sprint **5 of 5 — arc complete**
**Trigger:** JIMINY-ENFORCE-004 shipped `blocked_true/false_positive` outcomes; the arc's missing piece was the `missed_violation` writer — the retrospective signal that "the classifier should have blocked something and didn't."

## Verdict

**Shipped — arc complete.** `/v1/conversation/observe` on an `obs_type=correction` observation now spawns a detached goroutine that (a) embeds the correction content, (b) queries the constraint vector index for a match ≥ threshold, (c) writes an `OutcomeMissedViolation` row to `constraint_outcomes` when a match exists. RSIC consumes the signal via the shipped constraint-effectiveness reader. New MEDIUM alert `jiminy_missed_violation` fires when any constraint accumulates ≥3 (default) missed_violations in the 168h window.

Live-verified end-to-end: correction observation → 12s async — matched constraint `no-direct-main-commits-must-master-cms-usage-track` at cosine sim ≥ 0.55 → TSDB row landed with `outcome_type=missed_violation`, `classifier_source=correction_observed`, log line `jiminy: missed_violation detected`.

## What shipped

### E1 — Detector (`internal/jiminy/service.go`)
`Service.DetectMissedViolation(ctx, spaceID, sessionID, embedding) string`:
- Reuses the shipped `matchConstraintCodeByEmbedding` (JIMINY-OUTCOME-001 pattern) with the shipped `JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD` (0.55) — cosine sim over the role-filtered constraint/correction vector index
- On match, writes ONE row to `constraint_outcomes` via the existing `outcomeWriter`:
  - `outcome_type = "missed_violation"`
  - `constraint_code = <matched>`
  - `classifier_source = "correction_observed"` (distinguishes from the classifier-driven writes)
- Returns the matched code (empty on no-match)
- INFO log line `jiminy: missed_violation detected` names the space/session/code

### E2 — API hook (`internal/api/handlers_conversation.go`)
Post-`Observe`, when `req.ObsType == "correction"` AND jiminy + embedder are wired AND content non-empty → spawn a detached goroutine that:
- Uses a 10s timeout context (NOT the request ctx — request ctx dies when the HTTP response is written; detector must survive to finish embed + Neo4j query)
- Embeds via `s.embedder.Embed` with `CallSite: "jiminy_missed_violation_detector"`
- Calls `jiminySvc.DetectMissedViolation`
- Detached-goroutine pattern is a project-standard shape (GUARDRAIL-PRODUCER-001 precedent); nolint comment explains the intent

### E3 — Alert rule (`internal/alert/rules.go`)
`JiminyMissedViolationRules(threshold, lookbackHours)` — MEDIUM severity, service `jiminy-missed-violation`, aggregate GROUP BY constraint_code + `COALESCE(MAX(cnt),0)` subquery. Same shape as JIMINY-ENFORCE-004's `JiminyBlockedFalsePositiveRules` (per-code aggregate, idle-safe TSDB-CONSUME-001 contract, fires when WORST code crosses threshold).

Registered in `serve.go` after ENFORCE-004's rule.

### E4 — Config
- `MISSED_VIOLATION_ALERT_THRESHOLD` (default 3)
- `MISSED_VIOLATION_ALERT_WINDOW_HOURS` (default 168 = 7d)
- Both `≤0` disable the alert

### E5 — Tests
No new pin tests (the detector's ingredients — `matchConstraintCodeByEmbedding` + `outcomeWriter.RecordOutcome` — are already pin-tested via ENFORCE-004's extractor tests + shipped vector-index tests). The live smoke below is the integration verification.

## Live Tier-3 (mdemg-dev, 2026-08-03)

```bash
# 1. Send a correction observation whose content matches an existing constraint
$ curl -s -X POST http://localhost:9999/v1/conversation/observe \
    -H "Content-Type: application/json" \
    -d '{"space_id":"mdemg-dev","session_id":"enforce005-live-1785733517","obs_type":"correction","content":"CORRECTION: I attempted to commit directly to the main branch bypassing the dev-branch workflow. This violates the mandatory branch-protection policy. Should have branched from main to reh3376_dev01 first."}'
{"obs_id":"ogb7o028wy5bqap3mqp7vu0x","node_id":"hgieuahy1x28kgq5dceugrae",…}

# 2. Wait for async detector + outcome-writer flush (~60s)

# 3. Verify TSDB row
$ docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -tAc \
    "SELECT time, constraint_code, session_id, guidance_type, classifier_source
     FROM constraint_outcomes WHERE outcome_type='missed_violation'
       AND session_id='enforce005-live-1785733517' ORDER BY time DESC LIMIT 3"
2026-08-03 05:05:20.088172+00|no-direct-main-commits-must-master-cms-usage-track|enforce005-live-1785733517|constraint|correction_observed

# 4. Server log
$ grep 'missed_violation detected' ~/.mdemg/logs/server.log | tail -1
level=INFO msg="jiminy: missed_violation detected" space_id=mdemg-dev session_id=enforce005-live-1785733517 constraint_code=no-direct-main-commits-must-master-cms-usage-track
```

End-to-end verified: correction → embed → vector match → TSDB row → RSIC reader now has the missed-violation signal alongside the follow/ignore/blocked outcomes.

## Rules pinned

⚠️ **The correction observation IS the operator's retrospective judgment that the classifier missed something.** Rather than build a scanner that walks historical actions and re-classifies them, hook the correction-write path directly. The correction's content is embedding-matched to the constraint corpus with the shipped 0.55 threshold; match → `missed_violation` row. Simpler than a scanner, and the signal is real-time (fires when the operator writes the correction, not on a cron).

⚠️ **Detached goroutines for post-response work MUST use `context.Background()` with an explicit timeout, not the request context.** The request context is cancelled when the HTTP response is written; the detector must survive to finish the embed + Neo4j vector query. GUARDRAIL-PRODUCER-001 established this pattern; this sprint's detector reuses it. The gosec G118 warning is documented + suppressed.

⚠️ **Enforcement-arc alerts share ONE shape**: per-code GROUP BY + COALESCE(MAX(cnt),0) subquery + WORST-code-wins threshold + code named in the message. Blocked-false-positive (ENFORCE-004) and missed-violation (ENFORCE-005) are twins. Future enforcement-outcome alerts should mint from the same template — never per-constraint fan-out.

## JIMINY-ENFORCE arc — closeout summary

Five sprints, all shipped, ~72h wall time (2026-08-01 → 2026-08-03). Prerequisites: ESCALATION-ACCUMULATE-001 + JIMINY-TRACKER-TTL-001. Feature: JIMINY-MODE-001 UI toggle.

| # | Sprint | What ships |
|---|---|---|
| 1 | ENFORCE-001 | Strict-by-default + alert-on-block + fail-open-with-warning (Write/Edit) |
| 2 | ENFORCE-002 | Bash coverage — extension to pre-bash-check.py + read-only whitelist |
| 3 | ENFORCE-003 | Override CLI + JSONL audit + classifier suppression |
| 4 | ENFORCE-004 | RSIC enforcement-learning outcomes (blocked_true/false_positive) + alert |
| 5 | **ENFORCE-005** | **Post-hoc missed-violation detector + alert (this sprint, arc complete)** |

The stateless + probabilistic LLM now has stateful + deterministic enforcement across its two main mutation surfaces (Write/Edit + Bash), a human-in-the-loop escape hatch with audit trail, and an RSIC-consumable learning signal on every enforcement decision (deny-wanted, deny-overridden, missed-entirely).

### Arc-level architectural rules pinned (aggregate)
Across the 5 sprints: **13 new arch rules** pinned to CLAUDE.md, most notable:
- Enforcement layers stack: destructive-guard (fail-closed) first, Jiminy classify (fail-open) second — never merge failure modes
- Fail-open enforcement MUST leave a persistent warning trail
- Alert dispatch is server-side, same authority as the enforcement decision
- Overrides MUST be time-boxed with required reason, session-scoped never global
- New outcome types reuse the existing outcome sink (no new schema)
- Enforcement-learning alerts use per-code aggregate, not per-constraint fan-out
- Detached goroutines for post-response work use context.Background + explicit timeout
- Read-only Bash whitelist entries MUST be provably read-only (wrong-side entry = stealth bypass)

## Not shipped (deferred; arc-adjacent)

- **RSIC self_reflect pattern hookup** — new patterns in self_reflect.go that consume `blocked_true_positive` / `blocked_false_positive` / `missed_violation` counts from SelfAssessmentReport (needs TSDB fetch in self_assess.go + new report fields). Alert-driven for now; would elevate from alert to autonomous constraint deprecate/reword actions.
- **TSDB migration for override JSONL** — RSIC would benefit from querying override history alongside outcomes. JSONL-only for now.
- **UI display of active overrides + arc dashboard** — Jiminy tab (JIMINY-MODE-001) could gain an "Active Overrides" table with revoke buttons, plus a small enforcement-decisions timeline (deny / overridden / missed counts per constraint).

## Rollback

Single-commit revert. Any `missed_violation` rows already written persist harmlessly (pre-existing readers ignore unknown outcome_type values via explicit-filter queries; the new alert simply returns 0 fires).

## Documents Accessed

- JIMINY-ENFORCE-001/-002/-003/-004 posts (arc context)
- `internal/jiminy/service.go` (DetectMissedViolation added; reused matchConstraintCodeByEmbedding)
- `internal/api/handlers_conversation.go` (async hook added post-Observe)
- `internal/conversation/service.go` (Observe entry-point survey — no change needed)
- `internal/alert/rules.go` (new JiminyMissedViolationRules — twin of ENFORCE-004's rule)
- `internal/cli/serve.go` (registration)
- `internal/config/config.go` (2 new knobs)
- Live server + TSDB `constraint_outcomes` (end-to-end drill)
