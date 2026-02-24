# Relationship Extraction & Graph Topology Hardening (Phase 75)

Phase 75 extends MDEMG's symbol indexing from **declarations** (what exists) to **relationships** (how things connect). It also hardens the Neo4j graph schema with secondary labels, semantic edge disambiguation, and dynamic edge materialization.

---

## Overview

### Problem

MDEMG parsers extract symbols (`func Retrieve()` in `service.go`) but not how they relate (`Retrieve()` calls `ComputeActivation()` in `activation.go`). The graph knows **what exists** but not **how things connect**. Additionally, a codebase audit found six structural issues: MemoryNode label overload, GENERALIZES semantic overload, SymbolNode disconnected from activation, inconsistent edge properties, dormant dynamic edge types, and generic L4/L5 consolidation.

### Solution

Two parallel tracks:

- **Phase 75A — Relationship Extraction**: Tree-sitter query engine extracts `IMPORTS`, `CALLS`, `EXTENDS`, `IMPLEMENTS` edges between SymbolNodes. Cross-file resolution links references to their definitions.
- **Phase 75B — Topology Hardening**: Secondary labels on MemoryNodes, `THEME_OF` edge disambiguation, dynamic edge materialization with proper Neo4j relationship types, and L5 emergent concept layer.

---

## Architecture

### Relationship Extraction Pipeline

```
Source Code
    |
    v
[Tree-sitter Parser] ──> Symbols (existing)
    |
    v
[Query Engine (.scm)] ──> Raw Relationships (new)
    |
    v
[Resolver] ──> Resolved RelationshipRecords
    |
    v
[Neo4j Writer] ──> IMPORTS/CALLS/EXTENDS/IMPLEMENTS edges
```

### 5-Tier Extraction Model

| Tier | Type | Method | Confidence |
|------|------|--------|------------|
| 1 | IMPORTS | Tree-sitter `.scm` queries | 0.9 |
| 2 | EXTENDS, IMPLEMENTS | Tree-sitter `.scm` queries | 0.9 |
| 3 | CALLS | Tree-sitter `.scm` queries (capped at 50/file) | 0.9 |
| 4 | Cross-file resolution | Neo4j lookup (same-file > same-package > global) | 0.5-1.0 |
| 5 | go/types deep analysis | Stub (deferred — `golang.org/x/tools` not in go.mod) | N/A |

### Topology Hardening

| Fix | Description |
|-----|-------------|
| Secondary labels | `:MemoryNode:HiddenPattern`, `:MemoryNode:Concept`, etc. (10 labels) |
| THEME_OF edge | Conversation observation-to-theme edges separated from code GENERALIZES |
| Dynamic edges | `ANALOGOUS_TO`, `CONTRASTS_WITH`, `COMPOSES_WITH`, etc. as proper Neo4j relationship types |
| L5 emergent layer | Meta-patterns spanning L4 concepts, created from high-evidence cross-domain bridges |
| BaseEdgeProperties | Standardized edge metadata (edge_id, timestamps, version, weight, evidence_count) |
| SymbolNode activation | CO_ACTIVATED_WITH edges between co-retrieved SymbolNodes |

---

## Configuration

All configuration is via environment variables with sensible defaults.

### Relationship Extraction (Phase 75A)

```bash
REL_EXTRACT_IMPORTS=true           # Enable import relationship extraction (default: true)
REL_EXTRACT_INHERITANCE=true       # Enable inheritance/implements extraction (default: true)
REL_EXTRACT_CALLS=true             # Enable call expression extraction (default: true)
REL_CROSS_FILE_RESOLVE=true        # Enable cross-file symbol resolution (default: true)
GO_TYPES_ANALYSIS_ENABLED=false    # Enable go/types analysis - stub (default: false)
REL_MAX_CALLS_PER_FUNCTION=50      # Max CALLS edges per file (default: 50)
REL_BATCH_SIZE=500                 # Edges per Neo4j transaction (default: 500)
REL_RESOLUTION_TIMEOUT_SEC=60      # Max time for cross-file resolution (default: 60)
```

### Topology Hardening (Phase 75B)

```bash
DYNAMIC_EDGES_ENABLED=true         # Create typed dynamic edges during consolidation (default: true)
DYNAMIC_EDGE_DEGREE_CAP=10         # Max dynamic edges per node (default: 10)
DYNAMIC_EDGE_MIN_CONFIDENCE=0.5    # Minimum confidence for dynamic edge creation (default: 0.5)
L5_EMERGENT_ENABLED=true           # Enable L5 emergent concept layer (default: true)
L5_BRIDGE_EVIDENCE_MIN=1           # Minimum bridge evidence to trigger L5 (default: 1)
SYMBOL_ACTIVATION_ENABLED=true     # Enable SymbolNode co-activation learning (default: true)
SECONDARY_LABELS_ENABLED=true      # Apply secondary labels to MemoryNodes (default: true)
THEME_OF_EDGE_ENABLED=true         # Use THEME_OF for conversation edges (default: true)
L5_SOURCE_MIN_LAYER=3              # Minimum layer for L5/dynamic edge source nodes (default: 3)
```

---

## API Endpoints

### GET /v1/symbols/relationships

Returns relationship edge counts by type for a space.

**Query Parameters:**

- `space_id` (required): The space to query

**Response:**

```json
{
  "space_id": "mdemg-dev",
  "counts": {
    "IMPORTS": 1234,
    "CALLS": 5678,
    "EXTENDS": 12,
    "IMPLEMENTS": 44
  }
}
```

### GET /v1/symbols/{id}/relationships

Returns incoming and outgoing relationships for a specific symbol.

**Query Parameters:**

- `space_id` (required): The space to query

**Response:**

```json
{
  "space_id": "mdemg-dev",
  "symbol_id": "abc123",
  "relationships": [
    {
      "source_symbol_id": "abc123",
      "target_symbol_id": "def456",
      "relation": "CALLS",
      "tier": 3,
      "confidence": 0.9,
      "resolution_method": "same_file"
    }
  ],
  "count": 1
}
```

---

## How It Works

### Query Engine

The query engine loads `.scm` (S-expression) files from `internal/symbols/queries/` at compile time via `//go:embed`. Each file contains a tree-sitter query pattern for a specific relationship type in a specific language.

**Supported languages and queries:**

| Language | imports | inheritance | implements | calls |
|----------|:-------:|:-----------:|:----------:|:-----:|
| Go | Yes | N/A | N/A | Yes |
| Python | Yes | Yes | N/A | Yes |
| TypeScript | Yes | Yes | Yes | Yes |
| Rust | Yes | Yes | N/A | Yes |
| Java | Yes | Yes | N/A | Yes |
| C | Yes | N/A | N/A | Yes |
| C++ | Yes | Yes | N/A | Yes |

### Cross-File Resolution

The resolver matches raw relationship targets to actual SymbolNode IDs in Neo4j using a tiered priority:

1. **Same-file** (confidence 1.0): Target symbol defined in the same file
2. **Same-package** (confidence 0.9): Target symbol in the same directory/package
3. **Global unique** (confidence 0.5): Exactly one symbol with that name in the space
4. **Unresolved**: Skipped — no matching symbol found

### Dynamic Edge Materialization

During consolidation, `CreateDynamicEdges()` finds pairs of L4+ nodes that should be connected and uses `InferEdgeType()` to determine the relationship:

| Condition | Edge Type |
|-----------|-----------|
| High similarity + same layer | ANALOGOUS_TO |
| Low similarity + high co-activation | CONTRASTS_WITH |
| High co-activation + moderate similarity | COMPOSES_WITH |
| Cross-layer + moderate similarity | BRIDGES |
| Layer difference + similarity | SPECIALIZES or GENERALIZES_TO |
| Default | INFLUENCES |

Edges are created as proper Neo4j relationship types (not generic `DYNAMIC_EDGE`), with MERGE for idempotency and evidence_count tracking.

### L5 Emergent Layer

L5 nodes represent meta-patterns that span multiple L3+ concepts:

1. Query L3+ nodes (configurable via `L5_SOURCE_MIN_LAYER`, default: 3) connected by high-evidence ANALOGOUS_TO, BRIDGES, or COMPOSES_WITH edges
2. Group into clusters using union-find
3. Create L5 `:MemoryNode:EmergentConcept` nodes with ABSTRACTS_TO edges from members
4. Requires minimum `L5_BRIDGE_EVIDENCE_MIN` evidence count (default: 1)

> **Phase 75C:** The L5 step runs as a post-clustering pipeline step (phase 30) via `RunPhaseRange(25, 30)`. Dynamic edges (phase 25) are created first, ensuring qualifying edges exist before L5 clustering runs.

### SymbolNode Co-Activation

When symbols are co-retrieved (connected to co-activated MemoryNodes via DEFINES_SYMBOL), CO_ACTIVATED_WITH edges form between the SymbolNodes. This brings symbols into the Hebbian learning loop:

- Initial weight: 0.1, increments by 0.05 per co-retrieval, max 1.0
- Guarded by `SYMBOL_ACTIVATION_ENABLED` and learning freeze state

---

## Neo4j Migrations

| Migration | Purpose |
|-----------|---------|
| V0014 | Indexes for IMPORTS, CALLS, EXTENDS, IMPLEMENTS edges |
| V0015 | Secondary labels (10 types) + btree indexes on each |
| V0016 | Convert conversation GENERALIZES to THEME_OF |
| V0017 | Indexes for dynamic edge types + THEME_OF + DEFINES_SYMBOL |

---

## Adding a New Query Pattern

To add relationship extraction for a new language or relationship type:

1. Create a `.scm` file in `internal/symbols/queries/<language>/`:

   ```
   ;; Example: internal/symbols/queries/kotlin/imports.scm
   (import_header
     (identifier) @import_path)
   ```

2. The query engine loads all `.scm` files automatically via `//go:embed`

3. File naming convention determines the relationship type:
   - `imports.scm` → IMPORTS (Tier 1)
   - `inheritance.scm` → EXTENDS (Tier 2)
   - `implements.scm` → IMPLEMENTS (Tier 2)
   - `calls.scm` → CALLS (Tier 3)

4. Run tests: `go test ./internal/symbols/... -v`

---

## Monitoring

### Check relationship counts

```bash
curl -s "http://localhost:9999/v1/symbols/relationships?space_id=mdemg-dev" | jq
```

### Check dynamic edge creation during consolidation

```bash
curl -s -X POST http://localhost:9999/v1/memory/consolidate \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}' | jq '.data.dynamic_edges_created, .data.l5_nodes_created'
```

### Verify secondary labels

```cypher
MATCH (n:HiddenPattern) RETURN count(n);
MATCH (n:Concept) RETURN count(n);
MATCH (n:EmergentConcept {layer: 5}) RETURN count(n);
```

### Verify THEME_OF edges

```cypher
MATCH ()-[r:THEME_OF]->() RETURN count(r);
-- Should be >0 for spaces with conversation themes
```

---

## Key Files

| File | Purpose |
|------|---------|
| `internal/symbols/query_engine.go` | Tree-sitter query engine (loads `.scm` files via embed) |
| `internal/symbols/queries/` | 20 `.scm` query files across 7 languages |
| `internal/symbols/relationships.go` | Neo4j writer: SaveRelationships, QueryRelationships, RelationshipStats |
| `internal/symbols/resolver.go` | Cross-file resolution (same-file > same-package > global) |
| `internal/symbols/go_types.go` | Tier 5 stub (deferred) |
| `internal/models/edge_properties.go` | BaseEdgeProperties() standardized edge metadata |
| `internal/api/handlers_relationships.go` | 2 HTTP handlers for relationship endpoints |
| `internal/hidden/step_dynamic_edges.go` | Dynamic edge pipeline step (phase 25) |
| `internal/hidden/step_emergent_l5.go` | L5 emergent pipeline step (phase 30) |
| `internal/hidden/service.go` | CreateDynamicEdges (fixed), CreateL5EmergentNodes, THEME_OF + secondary labels |
| `internal/learning/service.go` | ApplySymbolCoactivation |
| `internal/config/config.go` | 16 new configuration fields |
| `internal/retrieval/activation.go` | EdgeAttentionWeights for 11 new edge types |
| `migrations/V0014-V0017` | Schema migrations for edges, labels, THEME_OF, dynamic indexes |
