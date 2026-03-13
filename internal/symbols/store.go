// Package symbols provides AST-based symbol extraction and Neo4j storage.
package symbols

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Store provides Neo4j persistence for symbols.
type Store struct {
	driver neo4j.DriverWithContext
}

// NewStore creates a new symbol store with the given Neo4j driver.
func NewStore(driver neo4j.DriverWithContext) *Store {
	return &Store{driver: driver}
}

// SymbolRecord represents a symbol ready for Neo4j storage.
// Field names follow UPTS (Universal Parser Test Specification) v1.0.0
type SymbolRecord struct {
	SpaceID            string    `json:"space_id"`
	SymbolID           string    `json:"symbol_id"`
	Name               string    `json:"name"`
	SymbolType         string    `json:"symbol_type"`
	Value              string    `json:"value,omitempty"`
	RawValue           string    `json:"raw_value,omitempty"`
	FilePath           string    `json:"file_path"`
	ParentNodeID       string    `json:"parent_node_id,omitempty"` // Direct link to MemoryNode (avoids MATCH)
	Line               int       `json:"line"`                     // 1-indexed start line (UPTS standard)
	LineEnd            int       `json:"line_end"`                 // End line for multi-line symbols
	Column             int       `json:"column,omitempty"`
	Exported           bool      `json:"exported"`
	DocComment         string    `json:"doc_comment,omitempty"`
	Signature          string    `json:"signature,omitempty"`
	Parent             string    `json:"parent,omitempty"`
	Language           string    `json:"language"`
	TypeAnnotation     string    `json:"type_annotation,omitempty"`
	Embedding          []float32 `json:"embedding,omitempty"`
	ContextSpecificity float64   `json:"context_specificity,omitempty"` // 0.0-1.0 indicating how context-specific this symbol is
}

// GenerateSymbolID creates a unique identifier for a symbol.
func GenerateSymbolID(spaceID, filePath, name string, lineNumber int) string {
	data := fmt.Sprintf("%s|%s|%s|%d", spaceID, filePath, name, lineNumber)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:16]) // 32 char hex string
}

// ToRecord converts a Symbol to a SymbolRecord for storage.
func ToRecord(spaceID string, sym Symbol) SymbolRecord {
	return SymbolRecord{
		SpaceID:        spaceID,
		SymbolID:       GenerateSymbolID(spaceID, sym.FilePath, sym.Name, sym.Line),
		Name:           sym.Name,
		SymbolType:     string(sym.Type),
		Value:          sym.Value,
		RawValue:       sym.RawValue,
		FilePath:       sym.FilePath,
		Line:    sym.Line,    // UPTS standard: 1-indexed line
		LineEnd: sym.LineEnd, // UPTS standard: end line
		Column:         sym.Column,
		Exported:       sym.Exported,
		DocComment:     sym.DocComment,
		Signature:      sym.Signature,
		Parent:         sym.Parent,
		Language:       string(sym.Language),
		TypeAnnotation: sym.TypeAnnotation,
	}
}

// calculateContextSpecificity determines how context-specific a symbol is.
// High specificity (0.8-1.0): Constants with specific numeric values, config values
// Medium specificity (0.5-0.7): Exported constants, enum values
// Low specificity (0.0-0.4): Generic variable names, common patterns
func calculateContextSpecificity(sym SymbolRecord) float64 {
	specificity := 0.5 // Base specificity

	// Constants with specific values are highly context-specific
	if sym.SymbolType == "const" {
		specificity += 0.2

		// Check if it has a specific numeric value (not 0 or 1)
		if sym.Value != "" && sym.Value != "0" && sym.Value != "1" {
			// Check for numeric values
			if matched, _ := regexp.MatchString(`^\d+$`, sym.Value); matched {
				specificity += 0.2 // Specific numeric value
			}
		}
	}

	// ALL_CAPS names indicate configuration constants - highly specific
	if isAllCaps(sym.Name) {
		specificity += 0.15
	}

	// Exported symbols are more likely to be important
	if sym.Exported {
		specificity += 0.1
	}

	// Enum values are moderately specific
	if sym.SymbolType == "enum_value" {
		specificity += 0.1
	}

	// Having a doc comment indicates intentional documentation
	if sym.DocComment != "" {
		specificity += 0.1
	}

	// Cap at 1.0
	if specificity > 1.0 {
		specificity = 1.0
	}

	return specificity
}

// isAllCaps checks if a string is in ALL_CAPS format (with underscores)
func isAllCaps(s string) bool {
	if len(s) < 2 {
		return false
	}
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return strings.Contains(s, "_") // Must have at least one underscore
}

// SaveSymbols stores symbols in Neo4j and links them to their parent MemoryNode.
// This is a batch operation for efficiency during ingestion.
func (s *Store) SaveSymbols(ctx context.Context, spaceID string, symbols []SymbolRecord) error {
	if len(symbols) == 0 {
		return nil
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Convert symbols to map format for Cypher
		symbolMaps := make([]map[string]any, len(symbols))
		for i, sym := range symbols {
			// Calculate context specificity if not already set
			contextSpec := sym.ContextSpecificity
			if contextSpec == 0 {
				contextSpec = calculateContextSpecificity(sym)
			}
			symbolMaps[i] = map[string]any{
				"space_id":            sym.SpaceID,
				"symbol_id":           sym.SymbolID,
				"name":                sym.Name,
				"symbol_type":         sym.SymbolType,
				"value":               sym.Value,
				"raw_value":           sym.RawValue,
				"file_path":           sym.FilePath,
				"parent_node_id":      sym.ParentNodeID,
				"line":                sym.Line,
				"line_end":            sym.LineEnd,
				"column":              sym.Column,
				"exported":            sym.Exported,
				"doc_comment":         sym.DocComment,
				"signature":           sym.Signature,
				"parent":              sym.Parent,
				"language":            sym.Language,
				"type_annotation":     sym.TypeAnnotation,
				"embedding":           sym.Embedding,
				"context_specificity": contextSpec,
			}
		}

		params := map[string]any{
			"space_id": spaceID,
			"symbols":  symbolMaps,
		}

		// Batch upsert symbols and link to parent MemoryNode
		_, err := tx.Run(ctx, `
UNWIND $symbols AS sym
MERGE (s:SymbolNode {space_id: sym.space_id, symbol_id: sym.symbol_id})
ON CREATE SET
  s.created_at = datetime(),
  s.updated_at = datetime()
ON MATCH SET
  s.updated_at = datetime()
SET
  s.name = sym.name,
  s.symbol_type = sym.symbol_type,
  s.value = sym.value,
  s.raw_value = sym.raw_value,
  s.file_path = sym.file_path,
  s.line = sym.line,
  s.line_end = sym.line_end,
  s.column = sym.column,
  s.exported = sym.exported,
  s.doc_comment = sym.doc_comment,
  s.signature = sym.signature,
  s.parent = sym.parent,
  s.language = sym.language,
  s.type_annotation = sym.type_annotation,
  s.context_specificity = sym.context_specificity
WITH s, sym
// Set embedding only if provided (using FOREACH to avoid filtering)
FOREACH (_ IN CASE WHEN sym.embedding IS NOT NULL AND size(sym.embedding) > 0 THEN [1] ELSE [] END |
  SET s.embedding = sym.embedding
)
WITH s, sym
// Link symbol to parent MemoryNode
// Prefer direct node_id link (avoids transaction isolation issues)
// Fall back to path matching if node_id not provided
OPTIONAL MATCH (m1:MemoryNode {space_id: sym.space_id, node_id: sym.parent_node_id})
WHERE sym.parent_node_id IS NOT NULL AND sym.parent_node_id <> ''
OPTIONAL MATCH (m2:MemoryNode {space_id: sym.space_id, path: sym.file_path})
WHERE sym.parent_node_id IS NULL OR sym.parent_node_id = ''
WITH s, sym, coalesce(m1, m2) AS m
WHERE m IS NOT NULL
MERGE (m)-[:DEFINES_SYMBOL {space_id: sym.space_id}]->(s)
RETURN count(s) AS created
		`, params)
		return nil, err
	})
	return err
}

// SaveSymbolsWithEmbeddings saves symbols that already have embeddings.
func (s *Store) SaveSymbolsWithEmbeddings(ctx context.Context, spaceID string, symbols []SymbolRecord) error {
	return s.SaveSymbols(ctx, spaceID, symbols)
}

// QueryByName finds symbols matching the given name pattern.
func (s *Store) QueryByName(ctx context.Context, spaceID, name string, limit int) ([]SymbolRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id": spaceID,
			"name":     name,
			"limit":    limit,
		}

		res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id})
WHERE s.name = $name OR s.name STARTS WITH $name
RETURN s
ORDER BY s.context_specificity DESC, s.name
LIMIT $limit
		`, params)
		if err != nil {
			return nil, err
		}

		var symbols []SymbolRecord
		for res.Next(ctx) {
			record := res.Record()
			node, _ := record.Get("s")
			if n, ok := node.(neo4j.Node); ok {
				symbols = append(symbols, nodeToSymbolRecord(n))
			}
		}
		return symbols, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SymbolRecord), nil
}

// QueryByType finds symbols of a specific type.
func (s *Store) QueryByType(ctx context.Context, spaceID, symbolType string, limit int) ([]SymbolRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id":    spaceID,
			"symbol_type": symbolType,
			"limit":       limit,
		}

		res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, symbol_type: $symbol_type})
RETURN s
ORDER BY s.context_specificity DESC, s.name
LIMIT $limit
		`, params)
		if err != nil {
			return nil, err
		}

		var symbols []SymbolRecord
		for res.Next(ctx) {
			record := res.Record()
			node, _ := record.Get("s")
			if n, ok := node.(neo4j.Node); ok {
				symbols = append(symbols, nodeToSymbolRecord(n))
			}
		}
		return symbols, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SymbolRecord), nil
}

// QueryByFile finds all symbols in a specific file.
func (s *Store) QueryByFile(ctx context.Context, spaceID, filePath string) ([]SymbolRecord, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id":  spaceID,
			"file_path": filePath,
		}

		res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, file_path: $file_path})
RETURN s
ORDER BY s.line
		`, params)
		if err != nil {
			return nil, err
		}

		var symbols []SymbolRecord
		for res.Next(ctx) {
			record := res.Record()
			node, _ := record.Get("s")
			if n, ok := node.(neo4j.Node); ok {
				symbols = append(symbols, nodeToSymbolRecord(n))
			}
		}
		return symbols, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SymbolRecord), nil
}

// VectorSearch finds symbols similar to the given embedding.
func (s *Store) VectorSearch(ctx context.Context, spaceID string, embedding []float32, limit int) ([]SymbolSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id":  spaceID,
			"embedding": embedding,
			"limit":     limit,
		}

		res, err := tx.Run(ctx, `
CALL db.index.vector.queryNodes('symbolNodeEmbedding', $limit, $embedding)
YIELD node, score
WHERE node.space_id = $space_id
RETURN node, score
ORDER BY score DESC
		`, params)
		if err != nil {
			return nil, err
		}

		var results []SymbolSearchResult
		for res.Next(ctx) {
			record := res.Record()
			node, _ := record.Get("node")
			score, _ := record.Get("score")
			if n, ok := node.(neo4j.Node); ok {
				results = append(results, SymbolSearchResult{
					Symbol: nodeToSymbolRecord(n),
					Score:  score.(float64),
				})
			}
		}
		return results, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SymbolSearchResult), nil
}

// FulltextSearch performs fulltext search on symbol names, values, and signatures.
func (s *Store) FulltextSearch(ctx context.Context, spaceID, query string, limit int) ([]SymbolSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id": spaceID,
			"query":    query,
			"limit":    limit,
		}

		res, err := tx.Run(ctx, `
CALL db.index.fulltext.queryNodes('symbolNodeFullText', $query)
YIELD node, score
WHERE node.space_id = $space_id
RETURN node, score
ORDER BY score DESC
LIMIT $limit
		`, params)
		if err != nil {
			return nil, err
		}

		var results []SymbolSearchResult
		for res.Next(ctx) {
			record := res.Record()
			node, _ := record.Get("node")
			score, _ := record.Get("score")
			if n, ok := node.(neo4j.Node); ok {
				results = append(results, SymbolSearchResult{
					Symbol: nodeToSymbolRecord(n),
					Score:  score.(float64),
				})
			}
		}
		return results, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SymbolSearchResult), nil
}

// FindExactConstant finds a constant by exact name and optionally type.
func (s *Store) FindExactConstant(ctx context.Context, spaceID, name string) ([]SymbolRecord, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id": spaceID,
			"name":     name,
		}

		res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, name: $name})
WHERE s.symbol_type IN ['const', 'var', 'enum_value']
RETURN s
ORDER BY s.context_specificity DESC, s.exported DESC, s.line
		`, params)
		if err != nil {
			return nil, err
		}

		var symbols []SymbolRecord
		for res.Next(ctx) {
			record := res.Record()
			node, _ := record.Get("s")
			if n, ok := node.(neo4j.Node); ok {
				symbols = append(symbols, nodeToSymbolRecord(n))
			}
		}
		return symbols, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SymbolRecord), nil
}

// GetSymbolsForMemoryNode returns all symbols defined in a MemoryNode.
// For hidden/concept nodes (layer > 0), traverses GENERALIZES edges to find
// symbols from descendant leaf nodes.
func (s *Store) GetSymbolsForMemoryNode(ctx context.Context, spaceID, nodeID string) ([]SymbolRecord, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id":    spaceID,
			"node_id":     nodeID,
			"max_symbols": 20,
		}

		// Query handles symbols from multiple sources:
		// 1. Direct symbols on the node (leaf nodes)
		// 2. Symbols from descendant leaf nodes via GENERALIZES (hidden/concept nodes)
		// 3. Symbols from parent file node for fragment nodes (path contains #)
		res, err := tx.Run(ctx, `
MATCH (m:MemoryNode {space_id: $space_id, node_id: $node_id})

// Collect direct symbols
OPTIONAL MATCH (m)-[:DEFINES_SYMBOL]->(s1:SymbolNode)
WITH m, collect(DISTINCT s1) AS directSymbols

// Collect symbols from descendant leaf nodes (for hidden/concept nodes)
OPTIONAL MATCH (leaf:MemoryNode {layer: 0})-[:GENERALIZES*]->(m)
OPTIONAL MATCH (leaf)-[:DEFINES_SYMBOL]->(s2:SymbolNode)
WITH m, directSymbols, collect(DISTINCT s2) AS descendantSymbols

// Collect symbols from parent file node (for fragment nodes with # in path)
OPTIONAL MATCH (parent:MemoryNode {space_id: $space_id, layer: 0})
WHERE m.path CONTAINS '#' AND parent.path = split(m.path, '#')[0]
OPTIONAL MATCH (parent)-[:DEFINES_SYMBOL]->(s3:SymbolNode)
WITH directSymbols, descendantSymbols, collect(DISTINCT s3) AS parentSymbols

// Combine all symbol sources
WITH directSymbols + descendantSymbols + parentSymbols AS allSymbols
UNWIND allSymbols AS s
WITH s WHERE s IS NOT NULL
RETURN s
ORDER BY s.context_specificity DESC, s.line
LIMIT $max_symbols
		`, params)
		if err != nil {
			return nil, err
		}

		var symbols []SymbolRecord
		for res.Next(ctx) {
			record := res.Record()
			node, _ := record.Get("s")
			if n, ok := node.(neo4j.Node); ok {
				symbols = append(symbols, nodeToSymbolRecord(n))
			}
		}
		return symbols, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]SymbolRecord), nil
}

// GetSymbolsForMemoryNodes returns symbols for multiple MemoryNodes in a single query.
// This avoids N+1 session overhead by using UNWIND + CALL {} subquery.
// Returns a map of nodeID -> []SymbolRecord.
func (s *Store) GetSymbolsForMemoryNodes(ctx context.Context, spaceID string, nodeIDs []string) (map[string][]SymbolRecord, error) {
	if len(nodeIDs) == 0 {
		return map[string][]SymbolRecord{}, nil
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
UNWIND $node_ids AS nid
CALL {
  WITH nid
  MATCH (m:MemoryNode {space_id: $space_id, node_id: nid})

  // Direct symbols
  OPTIONAL MATCH (m)-[:DEFINES_SYMBOL]->(s1:SymbolNode)
  WITH m, collect(DISTINCT s1) AS directSymbols

  // Descendant symbols via GENERALIZES (for hidden/concept nodes)
  OPTIONAL MATCH (leaf:MemoryNode {layer: 0})-[:GENERALIZES*]->(m)
  OPTIONAL MATCH (leaf)-[:DEFINES_SYMBOL]->(s2:SymbolNode)
  WITH m, directSymbols, collect(DISTINCT s2) AS descendantSymbols

  // Parent file symbols (for fragment nodes with # in path)
  OPTIONAL MATCH (parent:MemoryNode {space_id: $space_id, layer: 0})
  WHERE m.path CONTAINS '#' AND parent.path = split(m.path, '#')[0]
  OPTIONAL MATCH (parent)-[:DEFINES_SYMBOL]->(s3:SymbolNode)
  WITH m.node_id AS nodeId, directSymbols, descendantSymbols, collect(DISTINCT s3) AS parentSymbols

  // Combine, filter nulls, order, limit per node
  WITH nodeId, directSymbols + descendantSymbols + parentSymbols AS allSymbols
  UNWIND allSymbols AS s
  WITH nodeId, s WHERE s IS NOT NULL
  WITH nodeId, s ORDER BY s.context_specificity DESC, s.line
  RETURN nodeId, collect(s)[..20] AS syms
}
RETURN nodeId, syms
		`, map[string]any{
			"space_id": spaceID,
			"node_ids": nodeIDs,
		})
		if err != nil {
			return nil, err
		}

		resultMap := make(map[string][]SymbolRecord, len(nodeIDs))
		for res.Next(ctx) {
			record := res.Record()
			nid, _ := record.Get("nodeId")
			symsAny, _ := record.Get("syms")
			nodeID, _ := nid.(string)
			if symList, ok := symsAny.([]any); ok {
				records := make([]SymbolRecord, 0, len(symList))
				for _, sym := range symList {
					if n, ok := sym.(neo4j.Node); ok {
						records = append(records, nodeToSymbolRecord(n))
					}
				}
				if len(records) > 0 {
					resultMap[nodeID] = records
				}
			}
		}
		return resultMap, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string][]SymbolRecord), nil
}

// DeleteSymbolsForFile removes all symbols associated with a file.
// Used when re-indexing a file.
func (s *Store) DeleteSymbolsForFile(ctx context.Context, spaceID, filePath string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id":  spaceID,
			"file_path": filePath,
		}

		_, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, file_path: $file_path})
DETACH DELETE s
		`, params)
		return nil, err
	})
	return err
}

// GetStats returns statistics about symbols in a space.
func (s *Store) GetStats(ctx context.Context, spaceID string) (*SymbolStats, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{
			"space_id": spaceID,
		}

		res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id})
WITH count(s) AS total,
     count(DISTINCT s.file_path) AS files,
     collect(DISTINCT s.symbol_type) AS types
OPTIONAL MATCH (s2:SymbolNode {space_id: $space_id})
WHERE s2.embedding IS NOT NULL
WITH total, files, types, count(s2) AS with_embeddings
RETURN total, files, types, with_embeddings
		`, params)
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			record := res.Record()
			total, _ := record.Get("total")
			files, _ := record.Get("files")
			types, _ := record.Get("types")
			withEmb, _ := record.Get("with_embeddings")

			typeStrs := []string{}
			if typeList, ok := types.([]any); ok {
				for _, t := range typeList {
					if ts, ok := t.(string); ok {
						typeStrs = append(typeStrs, ts)
					}
				}
			}

			return &SymbolStats{
				TotalSymbols:         total.(int64),
				FilesWithSymbols:     files.(int64),
				SymbolTypes:          typeStrs,
				SymbolsWithEmbedding: withEmb.(int64),
			}, nil
		}
		return &SymbolStats{}, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(*SymbolStats), nil
}

// SymbolSearchResult pairs a symbol with its search score.
type SymbolSearchResult struct {
	Symbol SymbolRecord `json:"symbol"`
	Score  float64      `json:"score"`
}

// SymbolStats contains statistics about symbols in a space.
type SymbolStats struct {
	TotalSymbols         int64    `json:"total_symbols"`
	FilesWithSymbols     int64    `json:"files_with_symbols"`
	SymbolTypes          []string `json:"symbol_types"`
	SymbolsWithEmbedding int64    `json:"symbols_with_embedding"`
}

// nodeToSymbolRecord converts a Neo4j node to a SymbolRecord.
func nodeToSymbolRecord(n neo4j.Node) SymbolRecord {
	props := n.Props
	return SymbolRecord{
		SpaceID:            getString(props, "space_id"),
		SymbolID:           getString(props, "symbol_id"),
		Name:               getString(props, "name"),
		SymbolType:         getString(props, "symbol_type"),
		Value:              getString(props, "value"),
		RawValue:           getString(props, "raw_value"),
		FilePath:           getString(props, "file_path"),
		Line:    getInt(props, "line"),    // UPTS standard
		LineEnd: getInt(props, "line_end"), // UPTS standard
		Column:             getInt(props, "column"),
		Exported:           getBool(props, "exported"),
		DocComment:         getString(props, "doc_comment"),
		Signature:          getString(props, "signature"),
		Parent:             getString(props, "parent"),
		Language:           getString(props, "language"),
		TypeAnnotation:     getString(props, "type_annotation"),
		Embedding:          getFloat32Slice(props, "embedding"),
		ContextSpecificity: getFloat64(props, "context_specificity"),
	}
}

func getString(props map[string]any, key string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(props map[string]any, key string) int {
	if v, ok := props[key]; ok {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}

func getFloat64(props map[string]any, key string) float64 {
	if v, ok := props[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		case int:
			return float64(n)
		}
	}
	return 0
}

func getBool(props map[string]any, key string) bool {
	if v, ok := props[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getFloat32Slice(props map[string]any, key string) []float32 {
	if v, ok := props[key]; ok {
		switch arr := v.(type) {
		case []float32:
			return arr
		case []float64:
			result := make([]float32, len(arr))
			for i, f := range arr {
				result[i] = float32(f)
			}
			return result
		case []any:
			result := make([]float32, len(arr))
			for i, f := range arr {
				if fv, ok := f.(float64); ok {
					result[i] = float32(fv)
				}
			}
			return result
		}
	}
	return nil
}
