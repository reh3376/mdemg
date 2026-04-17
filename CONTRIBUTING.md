# Contributing to MDEMG

Thank you for your interest in contributing to MDEMG (Multi-Dimensional Emergent Memory Graph). This document provides guidelines for contributing to the project.

## Getting Started

### Prerequisites

- Go 1.24 or later
- Neo4j 5.x with vector index support
- An embedding provider (OpenAI API or Ollama)
- Python 3.10+ (for benchmark and parser test runners)

### Development Setup

1. Clone the repository:

   ```bash
   git clone https://github.com/reh3376/mdemg.git
   cd mdemg
   ```

2. Copy the example environment file:

   ```bash
   cp .env.example .env
   ```

3. Configure your `.env` file with:
   - Neo4j connection details
   - Embedding provider credentials

4. Start Neo4j (if using Docker):

   ```bash
   docker compose up -d
   ```

5. Build the unified CLI:

   ```bash
   make build-cli
   # Or directly:
   go build -o bin/mdemg ./cmd/mdemg
   ```

6. Run the server:

   ```bash
   ./bin/mdemg serve
   ```

## Code Style

- Follow standard Go conventions and idioms
- Run `go fmt` before committing
- Run `golangci-lint run` to catch lint issues (this runs in CI)
- Use meaningful variable and function names
- Add comments for exported functions and complex logic
- Keep functions focused and reasonably sized

## Testing

- Write tests for new functionality
- Run existing tests before submitting:

  ```bash
  # Unit tests
  go test ./internal/...

  # Integration tests (requires running Neo4j + MDEMG server)
  go test -tags=integration ./tests/integration/...

  # Parser validation — Go-native UPTS harness (no external deps)
  go test ./internal/languages/ -run TestUPTS -v

  # Parser validation — Python UPTS runner (requires bin/extract-symbols)
  make test-parsers

  # Validate a single language parser
  make test-parser-go
  make test-parser-python
  make test-parser-typescript

  # Go-native UPTS for a single language
  go test ./internal/languages/ -run TestUPTS/rust -v

  # API validation (UATS - requires running server)
  make test-api

  # RSIC-specific tests (requires running server + Neo4j)
  make test-rsic              # All RSIC tests (unit + integration + UATS)
  make test-rsic-unit         # RSIC unit tests only
  make test-rsic-integration  # RSIC integration tests (full stack, 6 tests)
  make test-rsic-uats         # RSIC contract tests (14 specs)

  # UNTS hash verification tests
  make test-unts              # UNTS unit tests (no server required)
  make test-unts-uats         # UNTS UATS contract tests (8 specs, requires UNTS_ENABLED=true)

  # Smoke tests (health + readiness only)
  make test-smoke

  # All tests (UPTS + UATS)
  make test-all
  ```

- Include both unit tests and integration tests where appropriate

### Test Frameworks

All test frameworks in this project follow the **UxTS (Universal-x Test Specification)** methodology — declarative JSON specs validated by executable runners governed by explicit schemas. For the comprehensive guide covering architecture, spec writing, CI integration, governance, and all 15 frameworks, see **[docs/guides/UXTS_DEVELOPER_GUIDE.md](docs/guides/UXTS_DEVELOPER_GUIDE.md)**.

**UPTS (Universal Parser Test Specification)** validates 27 language parsers against JSON spec files with SHA256 fixture verification. There are two runners:

1. **Go-native test harness** (`internal/languages/upts_test.go`): Loads UPTS specs, parses fixtures through the actual Go parser, and validates output against expected symbols. No external dependencies — runs via standard `go test`. This is the primary validation method.

2. **Python runner** (`docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py`): Validates via the `bin/extract-symbols` CLI binary. Useful for cross-validation and CI.

Specs live in `docs/lang-parser/lang-parse-spec/upts/specs/`. Fixtures live in `docs/lang-parser/lang-parse-spec/upts/fixtures/`.

### Adding or Updating a Parser Test

1. Create or edit `docs/lang-parser/lang-parse-spec/upts/specs/<language>.upts.json`
2. Create or edit the fixture file in `docs/lang-parser/lang-parse-spec/upts/fixtures/`
3. Run the Go-native harness: `go test ./internal/languages/ -run TestUPTS/<language> -v`
4. Optionally cross-validate with the Python runner: `make test-parser-<language>`

### Parser Development Workflow

When adding a new language parser or modifying an existing one:

1. **Create the parser** in `internal/languages/<language>_parser.go` (see that directory's README for the interface)
2. **Create a fixture** — a representative source file covering all symbol types the parser should extract
3. **Create a UPTS spec** — JSON file declaring expected symbols with name, type, line number, and export status
4. **Register in the ingester** — add the language to `getEnabledLanguages()` in **both** `internal/cli/ingest.go` (the shipped `mdemg` binary) and `cmd/ingest-codebase/main.go` (standalone dev tool). The `internal/cli/ingest.go` registration is what matters for released builds. Without this, files are silently skipped during ingestion.
5. **Validate** — run `go test ./internal/languages/ -run TestUPTS/<language> -v` until all assertions pass
6. **Key spec fields:**
   - `line_tolerance`: how far actual line can be from expected (default ±2)
   - `optional`: set `true` on symbols that may or may not be emitted (e.g., duplicate declarations)
   - `pattern`: document which extraction pattern this symbol tests (e.g., `P2_FUNCTION`)
   - See existing specs for examples of the full schema

**UATS (Universal API Test Specification)** validates API endpoints against JSON spec files. Specs live in `docs/api/api-spec/uats/specs/`. The `--base-url` flag in the Makefile dynamically reads the server's port from `.mdemg.port` (see Dynamic Port Allocation below). To add or update an API test:

1. Create or edit `docs/api/api-spec/uats/specs/<endpoint>.uats.json`
2. Run `make test-api-<endpoint>` to validate
3. Install UATS dependencies: `make uats-setup`

**UATS Tag Filtering** (Phase 90): Specs support optional tags in their `config.tags` array for selective execution:

```bash
# Run only RSIC specs
python3 runners/uats_runner.py validate-all --spec-dir specs/ --base-url ... --include-tag rsic

# Run all non-embedding specs (CI merge-gating)
python3 runners/uats_runner.py validate-all --spec-dir specs/ --base-url ... --exclude-tag embedding_required
```

Tags: `rsic` (14 RSIC/self-improve specs), `embedding_required` (4 embedding-dependent specs). **CI splits UATS into merge-gating (core) and best-effort (embedding).**

**UATS Sequential Mode**: Specs with `"sequential": true` in config run variants in order with previous response fields available as `prev_*` variables (used for idempotency testing).

**UBTS (Universal Benchmark Test Specification)** defines performance benchmarks with latency thresholds and throughput requirements. Specs live in `docs/tests/ubts/specs/`. Profiles (smoke, load, stress) control test intensity.

```bash
# Run a smoke benchmark
python docs/tests/ubts/runners/ubts_runner.py \
  --spec docs/tests/ubts/specs/retrieve_latency.ubts.json \
  --profile docs/tests/ubts/profiles/smoke.profile.json \
  --base-url http://localhost:9999

# Run all benchmarks with load profile
python docs/tests/ubts/runners/ubts_runner.py \
  --spec "docs/tests/ubts/specs/*.ubts.json" \
  --profile docs/tests/ubts/profiles/load.profile.json
```

Key threshold metrics: `p50_ms`, `p95_ms`, `p99_ms`, `max_ms`, `error_rate_pct`, `throughput_rps`.

**USTS (Universal Security Test Specification)** defines security tests mapped to OWASP Top 10 categories. Specs live in `docs/tests/usts/specs/`. Tests verify authentication, authorization, injection prevention, rate limiting, and data exposure.

```bash
# Run authentication tests
python docs/tests/usts/runners/usts_runner.py \
  --spec docs/tests/usts/specs/auth_required.usts.json \
  --base-url http://localhost:9999

# Run all security tests with API key
python docs/tests/usts/runners/usts_runner.py \
  --spec "docs/tests/usts/specs/*.usts.json" \
  --api-key "$MDEMG_API_KEY"
```

Severity levels: `critical`/`high` (exit 2), `medium`/`low` (exit 1). Custom injection payloads in `docs/tests/usts/payloads/`.

**UOBS (Universal Observability Specification)** validates metrics, health endpoints, tracing, and alerting. Specs live in `docs/tests/uobs/specs/`. Includes Prometheus alert rules and Grafana dashboard templates.

```bash
# Run metrics validation
python docs/tests/uobs/runners/uobs_runner.py \
  --spec docs/tests/uobs/specs/prometheus_metrics.uobs.json \
  --base-url http://localhost:9999

# Run all observability tests
python docs/tests/uobs/runners/uobs_runner.py \
  --spec "docs/tests/uobs/specs/*.uobs.json"
```

Required Prometheus metrics: `mdemg_http_requests_total`, `mdemg_http_request_duration_seconds`, `mdemg_retrieval_latency_seconds`, `mdemg_rate_limit_rejected_total`, `mdemg_circuit_breaker_state`, `mdemg_cache_hit_ratio`.

**UAMS (Universal Auth Method Specification)** defines authentication method contracts that authenticators implement. Specs live in `docs/tests/uams/specs/`. Current methods: `none`, `apikey`, `jwt`, `saml`.

```bash
# Validate UAMS specs against schema
npx ajv validate -s docs/tests/uams/schema/uams.schema.json \
  -d "docs/tests/uams/specs/*.uams.json"

# Run conformance tests
go test ./internal/auth/... -run TestUAMS -v
```

Specs define credential extraction sources, validation algorithms (timing-safe), principal construction, and error response contracts.

**UDTS (Universal DevSpace Test Specification)** validates gRPC services (Space Transfer, DevSpace Hub) against contract specs. Specs live in `docs/api/api-spec/udts/specs/`. Tests live in `tests/udts/`.

```bash
# Start the gRPC server
go run ./cmd/space-transfer/ serve -port 50051

# Run UDTS contract tests
export UDTS_TARGET=localhost:50051
go test ./tests/udts/... -v -count=1
```

Supports optional `proto_sha256` for contract stability verification.

**UVTS (Universal Validation Test Specification)** defines semantic accuracy validation benchmarks for MDEMG retrieval quality. Specs live in `docs/tests/uvts/specs/`. Tests measure mean score, evidence quality, and category-specific performance.

```bash
# Run validation benchmark
python docs/tests/uvts/runners/uvts_runner.py \
  --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json \
  --base-url http://localhost:9999
```

Thresholds: `mean_score`, `strong_evidence_pct`, `high_score_rate_pct`, `min_category_score`. Profiles: `quick` (16q), `standard` (40q), `full` (120q).

**UNTS (Universal Hash Test Specification)** maintains a current and historical record of SHA-256 hash verification for all framework-protected files across UPTS, UATS, UBTS, USTS, UOBS, UAMS, and UDTS. Registry lives in `docs/specs/unts-registry.json`. Implementation is in `internal/unts/` with gRPC service defined in `api/proto/unts.proto` and REST handlers in `internal/api/handlers_unts.go`.

```bash
# Run UNTS Go unit tests
go test ./internal/unts/... -v

# Run UNTS UATS contract tests (requires running server with UNTS_ENABLED=true)
make test-unts-uats

# Run all UNTS tests (unit only, no server required)
make test-unts

# UDTS contract tests for UNTS gRPC service
export UDTS_TARGET=localhost:50051
go test ./tests/udts/... -run TestUNTS -v
```

**REST Endpoints (8):** `POST /v1/hash-verification/register`, `GET /v1/hash-verification/files`, `GET /v1/hash-verification/files/{path}`, `POST /v1/hash-verification/verify`, `POST /v1/hash-verification/verify-all`, `POST /v1/hash-verification/update`, `POST /v1/hash-verification/revert`, `POST /v1/hash-verification/scan`. Config: `UNTS_ENABLED` (default: false), `UNTS_BASE_PATH` (default: "."). Returns 503 when disabled.

**gRPC RPCs (7):** `ListVerifiedFiles`, `GetFileStatus`, `GetHashHistory`, `VerifyNow`, `RevertToPreviousHash`, `UpdateHash`, `RegisterTrackedFile`. Each tracked file retains up to 3 historical hash values for revert. See `docs/specs/unts-hash-verification.md` for the full specification and `docs/specs/FRAMEWORK_GOVERNANCE.md` for governance context.

### Universal Test Specification Summary

| Framework | Purpose | Location |
|-----------|---------|----------|
| UPTS | Parser validation | `docs/lang-parser/lang-parse-spec/upts/` |
| UATS | HTTP API contracts | `docs/api/api-spec/uats/` |
| UBTS | Performance benchmarks | `docs/tests/ubts/` |
| USTS | Security tests (OWASP) | `docs/tests/usts/` |
| UOBS | Observability validation | `docs/tests/uobs/` |
| UAMS | Auth method contracts | `docs/tests/uams/` |
| UDTS | gRPC contract tests | `docs/api/api-spec/udts/`, `tests/udts/` |
| UVTS | Semantic accuracy | `docs/tests/uvts/` |
| UNTS | Hash verification | `internal/unts/`, `docs/specs/` |
| UETS | Emergence evaluation | `docs/tests/uets/` |
| UITS | Iterative-improvement | `docs/tests/uits/` |

### Dynamic Port Allocation

The MDEMG server uses dynamic port allocation. When started, it writes the actual bound port to `.mdemg.port`. All test commands (`make test-api`, `make test-smoke`) read this file automatically. If the file doesn't exist, port `9999` is used as the fallback default.

## Claude Code Hooks

Five hooks in `.claude/hooks/` are **tracked in the repository** (explicitly un-ignored in `.gitignore`). These are project-level infrastructure — CMS integration, destructive command guards, and context management — shared across all developers. Do not modify them without a PR.

The rest of `.claude/` (settings, memory, etc.) remains gitignored and developer-specific.

### Available Hooks

| Hook | Trigger | Purpose |
|------|---------|---------|
| `post-tool-observe.py` | After tool use | Auto-captures CMS observations (build results, errors, config changes); triggers codebase re-ingest on `git push`; detects degraded memory state in API responses |
| `pre-bash-check.py` | Before Bash | Blocks destructive commands (reset-db, rm -rf, force push, DROP TABLE) |
| `session-start.sh` | Session start | Restores CMS context via `/v1/conversation/resume`; detects 0-observation anomaly with CRITICAL warning; auto-triggers RSIC micro assessment; displays RSIC health summary |
| `prompt-context.sh` | Before prompt | Injects CMS recall context; warns on empty recall for non-trivial queries; displays session health ribbon |
| `pre-compact.sh` | Before compaction | Saves context snapshot to CMS before auto-compaction; includes session health in snapshot |

### Re-Ingest on Push

The `post-tool-observe.py` hook detects `git push` commands targeting the mdemg repo and automatically triggers an incremental codebase re-ingest + consolidation via `bin/ingest-codebase`. This keeps the `mdemg-dev` CMS space in sync with the latest code.

If this hook is missing or the binary is not built, re-ingest must be triggered manually:

```bash
# Manual re-ingest
curl -X POST http://localhost:9999/v1/memory/ingest/trigger \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","path":"/Users/reh3376/mdemg","incremental":true}'

# Check status
curl http://localhost:9999/v1/memory/ingest/status/<job_id>
```

### Hook Registration

The 5 tracked hooks are automatically available when you clone the repo. Hook registration in `.claude/settings.local.json` (developer-specific, gitignored) can be generated with:

```bash
# Automatic (recommended) — registers hooks in settings
mdemg sidecar generate-hooks

# Or one-command full setup (includes hooks + MCP + services)
mdemg sidecar quickstart

# Alternative: install from embedded templates
mdemg hooks install --type claude
```

See `CLAUDE.md` for the full hook specification.

## Branch Naming Convention

All collaborator branches must follow the pattern: `<github_handle>_dev<01-09>` with an optional descriptive suffix `_<kebab-description>`.

**Regex:** `^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?_dev0[1-9](_[a-z0-9]+(-[a-z0-9]+)*)?$`

### Category Codes

| Code | Category | Description |
|------|----------|-------------|
| `01` | General development | Broad or mixed-scope work spanning multiple concerns |
| `02` | Feature development | Single feature or capability addition |
| `03` | Documentation & linting | Docs, comments, lint fixes, formatting |
| `04` | Bug fixes | Correcting broken behavior |
| `05` | CI/CD & infrastructure | Build pipeline, GitHub Actions, goreleaser, Docker |
| `06` | Testing | Test additions, test framework work, benchmarks |
| `07` | Refactoring & tech debt | Structural improvements without behavior change |
| `08` | Release & packaging | Version bumps, Homebrew formula, installers, changelogs |
| `09` | Experimental & prototype | Exploratory work not yet committed to a roadmap |

### Examples

**Valid:**
- `reh3376_dev01` — general development
- `reh3376_dev02_teardown-wizard` — feature branch with description
- `jdoe_dev04_fix-neo4j-timeout` — bug fix with description
- `alice_dev09` — experimental work

**Invalid:**
- `feature/my-feature` — does not follow the naming pattern
- `dev01` — missing github handle
- `reh3376_dev10` — code must be 01-09
- `reh3376_dev1` — code must be two digits (01, not 1)

### Rules

- **One branch per category per collaborator** — merge or close your existing branch in a category before starting another in the same category
- **`main` is always protected** — changes reach it only via PR
- **Automation exemptions:** `dependabot/*`, `deps/*`, `release/*`, and `gh-pages` branches are exempt from this naming convention
- A GitHub Actions workflow (`.github/workflows/branch-naming.yml`) enforces this convention on push

> **Migration note:** The original development branch `mdemg-dev01` has been renamed to `reh3376_dev01` to comply with this convention.

## Submitting Changes

### Pull Request Process

1. Fork the repository and create a branch following the [naming convention](#branch-naming-convention):

   ```bash
   git checkout -b yourgithubhandle_dev02_your-feature
   ```

2. Make your changes following the code style guidelines

3. Write or update tests as needed

4. Commit your changes with clear, descriptive messages:

   ```bash
   git commit -m "feat: add new retrieval optimization"
   ```

   Use conventional commit prefixes:
   - `feat:` - New features
   - `fix:` - Bug fixes
   - `docs:` - Documentation changes
   - `test:` - Test additions or changes
   - `refactor:` - Code refactoring
   - `perf:` - Performance improvements

5. Push to your fork and open a Pull Request

6. Ensure CI checks pass and address any review feedback

### What to Include in a PR

- Clear description of the changes
- Motivation/context for the changes
- Any breaking changes noted
- Test plan or evidence of testing

## Project Structure

```
mdemg/
├── cmd/                  # CLI entry points (server, ingest-codebase, mcp-server, etc.)
├── internal/             # Internal packages
│   ├── api/              # HTTP API handlers
│   ├── retrieval/        # Core retrieval algorithms
│   ├── hidden/           # Hidden layer and concept abstraction
│   ├── learning/         # Hebbian learning edges
│   ├── embeddings/       # Embedding providers (OpenAI, Ollama)
│   ├── conversation/     # Conversation Memory System (CMS) - templates, snapshots, relevance
│   ├── symbols/          # Code symbol extraction
│   ├── optimistic/       # Optimistic locking with retry
│   ├── backup/           # Backup & restore (full, partial, scheduler, retention)
│   ├── backpressure/     # Memory pressure monitoring
│   └── plugins/          # Plugin system (scaffold, validate)
├── migrations/           # Neo4j schema migrations (Cypher)
├── tests/                # Integration tests
├── scripts/              # Utility scripts
├── docs/                 # Documentation and benchmarks
└── plugins/              # Plugin modules
```

## Pipeline Registry Pattern

MDEMG uses a **Dynamic Pipeline Registry** for consolidation node creation. Instead of adding new node types across four files, each step is a self-contained adapter implementing the `NodeCreator` interface and registered in a single pipeline. There are currently **9 registered steps** across 4 phase levels.

**Why:** Before Phase 46, every new consolidation step (concern, config, comparison, temporal, UI, constraint) required parallel edits to `service.go`, `handlers.go`, `types.go`, and `models.go`. This violated Open/Closed Principle and caused duplicate logic between the handler and service. The pipeline eliminates this — adding a new node type is now a two-file operation (create the step adapter, register it in `buildPipeline()`).

**How:** Each step implements `Name()`, `Phase()`, `Required()`, and `Run()`. The pipeline executes steps in phase order, aggregates results into a universal `StepResult` map, and handles required vs. optional error semantics. The API response includes both a dynamic `steps` map and backward-compatible flat fields.

**Split Execution (Phase 75C):** The pipeline supports `RunPhaseRange()` for selective phase execution. Pre-clustering steps (phases 10-20) run before multi-layer clustering. Post-clustering steps (phases 25-30: `dynamic_edges`, `emergent_l5`) run after clustering completes, ensuring they have access to fully clustered graph state.

Full details, code examples, and the guide for adding new steps: **[docs/development/REGISTRY.md](docs/development/REGISTRY.md)**

## API Endpoints

Full API specs are in `docs/api/api-spec/uats/specs/` (one `.uats.json` per endpoint). Below is the complete endpoint list registered in `internal/api/server.go`:

### Health & Readiness

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/healthz` | Liveness check |
| GET | `/readyz` | Readiness check (includes embedding provider status) |
| GET | `/v1/embedding/health` | Embedding provider health with active probe |

### Memory Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/memory/retrieve` | Semantic retrieval with graph expansion |
| POST | `/v1/memory/ingest` | Ingest a single observation |
| POST | `/v1/memory/ingest/batch` | Batch ingest observations |
| POST | `/v1/memory/reflect` | Deep reflection on a topic |
| GET | `/v1/memory/stats` | Memory graph statistics |
| POST | `/v1/memory/consolidate` | Trigger hidden layer consolidation |
| POST | `/v1/memory/archive/bulk` | Bulk archive nodes |
| * | `/v1/memory/nodes/{id}` | Node CRUD operations (archive, unarchive, delete) |
| GET | `/v1/memory/symbols` | Search code symbols |
| GET | `/v1/memory/distribution` | Score distribution and learning phase stats |
| GET | `/v1/memory/cache/stats` | Embedding/query cache stats |
| DELETE | `/v1/memory/cache` | Clear query cache |
| GET | `/v1/memory/query/metrics` | Query performance metrics |
| POST | `/v1/memory/consult` | SME-style Q&A |
| POST | `/v1/memory/suggest` | Suggest related concepts |

### Codebase Ingestion (Background Jobs)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/memory/ingest/trigger` | Start background ingestion job |
| GET | `/v1/memory/ingest/status/{id}` | Check job progress |
| POST | `/v1/memory/ingest/cancel/{id}` | Cancel running job |
| GET | `/v1/memory/ingest/jobs` | List all ingestion jobs |
| POST | `/v1/memory/ingest/files` | Ingest specific files with background job |
| * | `/v1/memory/ingest-codebase` | Codebase ingestion route (deprecated) |

### Freshness & Sync

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/memory/spaces/{id}/freshness` | Space freshness and staleness status |
| GET | `/v1/memory/freshness` | Batch freshness check across spaces |

### Learning & Hebbian Edges

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/learning/prune` | Prune low-weight edges |
| GET | `/v1/learning/stats` | Learning edge statistics |
| POST | `/v1/learning/freeze` | Freeze learning for stable scoring |
| POST | `/v1/learning/unfreeze` | Unfreeze learning |
| GET | `/v1/learning/freeze/status` | Check freeze status |

### Conversation Memory System (CMS)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/conversation/observe` | Store observation |
| POST | `/v1/conversation/correct` | Store correction |
| POST | `/v1/conversation/resume` | Resume session (restore context) |
| POST | `/v1/conversation/recall` | Recall conversation memory |
| POST | `/v1/conversation/consolidate` | Consolidate conversation themes |
| GET | `/v1/conversation/volatile/stats` | Volatile memory stats |
| POST | `/v1/conversation/graduate` | Graduate volatile to permanent |
| GET | `/v1/conversation/session/health` | Session health score |
| GET | `/v1/conversation/session/anomalies` | Aggregated session anomalies and health (Phase 80) |

### CMS Templates (Phase 60)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET/POST | `/v1/conversation/templates` | List or create observation templates |
| GET/PUT/DELETE | `/v1/conversation/templates/{id}` | Template CRUD operations |

### CMS Snapshots (Phase 60)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET/POST | `/v1/conversation/snapshot` | List or create session snapshots |
| GET | `/v1/conversation/snapshot/latest` | Get latest snapshot for session |
| POST | `/v1/conversation/snapshot/cleanup` | Clean up old snapshots |
| GET/DELETE | `/v1/conversation/snapshot/{id}` | Get or delete snapshot |

### CMS Org Reviews (Phase 60)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/conversation/org-reviews` | List pending org reviews |
| GET | `/v1/conversation/org-reviews/stats` | Org review statistics |
| POST | `/v1/conversation/org-reviews/{id}/decision` | Approve or reject observation |
| POST | `/v1/conversation/observations/{id}/flag-org` | Flag observation for org review |

### Self-Improvement Cycle — RSIC (Phase 60b)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/self-improve/assess` | Trigger on-demand self-assessment |
| GET | `/v1/self-improve/report` | Get active task report |
| GET | `/v1/self-improve/report/{cycle_id}` | Get specific cycle report |
| POST | `/v1/self-improve/cycle` | Trigger full RSIC cycle (assess→validate) |
| GET | `/v1/self-improve/history` | Cycle history with outcomes |
| GET | `/v1/self-improve/calibration` | Calibration metrics and confidence scores |
| GET | `/v1/self-improve/health` | Watchdog status and health score |
| GET | `/v1/self-improve/signals` | Signal emission/response effectiveness stats (Phase 80) |

### Skill Registry (Phase 48)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/skills?space_id=X` | List registered skills (discovered from pinned observations) |
| POST | `/v1/skills/{name}/recall` | Recall skill content by tag |
| POST | `/v1/skills/{name}/register` | Register skill sections as pinned observations |

### Cleanup & Edge Consistency

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/memory/cleanup/orphans` | Detect/archive/delete orphaned nodes |
| POST | `/v1/memory/cleanup/graph-orphans` | Cross-space zero-edge orphan scan/fix (scan, consolidate, archive, delete) |
| POST | `/v1/memory/cleanup/schedule` | Schedule automated cleanup |
| GET | `/v1/memory/cleanup/schedules` | List cleanup schedules |
| GET | `/v1/memory/cleanup/stats` | Cleanup statistics |
| GET | `/v1/memory/edges/stale/stats` | Get stale edge statistics |
| POST | `/v1/memory/edges/stale/refresh` | Trigger stale edge refresh |

### Webhooks

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/webhooks/linear` | Linear webhook receiver (HMAC-SHA256 verified) |
| POST | `/v1/webhooks/{source}` | Generic webhook receiver |

### Linear Integration (Phase 44)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET/POST | `/v1/linear/issues` | List or create Linear issues |
| GET/PUT/DELETE | `/v1/linear/issues/{id}` | Read, update, or delete issue |
| GET/POST | `/v1/linear/projects` | List or create Linear projects |
| GET/PUT | `/v1/linear/projects/{id}` | Read or update project |
| POST | `/v1/linear/comments` | Create comment on issue |

### Backup & Restore (Phase 70)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/backup/trigger` | Trigger backup (full or partial_space) |
| GET | `/v1/backup/status/{id}` | Backup job status and progress |
| GET | `/v1/backup/list` | List all backups (optional `?type=` filter) |
| GET | `/v1/backup/manifest/{id}` | Get full backup manifest |
| DELETE | `/v1/backup/{id}` | Delete a backup artifact |
| POST | `/v1/backup/restore` | Trigger restore from full backup |
| GET | `/v1/backup/restore/status/{id}` | Restore job status |

### Neo4j State Monitor (Phase 76)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/neo4j/overview` | Consolidated database health, space summaries, and backup status |

### Symbol Relationships (Phase 75)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/symbols/relationships?space_id=X` | Relationship edge counts by type |
| GET | `/v1/symbols/{id}/relationships?space_id=X` | Incoming/outgoing edges for a symbol |

### Hash Verification — UNTS (Phase 38)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/hash-verification/register` | Register a file for hash tracking |
| GET | `/v1/hash-verification/files` | List tracked files (optional `?framework=`/`?status=` filters) |
| GET | `/v1/hash-verification/files/{path}` | Get single file status and history |
| POST | `/v1/hash-verification/verify` | Verify single file hash against expected |
| POST | `/v1/hash-verification/verify-all` | Verify all tracked files |
| POST | `/v1/hash-verification/update` | Update expected hash for a file |
| POST | `/v1/hash-verification/revert` | Revert to a previous hash from history |
| POST | `/v1/hash-verification/scan` | Scan manifest + specs to auto-register files |

Config: `UNTS_ENABLED` (default: false), `UNTS_BASE_PATH` (default: "."). Returns 503 when disabled.

### Admin — Space Lifecycle Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/admin/spaces` | List all spaces with metadata and prunable status |
| PATCH | `/v1/admin/spaces/{id}` | Update space metadata (prunable flag) |
| POST | `/v1/admin/spaces/prune` | Batch prune prunable/orphan spaces (dry-run supported) |

Auto-prune scheduler runs in the background on a configurable interval (`SPACE_PRUNE_INTERVAL_HOURS`, default 24, 0 = disabled).

### System, Plugins & Monitoring

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/metrics` | Prometheus-style metrics |
| GET | `/v1/metrics/snapshot` | JSON metrics snapshot endpoint |
| GET | `/v1/modules` | List modules |
| POST | `/v1/modules/{id}` | Module sync |
| GET/POST/PUT/DELETE | `/v1/plugins` | Plugin operations |
| POST | `/v1/plugins/create` | Create plugin from spec |
| GET | `/v1/ape/status` | APE scheduler status |
| POST | `/v1/ape/trigger` | Trigger APE action |
| POST | `/v1/feedback` | Submit feedback |
| GET | `/v1/system/capability-gaps` | List capability gaps |
| * | `/v1/system/capability-gaps/{id}` | Capability gap operations |
| GET | `/v1/system/gap-interviews` | Gap interview sessions |
| * | `/v1/system/gap-interviews/{id}` | Gap interview operations |
| GET | `/v1/system/pool-metrics` | Neo4j connection pool metrics |
| GET | `/v1/jobs/{id}/stream` | SSE streaming for job progress |

### Jiminy Guidance

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/jiminy/guide` | Get proactive guidance for current context |
| POST | `/v1/jiminy/feedback` | Record guidance effectiveness (followed/ignored/contradicted) |
| POST | `/v1/jiminy/evaluate` | Evaluate agent output against constraints |

**Request body:**

```json
{
  "space_id": "my-project",
  "context": "Refactoring authentication middleware",
  "file_path": "internal/auth/middleware.go",
  "session_id": "session-1",
  "max_items": 10
}
```

Required fields: `space_id`, `context`. Optional: `file_path`, `agent_output`, `query`, `session_id`, `max_items`.

**Response:** Returns `guidance[]` items (type, priority, content, confidence, source_nodes, constraint_code), `prompt_augmentation` (injectable text block), `confidence`, `rationale`, `source_counts`, `protocol` (J17 version, trust_score, seq), and `guidance_id` (CUID2 identifier for effectiveness tracking). Returns 503 when `JIMINY_ENABLED=false`.

### J17 AI-to-AI Protocol (Phase J17)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/jiminy/bootstrap` | J17 bootstrap payload for new agent sessions |
| POST | `/v1/jiminy/protocol/feedback` | Submit comprehension feedback trials for protocol evolution |
| POST | `/v1/jiminy/protocol/learn` | Request constraint code re-generation (when existing code is ambiguous) |
| GET | `/v1/jiminy/protocol/metrics` | Protocol performance metrics (tier distribution, comprehension, compression) |

Requires `J17_ENABLED=true`. Protocol endpoints return 503 when disabled. See `docs/features/j17-ai2ai-protocol.md` for the full protocol specification.

## Reporting Issues

Use the [bug report](https://github.com/reh3376/mdemg/issues/new?template=bug_report.yml) or [feature request](https://github.com/reh3376/mdemg/issues/new?template=feature_request.yml) templates when opening issues.

## Code of Conduct

This project follows a Code of Conduct to ensure a welcoming and respectful environment for everyone. Please read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before participating.

## License

By contributing to MDEMG, you agree that your contributions will be licensed under the MIT License.
