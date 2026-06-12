# Phase 3: Inter-Agent Communications

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Parent plan:** [development-space-collaboration.md](./development-space-collaboration.md)  
**Status:** ✅ Complete (implementation + in-process test + Phase 3 verification 2026-01-22)  
**Date:** 2026-01-22

---

## Goal

- **Bidirectional messaging** between agents in the same DevSpace: share context, report bugs, notify issues in near real time via gRPC streaming.
- **Communicate framework:** Agents connect to the hub and exchange messages within a DevSpace (and optional topic/channel).

---

## Requirements

### Functional

- FR-1: Hub exposes a **messaging** surface: agents in the same `dev_space_id` can send and receive messages.
- FR-2: **Connect** (or equivalent): agent establishes a session; sends and receives a stream of messages (bidirectional stream or separate Subscribe + Publish).
- FR-3: Messages are **routed** by `dev_space_id`; optional `topic` or `channel` for filtering.
- FR-4: Only **registered** agents (via Phase 2 `RegisterAgent`) are allowed to send/receive in that DevSpace (MVP: same process; auth can be Phase 3b).
- FR-5: Optional: **offline queue** (e.g. `max_queue_size` per agent) for when an agent reconnects.

### Non-functional

- NFR-1: New RPCs have UDTS specs and runner coverage; proto_sha256 when proto is stable.
- NFR-2: New code in `internal/devspace/` (e.g. `messaging.go`, broker); extend `api/proto/devspace.proto` or new `devspace-messaging.proto`.

---

## Proto (to be added when Phase 2 verification is complete)

**Chosen: Option A — extend DevSpace in `api/proto/devspace.proto`.**

Add to `service DevSpace { ... }`:

```protobuf
  // Connect opens a bidirectional stream for inter-agent messaging (Phase 3).
  rpc Connect(stream AgentMessage) returns (stream AgentMessage);
```

Add new messages (e.g. at end of file):

```protobuf
// =============================================================================
// Inter-agent messaging (Phase 3)
// =============================================================================

message AgentMessage {
  string dev_space_id = 1;   // Required: target DevSpace
  string agent_id = 2;       // Sender agent (must be registered)
  string topic = 3;          // Optional: channel/topic for filtering (e.g. "bugs", "context")
  string payload_type = 4;   // Optional: e.g. "application/json", "text/plain"
  bytes payload = 5;
  int64 sequence = 6;        // Server-assigned order for delivery
}
```

MVP: One bidirectional stream per agent; server routes by `dev_space_id` and optional `topic`; only registered agents can send/receive.

---

## Implementation order (when Phase 2 verification is complete)

1. Add `Connect` and `AgentMessage` to `devspace.proto`; regenerate Go.
2. Implement in-memory broker in `internal/devspace/`: map `dev_space_id` (+ topic) → list of connected streams; broadcast or fan-out.
3. Implement `Connect` handler in `internal/devspace/server.go`: accept stream, register with broker, forward incoming messages to other agents in same DevSpace, stream back messages for this agent.
4. Wire broker into `NewServer` (catalog + broker).
5. UDTS: spec for `Connect` (e.g. connect and receive 0 or more messages; or send one and receive echo). Tests in `tests/udts/`.
6. Update manifest and Phase 3 acceptance in master plan.

---

## UDTS

- Spec(s): e.g. `devspace_connect.udts.json` (bidirectional stream; expectation may be “OK connection” or “receive N messages”).
- Runner: extend `tests/udts/contract_test.go` or add `TestDevSpaceConnect` (may require goroutine to receive stream).
- Proto hash: update after proto changes.

---

## Dependencies

- Phase 2 (hub + registration) **verified complete** so that “agents in same DevSpace” is well-defined and hub is running.

---

## Acceptance (to be checked when impl is done)

- [x] Proto defined and generated; UDTS spec(s) added.
- [x] At least two agents can exchange messages via hub (same dev_space_id); automated in `TestDevSpaceTwoAgentMessaging`.
- [x] UDTS / tests pass.
- [x] In-process integration tests: `TestDevSpaceServerInProcess`, `TestDevSpaceTwoAgentMessaging`.
