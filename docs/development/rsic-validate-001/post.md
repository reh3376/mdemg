# RSIC-VALIDATE-001 — Sprint Close

**Date:** 2026-06-11 · **Roadmap:** Q3 Phase 2, #6 overall (first Phase-2 sprint)

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Plan + line-level confirmation of all 5 defects | `e712477` |
| 1–2 | Single-source criteria metrics (19 keys) + fail-closed for mutating actions | `371ca9f` |
| 3 | tombstone_stale correction-linkage scoping + refresh_stale_edges real decay | `9e3418e` |
| 4 | Counter-free confidence calibration (circular signal pollution ended) | `fe704fb` |
| 5–6 | Live Tier 3 + docs | (this) |

**The headline:** a real RSIC cycle now records `criteria_met=false` with
concrete per-criterion deltas — self-improvement evaluates evidence instead
of rubber-stamping itself. The rollback path is reachable for the first time.

**Follow-ups:** the RSIC cycle endpoint outlives client windows (~6 min) —
same config-driven-budget class as consolidate; calibration scores will
drift down as honesty lands (expected, not a regression);
`alert_embedding_regression`'s empty-call_sites collector still merits a look
if it persists post-honesty.

**Documents Accessed:** internal/ape/{calibration,cycle,task_spec,
task_dispatch,types_rsic}.go, internal/jiminy/{service,confidence_updater}.go,
internal/api/rsic_adapters.go, live history endpoint, roadmap.
