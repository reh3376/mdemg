# MDEMG Agent Handoff Document

<!-- markdownlint-disable MD022 MD031 MD032 MD040 MD051 MD058 MD060 -->

**Date:** 2026-04-17
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
- Quality hardening — COMPLETE (208 UATS specs / 398 variants, 213 Go test files, 0 lint issues)
- ANN Optimization Suite — COMPLETE (10 optimizations, 28 config params)
- AutoResearch Integration — COMPLETE (AR-1 feedback loop, AR-2 effectiveness, AR-3 LLM intelligence)
- FSD-2026-001 Gap Closure — FULLY COMPLETE (21 gaps + NR-1 through NR-5 + F21)
- Debian Native Packaging — COMPLETE (.deb via goreleaser, APT repo, AUR PKGBUILD, APT publish verified)
- Doc Consolidation — COMPLETE (4 user-facing docs centralized in docs/user/)
- J17 Feedback Loop Closure — COMPLETE (state file bridge, hook feedback delivery, control char sanitization, bootstrap codification)
- J17 Protocol Pipeline 12-Break Cascading Fix — COMPLETE (code lookup, trust persistence, cache bypass, threshold sync, live collector wiring, all gauges flowing)
- RSIC Overhaul (RSIC-OVH-2026-04-09) — COMPLETE (configurable ProtectedSpaces, graph-relative blast radius, real executors, diagnostic classification, calibration-aware planner, 20-action LLM reflector, daily cycles)
- RSIC Hardening (RSIC-HDN-2026-04-09) — COMPLETE (32 deep dive findings remediated: nil postReport, nil driver guards, dryRun race, per-task CriteriaMet, executor correctness, watchdog lock/reset, safety fail-closed, LLM sanitization, SSE race fix)
- Training Data Quality (TRAIN-DQ-2026-04-10) — COMPLETE (model standardization to gpt-5.4, token counting fix tokens_in=0, RAFT context wiring for 2 tasks, 2 feature gate activations)
- UAITS Framework (UAITS-2026-04-10) — COMPLETE (Universal AI Training Specification: 10th UxTS framework for spec-driven training data curation with 4 paradigms SFT/DPO/RAFT/curriculum, DPO pair builder, paradigm router, 2 CLI commands, 260 Python tests, 41 runner checks)
- Adversarial Bug Fix Campaign (ACA-BFC-2026-04-10) — COMPLETE (14 bugs from adversarial analysis: 3 critical, 3 high, 6 medium, 2 low — infra/training/LLM client/Jiminy)
- Prometheus Observability Monitoring — COMPLETE (cache hit metrics, bootstrap RSIC assessment, self-monitoring probe, 4 alert rules)
- Gap Analysis — COMPLETE (Phases 1-4; GAP-13/14 deferred to future sprints)
- PR #215 Remediation Sprint — COMPLETE (gauge dirty flag, TSDB backup service, compose standardization, alert validation, 70/70 Playwright e2e)
- Training Data Collection Sprint — COMPLETE (7 sub-phases: InteractionRecord enrichment, guidance ID correlation, source linkage, privacy scrubber, quality annotation, data CLI, JSONL backup)
- FT Infrastructure Sprint — COMPLETE (4 phases: A=SanitizeResponse+prompt hash, B=RAFT context enrichment, C=ULTS spec framework, D=embedding/retrieval data collection)
- FT-DATA Sprint — COMPLETE (8 phases: UTDS spec framework, TSDB exporter+CLI+API, browser UI tab, quality_filter.py, format_converter.py, dataset_versioner.py, round-trip verification, documentation)
- FT Training Pipeline — COMPLETE (PRs #243-250: PROD-READINESS, compose embed, export-auto LaunchAgent, vllm-mlx/train_ft.py, evaluate_ft.py, regression_gate.py, teacher_distill.py/21 GRPO rewards, quantize_deploy.py)
- CI: ALL GREEN (push + pull_request + release) as of 2026-04-03
- Latest releases: CLI v0.5.4, GHCR mdemg:v0.5.4, GHCR neural-sidecar:v0.5.4, menubar v1.8.0, sidebar v0.3.0

WHAT REMAINS TO BE DONE:
=== COMPLETED SINCE LAST HANDOFF (2026-04-17) ===
- ✅ DH-004: J17 Protocol & Jiminy Dashboard Remediation (2026-04-17):
  - E1: Resolved `mdemg-j17.json` panel overlap at {x:6,y:24}; relocated Total Events to full-width {x:0,y:28,w:24,h:4}; annotated `jiminy_latest_age_ms` panel for /strict-mode expected staleness
  - E2: `TicketRestoreSuccessRate` defaults to 1.0 when `ticketRestoreTotal == 0` (matches codeCoverage null-tolerance pattern); new `TicketRestoreTotal` field distinguishes "no data" from "100% pass"
  - E3: `J17_SIDECAR_TIMEOUT_MS` default 200→1000 w/100ms floor; NLI fallback counting gated on `nliScorer.IsOperational()`; 8 J17 env vars exposed in all compose templates
  - E4: `CONSULTING_CLASSIFY_TIMEOUT_MS` default 15000→30000; new `LLM_RETRY_DEADLINE_ENABLED` (default true) adds budget-aware retry on `context.DeadlineExceeded`; new admin endpoints `GET/POST /v1/admin/breakers[/reset]`; closed cooldown TOCTOU race via atomic `TryRecord()`
  - E5: Context cooler graduation fix — `CoactivateSession` now calls `reinforceSessionObservations` to raise `stability_score` per session (was 0 before, kept 99.7% of obs volatile forever)
  - E6: CLAUDE.md + docs/user/cms-rsic-guide.md updated; 3-tier tests (unit + integration + concurrency + live dashboard validation)
  - 8 commits pushed; PR #325 with sprint summary

=== COMPLETED SINCE LAST HANDOFF (2026-04-10) ===
- ✅ ACA-BFC-2026-04-10: Adversarial Codebase Analysis Bug Fix Campaign (2026-04-10):
  - 14 bugs remediated from adversarial analysis with systematic refutation (3 critical, 3 high, 6 medium, 2 low)
  - E1 (Infra): Docker healthcheck port fix, CI coverage wiring, config comment alignment, TSDB env var rename, dead ScoringRho removal
  - E2 (Training): LoRA rank flag correction, real evaluation metrics (4 heuristic functions replace check_non_empty stubs)
  - E3 (LLM Client): Circuit breaker trip guard (atomic CompareAndSwap, idempotent alert), 502 retry
  - E4 (Jiminy): Semantic dedup, temporal correction decay, bounded ticket LRU, eval cache wiring, dead goroutine removal
  - New config: JIMINY_DEDUP_SIMILARITY_THRESHOLD (0.85), JIMINY_CORRECTION_DECAY_RATE (0.01), J17_TICKET_CACHE_SIZE (1000)
  - Removed: SCORING_RHO (dead field, suffixed variants unaffected)
  - 0 lint issues, all Go + Python tests pass

=== COMPLETED SINCE LAST HANDOFF (2026-04-09) ===
- ✅ RSIC-HDN-2026-04-09: RSIC Hardening — Deep Dive Remediation (2026-04-09):
  - 32 findings remediated (2 P0, 9 P1, 19 P2, 2 P3) across 6 sequential epics
  - P0: Nil postReport calibration corruption fix, nil driver guards (7 executors)
  - P1: dryRun data race eliminated, per-task CriteriaMet, executor correctness (flush buffer, params, OOM)
  - P2: Watchdog lock contention + reset timing, safety fail-closed, config cross-validation
  - P2: LLM reflector single action source (20 actions), prompt injection sanitization
  - P2: Orchestration atomic snapshot, synergy cache TTL, CompleteCycle timeout handling
  - P2: SSE job stream race fix (Job.Snapshot()), health formula extraction
  - P3: Dead code cleanup, cron parser documentation
  - CI: -race flag added to test pipeline
  - 0 lint issues, all tests pass with -race
- ✅ UI-AUDIT-2026-04-09: Browser UI Comprehensive Audit & Testing (2026-04-09):
  - E1: Screenshot baselines for all 10 tabs + JS error + API 5xx per-tab tests (30 new tests)
  - E1: TestTrainingDataTab (10 read-only tests) + TestTrainingDataAPI (3 endpoint tests)
  - E2: 10 TestInteractive* classes — all buttons/inputs/dropdowns/checkboxes tested (25 new tests)
  - E3: 2 bug fixes — training_data.js helpPanel signature, dom.js infoRow Node handling
  - E4: Gap analysis — 48/125 endpoints covered (38%), 77 uncovered routes documented
  - Suite: 309 total tests (306 pass, 3 skip), zero failures
  - Docs: CHANGELOG, AGENT_HANDOFF, ui-gap-analysis.md, browser-ui.md updated

=== COMPLETED SINCE LAST HANDOFF (2026-04-07) ===
- ✅ SNA-001: Server-Native Alerting & Final Resilience Hardening (2026-04-07):
  - E1: Trust persistence goroutine leak fixed — cancellable context, StopTrustPersistence() in Shutdown()
  - E1: Dead startup code wired — StartContextCoolerProcessing and StartWeeklyGapInterviews behind config gates
  - E2: Server-native alert evaluator — 13 TSDB-query rules migrated from Grafana, ForDuration state tracking
  - E3: Goroutine supervisor — panic recovery, exponential backoff restart, warning/critical alerts
  - E4: Grafana alert rules demoted to supplementary — contact point/policy disabled, endpoint preserved
  - Config: 4 new env vars (ALERT_EVALUATOR_ENABLED, ALERT_EVALUATOR_INTERVAL_SEC, CONTEXT_COOLER_ENABLED, WEEKLY_GAP_INTERVIEWS_ENABLED)
  - Tests: 7 evaluator + 6 supervisor tests, all passing
  - Grafana no longer required for alert evaluation — all 28 rules evaluated server-natively
- ✅ SR-001 Gap Closure Sprint (2026-04-07):
  - F-001: CooldownSec=0 now means "no cooldown" (was defaulting to 300s)
  - F-002: Health prober SetAlertCallback wired to alert dispatcher on transitions
  - F-003: TSDB LLM writer SetAlertCallback wired for buffer overflow events
  - G4+G11: LLM consecutive failure tracking with alert callback (threshold configurable via LLM_CONSECUTIVE_FAILURE_THRESHOLD, default: 3)
  - G8: Circuit breaker added to outcome classifier and constraint code generator
  - G5: /readyz check #5 upgraded to live CMS Ping (RETURN 1) — detects Neo4j degradation
  - Config: 1 new env var (LLM_CONSECUTIVE_FAILURE_THRESHOLD)
  - Tests: 3 consecutive failure tests + 1 zero cooldown test, all passing
  - B3 (Neo4j driver reconnection) deferred — Go driver v5 handles internally; prober now alerts on connectivity loss
- ✅ SR-001: Service Resilience & User Alerting Sprint (2026-04-07):
  - E1: Alert dispatcher package (internal/alert/) — file + macOS backends, cooldown dedup, atomic writes, FIFO eviction
  - E2: Hook alert delivery — prompt-context.sh and session-start.sh read alert file
  - E3: Wire dispatcher to RSIC actions (5 handlers), circuit breaker OnStateChange, health prober transitions
  - E4: LLM retry with exponential backoff — retries 429/503, Retry-After header support, package-level default config
  - E5: Enhanced /healthz with subsystem checks, TSDB buffer overflow detection
  - E6: Grafana contact point provisioning + 7 new alert rules (28 total)
  - E7: Documentation (CHANGELOG, AGENT_HANDOFF, CLAUDE.md)
  - Config: 15 new env vars (Alert*, HealthProbe*, LLMRetry*, TSDBWriterBufferMaxSize)
  - Tests: 12 alert tests + 10 retry tests + 4 Grafana webhook tests, all passing with -race
- ✅ SR-001 Live Validation & Hotfix (2026-04-07):
  - Live tested all SR-001 features across 10 phases (baseline, webhook, degraded healthz, LLM retry, TSDB overflow, hook delivery)
  - Fixed: macOS `timeout` command missing — broke alert banner rendering in both hooks (session-start.sh, prompt-context.sh)
  - New UATS specs: `grafana_alert_webhook.uats.json` (3 variants), `healthz_enhanced.uats.json` (8 assertions)
  - Updated: `health.uats.json` — added `$.checks` exists assertion for enhanced /healthz
  - Updated: `api-reference.md` — enhanced /healthz docs (checks map, degraded response), Grafana webhook endpoint
  - Evidence artifact: `scripts/tsdb_data_review_sr001.json` (CONDITIONAL PASS)
  - Findings: F-001 cooldown zero fallback, F-002 prober callback not wired, F-003 TSDB writer callback not wired
=== COMPLETED SINCE LAST HANDOFF (2026-04-06) ===
- ✅ FT Plan v4.0 Doc Update + Default LLM Migration (2026-04-06):
  - Default LLM migrated: gpt-5-nano → gpt-4.1-nano (non-tool-use, 2x cheaper output, 1M context)
  - RECLASS_MODEL: gpt-5.4 → gpt-4.1-nano, RERANK_MODEL: gpt-5.4 → gpt-4.1-nano
  - All 7 FT plan documents updated v3.0 → v4.0 (00_README through 06_CORRECTIONS_APPLIED)
  - Corrections Issues 20-28 documented: tool-use constraint (CRITICAL), default LLM change (CRITICAL), classifier overhaul training data boundary (CRITICAL), curated pipeline, reward count 18→21, TSDB schema 005→010, Jiminy quality signals, collection campaign
  - Training Data Versioning Boundary (§12 in 05_DATA_COLLECTION): v0.7.1 classifier overhaul creates hard boundary — pre-v0.7.1 jiminy.evaluate data has 82.4% misclassification from measurement error. Per-task EXCLUDE/SAFE recommendations for dataset_versioner.py
  - Config changes: config.go, yaml_config.go, compose template, init.go, j17-comprehension-test
  - Doc updates: cli-reference, architecture, installer sync, jiminy, neural-training-pipeline
=== COMPLETED SINCE LAST HANDOFF (2026-04-05) ===
- ✅ Training Data Export E2E Fix (2026-04-05) — P0 blocking bug fix:
  - G1: `data export` and `data export-auto` auto-generate `instance_id` as `{hostname}-{space_id}` when `MDEMG_INSTANCE_ID` not set
  - G2: `mdemg init` writes `MDEMG_INSTANCE_ID` to `.env` (both native and Docker modes)
  - Extracted `resolveInstanceID()` helper with 6 unit tests (flag > env > auto-generate)
  - E2E validation: 10/10 tests PASS (export, UTDS 36/36, quality_filter, format_converter, dataset_versioner, multi-source merge, train dry-run, export-auto, regression)
  - Remaining gaps (future): G3 (quality_filter --archive), G4 (data import), G5 (DevSpace gRPC), G7 (PYTHONPATH), G8 (pipeline script), G9 (export-auto UTDS)
- ✅ Codebase Hardening + Operational Readiness (2026-04-05) — v0.7.0:
  - P0 fixes: RRF activation seeding (BM25-only candidates no longer suppressed), pre-bash fail-closed guard, schema version 23 across all deploy configs + CI validation
  - P1 fixes: Signal learner Neo4j persistence (V0024, 30s flush), goroutine WaitGroup tracking, per-space consolidation TryLock, cache key completeness, nil-safe embedder
  - P2 fixes: Config.Validate() cross-field checks, pool metrics collector, writeback 10s timeout, sidecar confidence floor for NLI scorer
  - Scheduled maintenance LaunchAgent (weekly decay + prune)
  - 4 unit tests for activation seeding, all existing tests pass
- ✅ Init Config Propagation + Hook Port Discovery (2026-04-05) — v0.6.1:
  - Issue #265: `mdemg init` now writes Jiminy env vars to `.env` (JIMINY_ENABLED, synthesis model/provider, evaluate model/provider) for both native and Docker paths
  - Issue #267: Hook templates use runtime port discovery (`.mdemg.port` → `.env` MDEMG_PORT → 9999) instead of hardcoded URL baked at install time
  - All 5 hook templates include `# MDEMG` marker for lifecycle management
  - Upstreamed hook customizations: staleness check, ingest error logging, prune-guard error capture
  - `mdemg init` force-updates hooks to latest templates on re-run
  - 4 new unit tests (marker, no placeholder, shell port discovery, python port discovery)
  - Integration verified: Jiminy propagation, port cascade, no .env duplicates
- ✅ Graph Health User Upgrade Sprint (2026-04-05) — v0.6.0 release prep:
  - V0023 self-healing migration: batched SymbolNode dedup before uniqueness constraint
  - `mdemg graph repair` command: weight-preserving dedup with CO_ACTIVATED_WITH edge aggregation
  - Hidden layer OOM fix: batched orphan HiddenPattern deletion (500/tx)
  - Live validation fixes: python3 symlink, QUERY_CLASSIFY_ENABLED default, pre-campaign check, curation pipeline args
  - Live validation: 15/15 non-destructive PASS, 1 PARTIAL (launchd), 3 SKIP (destructive)
  - Documentation: CLI reference, upgrade guide, changelog, homebrew README, beta testing, CLAUDE.md
=== COMPLETED SINCE LAST HANDOFF (2026-04-02) ===
- ✅ Live Validation Hardening (2026-04-03) — Campaign hardening via PR #254:
  - PR #254: WithSpaceID context, compose env vars, campaign init prompt, migration 010, live_validation.py (19 tests), docker-publish cron
  - v0.5.3 tagged + released, GHCR images updated, full release chain verified
- ✅ Live Validation: Data Propagation (2026-04-03) — 5 data propagation fixes via PR #253:
  - PR #253: session_id propagation (F7), space_id reranker threading (F8), recorder init ordering (F9), export instance_id (F10), regression_gate docstring (F11)
  - All 19 live validation tests re-run: 4 FAILs → PASS, 15 PASSes held, 0 regressions
- ✅ Live Validation: Docker/Init Fixes (2026-04-02) — 6 infrastructure fixes via PR #252:
  - PR #252: neural-sidecar GHCR image, AUTO_MIGRATE, docker-publish CI trigger, .env loading, LaunchAgent embed, compose sync CI check
  - 11 findings from manual live validation sprint, first 6 fixed in-sprint
- ✅ FT Training Pipeline (2026-04-02) — Complete training pipeline via PRs #243-250:
  - PR #243: PROD-READINESS — QueryClassifier wired, session_id propagation, pre-campaign CLI
  - PR #244: Compose embed fix — docker-compose.yml embedded in binary for Homebrew/edge
  - PR #245: Export-auto + training-export LaunchAgent
  - PR #246: vllm-mlx setup guide + train_ft.py
  - PR #247: evaluate_ft.py — per-task evaluation against ULTS quality_metrics
  - PR #248: regression_gate.py — deployment gate (PASS/FAIL/WARN)
  - PR #249: teacher_distill.py + reward_functions.py (21 GRPO rewards)
  - PR #250: quantize_deploy.py — fuse + quantize for production
- ✅ FT-DATA Sprint (2026-04-01) — Full training data export + curation pipeline:
  - Phase 1: UTDS spec framework (14th UxTS, JSON Schema, 3 fixture specs, runner, 23 tests)
  - Phase 2: TSDB exporter + `mdemg data export` CLI + API handler (streaming, privacy scan 10 fields, tar.gz)
  - Phase 3: Browser UI Training Data tab (10th tab, export form, status polling, download)
  - Phase 4: quality_filter.py (8 gates, privacy hard-reject, ULTS validation, 25 tests)
  - Phase 5: format_converter.py (MLX chat format, RAFT 80/20, think-mode, 21 tests)
  - Phase 6: dataset_versioner.py (temporal split, dedup, SHA-256, manifest, 20 tests)
  - Phase 7: Round-trip verified: TSDB→export(449)→UTDS(26/26)→filter(287)→convert→version(229/28/30)
  - Known: retrieval_events.query_text contains codebase paths flagged by privacy scanner (correct behavior — different threat model for exported data)
- ✅ J17 Comprehension Pipeline Fix + Trust Rebalance (2026-04-01) — 5 independent pipeline breaks fixed:
  - Sigmoid midpoint 2.0→1.5 (retrieval_source.go + consulting/service.go): scores 1.0-1.5 now pass JiminyMinConfidence
  - NLI guard broadened: allows heuristic comprehension for non-constraint items
  - constraint_code propagation in hidden layer (constraint_nodes.go)
  - Trust relevance filter (trustRelevanceThreshold=0.5): prevents "not applicable" items from decaying trust
  - Trust parameters rebalanced: initial 0.65, threshold 0.75, boost +0.05/follow, decay -0.02/ignore (T1 achievable in 3 follows)
- ✅ TSDB Trust Score Historization (2026-04-01) — 4 new gauges (avg/min/max trust score, session count) wired through 8 files: TrustScorer.Aggregates() → ProtocolMetrics → adapter → publishProtocolGauges → TSDB flush. J17 Grafana dashboard: Trust Score row + trend panel.
- ✅ DATA-GOV Sprint 0 (2026-03-31) — Fixed TSDB_ENABLED=false in Docker deployments (env var + auto-migrate support)
- ✅ DATA-GOV Sprint 1 (2026-03-31) — Pipeline gap fixes: system prompt coverage, call_site propagation, PROMPT-COV
- ✅ DATA-GOV Sprint 2 (2026-04-01) — Python diagnostic script (scripts/tsdb_data_review.py): 7-section data quality report across all 8 TSDB tables. Text + JSON output. Privacy scrub verification. 17 unit tests. End-to-end verified against live TSDB.
- ✅ Docker Deployment Phase 3: Backup UI + Distribution + Cleanup (DOCKER-P3) — 9th Backup tab (trigger/list/restore/delete), credential prompts in `mdemg init`, enhanced post-install summary, removed Windows build/Scoop/nfpms from release, archived 5 submodules, deprecated `db start`/`db stop`, 221 Playwright tests
- ✅ Docker Deployment Phase 2: Browser Dashboard (DOCKER-P2) — 6-tab browser UI at /ui/ served via embed.FS, admin/config + admin/logs endpoints, LogRingBuffer, Catppuccin Mocha theme, Grafana deduplication (link don't duplicate)
- ✅ FT Infrastructure Sprint (2026-03-30) — 4 phases for fine-tuning data quality:
  - Phase A: SanitizeResponse (StripThinkBlock + StripCodeFence, 11 call sites, system prompt hash)
  - Phase B: RAFT Context Enrichment (RetrievalContext, consulting + jiminy wiring, migration 007, 26 TSDB columns)
  - Phase C: ULTS Spec Framework (16 LLM task specs, JSON schema, runner, 100% pass rate)
  - Phase D: Embedding/Retrieval Data Collection (2 new hypertables, WithEmbeddingMeta at 9 call sites, migration 006)

=== COMPLETED SINCE PREVIOUS HANDOFF (2026-03-28) ===
- ✅ J17 Feedback Loop Closure — State file bridge, hook feedback delivery, control char sanitization, bootstrap codification
- ✅ J17 Protocol Pipeline 12-Break Cascading Fix — Code lookup via content-similarity, TrustStore Neo4j persistence, cache bypass, threshold sync, effectiveness TTL 2h, live collector wiring, metrics snapshot refresh
- ✅ Prometheus Observability — Cache hit metrics, bootstrap RSIC assessment, self-monitoring probe, 4 alert rules
- ✅ TSDB Sprint infrastructure — Live collectors, trend analyzer, Grafana dashboards
- ✅ Training Data Collection Sprint (2026-03-30) — 7 gaps fixed for Qwen3-30B-A3B fine-tuning pipeline: InteractionRecord enrichment (6 fields, migration 005, schema v5), guidance ID correlation (context.WithValue threading), source document linkage (consulting classifier), privacy scrubber (5 regex categories, 12 tests), quality annotation pipeline (Python batch + report), data monitoring CLI (5 subcommands), JSONL backup integration

=== GAP ANALYSIS Phase 4 (COMPLETE) ===
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
- Branch: reh3376_dev01 — clean (all changes committed and pushed, CI green)
- TSDB schema version: 7 (migration 007: RAFT context columns)
- Tag: v0.4.1
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
| Gap | Gap Analysis Implementation | ✅ | Phases 1-4 complete. GAP-13 (Windows companion) and GAP-14 (DBSCAN profiling) deferred to future sprints |
| REM | PR #215 Remediation Sprint | ✅ | Gauge dirty flag (TSDB noise reduction), TSDB backup/restore (pg_dump, CLI, scheduler, retention), compose standardization, 21 alert rules validated, 70/70 Playwright e2e |
| DD-SPRINT | Deep-Dive Remediation Sprint | ✅ | 2026-03-29 | SEC-LEAK (56 error leaks sanitized), GAP-16 (RequireScope wired to 14 endpoints), DOC-REM (19 docs remediated), K8S-ALIGN (K8s/Helm + TimescaleDB + neural-sidecar), LLM-LOG (interaction logger), TXN-MGMT (32 session.Run → managed transactions) |
| SVC-RES | Service Resilience & Ingest Hardening | ✅ | 2026-03-30 | Hook auto-recovery (auto-start, visible warnings, error logging), ingest JSONL buffer (buffer/flush on server down), prune-guard detection, protected overflow (path-based ingest), macOS LaunchAgent supervision (3 plists), `mdemg service` CLI (5 subcommands), hook template sync (5/5 hooks registered with matchers), `mdemg data audit` |
| TD-SPRINT | Training Data Collection Sprint | ✅ | 2026-03-30 | TD-ENRICH (InteractionRecord 6 new fields, migration 005, TSDB schema v5), TD-CORR (guidance ID correlation via context.WithValue), TD-SRC (source document linkage in consulting classifier), TD-SCRUB (privacy scrubber, 5 regex categories), TD-QUAL (Python quality annotation pipeline + report), TD-CLI (`mdemg data` CLI with 5 subcommands), TD-BACKUP (JSONL backup integration in TSDB backup service) |
| TD-VERIFY | Training Data Capture Verification | ✅ | 2026-03-30 | 17 tests across 5 files: column-position verification (26+23+22 columns), privacy scrub completeness (5 patterns across 4 fields), scrub asymmetry (embedding TextContent only, not QueryText), response sanitization JSON round-trip, empty TaskName regression guard, batch ordering, training column initialization. `mockPool` upgraded to capture CopyFrom values. Doc: `docs/features/training-data-capture-verification.md` |
| DOCKER-P1 | Docker Deployment — Phase 1 | ✅ | 2026-03-30 | Docker Compose consolidation (5 services, parameterized ports, neo4j:5 community, multi-instance via COMPOSE_PROJECT_NAME). Docker image CI (GHCR multi-arch). `mdemg init` Docker-first (6-port scan, .env generation, compose up, health check). Dockerfile.prod healthcheck fix. `docs/user/quickstart-docker.md`. |
| DOCKER-P2 | Docker Deployment — Phase 2: Browser Dashboard | ✅ | 2026-03-30 | 6-tab browser UI at /ui/ via embed.FS, admin/config + admin/logs endpoints, LogRingBuffer, Catppuccin Mocha theme, Grafana deduplication |
| DOCKER-P2b | Docker Deployment — Phase 2b: UI/UX Overhaul | ✅ | 2026-03-30 | Field mapping fixes, RSIC service controls, server restart, editable config, Plugins + Features tabs, 193 Playwright tests |
| DOCKER-P3 | Docker Deployment — Phase 3: Backup UI + Distribution | ✅ | 2026-03-31 | Backup tab (9th), credential prompts, distribution cleanup, 5 submodules archived, 221 Playwright tests |
| FT-INFRA | FT Infrastructure Sprint | ✅ | 2026-03-30 | Phase A: SanitizeResponse, Phase B: RAFT context, Phase C: ULTS specs, Phase D: embedding/retrieval data collection. Migrations 006+007, TSDB schema v7 |
| DATA-GOV-S0 | DATA-GOV Sprint 0: TSDB_ENABLED Fix | ✅ | 2026-03-31 | TSDB_ENABLED=true in Docker compose, TSDB_AUTO_MIGRATE env var support. `docs/features/tsdb-data-governance.md` |
| DATA-GOV-S1 | DATA-GOV Sprint 1: Pipeline Gaps | ✅ | 2026-03-31 | System prompt coverage, call_site propagation, PROMPT-COV (system prompt hash + coverage) |
| DATA-GOV-S2 | DATA-GOV Sprint 2: Diagnostic Script | ✅ | 2026-04-01 | `scripts/tsdb_data_review.py` — 7-section diagnostic (schema, metrics, LLM, embeddings, retrieval, FT, cross-cutting). Text+JSON output. Privacy scrub verification. 17 unit tests. E2E verified |
| J17-COMP | J17 Comprehension Pipeline Fix | ✅ | 2026-04-01 | 5 independent breaks: sigmoid midpoint, NLI guard, constraint_code propagation, trust relevance filter, trust parameter rebalance. T1 compression achievable in single session |
| J17-TRUST | TSDB Trust Score Historization | ✅ | 2026-04-01 | 4 new TSDB gauges (avg/min/max trust, session count), Grafana J17 dashboard trust row + trend panel |
| PROD-READY | PROD-READINESS Sprint | ✅ | 2026-04-02 | PR #243: QueryClassifier wired, session_id propagation, pre-campaign CLI |
| COMPOSE-EMBED | Docker Compose Embed Fix | ✅ | 2026-04-02 | PR #244: docker-compose.yml embedded in binary for Homebrew/edge deployments |
| EXPORT-AUTO | Export-Auto + Training Export LaunchAgent | ✅ | 2026-04-02 | PR #245: Automated export trigger + macOS LaunchAgent for training data |
| FT-TRAIN | vllm-mlx Setup + train_ft.py | ✅ | 2026-04-02 | PR #246: vllm-mlx setup guide + train_ft.py training script |
| FT-EVAL | evaluate_ft.py | ✅ | 2026-04-02 | PR #247: Per-task evaluation against ULTS quality_metrics |
| FT-GATE | regression_gate.py | ✅ | 2026-04-02 | PR #248: Deployment gate with PASS/FAIL/WARN outcomes |
| FT-REWARD | teacher_distill.py + reward_functions.py | ✅ | 2026-04-02 | PR #249: Teacher distillation + 21 GRPO reward functions |
| FT-DEPLOY | quantize_deploy.py | ✅ | 2026-04-02 | PR #250: Fuse + quantize pipeline for production deployment |
| MULTI-INST | Multi-Instance Testing | ✅ | 2026-04-03 | PR #256: 4 simultaneous instances (20 containers), all isolated, FindFreePort verified. Resource: ~2.3 GiB/fresh, ~5.7 GiB/mature. Guide: docs/user/multi-instance.md |
| TEARDOWN-COMPOSE | Teardown Docker Compose Awareness | ✅ | 2026-04-03 | PR #257: Detects docker-compose.yml, runs docker compose down [-v]. Falls back to legacy container cleanup |
| TEARDOWN-TSDB | Teardown TSDB Backup | ✅ | 2026-04-03 | PR #258: Phase 0b — pg_dump via docker compose exec BEFORE volume destruction. Non-fatal on failure |
| SUBMOD-054 | Submodule Update v0.5.4 | ✅ | 2026-04-03 | PR #259: homebrew-mdemg submodule pointer to v0.5.4 |
| UPGRADE-AUTO | Upgrade Automation | ✅ | 2026-04-04 | PR #260: `mdemg upgrade` + `brew upgrade mdemg` now auto-update Docker instances. New flags: `--no-docker`, `--docker-only`. GoReleaser `post_install` hook. |
| TRAIN-DQ | Training Data Quality | ✅ | 2026-04-10 | Model standardization to gpt-5.4, token counting fix (tokens_in=0), RAFT context wiring (consulting.synthesis, retrieval.rerank_cross), 2 feature gate activations |
| UAITS | UAITS Framework Sprint | ✅ | 2026-04-10 | Universal AI Training Specification — 10th UxTS framework. 6 epics: UAITS schema + MDEMG spec (4 datasets), UAITS runner (41 checks), DPO pair builder, paradigm router + pipeline mods, CLI (`data curate`/`data validate`), docs. 260 Python tests, ruff clean, golangci-lint clean |

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
| TD-VERIFY | — | Training Data Capture Verification (17 tests: column positions, privacy scrub, response sanitization, metadata completeness) |
| TRAIN-DQ | — | Training Data Quality (model standardization, token counting fix, RAFT context wiring, feature gate activations) |
| UAITS | — | UAITS Framework Sprint (schema, spec, runner, DPO builder, paradigm router, CLI, docs — 6 epics) |
| DOCKER-P1/P2/P2b/P3 | — | Docker Deployment Phases 1-3 (compose, browser UI, UI/UX overhaul, backup tab + distribution) |
| FT-INFRA | — | FT Infrastructure Sprint (SanitizeResponse, RAFT context, ULTS specs, embedding/retrieval collection) |
| DATA-GOV-S0/S1/S2 | — | Data Governance Sprints 0-2 (TSDB_ENABLED fix, pipeline gaps, diagnostic script) |
| J17-COMP | — | J17 Comprehension Pipeline Fix (5 breaks: sigmoid, NLI guard, constraint_code, trust filter, trust rebalance) |
| J17-TRUST | — | TSDB Trust Score Historization (4 gauges, Grafana dashboard) |

---

## 5. Open Work Items

### DASH-001: RSIC Dashboard Data Fix — COMPLETE as of 2026-04-08

Fixes for RSIC Operations Grafana dashboard data issues (7 panels showing "No data" or 0):
- **SynergyFileReader** implemented (`internal/ape/synergy_reader.go`) — reads CLAUDE.md/MEMORY.md line counts, wired via `SetSynergyReader()` in `server.go`. Fixed 4 panels (Synergy gauge, CLAUDE.md Lines, MEMORY.md Lines, Synergy Overflow & Buffer).
- **Assessment confidence debug logging** — `computeConfidence()` logs data point values when confidence < 0.3 for faster diagnosis.
- **Dashboard noValue display** — Action Success Rate, Safety Blocks, Snapshots Created, Trigger Rejections panels show "None" instead of "No data" when empty.
- **Cycle completion** — Confirmed cycles complete normally (confidence=1.0) after clean server restart; previous low_confidence was transient from server bouncing during SNA-001 live testing.

### Gap Analysis Phase 4 — COMPLETE as of 2026-03-30

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

### Fine-Tuning Pipeline (ft-lora) — Status as of 2026-03-30

Source plan: `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md`

| Phase | Title | Status | Notes |
|-------|-------|--------|-------|
| 1 | LLM Interaction Logger | ✅ COMPLETE | PRs #217-#219. 16 consumers, TSDB writer, scrubber, quality pipeline, data CLI |
| 2 | Think Mode + Response Sanitization | ✅ COMPLETE | FT-INFRA Phase A: `SanitizeResponse(StripThinkBlock + StripCodeFence)`, 11 call sites, system prompt hash |
| 3 | vllm-mlx Integration | ✅ COMPLETE | PR #246 |
| 4+ | SFT/GRPO/DPO training | ✅ COMPLETE | PRs #246-250 |

**Next actionable step**: Collection Campaign running. Training pipeline complete (export → filter → convert → version → train → evaluate → gate → deploy). First training cycle pending sufficient data accumulation (~500+ records per task).
**Data collection**: Activated 2026-03-30. Accumulating since — expect sufficient volume by mid-April 2026.

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
| ~~Hook template drift~~ | ~~High~~ | ~~FIXED (2026-03-30)~~ — `claudeHookFiles()` expanded to all 5 hooks with Matcher support. Templates synced from active hooks with `{{SPACE_ID}}`/`{{MDEMG_URL}}` placeholders. `mergeClaudeSettings()` places matcher at group level. SVC-RES sprint Phase D. |
| DBSCAN clustering performance | Info | O(n^2) on CPU, 10-15min for 8K+ nodes. GPU investigation needed. |
| LaunchAgent labels not instance-scoped | Low | Multi-instance limitation: all instances share same LaunchAgent label. Documented in `docs/user/multi-instance.md`. |
| Graph health: 6 bugs, 4 gaps | Medium | Identified in codebase assessment (2026-04-04). Key: SymbolNode dedup, vendor nodes 44.7%, two decay systems, unprotected admin endpoints. |
| ~~Jiminy 82.4% false-ignore rate~~ | ~~High~~ | ~~FIXED (2026-04-06)~~ — Investigation (PR #273) found 0% classifier-human agreement. Root causes: disabled LLM classifier, terse action summaries, 92% untyped source nodes. Fix: enabled LLM tier, enriched summaries with content snippets + intent annotations, filtered GUIDANCE_OUTCOME to typed nodes, lowered thresholds (high 0.7→0.55, low 0.3→0.20), reduced cooldown 30→10s. Diagnostic: `scripts/jiminy_effectiveness_report.py`. |
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

# === TSDB Data Quality ===
bash scripts/tsdb_spot_check.sh                                   # Quick spot check (bash)
cd scripts && uv run python tsdb_data_review.py                   # Full diagnostic (text)
cd scripts && uv run python tsdb_data_review.py --format both     # Text + JSON output
cd scripts && uv run python tsdb_data_review.py --verbose         # Verbose with detail

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

*Last updated: 2026-04-11 — STRICT-P0P1 sprint: /strict mode foundation for deterministic agent governance. T1/T2 comprehension regression fix (P0), escalation persistence to Neo4j, strict mode toggle + state file, prompt reformulation endpoint, response classification + PreToolUse Write/Edit enforcement. New endpoints: /strict, /reformulate, /classify. New hook: pre-write-check.py. Graduated enforcement (SURFACED=advisory, WARNED+=blocking). Fail-open design.*
