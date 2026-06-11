# Sprint Plan HIDDEN-WEIGHT-001 — Real Weights on the Abstraction Hierarchy

## 1. Header & Metadata

- **Sprint ID:** HIDDEN-WEIGHT-001 (Roadmap Q3 Phase 1, rank #3)
- **Sprint line:** `docs/development/hidden-weight-001/`
- **Date opened:** 2026-06-11
- **Branch:** `reh3376_dev01`
- **Target version:** v0.10.x
- **Estimated effort:** ~2 dev-days
- **OpenAI spend:** $0
- **Risk level:** Medium (graph-wide backfill; mitigated by LIMIT-5-first + dry-run + batching)

## 2. Problem Statement

Every abstraction edge the hidden layer creates carries a NULL weight.
`point.distance()` is a spatial-Point function — on embedding float-lists it
returns NULL (proven live: NULL on a real pair where
`vector.similarity.cosine` returns 0.627). Three creation sites in
`internal/hidden/service.go` (~707 theme→GENERALIZES — no null-guard at all;
~4477 obs→theme GENERALIZES; ~5509 theme→concept **ABSTRACTS_TO** — the
audit missed this type) compute `1.0 - point.distance(...)/2.0` → NULL →
the property is never set.

**Live scale (2026-06-11):** GENERALIZES **28,332/28,332 NULL (100%)**;
ABSTRACTS_TO **36,110/37,996 NULL (95%)** — 64,442 weightless abstraction
edges, growing by thousands per day via consolidation churn. The 1,886
weighted ABSTRACTS_TO edges got `0.5` from the *missing-embedding* fallback
— edges with good embeddings got NULL, edges without got a value. Decay,
prune, the backward pass, and the RRF graph column all compute over this
missing data.

Bonus finding: all three sites mint `edge_id: randomUUID()` — the CUIDv2
identifier standard applies to touched Cypher.

## 3. Scope & Constraints

**In scope:**
- Replace `1.0 - point.distance(a,b)/2.0` with `vector.similarity.cosine(a,b)`
  at the 3 sites. Verified live: Neo4j's cosine returns **[0,1]** directly
  (identical=1.0, orthogonal=0.5, opposite=0.0) — same intended range, no
  transform. Null-guard CASE retained (fallback 0.5 unchanged).
- CUIDv2 `edge_id` at the touched sites (Go-side `cuid2.Generate()` per
  member, passed as zipped pairs — Cypher can't mint CUIDv2).
- **Backfill** both edge types where `weight IS NULL`: both endpoint
  embeddings present → `vector.similarity.cosine`; else 0.5 (the sites' own
  fallback semantic). Sets `similarity_score` alongside where the schema
  carries it. LIMIT-5 verification first, then batched
  (`CALL {} IN TRANSACTIONS`-style batching via Go loop), per the
  small-batch-first rule. Existing `edge_id`s untouched (historical UUIDs
  remain valid opaque strings — alert-ID precedent).
- **Observability:** per-space `neo4j_graph_null_weight_edges` gauge in the
  existing graph-stats collector (server.go Query-4 + collectors.go) →
  `metric_samples` → new DefaultRules entry `null_weight_abstraction_edges`
  (distinct service `graph-weight-integrity` per the cooldown rule). Post-
  backfill steady state is 0; any reappearance = this bug class regressing.
- 3-tier tests incl. live Tier 3; docs.

**Out of scope:**
- Concept identity/churn, the 9,687 childless L2s, coverage retune —
  HIDDEN-CHURN-001 (Phase 2).
- Decay/prune actually *running* on schedule — MAINT-LIVE-001 (next).
- Re-tuning decay/prune formulas for the now-real weights (they were
  designed for real weights; NULL was the anomaly).
- Migrating pre-existing UUID edge_ids.

**Constraints:** sequential epics; EXPLAIN-validate Cypher edits; destructive
ops dry-run + LIMIT 5 first; no hardcoded values (rule threshold
config-driven).

## 4. Dependencies

- Neo4j 5.x `vector.similarity.cosine` (verified live).
- Existing graph-stats collector (`server.go::collectGraphMetrics` pattern,
  `metrics.SpaceGraphData`, `CollectNeo4jGraphMetrics`) + `metric_samples`
  + DefaultRules.
- `github.com/nrednav/cuid2` (in go.mod).

## 5. Implementation Plan

- **Epic 0 — Investigation + plan (done):** sites read; bug + cosine
  semantics + scale proven live; alert vehicle identified.
- **Epic 1 — Fix the 3 creation sites (~0.5d):** cosine + retained
  null-guard; CUIDv2 edge ids via zipped member/edge-id pairs (Go-side
  generation, UNWIND over pairs); EXPLAIN-validate all three statements;
  unit-test the pair-building helper; learning/hidden suites green.
- **Epic 2 — Backfill (~0.5d):** Go CLI path `mdemg graph repair` extension
  or one-shot in `internal/hidden` invoked via a small admin path — decide
  at execution (disclosed; lean: extend `graph repair`, it already owns
  weight-preserving graph surgery + has dry-run/limit conventions). Steps:
  dry-run count → LIMIT 5 live + verify by hand → batched full run (1,000/
  batch) → post-counts (0 NULL expected) for BOTH edge types.
- **Epic 3 — Observability (~0.25d):** Query-4 in the stats collector
  (per-space NULL-weight count over GENERALIZES|ABSTRACTS_TO), gauge,
  DefaultRules entry (`NULL_WEIGHT_EDGE_ALERT_THRESHOLD` env, default 100 —
  tolerates in-flight creation bursts between collector ticks, catches
  systemic regression), distinct service.
- **Epic 4 — Tier 3 live verification (~0.5d):** trigger a real
  consolidation cycle → new edges carry real cosine weights (not NULL, not
  uniformly 0.5); weight distribution sanity (spread across [0,1], mean in
  a plausible band); gauge visible in `/v1/metrics` + `metric_samples`
  rows; rule loaded (evaluator count) and silent at steady state.
- **Epic 5 — Documentation (final, never cut):** CHANGELOG; CLAUDE.md
  architecture note; roadmap tick; feature-doc section (graph-weight
  integrity) folded into the existing graph-health docs; post.md.

## 6. Testing Plan (3 tiers)

- **Tier 1:** pair-builder unit tests (CUIDv2 ids unique per member); rule
  presence/SQL tests (mirror HookHealthRules tests); collectors gauge test
  if harness exists.
- **Tier 2:** EXPLAIN-validation of the 3 edited statements + the backfill
  Cypher against live Neo4j (side-effect-free); `go test ./internal/hidden/
  ./internal/api/ ./internal/alert/ ./internal/metrics/`.
- **Tier 3 (live):** LIMIT-5 backfill verified by hand → full backfill →
  0 NULL across both types; real consolidation creates real-weight edges;
  gauge + rule live in the running server. Live smoke item: *run a
  consolidation against the real system, observe new GENERALIZES/
  ABSTRACTS_TO edges with cosine weights in Neo4j and the
  null-weight gauge at 0 in metric_samples, confirm the alert stays
  silent.*
- **UVTS:** the RRF graph column consumes edge weights — run
  `make test-uvts-quick` after backfill as a regression guard (retrieval
  quality must not degrade; improvement is possible but not claimed).

## 7. Commit Strategy

One commit per epic; live-smoke surprises get standalone fix-commits;
push → auto-PR → summary.

## 8. Verification Checklist

- [ ] All 3 sites EXPLAIN-validate; new edges carry cosine weights (live)
- [ ] CUIDv2 edge ids on newly created abstraction edges
- [ ] LIMIT-5 backfill hand-verified before full run
- [ ] Post-backfill: 0 NULL-weight GENERALIZES AND ABSTRACTS_TO
- [ ] Weight distribution sane (not all 0.5; spread within [0,1])
- [ ] Gauge in metric_samples; rule loaded (evaluator count +1) and silent
- [ ] `make test-uvts-quick` no regression
- [ ] Suites green; lint clean; docs updated

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Backfill writes wrong weights at scale | Low | High | LIMIT-5 hand-verification first; batched; weights recomputable (pure function of embeddings) so a re-run heals |
| Consolidation churn races the backfill (new NULL edges mid-run) | High | Low | Fix the creation sites FIRST (Epic 1 ships before Epic 2 runs); post-fix edges are born correct; final count check catches stragglers |
| Real weights change decay/prune behavior unexpectedly | Medium | Medium | Decay/prune don't run on schedule yet (MAINT-LIVE-001 pending) — this lands while they're inert; UVTS quick guards retrieval |
| vector.similarity.cosine NULL on malformed embeddings | Low | Low | Null-guard CASE retained with the 0.5 fallback |
| Gauge query cost on large graphs | Low | Low | Single aggregate count per tick, same session as existing queries, collector cache unchanged |

## 11. Documents Accessed

- `internal/hidden/service.go` (3 sites, read in context)
- Live Neo4j: point.distance NULL proof, cosine range proof, edge counts
- `internal/api/server.go::collectGraphMetrics` + `internal/metrics/collectors.go`
- `internal/alert/rules.go` (DefaultRules graph rules + metric_samples)
- `docs/development/roadmap/ROADMAP_2026Q3.md` (scope)

## 12. Rollback Procedures

- Epic 1: revert — sites resume creating NULL weights (the prior state).
- Epic 2: weights are additive properties; a wrong backfill is re-runnable
  (pure function of embeddings). No restore needed.
- Epic 3: revert — gauge/rule disappear; no data migration.
