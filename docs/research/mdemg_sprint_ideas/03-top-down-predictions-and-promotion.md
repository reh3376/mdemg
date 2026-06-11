# 03 — Top-Down Predictions & Prediction-Error Promotion

**Sprint ID**: PC-REFRAME-B2
**Date**: 2026-04-21 (plan authored; execute after 02 ships and soaks ≥1 release cycle)
**Branch**: TBD
**Scope**: Add the missing reciprocal **top-down prediction stream** from higher layers to lower layers. Use the **prediction errors** that result (divergence between L_{n+1}'s prediction of L_n activity and actual L_n activity) as the primary signal for (a) promotion to higher layers, and (b) identifying nodes that need restructuring. Replaces or augments the current evidence-counting promotion rule.

**Upstream**: [01](01-pc-reframe-and-surprise-routing.md), [02](02-precision-weighted-hebbian-eta.md); [White Paper Review](mdemg-white-paper-review.md) Papers 1, 2, 5, 6, 9.

---

## Sprint Framing

This is the **highest-risk architectural sprint** in the PC/FEP thread. It adds a component that does not exist today: a reciprocal prediction stream where each higher-layer node maintains a **predicted activation pattern** over its lower-layer children, and the divergence from actual activation drives learning.

Why this matters — from the paper review:

1. **Papers 5 and 6** (Millidge et al.) are explicit that hierarchical predictive coding is defined by this reciprocal stream. Without it, MDEMG is a one-way hierarchy (bottom-up promotion only), not a true PC hierarchy.
2. **Paper 1** (Hawkins) makes a weaker but converging point: columns vote downward as well as upward, and the iterative consensus is what enables one-shot learning.
3. **Promotion quality**: the current promotion rule is evidence-accumulation ("this L0 pattern has been seen N times"). A prediction-error rule is information-theoretic ("this L0 pattern is not well explained by any existing L1 node, so it deserves its own L1 node"). The latter catches novel patterns faster and rejects redundant ones.

**Risk management**: this sprint produces **predictions without acting on them** in Phase 1-3, then gates the first production use on empirical validation in Phase 4. The architecture is added in shadow mode for at least 2 weeks before flowing into the actual promotion rule.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Prediction Schema & Computation | 1 | 4 | 1 | 0 | **6** |
| Prediction-Error Accumulation | 0 | 3 | 1 | 0 | **4** |
| Shadow-Mode Promotion Candidate Stream | 0 | 3 | 2 | 0 | **5** |
| Empirical Validation | 0 | 2 | 2 | 0 | **4** |
| Observability | 0 | 2 | 1 | 0 | **3** |
| Testing & Verification | 0 | 3 | 1 | 0 | **4** |
| Mandatory Documentation Phase | 0 | 5 | 2 | 0 | **7** |
| **Total** | **1** | **22** | **10** | **0** | **33** |

---

## Phase 1: Prediction Schema & Computation

**Goal**: Define what a top-down prediction *is* in MDEMG's graph, and compute them on every L_{n+1} node update.

### 1.1 Define the prediction data model (CRITICAL)

**Gap**: There is no data structure representing "what L_{n+1} expects L_n to look like."

**Decision** — a prediction is a **sparse distribution over L_n child nodes**: a set of (child_node_id, expected_activation) tuples. This is storable as a Neo4j relationship PREDICTS with a weight, or as a JSON property on the parent node. Go with the relationship form for Cypher-native querying.

```cypher
(parent:MemoryNode {layer: n+1})-[:PREDICTS {
  expected_activation: 0.73,
  updated_at: datetime(),
  prediction_version: 1
}]->(child:MemoryNode {layer: n})
```

**Files**: `internal/migrations/V0025_predicts_relationship.cypher` (new), `internal/hidden/types.go` (new `PredictionEdge` struct)

---

### 1.2 Prediction computation from co-activation history (HIGH)

**Gap**: No code computes predictions.

**Fix** — New file `internal/learning/predictions.go`:

```go
package learning

// ComputePredictions generates a top-down prediction for a parent node
// based on its children's historical co-activation pattern when the
// parent was active. The prediction is the probability that child c
// will be active given parent p is active, normalized to [0, 1].
//
// Input: parent node, all (child, coact_weight) edges from the parent,
// total activation count for the parent.
//
// Output: map[child_id]expected_activation, to be persisted as PREDICTS
// relationships.
func ComputePredictions(
    parentNodeID string,
    children []ChildCoactivation,
    parentActivations int64,
) map[string]float64 {
    predictions := make(map[string]float64, len(children))
    for _, c := range children {
        // Bayesian estimate: P(child active | parent active)
        // Add Laplace smoothing to handle small samples
        numerator := float64(c.CoactivationCount) + 1.0
        denominator := float64(parentActivations) + 2.0
        predictions[c.NodeID] = numerator / denominator
    }
    return predictions
}

type ChildCoactivation struct {
    NodeID            string
    CoactivationCount int64
    EdgeWeight        float64
}
```

**Trigger**: predictions are recomputed for parent P when (a) P gets reinforced, (b) a new child is linked to P, (c) > 1 hour has elapsed since last recompute (batched background job).

**Files**: `internal/learning/predictions.go` (new), `internal/learning/predictions_test.go` (new)

---

### 1.3 Persist predictions to Neo4j (HIGH)

**Fix** — In `internal/learning/service.go`, new method `PersistPredictions`:

```go
func (s *Service) PersistPredictions(ctx context.Context, spaceID, parentNodeID string, preds map[string]float64) error {
    // Batched MERGE of PREDICTS relationships; delete stale predictions.
    // ...
}
```

Cypher:

```cypher
UNWIND $preds AS p
MATCH (parent:MemoryNode {node_id: $parentId})
MATCH (child:MemoryNode {node_id: p.child_id})
MERGE (parent)-[r:PREDICTS]->(child)
SET r.expected_activation = p.expected,
    r.updated_at = datetime(),
    r.prediction_version = coalesce(r.prediction_version, 0) + 1
```

**Files**: `internal/learning/service.go`

---

### 1.4 Background job: predict refresh (HIGH)

**Fix** — Extend `internal/ape/cycle.go` micro cycle to include a `refreshPredictions()` step. Enumerates L_{n+1} nodes whose predictions are >1h stale, recomputes, persists.

**Files**: `internal/ape/cycle.go`

---

### 1.5 Env vars + config (HIGH)

```go
TopDownPredictionsEnabled      bool    // TOPDOWN_PREDICTIONS_ENABLED (default false — opt-in shadow mode)
PredictionRefreshIntervalSec   int     // PREDICTION_REFRESH_INTERVAL_SEC (default 3600)
PredictionMinEvidence          int64   // PREDICTION_MIN_EVIDENCE (default 5 — parent must have ≥5 activations before predictions are meaningful)
PredictionErrorFlagThreshold   float64 // PREDICTION_ERROR_FLAG_THRESHOLD (default 0.3 — divergence above this flags a promotion candidate)
```

**Files**: `internal/config/config.go`, `.env.example`, compose templates

---

### 1.6 Layer-pair scope (MEDIUM)

**Gap**: Enabling predictions for all layer pairs at once is risky. Need per-layer-pair enable.

**Fix** — env var `TOPDOWN_PREDICTIONS_LAYERS=L1_L0,L2_L1` to scope which layer pairs participate. Default empty (everywhere off). Start with L1_L0 only.

**Files**: `internal/config/config.go`

---

## Phase 2: Prediction-Error Accumulation

**Goal**: When L_n is active in a session, compare against L_{n+1}'s predictions; accumulate the error.

### 2.1 Prediction-error computation on activation (HIGH)

**Fix** — In `CoactivateSession` or a new sibling call, after L_n activations occur:

```cypher
// For each parent P of an activated L_n node, look up P's prediction
// for that child and record the difference.
MATCH (parent:MemoryNode)-[r:PREDICTS]->(child:MemoryNode {node_id: $activatedChildId})
WITH parent, r, child,
     r.expected_activation AS predicted,
     $actualActivation AS actual,
     $actualActivation - r.expected_activation AS error

// Accumulate error in the parent's prediction-error field
SET parent.prediction_error_sum = coalesce(parent.prediction_error_sum, 0) + abs(error),
    parent.prediction_error_count = coalesce(parent.prediction_error_count, 0) + 1,
    parent.prediction_error_last_updated = datetime()
```

**Files**: `internal/learning/service.go`, `internal/migrations/V0026_prediction_error_fields.cypher`

---

### 2.2 Precision-weighted error (HIGH)

**Gap**: A large error from a low-confidence node is less informative than the same error from a high-confidence node. We should weight by `activation_confidence` from Sprint 02.

**Fix** — If `PRECISION_WEIGHTED_ETA_ENABLED` is true (i.e., Sprint 02 is active), weight the accumulated error by the child's confidence:

```cypher
SET parent.prediction_error_sum = coalesce(parent.prediction_error_sum, 0)
    + abs(error) * coalesce(child.activation_confidence, 0.5)
```

This couples the two sprints: predictions-errors naturally use the precision signal from Sprint 02.

**Files**: `internal/learning/service.go`

---

### 2.3 Prediction-error decay (HIGH)

**Gap**: Without decay, parents that have ever had error accumulate it forever, biasing the signal toward old failures.

**Fix** — Exponential decay applied in the micro cycle. Half-life matches `CONFIDENCE_HALF_LIFE_SEC` from Sprint 02 (1 week by default).

**Files**: `internal/ape/cycle.go`

---

### 2.4 Per-parent normalized error rate (MEDIUM)

**Gap**: Raw error sum is hard to interpret. Normalize to a rate.

**Fix** — Computed metric: `prediction_error_rate = prediction_error_sum / max(1, prediction_error_count)`. Persisted as node property; surfaces in queries and metrics.

**Files**: `internal/hidden/service.go`, `internal/learning/service.go`

---

## Phase 3: Shadow-Mode Promotion Candidate Stream

**Goal**: Use prediction errors to identify promotion candidates — but do not promote yet. Compare against the existing evidence-counting rule for ≥2 weeks.

### 3.1 Shadow promotion candidate enumeration (HIGH)

**Fix** — New method `EnumeratePredictionBasedCandidates`:

```go
// Returns L_n nodes whose prediction error rate exceeds the threshold,
// suggesting they are not well-explained by any existing L_{n+1} node
// and are candidates for promotion.
func (s *Service) EnumeratePredictionBasedCandidates(
    ctx context.Context, spaceID string, fromLayer, toLayer int,
) ([]PromotionCandidate, error) {
    // Cypher: L_n nodes whose aggregated prediction error from all
    // parents exceeds PREDICTION_ERROR_FLAG_THRESHOLD
}
```

**Files**: `internal/hidden/service.go`, `internal/hidden/service_promotion_test.go`

---

### 3.2 Candidate persistence as shadow edges (HIGH)

**Gap**: Need to store shadow candidates for later comparison without mutating the production graph.

**Fix** — `(node:MemoryNode)-[:SHADOW_PROMOTION_CANDIDATE {source: 'prediction_error', score: 0.42, created_at: datetime()}]->(:ShadowMarker)` — the candidate is marked via a tagged relationship to a singleton `ShadowMarker` node per space.

**Files**: `internal/migrations/V0027_shadow_promotion.cypher`, `internal/hidden/service.go`

---

### 3.3 Compare to evidence-counting candidate set (HIGH)

**Fix** — Periodic (daily) job that enumerates both (a) current evidence-counting promotion candidates and (b) prediction-error candidates, computes set intersection / difference / Jaccard similarity, and records in metrics.

**Files**: `internal/ape/cycle.go` (macro cycle), `internal/metrics/registry.go`

---

### 3.4 Shadow-mode report CLI (MEDIUM)

`mdemg debug promotion-shadow --space <id>` — prints both candidate sets, overlap stats, top 10 disagreements.

**Files**: `internal/cli/debug.go`

---

### 3.5 Gate for live promotion (MEDIUM)

**Decision rule** — after ≥2 weeks of shadow mode, the prediction-error rule can be promoted to production only if:
- Jaccard similarity with evidence-counting rule ≥ 0.5 (we're not in a different universe)
- At least one disagreement case manually reviewed and the prediction-error candidate judged correct
- No RSIC red-alerts during the shadow period

**Files**: documented in `docs/features/predictive-coding-scaffold.md` (update), no code

---

## Phase 4: Empirical Validation

### 4.1 whk-wms benchmark with shadow mode (HIGH)

**Fix** — Run the benchmark with predictions enabled (shadow mode) and compare to baseline. Expect: no change in retrieval scores (predictions do not affect retrieval in shadow mode). If there is any change, there is a bug.

**Files**: `docs/tests/benchmarks/pc-b2-shadow-ab.md`

---

### 4.2 Manual review of disagreement cases (HIGH)

**Gap**: The literature supports prediction-error promotion, but empirical validation on MDEMG's domain is required.

**Fix** — After 1 week of shadow mode, developer manually reviews the top 20 disagreement cases (where prediction-error flagged a node that evidence-counting did not, and vice versa). Classify each: "prediction-error correct", "evidence-counting correct", "both correct", "both wrong". Target: prediction-error correct in ≥60% of contested cases.

**Files**: `docs/tests/benchmarks/pc-b2-manual-review.md`

---

### 4.3 Benchmark after flag-on (MEDIUM)

**Fix** — After promoting the rule to production (if gate passes), re-run whk-wms. Expect: modest improvement or no regression.

**Files**: `docs/tests/benchmarks/pc-b2-live-ab.md`

---

### 4.4 Rollback plan documentation (MEDIUM)

**Files**: inline in this plan (see Risk Analysis)

---

## Phase 5: Observability

### 5.1 Prometheus gauges (HIGH)

```
mdemg_predictions_count{space_id, parent_layer, child_layer}
mdemg_prediction_error_rate{space_id, layer} - histogram
mdemg_shadow_promotion_candidates{space_id, layer, source}
mdemg_promotion_candidate_jaccard{space_id} - between evidence and prediction rules
```

**Files**: `internal/metrics/registry.go`

---

### 5.2 Grafana panel (HIGH)

New dashboard `mdemg-predictions.json` with 6 panels:
- Total predictions per layer
- Prediction error rate distribution
- Shadow candidates count over time
- Jaccard similarity trend
- Top 10 highest-error nodes
- Refresh job duration

**Files**: `deploy/grafana/dashboards/mdemg-predictions.json` (new)

---

### 5.3 CLI: `mdemg predict inspect <node_id>` (MEDIUM)

Shows predictions out of / into the node, error history, promotion-candidate status.

**Files**: `internal/cli/predict.go` (new)

---

## Phase 6: Testing & Verification

### 6.1 Unit tests (HIGH)

- `internal/learning/predictions_test.go`: ComputePredictions property tests
- `internal/hidden/service_promotion_test.go`: candidate enumeration correctness
- `internal/ape/cycle_predictions_test.go`: refresh and decay cycle

### 6.2 Integration test (HIGH)

Seed a synthetic graph with known prediction structure, verify:
1. Predictions are computed with expected values (Laplace-smoothed)
2. Prediction errors accumulate correctly
3. Shadow-mode candidates are enumerated without touching production promotion

### 6.3 Chaos test (HIGH)

Inject a "surprise pattern" (L0 nodes that don't match any existing L1 prediction well), verify that the shadow candidate stream identifies them within 1 refresh cycle.

---

## Phase 7: Mandatory Documentation Phase

### 7.1 CHANGELOG.md (HIGH)
### 7.2 AGENT_HANDOFF.md (HIGH)
### 7.3 CLAUDE.md — update PC vocabulary, add top-down predictions section (HIGH)
### 7.4 VISION.md — add section on top-down predictions (HIGH)
### 7.5 `docs/features/predictive-coding-scaffold.md` — major update, remove "not yet implemented" top-down predictions (HIGH)
### 7.6 `docs/user/cli-reference.md` — new env vars, new CLI commands (MEDIUM)
### 7.7 Homebrew beta testing guide + submodule bump (MEDIUM)

---

## Risk Analysis & Rollback

### R1: Prediction computation is expensive at scale

**Likelihood**: High for large graphs (>1M nodes).

**Mitigation**: 
- Batched background refresh, not inline.
- `PREDICTION_MIN_EVIDENCE` gates predictions to parents with enough data.
- `TOPDOWN_PREDICTIONS_LAYERS` scopes which layer pairs are active.

**Rollback**: Set `TOPDOWN_PREDICTIONS_ENABLED=false`. PREDICTS relationships become stale but harmless; a cleanup migration can drop them.

### R2: Shadow mode and live mode disagree strongly

**Likelihood**: Medium. If the prediction-error rule identifies very different nodes from the evidence rule, both may be partially correct.

**Mitigation**: The decision gate in 3.5 requires Jaccard ≥ 0.5 before flag-on. If they're far apart, we investigate before flipping — this is expected behavior, not a bug.

**Rollback**: Stay in shadow mode indefinitely. Shadow candidates can inform manual promotion decisions without being automated.

### R3: Prediction-error rule promotes too aggressively

**Likelihood**: Medium. A low threshold would over-promote.

**Mitigation**: 
- `PREDICTION_ERROR_FLAG_THRESHOLD` defaults conservative (0.3).
- Manual review in Phase 4.2 calibrates.
- RSIC can detect over-promotion (sudden spike in L_{n+1} node count) and adjust.

**Rollback**: Increase threshold, or revert to evidence-counting rule entirely.

### R4: Combined with Sprint 02's precision weighting, compounds risk

**Likelihood**: Medium. Two changes to learning dynamics in sequence.

**Mitigation**: Mandatory soak of Sprint 02 for ≥1 release cycle before starting this sprint. Do not stack the flags.

**Rollback**: Either flag can be disabled independently.

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. Prediction Schema & Computation | 3 days |
| 2. Prediction-Error Accumulation | 2 days |
| 3. Shadow-Mode Candidate Stream | 2 days |
| 4. Empirical Validation | 2 weeks calendar (1 day dev + soak) |
| 5. Observability | 1.5 days |
| 6. Testing & Verification | 2 days |
| 7. Mandatory Documentation | 1 day |
| **Total dev time** | **~12 days** |
| **Total calendar** | **~3 weeks incl. shadow soak** |

---

## Dependencies

**Blocked by**: 01 (PC vocabulary), 02 (activation_confidence for precision-weighted errors).

**Blocks**: None in the numbered set. This is a leaf sprint in the PC/FEP thread.

---

## Documents Accessed

- `internal/learning/service.go`
- `internal/hidden/service.go`
- `internal/ape/cycle.go`
- 01-, 02- sprint plans
- White paper review Papers 1, 2, 5, 6, 9
