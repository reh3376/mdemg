package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

var (
	socketPath     = flag.String("socket", "", "Unix socket path")
	requestCounter uint64
)

const linearAPIEndpoint = "https://api.linear.app/graphql"

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if *socketPath == "" {
		slog.Error("--socket is required")
		os.Exit(1)
	}

	// Remove stale socket
	os.Remove(*socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	// Create gRPC server
	server := grpc.NewServer()

	// Load workflow engine
	wfEngine := NewWorkflowEngine()
	if err := wfEngine.LoadFromFile("workflows.yaml"); err != nil {
		slog.Warn("failed to load workflows.yaml, workflow engine disabled", "error", err)
	}

	// Register services
	module := &LinearModule{
		startTime:      time.Now(),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		workflowEngine: wfEngine,
	}
	pb.RegisterModuleLifecycleServer(server, module)
	pb.RegisterIngestionModuleServer(server, module)
	pb.RegisterCRUDModuleServer(server, module)

	slog.Info("linear module listening", "socket", *socketPath)

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		slog.Info("shutting down")
		server.GracefulStop()
	}()

	// Serve
	if err := server.Serve(listener); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// LinearModule implements the module interfaces for Linear integration
type LinearModule struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer
	pb.UnimplementedCRUDModuleServer

	startTime      time.Time
	httpClient     *http.Client
	apiKey         string
	config         map[string]string
	workflowEngine *WorkflowEngine

	mu          sync.RWMutex
	lastSyncAt  time.Time
	syncCursors map[string]string // team -> cursor
}

// Handshake implements ModuleLifecycle
func (m *LinearModule) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	slog.Info("handshake received", "mdemg_version", req.MdemgVersion, "socket", req.SocketPath)

	m.config = req.Config

	// Get API key from environment variable specified in config
	apiKeyEnv := req.Config["api_key_env"]
	if apiKeyEnv == "" {
		apiKeyEnv = "LINEAR_API_KEY"
	}
	m.apiKey = os.Getenv(apiKeyEnv)

	if m.apiKey == "" {
		slog.Warn("API key env not set, Linear API calls will fail", "env_var", apiKeyEnv)
	}

	m.syncCursors = make(map[string]string)

	return &pb.HandshakeResponse{
		ModuleId:      "linear-module",
		ModuleVersion: "1.0.0",
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"linear://", "application/vnd.linear.issue", "application/vnd.linear.project"},
		Ready:         true,
	}, nil
}

// HealthCheck implements ModuleLifecycle
func (m *LinearModule) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	uptime := time.Since(m.startTime).String()
	requests := atomic.LoadUint64(&requestCounter)

	m.mu.RLock()
	lastSync := m.lastSyncAt.Format(time.RFC3339)
	if m.lastSyncAt.IsZero() {
		lastSync = "never"
	}
	m.mu.RUnlock()

	healthy := true
	status := "ok"
	if m.apiKey == "" {
		status = "warning: no API key configured"
	}

	return &pb.HealthCheckResponse{
		Healthy: healthy,
		Status:  status,
		Metrics: map[string]string{
			"uptime":           uptime,
			"requests_handled": fmt.Sprintf("%d", requests),
			"last_sync":        lastSync,
			"api_configured":   fmt.Sprintf("%t", m.apiKey != ""),
		},
	}, nil
}

// Shutdown implements ModuleLifecycle
func (m *LinearModule) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	slog.Info("shutdown requested", "reason", req.Reason, "timeout_ms", req.TimeoutMs)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// Matches implements IngestionModule
func (m *LinearModule) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	matches := strings.HasPrefix(req.SourceUri, "linear://")
	confidence := float32(0.0)
	reason := "not a linear:// source"

	if matches {
		confidence = 1.0
		reason = "matches linear:// URI scheme"
	}

	// Also match Linear content types
	if strings.HasPrefix(req.ContentType, "application/vnd.linear.") {
		matches = true
		confidence = 1.0
		reason = "matches Linear content type"
	}

	return &pb.MatchResponse{
		Matches:    matches,
		Confidence: confidence,
		Reason:     reason,
	}, nil
}

// Parse implements IngestionModule - parses raw Linear data into observations
func (m *LinearModule) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// Parse the content based on content type
	var obs []*pb.Observation

	switch {
	case strings.Contains(req.ContentType, "issue"):
		issue, err := m.parseIssueJSON(req.Content)
		if err != nil {
			return &pb.ParseResponse{
				Error: fmt.Sprintf("failed to parse issue: %v", err),
			}, nil
		}
		obs = append(obs, issue)

	case strings.Contains(req.ContentType, "project"):
		project, err := m.parseProjectJSON(req.Content)
		if err != nil {
			return &pb.ParseResponse{
				Error: fmt.Sprintf("failed to parse project: %v", err),
			}, nil
		}
		obs = append(obs, project)

	default:
		// Try to auto-detect
		var data map[string]interface{}
		if err := json.Unmarshal(req.Content, &data); err != nil {
			return &pb.ParseResponse{
				Error: fmt.Sprintf("failed to parse JSON: %v", err),
			}, nil
		}

		if _, ok := data["identifier"]; ok {
			// Looks like an issue
			issue, err := m.parseIssueJSON(req.Content)
			if err != nil {
				return &pb.ParseResponse{
					Error: fmt.Sprintf("failed to parse issue: %v", err),
				}, nil
			}
			obs = append(obs, issue)
		}
	}

	return &pb.ParseResponse{
		Observations: obs,
		Metadata: map[string]string{
			"parsed_at":    time.Now().Format(time.RFC3339),
			"content_type": req.ContentType,
		},
	}, nil
}

// Sync implements IngestionModule - streams issues from Linear API
func (m *LinearModule) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	atomic.AddUint64(&requestCounter, 1)

	if m.apiKey == "" {
		return fmt.Errorf("LINEAR_API_KEY not configured")
	}

	// Parse the source ID to get team/filter
	// Format: linear://issues?team=TEAM_ID or linear://projects
	resourceType := "issues"
	teamFilter := ""

	sourceId := req.SourceId
	if sourceId == "" {
		sourceId = "linear://issues" // default
	}

	if strings.HasPrefix(sourceId, "linear://") {
		path := strings.TrimPrefix(sourceId, "linear://")
		parts := strings.SplitN(path, "?", 2)
		resourceType = parts[0]
		if len(parts) > 1 {
			// Parse query params
			for _, param := range strings.Split(parts[1], "&") {
				kv := strings.SplitN(param, "=", 2)
				if len(kv) == 2 && kv[0] == "team" {
					teamFilter = kv[1]
				}
			}
		}
	}

	// Use cursor from request or from stored state
	cursor := req.Cursor
	if cursor == "" {
		m.mu.RLock()
		cursor = m.syncCursors[teamFilter]
		m.mu.RUnlock()
	}

	ctx := stream.Context()
	var totalProcessed, totalCreated int32

	switch resourceType {
	case "all":
		// Sync everything: teams, projects, then issues
		// For "all", we send all data with HasMore=true until the final batch
		slog.Info("syncing teams")
		p, c, _, err := m.syncTeamsInternal(ctx, stream, true) // hasMore=true
		if err != nil {
			return err
		}
		totalProcessed += p
		totalCreated += c

		slog.Info("syncing projects")
		p, c, _, err = m.syncProjectsInternal(ctx, stream, "", true) // hasMore=true
		if err != nil {
			return err
		}
		totalProcessed += p
		totalCreated += c

		slog.Info("syncing issues")
		p, c, newCursor, err := m.syncIssues(ctx, stream, teamFilter, cursor)
		if err != nil {
			return err
		}
		totalProcessed += p
		totalCreated += c
		cursor = newCursor

	case "teams":
		processed, created, newCursor, err := m.syncTeams(ctx, stream)
		if err != nil {
			return err
		}
		totalProcessed = processed
		totalCreated = created
		cursor = newCursor

	case "issues":
		processed, created, newCursor, err := m.syncIssues(ctx, stream, teamFilter, cursor)
		if err != nil {
			return err
		}
		totalProcessed = processed
		totalCreated = created
		cursor = newCursor

	case "projects":
		processed, created, newCursor, err := m.syncProjects(ctx, stream, cursor)
		if err != nil {
			return err
		}
		totalProcessed = processed
		totalCreated = created
		cursor = newCursor

	default:
		return fmt.Errorf("unknown resource type: %s (supported: all, teams, projects, issues)", resourceType)
	}

	// Store the new cursor
	m.mu.Lock()
	m.syncCursors[teamFilter] = cursor
	m.lastSyncAt = time.Now()
	m.mu.Unlock()

	// Send final response
	return stream.Send(&pb.SyncResponse{
		Cursor:  cursor,
		HasMore: false,
		Stats: &pb.SyncStats{
			ItemsProcessed: totalProcessed,
			ItemsCreated:   totalCreated,
		},
	})
}

// syncIssues fetches issues from Linear and streams them as observations
func (m *LinearModule) syncIssues(ctx context.Context, stream pb.IngestionModule_SyncServer, teamFilter, cursor string) (int32, int32, string, error) {
	var processed, created int32
	hasMore := true

	for hasMore {
		// Build GraphQL query
		query := m.buildIssuesQuery(teamFilter, cursor, 50)

		// Execute query with per-batch timeout
		batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := m.executeGraphQL(batchCtx, query)
		cancel()
		if err != nil {
			return processed, created, cursor, fmt.Errorf("GraphQL error: %w", err)
		}

		// Parse response
		issues, nextCursor, more, err := m.parseIssuesResponse(result)
		if err != nil {
			return processed, created, cursor, fmt.Errorf("parse error: %w", err)
		}

		// Convert to observations and edges, then stream
		var obs []*pb.Observation
		var edges []*pb.Edge
		for _, issue := range issues {
			obs = append(obs, m.issueToObservation(issue))
			edges = append(edges, m.issueToEdges(issue)...)
			processed++
			created++
		}

		if len(obs) > 0 {
			if err := stream.Send(&pb.SyncResponse{
				Observations: obs,
				Edges:        edges,
				Cursor:       nextCursor,
				HasMore:      more,
				Stats: &pb.SyncStats{
					ItemsProcessed: processed,
					ItemsCreated:   created,
				},
			}); err != nil {
				return processed, created, cursor, err
			}
		}

		cursor = nextCursor
		hasMore = more

		// Rate limiting - be nice to Linear's API
		time.Sleep(100 * time.Millisecond)
	}

	return processed, created, cursor, nil
}

// syncProjects fetches projects from Linear
func (m *LinearModule) syncProjects(ctx context.Context, stream pb.IngestionModule_SyncServer, cursor string) (int32, int32, string, error) {
	return m.syncProjectsInternal(ctx, stream, cursor, false)
}

// syncProjectsInternal fetches projects from Linear with control over hasMore flag
func (m *LinearModule) syncProjectsInternal(ctx context.Context, stream pb.IngestionModule_SyncServer, cursor string, forceHasMore bool) (int32, int32, string, error) {
	var processed, created int32
	hasMorePages := true

	for hasMorePages {
		query := m.buildProjectsQuery(cursor, 50)

		batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := m.executeGraphQL(batchCtx, query)
		cancel()
		if err != nil {
			return processed, created, cursor, fmt.Errorf("GraphQL error: %w", err)
		}

		projects, nextCursor, morePages, err := m.parseProjectsResponse(result)
		if err != nil {
			return processed, created, cursor, fmt.Errorf("parse error: %w", err)
		}

		var obs []*pb.Observation
		for _, project := range projects {
			obs = append(obs, m.projectToObservation(project))
			processed++
			created++
		}

		// Determine if we should report more coming
		finalHasMore := morePages || forceHasMore

		if len(obs) > 0 {
			if err := stream.Send(&pb.SyncResponse{
				Observations: obs,
				Cursor:       nextCursor,
				HasMore:      finalHasMore,
				Stats: &pb.SyncStats{
					ItemsProcessed: processed,
					ItemsCreated:   created,
				},
			}); err != nil {
				return processed, created, cursor, err
			}
		}

		cursor = nextCursor
		hasMorePages = morePages

		// Rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	return processed, created, cursor, nil
}

// syncTeams fetches all teams from Linear
func (m *LinearModule) syncTeams(ctx context.Context, stream pb.IngestionModule_SyncServer) (int32, int32, string, error) {
	return m.syncTeamsInternal(ctx, stream, false)
}

// syncTeamsInternal fetches all teams from Linear with control over hasMore flag
func (m *LinearModule) syncTeamsInternal(ctx context.Context, stream pb.IngestionModule_SyncServer, hasMore bool) (int32, int32, string, error) {
	var processed, created int32

	query := map[string]interface{}{
		"query": `
			query {
				teams {
					nodes {
						id
						key
						name
						description
						color
						icon
						private
						timezone
						issueCount
						createdAt
						updatedAt
					}
				}
			}
		`,
	}

	batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	result, err := m.executeGraphQL(batchCtx, query)
	cancel()
	if err != nil {
		return processed, created, "", fmt.Errorf("GraphQL error: %w", err)
	}

	teams, err := m.parseTeamsResponse(result)
	if err != nil {
		return processed, created, "", fmt.Errorf("parse error: %w", err)
	}

	var obs []*pb.Observation
	for _, team := range teams {
		obs = append(obs, m.teamToObservation(team))
		processed++
		created++
	}

	if len(obs) > 0 {
		if err := stream.Send(&pb.SyncResponse{
			Observations: obs,
			HasMore:      hasMore,
			Stats: &pb.SyncStats{
				ItemsProcessed: processed,
				ItemsCreated:   created,
			},
		}); err != nil {
			return processed, created, "", err
		}
	}

	return processed, created, "", nil
}

// GraphQL query builders

func (m *LinearModule) buildIssuesQuery(teamFilter, cursor string, limit int) map[string]interface{} {
	afterClause := ""
	if cursor != "" {
		afterClause = fmt.Sprintf(`, after: "%s"`, cursor)
	}

	filterClause := ""
	if teamFilter != "" {
		filterClause = fmt.Sprintf(`, filter: { team: { key: { eq: "%s" } } }`, teamFilter)
	}

	return map[string]interface{}{
		"query": fmt.Sprintf(`
			query {
				issues(first: %d%s%s) {
					pageInfo {
						hasNextPage
						endCursor
					}
					nodes {
						id
						identifier
						title
						description
						priority
						state {
							name
							type
						}
						team {
							key
							name
						}
						project {
							id
							name
						}
						assignee {
							name
							email
						}
						labels {
							nodes {
								name
							}
						}
						createdAt
						updatedAt
						completedAt
						parent {
							id
							identifier
							title
						}
						children {
							nodes {
								id
								identifier
								title
							}
						}
						relations {
							nodes {
								type
								relatedIssue {
									id
									identifier
								}
							}
						}
					}
				}
			}
		`, limit, afterClause, filterClause),
	}
}

func (m *LinearModule) buildProjectsQuery(cursor string, limit int) map[string]interface{} {
	afterClause := ""
	if cursor != "" {
		afterClause = fmt.Sprintf(`, after: "%s"`, cursor)
	}

	return map[string]interface{}{
		"query": fmt.Sprintf(`
			query {
				projects(first: %d%s) {
					pageInfo {
						hasNextPage
						endCursor
					}
					nodes {
						id
						name
						description
						state
						progress
						targetDate
						startDate
						teams {
							nodes {
								key
								name
							}
						}
						lead {
							name
							email
						}
						createdAt
						updatedAt
					}
				}
			}
		`, limit, afterClause),
	}
}

// executeGraphQL sends a GraphQL query to Linear
func (m *LinearModule) executeGraphQL(ctx context.Context, query map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", linearAPIEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	// Linear API keys (lin_api_*) go directly, OAuth tokens (lin_oauth_*) need Bearer prefix
	authHeader := m.apiKey
	if strings.HasPrefix(authHeader, "lin_oauth_") {
		authHeader = "Bearer " + authHeader
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Linear API returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Check for GraphQL errors
	if errors, ok := result["errors"].([]interface{}); ok && len(errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", errors)
	}

	return result, nil
}

// Response parsers

type linearIssue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	Priority    int
	State       struct {
		Name string
		Type string
	}
	Team struct {
		Key  string
		Name string
	}
	Project *struct {
		ID   string
		Name string
	}
	Assignee *struct {
		Name  string
		Email string
	}
	Labels struct {
		Nodes []struct {
			Name string
		}
	}
	CreatedAt   string
	UpdatedAt   string
	CompletedAt *string
	Parent      *struct {
		ID         string
		Identifier string
		Title      string
	}
	Children []struct {
		ID         string
		Identifier string
		Title      string
	}
	Relations []struct {
		Type         string
		RelatedIssue struct {
			ID         string
			Identifier string
		}
	}
}

type linearProject struct {
	ID          string
	Name        string
	Description string
	State       string
	Progress    float64
	TargetDate  *string
	StartDate   *string
	Teams       struct {
		Nodes []struct {
			Key  string
			Name string
		}
	}
	Lead *struct {
		Name  string
		Email string
	}
	CreatedAt string
	UpdatedAt string
}

type linearTeam struct {
	ID          string
	Key         string
	Name        string
	Description string
	Color       string
	Icon        string
	Private     bool
	Timezone    string
	IssueCount  int
	CreatedAt   string
	UpdatedAt   string
}

func (m *LinearModule) parseTeamsResponse(result map[string]interface{}) ([]linearTeam, error) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no data in response")
	}

	teamsData, ok := data["teams"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no teams in response")
	}

	nodes, ok := teamsData["nodes"].([]interface{})
	if !ok {
		return nil, nil
	}

	var teams []linearTeam
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		team := linearTeam{
			ID:          getString(nodeMap, "id"),
			Key:         getString(nodeMap, "key"),
			Name:        getString(nodeMap, "name"),
			Description: getString(nodeMap, "description"),
			Color:       getString(nodeMap, "color"),
			Icon:        getString(nodeMap, "icon"),
			Private:     getBool(nodeMap, "private"),
			Timezone:    getString(nodeMap, "timezone"),
			IssueCount:  getInt(nodeMap, "issueCount"),
			CreatedAt:   getString(nodeMap, "createdAt"),
			UpdatedAt:   getString(nodeMap, "updatedAt"),
		}
		teams = append(teams, team)
	}

	return teams, nil
}

func (m *LinearModule) teamToObservation(team linearTeam) *pb.Observation {
	var content strings.Builder
	content.WriteString(fmt.Sprintf("# Team: %s (%s)\n\n", team.Name, team.Key))

	if team.Description != "" {
		content.WriteString(team.Description)
		content.WriteString("\n\n")
	}

	content.WriteString(fmt.Sprintf("**Issue Count:** %d\n", team.IssueCount))
	if team.Timezone != "" {
		content.WriteString(fmt.Sprintf("**Timezone:** %s\n", team.Timezone))
	}
	if team.Private {
		content.WriteString("**Private:** Yes\n")
	}

	tags := []string{"linear", "team", team.Key}

	metadata := map[string]string{
		"linear_id":   team.ID,
		"team_key":    team.Key,
		"issue_count": fmt.Sprintf("%d", team.IssueCount),
		"private":     fmt.Sprintf("%t", team.Private),
		"created_at":  team.CreatedAt,
		"updated_at":  team.UpdatedAt,
	}
	if team.Color != "" {
		metadata["color"] = team.Color
	}

	return &pb.Observation{
		NodeId:      fmt.Sprintf("linear-team-%s", team.ID),
		Path:        fmt.Sprintf("linear://teams/%s", team.Key),
		Name:        team.Name,
		Content:     content.String(),
		ContentType: "application/vnd.linear.team",
		Tags:        tags,
		Timestamp:   team.UpdatedAt,
		Source:      "linear-module",
		Metadata:    metadata,
	}
}

func (m *LinearModule) parseIssuesResponse(result map[string]interface{}) ([]linearIssue, string, bool, error) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, "", false, fmt.Errorf("no data in response")
	}

	issuesData, ok := data["issues"].(map[string]interface{})
	if !ok {
		return nil, "", false, fmt.Errorf("no issues in response")
	}

	pageInfo, _ := issuesData["pageInfo"].(map[string]interface{})
	hasNextPage, _ := pageInfo["hasNextPage"].(bool)
	endCursor, _ := pageInfo["endCursor"].(string)

	nodes, ok := issuesData["nodes"].([]interface{})
	if !ok {
		return nil, endCursor, false, nil
	}

	var issues []linearIssue
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		issue := linearIssue{
			ID:          getString(nodeMap, "id"),
			Identifier:  getString(nodeMap, "identifier"),
			Title:       getString(nodeMap, "title"),
			Description: getString(nodeMap, "description"),
			Priority:    getInt(nodeMap, "priority"),
			CreatedAt:   getString(nodeMap, "createdAt"),
			UpdatedAt:   getString(nodeMap, "updatedAt"),
		}

		if state, ok := nodeMap["state"].(map[string]interface{}); ok {
			issue.State.Name = getString(state, "name")
			issue.State.Type = getString(state, "type")
		}

		if team, ok := nodeMap["team"].(map[string]interface{}); ok {
			issue.Team.Key = getString(team, "key")
			issue.Team.Name = getString(team, "name")
		}

		if project, ok := nodeMap["project"].(map[string]interface{}); ok && project != nil {
			issue.Project = &struct {
				ID   string
				Name string
			}{
				ID:   getString(project, "id"),
				Name: getString(project, "name"),
			}
		}

		if assignee, ok := nodeMap["assignee"].(map[string]interface{}); ok && assignee != nil {
			issue.Assignee = &struct {
				Name  string
				Email string
			}{
				Name:  getString(assignee, "name"),
				Email: getString(assignee, "email"),
			}
		}

		if labels, ok := nodeMap["labels"].(map[string]interface{}); ok {
			if labelNodes, ok := labels["nodes"].([]interface{}); ok {
				for _, ln := range labelNodes {
					if lnMap, ok := ln.(map[string]interface{}); ok {
						issue.Labels.Nodes = append(issue.Labels.Nodes, struct{ Name string }{
							Name: getString(lnMap, "name"),
						})
					}
				}
			}
		}

		if completedAt := getString(nodeMap, "completedAt"); completedAt != "" {
			issue.CompletedAt = &completedAt
		}

		// Parse relationship fields
		if parent, ok := nodeMap["parent"].(map[string]interface{}); ok && parent != nil {
			issue.Parent = &struct {
				ID         string
				Identifier string
				Title      string
			}{
				ID:         getString(parent, "id"),
				Identifier: getString(parent, "identifier"),
				Title:      getString(parent, "title"),
			}
		}

		if children, ok := nodeMap["children"].(map[string]interface{}); ok {
			if childNodes, ok := children["nodes"].([]interface{}); ok {
				for _, cn := range childNodes {
					if cnMap, ok := cn.(map[string]interface{}); ok {
						issue.Children = append(issue.Children, struct {
							ID         string
							Identifier string
							Title      string
						}{
							ID:         getString(cnMap, "id"),
							Identifier: getString(cnMap, "identifier"),
							Title:      getString(cnMap, "title"),
						})
					}
				}
			}
		}

		if relations, ok := nodeMap["relations"].(map[string]interface{}); ok {
			if relNodes, ok := relations["nodes"].([]interface{}); ok {
				for _, rn := range relNodes {
					if rnMap, ok := rn.(map[string]interface{}); ok {
						rel := struct {
							Type         string
							RelatedIssue struct {
								ID         string
								Identifier string
							}
						}{
							Type: getString(rnMap, "type"),
						}
						if ri, ok := rnMap["relatedIssue"].(map[string]interface{}); ok {
							rel.RelatedIssue.ID = getString(ri, "id")
							rel.RelatedIssue.Identifier = getString(ri, "identifier")
						}
						issue.Relations = append(issue.Relations, rel)
					}
				}
			}
		}

		issues = append(issues, issue)
	}

	return issues, endCursor, hasNextPage, nil
}

func (m *LinearModule) parseProjectsResponse(result map[string]interface{}) ([]linearProject, string, bool, error) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, "", false, fmt.Errorf("no data in response")
	}

	projectsData, ok := data["projects"].(map[string]interface{})
	if !ok {
		return nil, "", false, fmt.Errorf("no projects in response")
	}

	pageInfo, _ := projectsData["pageInfo"].(map[string]interface{})
	hasNextPage, _ := pageInfo["hasNextPage"].(bool)
	endCursor, _ := pageInfo["endCursor"].(string)

	nodes, ok := projectsData["nodes"].([]interface{})
	if !ok {
		return nil, endCursor, false, nil
	}

	var projects []linearProject
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		project := linearProject{
			ID:          getString(nodeMap, "id"),
			Name:        getString(nodeMap, "name"),
			Description: getString(nodeMap, "description"),
			State:       getString(nodeMap, "state"),
			Progress:    getFloat(nodeMap, "progress"),
			CreatedAt:   getString(nodeMap, "createdAt"),
			UpdatedAt:   getString(nodeMap, "updatedAt"),
		}

		if targetDate := getString(nodeMap, "targetDate"); targetDate != "" {
			project.TargetDate = &targetDate
		}
		if startDate := getString(nodeMap, "startDate"); startDate != "" {
			project.StartDate = &startDate
		}

		if teams, ok := nodeMap["teams"].(map[string]interface{}); ok {
			if teamNodes, ok := teams["nodes"].([]interface{}); ok {
				for _, tn := range teamNodes {
					if tnMap, ok := tn.(map[string]interface{}); ok {
						project.Teams.Nodes = append(project.Teams.Nodes, struct {
							Key  string
							Name string
						}{
							Key:  getString(tnMap, "key"),
							Name: getString(tnMap, "name"),
						})
					}
				}
			}
		}

		if lead, ok := nodeMap["lead"].(map[string]interface{}); ok && lead != nil {
			project.Lead = &struct {
				Name  string
				Email string
			}{
				Name:  getString(lead, "name"),
				Email: getString(lead, "email"),
			}
		}

		projects = append(projects, project)
	}

	return projects, endCursor, hasNextPage, nil
}

// Conversion to observations

func (m *LinearModule) issueToObservation(issue linearIssue) *pb.Observation {
	// Build content as structured text
	var content strings.Builder
	content.WriteString(fmt.Sprintf("# %s: %s\n\n", issue.Identifier, issue.Title))

	if issue.Description != "" {
		content.WriteString(issue.Description)
		content.WriteString("\n\n")
	}

	content.WriteString(fmt.Sprintf("**State:** %s (%s)\n", issue.State.Name, issue.State.Type))
	content.WriteString(fmt.Sprintf("**Priority:** %d\n", issue.Priority))
	content.WriteString(fmt.Sprintf("**Team:** %s (%s)\n", issue.Team.Name, issue.Team.Key))

	if issue.Project != nil {
		content.WriteString(fmt.Sprintf("**Project:** %s\n", issue.Project.Name))
	}
	if issue.Assignee != nil {
		content.WriteString(fmt.Sprintf("**Assignee:** %s\n", issue.Assignee.Name))
	}

	// Build tags
	tags := []string{"linear", "issue", issue.Team.Key, issue.State.Type}
	for _, label := range issue.Labels.Nodes {
		tags = append(tags, label.Name)
	}

	// Build metadata
	metadata := map[string]string{
		"linear_id":     issue.ID,
		"identifier":    issue.Identifier,
		"team_key":      issue.Team.Key,
		"state":         issue.State.Name,
		"state_type":    issue.State.Type,
		"priority":      fmt.Sprintf("%d", issue.Priority),
		"created_at":    issue.CreatedAt,
		"updated_at":    issue.UpdatedAt,
	}
	if issue.Project != nil {
		metadata["project_id"] = issue.Project.ID
		metadata["project_name"] = issue.Project.Name
	}
	if issue.Assignee != nil {
		metadata["assignee"] = issue.Assignee.Name
	}
	if issue.CompletedAt != nil {
		metadata["completed_at"] = *issue.CompletedAt
	}

	return &pb.Observation{
		NodeId:      fmt.Sprintf("linear-issue-%s", issue.ID),
		Path:        fmt.Sprintf("linear://issues/%s", issue.Identifier),
		Name:        issue.Title,
		Content:     content.String(),
		ContentType: "application/vnd.linear.issue",
		Tags:        tags,
		Timestamp:   issue.UpdatedAt,
		Source:      "linear-module",
		Metadata:    metadata,
	}
}

// issueToEdges builds graph edges from issue relationships (parent, children, relations).
func (m *LinearModule) issueToEdges(issue linearIssue) []*pb.Edge {
	var edges []*pb.Edge
	nodeID := fmt.Sprintf("linear-issue-%s", issue.ID)

	if issue.Parent != nil {
		edges = append(edges, &pb.Edge{
			FromNodeId: nodeID,
			ToNodeId:   fmt.Sprintf("linear-issue-%s", issue.Parent.ID),
			RelType:    "PARENT_OF",
			Weight:     0.9,
			Properties: map[string]string{
				"source":    "linear",
				"parent_id": issue.Parent.Identifier,
			},
		})
	}

	for _, child := range issue.Children {
		edges = append(edges, &pb.Edge{
			FromNodeId: fmt.Sprintf("linear-issue-%s", child.ID),
			ToNodeId:   nodeID,
			RelType:    "PARENT_OF",
			Weight:     0.9,
			Properties: map[string]string{
				"source":   "linear",
				"child_id": child.Identifier,
			},
		})
	}

	for _, rel := range issue.Relations {
		edges = append(edges, &pb.Edge{
			FromNodeId: nodeID,
			ToNodeId:   fmt.Sprintf("linear-issue-%s", rel.RelatedIssue.ID),
			RelType:    mapLinearRelationType(rel.Type),
			Weight:     0.7,
			Properties: map[string]string{
				"source":      "linear",
				"linear_type": rel.Type,
			},
		})
	}

	return edges
}

// mapLinearRelationType converts Linear relation types to MDEMG edge types.
func mapLinearRelationType(linearType string) string {
	switch linearType {
	case "blocks":
		return "BLOCKS"
	case "is-blocked-by":
		return "BLOCKED_BY"
	case "related":
		return "RELATED_TO"
	case "duplicate":
		return "DUPLICATE_OF"
	default:
		return "RELATED_TO"
	}
}

func (m *LinearModule) projectToObservation(project linearProject) *pb.Observation {
	var content strings.Builder
	content.WriteString(fmt.Sprintf("# Project: %s\n\n", project.Name))

	if project.Description != "" {
		content.WriteString(project.Description)
		content.WriteString("\n\n")
	}

	content.WriteString(fmt.Sprintf("**State:** %s\n", project.State))
	content.WriteString(fmt.Sprintf("**Progress:** %.0f%%\n", project.Progress*100))

	if project.StartDate != nil {
		content.WriteString(fmt.Sprintf("**Start Date:** %s\n", *project.StartDate))
	}
	if project.TargetDate != nil {
		content.WriteString(fmt.Sprintf("**Target Date:** %s\n", *project.TargetDate))
	}
	if project.Lead != nil {
		content.WriteString(fmt.Sprintf("**Lead:** %s\n", project.Lead.Name))
	}

	// Teams
	if len(project.Teams.Nodes) > 0 {
		content.WriteString("**Teams:** ")
		var teamNames []string
		for _, t := range project.Teams.Nodes {
			teamNames = append(teamNames, t.Name)
		}
		content.WriteString(strings.Join(teamNames, ", "))
		content.WriteString("\n")
	}

	// Build tags
	tags := []string{"linear", "project", project.State}
	for _, team := range project.Teams.Nodes {
		tags = append(tags, team.Key)
	}

	metadata := map[string]string{
		"linear_id":  project.ID,
		"state":      project.State,
		"progress":   fmt.Sprintf("%.2f", project.Progress),
		"created_at": project.CreatedAt,
		"updated_at": project.UpdatedAt,
	}
	if project.Lead != nil {
		metadata["lead"] = project.Lead.Name
	}

	return &pb.Observation{
		NodeId:      fmt.Sprintf("linear-project-%s", project.ID),
		Path:        fmt.Sprintf("linear://projects/%s", project.ID),
		Name:        project.Name,
		Content:     content.String(),
		ContentType: "application/vnd.linear.project",
		Tags:        tags,
		Timestamp:   project.UpdatedAt,
		Source:      "linear-module",
		Metadata:    metadata,
	}
}

// =============================================================================
// CRUDModule Implementation
// =============================================================================

// Create implements CRUDModule — creates an entity in Linear.
func (m *LinearModule) Create(ctx context.Context, req *pb.CRUDCreateRequest) (*pb.CRUDCreateResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	if m.apiKey == "" {
		return &pb.CRUDCreateResponse{Success: false, Error: "LINEAR_API_KEY not configured"}, nil
	}

	var query map[string]interface{}
	var err error
	var entityType string

	switch req.EntityType {
	case "issue":
		entityType = "issue"
		query, err = buildIssueCreateMutation(req.Fields)
	case "project":
		entityType = "project"
		query, err = buildProjectCreateMutation(req.Fields)
	case "comment":
		entityType = "comment"
		query, err = buildCommentCreateMutation(req.Fields)
	default:
		return &pb.CRUDCreateResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown entity type: %s (supported: issue, project, comment)", req.EntityType),
		}, nil
	}

	if err != nil {
		return &pb.CRUDCreateResponse{Success: false, Error: err.Error()}, nil
	}

	result, err := m.executeGraphQL(ctx, query)
	if err != nil {
		return &pb.CRUDCreateResponse{Success: false, Error: fmt.Sprintf("GraphQL error: %v", err)}, nil
	}

	entity, err := m.extractMutationResult(result, entityType, "Create")
	if err != nil {
		return &pb.CRUDCreateResponse{Success: false, Error: err.Error()}, nil
	}

	// Trigger workflows asynchronously
	if m.workflowEngine != nil {
		go m.workflowEngine.EvaluateEvent("on-create", req.EntityType, entity.Fields, nil, m)
	}

	return &pb.CRUDCreateResponse{Entity: entity, Success: true}, nil
}

// Read implements CRUDModule — reads a single entity from Linear.
func (m *LinearModule) Read(ctx context.Context, req *pb.CRUDReadRequest) (*pb.CRUDReadResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	if m.apiKey == "" {
		return &pb.CRUDReadResponse{Success: false, Error: "LINEAR_API_KEY not configured"}, nil
	}

	var query map[string]interface{}
	var err error

	switch req.EntityType {
	case "issue":
		query, err = buildIssueReadQuery(req.Id)
	case "project":
		query, err = buildProjectReadQuery(req.Id)
	default:
		return &pb.CRUDReadResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown entity type for read: %s (supported: issue, project)", req.EntityType),
		}, nil
	}

	if err != nil {
		return &pb.CRUDReadResponse{Success: false, Error: err.Error()}, nil
	}

	result, err := m.executeGraphQL(ctx, query)
	if err != nil {
		return &pb.CRUDReadResponse{Success: false, Error: fmt.Sprintf("GraphQL error: %v", err)}, nil
	}

	entity, err := m.extractReadResult(result, req.EntityType)
	if err != nil {
		return &pb.CRUDReadResponse{Success: false, Error: err.Error()}, nil
	}

	return &pb.CRUDReadResponse{Entity: entity, Success: true}, nil
}

// Update implements CRUDModule — updates an entity in Linear.
func (m *LinearModule) Update(ctx context.Context, req *pb.CRUDUpdateRequest) (*pb.CRUDUpdateResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	if m.apiKey == "" {
		return &pb.CRUDUpdateResponse{Success: false, Error: "LINEAR_API_KEY not configured"}, nil
	}

	// Read previous state for workflow changed_to conditions
	var previousFields map[string]string
	if m.workflowEngine != nil && m.workflowEngine.HasChangedToConditions(req.EntityType) {
		prevResp, _ := m.Read(ctx, &pb.CRUDReadRequest{EntityType: req.EntityType, Id: req.Id})
		if prevResp != nil && prevResp.Entity != nil {
			previousFields = prevResp.Entity.Fields
		}
	}

	var query map[string]interface{}
	var err error
	var entityType string

	switch req.EntityType {
	case "issue":
		entityType = "issue"
		query, err = buildIssueUpdateMutation(req.Id, req.Fields)
	case "project":
		entityType = "project"
		query, err = buildProjectUpdateMutation(req.Id, req.Fields)
	default:
		return &pb.CRUDUpdateResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown entity type for update: %s (supported: issue, project)", req.EntityType),
		}, nil
	}

	if err != nil {
		return &pb.CRUDUpdateResponse{Success: false, Error: err.Error()}, nil
	}

	result, err := m.executeGraphQL(ctx, query)
	if err != nil {
		return &pb.CRUDUpdateResponse{Success: false, Error: fmt.Sprintf("GraphQL error: %v", err)}, nil
	}

	entity, err := m.extractMutationResult(result, entityType, "Update")
	if err != nil {
		return &pb.CRUDUpdateResponse{Success: false, Error: err.Error()}, nil
	}

	// Trigger workflows asynchronously
	if m.workflowEngine != nil {
		go m.workflowEngine.EvaluateEvent("on-update", req.EntityType, entity.Fields, previousFields, m)
	}

	return &pb.CRUDUpdateResponse{Entity: entity, Success: true}, nil
}

// Delete implements CRUDModule — archives/deletes an entity in Linear.
func (m *LinearModule) Delete(ctx context.Context, req *pb.CRUDDeleteRequest) (*pb.CRUDDeleteResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	if m.apiKey == "" {
		return &pb.CRUDDeleteResponse{Success: false, Error: "LINEAR_API_KEY not configured"}, nil
	}

	var query map[string]interface{}
	var err error

	switch req.EntityType {
	case "issue":
		query, err = buildIssueDeleteMutation(req.Id)
	default:
		return &pb.CRUDDeleteResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown entity type for delete: %s (supported: issue)", req.EntityType),
		}, nil
	}

	if err != nil {
		return &pb.CRUDDeleteResponse{Success: false, Error: err.Error()}, nil
	}

	result, err := m.executeGraphQL(ctx, query)
	if err != nil {
		return &pb.CRUDDeleteResponse{Success: false, Error: fmt.Sprintf("GraphQL error: %v", err)}, nil
	}

	// Check success field
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return &pb.CRUDDeleteResponse{Success: false, Error: "no data in response"}, nil
	}

	// Look for issueArchive or similar mutation result
	for _, v := range data {
		if mutResult, ok := v.(map[string]interface{}); ok {
			if success, ok := mutResult["success"].(bool); ok && success {
				// Trigger workflows asynchronously
				if m.workflowEngine != nil {
					go m.workflowEngine.EvaluateEvent("on-delete", req.EntityType, map[string]string{"id": req.Id}, nil, m)
				}
				return &pb.CRUDDeleteResponse{Success: true}, nil
			}
		}
	}

	return &pb.CRUDDeleteResponse{Success: false, Error: "delete operation did not return success"}, nil
}

// List implements CRUDModule — lists entities with optional filtering.
func (m *LinearModule) List(ctx context.Context, req *pb.CRUDListRequest) (*pb.CRUDListResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	if m.apiKey == "" {
		return &pb.CRUDListResponse{Success: false, Error: "LINEAR_API_KEY not configured"}, nil
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}

	switch req.EntityType {
	case "issue":
		query := buildIssueListQuery(req.Filters, limit, req.Cursor)
		result, err := m.executeGraphQL(ctx, query)
		if err != nil {
			return &pb.CRUDListResponse{Success: false, Error: fmt.Sprintf("GraphQL error: %v", err)}, nil
		}
		return m.extractIssueListResult(result)

	case "project":
		query := buildProjectListQuery(limit, req.Cursor)
		result, err := m.executeGraphQL(ctx, query)
		if err != nil {
			return &pb.CRUDListResponse{Success: false, Error: fmt.Sprintf("GraphQL error: %v", err)}, nil
		}
		return m.extractProjectListResult(result)

	default:
		return &pb.CRUDListResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown entity type for list: %s (supported: issue, project)", req.EntityType),
		}, nil
	}
}

// extractMutationResult extracts the entity from a create/update mutation response.
func (m *LinearModule) extractMutationResult(result map[string]interface{}, entityType, operation string) (*pb.CRUDEntity, error) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no data in response")
	}

	// Find the mutation result (e.g., issueCreate, issueUpdate, projectCreate, commentCreate)
	var mutResult map[string]interface{}
	for _, v := range data {
		if mr, ok := v.(map[string]interface{}); ok {
			mutResult = mr
			break
		}
	}

	if mutResult == nil {
		return nil, fmt.Errorf("no mutation result in response")
	}

	// Check success
	if success, ok := mutResult["success"].(bool); ok && !success {
		return nil, fmt.Errorf("%s operation failed", operation)
	}

	// Extract the entity node
	var entityNode map[string]interface{}
	for key, v := range mutResult {
		if key == "success" {
			continue
		}
		if node, ok := v.(map[string]interface{}); ok {
			entityNode = node
			break
		}
	}

	if entityNode == nil {
		return nil, fmt.Errorf("no entity in mutation response")
	}

	// Parse fields based on entity type
	var fields map[string]string
	switch entityType {
	case "issue":
		fields = parseIssueFields(entityNode)
	case "project":
		fields = parseProjectFields(entityNode)
	case "comment":
		fields = parseCommentFields(entityNode)
	}

	return &pb.CRUDEntity{
		Id:         fields["id"],
		EntityType: entityType,
		Fields:     fields,
		CreatedAt:  fields["created_at"],
		UpdatedAt:  fields["updated_at"],
	}, nil
}

// extractReadResult extracts the entity from a read query response.
func (m *LinearModule) extractReadResult(result map[string]interface{}, entityType string) (*pb.CRUDEntity, error) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no data in response")
	}

	// Find the entity node (e.g., data.issue or data.project)
	entityNode, ok := data[entityType].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("entity not found")
	}

	var fields map[string]string
	switch entityType {
	case "issue":
		fields = parseIssueFields(entityNode)
	case "project":
		fields = parseProjectFields(entityNode)
	}

	return &pb.CRUDEntity{
		Id:         fields["id"],
		EntityType: entityType,
		Fields:     fields,
		CreatedAt:  fields["created_at"],
		UpdatedAt:  fields["updated_at"],
	}, nil
}

// extractIssueListResult extracts a list of issues from a GraphQL response.
func (m *LinearModule) extractIssueListResult(result map[string]interface{}) (*pb.CRUDListResponse, error) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return &pb.CRUDListResponse{Success: false, Error: "no data in response"}, nil
	}

	issuesData, ok := data["issues"].(map[string]interface{})
	if !ok {
		return &pb.CRUDListResponse{Success: false, Error: "no issues in response"}, nil
	}

	pageInfo, _ := issuesData["pageInfo"].(map[string]interface{})
	endCursor, _ := pageInfo["endCursor"].(string)

	nodes, ok := issuesData["nodes"].([]interface{})
	if !ok {
		return &pb.CRUDListResponse{Success: true, Entities: nil}, nil
	}

	var entities []*pb.CRUDEntity
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		fields := parseIssueFields(nodeMap)
		entities = append(entities, &pb.CRUDEntity{
			Id:         fields["id"],
			EntityType: "issue",
			Fields:     fields,
			CreatedAt:  fields["created_at"],
			UpdatedAt:  fields["updated_at"],
		})
	}

	return &pb.CRUDListResponse{
		Entities:   entities,
		NextCursor: endCursor,
		TotalCount: int32(len(entities)),
		Success:    true,
	}, nil
}

// extractProjectListResult extracts a list of projects from a GraphQL response.
func (m *LinearModule) extractProjectListResult(result map[string]interface{}) (*pb.CRUDListResponse, error) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return &pb.CRUDListResponse{Success: false, Error: "no data in response"}, nil
	}

	projectsData, ok := data["projects"].(map[string]interface{})
	if !ok {
		return &pb.CRUDListResponse{Success: false, Error: "no projects in response"}, nil
	}

	pageInfo, _ := projectsData["pageInfo"].(map[string]interface{})
	endCursor, _ := pageInfo["endCursor"].(string)

	nodes, ok := projectsData["nodes"].([]interface{})
	if !ok {
		return &pb.CRUDListResponse{Success: true, Entities: nil}, nil
	}

	var entities []*pb.CRUDEntity
	for _, node := range nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		fields := parseProjectFields(nodeMap)
		entities = append(entities, &pb.CRUDEntity{
			Id:         fields["id"],
			EntityType: "project",
			Fields:     fields,
			CreatedAt:  fields["created_at"],
			UpdatedAt:  fields["updated_at"],
		})
	}

	return &pb.CRUDListResponse{
		Entities:   entities,
		NextCursor: endCursor,
		TotalCount: int32(len(entities)),
		Success:    true,
	}, nil
}

// JSON parsing for Parse endpoint

func (m *LinearModule) parseIssueJSON(content []byte) (*pb.Observation, error) {
	var issue linearIssue
	if err := json.Unmarshal(content, &issue); err != nil {
		return nil, err
	}
	return m.issueToObservation(issue), nil
}

func (m *LinearModule) parseProjectJSON(content []byte) (*pb.Observation, error) {
	var project linearProject
	if err := json.Unmarshal(content, &project); err != nil {
		return nil, err
	}
	return m.projectToObservation(project), nil
}

// Helper functions

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}
