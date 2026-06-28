# FT Recursive-Retraining Loop — Observability (Phase 6a)

> Status: **Phase 6a shipped** (FT-RECURSIVE-001). This documents the
> *observability + hardening* of the manual retrain path. The autonomous
> actuator (trigger → curate → train → gate → promote) is later phases
> (FT-RECURSIVE-002/003/004) — **nothing here trains, promotes, or runs
> autonomously.** Design: `docs/development/ft-recursive-001/SPEC_recursive_retraining_loop.md`.

## Why

The recursive-retraining loop's manual vertical slice (FT-CLASSIFY-002) was
walked by hand but left no trace: its stages never landed in
`scheduled_job_events`, a failed stage fired no alert, and several silent-failure
seams sat on the path. Before building the actuator, the manual path must be
**fully observable** — so a failure is loud and a dormant loop is detectable.

## What shipped

| Item | Before | After |
|---|---|---|
| **SF-3** distinct alert services | the 6 RSIC diagnostic alerts shared `("rsic", Medium)` → mutual suppression (the dispatcher cooldown key is `(Service,Severity)`) | each alerts under `rsic-<action_type>` (its own cooldown key) |
| **SF-7** per-gate readiness reasons | `TrainingDataReadiness` exposed only the `Ready` boolean | `mdemg data status` lists *why* a task is not ready (`insufficient_rows` / `error_rate_high` / `missing_system_prompt`) |
| **SF-1** readiness staleness | a silent readiness-query failure stopped insight #29 with only a WARN; the loop went dormant unnoticed | a heartbeat gauge (`mdemg_rsic_readiness_assessed`) is set on each *successful* assessment; the `training_readiness_stale` evaluator rule fires when it goes stale |
| **Stage instrumentation** | manual stages were untraced | `mdemg ft-loop report-stage` records each stage to `scheduled_job_events` (`job_name=ft-loop:<stage>`); a failure fires a high alert under the distinct `ft-loop` service |
| **Eval pin** `[AMD-2]` | the promotion-gate eval was an ad-hoc rebuild (gitignored data) | `docs/development/ft-recursive-001/augmented_eval_manifest.json` pins the recipe + SHA-256 + per-task spec hashes + leak verdict |
| **SF-6** export hygiene | export archives accumulated in `TempDir/mdemg-exports` forever | pruned older than `MDEMG_EXPORT_RETENTION_HOURS` (default 168h) on each export |
| docstrings | `evaluate_ft.py` referenced the decommissioned `:8101` | `:8102` (production llama-server) |

## How to use

**Instrument a manual retrain run** — report each stage as you run it:

```bash
mdemg ft-loop report-stage --stage capture  --status success --cycle-id ftc-2026-06-28 --latency-ms 140000 --detail "200/200 kept"
mdemg ft-loop report-stage --stage curate    --status success --cycle-id ftc-2026-06-28
mdemg ft-loop report-stage --stage train     --status success --cycle-id ftc-2026-06-28 --latency-ms 783000
mdemg ft-loop report-stage --stage benchmark --status success --cycle-id ftc-2026-06-28
mdemg ft-loop report-stage --stage gate      --status success --cycle-id ftc-2026-06-28 --detail "aggregate 0.852, classify +1.9pp"
mdemg ft-loop report-stage --stage promote   --status success --cycle-id ftc-2026-06-28
```

Stages: `capture | curate | train | benchmark | gate | promote`. A
`--status failure` (with `--detail`) records the failure **and** fires a
high-severity `ft-loop` alert — visible in `~/.mdemg/alerts/current.json` and
the session hooks. Best-effort + nil-safe: a TSDB/alert problem never changes
the command's exit status. (The CLI reads TSDB config from the environment —
load `.env` or export `TSDB_HOST_PORT` etc., as with the other CLI jobs.)

**See readiness + reasons:**

```bash
mdemg data status        # per-task Rows / Rate/day / Days Left / Ready + the failing gate(s)
```

## Config

| Env var | Default | Meaning |
|---|---|---|
| `FT_READINESS_STALENESS_MIN` | 30 | fire `training_readiness_stale` when no successful readiness assessment within N minutes |
| `MDEMG_EXPORT_RETENTION_HOURS` | 168 | prune export archives older than this (0 disables) |

## The actuator (Phase 6b — FT-RECURSIVE-002, shipped default-off)

Phase 6b makes the no-op `trigger_training_pipeline` real — a supervised
controller that runs a gated retrain cycle — but it ships **dormant behind
`FT_LOOP_ENABLED=false`**. Nothing trains or mutates serving state until the
operator opts in.

- **Ledger** (`ft_training_cycles`): the cycle state machine
  `triggered→curating→training→gating→promote_pending→{promoted|failed|rolled_back}`,
  event-sourced; an open cycle is DB-backed single-flight (survives restarts).
- **Trigger gate** (`internal/ftloop`): a cycle launches only when a task is
  Ready **AND** enough new signal exists since the last cycle (`FT_LOOP_MIN_FRESH_FRACTION`)
  **AND** none ran within `FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS`. When disabled or
  blocked, the trigger is **suppressed** (no alert — the SF-2 fix that ended the
  per-cycle `rsic-trigger_training_pipeline` spam).
- **Compute lease + RSIC quiesce**: a single-host lockfile (reclaimable on
  expiry so a crashed trainer can't wedge RSIC) + `OrchestrationPolicy.Quiesce`
  pauses new RSIC triggers while a retrain holds the box; disk-floor preflight.
- **Controller**: walks curate→train→gate as ctx-cancellable Python subprocesses,
  updating the ledger + `ft-loop:<stage>` jobhealth per stage. FAIL → archived +
  one `ft-loop` alert; lease-expiry/disk-floor → class-4 high alert-and-halt;
  PASS → `promote_pending` (it **halts** — promotion is operator-gated).
- **Promotion** (operator-confirm): `mdemg ft-loop promote --cycle-id <id>
  [--reject] [--reason …]` records the decision in the ledger. Auto-promote +
  canary are Phase 7.

To enable (after the live validation in the next sprint): set
`FT_LOOP_ENABLED=true`. Tunables: `FT_LOOP_POLL_INTERVAL_SEC` (60),
`FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS` (168), `FT_LOOP_MIN_FRESH_FRACTION` (0.30),
`FT_LOOP_LEASE_MAX_HOURS` (14), `FT_LOOP_MIN_FREE_DISK_GB` (100),
`TRAINING_READINESS_THRESHOLD` (+ per-task overrides), `FT_LORA_EPOCHS_CAP` (3),
`FT_EARLY_STOP_VAL_LOSS_FACTOR` (1.05).

> ⚠️ The enabled path's live validation (a real SFT cycle + the FAIL path; the
> subprocess arg-sets) is **Epic 6 — not yet run**. The default-off actuator is
> code-complete + unit-tested; the SF-2 suppression is live-verified.

## What's next

- **FT-RECURSIVE-002 Epic 6** — the live gated cycle (enable + run on a fast task).
- **FT-RECURSIVE-003 (Phase 7)** — RSIC integration, canary, auto-rollback.
- **FT-RECURSIVE-004 (Phase 9)** — drift monitoring + the issue filer.
- Prerequisite, separate: **GUARDRAIL-PRODUCER-001** — `guardrail.evaluate` has
  only 3 production rows (no live producer); it cannot be retrained until one exists.
