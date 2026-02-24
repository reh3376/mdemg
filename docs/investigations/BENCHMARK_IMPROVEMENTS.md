# MDEMG Benchmark-Driven Improvements

**Generated:** 2026-01-27
**Source:** Blueseer ERP Benchmark v3 Analysis
**Status:** In Progress

## Overview

This document tracks improvements to MDEMG identified from the blueseer benchmark analysis. Two key documents informed these tasks:

1. `docs/tests/blueseer/BENCHMARK_RESULTS_V3.md` - Comparative analysis (MDEMG vs baseline)
2. `docs/tests/blueseer/BENCHMARK_COMPLETE_ANALYSIS.md` - Deep dive into MDEMG behavior

## Key Findings Summary

| Finding | Impact | Severity |
|---------|--------|----------|
| Confidence scores degrade 87% with learning edges | Score interpretation unreliable | HIGH |
| 4.8% no-evidence rate (vs 0% baseline) | Missing file citations | MEDIUM |
| Non-code files pollute code queries | Reduced precision | MEDIUM |
| No normalized confidence metrics | Cross-run comparison impossible | MEDIUM |
| Learning edge effects undocumented | User confusion | LOW |
| No learning phase freeze capability | Production instability | MEDIUM |

## Improvement Tasks

### Priority 1: Critical Issues

#### Task #1: Investigate Confidence Score Degradation

**Status:** COMPLETED
**Problem:** HIGH confidence dropped from 15.2% → 1.6% as learning edges accumulated (0 → 24,860).

**Root Cause Identified: ACTIVATION DILUTION**

The spreading activation algorithm (`internal/retrieval/activation.go:58-83`) accumulates activation from ALL incoming edges. As CO_ACTIVATED_WITH edges accumulate:

1. Activation spreads through many more pathways
2. Previously low-scoring nodes receive activation from multiple sources
3. Score distribution compresses toward the middle
4. Fixed confidence thresholds become harder to achieve

**Key Code Path:**

```go
// activation.go:66-71
for _, e := range ins {
    srcA := act[e.Src]
    w := effectiveWeight(e)
    acc += srcA * w  // Accumulates from ALL edges
}
```

**Scoring Formula Impact:**

- `score = α*vecSim + β*activation + ...`
- β = 0.30 (30% weight) - activation is significant
- No normalization applied after scoring

**Solution:** Implement percentile-based confidence (Task #2) instead of absolute thresholds.

**Full analysis:** See `docs/CONFIDENCE_SCORE_INVESTIGATION.md`

---

#### Task #5: Reduce No-Evidence Rate

**Status:** Pending
**Problem:** MDEMG returned 7/140 (4.8%) answers without file:line citations while baseline had 0%.

**Analysis needed:**

- Review failed questions in `answers_mdemg_run*.jsonl`
- Identify question categories with failures
- Check if hidden layer fallback is working

---

### Priority 2: Feature Enhancements

#### Task #2: Add Normalized Confidence Percentiles

**Status:** COMPLETED AND VERIFIED
**Goal:** Provide both raw scores and normalized percentiles for meaningful cross-run comparisons.

**BUG FIXED (2026-01-27):** Reasoning module and reranker were replacing results, losing normalized_confidence.
**Fix:** Added `ApplyNormalizedConfidenceToResults()` call AFTER all post-processing in service.go.
**Verification:** API now returns normalized_confidence (0-100 percentile) and confidence_level (HIGH/MEDIUM/LOW).

**Implementation:**

- Added `NormalizedConfidence` (0-100 percentile) and `ConfidenceLevel` (HIGH/MEDIUM/LOW) to `RetrieveResult`
- Added `ApplyNormalizedConfidence()` function in `internal/retrieval/scoring.go`
- Percentile formula: `100 * (n-1-rank) / (n-1)` where rank 0 = best
- Confidence levels: HIGH (≥90%), MEDIUM (≥40%), LOW (<40%)
- Added unit tests in `internal/retrieval/scoring_test.go`

**API response now includes:**

```json
{
  "score": 0.72,
  "normalized_confidence": 85.0,
  "confidence_level": "HIGH"
}
```

---

#### Task #3: Filter Non-Code Files

**Status:** COMPLETED
**Problem:** README.md, TRADEMARK_POLICY.md appearing in code-focused queries.

**Implementation:**

- Added `FileFilter` struct with `IncludeExtensions` and `ExcludeExtensions` fields
- Added `CodeOnly` convenience flag (excludes md, txt, json, yaml, yml, toml, xml, csv, lock, sum)
- Modified `vectorRecall()` and `BM25Search()` to accept and apply filters
- Filters applied at Cypher query level for efficiency
- Added unit tests in `internal/retrieval/filter_test.go`

**API usage:**

```json
{
  "query_text": "How does order processing work?",
  "space_id": "blueseer-erp",
  "code_only": true
}
// OR explicit filtering:
{
  "include_extensions": ["java", "go"],
  "exclude_extensions": ["md", "txt"]
}
```

---

#### Task #6: Score Distribution Monitoring

**Status:** Pending
**Goal:** Track score distributions to detect calibration drift in production.

**Metrics:**

- Score histogram per space
- Confidence level percentages
- Learning edge count at measurement time

---

#### Task #7: Learning Phase Freeze

**Status:** Pending
**Goal:** Allow freezing learning for production stability.

**API:**

```bash
mdemg space freeze <space_id>
```

---

### Priority 3: Documentation

#### Task #4: Document Learning Edge Effects

**Status:** Pending
**Goal:** Create `docs/LEARNING_EDGES.md` explaining score behavior across learning phases.

---

## Benchmark Metrics Reference

### Blueseer v2 Benchmark Results (140 questions, 6 runs total)

**Date:** 2026-01-27
**Full Analysis:** `docs/tests/blueseer/BENCHMARK_V2_ANALYSIS.md`

#### Individual Run Scores

| Run | Mean | Std | CV% | High Score Rate | Strong Evidence |
|-----|------|-----|-----|-----------------|-----------------|
| Baseline Run 1 | 0.791 | 0.205 | 26.0% | 93.6% | 93.6% |
| Baseline Run 2 | 0.849 | 0.071 | 8.3% | 100.0% | 100.0% |
| Baseline Run 3 | 0.825 | 0.100 | 12.1% | 96.4% | 96.4% |
| MDEMG Run 1* | 0.364 | 0.027 | 7.4% | 0.0% | 0.0% |
| MDEMG Run 2 | 0.822 | 0.150 | 18.3% | 96.4% | 96.4% |
| MDEMG Run 3 | 0.717 | 0.023 | 3.2% | 100.0% | 100.0% |

*Run 1 had answer formatting issue (no file:line refs)

#### Aggregate Comparison

| Metric | Baseline (3 runs) | MDEMG (runs 2-3) | Delta |
|--------|-------------------|------------------|-------|
| Mean Score | **0.822** | 0.769 | -6.4% |
| Strong Evidence | 96.7% | **98.2%** | +1.5% |
| Best CV (consistency) | 8.3% | **3.2%** | 2.6x better |

#### Learning Edge Progression

| Run | Start | End | Change |
|-----|-------|-----|--------|
| MDEMG Run 1 | 0 | 10,466 | +10,466 |
| MDEMG Run 2 | 10,466 | 15,084 | +4,618 |
| MDEMG Run 3 | 15,084 | 15,256 | +172 |

### Legacy v3 Results (pre-improvements)

| Level | Run 1 | Run 3 | Change |
|-------|-------|-------|--------|
| HIGH | 15.2% | 1.6% | -87% |
| MEDIUM | 56.8% | 47.2% | -17% |
| LOW | 28.0% | 51.2% | +83% |

## Progress Log

| Date | Task | Action | Result |
|------|------|--------|--------|
| 2026-01-27 | All | Created improvement tasks from benchmark | 7 tasks created |
| 2026-01-27 | #1 | Investigated confidence score degradation | **ROOT CAUSE: Activation dilution** |
| | | Analyzed scoring.go, activation.go | Learning edges spread activation to more nodes |
| | | Documented in CONFIDENCE_SCORE_INVESTIGATION.md | Recommended fix: percentile-based confidence |
| 2026-01-27 | #2 | Implemented normalized confidence percentiles | **COMPLETED** |
| | | Added NormalizedConfidence, ConfidenceLevel to RetrieveResult | Immune to edge density changes |
| | | Added ApplyNormalizedConfidence() + unit tests | All tests pass |
| 2026-01-27 | #3 | Implemented file type filtering | **COMPLETED** |
| | | Added FileFilter struct, CodeOnly flag | Filters at Cypher query level |
| | | Modified vectorRecall() and BM25Search() | Added filter_test.go |
| 2026-01-27 | - | **V2 BENCHMARK COMPLETE** | 6 runs graded (3 baseline + 3 MDEMG) |
| | | Baseline: Mean 0.822, CV 8.3-26.0% | Best run: 0.849 |
| | | MDEMG (runs 2-3): Mean 0.769, CV 3.2-18.3% | Best consistency: 3.2% CV |
| | | Created BENCHMARK_V2_ANALYSIS.md | Full comparative analysis |

## Next Steps

1. ~~**Immediate:** Start Task #1 investigation into confidence score degradation~~ DONE
2. ~~**Short-term:** Address Tasks #2, #3 (feature enhancements)~~ DONE
3. ~~**Benchmark:** Re-run benchmark with improvements~~ DONE (v2 complete)
4. **Medium-term:** Implement Tasks #6, #7 (monitoring and controls)
5. **Ongoing:** Task #4, #5 documentation and no-evidence investigation

---

*This document will be updated as tasks are completed.*
