# MDEMG Benchmark Framework V2

**Version:** 2.3
**Status:** Active
**Created:** 2026-01-27
**Based on:** Lessons learned from Blueseer benchmark session

---

## 1. Overview

This framework defines a rigorous, reproducible methodology for benchmarking MDEMG retrieval performance against baseline (no-retrieval) agents on codebase comprehension tasks.

### Goals

- **Reproducibility**: Same inputs produce comparable outputs across runs
- **Isolation**: Each run is independent with no cross-contamination
- **Validation**: Real-time detection of execution issues
- **Fairness**: Identical conditions for baseline and MDEMG agents

### Non-Goals

- Speed optimization (quality over speed)
- Parallel execution of dependent runs (sequential for learning edge accumulation)

---

## 2. Pre-Execution Checklist

### 2.1 Environment Validation

```bash
# Verify MDEMG server running
curl -s http://localhost:9999/healthz | jq .status

# Verify target codebase exists
ls -la /path/to/target/repo

# Verify question file
python3 -c "import json; q=json.load(open('questions.json')); print(f'{len(q[\"questions\"])} questions')"

# Verify grading script
python3 grade_answers_v3.py --help
```

### 2.2 Space Preparation

For clean benchmarks, start with fresh ingestion:

```bash
# Clear existing space data
curl -X DELETE "http://localhost:9999/v1/memory/space/{space_id}"

# Re-ingest codebase
./bin/ingest-codebase -space-id {space_id} -endpoint http://localhost:9999 /path/to/repo

# Run consolidation
curl -X POST "http://localhost:9999/v1/memory/consolidate" -d '{"space_id": "{space_id}"}'

# Generate LLM summaries (if enabled)
curl -X POST "http://localhost:9999/v1/memory/summarize" -d '{"space_id": "{space_id}"}'

# Record baseline stats
curl -s "http://localhost:9999/v1/memory/stats?space_id={space_id}" > pre_benchmark_stats.json
```

### 2.3 Prompt Standardization

**CRITICAL**: All agent prompts must be finalized BEFORE any execution begins.

Create two prompt files:

- `baseline_prompt.txt` - For baseline agents
- `mdemg_prompt.txt` - For MDEMG agents

Prompts must include:

- [ ] Exact file access restrictions
- [ ] Forbidden tool list (WebSearch, WebFetch)
- [ ] Question ID integrity rules
- [ ] Output file path
- [ ] Output format specification
- [ ] Evidence requirements (file:line refs)

---

## 3. Agent Configuration

### 3.1 Baseline Agent Prompt Template

```
You are answering benchmark questions about the {CODEBASE_NAME} codebase.

## STRICT RULES - VIOLATION = DISQUALIFICATION
1. You may ONLY access files within: {REPO_PATH}
2. You may NOT use WebSearch or WebFetch
3. You MUST answer ALL {QUESTION_COUNT} questions
4. You MUST include file:line references in every answer
5. Use EXACT question ID from input - DO NOT renumber

## WORKFLOW (repeat for each question)
1. Read ONE question
2. Search for relevant files (Glob/Grep)
3. Read source code
4. Write answer IMMEDIATELY to output file
5. Move to next question

## OUTPUT FORMAT
File: {OUTPUT_FILE}
Format: {"id": N, "question": "...", "answer": "...", "file_line_refs": ["file.java:123"]}

## BEGIN
Read {QUESTION_FILE} and answer questions 1-{QUESTION_COUNT} sequentially.
```

### 3.2 MDEMG Agent Prompt Template

```
You are answering benchmark questions about the {CODEBASE_NAME} codebase using MDEMG retrieval.

## STRICT RULES - VIOLATION = DISQUALIFICATION
1. You may ONLY access files within: {REPO_PATH}
2. You may NOT use WebSearch or WebFetch
3. You MUST use MDEMG retrieval for EVERY question
4. You MUST answer ALL {QUESTION_COUNT} questions
5. You MUST include file:line references in every answer
6. Use EXACT question ID from input - DO NOT renumber

## WORKFLOW (repeat for each question)
1. Read ONE question
2. Query MDEMG: curl -s -X POST "http://localhost:9999/v1/memory/retrieve" \
     -H "Content-Type: application/json" \
     -d '{"space_id": "{SPACE_ID}", "query_text": "<question>", "top_k": 10}'
3. Read source files from MDEMG results
4. Write YOUR OWN answer (not raw MDEMG output) to output file
5. Move to next question

## OUTPUT FORMAT
File: {OUTPUT_FILE}
Format: {"id": N, "question": "...", "answer": "...", "file_line_refs": ["file.java:123"]}

## BEGIN
Read {QUESTION_FILE} and answer questions 1-{QUESTION_COUNT} sequentially.
```

### 3.3 Agent Isolation Requirements

| Requirement | Rationale |
|-------------|-----------|
| Fresh agent per run | Prevents context bleed between runs |
| No background execution for monitored runs | Enables real-time validation |
| Single model per benchmark | Ensures fair comparison |
| No agent restarts mid-run | Maintains consistency |

---

## 4. Execution Protocol

### 4.0 CRITICAL EXECUTION RULES (MANDATORY - READ FIRST)

**These rules exist because every benchmark failure has been an execution issue, NOT an MDEMG quality issue.**

#### NEVER DO (Violations INVALIDATE entire benchmark session)

| Rule | Violation | Consequence |
|------|-----------|-------------|
| **NEVER run multiple agents writing to same file** | Two agents both writing to `answers_mdemg_run1.jsonl` | Duplicate IDs, corrupted data |
| **NEVER run MDEMG runs in parallel** | Launching Run 2 before Run 1 completes | Learning edge contamination, file corruption |
| **NEVER start a run without clearing the output file** | Appending to existing file | Duplicate answers, invalid grading |
| **NEVER let agent access master question file** | Agent sees expected answers | Contaminated benchmark, useless results |
| **NEVER restart an agent mid-run** | Agent crashes at Q75, restart and continue | Context inconsistency, data loss |

#### ALWAYS DO (Pre-run checklist - MANDATORY)

```bash
# 1. VERIFY no other benchmark agents running
pgrep -f "benchmark" && echo "STOP: Agent already running"

# 2. CLEAR output file (or verify empty)
rm -f /path/to/answers_run_N.jsonl
touch /path/to/answers_run_N.jsonl

# 3. VERIFY file is empty
[[ $(wc -l < /path/to/answers_run_N.jsonl) -eq 0 ]] || echo "STOP: File not empty"

# 4. WAIT for previous MDEMG run to complete before starting next
# (For MDEMG runs only - learning edges must accumulate)
```

#### ONE AGENT AT A TIME (MDEMG)

```
WRONG:
  Launch MDEMG Run 1 (background)
  Launch MDEMG Run 2 (background)  ← VIOLATION: Run 1 not complete
  Launch MDEMG Run 3 (background)  ← VIOLATION: Run 2 not complete

CORRECT:
  Launch MDEMG Run 1 → WAIT for completion → Grade
  Launch MDEMG Run 2 → WAIT for completion → Grade
  Launch MDEMG Run 3 → WAIT for completion → Grade
```

#### UNIQUE OUTPUT FILES (MANDATORY)

Each run MUST have a unique output file path:

```
answers_baseline_run1.jsonl  ← Unique
answers_baseline_run2.jsonl  ← Unique
answers_baseline_run3.jsonl  ← Unique
answers_mdemg_run1.jsonl     ← Unique
answers_mdemg_run2.jsonl     ← Unique
answers_mdemg_run3.jsonl     ← Unique
```

**NEVER** reuse a file path within the same benchmark session.

---

### 4.1 Run Sequence

**Baseline Runs** (can be parallel - no learning edge dependency):

```
Baseline Run 1 → /tmp/answers_baseline_run1.jsonl
Baseline Run 2 → /tmp/answers_baseline_run2.jsonl
Baseline Run 3 → /tmp/answers_baseline_run3.jsonl
```

**MDEMG Runs** (MUST be sequential - learning edges accumulate):

```
MDEMG Run 1 (cold) → /tmp/answers_mdemg_run1.jsonl → WAIT → Grade → Record edges
MDEMG Run 2 (warm) → /tmp/answers_mdemg_run2.jsonl → WAIT → Grade → Record edges
MDEMG Run 3 (warm) → /tmp/answers_mdemg_run3.jsonl → WAIT → Grade → Record edges
```

**NOTE:** For MDEMG runs, WAIT means the orchestrator MUST NOT launch the next run until the previous run is FULLY COMPLETE (142/142 answers, no errors).

### 4.2 Per-Run Execution

```bash
# Before run
EDGES_BEFORE=$(curl -s "http://localhost:9999/v1/memory/stats?space_id={space_id}" | jq '.learning_activity.co_activated_edges')

# Execute agent (foreground for monitoring)
# ... agent execution ...

# After run - immediate validation
python3 validate_output.py /tmp/answers_{type}_run{N}.jsonl

# Record metrics
EDGES_AFTER=$(curl -s "http://localhost:9999/v1/memory/stats?space_id={space_id}" | jq '.learning_activity.co_activated_edges')
echo "Learning edges: $EDGES_BEFORE -> $EDGES_AFTER (+$(($EDGES_AFTER - $EDGES_BEFORE)))"
```

### 4.3 Disqualification Triggers

| Event | Action | Detection |
|-------|--------|-----------|
| WebSearch/WebFetch call | STOP immediately, disqualify run | Agent logs |
| File access outside repo | STOP immediately, disqualify run | Agent logs |
| Duplicate question IDs | STOP immediately, disqualify run | `grep -c '"id": 1' file` > 1 |
| Missing questions (gaps) | Flag as incomplete | Unique IDs < expected |
| Agent restart required | Discard run, start fresh | Manual observation |
| **Metadata dumping** | STOP immediately, disqualify run | See below |
| **Multiple agents same file** | STOP ALL, disqualify session | Process monitoring |

#### Metadata Dumping Detection (CRITICAL)

**Definition:** Agent copies raw MDEMG retrieval metadata as answers instead of synthesizing proper responses.

**Symptoms:**

- Answers contain `"Package:"`, `"Module:"`, `"Related to:"`
- References per question > 5 (typical: 1-3)
- Answers look like internal data structures, not natural language

**Detection Script:**

```bash
# Check for metadata patterns in answers
BAD_PATTERNS=$(grep -cE '"Package:|"Module:|"Related to:' answers.jsonl)
if [ "$BAD_PATTERNS" -gt 0 ]; then
    echo "STOP: Metadata dumping detected ($BAD_PATTERNS occurrences)"
    # Disqualify run
fi

# Check references per question
AVG_REFS=$(jq -r '.file_line_refs | length' answers.jsonl | awk '{sum+=$1} END {print sum/NR}')
if (( $(echo "$AVG_REFS > 5" | bc -l) )); then
    echo "WARNING: High ref count ($AVG_REFS avg) - check for metadata dumping"
fi
```

**Action on Detection:** STOP run immediately, mark as INVALID, investigate agent prompt.

---

## 5. Real-Time Validation

### 5.1 Output Validation Script

Create `validate_output.py`:

```python
#!/usr/bin/env python3
"""Real-time output validation for benchmark runs."""

import json
import sys
from collections import Counter

def validate(filepath, expected_count=140):
    errors = []
    warnings = []

    with open(filepath) as f:
        lines = f.readlines()

    ids = []
    for i, line in enumerate(lines, 1):
        try:
            obj = json.loads(line)
            ids.append(obj['id'])

            # Check required fields
            for field in ['id', 'question', 'answer', 'file_line_refs']:
                if field not in obj:
                    errors.append(f"Line {i}: Missing field '{field}'")

            # Check evidence
            if not obj.get('file_line_refs'):
                warnings.append(f"ID {obj['id']}: No file references")
            elif not any(':' in ref for ref in obj['file_line_refs']):
                warnings.append(f"ID {obj['id']}: Weak evidence (no line numbers)")

        except json.JSONDecodeError as e:
            errors.append(f"Line {i}: Invalid JSON - {e}")

    # Check for duplicates
    counts = Counter(ids)
    duplicates = {k: v for k, v in counts.items() if v > 1}
    if duplicates:
        errors.append(f"Duplicate IDs: {duplicates}")

    # Check for missing IDs
    missing = set(range(1, expected_count + 1)) - set(ids)
    if missing:
        errors.append(f"Missing IDs: {sorted(missing)[:10]}{'...' if len(missing) > 10 else ''}")

    # Report
    print(f"=== Validation: {filepath} ===")
    print(f"Lines: {len(lines)}")
    print(f"Unique IDs: {len(set(ids))}")
    print(f"Errors: {len(errors)}")
    print(f"Warnings: {len(warnings)}")

    for e in errors:
        print(f"  ERROR: {e}")
    for w in warnings[:5]:
        print(f"  WARN: {w}")

    return len(errors) == 0

if __name__ == "__main__":
    sys.exit(0 if validate(sys.argv[1]) else 1)
```

### 5.2 Real-Time Guardrails (MANDATORY)

Implement these guardrails to prevent run failures before they happen:

#### Checkpoint Validation (Every 10 Questions)

After every 10 answers written, automatically validate:

```python
def checkpoint_validate(filepath, last_check_count):
    """Run after every 10 new answers."""
    with open(filepath) as f:
        lines = f.readlines()

    new_answers = lines[last_check_count:]
    issues = []

    for line in new_answers:
        obj = json.loads(line)
        refs = obj.get('file_line_refs', [])

        # Check for missing line numbers
        if refs and not any(':' in ref for ref in refs):
            issues.append(f"Q{obj['id']}: Missing line numbers in refs")

    return issues
```

#### Hard-Stop Rules

| Condition | Action | Rationale |
|-----------|--------|-----------|
| **3 consecutive answers missing line numbers** | Auto-interrupt, inject correction | Prevents evidence degradation (MDEMG Run 2 failure mode) |
| **No new output for 2 minutes** | Stop run, mark INVALID | Prevents context exhaustion (Baseline Run 1 failure mode) |
| **Duplicate question ID detected** | Stop run, mark INVALID | Data integrity violation |
| **JSON parse error** | Stop run, mark INVALID | Output corruption |

#### Corrective Intervention Script

When line number issues detected:

```python
def inject_correction(agent_id, last_question_id):
    """Interrupt agent and inject correction instruction."""
    correction = f"""
STOP. Format violation detected.

Your last answers are missing line numbers in file_line_refs.
WRONG: ["ordData.java", "invData.java"]
RIGHT: ["ordData.java:123", "invData.java:456"]

Re-answer question {last_question_id} with proper file:line references.
Then continue with the next question.
"""
    # Implementation depends on agent framework
    return correction
```

#### Stall Detection

```python
import time
from pathlib import Path

def monitor_for_stall(filepath, timeout_seconds=120):
    """Return True if file hasn't been modified in timeout_seconds."""
    if not Path(filepath).exists():
        return False

    mtime = Path(filepath).stat().st_mtime
    age = time.time() - mtime
    return age > timeout_seconds
```

### 5.3 Monitoring During Execution

Every 30 seconds during agent execution:

```bash
# Check answer count
wc -l /tmp/answers_{type}_run{N}.jsonl

# Check last ID written
tail -1 /tmp/answers_{type}_run{N}.jsonl | jq .id

# Check for line number issues in last 5 answers
tail -5 /tmp/answers_{type}_run{N}.jsonl | python3 -c "
import sys, json
for line in sys.stdin:
    obj = json.loads(line)
    refs = obj.get('file_line_refs', [])
    has_lines = any(':' in r for r in refs)
    status = '✓' if has_lines else '✗ MISSING LINE NUMBERS'
    print(f'Q{obj[\"id\"]}: {status}')
"

# Detect stalls (no progress for 2+ minutes)
# If stalled: STOP RUN, mark INVALID (no restarts mid-run)
```

---

## 6. Post-Execution Analysis

### 6.1 Grading

```bash
# Grade each complete run
python3 grade_answers_v3.py \
    /tmp/answers_{type}_run{N}.jsonl \
    benchmark_questions_master.json \
    /tmp/grades_{type}_run{N}.json
```

### 6.2 Metrics to Collect

| Metric | Source | Purpose |
|--------|--------|---------|
| Mean score | Grading output | Primary quality metric |
| Strong evidence % | Grading output | Answer substantiation |
| High score rate (>=0.7) | Grading output | Consistency |
| CV (coefficient of variation) | Calculated | Score stability |
| Learning edges (before/after) | MDEMG stats | Edge accumulation |
| Completion rate | Output validation | Reliability |
| Auto-compact events | Agent logs | Context pressure indicator |

### 6.3 Run Validity Criteria

A run is **VALID** if:

- [ ] 100% questions answered (140/140)
- [ ] No duplicate IDs
- [ ] No disqualification events
- [ ] No agent restarts
- [ ] Output passes validation

A run is **PARTIAL** if:

- Questions answered < 100% but > 90%
- No disqualification events

A run is **INVALID** if:

- Disqualification event occurred
- Agent was restarted mid-run
- Duplicate IDs found
- < 90% questions answered

---

## 7. Reporting Requirements

### 7.1 Required Report Sections

1. **Overview**: Date, repo, commit, MDEMG version
2. **Configuration**: Question count, grading weights, model
3. **Run Summary**: Table of all runs with status
4. **Valid Results**: Only include VALID runs
5. **Aggregate Metrics**: Mean of valid runs only
6. **Learning Edge Progression**: For MDEMG runs
7. **Key Findings**: Insights from valid data
8. **Methodology Notes**: Any deviations from framework

### 7.2 What NOT to Include

- Results from INVALID runs (mention they were excluded)
- Results from PARTIAL runs (unless clearly labeled)
- Comparisons using inconsistent run counts

---

## 8. Known Issues & Mitigations

### 8.1 Context Exhaustion

**Symptom**: Agent stops writing answers or produces low-quality answers mid-run

**Mitigation**:

- Use smaller question batches (e.g., 50 instead of 140)
- Use haiku model for efficiency
- Monitor auto-compact events

### 8.2 Duplicate IDs

**Symptom**: Same question ID appears multiple times in output

**Cause**: Agent lost track after context compaction

**Mitigation**:

- Real-time validation catches this immediately
- If detected: discard run, do not deduplicate

### 8.3 Agent Prompt Drift

**Symptom**: Different runs produce wildly different answer styles

**Cause**: Prompt was modified between runs

**Mitigation**:

- Lock prompts before benchmark starts
- Hash prompt files and include in report

---

## 9. Checklist for Benchmark Session

### Pre-Session

- [ ] Environment validated (server, repo, scripts)
- [ ] Space cleared and re-ingested (if clean start)
- [ ] Prompts finalized and saved to files
- [ ] Question file validated
- [ ] Output directories cleared

### Per-Run

- [ ] Record pre-run learning edges (MDEMG)
- [ ] Launch agent with correct prompt
- [ ] Monitor progress every 30 seconds
- [ ] Validate output immediately after completion
- [ ] Record post-run learning edges (MDEMG)
- [ ] Grade if valid

### Post-Session

- [ ] Compile results from VALID runs only
- [ ] Calculate aggregate metrics
- [ ] Document any excluded runs with reasons
- [ ] Generate report

---

## 10. Standardized Test Summary Schema

Every benchmark session MUST produce a structured summary in JSON format for automated analysis and historical comparison.

### 10.1 Summary Schema Definition

```json
{
  "$schema": "benchmark_summary_v2",
  "metadata": {
    "benchmark_id": "string (unique identifier, e.g., blueseer-20260127-001)",
    "date": "ISO 8601 timestamp",
    "framework_version": "2.0",
    "operator": "string (who ran the benchmark)",
    "duration_minutes": "number"
  },
  "environment": {
    "mdemg_version": "string (git commit or version)",
    "mdemg_endpoint": "string (e.g., http://localhost:9999)",
    "model": "string (e.g., claude-haiku-4-5-20251001)",
    "target_repo": {
      "name": "string",
      "path": "string",
      "commit": "string (git SHA)",
      "url": "string (optional)"
    },
    "space_id": "string",
    "pre_benchmark_stats": {
      "memory_count": "number",
      "layer_0_count": "number",
      "layer_1_count": "number",
      "layer_2_count": "number",
      "initial_learning_edges": "number"
    }
  },
  "configuration": {
    "question_file": "string (path)",
    "question_count": "number",
    "grading_script": "string (path)",
    "grading_weights": {
      "evidence": "number (0-1)",
      "semantic": "number (0-1)",
      "concept": "number (0-1)"
    },
    "prompt_hash": {
      "baseline": "string (SHA256 of prompt file)",
      "mdemg": "string (SHA256 of prompt file)"
    }
  },
  "runs": [
    {
      "run_id": "string (e.g., baseline_run1)",
      "type": "baseline | mdemg",
      "sequence": "number (1, 2, 3...)",
      "status": "valid | partial | invalid | disqualified",
      "status_reason": "string (if not valid)",
      "completion": {
        "questions_answered": "number",
        "questions_expected": "number",
        "completion_rate": "number (0-1)"
      },
      "timing": {
        "start_time": "ISO 8601",
        "end_time": "ISO 8601",
        "duration_seconds": "number",
        "auto_compact_events": "number"
      },
      "output": {
        "file_path": "string",
        "file_size_bytes": "number",
        "unique_ids": "number",
        "duplicate_ids": "array of numbers (empty if none)"
      },
      "learning_edges": {
        "before": "number (MDEMG only)",
        "after": "number (MDEMG only)",
        "delta": "number (MDEMG only)"
      },
      "grading": {
        "graded": "boolean",
        "grades_file": "string (path, if graded)",
        "mean_score": "number (0-1)",
        "std_dev": "number",
        "cv_percent": "number",
        "high_score_rate": "number (0-1, scores >= 0.7)",
        "evidence_tiers": {
          "strong": "number (count with file:line)",
          "weak": "number (count with files only)",
          "none": "number (count with no refs)"
        },
        "by_difficulty": {
          "easy": {"count": "number", "mean": "number"},
          "medium": {"count": "number", "mean": "number"},
          "hard": {"count": "number", "mean": "number"}
        },
        "by_category": {
          "category_name": {"count": "number", "mean": "number"}
        }
      },
      "agent_metrics": {
        "agent_id": "string",
        "total_tokens": "number (if available)",
        "tool_calls": "number",
        "mdemg_queries": "number (MDEMG only)",
        "files_read": "number",
        "errors": "array of strings"
      }
    }
  ],
  "aggregate": {
    "baseline": {
      "valid_runs": "number",
      "mean_score": "number (average of valid runs)",
      "std_dev": "number",
      "completion_rate": "number (runs completed / runs attempted)",
      "strong_evidence_rate": "number"
    },
    "mdemg": {
      "valid_runs": "number",
      "mean_score": "number (average of valid runs)",
      "std_dev": "number",
      "completion_rate": "number",
      "strong_evidence_rate": "number",
      "total_learning_edges_created": "number"
    },
    "comparison": {
      "score_delta": "number (mdemg - baseline)",
      "score_delta_percent": "number",
      "completion_delta": "number",
      "evidence_delta": "number"
    }
  },
  "findings": {
    "key_insights": ["array of strings"],
    "anomalies": ["array of strings"],
    "excluded_runs": [
      {
        "run_id": "string",
        "reason": "string"
      }
    ]
  },
  "improvement_recommendations": {
    "mdemg_improvements": ["array of strings"],
    "framework_improvements": ["array of strings"],
    "question_improvements": ["array of strings"]
  }
}
```

### 10.2 Example Summary File

```json
{
  "$schema": "benchmark_summary_v2",
  "metadata": {
    "benchmark_id": "blueseer-20260127-001",
    "date": "2026-01-27T22:00:00Z",
    "framework_version": "2.0",
    "operator": "orchestrator",
    "duration_minutes": 45
  },
  "environment": {
    "mdemg_version": "8353e12f9766266cf0b5d00a0a6ad41073ba098e",
    "mdemg_endpoint": "http://localhost:9999",
    "model": "claude-haiku-4-5-20251001",
    "target_repo": {
      "name": "blueseer",
      "path": "/Users/reh3376/repos/blueseer",
      "commit": "1dd2ef15775ee019ee2b57794a733bf6c4ee20ba",
      "url": "https://github.com/blueseerERP/blueseer.git"
    },
    "space_id": "blueseer-erp",
    "pre_benchmark_stats": {
      "memory_count": 1303,
      "layer_0_count": 1282,
      "layer_1_count": 20,
      "layer_2_count": 1,
      "initial_learning_edges": 0
    }
  },
  "runs": [
    {
      "run_id": "baseline_run1",
      "type": "baseline",
      "sequence": 1,
      "status": "valid",
      "completion": {
        "questions_answered": 140,
        "questions_expected": 140,
        "completion_rate": 1.0
      },
      "grading": {
        "graded": true,
        "mean_score": 0.845,
        "high_score_rate": 0.993,
        "evidence_tiers": {
          "strong": 139,
          "weak": 1,
          "none": 0
        }
      }
    }
  ],
  "aggregate": {
    "baseline": {
      "valid_runs": 1,
      "mean_score": 0.845,
      "completion_rate": 1.0
    },
    "mdemg": {
      "valid_runs": 1,
      "mean_score": 0.716,
      "completion_rate": 1.0,
      "total_learning_edges_created": 22636
    }
  }
}
```

### 10.3 Summary Generation Script

Create `generate_summary.py`:

```python
#!/usr/bin/env python3
"""Generate standardized benchmark summary from run data."""

import json
import hashlib
from datetime import datetime
from pathlib import Path

def generate_summary(benchmark_id, runs_data, config):
    """Generate benchmark summary JSON."""
    summary = {
        "$schema": "benchmark_summary_v2",
        "metadata": {
            "benchmark_id": benchmark_id,
            "date": datetime.utcnow().isoformat() + "Z",
            "framework_version": "2.0",
            "operator": config.get("operator", "unknown"),
            "duration_minutes": config.get("duration_minutes", 0)
        },
        "environment": config.get("environment", {}),
        "configuration": config.get("configuration", {}),
        "runs": runs_data,
        "aggregate": calculate_aggregates(runs_data),
        "findings": {
            "key_insights": [],
            "anomalies": [],
            "excluded_runs": []
        },
        "improvement_recommendations": {
            "mdemg_improvements": [],
            "framework_improvements": [],
            "question_improvements": []
        }
    }
    return summary

def calculate_aggregates(runs):
    """Calculate aggregate metrics from runs."""
    baseline_valid = [r for r in runs if r["type"] == "baseline" and r["status"] == "valid"]
    mdemg_valid = [r for r in runs if r["type"] == "mdemg" and r["status"] == "valid"]

    aggregates = {
        "baseline": {
            "valid_runs": len(baseline_valid),
            "mean_score": avg([r["grading"]["mean_score"] for r in baseline_valid if r.get("grading", {}).get("graded")]),
            "completion_rate": avg([r["completion"]["completion_rate"] for r in baseline_valid])
        },
        "mdemg": {
            "valid_runs": len(mdemg_valid),
            "mean_score": avg([r["grading"]["mean_score"] for r in mdemg_valid if r.get("grading", {}).get("graded")]),
            "completion_rate": avg([r["completion"]["completion_rate"] for r in mdemg_valid]),
            "total_learning_edges_created": sum([r.get("learning_edges", {}).get("delta", 0) for r in mdemg_valid])
        }
    }

    # Calculate comparison
    if aggregates["baseline"]["mean_score"] and aggregates["mdemg"]["mean_score"]:
        delta = aggregates["mdemg"]["mean_score"] - aggregates["baseline"]["mean_score"]
        aggregates["comparison"] = {
            "score_delta": round(delta, 4),
            "score_delta_percent": round(delta / aggregates["baseline"]["mean_score"] * 100, 2)
        }

    return aggregates

def avg(values):
    """Calculate average, handling empty lists."""
    values = [v for v in values if v is not None]
    return round(sum(values) / len(values), 4) if values else None

if __name__ == "__main__":
    import sys
    # Usage: python generate_summary.py config.json output.json
    with open(sys.argv[1]) as f:
        config = json.load(f)
    summary = generate_summary(config["benchmark_id"], config["runs"], config)
    with open(sys.argv[2], "w") as f:
        json.dump(summary, f, indent=2)
```

---

## 11. Post-Test Run Analysis

After completing a benchmark session, perform systematic analysis to draw conclusions and identify MDEMG improvement opportunities.

### 11.1 Analysis Protocol

```
┌─────────────────────────────────────────────────────────────────┐
│                    POST-TEST ANALYSIS FLOW                       │
├─────────────────────────────────────────────────────────────────┤
│  1. Validate Results      → Confirm run validity                │
│  2. Score Analysis        → Compare baseline vs MDEMG           │
│  3. Evidence Analysis     → Examine evidence quality patterns   │
│  4. Retrieval Analysis    → Review MDEMG query effectiveness    │
│  5. Failure Analysis      → Investigate low-scoring answers     │
│  6. Edge Analysis         → Evaluate learning edge impact       │
│  7. Generate Insights     → Draw conclusions                    │
│  8. Propose Improvements  → Actionable MDEMG enhancements       │
└─────────────────────────────────────────────────────────────────┘
```

### 11.2 Score Analysis

Compare valid runs to understand performance characteristics:

```python
def analyze_scores(summary):
    """Analyze score patterns across runs."""
    analysis = {
        "score_comparison": {},
        "consistency": {},
        "trends": {}
    }

    # Compare baseline vs MDEMG means
    baseline_mean = summary["aggregate"]["baseline"]["mean_score"]
    mdemg_mean = summary["aggregate"]["mdemg"]["mean_score"]

    if baseline_mean and mdemg_mean:
        analysis["score_comparison"] = {
            "baseline_mean": baseline_mean,
            "mdemg_mean": mdemg_mean,
            "delta": round(mdemg_mean - baseline_mean, 4),
            "mdemg_advantage": mdemg_mean > baseline_mean,
            "advantage_percent": round((mdemg_mean - baseline_mean) / baseline_mean * 100, 2)
        }

    # Analyze run-to-run consistency
    mdemg_runs = [r for r in summary["runs"] if r["type"] == "mdemg" and r["status"] == "valid"]
    if len(mdemg_runs) >= 2:
        scores = [r["grading"]["mean_score"] for r in mdemg_runs]
        analysis["consistency"] = {
            "score_variance": round(max(scores) - min(scores), 4),
            "trend": "improving" if scores[-1] > scores[0] else "declining" if scores[-1] < scores[0] else "stable"
        }

    return analysis
```

**Key Questions:**

- Does MDEMG outperform baseline on average?
- Is performance consistent across runs or declining?
- Which difficulty levels show the largest MDEMG advantage?

### 11.3 Evidence Quality Analysis

Evidence quality is the primary score driver (70% weight). Analyze patterns:

```python
def analyze_evidence(grades_file):
    """Analyze evidence quality patterns."""
    with open(grades_file) as f:
        grades = json.load(f)

    analysis = {
        "strong_evidence_rate": 0,
        "weak_evidence_patterns": [],
        "category_evidence": {},
        "recommendations": []
    }

    # Count evidence tiers
    strong = sum(1 for g in grades["grades"] if g.get("evidence_tier") == "strong")
    weak = sum(1 for g in grades["grades"] if g.get("evidence_tier") == "weak")
    none = sum(1 for g in grades["grades"] if g.get("evidence_tier") == "none")

    analysis["strong_evidence_rate"] = strong / len(grades["grades"])

    # Find weak evidence patterns
    weak_answers = [g for g in grades["grades"] if g.get("evidence_tier") != "strong"]
    for answer in weak_answers:
        analysis["weak_evidence_patterns"].append({
            "question_id": answer["id"],
            "category": answer.get("category"),
            "issue": "missing_line_numbers" if answer.get("evidence_tier") == "weak" else "no_evidence"
        })

    # Recommendations based on patterns
    if analysis["strong_evidence_rate"] < 0.95:
        analysis["recommendations"].append(
            "Improve agent prompt to emphasize file:line references"
        )

    return analysis
```

**Key Questions:**

- What percentage of answers have strong evidence (file:line refs)?
- Which question categories have weak evidence?
- Are there patterns in missing evidence (e.g., cross-cutting concerns)?

### 11.4 MDEMG Retrieval Analysis

Evaluate how effectively MDEMG retrieval helped the agent:

```python
def analyze_retrieval(mdemg_queries_log, grades_file):
    """Analyze MDEMG retrieval effectiveness."""
    analysis = {
        "query_count": 0,
        "retrieval_hit_rate": 0,
        "missed_retrievals": [],
        "improvement_opportunities": []
    }

    # Load query log (if captured during run)
    # Expected format: {"question_id": N, "query": "...", "results": [...], "files_used": [...]}

    # Compare retrieved files vs files cited in answers
    # Identify cases where MDEMG didn't surface the needed file

    # Look for patterns:
    # 1. Questions where retrieved files weren't relevant
    # 2. Questions where correct file wasn't in top_k
    # 3. Query formulations that produced poor results

    return analysis
```

**Key Questions:**

- Did MDEMG surface relevant files in top_k results?
- Which questions had poor retrieval (correct file not retrieved)?
- Are there query patterns that consistently fail?

### 11.5 Failure Analysis

Investigate low-scoring answers to identify root causes:

```python
def analyze_failures(grades_file, threshold=0.5):
    """Analyze answers scoring below threshold."""
    with open(grades_file) as f:
        grades = json.load(f)

    failures = [g for g in grades["grades"] if g["score"] < threshold]

    analysis = {
        "failure_count": len(failures),
        "failure_rate": len(failures) / len(grades["grades"]),
        "failure_categories": {},
        "root_causes": [],
        "examples": []
    }

    # Categorize failures
    for f in failures:
        cat = f.get("category", "unknown")
        analysis["failure_categories"][cat] = analysis["failure_categories"].get(cat, 0) + 1

    # Identify root causes
    for f in failures[:5]:  # Sample top 5
        cause = determine_root_cause(f)
        analysis["root_causes"].append({
            "question_id": f["id"],
            "score": f["score"],
            "cause": cause
        })

    return analysis

def determine_root_cause(grade):
    """Determine why an answer scored poorly."""
    if grade.get("evidence_score", 0) < 0.3:
        return "missing_evidence"
    if grade.get("semantic_score", 0) < 0.3:
        return "semantic_mismatch"
    if grade.get("concept_score", 0) < 0.3:
        return "incorrect_concepts"
    return "unknown"
```

**Key Questions:**

- What percentage of answers score below 0.5?
- Which categories have the most failures?
- What are the root causes (missing evidence, wrong concepts, semantic mismatch)?

### 11.6 Learning Edge Analysis

Evaluate the impact of Hebbian learning edges:

```python
def analyze_learning_edges(summary):
    """Analyze learning edge accumulation and impact."""
    mdemg_runs = [r for r in summary["runs"] if r["type"] == "mdemg"]

    analysis = {
        "edge_progression": [],
        "edge_score_correlation": None,
        "saturation_detected": False,
        "recommendations": []
    }

    # Track edge growth
    for run in mdemg_runs:
        edges = run.get("learning_edges", {})
        analysis["edge_progression"].append({
            "run": run["run_id"],
            "edges_before": edges.get("before", 0),
            "edges_after": edges.get("after", 0),
            "delta": edges.get("delta", 0)
        })

    # Check for saturation (diminishing edge creation)
    if len(analysis["edge_progression"]) >= 2:
        deltas = [e["delta"] for e in analysis["edge_progression"]]
        if deltas[-1] < deltas[0] * 0.5:
            analysis["saturation_detected"] = True
            analysis["recommendations"].append(
                "Edge creation slowing - consider consolidation or hidden layer generation"
            )

    # Correlate edges with scores (if enough data points)
    # Higher edges should correlate with better retrieval

    return analysis
```

**Key Questions:**

- How many edges were created per run?
- Is edge creation rate declining (saturation)?
- Do more edges correlate with better scores?

### 11.7 Improvement Recommendations Generator

Based on analysis, generate actionable MDEMG improvements:

```python
def generate_improvements(score_analysis, evidence_analysis, retrieval_analysis, failure_analysis, edge_analysis):
    """Generate improvement recommendations based on analysis."""
    recommendations = {
        "mdemg_improvements": [],
        "retrieval_tuning": [],
        "prompt_improvements": [],
        "question_set_improvements": [],
        "framework_improvements": []
    }

    # MDEMG improvements based on retrieval gaps
    if retrieval_analysis.get("retrieval_hit_rate", 1) < 0.8:
        recommendations["mdemg_improvements"].append({
            "priority": "high",
            "area": "retrieval",
            "recommendation": "Improve semantic embedding quality - retrieval miss rate too high",
            "evidence": f"Only {retrieval_analysis['retrieval_hit_rate']*100:.1f}% of queries returned relevant files"
        })

    # Evidence-based improvements
    if evidence_analysis.get("strong_evidence_rate", 1) < 0.9:
        recommendations["prompt_improvements"].append({
            "priority": "high",
            "area": "evidence",
            "recommendation": "Strengthen prompt emphasis on file:line citations",
            "evidence": f"Strong evidence rate only {evidence_analysis['strong_evidence_rate']*100:.1f}%"
        })

    # Category-specific improvements
    failure_cats = failure_analysis.get("failure_categories", {})
    worst_category = max(failure_cats, key=failure_cats.get) if failure_cats else None
    if worst_category:
        recommendations["mdemg_improvements"].append({
            "priority": "medium",
            "area": "domain_coverage",
            "recommendation": f"Improve coverage for '{worst_category}' questions",
            "evidence": f"{failure_cats[worst_category]} failures in this category"
        })

    # Learning edge improvements
    if edge_analysis.get("saturation_detected"):
        recommendations["mdemg_improvements"].append({
            "priority": "medium",
            "area": "learning",
            "recommendation": "Run consolidation to promote edges to hidden layer concepts",
            "evidence": "Edge creation rate declining - possible saturation"
        })

    # Score trend improvements
    if score_analysis.get("consistency", {}).get("trend") == "declining":
        recommendations["framework_improvements"].append({
            "priority": "high",
            "area": "execution",
            "recommendation": "Investigate declining scores across runs - possible context exhaustion",
            "evidence": "Later runs scoring worse than earlier runs"
        })

    return recommendations
```

### 11.8 Analysis Report Template

Generate a human-readable analysis report:

```markdown
# Benchmark Analysis Report

## Executive Summary
- **Benchmark ID**: {benchmark_id}
- **Date**: {date}
- **Valid Runs**: {baseline_valid} baseline, {mdemg_valid} MDEMG
- **Overall Result**: MDEMG {advantage_direction} baseline by {delta_percent}%

## Score Analysis

### Baseline vs MDEMG
| Metric | Baseline | MDEMG | Delta |
|--------|----------|-------|-------|
| Mean Score | {baseline_mean} | {mdemg_mean} | {delta} |
| Strong Evidence | {baseline_evidence}% | {mdemg_evidence}% | {evidence_delta}% |
| Completion Rate | {baseline_completion}% | {mdemg_completion}% | {completion_delta}% |

### Run-to-Run Consistency
- Baseline variance: {baseline_variance}
- MDEMG variance: {mdemg_variance}
- Trend: {trend}

## Evidence Quality

### Evidence Tier Distribution
| Tier | Baseline | MDEMG |
|------|----------|-------|
| Strong (file:line) | {b_strong}% | {m_strong}% |
| Weak (files only) | {b_weak}% | {m_weak}% |
| None | {b_none}% | {m_none}% |

### Weak Evidence Patterns
{weak_evidence_patterns}

## Failure Analysis

### Low-Scoring Answers (< 0.5)
- Baseline: {baseline_failures} ({baseline_failure_rate}%)
- MDEMG: {mdemg_failures} ({mdemg_failure_rate}%)

### Failure Root Causes
{failure_root_causes}

## Learning Edge Analysis

### Edge Progression
| Run | Before | After | Delta |
|-----|--------|-------|-------|
{edge_table}

### Observations
- Saturation detected: {saturation}
- Total edges created: {total_edges}

## Improvement Recommendations

### High Priority
{high_priority_recommendations}

### Medium Priority
{medium_priority_recommendations}

### Low Priority
{low_priority_recommendations}

## Appendix: Excluded Runs
{excluded_runs_table}
```

### 11.9 Continuous Improvement Loop

Use analysis results to drive MDEMG development:

```
┌─────────────────────────────────────────────────────────────────┐
│                 CONTINUOUS IMPROVEMENT CYCLE                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│   ┌─────────┐    ┌──────────┐    ┌──────────┐    ┌─────────┐   │
│   │Benchmark│───▶│ Analyze  │───▶│ Identify │───▶│Implement│   │
│   │   Run   │    │ Results  │    │  Gaps    │    │  Fixes  │   │
│   └─────────┘    └──────────┘    └──────────┘    └────┬────┘   │
│        ▲                                              │         │
│        │                                              │         │
│        └──────────────────────────────────────────────┘         │
│                                                                  │
│   Examples of fixes based on analysis:                          │
│   • Low retrieval hit rate → Improve embeddings/chunking        │
│   • Missing line refs → Better LLM summary generation           │
│   • Category failures → Add domain-specific concept nodes       │
│   • Edge saturation → Tune consolidation thresholds             │
│   • Score decline → Fix context management in retrieval         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 11.10 Storing Analysis in MDEMG

Record analysis findings as observations for future reference:

```bash
# Store benchmark insights in MDEMG conversation memory
curl -X POST "http://localhost:9999/v1/conversation/observe" \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "claude-memory",
    "session_id": "benchmark-analysis-{benchmark_id}",
    "obs_type": "learning",
    "content": "{analysis_summary}",
    "tags": ["benchmark", "analysis", "{codebase_name}", "improvement"]
  }'
```

This creates a feedback loop where MDEMG learns from its own benchmark performance.

---

## 12. Answer Contamination Prevention (MANDATORY)

**The benchmark agent MUST NEVER have access to expected answers.**

| File Type | Contains Answers | Agent Access |
|-----------|------------------|--------------|
| `*_agent.json` | NO | ✅ YES - Give to agent |
| `*_master.json` | YES | ❌ NEVER - Grading only |
| `benchmark_questions_*.json` | YES | ❌ NEVER |
| `benchmark_questions_*_agent.json` | NO | ✅ YES |

**Critical Rules:**

1. Agent receives ONLY the `*_agent.json` file (questions without answers)
2. The `*_master.json` file is used ONLY by the grading script AFTER agent completes
3. NEVER include answer keys in agent prompts
4. NEVER let agent access the master question file
5. Violation of these rules INVALIDATES the entire benchmark

**File Separation:**

```
questions/
├── benchmark_questions_v1_agent.json   # → Agent input (NO answers)
└── benchmark_questions_v1_master.json  # → Grading script only (HAS answers)
```

---

## 13. Detailed Answer Format Specification

### JSONL Answer Format (MANDATORY)

Each answer MUST be a single JSON line with these exact fields:

```json
{"id": 1, "question": "What is MAX_TAKE?", "answer": "MAX_TAKE is 1000, defined in pagination.constants.ts:42", "files_consulted": ["src/pagination/pagination.constants.ts"], "file_line_refs": ["pagination.constants.ts:42"], "mdemg_skill_used": "retrieve", "confidence": "HIGH"}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | number | YES | Question ID from test set |
| `question` | string | YES | Full question text |
| `answer` | string | YES | Agent's answer (must include value + file:line) |
| `files_consulted` | array | YES | All files read/retrieved |
| `file_line_refs` | array | YES | Specific file:line citations |
| `mdemg_skill_used` | string | MDEMG only | "consult" or "retrieve" |
| `confidence` | string | YES | "HIGH", "MEDIUM", or "LOW" |

**Confidence Levels:**

- **HIGH**: Found exact value in correct file with line number
- **MEDIUM**: Found relevant file but value not confirmed
- **LOW**: Could not locate or uncertain

---

## 14. MDEMG API Configuration

### Skill vs API Access

**CRITICAL:** Sub-agents spawned via Task tool CANNOT access `/mdemg` skills. Skills are defined in `.claude/commands/` but are NOT inherited by sub-agents.

**SOLUTION:** Use direct curl API calls to the MDEMG server.

### MDEMG Skill Usage Pattern

| Question Type | MDEMG Method | Rationale |
|---------------|--------------|-----------|
| `symbol-lookup` | `/v1/memory/retrieve` | Direct symbol search |
| `multi-file`, `cross-module`, `system-wide` | `/v1/memory/consult` | Complex questions need SME synthesis |

### API Endpoints

| Endpoint | Purpose | Example |
|----------|---------|---------|
| `GET /healthz` | Health check | `curl localhost:9999/healthz` |
| `POST /v1/memory/consult` | Get SME advice | See below |
| `POST /v1/memory/retrieve` | Search memories | See below |
| `GET /v1/memory/stats` | Space statistics | `curl 'localhost:9999/v1/memory/stats?space_id=<id>'` |

### API Call Examples

```bash
# Consult API - for complex questions
curl -s 'http://localhost:9999/v1/memory/consult' \
  -H 'content-type: application/json' \
  -d '{"space_id":"<space_id>","context":"Answering benchmark question","question":"<question>"}'

# Retrieve API - for symbol lookups
curl -s 'http://localhost:9999/v1/memory/retrieve' \
  -H 'content-type: application/json' \
  -d '{"space_id":"<space_id>","query_text":"<symbol>","top_k":10}'
```

### Task Agent Configuration

**Required `allowed_tools` for benchmarks:**

| Tool | Purpose |
|------|---------|
| `Bash` | For curl API calls to MDEMG |
| `Read` | Reading question files and source files |
| `Write` | Writing JSONL output |
| `Grep`, `Glob` | File searching |

**NOTE:** Do NOT include `Skill(mdemg)` in `allowed_tools` - use curl API calls instead.

---

## 15. Advanced Metrics

### 15.1 Codebase Metrics (Collected Once Per Repo)

Capture BEFORE running benchmarks. Store in `codebase_profile.json`.

| Metric | Field Name | How to Collect |
|--------|------------|----------------|
| Total Files | `total_files` | `find . -type f \| wc -l` (filtered) |
| Total LOC | `total_loc` | `wc -l` on all source files |
| File Types | `file_types` | Extension breakdown |
| Module Count | `module_count` | Count top-level directories |
| Repo Commit | `repo_commit` | `git rev-parse HEAD` |

### 15.2 Learning & State Persistence Metrics

These metrics capture MDEMG's fundamental advantage: **state persistence under context churn**.

| Metric | Name | Definition | Why it matters |
|--------|------|------------|----------------|
| **CSC** | Compaction Survival Curve | `Score(k)` where `k` is compaction count | Baseline degrades; MDEMG stays flat |
| **PCD** | Post-Compaction Delta | `Δ score` after compaction | Makes "forgetting" measurable |
| **DP@K** | Decision Persistence at K | `%` of commitments remembered after K compactions | Long-context ≠ long-term memory |
| **RRAC** | Repeat Rate after Compaction | `%` of turns repeating prior work | Detects "looping" failures |
| **CCC** | Context Churn Cost | Tokens + tool calls to recover state | Efficiency penalty of working-memory-only |

### 15.3 Isolation & Reliability Metrics

| Metric | Name | Definition | Why it matters |
|--------|------|------------|----------------|
| **RAA** | Repo Attribution Accuracy | `%` correctly identifying source corpus | Proves graph partitions knowledge |
| **CRCR** | Cross-Repo Contamination Rate | `%` citing wrong `space_id` | Multi-tenant safety |
| **E-Acc** | Evidence Accuracy | `%` of citations that support claim | Punishes citation spam |
| **WER** | Wrong Evidence Rate | `%` with hallucinated citations | RAG confidence trick failure mode |

### 15.4 Skepticism Reduction Metrics (MANDATORY)

These 4 metrics expose ways systems "cheat" or collapse in real-world usage:

| Metric | Description |
|--------|-------------|
| **WER** (Wrong-Evidence Rate) | 1 - E-Acc |
| **Cross-Space Confusion Rate** | % citing files from wrong repo |
| **Bottom-Decile Score (p10)** | Worst 10% performance floor |
| **Completion Rate** | Graded / Expected |

### 15.5 Compaction Ladder Protocol

Stress test for persistent memory:

1. **Plant Commitments**: Give agent 10-15 questions establishing decisions
2. **Forced Compaction**: Force compaction/restart every N questions
3. **Trace Persistence**: Continue 50-100 questions depending on earlier decisions
4. **Calculate Metrics**: Compare baseline (memory=off) vs MDEMG

---

## 16. Codebase Preparation

### 16.1 Clone/Locate Repository

```bash
# Clone from URL
git clone https://github.com/org/repo.git /path/to/repo

# Or use existing path
cd /path/to/existing/repo
```

### 16.2 Generate File List

```bash
find /path/to/repo -type f \
  -name "*.ts" -o -name "*.py" -o -name "*.go" -o -name "*.java" \
  -o -name "*.rs" -o -name "*.json" -o -name "*.yaml" \
  | grep -v node_modules | grep -v .git \
  | sort > file-list.txt
```

### 16.3 Analyze Structure

```bash
tree -d -L 3 /path/to/repo > structure.txt
```

---

## 17. Question Development

### 17.1 Question Categories

Create questions across these categories:

| Category | Description | Example |
|----------|-------------|---------|
| `architecture_structure` | Module organization | "Why is X marked @Global?" |
| `service_relationships` | Dependencies | "What services does X inject?" |
| `business_logic_constraints` | Domain rules | "What prevents overlapping X?" |
| `data_flow_integration` | Request flows | "Trace the flow when X happens" |
| `cross_cutting_concerns` | Auth, logging | "How does audit logging work?" |
| `negative_control` | Features that DON'T exist | "Does X support Y?" (answer: No) |
| `calibration` | Easy baseline questions | "What file defines X?" |

### 17.2 Question Quality Requirements

Questions MUST be:

1. **Multi-file** - Require understanding 2+ files
2. **Verifiable** - Have concrete, code-referenced answers
3. **Non-trivial** - Cannot be answered from single function
4. **Specific** - Avoid vague "How does X work?"

### 17.3 Question Verification

**CRITICAL:** ~30-35% of LLM-generated answers contain errors.

Common errors:

- Wrong method names
- Overstated functionality
- Incorrect enums/constants

Always verify answers against actual code before using.

---

## 18. Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-27 | Initial BENCHMARK_AGENT_RULES.md |
| 2.0 | 2026-01-27 | Complete framework rewrite based on lessons learned |
| 2.1 | 2026-01-27 | Added standardized summary schema and post-test analysis |
| 2.2 | 2026-01-28 | Merged content from BENCHMARKING_GUIDE.md (contamination prevention, advanced metrics, API config, question development) |
| 2.3 | 2026-01-28 | Added Section 4.0 CRITICAL EXECUTION RULES - explicit NEVER/ALWAYS rules to prevent parallel agent corruption, metadata dumping detection, duplicate ID detection |

---

*This framework supersedes BENCHMARK_AGENT_RULES.md and BENCHMARKING_GUIDE.md for all future benchmark testing.*
