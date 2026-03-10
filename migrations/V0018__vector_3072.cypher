// V0018 — Upgrade vector indexes to 3072 dimensions (text-embedding-3-large)
// Drops and recreates all vector indexes with 3072 dimensions.
// Existing embeddings with different dimensions will need to be regenerated.

DROP INDEX memNodeEmbedding IF EXISTS;

CREATE VECTOR INDEX memNodeEmbedding IF NOT EXISTS
FOR (n:MemoryNode)
ON n.embedding
OPTIONS { indexConfig: {
  `vector.dimensions`: 3072,
  `vector.similarity_function`: 'cosine'
}};

DROP INDEX observationEmbedding IF EXISTS;

CREATE VECTOR INDEX observationEmbedding IF NOT EXISTS
FOR (o:Observation)
ON o.embedding
OPTIONS { indexConfig: {
  `vector.dimensions`: 3072,
  `vector.similarity_function`: 'cosine'
}};

DROP INDEX symbolNodeEmbedding IF EXISTS;

CREATE VECTOR INDEX symbolNodeEmbedding IF NOT EXISTS
FOR (s:SymbolNode)
ON s.embedding
OPTIONS { indexConfig: {
  `vector.dimensions`: 3072,
  `vector.similarity_function`: 'cosine'
}};

MERGE (m:Migration {version: 18})
ON CREATE SET m.name='V0018__vector_3072',
              m.applied_at=datetime(),
              m.checksum=null;

MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version,0) < 18
SET s.current_version = 18,
    s.updated_at = datetime();
