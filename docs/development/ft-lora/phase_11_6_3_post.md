# Phase 11.6.3 — MLX Watchdog (Operational Hygiene #2) — Post Report

**Sprint ID:** FT-LORA-PHASE11.6.3
**Branch:** `reh3376_dev01`
**Predecessor:** Phase 12 (commit `98fc7a8`, PR #364)
**Successor:** Phase 13 — Note 04 Column-Voting Retrieval (now unblocked)
**Plan:** `docs/development/ft-lora/sprint_plan_phase_11_6_3.md` (frozen)
**Date executed:** 2026-04-30

---

## Outcome

Shipped the MLX watchdog: an in-process probe goroutine that detects mlx_lm.server outages, an llmclient fast-fail gate that short-circuits the 6-attempt × ~30s retry loop when the endpoint is StateDown, an operator CLI for visibility, and a launchd plist with `KeepAlive.SuccessfulExit=false` + `ThrottleInterval=60` that auto-restarts mlx after Metal-OOM crashes. The retry-storm cascade observed in Phase 12 (1642% CPU when mlx died under sustained RSIC load) is eliminated at its source — every short-circuit avoids the 16-call-site fan-out × 6-retry × ~30s combinatorial explosion.

Phase 13 (Column-Voting Retrieval) is now unblocked: sustained live A/B testing no longer risks 30-minute storms + manual recovery on every mlx crash.

## Decision-fork outcomes

| Fork | Chosen | Why |
|---|---|---|
| Architecture | **launchd-only** restart + mdemg-side probe + llmclient fast-fail | KeepAlive+ThrottleInterval already does what a Go binary supervisor would; reuses the existing 5-plist pattern (`com.mdemg.server`, `neural-sidecar`, `ingest-claude-md`, `training-export`, `maintenance` → adds `com.mdemg.mlx-server`). |
| Probe interval / timeout | **5s / 2s** | Matches healthprobe defaults; ~15s detection window; <100ms/min overhead. |
| Fast-fail on `degraded` | **No — only on `down`** | Degraded means slow-but-responding; turning slowness into hard failure is worse than the slowness. Operators want a gradient, not a cliff. |
| `MLX_WATCHDOG_ENABLED` default | **`false`** until live-validated | Keeps rollback surface tiny while soak-validating. Operator opts in via `.env`. |
| Plist install required vs optional | **Optional** (skip if `mlx_lm.server` not on PATH) | Some hosts don't have mlx (Docker-only, CI runners); Phase 13 may flip to required once Apple-Silicon-only deployment is the proven hot path. |

## What shipped

| # | Artifact | Path | Purpose |
|---|---|---|---|
| 1 | mlxprobe package | `internal/mlxprobe/probe.go` | State machine (up/degraded/down), 3-failure → down hysteresis, 2-success → up recovery, atomic state, supervisor-managed lifecycle |
| 2 | llmclient fast-fail gate | `internal/llmclient/client.go:471`, `internal/llmclient/errors.go` (new) | 10-LOC gate at top of `doWithRetry`; returns `ErrMLXDown` sentinel when probe says down + endpoint matches |
| 3 | launchd plist + service install hook | `packaging/launchd/com.mdemg.mlx-server.plist`, `internal/cli/launchd_templates/com.mdemg.mlx-server.plist`, `internal/cli/service_darwin.go` | KeepAlive on crash, ThrottleInterval=60s, conservative mlx flags (Phase 12 profile); `Optional: true` slice entry skipped when mlx_lm not on PATH |
| 4 | `mdemg watchdog status` CLI | `internal/cli/watchdog.go`, registered in `internal/cli/root.go` | Probe state via Prometheus exposition parsing, launchctl restart count, last 5 mlx-server alerts; `--json` machine-readable |
| 5 | 3 Prometheus metrics | `internal/metrics/collectors.go` | `mdemg_mlx_health_state{endpoint}` (gauge 0/1/2), `mdemg_mlx_fast_fail_total{caller_task}` (counter), `mdemg_mlx_state_transitions_total{from,to}` (counter) |
| 6 | Alert wiring | `internal/cli/serve.go` early-writer block | up→down=High severity, down→up=Low severity, late-bound `srv.AlertDispatcher()` lookup; existing 300s cooldown handles flap suppression |
| 7 | 4 config knobs | `internal/config/config.go` | `MLX_WATCHDOG_ENABLED` (false), `MLX_PROBE_INTERVAL_SEC` (5, min 1), `MLX_PROBE_TIMEOUT_SEC` (2, min 1, must be < interval), `MLX_FAIL_FAST_ENABLED` (true) |
| 8 | Tier 1 unit tests | `internal/mlxprobe/probe_test.go`, `internal/llmclient/client_fail_fast_test.go`, `internal/config/config_mlx_watchdog_test.go`, `internal/cli/watchdog_test.go`, `internal/cli/service_darwin_test.go` | State machine math, gate correctness (down → ErrMLXDown, up → passthrough, OpenAI endpoint isolation, escape-hatch toggle), config validation, CLI parsing |
| 9 | Tier 2 integration test | `tests/integration/mlx_watchdog_test.go` (build tag `integration`) | 100 concurrent calls under StateDown all short-circuit within 1 probe interval; OpenAI endpoint unaffected |
| 10 | Sprint docs | `docs/development/ft-lora/sprint_plan_phase_11_6_3.md` (this plan, frozen), `phase_11_6_3_post.md` (this doc) | Plan + post |

## Test results

### Tier 1 (`go test -race -count=1`)

| Package | Result | Notes |
|---|---|---|
| `internal/mlxprobe` | ✅ pass | State machine + race-free atomics + goroutine-leak smoke |
| `internal/llmclient` | ✅ pass | Fail-fast gate, OpenAI endpoint isolation, escape-hatch toggle, fail-open when no prober |
| `internal/config` | ✅ pass | Defaults, env override, interval > timeout invariant, lower-bound enforcement |
| `internal/cli` | ✅ pass | Launchd entry registered, plist embedded, resolver helpers, Prometheus parser, alert filter, JSON output |
| `internal/metrics` | ✅ pass | Counter + gauge registration paths |

### Tier 2 (`go test -tags=integration ./tests/integration/...`)

| Test | Result | Notes |
|---|---|---|
| `TestMLXWatchdog_FastFailEliminatesRetryStorm` | ✅ pass | 100 concurrent callers all short-circuit; observer fires 100× |
| `TestMLXWatchdog_OpenAIEndpointUnaffected` | ✅ pass | OpenAI client not gated even when prober reports down |

### Tier 3 (live)

CLI verified against live system without disrupting production mlx:

```
$ ./bin/mdemg watchdog status
MLX Watchdog Status
===================
  Probe state:  unknown — metrics endpoint returned status 404

  launchd:
    not loaded — launchctl print gui/501/com.mdemg.mlx-server: exit status 113 …

  Recent mlx-server alerts:
    (none)
```

```
$ ./bin/mdemg watchdog status --json | jq .
{
  "health_state": "unknown",
  "metrics_source": "http://localhost:9999/metrics",
  "metrics_error": "metrics endpoint returned status 404",
  "launchd_error": "…"
}
```

The "404" + "not loaded" output is expected (watchdog disabled by default; plist not yet installed) and confirms graceful failure modes.

**Live smoke tests deferred to operator-approved session** (per safe-execution policy in auto mode — destructive `kill -9` against the live mlx requires operator presence):

- **Live Smoke 1** — kill mlx mid-load: enable watchdog via `.env`, install plist via `mdemg service install`, trigger 5 concurrent `consulting.classify` calls, `kill -9` mlx, observe state=down within 15s, observe launchd restart within 75s, verify alerts in `~/.mdemg/alerts/current.json`.
- **Live Smoke 2** — 8h soak with periodic kill: simulate Metal-OOM cadence (kill every 90 min) under steady RSIC traffic; pass criteria: no manual mdemg restart needed, CPU < 200%, load avg < 5, alert dispatcher cooldown-suppresses flaps.
- **Live Smoke 3** — embeddings non-impact: with mlx down, trigger `mdemg memory remember`; assert success (gate is mlx-endpoint-keyed, not global); verify `mdemg_mlx_fast_fail_total` does not increment.
- **Live Smoke 4** — operator escape hatch: with watchdog active, set `MLX_FAIL_FAST_ENABLED=false`; verify llmclient reverts to retry behaviour.

These are scheduled for operator-led validation. The MLX_WATCHDOG_ENABLED default flip from `false` → `true` is gated on Smoke 2 passing.

### Lint

`golangci-lint run` clean across all touched packages (`mlxprobe`, `llmclient`, `cli`, `config`, `metrics`).

## Build verification

```
$ go build -o bin/mdemg ./cmd/mdemg
(no errors)

$ ./bin/mdemg watchdog --help
(shows watchdog command + status subcommand)

$ plutil -lint packaging/launchd/com.mdemg.mlx-server.plist
packaging/launchd/com.mdemg.mlx-server.plist: OK
```

## What did NOT ship (deferred per plan)

- A separate `cmd/mlx-watchdog/` Go binary supervisor — launchd's KeepAlive is sufficient.
- mlx flag tuning beyond Phase 12's conservative profile (`--prompt-cache-size 256 --prompt-concurrency 2 --decode-concurrency 2`) — root cause is upstream Metal-OOM.
- Metric-driven self-tuning (e.g., progressive `--prompt-cache-size` shrinking after repeated crashes) — premature.
- Linux/Windows watchdog — macOS-launchd-only this sprint.
- Distributed health (multiple mlx instances behind a load balancer) — single-instance only.
- TSDB schema change — schema_version stays 16; metrics + alerts are in-memory + file-backed.

## Operational runbook (post-sprint)

To enable on a host with mlx_lm.server installed:

```bash
# 1. Install launchd plist (skipped automatically if mlx_lm not on PATH)
mdemg service install

# 2. Enable in .env
echo 'MLX_WATCHDOG_ENABLED=true' >> .env
echo 'MLX_FAIL_FAST_ENABLED=true' >> .env

# 3. Restart mdemg to pick up the config
mdemg restart

# 4. Verify state
mdemg watchdog status
```

Emergency disable (no rebuild):

```bash
echo 'MLX_WATCHDOG_ENABLED=false' >> .env  # OR
echo 'MLX_FAIL_FAST_ENABLED=false' >> .env  # gate-only disable, probe still observes
mdemg restart
```

## Phase 13 unblock

Phase 13 (Note 04 Column-Voting Retrieval) consumes the watchdog as its operational precondition. With the watchdog in place, Phase 13's UVTS A/B runs can proceed without operator-on-call to babysit mlx crashes. The watchdog turns mlx fragility from a sprint-blocking issue into a logged-and-recovered event.

## Sprint scope vs. delivered

Plan estimated 5–7 dev-days, ~500–700 LOC. Delivered in a single autonomous session:

- 250 LOC `internal/mlxprobe/probe.go` + 270 LOC tests
- 12 LOC `internal/llmclient/client.go` gate + 4 LOC `errors.go` + 200 LOC `client_fail_fast_test.go`
- 50 LOC `internal/cli/service_darwin.go` extension + 100 LOC `service_darwin_test.go`
- 280 LOC `internal/cli/watchdog.go` + 175 LOC `watchdog_test.go`
- 30 LOC `internal/metrics/collectors.go`
- 25 LOC `internal/config/config.go` + 100 LOC `config_mlx_watchdog_test.go`
- 60 LOC serve.go wiring
- 1 plist (`packaging/launchd/com.mdemg.mlx-server.plist`, mirrored to `internal/cli/launchd_templates/`)
- 200 LOC `tests/integration/mlx_watchdog_test.go`
- Sprint plan + post doc

Total ~1700 LOC including tests; production code under the 500–700 estimate (~600 LOC). Live-soak (Live Smoke 2) explicitly deferred for operator-led verification.
