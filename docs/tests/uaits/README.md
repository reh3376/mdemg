# UAITS — Universal AI Training Specification

## Overview

UAITS is the 10th UxTS framework in MDEMG, providing spec-driven governance for training data curation across multiple paradigms. It uses the UxTS template pattern (not an actual spec — UxTS is the template for building specification frameworks).

UAITS declares what training datasets an application produces, what quality contracts apply to each, and how records are routed through paradigm-specific processing pipelines.

## Framework Structure

```
docs/tests/uaits/
  schema/uaits.schema.json    # JSON Schema (draft/2020-12) contract
  specs/mdemg.uaits.json      # MDEMG concrete spec (4 datasets)
  runners/uaits_runner.py     # Schema validation + data compliance runner
```

## Paradigms

| Paradigm | Output Type | Description |
|----------|------------|-------------|
| `sft` | `chat` | Supervised fine-tuning pairs from LLM interactions |
| `dpo` | `dpo` | Direct preference optimization pairs from constraint outcomes |
| `raft` | `raft` | Retrieval-augmented fine-tuning triples |
| `curriculum` | `metadata` | Health metrics for quality weighting (not direct training data) |

## MDEMG Spec

`specs/mdemg.uaits.json` declares 4 datasets for Qwen3.6-35B-A3B fine-tuning:

| Dataset | Paradigm | Source Table | Key Properties |
|---------|----------|-------------|----------------|
| `sft_interactions` | sft | `llm_interactions` | All 16 tasks, dedup "prompt", min 500 examples |
| `dpo_preferences` | dpo | `constraint_outcomes` | Join on guidance_id, followed=chosen, contradicted=rejected |
| `raft_retrieval` | raft | `llm_interactions` | retrieval_node_ids NOT NULL, 3 tasks, ids_only mode |
| `curriculum_health` | curriculum | `metric_samples` | rsic_health_/jiminy_/j17_ metric prefixes |

## Running the Validator

Schema validation only:
```bash
mdemg data validate --spec docs/tests/uaits/specs/mdemg.uaits.json
```

Schema + data compliance:
```bash
mdemg data validate --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --data-dir /tmp/exported/
```

Save report:
```bash
mdemg data validate --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --report /tmp/uaits_report.json
```

Direct runner invocation:
```bash
python3 docs/tests/uaits/runners/uaits_runner.py \
    --spec "docs/tests/uaits/specs/*.uaits.json"
```

## Running the Curation Pipeline

```bash
mdemg data curate \
    --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --input-dir /tmp/exported/ \
    --output-dir /tmp/curated/ \
    --version v1
```

Dry run:
```bash
mdemg data curate \
    --spec docs/tests/uaits/specs/mdemg.uaits.json \
    --input-dir /tmp/exported/ \
    --output-dir /tmp/curated/ \
    --dry-run
```

## Pipeline Integration

The paradigm router (`neural/training/paradigm_router.py`) reads a UAITS spec and dispatches each dataset through its paradigm-specific pipeline:

| Paradigm | Pipeline |
|----------|---------|
| sft | quality_filter -> format_converter (chat) -> dataset_versioner |
| dpo | dpo_builder -> quality_filter -> format_converter (dpo) -> dataset_versioner |
| raft | quality_filter -> format_converter (chat+raft) -> dataset_versioner |
| curriculum | passthrough (record count only) |

## Adding a New Dataset

1. Add a dataset entry to your application's `.uaits.json` spec
2. Set `paradigm` to one of: `sft`, `dpo`, `raft`, `curriculum`
3. Configure `source.primary_table` and `source.filter`
4. Set `quality_gates` thresholds (privacy_scan, min_response_length, etc.)
5. Set `format.output_type` matching the paradigm
6. Configure `versioning` split ratios
7. Run `mdemg data validate` to verify the spec
8. Run `mdemg data curate` to process

## Cross-Application Portability

UAITS is designed for multi-application deployment. Any application (MDEMG, Forge hub, spoke modules) can declare a `.uaits.json` spec to participate in governed training data collection. The schema enforces structural consistency while allowing paradigm-specific configuration per dataset.

## Documents Accessed

| Path | Purpose |
|------|---------|
| `docs/tests/uaits/schema/uaits.schema.json` | JSON Schema contract |
| `docs/tests/uaits/specs/mdemg.uaits.json` | MDEMG concrete spec |
| `docs/tests/uaits/runners/uaits_runner.py` | Validation runner |
| `neural/training/paradigm_router.py` | Pipeline routing orchestrator |
| `neural/training/dpo_builder.py` | DPO preference pair builder |
| `neural/training/format_converter.py` | Chat + DPO format conversion |
| `neural/training/quality_filter.py` | Quality gate engine |
| `neural/training/dataset_versioner.py` | Temporal splits + versioning |
