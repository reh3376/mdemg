# INSTANCE-ID-HOSTNAME-DRIFT-001 — bug found + mitigation shipped

**Date discovered**: 2026-09-22
**Discovered while**: acknowledging the "wait for T+30d" plan on the 2026-09-21 dashboard review
**Severity**: HIGH (silent data-export loss)
**Status**: MITIGATED (config-only); follow-up sprint scoped

## The bug

`internal/cli/root.go::resolveInstanceID` falls back to `hostname-spaceID` when neither an explicit flag nor `MDEMG_INSTANCE_ID` env is set. This value is written into every TSDB row's `instance_id` column at record time AND used as the filter in `internal/tsdb/exporter.go::exportTable` WHERE clause:

```go
`AND (instance_id = $4 OR instance_id = '')`
```

**If the hostname changes**, the exporter's filter `$4=<new-hostname>-mdemg-dev` matches ZERO of the historical rows tagged with `<old-hostname>-mdemg-dev`. The exporter returns 0 rows for `llm_interactions` and fires the shipped guard at `exporter.go:289`:

> `no llm_interactions data found for space "mdemg-dev" in the specified time range`

Live evidence:
- Hostname change happened between 2026-09-21 11:49:55 UTC (last successful export) and 2026-09-22 11:49:56 UTC (first failure) — approximately 24h window
- 298 rows in the failed window all had `instance_id="Mac.lan-mdemg-dev"` (previous hostname)
- Current `hostname` command returns `reh3376s-MacBook-Pro.local` → resolveInstanceID returns `reh3376s-MacBook-Pro.local-mdemg-dev`
- Direct query with the actual WHERE clause: **0 rows matched**

## Class

This is the **hostname-as-identity anti-pattern** class. Two shipped modules could have prevented it:
- The `MDEMG_INSTANCE_ID` env override (already exists, just wasn't set)
- A one-time-generated persistent instance-id pin (does not exist)

## Mitigation shipped (Option A — 2026-09-22)

`.env` now pins:
```
MDEMG_INSTANCE_ID=Mac.lan-mdemg-dev  # pinned 2026-09-22 after hostname change broke export-auto
```

Verified: manual `MDEMG_INSTANCE_ID=Mac.lan-mdemg-dev mdemg data export-auto --space-id mdemg-dev --keep 30` completes cleanly (11883 rows / 4 tables / 0 privacy violations).

Next scheduled export (~2026-09-23 11:49 UTC) inherits this env from launchd's plist environment → alert will auto-clear.

## Real fix (deferred to follow-up sprint INSTANCE-ID-PIN-001)

Two options, scoped in a future sprint:

**Option 1: Auto-pin on first run** (~2-3h)
- On first server/CLI startup, if `MDEMG_INSTANCE_ID` unset AND no pin-file at `~/.mdemg/instance_id` exists, generate `<hostname>-<space>` and write to the pin-file
- Subsequent runs read the pin-file; hostname changes don't leak
- Backward-compat: pin-file absent → generate from hostname (current behavior) + write it

**Option 2: Widen exporter query** (~30 min)
- Change `AND (instance_id = $4 OR instance_id = '')` to `AND (instance_id = $4 OR instance_id = '' OR instance_id IS NULL)` — but this doesn't fix the hostname-drift class, only handles NULL

**Recommendation**: Ship Option 1. It's the same "durable, structurally-sound long-term solution" the operator's `no-short-term-mlx-patches` rule points at.

## Arch rule proposed (from this incident — not unilaterally pinned)

**Hostname as identity is fragile — persist any hostname-derived identifier to a pin-file at first-use.** Applies to `instance_id`, any per-machine cache keys, any file paths tied to hostname. When a MDEMG process derives an identifier from `os.Hostname()` for durable use (TSDB writes, exported artifacts, config), the first-run value MUST be persisted to `~/.mdemg/<identifier-name>` and subsequent runs MUST prefer the pinned value over re-derivation.

## Documents Accessed

- `internal/cli/data_export_auto.go` — export-auto CLI
- `internal/cli/root.go::resolveInstanceID` — the fragile function
- `internal/tsdb/exporter.go:289` — the guard that fired
- `internal/tsdb/exporter.go:365 exportTable` — the WHERE clause with the instance_id filter
- Live TSDB `llm_interactions` + `scheduled_job_events` — evidence
- `docs/development/dashboard-metrics-review-2026-09-21/report.md` — parent (this bug found during operator directive execution)
