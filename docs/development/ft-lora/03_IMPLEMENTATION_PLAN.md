# MDEMG Fine-Tuned LLM: Implementation Plan

**Date:** 2026-03-27 (v2.0 — all deep-dive corrections applied)
**Model:** Qwen3-30B-A3B MoE via vllm-mlx
**Scope:** 9 phases, 15 LLM consumers, ~55 files

---

## Phase 1: LLM Interaction Logger + LLMCompleter Interface (Go)

### 1A. New File: `internal/llmclient/interface.go`

Extract interface from concrete `*Client`:

```go
// LLMCompleter is the interface all LLM call sites use.
type LLMCompleter interface {
    Complete(ctx context.Context, messages []Message, opts CompleteOpts) (string, error)
    CompleteWithUsage(ctx context.Context, messages []Message, opts CompleteOpts) (string, int, error)
}
```

### 1B. New File: `internal/llmclient/interaction_logger.go`

Wraps any `LLMCompleter` and logs all I/O to JSONL:

```go
type InteractionLogger struct {
    inner     LLMCompleter
    collector *InteractionCollector
    taskName  string
}

type InteractionRecord struct {
    Timestamp    string  `json:"timestamp"`
    TraceID      string  `json:"trace_id"`
    TaskName     string  `json:"task_name"`
    SpaceID      string  `json:"space_id,omitempty"`
    SessionID    string  `json:"session_id,omitempty"`
    SystemPrompt string  `json:"system_prompt"`
    UserPrompt   string  `json:"user_prompt"`
    Response     string  `json:"response"`
    ThinkContent string  `json:"think_content,omitempty"`
    Think        bool    `json:"think"`
    LatencyMs    int64   `json:"latency_ms"`
    TokensIn     int     `json:"tokens_in"`
    TokensOut    int     `json:"tokens_out"`
    ModelName    string  `json:"model_name"`
    Provider     string  `json:"provider"`
    Error        string  `json:"error,omitempty"`
    Quality      float64 `json:"quality,omitempty"`
    QualitySource string `json:"quality_source,omitempty"`
}
```

### 1C. New File: `internal/llmclient/interaction_collector.go`

JSONL writer with rotation (50MB) and pruning (180 days). Same pattern as `rerank_collector.go`.

### 1D. New File: `internal/llmclient/scrubber.go`

Privacy scrubbing — strip API keys, tokens, passwords before writing to JSONL.

### 1E. Modify: All 15 LLM Consumers — `*Client` → `LLMCompleter`

| # | File | Field | Task Name |
|---|---|---|---|
| 1 | `internal/ape/llm_reflector.go` | `llm` | `rsic_reflection` |
| 2 | `internal/consulting/llm_classifier.go` | `llm` | `constraint_classification` |
| 3 | `internal/consulting/synthesis.go` | `llm` | `memory_synthesis` |
| 4 | `internal/hidden/cluster_summarizer.go` | `llm` | `cluster_summarization` |
| 5 | `internal/hidden/emergence_namer.go` | `llm` | `emergence_naming` |
| 6 | `internal/hidden/reclassifier.go` | `llm` | `node_reclassification` |
| 7 | `internal/jiminy/evaluator.go` | `llm` | `j9_evaluation` |
| 8 | `internal/jiminy/outcome_classifier.go` | `llm` | `outcome_classification` |
| 9 | `internal/jiminy/synthesizer.go` | `llm` | `guidance_synthesis` |
| 10 | `internal/jiminy/codegen.go` | `client` | `j17_codegen` |
| 11 | `internal/metalearn/generalizer.go` | `llm` | `cross_space_generalization` |
| 12 | `internal/retrieval/intent_translator.go` | `llm` | `intent_translation` |
| 13 | `internal/retrieval/query_classifier.go` | `llm` | `query_classification` |
| 14 | `internal/retrieval/rerank.go` | (inline) | `llm_reranking` |
| 15 | `internal/summarize/service.go` | `llm` | `code_summarization` |

### 1F. Modify: `internal/api/server.go`

Wire interaction loggers at all 15 creation points:

```go
// Pattern at each site:
rawLLM := llmclient.New(llmclient.Config{...})
loggedLLM := llmclient.NewInteractionLogger(rawLLM, interactionCollector, "rsic_reflection")
```

### 1G. Modify: `internal/config/config.go`

```go
LLMInteractionLogging  bool   // LLM_INTERACTION_LOGGING (default: true — ON by default)
LLMInteractionDir      string // LLM_INTERACTION_DIR (default: ".mdemg/neural/llm-interactions")
LLMInteractionMaxMB    int    // LLM_INTERACTION_MAX_MB (default: 50)
LLMInteractionRetainD  int    // LLM_INTERACTION_RETAIN_DAYS (default: 180)
```

**Effort:** M — mechanical but touches 15 files + server.go.

---

## Phase 2: Think Mode + Response Sanitization (Go)

### 2A. Modify: `internal/llmclient/client.go` — Add Think to CompleteOpts

```go
type CompleteOpts struct {
    MaxTokens   int
    Temperature *float64
    Format      json.RawMessage
    Options     map[string]any
    Think       bool            // NEW: enable /think mode for reasoning tasks
}
```

### 2B. Modify: `internal/llmclient/client.go` — Think Mode in OpenAI Provider

Since vllm-mlx is OpenAI-compatible, think mode is controlled via the system prompt or the vllm-mlx reasoning parser. The OpenAI provider prepends `/think` or `/no_think` to the first user message:

```go
func (c *Client) completeOpenAI(ctx context.Context, messages []Message, opts CompleteOpts) (string, error) {
    // Prepend think mode directive
    if len(messages) > 0 {
        for i, m := range messages {
            if m.Role == "user" {
                if opts.Think {
                    messages[i].Content = "/think\n" + m.Content
                } else {
                    messages[i].Content = "/no_think\n" + m.Content
                }
                break
            }
        }
    }
    // ... existing OpenAI request logic ...
}
```

### 2C. Modify: All 15 Call Sites — Set Think Mode

| Tasks with `Think: true` | Tasks with `Think: false` |
|---|---|
| rsic_reflection (1) | constraint_classification (2) |
| memory_synthesis (3) | emergence_naming (5) |
| cluster_summarization (4) | node_reclassification (6) |
| j9_evaluation (7) | outcome_classification (8) |
| guidance_synthesis (9) | j17_codegen (10) |
| cross_space_generalization (11) | intent_translation (12) |
| llm_reranking (14) | query_classification (13) |
| code_summarization (15) | |

### 2D. New File: `internal/llmclient/sanitize.go` *(CRITICAL — v2.0 addition)*

**9 of 15 consumers call `json.Unmarshal` on the raw LLM response.** Qwen3's think mode returns `<think>reasoning</think>\n{"json": "response"}`. Without stripping the think block, all JSON-parsing consumers break.

```go
// SanitizeResponse strips think blocks and code fences from LLM output.
// Must be called before json.Unmarshal in all consumers.
func SanitizeResponse(s string) string {
    s = StripThinkBlock(s)
    s = StripCodeFence(s)
    return strings.TrimSpace(s)
}

// StripThinkBlock removes <think>...</think> blocks from model output.
// Handles multiline think blocks, nested tags, and missing close tags.
func StripThinkBlock(s string) string {
    // Find <think> and </think> tags
    startIdx := strings.Index(s, "<think>")
    if startIdx == -1 {
        return s
    }
    endIdx := strings.Index(s, "</think>")
    if endIdx == -1 {
        // No closing tag — strip from <think> to end (malformed)
        return strings.TrimSpace(s[:startIdx])
    }
    // Strip the think block and return the remainder
    before := s[:startIdx]
    after := s[endIdx+len("</think>"):]
    result := before + after
    return strings.TrimSpace(result)
}
```

### 2E. Modify: All 9 JSON-Parsing Consumers

Replace `StripCodeFence` with `SanitizeResponse` in every consumer that parses JSON:

| File | Current | Updated |
|---|---|---|
| `ape/llm_reflector.go:246` | `raw = llmclient.StripCodeFence(raw)` | `raw = llmclient.SanitizeResponse(raw)` |
| `consulting/llm_classifier.go:223` | `raw = llmclient.StripCodeFence(raw)` | `raw = llmclient.SanitizeResponse(raw)` |
| `hidden/emergence_namer.go:198` | `raw = llmclient.StripCodeFence(raw)` | `raw = llmclient.SanitizeResponse(raw)` |
| `hidden/reclassifier.go:408` | `raw = llmclient.StripCodeFence(raw)` | `raw = llmclient.SanitizeResponse(raw)` |
| `jiminy/evaluator.go` | (check current) | `raw = llmclient.SanitizeResponse(raw)` |
| `jiminy/outcome_classifier.go` | (check current) | `raw = llmclient.SanitizeResponse(raw)` |
| `metalearn/generalizer.go` | (check current) | `raw = llmclient.SanitizeResponse(raw)` |
| `retrieval/query_classifier.go` | (check current) | `raw = llmclient.SanitizeResponse(raw)` |
| `summarize/service.go` | (check current) | `raw = llmclient.SanitizeResponse(raw)` |

### 2F. New: Format Retry Logic *(v2.0 addition)*

Fine-tuned models produce malformed JSON more often than external LLMs, especially in early training iterations. Add optional retry to `LLMCompleter`:

```go
// CompleteJSON wraps Complete with JSON validation and single retry.
func CompleteJSON(ctx context.Context, llm LLMCompleter, messages []Message, opts CompleteOpts) (string, error) {
    raw, err := llm.Complete(ctx, messages, opts)
    if err != nil {
        return "", err
    }

    raw = SanitizeResponse(raw)
    if json.Valid([]byte(raw)) {
        return raw, nil
    }

    // Retry with format correction prompt
    retryMessages := append(messages, Message{
        Role:    "assistant",
        Content: raw,
    }, Message{
        Role:    "user",
        Content: "Your response was not valid JSON. Respond with ONLY the JSON object, no explanation.",
    })

    raw, err = llm.Complete(ctx, retryMessages, opts)
    if err != nil {
        return "", err
    }
    return SanitizeResponse(raw), nil
}
```

Consumers that need JSON can optionally use `CompleteJSON` instead of `Complete` for resilience.

### 2G. New: System Prompt Compression Strategy *(v2.0 addition)*

After fine-tuning, the model already knows MDEMG's domain. The 15 system prompts (~200 lines total of domain explanation) become redundant. Plan for progressive compression:

```go
// SystemPromptMode controls prompt verbosity based on model type.
type SystemPromptMode int

const (
    PromptFull     SystemPromptMode = iota // External LLM: full explanation
    PromptCompact                          // Fine-tuned v1-v2: task prefix + key constraints only
    PromptMinimal                          // Fine-tuned v3+: task prefix only
)
```

Each consumer already has a compact variant (e.g., `classifySystemPromptCompact`, `guardrailSystemPromptCompact`). The mode is controlled by config:

```go
LLMPromptMode string // LLM_PROMPT_MODE — "full", "compact", "minimal" (default: "full")
```

Switch to "compact" after fine-tuned model benchmarks prove ≥95% quality parity with full prompts.

**Effort:** M — SanitizeResponse is critical path, rest is small additions.

---

## Phase 3: vllm-mlx Integration (Simplified from v1.0)

**v1.0 built a custom `generator.py`, `schemas_generate.py`, and `mlx.go`. v2.0 eliminates all three.**

### 3A. Install and Configure vllm-mlx

```bash
uv tool install git+https://github.com/waybarrios/vllm-mlx.git
```

### 3B. Add launchd Service for vllm-mlx

New file: `packaging/mdemg_linux/systemd/mdemg-vllm-mlx.service` (Linux) and corresponding launchd plist for macOS:

```plist
<!-- ~/Library/LaunchAgents/com.mdemg.vllm-mlx.plist -->
<plist version="1.0">
<dict>
    <key>Label</key><string>com.mdemg.vllm-mlx</string>
    <key>ProgramArguments</key>
    <array>
        <string>vllm-mlx</string>
        <string>serve</string>
        <string>mlx-community/Qwen3-30B-A3B-4bit</string>
        <string>--port</string><string>8100</string>
        <string>--continuous-batching</string>
        <string>--use-paged-cache</string>
        <string>--reasoning-parser</string><string>qwen3</string>
    </array>
    <key>RunAtLoad</key><true/>
    <key>KeepAlive</key><true/>
</dict>
</plist>
```

### 3C. Modify: `internal/config/config.go`

```go
// The existing LLM_BASE_URL config already supports this:
LLMBaseURL string // LLM_BASE_URL — for vllm-mlx: "http://localhost:8100/v1"
```

### 3D. No Custom Code Needed

vllm-mlx serves OpenAI-compatible `/v1/chat/completions`. The existing `llmclient` OpenAI provider works directly. Model loading/unloading for training cycles is handled via vllm-mlx's admin API or by stopping/starting the service.

**Effort:** S — config + service file only. No Go or Python code.

---

## Phase 4: Teacher Distillation Pipeline (Python)

### 4A. New File: `neural/training/input_extractor.py`

Extracts real task inputs from Neo4j and existing data sources.

### 4B. New File: `neural/training/teacher_distill.py`

Generates anchor training dataset by running real inputs through external LLM. Target: ~9,400 examples across 15 tasks.

### 4C. New File: `neural/training/synthetic_failures.py`

Generates training examples for silent failure detection (property mismatches, fallback cascades, circular dependencies, config default silencing).

### 4D. New File: `neural/training/quality_filter.py`

Filters raw data: removes errors, empties, duplicates, fallback records, timeout artifacts.

### 4E. New File: `neural/training/format_converter.py`

Converts filtered interaction records to MLX chat format with task-prefix system prompts.

**Effort:** M — 5 Python scripts, mostly data processing.

---

## Phase 5: Training Pipeline (Python + MLX)

### 5A. New File: `neural/training/train_ft.py`

LoRA fine-tuning using MLX. Uses `mlx-lm-lora` for MoE support:

```bash
python -m mlx_lm_lora.train \
  --model mlx-community/Qwen3-30B-A3B \
  --train --train-mode sft \
  --data .mdemg/neural/datasets/v1/train.jsonl \
  --train-type lora \
  --batch-size 4 --num-epochs 3 \
  --learning-rate 2e-4 \
  --max-seq-length 8192 \
  --adapter-path .mdemg/neural/models/ft/v1
```

### 5B. New File: `neural/training/evaluate_ft.py`

Per-task evaluation against held-out test set + baseline comparison.

### 5C. New File: `neural/training/regression_gate.py`

Version comparison: no task regresses >5%, at least 2 tasks improve ≥2%.

### 5D. New File: `neural/training/quantize_deploy.py`

Fuse LoRA adapter and quantize for production:

```bash
python -m mlx_lm.fuse --model mlx-community/Qwen3-30B-A3B \
  --adapter-path .mdemg/neural/models/ft/v1 \
  --save-path .mdemg/neural/models/deployed/v1

# Quantize for inference
python -m mlx_lm.convert --model .mdemg/neural/models/deployed/v1 \
  --quant 4 --save-path .mdemg/neural/models/deployed/v1-q4
```

**Effort:** M-L — 4 Python scripts with evaluation logic.

---

## Phase 6: Recursive Cycle Automation (Python)

### 6A. New File: `neural/training/cycle_runner.py`

Orchestrates the complete recursive cycle with anti-collapse protocol:

```python
def run_cycle(args) -> dict:
    # 1. Check minimum new interaction data
    new_data = collect_new_interactions(data_dir)
    if len(new_data) < min_new_examples:
        return {"status": "skipped", "reason": "insufficient data"}

    # 2. Check exogenous ratio (α ≥ 0.4) — ANTI-COLLAPSE
    anchor = load_anchor_data(data_dir)
    combined = combine_datasets(anchor, new_data, synthetic)
    alpha = compute_exogenous_ratio(combined)
    if alpha < 0.40:
        return {"status": "blocked", "reason": f"exogenous ratio {alpha:.2f} < 0.40"}

    # 3. Check entropy health — ANTI-COLLAPSE
    if not check_entropy_health(current_model, baseline_outputs):
        inject_fresh_exogenous(teacher_model, 200)  # emergency injection
        return {"status": "entropy_alert", "reason": "entropy decay detected"}

    # 4. Stop vllm-mlx, free memory for training
    stop_inference_server()

    # 5. Create versioned dataset with temporal split
    dataset = create_temporal_split(combined, train_end, valid_end)

    # 6. Train (SFT → GRPO → DPO)
    sft_model = train_sft(base_model, dataset)
    grpo_model = train_grpo(sft_model, grpo_data, reward_functions)
    dpo_model = train_dpo(grpo_model, dpo_pairs)

    # 7. Benchmark each stage
    final_bench = benchmark(dpo_model)
    if not regression_gate(previous_bench, final_bench):
        rollback()
        return {"status": "rejected"}

    # 8. Deploy
    quantize_and_deploy(dpo_model)
    restart_inference_server(new_model_path)
    return {"status": "deployed", "version": new_version}
```

### 6B. New File: `neural/training/anchor_manager.py`

Manages anchor dataset. Anchor data is NEVER deleted and is included in every training run.

### 6C. New File: `neural/training/entropy_monitor.py`

Anti-collapse: tracks output entropy across model versions, alerts on >10% decay.

### 6D. New File: `neural/training/dataset_versioner.py`

Assembles datasets with temporal splits, prompt deduplication, exogenous ratio enforcement, and provenance manifests.

### 6E. Dead-Man's Switch *(v2.0 addition)*

```python
MAX_CONSECUTIVE_REJECTIONS = 3

def check_dead_mans_switch(cycle_history):
    """If last 3 cycles all rejected, fall back to external LLM and retrain from base."""
    recent = cycle_history[-MAX_CONSECUTIVE_REJECTIONS:]
    if all(c["status"] == "rejected" for c in recent):
        switch_to_external_llm()
        regenerate_training_data_from_scratch()
        return True
    return False
```

**Effort:** M — orchestration + anti-collapse monitoring.

---

## Phase 7: RSIC Integration (Go)

### 7A. Modify: `internal/ape/self_reflect.go`

New reflection patterns:

| Pattern | Trigger | Action |
|---|---|---|
| 22 | Interaction data ≥ min_new_examples | `trigger_training_cycle` |
| 23 | Benchmark regression detected | `alert_ft_regression` |
| 24 | Benchmark stagnation (3+ cycles no improvement) | `alert_ft_stagnation` |
| 25 | Insufficient training data (<1000 total) | `alert_training_data_low` |
| 26 | Task data imbalance (any task <50 while others >500) | `alert_task_data_imbalance` |
| 27 | Entropy decay (<0.90 ratio) | `alert_entropy_decay` |
| 28 | Exogenous ratio <0.40 | `block_training_cycle` |

### 7B. Modify: `internal/ape/task_dispatch.go`

Add `trigger_training_cycle` handler — calls training pipeline via subprocess.

### 7C. Modify: `internal/config/config.go`

```go
FTEnabled           bool   // FT_ENABLED (default: false)
FTMinNewExamples    int    // FT_MIN_NEW_EXAMPLES (default: 200)
FTModelDir          string // FT_MODEL_DIR (default: ".mdemg/neural/models")
FTCycleIntervalH    int    // FT_CYCLE_INTERVAL_HOURS (default: 168)
```

**Effort:** S-M

---

## Phase 8: CLI Commands (Go)

### 8A. New File: `internal/cli/finetune.go`

```bash
mdemg finetune status          # Model version, interaction count, last cycle
mdemg finetune train           # Run training cycle manually
mdemg finetune eval            # Evaluate current model
mdemg finetune deploy --version 3
mdemg finetune rollback
```

### 8B. New File: `internal/cli/data.go`

```bash
mdemg data status              # Collection rates, storage usage
mdemg data stats               # Per-task counts, date ranges
mdemg data curate              # Run curation pipeline
mdemg data anchor generate     # Teacher distillation
mdemg data quality report      # Quality report
mdemg data quality entropy     # Entropy health check
mdemg data manifest --version v3
```

**Effort:** S-M

---

## Phase 9: Monitoring (Go + Grafana)

### 9A. Modify: `internal/metrics/collectors.go`

```go
// Model metrics
FTModelVersion         func(version string) *Gauge
FTInferenceLatency     func(task string) *Histogram
FTCycleTotal           func(status string) *Counter
FTInteractionCount     func(task string) *Counter

// Data governance metrics
FTExogenousRatio       func() *Gauge
FTEntropyRatio         func() *Gauge
FTDataStorageBytes     func() *Gauge

// Benchmark metrics
FTBenchmarkOverall     func(version string) *Gauge
FTBenchmarkTask        func(task, version string) *Gauge
FTBenchmarkRegression  func() *Counter
```

### 9B. New Grafana Dashboards

- `mdemg-finetune.json` — model version timeline, per-task latency, training cycles
- `mdemg-training-data.json` — collection rates, storage, task distribution, exogenous ratio, entropy

**Effort:** S

---

## Complete File Inventory

### New Files (17)

| File | Phase | Language | Purpose |
|---|---|---|---|
| `internal/llmclient/interface.go` | 1A | Go | `LLMCompleter` interface |
| `internal/llmclient/interaction_logger.go` | 1B | Go | Logging wrapper |
| `internal/llmclient/interaction_collector.go` | 1C | Go | JSONL writer |
| `internal/llmclient/scrubber.go` | 1D | Go | Privacy scrubbing |
| `internal/llmclient/sanitize.go` | 2D | Go | StripThinkBlock + SanitizeResponse |
| `internal/cli/finetune.go` | 8A | Go | Fine-tuning CLI |
| `internal/cli/data.go` | 8B | Go | Data management CLI |
| `neural/training/input_extractor.py` | 4A | Python | Extract inputs from Neo4j |
| `neural/training/teacher_distill.py` | 4B | Python | Anchor dataset generation |
| `neural/training/synthetic_failures.py` | 4C | Python | Failure detection examples |
| `neural/training/quality_filter.py` | 4D | Python | Quality gate filtering |
| `neural/training/format_converter.py` | 4E | Python | Interaction → MLX format |
| `neural/training/train_ft.py` | 5A | Python | LoRA fine-tuning |
| `neural/training/evaluate_ft.py` | 5B | Python | Per-task evaluation |
| `neural/training/regression_gate.py` | 5C | Python | Version comparison |
| `neural/training/quantize_deploy.py` | 5D | Python | Fuse + quantize |
| `neural/training/cycle_runner.py` | 6A | Python | Recursive cycle orchestrator |
| `neural/training/anchor_manager.py` | 6B | Python | Anchor dataset management |
| `neural/training/entropy_monitor.py` | 6C | Python | Anti-collapse monitoring |
| `neural/training/dataset_versioner.py` | 6D | Python | Dataset assembly + provenance |

### Modified Files (22)

| File | Phase | Change |
|---|---|---|
| `internal/llmclient/client.go` | 2A, 2B | Think in CompleteOpts, /think prefix |
| `internal/config/config.go` | 1G, 3C, 7C | Interaction + FT + prompt mode config |
| `internal/api/server.go` | 1F | Wire 15 interaction loggers |
| 15 × LLM consumer files | 1E, 2C, 2E | `*Client` → `LLMCompleter`, Think, SanitizeResponse |
| `internal/ape/self_reflect.go` | 7A | Patterns 22-28 |
| `internal/ape/task_dispatch.go` | 7B | Training cycle handler |
| `internal/metrics/collectors.go` | 9A | FT + data metrics |
| `internal/cli/root.go` | 8 | Register finetune + data commands |

### Files Eliminated (vs v1.0)

| v1.0 File | Reason Eliminated |
|---|---|
| `neural/neural_sidecar/generator.py` | Replaced by vllm-mlx |
| `neural/neural_sidecar/schemas_generate.py` | Replaced by vllm-mlx |
| `internal/llmclient/mlx.go` | Use existing OpenAI provider to vllm-mlx |

**Total: ~20 new + 22 modified = ~42 files** (reduced from 40 in v1.0 by eliminating 3 custom files, adding SanitizeResponse, format retry, prompt compression, and data management)

---

## Implementation Schedule

| Phase | Dependencies | Effort | Duration |
|---|---|---|---|
| **1** Interaction Logger | None | M | 1 week |
| **2** Think Mode + Sanitize | Phase 1 | M | 3-4 days |
| **3** vllm-mlx Integration | None (parallel) | S | 1-2 days |
| **4** Teacher Distillation | Phase 1 data | M | 1-2 weeks |
| **5** Training Pipeline | Phase 4 | M-L | 1-2 weeks |
| **6** Recursive Cycle | Phases 4+5 | M | 1 week |
| **7** RSIC Integration | Phases 1+6 | S-M | 3-4 days |
| **8** CLI Commands | Phases 5+6 | S | 2-3 days |
| **9** Monitoring | Phases 3+7 | S | 2-3 days |

**Critical path:** Phase 1 → Phase 4 → Phase 5 → Phase 6

**First milestone (week 3-4):** Interaction logger capturing all 15 tasks + teacher distillation producing 500+ examples + first LoRA training run on M5 Max + vllm-mlx serving fine-tuned model + MDEMG end-to-end with `LLM_BASE_URL=http://localhost:8100/v1`.
