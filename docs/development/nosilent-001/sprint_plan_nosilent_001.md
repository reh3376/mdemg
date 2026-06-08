# Sprint NOSILENT-001 — Fail-Loud Scheduled Jobs

## 1. Header & Metadata

- **Sprint ID:** NOSILENT-001
- **Sprint line:** `docs/development/nosilent-001/`
- **Date opened:** 2026-06-08
- **Target version:** v0.10.x (additive — new hypertable, writer, alert rules, job wiring)
- **Estimated effort:** ~1 dev-day
- **OpenAI spend:** $0
- **Risk level:** Low–Medium (additive observability; the one risk is wiring the backup scheduler's result hook without changing backup behavior — covered by live Tier-3)
- **Trigger:** A live-discovered silent failure — the TSDB backup scheduler (`TSDB_BACKUP_ENABLED=true`, 24h interval) was running `docker compose pg_dump` under the launchd minimal PATH, failing **every run**, surfacing only a buried `slog.Warn`. Fixed the docker cause (commit `4cc7608`), but the *class* — scheduled jobs that fail with no record and no alert — remains. Operator directive: *"We cannot build a prod-ready framework with silently failing processes."*

## 2. Problem Statement

MDEMG has three scheduled/background core jobs — **TSDB backup** (server scheduler), **maintenance** (decay+prune LaunchAgent), and **export-auto** (training-export LaunchAgent). None of them records success/failure anywhere, and none raises an alert on failure. The server-native alert evaluator has 13 TSDB-query rules but **cannot** detect "a job failed" or "a job never ran," because there is no job-event data to query. A core process can therefore fail indefinitely — as the backup just did — with the only evidence a log line nobody reads. That is not production-ready.

## 3. Scope & Constraints

**In scope:**
- V0024 `scheduled_job_events` hypertable + synchronous `RecordJobEvent` writer (mirrors V0021 `model_install_events`). Schema bump 23→24.
- A shared `jobhealth.Report` helper: record the event **and** fire a high-severity alert on failure (nil-safe on pool/dispatcher).
- Wire the **three** jobs: TSDB backup scheduler (server), `maintenance` (CLI), `export-auto` (CLI). Record success + failure; alert on failure.
- Two new alert-evaluator rules (server-side, the robust catch-all): `backup_no_recent_success` (staleness — fires even if the job never starts) and `scheduled_job_recent_failure`.
- Config-driven thresholds (no-hardcoding): `JOB_HEALTH_ALERT_ENABLED`, `JOB_BACKUP_STALENESS_HOURS` (derived default = backup interval × 2), `JOB_FAILURE_LOOKBACK_MIN`.
- Feature doc, CHANGELOG, CLAUDE.md, verification.md, post.md.

**Out of scope:**
- Re-architecting the schedulers (LaunchAgent → server-internal). The jobs keep their current runners.
- Alerting for the supervisor-managed goroutines (health prober, alert evaluator) — they already have panic-recovery + auto-restart + restart alerts (CLAUDE.md Service Alert System).
- Retry/backoff logic for failed jobs (a separate concern; this sprint makes failures *visible*, not auto-healing).
- A generic job-framework abstraction — only the three named jobs are wired; the writer + helper are reusable for future jobs.

**Constraints:**
- Sequential epics, docs before implementation (memory: `feedback_sequential_epics.md`).
- Live Tier-3 required: induce a real failure, observe a real alert (memory: `feedback_live_testing_required.md`).
- No hardcoding: thresholds + enable flags config-driven with sensible defaults (memory: `feedback_no_hardcoded_values.md`).
- CUIDv2 for event IDs (memory: `feedback_cuidv2_required.md`).
- TSDB migration checklist: bump `TSDB_REQUIRED_SCHEMA_VERSION` (memory: `project_tsdb_schema_version_ci_check.md`).

## 4. Dependencies

- Existing on branch: V0023 (schema 23). Alert dispatcher (`internal/alert/dispatcher.go`, `Send`/`SendAlert`, severities), evaluator + `DefaultRules()` (`internal/alert/rules.go`), `metric_samples` rule pattern. The docker fix (`4cc7608`) so the backup actually runs.
- `internal/tsdb/model_install_writer.go` (V0021) — the synchronous single-row writer pattern to mirror.
- Server holds `s.alertDispatcher` + a TSDB pool; `serve.go` holds `disp` + `tsdbClient.Pool()` (rule registration site); the CLI jobs build their own pool via `tsdb.NewClient` and can construct a file-backend dispatcher from config.

## 5. Implementation Plan

**Epic 0 — Sprint plan (this doc).**

**Epic 1 — V0024 `scheduled_job_events` + writer.**
- `internal/tsdb/migrations/024_scheduled_job_events.sql`: hypertable (`event_id`, `recorded_at`, `job_name`, `space_id`, `instance_id`, `success`, `latency_ms`, `error_message`, `metadata jsonb`), 7-day chunks, indexes `(job_name, recorded_at DESC)` + partial `WHERE success=false` + `(space_id, recorded_at DESC)`. Schema-meta → 24.
- Bump `TSDB_REQUIRED_SCHEMA_VERSION` default 23→24 (config.go).
- `internal/tsdb/job_events_writer.go`: `RecordJobEvent(ctx, pool, JobEventRow) error` (synchronous single-row INSERT, err truncation, nil-safe pool). `JobEventRow{JobName, SpaceID, InstanceID, Success, LatencyMS, ErrorMessage, Metadata map[string]any}`.
- Tier 1: writer field mapping, err truncation, nil-pool no-op, metadata JSON encode. Tier 2: live round-trip insert + read back.

**Epic 2 — `jobhealth.Report` + wire the three jobs.**
- `internal/jobhealth/jobhealth.go`: `Report(ctx, pool, disp, ev JobEvent)` — calls `tsdb.RecordJobEvent` and, when `!ev.Success`, `disp.SendAlert(high)`. Nil-safe on both. One place for the record+alert policy.
- **Backup scheduler** (`internal/tsdb/backup.go`): add an optional `JobResultFunc` hook (`SetResultHook`) called after each scheduled run with (success, latencyMs, err). Wire it in `server.go` (NewServer) to `jobhealth.Report` with the server pool + `s.alertDispatcher`. Keeps `internal/tsdb` decoupled from `internal/alert`.
- **`maintenance`** + **`export-auto`** (CLI): on completion, build a dispatcher from config (file backend) + call `jobhealth.Report` with the command's pool. Record success/failure; alert on failure.

**Epic 3 — Alert evaluator staleness + failure rules.**
- `internal/alert/rules.go`: `JobHealthRules(stalenessHours int, failureLookbackMin int)` returns the two rules:
  - `backup_no_recent_success` — `SELECT count(*) FROM scheduled_job_events WHERE job_name='tsdb-backup' AND success=true AND recorded_at > now() - interval 'N hours'` ; `Operator: lt`, `Threshold: 0.5` (i.e. zero successes → fire). Severity High.
  - `scheduled_job_recent_failure` — `SELECT count(*) FROM scheduled_job_events WHERE success=false AND recorded_at > now() - interval 'M minutes'` ; `Operator: gt`, `Threshold: 0`. Severity High.
- Append in `serve.go` after `DefaultRules()` when `JOB_HEALTH_ALERT_ENABLED`. Thresholds from config.
- New config fields + defaults.

**Epic 4 — Tier 3 live + docs + close.**
- Induce a real backup failure (e.g. point `MDEMG_DOCKER_BIN` at a false path, or temporarily break the compose file) → observe: a `scheduled_job_events` failure row, an immediate alert in `~/.mdemg/alerts/current.json`, and (after the staleness window) the `backup_no_recent_success` rule firing. Restore.
- Feature doc `docs/features/scheduled-job-health.md`; CHANGELOG; CLAUDE.md; verification.md; post.md.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** writer mapping/truncation/nil-pool; `jobhealth.Report` fires alert only on failure (mock dispatcher); rule SQL construction from config; `-race`.
- **Tier 2 (integration, live TSDB):** insert job events → read back; the staleness query returns 0 successes → rule condition true; a failure row → failure rule condition true.
- **Tier 3 (live e2e):** real induced backup failure → real failure row + real alert file entry + staleness rule fires through the running evaluator; success path clears it.

## 7. Commit Strategy

Sequential per epic on `reh3376_dev01`. Surprise live-smoke bugs get their own fix-commit. Final epic updates docs.

## 8. Verification Checklist

- [ ] V0024 applies idempotently; `mdemg tsdb status` → 24; indexes present; `TSDB_REQUIRED_SCHEMA_VERSION`=24; CI schema check green
- [ ] `RecordJobEvent` round-trips (Tier 1 + Tier 2)
- [ ] All three jobs write a `scheduled_job_events` row on success **and** failure
- [ ] Failure fires a high-severity alert (in-process for backup; file-backend for CLI jobs)
- [ ] `backup_no_recent_success` fires when no successful backup in the window — **even if the job process never runs** (the key guarantee)
- [ ] `scheduled_job_recent_failure` fires on any recent failure
- [ ] Thresholds + enable flags config-driven; no hardcoded literals
- [ ] **Live smoke:** induce a real backup failure → observe failure row + alert file entry + staleness rule fire; restore + confirm clears
- [ ] `golangci-lint` clean; full `go test` green
- [ ] Feature doc + CHANGELOG + CLAUDE.md + post.md

## 9. Documentation Update

Epic 4. New `docs/features/scheduled-job-health.md` (Why / How it works / config / how an operator sees a failure). Per the per-feature-docs rule.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Wiring the backup result hook changes backup behavior | Low | Med | Hook is post-run + fire-and-forget; backup path untouched; Tier-3 confirms backups still succeed |
| Alert storm (every 60s) on a persistent failure | Med | Low | Reuse the dispatcher cooldown (default 300s) + rule `ForDuration`; staleness rule fires once per window |
| CLI jobs (separate process) can't reach the in-process dispatcher | Certain | — | They construct a file-backend dispatcher from config → same `current.json` the hooks surface |
| Staleness false-positive right after first deploy (no backups yet) | Med | Low | Window = interval × 2 (configurable); first backup lands within one interval |
| internal/tsdb importing internal/alert creates a cycle | Low | Med | It doesn't — the scheduler exposes a callback hook; `jobhealth` (not `tsdb`) imports `alert` |

## 11. Documents Accessed

- `internal/tsdb/model_install_writer.go` + `migrations/021_model_install_events.sql` (writer pattern)
- `internal/alert/dispatcher.go`, `rules.go`, `evaluator.go` (alert API + rule pattern)
- `internal/tsdb/backup.go` (scheduler failure path), `internal/cli/maintenance.go`, `internal/cli/data_export_auto.go`
- `internal/cli/serve.go` (evaluator registration), `internal/api/server.go` (backup scheduler + dispatcher)
- `internal/config/config.go` (TSDB_REQUIRED_SCHEMA_VERSION, alert config)

## 12. Rollback Procedures

- **Migration:** `DROP TABLE IF EXISTS scheduled_job_events;` + revert `TSDB_REQUIRED_SCHEMA_VERSION` to 23.
- **Rules:** `JOB_HEALTH_ALERT_ENABLED=false` (or revert the serve.go append).
- **Job wiring:** revert the hook commits; jobs return to log-only behavior. No data-path impact.
