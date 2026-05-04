# Sprint POST-FT-LORA-PHASE14 — Notes 05+06: Sparse Fingerprints + Percentile Activation Gate

> **EXECUTED (narrowed) 2026-05-04.** Initial plan combined Note 05 (sparse fingerprints) + Note 06 (percentile activation gate) in one sprint. Operator-approved narrow-close after Epic 0+1+2 produced verdict-driven design questions for both: (a) Note 06 120q full failed per-question gate at MIN=10/p95 (mean parity, 7 boundary regressions concentrated in `architecture_structure`); (b) Epic 0 forensic found `whk-wms` has 0 symbol/0 role properties → spec's static 64/64/64/64 catalog bit allocation needs adaptive redesign for non-code spaces. Splitting into:
> - **Phase 14 (this sprint, narrow)**: Epic 0 forensic + Epic 1 Note 06 gate code + Epic 2 A/B verdict + V0019 metrics + Phase 13 Epic 6 audit-writer fix + Phase 11+ feature-doc backfill.
> - **Phase 14.1**: Note 06 adaptive per-category percentile (queued).
> - **Phase 14.2**: Note 05 with adaptive catalog bit allocation (queued).
>
> Sprint closes with Note 06 flag-off. Mirrors Phase 13 → Phase 13.1 split precedent.

## Context

Phase 13.1 shipped column-voting retrieval default-on with the embedding-heavy preset (commit `6ed411e`, merged in PR #365). The `consensus_strength` per-call signal is now flowing on every retrieve, persisted to V0017 `retrieval_audit`. Phase 14 is the first sprint in the **HTM-extension series** that consumes `consensus_strength` and the column-vote breakdowns to add two cortex-aligned mechanisms:

- **Note 05 — Context-Specific Activations**: each observation gets a 256-bit sparse "context fingerprint" (the bit positions encode which symbols / files / roles co-activated when the observation was captured). Retrieval candidates are weighted by fingerprint overlap with the query's current context. Solves the polysemy problem (e.g. `ErrorHandler` in `auth/` vs `payments/`) without duplicating graph nodes.

- **Note 06 — Sparse Activation Gate**: replace "score everything, return top-K" with "only candidates whose score crosses the activation percentile fire." Default p98 → top 2% candidates fire. Sharpens precision; enables set-algebra across queries (multi-query workflows); reduces downstream LLM context-window load on rerank + Jiminy-Guide.

Both are **HTM-aligned** (Hawkins & Ahmad 2016) and **parallel-safe** with each other. They compose: Note 06 gates the active set; Note 05's context-similarity is one of the signals contributing to per-candidate score before gating.

**Why now.** Phase 13 + 13.1 wired the `consensus_strength` signal end-to-end and emitted Phase 13 Epic 5 hooks (`RetrievalRerankConsumeConsensus`, `DH005ConsumeConsensus`) flag-off-but-wired. Phase 14 is the next sprint that uses these signals.

**Phase chain.** Phase 11.6.x → 12 → 11.6.3 → 13 → 13.5 → 13.5-telemetry → 13.1 → **Phase 14 (this)** → Phase 14.1 (per-category weights from 13.2 if needed) → future Notes 01/02/03/07/08/09.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | POST-FT-LORA-PHASE14 |
| Title | Notes 05+06 — Sparse Fingerprints + Percentile Activation Gate |
| Date | 2026-05-04 (plan) |
| Branch | `reh3376_dev01` (synced with main at `33b72d3`) |
| Predecessor | Phase 13.1 (commit `6ed411e`) |
| Successor | Phase 14.1 — Note 05/06 follow-up tuning OR Phase 13.2 per-category weights, whichever is queued |
| Type | Code-large + research (~1500 LOC production + ~800 LOC tests; 2 Neo4j migrations + 2 TSDB migrations + 1 background job + new package + new retrieval column) |
| Risk | MEDIUM-HIGH (changes the retrieval response shape — gated set < unfiltered set on default p98; downstream consumers must handle smaller K) |
| Budget | $25–50 OpenAI (multiple A/B cycles per Note + combined; full 120q runs for each major variant) |
| Effort estimate | 18 dev-days (~3–4 weeks) — Note 05 (~10.5 days) + Note 06 (~5.5 days) + Phase 11+ feature-doc backfill (~2 days for 5 new + 1 update) |
| New TSDB migrations | V0019 (`context_catalog_versions` — historical catalog snapshots) + V0020 (`sparse_gate_metrics` — active-set sizing observability) |
| New Neo4j migrations | V0028 (`MemoryNode.context_fingerprint_active`/`_version` properties) + V0029 (`ContextCatalog` node label + indexes) |
| Post-sprint artifacts | `internal/hidden/context_catalog.go` (new), `internal/conversation/fingerprint.go` (new), `internal/retrieval/column_context.go` (new), `internal/retrieval/gate.go` (new), `internal/cli/migrate_context_fingerprint.go` (new), 4 new env-var groups, ~12 new config knobs, 2 Grafana dashboard updates, post-doc + plan-frozen + 2 feature docs |

## 2. Problem Statement

### Problem A — polysemy without graph duplication (Note 05)

MDEMG today has **one MemoryNode per symbol with one embedding**. When the same symbol appears in unrelated contexts (`ErrorHandler` in `auth/auth.go` vs `payments/payments.go`), retrieval can't tell them apart. Workarounds (space scoping, role filtering, file-path filtering) only help when the caller knows to specify them.

The HTM solution: keep one node per symbol (graph stays stable) but each *observation* of that symbol carries a sparse fingerprint of what else was active when it was captured. Retrieval ranks observations by fingerprint similarity to the query's current context. This is the distal-dendrite story — same neuron, different distal inputs differentiate context.

**Concrete impact**: q `69` (`secretsManager + Azure Key Vault`) is now solved by Phase 13.1 because the embedding-heavy weights happened to surface the right files. Future queries where the right answer depends on context — e.g. "How does this error pattern get handled in the inventory module specifically?" — still don't have a per-context discriminator. Fingerprints add the missing axis.

### Problem B — dense ranked output noise (Note 06)

Today's retrieval returns top-K=20 candidates. Many of those are noise — barely above the K-th percentile. Three specific costs:

1. **Rerank prompt bloat**: rerank passes 20 candidates to the LLM in a 6–8K token prompt. If the right answer is in candidates 1–3, the remaining 17 are paying tokens for nothing.
2. **Consulting / Jiminy.Guide**: pack top-K into context windows. Same waste.
3. **Multi-query workflows can't compose**: "what did we see about X AND Y?" requires intersecting two ranked lists; with full top-K both sides, the intersection is dominated by the noise tier of each.

The HTM solution: a percentile-based **activation gate** at the end of the retrieval pipeline. Only candidates whose final score crosses the threshold "fire." Set sizes become variable: a sharp query may fire 3; a diffuse query may fire 25 (clamped at MAX_ACTIVE).

### Why combined into one sprint

- Both consume Phase 13's `consensus_strength` (Note 06 directly; Note 05 contributes a column whose vote affects consensus)
- Both share the same A/B harness (UVTS quick + full)
- Note 05's `ContextColumn` is a natural fit in Phase 13's column-voting architecture — adding a 5th column rather than a parallel scoring pathway
- Note 06's gate operates on the consensus-aggregator output regardless of how many columns vote — Note 05 doesn't change Note 06's contract

Splitting them into two sprints would duplicate the A/B + docs cycle without reducing risk.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Note 06 sparse activation gate | `internal/retrieval/gate.go` (new, ~80 LOC), `internal/retrieval/gate_test.go` (~120 LOC) |
| 2 | Note 06 integration into retrieval pipeline | `internal/retrieval/service.go` (post-aggregator, pre-rerank cut) |
| 3 | Note 06 config knobs (4) | `SPARSE_RETRIEVAL_ENABLED`, `SPARSE_ACTIVATION_PERCENTILE`, `SPARSE_MIN_ACTIVE`, `SPARSE_MAX_ACTIVE` |
| 4 | Note 06 per-query override | `?sparse=false` or `?sparse_percentile=N` query param |
| 5 | Note 05 fingerprint schema | Neo4j V0028 (`MemoryNode.context_fingerprint_active`, `_version`); type extension in `internal/hidden/types.go` |
| 6 | Note 05 catalog node + indexes | Neo4j V0029 (`ContextCatalog` label, per-space versioning) |
| 7 | Note 05 catalog package | `internal/hidden/context_catalog.go` (new, ~250 LOC) — assignment, lookup, refresh |
| 8 | Note 05 catalog refresh job | `internal/ape/cycle.go` macro-cycle hook (weekly per space) |
| 9 | Note 05 fingerprint computation at observation time | `internal/conversation/fingerprint.go` (new, ~150 LOC) wired into `service.observe` |
| 10 | Note 05 context column | `internal/retrieval/column_context.go` (new, ~120 LOC) — 5th RRF column or scoring term, depending on §13 fork |
| 11 | Note 05 query-side fingerprint | request body `context_fingerprint` field; `internal/api/handlers.go` populate-from-session helper |
| 12 | Note 05 strict-mode filtering | `?strict_context=true` + Jaccard threshold knob |
| 13 | Note 05 historical backfill CLI | `mdemg migrate context-fingerprint --space <id>` (resumable, batched) |
| 14 | TSDB V0019 + V0020 migrations | catalog version history + sparse-gate metrics |
| 15 | Prometheus metrics (~7 new) | `mdemg_context_fingerprint_size`, `mdemg_context_catalog_version`, `mdemg_context_similarity_score`, `mdemg_observations_without_fingerprint`, `mdemg_sparse_gate_active_count`, `mdemg_sparse_gate_dropped_fraction`, `mdemg_sparse_gate_threshold` |
| 16 | Grafana dashboard updates | new "Context Fingerprinting" + "Sparse Retrieval" rows on `mdemg-rsic` and `mdemg-overview` dashboards |
| 17 | Per-Note A/B verification | UVTS quick × 4 (gate-only, fingerprint-only, both, baseline) → full 120q on winners |
| 18 | Conditional default flips | `SPARSE_RETRIEVAL_ENABLED` and Note 05's `RETRIEVAL_CTX_SIMILARITY_WEIGHT` flipped per A/B verdicts |
| 19 | Sprint plan + post-doc + 2 feature docs | `sprint_plan_phase_14_*.md`, `phase_14_post.md`, `docs/features/context-fingerprinting.md`, `docs/features/sparse-retrieval.md` |

**Out of scope (deferred):**
- Per-space adaptive percentile (Note 06 §3.3) — extension knob, not core
- Re-fingerprinting on catalog refresh (Note 05 §4.3) — backfill job + version-mismatch fallback is sufficient for v1
- HTM sequence memory (Note 08) — depends on fingerprints but is its own sprint
- Active inference unification (Note 09)
- Phase 13.2 per-category weight tuning (still queued)
- Phase 13.6 backend-agnostic naming cleanup (still queued)

**Constraints (hard, MEMORY):**
- **Sequential epics** — diagnostic before implementation; per-Note A/B before combined; no parallelization across epics
- **No hardcoded values** — every threshold/weight/percentile/clamp goes through env var
- **Plan-options pattern** — at least 4 design forks (column vs scoring-term, percentile defaults, catalog refresh cadence, gate ordering vs context column) decided in §13 with cited evidence after Epic 0
- **Single batched commit at sprint close**
- **Sprint summary on PR comment**
- **CUIDv2 for any new IDs** (catalog versions, gate-metrics row IDs)
- **max_tokens ≥ 3000, latency_budget_ms ≥ 15000** — gate reduces context-window load but no new LLM call sites
- **Live-testing required** (Tier 3) — A/B against real `whk-wms` space + real grader
- **A/B merge gate** — same as Phase 13.1: B mean ≥ A mean AND no per-question regression > 10%
- **Default flips** are conditional on full 120q passing (per Phase 13.1 precedent)

## 4. Dependencies

**Consumed (code, pre-existing — reuse):**
- `internal/retrieval/consensus.go` — RRF aggregator (Phase 13)
- `internal/retrieval/scoring_rrf.go` — `Service.ScoreAndRankRRF` entry point (Phase 13 + 13.1)
- `internal/retrieval/columns/*` — 4 existing columns (Phase 13 Epic 1+2)
- `internal/retrieval/cache.go::CacheKey` + `Service.scorerVersion()` — namespace isolation (Phase 13.1 weight extension)
- `internal/conversation/service.go` — observation pipeline; `observe` is the natural fingerprint-computation hook
- `internal/hidden/types.go` — observation struct
- `internal/ape/cycle.go` — macro-cycle scheduler (weekly catalog refresh)
- `internal/config/config.go` — env-loading pattern
- `internal/tsdb/migrations/017_retrieval_audit.sql` — `consensus_strength` per-call rows (analysis input for percentile selection)
- `docs/tests/uvts/runners/uvts_runner.py` + `uvts_ab_compare.py` — A/B harness
- `scripts/phase13_1_ablation_runner.py` — extensible to sweep `SPARSE_ACTIVATION_PERCENTILE` ∈ {0.95, 0.98, 0.99}

**Consumed (data):**
- TSDB V0017 `retrieval_audit` rows — primary input for Epic 0 percentile-distribution analysis
- Neo4j `whk-wms` space — primary A/B target (matches Phase 13/13.1 baseline)
- Phase 13 baseline grades at `/tmp/uvts-baseline/grades.json` (16q reference)
- Phase 13.1 baseline grades at `/tmp/phase13_1_full/baseline/grades.json` (120q reference)

**Consumed (compute):**
- mdemg HTTP API
- llama-server at port 8102 (Phase 13.5 substrate, stable)
- TimescaleDB
- OpenAI API for UVTS grader (`gpt-5.4-mini`)

**External services:** mdemg + TSDB + OpenAI. No new infra.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch clean post-PR-365 merge; mdemg + llama-server healthy; TSDB schema 18; Phase 13.1 baseline grades preserved.

### Epic 0 — Preflight + score-distribution forensic analysis

> The percentile defaults (Note 06) and Jaccard thresholds (Note 05) need to be calibrated to the actual score distributions in production. Setting them blindly is the same mistake Phase 13 made with equal weights.

1. Query V0017 `retrieval_audit` for the past 7 days of retrieve calls — extract the per-row top_k_node_ids + scoring deltas from V0016 uvts_results
2. For each call, compute the score gradient from rank-1 to rank-K — identify the "knee" (where score drops off sharply). Plot or describe the distribution.
3. Compute candidate gate percentiles (95th, 98th, 99th) on real data — what fraction of queries fire ≥3? ≥10? ≥50?
4. Inventory current `consulting.classify`, `jiminy.codegen`, `jiminy.evaluate*` call shapes — how many candidates do they pass to LLM context today? What top-K assumption is baked in?
5. Audit existing Neo4j schema for properties that conflict with `context_fingerprint_active` (V0028 must MERGE without breaking existing constraints)
6. Snapshot current memory observation distribution per space (count, mean co-active sets per session) — informs catalog bit budget

**Output:** `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` with:
- Recommended `SPARSE_ACTIVATION_PERCENTILE` default (likely 0.98 per spec, but data may say otherwise on whk-wms)
- Recommended `SPARSE_MIN_ACTIVE` and `SPARSE_MAX_ACTIVE` (clamps)
- Recommended Jaccard threshold for Note 05 strict mode (likely 0.25, may shift based on observed fingerprint sizes)
- Flag any observation/catalog conflicts before V0028/V0029 ship

**Gate:** distribution analysis committed; all defaults are evidence-cited from real `retrieval_audit` rows.

### Epic 1 — Note 06 sparse activation gate (smaller, ships first)

> Smaller of the two; ships first to validate the A/B harness can detect a meaningful change before adding the more invasive Note 05 changes. Provides intermediate operational signal that informs Note 05 design.

1. New `internal/retrieval/gate.go` — `ApplySparseGate(candidates, cfg)` returning gated set + threshold metadata
2. Wire into `Service.ScoreAndRankRRF` (and legacy `ScoreAndRank`) post-aggregation, pre-rerank
3. 4 new config knobs: `SPARSE_RETRIEVAL_ENABLED` (default false initially), `SPARSE_ACTIVATION_PERCENTILE` (Epic 0 default), `SPARSE_MIN_ACTIVE` (3), `SPARSE_MAX_ACTIVE` (50)
4. Per-query override via `?sparse=false` / `?sparse_percentile=N`
5. Below-threshold candidates returned in debug response field (`debug.below_threshold[]`) when JiminyEnabled or debug requested
6. 3 new Prometheus metrics (active_count, dropped_fraction, threshold)
7. TSDB V0020 `sparse_gate_metrics` hypertable — per-call gate observability beyond Prometheus reset cycle
8. Tier 1 unit tests: percentile correctness, min/max clamping, empty-input, monotonicity (higher percentile = smaller set)

**Gate:** Note 06 ships flag-off; lint clean; unit tests green; smoke retrieve under flag-on shows sensible active set sizes (3–50) per Epic 0 distribution.

### Epic 2 — Note 06 A/B verification (gate alone vs baseline)

1. UVTS quick A/B: `SPARSE_RETRIEVAL_ENABLED=true` + Epic 0 percentile vs `false` (current production)
2. Sweep percentile ∈ {0.95, 0.98, 0.99} as a 3-preset sub-ablation (extend `phase13_1_ablation_runner.py`)
3. Acceptance: at least one percentile passes A/B (B mean ≥ A AND no per-question regression > 10%)
4. Observe: rerank prompt size reduction (instrumented via `mdemg_rerank_input_count` if exists, else add)
5. Full 120q on winning percentile

**Gate:** at least one percentile passes quick + full A/B; rerank prompt size measurably smaller (≥30% drop expected). On fail: stop at flag-off, scope Phase 14.1 with adaptive-per-query-type percentile.

### Epic 3 — Note 05 schema + catalog package

1. Neo4j V0028 — adds `context_fingerprint_active` (list of uint16) and `context_fingerprint_version` (int) properties on `MemoryNode` for `role_type='observation'` nodes. Index on `context_fingerprint_version` for version-mismatch fallback queries.
2. Neo4j V0029 — `ContextCatalog` node label; one per (space_id, version); contains `bits[]` array of `{position, kind, ref, token}` objects. Index on `(space_id, version)`.
3. New package `internal/hidden/context_catalog.go` (~250 LOC):
   - `Catalog` struct + `Builder` for constructing from graph stats
   - `SymbolBit(nodeID) (uint16, bool)`, `PathBit(path) (uint16, bool)`, `RoleBit(role) (uint16, bool)` lookups
   - `LoadActive(ctx, spaceID) (*Catalog, error)` — reads current version
   - `LoadVersion(ctx, spaceID, version) (*Catalog, error)` — for old-fingerprint queries
4. TSDB V0019 — `context_catalog_versions` hypertable for historical catalog snapshots (`catalog_id` CUIDv2, `recorded_at`, `space_id`, `version`, `bit_count`, `top_symbols TEXT[]` etc.)
5. Tier 1 unit tests on Builder + lookups

**Gate:** migrations apply forward + reverse on dev DB; catalog package + tests green; lint clean.

### Epic 4 — Note 05 fingerprint computation at observation time

1. New file `internal/conversation/fingerprint.go` (~150 LOC) — `computeContextFingerprint(ctx, obs, coActiveNodes)` per Note 05 §2.1
2. Wire into `Service.observe` (or wherever `MemoryNode` MERGE happens) — set `context_fingerprint_active` + `_version` on the observation node
3. Fallback: empty fingerprint when catalog cold (new space, no version yet) — graceful degradation
4. Catalog refresh job — weekly hook in `internal/ape/cycle.go` macro cycle: count top-N frequent features per space, MERGE new `ContextCatalog` version, retain previous versions for backward compat
5. New env knobs: `CONTEXT_FINGERPRINT_ENABLED` (default true), `CONTEXT_CATALOG_REFRESH_INTERVAL_HOURS` (168 = weekly), `CONTEXT_FINGERPRINT_BIT_BUDGET` (256)
6. Tier 1 + Tier 2 tests: deterministic fingerprint computation, idempotent catalog refresh

**Gate:** fingerprint + catalog refresh end-to-end on test space; observations created in test get non-empty fingerprints after first refresh.

### Epic 5 — Note 05 context-aware retrieval

1. **Decision fork (per §13)**: ContextColumn (5th RRF column) vs scoring-term-in-linear-scorer. Default: ContextColumn (Phase 13.1 default-on means RRF is the production path).
2. New file `internal/retrieval/column_context.go` (~120 LOC) implementing `Column` interface. Score = Jaccard similarity between observation fingerprint and query fingerprint.
3. Query-side fingerprint: request body adds optional `context_fingerprint` field; if absent, derive from session's recent co-activations via a helper.
4. Strict mode: `?strict_context=true` + `RETRIEVAL_CTX_STRICT_THRESHOLD` (default 0.25 Jaccard) — observations below threshold dropped pre-aggregation
5. Per-candidate explanation: matched-bits enumeration when `JiminyEnabled` or debug requested
6. New config: `RETRIEVAL_COLUMN_CONTEXT_ENABLED` (default true post-A/B), `RETRIEVAL_COLUMN_WEIGHT_CONTEXT` (default 0.10 — small initial weight; ablation can tune)
7. `Service.scorerVersion()` extended to include context column state (cache invalidation per Phase 13.1 pattern)
8. Tier 1 + Tier 2 tests on column behavior

**Gate:** context column live; cache namespace correctly flips; smoke retrieves with + without context fingerprint show different rankings on a polysemy fixture.

### Epic 6 — Note 05 historical backfill + opt-in rollout

1. New CLI subcommand `mdemg migrate context-fingerprint --space <id>` (`internal/cli/migrate_context_fingerprint.go`) — batched (1000 obs/batch), resumable (skips already-fingerprinted), progress-reported via stderr
2. Default behavior: NEW observations get fingerprints; historical ones don't until manually backfilled. `Service.observe` is the activation point.
3. Old observations without fingerprints match the "no context" fallback (empty intersection → low Jaccard → no boost; no penalty since context column weight is small)
4. Document the opt-in flow in `docs/features/context-fingerprinting.md`
5. Verify on a non-production space (`mdemg-dev`): backfill 100 observations, check fingerprint distribution

**Gate:** backfill CLI works on a test space; idempotent re-run produces no churn; doc complete.

### Epic 7 — Combined A/B verification + conditional default flips

1. UVTS A/B sweep (3 presets):
   - `gate_only` (Note 06 enabled, Note 05 disabled)
   - `fingerprint_only` (Note 05 enabled, Note 06 disabled)
   - `both` (Notes 05+06 enabled together)
2. Each compared against legacy baseline (Phase 13.1 production current state)
3. Quick profile (16q × 3 presets)
4. Winning preset(s) → full 120q profile
5. **Decision matrix**:
   - If `both` passes full A/B → flip both defaults
   - If only one passes → flip that one's default; ship the other flag-off
   - If neither passes → ship both flag-off, scope follow-up sprint

**Gate:** verdict captured for each preset; defaults flipped per matrix.

### Epic 8 — Documentation (Final Epic — Never Cut)

**Sprint-cycle docs:**
- `docs/development/post-ft-lora/sprint_plan_phase_14_sparse_fingerprints_and_gate.md` — frozen plan (this file)
- `docs/development/post-ft-lora/phase_14_post.md` — executed truth: A/B verdicts, default flips, OpenAI spend actual, fork outcomes
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` — Epic 0 forensic output

**Phase 14 feature docs (Notes 05+06 — primary sprint output):**
- `docs/features/context-fingerprinting.md` — Note 05 user/operator doc (Why / Choices / How it works / How to use)
- `docs/features/sparse-retrieval.md` — Note 06 user/operator doc (Why / Choices / How it works / How to use)

**Phase 11+ feature-doc backfill (operator request 2026-05-04 — every feature shipped since Phase 11 gets its own `docs/features/*.md` following `_TEMPLATE.md`, structured Why / Choices / How it works / How to use):**
- `docs/features/mlx-watchdog.md` (Phase 11.6.3) — endpoint-health watchdog state machine (`up → degraded → down`), `MLX_WATCHDOG_ENABLED` default-true, fast-fail gate at `llmclient/client.go:471`, `MDEMG_ALLOW_NO_MLX=1` escape hatch, `mdemg watchdog status [--json]` CLI, launchd plist, why MDEMG refuses to start without an LLM endpoint. Note that the name "mlx" is historical — the watchdog is now backend-agnostic post-Phase-13.5 (probes `/v1/models` against whichever endpoint `LLM_ENDPOINT` points at). Phase 13.6 backend-agnostic naming cleanup is queued separately; this doc captures current behavior + naming caveat
- `docs/features/uvts-validation.md` (Phase 12 + activation) — Universal Validation Test Specification semantic-quality framework, runner + grader + spec schema, `make test-uvts-{lint,quick,full}`, A/B harness (`uvts_ab_compare.py`) with Note 02 merge gate (B mean ≥ A AND no per-question regression > `ab_mode.regression_threshold_per_question`), V0016 `uvts_runs` + `uvts_results` schema, ConflictTracker production wiring
- `docs/features/column-voting-retrieval.md` (Phase 13 + 13.1) — 4-column RRF aggregator (Embedding / BM25 / Graph / Structural), `consensus_strength` per-call signal, `Service.scorerVersion()` cache namespacing per weight preset, `RETRIEVAL_COLUMN_VOTING_ENABLED` default-true since Phase 13.1, embedding-heavy preset `0.50/0.20/0.15/0.15` with the q-69 / hard_sym_4 forensic that justifies it, V0017 `retrieval_audit` schema, per-column suppression knobs, ablation runner (`scripts/phase13_1_ablation_runner.py`)
- `docs/features/local-llm-runtime.md` (Phase 13.5 + 13.5-telemetry) — llama.cpp llama-server cutover from `mlx_lm.server`, GGUF Q5_K_M production model at `mdemg-llm-v1.Q5_K_M.gguf`, why migrated (data-decided bake-off: 0 crashes / 160 min / 301 calls vs ~14-min crash cycle; latency p50 17s → 3.0s; UVTS at perfect parity 0.396 = 0.396), `llama-server --ctx-size 32768 --parallel 4 --cont-batching --jinja` config, `com.mdemg.llama-server.plist` launchd, V0018 `llm_endpoint_health_events` schema, MLX → GGUF conversion pipeline (dequantize → bf16 → f16 GGUF → Q5_K_M), rollback path
- **Update** `docs/features/service-resilience.md` to cover Phase 11.6.x additions: RSIC concurrency-limit semaphore, prompt-cache configuration, `JIMINY_ENABLED` defaults, ConflictTracker — these landed in Phase 11.6.x but the existing doc predates several knobs

**Cross-cutting doc updates:**
- `SPRINT_ROADMAP_POST_FT_LORA.md` — Phase 14 EXECUTED; queue Notes 07/08/09 status
- `AGENT_HANDOFF.md` top entry
- `CHANGELOG.md [Unreleased] ### Added/Changed`
- `CLAUDE.md` — new "Sparse Retrieval Gate (Phase 14)" + "Context Fingerprinting (Phase 14)" Architecture-Notes subsections

**Standing rule (effective Phase 14 onwards):** every future sprint that ships a user/operator-visible feature MUST add a `docs/features/<feature>.md` to its own Epic 8 (Documentation — Never Cut). This is captured as a memory feedback rule for future sprint plans.

**Gate:** all 7 feature docs landed (5 backfill + 2 Phase 14) + service-resilience update + sprint-cycle docs + cross-refs valid; conditional default flips applied.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit) — `go test -race`:**
- `gate_test.go` — percentile, clamping, empty input, monotonicity
- `column_context_test.go` — Jaccard correctness, version-mismatch fallback
- `context_catalog_test.go` — Builder determinism, top-N selection stable across re-runs
- `fingerprint_test.go` — compute determinism, empty-coactive fallback
- `config_phase14_test.go` — defaults, env overrides, range validation for all new knobs

**Tier 2 (Integration) — `go test -tags=integration`:**
- `tests/integration/sparse_gate_pipeline_test.go` — full retrieve with gate enabled, verify response shape + below_threshold debug field
- `tests/integration/context_fingerprint_e2e_test.go` — observe with co-active set, retrieve with context fingerprint, verify ranking shift
- `tests/integration/catalog_refresh_test.go` — schedule + verify new version created; old versions retained
- `tests/integration/v0019_v0020_migration_test.go` — TSDB forward + reverse + DO blocks

**Tier 3 (Live E2E) — MANDATORY:**
- **Epic 2 Note 06 A/B** — real mdemg + real grader, percentile sweep
- **Epic 7 combined A/B** — quick + full profile across 3 presets
- **Live polysemy demo** — seed 3 observations of `ErrorHandler` in distinct contexts, retrieve with each context; verify ranking shifts
- **Backfill smoke** — backfill on `mdemg-dev`, verify catalog versions in V0019
- **Long-running soak** (24 hr, low-volume): verify catalog refresh fires weekly; no migration deadlocks; Prometheus metrics flowing

**State restoration (MEMORY):** all changes additive or config-flagged. Rollback = `git revert <commit>` + revert migrations (V0028/V0029 reverse + V0019/V0020 drop). Feature flags give instant runtime disable.

**Gate:** all 3 tiers green; A/B verdicts captured; live polysemy demo shows expected ranking shift.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):
- Title (per-fork): `feat(retrieval): Sprint POST-FT-LORA-PHASE14 — Notes 05+06 sparse fingerprints + percentile activation gate (default-on/off per A/B verdict)`
- Body: Epic 0 distribution findings, A/B verdicts (per-preset), conditional default flip rationale, OpenAI spend actual, fork outcomes
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`
- Push → auto-PR → sprint summary comment posted

## 8. Verification Checklist

- [ ] Epic 0 — distribution analysis doc; defaults evidence-cited
- [ ] Epic 1 — Note 06 gate ships; Tier 1 + 2 green; smoke retrieve sensible
- [ ] Epic 2 — Note 06 A/B passes (or fails with documented gap)
- [ ] Epic 3 — V0028 + V0029 + V0019 + catalog package
- [ ] Epic 4 — fingerprint computation + weekly refresh job
- [ ] Epic 5 — context column wired (or scoring-term per fork); cache namespace flips
- [ ] Epic 6 — backfill CLI works; idempotent
- [ ] Epic 7 — combined A/B verdict matrix produces a clear default-flip decision
- [ ] Epic 8 — sprint plan + post + Epic 0 forensic doc + 7 feature docs (2 Phase 14 new: context-fingerprinting, sparse-retrieval; 4 Phase 11+ backfill: mlx-watchdog, uvts-validation, column-voting-retrieval, local-llm-runtime; 1 update: service-resilience) + ROADMAP + AGENT_HANDOFF + CHANGELOG + CLAUDE.md
- [ ] Commit pushed; auto-PR updated; sprint summary on PR
- [ ] All decision-fork outcomes disclosed
- [ ] `golangci-lint run ./...` clean
- [ ] CI green
- [ ] OpenAI spend logged (target $25–50)

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 8.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | Note 06 gate drops recall below acceptable | Medium | Epic 0 picks data-driven percentile; Epic 2 A/B is the gate; min/max clamps prevent empty active sets | Default flag-off; scope Phase 14.1 |
| 2 | Note 05 fingerprint adds noise (catalog poorly tuned) | Medium | Default Jaccard threshold low (0.25); ContextColumn weight low (0.10); strict mode opt-in | Set `RETRIEVAL_COLUMN_WEIGHT_CONTEXT=0` (off-via-weight) |
| 3 | Catalog refresh thrashes bit assignments | Low | Builder is deterministic given same input; bit positions reused across versions when symbols persist top-N | Pin catalog version in tests |
| 4 | Backfill on large space crashes mdemg or DB | Medium for spaces >1M observations | Batched (1000), resumable, progress-checkpointed, opt-in | Skip backfill; new obs only |
| 5 | V0028 ALTER on production-sized graph blocks | Low | Property addition only; Neo4j ALTER is fast | Test on dev space first |
| 6 | Cache pollution between Note 05/06 presets in A/B | Low | `Service.scorerVersion()` extended to include all knobs; verified Phase 13.1-style namespace flip | Manual cache flush between presets |
| 7 | OpenAI rate-limit during 3-preset × 2-profile A/B | Low | Total ~$25–50 spend; well within limits | Retry with backoff |
| 8 | Phase 13.1 baseline grades drift between sprints | Low | Use frozen baseline at `/tmp/phase13_1_full/baseline/grades.json` (preserved) OR re-capture if stale | Re-capture baseline as Epic 0 step |
| 9 | Combined Note 05+06 interaction unexpected | Medium | Epic 7 specifically tests combined preset against individual; flip both only if combined passes | Ship the one that passes individually; defer the combo to Phase 14.1 |
| 10 | ContextColumn weight competes with Phase 13.1 embedding-heavy preset | Medium | Default ContextColumn weight starts low (0.10); other 4 columns adjust proportionally; verify Phase 13.1's q 69 win still holds with context column active | Ablation runner sweeps Context weight ∈ {0.05, 0.10, 0.15, 0.20} |

## 11. Documents Accessed (during planning)

**Internal:**
- `docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md` — Note 05 design draft (380 lines)
- `docs/research/mdemg_sprint_ideas/06-sparse-retrieval-activation.md` — Note 06 design draft (287 lines)
- `docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` §3.3 / Phase 14 entry — strategic positioning
- `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md` — references to Phase 14 dependency
- `docs/development/post-ft-lora/phase_13_1_post.md` — Phase 13.1 default-on state, baseline grades reference
- `internal/retrieval/consensus.go` — Aggregator (Phase 14 gate operates after this)
- `internal/retrieval/scoring_rrf.go` — `Service.ScoreAndRankRRF` integration point
- `internal/retrieval/columns/*.go` — Column interface (Note 05's ContextColumn matches this)
- `internal/conversation/service.go` — observation pipeline (fingerprint computation hook)
- `internal/hidden/types.go` — observation struct (V0028 extension target)
- `internal/ape/cycle.go` — macro-cycle scheduler (catalog refresh hook)
- `internal/tsdb/migrations/017_retrieval_audit.sql` — primary input for Epic 0 distribution analysis
- `scripts/phase13_1_ablation_runner.py` — extensible to Phase 14 percentile sweep
- Memory: `feedback_sprint_plan_format.md`, `feedback_no_hardcoded_values.md`, `feedback_sequential_epics.md`, `feedback_plan_options_pattern.md`, `feedback_no_short_term_mlx_patches.md`, `feedback_data_decides_not_operator.md`, `feedback_live_testing_required.md`, `feedback_sprint_summary_on_pr.md`, `feedback_planning_before_code.md`

**External:** none required (Notes 05+06 are HTM-aligned but the implementation is internal — no external API research needed).

## 12. Rollback

All changes additive or feature-flagged.

1. `git revert <commit>` removes code, migrations, docs
2. **Runtime disable** (no rebuild): `SPARSE_RETRIEVAL_ENABLED=false` and/or `RETRIEVAL_COLUMN_WEIGHT_CONTEXT=0` in `.env` + `mdemg restart`. Cache becomes cold (different scorer_version namespace) — first 5 minutes uncached.
3. **Migration rollback** (if necessary): `mdemg db migrate --target V0027` reverts V0028+V0029 (drops `context_fingerprint_*` properties + `ContextCatalog` nodes); `mdemg tsdb migrate --target V0018` drops V0019 + V0020.
4. **Backfill data preserved across rollback** — fingerprints stay on observations; just unused with the column disabled. Re-enabling later doesn't require re-backfilling.

Phase 11 + 11.6.x + 12 + 11.6.3 + 13 + 13.1 + 13.5 artifacts untouched. Production model + llama-server config untouched.

---

## 13. Plan-Options (decision forks — pick at execution, disclose in PR)

Per MEMORY `feedback_plan_options_pattern.md`. Forks below are decided at Epic 0 close (data-cited where possible) or empirically by A/B (Epic 7).

| # | Fork | Recommendation (provisional) | Alternative(s) | Decision basis |
|---|---|---|---|---|
| 1 | **Note 05 integration: 5th RRF column vs scoring-term-in-linear** | **5th RRF column** (`ContextColumn`) | Scoring term in `internal/retrieval/scoring.go` (linear scorer fallback path) | Phase 13.1 made RRF default-on; legacy linear is fallback only; ContextColumn fits the architecture |
| 2 | **Note 06 percentile default** | **0.98** per spec, override per Epic 0 distribution data | 0.95 (more lax) or 0.99 (stricter) | Epic 0 forensic output |
| 3 | **Catalog refresh cadence** | **Weekly (168 hr)** per spec | Daily (24 hr) — too noisy; monthly — too stale | Spec default + bit-stability evidence |
| 4 | **Catalog bit budget** | **256 bits** per spec | 128 (cheaper but less expressive) or 512 (more expressive but doubles storage) | Spec default; Epic 0 data may say otherwise |
| 5 | **Bit assignment policy** | **64 symbols + 64 paths + 64 roles + 64 reserved** per spec | Adaptive split based on entropy of each kind | Spec default for v1; adaptive deferred |
| 6 | **Strict-mode default Jaccard threshold** | **0.25** per spec | Higher (0.4) for tighter context match; lower (0.1) for permissive | Spec default; Epic 0 fingerprint-size distribution informs |
| 7 | **ContextColumn default weight (if column path)** | **0.10** | 0.05 (gentler), 0.15 (stronger) | Empirical from Epic 7 A/B |
| 8 | **Sparse gate ordering: pre-rerank vs post-rerank** | **Pre-rerank** (gate first, rerank the survivors) | Post-rerank (rerank everything, gate after) | Pre-rerank captures the "reduce LLM context bloat" benefit; post-rerank wastes compute |
| 9 | **Default flip strategy** | **Per-Note** (flip Note 06 if Epic 2 passes; flip Note 05 if Epic 7 fingerprint_only passes; combined only if Epic 7 both passes) | Atomic (flip both or neither) | Per-Note matches Phase 11.6.3.1 / Phase 13.1 conditional-flip pattern |
| 10 | **Backfill rollout** | **Opt-in, manual CLI per space** | Auto-backfill on first refresh; never-backfill | Opt-in matches the existing operational hygiene (mdemg data export-auto pattern) |

If Epic 0 forensic data definitively contradicts a "spec default" pick (e.g. `whk-wms` average fingerprint size suggests p95 instead of p98), the data wins.

---

## Acceptance bar (top-level)

A successful Phase 14 ships when:
1. **Epic 0 forensic analysis** identifies data-driven percentile + Jaccard defaults
2. **Note 06 sparse gate** A/B passes 16q quick + 120q full at chosen percentile (mean ≥ baseline + no per-question regression > 10%)
3. **Note 05 context fingerprinting** A/B passes the same gate (per Note or combined per fork #9)
4. **Live polysemy demo** shows expected ranking shift on the test fixture
5. **Documentation complete** — 2 feature docs + post + roadmap + standard 4-doc footprint
6. **Default flips** applied per Epic 7 verdict matrix

A "failed" Phase 14 (no preset reaches A/B pass) ships value: distribution analysis + Notes 05/06 infrastructure flag-off + Phase 14.1 scoped. Mirrors Phase 13's first attempt that became Phase 13.1.

Anything beyond that outcome is Phase 14.1 / Phase 15.
