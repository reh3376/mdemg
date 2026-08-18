# CLAUDE-DOCS-TRAINING-004 — Sprint Plan

## 1. Header & Metadata

Sprint: `CLAUDE-DOCS-TRAINING-004` · opened 2026-08-17 · branch `reh3376_dev01`
Effort: ~30 min pipeline conversion + ~30 min UBENCH 50-row eval + ~1h write-up = ~2h wall
Target span: ~4-6h with waits
Risk: LOW (adapter is default-not-promoted; production llama-server on :8102 untouched until operator sign-off; all steps reversible)

**Follows**: `CLAUDE-DOCS-TRAINING-003` (2026-08-16) which invalidated the 2-epoch sweet-spot hypothesis but reconfirmed `adapter_002 + repetition_penalty=1.15 at inference` as the best-available config (3/3 fixtures bounded; Golden #2 preserved 4 of 5 exact enum values; Golden #3 hallucinates bounded 3-field TypedDict).

**Complements**: `MODEL-SWAP-QWEN27B-EVAL` (task #91, completed 2026-08-17) which ruled out base-model swap as a Claude Code knowledge solution — fresh Qwen3.6-27B FOSS scored **identical 0.000** to baseline on the 50-row `claude.code_knowledge` UBENCH holdout. LoRA remains the only viable path to non-zero Claude Code documentation knowledge.

## 2. Problem Statement

Production `mdemg-llm-v1` (14B, Phase-5 SFT) scores **literal 0.000** on the shipped `claude.code_knowledge` UBENCH task (50 golden holdout rows, 3 metrics: `factuality_score`, `citation_precision`, `concision_ratio`). Two prior training sprints validated the chunking + LoRA approach on hand-picked 3-row fixture smokes (adapter_002 got `EffortLevel` exact) but never ran the full 50-row eval to prove generalization.

Sprint 003's operator-recommended config (`adapter_002 + repetition_penalty=1.15`) has been validated only on the 3-row smoke via `mlx_lm.generate`. It has not been:
1. Converted to GGUF LoRA format (needed for production llama-server)
2. Loaded via `llama-server --lora`
3. Benchmarked on the full 50-row `claude.code_knowledge` holdout under the shipped UBENCH rubric

If UBENCH shows the same 0.000 as baseline, the LoRA track is a dead end and the operator can pivot to RAG or accept the gap. If it shows lift, this is the promote-ready artifact.

## 3. Scope & Constraints

**In scope:**

- (E1) Convert `adapters/claude_docs_002/adapters.safetensors` MLX → PEFT → GGUF LoRA via the shipped pipeline (MODEL-DIST-002)
- (E2) Load GGUF LoRA on side-port llama-server (`:8106`) with production base model + `--repetition-penalty 1.15`
- (E3) Reproduce 3-row Sprint 003 hand smoke on llama.cpp (not mlx_lm) to confirm the pipeline preserves the enum-recall win
- (E4) Full 50-row UBENCH `claude.code_knowledge` eval on the adapter-loaded side-port with rp=1.15
- (E5) Compare vs baseline; apply Note 02 gate (candidate mean ≥ baseline mean, no per-item regression > 0.10)
- (E6) Sprint post + operator promotion decision (do NOT auto-promote; operator flips via `mdemg ft-loop promote` after reviewing verdict)

**Out of scope:**

- Retraining a new adapter (adapter_002 exists; sprint 003 proved 2-epoch is worse; sprint 002 proved 3-epoch is best of the tried configs)
- Custom logits processor (deferred — repetition_penalty is proven cheaper first pass)
- Promotion to production (operator-gated per FT-RECURSIVE-003 shipped promotion contract)
- Changes to production `:8102` llama-server plist (side-port pattern only)
- Base-model swap (task #91 verdict already committed)

**Constraints:**

- Production `:8102` MUST stay online + untouched throughout (all work on side-port `:8106`)
- Zero production writes / substrate mutations
- adapter_002 is the fixed input; no re-training
- 50-row eval must complete under 30 min wall time (test practicality)
- Existing `mdemg_ft_loop:*` scheduled-job path NOT invoked (this is out-of-band manual eval)

## 4. Dependencies

- **adapter_002 on disk** (`adapters/claude_docs_002/adapters.safetensors`, 168 MB, MLX format) ✅
- **`scripts/mlx_adapter_to_peft.py`** (MODEL-DIST-002) ✅
- **`scripts/vendor/llama_cpp/convert_lora_to_gguf.py`** (pinned llama.cpp b9000, MODEL-DIST-002) ✅
- **Production base model** GGUF at `.local-models/serving/current.gguf` (Phase-5 14B) — shared with `:8102` ✅
- **`llama-server` b10450** (upgraded during task #91) ✅
- **`claude.code_knowledge` ULTS spec** + 50-row golden holdout (`training_data/eval/claude_code_knowledge_golden.jsonl`) ✅
- **UBENCH runner** (`neural.benchmarks.run_benchmark`) ✅

## 5. Implementation Plan (sequential)

**Epic 1 — MLX → GGUF LoRA conversion** (~15 min)
```
scripts/mlx_adapter_to_peft.py \
    --mlx-dir adapters/claude_docs_002 \
    --output .local-models/claude_docs_002_peft \
    --base-model mlx-community/Qwen3-14B-4bit

python scripts/vendor/llama_cpp/convert_lora_to_gguf.py \
    --outfile .local-models/claude_docs_002.gguf-lora \
    .local-models/claude_docs_002_peft
```
Output: `.local-models/claude_docs_002.gguf-lora` (~150 MB expected)

**Epic 2 — Side-port llama-server with adapter loaded** (~5 min)
```
llama-server \
    --model .local-models/serving/current.gguf \
    --lora .local-models/claude_docs_002.gguf-lora \
    --port 8106 --host 127.0.0.1 \
    --ctx-size 32768 --parallel 1 --flash-attn on \
    --n-gpu-layers 999 --cont-batching --metrics --jinja
```
Verify: `curl :8106/v1/models` → returns model with lora slot ready.

**Epic 3 — 3-row hand smoke on llama.cpp** (~5 min)
- Same 3 fixtures Sprint 003 used: query() vs ClaudeSDKClient / EffortLevel / McpServerStatusConfig
- Two runs each: with vs without `--repetition-penalty 1.15`
- Assert bounded outputs (Sprint 003 Rule C: `len(output) < 3× reference_length`)
- Compare enum-recall on EffortLevel — expect adapter_002 rp=1.15 to preserve 4/5 as in Sprint 003's mlx_lm result

**Epic 4 — Full 50-row UBENCH eval** (~20-30 min)
```
neural/.venv/bin/python -m neural.benchmarks.run_benchmark \
    --config configs/benchmark_phase10.yaml \
    --mlx-base-url http://127.0.0.1:8106/v1 \
    --mlx-model-name <adapter-served-model-name> \
    --rows-per-spec 0 --n-runs 1 \
    --tasks-only claude.code_knowledge \
    --mlx-timeout-s 180 \
    --out training_data/eval/claude-docs-training-004/adapter_002_rp115_20260817.json
```
Note: `--tasks-only` may not exist as flag; if not, use full-spec run and filter output. Also may need to pass rp=1.15 via server URL or bench config — verify via `--help`.

**Epic 5 — Compare vs baseline + verdict**
- Baseline `claude.code_knowledge` = 0.000 (n=3) from task #91 baseline run
- Adapter mean = ??? (n=50)
- Note 02 gate: mean ≥ 0 (trivially met if adapter has any signal); per-row regression not applicable (baseline is 0.000)
- Real bar: at least 1 of 3 metrics ≠ 0 on ≥5 rows — evidence adapter learned anything
- Stretch bar: `factuality_score` mean > 0.30 — evidence adapter is production-worthy

**Epic 6 — Sprint post + operator promotion decision**
- Write `docs/development/claude-docs-training-004/sprint_post.md` with verdict + artifacts
- Update CLAUDE.md pin ONLY IF verdict is PROMOTE (arch rule to preserve)
- Do NOT flip `FT_LOOP_AUTO_PROMOTE_AFTER` — operator confirms via `mdemg ft-loop promote`
- Emit alert with promote path if verdict is PROMOTE

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit** (existing pipeline scripts already covered by MODEL-DIST-002 tests; nothing new to write)

**Tier 2 — Integration** (Epic 3 3-row hand smoke; validates conversion pipeline preserved adapter behavior across the MLX → GGUF format shift)

**Tier 3 — Live e2e** (Epic 4 50-row UBENCH; real HTTP against real llama-server on real adapter-loaded model against real UBENCH runner)

## 7. Commit Strategy

- Single squash-merge commit at end of sprint; PR opened auto by push-to-dev workflow
- Working commits on `reh3376_dev01`: (a) sprint dir added, (b) UBENCH result files added, (c) sprint post added
- Adapter artifacts (.safetensors, .gguf-lora) NOT committed — .gitignored per prior sprints; regenerable
- If PROMOTE verdict: separate follow-up commit wires the adapter into launchd plist (operator sign-off first)

## 8. Verification Checklist

- [ ] E1: GGUF LoRA file exists at `.local-models/claude_docs_002.gguf-lora`, size 100-200 MB range
- [ ] E2: `curl :8106/v1/models` returns 200 with adapter-served model
- [ ] E3: 3-row hand smoke — outputs bounded < 3× reference length on all 3 fixtures
- [ ] E4: UBENCH result file exists; specs_with_matched_rows=1/1 for claude.code_knowledge; zero_success=0
- [ ] E5: Comparison table in sprint post with per-metric mean deltas vs baseline
- [ ] E6: Sprint post exists; verdict named; operator promotion command spelled out (if applicable)
- [ ] Live smoke: at least one adapter-answered claude.code_knowledge row inspected by hand for factual correctness

## 9. Documentation Update (final epic — never cut)

- `docs/development/claude-docs-training-004/sprint_post.md` — REQUIRED
- `CLAUDE.md` — pin arch rule ONLY IF verdict is PROMOTE
- `CHANGELOG.md` — Unreleased entry: "CLAUDE-DOCS-TRAINING-004: [verdict] on adapter_002+rp=1.15 vs baseline claude.code_knowledge"
- No feature doc update (adapter is not a shipped feature until promotion)

## 10. Risks & Mitigations

- **R1 (M)**: `convert_lora_to_gguf.py` pinned to llama.cpp b9000 may fail against b10450 GGUF base
  - Mitigation: fall back to `--HEAD` version of the convert script if needed; sprint MODEL-DIST-002 provides recipe
- **R2 (L)**: 50-row eval at 180s per call could hit 150 min wall — too slow to iterate
  - Mitigation: 14B baseline achieves ~50 tok/s; a 3000-token claude.code_knowledge answer = ~60s per call; 50 × 60s = 50 min max. Fits.
- **R3 (M)**: Adapter's Golden #3 infinite-loop failure mode from Sprint 002 could re-surface at scale even with rp=1.15
  - Mitigation: per-row output length tracked in UBENCH `truncated_rows` field; if > 5% rows truncate, revisit
- **R4 (L)**: Production `:8102` GPU contention slows the eval to unusable pace
  - Mitigation: Sprint 003 A/B showed 3.6 llama-server can co-serve at 9-10 tok/s under production load; 50 rows × ~2 min each = ~1h 40m. Acceptable.

## 11. Rollback Procedures

Zero rollback needed for the eval itself (read-only against out-of-band side-port).

For a hypothetical PROMOTION rollback (if operator flips `mdemg ft-loop promote` and later regrets):
- FT-RECURSIVE-003 shipped 3-layer promotion defense already handles auto-rollback tripwires
- Manual revert: `ln -sf <prior-serving-symlink-target> .local-models/serving/current.gguf` + launchd kickstart
- Adapter file removal: `rm .local-models/claude_docs_002.gguf-lora`

## 12. Documents Accessed

- `docs/development/claude-docs-training-003/sprint_post.md` — invalidated 2-epoch hypothesis + reconfirmed adapter_002+rp=1.15 as best config
- `docs/development/claude-docs-training-002/sprint_post.md` + `sprint_plan.md` — chunking rationale, 3-epoch training config, Golden #3 infinite-loop failure mode
- `docs/development/claude-docs-training-001/sprint_post.md` — original 14B FT baseline + ULTS registration
- `docs/development/model-dist-002/sprint_plan_model_dist_002.md` — MLX → PEFT → GGUF LoRA pipeline recipe
- `docs/tests/ults/specs/claude_code_knowledge.ults.json` — 50-row golden holdout definition + eval rubric
- Task #91 (MODEL-SWAP-QWEN27B-EVAL) results — 27B FOSS scored 0.000 too (base-model swap rule-out)
- `scripts/mlx_adapter_to_peft.py`, `scripts/vendor/llama_cpp/convert_lora_to_gguf.py` — shipped pipeline
- `neural/benchmarks/run_benchmark.py` — UBENCH runner internals
- `training_data/eval/qwen27b-bakeoff/baseline_20260817.json` — baseline 0.000 reference
