# Sprint Plan — MCP-REVIVE-001: Unified Memory Across Agent Channels

## 1. Header & Metadata
Sprint: MCP-REVIVE-001 · 2026-06-11 · branch `reh3376_dev01` ·
Roadmap Q3 Phase 3 · effort ~3d · risk low-medium (additive params +
new tools; the stdio server has zero tests today).

## 2. Problem Statement
The MCP channel — the agent's direct memory interface — strands
observations outside the spaces hooks retrieve from: `internal/cli/mcp.go`
(1,635 lines, **zero tests**) hardcodes `defaultSpaceID = "ide-agent"`
at 8 handler sites and as the fallback at the 3 tools that do accept a
`space_id` param. Anything stored via MCP lands in `ide-agent` while the
hooks recall from `mdemg-dev` — fragmenting the connection layer per
client. The federation (eventgraph) and /strict surfaces have no MCP
exposure. Hygiene: server restarts orphan plugin processes (3 stale
generations from Apr 30 / May 1 / May 7 observed live) and the
session-spawned `mdemg mcp` runs day-old binaries.

## 3. Scope & Constraints
**In**: (1) space resolution chain on every memory-touching tool:
explicit `space_id` param > `MDEMG_SPACE_ID` env > `ide-agent`
(back-compat); repo `.mcp.json` gains `MDEMG_SPACE_ID: mdemg-dev`.
(2) Contract suite: table-driven Go tests over every tool handler
against an httptest backend — asserting HTTP method/path/body mapping
(esp. space-resolution precedence) and response shaping; this is the
UATS-style floor for the untested file. (3) New tools:
`eventgraph_reinforcement_neighborhood`,
`eventgraph_guidance_outcome_neighborhood`, `jiminy_strict` (toggle).
(4) Plugin-orphan hygiene: reap stale plugin processes holding our
socket paths at manager start (the kickstart -k kill path skips
graceful Stop). (5) Live smoke: store via a fresh `mdemg mcp` (JSON-RPC
over stdio against the real binary) → recall the same observation via
the hook path (`/v1/memory/retrieve`, same space). **Out**: new MCP
transport modes; Linear tool changes beyond the contract tests; a new
UxTS framework (Go tests are the right tool; no matrix change);
auth on the MCP channel.

## 4. Dependencies
`internal/cli/mcp.go`; `.mcp.json`; `internal/plugins/manager.go`;
mcp-go library (CallToolRequest construction in tests); live stack +
fresh stdio process for Tier 3; EVENTGRAPH-001/002 federation endpoints;
`/v1/jiminy/strict`.

## 5. Implementation Plan
Epic 0 plan · **Epic 1** space resolution (server-state default from
env; param on all memory tools; helper `resolveSpace`; .mcp.json) ·
**Epic 2** contract suite (httptest backend; per-tool table; precedence
tests) · **Epic 3** eventgraph ×2 + strict tools (+ contract entries) ·
**Epic 4** plugin-orphan reaping (socket-collision kill at Start,
loud log) · **Epic 5** live smoke + docs
(`docs/features/mcp-memory-channel.md`, CHANGELOG, CLAUDE.md note,
post.md), push.

## 6. Testing Plan
Tier 1: contract suite (every tool: request mapping, space precedence
explicit>env>fallback, error paths); reaper unit test (fake socket +
process). Tier 2: full `go test ./internal/...`; `mdemg mcp` binary
boots and lists 23 tools over stdio (initialize + tools/list round-trip).
Tier 3 (live): fresh `mdemg mcp` against the live server — `memory_store`
an observation WITHOUT explicit space_id under `MDEMG_SPACE_ID=mdemg-dev`
→ verify via `/v1/memory/retrieve` (the hook path) the observation is
recallable in `mdemg-dev`; eventgraph tool returns live federation data;
strict toggle round-trips; restart server → zero new orphaned plugin
processes (and the pre-existing orphans reaped).

## 7. Commit Strategy
Per-epic commits · lint each · push once (auto-PR) · summary comment ·
CI watch. Live-smoke surprises get own fix commits.

## 8. Verification Checklist
- [ ] All 12 memory tools accept space_id; precedence explicit>env>fallback
- [ ] .mcp.json carries MDEMG_SPACE_ID=mdemg-dev
- [ ] Contract suite covers all 23 tools; suite green
- [ ] eventgraph ×2 + strict tools live-verified
- [ ] Orphan reaper: restart leaves zero stale plugin processes
- [ ] Live smoke: MCP store → hook-path recall, same space
- [ ] Feature doc + CHANGELOG + CLAUDE.md + post.md

## 9. Documentation Update — Epic 5 (never cut).

## 10. Risks & Mitigations
Back-compat: existing `ide-agent` data — the env default changes only
the repo's own client config; bare installs keep ide-agent (documented;
operators migrate spaces explicitly). mcp-go request construction in
tests brittle → pin to current library version's public structs. Reaper
kills a legitimate process → match BOTH the socket path argument AND the
plugin binary path; loud warn per kill; dry-run log first in tests.
Session's own MCP connection uses the old binary until reconnect →
documented; smoke uses a fresh process.

## 11. Documents Accessed
ROADMAP_2026Q3.md:56; internal/cli/mcp.go (read); .mcp.json;
internal/plugins/manager.go; live ps inventory (orphan generations);
EVENTGRAPH-001/002 + /strict endpoint docs (CLAUDE.md).

## 12. Rollback Procedures
Code-only; revert commits. .mcp.json change is one line. Reaper is one
function, independently removable.
