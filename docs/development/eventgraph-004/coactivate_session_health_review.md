# CoactivateSession Post-Revival Health Review (EVENTGRAPH-003 Follow-up)

**Date:** 2026-06-10 · **Space:** `mdemg-dev` · **Window:** ~30h since the
dormancy fix went live (`b3e61cb`, binary started 2026-06-09 18:09:53Z)

Closes the EVENTGRAPH-003 follow-up: *"Investigate the revived
CoactivateSession at scale — now that it actually runs, watch session
co-activation volume + its effect on graph health (it was dead for the
project's entire history)."* All measurements taken against the live system
(TSDB SQL + Neo4j Cypher).

## Volume (post-fix)

| Metric | Value |
|---|---|
| `coactivate_session` rows | 399 |
| Active sessions | 2 (1 real Claude Code session, 1 verification probe) |
| Rate (real session) | ~13 rows/hr ≈ 1 per 5 min (tracks observation cadence) |
| Distinct pairs touched | 105 (avg 3.76 reinforcements/pair, max 5) |
| New-edge formation | 141/395 = 36% |
| Strengthening existing | 254/395 = 64% |

In the last 7 days `coactivate_session` is the **dominant Hebbian path**
(399 rows vs 21 for retrieve-driven `apply_coactivation`) — the EVENTGRAPH-003
fix didn't just revive *a* path; in this observation-heavy workload it revived
the *highest-volume* path (~95% of Hebbian event volume).

## Weight dynamics — healthy

- avg `delta_weight` **+0.0052** (small positive, correct Hebbian direction)
- avg `new_weight` **0.116**, max **0.193** — far below saturation (~0.9–1.0)
- max `evidence_count_after` **5** — no runaway counters
- A handful of tiny negative deltas (−0.0008) on early-life pairs: the Hebbian
  formula's small downward pull when the initial weight (0.1) already sits at
  the activation-product level. Self-limiting, not a defect.

## Edge-formation pattern — textbook session engram

The 15-observation session formed exactly a **15-node within-session clique**:
C(15,2)=105 undirected edges, every node degree 14 (28 directed relationship
records — each undirected edge is stored as two directed
`CO_ACTIVATED_WITH` rels). No cross-session leakage, no orphan stars, no
preferential attachment to old hubs.

## Whole-space anomaly sweep — clean

Across all 48,661 directed `CO_ACTIVATED_WITH` edges in `mdemg-dev`:
avg weight 0.18, max 1.0; **0 null weights, 0 negative weights**;
3 self-loops (0.006%); 136 near-saturation edges (0.28%). All negligible.

## Decisions

1. **No runtime tuning.** Volume is sustainable, dynamics healthy, edge
   formation bounded and predictable (C(n,2) per session). No rate-limiting,
   weight-capping, or session-size-capping justified by observed data.
2. **Pre-fix orphans stay as-is (operator decision, 2026-06-10).** 957
   `claude-core` conversation observations (2026-02-07 → 2026-06-08) have
   permanently zero session-coactivation edges — the historical record of the
   dormancy bug. Backfill was considered (full / 30-day partial) and rejected:
   synthetic clique edges would carry no real surprise factor, activation
   product, or co-activation timing — fake engram data muddying metrics just
   measured clean. The fix is forward-only, like every other latent-dormancy
   fix shipped on this project. (The 4,454 `uxts-module` orphans are test data
   and correctly edgeless.)
