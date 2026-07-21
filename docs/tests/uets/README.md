# UETS — Universal Emergence Test Specification

Model-agnostic test framework for evaluating LLM-driven emergence concept naming quality in MDEMG's dynamic emergence pipeline (Phase 103).

## Purpose

When dense clusters of CO_ACTIVATED_WITH edges don't match any hardcoded pattern, MDEMG sends them to an LLM for automatic concept discovery and naming. UETS evaluates which models produce the best results across five quality dimensions.

## Directory Structure

```
uets/
├── schema/uets.schema.json          # Canonical schema definition
├── specs/                            # One spec per model
│   ├── qwen2.5-72b-mlx.uets.json
│   ├── qwen2.5-14b-ollama.uets.json
│   ├── qwen3-8b-ollama.uets.json
│   ├── llama3.3-70b-ollama.uets.json
│   ├── llama3.3-70b-macstudio.uets.json
│   ├── llama3.2-3b-ollama.uets.json
│   ├── llama3.2-3b-macstudio.uets.json
│   └── llama3.2-3b-fp16-macstudio.uets.json
├── fixtures/
│   └── clusters.json                 # Extracted CO_ACTIVATED_WITH clusters
├── runners/
│   └── uets_runner.py                # Python runner (validates, extracts, hashes)
└── README.md
```

## Evaluation Patterns

| Pattern | Name | What It Validates |
|---------|------|-------------------|
| E1 | JSON Conformance | Response is valid JSON with all 3 required fields (name, description, proposed_label) |
| E2 | Label Constraint | `proposed_label` is one of: pattern, principle, bridge, concern, workflow |
| E3 | Name Quality | Concept name has 3-6 words (concise, descriptive) |
| E4 | Description Quality | Description is 10-200 words explaining cluster coherence. Threshold: `description_quality_rate` |
| E5 | Latency | Average response time within acceptable bounds |

## Quick Start

```bash
cd docs/tests/uets/runners

# 1. Extract clusters from Neo4j (requires NEO4J_PASS)
python uets_runner.py extract-clusters --output ../fixtures/clusters.json

# 2. Add fixture hashes to specs
python uets_runner.py add-hashes --spec-dir ../specs/

# 3. Validate a single model
python uets_runner.py validate --spec ../specs/llama3.2-3b-ollama.uets.json

# 4. Validate all models and generate report
python uets_runner.py validate-all --spec-dir ../specs/ --report report.json
```

## Runner Commands

| Command | Description |
|---------|-------------|
| `validate --spec <path>` | Validate a single UETS spec against its model |
| `validate-all --spec-dir <dir>` | Validate all specs in a directory |
| `add-hashes --spec-dir <dir>` | Add/update SHA256 hashes for fixture files |
| `verify-hashes --spec-dir <dir>` | Verify fixture integrity via hashes |
| `extract-clusters --output <path>` | Extract clusters from Neo4j into fixture JSON |

Common flags: `--report <path>` (JSON report output), `--endpoint <url>` (override model endpoint).

### Remote Execution

Use `--endpoint` to run any spec against any host without modifying the spec file:

```bash
# Run a local spec against the Mac Studio Ollama
python uets_runner.py validate --spec ../specs/llama3.2-3b-ollama.uets.json \
  --endpoint http://172.21.21.11:11434/api/generate

# Run all specs against a specific endpoint
python uets_runner.py validate-all --spec-dir ../specs/ \
  --endpoint http://172.21.21.11:11434/api/generate
```

Specs also include `metadata.tags` to indicate intended host (e.g., `["mac-studio", "thunderbolt"]` vs `["local"]`).

## Spec Format

Each `.uets.json` spec defines a model endpoint and expected quality thresholds:

```json
{
  "uets_version": "1.0.0",
  "model": {
    "name": "qwen2.5-72b-mlx",
    "endpoint": "http://172.21.21.11:8080/v1/chat/completions",
    "model_id": "mlx-community/Qwen2.5-72B-Instruct-4bit",
    "type": "openai"
  },
  "config": {
    "temperature": 0.3,
    "max_tokens": 500,
    "timeout_ms": 60000,
    "num_ctx": 8192
  },
  "fixture": {
    "type": "file",
    "path": "../fixtures/clusters.json",
    "sha256": "..."
  },
  "expected": {
    "thresholds": {
      "json_valid_rate": 0.95,
      "label_valid_rate": 0.90,
      "name_quality_rate": 0.70,
      "max_avg_latency_ms": 15000
    }
  }
}
```

## Adding a New Model

1. Create `specs/<model-name>.uets.json` following the schema
2. Set `model.type` to `"openai"` (OpenAI-compatible API) or `"ollama"` (Ollama generate)
3. Set appropriate thresholds based on model size/capability
4. Run: `python uets_runner.py validate --spec specs/<model-name>.uets.json`

## Fixture Regeneration

Cluster fixtures are extracted from the live Neo4j graph. Regenerate when the graph changes significantly:

```bash
python uets_runner.py extract-clusters --output ../fixtures/clusters.json --min-weight 0.1
python uets_runner.py add-hashes --spec-dir ../specs/
```

## Report Format

JSON reports include per-model aggregates and per-cluster details:

```json
{
  "timestamp": "2026-02-24T...",
  "uets_version": "1.0.0",
  "summary": {
    "total_specs": 4,
    "passed": 2,
    "failed": 2,
    "pass_rate": 50.0
  },
  "results": [
    {
      "model_name": "qwen2.5-72b-mlx",
      "status": "pass",
      "json_valid_rate": 1.0,
      "label_valid_rate": 1.0,
      "name_quality_rate": 0.857,
      "avg_latency_ms": 3200,
      "details": [...]
    }
  ]
}
```

## Benchmark Results (2026-02-24)

All 7 specs passing (7 clusters per model, warm runs):

| Model | JSON% | Label% | Name% | Avg Latency | Host |
|-------|-------|--------|-------|-------------|------|
| llama3.2-3b-macstudio (Q4) | 100% | 100% | 85.7% | 1262ms | Mac Studio |
| llama3.2-3b-ollama (Q4) | 100% | 100% | 85.7% | 1457ms | MacBook |
| llama3.2-3b-fp16-macstudio | 100% | 100% | 85.7% | 1568ms | Mac Studio |
| qwen3-8b-ollama | 100% | 100% | 28.6% | 2126ms | MacBook |
| qwen2.5-14b-ollama | 100% | 100% | 0.0% | 4398ms | MacBook |
| qwen2.5-72b-mlx | 100% | 100% | 57.1% | 4553ms | Mac Studio |
| llama3.3-70b-ollama | 100% | 100% | 85.7% | 24866ms | MacBook |

**Recommendation**: `llama3.2:3b` Q4_K_M is the best model for emergence naming — fastest latency with top-tier name quality. FP16 adds no measurable accuracy benefit. Larger models (70B, 72B) are 4-20x slower with equal or worse name quality.

**Infrastructure**: Mac Studio M4 Max (128GB) via Thunderbolt bridge (`172.21.21.11`). Ollama with `OLLAMA_FLASH_ATTENTION=1 OLLAMA_KV_CACHE_TYPE=q8_0`.

## Relationship to Phase 103

- Phase 103 implemented LLM-driven dynamic emergence naming
- Phase 103b added `LLM_ENDPOINT` config separation and the UETS framework
- The Go implementation in `internal/hidden/emergence_namer.go` uses the exact same prompt
- UETS replicates the prompt format to ensure evaluation matches production behavior
