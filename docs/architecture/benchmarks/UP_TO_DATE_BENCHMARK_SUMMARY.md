# MDEMG Benchmark Summary

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Version:** 5.0
**Last Updated:** 2026-03-13
**Canonical Results:** cognitive_gap_validation_20260226/mdemg/run2

---

## Executive Summary

MDEMG with ANN Optimizations achieves **0.854 mean score** on the whk-wms 120-question cognitive gap benchmark using parallel agent execution (12 workers, 5.6 min wall clock).

| Metric | Temporal Baseline (Feb 3) | Cognitive Gap (Feb 25) | ANN Optimized + Parallel (Mar 13) |
|--------|--------------------------|------------------------|-----------------------------------|
| **Mean Score** | 0.783 | 0.816 | **0.854** |
| High Score Rate (≥0.7) | 100% | 88.3% | **97.5%** |
| Strong Evidence Rate | 100% | 88.3% | **97.5%** |
| Wall Clock | ~100 min | ~60 min | **5.6 min** |
| Cost | — | — | **$7.23** |

### Key Improvements (Mar 13)

- **ANN Optimizations**: Squared activation, local-first spreading, value residual bypass, multi-rate learning
- **Parallel Execution**: 12 workers via `--parallel 12` flag (8.4x speedup)
- **Edge Count Fix**: Health score now counts all edge types (449K edges), not just CO_ACTIVATED_WITH (1,661)
- **Configurable CLI**: All previously hardcoded values now exposed as CLI flags with env var fallbacks

---

## Canonical Benchmark

| Item | Value |
|------|-------|
| **Location** | `docs/benchmarks/whk-wms/cognitive_gap_validation_20260226/mdemg/run2/` |
| **Codebase** | whk-wms (507K LOC TypeScript) |
| **Questions** | 120 (Test Question Schema v1.0) |
| **Grader** | grader_v4.py |
| **Agent Model** | Claude Haiku (via Claude Code) |
| **Embedding Provider** | OpenAI text-embedding-3-large (3072 dims) |

### Files

```
cognitive_gap_validation_20260226/
├── config.json                    # Benchmark configuration
└── mdemg/
    ├── run1/                      # Serial run (35/120, killed)
    │   ├── answers.jsonl
    │   └── progress.json
    └── run2/                      # Parallel run (120/120, canonical)
        ├── answers.jsonl          # 120 answers
        ├── grades.json            # 0.854 mean score
        ├── progress.json          # Stats + speedup metrics
        ├── metadata.json          # Session + token usage
        └── batch_*.jsonl          # Per-worker batch files
```

### Previous Canonical Runs

```
temporal_validation_20260203/      # 0.783 mean (Sonnet, pre-ANN)
benchmark_run_20260130/            # 0.898 mean (Edge Attention, V4 runner)
cognitive_gap_validation_20260225/  # 0.816 mean (Haiku, serial)
```

---

## ANN Optimizations (Active)

The current benchmark uses the full ANN optimization suite — 10 neural learning improvements over the original Edge-Type Attention system.

### Key Optimizations

| Optimization | Description | Config |
|--------------|-------------|--------|
| **Squared Activation** | Sharper, sparser signal propagation | `SCORING_ACTIVATION_SQUARED=true` |
| **Local-First Spreading** | Per-hop weight thresholds (0.5 / 0.2 / 0.05) | `ACTIVATION_HOP{0,1,2}_MIN_WEIGHT` |
| **Value Residual Bypass** | High-confidence vector matches get bonus | `SCORING_BYPASS_THRESHOLD=0.85` |
| **Multi-Rate Learning** | Context-specific eta multipliers | `LEARNING_ETA_*_MULT` |
| **Learning Rate Schedule** | Maturity-based eta scaling (cold→saturated) | `LEARNING_SCHEDULE_*` |
| **Cautious Decay** | Skip decay for recently reinforced edges | `LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS=24` |
| **Edge-Type Attention** | Query-aware edge type weighting | `EDGE_ATTENTION_*` |
| **Negative Feedback** | Weight reduction for contradicted edges | `LEARNING_NEGATIVE_*` |

### Edge-Type Attention Weights

| Edge Type | Code Query Weight | Architecture Query Weight |
|-----------|-------------------|---------------------------|
| CO_ACTIVATED_WITH | 1.0 (boosted) | 0.68 (reduced) |
| GENERALIZES | 0.39 (reduced) | 0.975 (boosted) |
| ABSTRACTS_TO | 0.30 (reduced) | 0.90 (boosted) |
| ASSOCIATED_WITH | 0.52 | 0.78 |

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
docker compose up -d neo4j
./bin/mdemg serve --auto-migrate &

# 2. Ingest codebase
./bin/mdemg ingest --space-id=whk-wms --path=/path/to/whk-wms

# 3. Run consolidation
curl -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" -d '{"space_id": "whk-wms"}'

# 4. Run benchmark (parallel — recommended)
python run_cognitive_gap_benchmark.py \
  --mode mdemg \
  --codebase /path/to/whk-wms \
  --space-id whk-wms \
  --parallel 12 \
  --skip-verify

# 5. Grade results
python grader_v4.py \
  <output-dir>/mdemg/run1/answers.jsonl \
  whk-wms/test_questions_120.json \
  <output-dir>/mdemg/run1/grades.json
```

### Cognitive Gap Benchmark Runner CLI

All configuration is exposed via CLI flags with environment variable fallbacks.

```
Required:
  --mode {baseline,mdemg}    Agent mode
  --codebase PATH            Target codebase (env: BENCHMARK_CODEBASE_PATH)
  --space-id ID              MDEMG space ID (env: BENCHMARK_SPACE_ID, required for mdemg)

Execution:
  --parallel N               Parallel workers (default: 1 = serial, recommended: 8-12)
  --run N                    Run number (default: 1)
  --start-from N             Resume from index (serial only, incompatible with --parallel)
  --questions-limit N        Limit to first N questions (0 = all)

Configuration:
  --mdemg-endpoint URL       MDEMG API (env: MDEMG_ENDPOINT, default: http://localhost:9999)
  --model MODEL              Claude model (env: BENCHMARK_MODEL, default: haiku)
  --timeout SECS             Per-question timeout (env: BENCHMARK_AGENT_TIMEOUT, default: 120)
  --checkpoint-interval N    Save progress every N questions (env: BENCHMARK_CHECKPOINT_INTERVAL, default: 5)
  --max-retries N            Rate limit retries (env: BENCHMARK_MAX_RETRIES, default: 3)
  --master PATH              Master answer file (stashed during runs)

Flags:
  --skip-verify              Skip sandbox verification
  --validate                 Run post-hoc file validation
```

### Parallel vs Serial Mode

| Mode | Flag | Session | Speed | Use Case |
|------|------|---------|-------|----------|
| Serial | `--parallel 1` (default) | Single persistent session (`-c` continuation) | ~2 q/min | Drift analysis, compaction studies |
| Parallel | `--parallel 12` | Independent agents (no `-c`) | ~21 q/min | Standard benchmarking, fast iteration |

Parallel mode uses `ProcessPoolExecutor` — each batch runs in its own process with no shared state. Results are merged from per-worker JSONL files after all workers complete.

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BENCHMARK_CODEBASE_PATH` | Target codebase path | (none — required) |
| `BENCHMARK_SPACE_ID` | MDEMG space ID | (none — required for mdemg) |
| `MDEMG_ENDPOINT` | MDEMG API endpoint | `http://localhost:9999` |
| `BENCHMARK_MODEL` | Claude model | `haiku` |
| `BENCHMARK_AGENT_TIMEOUT` | Timeout per question (seconds) | `120` |
| `BENCHMARK_CHECKPOINT_INTERVAL` | Checkpoint frequency | `5` |
| `BENCHMARK_MAX_RETRIES` | Rate limit retry count | `3` |

---

## Historical Comparison

| Version | Date | Model | Mean Score | Notes |
|---------|------|-------|------------|-------|
| v22 (pre-attention) | 2026-01-29 | Sonnet | 0.793 | Path handling issues |
| v22 + Edge Attention | 2026-01-30 | Sonnet | 0.898 | V4 programmatic runner |
| Temporal Validation | 2026-02-03 | Sonnet | 0.783 | Canonical temporal baseline |
| Cognitive Gap (serial) | 2026-02-25 | Haiku | 0.816 | Agent-based runner |
| **ANN Optimized (parallel)** | **2026-03-13** | **Haiku** | **0.854** | **Current canonical — 12 workers, 5.6 min** |

---

## Configuration Reference

### Edge-Type Attention

| Variable | Default | Description |
|----------|---------|-------------|
| `EDGE_ATTENTION_ENABLED` | `true` | Feature toggle |
| `EDGE_ATTENTION_CO_ACTIVATED` | `0.85` | CO_ACTIVATED_WITH base weight |
| `EDGE_ATTENTION_GENERALIZES` | `0.65` | GENERALIZES base weight |
| `EDGE_ATTENTION_CODE_BOOST` | `1.2` | Multiplier for code queries |
| `EDGE_ATTENTION_ARCH_BOOST` | `1.5` | Multiplier for architecture queries |

### ANN Optimizations

See `docs/architecture/08_Config_and_Tuning.md` for the full list of 28 ANN config parameters covering learning rates, activation spreading, bypass thresholds, and consolidation settings.

---

## Archived Benchmarks

Historical benchmark runs have been archived to `docs/archive/benchmarks/`. These used different methodologies and should not be compared directly to current results.

Archived codebases:

- whk-wms (pre-v22 runs)
- megatron-lm
- zed
- blueseer
- clawdbot
- plc-gbt
- vscode-scale
