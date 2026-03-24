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

## GAP-29: Schema Dead Surface Area

The `uats.schema.json` defines features that are never used by any active spec and/or trigger hard `PARITY FAILURE` errors in the runner when present. This section documents the current state of each feature.

### Parity-Fail Features (cause hard runner errors)

These schema fields are detected by `_validate_supported_features()` in `uats_runner.py`. If a spec includes any of them, the runner emits `PARITY FAILURE: Runner does not implement these schema features: ...` and the spec is counted as an error, not a test failure.

| Schema Field | Specs Using It | Runner Behavior |
|---|---|---|
| `setup` | 0 | PARITY FAILURE — not implemented |
| `teardown` | 0 | PARITY FAILURE — not implemented |
| `chain` | 0 | PARITY FAILURE — not implemented |
| `request.body_file` | 0 | PARITY FAILURE — not implemented |
| `expected.body_file` | 0 | PARITY FAILURE — not implemented |
| `expected.body_schema` | 0 | PARITY FAILURE — not implemented |
| `config.retry_count` | 0 | PARITY FAILURE — not implemented |
| `config.retry_delay_ms` | 0 | PARITY FAILURE — not implemented |
| `config.strict_headers` | 0 | PARITY FAILURE — not implemented |
| `config.strict_body` | 0 | PARITY FAILURE — not implemented |
| `config.ignore_headers` | 0 | PARITY FAILURE — not implemented |
| `config.validate_schema` | 0 | PARITY FAILURE — not implemented |
| `config.validate_values` | 0 | PARITY FAILURE — not implemented |
| `auth.type == "oauth2"` | 0 | PARITY FAILURE — not implemented |
| `auth.api_key[in=query]` | 0 | PARITY FAILURE — not implemented |
| `captures.*.regex` | 0 | PARITY FAILURE — not implemented |
| `request.sha256` | 0 | PARITY FAILURE — not implemented (distinct from `config.sha256`, which is the spec integrity hash and IS supported) |
| `expected.response_time.p95_ms` | 0 | PARITY FAILURE — not implemented |
| `expected.response_time.p99_ms` | 0 | PARITY FAILURE — not implemented |
| `captures.*.from == "status"` | 0 | PARITY FAILURE — flagged even though the capture loop has partial handling for `status`; the parity guard runs first |
| `captures.*.from == "response_time"` | 0 | PARITY FAILURE — not implemented |

### Future/Aspirational Features (schema-valid, zero spec usage, no parity guard)

These fields are valid per the schema and do not trigger parity-fail errors (they are silently ignored or passed through). However, no active spec uses them, and their runner support ranges from partial to none.

| Schema Field | Specs Using It | Runner Support | Notes |
|---|---|---|---|
| `auth` (any type) | 0 | Partial | `bearer`, `basic`, `api_key[in=header]`, and `custom` are implemented in the HTTP client. `oauth2` triggers parity-fail. No spec currently uses any auth block. |
| `variables` (top-level) | 1 (`ingest_codebase.uats.json`) | Implemented | `env`, `generator` (uuid, timestamp, etc.) and literal values are resolved. Variant-level `variables` are also implemented. |
| `captures` | 1 (`ingest_codebase.uats.json`) | Partial | `from: body` and `from: header` are implemented. `from: status` and `from: response_time` trigger parity-fail. `captures.*.regex` triggers parity-fail. |
| `metadata.requires` | 0 | Not implemented | The `requires` array (ordering/dependency between specs) is parsed but never enforced by the runner. |
| `metadata.skip` / `metadata.skip_reason` | 0 | Implemented | Spec-level skip is handled; no active spec uses it at the metadata level (variants use `skip` directly). |
| `Matcher.not_in` (in `expected.headers`) | 0 | Not implemented | The `Matcher` type supports `not_in` in the schema but the runner's `_normalize_assertion` has no mapping for it; would produce `PARITY FAILURE: unsupported assertion operator 'not_in'` at runtime. |
| `Matcher.starts_with` (in `expected.headers`) | 0 | Not implemented | Same as `not_in` — no `_normalize_assertion` mapping; runtime PARITY FAILURE. |
| `Matcher.ends_with` (in `expected.headers`) | 0 | Not implemented | Same as `not_in` — no `_normalize_assertion` mapping; runtime PARITY FAILURE. |
| `expected.status` as range object `{"min": N, "max": N}` | 0 | Implemented | `check_status()` handles integer, array, and range object forms. No spec uses the range form. |
| `api.version` | Many | Decorative | Read by the runner for display only; has no effect on test logic. |
| `api.service` | Many | Decorative | Same as `api.version`. |
| `api.operation_id` | Many | Decorative | Same as `api.version`. |
| `api.tags` | Many | Decorative | Not the same as `config.tags`. `api.tags` are decorative; only `config.tags` and `variants[].tags` are used for `--exclude-tag` filtering. |
| `config.sequential` | 0 | Implemented | Sequential mode injects `prev_<field>` variables between variants; no active spec uses it. |
| `Step` (setup/teardown items): `type: sql` | N/A | Not implemented | `setup`/`teardown` are parity-fail blocked; `sql` step type would not work even if they were permitted. |
| `Step` (setup/teardown items): `type: command` | N/A | Not implemented | Same as `sql`. |

### Summary

The parity-fail guard in `_validate_supported_features()` protects against accidental use of 20 unimplemented fields. All 20 are currently at zero spec usage, so the guard is a safeguard for future spec authors rather than an active failure source. The three Matcher operators (`not_in`, `starts_with`, `ends_with`) are an additional gap: they are schema-valid and not pre-flight-checked, so a spec using them would produce a runtime PARITY FAILURE on the first affected assertion rather than a fast pre-flight error.

To extend runner support for any of these features, implement the corresponding logic in `docs/api/api-spec/uats/runners/uats_runner.py` and remove the field from the `_validate_supported_features` unsupported list.

---

## Stats

- **Canonical Specs:** 195
- **Total Variants:** 372 (including base + variant test cases)
- **Categories:** 25 (Health, Memory, Conversation CMS, Jiminy/J17, System, RSIC, Hash Verification, Constraints, Backup, Learning, Admin/Transfer, Scraper, Neural, Linear, Ingest, Filewatcher, Guardrail, Symbols, and more)
- **Most Complex:** ingest_codebase.uats.json (18 variants covering all config options)
- **Runner Version:** 1.2.0 (SHA256 integrity hashes, exclude-tag, variant assertion isolation)
