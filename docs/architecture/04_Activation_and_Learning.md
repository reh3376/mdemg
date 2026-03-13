# Activation + Learning Engine

This doc specifies the "physics" that produces emergent association patterns.

> **Design Principle:** Local rules produce global behavior. Simple mechanisms (Hebbian learning, decay) create complex emergent structures without explicit programming.

## The Emergence Principle

> "The system must be highly dynamic with the ability to reorder its nodes as new information causes unanticipated changes to the underlying data structures. Edges will not likely change, but the path to nodes will."

This captures the key insight: **relationships are stable, but the conceptual organization is fluid**. Just as human memory reorganizes concepts as understanding deepens, MDEMG allows nodes to migrate through layers while preserving their relational connections.

## 1) Activation state

Activation values are transient per query.

Define for each node i:

- `a_i ∈ [0,1]` activation
- optional: `b_i` baseline (e.g., recency/importance prior)

Seed activation for candidate nodes:

- from vector similarity score
- from explicit query node(s)

Example seed:

- `a_i(0) = clamp(score_i, 0, 1)`

## 2) Effective edge weight

Each relationship carries:

- base `weight`
- dimension scalars: `dim_semantic`, `dim_temporal`, `dim_causal`, `dim_coactivation`, ...

Define:

- `w_eff(i→j) = weight * (α*dim_semantic + β*dim_temporal + γ*dim_coactivation + ...) * recency_factor`
- `recency_factor = exp(-ρ * Δt)` using `updated_at` or `last_activated_at`

Treat `CONTRADICTS` edges as inhibitory:

- `w_inhib(i→j) = abs(weight) * dim_contradiction`

## 3) Spreading activation (discrete steps)

For T steps:

- `a_j(t+1) = clamp( (1-λ)*a_j(t) + Σ_i a_i(t)*w_eff(i→j) - Σ_k a_k(t)*w_inhib(k→j), 0, 1 )`

Knobs:

- `T` 2–5 is usually enough for retrieval contexts
- `λ` step decay prevents runaway amplification

## 4) Learning: Hebbian update (co-activation)

After retrieval, strengthen co-activated links among top-K nodes.

For nodes i,j in returned context:

- `Δw_ij = η * etaMult * a_i * a_j - μ * w_ij`
- `w_ij ← wmax * tanh(w_ij / wmax)` (smooth saturation via tanh soft-cap)

**Tanh Soft-Capping**: Replaces hard clamp `[wmin, wmax]` with smooth saturation. Edges near 1.0 continue learning (tanh(1)≈0.762). Lower bound remains a hard floor at 0.

**Multi-Rate Learning**: Context-specific eta multipliers computed in Cypher:
- Conversation observations: `× LEARNING_ETA_CONVERSATION_MULT` (default 2.0)
- Config↔code associations: `× LEARNING_ETA_CONFIG_MULT` (default 1.5)
- Same-directory nodes: `× LEARNING_ETA_SAME_DIR_MULT` (default 1.2)
- Multipliers stack (max ~3.6×), bounded by tanh cap

**Learning Rate Schedule**: Maturity-based eta scaling via edge count per space:
- Cold (0 edges): `× LEARNING_SCHEDULE_COLD_MULT` (default 2.0)
- Learning (1-10k): `× LEARNING_SCHEDULE_LEARNING_MULT` (default 1.0)
- Warm (10k-50k): `× LEARNING_SCHEDULE_WARM_MULT` (default 0.5)
- Saturated (50k+): `× LEARNING_SCHEDULE_SAT_MULT` (default 0.25)

Notes:

- apply to edge type `CO_ACTIVATED_WITH` (create if missing)
- increment `evidence_count`
- update timestamps
- keep the graph symmetric for CO_ACTIVATED_WITH: store both directions or enforce undirected semantics

## 4b) Negative Feedback

`POST /v1/learning/negative-feedback` allows explicit weakening of associations:

- If `CO_ACTIVATED_WITH` exists between query and rejected nodes: weaken by `LEARNING_NEGATIVE_WEIGHT` (floor at 0)
- If no existing edge: create `CONTRADICTS` edge (increment `evidence_count` on match)
- Capped at `LEARNING_NEGATIVE_MAX_PER_REQUEST` (default 20) rejected nodes per request

## 5) Decay (edge weight over wall-clock time)

Periodically:

- `w_ij ← w_ij * exp(-decay_rate * Δt)`

**Cautious Decay**: Edges reinforced within `LEARNING_CAUTIOUS_DECAY_WINDOW_HOURS` (default 24h) are skipped during decay. Uses existing `last_activated_at` property. Set to 0 to disable.

Prune if:

- `w_ij < prune_threshold` AND `evidence_count` low AND not pinned

## 5b) Local-First Activation Spreading

Per-hop minimum weight thresholds for activation spreading. Prevents weak edges from diluting signal:

- Hop 0: only strong edges ≥ `ACTIVATION_HOP0_MIN_WEIGHT` (default 0.5)
- Hop 1: moderate edges ≥ `ACTIVATION_HOP1_MIN_WEIGHT` (default 0.2)
- Hop 2+: all edges ≥ `ACTIVATION_HOP2_MIN_WEIGHT` (default 0.05)

**Critical**: Degree normalization uses filtered edge count, not total. Otherwise filtering reduces the denominator while keeping accumulated signal, causing artificial inflation.

## 6) Where to compute activation

**Recommended**: compute in service runtime.

- Step 1: fetch neighborhood edges for candidate nodes (1–2 hops)
- Step 2: run activation math in-memory (fast)
- Step 3: write only learning deltas back to Neo4j

Why: writing per-query activation into Neo4j creates write amplification and contention, and it’s not a durable “memory.”

## 7) Neo4j query patterns to support activation computation

Fetch a candidate neighborhood (bounded):

```cypher
MATCH (seed:MemoryNode {space_id:$spaceId})
WHERE seed.node_id IN $seedNodeIds
MATCH (seed)-[r]->(nbr:MemoryNode {space_id:$spaceId})
WHERE type(r) IN $allowedRels
RETURN seed.node_id AS src, nbr.node_id AS dst,
       type(r) AS relType,
       r.weight AS w,
       r.dim_semantic AS dim_semantic,
       r.dim_temporal AS dim_temporal,
       r.dim_coactivation AS dim_coactivation,
       r.updated_at AS updated_at
LIMIT $maxEdges;
```

## 8) Emergence failure modes (and how to not die)

- **Hub explosion**: one node connects to everything.
  - fix: degree caps, regularization, prune weak edges, penalize high-degree nodes in ranking
- **Clique spam**: CO_ACTIVATED_WITH grows dense.
  - fix: apply updates only to top-K and require minimum activation threshold
- **Forgetting everything**: decay too aggressive.
  - fix: lower decay, add pinning, add baseline importance

## 9) From Learning to Emergence

Hebbian learning is the foundation for emergent behavior. Over time:

### Pattern Detection

When nodes are repeatedly co-activated:

1. `CO_ACTIVATED_WITH` edges strengthen
2. Clusters form naturally
3. These clusters become candidates for **layer promotion**

### Promotion Trigger Signals

| Signal | Threshold | Action |
|--------|-----------|--------|
| **Cluster Density** | >5 nodes with mutual `CO_ACTIVATED_WITH` edges (weight > 0.3) | Flag for abstraction |
| **Retrieval Frequency** | Node appears in >10% of queries for space | Increase importance baseline |
| **Cross-Space Activation** | Pattern appears in 3+ distinct `space_id`s | Mark as generalizable |
| **Temporal Persistence** | Pattern stable for >7 days | Confirm not transient |

### Emergence Lifecycle

```
[Observations]     →     [Co-activation]     →     [Cluster Detection]
     ↓                        ↓                          ↓
Many L1 nodes          Edges strengthen          Consolidation job runs
                                                         ↓
                                                  [Abstraction Created]
                                                         ↓
                                                  New L(k+1) node
                                                  with ABSTRACTS_TO edges
```

### Success Metrics

The learning system is working when:

1. **`CO_ACTIVATED_WITH` edges form** - Nodes retrieved together develop connections
2. **Clusters emerge** - Dense subgraphs appear without explicit creation
3. **Abstractions crystallize** - Higher-layer nodes capture general principles
4. **Cross-pollination occurs** - Knowledge from one project helps another
