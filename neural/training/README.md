# Training Data Curation Pipeline

Python tools for processing TSDB exports into MLX-ready fine-tuning datasets.

## Pipeline

```
TSDB → mdemg data export → quality_filter.py → format_converter.py → dataset_versioner.py → MLX LoRA
```

## Tools

### quality_filter.py

8 quality gates applied to exported `llm_interactions.jsonl`:

| Gate | Check | Action |
|------|-------|--------|
| Privacy (P) | 5 regex patterns on all text fields | Hard reject |
| Empty response | `len(response) > 10` | Exclude |
| Error present | `error != ""` | Exclude |
| Duplicate prompt | SHA-256(system_prompt + user_prompt) | Keep first |
| Latency exceeded | `latency_ms < 60000` | Exclude |
| Unknown model | `model_name` non-empty | Flag |
| Stale prompt hash | Matches current ULTS spec hash | Exclude stale |
| ULTS output invalid | Matches ULTS output_schema | Exclude |

```bash
PYTHONPATH=. python3 -m training.quality_filter \
  --input /tmp/raw/llm_interactions.jsonl \
  --output /tmp/filtered/llm_interactions.jsonl \
  --ults-dir ../docs/tests/ults/specs/ \
  --report /tmp/filter_report.json
```

### format_converter.py

Converts filtered JSONL to HuggingFace MLX chat format:
```json
{"messages": [{"role": "system", "content": "..."}, {"role": "user", "content": "..."}, {"role": "assistant", "content": "..."}]}
```

RAFT 80/20 context handling: 80% of records with `retrieval_node_ids` include retrieval context metadata, 20% stripped. Deterministic via SHA-256(trace_id).

```bash
PYTHONPATH=. python3 -m training.format_converter \
  --input /tmp/filtered/llm_interactions.jsonl \
  --output /tmp/train.jsonl \
  --raft-ratio 0.8
```

### dataset_versioner.py

Assembles versioned datasets with temporal splits:

- Temporal train/test/val split (NEVER random)
- Cross-source deduplication via SHA-256(system_prompt + user_prompt)
- SHA-256 checksum per split file
- Task balance warnings
- Exogenous ratio check
- Dataset manifest with quality gates

```bash
PYTHONPATH=. python3 -m training.dataset_versioner \
  --input-dir /tmp/filtered/ \
  --output-dir /tmp/curated/v1/ \
  --version v1 \
  --train-ratio 0.8 --test-ratio 0.1 --val-ratio 0.1 \
  --min-per-task 10
```

## Dependencies

```bash
cd neural && uv pip install -e ".[training]"
```

Training optional deps: `jsonschema>=4.20.0`, `cuid2>=2.0.0`

## Tests

```bash
cd /path/to/mdemg && python3 -m pytest neural/training/tests/ -v
```

66 tests across 3 test files (quality_filter: 25, format_converter: 21, dataset_versioner: 20).
