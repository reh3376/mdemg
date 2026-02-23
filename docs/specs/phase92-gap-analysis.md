# Phase 92: Full System Gap Analysis — Deployable MDEMG Package

**Status**: Complete
**Priority**: Critical
**Date**: 2026-02-23
**Depends On**: Phase 91 (all prior phases complete)
**Related**: `AGENT_HANDOFF.md` → Phase Registry, `docs/development/DEVELOPMENT_ROADMAP.md`

---

## Purpose

Phase 100 is the end goal: a deployable MDEMG package that developers install repo-level into their existing codebases. This phase produces a comprehensive gap analysis identifying everything needed between now (Phase 91 complete) and Phase 100 (deployable package).

**No code changes** — this phase produces this document and updates `AGENT_HANDOFF.md` with the full Phase 93-100 roadmap.

### Phase 100 End Vision

```bash
brew install mdemg        # or curl installer
cd my-project
mdemg init                # creates .mdemg/, configures, starts services
mdemg ingest .            # indexes the codebase
# IDE picks up MCP automatically
# CMS works, RSIC runs, specs available
```

**Target audience**: Engineers and developers who want persistent, project-level memory for their AI coding agents. This is a developer tool, not an end-user product.

### Key Value Propositions

- Persistent org/SME-specific knowledge (not in LLM training data)
- Conversation Memory System (CMS) for session continuity
- Integration services (IDE, webhook, plugin system)
- Specification test frameworks (UATS, UPTS, etc.)
- RSIC self-improvement cycle for autonomous quality maintenance

---

## Gap Analysis Summary

| # | Gap | Severity | Effort | Phase |
|---|-----|----------|--------|-------|
| 1 | Distribution & Installation | CRITICAL | L | 98 |
| 2 | Unified CLI | CRITICAL | XL | 93 |
| 3 | Project Initialization | CRITICAL | L | 94 |
| 4 | Database Management | CRITICAL | L | 95 |
| 5 | Configuration Simplification | HIGH | L | 94 |
| 6 | Embedding Provider | HIGH | M | 95 |
| 7 | Schema & Migration Management | HIGH | M | 95 |
| 8 | IDE Integration | MEDIUM | M | 96 |
| 9 | Developer Onboarding | MEDIUM | M | 99 |
| 10 | Process Lifecycle | HIGH | M | 97 |
| 11 | Cross-Platform Build | HIGH | L | 98 |
| 12 | Repo Integration | MEDIUM | S | 96 |
| 13 | Test Framework Portability | LOW | M | 99 |
| 14 | Security & Auth | MEDIUM | M | 97 |
| 15 | Upgrade Path | MEDIUM | S | 98 |

---

## Gap 1: Distribution & Installation

**Severity**: CRITICAL | **Effort**: L | **Phase**: 98 | **Depends On**: Gap 2 (Unified CLI)

### Current State

- No installer, no package manager formula, no release automation.
- CI builds 3 binaries (ubuntu-latest only) but produces no downloadable artifacts.
- CGO dependency (tree-sitter C library) complicates cross-compilation.
- Users must `git clone` and `go build` to get a working binary.

### Required State

- `brew install mdemg` via Homebrew tap.
- `curl -sSL https://get.mdemg.dev | sh` one-line installer.
- GitHub Releases with pre-built binaries for darwin/arm64, darwin/amd64, linux/amd64.
- Release automation triggered by git tag push.

### Gap Details

The entire distribution pipeline is absent. There is no goreleaser config, no Homebrew formula, no installer script, and no GitHub Release workflow. The CGO dependency on tree-sitter adds complexity to cross-compilation — either Zig CC or Docker multi-arch builds are needed.

### Estimated Work

- goreleaser configuration with CGO cross-compile
- Homebrew tap repository + formula
- Shell installer script with platform detection
- GitHub Actions release workflow on tag push

---

## Gap 2: Unified CLI

**Severity**: CRITICAL | **Effort**: XL | **Phase**: 93 | **Depends On**: Nothing — this is the foundation

### Current State

- 12 separate binaries under `cmd/`:
  - `server` — main MDEMG server
  - `mcp-server` — MCP tool server for IDEs
  - `ingest-codebase` — codebase ingestion CLI
  - `consolidate` — consolidation CLI
  - `decay` — edge weight decay CLI
  - `prune` — edge pruning CLI
  - `extract-symbols` — symbol extraction CLI
  - `watch` — file watcher binary
  - `space-transfer` — space transfer CLI
  - `plugin-scaffold` — plugin scaffolding
  - `plugin-validate` — plugin validation
  - `reset-db` — database cleanup
- No CLI framework — raw `flag` package throughout.
- No umbrella `mdemg` command. Each binary is invoked separately.
- Inconsistent flag naming and help text across binaries.

### Required State

- Single `mdemg` binary with Cobra subcommands:
  - `mdemg serve` — start the server (foreground)
  - `mdemg init` — initialize a project
  - `mdemg ingest` — ingest codebase
  - `mdemg mcp` — start MCP server
  - `mdemg consolidate` — run consolidation
  - `mdemg decay` — run edge decay
  - `mdemg prune` — run edge pruning
  - `mdemg status` — show system status
  - `mdemg version` — show version info
  - `mdemg config` — config management (wizard, show, validate)
  - `mdemg db` — database management (start, stop, migrate, reset)
  - `mdemg plugin` — plugin management (scaffold, validate, list)
  - `mdemg space` — space management (list, export, import, transfer)
- Consistent flag naming, help text, and error messages.
- Old binaries remain temporarily for backward compatibility but marked deprecated.

### Gap Details

This is the largest gap and the foundation for all subsequent phases. Every other packaging feature (init, db management, daemon mode, config wizard) depends on having a unified CLI entry point. The 12 existing binaries each have their own `main.go` with `flag` package parsing. Migration strategy: create `cmd/mdemg/main.go` with Cobra, move existing logic into `internal/cli/` subpackages, keep old binaries as thin wrappers during transition.

### Estimated Work

- Cobra framework setup with root command
- 13+ subcommands migrated from existing binaries
- Shared flag groups (--neo4j-uri, --space-id, etc.)
- Global flags: --verbose, --quiet, --config, --format (json/text)
- Shell completion generation (bash, zsh, fish)

---

## Gap 3: Project Initialization

**Severity**: CRITICAL | **Effort**: L | **Phase**: 94 | **Depends On**: Gap 2 (Unified CLI), Gap 5 (Config)

### Current State

- No `.mdemg/` directory convention.
- No `mdemg init` command.
- Only `.mdemg.port` file for port discovery (created at runtime by server).
- Git hooks exist in `scripts/` but require manual installation.
- No project-level configuration file.

### Required State

- `mdemg init` creates:
  - `.mdemg/config.yaml` — project-level configuration
  - `.mdemgignore` — patterns to exclude from ingestion (seeded from `.gitignore`)
  - `.mdemg/.gitkeep` — ensures directory is tracked
- Optionally:
  - Installs git hooks (post-commit for incremental ingestion)
  - Detects Neo4j and Ollama availability
  - Writes IDE config files (`.cursor/mcp.json`, `.vscode/settings.json`, `.claude/mcp.json`)
- Interactive wizard mode for first-time setup.
- Non-interactive mode with sensible defaults for CI.

### Gap Details

External developers have no entry point for "start using MDEMG in my project." The init command needs to detect the local environment (what's available), create configuration, and guide the user through initial setup. This is the first thing a new user runs after installation.

### Estimated Work

- `mdemg init` command with interactive prompts
- `.mdemg/` directory structure definition
- `.mdemgignore` template generation
- Environment detection (Neo4j, Ollama, ports)
- IDE config file generation

---

## Gap 4: Database Management

**Severity**: CRITICAL | **Effort**: L | **Phase**: 95 | **Depends On**: Gap 2 (Unified CLI)

### Current State

- Hard dependency on external Neo4j Docker container.
- 17 Cypher migrations applied manually via `cypher-shell` piped commands.
- Server fatally exits on schema version mismatch (`REQUIRED_SCHEMA_VERSION` env var).
- Docker Compose allocates 12GB page cache (`NEO4J_server_memory_pagecache_size=12g`) — not developer-friendly.
- No automated migration runner in Go.

### Required State

- `mdemg db start` launches a lightweight Neo4j container (1GB heap, 512MB page cache).
- `mdemg db stop` / `mdemg db status` for lifecycle management.
- `mdemg db migrate` auto-applies pending migrations from `migrations/V*.cypher`.
- `mdemg serve --auto-migrate` applies migrations on startup.
- Schema version derived from migration file count (remove manual `REQUIRED_SCHEMA_VERSION`).
- `mdemg db shell` opens cypher-shell connected to the managed instance.

### Gap Details

The manual migration workflow (`for f in migrations/V*.cypher; do ... done`) is error-prone and undiscoverable. The 12GB page cache allocation makes it hostile to run on a development machine alongside other work. A Go-native migration runner that reads `migrations/V*.cypher` files, checks `SchemaMeta` version, and applies pending changes is essential. The server's fatal exit on version mismatch should become a helpful error message: "Run `mdemg db migrate` to upgrade from schema v15 to v17."

### Estimated Work

- Go-native migration runner (read files, apply, update SchemaMeta)
- Docker container management (start/stop/status via Docker SDK or CLI)
- Lightweight Neo4j profile (1GB/512MB)
- Auto-migrate flag on serve
- Remove REQUIRED_SCHEMA_VERSION in favor of derived version

---

## Gap 5: Configuration Simplification

**Severity**: HIGH | **Effort**: L | **Phase**: 94 | **Depends On**: Gap 2 (Unified CLI)

### Current State

- 269 config struct fields in `internal/config/config.go` (~1600 lines).
- ~160 environment variables, only 4 truly required (NEO4J_URI, NEO4J_USER, NEO4J_PASS, REQUIRED_SCHEMA_VERSION).
- `.env`-only configuration — no YAML, no profiles, no wizard.
- `.env.example` is stale (references schema version 4, actual is 17).
- No configuration validation beyond type parsing.

### Required State

- `mdemg.yaml` as primary config format with grouped sections:
  ```yaml
  server:
    port: 9999
    host: 0.0.0.0
  neo4j:
    uri: bolt://localhost:7687
    user: neo4j
    password: testpassword
  embedding:
    provider: ollama
    model: nomic-embed-text
  ```
- Layered configuration: defaults < config file < .env < env vars < CLI flags.
- Profiles: `dev` (minimal, sensible defaults) vs `production` (full config).
- `mdemg config wizard` for interactive setup.
- `mdemg config show` to display effective configuration.
- `mdemg config validate` to check configuration validity.

### Gap Details

The 160+ env vars are overwhelming for new users. Most have sensible defaults and never need changing. The config file should expose a curated set of ~20 commonly-adjusted settings in YAML, with the full 160+ available via env var override for advanced users. The stale `.env.example` actively misleads new developers.

### Estimated Work

- YAML config parser with section grouping
- Layered config resolution (file < env < flags)
- Profile system (dev/production)
- Config wizard (interactive prompts)
- Config show/validate commands
- Update `.env.example` or replace with config template

---

## Gap 6: Embedding Provider

**Severity**: HIGH | **Effort**: M | **Phase**: 95 | **Depends On**: Gap 5 (Config)

### Current State

- Two providers: OpenAI (`text-embedding-3-small`, 1536d) and Ollama (768d).
- Optional — server starts without embedder but CMS returns 503 on embedding-dependent endpoints.
- No bundled model. No offline fallback.
- Ollama requires separate process installation + model download (`ollama pull nomic-embed-text`).
- CI has no embedding provider — 25 UATS specs tagged `embedding_required` are best-effort only.

### Required State

- `mdemg init` detects Ollama availability, offers to configure it.
- `mdemg embeddings check` validates the configured provider is reachable and functional.
- Graceful degradation: when no embedder is configured, clearly warn but allow non-embedding features to work (CMS observations, graph queries, admin operations).
- Documentation: minimum viable setup = Ollama + `nomic-embed-text` (free, local, no API key).
- Future: consider bundling a small embedding model or providing `mdemg embeddings setup` that auto-installs Ollama + model.

### Gap Details

The embedding provider is a hard dependency for core features (semantic search, consolidation, CMS recall). Without one, ~40% of the API returns 503. For a deployable package, we need clear guidance and tooling to get an embedder running with minimal friction. The path of least resistance is Ollama + nomic-embed-text (free, local, no API key required).

### Estimated Work

- Provider detection and validation command
- Init integration for embedding setup
- Graceful degradation improvements
- Documentation for supported providers
- Optional: `mdemg embeddings setup` automation

---

## Gap 7: Schema & Migration Management

**Severity**: HIGH | **Effort**: M | **Phase**: 95 | **Depends On**: Gap 2 (Unified CLI), Gap 4 (Database Management)

### Current State

- 17 migration files in `migrations/V0001__*` through `V0017__*`.
- Schema version tracked in `SchemaMeta` Neo4j node.
- No Go-native migration runner — requires external `cypher-shell`.
- Server fatally exits on schema version mismatch.
- Manual `cypher-shell` application is the only migration path.

### Required State

- Go-native migration runner reads `migrations/V*.cypher` files.
- Applies pending migrations in order, updates `SchemaMeta`.
- `mdemg db migrate` CLI command.
- `mdemg db migrate --status` shows current vs available version.
- Auto-migrate flag on `mdemg serve --auto-migrate`.
- Helpful error on version mismatch: "Run `mdemg db migrate` to upgrade from v15 to v17."
- Migration runner reuses existing Neo4j driver connection (no `cypher-shell` dependency).

### Gap Details

This is closely tied to Gap 4 (Database Management) but focuses specifically on the migration runner code. The Go implementation needs to: (1) scan migration directory for `V*.cypher` files, (2) parse version numbers from filenames, (3) read current version from `SchemaMeta`, (4) apply each pending migration as a transaction, (5) update `SchemaMeta` after each successful migration.

### Estimated Work

- Migration runner implementation in `internal/db/migrate.go`
- CLI integration via `mdemg db migrate`
- Server startup integration (auto-migrate option)
- Error message improvements for version mismatch
- Remove manual `REQUIRED_SCHEMA_VERSION` env var

---

## Gap 8: IDE Integration

**Severity**: MEDIUM | **Effort**: M | **Phase**: 96 | **Depends On**: Gap 3 (Project Init), Gap 2 (Unified CLI)

### Current State

- MCP server exists (`cmd/mcp-server/`) with 20 tools, stdio mode.
- Port discovery via `.mdemg.port` file.
- Claude Code hooks are gitignored (`.claude/hooks/` in `.gitignore`).
- No auto-discovery mechanism for IDEs.
- No VS Code extension.
- Manual IDE configuration required (copy JSON snippets).

### Required State

- `mdemg init` writes IDE config files:
  - `.cursor/mcp.json` — Cursor IDE MCP configuration
  - `.vscode/settings.json` — VS Code MCP configuration
  - `.claude/mcp.json` — Claude Code MCP configuration
- MCP server auto-starts with `mdemg serve` (or can run standalone via `mdemg mcp`).
- `mdemg hooks install --ide claude-code` installs Claude Code hooks (optional).
- IDE config files template the correct port and binary path.
- Future: VS Code extension for richer integration.

### Gap Details

Currently, connecting an IDE to MDEMG requires manually creating config files with the correct port, binary path, and tool configuration. The init command should auto-detect installed IDEs and generate the appropriate config files. The MCP server should be launchable as a subprocess of `mdemg serve` rather than requiring a separate process.

### Estimated Work

- IDE detection logic (check for .cursor/, .vscode/, .claude/)
- Config file templates for each IDE
- MCP auto-start integration with serve
- Hook installation command
- Documentation for manual IDE setup

---

## Gap 9: Developer Onboarding

**Severity**: MEDIUM | **Effort**: M | **Phase**: 99 | **Depends On**: All other gaps substantially addressed

### Current State

- Rich internal documentation (~50 docs) oriented toward MDEMG contributors, not adopters.
- No quickstart guide for external developers.
- No demo mode or sample data.
- No tutorial or walkthrough.
- README.md describes the project from a contributor's perspective.

### Required State

- README rewritten for developer adopters (3-step quickstart: install → init → ingest).
- `docs/quickstart.md` — 10-minute tutorial for developers adding MDEMG to their project.
- `mdemg demo` command that seeds sample data and demonstrates features.
- FAQ document addressing common developer questions.
- Architecture overview diagram for adopters (simplified from contributor docs).
- Example `.mdemg/config.yaml` with inline comments.

### Gap Details

The documentation shift from "MDEMG contributor" to "MDEMG developer adopter" is significant. Contributors need to understand internal architecture; developer adopters need to know how to install, configure, and integrate MDEMG into their existing projects. The quickstart should take a developer from zero to working MDEMG in their project in under 10 minutes.

### Estimated Work

- README rewrite (user-facing perspective)
- Quickstart tutorial document
- `mdemg demo` command with sample data
- FAQ document
- Configuration examples with comments

---

## Gap 10: Process Lifecycle

**Severity**: HIGH | **Effort**: M | **Phase**: 97 | **Depends On**: Gap 2 (Unified CLI), Gap 4 (Database Management)

### Current State

- Server runs as a foreground process (`go run ./cmd/server`).
- Graceful shutdown exists (signal handling).
- Dynamic port allocation with `.mdemg.port` file for discovery.
- No daemonization, no `start`/`stop`/`restart` commands.
- No PID file management.
- No Neo4j lifecycle management (assumed to be running).

### Required State

- `mdemg serve` — foreground mode for development.
- `mdemg start` — background daemon mode with PID file. Manages Neo4j container lifecycle.
- `mdemg stop` — graceful shutdown of daemon and managed Neo4j.
- `mdemg restart` — stop + start.
- `mdemg status` — shows running/stopped state, port, PID, uptime, Neo4j status.
- Optional: macOS `launchd` plist for auto-start on login.

### Gap Details

Developers expect to run `mdemg start` and have it work in the background. The current model of opening a terminal and running `go run ./cmd/server` is a poor developer experience. The daemon needs to manage both the MDEMG server and the Neo4j container as a unit. PID file management, log rotation, and health checks should be included.

### Estimated Work

- Daemon mode with PID file management
- Neo4j container lifecycle integration
- start/stop/restart/status commands
- Log file management
- Optional launchd plist generation

---

## Gap 11: Cross-Platform Build

**Severity**: HIGH | **Effort**: L | **Phase**: 98 | **Depends On**: Gap 2 (Unified CLI)

### Current State

- Go 1.24 project.
- CI builds on ubuntu-latest only.
- CGO dependency on tree-sitter (C library) prevents simple cross-compilation.
- No goreleaser configuration.
- No pre-built binaries.
- Development exclusively on macOS (darwin/arm64).

### Required State

- goreleaser configuration for automated releases.
- Pre-built binaries for:
  - darwin/arm64 (Apple Silicon Mac)
  - darwin/amd64 (Intel Mac)
  - linux/amd64 (Linux servers, CI)
- CGO cross-compilation via Zig CC or Docker multi-arch builds.
- CI workflow: on tag push → goreleaser → GitHub Release with binaries.
- Windows deferred but architecture doesn't preclude it.

### Gap Details

The CGO dependency on tree-sitter is the main complication. Pure Go cross-compilation is trivial (`GOOS=linux GOARCH=amd64 go build`), but CGO requires a C cross-compiler for each target. goreleaser with Zig CC is the proven approach for this. The alternative is Docker multi-arch builds.

### Estimated Work

- goreleaser.yaml configuration
- Zig CC or Docker-based cross-compilation
- CI release workflow (tag → build → release)
- Binary signing (optional, macOS notarization)
- Checksum generation and verification

---

## Gap 12: Repo Integration

**Severity**: MEDIUM | **Effort**: S | **Phase**: 96 | **Depends On**: Gap 3 (Project Init)

### Current State

- Git hook scripts exist in `scripts/` (post-commit incremental ingestion).
- No `.mdemgignore` file convention.
- Exclude patterns via `--exclude` flag with hardcoded defaults in ingest CLI.
- File watcher exists (`internal/filewatcher/`) but requires API call to start.

### Required State

- `.mdemgignore` convention using gitignore syntax.
- `mdemg init` creates `.mdemgignore` seeded from `.gitignore` + MDEMG defaults (node_modules, .git, vendor, etc.).
- `mdemg hooks install` installs git hooks (refactors existing `scripts/` into CLI).
- `mdemg watch` starts file-system watching for the current project (wraps existing file watcher).
- Ingestion reads `.mdemgignore` by default.

### Gap Details

This is largely connecting existing functionality (file watcher, git hooks, exclude patterns) under the unified CLI and making them discoverable. The `.mdemgignore` convention is the only genuinely new feature. The file watcher already works via API — just needs a CLI frontend.

### Estimated Work

- `.mdemgignore` parsing and integration with ingestion
- `mdemg hooks install/uninstall` commands
- `mdemg watch` CLI frontend for file watcher
- Default ignore patterns

---

## Gap 13: Test Framework Portability

**Severity**: LOW | **Effort**: M | **Phase**: 99 | **Depends On**: Gap 2 (Unified CLI)

### Current State

- 9 test frameworks deeply embedded in MDEMG's repository structure:
  - UATS (API), UPTS (Parser), UDTS (gRPC), UBTS (Benchmark), USTS (Security), UOBS (Observability), UOTS (Operations), UAMS (Access), UVTS (Visual)
- Python runners require `requests` + `jsonpath-ng` dependencies.
- Framework specs are not packaged for external adopters.
- Runners reference internal paths.

### Required State

- UATS runner bundled with `mdemg` binary or installable via `pip install mdemg-test-runner`.
- `mdemg test run` CLI command to execute specs.
- Spec files extensible by adopters (e.g., `mdemg test init` creates a UATS spec template).
- Documentation for adopters on writing and running custom specs.
- At minimum: UATS framework portable. Other frameworks stay internal.

### Gap Details

The UATS framework is the most valuable for adopters — they can write contract tests for their MDEMG API usage. The Python runner is the main portability concern. Either bundle it as a Go subprocess or provide a Go-native runner for basic spec execution.

### Estimated Work

- UATS runner packaging (pip or embedded)
- `mdemg test` CLI subcommand
- Spec template generation
- Adopter documentation
- Go-native UATS runner (optional, for zero-dependency option)

---

## Gap 14: Security & Auth

**Severity**: MEDIUM | **Effort**: M | **Phase**: 97 | **Depends On**: Gap 5 (Config)

### Current State

- Auth system exists (API key, JWT, SAML) but disabled by default.
- TLS disabled by default.
- Secrets stored in `.env` files (plaintext on disk).
- No keychain integration.
- Security hardening complete (gosec, gitleaks, error sanitization) — Phase 50.

### Required State

- `mdemg.yaml` for non-sensitive config; secrets via platform keychain.
- `mdemg config set-secret openai-key` uses macOS Keychain / Linux secret-tool.
- `.env` included in `.mdemgignore` by default (prevent accidental ingestion of secrets).
- Security model documentation (what's authenticated, what's not, threat model).
- TLS auto-configuration for production profile.

### Gap Details

For a local developer tool, the security bar is different from a production service. The main concern is secret management — API keys for OpenAI/Ollama shouldn't live in plaintext `.env` files that might be committed to git. Platform keychain integration (macOS Keychain, Linux secret-tool) is the standard approach. Auth (API key, JWT) should remain optional but documented for team deployments.

### Estimated Work

- Keychain integration (macOS Keychain, Linux secret-tool)
- `mdemg config set-secret` / `get-secret` commands
- Security model documentation
- TLS configuration for production profile
- `.env` protection in `.mdemgignore`

---

## Gap 15: Upgrade Path

**Severity**: MEDIUM | **Effort**: S | **Phase**: 98 | **Depends On**: Gap 1 (Distribution), Gap 7 (Schema Management)

### Current State

- No versioning scheme beyond git tags.
- No `mdemg version` command.
- No release channels (stable, beta).
- No auto-upgrade mechanism.
- Config is append-only (backward compatible by convention).

### Required State

- SemVer with build-time embedding (`go build -ldflags "-X main.version=..."`)
- `mdemg version` shows version, commit, build date, Go version.
- `mdemg upgrade` self-update command (download latest binary, replace in place).
- Schema migration integrated with version upgrades (auto-migrate on version bump).
- Config migration from `.env` to `mdemg.yaml` (one-time migration tool).

### Gap Details

Version management is straightforward with Go's `ldflags` injection. The self-update mechanism is well-solved by libraries like `go-selfupdate`. The config migration (`.env` → YAML) is a one-time conversion tool for existing users transitioning from the developer setup to the packaged version.

### Estimated Work

- Build-time version injection
- `mdemg version` command
- Self-update mechanism
- Config migration tool
- Release channel support

---

## Phase Dependency Graph

```
Phase 92 (This Phase — Gap Analysis Document)
    |
    v
Phase 93 (Unified CLI Foundation) ──────────────────────────────┐
    |                                                            |
    +──> Phase 94 (Config Simplification + Project Init)         |
    |        |                                                   |
    |        +──> Phase 96 (IDE + Repo Integration)              |
    |        |                                                   |
    |        +──> Phase 95 (Database + Embedding + Migrations)   |
    |                 |                                          |
    |                 +──> Phase 97 (Process Lifecycle + Security)|
    |                          |                                 |
    |                          +──> Phase 98 (Build + Release + Upgrade)
    |                                   |
    |                                   +──> Phase 99 (Onboarding + Polish)
    |                                            |
    |                                            +──> Phase 100 (Deployable Package)
```

## Phase Summary Table

| Phase | Name | Scope | Effort | Deps |
|-------|------|-------|--------|------|
| **92** | Gap Analysis | This document + AGENT_HANDOFF update | S | None |
| **93** | Unified CLI Foundation | Cobra CLI, merge 12 binaries into `mdemg` | XL | None |
| **94** | Config + Project Init | YAML config, profiles, `mdemg init`, `.mdemg/` | L | 93 |
| **95** | Database + Embedding + Migrations | Go migration runner, managed Neo4j, embedder validation | L | 93 |
| **96** | IDE + Repo Integration | MCP auto-config, .mdemgignore, hooks, file watching | M | 94 |
| **97** | Process Lifecycle + Security | Daemon mode, start/stop, keychain secrets | M | 95 |
| **98** | Cross-Platform Build + Release | goreleaser, Homebrew, curl installer, self-update | L | 97 |
| **99** | Onboarding + Polish | README rewrite, quickstart, demo mode, test portability | M | 98 |
| **100** | Deployable Package (Mac) | Integration test: brew install → mdemg init → working | S | 99 |

---

## Phase 100 Acceptance Criteria

The following must pass for Phase 100 to be declared complete:

1. **Installation**: `brew install mdemg` succeeds on macOS (arm64 + amd64).
2. **Initialization**: `mdemg init` in a fresh git repo creates `.mdemg/`, detects environment.
3. **Database**: `mdemg db start` launches Neo4j; `mdemg db migrate` applies all schemas.
4. **Ingestion**: `mdemg ingest .` indexes the codebase into the graph.
5. **Server**: `mdemg start` runs server + MCP in background.
6. **IDE**: Cursor/VS Code/Claude Code can discover and use MCP tools.
7. **CMS**: `POST /v1/conversation/observe` and `POST /v1/conversation/resume` work.
8. **Retrieval**: `POST /v1/memory/retrieve` returns relevant results.
9. **RSIC**: Self-improvement cycle runs on schedule without errors.
10. **Upgrade**: `mdemg upgrade` self-updates to latest version.

---

## Documents Accessed

- `AGENT_HANDOFF.md` — Phase Registry, Phase Artifact Index, phase descriptions
- `CHANGELOG.md` — Unreleased section structure
- `docs/development/DEVELOPMENT_ROADMAP.md` — Feature tracks, phase structure
- `docs/specs/phase90-rsic-conformance-ci-gating.md` — Latest phase spec format
- `docs/specs/phase91-rsic-observability-operations.md` — Latest phase spec format
- `docs/development/RSIC_GAP_ANALYSIS.md` — Gap analysis format reference
- `CLAUDE.md` — Project instructions, architecture overview
- `internal/config/config.go` — Config struct reference (269 fields)
- `cmd/` directory — Current binary inventory (12 binaries)
- `migrations/` directory — Migration file inventory (17 files)
- `README.md` — Current README orientation
- `CONTRIBUTING.md` — Current contributor docs
- `API_REFERENCE.md` — Current API documentation
