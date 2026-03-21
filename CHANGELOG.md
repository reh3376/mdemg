# Changelog

All notable changes to MDEMG will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added (Unreleased)

- **Jiminy Init Wizard Integration**: `mdemg init` wizard now prompts for Jiminy inner-voice configuration (enabled by default). Users select a Jiminy-specific LLM model — defaults to `gpt-5.4-nano` (OpenAI) or `qwen3:8b` (Ollama) for cheap/fast JSON classification tasks. `--defaults`/`--quick` modes auto-configure with recommended settings. All 3 platform installers (macOS Homebrew caveats, Windows post-install, Linux post-install) updated to mention Jiminy. J13-J15 config vars added to jiminy-inner-voice.md. Fixed stale J15 defaults in documentation.
- **Debian Native Packaging (.deb + APT Repository)**: Native `.deb` package generation via GoReleaser nfpms plugin — no external `fpm` dependency needed. Packages include CLI binary, man pages, systemd template units (`mdemg@.service`, `mdemg-rsic@.service`, `mdemg-rsic@.timer`), and UxTS plugin manifest. Docker listed as `recommends` (not hard dependency) so the package installs cleanly on systems where Docker isn't yet configured. APT repository hosted on GitHub Pages (`apt-mdemg` repo) with GPG-signed Release files, automated by `apt-publish.yml` workflow triggered after each release. Users can install via `sudo apt install mdemg` after adding the repository. Sidebar `.deb` (built by Tauri) also included in the same APT repo. AUR PKGBUILD template provided for Arch Linux users (`packaging/aur/PKGBUILD`). Package scripts handle systemd daemon-reload on install, service stop on remove, and `/usr/share/mdemg` cleanup on purge. Flatpak was evaluated and rejected — MDEMG requires Docker for Neo4j, which conflicts with Flatpak's sandbox model.
- **Linux Distribution — Binary Builds + Sidebar Application**: Full Linux platform support with binary builds and desktop companion app. **Phase 1 (Binary Builds):** 4 goreleaser Linux build entries (mdemg + uxts-module, amd64 + arm64) using zig cross-compilation for CGO. Fixed `install.sh` systemd bug (units now bundled in release tarball for curl-pipe installs). Updated beta docs to reflect actual available install methods. **Phase 2 (Sidebar App):** Full Tauri 1.x implementation ported from macOS menubar — Rust backend (7 modules: types, api_client, cli_executor, server_discovery, instance_store, instance_scanner, 30+ commands) + vanilla JS frontend (pub/sub state, 7 tab renderers for Status/Memory/Learning/Neo4j/Config/Logs/RSIC, Catppuccin Mocha UI, polling manager, multi-instance support with auto-discovery). `cargo check` passes cleanly. Submodules: `packaging/mdemg_linux` (installer, systemd, docs), `packaging/mdemg-linux-sidebar` (Tauri app). Supports Ubuntu 20.04+, Debian 11+, Fedora 36+, RHEL 8+, Arch Linux.
- **AutoResearch Integration — Phase AR-1: RSIC Feedback Loop**: Post-cycle re-assessment populates `metrics_after` in `CycleOutcome` by running `Assessor.Assess()` after task execution, enabling before/after metric comparison. Success criteria evaluation checks `RSICTaskSpec.SuccessCriteria` against actual metric deltas — `CriteriaMet` and `CriteriaDetail` fields added to `CycleOutcome`. Auto-rollback for reversible actions (`tombstone_stale`, `graduate_volatile`) that didn't improve metrics via `SnapshotStore.Rollback()`. Prometheus counter `mdemg_rsic_rollbacks_total`. `UpdateCalibration` now only counts tasks as "success" if criteria were met. 8 new unit tests in `calibration_test.go`. Files: `internal/ape/calibration.go`, `internal/ape/cycle.go`, `internal/ape/types_rsic.go`.
- **AutoResearch Integration — Phase AR-2: Jiminy Guidance Effectiveness Tracking**: `POST /v1/jiminy/feedback` endpoint for correlating agent actions with Jiminy guidance. `GuidanceEffectivenessTracker` with LRU cache (TTL-based expiry, configurable via `JIMINY_EFFECTIVENESS_TTL_SEC`). `Guide()` now returns `guidance_id` (UUID) in response for tracking. Outcome classification via text overlap scoring with negation detection: `followed`, `ignored`, `contradicted`, `unknown`. Config: `JIMINY_EFFECTIVENESS_ENABLED` (default: true), `JIMINY_EFFECTIVENESS_TTL_SEC` (default: 1800). 9 new unit tests. Files: `internal/jiminy/effectiveness.go` (new), `internal/jiminy/service.go`, `internal/jiminy/types.go`, `internal/api/handlers_jiminy.go`.
- **AutoResearch Integration — Phase AR-3: LLM-Powered Intelligence**: Three LLM classifiers following the EmergenceNamer pattern (OpenAI/Ollama dual provider, circuit breaker, JSON grammar-constrained output, fail-open). (R3) LLM Reflector for RSIC — analyzes `SelfAssessmentReport` + last 5 cycle outcomes + calibration confidence to produce pattern insights, merged with rule-based results via `deduplicateInsights()`. (J3) LLM Constraint Classifier — replaces keyword-based constraint detection with LLM classification (`must`/`must_not`/`should`/`should_not`/`none`), LRU cache (512 entries), falls back to improved keyword matching that correctly prioritizes "must not" over "must". (C1) LLM Query Classifier — replaces regex-based query type detection with LLM few-shot classification into `code`/`architecture`/`relationship`/`data_flow`/`symbol_lookup`/`generic` with temporal intent, LRU cache (256 entries, SHA256 keyed), multi-label support with most-permissive hint selection. All opt-in via config (`RSIC_LLM_REFLECT_ENABLED`, `CONSULTING_LLM_CONSTRAINTS_ENABLED`, `RETRIEVAL_LLM_CLASSIFY_ENABLED`, all default: false). 12 new config vars. 27 new unit tests across 3 test files. Files: `internal/ape/llm_reflector.go` (new), `internal/ape/self_reflect.go`, `internal/consulting/llm_classifier.go` (new), `internal/consulting/service.go`, `internal/retrieval/query_classifier.go` (new), `internal/retrieval/scoring.go`.
- **AutoResearch Integration Tests**: 8 integration tests in `tests/integration/autoresearch_test.go` covering AR-1 metrics_after/criteria fields, AR-2 guidance_id/feedback roundtrip/validation, AR-3 LLM reflector fail-open behavior.
- **AutoResearch Feature Documentation**: 3 new feature docs — `docs/features/rsic-feedback-loop.md` (AR-1), `docs/features/jiminy-effectiveness-tracking.md` (AR-2), `docs/features/llm-powered-intelligence.md` (AR-3). Updated `docs/features/jiminy-inner-voice.md` with feedback endpoint and effectiveness tracking references.

- **Transfer HTTP API Endpoints (S15 Extension)**: 3 new HTTP endpoints for space export/import via API (previously CLI-only). `POST /v1/admin/spaces/export` — profile-based export with all filter overrides (obs_types, tags, exclude_volatile, only_pinned, min/max_layer, no_observations, no_symbols). `POST /v1/admin/spaces/import` — chunked import with conflict modes (skip, overwrite, error), optional space_id remapping. `GET /v1/admin/spaces/export/preview` — lightweight entity count estimation without data transfer. 3 UATS contract specs (9 variants). 20-step shell acceptance test (`scripts/transfer-acceptance.sh`). 8 new Go integration tests (filter coverage + conflict modes + chunk size control). Makefile: `test-transfer`, `test-transfer-unit`, `test-transfer-integration`, `test-transfer-acceptance` targets.
- **Shareable Knowledge Export/Import (Phase S15)**: Export organization-level CMS knowledge for sharing between MDEMG instances. New `--profile shareable` export profile filters to domain knowledge only (learning, decision, correction, technical_note, insight, preference), excluding volatile/session-specific data. Composable filters: `--obs-types`, `--tags`, `--exclude-volatile`, `--only-pinned`. Import enhancements: `--target-space` remaps space_id, `--consolidate` runs hidden layer pipeline, `--re-embed` regenerates embeddings. Menubar: Knowledge Sharing UI section in Memory tab with export/import buttons, profile picker, and post-import options.
- **Sidecar Quickstart & Hook Enhancements (PR #127 Gap Closure)**:
  - **`mdemg sidecar quickstart`**: One-command onboarding — runs `init → install → up → attach-agent → generate-hooks` sequentially with state-aware skipping and failure reporting. Flags: `--profile`, `--agents`, `--endpoint`, `--dry-run`, `--format json`. New file: `internal/cli/sidecar_quickstart.go`.
  - **`generate-hooks` now produces `prompt-context.sh`**: Previously only generated `session-start.sh`. Now generates both hooks with parameterized endpoint/space_id/session_id from sidecar config. Registers both in `.claude/settings.local.json` via `mergeClaudeSettings()`. The generated `prompt-context.sh` performs CMS recall, Jiminy guidance, and background spreading activation per prompt.
  - **`attach-agent` enables `enableAllProjectMcpServers`**: After writing `.claude/mcp.json`, the claude-code adapter now also sets `enableAllProjectMcpServers: true` in `.claude/settings.local.json` to prevent MCP from being silently disabled. New `--no-settings` flag to skip. New function: `ensureProjectMcpEnabled()`.
  - PR #127 (`feat/claude-code-plugin`) closed — its gaps addressed in the sidecar system instead of a standalone plugin package.
- **Phase Jiminy: Jiminy Inner Voice Guidance**: `POST /v1/jiminy/guide` proactive guidance endpoint for coding agents. Orchestrates 4 knowledge sources in parallel (constraints via `consulting.Suggest()`, correction vector search, contradiction edge queries, frontier node detection) with 6s timeout. Returns structured `GuidanceItem` array plus pre-formatted `═══ JIMINY GUIDANCE ═══` prompt augmentation block for hook injection. MCP `jiminy_guide` tool for IDE integration. Hook integration in `.claude/hooks/prompt-context.sh` (guarded by `JIMINY_ENABLED`). Fixed `LearningEdgeBoost` dead code in scoring pipeline — now computed as `(activation - vectorSim) * beta` when CO_ACTIVATED_WITH edges contribute. New package: `internal/jiminy/` (7 files). Config: 6 `JIMINY_*` env vars (default: `JIMINY_ENABLED=false`). UATS: 2 specs (8 variants, 100% passing).
- **Jiminy J6b-J6e: Hook Distribution & Cross-Platform Support**:
  - **J6b**: Embedded hook templates in binary via `//go:embed`. New package `internal/cli/hook_templates/` (embed.go, prompt-context.sh, session-start.sh). `mdemg hooks install --type claude` installs parameterized hook scripts with `{{SPACE_ID}}`/`{{MDEMG_URL}}` placeholder substitution and registers them in `.claude/settings.local.json`. `mdemg hooks uninstall --type claude` removes them.
  - **J6c**: `mdemg init` wizard auto-installs Claude Code hooks when `.claude/` directory is detected. Auto-installs in `--defaults`/`--quick` mode.
  - **J6d**: Windows PowerShell hook equivalents (`prompt-context.ps1`, `session-start.ps1`) using native `Invoke-RestMethod`/`ConvertFrom-Json`. Platform detection selects `.ps1` on Windows, `.sh` on Unix. PowerShell scripts invoked via `powershell.exe -ExecutionPolicy Bypass` in settings.
  - **J6e**: Settings merge (`mergeClaudeSettings()`) preserves existing user settings when registering hooks. Detects existing MDEMG hooks by command path, updates in-place.
- **ANN Optimization Suite (10 optimizations)**: Comprehensive neural learning improvements across learning, retrieval, consolidation, and API subsystems. 28 new config parameters. Inspired by techniques from autonomous research (Muon optimizer, ResFormer, Gemma soft-capping, sliding window attention).
  - **Tanh Soft-Capping**: Smooth saturation replaces hard weight clamp at `wmax`. Prevents edge weight plateaus at 1.0, allowing continued learning. Formula: `wmax * tanh(w / wmax)`. Applied in both Go helper and Cypher (using Neo4j native `exp()`).
  - **Cautious Decay**: Skip decay for edges reinforced within a configurable window (`LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS`, default 24h). Uses existing `last_activated_at` property. Avoids wasteful decay→re-strengthen cycles.
  - **Multi-Rate Learning**: Context-specific eta multipliers computed in Cypher. Conversation observations learn 2x faster, config↔code edges 1.5x, same-directory nodes 1.2x. Multipliers stack (max ~3.6x), bounded by tanh cap.
  - **Time-Based LR Schedule**: Maturity-aware learning rate scaling. Cold spaces (0 edges) learn at 2x, learning spaces (1-10k) at 1x, warm (10k-50k) at 0.5x, saturated (50k+) at 0.25x. Edge count cached with 5-min TTL.
  - **Squared Activation**: Sharper, sparser activation signals via `β * max(0, activation - floor)²`. Eliminates low-activation noise (floor=0.05) while preserving strong signals.
  - **Local-First Activation Spreading**: Per-hop minimum weight thresholds. Hop 0 requires strong edges (≥0.5), hop 1 moderate (≥0.2), hop 2+ any (≥0.05). Degree normalization uses filtered edge count.
  - **Value Residual Bypass**: Additive bypass bonus for high-confidence vector matches (VectorSim > 0.85). Query-type gated: code queries 1.3x, architecture 0.5x. Max bonus ~0.03 — gentle nudge, not dramatic reranking.
  - **L0 Skip Connections (GROUNDED_BY)**: L5 emergent concepts get direct edges to most representative L0 observations. Prevents grounding loss when intermediate layers merge/prune. New `GROUNDED_BY` edge type with attention weight support.
  - **Negative Result Tracking**: `POST /v1/learning/negative-feedback` endpoint. Weakens CO_ACTIVATED_WITH edges or creates CONTRADICTS edges for rejected results. Caps at 20 nodes per request. Closes the negative-feedback loop in learning.
  - **Frontier Detection**: `GET /v1/memory/frontiers` endpoint. Identifies L3+ nodes with low outgoing degree, sufficient evidence, and no L5 parent — candidates for concept expansion. Read-only, LIMIT-bounded.
- **RSIC Orchestration Reset**: `POST /v1/self-improve/orchestration/reset` endpoint for clearing active cycles, cooldown, and dedupe state. Used for test isolation between UATS runs. Added to Makefile `test-api` target.
- **Phase 104: Active MCP Guardrails**: `POST /v1/memory/guardrail/validate` endpoint for proactive constraint enforcement. 4-step pipeline: diff parsing (regex symbol extraction for Go/Python/JS) → constraint retrieval (vector similarity + keyword match against `role_type: 'constraint'` nodes) → LLM evaluation (OpenAI/Ollama dual provider, Temperature 0.0, circuit breaker protection) → response building (re-validates LLM output against actual constraint types, maps `must`/`must_not` to Block, `should`/`should_not` to Warning). Fail-open on any pipeline error (returns Pass with warning). MCP `validate_changes` tool for IDE integration. New package: `internal/guardrail/` (6 files). Config: 6 `GUARDRAIL_*` env vars (default: `GUARDRAIL_ENABLED=false`). Closes Gap 4 from Cognitive Intelligence Gap Analysis.
- **Phase 103b: Emergence Model Evaluation & MLX Server Integration**: `LLM_ENDPOINT` env var decouples LLM text-generation endpoints from embedding endpoints (`EffectiveLLMEndpoint()` falls back to `OPENAI_ENDPOINT` if unset). Ollama `format` JSON schema parameter for grammar-constrained output — eliminates invalid JSON regardless of model quality. UETS (Universal Emergence Test Specification) framework: schema, 8 model specs (qwen2.5-72b-mlx, qwen2.5-14b-ollama, qwen3-8b-ollama, llama3.2-3b-ollama, llama3.2-3b-macstudio, llama3.2-3b-fp16-macstudio, llama3.3-70b-ollama, llama3.3-70b-macstudio), Python runner with validate/validate-all/add-hashes/verify-hashes/extract-clusters commands plus `--endpoint` override for remote execution, `num_ctx` config support. 7 cluster fixtures from Neo4j. Baseline: 7/7 passing — `llama3.2:3b` Q4_K_M recommended as default emergence model (fastest latency, top name quality). Updated FRAMEWORK_GOVERNANCE.md and UXTS_FRAMEWORK_MATRIX.md with UETS.
- **Phase 103: Dynamic Emergence**: LLM-driven concept naming for unclassified clusters during consolidation via `enable_dynamic_emergence: true` request flag. Dense `CO_ACTIVATED_WITH` clusters that don't match any hardcoded pattern (concern, config, temporal, UI, comparison, constraint) are sent to an LLM for automatic naming and classification. Creates `:MemoryNode:EmergentConcept` nodes with `role_type: 'dynamic_emergent'` and `proposed_label` from constrained set (pattern, principle, bridge, concern, workflow). Pipeline step at phase 22 (after hardcoded patterns, before dynamic edges). Union-find clustering on behavioral edges with idempotency via `NOT EXISTS` subquery. Fail-open per cluster (LLM errors skip individual clusters, don't abort run). Circuit breaker protection via `openai-emergence`/`ollama-emergence` breakers. 8 config vars (`EMERGENCE_*`), 11 unit tests. Fully backward compatible — existing consolidation unchanged unless emergence explicitly requested. Closes Gap 3 from Cognitive Intelligence Gap Analysis.
- **Phase 101: SME Synthesis Engine**: Optional LLM synthesis for `/v1/memory/consult` via `llm_synthesis: true` request flag. Retrieved graph nodes + user question sent to LLM (OpenAI/Ollama) with a prompt that constrains the model to synthesize ONLY from graph evidence — producing coherent organizational SME narrative with mandatory `(Node: <node_id>)` citations. Three graceful fallback paths: `llm_synthesis=false` (skipped), `SYNTHESIS_ENABLED=false` (nil synthesizer), LLM error (debug populated, response intact). Circuit breaker protection via `openai-synthesis`/`ollama-synthesis` breakers. New `Synthesizer` interface in consulting package. 5 config vars (`SYNTHESIS_*`), 13 new tests (5 service integration + 8 unit). Fully backward compatible — existing responses unchanged unless synthesis explicitly requested.
- **Phase 102: Intent Translation**: LLM-driven query rewriting before vector embedding for `/v1/memory/retrieve`, `/v1/memory/consult`, and `/v1/memory/suggest` via `translate_intent: true` request flag. Conversational queries ("Why do we use Redis?") rewritten into keyword-dense search strings ("Redis session state architecture decision caching") optimized for vector similarity against declarative knowledge graph text. Three graceful fallback paths: `translate_intent=false` (skipped), `INTENT_ENABLED=false` (nil translator), LLM error (fail-open, original query used). Circuit breaker protection via `openai-intent`/`ollama-intent` breakers. Temperature 0.0 for deterministic rewrites. Strict 2s timeout (NFR-1). Original question preserved for Phase 101 synthesis — only embedding input is translated. New `IntentTranslator` interface in retrieval package. 5 config vars (`INTENT_*`), 11 new tests (7 unit + 4 consulting integration). `translated_intent` string exposed in API response for transparency. Fully backward compatible.
- **Phase 97: Process Lifecycle + Secret Management**: `mdemg start/stop/restart/status` for background daemon mode with PID file management (`.mdemg/mdemg.pid`), log file (`.mdemg/logs/mdemg.log`), and auto-start of Neo4j container. `mdemg config set-secret/get-secret/list-secrets` for system keychain integration via `go-keyring` (macOS Keychain, Linux secret-tool, Windows Credential Manager). Config priority updated: defaults → yaml → keychain → .env → env vars → flags. Default `.mdemgignore` now includes `.env` and `.env.*` patterns. New dependency: `github.com/zalando/go-keyring`.
- **Phase 96: IDE + Repo Integration**: `mdemg hooks install/uninstall/list` for standalone git hook management (install with `--force`/`--space-id`, uninstall only MDEMG-managed hooks, list shows hook status). Claude Code MCP config generation (`.claude/mcp.json`) in `mdemg init` when `.claude/` directory is detected. `mdemg serve --mcp` launches MCP server as a subprocess alongside the HTTP server with automatic `MDEMG_ENDPOINT` propagation and graceful co-shutdown. Shared `InstallGitHook()`/`UninstallGitHook()` functions extracted from init.go for reuse.
- **Phase 95: Database + Embedding + Migrations**: Go-native migration runner with `//go:embed` embedded Cypher files — `mdemg db migrate` with `--status`, `--dry-run`, `--migrations-dir` flags. Statement splitter handles `CALL {} IN TRANSACTIONS` blocks and `//` comments. `mdemg db start/stop/status/shell` Docker container management with lightweight dev profile (1GB heap, 512MB page cache). `mdemg embeddings check` performs actual test embedding (reports dimensions and provider status). `mdemg serve --auto-migrate` applies pending migrations on startup. `REQUIRED_SCHEMA_VERSION` auto-detects from embedded migrations if unset. CI simplified: replaced cypher-shell download + shell loop with `./bin/mdemg db migrate`. 10 unit tests for migration runner. Removed dead code (`countMigrations`, `portFromString` from init.go).
- **Phase 94: Config Simplification + Project Init**: `mdemg init` interactive wizard for project scaffolding (generates `.mdemg/config.yaml`, `.mdemgignore`, git hooks, IDE configs). `mdemg config show` displays effective configuration with source annotations (yaml/env/default). `mdemg config validate` checks YAML syntax and probes Neo4j/embedding reachability. YAML-to-env-var bridge: `.mdemg/config.yaml` exposes ~20 curated settings, converted to env vars before `FromEnv()` — zero changes to existing config parsing. Layered priority: defaults → yaml → .env → env vars → flags. `.mdemgignore` gitignore-style patterns applied during `mdemg ingest` file walk. Shared `loadConfig()` helper wired into all CLI commands. Schema version fixed in `.env.example` (4→17). Git hook updated to prefer `mdemg ingest` over legacy `ingest-codebase`.
- **Phase 93: Unified CLI Foundation**: Merged 12 separate Go binaries into single `mdemg` binary using Cobra CLI framework. Command tree: `serve`, `mcp`, `ingest`, `consolidate`, `decay`, `prune`, `extract-symbols`, `watch`, `db reset`, `space <sub>`, `plugin <sub>`, `version`. Shared Neo4j type conversion utilities in `internal/cli/neo4jutil/`. Languages package moved to `internal/languages/`. Old binaries converted to deprecation shims. Build-time version injection via ldflags. CI updated to build and test with unified binary. Makefile: `build-cli` target.
- **Phase 92: Gap Analysis — Deployable MDEMG Package**: Comprehensive gap analysis (`docs/specs/phase92-gap-analysis.md`) identifying 15 gap categories between current state and Phase 100 (deployable `mdemg` package for developers). Phase dependency graph mapping Phases 93-100: Unified CLI (93), Config + Init (94), Database + Embedding (95), IDE + Repo Integration (96), Process Lifecycle + Security (97), Build + Release (98), Onboarding (99), Deployable Package (100). AGENT_HANDOFF.md updated with full Phase 92-100 roadmap.
- **Phase 38: UNTS Hash Verification REST API**: 8 REST endpoints under `/v1/hash-verification/` exposing the UNTS hash verification registry via HTTP. Handlers call Registry/Scanner directly (not through gRPC). Endpoints: register, get, list, verify, verify-all, update, revert, scan. 8 UATS specs with 19 variants (100% pass rate). Config: `UNTS_ENABLED` (default: false), `UNTS_BASE_PATH` (default: "."). Makefile targets: `test-unts`, `test-unts-uats`.
- **Phase 91: RSIC Observability & Operations**: 12 Prometheus metrics (`mdemg_rsic_*`) across cycle, trigger, action, safety, watchdog, and calibration domains. Grafana dashboard with 16 panels across 4 rows (Overview, Cycles, Actions, Watchdog). 8 Prometheus alert rules (cycle failure, force triggers, rejection rate, action failures, safety blocks, low confidence, high decay, duration spikes). Operations Runbook §11 with failure mode playbooks, safe mode instructions, and SLO targets. UATS spec for Prometheus RSIC metric validation.
- **Phase 90: RSIC Conformance & CI Gating**: 6 Go integration tests (`tests/integration/rsic_test.go`), CI UATS pipeline split (core merge-gating vs embedding best-effort), UATS tag filtering (`--include-tag`/`--exclude-tag`), sequential mode for idempotency testing, Make targets (`test-rsic`, `test-rsic-unit`, `test-rsic-integration`, `test-rsic-uats`). Idempotency spec promoted from drafts, 7 draft stubs cleaned up. 109 specs, 180 variants, 100% passing.
- **Phase 89: RSIC Persistence & Multi-Space Correctness**: Write-behind persistence via Neo4j `RSICState` nodes (30s flush goroutine, dirty key tracking). Multi-space compliance (`RSICWatchdogSpaceID` config). DateTime coercion for Neo4j. Health endpoint persistence block. Session identity aggregation via `SessionTracker.GetAllStates()`.
- **Phase 88: RSIC Safety & Policy Enforcement**: Safety validator with blast-radius estimation and protected-space blocking. Dry-run mode with mutation deltas. Rollback support (tombstone/graduate reversible). `SafetyVersion = "phase88-v1"` stamped on all outcomes. 3 UATS specs.
- **Phase 87: RSIC Orchestration Activation**: Trigger source tracking (`manual_api`, `micro_auto`, `session_periodic`, `macro_cron`, `watchdog_force`). Cooldown/dedupe/overlap policy with configurable bounds (`RSIC_TRIGGER_COOLDOWN_SEC`, `RSIC_TRIGGER_DEDUPE_SEC`). Trigger metadata in cycle outcomes, health, and history responses. Macro cron scheduler, session-periodic meso on resume. 3 UATS promoted from drafts.
- **Cross-Space Graph Orphan Cleanup**: `POST /v1/memory/cleanup/graph-orphans` — scans all or specified spaces for zero-edge nodes with scan/consolidate/archive/delete fix actions. Protected space enforcement (mdemg-dev skipped for destructive actions). UATS spec with 6 variants.
- **Phase 49 Complete (LLM Plugin SDK)**: All deliverables verified — plugin scaffolding (`cmd/plugin-scaffold/`), validation framework (`cmd/plugin-validate/`, `internal/plugins/validator.go`), creation API (`POST /v1/plugins/create`, `GET /v1/plugins/{id}`, `POST /v1/plugins/{id}/validate`), capability gap detection (`internal/gaps/`). UATS specs: `plugin_create.uats.json` (6 variants), `capability_gaps.uats.json`, `capability_gaps_full.uats.json` (4 variants), `gap_interviews.uats.json`.
- **Phase 9.4: Plugin-Specific Triggers**: File watcher REST API (start/status/stop), event-driven module updates with `EventDispatcher`, wildcard subscription support. 3 UATS specs, 7 variants.
- **Phase 80: CMS ANN Meta-Cognition**: Server-side anomaly detection on resume/recall, HTTP headers (`X-MDEMG-Memory-State`, `X-MDEMG-Anomaly`), session anomalies endpoint, signal effectiveness endpoint. WatchdogSignalProvider for multi-dimensional monitoring. Hebbian SignalLearner for adaptive enforcement. 3 UATS specs. Config: 4 `METACOG_*` env vars.
- **Phase 76: Neo4j State Monitor**: `GET /v1/neo4j/overview` — consolidated database health, per-space statistics (nodes, edges, layers, health score, staleness, orphans, learning edges), and backup overview. 6 batched Cypher queries. 1 UATS spec with 7 body assertions.
- **Phase 75C: L5 Emergent Layer**: BRIDGES edge type, evidence threshold 3→1, L5 edges with COMPOSES_WITH, L3+ source layer for emergence, co-activation fix, dynamic edges via pipeline. Split pipeline execution (`RunPhaseRange`). New config: `L5SourceMinLayer`.
- **Phase 70: Neo4j Backup & Restore**: Full database dump via `docker exec neo4j-admin` and partial space-level export via `.mdemg` format. Ticker-based scheduler (full weekly, partial daily), retention engine (count/age/storage-based cleanup), restore from full dump. 7 API endpoints under `/v1/backup/`, 7 UATS specs, migration V0013. Config: 11 `BACKUP_*` env vars (default: `BACKUP_ENABLED=false`). E2E verified against live mdemg-dev space (21,033 nodes, 232,434 edges, 101MB backup).
- **Phase 51: Web Scraper Ingestion Module**: Plugin-based web scraping with section chunking, quality scoring, dedup, and user review workflow. 6 API endpoints under `/v1/scraper/`, 6 UATS specs, UPTS-validated MarkdownParser. Config: 8 `SCRAPER_*` env vars (default: `SCRAPER_ENABLED=false`).
- **Diagnostics Framework**: Structured `Diagnostic` struct with severity, code, message, parser, and context fields; `DiagnosticSummary` for aggregate reporting; `TruncateContentWithInfo()` and `NewDiagnostic()` helpers; wired into `walkCodebase` with summary logging
- **9 New Language Parsers**: C# (.cs), Kotlin (.kt, .kts), Terraform/HCL (.tf, .tfvars), Makefile (.mk, Makefile), Protocol Buffers (.proto), GraphQL (.graphql, .gql), OpenAPI (via content detection), Markdown (.md), XML (.xml, .csproj) — all with UPTS specs, test fixtures, and diagnostics support
- **UPTS Evidence Validation**: Structural consistency checks in the Go-native test harness — validates LineEnd consistency, CodeElement ranges, symbol containment, and LineEnd matching against specs; enabled for Go and Rust parsers
- **27 UPTS-Validated Parsers**: All 27 language parsers pass CI validation (100% pass rate) — Go, Python, TypeScript, Rust, Java, C, C++, CUDA, SQL, Cypher, YAML, TOML, JSON, INI, Dockerfile, Shell, C#, Kotlin, Terraform, Makefile, Protocol Buffers, GraphQL, OpenAPI, Markdown, XML, Lua, Scraper Markdown
- **UPTS Summary Document**: `docs/lang-parser/lang-parse-spec/upts/UPTS_SUMMARY.md` — comprehensive parser table with parent-child relationships, pattern coverage, and validation commands

### Fixed (Unreleased)

- **`space copy` infinite loop**: Cypher-based deduplication was unreliable for copy operations (creates new nodes, no natural termination). Replaced with two-phase approach: collect all source node IDs upfront, then batch by explicit ID list (`WHERE src.node_id IN $ids`). Added `:MemoryNode` label to all MATCH clauses for consistency with `delete` and `rename`. Previously caused 14,239 orphaned nodes from a 10-node source.
- **Full backup "database in use" failure**: `runFullBackup()` attempted `neo4j-admin database dump` which requires exclusive database access (incompatible with running MDEMG server). Replaced with logical export by delegating to `runPartialBackup()` with all spaces. Both `full` and `partial_space` backups now produce portable `.mdemg` files that work with a live database. Restore auto-detects format (`.mdemg` logical import vs legacy `.dump` physical restore).
- **Snapshot API plain text error responses**: All 7 snapshot handlers (`handleSnapshots`, `handleSnapshotByID`, `handleListSnapshots`, `handleCreateSnapshot`, `handleGetSnapshot`, `handleDeleteSnapshot`, `handleLatestSnapshot`, `handleCleanupSnapshots`) used `http.Error()` which returns plain text. Replaced 20+ calls with `writeJSON()` for consistent `{"error": "..."}` JSON responses with proper `Content-Type: application/json` header.
- **Linear module 503 error handling**: Unconfigured or unavailable Linear module now returns 503 (Service Unavailable) instead of 500/400. Handler detects gRPC `Unimplemented`/`unknown service` errors and `not configured`/`api_key` error strings. All 8 Linear UATS variants passing.
- **RSIC orchestration state leaking across test runs**: `OrchestrationPolicy.Hydrate()` restores cooldown records from Neo4j on startup. Previous UATS runs left 300s cooldown state, causing 409 on subsequent runs. Fixed by adding `ResetState()` method and calling it in Makefile before UATS runs.
- **UATS deep merge override for missing_space_id variants**: Empty objects `{}` still deep-merge with base request body/query. Fixed `frontier_detection` and `learning_negative_feedback` specs by explicitly setting `"space_id": ""` in variant body/query to override base values.
- **Phase 90: CycleOutcome missing `idempotency_key`**: Added field to struct and all 4 return paths in `RunCycle`. Dedup fast-path response now includes `trigger_source` and `idempotency_key`. `Hydrate()` filters expired trigger records to prevent stale cooldown on server restart.
- **Ingestion whitelist**: `getEnabledLanguages()` now includes all 27 registered parsers (was missing yaml, toml, ini, dockerfile, shell, cuda, cypher + new parsers)
- **OpenAPI parser routing**: YAML parser now skips files containing `openapi:` or `swagger:` markers to ensure OpenAPI parser handles them (Go map iteration order is non-deterministic)
- **Makefile parser `:=` assignment**: Fixed disambiguation logic that incorrectly rejected `:=` variable assignments as target definitions

### Previously Added

- **UPTS Go-Native Test Harness**: `upts_test.go` and `upts_types.go` — validates all language parsers directly via `go test` without external dependencies
- **Phase 9.5: Conflict Resolution & Consistency**: Data integrity during concurrent updates, orphan detection, and edge consistency
  - Version tracking: `version` counter incremented on every MERGE update, archive, and unarchive operation
  - `last_ingested_at` timestamp on every ingest update, distinct from `updated_at`
  - Conflict logging: DEBUG log when a node is updated (update_count > 1) with version and update_count
  - `POST /v1/memory/cleanup/orphans` — Orphan detection endpoint with `list`, `archive`, and `delete` actions; supports `dry_run` mode and `limit` parameter
  - Protected space enforcement: `delete` action blocked on protected spaces (e.g., `mdemg-dev`)
  - `edges_stale` flag: set on nodes when embedding changes during re-ingest
  - `RefreshStaleEdges()` method: refreshes ASSOCIATED_WITH edge weights for stale nodes, propagates staleness to parent hidden nodes
  - Edge refresh wired into consolidation pipeline as Step 6
- **Phase 9.4: Plugin-Specific Triggers**: Event-driven integration layer for external event sources
  - `TriggerEventWithContext()` on APE scheduler — passes `space_id`, `ingest_type`, and other context to APE modules
  - `POST /v1/webhooks/linear` — Linear webhook endpoint with HMAC-SHA256 signature verification, 10s debouncing, and automatic observation ingestion via plugin Parse
  - `cmd/watch` — Standalone file watcher binary using fsnotify; monitors directories for changes and triggers file ingestion via API
  - APE event wiring: `source_changed` and `ingest_complete` events fired after all ingest completion paths (batch, file, codebase)
  - Config: `LINEAR_WEBHOOK_SECRET`, `LINEAR_WEBHOOK_SPACE_ID` environment variables
- **Phase 9.1: Git Commit Hooks**: `--quiet` and `--log-file` CLI flags for `ingest-codebase`; git hook passes `--quiet` by default
- **Phase 9.2: Time-Based Scheduled Sync**: TapRoot freshness tracking (`last_ingest_at`, `last_ingest_type`, `ingest_count`), `GET /v1/memory/spaces/{space_id}/freshness` endpoint, periodic scheduled sync via `SYNC_INTERVAL_MINUTES`, stale space detection, MCP `memory_space_freshness` tool
- **Phase 9.3: User-Triggered Re-Ingestion**: Wired `runIngestJob()` to CLI binary with streaming progress via `--progress-json`
- **File-level re-ingest endpoint**: `POST /v1/memory/ingest/files` for targeted file re-ingestion (sync ≤50 files, background >50)
- **MCP tool `memory_ingest_files`**: Re-ingest specific files from IDE
- **CLI `--progress-json` flag**: Structured JSON progress events on stdout for `ingest-codebase`

### Fixed (Additional)

- **MCP `memory_ingest_trigger` field mismatch**: `source_path` → `path`, `mode` → `incremental`, `exclude_pattern` → `exclude_dirs`

### Deprecated

- **`/v1/memory/ingest-codebase` endpoint**: Superseded by `/v1/memory/ingest/trigger` with superior job tracking; responses include `Deprecation` header
- **Linear CRUD Operations**: Full Create/Read/Update/Delete for issues, projects, and comments via Linear GraphQL API
- **CRUDModule protobuf service**: Generic gRPC service with entity_type dispatch and map fields, reusable by future plugins
- **Linear REST API endpoints**: `/v1/linear/issues`, `/v1/linear/projects`, `/v1/linear/comments` with full HTTP method dispatch
- **Linear MCP tools**: 6 tools for IDE integration — `linear_create_issue`, `linear_list_issues`, `linear_read_issue`, `linear_update_issue`, `linear_add_comment`, `linear_search`
- **Workflow engine**: Config-driven YAML automation with triggers (on-create/update/delete), conditions (eq/neq/contains/changed_to/exists), and actions (add-comment, auto-assign, auto-label, auto-transition, set-field)
- **Plugin additional_services**: Backward-compatible mechanism for modules to declare extra capabilities (e.g., INGESTION + CRUD)
- Edge-Type Attention for query-aware activation spreading
- Query-type detection (symbol_lookup, data_flow, architecture, generic)
- RetrievalHints for fine-grained retrieval control
- Layer-specific temporal decay (L0: 0.05/day, L1: 0.02/day, L2: 0.01/day)
- Hybrid edge strategy with query-aware graph expansion
- Universal Parser Test Schema (UPTS) v1.1 with 16 language parsers passing
- Universal API Test Schema (UATS) v1.0.1 with 41 endpoint specs
- Conversation Memory System (CMS) with hooks and protocols
- MCP server for IDE integration
- Codebase ingestion CLI and API endpoint (`/v1/memory/ingest-codebase`)
- Hidden layer concept abstraction and consolidation
- Hebbian learning loop with co-activation edge creation
- Edge weight decay and pruning CLI commands
- Plugin system with scaffold and validation tools
- CI pipeline with build, test, lint, and Trivy security scanning
- SECURITY.md with vulnerability reporting policy
- CONTRIBUTING.md with development guidelines

### Fixed (Parser and Spec Quality)

- **Parser symbol extraction**: Fixed C, C++, CUDA, SQL, Cypher parsers for correct function name extraction (was extracting parameter names)
- **CUDA multi-line kernel signatures**: Kernel pattern now handles `__global__` functions with parameters spanning multiple lines
- **SQL DEFAULT value parsing**: Parenthesis balancing prevents truncation of function calls like `gen_random_uuid()`
- **Cypher symbol types**: Labels, relationships, constraints, and indexes now emit correct UPTS types
- **C++ `static const` extraction**: Parser now recognizes `static const` and `static constexpr` constants
- **UPTS spec corrections**: Fixed 45 spec authoring errors across C (16), C++ (21), and CUDA (16) specs where auto-generated entries had parameter names instead of function names
- VectorSim floor to prevent spurious learning edges
- Migration files excluded from learning edge creation
- L0-only learning scope to reduce noise
- File extension filter handling for `#symbol` suffix queries
- Duplicate node prevention via idempotent ingestion

### Changed

- Standardized symbol field names to UPTS across codebase
- Reorganized documentation structure

## [FSD-2026-001] — 2026-03-19

### Added

- **Constraint enforcement hook (GAP-01)**: PreToolUse hook blocks/warns on Write/Edit based on guardrail validation
- **Contradiction detection (GAP-02)**: Embedding similarity + negation heuristics, with optional NLI sidecar enhancement
- **Effectiveness feedback persistence (GAP-03/08)**: GUIDANCE_OUTCOME edges, Bayesian confidence evolution for constraints
- **Cross-constraint conflict detection (GAP-04)**: Pairwise conflict scan via embedding similarity + type opposition
- **Dynamic confidence inheritance (GAP-05)**: Constraints inherit detection confidence instead of hardcoded 0.8
- **LLM constraint classification gate (GAP-06)**: NLI sidecar confirms/rejects regex detections
- **Constraint scope filtering (GAP-07)**: File path glob matching limits constraint applicability
- **Determinism score metric (GAP-09/19)**: D = (informed/total) * compliance * coverage
- **Jiminy guidance cache (GAP-10)**: LRU + TTL cache for sub-second repeat queries
- **Configurable dimension weights (GAP-11)**: Semantic/temporal/coactivation weights via config
- **Prompt injection sanitization (GAP-14)**: Strips injection phrases, role lines, code fences, excessive repetition
- **Authority level filtering (GAP-20)**: org_policy > team_standard > preference hierarchy
- **Neural re-ranker Python sidecar (NR-2)**: FastAPI with cross-encoder re-ranking + NLI classification
- **Go neural integration (NR-3)**: HTTP client with circuit breaker for sidecar /rerank endpoint
- **Training data collection (NR-1)**: Async JSONL logging of (query, candidate, score) tuples
- **Python neural training pipeline (NR-4)**: `train.py` (fine-tune cross-encoder from collected JSONL data with configurable epochs, batch size, validation split, checkpoint resume), `evaluate.py` (offline evaluation comparing neural vs LLM re-rank scores, top-k reporting), model versioning with timestamped checkpoint directories, CLI entrypoints `mdemg-neural-train` and `mdemg-neural-evaluate`
- **LLM client deduplication (F21)**: Extracted unified `internal/llmclient/` package (725 lines) consolidating duplicate OpenAI/Ollama HTTP client code spread across 5 packages (`summarize`, `consulting`, `retrieval`, `hidden`, `guardrail`). Single `Client` type with `Complete()` and `CompleteWithUsage()` methods. Ollama returns `tokens=0` (no usage reporting). 16 unit tests in `internal/llmclient/client_test.go`.
- **NR-5 + FSD-Final**: E2E acceptance script (`scripts/fsd-acceptance.sh`, 23 test steps), Docker Compose neural sidecar service (`neural/Dockerfile`, profile `neural`), 6 new Makefile targets (`test-fsd`, `test-fsd-unit`, `test-fsd-integration`, `test-fsd-acceptance`, `build-sidecar`, `test-sidecar-python`), phase spec document (`docs/specs/phase-fsd-constraint-lifecycle.md`, 520 lines)
- 8 new API endpoints, 38 new config parameters (all default disabled), 12 new UATS specs
- 2 Neo4j migrations: V0020 (constraint lifecycle), V0021 (constraint conflicts)

### Changed

- `SanitizeUserContext` now appends "..." when truncating
- Constraint retrieval accepts `trustLevel` parameter for authority-based filtering
- Activation spreading uses configurable dimension weights instead of hardcoded values

### Technical

- 70+ files changed across FSD-2026-001 (33 new + 21 modified in core, plus acceptance, Dockerfile, Makefile, spec doc)
- 171 total UATS specs (up from 159)
- All Go tests passing, 0 lint issues

## [0.1.0] - 2026-01-15

### Added (0.1.0)

- Initial project scaffolding
- Neo4j graph database integration with vector indexes
- Semantic retrieval with embedding-based search (OpenAI, Ollama)
- Graph-based knowledge representation with memory nodes
- Core API server with health, ingest, retrieve, and consolidate endpoints
- Database migration framework (10 idempotent Cypher migrations)
- Docker Compose configuration for Neo4j
- Environment configuration via `.env` with example template
