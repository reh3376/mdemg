# Confidence Score Degradation Investigation

**Task:** #1 - Investigate confidence score degradation with learning edges
**Status:** ROOT CAUSE IDENTIFIED
**Date:** 2026-01-27

## Executive Summary

The confidence score degradation (HIGH dropped 87% as learning edges accumulated) is caused by **activation dilution** in the spreading activation algorithm. As CO_ACTIVATED_WITH edges accumulate, activation spreads to more nodes, compressing the score distribution and lowering absolute scores while maintaining relative ordering.

## Root Cause Analysis

### The Scoring Formula

From `internal/retrieval/scoring.go:293`:

```go
s = α*vecSim + β*activation + γ*recency + δ*confidence + boosts - penalties
```

| Parameter | Default | Weight |
|-----------|---------|--------|
| α (vector) | 0.55 | 55% |
| β (activation) | 0.30 | **30%** |
| γ (recency) | 0.10 | 10% |
| δ (confidence) | 0.05 | 5% |

**Critical insight:** Activation contributes 30% of the final score.

### Spreading Activation Algorithm

From `internal/retrieval/activation.go:58-83`:

```go
for t := 0; t < steps; t++ {
    for dst, ins := range incoming {
        acc := next[dst]
        for _, e := range ins {
            srcA := act[e.Src]
            w := effectiveWeight(e)
            acc += srcA * w  // <-- Accumulates from ALL edges
        }
        next[dst] = clamp01(acc)  // <-- Clamped to [0, 1]
    }
}
```

### The Dilution Problem

**Run 1 (0 learning edges):**

- Activation only spreads through ABSTRACTS_TO and ASSOCIATED_WITH edges
- Limited edge count = limited spreading pathways
- Top candidates maintain high activation relative to others
- Score distribution: wide spread, clear differentiation

**Run 3 (24,860 learning edges):**

- CO_ACTIVATED_WITH edges create many new pathways
- Activation spreads to many more nodes
- Previously low-scoring nodes receive activation from multiple sources
- Score distribution: compressed toward middle, reduced differentiation

### Visualization

```
Run 1 (sparse edges):           Run 3 (dense edges):

Node A ─── 0.85 activation      Node A ─── 0.72 activation
Node B ─── 0.62 activation      Node B ─── 0.68 activation
Node C ─── 0.45 activation      Node C ─── 0.61 activation
Node D ─── 0.20 activation      Node D ─── 0.55 activation

Spread: 0.65                    Spread: 0.17
```

The activation values compress as more edges allow propagation to more nodes.

### Why Retrieval Quality Doesn't Degrade

1. **Relative ordering preserved**: Top-K candidates are still similar
2. **File selection stable**: Same files retrieved, just with lower scores
3. **Confidence thresholds affected**: Fixed thresholds (e.g., >0.75 = HIGH) become harder to achieve

## Evidence

### Benchmark Data

| Metric | Run 1 | Run 3 | Change |
|--------|-------|-------|--------|
| Learning edges | 0 | 24,860 | +24,860 |
| HIGH confidence | 15.2% | 1.6% | **-87%** |
| LOW confidence | 28.0% | 51.2% | +83% |
| Mean retrieval score | 0.5895 | 0.5447 | -7.6% |
| File citation quality | Good | Good | Unchanged |

### Code Path Confirmation

1. `service.go:586-622`: CO_ACTIVATED_WITH edges fetched for spreading activation
2. `activation.go:66-71`: Edges contribute to activation accumulation
3. `scoring.go:287`: `actComponent = β * a` (30% of score)
4. No normalization applied after scoring

## Solution Options

### Option 1: Percentile-Based Confidence (Recommended)

Instead of absolute thresholds, use percentile rankings within each query's result set.

**Location:** `internal/retrieval/scoring.go`

```go
// After scoring all candidates, compute percentiles
scores := make([]float64, len(items))
for i, item := range items {
    scores[i] = item.Score
}
sort.Float64s(scores)

for i := range items {
    percentile := float64(sort.SearchFloat64s(scores, items[i].Score)) / float64(len(scores))
    items[i].Breakdown.NormalizedConfidence = percentile * 100

    // Confidence level based on percentile
    switch {
    case percentile >= 0.90:
        items[i].ConfidenceLevel = "HIGH"
    case percentile >= 0.50:
        items[i].ConfidenceLevel = "MEDIUM"
    default:
        items[i].ConfidenceLevel = "LOW"
    }
}
```

**Pros:**

- Immune to edge count changes
- Enables meaningful cross-run comparisons
- Simple to implement

**Cons:**

- Loses absolute score meaning (a weak result set still has "HIGH" top results)

### Option 2: Edge-Density Normalization

Scale activation component by edge density.

**Location:** `internal/retrieval/scoring.go`

```go
// Compute edge density factor
edgeDensity := float64(len(edges)) / float64(len(cands))
densityFactor := 1.0 / math.Log(1.0 + edgeDensity)

// Apply to activation component
actComponent := beta * a * densityFactor
```

**Pros:**

- Maintains absolute score meaning
- Compensates for graph growth

**Cons:**

- Requires tuning
- May over-correct for legitimate dense graphs

### Option 3: Activation Distribution Normalization

Apply z-score normalization to activation values before scoring.

**Location:** `internal/retrieval/activation.go`

```go
// After spreading, normalize activation distribution
mean, std := computeMeanStd(act)
for id, a := range act {
    zScore := (a - mean) / std
    act[id] = sigmoid(zScore)  // Map to [0, 1]
}
```

**Pros:**

- Preserves relative ordering
- Maintains score differentiation

**Cons:**

- Changes activation semantics
- May require recalibration of other components

### Option 4: Dynamic Beta (Activation Weight)

Scale β inversely with learning edge count.

**Location:** `internal/retrieval/scoring.go`

```go
// Get learning edge count from graph stats
baseBeta := cfg.ScoringBeta  // 0.30
edgeCount := getLearningEdgeCount(spaceID)

// Reduce beta as edges accumulate (saturates at 50% reduction)
reductionFactor := 1.0 / (1.0 + math.Log(1.0 + float64(edgeCount)/10000.0))
adjustedBeta := baseBeta * reductionFactor
```

**Pros:**

- Directly addresses the root cause
- Self-adjusting

**Cons:**

- Requires per-query edge count lookup
- Complex to tune

## Recommendation

**Implement Option 1 (Percentile-Based Confidence)** as the primary fix:

1. Simple to implement
2. Immune to graph density changes
3. Enables meaningful cross-run comparisons
4. Doesn't change core scoring algorithm

Additionally, **implement Option 2 (Edge-Density Normalization)** as an optional enhancement for users who need absolute score stability.

## Files to Modify

1. `internal/retrieval/scoring.go` - Add percentile calculation
2. `internal/models/retrieve.go` - Add NormalizedConfidence field
3. `internal/api/handlers.go` - Return new field in API response
4. `internal/config/config.go` - Add configuration for normalization options

## Next Steps

1. Implement percentile-based confidence (Task #2 already created)
2. Add tests for score normalization
3. Update benchmark to track normalized confidence
4. Document behavior in LEARNING_EDGES.md (Task #4)
