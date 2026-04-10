---
created: 2026-04-10
updated: 2026-04-10
version: v0.7.0
author: reh3376
status: active
phase: "UAITS-2026-04-10"
---

# UAITS Framework — Universal AI Training Specification

## Summary

**Feature**: UAITS Framework
**Summary**: Spec-driven training data curation with 4 paradigms (SFT, DPO, RAFT, curriculum) — the 10th UxTS framework in MDEMG, enabling governed multi-paradigm training data collection across applications.

---

## 1. Purpose & Goals

### Why UAITS Exists

MDEMG's training pipeline was originally hardcoded for a single paradigm: supervised fine-tuning (SFT) from `llm_interactions`. Three developments changed that:

1. **Constraint outcomes** (TSDB table `constraint_outcomes`, migration 011) — records whether Jiminy guidance was followed or contradicted, with `guidance_id` keys that join to `llm_interactions`. This enables DPO (Direct Preference Optimization) pair construction.

2. **RAFT context wiring** — `retrieval_node_ids` and `retrieval_scores` fields in `llm_interactions` enable retrieval-augmented fine-tuning triples for tasks that use retrieval context.

3. **Health metrics** — `metric_samples` table records RSIC health, Jiminy effectiveness, and J17 protocol metrics. While not direct training data, these provide curriculum scheduling signal for quality weighting.

With three new data types available, the pipeline needed a governance contract declaring what datasets an application produces, what quality gates apply per-dataset, and how to route records through paradigm-specific processing.

### Goals

| Goal | Description |
|------|-------------|
| **Spec-first governance** | Every training dataset must be declared in a machine-readable spec before data flows through the pipeline |
| **Multi-paradigm support** | Route records through SFT, DPO, RAFT, or curriculum pipelines based on spec declaration |
| **Quality contract enforcement** | Per-dataset quality gates (privacy, response length, latency, dedup mode) driven by spec, not hardcoded |
| **Cross-application portability** | Any application (MDEMG, Forge hub, spoke modules) can declare a UAITS spec to participate in governed training data collection |
| **Backward compatibility** | Existing pipeline behavior unchanged when no UAITS spec is provided |

### Relationship to UxTS

UxTS (Universal-x Test Specification) is a template pattern for building specification frameworks — it is not an actual spec itself. UAITS is the concrete framework built using the UxTS pattern, following the same four-layer architecture (schema, specs, runner, pipeline) used by all 15 UxTS frameworks in MDEMG.

UAITS complements two closely related frameworks:
- **ULTS** (Universal LLM Task Specification) — governs the 16 LLM task contracts (prompts, output schemas, quality metrics, training config)
- **UTDS** (Universal Training Data Specification) — governs export manifests, privacy gates, and archive integrity

Together, ULTS defines what tasks produce data, UTDS governs how that data is exported, and UAITS governs how it is curated into training datasets.

---

## 2. Structure

### Schema

`docs/tests/uaits/schema/uaits.schema.json` — JSON Schema (draft/2020-12) defining the contract.

**Required top-level fields:**

| Field | Type | Description |
|-------|------|-------------|
| `uaits_version` | string | Semver version (e.g., "1.0.0") |
| `application` | object | `name` and `version` of the declaring application |
| `metadata` | object | `author`, `created` date, `description` |
| `datasets` | array | One or more dataset declarations (minItems: 1) |

**Each dataset object requires:**

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique name matching `^[a-z][a-z0-9_]+$` |
| `paradigm` | enum | One of: `sft`, `dpo`, `raft`, `curriculum` |
| `description` | string | Human-readable description |
| `source` | object | `primary_table`, optional `join_tables` and `filter` |
| `quality_gates` | object | Privacy scan, response length, latency, dedup mode, model exclusions |
| `format` | object | `output_type` (must match paradigm), `raft_ratio`, `think_mode_preservation` |
| `versioning` | object | `train_ratio`, `test_ratio`, `val_ratio`, `temporal_split`, `min_per_task` |

**Paradigm-to-output-type mapping (enforced by runner):**

| Paradigm | Required `output_type` |
|----------|----------------------|
| `sft` | `chat` |
| `dpo` | `dpo` |
| `raft` | `raft` |
| `curriculum` | `metadata` |

### Spec

`docs/tests/uaits/specs/mdemg.uaits.json` — MDEMG's concrete spec declaring 4 datasets.

| Dataset | Paradigm | Source Table | Key Properties |
|---------|----------|-------------|----------------|
| `sft_interactions` | sft | `llm_interactions` | All 16 ULTS tasks, dedup "prompt", raft_ratio 0.8, min 500 examples |
| `dpo_preferences` | dpo | `constraint_outcomes` | Join on `guidance_id`, followed=chosen, contradicted=rejected, min 200 examples |
| `raft_retrieval` | raft | `llm_interactions` | `retrieval_node_ids` NOT NULL, 3 tasks, ids_only mode |
| `curriculum_health` | curriculum | `metric_samples` | `rsic_health_`/`jiminy_`/`j17_` metric prefixes, min 100 examples |

### Runner

`docs/tests/uaits/runners/uaits_runner.py` — validates specs against the schema and optionally checks data compliance. Uses the shared `uxts_report` module for canonical report format.

**Schema validation (41 checks per spec):**
- Required top-level fields present
- No unknown top-level fields
- Semver version format
- Application name and version present
- Each dataset has all required fields
- Paradigm is valid enum value
- `format.output_type` matches paradigm
- `versioning.temporal_split` is true
- Split ratios sum to ~1.0 (tolerance 0.01)
- Dataset names unique within spec
- `training_config.ults_task_names` match task name format
- Source table is a known TSDB table
- Quality gates dedup_mode is valid enum

**Data compliance checks (when `--data-dir` provided):**
- Corresponding JSONL file exists per dataset
- SFT records have non-empty `system_prompt`, `user_prompt`, `response`
- DPO records have `prompt`, `chosen`, `rejected` keys
- RAFT records have non-null `retrieval_node_ids`
- Sample privacy scan (first 100 records)
- Row count vs `training_config.min_examples` threshold

---

## 3. Operation

### Pipeline Architecture

```
UAITS Spec (.uaits.json)
    |
    v
Paradigm Router (paradigm_router.py)
    |
    +-- SFT:  quality_filter -> format_converter(chat) -> dataset_versioner
    |
    +-- DPO:  dpo_builder -> quality_filter -> format_converter(dpo) -> dataset_versioner
    |
    +-- RAFT: quality_filter -> format_converter(chat+raft) -> dataset_versioner
    |
    +-- Curriculum: passthrough (record count only)
```

### Paradigm Router (`neural/training/paradigm_router.py`)

The orchestrator that reads a UAITS spec and dispatches each declared dataset through its paradigm-specific pipeline.

**Key functions:**
- `load_uaits_spec(spec_path)` — load and basic-validate
- `route_dataset(dataset_config, input_dir, output_dir, ...)` — dispatch single dataset
- `run_curation(spec_path, input_dir, output_dir, ...)` — iterate all datasets, return aggregate report

**Per-paradigm routing:**

| Paradigm | Pipeline Steps | Input File |
|----------|---------------|------------|
| `sft` | quality_filter -> format_converter(chat) -> dataset_versioner | `llm_interactions.jsonl` |
| `dpo` | dpo_builder -> quality_filter -> format_converter(dpo) -> dataset_versioner | `llm_interactions.jsonl` + `constraint_outcomes.jsonl` |
| `raft` | quality_filter -> format_converter(chat+raft) -> dataset_versioner | `llm_interactions.jsonl` |
| `curriculum` | passthrough count | `metric_samples.jsonl` |

### DPO Pair Builder (`neural/training/dpo_builder.py`)

Constructs DPO preference pairs by joining `constraint_outcomes` with `llm_interactions` on `guidance_id`:

1. **Load** constraint outcomes and build a `guidance_id -> [interaction]` index
2. **Group** outcomes by `guidance_id`
3. **Pair**: for each `guidance_id` with BOTH `followed` and `contradicted` outcomes and >=2 interactions:
   - First interaction's response = **chosen** (from followed outcome)
   - Second interaction's response = **rejected** (from contradicted outcome)
4. **Filter**: skip pairs where privacy check fails or responses don't differ enough (diversity gate)

**Output format:**
```json
{
  "prompt": "<system_prompt>\n\n<user_prompt>",
  "chosen": "<response from followed interaction>",
  "rejected": "<response from contradicted interaction>",
  "metadata": {
    "guidance_id": "...",
    "constraint_id": "...",
    "constraint_code": "...",
    "chosen_similarity": 0.95,
    "rejected_similarity": 0.12,
    "task_name": "...",
    "time": "..."
  }
}
```

### Spec-Driven Quality Gates

When `uaits_spec_path` is provided to `quality_filter.py`, the following gate thresholds are overridden from the spec's `quality_gates` section:

| Gate | Spec Field | Default |
|------|-----------|---------|
| Min response length | `min_response_length` | 10 |
| Max latency | `max_latency_ms` | 60,000 ms |

When no spec is provided, the filter operates identically to its pre-UAITS behavior (full backward compatibility).

### DPO Format Support (`neural/training/format_converter.py`)

Three new functions support DPO format:
- `convert_dpo_record(record)` — normalize DPO pair to standard format
- `validate_dpo_format(record)` — check required keys (prompt, chosen, rejected)
- `run_dpo_converter(input_path, output_path)` — batch DPO conversion pipeline

The `--format dpo` CLI flag routes through the DPO pipeline instead of the chat pipeline.

### Paradigm-Aware Versioning (`neural/training/dataset_versioner.py`)

The `paradigm` parameter is now included in the versioned dataset manifest:
```json
{
  "dataset_id": "ds_abc123def456",
  "version": "v1",
  "paradigm": "dpo",
  ...
}
```

---

## 4. Cross-Application Portability

UAITS is designed for multi-application deployment. The Forge manufacturing hub and its 10+ spoke applications can each declare a `.uaits.json` spec to participate in governed training data collection.

**Application integration pattern:**
1. Application declares a UAITS spec (e.g., `forge.uaits.json`)
2. Spec declares datasets with paradigm, source tables, quality gates, and versioning rules
3. Application exports data via UTDS to JSONL
4. Paradigm router reads the spec and curates the data
5. UAITS runner validates the spec and optionally the curated output

**What varies per application:**
- Dataset names and paradigm selection
- Source tables and filters
- Quality gate thresholds
- ULTS task names (if applicable)
- Training config (min_examples, quality_gate threshold)

**What is shared:**
- Schema contract
- Runner validation logic
- Pipeline modules (quality_filter, format_converter, dataset_versioner, dpo_builder)
- Report format (via `uxts_report`)

---

## 5. CLI Commands

### `mdemg data validate`

Validates a UAITS spec against the schema and optionally checks data compliance.

```bash
# Schema validation only
mdemg data validate --spec docs/tests/uaits/specs/mdemg.uaits.json

# Schema + data compliance
mdemg data validate --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --data-dir /tmp/exported/

# Save report to file
mdemg data validate --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --report /tmp/uaits_report.json
```

### `mdemg data curate`

Runs the paradigm router to curate training data from exported JSONL.

```bash
# Full curation
mdemg data curate \
    --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --input-dir /tmp/exported/ \
    --output-dir /tmp/curated/ \
    --version v1

# Dry run
mdemg data curate \
    --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --input-dir /tmp/exported/ \
    --output-dir /tmp/curated/ \
    --dry-run
```

---

## 6. Testing

### Unit Tests (260 total)

| File | Tests | Covers |
|------|-------|--------|
| `test_dpo_builder.py` | 12 | Pair construction, filtering, format, edge cases, I/O |
| `test_paradigm_router.py` | 9 | Spec loading, routing dispatch, dry-run, curation |
| `test_format_converter.py` | 20 | Chat format + DPO format conversion + validation |
| `test_quality_filter.py` | 18 | 8 quality gates + spec-driven gate overrides + backward compat |
| `test_dataset_versioner.py` | 13 | Temporal splits, dedup, paradigm manifest metadata |

### Runner Validation

```bash
python3 docs/tests/uaits/runners/uaits_runner.py \
    --spec "docs/tests/uaits/specs/*.uaits.json"
# Output: 41/41 checks pass
```

---

## 7. Implementation Artifacts

### Files Created

| File | Purpose |
|------|---------|
| `docs/tests/uaits/schema/uaits.schema.json` | JSON Schema contract |
| `docs/tests/uaits/specs/mdemg.uaits.json` | MDEMG concrete spec (4 datasets) |
| `docs/tests/uaits/runners/uaits_runner.py` | Schema + data compliance runner |
| `neural/training/dpo_builder.py` | DPO preference pair builder |
| `neural/training/paradigm_router.py` | Spec-driven pipeline routing |
| `neural/training/tests/test_dpo_builder.py` | DPO builder tests |
| `neural/training/tests/test_paradigm_router.py` | Paradigm router tests |
| `internal/cli/data_curate.go` | `mdemg data curate` CLI |
| `internal/cli/data_validate.go` | `mdemg data validate` CLI |

### Files Modified

| File | Changes |
|------|---------|
| `neural/training/format_converter.py` | Added `convert_dpo_record()`, `validate_dpo_format()`, `run_dpo_converter()`, `--format dpo` |
| `neural/training/quality_filter.py` | Added `uaits_spec_path` parameter for spec-driven gate overrides |
| `neural/training/dataset_versioner.py` | Added `paradigm` parameter in manifest |
| `internal/cli/data.go` | Registered `curate` and `validate` subcommands |

---

## 8. Documents Accessed

| Path | Purpose |
|------|---------|
| `docs/tests/uaits/schema/uaits.schema.json` | JSON Schema contract |
| `docs/tests/uaits/specs/mdemg.uaits.json` | MDEMG concrete spec |
| `docs/tests/uaits/runners/uaits_runner.py` | Validation runner |
| `docs/tests/uaits/README.md` | Framework README |
| `docs/features/ults-framework.md` | Feature doc format reference |
| `docs/specs/FRAMEWORK_GOVERNANCE.md` | UxTS governance policy |
| `docs/development/UXTS_FRAMEWORK_MATRIX.md` | Framework operational inventory |
| `docs/guides/UXTS_DEVELOPER_GUIDE.md` | UxTS methodology reference |
| `docs/tests/uxts_report.py` | Canonical report builder |
| `neural/training/paradigm_router.py` | Pipeline routing orchestrator |
| `neural/training/dpo_builder.py` | DPO preference pair builder |
| `neural/training/format_converter.py` | Chat + DPO format conversion |
| `neural/training/quality_filter.py` | Quality gate engine |
| `neural/training/dataset_versioner.py` | Temporal splits + versioning |
| `internal/cli/data.go` | CLI command registration |
| `internal/tsdb/migrations/011_constraint_outcomes.sql` | constraint_outcomes table schema |
