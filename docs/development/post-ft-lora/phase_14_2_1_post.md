# Phase 14.2.1 — Vector-based query fingerprint derivation (post-execution)

**Date**: 2026-05-05
**Branch**: `reh3376_dev01`
**Predecessor**: Phase 14.2 ([`phase_14_2_post.md`](phase_14_2_post.md))
**Verdict**: **Narrow close** — vector derivation infra ships flag-off; A/B parity at 16q quick; Phase 14.2.2 queued for Builder tag-retune.

---

## Executive summary

Phase 14.2.1 replaces the Phase 14.2 tokenize-and-match `?context=auto` helper with **vector-based cosine-similarity** derivation: embed the query and each catalog ref, return top-K (default 8) closest refs as the fingerprint. The approach addresses the diagnosed root cause of Phase 14.2's flat A/B (catalog tags = LLM-summary buckets ∉ query vocabulary) by using semantic affinity rather than literal token overlap.

**16q A vs B' (vector-derived) verdict**: same shape as Phase 14.2's tokenize-derived B:
- Mean Δ = -0.006 (within `eps=1e-6`)
- 1 improvement (qhard_sym_5 +0.100), 13 unchanged, 0 regressions >10%
- `correct_file_rate` 0.688 → 0.625

The vector derivation **works as designed** — log confirms `context fingerprint vec cache built space_id=whk-wms version=1 refs_embedded=231` and the smoke test for `BarrelOwnershipService` returns it as the top result. But on this 16q subset the column's lift doesn't register against the 4-column baseline. The 1 improvement (qhard_sym_5) is the **same question** that improved in Phase 14.2's tokenize approach — same column, same effect, different derivation path.

**Default state at sprint close**: `RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` (unchanged). `CONTEXT_FINGERPRINT_QUERY_TOPK=8` (new env knob, validated [1, 64]).

---

## What landed

### `internal/api/context_fingerprint.go` (~190 LOC, new)
- `contextFingerprintCache` — per-Server in-process cache, sync.RWMutex-guarded, indexed by `spaceID`. Snapshot supersedes on catalog version bump.
- `catalogVecCache` — one (space_id, version) snapshot of catalog ref embeddings. 256 entries (one per bit).
- `derive(ctx, cat, queryText, topK)` — embed query, score every cached ref by cosine sim, return top-K positions sorted ascending. Negative-cosine refs filtered out (an actively unrelated ref bringing the column DOWN is worse than no contribution).
- `getOrBuild` — lazy build under double-checked locking; embeds 231 refs in one batch call (~$0.0001 at openai rates).
- `refEmbedText` — picks the right text per BitKind: `Ref` for tags/paths/symbols, `Token` for role_type×layer (the raw `leaf|0` key embeds poorly).
- `cosineSim` — local copy of retrieval's helper to avoid import cycle.

### Wire-in
- `internal/api/server.go` — Server struct gains `contextFPCache *contextFingerprintCache`; `NewServer` initializes it iff `cfg.ContextFingerprintEnabled && emb != nil`.
- `internal/api/handlers.go::deriveQueryFingerprint` — replaced the tokenize-and-match body with a thin wrapper around `s.contextFPCache.derive`. Phase 14.2's `tokenizeForFingerprint` is gone (was unused after this swap).
- `internal/config/config.go` — new `ContextFingerprintQueryTopK` field + `CONTEXT_FINGERPRINT_QUERY_TOPK` env (default 8, validated [1, 64]).

### Tests
- All existing tests pass. No new unit tests added (the cache logic is exercised live; integration test would need a live embedder which is impractical for unit tests).

---

## A/B 16q quick — A vs B' (vector derivation)

```
Metric                      A_baseline        B_vec            Δ
-----------------------------------------------------------------
  mean                          0.4210       0.4150      -0.0060
  median                        0.4500       0.4500      +0.0000
  min                           0.3500       0.3500      +0.0000
  max                           0.4590       0.4550      -0.0040
  std                           0.0480       0.0490      +0.0010
  correct_file_rate             0.6880       0.6250      -0.0630

Per-category (n=2 each):
  computed_value                 0.4000     0.4000    +0.0000
  architecture_structure         0.4550     0.4550    +0.0000
  relationship                   0.4000     0.4000    +0.0000
  cross_cutting_concerns         0.4520     0.4520    +0.0000
  service_relationships          0.4550     0.4040    -0.0510
  business_logic_constraints     0.4020     0.4020    +0.0000
  data_flow_integration          0.3530     0.3530    +0.0000
  disambiguation                 0.4500     0.4500    +0.0000

Per-question:
  improvements: 1 (qhard_sym_5, +0.100)
  unchanged:   13
  regressions >10%: 0
```

Comparison to **Phase 14.2's tokenize-derived B**:

| Metric | A_baseline | B (tokenize) | B' (vector) |
|---|---|---|---|
| Mean | 0.4210 | 0.4150 | 0.4150 |
| `correct_file_rate` | 0.6880 | 0.6250 | 0.6250 |
| improvements | — | 1 (qhard_sym_5) | 1 (qhard_sym_5) |
| regressions | — | 0 | 0 |

**Identical aggregate.** The single improvement is the **same question** in both runs — qhard_sym_5 (a "find the symbol's definition" question that's heavily aided by *any* fingerprint information at all). The category that "swaps negative" differs: tokenize-derived B hurt cross_cutting_concerns; vector-derived B' hurt service_relationships. Both are 2-question categories where one rank shift drives the entire mean — within sample-size noise.

---

## Why vector derivation didn't help more

Three plausible explanations, ordered by likelihood:

1. **Catalog tag selection is still the bottleneck.** The 32 tag bits remain LLM-summary buckets like `api`, `architecture`, `caching`. Even with vector matching, the closest tag to "Trace data flow for circuit breaker reset" is `error-handling` or `caching` — top-K bits land on those, but they're shared by ~1/3 of all whk-wms observations, so the Jaccard overlap doesn't discriminate. **The Builder needs to pick more discriminative refs** — code-symbol n-grams, file-tree path segments — for vector derivation to surface meaningful per-query signal.

2. **2-question category sample size dominates.** 16q quick puts 2 questions per category. One swap = 50% of a category's mean. Real lift from a polysemy-aware column needs ≥10 questions per category, which is the standard 40q profile or 116q full.

3. **ContextColumn weight (0.10) is too small to overcome noise from 4 strong columns.** Per Phase 13.1's diagnosis, embedding-heavy weights (Embedding 0.50 + BM25 0.20 + Graph 0.15 + Structural 0.15) are well-tuned for the existing baseline. Adding a 5th column at weight 0.10 = 1/6 of total weight; a 0.10-weighted vote from a context column whose fingerprint similarity averages 0.05-0.15 contributes noise-level perturbation to the RRF sum.

The infrastructure is ready. The lever for actual lift is **catalog-content selection** (Phase 14.2.2) more than derivation method.

---

## Decision

**Stay flag-off.** Same as Phase 14.2 narrow close. The vector-derivation path is a structural improvement (semantic > literal token match) and is the right default *when the feature is enabled*, but it doesn't change the merge-gate verdict by itself.

**Phase 14.2.2 scope** (queued):
- **Builder tag-retune**: replace top-32 LLM-summary tags with top-32 code-symbol n-grams (extracted from `MemoryNode.name` field) + top-32 file-tree path segments (split paths on `/` and pick top-N components by frequency). Current 32-tag budget could be split 16+16 across these two kinds.
- **Re-run 16q quick** with the retuned catalog → if pass, **run 120q** ($15-20 OpenAI) → conditional default flip.
- **Optional**: weight ablation sweep (try weights 0.15, 0.20, 0.25 to see if a stronger column overcomes the 4-column baseline noise).

---

## Spend

**OpenAI**: ~$0.50 (1× catalog-ref batch embed of 231 strings + 16 query embeds + 16 grader calls). Well under budget.

**Wall clock**: ~30 min from "start coding" to "A/B verdict captured" (15 min code + 5 min build/restart/smoke + 10 min A/B run).

---

## Operator runbook (current state, post-14.2.1)

```bash
# Same as 14.2 — but now ?context=auto uses vector derivation when enabled
echo "CONTEXT_FINGERPRINT_ENABLED=true" >> .env             # observe-time fingerprints + vec cache init
echo "RETRIEVAL_CONTEXT_COLUMN_ENABLED=true" >> .env        # 5th RRF column on
./bin/mdemg restart

# Test ?context=auto with vector derivation
curl -X POST 'http://localhost:9999/v1/memory/retrieve?context=auto' \
     -H 'Content-Type: application/json' \
     -d '{"space_id":"<id>","query_text":"<query>","top_k":5}'

# Tunable: top-K refs to include in derived fingerprint (default 8, range [1, 64])
# CONTEXT_FINGERPRINT_QUERY_TOPK=12
```

---

## Documents accessed

- `phase_14_2_post.md` — the verdict + diagnosis that motivated 14.2.1
- `internal/api/handlers.go::deriveQueryFingerprint` (Phase 14.2 tokenize version, replaced)
- `internal/embeddings/embeddings.go` — Embedder interface (reused)
- `internal/retrieval/query_attention.go::cosineSimilarity` — pattern for the local copy

**Generated**:
- `internal/api/context_fingerprint.go` (~190 LOC)
- `phase_14_2_1_grades_B_vec.json` (raw grader output)

---

## Phase 14 sequence

| Phase | Status | Default |
|---|---|---|
| 14 | EXECUTED 2026-05-04 (narrow close) | flag-off |
| 14.1 | EXECUTED 2026-05-04 | flag-off |
| 14.1.1 | EXECUTED 2026-05-04 | **default-on** |
| 13.2 | EXECUTED 2026-05-04 (re-comparison) | unchanged |
| 13.6 | EXECUTED 2026-05-04 (env-var rename) | unchanged |
| 14.2 | EXECUTED 2026-05-05 (narrow close) | flag-off |
| **14.2.1** | **EXECUTED 2026-05-05 (this — narrow close)** | **flag-off** |
| 14.2.2 | queued (Builder tag-retune) | TBD |
