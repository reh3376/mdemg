# plc-gbt Benchmark Report - 2026-01-28 v2

## Overview

| Field | Value |
|-------|-------|
| **Date** | 2026-01-28 |
| **Codebase** | plc-gbt (PLC Golden Batch Toolkit) |
| **Repo Commit** | `2f761094afe5df46ca9ea1a96bfc1c8dba303084` |
| **MDEMG Commit** | `b28adf73f6b09f6420724167f02310c31b272032` |
| **Framework** | BENCHMARK_FRAMEWORK_V2.md v2.3 |
| **Grading Script** | `grade_answers_v3.py` v4.0 (adaptive similarity scoring) |

### Codebase Profile

plc-gbt is a full-stack industrial automation platform for PLC golden batch testing. It includes:

- Next.js frontend (TypeScript, 8,056 .ts files)
- Python backend/analysis services (16,914 .py files)
- JSON schemas for control loop configuration
- n8n workflow automation
- ACD-to-L5X conversion tooling

### MDEMG Space

| Metric | Value |
|--------|-------|
| Space ID | `plc-gbt` |
| Total Memories | 17,801 |
| Layer 0 (source) | 17,534 |
| Layer 1 (concepts) | 262 |
| Layer 2 (abstractions) | 4 |
| Layer 3 (meta) | 1 |
| Embedding Coverage | 99.97% |
| Health Score | 0.9999 |
| Learning Edges (pre-run) | 21,734 |
| Learning Edges (post-run) | 21,992 (+258) |

---

## Configuration

| Parameter | Value |
|-----------|-------|
| Questions | 115 (from `benchmark_questions_v1_agent.json`) |
| Grading Weights | 70% evidence + 15% semantic + 15% concept + 10% file bonus |
| Agent Model | Claude Haiku 4.5 |
| Baseline Agent | Repo access via Glob/Grep/Read, no MDEMG |
| MDEMG Agent | Repo access + MDEMG retrieve API (`code_only: true`, `top_k: 10`) |
| Baseline Runs | 1 (valid) |
| MDEMG Runs | 1 (valid) |

---

## Results

### Run Summary

| Run | Type | Status | Answered | HIGH | MED | LOW | Strong Evidence | Mean Score |
|-----|------|--------|----------|------|-----|-----|-----------------|------------|
| baseline_run1 | Baseline | Valid | 115/115 | 58 | 23 | 34 | 70/115 (60.9%) | 0.594 |
| mdemg_run1 | MDEMG | Valid | 115/115 | 99 | 15 | 1 | 111/115 (96.5%) | **0.926** |

### Head-to-Head Comparison

| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| **Mean Score** | 0.594 | **0.926** | **+0.332 (+55.9%)** |
| Std Dev | 0.445 | 0.108 | -0.337 |
| CV (consistency) | 75.0% | **11.7%** | -63.3 pp |
| Median | 0.880 | — | — |
| High Score Rate (>=0.7) | 60.9% | **96.5%** | +35.6 pp |
| Strong Evidence (file:line) | 70/115 (60.9%) | **111/115 (96.5%)** | +41 |
| Weak Evidence (files only) | 0/115 | 4/115 | +4 |
| No Evidence | 45/115 (39.1%) | **0/115 (0.0%)** | -45 |
| Scores Below 0.5 | 45/115 | **1/115** | -44 |

### By Difficulty

| Difficulty | n | Baseline | MDEMG | Delta |
|------------|---|----------|-------|-------|
| Easy | 20 | 0.728 | 0.969 | +0.241 |
| Medium | 45 | 0.609 | 0.919 | +0.310 |
| **Hard** | **35** | **0.534** | **0.930** | **+0.396** |

MDEMG's largest advantage is on hard questions (+0.396), which require cross-file understanding where retrieval augmentation provides the most value.

### By Category

| Category | n | Baseline | MDEMG | Delta |
|----------|---|----------|-------|-------|
| acd_l5x_conversion | 10 | 0.121 | 0.851 | **+0.730** |
| system_purpose | 3 | 0.100 | 0.805 | **+0.705** |
| control_loop_architecture | 10 | 0.294 | 0.967 | **+0.673** |
| business_logic_workflows | 10 | 0.370 | 0.966 | **+0.596** |
| safety_security | 2 | 0.433 | 0.923 | +0.490 |
| data_models_schema | 10 | 0.502 | 0.924 | +0.422 |
| system_architecture | 4 | 0.496 | 0.916 | +0.420 |
| integration | 2 | 0.422 | 0.837 | +0.415 |
| security_authentication | 8 | 0.564 | 0.976 | +0.412 |
| n8n_workflow | 6 | 0.500 | 0.828 | +0.328 |
| database_persistence | 6 | 0.793 | 0.973 | +0.180 |
| ai_ml_integration | 10 | 0.732 | 0.835 | +0.103 |
| api_services | 10 | 0.900 | 1.000 | +0.100 |
| ui_architecture | 2 | 0.917 | 0.962 | +0.045 |
| ui_ux | 10 | 0.976 | 0.981 | +0.005 |
| configuration_infrastructure | 10 | 0.943 | 0.943 | +0.000 |
| data_architecture | 2 | 0.895 | 0.867 | -0.028 |

---

## Analysis

### MDEMG Strengths

1. **Domain-specific knowledge retrieval**: The largest improvements (+0.6 to +0.7) came in categories where the baseline had no parametric knowledge — `acd_l5x_conversion` (0.121 → 0.851), `system_purpose` (0.100 → 0.805), and `control_loop_architecture` (0.294 → 0.967). These are proprietary, domain-specific topics that an LLM cannot know without codebase access.

2. **Near-zero failures**: Only 1/115 MDEMG answers scored below 0.5 (Q82, ai_ml_integration, score 0.440), compared to 45/115 for baseline. MDEMG eliminated the failure mode almost entirely.

3. **Consistency**: MDEMG CV of 11.7% vs baseline 75.0% means MDEMG answers are uniformly high quality rather than bimodal (either correct or completely wrong).

4. **Evidence quality**: 96.5% of MDEMG answers had strong file:line evidence vs 60.9% for baseline. MDEMG answers are verifiable.

### Baseline Strengths

1. **Configuration/infrastructure**: Categories like `configuration_infrastructure` (0.943) and `ui_ux` (0.976) scored identically or near-identically — these involve common patterns (package.json, tsconfig, React components) where parametric knowledge is sufficient.

2. **data_architecture**: The only category where baseline edged MDEMG (-0.028), though the difference is within noise on n=2 questions.

### Failure Analysis

**MDEMG failures (score < 0.5): 1/115**

- Q82 (ai_ml_integration): Score 0.440. Evidence score 0.50 (weak ref), semantic 0.33, concept 0.27. The answer found relevant files but didn't extract the specific detail asked for.

**Baseline failures (score < 0.5): 45/115**

- Dominated by `acd_l5x_conversion` (10/10 failed), `control_loop_architecture` (7/10), `business_logic_workflows` (6/10) — all domain-specific categories where the LLM has no parametric knowledge.

### Learning Edge Impact

- Pre-run: 21,734 co-activated edges
- Post-run: 21,992 co-activated edges (+258)
- The space already had significant edge density from prior benchmark runs. The 258 new edges represent reinforcement of query-relevant pathways during this run.

---

## Methodology Notes

### v1 Run (Invalid — Excluded)

An initial MDEMG benchmark run (v1) scored 0.051 mean due to two structural issues in the agent prompt:

1. **Missing `code_only: true`** — MDEMG retrieve calls returned abstract concept nodes (layer 1/2) with null file paths instead of source code memories
2. **No fallback search** — When MDEMG returned unusable results, the agent wrote placeholder answers ("[Answer pending source code analysis]") for 108/115 questions

These were test harness issues, not MDEMG functionality issues. The v1 run was discarded and the agent prompt was corrected for v2.

### Grading Script Upgrade (v3.1 → v4.0)

The grading script was upgraded from v3.1 to v4.0 to be codebase-agnostic. Changes:

1. **Numeric tokenization** — Added version strings (1.0.0), decimals (10.0), integers (8192), hex (0xFF) to tokenizer
2. **Expanded concept extraction** — Added PascalCase identifiers, numeric values, regex patterns, and more suffix patterns (Config, Schema, Type, Level, Status, Request, Response, Store, Provider, Component)
3. **Recall-based adaptive similarity** — Short expected answers (< 10 tokens) use 80% recall + 20% Jaccard instead of pure Jaccard, preventing penalization when actual answers are longer than expected

Both baseline and MDEMG were graded with the same v4.0 script.

---

## Files

| File | Description |
|------|-------------|
| `answers_baseline_run1.jsonl` | Baseline agent answers (115 questions) |
| `answers_mdemg_run1.jsonl` | MDEMG agent answers (115 questions) |
| `grades_baseline_run1.json` | Baseline grades (v4.0 scoring) |
| `grades_mdemg_run1.json` | MDEMG grades (v4.0 scoring) |
| `BENCHMARK_REPORT.md` | This report |

---

## Conclusion

MDEMG retrieval provides a **+55.9% score improvement** over baseline on the plc-gbt codebase (0.926 vs 0.594). The advantage is strongest on domain-specific questions (+0.6 to +0.7 in ACD conversion, control loop architecture, and business logic), where parametric LLM knowledge is insufficient. MDEMG achieves near-perfect evidence rates (96.5%) and near-zero failures (1/115 below 0.5).
