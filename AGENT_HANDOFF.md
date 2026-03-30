# MDEMG Agent Handoff Document

<!-- markdownlint-disable MD022 MD031 MD032 MD040 MD051 MD058 MD060 -->

**Date:** 2026-03-30
**Branch:** `reh3376_dev01`
**Repository:** `/Users/reh3376/mdemg`
**Purpose:** Complete context for continuing development of the MDEMG framework

<!--
=== AGENT RESUME CONTEXT ===

WHAT IS MDEMG? (Read VISION.md for full philosophy)
MDEMG (Multi-Dimensional Emergent Memory Graph) is a cognitive substrate for AI agents —
the ANN equivalent of a human's internal dialogue. It gives AI agents persistent, emergent
long-term memory where higher-level concepts and relationships arise automatically from
accumulated observations through Hebbian learning. It is NOT a tool — it IS the agent's
memory. When CMS is disconnected, memory is disconnected.

Core architecture: Neo4j graph with native vector indexes, 5-layer emergent hierarchy
(L0 observations → L5 emergent concepts), Hebbian learning edges, temporal decay,
LLM re-ranking, activation spreading, and a full RSIC self-improvement cycle.

Only stores domain-specific, organization-specific, task-specific knowledge —
NOT information LLMs already possess.

PROJECT STATUS: ALL DEVELOPMENT PHASES COMPLETE
- 105 core phases (1-105) — ALL COMPLETE
- 16 sidecar phases (S0-S16) — ALL COMPLETE
- 5 cognitive gap phases (101-105) — ALL GAPS CLOSED
- Phase J17: AI-to-AI Communication Protocol — COMPLETE (5 sub-phases, 3-tier encoding, trust scoring, ML tier prediction)
- Phase J17-PC: J17 Prompt Compression — COMPLETE (5 LLM callers optimized, 5 config vars, 14 new tests)
- Phase RSIC-SK1: Jiminy Guidance Self-Calibration — COMPLETE (3 gap closures, 3 new RSIC actions, pattern #15, SignalLearner wiring)
- Deployable package chain (93-100) — COMPLETE (10/10 criteria pass, v0.2.1+ verified)
- Quality hardening — COMPLETE (195 UATS specs / 224 variants / 318 test cases, 213 Go test files, 0 lint issues)
- ANN Optimization Suite — COMPLETE (10 optimizations, 28 config params)
- AutoResearch Integration — COMPLETE (AR-1 feedback loop, AR-2 effectiveness, AR-3 LLM intelligence)
- FSD-2026-001 Gap Closure — FULLY COMPLETE (21 gaps + NR-1 through NR-5 + F21)
- Debian Native Packaging — COMPLETE (.deb via goreleaser, APT repo, AUR PKGBUILD, APT publish verified)
- Doc Consolidation — COMPLETE (4 user-facing docs centralized in docs/user/)
- J17 Feedback Loop Closure — COMPLETE (state file bridge, hook feedback delivery, control char sanitization, bootstrap codification)
- J17 Protocol Pipeline 12-Break Cascading Fix — COMPLETE (code lookup, trust persistence, cache bypass, threshold sync, live collector wiring, all gauges flowing)
- Prometheus Observability Monitoring — COMPLETE (cache hit metrics, bootstrap RSIC assessment, self-monitoring probe, 4 alert rules)
- Gap Analysis — IN PROGRESS (Phases 1-3 complete + sprint review remediations, Phase 4: GAP-02/18/20/21/26/27 done, claude .md ingestion done)
- PR #215 Remediation Sprint — COMPLETE (gauge dirty flag, TSDB backup service, compose standardization, alert validation, 70/70 Playwright e2e)
- Training Data Collection Sprint — COMPLETE (7 sub-phases: InteractionRecord enrichment, guidance ID correlation, source linkage, privacy scrubber, quality annotation, data CLI, JSONL backup)
- CI: ALL GREEN (push + pull_request + release) as of 2026-03-30
- Latest releases: CLI v0.3.4, menubar v1.8.0, sidebar v0.3.0

WHAT REMAINS TO BE DONE:
=== COMPLETED SINCE LAST HANDOFF (2026-03-28) ===
- ✅ J17 Feedback Loop Closure — State file bridge, hook feedback delivery, control char sanitization, bootstrap codification
- ✅ J17 Protocol Pipeline 12-Break Cascading Fix — Code lookup via content-similarity, TrustStore Neo4j persistence, cache bypass, threshold sync, effectiveness TTL 2h, live collector wiring, metrics snapshot refresh
- ✅ Prometheus Observability — Cache hit metrics, bootstrap RSIC assessment, self-monitoring probe, 4 alert rules
- ✅ TSDB Sprint infrastructure — Live collectors, trend analyzer, Grafana dashboards
- ✅ Training Data Collection Sprint (2026-03-30) — 7 gaps fixed for Qwen3-30B-A3B fine-tuning pipeline: InteractionRecord enrichment (6 fields, migration 005, schema v5), guidance ID correlation (context.WithValue threading), source document linkage (consulting classifier), privacy scrubber (5 regex categories, 12 tests), quality annotation pipeline (Python batch + report), data monitoring CLI (5 subcommands), JSONL backup integration

=== GAP ANALYSIS Phase 4 (in progress) ===
1. ✅ GAP-18: slog migration — ALL waves complete (internal/ + cmd/ + plugins/)
2. ✅ GAP-02: Obsidian vault ingestion — v2 complete (parser, walker, Sync RPC, CI, release packaging, docs)
3. ✅ GAP-26: Module developer tutorial with working example
4. ✅ GAP-20: Graph visualization — Grafana topology dashboard (Neo4j data source, 4 panels, UOTS governed)
5. GAP-13: Windows desktop companion (Tauri from Linux sidebar) — LOW PRIORITY, save for later
6. GAP-14: DBSCAN performance profiling + optimization — LOW PRIORITY, save for later

=== Pre-existing partial phases ===
7. ✅ Phase 47.2 — APE INGEST scheduled sync (freshness tracking + RSIC pipeline wired: assess, reflect, plan, dispatch)
9. TESTING: SSE streaming — accepted limitation (GAP-05: 14 Go unit tests cover streaming; UATS spec documents gap)
10. RESEARCH: AutoResearch integration analysis (docs/development/)

=== LIVE VERIFICATION — COMPLETE (2026-03-28) ===
11. ✅ J17 feedback loop live test — 12-break cascading fix verified: code_coverage=100%, T2 at 80%, compression_ratio=1.714, all gauges flowing
12. ✅ Server-side control char fix — internal/sanitize/controlchars.go strips U+0000-U+001F; perl hook kept as defense-in-depth

=== Gap Analysis — COMPLETED items (Phases 1-3 + partial Phase 4) ===
- Phase 1 (Quick Wins): GAP-06/-07/-12/-15/-17/-25 — doc/config fixes (+ sprint review remediations: GAP-01 plugin bridge, GAP-06 volume migration, GAP-25 Windows README, GAP-30 UATS assertions)
- Phase 2 (UATS): GAP-28/-29/-30/-31 — canonical grammar migration, CI gating
- Phase 3 (Core): GAP-01 (parser fallback), -04 (UVTS inline grading), -05 (SSE docs),
  -09 (pkg/mdemg/ public types), -16 (scope-based auth), -19 (auto-credentials)
- Phase 4 (partial): GAP-21 (UAMS runner), -27 (submodule changelogs)
- Infrastructure: Architecture map generator (scripts/generate_arch_maps.py),
  UITS optimization protection, session-start staleness check
- CI fixes: gosec G702 exclusion, UETS --no-llm flag for CI

- UxTS governance phases 81-85 COMPLETE (reconciliation, UOBS/UOTS convergence, CI gating, UNTS coverage, auth/perf)
- Phase 50 Public Readiness COMPLETE (MIT license exists, SemVer active at v0.3.0, standard Go layout)

REPO STATE:
- Branch: reh3376_dev01 — uncommitted changes from TSDB sprint + J17 feedback loop fix + Training Data sprint (60+ files)
- TSDB schema version: 5 (migration 005: interaction enrichment columns)
- Binary: bin/mdemg (rebuild with: go build -o bin/mdemg ./cmd/mdemg)
- CMS: MDEMG server on localhost:9999, Neo4j via docker compose (volume: mdemg_neo4j_data, 34K+ nodes)
- CRITICAL: ALWAYS use `docker compose up -d neo4j` to preserve CMS data. Never `mdemg db start` for CMS.
- Embedding dimensions: 3072 (text-embedding-3-large). Stub embedder matches at 3072.

KEY DOCUMENTS (read in order):
1. VISION.md — Core purpose, architecture philosophy, success metrics
2. CLAUDE.md — Commands, CMS connection, observation protocol, enforced hooks
3. This file — Phase registry, architecture, known issues
4. docs/development/COGNITIVE_INTELLIGENCE_GAP_ANALYSIS.md — 5 cognitive gaps (all closed)

USER-FACING DOCS (canonical location: docs/user/):
- api-reference.md (3,707 lines), cli-reference.md (2,658 lines)
- cms-rsic-guide.md (1,719 lines), ingestion-guide.md (1,460 lines)
- Submodule stubs redirect to these canonical URLs. Edit ONLY in docs/user/.
-->

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Architecture Summary](#2-architecture-summary)
3. [Environment Setup](#3-environment-setup)
4. [Phase Registry](#4-phase-registry)
5. [Open Work Items](#5-open-work-items)
6. [Governance & Testing](#6-governance--testing)
7. [Known Issues](#7-known-issues)
8. [Quick Reference Commands](#8-quick-reference-commands)

---

## 1. Project Overview

**MDEMG** (Multi-Dimensional Emergent Memory Graph) is a long-term memory system for AI agents, built on Neo4j with native vector indexes. It provides AI agents with the **ANN equivalent of human internal dialog** — persistent cognitive context that survives across sessions.

It stores **task history** and **SME domain knowledge** — NOT general knowledge that LLMs already possess.

### Read First

| Document | Path | Purpose |
|----------|------|---------|
| Vision | `VISION.md` | Core purpose, architecture philosophy, emergent layer design |
| Architecture | `CLAUDE.md` | Commands, directory structure, env vars, retrieval pipeline |
| Dev Roadmap | `docs/development/DEVELOPMENT_ROADMAP.md` | Feature tracks, benchmarks |
| API Reference | `docs/development/API_REFERENCE.md` | All HTTP endpoints |
| User Docs | `docs/user/` | CLI reference, API reference, CMS/RSIC guide, ingestion guide |
| Feature Docs | `docs/features/` | Per-feature documentation (jiminy, teardown, neural, etc.) |

### Technical Invariants

- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (computed in-memory, NEVER persisted)
- **DB writes = learning deltas only** (bounded, no per-request activation writes)

---

## 2. Architecture Summary

### Technology Stack

| Component | Technology | Notes |
|-----------|-----------|-------|
| Graph DB | Neo4j 5.x | Docker: `docker compose up -d neo4j` |
| Backend | Go 1.24 | Service at `cmd/mdemg/main.go` |
| gRPC | Protocol Buffers | `api/proto/*.proto` |
| Embeddings | OpenAI `text-embedding-3-large` (3072d) | Configurable; vector index at 3072d |
| Plugins | Binary sidecar via gRPC Unix sockets | `plugins/*/` |
| Neural Sidecar | Python FastAPI | `neural/` — re-ranking + NLI |

### Key Directories

```
cmd/mdemg/             # Unified CLI binary
internal/
  api/                 # HTTP handlers + middleware
  ape/                 # Active Participant Engine + RSIC
  config/              # Environment-based configuration
  consulting/          # Agent consulting service (consult, suggest, constraints)
  conversation/        # CMS (observe, recall, resume, correct)
  db/                  # Neo4j driver + schema validation + migrations
  guardrail/           # MCP guardrail validation pipeline
  hidden/              # Hidden layer abstraction/consolidation pipeline
  jiminy/              # Jiminy inner voice guidance service
  cli/                 # Unified CLI commands
  learning/            # Hebbian learning (CO_ACTIVATED_WITH)
  llmclient/           # Unified LLM client (OpenAI/Ollama)
  metalearn/           # Global meta-learning (cross-space promotion)
  metrics/             # Prometheus metrics + determinism scoring
  plugins/             # Plugin manager + scaffold
  retrieval/           # Core retrieval pipeline (vector + activation + scoring + cache)
  sanitize/            # Prompt injection sanitization + control char stripping
  scraper/             # Web scraper ingestion
  secrets/             # System keychain integration
  summarize/           # LLM summary service
  symbols/             # Symbol extraction (tree-sitter)
  transfer/            # Space transfer (export/import)
pkg/mdemg/             # Public API types for external Go consumers (GAP-09)
neural/                # Python sidecar (FastAPI, cross-encoder, NLI)
  training/            # Quality annotation pipeline (quality_annotator.py, quality_report.py)
migrations/            # Neo4j Cypher migrations (V0001-V0022)
plugins/               # Plugin binaries (linear, reflection, keyword-booster, uxts)
packaging/             # Submodules: homebrew-mdemg, mdemg-windows, mdemg_linux, apt-mdemg, mdemg-menubar, mdemg-linux-sidebar
scripts/               # generate_arch_maps.py, verify_sidecar_schemas.py, etc.
docs/
  user/                # Canonical user-facing docs (4 files)
  features/            # Feature documentation
  specs/               # Phase specifications
  architecture/maps/   # 10 compact architecture maps for Jiminy context injection
  architecture/        # Architecture docs (00-14 numbered)
  development/         # Dev guides, roadmap, API reference
  api/api-spec/        # UATS + UDTS specs, schemas, runners
  tests/               # UAMS, UETS, UITS, UOBS, UBTS, USTS, UVTS runners + specs
  benchmarks/          # Benchmark results and scripts
```

### Graph Schema

| Label | Purpose |
|-------|---------|
| `:TapRoot` | Singleton per `space_id` |
| `:MemoryNode` | Main memory nodes with 3072-dim embeddings |
| `:Observation` | Append-only events linked to MemoryNodes |
| `:SymbolNode` | Extracted code symbols |

### Retrieval Pipeline (`internal/retrieval/service.go`)

1. **Vector recall** → top-K candidates from `memNodeEmbedding` index
2. **Symbol search** → pattern-match for symbol names
3. **Bounded expansion** → 1-hop fetch with caps (max depth=3)
4. **Spreading activation** → in-memory with decay
5. **Scoring** → vector (0.55) + activation (0.30) + recency (0.10) + confidence (0.05) - hub penalty (0.08) - redundancy (0.12)
6. **Caching** → TTL-LRU cache

### Linux Systemd Architecture

Systemd unit files in `packaging/mdemg_linux/systemd/` are installed to two locations:
- `/etc/systemd/system/` — active units (tarball installs via `install.sh`)
- `/usr/lib/systemd/system/` — active units (`.deb` installs via nfpms)
- `/usr/local/share/mdemg/systemd/` — persisted copies for manual reference/fallback

Install/upgrade/teardown flow:
- `install.sh` → installs to both `SYSTEMD_DIR` and `SHARE_DIR/systemd/`
- `mdemg upgrade` → updates both locations if units already exist, runs `daemon-reload`
- `mdemg teardown --full` → checks both `/etc/systemd/system/` and `/usr/lib/systemd/system/`, cleans `SHARE_DIR/systemd/`

`mdemg.service` ExecStartPre tries `docker start mdemg-neo4j-dev` before `mdemg db start` to prefer existing containers. Uses `Wants=docker.service` (fail-open) instead of `Requires=`.

### Hook Tracking

5 active hooks in `.claude/hooks/` are tracked via `.gitignore` negation patterns (`.claude/*` ignores all, then `!.claude/hooks/<name>` re-includes specific hooks). All 5 have canonical templates in `internal/cli/hook_templates/` (embedded via `//go:embed *.sh *.ps1 *.py`).

---

## 3. Environment Setup

```bash
# Start Neo4j (preserves CMS data volume)
docker compose up -d neo4j

# Build and start server
go build -o bin/mdemg ./cmd/mdemg
./bin/mdemg start --auto-migrate

# Run tests
go test ./internal/... -v                                              # Unit tests
go test -tags=integration ./tests/integration/... -v                   # Integration tests
make test-api BASE_URL=http://localhost:9999                           # UATS contract tests
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...  # Lint

# Key env vars (see .env.example for full list)
NEO4J_URI=bolt://localhost:7687
NEO4J_USER=neo4j
NEO4J_PASS=testpassword
```

---

## 4. Phase Registry

### Status Legend

| Icon | Meaning |
|------|---------|
| ✅ | Complete |
| 🔄 | Partially complete |
| 📋 | Planned |

### Phase Status Table

Every completed phase has a spec doc — see the Spec column for details. Phase descriptions are NOT duplicated here; read the spec.

| Phase | Name | Status | Spec / Key Doc |
|-------|------|--------|----------------|
| 31 | Space Transfer | ✅ | `docs/specs/space-transfer.md` |
| 32 | DevSpace Hub | ✅ | `docs/specs/phase-devspace-hub.md` |
| 33 | Inter-Agent Comms | ✅ | `docs/specs/phase3-inter-agent-comms.md` |
| 34 | Incremental Sync | ✅ | `docs/specs/phase4-incremental-sync.md` |
| 35 | CRDT + Lineage | ✅ | `docs/specs/development-space-collaboration.md` §5 |
| 36 | Observation Forwarding | 📋 | `docs/specs/development-space-collaboration.md` §7 |
| 37 | Agent Health / Presence | ✅ | `docs/specs/development-space-collaboration.md` §8 |
| 38 | UNTS Hash Verification | ✅ | `docs/specs/unts-hash-verification.md` |
| 41 | Space Cleanup | ✅ | `docs/specs/phase1-space-cleanup.md` |
| 42 | Self-Ingest | ✅ | `docs/specs/phase2-self-ingest.md` |
| 43A | CMS Enforcement | ✅ | `docs/specs/phase3a-cms-enforcement.md` |
| 43B | CMS Quality | ✅ | `docs/specs/phase3b-cms-quality.md` |
| 43C | Multi-Agent CMS | ✅ | `docs/specs/phase3c-multi-agent.md` |
| 44 | Linear CRUD | ✅ | `docs/specs/phase4-linear-crud.md` — upgraded: slog, context cancellation, edge emission (U-1/U-2/U-3) |
| 45 | Modular Intelligence | ✅ | 45.1-45.2 ✅, 45.3 cancelled (parser RPC — researched, bad idea), 45.4 ✅ (Obsidian v2), 45.5 ✅ |
| 46 | Symbol Indexing | ✅ | `docs/development/DEVELOPMENT_ROADMAP.md` §8 |
| 46-PR | Dynamic Pipeline Registry | ✅ | `docs/development/REGISTRY.md` |
| 47 | Incremental Updates | ✅ | 47.1-47.5 ✅ — APE INGEST wired via StartScheduledSync + RSIC dispatcher |
| 48 | Query Optimization | ✅ | `docs/development/DEVELOPMENT_ROADMAP.md` §10 |
| 48-SR | CMS Skill Registry | ✅ | `internal/api/handlers_skills.go` |
| 49 | LLM Plugin SDK | ✅ | `docs/development/DEVELOPMENT_ROADMAP.md` §11 |
| 50 | Public Readiness | ✅ | `docs/development/repo-to-public-roadmap.md` |
| 51 | Web Scraper | ✅ | `docs/specs/phase51-web-scraper-ingestion.md` |
| 60 | CMS Advanced II | ✅ | `docs/specs/phase60-cms-advanced-ii.md` |
| 60b | RSIC | ✅ | `docs/development/RSIC_GAP_ANALYSIS.md` |
| 45.5 | Constraint Detection | ✅ | `internal/hidden/constraint_nodes.go` |
| 70 | Neo4j Backup | ✅ | `docs/specs/phase70-neo4j-backup.md` |
| 75 | Relationship Extraction | ✅ | `docs/specs/phase75-relationship-extraction.md` |
| 75C | L5 Emergent Layer | ✅ | `docs/features/l5-emergent-layer.md` |
| 76 | Neo4j State Monitor | ✅ | `docs/features/neo4j-state-monitor.md` |
| 80 | CMS Meta-Cognition | ✅ | `docs/specs/phase80-cms-metacognition.md` |
| 87 | RSIC Orchestration | ✅ | `docs/specs/phase87-rsic-orchestration-activation.md` |
| 88 | RSIC Safety | ✅ | `docs/specs/phase88-rsic-safety-policy-enforcement.md` |
| 89 | RSIC Persistence | ✅ | `docs/specs/phase89-rsic-persistence-multi-space.md` |
| 90 | RSIC Conformance | ✅ | `docs/specs/phase90-rsic-conformance-ci-gating.md` |
| 91 | RSIC Observability | ✅ | `docs/specs/phase91-rsic-observability-operations.md` |
| 92 | Gap Analysis | ✅ | `docs/specs/phase92-gap-analysis.md` |
| 93 | Unified CLI | ✅ | `docs/specs/phase93-unified-cli-foundation.md` |
| 94 | Config + Init | ✅ | `docs/specs/phase94-config-project-init.md` |
| 95 | Database + Migrations | ✅ | `docs/specs/phase95-database-embedding-migrations.md` |
| 96 | IDE + Repo Integration | ✅ | `docs/specs/phase96-ide-repo-integration.md` |
| 97 | Process Lifecycle | ✅ | `docs/specs/phase97-process-lifecycle-security.md` |
| 98 | Build + Release | ✅ | `.goreleaser.yaml`, `.github/workflows/release.yml` |
| 99 | Onboarding | ✅ | `README.md`, `docs/quickstart.md`, `docs/FAQ.md` |
| 100 | Deployable Package | ✅ | 10/10 criteria pass, brew install verified |
| 101 | SME Synthesis | ✅ | `docs/specs/phase101-sme-synthesis.md` |
| 102 | Intent Translation | ✅ | `docs/specs/phase102-intent-translation.md` |
| 103 | Dynamic Emergence | ✅ | `docs/specs/phase103-dynamic-emergence.md` |
| 103b | Emergence Model Eval | ✅ | `docs/tests/uets/` |
| 104 | Active Guardrails | ✅ | `docs/specs/phase104-active-mcp-guardrails.md` |
| 105 | Global Meta-Learning | ✅ | `docs/specs/phase105-global-meta-learning.md` |
| 9.4 | Plugin Triggers | ✅ | `internal/api/handlers_filewatcher.go` |
| Jiminy | Inner Voice | ✅ | `docs/specs/phase-jiminy-guidance.md`, `docs/features/jiminy-inner-voice.md` |
| J7-J12 | Cognitive Guidance | ✅ | `docs/features/jiminy-inner-voice.md` §J7-J12 |
| J16 | Full-Context Input | ✅ | Removed input truncation (200K default), fixed cache key collisions, 30s timeouts. Config: `JIMINY_GUIDANCE_CONTEXT_MAX_CHARS`, `JIMINY_GUIDANCE_OUTPUT_MAX_CHARS`, `JIMINY_EVALUATE_OUTPUT_MAX_CHARS`, `JIMINY_EVALUATE_ITEM_MAX_CHARS` |
| J17 | AI-to-AI Communication Protocol | ✅ | `docs/features/j17-ai2ai-protocol.md` (incl. §7 Control-Loop Optimization — 7 gap fixes, CUIDv2 IDs, control char sanitization, daemon .env fix) |
| J17-PC | J17 Prompt Compression | ✅ | `docs/features/j17-prompt-compression.md` |
| RSIC-SK1 | Jiminy Guidance Self-Calibration | ✅ | 3 RSIC executors, reflection pattern #15, SignalLearner wiring, confidence extension to all types |
| Testing & Quality | Testing & Quality Hardening | ✅ | 5 new UATS specs, UATS runner skipped-count fix, 28 new tests (7 guardrail unit, 16 scraper integration, 5 guardrail integration) |
| J-Init | Init Wizard + Installers | ✅ | `internal/cli/init.go`, `internal/config/yaml_config.go`, `.goreleaser.yaml` |
| S8 | Distribution Pipeline | ✅ | `docs/sidecar/roadmap.md` §S8 |
| S9 | Beta + Public | ✅ | `docs/sidecar/roadmap.md` §S9 |
| S10 | Dynamic Port | ✅ | `docs/sidecar/roadmap.md` §S10 |
| S11 | Sidecar LLM | ✅ | `docs/sidecar/roadmap.md` §S11 |
| S12 | Upgrade/Uninstall | ✅ | `docs/sidecar/roadmap.md` §S12 |
| S13 | Embedding Migration | ✅ | `docs/sidecar/roadmap.md` §S13 |
| S14 | Doc Cleanup | ✅ | `docs/sidecar/roadmap.md` §S14 |
| S15 | Shareable Export | ✅ | `internal/transfer/exporter.go` |
| S16 | Teardown | ✅ | `docs/features/teardown.md` |
| S16b | Removal Wizard | ✅ | `docs/features/teardown.md` |
| D | Validation | ✅ | `docs/architecture/benchmarks/` |
| FSD | Constraint Lifecycle | ✅ | `docs/specs/phase-fsd-constraint-lifecycle.md` |
| NR-4 | Neural Training | ✅ | `docs/features/neural-training-pipeline.md` |
| F21 | LLM Client Dedup | ✅ | `internal/llmclient/` |
| NR-5 | FSD Final | ✅ | `scripts/fsd-acceptance.sh` |
| AR | AutoResearch | ✅ | `docs/features/rsic-feedback-loop.md` |
| Deb | Debian Packaging | ✅ | `.goreleaser.yaml` (nfpms), `packaging/apt-mdemg`, `apt-publish.yml` |
| 81 | UxTS Governance Reconciliation | ✅ | `docs/specs/FRAMEWORK_GOVERNANCE.md`, `docs/development/UXTS_FRAMEWORK_MATRIX.md` |
| 82 | UOBS/UOTS Convergence | ✅ | Authority split defined in FRAMEWORK_GOVERNANCE.md |
| 83 | CI Gate Expansion | ✅ | UATS/UBTS/UPTS/UDTS/UVTS in CI |
| 84 | UNTS Full Coverage | ✅ | Scanner covers all 8 frameworks |
| 85 | Auth/Security/Perf Stabilization | ✅ | USTS+UBTS active, UAMS runner active (GAP-21) |
| 86 | UVTS Activation | ✅ | Runner active with inline grading (GAP-04), CI soft-fail |
| Synergy | Claude Code ↔ MDEMG Optimization | ✅ | `docs/features/synergy-optimization.md` |
| J17-FL | J17 Feedback Loop Closure | ✅ | `docs/features/j17-feedback-loop-closure.md` — state file bridge, hook feedback, control char fix, bootstrap codification |
| J17-FIX | J17 Protocol Pipeline 12-Break Fix | ✅ | 12 cascading breaks: code lookup (content-similarity), TrustStore (Neo4j write-behind), cache bypass, threshold sync, effectiveness TTL, live collector wiring, metrics snapshot refresh. Verified: code_coverage=100%, T2 80%, compression_ratio=1.714 |
| PROM | Prometheus Observability Monitoring | ✅ | `docs/features/prometheus-observability-monitoring.md` — cache hit metrics, bootstrap assessment, self-monitoring, 4 alert rules |
| Gap | Gap Analysis Implementation | 🔄 | Phases 1-3 complete, Phase 4 in progress. Plan: `.claude/plans/mellow-crunching-hopcroft.md` |
| REM | PR #215 Remediation Sprint | ✅ | Gauge dirty flag (TSDB noise reduction), TSDB backup/restore (pg_dump, CLI, scheduler, retention), compose standardization, 21 alert rules validated, 70/70 Playwright e2e |
| DD-SPRINT | Deep-Dive Remediation Sprint | ✅ | 2026-03-29 | SEC-LEAK (56 error leaks sanitized), GAP-16 (RequireScope wired to 14 endpoints), DOC-REM (19 docs remediated), K8S-ALIGN (K8s/Helm + TimescaleDB + neural-sidecar), LLM-LOG (interaction logger), TXN-MGMT (32 session.Run → managed transactions) |
| TD-SPRINT | Training Data Collection Sprint | ✅ | 2026-03-30 | TD-ENRICH (InteractionRecord 6 new fields, migration 005, TSDB schema v5), TD-CORR (guidance ID correlation via context.WithValue), TD-SRC (source document linkage in consulting classifier), TD-SCRUB (privacy scrubber, 5 regex categories), TD-QUAL (Python quality annotation pipeline + report), TD-CLI (`mdemg data` CLI with 5 subcommands), TD-BACKUP (JSONL backup integration in TSDB backup service) |

### Phase Numbering Convention

| Series | Range | Domain |
|--------|-------|--------|
| 30s | 31-38 | Space Transfer & DevSpace Collaboration |
| 40s | 41-43 | Core Engine |
| 44-52 | 44-52 | Advanced Features |
| 70s | 70-76 | Operations & Reliability |
| 80s | 80-91 | Meta-Cognition & RSIC |
| 92-100 | 92-100 | Deployable Package |
| 101-105 | 101-105 | Cognitive Intelligence |
| S-series | S8-S16 | Sidecar & Distribution |
| J17 | — | AI-to-AI Communication Protocol |
| FSD | — | Constraint Lifecycle & Neural Re-Ranker |
| AR | — | AutoResearch Integration |
| J17-FL | — | J17 Feedback Loop Closure |
| J17-FIX | — | J17 Protocol Pipeline 12-Break Cascading Fix |
| PROM | — | Prometheus Observability Monitoring |
| DDR | — | Deep-Dive Remediation Sprint |
| TD-SPRINT | — | Training Data Collection Sprint (7 sub-phases: TD-ENRICH, TD-CORR, TD-SRC, TD-SCRUB, TD-QUAL, TD-CLI, TD-BACKUP) |

---

## 5. Open Work Items

### Gap Analysis Phase 4 — Status as of 2026-03-28

Source plan: `.claude/plans/mellow-crunching-hopcroft.md`

| Gap | Title | Tier | Effort | Notes |
|-----|-------|------|--------|-------|
| GAP-18 | `log.Printf` → `slog` structured logging | 2 | M | ✅ ALL waves complete (internal/ + cmd/ + plugins/) |
| GAP-02 | Obsidian vault ingestion (= Phase 45.4) | 2 | M | ✅ v2 complete — parser, walker, Sync RPC, CI gate, release packaging, docs |
| GAP-26 | Module developer tutorial | 2 | M | ✅ Tutorial, echo-module README + Makefile, port fixes |
| GAP-20 | Graph visualization — Grafana dashboard | 2 | S-M | ✅ Neo4j data source plugin, 4-panel topology dashboard, UOTS governed |
| GAP-13 | Windows desktop companion | 3 | M | LOW PRIORITY — Tauri sidebar provides reusable architecture |
| GAP-14 | DBSCAN performance profiling | 3 | Research | LOW PRIORITY — O(n^2) distance matrix; matters at 50K+ nodes |

### Partially Complete Phases


### Research (No Implementation Yet)

| Topic | Deliverable |
|-------|-------------|
| AutoResearch integration analysis | `docs/development/AUTORESEARCH_INTEGRATION_ANALYSIS.md` |
| DBSCAN GPU acceleration | Metal/AMX investigation for clustering performance |

### New Infrastructure (from Gap Analysis)

| Component | Location | Purpose |
|-----------|----------|---------|
| Architecture map generator | `scripts/generate_arch_maps.py` | Generates 10 compact maps for Jiminy context; `--checksum`, `--dry-run`, `--force` modes |
| UITS optimization protection | `docs/tests/uits/schema/uits.schema.json` | `metadata.optimized: true` prevents generator from overwriting converged maps |
| Public API types | `pkg/mdemg/types.go` | Importable Go types for external consumers (GAP-09) |
| Scope-based auth | `internal/auth/types.go`, `middleware.go` | 7 scope constants defined in `auth/types.go`. Wired to 14 destructive endpoints via `scopedHandler` pattern in `server.go` (GAP-16) |
| Auto-credentials | `internal/cli/init.go`, `db.go` | `generatePassword()` via crypto/rand, env var fallback (GAP-19) |
| UAMS runner | `docs/tests/uams/runners/uams_runner.py` | 4 auth specs, 104 assertions (GAP-21) |
| Submodule changelogs | `packaging/*/CHANGELOG.md` | Keep a Changelog format for all 6 packaging submodules (GAP-27) |
| J17 feedback state file | `~/.mdemg/.jiminy-guidance-state` | Bridges prompt-context.sh (writes guidance_id) and post-tool-observe.py (reads, sends feedback) |
| Feedback cooldown file | `~/.mdemg/.jiminy-last-feedback` | Touched after each feedback submission; 30s cooldown between submissions |
| Prometheus alerts | `deploy/docker/prometheus/alerts/observability.yaml` | 4 alert rules: scrape target down, prometheus unhealthy, scrape slowdown, storage high |
| Cache hit metrics | `internal/jiminy/service.go:recordCacheHitMetrics()` | Records J17 protocol metrics on cached guidance responses |
| Bootstrap RSIC assessment | `internal/api/server.go` | Goroutine runs Assess() 10s after startup to populate health gauges |
| LLM Interaction Logger | `internal/llmclient/recorder.go`, `internal/tsdb/llm_writer.go` | `InteractionRecorder` interface + `LLMInteractionWriter` buffered TSDB writer. `SetDefaultRecorder` for zero-consumer-modification wiring. Config: `LLM_INTERACTION_LOGGING` (default: true). 16 consumers labeled via `WithContext()` |
| Managed Transactions | 8 files across `internal/` | All 32 `session.Run()` calls migrated to `ExecuteRead`/`ExecuteWrite`. Zero `session.Run()` remaining |
| K8s/Helm Manifests | `deploy/kubernetes/`, `deploy/helm/mdemg/templates/` | TimescaleDB StatefulSet + neural-sidecar Deployment. Conditional on `.Values.timescaledb.enabled` / `.Values.neuralSidecar.enabled` |
| Context metadata passing | `internal/llmclient/client.go` | `WithGuidanceID(ctx, id)` / `WithSourcePath(ctx, path)` — context.WithValue-based metadata threading for LLM interaction correlation |
| Privacy scrubber | `internal/llmclient/scrubber.go` | `Scrub(*InteractionRecord)` — regex-based scrubbing at TSDB write time (API keys, absolute paths, env secrets, emails, Neo4j credentials). 12 test cases in `scrubber_test.go` |
| Data monitoring CLI | `internal/cli/data.go` | `mdemg data` command with 5 subcommands: `status`, `inspect`, `stats`, `annotate`, `quality`. Training data readiness assessment |
| Quality annotation pipeline | `neural/training/quality_annotator.py`, `quality_report.py` | Python batch jobs: reads protocol JSONL, joins on guidance_id, writes quality scores back to llm_interactions. Task-specific scoring strategies |
| JSONL backup integration | `internal/tsdb/backup.go` | `tarGzDirectory()` helper, `JSONLTarPath`/`JSONLTarSize` fields in backup manifests. Includes `.mdemg/neural/training-data/` as `training-data.tar.gz` |
| InteractionRecord enrichment | `internal/llmclient/recorder.go` | 6 new fields: GuidanceID, SourcePath, ThinkContent, ThinkMode, Quality (*float64), QualitySource. TSDB writer expanded to 22 columns |
| TSDB migration 005 | `internal/tsdb/migrations/005_interaction_enrichment.sql` | Adds `guidance_id` + `source_path` columns with conditional indexes. Schema version 4 → 5 |

---

## 6. Governance & Testing

### Testing Frameworks

| Framework | Status | Location | CI Gated |
|-----------|--------|----------|----------|
| **UATS** (HTTP contract) | Active | `docs/api/api-spec/uats/` | ✅ Merge-blocking |
| **UPTS** (parser contract) | Active | `docs/lang-parser/lang-parse-spec/upts/` | ✅ Merge-blocking |
| **UDTS** (gRPC contract) | Active | `docs/api/api-spec/udts/` | ✅ Canonical-guard |
| **UBTS** (benchmark) | Active | `docs/tests/ubts/` | Soft-fail |
| **USTS** (security) | Active | `docs/tests/usts/` | ✅ Merge-blocking |
| **UAMS** (auth methods) | Active | `docs/tests/uams/` | Soft-fail |
| **UOBS** (observability — runtime) | Active | `docs/tests/uobs/` | Soft-fail |
| **UOTS** (observability — artifacts) | Active | `docs/api/api-spec/uots/` | Soft-fail |
| **UVTS** (semantic validation) | Active | `docs/tests/uvts/` | Soft-fail |
| **UNTS** (hash verification) | Active | `docs/specs/unts-hash-verification.md` | ✅ Merge-blocking |
| **UETS** (emergence eval) | Active | `docs/tests/uets/` | Soft-fail (`--no-llm` in CI) |
| **UITS** (iterative-improvement) | Active | `docs/tests/uits/` | Soft-fail |

### UATS Quick Reference

```bash
make test-api BASE_URL=http://localhost:9999                    # Run all specs
python3 docs/api/api-spec/uats/runners/uats_runner.py add-hashes --spec-dir docs/api/api-spec/uats/specs/
python3 docs/api/api-spec/uats/runners/uats_runner.py verify-hashes --spec-dir docs/api/api-spec/uats/specs/
# CI uses: --exclude-tag unts,llm_required,j17_disabled,jiminy_disabled,sidecar_required,constraint_scope_required
```

**Spec format**: top-level `request` + `expected`, variants in `variants[]`, inline operators (`equals`, `contains`, `type`, `exists`), `{{var}}` for spec variables, `${ENV_VAR}` for environment.

### Developer Guide

`docs/guides/UXTS_DEVELOPER_GUIDE.md` — authoritative reference for UxTS methodology, spec writing, CI integration, anti-patterns, and all 12 frameworks.

---

## 7. Known Issues

| Issue | Severity | Notes |
|-------|----------|-------|
| ~~Obsidian integration v2 in progress~~ | ~~Low~~ | ~~COMPLETE (2026-03-25)~~ — Phase 45.4 fully closed: v2 committed (parser+walker+yaml.v3+full Sync, 23 unit tests), CI gate, GoReleaser 4-platform builds, Homebrew+Scoop+.deb packaging, Windows release, ingestion-guide docs. |
| ~~Claude .md files not ingested into CMS~~ | ~~High~~ | ~~FIXED (2026-03-25)~~ — `mdemg ingest-claude-md` command with SHA256 content-hash change detection. 15 files tracked (3 in-repo, 6 auto-memory, 6 plans). Hooks: session-start (background), pre-compact (forced), post-tool-observe (on Write/Edit). `GET /v1/memory/node/meta` endpoint for hash comparison. |
| ~~Guidance response control characters~~ | ~~Medium~~ | ~~FIXED (2026-03-28)~~ — `internal/sanitize/controlchars.go` strips U+0000–U+001F server-side. Hook `perl` workaround still in place as defense-in-depth. |
| ~~J17 tier graduation — live verification~~ | ~~Info~~ | ~~VERIFIED (2026-03-28)~~ — 12-break cascading fix resolved: code_coverage=100%, T2 tier at 80%, compression_ratio=1.714, all J17 gauges flowing to TSDB. Trust persists via TrustStore to Neo4j. |
| ~~Error leaks in API handlers~~ | ~~High~~ | ~~56 `err.Error()` leaks across handlers (incl. `handlers_org_review.go`)~~ (Resolved in DD-SPRINT: all 56 err.Error() leaks sanitized) |
| DBSCAN clustering performance | Info | O(n^2) on CPU, 10-15min for 8K+ nodes. GPU investigation needed. |
| ~~`docker.go` volume name mismatch~~ | ~~Medium~~ | ~~FIXED (2026-03-25)~~ — `tryMigrateVolume()` in `internal/cli/docker.go` detects legacy hyphen-named volumes, migrates data to compose-style underscore volumes, wired into `mdemg db start`. |
| ~~apt-publish GPG fingerprint~~ | ~~Critical~~ | ~~FIXED (2026-03-20)~~ — `gpg --import-ownertrust` required 40-char fingerprint, was receiving 16-char key ID. PR #171. |
| ~~Linux docs: wrong Ollama model~~ | ~~Medium~~ | ~~FIXED (2026-03-20)~~ — README.md + beta guide recommended `nomic-embed-text` (768d, incompatible). Corrected to `qwen3-embedding:8b` (4096d → MRL truncate to 3072d). PR #172. |
| ~~Linux systemd 6 bugs~~ | ~~High~~ | ~~FIXED~~ — goreleaser archive split, install.sh persistence, upgrade.go systemd handling, teardown dual-path cleanup, ExecStartPre fix. |
| ~~Hook tracking inconsistency~~ | ~~Medium~~ | ~~FIXED~~ — `.gitignore` negation patterns for 5 active hooks, 3 new templates, deleted orphan `pre-tool-enforce.py`. |
| ~~CI Node.js 20 deprecation~~ | ~~Medium~~ | ~~FIXED~~ — Pinned trivy-action@v0.35.0, gitleaks-action@v2.3.9. Deadline: June 2, 2026. |

---

## 8. Quick Reference Commands

```bash
# === Build & Verify ===
go build -o bin/mdemg ./cmd/mdemg
go build ./...
go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...

# === Testing ===
go test ./internal/... -v                                              # Unit tests
go test -tags=integration ./tests/integration/... -v                   # Integration tests
make test-api BASE_URL=http://localhost:9999                           # UATS contract specs
make test-rsic                                                         # RSIC tests
make test-fsd                                                          # FSD tests
cd neural && uv run python -m pytest -v                                # Sidecar tests

# === Server ===
./bin/mdemg start --auto-migrate
./bin/mdemg stop
./bin/mdemg status
./bin/mdemg serve                                                      # Foreground (dev)

# === Database ===
docker compose up -d neo4j
./bin/mdemg db migrate
./bin/mdemg db status
./bin/mdemg db shell

# === Ingestion ===
./bin/mdemg ingest --path . --space-id my-project --extract-symbols
./bin/mdemg ingest --path . --incremental --since HEAD~5
./bin/mdemg consolidate --space-id my-project

# === Space Management ===
./bin/mdemg space list
./bin/mdemg space export --space-id demo --output demo.mdemg
./bin/mdemg space import --file demo.mdemg --conflict skip

# === Health ===
curl http://localhost:9999/healthz
curl http://localhost:9999/readyz
curl http://localhost:9999/v1/embedding/health

# === CMS Memory ===
curl -s -X POST http://localhost:9999/v1/conversation/resume \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","max_observations":10}'

# === Training Data ===
./bin/mdemg data status                                           # Per-task counts + JSONL sizes
./bin/mdemg data inspect --task jiminy.synthesize --last 5         # View recent records
./bin/mdemg data stats                                            # Per-task stats + readiness
./bin/mdemg data annotate --dry-run                               # Preview quality annotation
./bin/mdemg data quality                                          # Quality coverage report

# === Claude .md File Ingestion ===
./bin/mdemg ingest-claude-md --space-id mdemg-dev              # Normal (hash skip)
./bin/mdemg ingest-claude-md --space-id mdemg-dev --force      # Force all
./bin/mdemg ingest-claude-md --space-id mdemg-dev --dry-run    # Preview
curl -s "http://localhost:9999/v1/memory/node/meta?space_id=mdemg-dev&path=CLAUDE.md"

# === Proto Regeneration ===
protoc --go_out=. --go-grpc_out=. api/proto/space-transfer.proto
protoc --go_out=. --go-grpc_out=. api/proto/devspace.proto
protoc --go_out=. --go-grpc_out=. api/proto/mdemg-module.proto
```

---

*Last updated: 2026-03-30 — Training Data Collection Sprint (TD-SPRINT) COMPLETE: TD-ENRICH (InteractionRecord 6 new fields, migration 005, TSDB schema v5), TD-CORR (guidance ID correlation via context.WithValue), TD-SRC (source document linkage), TD-SCRUB (privacy scrubber, 5 regex categories), TD-QUAL (Python quality annotation pipeline), TD-CLI (mdemg data, 5 subcommands), TD-BACKUP (JSONL backup integration). CI: ALL GREEN.*
