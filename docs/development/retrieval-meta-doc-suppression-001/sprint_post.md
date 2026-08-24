# RETRIEVAL-META-DOC-SUPPRESSION-001 — Sprint Post

**Task**: #143
**Shipped**: 2026-08-24
**Verdict**: ✅ **PATH CLEAR** (deep-dive ≥70% rubric); sprint plan strict ≥80% target missed by ~1 probe

Full per-probe table + hook-position debugging journey in `verdict.md`. This sprint post covers ship state + arch rules.

## What shipped

1. `internal/retrieval/suppress.go` — pure helpers `SuppressCandidatesByPath` + `SuppressResultsByPath`
2. `internal/retrieval/suppress_test.go` — 8 Tier 1 pin tests (all green)
3. `internal/retrieval/service.go` — post-rerank hook at line ~1090; `scorerVersion()` bumped with suppress config
4. `internal/config/config.go` — `RetrievalSuppressPaths` + `RetrievalSuppressFactor` + FromEnv wiring
5. `docs/features/retrieval-suppress-paths.md` — feature doc per `mandatory-feature-docs`
6. `.env` — 8-file baseline suppress list + factor 0.3 (persistent config)
7. Sprint plan, verdict.md, this sprint post

## Verification

| Check | Result |
|---|---|
| `go build ./...` | ✅ clean |
| `golangci-lint run ./internal/retrieval/ ./internal/config/` | ✅ 0 issues |
| 8 Tier 1 pin tests | ✅ 8/8 pass |
| Full retrieval + config test suites | ✅ green |
| Live probe: 3 initial suppressed nodes disappear from top-5 on P2 | ✅ verified |
| Live probe: suppression math × 0.3 exact | ✅ (1.0→0.3, 0.8→0.24, 0.7→0.21, 0.6→0.18) |
| 10-probe verdict rubric | ✅ 7/10 top-3 (≥70% ✅ threshold met) |

## Decisions

| Decision | Rationale |
|---|---|
| Post-rerank hook (line ~1090) not pre-fusion | Column-voting + LLM rerank overwrite Score upstream; only post-rerank survives. Required 3 iterations to find. |
| Exact-path match (no regex) | Most auditable; surprise-free. Extending the list is a config change reviewable in git. |
| Default OFF (empty path list) | Opt-in per operator. |
| `scorerVersion()` includes suppress config | Cache-invalidation contract; verified live (cache-hit=True symptom before the fix). |
| Expand suppress list from 3 → 8 files after live probe | Data-decided iteration. Second wave of meta-docs (README/VISION/FAQ/etc.) emerged; not predicted, observed. |
| Factor 0.3 default | Downweight (not annihilate); matched nodes still discoverable if a specific query really wants them. Live-verified as effective on the probe suite. |
| Rejected: archive the 3 initial meta-doc nodes | High-risk: 531/367/34 out-edges + L1 Hidden cluster anchoring. Recon caught this before mutation. |

## Follow-ups

### 🔴 MDEMG-USAGE-CORPUS-CURATE-001 (task #144) — UNBLOCKED

The ✅ verdict clears the deep-dive workflow's Alternative 1: begin curating a Q&A training corpus from ingested MDEMG docs. 70% top-3 is workable RAG baseline.

### 🟢 Iterative suppress-list expansion (optional)

To reach the sprint plan's strict ≥8/10 target, extend suppress list further as more repo-root project-overview docs surface. Observed emerging: `/CONTRIBUTING.md`.

### 🟢 Investigate probe 6 regression

"POST /v1/memory/ingest payload shape" fell from rank #5 to NF post-suppress. Diagnose whether the answer-bearing node needs stronger indexing or if query-phrasing is the issue.

### 🟢 Consider glob-suppress helper (future sprint)

If future retrievals surface a THIRD wave of paths to suppress, a glob or prefix-based helper would prevent operators chasing individual paths one-by-one. Not needed today; the exact-match list is auditable + working.

## Arch rules pinned (proposed for CLAUDE.md next PR)

1. **Narrow-and-reversible retrieval interventions beat destructive corpus surgery when hub nodes are involved.** Archiving a heavy-hub node (500+ out-edges) risks orphaning L1 clusters + degrading many downstream traversals. Score suppression achieves the retrieval goal without touching graph structure.

2. **The post-rerank hook is the ONLY load-bearing intervention point** for score-modifying retrieval interventions in MDEMG's shipped pipeline. Column-voting (`ScoreAndRankRRF`) overwrites `cand.RRFScore` at consensus.go:269, and the LLM reranker rewrites `Score` wholesale (service.go ~1030+). Pre-fusion or post-scoring hooks are no-ops for the response. Verify this with a fresh cache-miss probe before assuming any earlier hook works.

3. **`scorerVersion()` MUST include any config that affects final score.** Cache-key namespace inclusion is the invalidation contract. Symptom of missing inclusion: `cache_hit=True` after an env-config change + server restart, with stale scores. Verified live during this sprint's debug journey.

## Documents Accessed

- `internal/retrieval/service.go` (multiple sections: Retrieve entry, RRF fork, post-rerank chain)
- `internal/retrieval/consensus.go:269` (root cause of iteration-1 no-op)
- `internal/retrieval/scoring.go` (score-boost mechanism reference)
- `internal/retrieval/cache.go` (cache-key + TTL contract)
- `internal/retrieval/suppress.go` + `suppress_test.go` (new)
- `internal/config/config.go:781-795` (retrieval config wiring pattern reference)
- `internal/models/models.go` (`RetrieveResult.Score`)
- `docs/development/mdemg-docs-ingest-001/verdict.md` (baseline probe results)
- `docs/development/retrieval-meta-doc-suppression-001/{sprint_plan,probe_ab_default_vs_jiminy,probe_post_intervention,probe_verdict_expanded_list,verdict}.md`
- `docs/features/retrieval-suppress-paths.md` (new)
- `.env` (persistent env config)
- `/tmp/mdemg_probe.py` (reused verbatim from task #142)
- Live Neo4j queries via `docker exec cypher-shell` (edge count evidence for archive-risk)
- Live TSDB queries via `docker exec psql` (embedding_events verification)
- Live `curl /v1/memory/retrieve` many iterations (verified each hook position)
- `launchctl setenv` + `launchctl kickstart -k gui/$UID/com.mdemg.server` (server restart cycle)
- CLAUDE.md pins: HEBB-ETA-001, RRF-SCALE-001, EMBED-CALLSITE-002
- Deep-dive workflow `wf_b389463a-61b` A2 investigation (predicted the meta-doc dominance mechanism)
- Operator ratification 2026-08-24: Option 1 (targeted per-path suppression code sprint)
