# Research: Fine-Tuning an Open-Source LLM for MDEMG Recursive Self-Improvement

**Date:** 2026-04-21 (v5.0 — Qwen3.6-35B-A3B upgrade + two-tier MoE LoRA + no-tool-calling policy per memo 07 v3.1)
**Hardware:** Apple M5 Max — 128GB unified memory, 614 GB/s bandwidth
**Model:** Qwen3.6-35B-A3B MoE (Apache 2.0, released 2026-04-16, 35B total / 3B active per token, 256 experts = 8 routed + 1 shared, Hybrid Gated DeltaNet + Gated Attention + MoE, MTP speculative decoding, 262K native context, /think + /no_think)
**Fallback:** Qwen3.5-35B-A3B (Apache 2.0, mature) — NOT Qwen3-30B-A3B (superseded by this version)
**Goal:** Replace all external LLM calls with a single fine-tuned model that recursively improves itself

---

## Changes in v5.0 (per memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1 — 2026-04-21)

Three locked-in decisions supersede the v4.0 plan:

1. **Base model upgrade**: Qwen3-30B-A3B → **Qwen3.6-35B-A3B** (§3). Fallback Qwen3.5-35B-A3B, **not** Qwen3-30B-A3B.
2. **No-tool-calling architectural policy** (§2.8) — all 16 MDEMG LLM call sites are single-shot structured-output / reasoning. Previously implicit; now explicit.
3. **Two-tier MoE-Sieve LoRA strategy** (§5) — Tier 1 (attention + shared expert, r=32 α=64, all 16 tasks) + Tier 2 (top-25% routed experts, r=8 α=16, per-family). Replaces any monolithic single-LoRA approach.

§1.1 was re-audited against the current codebase (2026-04-21): **16 distinct llmclient call sites confirmed**, with file-location drift corrected for `jiminy.evaluate_llm` and `jiminy.codegen` (both now live in `internal/api/server.go`).

---

## 1. What MDEMG Currently Uses LLMs For

### 1.1 Generative Tasks (16 Call Sites)

MDEMG makes **16** distinct generative LLM calls. All are routed through `llmclient.Client` configured via `LLM_PROVIDER` / `LLM_MODEL`. Each consumer is labeled with a `WithContext(taskName, spaceID)` call for interaction logging.

Task-type labels (used by the §5 family partition): **T** = think-mode reasoning, **C** = no-think classify (short structured decision), **J** = no-think JSON (longer structured output). See §5 "MoE Two-Tier LoRA Strategy" for how labels drive the Tier 2 family partition, and memo §3.3 for the corresponding sampling recipes.

| # | File | Task Label | Input | Output | Latency Req | JSON Parse? | Group |
|---|---|---|---|---|---|---|---|
| 1 | `ape/llm_reflector.go` | `ape.reflect` | Health report + stats | Structured JSON insights | Background | ✅ | **T** |
| 2 | `consulting/llm_classifier.go` | `consulting.classify` | Node content | JSON: is_constraint + confidence | Inline (<2s) | ✅ | **C** |
| 3 | `consulting/synthesis.go` | `consulting.synthesis` | Memories[] + query | Natural language narrative | Inline (<3s) | ❌ | **T** |
| 4 | `hidden/cluster_summarizer.go` | `hidden.summarize` | Cluster members | Concept name + summary text | Background | ❌ | **T** |
| 5 | `hidden/emergence_namer.go` | `hidden.name_emergence` | Layer members + edges | JSON: name + type | Background | ✅ | **J** |
| 6 | `hidden/reclassifier.go` | `hidden.reclassify` | Node content + context | JSON: type + confidence | Background | ✅ | **C** |
| 7 | `api/server.go` (wraps `jiminy/evaluator.go` logic) | `jiminy.evaluate_llm` | Agent output + LLM revalidation | JSON: revalidated items | Near-RT (<5s) | ✅ | **J** |
| 8 | `jiminy/outcome_classifier.go` | `jiminy.evaluate` | Guidance + action | JSON: followed/ignored/contradicted | Near-RT (<3s) | ✅ | **C** |
| 9 | `jiminy/synthesizer.go` | `jiminy.synthesize` | Items[] + context | Prompt augmentation text | Near-RT (<3s) | ❌ | **T** |
| 10 | `api/server.go` (wraps `jiminy/codegen.go` logic) | `jiminy.codegen` | Constraint type + desc | Kebab-case code string | Background | ❌ | **C** |
| 11 | `metalearn/generalizer.go` | `metalearn.generalize` | Concepts from spaces | JSON: generalized concept | Background | ✅ | **T** |
| 12 | `retrieval/intent_translator.go` | `retrieval.intent_translate` | User query | Rewritten query text | Inline (<2s) | ❌ | **C** |
| 13 | `retrieval/query_classifier.go` | `retrieval.query_classify` | User query | JSON: type + confidence | Inline (<2s) | ✅ | **C** |
| 14 | `retrieval/rerank.go` | `retrieval.rerank_cross` | Query + candidates | JSON: scored rankings | Inline (<3s) | ✅ | **J** |
| 15 | `retrieval/rerank.go` | `retrieval.rerank_nli` | Query + candidates | JSON: NLI-based rankings | Inline (<3s) | ❌ | **T** |
| 16 | `summarize/service.go` | `summarize.generate` | Code element struct | Summary text / JSON | Background | ✅ (partial) | **T** |

**Group totals (inputs to §5 family partition, provisional):**
- **T** (think-mode reasoning): 7 — `ape.reflect`, `consulting.synthesis`, `hidden.summarize`, `jiminy.synthesize`, `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate`
- **C** (no-think classify, short structured): 6 — `consulting.classify`, `hidden.reclassify`, `jiminy.evaluate`, `jiminy.codegen`, `retrieval.intent_translate`, `retrieval.query_classify` (note: `jiminy.codegen` outputs a short kebab-case code string — structurally "classify"-shaped)
- **J** (no-think JSON, longer structured): 3 — `hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`

(Totals: 7 + 6 + 3 = 16 ✓. The T/C/J split is coarse; a call site's group membership is a hypothesis validated by Sprint D expert activation profiling — see §5.)

**Re-audit note (Sprint FT-LORA-A, 2026-04-21):** Verified each label via `grep "WithContext(\"<task>\"" internal/ --include='*.go'`. Drift from v4.0 corrected:
- Row 7 in the v4.0 table (`jiminy/evaluator.go` → `jiminy.evaluate`) removed: `evaluator.go` still has a `CallSite: "jiminy.evaluate"` string for the **embedding recorder**, but its llmclient generative call now lives in `outcome_classifier.go` (same task label). The "17 rows because jiminy.evaluate appears in both" caveat in v4.0 no longer holds.
- `jiminy.evaluate_llm` llmclient call moved from `jiminy/evaluator.go` → `api/server.go:569`.
- `jiminy.codegen` llmclient call moved from `jiminy/codegen.go` → `api/server.go:538`.
- Row count is now **16 rows = 16 distinct task labels** (no double-counting).

**Critical finding:** 9 of 16 consumers call `json.Unmarshal` on the raw LLM response. Qwen3's think mode produces `<think>...</think>\n{json}` which breaks all JSON parsers. A `SanitizeResponse()` function is required (see Implementation Plan Phase 2D).

**Note: Guardrail LLM consumer — cutover blocker for local-model switch.** The guardrail service (`internal/guardrail/llm_evaluator.go`) makes direct HTTP calls to OpenAI/Ollama — it does NOT route through `llmclient`. This means guardrail LLM calls:

1. **Bypass the interaction logger** — guardrail calls are not captured in `llm_interactions` and are therefore **absent from every fine-tuning dataset**. The 16-row table above is the complete training-data universe; guardrail is a 17th call site outside it.
2. **Bypass every `llmclient`-level policy** — per-task timeouts, retry rules, circuit breakers, no-tool-calling enforcement (§2.8 grep patterns), and the future base_url/model swap to vllm-mlx all do **not** apply to guardrail today.
3. **Will not automatically switch to the fine-tuned Qwen3.6-35B-A3B** when §3 is executed. `GUARDRAIL_ENABLED=false` is the default, but any environment that enables guardrail will continue calling external OpenAI/Ollama after cutover unless migrated.

**Action required (queued for Sprint FT-LORA-B, code-migration epic):** migrate `internal/guardrail/llm_evaluator.go` to `llmclient.Client` with `WithContext("guardrail.evaluate", spaceID)`. This adds guardrail as the 17th call site in this table (projected Group: **C**, short structured decision) and brings it under the no-tool-calling policy, the interaction logger, and the cutover path. Until this migration lands, document any deployment that sets `GUARDRAIL_ENABLED=true` as **knowingly running a non-local model for guardrail** even after the local-model switch.

**Note: Per-task model overrides.** While `LLM_MODEL=gpt-4.1-nano` is the default, several tasks override to specific models: `gpt-4o-mini` (rerank, summary, synthesis, intent, emergence, guardrail), `gpt-4.1-nano` (reclassification). When switching to the fine-tuned Qwen3.6-35B-A3B, all overrides collapse to a single model — this is a key simplification the fine-tuning enables.

### Architectural Constraints — See §2.8

The "no tool-use" architectural constraint was previously documented inline here. It is now the canonical **§2.8 No-Tool-Calling Architectural Policy**, with full justification and per-task classification. The table above (§1.1) is the authoritative per-site catalog; §2.8 is the policy statement.

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

### 1.4 Embedding Model (Separate Workstream)

MDEMG's embedding model is architecturally independent from the generative LLM. The embedding pipeline (`internal/embeddings/`) produces **3072-dimension vectors** used for vector search in Neo4j:

| Provider | Model | Native Dims | Output Dims | Method |
|---|---|---|---|---|
| OpenAI | `text-embedding-3-large` | 3072 | 3072 | Native |
| Ollama | `qwen3-embedding:8b` | 4096 | 3072 | MRL truncation |

The Neo4j vector index is hardcoded to 3072 dimensions (`vectorIndexDimensions = 3072`). **Any future fine-tuned embedding model must produce 3072-dimension vectors** — changing dimensions would require re-embedding all 34K+ nodes.

Embedding fine-tuning uses **contrastive learning** (not SFT/GRPO) — a fundamentally different technique from generative LoRA. It is a separate future workstream with its own data collection pipeline:

| Data Source | What It Provides | Training Value |
|---|---|---|
| `embedding_events` (TSDB, planned) | Every Embed() call with parser metadata | What text gets embedded, chunking context |
| `retrieval_events` (TSDB, planned) | Full retrieval pipeline: query → recall → rerank → result | Contrastive pairs (positive + hard negatives) |
| Rerank JSONL (existing) | Cross-encoder scores | Ground truth relevance labels |

The gap between vector recall cosine similarity and cross-encoder reranking scores is the hard-negative mining signal — the most valuable training data for embedding fine-tuning. MDEMG's 27 language parsers produce AST-aware chunks with rich metadata (element kind, language, file path, chunk boundaries, signatures) that a domain-fine-tuned embedding model could leverage for better code retrieval.

### 1.4 Embedding Model (Separate Workstream)

MDEMG's embedding pipeline (`internal/embeddings/`) is architecturally separate from the generative LLM pipeline (`internal/llmclient/`). Embeddings use dedicated encoder models — `text-embedding-3-large` (OpenAI) or `qwen3-embedding:8b` (Ollama) — not the generative Qwen3.6-35B-A3B.

**Embedding fine-tuning is NOT part of the generative LoRA plan.** It uses a fundamentally different technique (contrastive learning, not SFT/GRPO), different models (encoder, not decoder), and different training data (retrieval events, not LLM I/O).

However, data collection for future embedding fine-tuning runs in parallel:

| Collector | Status | Training Signal |
|---|---|---|
| **Retrieval Event Logger** | Planned | (query, results, vector_sims, rerank_scores) |
| **Rerank JSONL Collector** | Built (OFF) | Cross-encoder relevance scores → implicit contrastive labels |
| **Chunk Provenance** | Planned | Parser name, language, chunk type on each embedded node |
| **Retrieval-to-Guidance Linkage** | Planned | Which retrieved nodes led to followed/ignored guidance |

The rerank cross-encoder scores provide the primary training signal: nodes with high vector similarity but low rerank scores are **hard negatives** — exactly what contrastive learning needs to improve the embedding space. Downstream guidance follow/ignore outcomes provide **gold-standard labels** for whether retrieval was actually useful.

Parser provenance (which of the 27 language parsers produced each chunk, what chunk type — function/class/import/observation) enables per-domain embedding quality analysis and chunk-aware training.

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
- Better base models become available (Qwen3.7, Qwen4)

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

### 2.6 Jiminy Guidance Outcomes as Training Quality Signal

The Jiminy effectiveness investigation (v0.7.1) established a new quality signal pipeline:

- **GUIDANCE_OUTCOME edges** in Neo4j record per-item outcomes (followed, partial_compliance, ignored, not_applicable) with similarity scores
- **guidance_id correlation** in `llm_interactions` links LLM calls to their downstream outcomes
- **Content normalization** transforms structured metadata into natural language for accurate embedding comparison
- **Trust-based tier data** records which encoding tier (T1/T2/T3) was used and whether the agent followed

This data enables quality annotation of Jiminy-related training examples:
- `jiminy.synthesize` examples can be labeled by downstream follow rate
- `jiminy.evaluate` / `jiminy.evaluate_llm` examples can be validated against GUIDANCE_OUTCOME ground truth
- `jiminy.codegen` examples can be assessed by whether generated codes achieved T1 comprehension

### 2.7 Training Data Versioning Note

The v0.7.1 classifier overhaul (thresholds, prompts, outcome types, negation detection) creates a hard boundary in Jiminy training data. Pre-v0.7.1 `jiminy.evaluate` data classified 82.4% of outcomes as "ignored" due to measurement error. This data should be excluded or down-weighted — see 05_DATA_COLLECTION for the full versioning boundary specification.

### 2.8 No-Tool-Calling Architectural Policy

**All 16 MDEMG generative LLM call sites (§1.1) are single-shot structured-output or reasoning. No call site uses tool-calling.** This is an architectural policy, not an incidental property:

- Every call site is pure text-in → JSON-out (9 sites) or text-in → text-out (7 sites).
- No call site calls external tools, searches the web, executes code, or engages in a multi-turn agent loop.
- The fine-tuned model is an **internal intelligence layer (oracle)**, not an agent.

**Task-pattern classification** (see §1.1 for per-site labels):

| Task Pattern | Examples | Output |
|---|---|---|
| Classification | consulting.classify, retrieval.query_classify, jiminy.evaluate | `{type, confidence}` or short kebab-case string |
| Synthesis | jiminy.synthesize, consulting.synthesis | Short NL or JSON |
| Evaluation | jiminy.evaluate_llm | `{outcome, confidence, reasoning}` |
| Reasoning | ape.reflect, metalearn.generalize | `{insights, recommendations}` |
| Naming | hidden.name_emergence, hidden.summarize | `{name, description}` |
| Reranking | retrieval.rerank_cross, retrieval.rerank_nli | `{scores}` |

**Why this matters for fine-tuning:** tool-use variants (e.g., the former default `gpt-5-nano`) emit tool-call JSON that breaks `json.Unmarshal` parsing on MDEMG's 9 JSON-parsing call sites. The target model **must** be a base or instruct variant, not a tool-use variant. This applies to both the fine-tuned Qwen3.6-35B-A3B and any external fallback LLM (currently `gpt-4.1-nano`).

**Per memo 07 v3.1 §2 — explicit rules:**

1. **No implementation of tool-calling anywhere in the MDEMG stack.** If a task requirement surfaces that seems to need tool-calling, escalate for policy revision; do not implement without explicit revision of this policy.
2. **No `--tool-call-parser` flag** on vllm-mlx (or any inference server) launch commands.
3. **No `enable-auto-tool-choice`, `tools: [...]`, `tool_use`, `tool_call`, or `function_call` patterns** in prompt templates, adapter configurations, or request/response schemas.
4. **No community adapter or template enabled as-is** without auditing for tool-calling artifacts. Sprint A Epic 10 performs the initial grep audit; future adapter adoption must re-run it.
5. **The Qwen3.6 `preserve_thinking` parameter is documented for multi-turn agent loops. MDEMG does not use this feature. `preserve_thinking` must remain at its default in all inference configurations.**

**Enforcement:** Sprint FT-LORA-B (code/config audit) will grep-verify zero references to the nine patterns above across `internal/`, `neural/`, `scripts/`, `packaging/`, `.github/`, and repo root. Any violation is a sprint-blocker to be resolved before Sprint C.

---

## 3. Model Selection: Qwen3.6-35B-A3B MoE

### 3.1 Why Qwen3.6-35B-A3B

Qwen3.6-35B-A3B (released 2026-04-16, Apache 2.0) is a MoE model that keeps the compute economy of Qwen3-30B-A3B while adding architectural features critical to MDEMG's workload: 262K native context, Hybrid Gated DeltaNet + Gated Attention + MoE (not pure Transformer MoE), and MTP speculative decoding. Per-token activations remain at 3B (same as Qwen3-30B-A3B), so throughput is comparable on the same hardware.

| Factor | Qwen3-30B-A3B MoE (v4.0 target, superseded) | **Qwen3.6-35B-A3B MoE (v5.0 target)** | Qwen3.5-35B-A3B (fallback) |
|---|---|---|---|
| License | Apache 2.0 | **Apache 2.0** | Apache 2.0 |
| Total / active params | 30B / 3B | **35B / 3B** | 35B / 3B |
| Experts (routed / shared) | 128 / 0 | **8 routed (from 256 available) + 1 shared** | 128 / 0 |
| Architecture | Transformer MoE | **Hybrid Gated DeltaNet + Gated Attention + MoE** | Transformer MoE |
| MTP speculative decoding | ❌ | ✅ | ❌ |
| Native context | 131K | **262K (YaRN extendable to 1M)** | 262K |
| Think mode (`/think` / `/no_think`) | ✅ | ✅ | ✅ |
| Memory for inference (Q4 on M5 Max) | ~20GB | **~20.9GB** | ~20.9GB |
| Throughput target (Q4 on M5 Max) | 64-88 tok/s | **≥60 tok/s (Sprint C gate 3)** | ~60-80 tok/s |
| MLX LoRA tooling | `mlx-lm-lora` | **`mlx-lm-lora` (+ `mlx-tune` for per-expert LoRA)** | `mlx-tune` listed |

**Rationale for choosing Qwen3.6 over Qwen3.5 despite Qwen3.5 being the "mature" path:**

1. **Shared expert architecture** — the 1 shared expert + 8-of-256 routed experts is the enabling structural property for the two-tier LoRA strategy (§5). Qwen3.5 has a flatter MoE that forces monolithic LoRA.
2. **MTP speculative decoding** — reduces inference latency on long generations (think-mode reasoning, §1.1 group T) without quality loss.
3. **262K native context with YaRN extension** — removes the 131K cap that constrains long-context retrieval in the recursive self-improvement loop.

**Risk:** Qwen3.6 is 5 days old at sprint start. Sprint C's three validation gates (mlx-lm-lora convergence on 500 examples, JSON ≥95% on 9 structured tasks, ≥60 tok/s) exist to catch early-release bugs. If any gate fails, fall back to **Qwen3.5-35B-A3B** — not Qwen3-30B-A3B (which lacks the shared-expert structure required by §5).

### 3.2 Training Data Estimate

**16 tasks × 500-1000 examples each = 8,000-16,000 total anchor examples**, plus ongoing production data collection, synthetic failure examples, and HITL preference pairs.

At current development velocity (~50-100 LLM interactions/day), reaching 500 samples per task takes approximately 2-3 months. Low-frequency tasks (metalearn.generalize, hidden.reclassify) will require teacher distillation to supplement.

For the two-tier LoRA (§5), Tier 1 (universal) consumes the balanced 16-task mix; Tier 2 (per-family) consumes each family's subset. Per-family data volumes will be uneven (e.g. the J family has 3 tasks vs T's 7); §5 documents the balancing approach.

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
│  │              │   │ Qwen3.6-35B-A3B  │                        │
│  │  TSDB ───────┼──→│ (MoE, ~20.9GB Q4)│  ┌───────────────────┐ │
│  │  (metrics +  │   │ ≥60 tok/s target  │  │ Training Pipeline │ │
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
| vllm-mlx + Qwen3.6-35B-A3B Q4 | ~21GB |
| **Total used** | **~42-43GB** |
| **Available for KV cache + context** | **~85-86GB** |
| **Training (bf16 LoRA, offline)** | ~74GB (128GB available when inference stopped) |

---

## 5. MoE Two-Tier LoRA Strategy

**Source:** memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1 §3. This section is the canonical summary; consult the memo for derivation detail.

Qwen3.6-35B-A3B's MoE architecture (256 experts per layer = 1 shared always-on + 255 routed, top-8 routed per token) is the enabling property. A monolithic LoRA that touches every expert wastes parameters on experts that rarely fire for MDEMG tasks. Instead, LoRA is partitioned into two tiers with **different rank, scope, and training data**.

### 5.1 Tier 1 — Universal LoRA (all 16 tasks, balanced)

| Property | Value |
|---|---|
| **Modules** | Attention projections (`q_proj`, `k_proj`, `v_proj`, `o_proj`) + shared-expert MLP (`gate_proj`, `up_proj`, `down_proj`) on **every** transformer layer |
| **Rank** | `r = 32` |
| **Alpha** | `α = 64` (2 × r) |
| **Training data** | Balanced 16-task mix (§1.1 — equal weight per task, NOT per-record) |
| **Purpose** | Domain adaptation that applies regardless of which expert the router picks. Attention adapts to MDEMG prompt shape; shared expert adapts to MDEMG vocabulary and reasoning patterns. |
| **Quantization during training** | BF16 |
| **Quantization at inference** | BF16 (shared expert + attention stay BF16 post-adapter merge) |

Tier 1 trains **first** and **alone** — no routed experts are adapted in this stage. Sprint E's instrumentation must expose `router_aux_loss_coef` (memo §3.5) and set it to **0.002** during Tier 1 to prevent expert-routing collapse during domain adaptation.

### 5.2 Tier 2 — Per-Family LoRA (top-25% routed experts, per family)

After Tier 1 converges, Sprint D runs **expert activation profiling** (`neural/training/profile_expert_routing.py`) to identify, for each family, the top-25% most-activated routed experts across all layers. Only those experts receive a Tier 2 LoRA.

| Property | Value |
|---|---|
| **Modules** | Top-25% routed-expert MLPs (`gate_proj`, `up_proj`, `down_proj`) per family — identified by activation profiling, not hand-picked |
| **Rank** | `r = 8` |
| **Alpha** | `α = 16` (2 × r) |
| **Training data** | Each family's subset only |
| **Purpose** | Task-shape specialization on the experts the router actually picks for that family's traffic |
| **Quantization during training** | BF16 |
| **Quantization at inference** | `MXFP4_MOE` (routed experts; 4-bit MoE-aware quantization) |
| **Attention during Tier 2** | Frozen (Tier 1 adapter merged, then held fixed) |

**Three families** (provisional — see note below):

| Family | Member tasks (provisional) | Count | Sampling group (memo §3.3) |
|---|---|---|---|
| `reasoning-think` | T-group (`ape.reflect`, `consulting.synthesis`, `hidden.summarize`, `jiminy.synthesize`, `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate`) | 7 | Think-mode: temp=0.6, top_p=0.95, top_k=20, min_p=0 |
| `classify-notink` | C-group (`consulting.classify`, `hidden.reclassify`, `jiminy.evaluate`, `jiminy.codegen`, `retrieval.intent_translate`, `retrieval.query_classify`) | 6 | No-think classify: temp=0.3, top_p=0.95, top_k=20, max_tokens=64 |
| `structured-notink` | J-group (`hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`) | 3 | No-think JSON: temp=0.7, top_p=0.95, top_k=20, **presence_penalty=1.5**, max_tokens=2048 |

### 5.3 Provisional-partition clause (REQUIRED)

> **Family partition (reasoning-think / classify-notink / structured-notink) is a _starting hypothesis_. Sprint D expert activation profiling will validate or revise it. Decision criteria: if cross-family expert overlap exceeds 80%, partition will be merged; if any family shows bimodal routing, it will be split.**

The partition is derived from task-type labels (§1.1 Group column), not from observed routing. Until Sprint D profiles real routing on the Tier-1-adapted model, the family boundaries are nominal. Sprint D outputs (`profile_routing_{family}.json` + heatmap) are the arbiter.

### 5.4 Asymmetric quantization policy

Memo §3.7:

| Component | Quantization | Rationale |
|---|---|---|
| Attention (q/k/v/o_proj) | BF16 | Tier 1 LoRA merges cleanly at higher precision; attention is quality-sensitive |
| Shared expert MLP | BF16 | Activates on every token; Tier 1 adapts it; merged adapter stays BF16 |
| Routed expert MLPs (top-25% per family) | MXFP4_MOE | 4-bit MoE-aware quant; Tier 2 LoRA trained in BF16 then merge-then-quantize |
| Routed expert MLPs (remaining 75%) | MXFP4_MOE | No LoRA applied; base-weight quantization only |
| Router / gate weights | BF16 | Tiny fraction of params; BF16 avoids router instability under quantization |

`mlx_lm.convert` must accept **per-module-class** quantization selectors (memo §3.8). Sprint E owns the patch; until it lands, asymmetric quant is validated manually via the `mlx-lm-lora` fork.

### 5.5 Load-balancing auxiliary loss

`router_aux_loss_coef = 0.002` (memo §3.5) during both Tier 1 and Tier 2 training. Below 0.001 the router collapses (few experts handle all traffic); above 0.005 the load-balancing loss dominates and training loss stagnates. The 0.002 default is memo-derived; Sprint C validation may revise if early runs show routing entropy drift.

### 5.6 Sprint D validation gates (for §5 overall)

1. Tier 1 converges on balanced 16-task mix with no router-entropy collapse (layer-level entropy ≥ 1.5 nats throughout training; memo §3.6)
2. Expert activation profiler produces reproducible top-25% lists per family (two runs, same seed → same experts within ±1)
3. Cross-family expert overlap ≤ 80% OR partition is merged per §5.3 clause
4. No family shows bimodal routing (KL divergence between two activation clusters < 0.3) OR family is split per §5.3

### 5.7 Open questions (flagged; resolved in Sprints C/D)

- **Memo §6.1 — shared-expert epochs**: default 3 (same as Tier 2). Sprint C may raise to 5 if Tier 1 underconverges on the 16-task balanced mix. Not resolved in Sprint A.
- **Tier 0 router-only LoRA**: memo leaves open whether an even-smaller LoRA on the router/gate itself is warranted. Current plan: **no** — router stays frozen with only `router_aux_loss_coef` regularization. Revisit if Sprint D profiling shows the router pathologically over- or under-using specific experts for MDEMG traffic.
- **Per-family epoch allocation**: Tier 2 trains per-family, but the memo does not prescribe differential epoch counts across families. Default: same cap (max 3) for all three families; early-stop policy (see `03_IMPLEMENTATION_PLAN_v2.md §5` — Sprint A Epic 3) applies per-family independently.
