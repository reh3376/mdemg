# 04 — Column-Voting Retrieval

**Sprint ID**: RETRIEVAL-COLUMN-VOTING
**Date**: 2026-04-21 (plan authored)
**Branch**: TBD (independent of PC track — can run in parallel with 02/03 on the other dev branch)
**Scope**: Replace the current weighted-linear-combination retrieval ranking with an **ensemble of independent retrieval "columns"**, each operating on a different view of the data, whose outputs are combined by **consensus voting** rather than linear interpolation. A node appearing in K-of-N columns with reasonable scores outranks a node with one spectacular score — this yields (a) better ranking robustness, (b) a natural uncertainty metric (consensus strength), and (c) a path to per-query explainability.

**Upstream**: [White Paper Review](mdemg-white-paper-review.md) Paper 1 (Hawkins et al. 2017 — columns vote to reach consensus).

---

## Sprint Framing

The current retrieval pipeline in MDEMG combines multiple scoring signals via a weighted linear sum:

```
score = α·embedding_sim + β·bm25 + γ·graph_proximity + δ·recency + ...
```

(α, β, γ are the hyperparameters tuned in DH-004 — α=0.60, β=0.20, γ=0.15 per `config.go:104-112`.) This is effective (the 58.4% improvement came partly from tuning these), but it has two fundamental limitations:

1. **A single column that "wins big" dominates the rank.** If embedding similarity returns one spectacular hit, it can outrank nodes that are moderately good on multiple signals.
2. **No uncertainty metric.** The combined score gives no sense of "how confident are we?"

The column-voting architecture from the Hawkins paper addresses both. Each column independently produces a ranked list. The final rank is determined by **consensus** — how many columns include the node in their top-K, weighted by the rank within each column. A node with rank-5 in all 5 columns beats a node with rank-1 in one column and rank-300 in the others.

This is not a replacement for the current scoring — it's a reorganization. The existing signals become the first few columns. New columns are added for views the system doesn't currently use.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Column Interface & Refactor | 1 | 3 | 0 | 0 | **4** |
| New Columns (structural, temporal, role-scoped) | 0 | 4 | 1 | 0 | **5** |
| Consensus Ranking Algorithm | 0 | 3 | 1 | 0 | **4** |
| Confidence Emission | 0 | 2 | 1 | 0 | **3** |
| A/B Benchmark Infrastructure | 0 | 2 | 1 | 0 | **3** |
| Observability | 0 | 1 | 2 | 0 | **3** |
| Testing & Verification | 0 | 3 | 1 | 0 | **4** |
| Mandatory Documentation Phase | 0 | 5 | 2 | 0 | **7** |
| **Total** | **1** | **23** | **9** | **0** | **33** |

---

## Phase 1: Column Interface & Refactor

**Goal**: Define the `RetrievalColumn` interface. Refactor the existing weighted scoring so that embedding, BM25, and graph-proximity become three columns. No behavior change yet — just restructure.

### 1.1 Define `RetrievalColumn` interface (CRITICAL)

**Gap**: The current `internal/api/handlers_retrieve.go` has an inline scoring pipeline. No abstraction for "a retrieval view."

**Fix** — New file `internal/retrieval/column.go`:

```go
package retrieval

import "context"

// Column is a single retrieval view that independently produces a ranked
// list of candidates for a query. Columns are intentionally independent —
// they do not share intermediate state — so that consensus voting produces
// a meaningful uncertainty signal.
type Column interface {
    // Name returns a stable identifier (e.g. "embedding", "bm25", "structural")
    Name() string

    // Retrieve produces up to topK ranked candidates for the query.
    // Implementations must return nodes with scores on their own scale;
    // consensus ranking normalizes across columns.
    Retrieve(ctx context.Context, query Query, topK int) (ColumnResult, error)

    // Weight returns this column's vote weight [0,1]. Used by consensus
    // ranking to emphasize high-quality columns (e.g. embedding > BM25).
    Weight() float64
}

// ColumnResult is the ranked output of one column, plus diagnostic info.
type ColumnResult struct {
    ColumnName string
    Candidates []RankedCandidate
    Latency    time.Duration
    Error      error // non-fatal errors; column produces empty result
}

type RankedCandidate struct {
    NodeID string
    Rank   int     // 1-indexed within this column
    Score  float64 // column-native score
}
```

**Files**: `internal/retrieval/column.go` (new), `internal/retrieval/types.go` (new for shared types)

---

### 1.2 Wrap existing embedding retrieval as `EmbeddingColumn` (HIGH)

**Fix** — Move embedding-scoring code to `internal/retrieval/column_embedding.go`. No logic change, just a new interface wrapper.

**Files**: `internal/retrieval/column_embedding.go` (new), `internal/api/handlers_retrieve.go` (delegate)

---

### 1.3 Wrap BM25 as `BM25Column` (HIGH)

**Fix** — Similar to 1.2. `internal/retrieval/column_bm25.go`. Use existing BM25 machinery.

**Files**: `internal/retrieval/column_bm25.go` (new)

---

### 1.4 Wrap graph-proximity as `GraphProximityColumn` (HIGH)

**Fix** — Extract graph-neighborhood scoring into `internal/retrieval/column_graph.go`. Uses existing CO_ACTIVATED_WITH / CODE_REL edges.

**Files**: `internal/retrieval/column_graph.go` (new)

---

## Phase 2: New Columns

**Goal**: Add the columns we don't have today — views of the data that the linear-combination pipeline does not capture.

### 2.1 `StructuralColumn` (HIGH)

**Rationale**: Two nodes in the same file/package/symbol tree are topologically related regardless of embedding similarity. A structural column surfaces these.

**Fix** — `internal/retrieval/column_structural.go`:

```go
// StructuralColumn retrieves nodes that share structural location with
// the query (same file, same package, same symbol tree). Uses the
// CODE_REL (contains, defined_in) graph.
type StructuralColumn struct { /* ... */ }

func (c *StructuralColumn) Retrieve(ctx, query, topK) ... {
    // Cypher: starting from query anchor, walk CODE_REL edges 1-3 hops,
    // rank by inverse hop distance.
}
```

**Files**: `internal/retrieval/column_structural.go` (new)

---

### 2.2 `TemporalColumn` (HIGH)

**Rationale**: Recently-touched nodes are often most relevant to current work, independent of similarity.

**Fix** — `internal/retrieval/column_temporal.go`: rank by `max(last_activated_at, updated_at)` with exponential decay (half-life ~1 day).

**Files**: `internal/retrieval/column_temporal.go` (new)

---

### 2.3 `RoleScopedColumn` (HIGH)

**Rationale**: Observations from the same developer / agent / task type are often more relevant than cross-role. Uses the role/source metadata already on observations.

**Fix** — `internal/retrieval/column_rolescoped.go`: restricts candidates to same-role observations (if query has a role context), then ranks by embedding within that scope.

**Files**: `internal/retrieval/column_rolescoped.go` (new)

---

### 2.4 Parallel execution (HIGH)

**Gap**: Running N columns sequentially adds latency.

**Fix** — In `internal/retrieval/ensemble.go` (new), execute columns in parallel via errgroup. Timeout per column = 80% of total retrieval budget. Slow columns drop out gracefully.

**Files**: `internal/retrieval/ensemble.go` (new)

---

### 2.5 Config-driven column enable (MEDIUM)

```go
RetrievalColumnsEnabled []string // RETRIEVAL_COLUMNS_ENABLED (csv; default: embedding,bm25,graph,structural,temporal,rolescoped)
RetrievalColumnWeights  map[string]float64 // RETRIEVAL_COLUMN_WEIGHT_<NAME> (per-column)
```

**Files**: `internal/config/config.go`

---

## Phase 3: Consensus Ranking Algorithm

**Goal**: Combine N column outputs into a single ranked list via consensus.

### 3.1 Reciprocal Rank Fusion baseline (HIGH)

**Fix** — Classic RRF:

```
RRF_score(node) = Σ_columns weight_c / (k + rank_c(node))
```

where k=60 (Cormack et al. standard) and rank_c(node) is the node's rank in column c (or infinity if not present).

```go
package retrieval

// ReciprocalRankFusion combines column results via RRF. Deterministic,
// explainable, parameter-light.
func ReciprocalRankFusion(results []ColumnResult, k float64) []RankedCandidate {
    // ...
}
```

**Files**: `internal/retrieval/consensus.go` (new), `internal/retrieval/consensus_test.go` (new)

---

### 3.2 Consensus strength as uncertainty metric (HIGH)

**Fix** — For each result, compute `consensus_strength = (columns_containing_node / total_columns_queried) × avg(normalized_rank)`. Persist on the response so downstream (re-ranker, LLM) can use it.

**Files**: `internal/retrieval/consensus.go`

---

### 3.3 Per-query column diagnostics (HIGH)

**Fix** — Attach per-column breakdown to the retrieval response (behind `?diagnostics=true` or a dedicated admin endpoint): which columns produced which results, which columns timed out, per-column latency. Enables the explainable-retrieval story.

**Files**: `internal/retrieval/ensemble.go`, `internal/api/handlers_retrieve.go`

---

### 3.4 Fallback to linear combination (MEDIUM)

**Fix** — If `RETRIEVAL_COLUMN_VOTING_ENABLED=false`, fall back to existing linear scoring. Feature flag gate.

**Files**: `internal/retrieval/ensemble.go`, `internal/config/config.go`

---

## Phase 4: Confidence Emission

### 4.1 Surface consensus strength to re-ranker (HIGH)

**Fix** — Pass `consensus_strength` from retrieval to the neural re-ranker as an input feature. High-consensus candidates already have a strong prior; re-ranker can focus on breaking ties.

**Files**: `internal/rerank/` (wherever the feature vector is built)

---

### 4.2 Surface to DH-005 confidence signal (HIGH)

**Fix** — The RSIC retrieval dimension's confidence (currently a learning-phase lookup) can be augmented with mean consensus strength over recent queries. High mean consensus = high retrieval confidence.

**Files**: `internal/ape/self_assess.go`

---

### 4.3 CLI inspection (MEDIUM)

`mdemg retrieve debug <query>` prints column-by-column breakdown and consensus strength.

**Files**: `internal/cli/retrieve.go` (new or extend existing)

---

## Phase 5: A/B Benchmark Infrastructure

### 5.1 Benchmark harness (HIGH)

**Fix** — `scripts/bench/column-voting-ab.sh`: runs whk-wms benchmark with (a) current linear scoring and (b) column-voting enabled. Paired comparison over 120 questions.

**Success criteria**: mean score equal or higher; high-score rate equal or higher; no individual question regressing >15%.

**Files**: `scripts/bench/column-voting-ab.sh`, `docs/tests/benchmarks/column-voting-ab.md`

---

### 5.2 Per-column ablation (HIGH)

**Fix** — Run with each column individually disabled, measure score impact. Identifies which columns are pulling weight and which are redundant.

**Files**: same

---

### 5.3 Latency benchmark (MEDIUM)

**Fix** — Measure p50/p95 retrieval latency with column voting vs linear. Parallel execution should keep latency in the same ballpark, but verify.

**Files**: `docs/tests/benchmarks/column-voting-latency.md`

---

## Phase 6: Observability

### 6.1 Prometheus metrics (HIGH)

```
mdemg_retrieval_column_latency_seconds{column, space_id} - histogram
mdemg_retrieval_column_timeout_total{column, space_id} - counter
mdemg_retrieval_consensus_strength{space_id} - histogram
mdemg_retrieval_column_contribution{column, space_id} - gauge (fraction of top-K that came from this column)
```

**Files**: `internal/metrics/registry.go`

### 6.2 Grafana panel (MEDIUM)

New row on `mdemg-overview.json`: "Retrieval Columns". Shows per-column latency, timeout rate, contribution.

**Files**: `deploy/grafana/dashboards/mdemg-overview.json`

### 6.3 Structured logging (MEDIUM)

Each retrieval call logs its column-by-column outcome at debug level.

**Files**: `internal/retrieval/ensemble.go`

---

## Phase 7: Testing & Verification

### 7.1 Unit tests (HIGH)
- Each column's `Retrieve` method (6 test files)
- `ReciprocalRankFusion` property tests (monotonic, deterministic, handles empty columns)
- Timeout handling (1 column slow, others fast)

### 7.2 Integration test (HIGH)
Seed graph, run a known query, verify consensus ranking matches expectation.

### 7.3 A/B benchmark (HIGH)
See Phase 5.1. Merge blocked until benchmark passes.

### 7.4 Load test (MEDIUM)
p95 latency under 200 QPS. Compared against linear baseline.

---

## Phase 8: Mandatory Documentation Phase

### 8.1 CHANGELOG.md (HIGH)
### 8.2 AGENT_HANDOFF.md (HIGH)
### 8.3 VISION.md — update retrieval section to describe column voting (HIGH)
### 8.4 CLAUDE.md — add retrieval-column vocabulary (HIGH)
### 8.5 `docs/architecture/06_Retrieval_API_and_Scoring.md` — major rewrite (HIGH)
### 8.6 `docs/user/cli-reference.md` — new env vars, new debug command (MEDIUM)
### 8.7 Homebrew beta testing guide + submodule bump (MEDIUM)

---

## Risk Analysis & Rollback

### R1: Latency regression from parallel columns

**Likelihood**: Low-Medium. Parallel execution should be no worse than sequential.

**Mitigation**: Per-column timeout. Slow columns drop silently rather than blocking. Latency benchmark (5.3) is a merge gate.

**Rollback**: `RETRIEVAL_COLUMN_VOTING_ENABLED=false` falls back to linear.

### R2: Benchmark regression on specific query types

**Likelihood**: Medium. Column voting trades per-query optimization for robustness; specific queries may score lower.

**Mitigation**: Per-column ablation (5.2) identifies which columns hurt on which query types. Adjust column weights.

**Rollback**: As in R1.

### R3: Consensus strength is misleading

**Likelihood**: Medium. With only 3-6 columns, strength is a coarse signal (steps of 0.17-0.33).

**Mitigation**: Document the coarseness. Treat strength as a three-tier signal (low / medium / high) not a continuous one.

**Rollback**: Ignore strength in downstream consumers; use raw score.

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. Column Interface & Refactor | 2 days |
| 2. New Columns | 3 days |
| 3. Consensus Ranking | 1.5 days |
| 4. Confidence Emission | 1 day |
| 5. A/B Benchmark | 1 day + benchmark time |
| 6. Observability | 1 day |
| 7. Testing & Verification | 2 days |
| 8. Mandatory Documentation | 0.5 day |
| **Total** | **~12 days** |

---

## Dependencies

**Blocks**: 06-sparse-retrieval-activation (which operates on the ensemble output).

**Blocked by**: None. Independent of the PC/FEP thread — can run in parallel with sprints 02/03 on the other dev branch.

**Touches but does not block**: 02 (consensus strength can feed precision weighting if both land).

---

## Documents Accessed

- `internal/api/handlers_retrieve.go` (to locate current scoring)
- `internal/config/config.go`
- `docs/architecture/06_Retrieval_API_and_Scoring.md`
- White paper review Paper 1
