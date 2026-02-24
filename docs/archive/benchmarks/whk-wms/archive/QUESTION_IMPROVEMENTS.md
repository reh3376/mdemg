# Complex Questions Improvement Report

**Date:** 2026-01-21
**Original File:** `whk-wms-questions-complex.json`
**Final File:** `whk-wms-questions-final.json`

---

## Summary of Changes

| Metric | Before | After | Change |
|--------|--------|-------|--------|
| Total Questions | 500 | 399 | -101 (filtered low-quality) |
| Multi-file Questions | 298 | 398 | +100 |
| Single-file Questions | 202 | 1 | -201 |
| Questions with Speculative Language | 43 | 0 | -43 |
| Verified/Corrected Answers | 0 | 19 | +19 |

---

## Improvements Applied

### 1. Answer Verification (43 Questions Reviewed)

- **24 verified as accurate** - no changes needed
- **19 corrected** - answers updated based on actual code review
- All speculative language ("likely", "probably", "suggests") removed

### 2. File Coverage Enhancement

- Added 170+ related files to single-file questions
- Used module relationship patterns (service ↔ module ↔ resolver)
- Questions now require understanding multiple files

### 3. Quality Filtering

- Removed questions answerable from single file without context
- Prioritized questions requiring cross-module understanding
- Kept only questions with 2+ required files or verified answers

---

## Corrected Questions (19 Total)

| ID | Category | Issue | Correction |
|----|----------|-------|------------|
| 22 | architecture | Wrong: "aggregate vs single-entity" | Fixed: Two different data models (Comments vs Comment tables) |
| 34 | architecture | Wrong: "'base' is generated CRUD" | Fixed: Core business logic, not generated |
| 35 | architecture | Wrong: "(dashboard) has layout" | Fixed: Layout is in /dashboard/, not /(dashboard)/ |
| 39 | architecture | Vague: "likely exports tools" | Fixed: 9 specific MCP tools documented |
| 40 | architecture | Wrong: "materialized views" | Fixed: Real-time SQL aggregation |
| 46 | architecture | Vague: "LLM providers" | Fixed: Specifically gpt-4o-mini |
| 56 | architecture | Wrong: "authorization bypass" | Fixed: Date-based temporal filtering |
| 57 | architecture | Wrong: "ScheduleModule.forRoot()" | Fixed: Bull + decorators + Redis locks |
| 58 | architecture | Vague: "tabular report" | Fixed: Point-in-time state reconstruction |
| 63 | architecture | Wrong: "multiple channels" | Fixed: Azure email only |
| 64 | architecture | Incomplete: "denormalized view" | Fixed: DISTINCT ON, 38+ filters |
| 68 | architecture | Wrong: "replaced older" | Fixed: Composition/specialization |
| 69 | architecture | Wrong: "Key Vault integration" | Fixed: ConfigService only |
| 249 | business_logic | Wrong: status values | Fixed: Only Boolean, Numeric |
| 253 | business_logic | Wrong: file name | Fixed: prediction-weights.dto.ts |
| 260 | data_flow | Wrong: enum values | Fixed: BUG_REPORT, FEATURE_REQUEST |
| 272 | data_flow | Wrong: "System role" | Fixed: Only USER, ASSISTANT |
| 275 | data_flow | Incomplete: metadata | Fixed: TTB compliance fields |
| 492 | cross_cutting | Wrong: "ERROR_CONFIGURATION" | Fixed: Array-based categorization |

---

## Category Distribution (Final)

| Category | Count | % |
|----------|-------|---|
| service_relationships | 98 | 24.6% |
| architecture_structure | 96 | 24.1% |
| business_logic_constraints | 79 | 19.8% |
| cross_cutting_concerns | 71 | 17.8% |
| data_flow_integration | 55 | 13.8% |
| **Total** | **399** | **100%** |

---

## File Coverage (Final)

| Files Required | Question Count |
|----------------|----------------|
| 2 files | 271 |
| 3 files | 100 |
| 4+ files | 27 |
| 1 file | 1 |

---

## Files Generated

| File | Description |
|------|-------------|
| `whk-wms-questions-final.json` | Final improved question set (399 questions) |
| `whk-wms-questions-verified.json` | Intermediate verified set |
| `whk-wms-questions-high-quality.json` | High-quality subset |
| `question_corrections.json` | Detailed corrections with rationale |

---

## Recommendations for Testing

1. **Use `whk-wms-questions-final.json`** as the primary test set
2. **Random sample 50-100 questions** per test run to manage time
3. **Verify answers against required_files** for ground truth
4. **Track which questions require the most context** to answer correctly

---

## Next Steps

1. Run baseline test with new question set
2. Run MDEMG test with new question set
3. Compare results with realistic expectations
4. Iterate on question quality based on test findings
