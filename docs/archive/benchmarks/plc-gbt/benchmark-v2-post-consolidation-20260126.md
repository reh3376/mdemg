# PLC-GBT Benchmark v2: Post-Consolidation Results

**Date:** 2026-01-26 08:13:38 EST
**Run Type:** Post-consolidation validation
**Operator:** reh3376

---

## Executive Summary

| Metric | Pre-Consolidation | Post-Consolidation | Delta |
|--------|-------------------|-------------------|-------|
| **Mean Retrieval Score** | 0.6489* | 0.6196 | -0.029 |
| **High Score (>0.7)** | 23% | 14% | -9% |
| **Medium Score (0.5-0.7)** | 77% | 85% | +8% |
| **Low Score (<0.5)** | 0% | 1% | +1% |
| **Layer 1 Nodes** | 0 | 404 | +404 |
| **Learning Edges** | 228 | 13,170 | +12,942 |

*Pre-consolidation score from raw retrieval similarity (different from graded score)

---

## Key Finding

**Consolidation did not improve raw retrieval scores.** The hidden layer nodes (404 L1, 12 L2, 2 L3) and 57x more learning edges did not translate to better vector similarity scores.

### Hypothesis for No Improvement

1. **Retrieval score ≠ Answer quality**: Raw retrieval scores measure embedding similarity, not whether the retrieved content contains the correct answer.

2. **Hidden nodes may help graph traversal**: The benefit of hidden nodes may appear in multi-hop queries and concept aggregation, not single-query similarity.

3. **Learning edges need activation**: CO_ACTIVATED_WITH edges improve retrieval via activation spread during inference, which may require the model to actually reason about results.

---

## Detailed Results

### Overall Statistics

| Metric | Value |
|--------|-------|
| Mean Score | 0.6196 |
| Median Score | 0.6133 |
| Std Dev | 0.0746 |
| Min Score | 0.4780 |
| Max Score | 0.7939 |
| CV (Coefficient of Variation) | 12.0% |

### Score Distribution

| Range | Count | Percentage |
|-------|-------|------------|
| High (>0.7) | 14 | 14.0% |
| Medium (0.5-0.7) | 85 | 85.0% |
| Low (<0.5) | 1 | 1.0% |

---

## Category Analysis

### Top 5 Categories

| Category | Mean Score | Count | High (>0.7) |
|----------|------------|-------|-------------|
| data_architecture | 0.671 | 1 | 0 |
| configuration_infrastructure | 0.665 | 9 | 4 |
| safety_security | 0.649 | 1 | 0 |
| api_services | 0.642 | 10 | 2 |
| system_architecture | 0.641 | 3 | 0 |

### Bottom 5 Categories

| Category | Mean Score | Count | High (>0.7) |
|----------|------------|-------|-------------|
| ui_ux | 0.583 | 10 | 0 |
| control_loop_architecture | 0.584 | 9 | 0 |
| integration | 0.589 | 2 | 0 |
| system_purpose | 0.592 | 1 | 0 |
| security_authentication | 0.606 | 7 | 0 |

### Persistent Weakness: UI/UX

UI/UX category remains the weakest at 0.583 (was 0.50 in graded scoring). This suggests:

- React/TypeScript component indexing needs improvement
- UI-specific terminology may need specialized embeddings
- Component hierarchy relationships not captured in current schema

---

## Environment

### MDEMG Configuration

| Field | Value |
|-------|-------|
| MDEMG Commit | `4e6dd73d5060339dc18bf7e45122435c0cba8cc0` |
| Space ID | plc-gbt |
| Query Params | candidate_k=50, top_k=10, hop_depth=2 |
| RERANK_ENABLED | true |
| RERANK_WEIGHT | 0.15 |

### Graph State

| Metric | Value |
|--------|-------|
| Total Nodes | 54,016 |
| Layer 0 | 53,598 |
| Layer 1 | 404 |
| Layer 2 | 12 |
| Layer 3 | 2 |
| Learning Edges | 13,170 |
| Embedding Coverage | 99.99% |

### Question Set

| Field | Value |
|-------|-------|
| File | test_questions_v2_selected.json |
| SHA-256 | `9126177df9be10163916277bd6fecdecaad954f51506ceb420bde007e32db944` |
| Total Questions | 100 |
| Seed | 42 |

---

## Artifacts

| File | Description |
|------|-------------|
| `/tmp/mdemg_benchmark_v2.jsonl` | Per-question results (100 records) |
| `/tmp/mdemg_benchmark_v2_summary.json` | Aggregate statistics |
| `preflight_receipt_v2_post_consolidation.md` | Pre-test environment capture |

---

## Recommendations

### Immediate Actions

1. **Run graded benchmark**: Use LLM to grade MDEMG-retrieved content against expected answers (not just similarity scores)

2. **Test activation spread**: Query with `include_learning_edges=true` to measure if learning edges improve results

3. **Increase hop_depth**: Test hop_depth=3 to see if hidden layer nodes provide better multi-hop retrieval

### Longer Term

1. **Symbol-aware retrieval**: Integrate symbol nodes for evidence-locked questions
2. **UI/UX module**: Create specialized ingestion for React components
3. **Evidence tracking**: Add ECR/HVRR metrics to benchmark pipeline

---

## Conclusion

Consolidation successfully built 418 hidden layer nodes and 13,170 learning edges, but **raw retrieval similarity scores did not improve**. This suggests the next benchmark should measure **answer quality** (graded by LLM against expected answers) rather than raw embedding similarity.

The scientific baseline is now established:

- **Mean retrieval score: 0.6196** (post-consolidation)
- **UI/UX remains the weakest category** (0.583)
- **99% of queries return scores >0.5** (consistent, no failures)

---

*Generated: 2026-01-26*
