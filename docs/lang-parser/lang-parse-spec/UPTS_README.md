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

---

## Directory Structure

```
lang-parse-spec/
├── UPTS_README.md             # This file
├── INCONSISTENCIES_REPORT.md  # Original analysis of parser inconsistencies
├── PARSER_GAPS.md             # Tracked parser limitations (fix targets)
│
└── upts/
    ├── schema/
    │   └── upts.schema.json       # JSON Schema (canonical definition)
    │
    ├── specs/
    │   ├── typescript.upts.json   # TypeScript test spec
    │   ├── go.upts.json           # Go test spec
    │   └── python.upts.json       # Python test spec
    │
    ├── fixtures/
    │   ├── typescript_test_fixture.ts
    │   ├── go_test_fixture.go
    │   └── python_test_fixture.py
    │
    └── runners/
        └── upts_runner.py         # Python test runner
```

## Related Documents

- **[PARSER_GAPS.md](./PARSER_GAPS.md)** - Known parser limitations with fix guidance
- **[INCONSISTENCIES_REPORT.md](./INCONSISTENCIES_REPORT.md)** - Original analysis that led to UPTS

---

## Quick Start

### 1. Run Tests Against Your Parser

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

### 2. Your Parser Must Output

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

### 3. Convert Existing Tests

```bash
python runners/upts_runner.py convert \
    --input old_expected.json \
    --language python \
    --fixture fixtures/python_test_fixture.py \
    --output specs/python.upts.json
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
    "validate_parent": true
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

1. **Create fixture** in `fixtures/`:

   ```
   fixtures/rust_test_fixture.rs
   ```

2. **Create spec** in `specs/`:

   ```json
   {
     "upts_version": "1.0.0",
     "language": "rust",
     "variants": [".rs"],
     "fixture": {"type": "file", "path": "fixtures/rust_test_fixture.rs"},
     "expected": {"symbols": [...]}
   }
   ```

3. **Run validation**:

   ```bash
   python runners/upts_runner.py validate --spec specs/rust.upts.json --parser ./parser
   ```

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
