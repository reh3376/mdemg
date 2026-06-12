# Dormancy Census — the Route↔Consumer Inventory Gate

**Sprint**: DORMANT-CENSUS-001 (2026-06-12) · **Status**: shipped, merge-blocking in CI

## Why

The quarter's worst bug classes shared one shape: **a surface with no
consumer, invisible until someone looked**. The 24-day Hebbian no-op
(a score field silently dropped), the 9-week guidance dormancy (a
threshold no retrieval score could clear), dead RSIC actuators, tables
with writers and no readers, the flat-dead surprise multiplier — every
one was found by a one-off census. This feature makes the census
standing: every registered API route must carry an adjudicated,
machine-checked consumer declaration, and the check is merge-blocking.

The census's own findings prove greps are not an inventory: recon
flagged 58 routes as zero-consumer, but `/v1/jiminy/classify` is called
by the /strict hook, `/viz/topology` is iframed by a Grafana dashboard,
`/api/graph/*` serves the Node Graph datasource, and the embedded
`/ui/` operator dashboard alone consumes ~35 routes invisible from
hooks/CLI/scripts. Conversely, a recon claim that pre-compact.sh calls
`/v1/conversation/snapshot` did NOT verify (it saves via
`/v1/conversation/observe`). Evidence-based adjudication, re-verified
per route, is the only honest form.

## Choices

- **One inventory file, not per-UATS fields** — covers all routes
  including the spec-less ones; per-spec backfill can come later.
  (Disclosed deviation from the roadmap's original wording.)
- **Dispositions, not deletions, for everything ambiguous** — only
  verified zero-producer routes with a named successor were pruned.
- **PRUNED entries are retained** with `removed_in`, so the ledger is
  also a history.
- **The gate is bidirectional** — a new route without an entry fails CI
  (new surface without a consumer declaration), and an entry whose
  route vanished fails CI (stale ledger).

## How it works

- `docs/api/route_consumer_inventory.json` — one entry per route:
  `consumers` (hook:/cli:/mcp:/script:/dashboard:/ui:/internal: with
  file:line evidence), `uats_specs`, `disposition`, `notes`.
- Dispositions: `ACTIVE` (production consumer), `OPERATOR_SURFACE`
  (manual escape hatch, intentionally automation-free), `INTERNAL`
  (server-internal / eval / framework surface), `DEFERRED:<trigger>`
  (future consumer named, e.g. the negative-feedback producer),
  `PRUNED` (removed, entry retained), `UNREVIEWED` (bootstrap marker —
  the gate fails on these).
- `scripts/verify_route_consumers.py` extracts the live route table
  from `internal/api/server.go` (`mux.Handle*` registrations) and
  verifies both directions. `--generate` bootstraps entries for new
  routes (as UNREVIEWED — you must adjudicate them).
- CI: the "Route consumer guard" step in `ci.yml`'s lint job, beside
  the config-consumer guard from CONFIG-DEADFLAG-001. Both gates kill
  the same class at different layers: config fields and HTTP surfaces.

## How to use

Adding a route: register it in server.go, run
`python3 scripts/verify_route_consumers.py --generate`, edit your new
entry with real consumers + disposition, commit both in the same PR.

Removing a route: delete the registration + handler, set the entry's
disposition to `PRUNED` with `removed_in`, remove/adjust any UATS specs
covering it (and update `UXTS_FRAMEWORK_MATRIX.md` counts).

## The headline wire: SignalLearner.GetStrength

The census's flagship dormant surface was not a route but a read side:
`ape.SignalLearner` (V0024 SignalState persistence, supervised 30s
flush, startup hydration, live emission/response stream since
HOOKWIRE-001) had **zero production callers** of `GetStrength`. The
learner had been learning for months with real variance (live:
strengths 0.1–0.75; `guidance:pattern` 0.75 vs `guidance:concept` 0.1)
and nothing ever read it.

It now feeds guidance ordering: `jiminy.Guide()` sorts within equal
priority by `(1-w)·confidence + w·strength` where
`w = JIMINY_SIGNAL_STRENGTH_WEIGHT` (default **0.2**; `0` = off,
restoring pure-confidence order; clamped to 1). Ordering only —
selection and filtering are untouched, so no guidance is gained or
lost, only re-ranked toward historically-effective signal codes.
Unknown codes blend the learner's 0.5 neutral default.

Live verification: at `w=1.0` a strength-0.9 item ordered above a
higher-confidence strength-0.1 item within the same priority —
learned effectiveness inverting raw confidence, which is the point.

## Pruned in this sprint

| Route | Successor / reason |
|---|---|
| `/v1/feedback` | `/v1/jiminy/feedback` (the live feedback channel); zero producers; isolated `gaps.ProcessFeedback` removed with it |
| `/v1/memory/ingest-codebase[/]` | `/v1/memory/ingest/*` (deprecated since Phase 94 with a `Deprecation` header) |
| `POST /v1/alerts/grafana` | server-native alert evaluator (SR-001/SNA-001); only reference was a commented-out contactpoint; compose env `MDEMG_GRAFANA_ALERT_WEBHOOK_URL` removed |

Restoration of any pruned route is a git revert plus flipping its
inventory entry back.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `JIMINY_SIGNAL_STRENGTH_WEIGHT` | 0.2 | Blend weight for learned signal strength in Guide() ordering; 0 = off |

## Limitations

- The inventory verifies *declarations*, not liveness — a consumer
  listed with stale evidence won't fail the gate. Periodic
  re-adjudication (DOC-AUDIT-style) keeps it honest.
- Route extraction is regex-based over server.go; routes registered
  outside `mux.Handle*` literals there would be invisible (none exist
  today).
