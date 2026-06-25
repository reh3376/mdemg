# Sprint Plan — HIDDEN-CHURN-002

## 1. Header & Metadata
- **Sprint ID:** HIDDEN-CHURN-002
- **Line:** `docs/development/hidden-churn-002/`
- **Date opened:** 2026-06-24
- **Target version:** v0.11.1 (patch — substrate-stability bugfix)
- **Estimated effort:** ~1 dev-day
- **OpenAI spend:** $0 (no LLM in scope)
- **Risk level:** Medium (mutates the live consolidation write path on the protected `mdemg-dev` substrate; mitigated by mirroring the already-live HIDDEN-CHURN-001 pattern + small-batch live verification)

## 2. Problem Statement
The hidden-pattern abstraction layer is **destroyed and recreated every ~5-min consolidation cycle**: ~2,636 `HiddenPattern` MemoryNodes (`role_type='hidden'`, layer 1) plus their ~31,106 `GENERALIZES` edges are wiped and rebuilt with fresh `randomUUID()` ids. Live-confirmed in TSDB (`mdemg_neo4j_graph_nodes`: steady 81,033 → **−2,676 at 21:13** → rebuilt within 4 min).

Consequences:
- Fires the **CRITICAL `graph_node_drop`** alert (true positive) and the **MEDIUM `High Orphan Ratio`** alert.
- Churns node identity → any `CO_ACTIVATED_WITH` / `reinforcement_events` / abstraction-edge weight referencing a hidden node_id is orphaned each cycle.
- Re-runs HIDDEN-WEIGHT-001's backward-pass weight computation from scratch every cycle (wasted work).
- **CUIDv2-rule violation**: `node_id: randomUUID()` (service.go:688) instead of CUIDv2.

This is the exact bug class **HIDDEN-CHURN-001 fixed for `conversation_theme`** (match-by-centroid + update-in-place), but that fix was never applied to the `hidden` pattern path (`CreateHiddenNodes`).

**Trigger (confirmed):** `RunConsolidation` → `RunNodeCreationPipeline` (phase 10–22) → `hiddenStep.Run` (phase 10) → `CreateHiddenNodes`, driven by the supervised periodic consolidation loop (~5 min) + RSIC watchdog.

## 3. Scope & Constraints
**In scope:**
- Rewrite `CreateHiddenNodes` (`internal/hidden/service.go`) from wipe-all-then-recreate to match-by-centroid → update-in-place → create-only-unmatched → delete-only-unmatched, mirroring `ClusterConversations`.
- Mint hidden-pattern `node_id` as **CUIDv2** (`cuid2.Generate()` Go-side, passed as param — Cypher cannot mint CUIDv2), replacing `randomUUID()`.
- New `internal/hidden/hidden_identity.go` mirroring `theme_identity.go`: `listHiddenPatterns`, `updateHiddenNodeWithEdges`, `deleteUnmatchedHiddenPatterns` (reuse the generic `matchTheme` + `themeRef`).
- New config knob `HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD` (default 0.90), parallel to `HIDDEN_THEME_IDENTITY_SIM_THRESHOLD`. No-hardcoding rule.
- Verify `graph_node_drop` + `High Orphan Ratio` alerts clear post-fix.

**Out of scope:**
- Re-id'ing the ~2,636 existing UUID-keyed hidden nodes (forward-only — they match-in-place going forward; the UUID population drains as unmatched ones age out; synthetic backfill avoided per the EVENTGRAPH-004 historical-record precedent).
- Hardening the `graph_node_drop` alert rule's snapshot-comparison fragility (stops firing once churn ends; follow-up only if it still false-fires post-fix).
- Emergent-concept (layer ≥2) clustering — separate path, not churning per the live label breakdown.

**Constraints:** sequential epics; Tier-3 live testing required; CUIDv2 mandatory; protected `mdemg-dev` deletion-safety preserved (delete-only-unmatched, never wipe-all).

## 4. Dependencies
- HIDDEN-CHURN-001 (`theme_identity.go` — `themeRef`, `matchTheme`, match/update/delete pattern) — shipped on `main`.
- `member_edges.go::memberEdgePairs` + `cuid2.Generate()` (CUIDv2 mint) — present.
- HIDDEN-WEIGHT-001 abstraction-edge weight logic (`vector.similarity.cosine`) — update-in-place must preserve.
- Live stack (real `bin/mdemg` + Neo4j + TSDB) for Tier-3.

## 5. Implementation Plan (sequential epics)

**Epic 0 — Plan doc + trigger confirmation.** Commit this plan. Confirm caller/cadence (done — see §2). *Gate: plan committed, trigger documented.*

**Epic 1 — `hidden_identity.go` helpers.** `listHiddenPatterns(ctx, spaceID) []themeRef` (node_id + centroid for `role_type='hidden'` layer-1); `updateHiddenNodeWithEdges(...)` (update name/centroid/updated_at + node-scoped GENERALIZES rewire to current members, HIDDEN-WEIGHT-001 cosine weights preserved); `deleteUnmatchedHiddenPatterns(...)` (delete only patterns claimed by no cluster this run, batched, mdemg-dev-safe). Reuse `matchTheme`. *Gate: compiles; unit tests per helper.*

**Epic 2 — Rewrite `CreateHiddenNodes` + CUIDv2.** Replace `detachBaseNodeHiddenEdges` wipe-all (step 1b) with list-existing + per-cluster match-or-update / create-unmatched, then `deleteUnmatchedHiddenPatterns`. `createHiddenNodeWithEdges` accepts a Go-minted `cuid2.Generate()` node_id (drop `randomUUID()`). Add config knob. *Gate: build + lint; `detachBaseNodeHiddenEdges` retired.*

**Epic 3 — Tier 1+2 tests.** Unit: CUIDv2 format; `matchTheme` reuse for hidden refs; update preserves node_id; delete-only-unmatched. Integration: double-run idempotency (ids + count stable); weight preservation on updated edges. *Gate: all green.*

**Epic 4 — Tier 3 live.** Real `bin/mdemg` against live mdemg-dev: capture hidden node_ids + count, drive 2 consolidation cycles, observe `mdemg_neo4j_graph_nodes` stops oscillating (no −2,600 dip), confirm node_ids survive across cycles (Neo4j query), confirm `graph_node_drop` + `High Orphan Ratio` alerts clear. Small-batch first. *Gate: gauge steady + ids stable + alerts cleared, observed live.*

**Epic 5 — Documentation (final, never cut).** Feature doc, `CLAUDE.md` Architecture Note, CHANGELOG, `post.md`, Documents Accessed.

## 6. Testing Plan (3 tiers)
- **Tier 1 unit:** CUIDv2 format; matchTheme-for-hidden; update-in-place id preservation; delete-only-unmatched selection.
- **Tier 2 integration:** double-run idempotency (ids + count stable); weight-preservation on updated edges.
- **Tier 3 live:** real binary, 2 live cycles, gauge-steady + id-survival + alert-clear observed via TSDB/Neo4j/alerts file.

## 7. Commit Strategy
Sequential per-epic commits on `reh3376_dev01`. CUIDv2 fix in the Epic 2 commit. Live-smoke surprise bugs get their own fix-commits (Phase 11.6.2 precedent). Final commit promotes CHANGELOG.

## 8. Verification Checklist
- [ ] `CreateHiddenNodes` matches-and-updates; no wipe-all
- [ ] New hidden node_ids are CUIDv2 (no `randomUUID()` in committed code)
- [ ] Double-run: node_ids + count stable
- [ ] Live: `mdemg_neo4j_graph_nodes` steady across ≥2 cycles (no −2,600 dip)
- [ ] Live: hidden node_ids survive across cycles
- [ ] `graph_node_drop` CRITICAL + `High Orphan Ratio` MEDIUM cleared
- [ ] HIDDEN-WEIGHT-001 cosine weights still computed on updated edges
- [ ] `golangci-lint run ./internal/hidden/...` clean
- [ ] Docs + CHANGELOG + CLAUDE.md updated

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| KMeans non-determinism shifts centroids → spurious create+delete instead of match | Med | Med | 0.90 identity threshold absorbs jitter (proven for themes); Tier-2 double-run test catches it |
| Update-in-place drops/mis-rewires member edges | Low | High | Node-scoped rewire mirrors `updateConversationThemeWithEdges`; integration test asserts edge count |
| Deleting unmatched hidden nodes removes legitimately-changed clusters' history | Low | Med | Same semantics as themes (re-cluster = new identity); forward-only, no protected-node loss |
| Existing ~2,636 UUID nodes never re-id'd | — | Low | Documented forward-only; population drains as unmatched ones age out |

## 11. Documents Accessed
- `internal/hidden/service.go` (CreateHiddenNodes 334-457, detachBaseNodeHiddenEdges 597-673, createHiddenNodeWithEdges 676-730, RunConsolidation 1600, RunNodeCreationPipeline 314)
- `internal/hidden/theme_identity.go` (reference fix: listConversationThemes, matchTheme, updateConversationThemeWithEdges, deleteUnmatchedThemes)
- `internal/hidden/member_edges.go` (CUIDv2 mint + memberEdgePairs)
- `internal/hidden/step_hidden.go` (phase-10 hiddenStep adapter)
- `internal/alert/rules.go` (graph_node_drop, High Orphan Ratio rules)
- Live TSDB `metric_samples` (`mdemg_neo4j_graph_nodes`) + Neo4j label/role_type breakdown

## 12. Rollback Procedures
Revert the `CreateHiddenNodes` + `hidden_identity.go` commits → restores prior behavior (re-introduces churn but no data loss). Existing CUIDv2 + UUID nodes both remain valid. Config knob defaults to off-impact (0.90).
