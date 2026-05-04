---
created: 2026-05-04
updated: 2026-05-04
status: phase 14 executed (narrow)
phase: POST-FT-LORA-PHASE14
predecessor: phase 13.1 (commit 6ed411e)
successors: phase 14.1 (Note 06 adaptive per-category), phase 14.2 (Note 05 adaptive catalog)
---

# Phase 14 — Executed Truth (Narrow Close)

> Narrow close per operator approval 2026-05-04 after Epic 0+1+2 produced design questions for both Notes that warranted dedicated follow-up sprints. This doc captures what shipped, what was deferred, and why.

## TL;DR

| Sprint goal (original) | Outcome | Reason |
|---|---|---|
| Note 05 — context fingerprints | **Deferred to Phase 14.2** | Epic 0 forensic found `whk-wms` has 0 symbols + 0 roles → spec's static 64/64/64/64 catalog bit split is wrong for non-code spaces; needs adaptive Builder redesign. Two open redesign questions (with Note 06's per-category) compounded sprint risk. |
| Note 06 — percentile activation gate | **Code shipped flag-off; defaults deferred to Phase 14.1** | Epic 2 16q quick passed at MIN=10/p95 (mean +0.019, 0 regressions). 120q full produced mean parity (0.413=0.413) with 7 boundary regressions concentrated in `architecture_structure` (3 of 7) — gate cuts citations needed for queries whose right answer lives at rank 11–20. Adaptive per-category percentile is the right fix. |
| Phase 13 Epic 6 V0017 audit writer | **Bug fix shipped** | Discovered during Epic 0: V0017 was empty since Phase 13 because `SetRetrievalAuditWriter` had no caller. Fixed in-flight (`tsdb/retrieval_audit_writer.go` + adapter wired in `api/server.go`). V0017 now collects forward. |
| Phase 11+ feature doc backfill | **5 docs written + 1 update** | Per operator request 2026-05-04. Each follows `_TEMPLATE.md`: Why / Choices / How it works / How to use. |

## What shipped

### 1. Phase 13 Epic 6 audit-writer fix

| Path | Notes |
|---|---|
| `internal/tsdb/retrieval_audit_writer.go` (new, ~165 LOC) | Buffered writer, 30s flush via CopyFrom; mirrors `LLMEndpointHealthWriter` (V0018) pattern |
| `internal/api/server.go` | New `retrievalAuditAdapter` translates `retrieval.RetrievalAuditRecord` → `tsdb.RetrievalAuditRow` (cycle-safe). Wired conditionally on `cfg.RetrievalAuditEnabled` |
| `.env` | `RETRIEVAL_AUDIT_ENABLED=true` flipped |

Live verification: 5 retrieves → 3 audit rows landed via flush (cache-hit retrieves skip audit, observation noted for Phase 14.1).

### 2. Note 06 sparse activation gate (flag-off)

| Path | Notes |
|---|---|
| `internal/retrieval/gate.go` (new, ~190 LOC) | `ApplySparseGate` with R-7 percentile + MIN/MAX clamps + per-call metadata |
| `internal/retrieval/gate_test.go` (new, ~210 LOC) | 9 unit tests: disabled passthrough, empty input, percentile cutoff, ceiling, floor, monotonicity, edge-cases, realistic heavy-tail, percentile-linear known values — all green |
| `internal/retrieval/service.go` | Gate fires post-aggregation, pre-rerank; emits Prometheus + V0019 row when on |
| `internal/api/handlers.go` | `?sparse=true|false` + `?sparse_percentile=N` per-request overrides |
| `internal/models/models.go` | `SparseEnabled` / `SparsePercentile` request fields |
| `internal/config/config.go` | 4 new knobs (`SPARSE_RETRIEVAL_ENABLED`, `SPARSE_ACTIVATION_PERCENTILE`, `SPARSE_MIN_ACTIVE`, `SPARSE_MAX_ACTIVE`); `TSDB_REQUIRED_SCHEMA_VERSION` 18→19 |
| `internal/metrics/collectors.go` | 3 new histograms (`sparse_gate_active_count`, `_dropped_fraction`, `_threshold`) |

**Defaults shipped** (gate flag-off; if operator opts in, these are the conservative starting points):
- `SPARSE_RETRIEVAL_ENABLED=false` (operator opt-in)
- `SPARSE_ACTIVATION_PERCENTILE=0.95`
- `SPARSE_MIN_ACTIVE=3`
- `SPARSE_MAX_ACTIVE=20`

Note: Epic 2 demonstrated MIN=3 produces boundary regressions on `whk-wms`. Operators who enable the gate should set `SPARSE_MIN_ACTIVE=10` based on the 16q quick pass; expect ~50% rerank input reduction with mean parity. Phase 14.1 is queued to ship adaptive per-category MIN_ACTIVE so the default-on flip can happen.

### 3. V0019 sparse_gate_metrics hypertable

| Path | Notes |
|---|---|
| `internal/tsdb/migrations/019_sparse_gate_metrics.sql` (new, 90 LOC) | Schema: `metric_id` (CUIDv2) + `recorded_at` + `space_id` + `percentile_applied` + `threshold_score` + `input_count` + `active_count` + `dropped_count` + `floor_applied` + `ceiling_applied` + `scorer_version`. Hypertable, 7-day chunks, indexes on space+time, percentile+time, clamp-fired filter |
| `internal/tsdb/sparse_gate_writer.go` (new, ~165 LOC) | Buffered writer; mirrors V0017 pattern |
| `internal/api/server.go` | `sparseGateRecorderAdapter` always wired (so per-request `?sparse=true` overrides record even when default off) |

Live verification: 8 retrieves with `?sparse=true` → 8 V0019 rows; mean active=3, dropped=17, floor_applied=8/8, ceiling_applied=0/8 (matches Epic 0 forensic prediction at MIN=3).

### 4. Documentation backfill

Per operator standing rule (saved as `feedback_per_feature_docs_required.md`): every sprint that ships a user/operator-visible feature must add a `docs/features/<feature>.md`. Backfill written this sprint for Phase 11+ features that lacked feature docs:

| Feature doc | Phase | Coverage |
|---|---|---|
| `docs/features/mlx-watchdog.md` (new) | 11.6.3 | Watchdog state machine, fast-fail gate, escape hatch, plist, naming caveat (backend-agnostic post-13.5) |
| `docs/features/uvts-validation.md` (new) | 12 | UVTS framework, runner+grader, A/B harness, V0016 schema, ConflictTracker |
| `docs/features/column-voting-retrieval.md` (new) | 13 + 13.1 | RRF aggregator, `consensus_strength`, scorer-version cache, embedding-heavy preset, V0017 audit, ablation runner, q69 forensic |
| `docs/features/local-llm-runtime.md` (new) | 13.5 + 13.5-telemetry | llama.cpp cutover, GGUF Q5_K_M, plist, V0018 health events, MLX→GGUF pipeline, rollback |
| `docs/features/sparse-retrieval.md` (new) | 14 | Note 06 gate (this sprint, flag-off), config knobs, per-request override, V0019 metrics, Phase 14.1 retune path |
| `docs/features/service-resilience.md` (extended) | 11.6.x | RSIC concurrency-limit semaphore, prompt-cache config, ConflictTracker addendum |

## What was deferred (with rationale)

### Phase 14.1 — Note 06 adaptive per-category percentile

**Why deferred**: Epic 2 120q full found `architecture_structure` is the dominant regression source (3 of 7 boundary regressions; -0.015 net category mean while every other category is ≥0). The static MIN/MAX/percentile cannot satisfy both code-shape queries (need rank 11–20 citations) and concept-shape queries (rank 1–10 sufficient).

**Phase 14.1 scope** (~3 dev-days):
1. Per-category percentile + clamp via `SPARSE_GATE_CATEGORY_OVERRIDES` config (JSON map: `{"architecture_structure": {"min_active": 20}, ...}`)
2. Re-run 120q A/B with adaptive defaults
3. If passes, flip default-on with category-aware config

### Phase 14.2 — Note 05 with adaptive catalog bit allocation

**Why deferred**: Epic 0 forensic found `whk-wms` MemoryNodes have:
- 0 distinct `symbol` properties (the field doesn't exist on whk-wms — it's a conversation-history space, not code)
- 0 distinct `role` properties (not populated)
- 8360 distinct `path` properties (rich)
- 5 `role_type` values + 5 `layer` values (categorical)

Spec's static 64-bit/64-bit/64-bit/64-bit allocation across (symbols, paths, roles, reserved) wastes 128 bits on this space. The right design is an adaptive Builder that counts feature density at refresh and allocates bits proportionally with floors. Shipping Note 05 in this sprint with the static spec would either miss the polysemy lift it's designed to produce, or require post-merge rework.

**Phase 14.2 scope** (~7 dev-days):
1. Adaptive catalog Builder (per-space density measurement → bit allocation with `role_type+layer` floor)
2. Neo4j V0028 (MemoryNode fingerprint properties) + V0029 (ContextCatalog node label)
3. TSDB V0020 (catalog_versions hypertable)
4. Fingerprint computation at observation time
5. Context column for retrieval (5th RRF column or scoring term — fork at execution per data)
6. Backfill CLI
7. Combined A/B with Note 06 gate (Phase 14.2 needs Phase 14.1 in flight first so the gate doesn't pollute the signal)

## Epic-by-epic outcomes

| Epic | Plan | Outcome |
|---|---|---|
| 0 — Preflight + forensic distribution analysis | Query V0017, compute percentile defaults, audit Neo4j catalog conflicts | **Done.** Output: `phase_14_score_distribution_analysis.md`. Found V0017 empty (Phase 13 Epic 6 gap → fixed in-flight); derived defaults from `llm_interactions.retrieval_scores` (99k+50k score points across `consulting.classify` + `retrieval.rerank_cross`); flagged catalog bit-allocation redesign needed for `whk-wms` |
| 1 — Note 06 sparse activation gate | Code + 4 knobs + per-query override + 3 metrics + V0019 + Tier 1 tests | **Done.** Live-verified with smoke + V0019 row landing |
| 2 — Note 06 A/B verification | UVTS quick + percentile sweep, full 120q on winner | **Done — verdict captured.** 16q quick at MIN=10/p95 PASSED; 120q full FAILED per-question gate (mean parity, 7 boundary regressions). Ship flag-off per §10 risk #1 contingency |
| 3 — Note 05 schema + catalog package | V0028+V0029+V0019 + catalog package | **Deferred to Phase 14.2** (catalog redesign needed) |
| 4 — Note 05 fingerprint at observation time | Wire into Service.observe + catalog refresh | Deferred |
| 5 — Note 05 context-aware retrieval | 5th RRF column or scoring term | Deferred |
| 6 — Note 05 historical backfill | CLI subcommand | Deferred |
| 7 — Combined A/B verification | 3-preset sweep, default flip matrix | Partial — Note 06 alone done; combined requires Phase 14.2 |
| 8 — Documentation | Sprint plan + post + 7 feature docs (5 backfill + 2 Phase 14) | **Done — narrowed.** Sparse-retrieval doc shipped (Note 06); context-fingerprinting doc deferred to Phase 14.2 |

## A/B verdict tables (full evidence)

### 16q quick sweep

| Preset | mean | mean_delta | regressions ≥0.10 | improvements | verdict |
|---|---|---|---|---|---|
| baseline-sparse-off | 0.4021 | — | — | — | A |
| sparse-p95 (MIN=3) | 0.3958 | -0.006 | 2 (q69, hard_sym_19) | 1 | fail |
| sparse-p98 (MIN=3) | 0.3958 | -0.006 | 2 (q263, q69) | 1 | fail |
| sparse-p99 (MIN=3) | 0.4021 | 0.000 | 1 (q69) | 1 | fail |
| **sparse-p95 + MIN=10** | **0.4210** | **+0.019** | **0** | **3** | **PASS** |

### 120q full sweep

| Preset | mean | mean_delta | regressions ≥0.10 | improvements | verdict |
|---|---|---|---|---|---|
| baseline-sparse-off (Phase 13.1 prod) | 0.4128 | — | — | — | A |
| **sparse-p95 + MIN=10** | **0.4128** | **0.000** | **7** | **7** | **fail per-question** |

### Per-category 120q (the diagnostic)

| Category | n | Baseline | Candidate | Δ | Regressions | Pattern |
|---|---|---|---|---|---|---|
| architecture_structure | 20 | 0.441 | 0.426 | **−0.015** | 3 | Concrete struct/file lookups → right citation often at rank 11–20 |
| business_logic_constraints | 20 | 0.387 | 0.397 | +0.010 | 1 | Net win |
| relationship | 6 | 0.417 | 0.433 | +0.017 | 0 | Net win |
| data_flow_integration | 20 | 0.397 | 0.392 | −0.005 | 2 | Mostly cancels |
| cross_cutting_concerns | 20 | 0.412 | 0.412 | 0.000 | 1 | Cancels |
| computed_value | 6 | 0.367 | 0.367 | 0.000 | 0 | Unchanged |
| disambiguation | 8 | 0.425 | 0.425 | 0.000 | 0 | Unchanged |
| service_relationships | 20 | 0.436 | 0.441 | +0.005 | 0 | Unchanged |
| **weighted total** | **120** | **0.4128** | **0.4128** | **0.000** | **7** | — |

**Insight**: 7 boundary regressions ≈ 7 boundary improvements ≈ floating-point noise on a tight score distribution. `architecture_structure` is the only category with material decline; it's also the category with the deepest right-citation rank (struct + file lookups frequently sit at rank 11–20 in the candidate list). Phase 14.1 should target this category-specifically.

## OpenAI spend (actual)

| Run | Cost estimate |
|---|---|
| 16q quick × 4 presets (initial sweep) | ~$2.40 |
| 16q quick × 1 preset (MIN=10 diagnostic) | ~$0.60 |
| 120q full × 1 preset (MIN=10 verification) | ~$10 |
| **Sprint total** | **~$13** |

Well under $100 cap; under $25-50 sprint budget.

## Decision-fork outcomes (sprint plan §13)

| Fork | Provisional | Outcome |
|---|---|---|
| #2 percentile default | 0.98 | Epic 0 → 0.95 in code defaults; Epic 2 confirmed clamp dominates choice |
| #3 catalog refresh cadence | weekly | Deferred to Phase 14.2 |
| #4 catalog bit budget | 256 | Deferred to Phase 14.2 |
| #5 bit assignment policy | static 64/64/64/64 | Epic 0 → adaptive (deferred to Phase 14.2 with redesign) |
| #6 Jaccard threshold | 0.25 | Deferred to Phase 14.2 |
| #8 sparse gate ordering | pre-rerank | Confirmed (Epic 1 wired pre-rerank) |
| #9 default flip strategy | per-Note conditional | Note 06 ships flag-off; Phase 14.1 will retune and flip |

## Memory observations recorded

- `rw0mzergwcqct8abpw0dli9x` — Phase 14 Epic 8 expanded to include feature-doc backfill (operator request 2026-05-04)
- `sc4iwy3of9ndn5kowja1i14i` — Epic 0 forensic complete; Phase 13 Epic 6 wiring gap closed
- `omr2rs5jppqrvee2k0l1xtd1` — Epic 1 complete with Tier 1 tests + lint clean
- `re4k7rpd3hjt5a52l8qwx8fp` — Epic 2 verdict; ship flag-off per §10 risk #1; Phase 14.1 scoped

## Documents accessed (during execution)

- `docs/development/post-ft-lora/sprint_plan_phase_14_*.md` (frozen plan)
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` (Epic 0 output)
- `docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md`
- `docs/research/mdemg_sprint_ideas/06-sparse-retrieval-activation.md`
- `internal/retrieval/service.go`, `scoring.go`, `scoring_rrf.go`, `retrieval_audit.go`, `gate.go`, `gate_test.go`
- `internal/api/server.go`, `handlers.go`
- `internal/tsdb/retrieval_audit_writer.go`, `sparse_gate_writer.go`, `migrations/019_sparse_gate_metrics.sql`
- `internal/config/config.go`
- `internal/metrics/collectors.go`
- `internal/models/models.go`
- TSDB: `retrieval_audit`, `uvts_runs`, `uvts_results`, `llm_interactions`, `sparse_gate_metrics`
- Neo4j: `whk-wms` MemoryNode property surface
- A/B verdicts: `/tmp/phase14_epic2/`, `/tmp/phase14_epic2_full/sparse-p95-min10/verdict.json`
