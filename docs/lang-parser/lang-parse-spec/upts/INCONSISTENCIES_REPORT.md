# Parser Test Specification Inconsistencies Report

**Date:** 2026-01-29  
**Files Analyzed:**

- `go_expected.json`
- `python_expected.json`
- `typescript_expected.json`

---

## Executive Summary

Your three existing `*_expected.json` files have **17 distinct inconsistencies** across field naming, structure, semantics, and completeness. These inconsistencies make it difficult to:

1. Write a single test runner that works across all languages
2. Compare parser quality across languages
3. Add new languages without ambiguity
4. Maintain consistency as parsers evolve

---

## Critical Inconsistencies

### 1. Line Number Field Name

| Language | Field Name | Example |
|----------|------------|---------|
| Go | `line` | `"line": 13` |
| Python | `line_number` | `"line_number": 12` |
| TypeScript | `line` | `"line": 5` |

**Impact:** Test runners must handle both field names or fail silently.

**UPTS Fix:** Standardize on `line` (shorter, matches source terminology).

---

### 2. Top-Level Language Identifier

| Language | Field Name | Value |
|----------|------------|-------|
| Go | `parser` | `"go"` |
| Python | `language` | `"python"` |
| TypeScript | `parser` | `"typescript"` |

**Impact:** Cannot programmatically determine which parser a spec is for.

**UPTS Fix:** Standardize on `language` (more accurate—the file describes a language, not a parser).

---

### 3. Fixture Path Field Name

| Language | Field Name | Value |
|----------|------------|-------|
| Go | `fixture` | `"go_test_fixture.go"` |
| Python | `fixture_file` | `"python_test_fixture.py"` |
| TypeScript | `fixture` | `"typescript_test_fixture.ts"` |

**Impact:** Test runners must check multiple field names.

**UPTS Fix:** Standardize on `fixture.path` (nested object allows for inline content option).

---

### 4. Line Tolerance Presence

| Language | Has `line_tolerance`? | Value |
|----------|----------------------|-------|
| Go | ✅ Yes | `2` |
| Python | ❌ No | - |
| TypeScript | ✅ Yes | `2` |

**Impact:** Python tests have undefined tolerance behavior.

**UPTS Fix:** Required field in `config` section with default value of `2`.

---

## Structural Inconsistencies

### 5. Optional Fields Vary by Language

| Field | Go | Python | TypeScript |
|-------|:--:|:------:|:----------:|
| `value` | ❌ | ✅ | ❌ |
| `signature` | ❌ | ✅ | ❌ |
| `doc_comment` | ❌ | ✅ | ❌ |
| `decorators` | ❌ | ❌ | ❌ |

**Example - Python has `value`:**

```json
{"name": "MAX_RETRIES", "type": "constant", "line_number": 12, "value": "3"}
```

**Example - Go lacks `value`:**

```json
{"name": "MaxRetries", "type": "constant", "line": 13}
```

**Impact:** Cannot validate constant extraction consistently across languages.

**UPTS Fix:** Define all fields as optional in schema; parsers include when available.

---

### 6. Parser Status Metadata

| Language | Has Status? | Value |
|----------|-------------|-------|
| Go | ❌ No | - |
| Python | ✅ Yes | `"Enhanced 2026-01-28"` |
| TypeScript | ❌ No | - |

**Impact:** No tracking of parser maturity across languages.

**UPTS Fix:** Add `metadata.parser_status` enum: `basic`, `functional`, `enhanced`, `complete`.

---

### 7. Enhancement Documentation

| Language | Has Enhancements? |
|----------|-------------------|
| Go | ❌ No |
| Python | ✅ Yes (`enhancements_made` object) |
| TypeScript | ❌ No |

**Python Example:**

```json
"enhancements_made": {
  "methods_extraction": "Methods inside classes now correctly typed as 'method' with Parent set",
  "enum_detection": "Enum subclasses detected as 'enum' type"
}
```

**Impact:** No audit trail for parser improvements in Go/TypeScript.

**UPTS Fix:** Optional `metadata.enhancements` or `metadata.changelog`.

---

## Semantic Inconsistencies

### 8. `parent` Field Has Different Meanings

**Go/TypeScript:** `parent` means "method belongs to this class/struct"

```json
{"name": "FindByID", "type": "method", "parent": "UserService"}
```

**Python:** `parent` means BOTH:

- Method belongs to class: `{"name": "find_by_id", "parent": "UserService"}`
- Class inherits from: `{"name": "User", "parent": "BaseEntity"}`
- Enum inherits from: `{"name": "Status", "parent": "Enum"}`

**Impact:** `parent` is overloaded with two different meanings in Python.

**UPTS Fix:**

- Use `parent` only for method ownership
- Add `extends` for class inheritance
- Add `base_class` in `language_specific` for Python enums/protocols

---

### 9. `exported` Field Inconsistently Applied

| Context | Go | Python | TypeScript |
|---------|:--:|:------:|:----------:|
| Constants | ✅ | ✅ | ✅ |
| Functions | ✅ | ✅ | ✅ |
| Classes | ✅ | ✅ | ✅ |
| Methods | ❌ | ✅ | ❌ |
| Interfaces | ✅ | ✅ | ✅ |

**Go methods lack `exported`:**

```json
{"name": "FindByID", "type": "method", "line": 50, "parent": "UserService"}
```

**Python methods have `exported`:**

```json
{"name": "find_by_id", "type": "method", "line_number": 60, "exported": true, "parent": "UserRepository"}
```

**Impact:** Cannot consistently test method visibility rules.

**UPTS Fix:** `exported` required for all symbols; methods inherit from parent if not explicit.

---

### 10. Type Names Not Normalized

| Concept | Go | Python | TypeScript |
|---------|-----|--------|------------|
| Class/Struct | `struct` | `class` | `class` |
| Interface | `interface` | `interface` | `interface` |
| Protocol | N/A | `interface` (for Protocol) | N/A |
| Trait | N/A | N/A | N/A |

**Impact:** Test runners must know `struct` ≈ `class` for comparison.

**UPTS Fix:** Define type compatibility groups in schema:

```json
{"class": ["class", "struct"], "interface": ["interface", "trait", "protocol"]}
```

---

## Missing Features

### 11. No Pattern Tagging

None of the files link symbols to canonical patterns (P1-P7).

**Impact:** Cannot verify all 7 patterns are tested per language.

**UPTS Fix:** Add `pattern` field: `"P1_CONSTANT"`, `"P2_FUNCTION"`, etc.

---

### 12. No Relationship Testing

None of the files test graph relationships.

**Example relationships that should be tested:**

- `UserService` → `DEFINES_METHOD` → `findById`
- `AdminUser` → `EXTENDS` → `BaseEntity`
- `AdminUser` → `IMPLEMENTS` → `UserDto`

**Impact:** Cannot validate parser produces correct graph edges.

**UPTS Fix:** Add `relationships` array to `expected`.

---

### 13. No Exclusion Testing

None of the files explicitly test what should NOT be extracted.

**Examples that should be excluded:**

- Private struct fields (`repo`, `logger` in Go)
- Constructor parameters
- Node modules / vendor code

**Impact:** Parsers might over-extract without detection.

**UPTS Fix:** Add `excluded` array with reasons.

---

### 14. No Signature Validation (Go/TypeScript)

| Language | Has Signature Testing? |
|----------|------------------------|
| Go | ❌ No |
| Python | ✅ Yes (full signature) |
| TypeScript | ❌ No |

**Python Example:**

```json
{"name": "calculate_total", "signature": "def calculate_total(items: List[\"Item\"]) -> int"}
```

**Impact:** Cannot verify function signatures in Go/TypeScript.

**UPTS Fix:** Add `signature` and `signature_contains` (partial match) fields.

---

### 15. No Fixture Integrity Verification

None of the files include a hash of the fixture content.

**Impact:** Fixture changes can silently break tests.

**UPTS Fix:** Add optional `fixture.sha256` field.

---

## Minor Inconsistencies

### 16. Expected Count Position Varies

**Go/TypeScript:** `expected_count` at end

```json
{
  "expected_symbols": [...],
  "expected_count": 17,
  "line_tolerance": 2
}
```

**Python:** `expected_count` near top

```json
{
  "language": "python",
  "fixture_file": "...",
  "expected_count": 35,
  "parser_status": "...",
  "expected_symbols": [...]
}
```

**Impact:** Minor, but suggests lack of standard structure.

---

### 17. Symbol Array Field Name

All three use `expected_symbols`, which is good—but the internal structure varies (as noted above).

---

## Inconsistency Matrix

| Feature | Go | Python | TypeScript | UPTS Standardized |
|---------|:--:|:------:|:----------:|:-----------------:|
| Line field | `line` | `line_number` | `line` | `line` |
| Language field | `parser` | `language` | `parser` | `language` |
| Fixture field | `fixture` | `fixture_file` | `fixture` | `fixture.path` |
| Line tolerance | ✅ | ❌ | ✅ | `config.line_tolerance` |
| Value field | ❌ | ✅ | ❌ | Optional |
| Signature field | ❌ | ✅ | ❌ | Optional |
| Doc comment | ❌ | ✅ | ❌ | Optional |
| Parser status | ❌ | ✅ | ❌ | `metadata.parser_status` |
| Enhancements | ❌ | ✅ | ❌ | `metadata.enhancements` |
| Pattern tags | ❌ | ❌ | ❌ | `pattern` |
| Relationships | ❌ | ❌ | ❌ | `relationships[]` |
| Exclusions | ❌ | ❌ | ❌ | `excluded[]` |
| Fixture hash | ❌ | ❌ | ❌ | `fixture.sha256` |

---

## Recommendations

### Immediate Actions

1. **Adopt UPTS format** for all new parser tests
2. **Convert existing files** using: `python upts_runner.py convert`
3. **Add missing fields** to Go/TypeScript specs (signatures, values)

### Schema Enforcement

1. **Validate specs against JSON Schema** before commit
2. **CI check** to reject non-conforming specs
3. **Auto-generate** skeleton specs for new languages

### Documentation

1. **Document canonical patterns** P1-P7 with examples per language
2. **Document type mappings** (struct↔class, protocol↔interface)
3. **Maintain changelog** in `metadata.enhancements`

---

## Appendix: Field Mapping Reference

### Old → UPTS Conversion

| Old Field (varies) | UPTS Field |
|--------------------|------------|
| `parser` / `language` | `language` |
| `fixture` / `fixture_file` | `fixture.path` |
| `line` / `line_number` | `line` |
| `expected_count` | `expected.symbol_count.min` |
| `line_tolerance` | `config.line_tolerance` |
| `parser_status` | `metadata.parser_status` |
| `enhancements_made` | `metadata.enhancements` |
| `expected_symbols` | `expected.symbols` |

### Type Compatibility Groups

```json
{
  "class": ["class", "struct"],
  "interface": ["interface", "trait", "protocol"],
  "function": ["function"],
  "method": ["method"],
  "constant": ["constant", "const"],
  "type": ["type", "type_alias", "typedef"],
  "enum": ["enum"]
}
```
