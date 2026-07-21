# JIMINY-ACTIONABILITY-INVERSION-001 — Sprint Post

**Shipped:** 2026-07-21 | **Branch:** `reh3376_dev01` | **PR:** (pending push)

## What shipped

Investigation-only sprint (no code changes). Diagnosed the DASHBOARD-TRUTH-002 triage's suspicious finding: **advisory guidance is followed at ~1.6× the rate of actionable guidance**, inverting the Should-Follow panel's premise.

## Verdict

**Not a bug.** The inversion is real but arises from correct interaction of three intentional design elements:

1. **Constraint semantics**: imperative "NEVER…" language. Actions that don't violate but don't demonstrate applying the rule get correctly judged `ignored`.
2. **Lever C over-surfacing** (dominant driver): `JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED` surfaces constraints at **22.7 events per unique ID** vs 9.9 for advisory (2.3× denominator inflation).
3. **LLM classifier's correct must-vs-should calibration**: symmetric prompt, but the invoked distinction (`must/must_not = strict; should/should_not = flexible`) produces asymmetric follow rates on constraint vs advisory items.

## Epics (all committed)

- **E0** — Sprint plan (bundled `40d55fa`)
- **E1-E7** — Investigation (`eab2917`)
  - E1 inversion reproduced (24h/7d/30d stability)
  - E2 hypothesis 1 (classifier asymmetry) — PARTIALLY CONFIRMED, correct behavior
  - E3 hypothesis 2 (4-band restoration) — MINOR CONTRIBUTOR
  - E4 hypothesis 3 (Lever C surface bias) — STRONGLY CONFIRMED
  - E5 hypothesis 4 (duplication) — SAME AS H3
  - E6 hypothesis 5 (prompt bias) — REFUTED
  - E7 root cause verdict + fix spec written
- **E8** — Canonical docs (this commit)

## Key evidence

```
30d follow rate by type (mdemg-dev):
constraint     10.0%   ← lowest actionable
correction     20.3%
learning       11.9%
pattern        21.3%   ← highest advisory

Actionable (constraint+correction):  10.4%
Advisory   (pattern+learning+concept+risk): 16.3%

Events per unique ID (30d):
constraint    22.7   ← 2.3× more than advisory
pattern        9.9

not_applicable rows in constraint_outcomes: 0
  ↑ correctly gated out of both TSDB and Neo4j writes
    (service.go:1730,1762)
```

## Side-finding worth pinning

`constraint_outcomes` is a FILTERED view — no `not_applicable` rows because both write paths (`PersistGuidanceOutcome` for Neo4j edges, `outcomeWriter` for TSDB) explicitly gate on `outcome != OutcomeNotApplicable`. This is intentional (topically unrelated items shouldn't decay constraint confidence) and the follow-rate denominator is thus ALREADY excluding the sub-0.10 not-applicable band. Any future work reading `constraint_outcomes` must remember it's outcome-typed only, not the full classification distribution.

## Fix spec highlights (deferred)

Three fixes ranked by leverage-to-risk:

1. **Fix 1 (zero risk)**: Rename Should-Follow → Actionable Compliance Rate + description reframe. Shippable as small doc-only sprint or folded into DASHBOARD-TRUTH-003.
2. **Fix 2 (HIGH leverage, LOW risk)**: Extend `classifySystemPrompt` with a "non-violation credit for must_not" rule. Predicted: routes ~50% of current `ignored` constraint rows to `not_applicable` (which the shipped gate filters out of the denominator), lifting constraint follow rate 10% → ~20%, closing the inversion. Spec'd as follow-up sprint **JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001** (~1.5 dev-days).
3. **Fix 3 (NOT recommended)**: reduce Lever C top-K. Cuts coverage; Fix 2 achieves same result without quality tradeoff.

## What I did NOT do

- No code changes (investigation sprint by design).
- No prompt edits (deferred to JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001).
- No panel edits (deferred; Fix 1 will land in a follow-up).
- No A/B tests (deferred; needed for Fix 2 before default-flip).

## Deviations

None. Plan executed as written.

## Rollback

- N/A (no code/data changes).

## Next up

Per sweep queue: **FT-BENCH-REFRESH-001** (re-run benchmark on GGUF endpoint + wire staleness detection), then **PROMETHEUS-SCRAPE-INVESTIGATION-001** (diagnose /metrics HTTP 404).
