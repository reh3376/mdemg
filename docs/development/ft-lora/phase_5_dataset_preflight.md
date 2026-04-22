# Phase 5 SFT — Dataset Pre-Flight Report

**Date:** 2026-04-22
**Author:** Pre-flight run post-Sprint-E merge (`14cd2b3`)
**Purpose:** Verify dataset readiness before authoring the Phase 5 SFT runbook. Per the sequencing memo received after PR #344 merged, this pre-flight either greenlights runbook authoring or surfaces blockers that need their own sprint.
**Verdict:** **🛑 BLOCKED — do NOT proceed to runbook authoring. A dedicated dataset sprint (working title: FT-LORA-DATA) is required first.**
**Budget consumed:** ~40 min planner time, $0 spend, no training.

---

## 1. Executive Summary

The Phase 5 cheat-sheet landed in Sprint E's `03_IMPLEMENTATION_PLAN_v2.md` (§5A/5C) and Sprint E commit body. It references four JSONL files under `training_data/sft/`:

```
training_data/sft/mdemg_sft.jsonl                   (Tier 1, balanced 16 tasks)
training_data/sft/family_reasoning_think.jsonl      (Tier 2, T-family)
training_data/sft/family_classify_notink.jsonl      (Tier 2, C-family)
training_data/sft/family_structured_notink.jsonl    (Tier 2, J-family)
```

**None of these files exist. The `training_data/sft/` directory does not exist.**

Three separate datasets that *do* exist approximate pieces of what Phase 5 needs, but none is drop-in:

| File | Rows | Format | task_name preserved? |
|---|---|---|---|
| `training_data/curated/sft_interactions/filtered.jsonl` | 38,412 | raw TSDB schema | **Yes** |
| `training_data/curated/sft_interactions/converted.jsonl` | 38,412 | chat `{"messages":[…]}` | **No** (stripped during conversion) |
| `training_data/openai_ft/20260420/combined_train.jsonl` | 2,000 | chat | No |
| `training_data/openai_ft/20260420/combined_val.jsonl` | 400 | chat | No |

Even if the file-existence gap were purely cosmetic (it is not), the **underlying data has structural problems** that block Tier-1 and Tier-2 SFT as specified in memo 07 v3.1 §3.2:

1. **Only 12 of 16 target tasks have any data.** 4 tasks — `consulting.synthesis`, `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate` — are completely absent from the TSDB export.
2. **Only 4 of 16 tasks have ≥500 rows** (the memo target). 7 tasks sit between 133 and 315 rows; 1 task has 1 row (`hidden.summarize`); 1 task has 45 rows (`jiminy.evaluate_llm`).
3. **Raw distribution is catastrophically skewed:** `ape.reflect` is 88.8% of the curated SFT corpus (34,093 / 38,412). Balanced per-task sampling per `05_DATA_COLLECTION_v2.md` Appendix A is not pre-applied in any existing artifact.
4. **No train/valid split exists at the granularity Phase 5 needs.** mlx_lm.lora expects `--data <dir>` containing `train.jsonl` + `valid.jsonl`; existing splits are at the monolithic-2400-row FT-OAI level, not per-tier/per-family.
5. **Provenance tagging is lost.** Sprint D flagged that donor-backfill was used for 5 under-served tasks in the anchor-prompt set (`sprint_plan_ft_lora_d.md` §Deviation). That same imbalance exists in the SFT corpus, but there is no metadata field in `converted.jsonl` or `filtered.jsonl` distinguishing production from donor rows.

Per the sequencing memo's explicit directive:

> PROBLEMS (any blocker surfaced): Do NOT proceed to runbook. Draft a separate dataset sprint (working title: FT-LORA-DATA) to address the gaps. … DO NOT fold dataset fixes into the Phase 5 runbook as "we'll clean it up during prep." Each discovery gets its own scope boundary.

This report enumerates the findings from each of the 7 required checks, then outlines the scope of the follow-up dataset sprint.

---

## 2. Check 1 — File Existence

**Expected (from Sprint E Phase 5 cheat-sheet):**

| Path | Role | Present? |
|---|---|---|
| `training_data/sft/mdemg_sft.jsonl` | Tier 1 balanced | ❌ ABSENT |
| `training_data/sft/family_reasoning_think.jsonl` | Tier 2 T | ❌ ABSENT |
| `training_data/sft/family_classify_notink.jsonl` | Tier 2 C | ❌ ABSENT |
| `training_data/sft/family_structured_notink.jsonl` | Tier 2 J | ❌ ABSENT |

**Actual state of `training_data/`:**

```
training_data/
├── converted/          (empty)
├── curated/
│   ├── raft_retrieval/ (RAFT dataset, 720 MB converted.jsonl)
│   └── sft_interactions/
│       ├── converted.jsonl  (719.5 MB — chat format, 38,412 rows)
│       ├── filtered.jsonl   (743.3 MB — raw schema, 38,412 rows)
│       └── versioned/
├── filtered/           (empty)
├── openai_ft/
│   └── 20260420/
│       ├── combined_train.jsonl  (39.1 MB — 2,000 rows, chat)
│       ├── combined_val.jsonl    ( 5.9 MB —   400 rows, chat)
│       ├── manifest.json
│       ├── run_notes.md (FT-OAI-001 forcing function)
│       └── …
├── raw/
│   └── extracted/
│       ├── llm_interactions.jsonl  (767 MB — 42,727 rows raw TSDB dump)
│       └── manifest.json
└── routing_profiles/   (Sprint D artifacts, complete)
```

**No `training_data/sft/` path anywhere.** All 4 expected files are absent.

---

## 3. Check 2 — Row Counts Per Task (16 tasks)

**Target per memo 07 v3.1 §3.2:** 500–1000 examples × 16 tasks = 8,000–16,000 rows for Tier 1.

**Task-to-family mapping** (from `04_BENCHMARK_RL_v2.md §11.2`):

- **T — reasoning-think (7 tasks):** `ape.reflect`, `consulting.synthesis`, `hidden.summarize`, `jiminy.synthesize`, `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate`
- **C — classify-notink (6 tasks):** `consulting.classify`, `hidden.reclassify`, `jiminy.evaluate`, `jiminy.codegen`, `retrieval.intent_translate`, `retrieval.query_classify`
- **J — structured-notink (3 tasks):** `hidden.name_emergence`, `jiminy.evaluate_llm`, `retrieval.rerank_cross`

**Observed per-task row counts in `curated/sft_interactions/filtered.jsonl`:**

| Family | Task | Rows | Status vs 500 target |
|---|---|---|---|
| T | `ape.reflect` | 34,093 | ✅ (oversupplied; 88.8% of corpus) |
| T | `consulting.synthesis` | **0** | 🛑 **HARD BLOCKER — absent** |
| T | `hidden.summarize` | 1 | 🛑 **HARD BLOCKER — 1 row** |
| T | `jiminy.synthesize` | 133 | ⚠️ below target (27% of floor) |
| T | `metalearn.generalize` | **0** | 🛑 **HARD BLOCKER — absent** |
| T | `retrieval.rerank_nli` | **0** | 🛑 **HARD BLOCKER — absent** |
| T | `summarize.generate` | **0** | 🛑 **HARD BLOCKER — absent** |
| C | `consulting.classify` | 1,040 | ✅ |
| C | `hidden.reclassify` | 223 | ⚠️ below target (45%) |
| C | `jiminy.evaluate` | 315 | ⚠️ below target (63%) |
| C | `jiminy.codegen` | 212 | ⚠️ below target (42%) |
| C | `retrieval.intent_translate` | 198 | ⚠️ below target (40%) |
| C | `retrieval.query_classify` | 146 | ⚠️ below target (29%) |
| J | `hidden.name_emergence` | 501 | ✅ (just clears floor) |
| J | `jiminy.evaluate_llm` | 45 | 🛑 **HARD BLOCKER — 9% of floor** |
| J | `retrieval.rerank_cross` | 1,505 | ✅ |

**Summary:**
- ≥500 rows: **4 / 16** (`ape.reflect`, `consulting.classify`, `hidden.name_emergence`, `retrieval.rerank_cross`)
- 100–499 rows: 7 tasks
- < 100 rows: 2 tasks
- **0 rows (absent entirely): 4 tasks**

**Family-level roll-up:**

| Family | Tasks | Total rows in filtered.jsonl | Usable tasks (≥500 rows) |
|---|---|---|---|
| T | 7 | 34,227 (99.6% is `ape.reflect`) | 1 / 7 |
| C | 6 | 2,134 (distributed across 6) | 1 / 6 |
| J | 3 | 2,051 (distributed across 3) | 2 / 3 |

**Tier 2 family files cannot be constructed** from existing data for T-family at anywhere near memo-target granularity — 6 of 7 T-tasks are effectively missing. Any T-family Tier 2 LoRA trained today would overfit on `ape.reflect` and leave the rest of the family unrepresented.

---

## 4. Check 3 — Schema Compatibility with mlx_lm.lora

**mlx_lm 0.31.2 `--data` semantics** (from `python -m mlx_lm.lora --help`):

> `--data DATA  Directory with {train, valid, test}.jsonl files or the name of a Hugging Face dataset`

**Two implications Sprint E's cheat-sheet got wrong:**

1. `--data` (alias `--dataset` in `train_ft.py`) takes a **directory**, not a single JSONL file. Sprint E's cheat-sheet writes `--dataset training_data/sft/mdemg_sft.jsonl` which would fail at argparse stage. The correct form is `--dataset training_data/sft/tier1/` with `train.jsonl`+`valid.jsonl` inside.
2. The val filename must be **`valid.jsonl`** (not `val.jsonl`, not `mdemg_sft.val.jsonl`).

**Record-level format supported by mlx_lm.lora:** chat (`{"messages":[{"role":...,"content":...}]}`) or completion (`{"prompt":..., "completion":...}`).

**Schemas in existing files:**

| File | Schema | mlx_lm-compatible? |
|---|---|---|
| `curated/sft_interactions/converted.jsonl` | `{"messages": [system, user, assistant]}` | ✅ yes |
| `curated/sft_interactions/filtered.jsonl` | raw TSDB schema (27 columns) | ❌ no — needs conversion |
| `openai_ft/20260420/combined_train.jsonl` | `{"messages": [system, user, assistant]}` | ✅ yes |
| `openai_ft/20260420/combined_val.jsonl` | `{"messages": [system, user, assistant]}` | ✅ yes |

**Sample row from `converted.jsonl` (Check 3 sample; hidden.name_emergence shape):**

```json
{"messages": [
  {"role": "system",    "content": "You are a code classification engine. …"  (645 chars)},
  {"role": "user",      "content": "Language category: python …"            (26,149 chars)},
  {"role": "assistant", "content": "[{\"name\":\"ml_models_and_training\" …"  (2,024 chars)}
]}
```

**Format is OK**, but `task_name` is stripped during the raw→chat conversion, so producing per-family subsets from `converted.jsonl` alone is not possible. A per-task/per-family split must join `converted.jsonl` with `filtered.jsonl` on `trace_id` (or regenerate `converted.jsonl` with `task_name` preserved as a metadata field).

**Status:** schema is compatible in principle, but **provenance join is required** to produce Tier 2 family files. That join is part of the follow-up sprint.

---

## 5. Check 4 — Balanced Sampling State

**Tier 1 target** (from `05_DATA_COLLECTION_v2.md` Appendix A): each of the 16 tasks weighted equally regardless of row count, via a per-task oversample-to-equal strategy or a per-task balanced sampler at batch construction time.

**Observed Tier 1 corpus shape** (would-be `mdemg_sft.jsonl` built from `converted.jsonl`):

| Task | Rows | % of corpus |
|---|---|---|
| `ape.reflect` | 34,093 | 88.76% |
| `retrieval.rerank_cross` | 1,505 | 3.92% |
| `hidden.name_emergence` | 501 | 1.30% |
| `consulting.classify` | 1,040 | 2.71% |
| (8 other tasks) | ~1,273 | 3.31% |
| (4 absent tasks) | 0 | 0.00% |

**Verdict:** raw distribution is catastrophically unbalanced. Even a perfect sampler cannot invent rows for the 4 absent tasks.

**`train_ft.py` sampler-wrapper status:** Sprint E's `train_ft.py` is a thin `subprocess.Popen` wrapper around `mlx_lm.lora` — it does not own the data pipeline. Mlx_lm.lora reads `train.jsonl` sequentially (no per-task balancing). **Balanced per-task sampling must happen before `train.jsonl` is written** (e.g., produce a balanced JSONL via a curation script that uses oversample-with-replacement on under-represented tasks up to the max per-task cap). This is straightforward to implement but is work that does not exist today.

**Flagged for follow-up sprint:** the balancing curator lives in the dataset sprint, not in `train_ft.py`.

---

## 6. Check 5 — Train/Valid/Test Split Presence

**Early-stop dependency:** Sprint E's `early_stop.py` parses `Val loss {:.3f}` from `mlx_lm.lora` stdout. `mlx_lm.lora` only emits this line when it has a `valid.jsonl` to evaluate against. **Without a val split, early-stop cannot fire and the epoch-cap policy becomes the only brake.** That's unsafe — FT-OAI-001 overfit at step 1200 with a val set; silencing the val signal removes the forcing-function's teeth.

**Observed split state:**

| Dataset | train split | valid split | test split |
|---|---|---|---|
| `curated/sft_interactions/` | no | no | no |
| `openai_ft/20260420/` | `combined_train.jsonl` (2,000) | `combined_val.jsonl` (400) — **wrong filename** for mlx_lm (expects `valid.jsonl`) | no |
| `sft/tier1/` (Phase 5 expected) | ❌ missing | ❌ missing | ❌ missing |
| `sft/family_*/` (Phase 5 expected) | ❌ missing × 3 | ❌ missing × 3 | ❌ missing × 3 |

**Split ratio recommendation** (not decided by this pre-flight; noted for the follow-up sprint): memo 07 v3.1 §3.2 implies ~90/10 train/val; FT-OAI-001 used 83/17 (2000/400 = 16.7% val). Pick one and apply it consistently across Tier 1 + 3 Tier 2 splits.

**Status:** ❌ **HARD BLOCKER** — no usable val split exists at the tier/family granularity Phase 5 needs.

---

## 7. Check 6 — SHA-256 Stamping

For Phase-5-runbook input-immutability verification (Sprint C model-SHA-pin pattern), the candidate source files and their full-file SHAs are:

```
training_data/curated/sft_interactions/filtered.jsonl
  sha256: 31b9f10032618588f080741d85f8fcfd0d5a8e3d2e22612344ee1a85f535c579
  size:   743.3 MB
  rows:   38,412

training_data/curated/sft_interactions/converted.jsonl
  sha256: d3be8d34bc64b12311458f20598e712cc8bb39188fd9af1223512f205f2a50f4
  size:   719.5 MB
  rows:   38,412

training_data/openai_ft/20260420/combined_train.jsonl
  sha256: 3b89b459812e1cf2225cc1245c724c78e9eebef9a2c4833ef28946560f834030
  size:    39.1 MB
  rows:   2,000

training_data/openai_ft/20260420/combined_val.jsonl
  sha256: faea8e5c1cc4f781df98d07798156573178908452bbe834f6ff4f328fd4b19b2
  size:     5.9 MB
  rows:   400
```

Source TSDB export row count: 42,727 (per `raw/extracted/manifest.json`) — so `filtered.jsonl` at 38,412 rows represents a ~90% retention after quality filtering, consistent with the documented 2.28% llm_error_rate + 976 empty-response rows.

**The SHAs above will be stale the moment the dataset sprint writes new files.** Phase 5 runbook will pin to the new files' SHAs, not these.

---

## 8. Check 7 — Donor-Backfill Audit

Sprint D's `sprint_c_d_profile_results.md` disclosed that 99 / 320 anchor prompts were donor-backfilled from same-shape tasks to compensate for 5 T-family tasks with 0–1 production records. That same structural imbalance propagates into the SFT corpus, but with a different failure mode:

- Sprint D only needed 20 prompts/task for routing-profile stability; donor rows at that scale are low-stakes.
- Phase 5 needs 500–1000 rows/task for SFT; donor rows at that scale will train the model on *donor behavior*, not the target task's behavior. That's only acceptable if donor-vs-production is **explicitly labeled** so the loss/eval pipeline can either weight donors lower or exclude them from val.

**Observed provenance-tag state:**

- `filtered.jsonl` has 27 columns. None is a provenance tag distinguishing production from donor. The closest available field is `source_path`, which carries the TSDB table/hypertable name (not helpful for this distinction).
- `converted.jsonl` preserves only `messages` — all original columns including `source_path`, `trace_id`, and `task_name` are dropped.
- `combined_train.jsonl` is a subsample of `converted.jsonl` and inherits its impoverished schema.

**Verdict:** donor rows cannot be distinguished from production rows in any existing artifact. For the 4 completely-absent T-tasks, the only options are (a) collect more production data, (b) synthesize from an LLM (donor with explicit tag), or (c) acknowledge the task is out of scope for the first SFT pass.

**Not a hard blocker by itself** — the memo doesn't mandate donor labeling — but combined with the 4-absent-tasks problem, it forces the follow-up sprint to make an explicit data-generation policy decision.

---

## 9. Decision

**Blocker tier (any one is sufficient to halt):**

1. 🛑 4 of 16 target tasks are entirely absent from the SFT corpus.
2. 🛑 Only 4 of 16 tasks meet the memo's 500-row floor.
3. 🛑 No Tier-1 balanced JSONL exists; no Tier-2 family JSONLs exist.
4. 🛑 No per-tier/per-family train/valid splits exist — early-stop cannot fire.

**Non-blocking findings worth recording:**

- Sprint E cheat-sheet's `--dataset <file.jsonl>` syntax is wrong (mlx_lm takes a directory). Fix in the Phase 5 runbook, not here.
- Donor-vs-production provenance is lost in `converted.jsonl`. Restore it in the re-curation step.
- `combined_val.jsonl` filename should be `valid.jsonl` inside a `<tier>/` dir for mlx_lm compatibility.

**Verdict: BLOCKED.**

**Do NOT proceed to Step 2 (Phase 5 SFT Runbook).**
**Do propose FT-LORA-DATA as the next sprint.**

---

## 10. Proposed Follow-Up Sprint: FT-LORA-DATA

Purely a proposal — user owns the decision to approve, revise, or scope out.

**Scope (planner-authored, no training, probably 1–2 working days):**

1. **Close the 4-task gap** on T-family absent tasks (`consulting.synthesis`, `metalearn.generalize`, `retrieval.rerank_nli`, `summarize.generate`):
   - (a) Investigate whether any of them exist under alternate `task_name` spellings in the raw TSDB (e.g., `summarize.generate` vs `summarize`); if so, relabel.
   - (b) For tasks that have zero production traffic, either (i) activate them in the live system and wait, (ii) synthesize with an explicit `provenance=synthetic` tag, or (iii) descope them from the first SFT pass and document in memo 07 v3.2.
   - Decision belongs to the user; sprint plan lays out the trade-offs.
2. **Re-curate `converted.jsonl` preserving provenance.** Keep `task_name`, `trace_id`, `provenance` (= `production` | `donor` | `synthetic`) as top-level fields alongside `messages`. Enables per-task filtering and donor-aware training.
3. **Produce the Phase 5 dataset artifacts** under `training_data/sft/`:
   - `training_data/sft/tier1/{train,valid}.jsonl` — 16-task balanced via oversample-with-replacement up to max-per-task cap (user picks cap; candidates: 500, 1000).
   - `training_data/sft/family_{reasoning_think,classify_notink,structured_notink}/{train,valid}.jsonl` — family subsets filtered from the provenance-preserving corpus.
   - Per-file SHA256 stamping + manifest.json recording row counts, per-task breakdowns, and provenance histograms.
4. **CLI additions to `mdemg data curate` / new `mdemg data build-sft-corpus`** as appropriate. These are small Python scripts, not new Go code.
5. **3-tier testing** (unit on the curator, integration on a small synthesized corpus, E2E producing the 4 artifacts end-to-end).
6. **Documentation:** sprint plan at `docs/development/ft-lora/sprint_plan_ft_lora_data.md`; update `03_IMPLEMENTATION_PLAN_v2.md` Phase 5 cheat-sheet to correct the `--dataset <dir>` syntax + reference the new files by SHA; update `00_README_v2.md` v5.4 → v5.5.

**Estimate:** 1–2 working days of planner time, $0 spend, no training.

**After FT-LORA-DATA merges:** Step 2 (Phase 5 SFT Runbook) becomes unblocked, with new, correct file paths and SHAs to pin.

---

## 11. Documents Accessed

**Read during pre-flight:**
- `/Users/reh3376/mdemg/training_data/` — full directory tree
- `/Users/reh3376/mdemg/training_data/raw/extracted/manifest.json` — TSDB export metadata, task_name enumeration (12 tasks), row count (42,727)
- `/Users/reh3376/mdemg/training_data/openai_ft/20260420/manifest.json` — FT-OAI subsample shape (2,000/400 rows), empty `task_breakdown`
- `/Users/reh3376/mdemg/training_data/curated/sft_interactions/filtered.jsonl` — full row count (38,412), per-task breakdown via `task_name` column
- `/Users/reh3376/mdemg/training_data/curated/sft_interactions/converted.jsonl` — schema (only `messages`), confirmation that `task_name` is dropped
- `/Users/reh3376/mdemg/docs/development/ft-lora/04_BENCHMARK_RL_v2.md:47-49,295-305` — T/C/J family task lists
- `/Users/reh3376/mdemg/docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md:276-310` — Phase 5 cheat-sheet (the expected file paths)
- `python -m mlx_lm.lora --help` — `--data` directory-with-`{train,valid,test}.jsonl` semantics
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_c_d_profile_results.md` — Sprint D donor-backfill disclosure pattern
- `/Users/reh3376/mdemg/docs/development/ft-lora/sprint_plan_ft_lora_d.md` — Sprint D 99/320 donor-backfill scope precedent

**Referenced but not re-read** (relied on Sprint E commit context):
- `05_DATA_COLLECTION_v2.md` Appendix A balanced-sampling spec
- `01_RESEARCH_v2.md §5` MoE-Sieve two-tier strategy
- Memo 07 v3.1 §3.2 500–1000 rows/task target

---

**End of pre-flight.** User review of this report recommended before FT-LORA-DATA is drafted.
