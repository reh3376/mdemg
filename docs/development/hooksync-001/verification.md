# HOOKSYNC-001 — Verification (Tiers 1–3)

**Date:** 2026-06-11 · **Stack:** the real Claude Code session (live hooks) +
native `mdemg serve` (rebuilt + restarted per epic) + Docker Neo4j/TSDB +
neural sidecar (launchd). Space `mdemg-dev`.

## Epic 1 — Drift reconcile (live)

All 6 hooks byte-identical to templates modulo `{{SPACE_ID}}`. One prompt now
renders, coexisting: pending-alert block + CMS recall + J17 T1 bootstrap
(`J17:INIT` + ACTIVE CONSTRAINTS — the reverse-drift block nearly lost in
the reconcile, now single-sourced in the template) + guidance + synergy
footer.

## Epic 2 — CI parity gate (proven both directions)

Local run of the exact CI step: exit 0 clean; deliberate drift appended to a
hook → `DRIFT: pre-bash-check.py`, exit 1; reverted.

## Epic 3 — Alert Cleared lifecycle (live, real backlog)

- Prompt 1: `!! MDEMG SERVICE ALERTS [50 pending, showing 10] !!` → file
  shows `cleared: 10/50`.
- Prompt 2: `[40 pending, showing 10]` — the NEXT batch, no re-render →
  `cleared: 20/50`. The backlog drains instead of spamming.
- During later epics the backlog organically drained to 2 pending purely
  through normal hook fires.
- Direct endpoint probes: `{}`→400, no-body→400, bad RFC3339→400,
  unknown-id→200 `cleared:0`. UATS `alerts_clear` 3/3 live (runner
  falsy-body inheritance discovered: variant bodies must be non-empty).
- Tier 1: clear by-id / by-time / idempotent / unknown-id / no-backend.

## Epic 4 — Absence detection (live)

- Real hook fires → `scheduled_job_events` rows: `hook:prompt-context`
  (latency 1000–1200ms, session metadata) + `hook:post-tool-observe`.
- Throttle: second post-tool fire inside the cooldown → still 1 row.
- Rule SQL proven on the real table: positive branch (5 active heartbeats,
  0 monitored) → 1; negative branch (real names, channel alive) → 0.
- **Rule loaded in the running server:** evaluator log
  `rules=15 → rules=16` across the Epic-4 restart.
- Endpoint validation: bad hook name → 400 (`^[a-z0-9][a-z0-9-]{0,63}$`).
  UATS `hooks_event` 3/3 live.

## Epic 5 — hooks doctor (live)

`mdemg hooks doctor --space-id mdemg-dev`: **11/11 PASS** (parity ×6,
registration, healthz, stdin self-test, alert file, heartbeat age — "last
fire 5s ago", fed by the doctor's own self-test). Deliberate drift →
`parity:pre-compact.sh FAIL`, command errors. TSDB-less environment →
heartbeat SKIPs (doesn't fail the run).

## Epic 6 — PORT-TRUTH (live)

- `lsof`: sidecar **127.0.0.1:8101** (was `*:8101`), fresh process started
  2026-06-10 22:15 (replaced the 2026-05-02 zombie — 39 days of stale code).
- `/health`: 200, both models loaded; NLI scoring probe 234ms (within the
  1000ms J17 budget). Server healthz all-ok after.
- Compose copies identical (CI-checked); wide bind now an explicit
  `MDEMG_BIND_ADDR` opt-in.
- Live-smoke note: the packaging plists are templates
  (`__HOME__`/`__PYTHON_BIN__`/`__PROJECT_DIR__`) — raw copy → launchd exit
  78; `mdemg service install` is the canonical substitution path (replicated
  manually here).

## Tier 2 — regression

`go test ./internal/{alert,api,config,cli}` green; lint 0 issues; full UATS
suite run against the live server post-change (see post.md for the count).

## Conclusion

The channel HOOKWIRE-001 revived is now drift-gated in CI, self-monitoring
(heartbeats + the `hook_channel_silent` rule), delivers + clears alerts as
designed, has a one-shot triage command, and the loopback bind defaults
close the off-host exposure. All verified against the real running system.
