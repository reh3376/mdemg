# RRF-SCALE-001 Epic 1 — Audit Findings

**Date:** 2026-06-03
**Method:** Full-repo grep for constant comparisons against post-RRF `RetrieveResult.Score` / `.Activation` / `.NormalizedConfidence` / score-derived confidence, plus live score-distribution sampling against `mdemg-dev`.

## Live score distribution (grounding data)

Sampled four strong constraint-matching queries (`top_k=10, candidate_k=50`) on the live RRF default scorer:

| Query | Top raw score | Results ≥ 0.55 | Top normconf |
|---|---|---|---|
| "never commit directly to main branch" | 0.495 | **0 / 10** | 100 |
| "no hardcoded values use config" | 0.556 | 2 / 10 | 100 |
| "always use CUIDv2 not UUID" | 0.519 | **0 / 10** | 100 |
| "mandatory testing tiers" | 0.582 | 1 / 10 | 100 |

**RRF strong-match top scores cluster 0.49–0.58.** The legacy `0.55` gate sits in the *middle* of the strong-match band — it rejects the single most relevant constraint for half the queries by a coin-flip margin.

Within-result-set normconf spread (query "never commit to main", all raw scores 0.505–0.506):

```
 score  normconf  name
 0.506    100.0   EmergentConcept-L3-corrections-commit
 0.506     88.9   EmergentConcept-L3-before-branch
 ...      ...
 0.505      0.0   Branch Change Reconciliation
```

**Critical:** `NormalizedConfidence` is a **positional percentile rank within the returned set** (`ApplyNormalizedConfidence`, scoring.go:982). When scores cluster (all ~0.505), it still spreads 100→0 by position, *not* by absolute relevance. A pure-percentile gate would therefore admit the top X% of *any* returned set — including all-noise sets. **This rules out plan Option A (percentile gate) as the sole mechanism.**

## Findings catalog

| # | Site | Current | Gates what | Reads RRF score? | Impact | Remediation |
|---|---|---|---|---|---|---|
| 1 | `consulting/service.go:1005` | `r.Score < 0.55` → skip | **constraint extraction** (the loop killer) | Yes | **HIGH** | Config-driven RRF-calibrated floor (default ~0.45) |
| 2 | `consulting/service.go:1081` | `r.Score > 0.6` | constraint confidence boost / authority | Yes | **HIGH** | Config-driven, scaled to RRF band |
| 3 | `consulting/service.go:1087` | `r.Score > 0.55` | constraint authority tier | Yes | **HIGH** | Config-driven, scaled to RRF band |
| 4 | `consulting/service.go:931` | `r.Score > 0.7` | deprecated-pattern conflict | Yes | **MED** | Config-driven conflict threshold |
| 5 | `consulting/service.go:944` | `r.Score > 0.6` | avoid/don't conflict | Yes | **MED** | Config-driven conflict threshold |
| 6 | `consulting/service.go:957` | `r.Score > 0.65` | naming-convention conflict | Yes | **MED** | Config-driven conflict threshold |
| 7 | `consulting/service.go:981` | `r.Score > 0.6` | sync/async contradiction | Yes | **MED** | Config-driven conflict threshold |
| 8 | `consulting/service.go:35-36` | `retrievalScoreMidpoint=1.5`, `Steepness=1.5` | confidence sigmoid (score→confidence) | Yes (derives from score) | **HIGH** | Recalibrate midpoint to RRF band (~0.45) + config-ify |
| 9 | `consulting/service.go:619` | `r.Score >= minConfidence` (`JiminyMinConfidence`, default 0.3) | noise pre-filter into `filteredResults` | Yes | **MED** | Already config-driven ✓; review default (0.3 keeps strong matches, drops 0.1 tail — adequate; consider 0.25 for headroom) |
| 10 | `retrieval/jiminy.go:45` | `breakdown.Activation > 0.01` | explanation: show activation factor | Activation (display only) | **LOW** | Display text only, no gating of guidance; config-ify or leave with note |
| 11 | `retrieval/jiminy.go:155` | `breakdown.Activation > 0.1` | explanation: rationale string | Activation (display only) | **LOW** | As above |
| 12 | `retrieval/jiminy.go:192` | `breakdown.Activation > 0.01` | explanation: score breakdown | Activation (display only) | **LOW** | As above |

### Classified NONE (not RRF-score consumers)
- `jiminy/service.go:2272` — `trial.Score < 9.0`: J17 protocol trial/interpretation score (0–10 comprehension scale), unrelated to retrieval scores.
- `jiminy/trust.go:133,136` — `entry.Score > 1.0 / < 0.0`: trust-score clamp to [0,1], a Jiminy-internal trust value, not a retrieval score.

## Remediation direction (decided)

**Mechanism: config-driven, RRF-calibrated absolute thresholds** (plan §5 Epic 2 Option B), chosen over Option A (percentile) on the basis of the distribution data above — percentile is positional and admits noise on uniform-score sets. Disclosure per `feedback_plan_options_pattern.md`.

Defaults derived from the live band:
- **Constraint/authority gates (#1–3):** strong matches live at 0.49–0.58, so a constraint floor of **0.45** admits them while rejecting the ~0.1 weak tail. Authority-tier boosts scale proportionally.
- **Conflict gates (#4–7):** conflicts are higher-confidence by design; default ~0.50 (still below the 0.55–0.70 legacy values, but above the constraint floor).
- **Sigmoid (#8):** midpoint 1.5 → **0.45**, steepness retained/tuned so a 0.5 raw score maps to ~0.6 confidence (was ~0.05 at midpoint 1.5).
- **Noise pre-filter (#9):** already config-driven; keep default 0.3.
- **Activation display (#10–12):** LOW — config-ify the thresholds for consistency (no-hardcoding), but these only affect explanation verbosity, not whether guidance surfaces.

**Secondary guard against noise:** the `minConfidence` pre-filter (#9, default 0.3) already establishes an absolute noise floor *before* the constraint gates run, so lowering the constraint gate to 0.45 cannot admit sub-0.3 noise. The two-stage filter (0.3 floor → 0.45 constraint gate) is retained.

**All thresholds become config-driven** (new `CONSULTING_*` env vars, Epic 2) with the RRF-calibrated defaults above. A CLAUDE.md "score-scale contract" note (Epic 5) records that these are RRF-scale-coupled and must be re-reviewed on any scorer change — the structural defense against a 4th instance of this bug class.

## Blast radius confirmed

The Jiminy `Guide` debug for "commit to main" showed `retrieval_found:10, suggest_constraints:0, suggest_suggestions:0, suggest_conflicts:0` — **the entire consulting output is empty**, consistent with findings #1–7 all over-gating simultaneously. Fixing the cluster revives constraints, suggestions, and conflicts together.
