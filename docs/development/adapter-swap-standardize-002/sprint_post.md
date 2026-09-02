# ADAPTER-SWAP-STANDARDIZE-002 — Sprint Post

**Task**: #146
**Completed**: 2026-09-02 (~1h wall-clock)
**Verdict**: ✅ SHIPPED — SIGTERM/SIGINT to `mdemg adapter benchmark` now triggers defer-cleanup; live-verified via real E3 adapter smoke.

Full plan at `sprint_plan.md`. Ship state + verification + arch rule pinned below.

## What shipped

| Artifact | Notes |
|---|---|
| `internal/cli/adapter_benchmark.go` | Added `signal.Notify(SIGTERM, SIGINT)` handler + goroutine `signalCleanupBenchmark` (extracted to standalone function for testability); `benchStarted atomic.Bool` guard; cleanup defer + `close(sigCh)` + `<-signalDone` on normal return; new imports `os/signal`, `sync/atomic`, `syscall` |
| `internal/cli/adapter_benchmark_signal_test.go` | 3 unit tests: cleanup fires on signal, normal shutdown exits cleanly, benchStarted guard prevents cleanup when bench never started |
| `docs/features/adapter-swap.md` | New §Guarantees entry documenting SIGTERM cleanup contract |
| `docs/development/adapter-swap-standardize-002/{sprint_plan,sprint_post}.md` | Sprint records |

## Verification

| Check | Result |
|---|---|
| `go build ./...` | ✅ clean |
| `golangci-lint run ./internal/cli/` | ✅ 0 issues |
| 3 unit tests (`-race`) | ✅ 3/3 pass |
| Full `internal/cli` test suite | ✅ no regressions |
| **Live Tier-3 SIGTERM smoke** | ✅ port 8103 freed + pidfile removed + no orphan mlx_lm.server + wrapper log shows `== bench-serve stop (port=8103)` |
| Production llama-server on port 8102 UNTOUCHED throughout | ✅ verified pre + post |
| `mdemg adapter benchmark --help` unchanged | ✅ (additive; no CLI surface change) |

**Live smoke trace**:
- Kicked off real `mdemg adapter benchmark --adapter adapters/phase_e3_v1_base_v3 --iter 1200 --out /tmp/sigterm_smoke_bench.json` via `run_in_background: true`
- Waited for bench-serve up on 8103 (poll `/v1/models`)
- Sent SIGTERM to the mdemg binary PID (`pgrep -f "^./bin/mdemg adapter benchmark"`)
- Within 5s: pidfile removed, port 8103 free, no orphan mlx_lm.server processes, wrapper exited, log showed the cleanup line
- Production llama-server on 8102 still serving current.gguf unchanged

## Sprint execution — one live-caught lesson

**Live smoke initial attempt hit a targeting issue**: my first `pgrep -f "bin/mdemg adapter benchmark"` returned BOTH the shell wrapper PID (spawned by `Bash tool run_in_background`) AND the actual mdemg binary PID; I SIGTERM'd the wrapper (which caused exit 144 in the harness) but the mdemg binary kept running with bench-serve alive. Corrected with `pgrep -f "^./bin/mdemg adapter benchmark"` (anchored) — targeted only the Go binary, and the signal handler fired correctly.

**Not a bug in the code being tested** — the code works. **A test-methodology lesson**: when signal-testing a Go binary spawned by a shell wrapper (harness `run_in_background`), always target the binary PID directly, not any shell parent. Pinned as a live-smoke arch note.

## Arch rule pinned (proposed for CLAUDE.md next PR)

**Long-running Go CLI orchestrators that manage subprocess lifecycles MUST install `signal.Notify(SIGTERM, SIGINT)` handlers that trigger the same cleanup path as `defer`.** Go's `defer` runs on normal return + panic but NOT on signal-based termination. Without a signal handler, `SIGTERM` (from operator ^C, `pkill`, Bash tool 10-min ceiling, etc.) leaves orphan state (pidfiles, subprocess pgroups, temp resources). The correct shape:

1. `signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)` — subscribe
2. `go signalCleanup(sigCh, doneCh, benchStarted, resources...)` — extract cleanup logic to a standalone testable function
3. Guard cleanup with a "started" flag (atomic.Bool) so early signals before setup are safe no-ops
4. Re-raise the signal via `signal.Reset` + `syscall.Kill(os.Getpid(), sig)` after cleanup so the process exits with the expected signal-terminated status
5. On normal return: `signal.Stop(sigCh)` + `close(sigCh)` + wait on `doneCh` for goroutine exit
6. Unit-test the extracted cleanup function directly (signal delivery timing is fragile in Go tests)

Applies to every future `mdemg` subcommand that spawns long-lived subprocesses (bench-serve orchestrators, training-run wrappers, ingest jobs, background compilation, etc.). Extends `mdemg adapter benchmark` today; `mdemg adapter bench-serve` doesn't need it because it's short-lived (start OR stop, not both in one invocation).

## Follow-ups

None. Sprint complete; single arch rule pinned; live-verified; feature doc updated.

## Documents Accessed

- `docs/development/adapter-swap-standardize-002/sprint_plan.md` (this sprint)
- `docs/development/adapter-swap-standardize-001/{sprint_plan,sprint_post}.md` (#139 predecessor — documented the SIGTERM defer gap as known limitation)
- `docs/development/mdemg-usage-lora-001/{sprint_plan,sprint_post,verdict}.md` (#145 — live-hit that motivated this sprint)
- `docs/features/adapter-swap.md` (target of doc update)
- `internal/cli/adapter_benchmark.go` (target of code change; added signal handler + extracted cleanup function)
- `internal/cli/adapter_bench_serve.go` (`stopBenchServe` used verbatim by cleanup path)
- `internal/cli/adapter_common.go` (`benchServePidFile`, `benchServePidRecord`, `writePidRecord` — used by test)
- `internal/cli/adapter_benchmark_signal_test.go` (NEW — 3 unit tests)
- Go stdlib: `os/signal`, `sync/atomic`, `syscall`
- Live processes: real `mdemg adapter benchmark` subprocess + `mlx_lm.server` on port 8103 + production llama-server on port 8102 (verified untouched)
- CLAUDE.md pins (§3 of sprint plan)
- Operator directive 2026-09-02 ("proceed with #146")
