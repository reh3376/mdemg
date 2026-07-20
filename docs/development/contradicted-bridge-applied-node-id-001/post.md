# CONTRADICTED-BRIDGE-APPLIED-NODE-ID-001 — Sprint Post

**Shipped:** 2026-07-20 | **Branch:** `reh3376_dev01` | **PR:** (pending push)

## What shipped

Closes the deferred follow-up from JIMINY-CONTRADICTED-BRIDGE-001. On approve, the contradicted-drafts HITL sink now writes the applied MemoryNode's `node_id` alongside the observation's `obs_id` — two distinct CUIDv2s that previously required an `obs_id → node_id` join for every graph-side consumer.

## Epics

- **E0** — Sprint plan (skill:sprint-planning v1.0 12-section format). Commit `84787ee`.
- **E1** — Additive V0031 migration + writer contract. `contradicted_correction_drafts` gains `applied_node_id TEXT`; `ContradictedDraftRow.AppliedNodeID`; `Flush` CopyFrom includes the column; `FetchPendingBySpace` / `FetchByID` SELECT + Scan the column; `MarkApproved(ctx, id, appliedObsID, appliedNodeID)` writes both. `TSDB_REQUIRED_SCHEMA_VERSION` 30 → 31. Commit `fc6fa35`.
- **E2** — Sink capture. `contradictedDraftsSink.Apply` captures `resp.NodeID` alongside `resp.ObsID` and threads both into `MarkApproved`. `contradictedDraftItem.Meta` surfaces `applied_node_id`. Commit `66ba500`.
- **E3** — Tier-1 pin. Extracted `contradictedDraftsWriterIface` (subset of `*tsdb.ContradictedDraftsWriter`). New `TestContradictedDraftsSink_Apply_ApprovePassesBothIdentifiers` uses a capturing mock to assert BOTH `obsID="obs-1234"` AND `nodeID="node-5678"` reach `MarkApproved`. All 8 sink tests + full `go test ./...` + `golangci-lint` clean. Commit `ff548a2`.
- **E4** — Live Tier-3. Rebuilt `bin/mdemg`, restarted the server via launchd, verified `tsdb_schema_meta.schema_version = 31` and the column present. Warmed with a contradicted-bait context ("do not commit directly to main — I am about to push a commit"); real LLM verdict `contradicted` (sim=0.85); new pending draft `mog55fzfa9mi22zqbuypx20d`. Submitted `POST /v1/review/grade` with `durable_rule=4` and `reinforce=true` → `reinforcement_applied=true`. Draft row post-approve: `applied_obs_id=cdo0jzbxh8rce8vprycz13n4`, `applied_node_id=d6pesuwmby8tmmmzdmr5k47e`, `status=approved`. Both IDs resolve in Neo4j to the same L0 correction node (confirming the two really are distinct).
- **E5** — Canonical docs. CHANGELOG [Unreleased] > Fixed entry; `docs/features/hitl-review.md` Applied-Identifiers addendum in the contradicted_drafts section; CLAUDE.md JIMINY-CONTRADICTED-BRIDGE-001 note updated in place (closes the "deferred follow-up: add applied_node_id column" disclosure, adds the "capture BOTH obs_id + node_id when a sink writes entity ids" architectural rule).

## Live evidence

```
Draft:      mog55fzfa9mi22zqbuypx20d
  status:            approved
  applied_obs_id:    cdo0jzbxh8rce8vprycz13n4
  applied_node_id:   d6pesuwmby8tmmmzdmr5k47e   <- the sprint's whole point
  applied_at:        2026-07-20 19:48:45.091731+00

Neo4j:      MemoryNode {node_id: 'd6pesuwmby8tmmmzdmr5k47e'}
  obs_id:        cdo0jzbxh8rce8vprycz13n4
  role_type:     conversation_observation
  layer:         0
  content:       CORRECTION: Incorrect: ... | Correct: ... | Context: ...
```

The two IDs are different by design (obs_id is stamped on the observation record; node_id is minted by Neo4j when the graph node is created). Pre-sprint code that only had `applied_obs_id` had to run an extra Cypher lookup to get to the node.

## Historical rows

The single pre-E1 approved draft (`c8jvgnmkl8zlmr4m58nl7rj3` from JIMINY-CONTRADICTED-BRIDGE-001 E5) keeps `applied_node_id=NULL`. Retrievable on demand via `MATCH (n:MemoryNode {obs_id:'<applied_obs_id>'}) RETURN n.node_id`. Backfill deferred as low-value cleanup — one row, and the join still works.

## Deviations

None. Plan executed as written.

## Rollback

- Data: safe (additive column; NULL for pre-migration data)
- Code: revert the four commits + accept the schema-version bump (V0031 is idempotent — a subsequent boot won't re-run the ADD)

## Next up

Per user direction: GRAFANA-PANEL-FILTER-001 (apply `NOT LIKE 'caller_canceled:%'` filter to dashboard panels), then a review of Grafana dashboard metrics that are lower than expected.
