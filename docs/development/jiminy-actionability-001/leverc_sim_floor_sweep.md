# Lever C — Tuning: relevance-balanced (ENABLED) + the >90%-actionable sweep

Two operator goals were explored. **The relevance-balanced config below is what is enabled live** (the >90% config is retained for reference — it hit the target but padded with moderately-relevant constraints and suppressed all context).

## ✅ ENABLED LIVE — relevance-balanced config

```
JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED=true
JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK=5
JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR=0.70        # only HIGHLY-relevant constraints
JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION=0.3       # guarantee the relevant constraints surface (~3 slots)
JIMINY_SURFACE_ACTIONABLE_WEIGHT=1.5             # rank actionables a touch higher
# NO abstraction cap → relevant patterns/learnings stay as context
```

**Live-measured (6 contexts): 25% actionable** (9 constraint / 36), 75% relevant context (13 pattern + 14 learning). Per-query 0–40%, varying with context (no padding to a target). **Relevance: every surfaced item was on-topic** — e.g. the "TSDB migration / bump schema version" context surfaced 2 TimescaleDB/schema constraints + patterns/learnings about `003_metric_types.sql`, `tsdb_schema_meta`, and schema-version tracking. This is the intended balance: the highly-relevant constraints (floor 0.70) surface via the modest quota, alongside relevant context.

**Tune knobs:** raise `MIN_ACTIONABLE_FRACTION` for more actionable weight; lower `SIM_FLOOR` (toward 0.6) to admit more constraints; add a `MAX_ABSTRACTION_FRACTION` cap to bound context.

---

## Reference — the >90%-actionable config (NOT enabled)

Operator goal: surfaced guidance **>90% actionable**. Achieved but with the tradeoffs below; superseded by the balanced config above.

## Config that hits the target

```
JIMINY_GUIDANCE_CONSTRAINT_BIAS_ENABLED=true      # Lever C: inject actionable candidates
JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK=15        # provide plenty (max_items is 10)
JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR=0.60         # sweet spot (see sweep)
JIMINY_SURFACE_MAX_ABSTRACTION_FRACTION=0.1       # Lever A: cap abstractions ≤10%
JIMINY_SURFACE_MIN_ACTIONABLE_FRACTION=0.9        # Lever A: reserve ≥90% for actionable
```

Lever C guarantees the pool has ≥10 actionable candidates; Lever A's cap+quota then force the surfaced 10-item set to be ≥90% actionable. **Live-measured: 100% actionable** (10/10 constraints) across every test query, incl. an unseen one.

## SIM_FLOOR sweep (Lever C + Lever A cap/quota, INCLUDE_TOPK=15)

| SIM_FLOOR | Surfaced actionable fraction | Items/query | Notes |
|---|---|---|---|
| 0.30 | **100%** | 10 | many marginal constraints admitted |
| 0.40 | **100%** | 10 | |
| 0.50 | **100%** | 10 | mixed relevance (some padding) |
| **0.60** | **100%** | 10 | **highest floor still filling 10 — best relevance** |
| 0.70 | 66% | 3 | only highly-relevant constraints clear, but <9 → below target |

The cliff is between 0.60 and 0.70: at 0.70 only the genuinely-on-topic constraints (cosine ≥0.7) survive, but there are fewer than 9 per query, so the surfaced fraction drops to ~66%. **0.60 is the highest floor that still holds >90%**, so it maximizes relevance among the target-hitting settings. (Floors <0.30 are moot — `JIMINY_MIN_CONFIDENCE=0.3` cuts them.)

## ⚠️ Honest tradeoff (the >90% target has a cost)

1. **100% actionable fully suppresses abstraction context.** With the cap at 0.1, patterns / learnings / decisions essentially never surface — the agent sees only constraints/corrections. That is the literal meaning of ">90% actionable," but it removes useful background context.
2. **Quantity vs. relevance.** This substrate doesn't have ≥9 *highly*-relevant (cosine ≥0.7) constraints per arbitrary context. To fill 10 actionable slots at floor 0.60, some constraints are **moderately** relevant, not all bullseye. The highly-relevant subset is ~2–3/query (the floor-0.70 result).

**If you want balance instead of maximal actionability:** raise `SIM_FLOOR` toward 0.70 (fewer, highly-relevant constraints) and/or relax `MAX_ABSTRACTION_FRACTION` toward 0.3 (≈70% actionable + 30% context). The shipped config above is tuned strictly to the >90% directive.

## Status
Enabled live (`.env`). The merged code keeps Lever C **default-off**; this is the operator's runtime opt-in. Outcome-side follow-rate (the multi-week signal) will now begin accruing in `constraint_outcomes`.
