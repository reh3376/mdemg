# MDEMG Context Retention Experiment - Comparison Report v2

**Date:** 2026-01-21
**Codebase:** whk-wms (Whiskey House Warehouse Management System)
**Questions:** 100 (20 per category, verified answers)

---

## Executive Summary

| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| **Total Score** | 85.5/100 | 42.5/100 | **-43.0** |
| **Average Score** | 0.855 | 0.425 | **-0.430** |
| Completely Correct (1.0) | 74 | ~15 | -59 |
| Partially Correct (0.5) | 23 | ~55 | +32 |
| Unable to Answer (0.0) | 3 | ~30 | +27 |
| Confidently Wrong (-1.0) | 0 | 0 | 0 |

**Result: Baseline significantly outperformed MDEMG by 43 points.**

---

## Score by Category

| Category | Baseline | MDEMG | Delta |
|----------|----------|-------|-------|
| architecture_structure | 17.5/20 (0.875) | 9.5/20 (0.475) | -8.0 |
| service_relationships | 17.0/20 (0.850) | 8.0/20 (0.400) | -9.0 |
| business_logic_constraints | 16.5/20 (0.825) | 8.5/20 (0.425) | -8.0 |
| data_flow_integration | 17.0/20 (0.850) | 8.5/20 (0.425) | -8.5 |
| cross_cutting_concerns | 17.5/20 (0.875) | 8.0/20 (0.400) | -9.5 |

---

## Key Findings

### 1. MDEMG Retrieval Limitations

The MDEMG test revealed critical gaps in the current implementation:

| Issue | Impact | Severity |
|-------|--------|----------|
| **Empty summaries** | Cannot understand file content from metadata alone | Critical |
| **No content retrieval** | Only file paths returned, not actual code | Critical |
| **Missing cross-module linking** | Cannot trace data flows across services | High |
| **No specific value retrieval** | Cannot answer questions about constants/configs | High |

### 2. What MDEMG Does Well

- **File discovery**: High vector similarity scores (0.882 avg)
- **Module grouping**: Successfully identifies related modules
- **Pattern detection**: Hidden layer nodes capture cross-cutting patterns
- **Documentation retrieval**: Finds relevant markdown files

### 3. Why Baseline Won

The baseline agent could:

- Read actual file contents
- Trace imports and dependencies
- Find specific function implementations
- Verify exact values and constants

### 4. Root Cause Analysis

The MDEMG API returns:

```json
{
  "nodes": [
    {
      "id": "...",
      "path": "/src/barrel/barrel.service.ts",
      "name": "barrel.service.ts",
      "summary": "",  // <-- EMPTY
      "vector_sim": 0.89,
      "activation": 0.52
    }
  ]
}
```

Without populated `summary` fields or content retrieval, the agent cannot:

- Understand what each file does
- Answer implementation-specific questions
- Trace code flows

---

## Recommendations

### Immediate Fixes (Critical)

1. **Populate summaries during ingestion**
   - Generate file/function summaries using LLM
   - Store in MemoryNode.summary field
   - Include key exports, dependencies, purpose

2. **Add content retrieval endpoint**
   - New endpoint: `/v1/memory/content?node_id=X`
   - Returns actual file content or relevant snippets
   - Supports RAG workflow: retrieve → read → answer

3. **Improve relationship edges**
   - Track imports/exports between files
   - Add CALLS, IMPORTS, DEPENDS_ON edges
   - Enable cross-module flow tracing

### Medium-Term Improvements

1. **Enhance hidden layer**
   - Cluster by semantic similarity + structural relationships
   - Generate cluster summaries
   - Use for high-level architecture questions

2. **Add observation tracking**
   - Record which files are accessed together
   - Strengthen CO_ACTIVATED_WITH edges
   - Improve retrieval for related code

3. **Implement semantic chunking**
   - Break files into function-level nodes
   - Store function signatures and docstrings
   - Enable precise code retrieval

---

## Test Validity Notes

- **Questions verified**: All 100 questions used verified/corrected answers
- **Seed reproducibility**: Random seed 2026 for question selection
- **Fair comparison**: Same questions for both tests
- **Limitations**:
  - Single run (no statistical significance)
  - MDEMG service may not have full codebase indexed
  - Embedding quality not validated

---

## Conclusion

The current MDEMG implementation provides good **file discovery** but fails at **content understanding**. The 43-point gap is primarily due to:

1. Empty summary fields (no semantic understanding)
2. No content retrieval API (cannot read code)
3. Missing cross-module relationships (cannot trace flows)

**Next Steps:**

1. Implement summary generation during ingestion
2. Add content retrieval endpoint
3. Re-run comparison test

The hypothesis that MDEMG can match or exceed baseline performance remains **unproven** but **achievable** with the recommended fixes.
