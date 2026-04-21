# Sprint FT-OAI-003 — Close the Gap to Production Base

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | FT-OAI-003 |
| **Title** | Close the Gap to Production Base (`FT(cheap-base) ≈ prod(prod-base)` north star) |
| **Date** | 2026-04-21 (drafted — execution gated behind FT-OAI-002 + economic analysis) |
| **Format version** | v1.0 (12-section standard) |
| **Branch** | `reh3376_dev01` |
| **Predecessors** | FT-OAI-001 (complete, PR #332), FT-OAI-002 (planned, task #142) |
| **Type** | v2 fine-tuning launch + economic analysis |
| **Owner** | reh3376 |
| **Planning model** | Opus |
| **Target FT training base** | TBD during E1 — `gpt-4.1-mini-2025-04-14` (default) OR `gpt-4o-mini` (if per-token cost savings justify the bench overhead) |
| **Production base (quality target)** | `gpt-5.4-mini` (current `.env` default) |
| **Reference FT model** | `ft:gpt-4.1-mini-2025-04-14:whiskey-house:mdemg-ftoai001:DX9KJuuq` (FT-OAI-001) |
| **Task ID** | #143 |

---

## 2. Problem Statement

FT-OAI-001 delivered the first in-house fine-tune and closed the end-to-end OpenAI training→eval loop. The in-frame headline (+0.032 mean cosine vs training base `gpt-4.1-mini`, 7.8:1 W/L, parse-pass preserved) looked promising. The cross-base bench (vs production base `gpt-5.4-mini`, same 300-record seeded sample, same 4096 cap) told a different story: **quality Δ = −0.034, W/L/T = 14/61/225, zero tasks where FT beats prod.**

Viewed as a pure quality evaluation, FT-OAI-001 is a regression. Viewed strategically, it is a **step toward a cost-saving win**:

| Anchor | Mean cosine | Distance from prod |
|---|---|---|
| Stock `gpt-4.1-mini` (training base) | 0.8322 | −0.0658 |
| **FT-OAI-001 (2026-04-21)** | **0.8641** | **−0.0339** |
| Stock `gpt-5.4-mini` (prod target) | 0.8980 | 0.0000 |

**FT-OAI-001 closed ~48% of the stock-4.1-mini → stock-5.4-mini quality gap** (0.0319 / 0.0658).

The north star: **`FT(cheap-base) ≈ stock(prod-base)` quality at the cheap base's inference cost.** `gpt-4.1-mini` is materially cheaper per token than `gpt-5.4-mini`. If FT-OAI-003 closes the remaining ~52% of the gap (a further ~+0.034 cosine), we get prod-level responses at the cheaper base's cost envelope — a significant economic win at scale, especially on volume tasks like `ape.reflect` (72% of production traffic).

**What FT-OAI-003 must deliver:**
1. **Quantified deploy criterion** — the OpenAI-bill per-token cost ratio of `gpt-4.1-mini` vs `gpt-5.4-mini` (and `gpt-4o-mini` if in play). Without this number, there is no principled quality-vs-cost trade-off.
2. **A second FT launch** — with the four identified levers applied (training-data noise fix, `n_epochs=2`, task reweighting, optionally a different base).
3. **A cap-symmetric cross-base re-eval** — FT-OAI-003 vs stock prod base, same seeded sample, same 4096 cap.
4. **A deploy decision** — per-task if the numbers support it, universal if they don't; `.env` / `.env.example` updated accordingly.

**Per-task targets (from FT-OAI-001 cross-base bench, worst → best):**
- `retrieval.intent_translate` (n=4): FT Δ=−0.149 vs prod. 0W/4L. Largest per-task delta.
- `hidden.name_emergence` (n=13): FT Δ=−0.114. 1W/10L/2T. Largest per-task delta with sample size that matters.
- `__unattributed__` (n=47): FT Δ=−0.113. 0W/13L/34T. Largest volume impact at −0.113. Probably training-data noise (FT-OAI-002 E4 investigation will clarify).
- `ape.reflect` (n=215, dominant): FT Δ=−0.011. 12W/31L/172T. Closest to parity — already 72% of prod traffic.

**Levers, ranked by expected ROI (informed by FT-OAI-001 per-task breakdown + FT-OAI-002 findings):**
- **L1 — Fix `__unattributed__` training-data noise.** Per FT-OAI-002 E4 findings. 47-record slice has the biggest per-task delta; if the delta is an artifact of attribution ambiguity rather than model failure, cleaning the training set (or reclassifying records before training) directly recovers ~0.113 on those records.
- **L2 — Force `n_epochs=2`.** FT-OAI-001 best val loss was at step 1200/1500 (mild overfitting after). OpenAI auto-selected 3 epochs. Explicit `--n-epochs 2` (now available per FT-OAI-002 E6) should recover some of that overfit.
- **L3 — Upweight regressed tasks via `--task-weights`.** `retrieval.intent_translate` (4 records), `hidden.name_emergence` (13 records). These are small slices — upweighting them in training doesn't meaningfully change total token volume or cost.
- **L4 — Change training base.** Default: stay on `gpt-4.1-mini` (same base as FT-OAI-001 — cleanest A/B). Alternative: `gpt-4o-mini` if its per-token cost is materially lower AND a pre-training bench shows it starts at a reasonable quality anchor. This is an E1 decision, not a foregone conclusion.

**Intended outcome:** Either a deployable FT model that closes most of the remaining gap at a cost ratio that justifies swapping some or all `.env` LLM calls, OR a documented conclusion that FT is not currently competitive and we revisit after a base-model upgrade. Either outcome is valuable — the current state (FT-OAI-001 in limbo, no deploy decision, no cost numbers) is not.

---

## 3. Scope & Constraints

### In scope

- **E1 — Economic analysis**: actual OpenAI-bill per-token cost ratio of `gpt-4.1-mini` / `gpt-5.4-mini` (and `gpt-4o-mini` if in play). 3× FT inference-cost multiplier accounted for.
- **E1 — Deploy criterion**: the cosine-Δ-per-dollar-saved threshold that would justify deploying the FT model, expressed as a plain-language rule.
- **E2 — Training-base decision**: stay on `gpt-4.1-mini` (default) or switch to `gpt-4o-mini` (if economic case + pre-training bench both favour it).
- **E3 — Training-data preparation**: apply FT-OAI-002 E4 findings to clean `__unattributed__` slice; apply per-task reweighting (L3) via `--task-weights` (FT-OAI-002 E6).
- **E4 — Hyperparameter preset**: explicit `--n-epochs 2` (FT-OAI-002 E6); document rationale. If OpenAI FT API exposes learning-rate / batch-size overrides, consider tighter tuning.
- **E5 — Adapter pass + manifest**: regenerate train/val/test via the hardened FT-OAI-002 pipeline. Cost envelope honoured.
- **E6 — FT launch**: upload + launch (cost-gated). Project quota pre-check (FT-OAI-002 E7) mandatory before accept.
- **E7 — Cap-symmetric cross-base re-eval**: `gpt-5.4-mini` baseline at `--max-output-tokens 4096` (available per FT-OAI-002 E3) vs FT-OAI-003 at 4096, same 300-record seeded sample.
- **E8 — Deploy decision + `.env` changes**: per-task or universal, per the E1 criterion. Updates `.env` / `.env.example` if deploy authorised.
- **E9 — Integration smoke**: FT model (if deployed) exercises a real MDEMG request path (at least one task per deployed env var) before commit.
- **E10 — Documentation** (final epic — never cut): feature doc, CHANGELOG, AGENT_HANDOFF, run_notes, sprint summary on PR.

### Out of scope

- Local LoRA training (separate sprint line, already complete)
- Anthropic / Fireworks / other-provider FT (sibling adapter pattern when needed)
- New task types or new ULTS specs
- Changing the production base (`gpt-5.4-mini` is held constant as the quality target)

### Hard constraints

- **Cost cap**: ≤ $250 total (single FT launch up to ~$180 training + ≤ $30 re-eval + buffer). Hard-gated by `--max-cost-usd` on every script.
- **Quality floor**: FT-OAI-003 must score **≥ stock `gpt-4.1-mini`** (0.8322) cross-base — a regression vs the training base itself invalidates the sprint.
- **Parse-pass floor**: FT-OAI-003 must not regress JSON parse-pass below 0.97 on the 300-record sample.
- **`gpt-5.4-mini` never fine-tuned**: would defeat the cost-saving north star.
- **`.env` changes require cross-base verification** — deployment gated on re-eval numbers, not on training metrics alone.
- **All model calls** in evaluation and production use the per-task env-var architecture (`LLM_MODEL`, `RERANK_MODEL`, `SYNTHESIS_MODEL`, `INTENT_MODEL`, `EMERGENCE_MODEL`, `GUARDRAIL_MODEL`, `JIMINY_SYNTHESIS_MODEL`, `LLM_SUMMARY_MODEL`) — per-task deploy remains possible.

---

## 4. Dependencies

**Hard blockers (must complete before FT-OAI-003 E2 starts):**
- **FT-OAI-002** — task #142. E1 (harness bugs), E2 (per-record fields), E3 (cap-symmetric baseline), E4 (`__unattributed__` investigation), E6 (`--n-epochs` + `--task-weights` flags). Without E4, the biggest ROI lever (L1) is blind.
- **Economic analysis** — actual OpenAI billing report for the period containing both `gpt-4.1-mini` and `gpt-5.4-mini` usage. Pulls from OpenAI dashboard or usage API.

**Soft dependencies:**
- Fresh TSDB data (via `mdemg data curate --paradigm sft` against current `mdemg-dev`)
- `openai>=1.50` Python SDK (already installed in `neural/.venv`)
- `tiktoken>=0.7` (already installed)
- `gpt-4o-mini` FT availability (OpenAI docs) — only if L4 alternative chosen

**Reference artifacts (read-only):**
- `training_data/openai_ft/20260420/run_notes.md` — FT-OAI-001 baseline
- `training_data/openai_ft/20260420/eval_comparison_vs_gpt54mini.md` — cross-base target
- FT-OAI-002 outputs (G1/G2/G3/R1/T1–T4/O1–O4 resolutions)

---

## 5. Implementation Plan

Sequential epics, each with explicit gate. Do **not** parallelize epics (per project rule).

### Epic 0 — Readiness gate

- FT-OAI-002 task #142 marked complete
- FT-OAI-002 E4 `__unattributed__` investigation findings reviewed; decision made: clean the slice, drop the slice, or keep it (with documented rationale)
- OpenAI project quota verified ≥ $250
- Branch `reh3376_dev01` fast-forwarded with `main`
- **Gate**: all bullets above confirmed in writing before Epic 1 begins

### Epic 1 — Economic analysis + deploy criterion

- Pull actual OpenAI-bill cost per 1K tokens for `gpt-4.1-mini`, `gpt-5.4-mini`, and `gpt-4o-mini` (if in play) for the most recent full billing period
- Account for the 3× FT-inference-cost multiplier
- Compute the per-token cost ratio `cost(prod) / cost(cheap_base * 3)` — this is the max tolerable quality loss per FT-task, in cosine-points-per-dollar-saved
- Write a plain-language deploy criterion: "If FT-OAI-003 cross-base Δ ≥ X on task T and task T carries ≥ Y% of production traffic, deploy FT for task T."
- **Gate**: criterion + cost ratio table committed to `training_data/openai_ft/<YYYYMMDD>/economic_analysis.md`. This is the number every later decision cites.

### Epic 2 — Training-base decision

- If Epic 1 shows `gpt-4o-mini` is materially cheaper than `gpt-4.1-mini` AND OpenAI FT supports it AND a quick 50-record bench shows `gpt-4o-mini` stock quality is ≥ 0.80 mean cosine: run candidate bench of `gpt-4o-mini` stock vs `gpt-4.1-mini` stock on same seeded sample
- Else: proceed with `gpt-4.1-mini` (default — cleanest A/B vs FT-OAI-001)
- Document the decision + numbers in `economic_analysis.md`
- **Gate**: training base selected and recorded. If `gpt-4o-mini` selected, bench numbers prove the quality starting point.

### Epic 3 — Training-data preparation

- **L1 — `__unattributed__` cleanup**: apply FT-OAI-002 E4 findings
  - If E4 concluded the slice is noisy heuristic output, either drop those records from training OR reclassify them correctly in `quality_filter.py` (or a new task-attribution pass) before they reach the adapter
  - If E4 concluded the slice is legitimate, re-examine whether FT-OAI-001 failure mode is recoverable by reweighting alone
- **L3 — Per-task reweighting**: via `--task-weights` (FT-OAI-002 E6)
  - `retrieval.intent_translate`: weight ×3 (4 records → effective 12)
  - `hidden.name_emergence`: weight ×2 (13 records → effective 26)
  - Other regressed tasks: weight ×1.5 if the aggregate weight budget allows
- Regenerate adapter output (`combined_train.jsonl`, `combined_val.jsonl`, `manifest.json`)
- **Gate**: manifest shows cleaned `__unattributed__` (or documented retention decision), task-weight application, cost estimate within Epic 1 budget.

### Epic 4 — Hyperparameter preset

- `--n-epochs 2` (FT-OAI-002 E6 flag) — per the best-val-step 1200/1500 signal from FT-OAI-001
- Other hyperparameter overrides (learning-rate multiplier, batch size) considered only if FT-OAI-002 exposed them and the evidence justifies
- Document rationale inline in the launch command + run_notes.md
- **Gate**: launch command in `run_notes.md` with the preset values and one-line rationale per flag.

### Epic 5 — Adapter pass + cost gate

- Run `python -m training.openai_ft_adapter` with E3 inputs + E4 hyperparameter flags
- Verify `manifest.json` cost estimate ≤ Epic 1 budget after auto-epoch multiplier
- Verify no new `rejection_log.jsonl` entries from the cleanup work (we should have cleaned, not filtered)
- **Gate**: manifest verified; rejection log empty or unchanged; cost estimate recorded.

### Epic 6 — FT launch

- Project quota pre-check (FT-OAI-002 E7 output) — abort if quota insufficient
- `scripts/openai_ft_upload_and_launch.py` with `--suffix mdemg-ftoai003 --max-cost-usd 250.00`
- Monitor via `scripts/openai_ft_check.py --watch`
- On completion, capture: model ID, trained tokens, actual epochs, final train/val loss, best val loss step, total cost
- Append to `run_notes.md`
- **Gate**: job status `succeeded`; FT model ID recorded; train/val loss trajectory matches expectation (n_epochs=2, no overfit inflection).

### Epic 7 — Cap-symmetric cross-base re-eval

- **7.1** — `gpt-5.4-mini` baseline at 4096 cap, seed=42, n=300 (reuses FT-OAI-002 E3 output if already produced)
- **7.2** — FT-OAI-003 eval at 4096 cap, seed=42, n=300 (same seeded records)
- **7.3** — Comparator: `scripts/openai_ft_compare.py --baseline <gpt54mini-4096> --ft <ftoai003>` → `eval_comparison_v3_vs_gpt54mini.md`
- **7.4** — Cross-base progress table: stock-4.1-mini anchor, FT-OAI-001 anchor, FT-OAI-003 result, stock-5.4-mini anchor. Compute % of gap closed.
- **Gate**: quality floor met (≥ 0.8322 cross-base); parse-pass ≥ 0.97; per-task breakdown produced; gap-closed % computed.

### Epic 8 — Deploy decision + `.env` changes

- Apply the Epic 1 criterion to the Epic 7 numbers
- For each of the 8 per-task env vars (`LLM_MODEL`, `RERANK_MODEL`, `SYNTHESIS_MODEL`, `INTENT_MODEL`, `EMERGENCE_MODEL`, `GUARDRAIL_MODEL`, `JIMINY_SYNTHESIS_MODEL`, `LLM_SUMMARY_MODEL`), compare the FT Δ on the relevant task slice against the criterion
- Produce a deploy matrix: `{env_var: model_to_deploy}`. Can be universal (all 8 → FT) or per-task (some → FT, some → `gpt-5.4-mini`).
- Update `.env` and `.env.example` with the matrix. Keep `.env.example` annotated with the reasoning inline.
- If deploy matrix is "stay on prod for all" — document that clearly and move straight to Epic 10 (documentation includes the learned lesson).
- **Gate**: deploy matrix explicit; `.env` / `.env.example` in sync; every change has inline justification.

### Epic 9 — Integration smoke

- If Epic 8 deployed FT to any env var: exercise that task via a real MDEMG request path
  - `LLM_MODEL` → generic chat completion via any `/v1/*` endpoint that calls `llmclient`
  - `RERANK_MODEL` → `/v1/retrieval/rerank` call
  - `SYNTHESIS_MODEL` → Jiminy synthesis flow
  - `INTENT_MODEL` → query-classify flow
  - `EMERGENCE_MODEL` → concept emergence path (if `EMERGENCE_ENABLED=true`)
  - `GUARDRAIL_MODEL` → guardrail classification path
  - `JIMINY_SYNTHESIS_MODEL` → Jiminy tier-2 outcome flow
  - `LLM_SUMMARY_MODEL` → summarisation path
- Verify no 400s (model ID recognized), latency within 3× expected, output parse-pass
- Roll back immediately if any smoke fails; document in run_notes.md
- **Gate**: at least one smoke call per deployed env var returns HTTP 200 with parseable output.

### Epic 10 — Documentation (never cut)

- `docs/features/fine-tuning-pipeline.md` — update Current State section with FT-OAI-003 numbers + gap-closing table
- `CHANGELOG.md` — `[Unreleased]` → new entry documenting FT-OAI-003, deploy matrix, cost saving estimate
- `AGENT_HANDOFF.md` — update Fine-Tuning Roadmap table; add FT-OAI-003 row with status + outcome
- `training_data/openai_ft/<YYYYMMDD>/run_notes.md` — full run log
- Post sprint summary to the auto-PR comments (per project rule: `feedback_sprint_summary_on_pr.md`)
- **Gate**: all 5 bullets checked; PR comment posted.

---

## 6. Testing Plan

Three-tier structure (mandatory per `memory/feedback_mandatory_testing_tiers.md`):

### Tier 1 — Unit / lint / static

- `golangci-lint run ./...` — zero new warnings
- Python adapter + scripts: `ruff check` + `mypy` (match FT-OAI-002 bar)
- `python -m pytest neural/training/tests/` — all passing
- Any new flag additions (unlikely — most additions land in FT-OAI-002) carry unit coverage

### Tier 2 — Integration / dry-run

- Adapter dry-run on 20 records, verify manifest fields + rejection log
- Upload script with `--dry-run` flag (mocked network) — verify cost gate behaviour
- Monitor script with a replayed prior job transcript — verify state transitions
- Eval harness with `--sample-size 5 --max-cost-usd 0.10` on a stock model — verify seed determinism + field completeness (parse_ok, finish_reason, tokens per FT-OAI-002 E1/E2)

### Tier 3 — E2E / live OpenAI

- **T3.1** — Full FT launch (Epic 6). Real tokens, real cost. Cost-gated at ≤ $250.
- **T3.2** — Full cross-base re-eval (Epic 7). Real tokens. Cost-gated at ≤ $30.
- **T3.3** — Integration smoke (Epic 9). If Epic 8 deployed. Real `/v1/*` calls against local MDEMG server with updated `.env`.

---

## 7. Commit Strategy

Sequential, Epic-boundary commits on `reh3376_dev01`:

| # | Commit subject | After Epic |
|---|---|---|
| 1 | `docs(ft-oai-003): economic analysis + deploy criterion` | E1 |
| 2 | `feat(ft-oai-003): training-base decision + pre-training bench` | E2 (if `gpt-4o-mini` path) |
| 3 | `feat(ft-oai-003): training-data prep — __unattributed__ cleanup + task reweighting` | E3 |
| 4 | `feat(ft-oai-003): hyperparameter preset — n_epochs=2 + task-weights` | E4 |
| 5 | `feat(ft-oai-003): regenerate adapter output for v3 training` | E5 |
| 6 | `chore(ft-oai-003): launch v3 fine-tuning job` | E6 (run_notes update only — large artifacts gitignored) |
| 7 | `feat(ft-oai-003): cross-base re-eval + comparator report` | E7 |
| 8 | `feat(.env): deploy FT-OAI-003 per deploy matrix` | E8 (only if any env var changes) |
| 9 | `test(ft-oai-003): integration smoke for deployed tasks` | E9 |
| 10 | `docs(ft-oai-003): feature doc + CHANGELOG + AGENT_HANDOFF` | E10 |

Branch naming: `reh3376_dev01` (primary dev). Auto-PR fires on push; sprint summary comment posted per project rule.

---

## 8. Verification Checklist

- [ ] FT-OAI-002 task #142 complete
- [ ] `economic_analysis.md` committed with cost ratio + deploy criterion
- [ ] Training base selected (default `gpt-4.1-mini` or justified `gpt-4o-mini`)
- [ ] `__unattributed__` cleanup applied (or retention documented)
- [ ] Task weights applied per L3
- [ ] `--n-epochs 2` enforced
- [ ] FT job succeeded; model ID captured
- [ ] Cross-base re-eval completed at 4096 cap, seed=42, n=300
- [ ] Quality floor met: cross-base mean cosine ≥ 0.8322 (stock `gpt-4.1-mini`)
- [ ] Parse-pass ≥ 0.97
- [ ] Deploy matrix committed with per-env-var justification
- [ ] `.env` + `.env.example` in sync (if any deploys)
- [ ] Integration smoke green for every deployed env var
- [ ] `docs/features/fine-tuning-pipeline.md` updated
- [ ] `CHANGELOG.md` updated
- [ ] `AGENT_HANDOFF.md` Fine-Tuning Roadmap row added
- [ ] Sprint summary posted to auto-PR comments

---

## 9. Documentation Update (Final Epic — never cut)

Covered by Epic 10 above. Rationale for carve-out: per `memory/feedback_mandatory_testing_tiers.md` and CLAUDE.md — docs updates are the LAST epic, never cut, every sprint.

---

## 10. Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| 1 | FT-OAI-002 E4 `__unattributed__` investigation inconclusive — can't tell if slice is noise or signal | M | M | Run E4 with a sample examined manually; if inconclusive, **retain** the slice with a weight of 0.5 rather than drop it. Track as an unresolved known-limitation in feature doc. |
| 2 | `gpt-4o-mini` is FT-unsupported or materially worse as a starting point | L | L | E2 bench catches this — fall back to default `gpt-4.1-mini`. |
| 3 | Cost ratio doesn't justify deployment at any quality Δ | L-M | M | Epic 8 produces a "do not deploy" verdict with full rationale. This is a valid outcome — the economic analysis itself has value. |
| 4 | FT-OAI-003 regresses against FT-OAI-001 (v3 worse than v2 on cross-base) | L | H | Quality floor in §3 (must be ≥ stock `gpt-4.1-mini`). If not met, do not deploy; post-mortem in run_notes.md. |
| 5 | OpenAI changes FT API or base model availability between FT-OAI-002 and FT-OAI-003 | L | M | Epic 0 readiness gate re-checks API availability. If breaking change, delay sprint; do not rush around it. |
| 6 | Auto-epoch cost multiplier still underestimated (FT-OAI-001 was 1.66× single-epoch) | M | L | Adapter cost envelope now models `n_epochs=2` explicitly (no auto). `--max-cost-usd 250` absorbs the uncertainty. |
| 7 | `.env` changes cause production regression on undeployed task slices | L | H | Integration smoke Epic 9 exercises every deployed env var. Roll back on any 400/timeout/parse failure. |
| 8 | Queue-wait time blocks sprint (FT-OAI-001 attempt 3 waited 9h 8m) | M | L | Start Epic 6 early in the day; use FT-OAI-002 O2 auto-resubmit-on-queue-stuck if available. |

---

## 11. Documents Accessed

- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/run_notes.md` (lines 95–158 — cross-base bench section, gap-closing table)
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/eval_comparison_vs_gpt54mini.md` (per-task breakdown, worst regressions)
- `/Users/reh3376/mdemg/docs/features/fine-tuning-pipeline.md` (current state, known limitations)
- `/Users/reh3376/mdemg/CHANGELOG.md` (FT-OAI-001 entry under [Unreleased])
- `/Users/reh3376/mdemg/AGENT_HANDOFF.md` (Fine-Tuning Roadmap section)
- `/Users/reh3376/mdemg/docs/development/ft-oai/sprint_plan_ft_oai_002.md` (scope, epic structure, cross-base finding section)
- `/Users/reh3376/mdemg/CLAUDE.md` (project rules: sequential epics, mandatory testing tiers, Documents Accessed appendix, sprint summary on PR)
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` (feedback files for sprint plan format, sequential epics, primary branch, auto-PR, sprint summary on PR)
- `/Users/reh3376/mdemg/neural/training/openai_ft_adapter.py` (adapter CLI flags — read indirectly via references in FT-OAI-002 plan)
- `/Users/reh3376/mdemg/scripts/openai_ft_baseline_eval.py` (pricing table; `gpt-5.4-mini` entry added during FT-OAI-001)

---

## 12. Rollback Procedures

Applies only if Epic 8 deploys FT to any env var.

| Trigger | Action |
|---|---|
| Integration smoke (Epic 9) 400 / timeout / parse failure | `git revert` the Epic 8 `.env` commit; confirm `gpt-5.4-mini` restored; re-run smoke |
| Production parse-pass regression detected post-deploy via TSDB `llm_interactions` | Same revert; open FT-OAI-004 post-mortem task |
| Cost explosion (FT-inference 3× multiplier not absorbed at actual traffic volume) | Per-task rollback of the most expensive env var first (`LLM_MODEL`); monitor 24h; further rollback if insufficient |

All rollbacks are **reversible** — `.env` is a config file, reverts in seconds. No database or schema state is modified by this sprint.

---

## Appendix A — Decision table for Epic 8 (template)

| Env var | Task(s) served | FT Δ vs prod (Epic 7) | Traffic % | Cost save | Deploy? | Justification |
|---|---|---|---|---|---|---|
| `LLM_MODEL` | generic chat | TBD | 100% | TBD | TBD | TBD |
| `RERANK_MODEL` | retrieval.rerank_cross | TBD | low | TBD | TBD | TBD |
| `SYNTHESIS_MODEL` | jiminy.synthesize | TBD | low | TBD | TBD | TBD |
| `INTENT_MODEL` | retrieval.intent_translate | TBD | low | TBD | TBD | — worst regression in FT-OAI-001; R1 deep-dive gates this |
| `EMERGENCE_MODEL` | hidden.name_emergence | TBD | low | TBD | TBD | — 2nd worst regression in FT-OAI-001 |
| `GUARDRAIL_MODEL` | consulting.classify | TBD | low | TBD | TBD | — FT tied prod in FT-OAI-001 |
| `JIMINY_SYNTHESIS_MODEL` | jiminy.evaluate_llm | TBD | low | TBD | TBD | — FT tied prod in FT-OAI-001 |
| `LLM_SUMMARY_MODEL` | summarisation | TBD | low | TBD | TBD | TBD |

Populated at end of Epic 7; drives Epic 8 decisions.
