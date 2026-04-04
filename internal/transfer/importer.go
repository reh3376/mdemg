package transfer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	pb "mdemg/api/transferpb"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Importer writes SpaceChunks into Neo4j.
type Importer struct {
	driver       neo4j.DriverWithContext
	conflictMode pb.ConflictMode
	ProgressFunc ProgressFunc // optional progress callback
}

// NewImporter creates a new Importer with the specified conflict handling mode.
func NewImporter(driver neo4j.DriverWithContext, mode pb.ConflictMode) *Importer {
	return &Importer{driver: driver, conflictMode: mode}
}

// ImportResult tracks statistics from the import operation.
type ImportResult struct {
	NodesCreated        int32
	NodesSkipped        int32
	NodesOverwritten    int32
	EdgesCreated        int32
	EdgesSkipped        int32
	EdgesMerged         int32 // Phase 35: CRDT merged edges
	ObservationsCreated int32
	SymbolsCreated      int32
	Warnings            []string
	Duration            time.Duration
}

// ToProto converts ImportResult to a protobuf ImportResponse.
func (r *ImportResult) ToProto() *pb.ImportResponse {
	return &pb.ImportResponse{
		Success: true,
		Stats: &pb.ImportStats{
			NodesCreated:        r.NodesCreated,
			NodesSkipped:        r.NodesSkipped,
			NodesOverwritten:    r.NodesOverwritten,
			EdgesCreated:        r.EdgesCreated,
			EdgesSkipped:        r.EdgesSkipped,
			EdgesMerged:         r.EdgesMerged,
			ObservationsCreated: r.ObservationsCreated,
			SymbolsCreated:      r.SymbolsCreated,
			DurationMs:          r.Duration.Milliseconds(),
		},
		Warnings: r.Warnings,
	}
}

// Import processes a sequence of SpaceChunks and writes them to Neo4j.
func (imp *Importer) Import(ctx context.Context, chunks []*pb.SpaceChunk) (*ImportResult, error) {
	start := time.Now()
	result := &ImportResult{}

	// Validate schema version from metadata chunk
	for _, chunk := range chunks {
		if chunk.ChunkType == pb.ChunkType_CHUNK_TYPE_METADATA {
			if err := imp.validateSchema(ctx, chunk); err != nil {
				return nil, err
			}
			break
		}
	}

	// Ensure TapRoot exists for the space
	if len(chunks) > 0 {
		spaceID := chunks[0].SpaceId
		prunable := strings.HasPrefix(spaceID, "test-") || strings.HasPrefix(spaceID, "uats-")
		if err := imp.ensureTapRoot(ctx, spaceID, prunable); err != nil {
			return nil, fmt.Errorf("ensure taproot: %w", err)
		}
	}

	// Process chunks in order
	for i, chunk := range chunks {
		if imp.ProgressFunc != nil {
			imp.ProgressFunc("chunks", int64(i+1), int64(len(chunks)))
		}
		switch chunk.ChunkType {
		case pb.ChunkType_CHUNK_TYPE_NODES:
			if chunk.Nodes != nil {
				stats, err := imp.importNodes(ctx, chunk.Nodes.Nodes)
				if err != nil {
					return nil, fmt.Errorf("import nodes (chunk %d): %w", chunk.Sequence, err)
				}
				result.NodesCreated += stats.created
				result.NodesSkipped += stats.skipped
				result.NodesOverwritten += stats.overwritten
				slog.Info("imported node chunk", "sequence", chunk.Sequence, "created", stats.created, "skipped", stats.skipped)
			}

		case pb.ChunkType_CHUNK_TYPE_EDGES:
			if chunk.Edges != nil {
				stats, err := imp.importEdges(ctx, chunk.Edges.Edges)
				if err != nil {
					return nil, fmt.Errorf("import edges (chunk %d): %w", chunk.Sequence, err)
				}
				result.EdgesCreated += stats.created
				result.EdgesSkipped += stats.skipped
				result.EdgesMerged += stats.merged
				slog.Info("imported edge chunk", "sequence", chunk.Sequence, "created", stats.created, "skipped", stats.skipped, "merged", stats.merged)
			}

		case pb.ChunkType_CHUNK_TYPE_OBSERVATIONS:
			if chunk.Observations != nil {
				count, err := imp.importObservations(ctx, chunk.Observations.Observations)
				if err != nil {
					return nil, fmt.Errorf("import observations (chunk %d): %w", chunk.Sequence, err)
				}
				result.ObservationsCreated += int32(count)
			}

		case pb.ChunkType_CHUNK_TYPE_SYMBOLS:
			if chunk.Symbols != nil {
				count, err := imp.importSymbols(ctx, chunk.Symbols.Symbols)
				if err != nil {
					return nil, fmt.Errorf("import symbols (chunk %d): %w", chunk.Sequence, err)
				}
				result.SymbolsCreated += int32(count)
			}

		case pb.ChunkType_CHUNK_TYPE_METADATA, pb.ChunkType_CHUNK_TYPE_SUMMARY:
			// Informational only
			continue
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (imp *Importer) validateSchema(ctx context.Context, chunk *pb.SpaceChunk) error {
	if chunk.Metadata == nil {
		return nil
	}

	localVersion, err := imp.getLocalSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("check local schema: %w", err)
	}

	exportVersion := int(chunk.Metadata.SchemaVersion)
	if exportVersion > localVersion {
		return fmt.Errorf("export schema version %d is newer than local schema version %d — apply migrations first", exportVersion, localVersion)
	}

	return nil
}

func (imp *Importer) getLocalSchemaVersion(ctx context.Context) (int, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
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

func (imp *Importer) ensureTapRoot(ctx context.Context, spaceID string, prunable bool) error {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `MERGE (t:TapRoot {space_id: $spaceId}) ON CREATE SET t.created_at = datetime(), t.prunable = $prunable`,
			map[string]any{"spaceId": spaceID, "prunable": prunable})
		return nil, err
	})
	return err
}

type batchStats struct {
	created     int32
	skipped     int32
	overwritten int32
	merged      int32 // Phase 35: CRDT merged edges
}

func (imp *Importer) importNodes(ctx context.Context, nodes []*pb.NodeData) (batchStats, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	var stats batchStats

	for _, nd := range nodes {
		result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			// Check if node exists
			existRes, err := tx.Run(ctx,
				`MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId}) RETURN count(n) AS cnt`,
				map[string]any{"spaceId": nd.SpaceId, "nodeId": nd.NodeId})
			if err != nil {
				return nil, err
			}
			exists := false
			if existRes.Next(ctx) {
				cnt, _ := existRes.Record().Get("cnt")
				exists = cnt.(int64) > 0
			}

			if exists {
				switch imp.conflictMode {
				case pb.ConflictMode_CONFLICT_SKIP:
					return "skipped", nil
				case pb.ConflictMode_CONFLICT_ERROR:
					return nil, fmt.Errorf("node %s already exists (conflict_mode=error)", nd.NodeId)
				case pb.ConflictMode_CONFLICT_OVERWRITE:
					// Fall through to create/merge
				}
			}

			// Build property map
			props := map[string]any{
				"space_id":  nd.SpaceId,
				"node_id":   nd.NodeId,
				"path":      nd.Path,
				"name":      nd.Name,
				"layer":     int64(nd.Layer),
				"role_type": nd.RoleType,
			}

			if nd.Version > 0 {
				props["version"] = int64(nd.Version)
			}
			if nd.Description != "" {
				props["description"] = nd.Description
			}
			if nd.Summary != "" {
				props["summary"] = nd.Summary
			}
			if nd.Confidence > 0 {
				props["confidence"] = nd.Confidence
			}
			if nd.Sensitivity != "" {
				props["sensitivity"] = nd.Sensitivity
			}
			if len(nd.Tags) > 0 {
				props["tags"] = nd.Tags
			}
			if nd.UserId != "" {
				props["user_id"] = nd.UserId
			}
			if nd.Visibility != "" {
				props["visibility"] = nd.Visibility
			}
			if nd.Volatile {
				props["volatile"] = true
			}
			if nd.Content != "" {
				props["content"] = nd.Content
			}
			if nd.ObsType != "" {
				props["obs_type"] = nd.ObsType
			}
			if nd.SurpriseScore > 0 {
				props["surprise_score"] = nd.SurpriseScore
			}
			if nd.SessionId != "" {
				props["session_id"] = nd.SessionId
			}
			if nd.AgentId != "" {
				props["agent_id"] = nd.AgentId
			}
			if nd.MemberCount > 0 {
				props["member_count"] = int64(nd.MemberCount)
			}
			if nd.DominantObsType != "" {
				props["dominant_obs_type"] = nd.DominantObsType
			}
			if nd.AvgSurpriseScore > 0 {
				props["avg_surprise_score"] = nd.AvgSurpriseScore
			}
			if len(nd.Keywords) > 0 {
				props["keywords"] = nd.Keywords
			}
			if nd.SessionCount > 0 {
				props["session_count"] = int64(nd.SessionCount)
			}
			if nd.AggregationCount > 0 {
				props["aggregation_count"] = int64(nd.AggregationCount)
			}
			if nd.StabilityScore > 0 {
				props["stability_score"] = nd.StabilityScore
			}
			if nd.CreatedAt != "" {
				props["created_at"] = nd.CreatedAt
			}
			if nd.UpdatedAt != "" {
				props["updated_at"] = nd.UpdatedAt
			}

			// Handle embeddings (convert float32 → float64 for Neo4j)
			if len(nd.Embedding) > 0 {
				emb := make([]float64, len(nd.Embedding))
				for i, v := range nd.Embedding {
					emb[i] = float64(v)
				}
				props["embedding"] = emb
			}
			if len(nd.MessagePassEmbedding) > 0 {
				emb := make([]float64, len(nd.MessagePassEmbedding))
				for i, v := range nd.MessagePassEmbedding {
					emb[i] = float64(v)
				}
				props["message_pass_embedding"] = emb
			}

			// Build dynamic SET clause from props
			setClauses := make([]string, 0, len(props))
			for k := range props {
				setClauses = append(setClauses, fmt.Sprintf("n.%s = $props.%s", k, k))
			}

			cypher := fmt.Sprintf(`MERGE (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
ON CREATE SET %s
ON MATCH SET %s`, strings.Join(setClauses, ", "), strings.Join(setClauses, ", "))

			_, err = tx.Run(ctx, cypher, map[string]any{
				"spaceId": nd.SpaceId,
				"nodeId":  nd.NodeId,
				"props":   props,
			})
			if err != nil {
				return nil, err
			}

			if exists && imp.conflictMode == pb.ConflictMode_CONFLICT_OVERWRITE {
				return "overwritten", nil
			}
			return "created", nil
		})

		if err != nil {
			return stats, err
		}

		switch result.(string) {
		case "created":
			stats.created++
		case "skipped":
			stats.skipped++
		case "overwritten":
			stats.overwritten++
		}
	}

	return stats, nil
}

func (imp *Importer) importEdges(ctx context.Context, edges []*pb.EdgeData) (batchStats, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	var stats batchStats

	for _, ed := range edges {
		result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			params := map[string]any{
				"spaceId":       ed.SpaceId,
				"fromId":        ed.FromNodeId,
				"toId":          ed.ToNodeId,
				"weight":        ed.Weight,
				"evidenceCount": int64(ed.EvidenceCount),
				"dimTemporal":   ed.DimTemporal,
				"dimSemantic":   ed.DimSemantic,
				"dimCausal":     ed.DimCausal,
			}

			var cypher string

			// Phase 35: CRDT mode uses special merge logic for CO_ACTIVATED_WITH edges
			if imp.conflictMode == pb.ConflictMode_CONFLICT_CRDT && ed.RelType == "CO_ACTIVATED_WITH" {
				// CRDT merge: sum evidence_count, max weight, max dimensional weights
				cypher = `
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $fromId})
MATCH (b:MemoryNode {space_id: $spaceId, node_id: $toId})
MERGE (a)-[r:CO_ACTIVATED_WITH]->(b)
ON CREATE SET
    r.weight = $weight,
    r.evidence_count = $evidenceCount,
    r.dim_temporal = $dimTemporal,
    r.dim_semantic = $dimSemantic,
    r.dim_causal = $dimCausal,
    r.created_at = datetime(),
    r.space_id = $spaceId
ON MATCH SET
    r.weight = CASE WHEN r.weight < $weight THEN $weight ELSE r.weight END,
    r.evidence_count = coalesce(r.evidence_count, 0) + $evidenceCount,
    r.dim_temporal = CASE WHEN coalesce(r.dim_temporal, 0) < $dimTemporal THEN $dimTemporal ELSE coalesce(r.dim_temporal, 0) END,
    r.dim_semantic = CASE WHEN coalesce(r.dim_semantic, 0) < $dimSemantic THEN $dimSemantic ELSE coalesce(r.dim_semantic, 0) END,
    r.dim_causal = CASE WHEN coalesce(r.dim_causal, 0) < $dimCausal THEN $dimCausal ELSE coalesce(r.dim_causal, 0) END,
    r.updated_at = datetime(),
    r.last_activated = datetime()
RETURN CASE WHEN r.created_at = r.updated_at THEN 'created' ELSE 'merged' END AS action`
			} else {
				// Standard merge: max weight only
				cypher = fmt.Sprintf(`
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $fromId})
MATCH (b:MemoryNode {space_id: $spaceId, node_id: $toId})
MERGE (a)-[r:%s]->(b)
ON CREATE SET r.weight = $weight, r.created_at = datetime(), r.space_id = $spaceId
ON MATCH SET r.weight = CASE WHEN r.weight < $weight THEN $weight ELSE r.weight END
RETURN 'created' AS action`, ed.RelType)
			}

			res, err := tx.Run(ctx, cypher, params)
			if err != nil {
				return nil, err
			}

			// Determine if this was a create or merge
			action := "created"
			if res.Next(ctx) {
				if a, ok := res.Record().Get("action"); ok {
					action = a.(string)
				}
			}
			return action, nil
		})

		if err != nil {
			// Log warning but continue — missing nodes are expected if partial export
			slog.Warn("edge import failed", "from", ed.FromNodeId, "to", ed.ToNodeId, "rel_type", ed.RelType, "error", err)
			stats.skipped++
			continue
		}

		switch result.(string) {
		case "created":
			stats.created++
		case "merged":
			stats.merged++
		default:
			stats.created++
		}
	}

	return stats, nil
}

func (imp *Importer) importObservations(ctx context.Context, obs []*pb.ObservationData) (int, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	count := 0
	for _, od := range obs {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			props := map[string]any{
				"space_id":  od.SpaceId,
				"obs_id":    od.ObsId,
				"content":   od.Content,
				"source":    od.Source,
				"timestamp": od.Timestamp,
			}
			if od.CreatedAt != "" {
				props["created_at"] = od.CreatedAt
			}
			if len(od.Embedding) > 0 {
				emb := make([]float64, len(od.Embedding))
				for i, v := range od.Embedding {
					emb[i] = float64(v)
				}
				props["embedding"] = emb
			}

			_, err := tx.Run(ctx, `MERGE (o:Observation {space_id: $spaceId, obs_id: $obsId})
ON CREATE SET o += $props`, map[string]any{
				"spaceId": od.SpaceId,
				"obsId":   od.ObsId,
				"props":   props,
			})
			if err != nil {
				return nil, err
			}

			// Link to parent node if specified
			if od.ParentNodeId != "" {
				_, err = tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
MATCH (o:Observation {space_id: $spaceId, obs_id: $obsId})
MERGE (n)-[:HAS_OBSERVATION]->(o)`, map[string]any{
					"spaceId": od.SpaceId,
					"nodeId":  od.ParentNodeId,
					"obsId":   od.ObsId,
				})
				if err != nil {
					slog.Warn("could not link observation to node", "obs_id", od.ObsId, "parent_node_id", od.ParentNodeId, "error", err)
				}
			}

			return nil, nil
		})
		if err != nil {
			slog.Warn("observation import failed", "obs_id", od.ObsId, "error", err)
			continue
		}
		count++
	}

	return count, nil
}

func (imp *Importer) importSymbols(ctx context.Context, syms []*pb.SymbolData) (int, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	count := 0
	for _, sd := range syms {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			props := map[string]any{
				"space_id":    sd.SpaceId,
				"symbol_id":   sd.SymbolId,
				"name":        sd.Name,
				"symbol_type": sd.SymbolType,
				"file_path":   sd.FilePath,
				"line":        int64(sd.Line),
				"exported":    sd.Exported,
			}
			if sd.LineEnd > 0 {
				props["line_end"] = int64(sd.LineEnd)
			}
			if sd.Parent != "" {
				props["parent"] = sd.Parent
			}
			if sd.Signature != "" {
				props["signature"] = sd.Signature
			}
			if sd.Value != "" {
				props["value"] = sd.Value
			}
			if sd.DocComment != "" {
				props["doc_comment"] = sd.DocComment
			}
			if sd.Language != "" {
				props["language"] = sd.Language
			}
			if len(sd.Embedding) > 0 {
				emb := make([]float64, len(sd.Embedding))
				for i, v := range sd.Embedding {
					emb[i] = float64(v)
				}
				props["embedding"] = emb
			}

			// MERGE on natural key to match store.go pattern and prevent duplicates
			_, err := tx.Run(ctx, `MERGE (s:SymbolNode {space_id: $spaceId, name: $name, file_path: $filePath, symbol_type: $symbolType})
ON CREATE SET s.symbol_id = $symbolId, s += $props
ON MATCH SET s.symbol_id = $symbolId, s += $props`, map[string]any{
				"spaceId":    sd.SpaceId,
				"name":       sd.Name,
				"filePath":   sd.FilePath,
				"symbolType": sd.SymbolType,
				"symbolId":   sd.SymbolId,
				"props":      props,
			})
			if err != nil {
				return nil, err
			}

			// Link to parent memory node (match by natural key for consistency)
			if sd.ParentNodeId != "" {
				_, err = tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
MATCH (s:SymbolNode {space_id: $spaceId, name: $name, file_path: $filePath, symbol_type: $symbolType})
MERGE (n)-[:HAS_SYMBOL]->(s)`, map[string]any{
					"spaceId":    sd.SpaceId,
					"nodeId":     sd.ParentNodeId,
					"name":       sd.Name,
					"filePath":   sd.FilePath,
					"symbolType": sd.SymbolType,
				})
				if err != nil {
					slog.Warn("could not link symbol to node", "symbol_id", sd.SymbolId, "parent_node_id", sd.ParentNodeId, "error", err)
				}
			}

			return nil, nil
		})
		if err != nil {
			slog.Warn("symbol import failed", "symbol_id", sd.SymbolId, "error", err)
			continue
		}
		count++
	}

	return count, nil
}
