// Package symbols provides AST-based symbol extraction and Neo4j storage.
package symbols

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// defaultRelBatchSize is the historical UNWIND batch size for SaveRelationships
// (CONFIG-DEADFLAG-001: was a literal 500).
const defaultRelBatchSize = 500

// effectiveRelBatchSize returns the configured relationship batch size,
// falling back to the default (500) when unset or invalid.
func (s *Store) effectiveRelBatchSize() int {
	if s.relBatchSize <= 0 {
		return defaultRelBatchSize
	}
	return s.relBatchSize
}

// RelationshipRecord represents a resolved relationship ready for Neo4j storage.
type RelationshipRecord struct {
	SourceSymbolID   string  `json:"source_symbol_id"`
	TargetSymbolID   string  `json:"target_symbol_id"`
	Relation         string  `json:"relation"`
	SpaceID          string  `json:"space_id"`
	Tier             int     `json:"tier"`
	Confidence       float64 `json:"confidence"`
	ImportPath       string  `json:"import_path,omitempty"`  // IMPORTS only
	CallCount        int     `json:"call_count,omitempty"`   // CALLS only
	ResolutionMethod string  `json:"resolution_method"`      // "ast", "tree_sitter_query", "go_types"
}

// SaveRelationships persists relationship edges in Neo4j.
// Uses separate UNWIND blocks per edge type (no APOC dependency).
// MERGE for idempotency; ON MATCH increments evidence_count.
func (s *Store) SaveRelationships(ctx context.Context, spaceID string, rels []RelationshipRecord) error {
	if len(rels) == 0 {
		return nil
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// Group by relation type
	grouped := make(map[string][]map[string]any)
	for _, rel := range rels {
		m := map[string]any{
			"source_id":         rel.SourceSymbolID,
			"target_id":         rel.TargetSymbolID,
			"space_id":          rel.SpaceID,
			"tier":              rel.Tier,
			"confidence":        rel.Confidence,
			"import_path":       rel.ImportPath,
			"call_count":        rel.CallCount,
			"resolution_method": rel.ResolutionMethod,
		}
		grouped[rel.Relation] = append(grouped[rel.Relation], m)
	}

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		now := time.Now().UTC()

		// Process each relationship type with its own Cypher
		for relType, records := range grouped {
			// CONFIG-DEADFLAG-001: REL_BATCH_SIZE — replaces the literal 500
			// UNWIND batch size.
			batchSize := s.effectiveRelBatchSize()
			for i := 0; i < len(records); i += batchSize {
				end := i + batchSize
				if end > len(records) {
					end = len(records)
				}
				batch := records[i:end]

				cypher := fmt.Sprintf(`
UNWIND $rels AS rel
MATCH (source:SymbolNode {symbol_id: rel.source_id, space_id: rel.space_id})
MATCH (target:SymbolNode {symbol_id: rel.target_id, space_id: rel.space_id})
MERGE (source)-[r:%s {space_id: rel.space_id}]->(target)
ON CREATE SET
    r.created_at = $now,
    r.updated_at = $now,
    r.tier = rel.tier,
    r.confidence = rel.confidence,
    r.import_path = rel.import_path,
    r.call_count = rel.call_count,
    r.resolution_method = rel.resolution_method,
    r.evidence_count = 1,
    r.weight = 1.0,
    r.status = 'active'
ON MATCH SET
    r.updated_at = $now,
    r.evidence_count = r.evidence_count + 1,
    r.confidence = CASE WHEN rel.confidence > r.confidence THEN rel.confidence ELSE r.confidence END
`, relType)

				_, err := tx.Run(ctx, cypher, map[string]any{
					"rels": batch,
					"now":  now,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to save %s relationships: %w", relType, err)
				}
			}
		}

		return nil, nil
	})

	if err != nil {
		return fmt.Errorf("SaveRelationships: %w", err)
	}

	slog.Info("saved relationships", "count", len(rels), "types", len(grouped), "space_id", spaceID)
	return nil
}

// QueryRelationships retrieves relationships for a symbol.
func (s *Store) QueryRelationships(ctx context.Context, spaceID, symbolID string) ([]RelationshipRecord, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {symbol_id: $symbol_id, space_id: $space_id})-[r]->(t:SymbolNode)
WHERE type(r) IN ['IMPORTS', 'CALLS', 'EXTENDS', 'IMPLEMENTS']
RETURN s.symbol_id AS source_id, t.symbol_id AS target_id, type(r) AS relation,
       r.tier AS tier, r.confidence AS confidence, r.import_path AS import_path,
       r.call_count AS call_count, r.resolution_method AS resolution_method
UNION
MATCH (s:SymbolNode)-[r]->(t:SymbolNode {symbol_id: $symbol_id, space_id: $space_id})
WHERE type(r) IN ['IMPORTS', 'CALLS', 'EXTENDS', 'IMPLEMENTS']
RETURN s.symbol_id AS source_id, t.symbol_id AS target_id, type(r) AS relation,
       r.tier AS tier, r.confidence AS confidence, r.import_path AS import_path,
       r.call_count AS call_count, r.resolution_method AS resolution_method
`, map[string]any{
			"symbol_id": symbolID,
			"space_id":  spaceID,
		})
		if err != nil {
			return nil, err
		}

		var records []RelationshipRecord
		for res.Next(ctx) {
			record := res.Record()
			rel := RelationshipRecord{
				SpaceID: spaceID,
			}
			if v, ok := record.Get("source_id"); ok && v != nil {
				rel.SourceSymbolID = v.(string)
			}
			if v, ok := record.Get("target_id"); ok && v != nil {
				rel.TargetSymbolID = v.(string)
			}
			if v, ok := record.Get("relation"); ok && v != nil {
				rel.Relation = v.(string)
			}
			if v, ok := record.Get("tier"); ok && v != nil {
				rel.Tier = int(v.(int64))
			}
			if v, ok := record.Get("confidence"); ok && v != nil {
				rel.Confidence = v.(float64)
			}
			if v, ok := record.Get("import_path"); ok && v != nil {
				rel.ImportPath = v.(string)
			}
			if v, ok := record.Get("call_count"); ok && v != nil {
				rel.CallCount = int(v.(int64))
			}
			if v, ok := record.Get("resolution_method"); ok && v != nil {
				rel.ResolutionMethod = v.(string)
			}
			records = append(records, rel)
		}
		return records, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]RelationshipRecord), nil
}

// RelationshipStats returns counts of relationships by type for a space.
func (s *Store) RelationshipStats(ctx context.Context, spaceID string) (map[string]int64, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		stats := make(map[string]int64)
		for _, relType := range []string{"IMPORTS", "CALLS", "EXTENDS", "IMPLEMENTS"} {
			cypher := fmt.Sprintf(`
MATCH (:SymbolNode {space_id: $space_id})-[r:%s]->(:SymbolNode)
RETURN count(r) AS cnt
`, relType)
			res, err := tx.Run(ctx, cypher, map[string]any{"space_id": spaceID})
			if err != nil {
				return nil, err
			}
			if res.Next(ctx) {
				if v, ok := res.Record().Get("cnt"); ok {
					stats[relType] = v.(int64)
				}
			}
		}
		return stats, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(map[string]int64), nil
}
