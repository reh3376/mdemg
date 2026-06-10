# Sprint EVENTGRAPH-003 — Reinforcement-Event Coverage for the Other Hebbian Paths

## 1. Header & Metadata

- **Sprint ID:** EVENTGRAPH-003
- **Sprint line:** `docs/development/eventgraph-003/`
- **Date opened:** 2026-06-09
- **Target version:** v0.10.x (additive — no schema change, no new endpoint)
- **Estimated effort:** ~1 dev-day
- **OpenAI spend:** $0
- **Risk level:** Low–Medium (the work is in live Hebbian-write Cypher; the risk is a RETURN-clause refactor altering the write behavior — covered by Tier-2 weight-update assertions + Tier-3 live smoke)
- **Predecessors:** EVENTGRAPH-001 (the `reinforcement_events` hypertable + writer + the one wired path), EVENTGRAPH-CLI-001 + EVENTGRAPH-002 (the federation read consumers — they pick up the new rows for free).

## 2. Problem Statement

EVENTGRAPH-001 federated Hebbian reinforcement events, but wired **only one** of the four Hebbian write paths — `ApplyCoactivation` (L0 code-pair co-activation). The other three weight-update paths fire `CO_ACTIVATED_WITH` writes that are **invisible** to `reinforcement_events` (and therefore to the federation API + `mdemg eventgraph reinforcement-neighborhood`):

- **`CoactivateSession`** — session-internal co-activation between conversation observations (full Hebbian formula).
- **`ApplySymbolCoactivation`** — co-activation between `SymbolNode` pairs derived from a retrieve.
- **`ApplyNegativeFeedback`** — *weakening* of co-activation edges on rejected results (negative delta).

So the federation's view of "what reinforced this neighborhood" is incomplete. EVENTGRAPH-003 closes that gap by emitting reinforcement events from all three, distinguished by `trigger_path`. The V0022 schema was explicitly designed for this extension (`trigger_path` column; signed `delta_weight`; `created_new_edge` bool).

## 3. Scope & Constraints

**In scope:**
- Wire `CoactivateSession`, `ApplySymbolCoactivation`, and `ApplyNegativeFeedback` (**weaken path only**) into the existing `reinforcementWriter` — extend each Cypher's `RETURN` to emit the per-pair telemetry, parse it, and `Record()` it with a distinct `trigger_path`:
  - `coactivate_session`
  - `apply_symbol_coactivation`
  - `apply_negative_feedback`
- Reuse `reinforcement_parser.go::parseReinforcementRow` (the RETURN field names match; NULLs handled for N/A fields).
- Tests (3 tiers) + docs.

**Out of scope (data-decided — see §10):**
- **No new TSDB migration / schema-version bump.** V0022 `reinforcement_events` already carries `trigger_path` + signed `delta_weight` + `created_new_edge`. Adding new `trigger_path` values needs no schema change.
- **The `ApplyNegativeFeedback` *contradict* path** (creates a `CONTRADICTS` edge, not a `CO_ACTIVATED_WITH` reinforcement). Deferred: (a) `delta_weight` for a *new contradiction* edge doesn't map cleanly to the weight-delta contract, and (b) the federation walk only traverses `CO_ACTIVATED_WITH|GENERALIZES`, so a `CONTRADICTS` event couldn't be graph-contextualized anyway. A future sprint can federate contradictions as their own event class (the EVENTGRAPH-002 guidance-outcome pattern).
- No change to the writer, the federation API, the CLI, or the migration. No new config.

**Constraints:**
- Sequential epics, docs before implementation (memory: `feedback_sequential_epics.md`).
- Live Tier-3 required: trigger each path against the real system, observe rows in TSDB + via the federation read (memory: `feedback_live_testing_required.md`).
- **The Cypher edits must not change the weight-update behavior** — only add a `RETURN`. Tier-2 asserts the same edge weights as before.
- No hardcoded values; `trigger_path` literals are the only new constants (they're the schema's discriminator, mirroring v1's `"apply_coactivation"`).

## 4. Dependencies

- All three functions are in `internal/learning/service.go::Service` (same service as `ApplyCoactivation`), and `s.reinforcementWriter` is **already injected** (`SetReinforcementWriter` at `service.go:80`, wired in `server.go:1319`) — **no new wiring**.
- `internal/tsdb/reinforcement_writer.go` (`ReinforcementEventRow` + `Record`) + `internal/learning/reinforcement_parser.go` (`parseReinforcementRow`) — reused as-is.
- `EVENTGRAPH_ENABLED` already gates the writer's construction; when off, `s.reinforcementWriter == nil` and the new `Record` calls no-op (same guard as the existing path).

## 5. Implementation Plan

**Epic 0 — Sprint plan (this doc).**

**Epic 1 — `CoactivateSession` (lowest friction).**
- Replace the terminal `RETURN count(*) AS edges_created` with a per-pair `RETURN` emitting the standard 17 fields (all already in scope: `a/b.node_id`, pre-SET `w`, post-SET `r.weight`, `evidence_count`, `$eta`, `surpriseFactor`, `prod=activation*activation`, NULL `path_sim`, `'conversation_observation'` roles, `obs_type`, `$sessionId`, `'bidirectional'`, `created_new_edge = (r.evidence_count = 1)`). Emit **one row per pair** (the forward edge — the reverse is a mirror; avoids double counting). Keep an aggregate count for the function's own return value if callers use it.
- Iterate the result rows post-`ExecuteWrite`, `parseReinforcementRow` → `Record` with `TriggerPath: "coactivate_session"`.

**Epic 2 — `ApplySymbolCoactivation` (medium).**
- Capture the pre-`SET` weight: add a `WITH` carrying `r.weight AS prevWeight` *before* the `ON MATCH SET` recomputes it (the one structural subtlety the map flagged), then `RETURN` the standard fields with `eta/surprise/activation/path_sim = NULL` (legitimately N/A for symbols), roles `'symbol_node'`, `created_new_edge = (r.evidence_count = 1)`.
- Parse + `Record` with `TriggerPath: "apply_symbol_coactivation"`.

**Epic 3 — `ApplyNegativeFeedback` (weaken-only).**
- Refactor the weaken branch out of the `FOREACH` into a `MATCH (q)-[coact:CO_ACTIVATED_WITH]->(r)` … `WITH coact.weight AS prevWeight, <clamped> AS newWeight` `SET coact.weight = newWeight` `RETURN …` form so it emits per-pair rows with a **negative `delta_weight`** (`newWeight - prevWeight`), `created_new_edge = false`, NULL eta/surprise/activation/path_sim. Leave the contradict `FOREACH`/`MERGE (q)-[:CONTRADICTS]->(r)` branch **behaviorally unchanged** (just not emitted).
- Parse + `Record` with `TriggerPath: "apply_negative_feedback"`.

**Epic 4 — Live Tier-3 + docs + close.**
- Trigger each of the three paths against the running stack; observe the new `trigger_path` rows in `reinforcement_events`; confirm they surface via `mdemg eventgraph reinforcement-neighborhood`. Update the feature doc, CHANGELOG, CLAUDE.md, verification.md, post.md.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** `parseReinforcementRow` already covered; add cases for the NULL-heavy symbol/negative rows (optional eta/surprise/path_sim → nil) and the negative `delta_weight` sign. `-race`.
- **Tier 2 (integration, live Neo4j+TSDB):** for each path — seed the precondition, call the function, assert (a) the **edge weights are unchanged vs the pre-sprint behavior** (the RETURN refactor is side-effect-free), and (b) a `reinforcement_events` row lands with the right `trigger_path`, signs, and `created_new_edge`.
- **Tier 3 (live e2e):** drive each path through the real system (a retrieve → symbol co-activation; a session co-activation cycle; a negative-feedback/rejection), then `SELECT trigger_path, count(*), … FROM reinforcement_events` and a `mdemg eventgraph reinforcement-neighborhood` run showing the new events in a neighborhood.

## 7. Commit Strategy

Sequential commits per epic on `reh3376_dev01`. Surprise live-smoke bugs get their own fix-commit. Final epic updates docs.

## 8. Verification Checklist

- [ ] `CoactivateSession` emits `coactivate_session` rows; full Hebbian fields populated
- [ ] `ApplySymbolCoactivation` emits `apply_symbol_coactivation` rows; pre-SET weight captured correctly; symbol-N/A fields NULL
- [ ] `ApplyNegativeFeedback` (weaken) emits `apply_negative_feedback` rows with **negative** `delta_weight`, `created_new_edge=false`; contradict branch unchanged + not emitted
- [ ] **Edge weights identical to pre-sprint** for all three (RETURN-only refactor — Tier 2)
- [ ] No schema change; `TSDB_REQUIRED_SCHEMA_VERSION` unchanged; writer/federation/CLI untouched
- [ ] **Live smoke:** trigger all three; observe their `trigger_path` rows in `reinforcement_events` and via `mdemg eventgraph reinforcement-neighborhood`
- [ ] `golangci-lint run ./...` clean; full `go test` green
- [ ] Feature doc + CHANGELOG + CLAUDE.md + post.md updated

## 9. Documentation Update

Epic 4. `docs/features/event-graph-federation.md`: update "the writer captures `ApplyCoactivation` only" → all four paths, with the `trigger_path` value table. CHANGELOG + CLAUDE.md note. Per the per-feature-docs rule.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| RETURN refactor changes a weight-update side effect | Med | High | Tier-2 asserts identical post-call edge weights; the `SET` stays, only `RETURN` is added |
| `ApplySymbolCoactivation` pre-SET weight captured wrong (off-by-one on create vs match) | Med | Med | Tier-2 checks `prev/new/delta` on both a fresh create and a re-activation |
| Negative `delta_weight` mis-signed | Low | Med | Tier-1 sign assertion + Tier-2 weaken case |
| Volume: `CoactivateSession` can emit many pairs per call | Low | Low | Buffered writer (V0019 pattern) already handles per-retrieve Hebbian volume; `EVENTGRAPH_WRITER_BUFFER_SIZE` caps it |
| Contradict events expected but absent | Med | Low | Documented out-of-scope + the deferral rationale; federation can't traverse `CONTRADICTS` anyway |

## 11. Documents Accessed

- `internal/learning/service.go` (`ApplyCoactivation` ~294 the template; `ApplySymbolCoactivation` ~540; `CoactivateSession` ~661; `ApplyNegativeFeedback` ~902; `SetReinforcementWriter` ~80)
- `internal/learning/reinforcement_parser.go`; `internal/tsdb/reinforcement_writer.go`; `internal/tsdb/migrations/022_reinforcement_events.sql`
- `internal/api/server.go` (~1319 writer injection)
- `docs/features/event-graph-federation.md`; EVENTGRAPH-001/002 sprint lines

## 12. Rollback Procedures

- Each path is independent: revert that epic's commit to drop its `Record` call + restore the original `RETURN`. No schema/data migration to undo (rows already written remain valid `reinforcement_events`, just stop accumulating for that `trigger_path`). No writer/endpoint/CLI changes to roll back.
