# Sprint FT-RECURSIVE-002 (Phase 6b) — The actuator

## 1. Header & Metadata
- **Sprint ID**: FT-RECURSIVE-002 (recursive-loop Phase 6b)
- **Sprint line**: `docs/development/ft-recursive-002/`
- **Design backbone**: `docs/development/ft-recursive-001/SPEC_recursive_retraining_loop.md` §3 (target state machine) + §5 forks (resolved)
- **Date opened**: 2026-06-28
- **Target version**: v0.12.x (new capability — the recursive-retrain actuator)
- **Estimated effort**: ~4 dev-days (the SPEC's estimate; the live gated cycle dominates wall-clock)
- **OpenAI spend**: $0–minimal (training + gate use the local llama-server / MLX; deterministic rewards need no judge — `--enable-judge` optional and off by default here)
- **Risk level**: **Medium-High** — introduces a long-running, state-mutating, exclusive-resource actuator that orchestrates real training. Mitigated by: promotion stays **operator-confirm** (the controller halts at `promote_pending`; no auto-promote/canary/rollback — that is Phase 7); SFT-only (fork 5); the action stays **diagnostic** (controller consumes the insight out-of-band via the ledger — the mutating-action class is Phase 7, fork 7); supervised + lease-expiring so a crashed trainer cannot wedge RSIC.

## 2. Problem Statement
The readiness→RSIC→insight-#29 sensing half is LIVE and correct, but the actuator (`task_dispatch.go` `trigger_training_pipeline` → `executeAlertLog`) is a **no-op**: it logs + alerts and returns. There is no curation, training, gating, or promotion — and because insight #29 has no "already-training" gate (SF-2), it re-fires every cycle (live now: `rsic-trigger_training_pipeline` every ~5 min). Phase 6a made the manual path observable; 6b makes the actuator **real** under the no-silent substrate, with single-flight mutual-exclusion, a compute lease, and a binding gate — promotion gated on the operator.

Exit criteria (SPEC §4): *RSIC trigger launches a real gated cycle end-to-end with operator-confirm promotion; the FAIL path is verified live; zero alert spam.*

## 3. Scope & Constraints
**In scope** (SPEC §3 design):
1. **`ft_training_cycles` ledger writer** — the table exists (V0002, zero writers); 6b becomes its writer. Persistent state machine: `triggered → curating → training → gating → promote_pending → {promoted|failed|rolled_back}`. DB-backed single-flight: an open cycle bars a new trigger across restarts.
2. **Trigger conditions** (replace the no-op): task `Ready` **AND** min-fresh-row fraction (`FT_LOOP_MIN_FRESH_FRACTION` 0.30 — rows newer than the last trained dataset's max(time)) **AND** no ledger cycle within `FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS` (168). **SF-2:** insight #29 (`self_reflect.go:546`) gated on the ledger — no re-fire while a cycle is open or within the interval.
3. **Compute lease + RSIC quiesce** — exclusive lease (DB row + local lockfile) acquired before TRAIN: (a) `OrchestrationPolicy.Quiesce(due)` pauses RSIC LLM fan-out (admission rejects non-critical cycles while held); (b) requires ≥ `FT_LOOP_MIN_FREE_DISK_GB` (100) free; (c) lease-expiring (`FT_LOOP_LEASE_MAX_HOURS` 14 ≈ 1.5× the 9 h actual) → expiry = class-4 alert-and-halt so a crashed trainer can't wedge RSIC forever.
4. **The controller** — in-Go (fork 2), registered with `internal/supervisor` (`sup.Go`), orchestrating the Python pipeline as **supervised subprocesses** (`exec.CommandContext`, the `data_curate.go:45` pattern): curate (`paradigm_router`) → train (`train_ft.py`) → benchmark+gate (`run_benchmark` + `regression_gate.py`). Each stage updates the ledger AND reports `jobhealth` reusing the **Phase-6a `ft-loop:<stage>` job names** (the controller is the first automated caller of that surface). Halts at `promote_pending` for operator confirm.
5. **`[AMD-7]`** — `TRAINING_READINESS_THRESHOLD` env (+ per-task overrides) replacing the hardcoded `DefaultReadinessThreshold=500`.
6. **`[AMD-1]`** — epoch-cap / early-stop as controller-passed env parameters (`LORA_N_EPOCHS_CAP` already exists in `train_ft.py`; wire `EARLY_STOP_VAL_LOSS_FACTOR`; the `auto` rejection stays a hard error — never weakened).
7. **Gate** — 16-task augmented (the FT-RECURSIVE-001 pinned manifest) + `valid_clean.jsonl`; dual 5a (vs current production aggregate — "the model being replaced", not the frozen 0.8338) / 5b (vs fresh-merge). FAIL is a **normal** outcome: archive candidate + ledger verdict row, no promotion, ONE medium alert (distinct Service).
8. **Operator-confirm promotion surface** — a CLI (`mdemg ft-loop promote --cycle-id <id> --confirm` / `--reject`) that the operator runs against a `promote_pending` cycle; records to the ledger (and `ft_hitl_decisions`). The actual symlink-swap + llama-server restart is performed here, operator-gated.

**Out of scope (Phase 7 / 9 / separate):** the new mutating-long-running RSIC action class + fail-closed validation (fork 7); canary (held-call replay) + auto-rollback + auto-promote-after-N (fork 3/4); drift monitoring + `ft_model_versions` writer + the issue filer (Phase 9 / FT-RECURSIVE-004); RL/DPO in the loop (fork 5 — SFT-only first); publishing to Ollama (promotion is local symlink, not distribution); GUARDRAIL-PRODUCER-001 (separate prerequisite).

**Constraints:** no-hardcoding (every threshold/interval/path is env with a default); sequential epics; 3 testing tiers incl. a **live gated cycle** (the exit criterion — real binary runs a real small SFT cycle, ~13–40 min for a fast task, observed in the ledger + TSDB); CUIDv2 cycle ids; docs final epic (feature doc `ft-recursive-loop.md` extended + `00_README` STATUS); new Python passes neural pytest+ruff + the UxTS drift checker.

## 4. Dependencies
- FT-RECURSIVE-001 (the `ft-loop:<stage>` jobhealth surface + pinned eval manifest + readiness reasons) — merged.
- `ft_training_cycles` (V0002), `internal/ape/orchestration_policy.go` (admission), `internal/supervisor`, `internal/jobhealth`, the Python pipeline (`paradigm_router.py`, `train_ft.py`, `run_benchmark.py`, `regression_gate.py`), llama-server :8102, the MLX training env (`mlx_lm` in `neural/.venv` — the run_record flagged it was absent post-Phase-13.5; **preflight must check it**).

## 5. Implementation Plan (sequential epics + gates)
**Epic 0 — Plan** (this doc).

**Epic 1 — `ft_training_cycles` ledger.** Buffered-or-sync writer + a typed Go API (`OpenCycle`/`AdvanceStage`/`CompleteCycle(verdict)`/`FailCycle(cause)`); `HasOpenOrRecentCycle(interval)` for single-flight. Gate: unit tests over the state transitions + single-flight predicate.

**Epic 2 — `[AMD-7]` + `[AMD-1]` config.** `TRAINING_READINESS_THRESHOLD` (+ `TRAINING_READINESS_THRESHOLD_OVERRIDES` JSON per-task) wired into `NewDatasetBuilder`; `EARLY_STOP_VAL_LOSS_FACTOR` passed through. Config-consumer guard green. Gate: readiness reflects the env threshold live.

**Epic 3 — Trigger + SF-2 gate.** Fresh-fraction computation (rows newer than last trained dataset max(time)); interval check via the ledger; insight #29 gated so it does not re-fire while a cycle is open/recent. Gate: insight #29 stops re-firing once a cycle is open (live); the `rsic-trigger_training_pipeline` spam ceases.

**Epic 4 — Compute lease + quiesce.** Lease (DB row + lockfile), `OrchestrationPolicy.Quiesce(until)` (admission rejects non-critical while held; critical still admitted), disk floor, lease expiry → alert-and-halt (class 4, distinct `ft-loop` Service). Gate: quiesce rejects a non-critical trigger live; lease expiry alerts.

**Epic 5 — The controller.** Supervised loop consuming an open `triggered` cycle → curate → train → gate via `exec.CommandContext` subprocesses, each updating the ledger + `ft-loop:<stage>` jobhealth; FAIL archives + one alert; PASS → `promote_pending`. Plus `mdemg ft-loop promote --cycle-id --confirm|--reject` (operator-gated symlink swap + restart, ledger + `ft_hitl_decisions`). Gate: a forced-FAIL cycle runs end-to-end and stops cleanly; the controller is supervised (panic→restart budget).

**Epic 6 — Testing (3 tiers).** See §6. The live gated cycle is the exit criterion.

**Epic 7 — Documentation (final).** Extend `docs/features/ft-recursive-loop.md` (the actuator + promotion); CHANGELOG; `00_README_v2` STATUS (Phase 6 → 6b shipped); CLAUDE.md note; `post.md`.

## 6. Testing Plan (3 tiers)
- **Tier 1 (unit):** ledger state transitions + single-flight; fresh-fraction math; trigger-condition truth table; lease acquire/expire; quiesce admission; readiness-threshold env; SF-2 insight gating. Reward/gate parameter wiring.
- **Tier 2 (integration):** controller drives a **mocked/fast** pipeline (tiny dataset, `--n-epochs 1` on a fast task) end-to-end through the ledger states with subprocess orchestration; FAIL path archives + single alert; quiesce blocks a concurrent trigger.
- **Tier 3 (live — the exit criterion):** real binary, real services. (a) A real **small** SFT cycle on a fast Ready task (e.g. a `--target` few-hundred-row task, ~13–40 min per the FT-CLASSIFY-002 actuals) launched via the RSIC trigger → ledger walks `triggered…gating…promote_pending`; stages land in `scheduled_job_events`; gate runs the pinned 16-task eval; candidate archived; **operator-confirm promotion exercised on a side port** (no production swap unless the operator opts in). (b) The **FAIL path** verified live (a deliberately-regressing candidate → `failed`, one alert, no promotion). (c) **Zero alert spam** — `rsic-trigger_training_pipeline` stops while the cycle is open (SF-2). (d) Lease expiry + quiesce drill.

## 7. Commit Strategy
Sequential commits per epic on `reh3376_dev01`. The live cycle (Epic 6) produces a `run_record.md` (the FT-CLASSIFY-002 precedent). Push → auto-PR.

## 8. Verification Checklist
- [ ] `ft_training_cycles` written; single-flight bars a 2nd trigger across restart
- [ ] Trigger fires only on Ready + fresh-fraction + interval; SF-2 re-fire stops (live)
- [ ] Compute lease + quiesce + disk floor + lease-expiry-halt all exercised
- [ ] Controller supervised; curate→train→gate via subprocesses; ledger + jobhealth per stage
- [ ] `[AMD-7]` threshold env + per-task overrides; `[AMD-1]` epoch/early-stop params; `auto` still hard-rejected
- [ ] Gate runs the pinned 16-task augmented + valid_clean, dual 5a/5b; FAIL = archive + 1 alert, no promote
- [ ] Operator-confirm promotion CLI; `ft_hitl_decisions` recorded; NO auto-promote/canary/rollback (boundary held)
- [ ] **Live: a real gated cycle end-to-end + the FAIL path** (run_record.md)
- [ ] build + lint + config-guard + neural pytest/ruff green
- [ ] feature doc + CHANGELOG + 00_README STATUS + CLAUDE note + post.md

## 9. Documentation Update — Epic 7 above

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Controller wedges RSIC (training holds the lease forever) | Med | High | Lease-expiring (`FT_LOOP_LEASE_MAX_HOURS` 14) → alert-and-halt; quiesce only pauses non-critical cycles; supervised restart budget |
| A real training run during the sprint disrupts the dev box | Med | Med | Tier-3 uses a *fast* task + tiny target; promotion is side-port + operator-gated; disk floor pre-checked |
| `mlx_lm` training env absent (run_record finding) | Med | High | Preflight check in Epic 5 hard-fails with a clear message before TRAIN (the run_record's #2 lesson) |
| Scope creep into Phase 7 (auto-promote/canary/mutating-action class) | Med | High | Hard boundary §3: promotion is operator-confirm, action stays diagnostic, no canary/rollback |
| Benchmark stage scored 0.0000 on a zero-call endpoint (run_record: 4× in one sprint) | Med | High | Benchmark stage **hard-fails on zero successful calls** (run_record lesson #3); candidate evals use the GGUF form; readiness=/health not /v1/models |
| Insight #29 gating misses an edge → loop never triggers OR double-triggers | Low | High | Ledger is the single source of truth (DB-backed single-flight); unit truth-table + live SF-2 check |

## 11. Documents Accessed
- `SPEC_recursive_retraining_loop.md` §3/§5; `ft-classify-002/run_record.md` (the 6a-lessons / live actuals)
- `internal/ape/{orchestration_policy,self_reflect,task_dispatch,self_assess}.go`, `internal/tsdb/dataset_builder.go`, `internal/supervisor/`, `internal/jobhealth/`, `internal/cli/{data_curate,ft_loop,serve}.go`
- `neural/training/{paradigm_router,train_ft,regression_gate}.py`, `neural/benchmarks/run_benchmark.py`
- `docs/development/ft-recursive-001/augmented_eval_manifest.json`; live `ft_training_cycles` schema, `mdemg data status`

## 12. Rollback Procedures
- The actuator is **default-gated**: a master `FT_LOOP_ENABLED` (default **false**) keeps the controller dormant until the operator opts in — the no-op-to-real cutover is itself reversible by config. Reverting the commits restores the diagnostic no-op. No serving mutation occurs without the operator-confirm promotion step; that step backs up the prior symlink target for one-command restore.
