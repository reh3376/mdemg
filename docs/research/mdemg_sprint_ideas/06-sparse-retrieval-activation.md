# 06 — Sparse Retrieval Activation

**Sprint ID**: SPARSE-RETRIEVAL
**Date**: 2026-04-21 (plan authored)
**Branch**: TBD
**Scope**: Replace the current "score all candidates densely, rank, return top-K" retrieval with **sparse activation** — only the top N% (default 2%) of nodes "fire" for a given query; everything else is zeroed at the gate. This sharpens signal, enables cheap set-algebra across queries, and aligns MDEMG with HTM's canonical sparse-distributed-representation pattern.

**Upstream**: [White Paper Review](mdemg-white-paper-review.md) Paper 2 (HTM SDR principle).

---

## Sprint Framing

The HTM tradition argues that representations should be **sparse** — in the brain, ~2% of cortical neurons fire at any moment. Dense representations are noise-prone and don't compose well; sparse representations are cleaner, compose via set operations, and match how the cortex actually works.

MDEMG's retrieval currently scores *every* candidate node against the query, then ranks, then returns top-K. This is dense in the sense that every node gets a real-valued score. The column-voting work (Sprint 04) is a step toward sparser representations — each column has a hard cutoff at its top-K — but the final consensus is still a dense ranked list.

Proposed change: add a **gating threshold** after retrieval. Only nodes whose consensus score (or linear score, if column voting isn't live) exceeds the threshold are considered "active" for this query. Top-K becomes "top-K *that fired*" rather than "top-K regardless."

Three benefits:

1. **Noise rejection**: candidates that barely squeak into top-K today (score just above the K-th percentile) are often close to noise. Dropping them sharpens precision.
2. **Set algebra**: two queries' active sets can be unioned/intersected meaningfully — essential for multi-query workflows (e.g., "what did we see about X AND Y?").
3. **Downstream efficiency**: the re-ranker and LLM only see the active set, reducing compute and context-window load.

Small sprint. Low risk if gated properly. The main question is tuning the threshold — too strict and recall drops; too lax and the benefit disappears.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Sparse Gating Implementation | 0 | 3 | 1 | 0 | **4** |
| Downstream Consumer Updates | 0 | 2 | 1 | 0 | **3** |
| Tuning & A/B Infrastructure | 0 | 2 | 1 | 0 | **3** |
| Observability | 0 | 1 | 1 | 0 | **2** |
| Testing & Verification | 0 | 2 | 1 | 0 | **3** |
| Mandatory Documentation Phase | 0 | 4 | 2 | 0 | **6** |
| **Total** | **0** | **14** | **7** | **0** | **21** |

---

## Phase 1: Sparse Gating Implementation

### 1.1 Sparse activation gate (HIGH)

**Gap**: No component today implements a "fire / don't fire" cutoff.

**Fix** — New function in `internal/retrieval/gate.go`:

```go
package retrieval

// ApplySparseGate retains only the candidates whose score exceeds the
// activation threshold, computed as a percentile of the full candidate
// set. Unlike a fixed top-K, this produces a variable-sized active set:
// a diffuse query may activate few nodes; a sharp query activates many.
//
// cfg.ActivationPercentile: defaults to 0.98 (top 2% fire).
// cfg.MinActive, MaxActive: clamps the active set size to [min, max]
// so that a pathological query doesn't return 0 or 10000.
func ApplySparseGate(candidates []RankedCandidate, cfg SparseGateConfig) []RankedCandidate {
    if len(candidates) == 0 {
        return candidates
    }
    // Compute score threshold at the Nth percentile
    scores := extractScores(candidates)
    threshold := percentile(scores, cfg.ActivationPercentile)

    active := filterByThreshold(candidates, threshold)
    active = clampSize(active, cfg.MinActive, cfg.MaxActive)
    return active
}

type SparseGateConfig struct {
    ActivationPercentile float64 // 0.98 = top 2%
    MinActive            int     // never fewer than this
    MaxActive            int     // never more than this
}
```

**Files**: `internal/retrieval/gate.go` (new), `internal/retrieval/gate_test.go` (new)

---

### 1.2 Integration into retrieval pipeline (HIGH)

**Fix** — In `internal/api/handlers_retrieve.go`, after scoring (linear or column-voted) and before returning:

```go
if s.cfg.SparseRetrievalEnabled {
    ranked = retrieval.ApplySparseGate(ranked, sparseGateConfig(s.cfg))
}
```

Feature-flagged. Default false initially; flip to true after A/B benchmark.

**Files**: `internal/api/handlers_retrieve.go`

---

### 1.3 Env vars + config (HIGH)

```go
SparseRetrievalEnabled       bool    // SPARSE_RETRIEVAL_ENABLED (default false)
SparseActivationPercentile   float64 // SPARSE_ACTIVATION_PERCENTILE (default 0.98)
SparseMinActive              int     // SPARSE_MIN_ACTIVE (default 3)
SparseMaxActive              int     // SPARSE_MAX_ACTIVE (default 50)
```

**Files**: `internal/config/config.go`, `.env.example`, compose templates

---

### 1.4 Per-query override (MEDIUM)

**Fix** — Allow `?sparse=false` or `?sparse_percentile=0.95` query parameter to override defaults per call.

**Files**: `internal/api/handlers_retrieve.go`

---

## Phase 2: Downstream Consumer Updates

### 2.1 Re-ranker sees active set only (HIGH)

**Gap**: If sparse gating reduces the candidate set from 50 to 8, the re-ranker should work on the 8 — not pull back to 50.

**Fix** — Pass the gated set to the re-ranker. No code change strictly required (the re-ranker already takes whatever it's given), but verify no pre-rerank expansion happens.

**Files**: verify in `internal/rerank/` or wherever rerank input is built

---

### 2.2 LLM context window reduction (HIGH)

**Gap**: Consulting service / Jiminy Guide often pack top-K into context. With sparse gating, the "K" is effectively smaller. Downstream code should scale accordingly.

**Fix** — Change fixed top-K constants to "top of active set" in:
- `internal/consulting/service.go`
- `internal/jiminy/service.go` (guide phase)

**Files**: both above

---

### 2.3 Explanation surface "below threshold" (MEDIUM)

**Fix** — When a candidate was computed but gated out, include it in a separate "below_threshold" field of the response (only in debug mode). Aids tuning.

**Files**: `internal/api/handlers_retrieve.go`

---

## Phase 3: Tuning & A/B Infrastructure

### 3.1 A/B benchmark harness (HIGH)

**Fix** — Run whk-wms with percentile ∈ {0.95, 0.98, 0.99} and baseline (gating off). Measure mean score, high-score rate, and *recall* (did the "correct" answer fall into the active set?).

**Success criteria**: at 0.98 default, mean score ≥ baseline, recall ≥ 95% of baseline.

**Files**: `scripts/bench/sparse-retrieval-ab.sh`, `docs/tests/benchmarks/sparse-retrieval-ab.md`

---

### 3.2 Percentile sweep analysis (HIGH)

**Fix** — Plot mean score and recall vs percentile. Identify the knee. Document recommended percentile per query type (if the knee varies).

**Files**: `docs/tests/benchmarks/sparse-retrieval-sweep.md`

---

### 3.3 Per-space adaptive percentile (MEDIUM)

**Fix** (optional extension) — If specific spaces benefit from different percentiles, expose `RSIC_SPARSE_PERCENTILE_<space_id>` overrides. Simple key-value lookup at query time.

**Files**: `internal/config/config.go`

---

## Phase 4: Observability

### 4.1 Prometheus metrics (HIGH)

```
mdemg_sparse_gate_active_count{space_id} - histogram (active set size per query)
mdemg_sparse_gate_dropped_fraction{space_id} - histogram (what fraction of candidates dropped)
mdemg_sparse_gate_threshold{space_id} - histogram (actual threshold score per query)
```

**Files**: `internal/metrics/registry.go`

### 4.2 Grafana + CLI (MEDIUM)

- New panel on `mdemg-overview.json`: "Sparse Retrieval"
- `mdemg debug sparse <query>` — shows full candidate list with above/below threshold markings

**Files**: `deploy/grafana/dashboards/mdemg-overview.json`, `internal/cli/debug.go`

---

## Phase 5: Testing & Verification

### 5.1 Unit tests (HIGH)
- Percentile calculation correctness
- Min/max clamping
- Empty-input handling

### 5.2 Integration test (HIGH)
Query that naturally has one strong hit and many weak ones → verify active set is small (1-5). Query that has many moderately-relevant hits → verify active set is larger (10-20).

### 5.3 A/B benchmark (HIGH)
See Phase 3.1. Merge blocked until benchmark passes.

---

## Phase 6: Mandatory Documentation Phase

### 6.1 CHANGELOG.md (HIGH)
### 6.2 AGENT_HANDOFF.md (HIGH)
### 6.3 CLAUDE.md — add sparse retrieval vocabulary (MEDIUM)
### 6.4 `docs/architecture/06_Retrieval_API_and_Scoring.md` — add sparse gating section (HIGH)
### 6.5 `docs/user/cli-reference.md` — 4 new env vars, new query param (HIGH)
### 6.6 Homebrew beta testing guide + submodule bump (MEDIUM)

---

## Risk Analysis & Rollback

### R1: Recall drops below acceptable level

**Likelihood**: Medium. Tight thresholds drop recall.

**Mitigation**: Phase 3.1 benchmark is the gate. If recall <95%, do not flip default.

**Rollback**: `SPARSE_RETRIEVAL_ENABLED=false`. Instant.

### R2: Specific query patterns produce empty active sets

**Likelihood**: Low. `MIN_ACTIVE` floor of 3 prevents this.

**Mitigation**: Per-query override available.

**Rollback**: As R1.

### R3: Interaction with column voting (if landed) is unexpected

**Likelihood**: Low-Medium. Consensus scores have a different distribution than linear scores; percentile tuning may need to differ.

**Mitigation**: A/B benchmark is run with current production mode (whatever the retrieval pipeline is at the time).

**Rollback**: As R1.

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. Sparse Gating | 1 day |
| 2. Downstream Updates | 1 day |
| 3. Tuning & A/B | 1 day |
| 4. Observability | 0.5 day |
| 5. Testing & Verification | 1.5 days |
| 6. Mandatory Documentation | 0.5 day |
| **Total** | **~5.5 days (1 sprint, small)** |

---

## Dependencies

**Blocks**: None.

**Blocked by**: None technically, but ideally **runs after 04-column-voting-retrieval** so gating operates on consensus scores (more interpretable) rather than linear scores.

**Touches but does not block**: 05 (fingerprint-based filtering is orthogonal to score gating; they compose).

---

## Documents Accessed

- `internal/api/handlers_retrieve.go`
- White paper review Paper 2 (HTM SDR)
