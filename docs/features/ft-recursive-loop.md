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

To enable: set `FT_LOOP_ENABLED=true`. Tunables: `FT_LOOP_POLL_INTERVAL_SEC` (60),
`FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS` (168), `FT_LOOP_MIN_FRESH_FRACTION` (0.30),
`FT_LOOP_LEASE_MAX_HOURS` (14), `FT_LOOP_MIN_FREE_DISK_GB` (100),
`TRAINING_READINESS_THRESHOLD` (+ per-task overrides), `FT_LORA_EPOCHS_CAP` (3),
`FT_EARLY_STOP_VAL_LOSS_FACTOR` (1.05). Epic-6 pipeline knobs (the proven
commands): `FT_LOOP_{WORK_DIR, BASE_MODEL, BASE_SHA, UAITS_SPEC, BENCHMARK_CONFIG,
LORA_RANK, LORA_ALPHA, GATE_PORT, EXPORT_SINCE_DAYS, GATE_TASK_FILTER,
GATE_MIN_SCORE}`.

### Epic 6 — the pipeline is wired and validated (2026-06-29/30)

The controller's five stages (`internal/ftloop/controller_stages.go`) are wired
with the exact commands proven **live against the real system**, stage by stage:

| Stage | Command (validated live) | Artifact |
|-------|--------------------------|----------|
| export  | `mdemg data export --tables llm_interactions --since <ts>` | `llm_interactions.jsonl` |
| curate  | `python -m training.paradigm_router --spec <uaits> …` (venv, cwd `neural/`) | versioned SFT split (`val.jsonl`→`valid.jsonl` bridged) |
| train   | `python -m training.train_ft --tier 1 --mode sft --base-model … --expected-sha256 … --rank 32 --alpha 64` | `adapters.safetensors` (real 168 MB LoRA) |
| convert | `mlx_lm.fuse --dequantize` → `convert_hf_to_gguf --outtype f16` → `llama-quantize Q5_K_M` | candidate `Q5_K_M.gguf` (~11 GB) |
| gate    | side-port `llama-server` + `python -m neural.benchmarks.run_benchmark` (real calls, no-zero-call) | `gate-report.json`; PASS → `promote_pending` |

The orchestration (lease / quiesce / ledger transitions / FAIL path) was validated
live in "Option A" — a real cycle walked `triggered→curating→failed`, the SF-2
suppression held, and the lease released cleanly. 14 findings are ledgered in
`docs/development/ft-recursive-002/epic6_issues.md`; the run timings are in
`run_record.md`.

> ⚠️ A real latent bug surfaced and was fixed here: the base-model SHA drift-guard
> had rotted (**E6-8** — upstream `mlx-community/Qwen3-14B-4bit` re-published its
> `config.json`; the pin would have failed *any* retrain). The new pin
> `a54ec18f…` is byte-identical to the production model. The quiesce design was
> also confirmed necessary (**E6-12**: an on-box training run saturates the
> machine and degrades the production `llama-server`).
>
> The one remaining item before flipping the default on a schedule is a single
> *enabled* tiny-subset integration drill (the heavy convert + the slow
> contended-box train end-to-end through the live controller, off-peak).

## What's next

- **A single enabled tiny-subset drill** — flip `FT_LOOP_ENABLED=true` off-peak,
  run one full cycle through the wired controller, confirm `promote_pending`.
- **FT-RECURSIVE-003 (Phase 7)** — RSIC integration, canary, auto-rollback, the
  mutating-action class (auto-promote).
- **FT-RECURSIVE-004 (Phase 9)** — drift monitoring + the issue filer.
- Prerequisite, separate: **GUARDRAIL-PRODUCER-001** — `guardrail.evaluate` has
  only 3 production rows (no live producer); it cannot be retrained until one exists.
