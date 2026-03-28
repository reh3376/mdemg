package api

import (
	_ "embed"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"mdemg/internal/models"
)

//go:embed static/topology3d.html
var topology3dHTML []byte

// handleVizTopology serves the 3D topology viewer.
func (s *Server) handleVizTopology(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(topology3dHTML) //nolint:errcheck
}

// Default edge types for topology visualization.
var defaultVizEdgeTypes = []string{
	"ABSTRACTS_TO", "CO_ACTIVATED_WITH", "GENERALIZES",
	"INFLUENCES", "BRIDGES", "GENERALIZES_TO",
	"SPECIALIZES", "ANALOGOUS_TO", "ASSOCIATED_WITH",
	"CAUSES", "ENABLES", "TEMPORALLY_ADJACENT",
	"IMPORTS", "CALLS", "EXTENDS", "IMPLEMENTS",
	"COMPOSES_WITH", "THEME_OF", "CONTRASTS_WITH",
	"CONTRADICTS", "ORIGINATED_FROM", "GROUNDED_BY",
	"DEFINES_SYMBOL", "INSTANTIATES",
}

// --- color maps ---

var layerColorMap = map[int]string{
	0: "#5794F2", // blue — raw
	1: "#5794F2", // blue — observations
	2: "#73BF69", // green — concepts
	3: "#FADE2A", // yellow — abstractions
	4: "#FF9830", // orange — meta
	5: "#F2495C", // red — paradigms
}

var edgeColorMap = map[string]string{
	// structural (purple)
	"ABSTRACTS_TO":    "#B877D9",
	"GENERALIZES":     "#B877D9",
	"GENERALIZES_TO":  "#B877D9",
	"SPECIALIZES":     "#B877D9",
	"INSTANTIATES":    "#B877D9",
	// learned (cyan)
	"CO_ACTIVATED_WITH": "#8AB8FF",
	// causal (orange)
	"INFLUENCES": "#FF9830",
	"CAUSES":     "#FF9830",
	"ENABLES":    "#FF9830",
	// cross-domain (magenta)
	"BRIDGES":      "#FF6EB4",
	"ANALOGOUS_TO": "#FF6EB4",
	// association (gray)
	"ASSOCIATED_WITH":     "#8F8F8F",
	"TEMPORALLY_ADJACENT": "#8F8F8F",
	// code (teal)
	"IMPORTS":        "#73BF69",
	"CALLS":         "#73BF69",
	"EXTENDS":       "#73BF69",
	"IMPLEMENTS":    "#73BF69",
	"DEFINES_SYMBOL": "#73BF69",
	// compositional (gold)
	"COMPOSES_WITH": "#FADE2A",
	"THEME_OF":      "#FADE2A",
	// provenance (light blue)
	"ORIGINATED_FROM": "#6ED0E0",
	"GROUNDED_BY":     "#6ED0E0",
	// contrast (red)
	"CONTRASTS_WITH": "#F2495C",
	"CONTRADICTS":    "#F2495C",
}

func layerColor(layer int) string {
	if c, ok := layerColorMap[layer]; ok {
		return c
	}
	if layer >= 5 {
		return "#F2495C"
	}
	return "#5794F2"
}

func edgeColor(relType string) string {
	if c, ok := edgeColorMap[relType]; ok {
		return c
	}
	return "#C0C0C0"
}

// uniformNodeRadius computes a single radius for all nodes based on how many
// nodes are in the viewport. Fewer nodes → larger; many nodes → smaller.
// Targets a ~1600x800 viewport area so nodes don't overlap or become tiny.
func uniformNodeRadius(nodeCount int) float64 {
	if nodeCount <= 0 {
		return 40
	}
	// viewport area ≈ 1600*800 = 1_280_000; each node occupies ~(2r)^2
	// solve: nodeCount * (2r)^2 * packing ≈ viewportArea
	// packing factor ~6 gives breathing room for edges/labels
	area := 1_280_000.0
	r := math.Sqrt(area/(float64(nodeCount)*6.0)) / 2.0
	if r < 20 {
		r = 20
	}
	if r > 60 {
		r = 60
	}
	return math.Round(r)
}

// parseLayers parses the "layers" query param (CSV of ints, or "ALL").
// Falls back to min_layer for backward compat. Returns []int64 for Neo4j.
func parseLayers(r *http.Request) []int64 {
	layersParam := r.URL.Query().Get("layers")
	if layersParam == "" {
		// backward compat: check min_layer
		if v := r.URL.Query().Get("min_layer"); v != "" {
			if minLayer, err := strconv.Atoi(v); err == nil && minLayer >= 0 && minLayer <= 5 {
				var out []int64
				for i := minLayer; i <= 5; i++ {
					out = append(out, int64(i))
				}
				return out
			}
		}
		return []int64{0, 1, 2, 3, 4, 5}
	}
	if strings.EqualFold(strings.TrimSpace(layersParam), "ALL") {
		return []int64{0, 1, 2, 3, 4, 5}
	}
	var out []int64
	for s := range strings.SplitSeq(layersParam, ",") {
		s = strings.TrimSpace(s)
		if v, err := strconv.Atoi(s); err == nil && v >= 0 && v <= 5 {
			out = append(out, int64(v))
		}
	}
	if len(out) == 0 {
		return []int64{0, 1, 2, 3, 4, 5}
	}
	return out
}

// parseFocusNodes parses a comma-separated list of node IDs.
func parseFocusNodes(r *http.Request) []string {
	param := r.URL.Query().Get("focus_nodes")
	if param == "" {
		return nil
	}
	var out []string
	for s := range strings.SplitSeq(param, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseEdgeTypes parses a comma-separated list of relationship types.
// Returns defaultVizEdgeTypes if empty.
func parseEdgeTypes(r *http.Request) []string {
	param := r.URL.Query().Get("edge_types")
	if param == "" {
		return defaultVizEdgeTypes
	}
	var out []string
	for s := range strings.SplitSeq(param, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return defaultVizEdgeTypes
	}
	return out
}

// parseHopDepth parses the hop_depth query param, clamped to 1-3.
func parseHopDepth(r *http.Request) int {
	v := r.URL.Query().Get("hop_depth")
	if v == "" {
		return 1
	}
	d, err := strconv.Atoi(v)
	if err != nil || d < 1 {
		return 1
	}
	if d > 3 {
		return 3
	}
	return d
}

// buildNode constructs a GraphVizNode from a Neo4j record.
func buildNode(rec *neo4j.Record) (models.GraphVizNode, bool) {
	nodeID := safeString(mustGet(rec, "id"))
	if nodeID == "" {
		return models.GraphVizNode{}, false
	}
	layer := safeInt(mustGet(rec, "layer"))
	confidence := safeFloat(mustGet(rec, "confidence"))
	edgeCount := safeInt(mustGet(rec, "edge_count"))
	roleType := safeString(mustGet(rec, "role_type"))
	stability := safeFloat(mustGet(rec, "stability"))

	// Enriched subtitle: "L3 · concept · 12 edges · stab:0.890"
	subtitle := fmt.Sprintf("L%d · %s · %d edges · stab:%.3f",
		layer, roleType, edgeCount, stability)

	return models.GraphVizNode{
		ID:               nodeID,
		Title:            safeString(mustGet(rec, "title")),
		SubTitle:         subtitle,
		MainStat:         fmt.Sprintf("%d edges", edgeCount),
		SecondaryStat:    fmt.Sprintf("C:%.2f", confidence),
		Color:            layerColor(layer),
		DetailRoleType:   roleType,
		DetailObsType:    safeString(mustGet(rec, "obs_type")),
		DetailTags:       safeStringList(mustGet(rec, "tags")),
		DetailStability:  fmt.Sprintf("%.3f", stability),
		DetailSurprise:   fmt.Sprintf("%.3f", safeFloat(mustGet(rec, "surprise"))),
		DetailCreatedAt:  safeString(mustGet(rec, "created_at")),
		DetailLastActive: safeString(mustGet(rec, "last_activated")),
		DetailEdgeCount:  strconv.Itoa(edgeCount),
		DetailStatus:     safeString(mustGet(rec, "status")),
	}, true
}

// enrichedNodeReturn is the RETURN clause for enriched node queries.
const enrichedNodeReturn = `
	RETURN n.node_id AS id,
	       COALESCE(n.name, n.summary, 'unnamed') AS title,
	       n.layer AS layer,
	       COALESCE(n.confidence, 0.0) AS confidence,
	       COALESCE(n.role_type, 'unknown') AS role_type,
	       COALESCE(n.obs_type, '') AS obs_type,
	       n.tags AS tags,
	       COALESCE(n.stability_score, 0.0) AS stability,
	       COALESCE(n.surprise_score, 0.0) AS surprise,
	       toString(n.created_at) AS created_at,
	       toString(n.last_activated) AS last_activated,
	       size([(n)-[]-() | 1]) AS edge_count,
	       COALESCE(n.status, 'active') AS status`

// handleGraphTopology returns graph nodes and edges in Grafana Node Graph format.
// GET /v1/memory/graph/topology?space_id=xxx&layers=2,3,4,5&page=1&page_size=500
func (s *Server) handleGraphTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}

	layers := parseLayers(r)
	focusNodes := parseFocusNodes(r)
	showHidden := r.URL.Query().Get("show_hidden") == "true"

	pageSize := 500
	if v := r.URL.Query().Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 5000 {
			pageSize = parsed
		}
	}
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			page = parsed
		}
	}
	// Backward compat: treat limit= as page_size when page= is absent.
	if r.URL.Query().Get("limit") != "" && r.URL.Query().Get("page") == "" {
		if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 && parsed <= 5000 {
			pageSize = parsed
		}
	}

	ctx := r.Context()
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Count total matching nodes for pagination metadata.
	var totalNodes int
	countParams := map[string]any{
		"spaceId":    spaceID,
		"layers":     layers,
		"showHidden": showHidden,
	}
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, cErr := tx.Run(ctx, `
			MATCH (n:MemoryNode {space_id: $spaceId})
			WHERE n.layer IN $layers
			  AND ($showHidden OR coalesce(n.status, 'active') <> 'hidden')
			RETURN count(n) AS total`, countParams)
		if cErr != nil {
			return nil, cErr
		}
		if res.Next(ctx) {
			totalNodes = int(res.Record().Values[0].(int64))
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("graph-topology: count query failed", "error", err)
		writeInternalError(w, err, "graph topology count")
		return
	}

	totalPages := max((totalNodes+pageSize-1)/pageSize, 1)
	if page > totalPages {
		page = totalPages
	}

	// Query nodes for the requested page.
	nodeMap := make(map[string]models.GraphVizNode)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		var cypher string
		params := map[string]any{
			"spaceId":    spaceID,
			"layers":     layers,
			"showHidden": showHidden,
			"skip":       int64((page - 1) * pageSize),
			"limit":      int64(pageSize),
		}

		if len(focusNodes) > 0 {
			// Focus mode: show selected nodes + 1-hop neighbors within layer filter
			cypher = `
				MATCH (center:MemoryNode {space_id: $spaceId})
				WHERE center.node_id IN $focusNodes AND center.layer IN $layers
				OPTIONAL MATCH (center)-[]-(neighbor:MemoryNode {space_id: $spaceId})
				WHERE neighbor.layer IN $layers
				  AND ($showHidden OR coalesce(neighbor.status, 'active') <> 'hidden')
				WITH collect(DISTINCT center) + collect(DISTINCT neighbor) AS all_nodes
				UNWIND all_nodes AS n
				WITH DISTINCT n` + enrichedNodeReturn + `
				ORDER BY edge_count DESC
				SKIP $skip LIMIT $limit`
			params["focusNodes"] = focusNodes
		} else {
			// Full topology mode — layer-balanced selection.
			// Pick top nodes from each layer proportionally so cross-layer edges
			// are visible (pure edge_count sort concentrates on L0 only).
			cypher = `
				MATCH (n:MemoryNode {space_id: $spaceId})
				WHERE n.layer IN $layers
				  AND ($showHidden OR coalesce(n.status, 'active') <> 'hidden')
				WITH n.layer AS lyr, count(n) AS lyrCount
				WITH lyr, lyrCount,
				     toInteger(ceil(toFloat(lyrCount) / $totalMatching * $limit)) AS quota
				ORDER BY lyr
				WITH collect({layer: lyr, quota: quota}) AS quotas
				UNWIND quotas AS q
				MATCH (n:MemoryNode {space_id: $spaceId})
				WHERE n.layer = q.layer
				  AND ($showHidden OR coalesce(n.status, 'active') <> 'hidden')
				WITH n, q.quota AS quota
				ORDER BY size([(n)-[]-() | 1]) DESC
				WITH n, quota
				WITH n.layer AS lyr, collect(n) AS layerNodes, quota
				WITH lyr, layerNodes[toInteger($skip * quota / $limit)..toInteger($skip * quota / $limit) + quota] AS pageNodes
				UNWIND pageNodes AS n` + enrichedNodeReturn + `
				ORDER BY edge_count DESC`
			params["totalMatching"] = int64(totalNodes)
		}

		res, qErr := tx.Run(ctx, cypher, params)
		if qErr != nil {
			return nil, qErr
		}
		for res.Next(ctx) {
			if node, ok := buildNode(res.Record()); ok {
				nodeMap[node.ID] = node
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("graph-topology: node query failed", "error", err)
		writeInternalError(w, err, "graph topology nodes")
		return
	}

	// Collect node IDs for the edge query (only edges between displayed nodes).
	collectedIDs := make([]string, 0, len(nodeMap))
	for id := range nodeMap {
		collectedIDs = append(collectedIDs, id)
	}

	edges := make([]models.GraphVizEdge, 0)
	if len(collectedIDs) > 0 {
		_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			res, eErr := tx.Run(ctx, `
				MATCH (a:MemoryNode {space_id: $spaceId})-[r]->(b:MemoryNode {space_id: $spaceId})
				WHERE a.node_id IN $nodeIds AND b.node_id IN $nodeIds
				  AND type(r) IN $edgeTypes
				RETURN a.node_id AS source, b.node_id AS target,
				       type(r) AS relType, elementId(r) AS rid`,
				map[string]any{
					"spaceId":   spaceID,
					"nodeIds":   collectedIDs,
					"edgeTypes": defaultVizEdgeTypes,
				})
			if eErr != nil {
				return nil, eErr
			}
			for res.Next(ctx) {
				rec := res.Record()
				relType := safeString(mustGet(rec, "relType"))
				edges = append(edges, models.GraphVizEdge{
					ID:       safeString(mustGet(rec, "rid")),
					Source:   safeString(mustGet(rec, "source")),
					Target:   safeString(mustGet(rec, "target")),
					MainStat: relType,
					Color:    edgeColor(relType),
				})
			}
			return nil, res.Err()
		})
		if err != nil {
			slog.Error("graph-topology: edge query failed", "error", err)
			writeInternalError(w, err, "graph topology edges")
			return
		}
	}

	// Compute uniform radius based on node count and apply to all nodes.
	radius := uniformNodeRadius(len(nodeMap))
	nodes := make([]models.GraphVizNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		n.NodeRadius = radius
		nodes = append(nodes, n)
	}

	writeJSON(w, http.StatusOK, models.GraphVizResponse{
		Nodes:      nodes,
		Edges:      edges,
		TotalNodes: totalNodes,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// handleGraphNeighborhood returns the N-hop neighborhood around one or more nodes.
// GET /v1/memory/graph/neighborhood?space_id=xxx&node_id=yyy,zzz&hop_depth=2&edge_types=ABSTRACTS_TO,CO_ACTIVATED_WITH&page_size=500
func (s *Server) handleGraphNeighborhood(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	spaceID := r.URL.Query().Get("space_id")
	if spaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "space_id is required"})
		return
	}

	// Accept comma-separated node_ids. Return empty graph when none selected
	// (Grafana sends empty string when focus_nodes variable has no selection).
	nodeIDParam := r.URL.Query().Get("node_id")
	var nodeIDs []string
	if nodeIDParam != "" {
		for s := range strings.SplitSeq(nodeIDParam, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				nodeIDs = append(nodeIDs, s)
			}
		}
	}
	if len(nodeIDs) == 0 {
		writeJSON(w, http.StatusOK, models.GraphVizResponse{
			Nodes: []models.GraphVizNode{}, Edges: []models.GraphVizEdge{},
			TotalNodes: 0, Page: 1, PageSize: 0, TotalPages: 0,
		})
		return
	}

	hopDepth := parseHopDepth(r)
	edgeTypes := parseEdgeTypes(r)

	limit := 500
	if v := r.URL.Query().Get("page_size"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 5000 {
			limit = parsed
		}
	}
	// Backward compat
	if r.URL.Query().Get("limit") != "" && r.URL.Query().Get("page_size") == "" {
		if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 && parsed <= 5000 {
			limit = parsed
		}
	}

	ctx := r.Context()
	sess := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Pass 1: Collect all distinct nodes reachable within hopDepth hops.
	// Neo4j doesn't support parameterized path lengths, so build dynamically.
	nodeMap := make(map[string]models.GraphVizNode)
	_, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		cypher := fmt.Sprintf(`
			MATCH (center:MemoryNode {space_id: $spaceId})
			WHERE center.node_id IN $nodeIds
			WITH center
			MATCH (center)-[*1..%d]-(n:MemoryNode {space_id: $spaceId})
			WITH collect(DISTINCT center) + collect(DISTINCT n) AS all_nodes
			UNWIND all_nodes AS n
			WITH DISTINCT n`, hopDepth) + enrichedNodeReturn + `
			LIMIT $limit`

		res, err := tx.Run(ctx, cypher, map[string]any{
			"spaceId": spaceID,
			"nodeIds": nodeIDs,
			"limit":   int64(limit),
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			if node, ok := buildNode(res.Record()); ok {
				nodeMap[node.ID] = node
			}
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("graph-neighborhood: node query failed", "error", err)
		writeInternalError(w, err, "graph neighborhood nodes")
		return
	}

	// Pass 2: Collect edges between the collected nodes, filtered by edge_types.
	collectedIDs := make([]string, 0, len(nodeMap))
	for id := range nodeMap {
		collectedIDs = append(collectedIDs, id)
	}

	edges := make([]models.GraphVizEdge, 0)
	_, err = sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `
			MATCH (a:MemoryNode {space_id: $spaceId})-[r]->(b:MemoryNode {space_id: $spaceId})
			WHERE a.node_id IN $nodeIds AND b.node_id IN $nodeIds
			  AND type(r) IN $edgeTypes
			RETURN a.node_id AS source, b.node_id AS target,
			       type(r) AS relType, elementId(r) AS rid
			LIMIT $edgeLimit`,
			map[string]any{
				"spaceId":   spaceID,
				"nodeIds":   collectedIDs,
				"edgeTypes": edgeTypes,
				"edgeLimit": int64(limit * 8),
			})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			relType := safeString(mustGet(rec, "relType"))
			edges = append(edges, models.GraphVizEdge{
				ID:       safeString(mustGet(rec, "rid")),
				Source:   safeString(mustGet(rec, "source")),
				Target:   safeString(mustGet(rec, "target")),
				MainStat: relType,
				Color:    edgeColor(relType),
			})
		}
		return nil, res.Err()
	})
	if err != nil {
		slog.Error("graph-neighborhood: edge query failed", "error", err)
		writeInternalError(w, err, "graph neighborhood edges")
		return
	}

	radius := uniformNodeRadius(len(nodeMap))
	nodes := make([]models.GraphVizNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		n.NodeRadius = radius
		nodes = append(nodes, n)
	}

	nodeCount := len(nodes)
	writeJSON(w, http.StatusOK, models.GraphVizResponse{
		Nodes:      nodes,
		Edges:      edges,
		TotalNodes: nodeCount,
		Page:       1,
		PageSize:   nodeCount,
		TotalPages: 1,
	})
}

// handleNodeGraphData serves the /api/graph/data endpoint expected by the
// Node Graph API plugin. Dispatches to topology or neighborhood based on
// the "type" query param.
func (s *Server) handleNodeGraphData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	switch r.URL.Query().Get("type") {
	case "neighborhood":
		s.handleGraphNeighborhood(w, r)
	default:
		s.handleGraphTopology(w, r)
	}
}

// handleNodeGraphFields returns field definitions for the Node Graph API plugin.
// GET /api/graph/fields
func (s *Server) handleNodeGraphFields(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"edges_fields": []map[string]string{
			{"field_name": "id", "type": "string"},
			{"field_name": "source", "type": "string"},
			{"field_name": "target", "type": "string"},
			{"field_name": "mainStat", "type": "string", "displayName": "Relationship"},
			{"field_name": "color", "type": "string"},
		},
		"nodes_fields": []map[string]string{
			{"field_name": "id", "type": "string"},
			{"field_name": "title", "type": "string"},
			{"field_name": "subTitle", "type": "string"},
			{"field_name": "mainStat", "type": "string", "displayName": "Edges"},
			{"field_name": "secondaryStat", "type": "string", "displayName": "Confidence"},
			{"field_name": "color", "type": "string"},
			{"field_name": "nodeRadius", "type": "number"},
			{"field_name": "detail__role_type", "type": "string", "displayName": "Role Type"},
			{"field_name": "detail__obs_type", "type": "string", "displayName": "Observation Type"},
			{"field_name": "detail__tags", "type": "string", "displayName": "Tags"},
			{"field_name": "detail__stability", "type": "string", "displayName": "Stability"},
			{"field_name": "detail__surprise", "type": "string", "displayName": "Surprise"},
			{"field_name": "detail__created_at", "type": "string", "displayName": "Created At"},
			{"field_name": "detail__last_activated", "type": "string", "displayName": "Last Activated"},
			{"field_name": "detail__edge_count", "type": "string", "displayName": "Edge Count"},
			{"field_name": "detail__status", "type": "string", "displayName": "Status"},
		},
	})
}

// handleNodeGraphHealth serves /api/graph/health for the Node Graph API plugin.
func (s *Server) handleNodeGraphHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

func mustGet(rec *neo4j.Record, key string) any {
	v, _ := rec.Get(key)
	return v
}

func safeString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func safeStringList(v any) string {
	if v == nil {
		return ""
	}
	if list, ok := v.([]any); ok {
		parts := make([]string, 0, len(list))
		for _, item := range list {
			if s, ok := item.(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return safeString(v)
}

func safeInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func safeFloat(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}
