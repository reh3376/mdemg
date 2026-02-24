# UBTS - Universal Benchmark Test Specification

A standardized framework for defining and running performance benchmarks against MDEMG.

## Overview

UBTS provides:

- **Declarative Specs**: JSON-based benchmark definitions
- **Reusable Profiles**: Load configurations (smoke, load, stress)
- **Automated Runner**: Python script for executing benchmarks
- **Threshold Validation**: Automatic pass/fail based on SLOs

## Directory Structure

```
ubts/
├── schema/
│   └── ubts.schema.json     # JSON Schema for validation
├── specs/
│   ├── retrieve_latency.ubts.json
│   ├── ingest_throughput.ubts.json
│   └── concurrent_load.ubts.json
├── profiles/
│   ├── smoke.profile.json   # Quick validation (10 requests)
│   ├── load.profile.json    # Normal load (1000 requests)
│   └── stress.profile.json  # Stress test (10000 requests)
├── runners/
│   └── ubts_runner.py       # Benchmark runner
└── README.md
```

## Quick Start

### 1. Install Dependencies

```bash
pip install requests
```

### 2. Run a Smoke Test

```bash
cd docs/tests/ubts
python runners/ubts_runner.py \
  --spec specs/retrieve_latency.ubts.json \
  --profile profiles/smoke.profile.json \
  --base-url http://localhost:9999
```

### 3. Run Full Load Test

```bash
python runners/ubts_runner.py \
  --spec specs/retrieve_latency.ubts.json \
  --profile profiles/load.profile.json \
  --output results/
```

### 4. Run All Benchmarks

```bash
python runners/ubts_runner.py \
  --spec "specs/*.ubts.json" \
  --profile profiles/stress.profile.json \
  --output results/
```

## Spec Format

```json
{
  "ubts_version": "1.0.0",
  "benchmark": {
    "name": "retrieve_latency",
    "endpoint": "/v1/memory/retrieve",
    "method": "POST",
    "payload_template": {
      "space_id": "{{space_id}}",
      "query_text": "{{query_text}}"
    }
  },
  "thresholds": {
    "p50_ms": 50,
    "p95_ms": 250,
    "p99_ms": 500,
    "error_rate_pct": 0.1
  }
}
```

## Thresholds

| Metric | Description | Example |
|--------|-------------|---------|
| `p50_ms` | 50th percentile latency | 50 |
| `p95_ms` | 95th percentile latency | 250 |
| `p99_ms` | 99th percentile latency | 500 |
| `max_ms` | Maximum latency | 2000 |
| `error_rate_pct` | Max error rate (%) | 0.1 |
| `throughput_rps` | Min throughput (rps) | 100 |

## Profile Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `total_requests` | Total requests to make | 1000 |
| `concurrent_users` | Concurrent connections | 10 |
| `ramp_up_seconds` | Ramp-up period | 30 |
| `think_time_ms` | Delay between requests | 500 |

## CI/CD Integration

Add to your pipeline:

```yaml
benchmark:
  script:
    - python docs/tests/ubts/runners/ubts_runner.py \
        --spec "docs/tests/ubts/specs/*.ubts.json" \
        --profile docs/tests/ubts/profiles/load.profile.json
  artifacts:
    paths:
      - results/
```

## Success Criteria

Phase 3 benchmarks must meet:

- p95 latency < 250ms under 100 concurrent requests
- Error rate < 0.1% under normal load
- Throughput > 100 rps sustained
