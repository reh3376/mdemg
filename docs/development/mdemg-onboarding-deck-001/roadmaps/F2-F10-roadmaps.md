# F2–F10 Section Roadmaps (Consolidated)

**Status**: Epic 2 output
**Governed by**: `../presentation_spec.md`, `../global_roadmap.md`
**Sibling**: `F1-opening.md` (which gets its own file because F1 is the operator-authorized 5-beat exception; F2-F10 share the 5-beat template and consolidate here for density)

Each section below enumerates: subsection list → slide count → Mermaid diagrams → factual-provenance anchors. Target budget per global_roadmap: 101-136 total slides across F1-F10. F1 claimed 12; F2-F10 share ~89-124.

---

## F2 — Substrate Core (1 full + 1 mini, 12-16 slides)

**Full component**: Neo4j CMS + 5-layer hierarchy + Hebbian learning + consolidation.
**Mini component**: TSDB telemetry plane.

### F2.1 Neo4j CMS — the substrate (Full, 5-beat, 7-9 slides)

| Beat | Slides | Content |
|---|---|---|
| What it is | 1 | Neo4j graph DB, `mdemg-dev` protected space, `MemoryNode` + typed edges |
| Why we use it | 1 | Decision: graph > vector-only — because connections ARE the deliverable (vs. doc chunks). Rejected alternatives: ChromaDB (no graph), Weaviate (hybrid but weaker Cypher) |
| How it works | 3 | Node schema (`role_type`, `layer`, `is_archived`, `activation_confidence`); edge types (CO_ACTIVATED_WITH, GENERALIZES, ABSTRACTS_TO, IMPLEMENTS_CORRECTION, CONTRADICTS); invariant: tombstone-only (`is_archived=true`), NEVER delete |
| Why it's important | 1 | Everything downstream reads from this surface; delete == lose history; the protected-space contract is a hardcoded code-level check (`internal/safety/`) |
| How it will improve | 1 | Open: dynamic-edges rewrite (CONSOLIDATE-PERF-002 shipped); typed-edge expansion (RETRIEVAL-TYPED-EDGES-002 shipped default-on); concept layer growth under JIMINY-CEILING-BREAK-2 |

**Mermaid F2-M1**: Node + edge schema (ER-style with role_type values + layer dimension).
**Mermaid F2-M2**: 5-layer hierarchy with real promotion predicates named on each arrow.

**Provenance**: `docs/features/hidden-pattern-identity.md`, `internal/hidden/*`, `internal/safety/validator.go`, `internal/tsdb/migrations/*.sql` (schema source of truth), CLAUDE.md § HIDDEN-WEIGHT-001, HIDDEN-CHURN-001/002/003, CONSOLIDATE-PERF-001/002.

### F2.2 Hebbian learning + consolidation (Full, extends F2.1 — same component, 3-4 slides)

- Slide 1: Co-activation on retrieve — ApplyCoactivation + CoactivateSession + ApplySymbolCoactivation + ApplyNegativeFeedback (all 4 federated via EVENTGRAPH-001..004)
- Slide 2: Consolidation pipeline (22 phases, runs ~5-min cycles); the dynamic_edges O(n²) bug that CONSOLIDATE-PERF-001/002 fixed
- Slide 3: Incremental hidden clustering (CHURN-003) — 100% survival across cycles; vs. the pre-fix destroy-recreate anti-pattern
- Slide 4 (optional): Precision-weighted η (HEBB-ETA-001) — primitives shipped default-off; the predictive-coding frontier

**Mermaid F2-M3**: Hebbian flow — retrieve → activation → edge update, with federation to `reinforcement_events` TSDB.

### F2.3 TSDB telemetry plane (Mini, 2-3 slides)

- Slide 1: What TSDB stores (hypertables for `metric_samples`, `retrieval_events`, `constraint_outcomes`, `reinforcement_events`, etc. — 26 tables, inventory-adjudicated via DORMANT-CENSUS-002)
- Slide 2: Buffered CopyFrom writer pattern + retention policies (90d telemetry / 180d forensics / 365d training-corpora)
- Slide 3: Access model (TSDB-only; `/metrics` 404 by design per PROMETHEUS-SCRAPE-INVESTIGATION-001); point to `docs/features/tsdb-data-management.md` for depth

**No Mermaid**: table list + policy chart.

**Provenance**: `docs/features/tsdb-data-management.md`, `docs/api/tsdb_consumer_inventory.json`, CLAUDE.md § TSDB-CONSUME-001, EVENTGRAPH-001..004, DORMANT-CENSUS-001/002/003.

---

## F3 — Retrieval Pipeline (1 full + 2 mini, 10-14 slides)

**Full**: RRF column-voting retrieval.
**Mini**: Context fingerprinting, Sparse retrieval gate, Rerank (cross-encoder vs. neural).

### F3.1 RRF retrieval (Full, 5-beat, 6-8 slides)

| Beat | Slides | Content |
|---|---|---|
| What it is | 1 | 4 parallel columns (Embedding + BM25 + Graph + Structural) → RRF fusion → optional LLM rerank → topK |
| Why we use it | 1 | Phase 13.1 column-voting A/B: embedding-heavy weights 0.50/0.20/0.15/0.15 beat equal-weights by +5.9%, 30 improvements, 2 regressions. Default-on since 2026-05-03 |
| How it works | 3 | Fetchers → scorers → RRF formula `1/(k + rank)` with k=60 → cache key = `scorerVersion()` → downstream guards (consulting score floors, sigmoid confidence) all config-driven per RRF-SCALE-001 |
| Why it's important | 1 | Every memory recall, every Jiminy guide, every HITL candidate query — same pipeline. Latency SLO: p95<120s (ALERT_RETRIEVE_P95_MS); typed-edge expansion shipped default-on (RETRIEVAL-TYPED-EDGES-002) |
| How it will improve | 1 | Open: concrete-recall quota (RETRIEVAL-LAYER-BALANCE-001 shipped) + reverse-ref lookup (RETRIEVAL-REVERSE-LOOKUP-001 shipped) + Lever D concept bias (LEVER-D-CONCEPT-BIAS-001 shipped) all default-off; A/B results documented |

**Mermaid F3-M1**: 4-column parallel fetcher + RRF fusion.

**Provenance**: `internal/retrieval/*`, CLAUDE.md § Column-Voting Retrieval, § RRF-SCALE-001, § RETRIEVAL-TYPED-EDGES-001/002, § RETRIEVAL-LAYER-BALANCE-001, § RETRIEVAL-REVERSE-LOOKUP-001, § RETRIEVAL-DIVERSITY-001.

### F3.2 Context fingerprinting (Mini, 2-3 slides)

256-bit sparse vectors per observation; discriminates same-node-in-different-context. Default-on since Phase 14.2.3. Catalog builder allocates bits 32/32/192 (role×layer / path-segment tokens / full-paths). Per-request `?context=auto` URL override. Pointer to `docs/features/context-fingerprinting.md`.

### F3.3 Sparse retrieval gate (Mini, 1 slide)

Percentile-activation gate; default-on since Phase 14.1.1 with `SPARSE_MIN_ACTIVE=15` + `data_flow_integration` category override at 20. ~25% rerank-input reduction. Pointer to `docs/features/sparse-retrieval.md`.

### F3.4 Rerank (Mini, 1-2 slides)

Cross-encoder vs. neural A/B: parity at mean, neural 20× faster latency but default stays openai. `rerankTemperature=0.0` for determinism (RERANK-LENGTH-STRICT-001). Pre-check budget guard (`RerankMinBudgetMs=12000` LLM-path, `NeuralRerankMinBudgetMs=1500` neural-path).

---

## F4 — Jiminy + Enforcement + J17 (2 full + 1 mini, 20-26 slides)

**The headline section.** Carries the deck's weight — Jiminy is the primary user-facing cognitive surface.

### F4.1 Jiminy classifier + guidance pipeline (Full, 5-beat, 8-10 slides)

| Beat | Slides | Content |
|---|---|---|
| What it is | 1 | Inner-voice classifier: `/v1/jiminy/classify`, `/v1/jiminy/guide`, `/v1/jiminy/latest` — turns raw retrieval into imperative-ish guidance |
| Why we use it | 2 | The connection-layer bet: a well-tuned inner voice beats dumping raw retrieval into the agent's prompt. Forcing-function: pre-Jiminy, agent ignored durable rules even when surfaced |
| How it works | 4 | Hooks → warm → guide → feedback → outcome. 3 Levers: A (surface quota), B (effectiveness prior), C (actionable direct-fetch), D (concept bias). Classifier: tier1 (embedding) + tier2 (LLM) with context-mismatch credit + mechanism-scope gate + non-violation credit |
| Why it's important | 1 | Follow-rate is the receiver-side view of substrate quality. Current honest ~13%, target ≥80% per JIMINY-CEILING-BREAK-2 arc |
| How it will improve | 2 | Arc Phases 1-5: corpus purge (shipped JIMINY-CORPUS-003) → retrieval precision (LEVER-C-TIGHTEN-002) → classifier semantics (CLASSIFIER-CONTEXT-002) → HITL velocity + retrain (Phase 4a shipped) → model swap (future) |

**Mermaid F4-M1**: Jiminy enforcement + guidance flow (hook → classify → warn → block → feedback → outcome).
**Mermaid F4-M2**: Lever A/B/C/D composition on the surface pool.

**Provenance**: `docs/features/jiminy-strict.md`, `docs/features/jiminy-actionability.md`, `docs/features/jiminy-metric-partition.md`, `docs/features/jiminy-lever-d-concept-bias.md`, `docs/features/jiminy-lever-c-activation.md`, `docs/features/constraint-detector.md`, `docs/features/hitl-auto-curation.md`, CLAUDE.md § JIMINY-ENFORCE-001..005, § JIMINY-CEILING-BREAK-2, § JIMINY-CORPUS-001/002/003, § LEVER-C-TIGHTEN-001/002, § JIMINY-CLASSIFIER-CONTEXT-001/002, § JIMINY-METRIC-PARTITION-001, § JIMINY-ROLETYPE-ADAPTER-001.

### F4.2 Enforcement arc — 5 sprints (Full, extends F4.1, 6-8 slides)

5-sprint arc shipped 2026-08-01→03 that elevated Jiminy from advisor to enforcer:
- Slide 1: ENFORCE-001 — strict-default + alert-on-block + fail-open-with-marker
- Slide 2: ENFORCE-002 — Bash coverage (2-check pipeline: destructive-guard fail-closed + classify fail-open + read-only whitelist)
- Slide 3: ENFORCE-003 — operator escape-hatch override CLI (`mdemg jiminy override apply|list|revoke` + JSONL audit)
- Slide 4: ENFORCE-004 — RSIC-consumable blocked_true_positive / blocked_false_positive outcome types
- Slide 5: ENFORCE-005 — post-hoc missed-violation detector via correction observations
- Slide 6: Current escalation state machine (surfaced → warned → escalated → blocked); ACCUMULATE-001 fix that first produced warned states ever
- Slide 7: Arc-adjacent follow-ups shipped (ENFORCE-OVERRIDES-TSDB, ENFORCE-UI-OVERRIDES, ENFORCE-AUTO-EXECUTE with 5-layer guard sequence)

**Mermaid F4-M3**: Enforcement outcome taxonomy (deny-wanted / deny-overridden / missed-entirely) → RSIC inputs.

**Provenance**: CLAUDE.md § JIMINY-ENFORCE-001..005, § ENFORCE-004-FOLLOWUP, § ENFORCE-OVERRIDES-TSDB, § ENFORCE-UI-OVERRIDES, § ENFORCE-AUTO-EXECUTE, § ESCALATION-ACCUMULATE-001.

### F4.3 J17 protocol — compressed-stateful messaging (Mini, 4-6 slides)

- Slide 1: What J17 is — compressed message format `TYPE:SEV|code|annotations|esc:N|src:NODE_ID`; semantic layer over Jiminy
- Slide 2: Protocol dims: TICKET (signed), SEQ (monotonic), FMT, SEV (!=must, ?=should, ~=info), ESC (0..3), CODES (C/X/F/D/P/L)
- Slide 3: Comprehension-keyed tier gate (`J17_TIER_GATE_MODE=comprehension` — J17-TIER-GATE-001). T1 denser encoding gated on comprehension axis, not compliance-trust
- Slide 4: NLI calibration + the bias-alert fix (ALERT-TRUTH-001, DASHBOARD-TRUTH-001 E2 — the alert was permanently red by construction)
- Slide 5: Agent briefing packet v1.1 — the recent compression-doc work (`docs/features/j17-agent-briefing-packet.md`)

**Mermaid F4-M4**: J17 message anatomy diagram.

**Provenance**: `docs/features/j17-*.md`, CLAUDE.md § J17-TIER-GATE-001, § ALERT-TRUTH-001, § DASHBOARD-TRUTH-001.

---

## F5 — LoRA + Qwen + Model Distribution (1 full + 1 mini + homebrew mini, 12-16 slides)

### F5.1 LoRA on Qwen3-14B — mdemg-llm-v1 (Full, 5-beat, 6-8 slides)

| Beat | Slides | Content |
|---|---|---|
| What it is | 1 | Phase 5 SFT: single-tier LoRA on `mlx-community/Qwen3-14B-4bit`, rank=32 α=64, 7 dense target modules, served as GGUF Q5_K_M via llama-server :8102 |
| Why we use it | 1 | The connection-layer bet applied to the LLM itself: fine-tuned on MDEMG's own call-site corpora (16 call sites) → better per-site structured outputs than generic gpt-4-mini |
| How it works | 3 | Pipeline: MLX safetensors (training) → `mlx_lm.fuse --dequantize` → f16 GGUF → `llama-quantize Q5_K_M` → llama-server with `--ctx-size 32768 --parallel 4`. Standing policies: no-tool-calling (16 sites all single-shot structured output) + overfitting prevention (3-epoch cap, early-stop on val-degradation) |
| Why it's important | 1 | Aggregate 0.9188 on 16-task augmented eval; beats gpt-mini (0.8317). The honest number comes from the APE-REFLECT-EVAL-REFRESH-001 cleanup — eval rows previously frozen pre-APE-PROMPT-BUDGET-001 budgets, which clipped. |
| How it will improve | 1 | FT-RECURSIVE arc (6a shipped observability, 6b shipped actuator default-off, 7 shipped promotion defense-in-depth, 9 complete). Phase E1-E3 corpus audit → curate → retrain (shipped stripped corpus 503 rows + task-behavior 7000 rows = 7503 rows E3 target) |

**Mermaid F5-M1**: Model pipeline MLX → GGUF → llama-server.

**Provenance**: `docs/features/jiminy-lora-qwen.md`, `docs/development/ft-lora/00_README_v2.md` (canonical), `docs/features/ft-recursive-loop.md`, CLAUDE.md § MDEMG Fine-Tuning Shipped State + Policies, § PHASE-E1/E2-CORPUS-*, § APE-REFLECT-EVAL-REFRESH-001.

### F5.2 Recursive-retraining loop (Mini, 3-4 slides)

- Slide 1: FT-RECURSIVE-001 Phase 6a — observability (`mdemg ft-loop report-stage`); scheduled_job_events `ft-loop:<stage>`; staleness alert
- Slide 2: FT-RECURSIVE-002 Phase 6b — the actuator (ledger + trigger + compute lease + controller + 5-stage pipeline). Default-off (`FT_LOOP_ENABLED=false`). Drill-caught 2 convert-environment defects
- Slide 3: FT-RECURSIVE-003 Phase 7 — promotion defense-in-depth (pre-swap structural canary → fail-closed SwapServing → post-swap tripwire auto-rollback)
- Slide 4: FT-RECURSIVE-004 Phase 9 — drift detection (`ft_production_drift` alert) + loop-never-ran alert + scheduled benchmark runner

**Mermaid F5-M2**: Recursive loop (drift → export → curate → train → convert → gate → promote).

### F5.3 Homebrew distribution (Mini, 3-4 slides)

- Slide 1: `mdemg model pull|list|verify|remove|where` — Ollama Library as distribution channel (not runtime); 3 fused GGUF quants published
- Slide 2: Ollama manifest layout → symlink → SHA-verify against embedded `quant_manifest.json`; air-gapped override via `MDEMG_MODEL_MANIFEST_PATH`
- Slide 3: RAM auto-detection (darwin: `sysctl hw.memsize`, linux: `/proc/meminfo`) + dispatch via `MDEMG_MODEL_RAM_TIERS` JSON
- Slide 4: Why not Ollama/LM Studio/MLC-LLM runtime — llama.cpp llama-server is the architecturally-bounded KV runtime

**Provenance**: `docs/features/local-model-distribution.md`, CLAUDE.md § MODEL-DIST-001/002, § HOMEBREW-INSTALLER-QWEN-UPDATE-001.

---

## F6 — UxTS Framework Family (1 full, 7-10 slides)

### F6.1 UxTS forcing-function family (Full, 5-beat, 7-10 slides)

| Beat | Slides | Content |
|---|---|---|
| What it is | 1 | 16 framework specifications — each a schema + spec dir + CI runner + matrix adjudication. UATS/UVTS/ULTS/UBTS/UOBS/UITS/UDTS/UETS/UOTS/UATS01/UBENCH/UAITS/UDTS/UTDS/USTS/UAMS |
| Why we use it | 1 | Forcing function: when a drift class is caught (e.g., ULTS prompt hashes drifting from code), the fix is a UxTS spec that CI gates on. Prevents regression by making the drift class itself impossible |
| How it works | 3 | Pattern: schema in `docs/tests/<x>/schema.json` → specs in `docs/tests/<x>/specs/*.<x>.json` → runner in `docs/tests/<x>/runners/` → CI job gating. UXTS_FRAMEWORK_MATRIX.md single source of truth for count |
| Why it's important | 1 | UATS caught the pre-bash-check-reformulate 500 defect; ULTS catches prompt drift; UBENCH catches benchmark drift; UOBS catches metric-emission drift. CI gating ensures a class never re-opens |
| How it will improve | 1 | New frameworks added when a new drift class is discovered. Follow the pattern: new drift → spec it → gate CI → close the class |

**Mermaid F6-M1**: UxTS pattern — drift class → schema → spec → runner → CI → matrix.
**Mermaid F6-M2**: 16-framework matrix (table-of-tables visual).

**Provenance**: `docs/features/uxts-frameworks.md`, `docs/development/UXTS_FRAMEWORK_MATRIX.md`, CLAUDE.md § UXTS-CI-001, § UATS-GAP-001, § UXTS-GAP-001, § UBENCH-framework.

---

## F7 — Ops (Grafana/alerts/backup/HITL/RSIC/scheduled-jobs/supervisor) (2 full + 2 mini, 14-18 slides)

### F7.1 RSIC self-improvement + alert correctness family (Full, 5-beat, 6-8 slides)

| Beat | Slides | Content |
|---|---|---|
| What it is | 1 | RSIC = Recursive Self-Improvement Cycle. 7-dim health assessment (retrieval/memory/edge/task/guidance/protocol/synergy) via `/v1/self-improve/assess`; LLM reflector produces insights → recommended actions → dispatcher |
| Why we use it | 1 | Honest observability of substrate state. The alert-correctness family (ALERT-TRUTH-001, DASHBOARD-TRUTH-001/002/003) is the sister work: most "scary" alerts were measurement artifacts, not regressions |
| How it works | 3 | Health formula: `overall = Σ(w_i·c_i·s_i) / Σ(w_i·c_i)` normalized weighted-confidence sum (DH-005). Alert rules: TSDB-query over `metric_samples` with COALESCE idle-safe aggregates; distinct Service labels (NOSILENT-001 cooldown-key contract). RSIC-STORM-001 reservation fix |
| Why it's important | 1 | Latest sprint MDEMG-ALERT-NOISE-REDUCTION-001 (2026-10-22) closed 2 chronic false-fire classes. ORPHAN-ALERT-001 shape generalizes: never alert on "no data = regression" |
| How it will improve | 1 | Open class: per-space graph-health alerts need min-nodes floor. New alerts must follow NOSILENT-001 + TSDB-CONSUME-001 contracts |

**Mermaid F7-M1**: RSIC loop with 7-dim gauges → reflector → actions → dispatcher.

**Provenance**: `docs/features/rsic-orchestration.md`, `docs/features/rsic-feedback-loop.md`, `docs/features/rsic-guidance-health-floors.md`, CLAUDE.md § ALERT-TRUTH-001, § DASHBOARD-TRUTH-001/002/003, § ORPHAN-ALERT-001, § NODE-DROP-CALIBRATION-001, § MDEMG-ALERT-NOISE-REDUCTION-001.

### F7.2 HITL review + curation (Full, 5-beat, 4-6 slides)

| Beat | Slides | Content |
|---|---|---|
| What it is | 1 | Human-in-the-loop review surface: 17 datasets registered (Guidance Corpus + 16 LLM call-site datasets). Dataset-agnostic via `ReviewableDataset` interface |
| Why we use it | 1 | Corpus-quality lever. Operator-authored gold is the retrain-trigger signal (JIMINY-RELEVANCE-001 arc) |
| How it works | 2 | `/v1/review/*` endpoints + Review UI tab. Buffered writer. HITL-AUTO-DISMISS-001 added `NonReinforcingApplier` for dim=0 dismissals. AUTOGRADE-SCHEDULE-001 runs 6h scheduled autograde with `:auto` grader_id prefix invariant |
| Why it's important | 1 | Reinforcement MUTATES substrate (`conversation.Correct`); reversible via inverse-delta `Reverse` endpoint. All 17 datasets gate on `rubric_version` idempotency |
| How it will improve | 1 | Open: guidance dataset extension (HITL-CURATION-003 shipped). UI keyboard-driven bulk-grade (JIMINY-HITL-VELOCITY-001 shipped: 0..4 keys, auto-advance 400ms) |

**Mermaid F7-M2**: HITL loop (candidate queue → operator grade → sink → substrate mutation OR L0 correction + retrain-signal).

**Provenance**: `docs/features/hitl-review.md`, `docs/features/hitl-auto-curation.md`, CLAUDE.md § HITL-REVIEW-001, § HITL-CURATION-002/003, § HITL-AUTO-DISMISS-001, § AUTOGRADE-SCHEDULE-001, § JIMINY-HITL-VELOCITY-001.

### F7.3 Backup/restore + scheduled jobs (Mini, 2-3 slides)

- Slide 1: Dual-sink backup (Neo4j dump + TSDB export). Partial always bundles `mdemg-dev`. Quota retention NEVER deletes newest-per-type (BACKUP-RESTORE-VERIFY-001)
- Slide 2: Scheduled-job health: `scheduled_job_events` V0024 + rule `scheduled_job_recent_failure` + rule `backup_no_recent_success` ("never ran" guarantee). ⚠️ NOSILENT-001 distinct-service cooldown-key contract
- Slide 3: Goroutine supervisor (SUPERVISOR-002) — 15 supervised loops; nil-return = intentional-completion contract; sliding-window restart budget

**Provenance**: `docs/features/backup-restore.md`, `docs/features/scheduled-job-health.md`, `docs/features/goroutine-supervisor.md`, CLAUDE.md § BACKUP-RESTORE-VERIFY-001, § NOSILENT-001, § SUPERVISOR-002.

### F7.4 Grafana (Mini, 2 slides)

- Slide 1: 8 dashboards + 1 Catppuccin theme contract (JIMINY-RULES-UI-001 dashboard-contrast rule)
- Slide 2: Grafana dashboards shipped via `//go:embed` + materialize-on-init (RELEASE-HYGIENE-001). `make sync-grafana-embed` + CI drift check

**Provenance**: `deploy/docker/grafana/`, `internal/cli/grafana_templates/`, CLAUDE.md § RELEASE-HYGIENE-001.

---

## F8 — CLI, Hooks, MCP, Multi-instance (2 mini + mentions, 6-8 slides)

### F8.1 Hooks + MCP channels (Mini, 2-3 slides)

- Slide 1: 7 hooks (session-start, prompt-context, post-tool-observe, pre-compact, pre-bash-check, pre-write-check). HOOKSYNC-001 contract: `.claude/hooks/*` + `internal/cli/hook_templates/*` byte-match modulo `{{SPACE_ID}}` — CI-gated
- Slide 2: MCP memory channel — 23 tools; `.mcp.json` sets `MDEMG_SPACE_ID=mdemg-dev` so agent + hook share one space (MCP-REVIVE-001)
- Slide 3: Hook stdin contract (HOOKWIRE-001) — `prompt` not `user_prompt`; `tool_response` not `tool_output`; always normalize via `_response_text`

**Provenance**: `docs/features/mcp-memory-channel.md`, `docs/features/hook-channel-health.md`, CLAUDE.md § HOOKSYNC-001, § HOOKWIRE-001, § MCP-REVIVE-001.

### F8.2 CLI commands (Mini, 2-3 slides)

- Slide 1: `mdemg init|serve|teardown|upgrade|data|tsdb|synergy|graph|concepts|corrections|jiminy|j17|watchdog|hooks|service|model|ft-loop|review|eventgraph|symbols` — the surface area
- Slide 2: Recent CLI additions (JIMINY-RULES-UI-001's `mdemg jiminy constraint mark/set-class/seed-classes`; INSTANCE-ID-PIN-001's instance-id resolver)

### F8.3 Multi-instance (Mention, 1-2 slides)

Each instance: 5 containers, 6 ports, ~2.3 GiB RAM. `COMPOSE_PROJECT_NAME=mdemg-{dirname}` isolation. Known limitation: LaunchAgent labels not instance-scoped. Pointer to `docs/user/multi-instance.md`.

---

## F9 — Beta Program + Security Hardening + Dormant Census (2 mini + mentions, 4-6 slides)

### F9.1 Beta corpus loop (Mini, 2-3 slides)

- Slide 1: `mdemg data export` + `mdemg beta-import <bundle>` + `mdemg beta-janitor` — closes the inbound half of the beta program
- Slide 2: Trust gates — per-JSONL SHA-256 + privacy re-scrub + attribution-remap to `beta-tester-<submission_id>` synthetic space
- Slide 3: 30-day retention via janitor (BETA-IMPORT-001)

**Provenance**: `docs/features/beta-import.md`, CLAUDE.md § BETA-IMPORT-001.

### F9.2 Security + dormant-census hygiene (Mini, 2 slides)

- Slide 1: SEC-TRANCHE-2/3 — path-injection sanitizer (`pathsafe.SafeJoinUnderDir`), URL-scheme deny-list widening, defensive 10000-element allocation caps. CodeQL contract: `#nosec` is gosec-only — must route through recognized sanitizers OR dismiss via GitHub API
- Slide 2: DORMANT-CENSUS-001/002/003 — three forcing-function registries (routes / TSDB tables / metrics). CI merge-blocking. Latest MDEMG-ALERT-NOISE-REDUCTION-001 (2026-10-22) + MDEMG-DOCS-INGEST-002 shape

**Provenance**: `docs/features/sec-tranche-*.md` (if exists), CLAUDE.md § SEC-TRANCHE-2/3, § DORMANT-CENSUS-001/002/003, § MDEMG-DOCS-INGEST-002.

---

## F10 — Conclusion (1 full, 6-8 slides)

### F10.1 What's left + end goals + continuous improvement (Operator-authored, 5-beat variation, 6-8 slides)

| Beat | Slides | Content |
|---|---|---|
| Current state (honest) | 1 | Follow rate ~13% today; aggregate 0.9188; 44 pending alerts (most measurement artifacts); 2 fresh PRs (#694/#695) in-flight. Trajectory language per the architectural directive |
| What's shipped vs. what's left | 2 | 2-column: shipped (105 pins) vs. open frontier (JIMINY-CEILING-BREAK-2 Phase 2-5; FT-RECURSIVE-004 drift observation window; HITL corpus growth target N=500+ golds for Phase 4b retrain) |
| End goal (vision) | 1 | ≥80% actionable follow rate, >10 beta testers active, FT recursive loop auto-promotes once per quarter without operator intervention, UxTS matrix stable at steady-state framework count |
| Why it's important | 1 | Vision tie-back: cognitive substrate must earn its persistence; a cognitive substrate with <50% follow rate is advisory + ignored; must cross the trust threshold |
| How it will improve (continuous) | 1-2 | Named arcs: JIMINY-CEILING-BREAK-2 (follow rate), FT-RECURSIVE-005+ (fully autonomous retrain), UxTS extensions (new drift classes), beta program growth, substrate growth (more role types / more layers / more edge types) |

**Mermaid F10-M1**: Named-arc trajectory map (what's shipped → open arcs → vision).
**Mermaid F10-M2**: Continuous-improvement loop diagram tying all 4 learning time constants together.

**Provenance**: all of CLAUDE.md + operator judgment. This section is explicitly operator-authored per sprint_plan.md.

### F10.2 Final slide — call to action (1 slide)

Where to go next:
- `CLAUDE.md` for the architecture pins
- `docs/features/*.md` for reference material
- `/ui` dashboard at `localhost:9999/ui` for the live surface
- `grafana` dashboards for observability
- `mdemg jiminy mode strict` to enable enforcement
- `/strict` mode to see guidance in action

Thank-you + where-to-find-me + quarterly-refresh commitment.

---

## Diagram Inventory Summary (F2-F10)

| Section | Diagram IDs | Est. count |
|---|---|---|
| F2 | F2-M1, F2-M2, F2-M3 | 3 |
| F3 | F3-M1 | 1 |
| F4 | F4-M1, F4-M2, F4-M3, F4-M4 | 4 |
| F5 | F5-M1, F5-M2 | 2 |
| F6 | F6-M1, F6-M2 | 2 |
| F7 | F7-M1, F7-M2 | 2 |
| F8 | (optional sequence diagrams) | 0-1 |
| F9 | (optional) | 0 |
| F10 | F10-M1, F10-M2 | 2 |
| **F1-F10 total** | | **~21 Mermaid diagrams** (plus F1's 5 = 26) |

Each Mermaid stays ≤20 nodes per presentation_spec §6; split-on-overflow discipline.

## Non-Goals (reaffirmed across F2-F10)

- No per-sprint decks (would be ~100 decks and all would rot)
- No deep code reads — explanations stay at the behavior/invariant layer
- No live-fetched metrics in the deck (operator refreshes quarterly)
- No PM-flavored copy — audience is senior engineers

## Documents Accessed

- F1-opening.md (sibling)
- ../sprint_plan.md
- ../presentation_spec.md
- ../global_roadmap.md
- CLAUDE.md § Architecture Notes (all ~105 pins, scanned as source material)
- CLAUDE.md § Project Context, § Testing, § Enforced Protocols
- `docs/development/UXTS_FRAMEWORK_MATRIX.md`
- `docs/features/*.md` (full directory, used as reference for each roadmap's provenance anchors)
