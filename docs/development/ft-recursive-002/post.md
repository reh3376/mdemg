# FT-RECURSIVE-002 (Phase 6b) — Post

**Status: actuator SHIPPED default-off (Epics 0–5 + docs); live gated cycle (Epic 6) deferred to the next session by operator decision.** · 2026-06-28 · branch `reh3376_dev01` · PR #486

The no-op `trigger_training_pipeline` actuator is now a real, supervised
controller — but it ships **dormant behind `FT_LOOP_ENABLED=false`**, so nothing
trains or mutates serving state until the operator opts in. The enabled path's
live validation (a real SFT cycle + the FAIL path) is Epic 6, run next session.

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

## ⚠️ Deferred to Epic 6 (next session)
- **The real gated cycle** end-to-end with `FT_LOOP_ENABLED=true` on a fast Ready
  task, + the FAIL path, + the lease/quiesce drill — the SPEC's exit criterion.
- **The Python subprocess arg-sets** (`execPythonStage`) are wired but their
  exact arguments are validated against the live pipeline in Epic 6 (the
  FT-CLASSIFY-002 manual run is the reference). Preflight (`mlx_lm` env present,
  disk floor, endpoint reachable, no-zero-call discipline) runs first.
- Produces `docs/development/ft-recursive-002/run_record.md`.

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
