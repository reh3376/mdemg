# How Jiminy Uses LoRA on Qwen to Run

**Audience**: engineer or peer AI onboarding to the MDEMG LLM stack.
**Scope**: the production path from Qwen3-14B base + LoRA adapter → GGUF Q5_K_M → llama-server at :8102 → Jiminy's 16 call sites.
**Status reference**: CLAUDE.md § "MDEMG Fine-Tuning — Shipped State & Policies" is canonical; this doc is the engineering-level narrative for that pin.

---

## 1 What Jiminy actually needs from an LLM

Jiminy is MDEMG's **enforcement + classification layer** for durable constraints. Every durable rule in the substrate (Neo4j `role_type IN ['constraint','correction']` nodes, ~215 live on `mdemg-dev`) has to be:

1. **Classified** — given an agent action, did it `follow` / `ignore` / `partial_comply` / `contradict` / `not_applicable` the rule? (`internal/jiminy/outcome_classifier.go`)
2. **Synthesized** — given a set of surfaced constraint nodes, generate a short imperative-shaped guidance narrative the agent will read as the first thing in the next prompt. (`jiminy.synthesize`, `internal/jiminy/service.go`)
3. **Reformulated** (/strict mode) — rewrite the surfaced constraints as deterministic imperatives rather than advisory prose. (`/v1/jiminy/reformulate`)
4. **Evaluated** (`handleJiminyClassify`) — on every Write/Edit/Bash via `pre-write-check.py` / `pre-bash-check.py`, decide if the proposed action violates an escalated constraint. This is the surface that produces `decision: deny` denials.

All four surfaces, plus 12 other MDEMG call sites (`ape.reflect`, `consulting.classify`, `retrieval.rerank_cross`, intent translation, SME synthesis, context cooler scoring, etc.), run **single-shot structured-output** — no tool-calling, no multi-turn. Nine patterns are grep-audited every sprint (`tool_use`, `tool_call`, `function_call`, `--tool-call-parser`, `enable-auto-tool-choice`, etc.) to keep this invariant.

What this means for the LLM: it must be fast, locally-hosted (no network dependency on the hot path), bounded (KV cache can't runaway), and reliably follow strict JSON schemas on constraint-classification outputs. **Qwen3-14B tuned via LoRA on MDEMG-authored data** is the shipped answer.

## 2 The base model

Shipped base: **`mlx-community/Qwen3-14B-4bit`** — a 14-billion-parameter dense transformer, pre-quantized to 4-bit for MLX (Apple Silicon's deep-learning runtime).

Why this base, not something else:

- **Dense, not MoE**. The original Phase 5 plan targeted Qwen3.6-35B-A3B MoE for its routing-expert efficiency. On 2026-04-22, dropped to dense Qwen3-14B after the **Metal 499K MTLResource ceiling** on M5 Max + macOS 26 blocked every non-trivial MoE LoRA backward pass. This is a hardware-architectural limit, not a quant-specific bug — no user-space override exists. The MoE path is permanently superseded; do not plan against it.
- **4-bit quant from upstream** (`mlx-community`). The adapter trains on this exact base so inference and training share quantization — no cross-quantization gradient drift.
- **14B parameters hits the sweet spot** for M5 Max: large enough to produce useful imperative directives + classify constraint-outcome nuance; small enough that KV cache + LoRA weights fit in 24GB working set with 4 parallel slots.

## 3 The LoRA adapter

Jiminy's "brains" are not the base — they're the LoRA adapter trained on MDEMG-specific tasks.

- **Rank**: 32
- **Alpha**: 64 (α/r = 2.0 — standard "amplify" ratio)
- **Target modules**: 7 dense modules per transformer block (query/key/value/out projections + MLP gates)
- **Dropout**: default MLX LoRA settings
- **Epochs**: hard cap at 3 (overfitting-prevention forcing function, pinned after FT-OAI-001's step-1200 overfit in `training_data/openai_ft/20260420/run_notes.md`)
- **Early-stop**: `val_loss > best × 1.05` for 2 consecutive evals (SFT) or `val_reward < best × 0.95` (RL)
- **`n_epochs=auto` is DISALLOWED** — explicit integer epoch counts only

The training corpus is **MDEMG-authored data from the shipped system itself**: `training_data/sft/` holds the SFT rows (claude_code_knowledge families + TASK-behavior rows), `training_data/openai_ft/` is the OpenAI-teacher parallel (used for distillation, now superseded for LoRA runs), and the recursive-retraining loop (`internal/ftloop`) sources from live `guidance_training_rows` TSDB rows that `/v1/jiminy/feedback` writes on every agent action.

Phase 5 SFT ran the dense adapter against the 14B base. Later phases:
- **Phase 11.x** — RL and eval harness (GRPO orchestrator in `neural/training/rl/`, DPO pairs in `neural/training/dpo/`)
- **Phase 11.5c** — honest clean eval `valid_clean.jsonl` (180 rows, 0% leakage, replacing the 99%-leaked `valid_golden.jsonl`)
- **Phase 11.6 cutover** — the Phase-5 adapter won the 16-task augmented bake-off against `gpt-5.4-mini` (0.8389 vs 0.8317)
- **Phase 13.5 runtime cutover** — moved the runtime off `mlx_lm.server` (per-14-min OOM crashes) onto `llama.cpp llama-server` (bounded KV, HTTP-stable on OOM)
- **APE-REFLECT-EVAL-REFRESH-001 (2026-07-21)** — refreshed truncation-poisoned `ape.reflect` eval rows; current honest aggregate is **0.9188** on 16 tasks

## 4 The conversion pipeline — MLX → GGUF

LoRA training lives in MLX (Apple Silicon-native, fastest training path on M5). Inference lives in `llama.cpp` llama-server (production-grade, bounded KV). The conversion is a three-step pipeline that runs once per retrain, orchestrated by the `internal/ftloop` controller:

```
MLX safetensors (28 GB fp16 fused)
   │
   │  mlx_lm.fuse --dequantize
   ▼
HF safetensors (bf16, 28 GB)
   │
   │  convert_hf_to_gguf.py --outtype f16
   ▼
GGUF f16 (29.5 GB)
   │
   │  llama-quantize Q5_K_M
   ▼
GGUF Q5_K_M (10.5 GB — shipped as mdemg-llm-v1)
```

Four quant tiers are published to Ollama Library (`reh3376/mdemg-llm-v1`):
- **Q4_K_M** (9.0 GB, 12 GB RAM minimum)
- **Q5_K_M** (10.5 GB, 14 GB RAM — **production canonical**)
- **Q8_0** (16 GB, 20 GB RAM)
- **Adapter-only GGUF LoRA** (257 MB, applies on top of base Qwen3-14B — for operators who already pulled the base elsewhere; shipped MODEL-DIST-002)

Key constraint: the 4bit-fused MLX adapter CANNOT `convert_hf_to_gguf` directly. The pipeline MUST route through `mlx_lm.fuse --dequantize` to bf16 safetensors first. The recursive-retraining loop (FT-RECURSIVE-002 Epic 6) learned this live and now enforces it as a pipeline stage.

## 5 Serving — llama-server at :8102

Production invocation (launchd plist at `~/Library/LaunchAgents/com.mdemg.llama-server.plist`):

```
llama-server \
  --model /Users/reh3376/mdemg/.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf \
  --port 8102 \
  --host 127.0.0.1 \
  --ctx-size 32768 \
  --parallel 4 \
  --cont-batching \
  --metrics \
  --jinja
```

Why these flags:

- `--ctx-size 32768 --parallel 4` → **8K per slot** bounded KV. This is the architectural fix that replaced `mlx_lm.server`'s unbounded KV → Metal OOM → SIGABRT crash loop.
- `--cont-batching` → continuous batching so 4 parallel Jiminy calls (consulting.classify, synthesize, reformulate, outcome_classify concurrent) share throughput.
- `--jinja` → Qwen3's chat template is Jinja; without this, system prompts aren't honored and the LoRA adapter's trained behaviors never trigger.

⚠️ **The 8K per-slot bound caps prompt + output together.** A large prompt starves generation. `ape.reflect` prompts grew to ~7489 tokens live, leaving ~700 tokens for output → 87% mid-JSON truncation. Fixed by APE-PROMPT-BUDGET-001 (`internal/ape/llm_reflector.go::buildUserPrompt` now bounds the reflection prompt to `RSIC_LLM_REFLECT_PROMPT_BUDGET_TOKENS` default 3500 via dataset gating + history cap + drop-oldest guard). When adding new LLM call sites whose prompts grow with state, bound the prompt — the KV slot is shared by input and output. This is the single most-common footgun on this runtime.

## 6 Reliability — the watchdog + preflight

MDEMG refuses to start if llama-server is unreachable (`internal/cli/preflight_mlx.go` — the name still references MLX; the probe is backend-agnostic). Escape hatch: `MDEMG_ALLOW_NO_LLM=1`.

The watchdog (`internal/mlxprobe`) probes `GET /v1/models` every 5s (`LLM_PROBE_INTERVAL_SEC`) with 2s timeout. State machine: `up → degraded → down` with hysteresis. On `down`, a fast-fail gate at `llmclient/client.go` short-circuits retries (`ErrMLXDown`) so a down-endpoint doesn't produce a retry storm against the already-dead server.

Operator surface: `mdemg watchdog status [--json]`.

## 7 Where Jiminy specifically consumes this LLM

Jiminy's main call sites (16 total; naming follows `llm_interactions.task_name` TSDB column):

| Call site | What it does | Prompt file / entry point |
|---|---|---|
| `jiminy.classify_outcome` | For each surfaced constraint × agent action pair, classify followed/ignored/partial/contradict/NA. Includes `nonviolation_credit`, `context_mismatch_credit`, `mechanism_scope_credit`, `mention_vs_perform_credit` clauses — all gated by env flags that compose additively (`JIMINY_NONVIOLATION_CREDIT_ENABLED`, etc.). | `internal/jiminy/outcome_classifier.go::resolveClassifySystemPrompt` |
| `jiminy.synthesize` | Short imperative narrative from a surfaced-constraint pool. Fire-and-forget on the warm path; 50s typical latency. | `internal/jiminy/service.go::synthesizeNarrative` |
| `jiminy.reformulate` (/strict only) | Convert advisory prose into imperative directives. | `/v1/jiminy/reformulate` handler |
| `jiminy.evaluate_llm` | On every Write/Edit/Bash, decide if a WARNED+ constraint is violated. Returns `verdict: deny` with constraint_code + reason. | `handleJiminyClassify` → `StrictClassifier.Classify` |
| `jiminy.generate_constraint_code` | Mint a mnemonic constraint code (e.g. `never-haiku-for-planning`) when a new constraint node is first created; fall back to `auto-<hash>` on LLM failure. | `internal/jiminy/service.go::BootstrapCodes` |
| `jiminy.detect_missed_violation` | On every observed correction (post-hoc), vector-match the correction's embedding against the constraint partition. Writes `missed_violation` outcome rows (ENFORCE-005). | `Service.DetectMissedViolation` |

Also consumed by Jiminy's periphery:
- `consulting.classify_constraint` — called during warm compute to decide which retrieved constraints are applicable to the agent's current query
- `ape.reflect` — RSIC self-reflection; sees aggregated metrics + proposes new insights (gated by `AllowedLLMActions`)
- `retrieval.rerank_cross` — cross-encoder rerank, run after RRF column voting; p95 ~11s, min-budget gate at `RERANK_MIN_BUDGET_MS` default 12s

Each site has its own TSDB task_name, latency SLO, timeout budget, and ULTS prompt-hash pin — any drift to the system prompt bumps the hash in `docs/tests/ults/specs/<task>.ults.json` and CI blocks the merge until re-pinned.

## 8 The recursive-retraining loop

The shipped `internal/ftloop` controller consumes the ledger-event `trigger_training_pipeline` and walks `export → curate → train → convert → gate` autonomously. Opt-in gated via `FT_LOOP_ENABLED=false` default. Phase 7 added a three-layer promotion defense (pre-swap canary → fail-closed `SwapServing` symlink → post-swap tripwire auto-rollback). Phase 9 added production drift detection + a weekly scheduled benchmark runner.

Flow when enabled:

```
LLM behavior drift detected
   │  (via ft_production_drift gauge, FT_DRIFT_MARGIN 0.05)
   ▼
RSIC pattern 31 emits `production_drift_detected`
   │
   │  Gate.EvaluateTrigger reserves cycle slot (ledger single-flight)
   ▼
controller walks curate → train → convert → gate
   │
   │  gate = benchmark against clean eval
   │  PASS → promote_pending (operator confirms)
   │  FAIL → file class-5 issue, roll back
   ▼
operator confirms N times (FT_LOOP_AUTO_PROMOTE_AFTER=0 means never)
   │
   ▼
SwapServing: pre-swap structural canary → symlink flip → tripwire monitor
```

Base model + adapter re-training IS the standard workflow for making Jiminy's classifier smarter. The HITL-curation substrate (operator grading via `/v1/review/*`) feeds high-quality training rows into the loop; the loop reads those rows automatically on retrain.

## 9 Why this architecture matters for Jiminy specifically

Jiminy is the ONE MDEMG surface where LLM behavior directly becomes **operator-blocking enforcement**. If the classifier calls `follow` on an action that is actually `ignore`, agents learn to ignore constraints as a class. If it calls `deny` on legitimate work, operators have to override repeatedly and lose trust in the gate.

The three load-bearing properties of this stack:

1. **Local + fast** — Jiminy's hot path cannot have a network dependency. 50ms added latency on `pre-write-check.py` becomes 50ms × N tool calls per session; operators notice. Local llama-server at :8102 keeps the enforcement surface responsive.
2. **Bounded KV** — `--ctx-size 32768 --parallel 4` means Jiminy's enforcement can be relied on to not OOM-crash the entire LLM substrate under load. Pre-Phase-13.5 `mlx_lm.server` crashed every 14 minutes; enforcement vanished for 30+ seconds per crash. llama.cpp with bounded KV stays HTTP-alive on OOM (returns HTTP 500 gracefully).
3. **LoRA on domain-specific data** — generic Qwen3-14B can read a constraint and roughly guess outcome. MDEMG's LoRA was trained on the actual (constraint, action, outcome) triples the system records; aggregate score on the honest eval moved from 0.80 → 0.9188 after Phase 5 SFT + prompt-budget fixes. That delta is the difference between "misclassifies 20% of operator corrections" and "misclassifies 8%" — the enforcement gate's reliability floor.

## 10 References

- `docs/development/ft-lora/00_README_v2.md` — canonical FT plan + full history (v5.12+)
- `docs/development/ft-lora/phase_5_sft_post.md` — MoE→dense pivot rationale
- `docs/development/post-ft-lora/phase_13_5_bakeoff_results.md` — mlx_lm.server → llama-server data-decided cutover
- `docs/features/ft-recursive-loop.md` — the autonomous retrain loop (Phase 6a/6b/7/9)
- `docs/features/local-model-distribution.md` — Ollama Library publishing (fused + adapter-only paths)
- `docs/features/jiminy-strict.md` — enforcement surface (how classifier calls become denials)
- `docs/features/jiminy-metric-partition.md` — the four verifiability-class gauges + how each is graded
- `docs/features/rsic-reflection-prompt-budget.md` — the KV-slot prompt-starvation fix (APE-PROMPT-BUDGET-001)
- `internal/jiminy/outcome_classifier.go` — classifier system prompt (ULTS-pinned)
- `internal/ftloop/` — recursive-retraining controller
- `internal/mlxprobe/` — endpoint watchdog
- `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` — shipped production weight (10.5 GB)
