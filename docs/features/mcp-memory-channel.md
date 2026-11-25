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

## Tool surface (26 tools)

15 memory tools (store/observe/correct/recall/associate/reject/reflect/status/
symbols/ingest×5/space_freshness — all space-aware), 6 Linear tools,
`validate_changes`, `jiminy_guide`, and three new in this sprint:

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

## Recording Jiminy rules over MCP (2026-11-25)

`memory_store` and `memory_associate` post to `/v1/memory/ingest`, which stores
plain `leaf` memory. Leaf nodes are searchable but are never promoted to Jiminy
rules: the rule promoters only read `conversation_observation` nodes. Before this
change an MCP-only agent (Cursor and VS Code, as configured by `mdemg init`) had
no way to create a rule.

- **`memory_observe`**: posts to `/v1/conversation/observe`. Rule wording ("must",
  "never", "always", "required", "should") is detected, and detected rules become
  enforced Jiminy rules at the next consolidation (automatic for the RSIC watchdog
  space; `POST /v1/memory/consolidate` elsewhere). `obs_type` defaults to
  `decision`; `progress`, `error`, `task` and `context` are never promoted. The
  tool result says whether rule wording was detected.
- **`memory_correct`**: posts to `/v1/conversation/correct` (`incorrect`,
  `correct`, optional `context`); becomes a correction rule at the next
  consolidation.
- Both default `session_id` to `mcp-agent`.

Detection is word-based, so any observed content with rule wording can become a
rule. The tool description tells agents to record one rule per call and to use
`memory_store` for notes.

What this does not add: blocking. Strict-mode blocking runs in the Claude Code
pre-edit hooks, and a rule escalates to blocking from feedback sent through
`/v1/jiminy/feedback`. MCP-only clients have neither, so for them the rules
surface as guidance through `jiminy_guide` and `validate_changes`. The same rules
do block Claude Code agents working in the same space.

## Operational notes

- A connected MCP client (e.g. a running Claude Code session) keeps its
  spawned `mdemg mcp` process until the client reconnects — new tools and
  space behavior appear on the next session/reconnect.
- The retrieve response's empty `content` field for parent nodes is the
  known RRF-SCALE-002 display follow-up (content lives on Observation
  children), unrelated to this sprint.

## Sprint record

`docs/development/mcp-revive-001/` — plan, post, live verification.
