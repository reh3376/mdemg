# CLAUDE-DOCS-TRAINING-004 — Sprint Post

**Shipped**: 2026-08-17 19:15 UTC (Epics 1-6 complete)
**Verdict**: **DO NOT PROMOTE.** adapter_002 is **−0.13 WORSE than baseline** (0.3787 → 0.248-0.264 overall) on the full 50-row `claude.code_knowledge` holdout, regardless of `repetition_penalty=1.15` setting.

## 1. What shipped

**Epic 1 — MLX → GGUF LoRA conversion** (SHIPPED, then verified inapplicable):
- `adapters/claude_docs_002/adapters.safetensors` (MLX, 168 MB) → `.local-models/claude_docs_002_peft/adapter_model.safetensors` (PEFT, 168 MB) → `.local-models/claude_docs_002.gguf-lora` (GGUF, 168 MB, 320 tensors)
- Used llama.cpp master `convert_lora_to_gguf.py` (b10450) because vendored b9000 script depends on `convert_hf_to_gguf` which requires the full llama.cpp source tree via PYTHONPATH

**Epic 2 — Load via llama-server side-port** (SHIPPED, then bypassed):
- Loaded `.local-models/claude_docs_002.gguf-lora` on side-port llama-server `:8106` with production `mdemg-llm-v1.Q5_K_M.gguf` as base + `--flash-attn on --parallel 1`
- Server accepts + reports `lora` in model list. But 3-row smoke showed **adapter has NO effect** on the FT base (see Epic 3).

**Epic 2b (unplanned) — Pivot to mlx_lm.server + raw base** (SHIPPED):
- `mlx_lm.server --model .local-models/qwen3-14b-4bit-base --adapter-path adapters/claude_docs_002 --port 8107`
- This is the training-native config (adapter trained on raw base) — recovers adapter behavior

**Epic 3 — 3-row hand smoke** (SHIPPED, PASSED via mlx path):
- Golden #2 EffortLevel: `low | medium | high | xhigh | max` — **5/5 EXACT** (matches Sprint 003 result)
- Golden #1 query() vs ClaudeSDKClient: Coherent Claude Code-specific explanation with doc URL references
- Golden #3 McpServerStatusConfig: Bounded 4-field TypedDict hallucination (same as Sprint 003's rp=1.15 result)

**Epic 3.5 (unplanned) — Reward functions were unimplemented** (BLOCKER RESOLVED):
- Discovered on first UBENCH run: baseline AND adapter both scored **literal 0.0000 on all 50 rows all 3 metrics** with `stop=50, truncated=0` (clean generation)
- Root cause: `factuality_score`, `citation_precision`, `concision_ratio` declared in ULTS spec BUT never implemented in `neural/training/reward_functions.py`. `compute_reward()` silently returns `0.0` for undefined functions.
- Implemented all three (see §Rewards below) + hand-verified on realistic paraphrase test

**Epic 4 — Full 50-row UBENCH eval** (SHIPPED, all 3 variants):
- Baseline (14B production `:8102`)
- Adapter (mlx_lm.server + adapter_002, no rp)
- Adapter (mlx_lm.server + adapter_002, rp=1.15 via patched config)

**Epic 5 — Comparison** — see §Verdict

**Epic 6 — Sprint post + decision** — this file

## 2. Rewards implemented (Sprint 004 addition)

Added to `neural/training/reward_functions.py`:

| Reward | Definition | Kwargs |
|---|---|---|
| `factuality_score` | Bigram F1 vs reference (0-1) | `expected`/`target` = golden assistant content |
| `citation_precision` | \|extracted_ids ∩ ref_ids\| / \|extracted_ids\|; 0 if response has no ids | same |
| `concision_ratio` | max(0.1, min(1.0, ref_len / response_len)) | same |

`_extract_identifiers(text)` recognizes: backtick-wrapped tokens, function-call form `name(`, PascalCase/CamelCase, ALL_CAPS, snake_case with underscore, dotted access `foo.bar`. Filters lowercase natural-language words.

Also patched `neural/benchmarks/sampling_policy.py::resolve_sampling` to forward optional `repetition_penalty` from recipe (previously silently dropped).

## 3. Verdict (all values 50-row means, higher=better, [0,1])

| Config | overall | factuality | citation | concision |
|---|---:|---:|---:|---:|
| **Baseline (14B prod, no adapter)** | **0.3787** | 0.033 | **0.197** | **0.906** |
| Adapter (mlx + adapter_002, no rp) | 0.248 | 0.034 | 0.072 | 0.638 |
| Adapter (mlx + adapter_002, rp=1.15) | 0.264 | 0.035 | 0.086 | 0.671 |

**Adapter is −0.115 to −0.131 WORSE than baseline overall.** rp=1.15 provides ~+0.016 lift over no-rp but doesn't come close to baseline.

Per-metric read:
- **factuality_score is effectively tied** (~0.034 across all configs). Bigram-F1 is inherently strict when models paraphrase — a metric floor, not a differentiator here.
- **citation_precision is the KILLER**: baseline 0.197 → adapter ~0.08. Adapter cites MORE identifiers (learned Claude Code docs' verbose citation style) but many are HALLUCINATED (not in reference). Baseline cites fewer, more of them correct.
- **concision_ratio is a KILLER**: baseline 0.906 → adapter ~0.65. Adapter generates VERBOSE output (learned Claude Code docs' long-form style) while baseline stays terse.

Combined: adapter_002 learned the SHAPE of Claude Code docs (verbose, many identifiers, code blocks) but doesn't reliably reproduce the SPECIFICS. Sounds correct, but is factually wrong more often. This is the classic "surface-learning without content-learning" LoRA failure.

## 4. What we learned (arch rules pinned)

⚠️ **Rule A — silent-zero reward returns are catastrophic and undetectable**: `compute_reward()` silently returns `0.0` for undefined function names. The three claude.code_knowledge rewards were declared in the ULTS spec 2026-08-14 (CLAUDE-DOCS-TRAINING-001) but never implemented — every eval since then produced literal 0.000 across the board without warning. Fix: **CI-audit that every reward function name across `docs/tests/ults/specs/*.ults.json` exists in `REWARD_REGISTRY`**. This sprint added the 3 functions but the systemic gap remains as a follow-up.

⚠️ **Rule B — cherry-picked 3-row hand smokes DO NOT generalize to holdouts**: adapter_002 got Golden #2 EffortLevel **5/5 EXACT** on hand smoke, adapter_003 sprint post called this "chunking hypothesis validated." But the full 50-row holdout shows adapter_002 is −0.13 WORSE than baseline. NEVER conclude "adapter works" from ≤5-row hand smokes; always run the full holdout before promoting. Extends CLAUDE-DOCS-TRAINING-003 Rule B ("randomness dominates at small delta between checkpoints").

⚠️ **Rule C — LoRA W_new = W + BA math depends on the base**: adapter trained on raw Qwen3-14B-4bit does NOT compose meaningfully on Phase-5 SFT of that raw base. GGUF LoRA loaded via llama.cpp on `mdemg-llm-v1.Q5_K_M.gguf` (Phase-5 FT base) produced OUTPUT INDISTINGUISHABLE from base-only inference. Only mlx_lm.server on the ORIGINAL raw base recovered adapter behavior. Rule: **adapter must be served on the exact base it was trained on** (SHA-checked). If deployment requires FT base + LoRA composition, either co-train them or accept that the adapter contribution will not compose.

⚠️ **Rule D — task #91 verdict on `claude.code_knowledge` is INVALIDATED**: MODEL-SWAP-QWEN27B-EVAL concluded "Qwen3.6-27B FOSS scored 0.000 same as baseline on claude.code_knowledge, so base-model swap doesn't help." That 0.000 was silent-zero from unimplemented rewards. **Re-running the 27B FOSS bake-off with implemented rewards is necessary** before concluding base-model swap can't help. Filed as follow-up MODEL-SWAP-QWEN27B-CLAUDE-DOCS-REEVAL.

⚠️ **Rule E — adapter LoRA that learns "shape" without "content" makes things WORSE, not neutral**: adapter_002 with factually-correct on 5/5 of the ONE row it was tested against (EffortLevel) but massive false-citation on 45 untested rows shows LoRA can teach a verbose-with-identifiers style even when the specific facts weren't learned. Combined with a bigram-F1 factuality metric that's near-zero for all configs (paraphrase-strict), the FALSE-CITATION penalty dominates. When adapter shows this pattern (high citation count, low citation accuracy), it's actively degrading — not just failing to help.

⚠️ **Rule F (operator correction, 2026-08-17) — Fact-recall tasks are substrate-ingest problems in MDEMG's architecture, not model-weight-fine-tune problems**: MDEMG IS an improved RAG architecture (RRF over 4-5 columns: Embedding + BM25 + Graph + Structural + Context-fingerprint; cross-encoder rerank; 5-layer abstraction hierarchy; Hebbian reinforcement; consulting + jiminy synthesis). Choose LoRA when you want to shift model STYLE / REASONING / CALIBRATION. Choose substrate-ingest when you want to expose FACTS retrievable at inference. The CLAUDE-DOCS-TRAINING-001-through-004 arc was a category error: 4 sprints of LoRA training proved by exclusion that model-weight fine-tuning cannot force >2000-row fact recall into 14B parameters — but that failure was already predictable from MDEMG's architectural intent. The correct successor is CLAUDE-DOCS-INGEST-001 (ingest into substrate, let retrieval surface at inference), NOT "add RAG hybrid on top of LoRA".

## 5. Follow-up options

⚠️ **Architectural correction (operator, 2026-08-17)**: MDEMG IS the retrieval architecture (RRF over 4-5 columns: Embedding + BM25 + Graph + Structural + Context-fingerprint; cross-encoder rerank; 5-layer abstraction hierarchy; Hebbian reinforcement; consulting + jiminy synthesis; **RSIC self-improvement loop**). The claim "add RAG hybrid" in an earlier draft of this section was confused — MDEMG is an improved RAG substrate at a level above vanilla RAG. The correct architectural separation is:

- **Model weights (Phase-5 SFT `mdemg-llm-v1`)**: reasoning, style, general knowledge
- **MDEMG substrate**: specific facts, retrievable at inference time via the 4-5-column retrieval pipeline
- **Consulting/Jiminy layer**: synthesizes retrieved context into task-appropriate guidance
- **RSIC (Recursive Self-Improving Cognition)**: continuously assesses substrate health across 7 dimensions (retrieval / memory / edge / task / guidance / protocol / synergy), reflects via LLM to produce insights, executes actions (graph repair, tombstone consolidation, drift detection, retrain triggers, etc.), and drives the substrate to improve autonomously. RSIC is what makes MDEMG an *improved* RAG — the substrate isn't static; it self-heals + self-optimizes based on measured retrieval quality + Hebbian reinforcement signals + consolidation cycles.

Sprint 004's failure mode reframes cleanly under this lens: adapter_002 tried to bake 2141 concrete doc rows into 14B parameters via LoRA — architecturally the wrong tool. The right tool is **ingest into substrate + let the retrieval pipeline surface them at inference**.

### Follow-up options (correct architectural framing)

1. **CLAUDE-DOCS-INGEST-001 (recommended)** — ingest the 2141-row Claude Code corpus into the MDEMG substrate as memory nodes (role_type=`documentation`/`reference`, layer=0 with abstraction to layer≥1 via existing consolidation). At inference, when a query touches Claude Code CLI/SDK concepts, MDEMG's retrieval pipeline surfaces the relevant docs; Phase-5 `mdemg-llm-v1` grounds its answer on retrieved context. Zero LoRA needed. Zero model weights touched. Zero base-model composition risk. This IS how MDEMG is designed to acquire knowledge.
2. **Accept LoRA path dead for Claude Code knowledge**. Model-weight fine-tuning is the wrong tool for large-corpus fact recall in this architecture.
3. **Re-run MODEL-SWAP-QWEN27B-EVAL with rewards implemented** — the 27B FOSS verdict was based on silent-zero data; cheap to re-measure now.
4. **Redesign the eval rubric**: bigram-F1 factuality is too strict when models paraphrase. Adding an LLM-judge factuality metric (per HITL-CURATION-002 pattern) could give real signal that this deterministic rubric misses. Necessary for both LoRA and substrate-ingest verdicts to be trustworthy.
5. **LoRA architecture iteration** (only if we insist on the weight-baking approach after option 1 proves insufficient): rank=32 α=64 on 7 target modules may be too much capacity, causing shape-overfit. Try rank=8 or fewer target modules.

**My recommendation**: option 1 (CLAUDE-DOCS-INGEST-001) as the architecturally-correct successor, option 3 as a cheap parallel task to clean up the invalidated data. Option 4 (LLM-judge eval) should be shared infrastructure that unblocks both.

**Concrete DO NOT PROMOTE**: Do not flip `mdemg ft-loop promote` for adapter_002. Leave production `mdemg-llm-v1.Q5_K_M.gguf` on `:8102` as canonical.

⚠️ **New arch rule from this correction (Rule F below)**: **Fact-recall tasks are substrate-ingest problems in MDEMG's architecture, not model-weight-fine-tune problems.** Choose LoRA when you want to shift model STYLE/REASONING/CALIBRATION; choose ingest when you want to expose FACTS. Sprint 001-004's LoRA arc was a category error — the 4 sprints of compute proved by exclusion that model-weight fine-tuning cannot force >2000-row fact recall into 14B parameters, but that was already predictable from MDEMG's architectural intent.

## 6. Verification

- [x] E1: GGUF LoRA file exists (168 MB, 320 tensors)
- [x] E2: llama-server loads adapter (no error) BUT adapter has zero effect on FT base
- [x] E2b: mlx_lm.server + raw base + adapter recovers behavior (Golden #2 5/5 EXACT)
- [x] E3: 3-row hand smoke bounded on all 3 fixtures via mlx path
- [x] E3.5: Reward functions implemented + hand-verified on realistic paraphrase (adapter-style scores 0.68, baseline-style scores 0.35 on synthetic reference — differentiate correctly)
- [x] E4: 50-row eval completed on all 3 configs (baseline, adapter-no-rp, adapter-rp=1.15) with clean `stop=50, truncated=0`
- [x] E5: Three-way delta table (above)
- [x] E6: Sprint post exists; verdict named + operator promotion command spelled out (WILL NOT ISSUE)
- [x] Live smoke: hand-inspected 3 adapter outputs; all showed hallucination pattern

## 7. Files touched

- `neural/training/reward_functions.py` — added `factuality_score`, `citation_precision`, `concision_ratio` + `_extract_identifiers` + `_bigrams` + `_tokenize_words` helpers; extended REWARD_REGISTRY 23 → 26 entries
- `neural/benchmarks/sampling_policy.py` — forward optional `repetition_penalty` from recipe
- `docs/development/claude-docs-training-004/sprint_plan.md` — this sprint's plan
- `docs/development/claude-docs-training-004/sprint_post.md` — this file
- `training_data/eval/claude-docs-training-004/baseline_20260817.json` — 50-row baseline result
- `training_data/eval/claude-docs-training-004/adapter_002_20260817.json` — 50-row adapter (no rp) result
- `training_data/eval/claude-docs-training-004/adapter_002_rp115_20260817.json` — 50-row adapter (rp=1.15) result

Untracked (regenerable, .gitignored):
- `.local-models/claude_docs_002_peft/*` — PEFT intermediate
- `.local-models/claude_docs_002.gguf-lora` — GGUF LoRA artifact

## 8. Documents Accessed

- `docs/development/claude-docs-training-003/sprint_post.md` — priors, adapter_002+rp=1.15 3-row smoke record
- `docs/development/claude-docs-training-002/sprint_post.md` — adapter_002 training recipe + Golden #2 5/5 EXACT smoke
- `docs/development/claude-docs-training-001/sprint_post.md` — adapter_001 baseline
- `docs/development/model-dist-002/sprint_plan_model_dist_002.md` — MLX → PEFT → GGUF LoRA pipeline
- `docs/development/claude-docs-training-004/sprint_plan.md` — this sprint's plan
- `docs/tests/ults/specs/claude_code_knowledge.ults.json` — reward function names + rubric
- `training_data/eval/valid_clean.jsonl` — 50 claude.code_knowledge rows source
- `training_data/eval/claude_code_knowledge_golden.jsonl` — original golden holdout (superseded by valid_clean's meta.task_name)
- `neural/benchmarks/run_benchmark.py` — UBENCH runner + `_extract_reward_kwargs`
- `neural/benchmarks/sampling_policy.py` — `resolve_sampling` + presence_penalty precedent
- `neural/training/reward_functions.py` — REWARD_REGISTRY + `compute_reward` silent-zero pattern
- `scripts/mlx_adapter_to_peft.py`, `scripts/vendor/llama_cpp/convert_lora_to_gguf.py` (via llama.cpp master PYTHONPATH) — MLX→GGUF pipeline
- Task #91 (MODEL-SWAP-QWEN27B-EVAL) results — retroactively invalidated by Rule D
- Live probing via `curl` + hand-inspection of adapter outputs across 3 Golden fixtures on both GGUF and MLX paths
