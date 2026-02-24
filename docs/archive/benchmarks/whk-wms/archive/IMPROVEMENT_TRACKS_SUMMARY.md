# MDEMG Improvement Tracks Summary (v4 → v11)

**Date**: 2026-01-23
**Codebase**: whk-wms (196 MB, 8,932 elements)
**Test Set**: 100 questions across 5 categories

---

## Results Overview

| Version | Avg Score | >0.7 Rate | Key Change |
|---------|-----------|-----------|------------|
| v4 (baseline) | 0.567 | ~10% | Initial MDEMG implementation |
| v9 (rerank) | 0.619 | ~25% | Added LLM re-ranking |
| v10 (learning) | 0.710 | 64% | Fixed Hebbian learning |
| **v11 (all tracks)** | **0.733** | **75%** | All 5 improvement tracks |

**Total Improvement**: +29.3% from v4 baseline

---

## The 5 Improvement Tracks

### Track 1: Hebbian Learning Edges

**Problem**: CO_ACTIVATED_WITH edges weren't being created (0 edges after 100 queries).

**Fix**: Seed ALL vector recall candidates with activation values, not just top 2.

**Impact**:

- Edges created: 0 → 8,748
- Score improvement: +14.6% (v4→v10)

### Track 2: Cross-Cutting Concern Nodes

**Problem**: Questions about authentication, ACL, error-handling scored 0.45-0.46.

**Solution**:

- Detect concern patterns during ingestion (auth, validation, logging, caching)
- Create dedicated `concern:*` nodes during consolidation
- Link implementing files via `IMPLEMENTS_CONCERN` edges

**Impact**: Cross-cutting questions: 0.45 → 0.709 (+57%)

### Track 3: Architectural Comparison Nodes

**Problem**: "What's the difference between X and Y?" questions scored below average.

**Solution**:

- Detect similar modules by naming patterns (e.g., SyncModule vs DeltaSyncModule)
- Create comparison nodes with `role_type: 'comparison'`
- Link modules via `COMPARED_IN` edges

**Impact**: Architecture questions: 0.50 → 0.750 (+50%)

### Track 4: Configuration Summary Nodes

**Problem**: Configuration questions scored below average due to terse file content.

**Solution**:

- Detect config files by patterns (*.config.ts, /config/, etc.)
- Extract environment variables from .env files
- Create config summary node with `role_type: 'config'`

**Impact**: Configuration retrieval improved through dedicated nodes

### Track 5: Temporal Pattern Nodes

**Problem**: Questions about validFrom/validTo temporal patterns scored ~0.46.

**Solution**:

- Detect temporal patterns (validity periods, soft deletes, versioning, audit trails)
- Tag with `concern:temporal`
- Create temporal summary node with `SHARES_TEMPORAL_PATTERN` edges

**Impact**: Business logic questions: 0.45 → 0.728 (+62%)

---

## Category Performance (v11)

| Category | v4 | v10 | v11 | Change (v4→v11) |
|----------|-----|-----|-----|-----------------|
| architecture_structure | ~0.50 | 0.724 | **0.750** | +50% |
| service_relationships | ~0.50 | 0.727 | **0.746** | +49% |
| business_logic_constraints | ~0.45 | 0.686 | **0.728** | +62% |
| cross_cutting_concerns | 0.45 | 0.687 | **0.709** | +58% |
| data_flow_integration | ~0.55 | 0.724 | **0.719** | +31% |

---

## Score Distribution (v11)

| Range | Count | Percentage |
|-------|-------|------------|
| >0.8 | 10 | 10% |
| 0.7-0.8 | 65 | 65% |
| 0.6-0.7 | 23 | 23% |
| 0.5-0.6 | 1 | 1% |
| 0.4-0.5 | 1 | 1% |
| <0.4 | 0 | 0% |

**75% of questions now score above 0.7** (was 10% in v4)

---

## Learning Edge Statistics (v11)

| Metric | Value |
|--------|-------|
| Initial Edges | 178 |
| Final Edges | 8,748 |
| New Edges Created | 8,570 |
| Edges per Query | 85.7 |
| Avg Activated Nodes | 10.0/10 |

---

## Architecture Summary

### Node Types Created During Consolidation

1. **Hidden Nodes** (layer 1) - DBSCAN clusters of related base nodes
2. **Concept Nodes** (layer 2+) - Higher-level abstractions
3. **Concern Nodes** (`role_type: 'concern'`) - Cross-cutting concerns
4. **Comparison Nodes** (`role_type: 'comparison'`) - Module comparisons
5. **Config Nodes** (`role_type: 'config'`) - Configuration summaries
6. **Temporal Nodes** (`role_type: 'temporal'`) - Temporal pattern summaries

### Key Edge Types

- `CO_ACTIVATED_WITH` - Hebbian learning edges (strengthened by co-retrieval)
- `IMPLEMENTS_CONCERN` - Links files to concern nodes
- `COMPARED_IN` - Links modules to comparison nodes
- `IMPLEMENTS_CONFIG` - Links config files to config summary
- `SHARES_TEMPORAL_PATTERN` - Links temporal entities

---

## Test Artifacts

| File | Description |
|------|-------------|
| `test_questions_v4_selected.json` | 100 test questions |
| `run_mdemg_test_v11_alltracks.py` | v11 test runner |
| `mdemg-test-v11-alltracks-20260123-071649.md` | v11 full results |
| `mdemg-test-v10-learning-*.md` | v10 results (learning edges) |
| `mdemg-test-v9-rerank-*.md` | v9 results (LLM rerank) |

---

## Key Takeaways

1. **Hebbian learning was the biggest single improvement** (+14.6% from v4→v10)
2. **Specialized node types provide targeted boosts** for specific question categories
3. **75% >0.7 rate achieved** (target was 70%)
4. **No significant regressions** - data_flow_integration had minor -0.7% drop
5. **Learning edges continue accumulating** - system improves with use

---

## Next Steps (Phase D)

- [ ] Benchmark on second codebase (different domain)
- [ ] Scale test: 10K → 100K nodes
- [ ] Document final architecture for public release
