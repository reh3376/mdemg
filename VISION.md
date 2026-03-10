# MDEMG Vision & Architecture

**Multi-Dimensional Emergent Memory Graph**

*A cognitive substrate for AI-assisted development*

---

## Executive Summary

MDEMG is an **emergent long-term memory system** designed to serve as the cognitive foundation for AI coding agents and multi-agent development workflows. Unlike static knowledge bases, MDEMG is a living system where higher-level concepts and relationships **emerge automatically** from accumulated observations through Hebbian learning principles.

---

## The Internal Dialog Analogy

### What MDEMG Is

MDEMG provides coding AI agents and sub-agents with what could be described as the **ANN (Artificial Neural Network) equivalent of an internal dialog**—similar to the experience humans have with biological neural networks.

When humans think through problems, they draw on:

- Past experiences and how they handled similar situations
- Domain expertise accumulated over years of specialization
- Relationships between concepts that aren't universally known
- The specific context of their work environment

MDEMG gives AI agents this same capability—a persistent "inner voice" of accumulated domain knowledge.

### What MDEMG Does NOT Store

**MDEMG does not store information and concepts that AI agents already possess.** Large language models already have extensive general knowledge—programming languages, algorithms, frameworks, best practices, etc. Storing this in MDEMG would be redundant and wasteful.

### What MDEMG DOES Store

MDEMG holds **specific and relevant information** related to:

1. **Tasks Performed** - What the agent has done, decisions made, problems solved
2. **Subject Matter Expert (SME) Knowledge** - Specialized, domain-specific expertise

### SME Knowledge Examples

Consider a software engineering team at "Whiskey House" (hypothetical industrial company):

| Role | SME Knowledge in MDEMG |
|------|------------------------|
| **Software Engineer** | Whiskey House codebase conventions, deployment procedures, the quirks of their legacy systems, which APIs are deprecated, tribal knowledge about why certain architectural decisions were made |
| **Process/Controls Engineer** | P&ID relationships (what valve connects to which tank), PLC program specifics (the logic behind specific rungs), process control automation team goals, safety interlock sequences |

This knowledge is:

- **Not universally available** - You can't Google "Whiskey House Tank 5 level control logic"
- **Highly contextual** - Makes sense only within the organization's context
- **Accumulated over time** - Built up through experience and interaction
- **Organizationally valuable** - Represents institutional knowledge that would otherwise be lost

### TapRoot and Concept Layers

The architecture reflects this purpose:

```
Concept Layers (n2, n3, n4...)
    ↑ Increasingly abstract relationships
    ↑ Emergent patterns and principles
    ↑ Cross-domain connections

n1_root (first concept layer)
    ↑ Patterns emerging from observations

n0_root0 (TapRoot level)
    ↑ Domain-specific SME knowledge
    ↑ Task execution history
    ↑ Specific procedural knowledge

[Raw observations from agent work]
```

The **TapRoot level** stores the concrete, domain-specific knowledge—the "institutional memory" of an organization. The **concept layers** above hold increasingly abstract relationships that emerge as the system learns patterns across observations.

---

## Core Purpose

### Primary Functions

1. **Long-Term Memory for AI Coding Agents**
   - Persistent context that survives across sessions
   - Code patterns, solutions, and architectural decisions
   - Project-specific knowledge that improves agent effectiveness

2. **Multi-Agent Coordination Layer**
   - Shared memory substrate for agent collaboration
   - Prevents redundant work across agents
   - Enables knowledge transfer between specialized agents

3. **Agent Consulting Service**
   - Proactively provides context-relevant suggestions
   - Subject matter expertise synthesized from accumulated knowledge
   - Process-specific guidance based on learned workflows
   - Higher-level concepts surfaced as they emerge

---

## Architectural Philosophy

### The Emergence Principle

> "The system must be highly dynamic with the ability to reorder its nodes as new information causes unanticipated changes to the underlying data structures. Edges will not likely change, but the path to nodes will."

This captures the key insight: **relationships are stable, but the conceptual organization is fluid**. Just as human memory reorganizes concepts as understanding deepens, MDEMG allows nodes to migrate through layers while preserving their relational connections.

### Modular Intelligence Architecture

MDEMG is evolving from a unified memory store into a **Modular Intelligence Engine**. Specific "Modules" can be plugged into the graph to grant it specialized perception and reasoning skills:

- **SME Ingestion Modules**: Domain-specific parsers (PLC, P&ID, Linear, Obsidian).
- **Reasoning Modules**: Specialized architectural pattern detectors (NestJS, Go-Micro) and **sophisticated re-ranking logic (LLM Re-ranker)**.
- **Active Participant Modules**: Proactive reflection, consistency checking, and explainable retrieval.

### Benchmarking & Performance Validation

MDEMG's effectiveness is continuously validated through rigorous benchmarking against real-world industrial codebases. The system has demonstrated a **0.898 mean retrieval score** with **100% high-score rate** and **100% strong evidence rate** on the whk-wms benchmark (507K LOC TypeScript, 120 questions). For the complete performance trajectory, see the [Up-to-Date Benchmark Summary](docs/tests/UP_TO_DATE_BENCHMARK_SUMMARY.md).

### Public Repository Standards

To facilitate global collaboration while maintaining the core system's integrity, MDEMG follows strict public-readiness standards. This ensures that the engine is secure, the architecture is extensible for contributors, and the "Internal Dialog" remains a reliable substrate for all AI agents. (See [Repo-to-Public Roadmap](docs/repo-to-public-roadmap.md) for the full strategy).

#### Integration with Retrieval Pipeline

Modules integrate with the retrieval pipeline at three critical points:

1. **Candidate Selection**: Perception modules tag nodes during ingest, allowing filtered recall.
2. **Reasoning (Re-ranking)**: Reasoning modules (like the v9 LLM re-ranker) process raw retrieval results to refine the final top-K list.
3. **Explanation**: The explainable retrieval layer traces which module influenced a node's score to provide a human-readable `rationale`.

### Multi-Dimensional Layered Graph

```
Layer N   [Principles / Axioms]           ← Most abstract
    ↑     Emerges from patterns in Layer N-1
Layer 3   [Concepts / Abstractions]
    ↑     Emerges from patterns in Layer 2
Layer 2   [Patterns / Regularities]
    ↑     Emerges from patterns in Layer 1
Layer 1   [Observations / Events]         ← Most concrete
    ↑
[Raw Input: code, decisions, conversations]
```

**Layer constraints:**

- **Minimum**: 1 (raw observations only)
- **Maximum**: Unconstrained (hardware-limited only)
- **Growth**: Dynamic - layers emerge as data density warrants

### Promotion Mechanics

Nodes are promoted to higher layers based on a **combination of signals**:

| Signal | Description |
|--------|-------------|
| **Frequency** | Pattern appears across multiple contexts |
| **Clustering** | Multiple L(n) nodes form stable associations |
| **Edge Strength** | CO_ACTIVATED_WITH edges exceed threshold |
| **Temporal Stability** | Pattern persists over time, not transient |
| **Cross-Domain Relevance** | Pattern applies across different projects/contexts |

### Dynamic Reorganization

Unlike traditional databases where structure is fixed:

- **Edges remain stable** - Once a relationship is learned, it persists
- **Node positions are fluid** - Concepts can move between layers
- **Paths adapt** - The route to reach a concept changes as organization evolves
- **No manual maintenance** - Reorganization happens automatically

---

## Integration Modes

MDEMG operates as a **full active participant** in the development workflow:

### 1. Background Service

- Always running, similar to claude-mem
- API available for agent queries
- Continuous learning from observations

### 2. Event-Driven Hooks

- Git commits trigger memory updates
- File saves capture context
- Session events (start/end) trigger reflection

### 3. Proactive Surfacing

| Mode | Behavior |
|------|----------|
| **Context Suggestions** | When working on code, surface related patterns/decisions |
| **Periodic Reflection** | Synthesize insights at session start/end |
| **Anomaly Detection** | Alert when current work contradicts stored knowledge |
| **Conflict Resolution** | Identify when new info conflicts with existing beliefs |

### 4. Agent Consulting Service

A higher-order capability where MDEMG acts as an **SME (Subject Matter Expert)** for coding agents:

- **Context provision**: "Based on this codebase's patterns..."
- **Process guidance**: "The typical workflow for this type of change is..."
- **Concept synthesis**: "This relates to the higher-level principle of..."
- **Risk awareness**: "Previous attempts at this approach encountered..."

---

## What MDEMG Stores

> **Important:** MDEMG stores only domain-specific, organization-specific, and task-specific knowledge. It does NOT duplicate general knowledge that LLMs already possess.

### Content Types (Domain-Specific SME Knowledge)

| Category | Examples | Why It Belongs in MDEMG |
|----------|----------|-------------------------|
| **Organizational Code Patterns** | "We always use Repository pattern for data access in this codebase" | Specific to your organization, not universal |
| **Architectural Decisions & Rationale** | "We chose Redis over Memcached because of X incident in 2024" | Institutional knowledge, would be lost otherwise |
| **Domain-Specific Procedures** | P&ID sequences, PLC logic explanations, safety interlock documentation | Highly specialized, not available anywhere else |
| **Project Context** | Which APIs are deprecated, why certain workarounds exist | Tribal knowledge accumulated over time |
| **Historical Problem/Solution Pairs** | "Last time we saw this error, the root cause was X" | Organization-specific debugging history |
| **Team Conventions** | PR review expectations, deployment checklists, on-call procedures | Process knowledge unique to this team |
| **Cross-Project Learnings** | "This pattern from Project A also worked well in Project B" | Connections that only exist within this organization |

### What NOT to Store

| Do NOT Store | Reason |
|--------------|--------|
| Python syntax | LLM already knows this |
| How React hooks work | Universally available documentation |
| General best practices | Already in training data |
| Standard library APIs | LLM has this knowledge |
| Common design patterns | Well-documented elsewhere |

**Rule of thumb:** If you could find it on Stack Overflow or in official documentation, it probably doesn't belong in MDEMG.

### Observation Sources

- Claude Code conversations (capturing context and decisions)
- Git commits and diffs (what changed and why)
- Code reviews and PR discussions (institutional feedback)
- Documentation and comments (domain-specific explanations)
- Error logs and debugging sessions (organizational problem-solving)
- Explicit user annotations (deliberate knowledge capture)

---

## Differentiation from Claude-Mem

| Aspect | claude-mem | MDEMG |
|--------|-----------|-------|
| **Architecture** | Flat vector store + SQLite | Multi-dimensional graph |
| **Learning** | Compression + retrieval | Hebbian emergence |
| **Structure** | Static | Dynamic reorganization |
| **Abstraction** | None | Automatic layer promotion |
| **Scope** | Single user sessions | Multi-agent coordination |
| **Role** | Context preservation | Cognitive substrate |
| **Integration** | Background only | Active participant |

**They are complementary:**

- claude-mem handles session-level context
- MDEMG handles knowledge-level emergence

---

## Emergent Layer Architecture

### Multi-Layer Hierarchy

MDEMG uses a 5-layer hierarchy where **constraints loosen as layers increase**, enabling emergent concept formation at higher levels of abstraction:

```
L5 (Emergent Concepts)    ← Very loose: eps=0.26, minSamples=2
    ↑ Message Passing
L4 (Abstract Concepts)    ← Loose: eps=0.22, minSamples=2
    ↑ Message Passing
L3 (Domain Concepts)      ← Moderate: eps=0.18, minSamples=2
    ↑ Message Passing
L2 (Concrete Concepts)    ← Tighter: eps=0.14, minSamples=3
    ↑ Message Passing
L1 (Hidden Aggregators)   ← Base: eps=0.10, minSamples=3
    ↑ DBSCAN Clustering
L0 (Base Observations)    ← Raw ingested data
```

### Adaptive Clustering Parameters

**Design principle:** Higher layers represent more abstract concepts that should cluster more freely.

| Parameter | Formula | Rationale |
|-----------|---------|-----------|
| **Epsilon** | `base_eps * (1 + 0.4 * layer)` | Abstract concepts are semantically farther apart but still related |
| **MinSamples** | `max(2, base_min - layer)` | Smaller emergent groups allowed at higher abstraction levels |
| **MaxClusterSize** | Generous (no aggressive shrinking) | Concepts can be broad at upper layers |

### Why Loose Constraints at Upper Layers?

1. **Lower layers** (L0-L1): Cluster tightly-related code elements (same file, same function pattern)
2. **Middle layers** (L2-L3): Group related concepts across files/modules (service patterns, data flows)
3. **Upper layers** (L4-L5): Emerge broad architectural patterns, cross-cutting concerns, system-wide behaviors

**Example emergence:**

- L0: Individual service methods
- L1: Hidden node grouping related methods
- L2: "Authentication Service" concept
- L3: "Security Infrastructure" pattern
- L4: "Cross-Cutting Concerns" abstraction
- L5: "System Architecture Principles" emergence

### Message Passing Between Layers

Each layer transition includes GraphSAGE-style message passing:

```
Forward Pass:  new_emb = α * self_emb + β * mean(neighbor_embs)
Backward Pass: updates flow down from abstract to concrete
```

This enables:

- Information propagation up (concrete → abstract)
- Context propagation down (abstract → concrete)
- Emergent representations refined through iteration

### No Early Termination

The system tries **ALL 5 layers** even if intermediate layers produce no clusters. Due to adaptive constraints, upper layers may cluster successfully even when middle layers don't.

---

## Development Roadmap

### Phase 1: Core Infrastructure ✅ COMPLETE

- [x] Neo4j graph with vector indexes
- [x] Go service with retrieval pipeline
- [x] Embedding generation (Ollama/OpenAI)
- [x] Embedding cache (LRU)
- [x] Learning loop (CO_ACTIVATED_WITH edges via Hebbian formula)
- [x] Edge weight decay CLI (`cmd/decay`)
- [x] Integration test suite

### Phase 2: Emergence Mechanics ✅ COMPLETE

- [x] Cluster detection for abstraction (`cmd/consolidate`)
- [x] Layer promotion via CLI
- [x] Automatic layer promotion triggers (Dynamic Pipeline Registry — Phase 46-PR)
- [x] Dynamic node reorganization (Pipeline split execution: RunPhaseRange)
- [x] Cross-layer relationship management (Phase 75: Cross-File Relationship Extraction)

### Phase 3: Active Participation ✅ COMPLETE

- [x] Reflection endpoint (`POST /v1/memory/reflect`)
- [x] Anomaly detection on ingest (duplicates, stale updates)
- [x] Graph health metrics (`GET /v1/metrics`)
- [x] Context-triggered suggestions (`POST /v1/memory/suggest`)
- [x] Periodic reflection summaries (RSIC Watchdog — Phase 60b)
- [x] Agent consulting service API (`POST /v1/memory/consult`)

### Phase 4: IDE Integration (Partial)

- [x] MCP server for IDE integration (`cmd/mcp-server/`)
- [ ] VS Code extension
- [ ] Cursor integration
- [ ] Real-time memory sidebar

### Phase 5: Multi-Agent Coordination ✅ COMPLETE

- [x] Agent workspace isolation (DevSpace Hub — Phase 32)
- [x] Shared memory protocols (Space Transfer — Phase 31)
- [x] Conflict resolution between agents (CRDT merge — Phase 35)
- [x] Collective learning aggregation (Global Meta-Learning — Phase 105)

### Phase 6: RSIC Self-Improvement Hardening ✅ COMPLETE

- [x] Recursive Self-Improvement Cycle foundation (Phase 60b)
- [x] Orchestration with trigger sources and cooldown policies (Phase 87)
- [x] Safety enforcement with dry-run and rollback (Phase 88)
- [x] Persistent state with multi-space correctness (Phase 89)
- [x] Conformance testing and CI gating — 6 integration tests, CI split, UATS tag filtering (Phase 90)

---

## Design Principles

1. **Emergence over engineering** - Let structure arise from data, don't impose it

2. **Stability of relationships** - Edges are the durable truth; organization is fluid

3. **Hardware as the only limit** - No arbitrary caps on layers or complexity

4. **Active over passive** - Don't wait to be asked; proactively surface value

5. **Local rules, global behavior** - Simple mechanisms (Hebbian learning, decay) produce complex emergent behavior

6. **Graceful degradation** - System should work at any scale, from 10 nodes to 10 million

---

## Technical Invariants (Do Not Violate)

These principles from the original design remain sacrosanct:

- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (computed in-memory, NOT persisted)
- **DB writes = learning deltas only** (bounded, no per-request activation writes)

---

## Success Metrics

How we'll know MDEMG is working:

1. **Reduced re-explanation** - Agents need less context to be productive
2. **Pattern recognition** - System identifies recurring patterns before humans do
3. **Cross-pollination** - Knowledge from Project A helps with Project B
4. **Emergent concepts** - Higher-layer nodes appear that weren't explicitly created
5. **Agent effectiveness** - Measurable improvement in agent task completion
6. **Coordination efficiency** - Multi-agent workflows with less conflict

---

*This document captures the vision as of March 2026. All 6 development phases are complete. Remaining work: IDE extensions (Phase 4 partial — VS Code, Cursor, real-time sidebar) and release infrastructure (Homebrew tap, v0.2.0 tag).*
