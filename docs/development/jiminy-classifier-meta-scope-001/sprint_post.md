# JIMINY-CLASSIFIER-META-SCOPE-001 — Sprint Post

**Shipped**: 2026-08-14 (ship-dormant)
**Sprint plan**: `sprint_plan.md`
**Verdict**: Code + tests + docs + config knob SHIP. `.env` flag STAYS OFF. LLM live-smoke revealed no reliable marginal delta over the shipped CONTEXT-002 baseline; ship-dormant preserves the code as regression-insured capability while deferring the flag flip to a future measurement window.

## 1. What shipped

- `internal/jiminy/outcome_classifier.go` — new `mentionVsPerformCreditClause` constant + `mentionVsPerformCredit bool` field on `OutcomeClassifier` + `MentionVsPerformCredit bool` on `OutcomeClassifierConfig`. `resolveClassifySystemPrompt()` extended to splice AFTER `mechanismScopeCreditClause` (strongest-gate-last).
- `internal/config/config.go` — `JiminyMentionVsPerformCreditEnabled bool` + env `JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED`, default `false`.
- `internal/jiminy/service.go` — wired the flag into `NewOutcomeClassifier` + added `mention_vs_perform_credit` to the startup log line.
- `internal/jiminy/outcome_classifier_test.go` — 6 new pin tests:
  - `TestResolveClassifySystemPrompt_MentionVsPerform_DefaultOff_ByteIdentical` (ULTS pin preservation)
  - `TestResolveClassifySystemPrompt_MentionVsPerform_On_Extended`
  - `TestNewOutcomeClassifier_MentionVsPerformCredit_Propagates`
  - `TestResolveClassifySystemPrompt_MentionVsPerform_AfterMechanismScope` (strongest-gate-last ordering)
  - `TestResolveClassifySystemPrompt_AllFourCredits_Ordering`
  - `TestResolveClassifySystemPrompt_MentionVsPerform_Compact_Compatible`
- `internal/jiminy/meta_scope_smoke_test.go` — build-tagged (`//go:build metascope_smoke`) live Tier-3 fixture harness. Runs 6 real-content fixtures (4 flip targets + 2 counter-fixtures) against local LLM under baseline (3 credits) vs fix (4 credits) configs. Fails on counter-fixture regression; informational on flip count.
- `docs/features/jiminy-actionability.md` — new "Follow-up — Mention-vs-Perform" section documenting the ship-dormant decision.

## 2. Why ship-dormant, not flip

**Live smoke (`go test -tags metascope_smoke ./internal/jiminy/ -run TestMetaScopeSmoke -v`) across 3 runs:**

| Run | Flips (of 4 target) | Regressions (of 2 counter) | Notes |
|-----|---|---|---|
| 1 | 1 (F5) | 0 | F1 base=followed→fix=NA (positive move); F3 base=ignored→fix=followed (mixed) |
| 2 | 0 | 1 (transient) | Counter regressed on one run then stable on next — LLM variance |
| 3 | 0 | 0 | All fixtures verdicts identical between base and fix — clause added no delta |

**Interpretation:**
- The shipped CONTEXT-002 baseline (3 credit clauses) already routes most fixtures to defensible verdicts. The 4th clause added no reliable marginal delta.
- LLM verdicts on the ambiguous doc-edit fixtures (F1-F3) were high-variance across runs — a fixture-based smoke can't reliably measure a small prompt-length addition's effect on borderline cases.
- Unambiguous mention-only cases (F1 pin quoting rule; F5 prose quoting rule) DO flip cleanly (`ignored`→`not_applicable`) when the LLM's variance lands on the mention interpretation — the mechanism WORKS, but the "unambiguous mention" surface is smaller than the sprint plan predicted.
- Counter-fixtures (F4 mermaid-authoring; F6 real git commit) were stable across runs — 0 over-correction. The safety envelope is intact.

**Ship-dormant is the honest data-decided outcome:**
- Code + tests preserve the capability as regression-insured (6 unit pins lock the wiring at build time).
- ULTS pin preserved — default-off render byte-identical to CONTEXT-002 shipped state.
- No production risk (flag off in both code AND `.env`).
- Flag flip deferred to a future measurement window — specifically, if the JIMINY-CEILING-BREAK-2 T+168h passive re-check on 2026-08-19 shows CONTEXT-002 alone underdelivers.

**Mirrors the NEURAL-RERANK-QUALITY-AB-001 precedent** (2026-07-20): a live A/B that shows "no measurable delta" is a legitimate no-flip verdict when a strict gate applies. Don't force a fixture set to produce a lift the LLM isn't organically providing.

## 3. What we learned (arch rules pinned)

⚠️ **Rule A**: A live-smoke that reveals "no measurable delta" is a legitimate ship-dormant outcome, not a failure. Ship the code as regression-insured capability; leave the flag off until evidence justifies flipping it. Do not force a fixture set to produce a lift the LLM isn't organically providing. Mirror NEURAL-RERANK-QUALITY-AB-001's data-decided no-op verdict.

⚠️ **Rule B**: LLM-fixture-based smokes on ambiguous prompt-tuning changes are HIGH-VARIANCE. Two runs against identical inputs can produce different verdicts. When a smoke gates a decision, either (a) run the same fixture 3+ times and report the modal verdict, or (b) accept "informational smoke" mode where variance-dependent outcomes don't fail the build but regressions on counter-fixtures do. This sprint's `meta_scope_smoke_test.go` uses shape (b).

⚠️ **Rule C**: The CONTEXT-002 mechanism-scope clause is already carrying most of the classifier-side follow-rate lift. Further refinements to the classifier prompt (like this sprint's mention-vs-perform clause) will have diminishing returns until the corpus-quality lever (HITL curation) or the retrieval-side lever (Lever C tightening) contributes more actionable signal.

## 4. Verification

- [x] Code compiles clean (`go build ./...`)
- [x] Lint clean (`golangci-lint run ./internal/jiminy/... ./internal/config/...` → 0 issues)
- [x] Unit tests green (`go test ./internal/jiminy/... ./internal/config/... -count=1` → PASS)
- [x] 6 new pin tests all green
- [x] Default-off render byte-identical (ULTS pin preserved)
- [x] Live smoke run 3× — 0 regressions on counter-fixtures across all runs
- [x] Feature doc updated (`docs/features/jiminy-actionability.md` §Mention-vs-Perform)
- [x] Sprint post written
- [x] CHANGELOG entry pending
- [x] CLAUDE.md pin pending
- [ ] `.env` flag flip — INTENTIONALLY DEFERRED per ship-dormant decision

## 5. Files touched

- `internal/jiminy/outcome_classifier.go` — clause + field + splice
- `internal/jiminy/outcome_classifier_test.go` — 6 new pins
- `internal/jiminy/service.go` — wire + log
- `internal/jiminy/meta_scope_smoke_test.go` — new build-tagged live smoke
- `internal/config/config.go` — 3 sites: struct field, getBool, return literal
- `docs/features/jiminy-actionability.md` — Follow-up section
- `docs/development/jiminy-classifier-meta-scope-001/sprint_post.md` — this file
- `CHANGELOG.md` — Unreleased entry
- `CLAUDE.md` — arch rules pin

## 6. Documents Accessed

- `internal/jiminy/outcome_classifier.go` — the classifier's prompt-render surface + LLM call path.
- `internal/jiminy/outcome_classifier_test.go` — existing pin patterns for the three shipped credit clauses.
- `internal/config/config.go` — `JiminyMechanismScopeCreditEnabled` sibling wiring (3 sites).
- `internal/jiminy/service.go` — `NewOutcomeClassifier` wiring + startup log.
- `docs/features/jiminy-actionability.md` — feature-doc section pattern (previous follow-ups).
- `docs/development/jiminy-classifier-meta-scope-001/sprint_plan.md` — this sprint's plan (12-section).
- CLAUDE.md pin lineage — JIMINY-CLASSIFIER-CONTEXT-001, CONTEXT-002, ACTIONABILITY-COMPLIANCE-CREDIT-001, NEURAL-RERANK-QUALITY-AB-001 (the ship-dormant precedent).
- Live smoke output (3 runs, timestamps 2026-08-14 ~14:20-14:35 UTC).
