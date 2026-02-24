# Go Service Framework (API + Retrieval + Learning)

This is the **reference service skeleton** for the MDEMG system. It is intentionally boring:

- Go, stdlib HTTP
- Neo4j driver
- In-memory activation and scoring
- Neo4j writes only for **learning deltas** and ingestion updates

## Principles (do not regress)

1. **Recall is vector search**: use `db.index.vector.queryNodes` on node embeddings.
2. **Reasoning is graph structure**: expand only a bounded neighborhood around candidates.
3. **Activation is runtime physics**: compute spreading activation in-memory for speed and to avoid write amplification.
4. **Persistence is learning deltas**: update only the small set of relationships that represent learned association changes.

---

## 1) Endpoints

### Health / readiness

- `GET /healthz` — process up
- `GET /readyz` — Neo4j reachable + schema version compatible

### Ingestion

- `POST /v1/memory/ingest`
  - Append-only `Observation`
  - Upsert / resolve `MemoryNode`
  - Update node summary/embedding (embedding generation is pluggable)
  - Create small, explainable edge updates (temporal adjacency, semantic association)

### Retrieval

- `POST /v1/memory/retrieve`
  - Requires `query_embedding` (v1). Optionally accepts `query_text` but embedding generation is intentionally out-of-scope.
  - Vector recall → bounded expansion → activation → scoring → response
  - **Then** bounded learning delta writeback (Hebbian co-activation)

### Maintenance (optional v1 stubs)

- `POST /v1/maintenance/decay`
- `POST /v1/maintenance/consolidate`

---

## 2) Request/response contracts

### Retrieve request

```json
{
  "space_id": "demo",
  "query_embedding": [0.01, 0.02, 0.03],
  "candidate_k": 200,
  "top_k": 20,
  "hop_depth": 2,
  "allowed_relationship_types": ["ASSOCIATED_WITH","CO_ACTIVATED_WITH","TEMPORALLY_ADJACENT"],
  "max_neighbors_per_node": 50,
  "max_total_edges": 2000,
  "policy_context": {"max_sensitivity": "internal"},
  "debug": false
}
```

### Retrieve response (shape)

```json
{
  "data": {
    "results": [
      {
        "node_id": "...",
        "path": "...",
        "name": "...",
        "summary": "...",
        "score": 0.812,
        "vector_sim": 0.77,
        "activation": 0.61,
        "explanation": {
          "top_contributors": [
            {"src_node_id":"...","rel_type":"ASSOCIATED_WITH","w_eff":0.42,"src_activation":0.55}
          ]
        }
      }
    ]
  }
}
```

---

## 3) Bounded expansion query (Neo4j)

Goal: fetch a *neighborhood subgraph* around candidate seeds while preventing hub explosions.

Approach (v1):

- fetch **direct neighbors** (1 hop) with a per-seed cap
- optionally add **2-hop** edges by expanding from the first-hop nodes (also capped)

Why not variable-length traversal directly? Because it’s hard to enforce per-node caps and easy to explode into the hub dimension.

### 3.1 One-hop (per seed) template

```cypher
UNWIND $seedNodeIds AS sid
MATCH (seed:MemoryNode {space_id:$spaceId, node_id:sid})
CALL {
  WITH seed
  MATCH (seed)-[r]->(nbr:MemoryNode {space_id:$spaceId})
  WHERE type(r) IN $allowedRels AND coalesce(r.status,'active')='active'
  RETURN seed.node_id AS src, nbr.node_id AS dst,
         type(r) AS rel_type,
         r.weight AS weight,
         coalesce(r.dim_semantic,0.0) AS dim_semantic,
         coalesce(r.dim_temporal,0.0) AS dim_temporal,
         coalesce(r.dim_coactivation,0.0) AS dim_coactivation,
         coalesce(r.dim_causal,0.0) AS dim_causal,
         coalesce(r.updated_at, r.created_at) AS updated_at
  ORDER BY coalesce(r.weight,0.0) DESC
  LIMIT $maxNeighborsPerNode
}
RETURN src, dst, rel_type, weight, dim_semantic, dim_temporal, dim_coactivation, dim_causal, updated_at;
```

### 3.2 Two-hop extension

Use the 1-hop result nodes as “frontier” and re-run the same pattern (dedupe in service).

---

## 4) In-memory activation physics

Use the activation rule from `04_Activation_and_Learning.md`:

- transient `a_i ∈ [0,1]`
- seed from vector similarity for top candidates
- run `T` steps over fetched subgraph
- treat `CONTRADICTS` as inhibitory

Implementation detail: store edges in adjacency lists with effective weights computed using config.

---

## 5) Learning deltas writeback (bounded)

We update only what the system “learned” from co-activation among returned nodes.

Rules:

- take top-K nodes with `activation ≥ activation_min_threshold`
- generate unique unordered pairs `(i,j)` and cap updates at `coactivation_update_cap_per_request`
- apply a Hebbian-style update with regularization:
  - `Δw = η * a_i * a_j - μ * w`
  - clamp to `[w_min, w_max]`

Writeback Cypher pattern:

```cypher
UNWIND $pairs AS p
MATCH (a:MemoryNode {space_id:$spaceId, node_id:p.a})
MATCH (b:MemoryNode {space_id:$spaceId, node_id:p.b})
MERGE (a)-[r:CO_ACTIVATED_WITH {space_id:$spaceId}]->(b)
ON CREATE SET r.edge_id=randomUUID(), r.created_at=datetime(), r.updated_at=datetime(),
              r.status='active', r.weight=$createWeight,
              r.dim_coactivation=1.0, r.decay_rate=$defaultDecay, r.evidence_count=1
ON MATCH SET r.updated_at=datetime(), r.evidence_count=r.evidence_count+1
SET r.weight = $newWeight;
```

Note: for symmetric semantics you can also write the reverse edge, or enforce undirected logic at query time.

---

## 6) Readiness gate

The service should refuse “ready” if:

- Neo4j cannot be reached
- required indexes are missing (vector index name)
- `SchemaMeta.current_version < REQUIRED_SCHEMA_VERSION`

This prevents “quietly wrong” retrieval.
