# Hidden-Pattern Identity Stability (HIDDEN-CHURN-002 + 003)

## Why

The hidden-pattern layer (`role_type='hidden'`, layer 1) is MDEMG's first abstraction tier — it generalizes raw L0 base nodes (code files, observations) into ~thousands of `HiddenPattern` nodes connected by `GENERALIZES` edges. Everything downstream references these nodes by `node_id`: `CO_ACTIVATED_WITH` / `reinforcement_events` Hebbian edges, abstraction-edge weights (HIDDEN-WEIGHT-001), and the RRF graph-retrieval column.

Two defects made that layer unstable for the project's history:

1. **Destroy-and-recreate churn.** `CreateHiddenNodes` deleted *every* L0→L1 `GENERALIZES` edge and every childless pattern at the start of each consolidation cycle, then recreated all patterns from scratch with a fresh `randomUUID()` each time. Result: ~2,636 nodes + ~31,106 edges churned every ~5 min, the CRITICAL `graph_node_drop` alert fired (live-measured −2,676 in one window), every Hebbian/abstraction edge referencing a pattern was orphaned each cycle, and the ids were UUIDs (CUIDv2-rule violation). This is the same bug class HIDDEN-CHURN-001 fixed for `conversation_theme` — never applied to the `hidden` path.

2. **Consolidation never completed.** Re-clustering all L0 nodes (~52k+) takes ~10 min, but `handleConsolidate` ran the whole pipeline under the **caller's cancellable request context** (`r.Context()`). Every trigger — the codebase-sync after ingest, RSIC, manual — timed out and **cancelled the cycle mid-write** (`context canceled`); live evidence: **236 consolidate-500s vs 58 200s** (the 200s all fast no-op runs). The hidden step never reached its end, so the new match-in-place delete never ran.

## Choices

- **Match-in-place, not wipe-and-recreate.** Each cluster matches an existing pattern and updates it in place (node_id survives); only patterns matched by no current cluster are deleted; genuinely new clusters get a **CUIDv2** id. Mirrors `ClusterConversations` (HIDDEN-CHURN-001).
- **Member-overlap (Jaccard) as the PRIMARY identity signal, centroid-cosine as fallback.** Centroid-cosine matching alone (the theme approach) left **~28% churn/cycle** in live Tier-3, because KMeans repartitions thousands of small clusters slightly differently each run and the centroids drift past the 0.90 cosine floor. A cluster's *member set* (the L0 nodes it groups) is far more stable than its centroid position under that jitter, so identity is matched by **Jaccard overlap of L0 members** first (`HIDDEN_PATTERN_MEMBER_JACCARD_THRESHOLD`, default 0.5), falling back to centroid cosine (`HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD`, default 0.90) only when membership turned over but the pattern is semantically the same.
- **Detach consolidation from caller cancellation.** The full cycle runs under `context.WithoutCancel(r.Context())` + a generous server-side deadline (`CONSOLIDATE_TIMEOUT_MS`, default 30 min), so a client timeout can no longer abort it. This is what lets the cycle reach `deleteUnmatched` — without it the match-in-place fix accumulates patterns unboundedly instead of churning them.
- **Forward-only.** Pre-existing UUID-keyed patterns are matched in place (their UUIDs persist) and age out naturally as unmatched ones are deleted; no synthetic re-id backfill (EVENTGRAPH-004 historical-record precedent).

## How it works

`CreateHiddenNodes` (`internal/hidden/service.go`):
1. Fetch all L0 base nodes; classify by extension; KMeans within each category.
2. `listHiddenPatternRefs` loads existing patterns with both their centroid **and** L0 member set; `buildMemberIndex` builds a member→pattern inverted index.
3. For each new cluster: `matchHiddenPattern` tallies member overlap via the index (Jaccard primary), falls back to centroid cosine. A match → `updateHiddenNodeWithEdges` (in-place; node_id + inbound refs survive; `GENERALIZES` weights recomputed via `vector.similarity.cosine`). No match → create with `cuid2.Generate()`.
4. After the loop: `deleteUnmatchedHiddenPatterns` removes only patterns claimed by no cluster this run (batched, mdemg-dev-safe).

`handleConsolidate` (`internal/api/handlers.go`) runs steps 1–5 under a detached, deadline-bounded context (`consolidateTimeout()`), so the cycle always completes.

`internal/hidden/hidden_identity.go` holds the identity helpers (`hiddenPatternRef`, `listHiddenPatternRefs`, `buildMemberIndex`, `matchHiddenPattern`, `updateHiddenNodeWithEdges`, `deleteUnmatchedHiddenPatterns`).

## How to use

Operator-transparent — the fix is in the consolidation write path. Tunables (all default-sane, no-hardcoding rule):

| Env var | Default | Meaning |
|---|---|---|
| `HIDDEN_PATTERN_MEMBER_JACCARD_THRESHOLD` | 0.5 | Primary member-overlap match floor; 0 disables (centroid-only) |
| `HIDDEN_PATTERN_IDENTITY_SIM_THRESHOLD` | 0.90 | Centroid-cosine fallback match floor |
| `CONSOLIDATE_TIMEOUT_MS` | 1800000 (30 min) | Server-side deadline for a detached consolidation cycle (floor 60000) |

## Live Tier-3 result

- Consolidation **completes** for the first time on mdemg-dev (created + updated + `deleteUnmatched` all run); previously 236× 500/`context canceled`.
- Hidden node_ids are **CUIDv2** for new patterns (was 100% `randomUUID()`).
- Identity churn per cycle: **100% (pre-fix) → ~25%** (75% of patterns survive in place across consecutive cycles — live-measured 3093/4144).
- Total-node gauge oscillation: **±2,676 → ±780**, converging.

## Residual fully remedied — incremental clustering (HIDDEN-CHURN-003)

CHURN-002 left ~25% per-cycle churn because the hidden step **re-clustered all ~52k L0 nodes from scratch every cycle** (KMeans jitter + non-deterministic `RECLASS_ENABLED` reclassification reshuffled membership). **HIDDEN-CHURN-003 closes it** with **incremental clustering** (default; `HIDDEN_INCREMENTAL_ENABLED=true`):

- The consolidation hidden step (`IncrementalHiddenNodes`) fetches only **orphan** L0 nodes (no `GENERALIZES`→pattern edge), assigns each to its nearest existing pattern (`nearestPatternByCentroid`, cosine ≥ `HIDDEN_INCREMENTAL_ASSIGN_SIM_THRESHOLD`, default 0.80) with a `GENERALIZES` edge + incremental-mean centroid update, and clusters only the **unassigned remainder** into new CUIDv2 patterns.
- **Existing patterns are never destroyed or re-clustered** → node identity is preserved.
- Full re-cluster (the CHURN-002 path) is retained as an explicit operator command: `mdemg concepts recluster --space-id <id>` (or the `full_recluster` consolidate field).

**Live Tier-3 result (mdemg-dev):** **100% identity survival** across consecutive cycles (4484/4484 — vs CHURN-002's 75%); patterns only **added**, never destroyed (4484→4758). Steady-state cycle wall-time **~10s** (82 orphans) vs the ~360s full re-cluster — the per-cycle Neo4j CPU cost drops with it.

**Known trade-off:** incremental-only clustering lets pattern count grow slowly (no periodic re-cluster on the auto path); the explicit `concepts recluster` command is the quality-maintenance escape hatch, and split/merge maintenance is a possible future refinement. Operator can fall back to the full path with `HIDDEN_INCREMENTAL_ENABLED=false`.

See `docs/development/hidden-churn-002/` and `docs/development/hidden-churn-003/` for sprint plans + verification.
