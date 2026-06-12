# Tree-Sitter Grammar Issues for 5 Languages

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date:** 2026-01-29
**Status:** 11/16 UPTS specs passing, 5 need tree-sitter grammars

## Problem Summary

The following 5 languages return `ERROR` status because the tree-sitter parser cannot parse them - the grammars are not loaded/compiled into the binary:

| Language | Spec File | Fixture File | Expected Symbols |
|----------|-----------|--------------|------------------|
| C | `upts/specs/c.upts.json` | `upts/fixtures/c_test_fixture.c` | 24 |
| C++ | `upts/specs/cpp.upts.json` | `upts/fixtures/cpp_test_fixture.cpp` | 30 |
| CUDA | `upts/specs/cuda.upts.json` | `upts/fixtures/cuda_test_fixture.cu` | 25 |
| Java | `upts/specs/java.upts.json` | `upts/fixtures/java_test_fixture.java` | 32 |
| Rust | `upts/specs/rust.upts.json` | `upts/fixtures/rust_test_fixture.rs` | 27 |

## Root Cause

The Go symbol parser uses tree-sitter via `github.com/smacker/go-tree-sitter`. Each language requires:

1. A tree-sitter grammar (C library)
2. Go bindings for that grammar
3. Registration in the parser's language detection

Currently, only these languages have working tree-sitter support:

- Go, Python, TypeScript (working)

## Files to Review

### 1. Parser Entry Point

**File:** `/Users/reh3376/mdemg/internal/symbols/parser.go`

This file contains the `Parse()` function and language detection. Look for:

- How languages are mapped to tree-sitter parsers
- The `getLanguage()` or similar function
- Import statements for tree-sitter bindings

### 2. Language-Specific Parsers

**Directory:** `/Users/reh3376/mdemg/cmd/ingest-codebase/languages/`

| File | Purpose |
|------|---------|
| `c_parser.go` | C language extraction logic |
| `cpp_parser.go` | C++ language extraction logic |
| `cuda_parser.go` | CUDA language extraction logic |
| `java_parser.go` | Java language extraction logic |
| `rust_parser.go` | Rust language extraction logic |
| `interface.go` | Common Symbol struct definition |

### 3. Go Module Dependencies

**File:** `/Users/reh3376/mdemg/go.mod`

Check for tree-sitter grammar dependencies. You may need to add:

```go
github.com/smacker/go-tree-sitter
github.com/tree-sitter/tree-sitter-c
github.com/tree-sitter/tree-sitter-cpp
github.com/tree-sitter/tree-sitter-java
github.com/tree-sitter/tree-sitter-rust
// CUDA may need custom grammar or cpp grammar with .cu extension
```

### 4. Extract-Symbols Binary

**File:** `/Users/reh3376/mdemg/cmd/extract-symbols/main.go`

This is the CLI that UPTS runner calls. Check how it routes to the parser.

## What Needs to Happen

1. **Add grammar dependencies** to `go.mod`
2. **Register grammars** in the parser initialization
3. **Map file extensions** to correct grammars:
   - `.c`, `.h` → C grammar
   - `.cpp`, `.cc`, `.cxx`, `.hpp` → C++ grammar
   - `.cu`, `.cuh` → CUDA (likely C++ grammar)
   - `.java` → Java grammar
   - `.rs` → Rust grammar

4. **Test each language:**

```bash
./bin/extract-symbols --json docs/lang-parser/lang-parse-spec/upts/fixtures/c_test_fixture.c
./bin/extract-symbols --json docs/lang-parser/lang-parse-spec/upts/fixtures/cpp_test_fixture.cpp
./bin/extract-symbols --json docs/lang-parser/lang-parse-spec/upts/fixtures/cuda_test_fixture.cu
./bin/extract-symbols --json docs/lang-parser/lang-parse-spec/upts/fixtures/java_test_fixture.java
./bin/extract-symbols --json docs/lang-parser/lang-parse-spec/upts/fixtures/rust_test_fixture.rs
```

## Verification Command

After fixing, run:

```bash
for lang in c cpp cuda java rust; do
  python3 docs/lang-parser/lang-parse-spec/upts/runners/upts_runner.py validate \
    --spec="docs/lang-parser/lang-parse-spec/upts/specs/${lang}.upts.json" \
    --parser="./bin/extract-symbols --json" 2>&1 | grep -E "Status:|Matched:"
done
```

## Notes

- CUDA typically uses C++ grammar since it's a superset
- The specs and fixtures are already complete - only parser infrastructure is missing
- Fallback parsers won't help here - these need real AST parsing for accurate symbol extraction
