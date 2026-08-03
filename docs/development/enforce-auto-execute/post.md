# ENFORCE-AUTO-EXECUTE — Sprint Post

**Date:** 2026-08-03 | **Branch:** `reh3376_dev01`
**Parent:** JIMINY-ENFORCE arc-adjacent follow-up (**3/3 — final follow-up shipped**)
**Trigger:** ENFORCE-004-FOLLOWUP shipped the RSIC insight (`enforcement_false_positive_high` recommending `archive_ineffective_constraints`). This sprint adds the AUTO-EXECUTE side: a targeted per-code archive with strict guards (default-off + dry-run + rate limit + protected-space + provenance stamp).

## Verdict

**Shipped default-OFF with dry-run default-TRUE.** New `archive_constraint_by_code` action + executor wired into RSIC's `Dispatcher`. Insight now emits this action with `TargetCode` set. All 5 guard layers verified via 7 pin tests. Skip on empty code / disabled / protected-space / rate-limited returns a shipped no-op deliverable; enabled + not-dry-run + guards-pass mutates via `ArchiveConstraintByCode` (reversible via `MATCH … SET is_archived=false REMOVE archive_reason`).

`adjust_guidance_confidence` for `enforcement_missed_violation_high` insights **NOT** auto-executed — mutation is not cleanly reversible without HITL; insight still fires + surfaces via the shipped alert path.

## What shipped

### E1 — Interface + calibrator method
- `GuidanceCalibrationProvider.ArchiveConstraintByCode(ctx, spaceID, code, reason) (found, archived, error)` — added to interface
- `internal/jiminy/confidence_updater.go`: implementation. Two-step (check-first, then write) so `found=false` cleanly distinguishes "no matching non-archived node" from "SET didn't mutate." Uses `is_archived=true` + `archived_at` + `archive_reason` — parity with the shipped JIMINY-CORPUS-001/-002 tombstone pattern. Reversible.
- `internal/jiminy/service.go`: exposed via `Service.ArchiveConstraintByCode`
- `internal/api/rsic_adapters.go`: adapter method for RSIC consumption
- `internal/ape/task_dispatch_guidance_test.go`: mock extended

### E2 — RSIC action registration
- `AllowedLLMActions` gets `archive_constraint_by_code` (17→18 entries)
- `TestLLMReflector_ValidActions_Has17Entries` bumped 17→18

### E3 — Insight update
- `internal/ape/self_reflect.go`: `enforcement_false_positive_high` now emits `RecommendedAction: "archive_constraint_by_code"` + `TargetCode: code` (was `archive_ineffective_constraints` bulk). `enforcement_missed_violation_high` unchanged (still `adjust_guidance_confidence`; auto-execution deferred).

### E4 — Executor + guards (`internal/ape/task_dispatch.go`)
`executeArchiveConstraintByCode(ctx, spaceID, targetCode)` layers 5 guards in strict order:

1. **Empty TargetCode** → `skipped_no_code` deliverable (no error; shipped bulk path handles code-less insights)
2. **`EnforcementAutoExecuteEnabled=false`** (default) → `skipped_disabled`
3. **Space in `RSICProtectedSpaces`** → `skipped_protected_space` + WARN log
4. **Rate-limit + cooldown** (via `enforceReserveSlot`):
   - Global sliding-window: max `EnforcementAutoExecuteMaxPerHour` (default 3) archives per rolling hour
   - Per-code cooldown: after archiving code X, X is blocked for `EnforcementAutoExecuteCooldownHours` (default 24h)
   - Either fails → `skipped_rate_limited` + WARN log
5. **`EnforcementAutoExecuteDryRun=true`** (default) → INFO log "would archive" + `dry_run_would_archive` deliverable (no mutation)

Only when all 5 pass does the executor call `ArchiveConstraintByCode` with `archive_reason="rsic_enforcement_false_positive_high"`. Success → INFO log + MEDIUM alert `rsic-auto-archive` with reversal instructions in the message body. Failure → returns the error (RSIC retry logic kicks in).

### E5 — Config knobs (4 new)
```
ENFORCEMENT_AUTO_EXECUTE_ENABLED         bool (default: false)  — master switch
ENFORCEMENT_AUTO_EXECUTE_DRY_RUN         bool (default: true)   — log-only when true
ENFORCEMENT_AUTO_EXECUTE_MAX_PER_HOUR    int  (default: 3)      — global sliding window
ENFORCEMENT_AUTO_EXECUTE_COOLDOWN_HOURS  int  (default: 24)     — per-code cooldown
```

### E6 — Dispatcher.SetConfig
New setter on `Dispatcher` — server.go wires cfg immediately after `NewDispatcher`. The executor consults `d.cfg.EnforcementAutoExecute*` at every dispatch (not memoized) so operators can flip flags via env + restart without a rebuild.

### Tests
7 new pins in `internal/ape/enforce_auto_execute_test.go`:
- `TestExecuteArchive_DisabledByDefault` — cfg zero-value → skipped_disabled
- `TestExecuteArchive_EmptyCodeSkipsBeforeGuards` — empty TargetCode → skipped_no_code
- `TestExecuteArchive_ProtectedSpaceSkipsBeforeMutation` — RSICProtectedSpaces guard
- `TestExecuteArchive_DryRunLogs` — dry-run mode returns dry_run_would_archive
- `TestExecuteArchive_RealArchiveOnLiveConfig` — enabled + not dry-run → real archive
- `TestExecuteArchive_PerCodeCooldown` — same-code retry blocks post-archive
- `TestExecuteArchive_GlobalRateLimit` — third distinct code blocks at MaxPerHour=2
- `TestEnforceReserveSlot_SlidingWindowExpires` — stale (>1h) timestamps prune correctly

All pass. Full test sweep + lint clean.

## Live Tier-3

Live smoke NOT run beyond a restart+health-check because:
- Default flags mean the executor NEVER fires on natural traffic (cfg.EnforcementAutoExecuteEnabled=false)
- Force-firing requires: enable flag + accumulate 3+ blocked_false_positive on a real constraint code + wait for RSIC assess+reflect+dispatch cycle
- The unit tests cover the guard sequence + mock-verified the archive call

Boot log confirms the config bindings load correctly (no WARN on any of the 4 new knobs). Real behavior verification will happen the first time an operator flips `ENFORCEMENT_AUTO_EXECUTE_ENABLED=true` on a substrate with real blocked_false_positive accumulation — the dry-run default gives them a safe first observation window before real mutations.

## Rules pinned

⚠️ **Auto-execute of any substrate-mutating RSIC action MUST layer at least 5 guards in strict order** (before the shipped calibrator/executor is called): (1) empty required field → no-op; (2) master enable flag off → no-op; (3) protected-space guard; (4) rate limit + cooldown; (5) dry-run mode → log-only. Any guard's ordering that lets a mutation happen when a preceding guard should have short-circuited is a stealth-mutation bug.

⚠️ **Auto-execute defaults MUST be off + dry-run true.** Even with perfect guard logic, defaulting to enabled + live-mutation would auto-archive on first cycle after upgrade — before the operator has seen the substrate's actual override pattern. Default-off keeps upgrade safe; dry-run-on lets the operator observe what WOULD happen before authorising real archives.

⚠️ **Reversibility check per action**: `archive_ineffective_constraints` / `archive_constraint_by_code` are safe to auto-execute because they set `is_archived=true` (a boolean flag reversible via `SET is_archived=false`). `adjust_guidance_confidence` mutates a scalar node property — reversing requires knowing the pre-change value, which requires either snapshotting or HITL. Deferred `adjust_guidance_confidence` auto-execution accordingly.

⚠️ **`Dispatcher.cfg` reads happen at dispatch time, NOT constructor time.** Operators flipping env vars + restart pick up the new values without a rebuild. Memoizing config at constructor time would create a stealth "restart to reset" trap.

## Not shipped (arc scope complete)

- **`adjust_guidance_confidence` auto-execute** — deferred; reversibility requires HITL. If HITL-CURATION-003 ships with a "confidence-adjust review queue," this becomes trivial.
- **UI toggle for the enforcement auto-execute flags** — CLI + env only for now. UI toggle (Jiminy tab) is a small follow-up.
- **RSIC calibration feedback loop** — currently the executor's dry-run/skipped/archived deliverables land in the RSIC action log but no separate signal feeds back to the reflector. A follow-up could wire "archive succeeded → weaken the pattern's re-fire threshold" so the auto-execute doesn't oscillate.

**JIMINY-ENFORCE arc-adjacent follow-ups: 3/3 shipped.**

## Rollback

Single-commit revert. Any nodes already archived via this path can be reversed with:
```cypher
MATCH (n:MemoryNode {space_id: 'X', archive_reason: 'rsic_enforcement_false_positive_high'})
SET n.is_archived = false REMOVE n.archive_reason, n.archived_at
```
No schema change; audit is via `constraint_overrides` (ENFORCE-OVERRIDES-TSDB) + log lines.

## Documents Accessed

- ENFORCE-004-FOLLOWUP post (parent insight source)
- ENFORCE-OVERRIDES-TSDB + ENFORCE-UI-OVERRIDES posts (arc siblings)
- JIMINY-CORPUS-001/-002 posts (`is_archived` + `archive_reason` tombstone pattern)
- RSIC-LLM-ALERT-GUARD-001 (AllowedLLMActions expansion contract)
- `internal/ape/task_dispatch.go` (dispatch table + executors)
- `internal/ape/types_rsic.go` (GuidanceCalibrationProvider interface)
- `internal/jiminy/confidence_updater.go` (ArchiveStaleConstraints reference)
- `internal/jiminy/service.go` (service wrapper pattern)
- `internal/api/server.go` + `internal/api/rsic_adapters.go` (adapter wiring)
- `internal/config/config.go` (4 new knobs)
- 7 new pin tests + 2 existing test updates (validActions count + insight action assertion)
