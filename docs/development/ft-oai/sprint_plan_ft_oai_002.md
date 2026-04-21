# Sprint FT-OAI-002 — Fine-Tuning v2 Data Capture & Harness Hardening

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | FT-OAI-002 |
| **Title** | Fine-Tuning v2 Data Capture & Harness Hardening |
| **Date** | 2026-04-21 |
| **Format version** | v1.0 (12-section standard) |
| **Branch** | `reh3376_dev01` |
| **Predecessors** | FT-OAI-001 (complete, commit `a16c9ff`, merged as auto-PR #329 pending) |
| **Type** | Follow-up / hardening sprint |
| **Owner** | reh3376 |
| **Planning model** | Opus (per project rule: Opus for planning, Haiku only for mechanical tasks) |
| **Target FT model (if re-launch authorised)** | `gpt-4.1-mini-2025-04-14` (same base as FT-OAI-001) |
| **Reference FT model** | `ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai001:DX9KJuuq` |

---

## 2. Problem Statement

FT-OAI-001 delivered the first in-house fine-tune and closed the end-to-end OpenAI loop. Held-out eval showed **+0.032 mean cosine Δ**, **7.8:1 W/L ratio**, and **zero JSON format regression (parse-pass 0.973 → 0.973)**. Verdict was **MARGINAL** — net positive, but below the +0.05 bar and with five concrete capture gaps that blocked deeper analysis:

**Harness bugs (blocking forensic analysis):**
- **G1a** — Per-record `parse_ok` in `eval/*/results.jsonl` is **always `False`**, even though the aggregate `parse_pass_rate` in `summary.json` is correctly 0.9733. Bug in the per-record attribute write; aggregate path is independent.
- **G1b** — Per-record `finish_reason` is **always `null`**. Eval script reads the wrong attribute off the OpenAI response object — we cannot distinguish truncation (`length`) from natural stop (`stop`).
- **G1c** — Per-record input/output **token counts are not persisted**. We know total cost from the manifest, not per-record distribution.

**Methodology gaps (blocking clean headline):**
- **G2** — Baseline was eval'd at `--max-output-tokens 1024`; FT at `4096` (bumped after discovering ~60% of FT responses exceeded 1024). The headline +0.032 Δ is measured across this asymmetric cap — apples-to-oranges at the tails.
- **G3** — The 5 worst regressions AND 5 best gains are **all on task=`__unattributed__`** with opposite hallucination patterns (`{"type":"none","summary":""}`). Indicates the task-type attribution heuristic is ambiguous, not that the model is worse — but we need to confirm or fix before we trust per-task verdicts.

**Training signal gaps:**
- **T1** — Per-epoch val loss lives only in the OpenAI-provided CSV; not persisted alongside manifest.
- **T2** — OpenAI auto-selected `n_epochs=3`; best val loss was at step 1200/1500 (mild overfitting after that). No programmatic override to force `n_epochs=2`.
- **T3** — `best_val_loss` step not surfaced in any operator-facing artifact.

**Operational gaps:**
- **O1** — Queue-wait time (9h 8m for attempt 3) is not captured as telemetry — only exists as manual notes in `run_notes.md`.
- **O2** — No auto-resubmit on queue-stuck (attempt 2 manually cancelled after 4h 15m).
- **O3** — No project-quota pre-check (attempt 1 failed `exceeded_quota` after accepting the job; $155.19 > $94.67 remaining).
- **O4** — Cost estimator does not model OpenAI's auto-epoch multiplier (actual ~$155 vs our single-epoch estimate of $93.13 = 1.66× multiplier).

**Open regression:**
- **R1** — `retrieval.intent_translate` (n=4) showed Δ=**−0.079** (0W/3L/1T). Small n but consistent direction — needs deep-dive before we trust the FT model on that task.

**Post-sprint cross-base finding (added 2026-04-21):** After FT-OAI-001 was delivered, a cross-base bench was run comparing the FT model against the **actual production base** (`gpt-5.4-mini`), not just against its own training base. Result: stock `gpt-5.4-mini` scored mean cosine **0.898** vs FT's **0.864** (quality Δ = −0.034, W/L/T = 14/61/225). **Strategic framing:** stock-4.1-mini 0.832 → FT 0.864 → stock-5.4-mini 0.898 — FT-OAI-001 closed ~48% of the stock-4.1-mini → stock-5.4-mini gap. The north star is `FT(gpt-4.1-mini) ≈ prod(gpt-5.4-mini)` quality at the cheaper base's inference cost. That target is **FT-OAI-003**. **FT-OAI-002 remains the prerequisite** — the harness bugs below (G1a/G1b/G1c, A1–A7, G2, G3, R1) are exactly what we need to trust per-task verdicts in FT-OAI-003 and to target the remaining ~52% of the gap. The G3 `__unattributed__` deep-dive is now extra-critical: that task is the single largest per-task regression vs prod (−0.113) and recovering it is the biggest ROI lever for FT-OAI-003.

**Intended outcome:** A hardened eval harness that records the missing fields, a cap-symmetric baseline re-run so the headline is clean, a clear verdict on whether `__unattributed__` noise is real or an artifact, operator-facing training signal visibility, and operational telemetry for the next FT launch. This sprint explicitly does **not** require a second FT launch — that decision is gated behind the v2 data-capture epic landing and user authorisation.

---

## 3. Scope & Constraints

### In scope

- Eval harness fixes (G1a, G1b, G1c) — per-record `parse_ok`, `finish_reason`, token counts
- Per-record metric field additions (A1–A7) — latency_ms, retry_count, truncation_flag, embedding_model_version, hallucination_indicator
- Baseline cap-symmetric re-eval at `--max-output-tokens 4096` (G2)
- Training signal persistence (T1, T2, T3) — per-epoch val loss JSON sidecar, `--n-epochs` override flag, best_val_loss step in manifest
- `__unattributed__` task attribution investigation (G3) — read actual records, confirm/fix heuristic
- `retrieval.intent_translate` regression deep-dive (R1) — per-record analysis of the 4 failing cases
- Operational telemetry (O1, O2, O3, O4) — queue-wait capture, auto-resubmit helper, quota pre-check, auto-epoch cost envelope
- Per-task sample weights (T4) — downweight noisy tasks if G3 confirms noise
- Re-run baseline comparator at fixed cap + update `eval_comparison.md` with clean headline
- Documentation update (final epic — never cut)

### Out of scope

- **Actual v2 FT launch** — gated behind this sprint's completion + explicit user authorisation in a follow-up sprint (FT-OAI-003 if pursued)
- **DPO paradigm** — defer to FT-OAI-004 or later; architecture must not preclude
- **RAFT paradigm experiment** (T5) — defer; keep `--raft-ratio 0.0` per FT-OAI-001 precedent
- **Multi-provider adapter** (Anthropic FT, Fireworks, etc.) — sibling pattern established, not implemented
- **Cross-task curriculum training** — out of scope
- Changes to `dataset_versioner.py` temporal-split logic (it is correct; don't touch)
- Changes to `quality_filter.py` gate logic (reviewed; no defects found in FT-OAI-001)

### Hard constraints

- **No destructive operations without explicit user confirmation** (per project Decision Protocol)
- **No direct commits to `main`** — `reh3376_dev01` only; auto-PR handles merge
- **Cost cap on any live OpenAI call** — `--max-cost-usd` enforced pre-network (inherited from FT-OAI-001)
- **Baseline re-run cost budget**: ≤ $5.00 (FT-OAI-001 baseline at 4096 cap should cost ~$4.05)
- **Sequential epic execution** — per `memory/feedback_sequential_epics.md`; no parallelisation
- **All sprint plans follow 12-section format** — per `memory/feedback_sprint_plan_format.md`
- **3 testing tiers per epic** — per `memory/feedback_mandatory_testing_tiers.md`
- **Documents Accessed appendix** — per project memory rule

---

## 4. Dependencies

### Upstream (must be satisfied before E1 starts)

- FT-OAI-001 merged to `main` (PR #329 auto-created on push of `a16c9ff`) — **Epic 0 gate**
- `neural/training/openai_ft_adapter.py` exists — ✅ (committed in `a16c9ff`)
- `scripts/openai_ft_*.py` exist — ✅ (committed in `a16c9ff`)
- FT-OAI-001 artifacts preserved at `training_data/openai_ft/20260420/` (local, not committed) — ✅
- Python deps: `openai>=1.50`, `tiktoken>=0.7`, `jsonschema>=4.20.0`, `cuid2>=2.0.0`

### Downstream (this sprint unblocks)

- FT-OAI-003 (v2 FT launch with clean data capture) — user-authorised follow-up, NOT in this sprint
- FT-OAI-004+ (DPO, multi-provider, RAFT experiments) — architecture must remain extensible

### External (read-only consultation)

- OpenAI API `fine_tuning/jobs/<id>` schema (for queue-wait field names)
- OpenAI API `billing/subscription` or equivalent (for quota pre-check — may require auth scope verification)

---

## 5. Implementation Plan

**Execute sequentially. Do NOT parallelise epics.** Documentation (E9) is the final epic and is **never cut**.

### Epic 0 — Readiness gate

Block until FT-OAI-001 is merged to `main`. Verify branch state, confirm no rebase/conflict outstanding.

**Steps:**
- `git fetch origin` and confirm `reh3376_dev01` is at `a16c9ff` or fast-forward of it
- Confirm PR #329 (auto-created on push of `a16c9ff`) is merged — check `gh pr list --state merged --search "FT-OAI-001"`
- Pull latest `main` into `reh3376_dev01`: `git checkout main && git pull && git checkout reh3376_dev01 && git merge main --ff-only` (abort and ask user if not fast-forward)
- Run `golangci-lint run ./...` and `cd neural && python3 -m pytest training/tests/ -v` — both MUST be green on starting state

**Gate:** PR #329 merged, branch clean, lint + existing tests green, `neural/training/openai_ft_adapter.py` present on disk.

### Epic 1 — Eval harness bug fixes (G1a, G1b, G1c)

Fix the three per-record bugs in `scripts/openai_ft_baseline_eval.py`.

**Steps:**
- **G1a — `parse_ok` always False**: locate the per-record write path, verify the attribute is being set from the correct variable (not a stale one from init). Add a unit test that asserts `parse_ok=True` for a valid JSON response and `False` for a malformed one.
- **G1b — `finish_reason` always null**: inspect OpenAI chat completion response object; the correct path is `resp.choices[0].finish_reason` (not `resp.finish_reason`). Fix the attribute read and add a unit test using a captured fixture response.
- **G1c — Per-record token counts**: capture `resp.usage.prompt_tokens` and `resp.usage.completion_tokens` into each `results.jsonl` row. Add unit test.
- **Backfill capability**: add a helper `scripts/openai_ft_results_backfill.py` that takes an existing `results.jsonl` and re-computes `parse_ok` from the response text (pure function, no network). Document that `finish_reason` and token counts **cannot** be retroactively recovered — they require a fresh eval run.

**Gate:**
- Unit tests pass for all three fields
- Running harness on a 3-record smoke sample produces `results.jsonl` with 3 rows each having non-null `parse_ok`, `finish_reason`, `prompt_tokens`, `completion_tokens`
- Backfill helper runs on FT-OAI-001 `results.jsonl` and updates `parse_ok` to match aggregate (0.973)

### Epic 2 — Per-record metric field additions (A1–A7)

Extend `results.jsonl` schema with operator-useful fields beyond E1's bug fixes.

**Fields added:**
- `latency_ms` (A1) — wall-clock from request start to response received
- `retry_count` (A2) — how many retries the OpenAI client performed (exposed via the SDK's retry hooks)
- `truncation_flag` (A3) — `finish_reason == "length"` boolean, derived convenience field
- `embedding_model_version` (A4) — the exact model string returned by the embedding API (e.g. `text-embedding-3-small`)
- `request_id` (A5) — OpenAI's request-id header, for incident correlation
- `hallucination_indicator` (A6) — when `ground_truth == {"type":"none","summary":""}` and `response != {"type":"none","summary":""}`, mark True. Enables quick filtering of the `__unattributed__` failure pattern.
- `input_chars` / `output_chars` (A7) — complement to token counts; useful sanity check when tokenizer mismatches are suspected

**Also update `summary.json`:**
- Mean latency, p50, p95, p99
- Truncation rate
- Mean retries (a spike here would flag upstream instability)
- Hallucination rate on `none`-typed ground truth (directly addresses G3 symptom)

**Gate:**
- Unit tests for each new field
- 3-record smoke run shows all A1–A7 fields populated and non-null
- `summary.json` carries the 5 new aggregate fields

### Epic 3 — Baseline cap-symmetric re-eval (G2)

Re-run the baseline at `--max-output-tokens 4096` against the **same** seeded sample (seed=42, n=300) used in FT-OAI-001. Update comparator.

**Steps:**
- Run baseline at 4096 cap, output to `training_data/openai_ft/20260420/eval/baseline_4096/` (preserving the old 1024-cap run for reference)
- Cost cap: `--max-cost-usd 5.00` (expected ~$4.05)
- Run comparator against the existing FT `eval/ft/` using the new baseline
- Regenerate `eval_comparison.md` as `eval_comparison_v2.md` with the clean headline
- Document both runs in `run_notes.md` under a new "## Cap-symmetric re-eval (FT-OAI-002 E3)" section
- Expected outcome: the headline Δ may shrink slightly (if some 1024-cap baseline records were truncated, giving them lower cosine; fixing that raises baseline and narrows Δ). Record the clean number regardless of direction.

**Gate:**
- Cap-symmetric comparator emits `eval_comparison_v2.md`
- Headline Δ, W/L/T, parse-pass rate all recomputed with matched caps
- `run_notes.md` updated with both runs labelled for reproducibility

### Epic 4 — `__unattributed__` attribution investigation (G3)

Read the actual records behind the 5 worst regressions + 5 best gains (all task=`__unattributed__`). Determine whether:
- (a) task attribution heuristic is broken (should have been `consulting.classify` or similar)
- (b) records are genuinely noisy (mixed ground truth)
- (c) records represent a real task type we haven't named yet (new UAITS spec needed)

**Steps:**
- Load the 10 records from `training_data/openai_ft/20260420/eval/ft/results.jsonl` where `task=="__unattributed__"` AND `|delta| > 0.5` vs baseline (from `results.jsonl` cross-reference)
- Read the original `llm_interactions` TSDB rows for those trace_ids (via `mdemg data inspect` or direct SQL)
- For each, check: system prompt, ULTS spec if any, `guidance_id`, `source_path`
- Categorise into (a)/(b)/(c) above
- If (a): propose a heuristic fix (do NOT apply mid-sprint — land in a follow-up data-prep change if warranted)
- If (b): propose filter rule for `quality_filter.py` (e.g. skip records where system prompt is empty AND ground-truth is `{"type":"none",...}`)
- If (c): propose new UAITS spec and task name — out of scope for this sprint, file as its own follow-up task

**Gate:**
- Written finding in `training_data/openai_ft/20260420/unattributed_investigation.md` with per-record classification
- Recommendation (apply in this sprint / defer) explicit for each category
- No silent changes to training-data curation logic

### Epic 5 — `retrieval.intent_translate` regression deep-dive (R1)

The 4 records with FT Δ=−0.079 are not random — direction is consistent (0W/3L/1T). Investigate.

**Steps:**
- Extract the 4 `retrieval.intent_translate` rows from baseline and FT `results.jsonl`
- Line up: system prompt, user prompt, ground truth, baseline response, FT response
- Identify the failure mode (wrong intent class? wrong temporal marker? over-specific?)
- Cross-reference training data: how many `retrieval.intent_translate` examples are in `training_data/curated/sft_interactions/versioned/train.jsonl`? (Low n in training → predictable weak fine-tune performance.)
- Written finding in `training_data/openai_ft/20260420/intent_translate_investigation.md`

**Gate:**
- Root cause classified: (a) under-represented in training, (b) ambiguous ground truth, (c) FT genuinely degrades on this task
- If (a): recommend upsampling in v2 training data (captured as input to Epic 6)
- No silent fixes

### Epic 6 — Training signal persistence + per-task weights (T1, T2, T3, T4)

**T1 — Per-epoch val loss sidecar**: after FT job completes, parse OpenAI's `result_file` CSV into `training_metrics.json` with per-step `train_loss`, `val_loss`, `val_accuracy`, and compute `best_val_loss_step` programmatically. Add to `scripts/openai_ft_check.py --on-complete` path so it runs automatically when the poller detects `status=succeeded`.

**T2 — `--n-epochs` override flag**: add to `scripts/openai_ft_upload_and_launch.py`. Default remains `auto`; accepts integer or `auto`. Rationale for any override goes in a required `--n-epochs-rationale` string that gets written into `run_notes.md`.

**T3 — `best_val_loss_step` in manifest**: surface in `run_notes.md` the `step_best_val / step_total` ratio (FT-OAI-001 had 1200/1500 = 0.80, an overfit indicator). Flag when ratio < 0.90 as "consider `--n-epochs` reduction next run."

**T4 — Per-task sample weights**: if Epic 4 finding recommends downweighting `__unattributed__` OR if Epic 5 finding recommends upweighting `retrieval.intent_translate`, implement via a new `--task-weights` flag on `neural/training/openai_ft_adapter.py` that duplicates or drops records deterministically by task. Default = uniform (1.0 per task). Write resolved weights into manifest.

**Gate:**
- Poller's `--on-complete` path produces `training_metrics.json` on a fresh (or replayed) FT completion
- `--n-epochs` flag accepted end-to-end, rationale persisted
- `best_val_loss_step` surfaced in `run_notes.md`
- `--task-weights` flag accepted; unit test confirms deterministic output for a fixed weight map

### Epic 7 — Operational telemetry (O1, O2, O3, O4)

**O1 — Queue-wait telemetry**: extend `scripts/openai_ft_check.py` to record `created_at`, `queued_at`, `running_at`, `finished_at` into a `job_lifecycle.json` file. Compute `queue_seconds = running_at - queued_at`, `train_seconds = finished_at - running_at`. Write to `run_notes.md` automatically.

**O2 — Auto-resubmit on queue-stuck**: add `--queue-timeout-minutes N` flag to the poller. If `queued_at` > N minutes ago AND no `running_at` yet, the poller emits an alert (writes to `alert.log` and prints to stdout) but does **NOT** auto-cancel — human confirmation still required. Rationale: automating the cancel could cost money if the job is about to start. Default: N=180 (3 hours, conservative).

**O3 — Project-quota pre-check**: in `scripts/openai_ft_upload_and_launch.py`, before calling `files.create()`, query the OpenAI billing API (if available for this API key tier) and compute `remaining_quota_usd`. If `remaining_quota_usd < cost_estimate * 1.66` (auto-epoch buffer), abort with a clear error. If the billing API is not available (some API key tiers), print a warning and continue — do not block.

**O4 — Auto-epoch cost envelope**: in `neural/training/openai_ft_adapter.py`, when the user does not override `--n-epochs`, surface the envelope as `cost_estimate_low` (1 epoch) and `cost_estimate_high` (3 epochs, OpenAI's observed auto ceiling) in the manifest. Keep `cost_estimate_usd` as the midpoint for cap-gating.

**Gate:**
- `job_lifecycle.json` produced for a replay of FT-OAI-001 attempt 3
- `--queue-timeout-minutes` flag emits alert when threshold crossed (unit test with mock poller)
- Quota pre-check either succeeds or prints a clear warning; no silent pass
- Manifest `cost_estimate_*` has low/mid/high fields

### Epic 8 — Integration dry-run (Tier 2 harness exercise)

Run the full updated harness end-to-end **without launching a new FT job** — against the existing FT-OAI-001 model — to verify the whole pipeline still works after E1–E7 changes.

**Steps:**
- Re-run `scripts/openai_ft_baseline_eval.py` on a 10-record smoke sample against both base and FT models (cost ~$0.15 total)
- Confirm all new fields present, all bugs fixed, `summary.json` has new aggregates
- Run `scripts/openai_ft_compare.py` on the 10-record pair — confirm it still renders
- Run the backfill helper on the FT-OAI-001 300-record `results.jsonl` for both baseline and FT — confirm `parse_ok` aggregate matches the previously-correct `parse_pass_rate` (0.973)

**Gate:**
- 10-record live smoke run green
- Backfilled 300-record `parse_ok` rate matches aggregate within 0.001
- No new warnings or regressions from the updated scripts

### Epic 9 — Documentation (FINAL, never cut)

Update user-facing docs to reflect E1–E8 changes.

**Changes:**
- `docs/features/fine-tuning-pipeline.md` — move G1/G2/G3/R1/O1-O4 from "Known Limitations" / "Future Improvements" to a new "## Resolved in FT-OAI-002" section; update the workflow's eval step with the new fields and flags
- `neural/training/README.md` — update `openai_ft_adapter.py` section with `--task-weights` and `--n-epochs` references
- `CHANGELOG.md` — new FT-OAI-002 entry under `[Unreleased]`
- `AGENT_HANDOFF.md` — add FT-OAI-002 row to the Fine-Tuning Pipeline phase table
- `training_data/openai_ft/20260420/run_notes.md` — final run summary with cap-symmetric headline, investigation findings, recommendations for FT-OAI-003 (if pursued)

**Gate:**
- All four doc files updated
- `grep "FT-OAI-002"` returns hits in all four
- PR description on push references FT-OAI-002 + links to investigation findings

---

## 6. Testing Plan (three tiers — mandatory)

### Tier 1 — Unit / lint / static

**Scope:** Every code change has a unit test. No lint warnings.

- `cd neural && python3 -m pytest training/tests/ -v` → all green including new tests for E1 (3 bug fixes), E2 (7 new fields), E6 (weights flag), E7 (quota pre-check, queue-timeout alert)
- `ruff check neural/training/ scripts/openai_ft_*.py` → clean
- `mypy neural/training/openai_ft_adapter.py` → clean
- `/Users/reh3376/go/bin/golangci-lint run ./...` → unchanged from baseline (this sprint touches no Go code)

**Per-epic unit test requirements:**

| Epic | New tests |
|---|---|
| E1 | `test_parse_ok_per_record`, `test_finish_reason_populated`, `test_token_counts_populated` |
| E2 | `test_latency_ms_recorded`, `test_truncation_flag_derived`, `test_hallucination_indicator`, `test_summary_aggregates` |
| E3 | (no new unit tests — Tier 3 run) |
| E4, E5 | Investigation epics — no unit tests |
| E6 | `test_n_epochs_flag_accepted`, `test_task_weights_deterministic`, `test_best_val_step_computed` |
| E7 | `test_job_lifecycle_computed`, `test_queue_timeout_alert`, `test_quota_precheck_abort`, `test_cost_envelope_fields` |

### Tier 2 — Integration / dry-run

**Scope:** End-to-end script runs using mocked OpenAI or replayed prior results.

- Replay FT-OAI-001 `results.jsonl` through the E1 backfill helper; confirm aggregate `parse_ok` rate matches previously-correct `parse_pass_rate` (0.973 ± 0.001)
- Dry-run the upload/launch script against a mock OpenAI server; confirm `--max-cost-usd` + quota pre-check gates behave correctly
- Dry-run the poller's `--on-complete` path against a captured `job_final.json` from FT-OAI-001 attempt 3; confirm `job_lifecycle.json` and `training_metrics.json` sidecars produced

### Tier 3 — E2E / live OpenAI

**Scope:** Real API calls, small samples only, cost-capped.

- E3: Live baseline re-eval at 4096 cap, n=300, cost cap $5.00 — THE expensive item in this sprint (~$4.05 actual)
- E8: 10-record smoke eval against both base and FT-OAI-001, cost cap $0.50 (actual ~$0.15)
- NO new FT job launched (out of scope; gated to FT-OAI-003)

**Total Tier 3 budget: $5.50 cap.**

---

## 7. Commit Strategy

One commit per epic. Conventional Commits. Auto-PR on push to `reh3376_dev01`.

| Epic | Commit message |
|---|---|
| E0 | (no commit — readiness gate only) |
| E1 | `fix(ft-oai): per-record parse_ok, finish_reason, token counts in eval results` |
| E2 | `feat(ft-oai): add latency, retries, truncation, hallucination fields to eval results` |
| E3 | `test(ft-oai): cap-symmetric baseline re-eval at 4096 tokens` |
| E4 | `docs(ft-oai): __unattributed__ task attribution investigation` |
| E5 | `docs(ft-oai): retrieval.intent_translate regression deep-dive` |
| E6 | `feat(ft-oai): persist per-epoch val loss, add --n-epochs and --task-weights` |
| E7 | `feat(ft-oai): queue-wait telemetry, quota pre-check, auto-epoch cost envelope` |
| E8 | (no commit — integration dry-run only, or small doc update if drift found) |
| E9 | `docs(ft-oai): FT-OAI-002 completion — docs, CHANGELOG, AGENT_HANDOFF` |

Each commit includes `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>` trailer per project convention.

Sprint completion: add sprint summary to PR comments per `memory/feedback_sprint_summary_on_pr.md`.

---

## 8. Verification Checklist

### Before opening/updating the PR

- [ ] Tier 1 (unit + lint) all green
- [ ] Tier 2 (replay + mock) all green
- [ ] Tier 3 (live at 4096 cap) complete, cost within cap
- [ ] `training_data/openai_ft/20260420/eval_comparison_v2.md` present with cap-symmetric headline
- [ ] `training_data/openai_ft/20260420/unattributed_investigation.md` present with per-record findings
- [ ] `training_data/openai_ft/20260420/intent_translate_investigation.md` present with root cause
- [ ] `job_lifecycle.json` produced and persisted for FT-OAI-001 replay
- [ ] `training_metrics.json` produced from OpenAI CSV
- [ ] `parse_ok` aggregate in backfilled E1 results == 0.973 ± 0.001
- [ ] All new script flags (`--n-epochs`, `--task-weights`, `--queue-timeout-minutes`) documented in `--help`
- [ ] CHANGELOG, README, feature doc, AGENT_HANDOFF updated
- [ ] Sprint summary comment added to PR
- [ ] No destructive ops performed without explicit user confirmation
- [ ] No direct commits to `main`

### Rollback triggers (if any verification item fails)

- E1 unit tests failing → revert E1 commit, re-diagnose
- E3 cap-symmetric delta shows regression (FT Δ < 0) → pause sprint, escalate to user
- E7 quota pre-check blocks a legitimate call → add `--skip-quota-check` escape hatch, land as separate commit

---

## 9. Documentation Update (FINAL EPIC — never cut)

**This is Epic 9.** Repeated here for emphasis per `memory/feedback_mandatory_testing_tiers.md` ("ALL new code/pipelines MUST be verified end-to-end; documentation updates are part of that contract").

Files updated:
- `docs/features/fine-tuning-pipeline.md` — resolved-in-v2 section
- `neural/training/README.md` — new flags
- `CHANGELOG.md` — `[Unreleased]` entry
- `AGENT_HANDOFF.md` — phase table row
- `training_data/openai_ft/20260420/run_notes.md` — final sprint summary (local artifact, not committed; referenced by commit message)

**Rule invoked:** Never drop the doc epic to save time. Undocumented changes are worse than no changes.

---

## 10. Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| R1 | `__unattributed__` investigation (E4) reveals the attribution heuristic is fundamentally broken, blowing up into a curation-pipeline rewrite | Medium | High | **Explicit scope containment:** E4 produces a written finding only. Any fix lands in a follow-up sprint. Do not let E4 metastasise. |
| R2 | `retrieval.intent_translate` regression (E5) root cause is "FT genuinely worse" with no clear fix path | Low | Medium | Finding is still valuable — informs the training data distribution for FT-OAI-003. Record as a known-weak-task, not a blocker. |
| R3 | OpenAI billing API not available for current API key tier (O3) | Medium | Low | Quota pre-check degrades to a warning, not a blocker. Documented in the flag's `--help`. |
| R4 | Cap-symmetric baseline re-eval (E3) costs more than $5 due to pricing change | Low | Low | `--max-cost-usd 5.00` hard-gates pre-network. If it aborts, inspect pricing and raise cap with user approval. |
| R5 | OpenAI SDK version drift breaks the retry-count hook (A2) | Low | Medium | Pin `openai>=1.50,<2.0` in requirements; add a smoke test against the pinned version |
| R6 | Backfill helper (E1) over-writes `results.jsonl` in place and corrupts it | Low | High | Backfill writes to a new file `results_backfilled.jsonl`; never mutates original. Unit test asserts this. |
| R7 | Auto-resubmit on queue-stuck (O2) accidentally cancels a job about to start | N/A | High | **Explicit design: no automatic cancel.** O2 only alerts; human decides. |
| R8 | Task weights flag (T4) introduces training-set bias that hurts tail tasks | Medium | Medium | Default weights = uniform; any non-uniform weighting requires rationale in manifest + Epic 4/5 finding as justification |
| R9 | Doc updates (E9) get compressed or skipped under time pressure | Low | High | Hard rule: E9 is the final epic and never cut. Enforced by project memory + this plan. |

---

## 11. Documents Accessed

Files read, consulted, or grep'd during planning:

| Path | Purpose |
|---|---|
| `/Users/reh3376/mdemg/training_data/openai_ft/20260420/run_notes.md` | FT-OAI-001 run log — lines 1–95; attempt 1/2/3 metadata, training metrics, eval results, per-task improvements/regressions |
| `/Users/reh3376/mdemg/training_data/openai_ft/20260420/eval_comparison.md` | Baseline vs FT comparison report — headline numbers, per-task table, 5 worst/best, verdict |
| `/Users/reh3376/mdemg/neural/training/openai_ft_adapter.py` (header) | Adapter responsibilities and design constraints — confirmed pure post-processor role |
| `/Users/reh3376/mdemg/neural/training/README.md` | Current training pipeline documentation |
| `/Users/reh3376/mdemg/CHANGELOG.md` (top 50 lines) | `[Unreleased]` section current state; FT-OAI-001 entry just landed |
| `/Users/reh3376/mdemg/AGENT_HANDOFF.md:690-710` | Fine-Tuning Pipeline status block; FT-OAI-001 row now present |
| `/Users/reh3376/mdemg/docs/features/fine-tuning-pipeline.md` | Newly-authored feature doc; Known Limitations + Future Improvements drive E1–E7 |
| `/Users/reh3376/mdemg/docs/features/_TEMPLATE.md` | Feature doc template — structural reference |
| `/Users/reh3376/mdemg/.gitignore` (diff) | `training_data/*` excluded with README/.gitkeep allow-list |
| `/Users/reh3376/mdemg/CLAUDE.md` | Project conventions, CLI commands, git workflow, orchestration protocol, sprint plan format rule |
| `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` | Mandatory workflow rules, testing tiers rule, sequential epics rule, sprint plan format rule, Documents Accessed rule |
| `/Users/reh3376/.claude/plans/breezy-dancing-lerdorf.md` | Prior review of FT-OAI-001 plan — precedent for surgical edit approach |
| `git log -5` | Commit style (Conventional Commits) |
| `git status` | Pre-commit state audit |

---

## 12. Rollback Procedures

Not applicable at the sprint level — this sprint performs **no destructive operations**:
- No schema migrations
- No TSDB data deletion
- No FT job launch (new one is gated to FT-OAI-003)
- No changes to `main` branch
- No `--force` pushes
- Local FT-OAI-001 artifacts at `training_data/openai_ft/20260420/` remain immutable; new artifacts go to subdirectories (`eval/baseline_4096/`, etc.)

Per-epic rollback:
- Any commit can be reverted with `git revert <sha>` and pushed — auto-PR will pick it up
- The FT-OAI-001 model (`ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai001:DX9KJuuq`) is not deleted and not modified by this sprint
- Investigation findings (E4, E5) are markdown documents — no rollback needed; wrong findings get corrected in a follow-up commit

---

## Appendix A — Out-of-sprint follow-ups captured during planning

Things to NOT do in this sprint but file as future tasks:

1. **FT-OAI-003** — if the E3 cap-symmetric headline + E6 signal capture looks promising, user may authorise a v2 FT launch with downweighted `__unattributed__` and `--n-epochs 2`. Do NOT start without explicit authorisation.
2. **FT-OAI-004** — DPO paradigm exploration. Adapter architecture must remain sibling-compatible.
3. **Multi-provider adapter** — Anthropic, Fireworks, etc. — sibling module to `openai_ft_adapter.py`.
4. **Jiminy investigation (carried from FT-OAI-001 planning)** — why did Jiminy not fire a plan-mode directive at the start of FT-OAI-001? Run `python3 scripts/jiminy_effectiveness_report.py --space-id mdemg-dev --days 7` and diagnose. Separate from this sprint.

## Appendix B — Reference commands

```bash
# Replay FT-OAI-001 baseline at 4096 cap (E3)
python scripts/openai_ft_baseline_eval.py \
  --model gpt-4.1-mini-2025-04-14 \
  --test-file training_data/curated/sft_interactions/versioned/test.jsonl \
  --output-dir training_data/openai_ft/20260420/eval/baseline_4096 \
  --sample-size 300 --seed 42 --max-output-tokens 4096 --max-cost-usd 5.00

# Regenerate comparator (E3)
python scripts/openai_ft_compare.py \
  --baseline-dir training_data/openai_ft/20260420/eval/baseline_4096 \
  --ft-dir training_data/openai_ft/20260420/eval/ft \
  --output training_data/openai_ft/20260420/eval_comparison_v2.md

# Backfill parse_ok on existing results (E1)
python scripts/openai_ft_results_backfill.py \
  --input training_data/openai_ft/20260420/eval/ft/results.jsonl \
  --output training_data/openai_ft/20260420/eval/ft/results_backfilled.jsonl
```
