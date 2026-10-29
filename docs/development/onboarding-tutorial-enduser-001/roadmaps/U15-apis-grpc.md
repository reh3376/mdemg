# U15 · APIs + gRPC (9 slides)

**Intent**: HTTP REST (199 routes under `/v1/*`) + 4 gRPC services + when operators touch which + inspection via `curl`/`grpcurl` + security boundary + the DORMANT-CENSUS forcing-function registries.
**Target slides**: 9
**Mermaid**: 1 (U15.2, surface map)
**Natural Q&A anchor**: after U15

---

## U15.1 · Section Landing

**Title**: `U15 · APIs + gRPC, The Programmatic Surfaces`

**Body**:
- Intent: "/ui/ and CLI are user-facing; APIs + gRPC are what EVERYTHING hits internally. 199 shipped HTTP routes + 4 gRPC services. Operators mostly OBSERVE these surfaces; developers extend them."
- Preview: surface map → HTTP /v1/* → 4 gRPC services → inspection patterns → DORMANT-CENSUS registries → security guards → trajectory → gotchas
- Points back to U1.4 "How does it solve them?", these are the implementation of surfaces #3 and #4

**Provenance**:
- `internal/api/server.go`, HTTP route registration
- `api/proto/*.proto`, gRPC schemas (4 shipped)
- `docs/api/route_consumer_inventory.json`, DORMANT-CENSUS-001 adjudicated routes

---

## U15.2 · The Surface Map (Mermaid)

**Title**: `Who Talks To What`

**Body**:
- **Mermaid #7** (surface map):
  - **Operators** → via CLI/UI → **mdemg HTTP :9999** `/v1/*`
  - **Agents (Claude Code hooks)** → via HTTP → **mdemg HTTP :9999** `/v1/*`
  - **Grafana** → via `timescaledb` datasource → **TimescaleDB :5432** (direct SQL, bypasses mdemg HTTP)
  - **MCP clients** → via Unix socket → **mdemg mcp** subprocess → HTTP → mdemg
  - **Plugins** → via gRPC → **mdemg-module** protocol → **mdemg** subprocess manager
  - **Dev-space** → via gRPC → `devspace.proto` interface
  - **UNTS** (hash registry) → via gRPC → `unts.proto`
  - **Space-transfer** → via gRPC → `space-transfer.proto` (cross-instance space replication)

**Provenance**:
- `api/proto/{mdemg-module,unts,devspace,space-transfer}.proto`
- `internal/{plugins,unts,devspace,transfer}` packages

**note**:
- Operator cue: "The HTTP API is the central nervous system. Everything else is specialized protocols for performance or boundary reasons."

---

## U15.3 · HTTP REST, `/v1/*` (199 routes)

**Title**: `The HTTP API, /v1/* Routes`

**Body**:
- 199 routes shipped (DORMANT-CENSUS-001 census)
- Grouped by domain:
  - `/v1/memory/*`, observe, recall, retrieve, correct, store (U3)
  - `/v1/jiminy/*`, latest, warm, feedback, evaluate, classify, strict, mode, override, rules CRUD (U4)
  - `/v1/conversation/*`, observe, correct
  - `/v1/self-improve/*`, assess, reflect (RSIC)
  - `/v1/retrieve` (direct retrieval; `?sparse=`, `?intent=`, `?reverse_ref=`, `?context=` URL overrides)
  - `/v1/review/*`, datasets, candidates, grade, reverse, autograde-preview (U7)
  - `/v1/alerts/*`, list, clear (U16)
  - `/v1/admin/*`, breakers (list, reset); gated by `AUTH_API_KEYS`
  - `/v1/eventgraph/*`, reinforcement-neighborhood, guidance-outcome-neighborhood (EVENTGRAPH-001/002)
  - `/v1/metrics/snapshot`, JSON metrics (no `/metrics` endpoint by design, PROMETHEUS-SCRAPE-INVESTIGATION-001)
  - `/ui/*`, UI static assets + the embedded tutorial
  - `/healthz`, subsystem probe

**Provenance**:
- `internal/api/server.go`, the mux
- `docs/api/route_consumer_inventory.json`

**note**:
- Operator cue: "`curl -s http://localhost:9999/v1/<endpoint> | jq` is the operator's debugging flashlight."

---

## U15.4 · gRPC, The 4 Services

**Title**: `The 4 gRPC Services`

**Body**:
- **mdemg-module** (`api/proto/mdemg-module.proto`), plugin interface; plugins implement, mdemg calls
- **unts** (`api/proto/unts.proto`), Universal Hash Test Specification registry service; verify + revert operations
- **devspace** (`api/proto/devspace.proto`), DevSpace workspace coordination
- **space-transfer** (`api/proto/space-transfer.proto`), export/import spaces across mdemg instances
- All 4 use gRPC over Unix sockets for in-process communication (performance; also easier security boundary vs TCP localhost)
- gRPC services NOT exposed over HTTP to external clients, they're internal to the mdemg process family

**Provenance**:
- `api/proto/*.proto`
- `internal/{plugins,unts,devspace,transfer}/*.go`

**note**:
- Operator cue: "gRPC is implementation detail unless you're writing a plugin. If you're writing a plugin, start with `api/proto/mdemg-module.proto`."

---

## U15.5 · Inspection, `curl` + `grpcurl`

**Title**: `Probing The Surfaces`

**Body**:
- HTTP:
  ```bash
  $ curl -s http://localhost:9999/healthz | jq
  $ curl -s http://localhost:9999/v1/memory/retrieve -X POST \
      -H "Content-Type: application/json" \
      -d '{"query":"foo","space_id":"mdemg-dev"}' | jq
  $ curl -s http://localhost:9999/v1/metrics/snapshot | jq .counters
  ```
- gRPC (needs `grpcurl` installed):
  ```bash
  $ grpcurl -plaintext unix:///path/to/plugin.sock list
  $ grpcurl -plaintext unix:///path/to/plugin.sock describe mdemg.Module
  ```
- Admin endpoints require auth: `curl -H "Authorization: Bearer $MDEMG_ADMIN_KEY" ...`

**Provenance**:
- `internal/api/server.go` + `internal/plugins/manager.go`

**note**:
- Operator cue: "`curl | jq` is the most-used operator debugging tool. Make sure `jq` is installed."

---

## U15.6 · DORMANT-CENSUS Forcing Functions

**Title**: `How New Surfaces Stay Audited`

**Body**:
- DORMANT-CENSUS forcing functions catch unused surfaces at merge time:
  - **DORMANT-CENSUS-001** (routes): every new mux-registered route needs `docs/api/route_consumer_inventory.json` entry; CI merge-blocks
  - **DORMANT-CENSUS-002** (TSDB tables): every new `migrations/*.sql` hypertable needs `docs/api/tsdb_consumer_inventory.json` entry
  - **DORMANT-CENSUS-003** (metrics): every declared gauge/counter/histogram needs `docs/api/metrics_consumer_inventory.json` entry
- Pre-commit hook `check_inventory_adjudication` catches at commit time (PRE-COMMIT-INVENTORY-GATE-001)
- Rationale: prevent "ship-and-forget" surfaces that languish unused + prevent silent breakage when a consumer disappears but the surface stays

**Provenance**:
- CLAUDE.md "DORMANT-CENSUS-001/002/003" + "PRE-COMMIT-INVENTORY-GATE-001"
- `scripts/verify_route_consumers.py`, `verify_tsdb_consumers.py`, `verify_metrics_consumers.py`

**note**:
- Operator cue: "If you're adding a route and CI fails on 'route_consumer_inventory.json needs entry', run `python3 scripts/verify_route_consumers.py --generate` + commit the inventory file."

---

## U15.7 · Security Boundary

**Title**: `What's Reachable, What's Not`

**Body**:
- Default bind: `127.0.0.1:9999` (localhost only); `MDEMG_BIND_ADDR=0.0.0.0` to expose on LAN (operator decision)
- Admin endpoints (`/v1/admin/*`): require `AUTH_API_KEYS` header; `AUTH_JWT_SECRET` for JWT (both masked by `isSensitive`)
- Path-safe guards on filesystem-touching routes (SEC-TRANCHE-2): all HTTP handlers constructing filesystem paths from URL segments route through `pathsafe.SafeJoinUnderDir`; regex allowlists at distant callers DO NOT close CodeQL flow
- Dormant-census routes stay BOUND but return 503 when their disposition is `DORMANT`; routes for shipped features return normal responses
- gRPC services bound to Unix sockets with 0600 perms (operator-only)

**Provenance**:
- CLAUDE.md "Path-injection tranche (SEC-TRANCHE-2)"
- `internal/pathsafe/`
- `internal/config/yaml_config.go::isSensitive`

**note**:
- Operator cue: "Default bind is localhost-only. ONLY flip `MDEMG_BIND_ADDR=0.0.0.0` after you've secured auth + firewall rules."

---

## U15.8 · Current State + Trajectory

**Title**: `APIs + gRPC Today, Honest`

**Body**:
- Current state (`mdemg-dev`, 2026-10):
  - 199 HTTP routes shipped, 195 live, 4 dormant (DORMANT-CENSUS-001 adjudicated)
  - 4 gRPC services shipped; all 4 active + testable
  - CodeQL backlog: SEC-TRANCHE-3/4 pre-existing (not caused by recent sprints); separate security sprint will close
- Trajectory items:
  - 199 routes is a LOT, DORMANT-CENSUS forcing function keeps this honest; next-sprint route adds trigger the adjudication registry
  - gRPC surface stable at 4; a 5th (recursive-retrain trigger) deferred to FT-RECURSIVE-005
- The `/metrics` endpoint intentionally doesn't exist, mdemg uses TSDB-recorded `metric_samples` + `/v1/metrics/snapshot` instead of Prometheus scrape (PROMETHEUS-SCRAPE-INVESTIGATION-001 verdict: (c) DELIBERATE REMOVAL per the 2026-03-28 Prometheus→TSDB migration)

**Provenance**:
- CLAUDE.md DORMANT-CENSUS pin + PROMETHEUS-SCRAPE-INVESTIGATION-001

**note**:
- Operator cue: "If a dashboard looks for `/metrics` and 404s, that's intentional. Use `/v1/metrics/snapshot` for live + SQL over `metric_samples` for historical."

---

## U15.9 · Gotchas + Where To Go Deeper

**Title**: `APIs + gRPC, Gotchas + More`

**Body**:
- Gotchas:
  - 199 routes isn't the right count to memorize; adjudicate via inventory JSON when needed
  - CodeQL doesn't recognize regex validation as a path-injection sanitizer, route through `pathsafe.SafeJoinUnderDir` (SEC-TRANCHE-2)
  - `#nosec` is gosec-only; CodeQL ignores it (close CodeQL alerts via sanitizer routing OR explicit `gh api PATCH` dismissal with rationale)
  - `writeInternalError` auto-classifies caller cancellation as 499 (not 500), any new handler routing through this helper inherits the behavior; new handlers that DON'T route through it must add their own classification (RETRIEVE-CALLER-CANCEL-001 arch rule)
  - gRPC Unix sockets persist until mdemg restart; stale sockets reaped on `kickstart -k` by pgrep (MCP-REVIVE-001)
- Feature docs:
  - `docs/api/api-spec/uats/specs/*.json`, 215 UATS contracts covering most HTTP routes (U6)
  - `docs/features/*.md`, many feature docs include their /v1/* endpoint references
  - CLAUDE.md DORMANT-CENSUS-001/002/003 pins
- Where this feeds into: U13 Grafana (reads TSDB directly, bypasses this API surface), U16 Alerts (admin breaker-reset uses /v1/admin/*)

**note**:
- Operator cue: "You rarely need to KNOW the API surface. You often need to PROBE it. `curl -s | jq` is the operator superpower."
- Transition: "U16 next, Alerts + Enforcement Escape Hatches, the operational decision surface."
