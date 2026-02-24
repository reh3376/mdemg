# MDEMG Enhancement Research Corpus

**Date:** 2026-01-30
**Purpose:** Deep research on four missing capabilities from the original MLX memory concept

---

## Table of Contents

1. [Temporal Decay in Retrieval Scoring](#1-temporal-decay-in-retrieval-scoring)
2. [Automatic Concept Merging](#2-automatic-concept-merging)
3. [Gated Memory Selection](#3-gated-memory-selection)
4. [Graph Attention Networks](#4-graph-attention-networks)

---

## 1. Temporal Decay in Retrieval Scoring

### 1.1 Current MDEMG State

**MDEMG already has recency decay!** Located in `internal/retrieval/scoring.go:348-355`:

```go
ageDays := now.Sub(c.UpdatedAt).Hours() / 24.0
r := math.Exp(-rho * ageDays)  // Exponential decay
recComponent := gamma * r       // gamma = recency weight
```

Current parameters (from config):

- `rho` (ScoringRho): decay rate per day
- `gamma` (ScoringGamma): weight of recency component in final score

**Limitation:** Uses `UpdatedAt` (last modification) not `CreatedAt` (original creation).

### 1.2 Theoretical Foundations

#### The Forgetting Curve (Ebbinghaus)

The [forgetting curve](https://en.wikipedia.org/wiki/Forgetting_curve) describes how memory "retrieval strength" decreases over time. Two competing models:

| Model | Formula | Characteristics |
|-------|---------|-----------------|
| **Exponential** | `R(t) = e^(-t/S)` | Sharp initial drop, faster convergence |
| **Power Law** | `R(t) = a * t^(-b)` | Slower decay, better fits long-term data |

[Kahana (2002)](https://memory.psych.upenn.edu/files/pubs/KahaAdle02.pdf) found power law better describes forgetting over longer periods.

#### Two-Component Memory Theory (Bjork & Bjork)

Memory has two facets:

- **Storage strength**: How well-learned (doesn't decay)
- **Retrieval strength**: How accessible now (decays but can be restored)

This maps to MDEMG:

- Storage strength → Node exists in graph
- Retrieval strength → Temporal decay factor

#### Memory Chain Model

[SuperMemo's model](https://supermemo.guru/wiki/Forgetting_curve): Memory passes through stores with different decay rates:

1. Hippocampus (fast decay, exponential)
2. Neocortex (slow decay, power law)

**MDEMG parallel:** L0 (raw memories) could decay faster than L1/L2 (consolidated concepts).

### 1.3 Enhancement Opportunities

#### A. Layer-Specific Decay Rates

```go
// Different decay rates per layer
func getDecayRate(layer int) float64 {
    switch layer {
    case 0: return 0.05  // L0: 5% decay per day (raw memories)
    case 1: return 0.01  // L1: 1% decay per day (concepts)
    case 2: return 0.002 // L2: 0.2% decay per day (meta-concepts)
    default: return 0.001
    }
}
```

#### B. Access-Based Decay Reset

Each retrieval "touches" a memory, resetting its retrieval strength:

```go
// On retrieval, update last_accessed
func touchMemory(nodeID string) {
    // Update last_accessed timestamp
    // This resets the decay clock
}

// Use last_accessed instead of updated_at for decay
ageDays := now.Sub(c.LastAccessed).Hours() / 24.0
```

#### C. Power Law vs Exponential (Configurable)

```go
type DecayModel int
const (
    DecayExponential DecayModel = iota  // r = e^(-rho * days)
    DecayPowerLaw                        // r = 1 / (1 + days)^alpha
    DecayHybrid                          // Exponential short-term, power-law long-term
)

func computeDecay(days float64, model DecayModel) float64 {
    switch model {
    case DecayExponential:
        return math.Exp(-rho * days)
    case DecayPowerLaw:
        return 1.0 / math.Pow(1.0 + days, alpha)
    case DecayHybrid:
        if days < 7 {
            return math.Exp(-rho * days)
        }
        return 1.0 / math.Pow(1.0 + days, alpha)
    }
}
```

#### D. "Evergreen" Memory Protection

Some memories shouldn't decay (core architectural knowledge):

```go
// Tag certain nodes as evergreen
if hasTag(c.Tags, "evergreen") || hasTag(c.Tags, "core") {
    r = 1.0  // No decay
}
```

### 1.4 Integration Points in MDEMG

| File | Function | Change |
|------|----------|--------|
| `scoring.go` | `ScoreAndRankWithBreakdown` | Add layer-specific decay |
| `models/models.go` | `Memory` struct | Add `LastAccessed` field |
| `config/config.go` | Config | Add `DecayModel`, per-layer rates |
| `api/handlers.go` | Retrieve handler | Touch accessed nodes |

### 1.5 Sources

- [Forgetting Curve - Wikipedia](https://en.wikipedia.org/wiki/Forgetting_curve)
- [Power Law of Forgetting - Kahana](https://memory.psych.upenn.edu/files/pubs/KahaAdle02.pdf)
- [Memory Chain Model - SuperMemo](https://supermemo.guru/wiki/Forgetting_curve)
- [Deep Knowledge Tracing with Forgetting](https://www.sciencedirect.com/science/article/pii/S0950705125019227)

---

## 2. Automatic Concept Merging

### 2.1 Current MDEMG State

MDEMG uses DBSCAN clustering in `internal/hidden/clustering.go` to form concepts, but does NOT merge similar existing concepts. New concepts are always created, leading to potential explosion.

Current consolidation flow:

```
L0 nodes → DBSCAN cluster → Create new L1 node → ABSTRACTS_TO edges
```

**Missing:** Check if new concept is similar to existing concept and merge instead.

### 2.2 Theoretical Foundations

#### Entity Resolution Process

From [Semantic Entity Resolution](https://towardsdatascience.com/the-rise-of-semantic-entity-resolution/):

1. **Blocking**: Group similar nodes for pairwise comparison (O(n) vs O(n²))
2. **Matching**: Compute similarity, decide if same entity
3. **Merging**: Combine matched nodes, resolve attribute conflicts
4. **Clustering**: Form connected components of SAME_AS edges

#### Recommended Thresholds

From [Unsupervised Graph-Based Entity Resolution](https://dl.acm.org/doi/10.1145/3533016):

| Threshold | Value | Purpose |
|-----------|-------|---------|
| Bootstrap (t_b) | 0.95 | Initial high-confidence matches |
| Merge (t_m) | 0.85 | General merge decision |
| Atomic (t_a) | 0.90 | Individual node similarity |

#### Neighborhood-Based Resolution

From [Cherre Blog](https://blog.cherre.com/2021/07/02/improving-knowledge-graph-quality-with-neighborhood-based-entity-resolution/):

> Nodes that are candidates for merging should have similar semantic neighborhoods (sets of immediate neighbors should heavily overlap).

This is powerful for MDEMG: L1 concepts with similar ABSTRACTS_TO children are merge candidates.

### 2.3 Enhancement Opportunities

#### A. Similarity Check Before Creating L1

```go
func (s *Service) createOrMergeConcept(ctx context.Context, spaceID string,
    newCentroid []float64, members []BaseNode) (string, error) {

    // Find existing concepts similar to new centroid
    existing, err := s.findSimilarConcepts(ctx, spaceID, newCentroid,
        threshold: 0.85, topK: 3)

    if len(existing) > 0 && existing[0].Similarity > 0.90 {
        // Merge into existing concept
        return s.mergeIntoConcept(ctx, existing[0].NodeID, members)
    }

    // Create new concept
    return s.createNewConcept(ctx, spaceID, newCentroid, members)
}
```

#### B. EMA Update for Merged Concepts

When merging, update the centroid using Exponential Moving Average:

```go
func updateCentroidEMA(existing, new []float64, alpha float64) []float64 {
    // alpha = 0.1 means 10% new, 90% existing (slow drift)
    result := make([]float64, len(existing))
    for i := range existing {
        result[i] = (1-alpha)*existing[i] + alpha*new[i]
    }
    return result
}
```

#### C. Neighborhood Similarity for Merge Candidates

```go
func neighborhoodSimilarity(conceptA, conceptB string) float64 {
    // Get ABSTRACTS_TO children for both concepts
    childrenA := getChildren(conceptA)
    childrenB := getChildren(conceptB)

    // Jaccard similarity of child sets
    intersection := len(intersect(childrenA, childrenB))
    union := len(union(childrenA, childrenB))

    return float64(intersection) / float64(union)
}
```

#### D. Periodic Merge Scan (Background Job)

```go
func (s *Service) RunConceptMergeScan(ctx context.Context, spaceID string) error {
    // Get all L1 concepts
    concepts, _ := s.FindEmergentConceptsForSpace(ctx, spaceID)

    // Build similarity matrix (use LSH for scale)
    candidates := findMergeCandidates(concepts, threshold: 0.85)

    // Merge similar pairs
    for _, pair := range candidates {
        s.mergeConcepts(ctx, pair.keepID, pair.mergeID)
    }

    return nil
}
```

### 2.4 Integration Points in MDEMG

| File | Function | Change |
|------|----------|--------|
| `hidden/service.go` | `RunConsolidation` | Add merge check before create |
| `hidden/service.go` | New: `mergeConcepts` | Merge logic with edge transfer |
| `hidden/service.go` | New: `findSimilarConcepts` | Vector similarity search |
| `hidden/clustering.go` | New: `neighborhoodSimilarity` | Structural similarity |
| `api/handlers.go` | New endpoint | `/v1/memory/merge-concepts` |

### 2.5 Sources

- [Semantic Entity Resolution - TDS](https://towardsdatascience.com/the-rise-of-semantic-entity-resolution/)
- [Unsupervised Graph-Based Entity Resolution](https://dl.acm.org/doi/10.1145/3533016)
- [Neighborhood-Based Entity Resolution](https://blog.cherre.com/2021/07/02/improving-knowledge-graph-quality-with-neighborhood-based-entity-resolution/)
- [Entity Resolution Techniques](https://spotintelligence.com/2024/01/22/entity-resolution/)

---

## 3. Gated Memory Selection

### 3.1 Current MDEMG State

MDEMG combines multiple memory sources with **fixed weights**:

```go
// scoring.go - Fixed coefficients
alpha := cfg.ScoringAlpha  // vector similarity weight
beta := cfg.ScoringBeta    // activation weight (from learning edges)
gamma := cfg.ScoringGamma  // recency weight
delta := cfg.ScoringDelta  // confidence weight

s := alpha*vectorSim + beta*activation + gamma*recency + delta*confidence
```

**Missing:** Dynamic, query-dependent weighting of memory sources.

### 3.2 Theoretical Foundations

#### Mixture of Experts (MoE)

From [Hugging Face MoE Explained](https://huggingface.co/blog/moe):

> The Sparsely-Gated MoE layer consists of numerous expert networks and a trainable gating network that selects a sparse combination of experts for each input.

Key concepts:

- **Experts**: Specialized sub-networks (in MDEMG: L0, L1, learning edges)
- **Gating network**: Learns which experts to use for each input
- **Top-k selection**: Only activate k experts (sparse computation)

#### Gating Mechanisms

From [Gating Mechanisms in Neural Architectures](https://medium.com/@adnanmasood/gating-mechanisms-in-modern-neural-architectures-6f5268412733):

```
gate = σ(W_g · [query; expert_output])  # Sigmoid gating
output = gate * expert_output + (1-gate) * identity
```

This learns WHEN to use each memory source.

#### Load Balancing

Challenge: Gating networks tend to over-use a few experts. Solutions:

- Add noise during training (`softmax(scores + noise)`)
- Auxiliary loss to encourage balanced usage
- Expert capacity limits

### 3.3 Enhancement Opportunities

#### A. Query-Aware Source Weighting

```go
type MemoryGate struct {
    // Learned weights for different query types
    VectorWeight    float64
    L1Weight        float64
    LearningWeight  float64
}

func computeGates(queryEmbedding []float64, queryFeatures QueryFeatures) MemoryGate {
    // Simple MLP to predict source weights
    // Input: query embedding + query features (length, keywords, etc.)
    // Output: softmax weights for each memory source

    features := append(queryEmbedding,
        float64(queryFeatures.TokenCount),
        float64(queryFeatures.HasCodeKeywords),
        float64(queryFeatures.IsComparison),
    )

    logits := mlp.Forward(features)  // 3-output MLP
    weights := softmax(logits)

    return MemoryGate{
        VectorWeight:   weights[0],
        L1Weight:       weights[1],
        LearningWeight: weights[2],
    }
}
```

#### B. Lightweight Rule-Based Gating (No Training)

Before full MoE, use heuristics:

```go
func computeGatesHeuristic(query string, stats SpaceStats) MemoryGate {
    // Heuristics based on query characteristics

    if isCodeQuery(query) {
        // Code queries: favor L0 (raw files) over L1 (concepts)
        return MemoryGate{Vector: 0.7, L1: 0.1, Learning: 0.2}
    }

    if isArchitectureQuery(query) {
        // Architecture queries: favor L1 concepts
        return MemoryGate{Vector: 0.3, L1: 0.5, Learning: 0.2}
    }

    if stats.LearningEdgeCount > 1000 {
        // Mature space: trust learning edges more
        return MemoryGate{Vector: 0.4, L1: 0.2, Learning: 0.4}
    }

    // Default balanced
    return MemoryGate{Vector: 0.5, L1: 0.3, Learning: 0.2}
}
```

#### C. Per-Source Retrieval with Late Fusion

```go
func retrieveWithGating(query string, spaceID string) []Result {
    gates := computeGates(query)

    // Retrieve from each source independently
    vectorResults := retrieveVector(query, topK: 20)
    l1Results := retrieveL1Concepts(query, topK: 10)
    learningResults := retrieveLearningActivated(query, topK: 10)

    // Score each result with gated weights
    combined := []ScoredResult{}

    for _, r := range vectorResults {
        combined = append(combined, ScoredResult{
            Result: r,
            Score:  gates.VectorWeight * r.Score,
            Source: "vector",
        })
    }

    for _, r := range l1Results {
        combined = append(combined, ScoredResult{
            Result: r,
            Score:  gates.L1Weight * r.Score,
            Source: "l1",
        })
    }

    // ... similar for learning results

    // Sort by combined score, dedupe, return top K
    return dedupeAndSort(combined, topK: 10)
}
```

#### D. Multi-Gate MoE (Per-Task Gating)

Different tasks need different memory mixtures:

```go
type TaskType int
const (
    TaskCodeSearch TaskType = iota
    TaskConceptual
    TaskTemporal
    TaskRelational
)

func detectTaskType(query string) TaskType {
    // Simple classifier based on keywords
    if containsAny(query, []string{"function", "class", "implement"}) {
        return TaskCodeSearch
    }
    if containsAny(query, []string{"architecture", "design", "pattern"}) {
        return TaskConceptual
    }
    // ...
}

func gatesForTask(task TaskType) MemoryGate {
    // Learned or configured gates per task type
    return taskGates[task]
}
```

### 3.4 Integration Points in MDEMG

| File | Function | Change |
|------|----------|--------|
| `retrieval/service.go` | `Retrieve` | Add gating computation |
| `retrieval/scoring.go` | `ScoreAndRank` | Use dynamic weights |
| `config/config.go` | Config | Add `GatingMode` (fixed/heuristic/learned) |
| New: `retrieval/gating.go` | Gate computation | Heuristics and ML gating |
| New: `retrieval/gating_model.go` | MLP model | For learned gating |

### 3.5 Sources

- [Mixture of Experts Explained - HuggingFace](https://huggingface.co/blog/moe)
- [Sparsely-Gated MoE - Shazeer et al.](https://arxiv.org/abs/1701.06538)
- [Gating Mechanisms in Neural Architectures](https://medium.com/@adnanmasood/gating-mechanisms-in-modern-neural-architectures-6f5268412733)
- [KAMoE - Gated Residual Networks](https://arxiv.org/pdf/2409.15161)

---

## 4. Graph Attention Networks

### 4.1 Current MDEMG State

MDEMG uses **uniform hop traversal** with simple activation spreading:

```go
// activation.go - Spreading activation
for _, e := range edges {
    if e.RelType != "CO_ACTIVATED_WITH" {
        continue  // Only propagate through learning edges
    }
    srcA := act[e.Src]
    w := effectiveWeight(e)
    degreeNorm := math.Sqrt(float64(len(ins)))
    acc += (srcA * w) / degreeNorm  // Degree-normalized sum
}
```

**Missing:** Learned attention weights that adapt to query and neighbor features.

### 4.2 Theoretical Foundations

#### GAT vs GCN

From [GAT vs GCN - TDS](https://towardsdatascience.com/graph-neural-networks-part-2-graph-attention-networks-vs-gcns-029efd7a1d92/):

| Aspect | GCN | GAT |
|--------|-----|-----|
| Neighbor weighting | Degree-based (fixed) | Attention-based (learned) |
| Feature dependence | No | Yes - weights depend on features |
| Computational cost | Lower | Higher but parallelizable |
| Interpretability | Limited | Attention weights explain |

#### GAT Attention Mechanism

```
attention(i,j) = softmax(LeakyReLU(a^T · [Wh_i || Wh_j]))

h'_i = σ(Σ_j attention(i,j) · Wh_j)
```

Where:

- `h_i, h_j`: Node feature vectors
- `W`: Shared weight matrix
- `a`: Attention vector
- `||`: Concatenation

#### Multi-Head Attention

Use K independent attention heads, concatenate outputs:

```
h'_i = ||_{k=1}^{K} σ(Σ_j α^k_{ij} · W^k h_j)
```

Benefits:

- Stabilizes learning
- Captures different relationship types
- Each head can focus on different neighbors

#### Edge Features in Attention

From [GTAT - Nature](https://www.nature.com/articles/s41598-025-88993-3):

> GAT can be extended to incorporate edge features in attention computation, allowing relationship types to influence neighbor importance.

### 4.3 Enhancement Opportunities

#### A. Simple Attention (No Training)

Replace degree normalization with similarity-based attention:

```go
func attentionActivation(query []float64, cands []Candidate, edges []Edge) map[string]float64 {
    act := map[string]float64{}

    // Seed from vector similarity
    for _, c := range cands {
        act[c.NodeID] = c.VectorSim
    }

    // Compute attention weights per edge
    for dst, ins := range incoming {
        var attnSum float64
        attnWeights := make([]float64, len(ins))

        // Compute raw attention scores
        for i, e := range ins {
            srcEmb := getEmbedding(e.Src)
            dstEmb := getEmbedding(e.Dst)

            // Attention = similarity(query, src) * edge_weight
            attnWeights[i] = cosineSim(query, srcEmb) * e.Weight
            attnSum += attnWeights[i]
        }

        // Normalize (softmax)
        for i := range attnWeights {
            attnWeights[i] /= attnSum
        }

        // Aggregate with attention weights
        for i, e := range ins {
            act[dst] += attnWeights[i] * act[e.Src]
        }
    }

    return act
}
```

#### B. Edge-Type Attention

Different edge types get different attention biases:

```go
var edgeTypeBias = map[string]float64{
    "CO_ACTIVATED_WITH": 1.0,   // Learning edges - high attention
    "ABSTRACTS_TO":      0.8,   // Concept edges - medium
    "ASSOCIATED_WITH":   0.3,   // Structural edges - low
    "SAME_SYMBOL":       0.5,   // Symbol edges - medium
}

func edgeAttention(e Edge, queryRelevance float64) float64 {
    typeBias := edgeTypeBias[e.RelType]
    return queryRelevance * e.Weight * typeBias
}
```

#### C. Lightweight GAT Layer

Implement a single GAT layer without full neural network:

```go
type GATLayer struct {
    // Attention vector (learned or fixed)
    attnVector []float64

    // Edge type embeddings
    edgeTypeEmb map[string][]float64
}

func (g *GATLayer) Attention(src, dst, query []float64, edgeType string) float64 {
    // Concatenate features
    concat := append(src, dst...)
    concat = append(concat, query...)
    concat = append(concat, g.edgeTypeEmb[edgeType]...)

    // Dot product with attention vector
    score := dotProduct(g.attnVector, concat)

    // LeakyReLU
    if score < 0 {
        score *= 0.2
    }

    return score
}

func (g *GATLayer) Aggregate(nodeID string, neighbors []Edge, act map[string]float64,
    query []float64) float64 {

    scores := make([]float64, len(neighbors))
    for i, e := range neighbors {
        srcEmb := getEmbedding(e.Src)
        dstEmb := getEmbedding(nodeID)
        scores[i] = g.Attention(srcEmb, dstEmb, query, e.RelType)
    }

    // Softmax
    weights := softmax(scores)

    // Weighted aggregation
    var result float64
    for i, e := range neighbors {
        result += weights[i] * act[e.Src]
    }

    return result
}
```

#### D. Pre-computed Attention Weights

For performance, pre-compute attention weights during consolidation:

```go
// During consolidation, compute and store attention weights
func precomputeAttentionWeights(spaceID string) {
    for _, edge := range getAllEdges(spaceID) {
        srcEmb := getEmbedding(edge.Src)
        dstEmb := getEmbedding(edge.Dst)

        // Base attention from embeddings
        baseAttn := cosineSim(srcEmb, dstEmb)

        // Store as edge property
        edge.AttentionWeight = baseAttn * edge.Weight
        updateEdge(edge)
    }
}

// At retrieval time, use pre-computed weights (fast)
func fastAttentionActivation(query []float64, edges []Edge) map[string]float64 {
    // Only compute query-specific modulation
    for _, e := range edges {
        queryMod := cosineSim(query, getEmbedding(e.Src))
        finalAttn := e.AttentionWeight * queryMod
        // aggregate...
    }
}
```

### 4.4 Integration Points in MDEMG

| File | Function | Change |
|------|----------|--------|
| `retrieval/activation.go` | `SpreadingActivation` | Replace degree norm with attention |
| New: `retrieval/attention.go` | GAT layer | Attention computation |
| `models/models.go` | `Edge` struct | Add `AttentionWeight` field |
| `hidden/service.go` | `RunConsolidation` | Pre-compute attention weights |
| `config/config.go` | Config | Add `AttentionMode` setting |

### 4.5 Sources

- [GAT Paper - Veličković et al.](https://petar-v.com/GAT/)
- [GAT vs GCN - TDS](https://towardsdatascience.com/graph-neural-networks-part-2-graph-attention-networks-vs-gcns-029efd7a1d92/)
- [DGL GAT Tutorial](https://www.dgl.ai/dgl_docs/en/2.0.x/tutorials/models/1_gnn/9_gat.html)
- [GTAT Cross Attention - Nature](https://www.nature.com/articles/s41598-025-88993-3)
- [Graph Attention Networks Review](https://www.mdpi.com/1999-5903/16/9/318)

---

## 5. Priority Matrix

Based on implementation complexity and expected impact:

| Enhancement | Complexity | Impact | Priority |
|-------------|------------|--------|----------|
| **Layer-specific decay** | Low | Medium | 1 - Quick Win |
| **Rule-based gating** | Low | Medium | 2 - Quick Win |
| **Concept merge check** | Medium | High | 3 - High Value |
| **Pre-computed attention** | Medium | Medium | 4 - Performance |
| **Learned gating (MoE)** | High | High | 5 - Research |
| **Full GAT layer** | High | Medium | 6 - Research |

### Recommended Implementation Order

1. **Phase 1 (Quick Wins):**
   - Add `LastAccessed` timestamp, update on retrieval
   - Implement layer-specific decay rates
   - Add heuristic query-type gating

2. **Phase 2 (Core Features):**
   - Implement concept merge check in consolidation
   - Add edge-type attention in spreading activation
   - Pre-compute base attention weights

3. **Phase 3 (Advanced):**
   - Train lightweight gating MLP
   - Implement single GAT layer
   - Benchmark and tune

---

## Appendix: Code Snippets from Original MLX Concept

### Temporal Encoder

```python
class TemporalEncoder:
    def __init__(self, dim=128, max_period=10000.0):
        self.dim = dim
        self.max_period = max_period

    def encode(self, timestamps):
        # Sinusoidal encoding like Transformer positions
        positions = timestamps / self.max_period
        div_term = mx.exp(mx.arange(0, self.dim, 2) * (-math.log(10000.0) / self.dim))
        pe = mx.zeros((len(timestamps), self.dim))
        pe[:, 0::2] = mx.sin(positions[:, None] * div_term)
        pe[:, 1::2] = mx.cos(positions[:, None] * div_term)
        return pe
```

### Gated Memory Integration

```python
class MemoryIntegration(nn.Module):
    def __call__(self, x, episodic, graph, semantic):
        # Each memory type has its own gate
        eps_out = self.episodic_attn(x, episodic)
        gate = self.episodic_gate(mx.concatenate([x, eps_out], axis=-1))
        output = x + gate * eps_out  # Gated residual

        # Repeat for graph and semantic...
        return output
```

### Memory Consolidation

```python
class MemoryConsolidator:
    def consolidate(self, episodic_store, semantic_store, cluster_threshold=0.7):
        # Sample high-importance episodes
        episodes = sample_by_importance(episodic_store, n=1000)

        # Project to concept space
        concepts = self.concept_former(episodes.values)

        # Cluster
        clusters = self._cluster(concepts, threshold=cluster_threshold)

        # Form or update concepts
        for cluster in clusters:
            centroid = cluster.mean()
            existing = semantic_store.find_similar(centroid, threshold=0.9)

            if existing:
                semantic_store.update_ema(existing.id, centroid, alpha=0.1)
            else:
                semantic_store.add_concept(centroid, sources=cluster.ids)
```
