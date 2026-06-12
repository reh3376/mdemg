# Phase 4: Incremental Sync

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Parent plan:** [development-space-collaboration.md](./development-space-collaboration.md)  
**Status:** Implementation in progress (proto + exporter + CLI done; UDTS pending)  
**Date:** 2026-01-22

---

## Goal

- **Delta transfer:** Export/import only changes since a given timestamp or since last sync cursor.
- Reduces payload and time for frequent syncs between agents or from hub.

---

## Requirements

### Functional

- FR-1: **Export** accepts an optional `since_timestamp` (ISO8601 or Unix ms) or `since_cursor` (opaque token from prior export response); returns only nodes/edges/observations/symbols **modified after** that point.
- FR-2: **Import** accepts the same conflict modes (skip/overwrite/error); applies deltas idempotently when the same cursor is re-applied.
- FR-3: Exporter filters by `updated_at` (or equivalent) on nodes/edges; cursor may encode last-seen timestamp or sequence id.
- FR-4: Optional: hub or client stores cursor per space (or per dev_space_id + space_id) for “sync from last” without client passing cursor.

### Non-functional

- NFR-1: New/updated RPCs have UDTS specs; proto_sha256 updated if proto changes.
- NFR-2: Changes in `internal/transfer/` (exporter/importer) and possibly `api/proto/space-transfer.proto` (extend ExportRequest/ExportResponse).

---

## Proto (to be added to space-transfer)

- **ExportRequest:** add optional `since_timestamp` (string) and/or `since_cursor` (string).
- **ExportResponse** (or last stream message): add optional `next_cursor` (string) for next delta call.
- Behavior: when `since_*` set, exporter returns only entities with `updated_at > since` (or cursor-equivalent); response includes `next_cursor` for next run.

---

## Implementation order (after Phase 3)

1. Extend `space-transfer.proto` with `since_timestamp`, `since_cursor`, `next_cursor`.
2. Exporter: filter nodes/edges/observations by `updated_at` when `since_*` provided; compute and return `next_cursor`.
3. Importer: unchanged conflict semantics; apply deltas (idempotent if same cursor re-applied).
4. UDTS: spec for Export with `since_timestamp`; test that result set is subset of full export.
5. Update manifest and Phase 4 acceptance in master plan.

---

## Dependencies

- Phase 1 (Space Transfer) complete. Phase 2/3 optional for “incremental sync via hub” (cursor could be stored per space on hub).

---

## Acceptance (when impl is done)

- [x] Export with `since_timestamp` or `since_cursor` returns only changed entities (exporter filters nodes/edges/observations/symbols by updated_at/created_at/timestamp; next_cursor in summary).
- [ ] Import applies deltas cleanly; re-applying same cursor is idempotent (unchanged conflict semantics).
- [ ] UDTS spec for Export with since_timestamp; tests pass.
- [ ] User or subagent verification of delta export/import.
