# Research: Fine-Tuning an Open-Source LLM for MDEMG Recursive Self-Improvement

**Date:** 2026-03-27 (v2.0 — corrected from deep-dive analysis)
**Hardware:** Apple M5 Max — 128GB unified memory, 614 GB/s bandwidth
**Model:** Qwen3-30B-A3B MoE (Apache 2.0, 30B total / 3B active, /think + /no_think)
**Goal:** Replace all external LLM calls with a single fine-tuned model that recursively improves itself

---

## 1. What MDEMG Currently Uses LLMs For

### 1.1 Generative Tasks (15 Call Sites)

MDEMG makes **15** distinct generative LLM calls (not 11 as originally assessed — codebase audit found 4 additional consumers). All are routed through `llmclient.Client` configured via `LLM_PROVIDER` / `LLM_MODEL`.

| # | File | Task | Input | Output | Latency Req | JSON Parse? |
|---|---|---|---|---|---|---|
| 1 | `ape/llm_reflector.go` | RSIC reflection | Health report + stats | Structured JSON insights | Background | ✅ |
| 2 | `consulting/llm_classifier.go` | Constraint classification | Node content | JSON: is_constraint + confidence | Inline (<2s) | ✅ |
| 3 | `consulting/synthesis.go` | Memory synthesis | Memories[] + query | Natural language narrative | Inline (<3s) | ❌ |
| 4 | `hidden/cluster_summarizer.go` | Cluster summarization | Cluster members | Concept name + summary text | Background | ❌ |
| 5 | `hidden/emergence_namer.go` | Emergence naming | Layer members + edges | JSON: name + type | Background | ✅ |
| 6 | `hidden/reclassifier.go` | Node reclassification | Node content + context | JSON: type + confidence | Background | ✅ |
| 7 | `jiminy/evaluator.go` | J9 evaluation | Agent output + constraints | JSON: violations + warnings | Near-RT (<5s) | ✅ |
| 8 | `jiminy/outcome_classifier.go` | Outcome classification | Guidance + action | JSON: followed/ignored/contradicted | Near-RT (<3s) | ✅ |
| 9 | `jiminy/synthesizer.go` | Guidance synthesis | Items[] + context | Prompt augmentation text | Near-RT (<3s) | ❌ |
| 10 | `jiminy/codegen.go` | J17 code generation | Constraint type + desc | Kebab-case code string | Background | ❌ |
| 11 | `metalearn/generalizer.go` | Cross-space generalization | Concepts from spaces | JSON: generalized concept | Background | ✅ |
| 12 | `retrieval/intent_translator.go` | Query rewriting | User query | Rewritten query text | Inline (<2s) | ❌ |
| 13 | `retrieval/query_classifier.go` | Query type classification | User query | JSON: type + confidence | Inline (<2s) | ✅ |
| 14 | `retrieval/rerank.go` | LLM-based reranking | Query + candidates | JSON: scored rankings | Inline (<3s) | ✅ |
| 15 | `summarize/service.go` | Code element summarization | Code element struct | Summary text / JSON | Background | ✅ (partial) |

**Critical finding:** 9 of 15 consumers call `json.Unmarshal` on the raw LLM response. Qwen3's think mode produces `<think>...</think>\n{json}` which breaks all JSON parsers. A `SanitizeResponse()` function is required (see Implementation Plan Phase 2F).

### 1.2 Cross-Encoder Tasks (3 Models in Neural Sidecar)

| # | Model | Params | Task | Fine-Tuned? |
|---|---|---|---|---|
| 16 | `cross-encoder/ms-marco-MiniLM-L-6-v2` | ~22M | Re-rank retrieval candidates | Yes (via `train.py`) |
| 17 | `cross-encoder/nli-deberta-v3-xsmall` | ~22M | NLI comprehension scoring | No |
| 18 | Configurable | ~22M | J17 tier prediction | Yes (via `train_protocol.py`) |

### 1.3 Existing Training Data Collection

| Collector | Status | Default |
|---|---|---|
| Rerank JSONL | Built, working | **OFF** (`NEURAL_DATA_COLLECTION=false`) |
| Protocol JSONL | Built, working | **OFF** (`J17_PROTOCOL_DATA_COLLECTION=false`) |
| LLM Interaction Logger | **NOT BUILT** | — |

**Neither existing collector captures generative LLM inputs/outputs.** The interaction logger (Phase 1 of implementation plan) is the critical path for training data.

---

## 2. The Recursive Self-Improvement Loop

### 2.1 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    ITERATION N                               │
│                                                              │
│  1. MDEMG operates with fine-tuned model v(N)                │
│     - All 15 tasks served by single MoE model               │
│     - Interaction logger captures all I/O to JSONL           │
│                                                              │
│  2. RSIC assesses quality using model v(N)                   │
│     - Health scores, comprehension, effectiveness            │
│     - Entropy monitor checks for model collapse              │
│                                                              │
│  3. Training pipeline processes accumulated data             │
│     - Quality filter → format converter → dataset versioner  │
│     - Anti-collapse: exogenous ratio α ≥ 0.4                │
│     - Temporal split: test data from AFTER training data     │
│                                                              │
│  4. Training stages:                                         │
│     a. SFT (LoRA on accumulated data + anchor)              │
│     b. GRPO (reward functions from MDEMG quality signals)    │
│     c. DPO (automated preference pairs)                     │
│     d. HITL (human review for subjective quality)            │
│                                                              │
│  5. Benchmark gate: v(N+1) vs v(N) on held-out test set     │
│     - All 15 tasks evaluated                                 │
│     - Regression: keep v(N)                                  │
│     - Improvement: deploy v(N+1) via vllm-mlx               │
│                                                              │
│  6. GOTO step 1 as iteration N+1                             │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Anti-Collapse Protocol

Peer-reviewed research (Nature 2024, ICLR RSI Workshop 2026) proves that recursive self-training causes model collapse when exogenous signal approaches zero. MDEMG's protocol:

1. **Minimum exogenous ratio (α ≥ 0.4):** Every training batch must contain ≥40% data NOT generated by the fine-tuned model (teacher-distilled, human-annotated, deterministically verified, or synthetic)
2. **Entropy monitoring:** Track output entropy across versions. If entropy decreases >10%, halt the loop and inject fresh exogenous data
3. **Fresh exogenous injection every 3 cycles:** Re-run teacher distillation from the latest external LLM
4. **Diversity sampling in GRPO:** Temperature ≥ 0.8, top_p = 0.95

### 2.3 Why Each Iteration Finds New Failures

Each version's fixes enable new data pipelines that reveal previously invisible patterns:

```
v1: Trained on schema docs → discovers role_type vs node_type bug
v2: Trained on v1 + fixed data → discovers NLI fallback cascade
    (comprehension data now flows, showing anomalous patterns)
v3: Trained on v2 + cascade fix → discovers trust burst issue
    (feedback data now flows, showing trust progression anomalies)
```

---

## 3. Model Selection: Qwen3-30B-A3B MoE

### 3.1 Why MoE over Dense (Corrected from v1.0)

Real-world benchmarks revealed Qwen3-32B dense runs at **10-22 tok/s** on Apple Silicon — not the ~24 tok/s originally estimated. The MoE model runs at **64-88 tok/s** on the same hardware with identical quality.

| Factor | Qwen3-32B Dense | Qwen3-30B-A3B MoE |
|---|---|---|
| License | Apache 2.0 | **Apache 2.0** |
| Quality (MMLU-Pro CS) | 82.20% | 82.20% |
| Generation speed (M5 Max Q4) | 15-22 tok/s | **64-88 tok/s** |
| Speed ratio | 1x | **4-5x faster** |
| Context window | 128K | 131K |
| Think mode | ✅ | ✅ |
| Memory for inference (Q4) | ~18GB | ~20GB |
| Training bf16 LoRA | ~66GB | ~74GB (fits 128GB) |
| MoE fine-tuning on MLX | N/A | ✅ (mlx-lm-lora, 12x faster MoE kernels) |

The Qwen3.5-35B-A3B is the newer successor with 262K context and native vision, but uses the Qwen License (not Apache 2.0). For an Apache 2.0 public project, Qwen3-30B-A3B is the correct choice. The pipeline supports upgrading to Qwen3.5 later.

### 3.2 Training Data Estimate

**15 tasks × 500-1000 examples each = 7,500-15,000 total anchor examples**, plus ongoing production data collection, synthetic failure examples, and HITL preference pairs.

---

## 4. Infrastructure

### 4.1 Production Architecture (Corrected from v1.0)

```
┌─────────────────────────────────────────────────────────────────┐
│                       M5 Max (128GB)                             │
│                                                                  │
│  ┌─────────────┐   ┌──────────────────┐  ┌───────────────────┐ │
│  │ MDEMG Server │   │   vllm-mlx       │  │ Neural Sidecar    │ │
│  │ (Go, :9999)  │   │   (:8100)        │  │ (:8101)           │ │
│  │              │   │                  │  │                   │ │
│  │  llmclient ──┼──→│ OpenAI-compat   │  │ POST /rerank      │ │
│  │  (provider:  │   │ Prefix caching   │  │ POST /nli         │ │
│  │   "openai"   │   │ Continuous batch │  │ POST /tier        │ │
│  │   base_url:  │   │ Reasoning parser │  │                   │ │
│  │   :8100/v1)  │   │                  │  └───────────────────┘ │
│  │              │   │ Qwen3-30B-A3B    │                        │
│  └─────────────┘   │ (MoE, ~20GB Q4)  │  ┌───────────────────┐ │
│                     │ ~80 tok/s         │  │ Training Pipeline │ │
│                     └──────────────────┘  │ (offline, MLX)    │ │
│                                           └───────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Key change from v1.0:** The custom `generator.py` in the neural sidecar is replaced by **vllm-mlx**, which provides an OpenAI-compatible API with prefix caching (5.8x TTFT speedup on shared system prompts), continuous batching, and native Qwen3 reasoning parser. Since it's OpenAI-compatible, `llmclient`'s existing OpenAI provider works directly — no new "mlx" provider is needed.

### 4.2 Memory Budget

| Service | Memory |
|---|---|
| macOS + desktop | ~8GB |
| Neo4j Docker | ~6GB |
| MDEMG Go server | ~1GB |
| Neural sidecar (cross-encoders) | ~2GB |
| Claude Code | ~2GB |
| vllm-mlx + Qwen3-30B-A3B Q4 | ~22GB |
| **Total used** | **~41GB** |
| **Available for KV cache + context** | **~87GB** |
| **Training (bf16 LoRA, offline)** | ~74GB (128GB available when inference stopped) |
