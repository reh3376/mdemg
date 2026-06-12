# MDEMG Benchmarking Guide

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Version:** 2.6
**Last Updated:** 2026-01-28
**Purpose:** Step-by-step guide for setting up, running, and analyzing MDEMG vs Baseline retrieval tests on new codebases

> **Note:** This guide incorporates learnings from BENCHMARK_FRAMEWORK_V2.md including real-time guardrails, defective agent detection, and the standardized summary schema.

---

## CANONICAL BENCHMARK SPECIFICATION

**This section contains the authoritative, non-negotiable rules for running benchmarks. Deviations from these specifications invalidate test results.**

### Core Parameters

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| **Agent Model** | `haiku` (Claude 4.5 Haiku) | Cost-effective, fast, consistent |
| **Runs Per Condition** | 3 | Detect variance without excessive cost |
| **Cold Start** | Clear learning edges before Run 1 only | Let edges accumulate across 3 runs |
| **Output Format** | JSONL (one JSON line per answer) | Machine-parseable, audit-friendly |
| **Grading Method** | Automated comparison script | Eliminates self-grading inconsistency |

### MDEMG Skill Usage Pattern

| Question Type | MDEMG Skill | Rationale |
|---------------|-------------|-----------|
| `symbol-lookup` | `/mdemg retrieve "<symbol_name>"` | Direct symbol search, no interpretation needed |
| `multi-file`, `cross-module`, `system-wide`, `disambiguation`, `computed_value`, `relationship` | `/mdemg consult "<question>"` | Complex questions need SME synthesis |

### Grading Formula (V3)

```
score = 0.70 * evidence_score + 0.15 * semantic_score + 0.15 * concept_score

where:
  evidence_score = Score based on file:line citation quality
                   - 1.0: Strong evidence (file:line refs present and valid)
                   - 0.5: Weak evidence (file refs without line numbers)
                   - 0.0: No evidence
  semantic_score = Cosine similarity between answer and expected answer embeddings
  concept_score  = Keyword/concept overlap with expected answer
```

### Evidence Tier Definitions

| Tier | Criteria | Score Weight |
|------|----------|--------------|
| **Strong** | Answer includes `file:line` references (e.g., `handler.py:42`) | Full evidence credit |
| **Weak** | Answer references files without line numbers | 50% evidence credit |
| **None** | No file references in answer | 0% evidence credit |

### Public Framing (The Elevator Pitch)

- **Non-Technical**: "Baseline can be accurate until the next context update. MDEMG is accurate because it doesn’t forget the work."
- **Technical**: "Long-context/RAG optimizes retrieval. MDEMG optimizes state persistence under context churn."

### Variance Reporting

Report ALL of the following for each condition:

- **Mean (μ)**: Average score across all questions
- **Std Dev (σ)**: Standard deviation
- **CV**: Coefficient of variation = 100 * σ / μ
- **Range**: [min_score, max_score]
- **Run Scores**: Individual mean per run (e.g., Run1: 0.72, Run2: 0.74, Run3: 0.71)

### ⛔ ANSWER CONTAMINATION PREVENTION (MANDATORY)

**The benchmark agent MUST NEVER have access to expected answers.**

| File Type | Contains Answers | Agent Access |
|-----------|------------------|--------------|
| `*_agent.json` | NO | ✅ YES - Give to agent |
| `*_master.json` | YES | ❌ NEVER - Grading only |
| `test_questions_120.json` | YES | ❌ NEVER |
| `test_questions_120_agent.json` | NO | ✅ YES |

**CRITICAL RULES:**

1. Agent receives ONLY the `*_agent.json` file (questions without answers)
2. The `*_master.json` file is used ONLY by the grading script AFTER agent completes
3. NEVER include answer keys in agent prompts
4. NEVER let agent access the master question file
5. Violation of these rules INVALIDATES the entire benchmark

**File Separation:**

```
questions/
├── test_questions_120_agent.json   # → Agent input (NO answers)
└── test_questions_120.json         # → Grading script only (HAS answers)
```

### Required Output Files

| File | Format | Description |
|------|--------|-------------|
| `answers_run{N}.jsonl` | JSONL | One JSON line per answer per run |
| `grades_run{N}.json` | JSON | Grading results per run |
| `aggregate_report.json` | JSON | Combined metrics across all 3 runs |
| `run_manifest.json` | JSON | Environment, commits, config snapshot |
| `BENCHMARK_RESULTS_V{X}.md` | **Markdown** | **REQUIRED** - Human-readable summary report |

### Final Summary Report Format (MANDATORY)

Every benchmark run MUST produce a markdown summary report (`BENCHMARK_RESULTS_V{X}.md`) that:

1. **References ALL associated files** generated during the test run
2. **Uses standardized sections** for each condition (baseline/MDEMG)
3. **Includes all metrics** from the Learning & Persistence section
4. **Is self-contained** - a reader can understand results without other files

**Required Sections:**

```markdown
# Benchmark Results V{X}

## Overview
- Run ID, date, purpose
- Codebase info (commits, space_id, memory counts)

## Repo & Ingest Scope (REQUIRED - kills "toy slice" complaint)

| Property | Value |
|----------|-------|
| Repo | {{REPO_PATH}} |
| Commit | {{SHA}} |
| Ingest scope | {{PATHS_INCLUDED}} |
| Excluded | dist/, node_modules/, .git/, vendor/ |
| LOC ingested | {{LOC}} |
| Files ingested | {{FILE_COUNT}} |
| Top extensions | .ts: X, .json: Y, .tsx: Z, ... |
| Top dirs by LOC | src/: X, apps/: Y, libs/: Z, ... |

## Test Configuration
- Question file, answer key, question count
- Grading weights, model used
- Cold start status, learning edges cleared

## Validity Checks (MUST PASS - prevents ID mismatch disasters)

Before a run is considered "valid", ALL must be checked:
- [ ] All question IDs match answer bank
- [ ] Questions Graded == Expected Question Count (120)
- [ ] No None or bonus IDs in answers
- [ ] Answer file lines == Question Count
- [ ] Dataset hash matches manifest

## Baseline Results (Standardized per run)
### Run 1
- Questions answered, mean, std, CV%
- Evidence rate, correct file rate
- Wall time, tokens consumed
### Run 2
[Same format]
### Run 3
[Same format]
### Baseline Aggregate
- Combined metrics across all 3 runs

## MDEMG Results (Standardized per run)
### Run 1 (COLD - 0 learning edges)
- Questions answered, mean, std, CV%
- Evidence rate, correct file rate
- Learning edges: before=0, after=X
- Wall time, tokens consumed
### Run 2 (WARM - with Run 1 edges)
[Same format + improvement delta from Run 1]
- Learning edges: before=X, after=Y
### Run 3 (WARM - accumulated edges)
[Same format + improvement delta from Run 1]
- Learning edges: before=Y, after=Z
### MDEMG Aggregate
- Combined metrics across all 3 runs
- Learning progression analysis

## Efficiency & Budget (REQUIRED - proves "substrate > brute force")

| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| Tokens/question (avg) | X | Y | -Z% |
| Tokens/question (p95) | X | Y | -Z% |
| Tool calls/question (avg) | X | Y | -Z% |
| Tool calls/question (p95) | X | Y | -Z% |
| Retrieval latency p50 (ms) | N/A | X | - |
| Retrieval latency p95 (ms) | N/A | X | - |
| Wall time / run | Xm | Ym | -Z% |
| Total tokens / run | X | Y | -Z% |

## Evidence Metrics (Precise Definitions)

| Metric | Definition | Formula |
|--------|------------|---------|
| **ECR** (Evidence Compliance Rate) | % answers with ≥1 file-path citation + concrete value | answers_with_evidence / total_answers |
| **E-Acc** (Evidence Accuracy) | % answers where cited file(s) contain claimed value(s) | correct_citations / answers_with_evidence |
| **WER** (Wrong Evidence Rate) | % of evidenced answers citing incorrect files/values | 1 - E-Acc |

## Comparison Summary
- Side-by-side baseline vs MDEMG
- Statistical significance
- Key findings

## Learning Progression Analysis
- Edge growth: Run 1 → Run 2 → Run 3
- Evidence score improvement
- Token efficiency comparison

## Skepticism Reduction Metrics (MANDATORY)
These 4 metrics expose the ways systems "cheat" or collapse in real-world usage:

| Metric | Run 1 | Run 2 | Run 3 | Description |
|--------|-------|-------|-------|-------------|
| **WER** (Wrong-Evidence Rate) | X% | X% | X% | 1 - E-Acc |
| **Cross-Space Confusion Rate** | X% | X% | X% | % citing files from wrong repo |
| **Bottom-Decile Score (p10)** | X.XX | X.XX | X.XX | Worst 10% performance floor |
| **Completion Rate** | X% | X% | X% | Graded / Expected |

## File References
- answers_baseline_run1.jsonl
- answers_baseline_run2.jsonl
- answers_baseline_run3.jsonl
- answers_mdemg_run1.jsonl
- answers_mdemg_run2.jsonl
- answers_mdemg_run3.jsonl
- grades_baseline_run1.json
- grades_baseline_run2.json
- grades_baseline_run3.json
- grades_mdemg_run1.json
- grades_mdemg_run2.json
- grades_mdemg_run3.json
- run_manifest.json
- grade_answers.py
```

### JSONL Answer Format (MANDATORY)

Each answer MUST be a single JSON line with these exact fields:

```json
{"id": "q001", "question": "What is MAX_TAKE?", "answer": "MAX_TAKE is 1000, defined in pagination.constants.ts:42", "files_consulted": ["src/pagination/pagination.constants.ts"], "file_line_refs": ["pagination.constants.ts:42"], "mdemg_skill_used": "retrieve", "confidence": "HIGH"}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | YES | Question ID from test set |
| `question` | string | YES | Full question text |
| `answer` | string | YES | Agent's answer (must include value + file:line) |
| `files_consulted` | array | YES | All files read/retrieved |
| `file_line_refs` | array | YES | Specific file:line citations |
| `mdemg_skill_used` | string | YES for MDEMG | "consult" or "retrieve" |
| `confidence` | string | YES | "HIGH", "MEDIUM", or "LOW" |

**CRITICAL:** Agents MUST append one line per answer to the output file. Do NOT output JSON arrays or pretty-printed JSON.

### Real-Time Guardrails (MANDATORY)

Implement these guardrails to prevent run failures before they happen:

#### Hard-Stop Rules

| Condition | Action | Rationale |
|-----------|--------|-----------|
| **3 consecutive answers missing line numbers** | Auto-interrupt, inject correction | Prevents evidence degradation |
| **No new output for 2 minutes** | Stop run, mark INVALID | Prevents context exhaustion stalls |
| **Duplicate question ID detected** | Stop run, mark INVALID | Data integrity violation |
| **JSON parse error in output** | Stop run, mark INVALID | Output corruption |
| **Metadata dumping detected** | Stop run, mark INVALID | Defective agent behavior |

#### Defective Agent Detection (Critical Lesson from Megatron-LM Benchmark)

Agents may exhibit "metadata dumping" behavior where they output raw MDEMG retrieval metadata instead of synthesized answers. Detect and immediately stop runs when answers contain these patterns:

| Pattern | Example | Action |
|---------|---------|--------|
| `Package: __init__` | Agent dumping element descriptions | STOP, mark INVALID |
| `Module: ... Contains N functions` | Raw MDEMG module metadata | STOP, mark INVALID |
| `Related to: authentication, error-handling` | Raw concern tags | STOP, mark INVALID |
| 10+ refs/question average | Dumping all retrieval results | Flag for investigation |
| Avg answer length > 150 chars with low semantic score | Nonsense padding | Flag for investigation |

**Detection Script:**

```python
def detect_metadata_dumping(answer_text):
    """Detect if answer contains raw MDEMG metadata instead of synthesized response."""
    patterns = [
        r'Package:\s*__init__',
        r'Module:.*Contains \d+ functions',
        r'Related to:\s*\w+,\s*\w+',
        r'Python module:',
        r'File:\s+.*\.py\s*\nImports:',
    ]
    for pattern in patterns:
        if re.search(pattern, answer_text):
            return True
    return False
```

### Run Validity Criteria

A run is **VALID** if:

- [ ] 100% questions answered
- [ ] No duplicate IDs
- [ ] No disqualification events (WebSearch, out-of-repo access)
- [ ] No agent restarts mid-run
- [ ] Output passes validation
- [ ] No metadata dumping detected

A run is **PARTIAL** if:

- Questions answered < 100% but > 90%
- No disqualification events

A run is **INVALID** if:

- Disqualification event occurred
- Agent was restarted mid-run
- Duplicate IDs found
- < 90% questions answered
- Metadata dumping detected

### Standardized Summary Schema

All benchmarks MUST produce a structured summary following the schema defined in `/docs/tests/blueseer/BENCHMARK_FRAMEWORK_V2.md` section 10. Key required fields:

```json
{
  "$schema": "benchmark_summary_v2",
  "metadata": { "benchmark_id": "...", "date": "...", "framework_version": "2.0" },
  "environment": { "mdemg_version": "...", "target_repo": {...} },
  "runs": [{ "run_id": "...", "status": "valid|partial|invalid", "grading": {...} }],
  "aggregate": { "baseline": {...}, "mdemg": {...}, "comparison": {...} },
  "findings": { "excluded_runs": [...], "key_insights": [...] }
}
```

---

## QUICK START CHECKLIST

Use this checklist to run a benchmark from scratch. Each step must be completed in order.

### Pre-Flight (Before Starting)

- [ ] **1. Verify MDEMG is running**: `curl -s localhost:9999/healthz`
- [ ] **2. Verify Neo4j is running**: `curl -s localhost:7474`
- [ ] **3. Verify space exists**: `curl -s 'localhost:9999/v1/memory/stats?space_id=<SPACE_ID>'`
- [ ] **4. Verify question files exist**:
  - [ ] Agent file (NO answers): `ls <path>/test_questions_*_agent.json`
  - [ ] Master file (HAS answers): `ls <path>/test_questions_*.json` (not `_agent`)
- [ ] **5. Record preflight receipts**:

  ```bash
  git rev-parse HEAD  # MDEMG commit
  cd <repo_path> && git rev-parse HEAD  # Repo commit
  shasum -a 256 <questions_agent.json>  # Question set hash
  ```

### Cold Start (Before Run 1 Only)

- [ ] **6. Clear learning edges**:

  ```bash
  docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword \
    "MATCH ()-[r:CO_ACTIVATED_WITH {space_id: '<SPACE_ID>'}]-() DELETE r RETURN count(r)"
  ```

- [ ] **7. Record initial edge count**: Should be 0

### Run Benchmarks (3 Runs Each)

- [ ] **8. Run Baseline condition (3 runs in PARALLEL)**:
  - Baseline runs have no dependencies - spawn all 3 agents simultaneously
  - [ ] Run 1: Spawn agent with `baseline_agent_prompt.txt`, output to `answers_baseline_run1.jsonl`
  - [ ] Run 2: Spawn agent with `baseline_agent_prompt.txt`, output to `answers_baseline_run2.jsonl`
  - [ ] Run 3: Spawn agent with `baseline_agent_prompt.txt`, output to `answers_baseline_run3.jsonl`

- [ ] **9. Run MDEMG condition (3 runs SEQUENTIALLY)**:
  - **CRITICAL:** MDEMG runs MUST be sequential (one at a time)
  - Learning edges develop during each run, improving retrieval for subsequent runs
  - Expected: MDEMG scores should IMPROVE with each run due to edge accumulation
  - [ ] Run 1: Spawn agent, wait for completion, then proceed to Run 2
  - [ ] Run 2: Spawn agent, wait for completion, then proceed to Run 3
  - [ ] Run 3: Spawn agent, wait for completion

### Grading (After All Runs Complete)

- [ ] **10. Grade each run**:

  ```bash
  python grade_answers.py answers_baseline_run1.jsonl test_questions_master.json grades_baseline_run1.json
  python grade_answers.py answers_baseline_run2.jsonl test_questions_master.json grades_baseline_run2.json
  python grade_answers.py answers_baseline_run3.jsonl test_questions_master.json grades_baseline_run3.json
  python grade_answers.py answers_mdemg_run1.jsonl test_questions_master.json grades_mdemg_run1.json
  python grade_answers.py answers_mdemg_run2.jsonl test_questions_master.json grades_mdemg_run2.json
  python grade_answers.py answers_mdemg_run3.jsonl test_questions_master.json grades_mdemg_run3.json
  ```

### Aggregate & Report

- [ ] **11. Run test harness** (or manually aggregate):

  ```bash
  python run_benchmark_harness.py --config benchmark_config.json
  ```

- [ ] **12. Verify output files**:
  - [ ] `aggregate_report.json` exists
  - [ ] All 6 `grades_*.json` files exist
  - [ ] All 6 `answers_*.jsonl` files exist

- [ ] **13. Generate markdown summary report**:
  - [ ] Create `BENCHMARK_RESULTS_V{X}.md` following the mandatory format
  - [ ] Include all standardized sections (Overview, Config, Baseline Results, MDEMG Results, Comparison, Learning Progression)
  - [ ] Include **Skepticism Reduction Metrics** table (Wrong-Evidence, Cross-Space, p10, Completion)
  - [ ] Reference ALL generated files in the File References section
  - [ ] Include learning edge counts after each MDEMG run

### Final Validation

- [ ] **14. Check results**:
  - [ ] Mean score reported for both conditions
  - [ ] CV% < 15% (indicates consistent runs)
  - [ ] Verdict is PASS or FAIL with clear delta
  - [ ] Markdown summary report is complete and references all files

---

## STANDARDIZED METRICS

### Codebase Metrics (Collected Once Per Repo)

Capture BEFORE running benchmarks. Store in `codebase_profile.json`.

| Metric | Field Name | How to Collect | Example |
|--------|------------|----------------|---------|
| Total Files | `total_files` | `find . -type f \| wc -l` (filtered) | 847 |
| Total LOC | `total_loc` | `wc -l` on all source files | 124,532 |
| File Types | `file_types` | Extension breakdown | `{"ts": 423, "tsx": 89, "json": 45}` |
| Module Count | `module_count` | Count top-level directories in src/ | 12 |
| Avg File Size (LOC) | `avg_file_loc` | total_loc / total_files | 147 |
| Max File Size (LOC) | `max_file_loc` | Largest single file | 2,341 |
| Dependency Count | `dependency_count` | package.json dependencies | 87 |
| Test Coverage % | `test_coverage_pct` | From coverage tool or estimate | 65.2 |
| Documentation % | `doc_coverage_pct` | Files with JSDoc/docstrings | 42.0 |
| Repo Commit | `repo_commit` | `git rev-parse HEAD` | "abc123..." |
| Repo URL | `repo_url` | Origin URL or local path | "/Users/x/repo" |

### Test Metrics (Collected Per Question Per Run)

Store in `answers_run{N}.jsonl` with each answer line.

| Metric | Field Name | How to Collect | Example |
|--------|------------|----------------|---------|
| Question ID | `id` | From question set | "q001" |
| Answer Text | `answer` | Agent output | "MAX_TAKE is 1000..." |
| Files Consulted | `files_consulted` | Agent tracking | ["pagination.ts"] |
| File:Line Refs | `file_line_refs` | Agent citations | ["pagination.ts:42"] |
| MDEMG Skill | `mdemg_skill_used` | "consult" or "retrieve" | "retrieve" |
| Confidence | `confidence` | Agent self-assessment | "HIGH" |
| Latency (ms) | `latency_ms` | Time to answer | 1234 |
| Tokens In | `tokens_in` | Input tokens | 2500 |
| Tokens Out | `tokens_out` | Output tokens | 450 |
| Tool Calls | `tool_call_count` | Number of tool invocations | 3 |
| MDEMG Nodes | `mdemg_nodes_retrieved` | Nodes returned by MDEMG | 8 |
| Timestamp | `timestamp` | ISO 8601 | "2026-01-26T15:30:00Z" |

### Grading Metrics (Computed Per Question)

Store in `grades_run{N}.json` after automated grading.

| Metric | Field Name | How to Compute | Example |
|--------|------------|----------------|---------|
| Value Score | `value_score` | 1.0 if expected value in answer, else 0.0 | 1.0 |
| Keyword Score | `keyword_score` | keywords_found / keywords_expected | 0.85 |
| Final Score | `score` | 0.70 *value_score + 0.30* keyword_score | 0.955 |
| Expected Value | `expected_value` | From answer key | "1000" |
| Expected Keywords | `expected_keywords` | From answer key | ["MAX_TAKE", "pagination"] |
| Keywords Found | `keywords_found` | Matched keywords | ["MAX_TAKE", "pagination"] |
| Evidence Found | `evidence_found` | Boolean: file:line present | true |
| Correct File | `correct_file_cited` | Expected file in citations | true |

### Summary Metrics (Aggregated Across All Questions and Runs)

Store in `aggregate_report.json`.

#### Quick Reference Table

| Metric | Formula | Example |
|--------|---------|---------|
| Mean | Σ scores / N | 0.742 |
| Std Dev | √(Σ(x-μ)²/N) | 0.089 |
| CV | 100 * σ / μ | 12.0% |
| Median | Middle value | 0.78 |
| Min | Lowest score | 0.21 |
| Max | Highest score | 1.00 |
| p10 | 10th percentile | 0.55 |
| p90 | 90th percentile | 0.92 |
| Completion Rate | answered / total | 100% |
| High Score Rate | score >= 0.7 / total | 68% |
| Evidence Rate | evidence_found / total | 95% |
| Correct File Rate | correct_file_cited / total | 82% |
| Auto-Compact Events | Count per run | 3 (baseline avg) |

#### Detailed Sections

**Core Metrics:**

```json
{
  "core": {
    "mean": 0.742,
    "std": 0.089,
    "cv_pct": 12.0,
    "median": 0.78,
    "min": 0.21,
    "max": 1.00,
    "p10": 0.55,
    "p25": 0.65,
    "p75": 0.85,
    "p90": 0.92,
    "completion_rate": 1.0,
    "high_score_rate": 0.68
  }
}
```

**Grounding Metrics:**

```json
{
  "grounding": {
    "evidence_compliance_rate": 0.95,
    "evidence_accuracy_rate": 0.88,
    "hard_value_resolution_rate": 0.82,
    "correct_file_rate": 0.82,
    "hallucination_rate": 0.03,
    "guess_rate": 0.12
  }
}
```

**Efficiency Metrics:**

```json
{
  "efficiency": {
    "latency_p50_ms": 1234,
    "latency_p95_ms": 3456,
    "tokens_per_question_avg": 2950,
    "tokens_per_question_p95": 4500,
    "tool_calls_per_question_avg": 3.2,
    "tool_calls_per_question_p95": 7,
    "total_wall_time_sec": 1847,
    "auto_compact_events": {
      "baseline_run1": 3,
      "baseline_run2": 4,
      "baseline_run3": 3,
      "baseline_total": 10,
      "mdemg_run1": 0,
      "mdemg_run2": 0,
      "mdemg_run3": 0,
      "mdemg_total": 0
    }
  }
}
```

**Learning & State Persistence Metrics (Persistent Memory Advantage):**

This section captures the fundamental advantage of MDEMG: **state persistence under context churn**. While baseline agents lose state every time the context is truncated, MDEMG maintains a durable "Internal Dialog" that survives compaction events.

| Metric | Name | Definition | Why it matters |
|--------|------|------------|----------------|
| **CSC** | Compaction Survival Curve | `Score(k)` where `k` is the compaction count | Baseline should show stepwise degradation; MDEMG should stay flat. |
| **PCD** | Post-Compaction Delta | `Δ score` immediately following a compaction | Makes "forgetting" measurable (Mean drop, p10 drop). |
| **DP@K** | Decision Persistence at K | `%` of early commitments remembered after `K` compactions | Proves "long-context" doesn't mean "long-term memory". |
| **RRAC** | Repeat Rate after Compaction | `%` of turns repeating prior questions/work after reset | Detects "looping" failure modes in baseline. |
| **CCC** | Context Churn Cost | Tokens + tool calls needed to recover state after reset | Shows the efficiency penalty of "working-memory-only" agents. |
| **SoWI** | State-of-Work Integrity | Rubric-based coherence score (objective, constraints, next step) | Measures the coherence of the agent's narrative. |

**Isolation & Reliability Metrics (Multi-Corpus Validation):**

| Metric | Name | Definition | Why it matters |
|--------|------|------------|----------------|
| **RAA** | Repo Attribution Accuracy | `%` of answers correctly identifying the source corpus | Proves the graph correctly partitions knowledge. |
| **CRCR** | Cross-Repo Contamination Rate | `%` of answers citing nodes from the wrong `space_id` | Proves zero mix-ups in multi-tenant environments. |
| **E-Acc** | Evidence Accuracy | `%` of cited `file:line` refs that actually support the claim | Credibility killer; punishes "citation spam". |
| **WER** | Wrong Evidence Rate | `%` of answers with "hallucinated" or irrelevant citations | Measures the "confidence trick" failure mode of standard RAG. |

### Compaction Defined

A **Compaction Event** is any time the agent's working context is truncated or replaced (e.g., `auto-compact`, summarization, session restart).

### Collection Protocol: The "Compaction Ladder"

1. **Plant Commitments**: Give the agent 10-15 questions that establish a working set of decisions (naming conventions, invariants, defaults).
2. **Forced Compaction**: Force a compaction/restart every N questions.
3. **Trace Persistence**: Continue 50-100 more questions that depend on those earlier decisions.
4. **Calculate Metrics**: Compare baseline (auto_compact=on, memory=off) vs MDEMG.

```json
{
  "persistent_memory_stress_test": {
    "csc_curve": {
      "k0_initial_score": 0.85,
      "k1_after_reset_1": 0.84,
      "k2_after_reset_2": 0.83,
      "decay_slope": -0.01
    },
    "dp_at_k": {
      "commitments_planted": 10,
      "remembered_after_k3": 9,
      "dp_score": 0.90
    },
    "rrac": {
      "repeated_subtasks": 2,
      "total_subtasks": 100,
      "repeat_rate_pct": 2.0
    },
    "ccc": {
      "recovery_tokens_avg": 450,
      "recovery_tool_calls_avg": 1.2
    }
  }
}
```

**Key Metrics Explained:**

| Metric | Description | How to Collect |
|--------|-------------|----------------|
| `learning_edges.after_runN` | CO_ACTIVATED_WITH edge count after run N | Neo4j: `MATCH ()-[r:CO_ACTIVATED_WITH]->() RETURN count(r)` |
| `learning_edges.growth_rate_per_query` | Avg edges created per query | (final_edges - initial_edges) / total_queries |
| `retrieval_improvement.runN_avg_nodes` | Avg MDEMG nodes returned per query in run N | Track `mdemg_nodes_retrieved` per answer |
| `retrieval_improvement.runN_evidence_score` | Avg evidence score in run N | From grading results |
| `context_persistence.baseline_context_resets` | Auto-compact events (baseline loses all context) | Track in agent summary |
| `token_efficiency.*` | Token consumption comparison | Track tokens per answer |

**Expected Patterns:**

- Learning edges should **increase** across MDEMG runs 1→2→3
- Evidence scores should **improve** as edges strengthen retrieval
- Baseline auto-compacts should be **higher** than MDEMG (context pressure)
- MDEMG token efficiency should be **better** (graph hints reduce search)

**Reproducibility Metadata:**

```json
{
  "reproducibility": {
    "run_count": 3,
    "run_means": [0.738, 0.745, 0.743],
    "run_stds": [0.091, 0.087, 0.089],
    "question_set_sha256": "abc123...",
    "question_set_file": "test_questions_120_agent.json",
    "mdemg_commit": "def456...",
    "cold_start": true,
    "initial_learning_edges": 0,
    "final_learning_edges": 8234
  }
}
```

**Per-Category Breakdown:**

```json
{
  "by_category": {
    "symbol-lookup": {"mean": 0.85, "count": 20},
    "multi-file": {"mean": 0.72, "count": 59},
    "cross-module": {"mean": 0.68, "count": 38},
    "system-wide": {"mean": 0.55, "count": 3}
  }
}
```

---

## Table of Contents

**CANONICAL SPECIFICATION (READ FIRST)**

- [Canonical Benchmark Specification](#canonical-benchmark-specification)
- [Quick Start Checklist](#quick-start-checklist)
- [Standardized Metrics](#standardized-metrics)
- [Learning & Persistence Metrics](#learning--persistence-metrics-mdemg-key-differentiator)

**SETUP & EXECUTION**

1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Phase 1: Codebase Preparation](#phase-1-codebase-preparation)
4. [Phase 2: Test Question Development](#phase-2-test-question-development)
5. [Phase 3: MDEMG Setup and Ingestion](#phase-3-mdemg-setup-and-ingestion)
6. [Phase 4: Running Tests](#phase-4-running-tests)
7. [Phase 5: Analysis and Reporting](#phase-5-analysis-and-reporting)

**REFERENCE**
8. [Roles and Responsibilities](#roles-and-responsibilities)
9. [Common Mistakes to Avoid](#common-mistakes-to-avoid)
10. [V6 Composite Test Set](#v6-composite-test-set)
11. [WHK-WMS 120-Question Test Set](#whk-wms-120-question-test-set)
12. [Baseline Benchmarking Rules](#baseline-benchmarking-rules)
13. [Required Benchmark Metrics](#required-benchmark-metrics)
14. [End-to-End Multi-Corpus Runbook](#end-to-end-multi-corpus-runbook)
15. [Appendix: Templates and Scripts](#appendix-templates-and-scripts)

---

## Overview

### What We're Testing

MDEMG (Multi-Dimensional Emergent Memory Graph) provides AI agents with persistent, retrievable memory for codebases. The benchmark compares:

| Approach | Description | Expected Behavior |
|----------|-------------|-------------------|
| **Baseline** | Traditional context window approach - read files, rely on context compression | Context compression loses critical details; accuracy degrades with codebase size |
| **MDEMG** | Retrieve relevant context via semantic search + graph activation | Maintains access to all indexed content; accuracy scales with codebase size |

### Success Metrics

| Metric | Description | Target |
|--------|-------------|--------|
| **Avg Score** | Mean score across all questions (0.0-1.0) | MDEMG > 0.65, Baseline < 0.15 |
| **Delta** | MDEMG score - Baseline score | > +0.50 |
| **>0.7 Rate** | Percentage of questions with score > 0.7 | MDEMG > 50% |
| **Learning Edge Growth** | CO_ACTIVATED_WITH edges created during test | > 20 pairs/query |

### Historical Results

| Version | Avg Score | Improvement | Key Feature |
|---------|-----------|-------------|-------------|
| Baseline | 0.050 | - | Context window only |
| v8 | 0.562 | +0.512 | Path/comparison boost |
| v9 | 0.619 | +10.2% vs v8 | LLM re-ranking |
| v10 | **0.710** | +14.6% vs v9 | Learning edge fix |

---

## Prerequisites

### System Requirements

```bash
# Hardware
- 16GB+ RAM (32GB recommended)
- 50GB+ free disk space
- macOS, Linux, or WSL2

# Software
- Docker & Docker Compose
- Go 1.21+
- Python 3.10+
- Node.js 18+ (if ingesting JS/TS codebases)
- Claude CLI (for running agent tests)
```

### Required Services

```bash
# 1. Neo4j (graph database)
docker compose up -d neo4j
# Verify: http://localhost:7474 (neo4j/testpassword)

# 2. MDEMG Service
cd mdemg_build/service
export NEO4J_URI=bolt://localhost:7687
export NEO4J_USER=neo4j
export NEO4J_PASS=testpassword
export REQUIRED_SCHEMA_VERSION=4
export VECTOR_INDEX_NAME=memNodeEmbedding
export OPENAI_API_KEY=sk-...  # For embeddings
go run ./cmd/server

# Verify: curl localhost:9999/healthz
```

---

## Phase 1: Codebase Preparation

### Step 1.1: Clone/Locate the Repository

```bash
# Option A: Clone from URL
git clone https://github.com/org/repo.git /path/to/repo
cd /path/to/repo

# Option B: Use existing local path
cd /path/to/existing/repo
```

### Step 1.2: Generate File List

```bash
# Create file list (exclude binaries, node_modules, etc.)
find /path/to/repo -type f \
  -name "*.ts" -o -name "*.tsx" -o -name "*.js" -o -name "*.jsx" \
  -o -name "*.py" -o -name "*.go" -o -name "*.java" \
  -o -name "*.rs" -o -name "*.c" -o -name "*.cpp" \
  -o -name "*.json" -o -name "*.yaml" -o -name "*.yml" \
  -o -name "*.md" -o -name "*.sql" \
  | grep -v node_modules \
  | grep -v dist \
  | grep -v build \
  | grep -v .git \
  | sort > /path/to/tests/file-list.txt

# Count files
wc -l /path/to/tests/file-list.txt
```

### Step 1.3: Analyze Codebase Structure

Before creating questions, understand the codebase:

```bash
# Get directory structure
tree -d -L 3 /path/to/repo > /path/to/tests/structure.txt

# Count lines by file type
find /path/to/repo -name "*.ts" -exec wc -l {} + | tail -1
find /path/to/repo -name "*.py" -exec wc -l {} + | tail -1

# Identify key modules
ls /path/to/repo/src/
ls /path/to/repo/apps/*/src/
```

---

## Phase 2: Test Question Development

### Step 2.1: Question Categories

Create questions across 5 categories (20 questions each for 100 total):

| Category | Description | Example Topics |
|----------|-------------|----------------|
| `architecture_structure` | Module organization, dependency patterns | "Why is Module X marked @Global?" |
| `service_relationships` | Inter-service dependencies, injection patterns | "What services does X inject?" |
| `business_logic_constraints` | Domain rules, validation logic | "What prevents overlapping ownership?" |
| `data_flow_integration` | Request/response flows, data transformations | "Trace the flow when X happens" |
| `cross_cutting_concerns` | Auth, logging, error handling, caching | "How does audit logging work?" |

### Step 2.2: Question Quality Requirements

**CRITICAL:** Questions must be:

1. **Multi-file** - Require understanding 2+ files to answer correctly
2. **Verifiable** - Have concrete, code-referenced answers
3. **Non-trivial** - Cannot be answered from a single function or enum
4. **Specific** - Avoid vague questions like "How does X work?"

### Step 2.3: Question JSON Format

```json
{
  "metadata": {
    "source": "path/to/repo",
    "total_questions": 100,
    "generated_at": "2026-01-23T12:00:00Z"
  },
  "questions": [
    {
      "id": 1,
      "category": "service_relationships",
      "question": "What services does BarrelEventService depend on for handling entry events at holding locations?",
      "answer": "BarrelEventService constructor injects: 1) PrismaService - database operations; 2) AndroidSyncErrorLoggerService (errorLogger) - logs warnings/errors; 3) LotVariationService - normalizes serial numbers; 4) FeatureFlagsService - controls feature behavior; 5) CachedEventTypeService - avoids in-transaction queries.",
      "required_files": [
        "/path/to/repo/src/barrelEvent/barrelEvent.service.ts",
        "/path/to/repo/src/androidSyncInbox/android-sync-error-logger.service.ts",
        "/path/to/repo/src/lotVariation/lotVariation.service.ts"
      ],
      "complexity": "multi-file"
    }
  ]
}
```

### Step 2.4: Question Generation Process

1. **Explore the codebase** to understand architecture
2. **Identify cross-cutting patterns** (DI, error handling, etc.)
3. **Write questions that require synthesis** from multiple files
4. **Verify answers against actual code** before finalizing
5. **Include file:line references** in expected answers

### Step 2.5: Verification Process

**CRITICAL:** Verify ALL answers before using in tests.

```bash
# Batch verification approach
# Split questions into batches of 4
# Have Claude verify each batch against actual code
# Track corrections in corrections.json
```

Common verification findings:

- ~30-35% of LLM-generated answers contain errors
- Most errors: wrong method names, overstated functionality, incorrect enums

---

## Phase 3: MDEMG Setup and Ingestion

### Step 3.1: Create Space

```bash
# Create a new space for the codebase
curl -s -X POST 'http://localhost:9999/v1/spaces' \
  -H 'content-type: application/json' \
  -d '{"space_id": "your-project-name", "description": "Description of project"}'
```

### Step 3.2: Ingest Codebase

**Option A: Using ingest-codebase tool**

```bash
cd mdemg_build/service
go run ./cmd/ingest-codebase \
  --space-id=your-project-name \
  --path=/path/to/repo \
  --file-list=/path/to/tests/file-list.txt
```

**Option B: Using batch API**

```bash
# For each batch of files
curl -s -X POST 'http://localhost:9999/v1/memory/batch-ingest' \
  -H 'content-type: application/json' \
  -d '{
    "space_id": "your-project-name",
    "items": [
      {"path": "<file_path>", "content": "<file_content>", "content_type": "code"}
    ]
  }'
```

### Step 3.3: Verify Ingestion

```bash
# Check node count
curl -s 'http://localhost:9999/v1/memory/stats?space_id=your-project-name' | jq

# Expected output:
# {
#   "node_count": 8906,
#   "edge_count": 94654,
#   "learning_activity": {
#     "co_activated_edges": 0
#   }
# }
```

### Step 3.4: Run Consolidation (Optional)

```bash
# Build hidden layer concepts
curl -s -X POST 'http://localhost:9999/v1/memory/consolidate' \
  -H 'content-type: application/json' \
  -d '{"space_id":"your-project-name"}' | jq
```

---

## Phase 4: Running Tests

### Benchmark Type Selection (CRITICAL)

**ALWAYS use Agent-Based Benchmarks.** There are two benchmark methodologies - only one is valid:

| Type | Description | Valid? |
|------|-------------|--------|
| ❌ **Retrieval Quality** | Query API directly, measure file_match_score | **NO** - Does not test agent workflows |
| ✅ **Agent-Based** | Agent uses MDEMG skills to answer questions, answers graded | **YES** - Tests real-world use case |

**Why Agent-Based benchmarks are required:**

1. Tests the actual agent workflow (skill invocation → context retrieval → answer synthesis)
2. Measures answer quality, not just retrieval scores
3. Validates MDEMG skills integration works correctly
4. Produces graded results comparable to baseline agent tests

**Agent-Based Benchmark Flow:**

```
Agent receives question
    ↓
Agent uses /mdemg consult or /mdemg retrieve skill
    ↓
Agent reads retrieved context
    ↓
Agent synthesizes answer with file:line citations
    ↓
Answer graded against expected answer (0.0-1.0)
```

### Step 4.1: Cold Start (Reset Learning Edges) ⚠️ CRITICAL

For reproducible results, reset CO_ACTIVATED_WITH edges before each test:

```bash
# Connect to Neo4j
docker exec -it mdemg-neo4j cypher-shell -u neo4j -p testpassword

# Delete learning edges for the space
MATCH ()-[r:CO_ACTIVATED_WITH {space_id: 'your-project-name'}]-()
DELETE r
RETURN count(r) as deleted;
```

**Learning Edge Behavior:**

Learning edges (CO_ACTIVATED_WITH) are created during MDEMG retrieval to capture query-node co-activation patterns. Their impact on benchmark scores depends on context:

| Scenario | Edge Behavior | Rationale |
|----------|---------------|-----------|
| Baseline vs MDEMG comparison | Clear before Run 1 | Fair comparison starts from same baseline |
| Within MDEMG multi-run | Let edges accumulate | Edges improve retrieval for subsequent queries |
| Fresh benchmark (new question set) | Clear all edges | Prevent leakage from prior benchmarks |

**Expected behavior across MDEMG runs:**

- **Run 1:** Cold start, no edges → baseline MDEMG performance
- **Run 2:** Edges from Run 1 help retrieval → improved scores expected
- **Run 3:** Accumulated edges → further improvement expected

**Best Practice:** Clear learning edges ONCE before Run 1 of the benchmark, then let them accumulate across the 3 MDEMG runs. This tests whether learning edges provide cumulative benefit.

### Step 4.2: MDEMG Test Script

Create `run_mdemg_test.py`:

```python
#!/usr/bin/env python3
"""MDEMG Retrieval Test"""

import json
import urllib.request
import time
from pathlib import Path
from datetime import datetime
from collections import defaultdict

TEST_DIR = Path(__file__).parent
QUESTIONS_FILE = TEST_DIR / "test_questions.json"
OUTPUT_FILE = TEST_DIR / f"mdemg-test-{datetime.now().strftime('%Y%m%d-%H%M%S')}.md"
MDEMG_ENDPOINT = "http://localhost:9999"
SPACE_ID = "your-project-name"

def load_questions():
    with open(QUESTIONS_FILE) as f:
        return json.load(f)['questions']

def query_mdemg(question: str) -> dict:
    try:
        data = json.dumps({
            "space_id": SPACE_ID,
            "query_text": question,
            "candidate_k": 50,
            "top_k": 10,
            "hop_depth": 2
        }).encode('utf-8')
        req = urllib.request.Request(
            f"{MDEMG_ENDPOINT}/v1/memory/retrieve",
            data=data,
            headers={'Content-Type': 'application/json'}
        )
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read().decode('utf-8'))
    except Exception as e:
        return {"error": str(e)}

def get_edge_count() -> int:
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/v1/memory/stats?space_id={SPACE_ID}")
        with urllib.request.urlopen(req, timeout=5) as resp:
            data = json.loads(resp.read().decode('utf-8'))
            return data.get('learning_activity', {}).get('co_activated_edges', 0)
    except:
        return 0

def run_test():
    print("=" * 60)
    print("MDEMG RETRIEVAL TEST")
    print("=" * 60)

    # Health check
    try:
        req = urllib.request.Request(f"{MDEMG_ENDPOINT}/healthz")
        with urllib.request.urlopen(req, timeout=5) as resp:
            print(f"MDEMG Status: {resp.read().decode('utf-8')}")
    except Exception as e:
        print(f"ERROR: MDEMG not reachable: {e}")
        return

    questions = load_questions()
    print(f"Loaded {len(questions)} questions")

    initial_edges = get_edge_count()
    print(f"Initial CO_ACTIVATED_WITH edges: {initial_edges}")
    print("-" * 60)

    results = []
    start_time = time.time()

    for i, q in enumerate(questions, 1):
        qtext = q['question']
        category = q['category']
        resp = query_mdemg(qtext)

        if "error" in resp:
            print(f"Q{i}: ERROR - {resp['error']}")
            results.append({"id": q['id'], "category": category, "score": 0})
            continue

        nodes = resp.get('results', [])
        top_score = nodes[0].get('score', 0) if nodes else 0

        results.append({
            "id": q['id'],
            "category": category,
            "score": top_score
        })

        if i % 10 == 0:
            elapsed = time.time() - start_time
            rate = i / elapsed
            current_edges = get_edge_count()
            new_edges = current_edges - initial_edges
            print(f"Progress: {i}/{len(questions)} ({rate:.2f} q/s) - Score: {top_score:.3f}, Edges: +{new_edges}")

    total_time = time.time() - start_time
    final_edges = get_edge_count()

    # Analysis
    scores = [r['score'] for r in results]
    by_category = defaultdict(list)
    for r in results:
        by_category[r['category']].append(r['score'])

    avg_score = sum(scores) / len(scores)

    print("\n" + "=" * 60)
    print("TEST COMPLETE")
    print("=" * 60)
    print(f"Avg Score: {avg_score:.3f}")
    print(f"Duration: {total_time:.1f}s")
    print(f"Learning Edges: {initial_edges} -> {final_edges} (+{final_edges - initial_edges})")
    print(f"\nBy Category:")
    for cat, vals in sorted(by_category.items()):
        print(f"  {cat}: {sum(vals)/len(vals):.3f}")

    # Write report
    with open(OUTPUT_FILE, 'w') as f:
        f.write(f"# MDEMG Test Results\n\n")
        f.write(f"**Date**: {datetime.now().isoformat()}\n")
        f.write(f"**Duration**: {total_time:.1f}s\n\n")
        f.write(f"## Summary\n\n")
        f.write(f"| Metric | Value |\n")
        f.write(f"|--------|-------|\n")
        f.write(f"| Avg Score | {avg_score:.3f} |\n")
        f.write(f"| Max Score | {max(scores):.3f} |\n")
        f.write(f"| Min Score | {min(scores):.3f} |\n")
        f.write(f"| Learning Edges Created | {final_edges - initial_edges} |\n")

    print(f"\nReport: {OUTPUT_FILE}")
    return {"avg": avg_score, "by_category": {c: sum(v)/len(v) for c,v in by_category.items()}}

if __name__ == "__main__":
    run_test()
```

### Step 4.3: Baseline Test Agent Prompt (CANONICAL)

**This is the EXACT prompt to use. Do not modify.**

Save as `baseline_agent_prompt.txt` and pass to Task agent:

```
BASELINE BENCHMARK - JSONL OUTPUT MODE

You are answering questions about a codebase WITHOUT access to MDEMG memory.
You MAY use Read, Glob, Grep tools to search the codebase directly.
You MAY NOT use /mdemg skills or access MDEMG API.

QUESTION FILE: {{QUESTION_FILE}}
OUTPUT FILE: {{OUTPUT_FILE}}
REPO PATH: {{REPO_PATH}}
TIME LIMIT: 20 minutes
QUESTION COUNT: {{QUESTION_COUNT}}

INSTRUCTIONS:
1. Read the question file (JSON with "questions" array)
2. You MUST answer ALL {{QUESTION_COUNT}} questions. Do not stop early.
3. Track AUTO-COMPACT EVENTS: Each time you notice your context has been summarized/compacted, increment your compact counter.
4. For EACH question in order:
   a. Search the codebase using Read/Glob/Grep
   b. Find the answer in source code
   c. **NOTE THE EXACT LINE NUMBER** - this is CRITICAL for scoring
   d. **VALIDATE before writing:** Does your answer include "filename:linenum"? If not, find it.
   e. Append ONE JSON line to the output file with file_line_refs populated
5. After all questions, append a SUMMARY line and output "BENCHMARK COMPLETE"

⚠️ FILE:LINE REFERENCES ARE MANDATORY FOR SCORING ⚠️
Your answer MUST include citations in format: "filename.ts:123"
The file_line_refs array MUST contain at least one "file:line" entry
Answers without file:line refs will score POORLY

OUTPUT FORMAT - Append ONE line per question (no pretty-printing):
{"id": "<question_id>", "question": "<full_question>", "answer": "<your_answer_with_file:line>", "files_consulted": ["<file1>", "<file2>"], "file_line_refs": ["<file:line>"], "confidence": "HIGH|MEDIUM|LOW", "timestamp": "<ISO8601>"}

FINAL SUMMARY LINE (append after last question):
{"type": "summary", "questions_answered": <count>, "auto_compact_events": <count>, "completed_at": "<ISO8601>"}

⚠️ WHAT NOT TO DO:
- DO NOT leave file_line_refs empty - ALWAYS include at least one "file:line" entry
- DO NOT write answers without a specific line number citation
- DO NOT stop before answering all questions

✅ WHAT TO DO:
- ALWAYS search for and READ the actual source files
- ALWAYS note the exact line number where you found the answer
- **ALWAYS INCLUDE file:line citation** (e.g., "service.ts:42") - THIS IS REQUIRED
- **ALWAYS populate file_line_refs array** with format ["filename.ts:linenum"]
- Before writing each answer, verify it contains a colon-separated file:line reference

RULES:
- You MUST answer ALL {{QUESTION_COUNT}} questions - stopping early invalidates the run
- Answer MUST include the specific value/constant if asked
- Answer MUST include file:line citation (e.g., "pagination.ts:42")
- file_line_refs array MUST have at least one entry - NEVER leave it empty
- If you cannot find the answer, set answer to "NOT_FOUND", confidence to "LOW", but still try to cite a relevant file
- Do NOT output anything except the JSON lines to the output file
- Do NOT use web search

CONFIDENCE LEVELS:
- HIGH: Found exact value in correct file with line number
- MEDIUM: Found relevant file but value not confirmed
- LOW: Could not locate or uncertain

BEGIN NOW - Read {{QUESTION_FILE}} and start answering ALL {{QUESTION_COUNT}} questions.
```

### Step 4.4: MDEMG Test Agent Prompt (CANONICAL)

**This is the EXACT prompt to use. Do not modify.**

Save as `mdemg_agent_prompt.txt` and pass to Task agent:

```
MDEMG BENCHMARK - JSONL OUTPUT MODE

You are answering questions about a codebase using MDEMG as a SEARCH INDEX.

CRITICAL UNDERSTANDING:
- MDEMG returns HINTS (file names, symbols) - NOT answers
- You MUST READ the files suggested by MDEMG to find actual values
- NEVER use MDEMG output as your answer - it's just a pointer to relevant files

REPOSITORY PATH: {{REPO_PATH}}
QUESTION FILE: {{QUESTION_FILE}}
OUTPUT FILE: {{OUTPUT_FILE}}
SPACE ID: {{SPACE_ID}}
MDEMG API: http://localhost:9999
TIME LIMIT: 20 minutes
QUESTION COUNT: {{QUESTION_COUNT}}

MDEMG API CALLS (use curl, NOT /mdemg skills):
- For "symbol-lookup" questions:
  curl -s 'http://localhost:9999/v1/memory/retrieve' -H 'content-type: application/json' \
    -d '{"space_id":"{{SPACE_ID}}","query_text":"<symbol_name>","top_k":10}'

- For ALL other questions:
  curl -s 'http://localhost:9999/v1/memory/consult' -H 'content-type: application/json' \
    -d '{"space_id":"{{SPACE_ID}}","context":"Answering benchmark question","question":"<full_question>"}'

WORKFLOW FOR EACH QUESTION (MANDATORY):
1. Call MDEMG API to get file/symbol hints
2. Parse the MDEMG response - extract file names from suggestions like:
   - "Module: barrel.module.ts in typescript" → read barrel.module.ts
   - "Export BarrelService in typescript" → search for BarrelService
3. READ the actual source files using Read tool at {{REPO_PATH}}/<filename>
4. Find the specific value, constant, or code pattern in the file
5. **NOTE THE EXACT LINE NUMBER** - this is CRITICAL for scoring
6. Synthesize your answer with the actual value AND file:line reference
7. **VALIDATE before writing:** Does your answer include "filename:linenum"? If not, go back and find it.
8. Write ONE JSON line to output file

⚠️ FILE:LINE REFERENCES ARE MANDATORY FOR SCORING ⚠️
Your answer MUST include citations in format: "filename.ts:123"
The file_line_refs array MUST contain at least one "file:line" entry
Answers without file:line refs will score POORLY

EXAMPLE WORKFLOW:
Question: "What is MAX_TAKE in pagination?"
1. curl MDEMG → returns {"suggestions": [{"content": "Module: pagination.constants.ts"}]}
2. Read {{REPO_PATH}}/src/pagination/pagination.constants.ts
3. Find line 42: export const MAX_TAKE = 1000;
4. Answer: "MAX_TAKE = 1000, defined in pagination.constants.ts:42"
5. file_line_refs: ["pagination.constants.ts:42"]  ← REQUIRED!

INSTRUCTIONS:
1. Read the question file (JSON with "questions" array)
2. You MUST answer ALL {{QUESTION_COUNT}} questions. Do not stop early.
3. For EACH question:
   a. Call MDEMG API (retrieve for symbol-lookup, consult for others)
   b. Extract file names from MDEMG suggestions
   c. READ those files using Read tool (MANDATORY - not optional)
   d. Find the actual value/code in the file
   e. Synthesize answer with value + file:line
   f. Append ONE JSON line to output file
4. After all questions, output "BENCHMARK COMPLETE"

⚠️ WHAT NOT TO DO (INSTANT FAILURE):
- DO NOT use MDEMG suggestion text as your answer
- DO NOT write answers like "Based on this codebase's patterns: ..."
- DO NOT write answers like "Module: foo.ts in typescript. Related to: ..."
- DO NOT skip reading the actual source files
- DO NOT guess values without reading the file
- DO NOT leave file_line_refs empty - ALWAYS include at least one "file:line" entry
- DO NOT write answers without a specific line number citation

✅ WHAT TO DO:
- USE MDEMG to find which files are relevant
- READ those files to find actual values
- INCLUDE the specific value/constant in your answer
- **ALWAYS INCLUDE file:line citation** (e.g., "service.ts:42") - THIS IS REQUIRED
- **ALWAYS populate file_line_refs array** with format ["filename.ts:linenum"]
- Before writing each answer, verify it contains a colon-separated file:line reference

OUTPUT FORMAT - Append ONE line per question (no pretty-printing):
{"id": "<question_id>", "question": "<full_question>", "answer": "<your_synthesized_answer_with_file:line>", "files_consulted": ["<file1>", "<file2>"], "file_line_refs": ["<file:line>"], "mdemg_api_used": "consult|retrieve", "mdemg_nodes_retrieved": <count>, "confidence": "HIGH|MEDIUM|LOW", "timestamp": "<ISO8601>"}

CONFIDENCE LEVELS:
- HIGH: Read the file AND found exact value with line number
- MEDIUM: Read the file but value not exactly matching question
- LOW: Could not read file or MDEMG returned no useful hints

BEGIN NOW - Read {{QUESTION_FILE}} and start answering ALL {{QUESTION_COUNT}} questions.
```

### Step 4.5: MDEMG API Configuration

**CRITICAL:** Sub-agents CANNOT access `/mdemg` skills via `allowed_tools`. Skills are defined in `.claude/commands/` but are NOT inherited by Task-spawned sub-agents.

**SOLUTION:** Use direct curl API calls to the MDEMG server instead of skills.

#### MDEMG API Endpoints

| Endpoint | Purpose | Example |
|----------|---------|---------|
| `GET /healthz` | Health check | `curl localhost:9999/healthz` |
| `POST /v1/memory/consult` | Get SME advice | See below |
| `POST /v1/memory/retrieve` | Search memories | See below |
| `GET /v1/memory/stats` | Space statistics | `curl 'localhost:9999/v1/memory/stats?space_id=whk-wms'` |

#### API Call Examples

```bash
# Verify MDEMG is accessible before spawning agents
curl -s localhost:9999/healthz

# Consult API - for complex questions (requires context + question)
curl -s 'http://localhost:9999/v1/memory/consult' \
  -H 'content-type: application/json' \
  -d '{"space_id":"whk-wms","context":"Answering benchmark question","question":"How should I handle authentication?"}'

# Retrieve API - for symbol lookups (uses query_text)
curl -s 'http://localhost:9999/v1/memory/retrieve' \
  -H 'content-type: application/json' \
  -d '{"space_id":"whk-wms","query_text":"JwtStrategy","top_k":10}'
```

#### Task Agent Configuration

**Required `allowed_tools` for MDEMG benchmarks:**

| Tool | Purpose |
|------|---------|
| `Bash` | For curl API calls to MDEMG, Neo4j queries |
| `Read` | Reading question files, results, and source files |
| `Write` | Writing JSONL output |
| `Grep`, `Glob` | File searching for verification |

**Example Task invocation:**

```
Task(
  description="MDEMG benchmark whk-wms 120q",
  prompt="Run benchmark per BENCHMARKING_GUIDE.md...",
  subagent_type="general-purpose",
  model="haiku",
  allowed_tools=["Bash", "Read", "Grep", "Glob", "Write"]
)
```

**NOTE:** Do NOT include `Skill(mdemg)` or `Skill(mdemg-consult)` in `allowed_tools` - they will NOT work in sub-agents. The MDEMG prompt instructs agents to use curl API calls directly.

### Step 4.6: Running Tests in Parallel

```bash
# Terminal 1: Run MDEMG test
claude --prompt-file mdemg_prompt.md --output mdemg_output.txt

# Terminal 2: Run Baseline test
claude --prompt-file baseline_prompt.md --output baseline_output.txt
```

---

## Phase 5: Analysis and Reporting

### Step 5.1: Comparison Report Template

```markdown
# MDEMG vs Baseline Comparison Report

**Date:** YYYY-MM-DD
**Codebase:** [project name]
**Questions:** 100

## Executive Summary

| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| **Total Score** | X/100 | Y/100 | +Z |
| **Average Score** | 0.XXX | 0.YYY | +0.ZZZ |
| Completely Correct (1.0) | A | B | +C |
| Partially Correct (0.5) | D | E | +F |
| Unable to Answer (0.0) | G | H | -I |

**Result: MDEMG outperformed Baseline by Z points (Xx better).**

## Score by Category

| Category | Baseline | MDEMG | Delta |
|----------|----------|-------|-------|
| architecture_structure | X/20 | Y/20 | +Z |
| service_relationships | X/20 | Y/20 | +Z |
| business_logic_constraints | X/20 | Y/20 | +Z |
| data_flow_integration | X/20 | Y/20 | +Z |
| cross_cutting_concerns | X/20 | Y/20 | +Z |

## Key Findings

1. **Context Window Limitations (Baseline)**
   - [Observations]

2. **MDEMG Retrieval Strengths**
   - [Observations]

3. **Areas for Improvement**
   - [Observations]

## Conclusion

[Summary of findings]
```

### Step 5.2: Generate Comparison Script

```python
#!/usr/bin/env python3
"""Generate comparison report from test outputs"""

def parse_results(filename):
    # Parse test output file
    # Return dict of {question_id: score}
    pass

def generate_comparison(baseline_file, mdemg_file):
    baseline = parse_results(baseline_file)
    mdemg = parse_results(mdemg_file)

    # Calculate metrics
    baseline_avg = sum(baseline.values()) / len(baseline)
    mdemg_avg = sum(mdemg.values()) / len(mdemg)

    print(f"Baseline Avg: {baseline_avg:.3f}")
    print(f"MDEMG Avg: {mdemg_avg:.3f}")
    print(f"Delta: {mdemg_avg - baseline_avg:+.3f}")
```

---

## Roles and Responsibilities

### Main AI (Orchestrator/Administrator)

The Main AI coordinates the testing process:

| Responsibility | Actions |
|---------------|---------|
| **Setup** | Prepare file lists, verify MDEMG running, create test prompts |
| **Coordination** | Launch baseline and MDEMG agents in parallel |
| **Monitoring** | Track progress, handle errors, verify completion |
| **Analysis** | Compare results, generate reports, identify patterns |
| **Quality Control** | Verify answer accuracy against expected answers |

### Baseline Agent

| Constraint | Rationale |
|------------|-----------|
| Must read ALL files first | Simulates "full context" approach |
| Cannot re-read files during questions | Tests context retention |
| Self-scores answers | Enables comparison with expected |
| Reports source of answers | Distinguishes memory vs lookup |

### MDEMG Agent

| Constraint | Rationale |
|------------|-----------|
| Must ingest all files to MDEMG | Builds the memory graph |
| Must query MDEMG first | Tests retrieval quality |
| May verify via file lookup | Allows hybrid approach |
| Self-scores answers | Enables comparison with expected |
| Reports MDEMG retrieval stats | Measures retrieval effectiveness |

---

## Common Mistakes to Avoid

### Question Development Mistakes

| Mistake | Why It's Bad | How to Avoid |
|---------|--------------|--------------|
| **Single-file questions** | Doesn't test cross-module understanding | Require 2+ files in `required_files` |
| **Speculative answers** | "likely", "probably" indicates unverified | Verify all answers against code |
| **Wrong method/field names** | ~35% of generated answers have these | Code-verify before finalizing |
| **Overstated functionality** | Claiming features that don't exist | Check actual implementation |
| **Enum value errors** | Wrong status codes, resolution types | Verify against actual enums |
| **Generic patterns assumed** | Assuming Redis when in-memory is used | Check actual implementation |

### Test Execution Mistakes

| Mistake | Why It's Bad | How to Avoid |
|---------|--------------|--------------|
| **Not verifying ingestion** | Test on incomplete data | Check node count matches file count |
| **Warm start without disclosure** | Learning edges affect results | Document edge count before/after |
| **Inconsistent scoring** | Incomparable results | Use strict scoring rubric |
| **Skipping Phase 1** | Baseline gets unfair advantage | Enforce all-files-first rule |

### Analysis Mistakes

| Mistake | Why It's Bad | How to Avoid |
|---------|--------------|--------------|
| **Comparing against wrong answers** | ~33% of questions may have errors | Verify question set first |
| **Ignoring category breakdown** | Misses strength/weakness patterns | Always report by category |
| **Single run conclusions** | Statistical noise | Run cold start tests multiple times |

---

## V6 Composite Test Set

### Overview

The V6 Composite Test Set is the standardized benchmark for VS Code codebase testing. It combines questions from multiple prior test versions into a single, balanced evaluation suite.

| Property | Value |
|----------|-------|
| **Total Questions** | 100 |
| **Categories** | 10 (10 questions each) |
| **Question Types** | simple_constant (61), multi_hop (18), hard (21) |
| **Codebase** | VS Code (`vscode-scale` space) |
| **Nodes in Graph** | ~29,000+ |

### Categories

| Category | Description | Example Question Type |
|----------|-------------|----------------------|
| `editor_config` | Editor settings, fonts, cursors | MAX_CURSOR_COUNT, tabSize defaults |
| `storage` | Storage backends, flush intervals | Flush timeouts, storage paths |
| `workbench_layout` | Panel sizes, grid dimensions | Sidebar widths, panel heights |
| `extensions` | Extension host, marketplace | Max extensions, timeout values |
| `terminal` | Terminal emulator settings | Scrollback limits, shell paths |
| `search` | Search features, limits | Max results, regex patterns |
| `files` | File handling, watchers | Max file size, watcher limits |
| `debug` | Debugger configuration | Timeout values, adapter settings |
| `notifications` | Notification system | Toast limits, duration values |
| `lifecycle` | App lifecycle, shutdown | Shutdown timeouts, startup flags |

### Question Types

1. **simple_constant** (61 questions)
   - Single-file evidence-locked questions
   - Require finding exact constant values
   - Example: "What is the DEFAULT_FLUSH_INTERVAL in storage.ts?"

2. **multi_hop** (18 questions)
   - Require tracing through 2-3 files
   - Test graph traversal capabilities
   - Example: "Trace how terminal scrollback limit is applied from config to renderer"

3. **hard** (21 questions)
   - Complex cross-file correlations
   - May require understanding of multiple interacting systems
   - Example: "What determines the maximum concurrent file watchers?"

### File Locations

```
docs/tests/vscode-scale/
├── test_questions_V6_composite.json    # Master set WITH expected answers
├── test_questions_V6_blind.json        # Blind set (questions only, for agents)
├── run_combined_benchmark.py           # Benchmark runner script
└── combined_benchmark_results.json     # Output from benchmark runs
```

### Master vs Blind Versions

| Version | File | Purpose | Contains Answers |
|---------|------|---------|------------------|
| **Master** | `test_questions_V6_composite.json` | Scoring, verification | YES |
| **Blind** | `test_questions_V6_blind.json` | Agent benchmark tasks | NO |

**Important:** Always use the **blind version** when testing agents to prevent answer leakage. The master version is for scoring and result verification only.

### Master Question Format

```json
{
  "id": "v6_editor_config_01",
  "category": "editor_config",
  "type": "simple_constant",
  "source": "v5_comprehensive",
  "question": "What is the MAX_CURSOR_COUNT constant in the VS Code editor?",
  "required_evidence": {
    "file": "cursor.ts",
    "value": "10000"
  }
}
```

### Blind Question Format

```json
{
  "id": "v6_editor_config_01",
  "category": "editor_config",
  "type": "simple_constant",
  "question": "What is the MAX_CURSOR_COUNT constant in the VS Code editor?"
}
```

### Running the Benchmark

```bash
# Activate test environment
source /tmp/mdemg-test-venv/bin/activate

# Run the benchmark (requires MDEMG running on localhost:9999)
python docs/tests/vscode-scale/run_combined_benchmark.py

# Results saved to combined_benchmark_results.json
```

### Scoring Criteria

| Score | Criteria |
|-------|----------|
| **1.0** | Correct file found AND correct value/evidence found |
| **0.5** | Correct file found but value not confirmed |
| **0.0** | Neither file nor evidence found |

### Baseline Results (Pre-Symbol Extraction)

| Metric | Value |
|--------|-------|
| Correct file in top-5 | 11.7% (7/60) |
| Evidence-locked accuracy | ~5% |
| Key Issue | Returns related files, not defining files |

This establishes the baseline that symbol extraction aims to improve.

---

## WHK-WMS 120-Question Test Set

### Overview

The WHK-WMS 120-question test set is the standardized benchmark for the warehouse management system codebase. It combines complex multi-file questions with symbol-lookup questions that test exact constant/value retrieval.

| Property | Value |
|----------|-------|
| **Total Questions** | 120 |
| **Categories** | 5 (architecture_structure, service_relationships, business_logic_constraints, data_flow_integration, cross_cutting_concerns) |
| **Question Types** | multi-file (59), cross-module (38), system-wide (3), symbol-lookup (20) |
| **Codebase** | WHK-WMS (`whk-wms` space) |

### Question Types

1. **multi-file** (59 questions)
   - Require understanding 2+ files to answer correctly
   - Test cross-file knowledge synthesis

2. **cross-module** (38 questions)
   - Require tracing through multiple modules
   - Test understanding of module interactions

3. **system-wide** (3 questions)
   - Complex architectural questions
   - Require broad system understanding

4. **symbol-lookup** (20 questions)
   - Test exact constant/value retrieval
   - Require finding specific symbol definitions (e.g., `MAX_EXPORT_SIZE`, `ID_CHUNK_SIZE`)
   - Symbol name included in agent version as search hint

### File Locations

```
docs/tests/whk-wms/
├── test_questions_120.json           # Master set WITH answers (for grading)
├── test_questions_120_agent.json     # Agent set WITHOUT answers (for task agents)
├── symbol-focused-questions-20.json  # Symbol questions source file
└── test_questions_100.json           # Original 100 questions (no symbols)
```

### Master vs Agent Versions

| Version | File | Purpose | Contains |
|---------|------|---------|----------|
| **Master** | `test_questions_120.json` | Scoring, grading | answers, required_files |
| **Agent** | `test_questions_120_agent.json` | Task agent input | questions only, NO answers |

**CRITICAL:** Always use the **agent version** (`_agent.json`) when feeding questions to task agents. The master version is for scoring and result verification only. Feeding answers to agents constitutes data contamination and invalidates test results.

### Master Question Format

```json
{
  "id": "sym_arch_3",
  "category": "architecture_structure",
  "question": "What is the ID_CHUNK_SIZE used by BarrelAggregatesService for chunked processing?",
  "answer": "5000",
  "symbol_name": "ID_CHUNK_SIZE",
  "required_files": ["/apps/whk-wms/src/barrelAggregates/barrel-aggregates.service.ts"],
  "complexity": "symbol-lookup"
}
```

### Agent Question Format (Answer Stripped)

```json
{
  "id": "sym_arch_3",
  "category": "architecture_structure",
  "question": "What is the ID_CHUNK_SIZE used by BarrelAggregatesService for chunked processing?",
  "complexity": "symbol-lookup",
  "symbol_name": "ID_CHUNK_SIZE"
}
```

Note: `symbol_name` is retained in the agent version because it appears in the question text and helps the agent know what to search for.

---

## Baseline Benchmarking Rules

### Time Limit

| Constraint | Value |
|------------|-------|
| **Total time to complete test** | 20 minutes |

### Model Selection

| Parameter | Value |
|-----------|-------|
| **Model** | Claude Haiku (haiku) |
| **Rationale** | Cost-effective for benchmark testing; Opus reserved for production |

When spawning task agents, always specify `model: "haiku"` to use Claude Haiku.

### Task Agent Instructions

When spawning task agents for baseline benchmarking, use this **EXACT prompt** (do not deviate):

```
BASELINE BENCHMARK RULES - READ CAREFULLY

1. USE TOOLS (Read, Glob, Grep) but DO NOT search the web
2. You may read the QUESTION FILE below, then ONLY interact with the whk-wms repo at {TARGET_REPO_PATH}
3. You are NOT ALLOWED to access any other repo or directory after reading the question file
4. These questions are complex and require EXPLANATION for full credit
5. Your primary goal is to be AS CORRECT AS POSSIBLE when answering questions
6. TIME LIMIT: 20 minutes total

WARNING: If you violate these rules you will be DISQUALIFIED and your task execution will end immediately.

QUESTION FILE (read this first, then work only in whk-wms): {MDEMG_PATH}/docs/tests/whk-wms/test_questions_120_agent.json

This file contains 120 questions across multiple categories and complexity levels. Answer them IN ORDER as they appear in the file.

SCORING CRITERIA:

For symbol-lookup questions (have "complexity": "symbol-lookup"):
- 1.0 = Exact value match from correct file with line number
- 0.5 = Correct file found but value not confirmed
- 0.0 = Wrong file or wrong value

For complex questions:
- 1.0 = Complete, correct answer with explanation and file references
- 0.75 = Correct answer with partial explanation
- 0.5 = Partially correct, missing key details
- 0.25 = Relevant information found but answer incomplete
- 0.0 = Unable to answer or incorrect

OUTPUT FORMAT for each question:
Q[id]: [question text truncated to 80 chars]
Files: [files searched/read]
Answer: [your answer with file:line references]
Confidence: [HIGH/MEDIUM/LOW]
Score: [self-assessed 0.0-1.0]

STRATEGY:
1. Read the question file first
2. Work through questions IN ORDER as they appear
3. For each question: search relevant files, synthesize answer, cite sources with file:line
4. Answer as many questions as possible within the time limit

FINAL REPORT FORMAT:
=== BASELINE BENCHMARK RESULTS ===
Time Elapsed: [minutes]
Questions Attempted: [X/120]
By Complexity:
  - symbol-lookup: [X attempted]
  - multi-file: [X attempted]
  - cross-module: [X attempted]
  - disambiguation: [X attempted]
  - computed_value: [X attempted]
  - relationship: [X attempted]
Disqualified: [Yes/No]

ANSWERS:
[List each answered question with: id, answer summary, file:line, confidence, score]
===

BEGIN NOW - Read the question file and start answering in order.
```

### Rule Violations

| Violation | Consequence |
|-----------|-------------|
| Searching the web | Immediate disqualification |
| Accessing repos outside whk-wms | Immediate disqualification |
| Exceeding 20-minute time limit | Test terminated, partial results only |

### Scoring for Complex Questions

| Score | Criteria |
|-------|----------|
| 1.0 | Complete, correct answer with explanation and file references |
| 0.75 | Correct answer with partial explanation |
| 0.5 | Partially correct, missing key details |
| 0.25 | Relevant information found but answer incomplete |
| 0.0 | Unable to answer or incorrect |

### Symbol-Lookup Question Scoring

| Score | Criteria |
|-------|----------|
| 1.0 | Exact value match from correct file |
| 0.5 | Correct file found but value not confirmed |
| 0.0 | Wrong file or wrong value |

---

## Required Benchmark Metrics

All benchmark reports MUST include the following metrics. This comprehensive list ensures reproducibility and blocks skepticism paths.

### Core Outcome Metrics (Must-Have)

These answer: "Did it work?"

| # | Metric | Description |
|---|--------|-------------|
| 1 | **Mean Score (μ)** | Primary scalar score across the battery |
| 2 | **Std Dev (σ) + CV** | CV = 100·σ/μ - Stability across runs |
| 3 | **Median + Percentiles** | p10 / p25 / p75 / p90 distribution |
| 4 | **Min Score** | Worst-case / edge-case survivability |
| 5 | **High-Confidence Rate** | % with score ≥ 0.7 (publish threshold explicitly) |
| 6 | **Completion Rate** | % answered without stalling, "UNKNOWN," or partials |

### Grounding & Evidence Metrics

These answer: "Was it actually grounded in repo truth?"

| # | Metric | Description |
|---|--------|-------------|
| 7 | **Evidence Compliance Rate (ECR)** | % answers with: file paths, symbol names, exact values, line anchors |
| 8 | **Evidence Correctness Rate (E-Accuracy)** | % cited values that are actually correct |
| 9 | **Hard-Value Resolution Rate (HVRR)** | % exact-value questions correctly resolved with evidence |
| 10 | **Refusal/Not-Found Correctness** | % correct "not found" on trap questions |
| 11 | **Hallucination Rate** | Count of claims not supported by cited sources |
| 12 | **Guess Rate vs Grounded Rate** | % using hedging ("likely," "probably") vs sourced answers |

### Prior-Resistance Metrics

These answer: "Did priors inflate results?"

| # | Metric | Description |
|---|--------|-------------|
| 13 | **Tier Split Performance** | Report separately for Tier 1 (prior-friendly) vs Tier 2 (evidence-locked) |
| 14 | **Evidence Lift (Tier 2)** | Tier 2 ECR/HVRR comparison baseline vs MDEMG |
| 15 | **Exactness Error Rate** | % returning plausible but wrong values on exact-value questions |

### Multi-hop / Multi-file Rigor Metrics

These answer: "Is this hard-mode or toy-mode?"

| # | Metric | Description |
|---|--------|-------------|
| 16 | **Multi-file Coverage** | % requiring ≥2 files, avg required files per question |
| 17 | **Multi-hop Depth** | Performance by hop count (2,3,4…) |
| 18 | **Cross-module Questions** | % spanning multiple subsystems, performance on subset |
| 19 | **Effective Default Accuracy** | Declared vs runtime-effective defaults |

### Scale & Performance Metrics

These answer: "Is it practical?"

| # | Metric | Description |
|---|--------|-------------|
| 20 | **Total Elapsed Test Time** | Wall-clock time from test start to completion (minutes) |
| 21 | **Auto-Compact Events Count** | Number of context window compaction events during test |
| 22 | **Total Tokens Consumed** | Sum of input + output tokens across entire test run |
| 23 | **Ingestion Throughput** | LOC, elements/sec, observations/sec, wall time |
| 24 | **Embedding Coverage + Health** | Track and publish for credibility |
| 25 | **DB Footprint** | Neo4j size, vector index size, peak memory |
| 26 | **Retrieval Latency** | p50/p95/p99 for: vector fetch, graph traversal, rerank, e2e |
| 27 | **Token Efficiency** | Tokens/question (avg/p95), total tokens/run |
| 28 | **Tool Call Budget** | Tool calls/question (avg/p95), total tool calls |
| 29 | **Cost Proxy** | Estimated $ cost (tokens + tool calls) or GPU-hours |

### Reproducibility & Integrity Metrics

These answer: "Can anyone rerun it and get the same thing?"

| # | Metric | Description |
|---|--------|-------------|
| 30 | **Repo commit hash + scope** | URL, commit, filetype allowlist/denylist, directory scope |
| 31 | **Question set identity** | Dataset SHA-256, question IDs, seeds if sampled |
| 32 | **Cold vs warm disclosure** | Learning edges reset? Caches cleared? |
| 33 | **Determinism controls** | Temperature, decoding settings, max tool calls/time |
| 34 | **Run Count (k)** | 3 runs per condition (canonical) |

### Multi-Repo Integrity (if applicable)

These answer: "Does it confuse corpora?"

| # | Metric | Description |
|---|--------|-------------|
| 35 | **Cross-Repo Contamination Rate (CRCR)** | % citing files from wrong repo |
| 36 | **Repo Attribution Accuracy (RAA)** | % citing exclusively from correct repo |
| 37 | **Collision Set Performance** | Performance on adversarial overlapping vocabulary questions |

### Minimum Publication Set (12 metrics)

For compact but defensible reports, publish at minimum:

| Category | Metrics |
|----------|---------|
| Distribution | Mean, Std, CV, Min, Median, p10/p90 |
| Quality | Completion rate, ECR, E-Accuracy, HVRR |
| Performance | Retrieval latency p50/p95, Tokens/question, Tool calls/question |
| Reproducibility | Dataset hash, seeds/IDs, repo commit, scope |

### Report Table Format

Use this format for condition comparison:

| Condition | Mean | CV% | Min | p10 | p90 | Comp% | ECR% | E-Acc% | HVRR% | p95 Lat(ms) | Tok/Q |
|-----------|------|-----|-----|-----|-----|-------|------|--------|-------|-------------|-------|
| Baseline  |      |     |     |     |     |       |      |        |       |             |       |
| MDEMG     |      |     |     |     |     |       |      |        |       |             |       |

**CRITICAL:** Always publish raw per-question outputs (JSONL) for audit.

---

## Appendix: Templates and Scripts

### A. Question Template

```json
{
  "id": 1,
  "category": "service_relationships",
  "question": "What services does X depend on for Y?",
  "answer": "X constructor injects: 1) Service A - purpose; 2) Service B - purpose; ...",
  "required_files": [
    "/path/to/service-a.ts",
    "/path/to/service-b.ts"
  ],
  "complexity": "multi-file"
}
```

### B. Scoring Rubric

| Score | Criteria |
|-------|----------|
| 1.0 | All key points correct, correct file references, correct terminology |
| 0.5 | Main concept correct but missing details, minor errors in specifics |
| 0.0 | Unable to answer or answer is vague/generic |
| -1.0 | Confidently states incorrect information |

### C. File List Generation

```bash
# TypeScript/JavaScript projects
find . -type f \( -name "*.ts" -o -name "*.tsx" -o -name "*.js" \) \
  | grep -v node_modules | grep -v dist | sort

# Python projects
find . -type f -name "*.py" \
  | grep -v __pycache__ | grep -v .venv | sort

# Go projects
find . -type f -name "*.go" \
  | grep -v vendor | sort
```

### D. Quick Verification Commands

```bash
# Check MDEMG health
curl -s localhost:9999/healthz

# Get space stats
curl -s 'localhost:9999/v1/memory/stats?space_id=your-project' | jq

# Test retrieval
curl -s localhost:9999/v1/memory/retrieve \
  -H 'content-type: application/json' \
  -d '{"space_id":"your-project","query_text":"How does X work?","top_k":5}' | jq '.results[].path'

# Check learning edges
docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword \
  "MATCH ()-[r:CO_ACTIVATED_WITH]->() RETURN count(r)"
```

### E. Sample Comparison Output

```
=== COMPARISON SUMMARY ===
Baseline Avg: 0.050
MDEMG Avg:    0.710
Delta:        +0.660 (14.2x improvement)

Score Distribution:
  >0.7:     Baseline 0%,  MDEMG 64%
  0.6-0.7:  Baseline 0%,  MDEMG 22%
  0.5-0.6:  Baseline 5%,  MDEMG 8%
  0.4-0.5:  Baseline 5%,  MDEMG 5%
  <0.4:     Baseline 90%, MDEMG 1%

Learning Edges Created: 8,622 (86 pairs/query)
===
```

### F. Automated Grading Script (CANONICAL)

Save as `grade_answers.py`:

```python
#!/usr/bin/env python3
"""
MDEMG Benchmark Grading Script
Grades agent answers against expected answers using value extraction + keyword overlap.

Usage:
    python grade_answers.py answers.jsonl questions_master.json grades.json
"""

import json
import re
import sys
from pathlib import Path
from typing import Dict, List, Any


def extract_keywords(text: str) -> set:
    """Extract meaningful keywords from text (lowercase, alphanumeric)."""
    words = re.findall(r'\b[a-zA-Z_][a-zA-Z0-9_]*\b', text.lower())
    # Filter out common words
    stopwords = {'the', 'a', 'an', 'is', 'are', 'was', 'were', 'be', 'been',
                 'being', 'have', 'has', 'had', 'do', 'does', 'did', 'will',
                 'would', 'could', 'should', 'may', 'might', 'must', 'shall',
                 'can', 'to', 'of', 'in', 'for', 'on', 'with', 'at', 'by',
                 'from', 'as', 'into', 'through', 'during', 'before', 'after',
                 'above', 'below', 'between', 'under', 'again', 'further',
                 'then', 'once', 'here', 'there', 'when', 'where', 'why',
                 'how', 'all', 'each', 'few', 'more', 'most', 'other', 'some',
                 'such', 'no', 'nor', 'not', 'only', 'own', 'same', 'so',
                 'than', 'too', 'very', 'just', 'and', 'but', 'if', 'or',
                 'because', 'until', 'while', 'this', 'that', 'these', 'those'}
    return {w for w in words if w not in stopwords and len(w) > 2}


def normalize_value(value: str) -> str:
    """Normalize a value for comparison (strip quotes, whitespace, common prefixes)."""
    if not value:
        return ""
    v = str(value).strip().lower()
    # Remove quotes
    v = v.strip('"\'`')
    # Remove common prefixes like "the value is", "it's", etc.
    prefixes = ['the value is ', 'value is ', 'it is ', "it's ", 'the answer is ', 'answer: ']
    for prefix in prefixes:
        if v.startswith(prefix):
            v = v[len(prefix):]
    return v.strip()


def extract_value(text: str, expected: str) -> bool:
    """Check if expected value appears in text using multiple matching strategies."""
    if not expected:
        return True  # No expected value = automatic pass

    # Normalize for comparison
    text_lower = text.lower()
    expected_norm = normalize_value(expected)

    # Strategy 1: Direct substring match
    if expected_norm in text_lower:
        return True

    # Strategy 2: Word boundary match (for short values)
    if len(expected_norm) >= 2:
        pattern = r'\b' + re.escape(expected_norm) + r'\b'
        if re.search(pattern, text_lower):
            return True

    # Strategy 3: Numeric comparison with tolerance
    try:
        exp_num = float(expected)
        # Look for numbers in text (including those with units like "10ms", "5KB")
        numbers = re.findall(r'(\d+(?:\.\d+)?)\s*(?:ms|s|kb|mb|gb|bytes?|%)?', text_lower)
        for num_str in numbers:
            try:
                if float(num_str) == exp_num:
                    return True
            except ValueError:
                continue
    except (ValueError, TypeError):
        pass

    # Strategy 4: Handle file paths (match basename)
    if '/' in str(expected) or '\\' in str(expected):
        expected_basename = Path(str(expected)).name.lower()
        if expected_basename in text_lower:
            return True

    # Strategy 5: Handle code identifiers (camelCase, snake_case variations)
    if '_' in expected_norm or any(c.isupper() for c in str(expected)):
        # Try snake_case to camelCase and vice versa
        snake_version = re.sub(r'([A-Z])', r'_\1', str(expected)).lower().strip('_')
        camel_version = ''.join(word.capitalize() for word in expected_norm.split('_'))
        if snake_version in text_lower or camel_version.lower() in text_lower:
            return True

    # Strategy 6: Token-level match (all tokens of expected appear in text)
    expected_tokens = set(re.findall(r'\b\w+\b', expected_norm))
    if len(expected_tokens) >= 2:
        text_tokens = set(re.findall(r'\b\w+\b', text_lower))
        if expected_tokens.issubset(text_tokens):
            return True

    return False


def grade_answer(answer: Dict, expected: Dict) -> Dict:
    """Grade a single answer against expected."""
    answer_text = answer.get('answer', '')
    expected_value = expected.get('answer', '')
    expected_files = expected.get('required_files', [])

    # Value score (0.0 or 1.0)
    value_found = extract_value(answer_text, expected_value)
    value_score = 1.0 if value_found else 0.0

    # Keyword score (0.0 to 1.0)
    expected_keywords = extract_keywords(expected_value)
    answer_keywords = extract_keywords(answer_text)

    if expected_keywords:
        keywords_found = expected_keywords & answer_keywords
        keyword_score = len(keywords_found) / len(expected_keywords)
    else:
        keyword_score = 1.0  # No keywords to match

    # Final score: 70% value, 30% keywords
    final_score = 0.70 * value_score + 0.30 * keyword_score

    # Check if correct file was cited
    cited_files = answer.get('file_line_refs', [])
    correct_file_cited = False
    for expected_file in expected_files:
        expected_basename = Path(expected_file).name
        for cited in cited_files:
            if expected_basename in cited:
                correct_file_cited = True
                break

    # Evidence found check
    evidence_found = bool(cited_files) and ':' in str(cited_files)

    return {
        'id': answer.get('id'),
        'value_score': value_score,
        'keyword_score': round(keyword_score, 3),
        'score': round(final_score, 3),
        'expected_value': expected_value[:100] if expected_value else None,
        'expected_keywords': list(expected_keywords)[:10],
        'keywords_found': list(expected_keywords & answer_keywords)[:10],
        'evidence_found': evidence_found,
        'correct_file_cited': correct_file_cited,
        'answer_preview': answer_text[:200] if answer_text else ''
    }


def grade_all(answers_file: Path, questions_file: Path, output_file: Path):
    """Grade all answers and compute aggregate metrics."""

    # Load questions with expected answers
    with open(questions_file) as f:
        questions_data = json.load(f)
    questions = {q['id']: q for q in questions_data.get('questions', questions_data)}

    # Load answers (JSONL format)
    answers = []
    with open(answers_file) as f:
        for line in f:
            line = line.strip()
            if line:
                answers.append(json.loads(line))

    # Grade each answer
    grades = []
    for answer in answers:
        qid = answer.get('id')
        if qid in questions:
            grade = grade_answer(answer, questions[qid])
            grades.append(grade)
        else:
            print(f"WARNING: Question {qid} not found in master set")

    # Compute aggregate metrics
    if not grades:
        print("ERROR: No grades computed")
        return

    scores = [g['score'] for g in grades]
    value_scores = [g['value_score'] for g in grades]

    import statistics

    aggregate = {
        'total_questions': len(grades),
        'mean': round(statistics.mean(scores), 3),
        'std': round(statistics.stdev(scores), 3) if len(scores) > 1 else 0,
        'cv_pct': round(100 * statistics.stdev(scores) / statistics.mean(scores), 1) if len(scores) > 1 and statistics.mean(scores) > 0 else 0,
        'median': round(statistics.median(scores), 3),
        'min': round(min(scores), 3),
        'max': round(max(scores), 3),
        'p10': round(sorted(scores)[int(len(scores) * 0.1)], 3),
        'p90': round(sorted(scores)[int(len(scores) * 0.9)], 3),
        'high_score_rate': round(sum(1 for s in scores if s >= 0.7) / len(scores), 3),
        'evidence_rate': round(sum(1 for g in grades if g['evidence_found']) / len(grades), 3),
        'correct_file_rate': round(sum(1 for g in grades if g['correct_file_cited']) / len(grades), 3),
        'value_hit_rate': round(statistics.mean(value_scores), 3)
    }

    # Write output
    output = {
        'aggregate': aggregate,
        'per_question': grades
    }

    with open(output_file, 'w') as f:
        json.dump(output, f, indent=2)

    print(f"Grading complete: {output_file}")
    print(f"  Mean: {aggregate['mean']}")
    print(f"  Std:  {aggregate['std']}")
    print(f"  CV:   {aggregate['cv_pct']}%")
    print(f"  High score rate (>=0.7): {aggregate['high_score_rate']*100:.1f}%")
    print(f"  Evidence rate: {aggregate['evidence_rate']*100:.1f}%")

    return aggregate


if __name__ == '__main__':
    if len(sys.argv) != 4:
        print("Usage: python grade_answers.py answers.jsonl questions_master.json grades.json")
        sys.exit(1)

    grade_all(Path(sys.argv[1]), Path(sys.argv[2]), Path(sys.argv[3]))
```

### G. Benchmark Test Harness (CANONICAL)

Save as `run_benchmark_harness.py`:

```python
#!/usr/bin/env python3
"""
MDEMG Benchmark Test Harness
Orchestrates 3 runs per condition and generates aggregate report.

Usage:
    python run_benchmark_harness.py --config benchmark_config.json

Config file example:
{
    "space_id": "whk-wms",
    "repo_path": "{TARGET_REPO_PATH}",
    "questions_file": "test_questions_120_agent.json",
    "questions_master": "test_questions_120.json",
    "output_dir": "benchmark_results",
    "run_count": 3,
    "conditions": ["baseline", "mdemg"]
}
"""

import argparse
import json
import subprocess
import sys
import hashlib
import statistics
from datetime import datetime
from pathlib import Path


def get_file_sha256(filepath: Path) -> str:
    """Compute SHA-256 of a file."""
    sha256 = hashlib.sha256()
    with open(filepath, 'rb') as f:
        for chunk in iter(lambda: f.read(4096), b''):
            sha256.update(chunk)
    return sha256.hexdigest()


def get_git_commit(repo_path: Path) -> str:
    """Get current git commit hash."""
    try:
        result = subprocess.run(
            ['git', 'rev-parse', 'HEAD'],
            cwd=repo_path,
            capture_output=True,
            text=True
        )
        return result.stdout.strip()
    except:
        return "unknown"


def clear_learning_edges(space_id: str) -> int:
    """Clear CO_ACTIVATED_WITH edges for cold start. Returns count deleted."""
    cypher = f"""
    MATCH ()-[r:CO_ACTIVATED_WITH {{space_id: '{space_id}'}}]-()
    DELETE r
    RETURN count(r) as deleted
    """
    try:
        result = subprocess.run(
            ['docker', 'exec', 'mdemg-neo4j', 'cypher-shell',
             '-u', 'neo4j', '-p', 'testpassword', cypher],
            capture_output=True,
            text=True
        )
        # Parse count from output
        for line in result.stdout.split('\n'):
            if line.strip().isdigit():
                return int(line.strip())
    except:
        pass
    return 0


def run_agent_benchmark(condition: str, run_num: int, config: dict, output_dir: Path) -> Path:
    """
    Run a single benchmark agent and return path to answers file.

    NOTE: This function is a placeholder. In practice, you would:
    1. Spawn a Claude agent with the appropriate prompt
    2. Wait for completion
    3. Return the path to the generated answers.jsonl file

    For actual execution, use the Task tool with the canonical prompts.
    """
    output_file = output_dir / f"answers_{condition}_run{run_num}.jsonl"

    print(f"[{condition}] Run {run_num}: Starting agent...")
    print(f"  Output: {output_file}")
    print(f"  NOTE: Spawn agent manually using Task tool with canonical prompt")

    # Placeholder - in real execution, this would be the agent's output
    return output_file


def extract_auto_compact_count(answers_file: Path) -> int:
    """Extract auto-compact event count from JSONL summary line."""
    auto_compact_count = 0
    try:
        with open(answers_file) as f:
            for line in f:
                line = line.strip()
                if line:
                    try:
                        obj = json.loads(line)
                        if obj.get('type') == 'summary':
                            auto_compact_count = obj.get('auto_compact_events', 0)
                            break
                    except json.JSONDecodeError:
                        continue
    except FileNotFoundError:
        pass
    return auto_compact_count


def grade_run(answers_file: Path, questions_master: Path, output_dir: Path, condition: str, run_num: int) -> dict:
    """Grade a single run's answers."""
    grades_file = output_dir / f"grades_{condition}_run{run_num}.json"

    # Extract auto-compact count from summary line
    auto_compact_count = extract_auto_compact_count(answers_file)

    # Run grading script
    result = subprocess.run(
        [sys.executable, 'grade_answers.py',
         str(answers_file), str(questions_master), str(grades_file)],
        capture_output=True,
        text=True
    )

    if result.returncode != 0:
        print(f"Grading failed: {result.stderr}")
        return {}

    with open(grades_file) as f:
        grades = json.load(f)

    # Add auto-compact count to grades
    grades['auto_compact_events'] = auto_compact_count
    return grades


def aggregate_runs(all_grades: list[dict], condition: str) -> dict:
    """Aggregate metrics across multiple runs."""
    if not all_grades:
        return {}

    # Collect per-run means
    run_means = [g['aggregate']['mean'] for g in all_grades]
    run_stds = [g['aggregate']['std'] for g in all_grades]

    # Collect auto-compact counts per run
    auto_compact_per_run = [g.get('auto_compact_events', 0) for g in all_grades]
    auto_compact_total = sum(auto_compact_per_run)

    # Combine all scores across runs
    all_scores = []
    for g in all_grades:
        all_scores.extend([q['score'] for q in g['per_question']])

    # Compute cross-run statistics
    cross_run_mean = statistics.mean(run_means)
    cross_run_std = statistics.stdev(run_means) if len(run_means) > 1 else 0
    cross_run_cv = 100 * cross_run_std / cross_run_mean if cross_run_mean > 0 else 0

    return {
        'condition': condition,
        'run_count': len(all_grades),
        'run_means': [round(m, 3) for m in run_means],
        'run_stds': [round(s, 3) for s in run_stds],
        'cross_run_mean': round(cross_run_mean, 3),
        'cross_run_std': round(cross_run_std, 3),
        'cross_run_cv_pct': round(cross_run_cv, 1),
        'cross_run_range': [round(min(run_means), 3), round(max(run_means), 3)],
        'pooled_mean': round(statistics.mean(all_scores), 3),
        'pooled_std': round(statistics.stdev(all_scores), 3) if len(all_scores) > 1 else 0,
        'pooled_min': round(min(all_scores), 3),
        'pooled_max': round(max(all_scores), 3),
        'high_score_rate': round(sum(1 for s in all_scores if s >= 0.7) / len(all_scores), 3),
        'auto_compact_events': {
            'per_run': auto_compact_per_run,
            'total': auto_compact_total
        }
    }


def generate_report(baseline_agg: dict, mdemg_agg: dict, config: dict, output_dir: Path):
    """Generate final aggregate report."""

    report = {
        'metadata': {
            'generated_at': datetime.now().isoformat(),
            'space_id': config['space_id'],
            'repo_path': config['repo_path'],
            'repo_commit': get_git_commit(Path(config['repo_path'])),
            'questions_file': config['questions_file'],
            'questions_sha256': get_file_sha256(Path(config['questions_file'])),
            'run_count': config['run_count'],
            'cold_start': True
        },
        'baseline': baseline_agg,
        'mdemg': mdemg_agg,
        'comparison': {
            'mean_delta': round(mdemg_agg['cross_run_mean'] - baseline_agg['cross_run_mean'], 3),
            'mean_delta_pct': round(100 * (mdemg_agg['cross_run_mean'] - baseline_agg['cross_run_mean']) / baseline_agg['cross_run_mean'], 1) if baseline_agg['cross_run_mean'] > 0 else 0,
            'high_score_delta': round(mdemg_agg['high_score_rate'] - baseline_agg['high_score_rate'], 3),
            'verdict': 'PASS' if mdemg_agg['cross_run_mean'] > baseline_agg['cross_run_mean'] else 'FAIL'
        }
    }

    report_file = output_dir / 'aggregate_report.json'
    with open(report_file, 'w') as f:
        json.dump(report, f, indent=2)

    print("\n" + "=" * 60)
    print("BENCHMARK COMPLETE")
    print("=" * 60)
    print(f"\nBaseline: {baseline_agg['cross_run_mean']:.3f} ± {baseline_agg['cross_run_std']:.3f}")
    print(f"MDEMG:    {mdemg_agg['cross_run_mean']:.3f} ± {mdemg_agg['cross_run_std']:.3f}")
    print(f"Delta:    {report['comparison']['mean_delta']:+.3f} ({report['comparison']['mean_delta_pct']:+.1f}%)")
    print(f"Verdict:  {report['comparison']['verdict']}")

    # Print auto-compact events summary
    baseline_compacts = baseline_agg.get('auto_compact_events', {})
    mdemg_compacts = mdemg_agg.get('auto_compact_events', {})
    print(f"\nAuto-Compact Events:")
    print(f"  Baseline: {baseline_compacts.get('per_run', [])} (total: {baseline_compacts.get('total', 0)})")
    print(f"  MDEMG:    {mdemg_compacts.get('per_run', [])} (total: {mdemg_compacts.get('total', 0)})")

    print(f"\nReport: {report_file}")

    return report


def main():
    parser = argparse.ArgumentParser(description='MDEMG Benchmark Test Harness')
    parser.add_argument('--config', required=True, help='Path to benchmark config JSON')
    args = parser.parse_args()

    with open(args.config) as f:
        config = json.load(f)

    output_dir = Path(config['output_dir'])
    output_dir.mkdir(parents=True, exist_ok=True)

    questions_master = Path(config['questions_master'])
    run_count = config.get('run_count', 3)

    print("=" * 60)
    print("MDEMG BENCHMARK TEST HARNESS")
    print("=" * 60)
    print(f"Space: {config['space_id']}")
    print(f"Repo: {config['repo_path']}")
    print(f"Runs per condition: {run_count}")
    print(f"Output: {output_dir}")

    # Cold start - clear learning edges before first run
    print("\nClearing learning edges (cold start)...")
    deleted = clear_learning_edges(config['space_id'])
    print(f"  Deleted {deleted} edges")

    # Run baseline benchmarks
    print("\n--- BASELINE CONDITION ---")
    baseline_grades = []
    for run_num in range(1, run_count + 1):
        answers_file = run_agent_benchmark('baseline', run_num, config, output_dir)
        if answers_file.exists():
            grades = grade_run(answers_file, questions_master, output_dir, 'baseline', run_num)
            if grades:
                baseline_grades.append(grades)

    # Run MDEMG benchmarks
    print("\n--- MDEMG CONDITION ---")
    mdemg_grades = []
    for run_num in range(1, run_count + 1):
        answers_file = run_agent_benchmark('mdemg', run_num, config, output_dir)
        if answers_file.exists():
            grades = grade_run(answers_file, questions_master, output_dir, 'mdemg', run_num)
            if grades:
                mdemg_grades.append(grades)

    # Aggregate and report
    baseline_agg = aggregate_runs(baseline_grades, 'baseline')
    mdemg_agg = aggregate_runs(mdemg_grades, 'mdemg')

    if baseline_agg and mdemg_agg:
        generate_report(baseline_agg, mdemg_agg, config, output_dir)
    else:
        print("\nWARNING: Incomplete data - cannot generate final report")
        print("  Ensure all agent benchmark runs completed and produced output files")


if __name__ == '__main__':
    main()
```

### H. Benchmark Config Template

Save as `benchmark_config.json`:

```json
{
    "space_id": "whk-wms",
    "repo_path": "{TARGET_REPO_PATH}",
    "questions_file": "docs/tests/whk-wms/test_questions_120_agent.json",
    "questions_master": "docs/tests/whk-wms/test_questions_120.json",
    "output_dir": "docs/tests/whk-wms/benchmark_results",
    "run_count": 3,
    "conditions": ["baseline", "mdemg"],
    "mdemg_endpoint": "http://localhost:9999",
    "cold_start": true
}
```

---

## End-to-End Multi-Corpus Runbook

This runbook defines the complete end-to-end process for multi-corpus benchmarking (e.g., WHK-WMS + plc-gbt). It ensures reproducibility, captures all required artifacts, and blocks skepticism paths.

### Phase 0 — Preflight Receipts (Must Capture)

Capture these BEFORE you touch the DB:

#### Environment + Config

```bash
# Capture preflight receipts
mdemg_commit=$(cd {MDEMG_PATH} && git rev-parse HEAD)
neo4j_version=$(curl -s http://localhost:7474/db/data/ | jq -r '.neo4j_version // "unknown"')
hardware_info=$(system_profiler SPHardwareDataType 2>/dev/null | grep -E "Chip|Memory|Cores" || echo "N/A")
disk_free=$(df -h . | tail -1 | awk '{print $4}')

echo "MDEMG Commit: $mdemg_commit"
echo "Neo4j Version: $neo4j_version"
echo "Hardware: $hardware_info"
echo "Disk Free: $disk_free"
```

| Receipt | How to Capture |
|---------|----------------|
| MDEMG commit hash | `git rev-parse HEAD` |
| Config snapshot | Copy `.env` or export to JSON |
| Neo4j version, DB name, path | `curl localhost:7474/db/data/` |
| Hardware | CPU, RAM, GPU (if any), disk free |

#### Corpora Identity

| Receipt | How to Capture |
|---------|----------------|
| whk-wms repo commit | `cd /path/to/whk-wms && git rev-parse HEAD` |
| whk-wms ingest scope | Filetype allowlist/denylist, excluded dirs |
| plc-gbt repo commit | `cd /path/to/plc-gbt && git rev-parse HEAD` |
| plc-gbt ingest scope | Filetype allowlist/denylist, excluded dirs |

#### Question Sets Identity

| Receipt | How to Capture |
|---------|----------------|
| Question bank file name(s) | Full paths |
| SHA-256 of question set(s) | `shasum -a 256 <file>` |
| Sampling seed or question IDs | If sampling, document seed or explicit ID list |

### Phase 1 — Benchmark Test Summary (Required Artifacts)

Every benchmark run MUST produce these artifacts:

#### Required Artifact Files

| File | Description |
|------|-------------|
| `run_manifest.json` | Immutable receipts (commits, config, timestamps) |
| `per_question_results.jsonl` | One JSON record per question |
| `aggregate_metrics.json` | Computed from JSONL |
| `neo4j_snapshot.json` | Counts + high-level DB state |
| `stdout.log` | Raw run output |

#### run_manifest.json Template

```json
{
  "run_id": "benchmark-v12-20260124-043000",
  "timestamp": "2026-01-24T04:30:00Z",
  "operator": "reh3376",
  "mdemg_commit": "abc123...",
  "neo4j_version": "5.x",
  "hardware": {
    "cpu": "Apple M2 Max",
    "ram": "64GB",
    "gpu": "N/A"
  },
  "corpora": {
    "whk-wms": {
      "commit": "def456...",
      "space_id": "whk-wms",
      "scope_paths": ["/apps/whk-wms", "/apps/whk-wms-front-end"],
      "filetypes": ["*.ts", "*.tsx", "*.json"],
      "excluded": [".git", "node_modules", "dist"]
    },
    "plc-gbt": {
      "commit": "ghi789...",
      "space_id": "plc-gbt",
      "scope_paths": ["/"],
      "filetypes": ["*.ts", "*.py", "*.json"],
      "excluded": [".git", "node_modules", "plc_backups", "n8n-framework"]
    }
  },
  "question_set": {
    "file": "test_questions_v4_selected.json",
    "sha256": "abc123...",
    "n_questions": 100,
    "sampling_seed": null
  },
  "config": {
    "RERANK_ENABLED": true,
    "RERANK_WEIGHT": 0.15,
    "RERANK_MODEL": "gpt-4o-mini"
  },
  "cold_start": false,
  "initial_learning_edges": 5926
}
```

#### per_question_results.jsonl Format

```jsonl
{"id":"q001","question":"What is MAX_TAKE?","score":0.85,"top_file":"pagination.constants.ts","latency_ms":234,"tokens":1200,"tool_calls":3,"evidence_found":true,"space_id":"whk-wms"}
{"id":"q002","question":"What services does X inject?","score":0.72,"top_file":"barrel.service.ts","latency_ms":456,"tokens":1800,"tool_calls":5,"evidence_found":true,"space_id":"whk-wms"}
```

#### Required Metrics to Compute

**Core Metrics:**

| Metric | Description |
|--------|-------------|
| mean score | Average across all questions |
| median | Middle value |
| std, CV | Standard deviation, Coefficient of Variation |
| p10/p90 | 10th/90th percentiles |
| min score | Worst case |
| completion rate | % answered without stalling |

**Grounding Metrics:**

| Metric | Description |
|--------|-------------|
| Evidence Compliance Rate (ECR) | % with file paths, line numbers, values |
| Evidence Accuracy (E-Acc) | % cited values actually correct |
| Hard-Value Resolution Rate (HVRR) | % exact-value questions correct |
| Hallucination rate | Claims not supported by citations |
| Refusal correctness | % correct "not found" on trap questions |

**Efficiency Metrics:**

| Metric | Description |
|--------|-------------|
| tokens/question avg, p95 | Token consumption |
| tool calls/question avg, p95 | Tool call budget |
| latency p50/p95 | Retrieval + end-to-end |
| total wall time | Complete run duration |

**Reproducibility:**

| Metric | Description |
|--------|-------------|
| k runs | Number of runs (3 per condition) |
| seeds/ID lists | For sampling |
| dataset hash | SHA-256 of question file |
| repo commit hashes | All corpora |

### Phase 2 — Ingest Second Corpus (Multi-Space)

**CRITICAL RULES:**

- Do NOT delete or clear existing space nodes/edges
- Ingest new corpus into its OWN SpaceID
- Cross-space edges should be disallowed or tracked as contamination

#### Ingest Command

```bash
# Ingest plc-gbt into its own space (whk-wms remains)
go run ./cmd/ingest-codebase/main.go \
  -path {TARGET_REPO_PATH} \
  -space-id plc-gbt \
  -extract-symbols=false \
  -exclude ".git,vendor,node_modules,plc_backups,n8n-framework,n8n-mcp"
```

#### Required Ingest Logs

| Metric | Description |
|--------|-------------|
| LOC ingested | Lines of code |
| filetype diversity | Count + list of file types |
| elements | Code elements extracted |
| observations | Observations created |
| embedding coverage | % with embeddings |
| health score | Post-ingest health |
| ingestion time | Wall clock time |
| throughput | elements/sec |

#### Post-Ingest Invariants

- [ ] Nodes/edges for first corpus unchanged (except global indexes)
- [ ] Second corpus has non-zero nodes/edges
- [ ] Cross-space edges = 0 (or tracked as contamination)

### Phase 3 — Consolidate (Multi-Corpus)

Run consolidation with both SpaceIDs present:

```bash
# Consolidate each space
curl -X POST 'http://localhost:9999/v1/memory/consolidate' \
  -H 'Content-Type: application/json' \
  -d '{"space_id":"whk-wms"}'

curl -X POST 'http://localhost:9999/v1/memory/consolidate' \
  -H 'Content-Type: application/json' \
  -d '{"space_id":"plc-gbt"}'
```

#### Required Consolidation Logs

| Metric | Description |
|--------|-------------|
| nodes/edges created | During consolidation |
| concept layer nodes per space | Hidden/concept nodes |
| co-activation edges | Learning edges created |
| time spent | Consolidation duration |
| post-consolidation health | Health score |

### Phase 4 — Neo4j Verification (Critical)

Execute these Cypher queries to verify isolation:

#### 4.1 Counts by SpaceID

```cypher
MATCH (n:MemoryNode)
RETURN n.spaceId AS spaceId, count(n) AS nodes
ORDER BY nodes DESC;
```

#### 4.2 Edges by SpaceID

```cypher
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
RETURN a.spaceId AS fromSpace, b.spaceId AS toSpace, type(r) AS relType, count(r) AS rels
ORDER BY rels DESC;
```

#### 4.3 Cross-Space Contamination Check (CRITICAL)

```cypher
-- Count cross-space relationships
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE a.spaceId <> b.spaceId
RETURN count(r) AS crossSpaceRels;

-- If non-zero, list examples:
MATCH (a:MemoryNode)-[r]->(b:MemoryNode)
WHERE a.spaceId <> b.spaceId
RETURN a.spaceId, type(r), b.spaceId, a.name, b.name
LIMIT 25;
```

#### 4.4 Hidden Layer Nodes per Space

```cypher
MATCH (n:MemoryNode)
WHERE n.layer > 0
RETURN n.spaceId AS spaceId, n.layer AS layer, count(n) AS count
ORDER BY spaceId, layer;
```

#### 4.5 File Distribution per Space

```cypher
MATCH (o:Observation)
RETURN o.spaceId AS spaceId, count(o) AS files
ORDER BY files DESC;
```

#### Pass Criteria for "Consolidation Confirmed"

- [ ] Both spaces present with expected magnitude
- [ ] crossSpaceRels == 0 (or explain why not)
- [ ] Concept/cluster layers exist in each space
- [ ] Basic retrieval returns evidence from correct space

### Phase 5 — Multi-Corpus Benchmark

Run benchmark with both corpora loaded:

#### Additional Required Metrics

| Metric | Description |
|--------|-------------|
| Repo Attribution Accuracy (RAA) | % answers citing only correct spaceId |
| Cross-Repo Contamination Rate (CRCR) | % citing wrong spaceId |
| Collision subset performance | Performance on overlapping vocabulary |

#### Collision Questions

If not present, add 10 questions designed to cause confusion between repos (e.g., shared constant names, similar module names).

### Benchmark Summary Template

Use this exact structure for reports:

```markdown
# Benchmark Summary

## 1) Run Identity

| Field | Value |
|-------|-------|
| Date/time (TZ) | |
| Operator | |
| MDEMG commit | |
| Neo4j version / DB | |
| Hardware | |
| Run type | single-corpus / multi-corpus / consolidation-verify |
| Cold-start? | Y/N - what was cleared? |

## 2) Corpora + Scope

### whk-wms
| Field | Value |
|-------|-------|
| commit | |
| scope paths | |
| filetypes | |
| LOC | |
| SpaceID | |

### plc-gbt
| Field | Value |
|-------|-------|
| commit | |
| scope paths | |
| filetypes | |
| LOC | |
| SpaceID | |

## 3) Ingestion Metrics

| Metric | whk-wms | plc-gbt |
|--------|---------|---------|
| elements | | |
| observations | | |
| embedding coverage | | |
| health score | | |
| ingest time | | |
| throughput (elem/sec) | | |

## 4) Consolidation Metrics

| Metric | Value |
|--------|-------|
| consolidation time | |
| nodes added | |
| edges added | |
| concept nodes added | |
| learning edges added | |
| post-consolidation health | |

## 5) DB Verification (Neo4j)

| Metric | Value |
|--------|-------|
| nodes by space | |
| rels by space | |
| cross-space relationships | |
| filetype distribution | |
| PASS/FAIL | |

## 6) Benchmark Battery

| Field | Value |
|-------|-------|
| question set name | |
| SHA-256 | |
| N questions | |
| tiers (Tier1/Tier2) | |
| sampling seed | |
| k runs | |

## 7) Results (Aggregate)

### Core
| Metric | Value |
|--------|-------|
| mean | |
| median | |
| std | |
| CV | |
| p10/p90 | |
| min | |
| completion % | |

### Grounding
| Metric | Value |
|--------|-------|
| ECR % | |
| E-Acc % | |
| HVRR % | |
| hallucination rate | |
| refusal correctness % | |

### Multi-corpus Integrity
| Metric | Value |
|--------|-------|
| RAA % | |
| CRCR % | |
| collision subset mean | |

### Efficiency
| Metric | Value |
|--------|-------|
| tokens/Q avg, p95 | |
| tool calls/Q avg, p95 | |
| latency p50/p95 | |
| wall time | |

## 8) Top Failure Modes

- Examples of wrong answers with evidence issues
- Examples of repo confusion (if any)
- Timeouts / stalls / unknowns
```

### Skeptic-Killer Requirements

1. **Evidence must include repo identity** (spaceId or repo label) in every cited file path
2. **Publish raw JSONL** per-question results for audit

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 2.0 | 2026-01-26 | **MAJOR REVISION for reproducibility.** Added CANONICAL BENCHMARK SPECIFICATION with non-negotiable rules. Added standardized metrics (Codebase, Test, Summary). Added automated grading script (70% value + 30% keyword). Added test harness for 3 runs per condition. Added JSONL output format requirement. Added answer contamination prevention section. Updated run count to 3 (was 5). Added canonical agent prompts for baseline and MDEMG. |
| 1.4 | 2026-01-26 | Added CRITICAL finding about edge accumulation causing score degradation; updated cold start procedure with data showing 0.740 vs 0.679 score difference |
| 1.3 | 2026-01-24 | Added End-to-End Multi-Corpus Runbook with Phase 0-5, artifact requirements, Neo4j verification queries, and benchmark summary template |
| 1.2 | 2026-01-23 | Added WHK-WMS 120-question test set, baseline benchmarking rules (20-min limit, repo restrictions, disqualification criteria), master vs agent file distinction |
| 1.1 | 2026-01-23 | Added V6 Composite Test Set documentation |
| 1.0 | 2026-01-23 | Initial comprehensive guide |
