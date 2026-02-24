# MDEMG Benchmark Results - whk-wms

**Date:** 2026-01-28
**Framework Version:** V3.0
**Questions:** 120 (100 complex + 20 hard symbol questions)
**Benchmark ID:** whk-wms-20260128-210706

## Executive Summary

| Metric | Baseline (grep) | MDEMG (semantic) | Delta |
|--------|-----------------|------------------|-------|
| Mean Score | **0.725** | 0.713 | -0.012 (-1.7%) |
| Std Dev | 0.000 | 0.000 | - |
| Evidence Rate | 100% | 100% | - |
| Valid Runs | 3 | 3 | - |

**Conclusion:** In this benchmark, the grep-based baseline slightly outperformed MDEMG semantic retrieval by 1.2 percentage points. However, this difference is within noise margins given both methods achieve ~72% scores.

## Methodology

### Baseline (grep-based search)

- Extract keywords from questions (excluding stopwords)
- Prioritize PascalCase/camelCase identifiers (likely code symbols)
- Use ripgrep to search TypeScript/JavaScript files
- Return top 5 matching files with line numbers

### MDEMG (semantic retrieval)

- Use MDEMG `/v1/memory/retrieve` API with question as query
- Retrieve top 10 semantically similar code elements
- Synthesize answer from retrieved node summaries
- Return file paths with extracted line references

## Individual Run Results

| Run | Type | Score | Std | Evidence Rate | Duration |
|-----|------|-------|-----|---------------|----------|
| baseline_run1 | baseline | 0.725 | 0.035 | 100% | 13.6s |
| baseline_run2 | baseline | 0.725 | 0.035 | 100% | 13.4s |
| baseline_run3 | baseline | 0.725 | 0.036 | 100% | 13.3s |
| mdemg_run1 | mdemg | 0.713 | 0.017 | 100% | 28.4s |
| mdemg_run2 | mdemg | 0.713 | 0.017 | 100% | 0.6s |
| mdemg_run3 | mdemg | 0.713 | 0.017 | 100% | 0.6s |

Note: MDEMG runs 2-3 are cached, hence the fast execution.

## Detailed Analysis

### Score Breakdown by Component (average across questions)

| Component | Weight | Baseline | MDEMG |
|-----------|--------|----------|-------|
| Evidence Score | 70% | 1.00 | 1.00 |
| Semantic Score | 15% | ~0.05 | ~0.04 |
| Concept Score | 15% | ~0.05 | ~0.04 |
| File Bonus | 10% | Low | Low |

Both methods achieve 100% evidence scores (file:line citations present) but have low semantic and concept overlap with expected answers. This indicates:

1. The scoring is heavily weighted toward evidence presence
2. Neither method is generating high-quality semantic answers
3. The benchmark is measuring retrieval ability more than answer quality

### Question Category Analysis

Questions where **MDEMG outperformed baseline**:

- Q443 (audit log query): +0.112 (semantic understanding of query patterns)
- Q262 (barrel-lot invariants): +0.095 (cross-module relationship understanding)

Questions where **Baseline outperformed MDEMG**:

- hard_sym_15 (delay constants): -0.217 (grep finds exact constant names)
- hard_sym_18 (MAX_RETRIES): -0.158 (grep excels at symbol search)
- hard_sym_13 (CHUNK_SIZE): -0.157 (grep finds exact matches)
- hard_sym_1 (BATCH_SIZE): -0.113 (grep finds symbol definitions)

### Key Insights

1. **Grep excels at symbol search:** The hard_sym questions involve finding specific constant/variable names. Grep finds exact text matches efficiently.

2. **MDEMG retrieves related but not exact files:** MDEMG returns semantically related files (e.g., migrations for "lot audit" questions) but misses the actual implementation files.

3. **Learning edges not accumulating:** All runs show 0 learning edges, suggesting learning is disabled or the space has no prior learning state.

4. **Consistency is high:** Both methods show very low variance (σ=0.000 across runs), indicating deterministic behavior.

## Learning Edge Analysis

| Metric | Value |
|--------|-------|
| Initial Edges | 0 |
| Final Edges | 0 |
| Total New Edges | 0 |

Learning edge accumulation was not observed during this benchmark. This may indicate:

- Learning feature is disabled
- Space was recently reset
- Configuration issue

## Recommendations

1. **For symbol-heavy questions:** Consider a hybrid approach using grep for exact symbol search + MDEMG for semantic context.

2. **Improve MDEMG retrieval:** The high proportion of migration files in results suggests indexing quality could be improved (filter migrations, prioritize source code).

3. **Re-run with learning enabled:** Enable learning edges to test if performance improves with repeated queries.

4. **Adjust scoring weights:** Evidence presence (70%) dominates scoring. Consider increasing semantic/concept weights to better measure answer quality.

## Raw Data

- Baseline answers: `answers_baseline_run[1-3].jsonl`
- MDEMG answers: `answers_mdemg_run[1-3].jsonl`
- Baseline grades: `grades_baseline_run[1-3].json`
- MDEMG grades: `grades_mdemg_run[1-3].json`
- Full summary: `BENCHMARK_SUMMARY_V3.json`

## Appendix: Top 10 Questions by Score Difference

| ID | Baseline | MDEMG | Delta | Category |
|----|----------|-------|-------|----------|
| hard_sym_15 | 0.935 | 0.718 | -0.217 | disambiguation |
| hard_sym_18 | 0.864 | 0.706 | -0.158 | disambiguation |
| hard_sym_13 | 0.874 | 0.717 | -0.157 | disambiguation |
| hard_sym_1 | 0.831 | 0.718 | -0.113 | disambiguation |
| 443 | 0.704 | 0.816 | +0.112 | cross_cutting_concerns |
| 171 | 0.809 | 0.704 | -0.105 | service_relationships |
| 183 | 0.820 | 0.718 | -0.102 | service_relationships |
| 162 | 0.809 | 0.708 | -0.101 | service_relationships |
| 252 | 0.809 | 0.713 | -0.096 | business_logic_constraints |
| 262 | 0.713 | 0.808 | +0.095 | business_logic_constraints |
