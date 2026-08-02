# ESCALATION-DIAGNOSIS-001 — Findings

**Date:** 2026-08-02 | **Type:** Investigation-only (no code change)
**Trigger:** JIMINY-ENFORCE-001 live-verify surfaced that 0 constraints have ever reached WARNED escalation, so the enforcement deny path had no natural trigger to test. This investigation isolates why.

## TL;DR

**The escalation-tracker wire is intact but produces zero WARNED states.** Root cause is a compound of THREE defects; the dominant one is a session_id-scoping mismatch that makes RecordOutcome operate on keys that were never created by RecordSurface — so ignore-count never accumulates past 0. Two supporting defects reduce persistence cadence and reset accumulation on decay. **All three must be addressed for JIMINY-ENFORCE-002/004 to have signal to consume.**

## Live evidence (mdemg-dev, 2026-08-02)

### State inventory (Neo4j `J12EscalationState`)
- **73 session rows** persisted (one per session ever active)
- Every row's `data` JSON contains 1-25 constraint-node entries; **100% at `level=surfaced` with `ignore_count=0`**
- `claude-core` session: 5 nodes tracked, `updated_at=2026-07-22T13:25:05` (**10 days stale**)
- Claude Code UUID session `46583515-3b99-488f-a0fa-057205bd4204`: 3 nodes tracked, `updated_at=2026-08-02T16:36:24` (fresh)

### Feedback pipeline is producing outcomes at production volumes
- `constraint_outcomes` (TSDB): **2,177 rows** in last 7d for session `46583515-3b99-488f-a0fa-057205bd4204`
- Mix: ignored/followed/contradicted/partial as expected
- Outcome constraint_ids sampled from last 2h: `myya3xf8kpk3wpbo0qonah99`, `yimid0i7uzy90dya7lrhdi7u`, `d4s33hmeh842nb7mw8t9p3m5`, `mfm4i6l9pb1nugun9egce9c6`

### The smoking-gun mismatch
- **Escalation state for UUID session has these node_ids:** `a5ax6i2owzuwlry7emx0pqkh`, `r7xt3abpn0qb0aebdbo7r6iw`, `sqe0bca027ynqkx10cd9j90q`
- **Outcome constraint_ids for same UUID session (same hour):** `myya3xf8...`, `yimid0i7...`, `d4s33hme...`, `mfm4i6l9...`
- **Zero overlap.** Surface state and outcome sink operate on disjoint node sets for the same session.

### Feedback-dropped metric is silent
- `mdemg_jiminy_feedback_dropped` returns no rows in TSDB → `tracker.Lookup(guidance_id)` is NOT returning nil → the feedback loop IS running → `RecordOutcome` on the escalation tracker IS being called for each `SourceNodes[0]` → **but the state map returns `EscalationInactive` because the key doesn't exist**.

## Root cause: three interacting defects

### DEFECT 1 — RecordOutcome silently drops on missing key (`escalation.go:115-118`) — DOMINANT

```go
func (et *EscalationTracker) RecordOutcome(sessionID, nodeID string, outcome GuidanceOutcome) EscalationLevel {
    ...
    state, ok := et.states[key]
    if !ok {
        return EscalationInactive  // ← the entire escalation cycle stops here
    }
```

An outcome on a `(session, node)` pair that was NEVER `RecordSurface`'d is dropped with no error, no metric, no log. The receiver has no idea the accumulation didn't happen. **This is the mechanism that produces zero WARNED states given the empirical mismatch above.**

### DEFECT 2 — Session_id resolution asymmetry between warm and feedback paths

- **Warm hook** (`prompt-context.sh:32-43`): resolves SESSION_ID as `MDEMG_SESSION_ID env → stdin session_id → ~/.mdemg/.claude-session file → "claude-core" fallback`. Guide() → `RecordSurface(req.SessionID, nodeID)`.
- **Feedback path** (`service.go:1675-1678`): uses `req.SessionID` from the /v1/jiminy/feedback POST body; falls back to `req.SpaceID` if missing. RecordOutcome uses this `feedbackSessionID`.

If the warm hook and the feedback POST resolve to different session_ids (e.g., MDEMG_SESSION_ID unset at warm time but set at feedback time, or fallback branch taken by one but not the other), the escalation tracker's `escalationKey{SessionID, NodeID}` compound-key differs → surface writes to key K1, outcome tries key K2, state[K2] doesn't exist → Defect 1 fires.

The empirical mismatch on UUID session `46583515-...` PROVES the surface-and-outcome key pairs are colliding to the same session_id (both use the UUID) — so this specific instance is NOT a session_id mismatch. But the DEFECT still stands as a class-of-bug: NOTHING enforces that warm and feedback resolve to the same session, and both hooks have multiple fallback branches.

### DEFECT 3 — RecordSurface resets IgnoreCount on decay (`escalation.go:94-102`)

```go
if time.Since(state.LastSurfaced) > et.decayDuration {  // default 60 min
    state.Level = EscalationSurfaced
    state.IgnoreCount = 0  // ← wipes accumulated ignores
    ...
}
```

When a constraint is re-surfaced after 60+ min of inactivity, its `IgnoreCount` is reset to 0 alongside its Level. Design intent: "constraint hasn't been relevant for a while, start fresh." But an ignore that lands 61 min after a surface + before the next surface is IMMEDIATELY wiped on the next surface. In low-activity sessions or on constraints that surface intermittently, this makes accumulation past 1 nearly impossible.

**Compound with Defect 1**: even if the surface-outcome key pair aligns, this reset erases any signal that would have made the warn-after threshold (default 2 ignores) fire.

### DEFECT 4 (supporting, non-root) — dirty flag only fires on new keys or decay-reset (`escalation.go:87-90, 99-101`)

RecordSurface only calls `onDirty(sessionID)` when:
1. A new escalation-key is being created, OR
2. The decay-reset branch fires

Repeated surfaces of an already-tracked constraint DO update `LastSurfaced` in-memory but do NOT mark the session dirty → no flush → the persisted state in Neo4j stays stale until either a new key is added or decay-reset fires.

This doesn't cause the missing WARNED states directly but explains why persistence timestamps look ~stuck compared to live activity.

## Why zero fires: putting it together

1. Guide() at T0 surfaces constraints A, B, C for session S → RecordSurface creates 3 in-memory states, onDirty fires, flush persists {A,B,C}
2. Guide() at T1 surfaces different constraints D, E, F for session S → RecordSurface creates 3 more, onDirty fires, flush persists {A,B,C,D,E,F}
3. …many Guide() calls, in-memory grows, persisted state grows…
4. Feedback POST for a guidance surfaced back at T-2 (BEFORE the last server restart of today) arrives with items whose SourceNodes are `myya3xf8...`, `yimid0i7...`
5. Server restarted during today's four sprints (LEVER-C-TIGHTEN-001 + HEBB-ETA-001 + FOLLOW-RATE-CALIBRATE-001 + JIMINY-ENFORCE-001). Restart → in-memory wiped → LoadAll → only pre-restart-persisted states loaded → `myya3xf8...` etc are NOT there
6. RecordOutcome fires on `myya3xf8...` → Defect 1 → EscalationInactive, silent drop
7. Constraint_outcomes IS still written (that sink doesn't consult escalation state) → so the outcome count grows while the escalation stays at 0

The pattern REPEATS every restart. Even without restarts, Defect 3 wipes any accumulation across 60-min gaps.

## Impact on the JIMINY-ENFORCE arc

- **JIMINY-ENFORCE-001** (this sprint): E2 deny-path can't be naturally triggered because no WARNED state exists. Wire is unit-test-verified but not e2e-live-verified. Sprint post-notes this honestly.
- **JIMINY-ENFORCE-002** (Bash coverage): same problem — enforcement gate has nothing to enforce against. The Bash hook would be a well-built pipe carrying zero signal.
- **JIMINY-ENFORCE-004** (RSIC enforcement-learning): explicitly requires `blocked_true_positive` / `blocked_false_positive` / `missed_violation` outcome events. If escalation → deny → block never happens, none of these outcomes ever land. The RSIC loop starves.

**Conclusion: the JIMINY-ENFORCE arc CANNOT usefully proceed until escalation accumulation works.**

## Recommended sprint: ESCALATION-ACCUMULATE-001 (fold before JIMINY-ENFORCE-002)

### Scope
1. **DEFECT 1 fix**: `RecordOutcome` on a missing key MUST create the state (with `IgnoreCount=1`) instead of returning `EscalationInactive`. The outcome IS the surface signal in this case — the constraint was clearly known to the classifier for it to produce an ignored/contradicted verdict. Alternative: emit a `escalation_outcome_orphaned` metric so the drop is visible. The right choice is CREATE, not warn — reasoning below.
2. **DEFECT 3 fix**: decay-reset should reset LEVEL only, NOT IgnoreCount. Or better: use a decay-decrement model (IgnoreCount decays by 1 per decayDuration since last outcome) so accumulation survives normal gaps.
3. **DEFECT 4 fix**: `RecordSurface` should call `onDirty` on every surface (including repeats) — persistence should track live activity. Small cost, prevents stale-state confusion.
4. **DEFECT 2 mitigation**: audit both hooks + service.go handleJiminyWarm + handleJiminyFeedback to enforce session_id parity. Add a startup-log line naming which resolution branch was taken. Doc the resolution rules in `docs/features/jiminy-strict.md`.
5. **Verification**: after fixes, wait 24-48h of natural activity, verify: (a) `IgnoreCount` values >0 exist on some state entries, (b) at least one constraint reaches WARNED on a repeat-ignored session.

### Why CREATE on missing key (Defect 1) is right, not warn-and-drop
The classifier's decision to record an outcome on a constraint IS proof the constraint was relevant. Requiring a preceding RecordSurface introduces a temporal-order fragility (surface-before-outcome, no restart between) that the current architecture can't honor. If the outcome fires, treat it as authoritative: `escalationKey{S,N}` gets a fresh state at `IgnoreCount=1` (or 0 for `followed`). The next outcome-or-surface on the same key resumes accumulation.

### Not shipped by this investigation
This is a diagnosis document. Implementation is ESCALATION-ACCUMULATE-001. Given operator directive to run JIMINY-ENFORCE arc sequentially, propose:

- **Reorder**: ESCALATION-ACCUMULATE-001 (1 sprint, ~half-day) → JIMINY-ENFORCE-002 (Bash coverage) → JIMINY-ENFORCE-003 (override CLI) → JIMINY-ENFORCE-004 (RSIC enforcement-learning) → JIMINY-ENFORCE-005 (missed-violation detector).

OR

- **Fold into ENFORCE-002**: ship the escalation fix as ENFORCE-002 Epic 0 before adding Bash coverage. Total scope grows but keeps the arc labeled JIMINY-ENFORCE-*.

Operator picks the framing.

## Documents Accessed

- `internal/jiminy/escalation.go` (all 298 lines)
- `internal/jiminy/escalation_store.go` (persistence + JSON schema)
- `internal/jiminy/service.go:1164-1173` (RecordSurface call site)
- `internal/jiminy/service.go:1650-1826` (RecordOutcome pipeline)
- `internal/jiminy/service.go:1675-1678` (session_id resolution)
- `internal/cli/hook_templates/prompt-context.sh:32-43, 190` (warm SESSION_ID resolution)
- Live Neo4j: 73 `J12EscalationState` rows, sampled 5 sessions including `claude-core` + Claude Code UUID
- Live TSDB: `constraint_outcomes` last 7d volume + last 2h constraint_ids for UUID session
- Live TSDB: `mdemg_jiminy_*` metrics last 4h (`feedback_dropped` never emitted, `guide_calls_total` = 1)
