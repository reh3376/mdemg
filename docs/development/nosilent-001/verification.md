# NOSILENT-001 — Live Verification (Tier 3)

**Date:** 2026-06-08
**Stack:** native `mdemg serve` (launchd, rebuilt from this branch) + Docker (Neo4j + TimescaleDB :5433) + llama-server :8102. Space `mdemg-dev`.

Per the directive — *we cannot build a prod-ready framework with silently failing processes* — every guarantee below was exercised against the real running server, observing real alerts.

## Acceptance bar: a scheduled job that fails — or never runs — is LOUD (recorded + alerted), not a silent log line.

### Tier 1 (unit, `-race`)
- `internal/tsdb/job_events_writer_test.go` — writer field mapping, optional-nulls, error truncation, nil-pool no-op, insert-error propagation.
- `internal/jobhealth/jobhealth_test.go` — `Report` fires an alert only on failure (real file-backend dispatcher), nil-safe.
- `internal/alert/job_rules_test.go` — failure rule always present (gt 0), staleness gated on backups-enabled (lt 0.5), windows from config, non-positive fallback, **distinct services** (cooldown-collision guard).

### Tier 2 (integration, live TSDB)
- `tests/integration/job_events_test.go::TestJobEvents_RoundTrip` — record success + failure → read back; the staleness (recent-success count) + failure (recent-failure count) query shapes both return the expected values. **PASS.** (This also covers the staleness *clear* path: success-count ≥ 1 ⇒ not in breach.)

### Tier 3 (live e2e — real failures, real alerts)

**1. CLI job failure → immediate record + alert.** Induced a real `maintenance` failure (bad Neo4j credentials):
```
$ mdemg maintenance --space-id mdemg-dev   (NEO4J_PASS=wrongpassword)
INFO alert: dispatching service=scheduled-job severity=high title="Scheduled job failed: maintenance"
Error: neo4j connectivity: Neo.ClientError.Security.Unauthorized
```
→ `scheduled_job_events` row: `maintenance | success=f | "neo4j connectivity: …"`.
→ Alert in `~/.mdemg/alerts/current.json`: `high | Scheduled job failed: maintenance | …neo4j connectivity…`.
The separate-process CLI job alerted the operator via the same alert file the hooks surface.

**2. Evaluator failure rule fires independently.** 30s after the failure row, the running server's evaluator detected it on its own:
```
15:04:40 alert: dispatching service=scheduled-job-failure severity=high title="Scheduled Job Recently Failed"
```

**3. Evaluator staleness rule fires — the "job never ran" guarantee.** Because the TSDB backup had been failing (docker-under-launchd) and the scheduler hadn't logged a single successful `tsdb-backup`, the staleness query returned 0 successes:
```
15:04:40 alert: dispatching service=scheduled-job-staleness severity=high title="No Successful TSDB Backup In Window"
```
This is the crucial property: the alert fires from the server observing **absent success**, so a job that silently died or never started is caught — not only one that ran and errored.

Both evaluator alerts landed as **distinct** entries in the alert file:
```
scheduled-job-staleness | high | No Successful TSDB Backup In Window
scheduled-job-failure   | high | Scheduled Job Recently Failed
```

## Surprise bug caught live (own fix, same epic)

The first Tier-3 run fired only the failure alert — the staleness alert was **missing**. Root cause: both evaluator rules used `Service="scheduled-jobs"`, and the dispatcher's cooldown key is `(Service, Severity)`, so the failure alert's cooldown **suppressed** the staleness alert. Exactly the silent-failure class this sprint exists to kill — one alarm masking another. Fixed by giving each rule a distinct service (`scheduled-job-failure` / `scheduled-job-staleness`); re-verified both fire independently (above). Pinned by a Tier-1 assertion that the two services differ.

## Acceptance criteria — met
1. ✅ All three jobs record a `scheduled_job_events` row (export-auto success live; maintenance failure live; backup via the wired hook).
2. ✅ Failure fires a high-severity alert immediately (CLI file-backend + in-process for backup).
3. ✅ `scheduled_job_recent_failure` fires through the live evaluator on a real failure row.
4. ✅ `backup_no_recent_success` fires when no successful backup exists in the window — the never-ran guarantee — through the live evaluator.
5. ✅ The two alerts surface independently (cooldown-collision fixed).
6. ✅ Thresholds/windows config-driven; staleness derives from backup interval × 2.
7. ✅ No live-DB pollution — the destructive-op guard blocked a fake-success INSERT/DELETE; the clear path is covered by Tier 2 instead.

## Conclusion
NOSILENT-001's bar is met and live-verified. A scheduled job that fails, or silently never runs, now produces a TSDB record **and** a high-severity alert surfaced through the operator alert file — the exact failure mode (a backup failing every 24h with only a buried `slog.Warn`) that triggered this sprint is now loud.
