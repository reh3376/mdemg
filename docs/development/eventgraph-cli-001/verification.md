# EVENTGRAPH-CLI-001 — Live Verification (Tier 3)

**Date:** 2026-06-08
**Stack:** native `mdemg serve` (launchd `com.mdemg.server`, rebuilt from this branch) + Docker (Neo4j `mdemg_neo4j_data` + TimescaleDB) + llama-server :8102. Space `mdemg-dev`.

Per the standing directive — *standard code testing is not sufficient to find problems in the live running framework* — every acceptance item below was exercised against the real running server with the real binary, not a mock.

## Acceptance bar: the CLI consumes the federation API end-to-end and surfaces real reinforcement events.

### Tier 1 (unit, `-race` clean)
- `internal/cli/eventgraph_test.go` — 8 tests: request-mapping omit-when-unset + unit conversion (`24h`→`86400`), `--query` seed resolution (httptest), no-results / invalid-`--since` / surfaced-503 errors, render (empty + table), helpers. **PASS.**
- `internal/eventgraph/query_test.go::TestFederationResult_EmptyArraysNotNull` — pins the JSON contract caught live (see below). **PASS.**

### Tier 2 (contract — UATS, live server)
`docs/api/api-spec/uats/specs/eventgraph_reinforcement_neighborhood.uats.json` — 6 cases validated **6/6** against `http://localhost:9999`:

```
Total specs : 6   Passed : 6   Failed : 0   Pass rate : 100.0%
```

happy 200 (response-shape assertions) · missing_space_id → 400 · missing_seed_node_id → 400 · negative_hops → 400 · hops_over_ceiling (999 > 2×default) → 400 · method_not_allowed (GET) → 405. sha256 integrity hash added + verified.

### Tier 3 (live e2e — real binary against real services)

**`--query` form** (resolves a real seed, then federates):
```
$ mdemg eventgraph reinforcement-neighborhood --query "circuit breaker state machine" --hops 2 --since 720h
resolved seed from query "circuit breaker state machine" → n_8d0b318843bbe8769c01
seed:         n_8d0b318843bbe8769c01
neighborhood: 5 nodes · hops: 2 · events: 20 · scanned: 20 · truncated: false

SRC            DST              Δweight    new_w direction     new nbhd  recorded
n_661cb5203f72 n_8d0b318843bb   +0.0086   0.1086 bidirectional   ✓   ✓✓  06-08 11:51:14
... (20 rows) ...
```

**Live-loop observation:** the 20 events are timestamped `11:51:14` — the moment the `--query` retrieval ran. The retrieval itself fired `ApplyCoactivation` over the 5-node circuit-breaker cluster (C(5,2)=10 pairs × 2 passes: first pass `created_new_edge=✓` 0.10→0.108, second pass strengthening 0.108→0.116), and the federation read those very events back. The CLI exercised the full **Hebbian-write → federation-read** loop in a single command — exactly the live-testing utility this sprint was built to provide.

**`--seed` form** (explicit seed, `--limit` enforced):
```
$ mdemg eventgraph reinforcement-neighborhood --seed n_8d0b318843bbe8769c01 --hops 1 --since 24h --limit 5
neighborhood: 5 nodes · hops: 1 · events: 5 · scanned: 5 · truncated: true   ← limit honored
```

**`--json` form** (machine output, jq-parseable):
```
$ mdemg eventgraph reinforcement-neighborhood --seed n_8d0b318843bbe8769c01 --hops 1 --since 24h --json | jq ...
{ "events": 20, "neighbors": 5, "hops": 1, "truncated": false,
  "first_event": { "src_node_id": "n_661cb5203f72...", "delta_weight": 0.00857..., "created_new_edge": true, "src_in_neighborhood": true } }
```

**Unknown seed** (graceful, and `neighbor_node_ids:[]` per the live-caught fix):
```
$ mdemg eventgraph reinforcement-neighborhood --seed n_does_not_exist_xyz
neighborhood: 0 nodes · hops: 2 · events: 0 · scanned: 0 · truncated: false
No reinforcement events in this neighborhood/window. (The graph walk succeeded — there were just no co-activation events touching it. …)
```

**No seed/query** → clear error: `a seed is required: pass --seed <node_id> or --query <text>`.

## Surprise bug caught live (own fix-commit, per precedent)

The UATS happy-path asserted `type_is array` on `$.neighbor_node_ids`, and **the live server returned `null`, not `[]`**, for an unknown seed — `walkNeighborhood` returns a nil slice that JSON-marshals to `null`, while `events` was already initialized to `[]`. Standard Go unit tests never caught it (they don't marshal the empty-neighborhood result); the live contract test did, immediately. Fixed at the source (`EventsInGraphNeighborhood` coalesces the nil slice) in its own fix-commit `9bf981b`, with `TestFederationResult_EmptyArraysNotNull` pinning the contract. Re-verified live: `neighbor_node_ids` now `[]`.

## Acceptance criteria — met
1. ✅ `mdemg eventgraph reinforcement-neighborhood` consumes the federation API live; both `--seed` and `--query` forms work.
2. ✅ Real reinforcement events surfaced (20 in a 5-node neighborhood) with correct annotations (`created_new_edge`, in-neighborhood flags).
3. ✅ `--limit` truncation, `--json`, unknown-seed, and no-arg error paths all behave correctly live.
4. ✅ No hardcoded `hops`/`since`/`limit` defaults in the CLI — unset flags omitted, server applies config (single source of truth).
5. ✅ UATS contract 6/6 live; sha256 verified. Backfills the UATS gap EVENTGRAPH-001 left.
6. ✅ Surprise contract bug (null vs `[]`) caught live, fixed at source in its own commit, pinned by a unit test.

## Conclusion
EVENTGRAPH-CLI-001's bar is met and live-verified. The federation API now has its first consumer and a live-testing harness for the EVENTGRAPH line (EVENTGRAPH-002/003). The live run doubled as a demonstration that the retrieve-time Hebbian write path and the federation read path are both healthy and mutually consistent.
