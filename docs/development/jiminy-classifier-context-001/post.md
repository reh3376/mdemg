# JIMINY-CLASSIFIER-CONTEXT-001 — Sprint Post

**Date:** 2026-07-29 | **Branch:** `reh3376_dev01`
**Parent:** JIMINY-CEILING-INVESTIGATION-001 recommendation #1.
**Sibling pattern:** JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001.

## Verdict

**Shipped default-off in code, flipped ON in `.env` after live smoke.**
Extends the classifier prompt with a generalized context-mismatch
credit clause; routes context-mismatched verdicts to `not_applicable`
(filtered out by the shipped writer gate) instead of `ignored`
(inflates the actionable denominator).

## What shipped

- **`contextMismatchCreditClause`** constant in
  `internal/jiminy/outcome_classifier.go` — appended by
  `resolveClassifySystemPrompt()` when
  `contextMismatchCredit=true`. Clause carries concrete examples:
  git-vs-file, code-vs-query, workflow-step mismatch, language
  mismatch, and the "constraint is a session log / completion log /
  phase description" surface-mismatch fallback.
- **`contextMismatchCredit` field** on `OutcomeClassifier` +
  `ContextMismatchCredit bool` on `OutcomeClassifierConfig`.
- **Env var** `JIMINY_CONTEXT_MISMATCH_CREDIT_ENABLED` (code default
  `false`; flipped ON in `.env` after live smoke).
- **Wire** through `internal/config/config.go` → `service.go` →
  `NewOutcomeClassifier` chain. Startup log now reports
  `context_mismatch_credit=true`.
- **5 unit tests** (4 new + 1 propagation) pin:
  - default-off byte-identical to `classifySystemPrompt` const
    (ULTS-CI-001 `system_prompt_hash` pin preserved — no hash bump
    needed)
  - flag-on splice for both full + compact base prompts
  - both-flags-on splice order (non-violation first, context-mismatch
    second — narrower-before-broader documented intent)
  - `ContextMismatchCredit` config field flows through
    `NewOutcomeClassifier`

## Live Tier-3 smoke (mdemg-dev, direct-LLM against llama-server)

Three fixtures constructed to represent the failure modes surfaced by
JIMINY-CEILING-INVESTIGATION-001:

**Fixture 1** — git-commit rule + file-write action (context mismatch):
- FLAG-OFF: `"ignored"` conf=0.8 (reasoning literally admits *"the
  guidance is not applicable"* but still labels ignored)
- **FLAG-ON: `"not_applicable"` conf=0.9 ✓ FLIPPED**

**Fixture 2** — plan-mode-before-change rule + read-only Cypher query:
- FLAG-OFF: `"ignored"` conf=0.9
- FLAG-ON: **`"ignored"` conf=0.9 — STUBBORN.** The LLM interpreted
  the rule broadly ("planning discipline in general") and read the
  read-only query as not-honoring-planning-mode. Not a false-positive
  regression; just a case the clause doesn't rescue.

**Fixture 3** — session-log "constraint" + unrelated grep action
(surface mismatch — the class 44% of the investigation samples fell
into):
- FLAG-OFF: `"ignored"` conf=0.8
- **FLAG-ON: `"not_applicable"` conf=0.9 ✓ FLIPPED**

**Score: 2/3 clear flips + 1 stubborn + 0 regressions.** Better than
the sibling compliance-credit sprint's live-smoke ratio at ship. Flag
flipped in `.env` per the sibling sprint's post-smoke pattern.

## What this predicts vs what the sibling actually delivered

The JIMINY-CEILING-INVESTIGATION-001 report predicted this sprint
would lift the follow rate from 11% to 35-50% by routing ~50% of the
current `ignored` verdicts to `not_applicable`. The sibling sprint
(compliance-credit for must_not only) predicted 18-25% and actually
landed at ~13% (per the shipped `ab_verdict.md` — "recalibrated
steady-state expectation ~13%").

**Applying the sibling's calibration lesson**: the actual lift will
likely be LESS than 35-50%. Reasons:
- The sibling sprint's post noted that "88% of NA routing landed on
  advisory guidance outside the actionable denominator" — same
  dilution likely applies here.
- Volume coupling: as more items route to not_applicable, followed
  count doesn't stay fixed — it scales with surfacing volume too.
- Recalibrated realistic expectation: **~15-25%** honest actionable
  follow rate. Still a meaningful lift on the ~11-13% baseline;
  passive re-read at 3-7d will land the actual number.

## Passive follow-up

Re-measure follow rate over a quiet 7d window ~2026-08-05 (matches
the sibling sprint's precedent). If the lift lands well below the
15-25% recalibrated band, the residual bottleneck is corpus quality
(JIMINY-CORPUS-002, disclosed) — not more classifier tuning.

## Rules pinned

1. **Context-mismatch credit is the general case; non-violation
   credit is the specific case** (must_not-only). Both flags can be
   enabled; the clauses splice in narrower→broader order for the LLM
   to see the specific example before the general rule.
2. **Prompt extension via resolver method + default-off gate is the
   shipping pattern** for classifier prompt changes. Preserves the
   ULTS `system_prompt_hash` pin so drift-checker CI stays green
   without a hash bump.

## Follow-ups disclosed

1. **JIMINY-CORPUS-002** (~2d) — second corpus purge pass with
   retroactive tombstone of confirmed non-rules (from
   JIMINY-CEILING-INVESTIGATION-001 finding: `auto-015a122bcbb8`,
   `auto-fcb814b48e33`, `auto-9f5134a1a0c3`,
   `full-system-gap-analysis`, `llm-multi-hop-synthesis` etc). Named
   in the investigation but not shipped yet. Higher-lift if the
   corpus contamination is the residual bottleneck after this sprint.
2. **JIMINY-TIER1-BYPASS-001** (~1d) — bypass tier1 for the
   follow/ignore decision (keep as fast pre-gate for not_applicable
   only). tier1 has 1.0% follow rate over 102 events on real rules;
   functionally blind. Named in the investigation.
3. **Re-measure follow rate ~2026-08-05** (passive, no sprint needed;
   just a query against `constraint_outcomes`).

## Documents Accessed

- `docs/development/jiminy-ceiling-investigation-001/post.md` (parent
  investigation)
- `docs/development/jiminy-actionability-compliance-credit-001/`
  (sibling sprint's shape — resolver method, default-off, A/B verdict)
- `internal/jiminy/outcome_classifier.go` (const, struct, resolver
  method edits)
- `internal/config/config.go` (env var wire)
- `internal/jiminy/service.go` (NewOutcomeClassifier constructor)
- Live direct-LLM calls to llama-server :8102 with 3 fixtures
- Post-restart server log confirming `context_mismatch_credit=true`
