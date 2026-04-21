# Training Data Curation Pipeline

Python tools for processing TSDB exports into MLX-ready fine-tuning datasets.

## Pipeline

```
TSDB → mdemg data export → quality_filter.py → format_converter.py → dataset_versioner.py → MLX LoRA
                                                                                      │
                                                                                      └─► openai_ft_adapter.py → OpenAI FT
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

### openai_ft_adapter.py

Post-processor that converts the curated MLX chat JSONL into OpenAI fine-tuning files. Does **not** split data — `dataset_versioner.py` owns the temporal split; this adapter consumes `train.jsonl`/`val.jsonl` and emits OpenAI-shaped `combined_{train,val}.jsonl` + manifest.

Responsibilities:

- Strip `<think>...</think>` blocks from assistant content
- Validate message schema (system/user/assistant, non-empty, str content)
- Count tokens via `tiktoken.encoding_for_model(<model>)` — **not** a hard-coded encoding (gpt-4o/4.1 families use `o200k_base`, not `cl100k_base`)
- Reject records exceeding per-record context limit (65,536 for gpt-4.1-mini/gpt-4o-mini FT)
- Optional per-task specialist files under `by_task/`
- Cost estimate from token totals + model price profile
- `manifest.json` with SHA256 digests, row/token stats, per-task breakdown
- `rejection_log.jsonl` with dropped records and reasons

```bash
PYTHONPATH=. python3 -m training.openai_ft_adapter \
  --input-dir training_data/curated/sft_interactions/versioned \
  --output-dir training_data/openai_ft/20260420 \
  --model gpt-4.1-mini-2025-04-14 \
  --by-task
```

Adding a new model (e.g. `qwen-3.5-chat`) only requires appending a profile entry to `_MODEL_PROFILES` with the correct tokenizer resolver, context limit, and price.

See also: `docs/features/fine-tuning-pipeline.md` for the full upload → launch → monitor → eval → compare workflow.
