# Test Question Schema Specification

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Version:** 1.0
**Last Updated:** 2026-01-28

---

## Overview

This document defines the canonical JSON schema for MDEMG benchmark test question sets. All benchmark question files MUST conform to this specification.

---

## File Naming Convention

| File Type | Pattern | Example |
|-----------|---------|---------|
| Master (with answers) | `benchmark_questions_v{N}_master.json` | `benchmark_questions_v1_master.json` |
| Agent (no answers) | `benchmark_questions_v{N}_agent.json` | `benchmark_questions_v1_agent.json` |
| Codebase Profile | `codebase_profile.json` | `codebase_profile.json` |

---

## Master Question File Schema

```json
{
  "metadata": {
    "codebase": "string (repo name, e.g., 'zed', 'blueseer')",
    "version": "string (e.g., '1.0')",
    "schema_version": "string ('v1')",
    "total_questions": "number",
    "created": "string (YYYY-MM-DD)",
    "repo_path": "string (absolute path to repo)",
    "space_id": "string (MDEMG space ID, should match codebase name)",
    "categories": ["array of category strings"],
    "difficulty_distribution": {
      "hard": "number",
      "medium": "number",
      "easy": "number"
    },
    "category_distribution": {
      "category_name": "number"
    }
  },
  "questions": [
    {
      "id": "number (1-indexed, sequential)",
      "category": "string (from metadata.categories)",
      "question": "string (the question text)",
      "expected_answer": "string (detailed expected answer)",
      "difficulty": "string ('easy' | 'medium' | 'hard')",
      "requires_files": ["array of file paths that answer requires"],
      "evidence": [
        {
          "file": "string (absolute file path)",
          "line_start": "number",
          "line_end": "number",
          "snippet": "string (relevant code snippet)"
        }
      ]
    }
  ]
}
```

---

## Agent Question File Schema

**CRITICAL**: Agent files MUST NOT contain answers. Used for blind testing.

```json
{
  "metadata": {
    "codebase": "string",
    "total_questions": "number",
    "contains_answers": false,
    "purpose": "Agent input - answers stripped to prevent contamination",
    "source_file": "string (master file this was derived from)"
  },
  "questions": [
    {
      "id": "number",
      "category": "string",
      "question": "string",
      "difficulty": "string"
    }
  ]
}
```

---

## Question Categories

### Standard Categories

| Category | Description | Example Topics |
|----------|-------------|----------------|
| `architecture_structure` | Module boundaries, design patterns, crate organization | How do modules interact? What pattern is used? |
| `service_relationships` | Interface contracts, dependencies, API boundaries | How does X depend on Y? What interface connects them? |
| `business_logic_constraints` | Validation rules, invariants, domain logic | What constraints apply? How is X validated? |
| `data_flow_integration` | Request/response flows, data transformations | How does data flow from A to B? |
| `cross_cutting_concerns` | Logging, security, caching, error handling | How is authentication handled? |
| `calibration` | Easy baseline questions to verify agent is working | What is the name of the main struct in X? |
| `negative_control` | Questions with no valid answer (tests hallucination) | Describe the FooBar module (which doesn't exist) |

### Difficulty Levels

| Level | Criteria |
|-------|----------|
| `easy` | Single file lookup, direct answer in code |
| `medium` | 2-3 files, requires understanding connections |
| `hard` | Cross-module, requires deep architectural understanding |

---

## Codebase Profile Schema

```json
{
  "codebase": "string (repo name)",
  "description": "string (one-line description)",
  "collected_at": "string (YYYY-MM-DD)",
  "repo_path": "string (absolute path)",
  "repo_url": "string (optional, git URL)",
  "repo_commit": "string (git SHA)",
  "space_id": "string (MDEMG space ID)",
  "total_files": "number",
  "total_loc": "number (lines of code)",
  "file_types": {
    "extension": "number (count)"
  },
  "avg_file_loc": "number",
  "primary_language": "string",
  "domain": "string (e.g., 'Code Editor', 'ERP System')",
  "key_modules": ["array of key module descriptions"],
  "key_patterns": ["array of important patterns used"],
  "subsystems": ["array of major subsystems"],
  "mdemg_stats": {
    "memory_count": "number",
    "layer_0": "number",
    "layer_1": "number",
    "layer_2": "number",
    "health_score": "number"
  }
}
```

---

## Validation Rules

### Master File

1. All question IDs must be unique and sequential (1 to N)
2. `expected_answer` must be non-empty
3. `requires_files` must list at least one file
4. `evidence` should have at least one entry per question
5. All file paths must be absolute and valid

### Agent File

1. Must NOT contain `expected_answer`, `requires_files`, or `evidence` fields
2. `contains_answers` must be `false`
3. Question IDs must match the master file exactly

### Distribution

- Recommended: 70-80% hard, 15-20% medium, 5-10% easy
- At least 20 questions per major category
- Include 5-10 calibration questions
- Include 5-10 negative control questions

---

## Script: Generate Agent File from Master

```python
#!/usr/bin/env python3
"""Generate agent file from master file by stripping answers."""
import json
import sys

def strip_answers(master_path, agent_path):
    with open(master_path) as f:
        master = json.load(f)

    agent = {
        "metadata": {
            "codebase": master["metadata"]["codebase"],
            "total_questions": master["metadata"]["total_questions"],
            "contains_answers": False,
            "purpose": "Agent input - answers stripped to prevent contamination",
            "source_file": master_path.split("/")[-1]
        },
        "questions": []
    }

    for q in master["questions"]:
        agent["questions"].append({
            "id": q["id"],
            "category": q["category"],
            "question": q["question"],
            "difficulty": q["difficulty"]
        })

    with open(agent_path, "w") as f:
        json.dump(agent, f, indent=2)

    print(f"Generated {agent_path} with {len(agent['questions'])} questions")

if __name__ == "__main__":
    strip_answers(sys.argv[1], sys.argv[2])
```

---

## Example Directory Structure

```
docs/tests/{codebase}/
├── codebase_profile.json           # Codebase metadata
├── benchmark_questions_v1_master.json  # Master with answers
├── benchmark_questions_v1_agent.json   # Agent without answers
├── grade_answers.py                # Grading script
├── answers_baseline_run1.jsonl     # Baseline answers
├── answers_mdemg_run1.jsonl        # MDEMG answers
├── grades_baseline_run1.json       # Graded baseline
├── grades_mdemg_run1.json          # Graded MDEMG
└── BENCHMARK_RESULTS.md            # Results summary
```
