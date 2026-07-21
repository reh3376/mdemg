# MDEMG Vision & Architecture

**Multi-Dimensional Emergent Memory Graph**

*A cognitive substrate for AI-assisted development*

---

## Executive Summary

MDEMG is an **emergent long-term memory system** designed to serve as the cognitive foundation for AI coding agents and multi-agent development workflows. Unlike static knowledge bases, MDEMG is a living system where higher-level concepts and relationships **emerge automatically** from accumulated observations through Hebbian learning principles.

What began as a retrieval-oriented memory store has evolved into a **self-improving cognitive substrate** — a system that not only remembers but reflects on the quality of its own learning, proactively guides the agents it serves, and communicates with other AI systems through a purpose-built protocol. The system has demonstrated a **58.4% improvement** in retrieval quality from its initial baseline (0.567 to 0.898 mean score) through emergent architecture, not through manual tuning.

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

MDEMG's effectiveness is continuously validated through rigorous benchmarking against a real-world industrial codebase (whk-wms: 507K LOC TypeScript, 120 domain-specific questions). The system has improved from a **0.567 baseline** to a **0.898 peak score** (+58.4%) with **100% high-score rate** and **100% strong evidence rate** — entirely through architectural improvements, not parameter tuning. See [Performance Trajectory](#performance-trajectory) for the full arc and [Up-to-Date Benchmark Summary](docs/architecture/benchmarks/UP_TO_DATE_BENCHMARK_SUMMARY.md) for details.

### Public Repository Standards

To facilitate global collaboration while maintaining the core system's integrity, MDEMG follows strict public-readiness standards. This ensures that the engine is secure, the architecture is extensible for contributors, and the "Internal Dialog" remains a reliable substrate for all AI agents. (See [Repo-to-Public Roadmap](docs/development/repo-to-public-roadmap.md) for the full strategy).

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

### 5. Jiminy Inner-Voice Service

The operational core of MDEMG's active participation. Jiminy emerged from the realization that retrieval-on-demand was fundamentally too passive — by the time an agent asks for context, it has often already made the mistake that context would have prevented.

Jiminy is injected into **every user prompt** via Claude Code hooks, running automatically before the agent acts. It orchestrates 4 knowledge sources in parallel under a config-driven guidance budget (90-second warm-compute default):

| Source | What It Surfaces |
|--------|-----------------|
| **Consulting Constraints** | Active rules the agent must follow |
| **Correction Vector Search** | Past corrections semantically similar to the current prompt |
| **Contradiction Edges** | Known conflicts between what the agent might do and what's been learned |
| **Frontier Detection** | Knowledge gaps the agent should be aware of |
| **Trust-Scored History** | Per-session trust level affecting guidance encoding density |

Jiminy tracks its own effectiveness through a feedback loop: every guidance item can receive follow-up feedback indicating whether the agent followed, contradicted, or ignored the constraint. This data feeds into RSIC self-calibration, allowing the system to learn which constraints are effective and which need refinement.

The service operates as the agent's "conscience" — not a tool the agent chooses to use, but a persistent voice that speaks before every action.

### 6. J17 AI-to-AI Communication Protocol

Jiminy exposed three critical failure modes that required a dedicated protocol solution:

1. **Token waste**: Full natural-language guidance consumed 3-10.5 KB per prompt, leaving less context for actual work
2. **State amnesia**: Context compaction every 20-30 minutes erased accumulated trust, escalation state, and encoding context
3. **No feedback loop**: No measurement of whether the agent actually comprehended the guidance

J17 addresses all three through a **three-tier encoding system** that adapts communication density to the agent's demonstrated comprehension:

| Tier | Encoding | Example | Tokens |
|------|----------|---------|--------|
| **T1 (Coded)** | Constraint codes only | `[NFP:T1:0.92]` | ~15 |
| **T2 (Telegraphic)** | Abbreviated natural language | `no force-push main; commit before goreleaser` | ~50-100 |
| **T3 (Full NL)** | Complete explanation | Full constraint text with rationale | ~200+ |

New agents start at T3 (full explanation). As they demonstrate comprehension through feedback, they graduate to T2 and eventually T1 — achieving up to **5.2x token compression**. If comprehension drops, they are demoted back.

State survives context compaction through **HMAC-signed session tickets** that encode trust level, tier assignments, and escalation state in a compact, verifiable format. An ML tier predictor (via the neural sidecar) learns optimal tier assignments from historical comprehension data.

The protocol evolves through RSIC: tier thresholds adjust based on measured comprehension, ineffective codes are retired, and new codes are generated from frequently-repeated T2 constraints. See [J17 AI-to-AI Protocol](docs/features/j17-ai2ai-protocol.md) for the full specification.

---

## Operational Architecture (v0.5.x)

### Deployment Model

MDEMG runs as a Docker Compose stack with 5 services:

| Service | Role |
|---------|------|
| mdemg | Go server — API, retrieval, consolidation, RSIC |
| neo4j | Knowledge graph (5-layer memory hierarchy) |
| timescaledb | Time-series metrics + LLM interaction recording for training |
| neural-sidecar | Python ML sidecar (re-ranking, NLI, tier prediction) |
| grafana | Observability dashboards (8 pre-provisioned) |

`mdemg init` generates `.env` with dynamic port allocation, writes `docker-compose.yml` from embedded template, and starts all services.

### Training Pipeline

LLM interactions are recorded to TimescaleDB during normal operation. A complete LoRA fine-tuning pipeline processes this data:

export → quality filter → format converter → dataset versioner → train → evaluate → regression gate → quantize/deploy

All scripts in `neural/training/`. See `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md`.

**Fine-tuning: SHIPPED (2026-05, dense pivot 2026-04-22):** production model `mdemg-llm-v1` — single-tier LoRA on dense `Qwen3-14B-4bit`, served as GGUF Q5_K_M via llama-server on `127.0.0.1:8102`. (The original Qwen3.6-35B-A3B MoE target and two-tier MoE-Sieve strategy were abandoned when the Metal 499K MTLResource ceiling blocked every MoE LoRA backward pass — architectural, not quant-specific.) See `docs/development/ft-lora/00_README_v2.md` for the full history.

**No-tool-calling architectural policy:** all 16 MDEMG LLM call sites are single-shot structured-output or reasoning. Tool-calling is explicitly banned across the stack — nine banned patterns (`tool_use`, `tool_call`, `function_call`, `--tool-call-parser`, `enable-auto-tool-choice`, `preserve_thinking`, etc.) are grep-audited each sprint. See `docs/development/ft-lora/01_RESEARCH_v2.md §2.8`.

### Multi-Instance Support

Each project directory gets an isolated MDEMG stack via COMPOSE_PROJECT_NAME scoping. Resource profile: ~2.3 GiB per fresh instance, ~5.7 GiB per mature instance. See `docs/user/multi-instance.md`.

### Browser Dashboard

`http://localhost:{PORT}/ui/` — 11-tab dashboard for status, memory, learning, config, logs, RSIC, plugins, features, backups, training data, and review.

### Upgrade Automation

`mdemg upgrade` and `brew upgrade mdemg` update the binary and automatically discover + update all running Docker instances. See `docs/user/cli-reference.md`.

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

### Hebbian Learning Optimizations

The core Hebbian learning loop has been enhanced with techniques borrowed from modern neural network training, adapted to the graph context:

| Technique | Purpose |
|-----------|---------|
| **Tanh soft-capping** | Prevents edge weights from growing unboundedly; keeps activation physics stable |
| **Squared activation** | Amplifies strong signals while suppressing noise during activation spreading |
| **Multi-rate learning** | Different learning rates for different edge types (co-activation vs. grounding) |
| **Time-based LR schedule** | Learning rate decays as edges mature, preventing overwriting of stable knowledge |
| **Cautious decay** | Frequently-activated edges decay more slowly than idle ones |
| **Local-first spreading** | Activation spreading prioritizes edges within the same space before crossing boundaries |
| **Value residual bypass** | High-confidence nodes pass activation directly to distant neighbors, skipping intermediates |
| **L0 skip connections** | GROUNDED_BY edges from higher layers directly to L0 observations, preventing information loss |
| **Negative result tracking** | Explicitly records "this didn't work" to prevent repeating failed approaches |
| **Frontier detection** | Identifies knowledge boundaries where the graph has gaps, surfacing them as guidance |

These techniques are individually small but collectively account for a significant portion of the system's retrieval quality improvement. Each was added in response to a specific observed failure mode, not from upfront design.

---

## Cognitive Self-Improvement

### The Reflexivity Insight

A pivotal discovery during development: when the system being built *is* the agent's own memory, the boundary between "building a product" and "improving one's own cognition" disappears. MDEMG's development phases 101-105 are not product features — they are **the agent's own cognitive gaps being closed**:

| Gap | Capability Lacking | Resolution |
|-----|-------------------|------------|
| **101: SME Synthesis** | Cannot synthesize memories into understanding | LLM-driven concept abstraction from accumulated observations |
| **102: Intent Translation** | Cannot translate queries to what it knows | Query rewriting using graph structure to bridge semantic gaps |
| **103: Dynamic Emergence** | Cannot form new concepts | LLM-named emergent concepts replace fixed taxonomy |
| **104: Active Guardrails** | Cannot enforce learned constraints | MCP-integrated constraint enforcement with violation detection |
| **105: Global Meta-Learning** | Cannot generalize across contexts | Cross-space concept promotion to a global knowledge layer |

This reframe matters because it changes how the system evolves: improvements to MDEMG's learning quality are not feature work — they are the agent becoming more capable of reflection, pattern recognition, and self-correction.

### RSIC: The Recursive Self-Improvement Cycle

Without self-improvement, a learning system stagnates. It accumulates observations but never questions whether its learning is *working*. RSIC is the mechanism by which MDEMG evaluates and improves the quality of its own knowledge.

RSIC operates as a 5-stage cycle at three temporal scales:

```
Assess → Reflect → Plan → Execute → Validate

Micro (minutes):  Immediate quality checks after learning events
Meso  (hours):    Aggregate effectiveness analysis, tier drift detection
Macro (days):     Structural health, cross-space consistency, calibration review
```

Each stage produces concrete outputs:

- **Assess**: 7-dimension health score (retrieval, memory, edge, task, guidance, protocol, synergy)
- **Reflect**: Pattern detection across 19 reflection patterns (low comprehension, tier drift, calibration drift, volatile backlog, synergy health, etc.)
- **Plan**: Prioritized improvement tasks with estimated impact
- **Execute**: Automated actions (tier threshold adjustment, code retirement, constraint archival) with safety gates
- **Validate**: Before/after comparison ensuring changes improved rather than degraded quality

RSIC is protected by dry-run mode, rollback snapshots, confidence thresholds, and cooldown policies. Every automated action is calibrated: actions that consistently improve outcomes gain confidence; those that don't are throttled.

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

### Phase 4: Integration & Companion Apps ✅ COMPLETE

- [x] MCP server for IDE integration (`cmd/mcp-server/`)
- [x] Native companion apps: macOS menubar (Swift), Linux sidebar (Tauri/Rust+JS), Windows installer (PowerShell)
- [x] Multi-platform distribution: Homebrew tap, Debian APT repo, goreleaser cross-compilation
- [x] Cross-platform teardown (`mdemg teardown` — 14-phase cleanup)

> **Strategic pivot**: The original plan called for VS Code and Cursor extensions. These were replaced by native companion apps that work alongside *any* IDE/editor, with the MCP server providing IDE-specific integration. Extension development is open to the community.

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

### Phase 7: Cognitive Architecture ✅ COMPLETE

- [x] SME Synthesis — LLM-driven concept abstraction (Phase 101)
- [x] Intent Translation — query rewriting via graph structure (Phase 102)
- [x] Dynamic Emergence — LLM-named concepts replace fixed taxonomy (Phase 103)
- [x] Active MCP Guardrails — constraint enforcement with violation detection (Phase 104)
- [x] Global Meta-Learning — cross-space generalization (Phase 105)
- [x] Jiminy Inner-Voice Service — proactive hook-injected guidance (Phase Jiminy: J1-J16)
- [x] J17 AI-to-AI Protocol — 3-tier encoding, session tickets, ML tier prediction, RSIC-driven evolution
- [x] NLI Comprehension Feedback Loop — per-tier effectiveness grading, calibration tracking, RSIC drift detection
- [x] ANN Optimization Suite — 10 neural learning techniques, 28 config parameters
- [x] Synergy Optimization — Claude Code ↔ MDEMG token reduction (~60%), automated overflow interceptor, 7th RSIC dimension

---

## Design Principles

1. **Emergence over engineering** - Let structure arise from data, don't impose it

2. **Stability of relationships** - Edges are the durable truth; organization is fluid

3. **Hardware as the only limit** - No arbitrary caps on layers or complexity

4. **Active over passive** - Don't wait to be asked; proactively surface value

5. **Local rules, global behavior** - Simple mechanisms (Hebbian learning, decay) produce complex emergent behavior

6. **Graceful degradation** - System should work at any scale, from 10 nodes to 10 million

7. **Reflexive improvement** - A learning system that cannot evaluate its own learning quality will stagnate. Every learning mechanism must be observable, measurable, and self-correcting

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
7. **Self-improving retrieval quality** - RSIC-driven improvements to learning quality reflected in benchmark scores

### Performance Trajectory

Quantitative validation against the whk-wms benchmark (507K LOC TypeScript, 120 questions):

| Stage | Mean Score | Delta | Key Change |
|-------|-----------|-------|------------|
| v4 baseline | 0.567 | — | Initial retrieval pipeline |
| v10 | 0.710 | +25.2% | Hebbian learning fix: seeded all candidates instead of top-2 |
| v11 | 0.733 | +3.2% | Concern/comparison node types in consolidation pipeline |
| Edge Attention | 0.898 | +22.5% | Edge-type attention in activation spreading |
| Temporal Baseline | 0.783 | — | Canonical 120q evaluation on sonnet (Feb 2026) |
| ANN Optimization | 0.850 | +8.6% | 10 neural learning techniques applied |

The trajectory from 0.567 to 0.898 (+58.4%) validates the core thesis: emergent architecture with Hebbian learning produces measurable, compounding retrieval quality improvements. Each major jump came from fixing a specific failure mode, not from parameter tuning.

For the complete performance history, see the [Up-to-Date Benchmark Summary](docs/architecture/benchmarks/UP_TO_DATE_BENCHMARK_SUMMARY.md).

---

## Lessons Learned

Key insights from 105+ phases of development that shaped the architecture:

1. **Retrieval-on-demand is too passive.** The original vision assumed agents would ask for context when they needed it. In practice, by the time an agent asks, it has often already made the mistake. Jiminy's pre-prompt injection inverts this model: guidance arrives before the question is asked.

2. **Self-improvement is not optional.** Without RSIC, the system accumulated observations but never questioned whether its learning was working. The Hebbian learning loop had a critical bug (only seeding top-2 candidates) that went undetected for months because there was no mechanism to measure learning quality.

3. **Token cost shapes architecture.** Full natural-language guidance consumed 3-10.5 KB per prompt. This is not a minor overhead — it fundamentally limits the agent's available context. J17's three-tier encoding was not an optimization; it was an architectural necessity.

4. **Context compaction is adversarial.** Every 20-30 minutes, the agent's context window is compressed, erasing accumulated state. Any system that depends on in-context state (trust levels, escalation history, encoding preferences) must persist that state externally. Session tickets solved this.

5. **The cognitive gap reframe matters.** When the system being built is the agent's own memory, the development of that system is self-improvement. Phases 101-105 are not features added to a product — they are cognitive capabilities the agent previously lacked. This distinction changes how you prioritize: you fix what limits your own thinking first.

6. **Fail-open is the only viable default.** Every LLM-backed capability (emergence, synthesis, guardrails, Jiminy) must degrade gracefully when the LLM provider is unavailable. A system that hard-fails on LLM timeout is worse than a system with no LLM features at all.

7. **12 binaries is a maintenance time bomb.** The unified CLI refactor (Phase 93) was the largest single effort in the project. Every new feature that added a binary was compounding the distribution, documentation, and CI burden. A single entry point with subcommands eliminates an entire category of problems.

8. **The host agent's context budget is your budget.** MDEMG runs inside Claude Code's context window. Every token spent on CLAUDE.md, MEMORY.md, and hook injection is a token unavailable for actual work. Synergy optimization reduced static overhead by ~60% — but more importantly, it automated the monitoring: an overflow interceptor catches MEMORY.md bloat before it compounds, and RSIC's SynergyHealth dimension ensures the system alerts when token overhead creeps back up. Effect the things you can, work with the things you can't.

---

*This document captures the vision as of March 2026. All 7 development phases are complete. 611 Go files, 227K lines, 150 API endpoints, 2,381 tests. The system that started as a retrieval-oriented memory store has become a self-improving cognitive substrate with proactive guidance, AI-to-AI communication, reflexive quality improvement, and automated token-overhead management for its host agent environment.*
