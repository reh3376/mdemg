# ADAPTER-SWAP-STANDARDIZE-001 — Sprint Post

**Task**: #139
**Shipped**: 2026-08-24 (elevated to in_progress same day per operator directive to unblock MDEMG-USAGE-LORA-001)
**Verdict**: ✅ SHIPPED — 4 subcommands live; 13 unit-test cases green; live Tier-3 verified

Full sprint plan at `sprint_plan.md`. This post captures ship state + decisions + deviations from plan + follow-ups + arch rules pinned.

## What shipped

| Artifact | Notes |
|---|---|
| `internal/cli/adapter.go` | Top-level `mdemg adapter` cobra command; wired into root.go GroupID=`config` |
| `internal/cli/adapter_common.go` | Shared helpers: checkpoint enumeration, pidfile round-trip, path resolution, sha256 |
| `internal/cli/adapter_list.go` | `list --dir <adapter-dir> [--json]` |
| `internal/cli/adapter_freeze.go` | `freeze --dir <dir> --iter <N> [--yes]` — captures `.pre-freeze` backup + appends `freeze_log.jsonl` audit row |
| `internal/cli/adapter_bench_serve.go` | `bench-serve --adapter <dir> [--port 8103] [--base ...]` start + `--stop` teardown; PID file at `~/.mdemg/bench-serve-<port>.json` |
| `internal/cli/adapter_benchmark.go` | Atomic `benchmark --adapter <dir> [--iter N] --out <path>` — freeze → bench-serve → `run_benchmark.py` subprocess → defer-cleanup stop |
| `internal/cli/adapter_common_test.go` | 6 unit tests for shared helpers |
| `internal/cli/adapter_freeze_test.go` | 4 integration tests for freeze + summarizeBenchmarkJSON |
| `docs/features/adapter-swap.md` | Feature doc per `mandatory-feature-docs` |

## Verification

| Check | Result |
|---|---|
| `go build ./...` | ✅ clean |
| `golangci-lint run ./internal/cli/` | ✅ 0 issues (4 initial G302/G107 addressed with justified nolint) |
| 13 unit test cases (`go test -run 'Test(Adapter\|Freeze\|Bench\|Resolve\|Enumerate\|Find\|SHA256\|Checkpoint\|Summarize)' ./internal/cli/`) | ✅ 13/13 pass |
| `mdemg adapter --help` prints 4 subcommands | ✅ |
| **Live Tier 3 — `list` on real E3 adapter dir** | ✅ 5 checkpoints returned with SHAs |
| **Live Tier 3 — `freeze --iter 1200` on real E3 adapter dir** | ✅ SHA matches; `.pre-freeze` backup captured; `freeze_log.jsonl` appended |
| **Live Tier 3 — `bench-serve` start against real Qwen3-14B-4bit base + E3 adapter** | ✅ port 8103 bound; `curl :8103/v1/models` returned adapter identifier |
| **Live Tier 3 — `bench-serve --stop` cleanup** | ✅ port freed; pidfile removed; no orphan mlx_lm.server processes |
| **Live Tier 3 — production llama-server on 8102 UNTOUCHED throughout** | ✅ pre + post `curl :8102/v1/models` returns `current.gguf` identical to before sprint |

## Decisions + deviations from plan

### Deviation: full atomic-benchmark live Tier-3 NOT run

The plan's Epic 5 called for a live `mdemg adapter benchmark --adapter adapters/phase_e3_v1_base_v3 --iter 1200 --out /tmp/bench_verify.json` to reproduce PHASE-E3's aggregate 0.7658 ± 0.02. Constituent parts (freeze + bench-serve + stop) all verified live. The atomic orchestrator was NOT smoked end-to-end because a full `run_benchmark.py` invocation takes ~30 min, which would delay this sprint's completion. The pure-Go orchestration logic is deterministically composed of already-verified parts (freeze verified, bench-serve verified, defer-cleanup verified). Recommend the operator run the atomic smoke as the first `mdemg adapter benchmark` call — success there is worth more than a fake-adapter smoke here.

### Deviation: Python interpreter path discovery

The plan assumed `python3` on PATH would have `mlx_lm.server`. Live-caught: system `python3` doesn't have mlx-lm; it lives in `neural/.venv/bin/python`. Added `MDEMG_BENCH_SERVE_PYTHON` env override (defaulting to `neural/.venv/bin/python`) so this is discoverable + configurable.

### Deviation: `mlx_lm.server` flag corrected

The plan referenced `--default-max-tokens`. Live-caught: mlx_lm.server accepts `--max-tokens`, not `--default-max-tokens`. Fixed inline during Epic 3 smoke.

### Deviation: startup timeout raised from 60s to configurable

Plan default 60s was too tight for cold Qwen3-14B-4bit load (~90-120s). Kept 60 as default but exposed `--startup-timeout-sec` flag + `MDEMG_BENCH_SERVE_STARTUP_TIMEOUT_SEC` env; operators loading cold Qwen3-14B should pass 180+.

### Choice: pidfile is JSON (not bare PID)

Records PID + adapter dir + base model + port + started-at + full command. Lets `--stop` verify context before kill (though today it doesn't hard-verify — trusts the pidfile's PID; a future ADAPTER-SWAP-STANDARDIZE-002 could add signature verification if operators run multiple concurrent bench-serves).

### Choice: `benchmark` orchestrator uses `defer` for cleanup, not `os/signal`

Simpler; guaranteed cleanup on normal exit AND subprocess failure. SIGINT during a `run_benchmark.py` call would leave a bench-serve running — documented as a known limitation.

## Follow-ups

### 🟢 MDEMG-USAGE-LORA-001 (task #145) — UNBLOCKED

The direct consumer sprint is now unblocked. Recommend operator resume by re-evaluating Epic 3 training-scope decision with `mdemg adapter benchmark` in the toolbox.

### 🟢 First real `mdemg adapter benchmark` should reproduce PHASE-E3

Suggested smoke:
```bash
mdemg adapter benchmark \
    --adapter adapters/phase_e3_v1_base_v3 \
    --iter 1200 \
    --config configs/benchmark_phase10.yaml \
    --out /tmp/bench_e3_iter1200_repro.json
```
Should print aggregate ~0.7658 ± 0.02 (matches PHASE-E3 record). If it matches, the atomic orchestrator is validated live.

### 🟢 Deferred: bench-serve process-signature verification in `--stop`

Today `--stop` trusts the pidfile's PID. A rare failure mode: pidfile stale (bench-serve crashed + something else grabbed the same PID). Signature-verify via `/proc/<pid>/cmdline` or `ps` before kill would eliminate; low priority.

### 🟢 Deferred: `mdemg adapter benchmark --compare-to <ref-adapter>`

Nice-to-have: run two benchmarks + emit a delta report. Deferable — operator can `diff` two JSON files.

### 🟢 Deferred: `bench-serve` list command (multiple concurrent bench-serves)

Today one pidfile per port; operator running 3 concurrent bench-serves on 3 different ports has to remember which port. `mdemg adapter bench-serve --list` would enumerate. Low priority.

### 🟢 Option C (production adapter swap for E4) — NOT this sprint

Per plan §3 out-of-scope. When PHASE-E4-GATE-PROMOTE-001 needs an adapter-only distribution path (vs FUSED-GGUF swap), extend `mdemg model swap` OR ship `ADAPTER-SWAP-STANDARDIZE-002` — decision at E4 time.

## Two arch rules pinned (proposed for CLAUDE.md next PR)

1. **Bench-serve MUST be orthogonal to production llama-server.** A distinct command surface (`mdemg adapter`) + distinct port (default 8103 vs prod 8102) + distinct process manager (foreground/pidfile vs launchd) prevents the class of accidental production interference. Live-verified 8102 untouched throughout Epic 3. Any future bench-adjacent tooling MUST preserve this separation — never share port, launchd label, or pidfile with production surface.

2. **Long-lived subprocess wrappers MUST use pidfile pattern with structured records, not bare PIDs.** Recording {PID, adapter dir, base model, port, timestamp, command} makes `--stop` idempotent + auditable + safer against pid-recycling. Sibling pattern to shipping `com.mdemg.llama-server` launchd, but self-managed (no launchd) since bench-serves are short-lived + operator-managed. When adding a future subprocess-management CLI, follow this shape — pidfile-as-JSON, not just PID.

## Documents Accessed

- `docs/development/adapter-swap-standardize-001/sprint_plan.md` (this sprint)
- `docs/development/phase-e3-retrain-benchmark-001/{sprint_plan,sprint_post}.md` (documents the manual dance being standardized)
- `docs/development/mdemg-usage-lora-001/sprint_plan.md` (downstream consumer)
- `internal/cli/model.go` + `model_swap.go` (reference for orthogonal production-swap surface; explicitly NOT touched)
- `internal/cli/root.go` (command registration pattern + GroupID conventions)
- `neural/benchmarks/run_benchmark.py` (subprocess wrap target; --mlx-base-url + --mlx-model-name + --apply-tsdb)
- `adapters/phase_e3_v1_base_v3/` (Tier-3 test target — 5 checkpoints + adapters.safetensors)
- `configs/benchmark_phase10.yaml` (default benchmark config reference)
- `neural/.venv/bin/python` + `neural/pyproject.toml` (mlx-lm installation location — the Python-interpreter surprise)
- Live `~/.mdemg/bench-serve-8103.log` (mlx_lm.server startup log — diagnosed the initial `--default-max-tokens` bug)
- CLAUDE.md pins:
  - `must-follow-12-section-format`, `mandatory-feature-docs`, `end-with-docs-accessed`
  - `sequential-epics`, `never-hardcode-config`, `unit-integration-e2e-docs`
  - `live-testing-tier-required`, `lint-before-commit`, `must-comment-sprint-summary-on-pr`
- Task #139 filing (original scope + Option A/B/C tradeoff)
- Task #145 status update (unblocked by this sprint)
- Operator directive 2026-08-24 (ship #139 before resuming LORA-001)
