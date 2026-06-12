# MCP-REVIVE-001 — Sprint Post

**Status: COMPLETE** · 2026-06-11 · branch `reh3376_dev01` · ~1d actual (vs 3d est)

## What shipped (per epic)

1. **Space resolution** (`2d0a6c2`) — explicit param > `MDEMG_SPACE_ID` >
   `ide-agent` across all memory tools; 6 tools gained the `space_id`
   param; `.mcp.json` sets `mdemg-dev`.
2. **Contract suite** (`06776da`) — 12 Go contracts over the
   previously zero-test surface (httptest backend; HTTP mapping, space
   precedence, single-space association, validation-before-HTTP,
   backend-500 → tool error).
3. **New tools** (`213fd36`) — `eventgraph_reinforcement_neighborhood`,
   `eventgraph_guidance_outcome_neighborhood` (seed-or-query; federation
   knobs omitted-when-unset), `jiminy_strict`. 23 tools total.
4. **Plugin-orphan reaper** (`213fd36`) — pgrep-by-socket-path reap at
   `startModuleInstance`, before spawn.
5. **Docs** — `docs/features/mcp-memory-channel.md`, CHANGELOG, CLAUDE.md,
   this post.

## Tier 3 live verification

- **Reaper**: 8 plugin processes across 4 generations before restart →
  **21 reap warnings** logged on first post-fix restart → exactly one
  generation of each plugin remains.
- **Stdio round-trip** (fresh `bin/mdemg mcp`, JSON-RPC over stdio,
  `MDEMG_SPACE_ID=mdemg-dev`): initialize ✓ · tools/list = **23** ✓ ·
  `memory_store` (no explicit space) → `n_1e6116a782aec5bf18b0` ✓ ·
  `eventgraph_reinforcement_neighborhood` by query → seed resolved, live
  federation data ✓ · `jiminy_strict` on/off round-trip ✓.
- **Unified-space proof**: the stored node + its Observation child
  (marker content verified in Neo4j) live in **mdemg-dev**, and the
  hook-path `/v1/memory/retrieve` recalls it (top-5; low score expected —
  fresh node, no Hebbian history).
- Full `go test ./internal/...`-relevant packages green; lint 0 issues.

## Notes / observations

- The retrieve response's empty parent-node `content` is the known
  RRF-SCALE-002 display follow-up (content lives on Observation
  children) — re-confirmed here, unrelated to this sprint.
- Test-decoy lesson: `sh -c` exec-optimizes the shell away, losing
  decorative argv — `tail -f <path>` carries the path like a real plugin.
- The session's own MCP connection runs the pre-sprint binary until
  reconnect; the smoke used a fresh process (documented in the feature
  doc).

## Documents Accessed

ROADMAP_2026Q3.md:56; internal/cli/mcp.go (read in full);
internal/plugins/manager.go; .mcp.json;
docs/api/api-spec/uats/specs/{eventgraph_reinforcement_neighborhood,
guidance_outcome_neighborhood}.uats.json; internal/api/handlers_jiminy.go
(strict handler); internal/cli/eventgraph.go (seed-or-query precedent);
internal/api/handlers.go + internal/retrieval/service.go (ingest
content path); live stack (ps inventory, Neo4j, stdio smoke).
