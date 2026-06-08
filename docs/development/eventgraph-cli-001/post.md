# EVENTGRAPH-CLI-001 — Sprint Close

**Date:** 2026-06-08 · **Branch:** `reh3376_dev01` · **Target:** v0.10.x (additive)

## What shipped

`mdemg eventgraph reinforcement-neighborhood` — the **first consumer** of the EVENTGRAPH-001 federation API (`POST /v1/eventgraph/reinforcement-neighborhood`), plus the **live-testing harness** for the EVENTGRAPH line. EVENTGRAPH-001 built the endpoint; nothing consumed it. This sprint makes it operator-usable and gives 002/003 a way to be live-tested as they're built.

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan (12-section) | (prior) |
| 1 | `internal/cli/eventgraph.go` + Tier 1 (8 tests, `-race`) + registration | `f67bfe4` |
| — | Live-caught contract fix: `neighbor_node_ids` `null`→`[]` | `9bf981b` |
| 2 | UATS contract spec (6 cases, 6/6 live) + sha256 | `3b6567d` |
| 3 | Tier 3 live e2e + docs (this) | (this commit) |

## How it works

- **Seed resolution:** `--seed n_…` explicit, or `--query "<text>"` → top `/v1/memory/retrieve` result becomes the seed.
- **Federation:** POSTs the endpoint; renders a summary + events table (`new` = `created_new_edge`, `nbhd` = which endpoints fell inside the N-hop walk) or `--json`.
- **Single source of truth:** `--hops`/`--since`/`--limit` are omitted from the request when unset, so the server applies its own config defaults — no second copy of the defaults lives in the CLI.

## UxTS mapping (per the standing directive)

- **UATS** ✅ — `eventgraph_reinforcement_neighborhood.uats.json`. This also **backfills the contract-test gap EVENTGRAPH-001 left** (that sprint shipped the endpoint without a UATS spec).
- **UVTS / UBENCH** — N/A (no retrieval-quality or LLM-output surface).
- **UOTS** — the EVENTGRAPH-001 Grafana panel ("Reinforcement Event Rate") still lacks a UOTS dashboard contract; tracked as a follow-up (carried over from EVENTGRAPH-001, not introduced here).

## Live testing found what code testing didn't

Honoring the standing directive (*standard code testing is not sufficient to find problems in the live running framework*), the UATS happy-path was run against the **real running server**, not a mock — and immediately caught that `neighbor_node_ids` serialized as `null` (not `[]`) for an unknown seed, because `walkNeighborhood` returns a nil slice. Go unit tests never exercised the marshal of an empty result, so they were silent. Fixed at the source in its own commit (per the live-smoke precedent that surprise bugs get their own fix-commit), pinned by `TestFederationResult_EmptyArraysNotNull`.

The Tier 3 run also doubled as a **live demonstration of the whole loop**: `--query "circuit breaker state machine"` surfaced 20 reinforcement events timestamped at the moment of the query — the retrieval itself fired `ApplyCoactivation` over the 5-node cluster (10 pairs × 2 passes: create then strengthen), and the federation read those very events back. Hebbian-write and federation-read are both healthy and mutually consistent.

## Follow-ups

- **EVENTGRAPH-002** — federate a second event class (guidance outcomes from `GUIDANCE_OUTCOME` edges). The CLI gains a sibling subcommand; each new endpoint gets its own UATS spec.
- **EVENTGRAPH-003** — wire the reinforcement writer into the other three Hebbian entry points (`ApplySymbolCoactivation`, `CoactivateSession`, `ApplyNegativeFeedback`).
- **UOTS** for the EVENTGRAPH-001 Grafana panel (carried over).

## Documents Accessed

- `docs/development/eventgraph-cli-001/sprint_plan_eventgraph_cli_001.md`
- `internal/cli/eventgraph.go`, `internal/cli/synergy.go` (CLI template), `internal/cli/root.go`
- `internal/api/eventgraph_handler.go`, `internal/eventgraph/query.go` (response contract)
- `docs/api/api-spec/uats/specs/admin_spaces_update.uats.json` (UATS variant template), `docs/api/api-spec/uats/runners/uats_runner.py`
- `docs/features/event-graph-federation.md`, `CHANGELOG.md`, `CLAUDE.md`
