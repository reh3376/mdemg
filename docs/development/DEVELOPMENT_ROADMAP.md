# MDEMG Development Roadmap

**Created**: 2026-01-22
**Updated**: 2026-02-23 (All phases complete through Phase 91. Phase 92 gap analysis complete — Phases 93-100 roadmap for deployable MDEMG package. Phase D validated. Space Pruning Framework + auto-prune scheduler. Security hardening complete: gosec, gitleaks, error sanitization. 113 UATS specs, 190 variants, 100% passing. 22 RSIC integration tests. CI gated on non-embedding specs.)
**Based on**: v4 Test Results (whk-wms codebase, 100-question evaluation)
**Goal**: Improve retrieval quality from 0.567 avg score to 0.70+ avg score
**Result**: v11 achieved 0.733, Edge Attention achieved **0.898 avg score** (+58.4% from v4 baseline, 100% high-score rate)

---

## Executive Summary

The v4 test validated MDEMG's core hypothesis (100% completion vs 0% baseline), but revealed specific retrieval weaknesses:

- Cross-cutting concerns: 0.45 → **0.709** (+57%) via ConcernNodes ✅
- Architecture questions: 0.50 → **0.750** (+50%) via ComparisonNodes ✅
- Configuration/constants: 0.45 → **0.719** (+60%) via ConfigNodes ✅
- Temporal patterns: 0.46 → **0.728** (+58%) via TemporalNodes ✅
- Learning edges: 0 created → **8,748 edges** via Hebbian learning ✅

This roadmap addressed these gaps through 5 improvement tracks. **All 5 tracks are now complete and validated.** Detailed metrics and the evolution of these improvements are documented in the [Up-to-Date Benchmark Summary](tests/UP_TO_DATE_BENCHMARK_SUMMARY.md).

---

## Priority Matrix

| Track | Impact | Effort | Priority | Status |
|-------|--------|--------|----------|--------|
| Edge Strengthening (Learning) | HIGH | MEDIUM | P0 | ✅ COMPLETE |
| Cross-Cutting Concern Nodes | HIGH | MEDIUM | P1 | ✅ COMPLETE |
| Architectural Comparison Nodes | MEDIUM | MEDIUM | P2 | ✅ COMPLETE |
| Configuration Boosting | MEDIUM | LOW | P2 | ✅ COMPLETE |
| Temporal Pattern Detection | LOW | MEDIUM | P3 | ✅ COMPLETE |
| **Symbol-Level Indexing** | **HIGH** | **HIGH** | **P1** | ⏳ VALIDATING |
| **Incremental Updates** | **MEDIUM** | **MEDIUM** | **P2** | ✅ COMPLETE (git hooks, scheduled sync, file-level re-ingest, plugin triggers, orphan detection) |
| **Query Optimization & Caching** | **HIGH** | **MEDIUM** | **P1** | ✅ COMPLETE (caching + profiling + indexes) |
| **LLM Plugin SDK** | **HIGH** | **MEDIUM** | **P1** | ✅ COMPLETE (SDK docs, semantic summaries, scaffolding, validation, creation API, gap detection) |

---

## Track 1: Edge Strengthening (Learning Edges) ✅ COMPLETE

**Priority**: P0 | **Status**: COMPLETE (v10)

### Problem (Resolved)

`co_activated_edges` remained at 0 during the entire test. The learning mechanism existed but wasn't creating edges.

### Root Cause (Identified)

Only top 2 candidates were seeded with activation values in `activation.go`. Combined with sparse graph connectivity, most returned nodes had zero activation and failed the 0.20 learning threshold.

### Fix Applied

Modified `activation.go` to seed ALL candidates with their VectorSim values:

```go
// Before: only top 2 seeded
// After: all candidates seeded
for _, c := range cands {
    act[c.NodeID] = c.VectorSim
}
```

### Results

| Metric | Before Fix | After Fix | Improvement |
|--------|------------|-----------|-------------|
| Pairs per query | ~0.8 | **43.1** | **54x faster** |
| Edges (cold start) | 0 | **8,622** | ✅ |
| Avg Score | 0.619 | **0.710** | **+14.6%** |

### Documentation

- See `docs/LEARNING_EDGES_ANALYSIS.md` for full diagnosis
- Commit: `3fc2136` - fix(learning): seed all candidates for Hebbian learning activation

---

## Track 2: Cross-Cutting Concern Nodes ✅ COMPLETE

**Priority**: P1 | **Status**: COMPLETE

### Problem (Resolved)

Questions about ACL, RBAC, authentication, and error-handling scored 0.45-0.46 (below average 0.567).

### Root Cause (Addressed)

Cross-cutting concerns span multiple files/modules but weren't linked in the graph. Individual file embeddings didn't capture the "concern" abstraction.

### Implementation (Completed)

#### 2.1 Concern Detection During Ingestion ✅

**File**: `cmd/ingest-codebase/main.go`

- ✅ Pattern-based detection for cross-cutting concerns:
  - `concernPatterns` map detects: authentication, authorization, error-handling, validation, logging, caching
  - File path patterns: `*auth*`, `*acl*`, `*permission*`, `*error*`, etc.
  - Content analysis with 2+ pattern match threshold
- ✅ NestJS decorator detection:
  - `@Guard`, `@UseGuards` → authorization
  - `@Interceptor`, `@UseInterceptors` → cross-cutting
  - `@Filter`, `@UseFilters`, `@Catch` → error-handling
- ✅ Tags with "concern:" prefix (e.g., "concern:authentication")

#### 2.2 Concern Node Creation During Consolidation ✅

**File**: `internal/hidden/service.go`

- ✅ `CreateConcernNodes()` creates dedicated nodes with `role_type: 'concern'`
- ✅ Called automatically from `RunConsolidation()` after hidden node creation
- ✅ Computes centroid embedding from all implementing nodes
- ✅ Auto-generates summaries: "Cross-cutting concern: {type} ({count} implementations)"

#### 2.3 Edge Types ✅

- ✅ `IMPLEMENTS_CONCERN` edge: (base_node)-[:IMPLEMENTS_CONCERN]->(concern_node)
  - Properties: space_id, edge_id, weight, concern_type, created_at, updated_at
- ⚠️ `SHARES_CONCERN` edge: Deferred (not critical for retrieval improvement)

### Results

- Concern nodes are created during consolidation for any `concern:*` tags detected
- Base nodes with same concern are linked to shared ConcernNode
- Retrieval can now return concern nodes as first-class results

---

## Track 3: Architectural Comparison Nodes ✅ COMPLETE

**Priority**: P2 | **Status**: COMPLETE

### Problem (Resolved)

Questions like "What is the purpose of having both DeltaSyncModule and SyncModule?" scored lower because understanding requires comparing two entities.

### Root Cause (Addressed)

Individual file embeddings didn't capture "this module is an alternative to / complement of that module" relationships.

### Implementation (Completed)

#### 3.1 Similar Module Detection ✅

**File**: `internal/hidden/service.go`

- ✅ `fetchModuleNodes()` finds nodes matching Module, Service, Controller, Provider, Handler, Manager patterns
- ✅ `groupSimilarModules()` groups by naming patterns (e.g., SyncModule vs DeltaSyncModule)
- ✅ Embedding similarity considered for grouping

#### 3.2 Comparison Node Generation ✅

- ✅ `CreateComparisonNodes()` creates comparison nodes with `role_type: 'comparison'`
- ✅ `COMPARED_IN` edges link similar modules: `(ModuleA)-[:COMPARED_IN]->(comparison)<-[:COMPARED_IN]-(ModuleB)`
- ✅ Auto-generates summaries listing compared modules

#### 3.3 Edge Types ✅

- ✅ `COMPARED_IN` - links modules to their comparison node
- ⚠️ `ALTERNATIVE_TO`, `COMPLEMENTS`, `EXTENDS` - deferred (basic comparison sufficient)

---

## Track 4: Configuration Boosting ✅ COMPLETE

**Priority**: P2 | **Status**: COMPLETE

### Problem (Resolved)

Questions about "runtime-config" and "system configurations" scored below average.

### Root Cause (Addressed)

Configuration files are often small, with terse content that doesn't embed as distinctively as implementation code.

### Implementation (Completed)

#### 4.1 Configuration Detection ✅

**File**: `cmd/ingest-codebase/main.go`

- ✅ `configFilePatterns` detects: `*.config.ts`, `*.config.js`, `config.json/yaml`, etc.
- ✅ `configDirPatterns` detects: `/config/`, `/configuration/`, `/configs/`, `/settings/`
- ✅ `isConfigFile()` checks file patterns, directory patterns, and Config suffix
- ✅ `.env.example`, `.env.sample`, `.env.template` files parsed
- ✅ Environment variables extracted (keys only, not values for security)
- ✅ Tagged with "config" tag

#### 4.2 Configuration Boosting ⚠️ PARTIAL

- ⚠️ Score boost not implemented in retrieval (config nodes help instead)
- ✅ Config files are tagged and linked to config summary node

#### 4.3 Configuration Node Creation ✅

**File**: `internal/hidden/service.go`

- ✅ `CreateConfigNodes()` creates dedicated node with `role_type: 'config'`
- ✅ `IMPLEMENTS_CONFIG` edge links config files to summary node
- ✅ Auto-categorizes: docker, environment, package, app-config
- ✅ Generates summary: "Configuration summary: N config files. Categories: ..."

#### 4.4 Environment Variable Extraction ✅

- ✅ `parseEnvFile()` extracts variable names from `.env.*` files
- ✅ Variables listed in element content and summary
- ✅ Comments extracted as documentation

---

## Track 5: Temporal Pattern Detection ✅ COMPLETE

**Priority**: P3 | **Status**: COMPLETE

### Problem (Resolved)

Questions about `validFrom/validTo` temporal patterns scored ~0.46.

### Root Cause (Addressed)

Temporal modeling patterns are domain-specific and weren't recognized as a cross-cutting concern.

### Implementation (Completed)

#### 5.1 Temporal Pattern Recognition ✅

**File**: `cmd/ingest-codebase/main.go`

- ✅ Added "temporal" to `concernPatterns` map with comprehensive patterns:
  - `validfrom`, `validto`, `valid_from`, `valid_to` - validity periods
  - `effectivedate`, `expirationdate` - effective dates
  - `createdat`, `updatedat`, `deletedat` - timestamps and soft delete
  - `softdelete`, `paranoid` - deletion patterns
  - `temporal`, `bitemporal`, `versioned` - temporal modeling
  - `daterange`, `historiz`, `audit_trail`, `snapshot` - history patterns
- ✅ Tagged with `concern:temporal`

#### 5.2 Temporal Pattern Edge ✅

**File**: `internal/hidden/service.go`

- ✅ `SHARES_TEMPORAL_PATTERN` edge type created
- ✅ Links all temporal entities to shared temporal pattern node

#### 5.3 Temporal Documentation Node ✅

- ✅ `CreateTemporalNodes()` creates summary node with `role_type: 'temporal'`
- ✅ Auto-detects pattern categories: validity-period, effective-dates, soft-delete, versioning, audit-history, timestamps, date-ranges
- ✅ Generates rich summary describing patterns in codebase
- ✅ Called from `RunConsolidation()` (Step 1e)

---

## Implementation Phases

### Phase A: Foundation ✅ COMPLETE

- [x] Track 1: Debug and enable learning edges
- [x] Track 4.1-4.2: Configuration detection and boosting
- [x] v10 test showed 14.6% improvement (0.567 → 0.710)

### Phase B: Concern Nodes ✅ COMPLETE

- [x] Track 2: Full cross-cutting concern implementation
- [x] Track 3.1: Similar module detection
- [x] ConcernNodes and ComparisonNodes created during consolidation

### Phase C: Advanced Relationships ✅ COMPLETE

- [x] Track 3.2-3.3: Comparison nodes and edge types
- [x] Track 5: Temporal pattern detection
- [x] Run validation test with full improvements (v11: 0.733 avg, +3.3% over v10)

### Phase D: Validation ✅ COMPLETE

- [x] Benchmark on second codebase (different domain) — plc-gbt (PLC Control Loop), 0.724 avg score. See `docs/archive/benchmarks/plc-gbt/BENCHMARK_SUMMARY.md`
- [x] Scale test: 10K → 100K nodes — VS Code repo, 28K nodes, linear ingestion scaling, 50ms warm latency. See `docs/architecture/benchmarks/SCALE_TEST_RESULTS.md`
- [x] Document final architecture — 14 architecture docs in `docs/architecture/` (00-14)

---

## Phase 6: Modular Intelligence & Active Participation

This phase transforms MDEMG into a plug-and-play cognitive engine. **Tracks 1-5 (Phases A-C) are foundational** and should proceed in parallel or prior to Phase 6.

### Deliverable 6.1: Jiminy (Explainable Retrieval) ✅ COMPLETE

- **Priority**: P0 | **Status**: COMPLETE
- [x] Update `/v1/memory/retrieve` to return `jiminy` block with rationale and confidence.
- [x] Trace retrieval path (Vector → Spreading Activation → LLM Rerank) for explanation.
- [x] Add `score_breakdown` showing contribution from each scoring component.
- [x] **Integration**: Connect with the v9 LLM re-ranker to explain *why* specific results were promoted.

### Deliverable 6.2: Binary Sidecar Host (Plugin Manager) ✅ COMPLETE

- **Priority**: P1 | **Status**: COMPLETE
- [x] Finalize `mdemg-module.proto` with lifecycle RPCs (Handshake, HealthCheck, Shutdown).
- [x] Implement **Plugin Manager** in Go:
  - Scan `/plugins` directory for module folders.
  - Parse `manifest.json` for each module.
  - Spawn binaries with Unix socket paths.
  - Maintain health check loops and restart crashed modules with backoff.
- [x] Create "echo" test module to validate RPC round-trip latency.
- [x] Wire REASONING modules into retrieval pipeline (`internal/retrieval/reasoning.go`)
- [x] Create sample `keyword-booster` reasoning module

**Documentation**: See [SDK Plugin Guide](SDK_PLUGIN_GUIDE.md) for complete plugin development documentation including:

- Module types (INGESTION, REASONING, APE) with full code templates
- manifest.json schema and configuration
- gRPC lifecycle management
- Best practices and troubleshooting

**Claude Skill**: See `.claude/skills/create-plugin.md` for LLM-assisted plugin creation workflow.

### Deliverable 6.3: Code Parser Module Migration

- **Priority**: P1 | **Effort**: 3-4 days
- [ ] Extract existing Go/TS/Python parsers into a standalone **Code Perception Module**.
- [ ] Refactor `ingest-codebase` to delegate parsing to the RPC layer.
- [ ] Benchmark: RPC overhead vs direct call (target: <5ms added latency).

### Deliverable 6.4: Non-Code Integration Modules ✅ PARTIAL

- **Priority**: P2 | **Status**: Linear CRUD COMPLETE (Phase 4), Obsidian pending
- [x] Implement **Linear Module** (Go binary) for engineering tasks.
  - Full sync of teams, projects, and issues
  - Streaming gRPC with batch ingestion
  - Incremental sync via cursor
  - **Phase 4**: Full CRUD operations (create/read/update/delete issues, projects, comments)
  - **Phase 4**: CRUDModule protobuf service with generic entity_type dispatch
  - **Phase 4**: REST API endpoints (`/v1/linear/issues`, `/v1/linear/projects`, `/v1/linear/comments`)
  - **Phase 4**: 6 MCP tools for IDE integration
  - **Phase 4**: Config-driven workflow engine (YAML triggers, conditions, actions)
- [ ] Implement **Obsidian Module** (Go binary) for SME notes.
- [ ] Define non-code specific edge types (e.g., `BLOCKS`, `ASSIGNED_TO`).

### Deliverable 6.5: Active Participant Engine (APE) ✅ COMPLETE

- **Priority**: P2 | **Status**: COMPLETE
- [x] Implement APE Scheduler (`internal/ape/scheduler.go`)
  - Cron-based scheduling
  - Event-triggered execution
  - Minimum interval enforcement
  - API: `GET /v1/ape/status`, `POST /v1/ape/trigger`
- [x] Implement **Reflection Module** (`plugins/reflection-module/`)
  - Subscribes to `session_end`, `consolidate` events
  - Hourly scheduled execution
- [x] Implement **Context Cooler** (`internal/conversation/cooler.go`, 440 lines, 6 tests): Graduation, decay, tombstoning, reinforcement. Config: 5 `COOLER_*` env vars.
- [x] Implement **Constraint Module** (`internal/conversation/constraint_detector.go`, 191 lines, 5 tests): 5 constraint types (must, must_not, should, should_not, deadline), auto-tagging, cooler integration. Config: 3 `CONSTRAINT_*` env vars.

## Phase 8: Symbol-Level Indexing (Evidence-Locked Retrieval)

**Motivation**: VS Code scale benchmark (28K elements) revealed that MDEMG returns *related files* but not *exact constant definitions*. For evidence-locked questions requiring specific values (e.g., "What is DEFAULT_FLUSH_INTERVAL?"), grep achieves 100% while MDEMG achieves ~20%.

**Goal**: Enable MDEMG to return symbol-level evidence, not just file paths.

---

### Graph Architecture for Symbols

```
TapRoot (space_id: "vscode-scale")
    │
    ├── MemoryNode (file: "storage.ts", layer: 0)
    │       │ properties: path, name, summary, embedding[1536]
    │       │
    │       └── DEFINED_IN ←── SymbolNode (const: DEFAULT_FLUSH_INTERVAL)
    │                           │ properties: name, type, value, line_number
    │                           │ embedding[1536] (from name + doc_comment)
    │                           │
    │                           └── REFERENCES ──→ SymbolNode (StorageService)
    │
    └── MemoryNode (file: "sidebarPart.ts", layer: 0)
            │
            └── DEFINED_IN ←── SymbolNode (property: minimumWidth)
                                properties: name="minimumWidth", value="170"
```

**Key Integration Points**:

1. `cmd/ingest-codebase` - Add symbol extraction step after file processing
2. `internal/retrieval/service.go` - Add symbol search to Retrieve()
3. `internal/models/models.go` - Add Symbol types to request/response
4. `api/proto/mdemg-module.proto` - Extend Observation with symbol fields

---

**Benchmark Evidence**:

| Query | MDEMG Returns | Ground Truth | Gap |
|-------|---------------|--------------|-----|
| DEFAULT_FLUSH_INTERVAL | `/src/vs/base/node/pfs.ts` | `/src/vs/platform/storage/common/storage.ts` | Wrong file |
| minimumWidth SidebarPart | `/src/vs/workbench/browser/parts/paneCompositeBar.ts` | `/src/vs/workbench/browser/parts/sidebar/sidebarPart.ts` | Related, not defining |
| quickSuggestionsDelay | Correct file | Correct file, wrong value (100 vs 10) | No value extraction |

---

### Deliverable 8.1: Symbol Parser Infrastructure

**Priority**: P0 | **Effort**: 3-4 days | **Status**: ✅ COMPLETE

#### 8.1.1 Tree-Sitter Integration

- [x] Add `github.com/smacker/go-tree-sitter` dependency
- [x] Add language grammars: TypeScript, JavaScript, Go, Python (Rust pending)
- [x] Create `internal/symbols/parser.go` with unified `ParseFile(path, lang) []Symbol` interface
- [x] Create `internal/symbols/types.go` with Symbol, FileSymbols, ParserConfig types
- [x] Handle parse errors gracefully (skip unparseable files, log warnings)
- [x] Expression evaluation: "60 * 1000" → "60000"
- [x] Doc comment extraction (JSDoc, GoDoc, docstrings)
- [x] **Tested**: 451 symbols from 49 Go files in MDEMG codebase

#### 8.1.2 Symbol Types to Extract

| Language | Symbol Types | Example |
|----------|-------------|---------|
| **TypeScript/JS** | `const`, `let`, `function`, `class`, `interface`, `type`, `enum` | `export const DEFAULT_TIMEOUT = 5000` |
| **Go** | `const`, `var`, `func`, `type`, `struct` | `const MaxRetries = 3` |
| **Python** | `CONSTANT`, `def`, `class`, `TypeAlias` | `MAX_CONNECTIONS = 100` |
| **Rust** | `const`, `static`, `fn`, `struct`, `enum`, `trait` | `pub const BUFFER_SIZE: usize = 4096` |

#### 8.1.3 Symbol Data Structure

```go
type Symbol struct {
    Name       string   // "DEFAULT_FLUSH_INTERVAL"
    Type       string   // "const", "function", "class", "interface", "enum"
    Value      string   // "60000" (for constants), "" for functions/classes
    FilePath   string   // "src/vs/platform/storage/common/storage.ts"
    LineNumber int      // 42
    EndLine    int      // 42 (or 50 for multi-line)
    Exported   bool     // true if public/exported
    DocComment string   // JSDoc/GoDoc comment above symbol
    Signature  string   // For functions: "(ctx context.Context, id string) error"
    Parent     string   // For class members: "StorageService"
}
```

#### 8.1.4 Extraction Rules

**Constants (highest priority for evidence-locked)**:

```
TypeScript: const X = <value>; | export const X = <value>;
Go:         const X = <value> | const X <type> = <value>
Python:     X = <value> (UPPER_CASE naming convention)
```

**Functions**:

```
TypeScript: function X() | export function X() | const X = () =>
Go:         func X() | func (r *Receiver) X()
Python:     def x():
```

**Classes/Interfaces/Types**:

```
TypeScript: class X | interface X | type X =
Go:         type X struct | type X interface
Python:     class X:
```

---

### Deliverable 8.2: Symbol Storage Schema

**Priority**: P0 | **Effort**: 1-2 days | **Status**: ✅ COMPLETE

#### 8.2.1 Neo4j Schema (V0007 Migration)

```cypher
// New label for symbol nodes
// :SymbolNode - extracted code symbols with values

// Constraints
CREATE CONSTRAINT symbol_node_id IF NOT EXISTS
FOR (s:SymbolNode) REQUIRE s.node_id IS UNIQUE;

CREATE CONSTRAINT symbol_space_path IF NOT EXISTS
FOR (s:SymbolNode) REQUIRE (s.space_id, s.file_path, s.name, s.line_number) IS UNIQUE;

// Indexes for fast lookup
CREATE INDEX symbol_name_idx IF NOT EXISTS
FOR (s:SymbolNode) ON (s.name);

CREATE INDEX symbol_type_idx IF NOT EXISTS
FOR (s:SymbolNode) ON (s.symbol_type);

CREATE INDEX symbol_file_idx IF NOT EXISTS
FOR (s:SymbolNode) ON (s.file_path);

// Fulltext index for fuzzy symbol search
CREATE FULLTEXT INDEX symbol_name_fulltext IF NOT EXISTS
FOR (s:SymbolNode) ON EACH [s.name, s.doc_comment];

// Vector index for symbol embeddings (semantic search on doc comments)
CREATE VECTOR INDEX symbolEmbedding IF NOT EXISTS
FOR (s:SymbolNode) ON (s.embedding)
OPTIONS {indexConfig: {`vector.dimensions`: 1536, `vector.similarity_function`: 'cosine'}};
```

#### 8.2.2 SymbolNode Properties

| Property | Type | Description |
|----------|------|-------------|
| `node_id` | string | UUID |
| `space_id` | string | Memory space |
| `name` | string | Symbol name (e.g., "DEFAULT_FLUSH_INTERVAL") |
| `symbol_type` | string | "const", "function", "class", "interface", "enum", "type" |
| `value` | string | Literal value for constants, "" otherwise |
| `file_path` | string | Relative path from repo root |
| `line_number` | int | Start line (1-indexed) |
| `end_line` | int | End line |
| `exported` | bool | Public/exported symbol |
| `doc_comment` | string | Documentation comment |
| `signature` | string | Function signature |
| `parent` | string | Parent class/module name |
| `embedding` | float[] | Vector embedding of name + doc_comment |
| `snippet` | string | Source code snippet (definition + 2 lines context) |
| `created_at` | datetime | Creation timestamp |
| `updated_at` | datetime | Last update timestamp |

#### 8.2.3 Edge Types

| Edge | Direction | Description |
|------|-----------|-------------|
| `DEFINED_IN` | `(SymbolNode)-[:DEFINED_IN]->(MemoryNode)` | Symbol is defined in file |
| `REFERENCES` | `(SymbolNode)-[:REFERENCES]->(SymbolNode)` | Symbol references another (future) |
| `MEMBER_OF` | `(SymbolNode)-[:MEMBER_OF]->(SymbolNode)` | Method/property belongs to class |

---

### Deliverable 8.3: Ingestion Pipeline Integration

**Priority**: P1 | **Effort**: 2-3 days | **Status**: ✅ COMPLETE

#### 8.3.1 Ingest Flow Update

```
Current:  File → Summarize → Embed → Create MemoryNode
New:      File → Summarize → Embed → Create MemoryNode
                    ↓
               Parse AST → Extract Symbols → Create SymbolNodes
                                                    ↓
                                            Link DEFINED_IN edges
```

#### 8.3.2 Configuration

```bash
# New environment variables
SYMBOL_EXTRACTION_ENABLED=true          # Enable/disable symbol extraction
SYMBOL_EXTRACTION_LANGUAGES=ts,js,go,py # Comma-separated language list
SYMBOL_EXTRACTION_MAX_PER_FILE=500      # Limit symbols per file (prevent bloat)
SYMBOL_EXTRACTION_MIN_NAME_LENGTH=2     # Skip single-char symbols
SYMBOL_EXTRACT_PRIVATE=false            # Include non-exported symbols
SYMBOL_EMBED_DOC_COMMENTS=true          # Generate embeddings for doc comments
```

#### 8.3.3 Batch Processing ✅ COMPLETE

- [x] Process symbols in batches for Neo4j writes — `UNWIND $symbols` in `internal/symbols/store.go:187`
- [x] Use UNWIND for efficient bulk creation — symbols and relationships both use UNWIND
- [x] Implement deduplication: `MERGE` on `{space_id, symbol_id}` with `ON CREATE SET` / `ON MATCH SET`

#### 8.3.4 File Changes

| File | Status | Changes |
|------|--------|---------|
| `internal/symbols/types.go` | ✅ DONE | Symbol, FileSymbols, ParserConfig types |
| `internal/symbols/parser.go` | ✅ DONE | Tree-sitter parsing for TS/JS/Go/Python |
| `internal/symbols/parser_test.go` | ✅ DONE | 12 unit tests |
| `internal/symbols/service.go` | ✅ DONE | Symbol service with caching and query methods |
| `internal/symbols/service_test.go` | ✅ DONE | Service unit tests (7 tests) |
| `internal/symbols/provider.go` | ✅ DONE | Provider interface for plugins |
| `api/proto/mdemg-module.proto` | ✅ DONE | SymbolInfo, SymbolQueryRequest/Response messages |
| `internal/models/models.go` | ✅ DONE | SymbolEvidence, IngestSymbol, Symbols field |
| `internal/symbols/store.go` | ✅ DONE | Neo4j persistence for SymbolNodes |
| `internal/symbols/store_test.go` | ✅ DONE | Store unit tests |
| `cmd/ingest-codebase/main.go` | ✅ DONE | --extract-symbols flag, TS/JS/Go/Python extractors |
| `internal/retrieval/service.go` | ✅ DONE | Symbol search methods, evidence attachment |
| `internal/api/server.go` | ✅ DONE | SymbolStore wiring to retrieval service |
| `internal/api/handlers.go` | ✅ DONE | storeSymbolsForBatch during ingestion |
| `migrations/V0007__symbol_nodes.cypher` | ✅ DONE | SymbolNode schema, indexes, constraints |

---

### Deliverable 8.4: Symbol-Aware Retrieval

**Priority**: P1 | **Effort**: 2-3 days | **Status**: ✅ COMPLETE

#### 8.4.1 Search Modes

| Mode | Query | Use Case |
|------|-------|----------|
| `semantic` | Vector similarity on query text | "How does caching work?" |
| `symbol_exact` | Exact match on symbol name | "DEFAULT_FLUSH_INTERVAL" |
| `symbol_prefix` | Prefix match on symbol name | "DEFAULT_" |
| `symbol_fuzzy` | Fulltext search on symbol name | "flush interval" |
| `hybrid` | Vector + symbol boost | Default mode |

#### 8.4.2 Hybrid Scoring Formula

```
score = α * vector_sim + β * activation + γ * recency + δ * confidence + ε * symbol_match

Where symbol_match:
  - 1.0 if exact symbol name match
  - 0.8 if symbol name contains query term
  - 0.5 if doc comment contains query term
  - 0.0 otherwise

New weight: ε = 0.25 (high weight for symbol matches)
Rebalanced: α = 0.40, β = 0.20, γ = 0.10, δ = 0.05, ε = 0.25
```

#### 8.4.3 API Changes

**Request** (updated `/v1/memory/retrieve`):

```json
{
  "space_id": "vscode-scale",
  "query_text": "DEFAULT_FLUSH_INTERVAL storage",
  "top_k": 10,
  "search_mode": "hybrid",
  "include_symbols": true,
  "symbol_types": ["const", "function"]
}
```

**Response** (new `symbols` and `evidence` fields):

```json
{
  "data": {
    "results": [...],
    "symbols": [
      {
        "name": "DEFAULT_FLUSH_INTERVAL",
        "type": "const",
        "value": "60000",
        "file_path": "src/vs/platform/storage/common/storage.ts",
        "line_number": 42,
        "snippet": "export const DEFAULT_FLUSH_INTERVAL = 60 * 1000; // 1 minute",
        "score": 0.95,
        "match_type": "exact"
      }
    ],
    "evidence": {
      "primary_symbol": "DEFAULT_FLUSH_INTERVAL",
      "definition_file": "src/vs/platform/storage/common/storage.ts",
      "definition_line": 42,
      "value": "60000",
      "confidence": 0.95,
      "snippet": "export const DEFAULT_FLUSH_INTERVAL = 60 * 1000; // 1 minute"
    },
    "jiminy": {
      "rationale": "Found exact symbol match for 'DEFAULT_FLUSH_INTERVAL' in storage.ts",
      "evidence_source": "symbol_exact_match",
      "confidence": 0.95
    }
  }
}
```

---

### Deliverable 8.5: Symbol Search Endpoint & Proto Integration

**Priority**: P2 | **Effort**: 1-2 days | **Status**: 📦 ARCHIVED (2026-02-06)

> **Archive Note**: Deliverables 8.5-8.6 retired per user decision. Proto integration complete; dedicated symbol endpoint deferred in favor of Phase 9 incremental updates. Symbol search remains available via `/v1/memory/retrieve` with `include_symbols: true`.

#### 8.5.0 Proto Updates (`api/proto/mdemg-module.proto`) ✅ COMPLETE

Extended the proto definition to support symbols in module communication:

```protobuf
// Added to Observation message
message Observation {
    // ... existing fields ...
    repeated SymbolInfo symbols = 10;  // Symbols extracted from this content
}

// Added to RetrievalCandidate for reasoning modules
message RetrievalCandidate {
    // ... existing fields ...
    repeated SymbolInfo symbols = 9;   // Symbols for evidence-locked retrieval
}

// SymbolInfo message for ingestion and retrieval
message SymbolInfo {
    string name = 1;
    string symbol_type = 2;
    string value = 3;
    string raw_value = 4;
    string file_path = 5;
    int32 line_number = 6;
    int32 end_line = 7;
    int32 column = 8;
    bool exported = 9;
    string doc_comment = 10;
    string signature = 11;
    string parent = 12;
    string language = 13;
    string type_annotation = 14;
}

// SymbolQueryRequest/Response for plugin symbol lookups
message SymbolQueryRequest {
    string space_id = 1;
    string query = 2;
    string file_path = 3;
    repeated string symbol_types = 4;
    string language = 5;
    bool exported_only = 6;
    int32 limit = 7;
}

message SymbolQueryResponse {
    repeated SymbolInfo symbols = 1;
    int32 total_count = 2;
    string error = 3;
}
```

This allows:

- Ingestion modules to emit symbols with their observations
- Reasoning modules to receive symbol evidence in retrieval candidates
- Plugins to query the core symbol service via gRPC

#### 8.5.1 New Endpoint: `GET /v1/memory/symbols`

Dedicated symbol search for IDE integration and direct queries.

**Request**:

```bash
GET /v1/memory/symbols?space_id=vscode&name=DEFAULT_*&type=const&limit=20
```

**Query Parameters**:

| Param | Type | Description |
|-------|------|-------------|
| `space_id` | string | Required - memory space |
| `name` | string | Symbol name pattern (supports `*` wildcard) |
| `type` | string | Filter by symbol type |
| `file` | string | Filter by file path pattern |
| `exported` | bool | Filter by export status |
| `limit` | int | Max results (default 20) |

**Response**:

```json
{
  "data": {
    "symbols": [
      {
        "name": "DEFAULT_FLUSH_INTERVAL",
        "type": "const",
        "value": "60000",
        "file_path": "src/vs/platform/storage/common/storage.ts",
        "line_number": 42,
        "exported": true,
        "doc_comment": "Flush interval for storage persistence in milliseconds"
      }
    ],
    "total": 1
  }
}
```

---

### Deliverable 8.6: Testing & Validation

**Priority**: P1 | **Effort**: 2 days | **Status**: 📦 ARCHIVED (2026-02-06)

> **Archive Note**: Deliverable 8.6 retired per user decision. Core symbol parsing (8.6.1 parser tests) complete. VS Code benchmark validation deferred; existing symbol integration in retrieve pipeline provides sufficient coverage for current use cases.

#### 8.6.1 Unit Tests (Completed)

- [x] `internal/symbols/parser_test.go` - Tree-sitter parsing for each language (12 tests)

#### 8.6.2-8.6.3 Deferred

Integration tests and VS Code benchmark validation archived for future consideration.

---

### Implementation Timeline

| Week | Deliverable | Tasks | Status |
|------|-------------|-------|--------|
| **Week 1** | 8.1 | Tree-sitter integration, parser, types | ✅ DONE |
| **Week 1-2** | 8.2 | V0007 migration, Neo4j schema, store.go | ✅ DONE |
| **Week 2** | 8.3 | Ingestion pipeline, cmd/ingest-codebase | ✅ DONE |
| **Week 3** | 8.4 | Hybrid retrieval, scoring formula update | ✅ DONE |
| **Week 4** | 8.5, 8.6 | Symbol endpoint, testing, VS Code re-benchmark | 📦 ARCHIVED |

### Current Progress Summary

**✅ Completed (Deliverable 8.1 - Parser Infrastructure)**:

- `internal/symbols/types.go` - Symbol, FileSymbols, ParserConfig structs
- `internal/symbols/parser.go` - Tree-sitter integration for TS/JS/Go/Python
- `internal/symbols/parser_test.go` - 12 passing unit tests
- **Validated**: 451 symbols extracted from 49 Go files in MDEMG codebase

**✅ Completed (Deliverable 8.2 - Storage Schema)**:

- `migrations/V0007__symbol_nodes.cypher` - SymbolNode label, indexes, constraints
- `internal/symbols/store.go` - Neo4j CRUD operations for SymbolNodes
- `internal/symbols/store_test.go` - Store unit tests

**✅ Completed (Deliverable 8.3 - Ingestion Integration)**:

- `cmd/ingest-codebase/main.go` - `--extract-symbols` flag, extractors for TS/JS/Go/Python
- `internal/models/models.go` - `IngestSymbol` struct, `Symbols` field on `BatchIngestItem`
- `internal/api/handlers.go` - `storeSymbolsForBatch()` stores symbols during batch ingest
- Symbol embedding via configured embedder (OpenAI/Ollama)

**✅ Completed (Deliverable 8.4 - Retrieval Integration)**:

- `internal/retrieval/service.go` - Symbol search in Retrieve() pipeline (step 1c)
- `internal/api/server.go` - SymbolStore wiring to retrieval service
- Symbol pattern matching: CamelCase, UPPER_CASE, snake_case extraction from queries
- Symbol evidence attachment to retrieval results

**Plugin Integration Architecture**:

```
┌─────────────────────────────────────────────────────────────────┐
│                         CORE (mdemg)                            │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ symbols.Service │  │ symbols.Parser  │  │ symbols.Store   │ │
│  │ (caching/query) │  │ (tree-sitter)   │  │ (Neo4j persist) │ │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘ │
│           │                    │                    │           │
│           └──────────┬─────────┴────────────────────┘           │
│                      │                                          │
│           ┌──────────▼──────────┐                               │
│           │ gRPC / HTTP API     │                               │
│           │ - SymbolQueryReq/Res│                               │
│           │ - SymbolEvidence    │                               │
│           └──────────┬──────────┘                               │
└──────────────────────┼──────────────────────────────────────────┘
                       │
        ┌──────────────┼──────────────┐
        │              │              │
        ▼              ▼              ▼
   ┌─────────┐   ┌─────────┐   ┌─────────┐
   │INGESTION│   │REASONING│   │   APE   │
   │ Module  │   │ Module  │   │ Module  │
   │         │   │         │   │         │
   │ Emits   │   │Receives │   │ Queries │
   │ symbols │   │ symbol  │   │ symbols │
   │ in Obs  │   │ evidence│   │ for     │
   │         │   │         │   │ analysis│
   └─────────┘   └─────────┘   └─────────┘
```

**📦 Archived (Deliverable 8.5-8.6)** (2026-02-06):

- Symbol search endpoint and VS Code benchmark validation deferred
- Core symbol infrastructure (8.1-8.4) complete and integrated into retrieve pipeline

**📦 Archived Remaining Items** (moved to backlog):

- `/v1/memory/symbols` endpoint - deferred, use retrieve with `include_symbols: true`
- VS Code benchmark validation - deferred

---

### Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Tree-sitter parse failures | Medium | Low | Graceful fallback, skip unparseable files |
| Storage bloat (10x nodes) | High | Medium | Configurable extraction, exported-only mode |
| Query latency increase | Medium | Medium | Separate symbol index, async enrichment |
| Language coverage gaps | Low | Low | Start with TS/Go, add languages incrementally |

---

### Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `github.com/smacker/go-tree-sitter` | latest | Multi-language AST parsing |
| `github.com/tree-sitter/tree-sitter-typescript` | latest | TypeScript grammar |
| `github.com/tree-sitter/tree-sitter-go` | latest | Go grammar |
| `github.com/tree-sitter/tree-sitter-python` | latest | Python grammar |

---

### Success Metrics

| Metric | Current | Phase 8 Target | Stretch |
|--------|---------|----------------|---------|
| Evidence-locked accuracy | ~20% | **80%** | 90% |
| Symbol definition retrieval | 0% | **90%** | 95% |
| Value extraction accuracy | 0% | **85%** | 90% |
| Query latency (p95) | 50ms | **75ms** | 60ms |
| Storage overhead | 1x | **5-10x** | <5x |

---

---

## Phase 9: Incremental Update & Re-Ingestion

**Motivation**: As codebases evolve, MDEMG must efficiently update its memory graph without full re-ingestion. Different update triggers serve different use cases: git commits for CI/CD integration, time-based for scheduled maintenance, user-triggered for on-demand refresh, and plugin-specific for domain events.

**Goal**: Provide flexible, efficient mechanisms to keep the memory graph synchronized with source data.

---

### Deliverable 9.1: Git Commit Hooks

**Priority**: P1 | **Effort**: 2-3 days | **Status**: ✅ COMPLETE

#### 9.1.1 Git Diff-Based Ingestion

- [x] Add `--incremental` flag to `cmd/ingest-codebase`
- [x] Add `--since` flag for specifying base commit (default: HEAD~1)
- [x] Parse `git diff` to identify changed/added/deleted files
- [x] Only process changed files (skip unchanged)
- [x] Handle deletions via `--archive-deleted` flag (default: true)
- [ ] Handle renames: update file_path, preserve node_id (deferred)

#### 9.1.2 Post-Commit Hook Integration

```bash
# .git/hooks/post-commit
#!/bin/bash
go run ./cmd/ingest-codebase --incremental --space-id="my-project" --quiet --log-file=".mdemg/incremental.log"
```

- [x] Support `--quiet` mode for background execution
- [x] Support `--log-file` for file logging
- [ ] Create `mdemg-cli` wrapper binary (deferred - use go run for now)

#### 9.1.3 CI/CD Integration

```yaml
# GitHub Actions example
- name: Update MDEMG Memory
  run: |
    go run ./cmd/ingest-codebase \
      --incremental \
      --space-id=${{ github.repository }} \
      --endpoint=${{ secrets.MDEMG_ENDPOINT }}
```

---

### Deliverable 9.2: Time-Based Scheduled Sync

**Priority**: P2 | **Effort**: 1-2 days | **Status**: ✅ COMPLETE (Freshness tracking + StartScheduledSync covers use case; APE INGEST action deferred — not needed)

#### 9.2.1 APE Scheduled Ingestion

Leverage existing APE scheduler for periodic re-ingestion:

```json
{
  "id": "scheduled-ingest",
  "type": "APE",
  "schedule": "0 2 * * *",
  "config": {
    "action": "full_reingest",
    "space_id": "my-project",
    "source_path": "/path/to/repo"
  }
}
```

- [ ] Add `INGEST` action type to APE modules
- [ ] Support `full_reingest` vs `incremental` modes
- [x] Track last_ingest_time per space_id (via TapRoot)
- [ ] Detect stale spaces (no update in N days) and trigger refresh

#### 9.2.2 Freshness Tracking ✅

```cypher
// TapRoot freshness properties (implemented)
MATCH (t:TapRoot {space_id: $space_id})
SET t.last_ingest_at = datetime(),
    t.last_ingest_type = 'incremental',
    t.ingest_count = coalesce(t.ingest_count, 0) + 1
```

- [x] Add freshness metadata to TapRoot nodes
- [x] API endpoint: `GET /v1/memory/spaces/{space_id}/freshness` (handlers_freshness.go)
- [x] Configurable stale threshold (`SyncStaleThresholdHours` in config)

---

### Deliverable 9.3: User-Triggered Updates

**Priority**: P1 | **Effort**: 1 day | **Status**: ✅ COMPLETE

> **UATS Coverage**: All endpoints have UATS specs in `docs/api/api-spec/uats/specs/`

#### 9.3.1 Manual Re-Ingest API ✅

```bash
POST /v1/memory/ingest/trigger
{
  "space_id": "my-project",
  "path": "/path/to/repo",
  "incremental": true,
  "extract_symbols": true,
  "consolidate": true
}
```

Response:

```json
{
  "job_id": "ingest-abc123",
  "status": "running",
  "message": "Ingestion job started"
}
```

- [x] Background job queue implemented (internal/jobs/jobs.go)
- [x] `POST /v1/memory/ingest/trigger` - start job (handlers.go:2162)
- [x] `GET /v1/memory/ingest/status/{job_id}` - poll progress (handlers.go:2458)
- [x] `POST /v1/memory/ingest/cancel/{job_id}` - cancel job
- [x] `GET /v1/memory/ingest/jobs` - list all jobs
- [x] UATS specs: ingest_trigger.uats.json, ingest_status.uats.json, ingest_cancel.uats.json, ingest_jobs.uats.json

#### 9.3.2 File-Level Re-Ingest ✅

```bash
POST /v1/memory/ingest/files
{
  "space_id": "my-project",
  "files": [
    "src/auth/service.ts",
    "src/users/controller.ts"
  ]
}
```

- [x] Re-ingest specific files without full scan (handlers.go:2591)
- [x] Synchronous for ≤50 files, background job for >50
- [x] Useful for IDE integration (save → update)
- [x] Tests in handlers_ingest_test.go

---

### Deliverable 9.4: Plugin-Specific Triggers

**Priority**: P2 | **Effort**: 2 days | **Status**: ✅ COMPLETE

#### 9.4.1 Linear Webhook Integration — COMPLETE

- [x] Webhook receiver: `internal/api/handle_webhooks.go` — HMAC-SHA256 verification, debouncing (10s)
- [x] Real-time issue/project sync via gRPC dispatch to `linear-module`
- [x] Batch ingest + APE `source_changed` event triggering
- [x] Route: `POST /v1/webhooks/linear` (server.go)
- [x] Config: `LinearWebhookSecret`, `LinearWebhookSpaceID` env vars

#### 9.4.2 File Watcher REST API — COMPLETE

- [x] Core: `internal/filewatcher/watcher.go` — Manager, debouncing, extension filtering, exclude patterns
- [x] REST API: `internal/api/handlers_filewatcher.go` — 3 endpoints for runtime management
  - `POST /v1/filewatcher/start` — start watcher for a space (validates path, resolves absolute)
  - `GET /v1/filewatcher/status` — list all active watchers with count
  - `POST /v1/filewatcher/stop` — stop watcher for a space
- [x] Config-based startup: `FILE_WATCHER_ENABLED`, `FILE_WATCHER_CONFIGS`
- [x] 3 UATS specs: filewatcher_start, filewatcher_status, filewatcher_stop

#### 9.4.3 Event-Driven Module Updates — COMPLETE

- [x] `EventSubscriptions` field on manifest `Capabilities` (`internal/plugins/types.go`)
- [x] `EventDispatcher` (`internal/plugins/events.go`) routes events to non-APE modules
- [x] INGESTION modules: dispatched via `IngestionClient.Parse` with event metadata
- [x] CRUD modules: logged (no OnEvent RPC yet)
- [x] Server wiring: `TriggerAPEEventWithContext` now dispatches to both APE scheduler and EventDispatcher
- [x] Wildcard subscription support (`*` matches all events)
- [x] Unit tests: `internal/plugins/events_test.go`

---

### Deliverable 9.5: Conflict Resolution & Consistency

**Priority**: P2 | **Effort**: 1-2 days | **Status**: ✅ COMPLETE

#### 9.5.1 Concurrent Update Handling ✅ COMPLETE

- [x] Optimistic locking on MemoryNode updates — `internal/optimistic/lock.go` (214 lines, 17 tests), exponential backoff with jitter
- [x] Versioned node/edge updates — `internal/retrieval/versioned_update.go` (425 lines), `ingest_retry.go` (225 lines)
- [x] Conflict logging via version mismatch detection and retry exhaustion reporting

#### 9.5.2 Orphan Detection ✅ COMPLETE

- [x] `POST /v1/memory/cleanup/orphans` — timestamp-based L0 orphan detection, actions: count/list/archive/delete
- [x] `POST /v1/memory/cleanup/graph-orphans` — cross-space zero-edge node scan, actions: scan/consolidate/archive/delete
- [x] Options: archive, delete, flag for review, dry-run support, protected-space blocking
- [x] 3 UATS specs (9 variants) — `orphan_cleanup.uats.json`, `graph_orphan_cleanup.uats.json`

#### 9.5.3 Edge Consistency ✅ COMPLETE

- [x] Refresh edges when connected nodes change (`RefreshStaleCoactivationEdges`)
- [x] Re-run Hebbian learning for updated nodes (staleness cascade via `PropagateEdgeStaleness`)
- [x] Invalidate stale hidden nodes if inputs changed (cache invalidation on edge changes)
- [x] `GET /v1/memory/edges/stale/stats` endpoint
- [x] `POST /v1/memory/edges/stale/refresh` endpoint
- [x] Optimistic lock retry with exponential backoff (`internal/optimistic/`)
- [x] Versioned node/edge updates with version mismatch detection

---

### Implementation Priority

| Deliverable | Impact | Effort | Priority |
|-------------|--------|--------|----------|
| 9.3 User-Triggered | HIGH | LOW | P0 |
| 9.1 Git Hooks | HIGH | MEDIUM | P1 |
| 9.2 Time-Based | MEDIUM | LOW | P2 |
| 9.4 Plugin Triggers | MEDIUM | MEDIUM | ✅ DONE |
| 9.5 Conflict Resolution | MEDIUM | MEDIUM | P2 |

### Success Metrics

| Metric | Full Re-Ingest | Incremental Target |
|--------|----------------|-------------------|
| Time for 1000 file change | 30-60 min | **< 2 min** |
| Time for 10 file change | 30-60 min | **< 10 sec** |
| CPU overhead (file watcher) | N/A | **< 5%** |
| Memory overhead (file watcher) | N/A | **< 50 MB** |

---

## Phase 10: Query Optimization & Caching ✅ COMPLETE

**Motivation**: As MDEMG scales to larger codebases (100K+ nodes) and handles more concurrent requests, query performance and data transmission efficiency become critical. This phase focuses on profiling, optimizing, and caching at multiple layers.

**Goal**: Reduce p95 query latency by 50% and enable efficient operation at 10x current scale.

**Progress** (2026-01-26):

- Deliverable 10.1: Query profiling + index optimization complete (29ms avg vectorRecall)
- Deliverable 10.2: Query result caching complete (98.9% latency improvement on repeated queries)

### Performance Summary

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Uncached query latency | 387ms | 29ms | **92.5%** |
| Cached query latency | N/A | 4ms | **98.9%** vs uncached |
| Slow queries (>100ms) | Unknown | 0% | Profiling enabled |
| Cache hit rate | N/A | 100% | On repeated queries |

**New Monitoring Endpoints:**

- `GET /v1/memory/cache/stats` - Cache hit rate, size, capacity, TTL
- `GET /v1/memory/query/metrics` - Query count, slow query %, avg duration

**New Neo4j Indexes:**

- `memory_space_archived` - (space_id, is_archived) for archive filtering
- `memory_created_at` - (created_at) range index for temporal queries

---

### Deliverable 10.1: Neo4j Query Optimization ✅ COMPLETE

**Priority**: P1 | **Effort**: 3-4 days | **Status**: ✅ COMPLETE (2026-01-26)

#### 10.1.1 Query Profiling & Analysis ✅ COMPLETE

- [x] Add query timing instrumentation to Neo4j operations (`internal/retrieval/profiling.go`)
- [x] Log slow queries (>100ms) with Cypher and sanitized parameters
- [x] New `/v1/memory/query/metrics` endpoint for monitoring
- [ ] Use `EXPLAIN` and `PROFILE` to analyze query plans (future)
- Performance baseline: vectorRecall = 29ms avg, 0% slow queries

#### 10.1.2 Index Optimization ✅ COMPLETE

Created missing indexes:

```cypher
CREATE INDEX memory_space_archived IF NOT EXISTS
FOR (m:MemoryNode) ON (m.space_id, m.is_archived);

CREATE RANGE INDEX memory_created_at IF NOT EXISTS
FOR (m:MemoryNode) ON (m.created_at);
```

- [x] Audit queries for index usage (existing indexes cover common patterns)
- [x] Add composite index for archived queries
- [x] Add range index for timestamp queries

#### 10.1.3 Query Rewriting (Future)

- [ ] Replace `OPTIONAL MATCH` with existence checks where possible
- [ ] Use `UNWIND` for batch operations instead of multiple queries
- [ ] Limit graph traversal depth with explicit bounds
- [ ] Use `WITH` to reduce intermediate result sets early

---

### Deliverable 10.2: Result Caching Layer ✅ COMPLETE

**Priority**: P1 | **Effort**: 2-3 days | **Status**: ✅ COMPLETE (2026-01-26)

#### Implementation Details

- **File**: `internal/retrieval/cache.go` - TTL-LRU cache implementation
- **Tests**: `internal/retrieval/cache_test.go` - 7 test cases covering key generation, TTL, LRU eviction, space invalidation
- **Endpoint**: `GET /v1/memory/cache/stats` - Returns cache hit rate, size, capacity, TTL

#### Performance Results

| Metric | Value |
|--------|-------|
| Cached query latency | **4ms** |
| Uncached query latency | 387ms |
| Latency improvement | **98.9%** |
| Cache hit rate (repeated queries) | **100%** |

#### Configuration

```bash
QUERY_CACHE_ENABLED=true      # Feature toggle (default: true)
QUERY_CACHE_CAPACITY=500      # LRU cache capacity (default: 500)
QUERY_CACHE_TTL_SECONDS=300   # TTL in seconds (default: 300)
```

#### Original Design (Reference)

```go
type QueryCache struct {
    cache    *lru.Cache[string, CacheEntry]
    ttl      time.Duration
    maxSize  int
}

type CacheEntry struct {
    Results   []RetrieveResult
    Timestamp time.Time
    HitCount  int64
}
```

- [ ] Implement LRU cache for retrieval results
- [ ] Cache key: hash of (space_id, query_text, top_k, filters)
- [ ] Configurable TTL (default: 5 minutes)
- [ ] Invalidate on ingest/update to affected space_id
- [ ] Cache hit/miss metrics for monitoring

#### 10.2.2 Embedding Cache Enhancement

Current: In-memory LRU cache for embeddings

- [ ] Add persistent cache option (Redis or disk-based)
- [ ] Pre-warm cache on server startup for frequent queries
- [ ] Batch embedding requests to reduce API round-trips
- [ ] Cache embedding provider responses with content hash

#### 10.2.3 Graph Structure Cache

```go
// Cache frequently-accessed graph structures
type GraphCache struct {
    tapRoots     map[string]*TapRoot      // space_id -> TapRoot
    nodeCount    map[string]int           // space_id -> count
    edgeStats    map[string]EdgeStats     // space_id -> edge statistics
}
```

- [ ] Cache TapRoot lookups (very frequent)
- [ ] Cache node/edge counts for stats endpoint
- [ ] Cache graph metadata (schema version, last ingest time)
- [ ] Invalidate selectively on graph mutations

---

### Deliverable 10.3: Data Transmission Optimization ✅ COMPLETE

**Priority**: P2 | **Effort**: 2-3 days | **Status**: ✅ COMPLETE (2026-02-06)

#### 10.3.1 Response Compression

- [x] Enable gzip compression for API responses (existing middleware)
- [ ] Compress large embedding vectors in transit
- [x] Use Protocol Buffers for internal gRPC (already in place)
- [ ] Benchmark: JSON vs gzip JSON vs Protobuf

#### 10.3.2 Pagination & Streaming

- [x] Implement cursor-based pagination fields (`Cursor`, `Limit`, `NextCursor`, `HasMore` in models.go)
- [ ] Add `limit` and `offset` to all list endpoints (retrieval logic pending)
- [x] SSE streaming for batch ingest job progress (`GET /v1/jobs/{job_id}/stream`)
- [ ] WebSocket support for real-time progress updates (deferred)

**New Files:**

- `internal/api/sse.go` — SSE streaming handler
- `internal/api/sse_test.go` — 22 unit tests (100% coverage)

#### 10.3.3 Selective Field Loading

- [ ] Support field selection in retrieve requests (FieldSelector helpers exist but not applied)
- [ ] Skip loading large fields (embedding, full content) when not needed
- [ ] Lazy loading for symbol details and evidence

---

### Deliverable 10.4: Connection Pooling & Resource Management ✅ COMPLETE

**Priority**: P2 | **Effort**: 1-2 days | **Status**: ✅ COMPLETE (2026-02-06)

#### 10.4.1 Neo4j Connection Pool Tuning

- [x] Connection pool metrics exported to Prometheus (7 gauges in collectors.go)
- [x] Pool configuration via env vars (existing: `NEO4J_POOL_*`)
- [x] Connection pool metrics endpoint (`/v1/system/pool-metrics`)

#### 10.4.2 Embedding API Rate Limiting ✅

- [x] Implement client-side rate limiting for OpenAI/Ollama (`internal/embeddings/ratelimit.go`)
- [x] Token bucket rate limiter with configurable RPS and burst
- [x] Circuit breaker for Ollama (matches OpenAI pattern)

**New Files:**

- `internal/embeddings/ratelimit.go` — Rate-limited embedder wrapper
- `internal/embeddings/ratelimit_test.go` — 11 unit tests (100% coverage)

**Configuration:**

```bash
EMBEDDING_RATE_LIMIT_ENABLED=false
EMBEDDING_OPENAI_RPS=500
EMBEDDING_OPENAI_BURST=1000
EMBEDDING_OLLAMA_RPS=100
EMBEDDING_OLLAMA_BURST=200
```

#### 10.4.3 Memory Management ✅

- [x] Memory pressure monitoring and backpressure (`internal/backpressure/memory.go`)
- [x] Middleware returns 503 with Retry-After when heap > threshold
- [x] Health endpoints bypass backpressure

**New Files:**

- `internal/backpressure/memory.go` — Memory pressure middleware
- `internal/backpressure/memory_test.go` — 11 unit tests (100% coverage)

**Configuration:**

```bash
MEMORY_PRESSURE_ENABLED=false
MEMORY_PRESSURE_THRESHOLD_MB=4096
```

---

### Deliverable 10.5: Benchmarking & Monitoring

**Priority**: P1 | **Effort**: 2 days | **Status**: ✅ PARTIAL (Observability complete, CI benchmark regression pending)

#### 10.5.1 Performance Benchmarks

```bash
# Benchmark suite
go test -bench=. -benchmem ./internal/retrieval/...
go test -bench=. -benchmem ./internal/symbols/...
```

- [x] Create benchmark suite for critical paths — 9 benchmarks in `internal/retrieval/retrieval_bench_test.go`
- [x] Measure: retrieval latency, ingest throughput, symbol search — 3 UBTS specs (`retrieve_latency`, `ingest_throughput`, `concurrent_load`) with smoke/load/stress profiles
- [ ] Track regression in CI (fail if >10% slower) — no `go test -bench` in CI pipeline yet
- [x] Document baseline metrics — targets documented in roadmap §10 Success Metrics

#### 10.5.2 Observability ✅ COMPLETE

```go
// Metrics to expose
mdemg_query_duration_seconds{space_id, query_type}
mdemg_cache_hits_total{cache_type}
mdemg_cache_misses_total{cache_type}
mdemg_neo4j_query_duration_seconds{query_name}
mdemg_embedding_requests_total{provider, status}
```

- [x] Add Prometheus metrics for all caches — 40+ metrics in `internal/metrics/collectors.go`, validated by UOBS spec
- [x] Track query latency histograms by type — HTTP request duration + retrieval latency histograms with configurable buckets
- [x] Monitor Neo4j driver pool statistics — 7 pool metrics (active, idle, waiting, acquired, created, closed, failed)
- [x] Alert on cache hit rate drops — 15+ alert rules in `deploy/docker/prometheus/alerts/` (latency SLOs, cache hit ratio, circuit breakers)

---

### Implementation Priority

| Deliverable | Impact | Effort | Priority |
|-------------|--------|--------|----------|
| 10.1 Query Optimization | HIGH | MEDIUM | P0 |
| 10.2 Result Caching | HIGH | MEDIUM | P1 |
| 10.5 Benchmarking | MEDIUM | LOW | P1 |
| 10.3 Data Transmission | MEDIUM | MEDIUM | P2 |
| 10.4 Connection Pooling | MEDIUM | LOW | P2 |

### Success Metrics

| Metric | Current | Target | Stretch |
|--------|---------|--------|---------|
| Retrieve p50 latency | ~30ms | **15ms** | 10ms |
| Retrieve p95 latency | ~80ms | **40ms** | 25ms |
| Query cache hit rate | 0% | **60%** | 80% |
| Embedding cache hit rate | ~40% | **80%** | 90% |
| Ingest throughput | 7/s | **15/s** | 25/s |
| Max concurrent retrievals | ~20 | **100** | 200 |

---

## Phase 11: LLM Plugin SDK & Self-Improvement Capabilities

**Motivation**: To achieve MDEMG's vision as a self-improving long-term memory system, LLM agents need the ability to extend MDEMG's capabilities by creating their own sidecar plugins. This phase provides the framework, tooling, and skills for autonomous plugin creation.

**Goal**: Enable LLM agents to identify capability gaps and create new plugins to fill them autonomously.

---

### Deliverable 11.1: Plugin SDK Documentation ✅ COMPLETE

- **Priority**: P0 | **Status**: COMPLETE (Agent: a4f38ec)
- [x] Comprehensive plugin development guide: `docs/SDK_PLUGIN_GUIDE.md` (1,582 lines)
- [x] Module type templates (INGESTION, REASONING, APE)
- [x] manifest.json schema with all configuration options
- [x] gRPC lifecycle documentation and best practices
- [x] Troubleshooting guide

### Deliverable 11.2: LLM Semantic Summary Service ✅ COMPLETE

- **Priority**: P1 | **Status**: COMPLETE (Agent: a3bff87)
- [x] LLM client for semantic summary generation: `internal/summarize/service.go`
- [x] Support for OpenAI and Ollama providers
- [x] LRU caching for repeated content
- [x] Batching support for API efficiency
- [x] Fallback to structural summaries on failure
- [x] Unit tests: `internal/summarize/service_test.go` (10 tests)
- [x] CLI integration: `--llm-summary`, `--llm-summary-model`, `--llm-summary-batch`, `--llm-summary-provider`

**Configuration**:

```bash
LLM_SUMMARY_ENABLED=true           # Feature toggle (default: false)
LLM_SUMMARY_PROVIDER=openai        # "openai" or "ollama"
LLM_SUMMARY_MODEL=gpt-4o-mini      # Model to use
LLM_SUMMARY_MAX_TOKENS=150         # Max tokens in response
LLM_SUMMARY_BATCH_SIZE=10          # Files per API call
LLM_SUMMARY_TIMEOUT_MS=30000       # Request timeout
LLM_SUMMARY_CACHE_SIZE=1000        # LRU cache size
```

### Deliverable 11.3: Claude Plugin Creation Skill ✅ COMPLETE

- **Priority**: P1 | **Status**: COMPLETE (Agent: af9daaf)
- [x] Claude skill for autonomous plugin creation: `.claude/skills/create-plugin.md` (931 lines)
- [x] Decision framework for choosing plugin type
- [x] Step-by-step workflow (5 phases)
- [x] Complete code templates for all module types
- [x] Validation checklist

### Deliverable 11.4: Plugin Scaffolding Generator ✅ COMPLETE

- **Priority**: P1 | **Status**: COMPLETE
- [x] CLI command: `cmd/plugin-scaffold/main.go`
- [x] Scaffold generator: `internal/plugins/scaffold/scaffold.go`
- [x] Generates directory structure with manifest.json, main.go, Makefile
- [x] Includes boilerplate gRPC handlers
- [x] Validates against proto definitions

### Deliverable 11.5: Plugin Validation & Testing Framework ✅ COMPLETE

- **Priority**: P1 | **Status**: COMPLETE
- [x] Automated manifest.json validation: `internal/plugins/validator.go`
- [x] Validation CLI: `cmd/plugin-validate/main.go`
- [x] gRPC contract testing against mdemg-module.proto
- [x] Health check simulation

### Deliverable 11.6: Plugin Creation API for LLM Agents ✅ COMPLETE

- **Priority**: P2 | **Status**: COMPLETE
- [x] `POST /v1/plugins/create` endpoint: `internal/api/plugin_handlers.go`
- [x] `GET /v1/plugins/{id}` — retrieve plugin by ID
- [x] `POST /v1/plugins/{id}/validate` — validate plugin
- [x] UATS spec: `plugin_create.uats.json` (6 variants)

### Deliverable 11.7: Capability Gap Detection ✅ COMPLETE

- **Priority**: P2 | **Status**: COMPLETE
- [x] Gap detector: `internal/gaps/detector.go`
- [x] Gap store: `internal/gaps/store.go`
- [x] Gap interviewer: `internal/gaps/interview.go`
- [x] API handlers: `internal/api/gaps_handlers.go`
- [x] UATS specs: `capability_gaps.uats.json`, `capability_gaps_full.uats.json` (4 variants), `gap_interviews.uats.json`

---

## Phase 7: Public Readiness & Open Source Hardening

MDEMG is being prepared for public release. This phase focuses on security, governance, and the establishment of a collaboration framework to allow external contributors to build SME Modules.

### Deliverable 7.1: Governance & CI/CD Guards ✅ COMPLETE

- [x] Implement Issue/PR templates and `CONTRIBUTING.md`.
- [x] Set up GitHub Actions for automated linting and integration testing.
- [x] CODEOWNERS, dependabot, auto-PR workflow, parser-tests workflow.
- [x] SECURITY.md, CODE_OF_CONDUCT.md.
- [x] Expanded `golangci-lint` config (govet, staticcheck, errcheck, ineffassign, unused, gosec — 6 linters).
- [x] UATS contract tests added to CI pipeline.

### Deliverable 7.2: Security Hardening ✅ COMPLETE

- [x] Perform secret scrubbing and path normalization across scripts and API handlers. (Zero hardcoded paths in Go source; benchmarks/archives contain `/Users/` metadata — acceptable for historical data)
- [x] Implement user-friendly error sanitization — `sanitizeError()` + `writeInternalError()` in `server.go`. All 20 raw `err.Error()` leaks sanitized across 7 handler files; `readJSON` utility sanitized (affects all JSON-parsing endpoints)
- [x] Add secret scanning (gitleaks) to CI pipeline — `.github/workflows/ci.yml` security job
- [x] Enable gosec in `.golangci.yml` — security-focused static analysis with documented exclusions for known false-positive categories (G104, G204, G304, G704, G706, etc.)

---

## Testing Strategy

### Regression Test Suite

Create dedicated test questions for each improvement:

- 10 questions per track (50 total)
- Measure before/after scores
- Track improvement percentage

### Test Questions by Track

**Track 1 (Learning Edges)**:

- Repeat the same question 5 times, measure if retrieval improves

**Track 2 (Cross-Cutting)**:

- "How does authentication work across the application?"
- "What modules use the ACL interceptor?"
- "How are errors handled globally?"

**Track 3 (Comparison)**:

- "What's the difference between ServiceA and ServiceB?"
- "Why are there two sync modules?"
- "Compare the validation approaches in X and Y"

**Track 4 (Configuration)**:

- "What environment variables are required?"
- "How is the database configured?"
- "What runtime configurations exist?"

**Track 5 (Temporal)**:

- "How does temporal ownership work?"
- "What entities use validFrom/validTo?"
- "How are date ranges validated?"

---

## Migration Path

### Schema Changes (V0006)

```cypher
// New edge types
CREATE CONSTRAINT concern_edge IF NOT EXISTS
FOR ()-[r:IMPLEMENTS_CONCERN]-() REQUIRE r.concern IS NOT NULL;

CREATE CONSTRAINT comparison_edge IF NOT EXISTS
FOR ()-[r:COMPARED_IN]-() REQUIRE r.created_at IS NOT NULL;

// New node labels
// :ConcernNode - cross-cutting concern abstractions
// :ComparisonNode - architectural comparisons
// :ConfigurationNode - configuration summaries
```

### Backward Compatibility

- All changes are additive (new edge types, new node types)
- Existing queries continue to work
- New consolidation creates new nodes alongside existing structure

---

## Success Criteria

### Quantitative

| Metric | Baseline (v4) | v9 Rerank | v10 Learning | v11 All Tracks | Target |
|--------|---------------|-----------|--------------|----------------|--------|
| Average retrieval score | 0.567 | 0.619 | 0.710 | **0.733 (+3.3%)** | 0.75+ |
| Cross-cutting questions | 0.45 | 0.52 | 0.687 | **0.709 (+3.2%)** | 0.70+ ✅ |
| Architecture questions | ~0.50 | 0.58 | 0.724 | **0.750 (+3.6%)** | 0.72+ ✅ |
| Service relationships | ~0.50 | 0.55 | 0.727 | **0.746 (+2.6%)** | 0.72+ ✅ |
| Business logic | ~0.45 | 0.50 | 0.686 | **0.728 (+6.1%)** | 0.68+ ✅ |
| Data flow | ~0.55 | 0.60 | 0.724 | **0.719 (-0.7%)** | 0.72+ ✅ |
| Learning edges created | 0 | 0 | 8,622 | **8,748** | ✅ ACHIEVED |
| Score >0.7 rate | ~10% | ~25% | 64% | **75%** | 70%+ ✅ |
| Score >0.8 rate | ~2% | ~5% | ~8% | **10%** | - |
| Max score | ~0.72 | ~0.78 | 0.814 | **0.866** | - |

### Qualitative

- [x] Retrieval returns concern/comparison nodes when appropriate
- [x] Generated summaries are accurate and useful
- [x] No regression in high-performing categories (data flow: 0.719, minor -0.7%)

---

## Phases 92-100: Deployable MDEMG Package Roadmap

**Added**: 2026-02-23
**Gap Analysis**: `docs/specs/phase92-gap-analysis.md`
**Goal**: Package MDEMG as a developer tool installable via `brew install mdemg` or curl installer.

### Phase Dependency Chain

```
92 (Gap Analysis) → 93 (Unified CLI) → 94 (Config + Init) → 96 (IDE + Repo)
                                     → 95 (DB + Embed)    → 97 (Lifecycle + Security)
                                                           → 98 (Build + Release)
                                                           → 99 (Onboarding)
                                                           → 100 (Deployable Package)
```

### Phase Summary

| Phase | Name | Effort | Status |
|-------|------|--------|--------|
| 92 | Gap Analysis | S | ✅ Complete |
| 93 | Unified CLI Foundation (Cobra, merge 12 binaries) | XL | 📋 Planned |
| 94 | Config Simplification + Project Init (YAML, mdemg init) | L | 📋 Planned |
| 95 | Database + Embedding + Migrations (Go runner, managed Neo4j) | L | 📋 Planned |
| 96 | IDE + Repo Integration (MCP auto-config, .mdemgignore) | M | 📋 Planned |
| 97 | Process Lifecycle + Security (daemon, keychain) | M | 📋 Planned |
| 98 | Cross-Platform Build + Release (goreleaser, Homebrew) | L | ✅ Complete |
| 99 | Onboarding + Polish (quickstart, demo, FAQ) | M | ✅ Complete |
| 100 | Deployable Package — Mac (integration test) | S | ✅ Complete |

### Phase 100 Acceptance Criteria

1. `brew install mdemg` succeeds on macOS (arm64 + amd64)
2. `mdemg init` in a fresh git repo creates `.mdemg/`, detects environment
3. `mdemg db start` launches Neo4j; `mdemg db migrate` applies schemas
4. `mdemg ingest .` indexes the codebase
5. `mdemg start` runs server + MCP in background
6. IDE can discover and use MCP tools
7. CMS observe/resume endpoints work
8. Retrieval returns relevant results
9. RSIC cycle runs on schedule
10. `mdemg upgrade` self-updates

See `docs/specs/phase92-gap-analysis.md` for the complete 15-gap analysis with current state, required state, effort estimates, and dependencies.

---

## Appendix: File Locations

### Core Files to Modify

```
mdemg_build/
├── service/
│   ├── cmd/ingest-codebase/     # Track 2.1, 4.1, 5.1
│   │   └── main.go              # Add pattern detection
│   └── internal/
│       ├── consolidation/        # Track 2.2, 3.2, 4.3, 5.3
│       │   └── service.go        # Add concern/comparison node creation
│       ├── learning/             # Track 1
│       │   └── service.go        # Debug/enable edge creation
│       └── retrieval/            # Track 4.2
│           └── scoring.go        # Add config boost
└── migrations/
    └── V0006__improvement_tracks.cypher  # New schema
```

### Test Files

```
docs/tests/
├── track_1_learning_questions.json
├── track_2_concern_questions.json
├── track_3_comparison_questions.json
├── track_4_config_questions.json
└── track_5_temporal_questions.json
```
