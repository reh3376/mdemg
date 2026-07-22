---
created: 2026-04-02
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "NR-4"
---

# Neural Training Pipeline & LLM Client Unification

## Summary

**Feature**: Neural Training Pipeline
**Summary**: Complete LoRA fine-tuning pipeline for personalizing the neural re-ranker, including data collection, quality filtering, training, evaluation, regression gating, and deployment.


**Phases NR-4 + F21** — Fine-tuning the neural re-ranker from collected data, and consolidating duplicate LLM client code into a shared package.

## Overview

These two features complete the neural re-ranker stack and clean up the LLM integration layer:

**NR-4 (Training Pipeline)** closes the learning loop for MDEMG's neural re-ranker. During normal retrieval, NR-1 passively collects `(query, candidate, LLM_score)` tuples as JSONL files. NR-4 consumes that data to fine-tune a cross-encoder model that learns to replicate LLM-quality relevance scoring at a fraction of the latency and cost. Over time, the re-ranker becomes specialized to the user's domain — the more the system is used, the better it scores.

**F21 (LLM Client Dedup)** extracts the duplicated OpenAI/Ollama HTTP client pattern — which was copy-pasted across 6 packages in 10 production files — into a single shared `internal/llmclient/` package. This eliminates 725 lines of redundant code, makes provider switching a one-line change, and gives every LLM-calling package a consistent, tested interface.

---

## NR-4: Training Pipeline

### Why This Matters

Before NR-4, the neural re-ranker could only use the base pre-trained model (`cross-encoder/ms-marco-MiniLM-L-6-v2`). This model is trained on MS MARCO web search data — it knows what general relevance looks like, but it has no understanding of your specific codebase, domain terminology, or the patterns that make a memory node useful in your context.

NR-1 already collects the signal needed to teach it: every time the LLM re-ranker scores a candidate during retrieval, the tuple `(query, candidate, LLM_score)` is logged to JSONL. These tuples capture what "relevant" means in your specific use case. NR-4 uses them to fine-tune the cross-encoder via MSE regression, so the small local model progressively learns to approximate the LLM's judgment.

The result: retrieval quality converges toward LLM-level scoring, but inference runs in ~5ms on CPU instead of ~500ms per LLM API call. For a typical recall with 20 candidates, that's 10 seconds of LLM re-ranking replaced by 100ms of local inference.

### How It Works

#### Data Flow

```
Normal retrieval (ongoing)
  │
  ▼
NR-1: rerank_collector.go logs (query, candidate, score) → JSONL
  │                                          ▲
  ▼                                          │ scores used to train
NR-4: train.py reads JSONL, fine-tunes ──────┘
  │
  ▼
Model saved to .mdemg/neural/models/v{N}/
  │
  ▼
NR-3: rerank_neural.go loads model via sidecar /rerank
  │
  ▼
Faster, cheaper, domain-specialized re-ranking
```

#### Training Process

1. **JSONL ingestion**: Reads all `*.jsonl` files from the data directory. Each line is:
   ```json
   {"query": "...", "candidate": "...", "score": 0.85, "timestamp": "...", "space_id": "..."}
   ```
   Malformed lines are skipped with a warning. Empty files are handled gracefully.

2. **Minimum sample check**: If fewer than `--min-samples` (default: 100) valid tuples exist, training is skipped. This prevents overfitting on tiny datasets and avoids wasting compute when there isn't enough signal.

3. **Train/validation split**: Data is shuffled and split into training and validation sets (default: 90/10). The split is reproducible within a run but varies across runs (random shuffle).

4. **MSE fine-tuning**: The cross-encoder is fine-tuned using mean squared error loss on the score labels. This is regression training — the model learns to predict the continuous LLM score (0.0–1.0) for any `(query, candidate)` pair. Training uses `sentence_transformers.CrossEncoder.fit()` with configurable epochs and batch size.

5. **Spearman evaluation**: After each epoch, `CECorrelationEvaluator` computes the Spearman rank correlation between the model's predicted scores and the true LLM scores on the validation set. This measures ranking quality — a Spearman of 1.0 means the model preserves the exact same ranking order as the LLM.

6. **Model versioning**: The fine-tuned model is saved to `.mdemg/neural/models/v{N}/` where N auto-increments. A `current` symlink is updated to point to the latest version. Each version directory also contains `training_metadata.json` recording the training parameters, sample count, timestamp, and base model.

7. **Incremental retraining**: Passing `--from-checkpoint /path/to/v{N}` loads a previously fine-tuned model as the starting point instead of the base model. This enables incremental improvement as more data accumulates — each training run builds on the last.

#### Offline Evaluation

`evaluate.py` provides offline comparison between neural scores and LLM scores:

- Loads a model and a JSONL test dataset
- Scores every `(query, candidate)` pair with the model
- Computes:
  - **Spearman rank correlation** — how well neural ranking preserves LLM ranking order
  - **NDCG@k** — normalized discounted cumulative gain for top-k ranking quality
  - **Score distribution stats** — mean, std, min, max, median for both sets
- Optionally compares base model vs fine-tuned model side-by-side
- Outputs a JSON report for tracking improvement over time

### How to Use

#### Prerequisites

- NR-1 data collection must be enabled: `NEURAL_DATA_COLLECTION=true`
- The neural sidecar must be running (for model loading): `mdemg sidecar up`
- Sufficient training data must be accumulated (default minimum: 100 samples)

#### Training

```bash
# Basic training with defaults
mdemg-neural-train

# Custom configuration
mdemg-neural-train \
  --data-dir .mdemg/neural/training-data \
  --model-dir .mdemg/neural/models \
  --epochs 5 \
  --batch-size 32 \
  --val-split 0.15 \
  --min-samples 200

# Incremental training from a previous checkpoint
mdemg-neural-train --from-checkpoint .mdemg/neural/models/v2
```

#### Evaluation

```bash
# Evaluate a fine-tuned model
mdemg-neural-evaluate \
  --model-path .mdemg/neural/models/current \
  --test-data .mdemg/neural/training-data/test.jsonl

# Compare base vs fine-tuned
mdemg-neural-evaluate \
  --model-path .mdemg/neural/models/current \
  --test-data .mdemg/neural/training-data/test.jsonl \
  --base-model cross-encoder/ms-marco-MiniLM-L-6-v2 \
  --output evaluation_report.json
```

#### Typical Workflow

```bash
# 1. Enable data collection (one-time)
export NEURAL_DATA_COLLECTION=true

# 2. Use MDEMG normally — training data accumulates during retrieval

# 3. After sufficient data (100+ samples), train
mdemg-neural-train

# 4. Evaluate the result
mdemg-neural-evaluate \
  --model-path .mdemg/neural/models/current \
  --test-data .mdemg/neural/training-data

# 5. Enable neural re-ranking to use the fine-tuned model
export NEURAL_RERANK_ENABLED=true

# 6. Periodically retrain as more data accumulates
mdemg-neural-train --from-checkpoint .mdemg/neural/models/current
```

### Configuration

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `NEURAL_DATA_COLLECTION` | bool | `false` | Enable training data logging during retrieval |
| `NEURAL_DATA_DIR` | string | `.mdemg/neural/training-data` | Directory for JSONL training data files |
| `NEURAL_RERANK_ENABLED` | bool | `false` | Use neural re-ranker instead of LLM re-ranker |
| `NEURAL_RERANK_URL` | string | `http://localhost:8100` | Sidecar endpoint URL |

The model checkpoint directory is CLI-only: `--model-dir` on `train.py` (default `.mdemg/neural/models`) — there is no `NEURAL_MODEL_DIR` env var.

### File Inventory

| File | Role |
|------|------|
| `neural/neural_sidecar/train.py` | Training pipeline: JSONL ingestion, MSE fine-tuning, model versioning |
| `neural/neural_sidecar/evaluate.py` | Offline evaluation: Spearman, NDCG@k, score distribution, model comparison |
| `neural/tests/test_train.py` | 8 unit tests for training pipeline |
| `neural/tests/test_evaluate.py` | 16 unit tests for evaluation pipeline |
| `neural/pyproject.toml` | Dependencies (scipy) and CLI entrypoints |
| `internal/retrieval/rerank_collector.go` | NR-1: async JSONL data collection (pre-existing) |
| `internal/retrieval/rerank_neural.go` | NR-3: Go HTTP client calling sidecar /rerank (pre-existing) |

---

## F21: LLM Client Unification

### Why This Matters

Before F21, every package that needed to call an LLM maintained its own copy of the same boilerplate:

```
internal/summarize/service.go        — OpenAI + Ollama types, HTTP calls, response parsing
internal/hidden/emergence_namer.go   — same types, same HTTP calls, same parsing
internal/hidden/reclassifier.go      — same again
internal/consulting/synthesis.go     — same again
internal/consulting/llm_classifier.go — same again
internal/metalearn/generalizer.go    — same again
internal/retrieval/intent_translator.go — same again
internal/retrieval/query_classifier.go  — same again
internal/retrieval/rerank.go         — same again
internal/hidden/cluster_summarizer.go — same again
```

This duplication created several problems:
- **Inconsistency**: Each copy drifted slightly. Some had `max_tokens`, others `max_completion_tokens`. Some handled Ollama's `format` field, others didn't.
- **Bug propagation**: A fix in one copy had to be manually applied to all others.
- **Provider changes**: Adding a new LLM provider (or updating the API contract) required touching 10+ files.
- **Testing burden**: Each package had to independently test its own HTTP client code.

### How It Works

F21 extracts the common pattern into `internal/llmclient/`:

#### Shared Types (`types.go`)

All OpenAI and Ollama request/response structures are defined once:

```go
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type OpenAIChatRequest struct {
    Model       string    `json:"model"`
    Messages    []Message `json:"messages"`
    MaxTokens   int       `json:"max_completion_tokens"`
    Temperature *float64  `json:"temperature,omitempty"`
}

// + OpenAIChatResponse, OpenAIChoice, OpenAIUsage, OpenAIError
// + OllamaGenerateRequest (with Format json.RawMessage), OllamaGenerateResponse
```

#### Unified Client (`client.go`)

A single `Client` struct handles both providers:

```go
client := llmclient.New(llmclient.Config{
    Provider:  "openai",                          // OpenAI-compatible protocol (local llama-server) — or "ollama"
    Model:     "mdemg-llm-v1",
    APIKey:    "sk-...",
    BaseURL:   "http://127.0.0.1:8102/v1",
    TimeoutMs: 30000,
})

// Simple completion
text, err := client.Complete(ctx, []llmclient.Message{
    {Role: "system", Content: "You are a classifier."},
    {Role: "user", Content: "Classify this observation."},
})

// Completion with token usage
text, tokens, err := client.CompleteWithUsage(ctx, messages)

// Ollama with structured output
text, err := client.Complete(ctx, messages, llmclient.CompleteOpts{
    Format:  json.RawMessage(`{"type": "object", ...}`),
    Options: map[string]any{"temperature": 0.1},
})
```

Provider routing is automatic:
- `"ollama"` → `POST {baseURL}/api/generate` with `stream: false`
- `"openai"` (or any other value, including empty) → `POST {baseURL}/chat/completions` with `Authorization: Bearer <key>`

#### Utility Functions

Two commonly duplicated helpers are now exported:

```go
// Strip ```json ... ``` wrappers from LLM output
clean := llmclient.StripCodeFence(rawOutput)

// Safe truncation for log messages
short := llmclient.TruncateForLog(longString, 200)
```

#### Migration Pattern

Each migrated package follows the same pattern:

```go
// Before (in each package):
type openAIChatRequest struct { ... }   // local duplicate
type openAIChatResponse struct { ... }  // local duplicate
func (s *Service) callLLM() {
    reqBody, _ := json.Marshal(openAIChatRequest{...})
    req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
    req.Header.Set("Authorization", "Bearer "+apiKey)
    resp, _ := s.httpClient.Do(req)
    var result openAIChatResponse
    json.NewDecoder(resp.Body).Decode(&result)
    return result.Choices[0].Message.Content
}

// After:
func (s *Service) callLLM() {
    text, err := s.llm.Complete(ctx, []llmclient.Message{...})
    return text
}
```

Circuit breaker logic stays in each caller (wrapping `llm.Complete()` inside `cb.Execute()`), since circuit breaker scoping is caller-specific.

### Impact

| Metric | Before | After |
|--------|--------|-------|
| Duplicated type definitions | ~50 across 10 files | 8 in one file |
| HTTP client boilerplate | ~150 lines per package | 0 (delegated to llmclient) |
| Lines of code | +1,947 across 10 files | +1,222 in llmclient = **-725 net** |
| Provider switch effort | Change 10 files | Change 1 file |
| Test coverage for LLM calls | Fragmented, inconsistent | 16 centralized tests |

### File Inventory

| File | Role |
|------|------|
| `internal/llmclient/types.go` | Shared OpenAI + Ollama type definitions |
| `internal/llmclient/client.go` | Unified client: New(), Complete(), CompleteWithUsage(), StripCodeFence(), TruncateForLog() |
| `internal/llmclient/client_test.go` | 16 tests with mock HTTP servers |

**Migrated files (14):**

| Package | Production | Tests |
|---------|-----------|-------|
| `internal/ape` | `llm_reflector.go` | — |
| `internal/consulting` | `llm_classifier.go`, `synthesis.go` | — |
| `internal/hidden` | `emergence_namer.go`, `reclassifier.go`, `cluster_summarizer.go` | `reclassifier_test.go` |
| `internal/metalearn` | `generalizer.go` | `service_test.go` |
| `internal/retrieval` | `intent_translator.go`, `query_classifier.go`, `rerank.go` | `intent_translator_test.go` |
| `internal/summarize` | `service.go` | `service_test.go`, `benchmark_test.go` |

---

## Test Coverage

| Test Suite | Count | What It Covers |
|------------|-------|----------------|
| `neural/tests/test_train.py` | 8 | JSONL loading (valid/empty/malformed), pair creation, model dir creation, versioning + symlinks, min-sample skip, checkpoint resume |
| `neural/tests/test_evaluate.py` | 16 | Stats computation, Spearman correlation (positive/negative/short input), DCG/NDCG (truncation, worst-case, empty), data loading (file/directory/malformed), model evaluation, empty data |
| `internal/llmclient/client_test.go` | 16 | OpenAI + Ollama creation/completion/errors, provider switching, timeout, empty choices, API errors, temperature, format pass-through, usage reporting, StripCodeFence (5 cases), TruncateForLog (4 cases) |
| `go test ./internal/...` | 42 packages | Full Go suite — 0 failures after F21 migration |

---

## Three Training Workstreams (FT Infrastructure Sprint)

MDEMG's fine-tuning infrastructure now supports three distinct training workstreams:

| Workstream | Technique | Model | Data Source | Status |
|---|---|---|---|---|
| **Cross-encoder reranker** (NR-4) | MSE regression | ms-marco-MiniLM-L-6-v2 | Rerank JSONL collector | Built |
| **Generative LoRA** (Phases 2-12) | SFT + GRPO | dense Qwen3-14B (`mdemg-llm-v1`; MoE Qwen3.6-35B-A3B abandoned 2026-04-22 — Metal ceiling) | `llm_interactions` hypertable (16 tasks) | Pipeline complete |
| **Embedding fine-tuning** (Phase D) | Contrastive learning | Domain-tuned 3072-dim model | `embedding_events` + `retrieval_events` hypertables | Collecting data |

**Generative LoRA** trains the generative model on LLM I/O from all 16 tasks. RAFT context enrichment ensures training data includes retrieval context (open-book mode). ULTS specs define quality thresholds for curation. See [RAFT Retrieval Context](raft-retrieval-context.md) and [ULTS Framework](ults-framework.md).

### Generative LoRA Pipeline (Complete)

The full pipeline from data collection to deployment:

| Step | Script | Description |
|------|--------|-------------|
| 1. Collect | `mdemg data export-auto` | Automated daily TSDB export with retention (LaunchAgent) |
| 2. Filter | `quality_filter.py` | 8 quality gates: privacy, empty, error, duplicate, latency, model, prompt hash, ULTS schema |
| 3. Convert | `format_converter.py` | Raw JSONL → HuggingFace MLX chat format with RAFT context + think-mode wrapping |
| 4. Version | `dataset_versioner.py` | Temporal train/test/val splits, dedup, exogenous ratio checks, manifest generation |
| 5. Train | `train_ft.py` | LoRA fine-tuning via mlx-lm-lora with manifest validation + anti-collapse gate |
| 6. Evaluate | `evaluate_ft.py` | Per-task evaluation against held-out test set using ULTS quality_metrics contract |
| 7. Gate | `regression_gate.py` | Deployment decision: no task regresses >5%, >=2 improve, JSON validity >=95% |
| 8. Deploy | `quantize_deploy.py` | Fuse adapter → quantize to 4-bit → verify with test inference |
| 9. Serve | `llama-server` (llama.cpp, :8102) | OpenAI-compatible inference serving `mdemg-llm-v1.Q5_K_M.gguf` (Phase 13.5 cutover; vllm-mlx decommissioned) |

**Supplementary tools:**
- `teacher_distill.py` — synthetic data generation for under-represented tasks using teacher LLM
- `reward_functions.py` — 21 GRPO reward functions for post-SFT reinforcement learning
- `test_vllm_mlx.py` — historical smoke test of the 16 ULTS tasks (vllm-mlx era; production serving is llama-server :8102 since Phase 13.5)

**Multi-paradigm curation** (UAITS framework, added 2026-04-10):
- `paradigm_router.py` — spec-driven curation across SFT, DPO, RAFT, and curriculum paradigms
- `dpo_builder.py` — DPO preference pair construction from `constraint_outcomes` + `llm_interactions`
- `mdemg data curate` / `mdemg data validate` — CLI commands for curation and spec validation
- See [UAITS Framework](uaits-framework.md) for full details

**Embedding fine-tuning** uses contrastive learning on domain-specific text pairs. The hard-negative mining signal (high vector similarity + low rerank score) is captured in `retrieval_events`. See [Embedding & Retrieval Data Collection](embedding-retrieval-data-collection.md).

## Documents Accessed

- `internal/llmclient/types.go`, `client.go`, `client_test.go`
- `neural/neural_sidecar/train.py`, `evaluate.py`
- `neural/tests/test_train.py`, `test_evaluate.py`
- `neural/pyproject.toml`
- `internal/retrieval/rerank_collector.go`, `rerank_neural.go`
- `internal/hidden/emergence_namer.go`, `reclassifier.go`, `cluster_summarizer.go`
- `internal/consulting/synthesis.go`, `llm_classifier.go`
- `internal/metalearn/generalizer.go`
- `internal/ape/llm_reflector.go`
- `internal/retrieval/intent_translator.go`, `query_classifier.go`, `rerank.go`
- `internal/summarize/service.go`
- `internal/config/config.go`
- `AGENT_HANDOFF.md`, `CHANGELOG.md`
- `docs/features/rsic-feedback-loop.md` (style reference)
