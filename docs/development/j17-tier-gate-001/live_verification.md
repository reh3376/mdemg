# J17-TIER-GATE-001 — E4 Live Tier-3 Verification

**Date:** 2026-07-21 | **Space:** `mdemg-dev` | **Result: first T1 messaging ever produced live**

## Procedure

1. `.env`: `J17_TIER_GATE_MODE=comprehension` + temporary `J17_TIER_COMPREHENSION_MIN_SAMPLES=1` (smoke only) → rebuild + restart.
2. Boot log: `j17: tier gate configured mode=comprehension comprehension_high=0.6 min_samples=1 provider_wired=true` ✓
3. In-process comprehension started at **0.000 (15 events)** — honest cold-start: post-restart events were tier1-band ignoreds (compScore 0) + non-constraint items; the gate correctly withheld T1 (encoded output showed T2 lines).
4. Drove 3 real `/v1/jiminy/feedback` rounds with constraint-related actions (dev-branch commits + CUIDv2 usage) against live guidance `zfr4cniax4i8jgtar3moqdl5`. Outcomes: 7× `followed`, 2× `ignored`, 6× `not_applicable` (the JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 clause visibly routing unrelated items in the same verdict batches).
5. `avg_comprehension → 1.0` (≥ 0.6 floor, samples ≥ min) via `/v1/jiminy/protocol/metrics`.
6. Fresh warm → `/v1/jiminy/latest` `encoded_augmentation`:

```
═══ JIMINY GUIDANCE ═══
C:!|auto-250af3293675
C:!|use-cuidv2-by-default
C:!|uuid-precedent-divergence
C:!|must-use-cuid2
C:!|jiminy-uuid-cuid2-enforce
K: EmergentConcept-L4-cuidv2-requirement-424: ... (T2)
...
T1-format lines detected: 6
```

**Six T1 pipe-coded lines** (`TYPE:SEV|code` — ~5-15 tokens each) replacing what were ~40-80-token T2 lines. Uncoded abstractions correctly remain T2/T3.

7. Restored `J17_TIER_COMPREHENSION_MIN_SAMPLES` to default (removed temp line; mode stays `comprehension`) + restart. Boot log: `min_samples=20` ✓.

## Causal chain proven end-to-end

real feedback → NLI/heuristic comprehension recorded (`RecordOutcomeWithTier`) → provider (`protocolMetrics.Snapshot().AvgComprehension/TotalEvents`) → `selectTier` comprehension branch → `TierCoded` → `formatT1` pipe encoding in the delivered augmentation.

## Discovered during smoke (documented, no code change)

- **In-process comprehension is composition-sensitive, not just count-sensitive**: 15 samples of tier1-band ignoreds read 0.000 regardless of MIN_SAMPLES. The conservative default (20) plus the 0.6 floor means **post-restart T1 re-locks until ~an hour of real traffic accumulates** — the same in-memory-reset property as all J17 protocol stats. By design (cold-start safety); operators should expect T1 to "warm up" after each restart.
- **Steady-state comprehension sits near the floor**: NLI scores observed 0.61-0.72; 48h historical avg 0.732 vs floor 0.6 — some flap risk at the boundary. `J17_TIER_COMPREHENSION_HIGH` is the tuning knob (lower to 0.5 if flap observed; the T1→T2 demotion gate remains the safety net).
- **Compression follow-on**: with T1 flowing, `mdemg_j17_compression_ratio` should climb from ~1.7 toward 3-5× as T1-encoded guidance accumulates in new protocol events — measurable over the coming hours/days on the J17 dashboard.

## Final state

`.env`: `J17_TIER_GATE_MODE=comprehension` (persistent, operator-authorized); MIN_SAMPLES default 20; demotion gate untouched.
