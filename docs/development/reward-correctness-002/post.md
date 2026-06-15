# REWARD-CORRECTNESS-002 — Sprint Post

**Date:** 2026-06-15 · branch `reh3376_dev01` · training-integrity remediation
(the REWARD-CORRECTNESS-001 live-findings follow-ups).

## Outcome
Three reward/schema mismatches that graded CORRECT responses wrong are fixed and
live-validated. The benchmark/baseline now measures these tasks accurately —
the precondition for the honest baseline recompute.

## What shipped
- **Epic 1 — hidden.summarize schema `object`→`string`.** Production
  (`cluster_summarizer.go::Summarize`) emits bare prose ("Output ONLY the
  summary text … under 50 words"); the object schema mis-flagged 72 valid
  summaries as invalid-JSON. Verified: prose now validates as string (1.0) vs
  0.0 under the old object schema. (The reward — coherence+coverage — was
  already length-neutral from RC-001; this corrects the spec.)
- **Epic 2 — `explanation_quality` schema-aware.** jiminy.evaluate /
  jiminy.evaluate_llm nest reasoning in `violations[].reasoning`, not a
  top-level field, so the flat-only lookup scored every correct response 0.0.
  Now: top-level explanation → score it; else credit nested
  `violations[]/warnings[]` reasoning; a valid no-violation verdict is a
  correct "no issues" answer (nothing to explain, not penalised); falls back to
  the flat path for non-JSON / other schemas.
- **Epic 3 — keyword-bag `specificity_score`/`actionability_score`
  substantive-floored.** A substantive, non-hedging response floors at 0.7;
  the hard-coded indicator words are a bounded BONUS toward 1.0; hedging,
  empty, and pure-repetition stay low. `follow_rate` (their mean) inherits it.
  Preserves specific-beats-generic ordering while no longer dropping valid
  concise guidance for lacking the ~6 magic words.

## Live Tier 3 (real production rows, old → new kept @0.8)
| Task | old mean | new mean | old kept | new kept |
|------|---------:|---------:|---------:|---------:|
| jiminy.evaluate | 0.667 | 0.967 | 0/60 | **60/60** |
| jiminy.evaluate_llm | 0.967 | 0.967 | 60/60 | 60/60 |
| jiminy.synthesize | 0.725 | 0.879 | 3/60 | **59/60** |
| ape.reflect | 0.848 | 0.956 | 47/60 | 60/60 |

New means 0.88–0.97 = genuinely-correct production output scoring correctly, no
over-inflation (the 1/60 jiminy.synthesize + a few ape.reflect still <0.8 are
degenerate). 87 reward unit tests + 609 neural tests + ruff green.

## Notes
- The keyword-bag functions are also used by ape.reflect + consulting.synthesis
  (actionability); the floor gave ape.reflect a small correct lift (its real
  blocker was the now-fixed truncation) and no absurd inflation.
- jiminy.synthesize capture (if ever distilled) can pair the new functions with
  REWARD-CORRECTNESS-001's per-task `--reward-threshold-map` to gate at its
  reward ceiling. Neither jiminy.evaluate nor jiminy.synthesize is a current
  distill target — this sprint's value is correct BENCHMARK grading.

## Carried forward
The honest baseline recompute (now unblocked: corpora are sound — corrupt rows
pruned, truncation fixed, reward grading correct).

## Documents Accessed
The 4 ULTS specs; `neural/training/reward_functions.py` + tests;
`internal/hidden/cluster_summarizer.go`; REWARD-CORRECTNESS-001 `live_findings.md`;
live TSDB rows for the 4 tasks.
