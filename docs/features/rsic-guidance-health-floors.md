# RSIC Guidance-Health Floors

**Shipped in:** DASHBOARD-TRUTH-003 (2026-08-01)

## Why

Two RSIC self-reflection patterns fire when `GuidanceHealth` (the honest windowed
follow-rate signal from `constraint_outcomes`) is too low:

- Pattern 9 `low_guidance_follow_rate` — "agent may be ignoring constraints"
- Pattern 15 `guidance_confidence_drift` — "per-constraint confidence may need
  calibration"

Their floors were hardcoded at 0.5 and 0.7 respectively — chosen when the
substrate was young. After JIMINY-CORPUS-001 (2026-07-03) purged the
half-junk constraint corpus and JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001
(2026-07-21) recalibrated the actionable-compliance denominator, the honest
steady-state GuidanceHealth on `mdemg-dev` settled at ~0.14 — well below
BOTH legacy floors, so both patterns fired essentially always. The insight
stream was chronically noisy; RSIC's reflection signal was drowned by two
"is-this-fine?" alerts against a healthy substrate.

## Choices

Three approaches considered:

1. **Bump the floors as hardcodes**. Cheap. But: the "correct" floor moves
   as the corpus quality and classifier semantics evolve — every future
   sprint that shifts the steady state would need a code edit. Reject.
2. **Delete the patterns**. Cleanest surface. But: the patterns exist for a
   reason — a genuine collapse (say, GuidanceHealth crashing to 0.02 for
   72h) SHOULD fire an insight. Reject.
3. **Extract to config with recalibrated defaults + opt-out at 0**. Adds two
   env knobs, but future-proofs against corpus recalibrations, preserves
   operator escape hatch, and disables cleanly if a future arc renders the
   signal unusable. **Chosen.**

## How it works

```
RSIC_GUIDANCE_HEALTH_FOLLOW_FLOOR (default: 0.20)
  ↳ pattern 9 fires when 0 < GuidanceHealth < floor
    (0-value means "no data" and never fires)

RSIC_GUIDANCE_HEALTH_DRIFT_FLOOR (default: 0.25)
  ↳ pattern 15 fires when 0 < GuidanceHealth < floor
```

- `floor > 0` gate — a floor of 0 disables the pattern entirely (operator opt-out).
- Insight description now includes the actual floor value at fire time (was
  hardcoded prose).
- Both floors independent — an operator can tune drift-detection
  aggressiveness separately from the compliance signal.

## How to use

Default (recommended, aligned with 2026-Q3 corpus + classifier state):

```bash
# .env — leave unset; the defaults 0.20 / 0.25 apply
```

Restore legacy semantics (if operating in a young/untuned space where the
substrate is expected to reach the 0.5-0.7 band):

```bash
RSIC_GUIDANCE_HEALTH_FOLLOW_FLOOR=0.5
RSIC_GUIDANCE_HEALTH_DRIFT_FLOOR=0.7
```

Fully disable both patterns (during a known-degraded window, e.g. mid-migration):

```bash
RSIC_GUIDANCE_HEALTH_FOLLOW_FLOOR=0
RSIC_GUIDANCE_HEALTH_DRIFT_FLOOR=0
```

Verify at boot: the config values echo into `/v1/config/dump` and appear in
the RSIC insight stream at fire time.

## Related sprints

- JIMINY-CORPUS-001 (2026-07-03) — set the modern steady-state band
- JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 (2026-07-21) — recalibrated the
  math the floor sits above
- DASHBOARD-TRUTH-001 / -002 / -003 — this line's shared architectural rule:
  "when a metric's steady state shifts, audit every hardcoded threshold that
  gates on it"
