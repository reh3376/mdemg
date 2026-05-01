# Sprint FT-LORA-PHASE11.6.3 — MLX Watchdog (Operational Hygiene #2)

## Context

Phase 12 (UVTS Activation, commit `98fc7a8`, PR #364) closed with mid-sprint operational findings that block the next research sprint (Phase 13 Column-Voting Retrieval) from running sustained live tests:

1. **MLX server fragility under sustained load.** `mlx_lm.server` crashes every 30–60 minutes with `kIOGPUCommandBufferCallbackErrorOutOfMemory` — a Metal command-buffer OOM (NOT raw RAM exhaustion; system has 128 GB unified memory). The crash is `std::runtime_error` thrown from `mlx::core::gpu::check_error` propagating through `com.Metal.CompletionQueueDispatch` with no catch handler → `abort()`. Aggressive flags (`--prompt-cache-size 4096 --prompt-concurrency 4`) accelerate the failure; conservative flags (`--prompt-cache-size 256 --prompt-concurrency 2`) extend the runway but don't eliminate it. Confirmed via macOS crash report PID 47175, Phase 12 Epic 7.

2. **Retry-storm cascade when mlx is unreachable.** When mlx dies, mdemg's 16 LLM call sites each independently retry 6× with exponential backoff (~30 s per call). RSIC orchestration auto-fires micro cycles; each cycle invokes the LLM-bound reflector. Phase 11.6.x's RSIC semaphore caps `ape.reflect` at 2 concurrent but the other 14 call sites are uncapped. Observed: **1642% CPU, load avg 30+** before manual `kill` of mdemg. The load-avg curve recovered ~10 minutes after mdemg termination, confirming the storm was the dominant load source.

The pattern recurred 4–5 times across this session. Each recurrence required manual operator intervention: notice via Grafana / log inspection, kill mdemg to stop the storm, restart mlx with the right flags, restart mdemg with the right `LLM_ENDPOINT` override.

This sprint **automates that operator loop** so transient mlx death is survivable: detected within seconds, mdemg degrades gracefully (no retry storm), mlx auto-restarts on a controlled cadence, operator is alerted but does not have to act. Underlying GPU driver behavior is **out of scope** — this sprint is operational hygiene, not Metal-driver work.

**Why this sprint NOW (vs continuing to Phase 13):** Per the post-Phase-12 commit's verdict, Phase 13 (Column-Voting Retrieval) needs sustained live A/B testing as its first UVTS consumer. With current mlx fragility, every A/B run risks a 30-minute storm + manual recovery. **The watchdog is the precondition for any sustained live work.** This is the same shape of "live testing surfaces what paper review missed" pattern that has driven the last three sprints.

**Scope option recommended (Option A — launchd-only restart + mdemg-side probe + llmclient fast-fail):** Option B (build a separate Go `cmd/mlx-watchdog/` binary that exec.Cmd's mlx) is more code than the problem warrants. Option C (shell wrapper script doing its own restart loop, supervised by launchd) duplicates what launchd's `KeepAlive` + `ThrottleInterval` already do. Option A reuses the existing `com.mdemg.server.plist` pattern (LaunchAgent + `KeepAlive.SuccessfulExit=false` + `ThrottleInterval`) with a new mlx plist; restart-on-death is the launchd job. The mdemg side adds a goroutine probe (extending `internal/healthprobe/` patterns) plus a single-line gate in `llmclient.doWithRetry` to fast-fail when probe says "down". This is the minimum increment that closes the loop.

**Phase dependency chain:** Phase 12 (commit `98fc7a8`, this branch's HEAD) → **Phase 11.6.3 (this) — MLX Watchdog** → Phase 13 (Column-Voting Retrieval, the first sustained live A/B sprint).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-PHASE11.6.3 |
| Title | MLX Watchdog — auto-restart + fast-fail + degraded-mode |
| Date | 2026-04-30 (plan) |
| Branch | `reh3376_dev01` |
| Predecessor | Phase 12 (commit `98fc7a8`, PR #364) |
| Successor | Phase 13 — Note 04 Column-Voting Retrieval (gated on this sprint) |
| Type | Code-medium (~500–700 LOC: probe goroutine + llmclient gate + 1 CLI subcommand + plist + tests); infra-light (no TSDB migration; new metrics + alerts only); compute-light |
| Risk | MEDIUM (architecture fork: launchd-only vs Go-binary supervisor; recommend launchd-only) |
| Budget | $0 OpenAI; ~3 hr local compute (live smoke kills + restarts mlx) |
| Effort estimate | 5–7 dev-days |
| New TSDB migration | None (additive metrics + alerts only; no schema change; schema_version stays 16) |
| New launchd plist | `packaging/launchd/com.mdemg.mlx-server.plist` (LaunchAgent, KeepAlive on crash, ThrottleInterval=60s) |
| Post-sprint artifacts | `internal/mlxprobe/probe.go` (new package); `internal/llmclient/client.go` (~10-line gate at line 471 doWithRetry); `packaging/launchd/com.mdemg.mlx-server.plist`; `internal/cli/service_darwin.go` (extend `launchdServices` slice); `internal/cli/watchdog.go` (new `mdemg watchdog status` CLI); `internal/metrics/collectors.go` (3 new metrics); 4 new config knobs in `internal/config/config.go`; sprint docs |

## 2. Problem Statement

Make transient mlx_lm.server death survivable so it does not require operator intervention or trigger a CPU storm. Specifically:

1. **Detect mlx-down within ~10 s.** New `internal/mlxprobe` goroutine polls `<LLM_ENDPOINT>/v1/models` every 5 s with a 2 s timeout. State machine: `up` → `degraded` (3 consecutive slow responses) → `down` (3 consecutive failures, OR connection-refused). Down→up transition requires 2 consecutive successes (single-success would flap on a flaky server).

2. **Fast-fail in llmclient when probe says down.** Single check at the top of `Client.doWithRetry` (`internal/llmclient/client.go:471`): if the per-endpoint health state is `down`, return immediately with a synthetic error rather than entering the 6-attempt × ~30 s retry loop. This kills the retry-storm pattern at its source. State is package-level singleton, atomic-read; the probe writes, all clients read. **Embeddings paths are unaffected** — the gate keys on `baseURL` and only mlx-targeted clients (i.e. `EffectiveLLMEndpoint()`) participate.

3. **Auto-restart mlx via launchd.** New `packaging/launchd/com.mdemg.mlx-server.plist` with `KeepAlive.SuccessfulExit=false` + `ThrottleInterval=60` mirrors the existing `com.mdemg.server.plist` pattern. mlx invocation hardcodes the conservative config validated in Phase 12 (`--prompt-cache-size 256 --prompt-concurrency 2 --decode-concurrency 2`). Plist installed via existing `mdemg service install` flow (extend `launchdServices` slice in `internal/cli/service_darwin.go`). When mlx dies, launchd restarts it after 60 s; mdemg's probe sees the down→up transition and clears the fast-fail gate automatically.

4. **Alert operator on state transitions.** Reuse `internal/alert/dispatcher.go`. On `up→down`: severity High, service `mlx-server`, title `mlx unreachable`, message includes endpoint + last error. On `down→up`: severity Low (cleared). Existing cooldown dedup prevents spam during launchd's restart cycles.

5. **Operator visibility.** New CLI `mdemg watchdog status` shows current probe state + restart count (parsed from launchd via `launchctl print`) + last 5 alert events (parsed from existing `~/.mdemg/alerts/current.json`). New Prometheus metrics: `mdemg_mlx_health_state{endpoint=...}` (gauge: 0=up, 1=degraded, 2=down), `mdemg_mlx_fast_fail_total{caller_task=...}` (counter, incremented when `doWithRetry` short-circuits), `mdemg_mlx_state_transitions_total{from,to}` (counter, useful for restart-frequency monitoring).

6. **Stay out of the GPU driver.** This sprint does NOT change mlx invocation flags beyond what was validated in Phase 12, does NOT modify mlx_lm Python code, does NOT introduce a new model serving stack. The Metal-OOM behavior is **upstream**; this sprint makes its consequences survivable, nothing more.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | mlx health probe goroutine + state machine + per-endpoint state cache | `internal/mlxprobe/probe.go` (new package, ~250 LOC) |
| 2 | llmclient fast-fail gate at `doWithRetry` entry | `internal/llmclient/client.go` (~10 LOC at line 471 + 1 import) |
| 3 | New CLI: `mdemg watchdog status` (probe state + restart count + recent alerts) | `internal/cli/watchdog.go` (new, ~80 LOC) |
| 4 | launchd plist for mlx_lm.server with conservative flags | `packaging/launchd/com.mdemg.mlx-server.plist` (new) |
| 5 | Extend `launchdServices` slice so `mdemg service install/uninstall/status/restart` covers mlx | `internal/cli/service_darwin.go` (~15 LOC) |
| 6 | New Prometheus metrics (3) | `internal/metrics/collectors.go` (~30 LOC) |
| 7 | Alert wiring on probe state transitions | `internal/cli/serve.go` (~20 LOC, callback wiring) |
| 8 | Config knobs: `MLX_WATCHDOG_ENABLED`, `MLX_PROBE_INTERVAL_SEC`, `MLX_PROBE_TIMEOUT_SEC`, `MLX_FAIL_FAST_ENABLED` | `internal/config/config.go` (~30 LOC, mirror existing knob pattern) |
| 9 | Unit + integration + LIVE e2e tests | `internal/mlxprobe/probe_test.go`; `internal/llmclient/client_fail_fast_test.go`; live smoke that kills + restarts mlx and observes recovery |
| 10 | Sprint docs | `docs/development/ft-lora/sprint_plan_phase_11_6_3.md` (this plan, copied + frozen); `phase_11_6_3_post.md` (executed-truth post) |
| 11 | Doc updates | `AGENT_HANDOFF.md` top entry; `CHANGELOG.md [Unreleased] ### Added`; `CLAUDE.md` — new "MLX Watchdog" subsection under Architecture Notes; `SPRINT_ROADMAP_POST_FT_LORA.md` — note Phase 13 unblocked |

**Out of scope (deferred):**
- A separate `cmd/mlx-watchdog/` Go binary. The launchd-only restart pattern is sufficient; a dedicated supervisor binary would duplicate `KeepAlive`+`ThrottleInterval`.
- mlx flag tuning beyond Phase 12's conservative profile (`--prompt-cache-size 256 --prompt-concurrency 2 --decode-concurrency 2`). The Metal-OOM root cause is upstream; this sprint accepts that and routes around it.
- Metric-driven self-tuning (e.g., progressively shrinking `--prompt-cache-size` after repeated crashes). Premature; revisit only if launchd restart frequency exceeds 5/hour in steady state.
- Linux/Windows watchdog. macOS-launchd-only this sprint; Linux systemd unit can mirror the pattern in a follow-up if needed.
- Distributed health (multiple mlx instances behind a load balancer). Single-instance only — matches the existing `MLX single-instance` MEMORY constraint.
- Embedding-endpoint probing. OpenAI embeddings (`cfg.OpenAIEndpoint`) have their own retry+breaker; this sprint does not touch them.

**Constraints (hard):**
- **MEMORY: no hardcoded values** — probe interval, timeout, fail-fast threshold all in config; mlx flags expressed in plist (per-installation override via `mdemg service install --mlx-cache-size N` if we expose it; otherwise edit-in-place).
- **MEMORY: sequential epics** — no parallel epic execution; docs before implementation within each epic.
- **MEMORY: 3-tier testing — Tier 3 MUST be live** (formalized in CLAUDE.md commit `d10c1a5`). Live smoke kills mlx with `kill <pid>`, observes probe transitions to `down`, observes llmclient fast-fail by reading `mdemg_mlx_fast_fail_total` increment, observes launchd restart, observes probe transition back to `up`, observes alerts dispatched. No mocking the probe.
- **MEMORY: plan-options pattern** — architecture fork + probe-interval fork + fast-fail-on-degraded fork are the three decision points; recommendations + rationale here; disclose at PR.
- **MEMORY: single batched commit at sprint close**.
- **MEMORY: sprint summary posted to PR comments immediately after push**.
- **Sprint plans live in `docs/development/<sprint-line>/`** — `ft-lora/` (this is operational hygiene continuing the 11.6.x line).
- **No new TSDB migration** — metrics + alerts are in-memory + file-backed respectively; no schema change. Schema version stays 16.
- **Embeddings traffic untouched** — fast-fail keys on `baseURL` and only mlx endpoints (matching `cfg.EffectiveLLMEndpoint()`) participate. OpenAI embeddings calls (which legitimately use `cfg.OpenAIEndpoint`) are unaffected.
- **No goroutine leaks** — probe goroutine integrates with `internal/supervisor/` (already wired in `cmd/mdemg/serve.go`) for panic recovery + ctx cancellation on shutdown.
- **Backward compat** — `MLX_WATCHDOG_ENABLED=false` default until live-validated, then flipped to true. Operator can disable post-rollout if it misbehaves.

## 4. Dependencies

**Consumed (code, pre-existing — reuse, do not duplicate):**
- `internal/healthprobe/prober.go` — pattern for ticker-driven HTTP probing with timeout + state, panic-recovered via supervisor. Mirror its lifecycle (Start/Stop with ctx cancellation).
- `internal/supervisor/supervisor.go` — exponential-backoff goroutine restart (5s base, 2× multiplier, 3-restart cap) with panic recovery. The mlxprobe goroutine registers here so a panic restarts it cleanly without leaking.
- `internal/circuitbreaker/breaker.go` — full state machine + `/v1/admin/breakers` admin endpoints (operator escape hatch). The probe is conceptually a per-endpoint breaker; we reuse the breaker package's atomic-state pattern (state stored as `atomic.Int32`) rather than rolling our own. Decision: probe runs as its own goroutine, but the per-endpoint state cache uses the breaker's existing storage idiom.
- `internal/alert/dispatcher.go` — `Send(ctx, severity, service, title, message)` API with cooldown dedup + atomic `TryRecord()` (DH-004 fix). Reuse for state-transition alerts. Existing `cooldown.Allow`/`TryRecord` prevents alert spam during launchd's restart cycles.
- `internal/llmclient/client.go:471` — `doWithRetry` is the single retry-loop entry point for all LLM call sites. Inject a 10-LOC gate at the top. **Do not modify the retry math** — only short-circuit when probe says down. The `baseURL` field at line 83 is the natural key for the per-endpoint state.
- `internal/llmclient/client.go:401-440` — `shouldRetry` currently treats connection-refused as retryable. Watchdog gate runs **before** this, so connection-refused-fast-fail short-circuits without entering the retry math; `shouldRetry` itself is unchanged.
- `internal/cli/service_darwin.go` — `launchdServices` slice is the registry of plist templates rendered by `mdemg service install`. Add 1 entry: `com.mdemg.mlx-server`.
- `packaging/launchd/com.mdemg.server.plist` — template substitution pattern (`__MDEMG_BIN__`, `__PROJECT_DIR__`, `__HOME__`); KeepAlive + ThrottleInterval idiom. Mirror for new mlx plist.
- `internal/metrics/collectors.go` — Prometheus collector registration pattern (3 new gauges/counters mirror DH-005 dimension confidence gauges).
- `internal/cli/serve.go` — early-writer block pattern: anything that writes Prometheus state or registers callbacks must initialize before `api.NewServer()`. Probe + alert wiring goes in this block.
- `internal/config/config.go` — `FromEnv()` pattern for new knobs; `Validate()` cross-field check (require `MLX_PROBE_INTERVAL_SEC > MLX_PROBE_TIMEOUT_SEC` to avoid overlap).

**Consumed (data):**
- `~/.mdemg/alerts/current.json` — alert dispatcher's file backend; `mdemg watchdog status` CLI reads this to surface recent state-transition events.
- launchd state via `launchctl print gui/$(id -u)/com.mdemg.mlx-server` — restart count, last exit code, current PID. CLI parses with `os/exec.Command`.
- Local mlx server on `127.0.0.1:8101` — probe target. `cfg.EffectiveLLMEndpoint()` resolves the same value llmclient uses; probe MUST read from this resolver, not a separate config key, to guarantee they target the same endpoint.

**Consumed (compute):**
- Local mlx_lm.server (Apple Silicon, M5 Max). Probe is HEAD/GET on `/v1/models` — single-digit-ms cost, negligible per-second load even at 5s interval.
- macOS launchd. Already in active use for `com.mdemg.server`, `com.mdemg.health`, `com.mdemg.scheduler`, `com.mdemg.maintenance`, `com.mdemg.dashboard`. Adding a 6th service is mechanical.

**External services:**
- mdemg HTTP API (`localhost:9999`) — `mdemg watchdog status` CLI may query it for live probe state if the running mdemg has the watchdog goroutine active; falls back to "watchdog not running" if not.
- launchctl (system binary) — restart count + state extraction.
- No TSDB writes from this sprint (no schema change). No Neo4j writes.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` clean; native binary running on host with `LLM_ENDPOINT=http://127.0.0.1:8101/v1`; mlx alive (conservative config from Phase 12); TSDB schema_version=16; mdemg health green; Neo4j up.

### Epic 0 — Preflight + File Inventory + Plist Drafting

1. Verify `cfg.EffectiveLLMEndpoint()` resolves to `http://127.0.0.1:8101/v1` on this host (re-read after Phase 11.6.2 cutover; sweep grep for any `cfg.LLMEndpoint` direct reads remaining).
2. Inventory `internal/healthprobe/`, `internal/supervisor/`, `internal/circuitbreaker/`, `internal/alert/` to confirm the reuse path before writing new code (avoid duplicating logic).
3. Read `packaging/launchd/com.mdemg.server.plist` end-to-end; note template variables. Confirm `mdemg service install` flow renders these via `embed.FS` and substitutes correctly.
4. Read `internal/llmclient/client.go` lines 1-100 + 401-510 to confirm doWithRetry shape + baseURL field — exact line for gate insertion is at the entry of `doWithRetry`, before any retry-counter init.
5. Confirm `mlx_lm.server` invocation flags from Phase 12: `mlx_lm.server --model /Users/reh3376/mdemg/.local-models/mdemg-llm-v1 --host 127.0.0.1 --port 8101 --prompt-cache-size 256 --prompt-concurrency 2 --decode-concurrency 2`.
6. Draft `com.mdemg.mlx-server.plist` template with `KeepAlive.SuccessfulExit=false` + `ThrottleInterval=60` + the conservative flags; do NOT install yet.

**Gate:** `cfg.EffectiveLLMEndpoint()` resolves correctly; reuse-vs-duplicate decision documented for each of the 4 candidate packages; line numbers for gate insertion verified; plist template lints (`plutil -lint`).

### Epic 1 — `internal/mlxprobe` Package + State Machine

1. New package `internal/mlxprobe/`:
   - `probe.go` — `Prober` struct (endpoint string, interval, timeout, http.Client, state atomic.Int32, lastError atomic.Value, transition callback chan).
   - State enum: `StateUp=0`, `StateDegraded=1`, `StateDown=2`. Atomic int storage (mirror circuitbreaker idiom).
   - `New(cfg) *Prober`, `Start(ctx) error`, `Stop()`, `State() State`, `LastError() error`, `OnTransition(fn func(from, to State))`.
   - Polling loop: `time.NewTicker(interval)`, GET `/v1/models` with timeout; success increments success-counter, failure increments failure-counter; transitions on threshold (configurable: default 3 consecutive failures → down; 2 consecutive successes → up).
2. Package-level singleton wiring: `mlxprobe.SetDefault(p *Prober)` + `mlxprobe.Default() *Prober` so llmclient can read state without a constructor parameter (mirror `llmclient.SetDefaultRecorder` pattern from CLAUDE.md "LLM recorder init order" rule).
3. Supervisor integration: register the probe goroutine via `supervisor.Run("mlx-probe", probeFunc)` in `internal/cli/serve.go` early-writer block. Panic recovery + ctx cancellation handled by supervisor.
4. State transition callback: on `up→down` and `down→up`, fire `alert.Send(...)`. On `up→degraded`, log only (do not alert; degraded is informational).
5. Embeddings safety: probe operates on `cfg.EffectiveLLMEndpoint()` only. Prober struct knows nothing about OpenAI embeddings.

**Gate:** `go test ./internal/mlxprobe/... -v -race` passes (Tier 1 unit covers state-machine transitions, threshold logic, atomic-read consistency); package builds; supervisor registers probe without leaking goroutines (verify via `goleak` in test).

### Epic 2 — llmclient Fast-Fail Gate

1. Edit `internal/llmclient/client.go`:
   - Add import: `"mdemg/internal/mlxprobe"`.
   - At top of `Client.doWithRetry` (line 471), before retry-counter init: if `c.baseURL` matches `cfg.EffectiveLLMEndpoint()` (via package helper) AND `mlxprobe.Default()` is non-nil AND `mlxprobe.Default().State() == mlxprobe.StateDown` AND `cfg.MLXFailFastEnabled` is true → increment `mdemg_mlx_fast_fail_total{caller_task=<task-from-ctx>}` counter, return `ErrMLXDown` (new sentinel error in `llmclient/errors.go`).
   - Total LOC: ~10 lines + 1 import + 1 sentinel error declaration.
2. Add `errors.Is(err, ErrMLXDown)` short-circuit in callers that currently log retry exhaustion; they already log a generic "all retries exhausted" — no new behavior needed; the sentinel just lets observability differentiate "fast-failed because down" from "exhausted retries".
3. Embeddings non-impact verified: search for all `llmclient.NewClient` constructions; confirm OpenAI-pointed clients do NOT have `baseURL == EffectiveLLMEndpoint()` (they use `cfg.OpenAIEndpoint`). The gate's baseURL match key isolates them.
4. Caller-task label extraction: probe-aware ctx key (mirror `WithSpaceID`/`WithSessionID` pattern from CLAUDE.md). The 16 LLM call sites already set this for TSDB recording; reuse.

**Gate:** `go test ./internal/llmclient/... -v -race` passes including new `client_fail_fast_test.go` (mocked prober set to StateDown → doWithRetry returns ErrMLXDown immediately, never enters retry math; mocked prober StateUp → normal retry path unchanged); `golangci-lint run ./internal/llmclient/...` clean; manual grep confirms no other call sites in the file regress.

### Epic 3 — launchd Plist + service install Extension

1. Add `packaging/launchd/com.mdemg.mlx-server.plist` to `embed.FS`:
   - `Label`: `com.mdemg.mlx-server`.
   - `ProgramArguments`: invoke `mlx_lm.server` with conservative flags. Path to mlx_lm Python via venv: parameterize `__MLX_LM_PATH__` template variable to `/Users/reh3376/<venv>/bin/mlx_lm.server` (resolve at install time).
   - `KeepAlive.SuccessfulExit`: `false` (restart only on crash).
   - `ThrottleInterval`: `60` (60s minimum between restart attempts).
   - `StandardOutPath`/`StandardErrorPath`: `~/.mdemg/logs/mlx-server.{out,err}.log`.
   - `EnvironmentVariables`: empty (mlx reads no env).
2. Extend `internal/cli/service_darwin.go` `launchdServices` slice:
   - Add entry: `{Name: "com.mdemg.mlx-server", Template: "com.mdemg.mlx-server.plist", Required: false}`.
   - `Required: false` means `mdemg service install` skips it unless `--with-mlx` flag is passed. This guards against installing on a host where mlx_lm isn't on PATH or venv isn't at the expected location. Phase 13 pre-flight makes it required.
3. Add `--with-mlx` flag to `mdemg service install`. When present, plist substitutes `__MLX_LM_PATH__` from `which mlx_lm.server` output (or `--mlx-lm-path` override).
4. Sweep `mdemg service uninstall/status/restart` to confirm they iterate `launchdServices` correctly — adding an entry should automatically extend coverage. No code change needed here if the slice is fully driven.
5. Document install workflow: `mdemg service install --with-mlx --mlx-lm-path /Users/reh3376/.venv/mdemg-ft-lora/bin/mlx_lm.server`.

**Gate:** `plutil -lint packaging/launchd/com.mdemg.mlx-server.plist` clean; `mdemg service install --with-mlx --dry-run` renders correctly; `mdemg service status` shows mlx-server alongside the other 5 services.

### Epic 4 — Metrics + Alert Wiring

1. `internal/metrics/collectors.go` — register 3 new metrics:
   - `mdemg_mlx_health_state{endpoint}` — gauge (0=up, 1=degraded, 2=down).
   - `mdemg_mlx_fast_fail_total{caller_task}` — counter.
   - `mdemg_mlx_state_transitions_total{from,to}` — counter.
2. Probe writes `mdemg_mlx_health_state` on every state read; transitions write `mdemg_mlx_state_transitions_total`.
3. Alert wiring in `internal/cli/serve.go`:
   - Construct `mlxprobe.Prober`; call `prober.OnTransition(...)` with a closure that calls `alert.Send(ctx, severity, "mlx-server", title, message)`.
   - On `up→down`: severity High, title "mlx unreachable", message includes endpoint + last error string + UTC timestamp.
   - On `down→up`: severity Low, title "mlx recovered", message includes restart count from launchd.
   - On `up→degraded` and `degraded→up`: log-only (zerolog Info), no alert.
4. Cooldown reuse: alert dispatcher's existing 300s cooldown (per-service-key) handles spam during launchd's 60s restart cycle. Verify by simulating 5 down/up flips in 5 minutes — alert dispatcher should record 1 down + 1 up + 4 suppressed.

**Gate:** Prometheus endpoint `/metrics` shows the 3 new metrics; alert e2e with simulated transitions writes to `~/.mdemg/alerts/current.json` and respects cooldown.

### Epic 5 — `mdemg watchdog status` CLI

1. New `internal/cli/watchdog.go`:
   - `watchdogCmd` parent with subcommands.
   - `mdemg watchdog status`:
     1. Query running mdemg's `/metrics` endpoint for current `mdemg_mlx_health_state` value (fallback: query the local probe directly via a debug endpoint if mdemg unreachable).
     2. Parse `launchctl print gui/$(id -u)/com.mdemg.mlx-server` for restart count + last exit code + PID.
     3. Read `~/.mdemg/alerts/current.json`; filter `service=="mlx-server"`; show last 5 events.
     4. Output: human-readable table (state, restart count, last exit, PID, last 5 alerts). `--json` flag for machine-readable.
2. Optional `mdemg watchdog test --kill` for live e2e — just runs `kill <pid-of-mlx>` after confirming with operator (NEVER auto, MEMORY: irreversible → ask user).

**Gate:** `mdemg watchdog status` runs cleanly when mlx is up; runs cleanly when mlx is killed (shows down + last error); `--json` shape parses with `jq`.

### Epic 6 — Config Knobs + Wiring

1. `internal/config/config.go`:
   - `MLXWatchdogEnabled bool` (default `false` — flipped to `true` after live validation in Epic 7).
   - `MLXProbeIntervalSec int` (default 5).
   - `MLXProbeTimeoutSec int` (default 2).
   - `MLXFailFastEnabled bool` (default `true` — only effective when watchdog enabled).
   - `Validate()` cross-field: assert `MLXProbeIntervalSec > MLXProbeTimeoutSec`; warn if not.
2. `FromEnv()` reads all 4; document in `internal/config/config.go` field comments.
3. `mdemg init` interactive prompt: do NOT add yet — too noisy for installer flow. Operator opts in via `.env` after `service install`.

**Gate:** `go test ./internal/config/...` passes new validation cases; `mdemg start` starts cleanly with watchdog disabled (no behavior change); starts cleanly with watchdog enabled (probe goroutine registers + state read works).

### Epic 7 — Testing (3 Tiers — Mandatory Per CLAUDE.md `d10c1a5`)

Covered in §6 below.

### Epic 8 — Documentation (Final Epic — Never Cut)

1. `docs/development/ft-lora/sprint_plan_phase_11_6_3.md` — this plan, frozen at sprint start (copy from this plan file).
2. `docs/development/ft-lora/phase_11_6_3_post.md` — executed-truth post: live-smoke run-by-run results (kill mlx → probe transition → fast-fail count → restart → recovery), launchd restart count after 8h soak, alert event audit, decision-fork outcomes, sprint wall-clock.
3. `SPRINT_ROADMAP_POST_FT_LORA.md` — mark Phase 11.6.3 EXECUTED with commit SHA; flag Phase 13 (Note 04 Column-Voting Retrieval) unblocked for sustained live A/B testing.
4. `AGENT_HANDOFF.md` top entry: Phase 11.6.3 complete; MLX watchdog active; retry storms eliminated; Phase 13 unblocked.
5. `CHANGELOG.md [Unreleased] ### Added`: mlxprobe goroutine + state machine, llmclient fast-fail gate, launchd plist for mlx-server, `mdemg watchdog status` CLI, 3 Prometheus metrics, 4 config knobs.
6. `CLAUDE.md` — new "MLX Watchdog" subsection under Architecture Notes covering: how to enable (`MLX_WATCHDOG_ENABLED=true`), what happens on mlx death, how to read `mdemg watchdog status`, how to disable in emergency (config flip).

**Gate:** all docs committed; cross-refs valid; `grep -r "Phase 11.6.3.*pending\|Phase 11.6.3.*planned" docs/development/ft-lora/` returns zero hits.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit) — `go test -race`:**
- `internal/mlxprobe/probe_test.go` — state machine: 3 consecutive failures → down; 2 consecutive successes → up; degraded threshold; concurrent state-read consistency (atomic); `Stop()` cancels ticker cleanly; `goleak.VerifyNone(t)` confirms no leaked goroutines.
- `internal/llmclient/client_fail_fast_test.go` — mocked prober StateDown → `doWithRetry` returns `ErrMLXDown` without invoking transport; mocked prober StateUp → normal retry path; baseURL mismatch (OpenAI endpoint) → gate skipped, retry path entered; `MLXFailFastEnabled=false` → gate skipped even when down.
- `internal/config/config_test.go` — new validation: `MLXProbeIntervalSec=2, MLXProbeTimeoutSec=5` triggers warning; defaults pass `Validate()`.
- `internal/cli/watchdog_test.go` — mock `~/.mdemg/alerts/current.json` + mock `launchctl print` output → `mdemg watchdog status` parses correctly; `--json` shape matches schema.

**Tier 2 (Integration) — go test build tag `integration`:**
- `tests/integration/mlxprobe_integration_test.go` — start probe against a fake HTTP server (`httptest.NewServer`); kill the test server; assert state transitions to `down` within `3 × interval`; restart server; assert state recovers to `up` within `2 × interval`. Uses real time, real http.Client, no mocking inside the package.
- `tests/integration/llmclient_watchdog_integration_test.go` — start fake mlx server + probe pointing at it; make 100 concurrent llmclient calls; kill fake server; assert ALL 100 calls fast-fail within 1 probe interval (no retry storm); restart fake server; assert subsequent calls succeed.
- `tests/integration/launchd_install_test.go` — `mdemg service install --with-mlx --dry-run` renders plist; `plutil -lint` on rendered output; `mdemg service uninstall` (dry-run) lists mlx-server in the unload set.

**Tier 3 (Live E2E) — MANDATORY per CLAUDE.md `d10c1a5`. Real binary, real services, observed outputs:**
- **Live Smoke 1 — kill mlx mid-load:**
  1. Start `com.mdemg.mlx-server` via `mdemg service install --with-mlx && launchctl kickstart gui/$(id -u)/com.mdemg.mlx-server`.
  2. Set `MLX_WATCHDOG_ENABLED=true MLX_FAIL_FAST_ENABLED=true` and start mdemg.
  3. Trigger 5 concurrent `consulting.classify` calls (small load).
  4. `kill -9 $(launchctl print gui/$(id -u)/com.mdemg.mlx-server | grep pid | awk '{print $3}')`.
  5. **Observe within 15s:** `mdemg watchdog status` shows state=down; Prometheus `mdemg_mlx_health_state{endpoint="http://127.0.0.1:8101/v1"}=2`; `mdemg_mlx_fast_fail_total > 0`; `mdemg_mlx_state_transitions_total{from="up",to="down"}=1`.
  6. **Observe within 75s:** launchd restarts mlx (`launchctl print` shows new PID + restart count incremented); probe transitions to up; `mdemg_mlx_state_transitions_total{from="down",to="up"}=1`; alert in `~/.mdemg/alerts/current.json` with severity=high (down) + severity=low (recovered).
  7. **Observe in mdemg log:** no retry-storm pattern (no 16-call-site spam); each fast-fail logs once with `ErrMLXDown`.
- **Live Smoke 2 — sustained-load soak with periodic kill (8 hr unattended):**
  1. Loop: every 90 minutes, `kill -9 <mlx-pid>` (simulates Metal-OOM cadence).
  2. mdemg under steady RSIC traffic (synthetic observation feed at 1/sec).
  3. **Pass criteria after 8h:** mdemg never required manual restart; mdemg never exceeded 200% CPU; load avg never exceeded 5; `mdemg_mlx_state_transitions_total{from="down",to="up"}` ≈ launchd restart count; alert dispatcher shows ~5 down + ~5 up alerts (cooldown-suppressed flaps); `~/.mdemg/alerts/current.json` size bounded (no unbounded growth).
- **Live Smoke 3 — embeddings non-impact:**
  1. With mlx down (`launchctl bootout`), trigger an embedding call (e.g., `mdemg memory remember --space mdemg-dev --content "test"` which embeds via OpenAI).
  2. **Assert:** embedding succeeds (gate is mlx-endpoint-keyed, not global). `mdemg_mlx_fast_fail_total` does NOT increment for the embedding call.
- **Live Smoke 4 — operator escape hatch:**
  1. With watchdog active and mlx down, set `MLX_FAIL_FAST_ENABLED=false` via `.env` + restart mdemg.
  2. **Assert:** llmclient calls revert to retry-storm behavior (the operator chose this); CPU climbs as expected; reset by stopping mdemg.

**State restoration (MEMORY):** all changes additive. Rollback = revert commit + `mdemg service uninstall com.mdemg.mlx-server` (CLI extension auto-detects). No TSDB rollback (no migration). No Neo4j rollback. Operator can disable runtime via `MLX_WATCHDOG_ENABLED=false` + restart.

**Gate:** all 3 tiers green. Tier 3 results recorded in `phase_11_6_3_post.md` with timestamps + Prometheus snapshots + `~/.mdemg/alerts/current.json` excerpts. NO sprint close until Live Smoke 2 has run for ≥8 hours.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(mlx-watchdog): Sprint FT-LORA-PHASE11.6.3 — auto-restart + fast-fail + degraded-mode`
- Body: scope summary, soak-test outcome (Live Smoke 2 pass), retry-storm elimination evidence (CPU + load-avg curves before/after), decision-fork outcomes (architecture: launchd-only chosen; probe-interval: 5s/2s chosen; fast-fail-on-degraded: degraded does NOT trigger fast-fail chosen — degraded is informational only), policy compliance checklist (no hardcoded values, sequential epics, 3-tier testing including live, single batched commit, sprint summary on PR).
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push to `reh3376_dev01` → auto-PR opens → **sprint summary comment posted to PR per MEMORY rule (not gated on CI)**.

## 8. Verification Checklist

- [ ] Epic 0: `cfg.EffectiveLLMEndpoint()` resolves correctly; reuse decision documented for healthprobe/supervisor/circuitbreaker/alert; line 471 doWithRetry insertion point verified; plist template lints
- [ ] Epic 1: `internal/mlxprobe/` package builds + Tier 1 + `goleak` clean; supervisor integration verified
- [ ] Epic 2: llmclient gate compiles + Tier 1 verifies fast-fail path; embeddings non-impact verified by grep
- [ ] Epic 3: `com.mdemg.mlx-server.plist` lints; `mdemg service install --with-mlx --dry-run` renders; service status covers mlx
- [ ] Epic 4: 3 Prometheus metrics registered; alert e2e fires on transitions; cooldown verified
- [ ] Epic 5: `mdemg watchdog status` works in up + down + watchdog-disabled scenarios; `--json` parses
- [ ] Epic 6: 4 config knobs read from env + `Validate()` cross-field check works; default `MLX_WATCHDOG_ENABLED=false` confirmed
- [ ] Epic 7 (Tier 1): `go test -race ./internal/mlxprobe/... ./internal/llmclient/... ./internal/config/... ./internal/cli/...` green
- [ ] Epic 7 (Tier 2): `go test -tags=integration ./tests/integration/...` for the 3 new files green
- [ ] Epic 7 (Tier 3 — MANDATORY): Live Smoke 1 + 2 + 3 + 4 results captured in `phase_11_6_3_post.md`; Live Smoke 2 has ≥8h soak data; no manual mdemg restart needed during soak
- [ ] Epic 8: sprint plan + post report + ROADMAP "Phase 11.6.3 EXECUTED" + AGENT_HANDOFF + CHANGELOG + CLAUDE.md (new MLX Watchdog subsection)
- [ ] `MLX_WATCHDOG_ENABLED` flipped to `true` default in config.go AFTER Live Smoke 2 passes
- [ ] Commit pushed; auto-PR updated; **sprint summary posted to PR immediately**
- [ ] No OpenAI spend (sprint is local-compute only)
- [ ] All 3 decision-fork choices disclosed in commit body + PR comment
- [ ] `golangci-lint run ./...` clean
- [ ] CI green on the auto-PR

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 8: `docs/development/ft-lora/sprint_plan_phase_11_6_3.md` (this plan, frozen), `docs/development/ft-lora/phase_11_6_3_post.md`, `SPRINT_ROADMAP_POST_FT_LORA.md` Phase 11.6.3 → EXECUTED with commit SHA, `AGENT_HANDOFF.md` prepended, `CHANGELOG.md [Unreleased] ### Added`, `CLAUDE.md` adds new "MLX Watchdog" subsection under Architecture Notes covering enable/disable + status CLI + behavior summary.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | **Decision fork: architecture** — launchd-only vs Go binary supervisor vs shell wrapper | Medium | Recommend launchd-only (Option A): reuses existing plist pattern (5 plists already in use), KeepAlive+ThrottleInterval native; minimal new code | Option B: build `cmd/mlx-watchdog/` (more LOC, duplicates launchd's role); Option C: shell wrapper (also duplicates launchd) — both ruled out in Context |
| 2 | **Decision fork: probe interval/timeout** — 5s/2s vs 10s/3s vs 1s/500ms | Medium | Recommend 5s/2s: detects within ~15s, adds <100ms/min overhead, matches healthprobe defaults | Option B: 10s/3s — slower detection, lower load (good if probe load matters); Option C: 1s/500ms — too aggressive, risks flaps + load |
| 3 | **Decision fork: fast-fail on `degraded` vs only on `down`** | Medium | Recommend `down` only (degraded is informational, log-only): degraded means slow-but-responding; fast-failing on slow risks turning a transient hiccup into a hard outage | Option B: fast-fail on degraded — over-aggressive, breaks recovery path |
| 4 | **launchd ThrottleInterval too low — restart storm** if mlx crashes back-to-back | Low | 60s ThrottleInterval is the floor; macOS enforces this. Operator visibility via `mdemg watchdog status` shows restart count if >5/hr → manual intervention | Increase ThrottleInterval to 300s if restart-frequency monitoring shows churn |
| 5 | **Probe goroutine leaks** if shutdown is botched | Low | Supervisor-managed lifecycle (`internal/supervisor/`); `goleak.VerifyNone(t)` in unit tests | Manual ctx.cancel() audit in `cmd/mdemg/serve.go`; smoke test with `pprof` goroutine snapshot before + after shutdown |
| 6 | **Fast-fail incorrectly triggers on slow-but-up mlx** | Low | Probe must observe 3 consecutive failures (not "slow"); slow-only path goes to `degraded`, not `down`. `MLX_FAIL_FAST_ENABLED=false` is the operator escape hatch | Add a 4th state `disabled` and route operator override there; defer until observed |
| 7 | **Embeddings traffic accidentally fast-failed** because of baseURL collision | Low | Gate keys on exact `cfg.EffectiveLLMEndpoint()` match (path-included). OpenAI endpoint is `https://api.openai.com/v1`, mlx is `http://127.0.0.1:8101/v1` — no overlap. Live Smoke 3 verifies | Make the match explicit `endpoint == cfg.EffectiveLLMEndpoint()` rather than substring; document in code comment |
| 8 | **Operator runs old `service install` (no `--with-mlx`)** and watchdog enabled but mlx not under launchd | Medium | Probe still works (it doesn't depend on launchd); restarts just won't auto-fire. `mdemg watchdog status` clearly shows "launchd: not registered" if so | Document the install workflow in CLAUDE.md MLX Watchdog subsection; `Required: false` slice entry means missed install doesn't break service install for other plists |
| 9 | **mlx_lm.server flags drift** between this plist and Phase 12 production invocation | Medium | Reference Phase 12's `phase_12_uvts_post.md` for the canonical flag set; capture in plist comment; flag any drift in `mdemg watchdog status` (parse mlx invocation from `launchctl print`) | Operator overrides via `mdemg service install --mlx-flags "..."` if needed; defer until observed |
| 10 | **macOS launchd quirks** (e.g., LaunchAgent restart cadence varies under power management) | Low | LaunchAgent runs in user session; sleep/wake cycles will pause restarts — accepted as out-of-scope (operator visible via `mdemg watchdog status`) | Document in CLAUDE.md MLX Watchdog subsection; add a `mdemg watchdog test --kill` for live-validation any time after a config change |
| 11 | **Goroutine panic in probe** crashes mdemg | Very Low | Supervisor wraps with `defer recover()`; auto-restart with exponential backoff (5s, 10s, 20s, cap 3 attempts); after 3 attempts, alerts operator | Disable watchdog: `MLX_WATCHDOG_ENABLED=false` + restart mdemg |
| 12 | **`launchctl print` output format changes** in a future macOS update — `mdemg watchdog status` parsing breaks | Low | Output parsing is best-effort with fallback to "unknown"; CLI doesn't exit non-zero on parse failure | Switch parser to `launchctl list` (older, more stable format) if needed |
| 13 | **Native binary on macOS host needs `LLM_ENDPOINT=127.0.0.1` override** (known from Phase 11.6.2 live test) | Certain | Watchdog reads from `cfg.EffectiveLLMEndpoint()` which already handles this; documented in CLAUDE.md; runbook in plist comments | If `cfg.EffectiveLLMEndpoint()` resolves wrong on a host, operator sets explicit env var |

## 11. Documents Accessed (during planning)

**Read during planning (3 parallel Explore agents):**
- `/Users/reh3376/mdemg/internal/healthprobe/prober.go` — ticker-driven HTTP probing pattern with timeout + state, panic-recovered via supervisor.
- `/Users/reh3376/mdemg/internal/supervisor/supervisor.go` — exponential-backoff goroutine restart (5s base, 2× multiplier, 3-restart cap) with panic recovery.
- `/Users/reh3376/mdemg/internal/circuitbreaker/breaker.go` — atomic-state pattern, `/v1/admin/breakers` admin endpoints (operator escape hatch).
- `/Users/reh3376/mdemg/internal/alert/dispatcher.go` — `Send(ctx, severity, service, title, message)` + cooldown dedup (atomic `TryRecord()` post-DH-004).
- `/Users/reh3376/mdemg/internal/llmclient/client.go` — line 83 baseURL field; line 401-440 shouldRetry; line 471 doWithRetry entry (gate insertion point).
- `/Users/reh3376/mdemg/packaging/launchd/com.mdemg.server.plist` — template substitution pattern (`__MDEMG_BIN__`, `__PROJECT_DIR__`, `__HOME__`); KeepAlive + ThrottleInterval idiom.
- `/Users/reh3376/mdemg/internal/cli/service.go` + `service_darwin.go` — `launchdServices` slice as registry of plist templates.
- `/Users/reh3376/mdemg/internal/metrics/collectors.go` — Prometheus collector registration pattern (mirror DH-005 dimension confidence gauges).
- `/Users/reh3376/mdemg/internal/cli/serve.go` — early-writer block pattern (anything that writes Prometheus state or registers callbacks must initialize before `api.NewServer()`).
- `/Users/reh3376/mdemg/internal/config/config.go` — `FromEnv()` pattern for new knobs; `Validate()` cross-field checks.
- `/Users/reh3376/mdemg/CLAUDE.md` — Architecture Notes section (LLM recorder init order; context helpers WithSpaceID/WithSessionID); Testing section with 3-tier mandate (live testing required, commit `d10c1a5`).
- `/Users/reh3376/mdemg/AGENT_HANDOFF.md` — Phase 12 close note + open follow-ups (sets context for this sprint as next-in-line).
- `/Users/reh3376/mdemg/docs/development/post-ft-lora/phase_12_uvts_post.md` — mlx crash report excerpts + retry-storm observation that motivate this sprint.
- `/Users/reh3376/mdemg/docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` — Phase 13 dependency on stable mlx (the unblock criterion).
- `/Users/reh3376/mdemg/internal/ape/conflict_tracker_hook_test.go` — Phase 12 Epic 6 nil-safe-Track test pattern; reference for nil-pool fail-open idiom in mlxprobe.
- `/Users/reh3376/mdemg/CHANGELOG.md` — current `[Unreleased]` section (where Phase 11.6.3 entries land).
- Memory: `feedback_sprint_plan_format.md`, `feedback_sprint_summary_on_pr.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `feedback_sprint_plans_location.md`, `feedback_live_testing_required.md`, `project_mdemg_purpose.md`.

## 12. Rollback

All changes additive — no schema migration, no destructive ops.

1. `git revert <final commit SHA>` — removes mlxprobe package, llmclient gate, watchdog CLI, plist template, service_darwin extension, metrics, config knobs, docs.
2. `mdemg service uninstall` — auto-detects mlx-server entry from rolled-back `launchdServices` slice and unloads `com.mdemg.mlx-server` plist (manual: `launchctl bootout gui/$(id -u)/com.mdemg.mlx-server && rm ~/Library/LaunchAgents/com.mdemg.mlx-server.plist`).
3. **Runtime emergency disable** (no rebuild needed): set `MLX_WATCHDOG_ENABLED=false` in `.env` + restart mdemg. The probe goroutine never starts; gate in llmclient is no-op (because `mlxprobe.Default()` returns nil, fail-open).
4. **Hot disable fast-fail only** (probe still observes): set `MLX_FAIL_FAST_ENABLED=false` + restart mdemg. Restores pre-sprint retry behavior under mlx outage. Useful for debugging.
5. mlx itself: if launchd plist installed but operator wants manual control, `launchctl bootout gui/$(id -u)/com.mdemg.mlx-server` stops auto-restart; `mdemg service uninstall` removes the plist file.
6. **No data to roll back** — no TSDB rows written by this sprint. Alert events in `~/.mdemg/alerts/current.json` are bounded (50-entry default) and self-purge.

Phase 11.5 + 11.6 + 11.6.x + 11.6.2 + 12 artifacts untouched. mlx model files untouched. Production `mdemg-llm-v1` symlink untouched.

---

## Plan-Options (decision forks — pick at execution, disclose in PR)

Per MEMORY `feedback_plan_options_pattern.md`:

| Fork | Recommended | Alternative(s) | Rationale for recommendation |
|---|---|---|---|
| **Architecture** | Launchd-only restart + mdemg-side probe + llmclient fast-fail | (B) Separate `cmd/mlx-watchdog/` Go binary; (C) Shell wrapper script | Launchd's KeepAlive+ThrottleInterval already does restart-on-crash; a Go binary just wraps the same OS facility. Shell wrapper duplicates that even more. Reuses existing 5-plist pattern. |
| **Probe interval / timeout** | 5s / 2s (~15s detection, <100ms/min overhead) | (B) 10s / 3s slower-but-cheaper; (C) 1s / 500ms — too aggressive, risks flaps under transient slowdowns | 5s/2s matches `healthprobe` defaults; detection is fast enough that retry-storm never engages; load is negligible. |
| **Fast-fail on `degraded` state** | NO — only fast-fail on `down`; degraded is log-only/informational | (B) Fast-fail on degraded too (more aggressive); (C) Add a 4th `disabled` state | Degraded means slow-but-responding; turning slowness into hard failure is worse than the slowness itself. Operators want a gradient, not a cliff. |
| **`MLX_WATCHDOG_ENABLED` default** | `false` initially; flip to `true` AFTER Live Smoke 2 passes (within this sprint) | Day-1 default `true` | Keeps the rollback surface tiny while soak-validating; flipping inside the same sprint is still single-commit (config default change is part of Epic 8). |
| **Plist install required vs optional** | `Required: false` (opt-in via `--with-mlx`) | `Required: true` (always installs) | Some hosts won't have mlx_lm on PATH (Docker-only deployments, CI runners); skipping by default is the safe choice. Phase 13 may flip to `Required: true` once Apple-Silicon-only deployment is the proven hot path. |
