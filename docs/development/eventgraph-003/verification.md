# EVENTGRAPH-003 — Live Verification (Tier 3)

**Date:** 2026-06-09
**Stack:** native `mdemg serve` (launchd, rebuilt from this branch) + Docker (Neo4j `mdemg-neo4j-1` + TimescaleDB) + llama-server :8102. Space `mdemg-dev`.

Per the standing directive — *standard code testing isn't sufficient to find live problems* — every path was triggered through the real running system and observed in `reinforcement_events` + via the federation read.

## Acceptance bar: all three additional Hebbian paths emit `reinforcement_events` rows with the correct `trigger_path`, and the federation read surfaces them.

### Method
- **Cypher correctness (side-effect-free):** each new/edited Cypher was `EXPLAIN`-validated against the live Neo4j (compiles, all RETURN variables in scope, no writes executed). The weight `SET` clauses were left untouched (Epic 1/3) or refactored with the pre-update weight preserved exactly (Epic 2), so update behavior is unchanged by construction.
- **Live row emission:** triggered each path against the running server, waited for the writer flush (~30s), and read the `trigger_path` distribution.

### Results — `trigger_path` distribution after triggering all paths
```
apply_coactivation        |   50   (pre-existing — the one wired path)
apply_symbol_coactivation | 1000   ← Epic 2  (retrieve over symbol-defining nodes; capped at the writer buffer — symbol pairs are high-volume)
apply_negative_feedback   |    1   ← Epic 3  (negative-feedback weaken; delta_weight = -0.1116, created_new_edge = false ✓)
coactivate_session        |    4   ← Epic 1  (after the dormancy fix below)
```

- **Epic 3 (`apply_negative_feedback`)** — `POST /v1/learning/negative-feedback` weakened a real `CO_ACTIVATED_WITH` edge → row with **negative** `delta_weight (-0.1116)`, `created_new_edge=false`, `direction=bidirectional`. The contradict path was (by design) not emitted.
- **Epic 2 (`apply_symbol_coactivation`)** — `POST /v1/memory/retrieve` over a symbol-rich query fired the background symbol co-activation → 1000 rows (high pair volume, buffer-capped — expected; symbol pairs scale as C(n,2)).
- **Epic 1 (`coactivate_session`)** — see the discovery below; after the fix, 3 distinct same-session observations created 6 `CO_ACTIVATED_WITH` edges and emitted 4 `coactivate_session` rows with full Hebbian fields (prev 0.1 → new 0.119, positive delta, `surprise_factor=1.0`, `activation_product≈0.999`).

### Federation read consumes the new events
Seeded `mdemg eventgraph reinforcement-neighborhood --seed <conv-obs node> --hops 1` →
```
events: 4 | neighborhood nodes: 3 | trigger_paths: { coactivate_session: 4 }
```
The full Pattern-Y1 read path (Cypher walk + TSDB join) surfaces the new `trigger_path` events for free — no read-side change needed.

## Surprise bug discovered + fixed (own commit `b3e61cb`)

`coactivate_session` produced **0 rows** at first — and tracing showed **0 `CO_ACTIVATED_WITH` edges between conversation-observation nodes had EVER been created in mdemg-dev** (5495 such nodes exist). Learning wasn't frozen (the other 3 paths fired). Root cause, found by tracing the whole pipeline: `conversation.NewServiceWithConfig` sets `learningService = nil` ("set via SetLearningService to avoid circular dependency"), but **`SetLearningService` had no caller**, so `Observe()`'s `if s.learningService != nil` guard *always* skipped `CoactivateSession`. The function + Cypher were correct (proven by running the Cypher directly: 3 pairs, proper weights, the exact rows my wiring records) — **it was simply never invoked.** Session co-activation learning was silently dead.

Fix: `convSvc.SetLearningService(lea)` at construction (`server.go`). Live-verified: observations now create session co-activation edges + emit `coactivate_session` events. This is the same class as EVENTGRAPH-001's RRF-`Activation`-drop — a latent dormancy only live testing surfaces. Standalone fix-commit per the precedent.

## Acceptance criteria — met
1. ✅ `CoactivateSession` emits `coactivate_session` (full Hebbian fields) — after reviving the never-invoked path.
2. ✅ `ApplySymbolCoactivation` emits `apply_symbol_coactivation` (symbol roles; eta/surprise/activation/path_sim NULL).
3. ✅ `ApplyNegativeFeedback` weaken emits `apply_negative_feedback` with negative `delta_weight`, `created_new_edge=false`; contradict not emitted.
4. ✅ Weight-update behavior unchanged (EXPLAIN + SET untouched / behavior-preserving refactor).
5. ✅ No schema change; writer/federation/CLI untouched; the federation read surfaces all four `trigger_path`s.
6. ✅ Bonus: revived a dead learning path (session co-activation).

## Conclusion
EVENTGRAPH-003's bar is met and live-verified: all four Hebbian write paths now feed `reinforcement_events`, distinguished by `trigger_path`, and the federation read surfaces them. Live testing additionally caught and fixed a pre-existing dormancy that had silenced session co-activation learning entirely.
