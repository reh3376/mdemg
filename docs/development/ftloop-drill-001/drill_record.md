# FTLOOP-DRILL-001 — Drill Record (2026-07-22 → 23)

**Cycle:** `gdd72ewed2t3vla9cnu7z1ft` | **Operator authorization:** "run the
ftloop drill overnight" (started early at operator prompt "Why wait till
23:37 to fire?")

## Timeline (UTC)

| Time | Event | Duration | Outcome |
|---|---|---|---|
| 19:47:28 | Cycle inserted (`triggered`, CUIDv2, Gate.OpenCycle row shape) | — | — |
| 19:47:38 | Controller consumed cycle (1 poll @30s); lease acquired; RSIC quiesced | — | ✓ |
| 19:47:38 | export (`mdemg data export`, 1-day window) | 794 ms | ✓ jobhealth `ft-loop:export` |
| 19:47:39 | curate (`paradigm_router`) → 814 train rows (sft_interactions + raft_retrieval) | 960 ms | ✓ `ft-loop:curate` |
| 19:47:40 | train (`mlx_lm.lora` via train_ft: 612 iters, epoch cap 3, rank 32/α64, base SHA `a54ec18f…` verified) | ~4h30m | ✓ checkpoints @100/200/300/…; 168 MB adapter |
| ~00:16 | fuse `--dequantize` → 28 GB bf16 | ~20 min | ✓ |
| 00:36:43 | convert-gguf | 7 s | **✗ `convert_failed`** — `can't open file convert_hf_to_gguf.py` |
| 00:36:43 | Failure alerting | — | ✓ `ft-loop:convert` HIGH + `cycle failed` MEDIUM (distinct services); ledger terminal `failed` |

## Defects caught (the drill's purpose) — both fixed same-night

1. **`resolveTool` PATH class** (commit `7c6ee1cb`): the convert stage used a
   bare `exec.LookPath("convert_hf_to_gguf.py")` — invisible under the
   launchd minimal PATH (the NOSILENT-001 dockerbin class; Epic 6 validated
   the command by hand in an interactive shell where `/opt/homebrew/bin`
   was on PATH). Fix: override (`FT_LOOP_CONVERT_SCRIPT`) → PATH →
   `/opt/homebrew/bin` → `/usr/local/bin` chain, applied to the convert
   script AND the gate stage's `llama-quantize` + `llama-server`
   (which would have failed next identically). Pin-tested.
2. **Missing `gguf` module** (commit `9e3d53f8`): the neural venv
   (`resolvePython`'s choice) couldn't import `gguf`, so the converter
   would have died even with the right path — Phase 13.5's manual
   conversion ran under a different python. Installed + pinned
   `gguf>=0.10` in `neural/pyproject.toml` [training]
   (the BENCH-SIDECAR-APPLY-001 psycopg precedent).

## Repaired-stage verification (against the real drill artifacts — no retrain)

- convert-gguf: 28 GB bf16 → **29.5 GB f16 GGUF** ✓ (~30 s write at ~1 GB/s)
- quantize: → **11 GB Q5_K_M candidate** ✓ (38 s)
- gate (stageGate's exact commands: side-port 18102 llama-server + scoped
  benchmark): **aggregate 0.8652 ≥ 0.80 floor — PASS; 100 rows, 0
  truncated** (no-zero-call discipline satisfied). The drill candidate
  would have reached `promote_pending`.

## Verdict

**Machinery PROVEN end-to-end.** Controller autonomy (ledger consume, lease,
quiesce, five supervised stages, failure alerting, terminal state) all
worked; the two convert-environment defects are exactly what a drill
exists to catch, and the repaired stages were verified against the real
artifacts. No promotion was performed (operator-only, per the hard rule);
the failed ledger state stands as the honest record of the original run.

## Teardown verification

- `.env` reverted (0 FT_LOOP lines) + kickstart + healthz ok
- Compute lease released; side-port 18102 free
- **Production llama-server pid 1265 UNCHANGED for the drill's entire
  duration**, still serving `mdemg-llm-v1.Q5_K_M.gguf`
- Transient artifacts (~70 GB: fused-bf16 28G + f16 29.5G + candidate 11G +
  adapter) left in `$TMPDIR/mdemg-ft-loop/gdd72ewed2t3vla9cnu7z1ft/` for OS
  tmp cleanup (the pre-bash guard blocks `rm -rf`; disclosed)

## Findings for FT-RECURSIVE-003

- The `ft-readiness` staleness alert (`training_readiness_stale`) fired
  ~30 min into the drill BECAUSE the quiesce worked — the readiness
  heartbeat pauses during a legitimate retrain. The staleness rule should
  be lease-aware (suppress or re-label while a cycle holds the lease).
- Ledger stage granularity: `train` covered 19:47→00:36 including the
  ~20-min fuse; a `converting` stage event before fuse would make the
  ledger timeline sharper.
- `scheduled-job-failure` (generic service) also fired for the convert
  failure alongside the dedicated `ft-loop` service alert — double-fire is
  by design (NOSILENT generic + FT-RECURSIVE-001 SF-3 specific), noted.
