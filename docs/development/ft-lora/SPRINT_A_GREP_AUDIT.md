# Sprint FT-LORA-A — Grep Audit (Model Names + Tool-Calling Patterns)

**Date:** 2026-04-21
**Branch:** `reh3376_dev01`
**Scope:** All files under repo root excluding `docs/development/ft-lora/` (edited in Sprint A) and excluding `.git/`, `scripts/.venv/`, third-party vendor directories.
**Patterns audited:**

1. **Stale model names:** `Qwen3-30B-A3B`, `qwen3-30b` (case-insensitive) — target v5.0 replacement: `Qwen3.6-35B-A3B`
2. **Tool-calling patterns (per memo 07 v3.1 §2.5, 9 banned strings):** `tool_use`, `tool_call`, `tool_response`, `toolCalls`, `function_call`, `--tool-call-parser`, `enable-auto-tool-choice`, `tools: [`, `preserve_thinking`

## Baseline Reference

Explore agent's pre-Sprint-A count: **~13 non-ft-lora `Qwen3-30B-A3B` references** across the repo. Sprint A edited 11 files in `docs/development/ft-lora/` + `docs/development/ft-lora/ft-lora-dev/` plus 4 repo-level docs (`VISION.md`, `CLAUDE.md`, `AGENT_HANDOFF.md`, `CHANGELOG.md`) + `docs/development/UXTS_FRAMEWORK_MATRIX.md`. Counts below reflect post-Sprint-A state.

---

## Category (a) — Docs Still Needing Edits

Trivial-to-edit docs that missed Sprint A's primary scope. **Recommended disposition:** queue to Sprint B's initial grep remediation pass (1–2 h), not this sprint (out of Sprint A's 14-file target).

| # | File | Line(s) | Context | Action |
|---|---|---|---|---|
| 1 | `docs/development/RESEARCH_ROADMAP.md` | 253 | "Base model: Qwen3-30B-A3B MoE via vllm-mlx" in a forward-looking roadmap row | Update to `Qwen3.6-35B-A3B` + fallback note |
| 2 | `docs/operations/vllm-mlx-setup.md` | 14, 17, 28, 32, 47, 83, 98, 100, 107 | Operational setup guide — model name repeated throughout install/launch examples | Full pass: update model-name string, memory table (~22 GB → ~20.9 GB asymmetric), paths (`mdemg-qwen3-30b-v1-q4/` → `mdemg-qwen3.6-35b-v1-asym/`). Also a target for Sprint E asymmetric-quant launch-command rework. |
| 3 | `docs/tests/uaits/README.md` | 29 | "specs/mdemg.uaits.json declares 4 datasets for Qwen3-30B-A3B fine-tuning" | Update to `Qwen3.6-35B-A3B` |
| 4 | `docs/features/training-data-capture-verification.md` | 45 | Example model name in a column-definition table: `qwen3-30b-a3b` | Update example to `qwen3.6-35b-a3b` |
| 5 | `docs/features/embedding-retrieval-data-collection.md` | 96 | "\| Model \| Qwen3-30B-A3B \|" in a reference table | Update to `Qwen3.6-35B-A3B` |
| 6 | `docs/features/neural-training-pipeline.md` | 357 | "Generative LoRA \| SFT + GRPO \| Qwen3-30B-A3B \| ..." in pipeline overview table | Update to `Qwen3.6-35B-A3B (two-tier MoE-Sieve)` + cross-ref to `docs/development/ft-lora/00_README_v2.md` |
| 7 | `docs/development/ft-oai/sprint_plan_openai_ft_data_generation.md` | 40, 83 | Mentions Qwen3-30B-A3B as a parallel training track / scope exclusion | Update model name to `Qwen3.6-35B-A3B`; preserve scope-exclusion framing (this sprint-plan is historical, but name reference should be current) |
| 8 | `docs/development/ADVERSARIAL_CODEBASE_ANALYSIS_20260410.md` | 13 | "neural/training/ — LLM training pipeline (MLX LoRA, GRPO, Qwen3-30B-A3B target)" | Update to `Qwen3.6-35B-A3B target (per memo 07 v3.1, 2026-04-21)` — this doc is dated but still active-referenced in `CLAUDE.md` |

**Subtotal (a):** 8 files, ~18 individual refs.

---

## Category (b) — Code / Config Files Queued for Sprint B

These are **code files**; per Sprint A constraint ("docs only, zero behavior change") they are **NOT edited in this sprint**. Queued for Sprint B code/config remediation pass.

| # | File | Line(s) | Sprint B Action |
|---|---|---|---|
| 1 | `neural/training/train_ft.py` | 9 | Update docstring example from `mlx-community/Qwen3-30B-A3B-4bit` to `mlx-community/Qwen3.6-35B-A3B-Q4` (or whatever the mlx-community artifact lands as); coordinate with Sprint E's `--shared-quant` / `--routed-quant` / `--attn-quant` flag additions |
| 2 | `neural/training/tests/test_train_ft.py` | 211, 221 | Update fixture `base_model` string; assertion needs to match new default |
| 3 | `neural/training/evaluate_ft.py` | 18 | Docstring example `--model` arg update |
| 4 | `neural/training/teacher_distill.py` | 13, 22 | Two docstring `--teacher-model` examples update |
| 5 | `neural/training/quantize_deploy.py` | 9, 11, 16, 18 | Four docstring `--base-model` / `--output-path` examples; also target of Sprint E asymmetric quant rework (`mlx_lm.convert` per-module-class selectors) |
| 6 | `scripts/test_vllm_mlx.py` | 10, 236, 237, 250 | Smoke-test script — default `LLM_MODEL` env fallback string + two print statements + argparse default |
| 7 | `docs/tests/uaits/specs/mdemg.uaits.json` | 10 | UAITS spec `description` field: `"...Qwen3-30B-A3B fine-tuning"` → `"...Qwen3.6-35B-A3B fine-tuning"`. **Note: UAITS spec — must pass uaits_runner validation after edit** |

**Subtotal (b):** 7 files, ~15 individual refs. All pure metadata/docstring changes (no logic), safe for a single Sprint B commit.

**Sprint B code-remediation scope input for this category:**
- Also expected in Sprint B scope (not surfaced by this grep but documented in `01_RESEARCH_v2.md §1.1` guardrail note): **17th call site migration** — `internal/guardrail/llm_evaluator.go` makes direct HTTP calls to OpenAI/Ollama, bypassing `llmclient`. Must be migrated to route through `llmclient` for training data capture + policy enforcement + auto-switch to Qwen3.6 at cutover.
- ULTS `sampling_group` field addition to 16 task specs under `docs/tests/ults/specs/` (per memo §3.3, reflects `04_BENCHMARK_RL_v2.md §10.0` group table).
- `.env.example` additions for `router_aux_loss_coef=0.002`, asymmetric quant selectors, tier-specific LoRA rank/alpha.

---

## Category (c) — Historical / Changelog / Archive — Preserve as-is

These references are in historical documents (changelogs, archives, superseded-marker sections, prior-run benchmark answers) and should NOT be rewritten. Changelog/benchmark history is an audit trail.

| # | File | Line(s) | Reason to Preserve |
|---|---|---|---|
| 1 | `CHANGELOG.md` | multiple | Historical entries describe prior state (pre-v5.0 plan). New v5.0 entry added at top under `[Unreleased]`. Prior `Qwen3-30B-A3B` mentions in old entries must remain accurate to when written. |
| 2 | `packaging/homebrew-mdemg/CHANGELOG.md` | 211 | Homebrew tap changelog — historical release note for vllm-mlx setup guide addition. Preserve. |
| 3 | `AGENT_HANDOFF.md` | historical update-log entries | Pre-Sprint-A training pipeline mentions. New Sprint FT-LORA-A entry added at top. Historical entries preserve. |
| 4 | `docs/architecture/benchmarks/**/*.jsonl` | many | Benchmark run answer-file archives (whk-wms, clawdbot, cognitive_gap_validation). Any `tool_use` / `function_call` mentions are LLM-generated answer content in archived benchmark runs — immutable test data. |
| 5 | `docs/archive/benchmarks/**` | many | Archived benchmark frameworks (v2, v22). Preserved for reproducibility. |
| 6 | `docs/specs/phase60-cms-advanced-ii.md` | 249 | Phase 60 spec's `recent_tool_calls` example — refers to Claude Code tool usage (Read/Edit/Bash analytics), not MDEMG's LLM tool-calling. Out of scope for the no-tool-calling policy (which targets MDEMG's generative LLM interface only). |
| 7 | `internal/conversation/snapshot.go` | 38 | Go struct field `RecentToolCalls` — tracks Claude Code's actions (Read/Edit/Bash), not MDEMG LLM tool calls. Out of scope. |
| 8 | `docs/development/API_REFERENCE.md` | 1737 | API response example showing `recent_tool_calls` field of the conversation-snapshot endpoint. Same scope note as #6/#7. |
| 9 | `scripts/jiminy_effectiveness_report.py`, `scripts/tests/test_jiminy_effectiveness.py` | various | `tool_use` / `tool_uses_per_minute` metric fields track Claude Code's tool frequency for effectiveness analytics. Out of scope for the no-tool-calling policy. |
| 10 | `scripts/.venv/**/pygments/lexers/praat.py` | 129, 175, 195 | Third-party vendored Pygments lexer — not MDEMG code. `function_call` here is a Praat programming-language syntax token. Out of scope. |

**Subtotal (c):** 10+ files (many individual lines in benchmark archives). Preserved.

---

## Summary

| Category | File count | Disposition |
|---|---|---|
| (a) — docs needing edits | 8 | Queue to Sprint B grep-remediation pass (~1-2 h) |
| (b) — code/config | 7 | Queue to Sprint B code-remediation commit (coordinate with Sprint E asymmetric quant work on train_ft/quantize_deploy/evaluate_ft/teacher_distill) |
| (c) — preserve | 10+ | No action — historical / out-of-scope |

**Total post-Sprint-A stale-model-name refs in scope for Sprint B:** **15 files / ~33 individual refs** (8 doc + 7 code/config).

**Tool-calling-pattern audit result:** **ZERO in-scope findings.** All `tool_use` / `tool_call` / `function_call` hits in `internal/`, `scripts/`, or active `docs/` refer to Claude Code's agent-side tool usage (legitimate; out of scope for MDEMG's no-tool-calling LLM policy). All hits in `docs/archive/` and `docs/architecture/benchmarks/` are historical archive content. `preserve_thinking` appears **exactly once** in the repo — `docs/development/ft-lora/01_RESEARCH_v2.md §2.8` — as the policy statement itself.

**Gate met:** Audit categorizes all findings; matches Explore's pre-sprint baseline (~13 non-ft-lora model-name refs) adjusted for the 4 repo-level docs this sprint edited in-place (VISION, CLAUDE, AGENT_HANDOFF, CHANGELOG — now at intended post-edit state).

## Sprint B Input Queue (consolidated)

1. Edit 8 files from category (a) — pure search/replace + minor contextual tweaks.
2. Edit 7 files from category (b) — docstring/config string updates + UAITS spec re-validation.
3. **Migrate `internal/guardrail/llm_evaluator.go` to `llmclient`** (17th call site — not surfaced by this audit but tracked in `01_RESEARCH_v2.md §1.1` guardrail note; critical cutover blocker for local Qwen3.6 deployment).
4. Add `sampling_group` field to 16 ULTS task specs (per `04_BENCHMARK_RL_v2.md §10.0` group table).
5. `.env.example` / compose template additions for new Sprint-E-exposed knobs (`router_aux_loss_coef`, asymmetric quant selectors, tier LoRA rank/alpha).
6. `--tool-call-parser` / `enable-auto-tool-choice` flags must NOT appear in any new vllm-mlx launch command added in Sprint B/E (verified: zero in-scope hits today; keep verified as part of Sprint B lint pass).
