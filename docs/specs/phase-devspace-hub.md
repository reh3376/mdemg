# Phase 2: DevSpace Hub and Out-of-Band Distribution

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Parent plan:** [development-space-collaboration.md](./development-space-collaboration.md)  
**Status:** ✅ Complete (verification 2026-01-22; ready for production use)  
**Date:** 2026-02-06

---

## Goal

- **DevSpace:** A named collaboration group (e.g. `my-team`) with a **hub** that tracks registered agents and a **catalog** of published exports.
- **Out-of-band distribution:** Agents (or users) **publish** an export to the hub; other **registered** members **list** and **pull** exports by name/space_id without ad-hoc file transfer.
- MVP: in-memory or file-based catalog; no auth required for first iteration (auth can be Phase 2b).

---

## Requirements

### Functional

- FR-1: Hub exposes **DevSpace** gRPC service: `RegisterAgent`, `DeregisterAgent`, `ListExports`, `PublishExport`, `PullExport`.
- FR-2: `RegisterAgent(DevSpaceId, AgentId, optional Metadata)` — register an agent in a DevSpace; idempotent.
- FR-3: `ListExports(DevSpaceId)` — return list of published exports (space_id, published_at, published_by_agent_id, optional label).
- FR-4: `PublishExport(DevSpaceId, SpaceId, ExportRef)` — register an export in the catalog (ExportRef = path to .mdemg file or stream reference). Optional: stream upload to hub storage.
- FR-5: `PullExport(DevSpaceId, ExportId)` — return export (file path, URL, or stream). Only allowed for registered agents (MVP: no auth, so any client can pull).
- FR-6: Hub can run as part of `space-transfer serve` (same process) or as separate `devspace-hub` binary; same proto.

### Non-functional

- NFR-1: All new RPCs have UDTS specs and runner coverage; proto_sha256 in manifest.
- NFR-2: New code in `internal/devspace/` and optional `cmd/devspace-hub/`; minimal changes to existing transfer code.

---

## Proto (new file)

- **File:** `api/proto/devspace.proto`
- **Package:** `mdemg.devspace.v1`
- **Services:**
  - `DevSpace`: `RegisterAgent`, `DeregisterAgent`, `ListExports`, `PublishExport`, `PullExport` (PullExport may stream SpaceChunk or return a URL/path; for MVP return metadata and client uses existing SpaceTransfer.Export or file path.)

MVP message shapes (minimal):

- `RegisterAgentRequest`: `dev_space_id`, `agent_id`, `metadata` (map)
- `RegisterAgentResponse`: `ok`
- `DeregisterAgentRequest`: `dev_space_id`, `agent_id`
- `ListExportsRequest`: `dev_space_id`
- `ListExportsResponse`: repeated `ExportEntry` (export_id, space_id, published_at_iso, published_by_agent_id, label)
- `PublishExportRequest`: `dev_space_id`, `space_id`, `label`, `export_ref` (e.g. path or inline chunk count)
- `PublishExportResponse`: `export_id`
- `PullExportRequest`: `dev_space_id`, `export_id`
- `PullExportResponse`: `export_ref` (path or URL) or stream of SpaceChunk (reuse SpaceTransfer)

For true out-of-band, hub must **store** or **reference** the export. MVP: hub stores export_id → path (server-local path where export was written); pull returns path or streams file. So client might call `PullExport` to get a path, then read file and import locally; or PullExport streams the file bytes.

Simpler MVP: **PublishExport** uploads the .mdemg file to hub (stream or path); hub writes to a directory. **PullExport** returns stream of bytes (the .mdemg file) or a path. So we need either:

- PullExport returns `file_path` (client must have filesystem access to hub — not great for remote)
- PullExport streams file bytes (opaque bytes)
- PullExport returns a URL (hub serves HTTP for that export) — then client fetches URL

Cleanest for remote: **PullExport** streams the export file bytes (or we reuse SpaceTransfer.Export by having hub proxy to a backend that has the space). For MVP: **PullExport** streams the .mdemg file content as bytes (one message or chunked). So:

- `PublishExport`: client streams .mdemg bytes to hub; hub saves to disk and catalogs.
- `PullExport`: client requests export_id; hub streams .mdemg bytes back.

Then we need `PublishExport(stream ExportFileChunk) returns (PublishExportResponse)` and `PullExport(PullExportRequest) returns (stream ExportFileChunk)`. Define `message ExportFileChunk { bytes data = 1; int64 sequence = 2; }` for raw bytes. Or reuse SpaceChunk if we want to keep .mdemg as structured chunks. Simpler: raw bytes for .mdemg file.

---

## UDTS

- Add `docs/api/api-spec/udts/specs/devspace_register_agent.udts.json`
- Add `docs/api/api-spec/udts/specs/devspace_list_exports.udts.json`
- Add `docs/api/api-spec/udts/specs/devspace_publish_export.udts.json` (optional for stream)
- Add `docs/api/api-spec/udts/specs/devspace_pull_export.udts.json`
- Runner: extend `tests/udts/` to load and run DevSpace specs when `UDTS_TARGET` points to hub.
- Proto hash: `api/proto/devspace.proto` in each spec's config.proto_sha256 after proto is stable.

---

## Implementation order

1. Add `api/proto/devspace.proto` (messages + DevSpace service).
2. Generate Go code; add to `api/devspacepb/` or under `api/`.
3. Implement `internal/devspace/catalog.go` (in-memory catalog: agents, exports).
4. Implement `internal/devspace/server.go` (gRPC server implementing DevSpace).
5. Wire hub into `cmd/space-transfer serve` (flag `-enable-devspace`) or new `cmd/devspace-hub`.
6. Add UDTS specs and tests.
7. Update manifest.sha256; mark Phase 2 complete in master plan.

---

## Acceptance criteria

- [ ] Proto defined; `protoc` generates Go; `go build ./...` passes.
- [ ] Hub serves DevSpace; RegisterAgent and ListExports work; PublishExport stores export; PullExport streams or returns export.
- [ ] UDTS specs for RegisterAgent, ListExports, PullExport (and PublishExport if unary); tests pass.
- [ ] Phase 2 marked complete in development-space-collaboration.md.
