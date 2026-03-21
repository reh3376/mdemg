# AI-to-AI Communication Protocols: Existing Landscape Survey

**Research Date**: 2026-03-21
**Purpose**: J17 protocol design — survey existing AI agent communication protocols
**Researcher**: Claude (Opus 4.6) via web search

---

## 1. Classical Agent Communication (1990s-2000s)

### KQML (Knowledge Query and Manipulation Language)

Developed as part of the DARPA Knowledge Sharing Effort (early 1990s). Defined a message format using "performatives" (speech acts): `tell`, `ask-one`, `ask-all`, `advertise`, `subscribe`. Three-layer structure: content layer (knowledge), message layer (performative + metadata), communication layer (transport).

**What worked**: Layered separation of concerns was prescient. Wrapping arbitrary content in a standardized performative envelope influenced every subsequent protocol.

**What failed**: Multiple incompatible implementations (dialects). No formal semantics — performative meaning was underspecified, causing interoperability failures.

- https://en.wikipedia.org/wiki/Knowledge_Query_and_Manipulation_Language
- https://www.cs.umbc.edu/research/kqml/

### FIPA ACL (Foundation for Intelligent Physical Agents)

Founded 1996 (Swiss non-profit, later IEEE 2005). Refined KQML with formal semantics based on BDI (Beliefs, Desires, Intentions). Defined 22 communicative acts: `inform`, `request`, `propose`, `accept-proposal`, `reject-proposal`, `failure`, `not-understood`, `confirm`, `cancel`, etc.

**What worked**:
- Complete standard stack (message structure, interaction protocols, agent management, directory services, transport)
- Speech act theory foundation
- Real implementations (JADE, FIPA-OS)
- Reusable interaction protocol patterns (Contract Net, Auctions)

**What failed (critically important for J17)**:
- **Mentalistic semantics are unverifiable.** Defines meaning via agents' private mental states (beliefs/intentions). Cannot verify from outside whether an agent believes what it `inform`s. Compliance testing impossible.
- **Sincerity assumption breaks for negotiation.** Assumes agents believe what they say.
- **No commercial adoption** despite IEEE backing.
- **Shared ontology requirement** — prohibitively complex in heterogeneous systems.
- **Too heavyweight** — full platform infrastructure most systems didn't need.

**J17 lesson**: Protocol semantics must be based on **observable behavior**, not private internal states. For Jiminy communicating with agents that undergo context resets, the only reliable semantics are grounded in verifiable external state — exactly what Neo4j provides.

- http://www.fipa.org/repository/aclspecs.html
- Criticism: https://link.springer.com/article/10.1023/A:1010016503852

---

## 2. Modern LLM Agent-to-Agent Protocols (2024-2026)

### Protocol Landscape Overview

| Protocol | Architecture | Discovery | Transport | Primary Use Case |
|----------|-------------|-----------|-----------|-----------------|
| MCP | Client-Server | Manual/Static | JSON-RPC, stdio, SSE | LLM tool augmentation |
| ACP | Brokered Registry | Registry-based | REST + streaming | Infrastructure agents |
| A2A | Peer-to-Peer | Agent Cards | JSON-RPC over HTTPS | Enterprise task delegation |
| ANP | Decentralized P2P | Search engine indexing | HTTPS + JSON-LD | Cross-platform marketplaces |
| LACP | Three-layer | TBD | HTTP/2, QUIC, WebSockets | Telecom/NextG networks |
| Agora | Meta-protocol | Protocol Documents | HTTPS + JSON | Scalable LLM networks |
| AMP | Federated | Provider-based | Ed25519-signed messages | Secure messaging |
| REP | Sensitivity-sharing | Inherited from transport | Structured JSON | Population coordination |

- Survey paper: https://arxiv.org/html/2505.02279v1
- Protocol comparison: https://arxiv.org/html/2504.16736

### LACP (LLM Agent Communication Protocol) — NeurIPS 2025

Telecom-inspired three-layer design:

1. **Semantic Layer**: Three universal message types — `PLAN` (express intent), `ACT` (invoke tools), `OBSERVE` (return results). "Narrow waist" principle.
2. **Transactional Layer**: JWS (JSON Web Signature) envelope. Cryptographic signing, sequencing, transaction IDs for idempotency, duplicate detection.
3. **Transport Layer**: Transport-agnostic (HTTP/2, QUIC, WebSockets).

Performance: ~3% latency overhead, 30% size overhead for large messages (acceptable for cryptographic security). Defeats tampering and replay attacks.

- Paper: https://arxiv.org/html/2510.13821v1
- GitHub: https://github.com/LiXin97/LACP

**J17 relevance**: `PLAN/ACT/OBSERVE` triple maps directly to Jiminy's guidance loop. JWS envelope ensures integrity across context boundaries. Transaction IDs prevent duplicate processing after context compaction.

### Agora Protocol — Oxford, 2024

Addresses the "Agent Communication Trilemma" (versatility vs. efficiency vs. portability) with three tiers:

1. **Standardized routines** for frequent communications (cheapest)
2. **LLM-written routines** for moderately frequent interactions (JSON schemas from Protocol Documents)
3. **Natural language** fallback for rare/novel exchanges

Protocol Documents (PDs) identified by SHA1 hash. When agents lack a common protocol, they negotiate one in natural language, then the LLM generates a structured implementation. **100-agent demo: 5x cost reduction** vs natural-language-only ($7.67 vs $36.23 for 1000 queries).

- Paper: https://arxiv.org/html/2410.11905v1
- Website: https://agoraprotocol.org/

**J17 relevance**: Tiered approach directly applicable. Jiminy's frequent guidance (constraints, corrections) should use compact structured format. Novel/complex guidance falls back to natural language.

### Ripple Effect Protocol (REP) — MIT, 2025

Goes beyond decision-sharing to **sensitivity-sharing**: agents broadcast not just what they decided, but how decisions would change if key variables shifted. Messages contain `(decision, sensitivity)` pairs where sensitivities are natural language counterfactuals: "If demand increases 10%, increase order by 15 units."

Performance: 41-100% improvement over A2A-style baselines across supply chains, resource allocation, consensus tasks. Supported by Anthropic and MIT.

- Paper: https://arxiv.org/html/2510.16572v1

**J17 relevance**: Communicating *sensitivities* rather than absolute rules. Instead of "don't do X," Jiminy communicates "if Y changes, then Z guidance applies" — conditional guidance that survives context resets because conditions are self-documenting.

### Emergent Machine Language Between LLM Agents (2025)

LLM agents developed their own non-human-interpretable communication protocol through referential games. Two agents developed a shared language for 541 objects through only 4 rounds of communication (max 3 attempts each).

Demonstrates that LLMs do not need natural language to communicate efficiently — they can develop compact, task-optimized codes.

- Paper: https://openreview.net/forum?id=zy06mHNoO2
- Related: https://aclanthology.org/2025.coling-main.667.pdf

---

## 3. Google A2A (Agent-to-Agent) Protocol

Announced April 2025, now under Linux Foundation with 150+ partners.

### Core Architecture
- **Transport**: HTTPS with JSON-RPC 2.0 (v0.3 added gRPC)
- **Discovery**: Agent Cards — JSON metadata at `/.well-known/agent.json`
- **Interaction modes**: Synchronous, streaming (SSE), async push notifications

### Task Lifecycle
`submitted` -> `working` -> `input-required` -> `completed` / `failed` / `canceled`

Tasks contain Messages (utterances), Parts (content units: text, files, JSON), and Artifacts (output deliverables).

### Key Methods
- `tasks/send`, `tasks/sendSubscribe`, `tasks/get`, `tasks/cancel`
- `tasks/pushNotification/set`, `tasks/pushNotification/get`

### What A2A Does NOT Define
- No internal agent architecture requirements (agents are opaque)
- No shared memory or state access
- No tool definitions (that's MCP's domain)
- No protocol negotiation beyond Agent Cards

- GitHub: https://github.com/a2aproject/A2A
- Spec: https://a2a-protocol.org/latest/specification/

---

## 4. MCP vs A2A — Complementary, Not Competing

**MCP is vertical (agent-to-tools); A2A is horizontal (agent-to-agent).**

| Dimension | MCP | A2A |
|-----------|-----|-----|
| Relationship | Host-to-tool (vertical) | Agent-to-agent (horizontal) |
| Architecture | Client-server (1:1) | Peer-to-peer |
| Discovery | Manual configuration | Agent Cards |
| State | Stateful sessions | Task-based lifecycle |
| Opacity | Server internals exposed via tools | Agents are opaque |
| Primary value | Give LLM access to capabilities | Let agents delegate work |

Real systems use both: MCP internally (agent to databases/APIs), A2A externally (agent to agent).

AWS has extended MCP for inter-agent communication by modeling other agents as MCP tools.

---

## 5. Open Source Projects

### Production-Ready
| Project | GitHub | Focus |
|---------|--------|-------|
| A2A | https://github.com/a2aproject/A2A | Peer-to-peer agent tasks (SDKs: Python, Go, JS, Java, .NET) |
| MCP SDKs | https://github.com/modelcontextprotocol/python-sdk | Agent-to-tool |
| ANP | https://github.com/agent-network-protocol/AgentNetworkProtocol | Decentralized agent network |
| AMP | https://github.com/agentmessaging/protocol/ | Federated secure messaging |

### Research/Emerging
| Project | GitHub | Focus |
|---------|--------|-------|
| LACP | https://github.com/LiXin97/LACP | Telecom-inspired three-layer |
| AIXP | https://github.com/davila7/AIXP | Compact agent exchange with loop prevention |
| ANEX | https://github.com/ammonhaggerty/ANEX | Agent negotiation and exchange |
| Agent Protocol | https://github.com/agi-inc/agent-protocol | Common agent interface |

---

## Key Takeaways for J17 Design

1. **Tiered communication** (Agora): Compact structured for frequent guidance, NL fallback for novel cases. 5x cost reduction proven.
2. **PLAN/ACT/OBSERVE triple** (LACP): Maps directly to Jiminy's guidance loop with transaction envelopes for idempotency.
3. **Sensitivity signals** (REP): Conditional guidance ("if X then Y") survives context resets better than absolute rules.
4. **Observable semantics** (FIPA's lesson): Ground protocol in verifiable external state (Neo4j), not agent internal state.
5. **Agent Cards** (A2A): Capability declaration for session bootstrap after context reset.
6. **Emergent compression**: LLMs can develop compact task-optimized codes — protocol negotiation is feasible.
7. **Transaction IDs** (LACP): Prevent duplicate processing after context compaction re-delivery.

---

*Documents Accessed: Web search results for FIPA ACL, KQML, Google A2A, Agora Protocol, LACP, REP, MCP, ANP, AMP, AIXP, emergent machine language papers.*
