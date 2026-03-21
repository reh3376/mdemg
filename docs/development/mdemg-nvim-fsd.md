# MDEMG Neovim Plugin — Functional Specification & Development Planning Guide

**Document ID:** MDEMG-NVIM-FSD-001
**Version:** 1.0.0
**Date:** 2026-03-19
**Author:** ACI Solutions LLC
**Status:** Final Draft — Pending team review of command/keymap conventions

---

## 1. Executive Summary

This document specifies the functional requirements, architecture, and phased development plan for `mdemg.nvim` — a Neovim plugin providing full integration with the MDEMG (Multi-Dimensional Emergent Memory Graph) system. The plugin targets Neovim 0.10+ and is written in Lua.

MDEMG exposes over 165 active API endpoints across 31 domains via a local REST API (default `localhost:9999`). The existing MCP server wraps only 20 of those endpoints (~12%) for AI agent consumption via stdio. This plugin fills the gap by giving human developers direct, keyboard-driven access to the complete API surface from within their editor. The development of a useful MCP server that exposes the full functionality of the mdemg application should be added as a future project as well.  

### 1.1 Goals

- Provide every developer on the team with zero-friction access to the full MDEMG API from Neovim.
- Eliminate context switching: no terminal hopping for memory operations, ingestion, diagnostics, or administrative tasks.
- Surface MDEMG health and state passively via statusline integration and diagnostics.
- Automate high-frequency workflows: ingest-on-save, pre-commit guardrail validation, contextual recall.
- Maintain a thin, maintainable codebase with minimal external dependencies.

### 1.2 Non-Goals

- Reimplementing MCP protocol handling. The plugin communicates directly with the REST API.
- Building a full graph visualization. The terminal is the wrong surface for spatial graph rendering.
- Replacing the CLI for one-time setup operations (`mdemg init`, `mdemg db start`). The plugin assumes MDEMG is already initialized and running.

### 1.3 Audience

All developers at WHK: Whiskey House of Kentucky using Neovim as their primary IDE. Usage of MDEMG is mandatory; this plugin is the primary human interface.

---

## 2. Technical Context

### 2.1 MDEMG Architecture Summary

MDEMG is a Go application backed by Neo4j with native vector indexes. It runs as a local HTTP server (daemon) and exposes a REST API. Each project gets its own Neo4j container and isolated data volume. The server resolves ports automatically if the default (7687/9999) is occupied.

Key integration points:

| Component | Role | Plugin Relevance |
|-----------|------|-----------------|
| REST API (`:9999`) | Primary interface — 165 endpoints, JSON in/out | Direct target for all plugin operations |
| MCP Server (stdio) | Agent-facing wrapper — 20 tools | Not used by the plugin |
| File Watcher (`mdemg watch`) | fsnotify-based daemon for auto-ingestion | Plugin replaces this with per-buffer `BufWritePost` ingestion |
| Sidecar System | Lifecycle management for agent integration | Plugin reads sidecar config for endpoint resolution |
| UATS Framework | 178 test spec files defining the API contract | Source of truth for endpoint behavior and validation |
| Config (`.mdemg/config.yaml`) | Per-project configuration | Plugin reads for space_id, endpoint, provider settings |

### 2.2 API Surface Inventory

**165 active endpoints across 31 domains.** The full route map grouped by domain:

#### Core Memory (38 endpoints)
The primary developer interaction surface. Retrieval, ingestion, reflection, consolidation, node lifecycle, symbol search, caching, distribution stats, edge management, frontiers, meta-learning, suggestions, and consultation.

Key endpoints:
- `POST /v1/memory/retrieve` — Semantic search with vector similarity and Hebbian activation scoring
- `POST /v1/memory/ingest` — Store a single observation
- `POST /v1/memory/ingest/batch` — Batch ingest multiple observations
- `POST /v1/memory/ingest/files` — Re-ingest specific changed files
- `POST /v1/memory/ingest/trigger` — Trigger async background codebase ingestion
- `GET /v1/memory/ingest/status/{id}` — Poll ingestion job progress
- `POST /v1/memory/ingest/cancel/{id}` — Cancel a running ingestion job
- `GET /v1/memory/ingest/jobs` — List all ingestion jobs
- `POST /v1/memory/ingest-codebase` — Full codebase ingestion
- `GET /v1/memory/ingest-codebase` — List codebase ingestion jobs
- `GET /v1/memory/ingest-codebase/{id}` — Codebase ingestion job status
- `DELETE /v1/memory/ingest-codebase/{id}` — Cancel codebase ingestion
- `POST /v1/memory/reflect` — Broader search with graph traversal
- `POST /v1/memory/consolidate` — Trigger memory consolidation
- `POST /v1/memory/consult` — Intent-translated consultation
- `POST /v1/memory/suggest` — Proactive suggestions based on context
- `GET /v1/memory/stats` — Memory space statistics
- `GET /v1/memory/symbols` — Code symbol search (functions, classes, types)
- `GET /v1/memory/distribution` — Node distribution across dimensions
- `GET /v1/memory/frontiers` — Knowledge frontier detection
- `POST /v1/memory/meta-learn` — Promote patterns to global space
- `POST /v1/memory/nodes/{id}/archive` — Archive a memory node
- `POST /v1/memory/nodes/{id}/unarchive` — Restore archived node
- `DELETE /v1/memory/nodes/{id}` — Permanently delete a node
- `POST /v1/memory/archive/bulk` — Bulk archive nodes
- `GET /v1/memory/cache/stats` — Retrieval cache statistics
- `DELETE /v1/memory/cache` — Clear retrieval cache
- `GET /v1/memory/query/metrics` — Query performance metrics
- `GET /v1/memory/edges/stale/stats` — Stale edge statistics
- `POST /v1/memory/edges/stale/refresh` — Refresh stale edges
- `GET /v1/memory/freshness` — Batch freshness across spaces
- `GET /v1/memory/spaces/{space_id}/freshness` — Per-space freshness
- `POST /v1/memory/guardrail/validate` — Validate changes against constraints
- `POST /v1/memory/cleanup/orphans` — Remove orphaned nodes
- `POST /v1/memory/cleanup/graph-orphans` — Remove graph-level orphans
- `POST /v1/memory/cleanup/schedule` — Schedule cleanup jobs
- `GET /v1/memory/cleanup/schedules` — List cleanup schedules
- `GET /v1/memory/cleanup/stats` — Cleanup statistics

#### Conversation Memory (24 endpoints)
Session-scoped volatile memory with graduation to persistent storage.

- `POST /v1/conversation/observe` — Capture observation with surprise detection
- `POST /v1/conversation/correct` — Correct a previous observation
- `POST /v1/conversation/resume` — Resume a session with context replay
- `POST /v1/conversation/recall` — Recall from conversation memory
- `POST /v1/conversation/consolidate` — Consolidate volatile to persistent
- `POST /v1/conversation/graduate` — Process graduation candidates
- `GET /v1/conversation/volatile/stats` — Volatile memory statistics
- `GET /v1/conversation/session/health` — Session health check
- `GET /v1/conversation/session/anomalies` — Session anomaly detection
- CMS Snapshots: create, list, get, delete, cleanup, latest (6 endpoints)
- CMS Templates: create, list, get, update, delete (5 endpoints)
- Org Reviews: list, stats, flag-org, decision (4 endpoints)

#### Jiminy Guidance (2 endpoints)
Proactive constraint-aware guidance system.

- `POST /v1/jiminy/guide` — Get guidance for current context
- `POST /v1/jiminy/feedback` — Provide feedback on guidance quality

#### Constraints (6 endpoints)
Organizational constraint management.

- `GET /v1/constraints` — List active constraints
- `GET /v1/constraints/stats` — Constraint statistics
- `GET /v1/constraints/effectiveness` — Constraint effectiveness metrics
- `GET /v1/constraints/conflicts` — List constraint conflicts
- `POST /v1/constraints/detect-conflicts` — Detect new conflicts
- `PATCH /v1/constraints/scope/{id}` — Update constraint scope

#### Guardrails (2 endpoints)
Pre-commit validation against organizational rules.

- `POST /v1/memory/guardrail/validate` — Validate diff against constraints
- `GET /v1/guardrail/events` — Guardrail event history

#### Symbols (2 endpoints)
Code symbol graph queries.

- `GET /v1/symbols/relationships` — Symbol relationship statistics
- `GET /v1/symbols/{id}/relationships` — Relationships for a specific symbol

#### Learning (6 endpoints)
Hebbian learning system control.

- `GET /v1/learning/stats` — Learning statistics
- `POST /v1/learning/prune` — Prune weak learned connections
- `POST /v1/learning/freeze` — Freeze learning (prevent weight updates)
- `POST /v1/learning/unfreeze` — Resume learning
- `GET /v1/learning/freeze/status` — Check freeze state
- `POST /v1/learning/negative-feedback` — Register negative feedback signal

#### Self-Improvement / RSIC (11 endpoints)
Recursive Self-Improvement Cycle control and monitoring.

- `POST /v1/self-improve/cycle` — Trigger improvement cycle
- `POST /v1/self-improve/assess` — Run self-assessment
- `GET /v1/self-improve/health` — RSIC health status
- `GET /v1/self-improve/history` — Improvement history
- `GET /v1/self-improve/signals` — Improvement signals
- `GET /v1/self-improve/calibration` — Calibration scores
- `GET /v1/self-improve/report` — Summary report
- `GET /v1/self-improve/report/{id}` — Specific report
- `GET /v1/self-improve/rollback` — Rollback history
- `POST /v1/self-improve/orchestration/reset` — Reset orchestration state
- `POST /v1/self-improve/report/{id}` — Act on report

#### APE — Autonomous Plugin Engine (2 endpoints)
Plugin execution monitoring and triggering.

- `GET /v1/ape/status` — APE status and loaded plugins
- `POST /v1/ape/trigger` — Trigger APE event

#### Plugins (2 endpoints)
Plugin registry management.

- `GET /v1/plugins` — List installed plugins
- `POST /v1/plugins/create` — Register a new plugin

#### Modules (2 endpoints)
Module sync and status.

- `GET /v1/modules` — List loaded modules
- `POST /v1/modules/{id}/sync` — Sync a module

#### System / Capability Gaps (12 endpoints)
Knowledge gap detection and resolution workflow.

- `GET /v1/system/capability-gaps` — List detected gaps
- `GET /v1/system/capability-gaps/metrics` — Gap metrics
- `GET /v1/system/capability-gaps/{id}` — Get specific gap
- `POST /v1/system/capability-gaps/analyze` — Analyze for new gaps
- `POST /v1/system/capability-gaps/{id}/address` — Mark gap as addressed
- `POST /v1/system/capability-gaps/{id}/dismiss` — Dismiss a gap
- `GET /v1/system/gap-interviews` — List gap interview prompts
- `GET /v1/system/gap-interviews/stats` — Interview statistics
- `POST /v1/system/gap-interviews/run` — Run gap interviews
- `POST /v1/system/gap-interviews/{id}/answer` — Answer an interview prompt
- `POST /v1/system/gap-interviews/{id}/skip` — Skip an interview prompt
- `GET /v1/system/pool-metrics` — Connection pool metrics

#### Backup (7 endpoints)
Backup and restore lifecycle.

- `POST /v1/backup/trigger` — Trigger backup
- `GET /v1/backup/status/{id}` — Backup status
- `GET /v1/backup/list` — List backups
- `GET /v1/backup/manifest/{id}` — Backup manifest
- `POST /v1/backup/restore` — Restore from backup
- `GET /v1/backup/restore/status/{id}` — Restore status
- `DELETE /v1/backup/{id}` — Delete backup

#### Scraper (6 endpoints)
External documentation scraping into memory.

- `POST /v1/scraper/jobs` — Create scraper job
- `GET /v1/scraper/jobs` — List scraper jobs
- `GET /v1/scraper/jobs/{id}` — Job status
- `DELETE /v1/scraper/jobs/{id}` — Cancel scraper job
- `POST /v1/scraper/jobs/{id}/review` — Review scraped content
- `GET /v1/scraper/spaces` — List scraped spaces

#### Skills (3 endpoints)
Skill registry for structured knowledge.

- `GET /v1/skills` — List registered skills
- `POST /v1/skills/{id}/register` — Register a skill
- `POST /v1/skills/{id}/recall` — Recall skill knowledge

#### Linear Integration (9 endpoints)
Issue tracker integration.

- `POST /v1/linear/issues` — Create issue
- `GET /v1/linear/issues` — List issues
- `GET /v1/linear/issues/{id}` — Read issue
- `PUT /v1/linear/issues/{id}` — Update issue
- `DELETE /v1/linear/issues/{id}` — Delete issue
- `POST /v1/linear/comments` — Add comment
- `GET /v1/linear/projects` — List projects
- `GET /v1/linear/projects/{id}` — Read project
- `PUT /v1/linear/projects/{id}` — Update project

#### Hash Verification (8 endpoints)
Content integrity verification.

- `POST /v1/hash-verification/register` — Register file hash
- `POST /v1/hash-verification/verify` — Verify single file
- `POST /v1/hash-verification/verify-all` — Verify all registered files
- `POST /v1/hash-verification/update` — Update file hash
- `POST /v1/hash-verification/revert` — Revert to known-good
- `POST /v1/hash-verification/scan` — Scan for changes
- `GET /v1/hash-verification/files` — List registered files
- `GET /v1/hash-verification/files/{id}` — File details

#### File Watcher (3 endpoints)
Server-side file watcher control.

- `POST /v1/filewatcher/start` — Start file watcher
- `POST /v1/filewatcher/stop` — Stop file watcher
- `GET /v1/filewatcher/status` — Watcher status

#### Admin (6 endpoints)
Space management and import/export.

- `GET /v1/admin/spaces` — List spaces
- `PATCH /v1/admin/spaces/{id}` — Update space settings
- `POST /v1/admin/spaces/prune` — Prune orphaned spaces
- `POST /v1/admin/spaces/export` — Export space
- `POST /v1/admin/spaces/import` — Import space
- `GET /v1/admin/spaces/export/preview` — Preview export

#### Webhooks (2 endpoints)
Inbound webhook processing.

- `POST /v1/webhooks/linear` — Linear webhook handler
- `POST /v1/webhooks/{source}` — Generic webhook handler

#### Observability (8 endpoints)
Health, metrics, and diagnostics.

- `GET /healthz` — Liveness probe
- `GET /readyz` — Readiness probe
- `GET /health` — Neural sidecar health
- `GET /v1/embedding/health` — Embedding provider health
- `GET /v1/metrics` — Internal metrics
- `GET /v1/metrics/determinism` — Retrieval determinism score
- `GET /v1/prometheus` — Prometheus metrics export
- `GET /v1/neo4j/overview` — Neo4j connection and index stats

#### Neural Sidecar (3 endpoints)
NLI and reranking services.

- `GET /v1/neural/status` — Neural sidecar status
- `POST /nli` — Natural Language Inference
- `POST /rerank` — Result reranking

#### Jobs (1 endpoint)
SSE streaming for async job progress.

- `GET /v1/jobs/{id}/stream` — Server-Sent Events for job progress

#### Feedback (1 endpoint)
General feedback submission.

- `POST /v1/feedback` — Submit feedback

---

## 3. Plugin Architecture

### 3.1 Directory Structure

```
mdemg.nvim/
├── lua/
│   └── mdemg/
│       ├── init.lua              -- Plugin entry point, setup(), config merging
│       ├── config.lua            -- Default configuration, config schema
│       ├── client.lua            -- HTTP client (vim.system wrapper)
│       ├── api/
│       │   ├── init.lua          -- API module loader
│       │   ├── memory.lua        -- /v1/memory/* endpoints
│       │   ├── conversation.lua  -- /v1/conversation/* endpoints
│       │   ├── jiminy.lua        -- /v1/jiminy/* endpoints
│       │   ├── constraints.lua   -- /v1/constraints/* endpoints
│       │   ├── learning.lua      -- /v1/learning/* endpoints
│       │   ├── self_improve.lua  -- /v1/self-improve/* endpoints
│       │   ├── system.lua        -- /v1/system/* endpoints
│       │   ├── symbols.lua       -- /v1/symbols/* endpoints
│       │   ├── backup.lua        -- /v1/backup/* endpoints
│       │   ├── scraper.lua       -- /v1/scraper/* endpoints
│       │   ├── skills.lua        -- /v1/skills/* endpoints
│       │   ├── linear.lua        -- /v1/linear/* endpoints
│       │   ├── hash.lua          -- /v1/hash-verification/* endpoints
│       │   ├── filewatcher.lua   -- /v1/filewatcher/* endpoints
│       │   ├── admin.lua         -- /v1/admin/* endpoints
│       │   ├── neural.lua        -- /v1/neural/*, /nli, /rerank, training pipeline
│       │   ├── plugins.lua       -- /v1/plugins/* + /v1/ape/* + /v1/modules/*
│       │   ├── webhooks.lua      -- /v1/webhooks/* endpoints
│       │   ├── health.lua        -- Health, readiness, metrics, neo4j, neural, embedding
│       │   └── jobs.lua          -- /v1/jobs/* SSE streaming
│       ├── ui/
│       │   ├── float.lua         -- Floating window management
│       │   ├── picker.lua        -- Telescope picker integration
│       │   ├── statusline.lua    -- Statusline component (lualine/heirline)
│       │   ├── notify.lua        -- Notification wrapper (vim.notify / nvim-notify)
│       │   ├── markdown.lua      -- Markdown rendering in buffers
│       │   └── progress.lua      -- Async job progress display (SSE consumer)
│       ├── actions/
│       │   ├── recall.lua        -- Context-aware memory recall workflows
│       │   ├── store.lua         -- Store observation from buffer/selection
│       │   ├── ingest.lua        -- Ingestion workflows (file, codebase, trigger)
│       │   ├── validate.lua      -- Guardrail validation (pre-commit, on-demand)
│       │   ├── guide.lua         -- Jiminy guidance integration
│       │   ├── reflect.lua       -- Topic reflection workflow
│       │   ├── consult.lua       -- Consultation workflow
│       │   └── gaps.lua          -- Gap interview interactive workflow
│       ├── auto/
│       │   ├── ingest_on_save.lua   -- BufWritePost autocmd for file re-ingestion
│       │   ├── session.lua          -- Session lifecycle management
│       │   └── health_poll.lua      -- Periodic health polling for statusline
│       └── util/
│           ├── config_reader.lua -- .mdemg/config.yaml parser
│           ├── instance.lua      -- Per-project instance resolution (.mdemg.port, endpoint caching)
│           ├── space.lua         -- Space ID resolution logic
│           ├── treesitter.lua    -- Treesitter context extraction
│           └── diff.lua          -- Git diff generation for guardrail validation
├── plugin/
│   └── mdemg.lua                 -- Vim command definitions (:Mdemg*)
├── doc/
│   └── mdemg.txt                 -- Vimdoc help file
├── tests/
│   ├── minimal_init.lua          -- Minimal Neovim config for testing
│   ├── mdemg/
│   │   ├── client_spec.lua       -- HTTP client tests
│   │   ├── config_spec.lua       -- Configuration tests
│   │   └── api/
│   │       └── memory_spec.lua   -- API module tests
│   └── fixtures/
│       └── responses/            -- Mock API response JSON files
├── Makefile                      -- Test runner, lint, format
├── stylua.toml                   -- Lua formatter config
├── .luacheckrc                   -- Lua linter config
└── README.md
```

### 3.2 Core Design Principles

**3.2.1 Direct REST, No MCP**

The plugin communicates exclusively with the MDEMG REST API via HTTP. The MCP server is designed for AI agent stdio communication and adds no value for a human-driven editor integration. Hitting REST directly gives access to all 165 endpoints rather than the 20 exposed through MCP tools.

**3.2.2 Minimal Dependencies**

Hard dependencies: Neovim 0.10+ (for `vim.system()` async subprocess support).

Optional dependencies (graceful degradation):
- `telescope.nvim` — Enhanced picker UI. Falls back to `vim.ui.select()`.
- `nvim-notify` — Enhanced notifications. Falls back to `vim.notify()`.
- `lualine.nvim` or `heirline.nvim` — Statusline integration. Falls back to manual `:MdemgStatus`.
- `treesitter` — Context-aware tag generation. Falls back to filename/filetype heuristics.

**3.2.3 Async-First**

Every API call is non-blocking. `vim.system()` runs curl in a subprocess; callbacks update UI on completion. Long-running operations (codebase ingestion, consolidation, RSIC cycles) use the SSE streaming endpoint (`/v1/jobs/{id}/stream`) for live progress updates.

**3.2.4 Instance Resolution & Configuration**

MDEMG runs as isolated per-project instances. Each project has its own Neo4j container, server process, and API endpoint. Multiple instances can run simultaneously on different ports. The `mdemg-menubar` application monitors all active instances independently.

The plugin resolves which instance to talk to based on the current buffer's project root.

Instance endpoint resolution (per-buffer, cached per project root):
1. Walk up from the buffer's file path to find the nearest `.mdemg/` directory
2. Read `.mdemg.port` file (written dynamically by `mdemg start` with the allocated port)
3. Read `.mdemg/config.yaml` for additional instance configuration
4. `MDEMG_ENDPOINT` environment variable (override for all buffers)
5. `setup()` call in user's Neovim config (global fallback)
6. Default: `http://localhost:9999`

Space ID resolution (derived from the same instance root):
1. `.mdemg/config.yaml` `space_id` field
2. `MDEMG_SPACE_ID` environment variable
3. Project directory basename (matches `mdemg init` default `repo-basename` strategy)

On `BufEnter`, the plugin resolves the instance for the buffer's project root and caches the result in `vim.b.mdemg_endpoint` and `vim.b.mdemg_space_id`. The filesystem walk only happens once per unique project root per session. When a developer has splits across two projects (e.g., `opc_hub` and `mdemg`), each buffer talks to its own instance automatically.

**3.2.5 Session Lifecycle**

On `VimEnter`, the plugin generates a session ID (`nvim-{timestamp}-{short_hash}`) and stores it in `vim.g.mdemg_session_id`. This session ID is passed automatically to all conversation API calls (`observe`, `recall`, `resume`), scoping volatile memory to the editing session.

On `VimLeavePre`, the plugin fires a conversation consolidation call to flush volatile observations to persistent storage. If the consolidation call fails (e.g., instance shutting down simultaneously), the observation data is not lost — it remains in the volatile store and will be consolidated on the next scheduled cycle.

The session ID is visible via `:MdemgStatus` and the session can be manually resumed from a previous ID via `:MdemgConversation resume <session_id>`.

### 3.3 HTTP Client Design (`client.lua`)

The client wraps `vim.system()` calling `curl` for maximum portability (curl is pre-installed on macOS, Linux, and modern Windows).

```
client.request(method, path, opts) -> void
  opts.body     -- table (auto-serialized to JSON) or string
  opts.params   -- table of query parameters
  opts.on_success(status, decoded_body) -- callback
  opts.on_error(err_msg)               -- callback
  opts.timeout  -- seconds (default: 30)
  opts.stream   -- boolean, if true use SSE mode
```

The client reads the resolved endpoint from `vim.b.mdemg_endpoint` (set by instance resolution on `BufEnter`). If no buffer-local endpoint is set (e.g., calling from a non-file buffer), it falls back to `vim.g.mdemg_endpoint` or the configured default.

Error handling: HTTP 4xx/5xx responses are decoded and the `error` field from the JSON body is passed to `on_error`. Connection refused errors produce a user-facing notification: "MDEMG instance not running — run `mdemg start` in project root." Since MDEMG runs locally in Docker, connection failures indicate the instance isn't started, not a network issue. No retry queuing or offline buffering is implemented.

Connection health is tracked per-instance. If 3 consecutive requests to the same endpoint fail, the statusline component switches to an error state for that instance until a health check succeeds.

---

## 4. Functional Specification — Feature Tiers

Features are organized into three tiers based on developer interaction frequency and implementation complexity. Each tier maps to a development phase.

### 4.1 Tier 1 — Core Developer Workflows (Phase 1)

These are the features every developer will use multiple times per session. They must be rock-solid, fast, and frictionless.

#### 4.1.1 Memory Recall (`:MdemgRecall`)

**Trigger:** `<leader>mr` (default keymap), `:MdemgRecall`, or `:MdemgRecall <query>`

**Behavior:**
1. If called with no arguments, opens a Telescope prompt (or `vim.ui.input`) for query entry.
2. If called with arguments, uses them as the query text directly.
3. If called in visual mode, uses the selected text as the query.
4. Sends `POST /v1/memory/retrieve` with `space_id`, `query_text`, and configurable `top_k` (default: 10).
5. Results render in a Telescope picker (if available) or a floating window with the following columns: rank, name, path, relevance %, vector similarity %.
6. Selecting a result opens a detail float showing the full node content, metadata, and tags.
7. Pressing `<CR>` on a detail view inserts the node content at cursor position (or yanks to a register).

**API Endpoints:** `POST /v1/memory/retrieve`

#### 4.1.2 Memory Store (`:MdemgStore`)

**Trigger:** `<leader>ms` (visual mode default), `:MdemgStore`

**Behavior:**
1. In visual mode: selected text becomes the observation `content`.
2. In normal mode: opens a multiline input float for freeform content entry.
3. Auto-generates metadata:
   - `source`: `"neovim-observation"`
   - `tags`: extracted from treesitter context (current function name, class, module) + filetype
   - `path`: hierarchical path derived from the file path relative to project root
   - `name`: auto-generated from first line of content (truncated) or prompted
4. User can edit tags and name before confirming.
5. Sends `POST /v1/memory/ingest`.
6. Displays confirmation notification with node ID.

**API Endpoints:** `POST /v1/memory/ingest`

#### 4.1.3 Ingest on Save (automatic)

**Trigger:** `BufWritePost` autocmd (opt-in via config, default: enabled)

**Behavior:**
1. On buffer write, checks if the file extension matches MDEMG's watched extensions (`.go`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx`, `.rs`, `.java`, `.md`, `.yaml`, `.yml`, `.json`, `.toml`, `.sql`).
2. If matched, sends `POST /v1/memory/ingest/files` with the saved file path to the buffer's resolved MDEMG instance.
3. Debounced: multiple rapid saves within 2 seconds coalesce into a single API call.
4. Silent on success. Shows notification only on error.
5. Configurable: can be disabled globally or per-buffer.
6. Skips silently if no MDEMG instance is resolved for the buffer's project root.

**API Endpoints:** `POST /v1/memory/ingest/files`

#### 4.1.4 Guardrail Validation (`:MdemgValidate`)

**Trigger:** `<leader>mv` (default), `:MdemgValidate`, or automatic pre-commit hook

**Behavior:**
1. Generates a unified diff of the current buffer against its git HEAD version (using `git diff` or `vim.diff`).
2. Sends `POST /v1/memory/guardrail/validate` with the diff and changed file list.
3. Results display in a floating window:
   - **Pass** (green): "No constraint violations."
   - **Warning** (yellow): List of warnings with constraint descriptions and rationale.
   - **Block** (red): List of violations. If blocking violations exist, optionally prevents write (configurable).
4. Each violation/warning links to the constraint node ID for traceability.

**API Endpoints:** `POST /v1/memory/guardrail/validate`

#### 4.1.5 Jiminy Guidance (`:MdemgGuide`)

**Trigger:** `<leader>mj` (default), `:MdemgGuide`

**Behavior:**
1. Automatically gathers context: current file path, buffer content around cursor (±50 lines), treesitter function/class scope.
2. Sends `POST /v1/jiminy/guide` with `context`, `file_path`, and optionally `agent_output` (if there's a visual selection of proposed code).
3. Guidance renders in a non-intrusive floating window (pinned to top-right by default).
4. Window auto-dismisses after configurable timeout or on cursor movement.

**API Endpoints:** `POST /v1/jiminy/guide`, `POST /v1/jiminy/feedback`

#### 4.1.6 Symbol Search (`:MdemgSymbols`)

**Trigger:** `<leader>mS` (default), `:MdemgSymbols`

**Behavior:**
1. Opens a Telescope picker (or `vim.ui.input`) for symbol name search.
2. Supports filter flags: `--type=function`, `--file=pattern`, `--exported`.
3. Sends `GET /v1/memory/symbols` with query parameters.
4. Results show: symbol name, type, file:line, exported status.
5. `<CR>` jumps to the symbol's file and line number.

**API Endpoints:** `GET /v1/memory/symbols`

#### 4.1.7 Memory Status & Statusline

**Passive display** via lualine/heirline component or a manual `:MdemgStatus` command.

**Statusline component shows:**
- Connection state: `MDEMG: ✓` (green) or `MDEMG: ✗` (red)
- Instance endpoint (port differentiates when multiple instances are running)
- Space ID
- Node count (from stats)
- Freshness indicator: `Fresh` or `Stale (4h)` with color coding

**Polls `/readyz` every 30 seconds** (configurable) and caches results per-instance. Full stats refresh (`/v1/memory/stats`) every 5 minutes.

**`:MdemgStatus` shows detailed output:**
- Instance endpoint and version
- Session ID (auto-generated on VimEnter)
- Embedding provider and dimensions
- Node count, edge count, space ID
- Last ingestion time and freshness
- Neo4j connection status
- Neural sidecar status (models loaded, last inference latency)
- Learning freeze status
- Cache hit rate

**API Endpoints:** `GET /readyz`, `GET /v1/memory/stats`, `GET /v1/memory/spaces/{id}/freshness`, `GET /v1/embedding/health`, `GET /v1/learning/freeze/status`

#### 4.1.8 Reflect (`:MdemgReflect`)

**Trigger:** `<leader>mR` (default), `:MdemgReflect <topic>`

**Behavior:**
1. Prompts for topic (or uses argument / visual selection).
2. Sends `POST /v1/memory/reflect` with configurable `depth` (1-3, default: 2).
3. Results render in a new split buffer (markdown formatted, read-only).
4. Grouped by relevance tier: Highly Relevant, Related, Tangentially Related.
5. Includes graph traversal statistics.

**API Endpoints:** `POST /v1/memory/reflect`

### 4.2 Tier 2 — Operational & Management Workflows (Phase 2)

Features used regularly but not on every coding session. Full command interface plus purpose-built UI where it adds value.

#### 4.2.1 Codebase Ingestion Management (`:MdemgIngest`)

**Subcommands:**
- `:MdemgIngest trigger [--mode=full|incremental] [--path=.]` — Trigger background ingestion, displays job ID
- `:MdemgIngest status <job_id>` — Show job progress (polls via SSE if available)
- `:MdemgIngest cancel <job_id>` — Cancel running job
- `:MdemgIngest jobs` — List all jobs in a picker/float
- `:MdemgIngest files <file1,file2,...>` — Re-ingest specific files

**API Endpoints:** `POST /v1/memory/ingest/trigger`, `GET /v1/memory/ingest/status/{id}`, `POST /v1/memory/ingest/cancel/{id}`, `GET /v1/memory/ingest/jobs`, `POST /v1/memory/ingest/files`, `POST /v1/memory/ingest-codebase`, `GET /v1/memory/ingest-codebase`, `GET /v1/memory/ingest-codebase/{id}`, `DELETE /v1/memory/ingest-codebase/{id}`

#### 4.2.2 Conversation Memory (`:MdemgConversation`)

**Subcommands:**
- `:MdemgConversation observe <content>` — Capture observation in current session
- `:MdemgConversation correct <obs_id> <correction>` — Correct an observation
- `:MdemgConversation recall [query]` — Recall from conversation memory
- `:MdemgConversation resume [session_id]` — Resume a session
- `:MdemgConversation consolidate` — Consolidate volatile to persistent
- `:MdemgConversation graduate` — Process graduation candidates
- `:MdemgConversation stats` — Show volatile memory stats
- `:MdemgConversation health` — Session health check
- `:MdemgConversation anomalies` — Show session anomalies

**API Endpoints:** All 24 conversation endpoints.

#### 4.2.3 Constraints Management (`:MdemgConstraints`)

**Subcommands:**
- `:MdemgConstraints list` — List all active constraints in a picker
- `:MdemgConstraints stats` — Show constraint statistics
- `:MdemgConstraints effectiveness` — Show effectiveness metrics
- `:MdemgConstraints conflicts` — List conflicts
- `:MdemgConstraints detect` — Run conflict detection
- `:MdemgConstraints scope <id>` — Update constraint scope

**API Endpoints:** All 6 constraint endpoints.

#### 4.2.4 Learning Controls (`:MdemgLearning`)

**Subcommands:**
- `:MdemgLearning stats` — Learning statistics
- `:MdemgLearning prune` — Prune weak connections
- `:MdemgLearning freeze` — Freeze learning
- `:MdemgLearning unfreeze` — Unfreeze learning
- `:MdemgLearning status` — Freeze status
- `:MdemgLearning feedback <negative|positive>` — Register feedback signal

**API Endpoints:** All 6 learning endpoints.

#### 4.2.5 Self-Improvement / RSIC (`:MdemgRSIC`)

**Subcommands:**
- `:MdemgRSIC cycle [--dry-run]` — Trigger improvement cycle
- `:MdemgRSIC assess` — Run self-assessment
- `:MdemgRSIC health` — RSIC health status
- `:MdemgRSIC history` — Improvement history
- `:MdemgRSIC signals` — List improvement signals
- `:MdemgRSIC calibration` — Calibration scores
- `:MdemgRSIC report [id]` — View report(s)
- `:MdemgRSIC rollback` — Rollback history
- `:MdemgRSIC reset` — Reset orchestration

**API Endpoints:** All 11 self-improve endpoints.

#### 4.2.6 Backup & Restore (`:MdemgBackup`)

**Subcommands:**
- `:MdemgBackup trigger` — Trigger backup
- `:MdemgBackup status <id>` — Check backup status
- `:MdemgBackup list` — List backups in a picker
- `:MdemgBackup manifest <id>` — View backup manifest
- `:MdemgBackup restore <id>` — Restore from backup (with confirmation prompt)
- `:MdemgBackup restore-status <id>` — Check restore progress
- `:MdemgBackup delete <id>` — Delete backup (with confirmation prompt)

**API Endpoints:** All 7 backup endpoints.

#### 4.2.7 Scraper (`:MdemgScraper`)

**Subcommands:**
- `:MdemgScraper create <url> [--space-id=...]` — Create scraper job for a URL
- `:MdemgScraper list` — List scraper jobs
- `:MdemgScraper status <id>` — Job status
- `:MdemgScraper cancel <id>` — Cancel job
- `:MdemgScraper review <id>` — Review scraped content
- `:MdemgScraper spaces` — List scraped spaces

**API Endpoints:** All 6 scraper endpoints.

#### 4.2.8 Capability Gaps (`:MdemgGaps`)

**Subcommands:**
- `:MdemgGaps list` — List detected gaps
- `:MdemgGaps metrics` — Gap metrics summary
- `:MdemgGaps get <id>` — View specific gap
- `:MdemgGaps analyze` — Analyze for new gaps
- `:MdemgGaps address <id>` — Mark gap as addressed
- `:MdemgGaps dismiss <id>` — Dismiss gap
- `:MdemgGaps interviews` — List interview prompts
- `:MdemgGaps interview-stats` — Interview statistics
- `:MdemgGaps run-interviews` — Start interactive gap interview session
- `:MdemgGaps answer <id> <response>` — Answer interview prompt
- `:MdemgGaps skip <id>` — Skip interview prompt

Interactive interview mode: `:MdemgGaps run-interviews` opens a guided float that presents prompts one at a time and accepts answers inline, sending them via the answer endpoint.

**API Endpoints:** All 12 system endpoints.

#### 4.2.9 Skills Registry (`:MdemgSkills`)

**Subcommands:**
- `:MdemgSkills list` — List registered skills
- `:MdemgSkills register <name>` — Register skill (opens editor for section content)
- `:MdemgSkills recall <name> [query]` — Recall skill knowledge

**API Endpoints:** All 3 skills endpoints.

#### 4.2.10 Hash Verification (`:MdemgHash`)

**Subcommands:**
- `:MdemgHash register [file]` — Register current file or specified file
- `:MdemgHash verify [file]` — Verify file integrity
- `:MdemgHash verify-all` — Verify all registered files
- `:MdemgHash update [file]` — Update hash for changed file
- `:MdemgHash revert [file]` — Revert to known-good version
- `:MdemgHash scan` — Scan for changes
- `:MdemgHash list` — List registered files

**API Endpoints:** All 8 hash-verification endpoints.

### 4.3 Tier 3 — Administrative & Specialized (Phase 3)

Lower-frequency operations and specialized subsystems. All accessible via commands; purpose-built UI only where it meaningfully improves the workflow.

#### 4.3.1 Admin / Space Management (`:MdemgAdmin`)

- `:MdemgAdmin spaces` — List spaces
- `:MdemgAdmin update <space_id> [key=value...]` — Update space settings
- `:MdemgAdmin prune` — Prune orphaned spaces
- `:MdemgAdmin export [--space-id=...] [--path=output.json]` — Export space
- `:MdemgAdmin export-preview` — Preview export
- `:MdemgAdmin import <path>` — Import space

**API Endpoints:** All 6 admin endpoints.

#### 4.3.2 Linear Integration (`:MdemgLinear`)

- `:MdemgLinear issues [--team=... --state=... --assignee=...]` — List/filter issues
- `:MdemgLinear issue <id>` — Read issue detail
- `:MdemgLinear create <title> --team=<id> [--desc=... --priority=...]` — Create issue
- `:MdemgLinear update <id> [field=value...]` — Update issue
- `:MdemgLinear delete <id>` — Delete issue
- `:MdemgLinear comment <id> <body>` — Add comment
- `:MdemgLinear projects` — List projects
- `:MdemgLinear project <id>` — Read project detail
- `:MdemgLinear project-update <id> [field=value...]` — Update project

**API Endpoints:** All 9 linear endpoints.

#### 4.3.3 Cleanup Operations (`:MdemgCleanup`)

- `:MdemgCleanup orphans` — Remove orphaned nodes
- `:MdemgCleanup graph-orphans` — Remove graph-level orphans
- `:MdemgCleanup schedule <cron>` — Schedule cleanup
- `:MdemgCleanup schedules` — List schedules
- `:MdemgCleanup stats` — Cleanup statistics

**API Endpoints:** 5 cleanup endpoints under `/v1/memory/cleanup/*`.

#### 4.3.4 CMS Snapshots & Templates (`:MdemgCMS`)

- `:MdemgCMS snapshots` — List snapshots
- `:MdemgCMS snapshot <id>` — View snapshot
- `:MdemgCMS snapshot-create` — Create snapshot
- `:MdemgCMS snapshot-delete <id>` — Delete snapshot
- `:MdemgCMS snapshot-latest` — View latest snapshot
- `:MdemgCMS snapshot-cleanup` — Run snapshot cleanup
- `:MdemgCMS templates` — List templates
- `:MdemgCMS template <id>` — View template
- `:MdemgCMS template-create` — Create template (opens editor)
- `:MdemgCMS template-update <id>` — Update template
- `:MdemgCMS template-delete <id>` — Delete template

**API Endpoints:** 11 CMS endpoints under `/v1/conversation/snapshot/*` and `/v1/conversation/templates/*`.

#### 4.3.5 Plugin/Module/APE Management (`:MdemgPlugins`)

- `:MdemgPlugins list` — List installed plugins
- `:MdemgPlugins create` — Register new plugin
- `:MdemgPlugins ape-status` — APE status
- `:MdemgPlugins ape-trigger <event>` — Trigger APE event
- `:MdemgPlugins modules` — List modules
- `:MdemgPlugins module-sync <id>` — Sync module

**API Endpoints:** 6 endpoints across plugins, ape, and modules.

#### 4.3.6 File Watcher Control (`:MdemgWatcher`)

- `:MdemgWatcher start [--path=. --space-id=...]` — Start server-side watcher
- `:MdemgWatcher stop` — Stop watcher
- `:MdemgWatcher status` — Watcher status

**API Endpoints:** All 3 filewatcher endpoints.

#### 4.3.7 Observability Dashboard (`:MdemgHealth`)

`:MdemgHealth` opens a comprehensive floating dashboard that aggregates:
- Server health (readyz, healthz)
- Embedding provider health
- Neo4j overview (connection stats, index status)
- Neural sidecar status
- Pool metrics
- Query metrics
- Determinism score
- Cache stats
- Stale edge stats
- Prometheus-formatted metrics (rendered as human-readable summary)

Auto-refreshes every 30 seconds while the float is open.

**API Endpoints:** All 8 observability endpoints + `GET /v1/memory/cache/stats`, `GET /v1/memory/query/metrics`, `GET /v1/memory/edges/stale/stats`, `GET /v1/system/pool-metrics`.

#### 4.3.8 Webhooks (`:MdemgWebhooks`)

- `:MdemgWebhooks linear <payload>` — Manually trigger Linear webhook
- `:MdemgWebhooks generic <source> <payload>` — Trigger generic webhook

These are primarily for testing/debugging. Rarely used directly by developers.

**API Endpoints:** Both webhook endpoints.

#### 4.3.9 Org Reviews (`:MdemgOrgReviews`)

- `:MdemgOrgReviews list` — List flagged observations
- `:MdemgOrgReviews stats` — Review statistics
- `:MdemgOrgReviews flag <obs_id>` — Flag observation for org review
- `:MdemgOrgReviews decide <obs_id> <approve|reject>` — Make review decision

**API Endpoints:** 4 org review endpoints.

#### 4.3.10 Node Operations (`:MdemgNodes`)

- `:MdemgNodes archive <id>` — Archive a node
- `:MdemgNodes unarchive <id>` — Unarchive a node
- `:MdemgNodes delete <id>` — Delete node (with confirmation)
- `:MdemgNodes bulk-archive <id1,id2,...>` — Bulk archive

**API Endpoints:** 4 node operation endpoints.

#### 4.3.11 Edge Operations (`:MdemgEdges`)

- `:MdemgEdges stale-stats` — Stale edge statistics
- `:MdemgEdges refresh` — Refresh stale edges

**API Endpoints:** 2 stale edge endpoints.

#### 4.3.12 Meta-Learning (`:MdemgMeta`)

- `:MdemgMeta promote` — Promote patterns to global space
- `:MdemgMeta consult` — Intent-translated consultation
- `:MdemgMeta suggest` — Get proactive suggestions
- `:MdemgMeta frontiers` — Knowledge frontier detection
- `:MdemgMeta distribution` — Node distribution stats

**API Endpoints:** 5 endpoints across meta-learn, consult, suggest, frontiers, distribution.

#### 4.3.13 Feedback (`:MdemgFeedback`)

- `:MdemgFeedback <content>` — Submit feedback

**API Endpoints:** `POST /v1/feedback`

---

## 5. Keymap Design

All keymaps are configurable. Defaults use `<leader>m` as the prefix (mnemonic: **m**emory).

### 5.1 Default Keymaps

| Keymap | Mode | Action | Command |
|--------|------|--------|---------|
| `<leader>mr` | n, v | Memory recall (query or selection) | `:MdemgRecall` |
| `<leader>ms` | v | Store selection as observation | `:MdemgStore` |
| `<leader>ms` | n | Store freeform observation | `:MdemgStore` |
| `<leader>mv` | n | Validate current buffer against guardrails | `:MdemgValidate` |
| `<leader>mj` | n | Get Jiminy guidance for current context | `:MdemgGuide` |
| `<leader>mR` | n, v | Reflect on topic | `:MdemgReflect` |
| `<leader>mS` | n | Symbol search | `:MdemgSymbols` |
| `<leader>mi` | n | Ingestion picker (codebase, linear, scrape, document, files) | `:MdemgIngest` |
| `<leader>mI` | n | RSIC assessment cycle | `:MdemgRSIC assess` |
| `<leader>mP` | n | Python learning cycle (neural sidecar training) | `:MdemgNeural train` |
| `<leader>mh` | n | Health dashboard | `:MdemgHealth` |
| `<leader>mc` | n | Quick status (statusline detail) | `:MdemgStatus` |
| `<leader>mg` | n | Capability gaps list | `:MdemgGaps list` |
| `<leader>mb` | n | Backup list | `:MdemgBackup list` |
| `<leader>ml` | n | Learning stats | `:MdemgLearning stats` |

### 5.2 Float Window Keymaps (active within floating windows)

| Keymap | Action |
|--------|--------|
| `q` | Close float |
| `<Esc>` | Close float |
| `<CR>` | Select / confirm / drill into detail |
| `r` | Refresh data |
| `y` | Yank selected content to clipboard |
| `?` | Show float-local keymap help |

---

## 6. Configuration Schema

```lua
require("mdemg").setup({
  -- Connection (global fallback — per-buffer instance resolution takes priority)
  endpoint = nil,             -- nil = auto-detect from .mdemg.port per project root
  space_id = nil,             -- nil = auto-detect from .mdemg/config.yaml or dirname
  timeout = 30,               -- HTTP timeout in seconds

  -- Keymaps
  keymaps = {
    enabled = true,           -- Set false to register no keymaps
    prefix = "<leader>m",     -- Global prefix for all keymaps
    -- Individual overrides (set to false to disable specific keymap):
    recall = "r",
    store = "s",
    validate = "v",
    guide = "j",
    reflect = "R",
    symbols = "S",
    ingest = "i",             -- Ingestion picker (codebase, linear, scrape, document, files)
    rsic_assess = "I",        -- RSIC assessment cycle
    neural_train = "P",       -- Python learning cycle (neural sidecar training)
    health = "h",
    status = "c",
    gaps = "g",
    backup = "b",
    learning = "l",
  },

  -- Session lifecycle
  session = {
    auto_create = true,       -- Generate session ID on VimEnter
    auto_consolidate = true,  -- Flush volatile observations on VimLeavePre
    id_prefix = "nvim",       -- Session ID prefix (format: {prefix}-{timestamp}-{hash})
  },

  -- Auto behaviors
  auto = {
    ingest_on_save = true,    -- BufWritePost re-ingestion
    ingest_debounce_ms = 2000,-- Debounce window for rapid saves
    ingest_extensions = nil,  -- nil = use MDEMG defaults, or override list
    health_poll_interval = 30,-- Seconds between health polls (0 = disabled)
    stats_refresh_interval = 300, -- Seconds between full stats refresh
  },

  -- UI
  ui = {
    float_border = "rounded",    -- Border style for floating windows
    float_width = 0.8,           -- Float width as fraction of screen
    float_height = 0.6,          -- Float height as fraction of screen
    float_anchor = "center",     -- "center", "top-right", "bottom-right"
    guide_anchor = "top-right",  -- Jiminy guidance float position
    guide_auto_dismiss = 10,     -- Seconds before guide auto-dismisses (0 = manual)
    use_telescope = true,        -- Use Telescope if available
    use_notify = true,           -- Use nvim-notify if available
    markdown_render = true,      -- Render markdown in result buffers
  },

  -- Statusline
  statusline = {
    enabled = true,
    format = "short",         -- "short" = icon + state, "long" = icon + state + space + nodes
    icons = {
      connected = "✓",
      disconnected = "✗",
      stale = "⚠",
      ingesting = "⟳",
    },
  },

  -- Guardrail
  guardrail = {
    validate_on_write = false,   -- Auto-validate on BufWritePre
    block_on_violation = false,  -- Prevent write if violations found
  },

  -- Logging
  log_level = "warn",         -- "debug", "info", "warn", "error"
})
```

---

## 7. Development Phases & Timeline

### Phase 1 — Foundation + Tier 1 Core (Weeks 1–3)

**Week 1: Infrastructure**
- Project scaffold: directory structure, Makefile, stylua, luacheck
- `config.lua`: configuration schema, defaults, merging logic
- `util/config_reader.lua`: `.mdemg/config.yaml` parser (YAML via `vim.fn.system('yq')` or pure Lua parser)
- `util/instance.lua`: per-project instance resolution (walk up to `.mdemg/`, read `.mdemg.port`, cache per project root)
- `util/space.lua`: space ID resolution from instance config
- `client.lua`: HTTP client with `vim.system()`, per-buffer endpoint from instance resolution, async callbacks, error handling, per-instance connection tracking
- `ui/float.lua`: floating window manager (create, update, close, keymap binding)
- `ui/notify.lua`: notification wrapper with fallback
- `plugin/mdemg.lua`: command registration skeleton
- `auto/session.lua`: session lifecycle — generate ID on VimEnter, consolidate on VimLeavePre
- Tests: client unit tests with mock responses, config tests, instance resolution tests

**Week 2: Tier 1 Features (Part 1)**
- `api/memory.lua`: retrieve, ingest, ingest/files, reflect, stats endpoints
- `api/health.lua`: readyz, healthz, embedding health, stats aggregation
- `actions/recall.lua`: full recall workflow with Telescope/float rendering
- `actions/store.lua`: store from visual selection and normal mode
- `auto/ingest_on_save.lua`: BufWritePost autocmd with debounce
- `ui/statusline.lua`: lualine component, health polling
- `auto/health_poll.lua`: periodic health check background loop
- Tests: API module tests with fixture responses

**Week 3: Tier 1 Features (Part 2)**
- `actions/validate.lua`: guardrail validation workflow
- `actions/guide.lua`: Jiminy guidance integration
- `actions/reflect.lua`: reflection workflow with markdown buffer
- `api/symbols.lua`: symbol search endpoint
- `ui/picker.lua`: Telescope integration (or vim.ui.select fallback)
- `ui/markdown.lua`: basic markdown rendering for result buffers
- `util/treesitter.lua`: context extraction for auto-tagging
- `util/diff.lua`: git diff generation
- Keymap registration, vimdoc
- Tests: integration tests for complete workflows

**Phase 1 Deliverable:** A fully functional plugin covering all 8 Tier 1 features. Developers can recall, store, ingest, validate, get guidance, search symbols, reflect, and monitor status — all without leaving Neovim.

### Phase 2 — Tier 2 Operational Workflows (Weeks 4–6)

**Week 4: Ingestion, Conversation, and Jobs**
- `api/jobs.lua`: SSE streaming consumer for `/v1/jobs/{id}/stream`
- `ui/progress.lua`: async job progress display using SSE data
- Complete `api/memory.lua`: remaining ingestion endpoints (trigger, status, cancel, jobs, codebase)
- `api/conversation.lua`: all 24 conversation endpoints
- `actions/ingest.lua`: multi-source ingestion picker (codebase, linear, scrape, document, files) and job management workflows
- `:MdemgIngest` subcommand tree (codebase, linear, scrape, document, files, trigger, status, cancel, jobs)
- `:MdemgConversation` subcommand tree

**Week 5: System Management**
- `api/constraints.lua`: all 6 constraint endpoints
- `api/learning.lua`: all 6 learning endpoints
- `api/self_improve.lua`: all 11 self-improve endpoints
- `api/backup.lua`: all 7 backup endpoints
- `api/scraper.lua`: all 6 scraper endpoints
- `api/neural.lua`: neural sidecar status, training pipeline trigger, NLI, rerank (5 endpoints)
- `:MdemgConstraints`, `:MdemgLearning`, `:MdemgRSIC`, `:MdemgBackup`, `:MdemgScraper`, `:MdemgNeural` command trees

**Week 6: Knowledge Management**
- `api/system.lua`: all 12 system/gap endpoints
- `api/skills.lua`: all 3 skills endpoints
- `api/hash.lua`: all 8 hash-verification endpoints
- `actions/gaps.lua`: interactive gap interview workflow
- `:MdemgGaps`, `:MdemgSkills`, `:MdemgHash` command trees
- Tests for all Phase 2 features

**Phase 2 Deliverable:** Complete operational coverage. Developers can manage the full MDEMG lifecycle from Neovim: multi-source ingestion pipelines, conversation memory, learning controls, self-improvement cycles, neural sidecar training, backups, scraping, gap analysis, skills, and integrity verification.

### Phase 3 — Tier 3 Admin & Polish (Weeks 7–8)

**Week 7: Remaining Domains**
- `api/admin.lua`: all 6 admin endpoints
- `api/linear.lua`: all 9 linear endpoints
- `api/plugins.lua`: plugins, APE, modules (6 endpoints)
- `api/filewatcher.lua`: all 3 filewatcher endpoints
- `api/webhooks.lua`: both webhook endpoints
- Remaining action modules: org reviews, nodes, edges, meta-learning, feedback
- `:MdemgAdmin`, `:MdemgLinear`, `:MdemgPlugins`, `:MdemgWatcher`, `:MdemgWebhooks`, `:MdemgOrgReviews`, `:MdemgNodes`, `:MdemgEdges`, `:MdemgMeta`, `:MdemgFeedback` command trees

**Week 8: Health Dashboard, Polish, Documentation**
- `ui/float.lua` enhancements: multi-section dashboard layout
- `:MdemgHealth` comprehensive dashboard (aggregates 12+ health/metrics endpoints)
- Vimdoc completion: full help file with all commands, keymaps, and configuration
- README: installation, quickstart, feature overview, screenshots
- Edge case hardening: connection loss recovery, timeout handling, large response pagination
- Performance audit: ensure no blocking calls, minimize poll overhead
- Test coverage sweep

**Phase 3 Deliverable:** Production-ready plugin with complete API coverage, comprehensive documentation, and hardened error handling.

---

## 8. Testing Strategy

### 8.1 Unit Tests

Run via `make test` using [plenary.nvim](https://github.com/nvim-lua/plenary.nvim) test harness or [mini.test](https://github.com/echasnovski/mini.test).

- **client.lua**: Mock `vim.system` to verify request construction, JSON serialization, error handling, timeout behavior, connection state tracking.
- **config.lua**: Merging logic, default resolution, config chain priority.
- **api/*.lua**: Each module tested against fixture JSON responses in `tests/fixtures/responses/`. Verify correct path construction, parameter serialization, response parsing.
- **util/*.lua**: Config reader YAML parsing, space ID resolution, diff generation.

### 8.2 Integration Tests

Require a running MDEMG instance. Gated behind `MDEMG_INTEGRATION=1` environment variable.

- Full recall → store → recall cycle: store an observation, retrieve it, verify content matches.
- Ingest file → symbol search: ingest a Go file, search for a function name, verify it's found.
- Guardrail validation: create a constraint, make a violating change, verify validation catches it.
- Health check aggregation: verify all health endpoints return parseable data.

### 8.3 Linting & Formatting

- `stylua` for consistent Lua formatting (configured in `stylua.toml`)
- `luacheck` for static analysis (configured in `.luacheckrc`)
- CI runs both on every PR

---

## 9. Distribution & Installation

### 9.1 Plugin Managers

```lua
-- lazy.nvim
{
  "reh3376/mdemg.nvim",
  dependencies = {
    "nvim-telescope/telescope.nvim", -- optional
    "rcarriga/nvim-notify",          -- optional
  },
  config = function()
    require("mdemg").setup({})
  end,
}

-- packer.nvim
use {
  "reh3376/mdemg.nvim",
  requires = {
    "nvim-telescope/telescope.nvim", -- optional
    "rcarriga/nvim-notify",          -- optional
  },
  config = function()
    require("mdemg").setup({})
  end,
}
```

### 9.2 Health Check

The plugin registers a Neovim health check (`:checkhealth mdemg`) that verifies:
- Neovim version ≥ 0.10
- `curl` available on PATH
- `.mdemg/` directory found for current project root
- `.mdemg.port` file present and instance endpoint reachable
- Space ID resolved
- Neo4j connected (via readyz)
- Embedding provider healthy
- Neural sidecar status (models loaded or not configured)
- Session ID generated
- Optional dependencies detected (Telescope, nvim-notify, lualine, treesitter)

---

## 10. Risk & Mitigation

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| MDEMG API changes break plugin | Medium | High | Pin to API version in client, UATS specs are the contract, add version check on connect |
| `vim.system()` curl overhead per-request | Low | Medium | Latency is ~5ms for localhost; benchmark and optimize if needed |
| Large retrieval results overwhelm float | Medium | Low | Paginate results, default `top_k=10`, truncate long content with expand-on-select |
| SSE streaming complexity | Medium | Medium | Implement SSE as a line-buffered reader over `vim.system` stdout; fall back to polling if SSE parsing fails |
| Team resistance to mandatory MDEMG adoption | Medium | High | Make the plugin frictionless enough that it's a net productivity gain, not overhead. Ingest-on-save and passive statusline make it invisible until you need it. |
| Treesitter not installed for all languages | Low | Low | Graceful fallback to filetype heuristics for auto-tagging |
| Instance not started for a project | Medium | Low | Clear error message directing to `mdemg start`, statusline shows disconnected state per-instance. `:checkhealth mdemg` diagnoses the issue. |
| Multi-instance port confusion | Low | Medium | Plugin reads `.mdemg.port` written by the running instance — port is always correct. Stale `.mdemg.port` from a dead instance detected by failed health check. |
| Concurrent agent + human edits to same instance | Medium | Low | MDEMG handles concurrent access natively via gRPC inter-agent coordination. Plugin observations are just another input source; no special locking needed. |

---

## 11. Success Criteria

1. **Zero terminal round-trips** for daily memory operations (recall, store, validate, ingest).
2. **Sub-200ms response time** for recall and symbol search on localhost.
3. **100% API coverage**: every one of the 165 endpoints is callable from the plugin.
4. **Passive health awareness**: every developer sees MDEMG connection state without actively checking.
5. **Adoption rate**: 100% of the dev team using the plugin within 2 weeks of release (enforced by policy, enabled by design).

---

## 12. Architectural Decisions (Resolved)

These questions were raised during initial specification and have been resolved based on the MDEMG per-instance architecture.

### 12.1 Session Management → Auto-create on VimEnter

**Decision:** Auto-create a session ID on `VimEnter`, opt-out via config.

**Rationale:** MDEMG instances are initialized per-project via `mdemg init`, which starts the Neo4j container, server daemon, and menubar app. Each instance exposes its own endpoints scoped to that codebase. The conversation API requires a `session_id` on every `observe` and `recall` call. Leaving session management manual would make conversation memory unusable since no developer will remember to call resume before their first observation.

**Implementation:** On `VimEnter`, generate `nvim-{timestamp}-{short_hash}` and store in `vim.g.mdemg_session_id`. On `VimLeavePre`, fire `POST /v1/conversation/consolidate` to flush volatile observations. Session ID visible via `:MdemgStatus`, manually overridable via `:MdemgConversation resume <session_id>`.

### 12.2 Multi-Instance Resolution → Per-buffer, cached on BufEnter

**Decision:** Per-buffer instance resolution based on the nearest `.mdemg/` directory. No multi-space logic within a single instance.

**Rationale:** MDEMG runs as isolated per-project instances — each project directory gets its own Neo4j container, server process, and port. Space IDs are instance-scoped, not a shared concern. When a developer has splits across `~/repos/opc_hub/` and `~/repos/mdemg/`, those are two separate MDEMG instances on different ports. The plugin resolves which instance to talk to by walking up from the buffer's file path to find `.mdemg.port`. The result is cached per project root so the filesystem walk happens only once.

**Implementation:** `BufEnter` autocmd → `util/space.lua:resolve_instance(bufpath)` → cache in `vim.b.mdemg_endpoint` and `vim.b.mdemg_space_id`. The `mdemg-menubar` app manages instance lifecycle independently; the plugin only needs to find and talk to the correct endpoint.

### 12.3 Telescope Integration → Bundled

**Decision:** Bundled within `mdemg.nvim`, not a separate extension.

**Rationale:** The Telescope integration is tightly coupled to the plugin's API modules, result formatting, and action handlers. Splitting it out would create two repos with coordinated versioning for no user benefit. Graceful degradation to `vim.ui.select()` when Telescope is not installed is the correct boundary — optional at runtime, bundled at the source level.

### 12.4 Client-Side Rate Limiting → Not implemented

**Decision:** No client-side rate limiting.

**Rationale:** MDEMG instances run locally. The architecture uses gRPC for inter-instance coordination when multiple agents or instances operate on the same codebase. Local Docker containers handle concurrent access without contention. The only debouncing needed is at the action level (e.g., ingest-on-save already debounced at 2 seconds), not a global request throttle.

### 12.5 Offline Mode → Not implemented

**Decision:** No offline queuing, retry logic, or write buffering.

**Rationale:** MDEMG runs locally in a Docker container started by `mdemg init`. It is never "offline" in the network sense — if the instance isn't responding, it means the instance isn't started, which is a developer action (`mdemg start`), not a transient failure to recover from. The plugin surfaces this clearly: statusline shows disconnected state, error notifications direct to `mdemg start`, and `:checkhealth mdemg` diagnoses the issue. Adding retry/queue complexity for a failure mode that doesn't exist in practice would be wasted effort.

---

## Appendix A: Command Reference Summary

| Command | Tier | Endpoints Covered |
|---------|------|-------------------|
| `:MdemgRecall` | 1 | 1 |
| `:MdemgStore` | 1 | 1 |
| `:MdemgValidate` | 1 | 1 |
| `:MdemgGuide` | 1 | 2 |
| `:MdemgSymbols` | 1 | 1 |
| `:MdemgReflect` | 1 | 1 |
| `:MdemgStatus` | 1 | 5+ |
| `:MdemgIngest` | 2 | 9+ (multi-source) |
| `:MdemgNeural` | 2 | 5 |
| `:MdemgConversation` | 2 | 24 |
| `:MdemgConstraints` | 2 | 6 |
| `:MdemgLearning` | 2 | 6 |
| `:MdemgRSIC` | 2 | 11 |
| `:MdemgBackup` | 2 | 7 |
| `:MdemgScraper` | 2 | 6 |
| `:MdemgGaps` | 2 | 12 |
| `:MdemgSkills` | 2 | 3 |
| `:MdemgHash` | 2 | 8 |
| `:MdemgAdmin` | 3 | 6 |
| `:MdemgLinear` | 3 | 9 |
| `:MdemgPlugins` | 3 | 6 |
| `:MdemgWatcher` | 3 | 3 |
| `:MdemgHealth` | 3 | 12+ |
| `:MdemgCMS` | 3 | 11 |
| `:MdemgWebhooks` | 3 | 2 |
| `:MdemgOrgReviews` | 3 | 4 |
| `:MdemgNodes` | 3 | 4 |
| `:MdemgEdges` | 3 | 2 |
| `:MdemgMeta` | 3 | 5 |
| `:MdemgFeedback` | 3 | 1 |
| **Total** | | **165+** |

---

## Appendix B: MDEMG MCP Tools → Plugin Mapping

The 20 MCP tools map to plugin features as follows. The plugin provides broader access to the underlying API endpoints than the MCP tools do.

| MCP Tool | Plugin Command | Plugin Advantage |
|----------|---------------|-----------------|
| `memory_store` | `:MdemgStore` | Auto-tags from treesitter context, visual selection support |
| `memory_recall` | `:MdemgRecall` | Telescope picker, detail drilldown, yank-to-register |
| `memory_associate` | `:MdemgStore` (via batch) | Richer metadata, not limited to query-based lookup |
| `memory_reflect` | `:MdemgReflect` | Markdown buffer rendering, configurable depth |
| `memory_status` | `:MdemgStatus` / statusline | Passive monitoring, aggregated health from multiple endpoints |
| `memory_symbols` | `:MdemgSymbols` | Jump-to-definition, Telescope integration |
| `memory_ingest_trigger` | `:MdemgIngest codebase` | Multi-source ingestion picker, SSE progress tracking, mode selection |
| `memory_ingest_status` | `:MdemgIngest status` | Live progress bar via SSE |
| `memory_ingest_cancel` | `:MdemgIngest cancel` | Integrated with job list picker |
| `memory_ingest_jobs` | `:MdemgIngest jobs` | Full job management UI |
| `memory_ingest_files` | Auto (ingest-on-save) | Zero-friction, automatic on buffer write |
| `memory_space_freshness` | `:MdemgStatus` / statusline | Passive staleness indicator |
| `validate_changes` | `:MdemgValidate` | Auto-diff generation, optional write-blocking |
| `jiminy_guide` | `:MdemgGuide` | Auto-context extraction, positioned float |
| `linear_*` (6 tools) | `:MdemgLinear` | Full CRUD, project support, not just issues |
| — | `:MdemgNeural train` | Neural sidecar training pipeline (no MCP equivalent) |
| — | `:MdemgRSIC assess` | RSIC assessment cycle (no MCP equivalent) |
| — | 140+ additional endpoints | Everything the MCP server doesn't expose |
