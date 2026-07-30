# JIMINY-TIER1-BYPASS-001 — Sprint Plan

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** JIMINY-CEILING-INVESTIGATION-001 defect **B**
(tier1 systematically mislabels follows as ignored). Q4 follow-up #4.

## 1. Header & Metadata

Bypass tier1 (embedding-similarity short-circuit) for the follow/
ignore decision in `outcome_classifier.go`. Keep tier1 only as a fast
pre-gate for `not_applicable` (unrelated). All ambiguous cases route
to the LLM tier2 which reads the constraint prompt with reasoning.
~1-1.5h implementation + ~1h live A/B + ~30m docs. Config-gated
default-off; flip after live smoke.

## 2. Problem Statement

The JIMINY-CEILING-INVESTIGATION-001 categorized 16 high-similarity
LLM samples and found:
- Tier1 (embedding cosine sim) is **functionally blind to follows**:
  1% follow rate over 102 real-durable-rule events (`tier1-only` cohort)
- Definitional: following a rule doesn't require semantic similarity
  between the rule text and the action. `git push origin
  reh3376_dev01` follows "never commit to main" but the embeddings
  aren't similar.

**Live 7d data on mdemg-dev (confirms):**

| classifier_source | outcome | n | avg_sim |
|---|---|---|---|
| tier1 | ignored | 414 | 0.166 |
| tier1 | followed | 6 | 0.570 |
| llm | ignored | 569 | 0.837 |
| llm | followed | 135 | 0.866 |
| llm | partial_compliance | 8 | 0.750 |

Tier1 is producing **414 sub-LOW-band `ignored` verdicts per 7d**
(sim in `[naThreshold=0.10, lowThreshold=0.20)` — topically related
but low sim), most of which are almost certainly mislabeled follows
(the action didn't happen to mirror the constraint's phrasing).

## 3. Scope & Constraints

**In scope (single-commit sprint):**

- Modify `internal/jiminy/outcome_classifier.go::Classify`:
  - Keep the `similarity < naThreshold` fast-path for `NotApplicable`
    (unchanged — this IS the pre-gate for irrelevance)
  - When bypass enabled AND `similarity in [naThreshold, lowThreshold)`:
    fall through to LLM tier2 (currently returns tier1 `Ignored`)
  - When bypass enabled AND `similarity ≥ highThreshold && !hasNegation`:
    fall through to LLM tier2 (currently returns tier1 `Followed`)
  - LLM path already handles everything correctly downstream
- New env var: `JIMINY_TIER1_BYPASS_ENABLED` (default false; flip
  after live smoke)
- ULTS `system_prompt_hash` pin unaffected — this is a control-flow
  change in Go, not a prompt change
- Live A/B: baseline (bypass off) vs candidate (bypass on) on a
  synthesized fixture set of ambiguous cases + real 24h traffic
- Flag flipped ON in `.env` after live smoke

**Out of scope:**

- Changes to LLM prompt (the shipped classifySystemPrompt already
  handles the follow/ignore decision correctly)
- Removing tier1 entirely (its `not_applicable` pre-gate is a real
  optimization — currently gates ~55% of surface volume before an
  LLM call)
- Changes to thresholds (config `JIMINY_OUTCOME_*` values unchanged)
- Cost/latency SLO changes — the estimated LLM cost lift is +60
  calls/day (from 102 → 162 baseline llm_interactions.task_name
  `jiminy.classify_outcome`), acceptable

## 4. Method

**Phase 1 — Code change**
- Add config `JiminyTier1BypassEnabled` + `atob` + `FromEnv` +
  struct assignment
- Add `oc.tier1BypassEnabled` field on `OutcomeClassifier`
- Wire the flag through `NewOutcomeClassifier` from `cfg`
- Modify `Classify` per Scope

**Phase 2 — Unit tests**
- Case: sub-naThreshold → `NotApplicable` (unchanged)
- Case: [naThreshold, lowThreshold), flag off → tier1 `Ignored`
- Case: [naThreshold, lowThreshold), flag on → LLM tier2 fires
- Case: ≥ highThreshold no-negation, flag off → tier1 `Followed`
- Case: ≥ highThreshold no-negation, flag on → LLM tier2 fires
- Case: middle band → LLM tier2 fires (unchanged both ways)

**Phase 3 — Live Tier-3**
- Baseline snapshot: 24h `constraint_outcomes` classifier_source
  split (currently ~35% tier1, ~60% llm, ~5% heuristic)
- Enable bypass; 1h observation window
- Query: post-bypass `classifier_source` distribution — expect tier1
  fraction to DROP (only the `not_applicable` sub-cases stay tier1)
- Sample 5-10 individual outcomes to spot-check LLM verdicts vs old
  tier1 verdicts on the same action shape
- Guard: verify LLM error rate doesn't spike (~5% baseline)
- If OK: flip flag in `.env`

**Phase 4 — Docs**
- Post + CHANGELOG + CLAUDE.md pin (with the pinned rule about tier1
  embedding-sim's structural blindness to follows)

## 5. Testing Plan

- **Tier 1 (unit)**: 6 test cases above, mocking the LLM client to
  verify the branch dispatch
- **Tier 2 (integration)**: none needed — the change is a control-flow
  gate, not a wire change; unit tests + live smoke cover it
- **Tier 3 (live)**:
  - Pre-bypass baseline capture (SQL against `constraint_outcomes`)
  - Enable bypass → 1h observation
  - Post-bypass `classifier_source` distribution query
  - Spot-check 5 sampled outcomes for correctness
  - LLM error rate guard

## 6. Commit Strategy

Single commit under `JIMINY-TIER1-BYPASS-001`.

## 7. Verification Checklist

- [ ] Config field + FromEnv + struct wire
- [ ] `oc.tier1BypassEnabled` field + constructor wire
- [ ] `Classify` modified per spec
- [ ] 6 unit tests green
- [ ] Live smoke: tier1 fraction DROPS after enable
- [ ] LLM error rate unchanged
- [ ] Sample verdicts spot-check clean
- [ ] Flag flipped ON in `.env`
- [ ] CHANGELOG + CLAUDE.md pin + post

## 8. Rollback

- Set `JIMINY_TIER1_BYPASS_ENABLED=false` in `.env` (byte-identical
  historical behavior)
- Long-term: revert commit

## 9. Risks

- **Risk**: LLM cost/latency lift. Estimated +60 calls/day is small
  vs current 102/day baseline. Guard: 1h observation window watches
  the alert-evaluator's `alert_llm_health` rule for saturation.
- **Risk**: LLM classifier makes MORE errors than tier1 on the
  currently-tier1-handled cases. Unlikely — the LLM has full prompt
  context including the classifier_prompt's context-mismatch rules
  (from JIMINY-CLASSIFIER-CONTEXT-001), while tier1 has only cosine
  sim. Guard: spot-check 5 sampled outcomes vs a hand-graded expected
  verdict.
- **Risk**: cache stampede on flag flip (new prompts). Mitigated by
  the existing `classifyCacheGet`/`Put` LRU which fills lazily.

## 10. Documents Accessed

Filled in `post.md`.
