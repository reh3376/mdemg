# MDEMG LoRA Training Strategy: Deep-Dive Analysis

**Date:** 2026-03-30
**Documents reviewed:** 00_README.md through 06_CORRECTIONS_APPLIED.md (v2.0 suite)
**Context:** Full codebase audit (PR #210–#219), 2026 LoRA/GRPO/RAFT research, MDEMG VISION.md goals
**Purpose:** Answer fundamental strategic questions about MDEMG's fine-tuning trajectory

---

## Part 1: Understanding What We're Actually Doing

Before evaluating the plan, we need to be precise about what MDEMG's fine-tuning is and isn't.

### What the Fine-Tuned Model Does

MDEMG uses external OpenAI models (default: `gpt-5-nano` for most tasks, `gpt-4o-mini` for reranking/summarization/synthesis/intent) routed through `llmclient` for 16 internal tasks — things like classifying whether a piece of code observation is a constraint, synthesizing guidance for the coding agent, evaluating whether the agent followed that guidance, and reflecting on system health.

The fine-tuned Qwen3-30B-A3B MoE replaces this external LLM. It doesn't replace Claude Code (the coding agent). It doesn't write code, generate files, or interact with the developer. It's an internal intelligence layer — the brain that powers MDEMG's constraint detection, guidance synthesis, quality evaluation, and self-improvement systems.

This distinction matters enormously for training strategy.

### What AI Coding Agents Actually Do

When Claude Code (or any AI coding agent) generates an answer, it:

1. **Receives context** — the conversation history, system prompt, project files, tool results
2. **Reasons** — internally processing the constraints, goals, and available information
3. **Decides** — which tool to use, what code to write, what questions to ask
4. **Acts** — executes the chosen action (Write, Edit, Bash, etc.)
5. **Gets feedback** — tool output, error messages, test results

MDEMG inserts itself into steps 1 and 4-5 of this loop. Before each prompt, `prompt-context.sh` injects Jiminy guidance into Claude's context window (step 1). After each tool call, `post-tool-observe.py` evaluates whether Claude followed the guidance (steps 4-5) and feeds the outcome back for learning.

The fine-tuned model needs to be excellent at:
- **Understanding what the coding agent did** (parsing action summaries, diffs, tool outputs)
- **Understanding organizational context** (the specific codebase, its constraints, its patterns)
- **Making fast, accurate judgments** (is this a constraint violation? did the agent follow guidance?)
- **Generating useful guidance** (synthesizing relevant constraints into actionable text)

It does NOT need to be excellent at:
- Writing code (Claude Code does that)
- General knowledge (the base Qwen3 model already has that)
- Long creative writing (all outputs are structured JSON or short narratives)
- Vector embeddings (a separate encoder model handles this — `text-embedding-3-large` or `qwen3-embedding:8b`)

**Embedding is a separate workstream.** MDEMG's embedding model (`internal/embeddings/`) is architecturally independent from the generative LLM (`internal/llmclient/`). Embedding models use contrastive learning, not SFT/GRPO. The LoRA fine-tuning plan targets only the 16 generative tasks. Embedding fine-tuning is a future optimization with its own data collection pipeline (embedding_events + retrieval_events tables in TimescaleDB). Any future fine-tuned embedding model must produce **3072-dimension vectors** — this matches the Neo4j vector index, OpenAI `text-embedding-3-large` (native 3072), and Ollama `qwen3-embedding:8b` (4096 MRL-truncated to 3072).

### The Core Insight: MDEMG is a RAFT System Already

Here's something the current plan doesn't articulate clearly enough: **MDEMG is already a hybrid RAG + fine-tuning system by architecture.** The Neo4j knowledge graph is the retrieval component. The LLM calls are the generation component. The fine-tuning plan adds behavioral optimization to the generation side.

This maps directly to the 2026 consensus: "RAG for facts, fine-tuning for behavior." MDEMG's knowledge graph holds the facts (constraints, observations, code patterns, organizational knowledge). The fine-tuned model learns the behavior (how to classify, synthesize, evaluate, and reflect on those facts).

The current plan implicitly follows this pattern but doesn't explicitly design for it. This is the single biggest architectural opportunity the plan misses — and it connects directly to the RAFT (Retrieval Augmented Fine-Tuning) research from UC Berkeley.

---

## Part 2: Is the Current Trajectory the Best Way?

### What the Plan Gets Right

**1. Single MoE model for all tasks:** The plan correctly identifies that 16 tasks can be served by a single fine-tuned model with task-specific system prompts. The MoE architecture (only 3B active parameters per token) gives 80+ tok/s inference on M5 Max — well within latency requirements for all tasks.

**2. SFT → GRPO/DPO training pipeline:** This is the 2026 gold standard. SFT establishes behavioral patterns, GRPO with verifiable rewards (RLVR) sharpens them. MDEMG has verifiable rewards for most tasks — JSON validity, classification accuracy, comprehension scores, follow rates — which makes GRPO particularly well-suited. The ToolRL paper (ICLR 2026) confirms that fine-grained reward design for tool use outperforms coarse answer-matching.

**3. Anti-collapse protocol (α ≥ 0.4):** This is well-researched and correctly implemented. Multiple peer-reviewed sources confirm that recursive self-training without sufficient exogenous signal causes model collapse. The 40% minimum is conservative and safe.

**4. Data collection infrastructure (PRs #217-#219):** The interaction logger, guidance_id correlation, source_path linkage, privacy scrubber, and quality annotation pipeline are production-grade. This infrastructure was the long pole for the entire plan and it's now largely complete.

**5. Think/no-think task routing:** Using Qwen3's built-in `/think` mode for complex reasoning tasks and `/no_think` for simple classification is efficient and well-designed.

### What the Plan Gets Wrong (or Misses)

**1. The plan treats fine-tuning as offline, periodic, and separate from operation.**

The current architecture is: collect data → stop inference → train → deploy new model → resume. This works for the first few iterations but doesn't scale. Every training cycle requires stopping the inference server, and the plan estimates 6-10 hours per training run.

What it should be: **continuous, incremental, and operational.** The system should always be collecting data, always have a quality-gated pipeline ready, and be able to train during low-usage periods without stopping inference. The M5 Max has 128GB — there's room to run a lightweight Q4 inference server (~22GB) while simultaneously doing QLoRA training (~40-50GB) if the training uses 4-bit quantization.

**Recommendation:** Design the training system for unattended operation from the start. The `cycle_runner.py` should handle the full lifecycle: detect sufficient new data → evaluate quality → train → benchmark → deploy/reject — all triggered by a cron job or RSIC cycle.

**2. The plan doesn't leverage RAFT (Retrieval-Augmented Fine-Tuning).**

This is the biggest strategic gap. MDEMG already has a retrieval system (Neo4j graph → embedding → rerank). The fine-tuned model already receives retrieved context in its prompts (constraints, observations, code patterns). But the training data doesn't include the retrieval context.

When the constraint classifier receives a node to classify, it gets the node content — but it doesn't get the surrounding graph context that helped identify the node. When the guidance synthesizer runs, it receives constraint items — but the training record doesn't capture which nodes were retrieved and which were distractors.

UC Berkeley's RAFT research shows that training a model to work in an "open-book" setting — where it must identify relevant documents and ignore distractors — significantly outperforms both pure RAG and pure fine-tuning. MDEMG is already operating in this setting; the training pipeline just doesn't capture it.

**Recommendation:** Enrich the training data to include retrieval context:
- For `consulting.classify`: include the query that triggered retrieval, the candidate nodes returned, and which one was classified
- For `jiminy.synthesize`: include the retrieval results that informed the guidance, not just the final guidance output
- For `jiminy.evaluate`: include the constraints that were retrieved for comparison
- Train with an 80/20 split: 80% of examples include the relevant retrieved context, 20% don't (forcing the model to learn when retrieval helps vs. when it should rely on internalized knowledge)

**3. The plan over-specifies Python training infrastructure that may not be needed.**

The plan calls for ~15 new Python files across training, benchmarks, HITL, and data processing. Many of these duplicate functionality available in existing tools (Unsloth, TRL, mlx-lm-lora). For example:
- `cycle_runner.py` orchestrates something that could be a shell script calling `mlx-lm-lora`
- `grpo_data_gen.py` transforms data into a format that TRL/Unsloth already accept
- `hitl_server.py` builds a full FastAPI app for preference review

A solo developer maintaining a Go project doesn't need a parallel Python ML engineering stack. The training infrastructure should be as thin as possible — just enough to bridge MDEMG's data (in TimescaleDB and JSONL) to existing training frameworks.

**Recommendation:** Use `mlx-lm-lora` (for MLX) or Unsloth (if switching to GPU) directly with minimal wrapper scripts. The `mdemg finetune` CLI should call these tools, not reimplment them.

**4. The plan doesn't account for system prompt evolution.**

The 16 system prompts (~200 lines total) define each task's behavior. As MDEMG evolves, these prompts change — new fields are added, output formats are refined, edge cases are addressed. Every prompt change invalidates some training data because the model was trained on the old prompt.

The plan mentions "progressive prompt compression" (Phase 2H) but doesn't address prompt versioning in the training data. If the constraint classifier's system prompt changes from "output `{type, confidence}`" to "output `{type, confidence, severity}`", all training examples using the old format become noise.

**Recommendation:** Version system prompts alongside training data. Each `InteractionRecord` should include a hash of the system prompt that was used. During training data curation, filter to the current prompt version (or the N most recent versions). The `dataset_versioner.py` should enforce this automatically.

**5. The plan doesn't consider per-task LoRA adapters.**

The current plan trains a single merged LoRA adapter for all 16 tasks. Research on multi-task LoRA (MeTA-LoRA, D²C clustering, Mixture-of-LoRAs) shows that tasks with very different requirements can interfere during joint training. MDEMG's 16 tasks span a wide range:

- **Simple classification** (query_classifier, constraint_classifier): r=16 is sufficient
- **Complex reasoning** (rsic_reflection, j9_evaluation): r=32-64 is better
- **Structured generation** (emergence_namer, j17_codegen): format-critical
- **Free-form generation** (guidance_synthesis, memory_synthesis): quality-critical

Training all of these jointly with the same rank and learning rate is suboptimal. The D²C research (Jan 2026) shows that clustering tasks by similarity and training separate adapters within clusters outperforms joint training.

**Recommendation:** Start with a single adapter (the plan's current approach) for simplicity. But design the infrastructure so that per-task or per-cluster adapters can be added later. vllm-mlx supports LoRA adapter swapping — the task router in `llmclient` could select different adapters based on `taskName`. This is a future optimization, not a v1 requirement.

---

## Part 3: How Important is LoRA to the Process?

### LoRA is essential but not sufficient

LoRA is the right technique for MDEMG's fine-tuning because:

1. **Memory fits:** bf16 LoRA on Qwen3-30B-A3B requires ~74GB — fits in 128GB with headroom
2. **Adapters are small:** ~200-400MB per version, enabling version management and rollback
3. **Training is fast:** ~6-10 hours on M5 Max for a full cycle, enabling iterative improvement
4. **No catastrophic forgetting:** The base model's general capabilities are preserved
5. **Adapter swapping:** vllm-mlx can hot-swap adapters without reloading the base model

But LoRA alone doesn't solve the problem. The critical success factors are:

1. **Data quality** — LoRA amplifies whatever patterns are in the training data. Bad data → confidently wrong model. This is why the privacy scrubber, quality annotation pipeline, and guidance_id correlation are prerequisites, not nice-to-haves.

2. **Reward function design** — For GRPO, the reward functions must accurately capture "good" vs. "bad" for each task. MDEMG has an advantage here: many tasks have deterministic rewards (JSON validity, classification accuracy, follow rates). But the subjective tasks (guidance quality, reflection insight quality) need careful LLM-as-judge design.

3. **Anti-collapse discipline** — The recursive loop is the plan's most innovative feature, but also its greatest risk. LoRA makes retraining cheap enough to do iteratively, but each iteration must maintain the α ≥ 0.4 exogenous ratio.

### When LoRA isn't the answer

Some of MDEMG's capabilities are better served by other approaches:

- **Retrieval quality:** The cross-encoder reranker already has its own training pipeline (`train.py` in neural sidecar). LoRA on the generative model won't improve retrieval — better embeddings and reranking will. Embedding fine-tuning is a separate workstream using contrastive learning on an encoder model that produces 3072-dim vectors. Data collection infrastructure (embedding_events + retrieval_events tables) captures the training signal: the gap between vector recall ranking and cross-encoder reranking identifies hard negatives that teach the embedding model what "actually relevant" means in MDEMG's domain.
- **Real-time context:** MDEMG's knowledge graph changes constantly. Fine-tuning can't keep up with daily ingestion. This is why the RAG/retrieval side must remain the primary knowledge source.
- **Format compliance:** If the main failure mode is "model sometimes outputs invalid JSON," prompt engineering or constrained decoding (which vllm-mlx supports) may be more effective than fine-tuning.

---

## Part 4: Should LLM Calls Use the UxTS Framework?

### Yes — and the framework already exists. It's called ULTS.

MDEMG's UxTS framework has 14 framework types covering everything from API contracts (UATS, 202 specs) to parser conformance (UPTS, 27 specs) to emergence naming quality (UETS, 8 specs). But there's no framework for LLM call contracts — no specification of "this task expects this input format and must produce this output format."

This is a gap that directly impacts training data quality. Right now, the system prompt constants define the expected behavior, but there's no machine-readable specification that can be:
- Validated at runtime (does the LLM output match the contract?)
- Used in training data curation (filter examples where the output didn't match)
- Enforced in benchmarks (the 04_BENCHMARK_RL.md task registry is essentially a manual version of this)

### Proposed: ULTS (Universal LLM Task Specification)

A new UxTS framework that defines, for each LLM task:

```json
{
  "task_name": "constraint_classification",
  "version": "1.2.0",
  "system_prompt_hash": "sha256:abc123...",
  "input_schema": {
    "type": "object",
    "properties": {
      "node_id": {"type": "string"},
      "text": {"type": "string", "maxLength": 2000}
    }
  },
  "output_schema": {
    "type": "object",
    "properties": {
      "is_constraint": {"type": "boolean"},
      "type": {"type": "string", "enum": ["architectural", "process", "security", "none"]},
      "confidence": {"type": "number", "minimum": 0, "maximum": 1},
      "summary": {"type": "string"}
    },
    "required": ["is_constraint", "type", "confidence"]
  },
  "think_mode": false,
  "latency_budget_ms": 2000,
  "quality_metrics": {
    "accuracy": {"threshold": 0.85, "weight": 0.3},
    "precision": {"threshold": 0.80, "weight": 0.25},
    "format_valid_rate": {"threshold": 0.95, "weight": 0.25}
  },
  "reward_functions": ["json_valid_reward", "classification_accuracy_reward"],
  "training_config": {
    "rank": 16,
    "min_examples": 500,
    "quality_gate": 0.7
  }
}
```

This gives us:

1. **Compile-time validation:** Go code can validate LLM responses against the output schema before `json.Unmarshal` — catching format errors at the call site, not in downstream code.

2. **Training data curation:** The quality annotator can automatically filter examples where the output didn't match the schema, where latency exceeded the budget, or where quality fell below thresholds.

3. **Benchmark automation:** The task registry in 04_BENCHMARK_RL.md becomes derivable from the ULTS specs rather than maintained separately.

4. **Prompt versioning:** The `system_prompt_hash` links each spec version to a specific system prompt, solving the training data invalidation problem.

5. **Adapter routing:** The `training_config.rank` field enables per-task adapter configuration if we move to per-task LoRA adapters later.

### Implementation

16 ULTS spec files (one per task) in `docs/tests/ults/specs/`. A `ults_runner.py` that validates runtime behavior against specs. Schema at `docs/tests/ults/schema/ults.schema.json`.

The existing UOBS spec `llm_interaction_logging.uobs.json` already covers observability. ULTS would cover the contract and quality dimensions.

---

## Part 5: How Do We Ensure High-Value Training Data?

### The Data Quality Stack

Training data quality is the single most important factor in fine-tuning success. The plan's existing data collection infrastructure (PRs #217-#219) is solid. Here's how to maximize the value of what's collected:

**Layer 1: Collection completeness (implemented)**
- All 16 LLM consumers labeled with task names ✅
- guidance_id correlation for Jiminy feedback loop ✅  
- source_path linkage for ingest-triggered calls ✅
- Privacy scrubbing at write time ✅
- JSONL collectors for rerank + protocol data (config flip needed)

**Layer 2: Quality filtering (partially implemented)**
- `quality_annotator.py` joins feedback outcomes with interaction records ✅
- Deterministic filters: JSON validity, non-empty response, non-error, latency < 60s
- Deduplication: SHA-256 hash of (system_prompt + user_prompt)
- Model version tracking: exclude data from models known to be degraded

**Layer 3: Curation for training (not yet implemented)**
- Temporal splits: train data BEFORE test data by timestamp
- Exogenous ratio enforcement: α ≥ 0.4 per dataset
- Task balance: upsample underrepresented tasks, downsample overrepresented ones
- Difficulty stratification: include easy, medium, and hard examples per task

**Layer 4: Anchor dataset (not yet implemented)**
- Teacher distillation: use the external LLM to generate high-quality examples for all 16 tasks
- This provides the exogenous signal that prevents model collapse
- Should be generated from real MDEMG inputs (extracted from Neo4j) with the best available external LLM
- Refreshed every 3 training cycles

### What Makes a Training Example "High Value"?

Not all collected data is equally useful. The highest-value examples are:

1. **Edge cases where the external LLM got it right despite difficulty** — these teach the fine-tuned model to handle hard inputs correctly
2. **Examples where guidance was followed** (quality=1.0) — these are positive training signals for the Jiminy tasks
3. **Examples where guidance was contradicted** (quality=0.0) with human confirmation that the contradiction was wrong — these are negative training signals
4. **Examples with rich retrieval context** — where the model had to filter relevant from irrelevant retrieved information (the RAFT pattern)
5. **Examples from diverse time periods** — preventing the model from overfitting to a specific development phase

The lowest-value examples are:
- Duplicate prompts (same question asked multiple times)
- Error responses (LLM timed out, circuit breaker tripped)
- Degraded-model outputs (from a model version that was later rejected)
- Examples where the system prompt changed and the output no longer matches the current format

---

## Part 6: Retraining Strategy and Future-Proofing

### Will Retraining Be Needed?

Yes, absolutely. MDEMG is a living system that evolves rapidly (~4-5 PRs/day). Retraining will be needed when:

1. **System prompts change** — new output fields, refined formats, edge case handling
2. **New tasks are added** — MDEMG's architecture is designed for extensibility
3. **The knowledge domain shifts** — new codebases, new teams, new organizational patterns
4. **Model performance degrades** — detected by benchmark regression or entropy decay
5. **Better base models become available** — Qwen3.5, Qwen4, or other Apache 2.0 models

### How Often?

Based on MDEMG's development velocity, expect:
- **Monthly SFT refreshes** — incorporate new production data, refresh anchor dataset
- **Quarterly GRPO cycles** — after sufficient quality-annotated data accumulates
- **Ad-hoc retraining** — when system prompts change significantly or new tasks are added
- **Base model upgrades** — when a significantly better Apache 2.0 model is released (maybe 1-2x/year)

### Design for Retrainability

The training infrastructure should assume retraining is routine, not exceptional. This means:

**1. Automated pipeline with regression gates:**
```
New data accumulates → Quality check → Dataset assembly → 
Train LoRA → Benchmark vs. current → Deploy if better / Reject if worse
```

**2. Version management:**
- Every LoRA adapter version gets a manifest: training data hash, base model version, system prompt hashes, benchmark scores, timestamp
- Keep the last 3 adapter versions for rollback
- Store adapters on external SSD (they're only 200-400MB each)

**3. Dataset versioning with provenance:**
- Each curated dataset tracks: which production data periods it includes, the exogenous ratio, the temporal split boundaries, the system prompt version, the quality thresholds applied
- Datasets are immutable once created — new data creates a new version

**4. Graceful degradation:**
- If the fine-tuned model is rejected by benchmarks, fall back to the external LLM automatically
- The `llmclient` already supports provider switching — this just needs a health check

**5. ULTS spec versioning:**
- When a system prompt changes, the ULTS spec version bumps
- Training data from old spec versions can be migrated forward if the change is additive (new fields)
- Training data is discarded if the change is breaking (removed fields, changed format)

---

## Part 7: Recommendations — What Should Change

### Immediate (Before First Training Cycle)

| # | Change | Impact | Effort |
|---|---|---|---|
| 1 | **Flip config switches** to start collecting rerank + protocol JSONL + enable TSDB backup | Every day without collection is lost data | 5 minutes |
| 2 | **Build SanitizeResponse/StripThinkBlock** (Phase 2D-2F of impl plan) | Required before switching to any local model | Small |
| 3 | **Add system prompt hash to InteractionRecord** | Enables training data filtering by prompt version | Small |
| 4 | **Create ULTS spec framework** (16 specs) | Formalizes LLM contracts, enables automated validation | Medium |
| 5 | **Build embedding + retrieval event loggers** | Starts collecting 3072-dim embedding training data (contrastive pairs from recall/rerank gap) | Medium |

### Strategic (Plan Modifications)

| # | Change | Current Plan | Recommendation |
|---|---|---|---|
| 6 | **Include retrieval context in training data** (RAFT pattern) | Training data only captures LLM I/O | Enrich InteractionRecord with retrieval context (retrieved nodes, scores) |
| 7 | **Design for concurrent inference + training** | Stop inference → train → restart | Use QLoRA during training so inference can continue at reduced capacity |
| 8 | **Thin the Python infrastructure** | ~15 new Python files | Use mlx-lm-lora/Unsloth directly with thin wrapper scripts |
| 9 | **Add prompt versioning to dataset curation** | Not addressed | Filter training data by system_prompt_hash; discard data from old prompt versions |
| 10 | **Plan for per-task adapters** as a future option | Single merged adapter | Design adapter routing in llmclient; implement as single adapter first |

### Long-Term (Design for the Future)

| # | Change | Rationale |
|---|---|---|
| 11 | **Automate the full cycle as a cron job** | Solo developer can't babysit training runs |
| 12 | **Build a model registry** with benchmark scores, provenance, and rollback | Essential for managing multiple versions |
| 13 | **Plan base model upgrades** | Qwen3.5-35B-A3B (262K context, vision) is the natural successor; the pipeline should support model swaps |

---

## Part 8: Summary — The Strategic Position

MDEMG's fine-tuning plan is fundamentally sound. The architecture — single MoE model, SFT → GRPO/DPO pipeline, anti-collapse protocol, hybrid RAG + fine-tuning — is exactly what 2026 research recommends. The data collection infrastructure (PRs #217-#219) is production-grade and ahead of where most projects are at this stage.

The key strategic insight is that **LoRA fine-tuning for MDEMG is not about teaching the model new knowledge — it's about teaching the model MDEMG-specific behavior.** The knowledge lives in the Neo4j graph and is retrieved at runtime. The behavior — how to classify constraints, how to synthesize guidance, how to evaluate agent actions — is what gets baked into the LoRA adapter.

This maps perfectly to the 2026 consensus: **"RAG for facts, fine-tuning for behavior."** MDEMG already has the RAG side (graph + retrieval + reranking). The fine-tuning side teaches the model to be an expert at MDEMG's specific tasks.

The most important changes are:
1. **Start collecting data now** (config flip — zero code)
2. **Formalize LLM contracts with ULTS specs** (enables automated quality gating)
3. **Include retrieval context in training data** (the RAFT pattern — the biggest quality improvement)
4. **Build embedding + retrieval event loggers** (start collecting 3072-dim embedding training data for future contrastive fine-tuning)
5. **Design for routine retraining** (it's not a one-time project, it's an ongoing capability)
6. **Keep the Python training stack thin** (use existing tools, don't rebuild them)

The plan has two distinct workstreams: **generative LoRA** (Qwen3-30B-A3B, 16 tasks, SFT → GRPO/DPO) and **embedding fine-tuning** (contrastive learning, 3072-dim target, hard-negative mining from recall/rerank gap). Both share the same data collection principle: collect now, curate later, train when ready.

The vision is achievable. The infrastructure is largely built. The remaining work is collecting enough data, curating it well, and running the first training cycle. At current development velocity (~4-5 PRs/day generating ~50-100 LLM interactions/day), you'll have enough data for a meaningful first generative SFT cycle in about 2-3 months. Embedding training data accumulates in parallel through the same development activity.
