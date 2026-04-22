# Sprint FT-LORA-DATA — Phase 5 Dataset Curation

## Context

Phase 5 dataset pre-flight (`aaa646e`) returned verdict **BLOCKED** for five reasons: 4 tasks have 0 rows in the 21-day window (consulting.synthesis, metalearn.generalize, retrieval.rerank_nli, summarize.generate); 12 tasks miss the planner's 500-row floor; ape.reflect's 34,093-row contribution is templated single-instance redundancy per diversity audit D-X5; jiminy.evaluate_llm at 45 rows would be 4.4× duplicated at a 200-floor; and the current curated file lacks full provenance fields (`task_name`, `trace_id`, `instance_id`, `space_id`, `dataset_ver`, `quality_source`).

Sprint FT-LORA-DATA is the last sprint before Phase 5 SFT. It produces the four training-ready datasets Phase 5 SFT consumes — `tier1/` plus three family dirs — each stratified 90/10, provenance-preserving, SHA256-stamped, dual-pinned to Sprint C's model config and to the raw input's SHA (computed at Epic 1 Step 0 of this sprint).

**Policy update (2026-04-22):** OpenAI fine-tuning (FT-OAI-003) is dropped entirely — not merely deferred. All fine-tuning work now targets local MLX LoRA on Qwen3.6-35B-A3B. No gpt-4.1-mini or gpt-5.4-mini *training* spend going forward. OpenAI API is still used as a **teacher** for synthesis (200 rows × 2 tasks = ~$0.23) but no fine-tuned OpenAI model is a deliverable.

**Sprint chain:** A(#335) → B(#336) → C(#338/339/340) → D(#343) → E(#344, merged `14cd2b3`) → pre-flight (`aaa646e`, BLOCKED) → **DATA (this sprint)** → Phase 5 runbook → Phase 5 SFT → F.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-DATA |
| Title | Phase 5 dataset — re-curate, synthesize absent tasks, balance, split, pin |
| Date | 2026-04-22 (plan) |
| Branch | `reh3376_dev01` |
| Predecessors | FT-LORA-E (PR #344, merged `14cd2b3`); Phase 5 dataset pre-flight (`aaa646e`, BLOCKED) |
| Successors | Phase 5 SFT runbook (unblocks on merge) |
| Type | Code + data + docs (real teacher spend: ~$0.70 expected across 5 teacher tasks; $100 hard cap per user directive 2026-04-22) |
| Risk | Medium (real OpenAI teacher spend, small; metalearn weak-signal; 4.4× duplication ceiling on jiminy.evaluate_llm; observability blindness in distill_driver surfaced 2026-04-22 — addressed in new Epic 6.0) |
| Budget | **$100 hard abort cap** (raised 2026-04-22 from plan-draft $0.50 per user: "its silly to worry about it. If cost will exceed $100 I want to know, otherwise dont worry about the llm call budget"); ~$0.70 expected spend (4 OpenAI teacher tasks × 200 rows, includes Epic 6.0 ~10-row smoke + full run) |
| **Model config.json SHA pin** | `cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734` (Sprint C) |
| **Raw dataset SHA pin** | `7caebf75fd59da37221acef887dc822ac9b80d04e19c19b750dd9a4e5eceb988` — computed 2026-04-22 at Epic 1 Step 0 against `training_data/raw/extracted/llm_interactions.jsonl` (21-day window 2026-03-31 → 2026-04-20, 42,727 rows). All downstream tools default `--expected-raw-sha256` to this value. |
| **Teacher temperature** | `0.7` (verified from `neural/training/teacher_distill.py:242`; matches Sprint C Gate 2 canonical J-group; no change) |
| **Synthesis version** | `v1-{commit_sha_short}` — stamped per-row; no retroactive relabeling (see §3) |
| **Canonical local MLX port** | `:8101` on `127.0.0.1` (pinned 2026-04-22; Docker's `com.docker.backend` reserves `:8100` via IPv6 LISTEN so MLX cannot bind there). CMS `project` memory pins this; pre-flight enforces exactly one `mlx_lm.server` instance. Change requires sprint-plan update + CMS re-pin. |
| **Base teacher model (MLX)** | `mlx-community/Qwen3.6-35B-A3B-mxfp4` (Sprint C SHA-pinned). NEVER overwritten or reused for fine-tuned variants. |
| **Post-fine-tuning model namespace** | `mdemg/qwen3.6-35b-a3b-mdemg-v{N}` — distinct from base to eliminate confusion (user directive 2026-04-22). Applies to Sprint E Tier 1 + Tier 2 adapters and any merged checkpoints downstream. CMS `project` memory reserves this namespace. |
| Artifacts | 4 Python modules + tests + 4 curated dataset dirs + manifest + pre-flight-post markdown + docs + **Epic 6.0 observability hardening** (new) |

## 2. Problem Statement

Five blockers identified in pre-flight `aaa646e`:

1. **4 absent tasks** (0 rows in 21 days): consulting.synthesis, metalearn.generalize, retrieval.rerank_nli, summarize.generate — each gated by a config flag that's off by default, architecturally present (pipeline exists in `internal/{consulting,metalearn,retrieval,summarize}/`). Cannot activate retroactively for 21 days of past traffic.
2. **12 tasks below planner's 500-row floor**. Re-evaluated at 200-row floor; still 8 miss it.
3. **ape.reflect diversity D-X5**: 34,093 rows from 1 instance, 1 space, 1 session, 2 system-prompt hashes, all user prompts length p25=13,259 / p50=13,273 / p75=13,309 chars (templated rolling stats). More rows ≠ more signal — 500-row target replaces 2,000.
4. **jiminy.evaluate_llm**: 45 rows → 200 floor = 4.4× duplication. At 5× ceiling; flag as highest-factor row in manifest.
5. **Provenance gap**: current curated file drops `task_name`, `trace_id`, `instance_id`, `space_id`, `dataset_ver`, `quality_source`. Manifest and Sprint F adapter-regression debugging need these.

This sprint produces:
- `training_data/sft/tier1/{train,valid}.jsonl + manifest.json` — 3,500 rows (15×200 + 1×500 ape.reflect) for the universal attn+shared-expert adapter.
- `training_data/sft/family_reasoning_think/{train,valid}.jsonl + manifest.json` — 1,700 rows (6 non-ape-reflect T-tasks × 200 + ape.reflect × 500).
- `training_data/sft/family_classify_notink/{train,valid}.jsonl + manifest.json` — 1,200 rows (6 × 200).
- `training_data/sft/family_structured_notink/{train,valid}.jsonl + manifest.json` — 600 rows (3 × 200).

## 3. Scope & Constraints

**In scope (deliverables):**

| # | Deliverable | Path |
|---|---|---|
| 1 | Provenance-preserving re-curation | `neural/training/recurate.py` (new) |
| 2 | Teacher-routing distillation driver | `neural/training/distill_driver.py` (new; thin orchestrator over existing `teacher_distill.py`) |
| 3 | Balanced sampler (pre-processing, per-tier) | `neural/training/balanced_sampler.py` (new) |
| 4 | Stratified 90/10 splitter + SHA256 stamping | `neural/training/stratified_split.py` (new) |
| 5 | Unit tests | `neural/training/tests/{test_recurate.py, test_distill_driver.py, test_balanced_sampler.py, test_stratified_split.py}` |
| 6 | Integration test + E2E fixture | `neural/training/tests/test_data_pipeline_integration.py`, `scripts/sprint_ft_lora_data_e2e.sh` |
| 7 | Sprint plan (this file → repo) | `docs/development/ft-lora/sprint_plan_ft_lora_data.md` |
| 8 | **Hand-written** post-run pre-flight report | `docs/development/ft-lora/phase_5_dataset_preflight_post.md` |
| 9 | Doc updates | `00_README_v2.md` v5.4→v5.5; `03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X`; `AGENT_HANDOFF.md`; `CHANGELOG.md` |
| 10 | Curated datasets | `training_data/sft/{tier1,family_reasoning_think,family_classify_notink,family_structured_notink}/{train,valid}.jsonl + manifest.json` |

**Out of scope / dropped:**
- **FT-OAI-003 (OpenAI fine-tuning calibration run) — DROPPED** (user direction 2026-04-22). All fine-tuning work targets local MLX LoRA. OpenAI API is used only as a **teacher** for synthesis, not as a fine-tuned-model deliverable. Memory entry `project_ft_oai_003_deferred.md` to be removed post-approval.
- **Automated pre-flight script** — ~200-300 LOC with its own test coverage, outside sprint budget. Queued to a future cleanup sprint per Revision 3. `AGENT_HANDOFF.md` entry: "Pre-flight automation deferred to future cleanup sprint."
- GRPO/DPO dataset curation (Phase 6+).
- Phase 5 runbook authoring (separate successor task).

**Constraints:**
- **Budget policy: $100 hard abort cap** (user directive 2026-04-22, supersedes plan-draft $0.50). Applied consistently at three sites:
  - §3 Constraints (this line).
  - `distill_driver.py` CLI flag `--budget-cap-usd` default = `100.00`.
  - Epic 2 Gate + Epic 6 acceptance: aggregate OpenAI spend ≤ $100.
  - Expected spend: ~$0.70 across 5 teacher tasks (4 absent + hidden.summarize) × 200 rows + Epic 6.0 ~10-row smoke. Cap is a safety net against runaway token cost, not a plan-tight ceiling.
- **Canonical local MLX endpoint: `http://127.0.0.1:8101/v1`** (pinned 2026-04-22). Only ONE `mlx_lm.server` instance may run. Epic 6.0 pre-flight enforces single-instance via `ps -Ao pid,command` check; stale :8200 (PID 70990) killed as prerequisite. Change of port or model requires sprint-plan update + CMS re-pin.
- **Base teacher model — NEVER overwritten:** `mlx-community/Qwen3.6-35B-A3B-mxfp4` (Sprint C SHA `cdc167566e…`). Used only for Epic 2 synthesis as teacher. Fine-tuned variants write to a **separate** namespace `mdemg/qwen3.6-35b-a3b-mdemg-v{N}` (user directive 2026-04-22 — "change the model name once we start fine-tuning so there is NO confusion"). Sprint E onward enforces the new namespace.
- **CMS pin observations** (recorded by Epic 6.0 Step 8, obs_type=`constraint`, space_id=`mdemg-dev`):
  1. "Local Qwen3.6-35B-A3B MLX teacher endpoint is `http://127.0.0.1:8101/v1`. Only one instance. Never share port with Docker backend."
  2. "Base teacher model `mlx-community/Qwen3.6-35B-A3B-mxfp4` is read-only. Fine-tuned variants land under namespace `mdemg/qwen3.6-35b-a3b-mdemg-v{N}`. Do not overwrite or re-use the base name."
- **Row targets**: 200-row floor on all 16 tasks; ape.reflect target = 500; duplication factor ceiling = 5×.
- **90/10 stratified split** per task (min 2 valid rows per task when count ≥ 20; otherwise count-wise split).
- **ULTS valid-rate ≥ 95%** per output dataset (schema-level; Q7 held — tightening to 98% would abort on noise without catching semantic metalearn weakness, which lives in Sprint F eval).
- **Model config.json SHA pin** (Sprint C) asserted on every tool invocation touching training-data assumptions.
- **Raw dataset SHA pin** (computed Step 0) asserted by `recurate.py`, `distill_driver.py`, `balanced_sampler.py`, `stratified_split.py` — drift aborts with field-level diff.
- **No hardcoded values** (MEMORY) — all thresholds (floors, caps, temperature, seed, duplication ceiling, budget cap) exposed as CLI flags with env fallbacks and sprint-plan-pinned defaults.
- **Sequential epics** (MEMORY).
- **Single batched commit at sprint close** (MEMORY).
- **3-tier testing** (unit/integration/e2e) (MEMORY).
- **Epoch cap 3, no `auto`**, SFT early-stop policy (memo 07 v3.1 + FT-OAI-001) — **not enforced here** (Sprint E owns), but dataset sizes support it.

**Synthesis versioning policy (Refinement 8c):**
- `synthesis_version = v1-{commit_sha_short}` stamped per-row in manifest for this sprint.
- Any future v2 synthesis pass (e.g., post-Sprint-F remediation) produces `synthesis_version = v2-{new_sha_short}` and creates a **separate** balanced corpus.
- **No retroactive relabeling** of v1 rows.
- v1 and v2 rows may coexist in a training corpus only if that sprint's decision explicitly merges them (manifest records the merge).

**Determinism scope (Refinement 6):**
- Determinism gate applies **only from balanced_sampler onward** given fixed synthesis outputs.
- `Epic 2 synthesis is non-deterministic` (teacher temperature=0.7).
- Full end-to-end pipeline SHA is **NOT** bit-reproducible.
- Gate language: "Re-run with identical seed AND identical input JSONLs produces identical SHA256."
- §10 Risks row on re-run SHA drift references this clarification.

## 4. Dependencies

**Consumed:**
- `training_data/raw/extracted/llm_interactions.jsonl` (42,727 rows; 21-day window 2026-03-31 → 2026-04-20). **SHA computed and pinned at Epic 1 Step 0** (§1 Header).
- `training_data/curated/sft_interactions/filtered.jsonl` (existing, lacks full provenance — re-curated in Epic 1).
- `neural/training/teacher_distill.py` (existing 430-line engine):
  - Lines 95–193: INPUT_TEMPLATES + VOCABULARY cover all 4 absent tasks.
  - **Line 242**: `"temperature": 0.7` — verified; matches Sprint C Gate 2 canonical J-group; no change needed.
  - Line 246: `chat_template_kwargs.enable_thinking` for think_mode.
  - Lines 278–294: `validate_output()` — ULTS schema validation.
  - Lines 300–370: `distill_task()` — per-task driver (wrapped by `distill_driver.py`).
- `docs/development/ft-lora/05_DATA_COLLECTION_v2.md` Appendix A (balanced-sampling recipe; Path A upsample/downsample).
- `mlx_lm 0.31.2` `tuner/datasets.py` (verified: no per-example weight surface; `load_local_dataset` expects directory with `train.jsonl` + `valid.jsonl` per lines 217–218).
- Sprint C model at `$HOME/.cache/huggingface/hub/models--mlx-community--Qwen3.6-35B-A3B-mxfp4`, config.json SHA `cdc167566e…`.
- Sprint D family-naming (`profile_routing_{reasoning_think,classify_notink,structured_notink}.json`) — not mutated; naming alignment only.

**External services:**
- OpenAI API (`gpt-5.4-mini`) — **teacher only, not fine-tuning target** — for consulting.synthesis + metalearn.generalize (200 rows each, ~$0.23 total).
- Local Qwen3.6 MLX endpoint (`http://localhost:8100/v1`) for retrieval.rerank_nli + summarize.generate (200 rows each, $0).

No network access beyond those two endpoints. No DB writes.

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch on `reh3376_dev01`, tree clean (ignore pre-existing untracked `scripts/tsdb_data_review_2026-04-01.json`); venv `mdemg-ft-lora` active; `python -c "import mlx_lm; assert mlx_lm.__version__=='0.31.2'"` passes; pre-flight baseline `phase_5_dataset_preflight.md` (`aaa646e`) present; Sprint C model SHA matches pin.

### Epic 1 — Provenance-preserving re-curation (`recurate.py`)

**Step 0 — Compute + pin raw dataset SHA (Blocking Revision 1):**
- Read `training_data/raw/extracted/llm_interactions.jsonl`; compute SHA256.
- Edit `docs/development/ft-lora/sprint_plan_ft_lora_data.md` §1 Header "Raw dataset SHA pin" row, replacing `TBD-populated-at-Epic-1-Step-0` with the computed hex.
- Stage only this doc edit (no code yet); commit message WIP lives in the final batched commit.
- This pin becomes the default for every `--expected-raw-sha256` flag downstream.

**Steps 1–N:**
1. Parse CLI: `--raw-input`, `--out`, `--expected-raw-sha256 <default: sprint-plan pin>`, `--dataset-ver`, `--min-quality` (default 0.6).
2. Assert raw SHA matches pin; abort with field-level diff on mismatch.
3. Stream raw JSONL; for each row:
   - Preserve `task_name`, `trace_id`, `instance_id`, `space_id`, `dataset_ver`, `quality`, `quality_source`, `source_path`.
   - Construct chat messages from `system_prompt` + `user_prompt` + `response`.
   - Emit: `{"messages": [...], "meta": {"task_name": ..., "trace_id": ..., "instance_id": ..., "space_id": ..., "source": "real", "dataset_ver": ..., "quality": ..., "quality_source": ...}}`.
4. Filter by `quality >= --min-quality`; record drop counts per task.
5. Write `training_data/curated/sft_interactions_v2/recurated.jsonl`.

**Gate:**
- Output row count ≤ raw row count; drop log per-task present.
- Every row has non-null `meta.task_name` and `meta.trace_id`.
- Unit test: synthetic 100-row fixture round-trips; quality-filter boundary behavior correct.
- SHA drift test: pre-mutated copy of raw input refuses to start with informative diff message.

### Epic 2 — Teacher-routing distillation (`distill_driver.py`)

Thin orchestrator over `teacher_distill.py` for the 4 absent tasks.

**DEFAULT_TEACHER_CONFIG:**
```python
{
  "consulting.synthesis":  {"teacher": "gpt-5.4-mini",  "endpoint": "openai",    "count": 200, "weak_signal": False},
  "metalearn.generalize":  {"teacher": "gpt-5.4-mini",  "endpoint": "openai",    "count": 200, "weak_signal": True},
  "retrieval.rerank_nli":  {"teacher": "qwen3.6-local", "endpoint": "mlx_local", "count": 200, "weak_signal": False},
  "summarize.generate":    {"teacher": "qwen3.6-local", "endpoint": "mlx_local", "count": 200, "weak_signal": False},
}
```

**CLI:** `--task <name>` (one at a time, or `--all`), `--out <path>`, `--budget-cap-usd <default 0.50>`, `--expected-raw-sha256 <default: sprint-plan pin>`, `--dry-run` (estimates tokens + cost without API calls), `--sleep-ms <default 200>`.

**Steps:**
1. Validate SHA pin.
2. For each task, load template + vocabulary from `teacher_distill.py`.
3. Run `distill_task()`; accumulate token counts + running cost estimate.
4. After every row: if cumulative estimated cost crosses `--budget-cap-usd`, abort with partial-output preservation + breakdown.
5. For each emitted row: append `meta` with `task_name`, `trace_id = cuid2()`, `instance_id = "synth-{teacher_id}"`, `space_id = "synth"`, `source = "synth-gpt-5.4-mini"` or `"synth-qwen-local"`, `synthesis_version = "v1-{commit_sha_short}"`, `weak_signal = True/False`, `dataset_ver = <arg>`.
6. Run ULTS `validate_output()`; record valid/invalid counts per task; if valid-rate < 0.95 warn (do NOT abort; per Q7).
7. Write 4 files: `training_data/curated/sft_synth_v1/{consulting_synthesis,metalearn_generalize,rerank_nli,summarize_generate}.jsonl`.

**Gate:**
- Aggregate spend ≤ $0.50 (budget cap honored).
- Each of 4 files has exactly 200 rows.
- ULTS valid-rate ≥ 95% per task (warn only at lower; gate does not abort at 95%, only records).
- metalearn.generalize rows carry `weak_signal: True`.
- Every row has `synthesis_version` field.
- Unit test: `--dry-run` against 4 tasks estimates token cost without network call.
- Unit test: budget-cap abort path produces partial output + breakdown.

### Epic 3 — Balanced sampler (`balanced_sampler.py`)

Pre-processing sampler (Appendix A Path A). NOT a mlx_lm patch.

**Signature:**
```python
def balanced_sample(rows, per_task_target: dict[str, int], seed: int = 42,
                    duplication_ceiling: int = 5) -> tuple[list[dict], dict]
```

**TIER_TASKS mapping** (embedded; single source of truth):
- `tier1`: all 16 task names.
- `T` (reasoning-think, 7 tasks): ape.reflect + 6 others.
- `C` (classify-notink, 6 tasks).
- `J` (structured-notink, 3 tasks).

**CLI:** `--tier {tier1,T,C,J}`, `--input-real <recurated.jsonl>`, `--input-synth-dir <sft_synth_v1/>`, `--per-task-floor 200`, `--ape-reflect-target 500`, `--duplication-ceiling 5`, `--seed 42`, `--expected-raw-sha256 <default: sprint-plan pin>`, `--out <out.jsonl>`.

**Behavior per task:**
- `count < target`: upsample via integer duplication; if `target/count > duplication_ceiling`, ABORT with task name + factor.
- `count > target`: `random.sample(rows, target)` with seeded RNG.
- `count == target`: pass-through.

**Post-sample**: record `duplication_factors` dict per task in returned metadata.

**Gate:**
- Unit test: synthetic 16-task corpus exercises upsample / downsample / pass-through paths; per-task counts match target exactly.
- Unit test: duplication-ceiling breach (e.g., jiminy.evaluate_llm at 30 rows → 200 target = 6.7×) raises with task name and factor.
- Unit test: seeded determinism — same seed + same input JSONL → identical SHA256 of output.
- **Cross-check T-family arithmetic** (Refinement 5): 6 non-ape × 200 + ape × 500 = **1,700 total** (NOT 1,900; ape.reflect is 1 of 7 T-tasks). Test asserts exact total.

### Epic 4 — Stratified 90/10 splitter (`stratified_split.py`)

**Step 1**: Consume `balanced_sample` output; for each task, partition 90/10 with seeded shuffle; enforce min 2 valid rows per task when count ≥ 20, else count-wise.

**Step 2**: Write `train.jsonl` + `valid.jsonl` under the tier/family directory. Emit `manifest.json`:
```json
{
  "sprint": "FT-LORA-DATA",
  "generator_sha": "<commit_sha>",
  "tier": "tier1|T|C|J",
  "family_name": "tier1|reasoning_think|classify_notink|structured_notink",
  "row_counts": {"train": N, "valid": M, "total": N+M},
  "per_task_counts": {"task.name": {"train": n, "valid": m, "total": n+m}, ...},
  "source_composition": {"real": N1, "real-upsampled": N2, "synth-gpt-5.4-mini": N3, "synth-qwen-local": N4},
  "weak_signal_rows": N5,
  "ape_reflect_target": 500,
  "duplication_factors": {"task.name": factor, ...},
  "seed": 42,
  "file_sha256": {"train.jsonl": "<sha>", "valid.jsonl": "<sha>"},
  "base_dataset_ver": "<recurate --dataset-ver arg>",
  "trained_against_model_sha": "cdc167566e…",
  "raw_dataset_sha_pin": "<from §1>",
  "ults_spec_versions": {...},
  "synthesis_version": "v1-{commit_sha_short}",
  "synthesis_non_determinism_note": "Teacher temp=0.7; Epic 2 outputs are non-reproducible bit-identical. Determinism applies from Epic 3 onward given fixed Epic 2 outputs."
}
```

**Step 3** — Full-tier output:
- `tier1/` — ~3,500 rows → ~3,150 train + ~350 valid.
- `family_reasoning_think/` — **~1,700 rows** (6 non-ape × 200 + ape × 500) → **~1,530 train + ~170 valid** (Refinement 5).
- `family_classify_notink/` — ~1,200 rows → ~1,080 train + ~120 valid.
- `family_structured_notink/` — ~600 rows → ~540 train + ~60 valid.

**Step 3a — Early mlx_lm fixture check (Refinement 4):**
- Before running full-scale split, generate a 20-row fixture with `meta` sidecar field populated (heterogeneous task mix).
- Invoke `python -m mlx_lm.lora --data <fixture_dir> --dry-run` against the fixture.
- **If exit == 0**: proceed with meta-embedded format for full-scale Tier 1 / families.
- **If exit != 0**: activate sibling-file fallback:
  - Strip `meta` from `train.jsonl` + `valid.jsonl`.
  - Write `meta.jsonl` as sibling under the same directory, with parallel row ordering (one meta entry per train/valid row, in emission order).
  - Document the fallback activation in `manifest.json` (`meta_placement: "sidecar"` vs `"embedded"`).
- **Cost of early trigger**: ~20 rows. If the schema-rejection risk were caught at 3,500 rows, re-serialization cost would be ~175× higher.

**Gate:**
- Epic 4 Step 3a mlx_lm fixture dry-run exits 0 (or fallback activated + documented).
- Per-task valid-row floor respected (2 when count ≥ 20).
- Manifest SHA256 matches on-disk file SHA256.
- All 4 datasets emit to disk with no schema errors.

### Epic 5 — Unit + integration tests

**Tier 1 (Unit):**
- `test_recurate.py`: round-trip, quality filter boundary, SHA drift rejection, provenance field preservation.
- `test_distill_driver.py`: `--dry-run` token estimate, budget-cap abort, synthesis_version stamping, weak_signal flagging.
- `test_balanced_sampler.py`: upsample/downsample/passthrough, duplication-ceiling breach, seeded determinism, T-family total = 1,700.
- `test_stratified_split.py`: 90/10 partition, min-2-valid rule, manifest schema, SHA stamp matches file.

**Tier 2 (Integration):** `test_data_pipeline_integration.py` — end-to-end on synthetic 16-task corpus (~500 rows); runs recurate → (mock synthesis substituted for Epic 2) → balance → split; asserts 4 output dirs present, manifest schemas valid, per-task counts match targets.

**Tier 3 (E2E):** `scripts/sprint_ft_lora_data_e2e.sh` — same as integration but against real recurate output + mocked synthesis outputs (saved fixtures from Epic 2 dry-run), plus Epic 4 Step 3a mlx_lm fixture dry-run.

**Gate:** all three tiers green; `pytest -xvs neural/training/tests/` clean; E2E script exit 0; no untracked build artifacts.

### Epic 6.0 — Execution Stabilization (NEW — added 2026-04-22)

**Problem observed 2026-04-22 during Epic 6 smoke test:** distill_driver.py ran for 5+ minutes emitting zero visible rows with no progress output, while the MLX server log showed only `/v1/models` GETs (no `/v1/chat/completions`). Root cause: the driver has multiple silent-failure paths (API errors counted but not logged; schema-invalid rows dropped without payload; no per-row output; no fout.flush()), so the operator cannot distinguish "working slowly" from "failing on every row." Two MLX instances were also running simultaneously (:8101 active + :8200 stale PID 70990), muddling diagnostics.

Epic 6.0 proactively addresses observability + environment hygiene **before** the real distillation run. Scope is contained: no behavior change to the synthesis output, only new logging/pre-flight/diagnostic surfaces.

**Work items:**

1. **Per-row structured logging with flush** — in `distill_driver.py` main loop:
   - On every row emit one line to stderr: `[{task} {i+1}/{count}] status=ok|api_error|schema_invalid|budget_abort cost=${cum:.4f} tokens_in={n} tokens_out={m} elapsed_ms={t}`.
   - Call `fout.flush()` after each successful write so row count on disk reflects real-time progress.
   - On non-ok status, log the first ~500 chars of the response payload (redact any keys) to aid debugging.

2. **Endpoint pre-flight assertion** — before the first request per task, issue a `GET {base_url}/models` (5s timeout) to confirm endpoint reachability. Reuse the exact pattern from `scripts/test_vllm_mlx.py:243-251`. On failure: abort with a clear "MLX endpoint unreachable at {url}" message instead of silently failing 200 rows.

3. **MLX single-instance enforcement** — pre-flight scans `ps -Ao pid,command | grep mlx_lm.server` (or equivalent via `psutil`). If >1 instance is running, ABORT with the list of PIDs and the expected canonical `:8101` instance. Operator must kill stragglers (e.g., stale :8200 PID 70990) before proceeding.

4. **Response payload capture on failure** — when HTTP status != 200 or schema validation fails, record `{"status_code": N, "body_preview": "...", "request_body_summary": {...}}` to a `--debug-log <path>` file (default `{out_dir}/_debug.jsonl`). Non-fatal; helps post-mortem without requiring re-run.

5. **HTTP timeout + retry policy** — add `--http-timeout-s` flag (default 60). On timeout / 5xx / 429: retry up to 3 times with exponential backoff (1s, 2s, 4s). Count retries toward the per-task budget but NOT toward the row count.

6. **`--count N` override** — CLI flag to override the 200-row-per-task target. Used for smoke tests (e.g., `--count 10`) without modifying `DEFAULT_TEACHER_CONFIG`. Default: `None` (use config value).

7. **`--strict` mode** — when set, any non-ok status aborts the task instead of continuing silently. Off by default (production runs tolerate sparse failures); on for smoke tests so issues surface immediately.

8. **CMS observations** — Epic 6.0 Step 8 records two `obs_type=constraint` observations to `space_id=mdemg-dev` via `POST /v1/conversation/observe`:
   - "Local Qwen3.6-35B-A3B MLX teacher endpoint is pinned to `http://127.0.0.1:8101/v1`. Single-instance enforcement required. Docker backend reserves :8100 via IPv6."
   - "Base teacher model `mlx-community/Qwen3.6-35B-A3B-mxfp4` is read-only. Fine-tuned variants use namespace `mdemg/qwen3.6-35b-a3b-mdemg-v{N}`. Never overwrite or re-use the base name."
   These surface on future session-start/prompt-context hooks so a fresh Claude session cannot misconfigure port/model.

9. **Live integration test** — new `neural/training/tests/test_distill_driver_live.py` with `@pytest.mark.skipif(os.environ.get("MDEMG_LIVE_MLX") != "1", reason="live test")`. Exercises:
   - Pre-flight passes against running :8101 MLX instance.
   - A 2-row-per-task run against `retrieval.rerank_nli` emits 2 valid rows with response text length > 0.
   - Single-instance guard fails fast when a fake second `mlx_lm.server`-like process is spawned.

10. **Unit tests** for the new paths — in `test_distill_driver.py`:
    - Per-row log line format + flush call.
    - Pre-flight fails when `/models` returns 404 (mocked).
    - Multi-instance detection returns 2 → ABORT.
    - `--count 3` overrides config value of 200.
    - `--strict` aborts on first api_error.
    - `--http-timeout-s 1` + mocked hanging endpoint triggers retry path.

**Gate:**
- New unit tests green (item 10).
- Live test green when `MDEMG_LIVE_MLX=1` (item 9; skipped in normal CI).
- Smoke test (`--task retrieval.rerank_nli --count 5 --strict`) completes in <60s, emits 5 per-row log lines, produces 5 rows on disk flushed live.
- Pre-flight actively aborts with informative message when stale :8200 instance is running.
- CMS observations retrievable via `POST /v1/memory/recall` with filter matching "MLX teacher endpoint" and "qwen3.6-35b-a3b-mdemg-v".
- Stale MLX :8200 (PID 70990) killed as part of Epic 6.0 execution prerequisite.

### Epic 6 — Real-run execution + manual post-pre-flight verification

**Step 1**: Run `recurate.py` against full raw input. Validate output row counts, per-task distribution against pre-flight baseline.

**Step 2**: Run `distill_driver.py --all` with `--budget-cap-usd 100.00`. Monitor spend via per-row log lines (Epic 6.0 item 1). Expected ~$0.70 across 5 teacher tasks × 200 rows.

**Step 3**: Run `balanced_sampler.py` four times (tier1 + 3 families). Validate per-task counts, duplication factors.

**Step 4 — Manual pre-flight verification (Blocking Revision 3):**
- Execute pre-flight verification **manually** against the 4 output directories (tier1 + 3 families).
- Inspect per-task row counts, schema, split ratios, SHAs.
- Verify all 7 original pre-flight checks against the new corpus.
- **No automation script**; execution is a one-time manual inspection.

**Step 5 — Hand-write `phase_5_dataset_preflight_post.md`:**
- Follow the **exact section structure and table formatting** of `docs/development/ft-lora/phase_5_dataset_preflight.md` (committed `aaa646e`).
- Verdict field: **CLEAR** or **BLOCKED** — no intermediate states.
- On BLOCKED: document the failure mode, list remediation options, do NOT merge the sprint. (If Epic 6 produces BLOCKED, sprint ends in a separate triage sub-sprint before merge.)
- On CLEAR: proceed to Epic 7.

**Step 6 — Production mlx_lm.lora `--dry-run` (Q6):**
- Invoke against the full Tier 1 corpus (~3,500 rows). Catches anything scale-dependent that the 20-row fixture missed.
- Exit 0 is mandatory for sprint merge.

**Gate:**
- Aggregate OpenAI spend ≤ $0.50 (budget cap honored).
- All 4 output dirs present with expected row counts (±5% tolerance).
- `phase_5_dataset_preflight_post.md` verdict = CLEAR.
- Full-scale mlx_lm.lora `--dry-run` exits 0 on Tier 1.

### Epic 7 — Documentation (final epic — never cut)

1. Copy `~/.claude/plans/breezy-dancing-lerdorf.md` → `docs/development/ft-lora/sprint_plan_ft_lora_data.md` (with §1 Header SHA pin populated at Epic 1 Step 0).
2. Append "Documents Accessed" appendix (§11).
3. `00_README_v2.md` v5.4 → v5.5: Document Map row for FT-LORA-DATA plan; status marker "Phase 5 SFT unblocked"; Key Decisions row for mixed-teacher strategy; **key decision row for FT-OAI-003 dropped** (2026-04-22, local MLX-only going forward).
4. `03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X`:
   - Mark FT-LORA-DATA executed; link all 4 new modules + test files + manifest schema.
   - **Cross-link baseline + post reports** (Refinement 8a): `phase_5_dataset_preflight.md` (baseline, `aaa646e`) and `phase_5_dataset_preflight_post.md` (FT-LORA-DATA result). Both are reference artifacts for the Phase 5 runbook.
   - **Replace Sprint E `--dataset` cheat-sheet** (Refinement 8b): single authoritative dir-vs-file guidance; do NOT add alongside the `2335023` version. Remove or supersede Sprint E's cheat-sheet block.
   - Document the directory layout `training_data/sft/{tier1,family_*}/{train,valid}.jsonl` that Phase 5 commands consume.
5. `AGENT_HANDOFF.md` — FT-LORA-DATA completion entry at top; explicit line: "Pre-flight automation deferred to future cleanup sprint."; explicit line: "FT-OAI-003 dropped 2026-04-22 — local MLX LoRA only going forward."
6. `CHANGELOG.md` `[Unreleased] ### Added`:
   - `neural/training/recurate.py` — provenance-preserving re-curation with raw-SHA pin assertion.
   - `neural/training/distill_driver.py` — mixed-teacher orchestrator ($0.50 hard cap).
   - `neural/training/balanced_sampler.py` — per-tier pre-processing sampler.
   - `neural/training/stratified_split.py` — 90/10 stratified splitter + manifest.
   - 4 curated datasets (`training_data/sft/{tier1,family_*}`).
   - `phase_5_dataset_preflight_post.md` — hand-written post-run verification.
7. `CHANGELOG.md` `[Unreleased] ### Removed`:
   - FT-OAI-003 calibration run scope (decision 2026-04-22 — local MLX LoRA only).

**Gate:** sprint plan in repo; baseline + post reports cross-linked; Sprint E cheat-sheet superseded (not duplicated); all cross-refs valid; CHANGELOG + AGENT_HANDOFF current; PR comment draft prepared.

## 6. Testing Plan (Three Tiers)

Covered by Epic 5 (unit + integration) and Epic 6 Step 6 (full-scale production dry-run). Summary:

- **Tier 1 (Static + Unit):** pytest on 4 new test files; ruff + mypy clean; `--help` smoke on all 4 CLIs.
- **Tier 2 (Integration):** `test_data_pipeline_integration.py` — synthetic 16-task corpus end-to-end with mocked synthesis.
- **Tier 3 (E2E):** `scripts/sprint_ft_lora_data_e2e.sh` (integration + Epic 4 Step 3a fixture) and Epic 6 Step 6 production mlx_lm.lora `--dry-run` against full Tier 1 corpus. Both mlx_lm invocations required (Q6).

State restoration (MEMORY): Epic 6 performs real synthesis spend + real dataset writes. Rollback via `git revert` + manual deletion of `training_data/sft/` dirs + `training_data/curated/sft_synth_v1/` + `training_data/curated/sft_interactions_v2/`. Sprint C model untouched. Sprint D artifacts untouched. Raw input untouched.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):

- Title: `feat(ft-lora): Sprint FT-LORA-DATA — Phase 5 dataset curated + pinned + split`
- Body: one bullet per epic + a **"Phase 5 SFT unblock"** section with:
  - Raw SHA pin value (computed Epic 1 Step 0).
  - 4 output dir row counts + per-task breakdown summary.
  - Aggregate OpenAI teacher spend + per-task breakdown.
  - ULTS valid-rate per task.
  - Duplication-factor ceiling findings (jiminy.evaluate_llm highlighted).
  - mlx_lm.lora `--dry-run` results (fixture + full-scale).
  - Pre-flight verdict: CLEAR.
  - Policy note: FT-OAI-003 dropped; local MLX LoRA only going forward.
- Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`

Push to `reh3376_dev01` → auto-PR opens → sprint summary comment posted (MEMORY `feedback_sprint_summary_on_pr.md`) with the same content + "Phase 5 SFT is now unblocked" statement.

## 8. Verification Checklist

- [ ] Pre-gate: branch clean, venv + mlx_lm 0.31.2 + pre-flight baseline present
- [ ] Epic 1 Step 0: raw SHA computed, pinned in §1 Header, doc edit staged
- [ ] Epic 1: `recurate.py` preserves provenance; SHA drift rejection tested
- [ ] Epic 2: 5 teacher tasks synthesized (4 absent + hidden.summarize); $100 cap honored; synthesis_version stamped; weak_signal flagged on metalearn
- [ ] Epic 3: balanced_sampler.py passes upsample / downsample / passthrough / ceiling tests; T-family total = 1,700 asserted
- [ ] Epic 4: stratified_split.py 90/10 + manifest schema; Epic 4 Step 3a mlx_lm fixture dry-run (or fallback activated + documented)
- [ ] Epic 5: 3 testing tiers green
- [ ] Epic 6.0: per-row log visible on smoke test; pre-flight aborts on unreachable endpoint + multi-instance MLX; stale :8200 PID killed; CMS constraint observations recorded (MLX :8101 pin + post-FT namespace); new unit tests + live test green (`MDEMG_LIVE_MLX=1`)
- [ ] Epic 6: real spend ≤ $100 (expected ~$0.70); 4 output dirs written; **hand-written** post-pre-flight = CLEAR; full-scale mlx_lm.lora `--dry-run` exits 0
- [ ] Epic 7: plan in repo; baseline + post reports cross-linked; Sprint E cheat-sheet superseded (not duplicated); AGENT_HANDOFF + CHANGELOG current; "Pre-flight automation deferred to future cleanup sprint" + "FT-OAI-003 dropped 2026-04-22" lines present
- [ ] Post-approval: memory entry `project_ft_oai_003_deferred.md` removed
- [ ] Commit pushed; auto-PR opened; sprint summary comment posted

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 7. Key deliverables: `sprint_plan_ft_lora_data.md`, `00_README_v2.md` v5.4→v5.5, `03_IMPLEMENTATION_PLAN_v2.md` Phase 5.X with baseline+post cross-links and superseded cheat-sheet, `phase_5_dataset_preflight_post.md` (hand-written), AGENT_HANDOFF + CHANGELOG entries (including FT-OAI-003 dropped), Documents Accessed appendix.

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|
| **metalearn.generalize weak-signal regression** — teacher synthesis produces plausibly-formatted but semantically weak corrective signal; post-fine-tuning task performance degrades vs base | Medium | `weak_signal: True` flag per row; manifest records weak-signal row count; Sprint F eval gate will catch on task-level metric | Re-synthesize with stronger prompt engineering (v2 corpus per versioning policy); or escalate to descope if v2 also weak |
| **jiminy.evaluate_llm 4.4× duplication overfit** — 45 rows upsampled to 200 risks overfitting to the 45 unique rows' idiosyncrasies | Medium | Duplication ceiling = 5× (current: 4.4×, at edge); manifest records factor; Sprint F adapter-eval flags if val_loss diverges from train_loss | Raise floor on this specific task in a future pass; or drop from balanced set |
| **OpenAI teacher spend overruns $100** — token miscount, retries, accidental re-execution | Low | Hard cap raised to $100 per user directive 2026-04-22; `--dry-run` estimate before real run; running-total abort; expected ~$0.70 | Partial-output preservation + manual review of spend breakdown |
| **Observability blindness in distill_driver** — OBSERVED 2026-04-22: 5+ min silent run with zero visible rows, only /v1/models GETs in MLX log, no per-row progress, no flush, API errors silently counted | High (already happened) | Epic 6.0 work items 1+4: per-row structured log with flush; response-payload capture on failure. Epic 6.0 Gate: smoke test must emit 5 log lines in <60s | Fall back to Tier 3 direct teacher_distill.py invocation (has stderr per-row logging at line 347) for one task at a time |
| **Multi-instance MLX contention** — OBSERVED 2026-04-22: stale :8200 PID 70990 running alongside canonical :8101, diagnostics ambiguous | Medium | Epic 6.0 item 3: pre-flight `ps` scan aborts on >1 instance; canonical `:8101` pinned in §1 Header + CMS observation | Manual `kill <pid>` of strays before rerun; never run Docker backend on :8100 → MLX on :8100 |
| **Post-FT model name collision** — fine-tuned variants accidentally write over base `mlx-community/Qwen3.6-35B-A3B-mxfp4` | Medium | Distinct namespace `mdemg/qwen3.6-35b-a3b-mdemg-v{N}` (§1 Header + §3 Constraints); Epic 6.0 item 8 records CMS constraint observation so future sessions cannot misconfigure | Revert via HF cache re-download of base model (Sprint C SHA pin provides verification) |
| **mlx_lm rejects `meta` field in chat rows** — schema-strict fail | Low-Medium | Epic 4 Step 3a early 20-row fixture `--dry-run` catches before full-scale | Sibling `meta.jsonl` format; manifest records `meta_placement` |
| **Raw dataset SHA drift mid-sprint** — someone re-exports raw while sprint in flight | Low | `--expected-raw-sha256` asserted on every tool invocation; abort with field-level diff | Sprint pauses until raw input is reverted OR pin is deliberately bumped in §1 Header |
| **Re-running Epic 6 produces different SHA** — output SHA changes between runs | Low | **Determinism gate applies only from balanced_sampler onward** given fixed synthesis outputs; Epic 2 is non-deterministic (teacher temp=0.7); manifest records `synthesis_non_determinism_note`; `synthesis_version = v1-{commit_sha_short}` pins the specific Epic 2 run | Full bit-reproducibility not a goal; v1 corpus is the canonical reference |
| **ape.reflect 500-row target too high given diversity** — even 500 rows may be redundant | Low-Medium | D-X5 audit shows single-instance templated; memo-specified 500 stands; Sprint F eval will reveal if reduction is warranted | v2 corpus with lower target; no retroactive change |
| **ULTS valid-rate < 95% on synthesis** — format-compliance drift from teacher | Low | Per-task valid-rate recorded; 95% is warn-threshold, not abort-threshold (Q7) | Re-synthesize failing task with adjusted prompt |
| **Post-pre-flight verdict = BLOCKED** | Low (given scope) | Epic 6 Step 5 hand-writes report; verdict binary CLEAR/BLOCKED; BLOCKED → triage sub-sprint, no merge | Sprint does not merge until CLEAR |

## 11. Documents Accessed

**Read during planning:**
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_5_dataset_preflight.md` (baseline, committed `aaa646e`)
- `/Users/reh3376/mdemg/neural/training/teacher_distill.py` — full 430 lines; temperature=0.7 confirmed at line 242
- `/Users/reh3376/mdemg/docs/development/ft-lora/05_DATA_COLLECTION_v2.md` §Appendix A (lines 470-501; Path A balanced-sampling recipe)
- `/Users/reh3376/.venv/mdemg-ft-lora/lib/python3.12/site-packages/mlx_lm/tuner/datasets.py` — 333 lines; confirmed no per-example weight surface; dir-based loader
- `/Users/reh3376/mdemg/docs/operations/campaign-task-activation.md` — 101 lines; absent-task gate config flags
- `/Users/reh3376/mdemg/training_data/curated/sft_interactions/filtered.jsonl` — existing curated dataset; provenance gap audit source
- `/Users/reh3376/mdemg/training_data/raw/extracted/llm_interactions.jsonl` — 42,727 rows; SHA pending Epic 1 Step 0
- `/Users/reh3376/mdemg/internal/config/config.go` (lines 190, 243, 273, 1721, 1900, 1979) — config flag defaults for absent-task gates
- `/Users/reh3376/mdemg/internal/{consulting/synthesis.go, metalearn/generalizer.go, retrieval/rerank.go, summarize/service.go}` — architectural confirmation of absent-task pipelines
- `/Users/reh3376/mdemg/docs/development/ft-lora/00_README_v2.md` — v5.4 → v5.5 bump target
- `/Users/reh3376/mdemg/docs/development/ft-lora/01_RESEARCH_v2.md §5` — MoE-Sieve two-tier strategy
- `/Users/reh3376/mdemg/docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X` — Phase 5 consumer spec
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_ft_lora_e.md` (Sprint E plan; cheat-sheet to supersede per Refinement 8b)
- `/Users/reh3376/mdemg/training_data/routing_profiles/profile_routing_*.json` (3 files; family-naming alignment reference)
- `/Users/reh3376/mdemg/CLAUDE.md` — overfitting policies + MEMORY rules
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` — sequential epics, no hardcoded values, 3-tier testing, sprint plan location, sprint summary on PR, plan-before-code, CUIDv2 required
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/run_notes.md` — FT-OAI-001 step-1200 overfit forcing function

**Referenced but not deeply read (will read at execution):**
- `neural/training/tests/test_*.py` — existing test structure (extend patterns)
- `scripts/` — E2E script location

**Read during Epic 6.0 root-cause analysis (2026-04-22):**
- `/Users/reh3376/mdemg/neural/training/distill_driver.py` lines 258-743 — silent-failure paths at 476-561 (api_error counted silently 520-522; schema invalid silent 524-526; double schema check silent 541-546; no fout.flush() after write 555; no per-row progress 477)
- `/Users/reh3376/mdemg/scripts/test_vllm_mlx.py` lines 243-251 — existing MLX pre-flight health-check pattern reused in Epic 6.0 item 2
- `/Users/reh3376/mdemg/neural/training/teacher_distill.py` lines 340-369 — per-row stderr logging in baseline engine (fallback diagnostic path)
- `/Users/reh3376/mdemg/neural/training/tests/test_distill_driver.py` lines 154-313 — 20 existing unit tests all mocked; confirms absence of live integration test (Epic 6.0 item 9 adds it)

## 12. Rollback

All changes are additive (new files + new datasets + new docs). Rollback = `git revert <sha>` + manual delete of `training_data/sft/`, `training_data/curated/sft_synth_v1/`, `training_data/curated/sft_interactions_v2/`. OpenAI teacher spend is sunk cost (~$0.23); no refund path. No Neo4j / TSDB writes. Sprint C model untouched. Sprint D artifacts untouched. Raw input untouched.

---

## Post-Sprint — Phase 5 Runbook Unblocks

FT-LORA-DATA merge triggers Phase 5 SFT runbook authoring (10 sections). Phase 5 commands consume:

```
adapters/tier1/: --data training_data/sft/tier1/
adapters/tier2_reasoning_think/: --data training_data/sft/family_reasoning_think/
adapters/tier2_classify_notink/: --data training_data/sft/family_classify_notink/
adapters/tier2_structured_notink/: --data training_data/sft/family_structured_notink/
```

Directory-vs-file cheat-sheet in `03_IMPLEMENTATION_PLAN_v2.md §Phase 5.X` (superseding Sprint E's version per Refinement 8b) is the single authoritative reference.

**FT-OAI-003 DROPPED 2026-04-22.** All fine-tuning targets local MLX LoRA on Qwen3.6-35B-A3B. OpenAI API used as teacher only, never as a fine-tuning deliverable. Memory entry `memory/project_ft_oai_003_deferred.md` to be removed post-sprint-approval.
