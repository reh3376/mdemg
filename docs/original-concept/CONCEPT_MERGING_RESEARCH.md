# Automatic Concept Merging and Deduplication in Knowledge Graphs

## Comprehensive Research Report

**Research Date:** 2026-01-30
**Context:** MDEMG (Multi-Dimensional Emergent Memory Graph) Enhancement
**Focus:** Automatic merging/deduplication strategies for L1 (hidden layer) nodes

---

## Executive Summary

This document synthesizes research on automatic concept merging and deduplication techniques for knowledge graphs, with specific application to MDEMG's hidden layer consolidation. Key findings:

1. **Current MDEMG Approach:** Uses DBSCAN clustering (eps=0.3, minSamples=3) with centroid averaging - creates NEW clusters but does NOT merge similar existing concepts
2. **Industry Best Practices:** Hybrid approaches combining similarity metrics, threshold-based merging, and conflict resolution strategies
3. **Recommended Enhancement:** Post-consolidation similarity-based merging with configurable thresholds and EMA updates

---

## 1. Theoretical Foundations

### 1.1 Entity Resolution and Record Linkage Theory

Entity resolution (also called identity resolution, data matching, or record linkage) is the computational process by which entities are de-duplicated and/or linked in a dataset. This has been studied for over 70 years with significant recent progress.

**Key Concepts:**

- **Precision vs Recall Tradeoff:** Determining the threshold for matching records is a critical balancing act
  - Too low threshold → false positives (incorrectly merging distinct entities)
  - Too high threshold → false negatives (failing to link records representing the same entity)
- **Graph-Based Entity Resolution:** Incorporates knowledge graphs into entity resolution processes to leverage relationships and dependencies between entities
- **Entity Resolved Knowledge Graphs (ERKG):** Harness entity resolution to increase accuracy, clarity, and utility of knowledge graphs

**Sources:**

- [Entity-Resolved Knowledge Graphs | Towards Data Science](https://towardsdatascience.com/entity-resolved-knowledge-graphs-6b22c09a1442/)
- [Combining entity resolution and knowledge graphs](https://linkurious.com/blog/entity-resolution-knowledge-graph/)
- [Entity Resolved Knowledge Graphs: A Tutorial - Neo4j](https://neo4j.com/blog/developer/entity-resolved-knowledge-graphs/)
- [Modern entity resolution: the AI superpower for Knowledge Graphs](https://www.linkedin.com/pulse/modern-entity-resolution-ai-superpower-knowledge-giovanni-tummarello)
- [Using Knowledge Graphs for Record Linkage: Challenges and Opportunities](https://link.springer.com/chapter/10.1007/978-3-031-34985-0_15)

### 1.2 Semantic Similarity Measures

Semantic similarity assesses the degree of relatedness between two entities by the similarity in meaning of their annotations.

**Two Main Approaches:**

1. **Edge-Based Measures**
   - Rely on the structure of the ontology
   - Use path length, common ancestors, depth in hierarchy

2. **Node-Based Measures**
   - Rely on the terms themselves
   - Use information content to quantify semantic meaning
   - Include corpus-based and thesaurus-based approaches

**Application to Clustering:**

- Genetic algorithms for text clustering based on ontology methods
- Machine learning approaches including hierarchical clustering
- Classification algorithms (SVM, Naive Bayes, k-NN) for classification into existing hierarchy

**Sources:**

- [Genetic algorithm for text clustering using ontology - ScienceDirect](https://www.sciencedirect.com/science/article/abs/pii/S0957417408009123)
- [Semantic Similarity of Ontology Instances](https://link.springer.com/chapter/10.1007/11914853_66)
- [Comparison of Ontology-based Semantic-Similarity Measures - PMC](https://pmc.ncbi.nlm.nih.gov/articles/PMC2655943/)
- [Semantic similarity and machine learning with ontologies](https://academic.oup.com/bib/article/22/4/bbaa199/5922325)

### 1.3 Ontology Alignment and Merging

Ontology merging is an operation where concepts of two or more ontologies are compared based on some similarity measure and merged when their similarity exceeds some predefined threshold.

**Techniques:**

- **String and Semantic Matching:** Integrating both with consideration of structural heterogeneity
- **FCA-based Methods:** Pay particular attention to good similarity measures for cross-ontology concept identification
- **Weak Unification:** Allows merging while preserving conceptual distinctions

**Sources:**

- [Ontology Merging Using the Weak Unification of Concepts - MDPI](https://www.mdpi.com/2504-2289/8/9/98)

---

## 2. Similarity Metrics for Concept Merging

### 2.1 Cosine Similarity Thresholds

**Key Findings:**

| Threshold Range | Use Case | Merge Decision |
|----------------|----------|----------------|
| > 0.95 | Very conservative | Extremely similar concepts only |
| > 0.85-0.90 | Moderate | Semantically close concepts |
| > 0.70-0.80 | Aggressive | Broadly related concepts |
| 0.50-0.70 | Very aggressive | Risk of over-merging |

**Threshold Decision Framework:**

- **Above threshold A:** Auto-merge
- **Between A and B:** Human review
- **Below B:** Ignore (no match)

**Context-Dependent Tuning:**

- Higher thresholds (e.g., 0.1 in semantic dedup) → more aggressive deduplication
- Lower thresholds → more strict, requiring higher similarity
- Typical semantic deduplication reduces dataset size by 20-50%

**Important Considerations:**

- Thresholds are NOT universal - they depend on:
  - Dataset characteristics
  - Embedding model used
  - Tolerance for false positives vs false negatives
  - Domain-specific requirements

**Sources:**

- [Fuzzy Matching | Boardflare](https://www.boardflare.com/tasks/nlp/fuzzy-match)
- [Why No Single Algorithm Solves Deduplication](https://hackernoon.com/why-no-single-algorithm-solves-deduplication-and-what-to-do-instead)
- [Semantic Deduplication — NVIDIA NeMo Framework](https://docs.nvidia.com/nemo-framework/user-guide/25.07/datacuration/semdedup.html)
- [Entity Resolution Explained: Techniques and Libraries](https://spotintelligence.com/2024/01/22/entity-resolution/)

### 2.2 Graph-Based Similarity

Beyond embedding similarity, graph-based metrics provide structural context:

**Metrics:**

- **Shared Neighbors:** Number of common adjacent nodes
- **Structural Equivalence:** Position in graph topology
- **Path-Based Similarity:** Length of shortest path between nodes
- **Neighborhood Overlap:** Jaccard similarity of neighbor sets

**Hybrid Approaches:**

- BM25 keyword matching to gather candidates
- Embedding similarity to select most semantically relevant
- Graph structure to validate consistency

**Sources:**

- [Cherre blog - Neighborhood-Based Entity Resolution](https://blog.cherre.com/2021/07/02/improving-knowledge-graph-quality-with-neighborhood-based-entity-resolution/)
- [Combining knowledge graphs, quickly and accurately - Amazon Science](https://www.amazon.science/blog/combining-knowledge-graphs-quickly-and-accurately)

---

## 3. Merging Strategies

### 3.1 Centroid Computation

**Simple Mean (Current MDEMG Approach):**

```
centroid[i] = Σ(embedding[j][i]) / count
```

**Advantages:**

- Simple, fast, deterministic
- Works well for balanced clusters
- Currently used in MDEMG for initial cluster creation

**Limitations:**

- Treats all members equally
- Sensitive to outliers
- Doesn't preserve important features

### 3.2 Weighted Mean Aggregation

**Formula:**

```
centroid[i] = Σ(weight[j] * embedding[j][i]) / Σ(weight[j])
```

**Weight Options:**

- **Edge Weight:** Use existing GENERALIZES edge weights
- **Recency:** Recent nodes weighted higher
- **Importance Score:** Based on retrieval frequency
- **Stability Score:** Favor stable, well-established concepts

**Used in MDEMG's Forward Pass:**

```go
// GraphSAGE-style weighted mean aggregation
aggregated[i] = Σ(neighbor.embedding[i] * edge.weight) / totalWeight
updated[i] = alpha * current[i] + beta * aggregated[i]
```

### 3.3 Exponential Moving Average (EMA)

**Formula:**

```
new_embedding[i] = alpha * old_embedding[i] + (1-alpha) * incoming_embedding[i]
```

**Benefits:**

- Gradual adaptation to new information
- Preserves historical signal
- Prevents concept drift
- Common alpha values: 0.6-0.9 (MDEMG uses 0.6 for forward pass)

**Applications:**

- Online/incremental updates
- Concept evolution over time
- Reducing impact of temporary anomalies

### 3.4 Medoid Selection

**Approach:** Instead of averaging, select the most representative member

**Selection Criteria:**

- Minimum sum of distances to all other members
- Closest to theoretical centroid
- Highest stability score

**Advantages:**

- Preserves real embedding (not synthetic average)
- More interpretable
- Robust to outliers

**Sources:**

- [Centroid-Based Memory Mechanisms](https://www.emergentmind.com/topics/centroid-based-memory-mechanisms)
- [Node Embeddings for Graph Merging - ACL Anthology](https://aclanthology.org/D19-5321/)

---

## 4. Systems That Use Concept Merging

### 4.1 Neo4j Node Merging Patterns

**APOC Library Procedures:**

```cypher
// APOC refactor.mergeNodes
CALL apoc.refactor.mergeNodes([node1, node2, ...], {
  properties: "discard" | "overwrite" | "combine",
  mergeRels: true
})
```

**Conflict Resolution Strategies:**

- **discard:** Ignores properties from subsequent nodes
- **overwrite:** Uses values from later nodes to replace earlier ones
- **combine:** Combines values from multiple nodes (creates arrays)

**Dynamic Merging:**

```cypher
// apoc.merge.node - for dynamic properties
CALL apoc.merge.node(['Label'], {key: value},
  {onCreate property},
  {onMatch property}
)
```

**Known Issues (2024):**

- IndexEntryConflictException when merged nodes have relationships with UNIQUE constraints
- Requires careful handling of constraint violations

**Sources:**

- [Merge nodes - APOC Core Documentation](https://neo4j.com/docs/apoc/current/graph-refactoring/merge-nodes/)
- [apoc.merge.node - APOC Core](https://neo4j.com/docs/apoc/current/overview/apoc.merge/apoc.merge.node/)
- [apoc.refactor.mergeNodes IndexEntryConflictException - GitHub Issue](https://github.com/neo4j/apoc/issues/699)

### 4.2 Topic Modeling and Topic Merging

**LDA (Latent Dirichlet Allocation) Approaches:**

1. **Two-Stage Method:**
   - Apply LDA to derive per-document topic probabilities
   - Apply hierarchical clustering with Hellinger distance to derive final topic clusters

2. **Integrated Document Clustering + Topic Modeling:**
   - Project documents into topic space
   - Perform K-means clustering
   - Extract cluster-specific local topics and cluster-independent global topics

**Hierarchical Topic Modeling (BERTopic):**

- Uses scipy hierarchical clustering capabilities
- Manual topic merging with `merge_topics` function
- Updates topic representation and entire model
- HDBSCAN provides hierarchical structure within clusters

**Sources:**

- [Latent Dirichlet Allocation (LDA) Survey](https://www.ccs.neu.edu/home/vip/teach/DMcourse/5_topicmodel_summ/LDA_TM/LDA_survey_1711.04305.pdf)
- [Integrating Document Clustering and Topic Modeling](https://arxiv.org/pdf/1309.6874)
- [Hierarchical Topic Modeling - BERTopic](https://maartengr.github.io/BERTopic/getting_started/hierarchicaltopics/hierarchicaltopics.html)
- [Two-stage topic modelling - PLOS One](https://journals.plos.org/plosone/article?id=10.1371/journal.pone.0243208)

### 4.3 Clustering-Based Deduplication

**Incremental vs Batch Clustering:**

| Approach | Advantages | Disadvantages |
|----------|-----------|---------------|
| **Batch** | Stronger structure detection | Requires full dataset in memory |
| **Incremental** | Handles streaming data | Weaker structure detection |
| **Batch-Incremental** | Best of both | More complex implementation |

**BIRCH Algorithm (Balanced Iterative Reducing and Clustering using Hierarchies):**

- Radius threshold determines when to merge
- Lower threshold → more splitting
- Higher threshold → more merging
- Suitable for large datasets

**EVT-Based Approach:**

- Uses Extreme Value Theory to determine merge thresholds
- Threshold adapts to actual clustering task
- Works under general conditions

**Sources:**

- [A Fast and Stable Incremental Clustering Algorithm](https://www.researchgate.net/publication/220840611_A_Fast_and_Stable_Incremental_Clustering_Algorithm)
- [Incremental Clustering: The Case for Extra Clusters](https://cseweb.ucsd.edu/~dasgupta/papers/oct31.pdf)
- [Birch — scikit-learn Documentation](https://scikit-learn.org/stable/modules/generated/sklearn.cluster.Birch.html)
- [Incremental hierarchical text clustering methods: a review](https://arxiv.org/html/2312.07769v1)

---

## 5. When to Merge: Threshold Selection Methods

### 5.1 Precision-Recall Tradeoff

**Critical Balance:**

- **High Precision (Minimize False Positives):**
  - Use higher similarity threshold
  - Critical when incorrect merging is costly (e.g., medical records)
  - F0.5 score when wrongly merging is expensive

- **High Recall (Minimize False Negatives):**
  - Use lower similarity threshold
  - Important when missing connections is costly
  - F2 score when missing matches is expensive

**Blocking Methods:**

- Sorted Neighborhood: Smaller window → higher precision, lower recall
- Block processing: Aims to improve precision while maintaining comparable recall

**Sources:**

- [Precision and Recall in Entity Resolution | Tilores](https://tilores.io/content/Precision-and-Recall-in-Entity-Identity-Resolution)
- [Estimating the Performance of Entity Resolution Algorithms](https://arxiv.org/pdf/2210.01230)
- [Deterministic Coreference Resolution - MIT Press](https://direct.mit.edu/coli/article/39/4/885/1463/Deterministic-Coreference-Resolution-Based-on)

### 5.2 Online vs Batch Merging

**Online (Incremental) Merging:**

- Process new nodes as they arrive
- Compare against existing concepts
- Immediate decision: merge or create new

**Batch Merging:**

- Process accumulated nodes periodically
- Global view of all candidates
- Better optimization opportunity

**Hybrid Approach (Recommended for MDEMG):**

- Batch consolidation (current DBSCAN approach)
- Online similarity check during ingest
- Periodic cleanup/refinement pass

### 5.3 Preventing Over-Merging

**Sieve Architecture:**
Apply deterministic models from highest to lowest precision:

1. High-precision rules first (conservative merging)
2. Medium-precision statistical methods
3. Low-precision fallback rules

**Hybrid Entity Resolution:**

- Deterministic rules for obvious cases
- Probabilistic matching for ambiguous cases
- Human review for borderline cases

**Structural Validation:**

- Check graph consistency after merge
- Verify edge weights remain sensible
- Ensure no circular dependencies created

**Sources:**

- [Entity Resolution: Identifying Real-World Entities - Towards Data Science](https://towardsdatascience.com/entity-resolution-identifying-real-world-entities-in-noisy-data-3e8c59f4f41c/)
- [Precision/Recall Tradeoff - Analytics Vidhya](https://medium.com/analytics-vidhya/precision-recall-tradeoff-79e892d43134)

---

## 6. MDEMG-Specific Analysis

### 6.1 Current L1 (Hidden Layer) Node Structure

**Node Properties:**

```go
type HiddenNode struct {
    NodeID               string
    SpaceID              string
    Name                 string
    Embedding            []float64         // Original centroid
    MessagePassEmbedding []float64         // Updated via forward pass
    AggregationCount     int               // Number of base nodes
    StabilityScore       float64           // Embedding stability
    LastForwardPass      *time.Time
    LastBackwardPass     *time.Time
}
```

**Edge Structure:**

```
(BaseNode)-[:GENERALIZES {weight: float}]->(HiddenNode)
(HiddenNode)-[:ABSTRACTS_TO {weight: float}]->(ConceptNode)
```

### 6.2 Current Consolidation Logic

**Process Flow:**

1. **Fetch Orphan Nodes:**

   ```cypher
   MATCH (b:MemoryNode {space_id: $spaceId, layer: 0})
   WHERE NOT (b)-[:GENERALIZES]->(:MemoryNode {layer: 1})
     AND b.embedding IS NOT NULL
   ```

2. **DBSCAN Clustering:**

   ```go
   labels := DBSCAN(embeddings, eps=0.3, minSamples=3)
   ```

3. **Centroid Computation:**

   ```go
   centroid[i] = Σ(embedding[j][i]) / count
   ```

4. **Create Hidden Nodes:**
   - Name: `Hidden-{path_prefix}-{id}`
   - Embedding: centroid
   - MessagePassEmbedding: centroid (initially)
   - Create GENERALIZES edges from members

**Key Observation: NO MERGE LOGIC EXISTS**

- Creates new clusters from orphan nodes
- Does NOT check for similar existing hidden nodes
- Does NOT merge similar hidden nodes post-creation

### 6.3 Forward and Backward Pass

**Forward Pass (Bottom-Up):**

```cypher
// Hidden Layer Update
WITH h, neighbors,
     reduce(totalW = 0.0, n IN neighbors | totalW + n.weight) AS totalWeight
WITH h, neighbors, totalWeight,
     [i IN range(0, size(h.embedding)-1) |
       reduce(sum = 0.0, n IN neighbors | sum + n.emb[i] * n.weight) / totalWeight
     ] AS aggregated
SET h.message_pass_embedding = [i IN range(0, size(h.embedding)-1) |
      $alpha * coalesce(h.embedding[i], 0) + $beta * aggregated[i]
    ]
```

**Parameters:**

- `alpha = 0.6` (weight of current embedding)
- `beta = 0.4` (weight of aggregated neighbors)

**Backward Pass (Top-Down):**

```cypher
// Hidden nodes receive signals from concepts (above) and base data (below)
SET h.embedding = [i IN range(0, size(h.embedding)-1) |
      $selfWeight * h.embedding[i] +
      $baseWeight * baseSignal[i] +
      $conceptWeight * conceptSignal[i]
    ]
```

### 6.4 Adaptive Clustering for Higher Layers

MDEMG implements **progressive relaxation** for concept clustering:

```go
// Epsilon grows with layer
layerFactor := float64(targetLayer - 1)
adaptiveEps := baseEps * (1.0 + 0.4*layerFactor)
// L2: 1.4x, L3: 1.8x, L4: 2.2x, L5: 2.6x (capped at 0.6)

// MinSamples shrinks with layer
adaptiveMinSamples := baseMinSamples - int(layerFactor)
// Minimum 2 nodes to form a cluster
```

**Rationale:** Higher layers represent more abstract concepts that should cluster more freely.

---

## 7. Proposed MDEMG Concept Merging Strategy

### 7.1 When to Merge

**Trigger Conditions (OR logic):**

1. **Post-Consolidation Cleanup:**
   - After CreateHiddenNodes completes
   - Check newly created nodes against existing nodes
   - Merge if similarity exceeds threshold

2. **Periodic Maintenance:**
   - Scheduled job (e.g., weekly)
   - Scans all hidden nodes for similar pairs
   - Batch merge similar concepts

3. **On-Demand:**
   - Manual trigger via API endpoint
   - Useful for testing/tuning thresholds

### 7.2 Similarity Criteria

**Multi-Metric Approach:**

```go
type MergeCriteria struct {
    // Primary metric
    EmbeddingSimilarity     float64  // cosine(emb1, emb2) > threshold

    // Secondary validation metrics
    NeighborOverlap         float64  // Jaccard(neighbors1, neighbors2)
    PathSimilarity          float64  // Overlap in base node paths

    // Thresholds
    EmbeddingThreshold      float64  // Default: 0.90 (conservative)
    NeighborThreshold       float64  // Default: 0.30 (30% overlap)
    PathThresholdOptional   bool     // Don't require path match for emergent patterns
}
```

**Recommended Thresholds:**

| Similarity Type | Conservative | Moderate | Aggressive |
|----------------|--------------|----------|------------|
| Embedding (cosine) | > 0.95 | > 0.90 | > 0.85 |
| Neighbor overlap | > 0.40 | > 0.30 | > 0.20 |
| Path similarity | > 0.60 | > 0.40 | N/A |

### 7.3 Merge Algorithm

```
ALGORITHM: MergeHiddenNodes(spaceID, criteria)

1. FETCH candidate pairs
   FOR each hidden node h1:
       FOR each hidden node h2 (where h2.created_at > h1.created_at):
           IF CosineSimilarity(h1.embedding, h2.embedding) > criteria.EmbeddingThreshold:
               ADD (h1, h2) to candidates

2. VALIDATE candidates
   FOR each pair (h1, h2) in candidates:
       neighborOverlap := JaccardSimilarity(h1.neighbors, h2.neighbors)
       IF neighborOverlap < criteria.NeighborThreshold:
           SKIP pair

       IF criteria.PathThresholdOptional == false:
           pathSim := PathSimilarity(h1.members, h2.members)
           IF pathSim < criteria.PathThreshold:
               SKIP pair

       ADD (h1, h2) to validated_pairs

3. SORT validated_pairs by similarity (descending)

4. MERGE pairs (greedy, first-come wins)
   merged := empty set
   FOR each pair (h1, h2) in validated_pairs:
       IF h1 in merged OR h2 in merged:
           CONTINUE  // Already processed

       keeper := h1  // Keep older node
       mergee := h2  // Merge newer into older

       // Update embedding using EMA
       keeper.embedding := alpha * keeper.embedding + (1-alpha) * mergee.embedding
       keeper.message_pass_embedding := alpha * keeper.message_pass_embedding +
                                        (1-alpha) * mergee.message_pass_embedding

       // Update metadata
       keeper.aggregation_count += mergee.aggregation_count
       keeper.stability_score := 0.5 * (keeper.stability_score + mergee.stability_score)

       // Redirect edges
       FOR each (base)-[:GENERALIZES]->(mergee):
           CREATE (base)-[:GENERALIZES {weight: ...}]->(keeper)
           DELETE (base)-[:GENERALIZES]->(mergee)

       FOR each (mergee)-[:ABSTRACTS_TO]->(concept):
           CREATE (keeper)-[:ABSTRACTS_TO {weight: ...}]->(concept)
           DELETE (mergee)-[:ABSTRACTS_TO]->(concept)

       // Delete merged node
       DELETE mergee

       ADD h1, h2 to merged

5. RETURN merge_count, merged_node_ids
```

### 7.4 Edge Conflict Resolution

**When Merging Edges:**

```cypher
// Scenario: Both h1 and h2 have GENERALIZES edge from same base node b
// Resolution: Keep edge with HIGHER weight, update timestamp

MATCH (b)-[r1:GENERALIZES]->(h1)
MATCH (b)-[r2:GENERALIZES]->(h2)
WHERE merge_h2_into_h1
WITH b, h1, h2, r1, r2,
     CASE WHEN r1.weight > r2.weight THEN r1.weight ELSE r2.weight END AS maxWeight
DELETE r2
SET r1.weight = maxWeight,
    r1.updated_at = datetime()
```

**For ABSTRACTS_TO Edges:**

- Combine weights if both connect to same concept: `new_weight = max(w1, w2)`
- Preserve all unique concept connections

### 7.5 Configuration Parameters

```go
type ConceptMergeConfig struct {
    // Feature flags
    Enabled                 bool    // Master switch
    DryRun                  bool    // Preview mode

    // Similarity thresholds
    EmbeddingThreshold      float64 // Default: 0.90
    NeighborThreshold       float64 // Default: 0.30
    PathThreshold           float64 // Default: 0.40
    RequirePathMatch        bool    // Default: false (allow emergent patterns)

    // EMA parameters
    EMAAlpha                float64 // Default: 0.7 (favor existing embedding)

    // Limits
    MaxMergesPerRun         int     // Default: 100
    MinStabilityScore       float64 // Default: 0.5 (don't merge unstable nodes)

    // Scheduling
    AutoMergeAfterConsolidate bool  // Run after CreateHiddenNodes
    PeriodicMergeInterval     string // e.g., "24h", "168h" (weekly)
}
```

### 7.6 API Endpoint

```go
// POST /v1/memory/merge-concepts
type MergeConceptsRequest struct {
    SpaceID   string              `json:"space_id"`
    Layer     int                 `json:"layer"`      // 1 for hidden, 2+ for concepts
    DryRun    bool                `json:"dry_run"`
    Config    *ConceptMergeConfig `json:"config"`     // Optional overrides
}

type MergeConceptsResponse struct {
    MergesExecuted     int                 `json:"merges_executed"`
    NodesRemoved       int                 `json:"nodes_removed"`
    EdgesRedirected    int                 `json:"edges_redirected"`
    Preview            []MergePreview      `json:"preview"`  // If dry_run=true
    Duration           string              `json:"duration"`
}

type MergePreview struct {
    KeeperID           string  `json:"keeper_id"`
    KeeperName         string  `json:"keeper_name"`
    MergeeID           string  `json:"mergee_id"`
    MergeeName         string  `json:"mergee_name"`
    Similarity         float64 `json:"similarity"`
    NeighborOverlap    float64 `json:"neighbor_overlap"`
    PathSimilarity     float64 `json:"path_similarity"`
    Reasoning          string  `json:"reasoning"`
}
```

---

## 8. Implementation Roadmap

### Phase 1: Foundation (Week 1)

- [ ] Implement similarity computation utilities
  - Cosine similarity (reuse from clustering.go)
  - Jaccard similarity for neighbor sets
  - Path similarity metric
- [ ] Add configuration struct and parameters
- [ ] Create dry-run capability for testing

### Phase 2: Core Merge Logic (Week 2)

- [ ] Implement candidate pair detection
- [ ] Implement validation filters (neighbor/path overlap)
- [ ] Implement EMA-based embedding update
- [ ] Implement edge redirection logic
- [ ] Add conflict resolution for duplicate edges

### Phase 3: Integration (Week 3)

- [ ] Add API endpoint `/v1/memory/merge-concepts`
- [ ] Integrate with consolidation job (optional auto-merge)
- [ ] Add periodic merge scheduling
- [ ] Implement merge statistics and reporting

### Phase 4: Testing and Tuning (Week 4)

- [ ] Unit tests for merge algorithm
- [ ] Integration tests with real MDEMG data
- [ ] Benchmark impact on retrieval quality
- [ ] Tune default thresholds based on whk-wms dataset
- [ ] Document merge behavior and configuration

### Phase 5: Advanced Features (Future)

- [ ] Layer-specific adaptive thresholds (like clustering)
- [ ] Human-in-the-loop review for borderline merges
- [ ] Merge undo capability (snapshot before merge)
- [ ] Merge suggestions API (preview without execution)
- [ ] Metrics dashboard for merge health

---

## 9. Validation Strategy

### 9.1 Correctness Metrics

**Before/After Comparison:**

- Node count reduction (expect 5-15% reduction)
- Edge count change (should increase or stay same)
- Average cluster size increase
- Duplicate name detection (e.g., "Hidden-apps-whk-wms-23" and "Hidden-apps-whk-wms-47")

**Graph Health Metrics:**

- No orphaned base nodes created
- No broken ABSTRACTS_TO paths
- Edge weight distribution remains sensible
- No circular dependencies

### 9.2 Quality Metrics

**Retrieval Quality:**

- Run whk-wms benchmark before/after merge
- Score should improve (more consolidated concepts → better retrieval)
- Target: +2-5% improvement in MDEMG score

**Semantic Coherence:**

- Sample merged nodes and validate their members make sense together
- Check that merged embeddings are still meaningful
- Verify merged nodes don't span unrelated concepts

### 9.3 Performance Metrics

**Computational Cost:**

- Merge execution time (target: < 5 seconds for 1000 nodes)
- Memory usage during merge
- Impact on consolidation job duration

**Retrieval Impact:**

- Query latency before/after merge (should improve)
- Cache hit rate (may decrease temporarily)
- Distribution stats (should show healthier score distribution)

---

## 10. Related Work and References

### Academic Papers

- [Unsupervised Graph-Based Entity Resolution for Complex Entities](https://dl.acm.org/doi/10.1145/3533016)
- [Named Entity Resolution in Personal Knowledge Graphs](https://arxiv.org/abs/2307.12173)
- [Text similarity measures in data deduplication pipeline](https://ceur-ws.org/Vol-3369/paper3.pdf)
- [A Pre-trained Deep Active Learning Model for Data Deduplication](https://arxiv.org/html/2308.00721v3)

### Industry Resources

- [Entity Resolution Explained: Top 12 Techniques](https://spotintelligence.com/2024/01/22/entity-resolution/)
- [Knowledge Graph Merging - Meegle](https://www.meegle.com/en_us/topics/knowledge-graphs/knowledge-graph-merging)
- [What Are Entity Resolved Knowledge Graphs? - Senzing](https://senzing.com/entity-resolved-knowledge-graphs/)

### Tools and Libraries

- Neo4j APOC Library (node merging procedures)
- scikit-learn BIRCH (incremental clustering)
- BERTopic (hierarchical topic merging)
- NVIDIA NeMo (semantic deduplication)

---

## 11. Conclusion

### Key Takeaways

1. **MDEMG Currently Has No Merge Logic**
   - DBSCAN creates new clusters but doesn't merge similar existing concepts
   - Opportunity for 5-15% node reduction through intelligent merging

2. **Hybrid Approach Recommended**
   - Primary: Cosine similarity > 0.90 (conservative)
   - Secondary validation: Neighbor overlap, path similarity
   - EMA updates to preserve stability

3. **Tradeoffs Exist**
   - Higher thresholds → safer, fewer merges, more duplicates
   - Lower thresholds → aggressive, more merges, risk of over-merging
   - Must tune based on actual MDEMG dataset characteristics

4. **Integration Points**
   - Post-consolidation auto-merge
   - Periodic maintenance job
   - On-demand API endpoint

### Next Steps

1. **Prototype Implementation**
   - Start with dry-run mode
   - Test on whk-wms dataset
   - Validate with retrieval benchmarks

2. **Threshold Tuning**
   - Experiment with conservative thresholds first (0.95)
   - Measure impact on benchmark scores
   - Gradually relax if quality improves

3. **Production Rollout**
   - Begin with manual triggers only
   - Add auto-merge after consolidation
   - Enable periodic cleanup once validated

---

**Document Version:** 1.0
**Last Updated:** 2026-01-30
**Author:** Research synthesis by Claude Sonnet 4.5
**Review Status:** Draft - Pending Implementation
