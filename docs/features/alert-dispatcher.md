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

**Atomic check-and-record (DH-004, 2026-04-17):** The cooldown tracker exposes `TryRecord(service, severity)` which checks the cooldown and stamps the entry under a single lock acquisition. The dispatcher uses this atomic path instead of separate `Allow()` + `Record()` calls — closing a TOCTOU race where concurrent `Dispatcher.Send()` calls could both pass `Allow()` before either recorded. Under concurrent load exactly one caller wins per cooldown window.

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
| `neo4j_high_cpu` | 5-min windowed AVG CPU > `NEO4J_CPU_ALERT_THRESHOLD_PERCENT` (default 500; docker-stats % is per-single-core) | 5m | warning |
| `graph_node_drop` | drop > 100/1h | 5m | critical |
| `rate_limiting_active` | rejects > 10/s | 2m | info |
| `low_cache_hit_ratio` | ratio < 0.5 | 10m | info |
| `jiminy_follow_rate_drop` | rate < 0.3 | 30m | warning |

Rules are defined in `internal/alert/rules.go`. SQL queries run against the `metric_samples` table via `pgxpool.Pool`. ForDuration state tracking ensures alerts only fire after the condition persists for the specified duration.

> ⚠️ The table above is the **original** Phase-3 rule set and has since drifted.
> `high_p95_latency`, `critical_p99_latency`, and `neo4j_pool_exhausted` were
> **removed** by TSDB-CONSUME-001 (they read lifetime-cumulative synthetic gauges
> or perpetually-zero fake gauges). Many rules are now config-parameterized and
> appended in `serve.go` (`OrphanRules`, `CoverageRules`, `RetrieveLatencyRules`,
> `Neo4jCPURule`, …). Treat `rules.go` + `serve.go` as the source of truth.

## Alert calibration (ALERT-TRUTH-001, 2026-06-30)

A Grafana review found 50 pending alerts while the substrate was healthy — the
channel was **miscalibration noise, not failure**. The fixes:

- **No `ORDER BY time DESC LIMIT 1`.** Every rule aggregates (`AVG`/`MAX`/`MIN`)
  + `COALESCE`s so it returns exactly one non-NULL row on an idle window. A
  single latest-sample read returns zero rows on idle (rule-health noise) and
  flaps on bursty gauges. The last 4 offenders (`neo4j_high_cpu`/`_memory`,
  `low_cache_hit_ratio`, `jiminy_follow_rate_drop`) were swept. **Contract pins**
  `TestAllRules_NoLimitOneAntiPattern` + `TestAllRules_DistinctServicePerSeverity`
  gather *every* rule group, so new rules are covered automatically.
- **Neo4j CPU is host-relative.** `docker stats` CPU% is reported **per single
  core**, so on multi-core hardware healthy parallel graph work (consolidation,
  Hebbian writes) routinely runs several hundred percent. The old fixed `80`
  fired on any real activity. `Neo4jCPURule(NEO4J_CPU_ALERT_THRESHOLD_PERCENT`,
  default **500**) evaluates the **5-min windowed AVG** against a host-relative
  threshold, calibrated above the worst normal-operation windowed AVG (live 24h
  max 471, 0 windows >500). Lower it on smaller machines. Distinct Service
  `neo4j-cpu` so it doesn't share the memory rule's `(neo4j, medium)` cooldown
  key. *Note:* the full 22-phase consolidation re-cluster on a large graph is
  expected to be CPU-heavy — reducing that cost is a separate concern.
- **LLM error-count floor.** `llm_error_rate_spike` requires an absolute
  `RSIC_LLM_ERROR_MIN_COUNT` (default 5) errors in addition to the rate gate, so
  a couple of transient errors at low call volume don't re-fire a HIGH alert. A
  genuine spike (e.g. a task at 9.6% / 9 errors) still fires — truthfully.
- **No stale NLI bias.** `GetNLICalibrationReport()` returns nil when the NLI
  sidecar isn't operational, so a gated-off sidecar's stale window can't emit a
  phantom `j17_nli_mean_bias` / pin a continuously-firing `nli_bias_alert`.

Companion Grafana dashboard fixes (same sprint): extended the latency histogram
buckets 10s→120s (LLM-backed p95/p99 were clamping at the 10s top bucket), and
corrected four panel threshold/unit/calc bugs (`P95 Latency`, `Neo4j Memory`,
`Null-Weight Abstraction Edges`, `Conversation Coverage Ratio`, `Avg
Comprehension`). New config: `NEO4J_CPU_ALERT_THRESHOLD_PERCENT` (500),
`RSIC_LLM_ERROR_MIN_COUNT` (5). Sprint: `docs/development/alert-truth-001/`.
