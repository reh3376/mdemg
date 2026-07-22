# Event Graph Federation (Pattern Y1)

**Sprint:** EVENTGRAPH-001 (2026-05-27)
**Status:** Default-on. Reinforcement events captured from `ApplyCoactivation`; federation API live at `POST /v1/eventgraph/reinforcement-neighborhood`.
**Feature surface:** `POST /v1/eventgraph/reinforcement-neighborhood`, V0022 `reinforcement_events` TSDB hypertable, Grafana panel on `mdemg-graph-topology`, 3 Prometheus counters.

## Why

The graph captures **state** (current `weight`, `evidence_count`, `last_activated_at` on each `CO_ACTIVATED_WITH` edge). It discards **history** — *when* an edge was strengthened, by how much, under what surprise factor, in what session. Questions like

- *"Which co-activations got strengthened during yesterday's planning session?"*
- *"Show me the reinforcement trajectory for edge A–B over the last 30 days."*
- *"What was the rate of new edge creation vs reinforcement over the last week, per `trigger_path`?"*

are time-series questions. They belong in TSDB, not the graph. Before EVENTGRAPH-001 there was no way to answer them — the data was silently overwritten on every Hebbian update.

This feature also lays the groundwork for the broader **TypeDB-inspired Neo4j refactor**: "events about edges" is the first hyperrelation-shaped use case. Federating it via TSDB rather than reifying it in the graph is the cheapest path to capability while preserving graph traversal via a Go orchestration layer (Pattern Y1).

## Choices

### Pattern Y1 (TSDB federation) over Pattern Y2 (link-node reification)

Two designs were considered:

| Pattern | What it does | Cost | When to use |
|---|---|---|---|
| **Y1 (chosen)** | Events live only in TSDB; graph stays unchanged; Go orchestrates the join | ~1 sprint | When events are analytical, not graph-traversable |
| Y2 | Skinny event-link nodes in Neo4j point at canonical TSDB rows by `event_id` | ~1 sprint + V0027 schema migration | When a query needs single-pass Cypher across events |

Y1 is the cheapest path that preserves graph traversal capability. Federation in Go is sufficient for the queries we have today. Y2 is held in reserve for when a query shape proves Y1 insufficient.

### Buffered writer (V0019 pattern) over synchronous insert (V0021 pattern)

Hebbian writes are **per-retrieve** — far higher volume than the CLI-driven `model_install_events`. V0021's synchronous-single-row INSERT pattern would put the writer in the hot path of every retrieve. V0019's buffered CopyFrom (30s auto-flush, 1000-row buffer with FIFO eviction on full) is the right shape: non-blocking enqueue, batched DB write.

### All five Hebbian entry points (the `trigger_path` discriminator)

All Hebbian write paths now feed `reinforcement_events`, distinguished by `trigger_path`:

| `trigger_path` | source | notes |
|---|---|---|
| `apply_coactivation` | `ApplyCoactivation` (retrieval hot path) | EVENTGRAPH-001; full Hebbian fields |
| `apply_symbol_coactivation` | `ApplySymbolCoactivation` (symbol-to-symbol) | EVENTGRAPH-003; eta/surprise/activation/path_sim N/A (NULL); high pair volume |
| `coactivate_session` | `CoactivateSession` (same-session observations) | EVENTGRAPH-003; full Hebbian fields |
| `apply_negative_feedback` | `ApplyNegativeFeedback` (weaken: existing `CO_ACTIVATED_WITH`) | EVENTGRAPH-003; **negative** `delta_weight`, `created_new_edge=false` |
| `apply_negative_feedback_contradict` | `ApplyNegativeFeedback` (contradict: `MERGE CONTRADICTS`) | EVENTGRAPH-004; see delta-semantics warning below |

⚠️ **`delta_weight` semantics on contradict rows:** the delta is the `CONTRADICTS` edge's *own* weight change — **+`negWeight`** (default +0.15) when the edge is created, **0** on an evidence-increment re-match. The *negative-feedback* semantics are carried by `trigger_path`, **not the sign**. Consumers summing `delta_weight` over a node MUST filter or group by `trigger_path` — a naive sum would read a contradiction as Hebbian strengthening.

EVENTGRAPH-001 shipped #1 (proving the pattern + schema + federation surface). EVENTGRAPH-003 added #2–#4 — each via a `RETURN`-extension + parse-and-record hook reusing the existing writer (no schema/writer/endpoint change; the federation read surfaces the new `trigger_path`s for free). EVENTGRAPH-003 live testing also revived a dormant path: `CoactivateSession` had never been invoked (its `learningService` was never injected) — fixed so session co-activation learning actually runs. EVENTGRAPH-004 added #5 by splitting `ApplyNegativeFeedback` into two statements in the same transaction (the `CONTRADICTS` MERGE lived inside a `FOREACH`, where the edge variable is invisible to `RETURN`); classification is unchanged. Contradict events reuse the existing sink rather than a new event class — zero `CONTRADICTS` edges existed at ship time and the endpoint has no automated producer yet, so this is telemetry-before-the-producer (the inverse of the dormancy pattern). The federation walk still traverses only `CO_ACTIVATED_WITH|GENERALIZES`; contradict events surface whenever either endpoint is in the neighborhood because the TSDB join is by node-id, not edge type.

### Forward-only — no historical backfill

There's no source to backfill from. Pre-V0022 history is permanently lost. The graph self-heals as new retrieves emit Hebbian updates.

## How it works

```
                  ┌──────────────────────────────────────────┐
                  │   POST /v1/memory/retrieve               │
                  │   (internal/api/handlers.go)             │
                  └──────────────┬───────────────────────────┘
                                 │
                                 ▼
                  ┌──────────────────────────────────────────┐
                  │   retrieval.Service.Retrieve()           │
                  │   (vector + BM25 + RRF + rerank)         │
                  └──────────────┬───────────────────────────┘
                                 │  RetrieveResponse with
                                 │  Activation populated
                                 │  (RRF path, post-fix)
                                 ▼
                  ┌──────────────────────────────────────────┐
                  │   go learning.Service.ApplyCoactivation  │
                  │   (asynchronous, doesn't block response) │
                  └──────────────┬───────────────────────────┘
                                 │
                 ┌───────────────┴───────────────┐
                 ▼                               ▼
       ┌─────────────────┐         ┌────────────────────────┐
       │   Neo4j Cypher  │         │   TSDB writer.Record   │
       │   MERGE         │         │   (per-pair telemetry, │
       │   CO_ACTIVATED  │         │    non-blocking)       │
       └─────────────────┘         └───────────┬────────────┘
                                               │
                                               │  30s flush
                                               ▼
                                  ┌────────────────────────┐
                                  │   reinforcement_events │
                                  │   (V0022 hypertable)   │
                                  └────────────────────────┘
```

### TSDB schema (V0022)

One row per logical co-activation pair. The Hebbian Cypher's final `RETURN` clause exposes 17 per-pair fields; the writer maps them into the hypertable:

- `event_id` (CUIDv2 PK), `recorded_at` (now()), `space_id`
- `src_node_id`, `dst_node_id`
- `prev_weight`, `new_weight`, `delta_weight` (signed; negative for weakening)
- `evidence_count_after`
- Optional float-nullable columns: `eta_effective`, `surprise_factor`, `activation_product`, `path_sim`
- Optional string-nullable columns: `role_a`, `role_b`, `obs_type_a`, `obs_type_b`, `session_id`, `direction` (`forward` | `reverse` | `bidirectional`)
- `created_new_edge` (true when ON CREATE fired; false when ON MATCH; reliable proxy for "new connection formed" vs "existing connection strengthened")
- `trigger_path` (`apply_coactivation` | `apply_symbol_coactivation` | `coactivate_session` | `apply_negative_feedback` | `apply_negative_feedback_contradict`)

Indexes: `(space_id, recorded_at DESC)`, `(space_id, src_node_id, recorded_at DESC)`, `(space_id, dst_node_id, recorded_at DESC)`, partial `(space_id, session_id, recorded_at DESC) WHERE session_id IS NOT NULL`.

7-day chunks (same as V0017–V0021).

### Federation helper

`internal/eventgraph/Service.EventsInGraphNeighborhood` runs three steps:

1. **Cypher graph walk** from `seed_node_id` at depth 0..`hops` via `CO_ACTIVATED_WITH | GENERALIZES`. Returns the DISTINCT neighborhood (includes the seed). Empty seed (no match) short-circuits.
2. **TSDB query** for events where `src OR dst` is in the neighborhood, within `since`, ordered newest-first, capped at `limit`.
3. **Go-side join** annotates each event with `src_in_neighborhood` and `dst_in_neighborhood` so consumers can distinguish "both endpoints in the subgraph" from "one endpoint outside the seed's reach but the event still touches our subgraph."

### API

```
POST /v1/eventgraph/reinforcement-neighborhood
Content-Type: application/json

{
  "space_id":       "mdemg-dev",        // required
  "seed_node_id":   "n_be61aa4671ef…",  // required
  "hops":           1,                  // optional; defaults to EVENTGRAPH_FEDERATION_DEFAULT_HOPS
  "since_seconds":  3600,               // optional; defaults to EVENTGRAPH_FEDERATION_DEFAULT_LOOKBACK_HOURS × 3600
  "limit":          50                  // optional; defaults to EVENTGRAPH_MAX_EVENTS_PER_QUERY
}
```

Response:

```json
{
  "events": [
    {
      "event_id": "…",
      "recorded_at": "2026-05-27T13:43:50Z",
      "src_node_id": "n_be61aa4671ef…",
      "dst_node_id": "n_226b0d7c4c84…",
      "prev_weight": 0.10,
      "new_weight":  0.1083,
      "delta_weight": 0.0083,
      "evidence_count_after": 1,
      "direction": "bidirectional",
      "session_id": "",
      "created_new_edge": true,
      "trigger_path": "apply_coactivation",
      "src_in_neighborhood": true,
      "dst_in_neighborhood": true
    }
  ],
  "neighbor_node_ids": ["…", "…"],
  "graph_hops": 1,
  "tsdb_rows_scanned": 10,
  "truncated": false
}
```

Auth: same convention as `/v1/admin/breakers` — gated when `AUTH_API_KEYS` is set, permissive when not.

Errors:
- `400` — missing `space_id` / `seed_node_id`, negative `hops`, `hops` > `2 × EVENTGRAPH_FEDERATION_DEFAULT_HOPS` ceiling
- `503` — `EVENTGRAPH_ENABLED=false` or TSDB unavailable at boot

## How to use

### Configuration

Every operator-visible value is dynamic (no-hardcoding rule). Defaults match the v1 production reality.

| Concern | Env Var | Default |
|---|---|---|
| Feature flag | `EVENTGRAPH_ENABLED` | `true` |
| Writer flush interval (seconds) | `EVENTGRAPH_WRITER_FLUSH_INTERVAL_SEC` | `30` (floor 5) |
| Writer buffer size (rows) | `EVENTGRAPH_WRITER_BUFFER_SIZE` | `1000` (0 = unlimited) |
| Pairs cap per batch | `EVENTGRAPH_MAX_PAIRS_PER_EVENT_BATCH` | `200` (matches `LearningEdgeCapPerRequest`) |
| Federation API row ceiling | `EVENTGRAPH_MAX_EVENTS_PER_QUERY` | `500` |
| Default hops | `EVENTGRAPH_FEDERATION_DEFAULT_HOPS` | `2` |
| Default lookback (hours) | `EVENTGRAPH_FEDERATION_DEFAULT_LOOKBACK_HOURS` | `24` |

### Sample query — recent reinforcements touching a node's 2-hop neighborhood

```bash
curl -X POST http://localhost:9999/v1/eventgraph/reinforcement-neighborhood \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","seed_node_id":"n_be61aa4671ef8194ae09","hops":2,"since_seconds":86400}'
```

### CLI — `mdemg eventgraph reinforcement-neighborhood`

The CLI is the operator-friendly consumer of the federation API (EVENTGRAPH-CLI-001). It wraps the same `POST /v1/eventgraph/reinforcement-neighborhood` endpoint and renders a summary + events table, or raw JSON for piping.

```bash
# Explicit seed node:
mdemg eventgraph reinforcement-neighborhood --seed n_8d0b318843bbe8769c01 --hops 2 --since 24h

# Resolve the seed from a query (top retrieval result becomes the seed):
mdemg eventgraph reinforcement-neighborhood --query "circuit breaker state machine"

# Machine-readable output for piping to jq:
mdemg eventgraph reinforcement-neighborhood --seed n_abc --json | jq '.events | length'
```

| Flag | Purpose | Default |
|---|---|---|
| `--seed` | Seed node_id to walk from (required unless `--query`) | — |
| `--query` | Resolve the seed from the top `/v1/memory/retrieve` result for this text | — |
| `--hops` | Graph traversal depth | server config (`EVENTGRAPH_FEDERATION_DEFAULT_HOPS`) |
| `--since` | Lookback window, e.g. `24h`, `90m` | server config (`EVENTGRAPH_FEDERATION_DEFAULT_LOOKBACK_HOURS`) |
| `--limit` | Max events returned | server config (`EVENTGRAPH_MAX_EVENTS_PER_QUERY`) |
| `--json` | Raw JSON instead of the table | `false` |
| `--space-id` | Space to query | `mdemg-dev` |

Unset flags are omitted from the request body so the server applies its own config defaults — there is no second copy of `hops`/`since`/`limit` defaults hardcoded in the CLI. The table marks new-edge formation (`new` column: `✓` = `created_new_edge`) and which endpoints fell inside the N-hop neighborhood (`nbhd` column: `✓✓` = both src + dst inside). The CLI is also the live-testing harness for the EVENTGRAPH line — running it against the real stack exercises the full Hebbian-write → federation-read loop end-to-end.

### Sample SQL — top-strengthened pairs in the last hour

```sql
SELECT src_node_id, dst_node_id, count(*) AS reinforcements,
       sum(delta_weight) AS total_delta,
       max(new_weight) AS final_weight
FROM reinforcement_events
WHERE space_id = 'mdemg-dev'
  AND recorded_at > NOW() - INTERVAL '1 hour'
GROUP BY src_node_id, dst_node_id
ORDER BY total_delta DESC
LIMIT 20;
```

### Observability

Three Prometheus counters mirror the writer's internal atomic counters:

- `mdemg_eventgraph_writer_rows_enqueued_total` — successful CopyFrom rows
- `mdemg_eventgraph_writer_rows_dropped_total` — FIFO-evicted rows (buffer full)
- `mdemg_eventgraph_writer_flush_failure_total` — CopyFrom errors

The Grafana panel "Reinforcement Event Rate (events/min)" on the `mdemg-graph-topology` dashboard plots all three at once. Sustained non-zero `rows_dropped` indicates the buffer is undersized for the retrieve rate — raise `EVENTGRAPH_WRITER_BUFFER_SIZE` or lower `EVENTGRAPH_WRITER_FLUSH_INTERVAL_SEC`.

### Disabling

`EVENTGRAPH_ENABLED=false` + restart. The Hebbian write path cleanly short-circuits the writer call; existing TSDB data is preserved; re-enable any time. No schema rollback needed.

## Guidance-Outcome Federation (EVENTGRAPH-002)

The second federated event class: **guidance outcomes** — the followed / ignored / contradicted feedback recorded for every surfaced guidance item. It answers a question per-constraint aggregation can't: *"How well is this constraint **and its graph-related constraints** being followed over a time window?"*

**Endpoint:** `POST /v1/eventgraph/guidance-outcome-neighborhood` · **CLI:** `mdemg eventgraph guidance-outcome-neighborhood`

### Why it reuses `constraint_outcomes` (no new sink)

Unlike reinforcement events, the guidance-outcome event stream **already lived in TSDB** before this sprint — the `constraint_outcomes` hypertable (migration 011), written by every `/v1/jiminy/feedback` call (RRF-SCALE-001 + JIMINY-OUTCOME-001). EVENTGRAPH-002 **federates the existing stream** rather than creating a parallel `guidance_outcome_events` table — duplicating a populated sink would violate the no-duplication rule. So this sprint added no new writer and no new enqueue site, only a read-side federation + one index.

### The join key is `constraint_code`, not `node_id`

The reinforcement federation joins TSDB events to the graph on `src/dst_node_id` (real Neo4j node IDs). Guidance outcomes can't: `constraint_outcomes.constraint_id` is a UUID that does **not** match the Neo4j `node_id` (CUID). The only reliable key is **`constraint_code`**, carried by both the Neo4j `role_type='constraint'` nodes (a property) and the `constraint_outcomes` rows (a column). The federation:

1. Walks `CO_ACTIVATED_WITH|GENERALIZES` from the seed, collecting each neighbor's `constraint_code` (+ a code→node map).
2. Queries `constraint_outcomes WHERE constraint_code = ANY(neighborhood_codes) AND time > window` — backed by the V0023 `idx_constraint_outcomes_code (space_id, constraint_code, time DESC)` index.
3. Joins in Go: each outcome's `constraint_node_id` resolves to the neighborhood constraint its code maps to.

Outcomes recorded **without** a `constraint_code` (unmatched at record time) are not joinable and won't appear — a documented limitation, not a defect.

### CLI usage

```bash
# Seed from a constraint node_id:
mdemg eventgraph guidance-outcome-neighborhood --seed myya3xf8kpk3wpbo0qonah99 --hops 1 --since 720h

# Resolve the seed from a query:
mdemg eventgraph guidance-outcome-neighborhood --query "never commit directly to main"

# Machine output:
mdemg eventgraph guidance-outcome-neighborhood --seed n_abc --json | jq '.outcomes | length'
```

The table shows the followed/ignored split + per-outcome `CONSTRAINT_CODE · OUTCOME · sim · g_type · guidance_id · recorded`. Flags mirror the reinforcement subcommand (`--seed`/`--query`/`--hops`/`--since`/`--limit`/`--json`/`--space-id`), and unset `--hops`/`--since`/`--limit` are omitted so the server applies the same `EVENTGRAPH_FEDERATION_DEFAULT_*` / `EVENTGRAPH_MAX_EVENTS_PER_QUERY` config — both federation endpoints share one gate + default-resolution helper server-side. Seeding by an explicit `--constraint-code` is a planned follow-up (needs server-side code→node resolution).

## Forward-looking

- **EVENTGRAPH-CLI-001 (shipped)** — `mdemg eventgraph reinforcement-neighborhood`, the first consumer of the federation API + the live-testing harness for the line (see the CLI section above).
- **EVENTGRAPH-002 (shipped)** — guidance-outcome federation (see above): `POST /v1/eventgraph/guidance-outcome-neighborhood` + `mdemg eventgraph guidance-outcome-neighborhood`, reusing `constraint_outcomes`, joined on `constraint_code`.
- **EVENTGRAPH-002 follow-up** — `--constraint-code` seeding (resolve a constraint node from its code server-side).
- **EVENTGRAPH-003 (shipped)** — wired the writer into the other three Hebbian entry points (`ApplySymbolCoactivation`, `CoactivateSession`, `ApplyNegativeFeedback` weaken-only), distinguished by `trigger_path`. Also revived a dormant path (`CoactivateSession` was never invoked).
- **EVENTGRAPH-004 (shipped)** — federated the contradict action (`apply_negative_feedback_contradict`), completing Hebbian-write coverage. Reuses the existing sink (no new event class — zero `CONTRADICTS` edges existed at ship time). See the delta-semantics warning in the trigger_path section.
- **Negative-feedback producer (open)** — `/v1/learning/negative-feedback` has no automated caller (no hook, MCP tool, CLI, or internal service). Candidate designs: an MCP `memory_reject` tool (the assistant flags a bad retrieval result) or a Jiminy bridge (contradicted guidance outcomes → negative feedback on the constraint's source nodes). Deciding *what counts as rejection signal* is the design question; telemetry is already in place.
- **`CONTRADICTS` in the federation walk (deferred)** — the walk traverses `CO_ACTIVATED_WITH|GENERALIZES` only. Extending through `CONTRADICTS` edges would change neighborhood semantics; revisit when contradiction data exists at meaningful volume.
- **Pattern Y2 escalation** — promote one event class to skinny graph link-nodes when a query proves single-pass Cypher across events is necessary. Triggered by, not assumed.
- **Retention policy (applied)** — V0025 (TSDB-CONSUME-001) added a 180-day retention policy + 14-day compression to `reinforcement_events`; chunks are no longer kept forever.
