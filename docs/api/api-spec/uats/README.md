# MDEMG API Test Specifications (UATS)

**Version:** 1.2.0
**Date:** 2026-03-24
**Specs:** 195 canonical specs across 25 categories

---

## Overview

Complete UATS test suite for all MDEMG API endpoints. Validates request/response contracts, error handling, and API behavior. For the full UxTS methodology guide covering architecture, spec writing, CI integration, and all 11 frameworks, see [docs/guides/UXTS_DEVELOPER_GUIDE.md](../../../../docs/guides/UXTS_DEVELOPER_GUIDE.md).

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

## Endpoint Coverage (195 specs, 25 categories)

| Category | Specs | Key Endpoints |
|----------|------:|---------------|
| Health | 2 | `/healthz`, `/readyz` |
| Memory (core) | 33 | `/v1/memory/retrieve`, `ingest`, `nodes`, `consolidate`, `cache`, `cleanup` |
| Conversation CMS | 27 | `/v1/conversation/observe`, `recall`, `resume`, `templates`, `snapshots`, `org-reviews` |
| Jiminy + J17 | 21 | `/v1/jiminy/guide`, `evaluate`, `feedback`, `healthz`, `ready`, `protocol/*` |
| System | 21 | `/v1/system/capability-gaps`, `gap-interviews`, `/v1/ape/*`, `/v1/plugins` |
| RSIC | 18 | `/v1/self-improve/cycle`, `assess`, `health`, `history`, `rollback` |
| Hash Verification | 9 | `/v1/hash-verification/register`, `verify`, `scan`, `revert` |
| Constraints | 7 | `/v1/constraints/detect-conflicts`, `conflicts`, `effectiveness`, `scope` |
| Backup | 7 | `/v1/backup/trigger`, `list`, `restore`, `status` |
| Learning | 6 | `/v1/learning/stats`, `freeze`, `unfreeze`, `prune` |
| Admin/Transfer | 6 | `/v1/admin/spaces/*`, `/v1/admin/spaces/export`, `import` |
| Scraper | 6 | `/v1/scraper/jobs/*` |
| Neural | 4 | `/v1/neural/status`, sidecar `/health`, `/nli`, `/rerank` |
| Linear | 4 | `/v1/linear/issues`, `projects`, `comments`, `/v1/webhooks/linear` |
| Ingest Jobs | 3 | `/v1/memory/ingest/trigger`, `status`, `cancel` |
| Filewatcher | 3 | `/v1/filewatcher/start`, `status`, `stop` |
| Guardrail | 2 | `/v1/guardrail/events`, `/v1/memory/guardrail/validate` |
| Symbols | 2 | `/v1/symbols/relationships` |
| Other | 14 | Embedding, Metrics, Modules, Skills, Webhooks, etc. |

---

## Directory Structure

```
docs/api/api-spec/uats/
├── schema/
│   └── uats.schema.json
├── specs/
│   └── *.uats.json          # 195 canonical spec files
├── drafts/
│   └── *.uats.json          # Draft specs (not run in CI)
├── runners/
│   └── uats_runner.py       # v1.2.0
├── HASH_VERIFICATION.md
└── README.md
```

---

## Makefile Targets

```makefile
# Run all API tests (with standard exclusions)
test-api:
 python3 docs/api/api-spec/uats/runners/uats_runner.py validate-all \
  --spec-dir docs/api/api-spec/uats/specs/ \
  --base-url http://localhost:9999 \
  --exclude-tag unts,llm_required,j17_disabled,jiminy_disabled,sidecar_required,constraint_scope_required \
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
- `config.sha256` — spec integrity hashes (see `HASH_VERIFICATION.md`)
- `variants[].variables`
- canonical body assertions using `{ "path", "op", "expected?" }`
- legacy matcher assertions (`equals`, `regex`, `type`, `exists`, etc.) for backward compatibility
- `--exclude-tag` for skipping specs by `config.tags` or variant-level `tags`

The runner fails fast when a spec uses schema features not yet implemented (`setup`, `teardown`, `chain`, `request.body_file`, `expected.body_file`, `expected.body_schema`, OAuth2 auth mode).

### Variant Merge Behavior

Variants are deep-merged with the base spec. Important: **if a variant overrides `expected` but omits `body_assertions`, the base spec's `body_assertions` are NOT inherited.** This prevents false failures when error variants (405, 400) have different response body shapes than the base 200 response. To explicitly inherit base assertions, include them in the variant's `expected.body_assertions`.

### Exclusion Tags

Tags in `config.tags` or `variants[].tags` can be excluded via `--exclude-tag`:

| Tag | Purpose |
|-----|---------|
| `unts` | Hash verification specs (require separate setup) |
| `llm_required` | Specs requiring live LLM API access |
| `j17_disabled` | J17 protocol disabled-state variants |
| `jiminy_disabled` | Jiminy disabled-state variants |
| `sidecar_required` | Neural sidecar specs (require separate sidecar service) |
| `constraint_scope_required` | Constraint scope PATCH (requires constraint features enabled) |

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
            --exclude-tag unts,llm_required,j17_disabled,jiminy_disabled,sidecar_required,constraint_scope_required \
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
| Scope | 27 languages | 195 API specs |
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

- **Canonical Specs:** 195
- **Total Variants:** 372 (including base + variant test cases)
- **Categories:** 25 (Health, Memory, Conversation CMS, Jiminy/J17, System, RSIC, Hash Verification, Constraints, Backup, Learning, Admin/Transfer, Scraper, Neural, Linear, Ingest, Filewatcher, Guardrail, Symbols, and more)
- **Most Complex:** ingest_codebase.uats.json (18 variants covering all config options)
- **Runner Version:** 1.2.0 (SHA256 integrity hashes, exclude-tag, variant assertion isolation)
