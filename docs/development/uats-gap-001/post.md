# UATS-GAP-001 — Sprint Post

Closed: 2026-06-11 · Branch: `reh3376_dev01` · Roadmap: Q3 Phase 2.

## Shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Sprint plan | `8be0b03` |
| fix | reformulate missing-context: 500 → 400 (live-caught while probing) | `b6d2b37` |
| fix | **P0**: nil-edge-identity panic killed the server during export (UATS-caught) | `bdc5169` |
| 1–4 | 8 new specs: jiminy_strict / reformulate / classify / warm / latest, admin_breakers_list / reset, memory_retrieve_sparse_context | (specs commit) |
| 5 | Suite hygiene: 8 env-conditional `*_disabled` variants → skip; deep-merge guard fix in j17_protocol_learn | (specs commit) |
| 6 | CHANGELOG + CLAUDE.md authoring-pitfalls note + post | (docs commit) |

## The forcing function worked — twice before any spec even ran

1. **Probing reformulate** for its contract found missing-`context`
   surfacing as 500 "internal error" — a request-validation failure must
   be 400. Fixed at the handler edge.
2. **Running the suite** crashed the server: the `backup_trigger` spec's
   live export hit an edge whose endpoint had no `node_id`, and the
   unchecked `fromID.(string)` assertions in `transfer/exporter.go`
   panicked the process (launchd restarted it; 167 connection errors were
   the symptom). The data shape is systemic (~231k edges to
   Observation/SymbolNode endpoints, which carry obs_id/symbol keys, not
   node_id) — an HTTP-triggerable request could kill the server at any
   time. Unexportable edges now skip with a warning.

## Verification (Tier 3 = the suite itself, live)

- New specs in isolation: **27/27 cases, 100%**, all hashes verified.
- Canonical gate `make test-api`: **0 failed, 0 errors**, 425 hashes
  verified (18 legit skips), after the P0 fix and suite hygiene.
- Full-corpus untagged run (479 cases incl. tsdb/llm-tagged specs that
  `make test-api` excludes): 30 pre-existing failures remain in the
  never-CI'd subset — **UXTS-CI-001 evidence**, not this sprint's scope
  (that sprint owns un-excluding the tsdb-tagged contracts + wiring the
  suite into CI).

## Conventions established (recorded in CLAUDE.md)

- Required-field variants use empty-string overrides (deep-merge).
- Disabled-path 503 contracts are `skip: true` variants with the
  documented contract in `skip_reason`.

## Documents Accessed

`sprint_plan_uats_gap_001.md` §11; `uats_runner.py`; `Makefile`
(test-api); `handlers_jiminy.go`, `handlers_breakers.go`, `handlers.go`,
`transfer/exporter.go`; live `/tmp/api-report.json`,
`~/.mdemg/logs/server.log` (panic stack), live endpoint probes.
