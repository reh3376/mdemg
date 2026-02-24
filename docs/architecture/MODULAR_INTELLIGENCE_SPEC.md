# MDEMG Modular Intelligence Specification

## 1. Overview

This document defines the **MDEMG Module System**, a plug-and-play architecture that transforms MDEMG from a passive memory store into a modular, skill-based cognitive substrate. Modules allow MDEMG to acquire domain-specific parsing skills, architectural reasoning patterns, and proactive reflection capabilities.

---

## 2. Module Definition (The `mdemg-module.json`)

Every module is defined by a manifest that specifies its hooks into the MDEMG lifecycle.

```json
{
  "id": "nestjs-architecture-module",
  "name": "NestJS Architecture SME",
  "version": "1.0.0",
  "type": "REASONING",
  "capabilities": {
    "ingestion_parsers": [".module.ts", ".controller.ts", ".service.ts"],
    "pattern_detectors": ["DependencyInjection", "GuardAuth", "InterceptorLog"],
    "reflection_strategies": ["ModuleDependencyGraph"]
  },
  "subscriptions": ["codebase-ingest", "session-end"],
  "config_schema": {
    "strict_dependency_check": "boolean",
    "generate_diagrams": "boolean"
  }
}
```

---

## 3. Module Types & Go Interfaces

To ensure technical depth and type safety while avoiding the fragility of native Go `plugin` binaries, MDEMG utilizes a **gRPC-based Sidecar Architecture**. This allows modules to be written in any language and avoids "dependency hell."

### A. Ingestion Modules (Perception)

* **Deliverable**: `internal/modules/ingest/`
* **GRPC Service Definition**:

```proto
service IngestionModule {
    rpc Matches(MatchRequest) returns (MatchResponse);
    rpc Parse(ParseRequest) returns (ParseResponse);
}
```

* **General Examples (Non-Code)**:
  * **P&ID Module**: Parses industrial process diagrams (XML/SVG).
  * **Linear Module**: High-velocity task history and engineering decision chains.
  * **Obsidian Module**: Unstructured second-brain notes, research fragments, and personal knowledge graphs.
  * **Conversation Module**: Extracts specific commitments from Slack/Discord logs.

### B. Reasoning Modules (Cognition)

* **Deliverable**: `internal/modules/reasoning/`
* **GRPC Service Definition**:

```proto
service ReasoningModule {
    rpc Process(ReasonRequest) returns (ReasonResponse);
}
```

* **General Examples (Non-Code)**:
  * **Strategic Consistency Module**: Cross-references new decisions against "Company North Star" principles.
  * **Logical Conflict Module**: Identifies when a new observation contradicts a 1-year-old "Ground Truth" node.

### C. Active Participant Modules (APE)

* **Deliverable**: `internal/modules/ape/`
* **GRPC Service Definition**:

```proto
service APEModule {
    rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

---

## 4. Detailed Technical Implementation Path

### Binary Sidecar Architecture

MDEMG uses **binary executables** (not Docker containers) for modules to minimize latency. Each module is a standalone binary that MDEMG spawns and communicates with via gRPC over Unix domain sockets.

```
mdemg-server (main process)
    │
    ├── Unix Socket: /tmp/mdemg-linear.sock
    │   └── linear-module (binary)
    │
    ├── Unix Socket: /tmp/mdemg-nestjs.sock
    │   └── nestjs-module (binary)
    │
    └── Unix Socket: /tmp/mdemg-obsidian.sock
        └── obsidian-module (binary)
```

**Why Binary over Docker:**

* ~10ms savings per RPC call (no network stack)
* Simpler deployment (single binary)
* Direct process management (signals, stdio)
* Better debugging (attach debugger directly)

**Trade-offs:**

* Must compile for target platform
* Dependency management is module's responsibility
* Need explicit build/release process per module

### Phase 1: Plugin Host (Discovery & Lifecycle)

#### 1.1 Plugin Directory Structure

```
/plugins/
├── linear-module/
│   ├── manifest.json        # Module metadata
│   ├── linear-module        # Binary executable
│   └── linear-module.exe    # Windows variant (optional)
├── nestjs-module/
│   ├── manifest.json
│   └── nestjs-module
└── .disabled/               # Disabled modules moved here
```

#### 1.2 Manifest Schema

```json
{
  "id": "linear-module",
  "name": "Linear Engineering Tasks",
  "version": "1.0.0",
  "type": "INGESTION",
  "binary": "linear-module",
  "capabilities": {
    "ingestion_sources": ["linear-api"],
    "content_types": ["task", "issue", "project"]
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000
}
```

#### 1.3 Module Lifecycle Proto

```proto
service ModuleLifecycle {
    // Called immediately after spawn to verify module is ready
    rpc Handshake(HandshakeRequest) returns (HandshakeResponse);

    // Periodic health check (every health_check_interval_ms)
    rpc HealthCheck(Empty) returns (HealthResponse);

    // Graceful shutdown signal
    rpc Shutdown(ShutdownRequest) returns (Empty);
}

message HandshakeRequest {
    string mdemg_version = 1;
    string socket_path = 2;
}

message HandshakeResponse {
    string module_id = 1;
    string module_version = 2;
    repeated string capabilities = 3;
    bool ready = 4;
}

message HealthResponse {
    bool healthy = 1;
    string status = 2;
    map<string, string> metrics = 3;
}
```

#### 1.4 Plugin Manager Responsibilities

1. **Scan** `/plugins` directory on startup
2. **Validate** manifest.json for each module
3. **Spawn** binary with Unix socket path as argument
4. **Handshake** to verify module is ready
5. **Monitor** health via periodic HealthCheck RPCs
6. **Restart** crashed modules (with backoff)
7. **Route** requests to appropriate module based on capabilities

### Phase 2: Refactoring Ingest & Retrieval for RPC

1. **Ingest Hook**: Instead of hardcoded Go parsers, `IngestObservation` calls the `IngestionModule.Parse` RPC for all registered modules.
2. **Retrieval Hook**: Refactor the retrieval pipeline to allow `ReasoningModule.Process` to run between the **Spreading Activation** and **Final Top-K** steps.

### Phase 2: Refactoring Ingest & Retrieval for RPC

1. **Ingest Hook**: Instead of hardcoded Go parsers, `IngestObservation` calls the `IngestionModule.Parse` RPC for all registered modules.
2. **Retrieval Hook**: Refactor the retrieval pipeline to allow `ReasoningModule.Process` to run between the **Spreading Activation** and **Final Top-K** steps.

### Phase 3: The "General Intelligence" Baseline

To prove MDEMG isn't just for code, we will implement the **"Internal Dialog Baseline"**:

* **Reflection Module**: A module that takes the last 10 observations (regardless of source) and generates a "What I learned today" node.
* **Commitment Module**: A module that extracts "IF/THEN" rules from text observations and creates specific `CONSTRAINT` nodes.

---

## 5. Active Participant Engine (APE) Detail

The **APE** orchestrates the lifecycle of the memory graph.

### Key Components

1. **Consistency Guard**: Background worker implementing `APEModule`. Prunes weak edges and ensures hierarchical integrity.
2. **LLM Reconciler**: Distinct from re-ranking. While re-ranking sorts results for a query, the **Reconciler** runs as an `APEModule` to resolve contradictory observations (e.g., if two agents report different versions of the same architectural decision).
3. **Context Cooler (Short-Term Buffer)**:
    * **Mechanism**: Observations are initially written with a `volatile: true` flag and a high `co_activation` weight.
    * **Graduation**: After a "cooling period" (e.g., 2 hours of inactivity), the APE runs a "graduation" task that removes the volatile flag, normalizes weights, and triggers consolidation.

---

## 5. Jiminy: Explainable Retrieval

**Jiminy** is MDEMG's "conscience" layer - it explains *why* specific memories were retrieved and how confident the system is in each result. Named after the character who guides and explains.

### Purpose

* Provide transparency into retrieval decisions
* Enable debugging of poor retrieval results
* Build trust by showing reasoning, not just results
* Allow modules to contribute explanations

### Deliverable: `RetrieveResponse` Extension

```json
{
  "data": [
    {
      "node_id": "auth_service_01",
      "jiminy": {
        "rationale": "Direct semantic match for 'authentication' + strongly linked to 'JWT' via CO_ACTIVATED_WITH (weight 0.85)",
        "confidence": 0.92,
        "retrieval_path": ["vector_recall", "spreading_activation", "llm_rerank"],
        "contributing_modules": ["nestjs-architecture-module"],
        "score_breakdown": {
          "vector_similarity": 0.78,
          "activation": 0.65,
          "rerank_boost": 0.12,
          "learning_edge_boost": 0.08
        }
      }
    }
  ]
}
```

### Go Interface

```go
type JiminyExplanation struct {
    Rationale           string            `json:"rationale"`
    Confidence          float64           `json:"confidence"`
    RetrievalPath       []string          `json:"retrieval_path"`
    ContributingModules []string          `json:"contributing_modules,omitempty"`
    ScoreBreakdown      map[string]float64 `json:"score_breakdown"`
}
```

---

## 6. Implementation Roadmap

### Phase 6.1: Ingestion Plugin Architecture ✅ COMPLETE

* ✅ Binary sidecar architecture with gRPC over Unix sockets
* ✅ Plugin Manager scans `/plugins` directory
* ✅ Manifest-based module configuration
* ✅ Health checks with auto-restart (max 3 attempts, exponential backoff)

### Phase 6.2: Module Manifest & Registry ✅ COMPLETE

* ✅ `GET /v1/modules` API lists all loaded modules with status
* ✅ `POST /v1/modules/{id}/sync` triggers ingestion module sync
* ✅ Module state tracked in memory (not Neo4j - avoids DB coupling)

### Phase 6.3: Active Participant Orchestrator ✅ COMPLETE

* ✅ APE Scheduler (`internal/ape/scheduler.go`) with cron support
* ✅ Event-triggered execution (`session_end`, `consolidate`, etc.)
* ✅ `GET /v1/ape/status` and `POST /v1/ape/trigger` APIs
* ✅ Sample `reflection-module` demonstrating APE pattern

### Phase 6.4: Reasoning Pipeline Integration ✅ COMPLETE

* ✅ `ReasoningProvider` interface (`internal/retrieval/reasoning.go`)
* ✅ `PluginReasoningProvider` wraps plugin manager
* ✅ Reasoning modules called after initial scoring, before LLM rerank
* ✅ Sample `keyword-booster` module demonstrating pattern

### Production Modules Implemented

| Module | Type | Status |
|--------|------|--------|
| `echo-module` | INGESTION | ✅ Test module |
| `linear-module` | INGESTION | ✅ Full Linear API integration |
| `keyword-booster` | REASONING | ✅ Query keyword boosting |
| `reflection-module` | APE | ✅ Session reflection |
