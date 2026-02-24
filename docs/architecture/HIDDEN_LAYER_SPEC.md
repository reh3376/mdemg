# Hidden Layer Architecture Technical Specification

**Version:** 1.0
**Status:** Draft
**Last Updated:** 2026-01-21

---

## Overview

This specification defines the implementation of hidden layers in MDEMG to provide generalized concept representations through hierarchical graph convolution. The hidden layer architecture addresses the limitations of pure Hebbian learning by adding intermediate abstraction layers that learn compressed, transferable representations.

---

## Problem Statement

### Current Limitations

1. **Hebbian Drift**: Pure Hebbian learning creates reinforcements that:
   - Degrade slowly and unevenly
   - Become overly rigid (overfitting to specific instances)
   - Don't generalize well across similar concepts

2. **Flat Abstraction**: Current consolidation creates abstractions directly from base data without intermediate generalization layers

3. **No Backpropagation**: Changes to concepts don't influence how base data is grouped

### Proposed Solution

Add hidden layer(s) between base data (Layer 0) and concepts (Layer 2+) that:

- Learn compressed representations through forward aggregation
- Receive feedback from concepts through backward propagation
- Provide regularization against Hebbian drift
- Enable transfer learning between similar patterns

---

## Layer Architecture

### Layer Definitions

| Layer | Name | Content | Role Type |
|-------|------|---------|-----------|
| 0 | Base Data | Raw observations, specific instances (stylesheets, code snippets, session insights) | `leaf`, `observation` |
| 1 | Hidden | Generalized patterns learned via clustering + message passing | `hidden`, `pattern` |
| 2+ | Concepts | Abstract rules, principles, high-level abstractions | `concept`, `abstraction` |

### Relationship Types

| Relationship | Direction | Purpose |
|--------------|-----------|---------|
| `GENERALIZES` | base → hidden | Links base data to hidden pattern |
| `AGGREGATES` | hidden → concept | Links hidden pattern to concept |
| `ABSTRACTS_TO` | layer N → layer N+1 | Generic upward abstraction (existing) |
| `INSTANTIATES` | layer N+1 → layer N | Inverse of ABSTRACTS_TO |

### Node Properties (Additions)

```cypher
// New properties on :MemoryNode
message_pass_embedding: [float64]  // Result of last message passing update
last_forward_pass: datetime        // Timestamp of last forward pass
last_backward_pass: datetime       // Timestamp of last backward pass
aggregation_count: int             // Number of children aggregated
stability_score: float64           // How stable this node's embedding is (0-1)
```

---

## Message Passing Algorithm

### Forward Pass (Base → Hidden → Concept)

Aggregates information from lower layers to higher layers, creating generalized representations.

```
FORWARD_PASS(space_id):
    # Phase 1: Update Hidden Layer from Base Data
    FOR each hidden_node h in Layer 1 WHERE h.space_id = space_id:
        base_neighbors = GET_NODES_VIA(h, GENERALIZES, direction=INCOMING)

        IF len(base_neighbors) == 0:
            CONTINUE

        # Weighted aggregation based on edge weights
        weights = [edge.weight for edge in GENERALIZES edges to h]
        embeddings = [n.embedding for n in base_neighbors]

        # GraphSAGE-style mean aggregation with attention
        aggregated = WEIGHTED_MEAN(embeddings, weights)

        # Combine with current embedding (prevents catastrophic forgetting)
        h.message_pass_embedding = σ(
            W_forward · CONCAT(h.embedding, aggregated)
        )
        h.last_forward_pass = NOW()
        h.aggregation_count = len(base_neighbors)

    # Phase 2: Update Concept Layer from Hidden
    FOR each concept_node c in Layer 2+ WHERE c.space_id = space_id:
        hidden_neighbors = GET_NODES_VIA(c, AGGREGATES, direction=INCOMING)

        IF len(hidden_neighbors) == 0:
            CONTINUE

        weights = [edge.weight for edge in AGGREGATES edges to c]
        embeddings = [h.message_pass_embedding OR h.embedding for h in hidden_neighbors]

        aggregated = WEIGHTED_MEAN(embeddings, weights)

        c.message_pass_embedding = σ(
            W_concept · CONCAT(c.embedding, aggregated)
        )
        c.last_forward_pass = NOW()
        c.aggregation_count = len(hidden_neighbors)
```

### Backward Pass (Concept → Hidden → Base)

Propagates feedback from concepts to hidden layers, enforcing generalization.

```
BACKWARD_PASS(space_id, trigger_reason):
    # trigger_reason: "new_data" | "concept_access" | "scheduled"

    FOR each hidden_node h in Layer 1 WHERE h.space_id = space_id:
        # Signal from parent concepts
        parent_concepts = GET_NODES_VIA(h, AGGREGATES, direction=OUTGOING)
        concept_signal = MEAN([c.message_pass_embedding OR c.embedding for c in parent_concepts])

        # Signal from child base data
        child_base = GET_NODES_VIA(h, GENERALIZES, direction=INCOMING)
        base_signal = MEAN([b.embedding for b in child_base])

        # Balance abstraction pressure (from concepts) with grounding (from base)
        IF concept_signal IS NOT NULL AND base_signal IS NOT NULL:
            h.message_pass_embedding = σ(
                W_back · CONCAT(h.embedding, concept_signal, base_signal)
            )

        h.last_backward_pass = NOW()

        # Hebbian reinforcement: strengthen edges to recently accessed children
        FOR each base_node b in child_base:
            IF b.last_accessed > (NOW() - 1 hour):
                edge = GET_EDGE(b, h, GENERALIZES)
                edge.weight = CLAMP(edge.weight * 1.05, 0, 1)  # 5% boost
```

### Activation Functions and Parameters

```go
// Configuration (environment variables)
const (
    // Forward pass
    MSG_PASS_ALPHA = 0.6  // Weight of current embedding vs aggregated
    MSG_PASS_BETA  = 0.4  // Weight of aggregated embedding

    // Backward pass
    MSG_PASS_CONCEPT_WEIGHT = 0.3  // Influence of concept signal
    MSG_PASS_BASE_WEIGHT    = 0.5  // Influence of base signal
    MSG_PASS_SELF_WEIGHT    = 0.2  // Influence of current embedding

    // Learning
    MSG_PASS_LEARNING_RATE = 0.1  // How much to update per pass
    MSG_PASS_DECAY         = 0.01 // Regularization decay
)

// Activation function: LeakyReLU for non-linearity
func leakyReLU(x float64) float64 {
    if x > 0 {
        return x
    }
    return 0.01 * x
}

// Vector combination with learned weights
func combineEmbeddings(current, aggregated []float64, alpha, beta float64) []float64 {
    result := make([]float64, len(current))
    for i := range current {
        result[i] = leakyReLU(alpha*current[i] + beta*aggregated[i])
    }
    return normalize(result)
}
```

---

## Hidden Node Creation (Clustering)

### Algorithm Selection: DBSCAN

DBSCAN (Density-Based Spatial Clustering) is preferred over k-means because:

- No need to specify number of clusters upfront
- Naturally handles varying cluster sizes
- Identifies noise points (outliers)
- Works well with embedding similarity as distance metric

### Adaptive Clustering Parameters (CRITICAL - Do Not Hardcode)

**Design Principle:** Constraints MUST loosen as layers increase to enable emergent concept formation.

```go
// Base parameters (configured via environment)
const (
    CLUSTER_EPS_BASE        = 0.10  // Base epsilon for L0→L1 clustering
    CLUSTER_MIN_SAMPLES_BASE = 3    // Base min samples
    CLUSTER_MAX_HIDDEN       = 100  // Max hidden nodes per run
)

// ADAPTIVE CALCULATION (in CreateConceptNodes):
func adaptiveParams(targetLayer int) (eps float64, minSamples int) {
    layerFactor := float64(targetLayer - 1)  // 1 for L2, 2 for L3, etc.

    // Epsilon GROWS with layer (looser clustering at higher abstraction)
    // L2: 1.4x, L3: 1.8x, L4: 2.2x, L5: 2.6x base epsilon
    eps = CLUSTER_EPS_BASE * (1.0 + 0.4*layerFactor)
    if eps > 0.6 {
        eps = 0.6  // Cap to maintain some semantic coherence
    }

    // MinSamples SHRINKS with layer (smaller emergent groups allowed)
    // L2: 3, L3: 2, L4: 2, L5: 2 (min 2)
    minSamples = CLUSTER_MIN_SAMPLES_BASE - int(layerFactor)
    if minSamples < 2 {
        minSamples = 2
    }

    return
}
```

### Layer-Specific Parameters

| Layer | Target | Epsilon | MinSamples | Rationale |
|-------|--------|---------|------------|-----------|
| L0→L1 | Hidden | 0.10 | 3 | Tight clustering for closely related code |
| L1→L2 | Concept | 0.14 | 3 | Moderate grouping of patterns |
| L2→L3 | Domain | 0.18 | 2 | Broader domain concepts |
| L3→L4 | Abstract | 0.22 | 2 | Abstract architectural patterns |
| L4→L5 | Emergent | 0.26 | 2 | Loose clustering for system-wide emergence |

### No Early Termination

The consolidation loop MUST try ALL 5 layers regardless of intermediate results:

```go
// In RunConsolidation and handleConsolidate:
maxLayers := 5
for targetLayer := 2; targetLayer <= maxLayers; targetLayer++ {
    conceptCreated, err := s.CreateConceptNodes(ctx, spaceID, targetLayer)
    // Don't break on zero - upper layers may still form clusters
    // due to adaptive (looser) constraints
    if conceptCreated > 0 {
        // Process results...
    }
    // Continue to next layer regardless
}
```

### MaxClusterSize (Relaxed)

Upper layers should NOT have artificially small cluster size limits:

```go
// OLD (too restrictive):
maxConceptSize := s.cfg.HiddenLayerMaxClusterSize / (targetLayer * 2)  // Shrinks aggressively

// NEW (allows broad concepts):
maxConceptSize := s.cfg.HiddenLayerMaxClusterSize
if targetLayer >= 4 {
    maxConceptSize = maxConceptSize * 3 / 4  // Only slight reduction at very high layers
}
```

### Clustering Process

```
CREATE_HIDDEN_NODES(space_id):
    # Step 1: Fetch base nodes without hidden parent
    orphan_base = QUERY(
        MATCH (b:MemoryNode {space_id: $space_id, layer: 0})
        WHERE NOT (b)-[:GENERALIZES]->(:MemoryNode {layer: 1})
        AND b.embedding IS NOT NULL
        RETURN b
    )

    IF len(orphan_base) < CLUSTER_MIN_SAMPLES:
        RETURN  # Not enough data to cluster

    # Step 2: Run DBSCAN on embeddings
    embeddings = [node.embedding for node in orphan_base]
    labels = DBSCAN(embeddings, eps=CLUSTER_EPS, min_samples=CLUSTER_MIN_SAMPLES)

    # Step 3: Create hidden nodes for each cluster
    clusters = GROUP_BY(orphan_base, labels)
    hidden_created = 0

    FOR cluster_id, members in clusters:
        IF cluster_id == -1:  # Noise points
            CONTINUE
        IF hidden_created >= CLUSTER_MAX_HIDDEN:
            BREAK

        # Compute centroid embedding
        centroid = MEAN([m.embedding for m in members])

        # Generate name via LLM (optional, can use placeholder)
        name = GENERATE_CLUSTER_NAME(members) OR f"Pattern-{cluster_id}"

        # Create hidden node
        hidden = CREATE(:MemoryNode {
            space_id: space_id,
            node_id: UUID(),
            layer: 1,
            role_type: 'hidden',
            name: name,
            embedding: centroid,
            message_pass_embedding: centroid,
            created_at: NOW(),
            aggregation_count: len(members)
        })

        # Create GENERALIZES edges from members to hidden
        FOR member in members:
            distance = 1 - COSINE_SIM(member.embedding, centroid)
            weight = 1 - distance  # Higher weight for closer nodes

            CREATE (member)-[:GENERALIZES {
                space_id: space_id,
                edge_id: UUID(),
                weight: weight,
                created_at: NOW()
            }]->(hidden)

        hidden_created++

    RETURN hidden_created
```

---

## Triggers and Scheduling

### Event Triggers

| Event | Action | Priority |
|-------|--------|----------|
| New base data ingested | Queue backward pass for affected hidden nodes | Medium |
| Concept accessed during retrieval | Backward pass for that concept's subtree | High |
| Hidden node accessed | Update Hebbian weights on edges | High |
| Manual trigger (CLI) | Full forward + backward pass | Low |

### Scheduled Jobs

| Job | Frequency | Description |
|-----|-----------|-------------|
| Clustering | Daily (2 AM) | Create hidden nodes from orphan base data |
| Forward Pass | Daily (3 AM) | Update all hidden/concept embeddings |
| Backward Pass | Daily (4 AM) | Propagate concept feedback |
| Stability Check | Weekly | Identify unstable hidden nodes for review |

### Job Configuration

```go
// Environment variables
HIDDEN_LAYER_CLUSTER_SCHEDULE  = "0 2 * * *"   // Cron: daily 2 AM
HIDDEN_LAYER_FORWARD_SCHEDULE  = "0 3 * * *"   // Cron: daily 3 AM
HIDDEN_LAYER_BACKWARD_SCHEDULE = "0 4 * * *"   // Cron: daily 4 AM
HIDDEN_LAYER_STABILITY_SCHEDULE = "0 5 * * 0" // Cron: weekly Sunday 5 AM
```

---

## Retrieval Integration

### Layer-Aware Retrieval

Modify retrieval to optionally traverse layer hierarchy:

```
RETRIEVE_WITH_LAYERS(space_id, query_embedding, opts):
    # Standard vector recall at all layers
    candidates = VECTOR_SEARCH(query_embedding, top_k=opts.candidate_k)

    IF opts.traverse_layers:
        # Expand to include parent/child nodes across layers
        FOR candidate in candidates:
            IF candidate.layer == 0:
                # Add hidden parents
                parents = GET_NODES_VIA(candidate, GENERALIZES, OUTGOING)
                candidates.extend(parents)

            IF candidate.layer == 1:
                # Add concept parents and base children
                parents = GET_NODES_VIA(candidate, AGGREGATES, OUTGOING)
                children = GET_NODES_VIA(candidate, GENERALIZES, INCOMING)
                candidates.extend(parents)
                candidates.extend(children)

            IF candidate.layer >= 2:
                # Add hidden children
                children = GET_NODES_VIA(candidate, AGGREGATES, INCOMING)
                candidates.extend(children)

    # Continue with spreading activation and scoring...
    RETURN standard_retrieval_pipeline(candidates, query_embedding)
```

### Scoring Adjustment for Layers

```go
// Add layer-based scoring component
func layerScore(node MemoryNode, queryContext QueryContext) float64 {
    // Prefer hidden layer for pattern matching
    if queryContext.Type == "pattern_search" && node.Layer == 1 {
        return 0.1  // Bonus for hidden layer
    }
    // Prefer concepts for rule lookup
    if queryContext.Type == "rule_lookup" && node.Layer >= 2 {
        return 0.1  // Bonus for concept layer
    }
    // Prefer base data for specific examples
    if queryContext.Type == "example_search" && node.Layer == 0 {
        return 0.1  // Bonus for base layer
    }
    return 0
}
```

---

## Implementation Plan

### Phase 1: Schema Changes (1-2 days)

1. Add new properties to MemoryNode schema
2. Add GENERALIZES relationship type
3. Update schema version and migrations
4. Add indexes for layer-based queries

### Phase 2: Clustering Service (2-3 days)

1. Implement DBSCAN in Go (or use existing library)
2. Create `internal/hidden/clustering.go`
3. Integrate with consolidate CLI
4. Add tests

### Phase 3: Message Passing Service (3-4 days)

1. Create `internal/hidden/message_passing.go`
2. Implement forward pass
3. Implement backward pass
4. Add weight matrices (fixed initial values)
5. Add tests with golden outputs

### Phase 4: Triggers and Scheduling (2-3 days)

1. Add event hooks to ingest pipeline
2. Add event hooks to retrieval pipeline
3. Implement scheduler for batch jobs
4. Add CLI flags for manual triggering

### Phase 5: Retrieval Integration (1-2 days)

1. Add layer traversal to retrieval service
2. Add layer-based scoring component
3. Update API to accept layer preferences
4. Add tests

### Phase 6: Monitoring and Tuning (Ongoing)

1. Add metrics for message passing
2. Add stability monitoring
3. Tune hyperparameters based on real usage
4. Document best practices

---

## Open Questions

1. **Weight matrix initialization**: Start with identity matrices or random initialization?
   - **Recommendation**: Identity with small random perturbation

2. **Embedding dimension**: Same dimension for all layers or compress in hidden?
   - **Recommendation**: Same dimension initially, consider compression later

3. **LLM for naming**: Use LLM to generate hidden node names or simple placeholders?
   - **Recommendation**: Placeholders initially, LLM naming as enhancement

4. **Backward pass frequency**: Every access or batched?
   - **Recommendation**: Batch for scheduled, immediate for high-priority events

---

## Testing Strategy

### Unit Tests

- Weighted mean aggregation
- Embedding combination
- DBSCAN clustering
- Edge weight updates

### Integration Tests

- Full forward pass on test graph
- Full backward pass on test graph
- Clustering creates expected hidden nodes
- Retrieval traverses layers correctly

### Golden Tests

- Fixed input graph → expected output embeddings
- Ensures deterministic behavior across refactors

---

## Expected Emergent Behavior

### Layer Specialization

As data accumulates, expect layers to specialize:

| Layer | Expected Content | Example |
|-------|-----------------|---------|
| L0 | Raw code elements | `AuthService.validateToken()`, `config.yaml`, `UserController` |
| L1 | Local patterns | "JWT validation pattern", "NestJS module structure" |
| L2 | Service-level concepts | "Authentication Service", "Data Transformation Pipeline" |
| L3 | Domain concepts | "Security Infrastructure", "Event Processing System" |
| L4 | Architectural patterns | "Cross-Cutting Concerns", "Microservice Communication" |
| L5 | System principles | "Defense in Depth", "Event-Driven Architecture" |

### Emergence Indicators

Monitor for these signs of healthy emergence:

1. **L4/L5 nodes appearing** - System is forming high-level abstractions
2. **Cross-domain connections** - Concepts linking unrelated modules
3. **Stable high-layer nodes** - Embeddings not changing rapidly
4. **Meaningful cluster names** - Patterns recognizable to humans

---

## Dynamic Edge and Node Types (L4-H4-L5)

### Design Philosophy

Upper layers (L4+) need **richer semantics** than lower layers. Rather than using fixed relationship types like `ASSOCIATED_WITH`, we **infer** relationship and concept types from structural analysis and embedding geometry.

### Dynamic Edge Types

| Type | Inference Criteria | Example |
|------|-------------------|---------|
| `ANALOGOUS_TO` | High cosine sim (≥0.7) + same layer | "Auth flow" ↔ "Payment flow" |
| `CONTRASTS_WITH` | Low sim (≤0.3) + high co-activation | "Sync" ↔ "Async" processing |
| `COMPOSES_WITH` | High co-activation + moderate sim | "Caching" + "Rate limiting" |
| `TENSIONS_WITH` | Frequently compared, opposing metrics | "Performance" ↔ "Maintainability" |
| `SPECIALIZES` | Lower layer + high similarity | "JWT Auth" → "Auth Pattern" |
| `GENERALIZES_TO` | Higher layer + high similarity | "Auth Pattern" → "Security" |
| `EMERGES_FROM` | L5 node ← multiple L4 patterns | Emergent formation marker |
| `BRIDGES` | Connects 3+ distinct domains | Cross-cutting connector |
| `INFLUENCES` | Default soft relationship | Moderate structural connection |

### Edge Type Inference Algorithm

```go
func InferEdgeType(source, target UpperLayerNode, coActivation float64) DynamicEdgeType {
    sim := cosineSimilarity(source.Embedding, target.Embedding)
    layerDist := abs(source.Layer - target.Layer)

    switch {
    case sim >= 0.7 && layerDist == 0:
        return EdgeAnalogous  // Same layer, very similar
    case sim <= 0.3 && coActivation >= 0.5:
        return EdgeContrasts  // Different but co-accessed
    case coActivation >= 0.5 && sim >= 0.4:
        return EdgeComposes   // Work together
    case layerDist > 0 && sim >= 0.5:
        if source.Layer > target.Layer {
            return EdgeGeneralizes
        }
        return EdgeSpecializes
    default:
        return EdgeInfluences
    }
}
```

### Dynamic Node Types

| Type | Inference Criteria | Meaning |
|------|-------------------|---------|
| `principle` | High stability + deep aggregation | Guiding architectural principle |
| `pattern` | Multiple children + high diversity | Recurring design pattern |
| `constraint` | Limiting relationships | Design constraint or rule |
| `bridge` | 3+ cross-domain links | Connects disparate domains |
| `hub` | High degree (≥10) | Central connecting concept |
| `emergent` | Low stability (<0.5) | Newly forming concept |
| `established` | High stability (≥0.8) | Mature, stable concept |
| `tension` | Represents tradeoffs | Design tradeoff node |
| `synthesis` | Combines multiple patterns | Unified abstraction |

### Node Type Inference Algorithm

```go
func InferNodeType(metrics NodeMetrics) DynamicNodeType {
    totalDegree := metrics.InDegree + metrics.OutDegree

    switch {
    case metrics.CrossDomainLinks >= 3:
        return NodeBridge
    case totalDegree >= 10:
        return NodeHub
    case metrics.StabilityScore >= 0.8 && metrics.AggregationDepth >= 2:
        return NodePrinciple
    case metrics.StabilityScore >= 0.8:
        return NodeEstablished
    case metrics.InDegree >= 3 && metrics.ChildDiversity >= 0.5:
        return NodePattern
    case metrics.StabilityScore < 0.5:
        return NodeEmergent
    default:
        return NodePattern
    }
}
```

### Implementation Files

- `internal/hidden/types.go`: Type definitions, thresholds, metrics
- `internal/hidden/service.go`: `InferEdgeType()`, `InferNodeType()`, `ClassifyUpperLayerNodes()`, `CreateDynamicEdges()`

---

## Future Work: Inter-Layer Hidden Aggregators

### Current Architecture

```
L0 ──→ L1 ──→ L2 ──→ L3 ──→ L4 ──→ L5
      ↑      ↑      ↑      ↑      ↑
   Cluster  Cluster Cluster Cluster Cluster
```

### Proposed Enhancement: Hidden Layers Between Concepts

```
L0 ─── H0 ──→ L1 ─── H1 ──→ L2 ─── H2 ──→ L3 ─── H3 ──→ L4 ─── H4 ──→ L5
       ↑            ↑            ↑            ↑            ↑
    Aggregate    Aggregate    Aggregate    Aggregate    Aggregate
    + Message    + Message    + Message    + Message    + Message
     Passing      Passing      Passing      Passing      Passing
```

Where `H_n` nodes are explicit message-passing aggregators that:

1. Receive forward signals from layer N
2. Send aggregated signals to layer N+1
3. Receive backward signals from layer N+1
4. Propagate refined signals back to layer N

This would enable:

- More nuanced information flow between layers
- Better gradient-like feedback propagation
- Distinct "compression" vs "concept" node types

**Status:** Planned for future implementation after validating adaptive clustering benefits.

---

## References

- Hamilton et al., "Inductive Representation Learning on Large Graphs" (GraphSAGE)
- Ester et al., "A Density-Based Algorithm for Discovering Clusters" (DBSCAN)
- Hebb, "The Organization of Behavior" (Hebbian Learning)
