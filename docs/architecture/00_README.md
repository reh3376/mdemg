# Multi-Dimensional Emergent Memory Graph (MDEMG) — Documentation Index

> **Start Here:** For the architectural vision and design philosophy, see [VISION.md](../../VISION.md)
>
> **For Development:** See [AGENT_HANDOFF.md](../../AGENT_HANDOFF.md) for current state and next tasks

## What is MDEMG?

MDEMG is a **cognitive substrate** for AI coding agents where higher-level concepts **emerge automatically** from accumulated observations through Hebbian learning. Key characteristics:

- **Multi-layer** node taxonomy (L1..Ln) from concrete observations to abstract principles
- **Dynamic layers** that grow without limit as data accumulates
- **Typed, weighted relationships** with evidence and decay
- **Edge stability** — relationships persist while node organization is fluid
- **Vector embeddings** + Neo4j vector indexes for semantic recall
- **Spreading activation** + Hebbian updates for emergent associations
- **Active participation** — proactive surfacing, anomaly detection, agent consulting

## Documentation Structure

### High-Level Documents (Repository Root)

| Document | Purpose |
|----------|---------|
| [VISION.md](../../VISION.md) | Architectural philosophy, design principles, success metrics |
| [AGENT_HANDOFF.md](../../AGENT_HANDOFF.md) | Development state, phase registry, next tasks |
| [CLAUDE.md](../../CLAUDE.md) | AI assistant context for Claude Code |

### Technical Documentation (This Directory)

| Document | Purpose |
|----------|---------|
| `01_Architecture.md` | Services, flows, integration modes, technical invariants |
| `02_Graph_Schema.md` | Labels, relationships, properties, layer promotion mechanics |
| `03_Vector_Embeddings_and_Indexes.md` | Neo4j vector index, embedding generation |
| `04_Activation_and_Learning.md` | Spreading activation, Hebbian learning, emergence |
| `05_Ingestion_Pipeline.md` | Upsert patterns, edge updates, observation events |
| `06_Retrieval_API_and_Scoring.md` | Candidate generation, scoring, explanations |
| `07_Consolidation_and_Pruning.md` | Abstraction promotion, pruning rules |
| `08_Config_and_Tuning.md` | Configuration knobs and tuning parameters |
| `09_Testing_and_Simulation.md` | Synthetic graphs, regression tests |
| `10_Operational_Notes.md` | Deployment, performance, security, backups |
| `11_Migrations_as_Code.md` | Schema migration patterns |
| `12_Retrieval_Scoring_Worked_Examples.md` | Concrete scoring examples |
| `13_Go_Service_Framework.md` | Go service implementation details |
| `14_Operations_Runbook.md` | Operational procedures |

## Build Priorities (MVP)

1. **Schema + constraints** — labels, relationship types, property conventions ✅
2. **Ingestion** — append-only observations + embedding creation + basic edges ✅
3. **Retrieval** — vector candidate recall + expansion + scoring ✅
4. **Learning loop** — co-activation edge strengthening + decay job ✅
5. **Consolidation** — create abstraction nodes from stable clusters ✅

## Design Principles

### Emergence from Local Rules

> "Local rules produce global behavior. Simple mechanisms (Hebbian learning, decay) create complex emergent structures without explicit programming."

- Activation spreads, but decays
- Weights strengthen, but regularize
- Graph grows, but prunes and compresses

### The Edge Stability Principle

> "Edges are the durable truth; node organization is fluid."

Relationships persist while concepts can migrate between layers as understanding deepens. When a node is promoted to a higher layer:

- Edges are NEVER deleted (unless explicit decay/pruning)
- Relationships are additive — new `ABSTRACTS_TO` edges supplement existing
- Queries can traverse both layers — concrete and abstract paths coexist

### Technical Invariants (DO NOT VIOLATE)

- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (computed in-memory, NOT persisted)
- **DB writes = learning deltas only** (bounded, no per-request activation writes)

## Quick Reference

### Key Labels

- `:TapRoot` — Singleton per space_id
- `:MemoryNode` — Main memory nodes (has `embedding` property)
- `:Observation` — Append-only events
- `:SchemaMeta` — Schema version tracking

### Key Relationships

- `ASSOCIATED_WITH` — Semantic similarity
- `CO_ACTIVATED_WITH` — Learning signal (Hebbian)
- `CAUSES`, `ENABLES` — Causal links
- `ABSTRACTS_TO` / `INSTANTIATES` — Layer hierarchy

### Promotion Signals

| Signal | Description |
|--------|-------------|
| Frequency | Pattern appears across multiple contexts |
| Clustering | Multiple nodes form stable associations |
| Edge Strength | `CO_ACTIVATED_WITH` edges exceed threshold |
| Temporal Stability | Pattern persists over time |
| Cross-Domain Relevance | Pattern applies across different spaces |

---

*If you skip decay + consolidation, you'll get a graph-shaped junk drawer.*
