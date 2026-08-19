# BETA-DOCS-MODEL-VERSIONING-001 — Sprint Plan

**Arc**: JIMINY-SUBSTRATE-NATIVE-001 Phase E-adjacent (beta docs)
**Ship state target**: doc-only, no code changes

## 1. Header & Metadata

- **Sprint ID**: `BETA-DOCS-MODEL-VERSIONING-001`
- **Author**: reh3376 / claude
- **Date**: 2026-08-19
- **Branch**: `reh3376_dev01`
- **Estimated wall-clock**: ~1 hour (all docs)
- **Sprint format**: v1.0 (12-section, brief)

## 2. Problem Statement

Operator directive 2026-08-19: **"the beta tester documentation will need to reflect the version implications of which LLM adapters and model gets used as default on install and how to upgrade to the new model and adapters once they are completed."**

Current beta docs (`packaging/homebrew-mdemg/README.md` lines 160-213) document `mdemg model pull` fetching `mdemg-llm-v1` from Ollama Library, without explaining:
- What "v1" means as a versioned production model
- That a `v2` is coming (per operator-locked scope of task #134 HOMEBREW-INSTALLER-QWEN-UPDATE-001: new 27B Qwen base + E3-retrained adapter on stripped corpus)
- The upgrade path (what command sequence to run when v2 ships)
- The rollback path (v1 stays fetchable indefinitely)

This sprint adds a **Model versioning + upgrade path** subsection to the shipped beta docs, honest about the current activation mechanism (env var + llama-server restart — there is no `mdemg model use` command today; the operator's earlier answer describing `mdemg model use` was aspirational for task #134's deliverable).

## 3. Scope & Constraints

### In scope
1. Add "Model versioning + upgrade path" subsection to `packaging/homebrew-mdemg/README.md` under the existing "Optional: Pull the local LLM" block.
2. Add brief pointer in `packaging/homebrew-mdemg/README_BETA.md` "Upgrades + versions" section.
3. Update CLAUDE.md architecture note (brief cross-ref).
4. CHANGELOG Unreleased entry (brief).

### Out of scope
- Actually shipping v2 (that's E3+E4)
- Building `mdemg model use` command (that's part of task #134)
- Changing runtime code (no Go changes)
- Updating `docs/features/local-model-distribution.md` (already comprehensive; contains the technical detail — beta docs cross-ref it)
- Updating `docs/releases/v0.11.0-beta.*.md` (frozen release notes)
- Updating `mdemg_beta_testing.md` beyond a brief pointer

### Hard invariants
- **Honest about what exists today**: describe env-var + kickstart activation flow that ships; don't invent `mdemg model use` command.
- **Honest about what's coming**: name `mdemg-llm-v2` as a FUTURE artifact; do NOT claim it ships today; reference task #134 + E3/E4 for tracking.
- **Preserve v1 rollback story**: v1 stays fetchable indefinitely per operator's backwards-compat decision.
- **Zero code touched**: no .go, .py, .js, .env, etc.

## 4. Dependencies

**Upstream (must be shipped)**:
- ✅ MODEL-DIST-001/002 (mdemg model pull shipped, Ollama Library integration)
- ✅ Operator-locked scope for task #134 (2026-08-19: 27B base, Ollama channel, version-tag both)

**Downstream (this sprint unblocks)**:
- Task #134 HOMEBREW-INSTALLER-QWEN-UPDATE-001 can now reference this doc for the operator-facing upgrade contract when it ships v2.

## 5. Implementation Plan

### Epic 1: README.md model-version subsection (~30min)
- Insert after the existing "Optional: Pull the local LLM" section
- Subsection title: "Model versioning + upgrade path"
- Content: what "v1" means, what v2 will bring (27B base + E3 retrain), the upgrade command sequence (`mdemg model pull --name mdemg-llm-v2 ... && edit .env MDEMG_MODEL_PATH && launchctl kickstart`), rollback (revert .env), forwards-compat tracking (task #134).

### Epic 2: README_BETA.md + CLAUDE.md + CHANGELOG (~15min)
- README_BETA.md — one-line pointer under "Upgrades + versions"
- CLAUDE.md — brief arch note under the Phase E2 pin, cross-ref this sprint
- CHANGELOG — Unreleased entry

### Epic 3: Sprint post (~15min)
- `docs/development/beta-docs-model-versioning-001/sprint_post.md` documenting what was added + operator's original directive verbatim + verification (rendered file check)

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit
N/A (doc-only sprint)

### Tier 2 — Integration
- Markdown renders cleanly (no broken links to shipped anchors like `docs/features/local-model-distribution.md`)
- No stray whitespace / trailing newlines

### Tier 3 — Live end-to-end
- View the rendered `README.md` and `README_BETA.md` in an editor (or `grip` if available); verify new section reads clearly + links resolve
- N/A for the substrate (no runtime touch)

## 7. Commit Strategy

- 1 primary commit: README + README_BETA + CLAUDE.md + CHANGELOG + sprint dir
- No follow-up commits expected

## 8. Verification Checklist

- [ ] `packaging/homebrew-mdemg/README.md` has "Model versioning + upgrade path" subsection
- [ ] Explains v1 default, v2 coming (name + base + tracking task)
- [ ] Documents the ACTUAL activation mechanism (env var + kickstart, NOT invented `mdemg model use`)
- [ ] Rollback path is clear
- [ ] `packaging/homebrew-mdemg/README_BETA.md` has one-line pointer under "Upgrades + versions"
- [ ] CLAUDE.md architecture note added
- [ ] CHANGELOG entry
- [ ] Sprint plan + post in `docs/development/beta-docs-model-versioning-001/`
- [ ] PR sprint-summary comment posted

## 9. Documentation Update

### Files modified
- `packaging/homebrew-mdemg/README.md` — new subsection
- `packaging/homebrew-mdemg/README_BETA.md` — one-line pointer
- `CLAUDE.md`, `CHANGELOG.md`

### Files created
- `docs/development/beta-docs-model-versioning-001/{sprint_plan,sprint_post}.md`

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Doc claims a command that doesn't exist (`mdemg model use`) | Low | Medium | Explicit: describe env-var + kickstart flow (the actual shipped mechanism); note "may ship as part of task #134" |
| Beta testers confused by "v2 coming" if timeline unclear | Medium | Low | Reference task #134 (tracking) + E3/E4 (retrain + promote sequence); do not commit to date |
| Doc drifts from actual v2 shape when v2 ships | Low | Medium | Task #134 sprint will re-check this doc + reconcile |

## 11. Rollback Procedures

- Zero substrate mutation; zero code change.
- Rollback = revert this sprint's commit.

## 12. Documents Accessed

- `packaging/homebrew-mdemg/README.md` (edit target; lines 150-213)
- `packaging/homebrew-mdemg/README_BETA.md` (edit target; "Upgrades + versions" section)
- `packaging/homebrew-mdemg/mdemg_beta_testing.md` (read for cross-ref, not edited)
- `docs/features/local-model-distribution.md` (cross-ref target; comprehensive tech doc)
- `internal/cli/model.go` (verified: `newModelSwapCmd` + `newModelRollbackCmd` exist; `newModelUseCmd` does NOT — hence honest doc about env-var + kickstart flow)
- Task #134 description (operator-locked scope)
- CLAUDE.md pins (PHASE-E2-CORPUS-CURATION-001, MODEL-DIST-001, MODEL-DIST-002, task #91 MODEL-SWAP-QWEN27B-EVAL)

---
