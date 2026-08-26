# ADAPTER-SWAP-STANDARDIZE-001 — Sprint Plan (v1.0)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | ADAPTER-SWAP-STANDARDIZE-001 |
| Task | #139 |
| Filed | 2026-08-22 (during PHASE-E3 benchmark kickoff) |
| Elevated to in_progress | 2026-08-24 (operator directive; blocker for MDEMG-USAGE-LORA-001) |
| Author | Roger Henley + Claude (proactive mode) |
| Branch | `reh3376_dev01` |
| Predecessor | PHASE-E3-RETRAIN-BENCHMARK-001 (task #138) — 5× manual dance flagged as toil that will scale badly to future retrains |
| Successor blocker | MDEMG-USAGE-LORA-001 (task #145) — Epic 3 waits for this sprint |
| Format version | 12-section v1.0 |
| Est. wall-clock | ~1 dev-day (~6-8h focused) |
| Est. spend | $0 (local subprocess management only) |

## 2. Problem Statement

LoRA adapter A/B benchmarking today requires 5 manual steps per candidate:

1. `pkill mlx_lm.lora` / `launchctl bootout llama-server` (depending on target)
2. `cp adapters/<name>/000NNNN_adapters.safetensors adapters/<name>/adapters.safetensors` to freeze a specific iter
3. `launchctl bootstrap llama-server` OR launch `mlx_lm.server --model <base> --adapter-path <adapter> --port <alt>` in background
4. `curl :8103/v1/models` to confirm serving
5. `run_benchmark --mlx-base-url http://127.0.0.1:<port>/v1 --mlx-model-name <base-path>` with the correct alt endpoint override

Every candidate benchmarked in PHASE-E3 required manual invocation of all 5. MDEMG-USAGE-LORA-001 (task #145) will need this × N candidates. Future retrains × N iters × M candidates. **Manual toil scales badly; this sprint standardizes.**

Concrete blocker: task #145 Epic 3 (LoRA retrain, ~30-60h wall-clock) is paused until this sprint ships. During that training window, the operator will want to bench several checkpoints (0000400, 0000800, ...) to pick the best-val-loss adapter for E4. Without standardization, each check is 5-step manual toil.

## 3. Scope & Constraints

### In scope (Option B per task #139)

- `mdemg adapter list --dir <adapter-dir>` — enumerate checkpoints (iter + size + SHA + val_loss if manifest present)
- `mdemg adapter freeze --dir <adapter-dir> --iter <N> [--yes]` — cp `000NNNN_adapters.safetensors` → `adapters.safetensors`, verify SHA, log the pin
- `mdemg adapter bench-serve --adapter <dir> [--port 8103] [--base <base-model>] [--max-tokens 4000]` — spawn `mlx_lm.server` in background; write PID file at `~/.mdemg/bench-serve-<port>.pid`; print URL
- `mdemg adapter bench-serve --stop [--port 8103]` — teardown by PID; delete pidfile
- `mdemg adapter benchmark --adapter <dir> [--iter N] [--config configs/benchmark_phase10.yaml] --out <path> [--apply-tsdb]` — atomic orchestrator: freeze (if --iter) → bench-serve → invoke `run_benchmark.py` via subprocess → teardown → optional TSDB apply

### Out of scope (explicit)

- **NO extension to production adapter swap** (`mdemg model swap` / `rollback` per FT-RECURSIVE-003 gate) — that's Option C per task #139; deferred to E4 promotion sprint or `ADAPTER-SWAP-STANDARDIZE-002`
- **NO changes to `mdemg model swap`** — different concern (FUSED-GGUF swap for shipped runtime); DO NOT touch (this sprint's `mdemg adapter` surface is orthogonal)
- **NO changes to MODEL-DIST-001/002 fetcher plumbing** — different concern
- **NO wrapping of `run_benchmark --apply-tsdb`** — reuse verbatim (BENCH-SIDECAR-APPLY-001 pattern)
- **NO GUI / UI** — CLI only
- **NO cross-host support** — bench-serve is local (localhost binding + local PID file)

### Constraints (must obey)

- `plan-mode-before-change` — this plan BEFORE any code
- `sequential-epics` — Epic N completes before N+1
- `never-hardcode-config` — every knob CLI/env-overridable
- `mandatory-feature-docs` — new feature doc at Epic 6
- `end-with-docs-accessed` — all docs
- `must-follow-12-section-format` — this file
- `must-comment-sprint-summary-on-pr` — PR comment
- `lint-before-commit` — golangci-lint clean
- `unit-integration-e2e-docs` — 3 testing tiers
- `live-testing-tier-required` — Tier 3 with real E3 adapter + real `run_benchmark.py`
- `must-use-cuid2` — bench run IDs (CUIDv2, not UUID)
- Non-destructive: never mutate `adapters.safetensors` without preserving the pre-image (freeze cmd captures `adapters.safetensors` → `.pre-freeze.safetensors` backup)

## 4. Dependencies

| Dependency | Status | Notes |
|---|---|---|
| `mlx_lm.server` (mlx-community/mlx-lm) | ✅ installed | Launched via subprocess with `--model --adapter-path --port` |
| `neural/benchmarks/run_benchmark.py` | ✅ shipped | Reused verbatim via `python -m neural.benchmarks.run_benchmark …` subprocess |
| `.local-models/qwen3-14b-4bit-base/` | ✅ shipped | Default `--base` value |
| `adapters/phase_e3_v1_base_v3/` | ✅ shipped | Real Tier-3 test target (Epic 5) |
| PHASE-E3 benchmark result reference | ✅ recorded | `training_data/eval/e3_benchmark.json` — sprint reproduces its aggregate 0.7658 as validation |
| llama-server on port 8102 (production) | ✅ live | Bench-serve MUST use different port (default 8103) to avoid collision |
| pgrep/pkill / launchctl | ✅ macOS | Used for process teardown |

**No blocking dependencies.**

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Core Go CLI scaffolding (~2h)

Deliverables:
- `internal/cli/adapter.go` — top-level `newAdapterCmd()` cobra command; wired into `root.go` GroupID=`config` (matches `model`/`config`/`hooks`)
- `internal/cli/adapter_list.go` — `list` subcommand
- `internal/cli/adapter_freeze.go` — `freeze` subcommand
- `internal/cli/adapter_bench_serve.go` — `bench-serve` subcommand (start + `--stop` mode)
- `internal/cli/adapter_benchmark.go` — `benchmark` subcommand (atomic orchestrator)
- Shared helpers `internal/cli/adapter_common.go`:
  - `resolveAdapterDir(path string) (string, error)`
  - `enumerateCheckpoints(dir string) ([]checkpoint, error)`
  - `benchServePidFile(port int) string`
  - `writePidFile(path string, pid int) error`
  - `readPidFile(path string) (int, error)`

Gate: `go build ./...` clean; `mdemg adapter --help` prints the 4 subcommands.

### Epic 2 — `list` + `freeze` (~1h)

- `list --dir <dir>` walks `<dir>/*_adapters.safetensors`, prints table (iter, size, sha256[:12], mtime)
- `freeze --dir <dir> --iter <N> [--yes]`:
  - Locates `<dir>/000NNNN_adapters.safetensors` (zero-padded lookup)
  - If `<dir>/adapters.safetensors` exists, back up to `<dir>/adapters.safetensors.pre-freeze` (preserves reversibility)
  - `cp` the checkpoint → `adapters.safetensors`
  - Emit SHA-256 + iter + timestamp to `<dir>/freeze_log.jsonl` (append-only audit trail)
  - Print result JSON to stdout

Gate: `mdemg adapter list --dir adapters/phase_e3_v1_base_v3` returns 5 checkpoints; `mdemg adapter freeze --dir adapters/phase_e3_v1_base_v3 --iter 1200 --yes` produces expected SHA + audit row.

### Epic 3 — `bench-serve` (~2h)

- Start mode `bench-serve --adapter <dir> [--port 8103] [--base <base>] [--max-tokens 4000]`:
  - Resolves base model path (default `.local-models/qwen3-14b-4bit-base`; overrideable via `--base` or `MDEMG_BENCH_SERVE_BASE`)
  - Checks port not already bound → early-fail if bound
  - Checks PID file exists → early-fail with instruction (`--stop` first)
  - Launches `python -m mlx_lm.server --model <base> --adapter-path <dir> --port <port>` as a detached process (`SysProcAttr.Setsid=true`)
  - Writes PID + adapter dir + timestamp to `~/.mdemg/bench-serve-<port>.json` (not just PID — records what's serving)
  - Polls `curl http://127.0.0.1:<port>/v1/models` for up to 60s; success → print URL + exit 0; timeout → kill spawned pid + fail
- Stop mode `bench-serve --stop [--port 8103]`:
  - Reads pidfile; kills process; deletes pidfile
  - Idempotent — no-op if pidfile absent

Env overrides: `MDEMG_BENCH_SERVE_PORT` (default 8103), `MDEMG_BENCH_SERVE_BASE` (default .local-models path), `MDEMG_BENCH_SERVE_STARTUP_TIMEOUT_SEC` (default 60)

Gate: `bench-serve --adapter <e3-dir> --port 8103` completes in < 60s and `curl :8103/v1/models` returns the adapter identifier; `bench-serve --stop --port 8103` cleans up.

### Epic 4 — `benchmark` (atomic orchestrator, ~1h)

`benchmark --adapter <dir> [--iter N] [--config configs/benchmark_phase10.yaml] --out <path> [--apply-tsdb]`:

1. If `--iter`, invoke freeze (Epic 2)
2. Invoke bench-serve (Epic 3)
3. Invoke `python -m neural.benchmarks.run_benchmark --config <config> --out <path> --mlx-base-url http://127.0.0.1:<port>/v1 --mlx-model-name <base-name> [--apply-tsdb]` via subprocess; forward stdout/stderr; capture exit code
4. Regardless of subprocess success, invoke bench-serve --stop (finally block for cleanup)
5. If subprocess failed, exit non-zero + print error
6. Print benchmark result summary (aggregate + per-task from the emitted JSON)

Env: reuses Epic 3's overrides; adds `MDEMG_BENCH_TIMEOUT_SEC` (default 3600 = 1h max wall for the run_benchmark subprocess)

Gate: `mdemg adapter benchmark --adapter adapters/phase_e3_v1_base_v3 --iter 1200 --out /tmp/bench_smoke.json` runs end-to-end + reproduces PHASE-E3's aggregate ± 0.02 tolerance.

### Epic 5 — Unit + integration tests + Live Tier-3 (~1h)

Unit (Go):
- `TestEnumerateCheckpoints` — fixture dir with 3 checkpoints → 3 rows sorted by iter
- `TestFreezeChecksum` — freeze rewrites adapters.safetensors + emits audit row + backup exists
- `TestPidfileRoundTrip` — write + read + delete idempotent
- `TestResolveAdapterDir` — absolute + relative + non-existent

Integration:
- `TestFreezeCreatesBackupWhenAdaptersExists` — freeze on a dir that already has adapters.safetensors → backup file with pre-freeze SHA

Live Tier-3:
- `mdemg adapter list --dir adapters/phase_e3_v1_base_v3` — see 5 checkpoints
- `mdemg adapter freeze --dir <same> --iter 1200 --yes` — verify SHA matches original
- `mdemg adapter bench-serve --adapter <same> --port 8103` — verify `curl :8103/v1/models` returns
- `mdemg adapter benchmark --adapter <same> --config configs/benchmark_phase10.yaml --out /tmp/bench_verify.json` — verify aggregate reproduces PHASE-E3's 0.7658 ± 0.02 (proves atomic orchestration matches the manual dance)
- Cleanup: `mdemg adapter bench-serve --stop --port 8103`; verify port free

### Epic 6 — Feature doc + sprint post + PR + follow-ups (~1h)

- `docs/features/adapter-swap.md` — new feature doc per `mandatory-feature-docs`
- `docs/development/adapter-swap-standardize-001/sprint_post.md`
- CHANGELOG.md Unreleased entry
- PR summary comment
- Update task #145's blocked-by to "unblocked" (this sprint ships → LORA-001 can resume)

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests (Go, `go test`)

- `internal/cli/adapter_common_test.go` — pure-function tests (checkpoint enumeration, pidfile round-trip, resolveAdapterDir)
- `internal/cli/adapter_freeze_test.go` — freeze idempotency + backup creation
- Target: 4-6 tests, ~50 LoC each

### Tier 2 — Integration test

- `TestAdapterBenchServeStartStop`: real subprocess spawn against a FAKE mlx_lm.server (a tiny python http.server fixture that answers /v1/models); verifies start blocks until 200; verifies --stop kills the PID
- Uses `t.TempDir()` for pidfile isolation

### Tier 3 — Live e2e on real system (`live-testing-tier-required`)

- Real `mdemg adapter benchmark --adapter adapters/phase_e3_v1_base_v3 --iter 1200 --out /tmp/bench_verify.json`
- Verify: run completes, JSON produced, aggregate ~0.7658 (PHASE-E3 baseline; ± 0.02 for reproducibility)
- Verify: `bench-serve --stop` cleanly frees port 8103
- Verify: production llama-server on port 8102 UNTOUCHED (SHA of `~/Library/LaunchAgents/com.mdemg.llama-server.plist` unchanged; `curl :8102/v1/models` still healthy)

## 7. Commit Strategy

Sequential commits:

1. `feat(cli): ADAPTER-SWAP-STANDARDIZE-001 Epic 1-2 — adapter list + freeze subcommands`
2. `feat(cli): ADAPTER-SWAP-STANDARDIZE-001 Epic 3 — adapter bench-serve + pidfile lifecycle`
3. `feat(cli): ADAPTER-SWAP-STANDARDIZE-001 Epic 4 — atomic adapter benchmark orchestrator`
4. `docs(cli): ADAPTER-SWAP-STANDARDIZE-001 Epic 5-6 — tests + feature doc + sprint post`

All commits lint-clean per `lint-before-commit`. Auto-PR to main via existing workflow.

## 8. Verification Checklist

- [ ] Epic 1: `go build ./...` clean; `mdemg adapter --help` shows list/freeze/bench-serve/benchmark
- [ ] Epic 2: `list` on real E3 adapter dir returns 5 checkpoints with SHAs; `freeze --iter 1200` emits audit + backup
- [ ] Epic 3: `bench-serve` starts + `curl :8103/v1/models` returns adapter within 60s; `bench-serve --stop` cleans up
- [ ] Epic 4: `benchmark --adapter adapters/phase_e3_v1_base_v3 --iter 1200` reproduces PHASE-E3 aggregate (~0.7658 ± 0.02)
- [ ] Epic 5: 4-6 unit tests pass; 1 integration test passes; live Tier-3 all green
- [ ] Epic 6: feature doc + sprint post + CHANGELOG + PR comment
- [ ] Production `llama-server` on port 8102 UNTOUCHED (verify `curl :8102/v1/models` still healthy pre + post sprint)
- [ ] Task #139 → completed
- [ ] Task #145 unblocked (blocked-by list updated)
- [ ] `golangci-lint run ./...` clean

## 9. Documentation Update

- `docs/features/adapter-swap.md` (new)
- `docs/development/adapter-swap-standardize-001/sprint_post.md` (new)
- CHANGELOG.md Unreleased entry
- PR summary comment
- Update `docs/development/mdemg-usage-lora-001/sprint_plan.md` Epic 3 to reference `mdemg adapter benchmark` instead of the manual dance

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| `bench-serve` collides with production llama-server (port 8102) | Default port 8103; explicit check that port not bound before spawn |
| `bench-serve` leaves orphan mlx_lm.server processes on crash | Pidfile at `~/.mdemg/bench-serve-<port>.json` includes command signature; `--stop` verifies before kill; `benchmark` uses defer-cleanup pattern |
| `freeze` overwrites production adapters.safetensors | Freeze target is BENCH adapter dir only (e.g. `adapters/phase_e3_v1_base_v3/`), NOT the production adapter path used by launchd's llama-server. Documented in feature doc. Backup file `.pre-freeze.safetensors` preserves the pre-image. |
| Subprocess launch fails on missing python env | Error message names the required env; fail fast |
| `run_benchmark.py` subprocess hangs | `MDEMG_BENCH_TIMEOUT_SEC` (default 3600) enforces upper bound; SIGKILL on timeout |
| Live Tier-3 needs actual mlx_lm.server + Qwen3-14B-4bit loaded (~10 GB) | Documented as required infra; only Tier 3 needs it (Tier 1+2 use httptest fixtures) |
| Concurrent bench-serve on same port | pidfile presence check errors early with fix hint |
| Adapter dir contains no `000NNNN_adapters.safetensors` (fresh dir) | `list` returns empty table (no error); `freeze --iter N` errors with "no such checkpoint" |
| `benchmark` interrupts mid-run leaves bench-serve running | `context.WithCancel` + defer stop; SIGINT handler stops before exit |

## 11. Documents Accessed

- `docs/development/phase-e3-retrain-benchmark-001/sprint_plan.md` (documents the manual dance being standardized)
- `docs/development/mdemg-usage-lora-001/sprint_plan.md` (the downstream sprint waiting on this one)
- `internal/cli/model.go` (subcommand pattern reference for `newAdapterCmd`)
- `internal/cli/model_swap.go` (production-swap pattern; do NOT duplicate)
- `internal/cli/root.go` (command registration pattern + GroupID)
- `neural/benchmarks/run_benchmark.py` (subprocess wrap target; `--mlx-base-url` + `--mlx-model-name` + `--apply-tsdb` flags)
- `adapters/phase_e3_v1_base_v3/` (Tier-3 test target — 5 checkpoints; adapters.safetensors)
- `configs/benchmark_phase10.yaml` (default benchmark config)
- CLAUDE.md pins:
  - `must-follow-12-section-format`, `mandatory-feature-docs`, `end-with-docs-accessed`
  - `sequential-epics`, `never-hardcode-config`, `unit-integration-e2e-docs`
  - `live-testing-tier-required`, `lint-before-commit`, `must-comment-sprint-summary-on-pr`
  - `must-use-cuid2` (bench run IDs; not necessary if we don't mint any — decision at Epic 4)
- Task #139 description (proposed CLI shape + Option A/B/C tradeoff)
- Task #145 description (this sprint's downstream consumer)
- Operator directive 2026-08-24 (proceed with this sprint before resuming LORA-001)

## 12. Rollback Procedures (destructive ops)

**Destructive surface**: subprocess kill (bench-serve --stop); file overwrite (freeze).

- `bench-serve --stop` kills a PID we spawned — reversible: rerun `bench-serve` starts a fresh one
- `freeze` copies checkpoint over `adapters.safetensors` — reversible: `cp <dir>/adapters.safetensors.pre-freeze <dir>/adapters.safetensors`
- `benchmark` orchestrator: any partial state (bench-serve up, benchmark JSON absent) recoverable via `bench-serve --stop` + rerun

**Zero substrate mutation** — no Neo4j writes; no TSDB writes (except `--apply-tsdb` passthrough to run_benchmark, which is opt-in + already reversible per BENCH-SIDECAR-APPLY-001)

**Full sprint rollback**:

```bash
# Remove new CLI files (if partially committed)
rm internal/cli/adapter*.go
rm docs/features/adapter-swap.md
rm -rf docs/development/adapter-swap-standardize-001/
# Revert wiring in internal/cli/root.go
# Everything is additive; no schema/config file mutations elsewhere
```

**Production llama-server protection**: this sprint MUST NOT touch `~/Library/LaunchAgents/com.mdemg.llama-server.plist` or the `com.mdemg.llama-server` launchd label. Bench-serve is a DIFFERENT process on a DIFFERENT port. Live Tier-3 explicitly verifies port 8102 untouched.
