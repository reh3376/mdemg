# Scheduled Maintenance — Decay + Prune That Actually Runs (MAINT-LIVE-001)

## Why

The weekly decay+prune LaunchAgent ran for the project's entire history
without ever executing: `mdemg maintenance` defaults `--dry-run=true` (a
safe default for manual use) and the plist passed no override — every
scheduled cycle previewed, reported success, and changed nothing. The
result was unbounded graph growth (Memory Bloat alerts at 79k+ nodes,
tens of thousands of near-zero stale edges) and a hygiene loop that
existed only on paper. NOSILENT-001's job-health rules couldn't see it:
the job *ran*, so neither the failure nor the staleness rule fired.

## Choices

- **The schedule is live; the CLI default stays safe.** The plist passes
  `--dry-run=false`; a human typing `mdemg maintenance` still gets a
  preview unless they opt in. The thing that must not silently no-op is
  the unattended schedule.
- **Dry-run visibility without schema changes.** Maintenance records
  `dry_run` into the job event's `metadata` JSONB (V0024) — the
  `maintenance_no_live_run` evaluator rule filters on it.
- **Only-ever-dry-runs self-reports.** Evaluator rule
  `maintenance_no_live_run` (service `maintenance-liveness`): maintenance
  rows exist in `MAINT_LIVE_LOOKBACK_DAYS` but none ran live → high alert.
  This is the NOSILENT "job never ran" guarantee extended to "the job ran
  but never did anything."
- **Orphan disposition is context-dependent (operator decision,
  2026-06-11).** A uniform degree/age rule conflates governance
  constraints, conversation history, test junk, and hierarchy debris.
  `--exclude-role-types` (env `PRUNE_EXCLUDE_ROLE_TYPES`) makes the policy
  expressible; the shipped schedule excludes
  `constraint,conversation_observation` (constraints are load-bearing
  governance rules at any degree; conversation observations differ by
  session, which a role-level knob can't express). Aged orphan hierarchy
  debris (`concept`/`leaf`/`emergent_concept`) stays eligible — that is
  the lifecycle working as designed.
- **Upgrades propagate the schedule.** `mdemg upgrade` (darwin) re-renders
  already-installed mdemg LaunchAgents from the new binary's embedded
  templates and re-syncs mdemg-managed Claude hooks — refresh-only, never
  installing new services. Without this, plist fixes ship in releases but
  never reach installed machines.

## How it works

Weekly (launchd `StartCalendarInterval`), the agent runs
`mdemg maintenance --space-id <space> --dry-run=false
--exclude-role-types constraint,conversation_observation`:

1. **Decay** — evidence-weighted weight decay across edges (rate 0.02,
   floor protections), now meaningful end-to-end since HIDDEN-WEIGHT-001
   made abstraction-edge weights real.
2. **Edge prune** — DELETEs only `weight < 0.01 AND evidence < 3 AND
   age > 30d` edges (re-learnable via the Hebbian paths).
3. **Orphan tombstoning** — marks (never deletes) `degree ≤ 1` nodes with
   no activity in 90 days, outside abstraction chains, and not in an
   excluded role_type. Tombstones are reversible status flags.
4. The run reports to `scheduled_job_events` with `dry_run` metadata;
   failures alert (NOSILENT-001), dry-run-only patterns alert
   (`maintenance_no_live_run`).

## How to use

```bash
mdemg maintenance --space-id myspace                      # safe preview
mdemg maintenance --space-id myspace --dry-run=false      # execute now
mdemg maintenance --space-id myspace --dry-run=false \
  --exclude-role-types constraint,conversation_observation
mdemg service install                                      # installs the live schedule
mdemg upgrade                                              # refreshes installed plists+hooks (darwin)
```

| Env | Default | Meaning |
|---|---|---|
| `PRUNE_EXCLUDE_ROLE_TYPES` | (empty) | role_type values never tombstoned (CLI flag overrides) |
| `MAINT_LIVE_ALERT_ENABLED` | `true` | enable the only-ever-dry-runs rule |
| `MAINT_LIVE_LOOKBACK_DAYS` | `8` | window requiring ≥1 live run when any maintenance runs exist |

Sprint: `docs/development/maint-live-001/`.
