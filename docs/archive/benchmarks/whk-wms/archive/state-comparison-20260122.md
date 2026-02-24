# MDEMG State Comparison: Current vs Fresh Consolidated State

**Date**: 2026-01-22
**Purpose**: Validate if running consolidate on existing state produces same results as fresh ingestion + consolidation

## Methodology

1. **Test A**: Run 100 questions on current state (multiple consolidations applied)
2. **Test B**: Delete all data, re-ingest codebase, consolidate fresh, run same 100 questions

Both tests used the same 100 questions (random seed=42) from whk-wms-questions-final.json.

## Results Summary

| Metric | Test A (Prior State) | Test B (Fresh State) | Diff |
|--------|---------------------|---------------------|------|
| Avg Score | 0.564 | 0.571 | +0.007 (+1.2%) |
| Max Score | 0.837 | 0.824 | -0.013 |
| Min Score | 0.087 | 0.221 | +0.134 (+154%) |

## By Category

| Category | Test A | Test B | Diff |
|----------|--------|--------|------|
| architecture_structure | 0.534 | 0.543 | +0.009 |
| business_logic_constraints | 0.610 | 0.598 | -0.011 |
| cross_cutting_concerns | 0.570 | 0.582 | +0.013 |
| data_flow_integration | 0.557 | 0.558 | +0.002 |
| service_relationships | 0.575 | 0.584 | +0.008 |

## Key Findings

1. **Average score similar**: Fresh state performs 1.2% better overall - within acceptable variance
2. **Min score significantly better on fresh**: The worst-performing question improved from 0.087 to 0.221
3. **Individual question variance**: Some questions showed large differences (up to ±0.32)
4. **Cross-cutting concerns benefit from fresh state**: +0.013 improvement

## Questions with Largest Differences

| Question | Test A | Test B | Diff |
|----------|--------|--------|------|
| Q14 | 0.137 | 0.456 | +0.319 |
| Q179 | 0.698 | 0.454 | -0.244 |
| Q56 | 0.470 | 0.697 | +0.227 |
| Q383 | 0.817 | 0.621 | -0.196 |
| Q75 | 0.659 | 0.473 | -0.186 |

## Conclusion

Fresh state and current state produce **comparable results** with fresh state having a **slight edge**:

- +1.2% average score improvement
- +154% minimum score improvement (better worst-case coverage)

The differences are likely due to:

1. Hidden layer clustering variations based on data order
2. Comparison node detection differences
3. Embedding cache state

**Recommendation**: For production testing, fresh ingestion + consolidation provides slightly better results, especially for worst-case scenarios.
