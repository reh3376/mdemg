# Sprint EVENTGRAPH-001 — Reinforcement-Event TSDB Hypertable + Graph Federation

> **Status:** DRAFT — awaiting user approval before any implementation work begins.
> **Target start:** TBD (post-approval).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | EVENTGRAPH-001 |
| **Sprint line** | `docs/development/eventgraph-001/` |
| **Date opened** | 2026-05-25 |
| **Target version** | v0.11.0 (minor — additive feature, no breaking changes) |
| **Estimated effort** | 1.5–2 dev-days |
| **OpenAI / LLM spend** | $0 |
| **Risk level** | Low–Medium. Touches the Hebbian hot write path (per-retrieval), so the new writer must be fully non-blocking and the Cypher RETURN-shape change must be backwards-compatible at the Go side. |
| **Successor sprints (if learning validates the pattern)** | EVENTGRAPH-002 (extend to guidance outcomes); EVENTGRAPH-003 (extend to symbol coactivations + session coactivations); EVENTGRAPH-Y2 (promote to link-node graph when a query proves federation-in-Go isn't enough). |

---

## 2. Problem Statement

MDEMG's Hebbian learning currently writes per-pair edge updates into Neo4j (`CO_ACTIVATED_WITH` edge property bag: `weight`, `evidence_count`, `version`, `last_activated_at`, `surprise_factor`). The graph captures **the current state** of every co-activation but discards **the history**: we cannot reconstruct *when* a given edge was strengthened, by how much, under what activation context, or how its weight evolved over the last N retrieves.

This is a connection-layer gap (per MDEMG's core purpose memo): we cannot answer questions like:

- *"Which co-activations got strengthened during yesterday's planning session?"*
- *"What was the rate of new edge creation vs reinforcement over the last week?"*
- *"Which session produced the most surprise-weighted reinforcement?"*
- *"Show me the reinforcement trajectory for the edge between node A and node B over the last 30 days."*

These are time-series questions that don't belong in the graph; they belong in TSDB. Today the answer to all of them is "we don't know — the data is silently overwritten on every Hebbian update."

The downstream effect: when MDEMG misbehaves (e.g. a concept consolidation that produced a bad cluster) we cannot retroactively inspect the reinforcement events that built it. We can only see the end state. This is the same class of gap that Phase 13 Epic 6's empty `retrieval_audit` table represented — fixing it is precedent.

The work also lays the groundwork for the broader **TypeDB-inspired Neo4j refactor** discussion: "events about edges" is the first hyperrelation-shaped use case, and federating it via TSDB rather than reifying it in the graph is the cheapest path to capability while preserving graph traversal via a Go orchestration layer.

---

## 3. Scope & Constraints

### In scope

1. **TSDB schema migration V0022** — new `reinforcement_events` hypertable (7-day chunks; same shape as V0017–V0021).
2. **Buffered writer** `internal/tsdb/reinforcement_writer.go` — CopyFrom + 30s auto-flush + graceful shutdown drain. Pattern: V0019 `SparseGateMetricsWriter`, NOT V0021 `RecordModelInstallEvent` (per-retrieve volume is too high for synchronous single-row INSERTs).
3. **Cypher RETURN-shape extension** in `internal/learning/service.go::ApplyCoactivation` — return per-pair `prev_weight`, `new_weight`, `evidence_count_after`, `created_new_edge`, plus the calculated `etaMult`, `surpriseFactor`, `activation_product`, `path_sim`, `direction`, `session_id`, `obs_type_a/b`, `role_a/b` so the writer can record a row per pair.
4. **Writer wired in** to `ApplyCoactivation` only — the hottest, most-instrumented entry point. Other Hebbian entry points (`ApplySymbolCoactivation`, `CoactivateSession`, `ApplyNegativeFeedback`) deferred to EVENTGRAPH-003.
5. **Federation helper** `internal/eventgraph/query.go::EventsInGraphNeighborhood` — two-step orchestrator: Neo4j path query → TSDB query for events touching the neighborhood → join in Go.
6. **API endpoint** `POST /v1/eventgraph/reinforcement-neighborhood` — exposes the federation helper for operators / external tools. Request gated by space membership; response capped at `EVENTGRAPH_MAX_EVENTS_PER_QUERY` rows.
7. **One Grafana panel** added to `mdemg-graph-topology.json` — "Reinforcement Event Rate per Space" (events/min over time, stacked by `trigger_path`). Lays the first paint stroke; later sprints can add panels for delta histograms and top-strengthened pairs.
8. **Configurability Contract** — every operator-visible value is dynamic (per `feedback_no_hardcoded_values.md`):
   - `EVENTGRAPH_ENABLED` (default `true` — feature flag for full disable)
   - `EVENTGRAPH_WRITER_FLUSH_INTERVAL_SEC` (default `30`, floor `5`)
   - `EVENTGRAPH_WRITER_BUFFER_SIZE` (default `1000`)
   - `EVENTGRAPH_MAX_PAIRS_PER_EVENT_BATCH` (default `200`, matches `LearningEdgeCapPerRequest` ceiling)
   - `EVENTGRAPH_MAX_EVENTS_PER_QUERY` (default `500`)
   - `EVENTGRAPH_FEDERATION_DEFAULT_HOPS` (default `2`)
   - `EVENTGRAPH_FEDERATION_DEFAULT_LOOKBACK_HOURS` (default `24`)
9. **Three testing tiers** (unit + integration + live e2e) per `feedback_mandatory_testing_tiers.md` + `feedback_live_testing_required.md`.
10. **Feature doc** `docs/features/event-graph-federation.md` per `feedback_per_feature_docs_required.md` (Why / Choices / How it works / How to use).

### Out of scope

- **Other Hebbian entry points** (`ApplySymbolCoactivation`, `CoactivateSession`, `ApplyNegativeFeedback`) — deferred to EVENTGRAPH-003 once the pattern proves out under production traffic.
- **Other event classes** (`GUIDANCE_OUTCOME` writes, consolidation/emergence events, constraint-conflict events) — each is a separate sprint in the EVENTGRAPH-00X series; this sprint establishes the pattern.
- **Pattern Y2 — link-node promotion in Neo4j.** Explicitly deferred; do not commit to it until a real query pattern proves federation-in-Go insufficient.
- **Backfill of historical reinforcement events.** Pre-V0022 history is lost permanently — there is no source to backfill from. Forward-only.
- **Dual-write reliability** — TSDB-first/Neo4j-best-effort or outbox patterns are not needed for Pattern Y1 (TSDB write is a fire-and-forget enqueue into the writer buffer; Neo4j is the source of truth for the graph state). Flagged here so reviewers don't waste cycles questioning it.
- **Read-side caching** of federation responses. Add when measured to matter.
- **Long-term retention policy** on `reinforcement_events`. Default TimescaleDB behavior (chunks forever) is fine for v1; revisit when chunk count exceeds 26 weeks.

### Constraints

- Sequential epics (per `feedback_sequential_epics.md`). Epic 1 (schema) lands before Epic 2 (writer); Epic 4 (wiring) lands after Epics 2 and 3.
- Tier 3 live e2e is required (per `feedback_live_testing_required.md`).
- No tight LLM budget caps — N/A here, $0 LLM spend (per `feedback_no_tight_llm_budget_caps.md`).
- `TSDB_REQUIRED_SCHEMA_VERSION` config default must bump 21 → 22 in `internal/config/config.go` (Epic 1).
- Writer drain on shutdown is required — the buffer must flush before the process exits (precedent: V0019 `flushLoop` + `Stop()` pattern).
- Feature flag (`EVENTGRAPH_ENABLED=false`) must short-circuit the writer call site cleanly so the Hebbian path stays identical to today when disabled.

---

## 4. Dependencies

- **Neo4j 5.x driver, Go pgxpool, TimescaleDB hypertables** — all in place. No new external deps.
- **V0021 model_install_events** (synchronous-single-row precedent) and **V0019 sparse_gate_metrics** (buffered CopyFrom precedent) — read both before drafting code. The new writer mirrors V0019.
- **`internal/learning/service.go::ApplyCoactivation`** — the wiring site. Currently RETURNs `count(*) AS updated`. Epic 3 extends this.
- **`mdemg-graph-topology.json`** Grafana dashboard — the panel addition site. Verify dashboard exists + the panel-add procedure works in the existing dev compose stack.
- **CMS / `mdemg-dev` space** must be running with real Neo4j + TSDB containers for Tier 3 (per `feedback_live_testing_required.md`).

---

## 5. Implementation Plan

### Epic 0 — Sprint plan + workspace prep (~0.1 day)

- Commit this plan file at `docs/development/eventgraph-001/sprint_plan_eventgraph_001.md`.
- No code touched in Epic 0. Open the dev branch off `main` if not already on `reh3376_dev01`.

### Epic 1 — TSDB schema migration V0022 (~0.2 day)

- Add `internal/tsdb/migrations/022_reinforcement_events.sql` (full schema below).
- Bump `internal/config/config.go` `tsdbRequiredSchemaVersion` default 21 → 22 (and update the comment).
- Run `mdemg tsdb migrate` against local stack — verify hypertable created, indexes created, `tsdb_schema_meta.value = '22'`.
- **Gate:** `mdemg tsdb status` shows `schema_version=22`; `\dt reinforcement_events` in psql shows hypertable; rerunning `mdemg tsdb migrate` is a no-op.

**Schema (V0022):**

```sql
-- Migration 022: reinforcement_events (Sprint EVENTGRAPH-001 — Pattern Y1)
-- Purpose: one row per Hebbian co-activation pair update written via
-- internal/learning/service.go::ApplyCoactivation. Buffered + flushed via
-- CopyFrom on the standard TSDB_FLUSH_INTERVAL_SEC cadence (default 30s).
-- Pattern: V0019 (sparse_gate_metrics) buffered writer, NOT V0021
-- (model_install_events) synchronous writer.
--
-- Federation API: POST /v1/eventgraph/reinforcement-neighborhood orchestrates
-- (Neo4j graph traversal) + (TSDB events touching the neighborhood) for the
-- "graph-walk + event context" query class.
--
-- Rollback (manual):
--   DROP TABLE IF EXISTS reinforcement_events CASCADE;
--   UPDATE tsdb_schema_meta SET value = '21' WHERE key = 'schema_version';

CREATE TABLE IF NOT EXISTS reinforcement_events (
    event_id              TEXT             NOT NULL,        -- CUIDv2
    recorded_at           TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    space_id              TEXT             NOT NULL,
    src_node_id           TEXT             NOT NULL,
    dst_node_id           TEXT             NOT NULL,
    prev_weight           DOUBLE PRECISION NOT NULL,
    new_weight            DOUBLE PRECISION NOT NULL,
    delta_weight          DOUBLE PRECISION NOT NULL,
    evidence_count_after  INTEGER          NOT NULL,
    eta_effective         DOUBLE PRECISION,
    surprise_factor       DOUBLE PRECISION,
    activation_product    DOUBLE PRECISION,
    path_sim              DOUBLE PRECISION,
    role_a                TEXT,
    role_b                TEXT,
    obs_type_a            TEXT,
    obs_type_b            TEXT,
    session_id            TEXT,
    direction             TEXT,                              -- 'forward' | 'reverse' | 'bidirectional'
    created_new_edge      BOOLEAN          NOT NULL,
    trigger_path          TEXT             NOT NULL,         -- 'apply_coactivation' for this sprint
    PRIMARY KEY (recorded_at, event_id)
);

SELECT create_hypertable('reinforcement_events', 'recorded_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_reinforcement_space_time
    ON reinforcement_events (space_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_reinforcement_src
    ON reinforcement_events (space_id, src_node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_reinforcement_dst
    ON reinforcement_events (space_id, dst_node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_reinforcement_session
    ON reinforcement_events (space_id, session_id, recorded_at DESC)
    WHERE session_id IS NOT NULL AND session_id <> '';

UPDATE tsdb_schema_meta SET value = '22' WHERE key = 'schema_version';
```

### Epic 2 — Buffered writer `internal/tsdb/reinforcement_writer.go` (~0.3 day)

- Mirror V0019 `SparseGateMetricsWriter`: `Enqueue(row)` (non-blocking, drops on full buffer with WARN), `Flush(ctx)` (auto-called by `flushLoop`), `Stop()` (drain + final flush). 
- Use `pool.CopyFrom` with `pgx.CopyFromRows`. Same `cuid2.Generate()` for `event_id`, same `time.Now()` for `recorded_at`.
- `flushSuccess` / `flushFailure` / `flushRows` atomics for Prometheus instrumentation later (3 gauges: `mdemg_eventgraph_writer_{success,failure,rows}_total`).
- Tier 1 unit tests: enqueue → flush → CopyFrom payload shape; buffer-full drop with WARN; Stop drains.
- **Gate:** unit tests pass; `go vet` clean; lint clean.

### Epic 3 — Cypher RETURN-shape change (~0.2 day)

- Edit `internal/learning/service.go::ApplyCoactivation` Cypher (around line 397–464) so the final `RETURN` shape exposes per-pair telemetry:
  ```cypher
  RETURN
    p.src AS src_node_id,
    p.dst AS dst_node_id,
    w AS prev_weight,
    r.weight AS new_weight,
    (r.weight - w) AS delta_weight,
    r.evidence_count AS evidence_count_after,
    ($eta * etaMult) AS eta_effective,
    surpriseFactor AS surprise_factor,
    prod AS activation_product,
    pathSim AS path_sim,
    roleA AS role_a,
    roleB AS role_b,
    obsTypeA AS obs_type_a,
    obsTypeB AS obs_type_b,
    coalesce(sessionA, sessionB, '') AS session_id,
    r.direction AS direction,
    (r.created_at = r.updated_at) AS created_new_edge
  ```
- Drain the result-set into a `[]ReinforcementEventRow` in Go inside `ExecuteWrite`, return alongside the existing error path.
- Symmetry edge (`MERGE (b)-[rr]->(a)`) recorded as a second row with `direction='reverse'` IF `LearningAsymmetricEnabled=true`, otherwise the forward row alone with `direction='bidirectional'` is the canonical event (avoids double-counting reinforcement when asymmetry is off).
- **Gate:** existing `internal/learning/service.go` tests still pass; new helper `parseReinforcementRows` has its own Tier 1 unit test against a fake `neo4j.ResultSummary`.

### Epic 4 — Wire writer into `ApplyCoactivation` (~0.2 day)

- Plumb `*tsdb.ReinforcementEventsWriter` into `learning.Service` (constructor injection, mirrors how `stabilityReinforcer` is set via `Set...` method — back-compat for tests that don't need it).
- After the `ExecuteWrite` returns, iterate the captured rows and call `writer.Enqueue(row)` for each. **No blocking** — buffer-full just drops with a Prometheus-counted warning.
- Gate the entire write at `cfg.EventGraphEnabled` so operators can fully disable the feature.
- Tier 2 integration test: run a real `Retrieve` against the local stack, verify N rows land in TSDB within `flush_interval + 1s`.
- **Gate:** integration test green; manual eyeball of a `SELECT ... FROM reinforcement_events ORDER BY recorded_at DESC LIMIT 20` after a retrieval.

### Epic 5 — Federation helper + API endpoint (~0.3 day)

- New package `internal/eventgraph/` with `query.go`:
  ```go
  type FederationRequest struct {
      SpaceID         string
      SeedNodeID      string
      Hops            int
      Since           time.Duration
      EventType       string  // "reinforcement" for v1; future: "guidance_outcome", etc.
      Limit           int
  }
  type EventWithContext struct {
      EventID       string
      RecordedAt    time.Time
      SrcNodeID     string
      DstNodeID     string
      DeltaWeight   float64
      NewWeight     float64
      Direction     string
      SessionID     string
      // Joined from graph traversal:
      SrcLayer      int
      DstLayer      int
      InNeighborhood bool  // true if both endpoints fell inside the N-hop graph walk
  }
  type FederationResult struct {
      Events          []EventWithContext
      NeighborNodeIDs []string
      GraphHops       int
      TSDBRowsScanned int
      Truncated       bool
  }
  func (s *Service) EventsInGraphNeighborhood(ctx context.Context, req FederationRequest) (*FederationResult, error)
  ```
- Step 1: Cypher graph walk (`MATCH (x:MemoryNode {node_id:$id})-[:CO_ACTIVATED_WITH|GENERALIZES*1..$hops]-(n)`) returns `node_id, layer` pairs.
- Step 2: TSDB query (`SELECT ... FROM reinforcement_events WHERE space_id=$1 AND (src_node_id = ANY($2) OR dst_node_id = ANY($2)) AND recorded_at > NOW() - $3::interval ORDER BY recorded_at DESC LIMIT $4`).
- Step 3: Go-side join: walk both lists, set `InNeighborhood` true, drop events where neither endpoint is in the neighborhood (defensive — shouldn't happen given the WHERE clause).
- HTTP handler at `internal/api/eventgraph_handler.go` — `POST /v1/eventgraph/reinforcement-neighborhood`, body validated, defaults from config, rate-limited via existing middleware. Authn = same `AUTH_API_KEYS` pattern as `/v1/admin/breakers` (gated when keys set, permissive when not — matches existing convention).
- **Decision deferred to execution per `feedback_plan_options_pattern.md`:**
  - **Option A — single endpoint with `event_type` query param.** Simpler, but commits to one URL shape across all future event classes.
  - **Option B — endpoint per event class** (`/reinforcement-neighborhood`, future `/guidance-outcome-neighborhood`, etc.). More REST-y, easier to evolve per-class semantics.
  - **Recommendation:** A for now (single URL, narrow query param), revisit when EVENTGRAPH-002 forces the question. Document the chosen approach in the PR description per the memory rule.
- **Gate:** `go test ./internal/eventgraph/...` green; manual `curl` against running server returns expected shape.

### Epic 6 — Grafana panel + Prometheus instrumentation (~0.15 day)

- Add 3 Prometheus gauges/counters exposed from the writer:
  - `mdemg_eventgraph_writer_rows_enqueued_total{trigger="apply_coactivation"}` (counter)
  - `mdemg_eventgraph_writer_rows_dropped_total{reason="buffer_full"}` (counter)
  - `mdemg_eventgraph_writer_flush_failure_total` (counter)
- Add one panel to `deploy/docker/grafana/dashboards/mdemg-graph-topology.json` — "Reinforcement Event Rate (events/min) by trigger_path", time-series, last-24h default, `space_id` template variable.
- Run `python3 scripts/grafana_panel_audit.py` (from GRAFANA-AUDIT-001) against the modified dashboard to verify the panel passes the audit.
- **Gate:** Grafana renders the panel against the live local stack with non-zero data; panel audit green.

### Epic 7 — Tier 3 live e2e (~0.15 day)

- Boot the full local stack (`docker compose up -d` + native `./bin/mdemg start --auto-migrate`).
- Issue a real retrieve against `mdemg-dev` space (e.g. `curl -X POST http://localhost:9999/v1/memory/retrieve -d '{"query":"...","space_ids":["mdemg-dev"]}'`).
- Within 30–35 seconds (one flush window), verify in psql:
  ```sql
  SELECT count(*), min(delta_weight), max(delta_weight), avg(new_weight)
  FROM reinforcement_events
  WHERE space_id='mdemg-dev' AND recorded_at > NOW() - INTERVAL '1 minute';
  ```
- Query the federation endpoint with a seed node from the retrieve and verify events returned correspond to the pairs the Hebbian write produced.
- Open Grafana, confirm the new panel shows non-zero events/min.
- Document the live-test transcript in `docs/development/eventgraph-001/verification.md`.
- **Gate:** all three observations confirmed; transcript written.

### Epic 8 — Documentation Update (final epic — never cut)

- **`docs/features/event-graph-federation.md`** (NEW, per `feedback_per_feature_docs_required.md`) — Why / Choices / How it works / How to use (with example `curl`). ~250 lines.
- **`CHANGELOG.md`** Unreleased: "Sprint EVENTGRAPH-001 — Reinforcement-event TSDB hypertable + graph federation."
- **`CLAUDE.md`** Architecture Notes: brief note under a new "Event Graph Federation (Pattern Y1)" subsection.
- **`docs/development/eventgraph-001/post.md`** — sprint close per the MODEL-DIST-002 precedent (epic-by-epic outcomes, acceptance-criteria check-off, surprise log, forward-looking).
- **`docs/development/eventgraph-001/verification.md`** — Epic 7 transcript.

---

## 6. Testing Plan — 3 tiers (required by memory rule)

### Tier 1 — Unit (target: 12–15 tests, all green before any commit)

- `internal/tsdb/reinforcement_writer_test.go`:
  - enqueue → buffer fills → Flush succeeds → buffer empty
  - buffer-full → next enqueue drops + emits WARN + increments drop counter
  - Stop() drains buffer + cancels ticker + idempotent
  - Flush with nil pool no-ops cleanly
  - Row serialization: nullable fields (`session_id=''`, `eta_effective=0`) → DB NULL
- `internal/learning/service_test.go` extension:
  - `parseReinforcementRows` against synthetic neo4j.Record values — verify per-pair row construction (prev/new weights, direction, created_new_edge)
  - Symmetry path: asymmetric mode → 2 rows per pair (forward + reverse); symmetric mode → 1 row (`direction='bidirectional'`)
- `internal/eventgraph/query_test.go`:
  - join logic: graph returns [A, B, C]; TSDB returns events on [A-B, B-D, C-E]; filter to events fully inside [A, B, C]
  - empty graph traversal → empty result (no TSDB call)
  - hops=0 special case → only seed node
  - `Limit` enforcement on truncation
  - `Since` clamped to ≥ 1 second
- `internal/api/eventgraph_handler_test.go`:
  - validation: missing space_id → 400
  - validation: hops < 0 or hops > MAX → 400
  - defaults applied: empty hops → cfg default; empty since → cfg default

### Tier 2 — Integration (target: 3 tests, run with `-tags=integration`)

- `tests/integration/eventgraph_writer_test.go`:
  - Boot real Neo4j + TSDB containers (existing testcontainer setup), apply V0022, run `ApplyCoactivation` against synthetic results, sleep one flush window, `SELECT count(*) FROM reinforcement_events` returns expected.
- `tests/integration/eventgraph_federation_test.go`:
  - Same setup + populate a small graph + emit ~10 reinforcement events + call `EventsInGraphNeighborhood` + assert event count + neighborhood inclusion.
- `tests/integration/eventgraph_writer_drain_test.go`:
  - Same setup + enqueue 100 rows + immediately call `Stop()` + verify all 100 land in TSDB (drain semantics).

### Tier 3 — Live e2e (Epic 7, required by `feedback_live_testing_required.md`)

- Real `mdemg` binary against the live Docker stack (Neo4j + TSDB + Grafana).
- Real `/v1/memory/retrieve` API call against the `mdemg-dev` space.
- Observe events in TSDB via `psql` query.
- Observe events in Grafana panel.
- Observe federation API returns sane events for a seed node from the retrieve.
- Transcript captured in `docs/development/eventgraph-001/verification.md`.

---

## 7. Commit Strategy

Sequential commits per epic on `reh3376_dev01`. The auto-PR workflow opens a single PR on first push (per `feedback_auto_pr.md`); subsequent pushes update the same PR.

1. **Epic 0** — `docs(eventgraph-001): sprint plan` (this file).
2. **Epic 1** — `feat(tsdb): V0022 reinforcement_events hypertable` (migration + config bump).
3. **Epic 2** — `feat(tsdb): buffered reinforcement_events writer` (writer + Tier 1 tests).
4. **Epic 3** — `refactor(learning): expose per-pair telemetry from Hebbian Cypher` (RETURN-shape change + parser + Tier 1 tests).
5. **Epic 4** — `feat(learning): record reinforcement events to TSDB` (wiring + Tier 2 integration test).
6. **Epic 5** — `feat(eventgraph): federation query helper + API endpoint` (Go package + handler + Tier 1 + Tier 2).
7. **Epic 6** — `feat(observability): Grafana panel + Prometheus counters for reinforcement events`.
8. **Epic 7** — `docs(eventgraph-001): Tier 3 live e2e verification` (verification.md only — no code).
9. **Epic 8** — `docs(eventgraph-001): feature doc + CHANGELOG + CLAUDE.md + post.md` (final epic).

A **sprint summary** is added to the PR comments after Epic 8 lands (per `feedback_sprint_summary_on_pr.md`).

---

## 8. Verification Checklist

- [ ] V0022 migration applied; `mdemg tsdb status` reports schema_version=22.
- [ ] `\d reinforcement_events` shows hypertable + 4 indexes + correct column types.
- [ ] Rerunning `mdemg tsdb migrate` is a no-op (idempotent).
- [ ] `internal/tsdb/reinforcement_writer.go` Tier 1 tests all green; `golangci-lint run ./...` clean.
- [ ] `internal/learning/service.go` Tier 1 tests still green after Cypher RETURN-shape change.
- [ ] `internal/eventgraph/...` Tier 1 + Tier 2 tests all green.
- [ ] `internal/api/eventgraph_handler_test.go` Tier 1 tests all green.
- [ ] Integration tests with `-tags=integration` all green against the local stack.
- [ ] Live e2e (Epic 7): real retrieve → events land in TSDB within ≤ 35 seconds.
- [ ] Live e2e: Grafana panel shows non-zero events/min after a retrieve.
- [ ] Live e2e: federation API returns events for a seed node touched by the retrieve.
- [ ] `python3 scripts/grafana_panel_audit.py` (from GRAFANA-AUDIT-001) passes against the modified dashboard.
- [ ] `EVENTGRAPH_ENABLED=false` short-circuits the writer call site (verified by toggling + checking retrieves produce zero events).
- [ ] `feature flag → buffer-full → drop with WARN` path exercised (test 2).
- [ ] `Stop()` drain verified by Tier 2 test.
- [ ] `docs/features/event-graph-federation.md` shipped.
- [ ] `CHANGELOG.md`, `CLAUDE.md`, `post.md`, `verification.md` updated.
- [ ] No banned no-tool-calling patterns introduced (`tool_use`, `tool_call`, etc. — per the CLAUDE.md FT-LORA policy, just a habit-check; this sprint has no LLM call sites).
- [ ] Sprint summary added to PR comments after Epic 8.

---

## 9. Documentation Update — Epic 8 above

See §5 Epic 8. The five docs touched: feature doc (new), CHANGELOG (Unreleased entry), CLAUDE.md (Architecture Notes subsection), post.md (sprint close), verification.md (Tier 3 transcript).

---

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Cypher RETURN-shape change breaks the existing `ApplyCoactivation` callers | Low | High | Callers consume only the error; `RETURN count(*)` was discarded. Per-pair RETURN drained into the new `parseReinforcementRows` helper inside the same `ExecuteWrite` block — no caller signature change. |
| Writer back-pressure on the Hebbian hot path | Low | Medium | Non-blocking `Enqueue` + bounded buffer + drop-with-WARN. Buffer size 1000 vs ~200 max pairs per retrieve = ≥5 retrieves of headroom before any drop. Prometheus counter on drops surfaces the problem before it hurts. |
| Per-retrieve row volume explodes TSDB chunks faster than chunk_time_interval anticipates | Medium | Low | 7-day chunks (same as V0017–V0021) absorb a lot of rows. At ~200 events/retrieve × ~100 retrieves/day ≈ 140k rows/week per chunk — well within Timescale's recommended chunk size. Add a 30-day retention policy in a follow-up sprint if chunks grow uncomfortably. |
| Federation API leaks information across spaces | Low | High | `space_id` is required in the request body + propagated to BOTH the Cypher walk WHERE clause AND the TSDB query WHERE clause. No cross-space joins possible. Tier 1 test asserts. |
| Asymmetric vs symmetric direction handling produces double-counted events | Medium | Medium | Epic 3 spec explicitly handles: asymmetric → forward + reverse rows; symmetric → single `direction='bidirectional'` row. Tier 1 test pins. |
| The federation API becomes a hot endpoint and adds Neo4j load | Low | Low | Gated by `EVENTGRAPH_MAX_EVENTS_PER_QUERY` (default 500) + `EVENTGRAPH_FEDERATION_DEFAULT_HOPS` (default 2). Rate-limit middleware exists. If load becomes a real concern, add a read-through cache (out of scope for v1). |
| Hebbian Cypher RETURN serialization slows the retrieval hot path | Low | Medium | Per-pair RETURN is ~17 cheap scalars per row × ~200 rows max = ~3.4k cells per query. Neo4j returns this trivially; the Go-side parse is O(N). Benchmark in Tier 2 if concerned; bail to a feature-flag-off if regression > 5%. |
| EVENTGRAPH-002+ sprints want to extend the writer to other event classes and the buffered-writer pattern can't be reused | Low | Low | Writer is parameterized by `trigger_path` column; same writer can ingest guidance-outcome events (with different per-row fields nullable). If the schema diverges too far, future sprints add a separate hypertable + writer. Pattern is precedent-validated by V0017/V0019. |

---

## 11. Documents Accessed

- `CLAUDE.md` — project context, testing rules, sprint plan v1.0 format reminder, per-feature docs rule, no-hardcoding rule, no-tight-LLM-budget-caps rule, sequential-epics rule
- `MEMORY.md` (auto-memory index) — mandatory workflow rules cross-checked
- `internal/learning/service.go` (lines 279–485, the Hebbian write path + `ApplyCoactivation`)
- `internal/tsdb/model_install_writer.go` (V0021 synchronous-single-row precedent, considered then rejected for this sprint's volume profile)
- `internal/tsdb/migrations/021_model_install_events.sql` (migration shape precedent)
- `internal/tsdb/sparse_gate_writer.go` (V0019 buffered-CopyFrom precedent — the pattern this sprint reuses)
- `internal/config/config.go` (lines 3828, 4617 — `tsdbRequiredSchemaVersion` default bump site)
- `internal/tsdb/migrations/019_sparse_gate_metrics.sql` and `020_context_catalog_versions.sql` (recent migration text for tone alignment)
- `deploy/docker/grafana/dashboards/mdemg-graph-topology.json` (panel-add site)
- `scripts/grafana_panel_audit.py` (audit harness used in Epic 6 gate)
- `docs/development/model-dist-002/sprint_plan_model_dist_002.md` (12-section structure exemplar)
- `docs/development/grafana-audit-001/` (sprint sibling — confirms `mdemg-graph-topology.json` is a real dashboard target)
- TypeDB vs Neo4j research summary from the current conversation (informed the Pattern Y1 vs Y2 decision)

---

## 12. Rollback Procedures

Pattern Y1 is intentionally non-destructive. Rollback paths:

- **Feature flag** (cheapest, no schema change): set `EVENTGRAPH_ENABLED=false` in `.env` and restart. Hebbian write path skips the writer call site entirely; existing data preserved; can re-enable any time.
- **Writer disable only** (keep data, stop new writes): same as feature flag.
- **API disable**: comment out the `POST /v1/eventgraph/reinforcement-neighborhood` route registration; data writer still functions.
- **Full schema rollback**: manual SQL per the migration header comment:
  ```sql
  DROP TABLE IF EXISTS reinforcement_events CASCADE;
  UPDATE tsdb_schema_meta SET value = '21' WHERE key = 'schema_version';
  ```
  Then revert `TSDB_REQUIRED_SCHEMA_VERSION` default to 21 in `internal/config/config.go`. Code that references the writer must also be reverted (revert the Epic 4 commit). Existing reinforcement event data is permanently lost — there is no source to backfill from. Forward-only.
- **Cypher RETURN-shape revert**: if the per-pair RETURN proves performance-problematic, revert Epic 3 commit. Writer remains, fed by an empty result loop. Either remove the writer (rollback Epic 4) or leave it dormant.

---

## Files to be created/modified (concrete inventory)

**New files (8):**
- `docs/development/eventgraph-001/sprint_plan_eventgraph_001.md` (this file — Epic 0)
- `docs/development/eventgraph-001/verification.md` (Epic 7)
- `docs/development/eventgraph-001/post.md` (Epic 8)
- `internal/tsdb/migrations/022_reinforcement_events.sql` (Epic 1)
- `internal/tsdb/reinforcement_writer.go` (Epic 2)
- `internal/tsdb/reinforcement_writer_test.go` (Epic 2 — Tier 1)
- `internal/eventgraph/query.go` (Epic 5)
- `internal/eventgraph/query_test.go` (Epic 5 — Tier 1)
- `internal/api/eventgraph_handler.go` (Epic 5)
- `internal/api/eventgraph_handler_test.go` (Epic 5 — Tier 1)
- `tests/integration/eventgraph_writer_test.go` (Epic 4 — Tier 2)
- `tests/integration/eventgraph_federation_test.go` (Epic 5 — Tier 2)
- `tests/integration/eventgraph_writer_drain_test.go` (Epic 4 — Tier 2)
- `docs/features/event-graph-federation.md` (Epic 8)

**Modified files (6):**
- `internal/config/config.go` — bump `tsdbRequiredSchemaVersion` 21→22; add 7 new `EVENTGRAPH_*` env knobs; wire `EventGraphEnabled` + writer config struct
- `internal/learning/service.go` — Cypher RETURN-shape change in `ApplyCoactivation`; new `parseReinforcementRows` helper; constructor injection for writer
- `internal/learning/service_test.go` — new Tier 1 tests for the parser + symmetry-direction handling
- `internal/api/server.go` — register the new `/v1/eventgraph/reinforcement-neighborhood` route; construct + own the writer's lifecycle (start at boot, Stop at shutdown)
- `deploy/docker/grafana/dashboards/mdemg-graph-topology.json` — one new panel + audit-passing rev
- `CHANGELOG.md` — Unreleased entry
- `CLAUDE.md` — Architecture Notes subsection
- `.env.example` — document the 7 new env vars

---

## Acceptance Criteria

1. Operator running a real `/v1/memory/retrieve` against the local stack sees corresponding rows land in `reinforcement_events` within ≤ 35 seconds.
2. `POST /v1/eventgraph/reinforcement-neighborhood` with a seed node from that retrieve returns events whose `src_node_id` / `dst_node_id` are within the requested `hops`-neighborhood of the seed.
3. The Grafana panel "Reinforcement Event Rate" shows non-zero events/min during retrieval load.
4. Toggling `EVENTGRAPH_ENABLED=false` + restart cleanly disables the writer (zero rows on subsequent retrieves) without breaking the Hebbian write itself (graph still updates).
5. `mdemg tsdb status` reports `schema_version=22` post-Epic-1, and `mdemg tsdb migrate` is idempotent.
6. `docs/features/event-graph-federation.md` exists and is accurate.
7. No Tier 1 or Tier 2 regression in the existing `internal/learning/service_test.go` or `tests/integration/` suites.
