# MCP Memory Channel — Unified Spaces, Federation Tools, Contract Floor (MCP-REVIVE-001)

## Why

The MCP server (`mdemg mcp`, stdio) is the agent's direct memory interface —
and it hardcoded `space_id = "ide-agent"` at every memory operation. Anything
an agent stored via MCP landed in `ide-agent` while the hooks recalled from
the project's space (`mdemg-dev` here): the connection layer was fragmented
per client. The 1,635-line tool surface also had zero tests, no exposure of
the event-graph federation or /strict surfaces, and server restarts orphaned
plugin processes (3 stale generations from Apr 30 / May 1 / May 7 were live).

## Space resolution

Every memory-touching tool resolves its space as:

1. explicit `space_id` argument on the tool call
2. `MDEMG_SPACE_ID` env var on the MCP server process
3. `ide-agent` (back-compat fallback)

The repo's `.mcp.json` sets `MDEMG_SPACE_ID=mdemg-dev`, so this repo's agent
channel and hook channel share one memory space. Live-verified end-to-end:
`memory_store` (no explicit space) → node + Observation in `mdemg-dev` →
recalled via the hook path (`/v1/memory/retrieve`).

Existing `ide-agent` data is untouched; bare installs (no env, no param)
keep the old behavior. Operators who want historical ide-agent observations
in their project space migrate them explicitly.

## Tool surface (23 tools)

12 memory tools (store/recall/associate/reflect/status/symbols/ingest×5/
space_freshness — all space-aware), 6 Linear tools, `validate_changes`,
`jiminy_guide`, and three new in this sprint:

- **`eventgraph_reinforcement_neighborhood`** — Hebbian activity around a
  memory (EVENTGRAPH-001 federation): seed via `seed_node_id` or `query`
  (top-retrieval resolution, the CLI precedent); `hops`/`since_hours`/
  `limit` are OMITTED when unset so server config stays the single source
  of truth.
- **`eventgraph_guidance_outcome_neighborhood`** — followed/ignored/
  contradicted outcomes for constraints in a node's neighborhood
  (EVENTGRAPH-002).
- **`jiminy_strict`** — toggle /strict deterministic governance per session.

## Contract floor

`internal/cli/mcp_contract_test.go`: every handler invoked against an
httptest MDEMG backend, asserting the HTTP mapping (method/path/body), the
space-resolution precedence, the single-space association invariant,
validation-before-HTTP, omit-when-unset federation defaults, and
backend-500 → tool error (never a Go error).

## Plugin-orphan reaping

`launchctl kickstart -k` kills the server without running `Manager.Stop`,
orphaning plugin children. `startModuleInstance` now reaps any
prior-generation process holding the module's socket path (full-path
`pgrep -f` match — surgical; loud warn per kill) before spawning.
Live-verified: 21 orphans reaped on the first post-fix restart; exactly one
generation of each plugin remains.

## Operational notes

- A connected MCP client (e.g. a running Claude Code session) keeps its
  spawned `mdemg mcp` process until the client reconnects — new tools and
  space behavior appear on the next session/reconnect.
- The retrieve response's empty `content` field for parent nodes is the
  known RRF-SCALE-002 display follow-up (content lives on Observation
  children), unrelated to this sprint.

## Sprint record

`docs/development/mcp-revive-001/` — plan, post, live verification.
