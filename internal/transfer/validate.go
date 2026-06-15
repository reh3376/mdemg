package transfer

import (
	"context"
	"fmt"

	pb "mdemg/api/transferpb"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ValidateImport checks whether an export file is compatible with the target Neo4j.
func ValidateImport(ctx context.Context, driver neo4j.DriverWithContext, chunks []*pb.SpaceChunk) error {
	// Find metadata chunk
	var meta *pb.SpaceMetadata
	for _, c := range chunks {
		if c.ChunkType == pb.ChunkType_CHUNK_TYPE_METADATA && c.Metadata != nil {
			meta = c.Metadata
			break
		}
	}
	if meta == nil {
		return fmt.Errorf("export file has no metadata chunk")
	}

	// Check schema version
	localVersion, err := getSchemaVersion(ctx, driver)
	if err != nil {
		return fmt.Errorf("check local schema: %w", err)
	}

	if int(meta.SchemaVersion) > localVersion {
		return fmt.Errorf("export requires schema version %d but local is %d — run migrations first", meta.SchemaVersion, localVersion)
	}

	return nil
}

// GetSpaceInfo retrieves detailed information about a space for pre-transfer validation.
func GetSpaceInfo(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (*pb.SpaceInfoResponse, error) {
	exp := NewExporter(driver)

	counts, err := exp.countEntities(ctx, DefaultExportConfig(spaceID))
	if err != nil {
		return nil, fmt.Errorf("count entities: %w", err)
	}

	schemaVersion, err := exp.getSchemaVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("get schema version: %w", err)
	}

	embDims, _ := exp.detectEmbeddingDimensions(ctx, spaceID)

	// Get distinct edge types and role types
	edgeTypes, err := getDistinctEdgeTypes(ctx, driver, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get edge types: %w", err)
	}

	roleTypes, err := getDistinctRoleTypes(ctx, driver, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get role types: %w", err)
	}

	nodesByLayer, err := getNodesByLayer(ctx, driver, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get nodes by layer: %w", err)
	}

	// Get last updated time
	lastUpdated, err := getLastUpdated(ctx, driver, spaceID)
	if err != nil {
		lastUpdated = ""
	}

	// Get max layer
	maxLayer := int32(0)
	for layer := range nodesByLayer {
		l, err := parseInt32(layer)
		if err == nil && l > maxLayer {
			maxLayer = l
		}
	}

	return &pb.SpaceInfoResponse{
		Summary: &pb.SpaceSummary{
			SpaceId:          spaceID,
			NodeCount:        counts.Nodes,
			EdgeCount:        counts.Edges,
			ObservationCount: counts.Observations,
			SymbolCount:      counts.Symbols,
			MaxLayer:         maxLayer,
			LastUpdated:      lastUpdated,
			SchemaVersion:    int32(schemaVersion),
		},
		SchemaVersion:       int32(schemaVersion),
		EdgeTypes:           edgeTypes,
		RoleTypes:           roleTypes,
		EmbeddingDimensions: int64(embDims),
		NodesByLayer:        nodesByLayer,
	}, nil
}

// ListSpaces returns summary info for all spaces.
func ListSpaces(ctx context.Context, driver neo4j.DriverWithContext) ([]*pb.SpaceSummary, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Exclude null/empty space_id: nodes without a space_id are not a
		// "space" (orphaned/infra artifacts), and a nil spaceId crashed the
		// sid.(string) assertion below (DATAPRUNE space-hygiene catch).
		res, err := tx.Run(ctx, `
MATCH (n:MemoryNode)
WHERE n.space_id IS NOT NULL AND n.space_id <> ''
WITH n.space_id AS spaceId, count(n) AS nodeCount, max(n.layer) AS maxLayer
ORDER BY nodeCount DESC
RETURN spaceId, nodeCount, maxLayer`, nil)
		if err != nil {
			return nil, err
		}

		var spaces []*pb.SpaceSummary
		for res.Next(ctx) {
			rec := res.Record()
			sid, _ := rec.Get("spaceId")
			nc, _ := rec.Get("nodeCount")
			ml, _ := rec.Get("maxLayer")

			sidStr, ok := sid.(string)
			if !ok || sidStr == "" {
				continue // defensive: skip any non-string/empty space_id
			}
			s := &pb.SpaceSummary{
				SpaceId:   sidStr,
				NodeCount: nc.(int64),
			}
			if ml != nil {
				s.MaxLayer = int32(ml.(int64))
			}
			spaces = append(spaces, s)
		}
		return spaces, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*pb.SpaceSummary), nil
}

// =============================================================================
// Helpers
// =============================================================================

func getSchemaVersion(ctx context.Context, driver neo4j.DriverWithContext) (int, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (s:SchemaMeta {key: "schema_version"}) RETURN s.value AS version`, nil)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			return res.Record().Values[0], nil
		}
		return int64(0), nil
	})
	if err != nil {
		return 0, err
	}
	return int(result.(int64)), nil
}

func getDistinctEdgeTypes(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) ([]string, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (a:MemoryNode {space_id: $spaceId})-[r]->(b:MemoryNode {space_id: $spaceId})
RETURN DISTINCT type(r) AS relType ORDER BY relType`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var types []string
		for res.Next(ctx) {
			v, _ := res.Record().Get("relType")
			types = append(types, v.(string))
		}
		return types, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

func getDistinctRoleTypes(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) ([]string, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (n:MemoryNode {space_id: $spaceId})
RETURN DISTINCT n.role_type AS roleType ORDER BY roleType`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var types []string
		for res.Next(ctx) {
			v, _ := res.Record().Get("roleType")
			if v != nil {
				types = append(types, v.(string))
			}
		}
		return types, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

func getNodesByLayer(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (map[string]int64, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (n:MemoryNode {space_id: $spaceId})
RETURN n.layer AS layer, count(n) AS cnt ORDER BY layer`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		m := make(map[string]int64)
		for res.Next(ctx) {
			rec := res.Record()
			layer, _ := rec.Get("layer")
			cnt, _ := rec.Get("cnt")
			m[fmt.Sprintf("%d", layer.(int64))] = cnt.(int64)
		}
		return m, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]int64), nil
}

func getLastUpdated(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) (string, error) {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (n:MemoryNode {space_id: $spaceId})
RETURN max(n.updated_at) AS lastUpdated`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			v, _ := res.Record().Get("lastUpdated")
			return getTime(map[string]any{"t": v}, "t"), nil
		}
		return "", nil
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func parseInt32(s string) (int32, error) {
	var v int32
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
