# Idea: Graph-Based Memory System for Improved Retrieval

## Context

I've been experimenting with a graph-based memory system called MDEMG and recently ran some benchmarks against the clawdbot codebase. The results were interesting enough that I wanted to share them in case they're useful for your roadmap.

This isn't a request or criticism - just sharing findings that might be worth evaluating.

## What I Tested

I ran a benchmark comparing two approaches for answering questions about the clawdbot codebase:

- **Baseline**: Standard file search (similar to Glob/Grep/Read patterns)
- **MDEMG**: A graph-based semantic memory system

The test used 130 questions across categories like architecture, data flow, and symbol lookup.

## Results

| Metric | Baseline | MDEMG |
|--------|----------|-------|
| Mean Score | 0.282 | 0.595 |
| Coefficient of Variation | 149.8% | 40.2% |
| No-Evidence Rate | 68.2% | 0.0% |

The grading weighted evidence quality heavily (70%) - answers needed file:line citations to score well.

## How MDEMG Differs

From what I understand of your current memory implementation:

- **Your approach**: SQLite + FTS5 + embeddings, hybrid vector/text search, context pruning with truncation
- **MDEMG**: Multi-layer knowledge graph with emergent concept abstraction and usage-based learning

The main difference is that MDEMG builds relationships between concepts rather than just storing flat embeddings. Whether that matters for your use case is something you'd need to evaluate.

## Files for Your Own Assessment

I've packaged everything needed to reproduce or modify this benchmark:

### Question Set

- `test_questions_130_master.json` - 130 questions with expected answers
- `test_questions_130_agent.json` - Same questions without answers (for blind testing)

### Grading

- `grade_answers.py` - Semantic similarity grading script (pure Python, no dependencies)

### Prompts

- `baseline_agent_prompt.md` - Instructions for baseline agent
- `mdemg_agent_prompt.md` - Instructions for MDEMG agent

### Raw Results

- `answers_baseline_run[1-3].jsonl` - Baseline agent responses
- `answers_mdemg_run[1-3].jsonl` - MDEMG agent responses
- `grades_*.json` - Per-question grading breakdown
- `aggregate_results.json` - Summary statistics

## Running Your Own Test

If you want to evaluate this yourself:

```bash
# Grade any answer file against the master questions
python grade_answers.py answers.jsonl test_questions_130_master.json grades.json
```

The grading script uses:

- N-gram Jaccard similarity
- Weighted word overlap (pseudo-IDF)
- Key concept extraction
- Evidence citation detection

## MDEMG Overview

MDEMG is a memory system I've been working on. Key features:

- **Multi-layer graph**: Code elements → emergent concepts → meta-patterns
- **Hebbian learning**: Usage reinforces retrieval paths
- **Evidence tracking**: Returns file:line citations with results

The repository is currently private while I finish hardening it for public release. If you're interested in early access or have questions about the implementation, feel free to reach out and I can provide access or more details.

Once public, basic usage looks like:

```bash
curl -X POST http://localhost:8090/v1/memory/consult \
  -d '{"space_id": "your-space", "question": "...", "include_evidence": true}'
```

## Caveats

- This benchmark tests retrieval accuracy, not end-to-end agent performance
- The baseline used standard file search, not your full memory system
- MDEMG requires running a separate service (repo currently private, public release planned)
- I may have biases in question design (you can review/modify)

## Summary

I found MDEMG helpful for my use cases and thought the benchmark results were worth sharing. Feel free to ignore if this doesn't fit your direction, or use the benchmark files to run your own evaluation.

Happy to answer questions or provide more context.

---

*Benchmark files attached. All code is available for independent verification.*
