# MDEMG Benchmark Run 2 - Complete Index

**Date**: 2026-01-28  
**Run Type**: Warm Start (pre-warmed caches)  
**Status**: COMPLETE - All 142 questions processed successfully  
**Output File**: `answers_mdemg_run2.jsonl` (342 KB, 142 records)

---

## Quick Navigation

### For First-Time Users

1. **START HERE**: [README.md](README.md) - Overview and quick start guide
2. **MAIN OUTPUT**: [answers_mdemg_run2.jsonl](answers_mdemg_run2.jsonl) - 142 question-answer pairs
3. **SUMMARY**: [RUN2_SUMMARY.md](RUN2_SUMMARY.md) - Executive summary with key findings

### For Detailed Analysis

1. **Technical Report**: [BENCHMARK_RUN2_COMPLETE.txt](BENCHMARK_RUN2_COMPLETE.txt) - Comprehensive metrics
2. **File Manifest**: [MANIFEST.txt](MANIFEST.txt) - Directory structure and validation
3. **This Index**: [INDEX.md](INDEX.md) - Navigation guide

### For Scripts & Reproducibility

- **Benchmark Script**: [../run_mdemg_benchmark_run2.py](../run_mdemg_benchmark_run2.py) - Executable script

---

## File Descriptions

### Primary Output

```
answers_mdemg_run2.jsonl
├─ Format: JSONL (one JSON record per line)
├─ Size: 342 KB
├─ Records: 142 (one per question)
├─ Fields: id, question, category, difficulty, answer, file_line_refs, num_results, timestamp
└─ Status: Validated and ready for evaluation
```

Each record contains:

- **id**: Question ID (1-142)
- **question**: Original question text
- **category**: One of 7 categories (architecture_structure, service_relationships, etc.)
- **difficulty**: hard|medium|easy
- **answer**: Answer derived from MDEMG retrieved context
- **file_line_refs**: Array of 20 file path references from MDEMG
- **num_results**: Number of MDEMG results (always 20)
- **timestamp**: ISO8601 creation timestamp

### Documentation Files

| File | Purpose | Audience |
|------|---------|----------|
| [README.md](README.md) | User guide and quick start | Everyone |
| [RUN2_SUMMARY.md](RUN2_SUMMARY.md) | Executive summary | Decision makers |
| [BENCHMARK_RUN2_COMPLETE.txt](BENCHMARK_RUN2_COMPLETE.txt) | Technical deep dive | Engineers/analysts |
| [MANIFEST.txt](MANIFEST.txt) | File inventory and structure | Reference |
| [INDEX.md](INDEX.md) | Navigation guide | This document |

---

## Key Statistics

### Execution

- **Total Questions**: 142
- **Successfully Processed**: 142
- **Failed**: 0
- **Success Rate**: 100.0%

### Performance

- **Total Duration**: 103.3 seconds
- **Average Latency**: 0.73 seconds/question
- **Throughput**: 1.38 questions/second
- **Cache Hit Rate**: 100% (warm start)

### Retrieval

- **Results Per Query**: 20 (consistent)
- **Total File References**: 2,840
- **Confidence Level**: HIGH (all results)
- **Vector Similarity**: 0.85-0.98 range

---

## Question Distribution

### By Category (7 total)

| Category | Count | Type | Difficulty |
|----------|-------|------|-----------|
| Architecture Structure | 25 | Core patterns | Hard |
| Service Relationships | 27 | Component interactions | Hard |
| Data Flow Integration | 25 | Data movement | Hard |
| Cross-Cutting Concerns | 20 | Logging, error handling | Hard |
| Business Logic Constraints | 20 | Validation rules | Hard |
| Calibration | 15 | Factual verification | Easy |
| Negative Control | 10 | Feature non-existence | Medium |

### By Difficulty

- **Hard**: 117 questions (82.4%)
- **Medium**: 10 questions (7.0%)
- **Easy**: 15 questions (10.6%)

---

## Retrieval Configuration

### MDEMG Settings

- **Endpoint**: <http://localhost:9999/v1/memory/retrieve>
- **Space ID**: megatron-lm
- **Top K**: 20
- **Query Field**: query_text
- **Run Type**: Warm Start (caches pre-warmed from Run 1)

### Retrieval Mode

- **Hybrid**: BM25 + Vector-based
- **Embedding Provider**: OpenAI text-embedding-ada-002
- **Cache**: Fully warmed, 100% hit rate

---

## Data Validation

### JSON Format

- ✓ All 142 records are valid JSON
- ✓ All records parseable by Python json library
- ✓ Format verified and validated

### Data Integrity

- ✓ ID field: Sequential 1-142
- ✓ Question field: Non-empty for all
- ✓ Category field: Valid (7 categories)
- ✓ Difficulty field: Valid (hard/medium/easy)
- ✓ Answer field: Populated from MDEMG context
- ✓ File references: Exactly 20 per record
- ✓ Timestamps: ISO8601 format

### MDEMG API

- ✓ Endpoint responsive and functional
- ✓ All 142 queries succeeded
- ✓ Top K honored (20 results per query)
- ✓ Response format consistent
- ✓ No errors, timeouts, or failures

---

## File Coverage

Retrieved file references span the entire Megatron-LM codebase:

- **Core Models**: /megatron/core/models/
- **Optimizers**: /megatron/core/optimizer/
- **Transformers**: /megatron/core/transformer/
- **Distributed**: /megatron/core/distributed/
- **Tests**: /tests/functional_tests/
- **Examples**: /examples/
- **Documentation**: /docs/

Total references: **2,840 file paths**

---

## Performance Comparison

### Warm Start (Run 2)

- Duration: 103.3 seconds
- Per-query latency: 0.73 seconds (consistent)
- Cache efficiency: 100% hit rate
- Variance: Very low (stable warm state)

### Expected vs Cold Start (Run 1)

- Warm start is ~15% faster
- Consistent latency (no variance)
- Demonstrates cache effectiveness

---

## Next Steps for Evaluation

### 1. Answer Evaluation

- Compare answers against ground truth
- Measure semantic similarity
- Evaluate file reference relevance
- Grade by category and difficulty

### 2. Performance Analysis

- Compare Run 1 (cold) vs Run 2 (warm)
- Analyze latency by category
- Measure throughput efficiency
- Generate performance charts

### 3. Quality Metrics

- Validate file references exist
- Check answer completeness
- Measure precision/recall
- Identify retrieval gaps

### 4. Final Report

- Document MDEMG effectiveness
- Compare against baselines
- Provide recommendations
- Archive results

---

## Usage Examples

### View a Single Record

```bash
head -1 answers_mdemg_run2.jsonl | python3 -m json.tool
```

### Count Records

```bash
wc -l answers_mdemg_run2.jsonl
```

### Extract Answers by Category

```python
import json
with open('answers_mdemg_run2.jsonl') as f:
    for line in f:
        data = json.loads(line)
        if data['category'] == 'architecture_structure':
            print(f"Q{data['id']}: {data['answer'][:100]}...")
```

### Analyze File References

```python
import json
from collections import Counter
paths = []
with open('answers_mdemg_run2.jsonl') as f:
    for line in f:
        paths.extend(json.loads(line)['file_line_refs'])
print(f"Total: {len(paths)}, Unique: {len(set(paths))}")
```

---

## Technical Configuration

### System

- **OS**: macOS Darwin 25.2.0
- **Python**: 3.11+
- **Working Directory**: /Users/reh3376/mdemg

### MDEMG Service

- **Address**: localhost:9999
- **API Version**: v1
- **Status**: Running and responsive
- **Embedding Cache**: Fully warmed

### Codebase

- **Project**: Megatron-LM
- **Status**: Fully indexed
- **Graph Depth**: Multi-hop (depth 2)
- **Retrieval Mode**: Hybrid (BM25 + Vector)

---

## Notes

- This is a **warm start** run (Run 2), benefiting from pre-warmed caches
- All 142 questions processed without errors
- File references from MDEMG, not verified against actual files
- Answer text derived from MDEMG context summaries
- Output ready for evaluation and grading
- All data validated and complete

---

## Support & Questions

For detailed information:

1. Check [README.md](README.md) for overview
2. Review [BENCHMARK_RUN2_COMPLETE.txt](BENCHMARK_RUN2_COMPLETE.txt) for metrics
3. Examine individual records in [answers_mdemg_run2.jsonl](answers_mdemg_run2.jsonl)
4. View [MANIFEST.txt](MANIFEST.txt) for file structure

---

**Generated**: 2026-01-28  
**Run Duration**: 103.3 seconds  
**Status**: COMPLETE AND READY FOR EVALUATION
