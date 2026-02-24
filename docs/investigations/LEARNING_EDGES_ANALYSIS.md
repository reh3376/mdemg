# Learning Edges Analysis

**Date**: 2026-01-22
**Status**: Root cause identified, fix proposed

## Summary

CO_ACTIVATED_WITH edge creation IS working, but at a very slow rate. After 100+ queries during v9 testing, only ~888 edges were created. The root cause is overly narrow activation seeding that results in most returned nodes having zero activation.

## Diagnosis

### Current Behavior

1. **SpreadingActivation** (`activation.go:22-36`) seeds only the **top 2 candidates**:

   ```go
   seedN := 2
   for i := 0; i < seedN; i++ {
       act[cands[i].NodeID] = cands[i].VectorSim
   }
   ```

2. Activation only propagates to other nodes if:
   - They're reachable via edges in the graph
   - Those edges have non-zero weights

3. **ApplyCoactivation** (`learning/service.go:42-46`) filters nodes:

   ```go
   for _, r := range resp.Results {
       if r.Activation >= minAct {  // default: 0.20
           nodes = append(nodes, r)
       }
   }
   ```

### Observed Activation Distribution

Sample query returned 6 nodes:

| Rank | Activation | Passes Threshold? |
|------|------------|-------------------|
| 1 | 0.637 | ✓ |
| 2 | 0.000 | ✗ |
| 3 | 0.725 | ✓ |
| 4 | 0.000 | ✗ |
| 5 | 0.051 | ✗ |
| 6 | 0.000 | ✗ |

**Result**: Only 2 nodes pass → 1 pair created per query

### Edge Growth Rate

| Before Test | After 5 Queries | Growth |
|-------------|-----------------|--------|
| 888 | 892 | +4 edges (2 pairs) |

**Rate**: ~0.8 pairs per query (less than 1 because some queries have <2 qualifying nodes)

## Root Cause

**Narrow Activation Seeding**: Only top 2 candidates get seeded. Combined with sparse graph connectivity (few existing edges for propagation), most returned nodes have zero activation.

This creates slow-start dynamics:

1. Few edges exist initially
2. Activation can't spread without edges
3. Few nodes pass threshold → few new edges
4. Slow accumulation of edges

## Proposed Fix

### Option A: Seed All Candidates (Recommended)

Modify `activation.go` to seed ALL candidates with their VectorSim values:

```go
// Seed: ALL candidates seeded from vector similarity
for _, c := range cands {
    v := c.VectorSim
    if v < 0 {
        v = 0
    }
    if v > 1 {
        v = 1
    }
    act[c.NodeID] = v
}
```

**Expected Impact**:

- All 10 returned nodes would have activation
- If 6+ pass threshold, that's 15+ pairs per query (C(6,2) = 15)
- Edge accumulation would be ~30x faster

**Risk**: Potential clique spam (many edges between frequently co-retrieved nodes)

**Mitigation**: Already handled by `LearningEdgeCapPerRequest` (default 200) which limits pairs per query

### Option B: Lower Threshold

Reduce `LearningMinActivation` from 0.20 to 0.05:

```go
if minAct <= 0 {
    minAct = 0.05  // lower threshold
}
```

**Pros**: Simpler change
**Cons**: Doesn't address root cause (still only 2 seeded)

### Option C: Use Score Instead of Activation

In learning, use the composite `Score` instead of `Activation`:

```go
for _, r := range resp.Results {
    if r.Score >= minScore {  // e.g., 0.30
        nodes = append(nodes, r)
    }
}
```

**Pros**: All returned nodes have meaningful scores
**Cons**: Changes the semantic meaning of "co-activation"

## Recommendation

**Implement Option A** (seed all candidates) because:

1. It fixes the root cause (narrow seeding)
2. Maintains the intended Hebbian semantics (activation-based learning)
3. Existing cap prevents runaway edge creation
4. Will bootstrap the feedback loop: more edges → more spreading → more learning

## Implementation Plan

1. Modify `activation.go:22-36` to seed all candidates
2. Add debug logging to track edge creation rate
3. Run 50-question test to validate increased edge creation
4. Monitor for clique spam via metrics endpoint

## Files Modified

- `mdemg_build/service/internal/retrieval/activation.go` - Seed all candidates instead of top 2

## Results After Fix

### Edge Creation Rate

| Metric | Before Fix | After Fix | Improvement |
|--------|------------|-----------|-------------|
| Pairs per query | ~0.8 | **43.1** | **54x faster** |
| 20 queries | ~16 pairs | **863 pairs** | - |

### Activation Distribution

Before fix:

- 2-3 nodes per query passed the 0.20 threshold
- Most nodes had 0 activation (not seeded)

After fix:

- **10 out of 10** results pass the 0.20 threshold
- All candidates seeded with VectorSim values
- Up to **45 pairs per query** (C(10,2) = 45)

### Verification

```
=== Before test ===
CO_ACTIVATED_WITH edges: 944

=== After 20 queries ===
CO_ACTIVATED_WITH edges: 2670

New edges created: 1726 (863 pairs)
Rate: 43.1 pairs/query
```

## Conclusion

The fix successfully bootstraps the Hebbian learning feedback loop:

1. More candidates seeded → more nodes pass threshold
2. More pairs created → more CO_ACTIVATED_WITH edges
3. More edges → better spreading activation in future queries
4. Self-reinforcing improvement over time
