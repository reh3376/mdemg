# Scheduled-Job Health (No Silent Failures)

## Why

MDEMG runs background/scheduled core jobs — **TSDB backup** (server scheduler), **maintenance** (decay+prune), and **export-auto** (training export). Before NOSILENT-001, none of them recorded success/failure anywhere, and none raised an alert. A core process could fail indefinitely with the only evidence a log line nobody reads.

That is exactly what happened: with `TSDB_BACKUP_ENABLED=true`, the backup scheduler ran every 24h, its `docker compose pg_dump` failing **every time** (docker not on the launchd PATH), surfacing only a buried `slog.Warn`. The data-protection job was silently broken. A production-ready cognitive substrate cannot have silently failing processes.

## How it works

Two mechanisms, so a failure is caught whether the job *ran and errored* or *silently never ran*:

### 1. Every job run is recorded + alerts on failure

Each scheduled job reports its outcome through `internal/jobhealth.Report`, the single policy point:
- writes one row to the **`scheduled_job_events`** hypertable (V0024): `job_name`, `success`, `latency_ms`, `error_message`, `metadata`, `recorded_at`;
- on failure, fires a **high-severity alert** via the dispatcher.

Wiring per job:
- **TSDB backup** (server): a decoupled result hook on the scheduler → `jobhealth.Report` with the server pool + in-process dispatcher.
- **maintenance / export-auto** (CLI, separate processes): a deferred reporter opens a short-lived pool + a **file-backed dispatcher** writing the same `~/.mdemg/alerts/current.json` the session-start / prompt hooks surface — so a separate-process job still reaches the operator.

### 2. The server watches for failures *and* absence-of-success

Two server-native alert-evaluator rules query `scheduled_job_events`:

| Rule | Fires when | Why it matters |
|---|---|---|
| `scheduled_job_recent_failure` | any job recorded a failure in the last `JOB_FAILURE_LOOKBACK_MIN` | catches a job that ran and errored |
| `backup_no_recent_success` | **zero** successful `tsdb-backup` in the staleness window | **catches a job that silently died or never started** — it fires from the server observing *absent* success, independent of whether the job process ran at all |

The staleness rule is the key guarantee: a counter on "did it run and fail" can't catch "it never ran." Observing absent success can.

The two rules use **distinct alert services** (`scheduled-job-failure` / `scheduled-job-staleness`) so the dispatcher cooldown — keyed on `(service, severity)` — never lets one mask the other. (That masking was itself a silent failure caught during live testing.)

## How an operator sees a failure

- The alert lands in `~/.mdemg/alerts/current.json` (and a macOS notification if `ALERT_MACOS_NOTIFY=true`).
- `session-start.sh` surfaces critical/high alerts; `prompt-context.sh` surfaces all pending alerts. So a failed backup shows up at the next session start, not 24h later in a log.
- Investigate directly: `SELECT job_name, success, error_message, recorded_at FROM scheduled_job_events WHERE success=false ORDER BY recorded_at DESC;`

## Configuration

Every value is config-driven (no hardcoded literals).

| Concern | Env Var | Default |
|---|---|---|
| Master gate for the job rules | `JOB_HEALTH_ALERT_ENABLED` | `true` |
| Backup staleness window (hours) | `JOB_BACKUP_STALENESS_HOURS` | `0` → derive from `TSDB_BACKUP_INTERVAL_HOURS` × 2 |
| Failure lookback (minutes) | `JOB_FAILURE_LOOKBACK_MIN` | `60` |
| Docker CLI path (for the backup's `docker compose`) | `MDEMG_DOCKER_BIN` | auto-resolve (PATH → well-known locations) |

The staleness rule is only active when `TSDB_BACKUP_ENABLED=true` (otherwise "0 successes" is expected, not an incident).

## Scope

The writer + `jobhealth.Report` are reusable for any future scheduled job — wiring a new one is: call `reportScheduledJob(...)` (CLI) or set a result hook (server). Retry/backoff is out of scope; this makes failures *visible*, the operator decides the response.

## Related

- The docker-PATH root cause that triggered this sprint: `internal/dockerbin` + commit `4cc7608`.
- Sprint: `docs/development/nosilent-001/`. Verification: `docs/development/nosilent-001/verification.md`.
