# State Persistence Across AI Context Resets

**Research Date**: 2026-03-21
**Purpose**: J17 protocol design — surviving context compaction and session boundaries
**Researcher**: Claude (Opus 4.6) via web search

---

## The Core Problem

Jiminy (persistent, server-side) must maintain effective guardrail enforcement over an AI coding agent (Claude) that undergoes context compaction every ~20-30 minutes. This is an **asymmetric state problem**: one party has infinite memory, the other has periodic amnesia.

---

## 1. Session Resumption Protocols

### TLS Session Tickets (RFC 5077 / TLS 1.3 RFC 8446)

TLS solves an almost identical problem: re-establishing a secure session after disconnection without repeating the full handshake.

**Session Tickets (Server-Side Stateless)**: Server encrypts session state into opaque "ticket" that client stores. On reconnection, client sends ticket back; server decrypts to recover full session state. Client never needs to understand ticket contents — it's an opaque blob echoed back.

**TLS 1.3 0-RTT**: After initial handshake, server and client agree on pre-shared key (PSK). On reconnection, client sends ticket in ClientHello and can immediately send encrypted data without waiting for a round trip.

**J17 applicability**: Jiminy issues an opaque **session ticket** at pre-compaction encoding current state (active constraints, conversation phase, escalation level). After compaction, hook injects ticket into new context. Jiminy decrypts/parses it to restore dialogue state. Agent never needs to understand ticket contents.

**0-RTT insight**: Jiminy supports "0-RTT guidance" — when agent sends ticket, Jiminy immediately returns guidance without full re-handshake.

- https://datatracker.ietf.org/doc/html/rfc5077
- https://blog.cloudflare.com/tls-session-resumption-full-speed-and-secure/

### WebSocket Reconnection with Sequence Numbers

**Monotonically increasing sequence numbers** track conversation state. On reconnect, client sends last received sequence ID, server replays everything after that point.

**SSE `Last-Event-ID`**: Built into protocol spec. Server assigns ID to each event. On reconnection, browser sends `Last-Event-ID` header, server replays missed events. Closest existing standard to J17 needs.

**J17 applicability**: Jiminy maintains **guidance event log** with monotonic sequence IDs. After compaction, agent sends last known sequence ID (from session ticket). Jiminy replays any guidance since that point. No guidance lost across compaction.

- https://websocket.org/guides/reconnection/
- https://javascript.info/server-sent-events

---

## 2. Stateless Protocol Design (REST/JWT Patterns)

### Self-Contained Requests

REST principle: every request carries all information needed. JWT encodes all session claims into a compact, signed token traveling with every request. Server stores nothing between requests.

**J17 applicability**: After compaction, the agent is a "new client." J17 uses a **JWT-like "context token"** encoding:
- Active constraint set
- Conversation phase
- Escalation state
- Trust level (compliant or drifting?)
- Sequence counter (monotonic, replay detection)

Injected into every prompt-context hook call. Each Jiminy request is fully self-contained.

**Critical design principle**: Token must be compact. Cap at fixed size (~1KB). Overflow stored in CMS, token serves as key/pointer.

- https://auth0.com/blog/stateless-auth-for-stateful-minds/
- https://blog.dreamfactory.com/stateless-vs-stateful-apis-key-differences/

---

## 3. Checkpoint/Snapshot Approaches

### LangGraph State Checkpointing

Saves graph state as checkpoints at every step, organized into threads. Full serialized state at each point. On resume, loads latest checkpoint and replays.

- `JsonPlusSerializer` for complex types
- Thread IDs organize logical conversation streams
- Time Travel: replay from any checkpoint
- Storage: InMemory (dev), Sqlite (local), Postgres (prod), Redis (low-latency)

- https://docs.langchain.com/oss/python/langgraph/persistence

### Google ADK Context Compaction

Sliding window: when history reaches threshold, older events summarized by LLM with **overlap** of previously compacted events for continuity.

**Key insight**: ADK separates **"Session State"** (key-value scratchpad) from **"Event History"** (chronological messages). State survives compaction; only history gets summarized. Jiminy's constraints and escalation = "state" (always preserved). Conversation narrative = "history" (can be summarized).

Performance: 60-80% token reduction while maintaining decision quality.

- https://google.github.io/adk-docs/context/compaction/
- https://google.github.io/adk-docs/sessions/

### MemGPT/Letta: Virtual Context Management

Applies **OS memory management** to LLM context:
- **Tier 1 (Main Context)**: Core memories actively in context window
- **Tier 2 (External)**: Recall/archival storage, paged in on demand

Agent manages memory via function calls — pages information in/out like OS paging between RAM and disk.

**J17 applicability**: Jiminy's guidance = **pinned Tier 1 memory block** that never gets paged out during compaction. Active constraints and session ticket are "pinned."

- https://arxiv.org/abs/2310.08560
- https://docs.letta.com/concepts/letta/

### A-Mem: Agentic Memory (NeurIPS 2025)

Self-organizing memory based on Zettelkasten principles. Memories stored as interlinked notes with structured attributes (descriptions, keywords, tags). Organization governed by agent itself.

- https://arxiv.org/abs/2502.12110

---

## 4. Contract/Handshake Protocols

### Google A2A Agent Cards

Every agent publishes JSON metadata at `/.well-known/agent.json` describing identity, capabilities, skills, auth. On connection, client fetches card to understand protocol.

**J17 applicability**: Jiminy publishes **Agent Card** defining capabilities, constraint types, escalation levels, protocol version. After compaction, session-start hook fetches card to re-establish protocol. Card is small, versioned, immutable between upgrades.

**Task State Lifecycle**: `submitted -> working -> input-required -> completed` maps to Jiminy enforcement: `monitoring -> warning-issued -> violation-detected -> escalation-pending -> resolved`.

- https://a2a-protocol.org/latest/specification/

### Proposed J17 Contract Structure

Compact JSON header (~500 bytes) re-injected after every compaction:

```json
{
  "protocol": "j17",
  "version": "1.0",
  "session_ticket": "<opaque_base64>",
  "last_seq": 47,
  "active_constraints": ["no-force-push", "test-before-commit"],
  "escalation_state": "clear",
  "trust_score": 0.92,
  "conversation_phase": "implementing-feature-X",
  "ttl": "2026-03-21T18:00:00Z"
}
```

---

## 5. Cognitive Architectures

### SOAR (State, Operator, And Result)

**Working Memory vs Long-Term Memory**:
- Working memory = agent's context window
- Semantic memory = Jiminy's CMS (general knowledge)
- Episodic memory = Jiminy's CMS (specific experiences)
- Procedural memory = Jiminy's constraints (production rules)

**Impasse Resolution**: When SOAR can't decide, it creates a substate to resolve. Maps to Jiminy detecting violation and creating guidance substate.

**Chunking**: When substate resolves, path compiled into production rules that fire automatically. Maps to Jiminy learning patterns and compiling into automatic constraint checks.

**J17 three-layer model from SOAR**:
1. **Automatic** (compiled rules): Constraints fire without reasoning — pre-bash-check.py
2. **Deliberative** (guidance): Full Jiminy reasoning when automatic rules insufficient
3. **Impasse protocol**: Agent escalates to Jiminy for novel situations, resolution compiled into new rule

- https://arxiv.org/pdf/2205.03854
- https://en.wikipedia.org/wiki/Soar_(cognitive_architecture)

### ACT-R Architecture

**Base-Level Activation**: Each memory chunk has activation that increases with use frequency and decreases with time (power-law decay: `A = ln(sum(t_i^-d))`). Below threshold = inaccessible.

**Spreading Activation**: Retrieved memories activate related memories. This IS MDEMG's Hebbian CO_ACTIVATED_WITH edges.

**J17 applicability**: Guidance uses activation-weighted retrieval. Frequently relevant constraints get priority in compact session ticket. Decayed constraints stored but not re-injected unless contextually triggered.

- ACT-R for LLM agents: https://dl.acm.org/doi/10.1145/3765766.3765803

### Blackboard Architecture

Shared knowledge space that multiple specialists read/write:
1. **Blackboard**: Current solution state
2. **Knowledge Sources**: Monitor and contribute partial solutions
3. **Control**: Determines which source to activate next

**J17 applicability**: Jiminy maintains blackboard (shared state). Both Jiminy and agent read/write. After compaction, agent reads blackboard to recover context. Structure is persistent, hierarchical, incrementally updated — survives compaction because it's server-side.

- https://en.wikipedia.org/wiki/Blackboard_system

---

## 6. Real-World: The "45-Minute Rule" Problem

### Instruction Centrifugation

AI coding agents **forget rules every ~45 minutes** due to context compression. System prompts get compressed away first because they're oldest tokens. Autoregressive models have strong recency bias — tokens near generation head exert disproportionate influence.

**Academic confirmation**: "When Refusals Fail" (AAAI 2026 TrustAgent) — models with 1M-2M context show **severe degradation at 100K tokens**, refusal rate drops exceeding 50%.

- https://dev.to/douglasrw/your-ai-agent-forgets-its-rules-every-45-minutes-heres-the-fix-151e
- https://arxiv.org/abs/2512.02445

### NVIDIA NeMo Guardrails Pattern

Runtime enforcement in execution loop. Guardrails are **programmatic rails** in Colang DSL, not prompt instructions. Input guardrails (validate before model) + output guardrails (check before returning).

**Key lesson**: Safety enforcement must NOT depend on model remembering instructions. Enforce programmatically.

- https://github.com/NVIDIA-NeMo/Guardrails

### External Enforcement > Better Prompts

"You would not give a contractor a list of security policies and hope they remember them. You would encode the policies in the system they use."

**DRIFT framework**: Dynamic Rule-based Isolation Framework. "Injection Isolator" polishes memory stream after each interaction, identifying and masking conflicting instructions.

- https://paircoder.ai/blog/enforcement-not-prompts/

### COLLAPSE.md Framework

Plain-text Markdown convention for detecting/preventing context exhaustion, model drift, coherence degradation. Defines summarization checkpoints, drift thresholds, recovery protocols.

- https://collapse.md/

### ACE: Agentic Context Engineering (NeurIPS)

Contexts as evolving playbooks. Three roles: Generator (produces strategies), Reflector (evaluates), Curator (maintains playbook). De-duplication removes redundancy. Addresses **brevity bias** and **context collapse**.

- https://arxiv.org/abs/2510.04618

### Event Sourcing / CQRS

All state changes as immutable events in append-only log. Current state derived by replaying. Snapshots optimize: periodically save full state, replay only events after snapshot.

**J17 applicability**: Jiminy maintains event-sourced guidance log. After compaction, agent needs latest snapshot + delta. Identical to TLS ticket + WebSocket sequence pattern applied to guidance domain.

---

## Recommended J17 Architecture (Three-Layer)

### Layer 1: Mechanical Enforcement (Already Exists)
Pre-bash-check.py, pre-compact.sh — fire regardless of context state. SOAR's "compiled production rules."

### Layer 2: Session Ticket + State Recovery (New for J17)
- Compact (~500 byte) signed session ticket at pre-compaction
- Encodes: active constraints, escalation state, trust score, sequence counter, phase
- Session-start hook injects ticket into prompt context
- Jiminy validates ticket, replays guidance events since last_seq
- TTL; expired tickets trigger full re-handshake

### Layer 3: Cognitive Guidance (Deliberative)
- Full Jiminy reasoning via `/v1/jiminy/guide`
- ACT-R activation weighting for constraint relevance
- Blackboard for shared state
- ACE playbook evolution for long-running tasks

### Design Principles

1. **External enforcement over prompt instructions** — never rely solely on agent remembering
2. **Compact state tokens over full history** — JWT/session ticket pattern
3. **Monotonic sequence counters** — gap detection (SSE Last-Event-ID)
4. **Separate state from history** — state always survives, history can be summarized
5. **Activation-weighted injection** — frequently relevant constraints get priority
6. **Snapshot + event replay** — snapshot at compaction, replay only delta
7. **Opaque ticket pattern** — agent carries ticket, doesn't need to understand it

---

*Documents Accessed: Web search results for TLS session resumption, WebSocket reconnection, LangGraph persistence, Google ADK, MemGPT, A2A protocol, SOAR, ACT-R, blackboard architecture, NeMo Guardrails, DRIFT framework, COLLAPSE.md, ACE, event sourcing.*
