# Sprint Plan HIDDEN-CHURN-001 — Stable Concept Identity (PR-A) + Coverage (PR-B)

## 1. Header & Metadata
- **Sprint ID:** HIDDEN-CHURN-001 — Q3 Phase 2, ranked #8 · ~6d budgeted
- **Line:** `docs/development/hidden-churn-001/` · **Date:** 2026-06-11 · **Branch:** `reh3376_dev01`
- **Delivery:** TWO PRs (disclosed): **PR-A** structural identity + emergence-skip fixes (this window); **PR-B** coverage retune + childless-L2 repair + `concepts trace` (follow-on)

## 2. Problem Statement (live-verified tonight)
1. **Theme identity churn:** `ClusterConversations` detaches ALL
   observation→theme edges, deletes childless themes, and re-creates
   every theme from scratch each cycle (~5 min) — new `node_id`s every
   run. Evidence chains referencing themes are destroyed continuously;
   recall surfaces stacks of near-identical "Emerging pattern: …"
   concepts (observed in this session's own prompts).
2. **Automated consolidation silently skips LLM emergence:**
   `dynamicEmergenceStep.Phase() = 22`, but `RunConsolidation` runs
   phase ranges **10–20** then **25–30** — phase 22 falls in the gap.
   The manual path (`RunNodeCreationPipeline`) correctly runs 10–22 with
   an emergence skip-flag; the automated path hardcodes its own ranges.
3. **Coverage gap (PR-B):** ~318 themed vs ~5,300 unthemed observations
   (≈94%); `maxThemes = ceil(n/10)` and min-samples are inline
   equations; sub-threshold clusters are dropped as noise.
4. **Childless L2s (PR-B):** 10,395 live (grew from the audit's 9,687).

## 3. Scope & Constraints
**PR-A (in):** centroid-matched theme identity (reuse an existing theme
when a new cluster's centroid cosine ≥ `HIDDEN_THEME_IDENTITY_SIM_THRESHOLD`,
default 0.90 — update in place, preserving node_id/edges/references;
theme-scoped edge rewiring replaces the global detach; only themes
matched by NO cluster are deleted); `RunConsolidation` delegates to
`RunNodeCreationPipeline(…, cfg.EmergenceEnabled)` (single range source —
phase 22 included, gated on config); Tier 1 + live Tier 3 (two
consecutive consolidations → stable node_ids; emergence step visibly
executes when enabled).
**PR-B (declared, follow-on):** config-driven maxThemes/min-samples +
density-based assignment of sub-threshold clusters + coverage gauge/alert;
childless-L2 repair/tombstone; `mdemg concepts trace`; the serve.go
consolidation-related hardcode (original line ref drifted — re-locate).
**Out:** DBSCAN ceiling (deferred per spec; tripwire is TSDB-CONSUME-001);
L5 emergent-node semantics.

## 4. Dependencies
HIDDEN-WEIGHT-001 (real edge weights — matching/update math meaningful);
`cosineSimilarity` + `ComputeCentroid` (exist); EMERGENCE_ENABLED config.

## 5. Implementation Plan (PR-A)
Epic 0 investigation (done). Epic A1: emergence-skip fix (single range
source). Epic A2: centroid-matched identity in ClusterConversations.
Epic A3: live Tier 3 — consolidate twice, prove identity stability +
emergence execution. Epic A4: docs + close (PR-B plan affirmed).

## 6. Testing Plan
Tier 1: centroid-match selection (match/no-match/threshold/claimed),
range-source test pinning phase 22 inclusion. Tier 2: hidden suite +
EXPLAIN for new Cypher. Tier 3: live double-consolidation node_id
stability + emergence-step execution log; recall sanity after.

## 7. Commit Strategy
One commit per epic; surprises standalone; push → auto-PR (PR-A) → summary.

## 8. Verification Checklist (PR-A)
- [ ] Two consecutive live consolidations keep matched theme node_ids
- [ ] Edges/evidence on matched themes survive a cycle
- [ ] Phase 22 executes under EMERGENCE_ENABLED=true via the automated path
- [ ] Unmatched-theme deletion still prevents orphan accumulation
- [ ] Suites green; lint clean; docs updated

## 9. Documentation Update — Epic A4 (+ PR-B carries its own).

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Centroid drift merges distinct themes over time | M | M | Threshold 0.90 conservative + config; PR-B's trace tool audits groundedness |
| In-place updates fight the 5-min cadence | L | M | Same lock as today; rewiring is theme-scoped (smaller txns than global detach) |
| Emergence (LLM) slows automated consolidation | M | L | Gated on EMERGENCE_ENABLED (default false); budget patterns from GUIDANCE-SYNTH apply |

## 11. Documents Accessed
internal/hidden/{service.go (ClusterConversations 4190-4480, RunConsolidation
1600-1700, RunNodeCreationPipeline 310-325), step_dynamic_emergence.go,
clustering.go, pipeline.go}; live Neo4j counts; roadmap; session recall
evidence (duplicate concepts).

## 12. Rollback
Revert PR-A — returns to churn (prior state); created themes remain valid;
no migration.
