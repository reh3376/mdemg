# Alert Dispatcher (SR-001)

Service resilience alert system that delivers MDEMG health events to the user in real-time through Claude Code hooks.

## Architecture

```
Detection Sources           Dispatcher              Delivery
  Alert evaluator    ──┐                        ┌── File backend (JSON)
  RSIC alert actions ──┤                        │
  Circuit breaker CB ──┼── alert.Dispatcher ────┤
  Health prober      ──┤    (cooldown dedup)    └── macOS notifications
  LLM failures      ──┤                             (opt-in)
  TSDB writer        ──┤                             │
  Grafana webhook    ──┘  (backward compat)          ▼
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
| LLM consecutive failures | `llm-{task}` | high |
| TSDB buffer overflow | `tsdb-writer` | medium |
| Alert evaluator (13 rules) | varies | low–critical |
| Supervisor restart | `supervisor` | medium |
| Supervisor permanent failure | `supervisor` | critical |
| Grafana webhook (backward compat) | `grafana` | varies |

## Server-Native Alert Evaluator (SNA-001)

The evaluator (`internal/alert/evaluator.go`) queries TSDB on a periodic schedule and fires alerts through the dispatcher when thresholds are breached.

| Config | Default | Description |
|--------|---------|-------------|
| `ALERT_EVALUATOR_ENABLED` | `true` | Enable server-native rule evaluation |
| `ALERT_EVALUATOR_INTERVAL_SEC` | `30` | Base evaluation tick interval |

### Rules (13)

| Rule ID | Threshold | For Duration | Severity |
|---------|-----------|-------------|----------|
| `high_p95_latency` | P95 > 250ms | 5m | warning |
| `critical_p99_latency` | P99 > 500ms | 2m | critical |
| `high_error_rate` | 5xx > 0.1% | 5m | warning |
| `low_graph_health` | health < 0.5 | 10m | warning |
| `high_orphan_count` | orphans > 50 | 15m | warning |
| `high_orphan_ratio` | ratio > 10% | 15m | warning |
| `neo4j_high_memory` | mem > 80% | 5m | warning |
| `neo4j_high_cpu` | CPU > 80% | 5m | warning |
| `neo4j_pool_exhausted` | waiting > 5 | 2m | critical |
| `graph_node_drop` | drop > 100/1h | 5m | critical |
| `rate_limiting_active` | rejects > 10/s | 2m | info |
| `low_cache_hit_ratio` | ratio < 0.5 | 10m | info |
| `jiminy_follow_rate_drop` | rate < 0.3 | 30m | warning |

Rules are defined in `internal/alert/rules.go`. SQL queries run against the `metric_samples` table via `pgxpool.Pool`. ForDuration state tracking ensures alerts only fire after the condition persists for the specified duration.
