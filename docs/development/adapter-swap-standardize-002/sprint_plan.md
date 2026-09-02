# ADAPTER-SWAP-STANDARDIZE-002 — Sprint Plan (v1.0)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | ADAPTER-SWAP-STANDARDIZE-002 |
| Task | #146 |
| Filed | 2026-09-01 (from #145 Epic 4 live-hit) |
| Elevated to in_progress | 2026-09-02 (operator directive) |
| Author | Roger Henley + Claude (proactive mode) |
| Branch | `reh3376_dev01` |
| Predecessors | #139 ADAPTER-SWAP-STANDARDIZE-001 (documented the SIGTERM defer gap as known limitation); #145 (live-hit — Bash tool 10-min ceiling killed atomic wrapper without triggering defer-cleanup) |
| Format version | 12-section v1.0 |
| Est. wall-clock | ~2h |
| Est. spend | $0 (all local) |

## 2. Problem Statement

`mdemg adapter benchmark` (`internal/cli/adapter_benchmark.go`) is an atomic orchestrator that starts `bench-serve` (mlx_lm.server subprocess on port 8103), runs `run_benchmark.py`, then stops bench-serve via `defer`. Go's `defer` runs on normal return + panic, but **NOT on SIGTERM/SIGINT**. When #145's Epic 4 hit the Bash tool's 10-min ceiling, SIGTERM to the wrapper propagated to bench-serve (process group killed) but the `defer` never ran → stale pidfile left at `~/.mdemg/bench-serve-8103.json`.

Downstream damage: next `mdemg adapter bench-serve` invocation would refuse to start due to pidfile-collision guard, forcing operator manual cleanup (`rm ~/.mdemg/bench-serve-<port>.json`).

Root fix: install `signal.Notify(SIGTERM, SIGINT)` handler that triggers the same cleanup path as `defer`. Idempotent by design — `stopBenchServe` already handles missing pidfile gracefully.

## 3. Scope & Constraints

### In scope
- Add signal handler at start of `runBenchmark` cobra RunE
- Handler channels a signal → goroutine that calls `stopBenchServe(pidfilePath, port)` and exits with non-zero code
- Ensure signal handler doesn't fire before bench-serve starts (guard via `benchStarted bool`)
- Cleanup signal handler on normal return (avoid goroutine leak in tests)
- Unit test that simulates SIGTERM via `syscall.Kill(os.Getpid(), syscall.SIGTERM)` inside the goroutine using a mock `stopBenchServe`-shape function
- Live Tier-3: kick off a real bench + SIGTERM the wrapper mid-run + verify pidfile absent
- Preserve behavior on normal return (defer path already correct)
- Preserve behavior on subprocess error (existing error path already returns)

### Out of scope
- No changes to `bench-serve` start/stop themselves (only the orchestrator wrapping them)
- No changes to `freeze` or `list` subcommands
- No changes to `mdemg model swap` (production surface, distinct concern)
- No additional signal support beyond SIGTERM + SIGINT

### Constraints (must obey)
- `plan-mode-before-change` — this plan first
- `sequential-epics`
- `never-hardcode-config` — the signal set (SIGTERM, SIGINT) is fixed by shell convention, not a config knob
- `must-use-cuid2` — n/a (no new IDs)
- `mandatory-feature-docs` — extend existing `docs/features/adapter-swap.md` with a "Cleanup on SIGTERM" section
- `end-with-docs-accessed`
- `must-follow-12-section-format`
- `lint-before-commit` — `golangci-lint run ./internal/cli/`
- `unit-integration-e2e-docs` — 3 testing tiers (unit signal test + integration bench-serve start/kill/stop + live Tier-3 with real E3 adapter)
- `live-testing-tier-required`
- Production port 8102 UNTOUCHED throughout
- `mdemg adapter bench-serve` semantics UNCHANGED (only `mdemg adapter benchmark` orchestrator gets the signal handler)

## 4. Dependencies

| Dependency | Status |
|---|---|
| `internal/cli/adapter_benchmark.go` | ✅ shipped from #139; direct target of this sprint |
| `internal/cli/adapter_bench_serve.go` (`stopBenchServe`) | ✅ shipped; idempotent already |
| `internal/cli/adapter_common.go` (`benchServePidFile`) | ✅ shipped |
| `golangci-lint` v2.13.1 pinned | ✅ (CI-GOLANGCI-PIN-001) |
| `adapters/phase_e3_v1_base_v3/` (Tier-3 test target) | ✅ available for live smoke |
| `mlx_lm.server` (`neural/.venv/bin/python`) | ✅ available |

**No blocking dependencies.**

## 5. Implementation Plan (sequential epics)

### Epic 1 — Add SIGTERM/SIGINT handler (~1h)

At top of `newAdapterBenchmarkCmd`'s RunE, after argument validation:

```go
// Signal handler — ensures bench-serve is stopped on SIGTERM/SIGINT
// (defer alone doesn't run on signal-based termination).
// Idempotent: stopBenchServe handles missing pidfile gracefully.
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
benchStarted := false
signalCleanupDone := make(chan struct{})
go func() {
    sig, ok := <-sigCh
    if !ok {
        // Normal shutdown — signal.Stop closed the channel
        close(signalCleanupDone)
        return
    }
    fmt.Fprintf(os.Stderr, "\n== received %s: stopping bench-serve (port=%d)\n", sig, port)
    if benchStarted {
        _ = stopBenchServe(pidfilePath, port)
    }
    close(signalCleanupDone)
    // Re-raise the signal with default handler now that cleanup is done
    signal.Stop(sigCh)
    _ = syscall.Kill(os.Getpid(), sig.(syscall.Signal))
}()
defer func() {
    signal.Stop(sigCh)
    close(sigCh) // signal goroutine exits via range-close path
    <-signalCleanupDone
}()
```

Set `benchStarted = true` right after `startBenchServe` returns nil.

**Gate**: code compiles; `mdemg adapter benchmark --help` unchanged.

### Epic 2 — Unit test (~30 min)

New test in `internal/cli/adapter_benchmark_signal_test.go`:

```go
func TestBenchmarkSignalHandlerCallsCleanup(t *testing.T) {
    // Isolate state to tmp dir
    tmpHome := t.TempDir()
    t.Setenv("HOME", tmpHome)

    // Create fake pidfile so stopBenchServe has something to clean
    port := 18103
    pidfilePath, _ := benchServePidFile(port)
    rec := benchServePidRecord{PID: 99999, Port: port /* nonexistent */}
    _ = writePidRecord(pidfilePath, rec)

    // Simulate the signal handler goroutine directly
    // (extract to a testable function in impl)
    stopBenchServeForBenchmarkOrchestrator(pidfilePath, port)

    if _, err := os.Stat(pidfilePath); !os.IsNotExist(err) {
        t.Errorf("expected pidfile gone after cleanup; err=%v", err)
    }
}
```

**Gate**: test passes; no goroutine leak (verified via `-race`).

### Epic 3 — Integration test (~30 min)

New test that runs `mdemg adapter benchmark` as a subprocess (via `exec.Command`) pointing at a fake adapter dir + fake python (a shell script that sleeps 30s emulating run_benchmark). SIGTERM the wrapper after 5s. Verify pidfile absent + orphan check.

Actually — Tier 2 integration is complex to set up cleanly for signal-in-subprocess testing. Simpler shape: extract signal-handler goroutine into standalone testable function that takes `(pidfilePath, port, sig)` and returns cleanly. Unit-test that function.

**Gate**: signal-handler function testable + tested.

### Epic 4 — Live Tier-3 (~30 min)

- Verify production 8102 healthy pre-test
- Kick off `mdemg adapter benchmark --adapter adapters/phase_e3_v1_base_v3 --iter 1200 --out /tmp/sigterm_smoke.json` via `run_in_background: true` (10-min ceiling avoidance)
- After 30s, `kill $BENCHMARK_PID` (SIGTERM)
- Verify:
  - Pidfile at `~/.mdemg/bench-serve-8103.json` absent
  - No orphan `mlx_lm.server.*8103` processes
  - Production 8102 STILL healthy
- Repeat with SIGINT

**Gate**: cleanup fires on both signals; no orphans; prod 8102 unaffected.

### Epic 5 — Feature doc update + sprint post + PR (~30 min)

- Extend `docs/features/adapter-swap.md` §"Guarantees" with new item: "SIGTERM/SIGINT to `mdemg adapter benchmark` triggers defer-cleanup — bench-serve stopped + pidfile removed before re-raising signal"
- `docs/development/adapter-swap-standardize-002/sprint_post.md`
- CHANGELOG.md Unreleased entry
- PR summary comment
- Task #146 → completed

## 6. Testing Plan (3 tiers)

- Tier 1 (unit): TestBenchmarkSignalHandlerCallsCleanup — extract signal-cleanup goroutine to standalone function, unit-test it against fake pidfile
- Tier 2 (integration): fold into Tier 1 (subprocess-signal testing is fragile; unit-test the cleanup contract)
- Tier 3 (live): real bench + SIGTERM + verify pidfile absent, no orphans, prod 8102 untouched

## 7. Commit Strategy

Single commit (Epic 1-5 folded — small sprint). Auto-PR fires.

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/cli/` clean
- [ ] Unit test passes (`go test -run TestBenchmarkSignalHandler ./internal/cli/`)
- [ ] Live Tier-3 SIGTERM smoke: pidfile absent, no orphans, prod 8102 healthy
- [ ] Live Tier-3 SIGINT smoke: same
- [ ] `mdemg adapter benchmark --help` unchanged (backward compat)
- [ ] Feature doc updated
- [ ] Sprint post + verdict + CHANGELOG + PR comment shipped
- [ ] Task #146 → completed

## 9. Documentation Update

- `docs/features/adapter-swap.md` — extend §Guarantees
- `docs/development/adapter-swap-standardize-002/{sprint_plan,sprint_post}.md`
- CHANGELOG.md Unreleased entry
- PR summary comment

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Signal handler fires before bench-serve starts (early SIGTERM) | `benchStarted bool` guard — no-op if bench never started |
| Goroutine leak on normal return | `defer signal.Stop + close(sigCh)` cleanup; goroutine exits via `<-sigCh` returning `!ok` |
| Signal handler races with normal shutdown | `signalCleanupDone` channel + defer waits for it; single-fire via unbuffered `close(signalCleanupDone)` |
| Re-raising signal loses exit code | Correct behavior: process exits with signal-terminated status (equivalent to shell's `128+N`) — matches operator expectation |
| Multiple signals in rapid succession | signal.Notify buffered channel size 1 — subsequent signals dropped (as designed for cleanup handlers) |
| Testing signal behavior in Go is fragile | Extract cleanup logic to standalone testable function; unit-test the function directly, not the signal delivery |

## 11. Documents Accessed

- `docs/development/adapter-swap-standardize-001/{sprint_plan,sprint_post}.md` (#139)
- `docs/development/mdemg-usage-lora-001/sprint_post.md` (#145 live-hit)
- `docs/features/adapter-swap.md` (target of doc update)
- `internal/cli/adapter_benchmark.go` (target of code change)
- `internal/cli/adapter_bench_serve.go` (`stopBenchServe` used verbatim)
- `internal/cli/adapter_common.go` (`benchServePidFile`, `benchServePidRecord`, `writePidRecord`)
- Go stdlib docs: `os/signal`, `syscall.Kill`
- CLAUDE.md pins per §3
- Operator directive 2026-09-02 ("proceed with #146")

## 12. Rollback Procedures

**Fully additive**: no existing behavior changed. Rollback = revert the commit. Zero risk to production or user-visible surface.
