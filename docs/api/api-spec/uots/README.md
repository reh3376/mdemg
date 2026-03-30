# UOTS — Universal Observability Test Specification

Spec-driven observability validation for MDEMG metrics, dashboards, and alert rules.

## Structure

```
uots/
  schema/uots.schema.json     # JSON Schema for .uots.json specs
  runners/uots_runner.py      # UOTS runner
  specs/                       # Test specifications
    *.uots.json
```

## Spec Types

| Type | Validates |
|------|-----------|
| `prometheus_metrics` | Metric presence, type, labels, value ranges on `/v1/metrics/snapshot` |
| `grafana_dashboard` | Dashboard JSON: UID, panels, queries, tags |
| `alert_rules` | Prometheus alert YAML: names, severity, expressions, durations |
| `log_format` | Structured log format compliance (future) |
| `trace_propagation` | OpenTelemetry trace context propagation (future) |

## Running

```bash
# Validate all UOTS specs
python3 docs/api/api-spec/uots/runners/uots_runner.py \
  --spec-dir docs/api/api-spec/uots/specs/ \
  --base-url http://localhost:9999

# Validate single spec
python3 docs/api/api-spec/uots/runners/uots_runner.py \
  --spec docs/api/api-spec/uots/specs/prometheus_neo4j_graph.uots.json \
  --base-url http://localhost:9999
```

## Authority Split (UOTS vs UOBS)

- `UOTS` is the canonical framework for artifact-level observability contracts:
  - Prometheus metrics
  - Grafana dashboards
  - Alert rule files
- `UOBS` remains focused on runtime service observability checks:
  - health endpoints
  - dependency probes
  - runtime tracing/logging behavior

## Current Specs

| Spec | Type | Checks |
|------|------|--------|
| `prometheus_neo4j_graph` | prometheus_metrics | 9 per-space graph gauges |
| `prometheus_neo4j_pool` | prometheus_metrics | 7 connection pool gauges |
| `grafana_neo4j_dashboard` | grafana_dashboard | 12 panels, 12 PromQL queries |
| `alert_rules_neo4j` | alert_rules | 7 alert rules with severity/expr |

## Reference

See [FRAMEWORK_GOVERNANCE.md](../../../specs/FRAMEWORK_GOVERNANCE.md) for governance context.
