# HOOKSYNC-001 — Sprint Close

**Date:** 2026-06-11 · **Branch:** `reh3376_dev01` · **Roadmap:** Q3 Phase 1, rank #2

## What shipped

The channel HOOKWIRE-001 revived is now drift-proof, self-monitoring, and
triageable — and the PORT-TRUTH quick fixes landed.

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Investigation + plan | `b2e7c49` |
| 1 | Bidirectional drift reconciled; alert delivery restored; T1 bootstrap single-sourced | `98c50a7` |
| 2 | CI hook-parity gate (proven red on deliberate drift) | `c27ee50` |
| 3 | Alert Cleared lifecycle: `POST /v1/alerts/clear` + display-then-clear hooks; CUIDv2 ids | `8c7732b` |
| 4 | Absence detection: `POST /v1/hooks/event` heartbeats + `hook_channel_silent` rule | `23fc539` |
| 5 | `mdemg hooks doctor` (11 checks) | `0e1450a` |
| 6 | PORT-TRUTH: loopback bind defaults + 39-day sidecar zombie replaced | `f1711b5` |
| 7–8 | Tier 3 verification + docs | (this) |

## Live highlights

- **The 50-alert backlog drained on real prompts** (10/prompt, no
  re-render) — down to 2 pending during the sprint purely through normal
  hook fires. The NOSILENT last mile is connected.
- **The channel self-reports now:** evaluator `rules=15 → 16` across the
  restart; rule SQL branches proven on the real table; heartbeats landing
  with session metadata + latency.
- **`hooks doctor` 11/11 PASS** on this machine; fails correctly on drift.
- **Sidecar:** fresh process on `127.0.0.1:8101` (was `*:8101`, 39 days
  stale), NLI probe 234ms.

## Findings logged

- The packaging plists are templates (`__HOME__`, `__PYTHON_BIN__`,
  `__PROJECT_DIR__`) — raw copies exit 78 under launchd; `mdemg service
  install` is the canonical substitution path.
- UATS runner inheritance: a falsy variant `body: {}` inherits the base
  request body — variant bodies must be non-empty objects (caught live in
  `alerts_clear`; pinned in both new specs).
- The activity heartbeat undercounts on failure-heavy stretches
  (PostToolUse success-only firing, HOOKWIRE-001) — absorbed by
  `HOOK_ACTIVITY_MIN_EVENTS`.

## UxTS mapping

- **UATS:** 2 new specs (`alerts_clear` 3/3, `hooks_event` 3/3) + full
  suite regression post-change.
- **UVTS/UBENCH:** N/A (no retrieval/LLM surface).
- **UOTS:** still the carried-over follow-up (now also covering the
  `hook_channel_silent` rule's Grafana surface, if/when panelized).

## Follow-ups

- **HOOKSRV-001** (server-side hook orchestration) — deferred per roadmap
  until this stabilizes.
- **STRICT-SCOPE** — global strict-mode file arms all conversations;
  trigger: first multi-session /strict use.
- `.ps1` hook variants — mirror the HOOKWIRE/HOOKSYNC blocks mechanically
  (Windows is WSL2-first; low priority).
- Roadmap Phase 1 next: **HIDDEN-WEIGHT-001** (22,170 NULL-weight
  GENERALIZES edges — 100%, worse than the audit's ~13.2k estimate).

## Documents Accessed

- `.claude/hooks/*` + `internal/cli/hook_templates/*`;
  `internal/cli/hooks.go` / `hooks_doctor.go` / `serve.go` / `job_report.go`
- `internal/alert/` (types, dispatcher, file_backend, rules, evaluator);
  `internal/jobhealth/`; `internal/tsdb/job_events_writer.go`
- `internal/api/handlers_alerts.go` / `handlers_hooks.go` / `server.go`
- `internal/config/config.go`; `.github/workflows/ci.yml`
- `docker-compose.yml` + compose template; `neural/neural_sidecar/config.py`;
  both `com.mdemg.neural-sidecar.plist` copies
- `~/.mdemg/alerts/current.json` (live), `scheduled_job_events` (live),
  server log (evaluator rule count)
