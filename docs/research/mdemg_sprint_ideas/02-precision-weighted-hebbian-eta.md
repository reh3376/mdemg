# 02 — Precision-Weighted Hebbian η

**Sprint ID**: PC-REFRAME-B1
**Date**: 2026-04-21 (plan authored; execute after 01 has soaked ≥1 release cycle)
**Branch**: TBD
**Scope**: Extend DH-005's precision-weighting principle from the health formula (where it already lives) into the Hebbian edge update rule itself. Modulate learning rate η by per-node activation confidence, so high-confidence observations have stronger learning effects than low-confidence ones.

**Upstream**: [01-pc-reframe-and-surprise-routing.md](01-pc-reframe-and-surprise-routing.md); [White Paper Review](mdemg-white-paper-review.md) Papers 5, 6 (predictive coding, precision weighting).

---

## Sprint Framing

Sprint 01 formalized DH-005 as precision-weighted evidence integration. That precision weighting currently applies only to **how health dimensions are combined**, not to **how the graph updates itself**. The Hebbian rule in `internal/learning/service.go`:

```
w' = clamp(wmin, wmax, (1-μ)·w + η·prod)
```

treats every co-activation the same: η is a single config value (possibly multiplied by maturity-phase and relationship-type factors), independent of how confident we are in the nodes being co-activated.

Predictive coding says the update magnitude should be **precision-weighted** — a small confident update beats a large unconfident update. Practically: an edge between two well-evidenced, recently-reinforced nodes should update *more* aggressively than an edge between two sparsely-seen nodes, because the signal-to-noise ratio of the former is higher.

Concrete change: introduce per-node **activation confidence** `c_n` ∈ [0,1], compute per-edge effective learning rate `η_eff = η · c_a · c_b`, and use this in the update. Surprise, which currently boosts initial weight only, also becomes a modulator of `c_n` over time.

This is a **medium-high risk** change. It alters learning dynamics for every edge going forward. It must ship behind a feature flag and be validated against the whk-wms benchmark before becoming default.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Schema & Confidence Accumulator | 0 | 3 | 1 | 0 | **4** |
| Hebbian Update Extension | 1 | 2 | 1 | 0 | **4** |
| Observability & A/B Infrastructure | 0 | 2 | 2 | 0 | **4** |
| Testing & Verification | 0 | 3 | 1 | 0 | **4** |
| Mandatory Documentation Phase | 0 | 5 | 2 | 0 | **7** |
| **Total** | **1** | **15** | **7** | **0** | **23** |

---

## Phase 1: Schema & Activation Confidence Accumulator

**Goal**: Introduce per-node activation confidence as a first-class property, compute it from existing signals, update it on every observation.

### 1.1 Add `activation_confidence` to MemoryNode (HIGH)

**Gap**: `MemoryNode` lacks any notion of per-node confidence. We have surprise, reinforcement count, and created_at separately, but no combined confidence signal.

**Fix** — Add to Cypher node creation / update paths, and to the Go struct in `internal/hidden/types.go`:

```go
// ActivationConfidence is a unit-interval precision proxy for this node's
// reliability as a source of co-activation evidence. Higher = more reliable
// signal for Hebbian updates. Updated on every observation that involves
// this node.
//
// Formula (Phase 1.2 computation):
//   c = σ(α·log(1+n_reinforce) + β·recency_decay - γ·surprise_variance)
//
// Defaults: α=1.0, β=0.5, γ=0.3. Clamped to [0.05, 1.0] — never zero
// so that new nodes still participate in learning.
ActivationConfidence float64 `json:"activation_confidence"`
ActivationConfidenceUpdatedAt time.Time `json:"activation_confidence_updated_at"`
```

**Cypher additions** — migration V0024:

```cypher
// Migration V0024: per-node activation confidence for PC-REFRAME-B1
MATCH (n:MemoryNode) WHERE n.activation_confidence IS NULL
SET n.activation_confidence = 0.5,
    n.activation_confidence_updated_at = datetime()
```

Default of 0.5 for existing nodes is neutral — no aggressive shift on migration.

**Files**: `internal/hidden/types.go`, `internal/hidden/service.go`, `internal/migrations/V0024_activation_confidence.cypher`

---

### 1.2 Compute activation confidence on observation (HIGH)

**Gap**: No code computes or updates `c_n` today.

**Fix** — New file `internal/hidden/confidence.go`:

```go
package hidden

import "math"

// ComputeActivationConfidence computes a node's current activation
// confidence from its reinforcement history, recency, and surprise
// variance. Output is clamped to [0.05, 1.0].
func ComputeActivationConfidence(
    reinforceCount int64,
    lastActivatedSecondsAgo float64,
    surpriseHistory []float64,
    cfg ConfidenceConfig,
) float64 {
    // Reinforcement term: log-saturating, rewards evidence accumulation
    nTerm := cfg.Alpha * math.Log1p(float64(reinforceCount))

    // Recency term: exponential decay with half-life = cfg.HalfLifeSec
    recencyTerm := cfg.Beta * math.Exp(-lastActivatedSecondsAgo/cfg.HalfLifeSec)

    // Surprise variance term: high variance in surprise = unstable signal
    var surpriseVar float64
    if len(surpriseHistory) > 1 {
        surpriseVar = variance(surpriseHistory)
    }
    varianceTerm := cfg.Gamma * surpriseVar

    raw := sigmoid(nTerm + recencyTerm - varianceTerm)
    return clamp(raw, 0.05, 1.0)
}

type ConfidenceConfig struct {
    Alpha       float64 // reinforcement weight
    Beta        float64 // recency weight
    Gamma       float64 // surprise-variance penalty
    HalfLifeSec float64 // recency decay half-life
}

func DefaultConfidenceConfig() ConfidenceConfig {
    return ConfidenceConfig{Alpha: 1.0, Beta: 0.5, Gamma: 0.3, HalfLifeSec: 604800} // 1 week
}
```

**Wiring** — in `internal/conversation/service.go` `Observe()` after node creation:

```go
// Update activation confidence for the observed node
s.updateNodeActivationConfidence(ctx, nodeID)
```

and similarly after any CoactivateSession reinforces a node.

**Files**: `internal/hidden/confidence.go` (new), `internal/hidden/confidence_test.go` (new), `internal/conversation/service.go`

---

### 1.3 Confidence persistence on edge operations (HIGH)

**Gap**: Update rule needs to read `c_a` and `c_b` during the Cypher update. Currently the update touches only the edge, not the nodes.

**Fix** — Extend the Cypher in `CoactivateSession` to project node confidences:

```cypher
// After MERGE (a)-[r]->(b), read node confidences:
WITH a, b, r, activation, temporalProximity, surpriseFactor, initialWeight,
     coalesce(a.activation_confidence, 0.5) AS ca,
     coalesce(b.activation_confidence, 0.5) AS cb,
     coalesce(r.weight, initialWeight) AS w,
     activation * activation AS prod

// Precision-weighted eta (only when feature flag is set)
WITH a, b, r, w, prod, ca, cb,
     CASE WHEN $precisionWeightedEta THEN $eta * ca * cb ELSE $eta END AS etaEff

SET r.weight = CASE
  WHEN ((1-$mu)*w + etaEff*prod) < $wmin THEN $wmin
  WHEN ((1-$mu)*w + etaEff*prod) > $wmax THEN $wmax
  ELSE ((1-$mu)*w + etaEff*prod)
END,
r.eta_effective = etaEff  // persisted for observability
```

**Files**: `internal/learning/service.go`

---

### 1.4 Env vars + config (MEDIUM)

```go
PrecisionWeightedEtaEnabled bool    // PRECISION_WEIGHTED_ETA_ENABLED (default false — opt-in)
ConfidenceAlpha             float64 // CONFIDENCE_ALPHA (default 1.0)
ConfidenceBeta              float64 // CONFIDENCE_BETA (default 0.5)
ConfidenceGamma             float64 // CONFIDENCE_GAMMA (default 0.3)
ConfidenceHalfLifeSec       float64 // CONFIDENCE_HALF_LIFE_SEC (default 604800)
```

**Files**: `internal/config/config.go`, `.env.example`, `deploy/docker/docker-compose*.yml`, `internal/config/yaml_config.go`

---

## Phase 2: Hebbian Update Extension

### 2.1 Feature-flagged update rule (CRITICAL)

**Gap**: The existing update runs unconditionally. Toggling the new rule must be reversible in production without rebuild.

**Fix** — Thread `PrecisionWeightedEtaEnabled` through the Cypher params (shown in 1.3). When false, behavior is byte-identical to current.

**Files**: `internal/learning/service.go`

---

### 2.2 Per-edge `eta_effective` persistence (HIGH)

**Gap**: Without persisting the effective η per update, we cannot distinguish "low weight because nodes are low-confidence" from "low weight because of decay" in post-hoc analysis.

**Fix** — Added in 1.3 Cypher (`r.eta_effective = etaEff`). Also persist `updated_with_precision_eta` boolean.

**Files**: `internal/learning/service.go`

---

### 2.3 Shadow-mode dual-write (HIGH)

**Gap**: Before flipping the flag globally, we want to see what η_eff *would* be on every update, without actually applying it.

**Fix** — When `PRECISION_WEIGHTED_ETA_SHADOW_MODE=true` and main flag is false, compute `etaEff` but use `$eta` (unchanged) for the weight update. Persist `r.eta_effective_shadow` for offline analysis.

**Files**: `internal/learning/service.go`, `internal/config/config.go`

---

### 2.4 RSIC adaptive override (MEDIUM)

**Gap**: If RSIC detects that precision-weighted η is causing regressions, it should be able to disable it autonomously (not wait for operator).

**Fix** — Add a new RSIC action type `disable_precision_eta` that clears the flag at runtime via a config-override mechanism. Gate on RSIC detecting sustained health drop (>3% for >2h).

**Files**: `internal/ape/rsic/actions.go`, `internal/ape/cycle.go`

---

## Phase 3: Observability & A/B Infrastructure

### 3.1 Prometheus gauges (HIGH)

```
mdemg_node_activation_confidence{space_id} - histogram
mdemg_edge_eta_effective{space_id} - histogram
mdemg_edge_eta_effective_shadow{space_id} - histogram (shadow mode only)
mdemg_precision_eta_enabled{space_id} - gauge (0/1)
```

**Files**: `internal/metrics/registry.go`, emission sites in `internal/learning/service.go` and `internal/hidden/service.go`

---

### 3.2 Grafana panel (MEDIUM)

New row on `mdemg-rsic.json`: "Precision-Weighted Learning (PC-REFRAME-B1)" with 4 panels:
- Histogram of node confidence distribution
- Histogram of edge η_effective (actual + shadow when available)
- Stat: current enablement state
- Time-series: delta between actual and shadow η over time (sanity check)

**Files**: `deploy/grafana/dashboards/mdemg-rsic.json`

---

### 3.3 A/B bench harness (HIGH)

**Gap**: Need a reproducible way to compare whk-wms benchmark scores with and without precision-weighted η.

**Fix** — Script `scripts/bench/pc-b1-ab.sh`:
1. Snapshot current graph
2. Run whk-wms 120-question benchmark with flag off → record score set A
3. Reset to snapshot
4. Run with flag on → record score set B
5. Paired t-test, report mean delta, 95% CI

**Success criteria**: B ≥ A with no individual question regressing more than 10%.

**Files**: `scripts/bench/pc-b1-ab.sh`, `docs/tests/benchmarks/pc-b1-ab.md`

---

### 3.4 CLI inspection (MEDIUM)

`mdemg debug confidence <node_id>` — shows node's computed confidence, inputs (reinforcement count, last activated, surprise history), what effective η would be with each of its edges.

**Files**: `internal/cli/debug.go`

---

## Phase 4: Testing & Verification

### 4.1 Unit tests (HIGH)

- `internal/hidden/confidence_test.go`: property tests (monotonic in reinforcement, decays with recency, penalized by variance, clamped to [0.05, 1.0])
- `internal/learning/service_precision_eta_test.go`: Cypher invocation with flag on/off, shadow-mode correctness
- `internal/ape/rsic/actions_test.go`: adaptive-override trigger condition

**Files**: all three test files

---

### 4.2 Integration test (HIGH)

`tests/integration/precision_eta_test.go`:
1. Seed graph with 100 nodes at varying confidences
2. Trigger 50 co-activation sessions
3. Assert: edges between high-confidence node pairs have grown faster than edges between low-confidence pairs
4. Assert: shadow mode produces identical weights to flag-off mode

**Files**: `tests/integration/precision_eta_test.go`

---

### 4.3 A/B benchmark execution (HIGH)

Run harness from Phase 3.3 on whk-wms. Record results in `docs/tests/benchmarks/pc-b1-ab.md`. Merge blocked until A/B result is favorable (B ≥ A - 0.5%) and no regressions >10% on individual questions.

---

### 4.4 Canary rollout (MEDIUM)

After flag-on merge: enable in dev spaces for 1 week → observability review → then production.

---

## Phase 5: Mandatory Documentation Phase

### 5.1 CHANGELOG.md (HIGH)

### 5.2 AGENT_HANDOFF.md (HIGH)

### 5.3 CLAUDE.md — update PC vocabulary table to mark this row as implemented (HIGH)

### 5.4 `docs/user/cli-reference.md` — 5 new env vars, new `debug confidence` command (HIGH)

### 5.5 `docs/features/predictive-coding-scaffold.md` — update "not yet implemented" list to remove "precision-weighted η" (HIGH)

### 5.6 Homebrew beta testing guide + submodule bump (MEDIUM)

### 5.7 Migration V0024 documentation in `docs/operations/migrations.md` (MEDIUM)

---

## Risk Analysis & Rollback

### R1: Learning dynamics regression

**Likelihood**: Medium. The rule changes update magnitudes globally; even a 1-2% systemic shift could impair retrieval.

**Mitigation**:
- Feature flag defaults false. Opt-in only.
- Shadow mode lets us see the intended behavior before flipping.
- A/B benchmark must pass before flag-on default.
- RSIC adaptive override can disable autonomously on regression.

**Rollback**: Set `PRECISION_WEIGHTED_ETA_ENABLED=false`. No data migration needed — edges retain their final weight under either rule. `activation_confidence` remains on nodes and can be reused later.

### R2: Confidence computation is expensive on every observation

**Likelihood**: Low. The formula is O(1) given the input signals; the surprise-variance term needs a small sliding window per node.

**Mitigation**:
- Cache `activation_confidence` on the node; recompute only on material change (new reinforcement, > HalfLife/4 elapsed).
- Surprise history kept as a fixed-size ring buffer (last 20 values).

**Rollback**: Disable confidence updates, fall back to default 0.5 for all nodes (effectively no-op in flag-on mode).

### R3: Cold-start nodes get unfairly small updates

**Likelihood**: Medium. New nodes start at confidence 0.5 or less; their first edges get lower η_eff.

**Mitigation**:
- Clamp lower bound to 0.05 — never zero.
- Consider: first N reinforcements use flag-off η (warm-up window).
- Monitor `mdemg_node_activation_confidence` histogram; if too bimodal, adjust α.

**Rollback**: As in R1.

### R4: Migration V0024 on large graphs is slow

**Likelihood**: Low-Medium. Large spaces (>1M nodes) may take minutes.

**Mitigation**:
- Batched SET with APOC (10k nodes per batch).
- Migration is idempotent (`WHERE n.activation_confidence IS NULL`).

**Rollback**: Migration sets a default value; no destructive change. Rolling back just leaves the column present.

---

## Files Changed (summary)

**New**: `internal/hidden/confidence.go`, `internal/hidden/confidence_test.go`, `internal/learning/service_precision_eta_test.go`, `internal/migrations/V0024_activation_confidence.cypher`, `scripts/bench/pc-b1-ab.sh`, `tests/integration/precision_eta_test.go`, `docs/tests/benchmarks/pc-b1-ab.md`

**Modified**: `internal/hidden/types.go`, `internal/hidden/service.go`, `internal/conversation/service.go`, `internal/learning/service.go`, `internal/config/config.go`, `internal/config/yaml_config.go`, `internal/metrics/registry.go`, `internal/ape/rsic/actions.go`, `internal/ape/cycle.go`, `internal/cli/debug.go`, `.env.example`, `deploy/docker/docker-compose*.yml`, `deploy/grafana/dashboards/mdemg-rsic.json`, plus all mandatory-doc files.

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. Schema & Confidence Accumulator | 2 days |
| 2. Hebbian Update Extension | 1.5 days |
| 3. Observability & A/B Infrastructure | 1.5 days |
| 4. Testing & Verification | 2 days |
| 5. Mandatory Documentation | 0.5 day |
| Canary + A/B soak | 1 week (calendar, not dev) |
| **Total dev time** | **~7.5 days** |
| **Total calendar** | **~2 weeks incl. soak** |

---

## Dependencies

**Blocks**: 03-top-down-predictions-and-promotion (B1's confidence accumulator feeds B2's prediction-error precision weighting).

**Blocked by**: 01-pc-reframe-and-surprise-routing (needs PC vocabulary, surprise routing infrastructure).

**Touches but does not block**: 04-column-voting-retrieval (node confidence could also feed column-voting confidence emission; optional integration).

---

## Documents Accessed

- `internal/learning/service.go`
- `internal/hidden/service.go`, `types.go`
- `internal/ape/health_formula.go`, `self_assess.go`
- 01-pc-reframe-and-surprise-routing.md
- White paper review Papers 5, 6
