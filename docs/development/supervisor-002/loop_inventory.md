# SUPERVISOR-002 Epic 0 — Background Loop Inventory (audited 2026-06-11)

Authoritative inventory of long-running goroutines in the mdemg server
process. Audit method: exhaustive sweep of `go func(` / `time.NewTicker` /
`for { select` across `internal/`, cross-referenced against
`supervisor.Register` call sites.

## Supervised today (3)

| Worker | Launch site | Shutdown |
|---|---|---|
| health-prober | `internal/cli/serve.go:424` | ctx |
| mlx-watchdog | `internal/cli/serve.go:435` | ctx |
| alert-evaluator | `internal/cli/serve.go:472` | `Stop()` → stopCh |

## In scope for SUPERVISOR-002 (12 — no panic recovery, silent death today)

| # | Worker | Launch site | Owner | Shutdown today |
|---|---|---|---|---|
| 1 | periodic-consolidation | `internal/api/server.go:1856` | Server | stopConsolidate + bgWg |
| 2 | context-cooler | `internal/api/server.go:1921` | Server | stopCooler + bgWg |
| 3 | space-prune-scheduler | `internal/api/server.go:1977` | Server | stopSpacePrune + bgWg |
| 4 | weekly-gap-interviews | `internal/api/server.go:2014` | Server | stopInterviewer + bgWg |
| 5 | scheduled-sync | `internal/api/server.go:2064` | Server | stopScheduledSync + bgWg |
| 6 | rsic-macro-cron | `internal/api/server.go:1654` | Server | macroCronCancel ctx |
| 7 | rsic-watchdog | `internal/ape/watchdog.go:59` | ape | w.ctx + wg |
| 8 | rsic-store-flush | `internal/ape/rsic_store.go:64` | ape | ctx |
| 9 | signal-learner-flush | `internal/ape/signal_learner.go:287` | ape | ctx |
| 10 | neo4j-backup-scheduler | `internal/backup/scheduler.go:36` | backup | stopCh |
| 11 | tsdb-backup-scheduler | `internal/tsdb/backup.go:360` | tsdb | stopCh |
| 12 | llm-fastfail-burst-flush | `internal/cli/serve.go:382` | serve | process lifetime |

## Out of scope — deferred to TSDB-CONSUME-001 (disclosed in plan §3)

Buffered TSDB event writers sharing the close(done)/30s-flush pattern:
llm-interaction, retrieval-audit, retrieval-event, embedding-event,
sparse-gate, constraint-outcomes, reinforcement-events, llm-endpoint-health,
metric-sample writers; plus metrics-recorder auto-flush and Jiminy trust
persistence. Failure mode is flush-error (logged; partially jobhealth-wired),
and flush observability is TSDB-CONSUME-001's deliverable
("FlushStats → Prometheus + flush-failure alert rule").

Bootstrap one-shots (live-collectors initial assessment, Jiminy bootstrap
codification) are not loops and are excluded.

## Special findings driving the sprint

1. **Restart budget never replenishes** — `internal/supervisor/supervisor.go:96`
   increments `w.restarts` unconditionally; nothing decrements. One transient
   per week permanently kills a worker after 3 weeks.
2. **Evaluator query failures are Debug-only** — `internal/alert/evaluator.go:109`.
   No meta-alert exists; a rule with broken SQL is silently disabled forever.
   Bitten twice in one week (HIDDEN-WEIGHT-001 rule; `recorded_at` column bug).
3. **RSIC insight 26 stale-window false criticals** — `internal/ape/self_assess.go:219`
   hardcodes a 24h error-rate window with no recency requirement; a 35-min
   timeout burst at 02:00 UTC 2026-06-11 produced 5 repeating CRITICAL
   "Jiminy Pipeline Critical" + HIGH "LLM error rate spike" alerts over the
   following 12 hours, all on data from an incident that had self-resolved.
