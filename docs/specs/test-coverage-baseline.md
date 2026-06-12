# Test Coverage Baseline

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-02-04
**Branch**: mdemg-dev01
**Command**: `go test -cover ./internal/...`

## Current Coverage by Package

| Package | Coverage | Notes |
|---------|----------|-------|
| `internal/anomaly` | 29.4% | Anomaly detection |
| `internal/ape` | 0.0% | Active Participant Engine (no tests) |
| `internal/api` | 11.3% | HTTP handlers |
| `internal/config` | 2.4% | Configuration loading |
| `internal/consulting` | 0.0% | Advisory service (no tests) |
| `internal/conversation` | 15.8% | CMS service |
| `internal/db` | 0.0% | Database driver (no tests) |
| `internal/domain` | 0.0% | Domain types (no tests) |
| `internal/embeddings` | 39.2% | Embedding clients |
| `internal/gaps` | 20.6% | Gap detection |
| `internal/hidden` | 15.1% | Hidden layer abstraction |
| `internal/jobs` | 0.0% | Job tracking (no tests) |
| `internal/learning` | 18.0% | Hebbian learning |
| `internal/models` | N/A | No test files (data types only) |
| `internal/observations` | 0.0% | Observation service (no tests) |
| `internal/plugins` | 16.9% | Plugin manager |
| `internal/plugins/scaffold` | 86.2% | Plugin scaffolding |
| `internal/retrieval` | 34.6% | Core retrieval engine |
| `internal/summarize` | 72.6% | LLM summaries |
| `internal/symbols` | 46.6% | Symbol extraction |
| `internal/validation` | 89.7% | Request validation |

## Coverage Gates

### New Code (Phase 1+)

- **Minimum**: 80% statement coverage for all new packages
- **Target**: 90%+ for critical paths (retrieval, conversation, learning)

### Existing Code

- No coverage regression allowed on existing packages
- Incremental improvement encouraged but not gated

## Verification

```bash
# Run with coverage
go test -cover ./internal/...

# Generate coverage profile
go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out

# HTML report
go tool cover -html=coverage.out -o coverage.html
```
