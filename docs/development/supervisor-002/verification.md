# SUPERVISOR-002 — Tier 3 Live Verification

Date: 2026-06-11 · Branch: `reh3376_dev01` · Live stack: native `bin/mdemg`
(LaunchAgent) + Docker Neo4j/TimescaleDB/sidecar/Grafana · Space: `mdemg-dev`

## 1. Worker registration (Epic 2)

Startup log, post-rebuild boot:

```
supervisor: started workers=12        (historical boots: workers=3)
```

12 = 3 original (health-prober, mlx-watchdog, alert-evaluator) + 9 newly
supervised loops enabled in this config: llm-fastfail-burst-flush,
rsic-store-flush, signal-learner-flush, neo4j-backup-scheduler,
tsdb-backup-scheduler, periodic-consolidation, space-prune-scheduler,
rsic-macro-cron, rsic-watchdog. The remaining 3 refactored loops
(context-cooler, weekly-gap-interviews, scheduled-sync) are disabled in this
config (`CONTEXT_COOLER_ENABLED=false`, `WEEKLY_GAP_INTERVIEWS_ENABLED=false`,
`SYNC_INTERVAL_MINUTES=0`) — they register only when enabled.

## 2. Graceful shutdown (no deadlock, dual stop paths)

`launchctl kickstart -k` cycles (multiple during this sprint) drained
cleanly; both exit paths observed:

```
supervisor: worker completed worker=tsdb-backup-scheduler     (stop-channel → nil return)
supervisor: worker completed worker=rsic-macro-cron
supervisor: worker completed worker=signal-learner-flush
supervisor: worker stopped (shutdown) worker=health-prober    (supervisor ctx)
supervisor: worker stopped (shutdown) worker=llm-fastfail-burst-flush
supervisor: worker stopped (shutdown) worker=mlx-watchdog
```

## 3. TSDB-stop drill (Epic 3 meta-alert) — run twice

**Drill 1 caught a real design flaw.** `docker stop mdemg-timescaledb-1` →
failures logged at Warn (was Debug) → but the freshness-window heuristic
misclassified outage ONSET: rules were succeeding seconds before the stop,
so the first 2 rules to reach threshold fired per-rule alerts before the
aggregate `alert-evaluator-degraded` landed at 15:12:13Z. Fixed in its own
commit (`28b0db0`): the discriminator is now streak-relative — per-rule only
when some other rule succeeded AFTER this rule's streak began. Pinned by
`TestRuleFailureStreak_OutageOnsetIsGlobal`.

**Drill 2 (post-fix), stop at 15:15:29Z:**

```
DEGRADED_ALERT_LANDED      (exactly one alert-evaluator-degraded, severity high)
per-rule leaks this drill: 0
TSDB_RESTARTED
```

Recovery: after `docker start`, zero further `alert evaluator: query failed`
lines — rules evaluating normally; meta-alerts re-armed by design
(success path resets streaks).

State restored: TSDB container up and healthy after both drills.

## 4. RSIC recency gate (Epic 4)

Live conditions were ideal: `llm_interactions` still contained the 02:00Z
`jiminy.synthesize` timeout burst (33.3% over 36 calls, last error
02:24:27Z — >12h stale) inside the 24h window that pre-fix produced 5
repeating CRITICAL "Jiminy Pipeline Critical" alerts across the morning.

Post-restart RSIC reflect cycle:

- `Jiminy guidance pipeline critical` since restart: **0** (was every cycle)
- `jiminy.synthesize` spike: absent from alerts (suppressed as stale) ✓
- **Fresh failures still fire**: `alert_llm_health … Task jiminy.evaluate_llm
  error rate 94.9% exceeds 5% (692 calls)` — last error minutes old,
  correctly passed the gate. The gate suppresses stale, not real.

## 5. Live-smoke surprise (own fix-commit `f3f50ad`)

The fresh `jiminy.evaluate_llm` failure in §4 was real: 657 "context
canceled" rows/24h (94.9% error rate). Root cause: the post-tool-observe
hook POSTs `/v1/jiminy/feedback` with `curl --max-time 5`; per-item Tier-2
outcome classification outlives the connection, and the request ctx
cancelled every in-flight LLM call — outcomes silently degraded to the
keyword heuristic (same class as GUIDANCE-SYNTH-001). Fix:
`handleJiminyFeedback` detaches via `context.WithoutCancel` with its own
budget `JIMINY_FEEDBACK_TIMEOUT_MS` (default 60000). Post-fix: zero new
"context canceled" rows; positive confirmation (successful rows) accrues
with normal hook traffic — checked again before push (§7).

## 6. Unit/lint gates

- `go test ./internal/{supervisor,alert,ape,api,tsdb,backup,cli,config}` — all green
- `golangci-lint run ./...` — 0 issues

## 7. Pre-push re-check

`jiminy.evaluate_llm` since the fix restart (15:13Z): zero rows of either
kind — the hook's feedback path (cooldown-gated) hasn't fired yet, and
crucially **zero new "context canceled" rows** (pre-fix cadence was ~27/h).
Positive confirmation (successful classifications) accrues with normal hook
traffic; the now-supervised alert evaluator + the LLM consecutive-failure
alert will surface any residual failure mode without manual checking —
which is the point of this sprint.

## Documents Accessed

- `docs/development/supervisor-002/sprint_plan_supervisor_002.md`, `loop_inventory.md`
- `internal/supervisor/supervisor.go`, `internal/alert/evaluator.go`
- `internal/ape/{watchdog,rsic_store,signal_learner,self_reflect,self_assess}.go`
- `internal/api/{server,handlers_jiminy}.go`, `internal/cli/serve.go`
- `internal/tsdb/{backup,dataset_builder}.go`, `internal/backup/scheduler.go`
- `.claude/hooks/post-tool-observe.py` (feedback `--max-time 5` forensic)
- Live: `~/.mdemg/logs/server.log`, `~/.mdemg/alerts/current.json`,
  TSDB `llm_interactions` (error-burst + cancellation forensics)
