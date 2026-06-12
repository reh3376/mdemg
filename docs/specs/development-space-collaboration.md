# Development Space Collaboration — Master Plan

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Master plan (Phases 1–8)
**Status**: Draft (Phase 2 complete ✅; Phases 3–8 in progress or planned)
**Author**: Agent (Cursor)
**Date**: 2026-02-06

---

## Overview

Enable **teams and agents** to share MDEMG spaces and collaborate via a **Development Space (DevSpace)**: out-of-band distribution of exports, inter-agent messaging, incremental sync, conflict-free replication for learned edges, space lineage, selective observation forwarding (CMS), and agent presence/heartbeat. All proto services and messaging are governed by **UDTS** (UPTS-type specs, runners, hash verification).

---

## Scope (Full Feature Set)

| # | Capability | Description | Phase |
|---|------------|-------------|-------|
| 1 | Import/Export & conflict handling | File + gRPC; skip/overwrite/error; profiles | **Phase 1** ✅ |
| 2 | Out-of-band distribution | Exports/imports distributed to registered members of a DevSpace (not just ad-hoc file share) | **Phase 2** ✅ complete |
| 3 | Inter-agent communications | Bidirectional messaging (context, bugs, issues) between agents in same DevSpace; gRPC streaming | **Phase 3** ✅ complete |
| 4 | Incremental sync | Transfer only deltas since last sync (timestamp or cursor) | Phase 4 |
| 5 | Conflict-free replication (CRDT) | CO_ACTIVATED_WITH edges merge with CRDT semantics | Phase 5 |
| 6 | Space lineage tracking | Audit trail: origin, merges, who shared what | Phase 6 |
| 7 | Selective observation forwarding | CMS: team-visible observations; forward selected obs into shared space | Phase 7 |
| 8 | Agent health / heartbeat / presence | Presence in DevSpace; heartbeat; optional offline queue (max_queue_size) | Phase 8 |
| — | **Hash Verification (UNTS)** | Current and historical hash verification for all framework-protected files (UPTS, UATS, UBTS, USTS, UOTS, UAMS, UDTS); status, last 3 hashes, revert; API/gRPC for monitoring and manipulation | **Governance** |

---

## Development Principles (Mandatory)

### Methodical and modular

- **One phase at a time.** No phase starts until the previous phase is **complete** (spec, impl, UDTS coverage, manifest hash).
- **New code in new packages.** Prefer `internal/devspace/`, `api/proto/devspace.proto`, `cmd/devspace-hub/`; avoid touching `internal/api/server.go` / `internal/config/config.go` for this feature set unless explicitly required.
- **Spec before impl.** For each phase: write or update the **phase spec** (this doc or linked sub-spec), then implement, then add UDTS specs and tests.

### UPTS-type governance (UDTS and related frameworks)

- **UDTS** — Use for all gRPC services (contract/integration tests). See [FRAMEWORK_GOVERNANCE.md](./FRAMEWORK_GOVERNANCE.md) for UBTS (benchmarking), USTS (security), UOTS (observability), and UAMS (auth) when those capabilities are added.

All gRPC/proto surface area must have:

1. **UDTS spec(s)** — JSON specs under `docs/api/api-spec/udts/specs/` for each RPC (or representative set). Schema: `docs/api/api-spec/udts/schema/udts.schema.json`.
2. **Runner / tests** — Go tests in `tests/udts/` (or equivalent) that load specs and assert; runnable with `UDTS_TARGET` when server is up.
3. **Hash verification** — `config.proto_sha256` in UDTS specs for the proto file(s) defining the service; runner verifies before running. On proto change, update hashes or accept intentional break.

When adding **benchmarking**, use **UBTS**; when adding **security** functionality, use **USTS**; when adding **observability**, use **UOTS**; when adding **auth**, use **UAMS**. See [FRAMEWORK_GOVERNANCE.md](./FRAMEWORK_GOVERNANCE.md).

**Hash Verification (UNTS / Nash Verification module):** Maintain current and historical record of hash verification for all mdemg files protected by hash verification (manifest, UDTS proto_sha256, and future UATS/UBTS/USTS/UOTS/UAMS/UPTS). Functionality: current hash-verified files with status and updated date; last 3 hash values with ability to revert to previous hash; API/gRPC for monitoring, observability, and manipulation. Spec: [unts-hash-verification.md](./unts-hash-verification.md).

Before a phase is marked **complete**:

- [ ] Phase spec updated and approved
- [ ] All new/changed RPCs have at least one UDTS spec
- [ ] UDTS runner/tests pass for that phase’s specs
- [ ] Proto (and spec doc if new) added to `docs/specs/manifest.sha256`
- [ ] `go build ./...` and `go test ./...` (excluding integration when not applicable) pass

---

## Phase 1: Space Transfer (complete)

- **Spec:** [space-transfer.md](./space-transfer.md)
- **Deliverables:** File + gRPC export/import, list/info, serve/pull, conflict handling (skip/overwrite/error), export profiles, progress, pre-export git check, UDTS (ListSpaces, SpaceInfo), unit + integration tests.
- **Status:** ✅ Complete. No further work in this phase.

---

## Phase 2: DevSpace hub and out-of-band distribution

### Goal

- A **DevSpace** is a named group (e.g. `my-team`) with **registered agents** (and optionally users).
- **Out-of-band distribution:** When an agent (or user) exports a space, the export can be **published** to the DevSpace hub; other **registered** members can **pull** or **list** available exports without ad-hoc file transfer.
- Optional: authentication (e.g. API key or token) for register/pull/publish.

### Deliverables

- Proto: `DevSpace` service (or extend `space-transfer.proto`): `RegisterAgent`, `DeregisterAgent`, `ListExports`, `PublishExport`, `PullExport` (or reuse SpaceTransfer.Export with auth).
- Hub implementation: `cmd/devspace-hub/` or extend `cmd/space-transfer serve` with DevSpace registration and export catalog.
- Distribution: exports stored by hub (or referenced by path/URL); pull by registered members only.
- UDTS: specs for `RegisterAgent`, `ListExports`, `PullExport` (or equivalent); runner/tests; proto hash.
- Docs: sub-spec `docs/specs/phase-devspace-hub.md` (optional) or section in this doc.

### Dependencies

- Phase 1 (Space Transfer) complete.
- Neo4j optional for hub (catalog can be in-memory or file-based for MVP).

### Acceptance

- [x] Proto defined and generated; UDTS specs added; hashes updated.
- [x] Hub serves SpaceTransfer + DevSpace registration (`space-transfer serve -enable-devspace`); registered agent can publish and another can pull.
- [x] All new RPCs covered by UDTS; tests in `tests/udts/` (run with `UDTS_TARGET=localhost:50051` and server with `-enable-devspace`).
- [x] **User verification:** Run `space-transfer serve -enable-devspace` and UDTS contract tests; confirm publish/pull flow. ✅ Complete 2026-01-22.

---

## Phase 3: Inter-agent communications

### Goal

- **Bidirectional messaging** between agents in the same DevSpace: share context, report bugs, notify issues in near real time via gRPC streaming.
- **Communicate framework:** e.g. `Connect(stream)` or `SendMessage` / `Subscribe` so agents can push and receive messages within the DevSpace.

### Deliverables

- Proto: `DevSpaceMessaging` or extend DevSpace with `Connect(stream AgentMessage) returns (stream AgentMessage)`, or `Subscribe` + `Publish`.
- Implementation: message broker (in-process or minimal external); route by DevSpace + optional topic.
- UDTS: specs for Connect/Subscribe/Publish (or equivalent); runner/tests; proto hash.
- Optional: offline queue (max_queue_size) for disconnected agents.

### Dependencies

- Phase 2 (hub + registration) so that “agents in same DevSpace” is well-defined.

### Acceptance

- [x] Proto and impl (Connect + AgentMessage; broker in internal/devspace); at least two agents can exchange messages via hub.
- [x] UDTS coverage (devspace_connect.udts.json + TestDevSpaceConnect); tests in tests/udts.
- [x] Verification: in-process integration test + UDTS (TestDevSpaceConnect) pass; server on port 50052 exercised.

---

## Phase 4: Incremental sync

### Goal

- **Delta transfer:** export/import only changes since a given timestamp or since last sync cursor.
- Reduces payload and time for frequent syncs.

### Deliverables

- Proto: extend `ExportRequest` with `since_timestamp` or `since_cursor`; export only nodes/edges/observations modified after.
- Exporter: filter by `updated_at` or equivalent; optional cursor stored by hub or client.
- Importer: merge deltas (same conflict modes); idempotent when re-applying same cursor.
- UDTS: spec for Export with `since_*`; test that delta export returns subset.

### Dependencies

- Phase 1. (Phase 2/3 optional for “incremental sync via hub”.)

### Acceptance

- [ ] Export with `since_timestamp` returns only changed entities; import applies cleanly.
- [ ] UDTS + tests pass.

---

## Phase 5: CRDT for learned edges + Space lineage

### Goal

- **CRDT:** CO_ACTIVATED_WITH edges merge without overwriting; use something like last-writer-wins or merge of weights/evidence_count so that concurrent updates from multiple agents don’t lose data.
- **Space lineage:** Each space (or export) can carry lineage metadata: origin space_id, export timestamps, who published; optional merge history.

### Deliverables

- CRDT: define merge rules for CO_ACTIVATED_WITH (e.g. max weight, sum evidence_count); implement in importer when conflict mode is “merge” or new “crdt” mode.
- Lineage: proto fields for lineage (e.g. `SpaceMetadata.lineage`); exporter records origin; importer can append to lineage on merge.
- UDTS: no new RPCs required; extend existing Export/Import specs or add integration test for CRDT merge and lineage round-trip.

### Dependencies

- Phase 1. Phase 4 helpful for delta + CRDT together.

### Acceptance

- [ ] CRDT merge behavior documented and implemented; tests.
- [ ] Lineage present in export and import; UDTS/manifest updated if proto changes.

---

## Phase 6: (Covered in Phase 5)

Space lineage is included in Phase 5 above.

---

## Phase 7: Selective observation forwarding (CMS)

### Goal

- **Selective observation forwarding:** Agents can mark observations as “team-visible” or forward selected observations into a **shared** space or DevSpace feed so other agents see them without full space sync.
- Integration with existing CMS (observe/recall) and optional new endpoint or extension.

### Deliverables

- Proto: e.g. `ForwardObservation` or extend CMS observe with `visibility: team` and DevSpace target.
- Implementation: store or route observations to DevSpace feed; recall can filter by visibility.
- UDTS: spec for new/updated RPCs; tests.

### Dependencies

- Phase 2 (DevSpace) and existing CMS.

### Acceptance

- [ ] Observations can be forwarded to DevSpace; other agents can recall or receive them.
- [ ] UDTS + tests pass.

---

## Phase 8: Agent health / heartbeat / presence

### Goal

- **Presence:** Agents registered in a DevSpace appear as “online” or “away” based on heartbeat.
- **Heartbeat:** Periodic ping from agent to hub; hub tracks last_seen; optional timeout for “away”.
- **Offline queue:** If an agent is disconnected, messages (or export notifications) can be queued up to `max_queue_size` and delivered when agent reconnects.

### Deliverables

- Proto: `Heartbeat`, `GetPresence` (or `ListAgents` with status); optional `SetQueueSize`.
- Implementation: hub stores last_heartbeat per agent; presence endpoint; optional bounded queue per agent.
- UDTS: specs for Heartbeat, GetPresence; tests.

### Dependencies

- Phase 2 (registration).

### Acceptance

- [x] Heartbeat updates last_seen; GetPresence returns agent list with status.
- [x] Optional queue: messages queued when agent offline; delivered on Connect.
- [x] UDTS + tests pass.
- [x] **100% function coverage** for all Phase 8 functions (39 unit tests).

---

## Governance Checklist (per phase)

Before marking any phase **complete**:

1. [ ] Phase spec (this doc or sub-spec) updated and accurate.
2. [ ] All new/changed proto files have UDTS specs for affected RPCs.
3. [ ] UDTS runner/tests pass (`UDTS_TARGET` set or CI).
4. [ ] Proto and spec doc hashes in `docs/specs/manifest.sha256`.
5. [ ] `go build ./...` and `go test ./...` pass (modulo known skips).
6. [ ] Development was methodical (spec → impl → tests) and modular (new code in new packages where possible).

---

## File and directory layout (target)

```
api/proto/
  space-transfer.proto   # existing
  devspace.proto         # Phase 2+: DevSpace, messaging, presence (or extend space-transfer)
docs/api/api-spec/udts/
  schema/udts.schema.json
  specs/
    space_transfer_*.udts.json
    devspace_*.udts.json   # Phase 2+
internal/
  transfer/              # existing
  devspace/              # Phase 2+: hub logic, registration, catalog
cmd/
  space-transfer/        # existing
  devspace-hub/          # Phase 2: optional dedicated hub binary (or extend space-transfer serve)
tests/
  udts/                  # existing; add specs for new RPCs
docs/specs/
  space-transfer.md
  development-space-collaboration.md  # this file
```

---

## Status summary

| Phase | Name | Status |
|-------|------|--------|
| 1 | Space Transfer | ✅ Complete |
| 2 | DevSpace hub + out-of-band distribution | ✅ Complete |
| 3 | Inter-agent communications | ✅ Complete |
| 4 | Incremental sync | ✅ Complete |
| 5 | CRDT + Space lineage | ✅ Complete |
| 7 | Selective observation forwarding | 📋 Not started |
| 8 | Agent health / heartbeat / presence | ✅ Complete |
| — | Hash Verification (UNTS) | ✅ Complete |

---

## Dependencies

- Depends on: Neo4j (for Space Transfer and optional hub state), existing CMS for Phase 7.
- Blocks: None (additive).

## References

- [space-transfer.md](./space-transfer.md) — Phase 1 spec (complete).
- [unts-hash-verification.md](./unts-hash-verification.md) — UNTS (Hash Verification / Nash Verification module) spec.
- [FRAMEWORK_GOVERNANCE.md](./FRAMEWORK_GOVERNANCE.md) — UNTS, UDTS, UBTS, USTS, UOTS, UAMS.
- [UDTS README](../api/api-spec/udts/README.md) — UDTS schema, specs, runner.
- [UPTS README](../lang-parser/lang-parse-spec/upts/README.md) — Governance model for parsers (same idea for proto).
