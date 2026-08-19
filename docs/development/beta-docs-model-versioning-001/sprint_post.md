# BETA-DOCS-MODEL-VERSIONING-001 — Sprint Post

**Shipped**: 2026-08-19 (doc-only)
**Operator directive**: "the beta tester documentation will need to reflect the version implications of which LLM adapters and model gets used as default on install and how to upgrade to the new model and adapters once they are completed."

## What shipped

1. **`packaging/homebrew-mdemg/README.md`** — new "Model versioning + upgrade path" subsection inserted after the existing "Optional: Pull the local LLM (`mdemg-llm-v1`)" block. Covers:
   - **Current default `mdemg-llm-v1`**: Qwen3-14B-4bit base, Phase 5 SFT, GGUF Q5_K_M via llama-server:8102, aggregate 0.9188 on the 16-task augmented eval.
   - **Coming `mdemg-llm-v2`** (not shipped): 27B Qwen base per task #91 + retrained on Phase E1/E2 stripped corpus; distributes via same Ollama Library channel (`reh3376/mdemg-llm-v2`); v1 stays fetchable indefinitely.
   - **Upgrade command sequence** — the ACTUAL shipped mechanism: `mdemg model pull --name mdemg-llm-v2` → edit `.env` `MDEMG_MODEL_PATH` → `launchctl kickstart -k gui/$UID/com.mdemg.llama-server` → verify via `curl /v1/models | jq .data[0].id`. Note that a `mdemg model use <name>` shorthand MAY ship as part of task #134; env-var path continues to work regardless.
   - **Rollback**: revert `.env` `MDEMG_MODEL_PATH` to v1 path; v1 GGUF stays on-disk unless explicitly removed via `mdemg model remove --name mdemg-llm-v1 --yes`.
   - **Which version is running**: `curl /v1/models | jq .data[0].id` (llama-server-side) + `mdemg model list` (local-symlink-side).
2. **`packaging/homebrew-mdemg/README_BETA.md`** — one-line pointer under "Upgrades + versions" section linking to the new README subsection.
3. **CLAUDE.md** — new architecture note pinning a rule: verify command syntax against `internal/cli/*.go` before documenting; don't invent syntax based on operator description.
4. **CHANGELOG** — Unreleased entry.

## Verification (applied `must-validate-all-claims-before-commit`)

| Claim | Verification | Verdict |
|---|---|---|
| `mdemg model use <name>` does not exist as a shipped command | grep on `newModelCmd` in `internal/cli/model.go`: subcommands are `pull|list|verify|remove|where|run|swap|rollback` — no `use` | ✅ Confirmed; doc describes actual env-var+kickstart flow |
| Current model activation mechanism = `MDEMG_MODEL_PATH` in `.env` + `launchctl kickstart` on `com.mdemg.llama-server` | Read `internal/cli/model.go` lines 344-347: pull command prints exactly this sequence as "Next steps" | ✅ Doc mirrors the printed instructions |
| v1 default = Qwen3-14B-4bit Phase 5 SFT, aggregate 0.9188 | Per CLAUDE.md "MDEMG Fine-Tuning — Shipped State & Policies" pin | ✅ |
| v2 scope per operator lock (task #134) = 27B Qwen base, Ollama Library, version-tag both | Per task #134 description + operator's Q&A answers (2026-08-19 this session) | ✅ |
| Existing shipped Ollama URLs for v1 quants | Read from README.md lines 178-181 (`ollama.com/reh3376/mdemg-llm-v1:Q{4,5,8}_K_M`) | ✅ |
| `docs/features/local-model-distribution.md` link resolves | Verified file exists at that path | ✅ |
| Markdown renders cleanly (no broken anchors) | Anchor `#model-versioning--upgrade-path` matches heading | ✅ |

## Decisions

| Decision | Rationale |
|---|---|
| Document the CURRENT env-var + kickstart activation flow, not the aspirational `mdemg model use` | Operator's original answer described `mdemg model use` as the upgrade command, but that command does NOT exist in shipped code. Documenting the aspirational syntax would give beta testers a broken command. Instead: describe what SHIPS, note the shorthand may land in task #134. Extends `must-validate-all-claims-before-commit` from code to docs. |
| Add subsection to the existing "Optional: Pull the local LLM" block (not a new top-level section) | Model versioning is naturally scoped to the existing model-pull discussion; keeps beta docs cohesive rather than fragmented. |
| Point `README_BETA.md` to the README subsection via one-line pointer, not duplicate content | Single source of truth; `README_BETA.md` is an index / onboarding page. |
| Don't edit `mdemg_beta_testing.md` | Model-pull is Tier 1 T1.6 in the test plan; the plan links to README already. Adding v2 upgrade to a test plan for v1 would create confusion. When v2 ships, a new tier for the upgrade path is appropriate. |
| Don't touch `docs/features/local-model-distribution.md` | It's already comprehensive (357+ lines covering the full technical detail). Beta docs cross-reference it. Editing that feature doc for v2 is task #134's job. |
| Don't touch `docs/releases/v0.11.0-beta.*.md` | Frozen release notes. Beta.4 (or later) release note will document v2 when it ships. |

## Follow-ups (disclosed, deferred)

- **Task #134 HOMEBREW-INSTALLER-QWEN-UPDATE-001** (blocking for E4 promote) — implements v2 as an operator-fetchable model on Ollama Library + updates the homebrew installer + optionally ships `mdemg model use` shorthand. This sprint's docs will be re-verified against actual v2 behavior during task #134.
- **`mdemg_beta_testing.md` v2 upgrade tier** — when v2 ships, add a new test tier "T*.* — Model version upgrade" documenting the fetch + swap + verify sequence, so beta testers exercise it as part of their existing playbook.
- **Release note for v2** — when v2 lands, its `docs/releases/*.md` entry names the aggregate-benchmark result vs v1's 0.9188 baseline and any operator-visible behavior change.

## Arch rules pinned

- **When a beta doc describes an upgrade path involving commands, verify against `internal/cli/*.go` that the commands exist as shipped before documenting**. Do NOT invent syntax based on operator description or aspirational design. If operator's answer references a not-yet-shipped shorthand, document the ACTUAL shipped mechanism AND note the shorthand as a task-tracked follow-up. Extends `must-validate-all-claims-before-commit` from code to operator-instructed docs.

## Documents Accessed

- `packaging/homebrew-mdemg/README.md` (edit target; lines 150-213 read, new subsection inserted after line ~213)
- `packaging/homebrew-mdemg/README_BETA.md` (edit target; "Upgrades + versions" section extended)
- `packaging/homebrew-mdemg/mdemg_beta_testing.md` (read for cross-ref; not edited)
- `docs/features/local-model-distribution.md` (read for verifying cross-ref target exists)
- `internal/cli/model.go` (grep-verified: `newModelSwapCmd`, `newModelRollbackCmd` exist; `newModelUseCmd` does NOT; pull command's "Next steps" printout mirrors doc's upgrade sequence)
- Task #134 description (operator-locked scope from prior turn's answers)
- CLAUDE.md pins (MDEMG Fine-Tuning shipped state, MODEL-DIST-001/002, task #91 MODEL-SWAP-QWEN27B-EVAL, PHASE-E1/E2 arc)
- `docs/development/beta-docs-model-versioning-001/sprint_plan.md`
