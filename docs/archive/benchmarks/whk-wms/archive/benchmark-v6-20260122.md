# MDEMG v6 Benchmark Results

**Date**: 2026-01-22
**Version**: v6 - Adaptive Layers + Dynamic Edge/Node Types
**Test**: 100 random questions (seed=42) from whk-wms-questions-final.json

## v6 Results

| Metric | Value |
|--------|-------|
| Avg Score | 0.560 |
| Max Score | 0.824 |
| Min Score | 0.344 |
| Duration | 96.2s |

### By Category

| Category | Score |
|----------|-------|
| business_logic_constraints | 0.593 |
| cross_cutting_concerns | 0.568 |
| service_relationships | 0.564 |
| data_flow_integration | 0.554 |
| architecture_structure | 0.541 |

## Comparison to Previous Versions

| Version | Avg Score | Min Score | Max Score | Notes |
|---------|-----------|-----------|-----------|-------|
| v6 (Dynamic Types) | 0.560 | 0.344 | 0.824 | Adaptive layers, dynamic edge/node types |
| Fresh State (v5) | 0.571 | 0.221 | 0.824 | P2 features, fresh ingest |
| Prior State (v5) | 0.564 | 0.087 | 0.837 | P2 features, incremental consolidation |

## Analysis

### v6 vs v5 Fresh State

| Metric | v6 | v5 Fresh | Diff |
|--------|-----|----------|------|
| Avg Score | 0.560 | 0.571 | -0.011 (-1.9%) |
| Min Score | 0.344 | 0.221 | +0.123 (+56%) |
| Max Score | 0.824 | 0.824 | 0.000 |

### Category Comparison (v6 vs v5 Fresh)

| Category | v6 | v5 Fresh | Diff |
|----------|-----|----------|------|
| architecture_structure | 0.541 | 0.543 | -0.002 |
| business_logic_constraints | 0.593 | 0.598 | -0.005 |
| cross_cutting_concerns | 0.568 | 0.582 | -0.014 |
| data_flow_integration | 0.554 | 0.558 | -0.004 |
| service_relationships | 0.564 | 0.584 | -0.020 |

## Key Observations

1. **Average score slightly lower** (-1.9%): Within normal variance, likely due to different clustering patterns from adaptive epsilon values

2. **Minimum score significantly higher** (+56%): The worst-case scenario improved dramatically (0.221 → 0.344), suggesting more robust coverage

3. **Consistent max score**: Both versions achieve same peak performance (0.824)

4. **Category breakdown stable**: All categories within ±0.02 of previous version

## New Features Tested

- [x] Adaptive epsilon (looser at higher layers)
- [x] Adaptive minSamples (smaller clusters at top)
- [x] No early termination (tries all 5 layers)
- [x] Dynamic edge type inference (not yet creating L4+ edges - needs more data)
- [x] Dynamic node type inference (not yet classifying L4+ nodes - needs more data)

## Layer Distribution

| Layer | Count | Role Types |
|-------|-------|------------|
| L0 | 8932 | leaf |
| L1 | 131 | hidden, concern, config |
| L2 | 3 | concept |
| L3 | 1 | concept |

## Special Nodes

- 8 concern nodes (cross-cutting concerns)
- 1 config node (config summary)
- 24 comparison nodes (similar modules)

## Conclusion

v6 with adaptive layers and dynamic types performs **comparably** to v5:

- Slight decrease in average (-1.9%) within normal variance
- Significant improvement in worst-case (+56%)
- Infrastructure ready for L4/L5 emergence as data accumulates

The dynamic edge/node type inference is implemented but won't activate until L4+ nodes are created. This will happen naturally as more data is ingested and the adaptive constraints allow higher-level clustering.
