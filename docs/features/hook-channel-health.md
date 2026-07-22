# Hook Channel Health (HOOKWIRE-001 + HOOKSYNC-001)

## Why

The Claude Code hooks are the delivery channel that makes MDEMG the
assistant's *internal dialogue* — per-prompt memory recall, Jiminy guidance,
/strict enforcement, alert delivery, and observation capture all flow
through them. That channel failed silently for months (a one-field stdin
contract mismatch; found only by a manual deep-dive), and its alert last
mile was severed by template↔live drift. This feature makes the channel
**drift-proof, self-monitoring, and triageable**.

## Choices

- **Templates are the single source.** `internal/cli/hook_templates/` is
  canonical; `.claude/hooks/` must equal it modulo `{{SPACE_ID}}`. A CI step
  fails the Build on drift (mirrors the compose/launchd parity checks).
- **Absence detection reuses `scheduled_job_events` (V0024)** — no new sink.
  Hooks POST heartbeats to `POST /v1/hooks/event`, recorded through the
  `jobhealth` policy point as `job_name='hook:<name>'`.
- **Two independent heartbeats**: `hook:prompt-context` per delivery (the
  monitored channel) and `hook:post-tool-observe` throttled
  (`HOOK_HEARTBEAT_COOLDOWN_SEC`, default 300) — the activity witness. The
  outage shape this guards against is exactly the one that happened: one
  hook kept working while the other silently died.
- **Cleared = delivered, not resolved.** Hooks display pending alerts then
  POST `/v1/alerts/clear` with the displayed ids. Conditions that persist
  re-fire new entries via the evaluator's ForDuration/cooldown machinery.
- **Loopback by default.** Compose publishes the API on
  `${MDEMG_BIND_ADDR:-127.0.0.1}`; the neural sidecar binds `127.0.0.1:8101`
  (config default + plist). Wide binds are explicit opt-ins.

## How it works

1. Each prompt: `prompt-context.sh` renders up to 10 pending alerts →
   clears them → recall → guidance (+ T1 bootstrap when coded constraints
   present) → warm + Hebbian-reinforce (background) → heartbeat
   (background). Session start renders critical/high alerts + degraded
   healthz.
2. The evaluator rule **`hook_channel_silent`** (service
   `hook-channel-silent`, high severity) fires when
   `hook:post-tool-observe` rows ≥ `HOOK_ACTIVITY_MIN_EVENTS` in
   `HOOK_SILENT_LOOKBACK_HOURS` AND `hook:prompt-context` rows = 0 —
   sessions demonstrably active while the per-prompt channel records
   nothing. The next contract drift self-reports instead of waiting for an
   audit.
3. `mdemg hooks doctor [--json]` triages in one shot: per-hook template
   parity, settings registration, server healthz, a stdin-contract
   self-test (real payload shape, asserts the synergy footer), alert-file
   state, and the last heartbeat age. Non-zero exit on any FAIL.

## How to use

```bash
mdemg hooks doctor                 # full triage; --json for machines
mdemg hooks install --type claude --force   # reinstall from templates
curl -X POST localhost:9999/v1/alerts/clear \
  -d '{"all_before":"2026-06-11T00:00:00Z"}'  # bulk-clear old alerts
```

Config (all default-on, no hardcoding):

| Env | Default | Meaning |
|---|---|---|
| `HOOK_HEALTH_ALERT_ENABLED` | `true` | enable the `hook_channel_silent` rule |
| `HOOK_SILENT_LOOKBACK_HOURS` | `24` | active-but-silent window |
| `HOOK_ACTIVITY_MIN_EVENTS` | `5` | activity heartbeats required before the rule is eligible |
| `HOOK_HEARTBEAT_COOLDOWN_SEC` | `300` | post-tool-observe heartbeat throttle |
| `MDEMG_BIND_ADDR` | `127.0.0.1` | compose publish address (`0.0.0.0` = wide, pair with `AUTH_API_KEYS`) |
| `NEURAL_HOST` | `127.0.0.1` | neural sidecar bind (pydantic-settings `env_prefix="NEURAL_"` + field `host`) |

Known limitation: PostToolUse fires only on successful tool completion
(HOOKWIRE-001), so the activity heartbeat undercounts on failure-heavy
stretches — the `HOOK_ACTIVITY_MIN_EVENTS` floor absorbs this.

Sprints: `docs/development/hookwire-001/`, `docs/development/hooksync-001/`.
