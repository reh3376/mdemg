# LSP vs UPTS: Research Analysis for MDEMG Ingestion

**Date:** 2026-01-22
**Purpose:** Determine if LSP language servers could benefit MDEMG code ingestion
**Status:** Research Complete — Decision Made
**Outcome:** LSP deferred in favor of AST-native relationship extraction. See revised spec: [`docs/specs/phase75-relationship-extraction.md`](../specs/phase75-relationship-extraction.md)

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [What LSP Actually Is](#2-what-lsp-actually-is)
3. [What UPTS Actually Is](#3-what-upts-actually-is)
4. [Field-by-Field Data Comparison](#4-field-by-field-data-comparison)
5. [What LSP Can Provide That UPTS Cannot](#5-what-lsp-can-provide-that-upts-cannot)
6. [What UPTS Provides That LSP Cannot](#6-what-upts-provides-that-lsp-cannot)
7. [Overlap Analysis](#7-overlap-analysis)
8. [Existing Projects Using LSP for Code Graphs](#8-existing-projects-using-lsp-for-code-graphs)
9. [Performance and Operational Characteristics](#9-performance-and-operational-characteristics)
10. [Evidence Assessment: Should MDEMG Adopt LSP?](#10-evidence-assessment-should-mdemg-adopt-lsp)
11. [Recommendation](#11-recommendation)

---

## 1. Executive Summary

**Question:** Is there strong indication that LSP could be beneficial for MDEMG ingestion?

**Answer:** **Yes, but narrowly.** LSP provides one category of data that UPTS fundamentally cannot: **cross-file semantic relationships** (what calls what, what implements what, what imports what). This is high-value data for MDEMG's memory graph. However, for the core ingestion task of symbol extraction (what exists and where), UPTS is equal or superior to LSP in every measurable dimension — speed, dependency footprint, language coverage, error tolerance, and output consistency.

**The strong indication is specifically for cross-file edges, not for replacing UPTS.**

| Capability | UPTS | LSP | Winner for MDEMG |
|------------|------|-----|------------------|
| Symbol extraction (declarations) | ✅ Full | ✅ Full | **Tie** — both extract the same declarations |
| Cross-file references | ❌ None | ✅ Full | **LSP** — UPTS cannot do this |
| Call graph (who calls whom) | ❌ None | ✅ Full | **LSP** — UPTS cannot do this |
| Interface implementations | ❌ None | ✅ Full | **LSP** — UPTS cannot do this |
| Import/dependency graph | ❌ None | ✅ Full | **LSP** — UPTS cannot do this |
| Type resolution | ❌ None | ✅ Full | **LSP** — UPTS cannot do this |
| Language coverage | 25 languages | 3-5 mature servers | **UPTS** — far broader |
| Speed (per file) | <1ms | 50-100ms | **UPTS** — 50-100x faster |
| External dependencies | Zero | Docker + per-lang server | **UPTS** — self-contained |
| Error tolerance | High (regex/AST) | Variable (some crash on broken code) | **UPTS** — more robust |
| Config/data file parsing | ✅ Full (YAML, TOML, JSON, INI, etc.) | ❌ No servers exist | **UPTS** — only option |
| Constant value extraction | ✅ Full | ❌ Not a standard LSP feature | **UPTS** — unique strength |

---

## 2. What LSP Actually Is

### Protocol, Not Parser

LSP (Language Server Protocol) is a **JSON-RPC communication protocol** between an editor (client) and a language-specific analysis server. It was created by Microsoft for VS Code to solve the "N × M problem" — instead of every editor implementing support for every language, each language has one server that speaks a standard protocol.

### How It Works

```
Client (editor/tool)                    Server (gopls, pyright, etc.)
      │                                        │
      ├── initialize ────────────────────────> │  (handshake, declare capabilities)
      │ <──────────────────────── initialized ──┤
      │                                        │
      ├── textDocument/didOpen ──────────────> │  (open a file)
      │                                        │
      ├── textDocument/documentSymbol ───────> │  (list symbols in file)
      │ <──────────────── DocumentSymbol[] ────┤
      │                                        │
      ├── textDocument/references ───────────> │  (find all references to symbol)
      │ <──────────────────── Location[] ──────┤
      │                                        │
      ├── textDocument/definition ───────────> │  (go to definition)
      │ <──────────────────── Location[] ──────┤
      │                                        │
      ├── callHierarchy/outgoingCalls ───────> │  (what does this function call?)
      │ <──────── CallHierarchyItem[] ─────────┤
      │                                        │
      ├── shutdown ──────────────────────────> │
      └── exit ──────────────────────────────> │
```

### LSP SymbolKind Enum (26 values)

LSP classifies symbols using the `SymbolKind` enum:

| # | Kind | # | Kind |
|---|------|---|------|
| 0 | File | 13 | Constant |
| 1 | Module | 14 | String |
| 2 | Namespace | 15 | Number |
| 3 | Package | 16 | Boolean |
| 4 | Class | 17 | Array |
| 5 | Method | 18 | Object |
| 6 | Property | 19 | Key |
| 7 | Field | 20 | Null |
| 8 | Constructor | 21 | EnumMember |
| 9 | Enum | 22 | Struct |
| 10 | Interface | 23 | Event |
| 11 | Function | 24 | Operator |
| 12 | Variable | 25 | TypeParameter |

### LSP DocumentSymbol Structure

Each symbol returned by `textDocument/documentSymbol`:

```json
{
  "name": "ParseFile",
  "detail": "func(root, path string) ([]CodeElement, error)",
  "kind": 11,
  "range": {
    "start": { "line": 38, "character": 0 },
    "end": { "line": 120, "character": 1 }
  },
  "selectionRange": {
    "start": { "line": 38, "character": 5 },
    "end": { "line": 38, "character": 14 }
  },
  "children": []
}
```

### Key LSP Methods Relevant to Ingestion

| Method | Returns | What It Tells You |
|--------|---------|-------------------|
| `textDocument/documentSymbol` | `DocumentSymbol[]` | Symbols in a file (name, kind, range) |
| `textDocument/references` | `Location[]` | All locations where a symbol is used |
| `textDocument/definition` | `Location[]` | Where a symbol is defined |
| `textDocument/implementation` | `Location[]` | Types that implement an interface |
| `textDocument/typeDefinition` | `Location[]` | Definition of a symbol's type |
| `callHierarchy/incomingCalls` | `CallHierarchyItem[]` | Functions that call this function |
| `callHierarchy/outgoingCalls` | `CallHierarchyItem[]` | Functions this function calls |
| `typeHierarchy/subtypes` | `TypeHierarchyItem[]` | Subtypes of a type |
| `typeHierarchy/supertypes` | `TypeHierarchyItem[]` | Supertypes of a type |
| `workspace/symbol` | `SymbolInformation[]` | Search all symbols across workspace |

### Major Language Servers

| Language | Server | Maturity | Backed By |
|----------|--------|----------|-----------|
| Go | gopls | Production | Google |
| Python | pyright | Production | Microsoft |
| TypeScript | tsserver | Production | Microsoft |
| Rust | rust-analyzer | Production | Community/Mozilla |
| Java | Eclipse JDT LS | Production | Eclipse Foundation |
| C/C++ | clangd | Production | LLVM/Google |
| C# | OmniSharp | Production | Microsoft |
| Kotlin | kotlin-language-server | Beta | JetBrains/Community |

---

## 3. What UPTS Actually Is

### Purpose-Built for MDEMG

UPTS (Universal Parser Test Specification) is a framework of 25 language-specific parsers designed specifically for extracting **symbol declarations** from source code for ingestion into the MDEMG memory graph. Each parser produces a normalized `Symbol` struct.

### UPTS Symbol Structure

```go
type Symbol struct {
    Name           string  // Symbol identifier
    Type           string  // "constant", "function", "class", "method", "interface", etc.
    Line           int     // 1-indexed start line
    LineEnd        int     // End line for multi-line symbols
    Exported       bool    // Public visibility
    Parent         string  // Parent class/struct for methods
    Signature      string  // Full function signature
    Value          string  // Constant value (evaluated)
    RawValue       string  // Original source text
    DocComment     string  // Documentation
    TypeAnnotation string  // Type annotation
    Language       string  // Source language
}
```

### UPTS Symbol Types (19 types)

```
constant, function, class, method, interface, enum, type, struct, trait,
macro, kernel, variable, field, module, namespace,
table, column, index, view, trigger, constraint,
label, relationship_type, section
```

### How UPTS Works

```
Source file (.go, .py, .rs, etc.)
    │
    ├── File extension lookup → select parser
    │
    ├── Parser processes file:
    │   ├── Go: go/ast + go/parser (real AST)
    │   ├── Rust: regex patterns
    │   ├── Python: regex patterns
    │   └── ... (25 parsers)
    │
    ├── Output: []CodeElement containing []Symbol
    │
    └── Symbols → SymbolRecord → Neo4j :SymbolNode
        with DEFINED_IN edge to file :MemoryNode
```

### What UPTS Extracts Per File

For each file, UPTS produces:

- **CodeElement**: File-level container (content, summary, package, concerns, diagnostics)
- **Symbol[]**: Individual declarations within the file

Each symbol becomes a `:SymbolNode` in Neo4j with a `DEFINED_IN` edge pointing to the file's `:MemoryNode`.

### UPTS Schema Already Defines Relationships

Notably, the UPTS JSON schema **already defines** a `Relationship` type:

```json
{
  "source": "MyService",
  "relation": "IMPLEMENTS",
  "target": "ServiceInterface"
}
```

Supported relationship types in the schema:

- `DEFINES_METHOD`
- `EXTENDS`
- `IMPLEMENTS`
- `IMPORTS`
- `CONTAINS`

**This is significant:** The UPTS schema was designed with relationship awareness, but the current regex parsers only extract declarations, not relationships. This is exactly the gap LSP could fill.

---

## 4. Field-by-Field Data Comparison

### Symbol Declaration Fields

| Field | UPTS | LSP DocumentSymbol | Equivalent? |
|-------|------|--------------------|-------------|
| **name** | `Name string` | `name string` | ✅ Identical |
| **type/kind** | `Type string` (19 custom values) | `kind SymbolKind` (26 enum values) | ⚠️ Overlapping but different taxonomies |
| **line** | `Line int` (1-indexed) | `range.start.line` (0-indexed) | ⚠️ Off-by-one conversion needed |
| **line_end** | `LineEnd int` | `range.end.line` | ⚠️ Off-by-one conversion needed |
| **exported** | `Exported bool` | Not a standard LSP field | ❌ UPTS-only |
| **parent** | `Parent string` | Implicit via `children` nesting | ⚠️ Derivable from LSP tree |
| **signature** | `Signature string` | `detail string` (similar) | ⚠️ Similar but format varies |
| **value** | `Value string` (evaluated constant) | Not available | ❌ UPTS-only |
| **raw_value** | `RawValue string` | Not available | ❌ UPTS-only |
| **doc_comment** | `DocComment string` | Not a standard DocumentSymbol field | ❌ UPTS-only (LSP provides via hover) |
| **type_annotation** | `TypeAnnotation string` | Not a standard field | ❌ UPTS-only |
| **column** | `Column int` | `selectionRange.start.character` | ✅ Equivalent |
| **children** | Not hierarchical | `children DocumentSymbol[]` | ❌ LSP has nested tree structure |
| **tags** | Not in Symbol (in CodeElement) | `tags SymbolTag[]` (deprecated flag) | ❌ Different semantics |

### Type Taxonomy Mapping

| UPTS Type | LSP SymbolKind | Notes |
|-----------|---------------|-------|
| `constant` | `Constant (13)` | Direct match |
| `function` | `Function (11)` | Direct match |
| `class` | `Class (4)` | Direct match |
| `method` | `Method (5)` | Direct match |
| `interface` | `Interface (10)` | Direct match |
| `enum` | `Enum (9)` | Direct match |
| `type` | `TypeParameter (25)` | Approximate |
| `struct` | `Struct (22)` | Direct match |
| `variable` | `Variable (12)` | Direct match |
| `field` | `Field (7)` | Direct match |
| `module` | `Module (1)` | Direct match |
| `namespace` | `Namespace (2)` | Direct match |
| `trait` | `Interface (10)` | Rust trait → LSP interface |
| `macro` | No equivalent | UPTS-only |
| `kernel` | No equivalent | UPTS-only (CUDA) |
| `table` | No equivalent | UPTS-only (SQL) |
| `column` | No equivalent | UPTS-only (SQL) |
| `view` | No equivalent | UPTS-only (SQL) |
| `trigger` | No equivalent | UPTS-only (SQL) |
| `constraint` | No equivalent | UPTS-only (Cypher) |
| `label` | No equivalent | UPTS-only (Cypher) |
| `relationship_type` | No equivalent | UPTS-only (Cypher) |
| `section` | No equivalent | UPTS-only (YAML/TOML/INI) |
| — | `Constructor (8)` | LSP-only |
| — | `Property (6)` | LSP-only |
| — | `EnumMember (21)` | LSP-only |
| — | `Event (23)` | LSP-only |
| — | `Operator (24)` | LSP-only |
| — | `Package (3)` | LSP-only |

**Finding:** 12 UPTS types map directly to LSP SymbolKinds. 11 UPTS types have no LSP equivalent (domain-specific: SQL, Cypher, config languages). 6 LSP kinds have no UPTS equivalent (mostly fine-grained OOP concepts).

---

## 5. What LSP Can Provide That UPTS Cannot

These are capabilities that are **fundamentally impossible** with regex/AST-only parsing:

### 5.1 Cross-File References

**LSP method:** `textDocument/references`

Given a symbol, LSP returns every location in the project where that symbol is used. This enables:

- "Where is this function called from?"
- "What files depend on this interface?"
- "How widely used is this constant?"

**MDEMG value:** Creates `REFERENCES` edges between `:SymbolNode` entities across files. Currently, the graph knows that `func ParseFile` exists in `go_parser.go` but has no idea that it's called from `main.go`, `service.go`, and 5 test files.

### 5.2 Call Graph (Call Hierarchy)

**LSP method:** `callHierarchy/incomingCalls`, `callHierarchy/outgoingCalls`

Returns the complete call tree for a function: what calls it (incoming) and what it calls (outgoing).

**MDEMG value:** Creates `CALLS` edges. Enables retrieval queries like "what is affected if I change this function?" — critical for impact analysis during code ingestion.

### 5.3 Interface Implementation Resolution

**LSP method:** `textDocument/implementation`

Given an interface, returns all concrete types that implement it. Given a concrete type, returns all interfaces it satisfies.

**MDEMG value:** Creates `IMPLEMENTS` edges. In Go, this is especially valuable because implementation is implicit (no `implements` keyword) — only the compiler (via gopls) can definitively resolve this.

### 5.4 Import/Dependency Graph

**LSP method:** `textDocument/definition` applied to import statements

Resolves import paths to actual file/package locations.

**MDEMG value:** Creates `IMPORTS` edges. Maps the dependency structure of the codebase.

### 5.5 Type Resolution

**LSP method:** `textDocument/typeDefinition`

Given a variable or parameter, returns the definition of its type.

**MDEMG value:** Could enrich `:SymbolNode` with resolved type information. Not as high-priority as relationships, but adds precision.

### 5.6 Semantic Understanding

LSP servers use actual compiler toolchains (Go's type checker, Python's type inference, TypeScript's compiler). They understand:

- Which `pop` function is in scope when multiple exist
- Whether a method satisfies an interface
- Generic type instantiation
- Build tags and conditional compilation

Regex parsers can never achieve this level of understanding.

---

## 6. What UPTS Provides That LSP Cannot

### 6.1 Constant Value Extraction

UPTS extracts and evaluates constant values:

```
Name: DEFAULT_FLUSH_INTERVAL
Type: constant
Value: 60000          ← Evaluated
RawValue: 60 * 1000   ← Original source
```

LSP's `documentSymbol` does not include constant values. The `detail` field sometimes contains type information but not values.

**MDEMG impact:** High. Constant values are critical evidence for the memory graph — they tell agents "what are the actual configured values" rather than just "a constant named X exists."

### 6.2 Domain-Specific Languages (11 UPTS types with no LSP equivalent)

No LSP servers exist for:

- **SQL**: tables, columns, views, triggers, indexes
- **Cypher**: labels, relationship types, constraints
- **YAML/TOML/JSON/INI**: config sections, key paths
- **Dockerfile**: stages, instructions, ARG/ENV
- **Makefile**: targets, variables, .PHONY
- **Protobuf**: messages, services, RPC methods
- **GraphQL**: types, queries, mutations
- **OpenAPI**: REST endpoints, HTTP methods, schemas

These represent 13 of UPTS's 25 parsers — over half the coverage.

**MDEMG impact:** Critical. MDEMG ingests entire codebases including infrastructure, config, and schema files. LSP simply cannot touch this space.

### 6.3 Documentation Comment Extraction

UPTS extracts doc comments (`DocComment` field) as part of symbol extraction. LSP provides documentation via the separate `textDocument/hover` method, which returns rendered markdown rather than raw doc strings.

### 6.4 Cross-Cutting Concern Detection

UPTS's `CodeElement` includes `Concerns []string` — cross-cutting concerns detected from file content (e.g., "authentication", "database", "logging"). This is a domain-specific MDEMG feature not present in LSP.

### 6.5 Error Tolerance and Diagnostics

UPTS parsers emit structured `Diagnostic` objects (TRUNCATED, LARGE_FILE, PARTIAL_PARSE, BINARY_DETECTED) and gracefully handle partial files. LSP servers have variable error tolerance — some crash or produce empty results on syntactically broken files.

---

## 7. Overlap Analysis

### Where They Do the Same Thing

For the 12 programming languages that have both UPTS parsers and mature LSP servers (Go, Python, TypeScript, Rust, Java, C#, Kotlin, C, C++, CUDA), both systems extract the same **declaration-level symbols**:

- Function definitions
- Class/struct/interface definitions
- Method definitions
- Constant definitions
- Enum definitions
- Type alias definitions

**For this overlap zone, UPTS is strictly better for MDEMG because:**

1. It's 50-100x faster (regex vs compiler)
2. It extracts additional fields LSP doesn't (Value, RawValue, DocComment, Exported)
3. It has zero external dependencies
4. Its output format is already normalized for Neo4j storage

**LSP provides no advantage for symbol declaration extraction.**

### Where They Don't Overlap

| Only UPTS | Only LSP |
|-----------|----------|
| Config languages (YAML, TOML, JSON, INI) | Cross-file references |
| Infrastructure (Dockerfile, Makefile, Terraform) | Call graph (incoming/outgoing calls) |
| Query languages (SQL, Cypher) | Interface implementation resolution |
| API schemas (Protobuf, GraphQL, OpenAPI) | Import dependency graph |
| Documentation (Markdown, XML) | Type resolution |
| Constant value extraction | Semantic scope understanding |
| Concern detection | — |
| Custom diagnostics | — |

**This is the key finding:** The overlap is in the least interesting area (basic declarations). The non-overlapping capabilities are where value lives — and they're complementary, not competitive.

---

## 8. Existing Projects Using LSP for Code Graphs

Several projects have already proven the viability of using LSP for code indexing and graph construction:

### 8.1 CodeGraphContext

- **What:** MCP server + CLI that indexes code into FalkorDB or Neo4j
- **How:** Uses tree-sitter (not LSP) for parsing, builds call graphs
- **Languages:** 12 languages
- **Relevance:** Does what MDEMG ingestion does but with a graph database backend. Uses tree-sitter for call graph construction, suggesting that even syntactic analysis can approximate some LSP capabilities.

### 8.2 lsproxy (Agentic Labs)

- **What:** REST API wrapping multiple language servers in Docker
- **How:** Runs LSP servers + ast-grep behind a unified API
- **Languages:** 15+ languages
- **Relevance:** Proves the "containerized LSP" approach is viable. Production-quality Rust implementation. Could potentially be used as-is rather than building from scratch.

### 8.3 LSAP (Language Server Agent Protocol)

- **What:** Semantic abstraction layer over LSP for AI agents
- **How:** Converts raw LSP responses into "agent-ready snapshots" (RPC → Markdown)
- **Languages:** Any LSP-supported language
- **Relevance:** Validates the concept of using LSP for AI agent code intelligence. Provides a Python SDK that could be used for batch processing.

### 8.4 LSP-MCP (jonrad)

- **What:** MCP server bridging LLMs with language servers
- **How:** Wraps LSP methods as MCP tools
- **Relevance:** Shows that LSP can be consumed by AI systems outside IDEs.

### 8.5 Code Pathfinder

- **What:** MCP server for semantic code analysis
- **How:** Call graph analysis, symbol search, dependency tracking
- **Relevance:** Enables natural language queries like "who calls this?" — similar to what MDEMG retrieval does with spreading activation.

**Conclusion from existing projects:** The "use LSP for code graph construction" approach is proven and actively being developed by multiple teams. MDEMG is not pioneering here — it would be joining an established pattern.

---

## 9. Performance and Operational Characteristics

### Speed Comparison

| Metric | UPTS | LSP (gopls) | LSP (pyright) |
|--------|------|-------------|---------------|
| Per-file symbol extraction | <1ms | 50-100ms | 50-100ms |
| Startup time | Instant (compiled in) | 2-5 seconds | 1-3 seconds |
| Memory per server | 0 (compiled in) | 200MB-1GB | 100MB-500MB |
| Cold start overhead | None | Language server + project indexing | Language server + project indexing |
| Project indexing | N/A | 5-30 seconds (depends on project size) | 3-15 seconds |

### Gopls Scalability (from Google's blog)

Gopls v0.12 achieved ~75% reduction in both memory and startup time through on-disk indexes. For a large Go repository:

- Memory: ~200MB (down from ~800MB)
- Startup: ~5 seconds (down from ~20 seconds)
- Subsequent queries: <100ms

### Pyright Scalability Concerns

Pyright's analyzer performs O(repo) operations on initialization. For very large Python repositories (500K+ modules), this can be problematic. For typical MDEMG-scale repositories (hundreds to low thousands of files), this is not an issue.

### Operational Requirements

| Requirement | UPTS | LSP |
|-------------|------|-----|
| Runtime dependency | None (compiled Go binary) | Docker + per-language container |
| Disk space | 0 additional | ~500MB per language server image |
| Network | None | Container networking |
| Maintenance | Update parsers with codebase | Pin server versions, update images |
| Failure modes | Parser crash → skip file | Server crash → restart container → retry |

---

## 10. Evidence Assessment: Should MDEMG Adopt LSP?

### Evidence FOR Integration

| Evidence | Strength | Impact on MDEMG |
|----------|----------|-----------------|
| LSP provides cross-file relationships UPTS cannot | **Strong** | New edge types (CALLS, IMPORTS, IMPLEMENTS) significantly enrich the graph |
| UPTS schema already defines relationship types (IMPLEMENTS, IMPORTS, etc.) but can't populate them | **Strong** | LSP fills an existing gap in the design |
| Multiple projects (CodeGraphContext, lsproxy, LSAP) prove the approach | **Strong** | Validated pattern, not experimental |
| Call hierarchy enables "impact analysis" queries in retrieval | **Strong** | Directly supports MDEMG's spreading activation model |
| Interface implementation resolution is impossible without compiler (especially Go) | **Strong** | High-value edges that no amount of regex can produce |
| gopls is production-quality, Google-backed, well-documented | **Strong** | Low risk for Go language server |
| Containerized approach (lsproxy) is proven | **Moderate** | Architecture already validated |

### Evidence AGAINST Integration

| Evidence | Strength | Impact on MDEMG |
|----------|----------|-----------------|
| UPTS already provides 100% of symbol declarations | **Strong** | LSP adds no value for the core declaration task |
| LSP covers only 3-5 languages vs UPTS's 25 | **Moderate** | Most of MDEMG's language coverage cannot benefit from LSP |
| Operational complexity (Docker, per-language servers) | **Moderate** | Significant infrastructure addition |
| LSP servers have variable stability | **Moderate** | Must handle crashes, timeouts, retries |
| Pyright has scalability concerns for large repos | **Weak** | Only affects very large Python monorepos |
| MDEMG is primarily a memory system, not code intelligence | **Moderate** | Cross-file edges are useful but not core to the memory mission |

### Net Assessment

The evidence **strongly supports** LSP as an **enrichment layer** for cross-file edges, specifically for Go, Python, and TypeScript. The evidence **does not support** LSP as a replacement for any UPTS functionality.

The key question is: **How much do cross-file edges improve MDEMG retrieval quality?**

If an agent queries "how does the retrieval pipeline work?" and the graph only has `DEFINED_IN` edges, spreading activation can only find symbols in the same file. With `CALLS` edges, activation can flow from `service.go:Retrieve()` → `activation.go:ComputeActivation()` → `scoring.go:Score()` — revealing the full pipeline across files.

This is a **qualitative improvement** in retrieval, not just a quantitative one.

---

## 11. Recommendation

### Verdict: **Adopt LSP as Optional Enrichment Layer**

The research supports the Phase 75 design as specified. The key refinements from this research:

### 11.1 Start with gopls Only

Go is MDEMG's implementation language and has the most mature, best-performing LSP server. Start with Go enrichment and measure the impact on retrieval quality before expanding to Python and TypeScript.

### 11.2 Focus on Call Hierarchy and Implementation Resolution

Of LSP's many capabilities, these two provide the highest-value edges for MDEMG:

- `callHierarchy/outgoingCalls` → `CALLS` edges
- `textDocument/implementation` → `IMPLEMENTS` edges
- `textDocument/references` → `REFERENCES` edges (lower priority — high fan-out, lower signal)

### 11.3 Evaluate lsproxy Before Building From Scratch

lsproxy (Agentic Labs) is a production-quality Rust implementation that already:

- Runs language servers in Docker
- Provides a REST API
- Supports 15+ languages
- Handles server lifecycle

Using lsproxy as the LSP backend could significantly reduce implementation effort for Phase 75.

### 11.4 Measure Retrieval Impact

Before committing to full production deployment, run a controlled experiment:

1. Ingest the MDEMG codebase with UPTS only (current state)
2. Manually add 50-100 `CALLS` edges based on gopls output
3. Run retrieval queries with and without the edges
4. Measure scoring differences

This provides concrete evidence of retrieval quality improvement before investing in the full Phase 75 implementation.

### 11.5 Do Not Replace UPTS

UPTS remains the primary parser for all 25 languages. LSP is strictly additive. If LSP is disabled, unavailable, or fails, the system operates normally on UPTS-extracted symbols.

---

## Sources

| Source | URL | Relevance |
|--------|-----|-----------|
| LSP 3.17 Specification | microsoft.github.io/language-server-protocol/specifications/lsp/3.17/ | Protocol specification |
| Gopls Feature Index | go.dev/gopls/features/ | gopls capabilities |
| Gopls Scalability Blog | go.dev/blog/gopls-scalability | Performance benchmarks |
| Tree-sitter vs LSP | lambdaland.org/posts/2026-01-21_tree-sitter_vs_lsp/ | Architectural comparison |
| lsproxy | github.com/agentic-labs/lsproxy | Containerized LSP for AI |
| LSAP | lsp-client.github.io/ | LSP abstraction for agents |
| CodeGraphContext | github.com/CodeGraphContext/CodeGraphContext | Code → graph indexing |
| LSP-MCP | github.com/jonrad/lsp-mcp | LSP → MCP bridge |
| Code Pathfinder | codepathfinder.dev/mcp | Semantic code MCP |
| Hybrid IDE Architecture | byteiota.com/tree-sitter-vs-lsp | Hybrid approach rationale |
