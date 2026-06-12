# Architecture (Neo4j + Vector Indexes)

> **See also:** [VISION.md](../VISION.md) for the philosophical foundation and design principles.

## Core Philosophy

MDEMG is a **cognitive substrate** for AI coding agents where higher-level concepts **emerge automatically** from accumulated observations. Unlike static knowledge bases:

- **Edges remain stable** - Once a relationship is learned, it persists
- **Node positions are fluid** - Concepts can move between layers as understanding deepens
- **Paths adapt** - The route to reach a concept changes as organization evolves
- **No manual maintenance** - Reorganization happens automatically through Hebbian learning

## Multi-Dimensional Layered Graph

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

## High-level components

1. **Neo4j**: property graph store for nodes/edges + weights + provenance.
2. **Neo4j Vector Index**: fast nearest-neighbor search over embeddings.
3. **Embedding generation**:
   - Option A: Neo4j **GenAI plugin** (`genai.vector.encode`, `encodeBatch`) to generate/store embeddings directly.
   - Option B: external embedding service (Python/Go) writes vectors to Neo4j.
4. **Memory Service (API)** (Go recommended for latency):
   - ingestion endpoint
   - retrieval endpoint
   - maintenance endpoints (decay, consolidation)
5. **Offline Jobs** (Python optional):
   - summarization refresh
   - consolidation clustering
   - health checks / anomaly detection

## Online flow (retrieve)

1. Query comes in (text + policy context).
2. Create query embedding (GenAI plugin or external).
3. Vector search: top N candidates via `db.index.vector.queryNodes`.
4. Graph expansion: 1–D hops from candidates using typed edges.
5. Activation pass: compute transient activation scores.
6. Rank and return:
   - top K nodes
   - explanation graph (why each node was selected)
7. Learning updates:
   - strengthen `CO_ACTIVATED_WITH` edges among selected nodes
   - bump evidence counts / timestamps

## Online flow (ingest)

1. Observation arrives with timestamp/source/content/tags.
2. Resolve target node(s) or create new.
3. Append observation event (immutable log).
4. Update node summary (small rolling summarization).
5. Generate/store embedding for node (or observation chunk).
6. Create/adjust edges:
   - semantic links (nearest neighbors)
   - temporal adjacency (recent event chain)
   - containment/path links
7. Optionally enqueue for consolidation evaluation.

## Offline flow (maintenance)

- **Decay job**: exponentially decay weights + prune weak edges.
- **Consolidation job**:
  - detect stable clusters at layer k
  - create abstraction node at layer k+1
  - add `ABSTRACTS_TO` edges
  - compress redundant lateral edges

## Key decision: where activation “lives”

Activation values are **transient**. Do NOT permanently write per-query activation to nodes unless:

- you store debug snapshots in a dedicated label (recommended for debugging only)
- or you want a short-lived cache with TTL semantics (hard in vanilla Neo4j)

Recommended:

- compute activation in the Memory Service runtime,
- write only *learning deltas* back to the graph.

## Integration Modes

MDEMG operates as a **full active participant** in the autonomous development workflow:

### 1. Claude Code Native Integration

- The primary consumer of MDEMG services is Claude Code via the enforced
  hooks (`session-start.sh`, `prompt-context.sh`, `post-tool-observe.py`)
  and the MCP memory channel (23 tools, `MDEMG_SPACE_ID`-scoped).
- (Historical note: an earlier host project, `aci-claude-go`, was the
  original primary consumer; its `internal/orchestrator` integration no
  longer exists in this repo.)

### 2. Background Service

- Always running, similar to claude-mem.
- API available for agent queries and TUI dashboard stats.
- Continuous learning from observations gathered during coding sessions.

### 3. Event-Driven Hooks

- Git worktree transitions trigger memory updates.
- Spec status changes (planning -> completed) trigger consolidation.
- Session lifecycle events (start/end) trigger higher-order reflection.

### 4. Proactive Surfacing

| Mode | Behavior |
|------|----------|
| **Context Suggestions** | When working on code, surface related patterns/decisions from MDEMG. |
| **Periodic Reflection** | Synthesize insights at session start/end to maintain conceptual continuity. |
| **Anomaly Detection** | Alert when current implementation diverges from stored requirements (Drift Detection). |
| **Subject Matter Expertise** | Provide the "SME substrate" for the Planner, Coder, and QA agents. |

### 5. Agent Consulting Service

A higher-order capability where MDEMG acts as an **SME (Subject Matter Expert)** for coding agents:

- **Context provision**: "Based on this codebase's patterns..."
- **Process guidance**: "The typical workflow for this type of change is..."
- **Concept synthesis**: "This relates to the higher-level principle of..."
- **Risk awareness**: "Previous attempts at this approach encountered..."

### 6. Jiminy Inner-Voice Service

Proactive guidance injected into every user prompt via Claude Code hooks (`prompt-context.sh`). Orchestrates 4 knowledge sources in parallel within the config-driven
guidance budget (`JIMINY_TIMEOUT_MS`, default 0 = derived from the 90s
`JIMINY_WARM_COMPUTE_TIMEOUT_MS` — JIMINY-BUDGET-001):

| Source | What It Finds |
|--------|---------------|
| **Consulting Suggestions** | Constraints, conflicts, patterns from the graph |
| **Correction Vector Search** | Past corrections semantically similar to current context |
| **Contradiction Edges** | `CONTRADICTS` relationships between relevant nodes |
| **Frontier Detection** | Thin knowledge areas where the agent should proceed carefully |

Results are merged, deduplicated, filtered by confidence, and formatted as an injectable `═══ JIMINY GUIDANCE ═══` block. This is the operational manifestation of the Agent Consulting Service (#5) — it runs automatically on every prompt rather than on-demand. See `internal/jiminy/` for implementation and `docs/features/jiminy-inner-voice.md` for the full feature guide.

## Modular Intelligence Architecture

MDEMG uses a **plugin-based architecture** for extensibility. Modules communicate via gRPC over Unix sockets for low latency.

### Module Types

| Type | Purpose | Example |
|------|---------|---------|
| **INGESTION** | Parse external sources into observations | `linear-module`, `obsidian-module` |
| **REASONING** | Process/re-rank retrieval results | `keyword-booster`, `llm-reranker` |
| **APE** | Background tasks and autonomous actions | `reflection-module`, `consistency-checker` |

### Plugin System Components

```
┌─────────────────────────────────────────────────────────────┐
│                      MDEMG Server                           │
├─────────────────────────────────────────────────────────────┤
│  Plugin Manager (internal/plugins/)                         │
│  ├── Discovery: scans /plugins directory                    │
│  ├── Lifecycle: spawn, handshake, health check, shutdown    │
│  └── Routing: match requests to capable modules             │
├─────────────────────────────────────────────────────────────┤
│  Reasoning Provider (internal/retrieval/reasoning.go)       │
│  └── Hooks reasoning modules into retrieval pipeline        │
├─────────────────────────────────────────────────────────────┤
│  APE Scheduler (internal/ape/scheduler.go)                  │
│  ├── Cron-based scheduling                                  │
│  └── Event-triggered execution                              │
└─────────────────────────────────────────────────────────────┘
          │ gRPC over Unix Sockets
          ▼
┌─────────────────────────────────────────────────────────────┐
│  /plugins/                                                  │
│  ├── linear-module/     (INGESTION)                         │
│  ├── keyword-booster/   (REASONING)                         │
│  └── reflection-module/ (APE)                               │
└─────────────────────────────────────────────────────────────┘
```

### Retrieval Pipeline with Modules

```
1. Vector recall           → candidates from vector index
2. BM25 + RRF fusion       → hybrid search (if enabled)
3. Graph expansion         → 1-D hop traversal
4. Spreading activation    → compute transient scores
5. Initial scoring         → ScoreAndRankRRF (default, Column-Voting RRF; legacy ScoreAndRank = fallback/breakdown path)
6. REASONING MODULES       → plugin-based re-ranking
7. Built-in LLM rerank     → optional LLM scoring
8. Jiminy explanations     → explainable retrieval (see internal/retrieval/jiminy.go)
                             + Jiminy Guide service  → proactive guidance injection (see internal/jiminy/)
```

### APE Event Triggers

| Event | When Triggered |
|-------|----------------|
| `session_end` | User session closes |
| `consolidate` | After consolidation completes |
| `ingest` | After batch ingest |
| `schedule` | Cron-based (e.g., hourly) |

## Key Data Structures (historical — aci-claude-go era)

- **InternalDialog**: A chronological thread of agent thoughts stored as linked `MemoryNode` entities.
- **SessionInsights**: Structured outcomes of agent sessions including discoveries, what worked, and what failed.
- **CodebaseDiscoveries**: Map of files understood and patterns identified by agents.

## Technical Invariants (DO NOT VIOLATE)

- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (computed in-memory, NOT persisted)
- **DB writes = learning deltas only** (bounded, no per-request activation writes)
- **Conceptual Continuity**: Internal Dialog must be preserved across session handoffs.
- **Surface convention**: newer endpoint families (j17, guardrail, eventgraph) wrap responses in a `{ "data": ... }` envelope; core endpoints (e.g. `/v1/memory/retrieve`) return unwrapped responses — check the handler, not a global rule.
- **Space Isolation**: Memories are partitioned by `space_id` (typically the project base name).
