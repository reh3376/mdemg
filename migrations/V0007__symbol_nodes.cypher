// V0007: Symbol Node Support
// Adds SymbolNode for code-level symbol indexing (constants, functions, classes, etc.)
// Enables evidence-locked retrieval by storing exact symbol definitions.
//
// Schema additions:
// - SymbolNode: Represents a code symbol (constant, function, class, etc.)
// - DEFINES_SYMBOL: Relationship from MemoryNode (file) to SymbolNode
//
// SymbolNode properties:
// - space_id: Space identifier
// - symbol_id: Unique symbol identifier (hash of space_id + file_path + name + line)
// - name: Symbol name (e.g., "MAX_CURSOR_COUNT")
// - symbol_type: Type of symbol (constant, function, class, interface, variable, etc.)
// - value: Literal value for constants (e.g., "10000")
// - file_path: Source file path
// - line_number: Line number where symbol is defined
// - signature: Function/method signature if applicable
// - doc_comment: Documentation comment if present
// - type_annotation: Type annotation if present (e.g., "number", "string")
// - embedding: Vector embedding of symbol name + context for semantic search

// Uniqueness constraint: symbol_id must be unique within a space
CREATE CONSTRAINT symbol_id_unique IF NOT EXISTS
FOR (s:SymbolNode) REQUIRE (s.space_id, s.symbol_id) IS UNIQUE;

// B-tree indexes for fast lookups
// Index on name for exact symbol name queries
CREATE INDEX symbol_name_idx IF NOT EXISTS
FOR (s:SymbolNode) ON (s.space_id, s.name);

// Index on symbol_type for filtering by type (constants, functions, etc.)
CREATE INDEX symbol_type_idx IF NOT EXISTS
FOR (s:SymbolNode) ON (s.space_id, s.symbol_type);

// Index on file_path for finding all symbols in a file
CREATE INDEX symbol_filepath_idx IF NOT EXISTS
FOR (s:SymbolNode) ON (s.space_id, s.file_path);

// Composite index for type + name queries (e.g., "find constant named X")
CREATE INDEX symbol_type_name_idx IF NOT EXISTS
FOR (s:SymbolNode) ON (s.space_id, s.symbol_type, s.name);

// Vector index for semantic symbol search
// Dimensions match the embedding model (1536 for text-embedding-3-small / qwen3-embedding:4b)
CREATE VECTOR INDEX symbolNodeEmbedding IF NOT EXISTS
FOR (s:SymbolNode)
ON s.embedding
OPTIONS { indexConfig: {
  `vector.dimensions`: 1536,
  `vector.similarity_function`: 'cosine'
}};

// Fulltext index for fuzzy symbol name matching
// Covers name, value, signature, and doc_comment for comprehensive search
CREATE FULLTEXT INDEX symbolNodeFullText IF NOT EXISTS
FOR (s:SymbolNode)
ON EACH [s.name, s.value, s.signature, s.doc_comment];

// Record migration
MERGE (m:Migration {version: 7})
ON CREATE SET m.name='V0007__symbol_nodes',
              m.applied_at=datetime(),
              m.checksum=null;

// Update schema version
MATCH (s:SchemaMeta {key:'schema'})
WITH s
WHERE coalesce(s.current_version, 0) < 7
SET s.current_version = 7,
    s.updated_at = datetime();
