# Sprint Plan — SUPERVISOR-002: Always-On Guarantee for Background Loops

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | SUPERVISOR-002 |
| Sprint line | `docs/development/supervisor-002/` |
| Date opened | 2026-06-11 |
| Branch | `reh3376_dev01` |
| Roadmap slot | Q3 Phase 2 — first of the committed "next in line" members |
| Estimated effort | 3 dev-days |
| OpenAI spend | $0 |
| Risk level | Medium (touches every background loop's launch site; regression risk is shutdown deadlocks) |

## 2. Problem Statement

The mdemg server is an always-on cognitive substrate, but its always-on guarantee
is fiction for most of the process:

1. **Only 3 of ~27 long-running loops are supervised.** `serve.go` registers
   health-prober, mlx-watchdog, and alert-evaluator with `internal/supervisor`;
   everything else — periodic consolidation, context cooler, space prune,
   gap interviews, scheduled sync, RSIC macro cron, RSIC watchdog, RSIC store
   flush, signal-learner flush, both backup schedulers, the fast-fail burst
   flush — is a bare `go func()` with **no panic recovery**. A panic in any of
   them silently kills that subsystem for the remainder of the process
   lifetime. MAINT-LIVE-001's class of bug ("the schedule never ran, for the
   project's entire history") becomes possible again the moment one of these
   loops dies.
2. **The supervisor's restart budget never replenishes.**
   `supervisor.go:96` increments `w.restarts` unconditionally and nothing ever
   decrements it — a worker that hits one transient per week is permanently
   killed after 3 weeks, with one critical alert as its obituary.
3. **Alert-rule query failures are Debug-only** (`evaluator.go:109`). A rule
   whose SQL errors (wrong column name, dropped table) is silently disabled
   forever. This has now bitten twice in one week: the HIDDEN-WEIGHT-001
   `null_weight_abstraction_edges` rule and the `recorded_at` column bug —
   both discovered by accident during later sprints, not by the system.
4. **RSIC LLM-health insights fire on stale data** (discovered live
   2026-06-11): insight 26 (`llm_error_rate_spike`) evaluates a hardcoded
   24-hour error-rate window (`self_assess.go:219`) with no recency
   requirement. A 35-minute timeout burst at 02:00 UTC produced repeating
   CRITICAL "Jiminy Pipeline Critical" + HIGH "LLM error rate spike" alerts
   for 12+ hours after the incident self-resolved. False criticals train the
   operator to ignore the alert channel — the opposite of NOSILENT.

## 3. Scope & Constraints

**In scope**
- Supervisor: sliding-window restart budget (replenishing), late registration
  (`Go(name, fn)`), config-driven knobs.
- Register the 12 unsupervised *scheduler/loop* goroutines (Categories A/B/C/G
  of the inventory) under the supervisor.
- Evaluator: per-rule consecutive-failure tracking, Warn-level logging, and a
  direct meta-alert when a rule fails N consecutive evaluations.
- RSIC: recency gate on `llm_error_rate_spike` (insight 26) via a new
  `LastErrorAt` field on `LLMPerformanceSummary`.
- 3-tier testing incl. live Tier 3 (TSDB-outage drill + restart evidence).

**Out of scope (disclosed)**
- The 9 buffered TSDB writers (Category D) + metrics recorder + Jiminy trust
  persistence (Categories E/F). Their failure mode is flush-error (already
  logged, partially job-health-wired), and flush observability is explicitly
  TSDB-CONSUME-001 scope ("FlushStats → Prometheus + flush-failure alert
  rule"). Supervising their goroutines without that telemetry would be
  motion without observability.
- Insight 25 (latency regression) recency gating — it compares against a
  7-day trend, which is inherently smoothed; only the spike insight (26)
  has the stale-window false-positive mechanism.
- RSIC LLM-reflector action whitelisting (the local model can still
  *recommend* `alert_jiminy_critical`; with insight 26 recency-gated the
  report no longer presents a stale spike, which removes the trigger).

**Constraints**
- Sequential epics; no hardcoded values (all new knobs env-driven with
  sensible defaults); CUIDv2 for any new identifiers; lint before commit;
  surprise live-smoke bugs get their own fix commit.
- Shutdown behavior must remain clean: `bgWg.Wait()` semantics preserved —
  no supervisor-induced deadlocks on `Shutdown()`.

## 4. Dependencies

- `internal/supervisor/` (SR-001/SNA-001 — exists, 153 lines).
- `internal/alert/` dispatcher + evaluator (NOSILENT-001 cooldown semantics:
  distinct `Service` per rule class).
- Loop inventory (this sprint's Epic 0 forensic, agent-audited 2026-06-11).
- `internal/tsdb/dataset_builder.go::LLMPerformance` (insight 26's data feed).
- No external/operator dependencies; no migrations.

## 5. Implementation Plan (sequential epics)

**Epic 0 — Sprint plan + loop inventory committed** (this document +
`loop_inventory.md`). Gate: plan committed.

**Epic 1 — Supervisor core: replenishing budget + late registration**
- Replace `restarts int` with a timestamped restart history; prune entries
  older than the window on each restart. Permanent failure only when
  restarts-within-window exceed the budget. Backoff exponent derives from
  the in-window count (so a worker that has been healthy for a window
  restarts fast again).
- `Go(name, fn)` — register *and start* a worker under supervision after
  `Start()` has been called (the API server starts its loops after the
  supervisor is already running). Idempotent shutdown via the existing ctx.
- Injectable clock (`now func() time.Time`) for deterministic tests.
- Config (new, `FromEnv`): `SUPERVISOR_MAX_RESTARTS` (default 3),
  `SUPERVISOR_RESTART_WINDOW_MIN` (default 60), `SUPERVISOR_BACKOFF_BASE_SEC`
  (default 5). Zero/negative → defaults with warning (DH-005 pattern).
- Tier 1: window replenishment, permanent-fail within window, post-window
  recovery, `Go` after `Start`, alert emission.
- Gate: `go test ./internal/supervisor/` green.

**Epic 2 — Register the 12 loops**
Mechanism (proposed; final pick disclosed in PR): inject a
`Supervise(name string, fn func(ctx context.Context) error)` hook into the
loop owners; default (nil hook) = current bare-goroutine behavior so unit
tests and non-server callers are unchanged. `serve.go` wires the hook to
`supervisor.Go`.
- `internal/api/server.go` Category A loops (6): periodic-consolidation,
  context-cooler, space-prune-scheduler, weekly-gap-interviews,
  scheduled-sync, macro-cron. Loop bodies become blocking
  `func(ctx) error`; existing stop channels remain the graceful path,
  supervisor ctx is the crash/shutdown path. `bgWg` accounting preserved.
- `internal/ape/` (3): rsic-watchdog, rsic-store-flush, signal-learner-flush.
- Backup schedulers (2): `internal/backup/scheduler.go`,
  `internal/tsdb/backup.go`.
- `serve.go` burst-flush (1): direct `supervisor.Go`.
- A loop that returns nil (graceful stop-channel exit) must NOT be
  restarted — supervisor treats nil-return-without-ctx-cancel as
  intentional completion (new semantics, unit-tested).
- Tier 1: each refactored launch site keeps its package tests green.
- Gate: `go build ./...` + full `go test ./internal/...` green; worker
  count visible in startup log.

**Epic 3 — Evaluator meta-alert (Debug → Warn + alert)**
- Per-rule `consecutiveFailures` counter in `ruleState`; reset on success.
- Failure log promoted to `Warn` with rule ID + error.
- On reaching `ALERT_RULE_FAILURE_THRESHOLD` (new config, default 3)
  consecutive failures: dispatch a high-severity alert directly (NOT via a
  rule — the meta-channel must not depend on the failing mechanism).
  Service label `alert-evaluator-rule-health` (distinct per NOSILENT
  cooldown semantics); message carries rule ID + last error. Fire once per
  failure streak (re-arm on success).
- Tier 1: threshold fire, re-arm, no-fire below threshold, success reset.
- Gate: `go test ./internal/alert/` green.

**Epic 4 — RSIC stale-window recency gate (insight 26)**
- `LLMPerformanceSummary` gains `LastErrorAt time.Time` (max error-row time
  in window; zero when no errors) — single query change in
  `dataset_builder.go`.
- Insight 26 fires only when `now − LastErrorAt ≤ RSIC_LLM_ERROR_RECENCY_MIN`
  (new config, default 60; `0` disables the gate = legacy behavior).
- Tier 1: stale spike suppressed, fresh spike fires, zero-time handled,
  gate-disabled passthrough.
- Gate: `go test ./internal/ape/ ./internal/tsdb/` green.

**Epic 5 — Tier 3 live verification**
- Rebuild `bin/mdemg`, `launchctl kickstart -k`; startup log shows
  supervisor worker count ≥ 15 (3 existing + 12 new).
- **Evaluator drill**: `docker stop mdemg-timescaledb-1` → within
  ~3 evaluation intervals the `alert-evaluator-rule-health` alert lands in
  `~/.mdemg/alerts/current.json` → `docker start` → rules recover, meta-alert
  re-arms. (Restore state after — memory rule.)
- **Recency-gate evidence**: live `llm_interactions` still contains last
  night's 02:00 UTC error burst inside 24h → RSIC micro cycle no longer
  emits `llm_error_rate_spike` / Jiminy-critical (server log + alert file).
- Graceful shutdown drill: server stop completes without deadlock; all
  workers log clean exit.
- Gate: all observations recorded in `verification.md`.

**Epic 6 — Documentation (final epic — never cut)**
- `docs/features/goroutine-supervisor.md` (new/expanded feature doc: Why /
  Choices / How it works / How to use, incl. restart-budget semantics and
  the rule-health meta-alert).
- CHANGELOG, CLAUDE.md architecture note, sprint `post.md`.

## 6. Testing Plan

- **Tier 1 (unit)**: supervisor window/replenish/Go/nil-return semantics
  (injected clock — no real sleeps); evaluator failure-streak machine;
  insight-26 recency gate; per-package suites for every refactored launch
  site.
- **Tier 2 (integration)**: existing `tests/integration` suites must stay
  green (consolidation, backup, RSIC paths exercise the refactored loops);
  supervisor + evaluator wired together against a real pgx pool where the
  integration env provides one.
- **Tier 3 (live e2e)**: Epic 5 — real binary, real Docker services, real
  alert file + server log + TSDB rows observed. Includes a destructive-ish
  drill (TSDB container stop) with explicit state restoration.

## 7. Commit Strategy

One commit per epic (conventional commits, `feat(supervisor-002)`/
`fix(...)`); live-smoke surprises get their own `fix:` commits; docs commit
last; push once at sprint end (auto-PR).

## 8. Verification Checklist

- [ ] Supervisor restart budget replenishes (unit-proven, window-pruned)
- [ ] `Go()` supervises post-`Start` workers; nil-return = no restart
- [ ] 12 loops registered; startup log shows ≥15 workers
- [ ] Evaluator failure → Warn log + meta-alert at threshold; re-arms
- [ ] Insight 26 suppressed on stale errors, fires on fresh
- [ ] live smoke: stop TSDB container, observe `alert-evaluator-rule-health`
      in `~/.mdemg/alerts/current.json`, restart TSDB, confirm recovery
- [ ] live smoke: RSIC micro cycle emits no stale-window LLM-health alert
      while last night's burst is still inside the 24h window
- [ ] Graceful shutdown clean (no deadlock, workers exit logged)
- [ ] `golangci-lint run ./...` clean; full `go test ./...` green
- [ ] Feature doc + CHANGELOG + CLAUDE.md + post.md updated

## 9. Documentation Update

Epic 6 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Shutdown deadlock from dual stop paths (stop channel + supervisor ctx) | Medium | High | Loops `select` on both; nil-return-no-restart semantics; graceful-shutdown drill in Epic 5 |
| Restart storm if a loop fails deterministically (e.g., nil pool) | Medium | Medium | Window budget caps at `MAX_RESTARTS`/window with critical alert — same terminal behavior as today, but recoverable after the window |
| Supervising a loop changes its timing (immediate re-run after restart) | Low | Low | Loop bodies re-enter their own ticker cadence; restart only re-arms the ticker |
| Meta-alert flaps when TSDB is briefly slow | Medium | Low | Threshold is *consecutive* failures (default 3 ≈ 90s) + dispatcher cooldown |
| Recency gate hides a genuinely recurring intermittent error | Low | Medium | Default 60-min window still catches anything recurring hourly; `0` disables gate |

## 11. Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q3.md` (SUPERVISOR-002 entry, line 50)
- `internal/supervisor/supervisor.go` (full), `supervisor_test.go`
- `internal/alert/evaluator.go` (full), `internal/alert/rules.go`
- `internal/cli/serve.go` (registration sites 424/435/472; burst flush 382)
- `internal/api/server.go` (Category A loop launch sites 1654–2064)
- `internal/ape/watchdog.go`, `rsic_store.go`, `signal_learner.go`,
  `self_assess.go` (window at 219), `self_reflect.go` (insights 17/26),
  `task_dispatch.go` (executeAlertJiminyCritical, 736)
- `internal/backup/scheduler.go`, `internal/tsdb/backup.go`
- `internal/tsdb/dataset_builder.go` (LLMPerformance, 115)
- Live: `~/.mdemg/logs/server.log`, `~/.mdemg/alerts/current.json`,
  TSDB `llm_interactions` (2026-06-11 error-burst forensic)
- Loop inventory: `docs/development/supervisor-002/loop_inventory.md`

## 12. Rollback Procedures

No migrations, no destructive data ops. Rollback = revert the sprint commits.
The `Supervise` hook defaults to legacy bare-goroutine behavior when unset,
so partial reverts degrade gracefully. The TSDB-stop drill in Epic 5 is
transient by design; the container is restarted and verified in the same
epic.
