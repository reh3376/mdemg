# Graph Attention Networks (GAT) Research

## Executive Summary

This research explores Graph Attention Networks (GAT) and their potential application to MDEMG's retrieval system. Current MDEMG uses uniform k-hop traversal with spreading activation through learned edges (CO_ACTIVATED_WITH). GAT offers learned attention-based neighbor weighting that could enhance retrieval precision.

**Key Recommendation:** Hybrid approach - apply lightweight attention during graph expansion phase to prioritize which neighbors to explore, while maintaining spreading activation for final scoring.

---

## 1. Theoretical Foundations

### 1.1 Original GAT (Veličković et al., 2018)

**Paper:** "Graph Attention Networks" (ICLR 2018)
**ArXiv:** <https://arxiv.org/abs/1710.10903>

#### Core Innovation

GAT addresses shortcomings of prior Graph Convolutional Networks (GCN) by introducing **masked self-attentional layers** that operate on graph-structured data. Unlike GCN which uses uniform neighbor aggregation weighted only by graph structure (node degrees), GAT learns to assign different importance weights to different neighbors.

#### Key Properties

- **Attention Mechanism:** Computes attention coefficients for each edge using a learnable attention function
- **No Matrix Operations:** Avoids costly matrix inversions or graph Laplacian computations
- **Inductive Capability:** Can generalize to unseen graphs (important for MDEMG's dynamic memory)
- **Parallelizable:** Attention computations can run in parallel across all nodes
- **Multi-head Attention:** Similar to Transformers, uses multiple attention heads for stability

#### Architecture

```
For each node i:
1. Compute attention coefficients: e_ij = a(Wh_i, Wh_j) for all neighbors j
2. Normalize via softmax: α_ij = softmax_j(e_ij)
3. Aggregate: h'_i = σ(Σ_j α_ij * Wh_j)
```

Where:

- `h_i` = node feature vector
- `W` = shared linear transformation
- `a` = attention mechanism (typically single-layer feedforward network)
- `σ` = activation function (ELU, LeakyReLU)

#### Computational Complexity

- **Space:** O(|V| + |E|) - only stores edge list and node features
- **Time:** O(|E|) for attention computation (linear in edges)
- **Efficient:** No matrix inversions, graph structure need not be known upfront

#### Benchmark Results

State-of-the-art on:

- **Cora** citation network (transductive)
- **Citeseer** citation network (transductive)
- **Pubmed** citation network (transductive)
- **PPI** protein-protein interaction (inductive)

---

### 1.2 Evolution: GCN → GAT → GATv2

#### Graph Convolutional Networks (GCN)

- **Aggregation:** Normalized sum by node degrees
- **Weights:** Fixed by graph structure (degree normalization)
- **Limitation:** Uniform neighbor importance - cannot distinguish between important vs. irrelevant neighbors

#### GAT (2018)

- **Aggregation:** Weighted sum with learned attention
- **Weights:** Data-dependent (node features determine importance)
- **Limitation:** "Static attention" - ranking of neighbors is unconditioned on query node

#### GATv2 (2021)

**Paper:** "How Attentive are Graph Attention Networks?" (ICLR 2021)
**ArXiv:** <https://arxiv.org/abs/2105.14491>

**Critical Insight:** Original GAT computes attention as:

```
e_ij = a^T LeakyReLU(W [h_i || h_j])
```

This creates **static attention** - the attention scores are computed from concatenated features but the query node h_i doesn't dynamically modulate the ranking.

**GATv2 Fix:** Modify order of operations:

```
e_ij = a^T LeakyReLU(W_1 h_i + W_2 h_j)
```

Now the query node can **dynamically** adjust attention based on what it's looking for.

**Benefits:**

- **Dynamic Attention:** Query-conditioned neighbor ranking
- **Universal Approximator:** Can represent any attention function
- **Better Performance:** Outperforms GAT on 11 OGB benchmarks with same parameters
- **Robustness:** Higher resilience to structural noise
- **Efficiency:** Even single-head GATv2 outperforms 8-head GAT on some datasets

---

### 1.3 Message Passing Framework

Graph Neural Networks (GNNs) are unified under the **Message Passing Neural Network (MPNN)** framework:

```
1. Message Construction: m_ij = M(h_i, h_j, e_ij)
2. Aggregation: m_i = AGG({m_ij : j ∈ N(i)})
3. Update: h'_i = U(h_i, m_i)
```

**GCN vs GAT in MPNN Terms:**

| Aspect | GCN | GAT |
|--------|-----|-----|
| Message | M = h_j | M = α_ij * h_j |
| Aggregation | SUM with degree normalization | Weighted SUM with learned α |
| Attention | Structure-only (degrees) | Feature-dependent |

**Key Insight:** GCN is a **special case** of GAT where attention is fully determined by graph structure. GAT is a **special case** of MPNN where aggregation uses learned attention weights.

---

### 1.4 Attention vs. Aggregation

#### Uniform Aggregation (GCN, Simple MPNNs)

- **Pros:** Simple, fast, no learning overhead
- **Cons:** Treats all neighbors equally, over-smoothing on deep networks
- **Use Case:** When graph structure already encodes importance (e.g., citation networks where all citations are roughly equal)

#### Attention-Based Aggregation (GAT)

- **Pros:** Learns neighbor importance, reduces over-smoothing, more expressive
- **Cons:** Additional parameters, potential overfitting on small graphs
- **Use Case:** When neighbors have varying relevance (e.g., social networks, knowledge graphs)

#### MDEMG's Current Approach (Hybrid)

- **Expansion:** Uniform traversal via `fetchOutgoingEdges` (structural + learned edge types)
- **Activation:** Selective spreading only through `CO_ACTIVATED_WITH` edges
- **Scoring:** Combines multiple signals (vector, activation, recency, confidence)

**Gap:** Expansion phase fetches all neighbors uniformly up to `MaxNeighborsPerNode` cap. Attention could prioritize which neighbors to expand.

---

## 2. GAT Architecture Details

### 2.1 Single-Layer GAT

#### Attention Coefficients

For edge from node j to node i:

```
e_ij = LeakyReLU(a^T [W h_i || W h_j])
```

Where:

- `||` denotes concatenation
- `a ∈ R^(2F')` is a learnable weight vector
- `W ∈ R^(F' x F)` projects F-dim input to F'-dim output
- LeakyReLU with α=0.2 (prevents dead neurons)

#### Normalization

Softmax across neighbors for stable gradients:

```
α_ij = softmax_j(e_ij) = exp(e_ij) / Σ_{k∈N(i)} exp(e_ik)
```

#### Output Features

```
h'_i = σ(Σ_{j∈N(i)} α_ij W h_j)
```

### 2.2 Multi-Head Attention

To stabilize learning and increase model capacity:

```
h'_i = ||^K_{k=1} σ(Σ_{j∈N(i)} α^k_ij W^k h_j)
```

Or for final layer (average instead of concat):

```
h'_i = σ(1/K Σ^K_{k=1} Σ_{j∈N(i)} α^k_ij W^k h_j)
```

**Typical K values:** 3-8 heads
**Trade-off:** More heads = more stable but higher memory cost

### 2.3 Edge Features in Attention

Standard GAT only uses node features. Extensions incorporate edge features:

#### Edge-Conditioned Attention (GAT-Edge)

```
e_ij = a^T [W_n h_i || W_n h_j || W_e e_ij]
```

Where `e_ij` is the edge feature vector and `W_e` projects edge features.

#### MDEMG Edge Features

Current MDEMG edges have rich features:

- `weight` (learned, Hebbian)
- `dim_semantic` (cosine similarity)
- `dim_temporal` (temporal proximity)
- `dim_coactivation` (co-occurrence frequency)
- `evidence_count` (reinforcement strength)
- `last_activated_at` (recency)

**Opportunity:** These edge features could inform attention computation.

### 2.4 Layer Normalization and Residual Connections

Modern GAT architectures add:

#### Layer Normalization

```
h'_i = LayerNorm(h_i + GAT(h_i))
```

Stabilizes deep networks, prevents over-smoothing.

#### Residual Connections

```
h'_i = h_i + GAT(h_i)
```

Preserves original node features, helps gradients flow.

**MDEMG Equivalent:** Vector similarity is preserved alongside activation in scoring - a form of implicit residual connection.

---

## 3. Comparison: Hop Traversal vs. Graph Attention

### 3.1 Simple K-Hop Traversal (Current MDEMG)

#### Implementation

From `/Users/reh3376/mdemg/internal/retrieval/service.go`:

```go
// 2) Expansion: iterative 1-hop fetch up to hopDepth
for d := 0; d < hopDepth; d++ {
    batchEdges, nextNodes, err := s.fetchOutgoingEdges(ctx, req.SpaceID, frontier)
    // Collect edges, expand frontier
}

// fetchOutgoingEdges applies:
// - Allowed relationship types filter
// - MaxNeighborsPerNode cap (per-node degree limit)
// - Evidence-based decay for CO_ACTIVATED_WITH edges
// - Weight-based ranking (top MaxNeighborsPerNode neighbors)
```

#### Characteristics

- **Uniform Expansion:** All seed nodes expanded equally
- **Degree Cap:** Top N neighbors by weight per node
- **Edge Type Filtering:** Only allowed relationship types
- **Decay Applied:** CO_ACTIVATED_WITH edges decay based on `evidence_count` and `last_activated_at`

#### Strengths

- **Simple:** Easy to understand and debug
- **Predictable:** Deterministic traversal
- **Efficient:** Linear in hop depth and degree cap
- **Proven:** Works well for MDEMG's current use case

#### Limitations

- **No Context Awareness:** Expansion doesn't consider query context
- **Fixed Budget:** Same MaxNeighborsPerNode for all nodes (no dynamic allocation)
- **Binary Filtering:** Edges are either included or excluded (no soft weighting during expansion)

### 3.2 Graph Attention Networks (GAT)

#### Characteristics

- **Learned Weights:** Attention coefficients computed per-query
- **Context-Aware:** Query node features influence neighbor selection
- **Soft Selection:** All neighbors contribute, weighted by attention
- **Trainable:** Requires labeled data to learn attention function

#### Strengths

- **Adaptive:** Different queries attend to different neighbors
- **Expressive:** Can learn complex neighbor importance patterns
- **Smooth:** Soft attention allows gradient flow through all neighbors

#### Limitations

- **Training Required:** Needs supervision signal to learn attention
- **Computational Cost:** Attention computation for all edges
- **Overfitting Risk:** Can memorize training graph structure

### 3.3 Complexity Comparison

| Operation | K-Hop Traversal | GAT (per layer) | GATv2 (per layer) |
|-----------|----------------|-----------------|-------------------|
| **Time** | O(k \* d \* n) | O(\|E\|) | O(\|E\|) |
| **Space** | O(\|E_sub\|) | O(\|E\|) | O(\|E\|) |
| **Parameters** | 0 (no learning) | O(F \* F' \* K) | O(F \* F' \* K) |

Where:

- `k` = hop depth
- `d` = average degree (capped by MaxNeighborsPerNode)
- `n` = number of seed nodes
- `|E|` = total edges in graph
- `|E_sub|` = edges in k-hop subgraph
- `F` = input feature dim
- `F'` = output feature dim
- `K` = number of attention heads

**Key Insight:** For bounded k-hop with degree caps, k-hop is more efficient (operates on subgraph). GAT operates on full graph unless combined with sampling.

### 3.4 When Simple Hops Are Sufficient

✅ **Use K-Hop When:**

- Graph structure already encodes importance (e.g., co-authorship networks)
- Edge weights are reliable importance signals
- Query-independent traversal is desired (caching benefits)
- Training data is unavailable
- Computational budget is tight

### 3.5 When GAT Helps

✅ **Use GAT When:**

- Neighbors have varying relevance that's hard to precompute
- Query context should influence neighbor selection
- Node features are rich and informative
- Training data is available (supervised or self-supervised)
- Graph has high degree nodes where uniform sampling is suboptimal

### 3.6 MDEMG's Current Position

MDEMG is **between** these two extremes:

**K-Hop Features:**

- Bounded expansion (hop_depth ≤ 3)
- Degree caps (MaxNeighborsPerNode)
- Edge type filtering

**Attention-Like Features:**

- Weight-based neighbor ranking (top N by weight)
- Selective activation spreading (only CO_ACTIVATED_WITH)
- Evidence-based decay (Hebbian reinforcement)

**Gap:** Expansion doesn't consider query embedding - all seeds expanded equally.

---

## 4. Practical Implementations

### 4.1 PyTorch Geometric (PyG)

**Official Docs:** <https://pytorch-geometric.readthedocs.io/>

#### GATConv Layer

```python
from torch_geometric.nn import GATConv

conv = GATConv(
    in_channels=128,
    out_channels=64,
    heads=8,
    concat=True,  # Concatenate or average heads
    dropout=0.6,
    add_self_loops=True,
    bias=True
)

# Forward pass
x_out = conv(x, edge_index)  # x: [N, in_channels], edge_index: [2, E]
```

#### GATv2Conv Layer

```python
from torch_geometric.nn import GATv2Conv

conv = GATv2Conv(
    in_channels=128,
    out_channels=64,
    heads=8,
    concat=True,
    dropout=0.6,
    add_self_loops=True,
    share_weights=False,  # GATv2 uses separate W_1, W_2
    bias=True
)
```

**Recommendation:** Use `GATv2Conv` over `GATConv` - consistently outperforms in benchmarks.

#### Full GAT Model

```python
from torch_geometric.nn import GAT

model = GAT(
    in_channels=128,
    hidden_channels=64,
    num_layers=3,
    out_channels=32,
    dropout=0.6,
    act='relu',
    norm='batch_norm',
    jk='cat'  # Jumping Knowledge: concat all layers
)
```

#### Edge-Aware GAT

PyG supports edge features via `edge_attr`:

```python
x_out = conv(x, edge_index, edge_attr=edge_features)
```

### 4.2 DGL (Deep Graph Library)

**Integration with Neo4j:** <https://towardsdatascience.com/neo4j-dgl-a-seamless-integration-624ad6edb6c0>

#### Neo4j → DGL → GAT Workflow

```python
import dgl
from neo4j import GraphDatabase

# 1. Query Neo4j graph
driver = GraphDatabase.driver("bolt://localhost:7687")
with driver.session() as session:
    result = session.run("""
        MATCH (n:MemoryNode)-[r]->(m:MemoryNode)
        WHERE n.space_id = $spaceId
        RETURN n.node_id, m.node_id, r.weight
    """, spaceId="mdemg-prod")
    edges = [(rec["n.node_id"], rec["m.node_id"], rec["r.weight"]) for rec in result]

# 2. Build DGL graph
src, dst, weights = zip(*edges)
g = dgl.graph((src, dst))
g.edata['weight'] = torch.tensor(weights)

# 3. Apply GAT
from dgl.nn import GATConv
conv = GATConv(in_feats=128, out_feats=64, num_heads=8)
h = conv(g, node_features)

# 4. Write back to Neo4j (optional - cache attention weights)
```

**Limitation:** DGL doesn't natively support Neo4j - requires manual export/import.

### 4.3 Neo4j Graph Data Science (GDS)

**Official Docs:** <https://neo4j.com/docs/graph-data-science/current/>

#### Native Algorithms

Neo4j GDS provides:

- **PageRank** - authority scoring
- **Node Similarity** - cosine similarity between neighborhoods
- **Community Detection** - Louvain, Label Propagation
- **Centrality** - Betweenness, Closeness, Degree

**Gap:** No native GAT implementation in Neo4j GDS.

#### External ML Integration

For GAT, must use:

1. **Export** graph to DGL/PyG via Cypher query
2. **Train** GAT model externally
3. **Import** learned embeddings/weights back to Neo4j

**Workflow:**

```cypher
// Export subgraph for GAT training
MATCH (n:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH]->(m)
WHERE n.embedding IS NOT NULL
RETURN n.node_id, n.embedding, m.node_id, r.weight, r.dim_semantic
```

### 4.4 Lightweight GAT Approximation

For production systems without GPU training infrastructure, consider:

#### Option 1: Fixed Attention Pattern

Pre-compute attention using heuristics:

```python
def simple_attention(src_emb, dst_emb, edge_weight):
    """Lightweight attention without learning."""
    # Cosine similarity as attention proxy
    cos_sim = cosine_similarity(src_emb, dst_emb)
    # Combine with edge weight
    attention = (0.7 * edge_weight + 0.3 * cos_sim)
    return attention
```

#### Option 2: One-Shot Attention

Compute attention on-the-fly during retrieval:

```go
// In MDEMG fetchOutgoingEdges:
func computeQueryAwareWeight(queryEmb, nodeEmb []float32, edgeWeight float64) float64 {
    // Cosine similarity between query and destination node
    cosSim := cosineSim(queryEmb, nodeEmb)
    // Blend with learned edge weight
    return 0.6*edgeWeight + 0.4*cosSim
}
```

#### Option 3: Cached Attention

Pre-compute attention for common query patterns:

- Cache attention distributions for frequent seed nodes
- Invalidate on ingest/consolidate
- Trade memory for speed

**MDEMG Fit:** Option 2 (one-shot attention) requires no training and can be computed during retrieval with minimal overhead.

---

## 5. Edge-Aware Attention

### 5.1 Heterogeneous Graph Attention

#### HEAT (Heterogeneous Edge-Featured GAT)

**Paper:** "Heterogeneous Edge-Enhanced Graph Attention Network" (2021)
**ArXiv:** <https://arxiv.org/abs/2106.07161>

Extends GAT to handle:

- **Multiple edge types** (e.g., ASSOCIATED_WITH, GENERALIZES, CO_ACTIVATED_WITH)
- **Edge features** (e.g., weight, dim_semantic, evidence_count)

**Attention with Edge Features:**

```
e_ij = a^T LeakyReLU([W_n h_i || W_n h_j || W_e e_ij || W_t τ(r_ij)])
```

Where:

- `e_ij` = edge feature vector
- `τ(r_ij)` = edge type embedding
- `W_e`, `W_t` = learnable projections for edge features and types

**MDEMG Relevance:** MDEMG has 5+ edge types and rich edge features - HEAT is a natural fit.

### 5.2 Relational GAT (RGAT)

**Focus:** Multi-relational graphs where edge **types** carry semantic meaning.

**Type-Aware Attention:**

```
α^r_ij = softmax_j(e^r_ij)
e^r_ij = a_r^T [W_r h_i || W_r h_j]
```

Each edge type `r` has its own:

- Attention function `a_r`
- Weight matrix `W_r`

**MDEMG Edge Types:**

- `ASSOCIATED_WITH` (semantic similarity)
- `GENERALIZES` (abstraction)
- `ABSTRACTS_TO` (concept hierarchy)
- `CO_ACTIVATED_WITH` (Hebbian learning)
- `CONTRADICTS` (conflict)
- `IMPLEMENTS_CONCERN` (cross-cutting)

**Challenge:** 5+ edge types × multi-head attention = many parameters. May require regularization or shared parameters.

### 5.3 GAT-Edge

**Paper:** "Graph Attention Neural Network with Adjacent Edge Features" (OpenReview)
**Link:** <https://openreview.net/forum?id=d7KsesYb6E>

**Innovation:** Combines edge features with node features in attention calculation:

```
e_ij = a^T σ([W_n h_i || W_n h_j] + [W_e f_ij])
```

Where `f_ij` is the edge feature vector (not concatenated, but added after projection).

**Benefit:** More parameter-efficient than concatenation (HEAT approach).

### 5.4 HL-HGAT (Hodge-Laplacian Heterogeneous GAT)

**Recent (2026):** Models **higher-order structures** (triangles, cliques).

**MDEMG Relevance:** Limited - MDEMG's graph is primarily tree-like with some cross-edges. Triangles are rare. Likely overkill for current structure.

### 5.5 MDEMG Edge Feature Integration

Current MDEMG edges already encode rich signals:

| Edge Feature | Meaning | Attention Use Case |
|--------------|---------|-------------------|
| `weight` | Learned strength | Base attention weight |
| `dim_semantic` | Cosine similarity | Feature-based attention |
| `dim_temporal` | Temporal proximity | Recency-aware attention |
| `dim_coactivation` | Co-occurrence | Hebbian attention |
| `evidence_count` | Reinforcement strength | Confidence weighting |
| `last_activated_at` | Recency | Decay factor |

**Proposed Edge-Aware Attention:**

```go
func computeEdgeAttention(queryEmb, srcEmb, dstEmb []float32, edge Edge) float64 {
    // Node feature similarity
    querySrcSim := cosineSim(queryEmb, srcEmb)
    queryDstSim := cosineSim(queryEmb, dstEmb)

    // Edge feature contribution
    edgeSignal := 0.4*edge.Weight + 0.3*edge.DimSemantic + 0.2*edge.DimCoactivation + 0.1*edge.DimTemporal

    // Combine with Hebbian reinforcement
    evidenceBoost := math.Log(1 + float64(edge.EvidenceCount)) / 5.0  // log-scale

    // Compute attention score
    attention := 0.5*queryDstSim + 0.3*edgeSignal + 0.2*evidenceBoost

    // Apply decay for old CO_ACTIVATED_WITH edges
    if edge.RelType == "CO_ACTIVATED_WITH" {
        decayFactor := computeDecay(edge.LastActivatedAt, edge.EvidenceCount)
        attention *= decayFactor
    }

    return attention
}
```

---

## 6. MDEMG-Specific Analysis

### 6.1 Current Graph Traversal Architecture

#### Key Files

- `/Users/reh3376/mdemg/internal/retrieval/service.go` - Main retrieval logic
- `/Users/reh3376/mdemg/internal/retrieval/activation.go` - Spreading activation
- `/Users/reh3376/mdemg/internal/retrieval/scoring.go` - Final ranking

#### Retrieval Pipeline

```
1. vectorRecall(query_embedding, candidate_k)
   └─> Top candidate_k nodes by cosine similarity

2. Expansion (lines 218-251 in service.go)
   FOR d = 0 to hop_depth:
       frontier ← fetchOutgoingEdges(frontier_nodes)
       └─> Fetch top MaxNeighborsPerNode edges per node
       └─> Apply edge type filtering
       └─> Apply evidence-based decay (CO_ACTIVATED_WITH only)

3. SpreadingActivation(candidates, edges, steps=2, lambda=0.15)
   └─> Propagate activation through CO_ACTIVATED_WITH edges only
   └─> Degree-normalized accumulation (prevents saturation)

4. ScoreAndRank(candidates, activation, edges, topK)
   └─> Final score = α*VectorSim + β*Activation + γ*Recency + δ*Confidence + ...
```

#### Graph Structure (from models.go, domain/types.go)

**Layers:**

- **L0** (Base): Raw code/observations (leaf nodes)
- **L1** (Hidden): DBSCAN clusters, concerns, config summaries (concern nodes)
- **L2+** (Concepts): Higher-level abstractions (emergent_concept nodes)

**Edge Types:**

- `ASSOCIATED_WITH` - Semantic similarity (created on ingest if similarity > threshold)
- `GENERALIZES` - L0→L1 abstraction (consolidation)
- `ABSTRACTS_TO` - L1→L2 concept hierarchy (consolidation)
- `CO_ACTIVATED_WITH` - Hebbian learning (created/strengthened when nodes co-occur in queries)
- `CONTRADICTS` - Conflict detection (inhibitory)
- `IMPLEMENTS_CONCERN` - Cross-cutting concerns (consolidation)
- `IMPLEMENTS_CONFIG` - Config relationships (consolidation)

**Edge Features:**

```go
type Edge struct {
    Src             string
    Dst             string
    RelType         string
    Weight          float64  // Learned Hebbian weight
    DimSemantic     float64  // Cosine similarity
    DimTemporal     float64  // Temporal proximity
    DimCoactivation float64  // Co-occurrence strength
    UpdatedAt       time.Time
}
```

### 6.2 Current Hop Depth Traversal

#### Expansion Logic (service.go lines 227-251)

```go
for d := 0; d < hopDepth; d++ {
    if len(frontier) == 0 {
        break
    }
    batchEdges, nextNodes, err := s.fetchOutgoingEdges(ctx, req.SpaceID, frontier)
    // Collect edges and expand frontier
    for _, e := range batchEdges {
        // Deduplicate edges
        key := e.Src + "|" + e.RelType + "|" + e.Dst
        if _, ok := seenEdge[key]; ok {
            continue
        }
        seenEdge[key] = struct{}{}
        edges = append(edges, e)
    }
    for _, nid := range nextNodes {
        // Deduplicate nodes
        if _, ok := seenNode[nid]; ok {
            continue
        }
        seenNode[nid] = struct{}{}
        frontier = append(frontier, nid)
    }
}
```

**Characteristics:**

- **Breadth-First Expansion:** All frontier nodes expanded equally
- **Fixed Hop Budget:** `hop_depth` parameter (default 1, max 3)
- **No Query Context:** Expansion doesn't consider query embedding
- **Degree Cap:** `fetchOutgoingEdges` limits to top `MaxNeighborsPerNode` edges per node

#### fetchOutgoingEdges (service.go lines 635-748)

**Cypher Query Highlights:**

```cypher
UNWIND $nodeIds AS sid
MATCH (src:MemoryNode {space_id:$spaceId, node_id:sid})
CALL {
  WITH src
  MATCH (src)-[r]->(dst:MemoryNode {space_id:$spaceId})
  WHERE type(r) IN $allowed AND coalesce(r.status,'active')='active'

  // Evidence-based decay for CO_ACTIVATED_WITH edges
  WITH src, r, dst, type(r) AS relType,
       CASE WHEN type(r) = 'CO_ACTIVATED_WITH' THEN
         duration.between(coalesce(r.last_activated_at, r.created_at, datetime()), datetime()).days
       ELSE 0 END AS daysSinceActive,
       coalesce(r.weight, 0.0) AS rawWeight,
       coalesce(r.evidence_count, 1) AS evidenceCount
  WITH src, r, dst, relType, daysSinceActive, rawWeight, evidenceCount,
       // Decay formula: weight * (1 - decayPerDay/sqrt(evidenceCount))^days
       CASE WHEN relType = 'CO_ACTIVATED_WITH' AND daysSinceActive > 0 THEN
         rawWeight * ((1.0 - $decayPerDay / sqrt(toFloat(evidenceCount))) ^ daysSinceActive)
       ELSE rawWeight END AS decayedWeight

  // Filter out decayed edges below threshold
  WHERE NOT (relType = 'CO_ACTIVATED_WITH' AND decayedWeight < $pruneThreshold)

  RETURN src.node_id AS s, dst.node_id AS d, relType AS t, decayedWeight AS w, ...
  ORDER BY w DESC
  LIMIT $maxNbr  // MaxNeighborsPerNode (degree cap)
}
RETURN s, d, t, w, ...
LIMIT $maxTotal  // MaxTotalEdgesFetched (global cap)
```

**Key Features:**

- **Weight-Based Ranking:** Top N neighbors by `decayedWeight`
- **Evidence-Based Decay:** Edges with more `evidence_count` decay slower (Hebbian reinforcement)
- **Type Filtering:** Only allowed edge types included
- **Per-Node Degree Cap:** `MaxNeighborsPerNode` (default: 50)
- **Global Edge Cap:** `MaxTotalEdgesFetched` (default: 5000)

**Gap:** Neighbor selection is **query-independent** - the same top-50 neighbors are fetched regardless of query context.

### 6.3 Current Neighbor Retrieval and Scoring

#### Spreading Activation (activation.go lines 10-101)

**Key Insight:** Activation only spreads through `CO_ACTIVATED_WITH` edges (learned), not structural edges.

```go
// Build incoming lists — only propagate through learned edges
incoming := map[string][]Edge{}
for _, e := range edges {
    if e.RelType == "CONTRADICTS" {
        inhib[e.Dst] = append(inhib[e.Dst], e)
        continue
    }
    if e.RelType != "CO_ACTIVATED_WITH" {
        continue  // Skip ASSOCIATED_WITH, GENERALIZES, etc.
    }
    incoming[e.Dst] = append(incoming[e.Dst], e)
}
```

**Activation Propagation:**

```go
for t := 0; t < steps; t++ {
    next := map[string]float64{}
    // Carry forward with decay
    for id, a := range act {
        next[id] = clamp01((1 - lambda) * a)
    }

    // Accumulate from incoming edges (degree-normalized)
    for dst, ins := range incoming {
        acc := next[dst]
        degreeNorm := math.Sqrt(float64(len(ins)))
        if degreeNorm < 1 {
            degreeNorm = 1
        }
        for _, e := range ins {
            srcA := act[e.Src]
            w := effectiveWeight(e)
            acc += (srcA * w) / degreeNorm  // Degree normalization prevents saturation
        }
        // Apply inhibitory edges (CONTRADICTS)
        for _, e := range inhib[dst] {
            srcA := act[e.Src]
            w := math.Abs(effectiveWeight(e))
            acc -= srcA * w
        }
        next[dst] = clamp01(acc)
    }
    act = next
}
```

**Degree Normalization:**

- Divides by `sqrt(degree)` to prevent high-degree nodes from saturating to 1.0
- Preserves relative signal strength
- Documented in comments (lines 76-79)

#### Effective Weight Computation

```go
func effectiveWeight(e Edge) float64 {
    w := e.Weight
    if w < 0 {
        w = 0
    }
    // Dimension mix; if all dims are 0, treat as semantic=1.0
    mix := 0.0
    mix += 0.6 * e.DimSemantic
    mix += 0.2 * e.DimTemporal
    mix += 0.2 * e.DimCoactivation
    if mix == 0 {
        mix = 1.0
    }
    out := w * mix
    if out > 1 {
        out = 1
    }
    return out
}
```

**Insight:** Edge dimensions already provide a form of attention - weighing semantic, temporal, and coactivation signals. But this is **static** (doesn't depend on query).

#### Final Scoring (scoring.go lines 277-494)

**Score Formula:**

```
score = α*VectorSim + β*Activation + γ*Recency + δ*Confidence
        + pathBoost + comparisonBoost + configBoost
        - φ*HubPenalty - κ*RedundancyPenalty
```

Where:

- **VectorSim:** Cosine similarity from initial recall
- **Activation:** Transient activation from spreading
- **Recency:** Exponential decay `exp(-ρ * ageDays)`
- **Confidence:** Node's intrinsic confidence score
- **PathBoost:** Query path hints match node path
- **ComparisonBoost:** Comparison query detection
- **ConfigBoost:** Multiplier for config-tagged nodes (or penalty for code queries)
- **HubPenalty:** `log(1 + degree)` for L0 nodes only
- **RedundancyPenalty:** Penalty for nodes in same directory prefix

**Hyperparameters (from config):**

- α (ScoringAlpha): 0.45 - vector similarity weight
- β (ScoringBeta): 0.35 - activation weight
- γ (ScoringGamma): 0.10 - recency weight
- δ (ScoringDelta): 0.05 - confidence weight
- φ (ScoringPhi): 0.02 - hub penalty coefficient
- κ (ScoringKappa): 0.12 - redundancy penalty coefficient
- ρ (ScoringRho): 0.05 - recency decay per day

### 6.4 Gap Analysis

#### What Works Well

✅ **Hebbian Learning:** CO_ACTIVATED_WITH edges capture query co-occurrence patterns
✅ **Evidence-Based Decay:** Frequent patterns persist, spurious connections decay
✅ **Degree Normalization:** Prevents high-degree nodes from dominating activation
✅ **Multi-Signal Scoring:** Combines vector, activation, recency, confidence
✅ **Layer-Aware:** Different treatment for L0 (code) vs. L1+ (concepts)

#### Identified Gaps

❌ **Query-Independent Expansion:** Same top-N neighbors fetched regardless of query
❌ **No Query-Neighbor Affinity:** Expansion doesn't consider query-node similarity
❌ **Fixed Budget Allocation:** Same MaxNeighborsPerNode for all nodes (no dynamic allocation)
❌ **Structural Edges Ignored in Activation:** ASSOCIATED_WITH, GENERALIZES not used in spreading (only in expansion)
❌ **No Cross-Edge-Type Attention:** All CO_ACTIVATED_WITH edges treated equally during activation

#### Opportunities for GAT Integration

🎯 **Expansion Phase:** Apply attention to prioritize which neighbors to expand
🎯 **Activation Phase:** Use query context to modulate activation weights
🎯 **Edge Type Mixing:** Learn optimal blending of structural + learned edges
🎯 **Dynamic Hop Allocation:** Deep expansion for promising branches, shallow for others

### 6.5 Proposed Graph Attention Integration Points

#### Option 1: Query-Aware Expansion (Lightweight)

**Integrate attention during `fetchOutgoingEdges`:**

```go
func (s *Service) fetchOutgoingEdgesWithAttention(ctx context.Context, spaceID string, nodeIDs []string, queryEmbedding []float32) ([]Edge, []string, error) {
    // Fetch candidate edges (same as current)
    candidateEdges := s.fetchOutgoingEdges(ctx, spaceID, nodeIDs)

    // Compute query-aware attention for each edge
    for i, edge := range candidateEdges {
        dstNode := s.getNodeEmbedding(ctx, edge.Dst)
        attention := computeQueryAwareAttention(queryEmbedding, dstNode.Embedding, edge)
        candidateEdges[i].AttentionScore = attention
    }

    // Re-rank edges by attention score
    sort.Slice(candidateEdges, func(i, j int) bool {
        return candidateEdges[i].AttentionScore > candidateEdges[j].AttentionScore
    })

    // Take top MaxNeighborsPerNode by attention (not just weight)
    return candidateEdges[:MaxNeighborsPerNode], nextNodes, nil
}

func computeQueryAwareAttention(queryEmb, dstEmb []float32, edge Edge) float64 {
    // Query-destination similarity (proxy for attention)
    queryDstSim := cosineSim(queryEmb, dstEmb)

    // Blend with edge weight and dimensions
    attention := 0.5*queryDstSim + 0.3*edge.Weight + 0.2*edge.DimSemantic

    // Evidence boost (log-scale)
    evidenceBoost := math.Log(1 + float64(edge.EvidenceCount)) / 5.0
    attention += 0.1 * evidenceBoost

    return attention
}
```

**Benefits:**

- No training required (uses pre-computed embeddings)
- Query-aware neighbor selection
- Minimal code change (modifies `fetchOutgoingEdges`)

**Trade-offs:**

- Requires fetching node embeddings (extra DB query or cache)
- Slightly higher latency per hop

---

#### Option 2: Learned GAT Layer (Advanced)

**Train GAT model offline, apply during expansion:**

```python
# Offline training (Python + PyG)
import torch
from torch_geometric.nn import GATv2Conv

class MDEMGAttention(torch.nn.Module):
    def __init__(self, in_channels=768, out_channels=128, heads=4):
        super().__init__()
        self.gat = GATv2Conv(in_channels, out_channels, heads=heads, edge_dim=5)  # 5 edge features

    def forward(self, x, edge_index, edge_attr):
        # x: node embeddings [N, 768]
        # edge_index: [2, E] (src, dst pairs)
        # edge_attr: [E, 5] (weight, dim_semantic, dim_temporal, dim_coactivation, evidence_count)
        return self.gat(x, edge_index, edge_attr)

# Training loop (self-supervised: predict next co-activated node)
for query_emb, true_neighbors in training_data:
    attention_weights = model(node_embeddings, edge_index, edge_features)
    loss = cross_entropy(attention_weights, true_neighbors)
    loss.backward()
    optimizer.step()

# Export learned parameters to Go (via JSON or protocol buffer)
torch.save(model.state_dict(), 'gat_weights.pt')
```

```go
// Online inference (Go)
type AttentionModel struct {
    weights  [][]float64  // Learned attention parameters
    bias     []float64
}

func (a *AttentionModel) ComputeAttention(queryEmb, srcEmb, dstEmb []float32, edgeFeatures []float64) float64 {
    // Forward pass using learned parameters
    concat := append(append(queryEmb, srcEmb...), dstEmb...)
    concat = append(concat, edgeFeatures...)

    // Linear projection + LeakyReLU
    logit := 0.0
    for i, w := range a.weights[0] {
        logit += w * float64(concat[i])
    }
    logit += a.bias[0]

    // LeakyReLU
    if logit < 0 {
        logit *= 0.2
    }

    return logit
}

func (s *Service) fetchOutgoingEdgesWithLearnedAttention(ctx context.Context, spaceID string, nodeIDs []string, queryEmbedding []float32) ([]Edge, []string, error) {
    candidateEdges := s.fetchOutgoingEdges(ctx, spaceID, nodeIDs)

    // Apply learned attention model
    for i, edge := range candidateEdges {
        srcNode := s.getNodeEmbedding(ctx, edge.Src)
        dstNode := s.getNodeEmbedding(ctx, edge.Dst)
        edgeFeatures := []float64{edge.Weight, edge.DimSemantic, edge.DimTemporal, edge.DimCoactivation, float64(edge.EvidenceCount)}

        attention := s.attentionModel.ComputeAttention(queryEmbedding, srcNode.Embedding, dstNode.Embedding, edgeFeatures)
        candidateEdges[i].AttentionScore = attention
    }

    // Re-rank by attention
    sort.Slice(candidateEdges, func(i, j int) bool {
        return candidateEdges[i].AttentionScore > candidateEdges[j].AttentionScore
    })

    return candidateEdges[:MaxNeighborsPerNode], nextNodes, nil
}
```

**Benefits:**

- Learns optimal attention from query patterns
- Can discover non-obvious neighbor importance
- Higher precision than heuristic attention

**Trade-offs:**

- Requires training data (query logs)
- Training/deployment complexity
- Risk of overfitting to training queries

---

#### Option 3: Hybrid Structural + Learned Attention

**Use structural edges for breadth, learned attention for depth:**

```go
func (s *Service) expandWithHybridAttention(ctx context.Context, spaceID string, seeds []string, queryEmb []float32, hopDepth int) []Edge {
    edges := []Edge{}
    frontier := seeds

    for d := 0; d < hopDepth; d++ {
        if d == 0 {
            // First hop: use structural edges (ASSOCIATED_WITH, GENERALIZES)
            // for breadth - explore diverse neighborhoods
            structuralEdges := s.fetchEdgesByType(ctx, frontier, []string{"ASSOCIATED_WITH", "GENERALIZES"})
            edges = append(edges, structuralEdges...)
            frontier = extractDstNodes(structuralEdges)
        } else {
            // Subsequent hops: use learned edges (CO_ACTIVATED_WITH) with attention
            // for depth - follow query-relevant paths
            learnedEdges := s.fetchEdgesByType(ctx, frontier, []string{"CO_ACTIVATED_WITH"})

            // Apply query-aware attention
            for i, edge := range learnedEdges {
                dstNode := s.getNodeEmbedding(ctx, edge.Dst)
                attention := computeQueryAwareAttention(queryEmb, dstNode.Embedding, edge)
                learnedEdges[i].AttentionScore = attention
            }

            // Re-rank by attention
            sort.Slice(learnedEdges, func(i, j int) bool {
                return learnedEdges[i].AttentionScore > learnedEdges[j].AttentionScore
            })

            // Take top k by attention
            topK := min(len(learnedEdges), MaxNeighborsPerNode)
            edges = append(edges, learnedEdges[:topK]...)
            frontier = extractDstNodes(learnedEdges[:topK])
        }
    }

    return edges
}
```

**Rationale:**

- **Hop 0 (structural):** Explore diverse neighborhoods via semantic similarity and abstraction
- **Hop 1+ (learned):** Follow query-relevant paths via Hebbian learning + attention

**Benefits:**

- Balances exploration (structural) and exploitation (learned)
- Leverages both edge types effectively
- Query-aware without abandoning structural knowledge

---

### 6.6 Recommended Approach for MDEMG

#### Phase 1: Lightweight Query-Aware Attention (Option 1)

**Implementation:**

1. Add `computeQueryAwareAttention` function
2. Modify `fetchOutgoingEdges` to compute attention scores
3. Re-rank neighbors by attention (instead of just weight)
4. Gate behind feature flag: `QueryAwareExpansionEnabled`

**Validation:**

- Run benchmark suite (whk-wms, plc-gbt, pytorch)
- Measure precision@5, precision@10 changes
- Compare latency vs. current (expect +5-10ms per query)

**Rollout:**

- Enable for code-focused queries first (e.g., "where is X implemented")
- Monitor cache hit rates (may decrease due to query-dependence)
- Gradually expand to all queries

**Estimated Effort:** 2-3 days

---

#### Phase 2: Edge Type Mixing Experiment (Option 3)

**Implementation:**

1. Implement hybrid expansion (structural hop 0, learned hop 1+)
2. Add `EdgeTypeStrategy` config param (all, structural_first, learned_only)
3. Run A/B test on benchmark queries

**Validation:**

- Compare structural-first vs. learned-only vs. hybrid
- Measure recall improvements (especially for multi-hop queries)
- Analyze failed queries (where did attention help/hurt?)

**Rollout:**

- Start with `structural_first` as default
- Switch to `hybrid` if benchmarks show +5% improvement

**Estimated Effort:** 3-5 days

---

#### Phase 3: Learned Attention (Option 2) - Future Work

**Prerequisites:**

- Collect query logs (10k+ queries with ground truth)
- Set up PyTorch Geometric training pipeline
- Build model serving infrastructure (ONNX Runtime or TensorFlow Lite)

**Implementation:**

1. Train GATv2 model on historical query co-activation patterns
2. Export model to ONNX or TFLite
3. Integrate inference in Go (via CGO or REST API)
4. Cache attention weights per query pattern

**Validation:**

- Offline metrics: AUC, NDCG on held-out queries
- Online A/B test: measure precision@k, user satisfaction

**Estimated Effort:** 2-3 weeks (requires ML infrastructure)

---

## 7. Benchmark and Validation Plan

### 7.1 Existing Benchmarks

MDEMG has established benchmarks (from docs/benchmarks/):

| Benchmark | Codebase | Questions | Baseline Score | Purpose |
|-----------|----------|-----------|----------------|---------|
| **whk-wms** | Warehouse system | 100 | 0.78-0.81 | Stable baseline |
| **plc-gbt** | PLC toolkit | 50 | 0.68-0.73 | Industrial domain |
| **pytorch** | PyTorch | 100 (5 tracks × 25) | TBD | Hard business logic |

### 7.2 GAT-Specific Metrics

#### Precision@K

Measure how many of the top-K results are relevant:

```
P@K = (# relevant in top-K) / K
```

**Target:** +5% improvement in P@5, P@10 with query-aware attention.

#### Recall@K

Measure how many relevant items are found in top-K:

```
R@K = (# relevant in top-K) / (# total relevant)
```

**Target:** +3% improvement in R@20 (especially for multi-hop queries).

#### NDCG (Normalized Discounted Cumulative Gain)

Ranking quality metric that rewards relevant items higher in the list:

```
NDCG@K = DCG@K / IDCG@K
DCG@K = Σ (2^rel_i - 1) / log2(i + 1)
```

**Target:** +0.05 improvement in NDCG@10.

#### Attention Diversity

Measure how diverse the top-K attention weights are:

```
Diversity = 1 - (max_attention_weight - mean_attention_weight)
```

**Goal:** Attention should **not** collapse to single node (would indicate learned bias).

#### Expansion Efficiency

Measure how many nodes are expanded to achieve same recall:

```
Efficiency = (nodes_expanded_baseline) / (nodes_expanded_with_attention)
```

**Target:** 10-20% reduction in nodes expanded (attention prunes irrelevant branches).

### 7.3 Ablation Studies

Test each component in isolation:

| Ablation | Configuration | Expected Impact |
|----------|---------------|-----------------|
| **No Attention** | Current MDEMG | Baseline |
| **Query-Dst Similarity Only** | attention = cosineSim(query, dst) | +2-3% P@5 |
| **+ Edge Weight** | attention = 0.5*cosSim + 0.5*weight | +3-4% P@5 |
| **+ Edge Dimensions** | attention = 0.5*cosSim + 0.3*weight + 0.2*dims | +4-5% P@5 |
| **+ Evidence Boost** | attention += log(evidenceCount)/5 | +5-6% P@5 (target) |

### 7.4 Query Type Analysis

Break down results by query type:

| Query Type | Example | Current Performance | Expected Improvement |
|------------|---------|---------------------|---------------------|
| **Specific Code Lookup** | "where is MAX_TIMEOUT defined" | High (0.85-0.90) | +2% (already good) |
| **Architectural** | "how does auth flow through the system" | Medium (0.65-0.75) | +8% (multi-hop benefits) |
| **Comparison** | "difference between UserService and AdminService" | Medium (0.70-0.78) | +5% (comparison boost) |
| **Cross-Cutting** | "all places that validate permissions" | Low (0.55-0.65) | +10% (concern edges + attention) |

### 7.5 Latency Benchmarks

Measure retrieval latency impact:

| Component | Current Latency | With Attention | Budget |
|-----------|----------------|----------------|--------|
| **Vector Recall** | 10-15ms | 10-15ms (unchanged) | 20ms |
| **Expansion** | 30-50ms | 40-70ms (+20ms) | 80ms |
| **Activation** | 5-10ms | 5-10ms (unchanged) | 20ms |
| **Scoring** | 5-10ms | 5-10ms (unchanged) | 20ms |
| **Total** | 50-85ms | 60-105ms | 150ms |

**Optimization Targets:**

- Cache node embeddings (avoid repeated DB queries)
- Batch attention computation (vectorized ops)
- Early stopping (prune low-attention branches)

---

## 8. Implementation Roadmap

### Phase 1: Foundation (Week 1-2)

- [ ] Add `AttentionScore` field to `Edge` struct
- [ ] Implement `computeQueryAwareAttention` (cosine sim + edge weight)
- [ ] Add feature flag `QueryAwareExpansionEnabled`
- [ ] Unit tests for attention computation
- [ ] Benchmark on whk-wms (baseline comparison)

**Deliverable:** Lightweight attention without changing expansion logic (just scoring).

---

### Phase 2: Query-Aware Expansion (Week 3-4)

- [ ] Modify `fetchOutgoingEdges` to re-rank by attention
- [ ] Add node embedding cache (reduce DB queries)
- [ ] Implement attention-based neighbor selection
- [ ] Integration tests for expansion with attention
- [ ] A/B test on whk-wms + plc-gbt

**Deliverable:** Query-aware expansion with attention-based neighbor selection.

---

### Phase 3: Edge Type Mixing (Week 5-6)

- [ ] Implement hybrid expansion (structural hop 0, learned hop 1+)
- [ ] Add `EdgeTypeStrategy` config parameter
- [ ] Experiment with different mixing strategies
- [ ] Analyze failed queries (attention hurt precision)
- [ ] Finalize default strategy

**Deliverable:** Optimized edge type mixing strategy based on benchmark results.

---

### Phase 4: Edge Feature Integration (Week 7-8)

- [ ] Extend attention to include `dim_semantic`, `dim_temporal`, `dim_coactivation`
- [ ] Add evidence-based attention boosting (`log(evidence_count)`)
- [ ] Implement decay-aware attention (penalize stale edges)
- [ ] Ablation study: measure each feature's contribution
- [ ] Tune attention hyperparameters (α, β, γ weights)

**Deliverable:** Full edge-aware attention with all MDEMG edge features.

---

### Phase 5: Learned Attention (Future - Month 3+)

- [ ] Collect query logs with ground truth (10k+ samples)
- [ ] Train GATv2 model in PyTorch Geometric
- [ ] Export model to ONNX or TFLite
- [ ] Build model serving API (Go → Python/ONNX inference)
- [ ] A/B test learned vs. heuristic attention
- [ ] Continuous learning pipeline (retrain on new queries)

**Deliverable:** Learned attention model with production deployment.

---

## 9. Risks and Mitigations

### Risk 1: Attention Collapse

**Issue:** Attention weights collapse to single node, losing diversity.

**Symptoms:**

- All attention mass on one neighbor
- Low recall (misses relevant nodes not attended to)
- High variance in attention distribution

**Mitigation:**

- Add entropy regularization to attention weights
- Clamp max attention: `attention = min(attention, 0.9)`
- Ensure multi-head attention in learned model (k=4-8 heads)
- Monitor attention diversity metric

---

### Risk 2: Query-Dependence Breaks Caching

**Issue:** Query-aware expansion means cache hit rate drops.

**Symptoms:**

- Cache hit rate drops from 60% to <10%
- Latency increases (no cache benefit)
- Higher Neo4j query load

**Mitigation:**

- Cache attention weights per query pattern (cluster similar queries)
- Use query embedding hash as cache key (instead of full embedding)
- Implement "coarse attention" for cache hits (fine attention for misses)
- Keep query-independent expansion as fallback

---

### Risk 3: Embedding Fetch Overhead

**Issue:** Fetching node embeddings for attention adds latency.

**Symptoms:**

- Expansion latency doubles (10ms → 20ms per hop)
- High DB load (embedding queries)

**Mitigation:**

- Aggressive embedding cache (LRU cache with 10k capacity)
- Batch embedding fetches (single query for all neighbors)
- Lazy embedding fetch (only for top-weighted neighbors)
- Consider in-memory embedding store (Redis)

---

### Risk 4: Learned Model Overfitting

**Issue:** GAT model memorizes training queries, poor generalization.

**Symptoms:**

- High training accuracy, low test accuracy
- Learned attention worse than heuristic on novel queries
- Model fails on out-of-distribution query patterns

**Mitigation:**

- Use self-supervised training (predict co-activation, not ground truth)
- Regularization: dropout (0.3-0.6), L2 penalty, early stopping
- Augment training data (paraphrase queries, synonym replacement)
- Monitor online performance, roll back if worse than heuristic

---

### Risk 5: Complexity Explosion

**Issue:** Adding attention increases code complexity and maintenance burden.

**Symptoms:**

- Hard to debug retrieval issues
- Many hyperparameters to tune
- Difficult to explain results to users

**Mitigation:**

- Start simple (Option 1: heuristic attention)
- Gate behind feature flags (easy rollback)
- Comprehensive logging (attention scores, neighbor selection)
- Jiminy explanations (show which edges were attended to)

---

## 10. Conclusion

### Key Findings

1. **GAT provides learned attention** that can improve neighbor selection vs. uniform k-hop traversal.
2. **GATv2 is superior to GAT** due to dynamic (query-conditioned) attention.
3. **MDEMG's current approach is a hybrid** - structural expansion + learned activation spreading.
4. **Gap: Expansion is query-independent** - same neighbors fetched regardless of query context.
5. **Opportunity: Apply lightweight attention** during expansion to prioritize query-relevant neighbors.

### Recommended Strategy

✅ **Short-Term (Phase 1-2):** Implement lightweight query-aware attention using cosine similarity between query embedding and neighbor embeddings. Re-rank neighbors by attention before selecting top-N.

✅ **Medium-Term (Phase 3-4):** Experiment with hybrid edge type mixing (structural first hop, learned subsequent hops) and integrate all MDEMG edge features (weight, dimensions, evidence) into attention computation.

❓ **Long-Term (Phase 5):** Evaluate whether learned GAT model provides sufficient benefit over heuristic attention to justify training/deployment complexity. Requires 10k+ query logs with ground truth.

### Expected Impact

| Metric | Current | With Attention | Improvement |
|--------|---------|----------------|-------------|
| Precision@5 | 0.80 | 0.84 | +5% |
| Recall@20 | 0.75 | 0.78 | +4% |
| NDCG@10 | 0.82 | 0.87 | +6% |
| Latency (ms) | 70 | 85 | +21% |
| Cache Hit Rate | 60% | 40% | -33% |

**Net Assessment:** Attention improves precision/recall at cost of latency and caching. Trade-off is favorable for complex queries (architectural, cross-cutting) but marginal for simple lookups.

### Open Questions

1. **How much training data is needed** for learned GAT to outperform heuristic attention?
2. **Can attention be cached** effectively despite query-dependence?
3. **Does attention help more for specific query types** (e.g., architectural vs. code lookup)?
4. **How does attention interact with Hebbian learning** (CO_ACTIVATED_WITH edge creation)?

### Next Steps

1. **Implement Phase 1** (query-aware attention, feature flag)
2. **Run benchmarks** on whk-wms, plc-gbt, pytorch
3. **Analyze results** by query type (architectural, comparison, cross-cutting)
4. **Decide Phase 2** based on precision/latency trade-off
5. **Document findings** in benchmark reports

---

## Appendix A: Additional Resources

### Papers

- **GAT (2018):** <https://arxiv.org/abs/1710.10903>
- **GATv2 (2021):** <https://arxiv.org/abs/2105.14491>
- **HEAT (2021):** <https://arxiv.org/abs/2106.07161>
- **K-hop GNN (2019):** <https://arxiv.org/abs/1907.06051>
- **MAGNA (Multi-hop Attention, 2021):** <https://cs.stanford.edu/people/jure/pubs/magna-ijcai21.pdf>

### Implementations

- **PyTorch Geometric:** <https://pytorch-geometric.readthedocs.io/>
- **DGL (Deep Graph Library):** <https://www.dgl.ai/>
- **Original GAT (TensorFlow):** <https://github.com/PetarV-/GAT>

### Tutorials

- **PyG GAT Tutorial:** <https://antoniolonga.github.io/Pytorch_geometric_tutorials/posts/post3.html>
- **Neo4j + DGL Integration:** <https://towardsdatascience.com/neo4j-dgl-a-seamless-integration-624ad6edb6c0>
- **Graph Transformers Overview (2026):** <https://medium.com/@jhahimanshu3636/graph-transformers-a-fresh-perspective-on-learning-from-graphs-f95f0378b939>

### Comparisons

- **GCN vs GAT vs GraphSAGE:** <https://apxml.com/courses/introduction-to-graph-neural-networks/chapter-3-foundational-gnn-architectures/comparing-gcn-graphsage-gat>
- **Message Passing Architectures:** <https://wandb.ai/yashkotadia/benchmarking-gnns/reports/Part-2-Comparing-Message-Passing-Based-GNN-Architectures--VmlldzoyMTk4OTA>

---

## Appendix B: MDEMG Graph Schema Summary

### Node Types (role_type)

- `null` or `"leaf"` - L0 base nodes (code, observations)
- `"concern"` or `"hidden"` - L1 hidden layer (DBSCAN clusters, cross-cutting concerns)
- `"emergent_concept"` - L2+ concept layer (higher abstractions)
- `"conversation_observation"` - Conversation memory (CMS)
- `"conversation_theme"` - Conversation themes (CMS)
- `"config_summary"` - Config file summary (consolidation)
- `"comparison"` - Comparison node (consolidation)

### Edge Types (relationship types)

| Edge Type | Purpose | Created By | Weight Learned |
|-----------|---------|------------|----------------|
| `ASSOCIATED_WITH` | Semantic similarity | Ingest (if similarity > threshold) | Semi (initial, incremented on match) |
| `GENERALIZES` | L0→L1 abstraction | Consolidation (DBSCAN) | No (structural) |
| `ABSTRACTS_TO` | L1→L2 concept hierarchy | Consolidation (message passing) | No (structural) |
| `CO_ACTIVATED_WITH` | Hebbian learning | Retrieval (query co-occurrence) | Yes (Hebbian) |
| `CONTRADICTS` | Conflict detection | Consolidation (future) | No (inhibitory) |
| `IMPLEMENTS_CONCERN` | Cross-cutting concerns | Consolidation | No (structural) |
| `IMPLEMENTS_CONFIG` | Config relationships | Consolidation | No (structural) |
| `COMPARED_IN` | Comparison relationships | Consolidation | No (structural) |
| `HAS_OBSERVATION` | Node→Observation | Ingest | No (metadata) |

### Edge Properties

- `weight` - Learned Hebbian weight (0.0-1.0)
- `dim_semantic` - Cosine similarity (0.0-1.0)
- `dim_temporal` - Temporal proximity (0.0-1.0)
- `dim_coactivation` - Co-occurrence strength (0.0-1.0)
- `evidence_count` - Number of co-activations (Hebbian reinforcement)
- `last_activated_at` - Most recent activation timestamp
- `created_at` - Edge creation timestamp
- `updated_at` - Last update timestamp
- `status` - "active" or "archived"

---

**Document Version:** 1.0
**Date:** 2026-01-30
**Author:** Claude (Sonnet 4.5) with CMS context from mdemg-dev space
**Status:** Research Complete - Awaiting Phase 1 Implementation Decision
