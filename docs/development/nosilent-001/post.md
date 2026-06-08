# NOSILENT-001 — Sprint Close

**Date:** 2026-06-08 · **Branch:** `reh3376_dev01` · **Target:** v0.10.x (additive + one additive migration)

## What shipped

A "no silent failures" guarantee for MDEMG's scheduled/background core jobs. A job that fails — or silently never runs — now produces a TSDB record **and** a high-severity alert surfaced through the operator alert file, instead of a log line nobody reads.

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan | `9c…` (Epic 0) |
| 1 | V0024 `scheduled_job_events` + `RecordJobEvent` writer (schema 23→24) | `df41e9d` |
| 2 | `jobhealth.Report` + wire backup/maintenance/export-auto | `cc67e9d` |
| 3 | `scheduled_job_recent_failure` + `backup_no_recent_success` rules | `4633c01` |
| — | live-caught cooldown-collision fix (distinct services) | `50e8feb` |
| 4 | Docs + close | (this) |

Preceded by the root-cause fix that triggered the sprint: `4cc7608` (`internal/dockerbin` — docker resolvable under the launchd minimal PATH).

## The trigger

A live-discovered silent failure: with `TSDB_BACKUP_ENABLED=true`, the backup scheduler ran every 24h, its `docker compose pg_dump` failing **every time** (docker not on the launchd PATH), surfacing only a buried `slog.Warn`. The docker cause was fixed first; this sprint fixed the *class* — scheduled jobs that fail with no record and no alert.

## Two mechanisms (so absence-of-success is caught too)

1. **Record + alert at the job.** `jobhealth.Report` writes a `scheduled_job_events` row and fires a high-severity alert on failure. Wired into all three jobs (backup via a decoupled scheduler hook + the server dispatcher; the two CLI jobs via a file-backed dispatcher writing the same `current.json` the hooks surface).
2. **Watch from the server.** `scheduled_job_recent_failure` catches "ran and errored"; `backup_no_recent_success` catches "**silently never ran**" by observing absent success — the property a run-and-fail counter can't provide.

## Live testing earned its keep (twice)

- **Induced a real failure**, didn't simulate: a bad-credentials `maintenance` run produced a real failure row + immediate alert; the evaluator independently fired the failure rule; the staleness rule fired for the never-succeeded backup. All observed in the real alert file.
- **Caught a masking bug:** the first Tier-3 run fired only the failure alert — both rules shared `Service="scheduled-jobs"`, and the `(service, severity)` cooldown suppressed the staleness alert. One alarm silencing another is the exact failure class this sprint exists to kill. Fixed (distinct services), re-verified, pinned by a unit assertion.
- **Refused to pollute live data:** the destructive-op guard blocked a fake-success INSERT/DELETE for the "clear" check; the clear path is covered by the Tier-2 query test instead.

## UxTS mapping

- **UATS / UVTS / UBENCH** — N/A (no new HTTP endpoint, retrieval, or LLM surface). The alert rules are validated by Tier-1 + the live Tier-3 evaluator run.
- **UOTS** — the existing alert/Grafana surfaces are unchanged; no new dashboard panel.

## Follow-ups

- **Retry/backoff** for failed jobs (this sprint makes failures *visible*; auto-healing is a separate concern).
- **Generalize** to any future scheduled job — the writer + `reportScheduledJob`/result-hook pattern is reusable; only the three named jobs are wired today.
- **`maintenance`/`export-auto` success metadata** (orphans pruned, rows exported) is recorded in `metadata` but not yet surfaced on a dashboard.

## Documents Accessed

- `docs/development/nosilent-001/sprint_plan_nosilent_001.md`
- `internal/tsdb/model_install_writer.go` + `migrations/021_model_install_events.sql` (writer pattern)
- `internal/alert/dispatcher.go`, `rules.go`, `evaluator.go`, `cooldown.go`
- `internal/tsdb/backup.go`, `internal/cli/maintenance.go`, `internal/cli/data_export_auto.go`, `internal/cli/serve.go`, `internal/api/server.go`
- `internal/config/config.go`, `.github/workflows/ci.yml` (schema-version check)
