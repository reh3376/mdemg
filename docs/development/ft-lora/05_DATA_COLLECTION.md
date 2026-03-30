# MDEMG Training Data Collection, Governance, Storage & Curation Plan

**Date:** 2026-03-27
**Purpose:** Define the complete data infrastructure needed to support the fine-tuning pipeline (Phases 1-12) and begin collecting training data immediately — before the fine-tuning code is built.
**Prerequisite reading:** MDEMG_FINE_TUNING_IMPLEMENTATION_PLAN.md, MDEMG_FT_BENCHMARK_RL_ADDENDUM.md, MDEMG_FT_PLAN_DEEP_DIVE_ANALYSIS.md

---

## 1. Current State Assessment

### 1.1 What Exists Today

MDEMG has two training data collectors already built:

| Collector | File | Data Format | Default | Status |
|---|---|---|---|---|
| Rerank collector | `internal/retrieval/rerank_collector.go` | JSONL: `(query, candidates[], rerank_scores[], latency_ms)` | **OFF** (`NEURAL_DATA_COLLECTION=false`) | Working, not collecting |
| Protocol collector | `internal/jiminy/protocol_data_collector.go` | JSONL: `(constraint_code, tier, outcome, comprehension, trust, ...)` | **OFF** (`J17_PROTOCOL_DATA_COLLECTION=false`) | Working, not collecting |

Both write timestamped JSONL files to `.mdemg/neural/training-data/` with automatic rotation (50MB max) and pruning (90-day retention).

### 1.2 What's Missing

The fine-tuning plan requires training data from **15 LLM call sites** — not the 11 originally identified. The full list:

| # | File | Task | Existing Collector? |
|---|---|---|---|
| 1 | `ape/llm_reflector.go` | RSIC reflection | **No** |
| 2 | `consulting/llm_classifier.go` | Constraint classification | **No** |
| 3 | `consulting/synthesis.go` | Memory synthesis | **No** |
| 4 | `hidden/cluster_summarizer.go` | Cluster summarization | **No** |
| 5 | `hidden/emergence_namer.go` | Emergence naming | **No** |
| 6 | `hidden/reclassifier.go` | Node reclassification | **No** |
| 7 | `jiminy/evaluator.go` | J9 post-action evaluation | **No** |
| 8 | `jiminy/outcome_classifier.go` | Outcome classification | **No** |
| 9 | `jiminy/synthesizer.go` | Guidance synthesis | **No** |
| 10 | `jiminy/codegen.go` | J17 code generation | **No** |
| 11 | `metalearn/generalizer.go` | Cross-space generalization | **No** |
| 12 | `retrieval/intent_translator.go` | Query rewriting | **No** |
| 13 | `retrieval/query_classifier.go` | Query type classification | **No** |
| 14 | `retrieval/rerank.go` | LLM-based reranking | Partial (rerank_collector captures neural scores, not LLM rerank I/O) |
| 15 | `summarize/service.go` | Code element summarization | **No** |

**The existing rerank and protocol collectors capture cross-encoder and J17 protocol data — not the generative LLM inputs and outputs needed for fine-tuning.** Zero of the 15 generative LLM call sites are currently logged.

### 1.3 Additional Data Sources (Not LLM Calls)

| Source | Location | Content | Fine-Tuning Value |
|---|---|---|---|
| Neo4j graph (34K+ nodes) | `bolt://localhost:7687` | MemoryNodes, Observations, constraint nodes, edges | Ground truth for classification, naming, synthesis |
| UATS specs (195 specs) | `docs/api/api-spec/uats/specs/` | Expected API responses | Format validation for structured output tasks |
| UETS specs (8 specs) | `docs/tests/uets/specs/` | Emergence naming quality criteria | Evaluation rubrics |
| Git history | `.git/` | Commit messages, diffs, decisions | Context for constraint detection |
| CMS observations | Neo4j `:Observation` nodes | Append-only development events | Real-world task context |
| System prompts | 15 `const` declarations in Go source | Task definitions | Training example system messages |
| RSIC cycle logs | In-memory + RSIC persistence | Health reports, reflection insights | Training data for reflection task |

---

## 2. Immediate Actions (Start Collecting Now)

These changes can be made **before any fine-tuning code is built**. Every day of operation without collection is lost training data.

### 2.1 Enable Existing Collectors

Add to `.mdemg/config.yaml` or environment:

```yaml
# Enable immediately — these collectors are built and tested
neural:
  data_collection: true        # NEURAL_DATA_COLLECTION=true
  data_dir: ".mdemg/neural/training-data"

j17:
  protocol_data_collection: true  # J17_PROTOCOL_DATA_COLLECTION=true
```

**Effort:** Zero code changes. Config-only. Do this today.

### 2.2 Enable RSIC Persistence

RSIC cycle logs (health reports, reflection insights, planned actions, execution outcomes) are valuable training data for the `rsic_reflection` task. Persistence is built but disabled by default.

```yaml
rsic:
  persistence_enabled: true     # RSIC_PERSISTENCE_ENABLED=true
```

**Effort:** Zero code changes. Config-only.

### 2.3 Enable Jiminy Persistence

Jiminy guidance items, feedback outcomes, and warm store snapshots provide training data for guidance synthesis, outcome classification, and J9 evaluation.

```yaml
jiminy:
  persistence_enabled: true     # JIMINY_PERSISTENCE_ENABLED=true
```

**Effort:** Zero code changes. Config-only.

---

## 3. Phase 0: LLM Interaction Logger (New Development)

This is the core data collection mechanism — **every LLM call in MDEMG gets logged**. This corresponds to Phase 1 of the implementation plan but is scoped here specifically as a data collection concern.

### 3.1 What Gets Logged

For each of the 15 LLM call sites, capture:

```go
type InteractionRecord struct {
    // Identity
    Timestamp     string `json:"timestamp"`       // RFC3339Nano
    TraceID       string `json:"trace_id"`        // unique per-call (UUIDv7)
    TaskName      string `json:"task_name"`       // e.g., "rsic_reflection"
    SpaceID       string `json:"space_id"`        // which MDEMG space
    SessionID     string `json:"session_id"`      // which agent session

    // Input
    SystemPrompt  string `json:"system_prompt"`   // full system message
    UserPrompt    string `json:"user_prompt"`     // full user message
    Think         bool   `json:"think"`           // was /think mode requested?

    // Output
    Response      string `json:"response"`        // full model response
    ThinkContent  string `json:"think_content"`   // extracted <think> block (if any)

    // Quality signals
    LatencyMs     int64  `json:"latency_ms"`
    TokensIn      int    `json:"tokens_in"`
    TokensOut     int    `json:"tokens_out"`
    ModelName     string `json:"model_name"`      // which model produced this
    Provider      string `json:"provider"`        // openai/ollama/mlx
    Error         string `json:"error,omitempty"` // error if call failed

    // Post-hoc annotations (filled later by curation pipeline)
    Quality       float64 `json:"quality,omitempty"`       // 0-1 expert rating
    QualitySource string  `json:"quality_source,omitempty"` // "human", "llm_judge", "deterministic"
    UsedForTrain  bool    `json:"used_for_train,omitempty"` // was this used in a training run?
    DatasetVer    string  `json:"dataset_ver,omitempty"`    // which dataset version included this
}
```

### 3.2 What Does NOT Get Logged

| Excluded | Reason |
|---|---|
| Embedding requests | Different model, different data format — already handled by embedding cache |
| Cross-encoder calls (rerank, NLI, tier) | These are discriminative, not generative — existing collectors handle them |
| API keys, auth tokens | Security — never write secrets to JSONL |
| Raw Neo4j credentials | Security |
| User PII (if any appears in observations) | Privacy — scrub before writing |

### 3.3 Privacy Scrubbing

Before writing any record to JSONL, apply scrubbing:

```go
func scrubRecord(rec *InteractionRecord) {
    // Remove API keys that might appear in prompts
    rec.SystemPrompt = scrubSecrets(rec.SystemPrompt)
    rec.UserPrompt = scrubSecrets(rec.UserPrompt)
    rec.Response = scrubSecrets(rec.Response)
}

func scrubSecrets(text string) string {
    // Pattern: API keys, bearer tokens, passwords
    patterns := []string{
        `(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*\S+`,
        `(?i)bearer\s+\S+`,
        `sk-[a-zA-Z0-9]{20,}`,  // OpenAI key pattern
    }
    for _, p := range patterns {
        text = regexp.MustCompile(p).ReplaceAllString(text, "[REDACTED]")
    }
    return text
}
```

### 3.4 Files Created

| File | Purpose |
|---|---|
| `internal/llmclient/interface.go` | `LLMCompleter` interface (Complete + CompleteWithUsage) |
| `internal/llmclient/interaction_logger.go` | Logging wrapper implementing `LLMCompleter` |
| `internal/llmclient/interaction_collector.go` | JSONL writer with rotation + pruning |
| `internal/llmclient/scrubber.go` | Privacy scrubbing before write |

### 3.5 Files Modified

All 15 LLM consumers change from `*llmclient.Client` to `llmclient.LLMCompleter`:

| File | Field | Task Name |
|---|---|---|
| `internal/ape/llm_reflector.go` | `llm` | `rsic_reflection` |
| `internal/consulting/llm_classifier.go` | `llm` | `constraint_classification` |
| `internal/consulting/synthesis.go` | `llm` | `memory_synthesis` |
| `internal/hidden/cluster_summarizer.go` | `llm` | `cluster_summarization` |
| `internal/hidden/emergence_namer.go` | `llm` | `emergence_naming` |
| `internal/hidden/reclassifier.go` | `llm` | `node_reclassification` |
| `internal/jiminy/evaluator.go` | `llm` | `j9_evaluation` |
| `internal/jiminy/outcome_classifier.go` | `llm` | `outcome_classification` |
| `internal/jiminy/synthesizer.go` | `llm` | `guidance_synthesis` |
| `internal/jiminy/codegen.go` | `client` | `j17_codegen` |
| `internal/metalearn/generalizer.go` | `llm` | `cross_space_generalization` |
| `internal/retrieval/intent_translator.go` | `llm` | `intent_translation` |
| `internal/retrieval/query_classifier.go` | `llm` | `query_classification` |
| `internal/retrieval/rerank.go` | (inline client) | `llm_reranking` |
| `internal/summarize/service.go` | `llm` | `code_summarization` |
| `internal/api/server.go` | (wiring) | — |

### 3.6 Configuration

```go
// New config fields:
LLMInteractionLogging  bool   // LLM_INTERACTION_LOGGING — enable LLM I/O JSONL capture (default: true)
LLMInteractionDir      string // LLM_INTERACTION_DIR — JSONL output directory (default: ".mdemg/neural/llm-interactions")
LLMInteractionMaxMB    int    // LLM_INTERACTION_MAX_MB — max file size before rotation (default: 50)
LLMInteractionRetainD  int    // LLM_INTERACTION_RETAIN_DAYS — prune files older than N days (default: 180)
```

**Default is ON** — unlike the existing collectors which default to off. Collection must be the default to avoid losing data.

---

## 4. Storage Architecture

### 4.1 Directory Layout

```
.mdemg/neural/
├── llm-interactions/                    # Phase 0: raw LLM I/O (all 15 tasks)
│   ├── interactions-20260327-143052.jsonl
│   ├── interactions-20260328-091215.jsonl
│   └── ...
│
├── training-data/                       # Existing: rerank + protocol JSONL
│   ├── rerank-20260327-143052.jsonl
│   ├── protocol-20260327-143052.jsonl
│   └── ...
│
├── datasets/                            # Phase 4+: curated training datasets
│   ├── v1/
│   │   ├── manifest.json                # provenance, α ratio, split info
│   │   ├── train.jsonl                  # training split
│   │   ├── valid.jsonl                  # validation split
│   │   └── test.jsonl                   # held-out test (temporal split)
│   ├── v2/
│   │   └── ...
│   └── anchor/                          # original teacher-distilled data (never deleted)
│       ├── anchor-teacher-distilled.jsonl
│       └── anchor-manifest.json
│
├── synthetic/                           # Phase 4B: synthetic failure examples
│   ├── property_mismatch.jsonl
│   ├── fallback_cascade.jsonl
│   ├── circular_dependency.jsonl
│   └── config_default_silencing.jsonl
│
├── hitl/                                # Phase 12: human review data
│   ├── queue/                           # items pending review
│   ├── decisions/                       # completed reviews
│   └── preferences/                     # DPO format pairs from reviews
│
├── models/                              # Fine-tuned model artifacts
│   ├── ft/v1/adapters.npz
│   ├── ft/v2/adapters.npz
│   ├── deployed/current -> ft/v2
│   └── archive/                         # old versions (external SSD)
│
└── benchmarks/                          # Phase 10: benchmark results
    ├── baseline.json                    # external LLM quality baseline
    ├── timeline.jsonl                   # all benchmark runs
    └── results/
        ├── run_20260401_060000.json
        └── ...
```

### 4.2 Storage Budget

| Data Type | Growth Rate | 6-Month Estimate | Location |
|---|---|---|---|
| LLM interactions | ~2-5 MB/day (active dev) | ~500 MB - 1 GB | Internal SSD |
| Rerank training data | ~1-3 MB/day | ~250-500 MB | Internal SSD |
| Protocol training data | ~0.5-1 MB/day | ~100-200 MB | Internal SSD |
| Curated datasets (per version) | ~20-50 MB each | ~200-500 MB (10 versions) | Internal SSD |
| Synthetic failure examples | ~5-10 MB (one-time) | ~10 MB | Internal SSD |
| HITL decisions | ~1-5 MB/month | ~10-30 MB | Internal SSD |
| Model artifacts (per version) | ~200-400 MB LoRA, ~26 GB quantized | ~130 GB (5 versions deployed) | External SSD |
| Model archive (old versions) | ~26 GB each | Unlimited | External SSD (TB5) |
| **Total (internal SSD)** | | **~2-3 GB** | Within 2TB budget |
| **Total (external SSD)** | | **~130 GB + archive** | Thunderbolt 5 external |

### 4.3 Rotation and Retention Policy

| Data Type | Rotation Trigger | Retention | Pruning |
|---|---|---|---|
| Raw LLM interactions | 50 MB per file | 180 days | Auto-prune by collector |
| Rerank JSONL | 50 MB per file | 90 days | Auto-prune (existing) |
| Protocol JSONL | 50 MB per file | 90 days | Auto-prune (existing) |
| Curated datasets | Per-version (immutable) | Indefinite | Manual archive after 10 versions |
| Anchor dataset | Never rotated | **Permanent** | Never pruned |
| Model artifacts (deployed) | Per-version | Last 5 versions on SSD | Auto-archive older to external |
| Benchmark results | Per-run | Indefinite | Manual archive after 1 year |
| HITL decisions | Per-review | Indefinite | Never pruned (human effort is irreplaceable) |

---

## 5. Data Governance

### 5.1 Data Classification

| Classification | Examples | Handling |
|---|---|---|
| **Training-safe** | System prompts, model responses, UATS specs, emergence names | Collect freely, include in training |
| **Scrub-required** | User prompts containing file paths, repo names, org-specific code | Scrub absolute paths, keep relative; scrub credentials |
| **Exclude** | API keys, auth tokens, passwords, PII | Never write to JSONL |
| **Provenance-tagged** | All data | Must carry source tag (teacher/production/synthetic/hitl) |

### 5.2 Data Quality Gates

Before any data enters a training dataset, it passes through quality filters:

| Gate | Check | Action on Failure |
|---|---|---|
| **Format validity** | Response parses as expected format (JSON for structured tasks) | Exclude from training, log to quality report |
| **Non-empty** | Response length > 10 characters | Exclude |
| **Non-error** | `error` field is empty | Exclude |
| **Non-fallback** | `score_source != "nli_fallback"` for protocol records | Exclude from comprehension training |
| **Non-duplicate** | Prompt hash not already in dataset | Keep first occurrence only |
| **Latency reasonable** | `latency_ms < 60000` (not a timeout-retry artifact) | Exclude |
| **Model version known** | `model_name` is non-empty | Flag for review |

### 5.3 Exogenous Data Ratio (Anti-Collapse Protocol)

Every curated dataset must maintain α ≥ 0.4 — at minimum 40% of training data originates from outside the fine-tuned model. Tracked in the manifest:

```json
{
    "version": "v3",
    "created": "2026-05-15T06:00:00Z",
    "total_examples": 12500,
    "source_breakdown": {
        "teacher_distilled": 4000,    // 32% — from external LLM (Claude/GPT-4)
        "production_genuine": 5500,   // 44% — from real MDEMG usage (before fine-tuned model)
        "production_ft_model": 1500,  // 12% — from fine-tuned model in production
        "synthetic_failures": 1000,   // 8%  — generated failure detection examples
        "hitl_reviewed": 500          // 4%  — human-reviewed preference pairs
    },
    "exogenous_ratio": 0.88,         // (4000 + 5500 + 1000 + 500) / 12500 = 0.88
    "exogenous_check": "PASS",       // >= 0.40
    "note": "production_genuine + teacher_distilled + synthetic + hitl are all exogenous (not from the fine-tuned model)"
}
```

### 5.4 Temporal Split Protocol

Test data MUST come from a later time period than training data to prevent temporal leakage:

```
Data timeline:
|--- Training window ---|--- Validation ---|--- Test ---|
   Week 1-6                 Week 7            Week 8-9

NOT acceptable:
|--- Random 80% train ---|--- Random 10% valid ---|--- Random 10% test ---|
  (temporal leakage: future data trains the model, past data tests it)
```

Implementation in `dataset_versioner.py`:

```python
def create_temporal_split(
    records: list[dict],
    train_end: str,      # ISO datetime: training data before this
    valid_end: str,      # ISO datetime: validation data before this
) -> tuple[list, list, list]:
    """Split records by timestamp, not randomly."""
    train = [r for r in records if r["timestamp"] < train_end]
    valid = [r for r in records if train_end <= r["timestamp"] < valid_end]
    test  = [r for r in records if r["timestamp"] >= valid_end]
    return train, valid, test
```

### 5.5 Prompt Deduplication

Hash all prompts to prevent train/test contamination:

```python
import hashlib

def prompt_hash(system: str, user: str) -> str:
    """Deterministic hash of a prompt pair."""
    combined = f"{system.strip()}|||{user.strip()}"
    return hashlib.sha256(combined.encode()).hexdigest()[:16]

def deduplicate(records: list[dict]) -> list[dict]:
    """Keep first occurrence of each unique prompt."""
    seen = set()
    result = []
    for r in records:
        h = prompt_hash(r["system_prompt"], r["user_prompt"])
        if h not in seen:
            seen.add(h)
            result.append(r)
    return result
```

---

## 6. Curation Pipeline

### 6.1 Pipeline Stages

```
Raw Collection          Filtering              Formatting              Dataset Assembly
─────────────          ─────────              ──────────              ────────────────
llm-interactions/ ──→  quality_filter.py ──→  format_converter.py ──→  dataset_versioner.py
training-data/    ──→                                                        │
synthetic/        ──→                                                        ▼
hitl/decisions/   ──→                                                  datasets/v{N}/
                                                                        ├── manifest.json
                                                                        ├── train.jsonl
                                                                        ├── valid.jsonl
                                                                        └── test.jsonl
```

### 6.2 New File: `neural/training/quality_filter.py`

```python
"""Filter raw interaction data for training quality.

Removes:
- Error records (LLM call failed)
- Empty/truncated responses
- Duplicate prompts (by hash)
- NLI fallback records
- Timeout artifacts (latency > 60s)
- Records from degraded-state periods (high NLI fallback rate)

Annotates:
- Quality score (deterministic where possible, LLM-judge for subjective)
- Source classification (teacher/production/synthetic/hitl)
- Prompt hash (for deduplication tracking)

Usage:
    python -m training.quality_filter \
        --input-dir .mdemg/neural/llm-interactions \
        --output-dir .mdemg/neural/filtered \
        --min-quality 0.6 \
        --since 2026-03-27
"""
```

### 6.3 New File: `neural/training/format_converter.py`

```python
"""Convert filtered interaction records to MLX chat training format.

Input format (MDEMG interaction JSONL):
{
    "task_name": "rsic_reflection",
    "system_prompt": "You are an RSIC...",
    "user_prompt": "{health_report_json}",
    "response": "{insights_json}",
    "think": true,
    ...
}

Output format (MLX chat JSONL):
{
    "messages": [
        {"role": "system", "content": "You are MDEMG's internal engine. Task: rsic_reflection\n\nYou are an RSIC..."},
        {"role": "user", "content": "{health_report_json}"},
        {"role": "assistant", "content": "<think>...</think>\n{insights_json}"}
    ]
}

Key transformations:
- Prepends task identifier to system prompt
- For think-mode tasks, wraps response in <think> block if think_content exists
- For no-think tasks, strips any <think> blocks from response

Usage:
    python -m training.format_converter \
        --input-dir .mdemg/neural/filtered \
        --output-dir .mdemg/neural/formatted
"""
```

### 6.4 New File: `neural/training/dataset_versioner.py`

```python
"""Assemble curated datasets with versioning, temporal splits, and provenance tracking.

Combines:
- Anchor data (teacher-distilled, always included)
- New production data (filtered interaction logs)
- Synthetic failure examples
- HITL preference pairs (converted to SFT format)

Enforces:
- Temporal split (train < valid < test by timestamp)
- Prompt deduplication (no prompt hash in both train and test)
- Exogenous ratio α ≥ 0.4 (anti-collapse)
- Minimum examples per task (at least 50 per task type)

Outputs:
- datasets/v{N}/manifest.json — full provenance
- datasets/v{N}/train.jsonl
- datasets/v{N}/valid.jsonl
- datasets/v{N}/test.jsonl

Usage:
    python -m training.dataset_versioner \
        --anchor-dir .mdemg/neural/datasets/anchor \
        --production-dir .mdemg/neural/formatted \
        --synthetic-dir .mdemg/neural/synthetic \
        --hitl-dir .mdemg/neural/hitl/preferences \
        --output-dir .mdemg/neural/datasets \
        --train-end 2026-05-01 \
        --valid-end 2026-05-08
"""

import json
import hashlib
from dataclasses import dataclass, field
from pathlib import Path
from collections import Counter

MIN_EXOGENOUS_RATIO = 0.40
MIN_EXAMPLES_PER_TASK = 50

@dataclass
class DatasetManifest:
    """Complete provenance for a curated dataset."""
    version: str
    created: str
    total_examples: int
    train_count: int
    valid_count: int
    test_count: int

    # Source breakdown
    source_breakdown: dict[str, int]
    exogenous_ratio: float
    exogenous_check: str           # PASS or FAIL

    # Task distribution
    task_distribution: dict[str, int]
    tasks_below_minimum: list[str]

    # Split info
    train_temporal_end: str
    valid_temporal_end: str
    prompt_hashes_deduplicated: int

    # Lineage
    anchor_version: str
    production_date_range: str
    synthetic_templates_used: list[str]
    hitl_reviews_included: int

    # Quality
    mean_quality_score: float
    records_excluded_by_quality: int
    records_excluded_by_dedup: int
    records_excluded_by_error: int
```

### 6.5 New File: `neural/training/entropy_monitor.py`

Anti-collapse monitoring across model versions:

```python
"""Monitor output entropy across fine-tuned model versions.

Tracks:
- Token-level entropy of model outputs
- Vocabulary diversity (unique tokens / total tokens)
- Response length distribution (mean, std, min, max)
- Per-task diversity metrics

Alerts if entropy drops > 10% between consecutive versions.

Usage:
    python -m training.entropy_monitor \
        --current .mdemg/neural/datasets/v3/test.jsonl \
        --baseline .mdemg/neural/datasets/v1/test.jsonl \
        --model .mdemg/neural/models/deployed/current \
        --output .mdemg/neural/benchmarks/entropy_report.json
"""

def compute_token_entropy(texts: list[str], tokenizer) -> float:
    """Shannon entropy over token distribution."""

def compute_vocabulary_diversity(texts: list[str], tokenizer) -> float:
    """Unique tokens / total tokens ratio."""

def check_entropy_health(
    current_outputs: list[str],
    baseline_outputs: list[str],
    tokenizer,
    threshold: float = 0.90,
) -> dict:
    """Returns health report. Flags if entropy ratio < threshold."""
    current_entropy = compute_token_entropy(current_outputs, tokenizer)
    baseline_entropy = compute_token_entropy(baseline_outputs, tokenizer)
    ratio = current_entropy / baseline_entropy
    return {
        "current_entropy": current_entropy,
        "baseline_entropy": baseline_entropy,
        "ratio": ratio,
        "healthy": ratio >= threshold,
        "alert": "ENTROPY_DECAY" if ratio < threshold else None,
    }
```

---

## 7. Teacher Distillation (Bootstrapping the Anchor Dataset)

Before the fine-tuned model exists, generate the initial training dataset using the current external LLM. This becomes the **anchor dataset** — permanently included in every training run.

### 7.1 New File: `neural/training/teacher_distill.py`

```python
"""Generate anchor training dataset via teacher distillation.

For each of the 15 tasks:
1. Collect real inputs from production interaction logs
2. Run through the external LLM (Claude/GPT-4) with the MDEMG system prompt
3. Score output quality (deterministic + LLM-judge)
4. Keep examples scoring ≥ 0.8 quality
5. Format as MLX chat JSONL

Target: 500-1000 examples per task = 7,500-15,000 total anchor examples.

Usage:
    python -m training.teacher_distill \
        --interaction-dir .mdemg/neural/llm-interactions \
        --output-dir .mdemg/neural/datasets/anchor \
        --teacher-provider openai \
        --teacher-model gpt-4o \
        --min-quality 0.8 \
        --max-per-task 1000
"""

# Task-specific generation configs
TASK_CONFIGS = {
    "rsic_reflection":           {"think": True,  "max_tokens": 2000, "target": 800},
    "constraint_classification": {"think": False, "max_tokens": 200,  "target": 1000},
    "memory_synthesis":          {"think": True,  "max_tokens": 1000, "target": 800},
    "cluster_summarization":     {"think": True,  "max_tokens": 500,  "target": 500},
    "emergence_naming":          {"think": False, "max_tokens": 200,  "target": 500},
    "node_reclassification":     {"think": False, "max_tokens": 200,  "target": 500},
    "j9_evaluation":             {"think": True,  "max_tokens": 1000, "target": 800},
    "outcome_classification":    {"think": False, "max_tokens": 200,  "target": 1000},
    "guidance_synthesis":        {"think": True,  "max_tokens": 500,  "target": 500},
    "j17_codegen":               {"think": False, "max_tokens": 50,   "target": 500},
    "cross_space_generalization": {"think": True,  "max_tokens": 500,  "target": 300},
    "intent_translation":        {"think": False, "max_tokens": 200,  "target": 500},
    "query_classification":      {"think": False, "max_tokens": 200,  "target": 500},
    "llm_reranking":             {"think": False, "max_tokens": 500,  "target": 500},
    "code_summarization":        {"think": False, "max_tokens": 300,  "target": 1000},
}
# Total target: ~9,400 anchor examples
```

### 7.2 Where Do Real Inputs Come From?

Teacher distillation needs real inputs. Before the interaction logger is built, extract inputs from existing data:

| Task | Real Input Source | Extraction Method |
|---|---|---|
| `rsic_reflection` | RSIC persistence (health reports) | Query Neo4j for RSIC cycle nodes |
| `constraint_classification` | MemoryNode contents | Random sample of nodes + known constraints |
| `memory_synthesis` | Retrieval results | Run consult queries, capture retrieved nodes |
| `cluster_summarization` | Consolidation output | Run consolidation, capture cluster members |
| `emergence_naming` | Existing named concepts | Query L3-L5 nodes with their members |
| `j9_evaluation` | Git diffs + constraints | Pair recent diffs with matching constraints |
| `outcome_classification` | Guidance + action pairs | From protocol collector (once enabled) |
| `intent_translation` | User queries | From retrieval logs or CMS resume calls |
| `query_classification` | User queries | Same source as intent translation |
| `code_summarization` | Ingested code elements | From ingest pipeline output |

### 7.3 New File: `neural/training/input_extractor.py`

Extracts real inputs from Neo4j and existing data sources for teacher distillation:

```python
"""Extract real task inputs from MDEMG for teacher distillation.

Sources:
- Neo4j graph (MemoryNodes, Observations, constraints, clusters)
- Interaction JSONL (if already collecting)
- RSIC persistence (health reports)
- Git history (diffs for J9 evaluation)

Usage:
    python -m training.input_extractor \
        --neo4j-uri bolt://localhost:7687 \
        --neo4j-user neo4j \
        --neo4j-pass testpassword \
        --output-dir .mdemg/neural/extracted-inputs \
        --task all
"""
```

---

## 8. CLI Commands

### 8.1 New Subcommands

```bash
# Data collection status
mdemg data status                     # Show collection rates, storage usage, quality stats

# Data inspection
mdemg data inspect --task rsic_reflection --last 10   # View recent interaction records
mdemg data stats                      # Per-task counts, date ranges, quality distribution

# Dataset management
mdemg data curate                     # Run full curation pipeline (filter → format → version)
mdemg data curate --dry-run           # Show what would be included/excluded
mdemg data anchor generate            # Generate anchor dataset via teacher distillation
mdemg data anchor verify              # Verify anchor dataset integrity

# Quality
mdemg data quality report             # Generate quality report across all collected data
mdemg data quality entropy            # Run entropy health check against baseline

# Governance
mdemg data manifest --version v3      # Show dataset manifest with provenance
mdemg data export --version v3 --format huggingface   # Export for external tools
```

### 8.2 New File: `internal/cli/data.go`

```go
func newDataCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "data",
        Short: "Manage training data collection and curation",
    }
    cmd.AddCommand(newDataStatusCmd())
    cmd.AddCommand(newDataInspectCmd())
    cmd.AddCommand(newDataStatsCmd())
    cmd.AddCommand(newDataCurateCmd())
    cmd.AddCommand(newDataAnchorCmd())
    cmd.AddCommand(newDataQualityCmd())
    cmd.AddCommand(newDataManifestCmd())
    cmd.AddCommand(newDataExportCmd())
    return cmd
}
```

---

## 9. RSIC Integration

### 9.1 New Reflection Patterns

| Pattern | Trigger | Action |
|---|---|---|
| **Pattern 25: Insufficient training data** | Total interaction records < 1000 across all tasks | `alert_training_data_low` — surface in RSIC report |
| **Pattern 26: Task data imbalance** | Any task has < 50 examples while others have > 500 | `alert_task_data_imbalance` — recommend targeted data collection |
| **Pattern 27: Entropy decay detected** | Entropy ratio < 0.90 between consecutive model versions | `alert_entropy_decay` — halt recursive loop, inject fresh exogenous data |
| **Pattern 28: Exogenous ratio violation** | Dataset α < 0.40 | `block_training_cycle` — refuse to train until more exogenous data is added |

### 9.2 New Prometheus Metrics

```go
// Data collection metrics
FTInteractionTotal      func(task string) *Counter   // mdemg_ft_interaction_total{task="..."}
FTInteractionErrorRate  func(task string) *Gauge     // mdemg_ft_interaction_error_rate{task="..."}
FTDataStorageBytes      func() *Gauge                // mdemg_ft_data_storage_bytes
FTDatasetVersion        func() *Gauge                // mdemg_ft_dataset_version (current)
FTExogenousRatio        func() *Gauge                // mdemg_ft_exogenous_ratio
FTEntropyRatio          func() *Gauge                // mdemg_ft_entropy_ratio
```

---

## 10. Grafana Dashboard

New dashboard: `deploy/docker/grafana/dashboards/mdemg-training-data.json`

Panels:
- **Data collection rate** — interactions/hour by task (time series)
- **Storage usage** — gauge showing current disk usage vs budget
- **Task distribution** — bar chart showing record counts per task
- **Quality distribution** — histogram of quality scores
- **Error rate** — per-task error rate over time
- **Exogenous ratio** — gauge with green/yellow/red zones (≥0.5 green, 0.4-0.5 yellow, <0.4 red)
- **Entropy health** — ratio vs baseline over model versions
- **Dataset version timeline** — annotations showing when datasets were created

---

## 11. File Inventory

### New Files (11)

| File | Language | Purpose |
|---|---|---|
| `internal/llmclient/interface.go` | Go | `LLMCompleter` interface |
| `internal/llmclient/interaction_logger.go` | Go | Logging wrapper |
| `internal/llmclient/interaction_collector.go` | Go | JSONL writer with rotation |
| `internal/llmclient/scrubber.go` | Go | Privacy scrubbing |
| `internal/cli/data.go` | Go | CLI commands for data management |
| `neural/training/quality_filter.py` | Python | Quality gate filtering |
| `neural/training/format_converter.py` | Python | Interaction → MLX format |
| `neural/training/dataset_versioner.py` | Python | Dataset assembly + provenance |
| `neural/training/entropy_monitor.py` | Python | Anti-collapse entropy tracking |
| `neural/training/teacher_distill.py` | Python | Anchor dataset generation |
| `neural/training/input_extractor.py` | Python | Extract real inputs from Neo4j |

### Modified Files (18)

| File | Change |
|---|---|
| 15 LLM consumer files | `*Client` → `LLMCompleter` |
| `internal/api/server.go` | Wire interaction loggers |
| `internal/config/config.go` | Data collection config fields |
| `internal/cli/root.go` | Register `data` command |

---

## 12. Implementation Priority

| Priority | Action | When | Effort |
|---|---|---|---|
| **P0 (Today)** | Enable existing collectors (config-only) | Immediately | None |
| **P0 (Today)** | Enable RSIC + Jiminy persistence (config-only) | Immediately | None |
| **P1 (Week 1)** | Build `LLMCompleter` interface + interaction logger | First week of implementation | M |
| **P1 (Week 1)** | Wire all 15 consumers through logger | Concurrent with interface work | M |
| **P2 (Week 2-3)** | Build quality filter + format converter | After 1-2 weeks of collection | S-M |
| **P2 (Week 2-3)** | Build input extractor (Neo4j → task inputs) | Parallel with filter work | M |
| **P3 (Week 3-4)** | Run teacher distillation (generate anchor dataset) | After extractor produces real inputs | M |
| **P3 (Week 3-4)** | Build dataset versioner + manifest system | After anchor dataset exists | S-M |
| **P4 (Week 4+)** | Build entropy monitor + anti-collapse checks | Before first recursive cycle | S |
| **P4 (Week 4+)** | CLI commands + Grafana dashboard | Ongoing | S |

**The single most important thing is P0: flip the config switches today.** Every hour of development generates LLM calls (via Claude Code hooks → Jiminy → RSIC) that are training data for constraint classification, outcome classification, guidance synthesis, and RSIC reflection. If those calls aren't being logged, that data is gone permanently.
