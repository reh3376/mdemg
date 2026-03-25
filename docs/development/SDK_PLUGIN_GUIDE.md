# MDEMG Plugin SDK Guide

This guide provides comprehensive documentation for building plugins (modules) for MDEMG (Memory-Driven Episodic Memory Graph). Following this guide step-by-step will allow you to create fully functional plugins.

## Table of Contents

1. [Overview](#1-overview)
2. [Quick Start](#2-quick-start)
3. [Plugin Architecture](#3-plugin-architecture)
4. [Module Type Reference](#4-module-type-reference)
5. [Best Practices](#5-best-practices)
6. [Troubleshooting](#6-troubleshooting)

---

## 1. Overview

### What are MDEMG Plugins?

MDEMG plugins (also called "modules") are standalone executables that extend MDEMG's functionality. They communicate with the MDEMG core via gRPC over Unix domain sockets, enabling:

- **Loose coupling**: Plugins run as separate processes
- **Language agnostic**: Any language with gRPC support can be used
- **Fault isolation**: Plugin crashes don't affect the core system
- **Hot reloading**: Plugins can be updated without restarting MDEMG

### Three Module Types

MDEMG supports three types of modules, each serving a distinct purpose:

| Type | Purpose | When Called |
|------|---------|-------------|
| **INGESTION** | Parse external sources into observations | During content ingestion |
| **REASONING** | Re-rank/filter retrieval results | During retrieval pipeline |
| **APE** | Background maintenance tasks | On schedule or event triggers |

### Communication Model

```
+----------------+                    +----------------+
|                |  gRPC over Unix    |                |
|   MDEMG Core   | <----------------> |   Plugin       |
|                |     Socket         |   (separate    |
|                |                    |    process)    |
+----------------+                    +----------------+
        |
        |  1. Spawn plugin binary with --socket flag
        |  2. Connect to socket
        |  3. Handshake (verify readiness)
        |  4. Periodic health checks
        |  5. Service-specific RPCs
        |  6. Shutdown signal
        v
```

---

## 2. Quick Start

### Minimal Working Plugin (Echo Module)

This example creates a minimal INGESTION module that echoes input as observations.

#### Step 1: Create Directory Structure

```bash
mkdir -p plugins/my-echo-module
cd plugins/my-echo-module
```

#### Step 2: Create manifest.json

```json
{
  "id": "my-echo-module",
  "name": "My Echo Module",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "my-echo-module",
  "capabilities": {
    "ingestion_sources": ["echo://"],
    "content_types": ["text/plain"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 5000
}
```

#### Step 3: Create main.go

```go
package main

import (
 "context"
 "flag"
 "fmt"
 "log"
 "net"
 "os"
 "os/signal"
 "strings"
 "syscall"
 "time"

 "google.golang.org/grpc"

 pb "mdemg/api/modulepb"
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

 // Create gRPC server
 server := grpc.NewServer()
 module := &MyModule{startTime: time.Now()}
 pb.RegisterModuleLifecycleServer(server, module)
 pb.RegisterIngestionModuleServer(server, module)

 log.Printf("Module listening on %s", *socketPath)

 // Handle shutdown gracefully
 sigChan := make(chan os.Signal, 1)
 signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

 go func() {
  <-sigChan
  log.Println("Shutting down...")
  server.GracefulStop()
 }()

 if err := server.Serve(listener); err != nil {
  log.Fatalf("Server error: %v", err)
 }
}

type MyModule struct {
 pb.UnimplementedModuleLifecycleServer
 pb.UnimplementedIngestionModuleServer
 startTime time.Time
}

// ============ Lifecycle RPCs (Required for ALL modules) ============

func (m *MyModule) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
 log.Printf("Handshake: mdemg_version=%s", req.MdemgVersion)
 return &pb.HandshakeResponse{
  ModuleId:      "my-echo-module",
  ModuleVersion: "1.0.0",
  ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
  Capabilities:  []string{"echo://", "text/plain"},
  Ready:         true,
 }, nil
}

func (m *MyModule) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
 return &pb.HealthCheckResponse{
  Healthy: true,
  Status:  "ok",
  Metrics: map[string]string{
   "uptime": time.Since(m.startTime).String(),
  },
 }, nil
}

func (m *MyModule) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
 log.Printf("Shutdown requested: reason=%s", req.Reason)
 return &pb.ShutdownResponse{Success: true, Message: "goodbye"}, nil
}

// ============ Ingestion RPCs ============

func (m *MyModule) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
 matches := strings.HasPrefix(req.SourceUri, "echo://") || req.ContentType == "text/plain"
 return &pb.MatchResponse{
  Matches:    matches,
  Confidence: 1.0,
  Reason:     "matches echo:// or text/plain",
 }, nil
}

func (m *MyModule) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
 obs := &pb.Observation{
  NodeId:      fmt.Sprintf("echo-%d", time.Now().UnixNano()),
  Path:        req.SourceUri,
  Name:        "echo-observation",
  Content:     string(req.Content),
  ContentType: req.ContentType,
  Tags:        []string{"echo"},
  Timestamp:   time.Now().Format(time.RFC3339),
  Source:      "my-echo-module",
 }
 return &pb.ParseResponse{Observations: []*pb.Observation{obs}}, nil
}

func (m *MyModule) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
 // Send a single observation for demonstration
 obs := &pb.Observation{
  NodeId:      fmt.Sprintf("echo-sync-%d", time.Now().UnixNano()),
  Path:        "echo://sync",
  Name:        "sync-observation",
  Content:     "sync content",
  ContentType: "text/plain",
  Tags:        []string{"echo", "sync"},
  Timestamp:   time.Now().Format(time.RFC3339),
  Source:      "my-echo-module",
 }
 return stream.Send(&pb.SyncResponse{
  Observations: []*pb.Observation{obs},
  Cursor:       "cursor-1",
  HasMore:      false,
  Stats:        &pb.SyncStats{ItemsProcessed: 1, ItemsCreated: 1},
 })
}
```

#### Step 4: Build and Deploy

```bash
# From the plugin directory
go build -o my-echo-module .

# The plugin directory structure should be:
# plugins/my-echo-module/
#   manifest.json
#   my-echo-module (binary)

# MDEMG will auto-discover plugins in the plugins/ directory on startup
```

---

## 3. Plugin Architecture

### Directory Structure

Each plugin must reside in its own directory under `plugins/`:

```
plugins/
  my-plugin/
    manifest.json     # Required: Plugin metadata and configuration
    my-plugin         # Required: Compiled binary (name must match manifest.binary)
    README.md         # Optional: Documentation
    config/           # Optional: Additional configuration files
```

### manifest.json Schema

The manifest file defines plugin metadata and behavior:

```json
{
  "id": "unique-plugin-id",
  "name": "Human Readable Name",
  "version": "1.0.0",
  "type": "INGESTION | REASONING | APE",
  "binary": "binary-name",
  "capabilities": {
    "ingestion_sources": ["source://"],
    "content_types": ["mime/type"],
    "pattern_detectors": ["pattern-name"],
    "event_triggers": ["event-name"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000,
  "config": {
    "key": "value"
  }
}
```

#### Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique identifier (lowercase, hyphens allowed). Used for socket naming. |
| `name` | string | Yes | Human-readable display name. |
| `version` | string | Yes | Semantic version (e.g., "1.0.0"). |
| `type` | string | Yes | One of: `INGESTION`, `REASONING`, `APE`. |
| `binary` | string | Yes | Name of the executable file in the plugin directory. |
| `capabilities` | object | No | Describes what the plugin can do. |
| `capabilities.ingestion_sources` | string[] | No | URI schemes this plugin handles (INGESTION only). |
| `capabilities.content_types` | string[] | No | MIME types this plugin can parse (INGESTION only). |
| `capabilities.pattern_detectors` | string[] | No | Pattern detection capabilities (REASONING only). |
| `capabilities.event_triggers` | string[] | No | Events that trigger execution (APE only). |
| `health_check_interval_ms` | int | No | Health check frequency. Default: 5000ms. |
| `startup_timeout_ms` | int | No | Max time to wait for handshake. Default: 10000ms. |
| `config` | object | No | Key-value configuration passed to plugin during handshake. |

### Plugin Lifecycle

```
1. SPAWN
   - MDEMG reads manifest.json
   - Spawns binary: ./plugin-binary --socket /tmp/mdemg-plugin-id.sock
   - Plugin creates Unix socket and listens

2. HANDSHAKE
   - MDEMG connects to socket
   - Sends HandshakeRequest with version, socket path, and config
   - Plugin responds with HandshakeResponse (ready=true)
   - MDEMG creates appropriate gRPC client (Ingestion/Reasoning/APE)

3. HEALTH CHECKS (ongoing)
   - MDEMG calls HealthCheck RPC at configured interval
   - Plugin responds with health status and metrics
   - If unhealthy, MDEMG may attempt restart

4. SERVICE CALLS
   - MDEMG calls service-specific RPCs based on module type
   - Plugin processes requests and returns responses

5. SHUTDOWN
   - MDEMG sends Shutdown RPC with timeout and reason
   - Plugin cleans up resources
   - Plugin exits gracefully
   - MDEMG removes socket file

6. CRASH RECOVERY
   - If plugin crashes, MDEMG detects via process monitoring
   - Automatic restart with exponential backoff (max 3 attempts)
   - Backoff: 2s, 4s, 6s
```

### Module States

| State | Description |
|-------|-------------|
| `starting` | Binary spawned, awaiting handshake |
| `ready` | Handshake complete, accepting requests |
| `unhealthy` | Health check failed |
| `stopping` | Shutdown in progress |
| `stopped` | Gracefully stopped |
| `crashed` | Process exited unexpectedly |

---

## 4. Module Type Reference

### 4.1 INGESTION Modules

**Purpose**: Parse external sources (files, APIs, webhooks) into MDEMG observations.

**Required Services**:

- `ModuleLifecycle` (all modules)
- `IngestionModule`

**Required RPCs**:

#### Matches(MatchRequest) -> MatchResponse

Called to determine if this module can handle a given source.

```protobuf
message MatchRequest {
    string source_uri = 1;          // e.g., "linear://workspace/issue/123"
    string content_type = 2;        // e.g., "application/json"
    map<string, string> metadata = 3;
}

message MatchResponse {
    bool matches = 1;               // Can this module handle it?
    float confidence = 2;           // 0.0-1.0 priority when multiple match
    string reason = 3;              // Explanation
}
```

#### Parse(ParseRequest) -> ParseResponse

Called to convert raw content into observations.

```protobuf
message ParseRequest {
    string source_uri = 1;
    string content_type = 2;
    bytes content = 3;              // Raw content bytes
    map<string, string> metadata = 4;
}

message ParseResponse {
    repeated Observation observations = 1;
    repeated Edge edges = 2;        // Relationships between observations
    map<string, string> metadata = 3;
    string error = 4;               // Set if parsing failed
}
```

#### Sync(SyncRequest) -> stream SyncResponse

Called for incremental synchronization with external sources.

```protobuf
message SyncRequest {
    string source_id = 1;           // Source identifier
    string cursor = 2;              // Resume from this point (empty = full sync)
    map<string, string> config = 3; // API keys, filters, etc.
}

message SyncResponse {
    repeated Observation observations = 1;
    repeated Edge edges = 2;
    string cursor = 3;              // Cursor for next sync
    bool has_more = 4;              // More data available?
    SyncStats stats = 5;
}
```

**Example Use Cases**:

- Parse Linear issues into observations
- Ingest Obsidian markdown notes
- Sync Slack messages
- Parse source code files

#### Complete INGESTION Template

```go
package main

import (
 "context"
 "flag"
 "fmt"
 "log"
 "net"
 "os"
 "os/signal"
 "strings"
 "sync/atomic"
 "syscall"
 "time"

 "google.golang.org/grpc"
 pb "mdemg/api/modulepb"
)

const (
 moduleID      = "my-ingestion-module"
 moduleVersion = "1.0.0"
)

var requestCounter uint64

func main() {
 socketPath := flag.String("socket", "", "Unix socket path")
 flag.Parse()

 if *socketPath == "" {
  log.Fatal("--socket is required")
 }

 os.Remove(*socketPath)

 listener, err := net.Listen("unix", *socketPath)
 if err != nil {
  log.Fatalf("Failed to listen: %v", err)
 }
 defer listener.Close()

 server := grpc.NewServer()
 module := &IngestionModule{startTime: time.Now()}
 pb.RegisterModuleLifecycleServer(server, module)
 pb.RegisterIngestionModuleServer(server, module)

 log.Printf("%s: listening on %s", moduleID, *socketPath)

 sigChan := make(chan os.Signal, 1)
 signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
 go func() {
  <-sigChan
  log.Printf("%s: shutting down", moduleID)
  server.GracefulStop()
 }()

 if err := server.Serve(listener); err != nil {
  log.Fatalf("Server error: %v", err)
 }
}

type IngestionModule struct {
 pb.UnimplementedModuleLifecycleServer
 pb.UnimplementedIngestionModuleServer
 startTime time.Time
 config    map[string]string
}

// ============ Lifecycle RPCs ============

func (m *IngestionModule) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
 log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

 // Store configuration from manifest
 m.config = req.Config

 return &pb.HandshakeResponse{
  ModuleId:      moduleID,
  ModuleVersion: moduleVersion,
  ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
  Capabilities:  []string{"myscheme://", "application/x-myformat"},
  Ready:         true,
 }, nil
}

func (m *IngestionModule) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
 return &pb.HealthCheckResponse{
  Healthy: true,
  Status:  "ok",
  Metrics: map[string]string{
   "uptime":           time.Since(m.startTime).String(),
   "requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&requestCounter)),
  },
 }, nil
}

func (m *IngestionModule) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
 log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
 return &pb.ShutdownResponse{Success: true, Message: "goodbye"}, nil
}

// ============ Ingestion RPCs ============

func (m *IngestionModule) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
 atomic.AddUint64(&requestCounter, 1)

 // Check if we can handle this source
 matches := strings.HasPrefix(req.SourceUri, "myscheme://") ||
  req.ContentType == "application/x-myformat"

 confidence := float32(0.0)
 reason := "not a supported source"
 if matches {
  confidence = 1.0
  reason = "matches myscheme:// or application/x-myformat"
 }

 return &pb.MatchResponse{
  Matches:    matches,
  Confidence: confidence,
  Reason:     reason,
 }, nil
}

func (m *IngestionModule) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
 atomic.AddUint64(&requestCounter, 1)

 // Parse content into observations
 // This is where your custom parsing logic goes

 obs := &pb.Observation{
  NodeId:      fmt.Sprintf("%s-%d", moduleID, time.Now().UnixNano()),
  Path:        req.SourceUri,
  Name:        "Parsed Item",
  Content:     string(req.Content),
  ContentType: req.ContentType,
  Tags:        []string{"parsed", "myformat"},
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

func (m *IngestionModule) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
 atomic.AddUint64(&requestCounter, 1)

 // Implement incremental sync logic
 // Use req.Cursor to resume from last position

 cursor := req.Cursor
 var processed, created int32

 // Example: fetch items in batches
 for {
  // Fetch next batch from external source
  items, nextCursor, hasMore := fetchItems(cursor, 50)

  var observations []*pb.Observation
  for _, item := range items {
   obs := itemToObservation(item)
   observations = append(observations, obs)
   processed++
   created++
  }

  // Send batch
  if err := stream.Send(&pb.SyncResponse{
   Observations: observations,
   Cursor:       nextCursor,
   HasMore:      hasMore,
   Stats: &pb.SyncStats{
    ItemsProcessed: processed,
    ItemsCreated:   created,
   },
  }); err != nil {
   return err
  }

  if !hasMore {
   break
  }
  cursor = nextCursor
 }

 return nil
}

// Helper functions for your implementation
func fetchItems(cursor string, limit int) ([]interface{}, string, bool) {
 // Implement fetching from your external source
 return nil, "", false
}

func itemToObservation(item interface{}) *pb.Observation {
 // Convert your item to an observation
 return &pb.Observation{}
}
```

---

### 4.2 REASONING Modules

**Purpose**: Re-rank, filter, or augment retrieval results during the query pipeline.

**Required Services**:

- `ModuleLifecycle` (all modules)
- `ReasoningModule`

**Required RPCs**:

#### Process(ProcessRequest) -> ProcessResponse

Called during retrieval to process/re-rank candidates.

```protobuf
message ProcessRequest {
    string query_text = 1;                 // Original query
    repeated RetrievalCandidate candidates = 2;
    int32 top_k = 3;                       // Return top K results
    map<string, string> context = 4;       // Additional context
}

message ProcessResponse {
    repeated RetrievalCandidate results = 1;
    map<string, string> metadata = 2;      // Processing info
    string error = 3;
}

message RetrievalCandidate {
    string node_id = 1;
    string path = 2;
    string name = 3;
    string summary = 4;
    float score = 5;                       // Can be modified by module
    float vector_sim = 6;
    float activation = 7;
    map<string, string> metadata = 8;
}
```

**Example Use Cases**:

- Keyword boosting (boost exact matches)
- Recency weighting
- Cross-encoder re-ranking
- Context-aware filtering
- Diversity optimization

#### Complete REASONING Template

```go
package main

import (
 "context"
 "flag"
 "log"
 "net"
 "os"
 "os/signal"
 "sort"
 "strconv"
 "strings"
 "sync"
 "syscall"
 "time"

 "google.golang.org/grpc"
 pb "mdemg/api/modulepb"
)

const (
 moduleID      = "my-reasoning-module"
 moduleVersion = "1.0.0"
)

func main() {
 socketPath := flag.String("socket", "", "Unix socket path")
 flag.Parse()

 if *socketPath == "" {
  log.Fatal("--socket is required")
 }

 os.Remove(*socketPath)

 listener, err := net.Listen("unix", *socketPath)
 if err != nil {
  log.Fatalf("Failed to listen: %v", err)
 }
 defer listener.Close()
 defer os.Remove(*socketPath)

 log.Printf("%s: listening on %s", moduleID, *socketPath)

 grpcServer := grpc.NewServer()
 s := &ReasoningServer{
  startTime:   time.Now(),
  boostFactor: 0.2,
 }

 pb.RegisterModuleLifecycleServer(grpcServer, s)
 pb.RegisterReasoningModuleServer(grpcServer, s)

 sigChan := make(chan os.Signal, 1)
 signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
 go func() {
  <-sigChan
  log.Printf("%s: shutting down", moduleID)
  grpcServer.GracefulStop()
 }()

 if err := grpcServer.Serve(listener); err != nil {
  log.Fatalf("Server error: %v", err)
 }
}

type ReasoningServer struct {
 pb.UnimplementedModuleLifecycleServer
 pb.UnimplementedReasoningModuleServer

 mu              sync.Mutex
 startTime       time.Time
 requestsHandled int64
 boostFactor     float64
}

// ============ Lifecycle RPCs ============

func (s *ReasoningServer) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
 log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

 // Parse configuration
 if factor, ok := req.Config["boost_factor"]; ok {
  if f, err := strconv.ParseFloat(factor, 64); err == nil {
   s.boostFactor = f
  }
 }

 return &pb.HandshakeResponse{
  ModuleId:      moduleID,
  ModuleVersion: moduleVersion,
  ModuleType:    pb.ModuleType_MODULE_TYPE_REASONING,
  Capabilities:  []string{"keyword_boost", "recency_weight"},
  Ready:         true,
 }, nil
}

func (s *ReasoningServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
 s.mu.Lock()
 requests := s.requestsHandled
 s.mu.Unlock()

 return &pb.HealthCheckResponse{
  Healthy: true,
  Status:  "ready",
  Metrics: map[string]string{
   "uptime":           time.Since(s.startTime).String(),
   "requests_handled": strconv.FormatInt(requests, 10),
   "boost_factor":     strconv.FormatFloat(s.boostFactor, 'f', 2, 64),
  },
 }, nil
}

func (s *ReasoningServer) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
 log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
 return &pb.ShutdownResponse{Success: true, Message: "goodbye"}, nil
}

// ============ Reasoning RPC ============

func (s *ReasoningServer) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
 s.mu.Lock()
 s.requestsHandled++
 s.mu.Unlock()

 if len(req.Candidates) == 0 {
  return &pb.ProcessResponse{Results: req.Candidates}, nil
 }

 log.Printf("%s: processing %d candidates for: %s",
  moduleID, len(req.Candidates), truncate(req.QueryText, 50))

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
  boost := calculateBoost(c.Name, c.Summary, keywords) * s.boostFactor

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
   "boost_factor": strconv.FormatFloat(s.boostFactor, 'f', 2, 64),
  },
 }, nil
}

// Helper functions

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
```

---

### 4.3 APE Modules (Active Participant Engine)

**Purpose**: Background maintenance tasks that run on schedules or event triggers.

**Required Services**:

- `ModuleLifecycle` (all modules)
- `APEModule`

**Required RPCs**:

#### GetSchedule(GetScheduleRequest) -> GetScheduleResponse

Called after handshake to determine when the module should run.

```protobuf
message GetScheduleRequest {
    // Empty
}

message GetScheduleResponse {
    string cron_expression = 1;            // e.g., "0 * * * *" (hourly)
    repeated string event_triggers = 2;    // e.g., ["session_end", "ingest"]
    int32 min_interval_seconds = 3;        // Minimum time between runs
}
```

#### Execute(ExecuteRequest) -> ExecuteResponse

Called when the schedule triggers or an event occurs.

```protobuf
message ExecuteRequest {
    string task_id = 1;                    // Unique execution ID
    string trigger = 2;                    // "schedule" or "event:EVENT_NAME"
    map<string, string> context = 3;       // Execution context
}

message ExecuteResponse {
    bool success = 1;
    string message = 2;
    ExecuteStats stats = 3;
    string error = 4;
}

message ExecuteStats {
    int32 nodes_created = 1;
    int32 nodes_updated = 2;
    int32 edges_created = 3;
    int32 edges_updated = 4;
    int64 duration_ms = 5;
}
```

**Event Triggers**:

| Event | Description |
|-------|-------------|
| `session_end` | User session completed |
| `ingest` | Content was ingested |
| `consolidate` | Consolidation cycle completed |
| `query` | Query was executed |

**Example Use Cases**:

- Session reflection/summarization
- Knowledge consolidation
- Stale content cleanup
- Pattern detection across observations
- Relationship inference

#### Complete APE Template

```go
package main

import (
 "context"
 "flag"
 "log"
 "net"
 "os"
 "os/signal"
 "strconv"
 "sync"
 "syscall"
 "time"

 "google.golang.org/grpc"
 pb "mdemg/api/modulepb"
)

const (
 moduleID      = "my-ape-module"
 moduleVersion = "1.0.0"
)

func main() {
 socketPath := flag.String("socket", "", "Unix socket path")
 flag.Parse()

 if *socketPath == "" {
  log.Fatal("--socket is required")
 }

 os.Remove(*socketPath)

 listener, err := net.Listen("unix", *socketPath)
 if err != nil {
  log.Fatalf("Failed to listen: %v", err)
 }
 defer listener.Close()
 defer os.Remove(*socketPath)

 log.Printf("%s: listening on %s", moduleID, *socketPath)

 grpcServer := grpc.NewServer()
 s := &APEServer{startTime: time.Now()}

 pb.RegisterModuleLifecycleServer(grpcServer, s)
 pb.RegisterAPEModuleServer(grpcServer, s)

 sigChan := make(chan os.Signal, 1)
 signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
 go func() {
  <-sigChan
  log.Printf("%s: shutting down", moduleID)
  grpcServer.GracefulStop()
 }()

 if err := grpcServer.Serve(listener); err != nil {
  log.Fatalf("Server error: %v", err)
 }
}

type APEServer struct {
 pb.UnimplementedModuleLifecycleServer
 pb.UnimplementedAPEModuleServer

 mu              sync.Mutex
 startTime       time.Time
 executionsTotal int64
 lastExecution   time.Time
}

// ============ Lifecycle RPCs ============

func (s *APEServer) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
 log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

 return &pb.HandshakeResponse{
  ModuleId:      moduleID,
  ModuleVersion: moduleVersion,
  ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
  Capabilities:  []string{"reflection", "cleanup"},
  Ready:         true,
 }, nil
}

func (s *APEServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
 s.mu.Lock()
 executions := s.executionsTotal
 lastExec := s.lastExecution
 s.mu.Unlock()

 metrics := map[string]string{
  "uptime":           time.Since(s.startTime).String(),
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

func (s *APEServer) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
 log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
 return &pb.ShutdownResponse{Success: true, Message: "goodbye"}, nil
}

// ============ APE RPCs ============

func (s *APEServer) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
 return &pb.GetScheduleResponse{
  // Run every hour
  CronExpression: "0 * * * *",
  // Also run on these events
  EventTriggers: []string{"session_end", "consolidate"},
  // Don't run more often than every 5 minutes
  MinIntervalSeconds: 300,
 }, nil
}

func (s *APEServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
 start := time.Now()

 s.mu.Lock()
 s.executionsTotal++
 s.lastExecution = start
 execNum := s.executionsTotal
 s.mu.Unlock()

 log.Printf("%s: executing task %s (trigger=%s, execution #%d)",
  moduleID, req.TaskId, req.Trigger, execNum)

 // Perform work based on trigger type
 var message string
 var stats *pb.ExecuteStats

 switch req.Trigger {
 case "event:session_end":
  // Analyze recent session activity
  message, stats = s.handleSessionEnd(ctx)

 case "event:consolidate":
  // Post-consolidation processing
  message, stats = s.handleConsolidate(ctx)

 case "schedule":
  // Periodic maintenance
  message, stats = s.handleScheduled(ctx)

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

// Task handlers

func (s *APEServer) handleSessionEnd(ctx context.Context) (string, *pb.ExecuteStats) {
 // Implement session reflection logic
 // - Query recent observations
 // - Generate summary nodes
 // - Create relationship edges

 return "Session reflection completed", &pb.ExecuteStats{
  NodesCreated: 0,
  NodesUpdated: 0,
  EdgesCreated: 0,
 }
}

func (s *APEServer) handleConsolidate(ctx context.Context) (string, *pb.ExecuteStats) {
 // Implement post-consolidation logic

 return "Post-consolidation processing completed", &pb.ExecuteStats{
  NodesUpdated: 0,
 }
}

func (s *APEServer) handleScheduled(ctx context.Context) (string, *pb.ExecuteStats) {
 // Implement periodic maintenance
 // - Cleanup stale data
 // - Recalculate statistics
 // - Detect patterns

 return "Scheduled maintenance completed", &pb.ExecuteStats{
  NodesUpdated: 0,
 }
}
```

---

## 5. Best Practices

### Error Handling

1. **Return errors in response fields, don't crash**:

   ```go
   func (m *Module) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
       result, err := doWork(req)
       if err != nil {
           // Return error in response, not as RPC error
           return &pb.ParseResponse{
               Error: fmt.Sprintf("parse failed: %v", err),
           }, nil
       }
       return &pb.ParseResponse{Observations: result}, nil
   }
   ```

2. **Use context for cancellation**:

   ```go
   func (m *Module) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
       select {
       case <-ctx.Done():
           return &pb.ProcessResponse{Error: "cancelled"}, nil
       default:
       }
       // Continue processing...
   }
   ```

3. **Handle gRPC errors gracefully**:

   ```go
   import "google.golang.org/grpc/status"

   if err := stream.Send(resp); err != nil {
       if status.Code(err) == codes.Canceled {
           log.Println("Client cancelled stream")
           return nil
       }
       return err
   }
   ```

### Logging and Metrics

1. **Use structured logging**:

   ```go
   log.Printf("%s: [%s] processing %d candidates",
       moduleID, req.QueryText[:min(20, len(req.QueryText))], len(req.Candidates))
   ```

2. **Track metrics for health checks**:

   ```go
   type Module struct {
       mu              sync.Mutex
       requestsTotal   int64
       errorsTotal     int64
       latencySum      time.Duration
       lastRequestAt   time.Time
   }

   func (m *Module) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
       m.mu.Lock()
       defer m.mu.Unlock()

       avgLatency := time.Duration(0)
       if m.requestsTotal > 0 {
           avgLatency = m.latencySum / time.Duration(m.requestsTotal)
       }

       return &pb.HealthCheckResponse{
           Healthy: m.errorsTotal < 100, // Example threshold
           Status:  "ok",
           Metrics: map[string]string{
               "requests_total": strconv.FormatInt(m.requestsTotal, 10),
               "errors_total":   strconv.FormatInt(m.errorsTotal, 10),
               "avg_latency_ms": strconv.FormatInt(avgLatency.Milliseconds(), 10),
           },
       }, nil
   }
   ```

### Configuration Management

1. **Use manifest config for static settings**:

   ```json
   {
       "config": {
           "api_key_env": "MY_API_KEY",
           "timeout_seconds": "30",
           "batch_size": "100"
       }
   }
   ```

2. **Read environment variables for secrets**:

   ```go
   func (m *Module) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
       apiKeyEnv := req.Config["api_key_env"]
       if apiKeyEnv == "" {
           apiKeyEnv = "DEFAULT_API_KEY"
       }
       m.apiKey = os.Getenv(apiKeyEnv)

       if m.apiKey == "" {
           return &pb.HandshakeResponse{
               Ready: false,
               Error: fmt.Sprintf("%s environment variable not set", apiKeyEnv),
           }, nil
       }
       // ...
   }
   ```

3. **Parse config values with defaults**:

   ```go
   timeout := 30 * time.Second
   if t, ok := req.Config["timeout_seconds"]; ok {
       if secs, err := strconv.Atoi(t); err == nil {
           timeout = time.Duration(secs) * time.Second
       }
   }
   ```

### Testing Your Plugin

1. **Unit test RPC handlers**:

   ```go
   func TestProcess(t *testing.T) {
       s := &ReasoningServer{boostFactor: 0.2}

       req := &pb.ProcessRequest{
           QueryText: "test query",
           Candidates: []*pb.RetrievalCandidate{
               {NodeId: "1", Name: "Test", Score: 0.5},
           },
           TopK: 10,
       }

       resp, err := s.Process(context.Background(), req)
       if err != nil {
           t.Fatalf("Process failed: %v", err)
       }

       if len(resp.Results) != 1 {
           t.Errorf("Expected 1 result, got %d", len(resp.Results))
       }
   }
   ```

2. **Integration test with Unix socket**:

   ```go
   func TestFullLifecycle(t *testing.T) {
       socketPath := "/tmp/test-module.sock"
       defer os.Remove(socketPath)

       // Start module in goroutine
       go func() {
           // ... start your module
       }()

       // Wait for socket
       time.Sleep(100 * time.Millisecond)

       // Connect as client
       conn, err := grpc.Dial("unix://"+socketPath, grpc.WithInsecure())
       if err != nil {
           t.Fatalf("Failed to connect: %v", err)
       }
       defer conn.Close()

       // Test handshake
       client := pb.NewModuleLifecycleClient(conn)
       resp, err := client.Handshake(context.Background(), &pb.HandshakeRequest{
           MdemgVersion: "1.0.0",
       })
       if err != nil {
           t.Fatalf("Handshake failed: %v", err)
       }
       if !resp.Ready {
           t.Errorf("Module not ready: %s", resp.Error)
       }
   }
   ```

---

## 6. Troubleshooting

### Common Errors and Solutions

#### "binary not found"

**Cause**: The binary specified in `manifest.json` doesn't exist or isn't executable.

**Solution**:

```bash
# Ensure binary exists and matches manifest
ls -la plugins/my-plugin/
# Should show: my-plugin (executable)

# Check manifest.binary field
cat plugins/my-plugin/manifest.json | jq .binary

# Make executable if needed
chmod +x plugins/my-plugin/my-plugin
```

#### "handshake failed: connection refused"

**Cause**: Plugin didn't create the socket in time or crashed during startup.

**Solution**:

1. Increase `startup_timeout_ms` in manifest
2. Check plugin logs for startup errors
3. Test plugin manually:

   ```bash
   ./my-plugin --socket /tmp/test.sock
   # In another terminal:
   ls -la /tmp/test.sock
   ```

#### "health check failed"

**Cause**: Plugin not responding to health checks within 2 seconds.

**Solution**:

1. Ensure HealthCheck returns quickly (no blocking operations)
2. Check for deadlocks in your code
3. Add timeout to any I/O operations:

   ```go
   func (m *Module) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
       // Don't do heavy work here!
       return &pb.HealthCheckResponse{Healthy: true, Status: "ok"}, nil
   }
   ```

#### "module not ready: <error>"

**Cause**: Plugin returned `Ready: false` in handshake response.

**Solution**: Check the error message in the handshake response and fix the underlying issue (missing config, missing API key, etc.)

#### Module keeps restarting

**Cause**: Plugin is crashing after startup.

**Solution**:

1. Check stderr output for panic messages
2. Add recovery handling:

   ```go
   func main() {
       defer func() {
           if r := recover(); r != nil {
               log.Printf("PANIC: %v\n%s", r, debug.Stack())
           }
       }()
       // ... rest of main
   }
   ```

3. MDEMG restarts crashed modules up to 3 times with backoff (2s, 4s, 6s)

### Debugging Tips

1. **Run plugin standalone**:

   ```bash
   ./my-plugin --socket /tmp/debug.sock
   ```

2. **Test with grpcurl**:

   ```bash
   grpcurl -plaintext -unix /tmp/debug.sock \
       mdemg.module.v1.ModuleLifecycle/HealthCheck
   ```

3. **Check socket permissions**:

   ```bash
   ls -la /var/run/mdemg/mdemg-*.sock
   ```

4. **Enable verbose logging**:

   ```go
   import "google.golang.org/grpc/grpclog"

   func init() {
       grpclog.SetLoggerV2(grpclog.NewLoggerV2(os.Stdout, os.Stdout, os.Stderr))
   }
   ```

5. **Monitor module status via API**:

   ```bash
   curl http://localhost:9999/v1/modules
   ```

---

## Appendix: Protocol Buffer Definitions

For reference, here are the complete protobuf message definitions:

```protobuf
// Observation - A node in the knowledge graph
message Observation {
    string node_id = 1;         // Optional - generated if not provided
    string path = 2;            // Path/URI of the source
    string name = 3;            // Human-readable name
    string content = 4;         // Content text
    string content_type = 5;    // Content type (e.g., "code", "task")
    repeated string tags = 6;   // Tags for categorization
    map<string, string> metadata = 7;
    string timestamp = 8;       // ISO8601 timestamp
    string source = 9;          // Source identifier
}

// Edge - A relationship between nodes
message Edge {
    string from_node_id = 1;
    string to_node_id = 2;
    string rel_type = 3;        // e.g., "BLOCKS", "RELATED_TO"
    float weight = 4;           // 0.0-1.0
    map<string, string> properties = 5;
}
```

---

## Appendix: Example manifest.json Files

### INGESTION Module (Linear Integration)

```json
{
  "id": "linear-module",
  "name": "Linear Integration Module",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "linear-module",
  "capabilities": {
    "ingestion_sources": ["linear://"],
    "content_types": ["application/vnd.linear.issue", "application/vnd.linear.project"]
  },
  "health_check_interval_ms": 10000,
  "startup_timeout_ms": 10000,
  "config": {
    "api_key_env": "LINEAR_API_KEY",
    "default_team": "",
    "sync_interval_minutes": "15"
  }
}
```

### REASONING Module (Keyword Booster)

```json
{
  "id": "keyword-booster",
  "name": "Keyword Booster Reasoning Module",
  "version": "1.0.0",
  "type": "REASONING",
  "binary": "keyword-booster",
  "capabilities": {
    "pattern_detectors": ["keyword_match", "term_frequency"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000,
  "config": {
    "boost_factor": "0.2"
  }
}
```

### APE Module (Reflection)

```json
{
  "id": "reflection-module",
  "name": "Reflection APE Module",
  "version": "1.0.0",
  "type": "APE",
  "binary": "reflection-module",
  "capabilities": {
    "event_triggers": ["session_end", "consolidate"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}
```
