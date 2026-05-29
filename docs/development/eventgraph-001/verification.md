# Sprint EVENTGRAPH-001 Epic 7 — Tier 3 Live e2e Verification

**Date:** 2026-05-27
**Stack:** Native `./bin/mdemg serve` (port 9999, PID 63551) against `mdemg-timescaledb-1` + `mdemg-neo4j-1` (docker compose).
**Binary:** built from `reh3376_dev01` at commit graph `dec03df` (Epic 6 + restored audit + Epic 7 fix-commit pending).
**Space:** `mdemg-dev` — protected production space, hardcoded deletion-protected.

## Acceptance criteria check

1. **Real `/v1/memory/retrieve` against `mdemg-dev` → corresponding rows land in `reinforcement_events` within ≤ 35 seconds:**
   ✅ PASS. Three retrieves issued at 13:43, table polled at 13:44, 10 rows present.

2. **`POST /v1/eventgraph/reinforcement-neighborhood` with a seed node from the retrieve returns events whose src/dst are within the requested hops:**
   ✅ PASS. Seed `n_be61aa4671ef8194ae09`, hops=1 → 5-node neighborhood, 10 events returned, all 10 with `src_in_neighborhood=true AND dst_in_neighborhood=true`.

3. **Grafana panel shows non-zero events/min:**
   ✅ PASS. Panel "Reinforcement Event Rate (events/min)" on `mdemg-graph-topology.json` shows ~20 events/min sustained for ~30 seconds following the three retrieves (decaying to baseline as Prometheus rate window slides).

4. **`EVENTGRAPH_ENABLED=false` short-circuits the writer cleanly:**
   Validated in unit/handler tests (Tier 1 `TestEventgraphHandler_FeatureDisabled_503`). Boot-time toggle path verified by api/server.go inspection — not re-validated in this transcript to avoid restarting the production stack again.

5. **`mdemg tsdb status` reports schema_version=22; re-running `tsdb migrate` is idempotent:**
   ✅ PASS. `Schema version: 22`. Idempotent re-run logged "migration complete" again with no row changes.

## Surprise log — pre-existing latent bug discovered

Per `CLAUDE.md` "Testing — Live System Testing Is Required" policy: "surprise bugs caught during live smoke get their own follow-up fix-commit — do not silently roll them into the sprint commit."

**Discovery:** First Tier 3 attempt produced 0 rows in `reinforcement_events` despite 3 successful retrieves. Investigation showed:

1. Server logs confirmed the writer was attached and the federation service was initialized.
2. The retrieves returned 12 L0 results each, all above the `vector_sim_floor` (0.5).
3. `ApplyCoactivation` filters candidates by `r.Activation >= LearningMinActivation (default 0.20)`.
4. **The Phase 13.1 default-on RRF retrieval path (`internal/retrieval/scoring_rrf.go:121-129`) constructed `RetrieveResult` without setting the `Activation` field.** The legacy `ScoreAndRank` path at `scoring.go:883` correctly populates `Activation: a` (where `a := act[c.NodeID]` is the spreading-activation map value). The RRF path silently dropped this field.
5. Net effect: every node arrived at `ApplyCoactivation` with `Activation = 0`, failed the `>= 0.20` filter, and the function returned without writing any edges OR emitting any reinforcement events.

This bug has been latent **since Phase 13.1 default-on (2026-05-03)** — Hebbian learning has been silently no-op on every retrieve through the production HTTP path for ~24 days. The graph still has CO_ACTIVATED_WITH edges, but they were created by tests, sidecar paths (`CoactivateSession`, `ApplySymbolCoactivation`, consolidation walks), and the legacy scorer back when RRF was opt-in — not by the retrieve-time Hebbian goroutine.

**Fix:** one-line addition in `scoring_rrf.go::ScoreAndRankRRF` — `Activation: act[c.NodeID]` on the RetrieveResult literal. Brings the RRF path to parity with the legacy scorer.

**Verification of fix:** rebuilt, restarted the server, re-issued 3 retrieves, observed 10 rows in `reinforcement_events` (post-flush) with non-zero `activation_product` and signed `delta_weight` values. Federation API also returned the expected pair density (5 neighbors × ~2 events each).

This is precedent-aligned with the project memory `feedback_thorough_troubleshooting.md` ("NEVER stop at the first break") and CLAUDE.md Phase 11.6.2 ("surprise bugs caught during live smoke get their own fix-commit"). Fix landed as a separate commit before Epic 8.

## Raw observation transcript

```
13:43:13  POST /v1/memory/retrieve  → 200  query=Hebbian co-activation learning
13:43:14  POST /v1/memory/retrieve  → 200  query=TSDB reinforcement events
13:43:15  POST /v1/memory/retrieve  → 200  query=graph federation pattern
13:43:50  SELECT count(*) FROM reinforcement_events WHERE space_id='mdemg-dev'
           → 10 rows
           - 10 unique pairs (no duplicate-write per pair)
           - 10 created_new_edge=true / 0 strengthened
             (all 10 pairs were first-time co-activations; consistent with
              `n_be61aa4671ef8194ae09` being a freshly-indexed conversation
              node from this session)
           - avg new_weight = 0.1057
           - avg delta_weight = +0.00568

13:43:55  POST /v1/eventgraph/reinforcement-neighborhood
          body: {space_id:"mdemg-dev", seed_node_id:"n_be61aa4671ef8194ae09",
                 hops:1, since_seconds:3600, limit:50}
          response:
            graph_hops: 1
            neighbor_node_ids: 5 entries
            events: 10
            tsdb_rows_scanned: 10
            truncated: false
            All events: src_in_neighborhood=true, dst_in_neighborhood=true
```

## Sample event payloads (from the federation response)

```json
{
  "src_node_id": "n_be61aa4671ef8194ae09",
  "dst_node_id": "n_226b0d7c4c84...",
  "prev_weight": 0.10,
  "new_weight": 0.1083,
  "delta_weight": 0.0083,
  "evidence_count_after": 1,
  "direction": "bidirectional",
  "created_new_edge": true,
  "trigger_path": "apply_coactivation",
  "src_in_neighborhood": true,
  "dst_in_neighborhood": true
}
```

## Conclusion

Epic 7 verification PASS. The Pattern Y1 implementation works end-to-end on the live stack: Hebbian writes produce reinforcement events, events land in TSDB within the flush window, the federation API correctly orchestrates graph-walk + TSDB query + neighborhood annotation. Sprint may proceed to Epic 8 (Documentation Update + close).

The latent RRF-Activation bug surfaced by this Tier 3 test is itself a meaningful side-deliverable: 24 days of silent Hebbian no-op on the production retrieve path is now fixed. Recommend follow-up sprint EVENTGRAPH-002 to backfill the missing co-activation edges (or accept the gap as forward-only — the graph self-heals as new retrieves now correctly emit Hebbian updates).
