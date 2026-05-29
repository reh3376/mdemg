# Sprint EVENTGRAPH-001 — Post

**Closed:** 2026-05-27
**Branch:** `reh3376_dev01`
**Plan:** [`sprint_plan_eventgraph_001.md`](sprint_plan_eventgraph_001.md)
**Verification:** [`verification.md`](verification.md)

## Outcome

Pattern Y1 shipped. The graph captures state; TSDB captures history. Reinforcement events from `ApplyCoactivation` flow into V0022 `reinforcement_events` via a buffered CopyFrom writer; the federation API at `POST /v1/eventgraph/reinforcement-neighborhood` orchestrates a Cypher graph walk + TSDB query + Go-side join with `src/dst_in_neighborhood` annotation. Three Prometheus counters surface writer health; one Grafana panel plots event rates.

## Epic-by-epic

| Epic | Status | Commit | Notes |
|---|---|---|---|
| 0 — Sprint plan | ✅ | `2122333` | 12-section v1.0 format; 8 sequential epics; ~1.5–2 dev-days estimated, came in roughly on target. |
| 1 — V0022 migration + config bump | ✅ | `992fa32` | `TSDB_REQUIRED_SCHEMA_VERSION` 21→22; 4 indexes (space+time, src+time, dst+time, partial session). Live-verified hypertable + indexes via psql; idempotent re-apply confirmed. |
| 2 — Buffered writer | ✅ | `a2d1e06` | 9 Tier 1 unit tests: enqueue/flush, empty-flush no-op, FIFO eviction on full, unlimited buffer, nullable serialization, flush-error counter, Close drain, idempotent Close, ticker fires. |
| 3 — Cypher RETURN-shape change | ✅ | `55a5ecc` | RETURN extended from `count(*)` to 17 per-pair columns. New `parseReinforcementRow` helper. **Plan deviation** (disclosed): 1 row per logical pair regardless of asymmetric/symmetric mode — `rr.weight = r.weight` always, so 2-row emission would double-count without adding signal. 6 Tier 1 unit tests. |
| 4 — Wire writer into ApplyCoactivation | ✅ | `b93df79` | `learning.Service.SetReinforcementWriter` setter (mirrors `SetStabilityReinforcer` precedent). 7 new env vars wired through config. api/server.go owns the writer's lifecycle (construct after TSDB ready, Close on shutdown). 2 Tier 2 integration tests against real TSDB. |
| 5 — Federation helper + API endpoint | ✅ | `2c70548` | `internal/eventgraph/Service.EventsInGraphNeighborhood`, `POST /v1/eventgraph/reinforcement-neighborhood`. **Plan decision disclosed**: single endpoint + `event_type` (implicit) over endpoint-per-class — narrow URL today, easy to evolve when EVENTGRAPH-002 arrives. 7 Tier 1 + 1 Tier 2 integration test. |
| 6 — Prometheus + Grafana | ✅ | `429a197` + `75c6ea7` (fix) | 3 counters via `PrometheusCounter` interface (avoids tsdb↔metrics import cycle). Panel on `mdemg-graph-topology` shows enqueued/dropped/flush-failures over time. **Fix-commit** restored full GRAFANA-AUDIT-001 `audit_results.json` after the targeted audit run on one dashboard overwrote the multi-dashboard baseline. |
| 7 — Tier 3 live e2e | ✅ | `f307f55` (fix) + `5a346c6` (verification) | Live retrieves → 10 events in TSDB within 35s flush window → federation API at hops=1 returned 5-node neighborhood + 10 in-neighborhood events. **Surprise-bug fix-commit** for RRF-path Activation drop — see "Surprises" below. |
| 8 — Documentation + sprint close | ✅ | `<this commit>` | Feature doc, CHANGELOG, CLAUDE.md, post.md. |

## Acceptance criteria (from plan §"Acceptance Criteria")

1. ✅ Real `/v1/memory/retrieve` against `mdemg-dev` → 10 rows in `reinforcement_events` within ≤35s.
2. ✅ Federation API with seed from the retrieve returned events with src/dst in the hops=1 neighborhood.
3. ✅ Grafana panel shows non-zero events/min during retrieval load.
4. ✅ `EVENTGRAPH_ENABLED=false` short-circuits the writer (validated via Tier 1 + boot-time path inspection).
5. ✅ `mdemg tsdb status` reports schema_version=22; idempotent re-migrate.
6. ✅ `docs/features/event-graph-federation.md` shipped with Why / Choices / How it works / How to use.
7. ✅ No regression in existing `internal/learning/service_test.go` or integration suites.

## Surprises / fix-commits

Per CLAUDE.md "Testing — Live System Testing Is Required" — *"surprise bugs caught during live smoke get their own fix-commit, do not silently roll them into the sprint commit."*

### `scoring_rrf.go` dropped the `Activation` field on `RetrieveResult`

Discovered during Epic 7. First Tier 3 attempt produced 0 rows in TSDB despite 3 successful retrieves. Investigation revealed that since Phase 13.1 default-on (2026-05-03), the RRF retrieval path silently zeroed out `Activation` — the legacy `ScoreAndRank` path sets it (`scoring.go:883`), the RRF path's conversion in `scoring_rrf.go:121-129` omitted it.

Net effect: `learning.Service.ApplyCoactivation` has been a silent no-op on the retrieve hot path for ~24 days. CO_ACTIVATED_WITH edges still existed because the other Hebbian entry points (`CoactivateSession`, `ApplySymbolCoactivation`, consolidation walks) wrote them, but the retrieve-time goroutine — the most frequent and most operator-visible Hebbian event class — was silently inactive.

**One-line fix.** `f307f55`: `Activation: act[c.NodeID]` on the RRF-path `RetrieveResult` literal. Brings RRF to parity with the legacy scorer. Forward-only; graph self-heals as new retrieves correctly emit Hebbian updates.

This was a **load-bearing find**: had EVENTGRAPH-001 not required Tier 3 live e2e, the bug would likely have stayed latent until the next deep-dive audit. The sprint's value extends beyond the new feature — it surfaced 24 days of silent regression.

### Audit-JSON overwrite during Epic 6

`scripts/grafana_panel_audit.py --dashboard mdemg-graph-topology.json` overwrote the full GRAFANA-AUDIT-001 multi-dashboard `audit_results.json` with the single-dashboard subset. Caught immediately via `git status` showing 2525 deletions on a docs file. Restored from the prior commit in fix-commit `75c6ea7`. Lesson: the audit harness should append/merge rather than overwrite when invoked with `--dashboard`. EVENTGRAPH-002 (or a GRAFANA-AUDIT-002 mini-sprint) candidate.

### Orphaned mdemg process holding port 9999

The original mdemg server (PID 38840, started May 7) was orphaned from launchd's view but still listening on 9999. `launchctl kickstart -k` started a *new* instance on port 10000 (the next free port) without touching the orphan. First Tier 3 retrieves went to the orphan, which didn't have the writer. Resolved by `launchctl bootout` + SIGTERM-on-orphan + `launchctl bootstrap` (operator-confirmed).

## Plan deviations disclosed

Per `feedback_plan_options_pattern.md`:

1. **One row per pair regardless of asymmetric/symmetric mode** (plan called for 2 rows in asymmetric mode). Rationale: `rr.weight = r.weight` in the existing Cypher means forward and reverse weights are identical; 2 rows would double-count without adding signal. Direction column distinguishes the mode. Disclosed in commit `55a5ecc` and the verification doc. Revisit if EVENTGRAPH-003 introduces a Hebbian path where forward/reverse weights diverge.
2. **Single endpoint + event_type implicit** (plan proposed Option A vs Option B = endpoint per class). Picked Option A. URL is explicit about the event class today (`/reinforcement-neighborhood`); EVENTGRAPH-002 can add a query param or split the URL when a second event class arrives — no breaking change either way.

## Forward-looking

- **EVENTGRAPH-002** — extend the federation API to a second event class. Strong candidate: `GUIDANCE_OUTCOME` edges from the Jiminy constraint-lifecycle path. Same hypertable shape with a new `trigger_path` value, OR a separate hypertable if the schema diverges.
- **EVENTGRAPH-003** — wire the writer into the other three Hebbian entry points (`ApplySymbolCoactivation`, `CoactivateSession`, `ApplyNegativeFeedback`).
- **Pattern Y2 escalation** — promote one event class to skinny graph link-nodes in Neo4j when a query proves single-pass Cypher across events is necessary. Trigger only on real query pressure; don't pre-build.
- **GRAFANA-AUDIT-002 (or follow-up patch)** — fix `scripts/grafana_panel_audit.py --dashboard` to merge rather than overwrite `audit_results.json`.
- **Retention policy on `reinforcement_events`** — default TimescaleDB behavior (chunks forever) is fine for v1; revisit when chunk count exceeds 26 weeks.

## Documents Accessed

- `CLAUDE.md` — testing rules, sprint plan v1.0 format, per-feature docs rule, no-hardcoding rule, no-tight-LLM-budget-caps rule, sequential-epics rule
- `MEMORY.md` — mandatory workflow rules cross-checked
- `internal/learning/service.go` (`ApplyCoactivation` at line 279–485 — the Hebbian write path)
- `internal/tsdb/model_install_writer.go` + `migrations/021_model_install_events.sql` (V0021 sync-INSERT precedent, considered then rejected)
- `internal/tsdb/sparse_gate_writer.go` (V0019 buffered-CopyFrom precedent — reused pattern)
- `internal/tsdb/llm_writer.go` (FIFO eviction precedent)
- `internal/config/config.go` (config-knob wiring sites)
- `internal/retrieval/scoring.go` + `scoring_rrf.go` (Activation-field gap discovery)
- `internal/retrieval/activation.go` (SpreadingActivation seeding)
- `internal/api/handlers_breakers.go` (handler shape precedent for the federation endpoint)
- `internal/api/server.go` (writer + handler lifecycle wiring sites)
- `internal/metrics/collectors.go` (StandardMetrics registration site)
- `deploy/docker/grafana/dashboards/mdemg-graph-topology.json` (panel-add site)
- `docs/development/model-dist-002/post.md` (12-section close exemplar)
- `tests/integration/helpers_test.go` + `tsdb_test.go` (test fixture patterns)
- TypeDB vs Neo4j research summary from the current conversation (informed Pattern Y1 design + the choice not to migrate)
