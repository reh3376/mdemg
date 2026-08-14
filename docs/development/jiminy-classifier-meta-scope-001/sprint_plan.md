# JIMINY-CLASSIFIER-META-SCOPE-001 — Sprint Plan

**Date:** 2026-08-14 | **Branch:** `reh3376_dev01`
**Parent arc:** JIMINY-CEILING-BREAK-2 — Phase 3.5 (bug-fix follow-up to Phase 3 / JIMINY-CLASSIFIER-CONTEXT-002)
**Sibling pattern:** JIMINY-CLASSIFIER-CONTEXT-002, JIMINY-CLASSIFIER-CONTEXT-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001
**Target commit date:** 2026-08-15 (must land inside the 2026-08-14 → 2026-08-19 arc measurement window so its lift is captured in the same 7d passive re-read as CONTEXT-002; if it slips past 2026-08-16, defer to 2026-08-20 window to preserve attribution).

## 1. Header & Metadata

Extend the tier-2 LLM classifier prompt with a **mention-vs-perform disambiguation clause** — a targeted narrowing of the shipped `mechanismScopeCreditClause` that stops it from over-triggering on doc-edit / prose actions whose action-text happens to contain the mechanism-verb as CONTENT rather than as an EXECUTED action. Effort ~1 day. Risk low: identical architectural shape as the three shipped credit clauses (default-off flag, resolver splice, ULTS byte-identical pin, live Tier-3 smoke gate).

- **Arc adjacency:** In-arc, Phase 3.5 of JIMINY-CEILING-BREAK-2. Not arc-external — the whole reason to ship inside the 2026-08-14 → 2026-08-19 window is that the CONTEXT-002 lift attribution is confounded by this failure mode; fixing it now lets the 2026-08-19 re-check give a clean read on Phase 3 + 3.5 combined.
- **Ordering discipline:** narrower→broader, strongest-gate-last. This clause sits AFTER `mechanismScopeCreditClause` (strengthens/qualifies it) so the LLM's recency-weighted attention gives the mention-vs-perform distinction the last word.

## 2. Problem Statement

`JIMINY-CLASSIFIER-CONTEXT-002` (shipped 2026-08-12) added a HARD-PRECEDENCE `mechanismScopeCreditClause` to `classifySystemPrompt` in `internal/jiminy/outcome_classifier.go` — it instructs the LLM to identify the constraint's mechanism-verb(s) (commit/push, generate-id, ALTER TABLE, etc.), and if the action-text doesn't contain any mechanism-verb OR describe performing it, verdict = `not_applicable` UNCONDITIONALLY.

Discovered failure mode after some usage: the gate OVER-TRIGGERS on a specific class — **editing content that MENTIONS the ruling but doesn't perform the mechanism**. Concrete examples the operator has flagged:

- A file edit that ADDS text mentioning "git commit" as prose (docs, sprint plans, CLAUDE.md, README, help text) — the classifier sees "commit" mechanism-verb in the action-text and DOES NOT route to not_applicable, but the action isn't actually committing.
- A sprint plan file that describes a "schema migration approach" — the mechanism-verb "ALTER TABLE" or "CREATE INDEX" appears in the action text as content being written, not being executed.
- CLAUDE.md pins that quote or discuss the rule itself — the rule's own mechanism-verbs appear in the action text.

The gate's design intent is "if the action text contains the mechanism-verb, MAYBE the action performs it — proceed to determine follow/ignore." But the LLM is over-applying "contains mechanism-verb → assume performing" → routing legitimate documentation edits to `ignored` when they should be `not_applicable`.

**Concrete operator-flagged example:**
Constraint `never-commit-directly-to-main` (must_not, mechanism-verb: git commit). Action: `Edited docs/development/jiminy-ceiling-break-2/README.md: replaced 'Phase 2 — surfacing tighten' with 'Phase 2 — surfacing tighten (shipped; see JIMINY-CORPUS-003 sprint post — landed via a single dev-branch commit)'`. Current classifier verdict: `ignored` (LLM sees "commit" in the NEW text, treats it as a performed commit action, judges the doc-edit as failing to respect the constraint). Correct verdict: `not_applicable` (the action is a file edit; no git commit was performed; the word "commit" is prose content, not an invoked mechanism). This mislabel currently counts against actionable follow-rate.

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- Add `mentionVsPerformCreditClause` constant to `internal/jiminy/outcome_classifier.go` (after `mechanismScopeCreditClause`, mirroring the shipped clause shape verbatim).
- Add `mentionVsPerformCredit bool` field to `OutcomeClassifier` + `MentionVsPerformCredit bool` to `OutcomeClassifierConfig`.
- Extend `resolveClassifySystemPrompt()` to splice the new clause when `mentionVsPerformCredit` is true, AFTER `mechanismScopeCreditClause` (strongest-gate-last ordering).
- Config knob `JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED` (code default `false`; operator flips in `.env` after live smoke). Naming rationale: mirrors the `*_CREDIT_ENABLED` suffix of the three shipped siblings; uses operator-facing action-language ("mention vs perform") rather than implementation-language ("disambig"); does not narrow to "doc-edit" surface — also fires on quoted-string mechanism-verbs in source code, JSON fixtures, string constants.
- Wire through `internal/config/config.go` (`Config` struct + `getBool` + wire-through), mirroring the three siblings' call-site pattern.
- Wire through `NewService`/`NewOutcomeClassifier` chain (already fully parameterised via `OutcomeClassifierConfig`).
- Pin tests at multiple layers (see §6).
- Live Tier-3 smoke on `mdemg-dev`: 6 real fixtures where flag-off gives `ignored` and flag-on gives `not_applicable`, with the delta driven ONLY by the new clause (mechanism-scope + context-mismatch + non-violation all held ON in both branches).
- Docs + CLAUDE.md pin + CHANGELOG entry.
- `.env` flag flip after smoke.

**Out of scope:**

- Any change to the shipped `mechanismScopeCreditClause` text — this sprint EXTENDS, does not modify. Preserves the shipped ULTS hash for the flag-off path.
- Bulk relabel of historic `constraint_outcomes` rows currently mislabeled `ignored` on this class. Forward-only fix.
- Changes to the writer gate at `internal/jiminy/service.go:1730,1762` (already correctly filters `not_applicable` out of `constraint_outcomes` and `guidance_training_rows`).
- Multi-day A/B window — piggybacks on the arc's 2026-08-19 passive re-read.
- Any tier-1 (embedding) or tier-3 (retrain) work. Pure prompt-extension sprint.

**Composition constraints:**

- Composable with the three shipped credit clauses — narrower→broader ordering, strongest-gate-last.
- With all four flags ON, the render order is exactly: base → non-violation → context-mismatch → mechanism-scope → mention-vs-perform.
- Default OFF in code (byte-identical to base prompt for the flag-off path), ON in `.env` after live smoke. ULTS `system_prompt_hash` pin preserved for the historical hash.

## 4. Dependencies

**Prerequisites (must be shipped BEFORE this sprint):**

- **JIMINY-CLASSIFIER-CONTEXT-002** (shipped 2026-08-12) — the clause this sprint disambiguates.
- **JIMINY-CLASSIFIER-CONTEXT-001** (shipped 2026-07-29) — established the credit-clause pattern.
- **JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001** — original credit-clause architecture.
- **Writer gate** at `internal/jiminy/service.go:1730,1762` — filters `not_applicable` rows out of `constraint_outcomes` and `guidance_training_rows`. This sprint's lift depends on the gate being correct.
- **ULTS-CI-001 hash pin** — `system_prompt_hash` for `jiminy.evaluate_llm` (default-off render must stay byte-identical to the historical const so ULTS does not need a hash-bump migration).

**Arc measurement window coupling:**

- JIMINY-CEILING-BREAK-2 arc has an authoritative baseline captured at 2026-08-12 18:17 UTC and a scheduled passive re-check at 2026-08-19.
- **Measurement window: 2026-08-14 → 2026-08-19.** For attribution, commit MUST land inside that window (target 2026-08-15) so the `.env` flip is live for at least the last ~72-96h.
- If commit slips past 2026-08-16, defer flag flip to the next 7d window (2026-08-20 → 2026-08-27) to preserve clean attribution.

## 5. Implementation Plan (sequential epics + gates)

### Epic 1 — Clause draft + wiring

Add `mentionVsPerformCreditClause` after the shipped `mechanismScopeCreditClause` in `internal/jiminy/outcome_classifier.go`. Draft text (operator-language, LLM-follow-friendly; sits WITHIN the mechanism-scope-gate step, refining the "invoked the mechanism" check):

```
MENTION-vs-PERFORM DISAMBIGUATION (refines the mechanism-scope gate above):
When checking whether the action invoked the constraint's mechanism-verb, distinguish MENTIONING the verb from PERFORMING it.

The verb is MENTIONED (not performed) when it appears as:
- Text CONTENT the agent wrote or edited into a file — prose in a doc, markdown, README, sprint plan, CLAUDE.md, help text, comment; the string body of a code literal, JSON fixture, or test-data blob; a quoted rule the agent is discussing.
- The OLD side of a "replaced 'OLD' with 'NEW'" edit summary — content REMOVED, not an action performed.
- A description in the action summary of what the agent will discuss, plan, quote, or document — not what it is executing.

The verb is PERFORMED when the action is:
- A Bash / tool invocation that actually runs the verb (`git commit`, `git push`, `psql -c "ALTER TABLE ..."`, an ID-generation call, a schema-migration script executed).
- A code change that invokes the mechanism at runtime (adding a call to a commit-triggering function, wiring an ID-mint into a code path that will run, an executable migration file).
- The agent's own summary explicitly describes performing the mechanism ("committed on branch X", "ran the migration", "generated a new identifier for record Y").

If the mechanism-verb appears ONLY as mentioned content — no execution, no runtime invocation, no performed action — treat the mechanism-scope gate as FAILING (verdict = "not_applicable" UNCONDITIONALLY, same as if the verb were absent). Editing a file that talks about commits is not committing. Writing a sprint plan that discusses "ALTER TABLE" is not altering a table. Quoting a rule about identifiers in CLAUDE.md is not generating an identifier.

Only treat the mechanism-scope gate as PASSING (proceed to followed / ignored / partial / contradicted) when the action actually performs the verb.

Examples:
- Constraint "never commit directly to main" + action "Edited README.md: added text describing the commit workflow" → not_applicable (mentioned, not performed).
- Constraint "never commit directly to main" + action "Ran `git commit -m 'x' && git push origin main`" → contradicted (performed on main).
- Constraint "use CUIDv2 for identifiers" + action "Edited docs/id-strategy.md: replaced 'uuid v4' with 'CUIDv2'" → not_applicable (mentioned; no identifier generated).
- Constraint "use CUIDv2 for identifiers" + action "Added `id := cuid.NewV2()` to internal/store/record.go" → followed.
- Constraint "never alter schema without a migration file" + action "Wrote sprint_plan.md: described an ALTER TABLE migration approach" → not_applicable (mentioned as plan content).
- Constraint "run lint before commit" + action "Edited CLAUDE.md: quoted the pre-commit lint rule" → not_applicable (mentioned; no commit prepared).

This is the strongest gate qualifier — apply it BEFORE finalising a mechanism-scope pass/fail decision.
```

Add `mentionVsPerformCredit bool` field on `OutcomeClassifier`, `MentionVsPerformCredit bool` on `OutcomeClassifierConfig`, and the splice in `resolveClassifySystemPrompt` after the shipped `mechanismScopeCreditClause` branch.

- **Gate:** `go build ./...` clean.

### Epic 2 — Config knob

Add `JiminyMentionVsPerformCreditEnabled bool` to `Config` in `internal/config/config.go` with the sibling-comment shape. Add `jiminyMentionVsPerformCreditEnabled := getBool("JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED", false)` in the config loader.

Wire into `NewService` / `NewOutcomeClassifier` where `MechanismScopeCredit` is passed today.

- **Gate:** `go build ./...` clean; `golangci-lint run` → 0 issues (watch for gosec G101 false-positive on "CREDIT" prose; add `//nolint:gosec` per shipped siblings).

### Epic 3 — Pin tests (Tier-1 unit)

See §6 for the full test list.

- **Gate:** `go test ./internal/jiminy/... -count=1` green; specifically the byte-identical default-off pin.

### Epic 4 — Live Tier-3 smoke fixtures

Assemble the fixture set in §6 Tier-3. Run each twice:
1. Baseline branch: all four credit flags ON except `mentionVsPerformCredit`.
2. Fix branch: all four flags ON.

Log verdict + reasoning JSON. Confirm ≥4/6 fixtures flip `ignored → not_applicable` with the delta attributable in the LLM's own reasoning.

- **Gate:** ≥4 clean flips; 0 regressions on the two counter-fixtures.

### Epic 5 — `.env` flip + attribution note

Flip `JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED=true` in `.env` on `mdemg-dev`. Add a line to the JIMINY-CEILING-BREAK-2 arc README under "Baseline snapshot" noting the Phase 3.5 knob flipped at [timestamp].

- **Gate:** service restart + tail logs for ≥5min; verify no classifier panics; ≥1 `jiminy.evaluate_llm` call round-trips with the new prompt hash.

### Epic 6 — Verification

Full checklist in §8.

### Epic 7 — Commit + push

Single commit per §7.

### Epic 8 — Documentation update

Never cut. See §9.

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit / pin tests

Added to `internal/jiminy/outcome_classifier_test.go`:

1. `TestResolveClassifySystemPrompt_MentionVsPerform_DefaultOff_ByteIdentical` — with `mentionVsPerformCredit=false` and all other flags false, render equals `classifySystemPrompt` exactly (ULTS pin preservation).
2. `TestResolveClassifySystemPrompt_MentionVsPerform_On_Extended` — flag on, all others off; render equals `classifySystemPrompt + mentionVsPerformCreditClause`; contains `"MENTION-vs-PERFORM DISAMBIGUATION"`.
3. `TestNewOutcomeClassifier_MentionVsPerformCredit_Propagates` — `OutcomeClassifierConfig.MentionVsPerformCredit=true` propagates to the internal field.
4. `TestResolveClassifySystemPrompt_MentionVsPerform_AfterMechanismScope` — with both flags true, mention-vs-perform appears AFTER mechanism-scope. Locks strongest-gate-last ordering.
5. `TestResolveClassifySystemPrompt_AllFourCredits_Ordering` — extends the shipped `AllThreeCredits_Ordering` to four flags. Asserts exact splice order.
6. `TestResolveClassifySystemPrompt_MentionVsPerform_Compact_Compatible` — with `compressPrompts=true`, clause still splices onto `classifySystemPromptCompact`.

### Tier 2 — Contract / integration

- `go build ./...` clean.
- `golangci-lint run` — 0 issues.
- `go test ./... -count=1` full green.
- ULTS `system_prompt_hash` pin verified — historical hashes MUST still be produced for the flag-off render.

### Tier 3 — Live fixtures

6 real content examples; each run flag-off vs flag-on, delta driven ONLY by the new clause:

| # | Constraint | Action (real content) | OLD verdict | NEW verdict |
|---|---|---|---|---|
| 1 | `never-commit-directly-to-main` (must_not) | Edited `CLAUDE.md`: added a pin quoting "never commit directly to main". | `ignored` | `not_applicable` |
| 2 | `use-cuidv2-for-identifiers` (must) | Edited `docs/development/id-strategy/sprint_plan.md`: wrote a section describing "generate CUIDv2 at record-mint time". | `ignored` | `not_applicable` |
| 3 | `never-alter-schema-without-migration` (must_not) | Edited `docs/development/schema-refactor/sprint_plan.md`: authored a plan describing an `ALTER TABLE constraints ADD COLUMN role_type` approach. | `ignored` | `not_applicable` |
| 4 | `mermaid-over-ASCII-diagrams` (should) | Edited `docs/features/jiminy-actionability.md`: added a section that CONTAINS a mermaid diagram block. | `partial_compliance` or `followed` | Same. Counter-fixture. |
| 5 | `run-lint-before-commit` (must) | Edited `docs/features/jiminy-actionability.md`: added prose text quoting the pre-commit lint rule. | `ignored` | `not_applicable` |
| 6 | `never-commit-directly-to-main` (must_not) | Ran `git commit -m 'META-SCOPE-001: draft clause' && git push origin reh3376_dev01`. | `followed` (dev branch) | Same. Counter-fixture. |

Success criteria: fixtures 1, 2, 3, 5 flip `ignored → not_applicable`. Fixtures 4 and 6 stay at their existing verdict.

**Expected lift arithmetic:**

- Pre-fix baseline: 1412 actionable outcomes / 7d, 166 followed, 1199 `ignored` → 13.25% actionable follow rate.
- Estimated fraction of the 1199 `ignored` verdicts that are doc-edit / prose actions: **~15-25%** → routed-to-NA count ~180-300.
- New denominator: ~1172. New follow rate: **~14.2%**.
- Combined with CONTEXT-002's Phase 3 projected +5-10pp, the arc's 2026-08-19 read should land in the **~19-24% band** if both levers behave as modelled.
- If actual delta from this clause is materially larger than +1pp (e.g., >+3pp), the doc-edit fraction was under-estimated OR the clause is over-correcting; audit fixture #6 and live samples.

## 7. Commit Strategy

Single commit under `JIMINY-CLASSIFIER-META-SCOPE-001`. Message body must:

- Reference JIMINY-CLASSIFIER-CONTEXT-002 as the sprint being narrowed.
- Note the arc adjacency (JIMINY-CEILING-BREAK-2 Phase 3.5).
- Include the ULTS-pin-preservation invariant statement.
- List the fixture flip count from Tier-3 smoke.

`.env` flip lands SEPARATELY (Epic 5) — not part of the commit.

## 8. Verification Checklist

- [ ] `mentionVsPerformCreditClause` constant added, positioned after `mechanismScopeCreditClause`.
- [ ] `mentionVsPerformCredit bool` field on `OutcomeClassifier` + `MentionVsPerformCredit bool` on config.
- [ ] `resolveClassifySystemPrompt()` extended; new clause splices AFTER `mechanismScopeCreditClause` when flag on.
- [ ] `JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED` env var wired; code default `false`.
- [ ] `NewOutcomeClassifier` receives new config field; internal boolean propagates.
- [ ] Six unit pin tests green (see §6).
- [ ] Byte-identical default-off render preserved — ULTS `system_prompt_hash` unchanged.
- [ ] `go build`, lint, tests full green.
- [ ] Six Tier-3 live fixtures run; ≥4 flips; both counter-fixtures preserve verdicts.
- [ ] Writer-gate invariant verified — `not_applicable` filtered at `service.go:1730,1762`.
- [ ] CHANGELOG entry.
- [ ] CLAUDE.md pin (pattern extension note with explicit "mention vs perform" wording).
- [ ] `docs/features/jiminy-actionability.md` §"Mention-vs-Perform" section.
- [ ] `.env` flip on mdemg-dev; service restart; ≥5min clean traffic post-flip.
- [ ] Arc README annotation added.

## 9. Documentation Update (Epic 8, never cut)

Three surfaces:

**`docs/features/jiminy-actionability.md`** — add `## Mention-vs-Perform (JIMINY-CLASSIFIER-META-SCOPE-001, 2026-08-14)` section matching existing follow-up shape.

**`CLAUDE.md`** — add pin under the JIMINY-CLASSIFIER-CONTEXT-002 pin, noting CONTEXT-002's mechanism-scope gate is REFINED by META-SCOPE-001's mention-vs-perform clause; both flags default-off in code but ON in `mdemg-dev` `.env`; warn future authors to preserve strongest-gate-last ordering.

**`CHANGELOG.md`** — single entry under `### Added`.

**JIMINY-CEILING-BREAK-2 arc README** — one-line annotation under Baseline snapshot / Attribution stating "Phase 3.5 flipped 2026-08-XX; 2026-08-19 re-check measures CONTEXT-002 + META-SCOPE-001 combined."

## 10. Risks & Mitigations

**Risk A — Over-correction (routing real perform-actions to `not_applicable`).** The clause is a narrowing qualifier ON TOP of the mechanism-scope gate. Mitigations: clause text lists PERFORM examples explicitly; Tier-3 fixtures 4 + 6 are counter-fixtures; post-flip arc re-check audit if follow rate climbs above the ~19-24% band.

**Risk B — Prompt-hash drift on the default-off path.** Mitigated by the byte-identical pin test (§6 test 1). Resolver method does not mutate the base const; new clause splices only when flag is on. CI catches accidental mutation.

**Risk C — Arc-window collision.** Arc README annotation (§9) makes composition explicit. Attribution isolation in Tier-3 provides an independent fixture-based read on this clause's marginal contribution. If schedule slips past 2026-08-16, defer flip to 2026-08-20 → 2026-08-27 window.

**Risk D — Interaction with `contextMismatchCreditClause`.** Both can route to `not_applicable` but cover disjoint failure modes (wrong-space vs mentioned-only). Additive, not conflicting. Pin test 5 locks the render order.

**Risk E — LLM ignores the narrowing.** If Tier-3 flip count is <4/6, strengthen with an EXPLICIT `Edited FILE:` action-summary pattern rule as fallback. Do NOT add before initial measurement.

## 11. Rollback Procedures

- **Level 1 (env flip):** `.env` line `JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED=true` → `false`; service restart. Classifier reverts to CONTEXT-002 behaviour immediately.
- **Level 2 (code revert):** revert the single sprint commit. Code default returns to flag-off, which by pin equals pre-sprint behaviour. Zero blast radius.
- **Level 3 (partial rollback):** leave code shipped as dormant capability; keep `.env` false indefinitely.

## 12. Documents Accessed

- `internal/jiminy/outcome_classifier.go` — constants block; `resolveClassifySystemPrompt`; struct + config field shape; `NewOutcomeClassifier` wiring.
- `internal/jiminy/outcome_classifier_test.go` — existing pin tests for the three credit clauses; ordering test.
- `internal/config/config.go` — `JiminyMechanismScopeCreditEnabled` position; `getBool` loader; `Config` return literal.
- `internal/jiminy/service.go` — writer gate at 1730, 1762.
- `docs/development/jiminy-classifier-context-001/sprint_plan.md` — canonical 12-section format.
- `docs/development/jiminy-classifier-context-002/sprint_post.md` — Phase 3 shipping context.
- `docs/development/jiminy-ceiling-break-2/README.md` — arc plan; baseline snapshot; 2026-08-19 re-check attribution structure.
- `docs/features/jiminy-actionability.md` — structure for Epic 8 doc update.
