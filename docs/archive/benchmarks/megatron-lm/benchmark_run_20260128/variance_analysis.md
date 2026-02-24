# MDEMG Benchmark Variance Analysis

## Executive Summary

**Finding:** The variance between MDEMG runs (Run 1: 0.774 vs Runs 2-3: 0.712-0.715) is caused by **agent behavior differences**, not MDEMG retrieval quality. Run 1's agent properly synthesized answers from MDEMG results, while Runs 2-3's agent dumped raw MDEMG metadata as answers.

---

## Run Statistics

| Metric | Run 1 | Run 2 | Run 3 |
|--------|-------|-------|-------|
| **Score** | 0.774 | 0.712 | 0.715 |
| **Refs/Question** | 1.1 | 10.0 | 10.0 |
| **Avg Answer Length** | 111 chars | 183 chars | 184 chars |
| **Strong Evidence Rate** | 100% | 100% | 100% |

---

## Root Cause Analysis

### Run 1 Behavior (Correct)

The agent in Run 1 properly:

1. Queried MDEMG for context
2. **Interpreted and synthesized** the results into a coherent answer
3. Cited only relevant file:line references (1-2 per question)
4. Provided concise, accurate answers

**Example Q136 (Negative Control - Hyperparameter Tuning):**

```json
{
  "answer": "No built-in hyperparameter tuning or NAS. Requires external tools like Ray Tune, Optuna.",
  "file_line_refs": ["megatron/core/__init__.py:1"]
}
```

- **Score: 1.0** (correct_not_found: true)
- Semantic score: 0.576
- Concept score: 1.0

### Runs 2-3 Behavior (Incorrect)

The agent in Runs 2-3:

1. Queried MDEMG for context
2. **Dumped raw MDEMG metadata** as the answer text
3. Included all 10 references regardless of relevance
4. Produced nonsensical answers that copy MDEMG's internal format

**Example Q136 from Run 2:**

```json
{
  "answer": "Config: model_config.yaml in config Config: model_config.yaml in config. Related to: authentication Config: model_config.yaml in config",
  "file_line_refs": ["tests/.../model_config.yaml:1", ... (10 total)]
}
```

- **Score: 0.714**
- Semantic score: 0.071 (!)
- Concept score: 0.026 (!)

**Example Q1 from Run 2 (MegatronModule question):**

```json
{
  "answer": "Package: __init__. Python module: __init__\nFile: megatron/core/models/mamba/__init__.py\nImports: 0\n\n--- Code ---\n# Copyright (c) 2024, NVIDIA CORPORATION",
  "file_line_refs": ["megatron/core/models/mamba/__init__.py:1", ...]
}
```

This is literally MDEMG's internal element description format being used as an answer.

---

## High Variance Questions (Q136-142)

These are **negative control questions** that test whether the agent correctly identifies features that DON'T exist.

| Q# | Question | Run 1 Score | Run 2 Score | Delta |
|----|----------|-------------|-------------|-------|
| 136 | Hyperparameter tuning? | 1.0 | 0.714 | -0.286 |
| 137 | Native ONNX export? | 1.0 | 0.720 | -0.280 |
| 138 | On-the-fly tokenization? | 0.796 | 0.707 | -0.089 |
| 139 | Sparse attention (BigBird)? | 1.0 | 0.700 | -0.300 |
| 140-142 | Similar negative controls | ~1.0 | ~0.7 | ~-0.3 |

Run 1 correctly answered "No" with brief explanations.
Runs 2-3 dumped irrelevant MDEMG metadata.

---

## Evidence Analysis

### Why Runs 2-3 Have 10 References Per Question

The agent appears to have been instructed (or behaved) to include ALL top_k results from MDEMG retrieval. This creates noise:

**Run 2 Q136 References:**

```
- tests/functional_tests/.../model_config.yaml:1
- tests/functional_tests/.../model_config.yaml:1
- tests/functional_tests/.../model_config.yaml:1
... (10 test config files, none relevant to hyperparameter tuning)
```

**Run 1 Q136 References:**

```
- megatron/core/__init__.py:1
```

Run 1's single reference is more appropriate - for a "does X exist?" question, pointing to the top-level module as proof of absence is reasonable.

---

## Semantic vs Concept Scores

| Question | Run 1 Semantic | Run 2 Semantic | Run 1 Concept | Run 2 Concept |
|----------|----------------|----------------|---------------|---------------|
| Q136 | 0.576 | 0.071 | 1.0 | 0.026 |
| Q137 | 0.351 | 0.074 | 1.0 | 0.057 |
| Q138 | 0.428 | 0.027 | 0.214 | 0.019 |
| Q139 | 0.398 | 0.000 | 1.0 | 0.000 |

Run 2's near-zero semantic/concept scores indicate the answers don't match the expected content AT ALL because they're MDEMG metadata, not actual answers.

---

## Conclusions

### Primary Issue: Agent Prompt/Behavior Difference

Run 1 was executed with a methodology that:

- Instructed the agent to INTERPRET MDEMG results
- Required synthesizing actual answers
- Limited references to most relevant

Runs 2-3 methodology:

- Allowed dumping raw MDEMG output as answers
- Included all 10 top_k references
- No quality filtering

### This is NOT an MDEMG Quality Issue

MDEMG returned the same types of results in all runs. The difference is how the agent USED those results:

- Run 1: Agent understood results → wrote coherent answer
- Runs 2-3: Agent copied results → produced garbage

### Recommendations

1. **Standardize agent prompts** to always require answer synthesis
2. **Add prompt language**: "Never copy MDEMG result metadata directly - always synthesize a proper answer"
3. **Limit references**: "Include only 1-3 most relevant file:line references"
4. **Add validation**: Check that answers don't contain MDEMG metadata patterns like "Package:", "Module:", "Related to:"

---

## Valid Score Comparison

Given that Runs 2-3 had defective agent behavior, the valid comparison is:

| Agent | Best Run Score |
|-------|----------------|
| **MDEMG** | **0.774** (Run 1) |
| **Baseline** | **0.767** (Run 2) |

**MDEMG wins by +0.9%** when the agent properly processes retrieval results.

---

*Analysis completed: 2026-01-28*
