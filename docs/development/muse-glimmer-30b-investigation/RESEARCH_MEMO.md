# Muse Glimmer 30B — model-swap research memo

**Investigator:** Claude Opus 4.7
**Date:** 2026-08-11
**Trigger:** operator direction "Ollama has new models that we should investigate. One that looks promising is Muse Glimmer 30B."
**Context:** Lever C of `jiminy-follow-rate-decline-2026-08-10` — model-swap angle to evaluate for post-beta.

## Bottom line

**Promising but NOT a drop-in swap.** Muse Glimmer 30B is a Meta Superintelligence Labs release (2026-08-10, one day old) purpose-built for LOCAL AGENTIC workloads. Architecture fits MDEMG's constraint envelope (dense, Apache 2.0, MLX + GGUF available in Ollama, fits M5 Max RAM easily). But it is TRAINED FOR TOOL CALLING — the exact anti-pattern MDEMG's 9 grep-audited banned patterns forbid — and its distinctive strengths (agentic loops, failure recovery, multi-step tool orchestration) don't map onto MDEMG's 16 single-shot LLM call sites (retrieval reranking, constraint classification, guidance synthesis, RSIC reflection).

**Recommended next step:** run the UBENCH suite against a locally-served Muse Glimmer instance in a controlled A/B, using the existing shipped `mdemg-llm-v1` (Qwen3-14B LoRA) as baseline. If UBENCH aggregate matches or beats the shipped baseline on MDEMG's SPECIFIC task set, propose a Phase 14 sprint to migrate. Do NOT flip production without that measurement — Muse Glimmer's published benchmarks (MCP Atlas 75.5, DeepSearch QA 74.6, τ3-Bench, SWE-Bench Pro 51.2) are all AGENTIC evals that don't correspond to MDEMG's call-site shape.

## Model card

| Property | Value |
|---|---|
| Origin | Meta Superintelligence Labs |
| Release date | 2026-08-10 |
| License | Apache 2.0 (permissive, no restriction) |
| Architecture | Dense causal transformer + perception encoder |
| Total params | ~29.6B (incl. ~1.8B vision encoder — 6% of weights) |
| Layers | 52 |
| Hidden dim | 6,656 |
| Attention heads | 32 Q / 2 KV (GQA 16:1 — aggressive KV compression) |
| Head dim | 128 |
| FFN | SwiGLU, intermediate 19,968 |
| Attention pattern | `[Local, Local, Local, Global]` repeating |
| Sliding window | 2,048 |
| Context length | 131,072+ |
| Vocab | 202,048 BPE |
| RoPE θ | 500,000 (local layers only) |
| Speculative decoding | DFlash drafter (5 layers, block 16, 3.1× RTX 5090 / 1.5-1.8× Apple Silicon) |

## Distribution

**Ollama library** (matches MODEL-DIST-001 pipeline):
- `muse-glimmer:latest` / `muse-glimmer:30b` — 18 GB GGUF, 128K ctx
- `muse-glimmer:30b-mlx` — 21 GB MLX, 128K ctx (Apple Silicon optimized)
- `muse-glimmer:30b-q4_K_M` — 4-bit GGUF

**Unsloth GGUF variants** (via `huggingface.co/unsloth/Muse-Glimmer-30B-GGUF`):

| Quant | Size | Notes |
|---|---|---|
| UD-IQ2_XXS → UD-Q3_K_XL | 10.7 – 14.1 GB | 2-3-bit (aggressive) |
| **UD-Q4_K_XL** | **15.9 GB** | **4-bit recommended** |
| UD-Q5_K_M | 19.2 GB | 5-bit (equivalent tier to current mdemg-llm-v1) |
| UD-Q5_K_XL | 21.8 GB | |
| UD-Q6_K_XL | 26.3 GB | |
| Q8_0 | 29.6 GB | |
| BF16 | 55.7 GB | full precision |

Fits M5 Max 128 GB RAM easily at any quant. Unsloth Dynamic 2.0 claims ≤1% degradation across 15 benchmarks vs full precision.

**Published latency on Apple Silicon** (from Meta blog):
- M4 Max: 23.7 → 37.8 tok/s (with DFlash)
- M5 Max: 26.6 → **50.2 tok/s** (with DFlash)

Compare to current `mdemg-llm-v1.Q5_K_M.gguf` served via llama-server on M5 Max: median call latency ~3s, retrieval-rerank p95 ~11s per LLM-HEALTH-INVESTIGATION-001. Direct throughput comparison requires an actual serve run.

## MDEMG compatibility — fit-check

### Green

- **Dense architecture** — avoids the Metal 499K MTLResource ceiling that killed the Qwen3.6-35B-A3B MoE→dense pivot per `project_phase5_moe_pivot`. No architectural blocker.
- **Apache 2.0** — no license issue for shipping via `mdemg model pull` (MODEL-DIST-001 supports arbitrary Ollama library models).
- **MLX + GGUF availability** — matches the current shipping shape (`mdemg-llm-v1` ships as MLX for research + GGUF Q5_K_M for production llama-server).
- **Ollama-native** — plugs into MODEL-DIST-001's fetcher pipeline out of the box.
- **131K context** — MDEMG's ape.reflect prompt budget (~7489 tokens live per APE-PROMPT-BUDGET-001) fits comfortably. No prompt-truncation regression risk.
- **GQA 16:1** — KV cache is small; 4× parallel serving (matching current llama-server `--parallel 4 --ctx-size 32768`) is inexpensive.
- **BF16 base available** for LoRA fine-tuning if we ever want to bake in MDEMG-specific behavior (the FT recursive-loop shipped path).

### Yellow

- **Trained for TOOL CALLING** — Meta's whole pitch is agentic loops with failure recovery. MDEMG's architectural policy (see CLAUDE.md `Standing policies` §1) BANS tool calling across all 16 call sites; nine patterns grep-audited every sprint. Muse Glimmer's chat template supports single-turn structured output without tools (Meta blog example shows `apply_chat_template()` with `reasoning_strength="low"`), but the model was TRAINED to emit tool-call sequences. Risk: it may generate spurious `tool_call`-shaped outputs even when we don't advertise tools, requiring output-post-processing or prompt engineering that adds fragility.
- **Multimodal (1.8B ViT-G/14 vision encoder)** — MDEMG is text-only. The vision encoder is ~6% of the weight budget that we'll never use — inefficient but not disqualifying. Text-only inference should skip the ViT.
- **30B vs Qwen3-14B (2.1× params)** — memory pressure higher (Q5_K_M 19.2 GB vs Qwen3-14B Q5_K_M ~10 GB). M5 Max 128 GB RAM handles it, but running side-by-side with the incumbent during A/B needs care.
- **Distilled from Muse Spark** (undocumented teacher). Quality inheritance depends on the teacher; unknown until we run our own eval.
- **LoRA target modules unpublished** — HF card says "LoRA SFT, BF16" is a supported workload but no recommended target-module list. Would need to derive from architecture (likely `q_proj, k_proj, v_proj, o_proj, gate_proj, up_proj, down_proj` per standard MLX-LM LoRA on GQA models) and validate empirically.
- **Sampling recommendations are AGENTIC-tuned** (temperature 1.0, top_p 0.95, top_k 64). MDEMG's structured-output tasks may want lower temperature (0.0-0.2). Need to override per-task; not a blocker.

### Red

None. The tool-calling training concern is a caveat, not a blocker — the model can be used single-shot with no tools advertised; production risk is bounded.

## Benchmark landscape

Meta published benchmarks vs `Gemma4-31B` and `Qwen3.6-27B` (both LARGER than Muse Glimmer's peer class — asymmetric comparison favoring Meta) but **NOT vs Qwen3-14B** (MDEMG's incumbent).

Muse Glimmer 30B headline numbers (high-reasoning config):

| Benchmark | Score | Class |
|---|---|---|
| AIME 2026 | 94.7 | reasoning ★ |
| AA-LCR | 80.0 | long-context reasoning |
| Charxiv Reasoning | 78.8 | chart-question reasoning |
| IFBench | 77.0 | instruction following ★ |
| MCP Atlas | 75.5 | agentic |
| DeepSearch QA | 74.6 | agentic search |
| Beam128K | 65.1 | long-context |
| Siren AgentDojo (utility) | 94.2 | agentic |
| SWE-Bench Pro | 51.2 | agentic coding |
| WildClawBench | 47.6 | multi-turn |
| Gaia2 | 43.3 | multimodal |
| τ3-Banking | 23.5 | tool-use |

★ = relevant to MDEMG's call shape (reasoning + instruction following). The other 10 are agentic evals; not predictive of MDEMG performance.

## Migration path (if we decide to swap)

**Phase 1 — pull + serve** (~1 hour):
```
mdemg model pull --backend ollama --name muse-glimmer --quant 30b-mlx
# or GGUF path:
ollama pull muse-glimmer:30b-q4_K_M
llama-server --model <blob-path> --port 8103 --ctx-size 32768 --parallel 4 --cont-batching
```
Use port 8103 to avoid conflict with the incumbent's :8102 llama-server.

**Phase 2 — UBENCH aggregate** (~30 min):
```
LLM_ENDPOINT=http://127.0.0.1:8103/v1 LLM_MODEL=muse-glimmer:30b python -m neural.benchmarks.run_benchmark \
  --config configs/benchmark_phase10.yaml --out training_data/eval/benchmark_muse_glimmer.json
```
Publish alongside the shipped `mdemg-llm-v1` baseline for direct task-by-task comparison. Look for regressions on the 3 highest-value tasks: `retrieval.rerank_cross`, `consulting.classify`, `jiminy.synthesize` (the JIMINY-CEILING-INVESTIGATION-001 shortlist).

**Phase 3 — UVTS 120q A/B** (~4-6 hours):
```
python3 docs/tests/uvts/runners/uvts_runner.py --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json \
  --base-url http://localhost:9999 --profile full --persist-tsdb \
  --env-override LLM_ENDPOINT=http://127.0.0.1:8103/v1 LLM_MODEL=muse-glimmer:30b
python3 docs/tests/uvts/runners/uvts_ab_compare.py --baseline mdemg-llm-v1.grades.json --candidate muse-glimmer.grades.json --out verdict.json
```
Apply Note 02 strict merge gate: candidate mean ≥ baseline mean AND no per-question regression > 0.10.

**Phase 4 — decision** (~1 hour):
- Pass → propose Phase 14 model-swap sprint (rebuild adapter, update MODEL-DIST-001 default, refresh Phase 10 benchmarks, update MODEL_UPDATE docs).
- Fail → close as evaluated-not-selected; document in this dir; move to next candidate.

**Phase 5 (optional, if base wins) — LoRA-tune on MDEMG corpus** (~1-2 days):
Follow the FT recursive-loop shipped path (`docs/features/ft-recursive-loop.md`) — export corpus → curate → train MLX LoRA → convert to GGUF → run gate benchmark → promote.

## Cost estimate

| Phase | Effort |
|---|---|
| P1 pull + serve | ~1 hour |
| P2 UBENCH baseline vs candidate | ~30 min |
| P3 UVTS 120q A/B | ~4-6 hours |
| P4 verdict + write-up | ~1 hour |
| **Total (research-only, no swap)** | **~1 day** |
| P5 LoRA retrain (if swap approved) | ~1-2 days additional |

## Recommendation

**Do not act on the model swap until beta pipeline is fully stable**, per the user's earlier standing directive: "Once we fully complete all beta-testing functionality, pipelines, etc.. I want to review currently avalible models to see if we should look at retraining using a newer LLM." Beta pipeline landed today; give it a week of live traffic to shake out issues before mixing in a model swap.

**Queue this as a formal sprint** `MODEL-SWAP-MUSE-GLIMMER-EVAL-001` scoped to P1–P4 above. Ships a verdict document + benchmarks, NOT a production swap. If verdict is positive, a separate `MODEL-SWAP-MUSE-GLIMMER-DEPLOY-001` sprint handles the migration path (default flip, docs refresh, adapter rebuild).

## Risks

**R1: Muse Glimmer's tool-calling training produces spurious tool-call output on structured-output prompts.** Verified during P2 (any UBENCH task whose validator sees `tool_call` / `function_call` / `{"tool_use":`-shaped output fails). Mitigation: strip common tool-call schemas from prompts; if that doesn't help, model is a non-fit.

**R2: Benchmark asymmetry.** Meta's published wins are on AGENTIC evals (MCP Atlas, DeepSearch, SWE-Bench Pro) that don't map to MDEMG. The absence of a direct Qwen3-14B comparison is itself informative. Do not trust Meta's headline; run our own UBENCH.

**R3: 30B is ~2× the params of Qwen3-14B → higher latency per call.** Even with DFlash speculative decoding, larger model means slower serve. If UBENCH shows p95 rerank_cross latency worse than the shipped baseline's 11.1s → net regression regardless of quality lift.

**R4: MLX LoRA support on Muse Glimmer unverified.** If the P5 retrain path is required for shipping (i.e. base-model quality alone isn't enough), the FT recursive-loop shipped path needs to be validated end-to-end against Muse Glimmer's architecture (52 layers, GQA 32/2). Test the pipeline on a tiny subset before committing to a full retrain.

## Open questions (deferred to the eval sprint)

1. Exact GGUF chat template — does the Jinja require a `tools` field even when empty? If so, MDEMG's llmclient needs updating to send `tools: []`.
2. Is `reasoning_strength="low"` the right setting for MDEMG's structured-output tasks, or should we use `"high"` for retrieval reranking?
3. Does Muse Glimmer's DFlash drafter conflict with llama.cpp's `--parallel 4 --cont-batching` setup? Runtime testing required.
4. Does Meta publish a smaller variant (7B/13B) that would compare more fairly to Qwen3-14B on parameter count? Search returned only the 30B; needs verification.

## Sources

- [Meta Superintelligence Labs: Introducing Muse Glimmer](https://research.meta.ai/blog/introducing-muse-glimmer-open-agentic-model)
- [Meta Publishes Muse Glimmer As 30B Open Agentic Model (Phoronix)](https://www.phoronix.com/news/Meta-Muse-Glimmer)
- [Muse Glimmer from Meta Superintelligence Labs is now available (Ollama Blog)](https://ollama.com/blog/muse-glimmer)
- [meta-models/Muse-Glimmer-30B (HuggingFace)](https://huggingface.co/meta-models/Muse-Glimmer-30B)
- [unsloth/Muse-Glimmer-30B-GGUF (HuggingFace)](https://huggingface.co/unsloth/Muse-Glimmer-30B-GGUF)
- [Meta is back with Muse Glimmer: local, agentic, multimodal, and open source (HuggingFace blog)](https://huggingface.co/blog/muse-glimmer)
- [Meta Muse Glimmer 30B: The New Mid-Sized LLM King (Medium)](https://medium.com/data-science-in-your-pocket/meta-muse-glimmer-30b-the-new-mid-sized-llm-king-c7aae687783a)
- [muse-glimmer library page](https://ollama.com/library/muse-glimmer)

## Related MDEMG documents

- `docs/development/jiminy-follow-rate-decline-2026-08-10/INVESTIGATION.md` — Lever C hint that motivated this evaluation
- `docs/development/jiminy-heuristic-default-001/post.md` — Lever A of the same arc; complementary, not competing
- `docs/features/local-model-distribution.md` — MODEL-DIST-001 pipeline that would ship this model if approved
- `docs/features/ft-recursive-loop.md` — FT retrain pipeline for P5
- `memory/project_phase5_moe_pivot.md` — the Metal 499K MTLResource lesson (avoids a class of failure modes for dense models)
- `docs/development/ft-lora/00_README_v2.md` — canonical FT plan (currently ships `mdemg-llm-v1` on Qwen3-14B)
- CLAUDE.md `Standing policies` §1 — no-tool-calling architectural policy (the caveat for Muse Glimmer's agentic training)
