package transfer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	pb "mdemg/api/transferpb"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ProgressFunc is called during export/import to report progress. Phase is a label
// (e.g. "nodes", "edges"); done and total are counts. If total is 0, it is unknown.
type ProgressFunc func(phase string, done, total int64)

// ExportConfig controls what gets exported.
type ExportConfig struct {
	SpaceID             string
	ChunkSize           int
	IncludeEmbeddings   bool
	IncludeObservations bool
	IncludeSymbols      bool
	IncludeLearnedEdges bool
	OnlyLearnedEdges    bool // when true, export only CO_ACTIVATED_WITH edges
	MetadataOnly        bool // when true, export only metadata + summary chunks (no node/edge/obs/symbol data)
	MinLayer            int
	MaxLayer            int // 0 = no max
	ProgressFunc        ProgressFunc
	// Phase 4: incremental sync — export only entities modified after this timestamp (ISO8601)
	SinceTimestamp string // from ExportRequest.since_timestamp or resolved from since_cursor
	SinceCursor    string // opaque cursor from prior export; resolved to SinceTimestamp when empty
	// Shareable knowledge filters
	ObsTypes        []string // Filter L0 nodes by obs_type (e.g., "learning", "decision")
	Tags            []string // Filter nodes by tag presence (any match)
	ExcludeVolatile bool     // Skip volatile/ungraduated observations (stability < 0.8)
	OnlyPinned      bool     // Only export pinned observations (+ always include L1+)
}

// DefaultExportConfig returns an ExportConfig with sensible defaults.
func DefaultExportConfig(spaceID string) ExportConfig {
	return ExportConfig{
		SpaceID:             spaceID,
		ChunkSize:           500,
		IncludeEmbeddings:   true,
		IncludeObservations: true,
		IncludeSymbols:      true,
		IncludeLearnedEdges: true,
		MinLayer:            0,
		MaxLayer:            0,
	}
}

// ExportProfile names for -profile flag.
const (
	ProfileFull      = "full"
	ProfileCodebase  = "codebase"
	ProfileCMS       = "cms"
	ProfileLearned   = "learned"
	ProfileMetadata  = "metadata"
	ProfileShareable = "shareable"
)

// ExportConfigForProfile returns an ExportConfig for a named profile.
// Use DefaultExportConfig and override flags when profile is "" or "full".
func ExportConfigForProfile(spaceID, profile string) (ExportConfig, error) {
	base := DefaultExportConfig(spaceID)
	switch profile {
	case "", ProfileFull:
		return base, nil
	case ProfileCodebase:
		base.IncludeObservations = false
		base.IncludeSymbols = true
		base.IncludeLearnedEdges = true
		return base, nil
	case ProfileCMS:
		base.IncludeObservations = true
		base.IncludeSymbols = false
		base.IncludeLearnedEdges = true
		return base, nil
	case ProfileLearned:
		base.IncludeObservations = false
		base.IncludeSymbols = false
		base.IncludeLearnedEdges = true
		base.OnlyLearnedEdges = true
		return base, nil
	case ProfileMetadata:
		base.MetadataOnly = true
		base.IncludeObservations = false
		base.IncludeSymbols = false
		base.IncludeLearnedEdges = false
		return base, nil
	case ProfileShareable:
		base.IncludeObservations = true
		base.IncludeSymbols = false
		base.IncludeLearnedEdges = true
		base.ExcludeVolatile = true
		base.ObsTypes = []string{"learning", "decision", "correction", "technical_note", "insight", "preference", "constraint"}
		return base, nil
	default:
		return ExportConfig{}, fmt.Errorf("unknown profile %q (use: full, codebase, cms, learned, metadata, shareable)", profile)
	}
}

// ExportFromRequest builds an ExportConfig from a protobuf ExportRequest.
func ExportFromRequest(req *pb.ExportRequest) ExportConfig {
	slog.Debug("ExportFromRequest", "space_id", req.SpaceId, "since_timestamp", req.SinceTimestamp, "since_cursor", req.SinceCursor)
	cfg := DefaultExportConfig(req.SpaceId)
	if req.ChunkSize > 0 {
		cfg.ChunkSize = int(req.ChunkSize)
	}
	if !req.IncludeEmbeddings {
		cfg.IncludeEmbeddings = false
	}
	if !req.IncludeObservations {
		cfg.IncludeObservations = false
	}
	if !req.IncludeSymbols {
		cfg.IncludeSymbols = false
	}
	if !req.IncludeLearnedEdges {
		cfg.IncludeLearnedEdges = false
	}
	cfg.MinLayer = int(req.MinLayer)
	cfg.MaxLayer = int(req.MaxLayer)
	cfg.SinceTimestamp = req.SinceTimestamp
	cfg.SinceCursor = req.SinceCursor
	if cfg.SinceTimestamp == "" && cfg.SinceCursor != "" {
		cfg.SinceTimestamp = cfg.SinceCursor // MVP: cursor is the timestamp for next delta
	}
	return cfg
}

// Exporter reads graph data from Neo4j and produces SpaceChunks.
type Exporter struct {
	driver neo4j.DriverWithContext
}

// NewExporter creates a new Exporter.
func NewExporter(driver neo4j.DriverWithContext) *Exporter {
	return &Exporter{driver: driver}
}

// ExportResult holds the complete export for file-based output.
type ExportResult struct {
	Chunks []*pb.SpaceChunk
}

// Export reads all data for a space and returns chunks.
// For gRPC streaming, use ExportStream instead.
func (e *Exporter) Export(ctx context.Context, cfg ExportConfig) (*ExportResult, error) {
	start := time.Now()
	result := &ExportResult{}

	// Get schema version
	schemaVersion, err := e.getSchemaVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("get schema version: %w", err)
	}

	// Count totals for metadata (filtered by since when incremental + shareable filters)
	counts, err := e.countEntities(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("count entities: %w", err)
	}

	// Detect embedding dimensions
	embDims, err := e.detectEmbeddingDimensions(ctx, cfg.SpaceID)
	if err != nil {
		slog.Warn("could not detect embedding dimensions", "error", err)
	}

	hostname, _ := os.Hostname()

	if cfg.ProgressFunc != nil {
		cfg.ProgressFunc("metadata", 1, 1)
	}
	// Chunk 0: metadata
	seq := int32(0)
	exportedAt := time.Now().UTC().Format(time.RFC3339)
	result.Chunks = append(result.Chunks, &pb.SpaceChunk{
		ChunkType:     pb.ChunkType_CHUNK_TYPE_METADATA,
		SpaceId:       cfg.SpaceID,
		SchemaVersion: int32(schemaVersion),
		Sequence:      seq,
		Metadata: &pb.SpaceMetadata{
			SpaceId:             cfg.SpaceID,
			SchemaVersion:       int32(schemaVersion),
			ExportedAt:          exportedAt,
			SourceHost:          hostname,
			TotalNodes:          counts.Nodes,
			TotalEdges:          counts.Edges,
			TotalObservations:   counts.Observations,
			TotalSymbols:        counts.Symbols,
			EmbeddingDimensions: int64(embDims),
			// Phase 35: Space lineage tracking
			Lineage: &pb.Lineage{
				OriginSpaceId: cfg.SpaceID,
				OriginHost:    hostname,
				CreatedAt:     exportedAt, // Use export time as creation (will be overwritten on subsequent exports)
				History: []*pb.LineageEvent{
					{
						EventType:  "export",
						Timestamp:  exportedAt,
						SourceHost: hostname,
						Notes:      "Initial export",
					},
				},
			},
		},
	})
	seq++

	if cfg.MetadataOnly {
		if cfg.ProgressFunc != nil {
			cfg.ProgressFunc("summary", 1, 1)
		}
		duration := time.Since(start)
		completedAt := time.Now().UTC().Format(time.RFC3339)
		sum := &pb.TransferSummary{
			NodesExported:        counts.Nodes,
			EdgesExported:        counts.Edges,
			ObservationsExported: counts.Observations,
			SymbolsExported:      counts.Symbols,
			DurationMs:           duration.Milliseconds(),
			CompletedAt:          completedAt,
		}
		if cfg.SinceTimestamp != "" {
			sum.NextCursor = completedAt
		}
		result.Chunks = append(result.Chunks, &pb.SpaceChunk{
			ChunkType:     pb.ChunkType_CHUNK_TYPE_SUMMARY,
			SpaceId:       cfg.SpaceID,
			SchemaVersion: int32(schemaVersion),
			Sequence:      seq,
			Summary:       sum,
		})
		return result, nil
	}

	// Export nodes in batches
	nodeChunks, err := e.exportNodes(ctx, cfg, schemaVersion, &seq)
	if err != nil {
		return nil, fmt.Errorf("export nodes: %w", err)
	}
	result.Chunks = append(result.Chunks, nodeChunks...)
	if cfg.ProgressFunc != nil {
		cfg.ProgressFunc("nodes", counts.Nodes, counts.Nodes)
	}

	// Export edges in batches
	edgeChunks, err := e.exportEdges(ctx, cfg, schemaVersion, &seq)
	if err != nil {
		return nil, fmt.Errorf("export edges: %w", err)
	}
	result.Chunks = append(result.Chunks, edgeChunks...)
	if cfg.ProgressFunc != nil {
		cfg.ProgressFunc("edges", counts.Edges, counts.Edges)
	}

	// Export observations (if requested)
	if cfg.IncludeObservations {
		obsChunks, err := e.exportObservations(ctx, cfg, schemaVersion, &seq)
		if err != nil {
			return nil, fmt.Errorf("export observations: %w", err)
		}
		result.Chunks = append(result.Chunks, obsChunks...)
		if cfg.ProgressFunc != nil {
			cfg.ProgressFunc("observations", counts.Observations, counts.Observations)
		}
	}

	// Export symbols (if requested)
	if cfg.IncludeSymbols {
		symChunks, err := e.exportSymbols(ctx, cfg, schemaVersion, &seq)
		if err != nil {
			return nil, fmt.Errorf("export symbols: %w", err)
		}
		result.Chunks = append(result.Chunks, symChunks...)
		if cfg.ProgressFunc != nil {
			cfg.ProgressFunc("symbols", counts.Symbols, counts.Symbols)
		}
	}

	if cfg.ProgressFunc != nil {
		cfg.ProgressFunc("summary", 1, 1)
	}
	// Final chunk: summary
	duration := time.Since(start)
	completedAt := time.Now().UTC().Format(time.RFC3339)
	sum := &pb.TransferSummary{
		NodesExported:        counts.Nodes,
		EdgesExported:        counts.Edges,
		ObservationsExported: counts.Observations,
		SymbolsExported:      counts.Symbols,
		DurationMs:           duration.Milliseconds(),
		CompletedAt:          completedAt,
	}
	if cfg.SinceTimestamp != "" {
		sum.NextCursor = completedAt // Client uses as since_cursor/since_timestamp for next delta
	}
	result.Chunks = append(result.Chunks, &pb.SpaceChunk{
		ChunkType:     pb.ChunkType_CHUNK_TYPE_SUMMARY,
		SpaceId:       cfg.SpaceID,
		SchemaVersion: int32(schemaVersion),
		Sequence:      seq,
		Summary:       sum,
	})

	return result, nil
}

type entityCounts struct {
	Nodes        int64
	Edges        int64
	Observations int64
	Symbols      int64
}

func (e *Exporter) getSchemaVersion(ctx context.Context) (int, error) {
	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
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

func (e *Exporter) countEntities(ctx context.Context, cfg ExportConfig) (entityCounts, error) {
	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	spaceID := cfg.SpaceID
	since := cfg.SinceTimestamp
	filters := BuildNodeFilterClauses(cfg)

	var counts entityCounts
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		params := map[string]any{"spaceId": spaceID, "since": since}
		mergeParams(params, filters.Params)
		nodeWhere := appendFilterWhere(
			`NOT coalesce(n.is_archived, false) AND ($since = '' OR n.updated_at > $since)`,
			filters.Clauses)
		cypher := fmt.Sprintf(`
MATCH (n:MemoryNode {space_id: $spaceId}) WHERE %s
WITH count(n) AS nodeCount
OPTIONAL MATCH (o:Observation {space_id: $spaceId}) WHERE ($since = '' OR o.created_at > $since OR o.timestamp > $since)
WITH nodeCount, count(o) AS obsCount
OPTIONAL MATCH (s:SymbolNode {space_id: $spaceId}) WHERE ($since = '' OR s.created_at > $since)
WITH nodeCount, obsCount, count(s) AS symCount
RETURN nodeCount, obsCount, symCount`, nodeWhere)
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			rec := res.Record()
			nc, _ := rec.Get("nodeCount")
			oc, _ := rec.Get("obsCount")
			sc, _ := rec.Get("symCount")
			return entityCounts{
				Nodes:        nc.(int64),
				Observations: oc.(int64),
				Symbols:      sc.(int64),
			}, nil
		}
		return entityCounts{}, nil
	})
	if err != nil {
		return counts, err
	}
	counts = result.(entityCounts)

	// Count edges separately (more efficient); apply endpoint filters for shareable export
	edgeParams := map[string]any{"spaceId": spaceID, "since": since}
	mergeParams(edgeParams, filters.Params)
	endpointWhere := buildEndpointFilterWhere(filters.Clauses)
	edgeCypher := fmt.Sprintf(`MATCH (a:MemoryNode {space_id: $spaceId})-[r]->(b:MemoryNode {space_id: $spaceId}) WHERE ($since = '' OR r.updated_at > $since)%s RETURN count(r) AS edgeCount`, endpointWhere)
	edgeResult, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, edgeCypher, edgeParams)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			v, _ := res.Record().Get("edgeCount")
			return v.(int64), nil
		}
		return int64(0), nil
	})
	if err != nil {
		return counts, err
	}
	counts.Edges = edgeResult.(int64)

	return counts, nil
}

func (e *Exporter) detectEmbeddingDimensions(ctx context.Context, spaceID string) (int, error) {
	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (n:MemoryNode {space_id: $spaceId}) WHERE n.embedding IS NOT NULL RETURN size(n.embedding) AS dims LIMIT 1`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			v, _ := res.Record().Get("dims")
			return v.(int64), nil
		}
		return int64(0), nil
	})
	if err != nil {
		return 0, err
	}
	return int(result.(int64)), nil
}

func (e *Exporter) exportNodes(ctx context.Context, cfg ExportConfig, schemaVersion int, seq *int32) ([]*pb.SpaceChunk, error) {
	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	var chunks []*pb.SpaceChunk
	skip := 0

	for {
		batch, err := e.fetchNodeBatch(ctx, sess, cfg, skip)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		chunks = append(chunks, &pb.SpaceChunk{
			ChunkType:     pb.ChunkType_CHUNK_TYPE_NODES,
			SpaceId:       cfg.SpaceID,
			SchemaVersion: int32(schemaVersion),
			Sequence:      *seq,
			Nodes:         &pb.NodeBatch{Nodes: batch},
		})
		*seq++
		skip += cfg.ChunkSize

		slog.Info("exported nodes", "count", skip, "batch", *seq-1)

		if len(batch) < cfg.ChunkSize {
			break
		}
	}

	return chunks, nil
}

func (e *Exporter) fetchNodeBatch(ctx context.Context, sess neo4j.SessionWithContext, cfg ExportConfig, skip int) ([]*pb.NodeData, error) {
	since := cfg.SinceTimestamp
	filters := BuildNodeFilterClauses(cfg)
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		where := appendFilterWhere(
			`NOT coalesce(n.is_archived, false) AND ($since = '' OR n.updated_at > $since)`,
			filters.Clauses)
		cypher := fmt.Sprintf(`MATCH (n:MemoryNode {space_id: $spaceId})
WHERE %s
RETURN n ORDER BY n.node_id SKIP $skip LIMIT $limit`, where)

		params := map[string]any{
			"spaceId": cfg.SpaceID,
			"since":   since,
			"skip":    skip,
			"limit":   cfg.ChunkSize,
		}
		mergeParams(params, filters.Params)

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var nodes []*pb.NodeData
		for res.Next(ctx) {
			rec := res.Record()
			nodeVal, _ := rec.Get("n")
			node := nodeVal.(neo4j.Node)
			props := node.Props

			nd := &pb.NodeData{
				NodeId:           getStr(props, "node_id"),
				SpaceId:          getStr(props, "space_id"),
				Path:             getStr(props, "path"),
				Name:             getStr(props, "name"),
				Layer:            int32(getInt(props, "layer")),
				RoleType:         getStr(props, "role_type"),
				Version:          int32(getInt(props, "version")),
				Description:      getStr(props, "description"),
				Summary:          getStr(props, "summary"),
				Confidence:       getFloat(props, "confidence"),
				Sensitivity:      getStr(props, "sensitivity"),
				Tags:             getStrSlice(props, "tags"),
				UserId:           getStr(props, "user_id"),
				Visibility:       getStr(props, "visibility"),
				Volatile:         getBool(props, "volatile"),
				IsArchived:       getBool(props, "is_archived"),
				Content:          getStr(props, "content"),
				ObsType:          getStr(props, "obs_type"),
				SurpriseScore:    getFloat(props, "surprise_score"),
				SessionId:        getStr(props, "session_id"),
				AgentId:          getStr(props, "agent_id"),
				MemberCount:      int32(getInt(props, "member_count")),
				DominantObsType:  getStr(props, "dominant_obs_type"),
				AvgSurpriseScore: getFloat(props, "avg_surprise_score"),
				Keywords:         getStrSlice(props, "keywords"),
				SessionCount:     int32(getInt(props, "session_count")),
				AggregationCount: int32(getInt(props, "aggregation_count")),
				StabilityScore:   getFloat(props, "stability_score"),
			}

			if t := getTime(props, "created_at"); t != "" {
				nd.CreatedAt = t
			}
			if t := getTime(props, "updated_at"); t != "" {
				nd.UpdatedAt = t
			}
			if t := getTime(props, "last_forward_pass"); t != "" {
				nd.LastForwardPass = t
			}
			if t := getTime(props, "last_backward_pass"); t != "" {
				nd.LastBackwardPass = t
			}

			if cfg.IncludeEmbeddings {
				nd.Embedding = getFloat32Slice(props, "embedding")
				nd.MessagePassEmbedding = getFloat32Slice(props, "message_pass_embedding")
			}

			nodes = append(nodes, nd)
		}
		return nodes, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*pb.NodeData), nil
}

func (e *Exporter) exportEdges(ctx context.Context, cfg ExportConfig, schemaVersion int, seq *int32) ([]*pb.SpaceChunk, error) {
	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	var chunks []*pb.SpaceChunk
	skip := 0

	for {
		batch, err := e.fetchEdgeBatch(ctx, sess, cfg, skip)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		chunks = append(chunks, &pb.SpaceChunk{
			ChunkType:     pb.ChunkType_CHUNK_TYPE_EDGES,
			SpaceId:       cfg.SpaceID,
			SchemaVersion: int32(schemaVersion),
			Sequence:      *seq,
			Edges:         &pb.EdgeBatch{Edges: batch},
		})
		*seq++
		skip += cfg.ChunkSize

		if len(batch) < cfg.ChunkSize {
			break
		}
	}

	return chunks, nil
}

func (e *Exporter) fetchEdgeBatch(ctx context.Context, sess neo4j.SessionWithContext, cfg ExportConfig, skip int) ([]*pb.EdgeData, error) {
	since := cfg.SinceTimestamp
	filters := BuildNodeFilterClauses(cfg)
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Build endpoint filter (rewrite "n." to "a." and "b." for both endpoints)
		endpointWhere := buildEndpointFilterWhere(filters.Clauses)

		var relMatch string
		if cfg.OnlyLearnedEdges {
			relMatch = `MATCH (a:MemoryNode {space_id: $spaceId})-[r:CO_ACTIVATED_WITH]->(b:MemoryNode {space_id: $spaceId})`
		} else {
			relMatch = `MATCH (a:MemoryNode {space_id: $spaceId})-[r]->(b:MemoryNode {space_id: $spaceId})`
		}
		cypher := fmt.Sprintf(`%s
WHERE ($since = '' OR r.updated_at > $since)%s
RETURN a.node_id AS fromId, b.node_id AS toId, type(r) AS relType, properties(r) AS props
ORDER BY a.node_id, b.node_id SKIP $skip LIMIT $limit`, relMatch, endpointWhere)

		params := map[string]any{
			"spaceId": cfg.SpaceID,
			"since":   since,
			"skip":    skip,
			"limit":   cfg.ChunkSize,
		}
		mergeParams(params, filters.Params)

		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}

		var edges []*pb.EdgeData
		for res.Next(ctx) {
			rec := res.Record()
			fromID, _ := rec.Get("fromId")
			toID, _ := rec.Get("toId")
			relType, _ := rec.Get("relType")
			propsVal, _ := rec.Get("props")

			props := make(map[string]any)
			if p, ok := propsVal.(map[string]any); ok {
				props = p
			}
			// UATS-GAP-001 live-caught P0: these were unchecked type
			// assertions — one edge whose endpoint node lacks node_id (nil
			// fromId/toId) PANICKED the whole server mid-backup (a UATS
			// backup_trigger run took the process down). Skip-with-warning;
			// an unexportable edge must never be fatal.
			fromIDStr, okFrom := fromID.(string)
			toIDStr, okTo := toID.(string)
			relTypeStr, okRel := relType.(string)
			if !okFrom || !okTo || !okRel {
				slog.Warn("transfer: skipping edge with nil identity fields",
					"space_id", cfg.SpaceID, "from_ok", okFrom, "to_ok", okTo, "rel_ok", okRel)
				continue
			}
			if !cfg.OnlyLearnedEdges && !cfg.IncludeLearnedEdges && relTypeStr == "CO_ACTIVATED_WITH" {
				continue
			}

			ed := &pb.EdgeData{
				FromNodeId:    fromIDStr,
				ToNodeId:      toIDStr,
				RelType:       relTypeStr,
				SpaceId:       cfg.SpaceID,
				EdgeId:        getStr(props, "edge_id"),
				Weight:        getFloat(props, "weight"),
				InitialWeight: getFloat(props, "initial_weight"),
				EvidenceCount: int32(getInt(props, "evidence_count")),
				CreatedAt:     getTime(props, "created_at"),
				UpdatedAt:     getTime(props, "updated_at"),
				LastActivated: getTime(props, "last_activated"),
			}
			edges = append(edges, ed)
		}
		return edges, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*pb.EdgeData), nil
}

func (e *Exporter) exportObservations(ctx context.Context, cfg ExportConfig, schemaVersion int, seq *int32) ([]*pb.SpaceChunk, error) {
	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	var chunks []*pb.SpaceChunk
	skip := 0

	for {
		batch, err := e.fetchObservationBatch(ctx, sess, cfg, skip)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		chunks = append(chunks, &pb.SpaceChunk{
			ChunkType:     pb.ChunkType_CHUNK_TYPE_OBSERVATIONS,
			SpaceId:       cfg.SpaceID,
			SchemaVersion: int32(schemaVersion),
			Sequence:      *seq,
			Observations:  &pb.ObservationBatch{Observations: batch},
		})
		*seq++
		skip += cfg.ChunkSize

		if len(batch) < cfg.ChunkSize {
			break
		}
	}

	return chunks, nil
}

func (e *Exporter) fetchObservationBatch(ctx context.Context, sess neo4j.SessionWithContext, cfg ExportConfig, skip int) ([]*pb.ObservationData, error) {
	since := cfg.SinceTimestamp
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (o:Observation {space_id: $spaceId})
WHERE ($since = '' OR o.created_at > $since OR o.timestamp > $since)
OPTIONAL MATCH (n:MemoryNode)-[:HAS_OBSERVATION]->(o)
RETURN o, n.node_id AS parentNodeId
ORDER BY o.obs_id SKIP $skip LIMIT $limit`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": cfg.SpaceID,
			"since":   since,
			"skip":    skip,
			"limit":   cfg.ChunkSize,
		})
		if err != nil {
			return nil, err
		}

		var obs []*pb.ObservationData
		for res.Next(ctx) {
			rec := res.Record()
			obsVal, _ := rec.Get("o")
			parentVal, _ := rec.Get("parentNodeId")
			node := obsVal.(neo4j.Node)
			props := node.Props

			od := &pb.ObservationData{
				ObsId:     getStr(props, "obs_id"),
				SpaceId:   getStr(props, "space_id"),
				Content:   getStr(props, "content"),
				Source:    getStr(props, "source"),
				Timestamp: getTime(props, "timestamp"),
				CreatedAt: getTime(props, "created_at"),
			}

			if pv, ok := parentVal.(string); ok {
				od.ParentNodeId = pv
			}

			if cfg.IncludeEmbeddings {
				od.Embedding = getFloat32Slice(props, "embedding")
			}

			obs = append(obs, od)
		}
		return obs, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*pb.ObservationData), nil
}

func (e *Exporter) exportSymbols(ctx context.Context, cfg ExportConfig, schemaVersion int, seq *int32) ([]*pb.SpaceChunk, error) {
	sess := e.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	var chunks []*pb.SpaceChunk
	skip := 0

	for {
		batch, err := e.fetchSymbolBatch(ctx, sess, cfg, skip)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}

		chunks = append(chunks, &pb.SpaceChunk{
			ChunkType:     pb.ChunkType_CHUNK_TYPE_SYMBOLS,
			SpaceId:       cfg.SpaceID,
			SchemaVersion: int32(schemaVersion),
			Sequence:      *seq,
			Symbols:       &pb.SymbolBatch{Symbols: batch},
		})
		*seq++
		skip += cfg.ChunkSize

		if len(batch) < cfg.ChunkSize {
			break
		}
	}

	return chunks, nil
}

func (e *Exporter) fetchSymbolBatch(ctx context.Context, sess neo4j.SessionWithContext, cfg ExportConfig, skip int) ([]*pb.SymbolData, error) {
	since := cfg.SinceTimestamp
	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := `MATCH (s:SymbolNode {space_id: $spaceId})
WHERE ($since = '' OR s.created_at > $since)
OPTIONAL MATCH (n:MemoryNode)-[:HAS_SYMBOL]->(s)
RETURN s, n.node_id AS parentNodeId
ORDER BY s.symbol_id SKIP $skip LIMIT $limit`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": cfg.SpaceID,
			"since":   since,
			"skip":    skip,
			"limit":   cfg.ChunkSize,
		})
		if err != nil {
			return nil, err
		}

		var syms []*pb.SymbolData
		for res.Next(ctx) {
			rec := res.Record()
			symVal, _ := rec.Get("s")
			parentVal, _ := rec.Get("parentNodeId")
			node := symVal.(neo4j.Node)
			props := node.Props

			sd := &pb.SymbolData{
				SymbolId:   getStr(props, "symbol_id"),
				SpaceId:    getStr(props, "space_id"),
				Name:       getStr(props, "name"),
				SymbolType: getStr(props, "symbol_type"),
				FilePath:   getStr(props, "file_path"),
				Line:       int32(getInt(props, "line")),
				LineEnd:    int32(getInt(props, "line_end")),
				Exported:   getBool(props, "exported"),
				Parent:     getStr(props, "parent"),
				Signature:  getStr(props, "signature"),
				Value:      getStr(props, "value"),
				DocComment: getStr(props, "doc_comment"),
				Language:   getStr(props, "language"),
			}

			if pv, ok := parentVal.(string); ok {
				sd.ParentNodeId = pv
			}

			if cfg.IncludeEmbeddings {
				sd.Embedding = getFloat32Slice(props, "embedding")
			}

			syms = append(syms, sd)
		}
		return syms, res.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]*pb.SymbolData), nil
}

// =============================================================================
// Neo4j property helpers
// =============================================================================

func getStr(props map[string]any, key string) string {
	if v, ok := props[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(props map[string]any, key string) int64 {
	if v, ok := props[key]; ok && v != nil {
		switch n := v.(type) {
		case int64:
			return n
		case float64:
			return int64(n)
		}
	}
	return 0
}

func getFloat(props map[string]any, key string) float64 {
	if v, ok := props[key]; ok && v != nil {
		switch n := v.(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		}
	}
	return 0
}

func getBool(props map[string]any, key string) bool {
	if v, ok := props[key]; ok && v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getStrSlice(props map[string]any, key string) []string {
	if v, ok := props[key]; ok && v != nil {
		if arr, ok := v.([]any); ok {
			result := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
			return result
		}
	}
	return nil
}

func getFloat32Slice(props map[string]any, key string) []float32 {
	if v, ok := props[key]; ok && v != nil {
		if arr, ok := v.([]any); ok {
			result := make([]float32, 0, len(arr))
			for _, item := range arr {
				switch n := item.(type) {
				case float64:
					result = append(result, float32(n))
				case float32:
					result = append(result, n)
				}
			}
			return result
		}
	}
	return nil
}

func getTime(props map[string]any, key string) string {
	if v, ok := props[key]; ok && v != nil {
		switch t := v.(type) {
		case time.Time:
			return t.UTC().Format(time.RFC3339)
		case string:
			return t
		}
	}
	return ""
}

// nodeFilterResult holds generated WHERE clause fragments and params for shareable filters.
type nodeFilterResult struct {
	Clauses []string
	Params  map[string]any
}

// BuildNodeFilterClauses constructs WHERE clause fragments from ExportConfig filter fields.
// The returned clauses should be ANDed into existing WHERE conditions.
// Node alias must be "n". L1+ nodes always pass ObsTypes/OnlyPinned filters.
func BuildNodeFilterClauses(cfg ExportConfig) nodeFilterResult {
	r := nodeFilterResult{Params: make(map[string]any)}

	if len(cfg.ObsTypes) > 0 {
		r.Clauses = append(r.Clauses,
			`(n.layer >= 1 OR (n.role_type = 'conversation_observation' AND n.obs_type IN $filterObsTypes))`)
		r.Params["filterObsTypes"] = cfg.ObsTypes
	}

	if len(cfg.Tags) > 0 {
		r.Clauses = append(r.Clauses,
			`ANY(t IN coalesce(n.tags, []) WHERE t IN $filterTags)`)
		r.Params["filterTags"] = cfg.Tags
	}

	if cfg.ExcludeVolatile {
		r.Clauses = append(r.Clauses,
			`(NOT coalesce(n.volatile, false) OR coalesce(n.stability_score, 0) >= 0.8)`)
	}

	if cfg.OnlyPinned {
		r.Clauses = append(r.Clauses,
			`(n.layer >= 1 OR coalesce(n.pinned, false))`)
	}

	if cfg.MinLayer > 0 {
		r.Clauses = append(r.Clauses, `n.layer >= $filterMinLayer`)
		r.Params["filterMinLayer"] = cfg.MinLayer
	}

	if cfg.MaxLayer > 0 {
		r.Clauses = append(r.Clauses, `n.layer <= $filterMaxLayer`)
		r.Params["filterMaxLayer"] = cfg.MaxLayer
	}

	return r
}

// appendFilterWhere appends filter clauses to a base WHERE string using AND.
func appendFilterWhere(base string, clauses []string) string {
	if len(clauses) == 0 {
		return base
	}
	return base + " AND " + strings.Join(clauses, " AND ")
}

// mergeParams copies filter params into a target map.
func mergeParams(dst map[string]any, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

// buildEndpointFilterWhere rewrites node filter clauses for edge queries,
// replacing "n." with "a." and "b." for both endpoints.
func buildEndpointFilterWhere(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	parts := make([]string, 0, len(clauses)*2)
	for _, clause := range clauses {
		parts = append(parts, strings.ReplaceAll(clause, "n.", "a."))
		parts = append(parts, strings.ReplaceAll(clause, "n.", "b."))
	}
	return " AND " + strings.Join(parts, " AND ")
}
