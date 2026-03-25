# Module Developer Tutorial

Build a working MDEMG plugin from scratch. This tutorial walks you from scaffolding
to a verified, running module in under 15 minutes. No prior plugin experience required.

---

## Table of Contents

1. [How Modules Work](#1-how-modules-work)
2. [Prerequisites](#2-prerequisites)
3. [Step 1: Scaffold Your Module](#3-step-1-scaffold-your-module)
4. [Step 2: Understand the Generated Code](#4-step-2-understand-the-generated-code)
5. [Step 3: Customize the Handler](#5-step-3-customize-the-handler)
6. [Step 4: Build the Module](#6-step-4-build-the-module)
7. [Step 5: Deploy and Verify](#7-step-5-deploy-and-verify)
8. [Step 6: Validate Your Module](#8-step-6-validate-your-module)
9. [Module Types Reference](#9-module-types-reference)
10. [Troubleshooting](#10-troubleshooting)

---

## 1. How Modules Work

MDEMG plugins are standalone executables that communicate with the server over
gRPC via Unix domain sockets. MDEMG spawns each module as a child process,
passes it a `--socket <path>` argument, then connects to that socket to issue
RPCs.

```
┌─────────┐   spawn + --socket   ┌─────────────┐
│  MDEMG  │ ──────────────────── │  Your Module │
│  Server │                      │  (binary)    │
│         │ ◄── gRPC (Unix) ──── │              │
└─────────┘   Handshake/Health   └─────────────┘
              Parse/Process/Exec
```

**Lifecycle**: Spawn → Handshake → Health Checks (periodic) → Service Calls → Shutdown

Three module types exist:

| Type | Purpose | Key RPCs |
|------|---------|----------|
| **INGESTION** | Convert external content into observations | `Matches`, `Parse`, `Sync` |
| **REASONING** | Re-rank/filter retrieval results mid-pipeline | `Process` |
| **APE** | Background tasks on schedule or events | `Execute`, `GetSchedule` |

---

## 2. Prerequisites

- **Go 1.21+** installed (`go version`)
- MDEMG repo cloned and buildable (`go build ./...`)
- MDEMG server running:

```bash
./bin/mdemg start --auto-migrate
curl -s http://localhost:9999/healthz
```

> **Note:** All plugins share the repo root `go.mod` (`module mdemg`). No per-plugin
> `go.mod` is needed — Go resolves imports by walking up to the root module.

---

## 3. Step 1: Scaffold Your Module

Generate the starter files with the scaffold CLI:

```bash
mdemg plugin scaffold --name "my-parser" --type INGESTION
```

The `--type` flag accepts: `INGESTION`, `REASONING`, or `APE`.

This creates:

```
plugins/my-parser/
  main.go         # gRPC server entrypoint (usually no edits needed)
  handler.go      # Your module logic — edit this
  manifest.json   # Module metadata
  Makefile        # build / clean / test / run targets
  README.md       # Module documentation
```

> **Note:** The scaffold derives the module ID from the name using kebab-case.
> `"my-parser"` becomes module ID `my-parser`.

---

## 4. Step 2: Understand the Generated Code

### main.go — The gRPC Server

This file creates the Unix socket listener, registers gRPC services, and handles
graceful shutdown. You generally do not edit `main.go`.

Key structure:

```go
func main() {
    socketPath := flag.String("socket", "", "Unix socket path")
    flag.Parse()

    // Create Unix socket listener
    listener, err := net.Listen("unix", *socketPath)
    // ...

    // Register services
    server := grpc.NewServer()
    handler := &MyParserHandler{}
    pb.RegisterModuleLifecycleServer(server, handler)   // Required for all modules
    pb.RegisterIngestionModuleServer(server, handler)    // INGESTION-specific

    // Handle SIGTERM/SIGINT for graceful shutdown
    // ...
    server.Serve(listener)
}
```

### handler.go — Your Logic

This is where you implement the module's behavior. It contains three groups of methods:

**Lifecycle RPCs** (required by every module type):
- `Handshake` — Returns your module's ID, version, type, and capabilities
- `HealthCheck` — Returns health status and metrics (called every 5s by default)
- `Shutdown` — Clean up resources before exit

**INGESTION RPCs** (for `--type INGESTION`):
- `Matches` — Does your module handle this source URI / content type?
- `Parse` — Convert raw content into observation nodes
- `Sync` — Stream observations from an external source incrementally

Look for `// TODO:` comments in the generated code — they mark where to add your logic.

### manifest.json

```json
{
  "id": "my-parser",
  "name": "My Parser",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "my-parser",
  "capabilities": {
    "ingestion_sources": ["custom://"],
    "content_types": ["text/plain"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000,
  "config": {}
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique module identifier (kebab-case) |
| `name` | Yes | Human-readable display name |
| `version` | Yes | Semantic version |
| `type` | Yes | `INGESTION`, `REASONING`, or `APE` |
| `binary` | Yes | Name of the executable (must match the built binary) |
| `capabilities` | Yes | What the module handles (type-specific) |
| `health_check_interval_ms` | No | Health check frequency (default: 5000) |
| `startup_timeout_ms` | No | Max time for handshake to complete (default: 10000) |
| `config` | No | Key-value pairs passed to the module during Handshake |

---

## 5. Step 3: Customize the Handler

The `plugins/echo-module/` directory contains a complete, working INGESTION module.
Use it as a reference while editing your `handler.go`.

### Customize Matches

`Matches` tells MDEMG whether your module can handle a given source. Return
`Matches: true` with a confidence score (0.0–1.0) for content you handle.

From `plugins/echo-module/main.go`:

```go
func (m *EchoModule) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
    matches := strings.HasPrefix(req.SourceUri, "echo://") || req.ContentType == "text/plain"
    confidence := float32(0.0)
    reason := "not an echo source"

    if matches {
        confidence = 1.0
        reason = "matches echo:// or text/plain"
    }

    return &pb.MatchResponse{
        Matches:    matches,
        Confidence: confidence,
        Reason:     reason,
    }, nil
}
```

Change `strings.HasPrefix(req.SourceUri, "echo://")` to match your source URI scheme.
Use `confidence < 1.0` for fallback handlers that should yield to more specific modules.

### Customize Parse

`Parse` converts raw content into one or more observation nodes.

```go
func (m *EchoModule) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
    obs := &pb.Observation{
        NodeId:      fmt.Sprintf("echo-%d", time.Now().UnixNano()),  // Unique ID
        Path:        req.SourceUri,                                   // Source URI
        Name:        "echo-observation",                              // Human label
        Content:     string(req.Content),                             // Raw text
        ContentType: req.ContentType,                                 // MIME type
        Tags:        []string{"echo", "test"},                        // For filtering
        Timestamp:   time.Now().Format(time.RFC3339),                 // ISO 8601
        Source:      "echo-module",                                   // Module ID
    }

    return &pb.ParseResponse{
        Observations: []*pb.Observation{obs},
        Metadata: map[string]string{
            "parsed_at": time.Now().Format(time.RFC3339),
            "bytes":     fmt.Sprintf("%d", len(req.Content)),
        },
    }, nil
}
```

You can return multiple observations from a single `Parse` call — useful for
chunking large documents into separate nodes.

### Customize Sync (optional)

Use `Parse` for on-demand conversion of known content. Use `Sync` when your
module needs to poll an external source incrementally using a cursor.

```go
func (m *EchoModule) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
    obs := &pb.Observation{
        NodeId:  fmt.Sprintf("echo-sync-%d", time.Now().UnixNano()),
        Path:    "echo://sync",
        Name:    "echo-sync-observation",
        Content: "sync test",
        Tags:    []string{"echo", "sync"},
    }

    return stream.Send(&pb.SyncResponse{
        Observations: []*pb.Observation{obs},
        Cursor:       "cursor-1",    // Opaque cursor for incremental sync
        HasMore:      false,         // Set true if more pages remain
        Stats:        &pb.SyncStats{ItemsProcessed: 1, ItemsCreated: 1},
    })
}
```

Set `HasMore: true` and update `Cursor` to support pagination. MDEMG will call
`Sync` again with the returned cursor.

---

## 6. Step 4: Build the Module

Build from the repo root (where `go.mod` lives):

```bash
go build -o plugins/my-parser/my-parser ./plugins/my-parser/
```

Or use the Makefile from the plugin directory:

```bash
cd plugins/my-parser && make build
```

> **Note:** `go build .` works from inside a plugin directory because Go walks
> up to find the root `go.mod`. The `-o` flag ensures the binary lands in the
> correct location.

The built binary name **must match** the `binary` field in `manifest.json`.

---

## 7. Step 5: Deploy and Verify

### Deploy

The binary is already in the correct location if you used the build command above.
Plugins are auto-discovered from the `plugins/` directory on server startup.

Your plugin directory must contain both files:

```
plugins/my-parser/
  manifest.json   ← required
  my-parser       ← required (matches manifest "binary" field)
```

### Start MDEMG

```bash
./bin/mdemg start --auto-migrate
```

If MDEMG is already running, restart to pick up new plugins:

```bash
./bin/mdemg restart
```

### Verify the Module Loaded

```bash
curl -s http://localhost:9999/v1/modules | jq .
```

Expected response:

```json
{
  "data": {
    "enabled": true,
    "modules": [
      {
        "id": "my-parser",
        "name": "My Parser",
        "version": "1.0.0",
        "type": "INGESTION",
        "state": "ready"
      }
    ]
  }
}
```

If `state` is `ready`, your module is running and MDEMG has completed the handshake.
If `state` is `starting` or `crashed`, see [Troubleshooting](#10-troubleshooting).

---

## 8. Step 6: Validate Your Module

The CLI includes a validation command that checks manifest correctness, proto
compliance, and lifecycle health:

```bash
# Full validation (manifest + proto + health + lifecycle)
mdemg plugin validate --plugin ./plugins/my-parser

# Manifest-only (fast, no running server needed)
mdemg plugin validate --plugin ./plugins/my-parser --manifest-only

# Verbose output
mdemg plugin validate --plugin ./plugins/my-parser --verbose
```

Use `--manifest-only` during development before the binary is built. Use full
validation to confirm the handshake works correctly.

---

## 9. Module Types Reference

### INGESTION Modules

Convert external content into MDEMG observations.

- Implements: `ModuleLifecycle` + `IngestionModule` (`Matches`, `Parse`, `Sync`)
- Scaffold: `mdemg plugin scaffold --name "X" --type INGESTION`
- Manifest capabilities: `ingestion_sources`, `content_types`
- Working example: `plugins/echo-module/`

### REASONING Modules

Re-rank or filter retrieval results after the scoring pipeline runs, before
LLM re-ranking.

- Implements: `ModuleLifecycle` + `ReasoningModule` (`Process`)
- Scaffold: `mdemg plugin scaffold --name "X" --type REASONING`
- `Process` receives: query text + scored candidates. Returns: re-scored/reordered candidates.
- Manifest capabilities: `pattern_detectors`
- Working example: `plugins/keyword-booster/`

### APE Modules

Background tasks that run on a cron schedule or in response to events (e.g.,
`session_end`, `consolidate`).

- Implements: `ModuleLifecycle` + `APEModule` (`Execute`, `GetSchedule`)
- Scaffold: `mdemg plugin scaffold --name "X" --type APE`
- `GetSchedule` returns: cron expression, event triggers, minimum interval
- `Execute` is called with a `trigger` field (`"schedule"`, `"event:session_end"`, etc.)
- Manifest capabilities: `event_triggers`
- Working example: `plugins/reflection-module/`

---

## 10. Troubleshooting

### "binary not found"

The binary name in `manifest.json` must match the actual file in the plugin directory.

```bash
ls -la plugins/my-parser/
# Verify the binary exists and name matches manifest.binary
```

### "handshake failed: connection refused"

The module crashed on startup. Run it standalone to see the error:

```bash
./plugins/my-parser/my-parser --socket /tmp/debug.sock
```

### "health check failed"

`HealthCheck` must return quickly. Avoid blocking I/O in the health check handler.
If your module needs initialization time, increase `startup_timeout_ms` in the manifest.

### Module state stays "starting"

The handshake is timing out. Increase `startup_timeout_ms` in `manifest.json`
(default: 10000ms).

### Module keeps restarting

MDEMG retries crashed modules up to 3 times with exponential backoff (2s, 4s, 6s).
Check the module's stderr output in the MDEMG server logs.

### Debug a module standalone

Run your module manually and probe it with grpcurl:

```bash
# Terminal 1: start the module
./plugins/my-parser/my-parser --socket /tmp/debug.sock

# Terminal 2: test RPCs
grpcurl -plaintext -unix /tmp/debug.sock mdemg.module.v1.ModuleLifecycle/HealthCheck
grpcurl -plaintext -unix /tmp/debug.sock mdemg.module.v1.ModuleLifecycle/Handshake
```

---

## Further Reading

- [SDK Plugin Guide](../development/SDK_PLUGIN_GUIDE.md) — Full API reference for all RPCs
- [Module Development Guide](../development/MODULE_DEVELOPMENT_GUIDE.md) — Detailed protocol reference
- [Plugin Triggers](../features/plugin-triggers.md) — Event subscription system
- [Echo Module](../../plugins/echo-module/) — Complete working INGESTION example
