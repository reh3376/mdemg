# `mdemg adapter` — LoRA A/B Benchmark Helper

**Sprint**: ADAPTER-SWAP-STANDARDIZE-001 (2026-08-24) · task #139
**Status**: shipped — standardizes the 5-step manual dance PHASE-E3 flagged during A/B benchmark work
**Command surface**: `mdemg adapter {list,freeze,bench-serve,benchmark}`

## Why

PHASE-E3-RETRAIN-BENCHMARK-001 (task #138) revealed that LoRA adapter A/B benchmarking required ~5 error-prone manual steps per candidate:

1. `pkill mlx_lm.lora` / `launchctl bootout llama-server` (depending on target)
2. `cp adapters/<name>/000NNNN_adapters.safetensors adapters/<name>/adapters.safetensors` to freeze a specific iter
3. `launchctl bootstrap llama-server` OR launch `mlx_lm.server --model <base> --adapter-path <adapter> --port <alt>` in background
4. `curl :8103/v1/models` to confirm serving
5. `run_benchmark --mlx-base-url http://127.0.0.1:<port>/v1 --mlx-model-name <base-path>` with the correct alt endpoint override

Every candidate benchmarked in E3 required all 5. MDEMG-USAGE-LORA-001 (task #145) and every future retrain × N iters × M candidates would repeat the same toil. This sprint replaces the manual sequence with a single command.

## Choices

### Orthogonal to `mdemg model swap`

`mdemg model swap` / `mdemg model rollback` govern the PRODUCTION FUSED-GGUF llama-server on port 8102 via the FT-RECURSIVE-003 gate. `mdemg adapter` is a DIFFERENT concern — bench mlx_lm.server on an alt port (default 8103) for A/B benchmarking WITHOUT touching launchd `com.mdemg.llama-server`. The two are byte-non-overlapping code paths; do NOT conflate them.

### Bench port defaults to 8103

Production llama-server is on 8102 by shipping convention. Bench-serve MUST use a different port; the code enforces this at start (checks port not already bound). Env override: `--port` flag or `MDEMG_BENCH_SERVE_PORT`.

### Python interpreter defaults to `neural/.venv/bin/python`

`mlx_lm.server` is installed in the neural training venv per `neural/pyproject.toml [training]`. Env override: `MDEMG_BENCH_SERVE_PYTHON`. Discovered live during sprint execution — system `python3` did not have `mlx_lm` installed.

### Freeze captures `.pre-freeze` backup

Every `freeze` that overwrites an existing `adapters.safetensors` first copies the pre-image to `adapters.safetensors.pre-freeze` (one backup slot per dir; subsequent freezes overwrite the backup — preserve manually if you need multi-generation history). Reversible: `cp adapters/<dir>/adapters.safetensors.pre-freeze adapters/<dir>/adapters.safetensors`.

### Append-only audit trail

Every `freeze` appends a row to `<dir>/freeze_log.jsonl` with iter + SHA-256 + timestamp + backup path/SHA if captured. Machine-readable; grows one line per pin.

### `benchmark` is atomic

`mdemg adapter benchmark` combines steps 1-5 into one command with `defer`-cleanup so bench-serve always stops even if `run_benchmark.py` fails mid-run.

## How it works

```
mdemg adapter list        → enumerate NNNN_adapters.safetensors checkpoints
mdemg adapter freeze      → cp 000NNNN → adapters.safetensors + backup + audit
mdemg adapter bench-serve → spawn mlx_lm.server on --port (writes ~/.mdemg/bench-serve-<port>.json)
mdemg adapter bench-serve --stop → SIGTERM pgid from pidfile + remove pidfile
mdemg adapter benchmark   → freeze (if --iter) → bench-serve → run_benchmark → stop (deferred)
```

### `bench-serve` process lifecycle

1. Spawn `python -m mlx_lm.server --model <base> --adapter-path <dir> --port <port> [--max-tokens N]` via `Setsid=true` so it's an independent process group
2. Write `~/.mdemg/bench-serve-<port>.json` with PID + adapter dir + base + port + started-at + full command (for audit)
3. Poll `http://127.0.0.1:<port>/v1/models` up to `--startup-timeout-sec` (default 60; 180 recommended for cold Qwen3-14B-4bit load); success → print result JSON + exit 0
4. On timeout: SIGTERM the process group + remove pidfile + return error (never leaves orphans)

### `bench-serve --stop`

Reads pidfile, sends SIGTERM to the process group (kills mlx_lm.server + all subprocesses), waits up to 2.5s for graceful exit, best-effort SIGKILL if still alive, removes pidfile. Idempotent — no-op if pidfile absent.

### `benchmark` atomic sequence

```
1. If --iter, freeze (backup existing adapters.safetensors)
2. bench-serve start
3. Run: python -m neural.benchmarks.run_benchmark --config X --out Y --mlx-base-url http://127.0.0.1:PORT/v1 --mlx-model-name BASE [--apply-tsdb]
4. defer: bench-serve --stop (ALWAYS runs, even if step 3 errors)
5. Print summary (aggregate + per-task means from the emitted JSON)
```

## How to use

### List checkpoints in an adapter dir

```bash
mdemg adapter list --dir adapters/phase_e3_v1_base_v3
# → ITER SIZE       SHA256[:12]   MTIME_UTC             PATH
#   400  513864405  77414e5e18ac  2026-08-22T01:58:23Z  ...
#   800  ...
#   1200 ...
mdemg adapter list --dir <dir> --json    # machine-readable
```

### Freeze a specific iter

```bash
mdemg adapter freeze --dir adapters/phase_e3_v1_base_v3 --iter 1200 --yes
# Refuses without --yes if adapters.safetensors already exists
# Emits result JSON + appends row to freeze_log.jsonl
```

### Start / stop bench-serve

```bash
# Start
mdemg adapter bench-serve --adapter adapters/phase_e3_v1_base_v3 \
                         --port 8103 --startup-timeout-sec 180
# → {"status": "up", "pid": ..., "url": "http://127.0.0.1:8103/v1/models"}

# Verify
curl -s http://127.0.0.1:8103/v1/models

# Stop
mdemg adapter bench-serve --stop --port 8103
```

### Atomic benchmark

```bash
mdemg adapter benchmark \
    --adapter adapters/phase_e3_v1_base_v3 \
    --iter 1200 \
    --config configs/benchmark_phase10.yaml \
    --out training_data/eval/e3_iter1200_bench.json \
    --apply-tsdb
# Runs full sequence: freeze → bench-serve → run_benchmark → stop
# Prints benchmark summary + writes JSON to --out
```

## Environment variables

| Env | Default | Purpose |
|---|---|---|
| `MDEMG_BENCH_SERVE_PORT` | 8103 | Default bench port (production llama-server is 8102; NEVER collide) |
| `MDEMG_BENCH_SERVE_BASE` | `.local-models/qwen3-14b-4bit-base` | Default base model path |
| `MDEMG_BENCH_SERVE_PYTHON` | `neural/.venv/bin/python` | Python interpreter with `mlx_lm.server` installed |
| `MDEMG_BENCH_SERVE_STARTUP_TIMEOUT_SEC` | 60 | Poll timeout for readiness (bump to 180+ for cold 14B load) |
| `MDEMG_BENCH_TIMEOUT_SEC` | 3600 | Max wall-time for `run_benchmark` subprocess in `benchmark` orchestrator |

## Guarantees

- `mdemg adapter` NEVER touches `~/Library/LaunchAgents/com.mdemg.llama-server.plist`
- `mdemg adapter` NEVER touches port 8102 (production llama-server)
- `mdemg adapter` NEVER touches `mdemg model` subcommand surface (production FUSED-GGUF swap)
- `bench-serve --stop` is idempotent (no-op if pidfile absent)
- `freeze --yes` captures pre-image backup before overwrite (reversible)
- `benchmark` defer-cleanup ensures bench-serve always stops even on subprocess failure
- Port collision on bench-serve start → early fail (checks port free before spawn)
- Pidfile collision on bench-serve start → early fail with fix hint (`--stop` first)

## Rollback

```bash
# Restore pre-freeze adapters.safetensors
cp adapters/<dir>/adapters.safetensors.pre-freeze adapters/<dir>/adapters.safetensors

# Stop any orphan bench-serve
mdemg adapter bench-serve --stop --port 8103

# Nuclear option: remove entire feature surface (not typically needed)
rm internal/cli/adapter*.go
# Revert wiring in internal/cli/root.go
```

## References

- Sprint plan + post: `docs/development/adapter-swap-standardize-001/`
- Predecessor: `docs/development/phase-e3-retrain-benchmark-001/` (documents the manual dance being standardized)
- Downstream consumer: `docs/development/mdemg-usage-lora-001/` (task #145; Epic 3 uses `mdemg adapter benchmark`)
- `docs/features/local-model-distribution.md` (adjacent: production `mdemg model` subcommands; do NOT touch from `mdemg adapter`)
