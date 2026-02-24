# MDEMG Context Retention Experiment Framework

## Overview

This framework tests MDEMG's ability to maintain codebase context across LLM context compressions by comparing baseline (direct file reading) performance against MDEMG-assisted retrieval.

## Experiment Design

### Codebase

- **Repository:** whk-wms (Whiskey House Warehouse Management System)
- **Size:** 196 MB, 8,937 code elements
- **Tech Stack:** NestJS, Next.js, Prisma, PostgreSQL, GraphQL

### Test Set

- **Total Questions:** 500
- **Questions per Test:** 50 (randomly selected)
- **Categories:**
  - Architecture (100 questions)
  - Implementation (100 questions)
  - Specific Code/Paths (100 questions)
  - Domain Knowledge (100 questions)
  - Deep Technical (100 questions)

### Scoring

| Score | Meaning |
|-------|---------|
| 1.0 | Completely correct with specific details |
| 0.5 | Partially correct, missing specifics |
| 0.0 | Unable to answer or completely wrong |
| -1.0 | Confidently wrong (hallucination) |

## Metrics Tracked

### Primary Metrics

1. **Average Test Score** (-1.0 to 1.0)
2. **Total Tokens Consumed** (input + output)
3. **Context Compressions** (number of times context was compressed)
4. **Time Elapsed** (seconds)

### Secondary Metrics

1. **Score by Category** (Architecture, Implementation, etc.)
2. **Retrieval Accuracy** (MDEMG only - relevance of retrieved context)
3. **Answer Confidence** (self-reported confidence)
4. **Hallucination Rate** (percentage of -1.0 scores)

## Test Execution Protocol

### Baseline Test (Non-MDEMG)

1. Start fresh conversation
2. Systematically read codebase files into context
3. Answer 50 randomly selected questions
4. Record metrics after each question
5. Document results

### MDEMG Test

1. Ingest full codebase into MDEMG
2. Run consolidation
3. Start fresh conversation (no direct file reading)
4. For each question, query MDEMG for relevant context
5. Answer question using only MDEMG-retrieved context
6. Record metrics after each question
7. Document results

## Output Files

Each test run produces:

- `./mdemg/docs/tests/baseline-{timestamp}.md` - Baseline results
- `./mdemg/docs/tests/mdemg-{timestamp}.md` - MDEMG results
- `./mdemg/docs/tests/comparison-{timestamp}.md` - Comparative analysis

## Result Documentation Template

```markdown
# {Test Name} Results

**Date:** {timestamp}
**Test Type:** {Baseline|MDEMG}
**Codebase:** whk-wms

## Summary Metrics

| Metric | Value |
|--------|-------|
| Average Score | {score} |
| Total Tokens | {tokens} |
| Context Compressions | {compressions} |
| Time Elapsed | {time}s |

## Score by Category

| Category | Questions | Score |
|----------|-----------|-------|
| Architecture | {n} | {score} |
| Implementation | {n} | {score} |
| Specific Code | {n} | {score} |
| Domain | {n} | {score} |
| Deep Technical | {n} | {score} |

## Question Results

| # | Category | Question | Score | Notes |
|---|----------|----------|-------|-------|
{question_results}

## Analysis

{analysis}
```

## Comparison Report Template

```markdown
# MDEMG Context Retention Experiment - Comparison

**Date:** {timestamp}
**Codebase:** whk-wms

## Summary

| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| Average Score | {b_score} | {m_score} | {delta} |
| Total Tokens | {b_tokens} | {m_tokens} | {delta} |
| Context Compressions | {b_comp} | {m_comp} | {delta} |
| Time Elapsed | {b_time}s | {m_time}s | {delta} |

## Category Comparison

| Category | Baseline | MDEMG | Delta |
|----------|----------|-------|-------|
| Architecture | {score} | {score} | {delta} |
| Implementation | {score} | {score} | {delta} |
| Specific Code | {score} | {score} | {delta} |
| Domain | {score} | {score} | {delta} |
| Deep Technical | {score} | {score} | {delta} |

## Key Findings

{findings}

## Recommendations

{recommendations}
```
