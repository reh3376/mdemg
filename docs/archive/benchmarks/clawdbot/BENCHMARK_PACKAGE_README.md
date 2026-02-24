# Clawdbot Benchmark Package

This package contains everything needed to independently evaluate the MDEMG benchmark results.

## Quick Start

```bash
# Grade any answer file
python grade_answers.py <answers.jsonl> test_questions_130_master.json <output.json>

# Example: grade the baseline run 1
python grade_answers.py answers_baseline_run1.jsonl test_questions_130_master.json my_grades.json
```

## Files Included

### Core Files (Required for Evaluation)

| File | Description |
|------|-------------|
| `grade_answers.py` | Grading script (Python 3.8+, no dependencies) |
| `test_questions_130_master.json` | Questions with expected answers |
| `test_questions_130_agent.json` | Questions only (for blind testing) |

### Agent Prompts

| File | Description |
|------|-------------|
| `baseline_agent_prompt.md` | Instructions for baseline (file search) agent |
| `mdemg_agent_prompt.md` | Instructions for MDEMG agent |

### Raw Results

| File | Description |
|------|-------------|
| `answers_baseline_run[1-3].jsonl` | Baseline agent responses (3 runs) |
| `answers_mdemg_run[1-3].jsonl` | MDEMG agent responses (3 runs) |
| `grades_baseline_run[1-3].json` | Grading results for baseline |
| `grades_mdemg_run[1-3].json` | Grading results for MDEMG |
| `aggregate_results.json` | Summary statistics |

### Reference

| File | Description |
|------|-------------|
| `architecture_overview.md` | Codebase architecture notes |
| `codebase_profile.json` | Repository metrics |

## Answer Format (JSONL)

Each line in an answer file:

```json
{"id": 1, "question": "...", "answer": "...", "files_consulted": ["..."], "file_line_refs": ["file.ts:123"], "confidence": "HIGH"}
```

## Grading Weights

The grading script uses:

- 70% evidence quality (file:line citations)
- 15% semantic similarity (n-gram overlap)
- 15% concept matching (technical terms)
- +10% bonus for citing expected files

## Question Categories

| Category | IDs | Count |
|----------|-----|-------|
| Architecture | 1-20 | 20 |
| Service Relationships | 21-40 | 20 |
| Business Logic | 41-60 | 20 |
| Data Flow | 61-80 | 20 |
| Cross-Cutting | 81-100 | 20 |
| Symbol Lookup | 101-130 | 30 |

## Running Your Own Agent

1. Use `test_questions_130_agent.json` (no answers) to avoid bias
2. Have your agent output JSONL format
3. Grade with: `python grade_answers.py your_answers.jsonl test_questions_130_master.json your_grades.json`

## Verification

To verify our results:

```bash
# Re-grade baseline run 1
python grade_answers.py answers_baseline_run1.jsonl test_questions_130_master.json verify_baseline1.json

# Compare with our grades
diff grades_baseline_run1.json verify_baseline1.json
```

The grading is deterministic - you should get identical results.
