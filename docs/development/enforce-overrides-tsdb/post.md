# ENFORCE-OVERRIDES-TSDB — Sprint Post

**Date:** 2026-08-03 | **Branch:** `reh3376_dev01`
**Parent:** JIMINY-ENFORCE arc follow-up (first of 3 remaining post-arc items)
**Trigger:** ENFORCE-003's override JSONL trail is durable + portable but not queryable — RSIC + UI both need SQL access to override history. This sprint migrates to TSDB while keeping the JSONL for forensic/portability.

## Verdict

**Shipped.** V0033 hypertable `constraint_overrides` persists every apply/revoke/expire alongside the JSONL. `DatasetProvider.OverrideHistory` reader added for RSIC + UI consumption. Live-verified: CLI apply → TSDB row lands; CLI revoke → second row lands with `op='revoke'`.

**Bug caught + fixed live:** initial audit wiring caused a deadlock (`audit()` was called under `mu.Lock()` in `Get()`'s lazy-purge branch AND from `mu.Unlock()`'d Apply — the `auditTSDB` helper's `mu.RLock()` deadlocked the lock-held caller). Fixed by removing the mutex from `auditTSDB` (SetTSDB is called once at startup, no runtime toggle → unsynchronised read of pool + spaceID is safe in the shipped call order). Pin test added.

## What shipped

### E1 — Migration V0033 (`internal/tsdb/migrations/033_constraint_overrides.sql`)
Hypertable partitioned on `time` with 7-day chunks (matches `constraint_outcomes` cadence). Columns: `time, space_id, session_id, constraint_code, reason, op, applied_at, expires_at`. Op enum enforced via CHECK constraint (`apply | revoke | expire`). Two indices: `(space_id, constraint_code, time DESC)` for RSIC+UI per-code queries, `(session_id, time DESC)` for session-scoped UI. 180d retention policy.

### E2 — Schema version bump
`TSDB_REQUIRED_SCHEMA_VERSION` default `32→33`. Applied live via `docker exec … psql` + `UPDATE tsdb_schema_meta SET value='33'`.

### E3 — Writer (`internal/jiminy/override.go`)
- `OverrideManager.SetTSDB(pool, defaultSpaceID)` — wired from `server.go::SetTSDBClient` after the pool is up
- `auditTSDB(op, entry)` — INSERT with 2s timeout on each apply/revoke/expire; WARN-log on failure but the override itself still succeeds
- Nil pool = no-op (in-memory + JSONL-only fallback)
- `audit()` now calls BOTH sinks (TSDB first, then JSONL)

### E4 — Reader (`internal/tsdb/dataset_builder.go`)
- `DatasetProvider.OverrideHistory(ctx, spaceID, window) -> []OverrideEvent` — one row per op
- New `OverrideEvent` struct (Time, SessionID, ConstraintCode, Reason, Op, AppliedAt, ExpiresAt)
- ORDER BY time DESC — newest first (matches UI timeline + RSIC "most recent overrides" query)
- `mockDatasetProvider` extended for tests

### E5 — Tests
- Pre-existing 9 OverrideManager tests all still pass (deadlock fix included)
- New `TestOverrideManager_SetTSDBNilPoolIsSafe` — regression pin: SetTSDB with nil pool doesn't crash Apply/Revoke (protects the JSONL-only fallback path)
- Mock provider extension keeps all 8+ pre-existing self_reflect_data tests green

## Live Tier-3 (mdemg-dev, 2026-08-03)

```bash
# Rebuild + restart + apply
$ mdemg jiminy override apply --constraint OVERRIDES-TSDB-LIVE \
    --reason "smoke test TSDB write" --duration 5m
override applied: constraint=OVERRIDES-TSDB-LIVE session=claude-core duration=5m0s

# TSDB row landed
$ psql -tAc "SELECT time, op, constraint_code FROM constraint_overrides
             WHERE constraint_code='OVERRIDES-TSDB-LIVE' ORDER BY time"
2026-08-03 12:48:25.852611+00 | apply  | OVERRIDES-TSDB-LIVE

# Revoke → second row
$ mdemg jiminy override revoke --constraint OVERRIDES-TSDB-LIVE
override revoked: constraint=OVERRIDES-TSDB-LIVE session=claude-core

$ psql ...
2026-08-03 12:48:25.852611+00 | apply  | OVERRIDES-TSDB-LIVE
2026-08-03 12:48:25.944541+00 | revoke | OVERRIDES-TSDB-LIVE
```

Both rows landed. Test session cleaned up post-verification.

## Rules pinned

⚠️ **Best-effort helpers called under a caller's held mutex MUST NOT reacquire the mutex.** `auditTSDB` initially took `mu.RLock()` to read pool + spaceID; `Get()`'s lazy-purge branch calls `audit()` under `mu.Lock()`, deadlocking. Fix: helpers with stable-after-startup fields skip the mutex (`SetTSDB` is called once at server-init; SetTSDB-after-Apply is not a shipped use case). If a helper's config MUST be runtime-mutable, use `atomic.Value` instead of `sync.RWMutex` — the mutex-under-mutex risk exists everywhere audit-like helpers are called under an owning caller's lock.

⚠️ **When migrating a JSONL audit trail to TSDB, keep the JSONL sink.** Portability across `mdemg` instances (import/export, cross-workstation) is easier via a file than an SQL dump; forensic tools can read JSONL without a live DB. The TSDB copy is the queryable + retentioned + partitioned source-of-truth; the JSONL is the durable-across-infrastructure-change record. Both are best-effort — a failure in either sink WARN-logs but the override itself still succeeds.

## Not shipped (arc-adjacent, disclosed)

- **UI display of active overrides + timeline** (next of the 3 items) — Jiminy tab extension reading `OverrideHistory` for timeline + list of active overrides with revoke buttons
- **RSIC action-execution layer** (third item) — consumes `EnforcementOutcomes` + `OverrideHistory` to auto-execute `archive_ineffective_constraints` on chronically-overridden codes (currently RSIC EMITS the insight, but the executor doesn't act on it)
- **Alert rule over `constraint_overrides`** (small follow-up) — "override rate on constraint X exceeds threshold" alongside the existing blocked_false_positive alert (they'd fire together but override rate is a leading indicator; false_positive lags by ForDuration + classifier verdict cycle)

## Rollback

Single-commit revert of Go code + `TSDB_REQUIRED_SCHEMA_VERSION` back to 32. The `constraint_overrides` table + its rows persist harmlessly (no readers = no downstream impact); operator can `DROP TABLE` if desired via `psql`. JSONL audit continues to work.

## Documents Accessed

- ENFORCE-003 sprint post (override manager shape)
- ENFORCE-004-FOLLOWUP post (DatasetProvider extension pattern)
- `internal/tsdb/migrations/030_contradicted_correction_drafts.sql` (template)
- `internal/tsdb/dataset_builder.go` (existing DatasetProvider methods)
- `internal/jiminy/override.go` (extended)
- `internal/jiminy/override_test.go` (+1 test)
- `internal/api/server.go` (SetTSDB wire in SetTSDBClient)
- `internal/config/config.go` (schema version)
- `internal/ape/self_reflect_data_test.go` (mock extension)
- Live TSDB (migration applied, schema version bumped, live drill)
