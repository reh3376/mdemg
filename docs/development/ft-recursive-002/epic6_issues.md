# FT-RECURSIVE-002 Epic 6 — Issues Ledger

Live findings from the Epic-6 cycle validation. Each issue is tagged for
disposition: **[fix-now]** (rolled into this work), **[6b-followup]** (a
focused continuation of FT-RECURSIVE-002 — the full successful-cycle pipeline
automation), or **[future]** (a later sprint / Phase 7+).

Preflight (2026-06-29): `mlx_lm 0.31.3` present in `neural/.venv`; 862 GB free;
llama-server `:8102` → 200; `consulting.classify` Ready (4782 rows).

| # | Issue | Where | Disposition |
|---|---|---|---|
| E6-1 | `execPythonStage` uses `FT_LOOP_PYTHON_BIN` default `python3` (system) — the training toolchain (`mlx_lm`) lives in `neural/.venv`; a real run needs the venv interpreter. | `ftloop/controller.go` | **[FIXED]** `resolvePython()` prefers `neural/.venv/bin/python`. **Live-validated:** curate ran the module (error was its usage message). |
| E6-2 | One `RepoDir` for all stages, but `python -m training.X` needs cwd `neural/` while `python -m neural.benchmarks.X` needs the repo root. | `ftloop/controller.go::execPythonStage` | **[FIXED]** `stageDir()` per-stage cwd. **Live-validated** (module resolved from `neural/`). |
| E6-3 | Stage arg-sets are placeholders. Real: curate needs `--spec/--input-dir/--output-dir/--version`; train needs `--tier/--mode/--base-model/--expected-sha256/--dataset/--adapter-path/--rank/--alpha/--n-epochs`; gate needs `--config/--out/--model`. **Live-confirmed:** curate failed exit-2 on the missing required args (the FAIL path). | `ftloop/controller.go` | [6b-followup] full arg-sets + the inter-stage artifact chain |
| E6-4 | No MLX→GGUF conversion stage between train and gate (the run_record rule: candidate evals always use the GGUF form). | controller pipeline | [6b-followup] add a convert stage |
| E6-5 | Curate `--input-dir` (exported JSONL) has no producer in the loop — the controller must run `mdemg data export` (or read TSDB) to materialize the curate input. | controller pipeline | [6b-followup] export-before-curate, or curate-from-TSDB |
| E6-6 | The quiesce-under-contention drill wasn't exercised: the curate-fail was sub-second, so no RSIC trigger arrived during the lease window to be rejected. The quiesce code path (set+clear) DID run. | controller / test | [6b-followup] exercised naturally once the real (multi-minute) train stage holds the lease; or a focused integration test |
| E6-7 | A failed/aborted cycle's start-time still counts for the `FT_LOOP_MIN_RETRAIN_INTERVAL_HOURS` interval gate, so a failed cycle suppresses re-triggers for the full window (168h default). Desirable for a *successful* cycle; possibly too long after a *failure* (you may want to retry sooner once the cause is fixed). | gate interval logic | [future] consider a shorter post-failure retry interval (`FT_LOOP_RETRY_INTERVAL_HOURS`) |

## Live validation result (2026-06-29, Option A)

Enabled `FT_LOOP_ENABLED=true` (poll 15s); RSIC insight #29 fired
(`consulting.classify` Ready) → the gate opened cycle `h1pb0cnya605…` → the
controller picked it up, acquired the lease, recorded `curating`, and ran the
curate subprocess. **Ledger walked `triggered → curating → failed`** (curate
exit-2 on missing args), with the `ft-loop:curate` jobhealth failure row + two
distinct-`ft-loop`-service alerts, and **the lease released** (lockfile gone).
The interval gate then **suppressed re-triggers** (one cycle only across the
next RSIC cycle — SF-2 holds across the lifecycle). Controller restored to
dormant; the test cycle + jobhealth rows cleaned.

**Validated end-to-end:** trigger→open→pick-up→stage→FAIL, jobhealth, distinct
alerts, lease acquire/release, SF-2 + interval suppression, E6-1/E6-2.
**Remaining for a successful cycle to `promote_pending`:** E6-3/4/5 (the real
arg-sets + the train→convert→gate artifact chain) — the focused 6b continuation.
