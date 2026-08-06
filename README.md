# MDEMG - Multi-Dimensional Emergent Memory Graph

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](https://golang.org/)
[![Neo4j](https://img.shields.io/badge/Neo4j-5.x-008CC1.svg)](https://neo4j.com/)
[![CI](https://github.com/reh3376/mdemg/actions/workflows/ci.yml/badge.svg)](https://github.com/reh3376/mdemg/actions/workflows/ci.yml)

A persistent memory system for AI agents built on Neo4j with native vector indexes. Implements semantic retrieval with hidden layer concept abstraction, recursive self-improvement and Hebbian learning.

> **Key insight**: The critical metric isn't average retrieval score—it's **state survival under context compaction**. Baseline agents forget architectural decisions after compactions. MDEMG maintains decision persistence indefinitely.

---

## 🧪 Beta Testers — Start Here

**Current beta**: `v0.11.0-beta.1` (2026-08-06). Thank you for testing.

- **Beta test plan** (69 tests across 7 tiers): [`packaging/homebrew-mdemg/mdemg_beta_testing.md`](packaging/homebrew-mdemg/mdemg_beta_testing.md)
- **Report an issue**: https://github.com/reh3376/mdemg/issues/new/choose
- **First-run gotcha**: on macOS, run `brew trust reh3376/mdemg` BEFORE `brew install` — Homebrew's default-blocks-untrusted-taps policy will otherwise fail with a Sorbet stack trace
- **You do NOT need an OpenAI or Ollama key to start** — `mdemg init --defaults` falls back to disabled mode; you can write observations, open the dashboard, and inspect data without any external provider

---

## Quick Start

### Prerequisites

- **Docker Desktop** (macOS/Windows) or **Docker Engine** (Linux) with Compose v2, running before `mdemg init`
- **Embedding provider** (OPTIONAL — MDEMG runs without one in disabled mode):
  - [OpenAI API key](https://platform.openai.com/api-keys) — simplest path to full features (1 env var)
  - [Ollama](https://ollama.com) — local & free, requires ~5 GB model download

### Step 1: Install

```bash
# macOS (Homebrew)
brew trust reh3376/mdemg    # REQUIRED before first install (Homebrew untrusted-tap policy)
brew tap reh3376/mdemg
brew install mdemg

# Linux / Windows (WSL2)
curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash

# Verify installation — expect v0.11.0-beta.1 for the current beta
mdemg version
```

> **Windows**: Install [WSL2](https://learn.microsoft.com/en-us/windows/wsl/install) first, then run all commands inside WSL2.

<details>
<summary>Build from source (alternative)</summary>

```bash
git clone https://github.com/reh3376/mdemg.git && cd mdemg
go build -o bin/mdemg ./cmd/mdemg
./bin/mdemg version
```

</details>

### Step 2: Initialize and Start (Docker Compose)

```bash
cd /path/to/your/project
mdemg init --defaults       # Recommended — non-interactive, works with or without OPENAI_API_KEY
# OR
mdemg init                  # Interactive wizard — prompts for OpenAI/Ollama/Jiminy choices
```

This will:
1. Check Docker is available and has adequate resources
2. Scan for **6 free TCP ports** (all dynamically assigned — no hardcoded defaults)
3. Generate `.env` with port assignments, credentials, and auto-seed `RSIC_PROTECTED_SPACES` with your new space
4. Create `.mdemg/config.yaml` and `.mdemgignore`
5. Run `docker compose up -d` (starts all 5 services)
6. Wait for the MDEMG server health check

> **Without `OPENAI_API_KEY` set**, `mdemg init --defaults` produces a **disabled-mode** config: ingest, dashboard, BM25 retrieval, observations all work; LLM synthesis + Jiminy guidance + vector retrieval need an operator opt-in. The init output prints exactly 2 next-step paths (OpenAI or Ollama) so you can enable the full feature set when ready.

Sanity-check the setup:

```bash
mdemg config validate
# Expected: "Validation: PASSED" or "Validation: PASSED (services not started — run: docker compose up -d)"
# Both exit 0. FAILED with exit 1 = real config problem worth reporting.
```

### Open the Dashboard

Once the stack is running, open the built-in browser dashboard. Find your port in `.env` (`MDEMG_PORT=`) or via `mdemg status`:

```
http://localhost:${MDEMG_PORT}/ui/
```

### Step 3: Write your first observation

The smallest possible success signal — works in disabled mode, no embedder required:

```bash
port=$(grep '^MDEMG_PORT' .env | cut -d= -f2)
curl -sS -X POST "http://localhost:${port}/v1/conversation/observe" \
  -H "Content-Type: application/json" \
  -d '{"space_id":"my-first-space","session_id":"first","content":"MDEMG is running","obs_type":"note"}'
# Expected: {"obs_id":"...","node_id":"...","surprise_score":0,...}
```

Then check the dashboard's Memory tab — your observation should appear.

<details>
<summary>Step 4 (optional): Pull the local LLM for full Jiminy + retrieval features</summary>

For the local-LLM path (`mdemg-llm-v1`, Phase 5 Qwen3-14B fine-tune served via `llama.cpp llama-server`):

```bash
brew install ollama          # one-time — Ollama is the distribution channel
mdemg model pull             # RAM-auto picks Q4_K_M / Q5_K_M / Q8_0 quant
# → prints the MDEMG_MODEL_PATH line to add to .env
```

Three quants serve three RAM tiers: Q4_K_M (~9 GB, 16 GB Macs), Q5_K_M (~11 GB, 24 GB — production canonical), Q8_0 (~16 GB, 32+ GB). See [`docs/features/local-model-distribution.md`](docs/features/local-model-distribution.md) for the full Configurability Contract.

Skip this step if you're using OpenAI for inference or running in disabled mode.

</details>

The dashboard has 11 tabs: Status (health + Grafana links + restart), Memory (layer breakdown + export/import), Learning (Hebbian stats + freeze/prune), Config (editable with Save All), Logs (searchable viewer), RSIC (service controls + trigger), Plugins (lifecycle management), Features (service status), Backup (backup + restore), Training Data (fine-tuning data pipeline), and Review (HITL dataset grading). Links to 8 Grafana dashboards for detailed metrics.

### Step 3: Ingest Your Codebase

```bash
mdemg ingest --path .       # Index your codebase into the memory graph
mdemg hooks install         # Auto-ingest on every git commit (optional)
```

> **Performance tip:** Use `--speed fast` for large codebases, `--speed thorough` for pre-benchmark runs. See [Ingest Performance Guide](docs/guides/ingest-performance.md) for details.

That's it. Your AI agent now has persistent memory.

### One-Command Setup (Sidecar)

For projects using Claude Code, the sidecar quickstart handles everything — config, services, MCP integration, and hooks — in a single command:

```bash
cd /path/to/your/project
mdemg sidecar quickstart    # init + install + up + attach-agent + generate-hooks
```

This creates `.mdemg/sidecar.yaml`, starts Neo4j + MDEMG server, writes `.claude/mcp.json`, generates `session-start.sh` and `prompt-context.sh` hooks, registers them in `settings.local.json`, and enables `enableAllProjectMcpServers`.

See the [Quickstart Guide](docs/quickstart.md) for a detailed walkthrough, or run `mdemg demo` to try it with sample data.

> **Upgrading?** Run `mdemg upgrade` to self-update to the latest release.

---

## Reproduce the Benchmark

Everything needed to independently verify our results is included.

### Prerequisites

- Docker (Neo4j)
- Go 1.24+
- Python 3.10+ (grading + utilities)

**Embeddings (choose one):**

- OpenAI API key, or
- Ollama (local embeddings)

**Agent under test (choose one):**

- Claude Code (recommended baseline runner), or
- Any tool-using LLM agent that can call the MDEMG API

### Reproduction Steps

```bash
# 1. Clone and setup
git clone https://github.com/reh3376/mdemg.git && cd mdemg
cp .env.example .env  # Add your embedding provider credentials

# 2. Build and start services
go build -o bin/mdemg ./cmd/mdemg
./bin/mdemg db start              # Launch Neo4j container
./bin/mdemg start --auto-migrate  # Start server daemon with migrations

# 3. Ingest test codebase (or use your own)
./bin/mdemg ingest --space-id=benchmark --path=/path/to/target-repo

# 4. Run consolidation
curl -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" -d '{"space_id": "benchmark"}'

# 5. Run benchmark
# Questions: docs/architecture/benchmarks/whk-wms/test_questions_120_agent.json
# Grader: docs/architecture/benchmarks/grader_v4.py
```

**Report output**: `grades_*.json` contains per-question scores with evidence breakdown.

### Verify Integrity

```bash
# Question bank hash (SHA-256)
shasum -a 256 docs/architecture/benchmarks/whk-wms/test_questions_120_agent.json
# Expected: 24aa17a215e4e58b8b44c7faef9f14228edb0e6d3f8f657d867b1bfa850f7e9e
```

---

## Benchmark Receipts

Full reproducibility details for skeptics:

| Item | Value |
|------|-------|
| **MDEMG Commit** | `779d753` |
| **Question Set** | `test_questions_120_agent.json` (120 questions) |
| **Question Hash** | `sha256:24aa17a2...` |
| **Answer Key** | `test_questions_120.json` |
| **Grader** | `docs/architecture/benchmarks/grader_v4.py` |
| **Scoring Weights** | Evidence: 0.70 / Concept: 0.15 / Semantic: 0.15 |
| **Target Codebase** | whk-wms (507K LOC TypeScript) |
| **Include Patterns** | `**/*.ts`, `**/*.tsx`, `**/*.json` |
| **Exclude Patterns** | `node_modules/`, `dist/`, `.git/`, `docs-website/` |
| **Agent Model** | Claude Haiku (via Claude Code) |
| **Runs** | 2 per condition (run 3 excluded for consistency) |
| **Embedding Provider** | OpenAI text-embedding-3-small |

> **Baseline definition:** Same agent runner and tool permissions, **no MDEMG retrieval**, relying on long-context + auto-compaction only (memory off).

### Key Results (2026-01-30, whk-wms 507K LOC)

| Metric | Baseline | MDEMG + Edge Attention | Delta |
|--------|----------|------------------------|-------|
| **Mean Score** | 0.854 | **0.898** | **+5.2%** |
| Standard Deviation | 0.088 | 0.059 | -51% variance |
| High Score Rate (≥0.7) | 97.9% | **100%** | +2.1pp |
| Strong Evidence Rate | 97.9% | **100%** | +2.1pp |

**Category Performance (Edge Attention):**

| Category | Mean Score |
|----------|------------|
| Disambiguation | 0.958 |
| Service Relationships | 0.916 |
| Architecture Structure | 0.889 |
| Data Flow Integration | 0.882 |

### Key Metric: State Survival

The Q&A battery measures single-turn retrieval accuracy. The critical differentiator is **state survival under compaction**:

| Metric | Baseline | MDEMG | Source |
|--------|----------|-------|--------|
| Decision Persistence @5 compactions | 0% | 95% | Compaction torture test |

When context windows fill and auto-compaction kicks in, baseline agents lose architectural decisions. MDEMG persists them in the graph.

**The baseline forgets under pressure. MDEMG remembers.**

---

## Overview

MDEMG provides long-term memory for AI agents, enabling them to:

- **Store observations**: Persist code patterns, decisions, and architectural knowledge
- **Semantic recall**: Retrieve relevant memories via vector similarity search
- **Concept abstraction**: Automatically form higher-level concepts from related memories (hidden layers)
- **Associative learning**: Build connections between memories through Hebbian reinforcement
- **LLM re-ranking**: Apply GPT-powered relevance scoring for improved retrieval quality

## Key Features

- **Multi-layer graph architecture**: Base observations (L0) → Hidden concepts (L1) → Abstract concepts (L2+)
- **Hybrid search**: Combines vector similarity with graph traversal, fused by RRF column voting with sparse gating and context fingerprinting (see [`docs/features/column-voting-retrieval.md`](docs/features/column-voting-retrieval.md))
- **Conversation Memory System (CMS)**: Persistent memory across agent sessions with surprise-weighted learning
- **Symbol extraction (UPTS)**: Unified Parser Test Schema supporting 28 languages with file:line evidence and SHA256 fixture verification
- **Plugin system**: Extensible via ingestion, reasoning, and APE (Autonomous Pattern Extraction) modules
- **Evidence-based retrieval**: Returns symbol-level citations (file:line references) with results
- **Capability gap detection**: Identifies missing knowledge areas for targeted improvement
- **Codebase ingestion API**: Background job processing for large codebase ingestion with consolidation
- **Git commit hooks**: Automatic incremental ingestion on every commit via post-commit hook
- **Freshness tracking**: TapRoot-level staleness detection with configurable thresholds
- **Scheduled sync**: Periodic background sync to keep memory graphs up-to-date
- **Webhook integration**: Linear webhook endpoint with HMAC signature verification and debouncing
- **File watcher**: Standalone `mdemg-watch` binary for automatic re-ingestion on file changes
- **Orphan detection**: Timestamp-based detection of nodes missing from re-ingestion with archive/delete/list actions
- **Graph orphan cleanup**: Cross-space zero-edge node scan with consolidate/archive/delete fix actions and protected space enforcement
- **Edge consistency**: Automatic staleness tracking and edge weight refresh during consolidation
- **Backup & restore**: Automated full database dumps and partial space exports with retention policies and scheduler
- **Neo4j state monitoring**: Single endpoint for consolidated database health, per-space statistics (nodes, edges, layers, health score, staleness), and backup overview
- **Meta-cognition enforcement**: Server-side anomaly detection (empty-resume, empty-recall), hook circuit breakers with CRITICAL warnings, multi-dimensional watchdog monitoring, Hebbian signal learning for adaptive enforcement
- **Jiminy Inner Voice**: Proactive guidance service injected into every prompt — surfaces constraints, prior corrections, contradictions, and frontier exploration opportunities from the knowledge graph via 4-source parallel fan-out under a config-derived guidance budget
- **J17 AI-to-AI Communication Protocol**: Three-tier encoding protocol (T1 coded ~15 tokens, T2 telegraphic, T3 full NL) for compact agent-to-agent communication. LLM-generated constraint codes, per-session trust scoring, signed session tickets for state persistence across context resets, RSIC-driven protocol evolution, and ML-powered tier prediction via neural sidecar
- **Never-silent operations**: a goroutine supervisor with panic recovery over every background loop, scheduled-job health that alerts on "failed OR never ran," hook-channel self-monitoring, restore-tested backups, and a server-native alert evaluator — engineered so a dead seam self-reports instead of failing quietly
- **Closed guidance→feedback→outcome→correction loop**: Jiminy guidance is scored against what the agent actually did — per-constraint effectiveness rates feed RSIC calibration. When the classifier fires a `contradicted` verdict, the JIMINY-CONTRADICTED-BRIDGE-001 sink emits a correction draft for HITL review; approved drafts flow through `/v1/conversation/correct` → L0 obs → JIMINY-CORRECTION-PRODUCER-001 promotion → L1 `role_type=correction` node → back into retrieval, closing the autonomous-learning loop end-to-end. `Incorrect`/`Correct`/`Context` fields propagate as structured properties (JIMINY-STRUCTURED-CORRECTION-001); Lever B synthesis renders `Do <correct> — not <incorrect>` from the structured pair. The `jiminy-governance` Claude Code skill makes Jiminy a deterministic source of context and constraints over J17, with a fail-closed bash guard and graduated `/strict`-mode enforcement on edits
- **Event-graph federation**: `mdemg eventgraph` surfaces reinforcement and guidance-outcome events in a node's graph neighborhood by federating TimescaleDB event streams with Neo4j traversal — events stay in TSDB, never reified into the graph
- **Space Transfer & DevSpace**: Export/import space graphs as `.mdemg` files or via gRPC; optional DevSpace hub for agent registration, publish/pull exports, and inter-agent messaging (see `cmd/space-transfer/README.md` and `docs/specs/development-space-collaboration.md`)

## Self-Improving Local LLM

MDEMG runs its **own fine-tuned model** for its internal LLM calls — the
classification, guidance synthesis, concept emergence, intent translation, and
re-ranking the graph depends on, across ~16 task surfaces. The shipped model is
`mdemg-llm-v1`: a fine-tuned dense Qwen3-14B, distributed as fused GGUF quants
and served locally via `llama.cpp llama-server` (an adapter-only pull is also
available — see Step 2b below for installation). Each task has a fixed JSON
output contract; the fine-tune exists to follow those contracts more reliably
and cheaply than a stock base. The LLM provider is operator-configurable —
MDEMG requires a reachable LLM endpoint to start.

The model improves through an operator-driven **curate → train → benchmark →
promote** loop: every internal LLM interaction is captured (quality-filtered and
sanitized) into TimescaleDB; `mdemg data curate` builds temporally-split
datasets whose held-out test slice training never sees; candidates train via
the local MLX LoRA pipeline and are benchmarked on the held-out slice
(UBENCH); promotion is gated on measured lift and operator approval — a
candidate that regresses is rejected, which is the gate doing its job. This is
the offline outer loop of MDEMG's self-improvement, complementing RSIC's
online micro-corrections; the fully-automated recursive-retraining loop is
built (`internal/ftloop`) and ships default-off (`FT_LOOP_ENABLED`), so by
default the loop runs operator-driven.

Feature docs: [`fine-tuning-pipeline.md`](docs/features/fine-tuning-pipeline.md),
[`neural-training-pipeline.md`](docs/features/neural-training-pipeline.md),
[`ubench-framework.md`](docs/features/ubench-framework.md),
[`local-model-distribution.md`](docs/features/local-model-distribution.md).

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      AI Coding Agent                        │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   IDE/CLI    │◄──►│  MCP Server  │◄──►│ MDEMG Client │  │
│  │  (Cursor)    │    │   (tools)    │    │    (HTTP)    │  │
│  └──────────────┘    └──────────────┘    └──────┬───────┘  │
└─────────────────────────────────────────────────┼──────────┘
                                                  │
                    ┌─────────────────────────────▼─────────┐
                    │     MDEMG Service (dynamic port)      │
                    │  ┌─────────┐  ┌───────────────────┐  │
                    │  │Embedding│  │    Neo4j Graph    │  │
                    │  │ Provider│  │ (Vector + Graph)  │  │
                    │  └─────────┘  └───────────────────┘  │
                    └───────────────────────────────────────┘
```

## Development Setup

### Prerequisites

- Go 1.24+
- Docker (for Neo4j)
- Embedding provider: [OpenAI API key](https://platform.openai.com/api-keys) or [Ollama](https://ollama.com) (local, free)

### Build from Source

```bash
git clone https://github.com/reh3376/mdemg.git && cd mdemg
go build -o bin/mdemg ./cmd/mdemg
./bin/mdemg --help            # See all commands
```

### Configure and Start

```bash
cd /path/to/your/project
mdemg init                    # Interactive wizard creates .mdemg/config.yaml + .env
mdemg db start                # Project-scoped Neo4j container (auto-selects free port)
mdemg start --auto-migrate    # Server daemon with schema migrations
```

### Ingest a Codebase

```bash
mdemg ingest --space-id=my-project --path=/path/to/repo
mdemg ingest --space-id=my-project --path=/path/to/repo --incremental  # Changed files only
mdemg hooks install  # Auto-ingest on git commit
```

## API Endpoints

### Core Memory Operations

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/memory/retrieve` | POST | Semantic search with optional LLM re-ranking |
| `/v1/memory/consult` | POST | SME-style Q&A with evidence citations |
| `/v1/memory/ingest` | POST | Store a single observation |
| `/v1/memory/ingest/batch` | POST | Store multiple observations |
| `/v1/memory/consolidate` | POST | Trigger hidden layer creation |
| `/v1/memory/stats` | GET | Per-space memory statistics |
| `/v1/memory/node/meta` | GET | Content-hash metadata for a single node (change detection) |
| `/v1/memory/ingest-codebase` | POST | Background codebase ingestion job |
| `/v1/memory/symbols` | GET | Query extracted code symbols |
| `/v1/memory/ingest/files` | POST | Ingest files with background job processing |
| `/v1/memory/spaces/{id}/freshness` | GET | Space freshness and staleness status |
| `/v1/webhooks/linear` | POST | Linear webhook receiver with HMAC verification |

### Web Scraper

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/scraper/jobs` | POST | Create a new scrape job |
| `/v1/scraper/jobs` | GET | List all scrape jobs |
| `/v1/scraper/jobs/{id}` | GET | Get job status and scraped content |
| `/v1/scraper/jobs/{id}` | DELETE | Cancel a running job |
| `/v1/scraper/jobs/{id}/review` | POST | Approve/reject/edit scraped content |
| `/v1/scraper/spaces` | GET | List available target spaces |

### Backup & Restore (Phase 70)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/backup/trigger` | POST | Trigger backup (full database dump or partial space export) |
| `/v1/backup/status/{id}` | GET | Backup job status and progress |
| `/v1/backup/list` | GET | List all backups (optional `?type=` filter) |
| `/v1/backup/manifest/{id}` | GET | Get full backup manifest (checksum, sizes, spaces) |
| `/v1/backup/{id}` | DELETE | Delete a backup artifact |
| `/v1/backup/restore` | POST | Trigger restore from full backup |
| `/v1/backup/restore/status/{id}` | GET | Restore job status |

### Hash Verification (UNTS)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/hash-verification/register` | POST | Register a file for hash tracking |
| `/v1/hash-verification/files` | GET | List tracked files (optional `?framework=`/`?status=` filters) |
| `/v1/hash-verification/files/{path}` | GET | Get single file status and history |
| `/v1/hash-verification/verify` | POST | Verify single file hash against expected |
| `/v1/hash-verification/verify-all` | POST | Verify all tracked files |
| `/v1/hash-verification/update` | POST | Update expected hash for a file |
| `/v1/hash-verification/revert` | POST | Revert to a previous hash from history |
| `/v1/hash-verification/scan` | POST | Scan manifest + specs to register files |

Requires `UNTS_ENABLED=true` and `UNTS_BASE_PATH` (default: `.`). Returns 503 when disabled.

### Admin — Space Lifecycle

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/admin/spaces` | GET | List all spaces with metadata and prunable status |
| `/v1/admin/spaces/{id}` | PATCH | Update space metadata (prunable flag) |
| `/v1/admin/spaces/prune` | POST | Batch prune prunable/orphan spaces (dry-run supported) |

A background auto-prune scheduler runs every 24 hours by default (`SPACE_PRUNE_INTERVAL_HOURS`, 0 to disable).

### Admin — Circuit Breakers

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/admin/breakers` | GET | List all registered circuit breakers with state (closed/open/half-open) and counts |
| `/v1/admin/breakers/reset` | POST | Force a named breaker to `StateClosed` (body: `{"name":"<breaker-name>"}`) |

Requires `AUTH_API_KEYS`. Operator escape hatch when a breaker trips on a transient incident but hasn't auto-recovered. See `docs/user/api-reference.md#admin-circuit-breakers`.

### Conversation Memory System (CMS)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/conversation/resume` | POST | Restore session context with themes and concepts |
| `/v1/conversation/observe` | POST | Record observations (decisions, corrections, learnings) |
| `/v1/conversation/correct` | POST | Record user corrections for learning |
| `/v1/conversation/recall` | POST | Query conversation history |
| `/v1/conversation/consolidate` | POST | Create themes from observations |
| `/v1/conversation/session/anomalies` | GET | Aggregated session anomalies and health |
| `/v1/conversation/templates` | GET/POST | List or create observation templates |
| `/v1/conversation/templates/{id}` | GET/PUT/DELETE | CRUD for specific template |
| `/v1/conversation/snapshot` | POST | Create session snapshot |
| `/v1/conversation/snapshot/{id}` | GET/DELETE | Retrieve or delete snapshot |
| `/v1/conversation/snapshot/{id}/restore` | POST | Restore session from snapshot |
| `/v1/conversation/relevance` | POST | Compute observation relevance scores |
| `/v1/conversation/truncate` | POST | Truncate old observations |
| `/v1/conversation/org-review` | GET | Get observations pending org review |
| `/v1/conversation/org-review/{id}/approve` | POST | Approve observation for org sharing |
| `/v1/conversation/org-review/{id}/reject` | POST | Reject observation from org sharing |

### Learning Control

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/v1/learning/stats` | GET | Hebbian learning edge statistics |
| `/v1/learning/freeze` | POST | Freeze learning for stable scoring |
| `/v1/learning/unfreeze` | POST | Resume learning edge creation |
| `/v1/learning/prune` | POST | Remove decayed edges |

## Symbol Extraction (UPTS)

> For the comprehensive guide to UxTS methodology (architecture, spec writing, CI integration, governance, and the full framework family), see [docs/guides/UXTS_DEVELOPER_GUIDE.md](docs/guides/UXTS_DEVELOPER_GUIDE.md).

MDEMG extracts code symbols during ingestion using the Unified Parser Test Schema (UPTS):

**Supported Languages (28 UPTS-validated, 100% pass rate):**

- **Systems**: Go, Rust, C, C++, CUDA
- **JVM**: Java, Kotlin
- **.NET**: C#
- **Scripting**: Python, TypeScript/JavaScript, PHP, Lua, Shell
- **API Schemas**: Protocol Buffers, GraphQL, OpenAPI
- **Configuration**: YAML, TOML, JSON, INI
- **Infrastructure**: Terraform/HCL, Dockerfile, Makefile
- **Database**: SQL, Cypher (Neo4j)
- **Documentation**: Markdown, XML, Scraper Markdown (web-scraped content with section chunking)

**Extracted Symbol Types:**

- Functions, methods, classes, interfaces, types
- Constants, variables, enums
- Imports, exports, module declarations

Symbols include file:line references for evidence-based retrieval.

## Conversation Memory System (CMS)

CMS provides persistent memory for AI agents across sessions:

```bash
# Resume session context (call at session start)
curl -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id": "my-agent", "session_id": "session-1", "max_observations": 10}'

# Record an observation (decision, learning, correction)
curl -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "my-agent",
    "session_id": "session-1",
    "content": "User prefers TypeScript for new files",
    "obs_type": "preference"
  }'
```

**Observation Types:** 15 types including `decision`, `correction`, `learning`, `preference`, `error`, `progress`, `constraint`, `insight`, and `note` (see `internal/conversation/types.go` for the full set)

Observations are surprise-weighted (novel information persists longer) and form themes via consolidation.

## MCP Integration

MDEMG provides an MCP server for IDE integration.

### Automatic Setup (Claude Code)

```bash
mdemg sidecar attach-agent claude-code   # Writes .claude/mcp.json + enables MCP in settings
# Or as part of full setup:
mdemg sidecar quickstart                 # init + install + up + attach + hooks
```

### Manual Setup (Cursor IDE)

Add to `~/.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "mdemg": {
      "command": "mdemg",
      "args": ["mcp"],
      "env": {
        "MDEMG_ENDPOINT": "http://localhost:9999"
      }
    }
  }
}
```

## Project Structure

```
mdemg/
├── cmd/                  # CLI tools (server, ingest, MCP, etc.)
├── internal/             # Core logic
│   ├── api/              # HTTP handlers
│   ├── retrieval/        # Search algorithms
│   ├── hidden/           # Concept abstraction
│   ├── learning/         # Hebbian edges
│   ├── conversation/     # Conversation Memory System (CMS)
│   ├── backup/           # Backup & restore (full, partial, scheduler, retention)
│   ├── symbols/          # Code symbol extraction
│   └── plugins/          # Plugin system
├── docs/                 # Documentation
│   └── benchmarks/       # Benchmark questions, graders, results
├── migrations/           # Neo4j schema (Cypher)
├── tests/                # Integration tests
└── docker-compose.yml    # Neo4j container
```

## Observability Stack

MDEMG includes a complete observability stack for monitoring and debugging.
The TimescaleDB telemetry plane that feeds it is governed end-to-end — every
hypertable carries retention and compression policies, and the alert evaluator
runs server-native (Grafana is dashboards-only, not required for alerting);
see [`docs/features/tsdb-data-management.md`](docs/features/tsdb-data-management.md).

### Quick Start

```bash
cd deploy/docker
docker compose -f docker-compose.observability.yml up -d

# Access dashboards
open http://localhost:3000  # Grafana (admin/admin)
```

### Components

| Component | Port | Description |
|-----------|------|-------------|
| TimescaleDB | 5433 | Metrics storage (time-series SQL) |
| Grafana | 3000 | Dashboard visualization (dashboards only — alerting is server-native) |

### MDEMG Overview Dashboard

Pre-configured dashboard with 12 panels:

- Request Rate, P95 Latency, Error Rate, Circuit Breakers
- Request Latency Distribution (p50/p95/p99)
- Requests by Status, Cache Hit Ratios
- Retrieval Latency, Rate Limit Rejections, Embedding Latency
- Historical Trends row: 90-day Health Trend

### Metrics Endpoint

```bash
curl http://localhost:9999/v1/metrics/snapshot
```

Returns a JSON metrics snapshot including counters, gauges, and histograms from the MetricsRecorder. Metrics are persisted to TimescaleDB.

---

## Development Roadmap

### Recently Completed

**2026 Guidance-Quality Arc + Substrate Honesty (post-Phase-105):**

| Sprint | Name | Status |
|--------|------|--------|
| LLM-HEALTH-INVESTIGATION-001 | RSIC alert honesty — recorder tags caller-cancellation, rerank pre-check skips insufficient-budget calls, alert-rule filter excludes caller_canceled from LLM error rate | ✅ Complete (2026-07-20) |
| JIMINY-STRUCTURED-CORRECTION-001 | Propagate `Incorrect`/`Correct`/`Context` triple from `POST /v1/conversation/correct` through L0 → L1 → GuidanceItem → Lever B synthesis; `mdemg corrections rehydrate-structured` backfill CLI | ✅ Complete (2026-07-20) |
| JIMINY-CONTRADICTED-BRIDGE-001 | Bridge Jiminy `contradicted` verdicts into V0030 `contradicted_correction_drafts` for HITL review; on approve, sink calls `conversation.Correct` → L0 → L1 correction via the shipped producer | ✅ Complete (2026-07-20) |
| JIMINY-CORRECTION-PRODUCER-001 | `CreateCorrectionNodes` promotes L0 `obs_type='correction'` observations to L1 `role_type='correction'` nodes via `IMPLEMENTS_CORRECTION` edges; `CorrectionPromotionGate` mirrors constraint gate | ✅ Complete (2026-07-20) |
| JIMINY-ROLETYPE-ADAPTER-001 | Propagate `role_type`/`obs_type` end-to-end through `models.RetrieveResult` → `Candidate` → `FusedCandidate` → `BM25Result` → `jiminy.RetrievalResult`; classifier prefers `role_type` before `ObsType` switch — retrieval-sourced guidance no longer defaults to `learning` | ✅ Complete (2026-07-17) |
| JIMINY-CORPUS-001 | Constraint corpus cleanup — `ConstraintPromotionGate` blocks non-durable observations at promotion; 140→61 live constraint nodes via operator-authorized purge; per-session cooldown + effectiveness-prior re-rank; precise 4-band relevance gate | ✅ Complete (2026-07-03) |
| DASHBOARD-TRUTH-001 | Made RSIC/J17/Jiminy dashboards honest — 6/8 alarming metrics were LIMIT-1/COALESCE-to-0/stale-gauge/wrong-anchor artifacts; hardened at source | ✅ Complete (2026-07-03) |
| HITL-REVIEW-001 | General-purpose Human-in-the-Loop review + live reinforcement platform (`ReviewableDataset` interface, `Sink` semantics, reversible substrate mutation, 17 registered datasets) | ✅ Complete (2026-06-24) |
| JIMINY-ACTIONABILITY-001 | Guidance surface bias + directive synthesis — Levers A/B/C to raise the actionable-guidance fraction; Lever C actionable-composition wins the A/B (11.1%→47.7% actionable fraction) | ✅ Complete (2026-06-24) |
| JIMINY-RELEVANCE-001 | Guidance training corpus — V0027 `guidance_training_rows` captures per-item evidence; auto-relabel job; should-follow rate over actionable types | ✅ Complete (2026-06-24) |
| RETRIEVAL-TYPED-EDGES-002 | Grew semantic edges (`dynamic_edges` vector-index rewrite, O(n²)→O(n·logn), 4415 edges) + flipped default-ON — clean A/B +0.001 mean, 0 regressions | ✅ Complete (2026-07-03) |
| SUPERVISOR-002 | Extended goroutine supervisor from 3 to 15 background loops with sliding-window restart budget + rule-health meta-alert + RSIC recency gate | ✅ Complete (2026-06-11) |
| ALERT-TRUTH-001 / TSDB-CONSUME-001 / NOSILENT-001 | Alert-rule SQL contract (idle-safe aggregate + COALESCE, no `ORDER BY … LIMIT 1`); scheduled-job health with `jobhealth.Report`; alert cooldown key = `(Service, Severity)` requires distinct Service per rule | ✅ Complete (2026-06) |
| MODEL-DIST-001/002 | `mdemg model pull` distributes `mdemg-llm-v1` via Ollama Library (3 fused GGUF quants + adapter-only path); RAM-tier auto-selection; 11 config-driven knobs, zero hardcoded values | ✅ Complete (2026-05) |

**Historical phase registry:**

| Phase | Name | Status |
|-------|------|--------|
| J17 | AI-to-AI Communication Protocol (3-tier encoding, trust scoring, ML tier prediction) | ✅ Complete |
| 104 | Active MCP Guardrails (constraint validation pipeline, MCP tool) | ✅ Complete |
| 103/103b | Dynamic Emergence + UETS Model Evaluation (LLM concept naming, 8-model benchmark) | ✅ Complete |
| 102 | Intent Translation (LLM query rewriting before vector embedding) | ✅ Complete |
| 101 | SME Synthesis Engine (LLM-driven synthesis for consult endpoint) | ✅ Complete |
| 97 | Process Lifecycle + Secret Management (daemon mode, keychain) | ✅ Complete |
| 96 | IDE + Repo Integration (git hooks, MCP config, serve --mcp) | ✅ Complete |
| 95 | Database + Embedding + Migrations (Go-native runner, Docker mgmt) | ✅ Complete |
| 94 | Config Simplification + Project Init (YAML config, mdemg init) | ✅ Complete |
| 93 | Unified CLI Foundation (12 binaries → single mdemg binary) | ✅ Complete |
| 92 | Gap Analysis — Deployable Package (15-gap analysis, Phase 93-100 roadmap) | ✅ Complete |
| — | Space Pruning Framework (admin API, auto-prune scheduler, UATS specs) | ✅ Complete |
| D | Validation (2nd codebase benchmark, 28K scale test, architecture docs) | ✅ Complete |
| 91 | RSIC Observability & Operations (TSDB metrics, Grafana dashboard, alert rules) | ✅ Complete |
| 90 | RSIC Conformance & CI Gating (integration tests, CI split, tag filtering) | ✅ Complete |
| 89 | RSIC Persistence & Multi-Space Correctness (write-behind, RSICState nodes) | ✅ Complete |
| 88 | RSIC Safety & Policy Enforcement (dry-run, rollback, blast-radius) | ✅ Complete |
| 87 | RSIC Orchestration Activation (trigger sources, cooldown, dedupe) | ✅ Complete |
| 80 | CMS ANN Meta-Cognition & Self-Improvement Enforcement | ✅ Complete |
| 76 | Neo4j State Monitor & Space Overview | ✅ Complete |
| 75 | Cross-File Relationship Extraction & Graph Topology Hardening | ✅ Complete |
| 70 | Neo4j Backup & Restore (Full & Partial) with Scheduler | ✅ Complete |
| 38 | UNTS Hash Verification REST API (8 endpoints, 8 UATS specs) | ✅ Complete |
| 60b | Recursive Self-Improvement Cycle (RSIC) | ✅ Complete |
| 60 | CMS Advanced Functionality II (Templates, Snapshots, Relevance, Truncation, Org-Review) | ✅ Complete |
| 51 | Web Scraper Ingestion Module | ✅ Complete |
| 49 | LLM Plugin SDK (Scaffolding, Validation, Gap Detection) | ✅ Complete |

| 98 | Cross-Platform Build + Release (goreleaser, Homebrew, self-update) | ✅ Complete |
| 99 | Onboarding + Polish (quickstart, demo, FAQ) | ✅ Complete |

See [AGENT_HANDOFF.md](AGENT_HANDOFF.md) for detailed phase specifications.

---

## Documentation

### User Guides

| Guide | What it covers |
|-------|---------------|
| [ELI5 (Explain Like I'm 5)](ELI5.md) | Simplified explanation of what MDEMG does and how it works |
| [Quickstart Guide](docs/quickstart.md) | 10-minute setup walkthrough |
| [FAQ](docs/FAQ.md) | Common questions and troubleshooting |
| [CLI Reference](docs/user/cli-reference.md) | All commands, flags, defaults, examples, environment variables |
| [API Reference](docs/user/api-reference.md) | Every HTTP endpoint with request/response shapes and curl examples |
| [CMS & RSIC Guide](docs/user/cms-rsic-guide.md) | Conversation memory, observation types, surprise scoring, self-improvement cycles |
| [Ingestion Guide](docs/user/ingestion-guide.md) | All 8 ingestion methods — codebase, scraper, Linear, webhooks, file watcher, API |

### Contributor & Architecture Docs

- [CLI Implementation](docs/features/unified-cli.md) - CLI internals and command structure
- [API Implementation](docs/development/API_REFERENCE.md) - Internal API documentation
- [Architecture](docs/architecture/01_Architecture.md) - System design and components
- [Graph Schema](docs/architecture/02_Graph_Schema.md) - Neo4j labels and relationships
- [Retrieval & Scoring](docs/architecture/06_Retrieval_API_and_Scoring.md) - Scoring algorithm details
- [Benchmarking Guide](docs/architecture/benchmarks/BENCHMARK_V4_README.md) - Running and validating benchmarks
- [Backup & Restore Guide](docs/development/NEO4J_BACKUP.md) - Backup configuration and retention
- [Agent Handoff](AGENT_HANDOFF.md) - Complete development context and phase registry

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## Security

See [SECURITY.md](SECURITY.md) for the vulnerability reporting policy.

## License

MIT License - see [LICENSE](LICENSE) for details.
