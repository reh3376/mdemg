// MDEMG MCP Server - Provides memory tools for coding agents
// Wraps the MCP server as a Cobra command for unified CLI
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

const (
	defaultMCPEndpoint = "http://localhost:9999"
	defaultSpaceID     = "ide-agent"
)

// mcpServer holds MCP server state (avoids global variables)
type mcpServer struct {
	endpoint   string
	httpClient *http.Client
}

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run MDEMG MCP server for IDE integration",
		Long: `Start the MDEMG MCP (Model Context Protocol) server.

This server provides memory tools for coding agents through the MCP protocol,
enabling integration with IDEs like Cursor and other AI coding assistants.

The server runs in stdio mode for agent communication.`,
		RunE: runMCPServer,
	}

	return cmd
}

func runMCPServer(cmd *cobra.Command, args []string) error {
	// Resolve endpoint via priority chain: MDEMG_ENDPOINT env > .mdemg.port file > LISTEN_ADDR > default
	endpoint := config.ResolveEndpoint(defaultMCPEndpoint)

	// Create MCP server state
	mcpSrv := &mcpServer{
		endpoint:   endpoint,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	// Create MCP server
	s := server.NewMCPServer(
		"MDEMG Memory",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register all tools
	mcpSrv.registerMemoryStoreToool(s)
	mcpSrv.registerMemoryRecallTool(s)
	mcpSrv.registerMemoryAssociateTool(s)
	mcpSrv.registerMemoryReflectTool(s)
	mcpSrv.registerMemoryStatusTool(s)
	mcpSrv.registerMemorySymbolsTool(s)
	mcpSrv.registerMemoryIngestTriggerTool(s)
	mcpSrv.registerMemoryIngestStatusTool(s)
	mcpSrv.registerMemoryIngestCancelTool(s)
	mcpSrv.registerMemoryIngestJobsTool(s)
	mcpSrv.registerMemoryIngestFilesTool(s)
	mcpSrv.registerMemorySpaceFreshnessTool(s)

	// Linear CRUD tools
	mcpSrv.registerLinearCreateIssueTool(s)
	mcpSrv.registerLinearListIssuesTool(s)
	mcpSrv.registerLinearReadIssueTool(s)
	mcpSrv.registerLinearUpdateIssueTool(s)
	mcpSrv.registerLinearAddCommentTool(s)
	mcpSrv.registerLinearSearchTool(s)

	// Guardrail tools (Phase 104)
	mcpSrv.registerValidateChangesTool(s)

	// Start server (stdio mode for Cursor integration)
	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}

	return nil
}

// =============================================================================
// Memory Tools
// =============================================================================

// memory_store - Store an observation about code, decisions, patterns, or learnings
func (m *mcpServer) registerMemoryStoreToool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_store",
		mcp.WithDescription(`Store a memory observation for later recall. Use this to remember:
- Code patterns and idioms you discover
- Decisions made and their rationale
- Problems solved and their solutions
- User preferences and conventions
- Project-specific knowledge

The memory system will automatically generate embeddings and link related concepts.`),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The content to remember. Be specific and include context.")),
		mcp.WithString("name",
			mcp.Description("Short name/title for this memory (optional, auto-generated if not provided)")),
		mcp.WithString("path",
			mcp.Description("Hierarchical path like /project/component/topic (optional)")),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags for categorization (e.g., 'golang,error-handling,pattern')")),
		mcp.WithString("source",
			mcp.Description("Source of this observation (e.g., 'code-review', 'debugging', 'user-request')")),
	)

	s.AddTool(tool, m.memoryStoreHandler)
}

func (m *mcpServer) memoryStoreHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	content, _ := args["content"].(string)
	if content == "" {
		return newToolResultError("content is required"), nil
	}

	name, _ := args["name"].(string)
	path, _ := args["path"].(string)
	tagsStr, _ := args["tags"].(string)
	source, _ := args["source"].(string)

	if source == "" {
		source = "agent-observation"
	}

	// Parse tags
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	// Build ingest request
	ingestReq := map[string]any{
		"space_id":  defaultSpaceID,
		"content":   content,
		"source":    source,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if name != "" {
		ingestReq["name"] = name
	}
	if path != "" {
		ingestReq["path"] = path
	}
	if len(tags) > 0 {
		ingestReq["tags"] = tags
	}

	// Call MDEMG API
	resp, err := m.callMDEMG("/v1/memory/ingest", ingestReq)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to store memory: %v", err)), nil
	}

	nodeID, _ := resp["node_id"].(string)
	embDims, _ := resp["embedding_dims"].(float64)

	result := fmt.Sprintf("Memory stored successfully.\nNode ID: %s\nEmbedding dimensions: %d\nPath: %s",
		nodeID, int(embDims), path)

	return mcp.NewToolResultText(result), nil
}

// memory_recall - Retrieve relevant memories based on the current context
func (m *mcpServer) registerMemoryRecallTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_recall",
		mcp.WithDescription(`Recall relevant memories based on a query. Use this to:
- Find previous solutions to similar problems
- Remember decisions made about a topic
- Retrieve learned patterns and idioms
- Get context about a project or component

Returns memories ranked by relevance, with semantic similarity and activation scores.`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("What you're looking for - describe the context or problem")),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of memories to return (default: 10)")),
	)

	s.AddTool(tool, m.memoryRecallHandler)
}

func (m *mcpServer) memoryRecallHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	query, _ := args["query"].(string)
	if query == "" {
		return newToolResultError("query is required"), nil
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	// Call MDEMG API
	retrieveReq := map[string]any{
		"space_id":   defaultSpaceID,
		"query_text": query,
		"top_k":      limit,
	}

	resp, err := m.callMDEMG("/v1/memory/retrieve", retrieveReq)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to recall memories: %v", err)), nil
	}

	results, _ := resp["results"].([]any)
	if len(results) == 0 {
		return mcp.NewToolResultText("No relevant memories found."), nil
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant memories:\n\n", len(results)))

	for i, r := range results {
		result, _ := r.(map[string]any)
		name, _ := result["name"].(string)
		path, _ := result["path"].(string)
		summary, _ := result["summary"].(string)
		score, _ := result["score"].(float64)
		vectorSim, _ := result["vector_sim"].(float64)

		sb.WriteString(fmt.Sprintf("## %d. %s\n", i+1, name))
		if path != "" {
			sb.WriteString(fmt.Sprintf("   Path: %s\n", path))
		}
		sb.WriteString(fmt.Sprintf("   Relevance: %.1f%% (semantic: %.1f%%)\n", score*100, vectorSim*100))
		if summary != "" {
			sb.WriteString(fmt.Sprintf("   Summary: %s\n", summary))
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_associate - Explicitly link two concepts or memories
func (m *mcpServer) registerMemoryAssociateTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_associate",
		mcp.WithDescription(`Create an explicit association between two concepts or memories.
Use this when you discover that two things are related in a way the system might not
automatically detect (e.g., a workaround relates to a specific bug, or a pattern
applies to a particular domain).`),
		mcp.WithString("source_query",
			mcp.Required(),
			mcp.Description("Query to find the source memory/concept")),
		mcp.WithString("target_query",
			mcp.Required(),
			mcp.Description("Query to find the target memory/concept")),
		mcp.WithString("relationship",
			mcp.Description("Type of relationship: 'related', 'causes', 'enables', 'similar' (default: 'related')")),
		mcp.WithString("reason",
			mcp.Description("Why these concepts are associated")),
	)

	s.AddTool(tool, m.memoryAssociateHandler)
}

func (m *mcpServer) memoryAssociateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	sourceQuery, _ := args["source_query"].(string)
	targetQuery, _ := args["target_query"].(string)

	if sourceQuery == "" || targetQuery == "" {
		return newToolResultError("both source_query and target_query are required"), nil
	}

	relationship, _ := args["relationship"].(string)
	if relationship == "" {
		relationship = "related"
	}
	reason, _ := args["reason"].(string)

	// First, find both memories
	sourceResp, err := m.callMDEMG("/v1/memory/retrieve", map[string]any{
		"space_id":   defaultSpaceID,
		"query_text": sourceQuery,
		"top_k":      1,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to find source: %v", err)), nil
	}

	targetResp, err := m.callMDEMG("/v1/memory/retrieve", map[string]any{
		"space_id":   defaultSpaceID,
		"query_text": targetQuery,
		"top_k":      1,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to find target: %v", err)), nil
	}

	sourceResults, _ := sourceResp["results"].([]any)
	targetResults, _ := targetResp["results"].([]any)

	if len(sourceResults) == 0 {
		return newToolResultError("Could not find a memory matching source_query"), nil
	}
	if len(targetResults) == 0 {
		return newToolResultError("Could not find a memory matching target_query"), nil
	}

	sourceNode, _ := sourceResults[0].(map[string]any)
	targetNode, _ := targetResults[0].(map[string]any)

	sourceName, _ := sourceNode["name"].(string)
	targetName, _ := targetNode["name"].(string)

	// For now, store the association as a new observation linking the concepts
	// In the future, this would create a direct edge in the graph
	associationContent := fmt.Sprintf("Association: '%s' %s '%s'", sourceName, relationship, targetName)
	if reason != "" {
		associationContent += fmt.Sprintf("\nReason: %s", reason)
	}

	_, err = m.callMDEMG("/v1/memory/ingest", map[string]any{
		"space_id":  defaultSpaceID,
		"content":   associationContent,
		"source":    "explicit-association",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"name":      fmt.Sprintf("Link: %s → %s", sourceName, targetName),
		"tags":      []string{"association", relationship},
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to store association: %v", err)), nil
	}

	result := fmt.Sprintf("Association created:\n  '%s' --[%s]--> '%s'", sourceName, relationship, targetName)
	return mcp.NewToolResultText(result), nil
}

// memory_reflect - Summarize what is known about a topic
func (m *mcpServer) registerMemoryReflectTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_reflect",
		mcp.WithDescription(`Reflect on what is known about a topic, triggering a broader search
and potentially identifying patterns or abstractions. Use this for:
- Understanding the full context around a concept
- Finding patterns across multiple related memories
- Preparing to make a decision by reviewing relevant knowledge`),
		mcp.WithString("topic",
			mcp.Required(),
			mcp.Description("The topic to reflect on")),
		mcp.WithNumber("depth",
			mcp.Description("How deeply to explore (1-3, default: 2)")),
	)

	s.AddTool(tool, m.memoryReflectHandler)
}

func (m *mcpServer) memoryReflectHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	topic, _ := args["topic"].(string)
	if topic == "" {
		return newToolResultError("topic is required"), nil
	}

	depth := 2
	if d, ok := args["depth"].(float64); ok && d >= 1 && d <= 3 {
		depth = int(d)
	}

	// Retrieve with more candidates for deeper reflection
	candidateK := 50 * depth
	topK := 10 * depth

	resp, err := m.callMDEMG("/v1/memory/retrieve", map[string]any{
		"space_id":    defaultSpaceID,
		"query_text":  topic,
		"candidate_k": candidateK,
		"top_k":       topK,
		"hop_depth":   depth,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to reflect: %v", err)), nil
	}

	results, _ := resp["results"].([]any)
	debug, _ := resp["debug"].(map[string]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Reflection on: %s\n\n", topic))

	if len(results) == 0 {
		sb.WriteString("No memories found on this topic yet.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	// Group by relevance tiers
	var highRelevance, medRelevance, lowRelevance []map[string]any
	for _, r := range results {
		result, _ := r.(map[string]any)
		score, _ := result["score"].(float64)
		if score >= 0.7 {
			highRelevance = append(highRelevance, result)
		} else if score >= 0.4 {
			medRelevance = append(medRelevance, result)
		} else {
			lowRelevance = append(lowRelevance, result)
		}
	}

	sb.WriteString(fmt.Sprintf("Found %d relevant memories (depth: %d hops)\n\n", len(results), depth))

	if len(highRelevance) > 0 {
		sb.WriteString("## Highly Relevant\n")
		for _, r := range highRelevance {
			name, _ := r["name"].(string)
			path, _ := r["path"].(string)
			score, _ := r["score"].(float64)
			sb.WriteString(fmt.Sprintf("- **%s** (%.0f%%) %s\n", name, score*100, path))
		}
		sb.WriteString("\n")
	}

	if len(medRelevance) > 0 {
		sb.WriteString("## Related\n")
		for _, r := range medRelevance {
			name, _ := r["name"].(string)
			score, _ := r["score"].(float64)
			sb.WriteString(fmt.Sprintf("- %s (%.0f%%)\n", name, score*100))
		}
		sb.WriteString("\n")
	}

	if len(lowRelevance) > 0 {
		sb.WriteString("## Tangentially Related\n")
		for _, r := range lowRelevance {
			name, _ := r["name"].(string)
			score, _ := r["score"].(float64)
			sb.WriteString(fmt.Sprintf("- %s (%.0f%%)\n", name, score*100))
		}
		sb.WriteString("\n")
	}

	// Add debug info
	if debug != nil {
		edgesFetched, _ := debug["edges_fetched"].(float64)
		if edgesFetched > 0 {
			sb.WriteString(fmt.Sprintf("*Graph traversal explored %d edges*\n", int(edgesFetched)))
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_status - Check the memory system status
func (m *mcpServer) registerMemoryStatusTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_status",
		mcp.WithDescription("Check if the MDEMG memory system is running and get its status."),
	)

	s.AddTool(tool, m.memoryStatusHandler)
}

func (m *mcpServer) memoryStatusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(m.endpoint + "/readyz")
	if err != nil {
		return newToolResultError(fmt.Sprintf("MDEMG service not reachable at %s: %v", m.endpoint, err)), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var status map[string]any
	_ = json.Unmarshal(body, &status)

	var sb strings.Builder
	sb.WriteString("MDEMG Memory System Status\n\n")
	sb.WriteString(fmt.Sprintf("Endpoint: %s\n", m.endpoint))
	sb.WriteString(fmt.Sprintf("Status: %v\n", status["status"]))

	if provider, ok := status["embedding_provider"].(string); ok {
		sb.WriteString(fmt.Sprintf("Embedding Provider: %s\n", provider))
	}
	if dims, ok := status["embedding_dimensions"].(float64); ok {
		sb.WriteString(fmt.Sprintf("Embedding Dimensions: %d\n", int(dims)))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_symbols - Search for code symbols (functions, classes, constants)
func (m *mcpServer) registerMemorySymbolsTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_symbols",
		mcp.WithDescription(`Search for code symbols in the memory graph. Use this to:
- Find function definitions and their locations
- Search for classes, interfaces, and types
- Locate constants and variables
- Discover exported vs unexported symbols

Returns symbols with their file locations, types, and metadata.`),
		mcp.WithString("name",
			mcp.Description("Symbol name pattern to search for (partial match supported)")),
		mcp.WithString("type",
			mcp.Description("Symbol type to filter by: function, class, method, constant, variable, interface, type")),
		mcp.WithString("file",
			mcp.Description("Filter symbols by file path (partial match)")),
		mcp.WithString("exported",
			mcp.Description("Filter by exported status: 'true' for exported only, 'false' for unexported only")),
		mcp.WithString("q",
			mcp.Description("Fulltext search query across all symbol metadata")),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of symbols to return (default: 20)")),
	)

	s.AddTool(tool, m.memorySymbolsHandler)
}

func (m *mcpServer) memorySymbolsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	// Build query parameters
	params := make(map[string]string)
	params["space_id"] = defaultSpaceID

	if name, ok := args["name"].(string); ok && name != "" {
		params["name"] = name
	}
	if symbolType, ok := args["type"].(string); ok && symbolType != "" {
		params["type"] = symbolType
	}
	if file, ok := args["file"].(string); ok && file != "" {
		params["file"] = file
	}
	if exported, ok := args["exported"].(string); ok && exported != "" {
		params["exported"] = exported
	}
	if q, ok := args["q"].(string); ok && q != "" {
		params["q"] = q
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	params["limit"] = fmt.Sprintf("%d", limit)

	// Call MDEMG API (GET request with query params)
	resp, err := m.callMDEMGGet("/v1/memory/symbols", params)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to search symbols: %v", err)), nil
	}

	symbols, _ := resp["symbols"].([]any)
	total, _ := resp["total"].(float64)

	if len(symbols) == 0 {
		return mcp.NewToolResultText("No symbols found matching the criteria."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d symbols (showing %d):\n\n", int(total), len(symbols)))

	for i, s := range symbols {
		sym, _ := s.(map[string]any)
		name, _ := sym["name"].(string)
		symType, _ := sym["type"].(string)
		file, _ := sym["file"].(string)
		line, _ := sym["line"].(float64)
		exported, _ := sym["exported"].(bool)

		exportedStr := ""
		if exported {
			exportedStr = " (exported)"
		}

		sb.WriteString(fmt.Sprintf("%d. **%s** [%s]%s\n", i+1, name, symType, exportedStr))
		if file != "" {
			if line > 0 {
				sb.WriteString(fmt.Sprintf("   Location: %s:%d\n", file, int(line)))
			} else {
				sb.WriteString(fmt.Sprintf("   File: %s\n", file))
			}
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_ingest_trigger - Trigger a background ingestion job
func (m *mcpServer) registerMemoryIngestTriggerTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_trigger",
		mcp.WithDescription(`Trigger a background ingestion job to import code from a directory.
The job runs asynchronously. Use memory_ingest_status to check progress.
Use this when you want to import or re-import a codebase into memory.`),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to the source directory to ingest")),
		mcp.WithString("mode",
			mcp.Description("Ingestion mode: 'full' (re-ingest everything) or 'incremental' (only changed files). Default: 'full'")),
		mcp.WithString("exclude_dirs",
			mcp.Description("Comma-separated directories to exclude (e.g., 'vendor,node_modules')")),
	)

	s.AddTool(tool, m.memoryIngestTriggerHandler)
}

func (m *mcpServer) memoryIngestTriggerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	path, _ := args["path"].(string)
	if path == "" {
		return newToolResultError("path is required"), nil
	}

	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "full"
	}

	ingestReq := map[string]any{
		"space_id": defaultSpaceID,
		"path":     path,
	}

	// Map mode to incremental boolean
	if mode == "incremental" {
		ingestReq["incremental"] = true
	}

	// Map exclude_dirs comma-separated string to []string
	if excludeDirs, ok := args["exclude_dirs"].(string); ok && excludeDirs != "" {
		dirs := strings.Split(excludeDirs, ",")
		trimmed := make([]string, 0, len(dirs))
		for _, d := range dirs {
			if s := strings.TrimSpace(d); s != "" {
				trimmed = append(trimmed, s)
			}
		}
		ingestReq["exclude_dirs"] = trimmed
	}

	resp, err := m.callMDEMG("/v1/memory/ingest/trigger", ingestReq)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to trigger ingestion: %v", err)), nil
	}

	jobID, _ := resp["job_id"].(string)
	status, _ := resp["status"].(string)

	result := fmt.Sprintf("Ingestion job started.\nJob ID: %s\nStatus: %s\n\nUse memory_ingest_status with job_id '%s' to check progress.", jobID, status, jobID)
	return mcp.NewToolResultText(result), nil
}

// memory_ingest_status - Check the status of an ingestion job
func (m *mcpServer) registerMemoryIngestStatusTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_status",
		mcp.WithDescription(`Check the status and progress of an ingestion job.
Returns the current status, progress percentage, and any errors.`),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("The job ID returned by memory_ingest_trigger")),
	)

	s.AddTool(tool, m.memoryIngestStatusHandler)
}

func (m *mcpServer) memoryIngestStatusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	jobID, _ := args["job_id"].(string)
	if jobID == "" {
		return newToolResultError("job_id is required"), nil
	}

	resp, err := m.callMDEMGGet("/v1/memory/ingest/status/"+jobID, nil)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get job status: %v", err)), nil
	}

	status, _ := resp["status"].(string)
	progress, _ := resp["progress"].(map[string]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Job ID: %s\n", jobID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", status))

	if progress != nil {
		current, _ := progress["current"].(float64)
		total, _ := progress["total"].(float64)
		percentage, _ := progress["percentage"].(float64)
		rate, _ := progress["rate"].(float64)

		if total > 0 {
			sb.WriteString(fmt.Sprintf("Progress: %d/%d (%.1f%%)\n", int(current), int(total), percentage))
		}
		if rate > 0 {
			sb.WriteString(fmt.Sprintf("Rate: %.1f items/sec\n", rate))
		}
	}

	if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s\n", errMsg))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_ingest_cancel - Cancel a running ingestion job
func (m *mcpServer) registerMemoryIngestCancelTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_cancel",
		mcp.WithDescription(`Cancel a running ingestion job. The job will be stopped as soon as possible.`),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("The job ID to cancel")),
	)

	s.AddTool(tool, m.memoryIngestCancelHandler)
}

func (m *mcpServer) memoryIngestCancelHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	jobID, _ := args["job_id"].(string)
	if jobID == "" {
		return newToolResultError("job_id is required"), nil
	}

	resp, err := m.callMDEMG("/v1/memory/ingest/cancel/"+jobID, map[string]any{})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to cancel job: %v", err)), nil
	}

	status, _ := resp["status"].(string)
	result := fmt.Sprintf("Job %s cancellation requested.\nNew status: %s", jobID, status)
	return mcp.NewToolResultText(result), nil
}

// memory_ingest_jobs - List all ingestion jobs
func (m *mcpServer) registerMemoryIngestJobsTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_jobs",
		mcp.WithDescription(`List all ingestion jobs with their current status.
Shows pending, running, completed, and failed jobs.`),
	)

	s.AddTool(tool, m.memoryIngestJobsHandler)
}

func (m *mcpServer) memoryIngestJobsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := m.callMDEMGGet("/v1/memory/ingest/jobs", nil)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
	}

	jobs, _ := resp["jobs"].([]any)
	if len(jobs) == 0 {
		return mcp.NewToolResultText("No ingestion jobs found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Ingestion Jobs (%d):\n\n", len(jobs)))

	for _, j := range jobs {
		job, _ := j.(map[string]any)
		jobID, _ := job["job_id"].(string)
		status, _ := job["status"].(string)
		jobType, _ := job["type"].(string)
		createdAt, _ := job["created_at"].(string)

		sb.WriteString(fmt.Sprintf("- **%s** [%s]\n", jobID, status))
		sb.WriteString(fmt.Sprintf("  Type: %s\n", jobType))
		sb.WriteString(fmt.Sprintf("  Created: %s\n", createdAt))

		if progress, ok := job["progress"].(map[string]any); ok {
			if pct, ok := progress["percentage"].(float64); ok {
				sb.WriteString(fmt.Sprintf("  Progress: %.1f%%\n", pct))
			}
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_ingest_files - Re-ingest specific files into memory
func (m *mcpServer) registerMemoryIngestFilesTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_files",
		mcp.WithDescription(`Re-ingest specific files into memory.
Use this when you want to update memory for specific files that have changed.
More targeted than a full codebase re-ingestion.`),
		mcp.WithString("files",
			mcp.Required(),
			mcp.Description("Comma-separated list of file paths to re-ingest")),
		mcp.WithString("space_id",
			mcp.Description("Space ID (default: ide-agent)")),
	)

	s.AddTool(tool, m.memoryIngestFilesHandler)
}

func (m *mcpServer) memoryIngestFilesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	filesStr, _ := args["files"].(string)
	if filesStr == "" {
		return newToolResultError("files is required"), nil
	}

	// Parse comma-separated file paths
	rawFiles := strings.Split(filesStr, ",")
	files := make([]string, 0, len(rawFiles))
	for _, f := range rawFiles {
		if s := strings.TrimSpace(f); s != "" {
			files = append(files, s)
		}
	}
	if len(files) == 0 {
		return newToolResultError("at least one file path is required"), nil
	}

	spaceID, _ := args["space_id"].(string)
	if spaceID == "" {
		spaceID = defaultSpaceID
	}

	ingestReq := map[string]any{
		"space_id": spaceID,
		"files":    files,
	}

	resp, err := m.callMDEMG("/v1/memory/ingest/files", ingestReq)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to ingest files: %v", err)), nil
	}

	totalFiles, _ := resp["total_files"].(float64)
	successCount, _ := resp["success_count"].(float64)
	errorCount, _ := resp["error_count"].(float64)
	jobID, _ := resp["job_id"].(string)

	if jobID != "" {
		result := fmt.Sprintf("File ingestion job started (too many files for synchronous processing).\nJob ID: %s\nTotal files: %d\n\nUse memory_ingest_status with job_id '%s' to check progress.",
			jobID, int(totalFiles), jobID)
		return mcp.NewToolResultText(result), nil
	}

	result := fmt.Sprintf("File ingestion complete.\nTotal: %d\nSuccess: %d\nErrors: %d",
		int(totalFiles), int(successCount), int(errorCount))
	return mcp.NewToolResultText(result), nil
}

// memory_space_freshness - Check the freshness/staleness of a memory space
func (m *mcpServer) registerMemorySpaceFreshnessTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_space_freshness",
		mcp.WithDescription(`Check the freshness of a memory space's ingested data.
Returns when the space was last ingested, how many ingestions have occurred,
and whether the data is considered stale based on the configured threshold.
Use this to determine if a re-ingestion is needed.`),
		mcp.WithString("space_id",
			mcp.Required(),
			mcp.Description("The space ID to check freshness for")),
	)

	s.AddTool(tool, m.memorySpaceFreshnessHandler)
}

func (m *mcpServer) memorySpaceFreshnessHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	spaceID, _ := args["space_id"].(string)
	if spaceID == "" {
		return newToolResultError("space_id is required"), nil
	}

	resp, err := m.callMDEMGGet("/v1/memory/spaces/"+spaceID+"/freshness", nil)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to check freshness: %v", err)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Space Freshness: %s\n\n", spaceID))

	if lastIngest, ok := resp["last_ingest_at"].(string); ok && lastIngest != "" {
		sb.WriteString(fmt.Sprintf("**Last Ingest:** %s\n", lastIngest))
	} else {
		sb.WriteString("**Last Ingest:** Never\n")
	}

	if ingestType, ok := resp["last_ingest_type"].(string); ok && ingestType != "" {
		sb.WriteString(fmt.Sprintf("**Ingest Type:** %s\n", ingestType))
	}

	if count, ok := resp["ingest_count"].(float64); ok {
		sb.WriteString(fmt.Sprintf("**Total Ingestions:** %d\n", int(count)))
	}

	if isStale, ok := resp["is_stale"].(bool); ok {
		if isStale {
			sb.WriteString("**Status:** STALE\n")
		} else {
			sb.WriteString("**Status:** Fresh\n")
		}
	}

	if staleHours, ok := resp["stale_hours"].(float64); ok && staleHours > 0 {
		sb.WriteString(fmt.Sprintf("**Hours Since Ingest:** %d\n", int(staleHours)))
	}

	if threshold, ok := resp["threshold_hours"].(float64); ok {
		sb.WriteString(fmt.Sprintf("**Stale Threshold:** %dh\n", int(threshold)))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// =============================================================================
// Linear CRUD Tools
// =============================================================================

func (m *mcpServer) registerLinearCreateIssueTool(s *server.MCPServer) {
	tool := mcp.NewTool("linear_create_issue",
		mcp.WithDescription(`Create a new issue in Linear. Requires title and team_id.
Returns the created issue with its ID and identifier.`),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Issue title")),
		mcp.WithString("team_id",
			mcp.Required(),
			mcp.Description("Team ID to create the issue in")),
		mcp.WithString("description",
			mcp.Description("Issue description (markdown supported)")),
		mcp.WithString("priority",
			mcp.Description("Priority: 1=urgent, 2=high, 3=medium, 4=low")),
		mcp.WithString("assignee_id",
			mcp.Description("User ID to assign the issue to")),
		mcp.WithString("project_id",
			mcp.Description("Project ID to associate the issue with")),
	)

	s.AddTool(tool, m.linearCreateIssueHandler)
}

func (m *mcpServer) linearCreateIssueHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	title, _ := args["title"].(string)
	if title == "" {
		return newToolResultError("title is required"), nil
	}
	teamID, _ := args["team_id"].(string)
	if teamID == "" {
		return newToolResultError("team_id is required"), nil
	}

	body := map[string]string{
		"title":   title,
		"team_id": teamID,
	}
	if v, ok := args["description"].(string); ok && v != "" {
		body["description"] = v
	}
	if v, ok := args["priority"].(string); ok && v != "" {
		body["priority"] = v
	}
	if v, ok := args["assignee_id"].(string); ok && v != "" {
		body["assignee_id"] = v
	}
	if v, ok := args["project_id"].(string); ok && v != "" {
		body["project_id"] = v
	}

	resp, err := m.callMDEMGWithMap("/v1/linear/issues", body)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create issue: %v", err)), nil
	}

	entity, _ := resp["entity"].(map[string]any)
	fields, _ := entity["fields"].(map[string]any)
	identifier, _ := fields["identifier"].(string)
	id, _ := entity["id"].(string)

	result := fmt.Sprintf("Issue created successfully.\nID: %s\nIdentifier: %s\nTitle: %s", id, identifier, title)
	return mcp.NewToolResultText(result), nil
}

func (m *mcpServer) registerLinearListIssuesTool(s *server.MCPServer) {
	tool := mcp.NewTool("linear_list_issues",
		mcp.WithDescription(`List issues from Linear with optional filters.
Returns issues with their ID, title, state, priority, and assignee.`),
		mcp.WithString("team",
			mcp.Description("Filter by team key (e.g., 'ENG')")),
		mcp.WithString("state",
			mcp.Description("Filter by state name (e.g., 'In Progress')")),
		mcp.WithString("assignee",
			mcp.Description("Filter by assignee name")),
		mcp.WithNumber("limit",
			mcp.Description("Max results to return (default: 20)")),
	)

	s.AddTool(tool, m.linearListIssuesHandler)
}

func (m *mcpServer) linearListIssuesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	params := make(map[string]string)
	if v, ok := args["team"].(string); ok && v != "" {
		params["team"] = v
	}
	if v, ok := args["state"].(string); ok && v != "" {
		params["state"] = v
	}
	if v, ok := args["assignee"].(string); ok && v != "" {
		params["assignee"] = v
	}

	limit := "20"
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = fmt.Sprintf("%d", int(l))
	}
	params["limit"] = limit

	resp, err := m.callMDEMGGet("/v1/linear/issues", params)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list issues: %v", err)), nil
	}

	entities, _ := resp["entities"].([]any)
	if len(entities) == 0 {
		return mcp.NewToolResultText("No issues found matching the filters."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d issues:\n\n", len(entities)))

	for i, e := range entities {
		entity, _ := e.(map[string]any)
		fields, _ := entity["fields"].(map[string]any)

		identifier, _ := fields["identifier"].(string)
		title, _ := fields["title"].(string)
		state, _ := fields["state"].(string)
		priority, _ := fields["priority"].(string)
		assignee, _ := fields["assignee_name"].(string)

		sb.WriteString(fmt.Sprintf("%d. **%s** — %s\n", i+1, identifier, title))
		sb.WriteString(fmt.Sprintf("   State: %s | Priority: %s", state, priority))
		if assignee != "" {
			sb.WriteString(fmt.Sprintf(" | Assignee: %s", assignee))
		}
		sb.WriteString("\n\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *mcpServer) registerLinearReadIssueTool(s *server.MCPServer) {
	tool := mcp.NewTool("linear_read_issue",
		mcp.WithDescription(`Read a single issue from Linear by ID.
Returns the full issue details including description, state, and comments.`),
		mcp.WithString("issue_id",
			mcp.Required(),
			mcp.Description("The Linear issue ID")),
	)

	s.AddTool(tool, m.linearReadIssueHandler)
}

func (m *mcpServer) linearReadIssueHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	issueID, _ := args["issue_id"].(string)
	if issueID == "" {
		return newToolResultError("issue_id is required"), nil
	}

	resp, err := m.callMDEMGGet("/v1/linear/issues/"+issueID, nil)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read issue: %v", err)), nil
	}

	entity, _ := resp["entity"].(map[string]any)
	fields, _ := entity["fields"].(map[string]any)

	identifier, _ := fields["identifier"].(string)
	title, _ := fields["title"].(string)
	description, _ := fields["description"].(string)
	state, _ := fields["state"].(string)
	stateType, _ := fields["state_type"].(string)
	priority, _ := fields["priority"].(string)
	teamKey, _ := fields["team_key"].(string)
	assignee, _ := fields["assignee_name"].(string)
	project, _ := fields["project_name"].(string)
	labels, _ := fields["labels"].(string)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s: %s\n\n", identifier, title))
	if description != "" {
		sb.WriteString(description)
		sb.WriteString("\n\n")
	}
	sb.WriteString(fmt.Sprintf("**State:** %s (%s)\n", state, stateType))
	sb.WriteString(fmt.Sprintf("**Priority:** %s\n", priority))
	sb.WriteString(fmt.Sprintf("**Team:** %s\n", teamKey))
	if assignee != "" {
		sb.WriteString(fmt.Sprintf("**Assignee:** %s\n", assignee))
	}
	if project != "" {
		sb.WriteString(fmt.Sprintf("**Project:** %s\n", project))
	}
	if labels != "" {
		sb.WriteString(fmt.Sprintf("**Labels:** %s\n", labels))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *mcpServer) registerLinearUpdateIssueTool(s *server.MCPServer) {
	tool := mcp.NewTool("linear_update_issue",
		mcp.WithDescription(`Update an existing issue in Linear. Provide only the fields you want to change.`),
		mcp.WithString("issue_id",
			mcp.Required(),
			mcp.Description("The Linear issue ID")),
		mcp.WithString("title",
			mcp.Description("New issue title")),
		mcp.WithString("description",
			mcp.Description("New issue description")),
		mcp.WithString("priority",
			mcp.Description("New priority: 1=urgent, 2=high, 3=medium, 4=low")),
		mcp.WithString("state_id",
			mcp.Description("New state ID")),
		mcp.WithString("assignee_id",
			mcp.Description("New assignee user ID")),
	)

	s.AddTool(tool, m.linearUpdateIssueHandler)
}

func (m *mcpServer) linearUpdateIssueHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	issueID, _ := args["issue_id"].(string)
	if issueID == "" {
		return newToolResultError("issue_id is required"), nil
	}

	body := make(map[string]string)
	for _, key := range []string{"title", "description", "priority", "state_id", "assignee_id"} {
		if v, ok := args[key].(string); ok && v != "" {
			body[key] = v
		}
	}

	if len(body) == 0 {
		return newToolResultError("at least one field to update is required"), nil
	}

	resp, err := m.callMDEMGPut("/v1/linear/issues/"+issueID, body)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to update issue: %v", err)), nil
	}

	entity, _ := resp["entity"].(map[string]any)
	fields, _ := entity["fields"].(map[string]any)
	identifier, _ := fields["identifier"].(string)
	title, _ := fields["title"].(string)

	result := fmt.Sprintf("Issue updated successfully.\nIdentifier: %s\nTitle: %s", identifier, title)
	return mcp.NewToolResultText(result), nil
}

func (m *mcpServer) registerLinearAddCommentTool(s *server.MCPServer) {
	tool := mcp.NewTool("linear_add_comment",
		mcp.WithDescription(`Add a comment to a Linear issue.`),
		mcp.WithString("issue_id",
			mcp.Required(),
			mcp.Description("The Linear issue ID to comment on")),
		mcp.WithString("body",
			mcp.Required(),
			mcp.Description("The comment body (markdown supported)")),
	)

	s.AddTool(tool, m.linearAddCommentHandler)
}

func (m *mcpServer) linearAddCommentHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	issueID, _ := args["issue_id"].(string)
	if issueID == "" {
		return newToolResultError("issue_id is required"), nil
	}
	body, _ := args["body"].(string)
	if body == "" {
		return newToolResultError("body is required"), nil
	}

	resp, err := m.callMDEMGWithMap("/v1/linear/comments", map[string]string{
		"issue_id": issueID,
		"body":     body,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to add comment: %v", err)), nil
	}

	entity, _ := resp["entity"].(map[string]any)
	commentID, _ := entity["id"].(string)

	result := fmt.Sprintf("Comment added successfully.\nComment ID: %s", commentID)
	return mcp.NewToolResultText(result), nil
}

func (m *mcpServer) registerLinearSearchTool(s *server.MCPServer) {
	tool := mcp.NewTool("linear_search",
		mcp.WithDescription(`Search for issues in Linear using fulltext search.`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query")),
		mcp.WithNumber("limit",
			mcp.Description("Max results to return (default: 20)")),
	)

	s.AddTool(tool, m.linearSearchHandler)
}

func (m *mcpServer) linearSearchHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	query, _ := args["query"].(string)
	if query == "" {
		return newToolResultError("query is required"), nil
	}

	limit := "20"
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = fmt.Sprintf("%d", int(l))
	}

	params := map[string]string{
		"query": query,
		"limit": limit,
	}

	resp, err := m.callMDEMGGet("/v1/linear/issues", params)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to search issues: %v", err)), nil
	}

	entities, _ := resp["entities"].([]any)
	if len(entities) == 0 {
		return mcp.NewToolResultText("No issues found matching the search query."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d issues for query %q:\n\n", len(entities), query))

	for i, e := range entities {
		entity, _ := e.(map[string]any)
		fields, _ := entity["fields"].(map[string]any)

		identifier, _ := fields["identifier"].(string)
		title, _ := fields["title"].(string)
		state, _ := fields["state"].(string)

		sb.WriteString(fmt.Sprintf("%d. **%s** — %s [%s]\n", i+1, identifier, title, state))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// =============================================================================
// Guardrail Tools (Phase 104)
// =============================================================================

// validate_changes - Check proposed code changes against active organizational constraints
func (m *mcpServer) registerValidateChangesTool(s *server.MCPServer) {
	tool := mcp.NewTool("validate_changes",
		mcp.WithDescription(`Validate proposed code changes against active organizational constraints.

Checks the diff against constraint nodes (must, must_not, should, should_not) in the memory graph.
Returns Pass, Warning, or Block status with specific constraint violations.

Use this before committing changes to ensure compliance with organizational rules and patterns.`),
		mcp.WithString("diff",
			mcp.Required(),
			mcp.Description("The unified diff of proposed changes")),
		mcp.WithString("files_changed",
			mcp.Required(),
			mcp.Description("Comma-separated list of changed file paths")),
		mcp.WithString("space_id",
			mcp.Description("Space ID to check constraints against (default: ide-agent)")),
	)

	s.AddTool(tool, m.validateChangesHandler)
}

func (m *mcpServer) validateChangesHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(request)

	diff, _ := args["diff"].(string)
	if diff == "" {
		return newToolResultError("diff is required"), nil
	}

	filesStr, _ := args["files_changed"].(string)
	if filesStr == "" {
		return newToolResultError("files_changed is required"), nil
	}

	spaceID, _ := args["space_id"].(string)
	if spaceID == "" {
		spaceID = defaultSpaceID
	}

	// Parse comma-separated files
	var files []string
	for _, f := range strings.Split(filesStr, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			files = append(files, f)
		}
	}

	// Call guardrail validate API
	body := map[string]any{
		"space_id":      spaceID,
		"files_changed": files,
		"diff":          diff,
	}

	resp, err := m.callMDEMG("/v1/memory/guardrail/validate", body)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Guardrail validation failed: %v", err)), nil
	}

	// Format response as markdown
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return newToolResultError("unexpected response format from guardrail API"), nil
	}

	status, _ := data["status"].(string)
	violations, _ := data["violations"].([]any)
	warnings, _ := data["warnings"].([]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Guardrail Status: %s\n\n", status))

	if len(violations) > 0 {
		sb.WriteString("### Violations (Block)\n\n")
		for i, v := range violations {
			vMap, _ := v.(map[string]any)
			desc, _ := vMap["description"].(string)
			rationale, _ := vMap["rationale"].(string)
			nodeID, _ := vMap["constraint_node_id"].(string)
			sb.WriteString(fmt.Sprintf("%d. **%s** (constraint: %s)\n   %s\n\n", i+1, desc, nodeID, rationale))
		}
	}

	if len(warnings) > 0 {
		sb.WriteString("### Warnings\n\n")
		for i, w := range warnings {
			wMap, _ := w.(map[string]any)
			desc, _ := wMap["description"].(string)
			rationale, _ := wMap["rationale"].(string)
			nodeID, _ := wMap["constraint_node_id"].(string)
			sb.WriteString(fmt.Sprintf("%d. **%s** (constraint: %s)\n   %s\n\n", i+1, desc, nodeID, rationale))
		}
	}

	if len(violations) == 0 && len(warnings) == 0 {
		sb.WriteString("No constraint violations or warnings found.\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// callMDEMG sends a POST request to the MDEMG API
func (m *mcpServer) callMDEMG(path string, body map[string]any) (map[string]any, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", m.endpoint+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if errMsg, ok := result["error"].(string); ok {
		return nil, fmt.Errorf("MDEMG error: %s", errMsg)
	}

	return result, nil
}

// callMDEMGGet sends a GET request to the MDEMG API
func (m *mcpServer) callMDEMGGet(path string, params map[string]string) (map[string]any, error) {
	req, err := http.NewRequest("GET", m.endpoint+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if params != nil {
		q := req.URL.Query()
		for k, v := range params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if errMsg, ok := result["error"].(string); ok {
		return nil, fmt.Errorf("MDEMG error: %s", errMsg)
	}

	return result, nil
}

// callMDEMGPut sends a PUT request to the MDEMG API
func (m *mcpServer) callMDEMGPut(path string, body map[string]string) (map[string]any, error) {
	bodyMap := make(map[string]any, len(body))
	for k, v := range body {
		bodyMap[k] = v
	}

	jsonBody, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("PUT", m.endpoint+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if errMsg, ok := result["error"].(string); ok {
		return nil, fmt.Errorf("MDEMG error: %s", errMsg)
	}

	return result, nil
}

// callMDEMGWithMap is a helper for POST requests with map[string]string body
func (m *mcpServer) callMDEMGWithMap(path string, body map[string]string) (map[string]any, error) {
	bodyMap := make(map[string]any, len(body))
	for k, v := range body {
		bodyMap[k] = v
	}
	return m.callMDEMG(path, bodyMap)
}

// getArgs extracts the arguments map from a CallToolRequest
// mcp-go v0.43+ changed Arguments from map[string]any to any
func getArgs(request mcp.CallToolRequest) map[string]any {
	args, _ := request.Params.Arguments.(map[string]any)
	if args == nil {
		args = make(map[string]any)
	}
	return args
}

// newToolResultError creates an error result for MCP tools
func newToolResultError(errMsg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(errMsg)},
		IsError: true,
	}
}
