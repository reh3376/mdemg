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
| E6-3 | Stage arg-sets are placeholders. Real: curate needs `--spec/--input-dir/--output-dir/--version`; train needs `--tier/--mode/--base-model/--expected-sha256/--dataset/--adapter-path/--rank/--alpha/--n-epochs`; gate needs `--config/--out/--model`. **Live-confirmed:** curate failed exit-2 on the missing required args (the FAIL path). | `ftloop/controller.go` | **[curate + train VALIDATED]** convert/gate + controller wiring [6b-continuation] |
| E6-4 | No MLX→GGUF conversion stage between train and gate (the run_record rule: candidate evals always use the GGUF form). | controller pipeline | [6b-followup] add a convert stage |
| E6-5 | Curate `--input-dir` (exported JSONL) has no producer in the loop — the controller must run `mdemg data export` (or read TSDB) to materialize the curate input. | controller pipeline | **[SOLVED]** `mdemg data export --tables llm_interactions[,…] --since … --output <tar>` → extract `llm_interactions.jsonl` into `<input-dir>/`. The export schema matches `quality_filter`'s `TEXT_FIELDS` exactly. |

| E6-8 | **Train base-model SHA pin is STALE (drift-guard rot).** `train_ft.SPRINT_C_CONFIG_SHA = cdc167566e…` but upstream `mlx-community/Qwen3-14B-4bit` `config.json` is now `a54ec18f…` (re-published since the pin). A real retrain **drift-aborts**. Verified: the current upstream config is **byte-identical** to the local production model's config (`qwen3-14b-mdemg-v1`), so the base is functionally the production base; the pin just rotted. Same class as the run_record's "configs rot at cutovers" — now for the base-model pin. | `neural/training/train_ft.py:83`; `expert_selection.py:10` | **[decision]** update the pin to `a54ec18f…` (safe — identical to production) **or** pin to a specific HF revision. A deliberate provenance call (the pin is a drift-guard) — surfaced, not silently bypassed. |
| E6-9 | **The dense 4bit base model dir is absent locally** (not in HF cache or `.local-models`; only the fine-tune + bf16 dequants are present). Train needs `--base-model <dir>` = the dense `Qwen3-14B-4bit` (~8 GB download). | `.local-models/` | [6b-continuation] download `mlx-community/Qwen3-14B-4bit` after the pin decision (E6-8) |

| E6-10 | **Curate→train dataset impedance:** curate's `paradigm_router` versioned output names the eval split `val.jsonl`, but `train_ft`/`mlx_lm.lora` expect `valid.jsonl`. | curate output vs train input | [6b-continuation] the controller's train step copies/renames `val.jsonl`→`valid.jsonl` (or curate emits both) |
| E6-11 | **train iters derive from the manifest, not the file:** `train_ft` computes `iters = n_epochs × ceil(train_rows/batch)` where `train_rows` = `manifest.splits.train.rows`. A subset/edited `train.jsonl` whose manifest still says the full count trains for the full iters (silently overtraining). | `train_ft.py:707-722` | [6b-continuation] keep the manifest row count authoritative + in sync with the data the controller hands to train |
| E6-12 | **Quiesce rationale confirmed live:** during the full train run, the M5's RAM/compute saturated and llama-server `:8102` started timing out → a HIGH `jiminy.evaluate_llm` consecutive-failure alert + `rsic-alert_llm_health`. This is exactly the RSIC↔trainer contention `OrchestrationPolicy.Quiesce` exists to prevent — strong evidence the quiesce (Epic 4) is necessary, and E6-6's contention drill will trigger naturally under a real-length train. | live | confirms Epic-4 design |
| E6-8 (update) | Pin updated to `a54ec18f…` across `train_ft.py`, `stratified_split.py`, `test_train_ft_integration.py` (3 functional consumers; `expert_selection.py` was a truncated docstring example). Base downloaded (`mlx-community/Qwen3-14B-4bit`, 7.8 GB) → config SHA matches the new pin. **Train ran: base loaded, SHA verified (no drift-abort), LoRA initialized (42M params), training started.** | — | **[RESOLVED]** |

## Stage-1 (data-prep + curate) — VALIDATED (2026-06-29)

Empirically validated against the live system (the exact commands the controller will wire):

```
# data-prep (E6-5): export production rows → input-dir
mdemg data export --space-id mdemg-dev --tables llm_interactions --since <RFC3339> --output <D>/export.tar.gz
tar -xzf <D>/export.tar.gz -C <D> ; cp <D>/**/llm_interactions.jsonl <D>/input/

# curate (E6-3): the proven command (cwd=neural/, venv python)
neural/.venv/bin/python -m training.paradigm_router \
  --spec docs/tests/uaits/specs/mdemg.uaits.json \
  --input-dir <D>/input --output-dir <D>/output --version <cycle-id>
```
Result: 7603 exported → 5826 filtered/converted → **versioned 4660 train / 582 test / 584 val** at `output/sft_interactions/versioned/{train,test,val}.jsonl` — the `--dataset` input for train. DPO/curriculum datasets skip cleanly when their tables aren't exported (export `constraint_outcomes`/`metric_samples` too if those paradigms are wanted; SFT-only is fork-5's recommendation).

**Next:** train (the real ~13–40 min run — operator checkpoint) → convert (MLX→GGUF) → gate.
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

| E6-13 | **Training on the dev box is very slow + degrades the live server.** With the mdemg server + neo4j running, mlx_lm.lora hit **~52 s/iter** (It/sec 0.019), peak 23.6 GB, and starved llama-server `:8102` → HIGH `jiminy.evaluate_llm` timeouts. A full 1-epoch run on 4660 rows (1165 iters) would be ~17 h at this rate; even a 50-iter subset is ~43 min. The controller's `Quiesce` pauses RSIC LLM fan-out but the server+neo4j load remains. | live (M5 Max) | **[viability]** a real retrain wants the box mostly idle (or dedicated/off-peak); consider the loop scheduling retrains during quiet windows, and/or a smaller per-cycle row budget. Pipeline-mechanics validation uses a tiny subset (quality irrelevant for convert/gate). |
