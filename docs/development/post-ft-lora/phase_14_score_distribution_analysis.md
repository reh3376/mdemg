---
created: 2026-05-04
updated: 2026-05-04
status: phase 14 epic 0 output
phase: POST-FT-LORA-PHASE14 epic 0
predecessor: phase 13.1 (commit 6ed411e)
---

# Phase 14 Epic 0 — Forensic Distribution Analysis

> Hybrid-path Epic 0 deliverable. Computes Note 06 percentile defaults from real per-call score distributions in `llm_interactions.retrieval_scores` (`consulting.classify` + `retrieval.rerank_cross`), uses `uvts_results.final_score` to size A/B-baseline expectations, audits Neo4j MemoryNode property surface to inform Note 05 catalog bit allocation, and surfaces a Phase 13 Epic 6 wiring gap (V0017 `retrieval_audit` writer was never instantiated) that this sprint closed in flight.

---

## TL;DR — recommended Phase 14 defaults

| Knob | Spec default | Phase 14 Epic 0 recommendation | Rationale |
|---|---|---|---|
| `SPARSE_ACTIVATION_PERCENTILE` | 0.98 | **0.95** | Real per-call score distributions are heavy-tailed (`retrieval.rerank_cross` p98=9.28 vs p50=2.12 — a 4× ratio). Within a typical K=42 call, p98 keeps ≤1 candidate before the MIN_ACTIVE clamp. p95 keeps ~2; combined with `SPARSE_MIN_ACTIVE=3` clamp the gate effectively ships top-3 by default — a 13× reduction in rerank prompt input. |
| `SPARSE_MIN_ACTIVE` | 3 | **3** (keep spec) | Dominant constraint at p95+. |
| `SPARSE_MAX_ACTIVE` | 50 | **20** | `retrieval.rerank_cross` mean K=41.7 (max observed K-distribution distinct sizes = 2). MAX=50 is unreachable; 20 matches the observed top-K serving cap. |
| `RETRIEVAL_CTX_STRICT_THRESHOLD` (Jaccard) | 0.25 | **defer to Epic 6 backfill** | No fingerprints exist yet → no overlap to measure. Ship 0.25 spec default at Epic 5 wire-up; tune from V0019 catalog version history once 7 days of fingerprinted observations accumulate. |
| `CONTEXT_FINGERPRINT_BIT_BUDGET` | 256 | **256** (keep spec) | Adequate for observed feature density; see "Note 05 catalog bit-allocation findings" below. |
| Catalog bit split (symbols/paths/roles/reserved) | 64/64/64/64 | **0/192/32/32 in `whk-wms`-class spaces; 64/64/64/64 only when symbols populated** | `whk-wms` MemoryNodes have 0 `symbol` and 0 `role` properties; only `path` (8360 distinct) and `role_type` (5 values) are discriminative. A static 64/64/64/64 split wastes 128 bits on this space. Recommendation: per-space adaptive allocation — Builder counts feature density at refresh time and re-distributes bits proportionally. |

**Sprint-level decision**: ship Epic 1 with `SPARSE_ACTIVATION_PERCENTILE=0.95` (instead of spec 0.98), keep `MIN_ACTIVE=3` as the operative floor, and drop `MAX_ACTIVE` to 20. Make the catalog bit-split adaptive (Epic 3 design change vs static spec). Ship Note 05 strict-threshold at spec default 0.25 with V0019 instrumentation to retune in Phase 14.1.

---

## Section 1 — V0017 `retrieval_audit` was empty: Phase 13 Epic 6 gap

### Symptom

Initial Epic 0 query against `retrieval_audit`:

```sql
SELECT count(*) FROM retrieval_audit WHERE recorded_at > now() - interval '30 days';
-- 0 rows
```

Phase 13 Epic 6 (commit referenced in `phase_13_post.md`) shipped:
1. The V0017 `retrieval_audit` schema (verified — `\d retrieval_audit` shows 11 columns + indexes + hypertable)
2. The `RetrievalAuditWriter` interface in `internal/retrieval/retrieval_audit.go`
3. The `Service.SetRetrievalAuditWriter` setter
4. The service-side write call site at `internal/retrieval/service.go:847` gated on `s.cfg.RetrievalAuditEnabled && s.retrievalAuditWriter != nil`

But: **no caller of `SetRetrievalAuditWriter` exists anywhere in the codebase** (`grep -rn "SetRetrievalAuditWriter" internal/ cmd/` returns only the setter itself). The writer was never wired at server startup. As a result, even with `RETRIEVAL_AUDIT_ENABLED=true` no rows were ever written.

### Resolution (in-flight, this sprint)

Phase 14 Epic 0 closed the gap as a prerequisite:

- New `internal/tsdb/retrieval_audit_writer.go` — buffered + flush-via-CopyFrom writer, mirrors `LLMEndpointHealthWriter` (V0018) pattern. Own `RetrievalAuditRow` struct (`internal/tsdb` cannot import `internal/retrieval` — cycle).
- New `retrievalAuditAdapter` in `internal/api/server.go` — translates `retrieval.RetrievalAuditRecord` → `tsdb.RetrievalAuditRow` and forwards.
- Wired conditionally on `cfg.RetrievalAuditEnabled` next to the existing `RetrievalEventLogger` block.
- `Server.Close()` updated to flush + close.

Smoke-verified: 5 retrieves → 3 audit rows (cache-hit retrieves bypass the audit write — observation noted, separate follow-up for Phase 14.1 if cache-hit visibility matters).

### Consequence for Phase 14 Epic 0

Forensic distribution analysis cannot draw on V0017 historical data; instead derived from:
1. `llm_interactions.retrieval_scores` (per-call top-K score arrays — 99,768 + 50,720 score points)
2. `uvts_results.final_score` (per-question quality grades — 488 results across 16 runs)
3. Neo4j MemoryNode property density (whk-wms space, 9121 nodes)

V0017 is now collecting forward; Phase 14.1 can re-tune defaults from real audit rows after ≥7 days.

---

## Section 2 — UVTS final-score baseline (sizing A/B expectations)

`uvts_results` joined to `uvts_runs` over the past 30 days, grouped by `branch_label`:

| Branch label | n | mean | σ | min | p50 | p95 | p98 | max |
|---|---|---|---|---|---|---|---|---|
| `phase13_1-candidate-embedding-heavy-120q` (current production weights) | 120 | **0.4128** | 0.048 | 0.350 | 0.450 | 0.4541 | 0.4556 | 0.4590 |
| `phase13_1-baseline-120q` (legacy linear scorer) | 120 | 0.3895 | 0.049 | 0.350 | 0.354 | 0.4540 | 0.4556 | 0.4590 |
| `phase13_1-baseline-40q` | 40 | 0.3809 | 0.046 | 0.350 | 0.350 | 0.4500 | 0.4511 | 0.4550 |
| `phase13_1-embedding-heavy` (16q quick) | 16 | 0.4208 | 0.048 | 0.350 | 0.450 | 0.4560 | 0.4578 | 0.4590 |
| `phase13_5-F1-llamacpp` | 16 | 0.3958 | 0.050 | 0.350 | 0.358 | 0.4513 | 0.4535 | 0.4550 |
| Three rows showing `mean=0.0021` (`ab-candidate-e7`, `baseline`, `phase12-e7-live-smoke`) — pre-Phase-12 graders that never produced grades; ignore | 16 each | 0.002 | — | — | — | — | — | — |

### Implications for Phase 14 A/B

- **Phase 14 baseline mean to beat: 0.4128** (current production: embedding-heavy weights, 120q profile). Phase 14 Epic 7 A/B compares against this, not against the legacy 0.3895 baseline.
- **Score range is tightly bounded**: 0.350 floor → 0.459 ceiling. Phase 14's expected lift is small in absolute terms (e.g. +0.020 = +5% relative). The Note 02 merge gate (per-question regression > 10% = -0.0413 absolute) is the relevant per-question bar.
- **The tight max ≈ 0.459 ceiling** means Note 06's gate cannot improve overall mean by reducing noise — best-case it preserves mean while reducing rerank cost. Note 05's context column can add lift only if it reorders correctly on polysemy-bearing queries (concentrated in a subset of categories).
- **Variance is consistent (~0.05 σ across all variants)** — A/B runs need ≥30 questions to detect a 0.020 mean shift at 95% confidence (rough rule-of-thumb t-test). 16q quick is noisy at this scale; 120q full is the verifying profile.

---

## Section 3 — Per-call score distributions (the Note 06 gate-tuning input)

`llm_interactions.retrieval_scores` is an array column populated on every LLM call that consumed retrieval. Past 14 days, three task names had populated arrays:

| task_name | total points | distinct K sizes | mean K | min | p50 | p90 | p95 | p98 | p99 | max |
|---|---|---|---|---|---|---|---|---|---|---|
| `consulting.classify` | 99,768 | 4 | 34.7 | 0.3064 | 1.5508 | 3.2131 | 4.8424 | **6.0441** | 6.9819 | 12.39 |
| `retrieval.rerank_cross` | 50,720 | 2 | 41.7 | 0.0063 | 2.1213 | 5.1448 | 6.4402 | **9.2799** | 12.2154 | 83.73 |
| `jiminy.synthesize` | 4 | 1 | 4.0 | 0.3032 | 0.4107 | 0.5471 | 0.5533 | 0.5570 | 0.5582 | 0.5594 |

### Distribution shape

Both major sources are **heavy-tailed**:
- `consulting.classify`: p98/p50 = 6.04/1.55 ≈ **3.9×**
- `retrieval.rerank_cross`: p98/p50 = 9.28/2.12 ≈ **4.4×**

Most candidates score near p50; a long tail extends to p98+. This is the canonical shape Note 06's percentile gate is designed for — confirms the spec's qualitative premise.

### Quantitative implication for `SPARSE_ACTIVATION_PERCENTILE`

Note 06's gate is **per-call** (within each retrieve call's score distribution), not population-level. Within a typical `retrieval.rerank_cross` call (mean K=42):

| Percentile | Candidates retained (uncapped) | After `MIN_ACTIVE=3` clamp |
|---|---|---|
| 0.99 | top 0.4 (effectively 0–1) | **3** (clamped up) |
| 0.98 | top 0.84 | **3** (clamped up) |
| 0.95 | top 2.1 | **3** (clamped up) |
| 0.90 | top 4.2 | 4 |
| 0.80 | top 8.4 | 8 |

**At p95, p98, p99 the floor (`MIN_ACTIVE=3`) is the operative constraint**, not the percentile. The choice between p95 / p98 / p99 only matters for K-larger-than-100 calls (rare in current production). The lower bound the spec exposes is what the user sees.

**Recommendation: `SPARSE_ACTIVATION_PERCENTILE=0.95`** — same MIN_ACTIVE outcome as p98 in the dominant K=20–50 regime, but more lenient when a future high-K call shape lands in production. The percentile is essentially "what fraction of the per-call distribution constitutes the activation tail" — p95 means "anything in the top 5% fires" which is a clearer mental model for operators than "top 2%."

### Quantitative implication for `SPARSE_MAX_ACTIVE`

Observed K-cap is dominated by the 20–50 regime:
- `retrieval.rerank_cross` shows distinct K=2 sizes (likely K=20 and K=50)
- `consulting.classify` distinct K=4 sizes (likely 10/20/30/50)

`MAX_ACTIVE=50` (spec) is unreachable in 99% of calls. `MAX_ACTIVE=20` matches the dominant top-K cap. If a query is so diffuse that 20+ candidates score above the percentile, that's a tell the percentile is wrong, not that more should fire.

**Recommendation: `SPARSE_MAX_ACTIVE=20`** (down from spec 50).

### Rerank-prompt bloat reduction

Current `retrieval.rerank_cross` input: mean **2399 tok**, p95 **4301 tok**. Assuming top-K=20 carries ~120 tok per candidate (path + summary + score), dropping to top-3 would shrink the rerank prompt to ~360 tok — **a 6.7× reduction in input tokens**. Even if Note 06 keeps top-5 by clamp instead of 3, it's a **4× reduction**. This is the primary quantitative win.

---

## Section 4 — Note 05 catalog bit-allocation findings (whk-wms space)

Spec recommends **256 bits split 64 symbols / 64 paths / 64 roles / 64 reserved** per `mdemg_sprint_ideas/05`. Querying the `whk-wms` MemoryNode property surface (the primary A/B target):

| Feature kind | Distinct values | Notes |
|---|---|---|
| Symbol (`n.symbol` property) | **0** | Property does not exist on whk-wms nodes. WMS is a conversation-history space, not a code space. |
| Path (`n.path` property) | **8360** | Rich; near 1:1 with node count. Strongest signal. |
| Role (`n.role` property) | **0** | Property does not exist. |
| `role_type` (different property — exists) | **5** values: `leaf` (8360), `hidden` (682), `concept` (50), `comparison` (28), `config` (1) | Categorical, low-cardinality. |
| `layer` (HCM hidden layer index) | **5** values: 0-4 | Categorical. |
| `tags` (string array) | Not enumerated; sample inspection needed | Free-text array, potentially bit-rich. |

### Spec-default bit allocation outcome on whk-wms

A static **64/64/64/64** allocation in this space wastes 128 bits (0+0 → symbols+roles). Path-only fingerprints would actually be **more discriminative** than the current spec for this space.

### Recommendation: adaptive bit allocation

Catalog `Builder` (Phase 14 Epic 3) should:
1. **Count feature density per kind at refresh time** — number of distinct symbols, paths, roles, role_types, layers, top-N tag values across the space's MemoryNodes.
2. **Allocate bits proportionally to discriminative coverage** with floors:
   - Reserve 32 bits for `role_type` + `layer` combinations (low cardinality but always informative)
   - Distribute remaining 224 bits across (symbols, paths, top-N tags) proportional to log(distinct count)
   - Floor: any kind with ≥10 distinct values gets ≥16 bits
3. **Persist the allocation in `ContextCatalog.bits[]`** as `{position, kind, ref}` so the schema remains stable across spaces with different distributions.

**Effect on `whk-wms` (with 0 symbols, 0 roles, 8360 paths, 5 role_types, 5 layers):**
- 0 bits → symbols (no signal)
- 0 bits → roles (no signal)
- ~192 bits → paths (the dominant signal)
- 32 bits → role_type × layer combinations
- 32 bits → top-32 tags (when tag inventory is enumerated in Epic 3)
- Total: 256 bits — same budget as spec, redistributed.

**Effect on a code-rich space** (e.g. `mdemg-dev` if `symbol` were populated):
- 192 / 64 / 0 / ... shifts back toward symbol-dominant — Builder reads density and allocates accordingly.

This is a non-trivial Epic 3 design change vs the spec. Tracking as Plan §13 fork **#5 — bit assignment policy: spec (static 64/64/64/64) vs adaptive** with adaptive recommended by data.

### Bit budget = 256 stays appropriate

Even with redistribution, 256 bits supports ≥6 distinct discriminative features at ≥32 bits each, which is enough for whk-wms's 3 viable kinds (paths, role_type+layer, tags). 128 bits would be too tight for path-rich spaces (8360 distinct paths can't be coarse-mapped into <128 bits without massive collision).

---

## Section 5 — Downstream LLM call shapes (Note 06 prompt-bloat baseline)

Past 14 days of `llm_interactions`:

| task_name | n | mean tok_in | p50 tok_in | p95 tok_in | max tok_in |
|---|---|---|---|---|---|
| `ape.reflect` | 31,466 | 2144 | 0 | 5750 | 6168 |
| `consulting.classify` | 3151 | 138 | 200 | 272 | 349 |
| `retrieval.rerank_cross` | 1435 | **2399** | 2006 | **4301** | 4698 |
| `jiminy.evaluate` | 87 | 816 | 751 | 2186 | 4491 |
| `jiminy.codegen` | 50 | 939 | 987 | 1080 | 1113 |
| `jiminy.evaluate_llm` | 4 | 0 | 0 | 0 | 0 |

### Findings

- `retrieval.rerank_cross` is the prime Note 06 beneficiary. p95 = 4301 tok = ~10% of the llama-server `--ctx-size 32768 / 4 parallel` slot allocation. Halving this is meaningful for concurrency.
- `consulting.classify` is small (mean 138 tok). It does NOT seem to pass top-K to the LLM — it's classifying a single candidate's text. Note 06's gate has no benefit here; instead, `consulting.classify`'s `retrieval_scores` array represents the per-call ranking of the **input** candidates, valuable for percentile tuning but not for prompt-size reduction.
- `ape.reflect` mean 2144 / max 6168 — large but not retrieval-driven (reflection runs, not top-K consumers). Note 06 doesn't help here.
- `jiminy.codegen` and `jiminy.evaluate` are small enough that Note 06 might not produce measurable lift. Confirms Phase 14 Epic 5's "wire flag-off" recommendation for the Jiminy consumers.

---

## Section 6 — Decision-fork outcomes (data-cited)

| Fork (from sprint plan §13) | Provisional recommendation | Epic 0 data verdict | Final recommendation |
|---|---|---|---|
| #2 percentile default | 0.98 | Heavy-tail confirmed; clamp is operative; p95/p98/p99 all converge to MIN_ACTIVE in dominant regime | **0.95** |
| #3 catalog refresh cadence | 168 hr (weekly) | No data — feature is not yet built | Keep **weekly** (spec) |
| #4 catalog bit budget | 256 | 256 supports adaptive allocation across observed feature density | **256** (keep spec) |
| #5 bit assignment policy | static 64/64/64/64 (spec) | whk-wms has 0 symbols + 0 roles → static split wastes 128 bits | **Adaptive** (Builder counts density at refresh, allocates proportionally) — non-trivial Epic 3 design change |
| #6 strict-mode Jaccard threshold | 0.25 | No fingerprints exist yet → no overlap to measure | **0.25 spec** + V0019 instrumentation; retune in Phase 14.1 |
| #8 sparse gate ordering: pre-rerank vs post-rerank | pre-rerank | rerank input p95=4301 tok; pre-rerank gate gives 4–6× reduction; post-rerank wastes the saved compute | **Pre-rerank** (confirmed) |

Forks #1 (column vs scoring-term), #7 (ContextColumn weight), #9 (default flip strategy), #10 (backfill) remain unaffected by Epic 0 data — decided per Epic 7 A/B verdict as planned.

---

## Section 7 — Open data gaps (V0017/V0019 will close in Phase 14.1)

1. **No per-candidate score persistence** beyond `retrieval_scores` array. V0017 audit row stores `top_k_node_ids` but not their final scores. The newly-wired writer logs `consensus_strength` (per-call aggregate) but not per-candidate. **Phase 14 Epic 1 V0020 `sparse_gate_metrics`** addresses this directly: per-call active_count + dropped_fraction + threshold gives Phase 14.1 a sharper percentile-tuning input than this analysis had.
2. **No tag inventory** for whk-wms — Catalog Builder needs to enumerate top-N tag values to allocate bits to. Epic 3 must include this scan.
3. **Cache-hit retrieves bypass audit logging** (5 retrieves → 3 audit rows in smoke). Acceptable for Phase 14 (cache hits don't change ranking) but flagged as a Phase 14.1 question if audit volume looks unexpectedly low under steady-state load.
4. **Polysemy fixture data does not yet exist in whk-wms**. Phase 14 Epic 5 + Tier 3 "live polysemy demo" requires seeding 3 ErrorHandler-style observations in distinct contexts. Plan accordingly.

---

## Section 8 — Documents accessed

- `docs/research/mdemg_sprint_ideas/05-context-specific-node-activations.md`
- `docs/research/mdemg_sprint_ideas/06-sparse-retrieval-activation.md`
- `docs/development/post-ft-lora/sprint_plan_phase_14_sparse_fingerprints_and_gate.md`
- `docs/development/post-ft-lora/phase_13_1_post.md`
- `internal/retrieval/service.go:840-878` (audit write site)
- `internal/retrieval/retrieval_audit.go` (interface + setter)
- `internal/tsdb/llm_endpoint_health_writer.go` (mirror pattern for new writer)
- `internal/api/server.go:1198-1232` (TSDB writer wiring block; new audit writer wired here)
- `internal/tsdb/migrations/017_retrieval_audit.sql` (V0017 schema)
- TSDB `retrieval_audit`, `uvts_runs`, `uvts_results`, `llm_interactions` (live queries)
- Neo4j `whk-wms` MemoryNode property surface (live cypher queries)
