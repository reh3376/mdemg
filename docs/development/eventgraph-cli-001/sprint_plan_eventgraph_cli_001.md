# Sprint EVENTGRAPH-CLI-001 — Federation Consumer CLI (`mdemg eventgraph`)

> **Status:** DRAFT — awaiting user approval before implementation.
> **Type:** Feature (first consumer for the EVENTGRAPH-001 federation API; live-testing harness for the EVENTGRAPH line).

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| **Sprint ID** | EVENTGRAPH-CLI-001 |
| **Sprint line** | `docs/development/eventgraph-cli-001/` |
| **Date opened** | 2026-06-08 |
| **Target version** | v0.12.0 (minor — new CLI surface) |
| **Estimated effort** | ~1–1.5 dev-days (CLI + a backfilled UATS contract spec for the federation API) |
| **OpenAI / LLM spend** | $0 |
| **Risk level** | Low. Read-only CLI consumer of an existing HTTP API; no schema/data mutation; mirrors the established `synergy.go` cobra pattern. |
| **Priority** | P2 — strategic enabler. EVENTGRAPH-001 shipped the `/v1/eventgraph/reinforcement-neighborhood` federation API with **no consumer**. This builds the first consumer, which (a) validates the Pattern Y1 bet (graph-walk + event context is actually useful), and (b) becomes the **live-testing harness** for EVENTGRAPH-002 (guidance-outcome federation) and -003 (complete reinforcement coverage). Per the user: build the consumer first to maximize utility + enable live testing for the development that follows. |

## 2. Problem Statement

EVENTGRAPH-001 federates "events about edges" into TSDB and exposes them via `POST /v1/eventgraph/reinforcement-neighborhood` (graph-walk from a seed → TSDB events touching the neighborhood → Go-side join). The API is healthy and the `reinforcement_events` data flows (verified: a code-node retrieve produced +20 events). But **nothing consumes the federation API** — no Grafana panel, hook, CLI, or tool calls it. So:

1. The Pattern Y1 value proposition is **unvalidated** — we've built the query layer but never queried it operationally.
2. There's **no harness** to live-test federation behavior, which the next sprints (-002/-003) will need to verify their new event classes + writers end-to-end.

The federation API is HTTP/JSON, so a **CLI command** is the natural consumer: operator-facing, scriptable (`--json`), and runnable on demand against the live system — exactly the live-testing surface the EVENTGRAPH line needs. (A Grafana panel can't consume it — Grafana's datasources are postgres/neo4j/nodegraph, not arbitrary HTTP.)

## 3. Scope & Constraints

### In scope
1. **`mdemg eventgraph reinforcement-neighborhood`** (cobra subcommand under a new `eventgraph` parent), mirroring the `synergy.go` pattern:
   - Flags: `--seed <node_id>` (required unless `--query`), `--query <text>` (convenience: resolve the seed to the top retrieval result), `--space-id` (default mdemg-dev), `--hops` (default: server default), `--since <dur>` (e.g. `24h`, default: server default), `--limit`, `--json`.
   - POST `/v1/eventgraph/reinforcement-neighborhood` with the resolved request.
   - Render: a human summary (neighborhood size, hops, events returned, TSDB rows scanned, truncated) + an events table (src→dst short IDs, Δweight, new_weight, direction, new-edge flag, src/dst-in-neighborhood, recorded_at), OR raw JSON with `--json`.
2. **`--query` seed resolution** (convenience + the key live-testing ergonomic): when `--seed` is absent and `--query` is given, call `/v1/memory/retrieve` and use the top result's `node_id` as the seed. Lets an operator test federation from a natural-language query without hand-copying node IDs.
3. **Register** the command in `root.go`.
4. **UATS contract spec for the federation API** (`eventgraph_reinforcement_neighborhood.uats.json`) — backfills the EVENTGRAPH-001 gap (the endpoint shipped with **no** UATS coverage). This is the project's API-contract framework and the right tool for the HTTP contract the CLI depends on; it **replaces** an ad-hoc Go integration test. Validated via `make test-api` / `uats_runner.py validate`, hashed via `add-hashes`.
5. **3-tier testing** — Tier 1 (CLI flag/request/render Go units), Tier 2 = the **UATS contract spec** (run against the live API), Tier 3 (live e2e: run the actual CLI, observe real events).
6. **Documentation** — feature doc, CHANGELOG, CLAUDE.md, post.md.

> **UxTS framework mapping** (per the directive — use UxTS where it applies): **UATS** ✅ applies to the federation HTTP API (added here, backfilling -001's gap). **UVTS** (retrieval quality) / **UBENCH/ULTS** (LLM benchmark) — N/A (no retrieval-quality or LLM surface in a federation CLI). **UOTS** (Grafana dashboard contracts) — the EVENTGRAPH-001 "Reinforcement Event Rate" panel also has **no UOTS spec**; that's a real gap but it's -001's panel, not this sprint's surface → noted as a follow-up, out of scope here. **CLI rendering** has no UxTS framework → Tier 1 Go units are appropriate.

### Out of scope
- **EVENTGRAPH-002/-003** themselves (guidance-outcome federation, more writers) — this is their enabler, not them.
- **UOTS spec for the EVENTGRAPH-001 Grafana panel** — a genuine coverage gap (the "Reinforcement Event Rate" panel has no UOTS spec), but it's -001's surface, not the CLI's → tracked as a follow-up.
- **A generic `eventgraph` query for arbitrary event classes** — v1 consumes only the existing reinforcement-neighborhood endpoint. The parent `eventgraph` cmd leaves room for `guidance-outcome-neighborhood` etc. later.
- **Grafana visualization** — not feasible (HTTP API, not a Grafana datasource); the CLI is the consumer.
- **Mutating operations** — read-only.
- **Pagination/streaming** — the API already caps via `limit`; the CLI surfaces `truncated`.

### Constraints
- Mirror the established CLI conventions (`resolveEndpoint()`, `resolveSpaceID(cmd)`, cobra `RunE`, `--json` flag) — no new HTTP/CLI scaffolding.
- No-hardcoding: endpoint via `resolveEndpoint()`; defaults deferred to the server (omit the field so the server applies its config default) rather than re-hardcoding hops/since/limit in the CLI.
- Tier 3 live testing required — the deliverable's whole point is being a live harness, so it must be demonstrated live.
- Graceful errors: clear messages on unreachable server, 400 (missing seed), 503 (eventgraph disabled / TSDB down), empty neighborhood.

## 4. Dependencies
- **EVENTGRAPH-001** (merged): `/v1/eventgraph/reinforcement-neighborhood`, `FederationResult`/`EventWithContext` shapes, `ReinforcementNeighborhoodRequest`.
- **CLI scaffolding**: `internal/cli/root.go` (`resolveSpaceID`, `AddCommand`), `internal/cli/synergy.go` (`resolveEndpoint`, HTTP-call + render pattern).
- **`/v1/memory/retrieve`** (for `--query` seed resolution).
- **Live stack** (server + Neo4j + TSDB with reinforcement_events data) for Tier 3.

## 5. Implementation Plan

### Epic 0 — Sprint plan (~0.1 day)
Commit this plan. No code.

### Epic 1 — `mdemg eventgraph` command (~0.5 day)
- New `internal/cli/eventgraph.go`: `newEventgraphCmd()` (parent) + `newEventgraphReinforcementNeighborhoodCmd()` (subcommand), registered in `root.go`.
- Flag handling + request construction. **Defaults deferral:** only set `hops`/`since_seconds`/`limit` in the request body when the operator passes the flag; otherwise omit so the server applies its configured default (no re-hardcoding).
- `--query` path: when `--seed` empty + `--query` set, POST `/v1/memory/retrieve` (top_k=1), take `results[0].node_id` as seed; error clearly if no results.
- POST the federation request; decode `FederationResult`; render:
  - Summary line(s): `neighborhood: N nodes · hops: H · events: E · scanned: S · truncated: T`.
  - Table (non-`--json`): short src→dst, Δweight (signed), new_weight, direction, new-edge (✓), in-nbhd (src/dst), recorded_at (local).
  - `--json`: pretty-print the raw `FederationResult`.
- Error UX: unreachable endpoint, 400/503 with the server's message, empty events (friendly "no reinforcement events in this neighborhood/window").
- Tier 1 unit tests: flag→request mapping (defaults omitted vs set), `--query` seed resolution wiring (mock HTTP), render of a sample `FederationResult` (table + JSON), empty-result message.
**Gate:** `go build`/lint clean; Tier 1 green; `mdemg eventgraph --help` + `... reinforcement-neighborhood --help` render.

### Epic 2 — UATS contract spec for the federation API (~0.3 day)
- Author `docs/api/api-spec/uats/specs/eventgraph_reinforcement_neighborhood.uats.json` (mirrors the existing spec shape: `api`/`metadata`/`config`/`request`/`expected`/`variants`):
  - **Happy path:** POST with a valid `{space_id, seed_node_id, hops, since_seconds, limit}` → `200`, body_assertions on the contract (`$.events` type array, `$.neighbor_node_ids` array, `$.graph_hops` number, `$.truncated` bool). Use a seed known to exist (or assert shape only, tolerant of empty events).
  - **Variants (error contract):** missing `space_id` → `400`; missing `seed_node_id` → `400`; negative `hops` → `400`; `hops` over the `2× default` ceiling → `400`; `GET` (method not allowed) → `405`. Each asserts `$.error type_is string`.
  - Tag appropriately (e.g. `eventgraph`); if it needs the eventgraph service up, ensure it isn't excluded by the default `make test-api` exclude-tags (or document the tag).
- Run `uats_runner.py add-hashes` to compute + insert `config.sha256`; `verify-hashes` clean.
- Validate live: `python3 docs/api/api-spec/uats/runners/uats_runner.py validate --spec docs/api/api-spec/uats/specs/eventgraph_reinforcement_neighborhood.uats.json --base-url http://localhost:9999` passes; `make test-api` includes it.
**Gate:** UATS spec validates green against the live server; hashes verify.

### Epic 3 — Live e2e + docs + close (~0.3 day)
- **Tier 3 live e2e (the harness demonstration):**
  1. Generate fresh reinforcement events (a code-node retrieve), pick a seed from them.
  2. Run `mdemg eventgraph reinforcement-neighborhood --seed <node> --hops 2 --since 1h` → observe the rendered neighborhood + events matching the federation API's JSON.
  3. Run the `--query` form (`--query "circuit breaker state machine"`) → confirm it resolves a seed + renders events — the natural live-testing flow.
  4. Confirm `--json` output parses + matches the API.
  - Transcript → `docs/development/eventgraph-cli-001/verification.md`.
- **Docs:** `docs/features/event-graph-federation.md` (extend with the CLI consumer section), CHANGELOG, CLAUDE.md (note the federation now has a consumer + the CLI is the EVENTGRAPH live-test harness), `post.md`.
**Gate:** command renders real federation output live (both `--seed` and `--query`); docs done.

## 6. Testing Plan (3 tiers, UxTS where it applies)
- **Tier 1 — Unit (Go):** flag→request mapping (omit-when-unset defaults), `--query` seed resolution (mocked retrieve), render (table + `--json` + empty), error messages. (CLI rendering has no UxTS framework → Go units.)
- **Tier 2 — Contract (UATS):** `eventgraph_reinforcement_neighborhood.uats.json` validates the federation API contract against the live server (happy path 200 + body shape; 400/405 error variants). Run via `make test-api` / `uats_runner.py validate`. This is the project's API-contract framework — used instead of a bespoke Go integration test, and it backfills the EVENTGRAPH-001 UATS gap.
- **Tier 3 — Live e2e:** the real `mdemg eventgraph` CLI runs against the live stack with fresh reinforcement events; `--seed` and `--query` forms both render; `--json` matches the API. Transcript in `verification.md`.

## 7. Commit Strategy
Sequential commits on `reh3376_dev01`; auto-PR. Epic 1 = command + register + Tier 1. Epic 2 = integration test + verification.md + docs. Sprint summary on PR after Epic 2.

## 8. Verification Checklist
- [ ] `mdemg eventgraph reinforcement-neighborhood` registered; `--help` renders for parent + subcommand.
- [ ] Flags map to the request; unset hops/since/limit are omitted (server default applies) — no re-hardcoded defaults.
- [ ] `--query` resolves a seed via `/v1/memory/retrieve`; clear error when no results.
- [ ] Human table + `--json` render correctly; `truncated`/empty handled.
- [ ] Error UX: unreachable / 400 / 503 / empty give clear messages.
- [ ] `go build ./...` + `golangci-lint` clean; Tier 1 green.
- [ ] Tier 2 integration (skip-on-empty) green.
- [ ] **Tier 3 live:** `--seed` form renders real neighborhood events; `--query` form resolves + renders; `--json` matches the API.
- [ ] Feature doc, CHANGELOG, CLAUDE.md, post.md, verification.md updated.
- [ ] Sprint summary on PR.

## 9. Documentation Update — Epic 2 above.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Empty neighborhood confuses operators (looks broken) | Medium | Low | Friendly "no events in this neighborhood/window" + show neighborhood size so it's clear the walk worked but no events matched. The `--query` examples in docs target nodes known to have events. |
| `--query` resolves an unhelpful seed (top result not in an event-rich neighborhood) | Medium | Low | Document that `--query` is a convenience; print the resolved seed node_id so the operator sees what was used; `--seed` for precision. |
| CLI re-hardcodes server defaults (hops/since) → drift | Low | Medium | Omit unset fields from the request so the server's config defaults apply; only send what the operator explicitly set. |
| Federation API shape changes later (-002) break the CLI | Low | Low | The CLI decodes the documented `FederationResult`; -002 adds a *new* endpoint, not a breaking change to this one. |

## 11. Documents Accessed
- `internal/cli/synergy.go` — cobra subcommand + `resolveEndpoint()` + HTTP-call + render pattern (the template)
- `internal/cli/root.go` — `resolveSpaceID()`, `AddCommand` registration
- `internal/api/eventgraph_handler.go` — `ReinforcementNeighborhoodRequest` (the request contract)
- `internal/eventgraph/query.go` — `FederationRequest`, `EventWithContext`, `FederationResult` (the response contract)
- `internal/models/models.go` — `RetrieveRequest`/`RetrieveResult` (for `--query` seed resolution)
- `docs/api/api-spec/uats/` — `specs/admin_config_patch.uats.json` (spec shape exemplar: request/expected/variants/body_assertions), `HASH_VERIFICATION.md` (sha256 normalization), `runners/uats_runner.py` (`validate` / `add-hashes` / `verify-hashes`), `Makefile` `test-api` target
- Live: reinforcement_events healthy (+20 from a code-node retrieve); federation API + its Grafana panel both have **no UATS/UOTS coverage** (gaps; the API one is backfilled here)

## 12. Rollback Procedures
- Pure additive CLI surface; rollback = revert the Epic 1 commit (remove `eventgraph.go` + the `root.go` registration). No schema/data/config changes, no impact on the running server or other commands.

---

## Files to be created/modified (anticipated)
**New:**
- `docs/development/eventgraph-cli-001/sprint_plan_eventgraph_cli_001.md` (Epic 0)
- `docs/development/eventgraph-cli-001/verification.md` (Epic 3)
- `docs/development/eventgraph-cli-001/post.md` (Epic 3)
- `internal/cli/eventgraph.go` (Epic 1)
- `internal/cli/eventgraph_test.go` (Tier 1)
- `docs/api/api-spec/uats/specs/eventgraph_reinforcement_neighborhood.uats.json` (Epic 2 — UATS contract, backfills the -001 gap)

**Modified:**
- `internal/cli/root.go` — register `eventgraphCmd`
- `docs/features/event-graph-federation.md` — CLI consumer section
- `CHANGELOG.md`, `CLAUDE.md` — Epic 2

## Acceptance Criteria
1. `mdemg eventgraph reinforcement-neighborhood --seed <node>` renders a real reinforcement-neighborhood (summary + events table) from the live federation API.
2. `--query <text>` resolves a seed via retrieval and renders events — the natural live-testing flow for the EVENTGRAPH line.
3. `--json` emits the raw `FederationResult` (scriptable / matches the API).
4. Defaults are server-driven (unset flags omitted); no re-hardcoded hops/since/limit in the CLI.
5. Errors (unreachable / missing seed / disabled / empty) give clear, actionable messages.
6. A **UATS contract spec** for `/v1/eventgraph/reinforcement-neighborhood` exists, hashes verify, and validates green via `make test-api` (backfills the EVENTGRAPH-001 UATS gap).
7. The federation API now has a working consumer + the EVENTGRAPH line has a live-testing harness for -002/-003.
