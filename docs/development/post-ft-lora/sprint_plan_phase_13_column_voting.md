# Sprint POST-FT-LORA-PHASE13 — Note 04 Column-Voting Retrieval

## Context

Phase 12 (UVTS Activation, commit `98fc7a8`) shipped the A/B harness and runner; Phase 11.6.3 (MLX Watchdog, commit `fb78394`) eliminated the retry-storm pattern that blocked sustained live A/B testing. With both prerequisites in place, Phase 13 is the first research-extension sprint that actually exercises the harness — and the first sprint whose merge gate is enforced via a UVTS A/B verdict.

**What it does.** Replace the linear-combination ranker at `internal/retrieval/scoring.go:797` with a **6-column Reciprocal Rank Fusion (RRF) ensemble**. The codebase already has a 2-column RRF (vector + BM25 at `hybrid.go:197-284`) and a heavily-tuned linear scorer with ~12 signal components. Phase 13 lifts the RRF pattern up the stack, makes it the canonical aggregator, and emits a new per-query `consensus_strength` uncertainty signal.

**Why now.** Per the post-FT-LORA roadmap §3.3, Note 04 is the lowest-risk + highest-coverage Tier 2 research extension — it produces a `consensus_strength` signal that downstream extensions (Notes 05, 06) consume. The 412-line draft at `docs/research/mdemg_sprint_ideas/04-column-voting-retrieval.md` has been sitting in the repo waiting for UVTS + watchdog to land. Both did. Phase 13 is the obvious next sprint and is parallel-safe with FT-LORA follow-ups (no PC/FEP dependencies).

**Why a feature flag.** The current ranker is heavily-tuned production code. A flag-default-off rollout (matching the Phase 11.6.3 pattern) gives operators an A/B cycle of soak time before flipping the default. A regression on retrieval quality has direct, high-blast-radius user impact.

**Phase dependency chain.** Phase 11.6.x → 11.6.2 → 12 → 11.6.3 → **Phase 13 (this)** → Phase 14 (Notes 05+06 consume `consensus_strength`).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | POST-FT-LORA-PHASE13 |
| Title | Note 04 — Column-Voting Retrieval (RRF over 6 columns + `consensus_strength`) |
| Date | 2026-05-01 (plan) |
| Branch | `reh3376_dev01` |
| Predecessor | Phase 11.6.3 (commit `fb78394`, this branch's HEAD) |
| Successor | Phase 14 — Notes 05+06 (Sparse fingerprints + percentile activation gate) |
| Type | Code-large (~950 LOC production + ~800 LOC tests across 6 new column files + new consensus aggregator + cache + scorer fork + config + downstream consumers); A/B-validated; one TSDB migration optional |
| Risk | HIGH (retrieval quality is user-facing and high-blast-radius; mitigated by feature flag default-off + UVTS A/B merge gate) |
| Budget | $5–25 OpenAI (UVTS grader on full 120-question profile × A/B cycles); ~6 hr local compute (mlx + UVTS runs + soak) |
| Effort estimate | 12 dev-days (~3 weeks) |
| New TSDB migration | Optional V0017 — only if `consensus_strength` is persisted on retrieval responses for historical analysis. Recommend: V0017 added (additive, low-risk) so Phase 14 has historical baseline rows. |
| Post-sprint artifacts | `internal/retrieval/columns/` (new package, 6 files); `internal/retrieval/consensus.go` (new); `internal/retrieval/scoring.go` (feature-flag fork); `internal/retrieval/cache.go` (scorer-version hash); `internal/retrieval/rerank.go` (optional `consensus_strength` consumption); `internal/ape/self_assess.go` (optional DH-005 confidence input); `internal/config/config.go` (~10 new knobs); migrations/017_consensus_strength_column.sql (optional); docs |

## 2. Problem Statement

The current MDEMG ranker is a single linear-combination formula at `scoring.go:797`:

```
s = vecComponent + bm25Component + actComponent + recComponent + confComponent
    + pathBoost + comparisonBoost + l1BoostEffect + bypassBonus
    - hubPenalty - redundancyPenalty - stalePenalty
```

Two structural issues this sprint addresses:

1. **Weight tuning is fragile.** 12 signal components × per-layer decay × query-gating produce a high-dimensional weight space. Any change in one signal can subtly degrade another. A/B validation is the only honest way to know.

2. **No uncertainty output.** The current ranker returns a final score per candidate. It does not signal *how much agreement* the underlying signals had. Downstream extensions (Note 05 sparse activation, Note 06 percentile gating) need an uncertainty signal to gate decisions on.

The Phase 13 architecture decomposes ranking into 6 independent **columns**, each producing its own ranked list. Reciprocal Rank Fusion (RRF, k=60) aggregates them with per-column weights. The aggregator additionally emits `consensus_strength = (columns_containing_node / total_columns_queried) × avg(normalized_rank)` — a coarse uncertainty signal (step size 0.17–0.33 across 3–6 columns) that consumes neatly into the rerank stage and the DH-005 retrieval-confidence dimension.

The 6 columns:

| Column | Status | Signal | Source |
|---|---|---|---|
| **Embedding** | refactor | Semantic similarity via learned vectors | Existing Neo4j vector index (`db.index.vector.queryNodes`) |
| **BM25** | refactor | Lexical/keyword relevance | Existing Neo4j fulltext index |
| **GraphProximity** | refactor | Topological relatedness | Existing spreading activation (`activation.go`) |
| **Structural** | NEW | Same-file/package/symbol-tree | Cypher: `contains`, `defined_in` edges, configurable hop depth (default 2) |
| **Temporal** | NEW | Recency bias | `last_activated_at`, `updated_at` fields with exponential decay (half-life ~1 day, configurable) |
| **RoleScoped** | NEW | Same-role/developer/task | Role/source metadata on observations; restricts candidate pool then ranks by embedding within scope |

Three orthogonal concerns guarded by the design:

- **Backward compat.** The existing linear scorer remains accessible via `RETRIEVAL_COLUMN_VOTING_ENABLED=false` (default false initially). No data is rewritten.
- **Cache correctness.** The cache key (`cache.go:56-83`) currently does NOT include scorer-version. Phase 13 adds a `scorer_version` hash to cache keys so weight changes invalidate stale entries. Same fix protects future ranker work.
- **Reranker isolation.** The LLM-bound reranker at `rerank.go:41-212` operates *post-scorer* and is orthogonal to this work. It optionally gains a `consensus_strength` input feature but its blend math is unchanged.

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | New `internal/retrieval/columns/` package | 6 column files: `embedding.go`, `bm25.go`, `graph.go`, `structural.go`, `temporal.go`, `role.go` (~600 LOC total — 3 refactor + 3 new) |
| 2 | RRF consensus aggregator + `consensus_strength` computation | `internal/retrieval/consensus.go` (new, ~150 LOC) |
| 3 | Scorer fork — feature-flag gate around current linear vs new RRF path | `internal/retrieval/scoring.go` (~50 LOC delta + new `ScoreAndRankRRF` function) |
| 4 | Cache key includes `scorer_version` hash | `internal/retrieval/cache.go` (~20 LOC delta) |
| 5 | Optional `consensus_strength` consumption in rerank | `internal/retrieval/rerank.go` (~30 LOC delta) |
| 6 | Optional `consensus_strength` input to DH-005 retrieval confidence | `internal/ape/self_assess.go` (~20 LOC delta) |
| 7 | Config knobs (~10 new) | `internal/config/config.go` (~80 LOC) |
| 8 | TSDB V0017 migration (optional, recommended) | `internal/tsdb/migrations/017_consensus_strength.sql` adds `consensus_strength NUMERIC` column to `llm_interactions` retrieval rows, OR a new `retrieval_audit` hypertable |
| 9 | Per-column unit tests + RRF property tests + golden-output regression tests (none exist today) | `internal/retrieval/columns/*_test.go`, `internal/retrieval/consensus_test.go`, `internal/retrieval/scoring_golden_test.go` |
| 10 | UVTS A/B validation — quick + full profile | Two distinct branch_label runs through `make test-uvts-quick`, then `uvts_ab_compare.py`; merge gate verdict captured in `uvts_runs` |
| 11 | Sprint docs | `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md` (frozen) + `phase_13_column_voting_post.md` (post) |
| 12 | Doc updates | `AGENT_HANDOFF.md`, `CHANGELOG.md`, `CLAUDE.md` (new "Column-Voting Retrieval" subsection under Architecture Notes), `SPRINT_ROADMAP_POST_FT_LORA.md` (mark EXECUTED + Phase 14 unblocked) |

**Out of scope (deferred):**
- Per-column weight ablation study. v1 ships with equal weights (`1.0/N` per active column); ablation is a Phase 13.1 follow-up.
- Phase 14 sparse fingerprints (Note 05) and percentile activation gating (Note 06) — they consume `consensus_strength` but ship in their own sprint.
- Reranker math changes. Reranker may *consume* `consensus_strength` as a feature but its blend formula is unchanged.
- Linear scorer removal. Stays as fallback indefinitely (gated by `RETRIEVAL_COLUMN_VOTING_ENABLED`); deletion deferred until Phase 13 has soaked ≥1 release cycle without regression.
- Cross-space A/B (different `space_id` between baseline and candidate). UVTS spec already supports this but Phase 13 measures within-space ranking change only.
- Promotion of A/B gate from advisory to required-blocking in CI. Stays advisory through Phase 13; Phase 14 promotes after one clean A/B cycle.

**Constraints (hard):**
- **MEMORY: no hardcoded values** — column weights, k constant (default 60), structural hop depth, temporal half-life, per-column timeout fraction all in config.
- **MEMORY: sequential epics** — no parallel epic execution; docs before implementation within each epic.
- **MEMORY: 3-tier testing — Tier 3 MUST be live** (formalized in CLAUDE.md commit `d10c1a5`). UVTS A/B against real mdemg + real TSDB is the canonical Tier 3.
- **MEMORY: plan-options pattern** — 5 decision forks documented in §13 below; recommendations baked into the plan; disclose at PR.
- **MEMORY: single batched commit at sprint close**.
- **MEMORY: sprint summary posted to PR comments immediately after push**.
- **MEMORY: CUIDv2 for any new IDs** (e.g. retrieval_audit row IDs if V0017 ships the audit table variant).
- **MEMORY: max_tokens ≥ 3000, latency_budget_ms ≥ 15000** — no LLM call sites added in this sprint, but if any reranker prompt template is touched, observe the floor.
- **Backward compat** — `RETRIEVAL_COLUMN_VOTING_ENABLED=false` default until A/B verdict passes. After pass, flipped to `true` in the same sprint commit (single config-default change is part of Epic 8 — same pattern as Phase 11.6.3 deferred-then-flipped).
- **Cache correctness** — scorer_version hash must be in the cache key BEFORE the new aggregator goes live; otherwise A/B results can be polluted by stale cache hits.
- **Quality bar (UVTS A/B merge gate)**: B mean ≥ A mean AND no per-question regression > 10% (matches `lnl_demo_validation.uvts.json::ab_mode.regression_threshold_per_question`). The Note 04 doc says ">15%"; resolve in favor of the spec (10% — stricter).
- **Latency bar**: p50 within 1.2× baseline; p95 within 1.5× baseline. Parallel column execution via `errgroup` makes this achievable since columns are independent.

## 4. Dependencies

**Consumed (code, pre-existing — reuse, do not duplicate):**
- `internal/retrieval/scoring.go:647-887` — current `ScoreAndRankWithBreakdown` linear formula. Stays as fallback path.
- `internal/retrieval/hybrid.go:197-284` — existing 2-column RRF (`ReciprocalRankFusion` + `ConvertFusedToCandidates`). Generalize to N columns (the new `consensus.go` lifts and extends this).
- `internal/retrieval/service.go:857-950` — `vectorRecall` (Embedding column input).
- `internal/retrieval/hybrid.go:35-144` — `BM25Search` (BM25 column input).
- `internal/retrieval/activation.go:14-134` (`SpreadingActivation`) and `activation.go:369-489` (`SpreadingActivationWithAttention`) — GraphProximity column input.
- `internal/retrieval/cache.go:56-83` — `CacheKey` function. Add `scorer_version` parameter.
- `internal/retrieval/rerank.go:41-212` — Rerank stage. Optional `consensus_strength` feature consumer.
- `internal/ape/self_assess.go` — DH-005 dimension confidence. Optional `consensus_strength` input.
- `internal/config/config.go::FromEnv` — env-loading pattern. Mirror `MLX_*` knob style from Phase 11.6.3.
- `docs/tests/uvts/runners/uvts_runner.py` + `uvts_ab_compare.py` — Phase 12 A/B harness. Operator-facing recipe.
- `docs/tests/uvts/specs/lnl_demo_validation.uvts.json` — `ab_mode.regression_threshold_per_question=0.10` is the canonical bar.
- `internal/conversation/conflict_tracker.go` — Phase 12 ConflictTracker. Phase 13's `consensus_strength` may surface as input to a new conflict heuristic in a follow-up sprint.

**Consumed (data):**
- Neo4j vector index, fulltext index, graph (CO_ACTIVATED_WITH, CODE_REL edges) — for the 3 existing columns.
- `last_activated_at`, `updated_at` properties on MemoryNode — Temporal column input. Verify these are populated in production data via `MATCH (n:MemoryNode) WHERE n.last_activated_at IS NULL RETURN count(n)` at Epic 0; if material gaps, ship Epic 0.5 backfill.
- Role/source metadata on observations — RoleScoped column input. Same Epic 0 verification.
- TSDB schema_version=16 (post-Phase 12) at preflight; bumps to 17 IF V0017 ships.

**Consumed (compute):**
- mdemg HTTP API at `localhost:9999` (post-Phase 11.6.3 with watchdog enabled if operator has flipped the default).
- Local mlx_lm.server at `127.0.0.1:8101` — for any reranker test paths during A/B; not strictly required if reranker is disabled during the A/B cycle.
- OpenAI API for UVTS grader (`gpt-5.4-mini` per spec default). Cost: 16-question quick × 4 runs (baseline×candidate × 2 cycles) ≈ $5; full 120-question A/B ≈ $20. Total estimate: $5–25.
- TimescaleDB at `localhost:5433` for `uvts_runs` + `uvts_results` writes.

**External services:**
- mdemg HTTP API for retrieve calls.
- TimescaleDB for V0016/V0017.
- OpenAI / Claude for UVTS grader.

No Neo4j writes from this sprint. No model training. No new launchd plists.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` clean (post-Phase-11.6.3); native binary running with `LLM_ENDPOINT=http://127.0.0.1:8101/v1` (or watchdog enabled); TSDB schema_version=16; mdemg health green; Neo4j up; Python venv ready for UVTS runs.

### Epic 0 — Preflight + data audit + branch hygiene

1. Run `MATCH (n:MemoryNode) WHERE n.last_activated_at IS NULL RETURN count(n)`, same for `updated_at`, role metadata. If >5% null, ship a backfill in Epic 0.5 before Temporal/RoleScoped columns light up.
2. Capture baseline UVTS quick-profile run on current ranker — `make test-uvts-quick UVTS_BASE_URL=http://localhost:9999` with `--persist-tsdb --branch-label baseline-pre-phase13 --codebase-sha $(git rev-parse HEAD)`. Save grades.json. This is the **A baseline** that all A/B comparisons measure against.
3. Inventory current ranker call sites: grep for `ScoreAndRank` / `ScoreAndRankWithBreakdown` to enumerate everywhere the scorer is invoked. Confirm only `service.Retrieve` (one site) so the feature-flag fork has minimal surface.
4. Read existing 2-column RRF (`hybrid.go:197-284`) end-to-end — the new `consensus.go` is a generalization of this; understand its quirks before lifting.
5. Confirm `cache.go::CacheKey` is the only cache-key generator (no shadow keys elsewhere). Audit cache_test.go for golden-key tests that would break when we add `scorer_version`.

**Gate:** Epic 0 baseline grades.json saved + uvts_runs row exists; null-data audit done; ranker call site inventory complete (single site = minimal blast radius); RRF lifted-pattern understood.

### Epic 1 — `internal/retrieval/columns/` package (3 refactor + scaffolding)

1. New package directory `internal/retrieval/columns/` with shared types: `Column` interface (`Run(ctx, query) ([]Result, error)`), `Result` struct (`{NodeID, Score, Rank, Latency}`).
2. Refactor existing logic into 3 column files (move, do not duplicate):
   - `columns/embedding.go` — wraps `service.vectorRecall`
   - `columns/bm25.go` — wraps `hybrid.BM25Search`
   - `columns/graph.go` — wraps `activation.SpreadingActivation` (with the activation-squared / floor knobs intact)
3. Each column implements `Column` interface; error returns are non-fatal (column treated as `rank=∞` for missing results in the aggregator).
4. Per-column timeout via ctx — config knob `RETRIEVAL_COLUMN_TIMEOUT_FRACTION` (default 0.8 of the total retrieve budget, matching Note 04 design).
5. Tier-1 unit tests per column (golden-output style — fixed seeds, fixed graph fixture, fixed expected ranks).
6. **No** behavior change for callers yet — old `service.Retrieve` still calls the legacy linear scorer.

**Gate:** `go test -race ./internal/retrieval/columns/...` green; 3 columns produce same ranks as legacy paths on a fixture (proof of refactor correctness).

### Epic 2 — 3 new column implementations

1. `columns/structural.go` — Cypher query: `MATCH (q)-[:contains|defined_in*1..N]-(n) RETURN n` where N = `RETRIEVAL_STRUCTURAL_HOPS` (default 2). Score = exponential decay on hop distance.
2. `columns/temporal.go` — exponential decay on `now() - last_activated_at` with half-life `RETRIEVAL_TEMPORAL_HALFLIFE_HOURS` (default 24). Pulls candidate set via vector index then re-ranks by recency.
3. `columns/role.go` — RoleScoped: filter candidate pool by `role` / `source` properties to scope (config: `RETRIEVAL_ROLE_SCOPE_FIELD` default `role`), then rank by embedding similarity within scope.
4. Each new column has Tier-1 unit tests against fixture data.
5. Each new column has its own config knob to disable: `RETRIEVAL_COLUMN_<NAME>_ENABLED` (default true) so an operator can suppress a misbehaving column without code changes.

**Gate:** all 6 columns implement `Column`; full Tier-1 suite green; `golangci-lint run ./internal/retrieval/columns/...` clean.

### Epic 3 — Consensus aggregator + `consensus_strength`

1. `internal/retrieval/consensus.go` — new file. Function: `Aggregate(ctx, columns []Column, query) (ranked []Candidate, consensusStrength float64, perColumnLatency map[string]time.Duration, err error)`.
2. Parallel column execution via `errgroup` with per-column timeout. Failed/timed-out columns excluded from aggregation but counted in `total_columns_queried` denominator (so missing columns *lower* consensus, not silently inflate).
3. RRF formula: `score(node) = Σ (weight_c / (k + rank_c(node)))` where `weight_c` from config, `k = RETRIEVAL_RRF_K` (default 60). Nodes not in column treated as rank=∞ (zero contribution from that column).
4. `consensus_strength = (columns_containing_node / total_columns_queried) × avg(normalized_rank)` — clipped to `[0, 1]`.
5. Property tests: equal weights + identical rankings → consensus_strength = 1.0; disjoint rankings → consensus_strength near 1/N; column failure → consensus drops proportionally.
6. Golden-output regression test on a small fixture (matches Epic 1's fixture).

**Gate:** `consensus.go` ships; property tests + golden test green; aggregator handles 0-column edge case (returns empty + zero strength).

### Epic 4 — Scorer fork + cache scorer-version

1. Edit `scoring.go` to add `ScoreAndRankRRF` function calling `consensus.Aggregate` over the active columns. Existing `ScoreAndRankWithBreakdown` untouched.
2. `service.Retrieve` selects scorer via `cfg.RetrievalColumnVotingEnabled` flag — feature-flag fork. Default `false` (legacy linear); flipped to `true` in Epic 8 commit IF A/B verdict passes.
3. `cache.CacheKey` adds `scorer_version` parameter — value derived from `sha256("v1-rrf6")[:8]` for new path, `sha256("v0-linear")[:8]` for legacy. Different versions = different cache namespaces — no cross-contamination. Bump `scorer_version` whenever weights or column set changes.
4. Tier-2 integration test: run `service.Retrieve` against a seeded Neo4j; toggle the feature flag; assert different rankings + different cache keys.

**Gate:** feature flag works in both directions; cache key bump verified by integration test (cache miss on flag flip); `go test -race ./internal/retrieval/...` green.

### Epic 5 — Downstream consumers (rerank + DH-005)

1. `rerank.go` — accept optional `consensus_strength` input; expose as a feature in the LLM prompt (see existing prompt template). Behavior change: none in legacy path; feature-flagged on `cfg.RetrievalRerankConsumeConsensus` (default false until A/B-validated independently).
2. `internal/ape/self_assess.go` — DH-005 retrieval-confidence dimension can use mean `consensus_strength` over recent retrieves as one of its inputs. Feature-flagged on `cfg.DH005ConsumeConsensus` (default false).
3. Both flags are optional independent toggles — Phase 13 can ship without them lighting up. They exist so Phase 14 doesn't have to touch this code.

**Gate:** both consumers compile; flags toggle correctly; legacy paths unchanged when flags=false.

### Epic 6 — Optional V0017 migration + observability

1. `internal/tsdb/migrations/017_retrieval_consensus.sql` — additive ALTER on `llm_interactions` retrieval rows OR new `retrieval_audit` hypertable. Recommendation: add `retrieval_audit` (clean separation; matches Phase 12 V0016 pattern) with columns `(audit_id CUIDv2 PK, recorded_at TIMESTAMPTZ, query_text_hash, scorer_version, consensus_strength NUMERIC, per_column_latency JSONB, columns_queried INT, columns_returned INT, top_k_node_ids TEXT[])`.
2. Bump `TSDB_REQUIRED_SCHEMA_VERSION` 16 → 17 in `internal/config/config.go`.
3. Forward + reverse migration test on dev DB. Pre+post DO blocks audit row counts.
4. Wire `service.Retrieve` to write a row per retrieve call when `RETRIEVAL_AUDIT_ENABLED=true` (default false to avoid unbounded TSDB growth; operator opts in for observation windows).
5. New Prometheus metrics:
   - `mdemg_retrieval_consensus_strength` (histogram, default buckets 0.0/0.2/0.4/0.6/0.8/1.0)
   - `mdemg_retrieval_column_latency_seconds{column}` (histogram per-column)
   - `mdemg_retrieval_column_failed_total{column,reason}` (counter)

**Gate:** V0017 forward + reverse green; prometheus endpoint shows new metrics; `RETRIEVAL_AUDIT_ENABLED=true` smoke writes rows correctly.

### Epic 7 — UVTS A/B validation (the merge gate)

1. **Run B (candidate)**: with `RETRIEVAL_COLUMN_VOTING_ENABLED=true` + equal column weights, `make test-uvts-quick UVTS_BASE_URL=http://localhost:9999 --persist-tsdb --branch-label phase13-rrf-v1 --codebase-sha $(git rev-parse HEAD)`. Save grades.json.
2. **Compare**: `python3 docs/tests/uvts/runners/uvts_ab_compare.py --baseline /tmp/baseline-grades.json --candidate /tmp/candidate-grades.json --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json --persist-tsdb --baseline-run-id <baseline-uuid> --candidate-run-id <candidate-uuid> --branch-label phase13-rrf-v1-vs-baseline --out /tmp/ab_verdict.json`.
3. **Verdict interpretation**:
   - Exit 0 / `verdict=pass` → flip `RETRIEVAL_COLUMN_VOTING_ENABLED=true` default in Epic 8 commit.
   - Exit 1 / `verdict=fail` → inspect `regressions[]`, iterate (tune column weights or drop a problematic column) and re-run. Stay in feature-flag-off rollout.
   - Exit 2 / drift → reconcile spec / question-set issue first.
4. **Latency check**: capture p50/p95 from `mdemg_http_request_duration_seconds{path=/v1/memory/retrieve}` over the candidate run. Pass criteria: p50 ≤ 1.2× baseline; p95 ≤ 1.5× baseline.
5. **Full-profile A/B** (optional, recommended): if quick passes, run `make test-uvts-full` for both branches. ~$20 OpenAI spend; covers 120 questions vs 16. Same A/B comparison logic.

**Gate:** A/B verdict captured + latency bounded. Pass → Epic 8 flips the default; fail → Epic 8 ships with default-off + post-doc explains regression.

### Epic 8 — Documentation (Final Epic — Never Cut)

1. `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md` — this plan, frozen.
2. `docs/development/post-ft-lora/phase_13_column_voting_post.md` — executed-truth: per-column unit-test green, A/B verdict (pass/fail + delta + regression count), latency p50/p95 deltas, OpenAI spend actual, V0017 row counts (if shipped), decision-fork outcomes.
3. `SPRINT_ROADMAP_POST_FT_LORA.md` — mark Phase 13 EXECUTED with commit SHA; flag Phase 14 (Notes 05+06) unblocked.
4. `AGENT_HANDOFF.md` top entry.
5. `CHANGELOG.md [Unreleased] ### Added`.
6. `CLAUDE.md` — new "Column-Voting Retrieval (Phase 13)" subsection under Architecture Notes.
7. **Conditional**: if A/B passed, flip `cfg.RetrievalColumnVotingEnabled` default `false → true` in `config.go` as part of this commit (single line + comment update).

**Gate:** all docs committed; cross-refs valid; conditional default flip applied if-and-only-if A/B passed.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit) — `go test -race`:**
- `internal/retrieval/columns/embedding_test.go`, `bm25_test.go`, `graph_test.go`, `structural_test.go`, `temporal_test.go`, `role_test.go` — each column on a fixed fixture, golden-output style.
- `internal/retrieval/consensus_test.go` — RRF math properties (identical rankings, disjoint rankings, column failure, weight=0 column treatment, 0-column edge case).
- `internal/retrieval/scoring_golden_test.go` — NEW. Fixed graph fixture + fixed query → fixed expected ranking under both legacy and RRF scorers. Catches future ranking drift.
- `internal/retrieval/cache_test.go` — extended with scorer-version isolation tests.
- `internal/config/config_phase13_test.go` — defaults + env override + Validate cross-field for the ~10 new knobs.

**Tier 2 (Integration) — `go test -tags=integration`:**
- `tests/integration/column_voting_pipeline_test.go` — seed Neo4j with a known graph, run `service.Retrieve` under both `cfg.RetrievalColumnVotingEnabled=false` (legacy) and `=true` (RRF), assert different rankings, assert cache key isolation, assert consensus_strength is in `[0,1]`.
- `tests/integration/v0017_migration_test.go` — forward + reverse on test DB; row counts; pre+post DO blocks emit expected NOTICEs (matches Phase 12 V0016 pattern).
- `tests/integration/retrieval_audit_writes_test.go` — with `RETRIEVAL_AUDIT_ENABLED=true`, assert one row per `service.Retrieve` call lands in `retrieval_audit`.

**Tier 3 (Live E2E) — MANDATORY per CLAUDE.md `d10c1a5`. Real binary, real services, observed outputs:**
- **Live A/B (the merge gate)** — Epic 7 above. UVTS runner + grader against real mdemg + real TSDB. Two branch_label runs + uvts_ab_compare → verdict.
- **Live latency check** — capture p50/p95 on `/v1/memory/retrieve` during the candidate run via Prometheus.
- **Live RetrievalAudit smoke** — with `RETRIEVAL_AUDIT_ENABLED=true`, run 50 retrieves through the real API; assert 50 rows land in `retrieval_audit` with non-zero `consensus_strength`.
- **Live failover** — kill a column's backing service mid-A/B (e.g. break the Neo4j vector index temporarily by closing the driver) and assert the aggregator gracefully degrades: total retrieve still returns; `mdemg_retrieval_column_failed_total{column=embedding}` increments; `consensus_strength` drops proportionally. Only run this on a non-production space (`mdemg-dev`).

**State restoration (MEMORY):** all changes additive. Rollback = `git revert <final commit>` + `mdemg tsdb migrate --target V0016` (drops `retrieval_audit` only). Feature flag `RETRIEVAL_COLUMN_VOTING_ENABLED=false` gives runtime-only emergency disable.

**Gate:** all 3 tiers green; A/B verdict captured (pass OR fail with regression detail); latency bounds met OR documented in post.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(retrieval): Sprint POST-FT-LORA-PHASE13 — Column-Voting Retrieval (RRF over 6 columns + consensus_strength)`
- Body: scope summary, A/B verdict (pass/fail + numbers), latency p50/p95 deltas, V0017 row counts (if shipped), decision-fork outcomes, OpenAI spend actual, conditional default-flip note (was `RETRIEVAL_COLUMN_VOTING_ENABLED` flipped to true? cite the A/B verdict that justified it), policy compliance checklist.
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`.

Push to `reh3376_dev01` → auto-PR opens → **sprint summary comment posted to PR per MEMORY rule (not gated on CI)**.

## 8. Verification Checklist

- [ ] Epic 0: data audit done (null %), baseline grades.json saved, ranker call sites enumerated, RRF lift-pattern understood
- [ ] Epic 1: `internal/retrieval/columns/` exists with 3 refactored columns; legacy paths still callable; Tier-1 green
- [ ] Epic 2: 3 new columns shipped + Tier-1 green; per-column disable knobs work
- [ ] Epic 3: `consensus.go` aggregator + property tests + golden test green
- [ ] Epic 4: scorer fork via `cfg.RetrievalColumnVotingEnabled`; cache key includes scorer_version; integration test confirms cache isolation
- [ ] Epic 5: rerank + DH-005 consumer hooks land (flagged off by default)
- [ ] Epic 6 (if shipped): V0017 forward + reverse green; 3 new Prometheus metrics registered; `retrieval_audit` smoke writes rows
- [ ] Epic 7 (Tier 3 — MANDATORY): UVTS A/B verdict captured (pass / fail / drift); latency p50/p95 within bounds; A/B run rows linked in `uvts_runs.ab_baseline_run_id` / `ab_candidate_run_id`
- [ ] Epic 8: sprint plan + post + ROADMAP "Phase 13 EXECUTED" + AGENT_HANDOFF + CHANGELOG + CLAUDE.md; `RETRIEVAL_COLUMN_VOTING_ENABLED` default flipped IF AND ONLY IF A/B passed
- [ ] Commit pushed; auto-PR updated; **sprint summary posted to PR immediately**
- [ ] OpenAI spend logged, under $100 cap (target $5–25)
- [ ] All 5 decision-fork choices disclosed in commit body + PR comment
- [ ] `golangci-lint run ./...` clean
- [ ] CI green on the auto-PR
- [ ] Memory observation captured (CMS): "Phase 13 Column-Voting shipped. consensus_strength signal now available for Phase 14 (Notes 05+06) consumption."

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 8: `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md` (this plan, frozen), `docs/development/post-ft-lora/phase_13_column_voting_post.md`, `SPRINT_ROADMAP_POST_FT_LORA.md` Phase 13 → EXECUTED with commit SHA, `AGENT_HANDOFF.md` prepended, `CHANGELOG.md [Unreleased] ### Added`, `CLAUDE.md` adds new "Column-Voting Retrieval (Phase 13)" subsection covering: feature-flag enable/disable, the 6 columns + their config knobs, `consensus_strength` consumers, A/B harness recipe.

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | **A/B regression on quick profile** — RRF v1 underperforms linear scorer on lnl_demo questions | Medium | Equal weights are a deliberately weak prior; expect to need ≥1 iteration. Start with quick profile (16q, ~$0.60) for fast feedback before full run | Iterate column weights / drop a column / inspect `regressions[]`; ship with default-off if can't reach parity in this sprint |
| 2 | **Latency regression** — 6 parallel columns + aggregator add overhead vs single linear pass | Medium | `errgroup` parallel execution + per-column timeout (80% of total budget); columns are independent | Drop the slowest column (likely Structural with multi-hop Cypher); tune hop depth; document p95 increase in post |
| 3 | **Cache pollution** — A/B results contaminated by stale cached entries from legacy ranker | High (without mitigation) | Cache key includes `scorer_version` hash; flag-flip = different namespace = automatic cold cache | Manual cache flush before A/B (`POST /v1/admin/cache/flush` if endpoint exists; else restart mdemg) |
| 4 | **Decision fork: TSDB schema (V0017 audit table vs no migration)** | Medium | Recommend V0017 with `retrieval_audit` hypertable. Adds ~1 day cost but starts the Phase 14 baseline-rows clock immediately | Skip V0017; rely on Prometheus metrics + log inspection for observability; can add V0017 in Phase 13.1 |
| 5 | **Decision fork: column weight defaults — equal vs heuristic vs ablation** | Medium | Recommend equal (`1.0/N` per active column) as v1 prior; ablation as Phase 13.1 follow-up | Ablation in-sprint adds ~3 days; deferred is the right call given the sprint is already 12 days |
| 6 | **Decision fork: structural hop depth (1 vs 2 vs 3)** | Low | Recommend 2 (Note 04 doc range midpoint); expose `RETRIEVAL_STRUCTURAL_HOPS` config knob so operator can tune post-merge | Start at 1 if Cypher latency on a populated graph blows the per-column timeout |
| 7 | **Decision fork: rollout — flag default false vs true** | High importance | Recommend default `false`, flip to `true` in same commit IF A/B passes. Matches Phase 11.6.3 pattern | Default `true` from day 1 if A/B pass is overwhelming (mean+1pp + zero regressions); operator escape hatch via `RETRIEVAL_COLUMN_VOTING_ENABLED=false` + restart |
| 8 | **Null/missing temporal or role metadata** | Medium | Epic 0 audit catches >5% null rate; Epic 0.5 backfill if needed | Disable Temporal/RoleScoped columns via per-column knob; ship with 4 active columns; document gap in post |
| 9 | **Reranker (LLM-bound) interaction surfaces unexpected behavior** | Low | Reranker is post-scorer + orthogonal; `consensus_strength` consumption is feature-flagged-off in this sprint | Disable reranker during A/B (`cfg.RerankEnabled=false`) for clean signal |
| 10 | **Phase 11.6.3 watchdog fires during A/B** (mlx Metal-OOM mid-A/B run interrupts the candidate measurement) | Low | Watchdog is precisely the mitigation for this; disabled-by-default though, so A/B operator should enable it for the duration of the soak | If Smoke 2 hasn't run yet on watchdog, do A/B with conservative mlx flags (Phase 11.6.3 plan) — same posture as Phase 12 Epic 7 |
| 11 | **Cross-cutting: stale `gpt-5.4-mini` rows from pre-cutover bypass sites** | Low (already swept) | Phase 11.6.2 closed the 6th cutover-bypass site. Pre-A/B sweep at Epic 0 re-checks `cfg.OpenAIEndpoint` direct uses | Patch as Phase 13.1 follow-up (mirrors 11.6.2 pattern) |
| 12 | **golangci-lint regression on the new columns package** | Low | Run lint per epic before gate; Phase 11.6.3 already established the lint-clean baseline | Fix in-sprint; lint rules are stable |
| 13 | **CI workflow doesn't trigger on retrieval changes** | Low | Existing path triggers cover `internal/retrieval/**`; verify by reading `.github/workflows/`. UVTS A/B is operator-run, not CI-run for this sprint | If CI doesn't fire, push a forced-touch commit to a covered file; document the gap |

## 11. Documents Accessed (during planning)

**Read during planning (3 parallel Explore agents):**
- `/Users/reh3376/mdemg/docs/research/mdemg_sprint_ideas/04-column-voting-retrieval.md` — 412-line design draft. RRF formula, 6 columns, consensus_strength, A/B gate, acceptance criteria, dependencies, open questions.
- `/Users/reh3376/mdemg/internal/retrieval/scoring.go:647-887` — current `ScoreAndRankWithBreakdown` linear formula. The replacement target. ~12 signal components.
- `/Users/reh3376/mdemg/internal/retrieval/hybrid.go:35-144` (`BM25Search`) + `:197-284` (`ReciprocalRankFusion`) — existing 2-column RRF; lifted/generalized in `consensus.go`.
- `/Users/reh3376/mdemg/internal/retrieval/service.go:313-765` (`Retrieve`) + `:857-950` (`vectorRecall`) + `:406-451` (RRF entry point) — pipeline orchestration; single ranker call site.
- `/Users/reh3376/mdemg/internal/retrieval/activation.go:14-134` (`SpreadingActivation`) + `:369-489` (`SpreadingActivationWithAttention`) — graph proximity column source.
- `/Users/reh3376/mdemg/internal/retrieval/cache.go:56-83` (`CacheKey`) — must add scorer_version.
- `/Users/reh3376/mdemg/internal/retrieval/rerank.go:41-212` — orthogonal LLM-bound reranker; optional consensus_strength feature consumer.
- `/Users/reh3376/mdemg/internal/api/handlers.go:431-550` (`handleRetrieve`) — pipeline entry/exit (HTTP boundary).
- `/Users/reh3376/mdemg/internal/config/config.go:94-178, 696-704` — current ranker config knobs (alpha/beta/gamma/etc.).
- `/Users/reh3376/mdemg/docs/tests/uvts/runners/uvts_runner.py` + `uvts_ab_compare.py` — Phase 12 A/B harness; merge-gate criterion implementation.
- `/Users/reh3376/mdemg/docs/tests/uvts/specs/lnl_demo_validation.uvts.json::ab_mode` — `regression_threshold_per_question=0.10` is the canonical bar.
- `/Users/reh3376/mdemg/docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` §3.3 / Phase 13 entry — strategic positioning, "lowest-risk highest-coverage Tier 2 extension."
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_11_6_3_post.md` — Phase 11.6.3 watchdog status (precondition for sustained live A/B).
- `/Users/reh3376/mdemg/CLAUDE.md` — Architecture Notes, MLX Watchdog subsection, live-testing-required Tier-3 mandate (commit `d10c1a5`).
- `/Users/reh3376/mdemg/internal/retrieval/scoring_test.go`, `hybrid_test.go`, `service_test.go`, `activation_test.go`, `rerank_compress_test.go`, `cache_test.go` — existing test inventory; no golden-output regression tests today.
- Memory: `feedback_sprint_plan_format.md`, `feedback_sprint_summary_on_pr.md`, `feedback_no_hardcoded_values.md`, `feedback_min_max_tokens_3000.md`, `feedback_min_latency_budget_15000.md`, `feedback_cuidv2_required.md`, `feedback_sequential_epics.md`, `feedback_mandatory_testing_tiers.md`, `feedback_plan_options_pattern.md`, `feedback_no_tight_llm_budget_caps.md`, `feedback_sprint_plans_location.md`, `feedback_live_testing_required.md`, `project_mdemg_purpose.md`, `feedback_rigorous_verification.md`, `feedback_planning_before_code.md`.

## 12. Rollback

All changes additive — no destructive ops on existing Neo4j or TSDB rows.

1. `git revert <final commit SHA>` — removes columns/, consensus.go, scorer fork, cache scorer_version, rerank consumer, ape consumer, config knobs, V0017 (if shipped), docs.
2. **Runtime emergency disable** (no rebuild needed): `RETRIEVAL_COLUMN_VOTING_ENABLED=false` in `.env` + `mdemg restart`. Reverts to legacy linear scorer immediately. Cache becomes cold (different scorer_version namespace) — first 5 minutes of post-disable retrieves are uncached, then warm.
3. **TSDB rollback** (only if V0017 shipped): `mdemg tsdb migrate --target V0016` drops `retrieval_audit` only. UVTS V0016 rows from Phase 12 preserved.
4. **Per-column suppress** (no rebuild, no restart needed if config hot-reload): `RETRIEVAL_COLUMN_<NAME>_ENABLED=false` for the misbehaving column. Aggregator skips it; consensus drops by 1/N.
5. **Cache flush** if cross-version pollution suspected (shouldn't happen due to scorer_version, but operator can force): `mdemg cache flush --space mdemg-dev`.

Phase 11.5 + 11.6 + 11.6.x + 11.6.2 + 12 + 11.6.3 artifacts untouched. mdemg-llm-v1 model untouched. No Neo4j writes from this sprint.

---

## 13. Plan-Options (decision forks — pick at execution, disclose in PR)

Per MEMORY `feedback_plan_options_pattern.md`:

| Fork | Recommended | Alternative(s) | Rationale for recommendation |
|---|---|---|---|
| **Validation merge bar** | **10% per-question regression** (matches `lnl_demo_validation.uvts.json::ab_mode.regression_threshold_per_question`) | (B) 15% per Note 04 doc; (C) Custom per-category bound | Spec is canonical (ConflictTracker logic in Phase 12 already keys on it); stricter is safer for first deploy. The Note 04 doc was written before the spec landed; resolve in favor of code. |
| **Cache invalidation** | **Scorer-version hash in cache key** | (B) Auto-flush on deploy; (C) No invalidation (accept stale) | Cache key change is surgical, automatic, and per-namespace. Auto-flush loses warm cache for hours after every deploy; accepting stale poisons A/B measurements (the failure mode this sprint must avoid). |
| **Structural column hop depth** | **2 hops** (Note 04 range midpoint) | (B) 1 hop (cheaper); (C) 3 hops (more recall) | 2 is the safe middle. `RETRIEVAL_STRUCTURAL_HOPS` config knob lets operators tune post-merge once latency data is in. Don't over-fit to a guess. |
| **Per-column weight defaults** | **Equal weights** (`1.0/N` per active column) | (B) Heuristic priors (e.g., embedding=0.4, others lower); (C) Defer to ablation study | Ablation requires A/B cycles per weight set, multiplying sprint cost. Equal weights is the honest "no prior knowledge" baseline. Phase 13.1 (ablation) is the right place for tuning, after baseline data. |
| **Rollout shape** | **Default `false`, flip to `true` in same commit IF A/B passes** | (B) Default `true` day-1 (Note 04 doc); (C) Default `false`, defer flip to follow-up sprint | Matches Phase 11.6.3 pattern. Tiny rollback surface. The flip-in-same-commit is the precedent set last week; deferring it would just be a 5-line follow-up sprint nobody benefits from. |
| **TSDB V0017 audit table** | **Ship V0017 with `retrieval_audit` hypertable** | (B) Skip V0017; rely on Prometheus + logs only; (C) Add to existing `llm_interactions` via discriminator column | Phase 14 (Notes 05+06) consumes `consensus_strength` as input; having historical baseline rows from day 1 of the rollout simplifies their A/B. ~1 day cost; clean separation. Mirrors Phase 12 V0016 design. |
| **Reranker `consensus_strength` consumption** | **Wire the hook, default flag off** | (B) Wire and enable; (C) Don't wire | Wire-but-off keeps Phase 14 from having to touch this code. Enabling now risks confounding the Phase 13 A/B signal with reranker behavior change. Same for DH-005 input. |

If A/B fails on the quick profile (16 questions, ~10 minutes per side), iterate weights or drop a column without leaving the sprint. If full-profile A/B (120 questions, ~$20) is needed for confidence, run it as part of Epic 7 — recommended since per-question regressions on rare categories may not surface in the quick profile.
