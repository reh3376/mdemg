# Sprint FT-LORA-A — Documentation Update Pass (Qwen3.6-35B-A3B + MoE Two-Tier LoRA + No-Tool-Calling)

**Status:** ✅ Completed 2026-04-21 on branch `reh3376_dev01`. Sprint plan copied from `~/.claude/plans/breezy-dancing-lerdorf.md` at sprint close per Epic 11. Grep audit delivered as `SPRINT_A_GREP_AUDIT.md`. Two planner-introduced engineering policies flagged in commit body + PR summary for audit trail (epoch cap 3 + `val_loss > best × 1.05` / `val_reward < best × 0.95` early-stop; `n_epochs=auto` disallowed).

> **Note on section renumbering:** The plan text below refers to the new no-tool-calling policy section as `01_RESEARCH_v2.md §2.6`. During execution §2.6 was already occupied by "Jiminy Guidance Outcomes as Training Quality Signal" — the new section was inserted as **§2.8 No-Tool-Calling Architectural Policy**. All finished-state cross-references throughout the v5.0 doc suite use `§2.8` consistently. Read "§2.6" in the plan text below as "§2.8" in the final state.

## Context

User selected **Option C**: full Sprint A→E serialization per `/Users/reh3376/Downloads/07_MODEL_UPDATE_AND_MOE_STRATEGY.md` (v3.1 memo, 2026-04-21), with frequent monitoring and explicit overfitting prevention. Phase 5 SFT unblocks only after Sprint C validation passes. FT-OAI-003 calibration run proceeds in parallel (commit `060c20e`, PR #334).

Sprint A is **docs-only**: aligns all planning documents with the memo's three locked-in decisions before any code touches the repo. Zero behavior change, zero spend, ~3 days. This is the lowest-regret move in the A→E chain because it creates the shared vocabulary that Sprints B–E reference.

The three decisions Sprint A propagates:
1. **Base model**: Qwen3-30B-A3B → **Qwen3.6-35B-A3B** (MoE, 35B total / 3B active, 256 experts = 8 routed + 1 shared, Apache 2.0 2026-04-16, 262K native context). Fallback Qwen3.5-35B-A3B — **not** Qwen3-30B-A3B.
2. **No-tool-calling policy** — all 16 MDEMG LLM call sites are single-shot structured-output / reasoning.
3. **Two-tier MoE-Sieve LoRA** — Tier 1 (attention + shared expert, r=32 α=64, all 16 tasks) + Tier 2 (top-25% routed experts, r=8 α=16, per-family: reasoning-think / classify-notink / structured-notink). Load-balancing aux coef=0.002. Asymmetric quant: shared BF16, routed MXFP4_MOE, attention BF16.

**Overfitting-prevention hook** (ties to user's directive): Sprint A docs **must** document the epoch-cap + early-stopping strategy now, so Sprint E builds the instrumentation and Phase 5 SFT follows it by default. The FT-OAI-001 overfitting-at-step-1200 is referenced as the forcing function in `03_IMPLEMENTATION_PLAN` Phase 5 and `04_BENCHMARK_RL`.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-LORA-A |
| Title | Documentation Update Pass — Qwen3.6-35B-A3B + MoE Two-Tier LoRA + No-Tool-Calling |
| Date | 2026-04-21 |
| Branch | `reh3376_dev01` |
| Predecessors | Memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1; FT-OAI-003 plan (`060c20e`) |
| Successors | FT-LORA-B (code/config updates, grep audit, sampling param apply) |
| Type | Documentation-only; zero behavior change |
| Risk | Low |
| Budget | $0 (docs only) |

## 2. Problem Statement

The existing `docs/development/ft-lora/` plan suite (v2 docs dated 2026-04-07 to 2026-04-20) targets Qwen3-30B-A3B with a monolithic LoRA approach and does not reflect:
- The Qwen3.6-35B-A3B release (2026-04-16) and its new architecture (Hybrid Gated DeltaNet + Gated Attention + 256 experts + MTP).
- MDEMG's architectural no-tool-calling policy (currently implicit; not documented as a constraint).
- The MoE-aware two-tier LoRA strategy mandated by memo §3.

Without the doc update, Sprints B–E (code, validation, profiling, infra patches) would diverge from the memo's intent. Engineering ambiguity compounds: the grep audit in Sprint B needs a canonical target model name, the sampling-param apply needs the memo's new `presence_penalty=1.5` documented per-task, and the eventual Phase 5 SFT needs the epoch-cap + early-stopping policy specified in advance.

## 3. Scope & Constraints

**In scope (12 files per memo §8 + 2 subdirectory docs + 13 scattered doc refs):**

| # | File | Action |
|---|---|---|
| 1 | `docs/development/ft-lora/00_README_v2.md` | Rewrite Key Decisions table; bump version header |
| 2 | `docs/development/ft-lora/01_RESEARCH_v2.md` | Rewrite §3 (model selection); add §2.6 (no-tool-calling); add §5 (MoE two-tier LoRA) |
| 3 | `docs/development/ft-lora/02_M5MAX_HARDWARE_v2.md` | Rewrite memory budget, throughput targets, quantization table |
| 4 | `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md` | Rewrite Phase 3 + Phase 5; add new Phase 5.X (expert activation profiling); add epoch-cap + early-stopping policy |
| 5 | `docs/development/ft-lora/04_BENCHMARK_RL_v2.md` | Update sampling params (incl. `presence_penalty=1.5`); update GRPO router-entropy handling; document val-loss-divergence early-stop trigger |
| 6 | `docs/development/ft-lora/05_DATA_COLLECTION_v2.md` | Append: balanced-sampling note, routing-profile artifact note |
| 7 | `docs/development/ft-lora/06_CORRECTIONS_APPLIED_v2.md` | Append v3.1 section (supersession record) |
| 8 | `VISION.md` | Add model-name + no-tool-calling policy refs |
| 9 | `CLAUDE.md` | Add no-tool-calling policy + MoE-strategy summary section |
| 10 | `AGENT_HANDOFF.md` | Append current-state update (post-memo direction) |
| 11 | `CHANGELOG.md` | Queue unreleased entry |
| 12 | `docs/development/UXTS_FRAMEWORK_MATRIX.md` | Review for tool-calling assumptions; annotate if any |
| 13 | `docs/development/ft-lora/ft-lora-dev/MDEMG_FT_PLAN_DEEP_DIVE_ANALYSIS_v2.md` | Model-name refresh; annotate MoE-strategy divergence |
| 14 | `docs/development/ft-lora/ft-lora-dev/SPRINT_EMBEDDING_DATA_COLLECTION_v2.md` | Model-name refresh only |

**Out of scope:**
- Code changes (Sprint B).
- Grep audit remediation of the 13 non-ft-lora docs with stale model names — scoped as Epic 10 below: **identify & queue** only, apply in Sprint B.
- `.env.example`, compose templates, Python training modules (Sprint B).
- `mlx_lm.convert` patch, `router_aux_loss_coef` exposure (Sprint E).
- Profiling script implementation (Sprint D; Sprint A only documents the intent).
- Qwen3.6 MLX validation runs (Sprint C).

**Constraints:**
- **Edit in place** on `_v2.md` files; bump internal version header (00_README says `Version: 4.0` → `5.0`) with a "Changed per memo 07 v3.1 (2026-04-21)" banner. Avoid v3 file proliferation.
- **Preserve historical text** in AGENT_HANDOFF.md and 06_CORRECTIONS_APPLIED.md — append, do not overwrite.
- **No new facts** beyond what the memo states. Where the memo is silent (e.g. exact SFT epoch numbers), use the memo's defaults (3 epochs same as Tier 2 per §6.1 open question) and flag as open question.
- Sequential epics — docs before downstream, no parallelization (per `feedback_sequential_epics.md`).
- Mid-sprint gate between Epic 7 (ft-lora docs done) and Epic 8 (repo-level docs). User review checkpoint.

## 4. Dependencies

- Read-only reference: `/Users/reh3376/Downloads/07_MODEL_UPDATE_AND_MOE_STRATEGY.md` (v3.1 memo)
- Commit `060c20e` (FT-OAI-003 reframe) — already pushed; this sprint does not touch it
- No new external deps; markdown only

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** confirm branch state clean; verify `060c20e` pushed and PR #334 open; grep `Qwen3-30B-A3B` across repo to confirm Explore's count (25 files).

### Epic 1 — `01_RESEARCH_v2.md` core rewrite
Rewrite §3 (model selection) to target Qwen3.6-35B-A3B with Qwen3.5-35B-A3B fallback. **Re-audit §1.1 "16 generative call sites" table against the current codebase state — update task-type labels (think / no-think, structured / classification / detection). This table is the factual foundation the family partition in §5 rests on; re-verifying it is the most important pre-requisite for any downstream family-assignment work.** Add §2.6 **"No-Tool-Calling Architectural Policy"** with justification (16 LLM sites audit, single-shot reasoning/structured-output pattern); include explicit sentence: _"The Qwen3.6 `preserve_thinking` parameter is documented for multi-turn agent loops. MDEMG does not use this feature. `preserve_thinking` must remain at its default in all inference configurations."_ Add §5 **"MoE Two-Tier LoRA Strategy"** covering Tier 1 / Tier 2 partitioning, family definitions, load-balancing aux coef=0.002, asymmetric quantization.
**Gate:**
- File lints clean; zero refs to `Qwen3-30B-A3B`
- §1.1 16-task roster matches current codebase (grep-verified against LLM call sites in `internal/` / `neural/`); task-type labels updated
- §2.6 and §5 are coherent sections with internal cross-refs
- §2.6 includes the `preserve_thinking` sentence verbatim
- **§5 includes provisional-partition note (required):** _"Family partition (reasoning-think / classify-notink / structured-notink) is a **starting hypothesis**. Sprint D expert activation profiling will validate or revise it. Decision criteria: if cross-family expert overlap exceeds 80%, partition will be merged; if any family shows bimodal routing, it will be split."_

### Epic 2 — `02_M5MAX_HARDWARE_v2.md` memory/throughput refresh
Rewrite memory table for Qwen3.6-35B-A3B Q4 (~20.9GB), shared-expert BF16 path, routed-expert MXFP4_MOE path. Update throughput targets to ≥60 tok/s (memo Sprint C gate 3). Update "Tool-use exclusion" paragraph to reference §2.6 in `01_RESEARCH`.
**Gate:** Numbers trace to memo §1.2 / §3.8; hardware claims pinned to M5 Max.

### Epic 3 — `03_IMPLEMENTATION_PLAN_v2.md` Phase 5 + 5.X rewrite
Rewrite Phase 5 to describe two-tier SFT: Tier 1 first (all tasks balanced), then per-family Tier 2. Add new **Phase 5.X "Expert Activation Profiling"** that Sprint D will implement (script `neural/training/profile_expert_routing.py`, outputs `profile_routing_{family}.json` + heatmap).

**⚠️ Planner-introduced policies (NEW — not in memo; flagged for user sign-off via commit message + PR summary):**

1. **Epoch cap + early-stop threshold** (closes memo §6.1 open question):
   - Default max 3 epochs (same as memo Tier 2 default).
   - Early-stop trigger: `val_loss > best_val_loss * 1.05` for 2 consecutive evals.
   - **Rationale for 1.05×/2-evals**: FT-OAI-001 crossed the 1.05× threshold between step 1250 and 1300 (val 0.684 → 0.792 = +16%), 2 evals past best. The 5% band tolerates expected noise; 2-eval patience avoids single-eval transient trips. If user prefers tighter (1.03×/1 eval = earlier stop, more false positives) or looser (1.10×/3 evals = later stop, more overfit risk), adjust here before Sprint C validation.
2. **`n_epochs=auto` is not allowed.** Explicit cap required on every LoRA run. FT-OAI-001's 3-epoch auto-inflation is the forcing function.

Both policies reference FT-OAI-001 `run_notes.md` step-1200 overfit finding.

**Gate:** Phase 5 and 5.X are sequential; early-stop policy is explicit with rationale; `n_epochs=auto` ban is explicit; FT-OAI-001 reference links to `training_data/openai_ft/20260420/run_notes.md`; commit message + PR summary flag these as **new policies introduced in Sprint A**.

### Epic 4 — `04_BENCHMARK_RL_v2.md` sampling + routing-entropy update
Apply memo §3.3's **three group recipes** (think-mode / no-think classify / no-think JSON) and document the **task → group mapping for all 16 tasks**. The memo does NOT specify per-task unique tuning; this sprint does NOT introduce per-task deviations (those would be new decisions). Document **`presence_penalty=1.5`** on the 9 structured-JSON tasks explicitly (critical new param, applies at group level). Add "Router entropy monitoring" subsection for GRPO (memo §3.6) with layer-level entropy thresholds. Document val-loss divergence as an RL-phase early-stop trigger (same policy as Phase 5 SFT, ties to overfitting-prevention directive).

**Gate:** Sampling table shows all 16 tasks correctly assigned to one of the three memo §3.3 groups; each task row shows temp / top_p / top_k / presence_penalty / max_tokens inherited from its group; no task left on old defaults. **Gate criterion is "all 16 tasks correctly grouped" — NOT "all 16 tasks with unique per-task tuning".**

### Epic 5 — `05_DATA_COLLECTION_v2.md` appendix additions
Append two short subsections: (a) "Balanced sampling for Tier 1 training" (all 16 tasks weighted equally), (b) "Routing profile artifact" (expected `profile_routing_{family}.json` format, where it lives in the data pipeline output).
**Gate:** Additions are < 2 pages; cross-ref to 03_IMPLEMENTATION_PLAN Phase 5.X.

### Epic 6 — `06_CORRECTIONS_APPLIED_v2.md` v3.1 section
Append new section "v3.1 — Model upgrade + MoE-aware LoRA (2026-04-21)". Summarize the three decisions with links to each. Add supersession note for Qwen3-30B-A3B mentions in prior sections (do not rewrite prior entries — mark them superseded).
**Gate:** Section header matches pattern of existing v1.0→v2.0→v3.0 entries.

### Epic 7 — `00_README_v2.md` Key Decisions + version bump
Rewrite Key Decisions table for Qwen3.6-35B-A3B + two-tier LoRA + no-tool-calling. Bump internal version 4.0 → 5.0. Add "Changes in v5.0 (per memo 07 v3.1 — 2026-04-21)" banner at top listing the three decisions with file pointers.
**Gate:** Table is the canonical summary; all four key decisions traceable to memo §§1, 2, 3.

🔸 **MID-SPRINT USER CHECKPOINT** 🔸

Pause for user review of all ft-lora/ doc edits (Epics 1–7). Confirm alignment before propagating to repo-level docs. User either approves to continue or requests revisions.

### Epic 8 — Repo-level doc updates
- `VISION.md`: add one-paragraph mention of Qwen3.6-35B-A3B target and no-tool-calling policy under the relevant section (identify section during execution).
- `CLAUDE.md`: add new section "MDEMG Fine-Tuning Target & Policies (Sprint FT-LORA onwards)" summarizing the three decisions with links to `docs/development/ft-lora/00_README_v2.md`.
- `AGENT_HANDOFF.md`: append to current-state section — agent handoff now tracks Sprint A→E chain; model target is Qwen3.6-35B-A3B.
- `CHANGELOG.md`: queue `[Unreleased]` entry: `docs(ft-lora): align to memo 07 v3.1 — Qwen3.6-35B-A3B, two-tier MoE LoRA, no-tool-calling policy`.
- `docs/development/UXTS_FRAMEWORK_MATRIX.md`: review for tool-calling assumptions; annotate "No tool-calling — see `01_RESEARCH_v2.md` §2.6" if applicable.

**Gate:** Each file change is minimal (paragraph or section, not rewrite); all pointers to `01_RESEARCH_v2.md` §2.6 / §5 resolve.

### Epic 9 — `ft-lora-dev/` subdirectory
Refresh model-name refs in `MDEMG_FT_PLAN_DEEP_DIVE_ANALYSIS_v2.md` and `SPRINT_EMBEDDING_DATA_COLLECTION_v2.md`. Add header note "Superseded in part — see `../01_RESEARCH_v2.md` §5 for current MoE strategy" on the deep-dive doc.
**Gate:** No remaining `Qwen3-30B-A3B` in these 2 files.

### Epic 10 — Grep audit + Sprint B queue
Run grep across repo for the following patterns:
- Model names: `Qwen3-30B-A3B`, `qwen3-30b` (case-insensitive)
- Tool-calling patterns (memo §2.5): `tool_use`, `tool_call`, `tool_response`, `toolCalls`, `function_call`, `--tool-call-parser`, `enable-auto-tool-choice`, `tools: [`, **`preserve_thinking`**

Scope: `docs/` (excluding ft-lora which this sprint just edited), `neural/`, `scripts/`, `packaging/`, `.github/`, repo root.

Produce `docs/development/ft-lora/SPRINT_A_GREP_AUDIT.md` with categorized findings:
- (a) docs still needing edits — fix in this sprint if trivial, else queue to Sprint B
- (b) code / config files queued for Sprint B
- (c) historical / changelog entries preserved as-is

**Do not edit code files in this sprint** — just list them.

**Gate:** Audit doc exists, categorizes all findings (a)/(b)/(c). Baseline reference: Explore agent counted ~13 non-ft-lora model-name refs pre-Sprint-A; the audit's (a)+(b)+(c) totals should reflect the post-Sprint-A state (docs count down, code/config roughly stable). The ≥13 figure is the **pre-Sprint-A baseline**, not a post-sprint floor.

### Epic 11 — Documentation Update (final epic — never cut per CLAUDE.md rule)
- Update `docs/development/ft-lora/00_README_v2.md` Document Map if any new file added (e.g. `SPRINT_A_GREP_AUDIT.md`).
- Add this sprint plan to `docs/development/ft-lora/` as `sprint_plan_ft_lora_a.md` (copy from `~/.claude/plans/breezy-dancing-lerdorf.md` on sprint close).
- Cross-reference check: every mention of `01_RESEARCH_v2.md §2.6` or `§5` in other files resolves.
- "Documents Accessed" appendix with every file + line-range consulted during this sprint (per mandatory rule).
**Gate:** Cross-ref grep returns no dangling pointers; sprint plan doc committed to repo.

## 6. Testing Plan (Three Tiers)

### Tier 1 — Static / Lint
- `markdownlint` or `mdl` on all 14 modified files (install via brew if not present) → zero errors
- `grep -rn "Qwen3-30B-A3B" docs/development/ft-lora/` → zero matches (target files only)
- `grep -rn "tool_use\|function_call\|preserve_thinking" docs/development/ft-lora/` → only appears in policy-statement contexts (no accidental endorsement); `preserve_thinking` appears exactly once in `01_RESEARCH_v2.md §2.6`
- All internal links (`01_RESEARCH_v2.md §5`, etc.) resolve — verify with a custom grep / manual review
- `grep -cE "^## [0-9]+" <this-plan>` → 10–12 sections (sprint plan v1.0 format compliance)

### Tier 2 — Structural / Cross-file Consistency
- Sampling params in `04_BENCHMARK_RL_v2.md` §X match memo §3.3 three-group recipes; 16 tasks each have an explicit group assignment
- Model name `Qwen3.6-35B-A3B` appears consistently spelled across all 14 files (no `Qwen3-6-`, `Qwen 3.6 -35B`, etc.)
- `01_RESEARCH_v2.md §1.1` 16-task roster is grep-verifiable against LLM call sites in `internal/` / `neural/` (spot-check at least 3 entries trace to real call sites)
- §5 "MoE Two-Tier LoRA Strategy" includes the provisional-partition note verbatim (80% overlap / bimodal split criteria)
- No-tool-calling policy statement in `CLAUDE.md` matches `VISION.md` statement (same phrasing); `preserve_thinking` sentence present
- `00_README_v2.md` Key Decisions table rows trace 1:1 to `01_RESEARCH_v2.md` §3 (model), §2.6 (no-tool-calling), §5 (MoE LoRA)
- `SPRINT_A_GREP_AUDIT.md` category (b) list = Sprint B's scope input

### Tier 3 — Human Review (User)
- User reads full diff of each ft-lora/ doc at the mid-sprint gate; approves or flags revisions
- User reads full diff of repo-level docs (VISION, CLAUDE, AGENT_HANDOFF, CHANGELOG, UXTS) after Epic 8; approves
- User confirms memo §8 Sprint A checklist items are all visually checkable against this sprint's output

## 7. Commit Strategy

Per user's "batch commits at end" directive observed in prior sprint. Single commit at sprint close:
- Title: `docs(ft-lora): align to memo 07 v3.1 — Qwen3.6-35B-A3B, two-tier MoE LoRA, no-tool-calling`
- Bullets: one per epic (E1–E11), each ≤1 line
- **Dedicated "New engineering policies" section in commit body** (not in title) — flags Epic 3's two planner-introduced policies for the audit trail:
  1. Epoch cap + early-stop threshold (`val_loss > best × 1.05` for 2 consecutive evals, max 3 epochs) — closes memo §6.1 open question
  2. `n_epochs=auto` disallowed on all LoRA runs — explicit cap required
- Footer: `Co-Authored-By: Claude Opus 4 <noreply@anthropic.com>`

**PR summary (auto-PR body) must also call out the two new policies in a "New engineering policies introduced" section** so reviewers see them independent of reading commit messages.

**Mid-sprint checkpoint does NOT commit** — diff stays in working tree until user approves at gate.

Push to `reh3376_dev01`; auto-PR follows. Sprint summary comment posted on the PR (per `feedback_sprint_summary_on_pr.md`).

## 8. Verification Checklist

- [ ] Pre-gate: branch state clean, `060c20e` pushed, PR #334 open, grep-audit baseline captured
- [ ] Epic 1–7: all ft-lora/ files edited; Tier 1 lint green on each; no `Qwen3-30B-A3B` refs
- [ ] Mid-sprint user checkpoint passed
- [ ] Epic 8: repo-level docs updated; pointers resolve
- [ ] Epic 9: ft-lora-dev/ subdir updated
- [ ] Epic 10: `SPRINT_A_GREP_AUDIT.md` exists and categorizes all ≥13 remaining refs
- [ ] Epic 11: sprint plan committed to `docs/development/ft-lora/sprint_plan_ft_lora_a.md`; Documents Accessed appendix complete
- [ ] Tier 1, 2 checks all green
- [ ] Tier 3 user approval at sprint close
- [ ] Memo §8 Sprint A checklist: all 12 items ✅
- [ ] Commit pushed; auto-PR opened; sprint summary comment posted

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 11 above. Key rule: this sprint **is** the documentation update — the sprint plan itself lands in the repo (`sprint_plan_ft_lora_a.md`) as part of closure, matching the `docs/development/ft-oai/` pattern.

## 10. Risks & Mitigations

| Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|
| Scope creep — user asks for code changes during Sprint A | Medium | Mid-sprint gate explicitly says "docs only"; queue code items for Sprint B via Epic 10 audit | Reject in-sprint, add to Sprint B plan |
| Mid-sprint user checkpoint finds factual errors | Low-Medium | Explore agent already verified file states; memo is single source of truth | Revise affected epics; re-run Tier 2 consistency check |
| Markdownlint not installed on dev machine | Low | Use `npx markdownlint-cli` fallback | Hand-review if install fails |
| Stale `Qwen3-30B-A3B` slips through in a doc we didn't catalog | Low | Epic 10 grep audit is the belt-and-braces pass | Append to audit, fix in Sprint B |
| Memo open question §6.1 (shared-expert epochs 3 vs 5) surfaces before we're ready to commit | Low | Document as explicit "Open Question" in `01_RESEARCH_v2.md §5`, do not pretend it's resolved | Keep as open in plan; revisit Sprint C |
| User wants to skip mid-sprint checkpoint and run E1–E11 in one pass | Low | Checkpoint is for user benefit ("frequent monitoring"); if user waives, log decision | Remove checkpoint, commit as single batch |
| §1.1 16-task roster re-audit finds codebase drift (e.g. 17 call sites, or 15) | Medium | Update table to match current state; flag discrepancy from memo's "16" in Epic 1 note; Sprint D partition work adapts to new count | Escalate to user if count change invalidates family partition hypothesis |
| Planner-introduced policies (1.05×/2-eval early-stop; no auto epochs) prove wrong during Sprint C | Medium | Policies are documented as Sprint A additions in commit + PR; revisiting requires an explicit doc revision in Sprint C, not silent change | Revise thresholds in a Sprint C addendum to `03_IMPLEMENTATION_PLAN_v2.md` |

## 11. Documents Accessed

During planning (read-only):
- `/Users/reh3376/Downloads/07_MODEL_UPDATE_AND_MOE_STRATEGY.md` (entire 454 lines)
- `docs/development/ft-lora/00_README_v2.md` (lines 1–30)
- `/Users/reh3376/mdemg/CLAUDE.md` (loaded via system context)
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` (loaded via system context)
- `/Users/reh3376/.claude/plans/breezy-dancing-lerdorf.md` (prior plan state)
- `docs/development/` directory listing
- `docs/development/ft-lora/` directory listing
- `docs/development/ft-lora/ft-lora-dev/` directory listing
- git log (top 5 commits)
- Explore agent report: current state of all 14 target files + grep audit of stale refs repo-wide

## 12. Rollback

Not applicable — docs-only; `git revert <sha>` restores prior state if needed. Never destructive.

---

## Post-Sprint A

Sprint A gate → **Sprint B** (code/config: grep audit remediation, `.env.example`, inference launch commands, sampling-param apply to 16 task configs). Sprint B plan drafted after Sprint A merges.

---

## 13. Documents Accessed (Sprint Execution)

Files read during Sprint A execution (per mandatory "Documents Accessed" rule):

**Source-of-truth memo (read-only):**
- `/Users/reh3376/Downloads/07_MODEL_UPDATE_AND_MOE_STRATEGY.md` — v3.1 memo, 2026-04-21 (454 lines)

**Target files edited in-place (Epics 1–9):**
- `docs/development/ft-lora/00_README_v2.md` — Key Decisions rewrite, v4.0→v5.0 bump
- `docs/development/ft-lora/01_RESEARCH_v2.md` — §1.1 16-task re-audit + §2.8 new + §5 new + diagram + memory-table updates
- `docs/development/ft-lora/02_M5MAX_HARDWARE_v2.md` — full rewrite (asymmetric quant table, Tier 1/Tier 2 memory, vllm-mlx launch)
- `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md` — Phase 1 consumer table, Phase 2C count, Phase 5 rewrite (5A-5F incl. ⚠️ policy)
- `docs/development/ft-lora/04_BENCHMARK_RL_v2.md` — §10.0 sampling recipes, §11.2 think split, §11.2.1 router entropy, §11.6 ⚠️ early-stop
- `docs/development/ft-lora/05_DATA_COLLECTION_v2.md` — Appendix A (balanced sampling) + Appendix B (routing artifact)
- `docs/development/ft-lora/06_CORRECTIONS_APPLIED_v2.md` — v4.0→v5.0 Strategic section (3 issues, 34 total)
- `docs/development/ft-lora/ft-lora-dev/MDEMG_FT_PLAN_DEEP_DIVE_ANALYSIS_v2.md` — supersession header, 3 model-name refreshes
- `docs/development/ft-lora/ft-lora-dev/SPRINT_EMBEDDING_DATA_COLLECTION_v2.md` — 1 model-name refresh

**Repo-level files edited (Epic 8):**
- `VISION.md` — paragraphs added (fine-tuning target + no-tool-calling policy)
- `CLAUDE.md` — new "MDEMG Fine-Tuning Target & Policies" section
- `AGENT_HANDOFF.md` — Sprint FT-LORA-A completion entry at top of update log
- `CHANGELOG.md` — `[Unreleased]` Changed entry
- `docs/development/UXTS_FRAMEWORK_MATRIX.md` — no-tool-calling constraint banner

**New files created (Epics 10–11):**
- `docs/development/ft-lora/SPRINT_A_GREP_AUDIT.md` — (a)/(b)/(c) categorization of post-sprint stale refs
- `docs/development/ft-lora/sprint_plan_ft_lora_a.md` — this file (copy of approved plan + status header + Documents Accessed appendix)

**Read for context / verification (not edited):**
- `/Users/reh3376/.claude/plans/breezy-dancing-lerdorf.md` — approved plan
- `/Users/reh3376/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md` — feedback rules (sequential epics, sprint plan format, batch commits, summary on PR)
- `training_data/openai_ft/20260420/run_notes.md` — FT-OAI-001 step-1200 overfitting evidence (forcing function for ⚠️ planner policies)
- `internal/guardrail/llm_evaluator.go` — confirmation of 17th-call-site bypass (referenced in §1.1 guardrail note)
- Git log top 5 commits; repo-wide grep results for `Qwen3-30B-A3B` / tool-calling patterns / `01_RESEARCH_v2.md §X` cross-references
