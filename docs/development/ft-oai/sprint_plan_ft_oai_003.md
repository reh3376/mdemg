# Sprint FT-OAI-003 — Calibration Run + LoRA Reference Point

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | FT-OAI-003 |
| **Title** | Calibration Run + LoRA Reference Point |
| **Date** | 2026-04-21 (drafted — execution gated behind FT-OAI-002 close-out + explicit user spend auth) |
| **Format version** | v1.0 (12-section standard) |
| **Branch** | `reh3376_dev01` |
| **Predecessors** | FT-OAI-001 (complete, PR #332), FT-OAI-002 (complete, PR #333, commit `1329f12`) |
| **Type** | Calibration FT launch — validates pipeline improvements, establishes quality reference point for local LoRA comparison |
| **Owner** | reh3376 |
| **Planning model** | Opus |
| **Target FT training base** | `gpt-4.1-mini-2025-04-14` (cleanest A/B vs FT-OAI-001 — no base-model confound) |
| **Production base (quality target)** | `gpt-5.4-mini` (current `.env` default) |
| **Reference FT model** | `ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai001:DX9KJuuq` (FT-OAI-001) |
| **Task ID** | #143 |

---

## 2. Problem Statement

**Strategic context (2026-04-21 user decision):** OpenAI FT is no longer the primary north-star pathway. Published-pricing analysis (cross-confirmed via OpenRouter + aipricing.guru) shows FT gpt-4.1-mini is 7–26% cheaper than gpt-5.4-mini at expected quality levels — economically viable but the margin is thin and the model is externally hosted. **Local LoRA on a small open-source base model is the larger prize** (10–100× cost advantage at ≥90% quality parity, plus data sovereignty). The `docs/development/ft-lora/` sprint line already has substantial pre-investment; that becomes the primary track.

**What FT-OAI-003 is, under this reframe:** a targeted calibration run — *not* an open-ended quality push. Its job is to (a) prove the FT-OAI-002 pipeline improvements work as modeled, and (b) produce a known-quality OpenAI-hosted reference point that future LoRA runs can be compared against on identical evaluation artefacts.

**What FT-OAI-003 is NOT:**
- Not a deploy commitment — `.env` changes are explicitly out of scope.
- Not an open-ended budget — hard cost cap is set for calibration, not quality maximisation.
- Not a model-choice exploration — `gpt-4.1-mini` is held constant for clean A/B vs FT-OAI-001.

**Concrete deliverables:**
1. **One FT launch** with the four FT-OAI-002 improvements applied (L1 cleanup, L2 n_epochs=2, L3 task-weights, L4 sys-prompt attribution in eval).
2. **Pipeline validation report** — did each improvement move the needle in the expected direction, and by how much?
3. **Reference-point eval artefact** — `eval_reference_point_v3.json` — a frozen cross-base comparison vs `gpt-5.4-mini` at 4096 cap, seeded at n=300, with a stable schema that matches what the local-LoRA eval harness will produce.
4. **LoRA handoff memo** — what to expect; how to produce a comparable artefact for each LoRA candidate; which tasks to prioritise for matching.

**Per-improvement expected directional deltas** (informed by FT-OAI-001 post-mortem + user overfitting finding 2026-04-21):

| Lever | From FT-OAI-002 | Expected direction |
|---|---|---|
| L1 — sys-prompt attribution in eval (not training) | E4 proof: 0 cross-task sha256 collisions across 14 unique prompts | Resolves `__unattributed__` Δ=−0.113 bucket (47 records) — recovers meaningful slice of FT-OAI-001's −0.034 cross-base deficit |
| L2 — `--n-epochs 2` (not auto) | E6/T2 flag shipped | Captures best-val checkpoint (step 1200 in v1) instead of overfitting past it (step 1500 val=0.813 vs 0.684). Also saves ~33% train cost. |
| L3 — `--task-weights` upsampling | E6/T4 flag shipped; integer-only byte-deterministic | Reduces `retrieval.intent_translate` regression (v1: n=4, Δ=−0.079). Target: upsample 8× → ~32 effective records. |
| L4 — `response_text` parse-pass fix | E1/G1a fix shipped | Cosmetic field correction; no training impact, but enables clean reference-point artefact |

**Success = pipeline validated (each lever moves in the expected direction) AND a reference-point artefact exists that LoRA runs can match against.** Not "beat gpt-5.4-mini." Not "deploy to `.env`."

---

## 3. Scope & Constraints

### In scope

- **E1 — Training-data prep**: apply L1 (keep records, add sys-prompt→task attribution pass for baseline eval) + L3 (task-weights for regressed tasks).
- **E2 — Launch config**: `--n-epochs 2 --n-epochs-rationale "FT-OAI-001 best_val_loss=0.684 at step 1200, degraded to 0.813 by step 1500"`.
- **E3 — FT launch**: one job, cost-gated. No quality targets other than the floor.
- **E4 — Reference-point eval**: FT-OAI-003 vs `gpt-5.4-mini` baseline (reusing FT-OAI-002 E3 cap-symmetric output if available; otherwise produced fresh at 4096 cap).
- **E5 — Pipeline-validation report**: per-lever attribution of the observed cross-base Δ. Did L1/L2/L3 each move the number in the expected direction?
- **E6 — LoRA handoff memo**: what a comparable LoRA-side artefact looks like; which tasks to match first; how to compute apples-to-apples quality-per-dollar.
- **E7 — Documentation** (final epic — never cut): CHANGELOG, AGENT_HANDOFF, run_notes, sprint-summary PR comment, feature-doc update.

### Out of scope

- **Deploy decision** — explicit carve-out. No `.env` changes, no per-task rollout. If post-calibration you decide to deploy, that's a separate FT-OAI-004 sprint.
- **Training-base exploration** (`gpt-4o-mini` alternative) — held constant on `gpt-4.1-mini-2025-04-14` for clean A/B.
- **Hyperparameter sweep beyond `--n-epochs`** — one run, fixed config.
- **Local LoRA work** — separate track (`docs/development/ft-lora/`, task #150, #151).
- **Production integration smoke** — N/A (no deploy).
- **Retraining on new TSDB data** — reuse the curated SFT output used for FT-OAI-001 (augmented per L1/L3); keeps this run an A/B of improvements, not a data-refresh confound.

### Hard constraints

- **Cost cap**: ≤ **$80 total** ($60 single FT launch at n_epochs=2 — roughly ⅔ of FT-OAI-001's $93 cost — plus ≤ $15 re-eval, plus buffer). Hard-gated by `--max-cost-usd` on every script. Lower than FT-OAI-001 because: (a) 2 epochs not 3, (b) no `gpt-4o-mini` bench, (c) cap-symmetric baseline may already be produced from FT-OAI-002 E3.
- **Quality floor**: FT-OAI-003 cross-base mean cosine **≥ 0.864** (FT-OAI-001 result). A regression vs the previous FT means the "improvements" regressed — failure mode worth documenting, but still valuable data. *Calibration failure is publishable; quality failure is not a deploy-blocker because there is no deploy.*
- **Parse-pass floor**: ≥ 0.97 on the 300-record seeded sample.
- **`gpt-5.4-mini` never fine-tuned** — held constant as the reference ceiling.
- **Artefact schema stability**: the `eval_reference_point_v3.json` schema must be identical in shape to what the local-LoRA eval harness will produce. No OpenAI-specific fields without a documented LoRA-side equivalent.

---

## 4. Dependencies

**Hard blockers:**
- FT-OAI-002 complete (✅ committed `1329f12`, PR #333, 2026-04-21) — delivers the `--n-epochs`, `--task-weights`, `--sys-prompt-map` flags.
- User explicit spend authorization for a ~$80 calibration run.
- FT-OAI-002 Epic 3 cap-symmetric baseline output (staged pending live-spend auth). If not yet run at FT-OAI-003 kickoff, fold it into Epic 4 here (budget already includes the $15 buffer).

**Soft dependencies:**
- `openai>=1.50` Python SDK (already installed in `neural/.venv`).
- `tiktoken>=0.7` (installed everywhere except the sandbox env where the pre-existing unit test fails).
- Branch `reh3376_dev01` fast-forwarded with `main` post-PR #333 merge.

**Reference artefacts (read-only, already exist):**
- `training_data/openai_ft/20260420/run_notes.md` — FT-OAI-001 + FT-OAI-002 summary.
- `training_data/openai_ft/20260420/eval_comparison_vs_gpt54mini.md` — cross-base target.
- `training_data/openai_ft/20260420/unattributed_investigation.md` — FT-OAI-002 E4.
- `training_data/openai_ft/20260420/intent_translate_investigation.md` — FT-OAI-002 E5.
- `training_data/openai_ft/20260420/manifest.json` — token counts + cost breakdown for comparison.

---

## 5. Implementation Plan

Sequential epics, each with explicit gate. Do **not** parallelize epics (per project rule — documentation before implementation).

### Epic 0 — Readiness gate

- PR #333 merged to `main`; `reh3376_dev01` fast-forwarded
- User spend auth confirmed in writing (slack/message/issue comment)
- OpenAI project quota verified ≥ $100 via `scripts/openai_ft_upload_and_launch.py --quota-buffer 1.66 --max-cost-usd 80` pre-flight
- Cap-symmetric baseline either already run (FT-OAI-002 E3 output exists) OR budget confirms it can be folded into Epic 4
- **Gate**: all four bullets confirmed before Epic 1 begins.

### Epic 1 — Training-data prep

**L1 — sys-prompt attribution (eval-side, not training-side):**
- Build `sys_prompt_to_task.json` from filtered.jsonl: `sha256(system_prompt) → task_name`. Per FT-OAI-002 E4 proof, this is 1:1 across 14 unique prompts, zero cross-task collisions.
- Pass to `openai_ft_adapter.py` via `--sys-prompt-map <path>` during regeneration.
- Apply the same map in the baseline-eval harness to eliminate `__unattributed__` from the reference-point artefact.

**L3 — Per-task upsampling:**
- `--task-weights '{"retrieval.intent_translate": 8, "hidden.name_emergence": 2}'`
- Rationale: v1 had 4 and 13 records respectively; 8× and 2× weights yield ~32 and ~26 effective records — enough for the learner to stop treating them as edge cases. Deterministic integer duplication (per FT-OAI-002 E6/T4).
- Document the weights + rationale inline in the launch command in `run_notes.md`.

**Regenerate adapter output:**
- Run `python -m training.openai_ft_adapter` with the E1 inputs.
- Verify `manifest.json` cost estimate at `n_epochs=2` ≤ $60.
- Verify `task_weights.applied == true` and `task_breakdown` shows post-weight counts.
- Verify no new `rejection_log.jsonl` entries (we are adding, not filtering).

**Gate**: new `combined_train.jsonl` + `combined_val.jsonl` + `manifest.json` produced; manifest cost ≤ $60; task_breakdown reflects weights; rejection log unchanged.

### Epic 2 — Launch config

- Command template staged in `run_notes.md`:
  ```bash
  python scripts/openai_ft_upload_and_launch.py \
    --input-dir training_data/openai_ft/<YYYYMMDD>/ \
    --suffix mdemg-ftoai003 \
    --n-epochs 2 \
    --n-epochs-rationale "FT-OAI-001 best_val_loss=0.684 at step 1200 (~2.4 effective epochs), degraded to 0.813 by step 1500; explicit --n-epochs 2 captures best-generalising checkpoint and saves ~33% train cost" \
    --quota-buffer 1.66 \
    --max-cost-usd 80
  ```
- No other hyperparameter overrides — one-lever-at-a-time discipline.
- **Gate**: command in `run_notes.md` ready to execute; rationale documents each flag choice.

### Epic 3 — FT launch + harvest

- Execute the Epic 2 command.
- Quota pre-check (FT-OAI-002 O3) must pass or produce graceful WARNING.
- Monitor via `scripts/openai_ft_check.py --watch`; alert if queue > 4h (FT-OAI-002 O2).
- On completion, run `scripts/openai_ft_check.py --on-complete --job-id <id> --queue-timeout-minutes 240` to harvest `job_lifecycle.json` + `training_metrics.json`.
- Verify `training_metrics.json:best_val_loss_step` is in the range [600, 1000] (n_epochs=2 ≈ 1000 steps; best-val earlier = improvement over v1).
- Verify `n_epochs_actual == 2` (not auto-inflated).
- **Gate**: job status `succeeded`; model ID recorded in `run_notes.md`; best_val_loss_step within expected range; final train/val losses logged.

### Epic 4 — Reference-point eval

- **4.1** — `gpt-5.4-mini` baseline at 4096 cap, seed=42, n=300 (reuse FT-OAI-002 E3 output if already produced; else produce fresh — budget accounts for it).
- **4.2** — FT-OAI-003 eval at 4096 cap, seed=42, n=300 (same seeded records).
- **4.3** — Both eval runs use the L1 sys-prompt attribution so every result has a resolved `task_name` (no `__unattributed__` bucket).
- **4.4** — `scripts/openai_ft_compare.py --baseline <gpt54mini-4096> --ft <ftoai003>` → `eval_comparison_v3_vs_gpt54mini.md`.
- **4.5** — Produce `eval_reference_point_v3.json` with a stable schema: `{model_id, eval_date, sample_size, seed, max_output_tokens, per_task_breakdown: [{task, n, mean_cosine, parse_pass, median_latency_ms}], aggregate: {mean_cosine, parse_pass, wl_t, total_cost_usd}}`. Schema is what the LoRA eval harness must emit.
- **Gate**: quality floor met (≥ 0.864 cross-base mean cosine); parse-pass ≥ 0.97; reference-point JSON produced with all 10 tasks resolved (no `__unattributed__`).

### Epic 5 — Pipeline-validation report

Produce `training_data/openai_ft/<YYYYMMDD>/pipeline_validation.md` that attributes the observed Δ (v3 vs v1) to the four levers:

| Lever | Expected Δ | Observed Δ | Direction | Notes |
|---|---|---|---|---|
| L1 — sys-prompt attribution | Resolves 47 `__unattributed__` → proper task slices; expect Δ ≥ +0.010 aggregate | TBD | TBD | Attribution is eval-side; if aggregate moves, the FT-OAI-001 Δ=−0.113 bucket was indeed attribution noise, not model failure |
| L2 — `--n-epochs 2` | Validation curve peaks before step 1000; no overfit inflection in last 200 steps | TBD | TBD | Confirmed by `best_val_loss_step` + `train_loss` vs `valid_loss` divergence |
| L3 — `--task-weights` | `retrieval.intent_translate` Δ ≥ 0 (was −0.079); `hidden.name_emergence` Δ ≥ −0.050 (was −0.114) | TBD | TBD | Small-slice tasks; high variance even post-upsampling |
| L4 — `response_text` parse-pass fix | parse-pass equals actual (was 0 due to bug); directional validation only | TBD | TBD | Cosmetic; no training impact |

If L1 + L2 + L3 all move in the expected direction and the aggregate floor is met → pipeline validated. If any lever regresses, document the surprise as an FT-OAI-004 input.

- **Gate**: `pipeline_validation.md` committed with per-lever direction + magnitude + commentary.

### Epic 6 — LoRA handoff memo

Produce `docs/development/ft-lora/oai_reference_point_handoff.md` covering:

- **Reference-point artefact schema** — exactly what LoRA eval output must match to be comparable.
- **Priority tasks for parity** — which of the 10 eval tasks carry the most production traffic (`ape.reflect` at 72% is the headliner; `retrieval.rerank_cross` second). LoRA should target these first.
- **Cost-parity bar** — for each task, the OpenAI FT reference cost per 1K queries; LoRA must beat this AND meet the quality bar (per-task cosine ≥ v3 reference).
- **Quality-per-dollar formula** — how to compute a scalar "value" for a LoRA candidate given its cosine, parse-pass, and inference cost (GPU/electricity amortisation).
- **Evaluation protocol** — how to run a LoRA candidate against the same 300-record seeded sample using `scripts/openai_ft_baseline_eval.py` with a `--local-endpoint` flag if the harness supports it, or a siblings script.

- **Gate**: memo committed to `docs/development/ft-lora/` (per project convention); reviewed by user before Epic 7.

### Epic 7 — Documentation (never cut)

- `docs/features/fine-tuning-pipeline.md` — update "Current State" with FT-OAI-003 numbers; add "LoRA reference point established" line.
- `CHANGELOG.md` — `[Unreleased]` → new entry documenting FT-OAI-003 (calibration + reference point).
- `AGENT_HANDOFF.md` — update Fine-Tuning Roadmap table; FT-OAI-003 row with outcome + "reference point for LoRA" note. Add pointer to LoRA track as next active work.
- `training_data/openai_ft/<YYYYMMDD>/run_notes.md` — full run log + pipeline validation + reference-point numbers + handoff pointer.
- Sprint-summary PR comment (per `feedback_sprint_summary_on_pr.md`).
- **Gate**: all five bullets checked; PR comment posted.

---

## 6. Testing Plan

Three-tier structure (mandatory per `memory/feedback_mandatory_testing_tiers.md`):

### Tier 1 — Unit / lint / static

- `python -m pytest neural/training/tests/ scripts/tests/` — all 34+ passing (inherits FT-OAI-002 suite; no new unit tests expected unless a bug is found during integration).
- `ruff check` + `mypy` on any modified scripts — match FT-OAI-002 bar.

### Tier 2 — Integration / dry-run

- Adapter dry-run on Epic 1 output: 20 records, verify `task_weights.applied == true`, `task_breakdown` shows post-weight counts.
- `openai_ft_check.py --validate <combined_train.jsonl>` pre-upload format check.
- Eval harness dry-run with `--n 5 --max-cost-usd 0.10` on stock `gpt-4.1-mini-2025-04-14` — verify `per_task_breakdown[*].task` is populated (no `__unattributed__`).
- Reference-point JSON schema validator (write inline: check all required keys present before marking E4 complete).

### Tier 3 — E2E / live OpenAI

- **T3.1** — FT launch (E3). Real tokens, real cost. Hard-capped at $60.
- **T3.2** — Cross-base reference-point eval (E4). Real tokens. Hard-capped at $15.
- **T3.3** — No production smoke (N/A — no deploy per scope).

---

## 7. Commit Strategy

Sequential, Epic-boundary commits on `reh3376_dev01`:

| # | Commit subject | After Epic |
|---|---|---|
| 1 | `feat(ft-oai-003): training-data prep — sys-prompt attribution + task reweighting` | E1 |
| 2 | `chore(ft-oai-003): launch config — n_epochs=2 explicit` | E2 (run_notes update only) |
| 3 | `chore(ft-oai-003): launch v3 fine-tuning job` | E3 (run_notes update only — large artefacts gitignored) |
| 4 | `feat(ft-oai-003): reference-point eval + comparator report` | E4 |
| 5 | `docs(ft-oai-003): pipeline-validation report` | E5 |
| 6 | `docs(ft-lora): OpenAI reference-point handoff memo` | E6 |
| 7 | `docs(ft-oai-003): feature doc + CHANGELOG + AGENT_HANDOFF` | E7 |

Per-epic commits (not batched) because each is independently reviewable and because Epic 3 is a billable live operation worth its own history entry.

Auto-PR fires on push; sprint summary comment posted per project rule.

---

## 8. Verification Checklist

- [ ] FT-OAI-002 PR #333 merged
- [ ] User spend auth confirmed in writing
- [ ] OpenAI project quota verified
- [ ] `sys_prompt_to_task.json` built from filtered.jsonl
- [ ] Task weights applied: `retrieval.intent_translate ×8`, `hidden.name_emergence ×2`
- [ ] `--n-epochs 2` enforced
- [ ] FT job succeeded; model ID captured; best_val_loss_step in [600, 1000]
- [ ] Cross-base reference-point eval completed at 4096 cap, seed=42, n=300
- [ ] Quality floor met: cross-base mean cosine ≥ 0.864
- [ ] Parse-pass ≥ 0.97
- [ ] `eval_reference_point_v3.json` produced with LoRA-compatible schema
- [ ] `pipeline_validation.md` — per-lever attribution committed
- [ ] `oai_reference_point_handoff.md` committed to `docs/development/ft-lora/`
- [ ] `docs/features/fine-tuning-pipeline.md` updated
- [ ] `CHANGELOG.md` updated
- [ ] `AGENT_HANDOFF.md` Fine-Tuning Roadmap row added
- [ ] Sprint summary posted to auto-PR comments
- [ ] No `.env` / `.env.example` modifications (calibration only, no deploy)

---

## 9. Documentation Update (Final Epic — never cut)

Covered by Epic 7 above. Never cut per `memory/feedback_mandatory_testing_tiers.md` and CLAUDE.md.

---

## 10. Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| 1 | FT-OAI-003 regresses below FT-OAI-001 on cross-base mean cosine | L-M | L | Calibration failure is publishable. Document in `pipeline_validation.md`; open FT-OAI-004 with root-cause hypothesis. No deploy is gated, so no production impact. |
| 2 | L2 (`--n-epochs 2`) under-trains — best val loss later than expected | L | L | `best_val_loss_step` in training_metrics.json tells us directly. If late, consider n_epochs=3 as FT-OAI-004 input. |
| 3 | L3 upweighting causes new regressions on other tasks (opportunity-cost effect) | M | L | Per-task breakdown catches this. Expected — document and treat as data point for LoRA weighting strategy. |
| 4 | Baseline 5.4-mini pricing or availability changes between FT-OAI-002 and FT-OAI-003 | L | L | Epic 0 readiness gate re-checks. If 5.4-mini deprecated, use `gpt-5-mini` as comparable tier. |
| 5 | Auto-epoch override silently ignored by OpenAI FT API | L | M | `n_epochs_actual` in `job_lifecycle.json` catches this. If ignored, abort and file OpenAI support ticket. |
| 6 | Queue-wait time blocks sprint (FT-OAI-001 attempt 3 waited 9h 8m) | M | L | Alert fires at 4h per FT-OAI-002 O2. Manual decision to wait or cancel. |
| 7 | `eval_reference_point_v3.json` schema requires rework after LoRA harness matures | M | M | Epic 6 handoff memo explicitly flags schema stability as the contract. If LoRA harness needs changes, bump to v4 schema and re-export; re-export is cheap (format transform, no re-eval). |
| 8 | User decides mid-sprint that even calibration cost ($80) isn't worth it | L | L | All epics prior to E3 are zero-cost. E3 is the budget gate — can abort there with all pipeline-validation still valid as a dry-run demonstration. |

---

## 11. Documents Accessed

- `/Users/reh3376/mdemg/docs/development/ft-oai/sprint_plan_ft_oai_002.md` (scope, epic structure, cost envelope section — sections 3, 5, 7)
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/run_notes.md` (FT-OAI-001 baseline + FT-OAI-002 summary + overfitting finding)
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/eval_comparison_vs_gpt54mini.md` (per-task breakdown, worst regressions)
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/unattributed_investigation.md` (FT-OAI-002 E4 proof: 0 cross-task sha256 collisions)
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/intent_translate_investigation.md` (FT-OAI-002 E5: n=4 regression root-cause)
- `/Users/reh3376/mdemg/docs/features/fine-tuning-pipeline.md` (current state, known limitations)
- `/Users/reh3376/mdemg/neural/training/openai_ft_adapter.py` (L117–145: `--task-weights`, `--sys-prompt-map` flags; `_infer_task()` fallback logic)
- `/Users/reh3376/mdemg/scripts/openai_ft_upload_and_launch.py` (`--n-epochs`, `--quota-buffer`, `--max-cost-usd` flags)
- `/Users/reh3376/mdemg/scripts/openai_ft_check.py` (`--on-complete`, `--job-id`, `--queue-timeout-minutes` flags; `parse_training_metrics_csv` + `best_val_loss_step` logic)
- `/Users/reh3376/mdemg/CHANGELOG.md` (FT-OAI-002 Added entry)
- `/Users/reh3376/mdemg/AGENT_HANDOFF.md` (Fine-Tuning Roadmap section — FT-OAI-002 row)
- `/Users/reh3376/mdemg/docs/development/ft-lora/` directory listing (06 files; 00_README_v2.md, 03_IMPLEMENTATION_PLAN_v2.md — surveyed for handoff memo integration)
- `/Users/reh3376/mdemg/CLAUDE.md` (project rules: sequential epics, mandatory testing tiers, Documents Accessed appendix, sprint summary on PR)
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` (feedback files for sprint plan format, sequential epics, primary branch, auto-PR, sprint summary on PR, sprint-plans-location)
- Web sources consulted during planning (OpenRouter + aipricing.guru pricing; see PR #333 sprint-summary comment for full list)

---

## 12. Rollback Procedures

**N/A.** This sprint makes no production changes, no `.env` modifications, no deployment. All work is in documentation, training-data preparation, and one billable FT launch whose output is a read-only reference artefact. Any epic can be rolled back via `git revert` on the corresponding commit; the FT model (if created) persists in the OpenAI account as a reference but is never wired into `.env`.

If the FT launch (Epic 3) produces a clearly unusable model (e.g., training fails to converge, cost explodes past cap), delete the model in the OpenAI console and mark the sprint incomplete-but-documented in `run_notes.md`. No rollback required because nothing has been deployed.

---

## Appendix A — Reference-point artefact schema (template)

`eval_reference_point_v3.json` — this is the contract that LoRA-side eval must match to be comparable:

```json
{
  "schema_version": "v3.0",
  "model_id": "ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai003:...",
  "model_type": "openai_ft",
  "base_model": "gpt-4.1-mini-2025-04-14",
  "eval_date": "2026-MM-DD",
  "sample": {
    "size": 300,
    "seed": 42,
    "source": "training_data/curated/sft_interactions/versioned/test.jsonl",
    "max_output_tokens": 4096
  },
  "aggregate": {
    "mean_cosine": 0.0,
    "parse_pass": 0.0,
    "wl_t": {"W": 0, "L": 0, "T": 0},
    "median_latency_ms": 0,
    "total_cost_usd": 0.0
  },
  "per_task_breakdown": [
    {
      "task": "ape.reflect",
      "n": 215,
      "mean_cosine": 0.0,
      "parse_pass": 0.0,
      "wl_t": {"W": 0, "L": 0, "T": 0},
      "median_latency_ms": 0
    }
  ],
  "comparison": {
    "baseline_model": "gpt-5.4-mini",
    "baseline_mean_cosine": 0.8980,
    "delta_mean_cosine": 0.0,
    "gap_closed_pct": 0.0
  }
}
```

LoRA equivalent replaces `model_type: "openai_ft"` with `"lora_adapter"`, `base_model` with the open-source base (e.g. `qwen3-8b`, `phi-4-mini`), `total_cost_usd` with GPU-hours-amortised cost, and `wl_t` with the head-to-head comparison against this very reference-point artefact. Everything else is identical.

---

## Appendix B — Per-lever hypothesis table (Epic 5 template)

Populated during Epic 5 from Epic 4 eval output. Drives the FT-OAI-004 decision surface.

| Lever | Expected direction | Observed direction | Magnitude | Surprised? | FT-OAI-004 input |
|---|---|---|---|---|---|
| L1 sys-prompt attribution | +aggregate, resolves `__unattributed__` | TBD | TBD | TBD | TBD |
| L2 `--n-epochs 2` | best_val_loss_step ≤ 1000, no late-epoch overfit | TBD | TBD | TBD | TBD |
| L3 `--task-weights` (intent_translate 8×) | Δ `retrieval.intent_translate` ≥ 0 | TBD | TBD | TBD | TBD |
| L3 `--task-weights` (name_emergence 2×) | Δ `hidden.name_emergence` ≥ −0.050 | TBD | TBD | TBD | TBD |
| L4 parse-pass fix | Reported parse-pass equals actual | TBD | TBD | TBD | TBD (cosmetic) |

If all five rows show "not surprised," pipeline validation is green. If any row surprises us, that's the signal to consider a rerun with an adjusted hypothesis (subject to re-authorisation of spend).
