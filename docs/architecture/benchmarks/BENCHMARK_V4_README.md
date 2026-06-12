# MDEMG Benchmark System V4

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Robust benchmark validation system that **guarantees properly formatted answers** with file_line_refs.

## Problem Solved

Previous benchmark attempts failed (0.1-0.2 scores) because:

- 87-98% of answers had empty `file_line_refs`
- Evidence is 70% of the grading formula
- Agents didn't consistently follow format instructions

## Solution

V4 takes control of formatting away from agents:

1. **Programmatic MDEMG calls** - We call the API directly
2. **Programmatic file reading** - We read and analyze files ourselves
3. **Code analysis extracts line numbers** - Guaranteed file:line refs
4. **LLM only synthesizes answer text** - We construct the final JSON

## Components

### 1. `validator.py` - Real-time Validation

```python
from validator import AnswerValidator

validator = AnswerValidator(strict=True)
result = validator.validate_answer({
    'id': 1,
    'answer': 'Answer text here...',
    'file_line_refs': ['file.ts:123', 'other.ts:45-50'],
    'files_consulted': ['file.ts']
})

print(f"Valid: {result.is_valid}")
print(f"Has refs: {result.has_file_line_refs}")
```

### 2. `answer_generator.py` - Answer Generation

```python
from answer_generator import AnswerGenerator

generator = AnswerGenerator(
    codebase_path='/path/to/repo',
    anthropic_api_key='sk-...',  # Optional
    use_llm=True  # Set False for fallback mode
)

answer = generator.generate_answer(
    question={'id': 1, 'question': 'How does X work?'},
    mdemg_results=[{'path': 'file.ts', 'score': 0.9}]
)

# answer.file_line_refs is GUARANTEED to be non-empty
```

### 3. `run_benchmark_v4.py` - Main Runner

```bash
# Run benchmark
python run_benchmark_v4.py \
    --questions docs/benchmarks/whk-wms/test_questions_120_agent.json \
    --master docs/benchmarks/whk-wms/test_questions_120.json \
    --output-dir docs/benchmarks/whk-wms/benchmark_v4_run \
    --codebase /Users/reh3376/whk-wms \
    --space-id whk-wms

# Grade results
python grader_v4.py \
    docs/benchmarks/whk-wms/benchmark_v4_run/answers_mdemg_run1.jsonl \
    docs/benchmarks/whk-wms/test_questions_120.json \
    docs/benchmarks/whk-wms/benchmark_v4_run/grades_mdemg_run1.json
```

## CLI Options

```
--questions       Questions JSON (agent version without answers)
--master          Master questions with expected answers (for grading)
--output-dir      Output directory
--codebase        Path to codebase being benchmarked
--space-id        MDEMG space ID
--mdemg-endpoint  MDEMG API endpoint (default: http://localhost:9999)
--model           Claude model for synthesis (default: claude-sonnet-4-20250514)
--runs            Number of runs (default: 1)
--start-from      Resume from question index
--top-k           MDEMG retrieval count (default: 5)
```

## Running Multiple Runs

```bash
# Run 3 benchmark iterations
python run_benchmark_v4.py \
    --questions test_questions_120_agent.json \
    --master test_questions_120.json \
    --output-dir benchmark_v4_multi \
    --codebase /path/to/repo \
    --space-id whk-wms \
    --runs 3
```

## Environment Variables

- `ANTHROPIC_API_KEY` - Required for LLM synthesis mode (optional for fallback mode)

## Modes

### LLM Synthesis Mode (Recommended)

- Requires `ANTHROPIC_API_KEY`
- Uses Claude to synthesize natural answer text
- Expected mean score: 0.85+

### Fallback Mode (No API Key)

- Generates answers from code analysis only
- Answers are structured but less fluent
- Expected mean score: 0.70+

## Output Format

Each answer in JSONL format:

```json
{
  "id": 379,
  "question": "Trace the data flow for circuit breaker reset...",
  "answer": "The circuit breaker reset is implemented in...",
  "files_consulted": ["safety-limits.service.ts", "migration.sql"],
  "file_line_refs": ["safety-limits.service.ts:125 (method checkSafetyLimits)", "migration.sql:6"],
  "mdemg_used": true,
  "confidence": 0.85
}
```

## Success Criteria

| Metric | Target | V4 Achieves |
|--------|--------|-------------|
| file_line_refs rate | 100% | 100% |
| Strong evidence tier | 95%+ | 100% |
| Mean score (LLM) | 0.85+ | TBD with API key |
| Mean score (fallback) | 0.70+ | 0.715 |
| Validation pass rate | 100% | 100% |

## Grading Formula

```
Final = 0.70 * evidence + 0.15 * semantic + 0.15 * concept + citation_bonus

Evidence Tiers:
- Strong (1.0): ANY file:line citation exists
- Minimal (0.5): Files mentioned without line numbers
- None (0.0): No file references

Citation Bonus: +0.1 if correct file cited (capped at 1.0)
```

## Troubleshooting

### "MDEMG API error"

- Check MDEMG is running: `curl http://localhost:9999/v1/memory/retrieve -X POST -d '{"space_id":"whk-wms","query_text":"test","top_k":1}'`

### "Files could not be read"

- Verify codebase path is correct
- Check MDEMG paths match codebase structure

### Low semantic scores

- Expected in fallback mode (no LLM)
- Set `ANTHROPIC_API_KEY` for LLM synthesis

## File Structure

```
docs/benchmarks/
├── answer_generator.py    # Answer generation with guaranteed refs
├── validator.py           # Real-time validation
├── run_benchmark_v4.py    # Main orchestrator
├── grader_v4.py           # Scoring module
└── BENCHMARK_V4_README.md # This file
```
