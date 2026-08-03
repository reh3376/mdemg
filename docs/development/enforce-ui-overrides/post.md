# ENFORCE-UI-OVERRIDES — Sprint Post

**Date:** 2026-08-03 | **Branch:** `reh3376_dev01`
**Parent:** JIMINY-ENFORCE arc-adjacent follow-up (2/3)
**Trigger:** ENFORCE-OVERRIDES-TSDB shipped the queryable data; this sprint exposes it in the /ui/ Jiminy tab so operators can see active overrides + recent apply/revoke/expire history without shell-out to psql or CLI.

## Verdict

**Shipped.** Jiminy tab now has 3 sections: (1) existing Mode toggle (JIMINY-MODE-001), (2) NEW Active Overrides table with per-row Revoke buttons, (3) NEW Recent Override Timeline reading the constraint_overrides hypertable. New `GET /v1/jiminy/override/history` endpoint. Live-verified: seeded override appears in both active + history endpoints; UI JS deployed at `/ui/tabs/jiminy.js`.

## What shipped

### E1 — HTTP endpoint (`internal/api/handlers_jiminy.go`)
`GET /v1/jiminy/override/history?space_id=X&hours=168` → `{data: {events: [...], count: N, space_id, hours}}`
- Constructs a fresh `tsdb.NewDatasetBuilder` per request (cheap wrapper around the pool; read endpoint runs infrequently)
- Defaults: space_id → RSICWatchdogSpaceID → "mdemg-dev"; hours → 168 (7d)
- Always returns `events: []` (never null) — client contract
- 503 when TSDB disabled; 405 on non-GET

### E2 — Route registration (`internal/api/server.go`)
Distinct path `/v1/jiminy/override/history` — separate from the shipped `/v1/jiminy/override` active-list endpoint so method-multiplex on the parent path doesn't collide.

### E3 — API helpers (`internal/api/ui/api.js`)
Four new exports:
- `jiminyOverrideList(sessionID)` — active list (existing endpoint)
- `jiminyOverrideApply(sessionID, code, reason, durationSec)` — POST apply
- `jiminyOverrideRevoke(sessionID, code)` — DELETE with body (uses direct fetch since the shared `del()` helper doesn't accept a body)
- `jiminyOverrideHistory(spaceID, hours)` — GET history (new endpoint)

### E4 — UI (`internal/api/ui/tabs/jiminy.js`)
Rewrote the tab as 3 stacked sections. `load()` fires all 3 fetches in parallel via `Promise.allSettled` — each panel degrades gracefully if its endpoint errors (mode failure doesn't kill overrides render, etc).

**Active Overrides table:**
- Columns: Constraint, Session, Reason, Expires (localized), Actions (Revoke button)
- Revoke button uses `confirm()` before firing; success → reload
- Empty state points to `mdemg jiminy override apply` CLI

**Recent Override Timeline:**
- Columns: Time, Op (color-coded badge), Constraint, Session, Reason
- Op badges: apply → warn (yellow), revoke → ok (green), expire → default (grey)
- Truncated to 50 rows with count footer
- Help panel explains the apply/revoke/expire semantics + how they feed the RSIC `enforcement_false_positive_high` pattern (ENFORCE-004-FOLLOWUP)

### E5 — Route inventory adjudication
`/v1/jiminy/override/history` added to `docs/api/route_consumer_inventory.json` as IN_USE with the UI consumer listed. Verifier passes.

## Live Tier-3 (mdemg-dev, 2026-08-03)

```bash
# Rebuild + restart + seed override
$ mdemg jiminy override apply --constraint UI-SMOKE-CODE \
    --reason "ui smoke test" --duration 10m

# Active list endpoint
$ curl -s http://localhost:9999/v1/jiminy/override | jq .data
{
  "count": 1,
  "overrides": [{"session_id":"claude-core","constraint_code":"UI-SMOKE-CODE",
                 "reason":"ui smoke test","applied_at":"...","expires_at":"..."}]
}

# History endpoint (V0033 TSDB read)
$ curl -s "http://localhost:9999/v1/jiminy/override/history?hours=24" | jq .data
{
  "count": 1, "hours": 24, "space_id": "mdemg-dev",
  "events": [{"time":"...","session_id":"claude-core",
              "constraint_code":"UI-SMOKE-CODE","reason":"ui smoke test",
              "op":"apply","applied_at":"...","expires_at":"..."}]
}

# UI JS deployed
$ curl -sf http://localhost:9999/ui/tabs/jiminy.js | head -1
// jiminy.js — Jiminy mode + operator overrides + enforcement timeline

# Cleanup
$ mdemg jiminy override revoke --constraint UI-SMOKE-CODE
```

Test override cleaned up post-verification. Browser-rendered UI verification is the next hop (operator's Ctrl-R in an open /ui/ Jiminy tab shows the 3 sections).

## Rules pinned

⚠️ **When an existing endpoint is method-multiplexed (POST/GET/DELETE on one path), add related read-only paths as DISTINCT paths — not as query-param toggles on the existing one.** `/v1/jiminy/override/history` is a separate path from `/v1/jiminy/override` because folding "history" as a query param would tangle the method-multiplex logic (GET already means "active list"; adding "GET with ?history=true means history" hurts readability + tooling). Distinct paths preserve REST-shape + route-inventory adjudication clarity.

⚠️ **UI tab data loads MUST use `Promise.allSettled`, not `Promise.all`, when each panel is independent.** With `Promise.all` a single endpoint failure blanks the whole tab; with `allSettled` each panel renders (or shows its own error) on the data it did receive. This tab has 3 independent fetches (mode, active, history); operator sees the parts that work even when one path is down.

## Not shipped (arc-adjacent, disclosed)

- **RSIC action-execution layer for enforcement patterns** (last of the 3 items) — consumes `EnforcementOutcomes` + `OverrideHistory` to auto-execute `archive_ineffective_constraints` on chronically-overridden codes. Currently RSIC EMITS the insight (ENFORCE-004-FOLLOWUP); the executor doesn't act on it.
- **UI: apply-new-override form** — operators still use the CLI. A form + POST integration would let the operator install overrides directly from the browser.
- **UI: polling** — currently loads on tab open. A 30s poll would keep the timeline fresh without manual refresh.

## Rollback

Single-commit revert. Any `constraint_overrides` rows already written persist (readers are additive). Route inventory entry stays as a `DORMANT_TO_REMOVE` candidate for a future cleanup — no runtime impact.

## Documents Accessed

- ENFORCE-OVERRIDES-TSDB post (TSDB reader shape)
- JIMINY-MODE-001 post (tab pattern)
- `internal/api/handlers_jiminy.go` (existing handler patterns)
- `internal/api/server.go` (route registration)
- `internal/api/ui/api.js` (helper patterns)
- `internal/api/ui/tabs/jiminy.js` (rewritten to 3 sections)
- `docs/api/route_consumer_inventory.json` (adjudicated)
- Live: all 3 endpoints returned expected data; UI JS deployed
