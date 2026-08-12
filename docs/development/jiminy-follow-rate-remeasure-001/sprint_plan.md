> ⚠️ **TRAJECTORY ANNOTATION (added 2026-08-12 by FRAMING-HYGIENE-SWEEP-001):** the framing in this doc calls the current follow-rate "honest steady state" / "not urgent" / "expected". That framing was **superseded** by the operator directive of 2026-08-11 ("If the main LLM is only following 10-13% of Jiminy's guidance this entire project is a complete failure") — the arc that owns the ≥80% target is [`docs/development/jiminy-ceiling-break-2/`](../jiminy-ceiling-break-2/README.md). Sprint history preserved below for context; do NOT act on the "not urgent" / "by design" conclusions in the body — those conclusions are wrong per the current architectural directive.

---

# JIMINY-FOLLOW-RATE-REMEASURE-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** JIMINY-FOLLOW-RATE-REMEASURE-001
- **Date:** 2026-08-08
- **Branch:** `reh3376_dev01`
- **Author:** Roger Henley (via Claude Opus 4.7)
- **Parent context:** Q4 roadmap §3 candidate #7; Q4 §5 post-roadmap disclosure. Passive audit sprint promised at close of JIMINY-CLASSIFIER-CONTEXT-001 + JIMINY-CORPUS-002 + JIMINY-TIER1-BYPASS-001 + CORRECTION-CODE-GEN-001 arc.
- **Effort:** 1d (passive measurement + writeup; no code change).

## 2. Problem Statement

The Jiminy ceiling investigation (JIMINY-CEILING-INVESTIGATION-001, 2026-07-29) predicted a lift from the observed **~11% follow rate → ~35-50% honest follow rate** if three shipped remediations landed: JIMINY-CORPUS-002 (corpus purge), JIMINY-CLASSIFIER-CONTEXT-001 (context-mismatch credit clause), JIMINY-TIER1-BYPASS-001 (tier1 systematic mislabel). CORRECTION-CODE-GEN-001 was a supporting shipment that added codes to correction nodes.

CLAUDE.md's post-COMPLIANCE-CREDIT pin recalibrated the honest steady-state expectation to **≈13%** after that sprint's 72h A/B closed at +3.0pp with the model of "18–25% band" invalidated by burst-adjusted analysis.

This sprint verifies which prediction was right — 35-50% or ~13% — using the same methodology as the CEILING investigation's §2 headline snapshot.

## 3. Scope & Constraints

- **In-scope:** TSDB query against `constraint_outcomes` on mdemg-dev; 7d window; actionable filter (`guidance_type IN ('constraint','correction')`); classifier_source + guidance_type + time-drift breakdowns; verdict document; CHANGELOG entry.
- **Out-of-scope:** Code change of any kind. HITL corpus quality lever (if verdict says corpus is the remaining ceiling, that's a separate sprint — HITL-CURATION-003 or similar).
- **Constraint:** window ≥7d post-latest-arc-member (last shipped 2026-07-30, 9d ago → satisfied).

## 4. Dependencies

- All four arc sprints shipped:
  - JIMINY-CORPUS-002 (2026-07-30)
  - JIMINY-TIER1-BYPASS-001 (2026-07-30)
  - CORRECTION-CODE-GEN-001 (2026-07-30)
  - JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 (2026-07-21)
- mdemg-dev TSDB reachable, `constraint_outcomes` table populated.
- Baseline snapshot from `docs/development/jiminy-ceiling-investigation-001/post.md §2`.

## 5. Implementation Plan (sequential)

**Epic 1 — Query the current state.** Run identical query shape as investigation §2. Break down by classifier_source (verify tier1 bypass), guidance_type (constraint vs correction), 7-3d vs 3-0d time-slice (verify stability).

**Epic 2 — Compare against predictions.** Chart current vs baseline vs CEILING prediction (35-50%) vs COMPLIANCE-CREDIT recalibration (~13%).

**Epic 3 — Write verdict.** Document the actual ceiling. Recommend the next lever if the honest ceiling isn't the operator target.

**Epic 4 — CHANGELOG + CLAUDE.md.** Ship as a measurement sprint entry. Update CLAUDE.md steady-state pin ONLY if the finding materially disagrees with the current pin.

## 6. Testing Plan

Not applicable for a passive audit — the "test" IS the measurement (Tier 3 live query against production TSDB).

**Live smoke:** query returns non-zero rows, actionable filter yields >100 samples (real signal), 7d window fully populated. — All satisfied.

## 7. Commit Strategy

Single commit: `docs(jiminy-follow-rate-remeasure-001): verdict — flat at 11.62%, matches CLAUDE.md ~13% recalibration`.

## 8. Verification Checklist

- [x] Query executed against real mdemg-dev TSDB (not a mock)
- [x] Sample size sufficient (1618 actionable > 100 minimum)
- [x] Time window covers ≥7d post-latest-arc-member
- [x] Verdict cites investigation baseline explicitly
- [x] Comparison against BOTH original prediction (35-50%) AND recalibration (~13%)
- [x] CHANGELOG entry added (post-verdict)

## 9. Documentation Update

- `docs/development/jiminy-follow-rate-remeasure-001/verdict.md` — the measurement result
- `CHANGELOG.md` — shipped entry pointing at verdict
- CLAUDE.md — no update; recalibrated ≈13% pin from COMPLIANCE-CREDIT sprint was correct and needs no revision

## 10. Risks & Mitigations

**R1:** Measurement is confounded by sample composition drift (different mix of guidance types in the 10d window vs the baseline 7d window).
*Mitigation:* per-guidance-type breakdown included (Epic 1). Both constraint and correction are within 0.34pp of each other, so composition drift is not masking real signal.

**R2:** Verdict biases toward whichever prediction I read first.
*Mitigation:* both predictions cited with source documents; the numerical delta is unambiguous.

## 11. Rollback Procedures

Not applicable — no code change.

## 12. Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q4.md` §3, §5
- `docs/development/jiminy-ceiling-investigation-001/post.md` §2, §5, §6
- `CHANGELOG.md` (JIMINY-CORPUS-002, JIMINY-TIER1-BYPASS-001, CORRECTION-CODE-GEN-001, JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 entries)
- `internal/tsdb/migrations/011_*.sql` (constraint_outcomes schema)
- Live query against `mdemg-dev` TSDB — `constraint_outcomes` hypertable, 7d window
