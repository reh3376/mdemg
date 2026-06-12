# UPTS Parser Summary

**Generated:** 2026-02-07
**Total Parsers:** 28 (PHP added 2026-04) UPTS-validated
**Pass Rate:** 100% (28/28)

---

## Parser Overview Table

| # | Name | Parser Type | File Extensions | Is Child Of | Has Children | Patterns Covered | Symbol Count | Key Features |
|---|------|-------------|-----------------|-------------|--------------|------------------|--------------|--------------|
| 1 | Go | AST-based | `.go` | - | - | P1,P2,P3,P4,P6,P7 (6/7) | 17-22 | Evidence validation enabled; no true enums in Go |
| 2 | Rust | Regex | `.rs` | - | - | 6 patterns | 27-42 | Traits, macros, modules (evidence validation NOT enabled — go/protobuf/graphql only) |
| 3 | Python | Regex | `.py`, `.pyi` | - | - | P1-P7 (7/7) | 35-40 | Protocol, dataclass, async support |
| 4 | TypeScript | Regex | `.ts`, `.tsx`, `.mts`, `.cts` | - | JS/JSX/TSX | P1-P7 (7/7) | 13-18 | Decorators, NestJS patterns, arrow functions |
| 5 | Java | Regex | `.java` | - | - | 5 patterns | 54-69 | Brace-depth scope tracking; annotations |
| 6 | C# | Regex | `.cs` | - | - | 7 patterns | 40-55 | Brace-depth scope tracking; records, properties |
| 7 | Kotlin | Regex | `.kt`, `.kts` | - | - | 6 patterns | 35-50 | Data/sealed/abstract classes, objects, companion |
| 8 | C++ | Regex | `.cpp`, `.hpp`, `.cc`, `.cxx`, `.hh`, `.hxx` | C | CUDA | 5 patterns | 73-88 | Templates, namespaces, classes |
| 9 | C | Regex | `.c`, `.h` | - | C++, CUDA | 6 patterns | 36-51 | Macros, structs, typedefs, function pointers |
| 10 | CUDA | Regex | `.cu`, `.cuh` | C/C++ | - | 4 patterns | 20-35 | Kernel functions, device functions, shared memory |
| 11 | SQL | Regex | `.sql` | - | - | 5 patterns | 25-40 | Tables, views, procedures, triggers, indexes |
| 12 | Cypher | Regex | `.cypher`, `.cql` | - | - | 4 patterns | 15-25 | Neo4j: labels, constraints, indexes, relationships |
| 13 | Terraform | Regex | `.tf`, `.tfvars` | - | - | 5 patterns | 20-35 | HCL: resources, data, modules, variables, outputs |
| 14 | YAML | Regex | `.yaml`, `.yml` | - | OpenAPI | 3 patterns | 20-30 | Hierarchical key paths; anchors/aliases |
| 15 | TOML | Regex | `.toml` | - | - | 3 patterns | 15-25 | Sections, key-value pairs, inline tables |
| 16 | JSON | Regex | `.json` | - | - | 2 patterns | 10-20 | Object keys, nested structures |
| 17 | INI | Regex | `.ini`, `.cfg`, `.env` | - | - | 2 patterns | 10-20 | Sections and key-value pairs |
| 18 | Makefile | Regex | `.mk`, `Makefile`, `GNUmakefile` | Shell | - | 4 patterns | 15-25 | Targets, variables, .PHONY, define/endef |
| 19 | Dockerfile | Regex | `Dockerfile`, `*.dockerfile` | - | - | 5 patterns | 10-20 | Instructions, multi-stage builds, ARG/ENV |
| 20 | Shell | Regex | `.sh`, `.bash`, `.zsh` | - | Makefile | 4 patterns | 15-25 | Functions, variables, aliases, exports |
| 21 | Protocol Buffers | Regex | `.proto` | - | - | P1,P3,P4,P5,P6,P8,P9,P10 (9) | 55-70 | gRPC services, messages, enums, nested types, oneof |
| 22 | GraphQL | Regex | `.graphql`, `.gql` | - | - | 11 patterns | 55-65 | Types, interfaces, inputs, enums, unions, directives, Query/Mutation/Subscription |
| 23 | OpenAPI | Regex | `.yaml`, `.yml`, `.json` (with openapi marker) | YAML | - | 8 patterns | 30-40 | REST endpoints, HTTP methods, operationIds, schemas, security |
| 24 | Markdown | Regex | `.md`, `.markdown` | - | - | 4 patterns | 15-25 | Headings (H1-H4), code blocks, links |
| 25 | XML | Regex | `.xml`, `.csproj`, `.vbproj` | - | - | 4 patterns | 8-15 | .NET project files: PackageReference, Target, PropertyGroup |
| 26 | Lua | Regex | `.lua` | - | - | 4 patterns | 15-25 | Functions, local variables, tables, metatables |
| 27 | Scraper Markdown | Regex (delegates to Markdown) | `.scraped.md` | Markdown | - | 5 patterns | 20-28 | Web-scraped docs: headings, code blocks, links; used by scraper section chunking |

---

## Parser Categories

### Systems / Embedded (4)

| Parser | Notes |
|--------|-------|
| C | Foundation for C++ and CUDA; macros, structs, typedefs |
| C++ | Extends C; templates, namespaces, classes, methods |
| CUDA | Extends C/C++; GPU kernels, device functions, shared memory |
| Rust | Traits, macros, modules |

### JVM Languages (2)

| Parser | Notes |
|--------|-------|
| Java | Brace-depth scope tracking; annotations, generics |
| Kotlin | Data/sealed classes, objects, companion objects, extension functions |

### .NET Languages (1)

| Parser | Notes |
|--------|-------|
| C# | Brace-depth scope tracking; records, properties, attributes, namespaces |

### Scripting Languages (4)

| Parser | Notes |
|--------|-------|
| Python | Protocol, dataclass, async/await, decorators |
| TypeScript | Includes JS/JSX/TSX; decorators, NestJS patterns |
| Shell | Functions, variables, aliases, exports |
| Lua | Functions, local variables, module tables, metatables |

### Configuration Languages (4)

| Parser | Notes |
|--------|-------|
| YAML | Hierarchical key paths; anchors/aliases |
| TOML | Sections, inline tables, arrays |
| JSON | Object structure, nested keys |
| INI | Sections and key-value pairs |

### Infrastructure / DevOps (3)

| Parser | Notes |
|--------|-------|
| Terraform | HCL: resources, data sources, modules, variables, outputs |
| Dockerfile | Instructions, multi-stage builds, ARG/ENV |
| Makefile | Targets, variables, .PHONY tracking |

### Database Languages (2)

| Parser | Notes |
|--------|-------|
| SQL | Tables, views, procedures, triggers, indexes |
| Cypher | Neo4j: labels, constraints, indexes |

### API Schema Languages (3)

| Parser | Notes |
|--------|-------|
| Protocol Buffers | gRPC services, messages, enums, nested types, oneof |
| GraphQL | Types, interfaces, inputs, unions, Query/Mutation/Subscription |
| OpenAPI | REST endpoints, HTTP methods, operationIds, schemas, security schemes |

### Documentation / Markup (3)

| Parser | Notes |
|--------|-------|
| Markdown | Headings, code blocks, links |
| XML | .NET project files, build targets |
| Scraper Markdown | Delegates to Markdown parser; used by web scraper for section chunking |

---

## Parent-Child Relationships

```
C ─┬─> C++
   └─> CUDA ─> (extends C++ patterns)

Shell ─> Makefile (recipe syntax)

YAML ─> OpenAPI (uses YAML parsing as base)

TypeScript ─> JavaScript, JSX, TSX (file extension variants)

Markdown ─> Scraper Markdown (delegates symbol extraction)
```

---

## Canonical Patterns Coverage

| Pattern | Code | Description | Parsers Supporting |
|---------|------|-------------|-------------------|
| P1 | `P1_CONSTANT` | Named constant values | All 27 |
| P2 | `P2_FUNCTION` | Standalone functions | 23/27 (not INI, JSON, Markdown, Scraper Markdown) |
| P3 | `P3_CLASS_STRUCT` | Classes/structs/messages | 18/27 |
| P4 | `P4_INTERFACE_TRAIT` | Interfaces/traits/protocols | 12/27 |
| P5 | `P5_ENUM` | Enumerations | 15/27 |
| P6 | `P6_METHOD` | Class/struct methods | 15/27 |
| P7 | `P7_TYPE_ALIAS` | Type aliases | 8/27 |

### Extended Patterns (Domain-Specific)

| Pattern | Domain | Description |
|---------|--------|-------------|
| P8_ENUM, P8_ENUM_VALUE | Protobuf | Enum and enum values |
| P9_FIELD | Protobuf, GraphQL | Message/type fields |
| P10_SECTION | Protobuf | Oneof sections |
| P_SCALAR | GraphQL | Custom scalar types |
| P_INPUT | GraphQL | Input types |
| P_UNION | GraphQL | Union types |
| P_DIRECTIVE | GraphQL | Directives (@auth, @deprecated) |
| P_ROOT_OPERATION | GraphQL | Query/Mutation/Subscription |
| P_PATH | OpenAPI | REST endpoints |
| P_METHOD | OpenAPI | HTTP methods (GET, POST, etc.) |
| P_OPERATION_ID | OpenAPI | Operation identifiers |
| P_PARAMETER | OpenAPI | Request parameters |
| P_SCHEMA | OpenAPI | Component schemas |
| P_HEADING | Markdown | Heading levels (H1-H4) |
| P_CODE_BLOCK | Markdown | Fenced code blocks |
| P_LINK | Markdown | Hyperlinks |

---

## Evidence Validation

Parsers with `validate_evidence: true` run additional structural consistency checks:

| Parser | Evidence Validation | Notes |
|--------|---------------------|-------|
| Go | ✅ Enabled | LineEnd, range consistency, containment |
| Rust | ✅ Enabled | LineEnd, range consistency, containment |
| Protocol Buffers | ✅ Enabled | Field numbers, nested message ranges |
| GraphQL | ✅ Enabled | Type definitions, field containment |
| All others | ❌ Disabled | Can opt in by setting `validate_evidence: true` |

---

## Validation Commands

```bash
# Run all UPTS tests
go test ./internal/languages/ -run TestUPTS -v

# Run single language test
go test ./internal/languages/ -run TestUPTS/kotlin -v

# Run with verbose output
go test ./internal/languages/ -run TestUPTS -v -count=1
```

---

## File Locations

| Resource | Path |
|----------|------|
| UPTS Specs | `docs/lang-parser/lang-parse-spec/upts/specs/*.upts.json` |
| Test Fixtures | `docs/lang-parser/lang-parse-spec/upts/fixtures/` |
| Parser Implementations | `internal/languages/*_parser.go` |
| Test Harness | `internal/languages/upts_test.go` |
| Type Definitions | `internal/languages/upts_types.go` |
| JSON Schema | `docs/lang-parser/lang-parse-spec/upts/schema/upts.schema.json` |

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-01-28 | Initial UPTS framework, 16 parsers |
| 1.1 | 2026-01-29 | Schema fix (fixture paths), runner fixes |
| 1.2 | 2026-02-04 | Added C#, Kotlin, Terraform, Makefile |
| 1.3 | 2026-02-05 | Added Protocol Buffers, GraphQL |
| 1.4 | 2026-02-05 | Added OpenAPI, Markdown, XML; 25 parsers total |
| 1.5 | 2026-02-07 | Added Lua, Scraper Markdown; 27 parsers total |
