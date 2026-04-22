# Sprint FT-LORA-D — Expert Activation Profiling

## Context

Sprint C passed (`bf5f9c6` / PR #342 merged, 2026-04-22): Qwen3.6-35B-A3B-mxfp4 clear-passes both throughput (126.65 tok/s median, 2.1× floor) and Path A quality parity (−13.83% gap, Qwen ahead, `clear_pass` band). Base-model plausibility is proven; training line is unblocked.

Sprint D's one job is to **decide which 64 of 256 routed experts per layer belong to each of three task families** (`reasoning-think` / `classify-notink` / `structured-notink`) so Sprint E's Tier 2 LoRA (r=8 α=16 per family) only unfreezes the top-25% most-activated routed experts per family — the "Sieve" half of the MoE-Sieve strategy (canonical: `03_IMPLEMENTATION_PLAN_v2.md` §Phase 5.X; `01_RESEARCH_v2.md` §5).

This is a **read-only profiling sprint** — no training, no model weight changes, no compute-intensive runs beyond forward passes over the 16-task anchor prompt set. Output is three JSON files (one per family) + a decision document. Zero training spend.

**Model-file clarity** (resolved mid-plan): the Sprint C artifact ships with config `model_type: qwen3_5_moe` and architecture class `Qwen3_5MoeForConditionalGeneration` (a VL-capable class whose vision head is stripped on load). mlx_lm 0.31.2 dispatches on `model_type` → loads `mlx_lm/models/qwen3_5_moe.py`, a 25-line wrapper whose only job is weight sanitization; the real MoE implementation (routing gate, SparseMoeBlock, experts) lives in `qwen3_5.py` which imports `Qwen3NextSparseMoeBlock` from `qwen3_next.py:308`. Sprint D hooks into that SparseMoeBlock's forward pass — the VL file `qwen3_vl_moe.py` is not reached and not relevant. "Qwen3.6" is mlx-community's repo display name; the checkpoint's internal model_type is `qwen3_5_moe` because the architecture is identical to Qwen3.5 MoE-text + mxfp4 quant.

**Sprint chain:** A (docs, done) → B (code alignment, done) → C (3-gate MLX validation, done 2026-04-22) → **D (this sprint)** → E (training infra patches) → Phase 5 SFT.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-D |
| Title | Expert Activation Profiling — Family Partition Decision |
| Date | 2026-04-22 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-C (PR #342, merged `bf5f9c6`) |
| Successors | FT-LORA-E (training infra patches — consumes `profile_routing_{family}.json`) |
| Type | Profiling script + decision doc + artifacts |
| Risk | Low (read-only forward passes; no training; no state mutation outside `training_data/routing_profiles/`) |
| Budget | $0 (local MLX inference only) |
| Artifacts | `training_data/routing_profiles/profile_routing_{family}.json` × 3 + `docs/development/ft-lora/sprint_c_d_profile_results.md` |

## 2. Problem Statement

Memo 07 v3.1 (`01_RESEARCH_v2.md` §5) locked in a **two-tier** LoRA strategy:

- **Tier 1** (all 16 tasks, balanced) — attention + shared expert, r=32 α=64. Trains first. Does not depend on Sprint D.
- **Tier 2** (per family) — top-25% routed experts only, r=8 α=16, one adapter per family. **Cannot start without Sprint D output.**

The family partition is provisional (`reasoning-think` / `classify-notink` / `structured-notink`) and must be empirically validated before Tier 2 training commits GPU/disk to per-family adapters. Two validation criteria per `sprint_plan_ft_lora_a.md:98`:
1. If **cross-family expert overlap exceeds 80%**, the partition is merged (families too similar to justify separate adapters).
2. If **any family shows bimodal routing** (two distinct expert cohorts within one family), that family is split.

Sprint C leaves us with a loaded, validated MLX model and a 16-task prompt set mapped to ULTS sampling groups T/C/J. Sprint D must:
- Run forward passes over representative prompts per task, capturing top-k routing decisions at every MoE layer
- Aggregate activation counts by (layer, expert) per task → per family
- Compute the Top-25% mask per (layer, family) = 64 of 256 experts
- Emit three artifacts consumable by Sprint E (`neural/training/train_ft.py --expert-selection-path=...`)
- Measure cross-family Jaccard overlap and per-family distribution modality; write the decision doc with go/no-go for the 3-family partition

## 3. Scope & Constraints

**In scope:**

| # | Deliverable | Path |
|---|---|---|
| 1 | Profiling script (new) | `neural/training/profile_expert_routing.py` |
| 2 | Anchor prompt set (new, derived from ULTS specs + benchmark set) | `training_data/routing_profiles/anchor_prompts.jsonl` |
| 3 | Per-family routing artifacts | `training_data/routing_profiles/profile_routing_{reasoning_think,classify_notink,structured_notink}.json` |
| 4 | Merged raw counts (debug/audit) | `training_data/routing_profiles/raw_activation_counts.json` |
| 5 | Decision doc | `docs/development/ft-lora/sprint_c_d_profile_results.md` |
| 6 | Unit tests | `neural/training/tests/test_profile_expert_routing.py` |
| 7 | Sprint D plan (this file → repo) | `docs/development/ft-lora/sprint_plan_ft_lora_d.md` |
| 8 | Doc updates | `00_README_v2.md` (version bump + Document Map row), `AGENT_HANDOFF.md`, `CHANGELOG.md`, `03_IMPLEMENTATION_PLAN_v2.md` §Phase 5.X (mark executed, link artifacts) |

**Out of scope (deferred to Sprint E or later):**
- Any Tier 1 or Tier 2 training launches
- Changes to `train_ft.py` beyond reading `--expert-selection-path` (Sprint E work)
- `mlx_lm.convert` asymmetric-quant selectors (Sprint E)
- Early-stop implementation (Sprint E)
- Router aux-loss exposure (Sprint E; placeholder var already seeded in Sprint B)

**Constraints:**
- **Sequential epics** (MEMORY rule `feedback_sequential_epics.md`). No parallel execution.
- **Single batched commit at sprint close** (MEMORY rule).
- **No destructive ops** — artifacts are new files only; overwriting a prior profile requires explicit `--force`.
- **No hardcoded values** (MEMORY rule): all paths, prompt-set sizes, layer ranges, top-k cutoffs exposed as CLI flags with sensible defaults.
- **Sprint C model SHA pin**: profiling must load the exact artifact stamped in Gate 3 (`cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734`). Script verifies SHA before running; aborts on mismatch.
- MLX forward passes only — no training, no gradient computation.
- Decision doc must explicitly state go/no-go per validation criterion (§ 2) before Sprint E can start.

## 4. Dependencies

- **Sprint C artifacts:** `~/.mdemg-sprint-c/model.json` (repo + SHA + local_path pin), `~/.cache/huggingface/hub/models--mlx-community--Qwen3.6-35B-A3B-mxfp4/`.
- **16-task ULTS specs:** `docs/tests/ults/specs/*.ults.json` — source for prompts per task, with `sampling_group` field (added Sprint B) deciding family membership.
- **Benchmark question set:** `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — fallback prompt source if an ULTS spec doesn't provide enough per-task example inputs (need ~20 per task).
- **mlx_lm 0.31.2** at `/Users/reh3376/.venv/mdemg-ft-lora/`: `mlx_lm/models/qwen3_5_moe.py` (loader wrapper), `qwen3_5.py` (text model), `qwen3_next.py:308` (`Qwen3NextSparseMoeBlock` — hook target).
- **Task → family mapping** (from ULTS `sampling_group`):
  - `reasoning-think` = group T = 7 tasks (ape.reflect, consulting.synthesis, hidden.summarize, jiminy.synthesize, metalearn.generalize, retrieval.rerank_nli, summarize.generate)
  - `classify-notink` = group C = 6 tasks (consulting.classify, hidden.reclassify, jiminy.evaluate, jiminy.codegen, retrieval.intent_translate, retrieval.query_classify)
  - `structured-notink` = group J = 3 tasks (hidden.name_emergence, jiminy.evaluate_llm, retrieval.rerank_cross)

No external service / network dependencies. No OpenAI API calls.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` fast-forwarded to `bf5f9c6`; tree clean (ignore pre-existing untracked `scripts/tsdb_data_review_2026-04-01.json`); venv `mdemg-ft-lora` active; `python -c "import mlx_lm, mlx.core"` succeeds; Sprint C model SHA matches.

### Epic 1 — Anchor prompt set assembly

Build a reproducible, task-labeled prompt corpus (~320 prompts = 20 per task × 16 tasks).

**Sub-steps:**
1. Parse all 16 `docs/tests/ults/specs/*.ults.json`; map `sampling_group` T/C/J → family (`reasoning_think` / `classify_notink` / `structured_notink`).
2. For tasks with fewer than 20 example inputs in their ULTS spec, backfill from `test_questions_120.json` filtered by category (whk-wms categories partition-map to tasks).
3. Emit `training_data/routing_profiles/anchor_prompts.jsonl`: one record per line `{"task", "family", "prompt", "source", "source_id"}`.
4. Write SHA256 of the finalized JSONL to `anchor_prompts.sha256` for reproducibility.

**Gate:**
- File exists, 320 records, 20 per task exactly.
- Per-family counts: reasoning_think=140, classify_notink=120, structured_notink=60 (or explain deviation).
- Every record's `family` agrees with its `task`'s `sampling_group`.

### Epic 2 — `profile_expert_routing.py` script

Create `neural/training/profile_expert_routing.py`. Key design:

1. **Router capture (full-generation mode):** mlx_lm 0.31.2 has no `return_router_logits` flag. Use a context-manager monkey-patch of `Qwen3NextSparseMoeBlock.__call__`, wrapping the original to capture `inds` (top-k expert indices, shape `[B, L, top_k=8]`, `qwen3_next.py:338`) and `scores` (routing weights, line 339). Store per-(layer_idx, prompt_id). `finally`-block restores original method.

   **Forward passes run in generation mode.** The profiler generates `max_new_tokens` per prompt (default 400, CLI-configurable) and captures routing decisions across **both prompt tokens and generated tokens**. Per-token aggregation sums across both. Rationale: Tier 2 adapters target output-production behavior; routing during generation — not just prompt comprehension — is the training signal of interest. Prompt-only profiling would miss decoder-side experts that matter most for the behaviors Tier 2 is meant to adapt. Budget implication: full-generation profiling takes ~2–3 hours for the 320-prompt set vs. ~30 min prompt-only; still single-session.
2. **Layer indexing:** iterate `model.language_model.model.layers` (40 layers per `text_config.num_hidden_layers`). Log any layer where `mlp` isn't a SparseMoeBlock.
3. **Aggregation:** per-token level, pooling prompt tokens + generated tokens. `activation_counts[layer][expert]` = sum of tokens across all family prompts that routed to that expert.
4. **Memory:** 22 GB model + forward-pass overhead fits easily in 128 GB. Cap prompts at 2048 tokens (CLI-configurable); truncate with warning.
5. **Top-25% mask:** 64 of 256 experts per layer per family. Ties broken deterministically (lowest index wins — documented behavior).
6. **Artifact schema** (`profile_routing_{family}.json`):
   ```json
   {
     "family": "reasoning_think",
     "model_sha256_config": "cdc167566e...",
     "num_layers": 40,
     "num_experts_per_layer": 256,
     "top_k_per_token": 8,
     "top_pct_selected": 25,
     "num_experts_selected_per_layer": 64,
     "prompts_processed": 140,
     "tokens_processed": <int>,
     "per_layer": [
       {"layer": 0, "top_experts": [153, 42, ...], "activation_counts": [...], "kl_div_vs_uniform": 2.31},
       ...
     ],
     "generated_at": "2026-04-22T...Z"
   }
   ```
7. **Raw counts dump:** `raw_activation_counts.json` with full per-(task, layer, expert) counts for post-hoc re-analysis.

**CLI:**
```
profile_expert_routing.py \
  --model-path <sprint-c local_path> \
  --expected-sha256 <sprint-c SHA>                  # hashes config.json only
  --anchor-prompts training_data/routing_profiles/anchor_prompts.jsonl \
  --output-dir training_data/routing_profiles/ \
  --top-pct 25 \
  --max-prompt-tokens 2048                          # cap on prompt length; truncate above
  --max-gen-tokens 400                              # tokens to generate per prompt
  [--force] [--limit N] [--task <task_name>] [--verbose]
```

`--expected-sha256` semantics: hashes `config.json` (same scope as Sprint C Gate 3 stamp's `model_sha256_config`). Sufficient for architectural profiling (same architecture = same routing topology). Does **not** guarantee bit-identical weights — Sprint F and Phase 5 SFT will additionally pin a safetensors weight hash; out of scope here.

**Gate:**
- `--help` prints usage; all flags documented including both `--max-prompt-tokens` and `--max-gen-tokens`.
- `--limit 5` dry run produces valid-schema artifacts.
- Dry run verifies **both prompt-side AND generated-token routing are captured** (not just one or the other) — inspect per-prompt token-count breakdown in the verbose log.
- SHA-guard aborts on mismatch (hand-test with bogus `--expected-sha256`).
- No hardcoded paths in script body.
- Progress logging per 10 prompts; final summary prints prompts / prompt-tokens / generated-tokens per family.

### Epic 3 — Full profiling run + validation-criterion analysis

**Sub-steps:**
1. Run: `python neural/training/profile_expert_routing.py --anchor-prompts ... --output-dir training_data/routing_profiles/`.
2. **Cross-family Jaccard overlap:** for each (layer, family-pair), `|A ∩ B| / |A ∪ B|` on top-64 sets. Average across 40 layers. Three pair averages: (T,C), (T,J), (C,J).

   **Decision rule (explicit, no mid-execution ambiguity):**
   - **0 pairs exceed 0.80** → 3-family partition confirmed; proceed as planned per memo 07 v3.1 §5. Verdict: `3-family-confirmed`.
   - **Exactly 1 pair exceeds 0.80** → merge that pair; proceed with 2 families. Name the merged family in the decision doc (e.g. T+C merged as `reasoning-classify` if (T,C) exceeds). Verdict: `2-family-merged-<pair>`.
   - **≥2 pairs exceed 0.80** → collapse to 1 family (single Tier 2 adapter over the full top-25% union). Transitively implies all three pairs are high-overlap; 3 families not justified. Verdict: `1-family-collapsed`.

3. **Per-family task-cohesion analysis (replaces bimodality coefficient):** BC = (skew² + 1) / kurt is from the univariate-continuous-distribution literature and is a methodological stretch applied to a 256-bin discrete activation distribution with n=60–140 prompts (structured_notink has only 3 tasks × 20 = 60 prompts). The raw per-(task, layer, expert) counts already get dumped to `raw_activation_counts.json`, so compute task-cohesion directly from that:

   For each family, compute per-task top-64 expert sets per layer from `raw_activation_counts`. Compute pairwise Jaccard between tasks within the family, averaged across 40 layers. Verdict per family:
   - **within-family mean pairwise Jaccard ≥ 0.70** → `cohesive` (tasks route similarly; no split needed).
   - **0.40 ≤ mean < 0.70** → `ambiguous` (report; no action).
   - **mean < 0.40** → `split-candidate` (tasks route divergently). Identify the task-cluster boundary via simple hierarchical clustering (agglomerative, Jaccard distance, single linkage) on the per-task top-64 sets. Report the boundary in the decision doc; **do NOT perform the split in Sprint D** — Sprint E consumes the recommendation.

   BC may be included as a **supplementary diagnostic** if it helps discussion, but must not trigger any sprint decision.

4. **KL divergence vs uniform:** per-layer `KL(routing_dist || uniform_256)`. High KL = concentrated (Sieve works well). Low KL = diffuse (flag).
5. Write `docs/development/ft-lora/sprint_c_d_profile_results.md`:
   - **Executive summary** — verdict as one of `{3-family-confirmed, 2-family-merged-<pair>, 1-family-collapsed}` plus per-family cohesion verdicts
   - **Cross-family overlap table** — 3 pair averages + per-layer breakdown
   - **Per-family task-cohesion table** — columns: `family | n_tasks | within-family mean Jaccard | verdict` (cohesive / ambiguous / split-candidate)
   - KL divergence summary
   - Recommendation to Sprint E: proceed with 3 adapters / merge X+Y / collapse to 1 / split Z with boundary {T_a,T_b} vs {T_c,...}
   - Artifact SHAs
   - Documents Accessed appendix

**Gate:**
- 3 `profile_routing_{family}.json` files exist, schema-valid, non-empty.
- Raw counts file exists.
- Decision doc exists with **explicit verdict code** (3-family-confirmed / 2-family-merged-<pair> / 1-family-collapsed), 3 cross-family overlap numbers quoted, and a per-family task-cohesion table.
- If verdict is merge/collapse/split-candidate: Sprint E planner receives the revised partition (and split boundary, if any) via the decision doc.

### Epic 4 — Unit + integration tests

**Tier 1 (Unit):**
- `test_anchor_prompts_loader` — valid JSONL → correct record count + family partition.
- `test_top_k_aggregation` — synthetic `inds` tensor → correct activation counts.
- `test_top_pct_mask` — handcrafted counts → exactly 64 of 256 per layer; tie-break behavior verified.
- `test_jaccard_overlap` — synthetic expert sets with known overlap → correct value.
- `test_task_cohesion_within_family` — synthetic per-task top-64 sets with known pairwise Jaccard → correct within-family mean + correct verdict (cohesive / ambiguous / split-candidate).
- `test_hierarchical_cluster_boundary` — handcrafted 5-task family with a clear 2-cluster split → clustering returns the expected boundary.
- `test_sha256_guard` — bad SHA → non-zero exit.

**Tier 2 (Integration):** `--limit 10` run against real Sprint C model, verify shapes + artifact structure + monkey-patch cleanup.

**Tier 3 (E2E):** full 320-prompt run; 3 artifacts schema-validated; re-run spot-check (see determinism tolerance below).

**Pre-Tier-3 determinism sanity check:** run the profiler twice on 5 prompts and diff the artifacts. MLX + mxfp4 forward passes may have non-bit-deterministic tie-breaking at the router level.
- **Bit-identical** → Tier 3 test: "same top-64 on two consecutive runs" (strict).
- **Non-identical but ≥95% top-64 match** → relax Tier 3 to "≥95% of top-64 experts match across runs, documented as 'expected MLX floating-point tie-breaking variance'".
- **<95% match** → investigate before running the full profile; something more than tie-breaking is at play.

Flag the determinism-sanity-check outcome in the commit body regardless of the path taken.

**Gate:** all three tiers green; pytest clean; integration + E2E commands exit 0; determinism tolerance recorded.

### Epic 5 — Documentation update (final epic — never cut)

Per MEMORY rule: documentation is the last epic, never dropped.

1. Copy `~/.claude/plans/breezy-dancing-lerdorf.md` → `docs/development/ft-lora/sprint_plan_ft_lora_d.md`.
2. Append "Documents Accessed" appendix to the copied plan.
3. Update `00_README_v2.md`: version 5.1 → 5.2; new Document Map row for Sprint D plan + decision doc; Key Decisions row "Family partition validated: [verdict]".
4. Update `03_IMPLEMENTATION_PLAN_v2.md` §Phase 5.X: mark executed, link 3 artifacts + decision doc.
5. Update `AGENT_HANDOFF.md` — Sprint D completion entry at top.
6. Update `CHANGELOG.md` `[Unreleased]` `### Added`:
   - `neural/training/profile_expert_routing.py` — expert activation profiler
   - `training_data/routing_profiles/profile_routing_{family}.json` × 3 — Sprint E Tier 2 inputs
   - `docs/development/ft-lora/sprint_c_d_profile_results.md` — family partition decision doc
7. Cross-reference check: every `sprint_plan_ft_lora_d.md` / §Phase 5.X pointer resolves.

**Gate:** sprint plan in repo; all cross-refs valid; CHANGELOG + AGENT_HANDOFF current; sprint summary drafted for PR comment.

## 6. Testing Plan (Three Tiers)

Covered by Epic 4. Summary:

- **Tier 1 (Static + Unit):** pytest on `test_profile_expert_routing.py`; ruff + mypy clean; `--help` CLI smoke test.
- **Tier 2 (Integration):** `--limit 10` run on real Sprint C model; artifact schema validation; monkey-patch cleanup verified.
- **Tier 3 (E2E):** full 320-prompt run; 3 artifacts + decision doc emitted; re-run spot-check per determinism tolerance (bit-identical / ≥95% / <95% branching rule, see Epic 4); Sprint-E-consumer dry-read succeeds.

State restoration (MEMORY rule): `training_data/routing_profiles/` is committed directly (no TSDB/graph mutation). `raw_activation_counts.json` added to `.gitignore` if not already (debug artifact, not a deliverable).

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY rule):

- Title: `feat(ft-lora): Sprint D — expert activation profiling + family partition decision`
- Body: one bullet per epic + a **"Family partition verdict"** section pulling the Epic 3 decision into the commit body (so reviewers see the outcome without opening the decision doc). Include: verdict code (3-family-confirmed / 2-family-merged-<pair> / 1-family-collapsed), the 3 cross-family Jaccard means, per-family task-cohesion verdicts, any split-boundary recommendations, the determinism-sanity-check outcome (bit-identical / ≥95% / <95%), and Sprint E downstream impact.
- Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`

Push to `reh3376_dev01` → auto-PR opens → sprint summary comment posted on PR (MEMORY rule) with verdict, artifact links, Sprint E unblock status.

## 8. Verification Checklist

- [ ] Pre-gate: branch at `bf5f9c6`, tree clean, venv + mlx_lm + model SHA all green
- [ ] Epic 1: `anchor_prompts.jsonl` (320 records, 20/task) + SHA file
- [ ] Epic 2: `profile_expert_routing.py` with working `--help`, SHA guard, monkey-patch capture; `--limit 5` dry run green
- [ ] Epic 3: full 320-prompt run complete; 3 family artifacts + raw counts emitted
- [ ] Epic 3: decision doc with explicit verdict code (3-family-confirmed / 2-family-merged-<pair> / 1-family-collapsed) + cross-family overlap table + per-family task-cohesion table
- [ ] Epic 4: pytest green; integration + E2E commands exit 0
- [ ] Epic 5: sprint plan copied to `docs/development/ft-lora/sprint_plan_ft_lora_d.md` with Documents Accessed appendix
- [ ] Epic 5: `00_README_v2.md` 5.1→5.2 + Document Map; `03_IMPLEMENTATION_PLAN_v2.md` §Phase 5.X marked executed
- [ ] Epic 5: AGENT_HANDOFF + CHANGELOG current
- [ ] Commit pushed; auto-PR opened; sprint summary comment posted

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 5. Key deliverables: `sprint_plan_ft_lora_d.md`, `sprint_c_d_profile_results.md`, `00_README_v2.md` bump, `03_IMPLEMENTATION_PLAN_v2.md` Phase 5.X execution pointer, AGENT_HANDOFF + CHANGELOG entries, Documents Accessed appendix.

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|
| Monkey-patch on `Qwen3NextSparseMoeBlock.__call__` corrupts model state for other callers | Low | Context-manager pattern + `finally`-block restoration; unit test verifies cleanup | Subclass the model and replace the MoE block with an instrumented variant |
| mlx_lm point-release breaks the monkey-patch | Low | Script asserts `mlx_lm.__version__ == "0.31.2"`, warns on mismatch | Inspect new SparseMoeBlock at mismatch, adjust the hook, bump Sprint D commit |
| Forward pass OOMs on long prompts | Low-Medium | Truncate to 2048 tokens (CLI-configurable); M5 Max has 128 GB | Reduce cap to 1024; or per-prompt streaming |
| Cross-family overlap > 80% invalidates 3-family partition | Medium | Decision doc handles this explicitly — recommend merge; Sprint E consumes revised partition | Reduce to 2 families (`think` vs `no_think`) in Sprint E |
| Bimodal family needs a split Sprint D can't decide | Medium | Flag-only in Sprint D; Sprint E performs sub-task clustering to decide split basis | Proceed with unsplit family in Sprint E; revisit after Tier 2 eval |
| KL divergence too low (diffuse routing) — Sieve thesis unsupported | Low-Medium | Document in decision doc; reference Sprint E commit-or-fallback path | Sprint E falls back to single-tier LoRA (attention + shared + full routed at r=8) |
| Anchor prompt set biased toward certain subtasks | Medium | Stratified sampling (20/task enforced by Epic 1 gate); source diversity (ULTS + whk-wms) | Sensitivity analysis: re-run with perturbed prompt set from whk-wms only |
| Family assignment from `sampling_group` doesn't match routing groupings | Low | `sampling_group` is canonical (memo 07 v3.1 §3.3); record discrepancies in decision doc | Decision doc proposes alternative naming for Sprint E |

## 11. Documents Accessed

**During planning:**
- `/Users/reh3376/.mdemg-sprint-c/gate3/passed_20260422T074500Z.json` — Gate 3 PASS stamp
- `/Users/reh3376/.mdemg-sprint-c/gate3/deviation.md` — Path A deviation context
- `/Users/reh3376/.mdemg-sprint-c/model.json` — model SHA pin
- `/Users/reh3376/.cache/huggingface/hub/models--mlx-community--Qwen3.6-35B-A3B-mxfp4/config.json` — verified `model_type: qwen3_5_moe`, 40 layers, 256 experts, top-k=8
- `/tmp/gate3_benchmark.py` — Path A driver (reference for prompt-handling patterns)
- `docs/development/ft-lora/00_README_v2.md` — Document Map, Key Decisions, v5.1 status
- `docs/development/ft-lora/01_RESEARCH_v2.md §5` — MoE-Sieve two-tier strategy
- `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md` — Phase 5.X artifact paths + Sprint E consumer interface
- `docs/development/ft-lora/sprint_plan_ft_lora_a.md:98` — family validation criteria (80% overlap, bimodality)
- `docs/development/ft-lora/02_M5MAX_HARDWARE_v2.md:191` — routing profile hardware budget
- `docs/tests/ults/specs/*.ults.json` (16 files) — task → sampling_group mapping
- `mlx_lm/utils.py:185-190` — MODEL_REMAPPING + dispatch confirming `qwen3_5_moe.py` is the loader
- `mlx_lm/models/qwen3_5_moe.py` — 25-line wrapper, `sanitize()` only
- `mlx_lm/models/qwen3_5.py:223` — DecoderLayer uses `SparseMoeBlock` when `num_experts > 0`
- `mlx_lm/models/qwen3_next.py:308-348` — `Qwen3NextSparseMoeBlock`, hook target; `inds` (line 338) + `scores` (line 339) are capture points
- `neural/training/` directory listing — confirmed no existing `profile_expert_routing.py`
- `CLAUDE.md` — project instructions, memory rules
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` — MEMORY rules: sequential epics, batched commit, sprint-plan location, no hardcoded values, 3-tier testing, Documents Accessed appendix, plan-before-code

**Referenced but not read in-depth (will read during execution):**
- `docs/architecture/benchmarks/whk-wms/test_questions_120.json` — fallback prompt source (Epic 1)
- `neural/training/train_ft.py` — Sprint E consumer (Sprint E reads, not Sprint D)

**During execution (2026-04-22):**
- `training_data/raw/extracted/llm_interactions.jsonl` — primary anchor-prompt source (221/320 records; `task_name` + `user_prompt` fields); replaced the plan's ULTS-example-inputs + whk-wms backfill path because (a) ULTS specs don't embed example inputs in scorable form, (b) same-shape donor-task production traces preserve task-family routing signal better than category-matched benchmark questions.
- `mlx_lm/generate.py` — `stream_generate` signature used for full-generation profiling.
- `mlx_lm/models/qwen3_next.py:308-348` — re-read during Epic 2 to confirm the exact forward-path to inline inside the RouterCapture wrapper (gate → softmax → argpartition → take_along_axis → norm_topk_prob → switch_mlp → shared_expert + shared_expert_gate).
- `training_data/routing_profiles/anchor_prompts.jsonl` (produced Epic 1) — consumed by Epic 2 integration test + Epic 3 full run.
- `~/.mdemg-sprint-c/gate4/profile_run.log` — full-run progress + per-segment token accounting (prompt vs generation).

## 12. Rollback

Read-only sprint — no DB migrations, no training, no shared state mutation. Rollback = `git revert <sha>` + delete `training_data/routing_profiles/` contents. No state recovery needed.

---

## Post-Sprint D

Sprint D → **Sprint E** (training infra patches: `router_aux_loss_coef` exposure, `mlx_lm.convert` per-module selectors, Tier 1/2 CLI flags consuming `--expert-selection-path=profile_routing_{family}.json`, early-stop implementation per overfitting-prevention policy). Sprint E plan drafted after Sprint D merges, consuming the Epic 3 decision-doc verdict.

Phase 5 SFT unblocks only after Sprint E ships.
