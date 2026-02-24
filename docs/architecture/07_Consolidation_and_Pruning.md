# Consolidation + Pruning (Abstraction and Forgetting)

## 1) Consolidation objective

Promote repeated, stable co-activation patterns from layer k into an abstraction node in layer k+1.
This compresses the graph and creates higher-level concepts.

## 2) When to consolidate (trigger rules)

A cluster is eligible if:

- members frequently co-activate (CO_ACTIVATED_WITH evidence_count above threshold)
- cluster density above threshold
- minimum size (e.g., 3–20 nodes)
- stable over time (not just one burst)

## 3) What consolidation does

Given cluster C at layer k:

1. Create new `:MemoryNode` at layer k+1
2. Set its summary as a synthesis of member summaries
3. Add edges:
   - for each member m in C: (m)-[:ABSTRACTS_TO]->(A)
   - (A)-[:INSTANTIATES]->(m) optional reverse link
4. Optionally thin lateral edges among members (keep only strongest)

## 4) Neo4j pattern: create abstraction node

```cypher
CREATE (a:MemoryNode {
  space_id:$spaceId,
  node_id:$newId,
  name:$name,
  path:$path,
  layer:$layerPlusOne,
  role_type:'trunk',
  version:1,
  status:'active',
  created_at:datetime(),
  updated_at:datetime(),
  update_count:0,
  description:$description,
  summary:$summary,
  confidence:0.7,
  sensitivity:'internal',
  tags:$tags
});
```

Create links:

```cypher
UNWIND $memberIds AS mid
MATCH (m:MemoryNode {space_id:$spaceId, node_id:mid})
MATCH (a:MemoryNode {space_id:$spaceId, node_id:$newId})
MERGE (m)-[r:ABSTRACTS_TO {space_id:$spaceId}]->(a)
ON CREATE SET r.edge_id=randomUUID(), r.created_at=datetime(), r.updated_at=datetime(),
              r.weight=0.8, r.dim_semantic=1.0, r.decay_rate=0.001, r.evidence_count=1, r.status='active'
ON MATCH SET r.updated_at=datetime(), r.evidence_count=r.evidence_count+1;
```

## 5) Pruning rules (keep the graph healthy)

### 5.1 Edge pruning

Prune edges if all are true:

- `weight < w_min`
- `evidence_count < e_min`
- `updated_at older than T_prune`
- edge not pinned

### 5.2 Node pruning / tombstoning

Tombstone nodes if:

- no observations beyond retention window
- low degree + low access frequency
- not part of any abstraction chain

Do NOT hard delete by default; tombstone for auditability.

## 6) Avoiding consolidation disasters

Failure mode: abstract nodes become generic dumping grounds.
Fixes:

- enforce narrow clusters (cap size)
- require high intra-cluster similarity
- store “definition” as summary + top distinguishing terms
- penalize abstraction nodes in retrieval unless query is high-level
