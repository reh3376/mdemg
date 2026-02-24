# MDEMG Module Development Guide

This guide explains how to develop plugin modules for the MDEMG memory system.

## Overview

MDEMG supports three types of external modules:

| Type | Purpose | Interface |
|------|---------|-----------|
| INGESTION | Parse external data sources into observations | `IngestionModule` |
| REASONING | Process retrieval results with pattern detection | `ReasoningModule` |
| APE | Active Participation Entity - autonomous actions | `APEModule` |

All modules communicate with MDEMG via **gRPC over Unix sockets** for low latency.

## Module Structure

```
plugins/
└── my-module/
    ├── manifest.json      # Required: module metadata
    ├── my-module          # Required: executable binary
    └── ... (other files)
```

### manifest.json

```json
{
  "id": "my-module",
  "name": "Human-Readable Name",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "my-module",
  "capabilities": {
    "ingestion_sources": ["myprotocol://"],
    "content_types": ["application/x-my-format"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 5000,
  "config": {
    "custom_key": "value"
  }
}
```

**Required fields:**

- `id` - Unique identifier (lowercase, alphanumeric + hyphens)
- `name` - Display name
- `version` - Semantic version
- `type` - One of: `INGESTION`, `REASONING`, `APE`
- `binary` - Executable filename (relative to module directory)

**Optional fields:**

- `capabilities` - What the module can handle
- `health_check_interval_ms` - Health check frequency (default: 5000)
- `startup_timeout_ms` - Max time to become ready (default: 10000)
- `config` - Custom key-value configuration passed during handshake

## Protocol Definition

The gRPC protocol is defined in `api/proto/mdemg-module.proto`. Generate bindings for your language:

```bash
# Go
protoc --go_out=. --go-grpc_out=. api/proto/mdemg-module.proto

# Python
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. api/proto/mdemg-module.proto
```

## Required: ModuleLifecycle Service

Every module **must** implement the `ModuleLifecycle` service:

```protobuf
service ModuleLifecycle {
    rpc Handshake(HandshakeRequest) returns (HandshakeResponse);
    rpc HealthCheck(HealthCheckRequest) returns (HealthCheckResponse);
    rpc Shutdown(ShutdownRequest) returns (ShutdownResponse);
}
```

### Handshake

Called once after the module starts. MDEMG provides version info and config; module returns capabilities.

```go
func (m *MyModule) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
    // req.MdemgVersion - MDEMG server version
    // req.SocketPath - Where this module is listening
    // req.Config - Custom config from manifest.json

    return &pb.HandshakeResponse{
        ModuleId:      "my-module",
        ModuleVersion: "1.0.0",
        ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
        Capabilities:  []string{"myprotocol://", "text/plain"},
        Ready:         true,
    }, nil
}
```

### HealthCheck

Called periodically by MDEMG. Return `Healthy: true` or the module will be marked unhealthy.

```go
func (m *MyModule) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
    return &pb.HealthCheckResponse{
        Healthy: true,
        Status:  "ok",
        Metrics: map[string]string{
            "requests_handled": "42",
            "uptime":           "1h30m",
        },
    }, nil
}
```

### Shutdown

Called when MDEMG is stopping. Clean up resources and exit gracefully.

```go
func (m *MyModule) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
    // req.Reason - Why shutdown was requested
    // req.TimeoutMs - How long you have to shut down

    // Clean up resources here

    return &pb.ShutdownResponse{
        Success: true,
        Message: "goodbye",
    }, nil
}
```

## Ingestion Modules

Implement the `IngestionModule` service to parse external data:

```protobuf
service IngestionModule {
    rpc Matches(MatchRequest) returns (MatchResponse);
    rpc Parse(ParseRequest) returns (ParseResponse);
    rpc Sync(SyncRequest) returns (stream SyncResponse);
}
```

### Matches

Check if this module can handle a given source URI or content type.

```go
func (m *MyModule) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
    canHandle := strings.HasPrefix(req.SourceUri, "myprotocol://")
    confidence := float32(0.0)
    if canHandle {
        confidence = 1.0
    }
    return &pb.MatchResponse{
        Matches:    canHandle,
        Confidence: confidence,
        Reason:     "supports myprotocol:// URIs",
    }, nil
}
```

### Parse

Parse content and return observations (memory nodes).

```go
func (m *MyModule) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
    // req.SourceUri - Where the content came from
    // req.Content - Raw bytes to parse
    // req.ContentType - MIME type
    // req.SpaceId - Memory space to ingest into

    obs := &pb.Observation{
        NodeId:      uuid.New().String(),
        Path:        req.SourceUri,
        Name:        "parsed-item",
        Content:     string(req.Content),
        ContentType: req.ContentType,
        Tags:        []string{"ingested"},
        Timestamp:   time.Now().Format(time.RFC3339),
        Source:      "my-module",
    }

    return &pb.ParseResponse{
        Observations: []*pb.Observation{obs},
        Metadata: map[string]string{
            "parsed_at": time.Now().Format(time.RFC3339),
        },
    }, nil
}
```

### Sync (Streaming)

For continuous data sources (e.g., file watchers, database polling).

```go
func (m *MyModule) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
    // req.SourceUri - What to sync
    // req.Cursor - Resume from this position

    for item := range dataSource {
        obs := convertToObservation(item)
        if err := stream.Send(&pb.SyncResponse{
            Observations: []*pb.Observation{obs},
            Cursor:       item.ID,
            HasMore:      true,
        }); err != nil {
            return err
        }
    }

    return stream.Send(&pb.SyncResponse{HasMore: false})
}
```

## Reasoning Modules

Implement `ReasoningModule` to process/re-rank retrieval results:

```protobuf
service ReasoningModule {
    rpc Process(ProcessRequest) returns (ProcessResponse);
}
```

### Process

Called during retrieval after initial scoring. Re-rank or filter candidates:

```go
func (m *MyModule) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
    // req.QueryText - The user's query
    // req.Candidates - Scored candidates from retrieval
    // req.TopK - Number of results to return
    // req.Context - Additional context (e.g., space_id)

    // Example: boost candidates matching keywords
    results := make([]*pb.RetrievalCandidate, len(req.Candidates))
    for i, c := range req.Candidates {
        boost := calculateKeywordMatch(c.Name, req.QueryText)
        c.Score = c.Score + float32(boost * 0.2)
        results[i] = c
    }

    // Sort by score and return
    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })

    return &pb.ProcessResponse{
        Results: results[:req.TopK],
        Metadata: map[string]string{
            "boost_applied": "keyword_match",
        },
    }, nil
}
```

### Pipeline Position

Reasoning modules are called in the retrieval pipeline:

```
1. Vector recall
2. BM25 + RRF fusion
3. Graph expansion
4. Spreading activation
5. Initial scoring
6. REASONING MODULES ← Your module here
7. Built-in LLM re-ranking (optional)
8. Jiminy explanations
```

Results include reasoning module info in debug output:

```json
{
  "debug": {
    "reasoning_module": "keyword-booster",
    "reasoning_latency_ms": 1.5,
    "reasoning_tokens": 0
  }
}
```

## APE Modules

Implement `APEModule` for background tasks and autonomous actions:

```protobuf
service APEModule {
    rpc Execute(ExecuteRequest) returns (ExecuteResponse);
    rpc GetSchedule(GetScheduleRequest) returns (GetScheduleResponse);
}
```

### GetSchedule

Return when and how the module should be executed:

```go
func (m *MyModule) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
    return &pb.GetScheduleResponse{
        CronExpression:     "0 * * * *",  // Every hour
        EventTriggers:      []string{"session_end", "consolidate"},
        MinIntervalSeconds: 300,  // Minimum 5 minutes between runs
    }, nil
}
```

### Execute

Called by the APE scheduler based on cron or event triggers:

```go
func (m *MyModule) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
    // req.TaskId - Unique execution ID
    // req.Trigger - What triggered this ("schedule" or "event:session_end")
    // req.Context - Additional context

    // Do background work here...

    return &pb.ExecuteResponse{
        Success: true,
        Message: "Reflection completed",
        Stats: &pb.ExecuteStats{
            NodesCreated: 5,
            NodesUpdated: 10,
            EdgesCreated: 3,
            DurationMs:   150,
        },
    }, nil
}
```

### APE API Endpoints

```bash
# Check scheduler status
curl http://localhost:9999/v1/ape/status

# Manually trigger an event
curl -X POST http://localhost:9999/v1/ape/trigger \
  -H "Content-Type: application/json" \
  -d '{"event": "session_end"}'
```

## Running Your Module

1. **Start the binary with `--socket` flag:**

   ```bash
   ./my-module --socket /tmp/mdemg-plugins/mdemg-my-module.sock
   ```

2. **MDEMG handles the rest:**
   - Discovers modules in `PLUGINS_DIR` (default: `./plugins`)
   - Spawns each module binary with appropriate socket path
   - Performs handshake
   - Starts health check loop
   - Auto-restarts on crash (up to 3 times with backoff)

## Example: Echo Module

See `plugins/echo-module/` for a complete working example:

```go
package main

import (
    "flag"
    "net"
    "os"
    "os/signal"
    "syscall"

    "google.golang.org/grpc"
    pb "mdemg/api/modulepb"
)

var socketPath = flag.String("socket", "", "Unix socket path")

func main() {
    flag.Parse()
    if *socketPath == "" {
        log.Fatal("--socket is required")
    }

    // Remove stale socket
    os.Remove(*socketPath)

    // Create listener
    listener, err := net.Listen("unix", *socketPath)
    if err != nil {
        log.Fatal(err)
    }
    defer listener.Close()

    // Create gRPC server and register services
    server := grpc.NewServer()
    module := &EchoModule{startTime: time.Now()}
    pb.RegisterModuleLifecycleServer(server, module)
    pb.RegisterIngestionModuleServer(server, module)

    // Handle shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    go func() {
        <-sigChan
        server.GracefulStop()
    }()

    // Serve
    server.Serve(listener)
}
```

## Configuration

Environment variables for the plugin system:

| Variable | Default | Description |
|----------|---------|-------------|
| `PLUGINS_ENABLED` | `true` | Enable/disable plugin system |
| `PLUGINS_DIR` | `./plugins` | Directory to scan for modules |
| `PLUGIN_SOCKET_DIR` | `/tmp/mdemg-plugins` | Directory for Unix sockets |
| `MDEMG_VERSION` | `0.6.0` | Version sent to modules during handshake |

## API Endpoint

List loaded modules:

```bash
curl http://localhost:8080/v1/modules | jq
```

Response:

```json
{
  "data": {
    "enabled": true,
    "modules": [
      {
        "id": "echo-module",
        "name": "Echo Test Module",
        "version": "1.0.0",
        "type": "INGESTION",
        "state": "ready",
        "socket_path": "/tmp/mdemg-plugins/mdemg-echo-module.sock",
        "pid": 12345,
        "started_at": "2026-01-23T04:57:03Z",
        "last_healthy": "2026-01-23T04:57:08Z",
        "capabilities": ["echo://", "text/plain"],
        "metrics": {
          "uptime": "5m",
          "requests_handled": "10"
        }
      }
    ]
  }
}
```

## Module States

| State | Description |
|-------|-------------|
| `starting` | Binary spawned, waiting for handshake |
| `ready` | Handshake complete, accepting requests |
| `unhealthy` | Health check failed |
| `stopping` | Shutdown in progress |
| `stopped` | Gracefully stopped |
| `crashed` | Process exited unexpectedly |

## Best Practices

1. **Keep modules focused** - One module per data source or capability
2. **Handle timeouts** - Use context deadlines for all operations
3. **Report meaningful metrics** - Include uptime, request counts, error rates
4. **Log to stderr** - MDEMG captures and logs module output
5. **Graceful shutdown** - Clean up resources when Shutdown is called
6. **Version your modules** - Use semantic versioning in manifest
