# Universal Parser Test Specification (UPTS)

A language-agnostic test specification format for validating MDEMG language parsers.

## Problem

Your parser tests had inconsistent formats:

| Field | Go | Python | TypeScript |
|-------|-----|--------|------------|
| Line number | `line` | `line_number` | `line` |
| Signature | ❌ | ✅ | ❌ |
| Value | ❌ | ✅ | ❌ |
| Doc comment | ❌ | ✅ | ❌ |

## Solution

UPTS provides:

1. **Single JSON Schema** - canonical definition for all languages
2. **Normalized specs** - consistent field names across all languages
3. **Language-agnostic runner** - validates any parser implementation
4. **Format converter** - migrate existing `*_expected.json` files

> For the full UxTS methodology guide covering architecture, spec writing, CI integration, and all 11 frameworks, see [docs/guides/UXTS_DEVELOPER_GUIDE.md](../../../../docs/guides/UXTS_DEVELOPER_GUIDE.md).

---

## Current Status

**28 UPTS-validated parsers** — all pass via `go test`:

| Language | Spec | Fixture | Parser |
|----------|------|---------|--------|
| Go | [`go.upts.json`](specs/go.upts.json) | [`go_test_fixture.go`](fixtures/go_test_fixture.go) | [`go_parser.go`](../../../../cmd/ingest-codebase/languages/go_parser.go) |
| Rust | [`rust.upts.json`](specs/rust.upts.json) | [`rust_test_fixture.rs`](fixtures/rust_test_fixture.rs) | [`rust_parser.go`](../../../../cmd/ingest-codebase/languages/rust_parser.go) |
| Python | [`python.upts.json`](specs/python.upts.json) | [`python_test_fixture.py`](fixtures/python_test_fixture.py) | [`python_parser.go`](../../../../cmd/ingest-codebase/languages/python_parser.go) |
| TypeScript | [`typescript.upts.json`](specs/typescript.upts.json) | [`typescript_test_fixture.ts`](fixtures/typescript_test_fixture.ts) | [`typescript_parser.go`](../../../../cmd/ingest-codebase/languages/typescript_parser.go) |
| Java | [`java.upts.json`](specs/java.upts.json) | [`java_test_fixture.java`](fixtures/java_test_fixture.java) | [`java_parser.go`](../../../../cmd/ingest-codebase/languages/java_parser.go) |
| C# | [`csharp.upts.json`](specs/csharp.upts.json) | [`csharp_test_fixture.cs`](fixtures/csharp_test_fixture.cs) | [`csharp_parser.go`](../../../../cmd/ingest-codebase/languages/csharp_parser.go) |
| PHP | [`php.upts.json`](specs/php.upts.json) | [`php_test_fixture.php`](fixtures/php_test_fixture.php) | [`php_parser.go`](../../../../internal/languages/php_parser.go) |
| Kotlin | [`kotlin.upts.json`](specs/kotlin.upts.json) | [`kotlin_test_fixture.kt`](fixtures/kotlin_test_fixture.kt) | [`kotlin_parser.go`](../../../../cmd/ingest-codebase/languages/kotlin_parser.go) |
| C++ | [`cpp.upts.json`](specs/cpp.upts.json) | [`cpp_test_fixture.cpp`](fixtures/cpp_test_fixture.cpp) | [`cpp_parser.go`](../../../../cmd/ingest-codebase/languages/cpp_parser.go) |
| C | [`c.upts.json`](specs/c.upts.json) | [`c_test_fixture.c`](fixtures/c_test_fixture.c) | [`c_parser.go`](../../../../cmd/ingest-codebase/languages/c_parser.go) |
| CUDA | [`cuda.upts.json`](specs/cuda.upts.json) | [`cuda_test_fixture.cu`](fixtures/cuda_test_fixture.cu) | [`cuda_parser.go`](../../../../cmd/ingest-codebase/languages/cuda_parser.go) |
| SQL | [`sql.upts.json`](specs/sql.upts.json) | [`sql_test_fixture.sql`](fixtures/sql_test_fixture.sql) | [`sql_parser.go`](../../../../cmd/ingest-codebase/languages/sql_parser.go) |
| Cypher | [`cypher.upts.json`](specs/cypher.upts.json) | [`cypher_test_fixture.cypher`](fixtures/cypher_test_fixture.cypher) | [`cypher_parser.go`](../../../../cmd/ingest-codebase/languages/cypher_parser.go) |
| Terraform | [`terraform.upts.json`](specs/terraform.upts.json) | [`terraform_test_fixture.tf`](fixtures/terraform_test_fixture.tf) | [`terraform_parser.go`](../../../../cmd/ingest-codebase/languages/terraform_parser.go) |
| YAML | [`yaml.upts.json`](specs/yaml.upts.json) | [`yaml_test_fixture.yaml`](fixtures/yaml_test_fixture.yaml) | [`yaml_parser.go`](../../../../cmd/ingest-codebase/languages/yaml_parser.go) |
| TOML | [`toml.upts.json`](specs/toml.upts.json) | [`toml_test_fixture.toml`](fixtures/toml_test_fixture.toml) | [`toml_parser.go`](../../../../cmd/ingest-codebase/languages/toml_parser.go) |
| JSON | [`json.upts.json`](specs/json.upts.json) | [`json_test_fixture.json`](fixtures/json_test_fixture.json) | [`json_parser.go`](../../../../cmd/ingest-codebase/languages/json_parser.go) |
| INI | [`ini.upts.json`](specs/ini.upts.json) | [`ini_test_fixture.ini`](fixtures/ini_test_fixture.ini) | [`ini_parser.go`](../../../../cmd/ingest-codebase/languages/ini_parser.go) |
| Makefile | [`makefile.upts.json`](specs/makefile.upts.json) | [`makefile_test_fixture.mk`](fixtures/makefile_test_fixture.mk) | [`makefile_parser.go`](../../../../cmd/ingest-codebase/languages/makefile_parser.go) |
| Dockerfile | [`dockerfile.upts.json`](specs/dockerfile.upts.json) | [`dockerfile_test_fixture.Dockerfile`](fixtures/dockerfile_test_fixture.Dockerfile) | [`dockerfile_parser.go`](../../../../cmd/ingest-codebase/languages/dockerfile_parser.go) |
| Shell | [`shell.upts.json`](specs/shell.upts.json) | [`shell_test_fixture.sh`](fixtures/shell_test_fixture.sh) | [`shell_parser.go`](../../../../cmd/ingest-codebase/languages/shell_parser.go) |
| Protocol Buffers | [`protobuf.upts.json`](specs/protobuf.upts.json) | [`protobuf_test_fixture.proto`](fixtures/protobuf_test_fixture.proto) | [`protobuf_parser.go`](../../../../cmd/ingest-codebase/languages/protobuf_parser.go) |
| GraphQL | [`graphql.upts.json`](specs/graphql.upts.json) | [`graphql_test_fixture.graphql`](fixtures/graphql_test_fixture.graphql) | [`graphql_parser.go`](../../../../cmd/ingest-codebase/languages/graphql_parser.go) |
| OpenAPI | [`openapi.upts.json`](specs/openapi.upts.json) | [`openapi_test_fixture.yaml`](fixtures/openapi_test_fixture.yaml) | [`openapi_parser.go`](../../../../cmd/ingest-codebase/languages/openapi_parser.go) |
| Markdown | [`markdown.upts.json`](specs/markdown.upts.json) | [`markdown_test_fixture.md`](fixtures/markdown_test_fixture.md) | [`markdown_parser.go`](../../../../cmd/ingest-codebase/languages/markdown_parser.go) |
| XML | [`xml.upts.json`](specs/xml.upts.json) | [`xml_test_fixture.xml`](fixtures/xml_test_fixture.xml) | [`xml_parser.go`](../../../../cmd/ingest-codebase/languages/xml_parser.go) |
| Lua | [`lua.upts.json`](specs/lua.upts.json) | [`lua_test_fixture.lua`](fixtures/lua_test_fixture.lua) | [`lua_parser.go`](../../../../cmd/ingest-codebase/languages/lua_parser.go) |
| Scraper Markdown | [`scraper_markdown.upts.json`](specs/scraper_markdown.upts.json) | [`scraper_markdown_test_fixture.md`](fixtures/scraper_markdown_test_fixture.md) | [`scraper_markdown_parser.go`](../../../../cmd/ingest-codebase/languages/scraper_markdown_parser.go) |

---

## Directory Structure

```
upts/
├── schema/
│   └── upts.schema.json             # JSON Schema (canonical definition)
│
├── specs/                            # 28 UPTS spec files
│   ├── go.upts.json
│   ├── rust.upts.json
│   ├── python.upts.json
│   ├── typescript.upts.json
│   ├── java.upts.json
│   ├── csharp.upts.json
│   ├── php.upts.json
│   ├── kotlin.upts.json
│   ├── cpp.upts.json
│   ├── c.upts.json
│   ├── cuda.upts.json
│   ├── sql.upts.json
│   ├── cypher.upts.json
│   ├── terraform.upts.json
│   ├── yaml.upts.json
│   ├── toml.upts.json
│   ├── json.upts.json
│   ├── ini.upts.json
│   ├── makefile.upts.json
│   ├── dockerfile.upts.json
│   ├── shell.upts.json
│   ├── protobuf.upts.json
│   ├── graphql.upts.json
│   ├── openapi.upts.json
│   ├── markdown.upts.json
│   ├── xml.upts.json
│   ├── lua.upts.json
│   └── scraper_markdown.upts.json
│
├── fixtures/                         # 28 test fixture files
│   ├── go_test_fixture.go
│   ├── rust_test_fixture.rs
│   ├── python_test_fixture.py
│   ├── ... (one per language)
│   └── shell_test_fixture.sh
│
├── runners/
│   └── upts_runner.py               # Python test runner (cross-validation)
│
├── CHANGELOG.md
├── PARSER_ROADMAP.md
├── PARSER_GAPS.md
└── README.md                        # This file
```

### Go-Native Test Harness

The primary validation method is the Go-native test harness in the parser directory:

- [`upts_test.go`](../../../../cmd/ingest-codebase/languages/upts_test.go) — Test harness
- [`upts_types.go`](../../../../cmd/ingest-codebase/languages/upts_types.go) — UPTS type definitions

---

## Quick Start

### 1. Run All UPTS Tests (Go-Native — Recommended)

```bash
# All 28 UPTS-validated parsers
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v

# Single language
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/kotlin -v
```

### 2. Run via Python Runner (Cross-Validation)

```bash
# Single spec
python runners/upts_runner.py validate \
    --spec specs/typescript.upts.json \
    --parser ./your-parser

# All specs
python runners/upts_runner.py validate-all \
    --spec-dir specs/ \
    --parser ./your-parser \
    --report report.json
```

### 3. Your Parser Must Output

```json
{
  "symbols": [
    {
      "name": "UserService",
      "type": "class",
      "line": 26,
      "exported": true,
      "parent": "",
      "signature": "",
      "value": "",
      "doc_comment": ""
    }
  ]
}
```

---

## UPTS Spec Format

```json
{
  "upts_version": "1.0.0",
  "language": "python",
  "variants": [".py", ".pyi"],
  
  "metadata": {
    "author": "reh3376",
    "created": "2026-01-28",
    "description": "Python parser - all canonical patterns",
    "parser_status": "enhanced",
    "tags": ["canonical", "protocol", "dataclass"]
  },
  
  "config": {
    "line_tolerance": 2,
    "require_all_symbols": true,
    "allow_extra_symbols": true,
    "validate_signature": true,
    "validate_value": true,
    "validate_parent": true,
    "validate_evidence": false
  },
  
  "fixture": {
    "type": "file",
    "path": "fixtures/python_test_fixture.py"
  },
  
  "expected": {
    "symbol_count": {"min": 35, "max": 40},
    
    "symbols": [
      {
        "name": "MAX_RETRIES",
        "type": "constant",
        "line": 12,
        "exported": true,
        "value": "3",
        "pattern": "P1_CONSTANT",
        "tags": ["constant", "int"]
      },
      {
        "name": "UserService",
        "type": "class",
        "line": 95,
        "exported": true,
        "pattern": "P3_CLASS_STRUCT",
        "tags": ["class", "service"]
      },
      {
        "name": "find_by_id",
        "type": "method",
        "line": 102,
        "parent": "UserService",
        "signature_contains": ["user_id", "Optional"],
        "pattern": "P6_METHOD"
      }
    ],
    
    "excluded": [
      {"name": "_repository", "reason": "private attribute"},
      {"name_pattern": "^__", "reason": "dunder methods"}
    ],
    
    "relationships": [
      {"source": "UserService", "relation": "DEFINES_METHOD", "target": "find_by_id"}
    ]
  },
  
  "patterns_covered": ["P1_CONSTANT", "P2_FUNCTION", "P3_CLASS_STRUCT", "P6_METHOD"]
}
```

---

## Symbol Fields (Normalized)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | ✅ | Symbol name |
| `type` | string | ✅ | Symbol type (see below) |
| `line` | int | ✅ | Line number (1-indexed) |
| `exported` | bool | ❌ | Public visibility |
| `parent` | string | ❌ | Parent symbol (for methods) |
| `signature` | string | ❌ | Full signature |
| `signature_contains` | string[] | ❌ | Partial signature matches |
| `value` | string | ❌ | Constant value |
| `doc_comment` | string | ❌ | Documentation/decorators |
| `decorators` | string[] | ❌ | Decorator names |
| `pattern` | string | ❌ | Canonical pattern reference |
| `tags` | string[] | ❌ | Custom tags for filtering |
| `optional` | bool | ❌ | Missing is not a failure |

### Symbol Types

```
constant, function, class, method, interface, enum, type, struct, 
trait, macro, kernel, variable, field
```

---

## Canonical Patterns

Every language parser should handle these 7 patterns:

| Pattern | Code | Example |
|---------|------|---------|
| `P1_CONSTANT` | Named constant values | `const MAX = 3` |
| `P2_FUNCTION` | Standalone functions | `func calc()` |
| `P3_CLASS_STRUCT` | Classes/structs | `class User` |
| `P4_INTERFACE_TRAIT` | Interfaces/traits/protocols | `interface Repo` |
| `P5_ENUM` | Enumerations | `enum Status` |
| `P6_METHOD` | Class/struct methods | `def find(self)` |
| `P7_TYPE_ALIAS` | Type aliases | `type ID = str` |

---

## Validation Rules

### Line Tolerance

```json
"config": {"line_tolerance": 2}
```

Allows ±2 lines from expected. Useful when comments shift line numbers.

### Type Compatibility

These types are considered equivalent:

- `class` ↔ `struct`
- `interface` ↔ `trait` ↔ `protocol`

### Parent Matching

For methods, `parent` must match exactly:

```json
{"name": "find_by_id", "type": "method", "parent": "UserService"}
```

### Signature Validation

Use `signature_contains` for partial matches:

```json
{"name": "fetch_user", "signature_contains": ["async", "user_id", "Optional"]}
```

---

## Evidence Validation

When `validate_evidence` is `true` in the spec config, the Go-native test harness runs structural consistency checks on parser output after symbol matching:

1. **Symbol.LineEnd consistency**: `LineEnd >= Line` when both are set
2. **CodeElement range consistency**: `StartLine <= EndLine` when both are set
3. **Symbol containment**: Symbol `Line` falls within its CodeElement's `[StartLine, EndLine]` range (warning only)
4. **UPTSSymbol.LineEnd matching**: When the spec has `line_end` for an expected symbol and the matched actual symbol has `LineEnd`, they must match within `line_tolerance`

Currently enabled for: Go, Rust. Other parsers can opt in by setting `"validate_evidence": true` in their spec's config section.

---

## Test Report Format

```json
{
  "timestamp": "2026-01-29T12:00:00Z",
  "upts_version": "1.0.0",
  "summary": {
    "total_specs": 3,
    "passed": 2,
    "failed": 1,
    "errors": 0,
    "pass_rate": 66.67
  },
  "results": [
    {
      "spec_path": "specs/python.upts.json",
      "language": "python",
      "status": "pass",
      "total_expected": 35,
      "matched": 35,
      "failed": 0,
      "pass_rate": 100.0,
      "duration_ms": 45.2,
      "failures": []
    }
  ]
}
```

---

## Integration with MDEMG

### Parser Output Contract

Your Go parsers (`*_parser.go`) should output:

```go
type SymbolOutput struct {
    Symbols []Symbol `json:"symbols"`
}

type Symbol struct {
    Name       string `json:"name"`
    Type       string `json:"type"`
    Line       int    `json:"line"`
    Exported   bool   `json:"exported"`
    Parent     string `json:"parent,omitempty"`
    Signature  string `json:"signature,omitempty"`
    Value      string `json:"value,omitempty"`
    DocComment string `json:"doc_comment,omitempty"`
}
```

### CI Integration

```yaml
# .github/workflows/parser-tests.yml
jobs:
  test-parsers:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Build parser
        run: go build -o parser ./cmd/ingest-codebase
      
      - name: Run UPTS tests
        run: |
          python upts/runners/upts_runner.py validate-all \
            --spec-dir upts/specs/ \
            --parser ./parser \
            --report parser-report.json
      
      - name: Upload report
        uses: actions/upload-artifact@v4
        with:
          name: parser-report
          path: parser-report.json
```

---

## Adding a New Language

### Step 1: Create the Parser

Create `cmd/ingest-codebase/languages/<language>_parser.go` implementing the `LanguageParser` interface. See the [languages README](../../../../cmd/ingest-codebase/languages/README.md) for the interface and best practices.

### Step 2: Create a Test Fixture

Create a representative source file that exercises all symbol types the parser should extract:

```
fixtures/<language>_test_fixture.<ext>
```

Include:

- Constants/variables at various scopes
- Functions (exported and private)
- Classes/structs with methods
- Interfaces/traits
- Enums with values
- Type aliases
- Nested structures
- Annotations/decorators

### Step 3: Create the UPTS Spec

Create `specs/<language>.upts.json`:

```json
{
  "upts_version": "1.0.0",
  "language": "<language>",
  "variants": [".<ext>"],
  "metadata": {
    "author": "your-name",
    "created": "2026-02-05",
    "description": "<Language> parser - all canonical patterns"
  },
  "config": {
    "line_tolerance": 2,
    "require_all_symbols": true,
    "allow_extra_symbols": true,
    "validate_parent": true,
    "validate_evidence": false
  },
  "fixture": {
    "type": "file",
    "path": "../fixtures/<language>_test_fixture.<ext>"
  },
  "expected": {
    "symbol_count": {"min": 15, "max": 25},
    "symbols": [...]
  },
  "patterns_covered": ["P1_CONSTANT", "P2_FUNCTION", ...]
}
```

### Step 4: Run Validation

```bash
# Build the parser
go build ./cmd/ingest-codebase/...

# Run the UPTS test
go test ./cmd/ingest-codebase/languages/ -run TestUPTS/<language> -v
```

Iterate until all expected symbols pass.

### Step 5: Update Documentation

- Add the language to the table in [this README](#current-status)
- Add the language to the table in the [languages README](../../../../cmd/ingest-codebase/languages/README.md)
- Update `CHANGELOG.md`

---

## Key Differences from Old Format

| Old | UPTS |
|-----|------|
| `line_number` (Python) vs `line` (Go) | Always `line` |
| `fixture_file` vs `fixture` | `fixture.path` |
| No schema validation | JSON Schema enforced |
| No pattern tagging | `pattern` field links to canonical patterns |
| No relationship testing | `relationships` array for graph edges |

---

## License

MIT
