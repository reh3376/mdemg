# MDEMG Benchmark Framework v1.0

**Locked:** 2026-01-30
**Purpose:** Stable benchmark framework for validating MDEMG improvements

---

## Framework Components

### Agent Model

- **Model:** Claude Haiku 4.5 (`haiku`)
- **Required for:** All baseline and MDEMG benchmark runs
- **Rationale:** Consistent model ensures reproducible results

### Grader

- **File:** `grader_v4_locked.py`
- **SHA-256:** `24dc39216748b79bed0b06a7f998419aefd1fc0b6da4aea385e4d79f7124aa41`
- **Version:** V4 with V3-compatible evidence logic

### Scoring Formula

```
final_score = min(1.0,
    0.70 * evidence_score +
    0.15 * semantic_score +
    0.15 * concept_score +
    citation_bonus
)
```

| Component | Weight | Description |
|-----------|--------|-------------|
| Evidence | 70% | 1.0 for any file:line citation, 0.5 for files only, 0.0 for none |
| Semantic | 15% | N-gram similarity with adaptive recall weighting |
| Concept | 15% | Technical concept overlap |
| Citation Bonus | +10% | If cited file matches expected file |

---

## Question Sets

### whk-wms (Primary)

- **Master:** `whk-wms/test_questions_120.json`
- **SHA-256:** `43201b16c0981fac000a6e15c95468fd8dd3ec55ae5978c365a8c21f669caaf4`
- **Agent version:** `whk-wms/test_questions_120_agent.json`
- **SHA-256:** `24aa17a215e4e58b8b44c7faef9f14228edb0e6d3f8f657d867b1bfa850f7e9e`
- **Questions:** 120
- **Categories:** architecture, service_relationships, data_flow, cross_cutting, business_logic, calibration

---

## Established Baselines

### whk-wms (120 questions)

| Mode | Mean | Std | CV% | Evidence Rate |
|------|------|-----|-----|---------------|
| **Baseline** | 0.84-0.86 | 0.06-0.11 | 7-13% | 96-100% |
| **MDEMG** | 0.78-0.81 | 0.12-0.13 | 14-16% | 90-93% |

**Reference runs:** `benchmark_run_20260130/`

---

## Running Benchmarks

### Prerequisites

```bash
# Verify grader integrity
shasum -a 256 docs/benchmarks/framework/grader_v4_locked.py
# Expected: 24dc39216748b79bed0b06a7f998419aefd1fc0b6da4aea385e4d79f7124aa41
```

### Grade Answers

```bash
python3 docs/benchmarks/framework/grader_v4_locked.py \
  <answers.jsonl> \
  <questions_master.json> \
  <output_grades.json>
```

### Answer File Format (JSONL)

```json
{"id": 379, "question": "...", "answer": "...", "file_line_refs": ["path/file.ts:123"]}
```

### Interpreting Results

| Score Range | Interpretation |
|-------------|----------------|
| 0.90+ | Excellent - likely has strong evidence + good semantic match |
| 0.80-0.90 | Good - solid evidence with moderate semantic match |
| 0.70-0.80 | Acceptable - evidence present but semantic/concept gaps |
| < 0.70 | Needs improvement - missing evidence or poor answer quality |

---

## Validation Checklist

Before comparing MDEMG changes:

- [ ] Agent model is Haiku 4.5
- [ ] Grader checksum matches
- [ ] Question file checksum matches
- [ ] Same codebase version ingested
- [ ] Same MDEMG server configuration
- [ ] Minimum 2 runs per condition
- [ ] All question IDs preserved (100% match)

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-30 | Initial locked framework |

---

## DO NOT MODIFY

This framework is locked. To make changes:

1. Create a new version (v1.1, v2.0, etc.)
2. Document rationale
3. Re-establish baselines
4. Update checksums
