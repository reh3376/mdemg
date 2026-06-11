# EVENTGRAPH-004 — Live Verification (Tier 3)

**Date:** 2026-06-10
**Stack:** native `mdemg serve` (launchd, restarted on the Epic-1 binary at
20:17:29 EDT) + Docker (Neo4j `mdemg-neo4j-1` + TimescaleDB) + llama-server
:8102. Space `mdemg-dev`.

Per the standing directive — *standard code testing isn't sufficient to find
live problems* — every branch was triggered through the real running system
and observed in Neo4j, `reinforcement_events`, and the federation CLI.

## Acceptance bar

The contradict action emits `reinforcement_events` rows with
`trigger_path='apply_negative_feedback_contradict'` (create + re-match
branches correct), the weaken path is byte-for-byte unchanged in behavior,
and the federation read surfaces the new events.

## Method

1. **EXPLAIN validation (side-effect-free):** both restructured statements
   (weaken, contradict) extracted from the Go source and `EXPLAIN`-validated
   against live Neo4j before commit — compile clean, all RETURN variables in
   scope, no writes executed. PASS/PASS.
2. **Probe topology:** 4 probe observations —
   A (`w02rjv…`, session `eg004-a-1781137064`) and B (`nrjii…`, session
   `eg004-b-1781137064`) in *different* sessions → no co-activation edge →
   the contradict branch; C (`b4z9d…`) and D (`ml6iz…`) in the *same*
   session `eg004-w-1781137064` → session co-activation edge (the
   EVENTGRAPH-003 dormancy fix at work) → the weaken branch.
   Preconditions verified live: `ab_coactivated=FALSE, ab_contradicts=FALSE,
   cd_coactivated=TRUE`.

## Results

### Classification (API responses)
```
A→B fire 1: {"processed":1,"weakened":0,"contradicted":1}   ← contradict create
C→D fire 1: {"processed":1,"weakened":1,"contradicted":0}   ← weaken (unchanged)
A→B fire 2: {"processed":1,"weakened":0,"contradicted":1}   ← contradict re-match
```

### Neo4j CONTRADICTS edge state after both fires
```
weight=0.15, evidence_count=2, created_at set, updated_at set
```

### TSDB rows (after writer flush)
```
trigger_path                        prev    new    delta     evid  new_edge  eta_null
apply_negative_feedback             0.119   0.000  -0.119    1     f         t   ← weaken: floored at 0, negative delta — unchanged behavior
apply_negative_feedback_contradict  0.000   0.150  +0.150    1     t         t   ← create branch ✓
apply_negative_feedback_contradict  0.150   0.150   0.000    2     f         t   ← re-match branch ✓
```

All Hebbian-only fields (`eta_effective`, `surprise_factor`,
`activation_product`, `path_sim`) NULL on every row, as designed.

### Federation read surfaces the contradict events
```
mdemg eventgraph reinforcement-neighborhood --seed <probe A> --hops 1 --json
→ events: 2 | trigger_paths: {apply_negative_feedback_contradict: 2}
  create:   delta=+0.1500 evid=1 new_edge=True  src_in=True dst_in=False
  re-match: delta=+0.0000 evid=2 new_edge=False src_in=True dst_in=False
```
Decision 3 confirmed live: the TSDB join is by node-id, so contradict events
surface without the walk traversing `CONTRADICTS` edges. `dst_in=False` is
the documented semantics (B is not in A's co-activation neighborhood).

### Tier 2 — UATS contract
`learning_negative_feedback.uats.json`: **5/5 specs PASS** against the live
server (hash-verified). Response contract (`processed/weakened/contradicted`)
unchanged by the two-statement split.

## Acceptance criteria — met

1. ✅ Contradict create: `trigger_path='apply_negative_feedback_contradict'`,
   `created_new_edge=true`, `delta_weight=+negWeight` (+0.15).
2. ✅ Contradict re-match: `evidence_count_after=2`, `delta_weight=0`,
   `created_new_edge=false`.
3. ✅ Weaken path behavior unchanged (negative delta, floor at 0,
   `created_new_edge=false`) — re-verified live post-split.
4. ✅ No schema/writer/endpoint change; federation read + CLI surface the new
   trigger_path for free.
5. ✅ EXPLAIN-validated statements; UATS 5/5; Tier 1 parser tests green;
   lint clean.

Probe nodes/edges remain in the `eg004-*` sessions as disclosed test data
(EVENTGRAPH-003 precedent).

## Conclusion

EVENTGRAPH-004's bar is met and live-verified: all Hebbian writes — including
the contradict action — now feed `reinforcement_events`, each with its own
`trigger_path`. The telemetry is in place *before* any producer calls the
endpoint, so when one is wired, the stream is observable from day one.
