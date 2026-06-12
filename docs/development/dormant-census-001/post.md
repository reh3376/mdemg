# Sprint Post — DORMANT-CENSUS-001

2026-06-12 · branch `reh3376_dev01` · the FINAL committed Q3 roadmap
sprint. Plan: `sprint_plan_dormant_census_001.md`.

## What shipped

1. **Epic 1 — inventory + gate** (`fbd2402`, `7bf04fa`):
   `scripts/verify_route_consumers.py` (bidirectional drift +
   UNREVIEWED detection, `--generate` bootstrap) + the adjudicated
   187-route `docs/api/route_consumer_inventory.json` + the
   merge-blocking ci.yml step beside the config-consumer guard.
   Final dispositions: 109 ACTIVE / 60 OPERATOR_SURFACE / 13 INTERNAL /
   4 PRUNED / 1 DEFERRED:GUARDRAIL-or-NEGFEED-producer. 171/187 routes
   carry matched UATS specs.

2. **Epic 2 — SignalLearner wire** (`df4ac3c`): `GetStrength` added to
   `SignalLearnerProvider`; `Guide()` orders within equal priority by
   `(1-w)·confidence + w·strength`, `JIMINY_SIGNAL_STRENGTH_WEIGHT`
   default 0.2, 0 = off. 6 Tier-1 tests pin the contract.

3. **Epic 3 — prune pass** (`c81dba0`): `/v1/feedback`,
   `/v1/memory/ingest-codebase[/]`, `POST /v1/alerts/grafana` removed
   (handlers, isolated service code, compose env, contactpoints
   comment, 6 UATS specs, UXTS matrix 220→214).

4. **Live-smoke fix commit** (`a48aaf4`): jiminy codegen
   collision-path self-deadlock (below).

## Census reversals (the method working)

- `/viz/topology` + `/api/graph/*` were on the PLAN's prune list —
  both are live Grafana consumers (topology-dashboard iframe at
  `mdemg-graph-topology.json:81`; nodegraph datasource
  `provisioning/datasources/nodegraph-api.yml`). NOT pruned.
- `/v1/conversation/snapshot*`: the recon claim "consumed by
  pre-compact.sh" did NOT verify — the hook saves via
  `/v1/conversation/observe`. Marked OPERATOR_SURFACE; no rewire
  needed (the observe path works).
- PREDICTS/FORESHADOWS: the recon lane's "remove from allowed
  relationship types" was a hallucination — those types exist nowhere
  in code (only in untracked research notes). No-op, disclosed.
- The embedded `/ui/` dashboard consumes ~35 routes invisible from
  hooks/CLI/scripts — the single biggest reason "no grep hits" must
  never equal "dormant".

## Live Tier 3

- Gate green: 183 live / 187 inventoried (4 PRUNED), exit 0.
- All 3 pruned routes return 404 live; neighbors (`/v1/alerts/clear`,
  capability-gaps) unaffected.
- SignalState variance in Neo4j is real learning: strengths 0.1–0.75
  over thousands of emissions (`guidance:pattern` 0.75 / 3,495
  emissions / 276 responses vs `guidance:concept` 0.1).
- Ordering extreme test: with `JIMINY_SIGNAL_STRENGTH_WEIGHT=1.0` and
  one signal temporarily boosted to 0.9 (original value recorded and
  restored, verified stable post-flush), the strength-0.9 item ordered
  ABOVE a conf-0.510/strength-0.1 item in the same priority group —
  strict confidence-order inversion by learned effectiveness.
  `.env` override removed after the test; server back on defaults.
- Full UATS suite post-fix: **388 passed / 0 failed / 18 skipped**;
  3 read-timeout errors under full-suite LLM saturation all pass 100%
  in isolation (the known local-LLM latency class).

## Live-smoke catch: codegen self-deadlock (own fix commit, precedent)

UATS `conversation_observe_pinned` hung 45–90s deterministically. The
goroutine dump (SIGQUIT) showed the cause exactly:
`ConstraintCodeGenerator.GenerateCode`'s collision branch holds `g.mu`
(codegen.go:93) and called `fallbackCode`, which locks `g.mu` again
(:121). sync.Mutex is not reentrant — the first LLM-returned code that
collided with a registered code (precisely what repeated UATS runs
with identical content produce) **wedged the generator for the life of
the process**, and every later constraint-typed
`/v1/conversation/observe` queued behind it (holder 18 min wedged, N
waiters at :47). The 44s observe latencies seen earlier in the day
were the same class. Fix: `fallbackCodeLocked` for callers already
holding the mutex; regression test drives the real collision path
through a fake OpenAI-compat endpoint with a 10s deadlock tripwire.
Post-fix: the exact failing request runs 1.8s cold / 17ms deduped.

## Follow-ups recorded (not actioned)

- `/v1/learning/negative-feedback` stays DEFERRED until the producer
  sprint (EVENTGRAPH-004 disclosure; GUARDRAIL-or-NEGFEED).
- `/v1/memory/reflect` is dormant-in-disguise (MCP `memory_reflect`
  calls retrieve, not reflect) — INTERNAL with note; candidate for a
  future wire-or-prune.
- Inventory liveness re-adjudication cadence (DOC-AUDIT-style) —
  the gate checks declarations, not evidence freshness.
- Full-suite UATS LLM saturation timeouts (3 specs) — pre-existing
  latency class, passes in isolation.

## Verification checklist (plan §8)

- [x] Inventory covers all 187 routes; gate merge-blocking + green
- [x] All 58 flagged routes verified; false positives corrected
- [x] GetStrength wired (weight config, 0=off); ordering live-verified
- [x] Prune list: each route individually re-verified before removal
- [x] PREDICTS/FORESHADOWS — resolved as no-op (exist nowhere in code)
- [x] ft_* KEEP override + triggers recorded in inventory (DEFERRED notes)
- [x] Feature doc (`docs/features/dormant-census.md`) + CHANGELOG + post

## Documents Accessed

ROADMAP_2026Q3.md:57; the 3 recon lane reports (a52df2bc / ac6551ae /
a628c8cd); internal/api/server.go route table; internal/ape/
signal_learner.go; internal/jiminy/{service,types,codegen}.go;
internal/gaps/detector.go; deploy/docker/grafana/provisioning/*;
internal/cli/hook_templates/pre-compact.sh; CHANGELOG.md:1253 (Phase 94
deprecation); docs/development/UXTS_FRAMEWORK_MATRIX.md;
scripts/verify_config_consumers.py (gate precedent); ci.yml;
EVENTGRAPH-004 sprint record (negative-feedback producer disclosure).
