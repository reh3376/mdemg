# MDEMG Language Parser Specification

**Version:** 1.0
**Last Updated:** 2026-02-02
**Status:** Active

---

## Overview

MDEMG's ingestion system uses language-specific parsers to extract symbols from source code. This document consolidates the authoritative specification for parser development, testing, and integration.

---

## Supported Languages

**All 28 parsers are shipped and UPTS-validated** (implementations in
`internal/languages/`, one spec + fixture per language in
`lang-parse-spec/upts/`): c, cpp, csharp, cuda, cypher, dockerfile, go,
graphql, ini, java, json, kotlin, lua, makefile, markdown, openapi, php,
protobuf, python, rust, scraper_markdown, shell, sql, terraform, toml,
typescript, xml, yaml.

The original launch trio (Go, Python, TypeScript) plus every "Phase 1–3
planned" language below shipped by 2026-02 (PHP added 2026-04). The
roadmap section is retained as design history of the build order.

---

## Universal Parser Test Specification (UPTS)

UPTS provides a language-agnostic test specification format for validating MDEMG language parsers.

### Directory Structure

```
lang-parser/
├── lang-parse-spec/
│   └── upts/
│       ├── schema/upts.schema.json     # JSON Schema (canonical definition)
│       ├── specs/                       # Language-specific test specs
│       │   ├── typescript.upts.json
│       │   ├── go.upts.json
│       │   └── python.upts.json
│       ├── fixtures/                    # Test fixture files
│       └── runners/upts_runner.py       # Python test runner
```

### UPTS Spec Format

```json
{
  "upts_version": "1.0.0",
  "language": "python",
  "variants": [".py", ".pyi"],

  "metadata": {
    "author": "mdemg",
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
    "validate_parent": true
  },

  "fixture": {
    "type": "file",
    "path": "fixtures/python_test_fixture.py"
  },

  "expected": {
    "symbol_count": {"min": 35, "max": 40},
    "symbols": [...]
  }
}
```

### Symbol Fields (Normalized)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Symbol name |
| `type` | string | Yes | Symbol type (see below) |
| `line` | int | Yes | Line number (1-indexed) |
| `exported` | bool | No | Public visibility |
| `parent` | string | No | Parent symbol (for methods) |
| `signature` | string | No | Full signature |
| `value` | string | No | Constant value |
| `doc_comment` | string | No | Documentation/decorators |
| `pattern` | string | No | Canonical pattern reference |

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

## Parser Output Contract

Parsers must output JSON matching this Go structure:

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

---

## Testing

### Running UPTS Tests

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

### Go Unit Tests

```bash
# Run the UPTS conformance harness over all 28 parsers
go test ./internal/languages/ -v -run TestUPTS

# Or via the Makefile target (runs the spec runner against all specs)
make test-parsers
```

### Test Report Format

```json
{
  "timestamp": "2026-01-29T12:00:00Z",
  "upts_version": "1.0.0",
  "summary": {
    "total_specs": 3,
    "passed": 2,
    "failed": 1,
    "pass_rate": 66.67
  }
}
```

---

## Parser Roadmap (historical — all phases shipped)

### Phase 1: Config Parsers (High Leverage, Low Complexity)

| Language | Extensions | Complexity | Priority |
|----------|------------|------------|----------|
| YAML | `.yml`, `.yaml` | Medium | Critical |
| TOML | `.toml` | Low | High |
| JSON/JSONC | `.json`, `.jsonc` | Low | High |
| INI/dotenv | `.env`, `.ini` | Very Low | Medium |
| Dockerfile | `Dockerfile*` | Low-Medium | Medium |
| Shell | `.sh`, `.bash` | Medium | Medium |

### Phase 2: Systems Languages

| Language | Extensions | Complexity |
|----------|------------|------------|
| Rust | `.rs` | High |
| Java | `.java` | Medium-High |
| C | `.c`, `.h` | Medium |
| C++ | `.cpp`, `.hpp` | Very High |
| CUDA | `.cu`, `.cuh` | High |

### Phase 3: Data & Documentation

| Language | Extensions | Complexity |
|----------|------------|------------|
| SQL | `.sql` | Medium |
| Cypher | `.cypher` | Low-Medium |
| Markdown | `.md`, `.mdx` | Low |
| XML | `.xml`, `.pom` | Medium |

---

## Adding a New Parser

1. Create test fixture in `fixtures/`
2. Create UPTS spec in `specs/`
3. Implement parser following output contract
4. Run UPTS validation
5. Add to CI pipeline

---

## Related Files

- `lang-parse-spec/upts/` - UPTS test specifications and fixtures
- `internal/languages/` - Parser implementations + UPTS harness (upts_test.go)
- `archive/` - Historical parser iterations

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-02 | Consolidated from multiple docs |
