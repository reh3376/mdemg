# Goroutine Supervisor — the Always-On Guarantee

## Why

MDEMG is a cognitive substrate: its memory, learning, alerting, and backup
loops must run for as long as the process does. Before SUPERVISOR-002, only
3 of ~27 long-running goroutines were supervised — a panic in periodic
consolidation, a backup scheduler, or the RSIC watchdog silently killed
that subsystem for the rest of the process lifetime, with no log, no alert,
and no restart. Two compounding mechanics made it worse:

1. The supervisor's restart counter only ever incremented — one transient
   failure a week permanently killed even a *supervised* worker after three
   weeks.
2. Alert-rule query failures were logged at Debug — a rule with broken SQL
   was a silently-disabled alert (bitten twice in one week:
   HIDDEN-WEIGHT-001's rule and the `recorded_at`-column bug).

## What it does

`internal/supervisor` runs named workers with **panic recovery** and a
**sliding-window restart budget**: a worker fails permanently only if it
restarts more than `SUPERVISOR_MAX_RESTARTS` times within
`SUPERVISOR_RESTART_WINDOW_MIN` minutes. Restarts older than the window are
forgotten, so occasional transients never accumulate. Each restart fires a
medium alert; permanent failure fires a critical alert. Backoff doubles per
in-window restart (capped at 2 minutes).

**Worker semantics**: a worker is a blocking `func(ctx) error`.

- panic or non-nil error → supervised restart (budget permitting)
- nil return without ctx cancellation → intentional completion (e.g. the
  loop's own stop channel closed) — NOT restarted
- supervisor ctx cancelled → shutdown, clean exit

**Late registration** (`Go(name, fn)`): the API server starts most loops
after the supervisor is already running; `Go` registers and launches
immediately. The supervisor outlives permanently-failed workers so late
workers stay supervised.

### Supervised loops (15 when fully enabled, 16 with the FT loop)

health-prober, mlx-watchdog, alert-evaluator (original 3) +
periodic-consolidation, context-cooler, space-prune-scheduler,
weekly-gap-interviews, scheduled-sync, rsic-macro-cron, rsic-watchdog,
rsic-store-flush, signal-learner-flush, neo4j-backup-scheduler,
tsdb-backup-scheduler, llm-fastfail-burst-flush (SUPERVISOR-002).
ft-loop-controller joins as a conditional 16th when `FT_LOOP_ENABLED=true`
(FT-RECURSIVE-002; `server.go::goSupervised("ft-loop-controller", …)`).

The buffered TSDB event writers are deliberately NOT supervised here —
their failure mode is flush-error and their observability is
TSDB-CONSUME-001 scope.

### Rule-health meta-alert (who watches the watcher)

Alert-rule query failures now log at **Warn**, and after
`ALERT_RULE_FAILURE_THRESHOLD` (default 3) consecutive failures a
high-severity meta-alert fires **directly via the dispatcher** (never via a
rule — the meta-channel must not depend on the failing mechanism):

- **One rule failing while peers succeed** (broken SQL class) →
  `rule-health-<rule-id>` alert naming the rule and the error.
- **Nothing succeeding since the streak began** (TSDB-level outage) → a
  single `alert-evaluator-degraded` alert per outage, instead of a per-rule
  storm duplicating the health prober's signal. The discriminator is
  streak-relative ("did any peer succeed after this rule started
  failing?"), not wall-clock freshness — at outage onset the last success
  is always recent, which a freshness window misclassifies (caught live in
  the Tier 3 TSDB-stop drill).

Success re-arms both alert types.

### RSIC LLM-health recency gate

RSIC insight 26 (`llm_error_rate_spike`, escalatable to "Jiminy Pipeline
Critical") evaluates a 24-hour error-rate window. Without a recency
requirement, a transient burst kept it alarming for up to a day after
self-resolving (observed live: 5 false CRITICALs across one morning). The
insight now fires only when the most recent error is within
`RSIC_LLM_ERROR_RECENCY_MIN` minutes (default 60; `0` restores legacy
behavior). The gate suppresses stale spikes only — fresh, ongoing failures
fire exactly as before.

## Configuration

| Env | Default | Meaning |
|---|---|---|
| `SUPERVISOR_MAX_RESTARTS` | 3 | restarts allowed within the window before permanent failure |
| `SUPERVISOR_RESTART_WINDOW_MIN` | 60 | sliding restart-budget window (minutes) |
| `SUPERVISOR_BACKOFF_BASE_SEC` | 5 | base restart backoff, doubles per in-window restart |
| `ALERT_RULE_FAILURE_THRESHOLD` | 3 | consecutive rule-query failures before the meta-alert |
| `RSIC_LLM_ERROR_RECENCY_MIN` | 60 | freshness required for the LLM error-rate insight (0 = off) |

## How to observe

- Startup: `supervisor: started workers=N` (+ `worker registered (late)`).
- Restarts: `supervisor: <name> restarted (k/N in window)` + medium alert.
- Permanent failure: critical alert `Worker failed permanently`.
- Rule health: `~/.mdemg/alerts/current.json` entries with service
  `rule-health-<rule-id>` or `alert-evaluator-degraded`.

Sprint: `docs/development/supervisor-002/` (plan, loop inventory, live
verification incl. two TSDB-stop drills).
