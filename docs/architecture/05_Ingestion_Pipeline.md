# Ingestion Pipeline (Observation → Nodes/Edges)

## 1) Observation payload

Minimum fields:

- `space_id`
- `timestamp`
- `source`
- `content` (string or JSON)
- `tags[]`
- `explicit_node_paths[]` or `explicit_node_ids[]`
- `layer_hint`, `role_hint`
- `sensitivity`, `confidence`

## 2) Node resolution strategy (v1)

In priority order:

1. explicit node reference (path or id)
2. exact path match
3. vector recall against node embeddings (top N)
4. create new node (default L0 leaf)
5. (optional, v2) Bayesian resolver to arbitrate between candidates vs "create new".

### Optional (v2): Bayesian resolver

- Use only once you have labeled decisions / corrections.
- Posterior: P(n|obs) ∝ exp(k*sim_emb(n,obs))* P_prior(n) * P_meta(n,obs).
- Guardrails: never override explicit refs; include NEW_NODE; if max P < tau (or entropy high) -> create new.
- Keep it in the service layer; it must not write per-request activation.

## 3) Create node (if new)

```cypher
CREATE (n:MemoryNode {
  space_id: $spaceId,
  node_id: $nodeId,
  name: $name,
  path: $path,
  layer: $layer,
  role_type: $roleType,
  version: 1,
  status: 'active',
  created_at: datetime(),
  updated_at: datetime(),
  update_count: 0,
  description: $description,
  summary: $initialSummary,
  confidence: coalesce($confidence, 0.6),
  sensitivity: coalesce($sensitivity, 'internal'),
  tags: coalesce($tags, [])
});
```

## 4) Append observation (immutable)

```cypher
MATCH (n:MemoryNode {space_id:$spaceId, node_id:$nodeId})
CREATE (o:Observation {
  space_id:$spaceId,
  obs_id:$obsId,
  timestamp: datetime($timestamp),
  source:$source,
  content:$content,
  math_block:$mathBlock,
  created_at: datetime()
})
MERGE (n)-[:HAS_OBSERVATION {space_id:$spaceId, created_at:datetime()}]->(o)
SET n.updated_at = datetime(),
    n.update_count = n.update_count + 1;
```

## 5) Update summary (rolling)

You can do this in the service layer (recommended) then write summary back:

```cypher
MATCH (n:MemoryNode {space_id:$spaceId, node_id:$nodeId})
SET n.summary = $newSummary,
    n.updated_at = datetime(),
    n.version = n.version + 1;
```

## 6) Create/update embedding

See `03_Vector_Embeddings_and_Indexes.md`.

## 7) Edge updates (v1 heuristic)

### 7.1 Temporal adjacency (event chain)

Link current node to most recently touched node(s):

```cypher
MATCH (a:MemoryNode {space_id:$spaceId, node_id:$prevNodeId})
MATCH (b:MemoryNode {space_id:$spaceId, node_id:$nodeId})
MERGE (a)-[r:TEMPORALLY_ADJACENT {space_id:$spaceId}]->(b)
ON CREATE SET r.edge_id=$edgeId, r.created_at=datetime(), r.updated_at=datetime(),
              r.weight=0.2, r.dim_temporal=1.0, r.decay_rate=0.01, r.evidence_count=1, r.status='active'
ON MATCH SET r.updated_at=datetime(), r.evidence_count=r.evidence_count+1,
             r.weight = min(1.0, r.weight + 0.02);
```

### 7.2 Semantic association via vector nearest neighbors

Your service does:

1) query index for nearest neighbors
2) create/update `ASSOCIATED_WITH` edges to top M neighbors
3) set `dim_semantic` based on similarity score

### 7.3 Containment via path conventions

If your `path` encodes hierarchy, derive CONTAINS/PART_OF edges accordingly.

## 8) Evidence discipline

Whenever you adjust an edge based on an observation, increment:

- `evidence_count`
and optionally store:
- `last_evidence_obs_id`

Keep the graph explainable.
