# Sprint Plan HOOKSYNC-001 — Drift-Proof + Self-Monitoring Hook Channel

## 1. Header & Metadata

- **Sprint ID:** HOOKSYNC-001 (Roadmap Q3 Phase 1, rank #2)
- **Sprint line:** `docs/development/hooksync-001/`
- **Date opened:** 2026-06-11
- **Branch:** `reh3376_dev01`
- **Target version:** v0.10.x
- **Estimated effort:** ~3 dev-days
- **OpenAI spend:** $0
- **Risk level:** Low-Medium (one new endpoint + evaluator rule; an operational service restart)

## 2. Problem Statement

HOOKWIRE-001 revived the per-prompt channel; this sprint makes it
drift-proof and self-monitoring, and lands the AUTH/PORT-TRUTH quick fixes.
Live-verified findings (2026-06-11):

1. **Bidirectional template↔live drift, alert delivery severed.** The
   installer templates carry alert-display blocks (`prompt-context.sh`:
   all pending; `session-start.sh`: critical/high + degraded-healthz) that
   the live hooks lack — the NOSILENT alert file is actively rotating at its
   50-entry cap **today** (17 rsic, 11 jiminy, 5 graph-health,
   `scheduled-job-staleness` for the never-succeeded backup…) and none of it
   has ever been shown to the operator. No CI check guards hook parity
   (compose + launchd have one; hooks don't).
2. **No Cleared lifecycle.** `Alert.Cleared` exists (`internal/alert/types.go:24`)
   but nothing ever sets it — once displayed, the same entries would
   re-render every prompt forever (noise → numbness). No `/v1/alert*`
   endpoints exist.
3. **No absence detection.** The channel just had a months-long outage that
   only a manual deep-dive caught. Nothing observes "sessions are active but
   the prompt-context hook never fires."
4. **PORT-TRUTH / AUTH rider:** compose template publishes
   `"${MDEMG_PORT}:9999"` on 0.0.0.0 (unauthenticated admin/destructive
   routes exposed off-host); the neural sidecar binds `0.0.0.0:8101`
   (config.py:9 default + launchd plist arg) and the running process is
   **39 days old** (started 2026-05-02 — predates six weeks of J17 sidecar
   fixes; it serves stale code).
5. **No diagnostic surface:** when the channel misbehaves, there is no
   `mdemg hooks doctor` to triage installed-vs-template, registration,
   server reachability, and stdin contract in one shot.

## 3. Scope & Constraints

**In scope:** Epics 1–8 below.
**Out of scope:** server-side hook orchestration (HOOKSRV-001, deferred by
the roadmap); the full auth model (scoped keys — deferred); /strict
multi-session scoping (STRICT-SCOPE, deferred with trigger); PowerShell
hook variants beyond mechanical mirroring of the fixed blocks.

**Design decisions (data-decided, disclosed):**
- **Absence detection reuses `scheduled_job_events` (V0024)** — no new
  hypertable. A tiny `POST /v1/hooks/event` records
  `job_name='hook:<name>'` rows through the existing jobhealth path
  (success=true, latency, metadata carries session_id). The corollary rule
  from EVENTGRAPH-002/-004 applies: don't build a new sink when a fitting
  populated one exists.
- **Evaluator rule `hook_channel_silent`** (distinct `Service` per the
  NOSILENT cooldown-collision rule): fires when conversation observations
  were recorded in the lookback window (sessions demonstrably active) AND
  zero `hook:prompt-context` rows landed — the "job never ran" guarantee
  applied to the delivery channel. Config: `HOOK_HEALTH_ALERT_ENABLED`
  (default true), `HOOK_SILENT_LOOKBACK_HOURS` (default 24).
- **Cleared = delivered-to-operator**, not resolved. The hook clears what it
  displays; conditions that persist re-fire new entries via the evaluator's
  existing ForDuration/cooldown machinery.
- **Bind defaults are config-driven** (no hardcoding): compose
  `"${MDEMG_BIND_ADDR:-127.0.0.1}:${MDEMG_PORT}:9999"`; sidecar
  `MDEMG_SIDECAR_HOST` env honored by config.py with `127.0.0.1` default;
  plists updated in BOTH packaging/launchd + internal/cli/launchd_templates
  (existing CI check covers them).

## 4. Dependencies

HOOKWIRE-001 (merged); V0024 `scheduled_job_events` + `internal/jobhealth`
(NOSILENT-001); alert evaluator + file dispatcher (`internal/alert`);
existing CI template-parity steps in `ci.yml` (pattern to mirror); live
stack for Tier 3.

## 5. Implementation Plan (sequential epics)

- **Epic 0 — Investigation + plan (done):** drift inventory (only
  `prompt-context.sh` 117 lines + `session-start.sh` 25 lines drifted after
  HOOKWIRE; template is the newer side in both), alert file state, zombie
  sidecar identification.
- **Epic 1 — Reconcile live hooks from templates (~0.25d):** adopt the
  template versions of the two drifted hooks into `.claude/hooks/`
  (SPACE_ID substituted) — restores alert display (all-pending per prompt;
  critical/high + degraded-healthz at session start). Templates already
  carry HOOKWIRE fixes; verify byte-parity modulo placeholder afterward.
- **Epic 2 — CI hook-parity gate (~0.25d):** `ci.yml` step mirroring the
  launchd pattern: for each `*.sh`/`*.py` template, `diff` against
  `.claude/hooks/` with `{{SPACE_ID}}`→`mdemg-dev` substituted. Fails the
  Build job on drift.
- **Epic 3 — Alert Cleared lifecycle (~0.5d):** `POST /v1/alerts/clear`
  (body: `{ids: […]}` or `{all_before: <ts>}`) → file-backend
  mark-cleared under its existing lock; hooks POST the displayed entries'
  ids after rendering (fire-and-forget, fail-open). Add `ID` to alert
  entries if absent (CUIDv2). UATS spec for the endpoint.
- **Epic 4 — Absence detection (~0.5d):** `POST /v1/hooks/event`
  (`{hook, session_id, duration_ms, ok}`) → jobhealth record as
  `hook:<name>`; prompt-context fires it in the background each run;
  evaluator rule `hook_channel_silent` (distinct service
  `hook-channel-silent`); config per §3. UATS spec.
- **Epic 5 — `mdemg hooks doctor` (~0.5d):** checks (1) installed hooks vs
  embedded templates (modulo SPACE_ID), (2) hook registration found in
  Claude settings, (3) server `/healthz`, (4) stdin contract self-test
  (pipe real-shape sample payloads, assert expected markers), (5) alert
  file readable + pending count, (6) last `hook:prompt-context` row age.
  Table output, `--json`, non-zero exit on failure.
- **Epic 6 — PORT-TRUTH rider (~0.25d):** compose bind default (both
  compose copies); sidecar `config.py` host default `127.0.0.1`
  (+ `MDEMG_SIDECAR_HOST` env); both plist copies `--host 127.0.0.1`;
  **operational:** reload the sidecar LaunchAgent — replaces the 39-day
  stale process with current code on the loopback bind.
- **Epic 7 — Tier 3 live verification (~0.5d):** real session: alerts
  render on a real prompt then clear (file shows `cleared=true`, no
  re-render next prompt); `hook:prompt-context` rows land in
  `scheduled_job_events`; evaluator rule visible in `/healthz`-adjacent
  status and fires under a forced-silent simulation (temporarily lowered
  lookback) then clears; sidecar serving on `127.0.0.1:8101` with fresh
  start time + J17 probes green; compose lint.
- **Epic 8 — Documentation (final, never cut):** feature doc
  `docs/features/hook-channel-health.md`; CHANGELOG; CLAUDE.md (alert
  delivery + doctor + bind defaults); roadmap tick; post.md.

## 6. Testing Plan (3 tiers)

- **Tier 1:** Go unit tests — alerts clear (file backend lock + idempotent
  re-clear), hooks-event handler validation, evaluator rule SQL + gating,
  doctor check functions; `bash -n`/`py_compile` on hooks; hooks embed
  tests.
- **Tier 2:** UATS specs `alerts_clear.uats.json` + `hooks_event.uats.json`
  (live server); existing UATS suite green; CI parity step proven by a
  deliberate local drift (then reverted).
- **Tier 3 (live):** Epic 7 — the real session and real services, outputs
  observed in the alert file, TSDB rows, evaluator state, and `lsof` bind
  checks.

## 7. Commit Strategy

One commit per epic; live-smoke surprises get standalone fix-commits;
push → auto-PR → sprint summary comment.

## 8. Verification Checklist

- [ ] Live hooks ≡ templates modulo SPACE_ID; CI gate red on deliberate
      drift, green after revert
- [ ] Real prompt renders pending alerts; second prompt does NOT re-render
      them; file entries `cleared=true`
- [ ] `hook:prompt-context` rows in `scheduled_job_events` per real prompt
- [ ] `hook_channel_silent` fires under forced silence + clears after
- [ ] `mdemg hooks doctor` passes on this machine; fails correctly on a
      simulated broken install
- [ ] `lsof`: sidecar on `127.0.0.1:8101`, fresh start time; J17 probe green
- [ ] Compose template binds `127.0.0.1` by default; both copies + CI sync
- [ ] UATS new specs green; lint clean; docs updated

## 9. Documentation Update — Epic 8 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Alert clear races the dispatcher's writes | Medium | Medium | Clear goes through the same file-backend lock as Send; Tier 1 concurrency test |
| Per-prompt rows bloat `scheduled_job_events` | Low | Low | ~100s rows/day worst case; table is a hypertable; retention policy is TSDB-CONSUME-001 scope |
| Sidecar restart trips J17 probes mid-restart | Low | Low | Restart during sprint window; breaker + 1000ms timeout tolerate a blip; verify probes after |
| `127.0.0.1` compose default breaks a remote-access operator | Low | Medium | `MDEMG_BIND_ADDR` env override documented in CHANGELOG + feature doc — explicit opt-in to wide bind |
| evaluator rule false-fires for observation-less-but-active sessions | Low | Low | Rule requires BOTH observations present AND zero hook rows; lookback config-driven |

## 11. Documents Accessed

- `.claude/hooks/*` + `internal/cli/hook_templates/*` (drift inventory)
- `~/.mdemg/alerts/current.json` (live state); `internal/alert/types.go`,
  `evaluator.go`, dispatcher/file backend
- `internal/jobhealth/` + V0024 migration (NOSILENT-001)
- `.github/workflows/ci.yml` (template-parity pattern)
- `internal/cli/compose_templates/docker-compose.yml`;
  `neural/neural_sidecar/config.py`; `com.mdemg.neural-sidecar.plist`
- `docs/development/roadmap/ROADMAP_2026Q3.md` (scope)

## 12. Rollback Procedures

- Hooks/CI/doctor/compose: revert commits.
- Alert clear + hooks-event endpoints: revert — recorded rows are inert.
- Sidecar bind: restore `0.0.0.0` plist arg + `kickstart` (not recommended).
- Cleared alerts are not restorable to uncleared (acceptable: display-state,
  not data).
