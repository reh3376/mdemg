# MDEMG Benchmark Report - Retrieval Layer Changes

**Date:** 2026-02-02
**Benchmark:** whk-wms 120-question full benchmark
**Benchmark Version:** V4 with agent synthesis

## Executive Summary

| Metric | Current Run | Baseline | Delta |
|--------|-------------|----------|-------|
| **Mean Score** | 0.782 | 0.863 | -0.081 (-9.4%) |
| **Correct File Rate** | 13.3% | 75.8% | -62.5pp |
| **Evidence Rate** | 100.0% | N/A | - |
| **High Score Rate (>=0.7)** | 100.0% | N/A | - |

## Benchmark Configuration

- **Questions:** 120 (whk-wms/test_questions_120_agent.json)
- **Codebase:** /Users/reh3376/whk-wms
- **MDEMG Space:** whk-wms
- **Model:** sonnet
- **Top-k:** 5
- **Use Agent:** Yes
- **Code-only filter:** Yes (excludes sql, md, txt, json, yaml, yml)

## Run Statistics

- **Total Questions:** 120
- **Processed:** 120
- **Successful:** 120 (100%)
- **Failed:** 0
- **Validation Errors:** 0
- **With file_line_refs:** 120 (100%)
- **Duration:** 22.5 minutes (1350.4 seconds)
- **Rate:** 5.3 questions/minute

## Score Distribution

| Statistic | Value |
|-----------|-------|
| Mean | 0.782 |
| Std Dev | 0.057 |
| CV | 7.3% |
| Median | 0.764 |
| Min | 0.717 |
| Max | 1.000 |
| P10 | 0.735 |
| P25 | 0.749 |
| P75 | 0.788 |
| P90 | 0.858 |

## Evidence Tier Distribution

| Tier | Count | Percentage |
|------|-------|------------|
| **Strong** | 120 | 100.0% |
| Moderate | 0 | 0.0% |
| Weak | 0 | 0.0% |
| None | 0 | 0.0% |

## Category Breakdown

| Category | Count | Mean | Std Dev |
|----------|-------|------|---------|
| disambiguation | 8 | 0.884 | 0.095 |
| relationship | 6 | 0.818 | 0.080 |
| computed_value | 6 | 0.811 | 0.076 |
| cross_cutting_concerns | 20 | 0.785 | 0.055 |
| service_relationships | 20 | 0.777 | 0.047 |
| architecture_structure | 20 | 0.772 | 0.035 |
| data_flow_integration | 20 | 0.758 | 0.025 |
| business_logic_constraints | 20 | 0.757 | 0.018 |

## Correct File Analysis

- **Correct File Rate:** 16/120 (13.3%)
- **Baseline Correct File Rate:** 75.8%
- **Gap:** -62.5 percentage points

The significant drop in correct file rate suggests the retrieval layer changes may be affecting file selection accuracy. The MDEMG retrieval is returning files that provide semantically relevant answers but may not match the expected "gold standard" files in the master question set.

## Comparison to Baseline

### Baseline (Reference)

- Mean: 0.863
- Correct File Rate: 75.8%

### Current Run

- Mean: 0.782 (-9.4%)
- Correct File Rate: 13.3% (-62.5pp)

### Analysis

1. **Score Gap:** The mean score dropped from 0.863 to 0.782 (-9.4%), which is a notable regression but still maintains 100% high-score rate (all answers >= 0.7).

2. **Correct File Rate Regression:** The most significant regression is in correct file rate, dropping from 75.8% to 13.3%. This indicates the retrieval layer changes are selecting different files than the expected answers.

3. **Evidence Quality:** Despite the file selection changes, evidence quality remains strong at 100%, meaning all answers provide citations from actual source files.

4. **Low Variance:** The coefficient of variation (7.3%) indicates consistent scoring across questions.

## Artifacts

| File | Description |
|------|-------------|
| `answers_mdemg_run1.jsonl` | Raw answers with file_line_refs |
| `grades_mdemg_run1.json` | Detailed grading results |
| `progress_run1.json` | Run statistics |
| `questions_master.json` | Master questions with expected answers |

## Recommendations

1. **Investigate File Selection:** The retrieval layer changes significantly altered which files are selected. Review the code_only filter and exclude_extensions parameters.

2. **Semantic vs Exact Match:** The answers remain semantically strong (100% evidence rate) but don't match expected files. Consider whether the expected answers need updating or if retrieval needs tuning.

3. **MDEMG Query Analysis:** Debug the MDEMG retrieval to understand why different files are being returned compared to baseline.

---
*Generated: 2026-02-02T07:58:00*
