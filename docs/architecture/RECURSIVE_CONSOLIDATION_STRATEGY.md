# MDEMG Recursive Consolidation & Semantic Consensus Strategy

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


## 1. Overview

This strategy explores the "massive upside" of running multiple ingestion and consolidation passes to refine the MDEMG graph. By treating consolidation as an iterative reinforcement process rather than a one-time event, we can identify **Deep Invariants**—architectural truths that persist across different "views" of the codebase.

---

## 2. The "Consensus" Principle

To prevent the graph from becoming a "junk yard" of redundant nodes, we introduce the **Stability Score**.

* **Rule**: Every node starts with a `stability_score` of `0.1`.
* **Reinforcement**: If a subsequent consolidation run identifies the same semantic cluster, the stability score of the existing hidden/concept node increases.
* **Promotion**: Once a node hits a stability threshold (e.g., `0.8`), it is marked as **Established** and is protected from aggressive pruning.
* **Decay**: Nodes that are not "rediscovered" in subsequent runs have their stability decayed and are eventually tombstoned.

---

## 3. Multi-Pass Ingestion ("Lens" Modules)

Instead of ingesting the codebase once, we can ingest it using different **Cognitive Lenses**:

| Lens | Parser Priority | Target Relationships |
|------|-----------------|----------------------|
| **Structural** | Class/Interface AST | `EXTENDS`, `IMPLEMENTS` |
| **Functional** | Function Call Graph | `CALLS`, `ENABLES` |
| **Data Flow** | DTO/Database Models | `PRODUCES`, `CONSUMES` |
| **Concern** | Security/Logging Tags | `IMPLEMENTS_CONCERN` |

**Iterative Workflow**:

1. Run **Structural Ingest** → Consolidate.
2. Run **Functional Ingest** → MERGE with existing nodes, add new edges → Consolidate.
3. Run **Consensus Pruning** → Merge highly similar nodes (Similarity > 0.95).

---

## 4. Activation-Biased Consolidation (ABC)

Current consolidation is based purely on embedding distance (DBSCAN). **ABC** introduces "Retrieval Evidence" into the clustering logic.

* **Feedback Loop**: Nodes that are frequently co-retrieved (high `CO_ACTIVATED_WITH` weight) are given a "Gravity Boost."
* **Gravity Boost**: During the next consolidation run, these nodes are treated as being semantically closer than their raw embedding distance suggests.
* **Result**: The graph's physical structure eventually "morphs" to match the actual usage patterns of the AI agents.

---

## 5. Deliverables & Implementation

### A. Stability Property (`V0007__stability_logic.cypher`)

* Add `stability_score` and `discovery_count` to `MemoryNode`.
* Add `last_confirmed_at` timestamp.

### B. Consensus API (`POST /v1/memory/consolidate/consensus`)

* A new endpoint that performs "Differential Consolidation."
* Instead of creating new nodes for every cluster, it attempts to match new clusters against existing low-stability nodes.

### C. The "Lean Graph" Guardian

* Integrated `prune` logic that runs automatically after every 3rd consolidation.
* **Automatic Merging**: Uses the `union-find` algorithm (already in `prune`) to collapse transitive merge chains.

---

## 6. Risks & Mitigations

* **Risk**: Recursive runs create "Ghost Clusters" (hallucinated patterns).
  * **Mitigation**: Minimum `member_count` for reinforcement (e.g., a cluster must have at least 3 nodes to be considered for a stability boost).
* **Risk**: High computational cost of repeated DBSCAN.
  * **Mitigation**: Incremental clustering (only run DBSCAN on `L0` nodes that have changed or are orphaned).
