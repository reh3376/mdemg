# MDEMG Parser Implementation Guide

**Last Updated:** 2026-01-29  
**Status:** Phase 1 Complete (Tree-sitter), Phase 2 In Progress (Config Parsers)

---

## Executive Summary

The MDEMG parser system supports **16 languages** across two parser architectures:

| Architecture | Languages | Status |
|--------------|-----------|--------|
| **Tree-sitter** | Go, Python, TypeScript, Rust, C, C++, CUDA, Java | 3/8 passing |
| **Config/Regex** | YAML, TOML, JSON, INI, Shell, Dockerfile, SQL, Cypher | 0/8 (needs integration) |

**Current Pass Rate:** 25% (3/16 languages)  
**Target:** 100% (16/16 languages)

---

## Current Status by Language

### ✅ Passing (Tree-sitter)

| Language | Symbols | Match Rate | Notes |
|----------|---------|------------|-------|
| Go | 17/17 | 100% | All patterns working |
| Python | 35/35 | 100% | Protocol/Enum/Method detection fixed |
| TypeScript | 13/13 | 100% | Arrow functions, class methods fixed |

### ❌ Tree-sitter Grammar Missing

| Language | Error | Fix Required |
|----------|-------|--------------|
| Rust | `no grammar loaded for language: rust` | Load rust grammar |
| C | Not tested | Load c grammar |
| C++ | Not tested | Load cpp grammar |
| CUDA | Not tested | Load cpp grammar + custom handling |
| Java | Not tested | Load java grammar |

### ❌ Needs Fallback Parser Integration

| Language | Existing Parser | Location |
|----------|-----------------|----------|
| YAML | `yaml_parser.go` | `cmd/ingest-codebase/languages/` |
| TOML | `toml_parser.go` | `cmd/ingest-codebase/languages/` |
| JSON | `json_parser.go` | `cmd/ingest-codebase/languages/` |
| INI/dotenv | `ini_parser.go` | `cmd/ingest-codebase/languages/` |
| Shell | `shell_parser.go` | `cmd/ingest-codebase/languages/` |
| Dockerfile | `dockerfile_parser.go` | `cmd/ingest-codebase/languages/` |
| SQL | `sql_parser.go` | `cmd/ingest-codebase/languages/` |
| Cypher | `cypher_parser.go` | `cmd/ingest-codebase/languages/` |

---

## Root Cause Analysis

### Problem: Two Parser Systems Not Integrated

```
cmd/extract-symbols/main.go
    └── internal/symbols/parser.go (tree-sitter ONLY)
        └── Supports: Go, Python, TypeScript, Rust*, C*, C++*, Java*
        └── * = grammar not loaded

cmd/ingest-codebase/main.go
    └── cmd/ingest-codebase/languages/*.go (regex-based)
        └── Supports: YAML, TOML, JSON, INI, Shell, Dockerfile, SQL, Cypher
```

**Result:** `extract-symbols` command fails for config languages because it only tries tree-sitter.

### Solution: Fallback Parser Pattern

```go
// In extract-symbols, after tree-sitter fails:
if strings.Contains(err.Error(), "no grammar loaded") {
    symbols, handled, fallbackErr := TryFallbackParser(filePath)
    if handled {
        return symbols, fallbackErr
    }
}
```

---

## Implementation Guide

### Step 1: Load Missing Tree-sitter Grammars

**File:** `internal/symbols/parser.go`

```go
import (
    sitter "github.com/smacker/go-tree-sitter"
    "github.com/smacker/go-tree-sitter/rust"
    "github.com/smacker/go-tree-sitter/c"
    "github.com/smacker/go-tree-sitter/cpp"
    "github.com/smacker/go-tree-sitter/java"
)

func loadGrammar(lang string) (*sitter.Language, error) {
    switch lang {
    case "go":
        return golang.GetLanguage(), nil
    case "python":
        return python.GetLanguage(), nil
    case "typescript", "tsx":
        return typescript.GetLanguage(), nil
    case "rust":
        return rust.GetLanguage(), nil  // ADD THIS
    case "c":
        return c.GetLanguage(), nil     // ADD THIS
    case "cpp", "cuda":
        return cpp.GetLanguage(), nil   // ADD THIS
    case "java":
        return java.GetLanguage(), nil  // ADD THIS
    default:
        return nil, fmt.Errorf("no grammar loaded for language: %s", lang)
    }
}
```

**Install dependencies:**

```bash
go get github.com/smacker/go-tree-sitter/rust
go get github.com/smacker/go-tree-sitter/c
go get github.com/smacker/go-tree-sitter/cpp
go get github.com/smacker/go-tree-sitter/java
```

---

### Step 2: Add Fallback Parser Integration

**File:** `cmd/extract-symbols/fallback_parsers.go`

Copy the provided `fallback_parsers.go` to this location. It contains regex-based parsers for:

- YAML (sections, anchors, GHA jobs, K8s kinds)
- TOML (tables, array-of-tables, key/values)
- JSON/JSONC (flattened paths, comment stripping)
- INI/dotenv (sections, key/values)
- Shell (functions, exports, readonly)
- Dockerfile (ARG, ENV, FROM stages, EXPOSE, CMD)
- SQL (tables, columns, views, functions, triggers)
- Cypher (labels, relationships, constraints, indexes)

**File:** `cmd/extract-symbols/main.go`

Add integration point:

```go
func extractSymbols(filePath string) (*SymbolOutput, error) {
    lang := detectLanguage(filePath)
    
    if lang != "" {
        // Try tree-sitter first
        symbols, err := parseWithTreeSitter(filePath, lang)
        if err == nil {
            return &SymbolOutput{Symbols: symbols}, nil
        }
        
        // Fallback on "no grammar loaded" error
        if strings.Contains(err.Error(), "no grammar loaded") {
            symbols, handled, fallbackErr := TryFallbackParser(filePath)
            if handled {
                if fallbackErr != nil {
                    return nil, fallbackErr
                }
                return &SymbolOutput{Symbols: symbols}, nil
            }
        }
        
        return nil, err
    }
    
    // No tree-sitter language detected, try fallback directly
    symbols, handled, err := TryFallbackParser(filePath)
    if handled {
        if err != nil {
            return nil, err
        }
        return &SymbolOutput{Symbols: symbols}, nil
    }
    
    return nil, fmt.Errorf("unsupported file type: %s", filePath)
}

func detectLanguage(filePath string) string {
    ext := strings.ToLower(filepath.Ext(filePath))
    
    switch ext {
    case ".go":
        return "go"
    case ".py":
        return "python"
    case ".ts", ".tsx":
        return "typescript"
    case ".js", ".jsx":
        return "javascript"
    case ".rs":
        return "rust"
    case ".c", ".h":
        return "c"
    case ".cpp", ".hpp", ".cc", ".cxx":
        return "cpp"
    case ".cu", ".cuh":
        return "cuda"
    case ".java":
        return "java"
    }
    
    // Return empty for config files - they'll use fallback
    return ""
}
```

---

### Step 3: Rebuild and Test

```bash
# Build
go build -o bin/extract-symbols ./cmd/extract-symbols

# Test single file
./bin/extract-symbols --json upts/fixtures/yaml_test_fixture.yaml

# Test all languages
make test-parsers

# Or run UPTS validation manually
cd upts && python runners/upts_runner.py validate-all \
    --spec-dir specs/ \
    --parser ../bin/extract-symbols
```

---

## Parser Output Contract

All parsers (tree-sitter and fallback) must output this JSON structure:

```json
{
  "symbols": [
    {
      "name": "string",           // Required: symbol name
      "type": "string",           // Required: constant|function|class|method|...
      "line": 42,                 // Required: 1-indexed line number
      "exported": true,           // Optional: public visibility
      "parent": "ClassName",      // Optional: containing class/struct
      "signature": "...",         // Optional: function signature
      "value": "...",             // Optional: constant value
      "doc_comment": "..."        // Optional: documentation
    }
  ]
}
```

**Valid types:**

- Code: `constant`, `function`, `class`, `method`, `interface`, `enum`, `type`, `struct`, `trait`, `macro`, `module`, `namespace`
- CUDA: `kernel`, `variable`
- SQL: `table`, `column`, `index`, `view`, `trigger`, `constraint`
- Cypher: `label`, `relationship_type`
- Config: `section`

---

## UPTS Specs and Fixtures

All 16 languages have UPTS specs and test fixtures ready:

### Tree-sitter Languages

| Language | Fixture | Spec | Symbols |
|----------|---------|------|---------|
| Go | `go_test_fixture.go` | `go.upts.json` | 17 |
| Python | `python_test_fixture.py` | `python.upts.json` | 35 |
| TypeScript | `typescript_test_fixture.ts` | `typescript.upts.json` | 13 |
| Rust | `rust_test_fixture.rs` | `rust.upts.json` | 27 |
| C | `c_test_fixture.c` | `c.upts.json` | 24 |
| C++ | `cpp_test_fixture.cpp` | `cpp.upts.json` | 30 |
| CUDA | `cuda_test_fixture.cu` | `cuda.upts.json` | 25 |
| Java | `java_test_fixture.java` | `java.upts.json` | 32 |

### Config Languages

| Language | Fixture | Spec | Symbols |
|----------|---------|------|---------|
| YAML | `yaml_test_fixture.yaml` | `yaml.upts.json` | 21 |
| TOML | `toml_test_fixture.toml` | `toml.upts.json` | 15 |
| JSON | `json_test_fixture.json` | `json.upts.json` | 14 |
| INI | `ini_test_fixture.env` | `ini.upts.json` | 15 |
| Shell | `shell_test_fixture.sh` | `shell.upts.json` | 14 |
| Dockerfile | `dockerfile_test_fixture.dockerfile` | `dockerfile.upts.json` | 16 |
| SQL | `sql_test_fixture.sql` | `sql.upts.json` | 17 |
| Cypher | `cypher_test_fixture.cypher` | `cypher.upts.json` | 14 |

---

## Canonical Patterns Coverage

Each language should support the 7 canonical patterns:

| Pattern | Description | Languages |
|---------|-------------|-----------|
| P1_CONSTANT | Named constant values | All |
| P2_FUNCTION | Standalone functions | All code languages |
| P3_CLASS_STRUCT | Classes/structs | All code languages |
| P4_INTERFACE_TRAIT | Interfaces/traits/protocols | Go, Python, TS, Rust, Java, C++ |
| P5_ENUM | Enumerations | All except config |
| P6_METHOD | Class/struct methods | All code languages |
| P7_TYPE_ALIAS | Type aliases | Go, Python, TS, Rust, C, C++ |

---

## TYPE_COMPAT Mappings

The UPTS runner handles cross-language type equivalences:

```python
TYPE_COMPAT = {
    # Structural: Go struct ≈ class in other languages
    "class": {"class", "struct"},
    "struct": {"class", "struct"},
    
    # Abstract: Go interface ≈ Rust trait ≈ Python Protocol
    "interface": {"interface", "trait", "protocol"},
    "trait": {"interface", "trait", "protocol"},
    "protocol": {"interface", "trait", "protocol"},
}
```

**Note:** These are semantic equivalences, NOT parser gap workarounds. All parser gaps have been fixed.

---

## Fixes Applied (Historical)

### Python Parser

| Issue | Fix | File |
|-------|-----|------|
| Protocol → `class` | Check superclasses for `Protocol` | `internal/symbols/parser.go` |
| Enum → `class` | Check superclasses for `Enum` | `internal/symbols/parser.go` |
| Methods have no `parent` | Track class context during AST walk | `internal/symbols/parser.go` |
| Type aliases → `variable` | `isPythonTypeAlias()` helper | `internal/symbols/parser.go` |

### TypeScript Parser

| Issue | Fix | File |
|-------|-----|------|
| Arrow functions → `constant` | Check `arrow_function` node type | `internal/symbols/parser.go` |
| Class methods not extracted | `extractTSClassMethods()` | `internal/symbols/parser.go` |
| Abstract classes skipped | Handle `abstract_class_declaration` | `internal/symbols/parser.go` |

### UPTS Runner

| Issue | Fix | File |
|-------|-----|------|
| shlex parsing errors | Use Python list for command | `upts_runner.py` |
| Fixture path resolution | Resolve relative to spec file | `upts_runner.py` |
| Line tolerance ignored | Add `--line-tolerance` flag | `upts_runner.py` |

---

## Verification Checklist

After implementing fixes, verify:

```bash
# 1. All tree-sitter grammars load
./bin/extract-symbols --json upts/fixtures/rust_test_fixture.rs

# 2. All fallback parsers work
./bin/extract-symbols --json upts/fixtures/yaml_test_fixture.yaml

# 3. UPTS validation passes
make test-parsers

# Expected output:
# Go: 17/17 (100%) PASS
# Python: 35/35 (100%) PASS
# TypeScript: 13/13 (100%) PASS
# Rust: 27/27 (100%) PASS
# C: 24/24 (100%) PASS
# C++: 30/30 (100%) PASS
# CUDA: 25/25 (100%) PASS
# Java: 32/32 (100%) PASS
# YAML: 21/21 (100%) PASS
# TOML: 15/15 (100%) PASS
# JSON: 14/14 (100%) PASS
# INI: 15/15 (100%) PASS
# Shell: 14/14 (100%) PASS
# Dockerfile: 16/16 (100%) PASS
# SQL: 17/17 (100%) PASS
# Cypher: 14/14 (100%) PASS
```

---

## Files Reference

### Created Files

| File | Purpose |
|------|---------|
| `upts/schema/upts.schema.json` | JSON Schema for UPTS specs |
| `upts/specs/*.upts.json` | 16 language specifications |
| `upts/fixtures/*` | 16 test fixtures |
| `upts/runners/upts_runner.py` | Python test runner |
| `fallback_parsers.go` | Fallback parser for config languages |

### Modified Files

| File | Change |
|------|--------|
| `internal/symbols/parser.go` | Python/TypeScript fixes, grammar loading |
| `cmd/extract-symbols/main.go` | Fallback integration |

---

## Next Steps

1. ✅ Tree-sitter languages passing (Go, Python, TypeScript)
2. ⏳ Load missing tree-sitter grammars (Rust, C, C++, Java)
3. ⏳ Integrate fallback parsers for config languages
4. ⏳ Run full UPTS validation
5. ⏳ Add to CI pipeline (`make test-parsers`)
6. ⏳ Add specs for additional languages (Kotlin, Swift, PHP, etc.)

---

## Troubleshooting

### "no grammar loaded for language: X"

**Cause:** Tree-sitter grammar not loaded for that language.

**Fix:** Add grammar to `loadGrammar()` in `internal/symbols/parser.go`:

```go
case "X":
    return X.GetLanguage(), nil
```

### Config language returns empty symbols

**Cause:** `extract-symbols` only tries tree-sitter, which doesn't support config languages.

**Fix:** Add `fallback_parsers.go` and integrate `TryFallbackParser()`.

### Line numbers don't match

**Cause:** Parser reports different line numbers than expected.

**Fix:**

1. Check `line_tolerance` in UPTS spec (default: 2)
2. Verify fixture hasn't changed
3. Check parser is using 1-indexed lines

### Parent not set for methods

**Cause:** Parser not tracking class context during AST walk.

**Fix:** Maintain current class/struct context during parsing:

```go
var currentClass string
// When entering class: currentClass = className
// When exiting class: currentClass = ""
// When extracting method: symbol.Parent = currentClass
```
