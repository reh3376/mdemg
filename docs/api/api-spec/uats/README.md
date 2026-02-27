# MDEMG API Test Specifications (UATS)

**Version:** 1.1.0
**Date:** 2026-02-26
**Specs:** 124 canonical specs + 7 drafts

---

## Overview

Complete UATS test suite for all MDEMG API endpoints. Validates request/response contracts, error handling, and API behavior.

---

## Quick Start

```bash
# Extract to mdemg root
tar -xzf uats-mdemg-full-v1.0.0.tar.gz

# Install dependencies
pip install requests jsonpath-ng

# Add Makefile targets
cat Makefile.uats >> Makefile

# Start server, then run tests
make test-api
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
docs/api/api-spec/uats/
├── schema/
│   └── uats.schema.json
├── specs/
│   └── *.uats.json          # 124 canonical spec files
├── drafts/
│   └── *.uats.json          # 7 draft specs (not run in CI)
├── runners/
│   └── uats_runner.py       # v1.1.0
└── README.md
```

---

## Makefile Targets

```makefile
# Run all API tests
test-api:
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
  --spec-dir docs/api/api-spec/uats/specs/ \
  --base-url http://localhost:9999 \
  --report /tmp/api-report.json

# Test single endpoint
test-api-%:
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
  --spec docs/api/api-spec/uats/specs/$*.uats.json \
  --base-url http://localhost:9999

# Smoke tests only (health + readiness)
test-smoke:
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
  --spec docs/api/api-spec/uats/specs/health.uats.json \
  --base-url http://localhost:9999
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate \
  --spec docs/api/api-spec/uats/specs/readiness.uats.json \
  --base-url http://localhost:9999

# Test by category
test-api-memory:
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
  --spec-dir docs/api/api-spec/uats/specs/ \
  --base-url http://localhost:9999 \
  --pattern "*retrieve*|*ingest*|*reflect*|*stats*"

test-api-learning:
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
  --spec-dir docs/api/api-spec/uats/specs/ \
  --base-url http://localhost:9999 \
  --pattern "learning_*.uats.json"

test-api-conversation:
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
  --spec-dir docs/api/api-spec/uats/specs/ \
  --base-url http://localhost:9999 \
  --pattern "conversation_*.uats.json"
```

---

## CLI Reference

```bash
# Validate single spec
python3 uats_runner.py validate \
    --spec specs/health.uats.json \
    --base-url http://localhost:9999

# Validate all specs
python3 uats_runner.py validate-all \
    --spec-dir specs/ \
    --base-url http://localhost:9999 \
    --report report.json

# With auth token
python3 uats_runner.py validate-all \
    --spec-dir specs/ \
    --base-url http://localhost:9999 \
    --token "$API_TOKEN"

# Custom timeout (seconds)
python3 uats_runner.py validate \
    --spec specs/retrieve.uats.json \
    --base-url http://localhost:9999 \
    --timeout 60
```

---

## Requirements

```bash
pip install requests jsonpath-ng
```

---

## Runner Feature Coverage (Current)

The UATS runner enforces the fields currently used by active specs, including:

- `config.response_time_max_ms`
- `config.follow_redirects`
- `variants[].variables`

The runner now fails fast when a spec uses schema features that are defined but not implemented in runtime behavior yet (for example: `setup`, `teardown`, `chain`, `request.body_file`, `expected.body_file`, `expected.body_schema`, and OAuth2 auth mode).

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
          ./bin/server &
          sleep 10
      
      - name: Run UATS tests
        run: |
          pip install requests jsonpath-ng
          python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
            --spec-dir docs/api/api-spec/uats/specs/ \
            --base-url http://localhost:9999 \
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
| Scope | 27 languages | 124 API specs |
| Input | Source files | HTTP requests |
| Output | Symbols JSON | HTTP responses |
| Validation | Symbol matching | Status, headers, body |
| Makefile | `make test-parsers` | `make test-api` |
| Directory | `docs/lang-parser/lang-parse-spec/upts/` | `docs/api/api-spec/uats/` |

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

1. Create `docs/api/api-spec/uats/specs/<name>.uats.json`
2. Follow the schema structure
3. Add error case variants
4. Run `make test-api-<name>`
5. Commit

---

## Stats

- **Canonical Specs:** 124
- **Draft Specs:** 7
- **Categories:** 15+ (Health, Memory, Ingest Jobs, Ingest Codebase, Learning, Conversation, System, Backup, CMS, Self-Improve/RSIC, Hash Verification, Scraper, Admin Spaces, Meta-Learn, Guardrails)
- **Most Complex:** ingest_codebase.uats.json (18 variants covering all config options)
- **Runner Version:** 1.1.0 (fail-fast for unimplemented schema features)
