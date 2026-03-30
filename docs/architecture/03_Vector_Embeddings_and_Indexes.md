# Vector Embeddings + Neo4j Vector Indexes (Native)

Neo4j supports vector indexing and querying for similarity search over embeddings, and the Neo4j **GenAI plugin**
can generate/store embeddings directly inside the database.

## A) Store embeddings on nodes

Embeddings are stored as a list property (and in newer Neo4j versions can be stored as the `VECTOR` property type).

### Recommended properties

- `:MemoryNode.embedding` — embedding for node summary/description
- Optional: `:Observation.embedding` — embedding for raw event chunk text

## B) Create a vector index (node index)

Example uses cosine similarity, typical for text embeddings.

```cypher
CREATE VECTOR INDEX memNodeEmbedding IF NOT EXISTS
FOR (n:MemoryNode)
ON n.embedding
OPTIONS { indexConfig: {
  `vector.dimensions`: 3072,  // text-embedding-3-large (Recommended)
  `vector.similarity_function`: 'cosine'
}};
```

**Note:** Production uses `text-embedding-3-large` at **3072 dimensions**. Ollama model `qwen3-embedding:8b` natively produces 4096 dimensions but supports Matryoshka Representation Learning (MRL) truncation to 3072 for index compatibility.

## C) Query the vector index

```cypher
WITH $queryEmbedding AS q
CALL db.index.vector.queryNodes('memNodeEmbedding', $k, q)
YIELD node, score
RETURN node.node_id AS node_id, node.path AS path, node.summary AS summary, score
ORDER BY score DESC;
```

## D) Generating embeddings with the Neo4j GenAI plugin

### Single value encode + store (OpenAI)

```cypher
MATCH (n:MemoryNode {space_id:$spaceId, node_id:$nodeId})
WITH n, (n.name || ' ' || coalesce(n.description,'') || ' ' || coalesce(n.summary,'')) AS text
WITH n, genai.vector.encode(text, 'OpenAI', { 
  token: $openaiToken, 
  model: 'text-embedding-3-large'
}) AS v
CALL db.create.setNodeVectorProperty(n, 'embedding', v)
RETURN n.node_id, size(n.embedding) AS dims;
```

### Batch encode (high throughput)

```cypher
MATCH (n:MemoryNode {space_id:$spaceId})
WHERE n.summary IS NOT NULL
WITH collect(n) AS nodes, 200 AS batchSize, count(*) AS total
UNWIND range(0, total-1, batchSize) AS batchStart
CALL (nodes, batchStart, batchSize) {
  WITH [x IN nodes[batchStart .. batchStart + batchSize]
        | x.name || ': ' || coalesce(x.summary,'')] AS batch
  CALL genai.vector.encodeBatch(batch, 'OpenAI', { 
    token: $openaiToken,
    model: 'text-embedding-3-small'
  }) YIELD index, vector
  CALL db.create.setNodeVectorProperty(nodes[batchStart + index], 'embedding', vector)
} IN CONCURRENT TRANSACTIONS OF 1 ROW;
```

## E) Edge embeddings (optional)

If you embed relationships (e.g., edge “meaning”), you can create a relationship vector index:

```cypher
CREATE VECTOR INDEX relEmbedding IF NOT EXISTS
FOR ()-[r:ASSOCIATED_WITH]-()
ON (r.embedding)
OPTIONS { indexConfig: { `vector.dimensions`: 3072, `vector.similarity_function`: 'cosine' }};
```

## F) Version + Java performance note

Neo4j vector indexes are Lucene-backed; Neo4j also documents an optional speedup via the incubated Java Vector API
(add JVM module flags). This matters at scale, and should be tracked in ops notes.
