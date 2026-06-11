# Sprint Plan — DOC-TRUTH-002: MoE-as-Current Residual Cleanup (modified)

## 1. Header & Metadata
Sprint: DOC-TRUTH-002 · 2026-06-11 · branch `reh3376_dev01` · docs-only ·
effort ~0.5d · risk low. Source: operator-provided prompt
(`PROMPT_doc_truth_002_moe_cleanup.md`), executed **with the five
modifications from the 2026-06-11 verification review** (sub-agent,
file:line-verified).

## 2. Problem Statement
Six files still present the permanently-abandoned MoE target
(`Qwen3.6-35B-A3B`, 2026-04-22 pivot) or the decommissioned vllm-mlx
runtime as *current*. DOC-TRUTH-001 scoped other files; its "0 hits in
living docs" sweep was narrower than its wording. Root cause: pre-pivot
Sprint B's grep-audit *introduced* the MoE name into these files; they
were never re-corrected.

## 3. Scope & Constraints
**In**: (1) `docs/features/neural-training-pipeline.md` — Generative LoRA
row :357 + vllm-mlx Serve refs :376/:381 (review mod #3);
(2) `docs/tests/uaits/specs/mdemg.uaits.json` description;
(3) `docs/tests/uaits/README.md:29` (review mod #2 — restates the spec
description); (4) `docs/development/ft-oai/sprint_plan_openai_ft_data_generation.md`
:40/:83; (5) `docs/features/embedding-retrieval-data-collection.md:96`
**column 2 only** (Generative LoRA model — review mod #1: the prompt
misread the table; column 3 is Phase D's embedding placeholder and stays);
(6) `docs/operations/vllm-mlx-setup.md` — superseded banner only (review
mod #2; AGENT_HANDOFF pattern, no rewrite).
**Out**: the ~50 deliberately-historical references (R-LT-4 keep-history);
sprint-plan archives' internal contracts beyond the two flagged lines.
**Rejected from the prompt**: "EMBED-WIRE-001 wired an OpenAI embedder by
default" (false — default is Ollama `qwen3-embedding:8b`); its UAITS
validation instruction (cited checkers don't cover uaits — review mod #4).

## 4. Dependencies
Canonical facts: CLAUDE.md FT section + `ft-lora/00_README_v2.md` v5.12
(dense `mlx-community/Qwen3-14B-4bit` → `mdemg-llm-v1`, GGUF Q5_K_M via
llama-server :8102, 0.8389 aggregate).

## 5. Implementation Plan
Epic 0 this plan · Epic 1 the six file edits · Epic 2 verification
(tiers below) · Epic 3 CHANGELOG + post + push (auto-PR).

## 6. Testing Plan
Tier 1: n/a (no code). Tier 2: pre/post grep sweep — the six targeted
"as-current" sites gone; historical refs untouched (count unchanged
elsewhere). Tier 3 (live): `mdemg data validate --spec
docs/tests/uaits/specs/mdemg.uaits.json` against the real binary (review
mod #4) + UxTS drift checker still green (uaits spec count unchanged).

## 7. Commit Strategy
One edit commit + docs-close commit; push once (auto-PR).

## 8. Verification Checklist
- [ ] All six sites corrected; column-3 of the embedding table untouched
- [ ] vllm-mlx-setup.md carries a superseded banner, content preserved
- [ ] `mdemg data validate` passes live on the edited uaits spec
- [ ] Grep sweep: no remaining as-current `Qwen3.6-35B-A3B` in features/,
      tests/uaits/, operations/ (development/ archives exempt)
- [ ] Drift checker green; CHANGELOG + post.md

## 9. Documentation Update — this sprint IS documentation.

## 10. Risks & Mitigations
Low; worst case is over-correcting a historical reference — mitigated by
the explicit six-site scope and the keep-history rule.

## 11. Documents Accessed
Operator prompt; verification review (sub-agent 2026-06-11);
`00_README_v2.md`; CLAUDE.md FT section; the six target files;
`internal/cli/data_validate.go` (validator truth).

## 12. Rollback Procedures
Docs-only — revert commits.
