# ESCALATION-ACCUMULATE-001 — Sprint Post

**Date:** 2026-08-02 | **Branch:** `reh3376_dev01`
**Arc:** JIMINY-ENFORCE prerequisite (inserted before ENFORCE-002)
**Trigger:** ESCALATION-DIAGNOSIS-001 findings — 100% of 73 `J12EscalationState` session rows sat at `level=surfaced ignore_count=0` despite 2,177 outcomes/7d, blocking every downstream step of the JIMINY-ENFORCE arc.

## Verdict

**Shipped.** All three diagnosed defects fixed, session_id parity normalized. Live Tier-3 produced the **first WARNED escalation state ever seen on the substrate** — 8 nodes at `level=warned, ignore_count=2` and 1 node at `level=escalated, ignore_count=4` after a controlled 2-ignore-feedback drill.

## What shipped

### Defect 1 fix — `RecordOutcome` creates state on missing key
`internal/jiminy/escalation.go:RecordOutcome`. Pre-fix: silent-drop returning `EscalationInactive` when `(session, node)` key not in state map. Post-fix: create the state on-demand at `Level=Surfaced, LastSurfaced=now()` and proceed to the outcome switch. **The outcome IS authoritative evidence the constraint was live** for the classifier — requiring a preceding RecordSurface introduced a fragility the shipped architecture couldn't honor (server restarts wipe in-memory; only pre-restart-persisted keys reload).

### Defect 3 fix — decay preserves `IgnoreCount`
`internal/jiminy/escalation.go:RecordSurface`. Pre-fix: the decay branch wiped `IgnoreCount=0` alongside resetting `Level=Surfaced`, making WARNED (default `warnAfter=2`) unreachable in any session where the 60min decay triggered between two ignores. Post-fix: **decay resets `Level` only, `IgnoreCount` survives.** Operator still sees the "constraint went quiet" signal via the level demoting; accumulation now survives normal activity gaps.

### Defect 4 fix — `onDirty` on every surface
Pre-fix: `RecordSurface` marked the session dirty only on new-key creation or decay-reset. Repeated surfaces of an already-tracked constraint updated `LastSurfaced` in-memory but didn't mark dirty → no flush → persisted state hours stale relative to in-memory. Post-fix: `onDirty` fires on every surface. Small per-surface cost; persistence tracks live activity.

### Defect 2 mitigation — session_id parity
`internal/api/handlers_jiminy.go:resolveJiminySessionID(sessionID, spaceID)` — new helper: explicit `session_id` wins; else fall back to `space_id`. **Both `/v1/jiminy/warm` and `/v1/jiminy/feedback` now use identical resolution.** Pre-fix, warm kept empty `session_id` as empty (would fail the `req.SessionID != ""` guard in `Guide()`); feedback fell back to `space_id`. Silent key-mismatch class of failure is now closed.

### Test coverage
Three new pins in `internal/jiminy/j7_j12_test.go`:
- `TestEscalationTracker_DecayPreservesIgnoreCount` — verifies IgnoreCount survives decay; without the fix, resurface after decay + one more ignore would land at count=1 (Surfaced), post-fix lands at count=2 (WARNED)
- `TestEscalationTracker_RecordOutcomeCreatesMissingKey` — verifies outcome on never-surfaced key creates state, second outcome reaches WARNED, followed resolves
- `TestEscalationTracker_RepeatSurfaceMarksDirty` — verifies `onDirty` fires on every surface, not just new-key

All 9 escalation-tracker tests pass (6 existing + 3 new).

## Live Tier-3 (mdemg-dev, 2026-08-02)

Drill setup — controlled session `escalate-accumulate-live-1785704013`:
1. `POST /v1/jiminy/warm` → 41s warm compute → 10 guidance items surfaced
2. `POST /v1/jiminy/feedback {outcome: "ignored"}` ×2

State post-flush (via APOC parse):
```
n_7f4d972379d1b1c1b089    level=warned      ignore_count=2
n_c266705408a59fa6fdee    level=warned      ignore_count=2
rktgw4xwpyn6b84kdjsto6q4  level=warned      ignore_count=2
n_e82dd0ec13cf73212637    level=warned      ignore_count=2
n_66b5d6e5fc36d2eda545    level=warned      ignore_count=2
gwc3twp6cpe1tdrfs7zcet5w  level=warned      ignore_count=2
n_21cea8e967a7b4501e77    level=warned      ignore_count=2
n_319e0c602ec84145cf52    level=escalated   ignore_count=4  ← surfaced earlier + accumulated
39b8c652-a691-4301-a08f-f59e848e6636  level=warned  ignore_count=2
```

**8 WARNED + 1 ESCALATED** — the first non-Surfaced levels ever produced. Test session cleaned up post-verification.

## Bonus finding uncovered during live test

Server log shows `jiminy: feedback dropped — guidance_id expired from tracker` fires **129 times across 12 distinct guidance_ids in the current log window**. Every one of those was a real feedback POST whose in-memory tracker entry had expired before the feedback arrived → tracker.Lookup returned nil → the entire feedback loop (including outcome writer, escalation update, trust EMA update) skipped.

This is a separate defect from the escalation gap; the DIAGNOSIS-001 read incorrectly concluded "tracker.Lookup succeeds always" by checking the wrong metric. The dropped rate needs its own investigation (tracker TTL setting? Restarts wiping tracker mid-guidance-lifecycle? Slow feedback latency?). **Follow-up sprint: JIMINY-TRACKER-TTL-001.**

Impact: the ESCALATION-ACCUMULATE-001 fix works for feedbacks that DO reach the loop; the tracker drop is a separate signal-loss upstream. Both need to work for the arc to fire on natural traffic; both must be addressed before JIMINY-ENFORCE-002.

## Rules pinned

⚠️ **`RecordOutcome` on a missing state key MUST create the state, not silent-drop.** The outcome is authoritative evidence the constraint was live for the classifier — requiring a preceding RecordSurface introduces temporal-order fragility server restarts can't honor. Silent-drop guarantees a broken accumulation loop.

⚠️ **Decay of a level MUST NOT reset an accumulator.** The two concerns are separable: level tracks "is this constraint currently escalated?" — a decay-to-surfaced is fine; count tracks "how many times has this been ignored?" — wiping it makes escalation thresholds unreachable in low-cadence sessions. Design rule: any counter that gates a threshold must survive TTL/decay events on its containing state.

⚠️ **Multi-handler paths that share a composite key MUST normalize their key components via a shared helper.** Warm + feedback both compute an `escalationKey{SessionID, NodeID}`; asymmetric fallbacks on `SessionID` silently produce composite-key mismatches. `resolveJiminySessionID` is the enforcement seam — both handlers now use it.

## Not shipped (disclosed)

- **JIMINY-TRACKER-TTL-001** — investigate + fix the `feedback dropped — guidance_id expired` class (129 fires/log-window on mdemg-dev)
- **RSIC signal-density recovery** — with escalation now producing WARNED states, the RSIC prune signal (`GetConstraintEffectiveness`) becomes actionable; JIMINY-ENFORCE-004 arc becomes viable

## Rollback

Single-commit revert. Neo4j `J12EscalationState` rows persist as-is (harmless — worst case they hold outdated snapshots that get overwritten on next flush).

## Documents Accessed

- ESCALATION-DIAGNOSIS-001 findings
- `internal/jiminy/escalation.go` (both RecordSurface + RecordOutcome)
- `internal/jiminy/escalation_store.go` (persistence — read-only, no changes)
- `internal/jiminy/j7_j12_test.go` (3 new tests, 6 existing verified)
- `internal/api/handlers_jiminy.go` (resolveJiminySessionID helper + warm handler)
- `internal/jiminy/service.go` (feedback path documentation update)
- Live Neo4j (73 pre-drill states, 1 post-drill test-session verification, cleaned up)
- Live server log (`~/.mdemg/logs/server.log`) — surfaced the `feedback dropped` class
