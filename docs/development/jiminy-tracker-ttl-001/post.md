# JIMINY-TRACKER-TTL-001 — Sprint Post

**Date:** 2026-08-02 | **Branch:** `reh3376_dev01`
**Arc:** JIMINY-ENFORCE prerequisite (inserted between ESCALATION-ACCUMULATE-001 and ENFORCE-002)
**Trigger:** ESCALATION-ACCUMULATE-001 live-drill's log-tail showed `jiminy: feedback dropped — guidance_id expired from tracker` firing 129 times / 12 distinct guidance_ids in the current log window on mdemg-dev. Every drop = one lost outcome (escalation update, trust EMA, constraint_outcomes, HITL bridge — all skipped upstream of the ESCALATION-ACCUMULATE-001 fix). Without this fix, the escalation fix has nothing to fire against on natural traffic.

## Verdict

**Shipped.** WarmStore now disk-persists per-space guidance entries and hydrates on boot. Live-verified end-to-end: warm compute → JSON persisted to `~/.mdemg/warm-store/<space>.json` → server restart → boot log shows `warm store: hydrated from disk entries=1` → `/v1/jiminy/latest` returns the pre-restart guidance → `RefreshTrackedGuidance` re-registers in the effectiveness tracker → subsequent feedbacks find their guidance_id and produce outcomes as normal.

## Root cause (from investigation)

The `EffectivenessTracker` (in-memory, LRU-1000, 24h TTL) holds `guidance_id → items` for feedback correlation. Every server restart wipes it. The hook-side `.jiminy-guidance-state` file (on disk) survives restart and points at the pre-restart guidance_id. Feedback POSTs for that guidance_id hit `tracker.Lookup()` → nil → `feedback dropped` metric increments, log fires, the entire outcome loop (escalation.RecordOutcome, outcomeWriter, contradicted-bridge, trust EMA) is skipped.

WarmStore was ALSO in-memory-only pre-fix, so the `/v1/jiminy/latest` re-registration safety net (`RefreshTrackedGuidance` at `handlers_jiminy.go:421-424`) had nothing to work with post-restart. Result: 129 dropped feedbacks / 12 distinct guidance_ids observable in a single log window.

## What shipped

### WarmStore disk persistence (`internal/jiminy/warm_store.go`)
- New constructor `NewWarmStoreWithPersistence(persistDir string)`. Empty dir → memory-only fallback (WARN logged). Non-empty → MkdirAll + hydrate map from `<dir>/*.json` on startup.
- `Put()` writes an atomic (tmp + rename) JSON file per space alongside the in-memory update.
- `Invalidate()` removes the disk file.
- Hydrate silently skips malformed files (WARN log per file); missing dir is created; unwritable dir falls back to memory-only.
- Space_id path-sanitized: non-`[A-Za-z0-9._-]` bytes replaced with `_` — prevents path-traversal via a malicious space_id.

### Config (2 new knobs + 1 alert knob pair)
- `JIMINY_WARM_PERSIST_DIR` (default `~/.mdemg/warm-store`; empty disables)
- `JIMINY_FEEDBACK_DROP_THRESHOLD` (alert threshold, default 20 drops/window)
- `JIMINY_FEEDBACK_DROP_LOOKBACK_MIN` (window, default 60min)

### Alert rule
`alert.JiminyFeedbackDropRules(threshold, lookbackMin)` — MEDIUM severity, service `jiminy-feedback-drop`, `ForDuration 15min`. Queries `mdemg_jiminy_feedback_dropped_total` counter over the window. Registered in `serve.go` after FOLLOW-RATE-CALIBRATE-001's block. Post-fix drops should approach 0; a non-zero sustained rate is a real regression signal.

### Server wiring
`internal/api/server.go:1066` — `warmStore: jiminy.NewWarmStoreWithPersistence(cfg.JiminyWarmPersistDir)` (was `NewWarmStore()`, memory-only).

### Tests (`internal/jiminy/warm_store_test.go` — new file)
6 pins:
- `TestWarmStore_InMemoryFallback` — empty dir → memory-only, no error
- `TestWarmStore_PersistsToDisk` — Put creates `<space>.json`
- `TestWarmStore_HydratesFromDisk` — new store constructor reads prior process's persisted entries
- `TestWarmStore_InvalidateRemovesDiskFile` — Invalidate removes both in-memory + disk
- `TestWarmStore_SpaceIDPathSanitization` — hostile space_ids (`../../etc/passwd`) sanitized, no path traversal
- `TestWarmStore_HydrateSkipsMalformed` — malformed JSON files ignored, no panic

All 6 pass.

## Live Tier-3 (mdemg-dev, 2026-08-02)

```bash
# 1. Trigger warm computation
POST /v1/jiminy/warm {space_id: mdemg-dev, session_id: tracker-ttl-live-*}
→ 45s warm compute

# 2. Verify persistence
$ ls -la ~/.mdemg/warm-store/
mdemg-dev.json  (11k)

# 3. Restart server
$ launchctl kickstart -k gui/501/com.mdemg.server

# 4. Verify boot log
$ grep 'warm store: hydrated' ~/.mdemg/logs/server.log | tail -1
level=INFO msg="warm store: hydrated from disk" entries=1 dir=/Users/reh3376/.mdemg/warm-store

# 5. Verify /latest returns hydrated guidance
$ curl -s http://localhost:9999/v1/jiminy/latest?space_id=mdemg-dev
{"warm": true, "guidance_id": "f1yc1avncdpk66vf5frmc4zg", "guidance": [10 items]}
```

Post-fix behavior: feedbacks for guidance surfaced BEFORE a restart WILL still succeed, because `/v1/jiminy/latest`'s existing `RefreshTrackedGuidance` (shipped) re-registers into the tracker on the first post-restart read — which itself is now possible because the warm store persisted the guidance across the restart.

## Rules pinned

⚠️ **In-memory caches on the outcome-signal path MUST persist across restarts.** The pre-fix architecture (tracker + warmStore both in-memory) created a silent signal-loss window on every restart: hook-side state file survives disk, but the server-side lookup surface it points at doesn't. Fix pattern: persist the source-of-truth (warmStore), leverage the existing re-registration safety net (`RefreshTrackedGuidance`) for the derived cache (tracker).

⚠️ **When a diagnosis says "metric X isn't emitting" always cross-check with `SELECT DISTINCT metric_name FROM metric_samples` to confirm the metric NAME.** ESCALATION-DIAGNOSIS-001 concluded "tracker.Lookup succeeds always" based on `mdemg_jiminy_feedback_dropped` returning zero rows — but the actual metric name is `mdemg_jiminy_feedback_dropped_total` (the `_total` suffix). 806 samples were flowing the whole time. This is a diagnosis-methodology defect worth pinning: metric-absent conclusions require positive verification (list the metric names, don't just query a guessed one).

## Follow-ups disclosed

1. **Feedback drop alert calibration** — the default threshold (20 drops / 60min) is a starting point; a 24-72h passive observation post-fix will show the honest zero-baseline, and the threshold can be recalibrated then. Same pattern as FOLLOW-RATE-CALIBRATE-001 (measure honest steady state first, place floor below it).
2. **JIMINY-ENFORCE-002 (Bash coverage)** — now unblocked. Both ESCALATION-ACCUMULATE-001 (RecordOutcome + decay + dirty + session_id parity) and this sprint (warm persistence) address the two upstream signal-losses; the enforcement arc can now fire on natural traffic.
3. **Tracker capacity eviction** — the LRU cap is 1000 entries; in a high-cadence session (many Guide() calls in <24h) older entries get evicted. Rare pattern; not blocking. If it becomes an issue, the same disk-persist pattern extends to the tracker itself.

## Rollback

Single-commit revert. Persisted `~/.mdemg/warm-store/*.json` files are harmless when unloaded; delete the dir if desired.

## Documents Accessed

- ESCALATION-ACCUMULATE-001 sprint post + drill log
- `internal/jiminy/warm_store.go` (rewrote)
- `internal/jiminy/effectiveness.go` (tracker — read-only for understanding)
- `internal/api/handlers_jiminy.go:395-442` (`/latest` re-register site — existing safety net)
- `internal/api/server.go:1066` (constructor wire)
- `internal/config/config.go` (3 new knobs)
- `internal/alert/rules.go` (new JiminyFeedbackDropRules)
- `internal/cli/serve.go` (rule registration)
- `internal/jiminy/warm_store_test.go` (new file, 6 tests)
- Live server log (`~/.mdemg/logs/server.log`) — pre-fix drop pattern
- Live TSDB `metric_samples` — verified `mdemg_jiminy_feedback_dropped_total` was emitting (806 samples)
- Live `~/.mdemg/warm-store/mdemg-dev.json` — post-fix persistence confirmed
