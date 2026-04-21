# training_data/

Working directory for MDEMG training-data pipelines. **Contents are never committed** (see `.gitignore`); only this README and `.gitkeep` are tracked.

## Layout

```
training_data/
├── raw/                        # TSDB exports (tar.gz + extracted JSONL)
├── filtered/                   # After quality_filter.py (ULTS gates + dedup)
├── converted/                  # After format_converter.py (MLX chat JSONL)
├── curated/                    # `mdemg data curate --paradigm <p>` output (preferred entry point)
│                               #   └── <paradigm>/{train,val,test}.jsonl + manifest.json
└── openai_ft/<YYYYMMDD>/       # openai_ft_adapter outputs for OpenAI fine-tuning
    ├── combined_train.jsonl    # Upload to OpenAI as training_file
    ├── combined_val.jsonl      # Upload to OpenAI as validation_file
    ├── by_task/                # Per-task specialist runs (optional)
    │   └── <task_name>.{train,val}.jsonl
    ├── manifest.json           # Row counts, tokens, cost estimate, validation checks
    ├── rejection_log.jsonl     # Records that failed OpenAI format/length gates
    ├── run_notes.md            # Job ID, fine_tuned_model, losses, cost, errors
    └── smoke_test.md           # Qualitative output comparison (base vs fine-tuned)
```

## Re-run protocol

This directory is **ephemeral**. To reset between runs:

```bash
rm -rf training_data/raw training_data/filtered training_data/converted training_data/curated training_data/openai_ft
```

For TSDB-side cleanup (removing stale error rows):
```bash
# Dry-run:
scripts/cleanup_training_data.sql | psql "$TSDB_DSN"
# Live:
mdemg data clean --space-id mdemg-dev --dry-run=false --force
```

## Related docs

- Sprint plan: `/Users/reh3376/Downloads/sprint_plan_openai_ft_data_generation.md` (FT-OAI-001)
- UAITS spec: `docs/tests/uaits/specs/mdemg.uaits.json`
- Neural training modules: `neural/training/`
