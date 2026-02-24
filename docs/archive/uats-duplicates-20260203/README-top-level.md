# MDEMG API Test Specifications (UATS)

**Version:** 1.0.1  
**Date:** 2026-01-29  
**Endpoints:** 45 specs, ~90 test variants

---

## Overview

Complete UATS test suite for all MDEMG API endpoints. Validates request/response contracts, error handling, and API behavior.

---

## Quick Start

```bash
# Install dependencies
pip install requests jsonpath-ng

# Start the MDEMG server (writes .mdemg.port with actual bound port)
./bin/mdemg &

# Run tests (Makefile reads port from .mdemg.port automatically)
make test-api
```

### Base URL Resolution

UATS spec files use `"base_url": "${MDEMG_BASE_URL}"` — an environment variable reference resolved at runtime by the runner. This keeps specs environment-agnostic.

**Resolution priority (highest wins):**

1. `--base-url` CLI flag (the Makefile always passes this)
2. `MDEMG_BASE_URL` environment variable (resolved from spec's `${MDEMG_BASE_URL}`)
3. Empty string (connection will fail — ensures misconfiguration is obvious)

The Makefile sets both `--base-url` and exports `MDEMG_BASE_URL` from `.mdemg.port` (falling back to port 9999). Contributors running the runner directly should set the env var:

```bash
export MDEMG_BASE_URL=http://localhost:$(cat .mdemg.port 2>/dev/null || echo 9999)
python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
    --spec docs/api/api-spec/uats/specs/health.uats.json
```

---

## Endpoint Coverage

### Health (2)

| Spec | Method | Endpoint |
|------|--------|----------|
| health | GET | /healthz |
| readiness | GET | /readyz |

### Core Memory (14)

| Spec | Method | Endpoint |
|------|--------|----------|
| retrieve | POST | /v1/memory/retrieve |
| ingest | POST | /v1/memory/ingest |
| ingest_batch | POST | /v1/memory/ingest/batch |
| reflect | POST | /v1/memory/reflect |
| stats | GET | /v1/memory/stats |
| nodes | GET/PATCH/DELETE | /v1/memory/nodes/{id} |
| consolidate | POST | /v1/memory/consolidate |
| archive_bulk | POST | /v1/memory/archive/bulk |
| symbols | POST | /v1/memory/symbols |
| distribution | GET | /v1/memory/distribution |
| consult | POST | /v1/memory/consult |
| suggest | POST | /v1/memory/suggest |
| cache_stats | GET | /v1/memory/cache/stats |
| query_metrics | GET | /v1/memory/query/metrics |

### Ingest Jobs (4)

| Spec | Method | Endpoint |
|------|--------|----------|
| ingest_trigger | POST | /v1/memory/ingest/trigger |
| ingest_status | GET | /v1/memory/ingest/status/{id} |
| ingest_cancel | POST | /v1/memory/ingest/cancel/{id} |
| ingest_jobs | GET | /v1/memory/ingest/jobs |

### Ingest Codebase (4 specs, 31 test cases)

| Spec | Method | Endpoint | Variants |
|------|--------|----------|----------|
| ingest_codebase | POST | /v1/memory/ingest-codebase | 18 |
| ingest_codebase_status | GET | /v1/memory/ingest-codebase/{id} | 4 |
| ingest_codebase_list | GET | /v1/memory/ingest-codebase | 2 |
| ingest_codebase_cancel | DELETE | /v1/memory/ingest-codebase/{id} | 3 |

### Learning (5)

| Spec | Method | Endpoint |
|------|--------|----------|
| learning_prune | POST | /v1/learning/prune |
| learning_stats | GET | /v1/learning/stats |
| learning_freeze | POST | /v1/learning/freeze |
| learning_unfreeze | POST | /v1/learning/unfreeze |
| learning_freeze_status | GET | /v1/learning/freeze/status |

### Conversation CMS (7)

| Spec | Method | Endpoint |
|------|--------|----------|
| conversation_observe | POST | /v1/conversation/observe |
| conversation_correct | POST | /v1/conversation/correct |
| conversation_resume | POST | /v1/conversation/resume |
| conversation_recall | POST | /v1/conversation/recall |
| conversation_consolidate | POST | /v1/conversation/consolidate |
| conversation_volatile_stats | GET | /v1/conversation/volatile/stats |
| conversation_graduate | POST | /v1/conversation/graduate |

### System (9)

| Spec | Method | Endpoint |
|------|--------|----------|
| capability_gaps | GET | /v1/system/capability-gaps |
| gap_interviews | GET | /v1/system/gap-interviews |
| pool_metrics | GET | /v1/system/pool-metrics |
| feedback | POST | /v1/feedback |
| metrics | GET | /v1/metrics |
| modules | GET | /v1/modules |
| plugins | GET | /v1/plugins |
| ape_status | GET | /v1/ape/status |
| ape_trigger | POST | /v1/ape/trigger |

---

## Directory Structure

```
docs/api-spec/uats/
├── schema/
│   └── uats.schema.json
├── specs/
│   ├── health.uats.json
│   ├── readiness.uats.json
│   ├── retrieve.uats.json
│   ├── ingest.uats.json
│   ├── ... (41 specs total)
├── runners/
│   └── uats_runner.py
└── README.md
```

---

## Makefile Targets

The Makefile uses dynamic port discovery via `.mdemg.port`:

```makefile
# Dynamic port discovery (reads .mdemg.port, falls back to 9999)
BASE_URL ?= http://localhost:$(shell cat .mdemg.port 2>/dev/null || echo 9999)
export MDEMG_BASE_URL ?= $(BASE_URL)

# Run all API tests
make test-api

# Test single endpoint
make test-api-health
make test-api-retrieve

# Smoke tests only (health + readiness)
make test-smoke

# All tests (UPTS parsers + UATS API)
make test-all

# Test with custom base URL (e.g., staging)
make test-api-remote BASE_URL=https://staging.example.com
```

---

## CLI Reference

```bash
# Set base URL from .mdemg.port (or use --base-url flag)
export MDEMG_BASE_URL=http://localhost:$(cat .mdemg.port 2>/dev/null || echo 9999)

# Validate single spec (uses MDEMG_BASE_URL from spec's ${MDEMG_BASE_URL})
python3 uats_runner.py validate \
    --spec specs/health.uats.json

# Validate with explicit --base-url (overrides spec's env var)
python3 uats_runner.py validate \
    --spec specs/health.uats.json \
    --base-url http://localhost:9999

# Validate all specs
python3 uats_runner.py validate-all \
    --spec-dir specs/ \
    --base-url $MDEMG_BASE_URL \
    --report report.json

# With auth token
python3 uats_runner.py validate-all \
    --spec-dir specs/ \
    --base-url $MDEMG_BASE_URL \
    --token "$API_TOKEN"

# Custom timeout (seconds)
python3 uats_runner.py validate \
    --spec specs/retrieve.uats.json \
    --base-url $MDEMG_BASE_URL \
    --timeout 60
```

---

## Requirements

```bash
pip install requests jsonpath-ng
```

---

## CI Integration

```yaml
# .github/workflows/api-tests.yml
name: API Tests

on: [push, pull_request]

jobs:
  api-tests:
    runs-on: ubuntu-latest
    services:
      neo4j:
        image: neo4j:5
        ports:
          - 7687:7687
        env:
          NEO4J_AUTH: neo4j/testpassword

    steps:
      - uses: actions/checkout@v4

      - name: Build and start server
        run: |
          go build -o bin/server ./cmd/server
          export PORT_FILE_PATH=.mdemg.port
          ./bin/server &
          # Wait for server to write its port file
          timeout 30s bash -c 'until [ -f .mdemg.port ]; do sleep 1; done'

      - name: Run UATS tests
        run: |
          pip install requests jsonpath-ng
          MDEMG_PORT=$(cat .mdemg.port)
          python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
            --spec-dir docs/api/api-spec/uats/specs/ \
            --base-url http://localhost:$MDEMG_PORT \
            --report api-report.json

      - name: Upload report
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: api-test-report
          path: api-report.json
```

---

## Comparison: UPTS vs UATS

| Aspect | UPTS (Parsers) | UATS (APIs) |
|--------|----------------|-------------|
| Scope | 16 languages | 41 endpoints |
| Input | Source files | HTTP requests |
| Output | Symbols JSON | HTTP responses |
| Validation | Symbol matching | Status, headers, body |
| Makefile | `make test-parsers` | `make test-api` |
| Directory | `docs/lang-parser/lang-parse-spec/upts/` | `docs/api-spec/uats/` |

---

## Example Output

```
============================================================
Health Check
GET /healthz
Status: ✓ PASS
HTTP: 200 (expected: 200)
Response Time: 8ms
Assertions: 2/2 passed

============================================================
Retrieve Memories
POST /v1/memory/retrieve
Status: ✓ PASS
HTTP: 200 (expected: 200)
Response Time: 127ms
Assertions: 3/3 passed

============================================================
Retrieve Memories [missing_query]
POST /v1/memory/retrieve
Status: ✓ PASS
HTTP: 400 (expected: 400)
Response Time: 5ms
Assertions: 1/1 passed

============================================================
UATS Test Summary
============================================================
Base URL: http://localhost:9999
Total Specs: 41
Total Variants: 58
Passed: 58
Failed: 0
Errors: 0
Pass Rate: 100.0%
```

---

## Adding New Specs

1. Create `docs/api-spec/uats/specs/<name>.uats.json`
2. Follow the schema structure
3. Add error case variants
4. Run `make test-api-<name>`
5. Commit

---

## Stats

- **Total Specs:** 45
- **Categories:** 7 (Health, Memory, Ingest Jobs, Ingest Codebase, Learning, Conversation, System)
- **Test Variants:** ~90
- **Most Complex:** ingest_codebase.uats.json (18 variants covering all config options)
- **Lines of Code:** ~2,500 (runner)
