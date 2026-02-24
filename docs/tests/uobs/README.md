# UOBS - Universal Observability Specification

A standardized framework for validating observability infrastructure in MDEMG.

## Overview

UOBS provides:

- **Metrics Validation**: Verify Prometheus metrics presence and format
- **Health Checks**: Validate health endpoint responses
- **Tracing**: Verify distributed tracing propagation
- **Alerting Rules**: Pre-built Prometheus alert configurations
- **Dashboards**: Grafana dashboard templates

## Directory Structure

```
uobs/
├── schema/
│   └── uobs.schema.json     # JSON Schema for validation
├── specs/
│   ├── prometheus_metrics.uobs.json
│   ├── health_endpoints.uobs.json
│   └── log_format.uobs.json
├── alerts/
│   └── latency_slo.yaml     # Prometheus alerting rules
├── dashboards/
│   └── overview.json        # Grafana dashboard
├── runners/
│   └── uobs_runner.py       # Observability test runner
└── README.md
```

## Quick Start

### 1. Install Dependencies

```bash
pip install requests
```

### 2. Run Metrics Validation

```bash
cd docs/tests/uobs
python runners/uobs_runner.py \
  --spec specs/prometheus_metrics.uobs.json \
  --base-url http://localhost:9999
```

### 3. Run All Observability Tests

```bash
python runners/uobs_runner.py \
  --spec "specs/*.uobs.json" \
  --output results/
```

## Test Types

| Type | Description |
|------|-------------|
| `metrics` | Prometheus metrics validation |
| `health` | Health endpoint validation |
| `logging` | Log format validation |
| `tracing` | Distributed tracing validation |

## Required Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `mdemg_http_requests_total` | counter | Total HTTP requests |
| `mdemg_http_request_duration_seconds` | histogram | Request latency |
| `mdemg_retrieval_latency_seconds` | histogram | Retrieval latency |
| `mdemg_rate_limit_rejected_total` | counter | Rate limited requests |
| `mdemg_circuit_breaker_state` | gauge | Circuit breaker state |
| `mdemg_cache_hit_ratio` | gauge | Cache hit ratio |

## Health Endpoints

| Endpoint | Description | Max Response Time |
|----------|-------------|-------------------|
| `/healthz` | Liveness probe | 100ms |
| `/readyz` | Readiness probe | 5000ms |

## Tracing Headers

| Header | Description |
|--------|-------------|
| `X-Trace-ID` | Distributed trace identifier |
| `X-Request-ID` | Unique request identifier |

## Alerting Rules

Import `alerts/latency_slo.yaml` into Prometheus:

| Alert | Condition | Severity |
|-------|-----------|----------|
| MDEMGHighP95Latency | P95 > 250ms | warning |
| MDEMGCriticalP99Latency | P99 > 500ms | critical |
| MDEMGHighErrorRate | Error rate > 0.1% | warning |
| MDEMGCircuitBreakerOpen | Circuit open | warning |

## Grafana Dashboard

Import `dashboards/overview.json` into Grafana for:

- Request rate and latency graphs
- Error rate visualization
- Circuit breaker status
- Cache performance metrics

## CI/CD Integration

```yaml
observability-test:
  script:
    - python docs/tests/uobs/runners/uobs_runner.py \
        --spec "docs/tests/uobs/specs/*.uobs.json"
  artifacts:
    paths:
      - results/
```

## Success Criteria

- All required Prometheus metrics present
- Health endpoints respond within time limits
- Trace IDs propagated correctly
- Alert rules valid (promtool check)
