# Language Parser Framework

This directory contains modular language parsers for the MDEMG codebase ingestion tool. Each programming language is implemented as a separate parser that auto-registers at program startup.

## Supported Languages

27 parsers, 27 with UPTS validation specs (100%).

| Language | File | Extensions | UPTS |
|----------|------|------------|------|
| Go | [`go_parser.go`](go_parser.go) | `.go` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/go.upts.json) |
| Rust | [`rust_parser.go`](rust_parser.go) | `.rs` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/rust.upts.json) |
| Python | [`python_parser.go`](python_parser.go) | `.py` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/python.upts.json) |
| TypeScript/JavaScript | [`typescript_parser.go`](typescript_parser.go) | `.ts`, `.tsx`, `.js`, `.jsx` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/typescript.upts.json) |
| Java | [`java_parser.go`](java_parser.go) | `.java` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/java.upts.json) |
| C# | [`csharp_parser.go`](csharp_parser.go) | `.cs` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/csharp.upts.json) |
| Kotlin | [`kotlin_parser.go`](kotlin_parser.go) | `.kt`, `.kts` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/kotlin.upts.json) |
| C++ | [`cpp_parser.go`](cpp_parser.go) | `.cpp`, `.cxx`, `.cc`, `.hpp`, `.hxx`, `.h` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/cpp.upts.json) |
| C | [`c_parser.go`](c_parser.go) | `.c`, `.h` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/c.upts.json) |
| CUDA | [`cuda_parser.go`](cuda_parser.go) | `.cu`, `.cuh` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/cuda.upts.json) |
| Protocol Buffers | [`protobuf_parser.go`](protobuf_parser.go) | `.proto` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/protobuf.upts.json) |
| GraphQL | [`graphql_parser.go`](graphql_parser.go) | `.graphql`, `.gql` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/graphql.upts.json) |
| OpenAPI/Swagger | [`openapi_parser.go`](openapi_parser.go) | `.yaml`, `.yml`, `.json` (with openapi/swagger marker) | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/openapi.upts.json) |
| SQL | [`sql_parser.go`](sql_parser.go) | `.sql` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/sql.upts.json) |
| Cypher (Neo4j) | [`cypher_parser.go`](cypher_parser.go) | `.cypher`, `.cql` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/cypher.upts.json) |
| Terraform/HCL | [`terraform_parser.go`](terraform_parser.go) | `.tf`, `.tfvars` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/terraform.upts.json) |
| YAML | [`yaml_parser.go`](yaml_parser.go) | `.yml`, `.yaml` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/yaml.upts.json) |
| TOML | [`toml_parser.go`](toml_parser.go) | `.toml` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/toml.upts.json) |
| JSON | [`json_parser.go`](json_parser.go) | `.json` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/json.upts.json) |
| INI | [`ini_parser.go`](ini_parser.go) | `.ini`, `.cfg`, `.conf` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/ini.upts.json) |
| Makefile | [`makefile_parser.go`](makefile_parser.go) | `.mk`, `Makefile` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/makefile.upts.json) |
| Dockerfile | [`dockerfile_parser.go`](dockerfile_parser.go) | `Dockerfile`, `*.dockerfile` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/dockerfile.upts.json) |
| Shell/Bash | [`shell_parser.go`](shell_parser.go) | `.sh`, `.bash`, `.zsh` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/shell.upts.json) |
| Markdown | [`markdown_parser.go`](markdown_parser.go) | `.md`, `.markdown` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/markdown.upts.json) |
| XML | [`xml_parser.go`](xml_parser.go) | `.xml`, `.xsd`, `.xsl`, etc. | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/xml.upts.json) |
| Lua | [`lua_parser.go`](lua_parser.go) | `.lua` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/lua.upts.json) |
| Scraper Markdown | [`scraper_markdown_parser.go`](scraper_markdown_parser.go) | `.scraped.md` | [Yes](../../../docs/lang-parser/lang-parse-spec/upts/specs/scraper_markdown.upts.json) |

## UPTS Validation

Each parser's symbol extraction is validated against a [UPTS (Universal Parser Test Specification)](../../../docs/lang-parser/lang-parse-spec/upts/README.md) spec file. The Go-native test harness loads the spec, parses the associated fixture file through the parser, and asserts that all expected symbols are found with correct name, type, line number, and export status.

```bash
# Run all 27 UPTS-validated parsers
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v

# Run a single language
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/rust -v
```

**Spec files**: [`docs/lang-parser/lang-parse-spec/upts/specs/`](../../../docs/lang-parser/lang-parse-spec/upts/specs/) — `<language>.upts.json`
**Fixture files**: [`docs/lang-parser/lang-parse-spec/upts/fixtures/`](../../../docs/lang-parser/lang-parse-spec/upts/fixtures/) — `<language>_test_fixture.<ext>`
**Test harness**: [`upts_test.go`](upts_test.go)
**Type definitions**: [`upts_types.go`](upts_types.go)
**UPTS changelog**: [`docs/lang-parser/lang-parse-spec/upts/CHANGELOG.md`](../../../docs/lang-parser/lang-parse-spec/upts/CHANGELOG.md)

### UPTS Spec Structure

```json
{
  "upts_version": "1.0.0",
  "language": "go",
  "variants": [".go"],
  "config": {
    "line_tolerance": 2,
    "require_all_symbols": true,
    "allow_extra_symbols": true
  },
  "fixture": {
    "type": "file",
    "path": "../fixtures/go_test_fixture.go",
    "sha256": "..."
  },
  "expected": {
    "symbol_count": { "min": 15, "max": 20 },
    "symbols": [
      { "name": "MyFunc", "type": "function", "line": 42, "exported": true }
    ]
  }
}
```

Key fields per symbol:
- `name`: exact match required
- `type`: symbol type (`function`, `struct`, `class`, `constant`, `macro`, `kernel`, etc.)
- `line`: expected line number (±`line_tolerance`)
- `exported`: visibility (`true` = public)
- `optional`: if `true`, a miss does not cause failure (used for duplicate declarations)
- `pattern`: documents which extraction pattern this symbol tests (informational)

## Adding a New Language

### Step 1: Create the Parser File

Create a new file `<language>_parser.go` in this directory.

### Step 2: Implement the LanguageParser Interface

```go
package languages

func init() {
    Register(&MyLangParser{})
}

type MyLangParser struct{}

func (p *MyLangParser) Name() string {
    return "mylang"
}

func (p *MyLangParser) Extensions() []string {
    return []string{".ml", ".myl"}
}

func (p *MyLangParser) CanParse(path string) bool {
    return strings.HasSuffix(strings.ToLower(path), ".ml") ||
           strings.HasSuffix(strings.ToLower(path), ".myl")
}

func (p *MyLangParser) IsTestFile(path string) bool {
    // Return true if path is a test file
    return strings.Contains(path, "/test/") ||
           strings.HasSuffix(path, "_test.ml")
}

func (p *MyLangParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
    // Parse the file and return code elements
    var elements []CodeElement

    content, err := ReadFileContent(path)
    if err != nil {
        return nil, err
    }

    relPath, _ := filepath.Rel(root, path)
    fileName := filepath.Base(path)

    // Extract structures using regex or AST
    // ...

    // Build content for embedding
    var contentBuilder strings.Builder
    contentBuilder.WriteString(fmt.Sprintf("MyLang file: %s\n", fileName))
    // Add summary information
    contentBuilder.WriteString("\n--- Code ---\n")
    contentBuilder.WriteString(TruncateContent(content, 4000))

    // Detect cross-cutting concerns
    concerns := DetectConcerns(relPath, content)
    tags := []string{"mylang", "module"}

    // Extract symbols if requested
    var symbols []Symbol
    if extractSymbols {
        symbols = p.extractSymbols(content)
    }

    // Create the main file element
    elements = append(elements, CodeElement{
        Name:     fileName,
        Kind:     "module",
        Path:     "/" + relPath,
        Content:  contentBuilder.String(),
        Package:  "...",
        FilePath: relPath,
        Tags:     tags,
        Concerns: concerns,
        Symbols:  symbols,
    })

    return elements, nil
}

func (p *MyLangParser) extractSymbols(content string) []Symbol {
    // Extract constants, functions, classes, etc.
    // ...
    return nil
}
```

### Step 3: Key Components to Implement

#### Name() and Extensions()
Return the language name and supported file extensions.

#### CanParse(path)
Return `true` if this parser should handle the given file path. Usually checks file extension.

#### IsTestFile(path)
Return `true` if the file is a test file. Used to filter test files when `--include-tests=false`.

#### ParseFile(root, path, extractSymbols)
The main parsing function. Should:

1. **Read file content** using `ReadFileContent(path)`
2. **Extract code structures** (classes, functions, etc.) using regex patterns
3. **Build embedding content** - Summary info + truncated code
4. **Detect concerns** using `DetectConcerns(relPath, content)`
5. **Extract symbols** if requested (constants, function signatures)
6. **Return CodeElement slice** for each major structure

#### extractSymbols(content)
Optional but recommended. Extract:
- Constants with their values
- Function signatures
- Class/struct definitions
- Enum values

### Step 4: Use Common Utilities

The `common.go` file provides shared utilities:

```go
// Read file content
content, err := ReadFileContent(path)

// Truncate long content (with truncation detection for diagnostics)
truncated, wasTruncated := TruncateContentWithInfo(content, 4000)

// Emit diagnostics when content is truncated
var diagnostics []Diagnostic
if wasTruncated {
    diagnostics = append(diagnostics, NewDiagnosticWithContext(
        "info", "TRUNCATED",
        fmt.Sprintf("Content truncated from %d to 4000 chars", len(content)),
        "mylang",
        map[string]string{"original_size": fmt.Sprintf("%d", len(content))},
    ))
}

// Detect cross-cutting concerns (auth, validation, logging, etc.)
concerns := DetectConcerns(relPath, content)

// Check if file is a config file
isConfig := IsConfigFile(path)

// Clean up values (remove quotes, trailing comments)
clean := CleanValue(rawValue)

// Regex helper - returns capture group 1 from all matches
names := FindAllMatches(content, `class\s+(\w+)`)
```

### Step 5: Create a UPTS Spec

Create a fixture file and spec to validate your parser's symbol extraction:

1. Write a representative fixture file at `docs/lang-parser/lang-parse-spec/upts/fixtures/<language>_test_fixture.<ext>`
2. Create the spec at `docs/lang-parser/lang-parse-spec/upts/specs/<language>.upts.json`
3. List all expected symbols with name, type, line, and exported status
4. Run validation: `go test ./cmd/ingest-codebase/languages/ -run TestUPTS/<language> -v`

See existing specs for examples of the full schema.

### Step 6: Rebuild

After adding your parser:

```bash
go build ./cmd/ingest-codebase/
```

The parser auto-registers via `init()`, so no other changes needed.

## Architecture

### Compile-Time Registration

Unlike plugin-based systems, parsers are compiled into the binary. This avoids:
- Go plugin version compatibility issues
- Runtime loading errors
- Platform limitations (plugins don't work on Windows)

### Interface-Based Design

All parsers implement `LanguageParser`:

```go
type LanguageParser interface {
    Name() string
    Extensions() []string
    CanParse(path string) bool
    ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error)
    IsTestFile(path string) bool
}
```

### Registry

Parsers register themselves via `Register()` called from `init()`:

```go
func init() {
    Register(&GoParser{})
}
```

The registry provides:
- `GetParser(name)` - Get parser by language name
- `GetParserForFile(path)` - Find parser for a file path
- `AllParsers()` - List all registered parsers
- `SupportedExtensions()` - List all supported extensions

## Data Structures

### CodeElement

Represents a parsed code unit (file, class, function, etc.):

```go
type CodeElement struct {
    // v1 fields (original)
    Name     string    // Element name (class name, function name, etc.)
    Kind     string    // Code construct: "class", "function", "struct", "module", "kernel", etc.
    Path     string    // Virtual path for linking (e.g., "/src/main.go#Handler")
    Content  string    // Text content for embedding generation
    Summary  string    // Brief summary (from docstrings)
    Package  string    // Package/module name
    FilePath string    // Relative file path
    Tags     []string  // Labels: language, kind, concerns
    Concerns []string  // Cross-cutting concerns detected
    Symbols  []Symbol  // Extracted code symbols

    // v2 fields (evidence and stability)
    ElementKind string // Ingestion unit type: "file", "symbol", "section", "keypath_fact", "unit", "snippet", "migration", "kernel", "other"
    StartLine   int    // First line of element in source file (1-indexed, 0 = not set)
    EndLine     int    // Last line of element in source file
    StableID    string // Deterministic ID for evidence tracking
    Signature   string // Human-readable signature
}
```

### Kind vs ElementKind

| Kind (code construct) | ElementKind (ingestion unit) | Notes |
|-----------------------|------------------------------|-------|
| function | symbol | Standard code symbol |
| class | symbol | Standard code symbol |
| struct | symbol | Standard code symbol |
| kernel | kernel | CUDA GPU kernel |
| module | unit | Represents a compilation unit |
| config | keypath_fact | Config file key-value |
| doc | section | Documentation section |

### Symbol

Represents an extracted code symbol (constant, function signature, etc.):

```go
type Symbol struct {
    Name           string // Symbol name
    Type           string // "constant", "function", "class", "variable", "struct", "enum", "method", "macro", "kernel", "type", "label", etc.
    Value          string // Value for constants
    RawValue       string // Original value string
    LineNumber     int    // Line number in source
    EndLine        int    // End line (for multi-line)
    Exported       bool   // Whether publicly visible
    DocComment     string // Documentation comment
    Signature      string // Function/method signature
    Parent         string // Parent class/struct
    TypeAnnotation string // Type annotation
    Language       string // Source language
}
```

### CUDA-Specific Symbol Types

The CUDA parser extracts these symbol types:
- `kernel` — CUDA `__global__` kernel functions (GPU entry points)
- `function` — `__device__` functions, `__host__ __device__` functions, and plain host functions
- `struct` — struct declarations (with `__align__` support)
- `variable` — `__shared__` memory declarations
- `macro` — `#define` preprocessor macros
- `constant` — `const`/`constexpr` values

Example:
```go
Symbol{
    Name: "matmul_kernel",
    Type: "kernel",
    Signature: "__global__ void matmul_kernel(...)",
    Language: "cuda",
}
```

## Diagnostics

Parsers can emit structured diagnostics via the `Diagnostic` struct (defined in [`interface.go`](interface.go)):

```go
type Diagnostic struct {
    Severity string            `json:"severity"`           // "info", "warning", "error"
    Code     string            `json:"code"`               // stable machine-readable code
    Message  string            `json:"message"`            // human-readable description
    Parser   string            `json:"parser,omitempty"`   // parser that emitted this
    Context  map[string]string `json:"context,omitempty"`  // additional key-value context
}
```

Attach diagnostics to `CodeElement.Diagnostics`. The ingestion pipeline aggregates them via `DiagnosticSummary` and logs a summary at the end of each run.

**Standard diagnostic codes:**

| Code | Severity | Emitted When |
|------|----------|-------------|
| `TRUNCATED` | info | `TruncateContentWithInfo()` truncates content |
| `LARGE_FILE` | warning | File exceeds size threshold in main.go |
| `PARTIAL_PARSE` | warning | Parser skips constructs it can't handle |
| `BINARY_DETECTED` | info | Non-text content detected |

All new parsers (C#, Kotlin, Terraform, Makefile) emit `TRUNCATED` diagnostics. Existing parsers can adopt diagnostics incrementally.

## Best Practices

1. **Extract meaningful structures** - Classes, functions, interfaces, not individual lines
2. **Include code in embedding** - Raw code improves semantic search quality
3. **Detect concerns** - Use `DetectConcerns()` for cross-cutting concepts
4. **Extract symbols** - Constants and function signatures help evidence-based retrieval
5. **Handle test files** - Implement `IsTestFile()` correctly
6. **Truncate long content** - Use `TruncateContent(content, 4000)` to prevent oversized embeddings
