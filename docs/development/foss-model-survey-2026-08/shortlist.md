# FOSS Base-Model Survey 2026-08 — Shortlist for Operator Pick

**Task**: #91 reframed 2026-08-16 ("MUSE-GLIMMER was just a suggestion — research current FOSS models to ensure we select well").
**Method**: two parallel Fable-5 research agents surveyed the 2026 open-weights landscape against MDEMG's hard filters.
**Full reports**: `dense_14b_to_40b.md` (general 14-40B) + `reasoning_and_coding.md` (reasoning-first + coding-specialized).

## Both surveys converged on the same top candidate — Qwen 27B dense

Every non-Qwen-dense candidate fails on one of three structural axes:
1. **MoE hits the Metal 499K MTLResource ceiling** on M5 Max/macOS 26 — proven in-house 2026-04-22, killed the Qwen3.6-35B-A3B pivot. Any MoE = inference-only, breaks the FT-RECURSIVE retrain loop.
2. **Function-calling-trained models leak `{"tool_call": ...}`** on structured-output prompts even when no tools are advertised — violates MDEMG's 9-pattern grep-audited no-tool-call policy. Applies to Gemma 4 (native function-calling), MUSE-GLIMMER (agentic-first), Nemotron 3.
3. **Custom licenses** — Nemotron OpenModel license, EXAONE non-commercial, Codestral non-production, NVIDIA license carve-outs. Only Apache 2.0 / MIT survives the filter.

## The two viable Qwen 27B dense options

| Dimension | **Qwen3.8-27B** | **Qwen3.6-27B** |
|---|---|---|
| Released | 2026-08-13 (3 days ago) | 2026-04-22 (~4 months mature) |
| License | Apache 2.0 | Apache 2.0 |
| Size | 27B dense | 27B dense |
| Context | 262K native | 262K native |
| GGUF availability | Landing (community conversions in-flight) | Full ecosystem (official GGUF + MLX, Ollama `batiai/qwen3.6-27b`) |
| LiveCodeBench v6 | **90.3** | ~84 (family-level cited by trackers) |
| SWE-bench Pro | 61.7 | not yet published |
| IFEval | Not yet published | Not yet published (Qwen3.5-27B is 95.0 for reference) |
| MDEMG lineage match | ✅ Same chat template + tokenizer as incumbent | ✅ Same chat template + tokenizer as incumbent |
| FT-RECURSIVE loop compat | ✅ Dense — Metal LoRA blocker N/A | ✅ Dense — Metal LoRA blocker N/A |
| Q5_K_M est. footprint | ~19 GB | ~19 GB |
| Risk | 3-day-old release — quantized-runtime maturity + agentic-benchmark tilt need bake-off before trust | 4-month track record, ecosystem stable, benchmarks aggregate through 3rd-party trackers only |

## Operator pick options

The two Fable-5 researchers converged independently on **Qwen3.8-27B as primary, Qwen3.6-27B as safer runner-up**. The operator has three coherent paths:

### Option A — Ship Qwen3.8-27B (aggressive)
- Best-published benchmarks (LiveCodeBench v6 90.3, SWE-bench Pro 61.7)
- Newest weights (theoretical peak of Qwen line as of Aug 2026)
- **Risk**: 3-day-old release. Community GGUF conversions may still have bugs. Reasoner may be needed for tokenizer/template edge cases discovered during bake-off. Pre-adoption gate: fresh 16-task UBENCH run with `finish_reason=length` checks; watch for tool-call leakage.

### Option B — Ship Qwen3.6-27B (conservative)
- 4-month ecosystem maturity (official MLX + Ollama + LM Studio all working)
- Same dense architecture, same Qwen chat template family
- Slightly older benchmarks but similar structural fit
- **Risk**: leaves 6 months of Qwen quality gains on the table

### Option C — Bake-off both (rigorous)
- Serve each on `:8103` / `:8104` side-ports (production `mdemg-llm-v1` stays on `:8102`)
- Run UBENCH aggregate + UVTS 120q A/B against each + baseline
- Pick winner based on empirical MDEMG-fit, not published benchmarks
- **Cost**: ~1 day (P1 pull + serve each ~30min; P2 UBENCH aggregate ~1h; P3 UVTS 120q A/B ~4-6h; P4 verdict)
- **My recommendation** if operator wants the highest-confidence answer

## What we ruled out (verified in both reports)

| Excluded | Reason |
|---|---|
| **MUSE-GLIMMER 30B** (Meta, Apache) | Agentic-tool-loop trained — leakage risk on structured-output sites |
| **Gemma 4 31B** (Google, Apache) | Native function-calling trained + community-reported MLX bugs; different chat template requires 16-ULTS-spec rework |
| **Mistral Small 4** (119B-A6.5B, Apache) | 82 GB resident + MoE-LoRA Metal blocker; excellent `reasoning_effort=none|low|high` request param is compelling but structurally disqualified |
| **DeepSeek-R2** | Never shipped |
| **DeepSeek-V4-Flash** | 284B — doesn't fit 128 GB |
| **Qwen3-Coder-Next** (80B-A3B, Apache) | MoE (Metal blocker) + tool-call-first + coding-only profile bad for jiminy.synthesize prose |
| **Qwen3.6-35B-A3B** (Apache) | MoE — this is EXACTLY the model MDEMG's 2026-04-22 pivot proved untrainable on Metal |
| **Nemotron 3 Nano 30B-A3B** | NVIDIA Open Model License (not Apache/MIT) + MoE + agentic-AI-first |
| **gpt-oss-20B/120B** (OpenAI) | Pre-2026 (Aug 2025); Harmony-format + tool-use-centric |
| **GLM-5.1/5.2** (Z.ai) | 754B / 40B active — outside size envelope |
| **Devstral 2** (Mistral, agent-first) | Agent-scaffold trained; misses release window by 3 weeks |
| **Codestral 25.x** | Non-production license |
| **StarCoder3** | Doesn't exist |
| **EXAONE 4.5** | Non-commercial license |
| **Anthropic Claude models** | Closed-source, API-only — structurally ineligible for local-first substrate |

## Recommended next step

Ask operator to pick **A / B / C**. On C selection, MODEL-SWAP-EVAL-002 sprint kicks off using the shipped methodology from `docs/development/model-swap-muse-glimmer-eval-001/sprint_plan.md` (side-port serve, UBENCH aggregate, UVTS 120q A/B, verdict.md).

Pre-requisites confirmed live 2026-08-16:
- ✅ Pick-up gates all clear (0 CRITICAL/HIGH alerts, beta pipeline stable, follow-rate stable)
- ⚠️ Ollama upgrade needed (currently 0.32.4; brew has 0.32.13) — quick fix at execution time
- Neural sidecar + llama-server + Neo4j + TSDB all reachable
