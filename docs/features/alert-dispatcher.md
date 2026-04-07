# Alert Dispatcher (SR-001)

Service resilience alert system that delivers MDEMG health events to the user in real-time through Claude Code hooks.

## Architecture

```
Detection Sources           Dispatcher              Delivery
  RSIC alert actions ──┐                        ┌── File backend (JSON)
  Circuit breaker CB ──┼── alert.Dispatcher ────┤
  Health prober      ──┤    (cooldown dedup)    └── macOS notifications
  Grafana webhook    ──┘                             (opt-in)
                                                     │
                                                     ▼
                                              prompt-context.sh
                                              session-start.sh
                                              (reads JSON file)
```

## Alert File Format

Location: `~/.mdemg/alerts/current.json` (configurable via `ALERT_FILE_PATH`)

```json
{
  "updated_at": "2026-04-07T10:00:00Z",
  "alerts": [
    {
      "id": "1712487600000000000",
      "time": "2026-04-07T10:00:00Z",
      "service": "circuit-breaker",
      "severity": "high",
      "title": "Circuit Breaker Opened: openai-embeddings",
      "message": "Circuit breaker \"openai-embeddings\" transitioned closed → open",
      "cleared": false
    }
  ]
}
```

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `ALERT_ENABLED` | `true` | Enable/disable the entire alert system |
| `ALERT_FILE_PATH` | `~/.mdemg/alerts/current.json` | Path to the alert JSON file |
| `ALERT_COOLDOWN_SEC` | `300` | Suppress duplicate alerts for this many seconds |
| `ALERT_MAX_ENTRIES` | `50` | Maximum alerts in file (FIFO eviction) |
| `ALERT_MACOS_NOTIFY` | `false` | Enable macOS notification center alerts |
| `ALERT_MACOS_NOTIFY_MIN_SEV` | `high` | Minimum severity for macOS notifications |

## Backends

### File Backend
- Atomic writes via `.tmp` + `os.Rename` for crash safety
- FIFO eviction when alert count exceeds `ALERT_MAX_ENTRIES`
- Newest alerts prepended (most recent first)
- Thread-safe concurrent reads

### macOS Notification Backend
- Uses `osascript` to display native notifications
- Build-tagged `//go:build darwin` with no-op stub for other platforms
- Opt-in via `ALERT_MACOS_NOTIFY=true`
- Filtered by minimum severity (`ALERT_MACOS_NOTIFY_MIN_SEV`)

## Cooldown Deduplication

Per-(service, severity) cooldown prevents alert storms. Same service+severity combination is suppressed for `ALERT_COOLDOWN_SEC` seconds after the first delivery. Different severities from the same service are not suppressed.

## Hook Delivery

### prompt-context.sh (every prompt)
Shows all non-cleared alerts, capped at 10 entries.

### session-start.sh (session start)
Shows only critical/high severity alerts with an "INVESTIGATE BEFORE PROCEEDING" banner.

## Alert Sources

| Source | Service | Severities |
|--------|---------|------------|
| RSIC: Jiminy critical | `jiminy` | critical |
| RSIC: Sidecar down | `neural-sidecar` | high |
| RSIC: Memory bloat | `memory` | medium |
| RSIC: Synergy overlap | `synergy` | low |
| RSIC: Generic log | `rsic` | medium |
| Circuit breaker open | `circuit-breaker` | high |
| Circuit breaker recovery | `circuit-breaker` | low |
| Health prober transition | (via callback) | varies |
| Grafana webhook | `grafana` | varies |
