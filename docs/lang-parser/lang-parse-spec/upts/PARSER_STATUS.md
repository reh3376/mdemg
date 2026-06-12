# Parser Implementation Status

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Generated:** 2026-01-29
**Last Updated:** 2026-02-07
**Schema Version:** 1.5 (added Lua and Scraper Markdown parsers)
**Status:** 27/27 UPTS-validated languages passing (100%)

---

## Schema Fix Applied (v1.0 → v1.1)

**Issue:** Fixture paths in specs were relative to `upts/` root instead of relative to the spec file location.

```json
// v1.0 (broken):
"fixture": {
  "type": "file",
  "path": "fixtures/c_test_fixture.c"  // Wrong: relative to upts/
}

// v1.1 (fixed):
"fixture": {
  "type": "file", 
  "path": "../fixtures/c_test_fixture.c"  // Correct: relative to specs/
}
```

**Applied to:** All 16 spec files in `specs/*.upts.json`

---

## Runner Fixes Applied (v1.1)

**1. Parser command parsing:**

```python
# v1.0 (broken with complex commands):
subprocess.run([self.parser_cmd, str(file_path)], ...)

# v1.1 (handles quoted args):
subprocess.run(shlex.split(self.parser_cmd) + [str(file_path)], ...)
```

**2. Config-aware validation:**

```python
# Now respects config flags:
validate_signature = self.spec.config.get("validate_signature", False)
validate_value = self.spec.config.get("validate_value", False)
validate_parent = self.spec.config.get("validate_parent", True)
```

**3. Conditional parent validation:**

```python
# Only validates parent when config enables it:
if validate_parent and exp_parent and actual_parent != exp_parent:
    issues.append(...)
```

---

## Current Status

All 27 UPTS-validated parsers pass via `go test ./cmd/ingest-codebase/languages/ -run TestUPTS -v`:

| Language | Parser Type | Status | Notes |
|----------|-------------|--------|-------|
| Go | AST-based | ✅ PASS | Evidence validation enabled |
| Rust | Regex | ✅ PASS | Evidence validation enabled |
| Python | Regex | ✅ PASS | |
| TypeScript | Regex | ✅ PASS | Includes JS/JSX/TSX |
| Java | Regex | ✅ PASS | Brace-depth scope tracking |
| C# | Regex | ✅ PASS | Brace-depth scope tracking |
| Kotlin | Regex | ✅ PASS | Handles data/sealed/object |
| C++ | Regex | ✅ PASS | |
| C | Regex | ✅ PASS | |
| CUDA | Regex | ✅ PASS | Kernel, device, shared memory |
| Protocol Buffers | Regex | ✅ PASS | gRPC services, messages, enums |
| GraphQL | Regex | ✅ PASS | Types, interfaces, queries, mutations |
| OpenAPI/Swagger | Regex | ✅ PASS | **NEW** - REST endpoints, methods, schemas, parameters |
| SQL | Regex | ✅ PASS | |
| Cypher | Regex | ✅ PASS | Neo4j labels, constraints, indexes |
| Terraform | Regex | ✅ PASS | HCL resource/variable/output |
| YAML | Regex | ✅ PASS | Hierarchical key paths |
| TOML | Regex | ✅ PASS | Section and key extraction |
| JSON | Regex | ✅ PASS | |
| INI | Regex | ✅ PASS | Section and key extraction |
| Makefile | Regex | ✅ PASS | Targets, variables, .PHONY |
| Dockerfile | Regex | ✅ PASS | |
| Shell | Regex | ✅ PASS | |
| Lua | Regex | ✅ PASS | Functions, local variables, tables |
| Scraper Markdown | Regex (delegates to Markdown) | ✅ PASS | **NEW** - Web-scraped content; headings, code blocks, links |

**Pass Rate:** 100% (27/27)

All 27 parsers now have UPTS validation specs.

---

## Recent Changes (2026-02-07)

### New Parsers (Phase 51: Web Scraper)

| Parser | Features |
|--------|----------|
| Scraper Markdown | Delegates to MarkdownParser.ExtractSymbols(); used by web scraper for section chunking with UPTS-validated symbol extraction |
| Lua | Functions, local variables, module tables, metatables |

---

## Previous Changes (2026-02-05)

### New Parsers (Phase 5: API Schemas)

| Parser | Features |
|--------|----------|
| Protocol Buffers | Messages (with nesting), enums, services, RPC methods (with signatures), fields, oneof, package, options |
| GraphQL | Types, interfaces, inputs, enums, unions, scalars, directives, Query/Mutation/Subscription, fields, extend type |
| OpenAPI/Swagger | REST endpoints, HTTP methods, operationIds, parameters, schemas, security schemes, server URLs |

### Previously Added Parsers (Phase 4: Extended)

| Parser | Features |
|--------|----------|
| C# | Classes, structs, interfaces, enums, records, properties, methods, constants, attributes, namespaces |
| Kotlin | Data/sealed/abstract classes, objects, companion objects, interfaces, enum classes, typealiases, extension functions |
| Terraform | Resources, data sources, modules, providers, variables (with default/description), outputs, locals |
| Makefile | Targets (with .PHONY export tracking), variables (all assignment operators), define/endef macros |

### Evidence Validation

Added `validate_evidence` config flag. When enabled, the test harness runs structural consistency checks:

- LineEnd >= Line for symbols
- StartLine <= EndLine for CodeElements
- Symbol Line within CodeElement range
- LineEnd matching against spec (within tolerance)

Currently enabled for Go and Rust. Other parsers can opt in.

### Diagnostics Framework

All new parsers emit `TRUNCATED` diagnostic when content exceeds 4000 chars. See [`interface.go`](../../../../cmd/ingest-codebase/languages/interface.go) for the `Diagnostic` struct.

### Ingestion Whitelist Fix

`getEnabledLanguages()` in `main.go` now includes all 22 parsers. Previously missing: yaml, toml, ini, dockerfile, shell, cuda, cypher.

---

## Archived: Previous Build Configuration Issues

*All previously documented issues have been resolved.*

### Historical Issue

Fix:

```

---

## Completed Fixes

### Tree-sitter Languages (7/7)

- ✅ Go - Original
- ✅ Python - Fixed parent parsing
- ✅ TypeScript - Original
- ✅ Rust - Added grammar
- ✅ C - Added grammar
- ✅ C++ - Added grammar
- ✅ Java - Added grammar

### Config Languages (4/8)

- ✅ YAML - Fallback parser
- ✅ TOML - Fallback parser
- ✅ JSON - Fallback parser
- ✅ INI/dotenv - Fallback parser

---

## Verification

```bash
# Run all specs
make test-parsers

# Expected output after build fixes:
# 16/16 languages passing (100%)
```

---

## Summary

| Category | Passing | Total | Rate |
|----------|---------|-------|------|
| Tree-sitter | 7 | 8 | 88% |
| Config | 4 | 8 | 50% |
| **Overall** | **11** | **16** | **69%** |

After build config changes: **16/16 (100%)**
