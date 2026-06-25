# Sprint Plan — HIDDEN-CHURN-003

## 1. Header & Metadata
- **Sprint ID:** HIDDEN-CHURN-003
- **Line:** `docs/development/hidden-churn-003/`
- **Date opened:** 2026-06-25
- **Target version:** v0.11.2 (patch — substrate-stability completion)
- **Estimated effort:** ~1–1.5 dev-days
- **OpenAI spend:** $0
- **Risk:** Medium (changes the default consolidation hidden-step write path; mitigated by mirroring the live `assignNoiseToThemes` pattern + a config fallback to the CHURN-002 full path)

## 2. Problem Statement
HIDDEN-CHURN-002 is the committed **partial** fix (must-fix obligation in its `post.md` + CMS): churn 100%→~25%/cycle, but the residual ~25% is structural — the hidden step **re-clusters all ~52k L0 nodes from scratch every cycle**, so KMeans jitter + non-deterministic LLM reclassification reshuffle membership and ~25% of patterns fail to match. That orphans ~25% of reinforcement/abstraction edges per cycle and drives sustained Neo4j High CPU (every completed cycle is a full ~10-min re-cluster). This sprint closes it.

## 3. Scope & Constraints
**In scope:**
- **Incremental hidden-layer clustering** as the default consolidation hidden step: fetch only **orphan** L0 nodes (no `GENERALIZES`→`HiddenPattern` edge), assign each to its nearest existing pattern (Go-side cosine ≥ `HIDDEN_INCREMENTAL_ASSIGN_SIM_THRESHOLD`) with a `GENERALIZES` edge + affected-pattern centroid recompute; cluster only the **unassigned remainder** into new CUIDv2 patterns. Existing patterns are **never destroyed or re-clustered** → ~0% per-cycle churn.
- Gate `HIDDEN_INCREMENTAL_ENABLED` (default **true**); false falls back to CHURN-002's full `CreateHiddenNodes`.
- Demote full re-cluster to an **explicit operator command** (`mdemg concepts recluster --space-id`), off the auto cycle.
- On success, **close the must-fix obligation** (docs + CMS).

**Out of scope:**
- Re-assigning *changed* (re-ingested) L0 nodes — orphan is the v1 input; changed-node re-assignment via the explicit re-cluster command (documented).
- Cluster-quality maintenance (split oversized / merge near-duplicate) — follow-up if drift observed.
- Conversation themes / L2+ concepts — already stabilized / out of scope.

**Constraints:** sequential epics; Tier-3 live required; CUIDv2; protected `mdemg-dev` safety (incremental path never deletes patterns); no-hardcoding.

## 4. Dependencies
- HIDDEN-CHURN-002 (`hidden_identity.go`: `listHiddenPatternRefs`, `hiddenPatternRef`, CUIDv2 mint, completion fix) — on `main`.
- HIDDEN-CHURN-001 `assignNoiseToThemes` (incremental-assignment template).
- `cosineSimilarity`, `ComputeCentroid`, `memberEdgePairs` (`cuid2`).

## 5. Implementation Plan (sequential epics)

**Epic 0 — Plan + fact-verify (done):** no `fetchOrphanBaseNodes` exists (write it); Go-side nearest over ~4k patterns is performant; no new vector index needed.

**Epic 1 — Incremental helpers** (`hidden_identity.go`): `fetchOrphanBaseNodes` (= `fetchAllBaseNodes` + `WHERE NOT (b)-[:GENERALIZES]->(:HiddenPattern {layer:1})`); `assignOrphansToPatterns` (Go-side nearest-pattern by cosine ≥ threshold → batch `GENERALIZES` edges, CUIDv2 edge ids, cosine weights, mirroring `assignNoiseToThemes`) + affected-pattern centroid recompute; `hiddenIncrementalAssignThreshold()`. Unit tests.

**Epic 2 — `IncrementalHiddenNodes` + wiring:** list patterns → fetch orphans → assign to nearest → KMeans the unassigned remainder into new CUIDv2 patterns (reuse `createHiddenNodeWithEdges`) → **no delete**. `hiddenStep.Run` dispatches on `HIDDEN_INCREMENTAL_ENABLED`. `mdemg concepts recluster` runs the existing full `CreateHiddenNodes`.

**Epic 3 — Tier 1+2 tests:** unit (nearest, threshold, centroid, remainder-cluster); integration (double-ingest → 0% churn + no deletes + correct assignment).

**Epic 4 — Tier 3 live:** mdemg-dev, 2 incremental cycles → ~100% survival (vs 75%), consolidation CPU/wall-time drop, gauge steady + alerts clear.

**Epic 5 — Docs + close obligation:** feature doc, CLAUDE.md (residual→remedied), CHANGELOG, post.md, ROADMAP §5 flip, CMS task resolved.

## 6. Testing Plan (3 tiers)
- **Tier 1:** assignment/threshold/centroid/remainder unit tests.
- **Tier 2:** incremental double-ingest integration — 0% churn + no deletes + correct assignment.
- **Tier 3:** live ~100% survival + CPU/wall-time reduction + alerts clear.

## 7. Commit Strategy
Sequential per-epic commits on `reh3376_dev01`; live-smoke surprises get own fix-commits; final commit promotes CHANGELOG + flips the must-fix notes to remedied.

## 8. Verification Checklist
- [ ] Incremental path touches only orphan L0; existing patterns never destroyed/re-clustered
- [ ] Live: ~100% pattern id survival (from ~75%)
- [ ] Live: consolidation wall-time / Neo4j CPU materially lower
- [ ] New patterns CUIDv2; assignment edges carry cosine weight + CUIDv2 edge id
- [ ] `HIDDEN_INCREMENTAL_ENABLED=false` falls back to CHURN-002 full path
- [ ] `mdemg concepts recluster` runs the explicit full re-cluster
- [ ] graph_node_drop / orphan / High-CPU alerts clear
- [ ] lint + config-guard clean; must-fix obligation flipped to remedied (docs + CMS)

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Incremental-only clustering drifts (bloat) over time | Med | Med | Explicit `concepts recluster`; split/merge follow-up if observed |
| Assign threshold mis-tuned (low→bad clusters / high→pattern explosion) | Med | Med | Config-tunable; Tier-3 calibration; default ≈0.80 (mirrors theme assign) |
| Changed (re-ingested) L0 not re-assigned | Low | Low | Documented; explicit re-cluster covers it; orphan path covers all new nodes |
| Fallback rot | Low | Low | `HIDDEN_INCREMENTAL_ENABLED=false` keeps CHURN-002 path live + tested |

## 11. Documents Accessed
- `internal/hidden/service.go` (`CreateHiddenNodes`, `fetchAllBaseNodes`), `hidden_identity.go`, `theme_identity.go` (`assignNoiseToThemes`), `step_hidden.go`, `clustering.go` (`cosineSimilarity`/`ComputeCentroid`), `types.go` (`BaseNode`)
- `migrations/` (vector indexes — confirmed reuse, no new index), `internal/config/config.go`
- Live mdemg-dev (pattern counts/ids, consolidation timing, alerts)

## 12. Rollback Procedures
`HIDDEN_INCREMENTAL_ENABLED=false` instantly reverts to the CHURN-002 full path (no logic redeploy). Revert Epic 2 commit to remove the incremental path; existing patterns + edges remain valid.
