# EVENTGRAPH-003 — Sprint Close

**Date:** 2026-06-09 · **Branch:** `reh3376_dev01` · **Target:** v0.10.x (additive — no schema/endpoint change)

## What shipped

All four Hebbian write paths now feed `reinforcement_events`, distinguished by `trigger_path`. EVENTGRAPH-001 shipped one (`ApplyCoactivation`); this sprint added the other three.

| Epic | Path → `trigger_path` | Commit |
|---|---|---|
| 0 | Sprint plan | `a8a324d` |
| 1 | `CoactivateSession` → `coactivate_session` | `0760927` |
| 2 | `ApplySymbolCoactivation` → `apply_symbol_coactivation` | `c35fb0d` |
| 3 | `ApplyNegativeFeedback` weaken → `apply_negative_feedback` | `e07099f` |
| — | **fix:** revive the never-invoked `CoactivateSession` | `b3e61cb` |
| 4 | Tier-3 live + docs | (this) |

## How it works

Each path got a Cypher `RETURN`-extension emitting the standard per-pair telemetry + a parse-and-record hook (reusing `reinforcement_parser.go` + the already-injected writer) tagged with its `trigger_path`. **No new migration, writer, endpoint, or config** — the V0022 schema already carried `trigger_path` + signed `delta_weight` + `created_new_edge`, and the federation API + CLI surface the new events for free.

- `CoactivateSession` (easy) — full Hebbian fields; one row per forward edge.
- `ApplySymbolCoactivation` (medium) — split the weight update out of the `ON MATCH` clause to capture the pre-update weight; symbol-N/A fields NULL.
- `ApplyNegativeFeedback` (weaken-only) — per-pair `RETURN` (was aggregated) → negative `delta_weight`, `created_new_edge=false`; FOREACH writes untouched. The **contradict** path is deferred (it creates `CONTRADICTS` edges, which the federation walk doesn't traverse — better as its own event class later).

## Live testing earned its keep — twice over

1. **Cypher validation without side effects:** every edit was `EXPLAIN`-validated against live Neo4j (compiles, all `RETURN` vars in scope, no writes), and the weight `SET` clauses were left untouched or refactored behavior-preservingly.
2. **A dead path discovered + fixed.** `coactivate_session` produced 0 rows at first. Tracing the *whole* pipeline (not stopping at "my code looks right") revealed **0 conversation-observation co-activation edges had EVER been created** in mdemg-dev — because `CoactivateSession` was **never invoked**: the conversation service's `learningService` was never injected (`SetLearningService` had no caller), so the `Observe()` nil-guard always skipped it. The function + Cypher were correct (proven by running the Cypher directly). One-line fix (`SetLearningService(lea)`) revived session co-activation learning entirely. This is the same latent-dormancy class as EVENTGRAPH-001's RRF-`Activation`-drop — invisible to unit tests, obvious under live smoke.

Final live state — all four `trigger_path`s emitting + surfaced by the federation read:
```
apply_coactivation 50 · apply_symbol_coactivation 1000 · apply_negative_feedback 1 · coactivate_session 4
mdemg eventgraph reinforcement-neighborhood --seed <conv-obs> → events: 4 { coactivate_session: 4 }
```

## UxTS mapping

- **UATS / UVTS / UBENCH** — N/A (no new endpoint, retrieval, or LLM surface; the federation contract is unchanged and already has its UATS spec). Validation was EXPLAIN + live Tier-3 row observation.
- **UOTS** — the EVENTGRAPH Grafana panel's UOTS contract remains the carried-over follow-up.

## Follow-ups

- **`CONTRADICTS` event class** — federate negative-feedback contradictions as their own class (the EVENTGRAPH-002 guidance-outcome pattern), since they aren't `CO_ACTIVATED_WITH` reinforcements.
- **Investigate the revived `CoactivateSession` at scale** — now that it actually runs, watch session co-activation volume + its effect on graph health (it was dead for the project's entire history).
- **UOTS** for the EVENTGRAPH Grafana panels (carried over).

## Documents Accessed

- `internal/learning/service.go` (`ApplyCoactivation` template, `CoactivateSession`, `ApplySymbolCoactivation`, `ApplyNegativeFeedback`); `internal/learning/reinforcement_parser.go`; `internal/tsdb/reinforcement_writer.go`; `internal/tsdb/migrations/022_reinforcement_events.sql`
- `internal/conversation/service.go` (`Observe`, `SetLearningService`, the `learningService` nil-guard); `internal/api/server.go` (conversation-service construction + the injection fix)
- `docs/features/event-graph-federation.md`; EVENTGRAPH-001/CLI-001/002 sprint lines
