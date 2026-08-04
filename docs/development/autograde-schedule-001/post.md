# AUTOGRADE-SCHEDULE-001 — Sprint Post

**Date:** 2026-08-04 | **Branch:** `reh3376_dev01`
**Trigger:** Follow-up disclosed by HITL-AUTO-DISMISS-001 + JIMINY-CONTRADICTED-BRIDGE-QUALITY-001. Sprint B added invariant-preserving auto-dismiss (the drain mechanism); Sprint C tightened the source-side gate (reduces accumulation rate). This closes the arc by running the autograder periodically under supervisor so the queue self-drains without operator ceremony — the `hitl-curation` alert no longer requires manual `mdemg review autograde` invocation to clear.

## Verdict

Shipped. New `review.AutogradeScheduler` supervised worker mirrors `ftloop.BenchScheduler` design (nil return = intentional completion when disabled, subprocess-spawned CLI reuses ALL shipped grade-writing logic). Default OFF — operators opt in via `REVIEW_AUTOGRADE_SCHEDULE_ENABLED=true`. Live-verified end-to-end on mdemg-dev.

## What shipped

### `internal/review/schedule.go` — the supervised worker
```go
type AutogradeScheduleConfig struct {
    Enabled         bool     // default: false (opt-in)
    IntervalHours   int      // default: 6
    InitialDelayMin int      // default: 15
    Datasets        []string // default: ["contradicted_drafts"]
    SpaceID         string   // default: RSICWatchdogSpaceID
    MinConfidence   float64  // default: 0.80
    Limit           int      // default: 50
    MdemgBin        string
    Endpoint        string
}

func (s *AutogradeScheduler) Run(ctx context.Context) error
```
- **SUPERVISOR-002 contract**: nil return on disabled / no-datasets / no-binary = intentional completion, supervisor doesn't restart
- **Subprocess-spawned CLI**: `mdemg review autograde --dataset X --space-id Y --min-confidence Z --limit N --endpoint http://127.0.0.1:PORT`. Reuses ALL shipped grade-writing logic (idempotency, sink dispatch, `NonReinforcingApplier` fall-through, `:auto` verb suffix). No new HTTP surface, no autograder duplication.
- **NEVER passes `--force`**: the scheduled loop is for organic accumulation, not backfill after sink-logic changes. Passing `--force` here would cause every pending item to be re-graded every 6h regardless of whether it already has a valid grade — LLM cost + noise. Pin-tested (`TestAutogradeScheduler_RunOne_NoForceFlag`).
- **Per-dataset iteration**: a failure on dataset A does NOT skip datasets B/C. Each dataset's run gets its own jobhealth report with distinct `job_name=scheduled-autograde:<dataset>` (NOSILENT-001 cooldown-key contract — per-dataset failures don't share cooldown).
- **Bounded per-run timeout**: 30 min per dataset (even 50 items × 10s/grade on the local LLM stays well under this).

### `internal/config/config.go` — six new knobs
```
REVIEW_AUTOGRADE_SCHEDULE_ENABLED           bool  (default: false)
REVIEW_AUTOGRADE_SCHEDULE_INTERVAL_HOURS    int   (default: 6, floor: 1)
REVIEW_AUTOGRADE_SCHEDULE_INITIAL_DELAY_MIN int   (default: 15, floor: 1)
REVIEW_AUTOGRADE_SCHEDULE_DATASETS          csv   (default: "contradicted_drafts")
REVIEW_AUTOGRADE_SCHEDULE_MIN_CONFIDENCE    float (default: 0.80)
REVIEW_AUTOGRADE_SCHEDULE_LIMIT             int   (default: 50)
```

### `internal/api/server.go` — supervised wire
Wired alongside the existing `ft-loop-tripwire` block (both are opt-in loops that depend on the review platform + jobhealth being set up). `resolveMdemgBin()` reuses the shipped binary-path resolver (same as FT-RECURSIVE-002's convert pipeline).

## Tests

9 pin tests in `internal/review/schedule_test.go`, all pass:
- `TestAutogradeScheduler_DisabledCompletes` — nil return contract (SUPERVISOR-002)
- `TestAutogradeScheduler_NoDatasetsNoOp` — enabled + no datasets = still nil-return
- `TestAutogradeScheduler_NoBinaryNoOp` — Docker-startup safety (binary not on PATH)
- `TestAutogradeScheduler_RunOne_BuildsCorrectArgs` — CLI flag names + order pinned
- `TestAutogradeScheduler_RunOne_NoForceFlag` — **`--force` MUST NEVER be passed** (regression pin)
- `TestAutogradeScheduler_RunAllDatasets_IteratesAll` — failure on A doesn't skip B; per-dataset jobhealth reports with distinct jobNames
- `TestAutogradeScheduler_RunAllDatasets_ContextCancelStopsIteration` — pre-cancelled ctx skips all
- `TestAutogradeScheduler_IntervalDefaults` — zero/negative interval falls back to 6h
- `TestAutogradeScheduler_RunRespectsCancellation` — Run() exits within 3s of ctx cancel

`go test ./internal/review/... ./internal/config/... ./internal/api/...` clean; lint 0 issues.

## Live Tier-3 (mdemg-dev)

Full opt-in loop verification:
```
# Enable via launchd env + short initial delay for smoke
launchctl setenv REVIEW_AUTOGRADE_SCHEDULE_ENABLED true
launchctl setenv REVIEW_AUTOGRADE_SCHEDULE_INITIAL_DELAY_MIN 1
launchctl kickstart -k gui/501/com.mdemg.server
```

**Boot log**: `INFO msg="scheduled autograde started" interval=6h0m0s initial_delay=1m0s datasets=[contradicted_drafts] space_id=mdemg-dev min_confidence=0.8 limit=50`

**T+1min (loop fires)**: `INFO msg="scheduled autograde: dataset run complete" dataset=contradicted_drafts took=18s`

**jobhealth event** (`scheduled_job_events` table):
```
job_name                                 | success | age_sec | latency_ms
scheduled-autograde:contradicted_drafts  | t       |      30 |      17595
```

Queue state unchanged (`approved=2, dismissed=4, pending=5` — the 5 pending are the 3 legitimate rules + 2 borderline items the autograder correctly left for operator; queue was already drained by Sprint B). Confirmed the autograder ran cleanly (18s, exit 0) but had nothing new to dismiss.

Reset with `launchctl unsetenv` + restart; boot log shows no scheduler registration (default OFF properly restored).

## Rules pinned

⚠️ **Scheduled subprocess-spawning loops MUST NEVER pass `--force` (or any operator escape-hatch flag)** — the scheduled invariant is "organic accumulation." `--force` bypasses idempotency; on a scheduled loop that would re-grade every pending item on every cycle, chewing through LLM budget and re-writing the same grade rows. If you need forced backfill (e.g. after a sink-logic change), the operator runs the CLI manually with `--force`.

⚠️ **Subprocess-spawning schedulers should mirror `ftloop.BenchScheduler` shape** — SUPERVISOR-002 nil-return-when-disabled contract, `runCmd` test seam for subprocess mocking, per-item iteration where a failure on item A doesn't skip B/C, per-item jobhealth reports with distinct jobNames (NOSILENT-001 cooldown-key contract).

⚠️ **Loopback endpoint resolution MUST handle both `:9999` and `127.0.0.1:9999` `ListenAddr` shapes** — the `strings.Contains(":")`/`[0] != ':'` normalization is subtle. Any refactor that centralizes this SHOULD add a helper (e.g. `Config.LoopbackURL()`) rather than duplicate the branch.

## Not shipped (intentional)

- **`--force` as a runtime knob**: intentionally NOT exposed as a scheduled config option. If the scheduled loop needs to occasionally force-grade (e.g. quarterly rubric-version bump), that's the operator's manual CLI invocation, not a config toggle.
- **Skipping dry-run in the scheduled path**: not exposed either — the scheduled loop is always live-write, mirroring the CLI's default. Dry-run remains an operator's CLI-only mode for inspection.
- **Metric-gauge for scheduler-driven auto-dismiss count**: could add `mdemg_review_scheduled_autograde_runs_total{dataset,outcome}`. Deferred to DORMANT-METRICS-CLEANUP-001's discipline (no metrics without alert consumers) — the jobhealth event stream already provides the run history.

## Follow-ups disclosed

- **Extending to LLM call-site datasets** (16 of them) — could schedule autograde over `llm:*` datasets too, but their NoopSink means auto-grades produce zero substrate effect. Deferred — the only value is fresh "gold" labels, which don't drive an operational alert today.
- **Cadence tuning**: 6h is a guess based on the historical accumulation rate (0.5-1 drafts/day on mdemg-dev). If the natural rate turns out to be much lower, cadence can go to daily (24h) or weekly (168h) via env — no code change.

## Rollback

Single-commit revert. Rollback method for operators: `REVIEW_AUTOGRADE_SCHEDULE_ENABLED=false` (default) + restart. No substrate mutation from the scheduler since the CLI it invokes always POSTs `reinforce:false` — the invariant HITL-AUTO-DISMISS-001 established is preserved end-to-end.

## Documents Accessed

- `internal/ftloop/bench_schedule.go` (design mirror — the FT-RECURSIVE-004 pattern)
- `internal/cli/review.go:170` (autograde CLI + `postAutoGrade` + `--force` flag added in HITL-AUTO-DISMISS-001)
- `internal/api/server.go:2263` (resolveMdemgBin), `:2323` (ft-benchmark wire pattern), `:2351` (post-wire insertion point)
- `internal/config/config.go:1295` (review config block)
- `internal/supervisor/*` (SUPERVISOR-002 contract)
- `internal/jobhealth/*` (ReportWithService + scheduled_job_events schema)
- HITL-AUTO-DISMISS-001 + JIMINY-CONTRADICTED-BRIDGE-QUALITY-001 posts (parent sprints)
- Live `scheduled_job_events` + `contradicted_correction_drafts` on mdemg-dev (verification)
