# EVENTGRAPH-002 — Sprint Close

**Date:** 2026-06-08 · **Branch:** `reh3376_dev01` · **Target:** v0.10.x (additive + one additive index migration)

## What shipped

The **second federated event class** (Pattern Y1): guidance outcomes. `POST /v1/eventgraph/guidance-outcome-neighborhood` + `mdemg eventgraph guidance-outcome-neighborhood` walk a constraint's Neo4j neighborhood and surface the time-windowed `constraint_outcomes` (followed/ignored/contradicted) for the constraint **and its graph-related constraints** — a question per-constraint aggregation (`GetConstraintEffectiveness`) can't answer.

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan | `7d33900` |
| 1 | V0023 `constraint_code` index + schema 22→23 | `a97dee6` |
| 2 | `GuidanceOutcomesInNeighborhood` federation method + Tier 1/2 | `48fbd80` |
| 3 | Handler + route + shared-helper refactor | `d8639a5` |
| 4 | CLI subcommand + Tier 1 | `0abf082` |
| 5 | UATS contract spec (6/6 live) | `8092e0d` |
| 6 | Tier 3 live verification | `d68fd53` |
| 7 | Docs + close | (this) |

## Two data-decided architecture calls (disclosed, not asked)

1. **Reuse `constraint_outcomes` — no new sink.** The guidance-outcome stream already lived in TSDB (migration 011, written by `/v1/jiminy/feedback`). A parallel `guidance_outcome_events` table would duplicate populated data and add a redundant writer/enqueue site. So EVENTGRAPH-002 is a **read-side federation** — far smaller than EVENTGRAPH-001 (no writer, no enqueue hook, one index).
2. **Join on `constraint_code`, not `node_id`.** Verified pre-plan against live data: TSDB `constraint_id` is a UUID that does not match the Neo4j `node_id` (CUID). `constraint_code` — carried by both sides — is the only viable key. One additive index (V0023) backs it.

## Single-source refactor

Per the standing dynamic-variables directive, the gate (method/enabled/service) + default-resolution (hops/since/limit/ceiling) shared by both federation endpoints were extracted into `eventgraphGate` + `resolveFederationDefaults`. The existing reinforcement handler now routes through them too — verified no regression (reinforcement UATS 6/6 live). One place to change the federation rules.

## Live testing earned its keep again

- **Cross-checked against ground truth:** the CLI's `--json` outcome count was asserted equal to a direct `constraint_outcomes` SQL query — **11 = 11, all followed**. The federation returns exactly what the DB holds.
- **Traced an apparent anomaly instead of assuming:** `--query "never commit directly to main"` surfaced 5 constraint codes but **0 outcomes**. Rather than call it a bug (or ignore it), I SQL-checked those 5 codes — they genuinely have no feedback in the window. So "0 outcomes" is correct: the federation distinguishes "code present in neighborhood" from "code has outcomes." Exactly the discipline the directive exists to enforce.

## UxTS mapping

- **UATS** ✅ — `guidance_outcome_neighborhood.uats.json` (6/6 live, tagged `tsdb`).
- **UVTS / UBENCH** — N/A (no retrieval-quality or LLM-output surface).
- **UOTS** — no Grafana panel added this sprint; the EVENTGRAPH-001 panel's UOTS contract remains the carried-over follow-up.

## Follow-ups

- **`--constraint-code` seeding** — resolve a constraint node from its code server-side (the natural entry for effectiveness queries; v1 uses `--seed`/`--query`).
- **EVENTGRAPH-003** — wire the reinforcement writer into the other three Hebbian entry points (`ApplySymbolCoactivation`, `CoactivateSession`, `ApplyNegativeFeedback`).
- **Uncoded outcomes** — outcomes recorded without a `constraint_code` aren't joinable and won't appear; if that fraction matters, a backfill or a `constraint_id`-based fallback join could be considered.
- **UOTS** for the EVENTGRAPH Grafana panels (carried over).

## Documents Accessed

- `docs/development/eventgraph-002/sprint_plan_eventgraph_002.md`
- `internal/tsdb/migrations/011_constraint_outcomes.sql`, `022_reinforcement_events.sql`, `023_constraint_outcomes_code_index.sql`
- `internal/eventgraph/query.go`, `guidance_outcomes.go`; `internal/api/eventgraph_handler.go`, `eventgraph_guidance_handler.go`, `server.go`; `internal/cli/eventgraph.go`
- `internal/jiminy/persistence.go`, `service.go` (outcome recording); `internal/tsdb/constraint_outcomes_writer.go`
- `internal/config/config.go` (TSDB_REQUIRED_SCHEMA_VERSION); `.github/workflows/ci.yml` (schema-version check)
- `docs/api/api-spec/uats/specs/eventgraph_reinforcement_neighborhood.uats.json` (template)
- Live: `constraint_outcomes` rows + Neo4j `role_type='constraint'` nodes
