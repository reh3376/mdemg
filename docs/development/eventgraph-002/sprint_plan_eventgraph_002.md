# Sprint EVENTGRAPH-002 — Guidance-Outcome Federation

## 1. Header & Metadata

- **Sprint ID:** EVENTGRAPH-002
- **Sprint line:** `docs/development/eventgraph-002/`
- **Date opened:** 2026-06-08
- **Target version:** v0.10.x (additive — new read-only federation endpoint + CLI + one additive index migration)
- **Estimated effort:** ~1 dev-day
- **OpenAI spend:** $0
- **Risk level:** Low–Medium (reuses an existing populated table; the one risk is the graph↔event join-key correctness, mitigated by live Tier-3 verification against the 1176 live `constraint_outcomes` rows)
- **Predecessors:** EVENTGRAPH-001 (federation pattern + reinforcement endpoint), EVENTGRAPH-CLI-001 (first consumer + CLI harness), RRF-SCALE-001 (created the `constraint_outcomes` TSDB sink), JIMINY-OUTCOME-001 (assigns `constraint_code` to outcomes via embedding match)

## 2. Problem Statement

EVENTGRAPH-001 federated **one** event class — Hebbian reinforcement events (`reinforcement_events`). The federation thesis (Pattern Y1) is that *any* "events about edges/nodes" class can be federated: events in TSDB, graph traversal in Neo4j, joined in Go. The second event class named in the EVENTGRAPH-001 forward-look is **guidance outcomes** (followed / ignored / contradicted feedback on surfaced guidance).

Today there is no way to ask: *"How well is this constraint — **and its graph-related constraints** — being followed over a time window?"* The existing read-side, `GetConstraintEffectiveness`, aggregates `GUIDANCE_OUTCOME` edges **per single constraint**, with no graph-neighborhood scope and no time-series. The raw, time-windowed outcome stream already lives in TSDB (`constraint_outcomes`, populated by every `/v1/jiminy/feedback` call), but nothing federates it against the graph. EVENTGRAPH-002 closes that gap.

## 3. Scope & Constraints

**In scope:**
- One additive TSDB migration (V0023): a `(space_id, constraint_code, time DESC)` index on the existing `constraint_outcomes` table to support the federation join. Bump `TSDB_REQUIRED_SCHEMA_VERSION` 22→23.
- A new federation query method `GuidanceOutcomesInNeighborhood` in `internal/eventgraph/` mirroring `EventsInGraphNeighborhood`.
- A new HTTP handler + route `POST /v1/eventgraph/guidance-outcome-neighborhood`.
- A new CLI subcommand `mdemg eventgraph guidance-outcome-neighborhood`.
- A new UATS contract spec (tagged `tsdb`).
- Feature doc section, CHANGELOG, CLAUDE.md, verification.md, post.md.

**Out of scope (data-decided — see §10):**
- A new `guidance_outcome_events` hypertable / writer / enqueue site. The data already lives in `constraint_outcomes` (written by the Jiminy outcome path). Creating a parallel sink would duplicate data and violate the no-duplication rule (memory: `feedback_no_hardcoded_values.md` sibling principle). **Decision: reuse `constraint_outcomes`.**
- Federating the Neo4j `GUIDANCE_OUTCOME` edges directly. Per the Pattern Y1 thesis, the *event stream* lives in TSDB; the Neo4j edges are the aggregate sink consumed by `GetConstraintEffectiveness`. Federate the TSDB stream.
- EVENTGRAPH-003 (the other three Hebbian entry points) — separate sprint.
- Any change to how outcomes are recorded or classified (that's JIMINY-OUTCOME-001 / GUIDANCE-SYNTH-001 territory).

**Constraints:**
- Sequential epics, docs before implementation (memory: `feedback_sequential_epics.md`).
- Live Tier-3 testing required (memory: `feedback_live_testing_required.md`).
- No hardcoded values — reuse the generic federation config knobs; no literal hops/lookback/limit copies in CLI or handler (memory: `feedback_no_hardcoded_values.md`).
- CUIDv2 for any new identifiers (memory: `feedback_cuidv2_required.md`) — N/A here (no new rows written).
- UxTS: every new endpoint gets a UATS spec (the standing UxTS directive).

## 4. Dependencies

- **Existing on `main`:** `constraint_outcomes` table (migration 011) + its writer (`internal/tsdb/constraint_outcomes_writer.go`), populated live (verified: 1176 rows / 223 constraints / latest today). `eventgraph.Service` (driver + pool) already constructed + wired in `server.go`. The reinforcement federation handler/CLI/UATS as the template.
- **Join key:** `constraint_code` — carried by both Neo4j `role_type='constraint'` nodes (property) and `constraint_outcomes` rows (column). Verified live: Neo4j `node_id` (CUID) ≠ TSDB `constraint_id` (UUID), so `constraint_code` is the **only** viable join key.
- **No external tooling. No OpenAI. No new Python.**

## 5. Implementation Plan

**Epic 0 — Sprint plan (this doc).** Commit to `docs/development/eventgraph-002/`.

**Epic 1 — V0023 migration (constraint_code index).**
- `internal/tsdb/migrations/023_constraint_outcomes_code_index.sql`: `CREATE INDEX IF NOT EXISTS idx_constraint_outcomes_code ON constraint_outcomes (space_id, constraint_code, time DESC);` + schema-version row bump to 23 (match the V0022 migration's version-update tail).
- Bump `TSDB_REQUIRED_SCHEMA_VERSION` default 22→23 in `internal/config/config.go` (and any deploy configs / compose templates that pin it — grep `TSDB_REQUIRED_SCHEMA_VERSION` and `schema_version`), per the TSDB migration checklist (memory: `project_tsdb_schema_version_ci_check.md`) so the CI validator passes.
- Tier 1/2: migration applies idempotently; `mdemg tsdb status` reports v23; index present (`\d constraint_outcomes`).

**Epic 2 — Federation query method.**
- New file `internal/eventgraph/guidance_outcomes.go`:
  - `GuidanceOutcomeRequest{ SpaceID, SeedNodeID, Hops, Since, Limit }` (mirror `FederationRequest`).
  - `GuidanceOutcomeWithContext{ EventID? (constraint_id+guidance_id), Time, ConstraintID, ConstraintCode, GuidanceID, SessionID, OutcomeType, Similarity, GuidanceType, ConstraintNodeID, InNeighborhood }`.
  - `GuidanceOutcomeResult{ Outcomes []…, NeighborNodeIDs []string, NeighborConstraintCodes []string, GraphHops, TSDBRowsScanned, Truncated }` (non-nil slices — the EVENTGRAPH-CLI-001 `null`→`[]` lesson baked in from the start).
  - `(s *Service) GuidanceOutcomesInNeighborhood(ctx, req)`:
    1. `walkNeighborhoodConstraintCodes` — reuse/extend the neighborhood walk to also return each neighbor's `constraint_code` (filter to nodes carrying one). Returns `(nodeIDs []string, codes []string)`.
    2. Query `constraint_outcomes WHERE space_id=$1 AND constraint_code = ANY($2) AND time > NOW() - $3::interval ORDER BY time DESC LIMIT $4`.
    3. Go-side join: stamp `InNeighborhood` (true — all returned rows match a neighborhood code; field kept for parity + future widening) and resolve `ConstraintNodeID` from the code→node map built in step 1.
  - Validation parity (space_id/seed required, hops≥0, Since floor, Limit default).
- Tier 1: validation guards, empty-arrays-not-null marshal test, the code-collection join logic factored + tested.
- Tier 2: integration test (`-tags=integration`, skip-on-empty) against live Neo4j+TSDB.

**Epic 3 — HTTP handler + route.**
- `internal/api/eventgraph_handler.go` (or sibling file): `handleEventgraphGuidanceOutcomeNeighborhood` — same shape as the reinforcement handler (method guard → `EVENTGRAPH_ENABLED` gate → service-nil 503 → parse → required fields → config defaults via the shared `EventGraphFederationDefault*` + `EventGraphMaxEventsPerQuery` → service call → writeJSON).
- Request struct with `*int`/`*int64` optional fields (omit-when-unset).
- Route: `mux.HandleFunc("/v1/eventgraph/guidance-outcome-neighborhood", …)` in `server.go`.

**Epic 4 — CLI subcommand.**
- `internal/cli/eventgraph.go`: add `newEventgraphGuidanceOutcomeNeighborhoodCmd()` as a sibling under `newEventgraphCmd()`.
- Flags: `--seed`/`--query`/`--hops`/`--since`/`--limit`/`--json`/`--space-id` (parallel to reinforcement). **Option (pick at execution):** add `--constraint-code <code>` to resolve the seed from a constraint node by code (natural entry for effectiveness queries). Omit-when-unset for hops/since/limit.
- Render: summary (neighborhood nodes · constraint codes · outcomes · followed/ignored split) + table (CODE · OUTCOME · similarity · guidance_id · session · recorded) or `--json`.
- Tier 1: flag→request mapping (omit-when-unset), `--query` seed resolution (httptest), render (empty + table), errors.

**Epic 5 — UATS contract spec.**
- `docs/api/api-spec/uats/specs/guidance_outcome_neighborhood.uats.json` (tagged `tsdb` so CI skips without TSDB — the EVENTGRAPH-CLI-001 lesson): happy-200 response shape, missing space_id/seed → 400, negative hops → 400, over-ceiling → 400, GET → 405. `add-hashes`; validate live 6/6.

**Epic 6 — Tier 3 live e2e.**
- Run the real binary against the live stack: `--query` resolving a constraint seed → surface real outcomes; `--seed` + `--constraint-code` (if shipped) forms; `--json`; empty/unknown; no-arg error. Document in `verification.md` with real output (we have 1176 live rows incl. `no-direct-main-commits` / `mandatory-use-cms-every-session`).

**Epic 7 — Documentation (final, never cut).**
- `docs/features/event-graph-federation.md`: new "Guidance-Outcome Federation" subsection + CLI section addition + forward-look update.
- `CHANGELOG.md` Unreleased: Added (endpoint + CLI) + Changed (TSDB schema 22→23).
- `CLAUDE.md`: extend the Event Graph Federation architecture note.
- `docs/development/eventgraph-002/post.md` — sprint close + UxTS mapping + follow-ups.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** `internal/eventgraph/guidance_outcomes_test.go` (validation, empty-arrays-not-null, code-collection join); `internal/cli/eventgraph_test.go` additions (new subcommand request-mapping, seed resolution, render, errors). `-race` clean.
- **Tier 2 (integration / contract):** `-tags=integration` federation test against live Neo4j+TSDB (skip-on-empty); UATS spec validated live (Epic 5).
- **Tier 3 (live e2e):** real `mdemg eventgraph guidance-outcome-neighborhood` against the running server, observing real outcomes in the table/JSON, cross-checked against a direct `constraint_outcomes` SQL query (Epic 6).

## 7. Commit Strategy

Sequential commits per epic on `reh3376_dev01`. Any surprise bug caught in live smoke gets its own fix-commit (the EVENTGRAPH-CLI-001 / Phase 11.6.2 precedent). Final epic updates CHANGELOG/CLAUDE.md/post.

## 8. Verification Checklist

- [ ] V0023 migration applies idempotently; `mdemg tsdb status` → v23; `idx_constraint_outcomes_code` present
- [ ] `TSDB_REQUIRED_SCHEMA_VERSION` bumped to 23 in config + all deploy configs; CI schema validator green
- [ ] `GuidanceOutcomesInNeighborhood` joins on `constraint_code`; non-nil slices; Tier 1 + Tier 2 green
- [ ] `POST /v1/eventgraph/guidance-outcome-neighborhood`: 200 happy / 400 required-field / 400 hops / 405 GET
- [ ] CLI `--seed`/`--query`(/`--constraint-code`)/`--json`/empty/error all correct live
- [ ] No hardcoded hops/since/limit defaults in CLI or handler (omit-when-unset; server config)
- [ ] **Live smoke:** run the CLI against the real system, observe real guidance outcomes (followed/ignored) for a constraint's neighborhood, confirm the rows match a direct `constraint_outcomes` SQL query
- [ ] UATS 6/6 live; sha256 verified; spec tagged `tsdb`
- [ ] `golangci-lint run ./...` clean; full `go test` green
- [ ] Feature doc + CHANGELOG + CLAUDE.md + post.md updated

## 9. Documentation Update

Epic 7 above. Feature doc gains a "Guidance-Outcome Federation" subsection (Why / How it works / CLI usage), the forward-look notes EVENTGRAPH-002 shipped, and CLAUDE.md's architecture note is extended. Per the per-feature-docs rule (memory: `feedback_per_feature_docs_required.md`).

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Join-key wrong (node_id vs constraint_code) produces empty/incorrect federation | Low (verified live pre-plan) | High | Join on `constraint_code` (the verified key); Tier 3 cross-checks CLI output against a direct SQL query on the same window |
| `constraint_code` index missing → seq scan as table grows | — | Med | V0023 adds `(space_id, constraint_code, time DESC)` index (Epic 1) |
| Outcomes with empty `constraint_code` can't be joined (won't appear in a neighborhood) | Med | Low | Documented limitation; these are pre-JIMINY-OUTCOME-001 / unmatched outcomes; federation surfaces coded outcomes by design |
| Schema-version bump breaks a deploy config the CI validator checks | Low | Med | Grep all `TSDB_REQUIRED_SCHEMA_VERSION` / `schema_version` sites in Epic 1; run the validator locally before push |
| Multiple Neo4j nodes share a `constraint_code` (verified: dup constraints exist) | Certain | Low | Federation collects the *set* of codes in the neighborhood; dedup codes before the `ANY(...)` query; semantics = "outcomes for codes present in the neighborhood" |

## 11. Documents Accessed

- `internal/tsdb/migrations/011_constraint_outcomes.sql` (table + index baseline)
- `internal/tsdb/migrations/022_reinforcement_events.sql` (migration + schema-version-bump template)
- `internal/tsdb/constraint_outcomes_writer.go`, `internal/tsdb/reinforcement_writer.go`
- `internal/eventgraph/query.go` (federation template), `internal/api/eventgraph_handler.go`, `internal/cli/eventgraph.go`
- `internal/jiminy/persistence.go` (`PersistGuidanceOutcome`, `GetConstraintEffectiveness`), `internal/jiminy/service.go` (`RecordOutcome`)
- `internal/config/config.go` (EVENTGRAPH_* + TSDB_REQUIRED_SCHEMA_VERSION)
- `docs/api/api-spec/uats/specs/eventgraph_reinforcement_neighborhood.uats.json` (UATS template)
- `docs/features/event-graph-federation.md`
- Live: `constraint_outcomes` row sample (UUID constraint_id, real constraint_code); Neo4j `role_type='constraint'` nodes (CUID node_id, `constraint_code` property)

## 12. Rollback Procedures

- **Migration:** `DROP INDEX IF EXISTS idx_constraint_outcomes_code;` + revert `TSDB_REQUIRED_SCHEMA_VERSION` to 22. Index-only, no data change — safe.
- **Endpoint/CLI/handler:** revert the additive commits; no other surface depends on them.
- **Config:** the federation knobs reused are pre-existing; nothing to roll back.
