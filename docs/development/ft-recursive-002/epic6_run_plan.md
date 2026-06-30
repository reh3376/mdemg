# FT-RECURSIVE-002 Epic 6 — Live Gated Cycle (run plan)

> ✅ **EXECUTED 2026-06-29/30.** Every pipeline stage was validated live and the
> controller wired with the proven commands (PRs #488/#489/#492); the
> orchestration (lease/quiesce/ledger/FAIL) ran live in "Option A". Outcomes are
> in `epic6_issues.md` + `run_record.md`. The one remaining item is a single
> *enabled* tiny-subset integration drill off-peak. This file is retained as the
> original run plan.

The actuator (Epics 0–5) is merged **default-off**. Epic 6 is the SPEC's exit
criterion: a real gated cycle end-to-end with `FT_LOOP_ENABLED=true`, plus the
FAIL path. This is the only remaining 6b item.

## Preflight (run FIRST — the run_record lessons)
1. **Training env present:** `neural/.venv` has `mlx_lm` (FT-CLASSIFY-002 found it
   absent post-Phase-13.5). `python3 -c "import mlx_lm"` must succeed.
2. **Disk floor:** ≥ `FT_LOOP_MIN_FREE_DISK_GB` (100) free on the repo volume.
3. **Endpoint:** llama-server `:8102/v1/models` reachable (the gate benchmark
   must **hard-fail on zero successful calls** — never score 0.0000).
4. **Pick a FAST task** (short prompts, ~13 min train per FT-CLASSIFY-002): e.g.
   a few-hundred-row Ready task. Confirm via `mdemg data status` (now shows
   per-gate reasons + the configured threshold).
5. **Validate the subprocess arg-sets** in `ftloop/controller.go::execPythonStage`
   against the real commands (curate `paradigm_router`, train `train_ft`, gate
   `run_benchmark` + `regression_gate`) — the FT-CLASSIFY-002 `run_record.md` is
   the reference. Wire the exact args; promotion-gate eval = the pinned
   `augmented_eval_manifest.json` (16-task) + `valid_clean.jsonl`, dual 5a/5b.

## Run
1. Set `FT_LOOP_ENABLED=true` (+ a low `FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS` for
   the drill, and `TRAINING_READINESS_THRESHOLD_OVERRIDES` if forcing a task
   Ready). Restart the server.
2. The trigger gate opens a `triggered` cycle; the controller acquires the
   lease, quiesces RSIC, and walks curate→train→gate.
3. **Watch:** `ft_training_cycles` walks `triggered→curating→training→gating→
   promote_pending`; each stage lands in `scheduled_job_events` (`ft-loop:<stage>`);
   `mdemg watchdog`/Grafana for resource use; the lease lockfile.
4. **Operator-confirm on a side port** (no production swap unless you opt in):
   `mdemg ft-loop promote --cycle-id <id>` (or `--reject`).

## Verify (exit criteria)
- [ ] A real cycle reaches `promote_pending`; all stages in `scheduled_job_events`.
- [ ] **FAIL path** (a deliberately-regressing candidate) → `failed`, ONE
      `ft-loop` alert, no promotion.
- [ ] **Zero alert spam** — `rsic-trigger_training_pipeline` stays silent while
      the cycle is open (SF-2).
- [ ] Lease + quiesce drill: a 2nd trigger is rejected (`quiesced`) while held;
      lease expiry → class-4 alert-and-halt.
- [ ] No zero-call 0.0000 gate scores (benchmark hard-fails on zero calls).

## Output
- `docs/development/ft-recursive-002/run_record.md` (the FT-CLASSIFY-002 precedent
  — stage timings, surprises, the gate verdict).
- If the live run surfaces arg-set fixes, they're their own fix-commit (the
  Phase-11.6.2 precedent — don't silently fold into the sprint commit).

## Safety
- The cycle runs on the dev box (~36 GB RAM peak, ~15–40 min for a fast task).
- Promotion is operator-gated + side-port; production serving is untouched unless
  explicitly confirmed. `FT_LOOP_ENABLED=false` reverts to dormant at any time.
