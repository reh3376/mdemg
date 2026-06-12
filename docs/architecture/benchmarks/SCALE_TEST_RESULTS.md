# MDEMG Scale Test Results

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-01-23
**Purpose**: Phase D Validation - Test MDEMG at 10K-100K node scale
**Target**: VS Code repository (Microsoft)

---

## Test Configuration

### Source Repository

| Attribute | Value |
|-----------|-------|
| **Repository** | microsoft/vscode |
| **Clone** | Shallow (--depth 1) |
| **Size** | 178 MB |
| **LOC** | 1,602,025 (TypeScript, src/ only) |
| **Source Files** | 6,260 TypeScript files |

### MDEMG Settings

| Parameter | Value |
|-----------|-------|
| Space ID | vscode-scale |
| Endpoint | <http://localhost:9999> |
| Batch size | 100 |
| Workers | 4 |
| Timeout | 300s |
| Excluded | node_modules, .git, out, dist |

---

## Ingestion Results

| Metric | Value |
|--------|-------|
| **Elements Ingested** | 28,406 |
| **Observations** | 30,706 |
| **Ingestion Time** | ~25 minutes |
| **Ingestion Rate** | ~19 elements/sec |
| **Embedding Coverage** | 99.99% |
| **Errors** | 0 |

### Comparison to Smaller Codebases

| Codebase | Elements | Ingestion Time | Rate |
|----------|----------|----------------|------|
| whk-wms | 9,067 | ~10 min | ~15/s |
| plc-gbt | 6,007 | ~6 min | ~17/s |
| **vscode-scale** | **28,406** | **~25 min** | **~19/s** |

**Observation**: Ingestion rate remains consistent (~15-19/s) regardless of scale. Time scales linearly with element count.

---

## Consolidation Results

| Metric | Value |
|--------|-------|
| **Consolidation Time** | 18 minutes |
| **Total Memories** | 28,960 |
| **L0 (Base Data)** | 28,406 |
| **L1 (Hidden Layer)** | 553 |
| **L2 (Concept Layer)** | 1 |
| **Avg Node Degree** | 14.5 |
| **Max Node Degree** | 12,926 |

### L1 Node Distribution

| Node Type | Count | Notes |
|-----------|------:|-------|
| comparison | 441 | VS Code has many similar modules |
| hidden | 100 | Capped at max_hidden |
| concern | 7 | Cross-cutting patterns detected |
| ui-* | 3 | UI pattern nodes (Track 6) |
| config | 1 | Configuration summary |
| temporal | 1 | Temporal patterns |
| **Total** | **553** | |

### Comparison to Smaller Codebases

| Codebase | Elements | L1 Nodes | Consolidation Time |
|----------|----------|----------|-------------------|
| whk-wms | 9,067 | ~150 | ~1 min |
| plc-gbt | 6,007 | 143 | ~48 sec |
| **vscode-scale** | **28,406** | **553** | **18 min** |

**Observation**: Consolidation time scales non-linearly. The high comparison node count (441) indicates VS Code's modular architecture with many similar components.

---

## Retrieval Performance

### Latency Measurements

| Dataset | Elements | Cold Query | Warm Query (avg) |
|---------|----------|-----------|------------------|
| vscode-scale | 28,960 | 275-1353ms | **50-55ms** |
| plc-gbt | 6,007 | 522ms | **58-66ms** |
| whk-wms | 9,067 | ~400ms | **~50ms** |

**Key Finding**: After initial cold query, latency drops to ~50ms regardless of dataset size. Caching is highly effective.

### Query Quality at Scale

| Query | Latency | Top Score |
|-------|---------|-----------|
| Extension host communication | 1353ms | 0.805 |
| TextEditor API | 816ms | 0.789 |
| Keyboard shortcuts handling | 708ms | 0.752 |
| Diff algorithm | 849ms | 0.792 |
| Settings editor | 271ms | 0.788 |

**Observation**: Retrieval quality remains high (0.75-0.80) at 28K scale.

### Learning Edge Creation

| Metric | Value |
|--------|-------|
| Initial edges (post-consolidation) | 664 |
| Edges after 20 queries | 2,460 |
| New edges created | 1,796 |
| Edges per query | ~90 |

**Observation**: Hebbian learning active and creating edges at expected rate.

---

## Resource Utilization

### Neo4j Database

| Metric | Value |
|--------|-------|
| Total nodes | 28,960 |
| Total relationships | ~420,000 (est) |
| Vector index entries | 28,960 |
| Database size | ~2 GB (est) |

### Memory/CPU

| Phase | Peak Memory | CPU |
|-------|-------------|-----|
| Ingestion | ~500 MB | 4 cores utilized |
| Consolidation | ~1.5 GB | Single core |
| Retrieval | ~200 MB | Single core |

---

## Conclusions

### Scale Test Validates Phase D

1. **Linear ingestion scaling** - 28K elements in 25 min at ~19/s (consistent with smaller codebases)

2. **Non-linear consolidation** - 18 min for 28K (vs ~1 min for 6K), but still acceptable

3. **Excellent warm query latency** - ~50ms regardless of scale after caching

4. **Quality maintained** - 0.75-0.80 retrieval scores at 28K (comparable to smaller codebases)

5. **Learning system active** - ~90 edges/query, Hebbian learning works at scale

6. **All 6 tracks functioning** - Concern, comparison, config, temporal, UI, and learning edges all active

### Recommendations

1. **Production viable at 30K nodes** - Current architecture handles this scale well

2. **Consider consolidation optimization** - 18 min is acceptable but could be improved with parallel processing

3. **Max node degree warning** - 12,926 degree node could be a performance concern; consider hub penalty tuning

4. **Ready for 100K test** - Current results suggest 100K would take ~2 hours to ingest, ~1 hour to consolidate

---

## Test Artifacts

| File | Description |
|------|-------------|
| Local VS Code shallow clone | VS Code shallow clone |
| Space ID: `vscode-scale` | MDEMG space with 28K elements |
| This document | Scale test results summary |

---

## Next Steps

- [ ] Test at 100K scale (multiple repos combined)
- [ ] Optimize consolidation for parallel processing
- [ ] Tune hub penalty for high-degree nodes
- [ ] Benchmark retrieval with 1000+ queries
