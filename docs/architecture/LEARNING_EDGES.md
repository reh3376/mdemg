# Learning Edges: Understanding Hebbian Learning in MDEMG

**Purpose:** This document explains how learning edges affect retrieval scoring and how to interpret scores across different learning phases.

## What Are Learning Edges?

Learning edges are `CO_ACTIVATED_WITH` relationships created by MDEMG's Hebbian learning system. They form between nodes that are frequently retrieved together, encoding usage patterns.

```
[FileA] ──CO_ACTIVATED_WITH──> [FileB]
         weight: 0.45
         evidence_count: 12
         surprise_factor: 1.5
```

### Edge Properties

| Property | Description | Range |
|----------|-------------|-------|
| `weight` | Connection strength | 0.0 - 1.0 |
| `evidence_count` | Times co-activated | 1 - ∞ |
| `surprise_factor` | Novelty boost (conversation only) | 1.0 - 2.0 |
| `decay_rate` | How fast weight decays | 0.001 default |

### How Edges Form

1. **Query issued** → Top-K results retrieved
2. **Results returned** → Nodes marked as "co-activated"
3. **Hebbian update** → Edges created/strengthened between co-activated nodes
4. **Weight formula:** `w_new = (1-μ)*w_old + η*(a_i * a_j)`

Where:

- `η` (eta) = learning rate (default 0.02)
- `μ` (mu) = decay rate (default 0.01)
- `a_i, a_j` = activation levels of nodes i and j

## How Edges Affect Retrieval Scoring

### The Scoring Formula

```go
score = α*vecSim + β*activation + γ*recency + δ*confidence + boosts - penalties
```

| Component | Weight | Description |
|-----------|--------|-------------|
| Vector Similarity (α) | 55% | Semantic match to query |
| **Activation (β)** | **30%** | Spreading activation through edges |
| Recency (γ) | 10% | How recently modified |
| Confidence (δ) | 5% | Prior retrieval success |

**Key insight:** Activation contributes 30% of the final score.

### Spreading Activation

When a query is issued:

1. Seed nodes (vector matches) receive initial activation
2. Activation spreads through edges for N steps
3. Each edge contributes: `activation += source_activation * edge_weight`
4. Final activation is clamped to [0, 1]

## The Activation Dilution Effect

### What Happens As Edges Accumulate

**Cold Start (0 edges):**

- Activation only spreads through structural edges (ABSTRACTS_TO, etc.)
- Limited pathways = concentrated activation
- Clear score differentiation between candidates

**Warm (1,000-10,000 edges):**

- CO_ACTIVATED_WITH edges create new pathways
- Activation spreads to more nodes
- Score distribution begins compressing

**Saturated (10,000+ edges):**

- Many overlapping pathways
- Most nodes receive some activation
- Scores compress toward the middle

### Visualization

```
Edge Count:     0         5,000      20,000
                |           |           |
Score Range:  0.3-0.9    0.4-0.8    0.5-0.75
                ▼           ▼           ▼
              [wide]    [medium]   [narrow]
```

### Impact on Confidence Levels

With fixed thresholds (e.g., score > 0.85 = HIGH):

| Phase | HIGH | MEDIUM | LOW |
|-------|------|--------|-----|
| Cold (0 edges) | 15% | 57% | 28% |
| Warm (10k edges) | 8% | 52% | 40% |
| Saturated (25k edges) | 2% | 47% | 51% |

**Critical:** Retrieval quality remains stable - only absolute scores change.

## Normalized Confidence (The Solution)

MDEMG provides **percentile-based confidence** that's immune to edge density:

```json
{
  "score": 0.72,
  "normalized_confidence": 85,
  "confidence_level": "HIGH"
}
```

### How It Works

1. Results sorted by score
2. Each result assigned percentile: `(n-1-rank) / (n-1) * 100`
3. Confidence levels assigned:
   - `normalized_confidence >= 90` → HIGH
   - `normalized_confidence >= 40` → MEDIUM
   - `normalized_confidence < 40` → LOW

### Why Percentiles Are Better

| Metric | Fixed Threshold | Normalized Percentile |
|--------|-----------------|----------------------|
| Edge density sensitivity | HIGH | NONE |
| Cross-run comparability | Poor | Excellent |
| Top result confidence | Varies (0.5-0.95) | Always 100 |

## Learning Phases

### Phase 1: Cold Start (0 edges)

- **Characteristics:** Pure semantic retrieval
- **Score behavior:** Wide distribution (0.3 - 0.9)
- **Best for:** Initial testing, baseline measurements

### Phase 2: Learning (0-10,000 edges)

- **Characteristics:** System learning usage patterns
- **Score behavior:** Distribution compressing
- **Best for:** Active development, pattern discovery

### Phase 3: Warm (10,000-50,000 edges)

- **Characteristics:** Rich associative network
- **Score behavior:** Stable compression
- **Best for:** Production use

### Phase 4: Saturated (50,000+ edges)

- **Characteristics:** Dense connections, diminishing returns
- **Score behavior:** Highly compressed
- **Consider:** Pruning low-evidence edges, freezing learning

## Best Practices

### For Development/Testing

1. **Reset edges between benchmark runs:**

   ```bash
   curl -X POST http://localhost:7474/db/neo4j/tx/commit \
     -d '{"statements":[{"statement":"MATCH ()-[r:CO_ACTIVATED_WITH]->() WHERE r.space_id = \"your-space\" DELETE r"}]}'
   ```

2. **Use normalized_confidence for comparisons**

3. **Track edge count alongside scores:**

   ```bash
   curl -X POST http://localhost:9999/v1/memory/learning/stats \
     -d '{"space_id":"your-space"}'
   ```

### For Production

1. **Monitor score distributions:**
   - Track mean, std, percentiles over time
   - Alert on sudden changes

2. **Consider learning freeze:**
   - Stop edge creation for stable scoring
   - Use `POST /v1/memory/learning/freeze` (when implemented)

3. **Periodic edge pruning:**
   - Remove low-evidence edges (< 3 evidence_count)
   - Prune edges with decayed weight < threshold

4. **Use percentile confidence:**
   - Always include `normalized_confidence` in responses
   - Base decisions on percentile, not raw score

### Interpreting Results

| Scenario | Interpretation |
|----------|----------------|
| High raw score (0.85+), HIGH normalized | Strong match, high confidence |
| Low raw score (0.5), HIGH normalized | Best available match in compressed distribution |
| High raw score, MEDIUM normalized | Good match but others are better |
| Low raw score, LOW normalized | Weak match, consider fallback |

## Monitoring Recommendations

### Key Metrics to Track

1. **Edge count by space:**

   ```cypher
   MATCH ()-[r:CO_ACTIVATED_WITH {space_id: $spaceId}]->()
   RETURN count(r) as edge_count
   ```

2. **Score distribution:**
   - Mean score per query
   - Score standard deviation
   - Percentage in each confidence band

3. **Edge growth rate:**
   - New edges per day
   - Evidence count growth

### Alert Conditions

| Condition | Action |
|-----------|--------|
| Edge count > 50,000 | Consider pruning |
| Mean score < 0.4 | Check query quality |
| Score std < 0.05 | Distribution too compressed |
| HIGH confidence < 1% | Check thresholds or use percentiles |

## Conversation Memory Specifics

Learning edges for conversation observations include additional features:

### Surprise Factor

Edges from surprising observations decay slower:

```
effective_decay = base_decay / sqrt(evidence_count * surprise_factor)
```

| Surprise Level | Factor | Decay Rate |
|----------------|--------|------------|
| HIGH (≥0.7) | 2.0 | 41% slower |
| MEDIUM (0.4-0.7) | 1.5 | 22% slower |
| NORMAL (<0.4) | 1.0 | Standard |

### Session-Based Coactivation

Observations in the same session are automatically linked:

- Edges weighted by temporal proximity
- Closer observations = stronger edges
- 1-hour proximity window

## Troubleshooting

### Problem: All scores are low (< 0.5)

**Causes:**

1. Dense edge network (normal after heavy use)
2. Query doesn't match indexed content
3. Embedding quality issues

**Solutions:**

1. Use `normalized_confidence` instead of raw scores
2. Check query against known-good examples
3. Verify embedding generation is working

### Problem: Score variance is very small

**Cause:** Activation dilution from many edges

**Solutions:**

1. This is normal - use percentile confidence
2. Consider edge pruning if > 50k edges
3. Adjust activation weight (β) if needed

### Problem: Confidence levels don't match expectations

**Cause:** Fixed thresholds don't account for distribution compression

**Solution:** Always use `normalized_confidence` and `confidence_level` fields

## References

- Investigation: `docs/CONFIDENCE_SCORE_INVESTIGATION.md`
- Benchmark analysis: `docs/BENCHMARK_IMPROVEMENTS.md`
- Scoring code: `internal/retrieval/scoring.go`
- Activation code: `internal/retrieval/activation.go`
- Learning code: `internal/learning/service.go`
