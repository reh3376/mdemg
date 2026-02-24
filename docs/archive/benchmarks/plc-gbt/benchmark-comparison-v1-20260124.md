# PLC-GBT Benchmark Comparison Report v1

**Date:** 2026-01-24
**Codebase:** plc-gbt (815K LOC, 12,376 files)
**Question Set:** 100 questions (randomly selected from 115, seed=42)
**Model:** Claude Haiku (both tests)

---

## Executive Summary

| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| **Completion Rate** | 26% | 100% | +74% |
| **Questions Answered** | 26/100 | 100/100 | +74 |
| **Avg Score (all questions)** | 0.245* | 0.615 | +0.370 |
| **Avg Score (attempted only)** | 0.942 | 0.615 | -0.327 |
| **Perfect Answers (1.0)** | ~24 (est) | 23 | ~same |
| **Partial Answers (0.5)** | ~2 (est) | 77 | +75 |
| **Wrong/Unanswered** | 74 | 0 | -74 |

*Baseline score calculated as: (26 × 0.942 + 74 × 0) / 100 = 0.245

---

## Key Findings

### 1. Completion Rate: MDEMG Wins Decisively

The baseline approach could only answer **26%** of questions within the time limit before context window exhaustion. MDEMG achieved **100% completion** by enabling targeted retrieval without requiring full codebase ingestion.

### 2. Accuracy Trade-off

When the baseline *could* answer, it achieved very high accuracy (0.942 avg). MDEMG's lower per-question accuracy (0.615) reflects the challenge of retrieving precisely relevant context. However, the net improvement is significant: **total score improved from 0.245 to 0.615** (+151%).

### 3. Zero Wrong Answers

MDEMG produced **0 completely wrong answers** (all scores were either 1.0 or 0.5). This indicates the retrieval system consistently returned relevant context, even if not perfectly aligned with expected answers.

---

## Detailed Results

### Baseline Performance (26 questions in ~18 min)

| Category | Avg Score | Count |
|----------|-----------|-------|
| api_services | 1.00 | 5 |
| configuration_infrastructure | 1.00 | 3 |
| control_loop_architecture | 0.95 | 6 |
| data_models_schema | 0.90 | 4 |
| database_persistence | 1.00 | 2 |
| ui_ux | 0.83 | 4 |
| ai_ml_integration | 1.00 | 2 |

**Time efficiency:** ~0.7 questions/minute (limited by context window, not model speed)

### MDEMG Performance (100 questions)

| Category | Avg Score | Count |
|----------|-----------|-------|
| api_services | 0.75 | 10 |
| configuration_infrastructure | 0.67 | 9 |
| control_loop_architecture | 0.56 | 9 |
| data_models_schema | 0.67 | 9 |
| database_persistence | 0.62 | 4 |
| ui_ux | 0.50 | 10 |
| ai_ml_integration | 0.61 | 9 |
| acd_l5x_conversion | 0.60 | 10 |
| business_logic_workflows | 0.62 | 8 |
| n8n_workflow | 0.67 | 6 |
| security_authentication | 0.57 | 7 |
| system_architecture | 0.50 | 3 |
| system_purpose | 0.50 | 1 |
| data_architecture | 1.00 | 1 |
| integration | 0.50 | 2 |
| safety_security | 0.50 | 1 |
| ui_architecture | 0.50 | 1 |

**Avg Similarity Score:** 0.649 (cosine similarity between retrieved context and expected answer)

---

## Analysis

### Categories Where Baseline Excelled

The baseline achieved near-perfect scores in categories it could reach:

- **api_services**: 1.00 (baseline) vs 0.75 (MDEMG)
- **configuration_infrastructure**: 1.00 vs 0.67
- **database_persistence**: 1.00 vs 0.62

This suggests these categories have concentrated, easily-greppable content.

### Categories Unique to MDEMG

The baseline never attempted these categories (ran out of time):

- acd_l5x_conversion (0.60 avg) - 10 questions
- n8n_workflow (0.67 avg) - 6 questions
- security_authentication (0.57 avg) - 7 questions
- business_logic_workflows (0.62 avg) - 8 questions

### Areas for MDEMG Improvement

Lower scores in certain categories suggest retrieval gaps:

- **ui_ux**: 0.50 avg - may need better component path embedding
- **control_loop_architecture**: 0.56 avg - complex nested JSON schemas
- **integration**: 0.50 avg - cross-system context needed

---

## Conclusions

### MDEMG Value Proposition Validated

For large codebases (815K LOC), MDEMG provides:

1. **100% question completion** vs 26% baseline
2. **151% improvement in total score** (0.245 → 0.615)
3. **Zero completely wrong answers**

### Trade-offs Observed

1. Per-question accuracy drops when using retrieval vs full context
2. Some categories (UI, complex schemas) need retrieval improvements
3. Baseline excels when it can reach content (but can't scale)

### Recommendations

1. **Run consolidation** to build hidden layer nodes (currently 0)
2. **Tune RERANK_WEIGHT** to improve precision (currently 0.15)
3. **Add category-specific retrieval strategies** for UI components and JSON schemas

---

## Test Environment

| Field | Value |
|-------|-------|
| MDEMG Commit | dcb355eeb3a87c51f9b262da295fc3b46087e6b5 |
| plc-gbt Commit | 2f761094afe5df46ca9ea1a96bfc1c8dba303084 |
| Hardware | Apple M4 Max, 64GB RAM |
| Neo4j Version | 5.x (Docker) |
| Memory Nodes | 53,598 |
| Embedding Coverage | 99.99% |

---

*Generated: 2026-01-24*
