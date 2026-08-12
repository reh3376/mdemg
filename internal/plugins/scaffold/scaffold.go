// Package scaffold provides plugin scaffolding generation functionality.
// Used by both the CLI tool (cmd/plugin-scaffold) and the API (POST /v1/plugins/create).
package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"mdemg/internal/pathsafe"
)

// ModuleType represents the type of MDEMG module
type ModuleType string

const (
	ModuleTypeIngestion ModuleType = "INGESTION"
	ModuleTypeReasoning ModuleType = "REASONING"
	ModuleTypeAPE       ModuleType = "APE"
)

// Config holds the configuration for scaffold generation
type Config struct {
	Name         string
	Type         ModuleType
	OutputDir    string
	Version      string
	Description  string
	Capabilities []string
	Author       string
	ModuleID     string // Lowercase, hyphenated version of Name
	StructName   string // PascalCase version for Go structs
}

// Result contains the scaffold generation result
type Result struct {
	PluginID     string   `json:"plugin_id"`
	PluginPath   string   `json:"plugin_path"`
	FilesCreated []string `json:"files_created"`
}

// ValidModuleTypes are the allowed module types
var ValidModuleTypes = map[string]bool{
	"INGESTION": true,
	"REASONING": true,
	"APE":       true,
}

// ToModuleID converts a plugin name to a lowercase, hyphenated module ID
func ToModuleID(name string) string {
	// Replace spaces and underscores with hyphens
	result := strings.ReplaceAll(name, " ", "-")
	result = strings.ReplaceAll(result, "_", "-")
	// Convert to lowercase
	result = strings.ToLower(result)
	// Remove any consecutive hyphens
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	// Trim leading/trailing hyphens
	result = strings.Trim(result, "-")
	return result
}

// ToStructName converts a plugin name to a PascalCase struct name
func ToStructName(name string) string {
	// Split on hyphens, underscores, and spaces
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	words := strings.Fields(name)

	var result strings.Builder
	for _, word := range words {
		if len(word) > 0 {
			result.WriteString(strings.ToUpper(word[:1]))
			if len(word) > 1 {
				result.WriteString(strings.ToLower(word[1:]))
			}
		}
	}

	// Ensure it starts with a letter
	s := result.String()
	if len(s) == 0 {
		return "Plugin"
	}
	return s
}

// Generate creates all scaffold files for the plugin
func Generate(cfg Config) (*Result, error) {
	// Normalize module ID and struct name
	if cfg.ModuleID == "" {
		cfg.ModuleID = ToModuleID(cfg.Name)
	}
	if cfg.StructName == "" {
		cfg.StructName = ToStructName(cfg.Name)
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}

	// SEC-TRANCHE-2 (2026-08-11): scaffold is reachable from
	// HTTP POST /v1/plugins/create (handlePluginCreate). cfg.Name is
	// user JSON; ToModuleID only lowercases + hyphenates (does NOT
	// strip `.` or `/`), so a name like "../../etc/passwd" produces
	// a ModuleID that filepath.Join would resolve outside OutputDir.
	// Route through pathsafe.SafeJoinUnderDir — same containment
	// contract as the plugin_handlers.go paths.
	pluginDir, err := pathsafe.SafeJoinUnderDir(cfg.OutputDir, cfg.ModuleID)
	if err != nil {
		return nil, fmt.Errorf("invalid module id %q for output dir %q: %w", cfg.ModuleID, cfg.OutputDir, err)
	}

	// Create plugin directory
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugin directory: %w", err)
	}

	result := &Result{
		PluginID:     cfg.ModuleID,
		PluginPath:   pluginDir,
		FilesCreated: []string{},
	}

	// Generate each file
	files := []struct {
		name     string
		template string
		perm     os.FileMode
	}{
		{"manifest.json", getManifestTemplate(cfg.Type), 0644},
		{"main.go", mainGoTemplate, 0644},
		{"handler.go", getHandlerTemplate(cfg.Type), 0644},
		{"Makefile", makefileTemplate, 0644},
		{"README.md", readmeTemplate, 0644},
	}

	for _, f := range files {
		// SEC-TRANCHE-2 (2026-08-11): f.name comes from the local
		// files slice (hardcoded literals), so this is defense-in-
		// depth against a future refactor introducing dynamic names.
		// Uniform pattern with the pluginDir join above.
		path, err := pathsafe.SafeJoinUnderDir(pluginDir, f.name)
		if err != nil {
			return nil, fmt.Errorf("invalid scaffold file name %q: %w", f.name, err)
		}
		content, err := executeTemplate(f.name, f.template, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to generate %s: %w", f.name, err)
		}
		if err := os.WriteFile(path, []byte(content), f.perm); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", f.name, err)
		}
		result.FilesCreated = append(result.FilesCreated, f.name)
	}

	return result, nil
}

// executeTemplate renders a template with the given config
func executeTemplate(name, tmplStr string, cfg Config) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// getManifestTemplate returns the manifest.json template for the given module type
func getManifestTemplate(mt ModuleType) string {
	switch mt {
	case ModuleTypeIngestion:
		return manifestIngestionTemplate
	case ModuleTypeReasoning:
		return manifestReasoningTemplate
	case ModuleTypeAPE:
		return manifestAPETemplate
	default:
		return manifestIngestionTemplate
	}
}

// getHandlerTemplate returns the handler.go template for the given module type
func getHandlerTemplate(mt ModuleType) string {
	switch mt {
	case ModuleTypeIngestion:
		return handlerIngestionTemplate
	case ModuleTypeReasoning:
		return handlerReasoningTemplate
	case ModuleTypeAPE:
		return handlerAPETemplate
	default:
		return handlerIngestionTemplate
	}
}

// GenerateManifest creates a manifest.json content for the plugin
func GenerateManifest(cfg Config) ([]byte, error) {
	manifest := map[string]interface{}{
		"id":      cfg.ModuleID,
		"name":    cfg.Name,
		"version": cfg.Version,
		"type":    string(cfg.Type),
		"binary":  cfg.ModuleID,
	}

	switch cfg.Type {
	case ModuleTypeIngestion:
		manifest["capabilities"] = map[string]interface{}{
			"ingestion_sources": []string{cfg.ModuleID + "://"},
			"content_types":     []string{"application/x-" + cfg.ModuleID},
		}
	case ModuleTypeReasoning:
		manifest["capabilities"] = map[string]interface{}{
			"pattern_detectors": []string{"custom_ranking"},
		}
	case ModuleTypeAPE:
		manifest["capabilities"] = map[string]interface{}{
			"event_triggers": []string{"session_end"},
		}
	}

	manifest["health_check_interval_ms"] = 5000
	manifest["startup_timeout_ms"] = 10000

	return json.MarshalIndent(manifest, "", "  ")
}

// Template definitions

const manifestIngestionTemplate = `{
  "id": "{{.ModuleID}}",
  "name": "{{.Name}}",
  "version": "{{.Version}}",
  "type": "INGESTION",
  "binary": "{{.ModuleID}}",
  "capabilities": {
    "ingestion_sources": ["{{.ModuleID}}://"],
    "content_types": ["application/x-{{.ModuleID}}"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}
`

const manifestReasoningTemplate = `{
  "id": "{{.ModuleID}}",
  "name": "{{.Name}}",
  "version": "{{.Version}}",
  "type": "REASONING",
  "binary": "{{.ModuleID}}",
  "capabilities": {
    "pattern_detectors": ["custom_ranking"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000,
  "config": {
    "boost_factor": "0.2"
  }
}
`

const manifestAPETemplate = `{
  "id": "{{.ModuleID}}",
  "name": "{{.Name}}",
  "version": "{{.Version}}",
  "type": "APE",
  "binary": "{{.ModuleID}}",
  "capabilities": {
    "event_triggers": ["session_end", "consolidate"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}
`

const mainGoTemplate = `package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

const (
	moduleID      = "{{.ModuleID}}"
	moduleVersion = "{{.Version}}"
)

func main() {
	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		log.Fatal("--socket is required")
	}

	// Remove stale socket
	os.Remove(*socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	log.Printf("%s: listening on %s", moduleID, *socketPath)

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Create and register module handler
	handler := New{{.StructName}}Handler()
	pb.RegisterModuleLifecycleServer(grpcServer, handler)
{{if eq .Type "INGESTION"}}	pb.RegisterIngestionModuleServer(grpcServer, handler)
{{else if eq .Type "REASONING"}}	pb.RegisterReasoningModuleServer(grpcServer, handler)
{{else if eq .Type "APE"}}	pb.RegisterAPEModuleServer(grpcServer, handler)
{{end}}
	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		log.Printf("%s: received shutdown signal", moduleID)
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
`

const handlerIngestionTemplate = `package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pb "mdemg/api/modulepb"
)

// {{.StructName}}Handler implements the INGESTION module interfaces
type {{.StructName}}Handler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer

	mu        sync.Mutex
	startTime time.Time
	config    map[string]string
}

var requestCounter uint64

// New{{.StructName}}Handler creates a new handler instance
func New{{.StructName}}Handler() *{{.StructName}}Handler {
	return &{{.StructName}}Handler{
		startTime: time.Now(),
		config:    make(map[string]string),
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *{{.StructName}}Handler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Store configuration from manifest
	h.mu.Lock()
	h.config = req.Config
	h.mu.Unlock()

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"{{.ModuleID}}://", "application/x-{{.ModuleID}}"},
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *{{.StructName}}Handler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&requestCounter)),
		},
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *{{.StructName}}Handler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ Ingestion RPCs ============

// Matches checks if this module can handle the given source.
func (h *{{.StructName}}Handler) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Customize matching logic for your data source
	matches := strings.HasPrefix(req.SourceUri, "{{.ModuleID}}://") ||
		req.ContentType == "application/x-{{.ModuleID}}"

	confidence := float32(0.0)
	reason := "not a supported source"
	if matches {
		confidence = 1.0
		reason = "matches {{.ModuleID}}:// or application/x-{{.ModuleID}}"
	}

	return &pb.MatchResponse{
		Matches:    matches,
		Confidence: confidence,
		Reason:     reason,
	}, nil
}

// Parse converts source content into MDEMG observations.
func (h *{{.StructName}}Handler) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement your parsing logic here
	// Convert the raw content into structured observations

	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-%d", moduleID, time.Now().UnixNano()),
		Path:        req.SourceUri,
		Name:        "Parsed Item", // TODO: Extract a meaningful name
		Content:     string(req.Content),
		ContentType: req.ContentType,
		Tags:        []string{moduleID}, // TODO: Add relevant tags
		Metadata:    map[string]string{"source_uri": req.SourceUri},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	return &pb.ParseResponse{
		Observations: []*pb.Observation{obs},
		Metadata: map[string]string{
			"parsed_at": time.Now().Format(time.RFC3339),
		},
	}, nil
}

// Sync performs incremental synchronization with an external source.
func (h *{{.StructName}}Handler) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement incremental sync logic
	// Use req.Cursor to resume from the last position
	// Fetch items from your external source in batches

	// Example: Send a single response (replace with your sync logic)
	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-sync-%d", moduleID, time.Now().UnixNano()),
		Path:        fmt.Sprintf("%s://sync", moduleID),
		Name:        "sync-observation",
		Content:     "TODO: Replace with synced content",
		ContentType: "text/plain",
		Tags:        []string{moduleID, "sync"},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	if err := stream.Send(&pb.SyncResponse{
		Observations: []*pb.Observation{obs},
		Cursor:       "cursor-1", // TODO: Return actual cursor for incremental sync
		HasMore:      false,
		Stats: &pb.SyncStats{
			ItemsProcessed: 1,
			ItemsCreated:   1,
		},
	}); err != nil {
		return err
	}

	return nil
}
`

const handlerReasoningTemplate = `package main

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
)

// {{.StructName}}Handler implements the REASONING module interfaces
type {{.StructName}}Handler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedReasoningModuleServer

	mu              sync.Mutex
	startTime       time.Time
	requestsHandled int64
	boostFactor     float64
}

// New{{.StructName}}Handler creates a new handler instance
func New{{.StructName}}Handler() *{{.StructName}}Handler {
	return &{{.StructName}}Handler{
		startTime:   time.Now(),
		boostFactor: 0.2, // Default boost factor
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *{{.StructName}}Handler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Parse configuration
	if factor, ok := req.Config["boost_factor"]; ok {
		if f, err := strconv.ParseFloat(factor, 64); err == nil {
			h.boostFactor = f
			log.Printf("%s: boost_factor set to %.2f", moduleID, h.boostFactor)
		}
	}

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_REASONING,
		Capabilities:  []string{"custom_ranking"},
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *{{.StructName}}Handler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	h.mu.Lock()
	requests := h.requestsHandled
	h.mu.Unlock()

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: map[string]string{
			"uptime":           time.Since(h.startTime).String(),
			"requests_handled": strconv.FormatInt(requests, 10),
			"boost_factor":     strconv.FormatFloat(h.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *{{.StructName}}Handler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ Reasoning RPCs ============

// Process takes retrieval candidates and returns refined results.
func (h *{{.StructName}}Handler) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
	h.mu.Lock()
	h.requestsHandled++
	h.mu.Unlock()

	if len(req.Candidates) == 0 {
		return &pb.ProcessResponse{Results: req.Candidates}, nil
	}

	log.Printf("%s: processing %d candidates for: %s",
		moduleID, len(req.Candidates), truncate(req.QueryText, 50))

	// TODO: Implement your re-ranking/filtering logic here
	// Example: Simple keyword-based boosting

	// Extract keywords from query
	keywords := extractKeywords(req.QueryText)

	// Score and boost candidates
	type scored struct {
		candidate *pb.RetrievalCandidate
		boost     float64
	}

	results := make([]scored, len(req.Candidates))
	for i, c := range req.Candidates {
		// Calculate boost based on keyword matches
		boost := calculateBoost(c.Name, c.Summary, keywords) * h.boostFactor

		// Apply boost to score
		c.Score = c.Score + float32(boost)

		results[i] = scored{candidate: c, boost: boost}
	}

	// Sort by new score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].candidate.Score > results[j].candidate.Score
	})

	// Build output
	output := make([]*pb.RetrievalCandidate, len(results))
	for i, r := range results {
		output[i] = r.candidate
	}

	// Apply top_k limit
	if req.TopK > 0 && int(req.TopK) < len(output) {
		output = output[:req.TopK]
	}

	return &pb.ProcessResponse{
		Results: output,
		Metadata: map[string]string{
			"keywords":     strings.Join(keywords, ","),
			"boost_factor": strconv.FormatFloat(h.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// ============ Helper Functions ============

// extractKeywords tokenizes the query into keywords (customize as needed)
func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"of": true, "at": true, "by": true, "for": true, "with": true,
		"to": true, "from": true, "in": true, "out": true, "on": true,
		"and": true, "or": true, "but": true, "if": true, "what": true,
		"this": true, "that": true, "how": true, "why": true, "where": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}/-")
		if len(word) >= 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

// calculateBoost scores how well a candidate matches the keywords
func calculateBoost(name, summary string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	combined := strings.ToLower(name + " " + summary)
	matchCount := 0

	for _, kw := range keywords {
		if strings.Contains(combined, kw) {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(keywords))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
`

const handlerAPETemplate = `package main

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	pb "mdemg/api/modulepb"
)

// {{.StructName}}Handler implements the APE module interfaces
type {{.StructName}}Handler struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time
}

// New{{.StructName}}Handler creates a new handler instance
func New{{.StructName}}Handler() *{{.StructName}}Handler {
	return &{{.StructName}}Handler{
		startTime: time.Now(),
	}
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

// Handshake is called immediately after spawn to verify module is ready.
func (h *{{.StructName}}Handler) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
		Capabilities:  []string{"background_task"}, // TODO: Describe your capabilities
		Ready:         true,
	}, nil
}

// HealthCheck is called periodically to verify module health.
func (h *{{.StructName}}Handler) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	h.mu.Lock()
	executions := h.executionsTotal
	lastExec := h.lastExecution
	h.mu.Unlock()

	metrics := map[string]string{
		"uptime":           time.Since(h.startTime).String(),
		"executions_total": strconv.FormatInt(executions, 10),
	}
	if !lastExec.IsZero() {
		metrics["last_execution"] = lastExec.Format(time.RFC3339)
	}

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: metrics,
	}, nil
}

// Shutdown is called when MDEMG is stopping or the module is being disabled.
func (h *{{.StructName}}Handler) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "goodbye",
	}, nil
}

// ============ APE RPCs ============

// GetSchedule returns the module's preferred execution schedule.
func (h *{{.StructName}}Handler) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	return &pb.GetScheduleResponse{
		// TODO: Customize your schedule
		CronExpression:     "0 * * * *", // Every hour
		EventTriggers:      []string{"session_end", "consolidate"},
		MinIntervalSeconds: 300, // Minimum 5 minutes between runs
	}, nil
}

// Execute runs a background task.
func (h *{{.StructName}}Handler) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	start := time.Now()

	h.mu.Lock()
	h.executionsTotal++
	h.lastExecution = start
	execNum := h.executionsTotal
	h.mu.Unlock()

	log.Printf("%s: executing task %s (trigger=%s, execution #%d)",
		moduleID, req.TaskId, req.Trigger, execNum)

	// TODO: Implement your background task logic here
	var message string
	var stats *pb.ExecuteStats

	switch req.Trigger {
	case "event:session_end":
		// TODO: Handle session end event
		message, stats = h.handleSessionEnd(ctx)

	case "event:consolidate":
		// TODO: Handle consolidation event
		message, stats = h.handleConsolidate(ctx)

	case "schedule":
		// TODO: Handle scheduled execution
		message, stats = h.handleScheduled(ctx)

	default:
		message = "Unknown trigger, no action taken"
		stats = &pb.ExecuteStats{DurationMs: time.Since(start).Milliseconds()}
	}

	stats.DurationMs = time.Since(start).Milliseconds()

	log.Printf("%s: task %s completed in %v", moduleID, req.TaskId, time.Since(start))

	return &pb.ExecuteResponse{
		Success: true,
		Message: message,
		Stats:   stats,
	}, nil
}

// ============ Task Handlers (Customize these) ============

func (h *{{.StructName}}Handler) handleSessionEnd(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement session reflection logic
	// - Query recent observations
	// - Generate summary nodes
	// - Create relationship edges

	return "Session processing completed", &pb.ExecuteStats{
		NodesCreated: 0,
		NodesUpdated: 0,
		EdgesCreated: 0,
	}
}

func (h *{{.StructName}}Handler) handleConsolidate(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement post-consolidation logic

	return "Post-consolidation processing completed", &pb.ExecuteStats{
		NodesUpdated: 0,
	}
}

func (h *{{.StructName}}Handler) handleScheduled(ctx context.Context) (string, *pb.ExecuteStats) {
	// TODO: Implement periodic maintenance
	// - Cleanup stale data
	// - Recalculate statistics
	// - Detect patterns

	return "Scheduled maintenance completed", &pb.ExecuteStats{
		NodesUpdated: 0,
	}
}
`

const makefileTemplate = `.PHONY: build clean test

BINARY_NAME = {{.ModuleID}}

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

test:
	go test -v ./...

run: build
	./$(BINARY_NAME) --socket /tmp/$(BINARY_NAME).sock
`

const readmeTemplate = `# {{.Name}}

{{if eq .Type "INGESTION"}}An INGESTION module for MDEMG that parses external sources into observations.{{else if eq .Type "REASONING"}}A REASONING module for MDEMG that re-ranks and filters retrieval results.{{else if eq .Type "APE"}}An APE (Active Participant Engine) module for MDEMG that performs background maintenance tasks.{{end}}

## Overview

- **Module ID**: ` + "`{{.ModuleID}}`" + `
- **Type**: {{.Type}}
- **Version**: {{.Version}}

## Building

` + "```bash" + `
make build
` + "```" + `

## Development

1. Edit ` + "`handler.go`" + ` to implement your custom logic
2. Update ` + "`manifest.json`" + ` with your capabilities
3. Build with ` + "`make build`" + `
4. Test locally: ` + "`make run`" + `

## Testing Locally

` + "```bash" + `
# Start the module
./{{.ModuleID}} --socket /tmp/{{.ModuleID}}.sock

# In another terminal, test with grpcurl
grpcurl -plaintext -unix /tmp/{{.ModuleID}}.sock \
    mdemg.module.v1.ModuleLifecycle/HealthCheck
` + "```" + `

## Deployment

Place the built binary and ` + "`manifest.json`" + ` in the MDEMG plugins directory:

` + "```" + `
plugins/
  {{.ModuleID}}/
    {{.ModuleID}}    # binary
    manifest.json
` + "```" + `

MDEMG will auto-discover the plugin on startup.

{{if eq .Type "INGESTION"}}## Ingestion Module

This module implements:
- **Matches**: Determine if this module can handle a given source
- **Parse**: Convert raw content into MDEMG observations
- **Sync**: Incrementally sync with external sources

### Customization

Edit the ` + "`Matches`" + ` function to define which sources this module handles.
Edit the ` + "`Parse`" + ` function to implement your parsing logic.
{{else if eq .Type "REASONING"}}## Reasoning Module

This module implements:
- **Process**: Re-rank/filter retrieval candidates

### Customization

Edit the ` + "`Process`" + ` function to implement your ranking algorithm.
The default implementation uses keyword-based boosting.
{{else if eq .Type "APE"}}## APE Module

This module implements:
- **GetSchedule**: Define when this module should run
- **Execute**: Perform background maintenance tasks

### Customization

Edit ` + "`GetSchedule`" + ` to define your cron schedule and event triggers.
Edit the handler functions (` + "`handleSessionEnd`" + `, etc.) to implement your logic.
{{end}}

## Configuration

Configuration is passed via ` + "`manifest.json`" + `:

` + "```json" + `
{
  "config": {
    "key": "value"
  }
}
` + "```" + `

Access configuration in your handler via ` + "`req.Config`" + ` in the Handshake RPC.
`
