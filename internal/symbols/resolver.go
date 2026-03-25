// Package symbols provides AST-based symbol extraction and Neo4j storage.
package symbols

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Resolver resolves raw relationships (from parser) into RelationshipRecords
// by matching source/target names to actual SymbolNode IDs in Neo4j.
type Resolver struct {
	driver neo4j.DriverWithContext
}

// NewResolver creates a new relationship resolver.
func NewResolver(driver neo4j.DriverWithContext) *Resolver {
	return &Resolver{driver: driver}
}

// Resolve takes raw relationships from parsing and resolves them to SymbolNode pairs.
// Resolution priority:
// 1. Same-file (confidence 1.0)
// 2. Same-package/directory (confidence 0.9)
// 3. Import-resolved (confidence 0.8)
// 4. Global unique match (confidence 0.5)
// 5. Unresolved → skip
func (r *Resolver) Resolve(ctx context.Context, spaceID string, rels []Relationship) ([]RelationshipRecord, error) {
	if len(rels) == 0 {
		return nil, nil
	}

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	var resolved []RelationshipRecord
	var unresolved int

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		var results []RelationshipRecord

		for _, rel := range rels {
			// For IMPORTS, resolve source file to a SymbolNode and target to an imported file's SymbolNode
			if rel.Relation == RelImports {
				// Resolve source: find any symbol in the source file
				sourceRec, _ := r.resolveFirstInFile(ctx, tx, spaceID, rel.SourceFile)
				if sourceRec == nil {
					unresolved++
					continue
				}
				// Resolve target: try to find a symbol in a file matching the import path
				targetRec, _ := r.resolveImportTarget(ctx, tx, spaceID, rel.Target)
				if targetRec == nil {
					unresolved++
					continue
				}
				results = append(results, RelationshipRecord{
					SourceSymbolID:   sourceRec.TargetSymbolID,
					TargetSymbolID:   targetRec.TargetSymbolID,
					Relation:         rel.Relation,
					SpaceID:          spaceID,
					Tier:             rel.Tier,
					Confidence:       0.8,
					ImportPath:       rel.Target,
					ResolutionMethod: "tree_sitter_query",
				})
				continue
			}

			// For CALLS, EXTENDS, IMPLEMENTS — resolve both source and target to SymbolNodes
			// Resolve source symbol (the caller/parent)
			var sourceSymbolID string
			if rel.Source != "" {
				srcRec, _ := r.resolveInFile(ctx, tx, spaceID, rel.Source, rel.SourceFile)
				if srcRec != nil {
					sourceSymbolID = srcRec.TargetSymbolID
				}
			}
			if sourceSymbolID == "" {
				// Fallback: use first symbol in the source file
				srcRec, _ := r.resolveFirstInFile(ctx, tx, spaceID, rel.SourceFile)
				if srcRec != nil {
					sourceSymbolID = srcRec.TargetSymbolID
				}
			}
			if sourceSymbolID == "" {
				unresolved++
				continue
			}

			targetName := rel.Target

			// Strategy 1: Same file
			record, conf := r.resolveInFile(ctx, tx, spaceID, targetName, rel.SourceFile)
			if record != nil {
				results = append(results, RelationshipRecord{
					SourceSymbolID:   sourceSymbolID,
					TargetSymbolID:   record.TargetSymbolID,
					Relation:         rel.Relation,
					SpaceID:          spaceID,
					Tier:             rel.Tier,
					Confidence:       conf,
					ResolutionMethod: "same_file",
				})
				continue
			}

			// Strategy 2: Same package/directory
			dir := filepath.Dir(rel.SourceFile)
			record, conf = r.resolveInPackage(ctx, tx, spaceID, targetName, dir)
			if record != nil {
				results = append(results, RelationshipRecord{
					SourceSymbolID:   sourceSymbolID,
					TargetSymbolID:   record.TargetSymbolID,
					Relation:         rel.Relation,
					SpaceID:          spaceID,
					Tier:             rel.Tier,
					Confidence:       conf,
					ResolutionMethod: "same_package",
				})
				continue
			}

			// Strategy 3: Global unique match
			record, conf = r.resolveGlobal(ctx, tx, spaceID, targetName)
			if record != nil {
				results = append(results, RelationshipRecord{
					SourceSymbolID:   sourceSymbolID,
					TargetSymbolID:   record.TargetSymbolID,
					Relation:         rel.Relation,
					SpaceID:          spaceID,
					Tier:             rel.Tier,
					Confidence:       conf,
					ResolutionMethod: "global_unique",
				})
				continue
			}

			unresolved++
		}

		return results, nil
	})

	if err != nil {
		return nil, fmt.Errorf("Resolve: %w", err)
	}

	resolved = result.([]RelationshipRecord)
	if unresolved > 0 {
		slog.Info("resolver: resolution complete", "resolved", len(resolved), "unresolved", unresolved, "space_id", spaceID)
	}

	return resolved, nil
}

func (r *Resolver) resolveInFile(ctx context.Context, tx neo4j.ManagedTransaction, spaceID, name, filePath string) (*RelationshipRecord, float64) {
	res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, name: $name, file_path: $file_path})
RETURN s.symbol_id AS symbol_id
LIMIT 1
`, map[string]any{
		"space_id":  spaceID,
		"name":      name,
		"file_path": filePath,
	})
	if err != nil {
		return nil, 0
	}
	if res.Next(ctx) {
		if v, ok := res.Record().Get("symbol_id"); ok && v != nil {
			return &RelationshipRecord{TargetSymbolID: v.(string)}, 1.0
		}
	}
	return nil, 0
}

func (r *Resolver) resolveInPackage(ctx context.Context, tx neo4j.ManagedTransaction, spaceID, name, dir string) (*RelationshipRecord, float64) {
	// Match symbols in the same directory (package)
	res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, name: $name})
WHERE s.file_path STARTS WITH $dir_prefix
RETURN s.symbol_id AS symbol_id
LIMIT 1
`, map[string]any{
		"space_id":   spaceID,
		"name":       name,
		"dir_prefix": strings.TrimSuffix(dir, "/") + "/",
	})
	if err != nil {
		return nil, 0
	}
	if res.Next(ctx) {
		if v, ok := res.Record().Get("symbol_id"); ok && v != nil {
			return &RelationshipRecord{TargetSymbolID: v.(string)}, 0.9
		}
	}
	return nil, 0
}

func (r *Resolver) resolveGlobal(ctx context.Context, tx neo4j.ManagedTransaction, spaceID, name string) (*RelationshipRecord, float64) {
	// Only resolve if there's exactly one match (unambiguous)
	res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, name: $name})
WITH collect(s) AS matches
WHERE size(matches) = 1
RETURN matches[0].symbol_id AS symbol_id
`, map[string]any{
		"space_id": spaceID,
		"name":     name,
	})
	if err != nil {
		return nil, 0
	}
	if res.Next(ctx) {
		if v, ok := res.Record().Get("symbol_id"); ok && v != nil {
			return &RelationshipRecord{TargetSymbolID: v.(string)}, 0.5
		}
	}
	return nil, 0
}

// resolveFirstInFile returns any SymbolNode from the given file (for source resolution fallback).
func (r *Resolver) resolveFirstInFile(ctx context.Context, tx neo4j.ManagedTransaction, spaceID, filePath string) (*RelationshipRecord, float64) {
	res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id, file_path: $file_path})
RETURN s.symbol_id AS symbol_id
LIMIT 1
`, map[string]any{
		"space_id":  spaceID,
		"file_path": filePath,
	})
	if err != nil {
		return nil, 0
	}
	if res.Next(ctx) {
		if v, ok := res.Record().Get("symbol_id"); ok && v != nil {
			return &RelationshipRecord{TargetSymbolID: v.(string)}, 1.0
		}
	}
	return nil, 0
}

// resolveImportTarget finds a SymbolNode in a file matching an import path.
func (r *Resolver) resolveImportTarget(ctx context.Context, tx neo4j.ManagedTransaction, spaceID, importPath string) (*RelationshipRecord, float64) {
	// Clean import path — remove quotes, leading ./
	clean := strings.Trim(importPath, "'\"")
	clean = strings.TrimPrefix(clean, "./")
	clean = strings.TrimPrefix(clean, "../")

	// Try to find a symbol in a file whose path ends with the import path
	res, err := tx.Run(ctx, `
MATCH (s:SymbolNode {space_id: $space_id})
WHERE s.file_path ENDS WITH $suffix OR s.file_path ENDS WITH $suffix_ts OR s.file_path ENDS WITH $suffix_index
RETURN s.symbol_id AS symbol_id
LIMIT 1
`, map[string]any{
		"space_id":     spaceID,
		"suffix":       clean,
		"suffix_ts":    clean + ".ts",
		"suffix_index": clean + "/index.ts",
	})
	if err != nil {
		return nil, 0
	}
	if res.Next(ctx) {
		if v, ok := res.Record().Get("symbol_id"); ok && v != nil {
			return &RelationshipRecord{TargetSymbolID: v.(string)}, 0.7
		}
	}
	return nil, 0
}
