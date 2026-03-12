# UxTS Framework Plugin

A REASONING plugin that enriches MDEMG retrieval results with UxTS test coverage, spec compliance, and drift detection metadata.

## Overview

When the retrieval pipeline calls `Process()`, this plugin annotates each candidate with:

- `uxts_coverage_count` — number of specs covering this candidate
- `uxts_frameworks` — comma-separated framework list (e.g., `"uats,udts"`)
- `uxts_spec_names` — comma-separated spec file names
- `uxts_compliance` — `"pass"`, `"fail"`, `"unknown"`, or `"untested"`
- `uxts_drift` — `"clean"` or `"drift_detected"`

Well-tested candidates receive a small score boost (+0.02), known failures get a penalty (-0.05).

## Supported Frameworks

UATS, UDTS, UPTS, UBTS, USTS, UOBS, UOTS, UETS, UVTS, UAMS

## Build

```bash
cd plugins/uxts-module
make build
```

## Test

```bash
go test ./plugins/uxts-module/...
```

## Configuration

All configuration is passed via `manifest.json` config and received in the Handshake RPC:

| Key | Default | Description |
|-----|---------|-------------|
| `spec_root` | `./docs` | Root directory for spec files |
| `frameworks` | `uats,udts,...` | Comma-separated frameworks to index |
| `mdemg_endpoint` | `http://localhost:9999` | MDEMG API for result storage |
| `space_id` | `mdemg-dev` | Space for observation storage |
| `enable_drift_check` | `true` | Enable SHA256 drift detection |
| `coverage_boost` | `0.02` | Score boost for covered candidates |
| `failure_penalty` | `0.05` | Score penalty for failed candidates |
| `max_specs_per_event` | `50` | Max specs per incremental validation |

## UDTS Contract Specs

5 UDTS specs validate this plugin's gRPC contract:

- `uxts_module_handshake.udts.json`
- `uxts_module_healthcheck.udts.json`
- `uxts_module_shutdown.udts.json`
- `uxts_module_process_empty.udts.json`
- `uxts_module_process_with_candidates.udts.json`
