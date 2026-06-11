package tsdb

import (
	"context"
	"fmt"

	"github.com/nrednav/cuid2"
)

// ContextCatalogVersionRow is one V0020 context_catalog_versions row — a
// historical snapshot of a ContextCatalog version (Phase 14.2 Note 05).
type ContextCatalogVersionRow struct {
	SpaceID           string
	Version           int
	TotalBits         int
	AllocationJSON    []byte // JSON array of {position, kind, ref, token}
	TopSymbols        []string
	TopPaths          []string
	TopTags           []string
	BitsRoleTypeLayer int
	BitsTag           int
	BitsPath          int
	BitsSymbol        int
}

// RecordContextCatalogVersion writes one V0020 row, synchronously
// (TSDB-CONSUME-001 — the V0020 writer-or-drop decision, decided: writer).
//
// The table existed since Phase 14.2 with ZERO writes ever: the migration
// header promised "written by Builder.BuildForSpace" but persistCatalog only
// wrote Neo4j. Sync single-row INSERT, not the buffered CopyFrom the header
// described — catalog builds happen ~once per space per week (the
// model_install_events precedent for one-shot, low-volume writes).
//
// Pass nil pool to no-op (TSDB disabled).
func RecordContextCatalogVersion(ctx context.Context, pool *Client, row ContextCatalogVersionRow) error {
	if pool == nil || pool.Pool() == nil {
		return nil
	}
	_, err := pool.Pool().Exec(ctx, `
		INSERT INTO context_catalog_versions (
			version_id, recorded_at, space_id, version, total_bits,
			allocation_json, top_symbols, top_paths, top_tags,
			bits_role_type_layer, bits_tag, bits_path, bits_symbol
		) VALUES ($1, NOW(), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		cuid2.Generate(), row.SpaceID, row.Version, row.TotalBits,
		row.AllocationJSON, row.TopSymbols, row.TopPaths, row.TopTags,
		row.BitsRoleTypeLayer, row.BitsTag, row.BitsPath, row.BitsSymbol)
	if err != nil {
		return fmt.Errorf("tsdb: record context catalog version: %w", err)
	}
	return nil
}
