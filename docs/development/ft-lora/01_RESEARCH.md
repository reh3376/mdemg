# Research: Fine-Tuning an Open-Source LLM for MDEMG Recursive Self-Improvement

**Date:** 2026-03-30 (v3.0 — aligned to codebase PRs #210-#219 + deep-dive analysis)
**Hardware:** Apple M5 Max — 128GB unified memory, 614 GB/s bandwidth
**Model:** Qwen3-30B-A3B MoE (Apache 2.0, 30B total / 3B active, /think + /no_think)
**Goal:** Replace all external LLM calls with a single fine-tuned model that recursively improves itself

---

## 1. What MDEMG Currently Uses LLMs For

### 1.1 Generative Tasks (16 Call Sites)

MDEMG makes **16** distinct generative LLM calls. All are routed through `llmclient.Client` configured via `LLM_PROVIDER` / `LLM_MODEL`. Each consumer is labeled with a `WithContext(taskName, spaceID)` call for interaction logging.

| # | File | Task Label | Input | Output | Latency Req | JSON Parse? |
|---|---|---|---|---|---|---|
| 1 | `ape/llm_reflector.go` | `ape.reflect` | Health report + stats | Structured JSON insights | Background | ✅ |
| 2 | `consulting/llm_classifier.go` | `consulting.classify` | Node content | JSON: is_constraint + confidence | Inline (<2s) | ✅ |
| 3 | `consulting/synthesis.go` | `consulting.synthesis` | Memories[] + query | Natural language narrative | Inline (<3s) | ❌ |
| 4 | `hidden/cluster_summarizer.go` | `hidden.summarize` | Cluster members | Concept name + summary text | Background | ❌ |
| 5 | `hidden/emergence_namer.go` | `hidden.name_emergence` | Layer members + edges | JSON: name + type | Background | ✅ |
| 6 | `hidden/reclassifier.go` | `hidden.reclassify` | Node content + context | JSON: type + confidence | Background | ✅ |
| 7 | `jiminy/evaluator.go` | `jiminy.evaluate` | Agent output + constraints | JSON: violations + warnings | Near-RT (<5s) | ✅ |
| 8 | `jiminy/evaluator.go` | `jiminy.evaluate_llm` | Agent output + LLM revalidation | JSON: revalidated items | Near-RT (<5s) | ✅ |
| 9 | `jiminy/outcome_classifier.go` | `jiminy.evaluate` (outcome) | Guidance + action | JSON: followed/ignored/contradicted | Near-RT (<3s) | ✅ |
| 10 | `jiminy/synthesizer.go` | `jiminy.synthesize` | Items[] + context | Prompt augmentation text | Near-RT (<3s) | ❌ |
| 11 | `jiminy/codegen.go` | `jiminy.codegen` | Constraint type + desc | Kebab-case code string | Background | ❌ |
| 12 | `metalearn/generalizer.go` | `metalearn.generalize` | Concepts from spaces | JSON: generalized concept | Background | ✅ |
| 13 | `retrieval/intent_translator.go` | `retrieval.intent_translate` | User query | Rewritten query text | Inline (<2s) | ❌ |
| 14 | `retrieval/query_classifier.go` | `retrieval.query_classify` | User query | JSON: type + confidence | Inline (<2s) | ✅ |
| 15 | `retrieval/rerank.go` | `retrieval.rerank_cross` | Query + candidates | JSON: scored rankings | Inline (<3s) | ✅ |
| 16 | `retrieval/rerank.go` | `retrieval.rerank_nli` | Query + candidates | JSON: NLI-based rankings | Inline (<3s) | ❌ |
| 17 | `summarize/service.go` | `summarize.generate` | Code element struct | Summary text / JSON | Background | ✅ (partial) |

Note: 17 rows because `jiminy.evaluate` appears in both evaluator.go (constraint checking) and outcome_classifier.go (feedback classification). The model serves **16 distinct task labels**.

**Critical finding:** 9 of 16 consumers call `json.Unmarshal` on the raw LLM response. Qwen3's think mode produces `<think>...</think>\n{json}` which breaks all JSON parsers. A `SanitizeResponse()` function is required (see Implementation Plan Phase 2D).

### 1.2 Cross-Encoder Tasks (3 Models in Neural Sidecar)

| # | Model | Params | Task | Fine-Tuned? |
|---|---|---|---|---|
| 18 | `cross-encoder/ms-marco-MiniLM-L-6-v2` | ~22M | Re-rank retrieval candidates | Yes (via `train.py`) |
| 19 | `cross-encoder/nli-deberta-v3-xsmall` | ~22M | NLI comprehension scoring | No |
| 20 | Configurable | ~22M | J17 tier prediction | Yes (via `train_protocol.py`) |

### 1.3 Training Data Collection (Current State)

| Collector | Status | Default | Storage |
|---|---|---|---|
| **LLM Interaction Logger** | ✅ BUILT (PR #217/#218) | **ON** | TimescaleDB `llm_interactions` table |
| **Privacy Scrubber** | ✅ BUILT (PR #219) | Wired into writer | At write time (5 regex patterns) |
| **Guidance ID Correlation** | ✅ BUILT (PR #219) | Via context.WithValue | `guidance_id` column |
| **Source Path Linkage** | ✅ BUILT (PR #219) | Via context.WithValue | `source_path` column |
| **Think Content Extraction** | ✅ BUILT (PR #219) | In recordInteraction | `think_content` column |
| **Quality Annotation** | ✅ BUILT (PR #219) | Manual trigger | `quality_annotator.py` |
| **Data CLI** | ✅ BUILT (PR #219) | `mdemg data` | status/inspect/stats/annotate/quality |
| Rerank JSONL | Built, working | **OFF** | `.mdemg/neural/training-data/*.jsonl` |
| Protocol JSONL | Built, working | **OFF** | `.mdemg/neural/training-data/*.jsonl` |

All 16 generative LLM call sites are logged to TimescaleDB with task labels, guidance_id correlation, source document linkage, privacy scrubbing, and think content extraction.

---

## 2. The Recursive Self-Improvement Loop

### 2.1 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    ITERATION N                               │
│                                                              │
│  1. MDEMG operates with fine-tuned model v(N)                │
│     - All 16 tasks served by single MoE model               │
│     - Interaction logger captures all I/O to TimescaleDB     │
│                                                              │
│  2. RSIC assesses quality using model v(N)                   │
│     - Health scores, comprehension, effectiveness            │
│     - Entropy monitor checks for model collapse              │
│                                                              │
│  3. Training pipeline processes accumulated data             │
│     - Quality filter → format converter → dataset versioner  │
│     - Anti-collapse: exogenous ratio α ≥ 0.4                │
│     - Temporal split: test data from AFTER training data     │
│     - RAFT enrichment: include retrieval context             │
│                                                              │
│  4. Training stages:                                         │
│     a. SFT (LoRA on accumulated data + anchor)              │
│     b. GRPO (reward functions from MDEMG quality signals)    │
│     c. DPO (automated preference pairs)                     │
│     d. HITL (human review for subjective quality)            │
│                                                              │
│  5. Benchmark gate: v(N+1) vs v(N) on held-out test set     │
│     - All 16 tasks evaluated via ULTS specs                  │
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

### 2.4 RAFT: Training for the Open-Book Setting (v3.0 Addition)

MDEMG operates in an "open-book" setting — every LLM call receives retrieved context from the Neo4j knowledge graph. The constraint classifier gets candidate nodes with relevance scores. The guidance synthesizer receives constraint items retrieved by embedding search. The evaluator gets active constraints matched against the agent's output.

UC Berkeley's RAFT research (Retrieval Augmented Fine-Tuning, COLM 2024) proves that training a model to work in this setting — where it must distinguish relevant retrieved information from distractors — significantly outperforms both pure RAG and pure fine-tuning.

MDEMG's RAFT implementation:

- **Training data includes retrieval context:** retrieved node IDs, relevance scores, and which node was the "oracle" (relevant) vs. distractor
- **80/20 split:** 80% of training examples include retrieved context in the prompt; 20% omit it (forcing the model to fall back on internalized knowledge)
- **Natural fit:** MDEMG's architecture is already hybrid RAG + fine-tuning. The graph holds facts (constraints, observations, patterns). The fine-tuned model learns behavior (how to classify, synthesize, evaluate, and reflect on those facts)
- **2026 consensus:** "RAG for facts, fine-tuning for behavior" — MDEMG embodies this pattern

This is the single most important training quality improvement. Without retrieval context, the model trains in closed-book mode but operates in open-book mode. With it, the model trains in the same mode it operates in.

### 2.5 Design for Routine Retraining (v3.0 Addition)

MDEMG evolves rapidly (~4-5 PRs/day). Retraining is routine maintenance, not a special project.

**Retraining triggers:**
- System prompts change (new output fields, refined formats)
- New tasks are added
- The knowledge domain shifts (new codebases, new teams)
- Model performance degrades (benchmark regression or entropy decay)
- Better base models become available (Qwen3.5, Qwen4)

**Expected frequency:**
- Monthly SFT refreshes (incorporate new production data)
- Quarterly GRPO cycles (after sufficient quality-annotated data accumulates)
- Ad-hoc retraining when system prompts change significantly
- Base model upgrades ~1-2x/year

**Design implications:**
- System prompt hash on every training record (enables data versioning)
- Automated pipeline with regression gates (can run unattended)
- Graceful fallback to external LLM when training cycle produces regression
- ULTS specs version alongside prompts (single source of truth for contracts)

---

## 3. Model Selection: Qwen3-30B-A3B MoE

### 3.1 Why MoE over Dense

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

**16 tasks × 500-1000 examples each = 8,000-16,000 total anchor examples**, plus ongoing production data collection, synthetic failure examples, and HITL preference pairs.

At current development velocity (~50-100 LLM interactions/day), reaching 500 samples per task takes approximately 2-3 months. Low-frequency tasks (metalearn.generalize, hidden.reclassify) will require teacher distillation to supplement.

---

## 4. Infrastructure

### 4.1 Production Architecture

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
│  │  TSDB ───────┼──→│ (MoE, ~20GB Q4)  │  ┌───────────────────┐ │
│  │  (metrics +  │   │ ~80 tok/s         │  │ Training Pipeline │ │
│  │   training   │   └──────────────────┘  │ (offline, MLX)    │ │
│  │   data)      │                         └───────────────────┘ │
│  └─────────────┘                                                │
└─────────────────────────────────────────────────────────────────┘
```

**Key design:** The custom `generator.py` from v1.0 is replaced by **vllm-mlx**, which provides an OpenAI-compatible API with prefix caching (5.8x TTFT speedup on shared system prompts), continuous batching, and native Qwen3 reasoning parser. Since it's OpenAI-compatible, `llmclient`'s existing OpenAI provider works directly — no new provider code needed.

### 4.2 Memory Budget

| Service | Memory |
|---|---|
| macOS + desktop | ~8GB |
| Neo4j Docker | ~6GB |
| TimescaleDB Docker | ~1-2GB |
| MDEMG Go server | ~1GB |
| Neural sidecar (cross-encoders) | ~2GB |
| Claude Code | ~2GB |
| vllm-mlx + Qwen3-30B-A3B Q4 | ~22GB |
| **Total used** | **~42-43GB** |
| **Available for KV cache + context** | **~85-86GB** |
| **Training (bf16 LoRA, offline)** | ~74GB (128GB available when inference stopped) |
