# UTDS — Universal Training Data Specification

14th UxTS framework. Validates TSDB training data export archives (`.tar.gz` containing JSONL + `manifest.json`).

## Structure

```
docs/tests/utds/
├── README.md              # This file
├── schema/
│   └── utds.schema.json   # JSON Schema validating manifest.json
├── specs/
│   ├── training_export_standard.utds.json   # All 3 tables
│   ├── training_export_llm_only.utds.json   # LLM interactions only
│   └── training_export_minimal.utds.json    # Minimal viable
└── runners/
    ├── utds_runner.py          # Validation runner
    └── test_utds_runner.py     # 23 unit tests
```

## Key Constraints

- `privacy_scrub_violations == 0` — hard gate, any PII makes the export invalid
- `schema_version >= 8` — minimum TSDB schema version (migration 008: instance_id)
- `export_id` must match pattern `^exp-`
- SHA-256 checksums must match actual file contents
- Row counts must match actual JSONL line counts

## Usage

```bash
# Validate a single export archive
python docs/tests/utds/runners/utds_runner.py validate --archive /tmp/export.tar.gz

# Validate with strict JSONL field checking
python docs/tests/utds/runners/utds_runner.py validate --archive /tmp/export.tar.gz --strict

# Self-check (schema validates all fixture specs)
python docs/tests/utds/runners/utds_runner.py self-check

# Unit tests
python -m pytest docs/tests/utds/runners/test_utds_runner.py -v
```

## Relationship to Other Frameworks

| Framework | Validates |
|-----------|-----------|
| ULTS | LLM task specs (system prompts, output schemas) |
| UATS | API endpoint contracts |
| UTDS | Training data export archives |

UTDS follows the same runner pattern as ULTS (`docs/tests/ults/runners/ults_runner.py`).
