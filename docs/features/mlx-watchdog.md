---
created: 2026-05-04
updated: 2026-05-04
version: v0.6.0
author: reh3376
status: active
phase: phase 11.6.3 (with phase 13.5 backend-agnostic note)
---

# MLX Watchdog (LLM Endpoint Health)

## Summary

**Feature**: `mlx-watchdog`
**Summary**: Always-on supervisor that probes the local LLM endpoint (`/v1/models`) every 5 s, runs a 3-state machine (`up → degraded → down`) with hysteresis, fast-fails LLM client retries when the endpoint is `Down` (eliminates the retry-storm pattern that masked Metal-OOM crashes), records every state transition + fast-fail burst to TSDB V0018, and exposes operator visibility via `mdemg watchdog status`.

> **Naming history**: the feature is still named "MLX Watchdog" because that was the original Phase 11.6.3 name; the implementation is **backend-agnostic** post-Phase 13.5 (probes whichever endpoint `LLM_ENDPOINT` points at). Phase 13.6 (2026-05-04) renamed the env vars to `LLM_*` primaries while keeping `MLX_*` as deprecated aliases (logs a warning at startup; works indefinitely until ≥1 release cycle elapses). The internal Go package (`internal/mlxprobe/`) and Prometheus metric prefix (`mdemg_mlx_*`) retain their MLX names — those are operator-invisible / Grafana-coupled and out of scope for the env-var rename.

## Vision & Goals

The MDEMG vision is to be the developer's **persistent emergent long-term memory** — a cognitive substrate, not a tool. That framing demands an **always-on local LLM endpoint** because all 16 MDEMG LLM call sites (RSIC, Jiminy, consulting, retrieval rerank, ape.reflect, embeddings, …) depend on the endpoint being reachable. A crashed endpoint or a silently failing one degrades memory itself, not just one feature.

Phase 11.6.3 was triggered by a recurring Metal-OOM crash pattern in `mlx_lm.server`: the process would die every ~14 minutes under sustained load. The retry logic in `llmclient` would then re-attempt the call several times before failing — creating a retry-storm that wasted compute and masked the underlying liveness issue from operators. The watchdog exists to:

1. **Detect endpoint health independently** of any single LLM call (background probe at fixed interval)
2. **Short-circuit retries** when the endpoint is unequivocally `Down` (saves compute; surfaces failures fast)
3. **Capture history** so operators can see crash-rate trends in Grafana, not just live state
4. **Refuse mdemg startup** if the endpoint isn't reachable at boot (with `MDEMG_ALLOW_NO_MLX=1` operator escape hatch for offline development)

Phase 13.5 later replaced `mlx_lm.server` with `llama-server` (data-decided bake-off — see `local-llm-runtime.md`). The watchdog is still mandatory because the always-on principle didn't change.

## Current State

### Architecture

Three components:

| Component | Path | Responsibility |
|---|---|---|
| Probe + state machine | `internal/mlxprobe/` | Background goroutine that hits `/v1/models` every 5 s, runs the 3-state FSM with hysteresis, fires `OnTransition` callbacks |
| Fast-fail gate | `internal/llmclient/client.go:471` (`ErrMLXDown`) | Read by the retry layer at `doWithRetry`. If the watchdog state is `Down`, the call short-circuits to `ErrMLXDown` instead of retrying |
| TSDB writer | `internal/tsdb/llm_endpoint_health_writer.go` (V0018 hypertable) | Records state transitions + fast-fail bursts. Drives the Grafana "LLM Endpoint Health" panel |

State machine:

- **Up**: probe succeeded ≥ N consecutive times (where N defaults to 1 — recovery is fast)
- **Degraded**: probe failed but not enough consecutive failures to declare `Down`. LLM calls still attempted; observability flag for operators.
- **Down**: probe failed ≥ `MLX_MAX_CONSECUTIVE_FAILURES` (default 3) times in a row. Fast-fail gate active.

Hysteresis prevents flap: the machine doesn't transition `Up → Down` until the failure count crosses the threshold; doesn't transition `Down → Up` until a probe succeeds. Single transient failures stay in `Degraded` and don't trip the gate.

### Workflow

1. Server startup (`internal/cli/preflight_mlx.go`) probes the endpoint synchronously. If unreachable AND `MDEMG_ALLOW_NO_MLX != "1"`, mdemg refuses to start.
2. After successful preflight, the watchdog goroutine starts via `internal/mlxprobe.Start()`. Probe interval and timeout from env.
3. On every probe:
   - Success → reset failure counter; if state was `Degraded`/`Down`, transition to `Up` and record `state_transition` event in V0018
   - Failure → increment counter; if counter crosses threshold, transition `→ Down` and record event
4. LLM calls in `internal/llmclient` consult the watchdog state via `mlxprobe.IsDown()`. When `Down`, the retry loop short-circuits to `ErrMLXDown`.
5. Fast-fail bursts (rapid sequence of `ErrMLXDown` returns) are aggregated by the rate-limited observer and recorded as one `fast_fail_burst` row per window in V0018.

### Configuration

| Env Var (primary) | Legacy alias (deprecated) | Default | Description |
|---|---|---|---|
| `LLM_WATCHDOG_ENABLED` | `MLX_WATCHDOG_ENABLED` | `true` | Master toggle (default-on per Phase 11.6.3 scope) |
| `LLM_PROBE_INTERVAL_SEC` | `MLX_PROBE_INTERVAL_SEC` | `5` | Seconds between probes |
| `LLM_PROBE_TIMEOUT_SEC` | `MLX_PROBE_TIMEOUT_SEC` | `2` | Per-probe HTTP timeout |
| `LLM_FAIL_FAST_ENABLED` | `MLX_FAIL_FAST_ENABLED` | `true` | Let llmclient short-circuit when probe says StateDown |
| `MDEMG_ALLOW_NO_LLM` | `MDEMG_ALLOW_NO_MLX` | unset | When `1`, mdemg startup proceeds even if endpoint unreachable. Use for local dev without llama-server running |
| `LLM_ENDPOINT` | — | `http://127.0.0.1:8102/v1` | The endpoint the watchdog probes (Phase 13.5 default; was `8101` for mlx-server) |

> **Aliases**: setting the legacy `MLX_*` name still works but emits a deprecation log at startup. Aliases will be removed ≥1 release cycle after Phase 13.6. Operators should migrate to `LLM_*` primaries.

### launchd integration

When `mdemg service install --with-mlx` is invoked, the watchdog is implicitly enabled because the launchd plist that supervises the LLM server (`com.mdemg.llama-server.plist` post-Phase-13.5) is installed alongside mdemg's own plist. KeepAlive on crash + ThrottleInterval=30s gives the watchdog a known recovery cadence. See `local-llm-runtime.md` for the plist details.

## Choices that were made

### Why a 3-state machine instead of 2-state

A pure `Up/Down` machine produces flapping under intermittent failures. The middle `Degraded` state lets the watchdog distinguish "one slow probe" (no behavior change) from "consistently failing" (gate engaged). It also gives operators a richer signal — the Grafana panel shows degraded windows distinct from outages.

### Why probe `/v1/models` (not `/healthz`)

`/v1/models` is the OpenAI-compat endpoint that the LLM server itself answers — probing it tests the actual code path that production calls use. Probing a side-channel `/healthz` would risk passing while the model-loading code path was broken (the exact failure shape we'd want to detect).

### Why fast-fail at the retry layer (not at the call site)

The retry layer is the single chokepoint that all LLM calls flow through. Adding the gate there gives every call site the benefit without touching 16 individual integrations. Fast-fail at `doWithRetry` also preserves the existing back-off semantics for transient errors that aren't `Down` (502 from a healthy endpoint, network blips not severe enough to trip the watchdog).

### Why launchd over systemd / docker-compose

macOS-native operators are MDEMG's primary target (Apple Silicon for the local LLM). launchd KeepAlive gives the recovery semantic needed without adding a Docker dependency for the LLM runtime. The Linux/Windows equivalents are scoped for future sprints.

### Why default-on (true) and refuse-startup-if-unreachable

The cognitive substrate framing means a silent endpoint is worse than a noisy one. Defaulting watchdog on and refusing to boot with no LLM endpoint forces the operator to either fix it or explicitly opt out — surface the failure rather than hide it.

### Why backend-agnostic naming was kept after Phase 13.5

Phase 13.5's cutover to llama.cpp didn't change the watchdog contract — same probe, same state machine, same fast-fail. Renaming env vars (`MLX_*` → `LLM_*`) and the package (`mlxprobe` → `llmprobe`) would touch ~30 files for zero behavior change. Phase 13.6 will batch this naming sweep with other backend-agnostic cleanups.

## Notes

### Known limitations

- **Single-endpoint scope**: watchdog probes one endpoint. Multi-LLM topologies (llama-server + remote OpenAI fallback) need a generalization to N endpoints, scoped for a future sprint.
- **No predictive signals**: the watchdog reacts to observed failures; it does not predict crashes from memory pressure or token rate. A predictive layer (probe `nvtop`-equivalent on Apple Silicon) was considered and deferred — the reactive approach has been sufficient since Phase 13.5 stabilized the substrate.
- **Naming inconsistency**: `MLX_*` env vars + `mlxprobe` package name don't reflect the post-13.5 backend-agnostic behavior. Phase 13.6 cleanup queued.

### Risks & gaps

- **CI test gating**: integration tests sometimes need `MDEMG_ALLOW_NO_MLX=1` + `MLX_WATCHDOG_ENABLED=false` to avoid flakiness in environments without llama-server. This is documented in `.github/workflows/ci.yml` but not enforced by the test framework — tests that forget to opt out fail at boot.
- **Probe load**: 5 s interval × 24 hr × 365 days = ~6.3M probes/yr per instance. Each probe is `<10 ms` on the local loopback so load is trivial, but multi-instance deployments should consider sharing probe state.

### Future improvements

- Backend-agnostic rename (Phase 13.6, queued)
- Multi-endpoint support (no sprint scoped yet)
- Predictive signals via Apple Silicon GPU memory metrics

## API Endpoints

The watchdog exposes state via the existing `/healthz` endpoint (in the `checks` map under key `llm_endpoint`) and via Prometheus metrics. There is no dedicated HTTP endpoint.

| Method | Endpoint | Description | UATS Spec |
|---|---|---|---|
| GET | `/healthz` | Includes watchdog state in `checks.llm_endpoint`: `up`, `degraded`, or `down` | — |
| GET | `/metrics` | Exposes `mdemg_mlx_watchdog_state` (numeric: 0=up, 1=degraded, 2=down), `mdemg_mlx_watchdog_transitions_total{from,to}`, `mdemg_mlx_fast_fail_total` | — |

## CLI Commands

| Command | Description |
|---|---|
| `mdemg watchdog status` | Human-readable summary: current state, last transition timestamp, consecutive-failure counter, last probe error |
| `mdemg watchdog status --json` | Same data as JSON for piping to `jq` or scripts |

## Configuration Reference

See "Configuration" table above. All knobs live in `.env`; Validate() applies sane bounds at startup.

## Dependencies

| Feature | Relationship |
|---|---|
| `local-llm-runtime` | The thing being watched. Phase 13.5 cutover replaced the underlying server but the watchdog contract is unchanged |
| TSDB V0018 (`llm_endpoint_health_events`) | Persistence layer for state transitions + fast-fail bursts |
| Grafana "LLM Endpoint Health" panel | Consumes V0018 |

## Related Files

- `internal/mlxprobe/` — probe goroutine + state machine
- `internal/llmclient/client.go` — `ErrMLXDown` fast-fail gate
- `internal/cli/preflight_mlx.go` — startup probe + refuse-to-start logic
- `internal/cli/watchdog.go` — `mdemg watchdog status` CLI
- `internal/tsdb/llm_endpoint_health_writer.go` — V0018 writer
- `internal/tsdb/migrations/018_llm_endpoint_health.sql` — V0018 schema
- `packaging/launchd/com.mdemg.llama-server.plist` — supervised LLM server
- `docs/development/ft-lora/phase_11_6_3_post.md` — origin sprint
- `docs/development/post-ft-lora/phase_13_5_post.md` — backend cutover
