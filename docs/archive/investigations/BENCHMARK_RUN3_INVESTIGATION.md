# MDEMG Run 3 Regression Investigation

**Date:** 2026-01-27
**Status:** ROOT CAUSES IDENTIFIED
**Impact:** Run 3 scores dropped from 0.822 (Run 2) to 0.717 (-12.8%)

## Executive Summary

The MDEMG Run 3 regression has **TWO distinct root causes**:

| Issue | Type | Impact | Status |
|-------|------|--------|--------|
| Answer Synthesis Failure | Benchmark Agent | 80% semantic score drop | IDENTIFIED |
| Normalized Confidence Not Applied | MDEMG Code | API not returning percentiles | NEEDS FIX |

## Issue 1: Answer Synthesis Failure (CRITICAL)

### Symptom

Run 3 answers are raw retrieval dumps, not synthesized responses.

### Evidence

**Run 2 Answer Example (scored 0.941):**

```
Package structure maps to ERP functional modules: inv (inventory),
ord (sales orders), pur (purchasing), shp (shipping), fgl (GL/finance)...
```

**Run 3 Answer Example (scored 0.709):**

```
Based on MDEMG retrieval results:
- Generic810o.java: java-class: Generic810o.java. Related to: error-handling (score: 0.719)
- jobSys: Class jobSys. Related to: error-handling (score: 0.608)
...
These 5 files appear related to the architectural pattern in question.
```

### Quantified Impact

- 100% of Run 3 answers start with "Based on MDEMG retrieval results"
- 0.7% of Run 2 answers use that pattern
- Semantic score: 0.259 → 0.052 (-80%)
- Concept score: 0.202 → 0.035 (-83%)

### Root Cause

The benchmark agent for Run 3 was not instructed (or chose not) to synthesize answers from retrieved context. It simply listed MDEMG retrieval metadata.

### Resolution

- **Discard Run 3** from comparative analysis OR
- **Re-run Run 3** with explicit answer synthesis instructions

---

## Issue 2: Normalized Confidence Not Returned (MDEMG BUG)

### Symptom

MDEMG API does not return `normalized_confidence` or `confidence_level` fields despite Task #2 being marked complete.

### Evidence

```bash
curl -X POST http://localhost:9999/v1/memory/retrieve -d '{"space_id":"blueseer-erp","query_text":"test","top_k":3}'
```

Response contains:

```json
{
  "node_id": "...",
  "score": 0.697,
  "normalized_confidence": N/A,  // MISSING
  "confidence_level": N/A         // MISSING
}
```

### Code Analysis

The code was added correctly:

- `models.RetrieveResult` has fields (models.go:31-33)
- `ApplyNormalizedConfidence()` function exists (scoring.go:387)
- Function is called after sorting (scoring.go:339)

**Possible causes:**

1. Server binary not rebuilt after code changes
2. Server not restarted after rebuild
3. Struct embedding issue (unlikely - code looks correct)

### Resolution Steps

1. Rebuild MDEMG: `go build -o bin/mdemg ./cmd/server`
2. Restart server
3. Verify API returns normalized_confidence field

### FIX APPLIED (2026-01-27)

**Root Cause:** The reasoning module and reranker were REPLACING results, losing the
normalized_confidence that was set earlier in the scoring pipeline.

**Fix:** Added `ApplyNormalizedConfidenceToResults()` call in service.go AFTER all
post-processing (reasoning, reranking, truncation) to ensure percentiles are computed
on the final result set.

**Files Changed:**

- `internal/retrieval/scoring.go` - Added `ApplyNormalizedConfidenceToResults()` function
- `internal/retrieval/service.go` - Added call after truncation step

**Verification:**

```bash
curl -X POST http://localhost:9999/v1/memory/retrieve -d '{"space_id":"blueseer-erp","query_text":"test","top_k":5}'
# Now returns: normalized_confidence=100, level=HIGH for top result
```

---

## Issue 3: Confidence Score Degradation (EXPECTED BEHAVIOR)

### Symptom

MDEMG confidence scores dropped dramatically:

- Run 2: 80.7% HIGH confidence
- Run 3: 0% HIGH confidence

### Root Cause

This is the **activation dilution** effect documented in Task #1:

- As CO_ACTIVATED_WITH edges accumulate (0 → 10,466 → 15,084)
- Activation spreads through more pathways
- Score distribution compresses toward the middle
- Fixed thresholds (>0.85 = HIGH) become harder to achieve

### Resolution

This is why Task #2 (percentile-based confidence) was implemented. The normalized confidence should be immune to learning edge density. But since Issue #2 exists, this mitigation isn't being applied.

---

## Learning Edge Progression

| Run | Start | End | Change | New Edges Created |
|-----|-------|-----|--------|-------------------|
| Run 1 | 0 | 10,466 | +10,466 | 10,466 |
| Run 2 | 10,466 | 15,084 | +4,618 | 4,618 |
| Run 3 | 15,084 | 15,256 | +172 | 172 |

**Note:** Run 3 created only 172 new edges (vs 4,618 in Run 2) - this is expected diminishing returns as the graph saturates.

---

## Benchmark Validity Assessment

| Run | Answer Quality | Evidence Quality | Valid for Comparison? |
|-----|----------------|------------------|----------------------|
| Baseline Run 1 | Good | 93.6% strong | YES |
| Baseline Run 2 | Good | 100% strong | YES |
| Baseline Run 3 | Good | 96.4% strong | YES |
| MDEMG Run 1 | No file:line refs | 0% strong | NO (formatting issue) |
| MDEMG Run 1 (fixed) | Template dumps | 100% strong | NO (synthesis issue) |
| MDEMG Run 2 | Good | 96.4% strong | YES |
| MDEMG Run 3 | Template dumps | 100% strong | NO (synthesis issue) |

**CRITICAL: Template Problem is Systematic**

The benchmark agent for MDEMG runs (except Run 2) is outputting template-style dumps:

```
Based on MDEMG retrieval results:
- file.java:1: java-class: file.java. Related to: error-handling (score: 0.XXX)
...
```

This pattern appears in:

- MDEMG Run 1 fixed: 100% template
- MDEMG Run 3: 100% template

But NOT in:

- MDEMG Run 2: 0.7% template (proper synthesis)

**Root Cause:** The agent prompt for Run 2 was different or the agent behaved differently.

**Valid Comparison:**

- Baseline: All 3 runs (mean 0.822)
- MDEMG: Run 2 only (score 0.822)

---

## Action Items

### Immediate (Before Next Benchmark)

1. [ ] Rebuild MDEMG binary with Task #2 changes
2. [ ] Verify normalized_confidence appears in API responses
3. [ ] Update benchmark agent prompts to require answer synthesis

### Short-term

1. [ ] Re-run MDEMG Run 1 with proper formatting (IN PROGRESS)
2. [ ] Re-run MDEMG Run 3 with proper synthesis instructions
3. [ ] Add benchmark validation step to check answer format before grading

### Medium-term

1. [ ] Add automated answer format validation in grading script
2. [ ] Document standard benchmark agent prompt template
3. [ ] Consider adding answer length/structure metrics

---

## Files Modified

- `docs/BENCHMARK_RUN3_INVESTIGATION.md` - This file
- `docs/BENCHMARK_IMPROVEMENTS.md` - Updated with findings
- `docs/tests/blueseer/BENCHMARK_V2_ANALYSIS.md` - Updated analysis

---

**Investigation Completed:** 2026-01-27
