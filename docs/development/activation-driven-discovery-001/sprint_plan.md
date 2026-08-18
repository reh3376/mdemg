# ACTIVATION-DRIVEN-DISCOVERY-001 — Sprint Plan

**Master arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase B1

## 1. Header & Metadata

- **Sprint ID**: `ACTIVATION-DRIVEN-DISCOVERY-001`
- **Arc**: JIMINY-SUBSTRATE-NATIVE-001 (Phase B1)
- **Author**: reh3376 / claude
- **Date**: 2026-08-18
- **Branch**: `reh3376_dev01`
- **PR target**: #TBD (auto-PR on push)
- **Estimated wall-clock**: ~4-6 hours
- **Sprint format**: v1.0 (12-section)

## 2. Problem Statement

Jiminy's **Lever C** (`fetchActionableCandidates`, `internal/jiminy/service.go:3372`) fetches actionable constraint/correction candidates via **pure role-filtered vector cosine similarity** — one Cypher query, no graph traversal, no Hebbian edge-weight signal, no `activation_confidence` weighting. This ignores the substrate MDEMG was designed around:

- **228,636 `CO_ACTIVATED_WITH` edges** on mdemg-dev carry Hebbian co-activation weights (mean 0.14, range [0.02, 1.0]) — this is the graph telling us which constraints tend to be relevant TOGETHER with which contexts. Currently unused.
- **`activation_confidence` populated on 79% of nodes** (69,635/88,276) — precision weighting per HEBB-ETA-001. Currently unused for guidance surfacing.
- **`SpreadingActivationWithAttention`** is a shipped primitive (`internal/retrieval/activation.go:371`) used by the graph column for retrieval; **Jiminy's Lever C bypasses it entirely**.

The result: Lever C surfaces constraints that are **topically similar** to the query text but ignores what the substrate has *learned* about which constraints activate together with which contexts. This is the core JIMINY-SUBSTRATE-NATIVE-001 arc thesis: Jiminy has been fighting against MDEMG's intended architecture by treating retrieval as pure semantic embedding rather than as an activation-driven graph process.

## 3. Scope & Constraints

### In scope
1. **New method on `retrieval.Service`**: `ExpandSeedsByActivation(ctx, spaceID, seeds []ActivationSeed, queryText string) (map[string]float64, error)` — takes seed node IDs with initial scores, fetches 1-hop outgoing edges from those seeds, runs `SpreadingActivationWithAttention` with query-context-derived edge attention, returns final activation scores keyed by node_id (includes seeds + newly-activated neighbors).
2. **Extend `jiminy.RetrievalProvider` interface** with the same method.
3. **Extend jiminy adapter** in `internal/api/rsic_adapters.go` to wire the new method through.
4. **Modify `fetchActionableCandidates`** in Jiminy to optionally rerank by activation-enriched score when the flag is on.
5. **Feature flag** `JIMINY_LEVER_C_ACTIVATION_ENABLED` (default false in code AND `.env`).
6. **Config knobs**: `JIMINY_LEVER_C_ACTIVATION_STEPS` (2), `JIMINY_LEVER_C_ACTIVATION_LAMBDA` (0.5), `JIMINY_LEVER_C_ACTIVATION_WEIGHT` (0.3) — the blend weight for activation score vs original cosine.
7. **URL override**: `?leverc_activation=true|false` — for A/B measurement (mirrors `?leverc_topk`, `?reverse_ref` shape).
8. **Pin tests**: default-off render is byte-identical to current Lever C output; activation-on produces different but valid ranking with same coverage.

### Out of scope (deferred to follow-ups or later phases)
- **B2 Hebbian-effectiveness prior via GUIDANCE_OUTCOME** — blocked: `GUIDANCE_OUTCOME` sink has **0 rows on mdemg-dev**. Investigation of *why* the shipped write path isn't producing rows is disclosed as a follow-up. B1 uses CO_ACTIVATED_WITH weights (populated) — B2 would additionally consume GUIDANCE_OUTCOME reinforcement (empty today).
- **B3 precision-confidence weighting** — could be additive in a small follow-up; deferred to keep this sprint compact and testable.
- **Full LLM-classifier prompt clause retirement (Phase D)** — depends on B1 + C first.

### Hard invariants (MUST NOT break)
- **Actionable coverage**: the current role-filtered cosine seed set MUST still be computed and included — activation reranks, it does NOT filter. Zero risk of an activation-driven variant returning fewer actionables than the current pure-cosine.
- **RRF-SCALE-001 contract**: activation output is stable [0,1], safe to combine with the cosine seed score. Never gate on `RetrieveResult.Score`.
- **CACHE-KEY-002 forcing function**: `RetrieveRequest.LeverCActivationOverridePresent` + `RetrieveRequest.LeverCActivationEnabled` classified in `cacheKeyNeutralFields` — same shape as ReverseRefOverride, ConcreteRecallOverride. (⚠️ This flag lives on Jiminy path, not RetrieveRequest — so no CacheKey change. Confirm during implementation.)
- **JIMINY_MODE=strict preserved**: no change to enforcement gate; only affects Lever C surfacing composition/ordering.
- **`must-validate-all-claims-before-commit`**: every substrate assumption verified live on mdemg-dev before drafting Cypher/code.

## 4. Dependencies

**Upstream (must be shipped)**:
- ✅ `SpreadingActivationWithAttention` (RETRIEVAL-TYPED-EDGES-001, 2026-07-01)
- ✅ `ComputeEdgeAttention` (RETRIEVAL-TYPED-EDGES-001)
- ✅ `fetchOutgoingEdges` (existing retrieval infrastructure)
- ✅ CO_ACTIVATED_WITH edge population (EVENTGRAPH-003+004, 2026-06-09/10)
- ✅ Lever C shipped (JIMINY-ACTIONABILITY-001 Epic 5)
- ✅ LEVER-C-TIGHTEN-001 + 002 shipped (Lever C is the active surface)

**Downstream (this sprint unblocks)**:
- Phase B2 (Hebbian effectiveness prior) — depends on this + GUIDANCE_OUTCOME sink repair
- Phase B3 (precision-confidence weighting) — additive on top of this
- Phase C (layer/edge-aware surfacing)

## 5. Implementation Plan

### Epic 1: retrieval.Service.ExpandSeedsByActivation primitive (~1h)
- New file `internal/retrieval/expand_seeds.go`
- Public type `ActivationSeed { NodeID string; Score float64 }`
- Method `func (s *Service) ExpandSeedsByActivation(ctx context.Context, spaceID string, seeds []ActivationSeed, queryText string) (map[string]float64, error)`
  - Convert seeds → `[]Candidate` (only NodeID + VectorSim populated — the primitive only reads those)
  - Extract seed IDs → `fetchOutgoingEdges(ctx, []string{spaceID}, seedIDs)`
  - Build `QueryContext{QueryText: queryText}` (Jiminy queries don't traverse the `isCodeQuery`/`isArchitectureQuery` gates — Jiminy's request.Context is constraint/action text, not code queries; use plain QueryText)
  - `attention := ComputeEdgeAttention(qctx, s.cfg)`
  - Steps + lambda from config (`LEVER_C_ACTIVATION_STEPS`, `_LAMBDA` if set; else defaults 2, 0.5)
  - `act := SpreadingActivationWithAttention(cands, edges, steps, lambda, attention, []float64{0.0, 0.05})`
  - Return act
- Fail-open: nil-driver / edge-fetch error → return empty map + WARN (Jiminy caller falls back to raw seeds)

### Epic 2: RetrievalProvider interface extension (~30min)
- Add to `internal/jiminy/types.go::RetrievalProvider`:
  ```go
  ExpandSeedsByActivation(ctx context.Context, spaceID string, seeds []ActivationSeed, queryText string) (map[string]float64, error)
  ```
- Mirror type `jiminy.ActivationSeed` (identical shape — decouples the packages; adapter maps between)
- Update `jiminyRetrievalAdapter` in `internal/api/rsic_adapters.go` — new method converts `[]jiminy.ActivationSeed` → `[]retrieval.ActivationSeed` → calls `retrieval.Service.ExpandSeedsByActivation` → returns map unchanged.
- Update `mockRetriever` in `internal/jiminy/j7_j12_test.go` — new method returns empty map + nil.

### Epic 3: Lever C activation-enrichment integration (~1.5h)
- New function in `internal/jiminy/service.go`:
  ```go
  func (s *Service) activationEnrichLeverC(ctx context.Context, spaceID string, actionables []GuidanceItem, queryText string) []GuidanceItem
  ```
- Guard: nil retriever, empty actionables, disabled flag → return input unchanged
- Build `[]ActivationSeed` from actionables (NodeID = SourceNodes[0], Score = Confidence)
- Call `s.retriever.ExpandSeedsByActivation(...)`
- For each actionable, compute blended score: `blended := (1-w)*item.Confidence + w*activation[item.NodeID]`
  - `w := s.cfg.JiminyLeverCActivationWeight` (default 0.3)
  - Nodes with no activation score → blended = item.Confidence (unchanged)
- Sort actionables by blended score DESC
- Return the top-K (same count as input — activation reranks; it does NOT change count)
- Update per-item Confidence to blended value so downstream ranking sees it
- Wire into `Guide()` at line ~1215: after `fetchActionableCandidates`, if flag on, `actionable = s.activationEnrichLeverC(ctx, req.SpaceID, actionable, queryText)`
- Debug field: `debug["leverc_activation_enriched"] = len(actionable)`, `debug["leverc_activation_edges"] = <count>` (if returned by primitive; add later if not)

### Epic 4: Config + URL override (~30min)
- 4 new config fields in `internal/config/config.go`:
  - `JiminyLeverCActivationEnabled bool` (env `JIMINY_LEVER_C_ACTIVATION_ENABLED`, default false)
  - `JiminyLeverCActivationSteps int` (env `..._STEPS`, default 2)
  - `JiminyLeverCActivationLambda float64` (env `..._LAMBDA`, default 0.5)
  - `JiminyLeverCActivationWeight float64` (env `..._WEIGHT`, default 0.3, floor 0.0, ceiling 1.0)
- Startup log line: `slog.Info("jiminy: lever c activation", "enabled", cfg.JiminyLeverCActivationEnabled, "steps", ..., "lambda", ..., "weight", ...)`
- URL override on `POST /v1/jiminy/latest` + `POST /v1/jiminy/guide`: `?leverc_activation=true|false` — parsed via existing `getBool` helper; overrides `cfg.JiminyLeverCActivationEnabled` for the single request. Since this changes surfacing composition, wire it through the existing warm/latest override plumbing.

### Epic 5: Testing (~1h)
- Unit tests in `internal/retrieval/expand_seeds_test.go`:
  - Empty seeds → empty map + nil error
  - Seeds with no edges → returns map with seed scores unchanged
  - Seeds with edges → returns map with additional keys + non-negative scores
  - Cancellation → propagates ctx error, no partial state
- Unit tests in `internal/jiminy/lever_c_activation_test.go`:
  - `activationEnrichLeverC` disabled → identity function
  - Enabled with mock retriever returning empty map → scores unchanged
  - Enabled with mock retriever adding activation to top item → reordering respects blend weight
  - Zero-weight config → identity function (defensive)
- Pin: `TestLeverC_ActivationDefaultOffByteIdentical` — with flag off, output list equals current fetchActionableCandidates output exactly (order + scores).

### Epic 6: Live Tier-3 verification on mdemg-dev (~30min)
- Build: `go build -o bin/mdemg ./cmd/mdemg`
- Launch pre-check: `curl -s http://127.0.0.1:8102/v1/models > /dev/null || (echo "llama-server down; abort"; exit 1)`
- Kickstart: `launchctl kickstart -k gui/501/com.mdemg.server`
- Verify boot log includes `jiminy: lever c activation enabled=... steps=2 lambda=0.5 weight=0.3`
- Baseline (flag OFF): `curl POST /v1/jiminy/guide` with a real constraint-topic query → capture surfaced items
- Comparison (flag ON via URL): `curl POST /v1/jiminy/guide?leverc_activation=true` with same query → capture surfaced items
- Diff: expect same coverage (same actionable node IDs present), potentially different ORDER
- Substrate cross-check via cypher-shell: pick a surfaced item, verify its 1-hop CO_ACTIVATED_WITH neighbors have non-zero weights; confirm the activation score for a well-connected item is higher than a poorly-connected item

### Epic 7: Documentation (~30min)
- **`docs/features/jiminy-lever-c-activation.md`** (new) — Why / Choices / How it works / How to use
- **`CLAUDE.md`** — architecture note: JIMINY-SUBSTRATE-NATIVE-001 Phase B1 shipped; Lever C now optionally enriches by activation spreading; default-off; env knobs; disclosed follow-up: GUIDANCE_OUTCOME sink=0 investigation
- **`CHANGELOG.md`** — Unreleased entry
- **`docs/development/activation-driven-discovery-001/sprint_post.md`** — sprint post-mortem with recon findings, decisions, live-smoke evidence, follow-ups

### Epic 8: Follow-up documentation
- Document the **GUIDANCE_OUTCOME sink=0 blocker** in sprint post — recommend a small investigation sprint before Phase B2 kickoff. Check: is `PersistGuidanceOutcome` being called from `RecordOutcome`? Is the Cypher failing? Is `constraint_code` matching finding zero constraint nodes? (The 79 recent `constraint_outcomes` TSDB rows suggest outcomes are recorded to TSDB but not to Neo4j — likely a specific code path fails silently.)

## 6. Testing Plan (3 tiers required by unit-integration-e2e-docs)

### Tier 1 — Unit (`go test ./internal/retrieval/... ./internal/jiminy/...`)
- Every new function has ≥1 unit test.
- `expand_seeds_test.go` — 4 subtests (empty seeds, no-edges, with-edges, cancellation).
- `lever_c_activation_test.go` — 4 subtests (disabled, empty activation map, reranking, zero-weight).
- Pin `TestLeverC_ActivationDefaultOffByteIdentical` — enforces the "default off = current behavior" invariant.
- Update `mockRetriever` in `j7_j12_test.go` (existing test file) to implement the new interface method.

### Tier 2 — Integration (`go build ./... && go test -tags=integration ./tests/integration/...`)
- Full compile clean; interface implementation checked at build time (mockRetriever + jiminyRetrievalAdapter both satisfy).
- Existing UATS suite (`make test-api`) must remain green — no schema/handler changes; the URL override is additive.

### Tier 3 — Live end-to-end (mdemg-dev)
- Fresh binary kickstart; boot log confirms flag state.
- `POST /v1/jiminy/guide?leverc_activation=false` (baseline) → capture response.
- `POST /v1/jiminy/guide?leverc_activation=true` (candidate) → capture response.
- Assert: identical `SourceNodes` set across both (coverage preserved).
- Assert: ordering differs OR is identical-but-with-different-Confidence (activation ran + affected scores).
- cypher-shell cross-check: for a top-ranked item in the activation-on response, verify 1-hop CO_ACTIVATED_WITH neighbors have edge weights that would produce a meaningful activation lift (weight * seed_score > blend threshold).
- Cleanup: no test data left in TSDB or Neo4j (Lever C is pure read; no writes).

## 7. Commit Strategy

- **1 primary commit** for the sprint shipped code (Epics 1-5).
- **1 follow-up commit** for docs (Epic 7) if the code-commit CI is green first.
- Any live-smoke-discovered surprise defect gets its own **fix-commit** (per Phase 11.6.2 precedent).
- No cross-cutting refactors bundled — this sprint stays additive.
- Commit message shape: `feat(jiminy): activation-driven Lever C reranking (default-off) [ACTIVATION-DRIVEN-DISCOVERY-001, phase b1]`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `/Users/reh3376/go/bin/golangci-lint run ./...` clean
- [ ] `go test ./internal/retrieval/... ./internal/jiminy/...` green
- [ ] `go test ./...` green (broader smoke)
- [ ] Boot log confirms `jiminy: lever c activation enabled=false ...` (default)
- [ ] Boot log confirms `jiminy: lever c activation enabled=true ...` when `.env` sets it
- [ ] `POST /v1/jiminy/guide?leverc_activation=false` returns current behavior (baseline)
- [ ] `POST /v1/jiminy/guide?leverc_activation=true` returns same actionable coverage
- [ ] Live smoke: pick a surfaced item, verify CO_ACTIVATED_WITH neighbors exist + have weights
- [ ] Sprint plan lives at `docs/development/activation-driven-discovery-001/` (per `project-planning-docs-in-repo-only`)
- [ ] `docs/features/jiminy-lever-c-activation.md` present (per `mandatory-feature-docs`)
- [ ] CLAUDE.md architecture note added (per `canonical_docs_per_sprint`)
- [ ] CHANGELOG.md Unreleased entry added
- [ ] Sprint plan Section 12 (Documents Accessed) present
- [ ] PR comment with sprint summary added after CI green (per `must-comment-sprint-summary-on-pr`)

## 9. Documentation Update

### Files created
- `docs/development/activation-driven-discovery-001/sprint_plan.md` (this file)
- `docs/development/activation-driven-discovery-001/sprint_post.md` (post-mortem)
- `docs/features/jiminy-lever-c-activation.md` (feature doc)
- `internal/retrieval/expand_seeds.go` + `_test.go`
- `internal/jiminy/lever_c_activation_test.go`

### Files modified
- `internal/retrieval/service.go` — nothing (new primitive in separate file)
- `internal/jiminy/service.go` — Lever C integration + `activationEnrichLeverC`
- `internal/jiminy/types.go` — interface extension + ActivationSeed type
- `internal/jiminy/j7_j12_test.go` — mockRetriever new method
- `internal/api/rsic_adapters.go` — adapter method
- `internal/api/handlers_jiminy.go` — URL override parsing
- `internal/config/config.go` — 4 new fields
- `internal/cli/serve.go` — boot log line
- `CLAUDE.md` — arch note
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Activation dominates cosine, hides actionables with high sim but low graph centrality | Medium | High | Default weight=0.3 (cosine dominates 70/30); flag default-off; A/B measurable via URL override before default-flip |
| Edge fetch adds latency to Guide() hot path | Low | Medium | 1-hop only; ~500 seed IDs max (topK); fetchOutgoingEdges is a batched Cypher — measured <100ms typical |
| Activation scores are near-zero for isolated actionable nodes (no edges) → blended = 0.7*cosine, effectively down-weighting them | Medium | Low | Blend formula `(1-w)*cosine + w*activation` means zero activation = cosine * 0.7 uniformly; ordering within isolated set preserved. Well-connected items get a lift; isolated items don't get penalized differentially. |
| ComputeEdgeAttention returns near-zero for Jiminy queries (no code-query/arch-query context) | Medium | Medium | ComputeEdgeAttention has sensible defaults for CoActivated (highest weight); Jiminy queries hit the default path which gives CO_ACTIVATED_WITH a strong weight — verified in code walk |
| Interface addition breaks mockRetriever in tests | High | Low | Update mockRetriever in same commit; compile catches any misses |
| ⚠️ GUIDANCE_OUTCOME=0 discovered blocker unrelated to this sprint | Confirmed | N/A | Explicitly out-of-scope; disclosed as follow-up; does NOT block B1 shipping |

## 11. Rollback Procedures

- **Zero substrate mutation** — Lever C is pure read; no writes to TSDB or Neo4j.
- **Feature-flag rollback**: set `JIMINY_LEVER_C_ACTIVATION_ENABLED=false` in `.env` + kickstart → activation disabled, baseline restored.
- **Code rollback**: revert the sprint commit; no schema changes to reverse.
- **URL override**: per-request `?leverc_activation=false` bypasses the flag entirely.

## 12. Documents Accessed

- `internal/jiminy/service.go` (lines 1180-1300, 3360-3460 — Lever C call site + implementation)
- `internal/jiminy/types.go` (line 211-215 — RetrievalProvider interface)
- `internal/jiminy/j7_j12_test.go` (line 106 — mockRetriever)
- `internal/retrieval/activation.go` (lines 189-340, 371 — SpreadingActivationWithAttention + ComputeEdgeAttention)
- `internal/retrieval/column_graph.go` (lines 40-160 — reference pattern for seed → edges → spread)
- `internal/retrieval/service.go` (lines 1354 — Candidate; 1795 — Edge; 1392 — vectorRecall; 1968 — fetchOutgoingEdges)
- `internal/api/rsic_adapters.go` (lines 384-395 — jiminyRetrievalAdapter)
- `internal/config/config.go` (JiminyLeverC* fields for pattern match)
- `internal/retrieval/cache.go` + `cache_key_coverage_test.go` (CACHE-KEY-002 contract; NOT affected here since flag lives on Jiminy path)
- Live cypher-shell queries on mdemg-dev: CO_ACTIVATED_WITH count/weight distribution, activation_confidence population, GUIDANCE_OUTCOME row count
- `CLAUDE.md` (LEVER-C-TIGHTEN-001/002, JIMINY-ACTIONABILITY-001, HEBB-ETA-001, RRF-SCALE-001, CACHE-KEY-002, JIMINY-SUBSTRATE-NATIVE-001 arc README)

---
