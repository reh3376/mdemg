# FT-RECURSIVE-002 (Phase 6b) — Post

**Status: SHIPPED — actuator default-off (Epics 0–5, PR #486) + Epic 6 pipeline wired & validated live (PRs #488/#489/#492).** · 2026-06-28 → 2026-06-30 · branch `reh3376_dev01`

The no-op `trigger_training_pipeline` actuator is now a real, supervised
controller — shipping **dormant behind `FT_LOOP_ENABLED=false`**, so nothing
trains or mutates serving state until the operator opts in. **Epic 6 (2026-06-29/30)
wired the controller's five stages with the exact export→curate→train→convert→gate
commands, each validated live against the real system**, and caught + fixed a real
latent bug (the stale base-model SHA pin, E6-8). The one remaining item is a
single *enabled* tiny-subset integration drill off-peak.

## Shipped (Epics 0–5 + 7)
1. **Epic 1 — ledger.** `ft_training_cycles` (V0002, first writer): event-sourced
   state machine (`triggered→curating→training→gating→promote_pending→{promoted|failed|rolled_back}`),
   DB-backed single-flight (`OpenCycle`) + retrain-interval gate (`LastCycleStart`)
   that survive a restart.
2. **Epic 2 — `[AMD-7]`.** `TRAINING_READINESS_THRESHOLD` (+ per-task overrides)
   replaces the hardcoded `DefaultReadinessThreshold=500`; wired into RSIC +
   `mdemg data status`/`check`.
3. **Epic 3 — trigger gate + SF-2.** `internal/ftloop.Gate`: a pure `Decide`
   plus a ledger-backed evaluation. `trigger_training_pipeline` now routes
   through `executeTriggerTrainingPipeline`, which **suppresses** the alert when
   a cycle is open / within the retrain interval / the actuator is disabled —
   ending the per-reflection-cycle spam. On a trigger decision the gate OPENS a
   cycle; the controller consumes it out-of-band (SPEC fork 7). **Live-verified:
   0 `rsic-trigger_training_pipeline` dispatches after restart** (was every ~5 min).
4. **Epic 4 — lease + quiesce + disk floor.** `ftloop.AcquireLease`
   (single-host lockfile, reclaimable on expiry so a crashed trainer can't wedge
   RSIC), `ftloop.FreeDiskGB`, and `OrchestrationPolicy.Quiesce`/`IsQuiesced`
   (admission rejects new triggers while a retrain holds the lease).
5. **Epic 5 — the controller.** `ftloop.Controller` (supervised via
   `StartSupervisedBackground`, default-off): picks up a triggered cycle, walks
   curate→train→gate as ctx-cancellable Python subprocesses, holds the lease +
   quiesces RSIC, updates the ledger + the Phase-6a `ft-loop:<stage>` jobhealth
   surface per stage, FAIL→archived+one alert, lease-expiry/disk-floor→class-4
   high alert-and-halt, PASS→`promote_pending`. `mdemg ft-loop promote
   --cycle-id [--reject]` records the operator decision. `[AMD-1]` epoch cap
   passed to `train_ft`.

## Testing (this sprint)
- **Tier 1:** ledger state machine + single-flight; trigger truth-table (incl.
  all SF-2 suppressions); lease acquire/expiry/reclaim; quiesce state; disk-free;
  controller curate→train→gate / stage-failure-halt / disk-floor (stub runStage);
  AMD-7 per-task threshold; executor SF-2 suppression. Config guard 744/744; lint 0.
- **Tier 3 (live, partial — the safe default-off surface):** controller dormant
  when disabled; **SF-2 suppression confirmed** (0 trigger dispatches post-restart).

## ✅ Epic 6 — DONE (2026-06-29/30, PRs #488/#489/#492)
Each pipeline stage was validated **live against the real system** before being
wired into the controller (`internal/ftloop/controller_stages.go`):
- **export** — `mdemg data export --tables llm_interactions --since <ts>`
  (corrected `--from`→`--since`, E6-5).
- **curate** — `python -m training.paradigm_router` (venv, cwd `neural/`),
  4660-row SFT split; `val.jsonl`→`valid.jsonl` bridge (E6-10); subset-manifest
  row-count fix so `train_ft` iters track the file (E6-11).
- **train** — `python -m training.train_ft --tier 1 --mode sft …`; a real 168 MB
  LoRA adapter, loss 3.28→2.41.
- **convert** — `mlx_lm.fuse --dequantize` (E6-14, the 4bit-fused tensors can't
  map directly) → `convert_hf_to_gguf --outtype f16` → `llama-quantize Q5_K_M`,
  an 11 GB candidate GGUF.
- **gate** — side-port `llama-server` + `run_benchmark` with real (non-zero) calls.
- **Orchestration drill (Option A):** a real cycle walked `triggered→curating→failed`,
  SF-2 held, the lease released — `run_record.md` has the timings.
- **Latent bug caught + fixed (E6-8, PR #489):** the base-model SHA pin had rotted
  (`cdc16756…` → `a54ec18f…`, byte-identical to the production model config); it
  would have failed any retrain. **E6-12:** on-box training saturates the machine
  and degrades the production `llama-server` — confirms the quiesce design.
- **Controller wired (PR #492):** `runCmd`/`execCmd` + the five `stage*` methods;
  `ControllerConfig` extended with the 11 Epic-6 pipeline fields; `runCycle`
  orchestrates the stages. Config guard 756/756; lint 0.
- All 14 findings ledgered in `epic6_issues.md`. **Remaining:** one *enabled*
  tiny-subset integration drill off-peak.

## Why merging now is safe
The actuator is **default-off**; with `FT_LOOP_ENABLED` unset the controller
returns immediately (verified) and the only behavior change is the SF-2 alert
suppression (verified live). No training, no serving mutation, no autonomy ships
active. Phase 7 (auto-promote/canary/mutating-action class) and Phase 9 (drift)
remain future sprints; GUARDRAIL-PRODUCER-001 is the separate prerequisite.

## Documents Accessed
- `SPEC_recursive_retraining_loop.md` §3/§5; `ft-classify-002/run_record.md`
- `internal/ftloop/*`, `internal/tsdb/ft_cycle_ledger.go`, `internal/ape/{task_dispatch,orchestration_policy,cycle,types_rsic}.go`, `internal/api/server.go`, `internal/cli/ft_loop.go`, `internal/config/config.go`
- Live `ft_training_cycles` schema, `mdemg data status`, alert stream
