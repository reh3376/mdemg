# Parser Gaps - UPTS Compliance Tracker

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Last Updated:** 2026-02-05
**Status:** ALL GAPS RESOLVED

All 20 UPTS-validated language parsers now pass at 100% with no workarounds.

---

## Current Status

| Language | Status | Notes |
|----------|--------|-------|
| Go | PASS | Evidence validation enabled |
| Rust | PASS | Evidence validation enabled |
| Python | PASS | Protocol/dataclass support |
| TypeScript | PASS | Includes JS/JSX/TSX |
| Java | PASS | Brace-depth scope tracking |
| C# | PASS | Records, properties, attributes |
| Kotlin | PASS | Data/sealed classes, objects |
| C++ | PASS | Templates, namespaces |
| C | PASS | Typedef struct, enums |
| CUDA | PASS | Kernels, device functions, shared memory |
| SQL | PASS | Tables, views, functions, triggers, enums, sequences |
| Cypher | PASS | Labels, constraints, indexes, relationships |
| Terraform | PASS | Resources, variables, outputs, locals |
| YAML | PASS | Hierarchical key paths |
| TOML | PASS | Section and key extraction |
| JSON | PASS | Nested object/array handling |
| INI | PASS | Section and key extraction |
| Makefile | PASS | Targets, variables, .PHONY |
| Dockerfile | PASS | Stages, ARG, ENV, EXPOSE, VOLUME |
| Shell | PASS | Functions, variables, exports |

**Total:** 20/20 UPTS-validated parsers passing (100%)

---

## Resolved Gaps

### Python Parser (Fixed 2026-01-29)

| Gap | Resolution |
|-----|------------|
| Protocol detection | Check for `Protocol` in superclasses → `interface` |
| Enum detection | Check for `Enum/IntEnum/StrEnum` in superclasses → `enum` |
| Method extraction | Track class context, methods have `parent` |
| Type alias detection | CamelCase = type expression → `type` |

**Changes in:** `internal/symbols/parser.go`

- `extractPythonClass()` - checks superclass for Enum/Protocol
- `extractPythonFunction()` - accepts currentClass parameter for method detection
- `extractPythonAssignment()` - `isPythonTypeAlias()` helper function
- `extractPythonSymbols()` - tracks class context during AST walk

### TypeScript Parser (Fixed 2026-01-29)

| Gap | Resolution |
|-----|------------|
| Arrow function detection | Check if const value is `arrow_function` → `function` |
| Class method extraction | Walk `class_body` for `method_definition` nodes |
| Abstract class detection | Handle `abstract_class_declaration` node type |

**Changes in:** `internal/symbols/parser.go`

- `extractTSVariableDeclaration()` - checks valueNode type for arrow functions
- `extractTSClassMethods()` - new function to extract methods from class body
- `extractTSMethod()` - new function to extract individual method
- AST walk now handles both `class_declaration` and `abstract_class_declaration`

---

## TYPE_COMPAT Configuration

With all parser gaps resolved, TYPE_COMPAT now contains only legitimate semantic equivalences:

```python
TYPE_COMPAT = {
    "class": {"class", "struct"},
    "struct": {"class", "struct"},
    "interface": {"interface", "trait", "protocol"},
    "trait": {"interface", "trait", "protocol"},
    "protocol": {"interface", "trait", "protocol"},
}
```

No workarounds. No hacks.

---

## Verification

```bash
go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v
```

Expected output:

```
--- PASS: TestUPTS (0.02s)
    --- PASS: TestUPTS/c (0.00s)
    --- PASS: TestUPTS/cpp (0.00s)
    --- PASS: TestUPTS/csharp (0.00s)
    --- PASS: TestUPTS/cuda (0.00s)
    --- PASS: TestUPTS/cypher (0.00s)
    --- PASS: TestUPTS/dockerfile (0.00s)
    --- PASS: TestUPTS/go (0.00s)
    --- PASS: TestUPTS/ini (0.00s)
    --- PASS: TestUPTS/java (0.00s)
    --- PASS: TestUPTS/json (0.00s)
    --- PASS: TestUPTS/kotlin (0.00s)
    --- PASS: TestUPTS/makefile (0.00s)
    --- PASS: TestUPTS/python (0.00s)
    --- PASS: TestUPTS/rust (0.00s)
    --- PASS: TestUPTS/shell (0.00s)
    --- PASS: TestUPTS/sql (0.00s)
    --- PASS: TestUPTS/terraform (0.00s)
    --- PASS: TestUPTS/toml (0.00s)
    --- PASS: TestUPTS/typescript (0.00s)
    --- PASS: TestUPTS/yaml (0.00s)
PASS
```
