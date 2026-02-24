# Baseline Benchmark Report - WHK-WMS

**Date:** 2026-01-23
**Test Type:** Baseline (Context Window Only)
**Codebase:** whk-wms
**Question Set:** test_questions_120_agent.json (120 questions)

---

## Executive Summary

| Metric | Value |
|--------|-------|
| **Questions Attempted** | 60/120 (50%) |
| **Symbol-Lookup Accuracy** | 20/20 (100%) |
| **Complex Questions** | 40/100 (40%) |
| **Time Elapsed** | 18 minutes |
| **Time Limit** | 20 minutes |
| **Disqualified** | No |

---

## Core Outcome Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| Completion Rate | 50.0% | 60/120 questions |
| Symbol-Lookup Completion | 100.0% | 20/20 - prioritized |
| Complex Completion | 40.0% | 40/100 - time limited |
| High-Confidence Rate (≥0.7) | 33.3% | 20/60 (symbol-lookup only) |

---

## Grounding & Evidence Metrics

### Symbol-Lookup Questions (Tier 2 - Evidence-Locked)

| Metric | Value |
|--------|-------|
| **Evidence Compliance Rate (ECR)** | 100% |
| **Evidence Correctness Rate (E-Accuracy)** | 100% |
| **Hard-Value Resolution Rate (HVRR)** | 100% |
| **Hallucination Rate** | 0% |
| **Guess Rate** | 0% |

All 20 symbol-lookup questions were answered with:

- ✓ Exact file paths
- ✓ Line numbers
- ✓ Exact values matching expected answers

### Symbol-Lookup Detail

| ID | Symbol | Expected | Found | File | Correct |
|----|--------|----------|-------|------|---------|
| sym_arch_1 | DEFAULT_MAX_COMPLEXITY | 1000 | 1000 | app.module.ts:217 | ✓ |
| sym_arch_2 | MAX_TAKE | 1000 | 1000 | pagination.constants.ts:2 | ✓ |
| sym_arch_3 | ID_CHUNK_SIZE | 5000 | 5000 | barrel-aggregates.service.ts:27 | ✓ |
| sym_arch_4 | MAX_BARRELS_FOR_AGGREGATION | 100000 | 100000 | barrel-aggregates.service.ts:21 | ✓ |
| sym_svc_1 | DEFAULT_SLOW_QUERY_THRESHOLD_MS | 1000 | 1000 | prisma.service.ts:42 | ✓ |
| sym_svc_2 | EMBEDDING_BATCH_SIZE | 50 | 50 | llama.service.ts:1196 | ✓ |
| sym_svc_3 | BATCH_DELAY_MS | 50 | 50 | llama.service.ts:818 | ✓ |
| sym_svc_4 | STARTUP_HEALTH_CHECK_DELAY_MS | 60000 | 60000 | indexing.scheduler.ts:9 | ✓ |
| sym_biz_1 | MAX_EXPORT_SIZE | 50000 | 50000 | barrel.controller.ts:598 | ✓ |
| sym_biz_2 | MAX_SORT_RECORDS | 10000 | 10000 | transfer.service.ts:33 | ✓ |
| sym_biz_3 | MAX_QUERY_SIZE | 10000 | 10000 | barrel.resolver.ts:647 | ✓ |
| sym_biz_4 | MAX_JOB_ID_FILTER_COUNT | 100 | 100 | warehouse-jobs.controller.ts:56 | ✓ |
| sym_data_1 | BATCH_SIZE | 10 | 10 | trust-mode-processing.service.ts:127 | ✓ |
| sym_data_2 | MAX_EMPTY_BATCHES | 3 | 3 | inventory-processing.service.ts:560 | ✓ |
| sym_data_3 | MAX_EXPORT_RECORDS | 100000 | 100000 | serverUtils.ts:1785 | ✓ |
| sym_data_4 | ALLOWED_ERROR_TYPES | [...] | [...] | deviceSyncError.service.ts:5-10 | ✓ |
| sym_cross_1 | CACHE_EXPIRATION_MS | 14400000 | 14400000 | EventTypesContext.tsx:26 | ✓ |
| sym_cross_2 | MAX_RETRY_ATTEMPTS | 3 | 3 | chatBotFallback.ts:25 | ✓ |
| sym_cross_3 | RETRY_DELAYS | [1000,3000,5000] | [1000,3000,5000] | chatBotFallback.ts:26 | ✓ |
| sym_cross_4 | HEALTH_CHECK_TIMEOUT | 5000 | 5000 | chatBotFallback.ts:24 | ✓ |

---

## Performance by Category

| Category | Attempted | Symbol-Lookup | Complex |
|----------|-----------|---------------|---------|
| architecture_structure | 14 | 4/4 | 10 |
| service_relationships | 9 | 4/4 | 5 |
| business_logic_constraints | 9 | 4/4 | 5 |
| data_flow_integration | 8 | 4/4 | 4 |
| cross_cutting_concerns | 10 | 4/4 | 6 |
| **Total** | **60** | **20/20** | **40** |

---

## Performance by Complexity Type

| Complexity | Total | Attempted | Completion % |
|------------|-------|-----------|--------------|
| symbol-lookup | 20 | 20 | 100.0% |
| multi-file | 59 | 25 | 42.4% |
| cross-module | 38 | 13 | 34.2% |
| system-wide | 3 | 2 | 66.7% |

---

## Constraint Compliance

| Rule | Status |
|------|--------|
| Time limit (20 min) | ✓ Respected (18 min) |
| No web search | ✓ Complied |
| WHK-WMS repo only | ✓ Complied |
| Tools used | Read, Glob, Grep |
| Disqualified | No |

---

## Observations

### Strengths

1. **Symbol-lookup perfect accuracy** - All 20 evidence-locked questions answered correctly with file:line references
2. **Time management** - Completed within limit by prioritizing high-value questions
3. **Evidence quality** - All answers grounded with specific file references

### Limitations

1. **Completion rate** - Only 50% of questions attempted due to time constraint
2. **Complex questions** - 40% coverage on multi-file/cross-module questions
3. **No MDEMG advantage** - Pure file search without semantic retrieval

---

## Comparison Baseline

This establishes the baseline for MDEMG comparison:

| Metric | Baseline | MDEMG Target |
|--------|----------|--------------|
| Completion Rate | 50% | >80% |
| Symbol-Lookup HVRR | 100% | 100% |
| Evidence Compliance | 100% | 100% |
| Time to Complete | 18 min | <15 min |

---

## Raw Data

Full results available in: `baseline_benchmark_results_20260123.json`

---

*Report generated: 2026-01-23*
