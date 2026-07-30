# RETRIEVAL-DIVERSITY-001 — Sprint Plan

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent trigger:** RETRIEVAL-QUALITY-AUDIT-001 recommendation #3
(cluster D: near-duplicate results waste ~11% of top-5 slots).

## 1. Header & Metadata

Add a **post-rerank diversity pass** that drops near-duplicate results
(same result-name appearing multiple times in top-K) with a
fill-from-skipped fallback so the caller never gets fewer results than
requested. Simple + targeted; ~1d effort.

## 2. Problem Statement

RQA-001 catalogued the near-duplicate pattern on q04:
```
[1] name=pre-bash-check   score=0.5053
[2] name=pre-bash-check   score=0.5050  ← duplicate name, near-identical score
[3] name=sql              score=0.0154
[4] name=sql              score=0.0148  ← duplicate name
```
and q14 (5 duplicate/near-duplicate L3/L4 emergent-concepts for
"CUIDv2 rules"). Result: **11% of top-5 slots surfaced redundant
content** instead of diverse coverage.

Root cause: the reranker doesn't apply any diversity penalty.

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- `internal/retrieval/diversity.go` — new `ApplyDiversityFilter`
  function that drops-if-name-already-kept, with a
  fill-from-skipped fallback (never shorter than requested topK)
- Config: `RETRIEVAL_DIVERSITY_ENABLED` (default false) +
  `RETRIEVAL_DIVERSITY_MAX_PER_NAME` (default 1 = strict dedup)
- Wire into `internal/retrieval/service.go` right BEFORE the topK
  truncation
- Unit tests: exact-name dedup, empty-name passthrough, over-topK
  fallback, disabled passthrough
- Live smoke: re-run RQA-001's q04 and q14 → verify diversity
- Docs + flag flip after smoke

**Out of scope:**

- Embedding-cosine diversity (would require per-result embeddings —
  most results already carry `vector_sim` which is query-cosine, not
  result-vs-result cosine; deferred as premature)
- Diversity across `role_type` or `path` (name-match handles the
  observed cases; broader can come later if new patterns surface)
- Any change to scoring / reranking upstream — this is pure
  post-rerank filtering

## 4. Dependencies

- Shipped `models.RetrieveResult` with `Name` field
- Shipped `req.TopK` in service.go — the sprint slots in right
  before the existing `if len(results) > topK { results[:topK] }`

## 5. Implementation Plan (single sprint, 3 phases)

**Phase 1 — Code + wire**
- New file `internal/retrieval/diversity.go` with `DiversityCfg`
  struct + `ApplyDiversityFilter(results, topK, cfg)` pure function
- Config fields + `FromEnv()` reads
- Wire in `service.go` right before the topK truncation, gated on
  `cfg.RetrievalDiversityEnabled`
- **Gate**: `go build ./...` clean

**Phase 2 — Unit tests**
- `TestApplyDiversityFilter_ExactNameDedup` — 5 results with names
  `[A, B, B, C, D]` and MaxPerName=1 → returns `[A, B, C, D]` when
  topK=5; the second B is dropped, no filler needed
- `TestApplyDiversityFilter_FillFromSkipped` — 5 results all named
  `A` (worst case) with topK=3 → returns 3 results (fills from
  skipped since strict dedup would leave 1); never fewer than
  requested
- `TestApplyDiversityFilter_DisabledPassthrough` — enabled=false →
  input unchanged
- `TestApplyDiversityFilter_EmptyName` — results with empty Name
  bypass the dedup logic (treated as always-diverse)
- `TestApplyDiversityFilter_MaxPerName_2` — MaxPerName=2 keeps 2 of
  the same name before starting to drop

**Phase 3 — Live smoke + flag flip + docs**
- Re-run RQA-001's q04 (pre-bash-check query) + q14 (CUIDv2 query)
  with flag OFF vs ON
- Verify: q04 flag-ON keeps only 1 pre-bash-check + 1 sql (fills
  slots 3-5 from skipped diverse candidates); q14 shows diverse
  results
- Flip `RETRIEVAL_DIVERSITY_ENABLED=true` in `.env` after smoke
- Docs (short — CLAUDE.md pin + CHANGELOG)

## 6. Testing Plan (3 tiers)

- **Tier 1**: 5 unit tests (Phase 2)
- **Tier 2**: `go build`, `golangci-lint 0 issues`, full `go test ./...`
- **Tier 3**: live re-run of q04/q14; verify diversity + no
  caller-shortening

## 7. Commit Strategy

Single commit under `RETRIEVAL-DIVERSITY-001`.

## 8. Verification Checklist

- [ ] `ApplyDiversityFilter` function + `DiversityCfg` struct
- [ ] Config knobs (default-off, MaxPerName=1)
- [ ] Service wire right BEFORE topK truncation
- [ ] All 5 unit tests green
- [ ] Fill-from-skipped fallback verified (never shorter than topK)
- [ ] `go build`, `golangci-lint`, `go test ./...` full green
- [ ] Live smoke on q04 + q14; flag flipped in `.env`
- [ ] CHANGELOG + CLAUDE.md pin

## 9. Rollback

- Revert commit — code default off, flag flip reversible via `.env`
- No schema change, no substrate mutation

## 10. Risks & Mitigations

- **Risk**: dropping same-name results loses genuinely different
  content that happens to share a name (e.g. two different sprint
  post.md files).
  - **Mitigation**: MaxPerName is config-tunable; can be raised to 2
    if the flag causes visible regressions. Also, most sprint post
    files are in different `path`s and the fallback fill from
    skipped still surfaces them if needed.
- **Risk**: reduces caller output count if MaxPerName is set too low.
  - **Mitigation**: fill-from-skipped guarantees never-shorter-than-
    topK. Pin-tested.
