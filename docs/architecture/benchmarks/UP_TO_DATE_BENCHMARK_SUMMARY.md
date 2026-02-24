# MDEMG Benchmark Summary

**Version:** 4.0
**Last Updated:** 2026-02-02
**Canonical Results:** benchmark_run_20260130

---

## Executive Summary

MDEMG with Edge-Type Attention achieves **0.898 mean score** on the whk-wms 120-question benchmark, surpassing baseline performance by 5.2%.

| Metric | Baseline | MDEMG + Edge Attention | Delta |
|--------|----------|------------------------|-------|
| **Mean Score** | 0.854 | **0.898** | **+5.2%** |
| Standard Deviation | 0.088 | 0.059 | -51% variance |
| High Score Rate (≥0.7) | 97.9% | **100%** | +2.1pp |
| Strong Evidence Rate | 97.9% | **100%** | +2.1pp |

---

## Canonical Benchmark

| Item | Value |
|------|-------|
| **Location** | `docs/benchmarks/whk-wms/benchmark_run_20260130/` |
| **Codebase** | whk-wms (507K LOC TypeScript) |
| **Questions** | 120 (Test Question Schema v1.0) |
| **Grader** | grader_v4.py |
| **Agent Model** | Claude Haiku (via Claude Code) |
| **Embedding Provider** | OpenAI text-embedding-3-small |

### Files

```
benchmark_run_20260130/
├── BENCHMARK_RESULTS.md          # Detailed analysis
├── grades_baseline_run1.json     # 0.863
├── grades_baseline_run2.json     # 0.845
├── grades_mdemg_run1.json        # 0.780 (pre-attention)
├── grades_mdemg_run2.json        # 0.806 (pre-attention)
└── answers_*.jsonl               # Raw answers

benchmark_run_20260130_edge_attention/
├── grades_mdemg.json             # 0.898 (with edge attention)
└── mdemg_answers.jsonl
```

---

## Edge-Type Attention

The key innovation enabling MDEMG to surpass baseline is **Edge-Type Attention** - query-aware weighting for different edge types during activation spreading.

### How It Works

| Edge Type | Code Query Weight | Architecture Query Weight |
|-----------|-------------------|---------------------------|
| CO_ACTIVATED_WITH | 1.0 (boosted) | 0.68 (reduced) |
| GENERALIZES | 0.39 (reduced) | 0.975 (boosted) |
| ABSTRACTS_TO | 0.30 (reduced) | 0.90 (boosted) |
| ASSOCIATED_WITH | 0.52 | 0.78 |

- **Code queries** prioritize CO_ACTIVATED_WITH edges (sibling symbols)
- **Architecture queries** prioritize GENERALIZES edges (L0→L1 concepts)

### Category Performance

| Category | Pre-Attention | Edge Attention | Improvement |
|----------|---------------|----------------|-------------|
| service_relationships | 0.769 | 0.916 | **+19.1%** |
| data_flow_integration | 0.753 | 0.882 | **+17.1%** |
| architecture_structure | 0.805 | 0.889 | **+10.4%** |
| cross_cutting_concerns | 0.802 | 0.870 | **+8.5%** |

---

## State Survival Under Compaction

Single-turn Q&A measures retrieval accuracy. The critical differentiator is **state survival**:

| Metric | Baseline | MDEMG |
|--------|----------|-------|
| Decision Persistence @5 compactions | 0% | 95% |

When context windows fill and auto-compaction occurs, baseline agents lose architectural decisions. MDEMG persists them in the graph.

---

## Grading Formula

```
score = min(0.70 * evidence + 0.15 * semantic + 0.15 * concept + citation_bonus, 1.0)
```

| Component | Weight | Description |
|-----------|--------|-------------|
| Evidence | 70% | 1.0 for file:line refs, 0.5 for files only |
| Semantic | 15% | N-gram Jaccard similarity |
| Concept | 15% | Technical concept overlap |
| Citation Bonus | +10% | Correct file basename cited |

---

## Reproducibility

### Question Hash

```bash
shasum -a 256 docs/benchmarks/whk-wms/test_questions_120_agent.json
# 24aa17a215e4e58b8b44c7faef9f14228edb0e6d3f8f657d867b1bfa850f7e9e
```

### Running the Benchmark

```bash
# 1. Start services
docker compose up -d
./bin/mdemg &

# 2. Ingest codebase
./bin/ingest-codebase --space-id=benchmark --path=/path/to/whk-wms

# 3. Run consolidation
curl -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" -d '{"space_id": "benchmark"}'

# 4. Run benchmark
python run_benchmark_v4.py \
  --questions test_questions_120_agent.json \
  --master test_questions_120.json \
  --output-dir benchmark_output \
  --codebase /path/to/whk-wms \
  --space-id benchmark
```

---

## Historical Comparison

| Version | Date | Baseline | MDEMG | Notes |
|---------|------|----------|-------|-------|
| v22 (pre-attention) | 2026-01-29 | 0.854 | 0.793 | Path handling issues |
| **v22 + Edge Attention** | **2026-01-30** | **0.854** | **0.898** | **Current best** |

---

## Configuration Reference

Edge-Type Attention environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `EDGE_ATTENTION_ENABLED` | `true` | Feature toggle |
| `EDGE_ATTENTION_CO_ACTIVATED` | `0.85` | CO_ACTIVATED_WITH base weight |
| `EDGE_ATTENTION_GENERALIZES` | `0.65` | GENERALIZES base weight |
| `EDGE_ATTENTION_CODE_BOOST` | `1.2` | Multiplier for code queries |
| `EDGE_ATTENTION_ARCH_BOOST` | `1.5` | Multiplier for architecture queries |

---

## Archived Benchmarks

Historical benchmark runs have been archived to `docs/archive/benchmarks/`. These used different methodologies and should not be compared directly to v22 results.

Archived codebases:

- whk-wms (pre-v22 runs)
- megatron-lm
- zed
- blueseer
- clawdbot
- plc-gbt
- vscode-scale
