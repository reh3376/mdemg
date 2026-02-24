# Edge-Type Attention Design Document

**Status:** VALIDATED
**Phase:** 3 (Post-benchmark validation)
**Author:** Claude
**Date:** 2026-01-30
**Implemented:** 2026-01-30
**Validated:** 2026-01-30 (0.898 mean score, +13.2% improvement)

---

## Executive Summary

Edge-Type Attention enhances MDEMG's activation spreading by replacing uniform edge treatment with attention-weighted aggregation. Different edge types (CO_ACTIVATED_WITH, ASSOCIATED_WITH, GENERALIZES, etc.) have different semantic meanings and should contribute differently to activation based on query context.

**Expected Impact:** 5-15% improvement in architecture/concept queries where hierarchical edges matter.

**Actual Impact (Validated 2026-01-30):**

- Overall: **+13.2%** (0.793 → 0.898)
- service_relationships: **+19.1%**
- data_flow_integration: **+17.1%**
- architecture_structure: **+10.4%**
- Variance reduced 51% (CV 16.3% → 6.6%)

---

## Current State Analysis

### Activation Spreading (activation.go:10-101)

The current implementation has a key limitation: **only CO_ACTIVATED_WITH edges are used for spreading**.

```go
// activation.go:46-53
for _, e := range edges {
    if e.RelType == "CONTRADICTS" {
        inhib[e.Dst] = append(inhib[e.Dst], e)
        continue
    }
    if e.RelType != "CO_ACTIVATED_WITH" {
        continue  // <-- ALL other edge types are IGNORED
    }
    incoming[e.Dst] = append(incoming[e.Dst], e)
}
```

### Edge Types in the System

| Edge Type | Direction | Purpose | Currently Used |
|-----------|-----------|---------|----------------|
| `CO_ACTIVATED_WITH` | Bidirectional | Hebbian learning patterns | YES |
| `ASSOCIATED_WITH` | Bidirectional | Semantic similarity | NO |
| `GENERALIZES` | L0 → L1 | Files to hidden concepts | NO |
| `ABSTRACTS_TO` | L1+ → higher | Concept hierarchy | NO |
| `TEMPORALLY_ADJACENT` | Sequential | Temporal proximity | NO |
| `CONTRADICTS` | Any | Inhibitory (opposing) | YES (inhibition) |
| `CAUSES`, `ENABLES` | Causal | Causal relationships | NO |

### Current Weight Calculation

```go
// activation.go:103-121
func effectiveWeight(e Edge) float64 {
    w := e.Weight
    // Fixed dimension mix - same for all edge types
    mix := 0.6*e.DimSemantic + 0.2*e.DimTemporal + 0.2*e.DimCoactivation
    if mix == 0 {
        mix = 1.0
    }
    return clamp(w * mix, 0, 1)
}
```

### Problem Statement

1. **Underutilized edges**: Hierarchical edges (GENERALIZES, ABSTRACTS_TO) are fetched but never influence activation
2. **Uniform treatment**: All CO_ACTIVATED_WITH edges treated equally regardless of query type
3. **No query awareness**: Edge importance doesn't adapt to whether user asks about code vs architecture

---

## Design: Edge-Type Attention

### Core Concept

Replace uniform edge filtering with attention-weighted aggregation:

```
activation[dst] += Σ(attention[edge_type, query_type] × weight × src_activation) / norm
```

Where `attention[edge_type, query_type]` is a learnable/configurable bias matrix.

### Attention Matrix (Default Values)

| Edge Type | Code Query | Architecture Query | Default |
|-----------|------------|-------------------|---------|
| CO_ACTIVATED_WITH | 1.0 | 0.7 | 0.85 |
| ASSOCIATED_WITH | 0.5 | 0.8 | 0.65 |
| GENERALIZES | 0.3 | 1.0 | 0.65 |
| ABSTRACTS_TO | 0.2 | 1.0 | 0.60 |
| TEMPORALLY_ADJACENT | 0.6 | 0.3 | 0.45 |

**Rationale:**

- Code queries benefit from CO_ACTIVATED_WITH (practical patterns observed in use)
- Architecture queries benefit from hierarchical edges (GENERALIZES, ABSTRACTS_TO)
- ASSOCIATED_WITH helps with semantic similarity (useful for both, more for architecture)

### Query Type Detection

Leverage existing `isCodeQuery()` and `isArchitectureQuery()` from scoring.go:

```go
// Already implemented in scoring.go:176-245
func isCodeQuery(query string) bool { ... }
func isArchitectureQuery(query string) bool { ... }
```

---

## Implementation Plan

### Phase 3.1: Configuration (config.go)

Add edge-type attention weights to config:

```go
// New config fields
EdgeAttentionEnabled      bool    // Feature toggle (default: true)
EdgeAttentionCoActivated  float64 // CO_ACTIVATED_WITH base weight (default: 0.85)
EdgeAttentionAssociated   float64 // ASSOCIATED_WITH base weight (default: 0.65)
EdgeAttentionGeneralizes  float64 // GENERALIZES base weight (default: 0.65)
EdgeAttentionAbstractsTo  float64 // ABSTRACTS_TO base weight (default: 0.60)
EdgeAttentionTemporal     float64 // TEMPORALLY_ADJACENT base weight (default: 0.45)

// Query-type modulation factors
EdgeAttentionCodeBoost    float64 // Multiplier for CO_ACTIVATED in code queries (default: 1.2)
EdgeAttentionArchBoost    float64 // Multiplier for hierarchical in arch queries (default: 1.5)
```

### Phase 3.2: Attention Types (activation.go)

Create new types and functions:

```go
// EdgeAttentionWeights holds per-edge-type attention weights
type EdgeAttentionWeights struct {
    CoActivated   float64
    Associated    float64
    Generalizes   float64
    AbstractsTo   float64
    Temporal      float64
}

// QueryContext provides context for attention modulation
type QueryContext struct {
    QueryText     string
    IsCodeQuery   bool
    IsArchQuery   bool
}

// ComputeEdgeAttention returns attention weights for a query context
func ComputeEdgeAttention(ctx QueryContext, cfg config.Config) EdgeAttentionWeights {
    weights := EdgeAttentionWeights{
        CoActivated: cfg.EdgeAttentionCoActivated,
        Associated:  cfg.EdgeAttentionAssociated,
        Generalizes: cfg.EdgeAttentionGeneralizes,
        AbstractsTo: cfg.EdgeAttentionAbstractsTo,
        Temporal:    cfg.EdgeAttentionTemporal,
    }

    if ctx.IsCodeQuery {
        // Boost CO_ACTIVATED for code queries
        weights.CoActivated *= cfg.EdgeAttentionCodeBoost
        // Reduce hierarchical edges for code queries
        weights.Generalizes *= 0.6
        weights.AbstractsTo *= 0.5
    }

    if ctx.IsArchQuery {
        // Boost hierarchical edges for architecture queries
        weights.Generalizes *= cfg.EdgeAttentionArchBoost
        weights.AbstractsTo *= cfg.EdgeAttentionArchBoost
        weights.Associated *= 1.2
        // Reduce CO_ACTIVATED slightly
        weights.CoActivated *= 0.8
    }

    return weights
}

// GetEdgeAttention returns the attention weight for a specific edge type
func (w EdgeAttentionWeights) GetEdgeAttention(relType string) float64 {
    switch relType {
    case "CO_ACTIVATED_WITH":
        return w.CoActivated
    case "ASSOCIATED_WITH":
        return w.Associated
    case "GENERALIZES":
        return w.Generalizes
    case "ABSTRACTS_TO":
        return w.AbstractsTo
    case "TEMPORALLY_ADJACENT":
        return w.Temporal
    default:
        return 0.5 // Unknown edge types get neutral weight
    }
}
```

### Phase 3.3: Modified Spreading Activation

Replace the current edge filtering with attention-weighted processing:

```go
// SpreadingActivationWithAttention computes activation with edge-type attention
func SpreadingActivationWithAttention(
    cands []Candidate,
    edges []Edge,
    steps int,
    lambda float64,
    attention EdgeAttentionWeights,
) map[string]float64 {
    act := map[string]float64{}

    // Seed from candidates (unchanged)
    for _, c := range cands {
        act[c.NodeID] = clamp01(c.VectorSim)
    }

    // Build incoming lists with attention weights
    // KEY CHANGE: Include ALL edge types, not just CO_ACTIVATED_WITH
    type WeightedEdge struct {
        Edge
        AttentionWeight float64
    }

    incoming := map[string][]WeightedEdge{}
    inhib := map[string][]Edge{}

    for _, e := range edges {
        if e.RelType == "CONTRADICTS" {
            inhib[e.Dst] = append(inhib[e.Dst], e)
            continue
        }

        attnWeight := attention.GetEdgeAttention(e.RelType)
        if attnWeight > 0.01 { // Skip near-zero attention edges
            incoming[e.Dst] = append(incoming[e.Dst], WeightedEdge{
                Edge:            e,
                AttentionWeight: attnWeight,
            })
        }
    }

    // Propagation steps
    for t := 0; t < steps; t++ {
        next := map[string]float64{}

        // Carry forward with decay
        for id, a := range act {
            next[id] = clamp01((1 - lambda) * a)
        }

        // Apply incoming with attention-weighted aggregation
        for dst, ins := range incoming {
            acc := next[dst]

            // Attention-weighted degree normalization
            // Edges with higher attention contribute more to normalization
            var totalAttnWeight float64
            for _, we := range ins {
                totalAttnWeight += we.AttentionWeight
            }
            degreeNorm := math.Sqrt(totalAttnWeight)
            if degreeNorm < 1 {
                degreeNorm = 1
            }

            for _, we := range ins {
                srcA := act[we.Src]
                w := effectiveWeight(we.Edge)
                // KEY: Apply attention weight to edge contribution
                acc += (srcA * w * we.AttentionWeight) / degreeNorm
            }

            // Inhibitory (unchanged)
            for _, e := range inhib[dst] {
                srcA := act[e.Src]
                w := math.Abs(effectiveWeight(e))
                acc -= srcA * w
            }

            next[dst] = clamp01(acc)
        }

        act = next
    }

    return act
}
```

### Phase 3.4: Integration with Retrieval Service

Modify `service.go:Retrieve()` to pass query context:

```go
// In Retrieve(), before calling SpreadingActivation:

// Build query context for edge attention
queryCtx := QueryContext{
    QueryText:   req.QueryText,
    IsCodeQuery: isCodeQuery(req.QueryText),
    IsArchQuery: isArchitectureQuery(req.QueryText),
}

// Compute attention weights
var attention EdgeAttentionWeights
if s.cfg.EdgeAttentionEnabled {
    attention = ComputeEdgeAttention(queryCtx, s.cfg)
} else {
    // Fallback: only CO_ACTIVATED_WITH (current behavior)
    attention = EdgeAttentionWeights{
        CoActivated: 1.0,
        Associated:  0.0,
        Generalizes: 0.0,
        AbstractsTo: 0.0,
        Temporal:    0.0,
    }
}

// Use new function
act := SpreadingActivationWithAttention(cands, edges, 2, 0.15, attention)
```

---

## Testing Strategy

### Unit Tests

```go
func TestEdgeAttentionWeights(t *testing.T) {
    // Test code query boosts CO_ACTIVATED
    ctx := QueryContext{IsCodeQuery: true}
    weights := ComputeEdgeAttention(ctx, defaultConfig)
    assert.Greater(t, weights.CoActivated, weights.Generalizes)

    // Test architecture query boosts hierarchical
    ctx = QueryContext{IsArchQuery: true}
    weights = ComputeEdgeAttention(ctx, defaultConfig)
    assert.Greater(t, weights.Generalizes, weights.CoActivated)
}

func TestSpreadingActivationWithAttention(t *testing.T) {
    edges := []Edge{
        {Src: "a", Dst: "b", RelType: "CO_ACTIVATED_WITH", Weight: 0.8},
        {Src: "a", Dst: "c", RelType: "GENERALIZES", Weight: 0.8},
    }

    // Code query: b should get more activation than c
    codeAttn := EdgeAttentionWeights{CoActivated: 1.0, Generalizes: 0.3}
    act := SpreadingActivationWithAttention(cands, edges, 2, 0.15, codeAttn)
    assert.Greater(t, act["b"], act["c"])

    // Arch query: c should get more activation than b
    archAttn := EdgeAttentionWeights{CoActivated: 0.5, Generalizes: 1.0}
    act = SpreadingActivationWithAttention(cands, edges, 2, 0.15, archAttn)
    assert.Greater(t, act["c"], act["b"])
}
```

### Benchmark Validation

Run the standard benchmark suite before and after:

```bash
# Before (baseline)
python3 docs/benchmarks/benchmark_runner_v3.py \
  --questions docs/benchmarks/whk-wms/test_questions_120.json \
  --output-dir /tmp/benchmark_pre_attention \
  --codebase /path/to/whk-wms \
  --space-id whk-wms

# After implementation
# Same command, different output dir
```

**Success Criteria:**

- Overall score maintains or improves (currently 0.880)
- Architecture-category questions show improvement
- No regression on code-focused questions

---

## Configuration Reference

| Env Variable | Default | Description |
|--------------|---------|-------------|
| `EDGE_ATTENTION_ENABLED` | `true` | Feature toggle |
| `EDGE_ATTENTION_CO_ACTIVATED` | `0.85` | Base weight for CO_ACTIVATED_WITH |
| `EDGE_ATTENTION_ASSOCIATED` | `0.65` | Base weight for ASSOCIATED_WITH |
| `EDGE_ATTENTION_GENERALIZES` | `0.65` | Base weight for GENERALIZES |
| `EDGE_ATTENTION_ABSTRACTS_TO` | `0.60` | Base weight for ABSTRACTS_TO |
| `EDGE_ATTENTION_TEMPORAL` | `0.45` | Base weight for TEMPORALLY_ADJACENT |
| `EDGE_ATTENTION_CODE_BOOST` | `1.2` | Multiplier for CO_ACTIVATED in code queries |
| `EDGE_ATTENTION_ARCH_BOOST` | `1.5` | Multiplier for hierarchical in arch queries |

---

## Files to Modify

| File | Changes |
|------|---------|
| `internal/config/config.go` | Add 8 new config fields for attention weights |
| `internal/retrieval/activation.go` | Add attention types, new spreading function |
| `internal/retrieval/service.go` | Build QueryContext, call new activation function |
| `internal/retrieval/activation_test.go` | Unit tests for attention mechanism |

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Performance regression | Low | Medium | Feature toggle, comprehensive testing |
| Worse scores on some query types | Medium | Medium | Configurable weights, A/B testing |
| Increased latency | Low | Low | Attention computation is O(edges), minimal |

---

## Future Enhancements

### Phase 3.5: Learned Attention (Future)

Instead of fixed/rule-based attention, learn optimal weights from retrieval feedback:

```go
// Record (query_type, edge_type, result_quality) tuples
// Periodically update attention weights via gradient-free optimization
// E.g., use feedback from user clicks, successful retrievals
```

### Phase 3.6: Per-Query Attention (Future)

Use query embedding to compute attention, not just query type classification:

```go
// attentionFromEmbedding computes edge attention from query embedding
func attentionFromEmbedding(queryEmb []float32, cfg config.Config) EdgeAttentionWeights {
    // Project query embedding through learned attention head
    // Return dynamically computed weights
}
```

---

## Summary

Edge-Type Attention extends MDEMG's activation spreading to leverage all edge types with query-aware weighting. The implementation:

1. **Includes all edge types** in spreading (not just CO_ACTIVATED_WITH)
2. **Applies attention weights** based on edge type and query context
3. **Modulates dynamically** for code vs architecture queries
4. **Maintains backward compatibility** via feature toggle and fallback behavior

Expected improvement: 5-15% on architecture queries, maintained performance on code queries.
