# Phase 89: RSIC Persistence & Multi-Space Correctness

**Status**: Complete
**Priority**: High
**Date**: 2026-02-16
**Depends On**: Phase 88 (`docs/specs/phase88-rsic-safety-policy-enforcement.md`)
**Related Handoff Section**: `AGENT_HANDOFF.md` → `RSIC Implementation`
**Gap References**: `docs/development/RSIC_GAP_ANALYSIS.md` — Gap #3 (calibration volatility), Gap #4 (hard-coded identity), Gap #5 (datetime handling), Gap #6 (task lifecycle)

---

## Purpose

Phase 89 makes RSIC state durable across server restarts and removes hard-coded identity assumptions that break multi-space correctness.

Currently **all RSIC learned state is in-memory only**: calibration confidence scores, cycle history, watchdog decay tracking, orchestration cooldowns, and rollback snapshots are lost on every server restart. The watchdog is hard-wired to `"mdemg-dev"` and signal paths assume `"claude-core"` as the session ID. The consolidation-age adapter has a Neo4j datetime type mismatch that silently zeroes the signal.

Phase 89 does **not** add new RSIC capabilities or API endpoints. It hardens the existing system for production reliability.

---

## Scope

- Persist calibration history and action confidence to Neo4j (survives restarts).
- Persist watchdog state (decay score, escalation level, last cycle time) to Neo4j.
- Persist recent cycle outcomes to Neo4j (queryable history across restarts).
- Add startup hydration: load persisted state when RSIC components initialize.
- Remove hard-coded `"mdemg-dev"` from watchdog initialization; use config value.
- Remove hard-coded `"claude-core"` from signal adapter paths; pass session context.
- Fix Neo4j datetime type assertion in consolidation-age adapter.
- Add lifecycle cleanup for Dispatcher `activeTasks` map (evict completed tasks after TTL).
- Persist orchestration policy cooldown/session-counter state (best-effort, non-blocking).

---

## Design Goals

1. **Restart-safe**: Calibration confidence, watchdog escalation, and cycle history survive server restarts with no manual intervention.
2. **Non-blocking persistence**: Write-behind pattern — state changes are persisted asynchronously. RSIC cycle latency must not increase.
3. **Backward-compatible**: Servers with no persisted state start clean (same as today). Persisted data is additive.
4. **Multi-space correct**: Watchdog and signal paths work for any `{space_id, session_id}` pair, not just hard-coded defaults.
5. **Minimal schema**: Use existing `MemoryNode` label with a new `role_type` for RSIC state nodes. No new Neo4j indexes required.

---

## Current State (Why This Is Critical)

### Ephemeral State Inventory

| Component | File | State Lost on Restart | Impact |
|-----------|------|----------------------|--------|
| **Calibrator** | `calibration.go` | `actionHistory` (per-action success/failure), `cycleHistory` (last 100 outcomes) | Confidence scores reset to defaults; no learned effectiveness |
| **Watchdog** | `watchdog.go` | `DecayScore`, `EscalationLevel`, `LastCycleTime`, `SessionHealthScore` | Overdue detection forgotten; escalation resets to Nominal |
| **OrchestrationPolicy** | `orchestration_policy.go` | `lastTrigger`, `sessionCounters`, `activeCycles`, `dedupeWindow` | Cooldowns reset; rapid re-triggering possible; session counters lost |
| **SnapshotStore** | `action_snapshot.go` | `snapshots` map (max 50, TTL-based) | All rollback capability lost |
| **Dispatcher** | `task_dispatch.go` | `activeTasks`, `reports` | Task history lost; no cleanup for completed tasks (unbounded growth) |

### Hard-Coded Identity Issues

| Location | Hard-Coded Value | Problem |
|----------|-----------------|---------|
| `server.go` ~L370 | `"mdemg-dev"` in `NewWatchdog(cfg, "mdemg-dev", ...)` | Watchdog only monitors one space |
| `server.go` ~L393 | `"claude-core"` in signal adapter | Signal collection assumes single session |
| `rsic_adapters.go` | String parsing for Neo4j datetime | Type mismatch silently zeroes consolidation-age signal |

---

## Persistence Model

### Storage Strategy

RSIC state is persisted as Neo4j nodes with label `MemoryNode:RSICState`. Each state type uses a distinct `rsic_type` property for querying.

```
(:MemoryNode:RSICState {
    node_id:    "rsic-state-<type>-<scope>",
    space_id:   <space>,
    rsic_type:  "calibration" | "watchdog" | "cycle_history" | "orchestration",
    data:       <JSON string>,
    updated_at: datetime()
})
```

**Why JSON string for `data`?** Neo4j property maps don't support nested structures. A JSON string in a single property avoids schema complexity while keeping queries simple. The RSIC state structures are small (< 10KB each).

### What Gets Persisted (and What Doesn't)

| State | Persist? | Reason |
|-------|----------|--------|
| Calibrator `actionHistory` | **Yes** | Learned effectiveness is the core RSIC value |
| Calibrator `cycleHistory` | **Yes** | Audit trail + feeds calibration scoring |
| Watchdog `state` | **Yes** | Decay/escalation must survive restarts |
| OrchestrationPolicy `lastTrigger` | **Yes** | Prevents rapid re-triggering after restart |
| OrchestrationPolicy `sessionCounters` | **Yes** | Preserves meso-periodic trigger progress |
| OrchestrationPolicy `activeCycles` | No | Short-lived (30min auto-clean); stale on restart anyway |
| OrchestrationPolicy `dedupeWindow` | No | Short TTL (600s); acceptable to lose |
| SnapshotStore `snapshots` | No | Pre-state data is large; TTL-based; rollback is best-effort |
| Dispatcher `activeTasks`/`reports` | No | Per-cycle transient state; add cleanup instead |

### Write-Behind Pattern

```
┌──────────────┐    sync     ┌──────────────┐   async    ┌─────────┐
│  RSIC State  │ ──────────> │  In-Memory   │ ────────>  │  Neo4j  │
│  (mutation)  │             │  (primary)   │  write-    │ (durable)│
└──────────────┘             └──────────────┘  behind    └─────────┘
                                    │
                              on startup
                                    │
                              ┌─────▼─────┐
                              │  Hydrate   │
                              │  from DB   │
                              └───────────┘
```

- **Writes**: State mutations update in-memory first (no latency change). A background goroutine flushes dirty state to Neo4j every 30 seconds.
- **Reads**: Always from in-memory (no Neo4j reads during cycle execution).
- **Startup**: `Hydrate()` loads persisted state from Neo4j into memory before RSIC components accept requests.
- **Failure**: If Neo4j write fails, log warning and retry on next flush. In-memory state is authoritative.

---

## Persistence Store

### New File: `internal/ape/rsic_store.go`

Core persistence layer for RSIC state.

```go
type RSICStore struct {
    driver    neo4j.DriverWithContext
    mu        sync.Mutex
    dirty     map[string]bool   // tracks which state keys need flushing
    flushTick *time.Ticker
    cancel    context.CancelFunc
}

func NewRSICStore(driver neo4j.DriverWithContext) *RSICStore
func (s *RSICStore) Start(ctx context.Context)           // starts flush goroutine
func (s *RSICStore) Stop()                                // stops flush goroutine
func (s *RSICStore) SaveCalibration(spaceID string, history []CycleOutcome, actions map[string][]actionOutcome) error
func (s *RSICStore) LoadCalibration(spaceID string) ([]CycleOutcome, map[string][]actionOutcome, error)
func (s *RSICStore) SaveWatchdogState(spaceID string, state WatchdogState) error
func (s *RSICStore) LoadWatchdogState(spaceID string) (*WatchdogState, error)
func (s *RSICStore) SaveOrchestrationState(triggers []triggerRecord, counters map[string]*sessionCounter) error
func (s *RSICStore) LoadOrchestrationState() ([]triggerRecord, map[string]*sessionCounter, error)
func (s *RSICStore) MarkDirty(key string)
func (s *RSICStore) Flush(ctx context.Context) error      // writes all dirty state to Neo4j
```

### Cypher Patterns

**Upsert state node:**
```cypher
MERGE (s:MemoryNode:RSICState {node_id: $nodeId, space_id: $spaceId})
SET s.rsic_type = $rsicType,
    s.data = $data,
    s.updated_at = datetime()
RETURN s.node_id
```

**Load state node:**
```cypher
MATCH (s:MemoryNode:RSICState {space_id: $spaceId, rsic_type: $rsicType})
RETURN s.data AS data, s.updated_at AS updated_at
ORDER BY s.updated_at DESC
LIMIT 1
```

**Cleanup expired state (called periodically):**
```cypher
MATCH (s:MemoryNode:RSICState)
WHERE s.updated_at < datetime() - duration({days: 30})
DETACH DELETE s
RETURN count(s) AS removed
```

---

## Multi-Space Correctness

### Watchdog Space Configuration

Replace hard-coded `"mdemg-dev"` with a config value:

```go
// config.go — new field
RSICWatchdogSpaceID string  // RSIC_WATCHDOG_SPACE_ID (default: "mdemg-dev")
```

**server.go change:**
```go
// Before (hard-coded):
rsicWatchdog = ape.NewWatchdog(cfg, "mdemg-dev", nil)

// After (configurable):
rsicWatchdog = ape.NewWatchdog(cfg, cfg.RSICWatchdogSpaceID, nil)
```

### Session Identity Scoping

Replace hard-coded `"claude-core"` in signal adapter with the watchdog's configured space context:

```go
// Before (hard-coded in rsicWatchdogSignalAdapter):
resume, _ := convSvc.Resume(ctx, "mdemg-dev", "claude-core", 0)

// After (scoped):
resume, _ := convSvc.Resume(ctx, spaceID, "", 0)  // empty session = aggregate
```

The signal adapter receives `spaceID` from the watchdog, which gets it from config. No session-specific signal collection — watchdog monitors space-level health.

### Neo4j DateTime Fix

**Current bug** in `rsic_adapters.go`:
```go
// Attempts string parsing on a Neo4j native datetime
age, _ := strconv.ParseFloat(val.(string), 64)  // PANICS or returns 0
```

**Fix:**
```go
switch v := val.(type) {
case time.Time:
    age = time.Since(v).Seconds()
case int64:
    age = float64(v)
case float64:
    age = v
case string:
    if t, err := time.Parse(time.RFC3339, v); err == nil {
        age = time.Since(t).Seconds()
    }
}
```

---

## Dispatcher Task Lifecycle Cleanup

### Problem
`activeTasks` map grows unboundedly — completed/failed tasks are never removed.

### Solution
Add a cleanup sweep after each cycle completes. Tasks in terminal state (`completed` or `failed`) older than 10 minutes are evicted.

```go
// task_dispatch.go — new method
func (d *Dispatcher) CleanupStaleTasks(maxAge time.Duration)

// Called from cycle.go after RunCycle completes
c.dispatcher.CleanupStaleTasks(10 * time.Minute)
```

Additionally, cap `activeTasks` at 1000 entries (evict oldest on overflow).

---

## Component Changes

### Calibrator Persistence Integration

**File:** `internal/ape/calibration.go`

- Add `store *RSICStore` field and `SetStore(s *RSICStore)` setter.
- After `UpdateCalibration()`: call `store.MarkDirty("calibration:" + spaceID)`.
- Add `Hydrate(spaceID string)` method: loads persisted history into in-memory maps.
- Export `actionOutcome` type (currently unexported) as `ActionOutcome` for serialization.

### Watchdog Persistence Integration

**File:** `internal/ape/watchdog.go`

- Add `store *RSICStore` field and `SetStore(s *RSICStore)` setter.
- After state mutations (`RecordCycle`, `updateDecay`, escalation changes): call `store.MarkDirty("watchdog:" + spaceID)`.
- Add `Hydrate(state *WatchdogState)` method: overwrites in-memory state with persisted values.
- Remove hard-coded space assumptions (already receives spaceID in constructor).

### OrchestrationPolicy Persistence Integration

**File:** `internal/ape/orchestration_policy.go`

- Add `store *RSICStore` field and `SetStore(s *RSICStore)` setter.
- After `RecordTrigger()` and `IncrementSession()`: call `store.MarkDirty("orchestration")`.
- Add `Hydrate(triggers []triggerRecord, counters map[string]*sessionCounter)` method.
- Export `triggerRecord` and `sessionCounter` types for serialization.

### Server Wiring

**File:** `internal/api/server.go`

- Create `RSICStore` after Neo4j driver init.
- Call `rsicStore.Start(ctx)` to begin flush goroutine.
- Wire store to Calibrator, Watchdog, OrchestrationPolicy via `SetStore()`.
- Call `Hydrate()` on each component after store is wired (before accepting requests).
- Call `rsicStore.Stop()` in `Shutdown()`.
- Replace `"mdemg-dev"` with `cfg.RSICWatchdogSpaceID`.

### Config

**File:** `internal/config/config.go`

- Add `RSICWatchdogSpaceID` field with `RSIC_WATCHDOG_SPACE_ID` env var (default: `"mdemg-dev"`).
- Add `RSICPersistenceEnabled` field with `RSIC_PERSISTENCE_ENABLED` env var (default: `true`). Gate for persistence — when false, behaves exactly as today.

### Adapter Fix

**File:** `internal/api/rsic_adapters.go`

- Fix consolidation-age datetime type assertion to handle `time.Time`, `int64`, `float64`, and `string` types.
- Remove hard-coded `"claude-core"` session ID from signal adapter.

---

## Internal Interfaces and Implementation Plan

### File Dependency Graph

```
config.go (2 new env vars)
    │
    ▼
rsic_store.go (NEW) ◄── neo4j.DriverWithContext
    │
    ├──► calibration.go (Hydrate + MarkDirty)
    ├──► watchdog.go (Hydrate + MarkDirty)
    ├──► orchestration_policy.go (Hydrate + MarkDirty)
    │
    ▼
server.go (wire store, hydrate on startup, stop on shutdown)
    │
    ├──► rsic_adapters.go (datetime fix, session ID fix)
    └──► task_dispatch.go (CleanupStaleTasks)
            │
            ▼
        cycle.go (call CleanupStaleTasks after cycle)
```

### Planned File-Level Changes

| File | Action | Estimated Lines | Changes |
|------|--------|----------------|---------|
| `internal/ape/rsic_store.go` | **Create** | ~250 | RSICStore struct, Start/Stop, Save/Load for 3 state types, Flush, MarkDirty, cleanup |
| `internal/ape/rsic_store_test.go` | **Create** | ~200 | Unit tests for serialization, hydration, flush, cleanup |
| `internal/ape/calibration.go` | Edit | +30 | store field, SetStore, Hydrate, MarkDirty calls, export ActionOutcome |
| `internal/ape/watchdog.go` | Edit | +20 | store field, SetStore, Hydrate, MarkDirty calls |
| `internal/ape/orchestration_policy.go` | Edit | +25 | store field, SetStore, Hydrate, MarkDirty calls, export types |
| `internal/ape/task_dispatch.go` | Edit | +25 | CleanupStaleTasks method, cap at 1000 |
| `internal/ape/cycle.go` | Edit | +3 | Call CleanupStaleTasks after cycle |
| `internal/config/config.go` | Edit | +10 | RSICWatchdogSpaceID, RSICPersistenceEnabled |
| `internal/api/server.go` | Edit | +25 | Create/wire/hydrate RSICStore, configurable watchdog space |
| `internal/api/rsic_adapters.go` | Edit | +15 | DateTime type switch, remove hard-coded session ID |

**Total:** 1 new file (~250 lines), 1 new test file (~200 lines), 8 modified files (~150 lines of changes).

---

## Acceptance Test Package

### Unit Tests (`internal/ape/rsic_store_test.go`)

- `TestRSICStore_SaveLoadCalibration` — round-trip serialize/deserialize
- `TestRSICStore_SaveLoadWatchdogState` — round-trip with all fields
- `TestRSICStore_SaveLoadOrchestration` — triggers + session counters
- `TestRSICStore_MarkDirtyAndFlush` — only dirty keys are written
- `TestRSICStore_FlushErrorDoesNotPanic` — graceful failure
- `TestRSICStore_CleanupExpiredState` — nodes older than 30d removed
- `TestRSICStore_HydrationWithEmptyDB` — returns nil/empty, no error
- `TestCalibrator_HydrateRestoresHistory` — calibration confidence matches pre-restart
- `TestWatchdog_HydrateRestoresDecayScore` — decay and escalation survive
- `TestOrchestrationPolicy_HydrateRestoresCooldowns` — cooldown timestamps honored
- `TestDispatcher_CleanupStaleTasks` — completed tasks evicted after TTL
- `TestDispatcher_TaskCapEviction` — oldest evicted at 1000 cap
- `TestDateTimeAdapter_HandlesAllTypes` — time.Time, int64, float64, string
- `TestDateTimeAdapter_InvalidStringReturnsZero` — graceful fallback

### Integration Tests

- `TestRSICPersistence_SurvivesRestart` — save state, recreate components, hydrate, verify
- `TestRSICPersistence_DisabledByConfig` — `RSIC_PERSISTENCE_ENABLED=false` skips all persistence
- `TestWatchdog_ConfigurableSpaceID` — watchdog uses config value, not hard-coded

### UATS Specs (Contract Tests)

| Spec | Endpoint | Validates |
|------|----------|-----------|
| `self_improve_health_persistence.phase89.uats.json` | `GET /v1/self-improve/health` | `persistence` block present with `enabled`, `last_flush`, `state_nodes` |
| `self_improve_history_persistent.phase89.uats.json` | `GET /v1/self-improve/history` | History returns entries (non-empty after cycle + restart) |

Draft specs will be placed in `docs/api/api-spec/uats/drafts/`.

---

## Health Endpoint Extension

**GET `/v1/self-improve/health`** adds `persistence` block:

```json
{
  "status": "ok",
  "persistence": {
    "enabled": true,
    "last_flush": "2026-02-16T14:30:00Z",
    "state_nodes": 3,
    "dirty_keys": 0,
    "flush_errors": 0
  },
  "safety": { ... },
  "orchestration": { ... },
  "watchdog": { ... }
}
```

---

## Interactive Testing Checklist (Required Before Marking Complete)

- Start server, trigger a cycle, verify calibration state saved (check Neo4j for RSICState nodes).
- Restart server, check `GET /v1/self-improve/health` — verify `persistence.state_nodes > 0`.
- After restart, trigger another cycle — verify calibration confidence reflects pre-restart history.
- After restart, check `GET /v1/self-improve/history` — verify pre-restart cycles appear.
- After restart, verify watchdog decay score is non-zero if time has elapsed.
- Set `RSIC_WATCHDOG_SPACE_ID` to a different space — verify watchdog monitors that space.
- Set `RSIC_PERSISTENCE_ENABLED=false` — verify no RSICState nodes are created.
- Trigger multiple cycles rapidly — verify Dispatcher `activeTasks` stays bounded.
- Check consolidation-age signal in health endpoint after restart — verify non-zero value.

---

## Acceptance Criteria

- [ ] Calibration history (action confidence + cycle outcomes) persists to Neo4j and loads on startup.
- [ ] Watchdog state (decay score, escalation level, last cycle time) persists and loads on startup.
- [ ] Orchestration cooldowns and session counters persist and load on startup.
- [ ] Persistence uses write-behind (async flush every 30s); cycle latency is unchanged.
- [ ] `RSIC_PERSISTENCE_ENABLED=false` disables all persistence (backward-compatible default behavior).
- [ ] `RSIC_WATCHDOG_SPACE_ID` env var replaces hard-coded `"mdemg-dev"` in watchdog init.
- [ ] Hard-coded `"claude-core"` session ID is removed from signal adapter paths.
- [ ] Neo4j datetime type assertion in consolidation-age adapter handles `time.Time`, `int64`, `float64`, `string`.
- [ ] Dispatcher `activeTasks` has lifecycle cleanup (10min TTL for terminal tasks, 1000 cap).
- [ ] Health endpoint includes `persistence` block with flush status and state node count.
- [ ] RSICState nodes older than 30 days are automatically cleaned up.
- [ ] Unit + integration + UATS coverage exists for persistence, hydration, and cleanup.
- [ ] Interactive testing is completed and verified by user before status is set to Complete.

---

## Rollout and Status Policy

Phase 89 status progression:

- `In Review` → design approved.
- `Awaiting Testing` → implementation merged, interactive testing pending.
- `Complete` → only after user-verified interactive behavior.

Current phase status: **Awaiting Testing**.
