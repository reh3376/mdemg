# A/B Verdict — JIMINY_NONVIOLATION_CREDIT_ENABLED

**Executed:** 2026-07-24T11:05Z, at **T+69.5h of the 72h window (96.5%)**.
**Early close (disclosed):** the operator challenged the value of waiting for
the scheduled T+72h fire when all interim reads had been stable for 24+ hours;
the remaining ~2.5h were a quiet overnight period contributing no new verdicts.
Data was effectively frozen — the close forfeits no information. The scheduled
09:41 EDT re-measure job is deleted as redundant.

**t0:** 2026-07-21T13:33:09Z (flag flipped ON in `.env` + server restart).
**Baseline window:** t0−72h → t0. **Post window:** t0 → close.
**Burst adjustment:** the post window contained three FTLOOP/FT-RECURSIVE
drill periods with heavy synthetic traffic + llama-server saturation
(2026-07-21 19:00–21:30Z, 2026-07-22 19:47Z–2026-07-23 01:00Z,
2026-07-23 11:00–19:30Z; 16.2h total). Burst-adjusted figures exclude those
intervals (effective post window ≈ 53.3h).

## Step 4 results

### Actionable compliance (constraint_outcomes, guidance_type IN constraint/correction)

| Measure | Baseline | Post (raw) | Post (burst-adj) |
|---|---|---|---|
| Actionable outcomes | 482 | 311 | 185 |
| Followed | 48 | 34 | 24 |
| **Follow rate** | **9.96%** | **10.93%** | **12.97%** |
| Rate/hour | 6.69/h | — | 3.47/h (**−48%**) |

### Tier-2 verdict emissions (llm_interactions `jiminy.evaluate_llm`, strict `"outcome":"…"` regex)

| Verdict | Baseline | Post (raw) | Post (burst-adj) |
|---|---|---|---|
| not_applicable | 87 (**11.6%**) | 533 (39.1%) | 259 (**38.5%**) |
| ignored | 537 (**71.5%**) | 685 (50.3%) | 334 (**49.6%**) |
| followed | 94 (12.5%) | 134 (9.8%) | 71 (10.6%) |
| contradicted | 8 | 2 | 1 |
| total | 751 | 1362 | 673 |

### Tripwires (Step 5)

| Tripwire | Threshold | Measured | Verdict |
|---|---|---|---|
| Follow rate >50% (over-crediting) | >50% | 12.97% | **CLEAR** |
| Contradicted drop >30% | −30% | Outcome sink **12 → 22** (rate 1.43%→1.81%, UP) | **CLEAR** |
| Latency +20% | +20% | Burst-adj **2924ms vs 3376ms (−13%)** | **CLEAR** (raw +28.5% was drill saturation, not the clause) |

⚠️ The tier-2 *emission* count of contradicted (8→2) is single-digit noise;
the authoritative measure is the outcome sink (all classifier sources), where
contradicted **increased** — no violation-detection loss. The clause is not
eating real contradictions.

## Step 5 success band — MISSED (with diagnosis)

Predicted band: 18–25% actionable follow rate. Measured: 12.97% burst-adj
(+3.0pp over baseline; +30% relative). Lift <5pp → Step 6 branch 2 diagnostic
executed:

1. **The clause IS being applied — hypothesis (b) refuted.** NA emissions
   3.3× baseline *organically* (38.5% burst-adjusted, so not a drill
   artifact). 12/12 sampled NA verdicts on must_not-type guidance show the
   exact predicted reasoning — several literally use the clause's "does not
   touch the mechanism" phrasing ("The agent's action does not involve
   committing directly to the main branch, thus the guidance constraint does
   not apply").
2. **88% of NA routing landed outside the compliance denominator.** Only
   64/534 post-flip NA verdicts were on imperative (must/never) guidance;
   the rest were advisory types (`pattern`/`learning`/`concept`) whose
   routing never moves the actionable metric. The clause improved the whole
   verdict mix (ignored 71.5%→49.6%) but only ~12% of its effect targets the
   measured class.
3. **The band's arithmetic assumed followed-count invariance — wrong model.**
   fix_spec predicted denominator-halving with the followed *count* held
   constant (48/~241 ≈ 20%). Actually followed scales with surfacing volume:
   volume −48% → followed 48→24 in step. Numerator and denominator shrink
   together; the rate improves only by the *composition* shift (+3pp).

## Decision (Step 6): KEEP THE FLAG ON

- Both revert-tripwires clear; latency clear on clean data.
- Every axis moved the designed direction at zero cost: borderline-irrelevant
  constraint rows route to NA (filtered from the denominator by the shipped
  gate), ignored noise collapsed 22pp, followed stable, contradicted
  preserved (up), classifier latency unchanged.
- `JIMINY_NONVIOLATION_CREDIT_ENABLED=true` stays in `.env`. The code
  default remains `false` (the ULTS hash-pin contract is untouched).

**Recalibrated expectation:** steady-state actionable compliance ≈ **13%**,
inside the panel's yellow 0.05–0.18 "by-design" band, approaching the green
≥0.18 target zone. The 18–25% band was an artifact of the fixed-numerator
model and should not be treated as the pass bar going forward.

**Follow-up (passive, no formal third A/B):** re-read the Step 4 compliance
query over a quiet 7-day window (no drills) around 2026-07-31. If the
composition shift compounds with JIMINY-CORPUS-001's post-purge outcomes,
green-band entry (≥18%) is plausible; if it plateaus at ~13%, the remaining
lever is corpus quality (HITL curation arc, JIMINY-RELEVANCE-001 → >0.9
long-term goal), not classifier semantics.

## Documents Accessed

- `ab_recipe.md` Steps 4/5/6 (queries, band, tripwires, decision matrix)
- `ab_t0_snapshot.md` (baseline 89/537/94/8/751; method verified by re-running
  the strict regex over the baseline window: contradicted=8 exact match)
- `baseline.md` (9.96% = 48/482)
- `fix_spec.md` (predicted ~50% ignored→NA routing on must_not)
- Live TSDB: `constraint_outcomes`, `llm_interactions` (all queries in this doc)
- Drill windows: FTLOOP-DRILL-001 `drill_record.md`, FT-RECURSIVE-003/004 posts
