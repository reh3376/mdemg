> ⚠️ **TRAJECTORY ANNOTATION (added 2026-08-12 by FRAMING-HYGIENE-SWEEP-001):** the framing in this doc calls the current follow-rate "honest steady state" / "not urgent" / "expected". That framing was **superseded** by the operator directive of 2026-08-11 ("If the main LLM is only following 10-13% of Jiminy's guidance this entire project is a complete failure") — the arc that owns the ≥80% target is [`docs/development/jiminy-ceiling-break-2/`](../jiminy-ceiling-break-2/README.md). Sprint history preserved below for context; do NOT act on the "not urgent" / "by design" conclusions in the body — those conclusions are wrong per the current architectural directive.

---

# JIMINY-FOLLOW-RATE-REMEASURE-001 — Verdict

**Date measured:** 2026-08-08
**Window:** 7d rolling on mdemg-dev
**Sample:** 1618 actionable outcomes (guidance_type in constraint|correction, outcome_type != not_applicable)
**Baseline:** 2026-07-29 CEILING investigation §2 headline — 472 actionable, 54 followed, **11.44%**

## Headline

**Follow rate: 11.62%** (188 followed / 1618 actionable)

**Vs baseline: +0.18 percentage points, effectively flat despite 3.4× traffic volume.**

## Deltas per prediction

| Prediction (source) | Predicted | Actual (2026-08-08) | Delta |
|---|---|---|---|
| CEILING §6 recommendation lift from JIMINY-CLASSIFIER-CONTEXT-001 | +24–39pp → 35-50% | +0.18pp → 11.62% | **MISSED by ~25pp minimum** |
| CLAUDE.md recalibrated pin (post JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001) | ≈13% steady-state | 11.62% | −1.4pp from center, within 1σ noise |
| Alert floor `GUIDANCE_SHOULD_FOLLOW_RATE_FLOOR` (post FOLLOW-RATE-CALIBRATE-001) | 5% floor for alert-not-fire | 11.62% | ×2.3 above floor → healthy per alert semantics |

**Verdict:** the recalibrated ≈13% pin is right. The CEILING investigation's original 35-50% recommendation was over-optimistic — the sub-sprint that shipped (JIMINY-CLASSIFIER-CONTEXT-001) already recorded this in its own A/B verdict (predicted 18-25% band missed at 12.97%). This remeasure confirms that story stabilized at 11.6% ≈ 13%.

## Breakdown 1 — classifier_source (verify tier1 bypass is working)

| classifier_source | n | followed | rate |
|---|---|---|---|
| llm | 1479 | 188 | 12.71% |
| heuristic | 128 | 0 | 0.00% |
| explicit | 10 | 0 | 0.00% |
| correction_observed | 1 | 0 | 0.00% |

- Tier1 bypass **is** routing (zero `tier1` rows for follow/ignore decisions — matches shipped behavior).
- LLM path 91.4% of actionable volume — the classifier is doing the work as designed.
- Heuristic 7.9% is the "LLM unavailable / saturated" burst-class rows, all correctly ignored.

## Breakdown 2 — guidance_type (verify correction-code shipment lifted correction follows)

| guidance_type | n | followed | rate |
|---|---|---|---|
| constraint | 1146 | 132 | 11.52% |
| correction | 472 | 56 | 11.86% |

- Constraint and correction rates within 0.34pp — no differential lift.
- CORRECTION-CODE-GEN-001 (backfilled 35 corrections with codes) did what it was designed to do (attribution + telemetry consistency), NOT lift follow rate. Attribution ≠ compliance.

## Breakdown 3 — time-slice (verify stability)

| window | n | followed | rate |
|---|---|---|---|
| d7 → d3 ago | 880 | 101 | 11.48% |
| last 3d | 738 | 87 | 11.79% |

- +0.31pp drift across the 7d window. Well within noise. Not trending up, not trending down.

## Why 35-50% didn't materialize

The CEILING investigation predicted that JIMINY-CLASSIFIER-CONTEXT-001's context-mismatch credit would move ~50% of the `ignored` verdicts on real rules to `not_applicable`, which the writer gate then strips from `constraint_outcomes`. That would shrink the denominator (1618 → ~800) and roughly double the follow rate (11.6% → ~23%).

The CLAUDE.md pin from that sprint's own A/B recorded WHY the shrink didn't happen at that magnitude:

> "88% of NA routing landed on advisory guidance outside the actionable denominator (64/534 imperative); followed count scales with surfacing volume (48→24 as volume −48%)."

Translation: the LLM classifier is calling `not_applicable` freely — but it's calling it on the abstract/advisory guidance that WASN'T in the actionable denominator to start with. On the imperative constraints and corrections that WERE in the denominator, NA routing is rare. So the denominator shrink was too small to move the actionable rate.

## Remaining ceiling — the corpus lever

With the classifier arc effectively closed at 11.6%, the remaining lever is **corpus quality**:

- Prune more "constraints" that aren't actually durable rules
- Extend the constraint-promotion reject-patterns (JIMINY-CORPUS-003 shape) as new junk classes surface
- Ship HITL-CURATION-003 to accelerate operator-graded rule tombstones
- Investigate why LLM classifier reads so many imperative constraint contexts as "ignored" rather than "not_applicable" — the CLAUDE.md pin says the classifier is under-routing NA on imperatives specifically; that's a prompt-engineering follow-up, not a systemic classifier defect

## Recommendation

**No CLAUDE.md update needed.** The recalibrated ≈13% pin from COMPLIANCE-CREDIT already predicted this result correctly.

**Next-lever candidate for a future sprint:**
- **HITL-CURATION-003** — direct corpus-quality lever, corpus is now the bottleneck
- **JIMINY-CLASSIFIER-CONTEXT-002** — investigate WHY NA-routing over-fires on advisory + under-fires on imperative; a prompt tightening could unlock the shrink the original design intended

Neither is urgent — 11.6% is stable, within noise of the recalibrated pin, and above the 5% alert floor. The 90% target the operator originally named remains genuinely unreachable under current measurement semantics without corpus-quality investment.

## Query used (reproducible)

```sql
-- headline
SELECT
  count(*) FILTER (WHERE outcome_type != 'not_applicable') AS actionable_total,
  count(*) FILTER (WHERE outcome_type = 'followed')       AS followed,
  count(*) FILTER (WHERE outcome_type = 'ignored')        AS ignored,
  round(100.0 * count(*) FILTER (WHERE outcome_type = 'followed') /
        NULLIF(count(*) FILTER (WHERE outcome_type != 'not_applicable'), 0), 2) AS follow_rate_pct
FROM constraint_outcomes
WHERE time > now() - interval '7 days'
  AND guidance_type IN ('constraint', 'correction')
  AND space_id = 'mdemg-dev';

-- classifier_source breakdown
SELECT classifier_source, count(*), count(*) FILTER (WHERE outcome_type = 'followed')
FROM constraint_outcomes
WHERE time > now() - interval '7 days'
  AND guidance_type IN ('constraint', 'correction')
  AND space_id = 'mdemg-dev'
GROUP BY classifier_source ORDER BY 2 DESC;
```

Re-runnable at any time via `docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -c "..."`.
